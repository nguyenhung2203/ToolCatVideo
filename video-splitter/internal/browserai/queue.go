package browserai

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

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
	Resolution     string         `json:"resolution"`
	State          QueueTaskState `json:"state"`
	ErrorMessage   string         `json:"errorMessage"`
	ResultPath     string         `json:"resultPath"`
}

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
	cancel      context.CancelFunc
	isRunning   bool
	service     *Service
	concurrency int // số worker (tab) chạy song song, kẹp trong [1, MaxBrowserConcurrency]
}

func NewAIQueueManager(service *Service) *AIQueueManager {
	return &AIQueueManager{
		tasks:       make([]ThumbnailTask, 0),
		service:     service,
		concurrency: DefaultBrowserConcurrency,
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
	defer qm.mu.Unlock()

	for _, t := range tasks {
		if t.State == "" {
			t.State = QueueStatePending
		}
		qm.tasks = append(qm.tasks, t)
	}

	if !qm.isRunning {
		qm.isRunning = true
		ctx, cancel := context.WithCancel(context.Background())
		qm.cancel = cancel
		n := qm.concurrency
		if n < 1 {
			n = 1
		}
		go qm.runWorkers(ctx, n)
	}
}

func (qm *AIQueueManager) Cancel() {
	qm.mu.Lock()
	if qm.cancel != nil {
		qm.cancel()
	}
	qm.isRunning = false
	for i := range qm.tasks {
		if qm.tasks[i].State == QueueStatePending || qm.tasks[i].State == QueueStateProcessing {
			qm.tasks[i].State = QueueStateCancelled
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
	qm.tasks = make([]ThumbnailTask, 0)
	qm.mu.Unlock()
	qm.emitProgress()
}

func (qm *AIQueueManager) GetStatus() QueueStatus {
	qm.mu.Lock()
	defer qm.mu.Unlock()

	total := len(qm.tasks)
	completed := 0
	failed := 0
	current := 0

	for i, t := range qm.tasks {
		if t.State == QueueStateCompleted {
			completed++
		} else if t.State == QueueStateFailed {
			failed++
		} else if t.State == QueueStateProcessing {
			current = i + 1
		}
	}

	tasksCopy := make([]ThumbnailTask, len(qm.tasks))
	copy(tasksCopy, qm.tasks)

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
func (qm *AIQueueManager) claimNextTask() (ThumbnailTask, int) {
	qm.mu.Lock()
	defer qm.mu.Unlock()
	for i := range qm.tasks {
		if qm.tasks[i].State == QueueStatePending {
			qm.tasks[i].State = QueueStateProcessing
			return qm.tasks[i], i
		}
	}
	return ThumbnailTask{}, -1
}

// runWorkers khởi chạy n worker song song, mỗi worker mở một tab Chrome riêng
// (chung profile) và xử lý task tới khi hết. Khi tất cả worker xong thì đặt lại
// isRunning=false và emit tiến độ cuối.
func (qm *AIQueueManager) runWorkers(ctx context.Context, n int) {
	defer func() {
		qm.mu.Lock()
		qm.isRunning = false
		qm.mu.Unlock()
		qm.emitProgress()
	}()

	// Không mở nhiều tab hơn số task đang chờ: 3 task + 5 luồng → chỉ mở 3 tab.
	qm.mu.Lock()
	pending := 0
	for i := range qm.tasks {
		if qm.tasks[i].State == QueueStatePending {
			pending++
		}
	}
	qm.mu.Unlock()
	if pending == 0 {
		return
	}
	if n > pending {
		n = pending
	}

	// Đảm bảo trình duyệt đã mở trước khi các worker tạo tab.
	if !qm.service.session.IsOpen() {
		if err := qm.service.OpenGoogleAI(string(ProviderFlow), true); err != nil {
			// Không mở được trình duyệt → đánh dấu mọi task pending là thất bại.
			qm.mu.Lock()
			for i := range qm.tasks {
				if qm.tasks[i].State == QueueStatePending || qm.tasks[i].State == QueueStateProcessing {
					qm.tasks[i].State = QueueStateFailed
					qm.tasks[i].ErrorMessage = err.Error()
				}
			}
			qm.mu.Unlock()
			return
		}
	}

	flowLogf("Hàng Đợi AI: %d task chờ, mở %d tab trình duyệt song song.", pending, n)

	var wg sync.WaitGroup
	for w := 0; w < n; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			qm.worker(ctx, workerID)
		}(w)
	}
	wg.Wait()
}

// worker mở một tab riêng và xử lý các task cho tới khi hết hoặc ctx bị hủy.
func (qm *AIQueueManager) worker(ctx context.Context, workerID int) {
	flowURL := "https://labs.google/fx/vi/tools/flow"
	page, err := qm.service.session.NewPage(flowURL)
	if err != nil {
		flowLogf("Worker #%d: không mở được tab mới: %v", workerID, err)
		return
	}
	defer func() {
		if page != nil {
			_ = page.Close()
		}
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
			BatchSize:           "1x",
			ConfirmBeforeCreate: "auto",
			OutputDir:           task.OutputDir,
			FileName:            task.FileName,
			InputImagePath:      firstInputPath,
			InputImagePaths:     inputImgPaths,
			Resolution:          resolution,
			TimeoutSecond:       180,
			ShowChrome:          true,
			LogPrefix:           fmt.Sprintf("[W%d %s] ", workerID+1, task.ClipName),
		}

		// Mỗi task chạy trong timeout riêng để một task treo không chặn worker mãi.
		taskCtx, cancel := context.WithTimeout(ctx, ImageGenerateTimeout)
		filePaths, genErr := GenerateFlowVideo(taskCtx, qm.service.session, page, nil, genReq)
		cancel()
		if cleanupTemp != nil {
			cleanupTemp()
		}
		// Dọn file frame tạm do luồng xuất video tự sinh (clearframe_*.jpg trong
		// TempDir) sau khi đã dùng xong. Chỉ xóa đúng file khớp mẫu này để KHÔNG
		// đụng ảnh người dùng tự chọn ở trang Tạo Ảnh AI (những task đó không mang
		// InputImagePath dạng clearframe).
		if task.InputImagePath != "" &&
			strings.HasPrefix(filepath.Base(task.InputImagePath), "clearframe_") &&
			strings.Contains(filepath.ToSlash(task.InputImagePath), "/video-splitter/") {
			_ = os.Remove(task.InputImagePath)
		}

		qm.mu.Lock()
		if genErr != nil {
			qm.tasks[idx].State = QueueStateFailed
			qm.tasks[idx].ErrorMessage = genErr.Error()
			flowLogf("Worker #%d: task %s thất bại: %v", workerID, task.ID, genErr)
		} else {
			qm.tasks[idx].State = QueueStateCompleted
			if len(filePaths) > 0 {
				qm.tasks[idx].ResultPath = filePaths[0]
			}
			completedTask := qm.tasks[idx]
			if qm.ctx != nil {
				runtime.EventsEmit(qm.ctx, "clip_ai_thumb_completed", completedTask)
			}
		}
		qm.mu.Unlock()

		qm.emitProgress()
	}
}
