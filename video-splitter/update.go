package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// CurrentAppVersion là phiên bản đang chạy. NÂNG số này mỗi lần phát hành bản mới,
// khớp with tag GitHub Release (release.ps1 tự đọc const này để đặt tag).
const CurrentAppVersion = "v2.0.0"

// updateRepo là repo GitHub chứa các bản Release + manifest.json.
const updateRepo = "nguyenhung2203/ToolCatVideo"

// getGitHubToken giải mã token đã XOR mã hóa ngầm để tránh rò rỉ chuỗi khi soi file .exe
func getGitHubToken() string {
	enc := "MD8nCG8UFRAcYxMwZmYHPGcbGR05Ng0dJDJlYR88MTQYAmcGGSNlIg=="
	data, err := base64.StdEncoding.DecodeString(enc)
	if err != nil {
		return ""
	}
	key := byte(0x57)
	for i := range data {
		data[i] ^= key
	}
	return string(data)
}

// ManifestFile mô tả MỘT file trong bộ ứng dụng: đường dẫn tương đối so với thư
// mục app (vd "TrafficTool.exe", "bin/worker.exe", "python_worker/main.py"), mã
// SHA256 để so khác biệt, URL tải trực tiếp từ GitHub Release, và kích thước.
type ManifestFile struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	URL    string `json:"url"`
	Size   int64  `json:"size"`
}

// Manifest là danh mục TOÀN BỘ file của một phiên bản. release.ps1 quét cả thư
// mục build để sinh ra nó -> thêm/sửa file nào cũng tự vào đây, không sót.
type Manifest struct {
	Version string         `json:"version"`
	Notes   string         `json:"notes"`
	Files   []ManifestFile `json:"files"`
}

// UpdateInfo là kết quả kiểm tra cập nhật trả về cho frontend.
type UpdateInfo struct {
	CurrentVersion string `json:"currentVersion"`
	LatestVersion  string `json:"latestVersion"`
	HasUpdate      bool   `json:"hasUpdate"`
	ReleaseNotes   string `json:"releaseNotes"`
	DownloadURL    string `json:"downloadUrl"`
	// DownloadSize: tổng dung lượng (byte) các file THỰC SỰ cần tải (chỉ file đã
	// đổi/thiếu), để UI hiện "cần tải X MB" thay vì cả bộ ~500MB.
	DownloadSize int64  `json:"downloadSize"`
	ChangedCount int    `json:"changedCount"`
	Error        string `json:"error,omitempty"`

	// manifest tải về được giữ lại (không xuất JSON) để ApplyManifestUpdate dùng
	// mà không phải tải lại — tránh lệch phiên bản giữa lúc check và lúc apply.
	manifest *Manifest `json:"-"`
}

// cachedUpdate giữ manifest của lần CheckForUpdates gần nhất cho ApplyManifestUpdate.
var cachedManifest *Manifest

// GetAppVersion trả về phiên bản hiện tại.
func (a *App) GetAppVersion() string {
	return CurrentAppVersion
}

// OpenWebURL mở trang web trên trình duyệt mặc định.
func (a *App) OpenWebURL(urlStr string) error {
	if urlStr == "" {
		return nil
	}
	var cmd *exec.Cmd
	if goruntime.GOOS == "windows" {
		cmd = exec.Command("cmd", "/c", "start", "", urlStr)
	} else if goruntime.GOOS == "darwin" {
		cmd = exec.Command("open", urlStr)
	} else {
		cmd = exec.Command("xdg-open", urlStr)
	}
	return cmd.Start()
}

// sha256File tính SHA256 của một file trên đĩa. Trả về "" nếu không đọc được
// (file thiếu) — coi như khác với mọi hash thật nên sẽ được tải về.
func sha256File(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return ""
	}
	return hex.EncodeToString(h.Sum(nil))
}

// createGitHubHTTPClient tạo HTTP Client tự chuyển tiếp chuyển hướng an toàn cho Private Repo
func createGitHubHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("quá nhiều chuyển hướng")
			}
			// Nếu chuyển hướng sang Domain khác (ví dụ objects.githubusercontent.com của AWS S3), xóa header Authorization
			if len(via) > 0 && req.URL.Host != via[0].URL.Host {
				req.Header.Del("Authorization")
			}
			return nil
		},
	}
}

// fetchManifest tải manifest.json từ Release mới nhất của repo.
func fetchManifest() (*Manifest, string, error) {
	client := createGitHubHTTPClient(30 * time.Second)
	req, err := http.NewRequest("GET", "https://api.github.com/repos/"+updateRepo+"/releases/latest", nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("User-Agent", "TrafficTool-Updater")
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	token := getGitHubToken()
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("không thể kết nối máy chủ GitHub (kiểm tra lại kết nối mạng)")
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return nil, "", fmt.Errorf("chưa phát hành bản Release nào trên GitHub (hoặc repo đang Private)")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("máy chủ GitHub phản hồi mã lỗi: %d", resp.StatusCode)
	}

	var release struct {
		TagName string `json:"tag_name"`
		Body    string `json:"body"`
		HTMLURL string `json:"html_url"`
		Assets  []struct {
			Name               string `json:"name"`
			URL                string `json:"url"`
			BrowserDownloadURL string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, "", fmt.Errorf("lỗi giải mã thông tin phiên bản từ GitHub")
	}

	assetApiMap := make(map[string]string)
	manifestApiURL := ""
	for _, asset := range release.Assets {
		assetApiMap[strings.ToLower(asset.Name)] = asset.URL
		if strings.EqualFold(asset.Name, "manifest.json") {
			manifestApiURL = asset.URL
		}
	}
	if manifestApiURL == "" {
		return nil, release.HTMLURL, fmt.Errorf("bản phát hành thiếu manifest.json (bản cũ không hỗ trợ cập nhật tự động — tải thủ công)")
	}

	mReq, _ := http.NewRequest("GET", manifestApiURL, nil)
	mReq.Header.Set("User-Agent", "TrafficTool-Updater")
	mReq.Header.Set("Accept", "application/octet-stream")
	token = getGitHubToken()
	if token != "" {
		mReq.Header.Set("Authorization", "Bearer "+token)
	}
	mResp, err := client.Do(mReq)
	if err != nil {
		return nil, release.HTMLURL, fmt.Errorf("không tải được manifest.json: %v", err)
	}
	bodyBytes, err := io.ReadAll(mResp.Body)
	if err != nil {
		return nil, release.HTMLURL, fmt.Errorf("không đọc được manifest.json: %v", err)
	}
	bodyBytes = bytes.TrimPrefix(bodyBytes, []byte("\xef\xbb\xbf"))

	var m Manifest
	if err := json.Unmarshal(bodyBytes, &m); err != nil {
		return nil, release.HTMLURL, fmt.Errorf("manifest.json không hợp lệ: %v", err)
	}
	if m.Version == "" {
		m.Version = strings.TrimSpace(release.TagName)
	}
	if m.Notes == "" {
		m.Notes = release.Body
	}

	// Cập nhật lại URL tải file theo API Asset để hỗ trợ Private repo
	for i := range m.Files {
		flatName := strings.ToLower(strings.ReplaceAll(m.Files[i].Path, "/", "__"))
		if apiURL, ok := assetApiMap[flatName]; ok {
			m.Files[i].URL = apiURL
		}
	}

	return &m, release.HTMLURL, nil
}

// CheckForUpdates tải manifest mới nhất, so sánh phiên bản và tính tổng dung lượng
// các file thực sự cần tải (chỉ file khác/thiếu trên máy khách).
func (a *App) CheckForUpdates() (*UpdateInfo, error) {
	info := &UpdateInfo{
		CurrentVersion: CurrentAppVersion,
		LatestVersion:  CurrentAppVersion,
	}

	m, htmlURL, err := fetchManifest()
	info.DownloadURL = htmlURL
	if err != nil {
		info.Error = err.Error()
		return info, nil
	}

	info.LatestVersion = m.Version
	info.ReleaseNotes = m.Notes

	if compareVersions(m.Version, CurrentAppVersion) <= 0 {
		// Đã ở bản mới nhất (hoặc mới hơn) -> không cần cập nhật.
		return info, nil
	}

	// Có phiên bản mới hơn -> tính các file cần tải bằng cách so SHA256 với máy khách.
	appDir := appBaseDir()
	var changed []ManifestFile
	var totalSize int64
	for _, f := range m.Files {
		localPath := filepath.Join(appDir, filepath.FromSlash(f.Path))
		if sha256File(localPath) != f.SHA256 {
			changed = append(changed, f)
			totalSize += f.Size
		}
	}

	info.HasUpdate = true
	info.ChangedCount = len(changed)
	info.DownloadSize = totalSize
	info.manifest = m
	cachedManifest = m
	return info, nil
}

// appBaseDir trả về thư mục chứa app (nơi TrafficTool.exe nằm). Mỗi đường dẫn
// trong manifest tính tương đối so với đây.
func appBaseDir() string {
	ex, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(ex)
}

// ApplyManifestUpdate tải các file đã đổi, verify SHA256, rồi thay thế an toàn.
// File đang bị khóa (exe chính đang chạy) dùng "rename trick"; file khác ghi đè
// trực tiếp. Xong thì khởi động lại app.
func (a *App) ApplyManifestUpdate() error {
	m := cachedManifest
	if m == nil {
		// Chưa check hoặc cache mất -> tải lại manifest.
		fetched, _, err := fetchManifest()
		if err != nil {
			return err
		}
		m = fetched
	}

	appDir := appBaseDir()
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("không xác định được file thực thi: %v", err)
	}
	execName := filepath.Base(execPath)

	// 1. Xác định danh sách file cần tải (khác SHA256).
	var todo []ManifestFile
	var totalSize int64
	for _, f := range m.Files {
		localPath := filepath.Join(appDir, filepath.FromSlash(f.Path))
		if sha256File(localPath) != f.SHA256 {
			todo = append(todo, f)
			totalSize += f.Size
		}
	}
	if len(todo) == 0 {
		runtime.EventsEmit(a.ctx, "update_status", "Không có file nào cần cập nhật.")
		return nil
	}

	runtime.EventsEmit(a.ctx, "update_status",
		fmt.Sprintf("Đang tải %d file (%.1f MB)...", len(todo), float64(totalSize)/1024/1024))

	// 2. Tải TẤT CẢ file mới về dạng "<path>.new" trong cùng thư mục đích, verify
	// SHA256. Chỉ khi TOÀN BỘ tải + verify xong mới bắt đầu thay — để không rơi vào
	// trạng thái nửa vời (một số file mới, một số cũ) nếu tải lỗi giữa chừng.
	client := createGitHubHTTPClient(30 * time.Minute)
	var downloaded int64
	var stagedNewPaths []string // các file .new đã tải xong (để dọn nếu lỗi)
	cleanupStaged := func() {
		for _, p := range stagedNewPaths {
			_ = os.Remove(p)
		}
	}

	for _, f := range todo {
		destPath := filepath.Join(appDir, filepath.FromSlash(f.Path))
		newPath := destPath + ".new"

		// Đảm bảo thư mục cha tồn tại (trường hợp bản mới thêm file trong thư mục mới).
		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			cleanupStaged()
			return fmt.Errorf("không tạo được thư mục cho %s: %v", f.Path, err)
		}

		req, _ := http.NewRequest("GET", f.URL, nil)
		req.Header.Set("User-Agent", "TrafficTool-Updater")
		req.Header.Set("Accept", "application/octet-stream")
		token := getGitHubToken()
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		resp, err := client.Do(req)
		if err != nil {
			cleanupStaged()
			return fmt.Errorf("không tải được %s: %v", f.Path, err)
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			cleanupStaged()
			return fmt.Errorf("tải %s thất bại (HTTP %d)", f.Path, resp.StatusCode)
		}

		out, err := os.Create(newPath)
		if err != nil {
			resp.Body.Close()
			cleanupStaged()
			return fmt.Errorf("không tạo được file tạm %s: %v", newPath, err)
		}

		h := sha256.New()
		buf := make([]byte, 64*1024)
		writeErr := error(nil)
		for {
			n, rErr := resp.Body.Read(buf)
			if n > 0 {
				if _, wErr := out.Write(buf[:n]); wErr != nil {
					writeErr = wErr
					break
				}
				h.Write(buf[:n])
				downloaded += int64(n)
				if totalSize > 0 {
					runtime.EventsEmit(a.ctx, "update_progress", float64(downloaded)/float64(totalSize)*100)
				}
			}
			if rErr == io.EOF {
				break
			}
			if rErr != nil {
				writeErr = rErr
				break
			}
		}
		out.Close()
		resp.Body.Close()

		if writeErr != nil {
			_ = os.Remove(newPath)
			cleanupStaged()
			return fmt.Errorf("lỗi ghi %s: %v", f.Path, writeErr)
		}

		// Verify checksum: file tải về phải khớp SHA256 trong manifest.
		if got := hex.EncodeToString(h.Sum(nil)); got != f.SHA256 {
			_ = os.Remove(newPath)
			cleanupStaged()
			return fmt.Errorf("file %s tải về bị hỏng (checksum không khớp)", f.Path)
		}

		stagedNewPaths = append(stagedNewPaths, newPath)
	}

	runtime.EventsEmit(a.ctx, "update_status", "Tải xong! Đang thay thế file và khởi động lại...")

	// 3. Thay thế: đổi mỗi "<path>.new" thành "<path>". File đang khóa (exe chính)
	// không ghi đè được nhưng đổi tên được -> đổi bản cũ sang ".old" trước rồi mới
	// đưa bản mới vào. Bản .old sẽ được dọn ở lần khởi động sau (cleanupOldUpdateFiles).
	for _, f := range todo {
		destPath := filepath.Join(appDir, filepath.FromSlash(f.Path))
		newPath := destPath + ".new"

		isRunningExe := strings.EqualFold(filepath.Base(destPath), execName)

		if isRunningExe {
			// Exe đang chạy: rename trick (không xóa được, nhưng đổi tên được).
			oldPath := destPath + ".old"
			_ = os.Remove(oldPath)
			if err := os.Rename(destPath, oldPath); err != nil {
				cleanupStaged()
				return fmt.Errorf("không đổi tên được file đang chạy: %v", err)
			}
			if err := os.Rename(newPath, destPath); err != nil {
				// Rollback: đưa bản cũ về chỗ để app vẫn chạy được.
				_ = os.Rename(oldPath, destPath)
				cleanupStaged()
				return fmt.Errorf("không đưa được bản mới vào chỗ: %v", err)
			}
		} else {
			// File thường: thử ghi đè trực tiếp; nếu bị khóa thì đổi bản cũ sang .old.
			if err := os.Remove(destPath); err != nil && !os.IsNotExist(err) {
				oldPath := destPath + ".old"
				_ = os.Remove(oldPath)
				if rErr := os.Rename(destPath, oldPath); rErr != nil {
					cleanupStaged()
					return fmt.Errorf("không thay được %s (file đang bị khóa?): %v", f.Path, rErr)
				}
			}
			if err := os.Rename(newPath, destPath); err != nil {
				cleanupStaged()
				return fmt.Errorf("không đưa được bản mới %s vào chỗ: %v", f.Path, err)
			}
		}
	}

	// 3b. Dọn file THỪA: file có trên máy khách nhưng KHÔNG còn trong manifest bản
	// mới (bản mới đã bỏ đi). Chỉ xóa trong các thư mục mà manifest có quản lý để
	// tránh xóa nhầm nếu manifest lỡ thiếu; user data nằm ở %AppData% nên an toàn.
	pruneStrayFiles(appDir, m, execName)

	// 4. Khởi động lại app mới rồi thoát tiến trình hiện tại.
	restart := exec.Command(execPath)
	restart.Dir = appDir
	if err := restart.Start(); err != nil {
		return fmt.Errorf("không khởi động lại được ứng dụng: %v", err)
	}
	go func() {
		time.Sleep(400 * time.Millisecond)
		os.Exit(0)
	}()
	return nil
}

// cleanupOldUpdateFiles xóa các file "*.old" và "*.new" còn sót từ lần cập nhật
// trước. Gọi lúc khởi động (sau khi bản mới đã chạy nên file .old không còn khóa).
func cleanupOldUpdateFiles() {
	appDir := appBaseDir()
	_ = filepath.WalkDir(appDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		name := strings.ToLower(d.Name())
		if strings.HasSuffix(name, ".old") || strings.HasSuffix(name, ".new") {
			_ = os.Remove(path)
		}
		return nil
	})
}

// pruneStrayFiles xóa file THỪA: file đang có trên máy khách nhưng bản mới đã bỏ
// khỏi manifest. Chỉ xóa file nằm trong THƯ MỤC mà manifest còn quản lý (có ít
// nhất 1 file khác trong đó) — nếu manifest lỡ thiếu nguyên một thư mục thì bỏ
// qua, không xóa liều. Không đụng exe đang chạy và file .old/.new tạm.
//
// An toàn với dữ liệu người dùng: settings.json, projects.db, log... đều nằm ở
// %AppData%\TrafficTool chứ không cạnh exe, nên cạnh exe chỉ có binary của app.
func pruneStrayFiles(appDir string, m *Manifest, execName string) {
	// Tập đường dẫn tuyệt đối hợp lệ theo manifest + tập thư mục manifest quản lý.
	keep := make(map[string]bool)
	managedDirs := make(map[string]bool)
	for _, f := range m.Files {
		abs := filepath.Join(appDir, filepath.FromSlash(f.Path))
		keep[strings.ToLower(abs)] = true
		managedDirs[strings.ToLower(filepath.Dir(abs))] = true
	}

	_ = filepath.WalkDir(appDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		lname := strings.ToLower(d.Name())
		// Bỏ qua file tạm của chính cơ chế cập nhật.
		if strings.HasSuffix(lname, ".old") || strings.HasSuffix(lname, ".new") {
			return nil
		}
		// Không đụng exe đang chạy (đang bị khóa, và là file sống còn).
		if strings.EqualFold(d.Name(), execName) {
			return nil
		}
		lpath := strings.ToLower(path)
		if keep[lpath] {
			return nil // file hợp lệ theo manifest
		}
		// Chỉ xóa nếu thư mục chứa nó được manifest quản lý → tránh xóa liều khi
		// manifest thiếu nguyên một nhánh.
		if managedDirs[strings.ToLower(filepath.Dir(path))] {
			_ = os.Remove(path)
		}
		return nil
	})
}

// compareVersions so sánh 2 chuỗi phiên bản dạng "v1.2.3". Trả về 1 nếu v1>v2,
// -1 nếu v1<v2, 0 nếu bằng.
func compareVersions(v1, v2 string) int {
	v1 = strings.TrimPrefix(strings.TrimSpace(v1), "v")
	v2 = strings.TrimPrefix(strings.TrimSpace(v2), "v")
	parts1 := strings.Split(v1, ".")
	parts2 := strings.Split(v2, ".")

	for i := 0; i < len(parts1) || i < len(parts2); i++ {
		var n1, n2 int
		if i < len(parts1) {
			fmt.Sscanf(parts1[i], "%d", &n1)
		}
		if i < len(parts2) {
			fmt.Sscanf(parts2[i], "%d", &n2)
		}
		if n1 > n2 {
			return 1
		}
		if n1 < n2 {
			return -1
		}
	}
	return 0
}
