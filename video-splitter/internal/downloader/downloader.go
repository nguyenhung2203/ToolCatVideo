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

	// Nếu là video đơn (linkType=="video"), loại bỏ tham số playlist ra khỏi URL
	// để tránh yt-dlp cào cả playlist khi link có &list=... (YouTube).
	if !isSearch && searchSource == "video" {
		if u, err := url.Parse(rawURL); err == nil {
			q := u.Query()
			removed := false
			for _, p := range []string{"list", "index", "start_radio"} {
				if q.Has(p) {
					q.Del(p)
					removed = true
				}
			}
			if removed {
				u.RawQuery = q.Encode()
				rawURL = u.String()
			}
		}
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
			ytResult, errYt := runYtDlpProbe(ctx, ytURL, cookieBrowser, limit, sortOrder, "")

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

	return runYtDlpProbe(ctx, rawURL, cookieBrowser, maxCount, sortOrder, searchSource)
}

func runYtDlpProbe(ctx context.Context, rawURL string, cookieBrowser string, maxCount int, sortOrder string, linkType string) (*URLProbeResult, error) {
	platform := DetectPlatform(rawURL)
	if strings.HasPrefix(rawURL, "ytsearch") {
		platform = "youtube"
	}

	args := []string{
		"--flat-playlist",
		"--dump-json",
		"--no-warnings",
		"--ignore-errors",
		"--extractor-args", "youtube:player_client=android",
		"--socket-timeout", "15",
		"--retries", "2",
	}
	// Video đơn: không mở rộng sang playlist/channel
	if linkType == "video" {
		args = append(args, "--no-playlist")
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

	// Thư mục tạm RIÊNG cho mỗi lượt tải. yt-dlp ghi .part/.ytdl/track rời vào đây
	// rồi mới move file hoàn chỉnh sang outputDir. Nhờ vậy hủy giữa dòng chỉ cần xóa
	// cả thư mục là sạch rác, không để lại .part lẫn trong thư mục video người dùng.
	tempDir, tmpErr := os.MkdirTemp(outputDir, ".tt_dl_")
	if tmpErr != nil {
		// Không tạo được thư mục tạm (ổ chỉ đọc?) → chấp nhận tải trực tiếp như cũ.
		tempDir = ""
	}
	cleanTemp := func() {
		if tempDir != "" {
			_ = os.RemoveAll(tempDir)
		}
	}
	defer cleanTemp()

	// File nhận ĐƯỜNG DẪN THẬT của video sau khi yt-dlp move/merge xong. Đây là nguồn
	// sự thật duy nhất: yt-dlp tự ghi ra, không phải ta đoán từ log. Trước đây parse
	// regex trên stdout/stderr rồi fallback findNewestFile — khi tải song song 2 video
	// thì fallback trả về file của video KIA, gây gán sai/trùng đường dẫn.
	pathListFile := ""
	if tempDir != "" {
		pathListFile = filepath.Join(tempDir, "final_path.txt")
	} else if f, ferr := os.CreateTemp("", "tt_dlpath_*.txt"); ferr == nil {
		pathListFile = f.Name()
		_ = f.Close()
		defer os.Remove(pathListFile)
	}

	args := []string{
		"--merge-output-format", "mp4",
		"--no-playlist",           // luôn tải 1 video (không cả playlist)
		"--newline",               // mỗi update progress trên 1 dòng (dễ parse)
		"--no-warnings",
		"--extractor-args", "youtube:player_client=android,web",
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

	// -o phải là đường dẫn TƯƠNG ĐỐI để --paths có hiệu lực (yt-dlp bỏ qua --paths
	// khi -o là đường dẫn tuyệt đối).
	if tempDir != "" {
		args = append(args,
			"-o", "%(title)s.%(ext)s",
			"--paths", "home:"+outputDir,
			"--paths", "temp:"+tempDir,
		)
	} else {
		args = append(args, "-o", filepath.Join(outputDir, "%(title)s.%(ext)s"))
	}

	// after_move:filepath = đường dẫn CUỐI CÙNG sau khi merge + move xong. Ghi ra file
	// thay vì stdout để không lẫn với dòng progress và không cần parse regex.
	if pathListFile != "" {
		args = append(args, "--print-to-file", "after_move:filepath", pathListFile)
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
		// YouTube, FB, etc: chọn chất lượng tốt nhất với fallback tương thích
		args = append(args,
			"--extractor-args", "youtube:player_client=android",
			"-f", "bestvideo[ext=mp4]+bestaudio[ext=m4a]/bestvideo+bestaudio/best[ext=mp4]/best",
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

	// Mốc thời gian trước khi tải: dùng để giới hạn findNewestFileAfter chỉ xét file
	// sinh ra TRONG lượt này. Trừ 2s cho lệch đồng hồ/độ phân giải mtime của filesystem.
	startedAt := time.Now().Add(-2 * time.Second)

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("không chạy được yt-dlp: %v", err)
	}

	// filePath chỉ là ĐƯỜNG DẪN DỰ PHÒNG parse từ log, dùng khi --print-to-file không
	// khả dụng. Cả 2 goroutine đọc log đều ghi vào biến này nên PHẢI có mutex, và phải
	// chờ chúng kết thúc (wgLog) trước khi đọc — trước đây thiếu cả hai nên vừa là data
	// race vừa có thể đọc giá trị chưa kịp ghi.
	var (
		logMu        sync.Mutex
		fallbackPath string
		wgLog        sync.WaitGroup
	)
	setFallback := func(p string) {
		logMu.Lock()
		fallbackPath = strings.TrimSpace(p)
		logMu.Unlock()
	}

	// scanLog đọc 1 stream, bắn progress và ghi nhận đường dẫn dự phòng.
	scanLog := func(r io.Reader) {
		defer wgLog.Done()
		sc := bufio.NewScanner(r)
		sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
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
				setFallback(m[1])
			}
			if m := mergeRegex.FindStringSubmatch(line); len(m) >= 2 {
				setFallback(m[1])
			}
			if m := alreadyRegex.FindStringSubmatch(line); len(m) >= 2 {
				setFallback(m[1])
			}
		}
	}

	wgLog.Add(2)
	go scanLog(stderr)
	go scanLog(stdout)

	waitErr := cmd.Wait()
	// Chờ 2 goroutine đọc hết stream ĐÃ đóng trước khi đọc fallbackPath.
	wgLog.Wait()

	if waitErr != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("yt-dlp tải thất bại: %v", waitErr)
	}

	// Ưu tiên tuyệt đối đường dẫn yt-dlp tự ghi ra (after_move:filepath).
	filePath := readFinalPath(pathListFile)

	if filePath == "" {
		logMu.Lock()
		filePath = fallbackPath
		logMu.Unlock()
	}

	// Đường dẫn parse từ log có thể sai do encoding OEM/ANSI trên Windows. Chỉ khi
	// KHÔNG xác định được file nào tồn tại mới dùng findNewestFile — và giới hạn ở file
	// vừa tạo trong lượt này để không bắt trúng video của lượt tải song song khác.
	if filePath != "" {
		if _, statErr := os.Stat(filePath); statErr != nil {
			filePath = ""
		}
	}
	if filePath == "" {
		filePath = findNewestFileAfter(outputDir, startedAt)
	}

	if filePath == "" {
		return nil, fmt.Errorf("không xác định được file đã tải — kiểm tra thư mục %s", outputDir)
	}

	// Emit 100% CHỈ khi đã chắc chắn có file thật, tránh báo hoàn tất rồi lại lỗi.
	if progressCb != nil {
		progressCb(DownloadProgress{
			Percent: 100,
			Speed:   "",
			ETA:     "00:00",
			VideoID: videoID,
			Title:   videoTitle,
		})
	}

	return &DownloadResult{
		FilePath: filePath,
		Title:    videoTitle,
	}, nil
}

// readFinalPath đọc đường dẫn video cuối cùng do yt-dlp tự ghi ra qua
// --print-to-file after_move:filepath. Đây là nguồn sự thật chính xác nhất: không
// phụ thuộc encoding console, không cần parse regex, không bị nhiễu khi tải song song.
// File có thể chứa nhiều dòng (VD tải kèm phụ đề) → lấy dòng cuối tồn tại thật.
func readFinalPath(pathListFile string) string {
	if pathListFile == "" {
		return ""
	}
	data, err := os.ReadFile(pathListFile)
	if err != nil {
		return ""
	}
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		p := strings.TrimSpace(lines[i])
		if p == "" {
			continue
		}
		if info, statErr := os.Stat(p); statErr == nil && !info.IsDir() {
			return p
		}
	}
	return ""
}

// findNewestFileAfter tìm file mới nhất trong dir được tạo SAU mốc after. Chỉ dùng làm
// phương án cuối khi cả --print-to-file lẫn log đều không cho đường dẫn dùng được.
//
// Mốc thời gian là điểm khác biệt quan trọng so với bản cũ (findNewestFile quét cả
// thư mục): khi tải song song nhiều video vào cùng thư mục, quét không lọc thời gian
// sẽ trả về video của lượt tải KHÁC, khiến 2 entry cùng trỏ 1 file. Bỏ qua file tạm
// (.part/.ytdl) và thư mục tạm để không trả về file chưa hoàn chỉnh.
func findNewestFileAfter(dir string, after time.Time) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	var newest string
	var newestTime time.Time
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := strings.ToLower(e.Name())
		if strings.HasSuffix(name, ".part") || strings.HasSuffix(name, ".ytdl") ||
			strings.HasSuffix(name, ".temp") || strings.HasPrefix(e.Name(), ".tt_dl_") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		mt := info.ModTime()
		if mt.Before(after) {
			continue
		}
		if mt.After(newestTime) {
			newestTime = mt
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
