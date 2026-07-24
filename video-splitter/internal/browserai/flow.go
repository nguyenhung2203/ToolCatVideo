package browserai

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"video-splitter/internal/utils"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/input"
	"github.com/go-rod/rod/lib/proto"
)

// GenerateFlowVideo tự động hóa Google Flow để tạo ảnh/video.
//
// workerPage: nếu != nil, chạy trên tab đó (dùng cho Hàng Đợi AI song song — mỗi
// worker một tab riêng) và LUÔN tạo project Flow mới để không lẫn ảnh giữa các tab.
// Nếu nil, dùng tab chính của session và tái sử dụng project đã lưu (hành vi tạo
// thủ công như cũ). tm có thể nil khi gọi từ worker queue (queue tự emit tiến độ).
func GenerateFlowVideo(
	ctx context.Context,
	session *BrowserSession,
	workerPage *rod.Page,
	tm *TaskManager,
	req GenerateRequest,
	milestone func(step string),
) ([]string, error) {
	// logDebug gắn LogPrefix (ví dụ "[W1 Clip #2] ") vào đầu mỗi dòng để phân biệt
	// luồng nào khi nhiều tab chạy song song. Rỗng → log như cũ.
	logDebug := func(msg string, args ...interface{}) {
		flowLogf(req.LogPrefix+msg, args...)
	}
	// mile phát một bước tiến trình NGẮN GỌN ra card log ở giao diện (nếu có callback).
	// Khác với logDebug (ghi chi tiết vào file): mile chỉ báo mốc quan trọng cho người dùng.
	mile := func(step string) {
		if milestone != nil {
			milestone(step)
		}
	}

	// Chuẩn hóa danh sách ảnh đính kèm: tự động nạp InputImagePath vào InputImagePaths nếu chưa có
	if len(req.InputImagePaths) == 0 && req.InputImagePath != "" {
		req.InputImagePaths = []string{req.InputImagePath}
	}

	// forceNewProject: worker queue (workerPage != nil) luôn tạo project mới riêng.
	forceNewProject := workerPage != nil

	sleep := func(base time.Duration) {
		time.Sleep(time.Duration(float64(base) * FlowActionDelayMultiplier))
	}

	// Đọc link project cũ đã lưu (chỉ dùng cho tab chính; worker luôn tạo mới)
	targetURL := "https://labs.google/fx/vi/tools/flow"
	if !forceNewProject {
		savedURL := readProjectURL()
		if savedURL != "" && strings.Contains(savedURL, "/project/") {
			targetURL = savedURL
			logDebug("Phát hiện link dự án cũ đã lưu: %s. Tiến hành mở dự án này...", savedURL)
		} else {
			logDebug("Không có link dự án cũ hoặc không hợp lệ. Tiến hành mở trang chủ...")
		}
	} else {
		logDebug("Worker song song: luôn tạo dự án Flow mới riêng cho tab này.")
	}

	var page *rod.Page
	var err error
	if workerPage != nil {
		page = workerPage
	} else {
		page, err = session.GetPage()
		if err != nil {
			return nil, err
		}
	}

	// findElemTimeout tìm element bằng JS NHƯNG có timeout — tránh treo vô hạn.
	// page.ElementByJS mặc định CHỜ MÃI đến khi element xuất hiện; ở chế độ ẩn trình
	// duyệt (headless) nút tải/độ phân giải có thể render chậm hoặc khác → treo cứng.
	// Bọc context có deadline để trả lỗi thay vì đứng im. Trả (nil, err) nếu quá hạn.
	findElemTimeout := func(timeout time.Duration, js string, args ...interface{}) (*rod.Element, error) {
		subCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		el, err := page.Context(subCtx).ElementByJS(rod.Eval(js, args...))
		if err != nil {
			return nil, err
		}
		return el.Context(ctx), nil
	}

	// Script ẩn danh (stealth) chống phát hiện webdriver/bot đã được đăng ký MỘT LẦN
	// lúc tạo page (applyStealthScript trong browser.go) và tồn tại qua mọi navigate.

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

	// Worker song song luôn tạo project mới riêng → ép hasProject=false dù URL hiện
	// tại tình cờ đang ở trong một project nào đó.
	if forceNewProject {
		hasProject = false
	}

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

	if !hasProject {
		logDebug("Dự án cũ không hoạt động hoặc không có. Tiến hành click tạo dự án mới...")
		tm.EmitStatus(TaskStateReady, "Đang vào không gian làm việc (tạo dự án mới)...", 21)

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
		
		// Đã tạo dự án mới thành công! Lưu link project mới này.
		// Worker song song KHÔNG ghi đè link chung (mỗi tab một project riêng, không
		// lưu để tránh các tab tranh chấp cùng một link project trong settings.json).
		info, _ = page.Info()
		if !forceNewProject {
			logDebug("Lưu link dự án mới vào settings.json: %s", info.URL)
			saveProjectURL(info.URL)
		} else {
			logDebug("Worker song song: đã tạo project riêng %s (không lưu link chung).", info.URL)
		}
	} else {
		// Kiểm tra xem dự án cũ có vượt quá ngưỡng ảnh cho phép hay không
		{
			totalCount := countProjectImages(page)
			logDebug("Số lượng hình ảnh hiện có trong dự án cũ: %d", totalCount)
			if totalCount >= MaxProjectImages {
				logDebug("Dự án hiện tại đã có %d hình ảnh (vượt ngưỡng %d ảnh để tránh lag). Xóa dự án cũ và tiến hành tự động tạo dự án mới ngay lập tức...", totalCount, MaxProjectImages)
				saveProjectURL("")
				
				// Quay lại trang chủ để bấm Tạo dự án mới ngay lập tức
				_ = page.Navigate("https://labs.google/fx/vi/tools/flow")
				_ = page.WaitDOMStable(2*time.Second, 0.5)

				logDebug("Tiến hành click tạo dự án mới thay thế cho dự án cũ quá %d ảnh...", MaxProjectImages)
				btn, errJS := page.ElementByJS(findBtnJS)
				if errJS == nil && btn != nil {
					_ = btn.ScrollIntoView()
					if errClick := btn.Click(proto.InputMouseButtonLeft, 1); errClick != nil {
						_, _ = btn.Eval("function() { this.click(); }")
					}
				}
				_ = page.WaitDOMStable(1*time.Second, 0.5)

				// Đợi chuyển hướng sang /project/
				for idx := 0; idx < 20; idx++ {
					info, err = page.Info()
					if err == nil && strings.Contains(info.URL, "/project/") {
						break
					}
					if idx > 0 && idx%3 == 0 {
						if retryBtn, errRetry := page.ElementByJS(findBtnJS); errRetry == nil && retryBtn != nil {
							_, _ = retryBtn.Eval("function() { this.click(); }")
						}
					}
					time.Sleep(750 * time.Millisecond)
				}

				info, _ = page.Info()
				if strings.Contains(info.URL, "/project/") {
					logDebug("Đã tạo dự án mới thay thế thành công! URL: %s", info.URL)
					saveProjectURL(info.URL)
				}
			} else {
				logDebug("Dự án cũ hoạt động bình thường! Tiếp tục sử dụng.")
			}
		}
	}
	logDebug("Đã vào dự án thành công!")
	mile("Đã vào dự án Flow")

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
	if err := ConfigureFlowSettings(ctx, page, req, tm, mile); err != nil {
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

		// 3. Đã xuất hiện thẻ ảnh đang tạo: hoặc có phần trăm tiến trình dạng "NN%",
		// hoặc caption trùng đúng prompt hiện ra NGOÀI ô nhập liệu. Đây là tín hiệu
		// chắc chắn nhất rằng prompt đã được gửi — dùng để chặn cú submit rỗng lần 2
		// khi chạy song song nhiều tab (React xóa ô prompt chậm hơn poll).
		startedObj, errStarted := page.Eval(`(p) => {
			const pctRe = /^\d{1,3}%$/;
			const nodes = Array.from(document.querySelectorAll('div, span'));
			for (const el of nodes) {
				const rect = el.getBoundingClientRect();
				if (rect.width <= 0 || rect.height <= 0) continue;
				const t = (el.textContent || '').trim();
				if (pctRe.test(t)) return true;
			}
			if (p) {
				const want = p.trim();
				for (const el of nodes) {
					if (el.isContentEditable) continue;
					if (el.closest && el.closest('[contenteditable="true"]')) continue;
					const rect = el.getBoundingClientRect();
					if (rect.width <= 0 || rect.height <= 0) continue;
					if ((el.textContent || '').trim() === want) return true;
				}
			}
			return false;
		}`, req.Prompt)
		if errStarted == nil && startedObj != nil && startedObj.Value.Bool() {
			return true
		}
		return false
	}

	// waitSent poll isPromptSent nhiều lần trong khoảng total thay vì kiểm tra 1
	// lần rồi bỏ cuộc — tránh race dưới tải song song khi React xóa ô prompt trễ.
	waitSent := func(total time.Duration) bool {
		deadline := time.Now().Add(total)
		for time.Now().Before(deadline) {
			if isPromptSent() {
				return true
			}
			time.Sleep(300 * time.Millisecond)
		}
		return isPromptSent()
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
	logDebug("Số lượng hình ảnh đang có trong dự án trước khi tạo: %d ảnh", countProjectImages(page))

	// Count initial download buttons and video tile cards if it is Video type
	initialVideoButtonsCount := 0
	initialVideoTilesCount := 0
	if req.MediaType == MediaTypeVideo {
		initialVideoButtonsCount, initialVideoTilesCount = countVideoButtonsAndTiles(page)
		logDebug("Số lượng nút tải video cũ: %d, Số lượng card video cũ: %d", initialVideoButtonsCount, initialVideoTilesCount)
	}

	maxAttempts := MaxGenerateAttempts
	var finalErr error

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if attempt > 1 {
			logDebug("Phát hiện lỗi từ Google Flow hoặc quá trình tạo thất bại. Tự động gửi lại prompt (Thử lại lần %d/%d)...", attempt, maxAttempts)
			tm.EmitStatus(TaskStateSubmitting, fmt.Sprintf("Gặp lỗi, đang tự động gửi lại prompt (Lần %d/3)...", attempt), 35)
			mile(fmt.Sprintf("⚠ Gặp lỗi, thử lại lần %d/%d...", attempt, maxAttempts))
			
			// Refind prompt input
			promptInput, err = FindFirstVisible(ctx, page, FlowSelectors.PromptInputs, 5*time.Second)
			if err != nil {
				finalErr = fmt.Errorf("không tìm thấy ô nhập liệu khi gửi lại: %w", err)
				continue
			}

			// Cập nhật lại số lượng nút tải & card video cũ trước khi retry lần tiếp theo
			if req.MediaType == MediaTypeVideo {
				initialVideoButtonsCount, initialVideoTilesCount = countVideoButtonsAndTiles(page)
				logDebug("Cập nhật lại số lượng nút tải cũ (%d) và card video cũ (%d) trước khi retry", initialVideoButtonsCount, initialVideoTilesCount)
			}
		}

		// KHÓA NHẬP LIỆU (dán ảnh → điền prompt → bấm gửi) cho toàn bộ giai đoạn này.
		// Clipboard Windows chỉ có 1 và Ctrl+V thật cần cửa sổ foreground, nên CẢ việc
		// dán ảnh LẪN điền prompt (FillFlowPrompt cũng dùng OS clipboard + Ctrl+V) đều
		// phải tuần tự giữa các cửa sổ. Nếu chỉ khóa lúc dán ảnh, worker khác sẽ đè ảnh
		// lên clipboard đúng lúc worker này đang dán TEXT prompt → dán nhầm ảnh, Slate
		// không nhận text, fill thất bại, không bao giờ tới bước bấm Gửi (đúng lỗi log).
		// Chỉ phần TẠO ẢNH (30-90s) mới chạy song song — nhả khóa ngay trước vòng chờ đó.
		session.LockPaste()
		inputLocked := true
		unlockInput := func() {
			if inputLocked {
				inputLocked = false
				session.UnlockPaste()
			}
		}
		defer unlockInput()
		// Đưa cửa sổ này lên foreground để Ctrl+V thật ăn đúng vào nó (chỉ khi hiện trình duyệt).
		if req.ShowChrome {
			_, _ = page.Activate()
		}

		// THỨ TỰ QUAN TRỌNG: ĐIỀN PROMPT TRƯỚC, DÁN ẢNH SAU.
		// Trước đây dán ảnh trước → ô Slate có thẻ ảnh (void node) → caret kẹt tại đó
		// → mọi cách chèn text đều bị Slate từ chối ("Slate không nhận text") → retry
		// làm loạn/mất ảnh. Điền text khi ô CÒN TRỐNG thì Slate nhận dễ nhất; sau đó
		// mới dán ảnh (giống người thật: gõ mô tả xong mới đính ảnh) nên text không bị phá.

		// 1. Điền văn bản prompt vào ô nhập liệu (khi ô còn trống)
		logDebug("Bắt đầu điền prompt vào ô nhập liệu: '%s'...", req.Prompt)
		tm.EmitStatus(TaskStateSubmitting, "Đang điền prompt...", 35)
		mile("Đang điền prompt...")
		err = FillFlowPrompt(page, promptInput, req.Prompt, logDebug)
		if err != nil {
			unlockInput()
			logDebug("Lỗi khi điền prompt: %v", err)
			mile("✗ Không điền được prompt (Slate không nhận text)")
			finalErr = fmt.Errorf("fill prompt: %w", err)
			continue
		}

		// 2. Dán ảnh vào ô prompt.
		// ƯU TIÊN cách JS thuần (PasteImageViaJS): dựng File+DataTransfer rồi bắn 'paste'
		// event thẳng vào editor — KHÔNG cần clipboard OS/foreground nên chạy được CẢ KHI
		// ẨN trình duyệt. Chỉ khi đính JS thất bại (Google không nhận event tổng hợp) mới
		// fallback về Ctrl+V native, và Ctrl+V chỉ có tác dụng khi cửa sổ đang hiện.
		if len(req.InputImagePaths) > 0 {
			logDebug("Phát hiện yêu cầu gửi kèm ảnh (%d ảnh). Thử dán bằng JS (chạy được cả khi ẩn)...", len(req.InputImagePaths))
			mile(fmt.Sprintf("Đang đính %d ảnh đầu vào...", len(req.InputImagePaths)))

			for _, imgPath := range req.InputImagePaths {
				_ = PasteImageViaJS(ctx, page, promptInput, imgPath, logDebug)
			}

			logDebug("Đã bắn paste JS. Đang chờ thẻ ảnh hiển thị trong ô prompt...")
			attachedJS := WaitUntilImageAttachedAndLoaded(ctx, page, 15*time.Second, logDebug)

			// Fallback: JS không đính được VÀ đang hiện trình duyệt → thử Ctrl+V thật.
			if !attachedJS && req.ShowChrome {
				logDebug("Dán JS chưa đính được ảnh. Fallback sang Ctrl+V native (chỉ dùng khi hiện trình duyệt)...")
				mile("Ảnh chưa đính, thử lại bằng Ctrl+V...")
				for _, imgPath := range req.InputImagePaths {
					_ = PasteImageNativeCtrlV(ctx, page, promptInput, imgPath, logDebug)
				}
				WaitUntilImageAttachedAndLoaded(ctx, page, 50*time.Second, logDebug)
			} else if !attachedJS {
				logDebug("CẢNH BÁO: Dán JS chưa xác nhận đính ảnh và đang ẩn trình duyệt (không thể fallback Ctrl+V). Vẫn tiếp tục gửi prompt...")
			}
			sleep(1500 * time.Millisecond) // Chờ thêm 1.5 giây cho React cập nhật state

			// Quét tìm lại ô prompt sau khi đính kèm ảnh
			promptInput, err = FindFirstVisible(ctx, page, FlowSelectors.PromptInputs, 5*time.Second)
			if err != nil {
				unlockInput()
				mile("✗ Không tìm thấy ô nhập sau khi đính ảnh")
				finalErr = fmt.Errorf("không tìm thấy ô nhập liệu sau khi upload ảnh: %w", err)
				continue
			}
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

		// QUAN TRỌNG (chạy song song): sau mỗi cách gửi phải RE-CHECK isPromptSent
		// trước khi escalate sang cách mạnh hơn. Dưới tải nhiều tab, React xóa ô
		// prompt trễ nên nếu chỉ chờ cứng rồi bấm tiếp sẽ bấm submit lên ô ĐÃ trống
		// → Google báo "Bạn phải cung cấp câu lệnh". waitSent poll nhiều lần để tránh.

		// 1. Try submitting by pressing Enter key on the keyboard
		logDebug("Thử gửi bằng phím Enter ảo...")
		_ = page.Keyboard.Press('\r')

		if waitSent(2500 * time.Millisecond) {
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

			if waitSent(2500 * time.Millisecond) {
				logDebug("Đã gửi prompt thành công qua sự kiện keydown Enter!")
			} else if isPromptSent() {
				// Chốt chặn cuối trước khi click nút: nếu generation đã bắt đầu thì
				// tuyệt đối KHÔNG click submit nữa (tránh cú gửi rỗng lần 2).
				logDebug("Prompt đã được gửi (phát hiện muộn). Bỏ qua bước click nút gửi.")
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

				// Re-check ngay trước khi click: nếu vừa gửi xong trong lúc quét thì thôi.
				if isPromptSent() {
					logDebug("Prompt đã gửi ngay trước khi click nút. Bỏ qua click.")
				} else if errSubmit == nil && submitBtn != nil {
					logDebug("Tiến hành click nút gửi...")
					_ = submitBtn.Click(proto.InputMouseButtonLeft, 1)
					_ = waitSent(1500 * time.Millisecond)
				} else {
					logDebug("Phương án quét JS thất bại. Thử dùng danh sách Selectors dự phòng...")
					submitBtnFb, errFb := FindFirstVisible(ctx, page, FlowSelectors.SubmitButtons, 3*time.Second)
					if isPromptSent() {
						logDebug("Prompt đã gửi ngay trước khi click nút dự phòng. Bỏ qua click.")
					} else if errFb == nil && submitBtnFb != nil {
						_ = submitBtnFb.Click(proto.InputMouseButtonLeft, 1)
						_ = waitSent(1500 * time.Millisecond)
					} else if errFb != nil {
						logDebug("Lỗi tìm nút gửi dự phòng: %v", errFb)
					}
				}
			}
		}

		// Đã bấm gửi xong: nhả khóa nhập liệu để worker khác bắt đầu dán ảnh/điền
		// prompt của nó. Phần còn lại (chờ tạo 30-90s) chạy song song thoải mái.
		unlockInput()
		if req.MediaType == MediaTypeVideo {
			mile("Đã gửi prompt, đang chờ AI tạo video...")
		} else {
			mile("Đã gửi prompt, đang chờ AI tạo ảnh...")
		}

		generationFailed := false

		if req.MediaType == MediaTypeImage {
			tm.EmitStatus(TaskStateGenerating, "Google AI đang tạo hình ảnh...", 50)

			// Số ảnh kỳ vọng theo Batch Size (1x/2x/3x/4x). Google Flow render từng ảnh
			// một nên ta phải CHỜ ĐỦ số ảnh này rồi mới chốt, thay vì thấy ảnh đầu tiên
			// đã dừng (bug cũ: 2x/4x chỉ bắt được 1 tấm rồi đóng tab).
			expectedImages := 1
			if d := strings.TrimSpace(strings.ToLower(strings.ReplaceAll(req.BatchSize, "x", ""))); d != "" {
				if n, errConv := strconv.Atoi(d); errConv == nil && n > 0 {
					expectedImages = n
				}
			}
			logDebug("Batch size yêu cầu: %s → chờ đủ %d ảnh mới sinh.", req.BatchSize, expectedImages)

			var previewBase64s []string
			var directUrls []string
			tickerImg := time.NewTicker(2 * time.Second)

			deadlineImg := time.Now().Add(ImageGenerateTimeout)
			pollCount := 0
			allErrorPolls := 0 // số poll liên tiếp thấy thẻ lỗi mà KHÔNG có ảnh thành công nào

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

				// Nhịp báo ra log card mỗi ~14s (7 vòng * 2s) để người dùng biết luồng
				// VẪN đang chạy trong lúc chờ AI tạo (giai đoạn im lặng dài nhất 30-90s).
				if pollCount > 0 && pollCount % 7 == 0 {
					mile(fmt.Sprintf("Vẫn đang chờ AI tạo ảnh... (%ds)", pollCount*2))
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

				// Chỉ kiểm tra thẻ báo lỗi sau ít nhất 6 giây (pollCount >= 3) để đảm bảo thẻ ảnh mới đã được nạo vào DOM.
				// Quét toàn bộ nhóm thẻ MỚI (kể từ ảnh cũ đầu tiên) để đếm số ảnh THÀNH CÔNG và số thẻ LỖI.
				// Chỉ retry cả batch khi toàn bộ đều lỗi (0 ảnh thành công); nếu có ít nhất 1 ảnh thành công
				// thì bỏ qua thẻ lỗi và để luồng gom ảnh xử lý batch thiếu (Google lọc bớt vài ảnh vi phạm).
				if pollCount >= 3 {
					batchStateObj, _ := page.Eval(`(lastGroupUrls) => {
						const tiles = Array.from(document.querySelectorAll('div[data-tile-id], div[role="button"][aria-roledescription="draggable"]'));
						if (tiles.length === 0) return { errorTiles: 0, successImgs: 0, generating: false };

						const lastSet = new Set(lastGroupUrls || []);
						const errorKeywords = [
							'lượt tạo này có thể vi phạm',
							'vi phạm các chính sách',
							'vi phạm chính sách',
							'policy violation',
							'thử một câu lệnh khác'
						];

						let errorTiles = 0;
						let successImgs = 0;
						let generating = false;

						// Chỉ xét các thẻ MỚI ở đầu danh sách: dừng khi chạm thẻ chứa ảnh cũ.
						for (const tile of tiles) {
							const txt = tile.textContent.trim().toLowerCase();

							// Thẻ đang loading (%) → vẫn đang tạo, chưa kết luận.
							if (/\d{1,2}%/.test(txt) || txt.includes('đang tạo') || txt.includes('generating')) {
								generating = true;
								continue;
							}

							const img = tile.querySelector('img[src*="getMediaUrl"], img[src*="/fx/api/"]');
							if (img) {
								let src = img.getAttribute('src') || '';
								if (src.startsWith('/')) src = 'https://labs.google' + src;
								// Chạm ảnh cũ đã có từ trước → hết nhóm mới, dừng quét.
								if (lastSet.size > 0 && lastSet.has(src)) break;
								successImgs++;
								continue;
							}

							const hasErrorTxt = errorKeywords.some(kw => txt.includes(kw));
							const warningEl = Array.from(tile.querySelectorAll('i, span')).find(el => {
								return el.textContent.trim().toLowerCase() === 'warning';
							});
							if (hasErrorTxt || warningEl) {
								errorTiles++;
							}
						}

						return { errorTiles, successImgs, generating };
					}`, lastGroupUrls)

					errorTiles := 0
					successImgs := 0
					generating := false
					if batchStateObj != nil {
						errorTiles = batchStateObj.Value.Get("errorTiles").Int()
						successImgs = batchStateObj.Value.Get("successImgs").Int()
						generating = batchStateObj.Value.Get("generating").Bool()
					}

					// Có ảnh thành công → không retry dù có thẻ lỗi. Reset bộ đếm lỗi.
					if successImgs > 0 {
						allErrorPolls = 0
					} else if errorTiles > 0 && !generating {
						// Toàn bộ batch lỗi, không còn thẻ đang tạo → xác nhận qua 2 poll liên tiếp
						// (tránh chốt lỗi khi DOM đang render dở), rồi mới retry.
						allErrorPolls++
						logDebug("Batch chỉ có thẻ lỗi, chưa có ảnh thành công (%d poll liên tiếp, %d thẻ lỗi).", allErrorPolls, errorTiles)
						if allErrorPolls >= 2 {
							logDebug("Xác nhận TOÀN BỘ batch lỗi / vi phạm chính sách (0 ảnh thành công). Kích hoạt thử lại tự động.")
							finalErr = NewError(ErrGenerationFailed, "Google Flow báo lỗi / vi phạm chính sách tạo hình ảnh (toàn bộ batch).")
							generationFailed = true
							break
						}
					}
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
						logDebug("Phát hiện nhóm ảnh mới bắt đầu sinh! Ảnh đầu tiên đã xuất hiện, chờ đủ %d ảnh của batch...", expectedImages)

						// JS đọc danh sách URL nhóm ảnh MỚI (chưa có trong lastGroupUrls).
						readNewGroupJS := `(lastGroupUrls) => {
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
						}`

						// Chờ đủ số ảnh của batch. Google Flow render từng ảnh một nên phải
						// poll tới khi số ảnh mới == expectedImages, hoặc hết grace timeout
						// (trường hợp Google trả ít hơn do lọc chính sách một vài ảnh). Đếm
						// số vòng ảnh KHÔNG tăng thêm để chốt sớm nếu batch trả thiếu.
						newGroupUrls := []string{}
						graceDeadline := time.Now().Add(90 * time.Second)
						stableCount := 0
						for time.Now().Before(graceDeadline) {
							select {
							case <-ctx.Done():
								return nil, ctx.Err()
							default:
							}

							curObj, errCur := page.Eval(readNewGroupJS, lastGroupUrls)
							cur := []string{}
							if errCur == nil && curObj != nil {
								for _, v := range curObj.Value.Arr() {
									cur = append(cur, v.Str())
								}
							}

							if len(cur) >= expectedImages {
								newGroupUrls = cur[:expectedImages]
								logDebug("Đã đủ %d/%d ảnh của batch.", len(newGroupUrls), expectedImages)
								break
							}

							if len(cur) > len(newGroupUrls) {
								newGroupUrls = cur
								stableCount = 0
								logDebug("Đã sinh %d/%d ảnh, tiếp tục chờ...", len(cur), expectedImages)
							} else {
								stableCount++
								// Số ảnh đứng yên ~16 giây (8 vòng * 2s) → coi như batch chốt ở
								// mức này (Google có thể đã lọc bỏ vài ảnh vi phạm chính sách).
								if stableCount >= 8 && len(newGroupUrls) > 0 {
									logDebug("Số ảnh đứng yên ở %d (kỳ vọng %d). Chốt ở mức hiện có.", len(newGroupUrls), expectedImages)
									break
								}
							}
							sleep(2000 * time.Millisecond)
						}

						{
							if len(newGroupUrls) == 0 {
								// Grace timeout mà không bắt được ảnh nào → thử đọc lần cuối.
								if curObj, errCur := page.Eval(readNewGroupJS, lastGroupUrls); errCur == nil && curObj != nil {
									for _, v := range curObj.Value.Arr() {
										newGroupUrls = append(newGroupUrls, v.Str())
									}
								}
							}
							logDebug("Chốt nhóm ảnh mới sinh: %d ảnh (kỳ vọng %d).", len(newGroupUrls), expectedImages)

							// Kiểm tra tổng số ảnh trên toàn trang để tránh lag
							totalCount := countProjectImages(page)
							logDebug("Tổng số lượng hình ảnh đang có trong dự án: %d", totalCount)
							if totalCount >= MaxProjectImages {
								logDebug("Dự án hiện tại có %d hình ảnh (vượt ngưỡng %d ảnh để tránh lag Chrome). Tiến hành xóa link dự án cũ để lần sau tự động tạo dự án mới...", totalCount, MaxProjectImages)
								saveProjectURL("") // Xóa link project để lần sau tạo dự án mới tinh!
							}

							// Bắt đầu xử lý preview: Đảm bảo có tiền tố https://labs.google
							directUrls = nil
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

			tickerImg.Stop()

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
				mile(fmt.Sprintf("✓ Đã sinh %d ảnh, bắt đầu tải về...", len(previewBase64s)))
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

			// Helper fallback: Tải trực tiếp qua Fetch URL / Base64 nếu giao diện bị ẩn/lỗi
			downloadDirectImageFallback := func(selIdx int, customFileName string) (string, error) {
				_ = os.MkdirAll(req.OutputDir, 0755)
				filePath := filepath.Join(req.OutputDir, customFileName+".jpg")
				
				// 1. Thử tải trực tiếp qua Fetch API từ page context
				if selIdx < len(directUrls) && directUrls[selIdx] != "" {
					urlStr := directUrls[selIdx]
					logDebug("Đang tải trực tiếp qua Fetch API từ URL %s...", urlStr)
					b64Obj, errFetch := page.Eval(`async (url) => {
						try {
							const resp = await fetch(url);
							const blob = await resp.blob();
							return new Promise((resolve) => {
								const reader = new FileReader();
								reader.onloadend = () => resolve(reader.result);
								reader.readAsDataURL(blob);
							});
						} catch (e) {
							return "";
						}
					}`, urlStr)
					if errFetch == nil && b64Obj != nil {
						b64Str := b64Obj.Value.Str()
						if strings.HasPrefix(b64Str, "data:image") {
							idx := strings.Index(b64Str, ",")
							if idx != -1 {
								data, errDec := base64.StdEncoding.DecodeString(b64Str[idx+1:])
								if errDec == nil && len(data) > 0 {
									if errWrite := os.WriteFile(filePath, data, 0644); errWrite == nil {
										logDebug("Tải trực tiếp ảnh qua Fetch API thành công: %s", filePath)
										return filePath, nil
									}
								}
							}
						}
					}
				}

				// 2. Thử lưu từ Base64 preview nếu có
				if selIdx < len(previewBase64s) && previewBase64s[selIdx] != "" {
					b64Str := previewBase64s[selIdx]
					if strings.HasPrefix(b64Str, "data:image") {
						idx := strings.Index(b64Str, ",")
						if idx != -1 {
							data, errDec := base64.StdEncoding.DecodeString(b64Str[idx+1:])
							if errDec == nil && len(data) > 0 {
								if errWrite := os.WriteFile(filePath, data, 0644); errWrite == nil {
									logDebug("Lưu ảnh từ Base64 preview thành công: %s", filePath)
									return filePath, nil
								}
							}
						}
					}
				}
				return "", fmt.Errorf("không tải được ảnh bằng luồng trực tiếp")
			}

			for idx, selIdx := range selectedIndexes {
				customFileName := req.FileName
				if len(selectedIndexes) > 1 {
					customFileName = fmt.Sprintf("%s_%d", req.FileName, idx+1)
				}

				logDebug("Đang tải hình ảnh được chọn thứ %d/%d (chỉ mục trong nhóm: %d)...", idx+1, len(selectedIndexes), selIdx)
				mile(fmt.Sprintf("Đang mở chi tiết ảnh %d/%d...", idx+1, len(selectedIndexes)))

				// Click the image card at index `selIdx` among the newly generated images
				clickedObj, errClick := page.Eval(`(selIdx) => {
					const imgs = Array.from(document.querySelectorAll('img')).filter(img => {
						const src = img.getAttribute('src') || '';
						return src.includes('getMediaUrl') || src.includes('/fx/api/');
					});
					if (selIdx < imgs.length) {
						const img = imgs[selIdx];
						const card = img.closest('a, button, [data-tile-id], [role="button"]') || img;
						try { card.scrollIntoView({ block: 'center' }); } catch(e){}
						card.click();
						return true;
					}
					return false;
				}`, selIdx)

				if errClick != nil || clickedObj == nil || !clickedObj.Value.Bool() {
					logDebug("Không thể click vào ảnh thứ %d ở chỉ mục %d, chuyển sang tải trực tiếp...", idx+1, selIdx)
					if fp, errFb := downloadDirectImageFallback(selIdx, customFileName); errFb == nil {
						mile(fmt.Sprintf("✓ Đã tải xong ảnh %d/%d (Tải trực tiếp)", idx+1, len(selectedIndexes)))
						downloadedPaths = append(downloadedPaths, fp)
						continue
					}
					mile(fmt.Sprintf("⚠ Không mở được chi tiết ảnh %d/%d, bỏ qua", idx+1, len(selectedIndexes)))
					continue
				}

				sleep(1500 * time.Millisecond) // wait for overlay to open fully
				mile(fmt.Sprintf("Đã mở chi tiết ảnh %d/%d, tìm nút tải...", idx+1, len(selectedIndexes)))

				// 3. Find the download dropdown button in the detail overlay (chấp nhận mọi thẻ nút/icon/aria).
				// Timeout 5s: nếu quá 5s không tìm thấy nút (do trình duyệt bị ẩn/thu nhỏ), tự tua sang tải trực tiếp!
				dlBtn, errDlBtn := findElemTimeout(5*time.Second, `() => {
					const candidates = Array.from(document.querySelectorAll('button, a, div[role="button"]'));
					return candidates.find(el => {
						const icon = el.querySelector('i, span, [class*="icon"], .google-symbols, .material-icons');
						const iconText = icon ? icon.textContent.trim().toLowerCase() : '';
						const aria = (el.getAttribute('aria-label') || '').toLowerCase();
						const title = (el.getAttribute('title') || '').toLowerCase();
						const txt = el.textContent.trim().toLowerCase();
						return (
							iconText === 'download' || iconText === 'file_download' || iconText === 'tải xuống' ||
							aria.includes('download') || aria.includes('tải xuống') || aria.includes('tải về') ||
							title.includes('download') || title.includes('tải xuống') || title.includes('tải về') ||
							txt.includes('tải xuống') || txt.includes('download')
						);
					});
				}`)
				if errDlBtn != nil || dlBtn == nil {
					logDebug("Giao diện chi tiết không hiển thị nút tải (ẩn trình duyệt), chuyển sang tải trực tiếp...")
					if fp, errFb := downloadDirectImageFallback(selIdx, customFileName); errFb == nil {
						mile(fmt.Sprintf("✓ Đã tải xong ảnh %d/%d (Tải trực tiếp)", idx+1, len(selectedIndexes)))
						downloadedPaths = append(downloadedPaths, fp)
						// Close overlay
						_, _ = page.Eval(`() => {
							const closeBtn = Array.from(document.querySelectorAll('button')).find(el => {
								const txt = el.textContent.toLowerCase().trim();
								return txt === 'xong' || txt === 'đóng' || txt === 'close' || txt === 'done';
							});
							if (closeBtn) closeBtn.click();
						}`)
						sleep(1000 * time.Millisecond)
						continue
					}
					mile(fmt.Sprintf("⚠ Không thấy nút tải cho ảnh %d/%d, bỏ qua", idx+1, len(selectedIndexes)))
					continue
				}
				
				err = dlBtn.Click(proto.InputMouseButtonLeft, 1)
				if err != nil {
					logDebug("Lỗi click nút tải xuống: %v, chuyển sang tải trực tiếp...", err)
					if fp, errFb := downloadDirectImageFallback(selIdx, customFileName); errFb == nil {
						mile(fmt.Sprintf("✓ Đã tải xong ảnh %d/%d (Tải trực tiếp)", idx+1, len(selectedIndexes)))
						downloadedPaths = append(downloadedPaths, fp)
						continue
					}
					continue
				}
				
				sleep(1200 * time.Millisecond) // wait for dropdown to open
				
				// Get target resolution from request
				targetRes := strings.ToLower(req.Resolution)
				if targetRes == "" {
					targetRes = "1k" // default to 1K
				}

				logDebug("Đang chọn độ phân giải %s cho ảnh thứ %d/%d...", strings.ToUpper(targetRes), idx+1, len(selectedIndexes))
				mile(fmt.Sprintf("Chọn độ phân giải %s cho ảnh %d/%d...", strings.ToUpper(targetRes), idx+1, len(selectedIndexes)))

				// Click resolution button (1K, 2K, 4K) in dropdown. Timeout 4s.
				optionBtn, errOpt := findElemTimeout(4*time.Second, `(target) => {
					const items = Array.from(document.querySelectorAll('button, div[role="menuitem"], div[role="button"], li, span'));
					return items.find(el => {
						const txt = el.textContent.toLowerCase();
						return txt.includes(target);
					});
				}`, targetRes)

				if errOpt != nil || optionBtn == nil {
					logDebug("Không tìm thấy tùy chọn độ phân giải %s cho ảnh thứ %d, thử tìm tùy chọn 1K mặc định...", targetRes, idx+1)
					optionBtn, _ = findElemTimeout(3*time.Second, `() => {
						const items = Array.from(document.querySelectorAll('button, div[role="menuitem"], div[role="button"], li, span'));
						return items.find(el => el.textContent.toLowerCase().includes('1k'));
					}`)
				}
				
				if optionBtn == nil {
					logDebug("Không thể chọn độ phân giải cho ảnh thứ %d, chuyển sang tải trực tiếp...", idx+1)
					if fp, errFb := downloadDirectImageFallback(selIdx, customFileName); errFb == nil {
						mile(fmt.Sprintf("✓ Đã tải xong ảnh %d/%d (Tải trực tiếp)", idx+1, len(selectedIndexes)))
						downloadedPaths = append(downloadedPaths, fp)
						_, _ = page.Eval(`() => {
							const closeBtn = Array.from(document.querySelectorAll('button')).find(el => {
								const txt = el.textContent.toLowerCase().trim();
								return txt === 'xong' || txt === 'đóng' || txt === 'close' || txt === 'done';
							});
							if (closeBtn) closeBtn.click();
						}`)
						sleep(1000 * time.Millisecond)
						continue
					}
					mile(fmt.Sprintf("⚠ Không chọn được độ phân giải cho ảnh %d, bỏ qua", idx+1))
					continue
				}

				// Tuần tự hóa đoạn tải: WaitDownload ở cấp browser nên nhiều tab tải
				// cùng lúc dễ bắt nhầm file của nhau. Chỉ khóa quanh đoạn download ngắn.
				session.LockDownload()
				dlDir := session.DownloadDir()
				waitDownload := session.Browser().WaitDownload(dlDir)
				// Click resolution option to trigger download / upscaling
				_ = optionBtn.Click(proto.InputMouseButtonLeft, 1)
				logDebug("Đã click chọn độ phân giải %s. Đang chờ file được tải về máy...", strings.ToUpper(targetRes))
				mile(fmt.Sprintf("Đã chọn %s cho ảnh %d/%d, đang chờ tải file về...", strings.ToUpper(targetRes), idx+1, len(selectedIndexes)))
				filePath, errMove := WaitAndMoveDownload(ctx, waitDownload, dlDir, req.OutputDir, customFileName, MediaTypeImage)
				session.UnlockDownload()
				if errMove == nil {
					logDebug("Tải ảnh thứ %d/%d thành công (%s): %s", idx+1, len(selectedIndexes), strings.ToUpper(targetRes), filePath)
					mile(fmt.Sprintf("✓ Đã tải xong ảnh %d/%d (%s)", idx+1, len(selectedIndexes), strings.ToUpper(targetRes)))
					downloadedPaths = append(downloadedPaths, filePath)
				} else {
					logDebug("Lỗi khi tải/lưu ảnh %d: %v", idx+1, errMove)
					mile(fmt.Sprintf("⚠ Lỗi tải ảnh %d/%d: %v", idx+1, len(selectedIndexes), errMove))
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
			// Helper fallback: Tải trực tiếp video qua Fetch API / Media URL nếu nút tải bị ẩn/lỗi
			downloadDirectVideoFallback := func(customFileName string) (string, error) {
				_ = os.MkdirAll(req.OutputDir, 0755)
				filePath := filepath.Join(req.OutputDir, customFileName+".mp4")
				logDebug("Đang thử tải trực tiếp video qua Fetch API / Media URL...")
				mile("Đang thử tải trực tiếp video qua link media...")

				vObj, errFetch := page.Eval(`async () => {
					try {
						const videos = Array.from(document.querySelectorAll('video'));
						if (videos.length === 0) return null;
						const v = videos[videos.length - 1];
						let src = v.src || v.currentSrc || v.querySelector('source')?.src || '';
						if (!src) return null;
						if (src.startsWith('/')) {
							src = window.location.origin + src;
						}
						if (src.startsWith('http://') || src.startsWith('https://')) {
							return { type: 'http', src: src };
						}
						// Với blob: URL, fetch và convert sang Base64
						const resp = await fetch(src);
						const blob = await resp.blob();
						return new Promise((resolve) => {
							const reader = new FileReader();
							reader.onloadend = () => resolve({ type: 'base64', src: reader.result });
							reader.readAsDataURL(blob);
						});
					} catch(e) {
						return null;
					}
				}`)

				if errFetch == nil && vObj != nil {
					vType := vObj.Value.Get("type").Str()
					vSrc := vObj.Value.Get("src").Str()

					if vType == "http" && vSrc != "" {
						logDebug("Tải trực tiếp video từ HTTP URL: %s", vSrc)
						resp, errGet := http.Get(vSrc)
						if errGet == nil && resp.StatusCode == 200 {
							defer resp.Body.Close()
							out, errCreate := os.Create(filePath)
							if errCreate == nil {
								_, errCopy := io.Copy(out, resp.Body)
								_ = out.Close()
								if errCopy == nil {
									logDebug("Tải trực tiếp video từ HTTP URL thành công: %s", filePath)
									return filePath, nil
								}
							}
						}
					} else if vType == "base64" && strings.HasPrefix(vSrc, "data:") {
						idx := strings.Index(vSrc, ",")
						if idx != -1 {
							data, errDec := base64.StdEncoding.DecodeString(vSrc[idx+1:])
							if errDec == nil && len(data) > 0 {
								if errWrite := os.WriteFile(filePath, data, 0644); errWrite == nil {
									logDebug("Tải trực tiếp video từ Base64 thành công: %s", filePath)
									return filePath, nil
								}
							}
						}
					}
				}
				return "", fmt.Errorf("không tải được video bằng link trực tiếp")
			}

			expectedVideos := 1
			if req.BatchSize != "" {
				cleanDigit := strings.ToLower(strings.TrimSpace(strings.ReplaceAll(req.BatchSize, "x", "")))
				if n, errConv := strconv.Atoi(cleanDigit); errConv == nil && n > 1 {
					expectedVideos = n
				}
			}

			var downloadBtn *rod.Element
			ticker := time.NewTicker(3 * time.Second)

			deadline := time.Now().Add(VideoGenerateTimeout)
			submittedAt := time.Now()           // Thời điểm submit prompt
			const gracePeriod = 25 * time.Second // Không detect lỗi trong 25s đầu (chờ spinner)
			autoRetryCount := 0                  // Số lần đã bấm nút Thử lại trong Google Flow
			consecutiveErrPolls := 0             // Số poll liên tiếp thấy lỗi mà không có progress
			everWasGenerating := false           // Đã từng thấy spinner/progress?
			stableFinishedPolls := 0            // Số poll liên tiếp không còn spinner/phần trăm
			pollCount := 0
			for {
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-ticker.C:
					pollCount++
				}

				if pollCount % 3 == 1 {
					logDebug("Đang chờ Google AI tạo xong video... (Đã chờ %d giây)", pollCount*3)
				}

				// Nhịp báo ra log card mỗi ~15s (5 vòng * 3s) để người dùng biết luồng vẫn đang chạy
				if pollCount > 0 && pollCount % 5 == 0 {
					mile(fmt.Sprintf("Vẫn đang chờ AI tạo video... (%ds)", pollCount*3))
				}

				if time.Now().After(deadline) {
					finalErr = NewError(ErrGenerationTimeout, "Quá thời gian chờ tạo video từ Google Flow.")
					generationFailed = true
					break
				}

				// Quét trạng thái tổng hợp chỉ bằng 1 CDP call duy nhất (Quota, Spinner, Card lỗi, Tile count, Download buttons)
				pollStateObj, _ := page.Eval(`() => {
					// 1. Quota limit check
					const limitWords = ['đạt đến hạn mức', 'hạn mức sử dụng', 'quay lại vào ngày mai', 'reached your limit', 'quota exceeded'];
					const elements = Array.from(document.querySelectorAll('div, span, p'));
					let quotaMsg = "";
					for (const el of elements) {
						const txt = el.textContent.trim().toLowerCase();
						if (el.getBoundingClientRect().width > 0 && limitWords.some(w => txt.includes(w))) {
							quotaMsg = el.textContent.trim();
							break;
						}
					}

					// 2. Bắt spinner/progress/phần trăm % ('18%', '24%', '99%') thực sự đang quay trên leaf text node
					const isGenerating = Array.from(document.querySelectorAll('div, span, p, md-circular-progress, [role="progressbar"]')).some(el => {
						const tag = el.tagName.toLowerCase();
						if (tag === 'md-circular-progress' || el.getAttribute('role') === 'progressbar') return true;

						// Chỉ kiểm tra các text node lá nhỏ chứa % (tránh kẹt bởi class static)
						if (el.children.length === 0 || (el.children.length <= 2 && el.querySelectorAll('div, p').length === 0)) {
							const txt = (el.textContent || '').trim();
							if (/^\d{1,2}\s*%$/.test(txt) || /^\d{1,2}%/.test(txt)) return true;

							const lower = txt.toLowerCase();
							if (lower.includes('đang tạo') || lower.includes('generating') || lower.includes('processing')) return true;
						}
						return false;
					});

					// 3. Quét trực tiếp các thẻ card tile (div[data-tile-id]) trên trang để phát hiện card báo lỗi
					const tiles = Array.from(document.querySelectorAll('div[data-tile-id], div[role="button"][aria-roledescription="draggable"], div[class*="tile"], video')).filter(el => {
						const rect = el.getBoundingClientRect();
						return rect.width > 80 && rect.height > 80;
					});

					let foundErrorTile = false;
					let retryBtnFound = false;

					for (const tile of tiles) {
						const txt = (tile.textContent || '').toLowerCase();
						const hasWarningIcon = Array.from(tile.querySelectorAll('i, span')).some(icon => {
							const iconText = (icon.textContent || '').trim().toLowerCase();
							return iconText === 'warning' || iconText === 'error' || iconText === 'report_problem';
						});

						const hasErrorText = txt.includes('không thành công') || 
						                     txt.includes('lượng truy cập cao') || 
						                     txt.includes('hoạt động bất thường') || 
						                     txt.includes('thử lại sau') || 
						                     txt.includes('something went wrong') || 
						                     txt.includes('vi phạm');

						const btn = Array.from(tile.querySelectorAll('button, div[role="button"]')).find(b => {
							const bTxt = (b.textContent || '').trim().toLowerCase();
							const iTxt = (b.querySelector('i, span')?.textContent || '').trim().toLowerCase();
							const aria = (b.getAttribute('aria-label') || '').toLowerCase();
							const titleAttr = (b.getAttribute('title') || '').toLowerCase();
							const isVisible = b.getBoundingClientRect().width > 0 || b.offsetWidth > 0;
							const isRetryIcon = iTxt === 'refresh' || iTxt === 'restart_alt' || iTxt === 'autorenew' || iTxt === 'replay' || iTxt === 'rotate_right' || iTxt === 'sync' || iTxt === 'loop' || iTxt === 'update';
							return isVisible && (bTxt.includes('thử lại') || bTxt.includes('retry') || aria.includes('thử lại') || titleAttr.includes('thử lại') || isRetryIcon);
						});

						if ((hasWarningIcon || hasErrorText) && btn) {
							foundErrorTile = true;
							retryBtnFound = true;
							break;
						}
					}

					// Dự phòng: Tìm nút Thử lại trên toàn trang nếu tile quét thiếu
					if (!retryBtnFound) {
						const globalBtn = Array.from(document.querySelectorAll('button, div[role="button"]')).find(b => {
							const bTxt = (b.textContent || '').trim().toLowerCase();
							const iTxt = (b.querySelector('i, span')?.textContent || '').trim().toLowerCase();
							const aria = (b.getAttribute('aria-label') || '').toLowerCase();
							const titleAttr = (b.getAttribute('title') || '').toLowerCase();
							const isVisible = b.getBoundingClientRect().width > 0 || b.offsetWidth > 0;
							const isRetryIcon = iTxt === 'refresh' || iTxt === 'restart_alt' || iTxt === 'autorenew';
							return isVisible && (bTxt.includes('thử lại') || bTxt.includes('retry') || aria.includes('thử lại') || titleAttr.includes('thử lại') || isRetryIcon);
						});
						if (globalBtn) retryBtnFound = true;
					}

					// 4. Tìm số nút tải xuống
					const dlBtns = Array.from(document.querySelectorAll('button, a, div[role="button"]')).filter(el => {
						const icon = el.querySelector('i, span.google-symbols, .material-icons, [class*="icon"]');
						const iconText = icon ? icon.textContent.trim().toLowerCase() : '';
						const aria = (el.getAttribute('aria-label') || '').toLowerCase();
						const title = (el.getAttribute('title') || '').toLowerCase();
						const txt = el.textContent.trim().toLowerCase();
						return iconText === 'download' || iconText === 'file_download' || 
						       aria.includes('download') || aria.includes('tải xuống') || aria.includes('tải về') ||
						       title.includes('download') || title.includes('tải xuống') || title.includes('tải về') ||
						       txt.includes('tải xuống') || txt.includes('download');
					});

					return { 
						quotaMsg,
						isGenerating, 
						isError: foundErrorTile || (!isGenerating && retryBtnFound), 
						hasRetryBtn: retryBtnFound,
						tileCount: tiles.length,
						dlCount: dlBtns.length
					};
				}`)

				var isGen bool
				var isErr bool
				var hasRetry bool
				currentTileCount := 0
				currentDlCount := 0

				if pollStateObj != nil {
					quotaMsg := pollStateObj.Value.Get("quotaMsg").Str()
					if quotaMsg != "" && quotaMsg != "<nil>" {
						logDebug("Phát hiện cảnh báo giới hạn hạn mức từ Google Flow: %s", quotaMsg)
						return nil, NewError(ErrQuotaExceeded, quotaMsg)
					}

					isGen = pollStateObj.Value.Get("isGenerating").Bool()
					isErr = pollStateObj.Value.Get("isError").Bool()
					hasRetry = pollStateObj.Value.Get("hasRetryBtn").Bool()
					currentTileCount = pollStateObj.Value.Get("tileCount").Int()
					currentDlCount = pollStateObj.Value.Get("dlCount").Int()
				}

					if isGen {
						everWasGenerating = true
						stableFinishedPolls = 0
						consecutiveErrPolls = 0
						logDebug("Đang tạo video... (everWasGenerating=true)")
					} else {
						elapsedSec := time.Since(submittedAt).Seconds()
						if everWasGenerating || currentDlCount > initialVideoButtonsCount || (elapsedSec > 20 && currentTileCount > initialVideoTilesCount) {
							stableFinishedPolls++
							logDebug("Kiểm tra độ ổn định video hoàn thành (%d/2 poll)...", stableFinishedPolls)
						}
					}

					if !isGen && isErr && hasRetry {
						elapsedSec := time.Since(submittedAt).Seconds()
						consecutiveErrPolls++

						// Nếu đã từng thấy generating → lỗi thật → retry nhanh (2 poll)
						// Chưa từng thấy generating → có thể là card cũ → grace 25s + 3 poll
						needConsecutive := 3
						var needGrace float64 = gracePeriod.Seconds()
						if everWasGenerating {
							needConsecutive = 2
							needGrace = 0
						}

						logDebug("Lỗi+retry: everWasGenerating=%v, consecutive=%d/%d, elapsed=%.0fs/%.0fs",
							everWasGenerating, consecutiveErrPolls, needConsecutive, elapsedSec, needGrace)

						if elapsedSec < needGrace {
							logDebug("Trong grace period (%.0fs/%.0fs). Bỏ qua.", elapsedSec, needGrace)
						} else if consecutiveErrPolls < needConsecutive {
							logDebug("Chưa đủ %d poll liên tiếp (%d). Chờ...", needConsecutive, consecutiveErrPolls)
						} else if autoRetryCount < 3 {
							autoRetryCount++
							consecutiveErrPolls = 0
							logDebug("Bấm Thử lại (%d/3) trong Google Flow. everWasGenerating=%v", autoRetryCount, everWasGenerating)
							mile(fmt.Sprintf("⚠ Gặp lỗi, bấm Thử lại (%d/3)...", autoRetryCount))
							_, _ = page.Eval(`() => {
								const btn = Array.from(document.querySelectorAll('button, div[role="button"]')).find(b => {
									const txt = (b.textContent || '').trim().toLowerCase();
									const iTxt = (b.querySelector('i, span')?.textContent || '').trim().toLowerCase();
									const aria = (b.getAttribute('aria-label') || '').toLowerCase();
									const title = (b.getAttribute('title') || '').toLowerCase();
									const isVisible = (b.getBoundingClientRect().width > 0 && b.getBoundingClientRect().height > 0) || b.offsetWidth > 0;
									const isRetryIcon = iTxt === 'refresh' || iTxt === 'restart_alt' || iTxt === 'autorenew' || iTxt === 'replay' || iTxt === 'rotate_right' || iTxt === 'sync' || iTxt === 'loop' || iTxt === 'update';
									return isVisible && (txt.includes('thử lại') || txt.includes('retry') || aria.includes('thử lại') || title.includes('thử lại') || isRetryIcon);
								});
								if (btn) {
									btn.click();
									try { btn.dispatchEvent(new MouseEvent('click', { bubbles: true, cancelable: true, view: window })); } catch(e){}
									try { btn.dispatchEvent(new PointerEvent('pointerdown', { bubbles: true })); btn.dispatchEvent(new PointerEvent('pointerup', { bubbles: true })); } catch(e){}
									return true;
								}
								return false;
							}`)
							submittedAt = time.Now() // Reset grace period cho lần tạo mới
							everWasGenerating = false // Reset để theo dõi lần mới
							continue
						} else {
							logDebug("Đã bấm Thử lại 3/3 lần vẫn lỗi. Báo lỗi để outer loop re-submit.")
							finalErr = NewError(ErrGenerationFailed, "Google Flow báo lỗi tạo video sau 3 lần thử.")
							generationFailed = true
							break
						}
					} else if !isGen && !isErr {
						consecutiveErrPolls = 0
					}

				hasNewTile := currentTileCount > initialVideoTilesCount
				hasNewDlBtn := currentDlCount > initialVideoButtonsCount
				elapsedSec := time.Since(submittedAt).Seconds()

				// Phát hiện video đã hoàn thành: KHÔNG còn spinner/phần trăm (!isGen) VÀ % đã mất hoàn toàn
				isFinishedGenerating := !isGen && (stableFinishedPolls >= 1 || hasNewDlBtn || (elapsedSec > 20 && hasNewTile))

				if isFinishedGenerating {
					logDebug("Phát hiện video mới đã tạo xong sau %.0fs (hasNewTile=%v, hasNewDlBtn=%v, everWasGenerating=%v, tileCount=%d)! Bắt đầu mở chi tiết video...", elapsedSec, hasNewTile, hasNewDlBtn, everWasGenerating, currentTileCount)
					mile("✓ Đã sinh video, bắt đầu tải về...")
					mile("Đang mở chi tiết video...")

					// 1. Click vào thẻ video card mới nhất bằng Rod CDP Native Click (giúp kích hoạt chính xác event của Chrome)
					tileEl, errFindTile := page.ElementByJS(rod.Eval(`() => {
						const tiles = Array.from(document.querySelectorAll('[data-tile-id], a[href*="/edit/"], button:has(video), video')).filter(el => {
							const rect = el.getBoundingClientRect();
							return rect.width > 80 && rect.height > 80;
						});
						if (tiles.length === 0) return null;
						const last = tiles[tiles.length - 1];
						return last.closest('a, button, [data-tile-id]') || last;
					}`))
					if errFindTile == nil && tileEl != nil {
						_ = tileEl.ScrollIntoView()
						_ = tileEl.Click(proto.InputMouseButtonLeft, 1)
					} else {
						_, _ = page.Eval(`() => {
							const tiles = Array.from(document.querySelectorAll('[data-tile-id], div[role="button"][aria-roledescription="draggable"], div[class*="tile"], video, img')).filter(el => {
								const rect = el.getBoundingClientRect();
								return rect.width > 80 && rect.height > 80;
							});
							if (tiles.length > 0) {
								const lastTile = tiles[tiles.length - 1];
								const clickTarget = lastTile.closest('a, button, [data-tile-id], [role="button"]') || lastTile;
								clickTarget.click();
								try { clickTarget.dispatchEvent(new MouseEvent('click', { bubbles: true, cancelable: true, view: window })); } catch(e){}
							}
						}`)
					}

					sleep(1500 * time.Millisecond) // Chờ popup chi tiết mở ra

					// 2. Chờ nút Tải xuống trong popup chi tiết sẵn sàng (hết trạng thái disabled / mờ)
					for waitBtn := 0; waitBtn < 10; waitBtn++ {
						btn, errCheckDl := page.ElementByJS(rod.Eval(`() => {
							const candidates = Array.from(document.querySelectorAll('button, a, div[role="button"]'));
							const found = [];
							for (const el of candidates) {
								const icon = el.querySelector('i, span.google-symbols, .material-icons, [class*="icon"]');
								const iconText = icon ? icon.textContent.trim().toLowerCase() : '';
								const aria = (el.getAttribute('aria-label') || '').toLowerCase();
								const title = (el.getAttribute('title') || '').toLowerCase();
								const txt = el.textContent.trim().toLowerCase();
								const isVisible = el.getBoundingClientRect().width > 0 || el.offsetWidth > 0;
								const isDisabled = el.hasAttribute('disabled') || el.getAttribute('aria-disabled') === 'true' || el.classList.contains('disabled');
								if (isVisible && !isDisabled && (
									iconText === 'download' || iconText === 'file_download' || iconText === 'tải xuống' ||
									aria.includes('download') || aria.includes('tải xuống') || aria.includes('tải về') ||
									title.includes('download') || title.includes('tải xuống') || title.includes('tải về') ||
									txt.includes('tải xuống') || txt.includes('download')
								)) {
									found.push(el);
								}
							}
							return found.length > 0 ? found[found.length - 1] : null;
						}`))
						if errCheckDl == nil && btn != nil {
							downloadBtn = btn
							logDebug("Đã tìm thấy nút tải video sẵn sàng trong popup chi tiết!")
							break
						}
						sleep(1000 * time.Millisecond)
					}

					if downloadBtn == nil {
						logDebug("Không thấy nút tải khả dụng trong popup chi tiết, chuyển sang tải trực tiếp...")
					}
					break // Luôn thoát vòng lặp poll để tải ngay!
				}
			}
			ticker.Stop()

			if generationFailed {
				continue
			}

			if expectedVideos > 1 {
				mile(fmt.Sprintf("✓ Đã sinh %d video, bắt đầu tải về...", expectedVideos))
			} else {
				mile("✓ Đã sinh video, bắt đầu tải về...")
			}

			var downloadedPaths []string

			for idx := 0; idx < expectedVideos; idx++ {
				customFileName := req.FileName
				if expectedVideos > 1 {
					customFileName = fmt.Sprintf("%s_%d", req.FileName, idx+1)
					mile(fmt.Sprintf("Đang mở chi tiết video %d/%d...", idx+1, expectedVideos))
				} else {
					mile("Đang mở chi tiết video...")
				}

				// 1. Nếu đang ở trang chi tiết (/edit/), quay về trang lưới dự án trước khi chọn card mới
				_, _ = page.Eval(`() => {
					if (window.location.href.includes('/edit/')) {
						const backBtn = Array.from(document.querySelectorAll('button, a')).find(b => {
							const aria = (b.getAttribute('aria-label') || '').toLowerCase();
							const title = (b.getAttribute('title') || '').toLowerCase();
							const iTxt = (b.querySelector('i, span')?.textContent || '').trim().toLowerCase();
							const txt = (b.textContent || '').trim().toLowerCase();
							const isVisible = (b.getBoundingClientRect().width > 0 || b.offsetWidth > 0);
							return isVisible && (iTxt === 'arrow_back' || aria.includes('back') || aria.includes('quay lại') || title.includes('back') || txt === 'quay lại');
						});
						if (backBtn) {
							backBtn.click();
						} else {
							window.history.back();
						}
					}
				}`)
				sleep(1200 * time.Millisecond)

				// 2. Click vào thẻ video card thứ idx chính xác trong nhóm mới sinh (LỰA CHỌN CHÍNH XÁC PHẦN TỬ CON, KHÔNG BẤM NÚT CHA HÀNG HÀNG)
				tileEl, errFindTile := page.ElementByJS(rod.Eval(`(targetIdx, totalExp) => {
					// 1. Quét các thẻ card video trên lưới chính theo cấu trúc DOM thực tế (DevTools)
					const cards = Array.from(document.querySelectorAll('div[class*="38169"] > div, div[style*="width: 166"], div[style*="height: 296"]')).filter(el => {
						if (el.closest('[role="dialog"], [class*="modal"], [class*="overlay"]')) return false;
						const rect = el.getBoundingClientRect();
						return rect.width > 60 && rect.height > 60;
					});

					let tiles = cards;
					if (tiles.length < totalExp) {
						// Fallback: Quét tất cả thẻ chứa video/canvas hoặc icon play
						const media = Array.from(document.querySelectorAll('a[href*="/edit/"], video, canvas, i.google-symbols, span.google-symbols')).filter(el => {
							if (el.closest('[role="dialog"], [class*="modal"], [class*="overlay"]')) return false;
							const rect = el.getBoundingClientRect();
							return rect.width > 60 && rect.height > 60;
						});

						const unique = [];
						const seen = new Set();
						for (const m of media) {
							const r = m.getBoundingClientRect();
							const key = Math.round(r.left) + '_' + Math.round(r.top);
							if (!seen.has(key)) {
								seen.add(key);
								unique.push(m);
							}
						}
						if (unique.length > 0) tiles = unique;
					}

					if (tiles.length === 0) return null;

					// Lấy nhóm totalExp thẻ mới nhất ở cuối danh sách
					const batchTiles = tiles.slice(Math.max(0, tiles.length - totalExp));
					// Sắp xếp các thẻ trong batch từ trái sang phải (theo vị trí left) để đảm bảo index 0, 1, 2, 3 khớp đúng tuần tự visual
					batchTiles.sort((a, b) => a.getBoundingClientRect().left - b.getBoundingClientRect().left);

					const targetTile = batchTiles[targetIdx] || batchTiles[Math.min(targetIdx, batchTiles.length - 1)];
					if (!targetTile) return null;

					// BẤM TRỰC TIẾP VÀO PHẦN TỬ CON (CANVAS / VIDEO / ICON / IMG), KHÔNG NHẢY LÊN THẺ CHA CONTAINER HÀNG HÀNG
					const clickTarget = targetTile.querySelector('video, canvas, i, span, img') || targetTile;
					try { clickTarget.scrollIntoView({ block: 'center' }); } catch(e){}
					clickTarget.click();
					try { clickTarget.dispatchEvent(new MouseEvent('click', { bubbles: true, cancelable: true, view: window })); } catch(e){}
					return clickTarget;
				}`, idx, expectedVideos))

				if errFindTile == nil && tileEl != nil {
					_ = tileEl.Click(proto.InputMouseButtonLeft, 1)
				} else {
					_, _ = page.Eval(`(targetIdx, totalExp) => {
						const media = Array.from(document.querySelectorAll('video, canvas')).filter(el => {
							if (el.closest('[role="dialog"], [class*="modal"], [class*="overlay"]')) return false;
							const rect = el.getBoundingClientRect();
							return rect.width > 60 && rect.height > 60;
						});
						if (media.length > 0) {
							const batchMedia = media.slice(Math.max(0, media.length - totalExp));
							batchMedia.sort((a, b) => a.getBoundingClientRect().left - b.getBoundingClientRect().left);
							const target = batchMedia[targetIdx] || batchMedia[batchMedia.length - 1];
							const clickTarget = target.querySelector('video, canvas, i, span, img') || target;
							clickTarget.click();
							try { clickTarget.dispatchEvent(new MouseEvent('click', { bubbles: true, cancelable: true, view: window })); } catch(e){}
						}
					}`, idx, expectedVideos)
				}

				sleep(1500 * time.Millisecond)

				// 2. Chờ nút Tải xuống trong popup chi tiết sẵn sàng
				var btnDl *rod.Element
				for waitBtn := 0; waitBtn < 10; waitBtn++ {
					btn, errCheckDl := page.ElementByJS(rod.Eval(`() => {
						const candidates = Array.from(document.querySelectorAll('button, a, div[role="button"]'));
						const found = [];
						for (const el of candidates) {
							const icon = el.querySelector('i, span.google-symbols, .material-icons, [class*="icon"]');
							const iconText = icon ? icon.textContent.trim().toLowerCase() : '';
							const aria = (el.getAttribute('aria-label') || '').toLowerCase();
							const title = (el.getAttribute('title') || '').toLowerCase();
							const txt = el.textContent.trim().toLowerCase();
							const isVisible = el.getBoundingClientRect().width > 0 || el.offsetWidth > 0;
							const isDisabled = el.hasAttribute('disabled') || el.getAttribute('aria-disabled') === 'true' || el.classList.contains('disabled');
							if (isVisible && !isDisabled && (
								iconText === 'download' || iconText === 'file_download' || iconText === 'tải xuống' ||
								aria.includes('download') || aria.includes('tải xuống') || aria.includes('tải về') ||
								title.includes('download') || title.includes('tải xuống') || title.includes('tải về') ||
								txt.includes('tải xuống') || txt.includes('download')
							)) {
								found.push(el);
							}
						}
						return found.length > 0 ? found[found.length - 1] : null;
					}`))
					if errCheckDl == nil && btn != nil {
						btnDl = btn
						break
					}
					sleep(1000 * time.Millisecond)
				}

				dlSuccess := false
				if btnDl != nil {
					if expectedVideos > 1 {
						tm.EmitStatus(TaskStateDownloading, fmt.Sprintf("Đang tải video %d/%d...", idx+1, expectedVideos), 85)
					} else {
						tm.EmitStatus(TaskStateDownloading, "Đang tải video xuống...", 85)
					}
					session.LockDownload()
					dlDir := session.DownloadDir()
					waitDownload := session.Browser().WaitDownload(dlDir)
					_ = btnDl.Click(proto.InputMouseButtonLeft, 1)

					filePath, errMove := WaitAndMoveDownload(ctx, waitDownload, dlDir, req.OutputDir, customFileName, MediaTypeVideo)
					session.UnlockDownload()

					if errMove == nil {
						logDebug("Tải video %d/%d thành công: %s", idx+1, expectedVideos, filePath)
						if expectedVideos > 1 {
							mile(fmt.Sprintf("✓ Đã tải xong video %d/%d", idx+1, expectedVideos))
						} else {
							mile("✓ Đã tải xong video")
						}
						downloadedPaths = append(downloadedPaths, filePath)
						dlSuccess = true
					}
				}

				if !dlSuccess {
					logDebug("Thử tải trực tiếp video %d/%d qua Fetch API...", idx+1, expectedVideos)
					if fp, errFb := downloadDirectVideoFallback(customFileName); errFb == nil {
						if expectedVideos > 1 {
							mile(fmt.Sprintf("✓ Đã tải xong video %d/%d (Tải trực tiếp)", idx+1, expectedVideos))
						} else {
							mile("✓ Đã tải xong video (Tải trực tiếp)")
						}
						downloadedPaths = append(downloadedPaths, fp)
					} else if expectedVideos > 1 {
						mile(fmt.Sprintf("⚠ Lỗi tải video %d/%d", idx+1, expectedVideos))
					}
				}

				// Quay lại trang lưới dự án (Back Arrow ← / history.back) sau khi tải xong từng video
				_, _ = page.Eval(`() => {
					// 1. Click nút Back Arrow (←) ở góc trên bên trái trang Google Flow edit
					const backBtn = Array.from(document.querySelectorAll('button, a')).find(b => {
						const aria = (b.getAttribute('aria-label') || '').toLowerCase();
						const title = (b.getAttribute('title') || '').toLowerCase();
						const iTxt = (b.querySelector('i, span')?.textContent || '').trim().toLowerCase();
						const txt = (b.textContent || '').trim().toLowerCase();
						const isVisible = (b.getBoundingClientRect().width > 0 || b.offsetWidth > 0);
						return isVisible && (iTxt === 'arrow_back' || aria.includes('back') || aria.includes('quay lại') || title.includes('back') || txt === 'quay lại');
					});
					if (backBtn) {
						backBtn.click();
						try { backBtn.dispatchEvent(new MouseEvent('click', { bubbles: true, cancelable: true, view: window })); } catch(e){}
						return;
					}
					// 2. Dự phòng: history.back()
					if (window.location.href.includes('/edit/')) {
						window.history.back();
					}
				}`)
				sleep(1500 * time.Millisecond)
			}

			if len(downloadedPaths) > 0 {
				return downloadedPaths, nil
			}
			finalErr = fmt.Errorf("không có video nào được tải về thành công")
		}
	}

	if finalErr != nil {
		return nil, finalErr
	}
	return nil, fmt.Errorf("quá trình tạo nội dung Google AI không thành công sau %d lần thử", maxAttempts)
}

func ConfigureFlowSettings(ctx context.Context, page *rod.Page, req GenerateRequest, tm *TaskManager, milestone func(step string)) error {
	logDebug := func(msg string, args ...interface{}) {
		flowLogf(req.LogPrefix+msg, args...)
	}
	// mile phát 1 bước NGẮN GỌN ra card log ở giao diện (nil thì bỏ qua).
	mile := func(step string) {
		if milestone != nil {
			milestone(step)
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

	checkConfigMatches := func(btnText string) (isModelMatch, isRatioMatch, isBatchMatch, isDurationMatch, isMediaTypeMatch bool) {
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

		// 4. Kiểm tra Duration (4s, 6s, 8s) cho Video mode (khớp chính xác dạng '4s', tránh nhầm với 'x4')
		isDurationMatch = true
		if req.MediaType == MediaTypeVideo && req.Duration != "" {
			durDigit := strings.ToLower(strings.TrimSpace(strings.ReplaceAll(req.Duration, "s", "")))
			targetDurStr := durDigit + "s"
			isDurationMatch = strings.Contains(btnTextLower, targetDurStr)
		}

		checkIsMediaTypeMatch := true
		if req.MediaType == MediaTypeVideo {
			// Đang cần tạo video: nút phải chứa "video" hoặc "veo"
			checkIsMediaTypeMatch = strings.Contains(btnTextLower, "video") || strings.Contains(btnTextLower, "veo")
		} else {
			// Đang cần tạo ảnh: nút không được chứa "video" (trừ khi đồng thời có "banana"/"imagen"/"omni")
			hasImageKeyword := strings.Contains(btnTextLower, "banana") || strings.Contains(btnTextLower, "imagen") || strings.Contains(btnTextLower, "omni")
			if strings.Contains(btnTextLower, "video") && !hasImageKeyword {
				checkIsMediaTypeMatch = false
			}
		}

		return isModelMatch, isRatioMatch, isBatchMatch, isDurationMatch, checkIsMediaTypeMatch
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
					mile("⚠ Không tắt được nút Tác nhân")
				} else {
					logDebug("Đã tắt nút Tác nhân thành công sau lần click thứ 2.")
					mile("Đã tắt nút Tác nhân")
				}
			} else {
				logDebug("Đã tắt nút Tác nhân thành công (aria-pressed=false).")
				mile("Đã tắt nút Tác nhân")
			}
		} else {
			logDebug("Nút Tác nhân đã ở trạng thái TẮT (aria-pressed=false) sau khi đã chờ React load. Bỏ qua.")
			mile("Nút Tác nhân đã tắt sẵn")
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

	// 2. Tìm nút mở hộp thoại cấu hình (radix-:rn: / nút hiển thị model/video/tỷ lệ hiện tại ở thanh prompt)
	configTriggerBtn, err := getElement(5*time.Second, `() => {
		const buttons = Array.from(document.querySelectorAll('button'));
		const windowHeight = window.innerHeight || 800;

		// 1. Ưu tiên tìm nút menu cài đặt nằm ngay trong/cùng container với ô nhập prompt (Slate editor)
		try {
			const editor = document.querySelector('[data-slate-editor="true"], div[role="textbox"], textarea, [class*="prompt"]');
			if (editor) {
				let parent = editor.parentElement;
				for (let depth = 0; depth < 8 && parent && parent !== document.body; depth++) {
					const btns = Array.from(parent.querySelectorAll('button[aria-haspopup="menu"]'));
					const target = btns.find(b => {
						const txt = (b.textContent || '').trim().toLowerCase();
						const isAgent = txt.includes('tác nhân') || txt.includes('agent');
						const isVisible = b.getBoundingClientRect().width > 0;
						return isVisible && !isAgent;
					});
					if (target) return target;
					parent = parent.parentElement;
				}
			}
		} catch(e) {}

		// 2. Tìm nút ở NỬA DƯỚI màn hình có id^="radix-" và aria-haspopup="menu" (loại trừ nút Tác nhân & loại trừ header gear ở top)
		let btn = buttons.find(b => {
			const id = b.getAttribute('id') || '';
			const hasPopup = b.getAttribute('aria-haspopup') === 'menu';
			const txt = (b.textContent || '').trim().toLowerCase();
			const rect = b.getBoundingClientRect();
			const isVisible = rect.width > 0 && rect.height > 0;
			const isBottomArea = rect.top > (windowHeight * 0.3); // Loại bỏ top header settings gear ở góc trên màn hình
			const isAgent = txt.includes('tác nhân') || txt.includes('agent');

			const hasPromptConfigKeywords = txt.includes('video') || txt.includes('banana') || txt.includes('veo') || 
			                                txt.includes('imagen') || txt.includes('omni') || txt.includes('1x') || 
			                                txt.includes('2x') || txt.includes('3x') || txt.includes('4x') || 
			                                txt.includes('8s') || txt.includes('5s') || txt.includes('9:16') || 
			                                txt.includes('16:9') || txt.includes('1:1') || txt.includes('hình ảnh') || 
			                                txt.includes('image') || txt.includes('🍌');

			return isVisible && id.startsWith('radix-') && hasPopup && isBottomArea && !isAgent && hasPromptConfigKeywords;
		});
		if (btn) return btn;

		// 3. Dự phòng 2: Bất kỳ nút radix- menu nào ở nửa dưới màn hình (không phải Tác nhân)
		btn = buttons.find(b => {
			const id = b.getAttribute('id') || '';
			const hasPopup = b.getAttribute('aria-haspopup') === 'menu';
			const txt = (b.textContent || '').trim().toLowerCase();
			const rect = b.getBoundingClientRect();
			const isVisible = rect.width > 0;
			const isBottomArea = rect.top > (windowHeight * 0.3);
			const isAgent = txt.includes('tác nhân') || txt.includes('agent');
			return isVisible && id.startsWith('radix-') && hasPopup && isBottomArea && !isAgent;
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

	// 2.5. Kiểm tra chi tiết từng cài đặt trên nút (bao gồm media type ảnh/video, duration, batch)
	isModelMatch, isRatioMatch, isBatchMatch, isDurationMatch, isMediaTypeMatch := checkConfigMatches(currentConfigText)
	isAllMatch := isModelMatch && isRatioMatch && isBatchMatch && isDurationMatch && isMediaTypeMatch
	logDebug("NÚT CẤU HÌNH ĐƯỢC TÌM THẤY: ID=%s, Text=[%s]", idStr, strings.ReplaceAll(currentConfigText, "\n", " "))
	logDebug("ĐỐI CHIẾU CẤU HÌNH: Model=%v, AspectRatio=%v, BatchSize=%v, Duration=%v, MediaType=%v -> Tất cả trùng khớp: %v", isModelMatch, isRatioMatch, isBatchMatch, isDurationMatch, isMediaTypeMatch, isAllMatch)

	if isAllMatch {
		logDebug("Cấu hình hiện tại ĐÃ TRÙNG KHỚP hoàn toàn với yêu cầu. BỎ QUA toàn bộ các bước mở cấu hình.")
		mile("Cấu hình đã đúng sẵn (model/tỷ lệ/số lượng/loại)")
		return nil
	}

	logDebug("Cấu hình chưa trùng khớp (Model: %v, Ratio: %v, Batch: %v, MediaType: %v). Tiến hành mở popover để điều chỉnh duy nhất các phần chưa đúng...", isModelMatch, isRatioMatch, isBatchMatch, isMediaTypeMatch)
	mile("Đang mở cấu hình để chỉnh model/tỷ lệ/số lượng...")

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
			mile(fmt.Sprintf("Đã chỉnh tỷ lệ khung hình → %s", req.AspectRatio))
		} else {
			logDebug("Không cấu hình được tỷ lệ khung hình: %v", errRatio)
			mile("⚠ Không chỉnh được tỷ lệ khung hình")
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
			mile(fmt.Sprintf("Đã chỉnh số lượng sinh → %s", req.BatchSize))
		} else {
			logDebug("Không cấu hình được số lượng batch size: %v", errBatch)
			mile("⚠ Không chỉnh được số lượng sinh")
		}
	} else if isBatchMatch {
		logDebug("Số lượng Batch Size (%s) đã đúng sẵn, bỏ qua không chỉnh lại.", req.BatchSize)
	}

	// 5.5. Cấu hình Thời lượng Video (4s, 6s, 8s) cho Video mode
	if isVideoType && req.Duration != "" {
		targetDuration := strings.ToLower(strings.TrimSpace(req.Duration))
		if !strings.HasSuffix(targetDuration, "s") && targetDuration != "" {
			targetDuration += "s"
		}
		logDebug("Cấu hình thời lượng video: %s", targetDuration)
		clickDurationBtn, errDur := getElement(3*time.Second, `(dur) => {
			const cleanDigit = dur.toLowerCase().replace('s', '').trim();
			// QUAN TRỌNG: KHÔNG dùng id$="-trigger-N" (id trần) cho thời lượng — nút
			// batch size (x4) cũng có id kết thúc "-trigger-4", và querySelector trả về
			// nút ĐẦU TIÊN theo thứ tự DOM (batch đứng trước) → bấm nhầm x4 thay vì 4s.
			// Nút thời lượng có text DUY NHẤT dạng "4s"/"6s"/"8s" (batch là "x4"/"4x"),
			// nên khớp theo text chính xác là an toàn nhất.

			// 1. Ưu tiên khớp text chính xác "Ns" trên toàn trang (duy nhất, không lẫn batch)
			const wantText = cleanDigit + 's';
			const exactByText = Array.from(document.querySelectorAll('button[role="tab"], button')).find(b => {
				return b.textContent.trim().toLowerCase() === wantText;
			});
			if (exactByText) return exactByText;

			// 2. Thử id trigger CÓ HẬU TỐ 's' (button[id$="-trigger-6s"]) — không đụng batch
			const btnIdS = document.querySelector('button[id$="-trigger-' + cleanDigit + 's"]');
			if (btnIdS) return btnIdS;

			// 3. Dự phòng: tìm trong popover đang mở theo text
			const popover = document.querySelector('[data-state="open"][role="menu"], [data-state="open"]');
			if (popover) {
				const buttons = Array.from(popover.querySelectorAll('button'));
				return buttons.find(b => {
					const txt = b.textContent.toLowerCase().trim();
					return txt === dur || txt === wantText;
				}) || null;
			}
			return null;
		}`, targetDuration)

		if errDur == nil && clickDurationBtn != nil {
			errClick := HumanClick(page, clickDurationBtn)
			if errClick != nil {
				logDebug("Lỗi click duration bằng chuột: %v. Thử bằng JS...", errClick)
				_, _ = clickDurationBtn.Eval("function() { this.click(); }")
			}
			sleep(500 * time.Millisecond)
			mile(fmt.Sprintf("Đã chỉnh thời lượng video → %s", targetDuration))
		} else {
			logDebug("Không cấu hình được thời lượng video %s: %v", targetDuration, errDur)
		}
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
				mile(fmt.Sprintf("Đã chỉnh model → %s", modelToSelect))
			} else {
				logDebug("Không cấu hình được model item %s: %v", modelToSelect, errItem)
				mile("⚠ Không chỉnh được model")
			}
		} else {
			logDebug("Không tìm thấy nút dropdown chọn Model trong popover: %v", errModelDropdown)
			mile("⚠ Không tìm thấy dropdown model")
		}
	} else if isModelMatch {
		logDebug("Model (%s) đã đúng sẵn, bỏ qua không chỉnh lại.", req.Model)
	}

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

// promptTextContent đọc phần văn bản người dùng đã nhập trong ô prompt Slate,
// bỏ qua nội dung của thẻ ảnh đính kèm (alt/aria) để verify prompt đã vào thật hay chưa.
func promptTextContent(promptInput *rod.Element) string {
	res, err := promptInput.Eval(`() => {
		// Slate lưu text trong các node [data-slate-string]; nếu không có thì lấy textContent.
		const spans = this.querySelectorAll('[data-slate-string="true"]');
		if (spans.length > 0) {
			return Array.from(spans).map(s => s.textContent || '').join('');
		}
		return this.textContent || '';
	}`)
	if err != nil || res == nil {
		return ""
	}
	return strings.TrimSpace(res.Value.Str())
}

// promptContainsText kiểm tra prompt đã thực sự nằm trong ô nhập liệu chưa.
// So khớp linh hoạt: Slate có thể chèn thêm khoảng trắng/xuống dòng quanh thẻ ảnh.
func promptContainsText(promptInput *rod.Element, prompt string) bool {
	want := strings.TrimSpace(prompt)
	if want == "" {
		return true
	}
	got := promptTextContent(promptInput)
	if got == "" {
		return false
	}
	// Khớp trực tiếp, hoặc bỏ mọi khoảng trắng hai bên để tránh sai lệch do Slate chèn whitespace.
	if strings.Contains(got, want) {
		return true
	}
	normalize := func(s string) string { return strings.Join(strings.Fields(s), " ") }
	return strings.Contains(normalize(got), normalize(want))
}

// moveCaretToEnd đặt con trỏ về cuối nội dung ô prompt (sau thẻ ảnh nếu có).
func moveCaretToEnd(promptInput *rod.Element) {
	_, _ = promptInput.Eval(`() => {
		this.focus();
		try {
			const textNodes = [];
			const walk = document.createTreeWalker(this, NodeFilter.SHOW_TEXT, null, false);
			let n;
			while (n = walk.nextNode()) {
				textNodes.push(n);
			}
			const range = document.createRange();
			const sel = window.getSelection();
			if (textNodes.length > 0) {
				const lastText = textNodes[textNodes.length - 1];
				range.setStart(lastText, lastText.nodeValue.length);
				range.setEnd(lastText, lastText.nodeValue.length);
			} else {
				range.selectNodeContents(this);
				range.collapse(false);
			}
			sel.removeAllRanges();
			sel.addRange(range);
		} catch (e) {}
	}`)
}

// CopyTextToClipboardWindows nạp xâu văn bản trực tiếp vào Clipboard của hệ thống Windows qua PowerShell
func CopyTextToClipboardWindows(text string) error {
	escaped := strings.ReplaceAll(text, "'", "''")
	cmdStr := fmt.Sprintf(`Add-Type -Assembly System.Windows.Forms; [System.Windows.Forms.Clipboard]::SetText('%s')`, escaped)
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", cmdStr)
	utils.HideCmdWindow(cmd)
	return cmd.Run()
}

// FillFlowPrompt điền văn bản prompt vào ô nhập liệu Slate của Google Flow.
// logDebug có thể nil (mặc định flowLogf); truyền vào để log mang prefix luồng [W#].
func FillFlowPrompt(page *rod.Page, promptInput *rod.Element, prompt string, logDebug func(string, ...interface{})) error {
	if logDebug == nil {
		logDebug = flowLogf
	}
	if strings.TrimSpace(prompt) == "" {
		return nil
	}

	dispatchInputEvents := func() {
		_, _ = promptInput.Eval(`() => {
			this.dispatchEvent(new Event('input', { bubbles: true }));
			this.dispatchEvent(new Event('change', { bubbles: true }));
		}`)
	}

	// Phát hiện có thẻ ảnh đính kèm trong ô không. Selector rộng: thẻ tile của Slate
	// HOẶC bất kỳ <img> nào nằm trong ô prompt. Trước đây chỉ tìm data-tile-id nên
	// bỏ sót → cách 4 chạy nhầm SelectAllText+Input("") và XÓA MẤT ảnh đã dán.
	hasImage := func() bool {
		r, _ := promptInput.Eval(`() => {
			return this.querySelector('div[data-tile-id], div[role="button"][aria-roledescription="draggable"], img') !== null;
		}`)
		return r != nil && r.Value.Bool()
	}

	// focusCaretEnd: focus ô rồi đưa con trỏ về CUỐI nội dung (sau thẻ ảnh). Sau khi
	// dán ảnh, caret kẹt tại void node của ảnh nên Slate từ chối chèn text ngay đó —
	// đây là lý do mọi cách điền đều trượt khi có ảnh. Đặt caret về cuối trước khi điền.
	focusCaretEnd := func() {
		_ = HumanClick(page, promptInput)
		_ = promptInput.Focus()
		moveCaretToEnd(promptInput)
		time.Sleep(150 * time.Millisecond)
	}

	// Cách 1: CDP InsertText tại cuối nội dung (sau thẻ ảnh)
	focusCaretEnd()
	_ = page.InsertText(prompt)
	time.Sleep(300 * time.Millisecond)
	dispatchInputEvents()
	time.Sleep(200 * time.Millisecond)
	if promptContainsText(promptInput, prompt) {
		return nil
	}

	// Cách 2: document.execCommand('insertText') - Chèn text chuẩn HTML5/Chrome rich-text
	logDebug("Prompt chưa vào ô sau InsertText. Thử chèn bằng execCommand('insertText')...")
	focusCaretEnd()
	_, _ = promptInput.Eval(`(txt) => {
		this.focus();
		document.execCommand('insertText', false, txt);
	}`, prompt)
	time.Sleep(300 * time.Millisecond)
	dispatchInputEvents()
	time.Sleep(200 * time.Millisecond)
	if promptContainsText(promptInput, prompt) {
		return nil
	}

	// Cách 3: Dán prompt qua OS Clipboard + CDP Ctrl+V (Cách dán 100% Slate nhận diện)
	logDebug("Prompt chưa vào ô sau execCommand. Thử dán prompt bằng OS Clipboard + Ctrl+V...")
	if err := CopyTextToClipboardWindows(prompt); err == nil {
		focusCaretEnd()
		_ = page.KeyActions().Press(input.ControlLeft).Press(input.KeyV).Do()
		time.Sleep(400 * time.Millisecond)
		dispatchInputEvents()
		time.Sleep(200 * time.Millisecond)
		if promptContainsText(promptInput, prompt) {
			return nil
		}
	}

	// Cách 4: gõ trực tiếp bằng Input(). CHỈ khi KHÔNG có ảnh — vì SelectAllText+Input("")
	// sẽ xóa sạch nội dung (gồm cả thẻ ảnh). Có ảnh thì tuyệt đối không chạy nhánh này.
	if !hasImage() {
		logDebug("Không có ảnh đính kèm, thử gõ trực tiếp bằng Input()...")
		_ = promptInput.Focus()
		_ = promptInput.SelectAllText()
		_ = promptInput.Input("")
		if err := promptInput.Input(prompt); err == nil {
			time.Sleep(300 * time.Millisecond)
			dispatchInputEvents()
			if promptContainsText(promptInput, prompt) {
				return nil
			}
		}
	} else {
		logDebug("Có ảnh đính kèm — bỏ qua cách gõ trực tiếp (tránh xóa mất ảnh).")
	}

	return fmt.Errorf("không điền được prompt vào ô nhập liệu sau 4 cách thử (Slate không nhận text)")
}

func getSettingsPath() string {
	return getSettingsFilePath()
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
func CopyImageToClipboardWindows(imagePath string) error {
	absPath, err := filepath.Abs(imagePath)
	if err != nil {
		absPath = imagePath
	}

	cmdStr := fmt.Sprintf(`Add-Type -Assembly System.Windows.Forms; Add-Type -Assembly System.Drawing; $img = [System.Drawing.Image]::FromFile('%s'); [System.Windows.Forms.Clipboard]::SetImage($img); $img.Dispose()`, absPath)
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", cmdStr)
	utils.HideCmdWindow(cmd)
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

// imageMimeFromExt suy ra MIME type từ đuôi file ảnh (mặc định image/png).
func imageMimeFromExt(imagePath string) string {
	switch strings.ToLower(filepath.Ext(imagePath)) {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".bmp":
		return "image/bmp"
	default:
		return "image/png"
	}
}

// PasteImageViaJS dán ảnh vào ô prompt Slate HOÀN TOÀN bằng JS, KHÔNG dùng clipboard
// OS và KHÔNG cần cửa sổ foreground — nên chạy được cả khi ẩn trình duyệt (headless
// hoặc cửa sổ nền). Cơ chế: đọc file ở Go → base64 → JS dựng lại File + DataTransfer
// rồi bắn một 'paste' event tổng hợp thẳng vào editor (giống hệt khi người dùng Ctrl+V).
// Trả về true nếu editor có vẻ đã nhận (không đảm bảo 100% vì tùy Google xử lý event).
func PasteImageViaJS(ctx context.Context, page *rod.Page, promptInput *rod.Element, imagePath string, logDebug func(string, ...interface{})) bool {
	data, err := os.ReadFile(imagePath)
	if err != nil {
		if logDebug != nil {
			logDebug("PasteImageViaJS: không đọc được file ảnh %s: %v", imagePath, err)
		}
		return false
	}
	mime := imageMimeFromExt(imagePath)
	dataURL := "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data)
	fileName := filepath.Base(imagePath)

	_ = promptInput.Focus()

	res, errEval := promptInput.Eval(`async (dataURL, mime, fileName) => {
		try {
			// data URL → Blob → File (mô phỏng file người dùng dán từ clipboard)
			const resp = await fetch(dataURL);
			const blob = await resp.blob();
			const file = new File([blob], fileName, { type: mime });

			const dt = new DataTransfer();
			dt.items.add(file);

			// Bắn 'paste' event tổng hợp mang theo file vào editor. React/Slate của
			// Google Flow lắng nghe onPaste và tự đọc clipboardData.files để đính ảnh.
			const target = this.querySelector('[data-slate-editor="true"]') || this;
			target.focus();
			const ev = new ClipboardEvent('paste', {
				bubbles: true,
				cancelable: true,
				clipboardData: dt,
			});
			// Một số bản Chrome không cho set clipboardData qua constructor → gán lại thủ công.
			try {
				Object.defineProperty(ev, 'clipboardData', { value: dt });
			} catch (e) {}
			target.dispatchEvent(ev);
			return true;
		} catch (e) {
			return 'ERR:' + (e && e.message ? e.message : String(e));
		}
	}`, dataURL, mime, fileName)

	if errEval != nil {
		if logDebug != nil {
			logDebug("PasteImageViaJS: lỗi dispatch paste event: %v", errEval)
		}
		return false
	}
	if res != nil && res.Value.Str() != "" && strings.HasPrefix(res.Value.Str(), "ERR:") {
		if logDebug != nil {
			logDebug("PasteImageViaJS: JS báo lỗi: %s", res.Value.Str())
		}
		return false
	}
	if logDebug != nil {
		logDebug("PasteImageViaJS: đã bắn paste event mang ảnh %s vào editor.", fileName)
	}
	time.Sleep(500 * time.Millisecond)
	return true
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

// jsMediaImgFilter là biểu thức JS dùng chung để lọc các <img> là ảnh media do
// Flow sinh ra (src chứa getMediaUrl hoặc /fx/api/), tránh lặp lại ở nhiều chỗ.
const jsMediaImgFilter = `Array.from(document.querySelectorAll('img')).filter(img => {
	const src = img.getAttribute('src') || '';
	return src.includes('getMediaUrl') || src.includes('/fx/api/');
})`

// countProjectImages đếm tổng số ảnh media đang hiển thị trong dự án Flow.
func countProjectImages(page *rod.Page) int {
	obj, err := page.Eval(`() => { return ` + jsMediaImgFilter + `.length; }`)
	if err != nil || obj == nil {
		return 0
	}
	return obj.Value.Int()
}

// countVideoButtonsAndTiles đếm số nút Tải xuống và số thẻ card video/tile đang
// có trên trang. Dùng để so mốc "trước/sau khi tạo" nhằm phát hiện video mới sinh.
func countVideoButtonsAndTiles(page *rod.Page) (dlCount, tileCount int) {
	if dlObj, err := page.Eval(`() => {
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
	}`); err == nil && dlObj != nil {
		dlCount = dlObj.Value.Int()
	}

	if tileObj, err := page.Eval(`() => {
		const tiles = Array.from(document.querySelectorAll('[data-tile-id], div[role="button"][aria-roledescription="draggable"], div[class*="tile"], video')).filter(el => {
			const rect = el.getBoundingClientRect();
			return rect.width > 80 && rect.height > 80;
		});
		return tiles.length;
	}`); err == nil && tileObj != nil {
		tileCount = tileObj.Value.Int()
	}
	return dlCount, tileCount
}
