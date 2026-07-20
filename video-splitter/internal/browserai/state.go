package browserai

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type TaskManager struct {
	mu         sync.Mutex
	ctx        context.Context // Wails app context
	activeTask *activeTask
}

type activeTask struct {
	ID            string
	Request       GenerateRequest
	State         TaskState
	Message       string
	Progress      int
	StartedAt     time.Time
	Cancel        context.CancelFunc
	SelectionChan chan []int // Receive selected image indexes from frontend
}

func NewTaskManager() *TaskManager {
	return &TaskManager{}
}

func (tm *TaskManager) SetContext(ctx context.Context) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.ctx = ctx
}

func (tm *TaskManager) StartTask(id string, req GenerateRequest, cancel context.CancelFunc) (*activeTask, error) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if tm.activeTask != nil {
		s := tm.activeTask.State
		if s != TaskStateCompleted && s != TaskStateFailed && s != TaskStateCancelled {
			return nil, NewError(ErrProfileLocked, "Đang có một tác vụ Google AI được xử lý.")
		}
	}

	task := &activeTask{
		ID:            id,
		Request:       req,
		State:         TaskStateIdle,
		Message:       "Bắt đầu tác vụ",
		Progress:      0,
		StartedAt:     time.Now(),
		Cancel:        cancel,
		SelectionChan: make(chan []int, 1),
	}
	tm.activeTask = task
	return task, nil
}

func (tm *TaskManager) GetActiveTask() *activeTask {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	return tm.activeTask
}

func (tm *TaskManager) EmitStatus(state TaskState, message string, progress int) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if tm.activeTask == nil {
		return
	}

	tm.activeTask.State = state
	tm.activeTask.Message = message
	tm.activeTask.Progress = progress

	if tm.ctx != nil {
		runtime.EventsEmit(tm.ctx, "browser-ai:status", StatusEvent{
			TaskID:   tm.activeTask.ID,
			State:    state,
			Message:  message,
			Progress: progress,
		})
	}
}

func (tm *TaskManager) EmitError(err error) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if tm.activeTask == nil {
		return
	}

	tm.activeTask.State = TaskStateFailed
	tm.activeTask.Message = err.Error()

	if tm.ctx != nil {
		runtime.EventsEmit(tm.ctx, "browser-ai:error", map[string]string{
			"taskId":  tm.activeTask.ID,
			"message": err.Error(),
		})
	}
}

func (tm *TaskManager) EmitResult(result GenerateResult) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if tm.activeTask == nil {
		return
	}

	tm.activeTask.State = TaskStateCompleted
	tm.activeTask.Message = "Tác vụ hoàn thành thành công"
	tm.activeTask.Progress = 100

	if tm.ctx != nil {
		runtime.EventsEmit(tm.ctx, "browser-ai:result", result)
	}
}

func (tm *TaskManager) CancelTask(id string) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if tm.activeTask == nil || tm.activeTask.ID != id {
		return nil // Idempotent
	}

	if tm.activeTask.Cancel != nil {
		tm.activeTask.Cancel()
	}

	tm.activeTask.State = TaskStateCancelled
	tm.activeTask.Message = "Tác vụ bị hủy bởi người dùng"

	if tm.ctx != nil {
		runtime.EventsEmit(tm.ctx, "browser-ai:status", StatusEvent{
			TaskID:   tm.activeTask.ID,
			State:    TaskStateCancelled,
			Message:  "Tác vụ bị hủy bởi người dùng",
			Progress: tm.activeTask.Progress,
		})
	}

	return nil
}

func (tm *TaskManager) EmitSelectionRequired(taskID string, previews []string) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if tm.activeTask == nil || tm.activeTask.ID != taskID {
		return
	}

	tm.activeTask.State = TaskStateSelectionRequired
	tm.activeTask.Message = "Yêu cầu người dùng chọn ảnh"

	if tm.ctx != nil {
		runtime.EventsEmit(tm.ctx, "browser-ai:selection-required", SelectionEvent{
			TaskID:   taskID,
			Previews: previews,
		})
		// Also emit a general status event so the progress states align
		runtime.EventsEmit(tm.ctx, "browser-ai:status", StatusEvent{
			TaskID:   taskID,
			State:    TaskStateSelectionRequired,
			Message:  "Vui lòng chọn ảnh bạn muốn tải",
			Progress: 75,
		})
	}
}

func (tm *TaskManager) SubmitSelection(taskID string, selectedIndexes []int) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if tm.activeTask == nil || tm.activeTask.ID != taskID {
		return fmt.Errorf("không tìm thấy tác vụ đang hoạt động")
	}

	select {
	case tm.activeTask.SelectionChan <- selectedIndexes:
		return nil
	default:
		return fmt.Errorf("đã gửi lựa chọn trước đó")
	}
}
