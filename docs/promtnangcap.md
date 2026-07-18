Kết luận

Code này chưa ổn để đạt mục tiêu “nhanh và chính xác như CapCut”. Nó có ý tưởng tốt, nhưng hiện tại vẫn là pipeline Python + FFmpeg + OpenCV + Librosa, chưa có GStreamer, D3D11, NVDEC, CUDA hay hardware decode thực sự.

Đánh giá tổng thể:

Cấu trúc code: 7/10
Khả năng chạy thử: 7/10
Hiệu năng video dài: 4/10
Độ chính xác Fast mode: 3/10
Khả năng production: 4/10

Tệp này cũng chỉ chứa phần phân tích điểm cắt, chưa có code xuất/cắt clip nên chưa thể đánh giá tốc độ cắt đầu ra.

Những phần làm đúng

Code đã có một số hướng đúng:

Chia thành fast, smart, precise.
Có proxy path và audio path riêng.
Có timeout cho FFmpeg.
Có multiprocessing.freeze_support() cho Windows.
Phase A chạy scene, black và silence song song.
Có bước gộp các tín hiệu gần nhau.
Smart mode có quét sơ bộ rồi tinh chỉnh.
Không xuất hàng nghìn frame thành file ảnh.

Đây là nền tảng dùng được, nhưng cách triển khai hiện tại vẫn tạo nhiều lần đọc và decode video.

Các vấn đề nghiêm trọng
1. Fast mode không hề “không decode frame”

Phần mô tả ghi:

Không decode frame nào

Nhưng _fast_black() chạy:

ffmpeg -i source -vf scale=320:180,blackdetect ...

FFmpeg vẫn phải decode toàn bộ video, sau đó mới scale và chạy blackdetect.

Đồng thời Fast mode chạy song song:

ffprobe quét tất cả packet
FFmpeg decode toàn bộ audio
FFmpeg decode toàn bộ video

Nghĩa là cùng lúc có ba process đọc cùng một file. Với HDD hoặc SSD chậm, điều này có thể làm tốc độ tệ hơn chạy tuần tự.

Dòng mô tả:

xong trong 5-20s bất kể độ dài video

là không thực tế. Video 1 giờ không thể bảo đảm thời gian giống video 5 phút.

2. Fast mode có độ chính xác rất thấp

Logic hiện tại là:

if not candidates_map and keyframes:
    dùng toàn bộ keyframes làm candidates

Điều này gây hai lỗi đối nghịch.

Trường hợp có silence hoặc black frame

Nếu video có dù chỉ một khoảng lặng, code không sử dụng keyframe nữa. Những điểm chuyển clip bình thường không có màn hình đen hoặc khoảng lặng sẽ bị bỏ sót.

Trường hợp không có silence và black frame

Code dùng toàn bộ keyframe làm điểm cắt. Nhưng keyframe không đồng nghĩa với clip mới. Encoder thường đặt keyframe định kỳ, ví dụ mỗi 1–5 giây.

Kết quả có thể là:

Video 10 phút
→ hàng trăm keyframe
→ hàng trăm điểm cắt giả

Vì vậy không nên ghi Fast mode có “sai số dưới 2 giây”. Vấn đề không chỉ là sai timestamp, mà còn có thể sai hoàn toàn số lượng clip.

3. Smart mode có thể vô tình quét video gốc toàn bộ

Đoạn này:

target = proxy_path if proxy_path else source_path

Nếu proxy bị thiếu, lỗi hoặc truyền path rỗng, OpenCV sẽ đọc trực tiếp video nguồn.

Khi đó:

while True:
    ok, frame = cap.read()

sẽ decode:

mọi frame;
ở FPS gốc;
ở độ phân giải gốc.

Video 10 phút, 30 FPS có khoảng 18.000 frame. Video 1 giờ có khoảng 108.000 frame. Với video 1080p, đây là nguyên nhân rất rõ khiến tool lâu.

Không nên âm thầm fallback từ proxy sang source trong Smart mode. Phải:

Proxy hợp lệ → chạy Smart
Proxy thiếu → tạo proxy
Tạo proxy lỗi → báo fallback rõ ràng
4. Pass 2 đang seek lại video quá nhiều lần

Đoạn này là bottleneck lớn:

for rough_ts in rough_candidates:
    cap3.set(cv2.CAP_PROP_POS_FRAMES, sf)

Mỗi candidate lại gọi seek một lần. Video nén không thể nhảy thẳng đến mọi frame; decoder thường phải tìm keyframe gần nhất rồi decode tiến tới vị trí cần thiết.

Nếu Pass 1 tạo 300 candidate:

300 lần seek
300 lần decoder quay lại vùng keyframe
300 vùng bị đọc lại

Fade hoặc chuyển động mạnh còn có thể tạo nhiều candidate liên tiếp trong cùng một vùng.

Bạn cần gộp candidate trước Pass 2, ví dụ:

12.00
12.25
12.50
12.75

phải thành một vùng:

12.00–12.75

rồi chỉ refine một lần.

Hiện tại code chỉ chống trùng sau khi refine:

abs(best_ts - refined[-1]) > 0.3

Lúc đó chi phí seek đã xảy ra rồi.

5. Layout detector tiếp tục mở và decode video lần nữa

Sau scene detection, code chạy:

results['layout'] = _task_layout(...)

analyze_layout() mở cv2.VideoCapture mới và lại seek quanh từng candidate.

Như vậy Smart mode có thể gồm:

Lần 1: OpenCV quét scene
Lần 2: OpenCV refine scene
Lần 3: OpenCV quét layout
Lần 4: FFmpeg quét black
Lần 5: FFmpeg quét audio

Dù dùng proxy, đây vẫn là nhiều lần mở và đọc dữ liệu không cần thiết.

Scene score, layout score và black-frame score nên được tính trong cùng một vòng lặp đọc frame.

6. Chạy song song chưa chắc nhanh hơn

Phase A chạy ba task:

black
silence
scenes

Nếu có proxy và WAV riêng thì có thể chấp nhận.

Nhưng nếu thiếu proxy hoặc audio:

black đọc video nguồn;
scenes cũng đọc video nguồn;
silence tiếp tục đọc video nguồn.

CPU, ổ đĩa và decoder cùng tranh chấp tài nguyên. Chạy ba task song song trong trường hợp này có thể chậm hơn chạy pipeline hợp nhất.

Ngoài ra, ProcessPoolExecutor trên Windows phải khởi động Python process mới, import module và có thể load OpenCV. Nếu đóng gói bằng PyInstaller thì chi phí còn cao hơn.

7. Precise mode có thể ngốn RAM rất lớn

Đoạn:

y, sr = librosa.load(audio_path, sr=16000, mono=True)

đọc toàn bộ audio vào RAM.

Audio 1 giờ ở 16 kHz có khoảng:

57,6 triệu sample

Chỉ mảng float32 đã khoảng 230 MB, chưa tính:

dữ liệu nguồn;
MFCC;
mảng chuẩn hóa;
diff;
overhead của Python và NumPy.

Precise mode có thể dùng hàng trăm MB đến hơn 1 GB RAM.

Cần chuyển sang:

xử lý audio theo chunk;
hoặc FFmpeg astats, silencedetect, ebur128;
hoặc streaming MFCC theo từng đoạn.
8. Logic confidence chưa đáng tin

Hiện tại confidence được cộng thẳng:

cand['confidence'] = min(100, cand['confidence'] + conf)

Ví dụ:

silence đơn lẻ = 30
scene đơn lẻ = 40
black frame đơn lẻ = 50

Nhưng cuối hàm không có:

confidence tối thiểu;
thời lượng clip tối thiểu;
lọc candidate sát đầu/cuối;
kiểm tra khoảng cách với điểm cắt trước;
quy tắc bắt buộc nhiều tín hiệu.

Vì vậy mọi khoảng lặng đều có thể trở thành điểm cắt. Một người dừng nói 0,5 giây cũng có thể bị hiểu là video mới.

9. Timestamp sau khi merge có thể bị sai

Ví dụ:

Silence start: 10.00s
Scene change: 10.35s

Do nằm trong MERGE_WINDOW = 0.4, hai điểm được gộp. Nhưng timestamp vẫn giữ 10.00, vì candidate đầu tiên không được cập nhật.

Kết quả tool có thể cắt trước cảnh thật 0,35 giây.

Nên ưu tiên:

hard cut timestamp
> black transition midpoint
> audio boundary
> silence timestamp

Hoặc tính weighted timestamp thay vì luôn giữ timestamp đầu tiên.

10. MERGE_WINDOW = 0.4 cố định chưa hợp lý

Proxy 4 FPS có khoảng cách frame là:

0,25 giây

Cửa sổ 0,4 giây khá sát. Fade kéo dài 0,5–1 giây có thể vẫn sinh nhiều candidate.

Nên dùng:

Hard cut: 0,3–0,5 giây
Fade: 0,8–1,5 giây
Audio/silence: 0,5–1 giây

Tức merge window phụ thuộc loại tín hiệu, không dùng một giá trị cho tất cả.

11. Khoảng lặng đang lấy sai vị trí cắt

Code chỉ lấy:

silence_start

Nhưng tùy trường hợp, điểm chuyển clip phù hợp có thể là:

silence_start;
silence_end;
trung điểm khoảng lặng;
scene cut gần nhất trong khoảng lặng.

Ví dụ người nói xong ở 10 giây, video mới bắt đầu ở 10,8 giây. Cắt tại silence_start có thể để phần im lặng ở đầu clip sau hoặc cắt mất đuôi clip trước.

Nên đọc cả:

silence_start
silence_end
silence_duration

rồi ghép với visual candidate.

12. Không có hardware decode thực tế

Trong toàn bộ file không có:

-hwaccel;
NVDEC;
D3D11;
DXVA;
CUDA;
GStreamer;
OpenCV hardware acceleration.

Vì vậy code này chưa phải bản nâng cấp công nghệ mà prompt trước đó yêu cầu. Nó vẫn sử dụng decode mặc định bằng CPU trong phần lớn trường hợp.

Nguyên nhân tool vẫn lâu

Theo code này, ba nguyên nhân lớn nhất là:

1. Video bị đọc và decode nhiều lần

Smart mode có thể decode qua:

scene pass 1
scene fallback
scene pass 2
layout
blackdetect
silence
2. Seek ngẫu nhiên cho từng candidate

CAP_PROP_POS_FRAMES được gọi lặp lại nhiều lần, đặc biệt nghiêm trọng khi có nhiều candidate.

3. Không bảo đảm proxy thật sự được sử dụng

Nếu proxy path trống, hệ thống âm thầm quét video nguồn full FPS/full resolution.

Kiến trúc nên sửa
Pipeline Smart hợp lý hơn
Tạo hoặc lấy proxy cache 320×180, 4 FPS
             ↓
Một lần đọc tuần tự proxy
             ↓
Tính đồng thời:
- histogram difference
- mean absolute difference
- black score
- layout/edge score
             ↓
Cluster candidate ngay lập tức
             ↓
Một lần phân tích audio
             ↓
Ghép audio và visual
             ↓
Chỉ refine các candidate đã cluster
             ↓
Lọc theo confidence + min clip duration

Như vậy proxy chỉ bị đọc một lần, thay vì ba hoặc bốn lần.

Những sửa đổi ưu tiên
P0 — Phải sửa ngay
Không cho Smart mode âm thầm dùng source nếu thiếu proxy.
Cluster rough_candidates trước Pass 2.
Gộp scene, layout và black detection vào một vòng đọc proxy.
Bổ sung min_clip_duration.
Bổ sung minimum_confidence.
Không dùng toàn bộ keyframe làm điểm cắt.
Không coi silence đơn lẻ là điểm cắt chắc chắn.
Đo thời gian từng detector.
P1 — Tối ưu mạnh
Tạo proxy bằng hardware decode.
Cache proxy theo hash của video.
Thay Librosa toàn-file bằng audio streaming.
Tinh chỉnh timestamp theo batch thay vì seek từng điểm.
Chọn timestamp visual làm mốc chính khi merge.
Thêm -hide_banner -nostdin -nostats cho FFmpeg.
P2 — Production
Worker chạy lâu dài thay vì khởi động Python mỗi lần.
Có cancel thực sự.
Có benchmark 10/30/60 phút.
Có bộ dữ liệu ground truth để đo precision/recall.
Tách analyzer ra khỏi protocol in log hiện tại.
Chốt

Code này là một prototype có cấu trúc tương đối tốt, nhưng chưa phải engine tốc độ cao. Smart mode có tiềm năng nhất, nhưng phải loại bỏ việc đọc lại proxy nhiều lần và seek theo từng candidate.

Fast mode hiện tại nên đổi tên thành:

heuristic mode

và không nên quảng cáo:

sai số dưới 2 giây
5–20 giây bất kể độ dài

vì logic hiện tại không bảo đảm được hai điều đó.