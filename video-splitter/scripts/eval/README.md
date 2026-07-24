# Harness đo chất lượng phát hiện điểm cắt

Công cụ đo **khách quan** chất lượng thuật toán tách video, để so sánh giữa các
phiên bản thuật toán thay vì đánh giá bằng mắt.

## Chuẩn bị video test

1. Bỏ vài video thật vào một thư mục (ví dụ `testset/`), đa dạng loại:
   vlog nói liên tục, compilation có màn đen, gameplay, video dọc, video không audio, VFR...
2. Với mỗi video, tạo file ground-truth **cạnh nó**, tên `<tên-video>.cuts.json`:

```json
{
  "source": "vlog_01.mp4",
  "category": "vlog",
  "tolerance": 0.5,
  "cuts": [12.34, 45.6, 78.9]
}
```

- `cuts`: thời điểm (giây) **bắt đầu mỗi clip mới** — tức các điểm cắt thật.
  KHÔNG tính điểm 0 (đầu video) và điểm cuối (hết video).
- `category`: nhãn loại video để gộp theo nhóm (tuỳ chọn).
- `tolerance`: dung sai match tính bằng giây (mặc định 0.5). Điểm dự đoán lệch
  quá dung sai này so với điểm thật được coi là sai.

> Cách lấy `cuts` nhanh: mở video, tua tới từng chỗ chuyển clip, ghi lại mốc giây.

## Chạy đo

```bash
cd video-splitter/scripts/eval

# Đo cả thư mục testset, mode smart
python eval.py --worker ../../python_worker/main.py --dir testset --mode smart

# Lưu baseline TRƯỚC khi sửa thuật toán
python eval.py --worker ../../python_worker/main.py --dir testset --mode smart --save baseline_smart.json

# Sau khi sửa thuật toán, so với baseline
python eval.py --worker ../../python_worker/main.py --dir testset --mode smart --baseline baseline_smart.json
```

## Đọc kết quả

- **Precision** cao = ít cắt nhầm (ít điểm cắt thừa).
- **Recall** cao = ít bỏ sót ranh giới thật.
- **F1** = cân bằng hai cái trên (chỉ số tổng hợp chính).
- **CutErr** = sai số vị trí điểm cắt (giây), **càng thấp càng tốt**.

Quy trình đại tu: lưu baseline → sửa từng giai đoạn → chạy lại → mỗi giai đoạn
không được làm tụt F1 tổng.
