import os
import sys
import json
import cv2

# Force UTF-8 stdout
sys.stdout.reconfigure(encoding='utf-8')
sys.stderr.reconfigure(encoding='utf-8')

def main():
    if len(sys.argv) < 3 or sys.argv[1] != "--video":
        print("Usage: python extract_thumbs.py --video <path>", file=sys.stderr)
        sys.exit(1)

    video_path = sys.argv[2]
    if not os.path.exists(video_path):
        print(f"Error: Video file not found: {video_path}", file=sys.stderr)
        sys.exit(1)

    # Read jobs from stdin
    try:
        jobs_data = sys.stdin.read()
        jobs = json.loads(jobs_data)
    except Exception as e:
        print(f"Error reading JSON jobs from stdin: {e}", file=sys.stderr)
        sys.exit(1)

    if not jobs:
        sys.exit(0)

    import ctypes
    video_path_to_open = video_path
    if os.name == 'nt':
        try:
            output_buf = ctypes.create_unicode_buffer(1024)
            ctypes.windll.kernel32.GetShortPathNameW(video_path, output_buf, 1024)
            if output_buf.value:
                video_path_to_open = output_buf.value
        except Exception:
            pass

    cap = cv2.VideoCapture(video_path_to_open)
    if not cap.isOpened():
        print(f"Error: Could not open video file with OpenCV: {video_path} (short_path: {video_path_to_open})", file=sys.stderr)
        sys.exit(1)

    for job in jobs:
        index = job.get("index")
        timestamp = job.get("timestamp")
        path = job.get("path")
        job_type = job.get("type", "start") # "start" or "end"

        if index is None or timestamp is None or not path:
            continue

        # Seek to timestamp in milliseconds
        cap.set(cv2.CAP_PROP_POS_MSEC, timestamp * 1000.0)
        ret, frame = cap.read()
        if ret and frame is not None:
            # Ensure output folder exists
            os.makedirs(os.path.dirname(path), exist_ok=True)
            cv2.imwrite(path, frame)
            # Notify Go that this job is complete
            print(f"DONE:{index}|{job_type}|{path}", flush=True)
        else:
            print(f"FAILED:{index}|{job_type}", file=sys.stderr)

    cap.release()

if __name__ == "__main__":
    main()
