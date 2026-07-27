package browserai

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"video-splitter/internal/media"

	"github.com/go-rod/rod/lib/proto"
)

// WaitAndMoveDownload waits for a download to finish, validates it, and moves it to outputDir
func WaitAndMoveDownload(
	ctx context.Context,
	waitDownload func() *proto.PageDownloadWillBegin,
	downloadDir string,
	outputDir string,
	fileName string,
	expectedType MediaType,
) (string, error) {
	// 1. Wait for Google Chrome to register download.
	// waitDownload() blocks until Chrome fires PageDownloadWillBegin — this NEVER returns
	// when the browser is hidden and the button click didn't actually trigger a download
	// (getBoundingClientRect returns 0 in windowless mode → button not found / not clicked).
	// We run it in a goroutine with a 45-second context-aware timeout so the caller
	// (and its LockDownload mutex) are never stuck.
	const waitDownloadTimeout = 45 * time.Second
	type dlResult struct {
		info *proto.PageDownloadWillBegin
	}
	dlCh := make(chan dlResult, 1)
	go func() {
		dlCh <- dlResult{info: waitDownload()}
	}()

	var downloadInfo *proto.PageDownloadWillBegin
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case <-time.After(waitDownloadTimeout):
		return "", errors.New("trình duyệt không bắt đầu tải file sau 45 giây (có thể cửa sổ bị ẩn hoặc nút tải không được kích hoạt)")
	case res := <-dlCh:
		downloadInfo = res.info
	}
	if downloadInfo == nil {
		return "", errors.New("trình duyệt không bắt đầu tải file hoặc tiến trình bị hủy")
	}


	tempPath := filepath.Join(downloadDir, downloadInfo.GUID)
	// Chrome tải ra file tạm ĐÚNG TÊN "GUID.crdownload" rồi đổi tên thành "GUID" khi
	// xong. Nên chỉ theo dõi đúng cặp file của download này — KHÔNG quét cả thư mục
	// (một .crdownload mồ côi của lần tải hỏng trước sẽ khiến vòng lặp chờ tới hết
	// timeout dù file của ta đã xong từ lâu).
	crDownloadPath := tempPath + ".crdownload"

	// 2. Wait for download completion (temporary .crdownload file to disappear and tempPath to exist)
	// We poll and wait.
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	// Timeout for download wait: default 10 minutes
	deadline := time.Now().Add(10 * time.Minute)

	var lastSize int64 = -1
	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-ticker.C:
		}

		if time.Now().After(deadline) {
			return "", NewError(ErrGenerationTimeout, "Quá thời gian tải file.")
		}

		// File .crdownload của CHÍNH download này vẫn còn → đang tải, chờ tiếp.
		if _, err := os.Stat(crDownloadPath); err == nil {
			lastSize = -1 // reset theo dõi kích thước ổn định
			continue
		}

		// File đích đã tồn tại và kích thước không đổi giữa 2 vòng poll → coi như
		// Chrome đã ghi xong (chờ size ổn định thay cho sleep cứng, chắc chắn hơn).
		info, err := os.Stat(tempPath)
		if err != nil || info.Size() == 0 {
			continue
		}
		if info.Size() == lastSize {
			break
		}
		lastSize = info.Size()
	}

	// 3. Validate file
	mimeType, size, err := ValidateDownloadedFile(tempPath, expectedType)
	if err != nil {
		_ = os.Remove(tempPath)
		return "", err
	}

	// 4. Generate final safe output path
	ext := ".jpg"
	if expectedType == MediaTypeVideo {
		ext = ".mp4"
		if strings.Contains(mimeType, "video/webm") {
			ext = ".webm"
		}
	} else {
		if strings.Contains(mimeType, "image/png") {
			ext = ".png"
		} else if strings.Contains(mimeType, "image/webp") {
			ext = ".webp"
		}
	}

	safeName := sanitizeFileName(fileName)
	if safeName == "" {
		safeName = fmt.Sprintf("generate_%d", time.Now().Unix())
	}

	destPath := filepath.Join(outputDir, safeName+ext)
	destPath = getUniqueFilePath(destPath)

	// 5. Move file (handle cross-volume move by copying and deleting)
	err = moveFile(tempPath, destPath)
	if err != nil {
		_ = os.Remove(tempPath)
		return "", fmt.Errorf("không di chuyển được file tải về vào thư mục lưu trữ: %w", err)
	}

	fmt.Printf("File tải về đã được chuyển đến: %s (Kích thước: %d bytes)\n", destPath, size)
	return destPath, nil
}

func ValidateDownloadedFile(path string, expectedType MediaType) (mimeType string, size int64, err error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", 0, fmt.Errorf("không đọc được thông tin file: %w", err)
	}
	size = info.Size()
	if size == 0 {
		return "", 0, NewError(ErrDownloadInvalid, "File tải về trống (0 bytes).")
	}

	// Detect MIME type
	f, err := os.Open(path)
	if err != nil {
		return "", 0, fmt.Errorf("không mở được file: %w", err)
	}
	defer f.Close()

	// Read first 512 bytes for content type detection
	buffer := make([]byte, 512)
	n, _ := f.Read(buffer)
	mimeType = http.DetectContentType(buffer[:n])

	if expectedType == MediaTypeImage {
		if !strings.HasPrefix(mimeType, "image/") {
			return "", 0, NewError(ErrDownloadInvalid, "Định dạng file không phải là ảnh hợp lệ: "+mimeType)
		}
	} else if expectedType == MediaTypeVideo {
		// Use ffprobe to validate video
		vi, errVideo := media.GetVideoInfo(path)
		if errVideo != nil || vi == nil || vi.Duration <= 0 {
			return "", 0, NewError(ErrDownloadInvalid, "Video tải về bị lỗi hoặc không đọc được bằng ffprobe.")
		}
		// Mime type could be generic, set as video/mp4 if not matched
		if !strings.HasPrefix(mimeType, "video/") && !strings.Contains(mimeType, "application/octet-stream") {
			mimeType = "video/mp4"
		}
	}

	return mimeType, size, nil
}

func sanitizeFileName(s string) string {
	s = strings.ReplaceAll(s, "..", "")
	s = strings.TrimLeft(s, "/\\_ ")

	if len(s) > 80 {
		s = s[:80]
	}
	// Replace illegal chars
	r := strings.NewReplacer(
		`\`, "_",
		`/`, "_",
		`:`, "_",
		`*`, "_",
		`?`, "_",
		`"`, "_",
		`<`, "_",
		`>`, "_",
		`|`, "_",
		` `, "_",
	)
	s = r.Replace(s)
	return s
}

func getUniqueFilePath(path string) string {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return path
	}
	ext := filepath.Ext(path)
	dir := filepath.Dir(path)
	base := strings.TrimSuffix(filepath.Base(path), ext)

	counter := 1
	for {
		newPath := filepath.Join(dir, fmt.Sprintf("%s-%d%s", base, counter, ext))
		if _, err := os.Stat(newPath); os.IsNotExist(err) {
			return newPath
		}
		counter++
	}
}

func moveFile(src string, dst string) error {
	// Make sure destination folder exists
	_ = os.MkdirAll(filepath.Dir(dst), 0755)

	// Attempt rename
	err := os.Rename(src, dst)
	if err == nil {
		return nil
	}

	// Fallback to copy & delete (cross-volume)
	source, err := os.Open(src)
	if err != nil {
		return err
	}
	defer source.Close()

	destination, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destination.Close()

	_, err = io.Copy(destination, source)
	if err != nil {
		return err
	}

	source.Close()
	return os.Remove(src)
}
