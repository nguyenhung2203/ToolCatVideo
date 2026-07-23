//go:build windows
// +build windows

package browserai

// launchHidden launches Chrome on Windows with CREATE_NO_WINDOW so no CMD
// flashes appear when running as a packaged .exe (wails build).
// go-rod's default launcher.Launch() sets CREATE_NEW_PROCESS_GROUP but NOT
// CREATE_NO_WINDOW, which causes a brief black console window on every start.

import (
	"bufio"
	"fmt"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/go-rod/rod/lib/launcher"
)

const (
	createNoWindow = 0x08000000
	createNewPG    = 0x00000200
)

// killOrphanChromeForProfile giết các tiến trình chrome.exe cũ (mồ côi) đang bám
// đúng profile của app trước khi khởi chạy Chrome mới. BẮT BUỘC cho bản phát hành:
// nếu app crash / bị tắt bằng Task Manager / mất điện, Close() không kịp chạy nên
// Chrome ngầm vẫn giữ --user-data-dir; lần mở sau Chrome mới sẽ handoff cho instance
// cũ rồi tự thoát → không có cửa sổ nào hiện lên. Lọc theo đường dẫn profile RIÊNG
// của app (SmartSplitter\browser-profile\...) nên KHÔNG đụng Chrome cá nhân người dùng.
func killOrphanChromeForProfile(profileDir string) {
	if strings.TrimSpace(profileDir) == "" {
		return
	}
	// PowerShell CIM: tìm mọi chrome.exe có CommandLine chứa đường dẫn profile rồi kill.
	// -replace để escape dấu \ và ' trong đường dẫn khi nhúng vào chuỗi PowerShell.
	safe := strings.ReplaceAll(profileDir, "'", "''")
	ps := fmt.Sprintf(
		`Get-CimInstance Win32_Process -Filter "Name='chrome.exe'" | `+
			`Where-Object { $_.CommandLine -like '*%s*' } | `+
			`ForEach-Object { Stop-Process -Id $_.ProcessId -Force -ErrorAction SilentlyContinue }`,
		safe,
	)
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", ps)
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNoWindow | createNewPG}
	_ = cmd.Run()
}

// launchBrowserHidden prepares a launcher l (already configured with Bin,
// Headless, UserDataDir, flags …) and launches Chrome with a hidden console
// window.  It returns the CDP ws URL the same way launcher.Launch() does.
//
// Internally it:
//  1. Disables leakless (which adds its own helper process we can't control)
//  2. Calls l.FormatArgs() to obtain the final Chrome CLI flags
//  3. Builds an exec.Cmd with CREATE_NO_WINDOW | CREATE_NEW_PROCESS_GROUP
//  4. Waits for Chrome to print its ws:// debug URL on stderr/stdout
func launchBrowserHidden(l *launcher.Launcher) (string, *exec.Cmd, error) {
	// Disable leakless so launcher doesn't wrap chrome in a helper subprocess
	l.Leakless(false)

	// go-rod stores the Chrome binary path under the "rod-bin" flag
	bin := l.Get("rod-bin")
	if bin == "" {
		return "", nil, fmt.Errorf("launchBrowserHidden: Chrome binary path not set in launcher")
	}

	args := l.FormatArgs()

	cmd := exec.Command(bin, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: createNoWindow | createNewPG,
	}

	// Chrome prints its debug port line on stderr.
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return "", nil, fmt.Errorf("stderr pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", nil, fmt.Errorf("stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return "", nil, fmt.Errorf("start chrome: %w", err)
	}

	// Read lines from stderr/stdout until we see the DevTools listening line.
	urlCh := make(chan string, 1)
	scan := func(r *bufio.Scanner) {
		for r.Scan() {
			line := r.Text()
			if strings.Contains(line, "DevTools listening on") {
				// Line looks like: DevTools listening on ws://127.0.0.1:PORT/...
				idx := strings.Index(line, "ws://")
				if idx >= 0 {
					select {
					case urlCh <- line[idx:]:
					default:
					}
				}
			}
		}
	}
	go scan(bufio.NewScanner(stderr))
	go scan(bufio.NewScanner(stdout))

	// Phát hiện Chrome thoát SỚM: khi profile (--user-data-dir) đã bị một Chrome
	// khác giữ, tiến trình mới sẽ chuyển lệnh cho instance cũ rồi tự thoát NGAY mà
	// KHÔNG in dòng "DevTools listening". Nếu không bắt trường hợp này ta sẽ treo
	// đủ 30s rồi mới báo lỗi (biểu hiện: bấm tạo ảnh mà không có gì mở lên).
	exitCh := make(chan error, 1)
	go func() { exitCh <- cmd.Wait() }()

	select {
	case wsURL := <-urlCh:
		url, err := resolveDebugURL(wsURL)
		if err != nil {
			_ = cmd.Process.Kill()
			return "", nil, err
		}
		return url, cmd, nil
	case <-exitCh:
		// Chrome thoát trước khi kịp báo DevTools URL → gần như chắc chắn do profile
		// đang bị một tiến trình Chrome cũ (mồ côi) khoá.
		return "", nil, fmt.Errorf("Chrome thoát ngay khi khởi chạy — có thể còn một tiến trình Chrome cũ đang chạy ngầm và giữ hồ sơ. Hãy đóng hết Chrome của công cụ (hoặc dùng nút Xóa hồ sơ trình duyệt) rồi thử lại")
	case <-time.After(30 * time.Second):
		_ = cmd.Process.Kill()
		return "", nil, fmt.Errorf("timeout waiting for Chrome DevTools URL")
	}
}

// resolveDebugURL trả về NGUYÊN full ws:// URL mà Chrome in ra trên stderr, dạng
// ws://127.0.0.1:PORT/devtools/browser/UUID. Đây chính là URL mà rod truyền thẳng
// vào cdp WebSocket.Connect để bắt tay — nó KHÔNG tự resolve, nên bắt buộc phải có
// đủ path /devtools/browser/UUID. Nếu cắt còn http://host:port (không path) thì
// handshake sẽ vào "/" và Chrome trả 404 Not Found ("websocket bad handshake").
func resolveDebugURL(wsURL string) (string, error) {
	wsURL = strings.TrimSpace(wsURL)
	if wsURL == "" {
		return "", fmt.Errorf("empty DevTools ws URL")
	}
	return wsURL, nil
}
