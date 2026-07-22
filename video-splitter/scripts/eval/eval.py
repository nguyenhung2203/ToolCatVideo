"""
eval.py — Harness đo chất lượng phát hiện điểm cắt của worker.

Mục đích: đo Precision / Recall / F1 và Cut-Error (sai số vị trí điểm cắt)
của thuật toán phát hiện ranh giới, để so sánh KHÁCH QUAN giữa các phiên bản
thuật toán (regression tracking) thay vì đánh giá cảm tính.

Cách dùng:
    # Đo 1 video (cần file ground-truth <video>.cuts.json cạnh video)
    python eval.py --worker ../../python_worker/main.py --video path/to/x.mp4 --mode smart

    # Đo cả thư mục (mọi video có sidecar .cuts.json)
    python eval.py --worker ../../python_worker/main.py --dir path/to/testset --mode smart

    # So với baseline đã lưu
    python eval.py --dir testset --mode smart --baseline baseline_smart.json

Ground-truth sidecar (đặt cạnh video, tên = <video>.cuts.json):
    {
        "source": "x.mp4",
        "category": "vlog",              # loại video (theo docs/promt.md §16), tuỳ chọn
        "tolerance": 0.5,                 # dung sai match (giây), tuỳ chọn (mặc định 0.5)
        "cuts": [12.34, 45.6, 78.9]       # thời điểm BẮT ĐẦU mỗi clip (bỏ 0 và điểm cuối)
    }

Chỉ số:
    Precision = TP / (TP + FP)   — trong các điểm cắt dự đoán, bao nhiêu là đúng
    Recall    = TP / (TP + FN)   — trong các ranh giới thật, bắt được bao nhiêu
    F1        = 2PR / (P + R)
    CutError  = median |pred - truth| trên các cặp đã match (giây)
"""

import argparse
import json
import os
import subprocess
import sys
import glob
import tempfile
from statistics import median

# stdout UTF-8 để in tiếng Việt trên Windows (console mặc định cp1252).
for _s in (sys.stdout, sys.stderr):
    try:
        _s.reconfigure(encoding="utf-8")
    except Exception:
        pass


def _make_proxy(video_path, ffmpeg, tmpdir):
    """Tạo proxy 320×180@4fps giống app thật (smart/precise cần proxy để quét hình ảnh)."""
    out = os.path.join(tmpdir, "proxy.mp4")
    cmd = [ffmpeg, "-y", "-hide_banner", "-loglevel", "error", "-i", video_path,
           "-vf", "scale=320:180,fps=4", "-an", "-fps_mode", "vfr",
           "-c:v", "libx264", "-preset", "ultrafast", "-crf", "30", out]
    subprocess.run(cmd, stderr=subprocess.DEVNULL, timeout=1800)
    return out if os.path.exists(out) else ""


def _make_wav(video_path, ffmpeg, tmpdir):
    """Trích WAV mono 16k giống app thật (smart/precise dùng audio novelty + continuity)."""
    out = os.path.join(tmpdir, "audio.wav")
    cmd = [ffmpeg, "-y", "-hide_banner", "-loglevel", "error", "-i", video_path,
           "-vn", "-map", "0:a:0?", "-acodec", "pcm_s16le", "-ar", "16000", "-ac", "1", out]
    subprocess.run(cmd, stderr=subprocess.DEVNULL, timeout=1800)
    return out if os.path.exists(out) else ""


def run_worker(worker_path, video_path, mode, ffmpeg="ffmpeg", ffprobe="ffprobe",
               proxy="", audio="", python_exe=None):
    """Chạy worker main.py trên 1 video, trả về list timestamp điểm cắt dự đoán.

    Tạo proxy + WAV tạm giống hệt app thật (app.go) để đo ĐÚNG những gì người dùng
    nhận được: smart/precise cần proxy để quét hình ảnh, và WAV cho audio novelty.
    """
    import tempfile, shutil
    exe = python_exe or sys.executable
    tmpdir = tempfile.mkdtemp(prefix="vs_eval_")
    try:
        if mode != "fast":
            if not proxy:
                proxy = _make_proxy(video_path, ffmpeg, tmpdir)
            if not audio:
                audio = _make_wav(video_path, ffmpeg, tmpdir)

        cmd = [
            exe, worker_path,
            "--source", video_path,
            "--proxy", proxy,
            "--audio", audio,
            "--ffmpeg", ffmpeg,
            "--mode", mode,
        ]
        env = dict(os.environ, PYTHONUTF8="1", PYTHONIOENCODING="utf-8")
        proc = subprocess.run(cmd, capture_output=True, text=True, encoding="utf-8", env=env)
    finally:
        shutil.rmtree(tmpdir, ignore_errors=True)

    # Worker in log (PROGRESS:/STATUS_LOG:/DETECTOR_DONE:) lẫn JSON ra stdout.
    # JSON là các dòng KHÔNG có prefix — gom lại rồi parse (giống analyzer.go).
    json_buf = []
    for line in proc.stdout.splitlines():
        if line.startswith(("PROGRESS:", "STATUS_LOG:", "DETECTOR_DONE:")):
            continue
        json_buf.append(line)
    raw = "".join(json_buf).strip()

    if not raw:
        raise RuntimeError(f"Worker không trả JSON.\nstderr:\n{proc.stderr[-2000:]}")

    data = json.loads(raw)
    if data.get("status") != "success":
        raise RuntimeError(f"Worker lỗi: {data.get('message')}")

    cands = data.get("candidates", [])
    return sorted(c["timestamp"] for c in cands)


def match_cuts(pred, truth, tolerance):
    """Greedy matching: mỗi truth match tối đa 1 pred gần nhất trong tolerance.

    Trả về (tp, fp, fn, errors) với errors = list |pred-truth| của cặp matched.
    """
    truth_used = [False] * len(truth)
    errors = []
    tp = 0

    # Sắp pred theo thứ tự, ưu tiên match truth gần nhất chưa dùng.
    for p in pred:
        best_i, best_d = -1, float("inf")
        for i, t in enumerate(truth):
            if truth_used[i]:
                continue
            d = abs(p - t)
            if d <= tolerance and d < best_d:
                best_d, best_i = d, i
        if best_i >= 0:
            truth_used[best_i] = True
            tp += 1
            errors.append(best_d)

    fp = len(pred) - tp
    fn = len(truth) - sum(truth_used)
    return tp, fp, fn, errors


def prf(tp, fp, fn):
    p = tp / (tp + fp) if (tp + fp) else 0.0
    r = tp / (tp + fn) if (tp + fn) else 0.0
    f1 = 2 * p * r / (p + r) if (p + r) else 0.0
    return p, r, f1


def find_sidecar(video_path):
    """Tìm file ground-truth cạnh video: <video>.cuts.json hoặc <stem>.cuts.json."""
    for cand in (video_path + ".cuts.json",
                 os.path.splitext(video_path)[0] + ".cuts.json"):
        if os.path.exists(cand):
            return cand
    return None


def eval_one(worker_path, video_path, mode, ffmpeg, ffprobe, python_exe):
    sidecar = find_sidecar(video_path)
    if not sidecar:
        return None
    with open(sidecar, "r", encoding="utf-8") as f:
        gt = json.load(f)
    tol = gt.get("tolerance", 0.5)
    truth = sorted(gt.get("cuts", []))
    category = gt.get("category", "uncategorized")

    pred = run_worker(worker_path, video_path, mode, ffmpeg, ffprobe, python_exe=python_exe)
    tp, fp, fn, errors = match_cuts(pred, truth, tol)
    p, r, f1 = prf(tp, fp, fn)

    return {
        "video": os.path.basename(video_path),
        "category": category,
        "n_truth": len(truth),
        "n_pred": len(pred),
        "tp": tp, "fp": fp, "fn": fn,
        "precision": round(p, 3),
        "recall": round(r, 3),
        "f1": round(f1, 3),
        "cut_error_median": round(median(errors), 3) if errors else None,
        "tolerance": tol,
    }


def aggregate(results):
    """Gộp micro-average (cộng dồn tp/fp/fn) + cut-error toàn cục."""
    tp = sum(r["tp"] for r in results)
    fp = sum(r["fp"] for r in results)
    fn = sum(r["fn"] for r in results)
    p, r_, f1 = prf(tp, fp, fn)
    all_err = [r["cut_error_median"] for r in results if r["cut_error_median"] is not None]
    return {
        "n_videos": len(results),
        "tp": tp, "fp": fp, "fn": fn,
        "precision": round(p, 3),
        "recall": round(r_, 3),
        "f1": round(f1, 3),
        "cut_error_median": round(median(all_err), 3) if all_err else None,
    }


def per_category(results):
    cats = {}
    for r in results:
        cats.setdefault(r["category"], []).append(r)
    return {c: aggregate(rs) for c, rs in cats.items()}


def print_table(results, overall, by_cat):
    print("\n" + "=" * 78)
    print(f"{'VIDEO':<28}{'CAT':<12}{'P':>6}{'R':>6}{'F1':>6}{'CutErr':>9}{'pred/tru':>10}")
    print("-" * 78)
    for r in sorted(results, key=lambda x: (x["category"], x["video"])):
        ce = f"{r['cut_error_median']:.3f}" if r["cut_error_median"] is not None else "-"
        print(f"{r['video'][:27]:<28}{r['category'][:11]:<12}"
              f"{r['precision']:>6.2f}{r['recall']:>6.2f}{r['f1']:>6.2f}"
              f"{ce:>9}{r['n_pred']:>5}/{r['n_truth']:<4}")

    print("-" * 78)
    print("THEO LOẠI:")
    for cat, agg in sorted(by_cat.items()):
        ce = f"{agg['cut_error_median']:.3f}" if agg["cut_error_median"] is not None else "-"
        print(f"  {cat:<24}P={agg['precision']:.2f} R={agg['recall']:.2f} "
              f"F1={agg['f1']:.2f} CutErr={ce} ({agg['n_videos']} video)")

    print("-" * 78)
    ce = f"{overall['cut_error_median']:.3f}" if overall["cut_error_median"] is not None else "-"
    print(f"TỔNG (micro): P={overall['precision']:.3f} R={overall['recall']:.3f} "
          f"F1={overall['f1']:.3f} CutErr={ce}")
    print("=" * 78)


def compare_baseline(overall, baseline_path):
    if not os.path.exists(baseline_path):
        print(f"\n[baseline] Không tìm thấy {baseline_path} — bỏ qua so sánh.")
        return
    with open(baseline_path, "r", encoding="utf-8") as f:
        base = json.load(f).get("overall", {})
    print("\n[SO VỚI BASELINE]")
    for k in ("precision", "recall", "f1"):
        cur, old = overall.get(k, 0), base.get(k, 0)
        delta = cur - old
        arrow = "▲" if delta > 0 else ("▼" if delta < 0 else "=")
        print(f"  {k:<12}{old:.3f} → {cur:.3f}  {arrow}{abs(delta):.3f}")
    cur, old = overall.get("cut_error_median"), base.get("cut_error_median")
    if cur is not None and old is not None:
        delta = cur - old  # thấp hơn = tốt hơn
        arrow = "▲(tốt)" if delta < 0 else ("▼(tệ)" if delta > 0 else "=")
        print(f"  {'cut_error':<12}{old:.3f} → {cur:.3f}  {arrow}{abs(delta):.3f}")


def main():
    ap = argparse.ArgumentParser(description="Đo chất lượng phát hiện điểm cắt")
    ap.add_argument("--worker", required=True, help="Đường dẫn main.py của worker")
    ap.add_argument("--video", help="1 video cụ thể")
    ap.add_argument("--dir", help="Thư mục chứa nhiều video (quét *.cuts.json)")
    ap.add_argument("--mode", default="smart", choices=["fast", "smart", "precise"])
    ap.add_argument("--ffmpeg", default="ffmpeg")
    ap.add_argument("--ffprobe", default="ffprobe")
    ap.add_argument("--python", default=None, help="Python exe (mặc định: hiện tại)")
    ap.add_argument("--baseline", default=None, help="File baseline JSON để so sánh")
    ap.add_argument("--save", default=None, help="Lưu kết quả ra file JSON")
    args = ap.parse_args()

    videos = []
    if args.video:
        videos.append(args.video)
    if args.dir:
        for ext in ("*.mp4", "*.mkv", "*.mov", "*.avi", "*.webm", "*.ts"):
            videos.extend(glob.glob(os.path.join(args.dir, ext)))
    if not videos:
        print("Không có video nào. Dùng --video hoặc --dir.", file=sys.stderr)
        sys.exit(1)

    results = []
    for v in videos:
        if not find_sidecar(v):
            print(f"[skip] {os.path.basename(v)} — thiếu ground-truth .cuts.json")
            continue
        try:
            print(f"[run ] {os.path.basename(v)} (mode={args.mode})...", flush=True)
            res = eval_one(args.worker, v, args.mode, args.ffmpeg, args.ffprobe, args.python)
            if res:
                results.append(res)
        except Exception as e:
            print(f"[err ] {os.path.basename(v)}: {e}", file=sys.stderr)

    if not results:
        print("Không đo được video nào (thiếu ground-truth hoặc worker lỗi).")
        sys.exit(1)

    overall = aggregate(results)
    by_cat = per_category(results)
    print_table(results, overall, by_cat)

    if args.baseline:
        compare_baseline(overall, args.baseline)

    if args.save:
        with open(args.save, "w", encoding="utf-8") as f:
            json.dump({"mode": args.mode, "overall": overall,
                       "by_category": by_cat, "per_video": results},
                      f, ensure_ascii=False, indent=2)
        print(f"\nĐã lưu kết quả: {args.save}")


if __name__ == "__main__":
    main()
