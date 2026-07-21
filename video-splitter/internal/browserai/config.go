package browserai

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Ngưỡng và hằng số dùng chung cho luồng tự động hóa Google Flow.
const (
	// MaxProjectImages là số ảnh tối đa cho phép tồn tại trong một dự án Google Flow
	// trước khi tự động tạo dự án mới (tránh Chrome bị lag khi DOM quá nhiều thẻ ảnh).
	MaxProjectImages = 20

	// MaxGenerateAttempts là số lần thử lại tối đa khi gửi prompt gặp lỗi/vi phạm chính sách.
	MaxGenerateAttempts = 3

	// ImageGenerateTimeout là thời gian chờ tối đa Google Flow tạo xong hình ảnh.
	ImageGenerateTimeout = 10 * time.Minute

	// VideoGenerateTimeout là thời gian chờ tối đa Google Flow tạo xong video.
	VideoGenerateTimeout = 20 * time.Minute

	// MaxBrowserConcurrency là số luồng (tab Chrome) tối đa cho phép chạy song song
	// trong Hàng Đợi AI. Mỗi tab dùng chung profile nhưng làm việc trên 1 project riêng.
	MaxBrowserConcurrency = 16

	// DefaultBrowserConcurrency là số luồng mặc định khi người dùng chưa cấu hình.
	DefaultBrowserConcurrency = 3

	// FlowActionDelayMultiplier là hệ số delay cho các thao tác tự động hóa trên Google
	// Flow (thay cho ô "Độ trễ" chỉnh tay trước đây). Hardcode để đảm bảo ổn định.
	FlowActionDelayMultiplier = 1.0
)

// debugLogFileName là tên file log ghi trong thư mục cấu hình của người dùng.
const debugLogFileName = "flow_submit_debug.log"

var (
	debugLogPathOnce sync.Once
	debugLogPath     string
)

// resolveDebugLogPath trả về đường dẫn file log nằm trong thư mục cấu hình của
// người dùng (không hard-code ổ đĩa) để hoạt động trên mọi máy.
func resolveDebugLogPath() string {
	debugLogPathOnce.Do(func() {
		dir, err := os.UserConfigDir()
		if err != nil || dir == "" {
			dir = os.TempDir()
		}
		appDir := filepath.Join(dir, "video-splitter")
		_ = os.MkdirAll(appDir, 0755)
		debugLogPath = filepath.Join(appDir, debugLogFileName)
	})
	return debugLogPath
}

// flowLogf ghi một dòng log debug cho luồng Google Flow: in ra stdout và ghi
// nối vào file log trong thư mục cấu hình người dùng. Dùng chung cho mọi hàm
// trong package thay cho các closure logDebug hard-code trước đây.
func flowLogf(msg string, args ...interface{}) {
	formatted := fmt.Sprintf(msg, args...)
	fmt.Println("[FlowDebug]", formatted)

	f, err := os.OpenFile(resolveDebugLogPath(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.WriteString(time.Now().Format("2006-01-02 15:04:05") + " - " + formatted + "\n")
}
