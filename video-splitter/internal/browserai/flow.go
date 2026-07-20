package browserai

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-rod/rod"
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
		_, errCheckPrompt := FindFirstVisible(ctx, page, FlowSelectors.PromptInputs, 5*time.Second)
		if errCheckPrompt != nil {
			logDebug("Không tìm thấy ô nhập liệu trong dự án cũ sau 5 giây. Có vẻ dự án này đã bị lỗi hoặc bị xóa. Tiến hành mở trang chủ để tạo dự án mới...")
			hasProject = false
			errNavigateHome := page.Navigate("https://labs.google/fx/vi/tools/flow")
			if errNavigateHome == nil {
				_ = page.WaitDOMStable(2*time.Second, 0.5)
			}
		}
	}

	if !hasProject {
		logDebug("Dự án cũ không hoạt động hoặc không có. Tiến hành click tạo dự án mới...")
		tm.EmitStatus(TaskStateReady, "Đang vào không gian làm việc (tạo dự án mới)...", 21)
		
		// 1. Try JS-based clicker with xpath and case-insensitive text fallback
		_, _ = page.Eval(`() => {
			try {
				const xpathResult = document.evaluate("//*[@id='__next']/div[2]/div/div/button", document, null, XPathResult.FIRST_ORDERED_NODE_TYPE, null);
				const xpathBtn = xpathResult.singleNodeValue;
				if (xpathBtn) {
					xpathBtn.click();
					return;
				}
			} catch (e) {}

			const elements = Array.from(document.querySelectorAll('button, div[role="button"], [class*="project"], [class*="Project"]'));
			for (const el of elements) {
				const txt = el.textContent.toLowerCase();
				if (txt.includes('dự án mới') || txt.includes('new project') || txt.includes('create project') || txt.includes('dự án')) {
					el.click();
					return;
				}
			}
		}`)
		_ = page.WaitDOMStable(2*time.Second, 0.5)

		// 2. Fallback to original rod method if URL is still not in a project workspace
		info, err = page.Info()
		if err != nil || !strings.Contains(info.URL, "/project/") {
			newProjectBtn, errBtn := FindFirstVisible(ctx, page, []SelectorCandidate{
				{Selector: "button:contains('Dự án mới')"},
				{Selector: "div:contains('Dự án mới')"},
				{Selector: "span:contains('Dự án mới')"},
				{Selector: "button:contains('New project')"},
				{Selector: "div:contains('New project')"},
				{Selector: "span:contains('New project')"},
				{Selector: "*:contains('Dự án mới')"},
				{Selector: "*:contains('New project')"},
			}, 5*time.Second)
			if errBtn == nil && newProjectBtn != nil {
				_, _ = newProjectBtn.Eval("el => el.click()")
				_ = page.WaitDOMStable(2*time.Second, 0.5)
			}
		}

		// 3. Wait for URL redirection to contain "/project/"
		urlSuccess := false
		logDebug("Đang chờ URL chuyển hướng sang '/project/'...")
		for idx := 0; idx < 30; idx++ { // Wait up to 15 seconds (30 * 500ms)
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
			time.Sleep(500 * time.Millisecond)
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

	// Lấy danh sách URL của nhóm ảnh cuối cùng trước khi bấm Tạo
	var lastGroupUrls []string
	if req.MediaType == MediaTypeImage {
		lastUrlsObj, errLast := page.Eval(`() => {
			const buttons = Array.from(document.querySelectorAll('button')).filter(btn => {
				const img = btn.querySelector('img');
				return img && img.getAttribute('src')?.includes('getMediaUrl');
			});
			if (buttons.length === 0) return [];
			let parent = buttons[buttons.length - 1].parentElement;
			while (parent && !parent.className.includes('layout') && !parent.className.includes('grid') && parent.tagName !== 'SECTION') {
				parent = parent.parentElement;
			}
			if (!parent) parent = buttons[buttons.length - 1].parentElement;
			if (parent) {
				const siblingButtons = Array.from(parent.querySelectorAll('button')).filter(btn => {
					const img = btn.querySelector('img');
					return img && img.getAttribute('src')?.includes('getMediaUrl');
				});
				return siblingButtons.map(btn => {
					const img = btn.querySelector('img');
					return img ? img.getAttribute('src') : '';
				}).filter(src => src !== '');
			}
			return [];
		}`)
		if errLast == nil && lastUrlsObj != nil {
			for _, v := range lastUrlsObj.Value.Arr() {
				lastGroupUrls = append(lastGroupUrls, v.Str())
			}
		}
		logDebug("URL nhóm ảnh cuối cùng trước khi tạo: %v", lastGroupUrls)
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

		tm.EmitStatus(TaskStateSubmitting, "Đang điền prompt...", 35)
		err = FillFlowPrompt(page, promptInput, req.Prompt)
		if err != nil {
			finalErr = fmt.Errorf("fill prompt: %w", err)
			continue
		}

		if len(req.InputImagePaths) > 0 {
			logDebug("Phát hiện yêu cầu gửi kèm ảnh (%d ảnh). Đang upload ảnh...", len(req.InputImagePaths))
			// Tìm file input
			fileInput, errInput := page.Element("input[type=file]")
			if errInput != nil {
				logDebug("Không tìm thấy input[type=file] ẩn. Thử click nút '+' để kích hoạt...")
				// Tìm nút +
				addBtn, errAdd := page.ElementByJS(rod.Eval(`() => {
					const buttons = Array.from(document.querySelectorAll('button'));
					return buttons.find(btn => {
						const txt = btn.textContent.toLowerCase();
						const icon = btn.querySelector('span, i');
						const iconTxt = icon ? icon.textContent.trim().toLowerCase() : '';
						return iconTxt === 'add' || txt.includes('tải lên') || txt.includes('upload') || btn.querySelector('svg');
					});
				}`))
				if errAdd == nil && addBtn != nil {
					_ = addBtn.Click(proto.InputMouseButtonLeft, 1)
					sleep(1000 * time.Millisecond)
					fileInput, errInput = page.Element("input[type=file]")
				}
			}

			if errInput == nil && fileInput != nil {
				errSet := fileInput.SetFiles(req.InputImagePaths)
				if errSet != nil {
					logDebug("Lỗi khi set file upload: %v", errSet)
				} else {
					logDebug("Đã upload %d ảnh thành công! Đang chờ 3 giây để upload hoàn tất...", len(req.InputImagePaths))
					sleep(3000 * time.Millisecond) // Chờ ảnh upload xong và hiển thị trong ô prompt
				}
			} else {
				logDebug("Không tìm thấy phần tử tải file lên trên Google Flow")
			}
		}

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
			_, errEv := promptInput.Eval(`(el) => {
				const ev = new KeyboardEvent('keydown', {
					key: 'Enter',
					code: 'Enter',
					keyCode: 13,
					which: 13,
					bubbles: true,
					cancelable: true
				});
				el.dispatchEvent(ev);
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
				submitBtn, errSubmit := promptInput.ElementByJS(rod.Eval(`(textbox) => {
					if (!textbox) return null;
					let parent = textbox.parentElement;
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
			
			for {
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-tickerImg.C:
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

				// Check for error card via page.Eval JS
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
					logDebug("Phát hiện thẻ lỗi trên Google Flow (Không thành công / Rất tiếc, đã xảy ra lỗi) khi tạo hình ảnh.")
					finalErr = NewError(ErrGenerationFailed, "Google Flow báo lỗi tạo hình ảnh.")
					generationFailed = true
					break
				}
				
				// Quét nhóm ảnh cuối cùng hiện tại
				currentUrlsObj, errCurr := page.Eval(`() => {
					const buttons = Array.from(document.querySelectorAll('button')).filter(btn => {
						const img = btn.querySelector('img');
						return img && img.getAttribute('src')?.includes('getMediaUrl');
					});
					if (buttons.length === 0) return [];
					let parent = buttons[buttons.length - 1].parentElement;
					while (parent && !parent.className.includes('layout') && !parent.className.includes('grid') && parent.tagName !== 'SECTION') {
						parent = parent.parentElement;
					}
					if (!parent) parent = buttons[buttons.length - 1].parentElement;
					if (parent) {
						const siblingButtons = Array.from(parent.querySelectorAll('button')).filter(btn => {
							const img = btn.querySelector('img');
							return img && img.getAttribute('src')?.includes('getMediaUrl');
						});
						return siblingButtons.map(btn => {
							const img = btn.querySelector('img');
							return img ? img.getAttribute('src') : '';
						}).filter(src => src !== '');
					}
					return [];
				}`)

				if errCurr == nil && currentUrlsObj != nil {
					var currentUrls []string
					for _, v := range currentUrlsObj.Value.Arr() {
						currentUrls = append(currentUrls, v.Str())
					}

					// So sánh với lastGroupUrls
					isNew := false
					if len(lastGroupUrls) == 0 {
						if len(currentUrls) > 0 {
							isNew = true
						}
					} else {
						if len(currentUrls) > 0 && (len(currentUrls) != len(lastGroupUrls) || currentUrls[len(currentUrls)-1] != lastGroupUrls[len(lastGroupUrls)-1]) {
							isNew = true
						}
					}

					if isNew {
						sleep(2000 * time.Millisecond) // Chờ thêm 2 giây để ảnh load hoàn toàn
						
						// Đọc lại danh sách URL chính xác nhất sau khi đã load xong
						currentUrlsObjSec, errCurrSec := page.Eval(`() => {
							const buttons = Array.from(document.querySelectorAll('button')).filter(btn => {
								const img = btn.querySelector('img');
								return img && img.getAttribute('src')?.includes('getMediaUrl');
							});
							if (buttons.length === 0) return [];
							let parent = buttons[buttons.length - 1].parentElement;
							while (parent && !parent.className.includes('layout') && !parent.className.includes('grid') && parent.tagName !== 'SECTION') {
								parent = parent.parentElement;
							}
							if (!parent) parent = buttons[buttons.length - 1].parentElement;
							if (parent) {
								const siblingButtons = Array.from(parent.querySelectorAll('button')).filter(btn => {
									const img = btn.querySelector('img');
									return img && img.getAttribute('src')?.includes('getMediaUrl');
								});
								return siblingButtons.map(btn => {
									const img = btn.querySelector('img');
									return img ? img.getAttribute('src') : '';
								}).filter(src => src !== '');
							}
							return [];
						}`)
						if errCurrSec == nil && currentUrlsObjSec != nil {
							newGroupUrls := []string{}
							for _, v := range currentUrlsObjSec.Value.Arr() {
								newGroupUrls = append(newGroupUrls, v.Str())
							}
							logDebug("Tìm thấy nhóm ảnh mới sinh thành công! Số lượng: %d", len(newGroupUrls))
							
							// Kiểm tra tổng số ảnh trên toàn trang để tránh lag
							totalCardsCountObj, errTotalCount := page.Eval(`() => {
								const buttons = Array.from(document.querySelectorAll('button'));
								let count = 0;
								for (const btn of buttons) {
									const img = btn.querySelector('img');
									if (img && img.getAttribute('src')?.includes('getMediaUrl')) {
										count++;
									}
								}
								return count;
							}`)
							if errTotalCount == nil && totalCardsCountObj != nil {
								totalCount := totalCardsCountObj.Value.Int()
								logDebug("Tổng số lượng hình ảnh đang có trong dự án: %d", totalCount)
								if totalCount > 50 {
									logDebug("Dự án hiện tại có %d hình ảnh (vượt ngưỡng 50 ảnh để tránh lag Chrome). Tiến hành xóa link dự án cũ để lần sau tạo dự án mới...", totalCount)
									saveProjectURL("") // Xóa link project để lần sau tạo dự án mới tinh!
								}
							}

							// Bắt đầu xử lý preview
							var directUrls []string
							for _, src := range newGroupUrls {
								tryUrl := src
								if strings.Contains(src, "getMediaUrlRedirect") {
									parts := strings.Split(src, "?")
									if len(parts) > 1 {
										queryParams := strings.Split(parts[1], "&")
										for _, param := range queryParams {
											kv := strings.Split(param, "=")
											if len(kv) == 2 && kv[0] == "name" {
												tryUrl = "https://flow-content.google/image/" + kv[1]
												break
											}
										}
									}
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

			// Gửi preview về frontend và chờ người dùng chọn
			taskID := "temp"
			if active := tm.GetActiveTask(); active != nil {
				taskID = active.ID
			}

			logDebug("Đang gửi yêu cầu chọn ảnh lên UI...")
			tm.EmitSelectionRequired(taskID, previewBase64s)

			var selectedIndexes []int
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case selectedIndexes = <-tm.GetActiveTask().SelectionChan:
				logDebug("Đã nhận được danh sách ảnh được chọn từ UI: %v", selectedIndexes)
			}

			if len(selectedIndexes) == 0 {
				logDebug("Người dùng không chọn ảnh nào để tải.")
				return nil, fmt.Errorf("không có hình ảnh nào được chọn để tải")
			}

			tm.EmitStatus(TaskStateDownloading, "Đang tải các ảnh đã chọn...", 85)

			var downloadedPaths []string
			for idx, selIdx := range selectedIndexes {
				logDebug("Đang tải hình ảnh được chọn thứ %d/%d (chỉ mục trong nhóm: %d)...", idx+1, len(selectedIndexes), selIdx)

				// Click the image card in the last layout group at index `selIdx` using atomic JS
				clickedObj, errClick := page.Eval(`(selIdx) => {
					const buttons = Array.from(document.querySelectorAll('button')).filter(btn => {
						const img = btn.querySelector('img');
						return img && img.getAttribute('src')?.includes('getMediaUrl');
					});
					if (buttons.length === 0) return false;
					let parent = buttons[buttons.length - 1].parentElement;
					while (parent && !parent.className.includes('layout') && !parent.className.includes('grid') && parent.tagName !== 'SECTION') {
						parent = parent.parentElement;
					}
					if (!parent) parent = buttons[buttons.length - 1].parentElement;
					if (parent) {
						const siblingButtons = Array.from(parent.querySelectorAll('button')).filter(btn => {
							const img = btn.querySelector('img');
							return img && img.getAttribute('src')?.includes('getMediaUrl');
						});
						if (selIdx < siblingButtons.length) {
							const el = siblingButtons[selIdx];
							el.scrollIntoView({ block: 'center' });
							el.click();
							return true;
						}
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

				hasUltra, _ := page.Eval(`() => {
					// Check if user has ULTRA badge on screen
					return Array.from(document.querySelectorAll('span, div')).some(el => {
						return el.textContent.toUpperCase().includes('ULTRA') && el.getBoundingClientRect().width > 0;
					});
				}`)

				if targetRes == "4k" && (hasUltra != nil && !hasUltra.Value.Bool()) {
					logDebug("Tài khoản chưa đăng ký gói ULTRA (không có badge ULTRA). Tự động lùi độ phân giải từ 4K về 2K.")
					targetRes = "2k"
				}
				
				// Click resolution button
				optionBtn, errOpt := page.ElementByJS(rod.Eval(`(target) => {
					const buttons = Array.from(document.querySelectorAll('button'));
					const match = buttons.find(el => el.textContent.toLowerCase().includes(target));
					if (match) {
						if (target === '4k') {
							const upgradeBtn = Array.from(match.querySelectorAll('button, span, div')).find(sub => {
								const subTxt = sub.textContent.toLowerCase();
								return subTxt.includes('nâng cấp') || subTxt.includes('upgrade');
							});
							if (upgradeBtn) {
								return upgradeBtn;
							}
						}
						return match;
					}
					return null;
				}`, targetRes))
				
				if errOpt != nil || optionBtn == nil {
					logDebug("Không tìm thấy tùy chọn độ phân giải %s cho ảnh thứ %d, bỏ qua", req.Resolution, idx+1)
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

				// Check if we need to wait for upscaling (for 2K/4K if it triggers upscale)
				isUpscalingTriggered := false
				if targetRes == "2k" || targetRes == "4k" {
					isUpscalingTriggered = true
				}
				
				_ = optionBtn.Click(proto.InputMouseButtonLeft, 1)

				if isUpscalingTriggered {
					logDebug("Đã click kích hoạt nâng cấp lên %s. Đang chờ quá trình nâng cấp hoàn tất...", req.Resolution)
					upscaleTicker := time.NewTicker(2 * time.Second)
					defer upscaleTicker.Stop()
					upscaleDeadline := time.Now().Add(60 * time.Second)
					
					for {
						select {
						case <-ctx.Done():
							return nil, ctx.Err()
						case <-upscaleTicker.C:
						}
						
						if time.Now().After(upscaleDeadline) {
							logDebug("Cảnh báo: Đợi nâng cấp %s quá thời gian chờ (timeout), tiếp tục tải về", req.Resolution)
							break
						}
						
						// Re-evaluate button status
						stillUpscaling, _ := page.Eval(`(target) => {
							const btn = Array.from(document.querySelectorAll('button')).find(el => el.textContent.toLowerCase().includes(target));
							if (!btn) return false;
							const txt = btn.textContent.toLowerCase();
							return txt.includes('nâng cấp') || txt.includes('đang') || txt.includes('progress') || txt.includes('loading');
						}`, targetRes)
						
						if stillUpscaling != nil && !stillUpscaling.Value.Bool() {
							logDebug("Nâng cấp lên %s hoàn tất!", req.Resolution)
							sleep(1500 * time.Millisecond) // wait for UI stabilization
							
							// Now click the option again since it is upscaled and ready for download!
							newOptionBtn, errNewOpt := page.ElementByJS(rod.Eval(`(target) => {
								return Array.from(document.querySelectorAll('button')).find(el => {
									const txt = el.textContent.toLowerCase();
									return txt.includes(target) && !txt.includes('nâng cấp');
								});
							}`, targetRes))
							if errNewOpt == nil && newOptionBtn != nil {
								optionBtn = newOptionBtn
							}
							break
						}
					}
				}
				
				// Initiate Wails download capturing
				waitDownload := session.browser.WaitDownload(session.downloadDir)
				
				// Click the finalized option to start download
				_ = optionBtn.Click(proto.InputMouseButtonLeft, 1)
				
				// Generate unique filename for each image in batch
				customFileName := req.FileName
				if len(selectedIndexes) > 1 {
					customFileName = fmt.Sprintf("%s_%d", req.FileName, idx+1)
				}
				
				filePath, errMove := WaitAndMoveDownload(ctx, waitDownload, session.downloadDir, req.OutputDir, customFileName, MediaTypeImage)
				if errMove == nil {
					logDebug("Tải ảnh thành công: %s", filePath)
					downloadedPaths = append(downloadedPaths, filePath)
				} else {
					logDebug("Lỗi khi lưu ảnh %d: %v", idx+1, errMove)
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
				sleep(1200 * time.Millisecond)
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

	// If no custom options are specified, we don't need to open the settings
	if req.ConfirmBeforeCreate == "" && req.Model == "" && req.BatchSize == "" && req.AspectRatio == "" {
		return nil
	}

	tm.EmitStatus(TaskStateSubmitting, "Đang cấu hình cài đặt tác nhân Google Flow...", 28)

	// 1. Find the settings gear/slider button using safe JS evaluation
	logDebug("Đang quét tìm nút Cài đặt tác nhân (tune/settings)...")
	settingsBtn, err := page.ElementByJS(rod.Eval(`() => {
		const buttons = Array.from(document.querySelectorAll('button'));
		for (const btn of buttons) {
			const aria = (btn.getAttribute('aria-label') || '').toLowerCase();
			const title = (btn.getAttribute('title') || '').toLowerCase();
			const icon = btn.querySelector('i, span.google-symbols, .material-symbols-outlined, [class*="icon"]');
			const iconText = icon ? icon.textContent.trim().toLowerCase() : '';
			
			// Nút settings tác nhân có icon là "tune" hoặc aria-label chứa "cài đặt"/"settings"
			// Loại trừ các nút quay lại (arrow_back, arrow_forward) hoặc close
			if (iconText === 'tune' || 
			    aria.includes('cài đặt tác nhân') || aria.includes('agent settings') || aria.includes('cấu hình') ||
			    title.includes('cài đặt tác nhân') || title.includes('agent settings')) {
				
				const rect = btn.getBoundingClientRect();
				if (rect.width > 0 && rect.height > 0) {
					return btn;
				}
			}
		}
		
		// Fallback sang check icon settings chung nhưng ở nửa dưới màn hình (gần ô prompt)
		for (const btn of buttons) {
			const icon = btn.querySelector('i, span.google-symbols, .material-symbols-outlined, [class*="icon"]');
			const iconText = icon ? icon.textContent.trim().toLowerCase() : '';
			if (iconText === 'settings' || iconText === 'tune') {
				const rect = btn.getBoundingClientRect();
				if (rect.width > 0 && rect.height > 0 && rect.top > 300) { // Thường nằm cạnh ô nhập prompt ở góc dưới
					return btn;
				}
			}
		}
		return null;
	}`))

	// Fallback to original FindFirstVisible if JS evaluator failed
	if err != nil || settingsBtn == nil {
		logDebug("Tìm nút cài đặt qua JS thất bại (%v). Thử tìm qua FindFirstVisible...", err)
		settingsBtn, err = FindFirstVisible(ctx, page, []SelectorCandidate{
			{Selector: "button[aria-label*='tune']"},
			{Selector: "button[aria-label*='Tune']"},
			{Selector: "button[aria-label*='settings']"},
			{Selector: "button[aria-label*='Cài đặt']"},
			{Selector: "button[aria-label*='cài đặt']"},
			{Selector: "button[title*='Settings']"},
			{Selector: "button[title*='Cài đặt']"},
		}, 5*time.Second)
	}

	if err != nil || settingsBtn == nil {
		logDebug("Không tìm thấy nút cài đặt tác nhân trên trang.")
		return fmt.Errorf("không tìm thấy nút cài đặt trên Google Flow: %w", err)
	}

	htmlBtn, _ := settingsBtn.HTML()
	logDebug("Đã tìm thấy nút cài đặt tác nhân! HTML: %s", htmlBtn)

	err = settingsBtn.Click(proto.InputMouseButtonLeft, 1)
	if err != nil {
		return fmt.Errorf("click settings gear: %w", err)
	}

	time.Sleep(1500 * time.Millisecond) // wait for settings modal to open

	// 2. Find the settings side panel/dialog
	logDebug("Đang quét tìm hộp thoại/panel Cài đặt tác nhân...")
	dialog, errDlg := page.ElementByJS(rod.Eval(`() => {
		// Tìm các div, section, hoặc role dialog lớn chứa tiêu đề "Cài đặt tác nhân" hoặc "Agent settings" hoặc chứa chữ "Xác nhận trước khi tạo"
		const elements = Array.from(document.querySelectorAll('div, section, form, [role="dialog"], [role="menu"]'));
		for (const el of elements) {
			const txt = el.textContent;
			if (txt.includes('Cài đặt tác nhân') || txt.includes('Agent settings') || txt.includes('Xác nhận trước khi tạo')) {
				const rect = el.getBoundingClientRect();
				if (rect.width > 200 && rect.height > 200) {
					return el;
				}
			}
		}
		return null;
	}`))

	// Fallback to original FindFirstVisible candidates
	if errDlg != nil || dialog == nil {
		logDebug("Tìm panel cài đặt qua JS thất bại (%v). Thử tìm qua FindFirstVisible...", errDlg)
		dialogCandidates := []SelectorCandidate{
			{Selector: "div[role='dialog']"},
			{Selector: "div[role='menu']"},
			{Selector: "div[data-radix-menu-content]"},
			{Selector: "//*[@id=\"__next\"]/div[1]/div[4]/div[2]/div[2]/div/div/div[2]"},
		}
		dialog, errDlg = FindFirstVisible(ctx, page, dialogCandidates, 5*time.Second)
	}

	if errDlg != nil || dialog == nil {
		logDebug("Không tìm thấy hộp thoại cấu hình tác nhân.")
		return fmt.Errorf("không tìm thấy hộp thoại cấu hình tác nhân: %w", errDlg)
	}

	logDebug("Đã tìm thấy hộp thoại/panel cài đặt tác nhân thành công!")



	isVideoType := req.MediaType == MediaTypeVideo

	// 2. Configure "Xác nhận trước khi tạo"
	if req.ConfirmBeforeCreate != "" {
		var candidates []SelectorCandidate
		if req.ConfirmBeforeCreate == "always" {
			candidates = []SelectorCandidate{
				{Selector: "//*[@id=\"__next\"]/div[1]/div[4]/div[2]/div[2]/div/div/div[2]/div[2]/div[1]/div[1]/div/button[1]"},
				{Selector: "button[value='ALWAYS_ASK']"},
				{Selector: "button:contains('Luôn luôn')"},
				{Selector: "button:contains('Always')"},
			}
		} else {
			candidates = []SelectorCandidate{
				{Selector: "//*[@id=\"__next\"]/div[1]/div[4]/div[2]/div[2]/div/div/div[2]/div[2]/div[1]/div[1]/div/button[2]"},
				{Selector: "button[value='AUTO_APPROVE']"},
				{Selector: "button:contains('Không bao giờ')"},
				{Selector: "button:contains('Never')"},
			}
		}
		btn, errBtn := FindFirstVisible(ctx, page, candidates, 2*time.Second)
		if errBtn == nil && btn != nil {
			_ = btn.Click(proto.InputMouseButtonLeft, 1)
		}
	}

	// Helper to click aspect ratio using dynamic Radix selector suffixes and user's XPaths
	clickAspectRatio := func(ratio string, isVideo bool) {
		var candidates []SelectorCandidate
		switch ratio {
		case "16:9":
			candidates = []SelectorCandidate{
				{Selector: "button[id$='-trigger-LANDSCAPE']"},
				{Selector: "//*[@id=\"radix-:r2t:-trigger-LANDSCAPE\"]"},
				{Selector: "button:contains('16:9')"},
			}
		case "4:3":
			candidates = []SelectorCandidate{
				{Selector: "button[id$='-trigger-LANDSCAPE_4_3']"},
				{Selector: "//*[@id=\"radix-:r2t:-trigger-LANDSCAPE_4_3\"]"},
				{Selector: "button:contains('4:3')"},
			}
		case "1:1":
			candidates = []SelectorCandidate{
				{Selector: "button[id$='-trigger-SQUARE']"},
				{Selector: "//*[@id=\"radix-:r2t:-trigger-SQUARE\"]"},
				{Selector: "button:contains('1:1')"},
			}
		case "3:4":
			candidates = []SelectorCandidate{
				{Selector: "button[id$='-trigger-PORTRAIT_3_4']"},
				{Selector: "//*[@id=\"radix-:r2t:-trigger-PORTRAIT_3_4\"]"},
				{Selector: "button:contains('3:4')"},
			}
		case "9:16":
			candidates = []SelectorCandidate{
				{Selector: "button[id$='-trigger-PORTRAIT']"},
				{Selector: "//*[@id=\"radix-:r2t:-trigger-PORTRAIT\"]"},
				{Selector: "button:contains('9:16')"},
			}
		default:
			return
		}

		for _, cand := range candidates {
			var elements rod.Elements
			var err error
			if strings.HasPrefix(cand.Selector, "/") || strings.HasPrefix(cand.Selector, "(") {
				elements, err = dialog.ElementsX(cand.Selector)
			} else {
				elements, err = dialog.Elements(cand.Selector)
			}
			if err == nil && len(elements) > 0 {
				var targetEl *rod.Element
				if isVideo && len(elements) >= 2 {
					targetEl = elements[1]
				} else {
					targetEl = elements[0]
				}
				if visible, _ := targetEl.Visible(); visible {
					_ = targetEl.Click(proto.InputMouseButtonLeft, 1)
					return
				}
			}
		}
	}

	// Helper to click batch size using dynamic Radix selector suffixes and user's XPaths
	clickBatchSize := func(batch string, isVideo bool) {
		var candidates []SelectorCandidate
		switch batch {
		case "1x":
			candidates = []SelectorCandidate{
				{Selector: "button[id$='-trigger-1']"},
				{Selector: "//*[@id=\"radix-:r33:-trigger-1\"]"},
				{Selector: "button:contains('1x')"},
			}
		case "2x", "x2":
			candidates = []SelectorCandidate{
				{Selector: "button[id$='-trigger-2']"},
				{Selector: "//*[@id=\"radix-:r33:-trigger-2\"]"},
				{Selector: "button:contains('x2')"},
				{Selector: "button:contains('2x')"},
			}
		case "3x", "x3":
			candidates = []SelectorCandidate{
				{Selector: "button[id$='-trigger-3']"},
				{Selector: "//*[@id=\"radix-:r33:-trigger-3\"]"},
				{Selector: "button:contains('x3')"},
				{Selector: "button:contains('3x')"},
			}
		case "4x", "x4":
			candidates = []SelectorCandidate{
				{Selector: "button[id$='-trigger-4']"},
				{Selector: "//*[@id=\"radix-:r33:-trigger-4\"]"},
				{Selector: "button:contains('x4')"},
				{Selector: "button:contains('4x')"},
			}
		default:
			return
		}

		for _, cand := range candidates {
			var elements rod.Elements
			var err error
			if strings.HasPrefix(cand.Selector, "/") || strings.HasPrefix(cand.Selector, "(") {
				elements, err = dialog.ElementsX(cand.Selector)
			} else {
				elements, err = dialog.Elements(cand.Selector)
			}
			if err == nil && len(elements) > 0 {
				var targetEl *rod.Element
				if isVideo && len(elements) >= 2 {
					targetEl = elements[1]
				} else {
					targetEl = elements[0]
				}
				if visible, _ := targetEl.Visible(); visible {
					_ = targetEl.Click(proto.InputMouseButtonLeft, 1)
					return
				}
			}
		}
	}

	// 3. Configure Aspect Ratio (e.g., "16:9", "9:16")
	if req.AspectRatio != "" {
		clickAspectRatio(req.AspectRatio, isVideoType)
	}

	// 4. Configure Batch size (e.g., "1x", "2x")
	if req.BatchSize != "" {
		clickBatchSize(req.BatchSize, isVideoType)
	}

	// 5. Configure Model dropdown selection
	if req.Model != "" {
		// Map user model names to flow models
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

		// Click the dropdown trigger
		var dropdownCandidates []SelectorCandidate
		if isVideoType {
			dropdownCandidates = []SelectorCandidate{
				{Selector: "button:contains('Omni Flash')"},
				{Selector: "button:contains('Veo')"},
				{Selector: "button[id^='radix-']"},
				{Selector: "//*[@id=\"radix-:r38:\"]"},
			}
		} else {
			dropdownCandidates = []SelectorCandidate{
				{Selector: "button:contains('Nano Banana')"},
				{Selector: "button:contains('Imagen')"},
				{Selector: "button[id^='radix-']"},
				{Selector: "//*[@id=\"radix-:r38:\"]"},
			}
		}

		// Find the dropdown buttons
		var dropdown *rod.Element
		dropdowns, errDrops := dialog.Elements("button, [role='combobox'], [id^='radix-']")
		if errDrops == nil && len(dropdowns) > 0 {
			// Find visible buttons that look like dropdown triggers
			var visibleTriggers []*rod.Element
			for _, el := range dropdowns {
				if visible, _ := el.Visible(); visible {
					txt, _ := el.Text()
					// Dropdowns usually show model name or have chevron
					if strings.Contains(txt, "Banana") || strings.Contains(txt, "Omni") || strings.Contains(txt, "Veo") || strings.Contains(txt, "Imagen") {
						visibleTriggers = append(visibleTriggers, el)
					}
				}
			}
			
			if len(visibleTriggers) > 0 {
				if isVideoType && len(visibleTriggers) >= 2 {
					dropdown = visibleTriggers[1]
				} else {
					dropdown = visibleTriggers[0]
				}
			}
		}

		// Fallback to FindFirstVisible candidates
		if dropdown == nil {
			dropdown, _ = FindFirstVisible(ctx, page, dropdownCandidates, 2*time.Second)
		}

		if dropdown != nil {
			_ = dropdown.Click(proto.InputMouseButtonLeft, 1)
			time.Sleep(800 * time.Millisecond) // wait for dropdown menu to open

			// Find the model option in the page using JS and click it via Go to trigger Radix UI properly
			opt, errEval := page.ElementByJS(rod.Eval(`(target) => {
				const menu = document.querySelector('[data-radix-dropdown-menu-content], [role="menu"]');
				if (!menu) return null;
				
				const items = Array.from(menu.querySelectorAll('button, [role="menuitem"], [role="option"]'));
				const cleanTarget = target.toLowerCase().replace(/[^a-z0-9 ]/g, "").replace(/\s+/g, " ").trim();
				
				// 1. Exact match (ignoring special characters/emojis)
				for (const item of items) {
					const text = item.textContent.toLowerCase().replace(/[^a-z0-9 ]/g, "").replace(/\s+/g, " ").trim();
					if (text === cleanTarget) {
						return item;
					}
				}
				
				// 2. Substring match fallback
				for (const item of items) {
					const text = item.textContent.toLowerCase().replace(/[^a-z0-9 ]/g, "").replace(/\s+/g, " ").trim();
					if (text.includes(cleanTarget)) {
						return item;
					}
				}
				return null;
			}`, modelToSelect))

			if errEval == nil && opt != nil {
				_ = opt.Click(proto.InputMouseButtonLeft, 1)
				time.Sleep(500 * time.Millisecond)
			} else {
				// Fallback to FindFirstVisible with 2-second timeout (never blocks forever!)
				optCandidates := []SelectorCandidate{
					{Selector: fmt.Sprintf("button:contains('%s')", modelToSelect)},
					{Selector: fmt.Sprintf("span:contains('%s')", modelToSelect)},
				}
				optFb, errOpt := FindFirstVisible(ctx, page, optCandidates, 2*time.Second)
				if errOpt == nil && optFb != nil {
					_ = optFb.Click(proto.InputMouseButtonLeft, 1)
				}
			}
		}
	}

	// 6. Click "Lưu" (Save) button to apply settings
	saveCandidates := []SelectorCandidate{
		{Selector: "//*[@id=\"__next\"]/div[1]/div[4]/div[2]/div[2]/div/div/div[2]/div[2]/div[2]/button"},
		{Selector: "button:contains('Lưu')"},
		{Selector: "button:contains('Save')"},
	}
	saveBtn, errSave := FindFirstVisible(ctx, page, saveCandidates, 5*time.Second)
	if errSave == nil && saveBtn != nil {
		_ = saveBtn.Click(proto.InputMouseButtonLeft, 1)
	}

	time.Sleep(800 * time.Millisecond) // wait for save
	return nil
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

func FillFlowPrompt(page *rod.Page, input *rod.Element, prompt string) error {
	// Click the prompt input to activate it and place the cursor
	if err := input.Click(proto.InputMouseButtonLeft, 1); err != nil {
		return fmt.Errorf("click prompt input: %w", err)
	}
	time.Sleep(200 * time.Millisecond)

	// Clear any existing text using JS
	_, _ = input.Eval(`(el) => {
		el.textContent = "";
		el.dispatchEvent(new Event('input', { bubbles: true }));
	}`)
	time.Sleep(100 * time.Millisecond)

	// Use standard go-rod input typing on focused contenteditable
	err := input.Input(prompt)
	if err != nil {
		// Fallback: try JS setting textContent + event dispatching (safe with arguments)
		_, errJS := input.Eval(
			`(el, val) => {
				el.textContent = val;
				el.dispatchEvent(new Event('input', { bubbles: true }));
				el.dispatchEvent(new Event('change', { bubbles: true }));
			}`,
			prompt,
		)
		if errJS != nil {
			return fmt.Errorf("input prompt and JS fallback failed: %w", err)
		}
	}

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
