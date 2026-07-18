Bạn là Senior Performance Engineer chuyên tối ưu hệ thống xử lý video, FFmpeg, CPU, GPU, RAM và xử lý song song.

## Mục tiêu

Tối ưu tốc độ cho tool cắt video hiện tại.

Tool đã hoàn thiện chức năng và đang hoạt động đúng. Không xây dựng lại, không thêm tính năng mới, không thay đổi giao diện và không thay đổi kết quả cắt hiện tại.

Mục tiêu hiệu năng:

* Video dài 10 phút: xử lý dưới 2 phút.
* Video dài 30 phút: xử lý dưới 5 phút.
* Video dài 60 phút: xử lý dưới 10 phút.
* Không làm giảm đáng kể độ chính xác phát hiện và cắt clip.
* Không làm thay đổi định dạng đầu ra hiện tại nếu không thật sự cần thiết.

## Nguyên tắc bắt buộc

1. Đọc toàn bộ luồng xử lý hiện tại trước khi sửa.
2. Không refactor lớn nếu chưa chứng minh được lợi ích.
3. Không viết lại kiến trúc hoặc thay framework.
4. Không thay đổi logic nghiệp vụ.
5. Không thay đổi thuật toán phát hiện điểm cắt nếu thuật toán đó đang hoạt động đúng, trừ khi thay đổi giúp tăng tốc mà vẫn giữ kết quả tương đương.
6. Mọi tối ưu phải có số liệu trước và sau.
7. Ưu tiên sửa bottleneck lớn nhất trước.
8. Mỗi lần chỉ sửa một nhóm vấn đề để dễ đo hiệu quả và rollback.

## Bước 1: Benchmark hiện trạng

Trước khi sửa code, hãy đo thời gian từng công đoạn:

* Đọc metadata.
* Decode video.
* Phân tích hình ảnh.
* Phân tích âm thanh.
* Phát hiện điểm cắt.
* Ghép và lọc điểm cắt.
* Xuất từng clip.
* Encode hoặc render.
* Ghi file xuống ổ đĩa.
* Tổng thời gian xử lý.

Ghi lại:

* Thời gian từng bước.
* Phần trăm thời gian của từng bước.
* CPU sử dụng.
* RAM sử dụng.
* GPU sử dụng nếu có.
* Số lần video bị mở hoặc decode lại.
* Số tiến trình FFmpeg được tạo.
* Số frame được phân tích.
* Tốc độ xử lý so với realtime.

Không sửa code trước khi xác định được bottleneck chính.

## Bước 2: Tìm nguyên nhân chậm

Kiểm tra kỹ các vấn đề sau:

* Có decode toàn bộ video nhiều lần không.
* Có phân tích mọi frame không.
* Có phân tích video ở độ phân giải gốc không.
* Có chạy các bước độc lập theo tuần tự không.
* Có tạo một FFmpeg process cho từng thao tác nhỏ không.
* Có encode lại clip dù chỉ cần cắt không.
* Có đọc lại cùng một đoạn video nhiều lần không.
* Có ghi file tạm quá nhiều không.
* Có copy dữ liệu lớn trong RAM không.
* Có vòng lặp xử lý frame không cần thiết không.
* Có log quá nhiều trong vòng lặp không.
* Có chờ process theo kiểu blocking không.
* Có giới hạn worker chưa hợp lý không.
* Có dùng preset encode quá chậm không.
* Có sử dụng GPU sai cách khiến phải chuyển dữ liệu CPU–GPU nhiều lần không.

Sau khi kiểm tra, lập bảng:

| Bottleneck | File liên quan | Nguyên nhân | Mức ảnh hưởng | Cách tối ưu |
| ---------- | -------------- | ----------- | ------------- | ----------- |

## Bước 3: Tối ưu theo thứ tự ưu tiên

### Ưu tiên 1: Giảm số lần decode

* Không decode lại cùng một video hoặc cùng một đoạn video nhiều lần.
* Tái sử dụng metadata và dữ liệu phân tích.
* Gộp các thao tác FFmpeg nếu có thể.
* Tránh mở lại file nguồn cho từng bước nhỏ.
* Nếu hình ảnh và âm thanh đang được phân tích riêng bằng nhiều lần đọc nguồn, xem xét dùng chung một luồng đọc hoặc tạo proxy một lần.

### Ưu tiên 2: Giảm lượng frame cần phân tích

* Không phân tích tất cả frame nếu thuật toán không yêu cầu.
* Sử dụng frame sampling hợp lý.
* Có thể phân tích thưa trước, sau đó chỉ phân tích chi tiết quanh vùng nghi ngờ có điểm cắt.
* Sử dụng proxy 360p hoặc 480p cho bước phát hiện.
* Chỉ dùng video gốc khi cần tinh chỉnh chính xác timestamp.

Không giảm sampling quá mức làm bỏ sót clip.

### Ưu tiên 3: Cắt không encode lại

Nếu clip không có chỉnh sửa hình ảnh:

* Ưu tiên FFmpeg stream copy bằng `-c copy`.
* Không render lại toàn bộ clip.
* Nếu cần chính xác từng frame, chỉ re-encode đoạn biên cần thiết.
* Không re-encode toàn bộ video nguồn chỉ để tạo các clip con.

Nếu tool có bước chỉnh sửa bắt buộc:

* Chỉ encode các clip đầu ra.
* Không encode lại các dữ liệu trung gian không cần thiết.
* Chọn preset encode nhanh phù hợp với yêu cầu chất lượng hiện tại.

### Ưu tiên 4: Xử lý song song có giới hạn

Kiểm tra các bước có thể chạy song song:

* Phân tích hình ảnh.
* Phân tích âm thanh.
* Xuất các clip độc lập.
* Xử lý hậu kỳ từng clip.

Yêu cầu:

* Không tạo worker không giới hạn.
* Số worker phải dựa trên CPU, RAM, GPU và tốc độ ổ đĩa.
* Có hàng đợi xử lý.
* Tránh nhiều FFmpeg process cùng tranh chấp CPU và ổ đĩa.
* Benchmark nhiều mức worker để chọn cấu hình nhanh nhất.
* Không mặc định số worker càng nhiều càng tốt.

### Ưu tiên 5: Tối ưu FFmpeg

Rà soát toàn bộ command FFmpeg hiện tại:

* Loại bỏ filter không cần thiết.
* Loại bỏ encode trung gian không cần thiết.
* Sử dụng `-threads` hợp lý.
* Sử dụng preset phù hợp.
* Giảm log FFmpeg nếu log đang ảnh hưởng hiệu năng.
* Sử dụng `-nostdin` trong xử lý nền nếu phù hợp.
* Hạn chế tạo file tạm.
* Ưu tiên pipe khi hiệu quả hơn file trung gian.
* Không dùng pipe nếu làm tăng RAM hoặc gây deadlock.
* Kiểm tra timestamp, keyframe và seek để chọn cách cắt nhanh nhất.

### Ưu tiên 6: Tận dụng GPU

Nếu máy có GPU, kiểm tra khả năng dùng:

* NVIDIA NVDEC/NVENC.
* Intel Quick Sync.
* AMD AMF.
* Apple VideoToolbox.

Yêu cầu:

* Tự động phát hiện GPU.
* Có fallback CPU.
* Chỉ dùng GPU khi benchmark thực tế nhanh hơn.
* Không ép GPU nếu video ngắn hoặc dữ liệu truyền CPU–GPU làm chậm hơn.
* Không làm thay đổi chất lượng đầu ra ngoài ngưỡng cho phép.

### Ưu tiên 7: Cache

Nếu người dùng chạy lại cùng một video:

* Tái sử dụng metadata.
* Tái sử dụng proxy.
* Tái sử dụng kết quả phân tích hình ảnh.
* Tái sử dụng kết quả phân tích âm thanh.
* Tái sử dụng danh sách điểm cắt.

Cache phải tự hết hiệu lực khi file nguồn thay đổi.

### Ưu tiên 8: Tối ưu code

Kiểm tra:

* Vòng lặp xử lý frame.
* Copy mảng hoặc buffer không cần thiết.
* Cấp phát bộ nhớ liên tục.
* Chuyển đổi format nhiều lần.
* Serialize/deserialize dữ liệu trung gian.
* Ghi log trong vòng lặp.
* Polling process quá dày.
* Chờ tuần tự trong khi có thể dùng async.
* Đọc và ghi file bằng buffer quá nhỏ.
* Giữ toàn bộ frame trong RAM khi có thể xử lý streaming.

## Bước 4: Thực hiện sửa code

Sau khi xác định bottleneck:

1. Nêu rõ file cần sửa.
2. Nêu nguyên nhân chậm.
3. Nêu giải pháp.
4. Ước lượng mức cải thiện.
5. Thực hiện sửa.
6. Chạy test chức năng.
7. Chạy benchmark lại.
8. So sánh trước và sau.

Không được chỉ đưa đề xuất. Hãy trực tiếp sửa code trong repository.

## Bước 5: Kiểm tra không làm sai chức năng

Sau mỗi tối ưu, kiểm tra:

* Số clip phát hiện không thay đổi bất thường.
* Timestamp đầu và cuối không lệch quá ngưỡng cho phép.
* Không mất đầu hoặc cuối clip.
* Không sinh clip rỗng.
* Không sinh clip quá ngắn do lỗi.
* Âm thanh và hình ảnh không lệch nhau.
* Không làm hỏng codec hoặc container.
* Tên file và cấu trúc thư mục đầu ra vẫn đúng.
* Chức năng chỉnh sửa video hiện tại vẫn hoạt động.
* Giao diện và API hiện tại không bị thay đổi.

## Benchmark bắt buộc

Chạy ít nhất các trường hợp:

* Video 10 phút.
* Video 30 phút.
* Video 60 phút.
* Video có nhiều clip ngắn.
* Video có chuyển cảnh rõ.
* Video có fade hoặc chuyển cảnh nhẹ.
* Video độ phân giải 720p.
* Video độ phân giải 1080p.

Báo cáo theo mẫu:

| Video   | Trước tối ưu | Sau tối ưu | Mức cải thiện | Kết quả cắt |
| ------- | -----------: | ---------: | ------------: | ----------- |
| 10 phút |              |            |               |             |
| 30 phút |              |            |               |             |
| 60 phút |              |            |               |             |

Và báo cáo chi tiết:

| Công đoạn          | Trước | Sau | Cải thiện |
| ------------------ | ----: | --: | --------: |
| Decode             |       |     |           |
| Phân tích hình ảnh |       |     |           |
| Phân tích âm thanh |       |     |           |
| Phát hiện điểm cắt |       |     |           |
| Xuất clip          |       |     |           |
| Tổng               |       |     |           |

## Tiêu chí hoàn thành

Chỉ được coi là hoàn thành khi:

* Đã xác định rõ bottleneck.
* Đã sửa code thực tế.
* Test chức năng hiện tại vẫn pass.
* Kết quả cắt không bị sai đáng kể.
* Có benchmark trước và sau.
* Video 60 phút tiến gần hoặc đạt mục tiêu dưới 10 phút.
* Không thêm tính năng mới.
* Không thay đổi giao diện.
* Không thay đổi nghiệp vụ.
* Không để lại code thử nghiệm, log debug hoặc file tạm.

Nếu chưa đạt video 60 phút dưới 10 phút, phải chỉ ra chính xác:

* Công đoạn còn chậm.
* Giới hạn nằm ở CPU, GPU, RAM, ổ đĩa hay thuật toán.
* Mức thời gian thấp nhất đã đạt được.
* Các tối ưu tiếp theo có thể thực hiện.
* Rủi ro nếu tiếp tục giảm thời gian.

Bắt đầu bằng việc đọc source code, chạy benchmark hiện trạng và báo cáo bottleneck trước khi sửa.
