package browserai

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"video-splitter/internal/exporter"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type QueueTaskState string

const (
	QueueStatePending    QueueTaskState = "pending"
	QueueStateProcessing QueueTaskState = "processing"
	QueueStateCompleted  QueueTaskState = "completed"
	QueueStateFailed     QueueTaskState = "failed"
	QueueStateCancelled  QueueTaskState = "cancelled"
)

type ThumbnailTask struct {
	ID             string         `json:"id"`
	ClipName       string         `json:"clipName"`
	ClipPath       string         `json:"clipPath"`
	OutputDir      string         `json:"outputDir"`
	FileName       string         `json:"fileName"`
	Prompt         string         `json:"prompt"`
	InputImagePath string         `json:"inputImagePath"`
	// InputImagePaths: nhiều ảnh đầu vào cho 1 prompt (tối đa ~3). Nếu rỗng thì
	// fallback dùng InputImagePath (1 ảnh) để tương thích task cũ từ bên cắt video.
	InputImagePaths []string      `json:"inputImagePaths"`
	// InputImageBase64s: ảnh dán từ clipboard (base64, chưa có file trên đĩa).
	// Worker sẽ decode ra file tạm trước khi dán vào Flow.
	InputImageBase64s []string    `json:"inputImageBase64s"`
	Provider       Provider       `json:"provider"`
	Model          string         `json:"model"`
	AspectRatio    string         `json:"aspectRatio"`
	BatchSize      string         `json:"batchSize"`
	Resolution     string         `json:"resolution"`
	State          QueueTaskState `json:"state"`
	ErrorMessage   string         `json:"errorMessage"`
	ResultPath     string         `json:"resultPath"`
	ResultPaths    []string       `json:"resultPaths"`
	// Source: nguồn tạo task, dùng để lập lịch luân phiên công bằng giữa 2 nguồn.
	// "video-cut" = thumbnail tự sinh từ luồng cắt video; "ai-image" = tạo ảnh AI
	// độc lập ở trang Tạo Ảnh. Rỗng → coi như "ai-image" (tương thích task cũ).
	Source string `json:"source"`

	// === Luồng "tạo thumbnail trước → ghép vào đầu clip" (chỉ dùng cho video-cut) ===
	// PrependToVideo=true: sau khi thumbnail tạo xong (hoặc thất bại → dùng frame
	// gốc InputImagePath làm fallback), ghép ảnh đó thành đoạn intro tĩnh dài
	// IntroDuration giây rồi nối vào ĐẦU clip đã cắt (ClipPath) → ghi ra FinalVideoPath.
	PrependToVideo bool    `json:"prependToVideo"`
	IntroDuration  float64 `json:"introDuration"`
	// FinalVideoPath: đường dẫn video kết quả (intro + clip) mà người dùng nhận.
	// ClipPath lúc này trỏ tới file clip TẠM (đã cắt nhưng chưa ghép intro).
	FinalVideoPath string `json:"finalVideoPath"`
	// Thông số encode để ghép intro cho khớp với cấu hình xuất của người dùng.
	ExportPreset  string `json:"exportPreset"`
	ExportCRF     int    `json:"exportCRF"`
	ExportHWAccel string `json:"exportHWAccel"`
	HWAccel      string `json:"hwAccel"`

	// Hidden=true: task bị "ẩn" khỏi thống kê/hiển thị (đã kết thúc và bị dọn khi
	// nguồn cùng loại enqueue job mới, hoặc bị Dừng theo nguồn). KHÔNG xóa phần tử
	// khỏi slice qm.tasks để idx mà worker đang giữ không bị lệch — chỉ đánh dấu ẩn.
	Hidden bool `json:"hidden"`
}

const (
	SourceVideoCut = "video-cut"
	SourceAIImage  = "ai-image"
)

type QueueStatus struct {
	Total     int             `json:"total"`
	Completed int             `json:"completed"`
	Failed    int             `json:"failed"`
	Current   int             `json:"current"`
	IsRunning bool            `json:"isRunning"`
	Tasks     []ThumbnailTask `json:"tasks"`
}

type AIQueueManager struct {
	mu          sync.Mutex
	tasks       []ThumbnailTask
	ctx         context.Context
	runCtx      context.Context // ctx của lần chạy hàng đợi hiện tại (worker dùng)
	cancel      context.CancelFunc
	isRunning   bool
	activeWorkers int          // số worker (tab) đang sống — dùng để scale động
	nextWorkerID  int          // ID tăng dần để gắn nhãn log [W1]/[W2]... cho từng tab
	lastSource    string       // nguồn của task vừa claim — để luân phiên công bằng
	spawnMu     sync.Mutex     // serialize việc mở browser + sinh thêm worker
	service     *Service
	concurrency int // số worker (tab) chạy song song, kẹp trong [1, MaxBrowserConcurrency]
	// taskCancels: hàm hủy per-task theo task ID, để dừng ĐÚNG task đang chạy của
	// MỘT nguồn (video-cut / ai-image) mà không đụng task nguồn kia. Worker đăng ký
	// khi bắt đầu task và gỡ khi xong.
	taskCancels map[string]context.CancelFunc
}

func NewAIQueueManager(service *Service) *AIQueueManager {
	return &AIQueueManager{
		tasks:       make([]ThumbnailTask, 0),
		service:     service,
		concurrency: DefaultBrowserConcurrency,
		taskCancels: make(map[string]context.CancelFunc),
	}
}

func (qm *AIQueueManager) SetContext(ctx context.Context) {
	qm.mu.Lock()
	defer qm.mu.Unlock()
	qm.ctx = ctx
}

// SetConcurrency đặt số worker (tab) chạy song song cho Hàng Đợi AI.
// Giá trị được kẹp trong [1, MaxBrowserConcurrency]. Thay đổi chỉ áp dụng cho
// lần chạy hàng đợi kế tiếp (không tác động tới các worker đang chạy).
func (qm *AIQueueManager) SetConcurrency(n int) {
	if n < 1 {
		n = 1
	}
	if n > MaxBrowserConcurrency {
		n = MaxBrowserConcurrency
	}
	qm.mu.Lock()
	qm.concurrency = n
	qm.mu.Unlock()
}

func (qm *AIQueueManager) Enqueue(tasks []ThumbnailTask) {
	qm.mu.Lock()
	// Tự dọn job CŨ đã kết thúc trước khi nạp job mới: nếu hàng đợi KHÔNG còn
	// chạy (mọi task trước đã completed/failed/cancelled) thì các task cũ chỉ là
	// "rác lịch sử" — xóa sạch để lần xuất mới đếm lại từ 0, tránh cộng dồn lỗi
	// của lần trước. Nếu đang chạy (runCtx != nil) thì GỘP thêm để "vừa xuất vừa
	// gửi" — worker đang sống sẽ tự nhặt task mới.
	if qm.runCtx == nil {
		qm.tasks = qm.tasks[:0]
	}
	for _, t := range tasks {
		if t.State == "" {
			t.State = QueueStatePending
		}
		qm.tasks = append(qm.tasks, t)
	}
	qm.isRunning = true
	if qm.runCtx == nil {
		ctx, cancel := context.WithCancel(context.Background())
		qm.runCtx = ctx
		qm.cancel = cancel
	}
	qm.mu.Unlock()

	// Sinh thêm worker (tab) nếu còn thiếu so với số task chờ. Đây là mấu chốt cho
	// "vừa xuất vừa gửi": các clip enqueue dần dần, mỗi lần đều top-up để tab mới mở
	// ra chạy song song thay vì chỉ 1 tab đã chốt lúc task đầu tiên vào.
	qm.topUpWorkers()
}

// topUpWorkers mở browser (nếu chưa) rồi sinh thêm worker cho tới khi số worker
// đang sống = min(concurrency, activeWorkers + pending). Gọi mỗi lần Enqueue.
// spawnMu tuần tự hóa việc mở browser + quyết định sinh worker để tránh mở dư.
func (qm *AIQueueManager) topUpWorkers() {
	qm.spawnMu.Lock()
	defer qm.spawnMu.Unlock()

	cfg := LoadGlobalSettingsConfig()
	if !qm.service.session.IsOpen() || qm.service.session.IsHeadless() != !cfg.BrowserAIShowChrome {
		if err := qm.service.OpenGoogleAI(string(ProviderFlow), cfg.BrowserAIShowChrome); err != nil {
			qm.mu.Lock()
			for i := range qm.tasks {
				if qm.tasks[i].State == QueueStatePending || qm.tasks[i].State == QueueStateProcessing {
					qm.tasks[i].State = QueueStateFailed
					qm.tasks[i].ErrorMessage = err.Error()
				}
			}
			qm.isRunning = false
			qm.runCtx = nil
			qm.cancel = nil
			qm.mu.Unlock()
			qm.emitProgress()
			return
		}
	}

	qm.mu.Lock()
	pending := 0
	for i := range qm.tasks {
		if qm.tasks[i].State == QueueStatePending {
			pending++
		}
	}
	target := qm.concurrency
	if target > qm.activeWorkers+pending {
		target = qm.activeWorkers + pending
	}
	toSpawn := target - qm.activeWorkers
	ctx := qm.runCtx
	spawned := 0
	for k := 0; k < toSpawn && ctx != nil; k++ {
		qm.activeWorkers++
		wid := qm.nextWorkerID
		qm.nextWorkerID++
		spawned++
		go qm.worker(ctx, wid)
	}
	total := qm.activeWorkers
	qm.mu.Unlock()

	if spawned > 0 {
		flowLogf("Hàng Đợi AI: %d task chờ, sinh thêm %d tab (tổng %d tab song song).", pending, spawned, total)
	}
}

func (qm *AIQueueManager) Cancel() {
	qm.mu.Lock()
	if qm.cancel != nil {
		qm.cancel()
	}
	qm.isRunning = false
	// Xóa run-state để lần Enqueue sau tạo run mới. Worker cũ đang chạy sẽ thấy
	// ctx.Err() != nil nên tự thoát, và guard runCtx==ctx ở retireWorker chặn
	// chúng nil nhầm run-state mới.
	qm.runCtx = nil
	qm.cancel = nil
	// Người dùng bấm Dừng → XÓA SẠCH task luôn (theo yêu cầu), không giữ lại task
	// Cancelled lơ lửng để lần xuất/tạo sau bắt đầu hàng đợi trống hoàn toàn.
	qm.tasks = make([]ThumbnailTask, 0)
	qm.mu.Unlock()
	qm.emitProgress()
}

// CancelSource dừng và ẩn CHỈ các task của một nguồn (video-cut / ai-image), giữ
// nguyên nguồn kia. Dùng cho nút Dừng của mỗi trang: bấm Dừng ở trang Tạo Ảnh AI
// không được giết luồng thumbnail đang chạy từ cắt video, và ngược lại.
//
// KHÔNG cancel runCtx (đó là ctx chung của mọi worker) — chỉ cancel per-task đang
// chạy của nguồn này qua taskCancels, và đánh Hidden+Cancelled cho task pending của
// nguồn này để claimNextTask bỏ qua và GetStatus không còn đếm. Worker vẫn sống,
// tự claim task nguồn kia; nếu hết sạch task thì tự thoát qua retireWorker.
func (qm *AIQueueManager) CancelSource(source string) {
	qm.mu.Lock()
	for i := range qm.tasks {
		if qm.tasks[i].Source != source {
			continue
		}
		// Task đang chạy của nguồn này → hủy ctx của đúng task đó để nó dừng ngay.
		if qm.tasks[i].State == QueueStateProcessing {
			if c, ok := qm.taskCancels[qm.tasks[i].ID]; ok && c != nil {
				c()
			}
		}
		// Ẩn khỏi hàng đợi (pending lẫn processing) để không claim lại/không đếm nữa.
		if qm.tasks[i].State == QueueStatePending || qm.tasks[i].State == QueueStateProcessing {
			qm.tasks[i].State = QueueStateCancelled
			qm.tasks[i].Hidden = true
		}
	}
	qm.mu.Unlock()
	qm.emitProgress()
}

func (qm *AIQueueManager) Clear() {
	qm.mu.Lock()
	if qm.cancel != nil {
		qm.cancel()
	}
	qm.isRunning = false
	qm.runCtx = nil
	qm.cancel = nil
	qm.tasks = make([]ThumbnailTask, 0)
	qm.mu.Unlock()
	qm.emitProgress()
}

func (qm *AIQueueManager) GetStatus() QueueStatus {
	qm.mu.Lock()
	defer qm.mu.Unlock()

	// Bỏ qua task Hidden (đã bị CancelSource ẩn đi) khỏi cả số đếm lẫn mảng Tasks.
	// Frontend mỗi trang sẽ TỰ lọc Tasks theo Source của mình rồi tự tính số đếm —
	// nên backend chỉ cần loại rác Hidden ra, không cần tách theo nguồn ở đây.
	total := 0
	completed := 0
	failed := 0
	current := 0

	tasksCopy := make([]ThumbnailTask, 0, len(qm.tasks))
	for _, t := range qm.tasks {
		if t.Hidden {
			continue
		}
		tasksCopy = append(tasksCopy, t)
		total++
		if t.State == QueueStateCompleted {
			completed++
		} else if t.State == QueueStateFailed {
			failed++
		} else if t.State == QueueStateProcessing {
			current = total
		}
	}

	return QueueStatus{
		Total:     total,
		Completed: completed,
		Failed:    failed,
		Current:   current,
		IsRunning: qm.isRunning,
		Tasks:     tasksCopy,
	}
}

func (qm *AIQueueManager) emitProgress() {
	status := qm.GetStatus()
	qm.mu.Lock()
	ctx := qm.ctx
	qm.mu.Unlock()

	if ctx != nil {
		runtime.EventsEmit(ctx, "browser-ai:queue-progress", status)
	}
}

// claimNextTask lấy task pending kế tiếp, đánh dấu Processing và trả về (bản sao, chỉ số).
// Trả về idx = -1 nếu không còn task nào. An toàn khi gọi từ nhiều worker.
//
// LUÂN PHIÊN CÔNG BẰNG theo nguồn (Source): ưu tiên task có nguồn KHÁC với task vừa
// claim gần nhất. Nhờ vậy khi cắt video nạp 50 task rồi tạo ảnh AI, ảnh AI không phải
// đợi hết 50 mà chen vào xen kẽ (video, ảnh, video, ảnh...). Nếu chỉ còn 1 nguồn thì
// chạy tuần tự nguồn đó — không lãng phí luồng.
func (qm *AIQueueManager) claimNextTask() (ThumbnailTask, int) {
	qm.mu.Lock()
	defer qm.mu.Unlock()

	firstPending := -1
	altPending := -1 // task pending đầu tiên có nguồn KHÁC lastSource
	for i := range qm.tasks {
		if qm.tasks[i].State != QueueStatePending || qm.tasks[i].Hidden {
			continue
		}
		if firstPending == -1 {
			firstPending = i
		}
		if qm.tasks[i].Source != qm.lastSource {
			altPending = i
			break
		}
	}

	pick := altPending
	if pick == -1 {
		pick = firstPending
	}
	if pick == -1 {
		return ThumbnailTask{}, -1
	}

	qm.tasks[pick].State = QueueStateProcessing
	qm.lastSource = qm.tasks[pick].Source
	return qm.tasks[pick], pick
}

// retireWorker giảm activeWorkers khi một worker thoát. Nếu vẫn còn task pending
// (trường hợp hiếm: worker thoát đúng lúc task mới vừa vào) thì respawn để không
// bỏ sót. Khi worker cuối cùng thoát và không còn pending → reset run-state.
func (qm *AIQueueManager) retireWorker(ctx context.Context, workerID int) {
	qm.mu.Lock()
	qm.activeWorkers--
	pending := 0
	for i := range qm.tasks {
		if qm.tasks[i].State == QueueStatePending {
			pending++
		}
	}
	// Còn task chờ nhưng worker này sắp chết → hồi sinh chính nó (chống orphan).
	if pending > 0 && ctx.Err() == nil {
		qm.activeWorkers++
		qm.mu.Unlock()
		go qm.worker(ctx, workerID)
		return
	}
	// Worker cuối cùng thoát → dọn run-state để lần Enqueue sau khởi động lại sạch.
	// Guard qm.runCtx == ctx: nếu Cancel/Clear đã tạo (hoặc xóa) một run mới thì
	// worker cũ này KHÔNG được nil nhầm run-state mới.
	//
	// CỐ Ý GIỮ TRÌNH DUYỆT SỐNG (không Close session) khi hết worker: mỗi worker đã
	// tự đóng cửa sổ riêng của nó, chỉ còn lại tab gốc (trang Flow mở kèm lúc
	// OpenGoogleAI). Tab gốc này chính là "keep-alive" giữ browser sống để lần chạy
	// kế tiếp topUpWorkers thấy IsOpen()==true nên KHÔNG phải khởi động lại Chrome +
	// nạp lại profile/đăng nhập (rất chậm). Đổi lại chỉ tốn 1 tab nền im lặng.
	if qm.activeWorkers == 0 && qm.runCtx == ctx {
		qm.isRunning = false
		qm.runCtx = nil
		qm.cancel = nil
	}
	qm.mu.Unlock()
	qm.emitProgress()
}

// worker mở một tab riêng và xử lý các task cho tới khi hết hoặc ctx bị hủy.
func (qm *AIQueueManager) worker(ctx context.Context, workerID int) {
	flowURL := "https://labs.google/fx/vi/tools/flow"
	page, err := qm.service.session.NewPage(flowURL)
	if err != nil {
		flowLogf("Worker #%d: không mở được tab mới: %v", workerID, err)
		qm.retireWorker(ctx, workerID)
		return
	}
	defer func() {
		if page != nil {
			_ = page.Close()
		}
		qm.retireWorker(ctx, workerID)
	}()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		task, idx := qm.claimNextTask()
		if idx == -1 {
			return // hết task
		}

		qm.emitProgress()

		provider := task.Provider
		if provider == "" {
			provider = ProviderFlow
		}
		model := task.Model
		if model == "" {
			model = "Nano Banana 2"
		}
		resolution := task.Resolution
		if resolution == "" {
			resolution = "1K"
		}
		batchSize := task.BatchSize
		if batchSize == "" {
			batchSize = "1x"
		}

		cfg := LoadGlobalSettingsConfig()

		// Ưu tiên danh sách nhiều ảnh (tối đa 3/prompt); fallback về ảnh đơn cũ.
		inputImgPaths := []string{}
		if len(task.InputImagePaths) > 0 {
			inputImgPaths = append(inputImgPaths, task.InputImagePaths...)
		} else if task.InputImagePath != "" {
			inputImgPaths = append(inputImgPaths, task.InputImagePath)
		}

		// Ảnh dán từ clipboard (base64, không có đường dẫn file) → giải mã ra file
		// tạm để dán được. Worker gọi thẳng GenerateFlowVideo nên không đi qua bước
		// decode ở service.Generate; phải tự decode ở đây. Cleanup gọi tường minh sau
		// khi task xong (không dùng defer vì đây là vòng lặp — file sẽ tích tụ).
		var cleanupTemp func()
		if len(task.InputImageBase64s) > 0 {
			decodedPaths, cleanup := decodeBase64ImagesToTemp(task.ID, task.InputImageBase64s)
			cleanupTemp = cleanup
			inputImgPaths = append(inputImgPaths, decodedPaths...)
		}

		firstInputPath := ""
		if len(inputImgPaths) > 0 {
			firstInputPath = inputImgPaths[0]
		}

		genReq := GenerateRequest{
			Provider:            provider,
			MediaType:           MediaTypeImage,
			Prompt:              task.Prompt,
			Model:               model,
			AspectRatio:         task.AspectRatio,
			BatchSize:           batchSize,
			ConfirmBeforeCreate: "auto",
			OutputDir:           task.OutputDir,
			FileName:            task.FileName,
			InputImagePath:      firstInputPath,
			InputImagePaths:     inputImgPaths,
			Resolution:          resolution,
			TimeoutSecond:       180,
			ShowChrome:          cfg.BrowserAIShowChrome,
			LogPrefix:           fmt.Sprintf("[W%d %s] ", workerID+1, task.ClipName),
		}

		// emitLog phát 1 dòng log NGẮN GỌN ra card log ở giao diện (kèm nhãn W# + clip).
		// level: "info" | "success" | "error" để frontend tô màu.
		taskSource := task.Source
		if taskSource == "" {
			taskSource = SourceAIImage
		}
		emitLog := func(level, step string) {
			if qm.ctx != nil {
				runtime.EventsEmit(qm.ctx, "browser-ai:queue-log", map[string]interface{}{
					"worker": workerID + 1,
					"name":   task.ClipName,
					"level":  level,
					"step":   step,
					"source": taskSource,
				})
			}
		}
		emitLog("info", "Bắt đầu")

		// Mỗi task chạy trong timeout riêng để một task treo không chặn worker mãi.
		taskCtx, cancel := context.WithTimeout(ctx, ImageGenerateTimeout)
		// Đăng ký cancel theo task ID để CancelSource dừng đúng task đang chạy của
		// một nguồn (video-cut / ai-image) mà không đụng nguồn kia.
		qm.mu.Lock()
		qm.taskCancels[task.ID] = cancel
		qm.mu.Unlock()
		filePaths, genErr := GenerateFlowVideo(taskCtx, qm.service.session, page, nil, genReq, func(step string) {
			emitLog("info", step)
		})
		cancel()
		qm.mu.Lock()
		delete(qm.taskCancels, task.ID)
		qm.mu.Unlock()
		if cleanupTemp != nil {
			cleanupTemp()
		}
		// Dọn file frame tạm do luồng xuất video tự sinh (clearframe_*.jpg trong
		// TempDir) sau khi đã dùng xong. Chỉ xóa đúng file khớp mẫu này để KHÔNG
		// đụng ảnh người dùng tự chọn ở trang Tạo Ảnh AI (những task đó không mang
		// InputImagePath dạng clearframe).
		// HOÃN với task PrependToVideo: nhánh fallback (AI lỗi) cần chính frame gốc
		// này làm ảnh intro, nên chỉ xóa SAU khi ghép xong (xử lý trong mergeIntroForTask).
		if !task.PrependToVideo && task.InputImagePath != "" &&
			strings.HasPrefix(filepath.Base(task.InputImagePath), "clearframe_") &&
			strings.Contains(filepath.ToSlash(task.InputImagePath), "/video-splitter/") {
			_ = os.Remove(task.InputImagePath)
		}

		// Với task PrependToVideo: AI lỗi KHÔNG phải là hỏng hẳn — ta fallback dùng
		// chính frame gốc (InputImagePath) làm ảnh intro rồi vẫn ghép vào đầu clip.
		// Nên xử lý ghép NGOÀI khóa (ffmpeg chạy lâu), rồi mới cập nhật state.
		if task.PrependToVideo {
			introImg := ""
			usedFallback := false
			if genErr == nil && len(filePaths) > 0 {
				introImg = filePaths[0]
			} else {
				// AI thất bại → dùng frame gốc làm ảnh bìa intro.
				introImg = task.InputImagePath
				usedFallback = true
				emitLog("info", "AI tạo ảnh lỗi → dùng frame gốc làm ảnh bìa")
			}

			mergeErr := qm.mergeIntroForTask(ctx, task, introImg, func(step string) { emitLog("info", step) })

			// Dọn frame gốc tạm sau khi ghép xong (đã hoãn ở trên cho task này).
			if task.InputImagePath != "" &&
				strings.HasPrefix(filepath.Base(task.InputImagePath), "clearframe_") &&
				strings.Contains(filepath.ToSlash(task.InputImagePath), "/video-splitter/") {
				_ = os.Remove(task.InputImagePath)
			}
			// KHÔNG xóa ảnh thumbnail AI: nó được lưu vào outImageDir (thư mục người
			// dùng) và chính là ảnh bìa họ muốn giữ song song với video đã ghép intro.

			qm.mu.Lock()
			if mergeErr != nil {
				qm.tasks[idx].State = QueueStateFailed
				qm.tasks[idx].ErrorMessage = mergeErr.Error()
				failedTask := qm.tasks[idx]
				flowLogf("[W%d %s] Ghép intro vào clip thất bại: %v", workerID+1, task.ClipName, mergeErr)
				emitLog("error", "Ghép intro lỗi: "+mergeErr.Error())
				qm.mu.Unlock()
				if qm.ctx != nil {
					runtime.EventsEmit(qm.ctx, "export_log", fmt.Sprintf("⚠ %s: Ghép ảnh bìa vào đầu video thất bại (%v).", task.ClipName, mergeErr))
					runtime.EventsEmit(qm.ctx, "clip_ai_thumb_failed", failedTask)
				}
			} else {
				qm.tasks[idx].State = QueueStateCompleted
				qm.tasks[idx].ResultPath = task.FinalVideoPath
				qm.tasks[idx].ResultPaths = []string{task.FinalVideoPath}
				completedTask := qm.tasks[idx]
				qm.mu.Unlock()
				if usedFallback {
					emitLog("success", "Đã ghép ảnh bìa (frame gốc) vào đầu video")
				} else {
					emitLog("success", "Đã ghép ảnh bìa AI vào đầu video")
				}
				if qm.ctx != nil {
					runtime.EventsEmit(qm.ctx, "clip_ai_thumb_completed", completedTask)
				}
			}
			qm.emitProgress()
			continue
		}

		qm.mu.Lock()
		if genErr != nil {
			qm.tasks[idx].State = QueueStateFailed
			qm.tasks[idx].ErrorMessage = genErr.Error()
			failedTask := qm.tasks[idx]
			flowLogf("[W%d %s] KHÔNG tạo được thumbnail: %v", workerID+1, task.ClipName, genErr)
			emitLog("error", "Thất bại: "+genErr.Error())
			// Cảnh báo người dùng: clip này KHÔNG còn ảnh dự phòng (đã bỏ frame gốc).
			if qm.ctx != nil {
				runtime.EventsEmit(qm.ctx, "export_log", fmt.Sprintf("⚠ %s: KHÔNG tạo được ảnh thumbnail AI (%v). Clip này hiện chưa có ảnh bìa.", task.ClipName, genErr))
				runtime.EventsEmit(qm.ctx, "clip_ai_thumb_failed", failedTask)
			}
		} else {
			qm.tasks[idx].State = QueueStateCompleted
			qm.tasks[idx].ResultPaths = filePaths
			if len(filePaths) > 0 {
				qm.tasks[idx].ResultPath = filePaths[0]
			}
			completedTask := qm.tasks[idx]
			emitLog("success", "Đã tạo xong ảnh")
			if qm.ctx != nil {
				runtime.EventsEmit(qm.ctx, "clip_ai_thumb_completed", completedTask)
			}
		}
		qm.mu.Unlock()

		qm.emitProgress()
	}
}

// mergeIntroForTask dựng đoạn intro từ ảnh (thumbnail AI hoặc frame gốc) rồi ghép
// vào ĐẦU clip đã cắt (task.ClipPath) → ghi ra task.FinalVideoPath. Clip tạm bị
// xóa sau khi ghép thành công. Chạy NGOÀI khóa qm.mu vì ffmpeg re-encode chậm.
func (qm *AIQueueManager) mergeIntroForTask(ctx context.Context, task ThumbnailTask, introImg string, milestone func(string)) error {
	if introImg == "" {
		return fmt.Errorf("không có ảnh bìa để ghép intro")
	}
	if _, err := os.Stat(introImg); err != nil {
		return fmt.Errorf("ảnh bìa không tồn tại: %w", err)
	}
	if task.ClipPath == "" {
		return fmt.Errorf("thiếu đường dẫn clip đã cắt để ghép")
	}
	if _, err := os.Stat(task.ClipPath); err != nil {
		return fmt.Errorf("clip đã cắt không tồn tại: %w", err)
	}
	if task.FinalVideoPath == "" {
		return fmt.Errorf("thiếu đường dẫn video kết quả")
	}

	introDur := task.IntroDuration
	if introDur <= 0 {
		introDur = 2.0
	}

	if milestone != nil {
		milestone(fmt.Sprintf("Đang ghép ảnh bìa (%.1fs) vào đầu video...", introDur))
	}

	if err := exporter.PrependThumbnailIntro(ctx, task.ClipPath, introImg, task.FinalVideoPath, introDur, task.ExportPreset, task.ExportCRF, task.ExportHWAccel); err != nil {
		return err
	}

	// Ghép xong → xóa clip tạm (chỉ xóa file khớp mẫu clip tạm để tránh xóa nhầm).
	if strings.Contains(filepath.ToSlash(task.ClipPath), "/video-splitter/") &&
		strings.Contains(filepath.Base(task.ClipPath), "cliptmp_") {
		_ = os.Remove(task.ClipPath)
	}
	return nil
}
