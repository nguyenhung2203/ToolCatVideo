import sys
import json
import argparse
import subprocess
import re
from concurrent.futures import ThreadPoolExecutor, as_completed


def analyze_silence(source_path, ffmpeg_path="ffmpeg", silence_db=-30, silence_duration=0.5):
    """Tìm điểm im lặng — chạy TRỰC TIẾP trên video gốc (chỉ decode audio, rất nhanh)."""
    cmd = [
        ffmpeg_path, '-i', source_path,
        '-vn',  # bỏ video → chỉ decode audio
        '-af', f'silencedetect=noise={silence_db}dB:d={silence_duration}',
        '-f', 'null', '-'
    ]
    silence_starts = []
    try:
        result = subprocess.run(cmd, stderr=subprocess.PIPE, text=True, timeout=300)
        matches = re.finditer(r'silence_start:\s+([\d\.]+)', result.stderr)
        for match in matches:
            silence_starts.append(float(match.group(1)))
    except Exception as e:
        print(f"Loi khi chay ffmpeg silencedetect: {e}", file=sys.stderr)
    return silence_starts


def analyze_black(source_path, ffmpeg_path="ffmpeg", black_duration=0.05):
    """Phát hiện màn hình đen — chạy trên video gốc với inline scale 240p (nhanh hơn proxy riêng).
    Giảm black_duration 0.1→0.05 để bắt fade ngắn."""
    cmd = [
        ffmpeg_path, '-i', source_path,
        '-vf', f'scale=-2:240,blackdetect=d={black_duration}:pic_th=0.98',
        '-an', '-f', 'null', '-'
    ]
    black_starts = []
    try:
        result = subprocess.run(cmd, stderr=subprocess.PIPE, text=True, timeout=300)
        matches = re.finditer(r'black_start:([\d\.]+)', result.stderr)
        for match in matches:
            black_starts.append(float(match.group(1)))
    except Exception as e:
        print(f"Loi khi chay ffmpeg blackdetect: {e}", file=sys.stderr)
    return black_starts


def analyze_scenes(source_path, scene_threshold=20.0):
    """Chuyển cảnh nội dung — PySceneDetect chạy TRỰC TIẾP trên video gốc.
    PySceneDetect tự decode hiệu quả, KHÔNG cần proxy.
    Giảm threshold 27→20 để nhạy hơn ~35%."""
    from scenedetect import detect, ContentDetector
    scene_list = detect(source_path, ContentDetector(threshold=scene_threshold))
    return [s[0].get_seconds() for s in scene_list]


def analyze_layout(source_path, sample_fps=1.0):
    """Phát hiện thay đổi bố cục — chạy trên video gốc, sample 1fps (đủ cho layout).
    Giảm sample_fps 2→1 để nhanh hơn 2x."""
    try:
        import cv2
        import numpy as np
    except Exception as e:
        print(f"OpenCV/numpy khong san sang, bo qua layout: {e}", file=sys.stderr)
        return []

    cap = cv2.VideoCapture(source_path)
    if not cap.isOpened():
        return []

    fps = cap.get(cv2.CAP_PROP_FPS) or 30.0
    step = max(1, int(round(fps / sample_fps)))

    changes = []
    prev_hist = None
    prev_edge = None
    idx = 0
    while True:
        ret = cap.grab()
        if not ret:
            break
        if idx % step == 0:
            ok, frame = cap.retrieve()
            if ok and frame is not None:
                # Scale xuống nhỏ để xử lý nhanh (chỉ cần histogram + edge)
                small = cv2.resize(frame, (320, 180))
                gray = cv2.cvtColor(small, cv2.COLOR_BGR2GRAY)
                hist = cv2.calcHist([gray], [0], None, [32], [0, 256])
                cv2.normalize(hist, hist)
                col_energy = np.mean(np.abs(np.diff(gray.astype(np.float32), axis=1)), axis=0)
                col_energy = col_energy / (np.linalg.norm(col_energy) + 1e-6)

                if prev_hist is not None:
                    hist_diff = cv2.compareHist(prev_hist, hist, cv2.HISTCMP_BHATTACHARYYA)
                    edge_diff = float(np.linalg.norm(col_energy - prev_edge))
                    if hist_diff > 0.30 and edge_diff > 0.40:
                        changes.append(idx / fps)

                prev_hist = hist
                prev_edge = col_energy
        idx += 1

    cap.release()
    return changes


def analyze_audio_spectral(source_path, sr=16000, hop=8000, delta_threshold=0.55):
    """Phát hiện thay đổi phổ âm thanh — load audio trực tiếp từ video (librosa hỗ trợ)."""
    try:
        import librosa
        import numpy as np
    except Exception as e:
        print(f"librosa khong san sang, bo qua audio spectral: {e}", file=sys.stderr)
        return []

    try:
        y, sr = librosa.load(source_path, sr=sr, mono=True)
    except Exception as e:
        print(f"Loi doc audio: {e}", file=sys.stderr)
        return []

    if y is None or len(y) == 0:
        return []

    mfcc = librosa.feature.mfcc(y=y, sr=sr, n_mfcc=13, hop_length=hop)
    if mfcc.shape[1] < 3:
        return []

    mfcc = (mfcc - mfcc.mean(axis=1, keepdims=True)) / (mfcc.std(axis=1, keepdims=True) + 1e-6)
    diffs = np.linalg.norm(np.diff(mfcc, axis=1), axis=0)
    if len(diffs) == 0:
        return []
    diffs = diffs / (diffs.max() + 1e-6)

    frame_time = hop / sr
    changes = []
    for i, d in enumerate(diffs):
        if d > delta_threshold:
            changes.append((i + 1) * frame_time)
    return changes


# Giảm cửa sổ gom từ 0.6→0.4s — gom chính xác hơn, ít gom nhầm sự kiện khác.
MERGE_WINDOW = 0.4


def add_signal(candidates_map, timestamp, conf, signal_key, signal_val, reason_str):
    """Gộp tín hiệu vào candidate GẦN NHẤT (khoảng cách nhỏ nhất) trong cửa sổ."""
    best_ts = None
    best_dist = float('inf')

    for ts in candidates_map:
        dist = abs(ts - timestamp)
        if dist < MERGE_WINDOW and dist < best_dist:
            best_dist = dist
            best_ts = ts

    if best_ts is not None:
        cand = candidates_map[best_ts]
        cand['signals'][signal_key] = max(cand['signals'].get(signal_key, 0), signal_val)
        if reason_str not in cand['reason']:
            cand['reason'] += f" + {reason_str}"
        cand['confidence'] = min(100, cand['confidence'] + conf)
        return

    candidates_map[timestamp] = {
        "timestamp": timestamp,
        "confidence": conf,
        "signals": {
            "visual_change": 0,
            "black_frame": 0,
            "silence": 0,
            "layout_change": 0,
            "audio_change": 0,
        },
        "reason": reason_str,
    }
    candidates_map[timestamp]['signals'][signal_key] = signal_val


def analyze_video(source_path, ffmpeg_path="ffmpeg", mode="smart",
                  scene_threshold=20.0, silence_db=-30, silence_duration=0.5):
    """Chạy các detector TRỰC TIẾP trên video gốc — không cần proxy.

    Pipeline mới:
    - Bỏ bước tạo proxy (tiết kiệm 60-90s cho video 10 phút)
    - Bỏ bước extract audio riêng (FFmpeg silencedetect đọc audio trực tiếp)
    - Tất cả detector chạy song song trên video gốc
    - SceneDetect threshold giảm 27→20 để nhạy hơn
    """
    candidates_map = {}
    results = {}

    def run_black():
        return analyze_black(source_path, ffmpeg_path=ffmpeg_path)

    def run_silence():
        return analyze_silence(source_path, ffmpeg_path=ffmpeg_path,
                               silence_db=silence_db, silence_duration=silence_duration)

    def run_scenes():
        return analyze_scenes(source_path, scene_threshold=scene_threshold)

    def run_layout():
        return analyze_layout(source_path)

    def run_audio_spectral():
        return analyze_audio_spectral(source_path)

    # === Xác định detector cần chạy theo mode ===
    tasks = {
        'black': run_black,
        'silence': run_silence,
    }
    if mode in ("smart", "precise"):
        tasks['scenes'] = run_scenes
        tasks['layout'] = run_layout
    if mode == "precise":
        tasks['audio_spectral'] = run_audio_spectral

    # === Chạy song song tất cả detector ===
    print("PROGRESS:10", flush=True)

    with ThreadPoolExecutor(max_workers=len(tasks)) as executor:
        futures = {executor.submit(fn): name for name, fn in tasks.items()}
        completed = 0
        for future in as_completed(futures):
            name = futures[future]
            completed += 1
            try:
                results[name] = future.result()
            except Exception as e:
                print(f"Detector '{name}' loi: {e}", file=sys.stderr)
                results[name] = []

            prog = 10 + int(completed / len(tasks) * 80)
            print(f"PROGRESS:{prog}", flush=True)

    # === Gộp kết quả vào candidates_map ===
    for start_time in results.get('black', []):
        add_signal(candidates_map, start_time, 50, "black_frame", 30, "Black frame")

    for s_time in results.get('silence', []):
        add_signal(candidates_map, s_time, 30, "silence", 20, "Silence start")

    for t in results.get('scenes', []):
        add_signal(candidates_map, t, 40, "visual_change", 30, "Scene change")

    for t in results.get('layout', []):
        add_signal(candidates_map, t, 35, "layout_change", 25, "Layout change")

    for t in results.get('audio_spectral', []):
        add_signal(candidates_map, t, 30, "audio_change", 20, "Audio change")

    print("PROGRESS:95", flush=True)

    candidates = list(candidates_map.values())
    candidates.sort(key=lambda x: x['timestamp'])

    return {
        "status": "success",
        "candidates": candidates,
    }


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description="Media Analyzer Worker")
    parser.add_argument("--source", type=str, required=True, help="Path to source video (used directly)")
    # Legacy args — kept for backward compat but ignored
    parser.add_argument("--proxy", type=str, default="", help="(deprecated, ignored)")
    parser.add_argument("--audio", type=str, default="", help="(deprecated, ignored)")
    parser.add_argument("--ffmpeg", type=str, default="ffmpeg", help="Path to ffmpeg executable")
    parser.add_argument("--mode", type=str, default="smart", choices=["fast", "smart", "precise"], help="Analysis mode")
    parser.add_argument("--scene-threshold", type=float, default=20.0, help="Scene detection threshold (default: 20.0)")
    parser.add_argument("--silence-db", type=float, default=-30, help="Silence noise threshold in dB (default: -30)")
    parser.add_argument("--silence-duration", type=float, default=0.5, help="Min silence duration in seconds (default: 0.5)")

    args = parser.parse_args()

    try:
        data = analyze_video(
            args.source,
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
