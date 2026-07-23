package browserai

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
)

type BrowserSession struct {
	mu          sync.Mutex
	launcher    *launcher.Launcher
	// chromeCmd là tiến trình Chrome do launchBrowserHidden tự khởi (Windows). Vì
	// ta KHÔNG gọi launcher.Launch() nên launcher.Kill() là no-op — phải tự giữ PID
	// ở đây và Kill lúc đóng, nếu không mỗi lần đóng sẽ rò 1 Chrome ngầm giữ profile
	// khiến lần mở sau bị handoff → không có cửa sổ nào hiện lên.
	chromeCmd   *exec.Cmd
	browser     *rod.Browser
	page        *rod.Page
	profileDir  string
	downloadDir string
	isHeadless  bool
	// downloadMu tuần tự hóa đoạn tải file giữa nhiều tab song song: WaitDownload
	// hoạt động ở cấp browser nên nếu 2 tab tải cùng lúc dễ bắt nhầm file của nhau.
	downloadMu sync.Mutex
	// pasteMu tuần tự hóa bước dán ảnh (Ctrl+V) giữa các cửa sổ song song:
	// Clipboard Windows chỉ chứa 1 ảnh cho toàn máy, nên 2 worker copy ảnh cùng
	// lúc sẽ đè clipboard của nhau → dán nhầm ảnh. Khóa quanh copy+Ctrl+V+chờ đính kèm.
	pasteMu     sync.Mutex
	rootInUse   bool
}

func NewBrowserSession() *BrowserSession {
	return &BrowserSession{}
}

func resolveProfileDir() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config dir: %w", err)
	}

	profileDir := filepath.Join(
		configDir,
		"SmartSplitter",
		"browser-profile",
		"google-ai",
	)

	if err := os.MkdirAll(profileDir, 0700); err != nil {
		return "", fmt.Errorf("create browser profile: %w", err)
	}

	return profileDir, nil
}

func resolveDownloadDir() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve user config dir: %w", err)
	}

	downloadDir := filepath.Join(
		configDir,
		"SmartSplitter",
		"browser-downloads",
	)

	if err := os.MkdirAll(downloadDir, 0755); err != nil {
		return "", fmt.Errorf("create downloads folder: %w", err)
	}

	return downloadDir, nil
}

func FindChromeExecutable(configuredPath string) (string, error) {
	if configuredPath != "" {
		if _, err := os.Stat(configuredPath); err == nil {
			return configuredPath, nil
		}
	}

	paths := []string{
		`C:\Program Files\Google\Chrome\Application\chrome.exe`,
		`C:\Program Files (x86)\Google\Chrome\Application\chrome.exe`,
		filepath.Join(os.Getenv("LOCALAPPDATA"), `Google\Chrome\Application\chrome.exe`),
	}

	for _, p := range paths {
		if p == "" {
			continue
		}
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}

	return "", fmt.Errorf("không tìm thấy Google Chrome. Hãy chắc chắn Chrome đã được cài đặt.")
}

func (b *BrowserSession) Start(
	ctx context.Context,
	chromePath string,
	startURL string,
	headless bool,
) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.browser != nil {
		if b.isHeadless == headless {
			// Check if browser is actually connected and working
			var errCheck error
			if b.page != nil {
				_, errCheck = b.page.Info()
			} else {
				errCheck = fmt.Errorf("no page reference")
			}
			if errCheck == nil && b.page != nil {
				// Browser is already running with requested headless mode, just navigate/create target
				page, err := b.browser.Page(proto.TargetCreateTarget{URL: startURL})
				if err == nil {
					enableFocusEmulation(page)
					b.page = page
					return nil
				}
			}
		}
		// If check failed or headless mode changed, clean up the old browser reference and recreate it
		if b.browser != nil {
			_ = b.browser.Close()
			b.browser = nil
		}
		if b.launcher != nil {
			b.launcher.Kill()
			b.launcher = nil
		}
		// Kill Chrome cũ do launchBrowserHidden khởi: nếu bỏ, toggle ẩn/hiện sẽ để lại
		// Chrome ngầm giữ profile → lần mở lại bị handoff, không cửa sổ nào hiện lên.
		if b.chromeCmd != nil && b.chromeCmd.Process != nil {
			_ = b.chromeCmd.Process.Kill()
			b.chromeCmd = nil
		}
		b.page = nil
	}

	profileDir, err := resolveProfileDir()
	if err != nil {
		return err
	}

	downloadDir, err := resolveDownloadDir()
	if err != nil {
		return err
	}

	chromeExecutable, err := FindChromeExecutable(chromePath)
	if err != nil {
		return err
	}

	l := launcher.New().
		Headless(headless).
		UserDataDir(profileDir).
		Set("no-first-run").
		Set("no-default-browser-check")

	if !headless {
		l = l.Set("start-maximized")
	}

	// Delete default automation flags to bypass Google anti-bot login block
	l.Delete("enable-automation")
	l.Delete("disable-extensions")
	l.Set("disable-blink-features", "AutomationControlled")

	l = l.Bin(chromeExecutable)

	// Dọn Chrome mồ côi còn bám profile trước khi launch. Tại đây nhánh cleanup đầu
	// hàm đã đóng browser cũ của app (nếu có), nên MỌI chrome.exe còn giữ profile này
	// đều là tiến trình rò từ lần chạy trước (app crash / tắt bằng Task Manager) —
	// giết hết để tránh handoff làm Chrome mới thoát ngay. Lọc theo đường dẫn profile
	// riêng nên không đụng Chrome cá nhân của người dùng.
	killOrphanChromeForProfile(profileDir)

	controlURL, chromeCmd, err := launchBrowserHidden(l)
	if err != nil {
		return fmt.Errorf("launch Chrome: %w", err)
	}

	browser := rod.New().
		ControlURL(controlURL).
		NoDefaultDevice().
		Context(ctx)

	if err := browser.Connect(); err != nil {
		if chromeCmd != nil && chromeCmd.Process != nil {
			_ = chromeCmd.Process.Kill()
		}
		l.Kill()
		return fmt.Errorf("connect Chrome: %w", err)
	}

	page, err := browser.Page(
		proto.TargetCreateTarget{
			URL: startURL,
		},
	)
	if err != nil {
		if chromeCmd != nil && chromeCmd.Process != nil {
			_ = chromeCmd.Process.Kill()
		}
		_ = browser.Close()
		l.Kill()
		return fmt.Errorf("open page: %w", err)
	}
	enableFocusEmulation(page)

	b.launcher = l
	b.chromeCmd = chromeCmd
	b.browser = browser
	b.page = page
	b.profileDir = profileDir
	b.downloadDir = downloadDir
	b.isHeadless = headless

	return nil
}

// enableFocusEmulation ép Chrome coi trang LUÔN đang được focus + active, độc lập
// với việc cửa sổ có hiển thị/foreground hay không. BẮT BUỘC cho chế độ ẩn trình
// duyệt: ô nhập Slate (contenteditable của React) từ chối nhận text khi
// document.hasFocus()==false — headless không bao giờ có focus nên mọi cách điền
// prompt (InsertText/execCommand/Ctrl+V/Input) đều trượt. Cũng có lợi khi chạy
// song song nhiều cửa sổ (chỉ 1 cửa sổ thật sự foreground tại một thời điểm).
func enableFocusEmulation(page *rod.Page) {
	if page == nil {
		return
	}
	_ = proto.EmulationSetFocusEmulationEnabled{Enabled: true}.Call(page)
}

func (b *BrowserSession) GetPage() (*rod.Page, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.browser == nil || b.page == nil {
		return nil, fmt.Errorf("trình duyệt chưa được khởi chạy")
	}
	return b.page, nil
}

// NewPage mở một CỬA SỔ mới (không phải tab) trên cùng browser đang chạy và
// điều hướng tới startURL. Mọi cửa sổ dùng chung UserDataDir nên chia sẻ
// profile/đăng nhập, NHƯNG mỗi cửa sổ có foreground riêng → hiện đồng thời và
// thao tác (Ctrl+V dán ảnh) được song song, không phải giành nhau tab active
// như khi mở nhiều tab trong cùng một cửa sổ.
func (b *BrowserSession) NewPage(startURL string) (*rod.Page, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.browser == nil {
		return nil, fmt.Errorf("trình duyệt chưa được khởi chạy")
	}
	page, err := b.browser.Page(proto.TargetCreateTarget{URL: startURL, NewWindow: true})
	if err != nil {
		return nil, fmt.Errorf("mở cửa sổ mới: %w", err)
	}
	enableFocusEmulation(page)
	return page, nil
}

// AcquirePageForWorker cấp trang cho worker trong Hàng Đợi AI song song.
// Worker đầu tiên đến (rootInUse == false) tái sử dụng tab gốc (b.page) để
// không thừa 1 tab rảnh ngồi chơi và giữ tổng số tab = số luồng cấu hình.
// Các worker sau (hoặc khi tab gốc bận) mở cửa sổ mới (isRoot = false).
// NOTE: không dùng workerID == 0 vì nextWorkerID không reset giữa các lần
// chạy → lần 2 workerID bắt đầu từ 3,4,5... không ai là 0, mọi worker đều
// mở tab mới → tổng tab = 1 (keep-alive) + n (worker) thay vì đúng n tab.
func (b *BrowserSession) AcquirePageForWorker(workerID int, startURL string) (page *rod.Page, isRoot bool, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.browser == nil {
		return nil, false, fmt.Errorf("trình duyệt chưa được khởi chạy")
	}

	// Worker đầu tiên đến → tận dụng tab gốc (bất kể workerID là bao nhiêu)
	if b.page != nil && !b.rootInUse {
		_, errCheck := b.page.Info()
		if errCheck == nil {
			b.rootInUse = true
			enableFocusEmulation(b.page)
			_ = b.page.Navigate(startURL)
			return b.page, true, nil
		}
	}

	// Mở cửa sổ/tab mới cho các worker sau (hoặc khi tab gốc không sẵn sàng)
	newPage, err := b.browser.Page(proto.TargetCreateTarget{URL: startURL, NewWindow: true})
	if err != nil {
		return nil, false, fmt.Errorf("mở cửa sổ mới: %w", err)
	}
	enableFocusEmulation(newPage)
	return newPage, false, nil
}

// ReleaseWorkerPage giải phóng tab khi worker kết thúc.
// Nếu là tab gốc (isRoot = true), giữ tab gốc sống làm keep-alive cho lần sau.
// Nếu là tab phụ (isRoot = false), đóng tab đó lại.
func (b *BrowserSession) ReleaseWorkerPage(page *rod.Page, isRoot bool) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if isRoot {
		b.rootInUse = false
	} else if page != nil {
		_ = page.Close()
	}
}

// LockDownload / UnlockDownload tuần tự hóa đoạn tải file giữa các tab song song.
func (b *BrowserSession) LockDownload()   { b.downloadMu.Lock() }
func (b *BrowserSession) UnlockDownload() { b.downloadMu.Unlock() }

// LockPaste / UnlockPaste tuần tự hóa đoạn dán ảnh giữa các cửa sổ song song.
// Clipboard Windows chỉ giữ được 1 ảnh cho toàn máy nên nếu 2 worker cùng
// copy-ảnh-rồi-Ctrl+V thì sẽ dán nhầm ảnh của nhau. Chỉ khóa quanh đoạn dán ngắn.
func (b *BrowserSession) LockPaste()   { b.pasteMu.Lock() }
func (b *BrowserSession) UnlockPaste() { b.pasteMu.Unlock() }

// DownloadDir trả về thư mục Chrome tải file tạm về.
func (b *BrowserSession) DownloadDir() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.downloadDir
}

// Browser trả về đối tượng rod.Browser đang chạy (nil nếu chưa mở).
func (b *BrowserSession) Browser() *rod.Browser {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.browser
}

func (b *BrowserSession) IsOpen() bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.browser == nil || b.page == nil {
		return false
	}
	_, err := b.page.Info()
	return err == nil
}

func (b *BrowserSession) IsHeadless() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.isHeadless
}

func (b *BrowserSession) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	var err error
	if b.browser != nil {
		err = b.browser.Close()
		b.browser = nil
	}
	if b.launcher != nil {
		b.launcher.Kill()
		b.launcher = nil
	}
	// Kill tiến trình Chrome do launchBrowserHidden tự khởi: launcher.Kill() không
	// đụng tới nó vì ta chưa từng gọi launcher.Launch(). Bỏ bước này thì browser.Close()
	// chỉ ngắt kết nối CDP còn Chrome vẫn sống ngầm, giữ profile → rò tiến trình.
	if b.chromeCmd != nil && b.chromeCmd.Process != nil {
		_ = b.chromeCmd.Process.Kill()
		b.chromeCmd = nil
	}
	b.page = nil
	return err
}
