# PROMPT CHUẨN BỊ XÂY DỰNG TOOL TỰ ĐỘNG TÁCH NHIỀU VIDEO NGẮN TỪ MỘT VIDEO DÀI

## 1. Vai trò

Bạn là **Senior Software Architect, Video Processing Engineer và AI Engineer**, có kinh nghiệm thực tế với:

* Wails.
* Go.
* Vue 3.
* TypeScript.
* FFmpeg.
* ffprobe.
* Python.
* OpenCV.
* PySceneDetect.
* Xử lý hình ảnh và âm thanh.
* Thiết kế ứng dụng desktop chạy offline.
* Xử lý video hàng loạt.
* Quản lý tiến trình và tài nguyên CPU, GPU, RAM.

Nhiệm vụ của bạn là phân tích và chuẩn bị kiến trúc kỹ thuật cho một ứng dụng desktop có khả năng tự động tách toàn bộ video ngắn đang được ghép nối trong một video dài.

Không code ngay khi chưa hoàn thành bước phân tích và thiết kế.

---

## 2. Mục tiêu sản phẩm

Xây dựng một ứng dụng desktop cho Windows có chức năng:

1. Người dùng chọn một video dài.
2. Tool tự động phân tích toàn bộ video.
3. Tool phát hiện điểm bắt đầu và kết thúc của từng video ngắn.
4. Tool phân biệt:

   * Ranh giới giữa hai video ngắn.
   * Chuyển cảnh bình thường bên trong cùng một video ngắn.
5. Tool tạo danh sách toàn bộ video ngắn đã phát hiện.
6. Người dùng có thể xem trước, chỉnh sửa hoặc loại bỏ điểm cắt sai.
7. Tool xuất toàn bộ video ngắn thành các file riêng.
8. Video đầu ra phải:

   * Cắt đúng thời điểm.
   * Không lệch hình và tiếng.
   * Không mất frame đầu.
   * Không mất âm thanh đầu clip.
   * Không bị màn hình đen không mong muốn.
   * Có tên file và thứ tự rõ ràng.
9. Tool chạy offline trên máy người dùng.
10. Phiên bản đầu không cần tài khoản, server hoặc hệ thống cloud.

Ví dụ kết quả:

```text
video_dai.mp4
├── video_001.mp4
├── video_002.mp4
├── video_003.mp4
├── video_004.mp4
└── ...
```

---

## 3. Phương án kỹ thuật đã chọn

Sử dụng phương án **kết hợp phân tích hình ảnh và âm thanh**.

Không được chỉ dùng phát hiện chuyển cảnh vì một video ngắn có thể chứa nhiều cảnh quay khác nhau.

Hệ thống phải kết hợp nhiều tín hiệu để xác định ranh giới thật giữa các video.

### Tín hiệu hình ảnh

Phân tích:

* Thay đổi mạnh giữa các frame.
* Chuyển cảnh đột ngột.
* Màn hình đen.
* Màn hình trắng.
* Fade in.
* Fade out.
* Frame đứng yên.
* Thay đổi bố cục.
* Thay đổi tỷ lệ hoặc vùng hiển thị nội dung.
* Thay đổi watermark.
* Thay đổi username.
* Thay đổi kiểu, màu sắc hoặc vị trí phụ đề.
* Sự xuất hiện của intro hoặc outro.
* Sự thay đổi nền, logo hoặc khung video.

### Tín hiệu âm thanh

Phân tích:

* Khoảng im lặng.
* Thay đổi âm lượng đột ngột.
* Thay đổi nhạc nền.
* Thay đổi phổ âm thanh.
* Thay đổi môi trường ghi âm.
* Thay đổi người nói.
* Âm thanh bị ngắt rồi bắt đầu lại.
* Câu nói có tiếp tục xuyên qua điểm chuyển cảnh hay không.
* Sự xuất hiện của intro hoặc outro âm thanh.

---

## 4. Kiến trúc công nghệ dự kiến

Sử dụng kiến trúc sau:

```text
Wails Desktop Application
│
├── Vue 3 + TypeScript
│   ├── Import video
│   ├── Video preview
│   ├── Timeline
│   ├── Danh sách clip
│   ├── Preview đầu và cuối clip
│   ├── Chỉnh điểm bắt đầu và kết thúc
│   ├── Cấu hình độ nhạy
│   └── Theo dõi tiến trình xuất video
│
├── Go Backend
│   ├── Quản lý project
│   ├── Quản lý file
│   ├── Điều phối tiến trình
│   ├── Gọi FFmpeg và ffprobe
│   ├── Gọi Python worker
│   ├── Hợp nhất kết quả phân tích
│   ├── Tính điểm ranh giới
│   ├── Quản lý hàng đợi xuất video
│   ├── Hủy tác vụ
│   └── Xử lý lỗi
│
├── FFmpeg + ffprobe
│   ├── Đọc metadata video
│   ├── Tạo proxy
│   ├── Tách âm thanh
│   ├── Phát hiện màn hình đen
│   ├── Phát hiện khoảng im lặng
│   ├── Cắt video
│   ├── Encode video
│   └── Kiểm tra đầu ra
│
├── Python Worker
│   ├── PySceneDetect
│   ├── OpenCV
│   ├── Phân tích frame
│   ├── Phân tích bố cục
│   ├── Phân tích đặc trưng âm thanh
│   └── Các module AI nâng cấp sau này
│
└── SQLite
    ├── Project
    ├── Video nguồn
    ├── Các điểm ranh giới
    ├── Danh sách clip
    ├── Cấu hình
    └── Lịch sử xuất video
```

---

## 5. Nguyên tắc phát hiện ranh giới

Không được áp dụng quy tắc đơn giản:

```text
Có chuyển cảnh → cắt video
```

Phải sử dụng hệ thống chấm điểm ranh giới.

Ví dụ:

```text
Boundary Score =
    Visual Change Score
  + Audio Change Score
  + Black/Fade Score
  + Silence Score
  + Layout Change Score
  + Subtitle/Watermark Change Score
  + Speaker Change Score
  + Intro/Outro Score
  - Continuity Penalty
```

Trong đó `Continuity Penalty` dùng để giảm điểm khi:

* Giọng nói vẫn tiếp tục.
* Câu nói chưa kết thúc.
* Nhạc nền không thay đổi.
* Phụ đề vẫn cùng kiểu.
* Bố cục vẫn giống nhau.
* Điểm nghi ngờ chỉ là chuyển cảnh minh họa.

Hãy đề xuất trọng số ban đầu cho từng tín hiệu.

Các trọng số phải có thể cấu hình và điều chỉnh theo dữ liệu thực tế.

---

## 6. Phân loại kết quả

Mỗi điểm ranh giới phải có:

* Thời gian.
* Frame hoặc PTS tương ứng.
* Điểm tin cậy.
* Các tín hiệu đã phát hiện.
* Lý do được chọn.
* Thumbnail trước điểm cắt.
* Thumbnail sau điểm cắt.

Chia thành ba mức:

```text
Từ 85% trở lên:
Tự động chấp nhận làm ranh giới.

Từ 60% đến 84%:
Yêu cầu người dùng kiểm tra.

Dưới 60%:
Không sử dụng làm ranh giới.
```

Các ngưỡng này phải có thể cấu hình.

---

## 7. Quy trình xử lý dự kiến

Thiết kế pipeline theo hướng:

```text
Bước 1: Người dùng chọn video dài
        ↓
Bước 2: ffprobe đọc metadata
        ↓
Bước 3: Tạo proxy video độ phân giải thấp
        ↓
Bước 4: Tách audio dùng cho phân tích
        ↓
Bước 5: Phân tích hình ảnh sơ bộ
        ↓
Bước 6: Phân tích âm thanh sơ bộ
        ↓
Bước 7: Tạo danh sách điểm nghi ngờ
        ↓
Bước 8: Gom các điểm gần nhau
        ↓
Bước 9: Phân tích chi tiết quanh từng điểm
        ↓
Bước 10: Tính Boundary Score
        ↓
Bước 11: Áp dụng quy tắc hậu xử lý
        ↓
Bước 12: Tạo danh sách clip
        ↓
Bước 13: Hiển thị để người dùng kiểm tra
        ↓
Bước 14: Cắt chính xác từ video gốc
        ↓
Bước 15: Dùng ffprobe kiểm tra file đầu ra
```

---

## 8. Quy tắc hậu xử lý bắt buộc

Thiết kế các quy tắc:

1. Khoảng cách tối thiểu giữa hai ranh giới.
2. Thời lượng clip tối thiểu.
3. Thời lượng clip tối đa.
4. Gom các điểm nghi ngờ nằm gần nhau.
5. Loại bỏ flash, hiệu ứng chớp sáng hoặc frame lỗi.
6. Không cắt khi câu nói vẫn tiếp tục.
7. Không cắt chỉ vì thay đổi camera.
8. Kiểm tra tín hiệu mới có duy trì sau điểm chuyển hay không.
9. Kiểm tra clip đầu tiên bắt đầu từ đầu video.
10. Kiểm tra clip cuối cùng kết thúc tại cuối video.
11. Không tạo clip rỗng.
12. Không tạo clip chỉ dài vài frame.
13. Xử lý video không có audio.
14. Xử lý video Variable Frame Rate.
15. Xử lý video có nhiều audio track.
16. Xử lý timestamp không bắt đầu từ 0.
17. Xử lý video bị xoay theo metadata.
18. Xử lý codec và container khác nhau.

---

## 9. Yêu cầu cắt video chính xác

Không dùng stream copy làm chế độ chính xác mặc định.

Chế độ xuất chính xác phải:

* Decode và encode lại.
* Cắt dựa trên timestamp, frame hoặc PTS chính xác.
* Đồng bộ video và audio.
* Reset timestamp của từng clip về 0.
* Không làm mất phần đầu âm thanh.
* Không để âm thanh bắt đầu trước hình ảnh.
* Kiểm tra duration thực tế sau khi xuất.
* Cho phép sai số tối đa khoảng một frame hoặc giới hạn kỹ thuật hợp lý của codec.

Hãy phân tích cách sử dụng:

* `trim`.
* `atrim`.
* `setpts`.
* `asetpts`.
* `-ss`.
* `-to`.
* `-t`.
* `-copyts`.
* `-start_at_zero`.
* `-avoid_negative_ts`.
* `-movflags +faststart`.

Không dùng tham số một cách máy móc. Phải giải thích tham số nào thực sự cần cho kiến trúc này.

---

## 10. Chế độ hoạt động của sản phẩm

Thiết kế ít nhất ba chế độ:

### Chế độ Tách nhanh

* Dựa chủ yếu vào màn hình đen, im lặng và fade.
* Tốc độ cao.
* Phù hợp video có dấu ngắt rõ.

### Chế độ Tách thông minh

* Kết hợp hình ảnh, âm thanh, bố cục, phụ đề và watermark.
* Đây là chế độ mặc định.
* Cân bằng độ chính xác và tốc độ.

### Chế độ Chính xác cao

* Phân tích nhiều frame hơn.
* Phân tích chi tiết hơn quanh các điểm nghi ngờ.
* Có thể thêm OCR, transcript hoặc speaker analysis.
* Chậm hơn nhưng giảm sai sót.

---

## 11. Giao diện MVP

Thiết kế các màn hình:

### Màn hình bắt đầu

* Kéo thả video.
* Chọn file.
* Hiển thị thông tin video.
* Chọn chế độ phân tích.
* Chọn thư mục đầu ra.

### Màn hình phân tích

* Tiến trình hiện tại.
* Giai đoạn đang chạy.
* Số điểm nghi ngờ đã tìm thấy.
* Số clip dự kiến.
* Nút tạm dừng.
* Nút hủy.

### Màn hình kiểm tra kết quả

Mỗi clip hiển thị:

* Số thứ tự.
* Thumbnail đầu.
* Thumbnail cuối.
* Thời gian bắt đầu.
* Thời gian kết thúc.
* Thời lượng.
* Điểm tin cậy.
* Lý do xác định ranh giới.
* Nút xem trước.
* Nút chỉnh đầu clip.
* Nút chỉnh cuối clip.
* Nút gộp với clip trước.
* Nút gộp với clip sau.
* Nút chia clip.
* Nút loại bỏ clip.

### Màn hình xuất video

* Preset chất lượng.
* Codec.
* Độ phân giải.
* Thư mục đầu ra.
* Quy tắc đặt tên.
* Số job chạy song song.
* Tiến trình từng clip.
* Tiến trình tổng.
* Danh sách lỗi.
* Nút mở thư mục kết quả.

---

## 12. Yêu cầu hiệu năng

Phải phân tích và đề xuất:

* Khi nào dùng proxy video.
* Độ phân giải proxy phù hợp.
* Tần suất lấy mẫu frame.
* Khi nào cần phân tích mọi frame.
* Cách hạn chế dùng RAM.
* Cách quản lý file tạm.
* Cách chia batch.
* Số tiến trình FFmpeg chạy song song.
* Cách phát hiện CPU, RAM và GPU.
* Cách chọn encoder CPU hoặc GPU.
* Cách hủy tiến trình FFmpeg an toàn.
* Cách phục hồi khi ứng dụng bị đóng giữa chừng.
* Cách tránh decode toàn bộ video nhiều lần.
* Cách lưu cache kết quả phân tích.

Không được chạy số lượng tiến trình không giới hạn.

---

## 13. Yêu cầu về cấu trúc dự án

Đề xuất cấu trúc thư mục rõ ràng, ví dụ:

```text
video-splitter/
├── apps/
│   ├── desktop/
│   └── worker/
├── frontend/
├── backend/
├── internal/
│   ├── project/
│   ├── media/
│   ├── analyzer/
│   ├── boundary/
│   ├── exporter/
│   ├── process/
│   └── storage/
├── python/
│   ├── scene/
│   ├── visual/
│   ├── audio/
│   └── protocol/
├── migrations/
├── scripts/
├── tests/
├── docs/
└── assets/
```

Không bắt buộc giữ nguyên cấu trúc trên nếu có phương án tốt hơn.

Mỗi thư mục và module phải được giải thích rõ trách nhiệm.

---

## 14. Giao tiếp giữa Go và Python

Phân tích và chọn một phương án phù hợp cho MVP:

* Chạy Python bằng process.
* Truyền dữ liệu bằng JSON qua stdin/stdout.
* Dùng file JSON trung gian.
* Dùng local HTTP.
* Dùng gRPC.

Ưu tiên phương án đơn giản, ổn định và dễ đóng gói.

Không đưa gRPC hoặc WebSocket vào chỉ vì chúng phổ biến.

Hãy chốt một phương án và giải thích:

* Vì sao chọn.
* Dữ liệu gửi sang Python.
* Dữ liệu Python trả về.
* Cách truyền tiến trình.
* Cách truyền lỗi.
* Cách hủy tác vụ.
* Cách version hóa protocol.

---

## 15. Định dạng dữ liệu đề xuất

Thiết kế schema dữ liệu cho:

### Project

```json
{
  "id": "",
  "sourcePath": "",
  "proxyPath": "",
  "duration": 0,
  "width": 0,
  "height": 0,
  "fps": "",
  "timeBase": "",
  "status": ""
}
```

### Boundary Candidate

```json
{
  "timestamp": 0,
  "pts": 0,
  "frameNumber": 0,
  "confidence": 0,
  "signals": {},
  "reason": "",
  "accepted": false
}
```

### Clip

```json
{
  "id": "",
  "index": 1,
  "startTime": 0,
  "endTime": 0,
  "startPts": 0,
  "endPts": 0,
  "duration": 0,
  "confidence": 0,
  "status": ""
}
```

Hãy chuẩn hóa và mở rộng các schema này nếu cần.

---

## 16. Kiểm thử bắt buộc

Lập kế hoạch test cho các trường hợp:

1. Video có màn hình đen giữa các clip.
2. Video nối liền hoàn toàn.
3. Video có nhiều chuyển cảnh trong cùng clip.
4. Video có nhạc nền xuyên suốt.
5. Video không có audio.
6. Video có khoảng im lặng dài giữa câu nói.
7. Video có nhiều watermark khác nhau.
8. Video cùng một watermark.
9. Video Variable Frame Rate.
10. Video 30 FPS.
11. Video 60 FPS.
12. Video dọc.
13. Video ngang.
14. Video 4K.
15. Video dài nhiều giờ.
16. Video bị lỗi nhẹ.
17. Video có timestamp âm.
18. Video có nhiều audio stream.
19. Video có intro và outro lặp lại.
20. Video reaction có nhiều vùng nội dung.
21. Một video chỉ chứa một clip.
22. Video chứa hàng trăm clip ngắn.

Mỗi test phải có:

* Input.
* Kết quả mong đợi.
* Sai số cho phép.
* Cách đo precision.
* Cách đo recall.
* Cách đo sai số điểm cắt.
* Cách đo độ lệch audio/video.

---

## 17. Ràng buộc

* Không sử dụng cloud trong MVP.
* Không yêu cầu đăng nhập.
* Không dùng PostgreSQL.
* Không dùng Redis.
* Không dùng microservice.
* Không dùng Docker làm yêu cầu bắt buộc cho người dùng cuối.
* Không thêm công nghệ không cần thiết.
* Không để Vue gọi FFmpeg trực tiếp.
* Không để Go xử lý thuật toán thị giác phức tạp nếu Python phù hợp hơn.
* Không để Python quản lý toàn bộ ứng dụng desktop.
* Không cắt video ngay khi chỉ có một tín hiệu yếu.
* Không cam kết chính xác 100% với mọi loại video.
* Phải có màn hình kiểm tra kết quả trước khi xuất.
* Mọi quyết định công nghệ phải có lý do.
* Ưu tiên MVP có thể hoàn thành và kiểm thử được.

---

## 18. Format đầu ra bắt buộc

Hãy tạo một báo cáo chuẩn bị dự án dưới dạng Markdown, gồm đúng các phần:

### 1. Tóm tắt bài toán

Giải thích rõ tool đang giải quyết vấn đề gì.

### 2. Phân tích khó khăn kỹ thuật

Phân biệt rõ chuyển cảnh và ranh giới video.

### 3. Kiến trúc tổng thể

Có sơ đồ luồng dữ liệu.

### 4. Công nghệ được chọn

Mỗi công nghệ phải ghi:

* Dùng để làm gì.
* Nằm ở đâu trong dự án.
* Vì sao chọn.
* Không dùng nó cho việc gì.

### 5. Pipeline phân tích hình ảnh

Mô tả từng bước.

### 6. Pipeline phân tích âm thanh

Mô tả từng bước.

### 7. Thuật toán Boundary Score

Có công thức, trọng số ban đầu và pseudo-code.

### 8. Quy tắc hậu xử lý

Liệt kê rõ.

### 9. Pipeline cắt và xuất video

Giải thích chính xác cách tránh lệch hình và tiếng.

### 10. Thiết kế dữ liệu

Bao gồm Project, Boundary, Clip, Analysis Job và Export Job.

### 11. Cấu trúc thư mục

Giải thích từng module.

### 12. Thiết kế giao diện MVP

Liệt kê màn hình, component và thao tác chính.

### 13. Kế hoạch triển khai theo giai đoạn

Chia thành:

* Giai đoạn 0: nghiên cứu và tạo bộ dữ liệu test.
* Giai đoạn 1: cắt video thủ công chính xác.
* Giai đoạn 2: phát hiện màn hình đen và im lặng.
* Giai đoạn 3: phát hiện hình ảnh và âm thanh kết hợp.
* Giai đoạn 4: màn hình kiểm tra và sửa ranh giới.
* Giai đoạn 5: tối ưu xử lý hàng loạt.
* Giai đoạn 6: bổ sung OCR, transcript hoặc AI.

### 14. Danh sách task triển khai

Tách task theo:

* Frontend.
* Go backend.
* Python worker.
* FFmpeg.
* Database.
* Testing.
* Packaging.

Mỗi task phải có:

* Mục tiêu.
* File hoặc module dự kiến.
* Phụ thuộc.
* Kết quả đầu ra.
* Tiêu chí hoàn thành.

### 15. Rủi ro kỹ thuật

Phân tích rủi ro và phương án giảm thiểu.

### 16. Tiêu chí hoàn thành MVP

Viết thành checklist có thể kiểm tra.

---

## 19. Tiêu chí hoàn thành tài liệu chuẩn bị

Tài liệu chỉ được coi là hoàn thành khi:

* Chốt được kiến trúc cụ thể.
* Chốt được trách nhiệm của Vue, Go, Python và FFmpeg.
* Có pipeline xử lý từ đầu đến cuối.
* Có thuật toán chấm điểm ranh giới ban đầu.
* Có quy tắc tránh cắt nhầm chuyển cảnh.
* Có cấu trúc project.
* Có schema dữ liệu.
* Có kế hoạch test.
* Có kế hoạch phát triển theo giai đoạn.
* Có danh sách task đủ chi tiết để bắt đầu code.
* Không còn công nghệ được liệt kê nhưng không rõ dùng ở đâu.
* Không thêm thành phần thừa.
* Phân biệt rõ phần bắt buộc của MVP và phần nâng cấp sau.
* Mọi quyết định quan trọng đều có lý do kỹ thuật.

Sau khi hoàn thành báo cáo, hãy kết luận rõ:

1. Kiến trúc cuối cùng được chọn.
2. Phạm vi MVP.
3. Những chức năng chưa làm trong MVP.
4. Module nào cần xây đầu tiên.
5. Thứ tự triển khai phù hợp nhất.
6. Những thử nghiệm kỹ thuật cần làm trước khi bắt đầu xây giao diện hoàn chỉnh.
