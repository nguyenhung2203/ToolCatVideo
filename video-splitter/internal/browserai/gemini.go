package browserai

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

func DetectLoginState(page *rod.Page) (LoginStatus, error) {
	info, err := page.Info()
	if err != nil {
		return LoginUnknown, err
	}
	url := info.URL
	if strings.Contains(url, "accounts.google.com") {
		return LoginRequired, nil
	}

	// Check if sign-in markers are visible
	_, err = FindFirstVisible(context.Background(), page, GeminiSelectors.LoginMarkers, 1*time.Second)
	if err == nil {
		return LoginRequired, nil
	}

	// Check if prompt input is visible
	_, err = FindFirstVisible(context.Background(), page, GeminiSelectors.PromptInputs, 1*time.Second)
	if err == nil {
		return LoginReady, nil
	}

	return LoginUnknown, nil
}

func FillPrompt(page *rod.Page, input *rod.Element, prompt string) error {
	if err := input.Focus(); err != nil {
		return fmt.Errorf("focus prompt input: %w", err)
	}

	_ = input.SelectAllText()
	_ = input.Input("")

	err := input.Input(prompt)
	if err != nil {
		return fmt.Errorf("input prompt: %w", err)
	}

	return nil
}

func GenerateGeminiImage(
	ctx context.Context,
	session *BrowserSession,
	tm *TaskManager,
	req GenerateRequest,
) ([]string, error) {
	page, err := session.GetPage()
	if err != nil {
		return nil, err
	}

	tm.EmitStatus(TaskStateLaunching, "Đang mở trang Gemini...", 10)
	err = page.Navigate("https://gemini.google.com/app")
	if err != nil {
		return nil, fmt.Errorf("navigate to Gemini: %w", err)
	}

	_ = page.WaitDOMStable(1*time.Second, 0.5)

	tm.EmitStatus(TaskStateLaunching, "Đang kiểm tra trạng thái đăng nhập...", 15)
	loginStat, err := DetectLoginState(page)
	if err != nil {
		return nil, err
	}
	if loginStat == LoginRequired {
		tm.EmitStatus(TaskStateLoginRequired, "Yêu cầu đăng nhập tài khoản Google.", 20)
		return nil, NewError(ErrLoginRequired, "Vui lòng đăng nhập tài khoản Google của bạn trên cửa sổ Chrome.")
	}

	// Clear welcome overlays if present
	DismissWelcomeModals(ctx, page)

	tm.EmitStatus(TaskStateReady, "Đang tìm ô nhập Prompt...", 25)
	promptInput, err := FindFirstVisible(ctx, page, GeminiSelectors.PromptInputs, 10*time.Second)
	if err != nil {
		return nil, NewError(ErrSelectorNotFound, "Không tìm thấy ô nhập prompt. Vui lòng mở Chrome để kiểm tra lại.")
	}

	tm.EmitStatus(TaskStateSubmitting, "Đang chuẩn bị prompt...", 35)
	aspectStr := req.AspectRatio
	if aspectStr == "" {
		aspectStr = "9:16"
	}
	normalizedPrompt := fmt.Sprintf(
		"Tạo một ảnh theo yêu cầu sau:\n%s\n\nYêu cầu đầu ra:\n- Tỷ lệ: %s.\n- Chất lượng cao.\n- Không thêm logo.\n- Không thêm watermark.\n- Không thêm chữ nếu không được yêu cầu.\n- Chỉ tạo ảnh, không mô tả bằng văn bản.",
		req.Prompt, aspectStr,
	)

	err = FillPrompt(page, promptInput, normalizedPrompt)
	if err != nil {
		return nil, fmt.Errorf("fill prompt: %w", err)
	}

	submitBtn, err := FindFirstVisible(ctx, page, GeminiSelectors.SubmitButtons, 5*time.Second)
	if err != nil {
		return nil, NewError(ErrSelectorNotFound, "Không tìm thấy nút gửi prompt.")
	}

	tm.EmitStatus(TaskStateSubmitting, "Đang gửi prompt...", 40)
	err = submitBtn.Click(proto.InputMouseButtonLeft, 1)
	if err != nil {
		return nil, fmt.Errorf("click submit: %w", err)
	}

	tm.EmitStatus(TaskStateGenerating, "Đang chờ Google AI tạo ảnh (quá trình này có thể mất 1-2 phút)...", 50)

	var downloadBtn *rod.Element
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	// Timeout for image generation is 5 minutes
	deadline := time.Now().Add(5 * time.Minute)

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
		}

		if time.Now().After(deadline) {
			return nil, NewError(ErrGenerationTimeout, "Quá thời gian chờ tạo ảnh từ Gemini.")
		}

		errEl, errCheckErr := FindFirstVisible(ctx, page, GeminiSelectors.ErrorMarkers, 500*time.Millisecond)
		if errCheckErr == nil && errEl != nil {
			txt, _ := errEl.Text()
			if strings.Contains(strings.ToLower(txt), "quota") || strings.Contains(strings.ToLower(txt), "giới hạn") {
				return nil, NewError(ErrQuotaExceeded, "Tài khoản đạt giới hạn tạo ảnh của Gemini: "+txt)
			}
			if strings.Contains(strings.ToLower(txt), "cannot") || strings.Contains(strings.ToLower(txt), "không thể") || strings.Contains(strings.ToLower(txt), "chính sách") {
				return nil, NewError(ErrGenerationRejected, "Yêu cầu bị từ chối do chính sách nội dung của Google: "+txt)
			}
		}

		btn, errCheckDl := FindFirstVisible(ctx, page, GeminiSelectors.DownloadButtons, 500*time.Millisecond)
		if errCheckDl == nil && btn != nil {
			downloadBtn = btn
			break
		}
	}

	tm.EmitStatus(TaskStateDownloading, "Đang tải ảnh xuống...", 85)
	waitDownload := session.browser.WaitDownload(session.downloadDir)
	err = downloadBtn.Click(proto.InputMouseButtonLeft, 1)
	if err != nil {
		return nil, fmt.Errorf("click download button: %w", err)
	}

	taskID := "temp"
	if active := tm.GetActiveTask(); active != nil {
		taskID = active.ID
	}
	tempOutputDir := filepath.Join(session.downloadDir, "preview_temp", taskID)

	filePath, err := WaitAndMoveDownload(ctx, waitDownload, session.downloadDir, tempOutputDir, fmt.Sprintf("temp_%d", 1), MediaTypeImage)
	if err != nil {
		return nil, err
	}

	return []string{filePath}, nil
}
