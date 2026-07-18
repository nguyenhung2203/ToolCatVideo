package downloader

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"video-splitter/internal/utils"
)

// ─── TYPES ─────────────────────────────────────────────────────────────────────

// VideoEntry chứa metadata 1 video tìm được khi probe URL.
type VideoEntry struct {
	ID         string  `json:"id"`
	URL        string  `json:"url"`
	Title      string  `json:"title"`
	Duration   float64 `json:"duration"`   // giây
	ViewCount  int64   `json:"viewCount"`
	LikeCount  int64   `json:"likeCount"`
	UploadDate string  `json:"uploadDate"` // "20250715"
	Thumbnail  string  `json:"thumbnail"`
	Platform   string  `json:"platform"`   // "youtube" | "tiktok" | "facebook" | "generic"
}

// URLProbeResult chứa kết quả dò link.
type URLProbeResult struct {
	Type     string       `json:"type"`     // "video" | "playlist"
	Platform string       `json:"platform"` // "youtube" | "tiktok" | "facebook" | "generic"
	Title    string       `json:"title"`    // tên kênh/playlist (nếu là playlist)
	Entries  []VideoEntry `json:"entries"`
}

// DownloadResult chứa kết quả tải 1 video.
type DownloadResult struct {
	FilePath string `json:"filePath"`
	Title    string `json:"title"`
	Duration float64 `json:"duration"`
}

// DownloadProgress chứa tiến trình tải video, gửi qua callback.
type DownloadProgress struct {
	Percent float64 `json:"percent"`
	Speed   string  `json:"speed"`
	ETA     string  `json:"eta"`
	VideoID string  `json:"videoId"`
	Title   string  `json:"title"`
}

// ─── PLATFORM DETECTION ────────────────────────────────────────────────────────

// DetectPlatform phát hiện nền tảng từ URL.
func DetectPlatform(rawURL string) string {
	u := strings.ToLower(rawURL)
	switch {
	case strings.Contains(u, "youtube.com") || strings.Contains(u, "youtu.be"):
		return "youtube"
	case strings.Contains(u, "tiktok.com"):
		return "tiktok"
	case strings.Contains(u, "facebook.com") || strings.Contains(u, "fb.watch") || strings.Contains(u, "fb.com"):
		return "facebook"
	default:
		return "generic"
	}
}

// ─── PROBE URL ─────────────────────────────────────────────────────────────────

// ytDlpFlatEntry là struct parse JSON trả về từ yt-dlp --flat-playlist --dump-json.
type ytDlpFlatEntry struct {
	ID          string  `json:"id"`
	URL         string  `json:"url"`
	WebpageURL  string  `json:"webpage_url"`
	Title       string  `json:"title"`
	Duration    float64 `json:"duration"`
	ViewCount   int64   `json:"view_count"`
	LikeCount   int64   `json:"like_count"`
	UploadDate  string  `json:"upload_date"`
	Thumbnail   string  `json:"thumbnail"`
	Extractor   string  `json:"extractor"`
	Type        string  `json:"_type"` // "url" cho playlist entry, trống cho single
	PlaylistTitle string `json:"playlist_title"`
}

// ProbeURL dò link URL bằng yt-dlp, trả về danh sách video (1 nếu video đơn).
// cookieBrowser: "chrome" | "edge" | "firefox" | "" (không dùng cookie)
// maxCount: giới hạn số video lấy (0 = không giới hạn)
// sortOrder: "newest" (mặc định) | "oldest" (đảo ngược playlist)
func ProbeURL(ctx context.Context, rawURL string, cookieBrowser string, maxCount int, sortOrder string, searchSource string) (*URLProbeResult, error) {
	isSearch := false
	query := rawURL
	if !strings.HasPrefix(strings.ToLower(rawURL), "http://") && !strings.HasPrefix(strings.ToLower(rawURL), "https://") {
		isSearch = true
	}

	if isSearch {
		if searchSource == "tiktok" {
			entries, err := searchTikTok(ctx, query, maxCount)
			if err != nil {
				return nil, err
			}
			return &URLProbeResult{
				Type:     "playlist",
				Platform: "tiktok",
				Title:    "TikTok Search: " + query,
				Entries:  entries,
			}, nil
		}

		if searchSource == "facebook" {
			entries, err := searchFacebook(ctx, query, maxCount)
			if err != nil {
				return nil, err
			}
			return &URLProbeResult{
				Type:     "playlist",
				Platform: "facebook",
				Title:    "Facebook Search: " + query,
				Entries:  entries,
			}, nil
		}

		if searchSource == "all" {
			limit := 10
			if maxCount > 0 {
				limit = maxCount / 3
				if limit < 5 {
					limit = 5
				}
			}
			// Search YouTube
			ytURL := fmt.Sprintf("ytsearch%d:%s", limit, query)
			ytResult, errYt := runYtDlpProbe(ctx, ytURL, cookieBrowser, limit, sortOrder)

			// Search TikTok
			ttEntries, _ := searchTikTok(ctx, query, limit)

			// Search Facebook
			fbEntries, _ := searchFacebook(ctx, query, limit)

			var merged []VideoEntry
			if errYt == nil && ytResult != nil {
				merged = append(merged, ytResult.Entries...)
			}
			merged = append(merged, ttEntries...)
			merged = append(merged, fbEntries...)

			if len(merged) == 0 {
				return nil, fmt.Errorf("không tìm thấy kết quả nào từ YouTube, TikTok hay Facebook cho từ khóa này")
			}

			return &URLProbeResult{
				Type:     "playlist",
				Platform: "mixed",
				Title:    "Tìm kiếm hỗn hợp: " + query,
				Entries:  merged,
			}, nil
		}

		// YouTube search (mặc định)
		limit := 30
		if maxCount > 0 {
			limit = maxCount
		}
		rawURL = fmt.Sprintf("ytsearch%d:%s", limit, query)
	}

	return runYtDlpProbe(ctx, rawURL, cookieBrowser, maxCount, sortOrder)
}

func runYtDlpProbe(ctx context.Context, rawURL string, cookieBrowser string, maxCount int, sortOrder string) (*URLProbeResult, error) {
	platform := DetectPlatform(rawURL)
	if strings.HasPrefix(rawURL, "ytsearch") {
		platform = "youtube"
	}

	args := []string{
		"--flat-playlist",
		"--dump-json",
		"--no-warnings",
		"--ignore-errors",
	}
	if maxCount > 0 {
		args = append(args, "--playlist-end", strconv.Itoa(maxCount))
	}
	if sortOrder == "oldest" {
		args = append(args, "--playlist-reverse")
	}
	if cookieBrowser != "" {
		args = append(args, "--cookies-from-browser", cookieBrowser)
	}
	args = append(args, rawURL)

	cmd := exec.CommandContext(ctx, utils.GetYtDlpPath(), args...)
	utils.HideCmdWindow(cmd)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("không tạo được stdout pipe: %v", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("không chạy được yt-dlp: %v (đường dẫn: %s)", err, utils.GetYtDlpPath())
	}

	var entries []VideoEntry
	var playlistTitle string
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 1024*1024), 4*1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}

		var raw ytDlpFlatEntry
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			continue
		}

		if raw.PlaylistTitle != "" && playlistTitle == "" {
			playlistTitle = raw.PlaylistTitle
		}

		entryURL := raw.WebpageURL
		if entryURL == "" {
			entryURL = raw.URL
		}
		if entryURL == "" {
			entryURL = rawURL
		}

		thumbnailURL := raw.Thumbnail
		if thumbnailURL == "" && platform == "youtube" && raw.ID != "" {
			thumbnailURL = fmt.Sprintf("https://i.ytimg.com/vi/%s/hqdefault.jpg", raw.ID)
		}

		entry := VideoEntry{
			ID:         raw.ID,
			URL:        entryURL,
			Title:      raw.Title,
			Duration:   raw.Duration,
			ViewCount:  raw.ViewCount,
			LikeCount:  raw.LikeCount,
			UploadDate: raw.UploadDate,
			Thumbnail:  thumbnailURL,
			Platform:   platform,
		}
		entries = append(entries, entry)
	}

	_ = cmd.Wait()

	if len(entries) == 0 {
		return nil, fmt.Errorf("không tìm thấy video nào ở URL này.")
	}

	resultType := "video"
	if len(entries) > 1 {
		resultType = "playlist"
	}

	return &URLProbeResult{
		Type:     resultType,
		Platform: platform,
		Title:    playlistTitle,
		Entries:  entries,
	}, nil
}

func searchTikTok(ctx context.Context, query string, maxCount int) ([]VideoEntry, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	searchURL := "https://html.duckduckgo.com/html/?q=site:tiktok.com+" + url.QueryEscape(query)
	req, err := http.NewRequestWithContext(ctx, "GET", searchURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("DuckDuckGo returned status %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	htmlContent := string(bodyBytes)

	// Regexp để lấy các link video của TikTok
	re := regexp.MustCompile(`https?://(?:www\.)?tiktok\.com/@[a-zA-Z0-9_.-]+/video/\d+`)
	matches := re.FindAllString(htmlContent, -1)

	seen := make(map[string]bool)
	var urls []string
	for _, u := range matches {
		if !seen[u] {
			seen[u] = true
			urls = append(urls, u)
		}
	}

	if len(urls) == 0 {
		return nil, fmt.Errorf("không tìm thấy video TikTok nào cho từ khóa này")
	}

	if maxCount > 0 && len(urls) > maxCount {
		urls = urls[:maxCount]
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	var entries []VideoEntry

	sem := make(chan struct{}, 5)
	for _, u := range urls {
		wg.Add(1)
		go func(videoURL string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			args := []string{
				"--dump-json",
				"--no-warnings",
				"--ignore-errors",
				videoURL,
			}
			cmd := exec.CommandContext(ctx, utils.GetYtDlpPath(), args...)
			utils.HideCmdWindow(cmd)

			out, err := cmd.Output()
			if err != nil {
				mu.Lock()
				entries = append(entries, VideoEntry{
					ID:        extractVideoID(videoURL),
					URL:       videoURL,
					Title:     "TikTok Video (" + query + ")",
					Platform:  "tiktok",
					Thumbnail: "https://www.tiktok.com/favicon.ico",
				})
				mu.Unlock()
				return
			}

			var raw ytDlpFlatEntry
			if err := json.Unmarshal(out, &raw); err != nil {
				return
			}

			entry := VideoEntry{
				ID:         raw.ID,
				URL:        videoURL,
				Title:      raw.Title,
				Duration:   raw.Duration,
				ViewCount:  raw.ViewCount,
				LikeCount:  raw.LikeCount,
				UploadDate: raw.UploadDate,
				Thumbnail:  raw.Thumbnail,
				Platform:   "tiktok",
			}
			mu.Lock()
			entries = append(entries, entry)
			mu.Unlock()
		}(u)
	}
	wg.Wait()

	return entries, nil
}

func extractVideoID(videoURL string) string {
	parts := strings.Split(videoURL, "/video/")
	if len(parts) > 1 {
		return parts[1]
	}
	return "tiktok_video"
}

// ─── DOWNLOAD VIDEO ────────────────────────────────────────────────────────────

// progressRegex parse dòng progress từ yt-dlp: [download]  45.2% of 23.50MiB at 2.30MiB/s ETA 00:12
var progressRegex = regexp.MustCompile(`\[download\]\s+([\d.]+)%\s+of\s+\S+\s+at\s+(\S+)\s+ETA\s+(\S+)`)

// mergeRegex: [Merger] Merging formats into "output.mp4"
var mergeRegex = regexp.MustCompile(`\[Merger\] Merging formats into "(.+)"`)

// destRegex: [download] Destination: /path/to/file.mp4
var destRegex = regexp.MustCompile(`\[download\] Destination:\s+(.+)`)

// alreadyRegex: [download] /path/to/file.mp4 has already been downloaded
var alreadyRegex = regexp.MustCompile(`\[download\]\s+(.+) has already been downloaded`)

// DownloadVideo tải 1 video từ URL, gọi progressCb để cập nhật tiến trình.
// Trả về DownloadResult chứa đường dẫn file đã tải.
func DownloadVideo(
	ctx context.Context,
	rawURL string,
	outputDir string,
	cookieBrowser string,
	videoID string,
	videoTitle string,
	progressCb func(DownloadProgress),
) (*DownloadResult, error) {
	_ = os.MkdirAll(outputDir, 0755)

	// Template output: giữ tên video gốc, luôn xuất mp4
	outputTemplate := filepath.Join(outputDir, "%(title)s.%(ext)s")

	args := []string{
		"-o", outputTemplate,
		"--merge-output-format", "mp4",
		"--no-playlist",           // luôn tải 1 video (không cả playlist)
		"--newline",               // mỗi update progress trên 1 dòng (dễ parse)
		"--no-warnings",
		"--progress",
		"--console-title",
		// === TẢI VIDEO SẠCH: không logo, không watermark, không metadata thừa ===
		"--no-embed-metadata",     // không nhúng metadata vào file
		"--no-embed-thumbnail",    // không nhúng thumbnail
		"--no-embed-chapters",     // không nhúng chapters
		"--no-embed-info-json",    // không nhúng info json
		"--no-write-thumbnail",    // không tải thumbnail riêng
		"--no-write-info-json",    // không ghi file info json
		"--clean-info-json",       // dọn dẹp info json
	}

	// Chọn format tốt nhất KHÔNG có watermark
	// TikTok: format "download_addr-0" hoặc tương tự có watermark, ta bỏ qua
	// Ưu tiên: video tốt nhất + audio tốt nhất, loại format có "watermark" trong tên
	platform := DetectPlatform(rawURL)
	switch platform {
	case "tiktok":
		// TikTok: chọn format không có watermark trong ID/note
		// Format "0" thường là bản không watermark trên TikTok
		args = append(args,
			"-f", "best[format_note!*=watermark]/best",
		)
	default:
		// YouTube, FB, etc: chọn chất lượng tốt nhất
		args = append(args,
			"-f", "bestvideo[ext=mp4]+bestaudio[ext=m4a]/bestvideo+bestaudio/best",
		)
	}

	if cookieBrowser != "" {
		args = append(args, "--cookies-from-browser", cookieBrowser)
	}
	args = append(args, rawURL)

	cmd := exec.CommandContext(ctx, utils.GetYtDlpPath(), args...)
	utils.HideCmdWindow(cmd)

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("không tạo được stderr pipe: %v", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("không tạo được stdout pipe: %v", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("không chạy được yt-dlp: %v", err)
	}

	var filePath string

	// Đọc stderr (yt-dlp ghi progress + info vào stderr)
	go func() {
		sc := bufio.NewScanner(stderr)
		for sc.Scan() {
			line := sc.Text()

			// Parse progress
			if m := progressRegex.FindStringSubmatch(line); len(m) >= 4 {
				pct, _ := strconv.ParseFloat(m[1], 64)
				if progressCb != nil {
					progressCb(DownloadProgress{
						Percent: pct,
						Speed:   m[2],
						ETA:     m[3],
						VideoID: videoID,
						Title:   videoTitle,
					})
				}
			}

			// Parse destination file
			if m := destRegex.FindStringSubmatch(line); len(m) >= 2 {
				filePath = strings.TrimSpace(m[1])
			}
			if m := mergeRegex.FindStringSubmatch(line); len(m) >= 2 {
				filePath = strings.TrimSpace(m[1])
			}
			if m := alreadyRegex.FindStringSubmatch(line); len(m) >= 2 {
				filePath = strings.TrimSpace(m[1])
			}
		}
	}()

	// Đọc stdout (yt-dlp --newline cũng ghi progress vào stdout)
	go func() {
		sc := bufio.NewScanner(stdout)
		for sc.Scan() {
			line := sc.Text()
			if m := progressRegex.FindStringSubmatch(line); len(m) >= 4 {
				pct, _ := strconv.ParseFloat(m[1], 64)
				if progressCb != nil {
					progressCb(DownloadProgress{
						Percent: pct,
						Speed:   m[2],
						ETA:     m[3],
						VideoID: videoID,
						Title:   videoTitle,
					})
				}
			}
			if m := destRegex.FindStringSubmatch(line); len(m) >= 2 {
				filePath = strings.TrimSpace(m[1])
			}
			if m := mergeRegex.FindStringSubmatch(line); len(m) >= 2 {
				filePath = strings.TrimSpace(m[1])
			}
			if m := alreadyRegex.FindStringSubmatch(line); len(m) >= 2 {
				filePath = strings.TrimSpace(m[1])
			}
		}
	}()

	if err := cmd.Wait(); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("yt-dlp tải thất bại: %v", err)
	}

	// Emit 100%
	if progressCb != nil {
		progressCb(DownloadProgress{
			Percent: 100,
			Speed:   "",
			ETA:     "00:00",
			VideoID: videoID,
			Title:   videoTitle,
		})
	}

	if filePath == "" {
		filePath = findNewestFile(outputDir)
	} else {
		// Kiểm tra xem file có thực sự tồn tại với đường dẫn đã parse không.
		// Trên Windows, yt-dlp ghi log ra stdout/stderr bằng encoding hệ thống (OEM/ANSI)
		// nên khi Go đọc bằng UTF-8 sẽ bị mất dấu hoặc sai ký tự, dẫn đến file không tìm thấy.
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			if fallback := findNewestFile(outputDir); fallback != "" {
				filePath = fallback
			}
		}
	}

	if filePath == "" {
		return nil, fmt.Errorf("không xác định được file đã tải — kiểm tra thư mục %s", outputDir)
	}

	return &DownloadResult{
		FilePath: filePath,
		Title:    videoTitle,
	}, nil
}

// findNewestFile tìm file mới nhất trong thư mục (fallback khi không parse được path).
func findNewestFile(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	var newest string
	var newestTime int64
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Unix() > newestTime {
			newestTime = info.ModTime().Unix()
			newest = filepath.Join(dir, e.Name())
		}
	}
	return newest
}

func searchFacebook(ctx context.Context, query string, maxCount int) ([]VideoEntry, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	searchURL := "https://html.duckduckgo.com/html/?q=site:facebook.com/watch+" + url.QueryEscape(query)
	req, err := http.NewRequestWithContext(ctx, "GET", searchURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("DuckDuckGo returned status %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	htmlContent := string(bodyBytes)

	// Regexp để lấy các link video Facebook
	re := regexp.MustCompile(`https?://(?:www\.)?facebook\.com/watch/\?v=\d+`)
	matches := re.FindAllString(htmlContent, -1)

	reReel := regexp.MustCompile(`https?://(?:www\.)?facebook\.com/reel/\d+`)
	matches = append(matches, reReel.FindAllString(htmlContent, -1)...)

	seen := make(map[string]bool)
	var urls []string
	for _, u := range matches {
		if !seen[u] {
			seen[u] = true
			urls = append(urls, u)
		}
	}

	if len(urls) == 0 {
		return nil, fmt.Errorf("không tìm thấy video Facebook nào cho từ khóa này")
	}

	if maxCount > 0 && len(urls) > maxCount {
		urls = urls[:maxCount]
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	var entries []VideoEntry

	sem := make(chan struct{}, 5)
	for _, u := range urls {
		wg.Add(1)
		go func(videoURL string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			args := []string{
				"--dump-json",
				"--no-warnings",
				"--ignore-errors",
				videoURL,
			}
			cmd := exec.CommandContext(ctx, utils.GetYtDlpPath(), args...)
			utils.HideCmdWindow(cmd)

			out, err := cmd.Output()
			if err != nil {
				mu.Lock()
				entries = append(entries, VideoEntry{
					ID:        extractFBVideoID(videoURL),
					URL:       videoURL,
					Title:     "Facebook Video (" + query + ")",
					Platform:  "facebook",
					Thumbnail: "https://www.facebook.com/favicon.ico",
				})
				mu.Unlock()
				return
			}

			var raw ytDlpFlatEntry
			if err := json.Unmarshal(out, &raw); err != nil {
				return
			}

			entry := VideoEntry{
				ID:         raw.ID,
				URL:        videoURL,
				Title:      raw.Title,
				Duration:   raw.Duration,
				ViewCount:  raw.ViewCount,
				LikeCount:  raw.LikeCount,
				UploadDate: raw.UploadDate,
				Thumbnail:  raw.Thumbnail,
				Platform:   "facebook",
			}
			mu.Lock()
			entries = append(entries, entry)
			mu.Unlock()
		}(u)
	}
	wg.Wait()

	return entries, nil
}

func extractFBVideoID(videoURL string) string {
	parts := strings.Split(videoURL, "?v=")
	if len(parts) > 1 {
		return parts[1]
	}
	partsReel := strings.Split(videoURL, "/reel/")
	if len(partsReel) > 1 {
		return partsReel[1]
	}
	return "facebook_video"
}
