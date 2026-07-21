package browserai

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/input"
	"github.com/go-rod/rod/lib/proto"
)

func GenerateFlowVideo(
	ctx context.Context,
	session *BrowserSession,
	tm *TaskManager,
	req GenerateRequest,
) ([]string, error) {
	logDebug := func(msg string, args ...interface{}) {
		formatted := fmt.Sprintf(msg, args...)
		fmt.Println("[FlowDebug]", formatted)
		
		// Write to a local log file in workspace for the user to view
		f, err := os.OpenFile("d:\\CongTy\\ToolCatVideoNgan\\flow_submit_debug.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
		if err == nil {
			defer f.Close()
			_, _ = f.WriteString(time.Now().Format("2006-01-02 15:04:05") + " - " + formatted + "\n")
		}
	}

	sleep := func(base time.Duration) {
		delayMult := req.DelaySecond
		if delayMult <= 0 {
			delayMult = 1.0 // default multiplier is 1x
		}
		time.Sleep(time.Duration(float64(base) * delayMult))
	}

	// Đọc link project cũ đã lưu
	savedURL := readProjectURL()
	targetURL := "https://labs.google/fx/vi/tools/flow"
	if savedURL != "" && strings.Contains(savedURL, "/project/") {
		targetURL = savedURL
		logDebug("Phát hiện link dự án cũ đã lưu: %s. Tiến hành mở dự án này...", savedURL)
	} else {
		logDebug("Không có link dự án cũ hoặc không hợp lệ. Tiến hành mở trang chủ...")
	}

	page, err := session.GetPage()
	if err != nil {
		return nil, err
	}

	// Đăng ký script ẩn danh (stealth) để vượt qua bộ quét bot/webdriver của Google
	_, _ = page.EvalOnNewDocument(`() => {
		// 1. Ghi đè navigator.webdriver thành undefined để chống phát hiện tự động
		Object.defineProperty(navigator, 'webdriver', {
			get: () => undefined
		});

		// 2. Xóa các biến signature ẩn của Chrome DevTools Protocol
		try {
			delete window.cdc_adoQyhkntgdgCwRuuFormiP_Array;
			delete window.cdc_adoQyhkntgdgCwRuuFormiP_Promise;
		} catch (e) {}

		// 3. Giả lập chrome object giống trình duyệt người dùng bình thường
		window.chrome = {
			runtime: {},
			loadTimes: function() {},
			csi: function() {},
			app: {}
		};

		// 4. Khai báo plugins giả
		Object.defineProperty(navigator, 'plugins', {
			get: () => [
				{ name: 'Chrome PDF Viewer', filename: 'internal-pdf-viewer', description: 'Portable Document Format' },
				{ name: 'Chromium PDF Viewer', filename: 'internal-pdf-viewer', description: 'Portable Document Format' }
			]
		});

		// 5. Khai báo danh sách ngôn ngữ chuẩn
		Object.defineProperty(navigator, 'languages', {
			get: () => ['vi-VN', 'vi', 'en-US', 'en']
		});
	}`)

	logDebug("Bắt đầu mở trang Google Flow: %s", targetURL)
	tm.EmitStatus(TaskStateLaunching, "Đang mở trang Google Flow...", 10)
	err = page.Navigate(targetURL)
	if err != nil {
		logDebug("Lỗi Navigate: %v", err)
		return nil, fmt.Errorf("navigate to Google Flow: %w", err)
	}

	// Wait explicitly for the browser URL to transition to the target page or accounts login
	logDebug("Đang chờ trình duyệt tải xong trang Google Flow...")
	pageLoaded := false
	var info *proto.TargetTargetInfo
	for i := 0; i < 20; i++ { // Wait up to 10 seconds
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		info, err = page.Info()
		if err == nil {
			logDebug("URL hiện tại của trình duyệt (chờ load): %s", info.URL)
			if strings.Contains(info.URL, "labs.google/fx/vi/tools/flow") || strings.Contains(info.URL, "accounts.google.com") {
				pageLoaded = true
				break
			}
		}
		time.Sleep(500 * time.Millisecond)
	}

	if !pageLoaded {
		urlStr := ""
		if info != nil {
			urlStr = info.URL
		}
		logDebug("Timeout chờ tải trang chủ. URL hiện tại: %s", urlStr)
		return nil, fmt.Errorf("timeout chờ tải trang chủ Google Flow (URL hiện tại: %s)", urlStr)
	}

	_ = page.WaitDOMStable(1*time.Second, 0.5)

	tm.EmitStatus(TaskStateLaunching, "Đang kiểm tra trạng thái đăng nhập...", 15)
	logDebug("Bắt đầu kiểm tra trạng thái đăng nhập...")
	
	// Wait up to 5 seconds for redirects if we are on accounts.google.com
	for i := 0; i < 10; i++ {
		info, err = page.Info()
		if err != nil {
			return nil, err
		}
		if !strings.Contains(info.URL, "accounts.google.com") {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	logDebug("URL sau khi kiểm tra redirect login: %s", info.URL)

	// Check if landing page button is visible (meaning not logged in)
	hasLandingBtn, _ := page.Eval(`() => {
		const buttons = Array.from(document.querySelectorAll('button'));
		return buttons.some(el => {
			const txt = el.textContent.toLowerCase();
			const isVisible = el.getBoundingClientRect().width > 0;
			return isVisible && (txt.includes('create with google flow') || txt.includes('bắt đầu với google flow') || txt.includes('đăng nhập') || txt.includes('sign in'));
		});
	}`)
	if (hasLandingBtn != nil && hasLandingBtn.Value.Bool()) || strings.Contains(info.URL, "accounts.google.com") {
		logDebug("Phát hiện chưa đăng nhập Google (Landing button: %v, URL: %s)", hasLandingBtn != nil && hasLandingBtn.Value.Bool(), info.URL)
		tm.EmitStatus(TaskStateLoginRequired, "Yêu cầu đăng nhập tài khoản Google.", 20)
		return nil, NewError(ErrLoginRequired, "Vui lòng đăng nhập tài khoản Google của bạn trên cửa sổ Chrome.")
	}

	logDebug("Đăng nhập thành công. Đang đóng welcome overlays nếu có...")
	// Clear welcome overlays if present
	DismissWelcomeModals(ctx, page)

	// Check if we need to click "+ Dự án mới" (New project) to enter project workspace
	info, err = page.Info()
	hasProject := err == nil && strings.Contains(info.URL, "/project/")
	
	// Nếu chúng ta đã mở link project cũ nhưng sau khi load xong nó KHÔNG ở trong project (ví dụ bị đẩy về trang chủ)
	// Hoặc nếu ta không tìm thấy ô nhập prompt sau 5 giây, ta xem như project đó bị lỗi/bị xóa và cần tạo mới!
	if hasProject {
		logDebug("Xác minh dự án cũ có hoạt động bình thường không...")
		_, errCheckPrompt := FindFirstVisible(ctx, page, FlowSelectors.PromptInputs, 30*time.Second)
		if errCheckPrompt != nil {
			logDebug("Không tìm thấy ô nhập liệu trong dự án cũ sau 30 giây. Có vẻ dự án này đã bị lỗi hoặc bị xóa. Tiến hành mở trang chủ để tạo dự án mới...")
			hasProject = false
			saveProjectURL("") // Xóa link dự án cũ bị lỗi/bị xóa để lần sau không thử lại nữa
			errNavigateHome := page.Navigate("https://labs.google/fx/vi/tools/flow")
			if errNavigateHome == nil {
				_ = page.WaitDOMStable(2*time.Second, 0.5)
			}
		}
	}

	if !hasProject {
		logDebug("Dự án cũ không hoạt động hoặc không có. Tiến hành click tạo dự án mới...")
		tm.EmitStatus(TaskStateReady, "Đang vào không gian làm việc (tạo dự án mới)...", 21)
		
		// Hàm tìm nút tạo dự án mới bằng JS, ưu tiên XPath chính xác của người dùng trước
		findBtnJS := rod.Eval(`() => {
			// 1. Thử tìm bằng các đường dẫn XPath cụ thể
			const xpaths = [
				` + "`" + `//*[@id="__next"]/div[2]/div/div/button` + "`" + `,
				` + "`" + `//*[@id="__next"]/div[1]/div/div/button` + "`" + `,
				` + "`" + `//*[@id="__next"]/div/div/div/button` + "`" + `
			];
			for (const xpath of xpaths) {
				try {
					const res = document.evaluate(xpath, document, null, XPathResult.FIRST_ORDERED_NODE_TYPE, null).singleNodeValue;
					if (res) {
						const rect = res.getBoundingClientRect();
						if (rect.width > 0 && rect.height > 0) {
							return res;
						}
					}
				} catch(e) {}
			}

			// 2. Dự phòng: Tìm theo từ khóa chữ thường không phân biệt hoa thường
			const tags = ['button', 'div', 'span', 'a', '*'];
			const keywords = ['dự án mới', 'new project', 'create project'];
			for (const tag of tags) {
				const elements = Array.from(document.querySelectorAll(tag));
				for (const el of elements) {
					if (!el.textContent) continue;
					const text = el.textContent.toLowerCase().trim();
					for (const kw of keywords) {
						if (text.includes(kw)) {
							const rect = el.getBoundingClientRect();
							if (rect.width > 0 && rect.height > 0) {
								return el;
							}
						}
					}
				}
			}
			return null;
		}`)

		// Click lần đầu
		btn, errJS := page.ElementByJS(findBtnJS)
		if errJS == nil && btn != nil {
			logDebug("Đã tìm thấy nút Tạo dự án mới. Đang mô phỏng click chuột thật...")
			_ = btn.ScrollIntoView()
			if errClick := btn.Click(proto.InputMouseButtonLeft, 1); errClick != nil {
				logDebug("Click chuột thật thất bại, dùng JS click dự phòng: %v", errClick)
				_, _ = btn.Eval("function() { this.click(); }")
			}
		} else {
			logDebug("Không tìm thấy nút tạo dự án mới qua JS. Thử tìm qua Selector...")
			newProjectBtn, errBtn := FindFirstVisible(ctx, page, []SelectorCandidate{
				{Selector: "button:contains('Dự án mới')"},
				{Selector: "button:contains('Dự Án Mới')"},
				{Selector: "button:contains('New project')"},
				{Selector: "button:contains('New Project')"},
				{Selector: "button"},
				{Selector: "div[role='button']"},
			}, 5*time.Second)
			if errBtn == nil && newProjectBtn != nil {
				_ = newProjectBtn.ScrollIntoView()
				_, _ = newProjectBtn.Eval("function() { this.click(); }")
			}
		}
		_ = page.WaitDOMStable(1*time.Second, 0.5)

		// 3. Đợi chuyển hướng URL sang "/project/", tiến hành click lại nếu bị trễ Event Listener của React
		urlSuccess := false
		logDebug("Đang chờ URL chuyển hướng sang '/project/'...")
		for idx := 0; idx < 20; idx++ { // Đợi tối đa 15 giây (20 * 750ms)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
			}
			info, err = page.Info()
			if err == nil {
				logDebug("URL hiện tại của trình duyệt (chờ /project/): %s", info.URL)
				if strings.Contains(info.URL, "/project/") {
					urlSuccess = true
					break
				}
			}

			// Cứ sau mỗi 2.2 giây (3 lần lặp * 750ms) nếu chưa chuyển hướng thì click lại
			if idx > 0 && idx % 3 == 0 {
				logDebug("Vẫn chưa chuyển hướng. Thử click lại nút Tạo dự án mới...")
				if retryBtn, errRetry := page.ElementByJS(findBtnJS); errRetry == nil && retryBtn != nil {
					_ = retryBtn.ScrollIntoView()
					if errClick := retryBtn.Click(proto.InputMouseButtonLeft, 1); errClick != nil {
						_, _ = retryBtn.Eval("function() { this.click(); }")
					}
				}
			}
			time.Sleep(750 * time.Millisecond)
		}
		if !urlSuccess {
			urlStr := ""
			if info != nil {
				urlStr = info.URL
			}
			logDebug("Timeout chờ chuyển hướng sang /project/. URL hiện tại: %s", urlStr)
			return nil, fmt.Errorf("không thể vào không gian làm việc của dự án (timeout chuyển hướng URL sang /project/ - URL hiện tại: %s)", urlStr)
		}
		
		// Đã tạo dự án mới thành công! Lưu link project mới này
		info, _ = page.Info()
		logDebug("Lưu link dự án mới vào settings.json: %s", info.URL)
		saveProjectURL(info.URL)
	} else {
		// Kiểm tra xem dự án cũ có vượt quá 50 ảnh hay không
		totalCardsCountObj, errTotalCount := page.Eval(`() => {
			const imgs = Array.from(document.querySelectorAll('img')).filter(img => {
				const src = img.getAttribute('src') || '';
				return src.includes('getMediaUrl') || src.includes('/fx/api/');
			});
			return imgs.length;
		}`)
		if errTotalCount == nil && totalCardsCountObj != nil {
			totalCount := totalCardsCountObj.Value.Int()
			logDebug("Số lượng hình ảnh hiện có trong dự án cũ: %d", totalCount)
			if totalCount >= 30 {
				logDebug("Dự án hiện tại đã có %d hình ảnh (vượt quá ngưỡng 50 ảnh để tránh lag). Tiến hành xóa link dự án cũ để lần tới tự động tạo dự án mới...", totalCount)
				saveProjectURL("")
			}
		}
		logDebug("Dự án cũ hoạt động bình thường! Tiếp tục sử dụng.")
	}
	logDebug("Đã vào dự án thành công!")

	// Detect if flow feature is visible/available
	mediaText := "Video"
	if req.MediaType == MediaTypeImage {
		mediaText = "Hình ảnh"
	}
	tm.EmitStatus(TaskStateReady, fmt.Sprintf("Đang kiểm tra quyền truy cập tính năng tạo %s...", mediaText), 22)
	logDebug("Bắt đầu quét tìm ô PromptInput...")
	promptInput, err := FindFirstVisible(ctx, page, FlowSelectors.PromptInputs, 15*time.Second)
	if err == nil && promptInput != nil {
		htmlSnippet, _ := promptInput.HTML()
		if len(htmlSnippet) > 200 {
			htmlSnippet = htmlSnippet[:200] + "..."
		}
		logDebug("Đã tìm thấy PromptInput! HTML snippet: %s", htmlSnippet)
	}
	if err != nil || promptInput == nil {
		if err == nil {
			err = fmt.Errorf("không tìm thấy ô nhập prompt")
		}
		// Check if there is an explicit unauthorized/waitlist page
		txt, _ := page.HTML()
		if strings.Contains(strings.ToLower(txt), "waitlist") || strings.Contains(strings.ToLower(txt), "not available") {
			if req.MediaType == MediaTypeImage {
				return nil, NewError(ErrFeatureUnavailable, "Tài khoản hiện tại không hiển thị hoặc chưa được cấp quyền sử dụng tính năng tạo hình ảnh.")
			}
			return nil, NewError(ErrFeatureUnavailable, "Tài khoản hiện tại không hiển thị hoặc chưa được cấp quyền sử dụng tính năng tạo video (Veo/VideoFX).")
		}
		if req.MediaType == MediaTypeImage {
			return nil, NewError(ErrSelectorNotFound, "Không tìm thấy giao diện tạo hình ảnh: " + err.Error())
		}
		return nil, NewError(ErrSelectorNotFound, "Không tìm thấy giao diện tạo video: " + err.Error())
	}

	// Configure default settings in Google Flow (Model, aspect ratio, confirmation prompt, etc.)
	if err := ConfigureFlowSettings(ctx, page, req, tm); err != nil {
		return nil, fmt.Errorf("cấu hình cài đặt thất bại: %w", err)
	}

	isPromptSent := func() bool {
		// 1. If the textbox text no longer contains the typed prompt, it means it has been sent/cleared!
		resText, errText := promptInput.Eval(`el => el.textContent`)
		if errText == nil && !strings.Contains(resText.Value.Str(), req.Prompt) {
			return true
		}

		// 2. Check if the submit button has changed to a "stop" button (indicating active generation)
		btnHtml, errBtn := page.Eval(`() => {
			const btn = Array.from(document.querySelectorAll('button')).find(el => {
				const icon = el.querySelector('i');
				return icon && (icon.textContent.trim() === 'arrow_forward' || icon.textContent.trim() === 'stop');
			});
			return btn ? btn.outerHTML : '';
		}`)
		if errBtn == nil && btnHtml != nil {
			htmlStr := btnHtml.Value.Str()
			if strings.Contains(htmlStr, "stop") || strings.Contains(htmlStr, "Dừng") || strings.Contains(htmlStr, "Stop") {
				return true
			}
		}
		return false
	}

	// Lấy danh sách URL tất cả ảnh hiện có trong dự án trước khi bấm Tạo
	var lastGroupUrls []string
	if req.MediaType == MediaTypeImage {
		lastUrlsObj, errLast := page.Eval(`() => {
			const imgs = Array.from(document.querySelectorAll('img')).filter(img => {
				const src = img.getAttribute('src') || '';
				return src.includes('getMediaUrl') || src.includes('/fx/api/');
			});
			return imgs.map(img => {
				let src = img.getAttribute('src') || '';
				if (src.startsWith('/')) {
					src = 'https://labs.google' + src;
				}
				return src;
			}).filter(src => src !== '');
		}`)
		if errLast == nil && lastUrlsObj != nil {
			for _, v := range lastUrlsObj.Value.Arr() {
				lastGroupUrls = append(lastGroupUrls, v.Str())
			}
		}
		logDebug("URL tất cả ảnh hiện tại trước khi tạo (%d ảnh): %v", len(lastGroupUrls), lastGroupUrls)
	}

	// Kiểm tra và in log tổng số lượng hình ảnh hiện có trong dự án trước khi bấm Tạo
	totalCardsCountObj, errTotalCount := page.Eval(`() => {
		const imgs = Array.from(document.querySelectorAll('img')).filter(img => {
			const src = img.getAttribute('src') || '';
			return src.includes('getMediaUrl') || src.includes('/fx/api/');
		});
		return imgs.length;
	}`)
	if errTotalCount == nil && totalCardsCountObj != nil {
		logDebug("Số lượng hình ảnh đang có trong dự án trước khi tạo: %d ảnh", totalCardsCountObj.Value.Int())
	}

	// Count initial download buttons if it is Video type
	initialVideoButtonsCount := 0
	if req.MediaType == MediaTypeVideo {
		hasDlCount, errDlCount := page.Eval(`() => {
			const candidates = Array.from(document.querySelectorAll('button, a'));
			let count = 0;
			for (const el of candidates) {
				const icon = el.querySelector('i, span.google-symbols, .material-icons, [class*="icon"]');
				const iconText = icon ? icon.textContent.trim().toLowerCase() : '';
				const aria = (el.getAttribute('aria-label') || '').toLowerCase();
				const title = (el.getAttribute('title') || '').toLowerCase();
				if (iconText === 'download' || iconText === 'file_download' || 
				    aria.includes('download') || aria.includes('tải xuống') || aria.includes('tải về') ||
				    title.includes('download') || title.includes('tải xuống') || title.includes('tải về')) {
					count++;
				}
			}
			return count;
		}`)
		if errDlCount == nil && hasDlCount != nil {
			initialVideoButtonsCount = hasDlCount.Value.Int()
		}
		logDebug("Số lượng nút tải video cũ đã có: %d", initialVideoButtonsCount)
	}

	maxAttempts := 3
	var finalErr error

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if attempt > 1 {
			logDebug("Phát hiện lỗi từ Google Flow hoặc quá trình tạo thất bại. Tự động gửi lại prompt (Thử lại lần %d/%d)...", attempt, maxAttempts)
			tm.EmitStatus(TaskStateSubmitting, fmt.Sprintf("Gặp lỗi, đang tự động gửi lại prompt (Lần %d/3)...", attempt), 35)
			
			// Refind prompt input
			promptInput, err = FindFirstVisible(ctx, page, FlowSelectors.PromptInputs, 5*time.Second)
			if err != nil {
				finalErr = fmt.Errorf("không tìm thấy ô nhập liệu khi gửi lại: %w", err)
				continue
			}
		}

		// 1. Dán ảnh bằng phím tắt Ctrl+V thật từ bàn phím hệ thống (Windows Clipboard + CDP Keyboard)
		if len(req.InputImagePaths) > 0 {
			logDebug("Phát hiện yêu cầu gửi kèm ảnh (%d ảnh). Đang nạp vào Clipboard Windows và bấm Ctrl+V thật...", len(req.InputImagePaths))
			
			for _, imgPath := range req.InputImagePaths {
				_ = PasteImageNativeCtrlV(ctx, page, promptInput, imgPath, logDebug)
			}

			logDebug("Đã bấm Ctrl+V! Đang chờ DOM tải xong 100% (thẻ ảnh hiển thị trong ô prompt)...")
			WaitUntilImageAttachedAndLoaded(ctx, page, 25*time.Second, logDebug)
			sleep(1500 * time.Millisecond) // Chờ thêm 1.5 giây cho React cập nhật state

			// Quét tìm lại ô prompt sau khi đính kèm ảnh
			promptInput, err = FindFirstVisible(ctx, page, FlowSelectors.PromptInputs, 5*time.Second)
			if err != nil {
				finalErr = fmt.Errorf("không tìm thấy ô nhập liệu sau khi upload ảnh: %w", err)
				continue
			}
		}

		// 2. Điền văn bản prompt vào ô nhập liệu (sau khi đã đính kèm ảnh)
		logDebug("Bắt đầu điền prompt vào ô nhập liệu: '%s'...", req.Prompt)
		tm.EmitStatus(TaskStateSubmitting, "Đang điền prompt...", 35)
		err = FillFlowPrompt(page, promptInput, req.Prompt)
		if err != nil {
			logDebug("Lỗi khi điền prompt: %v", err)
			finalErr = fmt.Errorf("fill prompt: %w", err)
			continue
		}
		logDebug("Đã điền prompt xong thành công!")
		sleep(1000 * time.Millisecond)

		// Cập nhật lại danh sách tất cả URL ảnh đang có trên trang NGAY TRƯỚC KHI BẤM NÚT GỬI PROMPT
		// (bao gồm cả URL của ảnh vừa dán vào) để chắc chắn KHÔNG nhầm ảnh upload với ảnh do AI vừa tạo ra!
		lastUrlsObj, errLast := page.Eval(`() => {
			const imgs = Array.from(document.querySelectorAll('img')).filter(img => {
				const src = img.getAttribute('src') || '';
				return src.includes('getMediaUrl') || src.includes('/fx/api/');
			});
			return imgs.map(img => {
				let src = img.getAttribute('src') || '';
				if (src.startsWith('/')) {
					src = 'https://labs.google' + src;
				}
				return src;
			}).filter(src => src !== '');
		}`)
		if errLast == nil && lastUrlsObj != nil {
			lastGroupUrls = nil
			for _, v := range lastUrlsObj.Value.Arr() {
				lastGroupUrls = append(lastGroupUrls, v.Str())
			}
		}
		logDebug("Cập nhật lại danh sách URL ảnh hiện có trước khi bấm Gửi (%d ảnh): %v", len(lastGroupUrls), lastGroupUrls)

		logDebug("Bắt đầu thực hiện gửi prompt...")
		tm.EmitStatus(TaskStateSubmitting, "Đang gửi prompt...", 40)

		// 1. Try submitting by pressing Enter key on the keyboard
		logDebug("Thử gửi bằng phím Enter ảo...")
		_ = page.Keyboard.Press('\r')
		sleep(500 * time.Millisecond)

		if isPromptSent() {
			logDebug("Đã gửi prompt thành công qua phím Enter ảo!")
		} else {
			// 2. Try dispatching keydown Enter event via JS on the input element
			logDebug("Thử gửi bằng sự kiện keydown Enter (JS)...")
			_, errEv := promptInput.Eval(`function() {
				const ev = new KeyboardEvent('keydown', {
					key: 'Enter',
					code: 'Enter',
					keyCode: 13,
					which: 13,
					bubbles: true,
					cancelable: true
				});
				this.dispatchEvent(ev);
			}`)
			if errEv != nil {
				logDebug("Lỗi dispatch event Enter: %v", errEv)
			}
			sleep(800 * time.Millisecond)

			if isPromptSent() {
				logDebug("Đã gửi prompt thành công qua sự kiện keydown Enter!")
			} else {
				// 3. Fallback: find and click the physical submit button using robust scoped icon detection
				logDebug("Bắt đầu quét tìm nút gửi bằng JS...")
				submitBtn, errSubmit := promptInput.ElementByJS(rod.Eval(`function() {
					let parent = this.parentElement;
					while (parent && parent.tagName !== 'BODY') {
						const buttons = Array.from(parent.querySelectorAll('button'));
						if (buttons.length > 0) {
							const matched = buttons.find(btn => {
								const html = btn.innerHTML.toLowerCase();
								const aria = (btn.getAttribute('aria-label') || '').toLowerCase();
								const title = (btn.getAttribute('title') || '').toLowerCase();
								return aria.includes('gửi') || aria.includes('send') || aria.includes('submit') ||
								       title.includes('gửi') || title.includes('send') || title.includes('submit') ||
								       html.includes('arrow_upward') || html.includes('arrow_forward') ||
								       (btn.querySelector('svg') && (aria.includes('gửi') || aria.includes('send') || html.includes('path') || html.includes('svg')));
							});
							if (matched) return matched;
							
							const visibleButtons = buttons.filter(btn => btn.getBoundingClientRect().width > 0);
							if (visibleButtons.length > 0) {
								return visibleButtons[visibleButtons.length - 1];
							}
						}
						parent = parent.parentElement;
					}
					return null;
				}`))

				if errSubmit == nil && submitBtn != nil {
					logDebug("Tiến hành click nút gửi...")
					_ = submitBtn.Click(proto.InputMouseButtonLeft, 1)
					sleep(1000 * time.Millisecond)
				} else {
					logDebug("Phương án quét JS thất bại. Thử dùng danh sách Selectors dự phòng...")
					submitBtnFb, errFb := FindFirstVisible(ctx, page, FlowSelectors.SubmitButtons, 3*time.Second)
					if errFb == nil && submitBtnFb != nil {
						_ = submitBtnFb.Click(proto.InputMouseButtonLeft, 1)
						sleep(1000 * time.Millisecond)
					} else if errFb != nil {
						logDebug("Lỗi tìm nút gửi dự phòng: %v", errFb)
					}
				}
			}
		}

		generationFailed := false

		if req.MediaType == MediaTypeImage {
			tm.EmitStatus(TaskStateGenerating, "Google AI đang tạo hình ảnh...", 50)
			
			var previewBase64s []string
			tickerImg := time.NewTicker(2 * time.Second)
			defer tickerImg.Stop()
			
			deadlineImg := time.Now().Add(10 * time.Minute)
			pollCount := 0
			
			for {
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-tickerImg.C:
					pollCount++
				}
				
				if pollCount % 3 == 1 {
					logDebug("Đang chờ Google AI tạo xong hình ảnh... (Đã chờ %d giây)", pollCount*2)
				}
				
				if time.Now().After(deadlineImg) {
					finalErr = NewError(ErrGenerationTimeout, "Quá thời gian chờ tạo hình ảnh từ Google Flow.")
					generationFailed = true
					break
				}

				// Check for quota warning on page
				quotaObj, _ := page.Eval(`() => {
					const limitWords = [
						'đạt đến hạn mức',
						'hạn mức sử dụng',
						'quay lại vào ngày mai',
						'tool agent usage limit',
						'reached your limit',
						'quota exceeded'
					];
					const elements = Array.from(document.querySelectorAll('div, span, p'));
					let matchedEl = null;
					for (const el of elements) {
						const txt = el.textContent.trim().toLowerCase();
						const isVisible = el.getBoundingClientRect().width > 0;
						if (isVisible && limitWords.some(word => txt.includes(word))) {
							const hasChildMatch = Array.from(el.children).some(child => {
								const childVisible = child.getBoundingClientRect().width > 0;
								const childTxt = child.textContent.toLowerCase();
								return childVisible && limitWords.some(word => childTxt.includes(word));
							});
							if (!hasChildMatch) {
								matchedEl = el;
								break;
							}
						}
					}
					if (matchedEl) {
						return matchedEl.textContent.trim();
					}
					return "";
				}`)
				if quotaObj != nil && quotaObj.Value.Str() != "" && quotaObj.Value.Str() != "<nil>" {
					quotaMsg := quotaObj.Value.Str()
					logDebug("Phát hiện cảnh báo giới hạn hạn mức từ Google Flow: %s", quotaMsg)
					return nil, NewError(ErrQuotaExceeded, quotaMsg)
				}

				// Check for error card or policy violation card via page.Eval JS
				isError, _ := page.Eval(`() => {
					const errorKeywords = [
						'không thành công',
						'vi phạm các chính sách',
						'vi phạm chính sách',
						'policy violation',
						'lượt tạo này có thể vi phạm',
						'thử một câu lệnh khác',
						'something went wrong',
						'rất tiếc, đã xảy ra lỗi',
						'failed to generate',
						'unable to generate'
					];
					return Array.from(document.querySelectorAll('div, span, button, p')).some(el => {
						const txt = el.textContent.toLowerCase();
						const isVisible = el.getBoundingClientRect().width > 0;
						return isVisible && errorKeywords.some(kw => txt.includes(kw));
					});
				}`)
				if isError != nil && isError.Value.Bool() {
					logDebug("Phát hiện thẻ báo lỗi / vi phạm chính sách tạo ảnh trên Google Flow. Đang kích hoạt thử lại tự động (Tối đa 3 lần)...")
					finalErr = NewError(ErrGenerationFailed, "Google Flow báo lỗi / vi phạm chính sách tạo hình ảnh.")
					generationFailed = true
					break
				}
				
				// Quét tất cả các ảnh mới vừa sinh ra ở đầu danh sách (chưa có trong lastGroupUrls)
				currentUrlsObj, errCurr := page.Eval(`(lastGroupUrls) => {
					const imgs = Array.from(document.querySelectorAll('img')).filter(img => {
						const src = img.getAttribute('src') || '';
						return src.includes('getMediaUrl') || src.includes('/fx/api/');
					});
					if (imgs.length === 0) return [];
					const lastSet = new Set(lastGroupUrls || []);
					const newUrls = [];
					for (const img of imgs) {
						let src = img.getAttribute('src') || '';
						if (src.startsWith('/')) {
							src = 'https://labs.google' + src;
						}
						if (lastSet.size > 0 && lastSet.has(src)) {
							break; // Đã chạm đến ảnh cũ có từ trước -> Dừng!
						}
						newUrls.push(src);
					}
					if (newUrls.length === 0 && imgs.length > 0 && lastSet.size === 0) {
						return imgs.map(img => {
							let src = img.getAttribute('src') || '';
							return src.startsWith('/') ? 'https://labs.google' + src : src;
						});
					}
					return newUrls;
				}`, lastGroupUrls)

				if errCurr == nil && currentUrlsObj != nil {
					var currentUrls []string
					for _, v := range currentUrlsObj.Value.Arr() {
						currentUrls = append(currentUrls, v.Str())
					}

					// So sánh nhóm ảnh mới ở vị trí đầu tiên [0] với nhóm ảnh trước khi tạo
					isNew := false
					if len(lastGroupUrls) == 0 {
						if len(currentUrls) > 0 {
							isNew = true
						}
					} else {
						if len(currentUrls) > 0 && currentUrls[0] != lastGroupUrls[0] {
							isNew = true
						}
					}

					if isNew {
						logDebug("Phát hiện nhóm ảnh mới đã sinh xong! Số lượng mới sinh: %d ảnh. URL: %v", len(currentUrls), currentUrls)
						sleep(2000 * time.Millisecond) // Chờ thêm 2 giây để ảnh load hoàn toàn
						
						// Đọc lại danh sách URL chính xác nhất sau khi đã load xong
						currentUrlsObjSec, errCurrSec := page.Eval(`(lastGroupUrls) => {
							const imgs = Array.from(document.querySelectorAll('img')).filter(img => {
								const src = img.getAttribute('src') || '';
								return src.includes('getMediaUrl') || src.includes('/fx/api/');
							});
							if (imgs.length === 0) return [];
							const lastSet = new Set(lastGroupUrls || []);
							const newUrls = [];
							for (const img of imgs) {
								let src = img.getAttribute('src') || '';
								if (src.startsWith('/')) {
									src = 'https://labs.google' + src;
								}
								if (lastSet.size > 0 && lastSet.has(src)) {
									break;
								}
								newUrls.push(src);
							}
							if (newUrls.length === 0 && imgs.length > 0 && lastSet.size === 0) {
								return imgs.map(img => {
									let src = img.getAttribute('src') || '';
									return src.startsWith('/') ? 'https://labs.google' + src : src;
								});
							}
							return newUrls;
						}`, lastGroupUrls)
						if errCurrSec == nil && currentUrlsObjSec != nil {
							newGroupUrls := []string{}
							for _, v := range currentUrlsObjSec.Value.Arr() {
								newGroupUrls = append(newGroupUrls, v.Str())
							}
							logDebug("Tìm thấy nhóm ảnh mới sinh thành công! Số lượng: %d", len(newGroupUrls))
							
							// Kiểm tra tổng số ảnh trên toàn trang để tránh lag
							totalCardsCountObj, errTotalCount := page.Eval(`() => {
								const imgs = Array.from(document.querySelectorAll('img')).filter(img => {
									const src = img.getAttribute('src') || '';
									return src.includes('getMediaUrl') || src.includes('/fx/api/');
								});
								return imgs.length;
							}`)
							if errTotalCount == nil && totalCardsCountObj != nil {
								totalCount := totalCardsCountObj.Value.Int()
								logDebug("Tổng số lượng hình ảnh đang có trong dự án: %d", totalCount)
								if totalCount >= 50 {
									logDebug("Dự án hiện tại có %d hình ảnh (vượt ngưỡng 50 ảnh để tránh lag Chrome). Tiến hành xóa link dự án cũ để lần sau tự động tạo dự án mới...", totalCount)
									saveProjectURL("") // Xóa link project để lần sau tạo dự án mới tinh!
								}
							}

							// Bắt đầu xử lý preview: Đảm bảo có tiền tố https://labs.google
							var directUrls []string
							for _, src := range newGroupUrls {
								tryUrl := src
								if strings.HasPrefix(src, "/") {
									tryUrl = "https://labs.google" + src
								} else if !strings.HasPrefix(src, "http") {
									tryUrl = "https://labs.google/" + strings.TrimPrefix(src, "/")
								}
								directUrls = append(directUrls, tryUrl)
							}

							var base64s []string
							for idx, urlStr := range directUrls {
								logDebug("Đang chuyển đổi ảnh %d/%d sang Base64 (URL: %s)...", idx+1, len(directUrls), urlStr)
								base64Obj, errFetch := page.Eval(`async (url) => {
									try {
										const response = await fetch(url);
										const blob = await response.blob();
										return new Promise((resolve) => {
											const reader = new FileReader();
											reader.onloadend = () => resolve(reader.result);
											reader.readAsDataURL(blob);
										});
									} catch (e) {
										return "";
									}
								}`, urlStr)
								if errFetch == nil && base64Obj != nil && base64Obj.Value.Str() != "" {
									base64s = append(base64s, base64Obj.Value.Str())
								} else {
									logDebug("Fetch Base64 thất bại cho ảnh %d, sử dụng link direct làm fallback: %s", idx+1, urlStr)
									base64s = append(base64s, urlStr)
								}
							}
							
							previewBase64s = base64s
							break
						}
					}
				}
			}

			if generationFailed {
				continue
			}

			if len(previewBase64s) == 0 {
				finalErr = fmt.Errorf("không tìm thấy hình ảnh xem trước được sinh ra")
				continue
			}

			// Gửi preview về frontend và chờ người dùng chọn (Nếu là tác vụ tự động hàng đợi thì tự động chọn)
			taskID := "temp"
			if active := tm.GetActiveTask(); active != nil {
				taskID = active.ID
			}

			var selectedIndexes []int
			if req.ConfirmBeforeCreate == "auto" {
				logDebug("Tác vụ tự động từ Hàng Đợi (ConfirmBeforeCreate=auto): Tự động chọn tải ảnh...")
				for i := 0; i < len(previewBase64s); i++ {
					selectedIndexes = append(selectedIndexes, i)
				}
			} else {
				logDebug("Đang gửi yêu cầu chọn ảnh lên UI...")
				tm.EmitSelectionRequired(taskID, previewBase64s)

				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case selectedIndexes = <-tm.GetActiveTask().SelectionChan:
					logDebug("Đã nhận được danh sách ảnh được chọn từ UI: %v", selectedIndexes)
				}
			}

			if len(selectedIndexes) == 0 {
				logDebug("Người dùng không chọn ảnh nào để tải.")
				return nil, fmt.Errorf("không có hình ảnh nào được chọn để tải")
			}

			tm.EmitStatus(TaskStateDownloading, "Đang tải các ảnh đã chọn...", 85)

			var downloadedPaths []string
			for idx, selIdx := range selectedIndexes {
				logDebug("Đang tải hình ảnh được chọn thứ %d/%d (chỉ mục trong nhóm: %d)...", idx+1, len(selectedIndexes), selIdx)

				// Click the image card at index `selIdx` among the newly generated images
				clickedObj, errClick := page.Eval(`(selIdx) => {
					const imgs = Array.from(document.querySelectorAll('img')).filter(img => {
						const src = img.getAttribute('src') || '';
						return src.includes('getMediaUrl') || src.includes('/fx/api/');
					});
					if (selIdx < imgs.length) {
						const img = imgs[selIdx];
						const card = img.closest('a, button, [data-tile-id], [role="button"]') || img;
						card.scrollIntoView({ block: 'center' });
						card.click();
						return true;
					}
					return false;
				}`, selIdx)

				if errClick != nil || clickedObj == nil || !clickedObj.Value.Bool() {
					logDebug("Không thể click vào ảnh thứ %d ở chỉ mục %d, bỏ qua", idx+1, selIdx)
					continue
				}

				sleep(1500 * time.Millisecond) // wait for overlay to open fully
				
				// 3. Find the download dropdown button in the detail overlay
				dlBtn, errDlBtn := page.ElementByJS(rod.Eval(`() => {
					const buttons = Array.from(document.querySelectorAll('button'));
					return buttons.find(el => {
						const icon = el.querySelector('i');
						const isVisible = el.getBoundingClientRect().width > 0;
						return isVisible && icon && icon.textContent.trim() === 'download';
					});
				}`))
				if errDlBtn != nil || dlBtn == nil {
					logDebug("Không tìm thấy nút tải xuống cho ảnh thứ %d, bỏ qua", idx+1)
					// Try to close overlay using exact text search
					_, _ = page.Eval(`() => {
						const closeBtn = Array.from(document.querySelectorAll('button')).find(el => {
							const txt = el.textContent.toLowerCase().trim();
							const isVisible = el.getBoundingClientRect().width > 0;
							return isVisible && (txt === 'xong' || txt === 'đóng' || txt === 'close' || txt === 'done');
						});
						if (closeBtn) closeBtn.click();
					}`)
					sleep(1200 * time.Millisecond)
					continue
				}
				
				err = dlBtn.Click(proto.InputMouseButtonLeft, 1)
				if err != nil {
					logDebug("Lỗi click nút tải xuống: %v", err)
					continue
				}
				
				sleep(1200 * time.Millisecond) // wait for dropdown to open
				
				// Get target resolution from request
				targetRes := strings.ToLower(req.Resolution)
				if targetRes == "" {
					targetRes = "1k" // default to 1K
				}

				logDebug("Đang chọn độ phân giải %s cho ảnh thứ %d/%d...", strings.ToUpper(targetRes), idx+1, len(selectedIndexes))

				// Click resolution button (1K, 2K, 4K) in dropdown
				optionBtn, errOpt := page.ElementByJS(rod.Eval(`(target) => {
					const items = Array.from(document.querySelectorAll('button, div[role="menuitem"], div[role="button"]'));
					return items.find(el => {
						const txt = el.textContent.toLowerCase();
						const isVisible = el.getBoundingClientRect().width > 0;
						return isVisible && txt.includes(target);
					});
				}`, targetRes))
				
				if errOpt != nil || optionBtn == nil {
					logDebug("Không tìm thấy tùy chọn độ phân giải %s cho ảnh thứ %d, thử tìm tùy chọn 1K mặc định...", targetRes, idx+1)
					optionBtn, _ = page.ElementByJS(rod.Eval(`() => {
						const items = Array.from(document.querySelectorAll('button, div[role="menuitem"]'));
						return items.find(el => el.getBoundingClientRect().width > 0 && el.textContent.toLowerCase().includes('1k'));
					}`))
				}
				
				if optionBtn == nil {
					logDebug("Không thể chọn độ phân giải cho ảnh thứ %d, bỏ qua", idx+1)
					_, _ = page.Eval(`() => {
						const closeBtn = Array.from(document.querySelectorAll('button')).find(el => {
							const txt = el.textContent.toLowerCase().trim();
							const isVisible = el.getBoundingClientRect().width > 0;
							return isVisible && (txt === 'xong' || txt === 'đóng' || txt === 'close' || txt === 'done');
						});
						if (closeBtn) closeBtn.click();
					}`)
					sleep(1200 * time.Millisecond)
					continue
				}

				// Initiate Wails download capturing BEFORE clicking option
				waitDownload := session.browser.WaitDownload(session.downloadDir)
				
				// Click resolution option to trigger download / upscaling
				_ = optionBtn.Click(proto.InputMouseButtonLeft, 1)
				logDebug("Đã click chọn độ phân giải %s. Đang chờ file được tải về máy...", strings.ToUpper(targetRes))
				
				// Generate unique filename for each image in batch
				customFileName := req.FileName
				if len(selectedIndexes) > 1 {
					customFileName = fmt.Sprintf("%s_%d", req.FileName, idx+1)
				}
				
				filePath, errMove := WaitAndMoveDownload(ctx, waitDownload, session.downloadDir, req.OutputDir, customFileName, MediaTypeImage)
				if errMove == nil {
					logDebug("Tải ảnh thứ %d/%d thành công (%s): %s", idx+1, len(selectedIndexes), strings.ToUpper(targetRes), filePath)
					downloadedPaths = append(downloadedPaths, filePath)
				} else {
					logDebug("Lỗi khi tải/lưu ảnh %d: %v", idx+1, errMove)
				}
				
				// Close the detail overlay
				_, _ = page.Eval(`() => {
					const closeBtn = Array.from(document.querySelectorAll('button')).find(el => {
						const txt = el.textContent.toLowerCase().trim();
						const isVisible = el.getBoundingClientRect().width > 0;
						return isVisible && (txt === 'xong' || txt === 'đóng' || txt === 'close' || txt === 'done');
					});
					if (closeBtn) {
						closeBtn.click();
						return;
					}
					const event = new KeyboardEvent('keydown', { key: 'Escape', code: 'Escape', keyCode: 27, which: 27, bubbles: true });
					document.dispatchEvent(event);
				}`)
				sleep(1500 * time.Millisecond)
			}
			
			if len(downloadedPaths) > 0 {
				return downloadedPaths, nil
			}
			finalErr = fmt.Errorf("không có hình ảnh nào được tải về thành công")
		} else {
			// Video generation and download flow
			var downloadBtn *rod.Element
			ticker := time.NewTicker(3 * time.Second)
			defer ticker.Stop()

			// Timeout for video generation is 20 minutes
			deadline := time.Now().Add(20 * time.Minute)

			for {
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-ticker.C:
				}

				if time.Now().After(deadline) {
					finalErr = NewError(ErrGenerationTimeout, "Quá thời gian chờ tạo video từ Google Flow.")
					generationFailed = true
					break
				}

				// Check for quota warning on page
				quotaObj, _ := page.Eval(`() => {
					const limitWords = [
						'đạt đến hạn mức',
						'hạn mức sử dụng',
						'quay lại vào ngày mai',
						'tool agent usage limit',
						'reached your limit',
						'quota exceeded'
					];
					const elements = Array.from(document.querySelectorAll('div, span, p'));
					let matchedEl = null;
					for (const el of elements) {
						const txt = el.textContent.trim().toLowerCase();
						const isVisible = el.getBoundingClientRect().width > 0;
						if (isVisible && limitWords.some(word => txt.includes(word))) {
							// Check if this is the deepest leaf element containing the warning text
							const hasChildMatch = Array.from(el.children).some(child => {
								const childVisible = child.getBoundingClientRect().width > 0;
								const childTxt = child.textContent.toLowerCase();
								return childVisible && limitWords.some(word => childTxt.includes(word));
							});
							if (!hasChildMatch) {
								matchedEl = el;
								break;
							}
						}
					}
					if (matchedEl) {
						return matchedEl.textContent.trim();
					}
					return "";
				}`)
				if quotaObj != nil && quotaObj.Value.Str() != "" && quotaObj.Value.Str() != "<nil>" {
					quotaMsg := quotaObj.Value.Str()
					logDebug("Phát hiện cảnh báo giới hạn hạn mức từ Google Flow: %s", quotaMsg)
					return nil, NewError(ErrQuotaExceeded, quotaMsg)
				}

				// Check for quota or refusal errors
				errEl, errCheckErr := FindFirstVisible(ctx, page, FlowSelectors.ErrorMarkers, 500*time.Millisecond)
				if errCheckErr == nil && errEl != nil {
					txt, _ := errEl.Text()
					if strings.Contains(strings.ToLower(txt), "quota") || strings.Contains(strings.ToLower(txt), "limit") {
						return nil, NewError(ErrQuotaExceeded, "Tài khoản đạt giới hạn tạo video của Google FX: "+txt)
					}
					if strings.Contains(strings.ToLower(txt), "cannot") || strings.Contains(strings.ToLower(txt), "policy") {
						return nil, NewError(ErrGenerationRejected, "Yêu cầu bị từ chối do chính sách nội dung: "+txt)
					}
				}

				// Check for general error card
				isError, _ := page.Eval(`() => {
					return Array.from(document.querySelectorAll('div, span, button')).some(el => {
						const txt = el.textContent.toLowerCase();
						const isVisible = el.getBoundingClientRect().width > 0;
						return isVisible && (
							(txt.includes('không thành công') && txt.includes('lỗi')) ||
							txt.includes('something went wrong') ||
							txt.includes('rất tiếc, đã xảy ra lỗi') ||
							(txt.includes('failed') && txt.includes('create'))
						);
					});
				}`)
				if isError != nil && isError.Value.Bool() {
					logDebug("Phát hiện thẻ lỗi trên Google Flow (Không thành công / Rất tiếc, đã xảy ra lỗi) khi tạo video.")
					finalErr = NewError(ErrGenerationFailed, "Google Flow báo lỗi tạo video.")
					generationFailed = true
					break
				}

				// Try to hover on media element to reveal download button
				hoverEl, errHover := page.ElementByJS(rod.Eval(`() => {
					return Array.from(document.querySelectorAll('img, video')).find(el => {
						const rect = el.getBoundingClientRect();
						return rect.width > 100 && rect.height > 100;
					});
				}`))
				if errHover == nil && hoverEl != nil {
					_ = hoverEl.Hover()
				}

				// Check if new download button appears
				currentDlCount := 0
				hasDlCount, errDlCount := page.Eval(`() => {
					const candidates = Array.from(document.querySelectorAll('button, a'));
					let count = 0;
					for (const el of candidates) {
						const icon = el.querySelector('i, span.google-symbols, .material-icons, [class*="icon"]');
						const iconText = icon ? icon.textContent.trim().toLowerCase() : '';
						const aria = (el.getAttribute('aria-label') || '').toLowerCase();
						const title = (el.getAttribute('title') || '').toLowerCase();
						if (iconText === 'download' || iconText === 'file_download' || 
						    aria.includes('download') || aria.includes('tải xuống') || aria.includes('tải về') ||
						    title.includes('download') || title.includes('tải xuống') || title.includes('tải về')) {
							count++;
						}
					}
					return count;
				}`)
				if errDlCount == nil && hasDlCount != nil {
					currentDlCount = hasDlCount.Value.Int()
				}

				if currentDlCount > initialVideoButtonsCount {
					btn, errCheckDl := page.ElementByJS(rod.Eval(`() => {
						const candidates = Array.from(document.querySelectorAll('button, a'));
						const found = [];
						for (const el of candidates) {
							const icon = el.querySelector('i, span.google-symbols, .material-icons, [class*="icon"]');
							const iconText = icon ? icon.textContent.trim().toLowerCase() : '';
							const aria = (el.getAttribute('aria-label') || '').toLowerCase();
							const title = (el.getAttribute('title') || '').toLowerCase();
							if (iconText === 'download' || iconText === 'file_download' || 
							    aria.includes('download') || aria.includes('tải xuống') || aria.includes('tải về') ||
							    title.includes('download') || title.includes('tải xuống') || title.includes('tải về')) {
								found.push(el);
							}
						}
						return found.length > 0 ? found[found.length - 1] : null;
					}`))
					if errCheckDl == nil && btn != nil {
						downloadBtn = btn
						logDebug("Tìm thấy nút tải video mới xuất hiện!")
						break
					}
				}
			}

			if generationFailed {
				continue
			}

			// Download video
			tm.EmitStatus(TaskStateDownloading, "Đang tải video xuống...", 85)
			waitDownload := session.browser.WaitDownload(session.downloadDir)
			_ = downloadBtn.Click(proto.InputMouseButtonLeft, 1)

			filePath, errMove := WaitAndMoveDownload(ctx, waitDownload, session.downloadDir, req.OutputDir, req.FileName, MediaTypeVideo)
			if errMove == nil {
				logDebug("Tải video thành công: %s", filePath)
				return []string{filePath}, nil
			}
			finalErr = fmt.Errorf("lỗi di chuyển file video tải về: %w", errMove)
		}
	}

	if finalErr != nil {
		return nil, finalErr
	}
	return nil, fmt.Errorf("quá trình tạo nội dung Google AI không thành công sau %d lần thử", maxAttempts)
}

func ConfigureFlowSettings(ctx context.Context, page *rod.Page, req GenerateRequest, tm *TaskManager) error {
	logDebug := func(msg string, args ...interface{}) {
		formatted := fmt.Sprintf(msg, args...)
		fmt.Println("[FlowDebug]", formatted)
		
		f, err := os.OpenFile("d:\\CongTy\\ToolCatVideoNgan\\flow_submit_debug.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
		if err == nil {
			defer f.Close()
			_, _ = f.WriteString(time.Now().Format("2006-01-02 15:04:05") + " - " + formatted + "\n")
		}
	}

	// Lấy hệ số delay multiplier từ yêu cầu của người dùng (ví dụ Độ trễ: 2 giây)
	delayMult := req.DelaySecond
	if delayMult <= 0 {
		delayMult = 1.0
	}

	// Helper sleep sử dụng hệ số delay của người dùng
	sleep := func(base time.Duration) {
		time.Sleep(time.Duration(float64(base) * delayMult))
	}

	// Helper lấy element có timeout tỷ lệ thuận với hệ số delay
	getElement := func(timeout time.Duration, js string, args ...interface{}) (*rod.Element, error) {
		adjustedTimeout := time.Duration(float64(timeout) * delayMult)
		subCtx, cancel := context.WithTimeout(ctx, adjustedTimeout)
		defer cancel()
		el, err := page.Context(subCtx).ElementByJS(rod.Eval(js, args...))
		if err != nil {
			return nil, err
		}
		return el.Context(ctx), nil
	}

	checkConfigMatches := func(btnText string) (isModelMatch, isRatioMatch, isBatchMatch bool) {
		btnTextLower := strings.ToLower(btnText)

		// 1. Kiểm tra Model
		isModelMatch = true
		if req.Model != "" {
			targetModel := strings.ToLower(req.Model)
			displayModel := targetModel
			if targetModel == "imagen 3" {
				displayModel = "nano banana 2"
			} else if strings.Contains(targetModel, "quality") || strings.Contains(targetModel, "pro") {
				displayModel = "nano banana pro"
			} else if strings.Contains(targetModel, "fast") || strings.Contains(targetModel, "lite") {
				displayModel = "nano banana 2 lite"
			}

			if strings.Contains(displayModel, "pro") {
				if !strings.Contains(btnTextLower, "pro") {
					isModelMatch = false
				}
			} else if strings.Contains(displayModel, "lite") {
				if !strings.Contains(btnTextLower, "lite") {
					isModelMatch = false
				}
			} else {
				if strings.Contains(btnTextLower, "pro") || strings.Contains(btnTextLower, "lite") {
					isModelMatch = false
				}
			}

			family := ""
			if strings.Contains(displayModel, "banana") {
				family = "banana"
			} else if strings.Contains(displayModel, "veo") {
				family = "veo"
			} else if strings.Contains(displayModel, "imagen") {
				family = "imagen"
			} else if strings.Contains(displayModel, "omni") {
				family = "omni"
			}
			if family != "" && !strings.Contains(btnTextLower, family) {
				isModelMatch = false
			}
		}

		// 2. Kiểm tra Aspect Ratio
		isRatioMatch = true
		if req.AspectRatio != "" {
			ratio := req.AspectRatio
			cleanRatio := strings.ReplaceAll(ratio, ":", "_")
			
			if ratio == "16:9" {
				isRatioMatch = strings.Contains(btnTextLower, "16_9") || strings.Contains(btnTextLower, "16:9") || strings.Contains(btnTextLower, "landscape")
			} else if ratio == "9:16" {
				isRatioMatch = strings.Contains(btnTextLower, "9_16") || strings.Contains(btnTextLower, "9:16") || strings.Contains(btnTextLower, "portrait")
			} else if ratio == "1:1" {
				isRatioMatch = strings.Contains(btnTextLower, "1_1") || strings.Contains(btnTextLower, "1:1") || strings.Contains(btnTextLower, "square") || strings.Contains(btnTextLower, "din")
			} else {
				isRatioMatch = strings.Contains(btnTextLower, ratio) || strings.Contains(btnTextLower, cleanRatio)
			}
		}

		// 3. Kiểm tra Batch Size (Khớp chính xác dạng x2, 2x, x 2)
		isBatchMatch = true
		if req.BatchSize != "" {
			batchDigit := strings.ToLower(strings.TrimSpace(strings.ReplaceAll(req.BatchSize, "x", "")))
			if strings.Contains(btnTextLower, "x"+batchDigit) || strings.Contains(btnTextLower, batchDigit+"x") || strings.Contains(btnTextLower, "x "+batchDigit) {
				isBatchMatch = true
			} else {
				isBatchMatch = false
			}
		}

		return isModelMatch, isRatioMatch, isBatchMatch
	}

	// A. Đảm bảo nút "Tác nhân" (Agent) được tắt trước tiên (chuyển aria-pressed="true" thành "false")
	logDebug("Kiểm tra trạng thái nút Tác nhân (Agent)...")
	
	queryAgentBtn := func() (*rod.Element, error) {
		return getElement(3*time.Second, `() => {
			const buttons = Array.from(document.querySelectorAll('button'));
			
			// 1. Tìm nút có chứa text "tác nhân"/"agent" và có thuộc tính "aria-pressed"
			let btn = buttons.find(b => {
				const txt = b.textContent.toLowerCase().trim();
				const hasAgentText = txt === 'tác nhân' || txt === 'agent' || (txt.includes('tác nhân') && !txt.includes('hướng dẫn'));
				const hasAriaPressed = b.hasAttribute('aria-pressed');
				const isVisible = b.getBoundingClientRect().width > 0;
				return isVisible && hasAgentText && hasAriaPressed;
			});
			if (btn) return btn;

			// 2. Dự phòng: Tìm nút có text chính xác (Tác nhân / Agent)
			btn = buttons.find(b => {
				const txt = b.textContent.toLowerCase().trim();
				const isVisible = b.getBoundingClientRect().width > 0;
				return isVisible && (txt === 'tác nhân' || txt === 'agent');
			});
			if (btn) return btn;

			// 3. Dự phòng tiếp theo: Thử tìm theo các XPath cụ thể
			const xpaths = [
				"//*[@id='__next']/div[1]/div[5]/div/div/div/div/div[2]/div[1]/div/button[2]",
				"//*[@id='__next']/div[1]/div[4]/div[2]/div[2]/div/div/div[2]/div[2]/div/div[2]/div[1]/div/button[2]"
			];
			for (const xpath of xpaths) {
				try {
					const result = document.evaluate(xpath, document, null, XPathResult.FIRST_ORDERED_NODE_TYPE, null);
					const el = result.singleNodeValue;
					if (el && el.getBoundingClientRect().width > 0) return el;
				} catch(e) {}
			}

			return null;
		}`)
	}

	agentBtn, errAgent := queryAgentBtn()

	if errAgent == nil && agentBtn != nil {
		if htmlSnippet, errHtml := agentBtn.HTML(); errHtml == nil {
			logDebug("Đã tìm thấy nút Tác nhân! HTML: %s", htmlSnippet)
		} else {
			logDebug("Đã tìm thấy nút Tác nhân nhưng không lấy được HTML: %v", errHtml)
		}
		// Đọc trạng thái aria-pressed qua JS với vòng lặp chờ tối đa 6 lần (tổng ~1.8s) để tránh race condition khi React đang tải
		var isPressed bool
		for i := 0; i < 6; i++ {
			isPressedObj, errPressed := agentBtn.Eval("function() { return this.getAttribute('aria-pressed') === 'true'; }")
			if errPressed != nil {
				logDebug("Vòng %d: Lỗi khi kiểm tra aria-pressed: %v", i+1, errPressed)
			}
			isPressed = errPressed == nil && isPressedObj != nil && isPressedObj.Value.Bool()
			if isPressed {
				logDebug("Nút Tác nhân đang hoạt động (phát hiện ở vòng lặp %d, aria-pressed=true).", i+1)
				break
			}
			sleep(300 * time.Millisecond)
		}
		
		if isPressed {
			logDebug("Tiến hành click để TẮT nút Tác nhân...")
			_, errClick := agentBtn.Eval("function() { this.click(); }")
			if errClick != nil {
				logDebug("Lỗi khi click tắt nút Tác nhân: %v", errClick)
			}
			sleep(800 * time.Millisecond) // Chờ trạng thái cập nhật
			
			// Kiểm tra lại sau khi click (Re-query để tránh dùng phải phần tử React cũ đã bị render lại / hủy bỏ khỏi DOM)
			if freshBtn, errFresh := queryAgentBtn(); errFresh == nil && freshBtn != nil {
				agentBtn = freshBtn
			}
			
			checkPressedObj, errCheck := agentBtn.Eval("function() { return this.getAttribute('aria-pressed') === 'true'; }")
			stillPressed := errCheck == nil && checkPressedObj != nil && checkPressedObj.Value.Bool()
			if stillPressed {
				logDebug("CẢNH BÁO: Đã click nhưng nút Tác nhân vẫn BẬT! Thử click lại lần 2...")
				_, _ = agentBtn.Eval("function() { this.click(); }")
				sleep(500 * time.Millisecond)
				
				// Re-query lần nữa
				if freshBtn, errFresh := queryAgentBtn(); errFresh == nil && freshBtn != nil {
					agentBtn = freshBtn
				}
				
				finalCheck, _ := agentBtn.Eval("function() { return this.getAttribute('aria-pressed') === 'true'; }")
				if finalCheck != nil && finalCheck.Value.Bool() {
					logDebug("LỖI: Không thể tắt nút Tác nhân.")
				} else {
					logDebug("Đã tắt nút Tác nhân thành công sau lần click thứ 2.")
				}
			} else {
				logDebug("Đã tắt nút Tác nhân thành công (aria-pressed=false).")
			}
		} else {
			logDebug("Nút Tác nhân đã ở trạng thái TẮT (aria-pressed=false) sau khi đã chờ React load. Bỏ qua.")
		}
	} else {
		logDebug("Không tìm thấy nút Tác nhân (Agent) trên giao diện hoặc gặp lỗi: %v", errAgent)
	}

	// 1. Kiểm tra xem có cần cấu hình gì thêm không
	if req.Model == "" && req.BatchSize == "" && req.AspectRatio == "" {
		logDebug("Không có yêu cầu cấu hình cài đặt thêm. Bỏ qua.")
		return nil
	}

	tm.EmitStatus(TaskStateSubmitting, "Đang cấu hình cài đặt tác nhân Google Flow...", 28)
	logDebug("Bắt đầu cấu hình cài đặt tác nhân (Model: %s, Aspect: %s, Batch: %s)...", req.Model, req.AspectRatio, req.BatchSize)

	// 2. Tìm nút mở hộp thoại cấu hình (radix-:rn: / nút hiển thị model hiện tại)
	configTriggerBtn, err := getElement(5*time.Second, `() => {
		const buttons = Array.from(document.querySelectorAll('button'));

		// 1. Tìm nút hiển thị Model hiện tại có aria-haspopup="menu" (chứa Banana, Veo, Imagen, Omni, hoặc biểu tượng 🍌)
		let btn = buttons.find(b => {
			const id = b.getAttribute('id') || '';
			const hasPopup = b.getAttribute('aria-haspopup') === 'menu';
			const txt = b.textContent.toLowerCase();
			const hasModelText = txt.includes('banana') || txt.includes('veo') || txt.includes('imagen') || txt.includes('omni') || txt.includes('🍌');
			const isVisible = b.getBoundingClientRect().width > 0;
			return isVisible && id.startsWith('radix-') && hasPopup && hasModelText;
		});
		if (btn) return btn;

		// 2. Dự phòng: Tìm nút cấu hình gần nhất với ô nhập prompt
		try {
			const editor = document.querySelector('[data-slate-editor="true"], div[role="textbox"], textarea');
			if (editor) {
				let parent = editor.parentElement;
				while (parent && parent.tagName !== 'BODY') {
					const btns = Array.from(parent.querySelectorAll('button[aria-haspopup="menu"]'));
					const target = btns.find(b => {
						const aria = (b.getAttribute('aria-label') || '').toLowerCase();
						const title = (b.getAttribute('title') || '').toLowerCase();
						const html = b.innerHTML.toLowerCase();
						return aria.includes('cài đặt') || aria.includes('settings') || aria.includes('tune') ||
						       title.includes('cài đặt') || title.includes('settings') ||
						       html.includes('tune') || html.includes('settings') || html.includes('slider') ||
						       b.querySelector('span.google-symbols')?.textContent.trim().toLowerCase() === 'tune' ||
						       b.querySelector('i')?.textContent.trim().toLowerCase() === 'tune';
					});
					if (target && target.getBoundingClientRect().width > 0) {
						return target;
					}
					parent = parent.parentElement;
				}
			}
		} catch(e) {}

		// 3. Dự phòng 2: Tìm theo class + role + text/icon đặc trưng trên toàn trang
		btn = buttons.find(b => {
			const id = b.getAttribute('id') || '';
			const ariaHasPopup = b.getAttribute('aria-haspopup');
			const aria = (b.getAttribute('aria-label') || '').toLowerCase();
			const title = (b.getAttribute('title') || '').toLowerCase();
			const html = b.innerHTML.toLowerCase();
			
			const isRadixMenu = id.startsWith('radix-') && ariaHasPopup === 'menu';
			const isSettings = aria.includes('cài đặt') || aria.includes('settings') || aria.includes('tune') ||
			                   title.includes('cài đặt') || title.includes('settings') ||
			                   html.includes('tune') || html.includes('settings') || html.includes('slider') ||
			                   b.querySelector('span.google-symbols')?.textContent.trim().toLowerCase() === 'tune' ||
			                   b.querySelector('i')?.textContent.trim().toLowerCase() === 'tune';

			return isRadixMenu && isSettings && b.getBoundingClientRect().width > 0;
		});
		return btn || null;
	}`)

	if err != nil || configTriggerBtn == nil {
		logDebug("Không tìm thấy nút mở cấu hình tác nhân: %v", err)
		return fmt.Errorf("không tìm thấy nút mở cấu hình tác nhân: %w", err)
	}

	var currentConfigText string
	var idStr string
	if configTriggerBtn != nil {
		idVal, _ := configTriggerBtn.Attribute("id")
		if idVal != nil {
			idStr = *idVal
		}
		currentConfigText, _ = configTriggerBtn.Text()
	}

	// 2.5. Kiểm tra chi tiết từng cài đặt trên nút
	isModelMatch, isRatioMatch, isBatchMatch := checkConfigMatches(currentConfigText)
	isAllMatch := isModelMatch && isRatioMatch && isBatchMatch
	logDebug("NÚT CẤU HÌNH ĐƯỢC TÌM THẤY: ID=%s, Text=[%s]", idStr, strings.ReplaceAll(currentConfigText, "\n", " "))
	logDebug("ĐỐI CHIẾU CẤU HÌNH: Model=%v, AspectRatio=%v, BatchSize=%v -> Tất cả trùng khớp: %v", isModelMatch, isRatioMatch, isBatchMatch, isAllMatch)

	if isAllMatch {
		logDebug("Cấu hình hiện tại ĐÃ TRÙNG KHỚP hoàn toàn với yêu cầu. BỎ QUA toàn bộ các bước mở cấu hình.")
		return nil
	}
	
	logDebug("Cấu hình chưa trùng khớp (Model: %v, Ratio: %v, Batch: %v). Tiến hành mở popover để điều chỉnh duy nhất các phần chưa đúng...", isModelMatch, isRatioMatch, isBatchMatch)

	// 3. Kiểm tra xem popover đang mở hay đóng
	isOpened := isFlowPopoverOpen(page)

	if !isOpened {
		logDebug("Mở popover cấu hình bằng cách click nút cài đặt (Human Click)...")
		errClick := HumanClick(page, configTriggerBtn)
		if errClick != nil {
			logDebug("Lỗi click chuột thật: %v. Thử click bằng JS...", errClick)
			_, _ = configTriggerBtn.Eval("function() { this.click(); }")
		}
		sleep(1500 * time.Millisecond) // Chờ popover hiển thị (sử dụng sleep tỉ lệ với delay của người dùng)
	}

	// 3.5. Cấu hình Loại Media (Hình ảnh / Video) tab trước tiên để tải đúng model/tỷ lệ
	isVideoType := req.MediaType == MediaTypeVideo
	targetTab := "IMAGE"
	if isVideoType {
		targetTab = "VIDEO"
	}
	
	logDebug("Cấu hình loại media tab: %s", targetTab)
	
	// Thử dump toàn bộ buttons để chuẩn đoán trước khi click
	dumpBefore, _ := page.Eval(`() => {
		return Array.from(document.querySelectorAll('button')).map(b => ({
			id: b.getAttribute('id') || '',
			ariaHasPopup: b.getAttribute('aria-haspopup') || '',
			ariaLabel: b.getAttribute('aria-label') || '',
			title: b.getAttribute('title') || '',
			text: b.textContent.trim(),
			class: b.getAttribute('class') || '',
			visible: b.getBoundingClientRect().width > 0
		}));
	}`)
	if dumpBefore != nil {
		logDebug("DUMP BUTTONS BEFORE CONFIG: %s", dumpBefore.Value.String())
	}

	clickTabBtn, errTab := getElement(3*time.Second, `(tab) => {
		// Tìm trực tiếp trên toàn trang vì các hậu tố này là duy nhất toàn cục khi popover mở
		const btnId = document.querySelector('button[id$="-trigger-' + tab + '"]');
		if (btnId) return btnId;
		
		// Tìm dự phòng theo text hiển thị trong popover đang mở
		const popover = document.querySelector('[data-state="open"][role="menu"], [data-state="open"]');
		if (popover) {
			const buttons = Array.from(popover.querySelectorAll('button'));
			return buttons.find(b => {
				const txt = b.textContent.trim().toLowerCase();
				if (tab === 'IMAGE') {
					return txt === 'hình ảnh' || txt === 'image';
				} else {
					return txt === 'video';
				}
			}) || null;
		}
		return null;
	}`, targetTab)

	if errTab == nil && clickTabBtn != nil {
		errClick := HumanClick(page, clickTabBtn)
		if errClick != nil {
			logDebug("Lỗi click tab bằng chuột: %v. Thử bằng JS...", errClick)
			_, _ = clickTabBtn.Eval("function() { this.click(); }")
		}
		sleep(600 * time.Millisecond)
	} else {
		logDebug("Không cấu hình được media tab: %v", errTab)
	}

	// 4. Cấu hình Aspect Ratio (Chỉ chỉnh nếu chưa đúng)
	if req.AspectRatio != "" && !isRatioMatch {
		logDebug("Cấu hình tỷ lệ khung hình: %s (chỉnh lại vì chưa trùng khớp)", req.AspectRatio)
		clickRatioBtn, errRatio := getElement(3*time.Second, `(ratio) => {
			let suffix = "";
			if (ratio === "16:9") suffix = "LANDSCAPE";
			else if (ratio === "4:3") suffix = "LANDSCAPE_4_3";
			else if (ratio === "1:1") suffix = "SQUARE";
			else if (ratio === "3:4") suffix = "PORTRAIT_3_4";
			else if (ratio === "9:16") suffix = "PORTRAIT";
			
			if (suffix) {
				const btn = document.querySelector('button[id$="-trigger-' + suffix + '"]');
				if (btn) return btn;
			}
			
			// Dự phòng tìm trong popover đang mở
			const popover = document.querySelector('[data-state="open"][role="menu"], [data-state="open"]');
			if (popover) {
				const buttons = Array.from(popover.querySelectorAll('button'));
				return buttons.find(b => b.textContent.trim() === ratio) || null;
			}
			return null;
		}`, req.AspectRatio)

		if errRatio == nil && clickRatioBtn != nil {
			errClick := HumanClick(page, clickRatioBtn)
			if errClick != nil {
				logDebug("Lỗi click ratio bằng chuột: %v. Thử bằng JS...", errClick)
				_, _ = clickRatioBtn.Eval("function() { this.click(); }")
			}
			sleep(500 * time.Millisecond)
		} else {
			logDebug("Không cấu hình được tỷ lệ khung hình: %v", errRatio)
		}
	} else if isRatioMatch {
		logDebug("Tỷ lệ khung hình (%s) đã đúng sẵn, bỏ qua không chỉnh lại.", req.AspectRatio)
	}

	// 5. Cấu hình Batch Size (Chỉ chỉnh nếu chưa đúng)
	if req.BatchSize != "" && !isBatchMatch {
		logDebug("Cấu hình số lượng (batch size): %s (chỉnh lại vì chưa trùng khớp)", req.BatchSize)
		clickBatchBtn, errBatch := getElement(3*time.Second, `(batch) => {
			const cleanDigit = batch.toLowerCase().replace('x', '').trim();
			if (cleanDigit) {
				const btn = document.querySelector('button[id$="-trigger-' + cleanDigit + '"]');
				if (btn) return btn;
			}
			
			// Dự phòng tìm trong popover đang mở
			const popover = document.querySelector('[data-state="open"][role="menu"], [data-state="open"]');
			if (popover) {
				const buttons = Array.from(popover.querySelectorAll('button'));
				return buttons.find(b => {
					const txt = b.textContent.toLowerCase().trim();
					return txt === batch.toLowerCase() || txt === 'x' + cleanDigit || txt === cleanDigit + 'x';
				}) || null;
			}
			return null;
		}`, req.BatchSize)

		if errBatch == nil && clickBatchBtn != nil {
			errClick := HumanClick(page, clickBatchBtn)
			if errClick != nil {
				logDebug("Lỗi click batch bằng chuột: %v. Thử bằng JS...", errClick)
				_, _ = clickBatchBtn.Eval("function() { this.click(); }")
			}
			sleep(500 * time.Millisecond)
		} else {
			logDebug("Không cấu hình được số lượng batch size: %v", errBatch)
		}
	} else if isBatchMatch {
		logDebug("Số lượng Batch Size (%s) đã đúng sẵn, bỏ qua không chỉnh lại.", req.BatchSize)
	}

	// 6. Cấu hình Model (Chỉ chỉnh nếu chưa đúng)
	if req.Model != "" && !isModelMatch {
		logDebug("Cấu hình Model: %s (chỉnh lại vì chưa trùng khớp)", req.Model)
		// Tìm nút mở dropdown chọn model bên trong popover (sử dụng XPath, class và role)
		clickModelDropdownBtn, errModelDropdown := getElement(3*time.Second, `() => {
			// Tìm popover thực sự chứa nút chọn Model (không lấy nhầm nút trigger)
			const popovers = Array.from(document.querySelectorAll('[data-state="open"]'));
			const popover = popovers.find(el => el.tagName !== 'BUTTON' && el.tagName !== 'SPAN' && el.querySelector('button'));
			if (!popover) return null;
			
			const buttons = Array.from(popover.querySelectorAll('button'));
			// Tìm nút có aria-haspopup="menu" hiển thị thông tin Model hiện tại
			return buttons.find(b => {
				const id = b.getAttribute('id') || '';
				const hasPopup = b.getAttribute('aria-haspopup') === 'menu' || b.getAttribute('aria-haspopup') === 'listbox';
				const txt = b.textContent.toLowerCase();
				return hasPopup && id.startsWith('radix-') && (txt.includes('banana') || txt.includes('veo') || txt.includes('imagen') || txt.includes('omni'));
			}) || null;
		}`)

		if errModelDropdown == nil && clickModelDropdownBtn != nil {
			errClick := HumanClick(page, clickModelDropdownBtn)
			if errClick != nil {
				logDebug("Lỗi click model dropdown bằng chuột: %v. Thử bằng JS...", errClick)
				_, _ = clickModelDropdownBtn.Eval("function() { this.click(); }")
			}
			sleep(800 * time.Millisecond) // Chờ menu model mở ra

			// Chuẩn hóa tên Model để so khớp chính xác
			modelToSelect := req.Model
			if !isVideoType {
				if modelToSelect == "Imagen 3" {
					modelToSelect = "Nano Banana 2"
				} else if strings.Contains(modelToSelect, "Quality") {
					modelToSelect = "Nano Banana Pro"
				} else if strings.Contains(modelToSelect, "Fast") || strings.Contains(modelToSelect, "Lite") {
					modelToSelect = "Nano Banana 2 Lite"
				}
			}

			modelItem, errItem := getElement(3*time.Second, `(modelName) => {
				// Tìm dropdown menu mới nhất vừa được mở ra (lấy menu cuối cùng trong DOM để tránh lấy phải popover cha)
				const dropdowns = Array.from(document.querySelectorAll('[role="menu"][id^="radix-"], [role="listbox"]'));
				const dropdown = dropdowns.length > 0 ? dropdowns[dropdowns.length - 1] : null;
				if (!dropdown) return null;
				
				const buttons = Array.from(dropdown.querySelectorAll('button, [role="menuitem"]'));
				const cleanTarget = modelName.toLowerCase().replace(/[^a-z0-9]/g, "").trim();
				return buttons.find(b => {
					const txt = b.textContent.toLowerCase().replace(/[^a-z0-9]/g, "").trim();
					return txt === cleanTarget || txt.includes(cleanTarget);
				}) || null;
			}`, modelToSelect)

			if errItem == nil && modelItem != nil {
				errClick := HumanClick(page, modelItem)
				if errClick != nil {
					logDebug("Lỗi click model item bằng chuột: %v. Thử bằng JS...", errClick)
					_, _ = modelItem.Eval("function() { this.click(); }")
				}
				sleep(500 * time.Millisecond)
			} else {
				logDebug("Không cấu hình được model item %s: %v", modelToSelect, errItem)
			}
		} else {
			logDebug("Không tìm thấy nút dropdown chọn Model trong popover: %v", errModelDropdown)
		}
	} else if isModelMatch {
		logDebug("Model (%s) đã đúng sẵn, bỏ qua không chỉnh lại.", req.Model)
	}

	// 7. Đóng popover bằng cách click lại nút menu hoặc click vào ô nhập prompt
	logDebug("Đóng hộp thoại cấu hình tác nhân...")
	
	// 7. Đóng popover bằng cách click lại nút menu trigger
	logDebug("Đóng hộp thoại cấu hình tác nhân bằng cách click lại nút cài đặt...")
	if isFlowPopoverOpen(page) && configTriggerBtn != nil {
		_ = HumanClick(page, configTriggerBtn)
		sleep(800 * time.Millisecond) // Chờ popover đóng hẳn
	}

	// Dự phòng cuối cùng: click body
	if isFlowPopoverOpen(page) {
		logDebug("Hộp thoại vẫn mở. Gửi click ẩn danh lên body để đóng...")
		_, _ = page.Eval(`() => {
			document.body.click();
		}`)
		sleep(600 * time.Millisecond)
	}

	// Đảm bảo focus lại ô nhập prompt cuối cùng để sẵn sàng nhập văn bản
	promptInput, errPrompt := page.Element("div[role='textbox'][contenteditable='true'], div[contenteditable='true'], textarea")
	if errPrompt == nil && promptInput != nil {
		_ = promptInput.Focus()
	}

	return nil
}

// Kiểm tra xem popover cấu hình của Google Flow có thực sự đang hiển thị trên màn hình hay không
func isFlowPopoverOpen(page *rod.Page) bool {
	res, err := page.Eval(`() => {
		const elements = Array.from(document.querySelectorAll('button, div, span, label'));
		return elements.some(el => {
			const txt = el.textContent.trim();
			const isVisible = el.getBoundingClientRect().width > 0 && el.getBoundingClientRect().height > 0;
			return isVisible && (txt === '9:16' || txt === '16:9' || txt === '1:1' || txt.includes('tín dụng'));
		});
	}`)
	if err != nil {
		return false
	}
	return res.Value.Bool()
}

// Giả lập click chuột giống như người thật (chống quét bot của Google)
func HumanClick(page *rod.Page, el *rod.Element) error {
	if el == nil {
		return fmt.Errorf("element is nil")
	}

	// Trễ ngẫu nhiên trước khi click
	time.Sleep(time.Duration(randomRange(150, 400)) * time.Millisecond)

	// Thực hiện cuộn và click mô phỏng thật của Rod
	_ = el.ScrollIntoView()
	err := el.Click(proto.InputMouseButtonLeft, 1)

	// Trễ sau khi click để hệ thống phản hồi
	time.Sleep(time.Duration(randomRange(400, 800)) * time.Millisecond)

	return err
}

func randomRange(min, max int) int {
	if min >= max {
		return min
	}
	return rand.Intn(max-min+1) + min
}
func DismissWelcomeModals(ctx context.Context, page *rod.Page) {
	// Scroll any scrollable container in terms/welcome dialog to bottom to enable Continue buttons
	_, _ = page.Eval(`() => {
		const dialog = document.querySelector('div[role="dialog"]');
		if (dialog) {
			const scrollables = dialog.querySelectorAll('*');
			for (const el of scrollables) {
				if (el.scrollHeight > el.clientHeight) {
					el.scrollTop = el.scrollHeight;
				}
			}
		}
	}`)
	time.Sleep(500 * time.Millisecond)

	welcomeButtons := []string{
		"Tiếp theo", "Next", "Continue", "Tiếp tục", "Đồng ý", "I agree", "Got it", "Tôi đồng ý", "Accept", "Chấp nhận", "Bắt đầu", "Get started", "Done", "Xong",
	}

	for i := 0; i < 4; i++ {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// Scroll again in case new page of wizard opens
		_, _ = page.Eval(`() => {
			const dialog = document.querySelector('div[role="dialog"]');
			if (dialog) {
				const scrollables = dialog.querySelectorAll('*');
				for (const el of scrollables) {
					if (el.scrollHeight > el.clientHeight) {
						el.scrollTop = el.scrollHeight;
					}
				}
			}
		}`)

		clickedAny := false
		for _, text := range welcomeButtons {
			elements, err := page.Elements(fmt.Sprintf("button:contains('%s')", text))
			if err == nil {
				for _, el := range elements {
					if visible, _ := el.Visible(); visible {
						_ = el.Click(proto.InputMouseButtonLeft, 1)
						time.Sleep(1 * time.Second) // wait for popup step transition
						clickedAny = true
						break
					}
				}
			}
			if clickedAny {
				break
			}
		}
		if !clickedAny {
			break
		}
	}
}

func FillFlowPrompt(page *rod.Page, promptInput *rod.Element, prompt string) error {
	// 1. Luôn Focus vào ô promptInput trước để đảm bảo Chrome có điểm tập trung bàn phím
	_ = promptInput.Focus()
	time.Sleep(150 * time.Millisecond)

	// 2. Kiểm tra xem trong ô prompt có thẻ ảnh đính kèm hay không
	hasAttachedCard, _ := promptInput.Eval(`() => {
		const cards = this.querySelectorAll('img, canvas, div[data-tile-id], [aria-label*="remove"], [aria-label*="xóa"]');
		return cards.length > 0;
	}`)

	if hasAttachedCard != nil && hasAttachedCard.Value.Bool() {
		// TRƯỜNG HỢP 1: Có ảnh đính kèm -> Đặt con trỏ sau thẻ ảnh & chèn prompt qua CDP InsertText
		_, _ = promptInput.Eval(`() => {
			this.focus();
			try {
				const range = document.createRange();
				range.selectNodeContents(this);
				range.collapse(false);
				const sel = window.getSelection();
				sel.removeAllRanges();
				sel.addRange(range);
			} catch (e) {}
		}`)
		time.Sleep(150 * time.Millisecond)
		_ = page.InsertText(prompt)
	} else {
		// TRƯỜNG HỢP 2: KHÔNG có ảnh đính kèm (Chỉ nhập câu lệnh văn bản thường) -> Click & Gõ prompt
		_ = HumanClick(page, promptInput)
		_ = promptInput.Focus()
		time.Sleep(150 * time.Millisecond)
		
		_ = promptInput.SelectAllText()
		_ = promptInput.Input("")
		
		errInput := promptInput.Input(prompt)
		if errInput != nil {
			_ = page.InsertText(prompt)
		}
	}

	time.Sleep(400 * time.Millisecond)

	// 3. Kích hoạt event input/change để React cập nhật state và ẩn chữ mờ placeholder
	_, _ = promptInput.Eval(`() => {
		this.dispatchEvent(new Event('input', { bubbles: true }));
		this.dispatchEvent(new Event('change', { bubbles: true }));
	}`)

	return nil
}

func getSettingsPath() string {
	dirUser, err := os.UserConfigDir()
	if err != nil {
		dirUser, _ = os.UserHomeDir()
	}
	return filepath.Join(dirUser, "video-splitter", "settings.json")
}

func readProjectURL() string {
	path := getSettingsPath()
	file, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer file.Close()
	
	data, err := io.ReadAll(file)
	if err != nil {
		return ""
	}
	
	var config map[string]interface{}
	if err := json.Unmarshal(data, &config); err != nil {
		return ""
	}
	
	if val, ok := config["googleFlowProjectURL"]; ok {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return ""
}

func saveProjectURL(projectURL string) {
	path := getSettingsPath()
	
	var config map[string]interface{} = make(map[string]interface{})
	
	file, err := os.Open(path)
	if err == nil {
		data, errRead := io.ReadAll(file)
		file.Close()
		if errRead == nil {
			_ = json.Unmarshal(data, &config)
		}
	}
	
	config["googleFlowProjectURL"] = projectURL
	
	newData, errMarshal := json.MarshalIndent(config, "", "  ")
	if errMarshal == nil {
		_ = os.MkdirAll(filepath.Dir(path), 0755)
		_ = os.WriteFile(path, newData, 0644)
	}
}


// CopyImageToClipboardWindows nạp file ảnh trực tiếp vào Clipboard của hệ thống Windows qua PowerShell
// CopyImageToClipboardWindows nạp file ảnh trực tiếp vào Clipboard của hệ thống Windows qua PowerShell
func CopyImageToClipboardWindows(imagePath string) error {
	absPath, err := filepath.Abs(imagePath)
	if err != nil {
		absPath = imagePath
	}

	cmdStr := fmt.Sprintf(`Add-Type -Assembly System.Windows.Forms; Add-Type -Assembly System.Drawing; $img = [System.Drawing.Image]::FromFile('%s'); [System.Windows.Forms.Clipboard]::SetImage($img); $img.Dispose()`, absPath)
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", cmdStr)
	return cmd.Run()
}

// PasteImageNativeCtrlV thực hiện dán Ctrl+V thật sự thông qua Clipboard hệ thống OS & phím Ctrl+V bàn phím thật
func PasteImageNativeCtrlV(ctx context.Context, page *rod.Page, promptInput *rod.Element, imagePath string, logDebug func(string, ...interface{})) error {
	if logDebug != nil {
		logDebug("Đang nạp ảnh vào Clipboard hệ thống Windows: %s...", imagePath)
	}

	errCopy := CopyImageToClipboardWindows(imagePath)
	if errCopy != nil && logDebug != nil {
		logDebug("Lỗi nạp ảnh vào Clipboard hệ thống: %v", errCopy)
	} else if logDebug != nil {
		logDebug("✓ Đã nạp ảnh vào Clipboard hệ thống Windows thành công!")
	}

	// Focus ô prompt
	_ = promptInput.Hover()
	_ = HumanClick(page, promptInput)
	_ = promptInput.Focus()
	time.Sleep(300 * time.Millisecond)

	if logDebug != nil {
		logDebug("Đang bấm phím Ctrl+V thật từ bàn phím hệ thống qua KeyActions CDP...")
	}

	// Gửi tổ hợp phím Ctrl+V thật từ CDP Keyboard Action
	_ = page.KeyActions().Press(input.ControlLeft).Press(input.KeyV).Release(input.ControlLeft).Do()
	time.Sleep(800 * time.Millisecond)

	return nil
}
// WaitUntilImageAttachedAndLoaded chờ cho tới khi ảnh upload đã hiển thị hoàn toàn trong ô prompt
func WaitUntilImageAttachedAndLoaded(ctx context.Context, page *rod.Page, timeout time.Duration, logDebug func(string, ...interface{})) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return false
		default:
		}

		isLoaded, _ := page.Eval(`() => {
			const hasLoadingPercentage = Array.from(document.querySelectorAll('div, span, p, h1, h2, h3, canvas')).some(el => {
				const txt = el.textContent.trim();
				const isVisible = el.getBoundingClientRect().width > 0;
				return isVisible && /\d{1,2}%/.test(txt);
			});

			if (hasLoadingPercentage) {
				return false;
			}

			const hasAttachedCard = Array.from(document.querySelectorAll('img, canvas, div[data-tile-id]')).some(el => {
				const isVisible = el.getBoundingClientRect().width > 0;
				const src = el.getAttribute('src') || '';
				const alt = el.getAttribute('alt') || '';
				const inPrompt = el.closest('div[role="textbox"], [contenteditable="true"]') !== null ||
				                 el.closest('form, div[class*="input"], div[class*="prompt"]') !== null;
				return isVisible && (inPrompt || src.includes('blob:') || src.includes('getMediaUrl') || src.includes('/fx/api/') || alt.includes('thumb'));
			}) || Array.from(document.querySelectorAll('button, div, span')).some(el => {
				const isVisible = el.getBoundingClientRect().width > 0;
				const aria = (el.getAttribute('aria-label') || '').toLowerCase();
				const title = (el.getAttribute('title') || '').toLowerCase();
				return isVisible && (aria.includes('remove') || aria.includes('xóa') || aria.includes('delete') || aria.includes('close') || title.includes('remove') || title.includes('xóa'));
			});

			return hasAttachedCard;
		}`)

		if isLoaded != nil && isLoaded.Value.Bool() {
			if logDebug != nil {
				logDebug("✓ Đã xác nhận: Thẻ ảnh đã đính kèm và load xong 100%% trong ô prompt!")
			}
			return true
		}

		time.Sleep(500 * time.Millisecond)
	}

	if logDebug != nil {
		logDebug("Cảnh báo: Hết thời gian chờ (%v) nhưng vẫn tiếp tục luồng...", timeout)
	}
	return false
}
