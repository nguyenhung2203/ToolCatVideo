package browserai

import (
	"context"
	"sync"
	"time"

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
	Provider       Provider       `json:"provider"`
	Model          string         `json:"model"`
	AspectRatio    string         `json:"aspectRatio"`
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
	mu        sync.Mutex
	tasks     []ThumbnailTask
	ctx       context.Context
	cancel    context.CancelFunc
	isRunning bool
	service   *Service
}

func NewAIQueueManager(service *Service) *AIQueueManager {
	return &AIQueueManager{
		tasks:   make([]ThumbnailTask, 0),
		service: service,
	}
}

func (qm *AIQueueManager) SetContext(ctx context.Context) {
	qm.mu.Lock()
	defer qm.mu.Unlock()
	qm.ctx = ctx
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
		go qm.processWorker(ctx)
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

func (qm *AIQueueManager) processWorker(ctx context.Context) {
	defer func() {
		qm.mu.Lock()
		qm.isRunning = false
		qm.mu.Unlock()
		qm.emitProgress()
	}()

	for {
		qm.mu.Lock()
		var nextIdx int = -1
		for i, t := range qm.tasks {
			if t.State == QueueStatePending {
				nextIdx = i
				break
			}
		}
		if nextIdx == -1 {
			qm.mu.Unlock()
			return
		}

		qm.tasks[nextIdx].State = QueueStateProcessing
		task := qm.tasks[nextIdx]
		qm.mu.Unlock()

		qm.emitProgress()

		provider := task.Provider
		if provider == "" {
			provider = ProviderFlow
		}
		model := task.Model
		if model == "" {
			model = "Nano Banana 2"
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
			InputImagePath:      task.InputImagePath,
			TimeoutSecond:       180,
			ShowChrome:          true,
		}

		taskInfo, err := qm.service.Generate(genReq)

		if err != nil {
			qm.mu.Lock()
			qm.tasks[nextIdx].State = QueueStateFailed
			qm.tasks[nextIdx].ErrorMessage = err.Error()
			qm.mu.Unlock()
		} else {
			active := qm.service.tm.GetActiveTask()
			if active != nil && active.ID == taskInfo.TaskID {
				select {
				case <-ctx.Done():
					_ = qm.service.tm.CancelTask(taskInfo.TaskID)
					return
				case <-active.DoneChan:
					qm.mu.Lock()
					if active.Err != nil {
						qm.tasks[nextIdx].State = QueueStateFailed
						qm.tasks[nextIdx].ErrorMessage = active.Err.Error()
					} else {
						qm.tasks[nextIdx].State = QueueStateCompleted
						if active.Result != nil && len(active.Result.FilePaths) > 0 {
							qm.tasks[nextIdx].ResultPath = active.Result.FilePaths[0]
						}
						if qm.ctx != nil {
							runtime.EventsEmit(qm.ctx, "clip_ai_thumb_completed", qm.tasks[nextIdx])
						}
					}
					qm.mu.Unlock()
				}
			} else {
				qm.mu.Lock()
				qm.tasks[nextIdx].State = QueueStateCompleted
				qm.mu.Unlock()
			}
		}

		qm.emitProgress()

		select {
		case <-ctx.Done():
			return
		case <-time.After(1 * time.Second):
		}
	}
}
