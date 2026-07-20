package browserai

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"video-splitter/internal/utils"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type Service struct {
	mu         sync.Mutex
	appContext context.Context
	session    *BrowserSession
	tm         *TaskManager
}

func NewService() *Service {
	return &Service{
		session: NewBrowserSession(),
		tm:      NewTaskManager(),
	}
}

func (s *Service) Startup(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.appContext = ctx
	s.tm.SetContext(ctx)
}

func (s *Service) Shutdown(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()
	active := s.tm.GetActiveTask()
	if active != nil && active.Cancel != nil {
		active.Cancel()
	}
	_ = s.session.Close()
}

func (s *Service) OpenGoogleAI(provider string, showChrome bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	startURL := "https://gemini.google.com/app"
	if Provider(provider) == ProviderFlow {
		startURL = "https://labs.google/fx/vi/tools/flow"
	}

	// We use background context for browser connection to survive tasks cancellation
	err := s.session.Start(context.Background(), "", startURL, !showChrome)
	if err != nil {
		return fmt.Errorf("không khởi chạy được trình duyệt: %w", err)
	}

	return nil
}

func (s *Service) GetBrowserStatus() BrowserStatus {
	profileDir, _ := resolveProfileDir()
	isOpen := s.session.IsOpen()
	currentProvider := ""
	if isOpen {
		if p, err := s.session.GetPage(); err == nil && p != nil {
			info, _ := p.Info()
			if info != nil {
				if strings.Contains(info.URL, "gemini.google") {
					currentProvider = string(ProviderGemini)
				} else if strings.Contains(info.URL, "labs.google/fx") {
					currentProvider = string(ProviderFlow)
				}
			}
		}
	}
	return BrowserStatus{
		IsOpen:          isOpen,
		CurrentProvider: currentProvider,
		ProfileDir:      profileDir,
	}
}

func (s *Service) CheckLogin(provider string) (LoginStatus, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	page, err := s.session.GetPage()
	if err != nil {
		return LoginUnknown, err
	}

	if Provider(provider) == ProviderGemini {
		return DetectLoginState(page)
	}

	// For Flow, check if URL redirects to accounts.google.com
	info, err := page.Info()
	if err != nil {
		return LoginUnknown, err
	}
	if strings.Contains(info.URL, "accounts.google.com") {
		return LoginRequired, nil
	}

	// Also check if we are on the landing page with the "Create with Google Flow" button
	hasLandingBtn, _ := page.Eval(`() => {
		const buttons = Array.from(document.querySelectorAll('button'));
		return buttons.some(el => {
			const txt = el.textContent.toLowerCase();
			const isVisible = el.getBoundingClientRect().width > 0;
			return isVisible && (txt.includes('create with google flow') || txt.includes('bắt đầu với google flow') || txt.includes('đăng nhập') || txt.includes('sign in'));
		});
	}`)
	if hasLandingBtn != nil && hasLandingBtn.Value.Bool() {
		return LoginRequired, nil
	}

	return LoginReady, nil
}

func (s *Service) Generate(req GenerateRequest) (TaskInfo, error) {
	fmt.Printf("[BrowserAI] Generate Request: Provider=%s, MediaType=%s, Model=%s, Prompt=%s\n", req.Provider, req.MediaType, req.Model, req.Prompt)
	// 1. Request Validation
	if strings.TrimSpace(req.Prompt) == "" {
		return TaskInfo{}, fmt.Errorf("vui lòng nhập mô tả prompt")
	}
	if len(req.Prompt) > 2000 {
		return TaskInfo{}, fmt.Errorf("prompt quá dài (tối đa 2000 ký tự)")
	}
	if req.MediaType != MediaTypeImage && req.MediaType != MediaTypeVideo {
		return TaskInfo{}, fmt.Errorf("loại media không hợp lệ")
	}
	if req.Provider != ProviderGemini && req.Provider != ProviderFlow {
		return TaskInfo{}, fmt.Errorf("nhà cung cấp dịch vụ không hợp lệ")
	}
	if req.OutputDir == "" {
		return TaskInfo{}, fmt.Errorf("thư mục lưu kết quả không được để trống")
	}

	// Prevent path traversal
	req.OutputDir = filepath.Clean(req.OutputDir)
	if strings.Contains(req.OutputDir, "..") {
		return TaskInfo{}, fmt.Errorf("đường dẫn lưu trữ không hợp lệ")
	}

	_ = os.MkdirAll(req.OutputDir, 0755)

	// Ensure Browser is open
	if !s.session.IsOpen() {
		err := s.OpenGoogleAI(string(req.Provider), req.ShowChrome)
		if err != nil {
			return TaskInfo{}, err
		}
	}

	// 2. Initialize activeTask
	taskID := fmt.Sprintf("task_%d", time.Now().UnixNano())
	taskCtx, cancel := context.WithCancel(s.appContext)

	timeoutSec := req.TimeoutSecond
	if timeoutSec <= 0 {
		if req.MediaType == MediaTypeImage {
			timeoutSec = 300 // 5 mins
		} else {
			timeoutSec = 1200 // 20 mins
		}
	}
	if timeoutSec > 1800 {
		timeoutSec = 1800 // Limit to max 30 mins
	}

	taskCtx, cancel = context.WithTimeout(taskCtx, time.Duration(timeoutSec)*time.Second)

	task, err := s.tm.StartTask(taskID, req, cancel)
	if err != nil {
		cancel()
		return TaskInfo{}, err
	}

	// 3. Start automation execution in background goroutine
	go func() {
		defer cancel()

		// Decode pasted base64 images if exist
		var decodedPaths []string
		if len(req.InputImageBase64s) > 0 {
			for idx, b64 := range req.InputImageBase64s {
				if b64 == "" {
					continue
				}
				parts := strings.Split(b64, ",")
				base64Data := b64
				ext := ".png"
				if len(parts) > 1 {
					base64Data = parts[1]
					header := parts[0]
					if strings.Contains(header, "image/jpeg") || strings.Contains(header, "image/jpg") {
						ext = ".jpg"
					} else if strings.Contains(header, "image/gif") {
						ext = ".gif"
					}
				}

				data, errDecode := base64.StdEncoding.DecodeString(base64Data)
				if errDecode == nil {
					tempDir := os.TempDir()
					tempFile := filepath.Join(tempDir, fmt.Sprintf("pasted_img_%s_%d%s", taskID, idx, ext))
					errWrite := os.WriteFile(tempFile, data, 0644)
					if errWrite == nil {
						decodedPaths = append(decodedPaths, tempFile)
						fmt.Println("[Base64] Lưu ảnh thành công từ clipboard:", tempFile)
					}
				}
			}
		} else if req.InputImageBase64 != "" {
			parts := strings.Split(req.InputImageBase64, ",")
			base64Data := req.InputImageBase64
			ext := ".png"
			if len(parts) > 1 {
				base64Data = parts[1]
				header := parts[0]
				if strings.Contains(header, "image/jpeg") || strings.Contains(header, "image/jpg") {
					ext = ".jpg"
				} else if strings.Contains(header, "image/gif") {
					ext = ".gif"
				}
			}

			data, errDecode := base64.StdEncoding.DecodeString(base64Data)
			if errDecode == nil {
				tempDir := os.TempDir()
				tempFile := filepath.Join(tempDir, fmt.Sprintf("pasted_img_%s%s", taskID, ext))
				errWrite := os.WriteFile(tempFile, data, 0644)
				if errWrite == nil {
					decodedPaths = append(decodedPaths, tempFile)
					fmt.Println("[Base64] Lưu ảnh thành công từ clipboard:", tempFile)
				}
			}
		}

		// Combine file paths
		var allInputPaths []string
		if len(req.InputImagePaths) > 0 {
			allInputPaths = append(allInputPaths, req.InputImagePaths...)
		} else if req.InputImagePath != "" {
			allInputPaths = append(allInputPaths, req.InputImagePath)
		}
		allInputPaths = append(allInputPaths, decodedPaths...)

		req.InputImagePaths = allInputPaths

		// Register cleanup
		if len(decodedPaths) > 0 {
			defer func() {
				for _, p := range decodedPaths {
					_ = os.Remove(p)
				}
			}()
		}

		var filePaths []string
		var genErr error

		if req.Provider == ProviderGemini {
			filePaths, genErr = GenerateGeminiImage(taskCtx, s.session, s.tm, req)
		} else {
			filePaths, genErr = GenerateFlowVideo(taskCtx, s.session, s.tm, req)
		}

		if genErr != nil {
			if taskCtx.Err() != nil && errors.Is(taskCtx.Err(), context.Canceled) {
				s.tm.EmitStatus(TaskStateCancelled, "Tác vụ đã bị hủy bởi người dùng.", 0)
			} else {
				s.tm.EmitError(genErr)
			}
			return
		}

		// Read file size of the primary/first file
		var size int64
		primaryPath := ""
		primaryName := ""
		if len(filePaths) > 0 {
			primaryPath = filePaths[0]
			primaryName = filepath.Base(primaryPath)
			if info, statErr := os.Stat(primaryPath); statErr == nil {
				size = info.Size()
			}
		}

		// Completed status emit
		s.tm.EmitResult(GenerateResult{
			TaskID:      taskID,
			Provider:    req.Provider,
			MediaType:   req.MediaType,
			FilePath:    primaryPath,
			FileName:    primaryName,
			FilePaths:   filePaths,
			MimeType:    "application/octet-stream", // will be adjusted in frontend
			FileSize:    size,
			DurationMS:  0,
			CompletedAt: time.Now().Format(time.RFC3339),
		})
	}()

	return TaskInfo{
		TaskID:    taskID,
		State:     task.State,
		Message:   "Khởi tạo tác vụ thành công",
		StartedAt: task.StartedAt.Format(time.RFC3339),
	}, nil
}

func (s *Service) Cancel(taskID string) error {
	return s.tm.CancelTask(taskID)
}

func (s *Service) SubmitSelection(taskID string, selectedIndexes []int) error {
	return s.tm.SubmitSelection(taskID, selectedIndexes)
}

func (s *Service) OpenOutputFolder(path string) error {
	if path == "" {
		return fmt.Errorf("đường dẫn trống")
	}
	cleanPath := filepath.Clean(path)
	if _, err := os.Stat(cleanPath); os.IsNotExist(err) {
		_ = os.MkdirAll(cleanPath, 0755)
	}
	cmd := exec.Command("explorer", cleanPath)
	utils.HideCmdWindow(cmd)
	return cmd.Start()
}

func (s *Service) CloseBrowser() error {
	return s.session.Close()
}

func (s *Service) ClearBrowserProfile() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 1. Force close browser first
	_ = s.session.Close()

	// 2. Delete user profile directory
	profileDir, err := resolveProfileDir()
	if err != nil {
		return err
	}

	err = os.RemoveAll(profileDir)
	if err != nil {
		return fmt.Errorf("không xóa được profile (có thể Chrome vẫn đang chạy ngầm): %w", err)
	}

	// Recreate folder to keep structure clean
	_ = os.MkdirAll(profileDir, 0700)

	if s.appContext != nil {
		runtime.EventsEmit(s.appContext, "browser-ai:browser-closed", nil)
	}

	return nil
}

func (s *Service) ConfirmSelectedImages(
	taskID string,
	selectedPaths []string,
	outputDir string,
	fileName string,
) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(selectedPaths) == 0 {
		return nil, fmt.Errorf("không có ảnh nào được chọn")
	}

	// Đảm bảo thư mục output tồn tại
	_ = os.MkdirAll(outputDir, 0755)

	var movedPaths []string
	for idx, srcPath := range selectedPaths {
		if _, err := os.Stat(srcPath); os.IsNotExist(err) {
			continue
		}

		// Tạo tên file an toàn dạng: fileName_1.jpg, fileName_2.jpg... hoặc giữ nguyên nếu chỉ chọn 1 tấm
		ext := filepath.Ext(srcPath)
		var destName string
		if len(selectedPaths) > 1 {
			destName = fmt.Sprintf("%s_%d%s", fileName, idx+1, ext)
		} else {
			destName = fmt.Sprintf("%s%s", fileName, ext)
		}

		destPath := filepath.Join(outputDir, sanitizeFileName(destName))
		destPath = getUniqueFilePath(destPath)

		err := moveFile(srcPath, destPath)
		if err != nil {
			return nil, fmt.Errorf("lỗi di chuyển file %s: %w", srcPath, err)
		}
		movedPaths = append(movedPaths, destPath)
	}

	// Xóa thư mục tạm của task này
	if len(selectedPaths) > 0 {
		tempDir := filepath.Dir(selectedPaths[0])
		if strings.Contains(tempDir, "preview_temp") {
			_ = os.RemoveAll(tempDir)
		}
	}

	return movedPaths, nil
}
