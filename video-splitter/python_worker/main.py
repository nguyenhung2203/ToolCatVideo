"""
Python Worker — Smart Video Analyzer
Kiến trúc: proxy đọc MỘT LẦN, tính đồng thời scene+layout+black, cluster trước Pass 2.

P0 đã fix:
- Smart mode KHÔNG âm thầm fallback sang source nếu thiếu proxy
- Proxy chỉ đọc 1 lần: scene + layout + black tính đồng thời trong cùng vòng lặp
- Cluster candidates TRƯỚC Pass 2 (tránh 300 seek riêng lẻ)
- Lấy silence_end (hoặc midpoint) thay vì luôn lấy silence_start
- Merge window theo loại tín hiệu (hard cut 0.4s, silence 0.8s, black 0.6s)
- Timestamp ưu tiên: hard cut > black midpoint > audio boundary > silence_start
- Thêm min_confidence filter
- Fast mode KHÔNG dùng keyframe làm cut point
- Đo thời gian từng bước
"""

import sys
import io
import json
import argparse
import subprocess
import re
import time
import multiprocessing
import os
import ctypes
from concurrent.futures import ThreadPoolExecutor

# Force stdout/stderr to use UTF-8 encoding to prevent charmap codec errors on Windows with Vietnamese characters
if sys.stdout and sys.stdout.encoding != 'utf-8':
    try:
        sys.stdout.reconfigure(encoding='utf-8')
    except AttributeError:
        sys.stdout = io.TextIOWrapper(sys.stdout.buffer, encoding='utf-8')

if sys.stderr and sys.stderr.encoding != 'utf-8':
    try:
        sys.stderr.reconfigure(encoding='utf-8')
    except AttributeError:
        sys.stderr = io.TextIOWrapper(sys.stderr.buffer, encoding='utf-8')


# ──────────────────────────────────────────────────────────
# Utilities
# ──────────────────────────────────────────────────────────
def get_short_path(path):
    if not path or os.name != 'nt':
        return path
    try:
        buf = ctypes.create_unicode_buffer(1024)
        ctypes.windll.kernel32.GetShortPathNameW(path, buf, 1024)
        return buf.value or path
    except Exception:
        return path


def _log_time(label, t0):
    elapsed = time.time() - t0
    print(f"STATUS_LOG:[TIMING] {label}: {elapsed:.2f}s", flush=True)
    return elapsed


# ──────────────────────────────────────────────────────────
# Thống kê robust — dùng cho adaptive threshold & chuẩn hoá cường độ
# ──────────────────────────────────────────────────────────
def robust_threshold(vals, k, floor):
    """Ngưỡng thích ứng theo phân phối: median + k * (1.4826 * MAD).

    Dùng median + MAD (Median Absolute Deviation) thay vì mean + std vì phân phối
    frame-diff heavy-tailed: một vài cut mạnh kéo lệch std, khiến ngưỡng mean+std
    quá cao và bỏ sót cut vừa. MAD chống outlier tốt hơn nhiều.

    floor: sàn tối thiểu để video quá đồng nhất (gần như không đổi) không sinh
    ngưỡng gần 0 rồi bắt nhiễu lung tung.
    """
    import numpy as np
    if vals is None or len(vals) == 0:
        return floor
    arr = np.asarray(vals, dtype=np.float64)
    med = float(np.median(arr))
    mad = float(np.median(np.abs(arr - med)))
    thr = med + k * 1.4826 * mad
    return max(floor, thr)


def intensity_from(value, thr, hi):
    """Chuẩn hoá một metric thành cường độ 0-100 theo khoảng cách TRÊN ngưỡng.

    value <= thr        → 0   (dưới ngưỡng, không tính là tín hiệu)
    value >= hi         → 100 (mạnh tối đa, hi thường là p99 của phân phối)
    ở giữa              → nội suy tuyến tính

    Nhờ đó mọi tín hiệu (scene/layout/black/silence/audio) đều nằm trên CÙNG một
    thang 0-100, để trọng số bên Go (boundaryScore) so sánh công bằng.
    """
    if hi <= thr:
        return 100 if value > thr else 0
    frac = (value - thr) / (hi - thr)
    if frac <= 0:
        return 0
    if frac >= 1:
        return 100
    return int(round(frac * 100))


def _percentile(vals, q, default=0.0):
    import numpy as np
    if vals is None or len(vals) == 0:
        return default
    return float(np.percentile(np.asarray(vals, dtype=np.float64), q))


def _median(vals, default=0.0):
    import numpy as np
    if vals is None or len(vals) == 0:
        return default
    return float(np.median(np.asarray(vals, dtype=np.float64)))


# ──────────────────────────────────────────────────────────
# Silence detection — trả về toàn bộ regions (start, end, best_cut)
# ──────────────────────────────────────────────────────────
def analyze_silence_regions(source_path, ffmpeg_path="ffmpeg", silence_db=-30, silence_duration=0.5):
    """Trả về list dict {start, end, dur, best_cut}.
    best_cut = silence_end (clip mới thường bắt đầu ngay sau khoảng lặng),
               fallback về midpoint nếu khoảng lặng ngắn.
    """
    print("STATUS_LOG:Phát hiện khoảng lặng: Đang phân tích...", flush=True)
    t0 = time.time()
    cmd = [
        ffmpeg_path, '-hide_banner', '-nostdin', '-nostats',
        '-i', source_path, '-vn',
        '-af', f'silencedetect=noise={silence_db}dB:d={silence_duration}',
        '-f', 'null', '-'
    ]
    regions = []
    try:
        r = subprocess.run(cmd, stderr=subprocess.PIPE, text=True, timeout=1800)
        stderr = r.stderr
        starts  = [float(m.group(1)) for m in re.finditer(r'silence_start:\s*([\d.]+)', stderr)]
        ends    = [float(m.group(1)) for m in re.finditer(r'silence_end:\s*([\d.]+)', stderr)]
        durs    = [float(m.group(1)) for m in re.finditer(r'silence_duration:\s*([\d.]+)', stderr)]

        for i, start in enumerate(starts):
            end  = ends[i]  if i < len(ends)  else None
            dur  = durs[i]  if i < len(durs)  else (end - start if end else 0.0)
            if end is not None:
                # Clip mới thường bắt đầu ở silence_end;
                # nếu khoảng lặng < 0.8s thì lấy midpoint để tránh cắt quá sớm/muộn
                best_cut = end if dur >= 0.8 else (start + end) / 2.0
            else:
                best_cut = start
            regions.append({'start': start, 'end': end, 'dur': dur, 'best_cut': best_cut})
    except Exception as e:
        print(f"silence error: {e}", file=sys.stderr)

    _log_time("silence_detect", t0)
    print(f"STATUS_LOG:Phát hiện khoảng lặng: {len(regions)} vùng.", flush=True)
    return regions


# ──────────────────────────────────────────────────────────
# Unified proxy scan — MỘT lần đọc, tính đồng thời scene+layout+black
# ──────────────────────────────────────────────────────────
def scan_proxy_unified(proxy_path, scene_threshold=20.0, k_detect=3.0):
    """Quét proxy VIDEO MỘT LẦN (2 pha trong cùng 1 lần decode):

    k_detect: hệ số nhân MAD khi tính ngưỡng thích nghi (robust_threshold).
      Thấp → ngưỡng thấp → bắt NHIỀU điểm cắt hơn (chế độ Kỹ dùng k thấp để
      không bỏ sót chuyển cảnh vừa/yếu). Cao → nghiêm ngặt hơn, ít điểm rác.

    PHA 1 — thu thập metric mỗi frame (KHÔNG threshold ngay):
      - histogram diff (Bhattacharyya) → scene change
      - mean abs diff (MAD)            → hard cut (chính xác nhất)
      - edge/col energy diff           → layout change
      - brightness mean                → black frame

    PHA 2 — tính NGƯỠNG THÍCH NGHI từ phân phối (median + MAD, chống heavy-tail)
    rồi lọc candidate và CHUẨN HOÁ CƯỜNG ĐỘ về 0-100 nhất quán.

    Trả về danh sách raw_candidates:
        {ts, scene, layout, hard_cut, black,          # giá trị thô (dùng để cluster)
         scene_i, layout_i, hard_cut_i, black_i}      # cường độ 0-100 (dùng để chấm điểm)

    Vì proxy 320×180 @ ~4fps, mỗi frame chỉ lưu ~5 số float → RAM không đáng kể
    (1h video ≈ 14.4k frame). Đổi lại: ngưỡng tự thích nghi theo từng video thay
    vì hằng số cảm tính, hoạt động tốt trên cả video tối/sáng/nhiều-ít chuyển cảnh.
    """
    try:
        import cv2
        import numpy as np
    except Exception as e:
        print(f"cv2 loi: {e}", file=sys.stderr)
        return []

    if not proxy_path or not os.path.exists(proxy_path):
        print("STATUS_LOG:[WARN] proxy_path trống/không tồn tại, bỏ qua scan.", flush=True)
        return []

    t0 = time.time()
    cap = cv2.VideoCapture(proxy_path)
    if not cap.isOpened():
        print(f"Khong mo duoc proxy: {proxy_path}", file=sys.stderr)
        return []

    fps          = cap.get(cv2.CAP_PROP_FPS) or 4.0
    total_frames = int(cap.get(cv2.CAP_PROP_FRAME_COUNT))
    duration     = total_frames / fps

    print(f"STATUS_LOG:Quét proxy: {total_frames} frame @ {fps:.1f} FPS ({duration:.0f}s)...", flush=True)

    # ── PHA 1: thu thập metric mỗi frame (không lọc) ──
    metrics = []   # [{ts, scene, layout, hard_cut, brightness}]
    prev_hist = None
    prev_gray = None
    prev_edge = None
    idx = 0
    report_step = max(1, total_frames // 10)

    while True:
        ok, frame = cap.read()
        if not ok or frame is None:
            break

        gray = cv2.cvtColor(frame, cv2.COLOR_BGR2GRAY)
        brightness = float(np.mean(gray))

        hist = cv2.calcHist([gray], [0], None, [64], [0, 256])
        cv2.normalize(hist, hist)

        col_e = np.mean(np.abs(np.diff(gray.astype(np.float32), axis=1)), axis=0)
        norm_e = np.linalg.norm(col_e) + 1e-6
        col_e = col_e / norm_e

        if prev_hist is not None:
            hist_d = cv2.compareHist(prev_hist, hist, cv2.HISTCMP_BHATTACHARYYA)
            edge_d = float(np.linalg.norm(col_e - prev_edge))
            mad    = float(np.mean(np.abs(gray.astype(np.float32) - prev_gray.astype(np.float32)))) / 255.0
            metrics.append({
                'ts':         idx / fps,
                'scene':      hist_d,
                'layout':     edge_d,
                'hard_cut':   mad,
                'brightness': brightness,
            })

        prev_hist = hist
        prev_gray = gray
        prev_edge = col_e
        idx += 1

        if idx % report_step == 0:
            pct = int(idx / total_frames * 100) if total_frames > 0 else 0
            print(f"STATUS_LOG:Quét proxy: {pct}% ({len(metrics)} frame)...", flush=True)

    cap.release()

    if not metrics:
        _log_time("proxy_scan", t0)
        return []

    # ── PHA 2: tính ngưỡng thích nghi + chuẩn hoá cường độ ──
    scene_arr  = [m['scene']    for m in metrics]
    layout_arr = [m['layout']   for m in metrics]
    mad_arr    = [m['hard_cut'] for m in metrics]
    bri_arr    = [m['brightness'] for m in metrics]

    # scene_threshold (UI, mặc định 20) map thành hệ số k: slider cao → k cao → ít
    # candidate hơn (nghiêm ngặt hơn). Giữ slider có ý nghĩa với người dùng.
    k_scale = max(0.3, scene_threshold / 20.0)
    # k_eff: gộp knob độ nhạy (k_scale) với hệ số theo mode (k_detect). Mode Kỹ
    # truyền k_detect thấp (~2.0) để hạ ngưỡng, bắt thêm chuyển cảnh vừa/yếu.
    k_eff = k_detect * k_scale
    scene_thr  = robust_threshold(scene_arr,  k_eff, floor=0.08)
    layout_thr = robust_threshold(layout_arr, k_eff, floor=0.15)
    mad_thr    = robust_threshold(mad_arr,    k_eff, floor=0.12)
    # Black: tương đối theo độ sáng tổng thể → video tối tuyệt đối vẫn phát hiện được
    # cú "sụt sáng" (dip), video sáng đều không báo nhầm.
    black_thr  = min(16.0, 0.25 * _median(bri_arr))

    # p99 để chuẩn hoá cường độ (khoảng cách trên ngưỡng, kẹp tại phân vị 99)
    scene_p99  = _percentile(scene_arr,  99.0)
    layout_p99 = _percentile(layout_arr, 99.0)
    mad_p99    = _percentile(mad_arr,    99.0)

    raw_candidates = []
    for m in metrics:
        is_black = m['brightness'] < black_thr
        hit = (m['scene'] > scene_thr or m['layout'] > layout_thr
               or m['hard_cut'] > mad_thr or is_black)
        if not hit:
            continue
        # Cường độ đen: càng tối dưới ngưỡng càng mạnh.
        black_i = 0
        if is_black:
            black_i = intensity_from(black_thr - m['brightness'], 0.0, black_thr)
        raw_candidates.append({
            'ts':         m['ts'],
            'scene':      m['scene'],
            'layout':     m['layout'],
            'hard_cut':   m['hard_cut'],
            'black':      1.0 if is_black else 0.0,
            'scene_i':    intensity_from(m['scene'],    scene_thr,  scene_p99),
            'layout_i':   intensity_from(m['layout'],   layout_thr, layout_p99),
            'hard_cut_i': intensity_from(m['hard_cut'], mad_thr,    mad_p99),
            'black_i':    black_i,
        })

    _log_time("proxy_scan", t0)
    print(f"STATUS_LOG:Ngưỡng thích nghi: scene>{scene_thr:.3f} layout>{layout_thr:.3f} "
          f"hardcut>{mad_thr:.3f} black<{black_thr:.1f}", flush=True)
    print(f"STATUS_LOG:Quét proxy xong: {len(raw_candidates)}/{len(metrics)} frame vượt ngưỡng.", flush=True)
    return raw_candidates


# ──────────────────────────────────────────────────────────
# Cluster candidates TRƯỚC Pass 2 (tránh 300 lần seek riêng lẻ)
# ──────────────────────────────────────────────────────────
def cluster_raw_candidates(raw_candidates, cluster_window=0.6):
    """Gộp các điểm gần nhau thành cluster, trả về một representative mỗi cluster.
    Representative = điểm có score tổng cao nhất trong cluster.
    """
    if not raw_candidates:
        return []
    sorted_cands = sorted(raw_candidates, key=lambda x: x['ts'])
    clusters = []
    current = [sorted_cands[0]]
    for c in sorted_cands[1:]:
        if c['ts'] - current[0]['ts'] <= cluster_window:
            current.append(c)
        else:
            clusters.append(current)
            current = [c]
    clusters.append(current)

    result = []
    for cluster in clusters:
        # Chọn điểm tốt nhất: ưu tiên black, rồi hard_cut, rồi scene
        best = max(cluster, key=lambda x: x['black'] * 3 + x['hard_cut'] * 2 + x['scene'])
        result.append(best)
    return result


# ──────────────────────────────────────────────────────────
# Pass 2 — refine clusters (batch seek theo vùng đã cluster)
# ──────────────────────────────────────────────────────────
def refine_clusters(proxy_path, clusters, fps=4.0, window_sec=0.5):
    """Quét kỹ ±window_sec quanh mỗi cluster representative.
    Trả về list timestamp đã tinh chỉnh.
    """
    try:
        import cv2
        import numpy as np
    except Exception as e:
        print(f"cv2 loi refine: {e}", file=sys.stderr)
        return [c['ts'] for c in clusters]

    if not clusters or not proxy_path or not os.path.exists(proxy_path):
        return [c['ts'] for c in clusters]

    t0 = time.time()
    cap = cv2.VideoCapture(proxy_path)
    if not cap.isOpened():
        return [c['ts'] for c in clusters]

    fps_actual   = cap.get(cv2.CAP_PROP_FPS) or fps
    total_frames = int(cap.get(cv2.CAP_PROP_FRAME_COUNT))
    win_f        = max(1, int(window_sec * fps_actual))

    refined = []
    for cluster in clusters:
        rough_ts = cluster['ts']
        sf = max(0, int(rough_ts * fps_actual) - win_f)
        ef = min(total_frames - 1, int(rough_ts * fps_actual) + win_f)
        cap.set(cv2.CAP_PROP_POS_FRAMES, sf)

        prev_g, best_ts, best_d = None, rough_ts, 0.0
        for fi in range(sf, ef + 1):
            ok, f = cap.read()
            if not ok or f is None:
                break
            g = cv2.cvtColor(f, cv2.COLOR_BGR2GRAY).astype(np.float32)
            if prev_g is not None:
                d = float(np.mean(np.abs(g - prev_g))) / 255.0
                if d > best_d:
                    best_d = d
                    best_ts = fi / fps_actual
            prev_g = g

        # Ưu tiên timestamp của hard cut / black frame nếu score cao
        if cluster['black'] >= 0.5:
            # Black frame: dùng frame đầu của vùng đen làm timestamp
            refined.append(best_ts)
        else:
            refined.append(best_ts)

    cap.release()
    _log_time("refine_pass2", t0)
    return refined


# ──────────────────────────────────────────────────────────
# Pass 2 (chính xác cao) — refine trên SOURCE full-fps thay vì proxy 4fps
# ──────────────────────────────────────────────────────────
def refine_clusters_on_source(source_path, clusters, ffmpeg_path="ffmpeg",
                              window_sec=0.5, max_refine=200):
    """Tinh chỉnh timestamp điểm cắt bằng cách decode cửa sổ NHỎ ở SOURCE full-fps.

    Vì sao: refine trên proxy 4fps không thể vượt sai số ±0.125s. Ở đây seek vào
    source, decode ±window_sec quanh mỗi cluster ở fps GỐC (scale 160×90 gray), và
    LẤY TIMESTAMP THẬT của từng khung qua filter showinfo (pts_time) thay vì suy ra
    từ idx/fps. Nhờ vậy:
      - Chính xác ~1 khung ở fps gốc (30fps ≈ 0.033s).
      - Đúng cho cả VFR (khung không đều) vì dùng pts thật, không giả định đều.
      - Không phụ thuộc seek nhảy về keyframe: pts_time (KHÔNG -copyts) tính từ đầu
        cửa sổ seek nên cộng vào `seek` ra timestamp trong cùng hệ 0-based của pipeline.

    - black cluster: khung ĐẦU TIÊN có brightness thấp (black onset).
    - hard cut : khung có MAD lớn nhất so với khung trước.
    Chỉ refine top-`max_refine` cluster mạnh nhất để giới hạn số lần seek source.

    Trả về list timestamp đã refine, THEO ĐÚNG THỨ TỰ clusters đầu vào.
    """
    try:
        import numpy as np
    except Exception as e:
        print(f"numpy loi refine-source: {e}", file=sys.stderr)
        return [c['ts'] for c in clusters]

    if not clusters or not source_path or not os.path.exists(source_path):
        return [c['ts'] for c in clusters]

    t0 = time.time()
    W, H = 160, 90
    frame_bytes = W * H

    # Chọn top cluster theo cường độ để giới hạn chi phí seek (giữ nguyên index gốc).
    def _strength(c):
        return max(c.get('scene_i', 0), c.get('hard_cut_i', 0),
                   c.get('layout_i', 0), c.get('black_i', 0))
    order = sorted(range(len(clusters)), key=lambda i: _strength(clusters[i]), reverse=True)
    refine_set = set(order[:max_refine])

    refined = [c['ts'] for c in clusters]  # mặc định giữ nguyên nếu không refine

    for i, cluster in enumerate(clusters):
        if i not in refine_set:
            continue
        rough_ts = cluster['ts']
        seek = max(0.0, rough_ts - window_sec)
        dur = window_sec * 2.0
        is_black = cluster.get('black', 0) >= 0.5

        # showinfo (in ra stderr ở loglevel info) cấp pts_time THẬT của từng khung →
        # xử lý đúng VFR và không giả định khung đều. Không dùng -copyts: pts_time
        # tính từ đầu cửa sổ seek (0-based), cộng vào seek ra hệ 0-based của pipeline.
        vf = f"scale={W}:{H},format=gray,showinfo"
        cmd = [
            ffmpeg_path, '-hide_banner', '-nostdin', '-loglevel', 'info',
            '-ss', f"{seek:.3f}", '-i', source_path, '-t', f"{dur:.3f}",
            '-vf', vf, '-f', 'rawvideo', '-pix_fmt', 'gray', 'pipe:1',
        ]
        try:
            proc = subprocess.run(cmd, stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=30)
            raw = proc.stdout
            info = proc.stderr.decode('utf-8', 'ignore')
        except Exception as e:
            print(f"refine-source seek loi @ {rough_ts:.2f}s: {e}", file=sys.stderr)
            continue

        n_frames = len(raw) // frame_bytes
        if n_frames < 2:
            continue
        arr = np.frombuffer(raw[:n_frames * frame_bytes], dtype=np.uint8).reshape(n_frames, H, W).astype(np.float32)

        # pts_time thật của từng khung, đúng thứ tự decode (khớp với thứ tự khung raw).
        pts_list = [float(x) for x in re.findall(r'pts_time:\s*([\d.]+)', info)]

        if is_black:
            # black onset: khung đầu tiên có độ sáng tụt dưới 30% mức nền của cửa sổ.
            bri = arr.mean(axis=(1, 2))
            thr = max(16.0, 0.3 * float(np.median(bri)))
            onset = None
            for fi in range(n_frames):
                if bri[fi] < thr:
                    onset = fi
                    break
            best_i = onset if onset is not None else int(np.argmin(bri))
        else:
            # hard cut: khung có MAD lớn nhất so với khung trước.
            mad = np.mean(np.abs(np.diff(arr, axis=0)), axis=(1, 2)) / 255.0  # (n-1,)
            best_i = int(np.argmax(mad)) + 1  # +1 vì diff lệch 1 khung

        # Ưu tiên pts_time thật; chỉ fallback idx/fps khi showinfo không trả pts.
        if best_i < len(pts_list):
            refined[i] = seek + pts_list[best_i]
        else:
            local_fps = n_frames / dur if dur > 0 else 30.0
            refined[i] = seek + best_i / local_fps

    _log_time("refine_pass2_source", t0)
    print(f"STATUS_LOG:Pass 2 (source): refine {len(refine_set)}/{len(clusters)} điểm ở fps gốc (pts thật).", flush=True)
    return refined


# ──────────────────────────────────────────────────────────
# Librosa MFCC — chunk-based để tránh ngốn RAM
# ──────────────────────────────────────────────────────────
def analyze_audio_spectral_chunked(audio_path, sr=16000, chunk_sec=120, delta_threshold=0.55):
    """Phân tích phổ âm thanh Librosa theo chunk để tránh load toàn bộ RAM.
    chunk_sec=120 → mỗi lần chỉ load 2 phút audio (≈ 30MB float32).
    """
    print("STATUS_LOG:Phổ âm thanh: Phân tích MFCC theo chunk...", flush=True)
    t0 = time.time()
    try:
        import librosa
        import numpy as np
        import soundfile as sf
    except Exception as e:
        print(f"librosa/soundfile loi: {e}", file=sys.stderr)
        return []

    try:
        total_frames_sf = sf.info(audio_path).frames
        sr_actual       = sf.info(audio_path).samplerate
    except Exception:
        total_frames_sf = None
        sr_actual = sr

    hop    = 8000
    changes = []
    offset  = 0.0

    try:
        with sf.SoundFile(audio_path) as f:
            sr_actual = f.samplerate
            chunk_samples = int(chunk_sec * sr_actual)
            frame_time = hop / sr

            while True:
                data = f.read(chunk_samples, dtype='float32', always_2d=False)
                if len(data) == 0:
                    break
                # Downsample nếu cần
                if sr_actual != sr:
                    import resampy
                    data = resampy.resample(data, sr_actual, sr)

                mfcc = librosa.feature.mfcc(y=data, sr=sr, n_mfcc=13, hop_length=hop)
                if mfcc.shape[1] < 3:
                    offset += chunk_sec
                    continue
                mfcc = (mfcc - mfcc.mean(axis=1, keepdims=True)) / (mfcc.std(axis=1, keepdims=True) + 1e-6)
                diffs = np.linalg.norm(np.diff(mfcc, axis=1), axis=0)
                if len(diffs) > 0:
                    diffs = diffs / (diffs.max() + 1e-6)
                    for i, d in enumerate(diffs):
                        if d > delta_threshold:
                            changes.append(offset + (i + 1) * frame_time)

                offset += len(data) / sr_actual
    except Exception as e:
        print(f"Loi chunk MFCC: {e}", file=sys.stderr)
        # Fallback: load toan bo (nhu cu)
        try:
            y, sr_lb = librosa.load(audio_path, sr=sr, mono=True)
            mfcc = librosa.feature.mfcc(y=y, sr=sr_lb, n_mfcc=13, hop_length=hop)
            if mfcc.shape[1] >= 3:
                mfcc = (mfcc - mfcc.mean(axis=1, keepdims=True)) / (mfcc.std(axis=1, keepdims=True) + 1e-6)
                diffs = np.linalg.norm(np.diff(mfcc, axis=1), axis=0)
                diffs = diffs / (diffs.max() + 1e-6)
                changes = [((i + 1) * hop / sr_lb) for i, d in enumerate(diffs) if d > delta_threshold]
        except Exception as e2:
            print(f"Loi fallback MFCC: {e2}", file=sys.stderr)

    _log_time("audio_spectral", t0)
    print(f"STATUS_LOG:Phổ âm thanh: {len(changes)} điểm.", flush=True)
    return changes


# ──────────────────────────────────────────────────────────
# Audio features — pure numpy/soundfile (KHÔNG librosa → freeze PyInstaller an toàn)
# Dùng chung cho audio-novelty (GĐ3 smart) và speech-continuity.
# ──────────────────────────────────────────────────────────
def extract_audio_features(audio_path, sr=16000, win=2048, hop=1024):
    """Đọc WAV mono theo chunk, trả về dict:
        {times, rms, centroid, rolloff, zcr}   (mỗi phần tử = 1 frame)
    hoặc None nếu không đọc được / không có audio.

    Chỉ dùng numpy + soundfile:
      - rms      : năng lượng khung (phát hiện im lặng / mức âm lượng)
      - centroid : trọng tâm phổ (âm sắc; đổi nhạc/giọng → dịch)
      - rolloff  : tần số chứa 85% năng lượng (độ "sáng" âm thanh)
      - zcr      : zero-crossing rate (phân biệt giọng nói / nhiễu / nhạc)
    """
    try:
        import numpy as np
        import soundfile as sf
    except Exception as e:
        print(f"soundfile/numpy loi (audio features): {e}", file=sys.stderr)
        return None

    if not audio_path or not os.path.exists(audio_path):
        return None

    t0 = time.time()
    win_fn = None
    freqs = None
    times, rms_l, cen_l, rol_l, zcr_l = [], [], [], [], []

    try:
        with sf.SoundFile(audio_path) as f:
            sr_actual = f.samplerate
            chunk_frames = 240000  # ~15s @16k, đọc theo lô để không ngốn RAM
            carry = np.zeros(0, dtype=np.float32)
            base_idx = 0  # số mẫu đã tiêu thụ (để tính thời gian tuyệt đối)

            while True:
                block = f.read(chunk_frames, dtype='float32', always_2d=False)
                if len(block) == 0:
                    break
                if block.ndim > 1:
                    block = block.mean(axis=1)
                buf = np.concatenate([carry, block])

                # Cửa sổ trượt trên buf
                n = len(buf)
                pos = 0
                if win_fn is None:
                    win_fn = np.hanning(win).astype(np.float32)
                    freqs = np.fft.rfftfreq(win, d=1.0 / sr_actual)
                while pos + win <= n:
                    seg = buf[pos:pos + win]
                    # RMS
                    rms = float(np.sqrt(np.mean(seg * seg)) + 1e-9)
                    # ZCR
                    zc = float(np.mean(np.abs(np.diff(np.sign(seg))) > 0))
                    # Spectrum
                    mag = np.abs(np.fft.rfft(seg * win_fn))
                    mag_sum = float(np.sum(mag)) + 1e-9
                    centroid = float(np.sum(freqs * mag) / mag_sum)
                    # Rolloff 85%
                    cumsum = np.cumsum(mag)
                    roll_idx = int(np.searchsorted(cumsum, 0.85 * cumsum[-1]))
                    rolloff = float(freqs[min(roll_idx, len(freqs) - 1)])

                    t_sample = base_idx + pos
                    times.append(t_sample / sr_actual)
                    rms_l.append(rms)
                    cen_l.append(centroid)
                    rol_l.append(rolloff)
                    zcr_l.append(zc)
                    pos += hop

                # Giữ lại phần dư (< win) để nối với chunk kế
                carry = buf[pos:]
                base_idx += pos
    except Exception as e:
        print(f"Loi extract_audio_features: {e}", file=sys.stderr)
        return None

    if not times:
        return None

    _log_time("audio_features", t0)
    return {
        'times':    np.asarray(times),
        'rms':      np.asarray(rms_l),
        'centroid': np.asarray(cen_l),
        'rolloff':  np.asarray(rol_l),
        'zcr':      np.asarray(zcr_l),
        'sr':       sr_actual if 'sr_actual' in dir() else sr,
    }


def _zscore(arr):
    import numpy as np
    a = np.asarray(arr, dtype=np.float64)
    mu = a.mean()
    sd = a.std() + 1e-9
    return (a - mu) / sd


def _pick_peaks(values, times, thr, min_gap_sec):
    """Chọn đỉnh cục bộ vượt ngưỡng thr, cách nhau tối thiểu min_gap_sec.
    Trả về list index. Thuần numpy — không cần scipy."""
    import numpy as np
    v = np.asarray(values)
    n = len(v)
    peaks = []
    last_t = -1e9
    for i in range(1, n - 1):
        if v[i] < thr:
            continue
        if v[i] >= v[i - 1] and v[i] >= v[i + 1]:
            if times[i] - last_t >= min_gap_sec:
                peaks.append(i)
                last_t = times[i]
            elif peaks and v[i] > v[peaks[-1]]:
                # gần đỉnh trước nhưng mạnh hơn → thay thế
                peaks[-1] = i
                last_t = times[i]
    return peaks


def analyze_audio_novelty_light(feats, k_scale=1.0, min_gap_sec=1.5):
    """Phát hiện điểm ĐỔI đặc trưng âm thanh (nhạc nền / môi trường / người nói)
    từ feats đã trích. Trả về list {ts, intensity} (intensity 0-100).

    novelty[i] = ||z(feat[i]) - z(feat[i-1])|| trên [rms, centroid, rolloff, zcr].
    Đây là tín hiệu KEY cho video nói liên tục (vlog/podcast) — nơi hình ảnh
    gần như không đổi nhưng âm thanh đổi rõ giữa các đoạn.

    k_scale: hệ số độ nhạy từ knob "Độ nhạy cắt" (scene_threshold/20). Cao → ngưỡng
      cao → ÍT điểm audio hơn. Nhờ vậy knob điều tiết được CẢ tín hiệu audio, không
      chỉ hình ảnh (trước đây audio dùng ngưỡng cứng nên knob không giảm được điểm).
    min_gap_sec: khoảng cách tối thiểu giữa hai đỉnh (nên = MinClipDuration) để không
      sinh điểm cắt dày hơn độ dài clip tối thiểu."""
    if not feats or len(feats['times']) < 4:
        return []
    import numpy as np

    z = np.vstack([
        _zscore(feats['rms']),
        _zscore(feats['centroid']),
        _zscore(feats['rolloff']),
        _zscore(feats['zcr']),
    ])  # shape (4, N)

    diff = np.linalg.norm(np.diff(z, axis=1), axis=0)  # (N-1,)
    # Làm trơn nhẹ để bớt nhiễu khung-đơn.
    if len(diff) >= 5:
        k = np.ones(5) / 5.0
        diff = np.convolve(diff, k, mode='same')

    times = feats['times'][1:]
    # Ngưỡng thích nghi CÓ điều tiết bởi knob: k tỉ lệ với k_scale, floor = p60
    # (cao hơn median cũ) để bỏ các dao động âm thanh yếu → bớt điểm rác.
    thr = robust_threshold(diff, k=3.0 * k_scale,
                           floor=float(_percentile(diff, 60.0)) + 1e-6)
    hi = _percentile(diff, 99.0)
    gap = max(1.0, min_gap_sec * k_scale)
    peak_idx = _pick_peaks(diff, times, thr, min_gap_sec=gap)

    out = []
    for i in peak_idx:
        out.append({'ts': float(times[i]),
                    'intensity': intensity_from(float(diff[i]), thr, hi)})
    print(f"STATUS_LOG:Audio novelty: {len(out)} điểm đổi âm thanh (k_scale={k_scale:.2f}).", flush=True)
    return out


def _speech_flags(feats):
    """VAD nhẹ: đánh dấu frame có giọng nói dựa RMS + ZCR (không cần thư viện).
    Giọng nói: năng lượng trên nền + ZCR trong dải hợp lý (không quá cao như nhiễu)."""
    import numpy as np
    rms = feats['rms']
    zcr = feats['zcr']
    noise_floor = _percentile(rms, 10.0)  # mức nền
    gate = max(noise_floor * 2.0, _percentile(rms, 40.0) * 0.5)
    # ZCR quá cao (>0.35) thường là nhiễu/xì; giọng nói thường 0.02–0.25
    speech = (rms > gate) & (zcr < 0.35)
    return speech


def speech_continuity_at(feats, candidate_ts_list, half_window=0.35):
    """Với mỗi candidate ts, đo mức NGẮT giọng nói trong [ts-hw, ts+hw].
    Trả về dict ts -> speech_break (0-100):
      100 = cửa sổ gần như không có giọng nói (khoảng lặng rõ → ranh giới hợp lệ)
        0 = giọng nói phủ kín (nói xuyên qua → nên phạt continuity).
    """
    if not feats or not candidate_ts_list or len(feats['times']) < 4:
        return {}
    import numpy as np
    times = feats['times']
    speech = _speech_flags(feats)

    out = {}
    for ts in candidate_ts_list:
        lo = ts - half_window
        hi = ts + half_window
        mask = (times >= lo) & (times <= hi)
        if not np.any(mask):
            out[ts] = 0
            continue
        frac_speech = float(np.mean(speech[mask]))
        out[ts] = int(round((1.0 - frac_speech) * 100))
    return out


# ──────────────────────────────────────────────────────────
# add_signal — merge window theo loại tín hiệu, ưu tiên timestamp tốt hơn
# ──────────────────────────────────────────────────────────
# Merge window theo loại tín hiệu
MERGE_WINDOWS = {
    'black':    0.60,  # Black frame có thể kéo dài 0.5s+
    'scene':    0.40,  # Hard cut tức thì
    'layout':   0.50,  # Layout change thường span ~0.5s
    'silence':  0.80,  # Khoảng lặng thường dài hơn
    'audio':    0.70,
}

# Độ ưu tiên timestamp — cao hơn = timestamp được giữ khi merge
SIGNAL_PRIORITY = {
    'black':    4,   # Cao nhất: black frame rõ ràng nhất
    'scene':    3,   # Hard cut
    'layout':   2,   # Layout
    'silence':  1,   # Silence
    'audio':    1,
}

# Confidence tối thiểu để được coi là candidate hợp lệ
MIN_CONFIDENCE = 35


def add_signal(candidates_map, timestamp, conf, signal_type, signal_val, reason_str):
    """Thêm tín hiệu vào candidates_map.
    - merge_window phụ thuộc signal_type
    - Khi merge: cập nhật timestamp nếu signal mới có độ ưu tiên cao hơn
    """
    window = MERGE_WINDOWS.get(signal_type, 0.50)
    new_priority = SIGNAL_PRIORITY.get(signal_type, 1)

    # Tìm candidate gần nhất trong window
    best_ts, best_dist = None, float('inf')
    for ts in candidates_map:
        dist = abs(ts - timestamp)
        if dist <= window and dist < best_dist:
            best_dist = dist
            best_ts = ts

    if best_ts is not None:
        cand = candidates_map[best_ts]
        old_priority = cand.get('_priority', 0)

        # Nếu signal mới có ưu tiên cao hơn → cập nhật timestamp
        if new_priority > old_priority:
            # Di chuyển entry sang timestamp mới
            cand['timestamp'] = timestamp
            cand['_priority'] = new_priority
            candidates_map[timestamp] = cand
            if best_ts != timestamp:
                del candidates_map[best_ts]
            best_ts = timestamp
            cand = candidates_map[best_ts]

        cand['signals'][signal_type] = max(cand['signals'].get(signal_type, 0), signal_val)
        if reason_str not in cand['reason']:
            cand['reason'] += f" + {reason_str}"
        cand['confidence'] = min(100, cand['confidence'] + conf)
        return

    # Candidate mới
    candidates_map[timestamp] = {
        'timestamp':  timestamp,
        'confidence': conf,
        '_priority':  new_priority,
        'signals':    {'scene': 0, 'black': 0, 'silence': 0, 'layout': 0, 'audio': 0, 'speech_break': 0},
        'reason':     reason_str,
    }
    candidates_map[timestamp]['signals'][signal_type] = signal_val


# ──────────────────────────────────────────────────────────
# Fast mode — heuristic nhẹ: silence + black + scene + audio (không dùng keyframe làm cut)
# ──────────────────────────────────────────────────────────
def _fast_extract_wav(source_path, ffmpeg_path, has_audio):
    """Trích WAV mono 8k tạm cho fast mode (nhẹ, chỉ để phân tích audio novelty).
    Trả về đường dẫn WAV tạm (caller tự xoá) hoặc "" nếu không có audio / lỗi."""
    if not has_audio:
        return ""
    import tempfile
    wav = os.path.join(tempfile.gettempdir(),
                       f"vs_fast_{abs(hash(source_path)) % (10**8)}.wav")
    cmd = [
        ffmpeg_path, '-y', '-hide_banner', '-nostdin', '-loglevel', 'error',
        '-i', source_path, '-vn', '-map', '0:a:0?',
        '-acodec', 'pcm_s16le', '-ar', '8000', '-ac', '1', wav,
    ]
    try:
        subprocess.run(cmd, stderr=subprocess.DEVNULL, timeout=600)
        return wav if os.path.exists(wav) else ""
    except Exception as e:
        print(f"fast wav extract loi: {e}", file=sys.stderr)
        return ""


def analyze_fast_mode(source_path, ffmpeg_path="ffmpeg", silence_db=-30,
                      silence_duration=0.5, has_audio=True):
    """Chế độ Nhanh: silence + black/scene bằng FFmpeg + audio-novelty nhẹ.

    Black và scene được phát hiện trong cùng một lượt decode 160x90 để vẫn nhanh.
    Scene giúp nhận ra video ghép clip có âm thanh liền mạch mà pipeline cũ bỏ sót.
    """
    print("STATUS_LOG:Chế độ Nhanh: silence + black/scene + audio-novelty...", flush=True)
    print("PROGRESS:5", flush=True)
    t0 = time.time()

    wav_path = ""
    # Chạy song song 3 nhánh. Black + scene dùng chung một lượt decode video nhẹ.
    with ThreadPoolExecutor(max_workers=3) as ex:
        f_sil = ex.submit(analyze_silence_regions, source_path, ffmpeg_path, silence_db, silence_duration)
        f_vis = ex.submit(_fast_visual_ffmpeg, source_path, ffmpeg_path)
        f_wav = ex.submit(_fast_extract_wav, source_path, ffmpeg_path, has_audio)
        silence_regions = f_sil.result()
        black_regions, scene_points = f_vis.result()
        wav_path = f_wav.result()

    print("PROGRESS:70", flush=True)
    candidates_map = {}

    # Silence: cường độ theo thời lượng khoảng lặng (khớp smart/precise).
    for reg in silence_regions:
        dur = reg.get('dur') or 0.0
        sil_i = intensity_from(dur, 0.4, 3.0)
        add_signal(candidates_map, reg['best_cut'], 30, 'silence', max(15, sil_i),
                   f"Silence {reg['start']:.2f}-{reg['end'] or '?':.2f}s")

    # Black: cường độ cố định cao (màn đen là tín hiệu ngắt rất rõ).
    for b_start in black_regions:
        add_signal(candidates_map, b_start, 50, 'black', 80, "Black frame")

    # Scene FFmpeg nhẹ: cường độ vừa-cao; Go vẫn chấm/phạt continuity để hạn chế cắt nhầm.
    for ts in scene_points:
        add_signal(candidates_map, ts, 40, 'scene', 75, "Fast scene change")

    # Audio novelty (nếu có audio) — điểm KEY cho video nói liên tục.
    audio_feats = None
    if wav_path:
        audio_feats = extract_audio_features(wav_path, sr=8000)
        if audio_feats:
            for pt in analyze_audio_novelty_light(audio_feats):
                add_signal(candidates_map, pt['ts'], 25, 'audio', pt['intensity'], "Audio novelty")

    print("PROGRESS:85", flush=True)

    # Speech continuity cho mọi candidate (dùng lại đặc trưng đã trích).
    if audio_feats:
        sb_map = speech_continuity_at(audio_feats, list(candidates_map.keys()))
        for ts, cand in candidates_map.items():
            cand['signals']['speech_break'] = sb_map.get(ts, 0)

    if wav_path:
        try:
            os.remove(wav_path)
        except OSError:
            pass

    _log_time("fast_mode_total", t0)
    print("PROGRESS:90", flush=True)
    return _finalize_candidates(candidates_map, min_confidence=30)


def _fast_visual_ffmpeg(source_path, ffmpeg_path):
    """Phát hiện black + hard scene trong một lượt decode nhỏ 160x90.

    Ngưỡng scene 0.35 cố ý bảo thủ cho chế độ Nhanh: bắt hard cut rõ ràng nhưng
    không cố bắt fade/chuyển động yếu như Tự động hoặc Kỹ.
    """
    print("STATUS_LOG:Nhanh: Phát hiện màn hình đen và chuyển cảnh...", flush=True)
    vf = r"scale=160:90,blackdetect=d=0.05:pic_th=0.98,select=gt(scene\,0.35),showinfo"
    cmd = [
        ffmpeg_path, '-hide_banner', '-nostdin', '-nostats',
        '-i', source_path, '-vf', vf, '-an', '-f', 'null', '-'
    ]
    try:
        r = subprocess.run(cmd, stderr=subprocess.PIPE, text=True, timeout=600)
        black = [float(m.group(1)) for m in re.finditer(r'black_start:([\d.]+)', r.stderr)]
        scenes = [float(m.group(1)) for m in re.finditer(r'pts_time:\s*([\d.]+)', r.stderr)]
        print(f"STATUS_LOG:Nhanh: {len(black)} vùng đen, {len(scenes)} chuyển cảnh rõ.", flush=True)
        return black, scenes
    except Exception as e:
        print(f"fast visual detect error: {e}", file=sys.stderr)
        return [], []


# ──────────────────────────────────────────────────────────
# Finalize — lọc theo confidence, loại bỏ duplicate
# ──────────────────────────────────────────────────────────
def _finalize_candidates(candidates_map, min_confidence=MIN_CONFIDENCE):
    """Lọc candidates: bỏ confidence thấp, loại bỏ metadata nội bộ, chuẩn hóa key tín hiệu khớp với Go struct."""
    result = []
    # Map Python internal keys to Go struct JSON keys
    key_mapping = {
        'scene': 'visual_change',
        'black': 'black_frame',
        'silence': 'silence',
        'layout': 'layout_change',
        'audio': 'audio_change',
        'speech_break': 'speech_break'
    }
    
    for cand in candidates_map.values():
        if cand['confidence'] < min_confidence:
            continue
        clean = {k: v for k, v in cand.items() if not k.startswith('_')}
        
        # Map signal keys to match Go's expectations
        if 'signals' in clean:
            mapped_signals = {}
            for pk, gk in key_mapping.items():
                mapped_signals[gk] = clean['signals'].get(pk, 0)
            clean['signals'] = mapped_signals
            
        result.append(clean)
    result.sort(key=lambda x: x['timestamp'])
    return result


# ──────────────────────────────────────────────────────────
# Main pipeline
# ──────────────────────────────────────────────────────────
def analyze_video(proxy_path, audio_path, source_path=None, ffmpeg_path="ffmpeg", mode="smart",
                  scene_threshold=20.0, silence_db=-30, silence_duration=0.5,
                  has_audio=True):
    """
    Pipeline theo mode:
    ┌──────────┬──────────────────────────────────────────────────────────────────┐
    │ fast     │ ffmpeg silence + black (heuristic, độ chính xác thấp hơn)        │
    │ smart    │ proxy 1 lần (scene+layout+black) + cluster + refine + silence     │
    │ precise  │ proxy 1 lần (fine threshold) + cluster + refine + Librosa MFCC  │
    └──────────┴──────────────────────────────────────────────────────────────────┘
    """
    proxy_path  = get_short_path(proxy_path)
    audio_path  = get_short_path(audio_path)
    source_path = get_short_path(source_path)

    t_total = time.time()

    # ══════ FAST: heuristic mode ══════
    if mode == "fast":
        cands = analyze_fast_mode(source_path, ffmpeg_path, silence_db,
                                  silence_duration, has_audio=has_audio)
        print("PROGRESS:95", flush=True)
        _log_time("TOTAL fast", t_total)
        return {"status": "success", "candidates": cands}

    # ══════ SMART / PRECISE: cần proxy ══════
    if not proxy_path or not os.path.exists(proxy_path):
        # Không âm thầm fallback — báo rõ ràng
        print("STATUS_LOG:[WARN] Proxy không tồn tại! Smart/Precise mode cần proxy. Dừng scan hình ảnh.", flush=True)
        # Vẫn chạy được silence detection
        proxy_path = None

    print("PROGRESS:5", flush=True)

    # ── Preset tham số phát hiện theo MODE ──
    # Ba tham số dưới đây tách biệt Smart (cân bằng) khỏi Precise (kỹ, bắt đủ điểm):
    #   k_detect      : hệ số ngưỡng thích nghi. Precise dùng thấp (2.0) → ngưỡng thấp
    #                   → bắt thêm chuyển cảnh vừa/yếu, KHÔNG bỏ sót ranh giới clip.
    #   cluster_window: cửa sổ gộp điểm gần nhau. Precise hẹp (0.30s) để KHÔNG gộp
    #                   nhầm hai clip thật nằm sát nhau thành một điểm.
    #   max_refine    : số cluster được tinh chỉnh về fps gốc. Precise dùng trần cao
    #                   nhưng hữu hạn để video rất dài không tạo hàng nghìn lần seek.
    if mode == "precise":
        k_detect, cluster_window, max_refine = 2.0, 0.30, 600
    else:  # smart (mặc định) — cân bằng tốc độ/độ chính xác
        k_detect, cluster_window, max_refine = 3.0, 0.60, 200

    # ── Bước 1: Chạy song song silence (I/O) + proxy scan (CPU) ──
    # Silence đọc audio stream → không tranh chấp với proxy scan (video stream)
    candidates_map = {}

    t1 = time.time()
    # Dùng ThreadPoolExecutor (không overhead spawn process) cho 2 task độc lập về I/O
    with ThreadPoolExecutor(max_workers=2) as ex:
        f_sil  = ex.submit(analyze_silence_regions, source_path, ffmpeg_path, silence_db, silence_duration)
        f_scan = ex.submit(scan_proxy_unified, proxy_path, scene_threshold, k_detect) if proxy_path else None

        silence_regions = f_sil.result()
        raw_cands       = f_scan.result() if f_scan else []

    _log_time("step1_parallel_scan", t1)
    print("PROGRESS:45", flush=True)

    # ── Bước 2: Cluster TRƯỚC Pass 2 ──
    t2 = time.time()
    clusters = cluster_raw_candidates(raw_cands, cluster_window=cluster_window)
    print(f"STATUS_LOG:Cluster: {len(raw_cands)} điểm thô → {len(clusters)} cluster.", flush=True)
    _log_time("step2_cluster", t2)
    print("PROGRESS:55", flush=True)

    # ── Bước 3: Pass 2 refine — tinh chỉnh điểm cắt ở fps GỐC (source), không proxy 4fps ──
    # Ưu tiên refine trên source (chính xác ~1/fps_gốc); nếu không có source thì
    # fallback refine trên proxy (cũ) để vẫn hoạt động.
    if clusters and source_path and os.path.exists(source_path):
        t3 = time.time()
        refined_ts = refine_clusters_on_source(source_path, clusters, ffmpeg_path, max_refine=max_refine)
        _log_time("step3_refine", t3)
        print(f"STATUS_LOG:Pass 2: {len(refined_ts)} điểm đã tinh chỉnh (source fps gốc).", flush=True)
    elif proxy_path and clusters:
        t3 = time.time()
        import cv2
        fps_proxy = cv2.VideoCapture(proxy_path).get(cv2.CAP_PROP_FPS) or 4.0
        refined_ts = refine_clusters(proxy_path, clusters, fps=fps_proxy)
        _log_time("step3_refine", t3)
        print(f"STATUS_LOG:Pass 2: {len(refined_ts)} điểm đã tinh chỉnh (proxy).", flush=True)
    else:
        refined_ts = [c['ts'] for c in clusters]

    print("PROGRESS:70", flush=True)

    # ── Bước 4: Phân tích âm thanh ──
    # SMART + PRECISE: trích đặc trưng nhẹ (pure numpy) DÙNG CHUNG cho audio-novelty
    #   và speech-continuity — đây là tín hiệu KEY cho video nói liên tục (vlog/podcast).
    # PRECISE: thêm Librosa MFCC (chi tiết hơn, chấp nhận chậm hơn).
    audio_feats = None
    novelty_pts = []
    spectral_ts = []
    have_audio = bool(audio_path and os.path.exists(audio_path))
    # Knob "Độ nhạy cắt" điều tiết CẢ audio (không chỉ hình ảnh): cao → ít điểm audio.
    k_scale = max(0.3, scene_threshold / 20.0)
    if have_audio:
        print("PROGRESS:72", flush=True)
        audio_feats = extract_audio_features(audio_path)
        if audio_feats:
            novelty_pts = analyze_audio_novelty_light(audio_feats, k_scale=k_scale)
        if mode == "precise":
            spectral_ts = analyze_audio_spectral_chunked(audio_path)
        print("PROGRESS:85", flush=True)

    print("PROGRESS:87", flush=True)

    # ── Bước 5: Gộp tất cả vào candidates_map ──
    # Dùng CƯỜNG ĐỘ đã chuẩn hoá 0-100 (scene_i/layout_i/hard_cut_i/black_i) từ
    # scan_proxy_unified thay vì cap cảm tính cũ. conf (đối số 3) chỉ dùng cho bộ
    # lọc MIN_CONFIDENCE nội bộ; signal_val (đối số 5) mới là cường độ Go dùng chấm điểm.
    for i, cluster in enumerate(clusters):
        ts = refined_ts[i] if i < len(refined_ts) else cluster['ts']
        c  = cluster
        if c.get('black_i', 0) > 0:
            add_signal(candidates_map, ts, 50, 'black', c['black_i'], "Black frame")
        if c.get('scene_i', 0) > 0:
            add_signal(candidates_map, ts, 40, 'scene', c['scene_i'], "Scene change")
        if c.get('layout_i', 0) > 0:
            add_signal(candidates_map, ts, 30, 'layout', c['layout_i'], "Layout change")
        if c.get('hard_cut_i', 0) > 0:
            add_signal(candidates_map, ts, 35, 'scene', c['hard_cut_i'], "Hard cut")

    # Silence: cường độ theo thời lượng khoảng lặng (dài hơn → ngắt mạch rõ hơn).
    # 0.5s→~30, 2s→~85, ≥3s→100. Thay hằng số 20 cũ.
    for reg in silence_regions:
        dur = reg.get('dur') or 0.0
        sil_i = intensity_from(dur, 0.4, 3.0)
        add_signal(candidates_map, reg['best_cut'], 30, 'silence', max(15, sil_i),
                   f"Silence {reg['start']:.2f}–{reg['end'] or '?':.2f}s")

    # Audio novelty (smart + precise): điểm đổi nhạc nền/môi trường/người nói với
    # cường độ thật đã chuẩn hoá 0-100. Bắt ranh giới ở video nói liên tục.
    for pt in novelty_pts:
        add_signal(candidates_map, pt['ts'], 25, 'audio', pt['intensity'], "Audio novelty")

    # Spectral MFCC (precise only) — cường độ cố định vừa phải, bổ trợ novelty.
    for ts in spectral_ts:
        add_signal(candidates_map, ts, 25, 'audio', 45, "Audio spectral change")

    print("PROGRESS:92", flush=True)

    # ── Bước 5.5: Speech continuity — đo giọng nói có xuyên qua điểm cắt không.
    # Gán speech_break (0-100) cho MỌI candidate để Go điều tiết continuity penalty:
    #   cao = có khoảng lặng giọng nói tại điểm cắt (ranh giới hợp lệ)
    #   thấp = nói liên tục xuyên qua (nhiều khả năng chỉ đổi góc quay → phạt).
    if audio_feats:
        cand_ts = list(candidates_map.keys())
        sb_map = speech_continuity_at(audio_feats, cand_ts)
        for ts, cand in candidates_map.items():
            cand['signals']['speech_break'] = sb_map.get(ts, 0)

    print("PROGRESS:93", flush=True)

    # Kỹ giữ cả tín hiệu layout/audio đơn lẻ để Go chấm điểm tiếp, tránh Python
    # loại quá sớm các ranh giới mà nhiều detector yếu cùng có thể bổ trợ nhau.
    # Tự động giữ audio/layout từ mức 25 để đúng nghĩa lai hình ảnh + âm thanh.
    min_confidence = 20 if mode == "precise" else 25
    result = _finalize_candidates(candidates_map, min_confidence=min_confidence)

    # Detector done summary
    print(f"DETECTOR_DONE:Chuyển cảnh|{len([c for c in result if c['signals'].get('visual_change',0)>0])}", flush=True)
    print(f"DETECTOR_DONE:Khoảng lặng|{len(silence_regions)}", flush=True)
    print(f"DETECTOR_DONE:Màn hình đen|{len([c for c in result if c['signals'].get('black_frame',0)>0])}", flush=True)

    print("PROGRESS:95", flush=True)
    _log_time("TOTAL smart/precise", t_total)
    return {"status": "success", "candidates": result}


# ──────────────────────────────────────────────────────────
# Transcribe (Whisper offline) — nghe tiếng ra phụ đề có timestamp
# ──────────────────────────────────────────────────────────
def transcribe_audio(media_path, language="auto", model_name="small",
                     model_dir=None, clip_start=None, clip_end=None,
                     ffmpeg_path="ffmpeg"):
    """Nghe tiếng trong 1 file (audio hoặc video) bằng faster-whisper.

    Trả về dict {status, language, segments:[{start,end,text}]}.
    - language="auto" → tự nhận diện; hoặc mã ISO ("vi","en","ja"...).
    - model_name: base/small/medium/large-v3 (mọi model nghe 99 ngôn ngữ,
      size chỉ đổi độ chính xác).
    - clip_start/clip_end: nếu có, chỉ nghe đoạn [start,end] của media (giây).
      Ta trích đoạn ra WAV tạm bằng ffmpeg rồi nghe → timestamp segment sẽ
      0-based theo đoạn (khớp clip khi burn).
    - model_dir: nơi cache model (tải tự động lần đầu).
    """
    try:
        from faster_whisper import WhisperModel
    except Exception as e:
        return {"status": "error",
                "message": f"Chưa cài faster-whisper trong worker: {e}"}

    src = get_short_path(media_path)
    if not src or not os.path.exists(src):
        return {"status": "error", "message": f"Không tìm thấy file: {media_path}"}

    # Nếu chỉ nghe 1 đoạn → trích WAV tạm 16k mono cho đoạn đó.
    tmp_wav = ""
    listen_path = src
    if clip_start is not None and clip_end is not None and clip_end > clip_start:
        import tempfile
        tmp_wav = os.path.join(
            tempfile.gettempdir(),
            f"vs_trans_{abs(hash(media_path)) % (10**8)}_{clip_start:.0f}.wav")
        dur = clip_end - clip_start
        cmd = [
            ffmpeg_path, '-y', '-hide_banner', '-nostdin', '-loglevel', 'error',
            '-ss', f"{clip_start:.3f}", '-i', src, '-t', f"{dur:.3f}",
            '-vn', '-map', '0:a:0?', '-acodec', 'pcm_s16le',
            '-ar', '16000', '-ac', '1', tmp_wav,
        ]
        try:
            subprocess.run(cmd, stderr=subprocess.DEVNULL, timeout=600)
            if os.path.exists(tmp_wav):
                listen_path = tmp_wav
        except Exception as e:
            print(f"trích đoạn audio lỗi: {e}", file=sys.stderr)

    print("STATUS_LOG:Đang tải mô hình Whisper (lần đầu có thể mất vài phút)...", flush=True)
    print("PROGRESS:5", flush=True)
    t0 = time.time()
    try:
        model = WhisperModel(model_name, device="cpu", compute_type="int8",
                             download_root=model_dir)
    except Exception as e:
        if tmp_wav and os.path.exists(tmp_wav):
            try: os.remove(tmp_wav)
            except OSError: pass
        return {"status": "error",
                "message": f"Không tải được mô hình Whisper '{model_name}': {e}"}

    _log_time("whisper_load", t0)
    print("STATUS_LOG:Đang nghe tiếng và tạo phụ đề...", flush=True)
    print("PROGRESS:15", flush=True)

    lang_arg = None if (not language or language == "auto") else language
    t1 = time.time()
    segments_out = []
    detected_lang = language
    try:
        seg_iter, info = model.transcribe(listen_path, language=lang_arg,
                                          vad_filter=True)
        detected_lang = getattr(info, "language", None) or language
        total_dur = float(getattr(info, "duration", 0.0)) or 0.0
        for seg in seg_iter:
            text = (seg.text or "").strip()
            if text:
                segments_out.append({
                    "start": float(seg.start),
                    "end": float(seg.end),
                    "text": text,
                })
            if total_dur > 0:
                pct = 15 + int(min(1.0, seg.end / total_dur) * 80)
                print(f"PROGRESS:{pct}", flush=True)
    except Exception as e:
        if tmp_wav and os.path.exists(tmp_wav):
            try: os.remove(tmp_wav)
            except OSError: pass
        return {"status": "error", "message": f"Lỗi khi nghe tiếng: {e}"}

    if tmp_wav and os.path.exists(tmp_wav):
        try: os.remove(tmp_wav)
        except OSError: pass

    _log_time("whisper_transcribe", t1)
    print(f"STATUS_LOG:Nghe xong: {len(segments_out)} câu phụ đề ({detected_lang}).", flush=True)
    print("PROGRESS:98", flush=True)
    return {"status": "success", "language": detected_lang, "segments": segments_out}


# ──────────────────────────────────────────────────────────
# Entry point
# ──────────────────────────────────────────────────────────
if __name__ == "__main__":
    multiprocessing.freeze_support()
    parser = argparse.ArgumentParser(description="Media Analyzer Worker")
    parser.add_argument("--proxy",            default="",      help="Path to proxy video (smart/precise only)")
    parser.add_argument("--audio",            default="",      help="Path to audio WAV (precise only)")
    parser.add_argument("--source",           default="",      help="Path to original source video")
    parser.add_argument("--ffmpeg",           default="ffmpeg")
    parser.add_argument("--mode",             default="smart", choices=["fast", "smart", "precise"])
    parser.add_argument("--scene-threshold",  default=20.0,    type=float)
    parser.add_argument("--silence-db",       default=-30,     type=float)
    parser.add_argument("--silence-duration", default=0.5,     type=float)
    # no-audio: bỏ nhánh phân tích âm thanh (video câm). VFR & start_time KHÔNG cần
    # tham số nữa — refine-source đọc pts_time thật qua showinfo nên tự đúng cho cả
    # VFR, và timestamp giữ hệ 0-based nhất quán với lúc cắt (không cộng start_time).
    parser.add_argument("--no-audio",   action="store_true", help="Video KHÔNG có audio stream")
    # === Transcribe (Whisper) — chế độ riêng: nếu có --transcribe thì bỏ qua analyze ===
    parser.add_argument("--transcribe",     default="",       help="Path media để nghe tiếng ra phụ đề (bật chế độ transcribe)")
    parser.add_argument("--language",       default="auto",   help="Ngôn ngữ nguồn: auto / vi / en / ...")
    parser.add_argument("--whisper-model",  default="small",  help="Model Whisper: base/small/medium/large-v3")
    parser.add_argument("--model-dir",      default="",       help="Thư mục cache model Whisper")
    parser.add_argument("--clip-start",     default=None,     type=float, help="Chỉ nghe từ giây này (tùy chọn)")
    parser.add_argument("--clip-end",       default=None,     type=float, help="Chỉ nghe tới giây này (tùy chọn)")
    args = parser.parse_args()

    # ══════ Chế độ TRANSCRIBE (Whisper) ══════
    if args.transcribe:
        try:
            data = transcribe_audio(
                args.transcribe,
                language=args.language,
                model_name=args.whisper_model,
                model_dir=(args.model_dir or None),
                clip_start=args.clip_start,
                clip_end=args.clip_end,
                ffmpeg_path=args.ffmpeg,
            )
            print(json.dumps(data))
        except Exception as e:
            print(json.dumps({"status": "error", "message": str(e)}))
            sys.exit(1)
        sys.exit(0)

    try:
        data = analyze_video(
            args.proxy, args.audio,
            source_path=args.source,
            ffmpeg_path=args.ffmpeg,
            mode=args.mode,
            scene_threshold=args.scene_threshold,
            silence_db=args.silence_db,
            silence_duration=args.silence_duration,
            has_audio=not args.no_audio,
        )
        print(json.dumps(data))
    except Exception as e:
        print(json.dumps({"status": "error", "message": str(e)}))
        sys.exit(1)
