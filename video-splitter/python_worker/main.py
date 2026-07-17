import sys
import json
import argparse
import subprocess
import re


def analyze_silence(audio_path, ffmpeg_path="ffmpeg", silence_db=-30, silence_duration=0.5):
    """Tìm các điểm bắt đầu khoảng lặng bằng ffmpeg silencedetect (một pass, rẻ)."""
    cmd = [
        ffmpeg_path, '-i', audio_path,
        '-af', f'silencedetect=noise={silence_db}dB:d={silence_duration}',
        '-f', 'null', '-'
    ]
    silence_starts = []
    try:
        result = subprocess.run(cmd, stderr=subprocess.PIPE, text=True)
        matches = re.finditer(r'silence_start:\s+([\d\.]+)', result.stderr)
        for match in matches:
            silence_starts.append(float(match.group(1)))
    except Exception as e:
        print(f"Loi khi chay ffmpeg silencedetect: {e}", file=sys.stderr)
    return silence_starts


def analyze_black(proxy_path, ffmpeg_path="ffmpeg", black_duration=0.1):
    """Phát hiện màn hình đen bằng ffmpeg blackdetect (native, một pass — không cần
    decode lại bằng Python như ThresholdDetector). Trả về danh sách thời điểm bắt đầu."""
    cmd = [
        ffmpeg_path, '-i', proxy_path,
        '-vf', f'blackdetect=d={black_duration}:pic_th=0.98',
        '-an', '-f', 'null', '-'
    ]
    black_starts = []
    try:
        result = subprocess.run(cmd, stderr=subprocess.PIPE, text=True)
        matches = re.finditer(r'black_start:([\d\.]+)', result.stderr)
        for match in matches:
            black_starts.append(float(match.group(1)))
    except Exception as e:
        print(f"Loi khi chay ffmpeg blackdetect: {e}", file=sys.stderr)
    return black_starts


def analyze_scenes(proxy_path, scene_threshold=27.0):
    """Chuyển cảnh nội dung — scenedetect decode proxy MỘT LẦN. Trả về (times, scene_list)."""
    from scenedetect import detect, ContentDetector
    scene_list = detect(proxy_path, ContentDetector(threshold=scene_threshold))
    return [s[0].get_seconds() for s in scene_list]


def analyze_layout(proxy_path, sample_fps=2.0):
    """Phát hiện thay đổi bố cục / vùng nội dung bằng OpenCV.

    Lấy mẫu frame theo sample_fps, so sánh histogram vùng biên (letterbox, watermark,
    thay đổi khung hình). Khi độ chênh lệch vượt ngưỡng → coi là điểm đổi bố cục.
    Trả về danh sách timestamp nghi ngờ đổi layout.
    """
    try:
        import cv2
        import numpy as np
    except Exception as e:
        print(f"OpenCV/numpy khong san sang, bo qua layout: {e}", file=sys.stderr)
        return []

    cap = cv2.VideoCapture(proxy_path)
    if not cap.isOpened():
        return []

    fps = cap.get(cv2.CAP_PROP_FPS) or 15.0
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
                gray = cv2.cvtColor(frame, cv2.COLOR_BGR2GRAY)
                # Histogram toàn khung (thay đổi tông màu / nền tổng thể).
                hist = cv2.calcHist([gray], [0], None, [32], [0, 256])
                cv2.normalize(hist, hist)
                # "Edge profile" theo hàng/cột — bắt letterbox và vùng nội dung.
                col_energy = np.mean(np.abs(np.diff(gray.astype(np.float32), axis=1)), axis=0)
                col_energy = col_energy / (np.linalg.norm(col_energy) + 1e-6)

                if prev_hist is not None:
                    hist_diff = cv2.compareHist(prev_hist, hist, cv2.HISTCMP_BHATTACHARYYA)
                    edge_diff = float(np.linalg.norm(col_energy - prev_edge))
                    # Ngưỡng thực nghiệm: kết hợp cả hai để giảm nhiễu.
                    if hist_diff > 0.35 and edge_diff > 0.5:
                        changes.append(idx / fps)

                prev_hist = hist
                prev_edge = col_energy
        idx += 1

    cap.release()
    return changes


def analyze_audio_spectral(audio_path, sr=16000, hop=8000, delta_threshold=0.55):
    """Phát hiện thay đổi phổ âm thanh (đổi nhạc nền / môi trường / người nói) bằng librosa.

    Tính MFCC theo khung, lấy khoảng cách giữa các khung liên tiếp; đỉnh lớn → nghi
    ngờ đổi nguồn âm thanh. Trả về danh sách timestamp.
    """
    try:
        import librosa
        import numpy as np
    except Exception as e:
        print(f"librosa khong san sang, bo qua audio spectral: {e}", file=sys.stderr)
        return []

    try:
        y, sr = librosa.load(audio_path, sr=sr, mono=True)
    except Exception as e:
        print(f"Loi doc audio: {e}", file=sys.stderr)
        return []

    if y is None or len(y) == 0:
        return []

    mfcc = librosa.feature.mfcc(y=y, sr=sr, n_mfcc=13, hop_length=hop)
    if mfcc.shape[1] < 3:
        return []

    # Chuẩn hóa từng chiều rồi tính khoảng cách khung liên tiếp.
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


def analyze_video(proxy_path, audio_path, ffmpeg_path="ffmpeg", mode="smart",
                  scene_threshold=27.0, silence_db=-30, silence_duration=0.5,
                  source_path=None):
    """Chạy các detector theo chế độ.

    fast    : màn hình đen + im lặng (nhanh nhất, dựa dấu ngắt rõ).
    smart   : + chuyển cảnh nội dung + đổi bố cục (mặc định, cân bằng).
    precise : + phổ âm thanh (librosa), chậm hơn nhưng ít sót/nhầm.
    """
    candidates_map = {}

    # Dùng video gốc cho blackdetect/silencedetect (timestamp chính xác).
    # Proxy chỉ dùng cho scenedetect/layout (cần decode frame, proxy nhẹ hơn).
    detect_path = source_path if source_path else proxy_path

    def add_signal(timestamp, conf, signal_key, signal_val, reason_str):
        """Gộp các tín hiệu nằm gần nhau (< 1.0s) vào cùng một candidate.
        Tăng từ 0.5s lên 1.0s vì proxy FPS thấp (15fps) gây chênh lệch
        timestamp giữa các detector lên tới ~1s."""
        for ts, cand in candidates_map.items():
            if abs(ts - timestamp) < 1.0:
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

    # --- Tín hiệu dùng chung cho mọi chế độ: màn hình đen + im lặng ---
    print("PROGRESS:20", flush=True)
    black_starts = analyze_black(detect_path, ffmpeg_path=ffmpeg_path)
    for start_time in black_starts:
        add_signal(start_time, 50, "black_frame", 30, "Black frame")

    print("PROGRESS:35", flush=True)
    # silencedetect luôn chạy trên audio gốc (đã extract từ source) — đúng timestamp.
    silences = analyze_silence(audio_path, ffmpeg_path=ffmpeg_path,
                               silence_db=silence_db, silence_duration=silence_duration)
    for s_time in silences:
        add_signal(s_time, 30, "silence", 20, "Silence start")

    # --- smart & precise: thêm chuyển cảnh + layout ---
    if mode in ("smart", "precise"):
        print("PROGRESS:55", flush=True)
        for t in analyze_scenes(proxy_path, scene_threshold=scene_threshold):
            add_signal(t, 40, "visual_change", 30, "Scene change")

        print("PROGRESS:75", flush=True)
        for t in analyze_layout(proxy_path):
            add_signal(t, 35, "layout_change", 25, "Layout change")

    # --- precise: thêm phổ âm thanh ---
    if mode == "precise":
        print("PROGRESS:88", flush=True)
        for t in analyze_audio_spectral(audio_path):
            add_signal(t, 30, "audio_change", 20, "Audio change")

    print("PROGRESS:95", flush=True)

    candidates = list(candidates_map.values())
    candidates.sort(key=lambda x: x['timestamp'])

    return {
        "status": "success",
        "proxy_path": proxy_path,
        "audio_path": audio_path,
        "candidates": candidates,
    }


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description="Media Analyzer Worker")
    parser.add_argument("--proxy", type=str, required=True, help="Path to proxy video")
    parser.add_argument("--audio", type=str, required=True, help="Path to audio file")
    parser.add_argument("--source", type=str, default="", help="Path to original source video (for blackdetect/silencedetect)")
    parser.add_argument("--ffmpeg", type=str, default="ffmpeg", help="Path to ffmpeg executable")
    parser.add_argument("--mode", type=str, default="smart", choices=["fast", "smart", "precise"], help="Analysis mode")
    parser.add_argument("--scene-threshold", type=float, default=27.0, help="Scene detection threshold (default: 27.0)")
    parser.add_argument("--silence-db", type=float, default=-30, help="Silence noise threshold in dB (default: -30)")
    parser.add_argument("--silence-duration", type=float, default=0.5, help="Min silence duration in seconds (default: 0.5)")

    args = parser.parse_args()

    try:
        data = analyze_video(
            args.proxy,
            args.audio,
            ffmpeg_path=args.ffmpeg,
            mode=args.mode,
            scene_threshold=args.scene_threshold,
            silence_db=args.silence_db,
            silence_duration=args.silence_duration,
            source_path=args.source if args.source else None,
        )
        print(json.dumps(data))
    except Exception as e:
        print(json.dumps({"status": "error", "message": str(e)}))
        sys.exit(1)
