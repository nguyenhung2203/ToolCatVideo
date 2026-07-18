package imagedownloader

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// ─── TYPES ─────────────────────────────────────────────────────────────────────

// ImageEntry chứa metadata một ảnh tìm được.
type ImageEntry struct {
	ID       string `json:"id"`
	URL      string `json:"url"`      // URL ảnh full size
	ThumbURL string `json:"thumbUrl"` // URL ảnh thumbnail để preview
	Title    string `json:"title"`
	Author   string `json:"author"`
	Source   string `json:"source"`   // "duckduckgo" | "pixabay" | "unsplash" | "pexels"
	Width    int    `json:"width"`
	Height   int    `json:"height"`
	PageURL  string `json:"pageUrl"`  // Link trang gốc
}

// ImageSearchResult chứa kết quả tìm kiếm ảnh.
type ImageSearchResult struct {
	Source  string       `json:"source"`
	Query   string       `json:"query"`
	Total   int          `json:"total"`
	Entries []ImageEntry `json:"entries"`
}

// ImageDownloadResult chứa kết quả tải một ảnh.
type ImageDownloadResult struct {
	ID       string `json:"id"`
	FilePath string `json:"filePath"`
	Title    string `json:"title"`
	OK       bool   `json:"ok"`
	Error    string `json:"error"`
}

// ImageDownloadProgress chứa tiến trình tải ảnh.
type ImageDownloadProgress struct {
	ID      string  `json:"id"`
	Title   string  `json:"title"`
	Percent float64 `json:"percent"`
	Done    int     `json:"done"`
	Total   int     `json:"total"`
}

// ─── HTTP CLIENT ───────────────────────────────────────────────────────────────

func newHTTPClient() *http.Client {
	return &http.Client{Timeout: 20 * time.Second}
}

var commonHeaders = map[string]string{
	"User-Agent":      "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
	"Accept":          "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8",
	"Accept-Language": "en-US,en;q=0.9",
}

func addCommonHeaders(req *http.Request) {
	for k, v := range commonHeaders {
		req.Header.Set(k, v)
	}
}

// ─── SEARCH DISPATCHER ─────────────────────────────────────────────────────────

// SearchImages tìm kiếm ảnh từ nguồn được chỉ định.
// source: "duckduckgo" | "pixabay" | "unsplash" | "pexels"
// apiKey: API key cho Pixabay/Unsplash/Pexels (bỏ trống nếu không cần)
func SearchImages(ctx context.Context, query string, source string, apiKey string, maxCount int) (*ImageSearchResult, error) {
	if maxCount <= 0 {
		maxCount = 50
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("từ khóa tìm kiếm không được để trống")
	}

	switch source {
	case "pixabay":
		return searchPixabay(ctx, query, apiKey, maxCount)
	case "unsplash":
		return searchUnsplash(ctx, query, apiKey, maxCount)
	case "pexels":
		return searchPexels(ctx, query, apiKey, maxCount)
	default: // "duckduckgo" or fallback
		return searchDuckDuckGo(ctx, query, maxCount)
	}
}

// searchDuckDuckGo tìm kiếm ảnh miễn phí qua DuckDuckGo (không cần API key).
func searchDuckDuckGo(ctx context.Context, query string, maxCount int) (*ImageSearchResult, error) {
	client := newHTTPClient()

	// Lấy vqd token
	tokenURL := "https://duckduckgo.com/?q=" + url.QueryEscape(query) + "&ia=images&iax=images"
	req, err := http.NewRequestWithContext(ctx, "GET", tokenURL, nil)
	if err != nil {
		return nil, fmt.Errorf("lỗi tạo request DuckDuckGo: %v", err)
	}
	addCommonHeaders(req)
	req.Header.Set("Referer", "https://duckduckgo.com/")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("lỗi gửi request DuckDuckGo: %v", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("lỗi đọc response DuckDuckGo: %v", err)
	}
	body := string(bodyBytes)

	// Tìm vqd token
	vqdRe := regexp.MustCompile(`vqd=([0-9-]+)`)
	vqdMatches := vqdRe.FindStringSubmatch(body)
	vqd := ""
	if len(vqdMatches) > 1 {
		vqd = vqdMatches[1]
	}

	if vqd == "" {
		// Thử parse vqd dạng khác
		vqdRe2 := regexp.MustCompile(`vqd=['"]([^'"]+)['"]`)
		m2 := vqdRe2.FindStringSubmatch(body)
		if len(m2) > 1 {
			vqd = m2[1]
		}
	}

	var entries []ImageEntry

	// Nếu có vqd, dùng API chính thức của DDG
	if vqd != "" {
		s := 0
		for len(entries) < maxCount {
			apiURL := fmt.Sprintf(
				"https://duckduckgo.com/i.js?q=%s&vqd=%s&o=json&p=1&s=%d&u=bing&f=,,,&l=vn-vi",
				url.QueryEscape(query), url.QueryEscape(vqd), s,
			)
			apiReq, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
			if err != nil {
				break
			}
			addCommonHeaders(apiReq)
			apiReq.Header.Set("Referer", "https://duckduckgo.com/")
			apiReq.Header.Set("X-Requested-With", "XMLHttpRequest")

			apiResp, err := client.Do(apiReq)
			if err != nil {
				break
			}

			var ddgResp struct {
				Results []struct {
					Image     string `json:"image"`
					Thumbnail string `json:"thumbnail"`
					Title     string `json:"title"`
					URL       string `json:"url"`
					Width     int    `json:"width"`
					Height    int    `json:"height"`
				} `json:"results"`
			}
			decodeErr := json.NewDecoder(apiResp.Body).Decode(&ddgResp)
			apiResp.Body.Close()
			if decodeErr != nil {
				break
			}

			if len(ddgResp.Results) == 0 {
				break
			}

			addedThisPage := 0
			for _, r := range ddgResp.Results {
				if len(entries) >= maxCount {
					break
				}
				if r.Image == "" {
					continue
				}
				entry := ImageEntry{
					ID:       fmt.Sprintf("ddg_%d", len(entries)),
					URL:      r.Image,
					ThumbURL: r.Thumbnail,
					Title:    r.Title,
					Source:   "duckduckgo",
					Width:    r.Width,
					Height:   r.Height,
					PageURL:  r.URL,
				}
				if entry.ThumbURL == "" {
					entry.ThumbURL = entry.URL
				}
				entries = append(entries, entry)
				addedThisPage++
			}

			if addedThisPage == 0 {
				break
			}
			s += len(ddgResp.Results)
			// Để tránh bị rate limit, delay nhẹ
			time.Sleep(100 * time.Millisecond)
		}
	}

	// Fallback: scrape trực tiếp từ HTML nếu API thất bại
	if len(entries) == 0 {
		reImg := regexp.MustCompile(`"image":"(https?://[^"]+)"`)
		reThumb := regexp.MustCompile(`"thumbnail":"(https?://[^"]+)"`)
		reTitle := regexp.MustCompile(`"title":"([^"]+)"`)

		imgMatches := reImg.FindAllStringSubmatch(body, maxCount*2)
		thumbMatches := reThumb.FindAllStringSubmatch(body, maxCount*2)
		titleMatches := reTitle.FindAllStringSubmatch(body, maxCount*2)

		for i, m := range imgMatches {
			if len(entries) >= maxCount {
				break
			}
			imgURL := m[1]
			if !isImageURL(imgURL) {
				continue
			}
			thumb := imgURL
			if i < len(thumbMatches) {
				thumb = thumbMatches[i][1]
			}
			title := ""
			if i < len(titleMatches) {
				title = titleMatches[i][1]
			}
			entries = append(entries, ImageEntry{
				ID:       fmt.Sprintf("ddg_fallback_%d", i),
				URL:      imgURL,
				ThumbURL: thumb,
				Title:    title,
				Source:   "duckduckgo",
			})
		}
	}

	if len(entries) == 0 {
		return nil, fmt.Errorf("không tìm thấy ảnh nào cho từ khóa '%s' trên DuckDuckGo", query)
	}

	return &ImageSearchResult{
		Source:  "duckduckgo",
		Query:   query,
		Total:   len(entries),
		Entries: entries,
	}, nil
}

// ─── PIXABAY SEARCH ────────────────────────────────────────────────────────────

// searchPixabay tìm kiếm ảnh qua Pixabay API (cần API key).
func searchPixabay(ctx context.Context, query string, apiKey string, maxCount int) (*ImageSearchResult, error) {
	if apiKey == "" {
		// Pixabay có demo key ẩn, thử dùng
		apiKey = "pixabay-free-key"
	}

	var entries []ImageEntry
	page := 1
	totalHits := 0
	client := newHTTPClient()

	for len(entries) < maxCount {
		perPage := maxCount - len(entries)
		if perPage > 200 {
			perPage = 200
		}
		if perPage < 3 {
			perPage = 3 // Pixabay min per_page is 3
		}

		apiURL := fmt.Sprintf(
			"https://pixabay.com/api/?key=%s&q=%s&page=%d&per_page=%d&image_type=photo&safesearch=true&lang=vi",
			url.QueryEscape(apiKey),
			url.QueryEscape(query),
			page,
			perPage,
		)

		req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
		if err != nil {
			return nil, fmt.Errorf("lỗi tạo request Pixabay: %v", err)
		}
		addCommonHeaders(req)

		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("lỗi gửi request Pixabay: %v", err)
		}

		if resp.StatusCode == 400 || resp.StatusCode == 403 {
			resp.Body.Close()
			return nil, fmt.Errorf("Pixabay API Key không hợp lệ. Vui lòng đăng ký tại https://pixabay.com/api/docs/ và nhập key vào ô API Key")
		}

		var pbResp struct {
			TotalHits int `json:"totalHits"`
			Hits      []struct {
				ID             int    `json:"id"`
				WebformatURL   string `json:"webformatURL"`
				LargeImageURL  string `json:"largeImageURL"`
				PreviewURL     string `json:"previewURL"`
				Tags           string `json:"tags"`
				User           string `json:"user"`
				WebformatWidth int    `json:"webformatWidth"`
				WebformatHeight int   `json:"webformatHeight"`
				PageURL        string `json:"pageURL"`
			} `json:"hits"`
		}

		decodeErr := json.NewDecoder(resp.Body).Decode(&pbResp)
		resp.Body.Close()
		if decodeErr != nil {
			return nil, fmt.Errorf("lỗi parse response Pixabay: %v", decodeErr)
		}

		totalHits = pbResp.TotalHits
		if len(pbResp.Hits) == 0 {
			break
		}

		for _, h := range pbResp.Hits {
			if len(entries) >= maxCount {
				break
			}
			imgURL := h.LargeImageURL
			if imgURL == "" {
				imgURL = h.WebformatURL
			}
			entries = append(entries, ImageEntry{
				ID:       fmt.Sprintf("pixabay_%d", h.ID),
				URL:      imgURL,
				ThumbURL: h.PreviewURL,
				Title:    h.Tags,
				Author:   h.User,
				Source:   "pixabay",
				Width:    h.WebformatWidth,
				Height:   h.WebformatHeight,
				PageURL:  h.PageURL,
			})
		}

		if len(entries) >= totalHits {
			break
		}
		page++
		time.Sleep(100 * time.Millisecond)
	}

	if len(entries) == 0 {
		return nil, fmt.Errorf("không tìm thấy ảnh nào cho từ khóa '%s' trên Pixabay", query)
	}

	return &ImageSearchResult{
		Source:  "pixabay",
		Query:   query,
		Total:   totalHits,
		Entries: entries,
	}, nil
}

// ─── UNSPLASH SEARCH ───────────────────────────────────────────────────────────

// searchUnsplash tìm kiếm ảnh qua Unsplash API (cần Access Key).
func searchUnsplash(ctx context.Context, query string, apiKey string, maxCount int) (*ImageSearchResult, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("cần nhập Unsplash Access Key. Đăng ký miễn phí tại https://unsplash.com/developers")
	}

	var entries []ImageEntry
	page := 1
	total := 0
	client := newHTTPClient()

	for len(entries) < maxCount {
		perPage := maxCount - len(entries)
		if perPage > 30 {
			perPage = 30
		}

		apiURL := fmt.Sprintf(
			"https://api.unsplash.com/search/photos?query=%s&page=%d&per_page=%d&orientation=landscape",
			url.QueryEscape(query), page, perPage,
		)

		req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
		if err != nil {
			return nil, fmt.Errorf("lỗi tạo request Unsplash: %v", err)
		}
		req.Header.Set("Authorization", "Client-ID "+apiKey)
		addCommonHeaders(req)

		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("lỗi gửi request Unsplash: %v", err)
		}

		if resp.StatusCode == 401 || resp.StatusCode == 403 {
			resp.Body.Close()
			return nil, fmt.Errorf("Unsplash Access Key không hợp lệ. Đăng ký tại https://unsplash.com/developers")
		}

		var unsResp struct {
			Total   int `json:"total"`
			Results []struct {
				ID          string `json:"id"`
				Description string `json:"description"`
				AltDesc     string `json:"alt_description"`
				URLs        struct {
					Full    string `json:"full"`
					Regular string `json:"regular"`
					Small   string `json:"small"`
					Thumb   string `json:"thumb"`
				} `json:"urls"`
				User struct {
					Name string `json:"name"`
				} `json:"user"`
				Width  int    `json:"width"`
				Height int    `json:"height"`
				Links  struct {
					HTML string `json:"html"`
				} `json:"links"`
			} `json:"results"`
		}

		decodeErr := json.NewDecoder(resp.Body).Decode(&unsResp)
		resp.Body.Close()
		if decodeErr != nil {
			return nil, fmt.Errorf("lỗi parse response Unsplash: %v", decodeErr)
		}

		total = unsResp.Total
		if len(unsResp.Results) == 0 {
			break
		}

		for _, r := range unsResp.Results {
			if len(entries) >= maxCount {
				break
			}
			title := r.Description
			if title == "" {
				title = r.AltDesc
			}
			entries = append(entries, ImageEntry{
				ID:       "unsplash_" + r.ID,
				URL:      r.URLs.Regular,
				ThumbURL: r.URLs.Small,
				Title:    title,
				Author:   r.User.Name,
				Source:   "unsplash",
				Width:    r.Width,
				Height:   r.Height,
				PageURL:  r.Links.HTML,
			})
		}

		if len(entries) >= total {
			break
		}
		page++
		time.Sleep(100 * time.Millisecond)
	}

	if len(entries) == 0 {
		return nil, fmt.Errorf("không tìm thấy ảnh nào cho từ khóa '%s' trên Unsplash", query)
	}

	return &ImageSearchResult{
		Source:  "unsplash",
		Query:   query,
		Total:   total,
		Entries: entries,
	}, nil
}

// ─── PEXELS SEARCH ─────────────────────────────────────────────────────────────

// searchPexels tìm kiếm ảnh qua Pexels API (cần API key).
func searchPexels(ctx context.Context, query string, apiKey string, maxCount int) (*ImageSearchResult, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("cần nhập Pexels API Key. Đăng ký miễn phí tại https://www.pexels.com/api/")
	}

	var entries []ImageEntry
	page := 1
	totalResults := 0
	client := newHTTPClient()

	for len(entries) < maxCount {
		perPage := maxCount - len(entries)
		if perPage > 80 {
			perPage = 80
		}

		apiURL := fmt.Sprintf(
			"https://api.pexels.com/v1/search?query=%s&page=%d&per_page=%d",
			url.QueryEscape(query), page, perPage,
		)

		req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
		if err != nil {
			return nil, fmt.Errorf("lỗi tạo request Pexels: %v", err)
		}
		req.Header.Set("Authorization", apiKey)
		addCommonHeaders(req)

		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("lỗi gửi request Pexels: %v", err)
		}

		if resp.StatusCode == 401 || resp.StatusCode == 403 {
			resp.Body.Close()
			return nil, fmt.Errorf("Pexels API Key không hợp lệ. Đăng ký tại https://www.pexels.com/api/")
		}

		var pxResp struct {
			TotalResults int `json:"total_results"`
			Photos       []struct {
				ID     int    `json:"id"`
				Width  int    `json:"width"`
				Height int    `json:"height"`
				Alt    string `json:"alt"`
				URL    string `json:"url"`
				Src    struct {
					Original string `json:"original"`
					Large    string `json:"large"`
					Medium   string `json:"medium"`
					Small    string `json:"small"`
				} `json:"src"`
				Photographer string `json:"photographer"`
			} `json:"photos"`
		}

		decodeErr := json.NewDecoder(resp.Body).Decode(&pxResp)
		resp.Body.Close()
		if decodeErr != nil {
			return nil, fmt.Errorf("lỗi parse response Pexels: %v", decodeErr)
		}

		totalResults = pxResp.TotalResults
		if len(pxResp.Photos) == 0 {
			break
		}

		for _, p := range pxResp.Photos {
			if len(entries) >= maxCount {
				break
			}
			imgURL := p.Src.Large
			if imgURL == "" {
				imgURL = p.Src.Medium
			}
			entries = append(entries, ImageEntry{
				ID:       fmt.Sprintf("pexels_%d", p.ID),
				URL:      imgURL,
				ThumbURL: p.Src.Small,
				Title:    p.Alt,
				Author:   p.Photographer,
				Source:   "pexels",
				Width:    p.Width,
				Height:   p.Height,
				PageURL:  p.URL,
			})
		}

		if len(entries) >= totalResults {
			break
		}
		page++
		time.Sleep(100 * time.Millisecond)
	}

	if len(entries) == 0 {
		return nil, fmt.Errorf("không tìm thấy ảnh nào cho từ khóa '%s' trên Pexels", query)
	}

	return &ImageSearchResult{
		Source:  "pexels",
		Query:   query,
		Total:   totalResults,
		Entries: entries,
	}, nil
}

// ─── DOWNLOAD IMAGES ───────────────────────────────────────────────────────────

// DownloadImages tải danh sách ảnh song song (giới hạn 4 luồng).
// progressCb: callback cập nhật tiến trình
func DownloadImages(
	ctx context.Context,
	entries []ImageEntry,
	outputDir string,
	progressCb func(ImageDownloadProgress),
) ([]ImageDownloadResult, error) {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, fmt.Errorf("không tạo được thư mục: %v", err)
	}

	results := make([]ImageDownloadResult, len(entries))
	var wg sync.WaitGroup
	var mu sync.Mutex
	done := 0
	total := len(entries)
	sem := make(chan struct{}, 4) // 4 luồng song song

	for i, entry := range entries {
		wg.Add(1)
		sem <- struct{}{}
		go func(idx int, e ImageEntry) {
			defer wg.Done()
			defer func() { <-sem }()

			// Emit progress bắt đầu
			mu.Lock()
			if progressCb != nil {
				progressCb(ImageDownloadProgress{
					ID:      e.ID,
					Title:   e.Title,
					Percent: 0,
					Done:    done,
					Total:   total,
				})
			}
			mu.Unlock()

			result := downloadSingleImage(ctx, e, outputDir)

			mu.Lock()
			done++
			results[idx] = result
			if progressCb != nil {
				progressCb(ImageDownloadProgress{
					ID:      e.ID,
					Title:   e.Title,
					Percent: float64(done) / float64(total) * 100,
					Done:    done,
					Total:   total,
				})
			}
			mu.Unlock()
		}(i, entry)
	}

	wg.Wait()
	return results, nil
}

// downloadSingleImage tải một ảnh về thư mục outputDir.
func downloadSingleImage(ctx context.Context, entry ImageEntry, outputDir string) ImageDownloadResult {
	result := ImageDownloadResult{ID: entry.ID, Title: entry.Title}

	if entry.URL == "" {
		result.Error = "URL ảnh trống"
		return result
	}

	// Xác định tên file từ URL
	fileName := generateFileName(entry)
	filePath := filepath.Join(outputDir, fileName)

	client := newHTTPClient()
	req, err := http.NewRequestWithContext(ctx, "GET", entry.URL, nil)
	if err != nil {
		result.Error = fmt.Sprintf("lỗi tạo request: %v", err)
		return result
	}
	addCommonHeaders(req)
	req.Header.Set("Referer", "https://www.google.com/")

	resp, err := client.Do(req)
	if err != nil {
		result.Error = fmt.Sprintf("lỗi tải ảnh: %v", err)
		return result
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		result.Error = fmt.Sprintf("server trả về lỗi %d", resp.StatusCode)
		return result
	}

	// Xác định extension từ Content-Type
	ct := resp.Header.Get("Content-Type")
	ext := extensionFromContentType(ct)
	if ext != "" && !strings.HasSuffix(fileName, ext) {
		fileName = strings.TrimSuffix(fileName, filepath.Ext(fileName)) + ext
		filePath = filepath.Join(outputDir, fileName)
	}

	// Ghi file
	f, err := os.Create(filePath)
	if err != nil {
		result.Error = fmt.Sprintf("lỗi tạo file: %v", err)
		return result
	}
	defer f.Close()

	if _, err := io.Copy(f, resp.Body); err != nil {
		result.Error = fmt.Sprintf("lỗi ghi file: %v", err)
		os.Remove(filePath)
		return result
	}

	result.FilePath = filePath
	result.OK = true
	return result
}

// ─── HELPERS ───────────────────────────────────────────────────────────────────

// isImageURL kiểm tra URL có phải là ảnh không.
func isImageURL(rawURL string) bool {
	lower := strings.ToLower(rawURL)
	exts := []string{".jpg", ".jpeg", ".png", ".webp", ".gif", ".bmp"}
	for _, ext := range exts {
		if strings.Contains(lower, ext) {
			return true
		}
	}
	// Các domain thường chứa ảnh
	domains := []string{"i.imgur.com", "images.unsplash.com", "cdn.pixabay.com", "images.pexels.com"}
	for _, d := range domains {
		if strings.Contains(lower, d) {
			return true
		}
	}
	return false
}

// generateFileName tạo tên file an toàn cho ảnh.
func generateFileName(entry ImageEntry) string {
	// Lấy đuôi file từ URL
	parsedURL, err := url.Parse(entry.URL)
	ext := ".jpg"
	if err == nil {
		path := parsedURL.Path
		if e := filepath.Ext(path); e != "" {
			ext = strings.ToLower(e)
			// Chỉ chấp nhận các định dạng ảnh phổ biến
			switch ext {
			case ".jpg", ".jpeg", ".png", ".webp", ".gif":
			default:
				ext = ".jpg"
			}
		}
	}

	// Tạo tên từ ID và thời gian
	safeName := sanitizeFileName(entry.ID)
	if safeName == "" {
		safeName = fmt.Sprintf("image_%d", time.Now().UnixNano())
	}
	return safeName + ext
}

// sanitizeFileName loại bỏ ký tự không hợp lệ trong tên file.
func sanitizeFileName(s string) string {
	// Giới hạn độ dài
	if len(s) > 80 {
		s = s[:80]
	}
	// Thay ký tự đặc biệt bằng _
	re := regexp.MustCompile(`[\\/:*?"<>|]`)
	s = re.ReplaceAllString(s, "_")
	s = strings.TrimSpace(s)
	return s
}

// extensionFromContentType trả về extension file từ Content-Type header.
func extensionFromContentType(ct string) string {
	switch {
	case strings.Contains(ct, "image/jpeg"):
		return ".jpg"
	case strings.Contains(ct, "image/png"):
		return ".png"
	case strings.Contains(ct, "image/webp"):
		return ".webp"
	case strings.Contains(ct, "image/gif"):
		return ".gif"
	default:
		return ""
	}
}
