package imagedownloader

import (
	"context"
	"crypto/sha1"
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
	// Status cho biết KẾT QUẢ của riêng ảnh này: "downloading" | "done" | "error".
	// Thiếu trường này thì UI chỉ biết tổng số đã xử lý, không phân biệt được ảnh
	// tải thành công với ảnh lỗi — người dùng thấy mọi ảnh "đang tải" mãi mãi.
	Status   string `json:"status"`
	Error    string `json:"error"`
	FilePath string `json:"filePath"`
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
// source: "duckduckgo" | "pinterest" | "pixabay" | "unsplash" | "pexels"
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
	case "pinterest":
		return searchPinterest(ctx, query, apiKey, maxCount)
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

// ─── PINTEREST SEARCH ──────────────────────────────────────────────────────────

func findAndExtractPins(data interface{}, cb func(map[string]interface{})) {
	switch v := data.(type) {
	case map[string]interface{}:
		if _, hasImages := v["images"]; hasImages {
			cb(v)
		}
		for _, child := range v {
			findAndExtractPins(child, cb)
		}
	case []interface{}:
		for _, item := range v {
			findAndExtractPins(item, cb)
		}
	}
}

func extractCSRFToken(cookieStr string) string {
	re := regexp.MustCompile(`csrftoken=([a-zA-Z0-9_-]+)`)
	m := re.FindStringSubmatch(cookieStr)
	if len(m) > 1 {
		return m[1]
	}
	return ""
}

const defaultPinterestCookie = `csrftoken=169a70558a5447a815243ce08ee3c915; _b="AZbju7svdUxC0Jl5KT4PvYBgyFr8oAxRW8i5ou4Gf8gVQuLs9W32zJgdee5KjQhOaMo="; _routing_id="07489393-6845-4e41-bd08-e194c3ed193d"; sessionFunnelEventLogged=1; _auth=1; _pinterest_sess=TWc9PSZoTGtzVWVicS8yYXR0Z09mRTdqTU5uemk0T3ZCaU13dzFTMzNuVjNyeHB1bzBjaFVwalBQYm9FUi9lekpzNCs5QkFJNGMxVUNDbEhuQU1XallXODJxMTNITXcwSHpuYVd2dXB4Y1JxK1pzOHdrUmtxQWlDd2dZMUxFM21jVkIrZnBkY1VxcHpuRUg1QXNodEJVRU83dVVsTEFvK1plaWhHK3kwV1RtdXRYejF4aEJOaWtua1hNMHpUNWRrWmdncjVMU21ZSFVkeTJhT01PdWpKL0pDdEY5cWtJSWtQeFBQaktNQlhxWExrd1VsamwvdytCcWZobXZTdG1HK25VaHFGclBieDcydUFsaytueGNkWG9QYkQ4N0ZXelZEa20yUGJ3WGcyWkIxTzBzQ0dCTlRKRDZqUW1TZUlmdXExSWxpdWNheHlDU1YyYmF4UU53ZEVkWUlqSkx4clpFTG84R1JqNEo0TGh1MGRMUktZY21wODBDOEw4bWl2RU5NNFpjOW1RWDNhVk1Pb1lseFVhbXFzUnl5eG95Tnk2eFlrUk5DelFwbThNWk8vMitTcXc1SDJJTlk2ZkRBdE9XOGhJRXRQY2VjMU5kcCt1dnJmVDNIbjhYakdraHJqMitDQ3dVbHpqYUp3RDVEaTE5RTVTbFhWNFNZQzJjQjgvKzNxSUxNU1FMdndlUmU2Q3pzcXNIbjFPQjBuaTNDbXQrRklFekJ5UHJYTG0vSlNQM2RGOTZSMjd3VVpBMDBrcnNPajV2NFgvM0pxWEsvR3lycUlNeGZnZ3VqRThsOEhaUzJvalRsTDdwaEtoS3I4cUxwNXJFYUd2VTNPOGoyNEplNXlyTDk2SUVWc013SjBDNFI4MkNFaUQzYUZXMVNDYUplcWFpR3g4eXhJVHgwMXpxWGw2WCszcVhKZDVpUHJ4cnZqdEdtYnZKQ0w3SzQzU2ZKS3ZkdXZpZks3OVltQkdxRUk0VDBCZjFOcElRbUgxOENzU2dYSHVhb094VDdGVnV1cElnZ1lpZFFhM1JLczJtLy8wcGtVbGlEUDdrZVFYa1VwTm5odzdOamYxWmRqejM1a1J3QjV0VHFMbVg0bWNzSnNGVGxjWmJIczUrWjdBTG40ZFFBeFhvc2p0a2NaL0Z4Mnh3bzJHclNIMEordVExbUZOQVUzNFdReCtVU1VuTzFXdnNUd09jWVF0d0UvUVQyR29Lb0szbTE1UFI4S0lRRmlENU8xaEViNC9UZjMrd0U4Q1Jxb25mM1l3bytOYzJWWkdRd0JLbHZ4VG5ZYXhQR0F3dStQMlhPSEdSNFJwRmlMSnI3SElYK3ZBbVVqbEFFNHJTL2pLd2JESWhJN2ppR2NSVWxGeXpCTUthd3JlcW15TWg3bW1KaUkyQkVQeFU0ejlKKzZldGVpamswcHFQMjN5N0daMCtFOTM2VC9KbnpVTWI2OUVxMXdwRk5jTHo5dHpLRXg3VklHcEd1SysvQ3Q3T3J1V2NPUHFxMitVL1hWaUJSUlg3QnJ0S0dLOEZBRXBNRDV5UHJuTzFKd1c4OWVHR0dEKzJiaEQ0RCswNWZDRlIwcXFEcUc0N1lXb2poeHdUam05QTQwYWVsR3ZhUTFRdEZwdDh1Z3Qwd2YxOE9ncnlaRW5QVmYxWTUxc0VaV0xCekowUWQvM0tTblpjZHZ0bGFiQXBvMHROL0F4b0w2M0lpcnBlYVpyQXFrZjVGRTFHakhtamVTQmNhUSsyRE1VN0NYak9wcXdGQVUyNk9JY0t0RjV0VXBqOFo1NmUvZUp6TkomaDh4UUtRNEpONWRTTzFuMXJQL0VyeWhJVVdvPQ==; __Secure-s_a=UmV5d3Joc204OFNINUVGeExhajNaV0pybzhqeENtZTA4Z05yVS9FVWtrVmI5dVFYSkJMckR6OWloNUFHbXEwei9SRGRTYjRaYnZKek5qdmsrdVovbW5wTFUwVUx1YndlZDNDS20wOG1pSU84cTdkUzd4RjlrdlA2L1V4T2Jsd0ErdlZJNmM4dXpCUmYzWVVUcVVUMGFpOXlCYkNnNDZaWGpaZ21LNW8xQXlWZHZEWGo4VDRoQUF1M2RZVEtDM3JpYm8wWXh0cklleW91UXNMekdtbXA2V3NaMDZYNmR5MU1uQ1VWRFlTZ3Vja3hvODh6bStnajFHbEZOY2U1LzhReWxSS3FscUdGcG5TZHRwTTRCTG5FNlpoeWh0V2JGRlQzZ1B1UmRXN0llaWsvVUltajUwMStvOVNyWGhaK09JZVlGOVEvY2ZacmY4ZjArVmEwUHhQQXFjZi9jQ0pPRjlUeWxFWGZYTEVMbHNPa0NTWTk1OFp1WVZPa1FZR2NYcEFrVlFHVWxwb29FSFlVMlhSSkJEZytUdGhYYW5HcUxtWmhWbjdHVzNJZS92d2hON0FFTEFCM3pjTUZQY2g4TXNFMzRFd0cwUm5oU3R3ZkkvVldWbmFJSlQzS0D3QnQ5SlpkTUVleUN6ZnFyUlFkelJ0TVVlVjltM2VqME9RSFVsc0tEeUxPd1pja3NhKzh1YUNCSFE4RklSeGdaZms2NHQyYWNuMkh0QWc3ZXFaQk15aVpiT0NsTnpTUFhrOFNuN0hFUzRMQXVYbXdScCs2ck9pNnZLQUpsaytsMmpLSER0QzZKbFcxZ01Gck5MVm44LzVxYSt0TkpKc3h4K1p6Y2NUWG5zSnBJWXlwR2ZmdHRkWnppS1pIOUJsNUE1aXY3VVJ1WEloVEVtV3FkSm5GY2ZBK005M2VlQkE3a3Z1b2RCTnZxVENnYkxvNktmSC9hd0Y4WXdQR1FjUWViN2krWTJzRFM3V2JSSDZBcVRFaExRRUY1aTVXZU9CSVJSa2FPWncxODZEc1d1dGVFOUZCaGxqUlZjQ1ZocnBiTkhLYTQ4cFBvVWhXeHQ1RjVQcUlnekFWTjgxSmJZaE5IVUFLOEVEaVZiblRDbkc3cGlkU1QzR2lvQWNuMHc0b0lNZlpuTmg5YU5HNVE4aktJMUlwNVZKTWpCZ1VGWFY5TmhCUGxqdWdaNlBEZnpVa3pYT3RLSk1mNHBuSmRreXkyQkpCdXZqbXdZTldkR3UxRm10WXgzYzRySmVhbEgzUlR4bW01ck9OT2t3UDlSVkVFRWFvbDVDYkg1bCtQMDJXcjl5dUZpelk3Q3JMTkpldjdhMD0mTlUvcmlEcmNzdUlqSE9oOG9xc1Y4dE0vNVlrPQ==; ujr=1; usersync=%7B%22magnite%22%3A%7B%22id%22%3A%22MQ8YSJ2Z-M-F64B%22%2C%22ts%22%3A1784623031839%7D%7D`

// searchPinterest tìm kiếm ảnh từ Pinterest (hỗ trợ HTML scraping + Cookie API).
func searchPinterest(ctx context.Context, query string, cookie string, maxCount int) (*ImageSearchResult, error) {
	client := newHTTPClient()
	var entries []ImageEntry
	seenSigs := make(map[string]bool)
	cookie = strings.TrimSpace(cookie)
	if cookie == "" {
		cookie = defaultPinterestCookie
	}

	addEntry := func(id, origURL, thumbURL, title, author string, width, height int) {
		if len(entries) >= maxCount {
			return
		}
		if origURL == "" {
			return
		}
		// Bỏ qua ảnh rác / logo Pinterest mặc định (d53b014d86a6b6761bf649a0ed813c2b)
		if strings.Contains(origURL, "d53b014d86a6b6761bf649a0ed813c2b") || strings.Contains(thumbURL, "d53b014d86a6b6761bf649a0ed813c2b") || strings.Contains(id, "d53b014d86a6b6761bf649a0ed813c2b") {
			return
		}
		if seenSigs[origURL] {
			return
		}
		seenSigs[origURL] = true

		if title == "" {
			title = query
		}
		if id == "" {
			id = fmt.Sprintf("pin_%d", len(entries))
		}

		entries = append(entries, ImageEntry{
			ID:       "pinterest_" + id,
			URL:      origURL,
			ThumbURL: thumbURL,
			Title:    title,
			Author:   author,
			Source:   "pinterest",
			Width:    width,
			Height:   height,
			PageURL:  fmt.Sprintf("https://www.pinterest.com/pin/%s/", id),
		})
	}

	// 1. Fetch HTML search page
	htmlURL := fmt.Sprintf("https://www.pinterest.com/search/pins/?q=%s", url.QueryEscape(query))
	htmlReq, errHtml := http.NewRequestWithContext(ctx, "GET", htmlURL, nil)
	if errHtml == nil {
		addCommonHeaders(htmlReq)
		htmlReq.Header.Set("Referer", "https://www.pinterest.com/")
		htmlReq.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
		if cookie != "" {
			htmlReq.Header.Set("Cookie", cookie)
		}

		htmlResp, errDo := client.Do(htmlReq)
		if errDo == nil && htmlResp.StatusCode == http.StatusOK {
			htmlBody, _ := io.ReadAll(htmlResp.Body)
			htmlResp.Body.Close()
			bodyStr := string(htmlBody)

			// Parse JSON inside script tags
			reScript := regexp.MustCompile(`<script[^>]*type="application/json"[^>]*>([\s\S]*?)</script>`)
			scriptMatches := reScript.FindAllStringSubmatch(bodyStr, -1)
			for _, sm := range scriptMatches {
				if len(entries) >= maxCount {
					break
				}
				if len(sm) < 2 {
					continue
				}
				var rawData map[string]interface{}
				if errJson := json.Unmarshal([]byte(sm[1]), &rawData); errJson != nil {
					continue
				}

				findAndExtractPins(rawData, func(pin map[string]interface{}) {
					id, _ := pin["id"].(string)
					title, _ := pin["grid_title"].(string)
					if title == "" {
						title, _ = pin["title"].(string)
					}
					if title == "" {
						title, _ = pin["description"].(string)
					}
					author := ""
					if pinner, ok := pin["pinner"].(map[string]interface{}); ok {
						author, _ = pinner["full_name"].(string)
						if author == "" {
							author, _ = pinner["username"].(string)
						}
					}

					if images, ok := pin["images"].(map[string]interface{}); ok {
						var fullURL, thumbURL string
						var w, h int

						resKeys := []string{"orig", "736x", "474x", "236x", "170x"}
						for _, k := range resKeys {
							if imgObj, ok := images[k].(map[string]interface{}); ok {
								if u, ok := imgObj["url"].(string); ok && u != "" {
									if fullURL == "" {
										fullURL = u
										if widthNum, ok := imgObj["width"].(float64); ok {
											w = int(widthNum)
										}
										if heightNum, ok := imgObj["height"].(float64); ok {
											h = int(heightNum)
										}
									}
								}
							}
						}

						if fullURL == "" {
							for _, imgVal := range images {
								if imgObj, ok := imgVal.(map[string]interface{}); ok {
									if u, ok := imgObj["url"].(string); ok && u != "" {
										fullURL = u
										if widthNum, ok := imgObj["width"].(float64); ok {
											w = int(widthNum)
										}
										if heightNum, ok := imgObj["height"].(float64); ok {
											h = int(heightNum)
										}
										break
									}
								}
							}
						}

						thumbKeys := []string{"236x", "170x", "474x"}
						for _, k := range thumbKeys {
							if imgObj, ok := images[k].(map[string]interface{}); ok {
								if u, ok := imgObj["url"].(string); ok && u != "" {
									thumbURL = u
									break
								}
							}
						}
						if thumbURL == "" {
							thumbURL = fullURL
						}

						if fullURL != "" {
							addEntry(id, fullURL, thumbURL, title, author, w, h)
						}
					}
				})
			}

			// Fallback: Regex scan for image signature paths in HTML
			if len(entries) < maxCount {
				rePinImg := regexp.MustCompile(`https:\\?/\\?/i\.pinimg\.com\\?/(originals|[0-9]+x)\\?/([a-f0-9]{2})\\?/([a-f0-9]{2})\\?/([a-f0-9]{2})\\?/([a-f0-9]{32})\.(jpg|png|webp)`)
				imgMatches := rePinImg.FindAllStringSubmatch(bodyStr, maxCount*10)
				for _, m := range imgMatches {
					if len(entries) >= maxCount {
						break
					}
					if len(m) < 7 {
						continue
					}
					p1, p2, p3, sig, ext := m[2], m[3], m[4], m[5], m[6]
					origURL := fmt.Sprintf("https://i.pinimg.com/originals/%s/%s/%s/%s.%s", p1, p2, p3, sig, ext)
					thumbURL := fmt.Sprintf("https://i.pinimg.com/236x/%s/%s/%s/%s.jpg", p1, p2, p3, sig)

					addEntry(sig, origURL, thumbURL, query, "", 0, 0)
				}
			}
		}
	}

	// 2. Fetch via Pinterest BaseSearchResource API (cho phép trang tiếp theo / dùng Cookie người dùng cung cấp)
	bookmark := ""
	for len(entries) < maxCount {
		queryEscaped := strings.ReplaceAll(url.QueryEscape(query), "+", "%20")
		sourceURL := fmt.Sprintf("/search/pins/?q=%s&rs=typed", queryEscaped)

		optionsMap := map[string]interface{}{
			"query":                    query,
			"scope":                    "pins",
			"appliedProductFilters":    "---",
			"domains":                  nil,
			"user":                     nil,
			"seoDrawerEnabled":         false,
			"applied_unified_filters":  nil,
			"auto_correction_disabled": false,
			"journey_depth":            nil,
			"source_id":                nil,
			"source_module_id":         nil,
			"source_url":               sourceURL,
			"static_feed":              false,
			"selected_one_bar_modules":  nil,
			"query_pin_sigs":           nil,
			"page_size":                nil,
			"price_max":                nil,
			"price_min":                nil,
			"query_image_pins":         nil,
			"request_params":           nil,
			"top_pin_ids":              nil,
			"article":                  nil,
			"corpus":                   nil,
			"filters":                  nil,
			"rs":                       "typed",
		}
		if bookmark != "" {
			optionsMap["bookmarks"] = []string{bookmark}
		}

		dataObj := map[string]interface{}{
			"options": optionsMap,
			"context": map[string]interface{}{},
		}

		dataJSON, err := json.Marshal(dataObj)
		if err != nil {
			break
		}

		encodeParam := func(s string) string {
			return strings.ReplaceAll(url.QueryEscape(s), "+", "%20")
		}

		apiURL := fmt.Sprintf(
			"https://www.pinterest.com/resource/BaseSearchResource/get/?source_url=%s&data=%s",
			encodeParam(sourceURL),
			encodeParam(string(dataJSON)),
		)

		req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
		if err != nil {
			break
		}

		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
		req.Header.Set("Accept", "application/json, text/javascript, */*; q=0.01")
		req.Header.Set("Accept-Language", "vi,en;q=0.9")
		req.Header.Set("Referer", "https://www.pinterest.com/")
		req.Header.Set("X-Requested-With", "XMLHttpRequest")
		req.Header.Set("x-pinterest-appstate", "active")
		req.Header.Set("x-pinterest-pws-handler", "www/index.js")
		req.Header.Set("sec-ch-ua", `"Not;A=Brand";v="8", "Chromium";v="150", "Google Chrome";v="150"`)
		req.Header.Set("sec-ch-ua-mobile", "?0")
		req.Header.Set("sec-ch-ua-platform", `"Windows"`)
		req.Header.Set("sec-fetch-dest", "empty")
		req.Header.Set("sec-fetch-mode", "cors")
		req.Header.Set("sec-fetch-site", "same-origin")

		if cookie != "" {
			req.Header.Set("Cookie", cookie)
			if csrf := extractCSRFToken(cookie); csrf != "" {
				req.Header.Set("X-CSRFToken", csrf)
			}
		}

		resp, err := client.Do(req)
		if err != nil {
			break
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			break
		}

		bodyBytes, errRead := io.ReadAll(resp.Body)
		resp.Body.Close()
		if errRead != nil {
			break
		}

		var pinResp struct {
			ResourceResponse struct {
				Bookmark string `json:"bookmark"`
				Data     struct {
					Results []struct {
						ID          string `json:"id"`
						Title       string `json:"title"`
						GridTitle   string `json:"grid_title"`
						Description string `json:"description"`
						Images      map[string]struct {
							URL    string `json:"url"`
							Width  int    `json:"width"`
							Height int    `json:"height"`
						} `json:"images"`
						Pinner struct {
							FullName string `json:"full_name"`
							Username string `json:"username"`
						} `json:"pinner"`
					} `json:"results"`
				} `json:"data"`
			} `json:"resource_response"`
		}

		if errUnmarshal := json.Unmarshal(bodyBytes, &pinResp); errUnmarshal == nil {
			results := pinResp.ResourceResponse.Data.Results
			if len(results) == 0 {
				fmt.Println("[Pinterest API Debug] status 200 but results count is 0. Body:", string(bodyBytes))
				break
			}
			for _, r := range results {
				var fullURL, thumbURL string
				var w, h int
				resKeys := []string{"orig", "736x", "474x", "236x", "170x"}
				for _, k := range resKeys {
					if imgObj, ok := r.Images[k]; ok && imgObj.URL != "" {
						fullURL = imgObj.URL
						w = imgObj.Width
						h = imgObj.Height
						break
					}
				}
				if fullURL == "" {
					for _, imgObj := range r.Images {
						if imgObj.URL != "" {
							fullURL = imgObj.URL
							w = imgObj.Width
							h = imgObj.Height
							break
						}
					}
				}
				thumbKeys := []string{"236x", "170x", "474x"}
				for _, k := range thumbKeys {
					if imgObj, ok := r.Images[k]; ok && imgObj.URL != "" {
						thumbURL = imgObj.URL
						break
					}
				}
				if thumbURL == "" {
					thumbURL = fullURL
				}

				title := strings.TrimSpace(r.GridTitle)
				if title == "" {
					title = strings.TrimSpace(r.Title)
				}
				if title == "" {
					title = strings.TrimSpace(r.Description)
				}
				author := strings.TrimSpace(r.Pinner.FullName)
				if author == "" {
					author = strings.TrimSpace(r.Pinner.Username)
				}

				if fullURL != "" {
					addEntry(r.ID, fullURL, thumbURL, title, author, w, h)
				}
			}

			nextBookmark := pinResp.ResourceResponse.Bookmark
			if nextBookmark == "" || nextBookmark == bookmark {
				break
			}
			bookmark = nextBookmark
			time.Sleep(150 * time.Millisecond)
		} else {
			break
		}
	}

	// 3. Bổ sung nguồn Pinterest qua site:pinterest.com nếu kết quả trực tiếp chưa đủ maxCount
	if len(entries) < maxCount {
		ddgQuery := fmt.Sprintf("site:pinterest.com %s", query)
		ddgRes, errDDG := searchDuckDuckGo(ctx, ddgQuery, maxCount*2)
		if errDDG == nil && ddgRes != nil {
			for _, item := range ddgRes.Entries {
				if len(entries) >= maxCount {
					break
				}
				origURL := item.URL
				if strings.Contains(origURL, "pinimg.com") {
					for _, sizeKey := range []string{"/736x/", "/474x/", "/236x/", "/170x/", "/136x136/"} {
						if strings.Contains(origURL, sizeKey) {
							origURL = strings.Replace(origURL, sizeKey, "/originals/", 1)
							break
						}
					}
				}
				addEntry(item.ID, origURL, item.ThumbURL, item.Title, item.Author, item.Width, item.Height)
			}
		}
	}

	if len(entries) == 0 {
		return nil, fmt.Errorf("không tìm thấy ảnh nào cho từ khóa '%s' trên Pinterest", query)
	}

	return &ImageSearchResult{
		Source:  "pinterest",
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
					Status:  "downloading",
				})
			}
			mu.Unlock()

			result := downloadSingleImage(ctx, e, outputDir)

			mu.Lock()
			done++
			results[idx] = result
			if progressCb != nil {
				status := "error"
				if result.OK {
					status = "done"
				}
				progressCb(ImageDownloadProgress{
					ID:       e.ID,
					Title:    e.Title,
					Percent:  float64(done) / float64(total) * 100,
					Done:     done,
					Total:    total,
					Status:   status,
					Error:    result.Error,
					FilePath: result.FilePath,
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

	// Tên file đã gồm hash URL nên trùng tên = ĐÚNG ảnh đó đã tải trước đó. Bỏ qua
	// luôn, khỏi tốn băng thông và khỏi sinh bản sao "_1, _2" của cùng một ảnh.
	if st, err := os.Stat(filePath); err == nil && st.Size() >= 512 {
		result.FilePath = filePath
		result.OK = true
		return result
	}

	client := newHTTPClient()
	req, err := http.NewRequestWithContext(ctx, "GET", entry.URL, nil)
	if err != nil {
		result.Error = fmt.Sprintf("lỗi tạo request: %v", err)
		return result
	}
	addCommonHeaders(req)
	if entry.Source == "pinterest" || strings.Contains(entry.URL, "pinimg.com") {
		req.Header.Set("Referer", "https://www.pinterest.com/")
	} else {
		req.Header.Set("Referer", "https://www.google.com/")
	}

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

	// Chặn trang HTML: nhiều CDN trả về trang "chặn bot" / trang lỗi kèm status 200.
	// Nếu cứ lưu, người dùng nhận file .jpg mở không được mà tưởng tải thành công.
	ct := resp.Header.Get("Content-Type")
	lowerCT := strings.ToLower(ct)
	if strings.Contains(lowerCT, "text/") || strings.Contains(lowerCT, "html") || strings.Contains(lowerCT, "json") {
		result.Error = "server trả về trang web thay vì ảnh (có thể bị chặn bot)"
		return result
	}

	// Xác định extension từ Content-Type
	ext := extensionFromContentType(ct)
	if ext != "" && !strings.HasSuffix(fileName, ext) {
		fileName = strings.TrimSuffix(fileName, filepath.Ext(fileName)) + ext
		filePath = filepath.Join(outputDir, fileName)
	}

	// Chốt tên file cuối cùng: không ghi đè ảnh cũ, không đụng nhau giữa 4 luồng tải.
	filePath = uniqueFilePath(filePath)

	// Ghi file
	f, err := os.Create(filePath)
	if err != nil {
		result.Error = fmt.Sprintf("lỗi tạo file: %v", err)
		return result
	}

	// Giới hạn dung lượng để 1 URL lỗi (stream vô hạn / file 2GB) không làm đầy ổ đĩa.
	const maxImageBytes = 50 << 20 // 50MB
	written, copyErr := io.Copy(f, io.LimitReader(resp.Body, maxImageBytes+1))
	closeErr := f.Close()

	if copyErr != nil {
		os.Remove(filePath)
		result.Error = fmt.Sprintf("lỗi ghi file: %v", copyErr)
		return result
	}
	if closeErr != nil {
		os.Remove(filePath)
		result.Error = fmt.Sprintf("lỗi ghi file: %v", closeErr)
		return result
	}
	if written > maxImageBytes {
		os.Remove(filePath)
		result.Error = "ảnh quá lớn (>50MB), đã bỏ qua"
		return result
	}
	// File rỗng / quá nhỏ thì chắc chắn không phải ảnh dùng được.
	if written < 512 {
		os.Remove(filePath)
		result.Error = fmt.Sprintf("file tải về không hợp lệ (%d bytes)", written)
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
	domains := []string{"i.imgur.com", "images.unsplash.com", "cdn.pixabay.com", "images.pexels.com", "i.pinimg.com"}
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

	// Tên file phải DUY NHẤT THEO URL ẢNH, không theo thứ tự trong 1 lần tìm. ID do
	// các hàm search sinh ra chỉ là số đếm (ddg_0, ddg_1...) nên tìm "mèo" tải xong,
	// rồi tìm "chó" tải vào CÙNG thư mục sẽ tạo lại ddg_0.jpg và GHI ĐÈ ảnh mèo cũ.
	// Ghép thêm hash của URL để 2 ảnh khác nhau không bao giờ cùng tên, mà tải lại
	// đúng ảnh đó vẫn cho cùng tên (không sinh rác trùng nội dung).
	safeName := sanitizeFileName(entry.ID)
	if safeName == "" {
		safeName = "image"
	}
	sum := sha1.Sum([]byte(entry.URL))
	return fmt.Sprintf("%s_%x%s", safeName, sum[:4], ext)
}

// uniqueFilePath thêm hậu tố _1, _2... nếu file đã tồn tại, để không bao giờ ghi đè
// ảnh cũ. Bọc trong mutex vì DownloadImages chạy 4 luồng song song: 2 goroutine cùng
// thấy "chưa tồn tại" rồi cùng os.Create một đường dẫn sẽ mất 1 ảnh.
var uniqueNameMu sync.Mutex

func uniqueFilePath(path string) string {
	uniqueNameMu.Lock()
	defer uniqueNameMu.Unlock()

	if _, err := os.Stat(path); os.IsNotExist(err) {
		// Đặt chỗ ngay để luồng song song khác không chọn trùng tên này.
		if f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644); err == nil {
			_ = f.Close()
			return path
		}
	}
	ext := filepath.Ext(path)
	base := strings.TrimSuffix(path, ext)
	for i := 1; i < 10000; i++ {
		cand := fmt.Sprintf("%s_%d%s", base, i, ext)
		if _, err := os.Stat(cand); os.IsNotExist(err) {
			if f, err := os.OpenFile(cand, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644); err == nil {
				_ = f.Close()
				return cand
			}
		}
	}
	return fmt.Sprintf("%s_%d%s", base, time.Now().UnixNano(), ext)
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
