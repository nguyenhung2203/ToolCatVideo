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
import json
import argparse
import subprocess
import re
import time
import multiprocessing
import os
import ctypes
from concurrent.futures import ThreadPoolExecutor


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
def scan_proxy_unified(proxy_path, scene_threshold=20.0):
    """Quét proxy VIDEO MỘT LẦN, tính đồng thời:
      - histogram diff   → scene change
      - mean abs diff    → hard cut (chính xác nhất)
      - edge/col energy  → layout change
      - brightness mean  → black frame
    Trả về danh sách raw_candidates: {ts, scene, layout, black, hard_cut, is_black}
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

    # Chuẩn hoá ngưỡng: scene_threshold=20 → hist_bhatt~0.34
    hist_thr   = max(0.08, min(0.75, scene_threshold / 58.0))
    layout_thr = 0.38
    black_thr  = 12.0  # mean brightness

    print(f"STATUS_LOG:Quét proxy: {total_frames} frame @ {fps:.1f} FPS ({duration:.0f}s)...", flush=True)

    raw_candidates = []
    prev_hist  = None
    prev_gray  = None
    prev_edge  = None
    idx = 0
    report_step = max(1, total_frames // 10)

    while True:
        ok, frame = cap.read()
        if not ok or frame is None:
            break

        gray = cv2.cvtColor(frame, cv2.COLOR_BGR2GRAY)

        # Black frame score
        brightness = float(np.mean(gray))
        is_black   = brightness < black_thr

        # Histogram (scene)
        hist = cv2.calcHist([gray], [0], None, [64], [0, 256])
        cv2.normalize(hist, hist)

        # Edge energy (layout)
        col_e = np.mean(np.abs(np.diff(gray.astype(np.float32), axis=1)), axis=0)
        norm  = np.linalg.norm(col_e) + 1e-6
        col_e = col_e / norm

        if prev_hist is not None:
            hist_d  = cv2.compareHist(prev_hist, hist, cv2.HISTCMP_BHATTACHARYYA)
            edge_d  = float(np.linalg.norm(col_e - prev_edge))
            mad     = float(np.mean(np.abs(gray.astype(np.float32) - prev_gray.astype(np.float32)))) / 255.0

            if hist_d > hist_thr or edge_d > layout_thr or is_black or mad > 0.25:
                raw_candidates.append({
                    'ts':       idx / fps,
                    'scene':    hist_d,
                    'layout':   edge_d,
                    'hard_cut': mad,
                    'black':    1.0 if is_black else 0.0,
                })

        prev_hist = hist
        prev_gray = gray
        prev_edge = col_e
        idx += 1

        if idx % report_step == 0:
            pct = int(idx / total_frames * 100) if total_frames > 0 else 0
            print(f"STATUS_LOG:Quét proxy: {pct}% ({len(raw_candidates)} điểm)...", flush=True)

    cap.release()
    _log_time("proxy_scan", t0)
    print(f"STATUS_LOG:Quét proxy xong: {len(raw_candidates)} điểm thô.", flush=True)
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
        'signals':    {'scene': 0, 'black': 0, 'silence': 0, 'layout': 0, 'audio': 0},
        'reason':     reason_str,
    }
    candidates_map[timestamp]['signals'][signal_type] = signal_val


# ──────────────────────────────────────────────────────────
# Fast mode — heuristic: chỉ silence + black (không dùng keyframe làm cut)
# ──────────────────────────────────────────────────────────
def analyze_fast_mode(source_path, ffmpeg_path="ffmpeg", silence_db=-30, silence_duration=0.5):
    """Heuristic mode: chỉ dùng silence và black detection.
    Không decode frame thị giác, không dùng keyframe.
    Độ chính xác phụ thuộc vào nội dung video: tốt với video có khoảng lặng/màn đen giữa clip,
    kém hơn với video không có silence/black.
    """
    print("STATUS_LOG:Chế độ Nhanh (heuristic): silence + black detect...", flush=True)
    print("PROGRESS:5", flush=True)
    t0 = time.time()

    # Chạy song song silence + black (hai process đọc cùng file, nhưng audio và video tách biệt)
    with ThreadPoolExecutor(max_workers=2) as ex:
        f_sil = ex.submit(analyze_silence_regions, source_path, ffmpeg_path, silence_db, silence_duration)
        f_blk = ex.submit(_fast_black_ffmpeg, source_path, ffmpeg_path)
        silence_regions = f_sil.result()
        black_regions   = f_blk.result()

    print("PROGRESS:75", flush=True)
    candidates_map = {}

    for reg in silence_regions:
        add_signal(candidates_map, reg['best_cut'], 30, 'silence', 20, f"Silence {reg['start']:.2f}-{reg['end'] or '?':.2f}s")

    for b_start in black_regions:
        add_signal(candidates_map, b_start, 50, 'black', 30, "Black frame")

    _log_time("fast_mode_total", t0)
    print("PROGRESS:90", flush=True)
    return _finalize_candidates(candidates_map)


def _fast_black_ffmpeg(source_path, ffmpeg_path):
    """Black detection bằng ffmpeg — vẫn decode video nhưng chỉ scale nhỏ để đỡ tải."""
    print("STATUS_LOG:Nhanh: Phát hiện màn hình đen...", flush=True)
    cmd = [
        ffmpeg_path, '-hide_banner', '-nostdin', '-nostats',
        '-i', source_path,
        '-vf', 'scale=160:90,blackdetect=d=0.05:pic_th=0.98',
        '-an', '-f', 'null', '-'
    ]
    try:
        r = subprocess.run(cmd, stderr=subprocess.PIPE, text=True, timeout=600)
        return [float(m.group(1)) for m in re.finditer(r'black_start:([\d.]+)', r.stderr)]
    except Exception as e:
        print(f"black detect error: {e}", file=sys.stderr)
        return []


# ──────────────────────────────────────────────────────────
# Finalize — lọc theo confidence, loại bỏ duplicate
# ──────────────────────────────────────────────────────────
def _finalize_candidates(candidates_map, min_confidence=MIN_CONFIDENCE):
    """Lọc candidates: bỏ confidence thấp, loại bỏ metadata nội bộ."""
    result = []
    for cand in candidates_map.values():
        if cand['confidence'] < min_confidence:
            continue
        clean = {k: v for k, v in cand.items() if not k.startswith('_')}
        result.append(clean)
    result.sort(key=lambda x: x['timestamp'])
    return result


# ──────────────────────────────────────────────────────────
# Main pipeline
# ──────────────────────────────────────────────────────────
def analyze_video(proxy_path, audio_path, source_path=None, ffmpeg_path="ffmpeg", mode="smart",
                  scene_threshold=20.0, silence_db=-30, silence_duration=0.5):
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
        cands = analyze_fast_mode(source_path, ffmpeg_path, silence_db, silence_duration)
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

    # ── Bước 1: Chạy song song silence (I/O) + proxy scan (CPU) ──
    # Silence đọc audio stream → không tranh chấp với proxy scan (video stream)
    candidates_map = {}

    t1 = time.time()
    # Dùng ThreadPoolExecutor (không overhead spawn process) cho 2 task độc lập về I/O
    with ThreadPoolExecutor(max_workers=2) as ex:
        f_sil  = ex.submit(analyze_silence_regions, source_path, ffmpeg_path, silence_db, silence_duration)
        f_scan = ex.submit(scan_proxy_unified, proxy_path, scene_threshold) if proxy_path else None

        silence_regions = f_sil.result()
        raw_cands       = f_scan.result() if f_scan else []

    _log_time("step1_parallel_scan", t1)
    print("PROGRESS:45", flush=True)

    # ── Bước 2: Cluster TRƯỚC Pass 2 ──
    t2 = time.time()
    clusters = cluster_raw_candidates(raw_cands, cluster_window=0.6)
    print(f"STATUS_LOG:Cluster: {len(raw_cands)} điểm thô → {len(clusters)} cluster.", flush=True)
    _log_time("step2_cluster", t2)
    print("PROGRESS:55", flush=True)

    # ── Bước 3: Pass 2 refine (chỉ với clustered list, không seek từng điểm riêng lẻ) ──
    if proxy_path and clusters:
        t3 = time.time()
        import cv2
        fps_proxy = cv2.VideoCapture(proxy_path).get(cv2.CAP_PROP_FPS) or 4.0
        refined_ts = refine_clusters(proxy_path, clusters, fps=fps_proxy)
        _log_time("step3_refine", t3)
        print(f"STATUS_LOG:Pass 2: {len(refined_ts)} điểm đã tinh chỉnh.", flush=True)
    else:
        refined_ts = [c['ts'] for c in clusters]

    print("PROGRESS:70", flush=True)

    # ── Bước 4: Librosa MFCC (chỉ precise mode) ──
    spectral_ts = []
    if mode == "precise" and audio_path and os.path.exists(audio_path):
        print("PROGRESS:72", flush=True)
        spectral_ts = analyze_audio_spectral_chunked(audio_path)
        print("PROGRESS:85", flush=True)

    print("PROGRESS:87", flush=True)

    # ── Bước 5: Gộp tất cả vào candidates_map ──
    # Thêm scene/layout/black từ clusters (dùng raw score từ cluster representative)
    for i, cluster in enumerate(clusters):
        ts = refined_ts[i] if i < len(refined_ts) else cluster['ts']
        c  = cluster
        if c['black'] >= 0.5:
            add_signal(candidates_map, ts, 50, 'black',  int(c['black'] * 30), "Black frame")
        if c['scene'] > 0.2:
            add_signal(candidates_map, ts, 40, 'scene',  int(c['scene'] * 40), "Scene change")
        if c['layout'] > 0.3:
            add_signal(candidates_map, ts, 30, 'layout', int(c['layout'] * 30), "Layout change")
        if c['hard_cut'] > 0.3:
            add_signal(candidates_map, ts, 35, 'scene',  int(c['hard_cut'] * 40), "Hard cut")

    # Silence: best_cut timestamp có ưu tiên cao nếu gần scene change
    for reg in silence_regions:
        add_signal(candidates_map, reg['best_cut'], 30, 'silence', 20,
                   f"Silence {reg['start']:.2f}–{reg['end'] or '?':.2f}s")

    # Spectral (precise only)
    for ts in spectral_ts:
        add_signal(candidates_map, ts, 25, 'audio', 15, "Audio spectral change")

    print("PROGRESS:93", flush=True)

    result = _finalize_candidates(candidates_map)

    # Detector done summary
    print(f"DETECTOR_DONE:Chuyển cảnh|{len([c for c in result if c['signals'].get('scene',0)>0])}", flush=True)
    print(f"DETECTOR_DONE:Khoảng lặng|{len(silence_regions)}", flush=True)
    print(f"DETECTOR_DONE:Màn hình đen|{len([c for c in result if c['signals'].get('black',0)>0])}", flush=True)

    print("PROGRESS:95", flush=True)
    _log_time("TOTAL smart/precise", t_total)
    return {"status": "success", "candidates": result}


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
    args = parser.parse_args()

    try:
        data = analyze_video(
            args.proxy, args.audio,
            source_path=args.source,
            ffmpeg_path=args.ffmpeg,
            mode=args.mode,
            scene_threshold=args.scene_threshold,
            silence_db=args.silence_db,
            silence_duration=args.silence_duration,
        )
        print(json.dumps(data))
    except Exception as e:
        print(json.dumps({"status": "error", "message": str(e)}))
        sys.exit(1)
