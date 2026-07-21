package browserai

import (
	"encoding/json"
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
	debugLogMu       sync.Mutex // tuần tự hóa ghi + rotation khi nhiều worker log song song
)

// maxDebugLogSize là ngưỡng kích thước file log (10MB). Vượt ngưỡng thì cắt bớt,
// chỉ giữ lại nửa cuối gần nhất để file không phình vô hạn theo thời gian dùng.
const maxDebugLogSize = 10 * 1024 * 1024

// rotateDebugLogIfNeeded cắt file log khi vượt ngưỡng: đọc toàn bộ, giữ lại nửa
// sau (bỏ phần cũ nhất), rồi ghi đè. Gọi trong debugLogMu nên an toàn đồng thời.
func rotateDebugLogIfNeeded(path string) {
	info, err := os.Stat(path)
	if err != nil || info.Size() < maxDebugLogSize {
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	// Giữ nửa cuối; cắt tới đầu dòng kế tiếp để không bỏ dở giữa một dòng.
	half := data[len(data)/2:]
	if idx := indexByte(half, '\n'); idx >= 0 && idx+1 < len(half) {
		half = half[idx+1:]
	}
	_ = os.WriteFile(path, half, 0644)
}

// indexByte trả về vị trí byte b đầu tiên trong s, hoặc -1. Tránh import bytes.
func indexByte(s []byte, b byte) int {
	for i := range s {
		if s[i] == b {
			return i
		}
	}
	return -1
}

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
	// Không in ra stdout nữa để tránh spam CMD; chỉ ghi vào file log debug.

	path := resolveDebugLogPath()
	debugLogMu.Lock()
	defer debugLogMu.Unlock()

	rotateDebugLogIfNeeded(path)

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.WriteString(time.Now().Format("2006-01-02 15:04:05") + " - " + formatted + "\n")
}

// getSettingsFilePath trả về đường dẫn file settings.json tập trung
func getSettingsFilePath() string {
	dir, err := os.UserConfigDir()
	if err != nil || dir == "" {
		dir = os.TempDir()
	}
	appDir := filepath.Join(dir, "video-splitter")
	_ = os.MkdirAll(appDir, 0755)
	return filepath.Join(appDir, "settings.json")
}

// GlobalConfigData biểu diễn thông số cấu hình tập trung liên quan tới AI
type GlobalConfigData struct {
	BrowserAIConcurrency int  `json:"browserAIConcurrency"`
	BrowserAIShowChrome  bool `json:"browserAIShowChrome"`
}

// LoadGlobalSettingsConfig đọc cấu hình tập trung từ settings.json cho package browserai
func LoadGlobalSettingsConfig() GlobalConfigData {
	cfg := GlobalConfigData{
		BrowserAIConcurrency: DefaultBrowserConcurrency,
		BrowserAIShowChrome:  true,
	}
	data, err := os.ReadFile(getSettingsFilePath())
	if err == nil && len(data) > 0 {
		_ = json.Unmarshal(data, &cfg)
	}
	if cfg.BrowserAIConcurrency <= 0 {
		cfg.BrowserAIConcurrency = DefaultBrowserConcurrency
	}
	if cfg.BrowserAIConcurrency > MaxBrowserConcurrency {
		cfg.BrowserAIConcurrency = MaxBrowserConcurrency
	}
	return cfg
}
