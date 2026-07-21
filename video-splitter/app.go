package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"video-splitter/internal/boundary"
	"video-splitter/internal/downloader"
	"video-splitter/internal/exporter"
	"video-splitter/internal/imagedownloader"
	"video-splitter/internal/media"
	"video-splitter/internal/project"
	"video-splitter/internal/storage"
	"video-splitter/internal/utils"
	"video-splitter/internal/browserai"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	"google.golang.org/genai"
)

// hashPath trả về một khóa ngắn duy nhất theo đường dẫn nguồn, dùng để đặt tên
// thư mục tạm riêng cho mỗi video (tránh nhiều video ghi đè proxy/audio của nhau).
func hashPath(sourcePath string) string {
	sum := sha1.Sum([]byte(sourcePath))
	return hex.EncodeToString(sum[:])[:12]
}

// App struct
type App struct {
	ctx               context.Context
	browserAIService  *browserai.Service
	cancelFuncs       map[string]context.CancelFunc
	cancelMu          sync.Mutex
	exportCancelFuncs map[string]context.CancelFunc
	exportCancelMu    sync.Mutex
	isExportCancelled bool
	streamPort        int
	streamToken       string
	store             *storage.Store
	// Download online video (yt-dlp)
	downloadCancelFuncs map[string]context.CancelFunc
	downloadCancelMu    sync.Mutex
	isDownloadCancelled bool
	// Download images
	imageDownloadCancel context.CancelFunc
	imageDownloadMu     sync.Mutex
}

// NewApp creates a new App application struct
func NewApp(browserAIService *browserai.Service) *App {
	return &App{
		browserAIService: browserAIService,
	}
}

// startup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	if a.browserAIService != nil {
		a.browserAIService.Startup(ctx)
	}
	a.cancelFuncs = make(map[string]context.CancelFunc)
	a.exportCancelFuncs = make(map[string]context.CancelFunc)
	a.isExportCancelled = false
	a.downloadCancelFuncs = make(map[string]context.CancelFunc)
	a.isDownloadCancelled = false
	a.imageDownloadCancel = nil

	// Mở kho lưu trữ SQLite (lưu project/clip/config để mở lại không mất việc).
	// Lỗi mở db không nên chặn app khởi động — chỉ mất tính năng lưu.
	if store, err := storage.Open(storage.DefaultDBPath()); err == nil {
		a.store = store
	} else {
		fmt.Printf("Không mở được database, tính năng lưu project sẽ tắt: %v\n", err)
	}

	// Token ngẫu nhiên bảo vệ stream server: chỉ request kèm đúng token mới được phục vụ,
	// tránh việc bất kỳ tiến trình local nào cũng đọc được file tùy ý qua ?path=.
	tokenBytes := make([]byte, 16)
	if _, err := rand.Read(tokenBytes); err == nil {
		a.streamToken = hex.EncodeToString(tokenBytes)
	}

	// Khởi chạy HTTP Server phục vụ stream video cục bộ (hỗ trợ tua video)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err == nil {
		a.streamPort = listener.Addr().(*net.TCPAddr).Port
		go func() {
			mux := http.NewServeMux()
			mux.HandleFunc("/stream", func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Access-Control-Allow-Origin", "*")
				if r.URL.Query().Get("token") != a.streamToken {
					http.Error(w, "forbidden", http.StatusForbidden)
					return
				}
				filePath := r.URL.Query().Get("path")
				if filePath != "" {
					http.ServeFile(w, r, filePath)
				}
			})
			http.Serve(listener, mux)
		}()
	}
}

// shutdown được gọi khi app đóng — đóng database để flush an toàn.
func (a *App) shutdown(ctx context.Context) {
	if a.browserAIService != nil {
		a.browserAIService.Shutdown(ctx)
	}
	if a.store != nil {
		_ = a.store.Close()
	}
}

// GetStreamURL trả về địa chỉ HTTP cục bộ để phát video.
// Path được encode đúng chuẩn query (xử lý dấu cách, ký tự đặc biệt) và kèm token.
func (a *App) GetStreamURL(filePath string) string {
	q := url.Values{}
	q.Set("path", filePath)
	q.Set("token", a.streamToken)
	return fmt.Sprintf("http://127.0.0.1:%d/stream?%s", a.streamPort, q.Encode())
}

// SelectFiles mở hộp thoại chọn nhiều file video
func (a *App) SelectFiles() ([]string, error) {
	selection, err := runtime.OpenMultipleFilesDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Chọn Video",
		Filters: []runtime.FileFilter{
			{
				DisplayName: "Video Files (*.mp4;*.mkv;*.avi)",
				Pattern:     "*.mp4;*.mkv;*.avi",
			},
		},
	})
	if err != nil {
		return nil, err
	}
	return selection, nil
}

// SelectFolder mở hộp thoại chọn thư mục xuất
func (a *App) SelectFolder() (string, error) {
	dir, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Chọn thư mục xuất video",
	})
	if err != nil {
		return "", err
	}
	return dir, nil
}

// FetchImageAsBase64 tải ảnh từ URL bên ngoài và trả về dạng base64 data URI,
// giúp WebView2 hiển thị ảnh thumbnail từ YouTube/TikTok mà không bị chặn CSP.
func (a *App) FetchImageAsBase64(imageURL string) string {
	if imageURL == "" {
		return ""
	}
	client := &http.Client{Timeout: 8 * time.Second}
	req, err := http.NewRequest("GET", imageURL, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	req.Header.Set("Referer", "https://www.youtube.com/")
	resp, err := client.Do(req)
	if err != nil || resp.StatusCode != 200 {
		return ""
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return ""
	}
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "image/jpeg"
	}
	b64 := base64.StdEncoding.EncodeToString(data)
	return "data:" + contentType + ";base64," + b64
}

// GetVideoInfo lấy thông tin của video qua ffprobe
func (a *App) GetVideoInfo(filePath string) (*project.VideoInfo, error) {
	return media.GetVideoInfo(filePath)
}

// GetDefaultConfig trả về cấu hình mặc định cho frontend.
// Tự động phát hiện GPU của máy và bật tăng tốc phần cứng nếu có.
func (a *App) GetDefaultConfig() project.AnalyzerConfig {
	cfg := project.DefaultConfig()
	// Tự động phát hiện GPU: nvidia / intel / amd / none
	cfg.HardwareAccel = media.DetectGPU()
	return cfg
}

// Analyze chạy pipeline phân tích (Proxy -> Audio -> Python Worker -> Boundary Score)
func (a *App) Analyze(sourcePath string, cfg project.AnalyzerConfig) ([]project.Clip, error) {
	if cfg.Mode == "fixed" {
		return a.analyzeFixed(sourcePath, cfg)
	}

	// Phân giải GPU tự động: nếu người dùng chọn "auto", tự phát hiện GPU phù hợp
	if cfg.HardwareAccel == "auto" || cfg.HardwareAccel == "" {
		cfg.HardwareAccel = media.DetectGPU()
	}

	// Thư mục tạm riêng theo từng video (hash đường dẫn).
	key := hashPath(sourcePath)
	workDir := filepath.Join(os.TempDir(), "video-splitter", key)
	_ = os.MkdirAll(workDir, 0755)
	proxyPath := filepath.Join(workDir, "proxy.mp4")
	audioPath := filepath.Join(workDir, "audio.wav")

	// Dọn dẹp proxy/audio sau khi hoàn tất.
	defer func() {
		_ = os.Remove(proxyPath)
		_ = os.Remove(audioPath)
	}()

	ctx, cancel := context.WithCancel(a.ctx)
	a.cancelMu.Lock()
	a.cancelFuncs[sourcePath] = cancel
	a.cancelMu.Unlock()
	defer func() {
		a.cancelMu.Lock()
		delete(a.cancelFuncs, sourcePath)
		a.cancelMu.Unlock()
	}()

	// 1. Tạo proxy (và trích xuất audio nếu dùng chế độ precise)
	// fast mode: KHÔNG tạo proxy — chạy thẳng trên video gốc để tiết kiệm thời gian
	// smart/precise: tạo proxy 320×180 4fps để phân tích nhanh hơn nhiều
	needProxy := cfg.Mode != project.ModeFast
	needAudio := cfg.Mode == project.ModePrecise // WAV chỉ cần cho Librosa (precise mode)

	// Lấy thời lượng video gốc để tính tiến độ thời gian thực cho Bước 1
	var totalDuration float64
	if vi, err := media.GetVideoInfo(sourcePath); err == nil && vi != nil {
		totalDuration = vi.Duration
	}

	var errProxy, errAudio error
	if needProxy || needAudio {
		gpuLabel := cfg.HardwareAccel
		if gpuLabel == "" || gpuLabel == "none" {
			gpuLabel = "CPU"
		} else {
			gpuLabel = strings.ToUpper(gpuLabel)
		}
		if needProxy && needAudio {
			runtime.EventsEmit(a.ctx, "analyze_log", fmt.Sprintf("Bước 1/3: Đang tối ưu hóa video bằng %s (tạo proxy 320×180 4fps & trích xuất âm thanh song song)...", gpuLabel))
		} else if needProxy {
			runtime.EventsEmit(a.ctx, "analyze_log", fmt.Sprintf("Bước 1/3: Đang tối ưu hóa video bằng %s (tạo proxy 320×180 4fps)...", gpuLabel))
		} else {
			runtime.EventsEmit(a.ctx, "analyze_log", "Bước 1/3: Đang trích xuất âm thanh...")
		}
		runtime.EventsEmit(a.ctx, "analyze_progress", map[string]interface{}{"path": sourcePath, "progress": 1})

		var wg sync.WaitGroup

		if needProxy {
			wg.Add(1)
			go func() {
				defer wg.Done()
				proxyFPS := strconv.Itoa(cfg.ProxyFPS)
				// Truyền callback tiến độ: map % proxy (0-100) vào khoảng (1-14) trên thanh tổng
				errProxy = media.GenerateProxy(ctx, sourcePath, proxyPath, proxyFPS, cfg.HardwareAccel, totalDuration, func(pct int) {
					realProg := 1 + (pct * 13 / 100) // 1% → 14%
					runtime.EventsEmit(a.ctx, "analyze_progress", map[string]interface{}{"path": sourcePath, "progress": realProg})
				})
			}()
		}

		if needAudio {
			wg.Add(1)
			go func() {
				defer wg.Done()
				errAudio = media.ExtractAudio(ctx, sourcePath, audioPath)
			}()
		}

		wg.Wait()

		if needProxy && errProxy != nil {
			return nil, fmt.Errorf("lỗi tạo proxy: %v", errProxy)
		}
		if needAudio && errAudio != nil {
			return nil, fmt.Errorf("lỗi tạo audio: %v", errAudio)
		}
	} else {
		runtime.EventsEmit(a.ctx, "analyze_log", "Bước 1/3: Bỏ qua tạo proxy & trích xuất âm thanh trong chế độ Tách nhanh.")
		runtime.EventsEmit(a.ctx, "analyze_progress", map[string]interface{}{"path": sourcePath, "progress": 15})
	}


	// 2. Chạy Python worker trên proxy (240p) và audio WAV (nếu có)
	runtime.EventsEmit(a.ctx, "analyze_log", fmt.Sprintf("Bước 2/3: Đang phân tích chuyển cảnh (scene=%.1f, minClip=%.0fs, maxClip=%.0fs)...", cfg.SceneThreshold, cfg.MinClipDuration, cfg.MaxClipDuration))
	runtime.EventsEmit(a.ctx, "analyze_progress", map[string]interface{}{"path": sourcePath, "progress": 15})
	
	passedProxyPath := ""
	if needProxy {
		passedProxyPath = proxyPath
	}
	passedAudioPath := ""
	if needAudio {
		passedAudioPath = audioPath
	}
	pythonExe := "python"
	venvPython := filepath.Join(filepath.Dir(utils.GetWorkerScript()), ".venv", "Scripts", "python.exe")
	if _, err := os.Stat(venvPython); err == nil {
		pythonExe = venvPython
	}

	clips, err := boundary.AnalyzeVideo(ctx, sourcePath, passedProxyPath, passedAudioPath, pythonExe, cfg, totalDuration)
	if err != nil {
		return nil, fmt.Errorf("lỗi phân tích: %v", err)
	}

	// An toàn: cập nhật EndTime clip cuối nếu analyzer chưa biết.
	if totalDuration > 0 && len(clips) > 0 && clips[len(clips)-1].EndTime <= 0 {
		clips[len(clips)-1].EndTime = totalDuration
		clips[len(clips)-1].Duration = totalDuration - clips[len(clips)-1].StartTime
	}

	// 3. Tạo thumbnails cho từng clip song song (hạn chế 8 luồng ffmpeg đồng thời để tránh làm nghẽn CPU)
	runtime.EventsEmit(a.ctx, "analyze_log", fmt.Sprintf("✓ Bước 2 hoàn tất: phát hiện %d phân đoạn video.", len(clips)))
	runtime.EventsEmit(a.ctx, "analyze_progress", map[string]interface{}{"path": sourcePath, "progress": 90})

	// Gửi TẤT CẢ clip cho frontend NGAY SAU khi phát hiện xong (chưa có ảnh xem trước)
	// → Clip hiện lên giao diện tức thì, ảnh sẽ được bổ sung dần ở Bước 3
	runtime.EventsEmit(a.ctx, "clips_detected", map[string]interface{}{
		"path":  sourcePath,
		"clips": clips,
	})

	runtime.EventsEmit(a.ctx, "analyze_progress", map[string]interface{}{"path": sourcePath, "progress": 92})
	runtime.EventsEmit(a.ctx, "analyze_log", fmt.Sprintf("Bước 3/3: Đang trích xuất %d ảnh xem trước (8 luồng song song)...", len(clips)))
	thumbDir := filepath.Join(workDir, "thumbnails")
	_ = os.MkdirAll(thumbDir, 0755)

	// Kiểm tra xem proxy có tồn tại thực sự hay không để dùng trích xuất thumbnail nhanh
	thumbInputPath := sourcePath
	if needProxy {
		if _, err := os.Stat(proxyPath); err == nil {
			thumbInputPath = proxyPath
		}
	}

	type pythonThumbJob struct {
		Index     int     `json:"index"`
		Timestamp float64 `json:"timestamp"`
		Path      string  `json:"path"`
		Type      string  `json:"type"`
	}

	var pythonJobs []pythonThumbJob
	for i := 0; i < len(clips); i++ {
		thumbPath := filepath.Join(thumbDir, fmt.Sprintf("thumb_%s_%d.jpg", clips[i].ID, i))
		pythonJobs = append(pythonJobs, pythonThumbJob{
			Index:     i,
			Timestamp: clips[i].StartTime,
			Path:      thumbPath,
			Type:      "start",
		})

		endThumbPath := filepath.Join(thumbDir, fmt.Sprintf("thumbend_%s_%d.jpg", clips[i].ID, i))
		endAt := clips[i].EndTime - 0.1
		if endAt < clips[i].StartTime {
			endAt = clips[i].StartTime
		}
		pythonJobs = append(pythonJobs, pythonThumbJob{
			Index:     i,
			Timestamp: endAt,
			Path:      endThumbPath,
			Type:      "end",
		})
	}

	jobsBytes, err := json.Marshal(pythonJobs)
	if err != nil {
		return nil, fmt.Errorf("lỗi serialize thumbnail jobs: %v", err)
	}

	scriptPath := filepath.Join(filepath.Dir(utils.GetWorkerScript()), "extract_thumbs.py")

	cmd := exec.CommandContext(ctx, pythonExe, scriptPath, "--video", thumbInputPath)
	cmd.Env = append(os.Environ(), "PYTHONUTF8=1", "PYTHONIOENCODING=utf-8")
	utils.HideCmdWindow(cmd)

	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("lỗi tạo stdin pipe cho thumbnail extractor: %v", err)
	}

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("lỗi tạo stdout pipe cho thumbnail extractor: %v", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("lỗi chạy thumbnail extractor: %v", err)
	}

	go func() {
		defer stdinPipe.Close()
		_, _ = stdinPipe.Write(jobsBytes)
	}()

	clipDone := make(map[int]int)
	var completedJobs int
	numJobs := len(pythonJobs)
	var mu sync.Mutex

	scanner := bufio.NewScanner(stdoutPipe)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "DONE:") {
			parts := strings.Split(strings.TrimPrefix(line, "DONE:"), "|")
			if len(parts) == 3 {
				idx, _ := strconv.Atoi(parts[0])
				jobType := parts[1]
				path := parts[2]

				mu.Lock()
				if jobType == "start" {
					clips[idx].Thumbnail = path
				} else {
					clips[idx].ThumbEnd = path
				}

				completedJobs++
				prog := 92 + (completedJobs * 7 / numJobs)
				if prog > 99 {
					prog = 99
				}
				runtime.EventsEmit(a.ctx, "analyze_progress", map[string]interface{}{"path": sourcePath, "progress": prog})

				clipDone[idx]++
				if clipDone[idx] >= 2 {
					doneClips := 0
					for _, v := range clipDone {
						if v >= 2 {
							doneClips++
						}
					}
					totalClips := len(clips)
					runtime.EventsEmit(a.ctx, "analyze_log", fmt.Sprintf("  Bước 3/3: Đã trích xuất ảnh clip %d/%d", doneClips, totalClips))
					runtime.EventsEmit(a.ctx, "clip_thumb_update", map[string]interface{}{
						"path":      sourcePath,
						"clipId":    clips[idx].ID,
						"thumbnail": clips[idx].Thumbnail,
						"thumbEnd":  clips[idx].ThumbEnd,
					})
				}
				mu.Unlock()
			}
		}
	}

	if err := cmd.Wait(); err != nil {
		runtime.EventsEmit(a.ctx, "analyze_log", "Cảnh báo: Lỗi batch thumbnail. Đang chạy chế độ fallback bằng ffmpeg...")
		var wgThumbs sync.WaitGroup
		sem := make(chan struct{}, 8)
		var completed int
		var mu2 sync.Mutex

		for i := 0; i < len(clips); i++ {
			wgThumbs.Add(1)
			go func(idx int) {
				defer wgThumbs.Done()
				sem <- struct{}{}
				defer func() { <-sem }()

				thumbPath := filepath.Join(thumbDir, fmt.Sprintf("thumb_%s_%d.jpg", clips[idx].ID, idx))
				_ = media.ExtractFrame(ctx, thumbInputPath, clips[idx].StartTime, thumbPath)

				mu2.Lock()
				clips[idx].Thumbnail = thumbPath
				completed++
				prog := 92 + (completed * 7 / (len(clips) * 2))
				if prog > 99 {
					prog = 99
				}
				runtime.EventsEmit(a.ctx, "analyze_progress", map[string]interface{}{"path": sourcePath, "progress": prog})
				mu2.Unlock()
			}(i)

			wgThumbs.Add(1)
			go func(idx int) {
				defer wgThumbs.Done()
				sem <- struct{}{}
				defer func() { <-sem }()

				endThumbPath := filepath.Join(thumbDir, fmt.Sprintf("thumbend_%s_%d.jpg", clips[idx].ID, idx))
				endAt := clips[idx].EndTime - 0.1
				if endAt < clips[idx].StartTime {
					endAt = clips[idx].StartTime
				}
				_ = media.ExtractFrame(ctx, thumbInputPath, endAt, endThumbPath)

				mu2.Lock()
				clips[idx].ThumbEnd = endThumbPath
				completed++
				prog := 92 + (completed * 7 / (len(clips) * 2))
				if prog > 99 {
					prog = 99
				}
				runtime.EventsEmit(a.ctx, "analyze_progress", map[string]interface{}{"path": sourcePath, "progress": prog})
				mu2.Unlock()
			}(i)
		}
		wgThumbs.Wait()

		for idx := range clips {
			runtime.EventsEmit(a.ctx, "clip_thumb_update", map[string]interface{}{
				"path":      sourcePath,
				"clipId":    clips[idx].ID,
				"thumbnail": clips[idx].Thumbnail,
				"thumbEnd":  clips[idx].ThumbEnd,
			})
		}
	}

	runtime.EventsEmit(a.ctx, "analyze_log", fmt.Sprintf("Phân tích hoàn tất! Đã tìm thấy %d phân đoạn.", len(clips)))
	return clips, nil
}

// CancelAnalysis dừng tiến trình phân tích hiện tại bằng cách hủy Context
func (a *App) CancelAnalysis() {
	a.cancelMu.Lock()
	defer a.cancelMu.Unlock()
	for _, cancel := range a.cancelFuncs {
		if cancel != nil {
			cancel()
		}
	}
}

// GenerateThumbnail trích một khung hình tại timeSec của video nguồn và trả về
// đường dẫn ảnh. Dùng khi frontend chia/sửa clip và cần ảnh xem trước mới.
// Ảnh lưu trong workDir theo video (giữ lại để UI hiển thị).
func (a *App) GenerateThumbnail(sourcePath string, timeSec float64) (string, error) {
	workDir := filepath.Join(os.TempDir(), "video-splitter", hashPath(sourcePath), "thumbnails")
	if err := os.MkdirAll(workDir, 0755); err != nil {
		return "", err
	}
	// Tên theo mốc thời gian (mili-giây) để mỗi vị trí có ảnh riêng, tránh ghi đè.
	thumbPath := filepath.Join(workDir, fmt.Sprintf("edit_%d.jpg", int64(timeSec*1000)))
	if err := media.ExtractFrame(a.ctx, sourcePath, timeSec, thumbPath); err != nil {
		return "", err
	}
	return thumbPath, nil
}

// ExportResult tóm tắt kết quả xuất một clip cho frontend.
type ExportResult struct {
	ClipID   string  `json:"clipId"`
	Index    int     `json:"index"`
	OK       bool    `json:"ok"`
	OutPath  string  `json:"outPath"`
	Duration float64 `json:"duration"` // thời lượng thực đo bằng ffprobe
	Error    string  `json:"error"`
}

// ExportClips cắt danh sách clip, chạy song song có giới hạn số job, mỗi clip được
// verify bằng ffprobe (thời lượng khớp trong dung sai ~0.5s). Trả về kết quả từng clip.
// jobs<=0 → mặc định 2 (tránh chạy vô hạn tiến trình ffmpeg như spec yêu cầu).
// CancelExport dừng tiến trình xuất video hiện tại và giết ffmpeg
func (a *App) CancelExport() {
	a.exportCancelMu.Lock()
	a.isExportCancelled = true
	for _, cancel := range a.exportCancelFuncs {
		cancel()
	}
	a.exportCancelFuncs = make(map[string]context.CancelFunc)
	a.exportCancelMu.Unlock()
	runtime.EventsEmit(a.ctx, "export_log", "Đã dừng tiến trình xuất video!")
}

func sanitizeFilename(s string) string {
	r := strings.NewReplacer(
		`\`, "_",
		`/`, "_",
		`:`, "_",
		`*`, "_",
		`?`, "_",
		`"`, "_",
		`<`, "_",
		`>`, "_",
		`|`, "_",
		` `, "_",
	)
	return r.Replace(s)
}

// ExportClips xuất nhiều clip song song sử dụng ffmpeg.
// Tên video đầu ra được đặt theo định dạng: [Tên dự án]_[Tên video gốc]_[Số thứ tự clip].mp4
func (a *App) ExportClips(projectName string, sourcePath string, clips []project.Clip, outDir string, outImageDir string, cfg project.AnalyzerConfig, jobs int) ([]ExportResult, error) {
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return nil, fmt.Errorf("không tạo được thư mục xuất: %v", err)
	}
	if jobs <= 0 {
		jobs = 2
	}
	if jobs > 8 {
		jobs = 8
	}

	// Phân giải GPU tự động khi xuất
	if cfg.HardwareAccel == "auto" || cfg.HardwareAccel == "" {
		cfg.HardwareAccel = media.DetectGPU()
	}

	a.exportCancelMu.Lock()
	a.isExportCancelled = false
	a.exportCancelFuncs = make(map[string]context.CancelFunc)
	a.exportCancelMu.Unlock()

	results := make([]ExportResult, len(clips))
	var enqueuedCount int
	sem := make(chan struct{}, jobs)
	var wg sync.WaitGroup
	var done int
	var mu sync.Mutex

	for i, clip := range clips {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, clip project.Clip) {
			defer wg.Done()
			defer func() { <-sem }()

			a.exportCancelMu.Lock()
			if a.isExportCancelled {
				a.exportCancelMu.Unlock()
				res := ExportResult{ClipID: clip.ID, Index: clip.Index, Error: "Tiến trình xuất bị dừng"}
				results[i] = res
				return
			}
			clipCtx, cancel := context.WithCancel(a.ctx)
			a.exportCancelFuncs[clip.ID] = cancel
			a.exportCancelMu.Unlock()

			defer func() {
				a.exportCancelMu.Lock()
				delete(a.exportCancelFuncs, clip.ID)
				a.exportCancelMu.Unlock()
			}()

			videoName := filepath.Base(sourcePath)
			videoName = strings.TrimSuffix(videoName, filepath.Ext(videoName))

			cleanProjName := sanitizeFilename(projectName)
			cleanVideoName := sanitizeFilename(videoName)

			outName := fmt.Sprintf("%s_%s_%d.mp4", cleanProjName, cleanVideoName, clip.Index)
			outPath := filepath.Join(outDir, outName)
			res := ExportResult{ClipID: clip.ID, Index: clip.Index, OutPath: outPath}
			threads := 0
			if jobs > 1 {
				threads = 2 // Giới hạn 2 threads mỗi clip để chạy song song mượt mà
			}
			runtime.EventsEmit(a.ctx, "export_log", fmt.Sprintf("Đang xử lý Video: %s (Clip #%d): Đang cắt video...", filepath.Base(sourcePath), clip.Index))
			err := exporter.CutVideo(clipCtx, sourcePath, clip, outPath, cfg.ExportPreset, cfg.ExportCRF, threads, cfg.HardwareAccel, cfg.Mode)
			if err != nil {
				if clipCtx.Err() != nil {
					res.Error = "Tiến trình bị dừng"
				} else {
					res.Error = err.Error()
				}
			} else if dur, verr := exporter.VerifyOutput(outPath, clip.EndTime-clip.StartTime, 0.5); verr != nil {
				if dur <= 0 {
					if clipCtx.Err() != nil {
						res.Error = "Tiến trình bị dừng"
					} else {
						res.Error = verr.Error()
					}
				} else {
					res.OK = true
					res.Duration = dur
					res.Error = "cảnh báo: " + verr.Error()
				}
			} else {
				res.OK = true
				res.Duration = dur
			}

			// Copy hoặc tự sinh thumbnail sang outImageDir nếu có cấu hình và video xuất thành công
			if res.OK && outImageDir != "" {
				destThumbName := fmt.Sprintf("%s_%s_%d.jpg", cleanProjName, cleanVideoName, clip.Index)
				destThumbPath := filepath.Join(outImageDir, destThumbName)
				_ = os.MkdirAll(outImageDir, 0755)

				// Nếu clip đã có sẵn ảnh bìa AI chỉnh tay (có chữ _ai trong tên file), chỉ cần copy sang
				if clip.Thumbnail != "" && strings.Contains(clip.Thumbnail, "_ai") {
					if _, errStat := os.Stat(clip.Thumbnail); errStat == nil {
						_ = copyFile(clip.Thumbnail, destThumbPath)
					}
				} else {
					// Trích xuất 1 khung hình rõ nét (tự động lọc bỏ khung hình bị đen/tối)
					clipDuration := clip.EndTime - clip.StartTime
					clearFramePath := filepath.Join(os.TempDir(), "video-splitter", fmt.Sprintf("clearframe_%s_%d.jpg", clip.ID, clip.Index))
					extractedFrame, errFrame := media.ExtractClearFrame(clipCtx, sourcePath, clipDuration, clearFramePath)

					if errFrame == nil && extractedFrame != "" {
						// Tạo ảnh thumbnail mặc định ban đầu từ frame cắt được
						_ = copyFile(extractedFrame, destThumbPath)

						theme := cfg.Prompt
						if theme == "" {
							theme = "Tạo ảnh thumbnail đẹp, ấn tượng và thu hút cho video ngắn"
						}

						aspectRatio := "9:16"
						if clip.Edit.Aspect.Enabled && clip.Edit.Aspect.Ratio != "" {
							aspectRatio = clip.Edit.Aspect.Ratio
						} else {
							if vi, errVi := media.GetVideoInfo(sourcePath); errVi == nil && vi != nil {
								if vi.Width > vi.Height {
									aspectRatio = "16:9"
								} else {
									aspectRatio = "9:16"
								}
							}
						}

						task := browserai.ThumbnailTask{
							ID:             fmt.Sprintf("task_thumb_%s_%d", clip.ID, clip.Index),
							ClipName:       fmt.Sprintf("Clip #%d (%s)", clip.Index, cleanVideoName),
							ClipPath:       outPath,
							OutputDir:      outImageDir,
							FileName:       strings.TrimSuffix(destThumbName, ".jpg"),
							Prompt:         theme,
							InputImagePath: extractedFrame,
							Provider:       browserai.ProviderFlow,
							Model:          "Nano Banana 2",
							AspectRatio:    aspectRatio,
						}

						// Nạp task vào Hàng Đợi AI NGAY khi clip này cắt xong (vừa xuất
						// vừa gửi) thay vì đợi cắt hết mọi clip. Enqueue an toàn khi hàng
						// đợi đang chạy — worker sẽ tự nhặt task mới.
						if a.browserAIService != nil {
							a.browserAIService.EnqueueThumbnailTasks([]browserai.ThumbnailTask{task})
							mu.Lock()
							enqueuedCount++
							mu.Unlock()
						}
					} else if clip.Thumbnail != "" {
						if _, errStat := os.Stat(clip.Thumbnail); errStat == nil {
							_ = copyFile(clip.Thumbnail, destThumbPath)
						}
					}
				}
			}

			results[i] = res

			mu.Lock()
			done++
			runtime.EventsEmit(a.ctx, "export_progress", map[string]any{
				"done": done, "total": len(clips), "clipId": clip.ID, "ok": res.OK, "outPath": res.OutPath,
			})
			statusW := statusWord(res.OK)
			if clipCtx.Err() != nil {
				statusW = "ĐÃ DỪNG"
			}
			runtime.EventsEmit(a.ctx, "export_log", fmt.Sprintf("Đang xử lý Video: %s (Clip #%d): Hoàn thành! (%s)", filepath.Base(sourcePath), clip.Index, statusW))
			mu.Unlock()
		}(i, clip)
	}
	wg.Wait()

	if enqueuedCount > 0 {
		runtime.EventsEmit(a.ctx, "export_log", fmt.Sprintf("🔥 Đã nạp %d clip vào Hàng Đợi AI tự động sinh Thumbnail trên Google Flow!", enqueuedCount))
	}

	okCount := 0
	for _, r := range results {
		if r.OK {
			okCount++
		}
	}
	if a.store != nil {
		_ = a.store.AddExportRecord(hashPath(sourcePath), outDir, len(clips), okCount)
	}
	runtime.EventsEmit(a.ctx, "export_log", fmt.Sprintf("Hoàn tất: %d/%d clip thành công.", okCount, len(clips)))
	return results, nil
}

func statusWord(ok bool) string {
	if ok {
		return "OK"
	}
	return "LỖI"
}

// MergeClips cắt từng clip (áp edit) rồi ghép thành MỘT video, dùng transition
// của clip đầu tiên có cấu hình (đồng nhất cho toàn bộ mối nối). Trả về đường dẫn file ghép.
// Clip trung gian cắt vào thư mục tạm, dọn sau khi ghép xong.
func (a *App) MergeClips(sourcePath string, clips []project.Clip, outPath string, cfg project.AnalyzerConfig) (string, error) {
	if len(clips) == 0 {
		return "", fmt.Errorf("không có clip để ghép")
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
		return "", fmt.Errorf("không tạo được thư mục xuất: %v", err)
	}

	tmpDir := filepath.Join(os.TempDir(), "video-splitter", hashPath(sourcePath), "merge")
	_ = os.MkdirAll(tmpDir, 0755)
	defer os.RemoveAll(tmpDir)

	var parts []string
	for i, clip := range clips {
		runtime.EventsEmit(a.ctx, "export_log", fmt.Sprintf("Ghép: đang chuẩn bị phân đoạn %d/%d...", i+1, len(clips)))
		p := filepath.Join(tmpDir, fmt.Sprintf("part_%03d.mp4", i))
		if err := exporter.CutVideo(context.Background(), sourcePath, clip, p, cfg.ExportPreset, cfg.ExportCRF, 0, cfg.HardwareAccel, cfg.Mode); err != nil {
			return "", fmt.Errorf("lỗi chuẩn bị clip #%d: %v", clip.Index, err)
		}
		parts = append(parts, p)
	}

	// Transition: lấy từ clip đầu tiên có cấu hình transition (áp đồng nhất mọi mối nối).
	transType, transDur := "", 0.0
	for _, c := range clips {
		if c.Edit.Transition.Type != "" && c.Edit.Transition.Duration > 0 {
			transType = c.Edit.Transition.Type
			transDur = c.Edit.Transition.Duration
			break
		}
	}

	runtime.EventsEmit(a.ctx, "export_log", "Ghép: đang nối các phân đoạn thành video hoàn chỉnh...")
	if err := exporter.ConcatClips(parts, outPath, transType, transDur, cfg.ExportPreset, cfg.ExportCRF, cfg.HardwareAccel); err != nil {
		return "", err
	}
	if _, err := exporter.VerifyOutput(outPath, 0, 0); err != nil {
		return "", fmt.Errorf("video ghép không hợp lệ: %v", err)
	}
	runtime.EventsEmit(a.ctx, "export_log", "Đã ghép xong video hoàn chỉnh!")
	return outPath, nil
}

// SelectImageFile mở hộp thoại chọn ảnh (watermark/logo).
func (a *App) SelectImageFile() (string, error) {
	return runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Chọn ảnh watermark / logo",
		Filters: []runtime.FileFilter{
			{DisplayName: "Ảnh (*.png;*.jpg;*.jpeg;*.webp)", Pattern: "*.png;*.jpg;*.jpeg;*.webp"},
		},
	})
}

// SelectImageFiles mở hộp thoại chọn nhiều ảnh đầu vào.
func (a *App) SelectImageFiles() ([]string, error) {
	return runtime.OpenMultipleFilesDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Chọn ảnh làm đầu vào",
		Filters: []runtime.FileFilter{
			{DisplayName: "Ảnh (*.png;*.jpg;*.jpeg;*.webp)", Pattern: "*.png;*.jpg;*.jpeg;*.webp"},
		},
	})
}

// SelectAudioFile mở hộp thoại chọn file nhạc nền.
func (a *App) SelectAudioFile() (string, error) {
	return runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Chọn nhạc nền",
		Filters: []runtime.FileFilter{
			{DisplayName: "Âm thanh (*.mp3;*.wav;*.aac;*.m4a)", Pattern: "*.mp3;*.wav;*.aac;*.m4a"},
		},
	})
}

// === PERSISTENCE (SQLite) ===

// SaveProject lưu (hoặc cập nhật) một phiên làm việc. ID dựa trên đường dẫn nguồn
// để mỗi video map tới một project ổn định. Trả về project đã lưu (kèm ID).
func (a *App) SaveProject(sourcePath string, clips []project.Clip, cfg project.AnalyzerConfig) (*project.Project, error) {
	if a.store == nil {
		return nil, fmt.Errorf("kho lưu trữ chưa sẵn sàng")
	}

	// Tái dùng project cũ (giữ CreatedAt) nếu đã có cho video này.
	existing, _ := a.store.FindProjectBySource(sourcePath)

	p := &project.Project{
		ID:         hashPath(sourcePath),
		SourcePath: sourcePath,
		Name:       filepath.Base(sourcePath),
		Status:     "analyzed",
		Config:     cfg,
		Clips:      clips,
	}
	if info, _ := media.GetVideoInfo(sourcePath); info != nil {
		p.Duration = info.Duration
		p.Width = info.Width
		p.Height = info.Height
		p.FPS = info.FPS
	}
	if existing != nil {
		p.CreatedAt = existing.CreatedAt
	}

	if err := a.store.SaveProject(p); err != nil {
		return nil, err
	}
	return p, nil
}

// LoadProjectBySource nạp lại project theo đường dẫn video. (nil, nil) nếu chưa có.
func (a *App) LoadProjectBySource(sourcePath string) (*project.Project, error) {
	if a.store == nil {
		return nil, fmt.Errorf("kho lưu trữ chưa sẵn sàng")
	}
	return a.store.FindProjectBySource(sourcePath)
}

// ListProjects trả về danh sách tóm tắt các phiên làm việc đã lưu.
func (a *App) ListProjects() ([]storage.ProjectSummary, error) {
	if a.store == nil {
		return nil, fmt.Errorf("kho lưu trữ chưa sẵn sàng")
	}
	return a.store.ListProjects()
}

// DeleteProject xóa một project theo ID.
func (a *App) DeleteProject(id string) error {
	if a.store == nil {
		return fmt.Errorf("kho lưu trữ chưa sẵn sàng")
	}
	return a.store.DeleteProject(id)
}

func (a *App) analyzeFixed(sourcePath string, cfg project.AnalyzerConfig) ([]project.Clip, error) {
	key := hashPath(sourcePath)
	workDir := filepath.Join(os.TempDir(), "video-splitter", key)
	_ = os.MkdirAll(workDir, 0755)

	ctx, cancel := context.WithCancel(a.ctx)
	a.cancelMu.Lock()
	a.cancelFuncs[sourcePath] = cancel
	a.cancelMu.Unlock()
	defer func() {
		a.cancelMu.Lock()
		delete(a.cancelFuncs, sourcePath)
		a.cancelMu.Unlock()
	}()

	info, err := media.GetVideoInfo(sourcePath)
	if err != nil {
		return nil, fmt.Errorf("không đọc được thông tin video: %v", err)
	}
	totalDuration := info.Duration

	fixedLen := cfg.MaxClipDuration
	if fixedLen <= 0 {
		fixedLen = 30.0
	}

	var clips []project.Clip
	startTime := 0.0
	index := 1
	for startTime < totalDuration {
		endTime := startTime + fixedLen
		if endTime > totalDuration {
			endTime = totalDuration
		}

		// Gộp đoạn cuối dư thừa nếu quá ngắn (dưới 50% thời lượng đích)
		remainder := totalDuration - startTime
		if len(clips) > 0 && remainder < fixedLen*0.5 {
			clips[len(clips)-1].EndTime = math.Round(totalDuration)
			clips[len(clips)-1].Duration = clips[len(clips)-1].EndTime - clips[len(clips)-1].StartTime
			break
		}

		clipID := fmt.Sprintf("%s_%d", key, index)
		clip := project.Clip{
			ID:         clipID,
			Index:      index,
			StartTime:  math.Round(startTime),
			EndTime:    math.Round(endTime),
			Duration:   math.Round(endTime) - math.Round(startTime),
			Status:     "pending",
			Tier:       project.TierAuto,
			Reason:     "Cắt đều theo thời lượng",
			Edit:       project.DefaultEditOps(),
		}
		clips = append(clips, clip)

		startTime = endTime
		index++
	}

	runtime.EventsEmit(a.ctx, "analyze_log", fmt.Sprintf("Bước 1/2: Đã phân chia %d đoạn đều nhau %.0f giây...", len(clips), fixedLen))
	runtime.EventsEmit(a.ctx, "analyze_progress", map[string]interface{}{"path": sourcePath, "progress": 30})

	thumbDir := filepath.Join(workDir, "thumbnails")
	_ = os.MkdirAll(thumbDir, 0755)

	for i := 0; i < len(clips); i++ {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		runtime.EventsEmit(a.ctx, "analyze_log", fmt.Sprintf("Bước 2/2: Đang trích xuất ảnh thumbnail %d/%d (Clip #%d)...", i+1, len(clips), clips[i].Index))
		
		thumbPath := filepath.Join(thumbDir, fmt.Sprintf("thumb_%s_%d.jpg", clips[i].ID, i))
		_ = media.ExtractFrame(ctx, sourcePath, clips[i].StartTime, thumbPath)
		clips[i].Thumbnail = thumbPath

		if clips[i].EndTime > clips[i].StartTime {
			endThumbPath := filepath.Join(thumbDir, fmt.Sprintf("thumbend_%s_%d.jpg", clips[i].ID, i))
			endAt := clips[i].EndTime - 0.1
			if endAt < clips[i].StartTime {
				endAt = clips[i].StartTime
			}
			_ = media.ExtractFrame(ctx, sourcePath, endAt, endThumbPath)
			clips[i].ThumbEnd = endThumbPath
		}

		prog := 30 + (i+1)*70/len(clips)
		runtime.EventsEmit(a.ctx, "analyze_progress", map[string]interface{}{"path": sourcePath, "progress": prog})
	}

	runtime.EventsEmit(a.ctx, "analyze_log", fmt.Sprintf("Cắt đều hoàn tất! Đã tạo %d phân đoạn.", len(clips)))
	return clips, nil
}

// SaveGlobalSettings lưu cấu hình cài đặt chung của người dùng vào file settings.json
func (a *App) SaveGlobalSettings(settingsJSON string) error {
	dir, err := os.UserConfigDir()
	if err != nil || dir == "" {
		dir = os.TempDir()
	}
	appDir := filepath.Join(dir, "video-splitter")
	_ = os.MkdirAll(appDir, 0755)

	settingsPath := filepath.Join(appDir, "settings.json")
	return os.WriteFile(settingsPath, []byte(settingsJSON), 0644)
}

// GetGlobalSettings đọc cấu hình cài đặt chung của người dùng từ file settings.json.
// Trả về chuỗi rỗng nếu file chưa tồn tại.
func (a *App) GetGlobalSettings() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil || dir == "" {
		dir = os.TempDir()
	}
	settingsPath := filepath.Join(dir, "video-splitter", "settings.json")
	if _, err := os.Stat(settingsPath); os.IsNotExist(err) {
		return "", nil
	}
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// ExtractClipFrames trích xuất 3 khung hình từ video gốc làm ảnh tham chiếu để tạo thumbnail AI
func (a *App) ExtractClipFrames(videoPath string, startTime float64, endTime float64) ([]string, error) {
	dir, err := os.UserConfigDir()
	if err != nil || dir == "" {
		dir = os.TempDir()
	}
	tempDir := filepath.Join(dir, "video-splitter", "temp_frames")
	_ = os.MkdirAll(tempDir, 0755)

	// Dọn dẹp các frame cũ
	files, _ := os.ReadDir(tempDir)
	for _, f := range files {
		_ = os.Remove(filepath.Join(tempDir, f.Name()))
	}

	duration := endTime - startTime
	if duration <= 0 {
		duration = 10.0
	}

	timestamps := []float64{
		startTime + duration*0.25,
		startTime + duration*0.5,
		startTime + duration*0.75,
	}

	var framePaths []string
	for i, ts := range timestamps {
		outputPath := filepath.Join(tempDir, fmt.Sprintf("frame_%d_%d.jpg", i+1, time.Now().UnixNano()))
		err := media.ExtractFrame(a.ctx, videoPath, ts, outputPath)
		if err != nil {
			continue
		}
		framePaths = append(framePaths, outputPath)
	}

	return framePaths, nil
}

// GenerateAIThumbnail sử dụng Gemini 1.5 Flash để tối ưu hóa prompt từ ảnh mẫu, sau đó gọi Imagen 4 để tạo ảnh thumbnail
func (a *App) GenerateAIThumbnail(apiKey string, userPrompt string, imagePaths []string, videoPath string, clipIndex int, aspectRatio string, editConfigJSON string) (string, error) {
	if apiKey == "" {
		return "", fmt.Errorf("vui lòng cung cấp Gemini API Key trong phần Cài đặt chung")
	}

	if aspectRatio == "" {
		aspectRatio = "9:16"
	}

	// Phân tích các thông tin hiệu ứng chỉnh sửa của clip để hướng dẫn Gemini vẽ ảnh đồng bộ
	var editOps project.EditOps
	remixDetails := ""
	if editConfigJSON != "" {
		if err := json.Unmarshal([]byte(editConfigJSON), &editOps); err == nil {
			var details []string
			if editOps.HFlip {
				details = append(details, "HORIZONTALLY MIRRORED/FLIPPED (Video bị lật ngược chiều ngang)")
			}
			if editOps.Aspect.Enabled {
				details = append(details, fmt.Sprintf("Aspect ratio: %s, mode: %s", editOps.Aspect.Ratio, editOps.Aspect.Mode))
			}
			if editOps.Color.Enabled {
				colorPreset := editOps.Color.Preset
				if colorPreset == "" {
					colorPreset = "custom settings"
				}
				details = append(details, fmt.Sprintf("Color preset/filter applied: %s (brightness: %.2f, contrast: %.2f, saturation: %.2f)", colorPreset, editOps.Color.Brightness, editOps.Color.Contrast, editOps.Color.Saturation))
			}
			if len(details) > 0 {
				remixDetails = "\nApplied Video Remix / Anti-Copyright Effects (Make sure the generated image matches these visual changes):\n- " + strings.Join(details, "\n- ")
			}
		}
	}

	// 1. Chuẩn bị ảnh base64 gửi cho Gemini làm tài liệu tham khảo phong cách hình ảnh
	const maxReferenceFrames = 3
	const maxRawImageBytes = 12 * 1024 * 1024

	var imageParts []map[string]interface{}
	totalImageBytes := 0

	for _, ip := range imagePaths {
		if len(imageParts) >= maxReferenceFrames {
			break
		}

		data, err := os.ReadFile(ip)
		if err != nil {
			continue
		}

		mimeType := http.DetectContentType(data)
		switch mimeType {
		case "image/jpeg", "image/png", "image/webp":
		default:
			continue
		}

		if totalImageBytes+len(data) > maxRawImageBytes {
			break
		}
		totalImageBytes += len(data)

		imageParts = append(imageParts, map[string]interface{}{
			"inlineData": map[string]string{
				"mimeType": mimeType,
				"data":     base64.StdEncoding.EncodeToString(data),
			},
		})
	}

	// 2. Gọi Gemini để phân tích phong cách các frames và viết prompt chi tiết cho Imagen
	geminiPrompt := fmt.Sprintf(
		`You are a professional YouTube, TikTok, Shorts, and Reels thumbnail designer.

Analyze the provided reference frames and the user's requested video theme.

Create one detailed English image-generation prompt for a premium thumbnail with a %s aspect ratio.

Use one clear primary subject, a strong mobile-friendly focal point, expressive emotion, cinematic lighting, high contrast, rich colors, clean depth, and clear separation between the subject and background.

Match the characters, clothing, environment, camera angle, color palette, lighting, and mood visible in the reference frames without directly copying a frame.

Do not include text, letters, numbers, captions, logos, UI elements, borders, watermarks, collages, or split-screen compositions.

Keep the final prompt under 300 English words and under 450 tokens.

Return only the final English image prompt without headings, explanations, markdown, quotation marks, or backticks.`,
		aspectRatio,
	)

	geminiPayload := map[string]interface{}{
		"contents": []map[string]interface{}{
			{
				"parts": append([]map[string]interface{}{
					{"text": geminiPrompt},
					{"text": fmt.Sprintf("User Video Theme: %s%s", userPrompt, remixDetails)},
				}, imageParts...),
			},
		},
		"generationConfig": map[string]interface{}{
			"temperature":     0.7,
			"maxOutputTokens": 450,
		},
	}

	geminiPayloadBytes, err := json.Marshal(geminiPayload)
	if err != nil {
		return "", fmt.Errorf("lỗi tạo request payload cho Gemini: %v", err)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	apiVersions := []string{"v1beta"}
	modelsToTry := []string{
		"gemini-3.5-flash",
		"gemini-2.5-flash",
		"gemini-2.0-flash",
		"gemini-2.0-flash-lite",
		"gemini-1.5-flash",
	}
	var optimizedPrompt string
	var lastErr error

	success := false
	for _, version := range apiVersions {
		for _, modelName := range modelsToTry {
			geminiURL := fmt.Sprintf("https://generativelanguage.googleapis.com/%s/models/%s:generateContent", version, modelName)
			req, err := http.NewRequest("POST", geminiURL, bytes.NewBuffer(geminiPayloadBytes))
			if err != nil {
				lastErr = err
				continue
			}
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("x-goog-api-key", apiKey)

			resp, err := client.Do(req)
			if err != nil {
				lastErr = err
				continue
			}

			if resp.StatusCode != http.StatusOK {
				bodyBytes, _ := io.ReadAll(resp.Body)
				resp.Body.Close()
				lastErr = fmt.Errorf("model %s (%s) trả về lỗi %d: %s", modelName, version, resp.StatusCode, string(bodyBytes))
				continue
			}

			var geminiResponse struct {
				Candidates []struct {
					Content struct {
						Parts []struct {
							Text string `json:"text"`
						} `json:"parts"`
					} `json:"content"`
				} `json:"candidates"`
			}

			if err := json.NewDecoder(resp.Body).Decode(&geminiResponse); err != nil {
				resp.Body.Close()
				lastErr = err
				continue
			}
			resp.Body.Close()

			if len(geminiResponse.Candidates) > 0 && len(geminiResponse.Candidates[0].Content.Parts) > 0 {
				optimizedPrompt = strings.TrimSpace(geminiResponse.Candidates[0].Content.Parts[0].Text)
				if optimizedPrompt != "" {
					lastErr = nil
					success = true
					break
				}
			}
		}
		if success {
			break
		}
	}

	if optimizedPrompt == "" {
		return "", fmt.Errorf("không thể tối ưu hóa prompt bằng Gemini (lỗi cuối cùng: %v)", lastErr)
	}

	// 3. Sinh ảnh bằng Imagen 4 Ultra (hoặc Imagen 3 dự phòng) dùng official SDK
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	imageClient, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  apiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return "", fmt.Errorf("không thể khởi tạo Gemini client: %w", err)
	}

	var imgData []byte
	var imagenErr error

	// Try the new Gemini Image models (such as gemini-3-pro-image and gemini-3.1-flash-image) first
	geminiImageModels := []string{"gemini-3-pro-image", "gemini-3.1-flash-image"}
	for _, imgModel := range geminiImageModels {
		config := &genai.GenerateContentConfig{
			ResponseModalities: []string{"IMAGE"},
		}

		res, errGen := imageClient.Models.GenerateContent(ctx, imgModel, genai.Text(optimizedPrompt), config)
		if errGen != nil {
			imagenErr = errGen
			continue
		}

		if len(res.Candidates) > 0 && res.Candidates[0].Content != nil {
			for _, part := range res.Candidates[0].Content.Parts {
				if part.InlineData != nil && len(part.InlineData.Data) > 0 {
					imgData = part.InlineData.Data
					imagenErr = nil
					break
				}
			}
		}
		if len(imgData) > 0 {
			break
		}
	}

	// Fallback to legacy Imagen models if Gemini image models failed
	if len(imgData) == 0 {
		imagenModels := []string{"imagen-4.0-generate-001", "imagen-3.0-generate-002"}
		for _, imgModel := range imagenModels {
			config := &genai.GenerateImagesConfig{
				NumberOfImages:   1,
				AspectRatio:      aspectRatio,
				OutputMIMEType:   "image/jpeg",
				PersonGeneration: genai.PersonGenerationAllowAdult,
			}
			if strings.Contains(imgModel, "imagen-4.0") {
				config.ImageSize = "2K"
			}

			imageResponse, errGen := imageClient.Models.GenerateImages(ctx, imgModel, optimizedPrompt, config)
			if errGen != nil {
				imagenErr = errGen
				continue
			}

			if len(imageResponse.GeneratedImages) > 0 &&
				imageResponse.GeneratedImages[0] != nil &&
				imageResponse.GeneratedImages[0].Image != nil &&
				len(imageResponse.GeneratedImages[0].Image.ImageBytes) > 0 {
				imgData = imageResponse.GeneratedImages[0].Image.ImageBytes
				imagenErr = nil
				break
			}
		}
	}

	if len(imgData) == 0 {
		if imagenErr != nil {
			return "", fmt.Errorf("lỗi Imagen: %w", imagenErr)
		}
		return "", fmt.Errorf("Imagen không sinh ra ảnh hoặc ảnh bị bộ lọc an toàn chặn")
	}

	// 4. Ghi đè vào thư mục ai_thumbnails trong workspace dự án
	h := hashPath(videoPath)
	dirUser, err := os.UserConfigDir()
	if err != nil || dirUser == "" {
		dirUser = os.TempDir()
	}
	workDir := filepath.Join(dirUser, "video-splitter", "projects", h)
	aiThumbDir := filepath.Join(workDir, "ai_thumbnails")
	_ = os.MkdirAll(aiThumbDir, 0755)

	destPath := filepath.Join(aiThumbDir, fmt.Sprintf("clip_%d_ai.jpg", clipIndex))
	err = os.WriteFile(destPath, imgData, 0644)
	if err != nil {
		return "", fmt.Errorf("lỗi ghi file thumbnail AI: %v", err)
	}

	return destPath, nil
}

// ═══════════════════════════════════════════════════════════════════════════════
// ONLINE VIDEO DOWNLOADER (yt-dlp integration)
// ═══════════════════════════════════════════════════════════════════════════════

// ProbeOnlineURL dò link URL: trả về danh sách video (1 nếu video đơn, N nếu profile/playlist).
// cookieBrowser: "chrome" | "edge" | "firefox" | "" (không dùng cookie)
// maxCount: giới hạn số video lấy (0 = không giới hạn)
// sortOrder: "newest" (mặc định) | "oldest"
func (a *App) ProbeOnlineURL(rawURL string, cookieBrowser string, maxCount int, sortOrder string, searchSource string) (*downloader.URLProbeResult, error) {
	if maxCount > 0 {
		runtime.EventsEmit(a.ctx, "download_log", fmt.Sprintf("Đang dò link: %s (tối đa %d video, %s, nguồn: %s) ...", rawURL, maxCount, sortOrder, searchSource))
	} else {
		runtime.EventsEmit(a.ctx, "download_log", fmt.Sprintf("Đang dò link: %s (nguồn: %s) ...", rawURL, searchSource))
	}
	result, err := downloader.ProbeURL(a.ctx, rawURL, cookieBrowser, maxCount, sortOrder, searchSource)
	if err != nil {
		runtime.EventsEmit(a.ctx, "download_log", fmt.Sprintf("Lỗi dò link: %v", err))
		return nil, err
	}
	runtime.EventsEmit(a.ctx, "download_log", fmt.Sprintf("Tìm thấy %d video từ %s (%s). Đang tải ảnh xem trước...", len(result.Entries), result.Platform, result.Type))

	// Tải song song tất cả thumbnail về dạng base64 để bypass CSP/CORS
	var wg sync.WaitGroup
	for i := range result.Entries {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			thumbURL := result.Entries[idx].Thumbnail
			if thumbURL != "" && !strings.HasPrefix(thumbURL, "data:") {
				b64 := a.FetchImageAsBase64(thumbURL)
				if b64 != "" {
					result.Entries[idx].Thumbnail = b64
				}
			}
		}(i)
	}
	wg.Wait()

	runtime.EventsEmit(a.ctx, "download_log", fmt.Sprintf("Đã tải xong ảnh xem trước cho %d video.", len(result.Entries)))
	return result, nil
}

// DownloadOnlineVideo tải 1 video từ URL, emit event download_progress realtime.
func (a *App) DownloadOnlineVideo(rawURL string, outputDir string, cookieBrowser string, videoID string, videoTitle string) (*downloader.DownloadResult, error) {
	if outputDir == "" {
		outputDir = a.GetDefaultDownloadDir()
	}

	ctx, cancel := context.WithCancel(a.ctx)
	a.downloadCancelMu.Lock()
	if a.isDownloadCancelled {
		a.downloadCancelMu.Unlock()
		cancel()
		return nil, fmt.Errorf("tiến trình tải đã bị hủy")
	}
	a.downloadCancelFuncs[videoID] = cancel
	a.downloadCancelMu.Unlock()

	defer func() {
		a.downloadCancelMu.Lock()
		delete(a.downloadCancelFuncs, videoID)
		a.downloadCancelMu.Unlock()
	}()

	runtime.EventsEmit(a.ctx, "download_log", fmt.Sprintf("Bắt đầu tải: %s", videoTitle))

	result, err := downloader.DownloadVideo(ctx, rawURL, outputDir, cookieBrowser, videoID, videoTitle,
		func(p downloader.DownloadProgress) {
			runtime.EventsEmit(a.ctx, "download_progress", map[string]interface{}{
				"videoId": p.VideoID,
				"percent": p.Percent,
				"speed":   p.Speed,
				"eta":     p.ETA,
				"title":   p.Title,
			})
		},
	)

	if err != nil {
		runtime.EventsEmit(a.ctx, "download_complete", map[string]interface{}{
			"videoId": videoID, "ok": false, "error": err.Error(), "title": videoTitle,
		})
		return nil, err
	}

	// Đo duration thật bằng ffprobe
	if info, infoErr := media.GetVideoInfo(result.FilePath); infoErr == nil && info != nil {
		result.Duration = info.Duration
	}

	runtime.EventsEmit(a.ctx, "download_complete", map[string]interface{}{
		"videoId":  videoID,
		"ok":       true,
		"filePath": result.FilePath,
		"title":    result.Title,
		"duration": result.Duration,
	})
	runtime.EventsEmit(a.ctx, "download_log", fmt.Sprintf("Tải xong: %s → %s", videoTitle, result.FilePath))

	return result, nil
}

// DownloadOnlineVideos tải danh sách video song song (giới hạn 2 luồng đồng thời).
func (a *App) DownloadOnlineVideos(entries []downloader.VideoEntry, outputDir string, cookieBrowser string) ([]downloader.DownloadResult, error) {
	if outputDir == "" {
		outputDir = a.GetDefaultDownloadDir()
	}
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, fmt.Errorf("không tạo được thư mục tải: %v", err)
	}

	a.downloadCancelMu.Lock()
	a.isDownloadCancelled = false
	a.downloadCancelMu.Unlock()

	jobs := 2
	sem := make(chan struct{}, jobs)
	var wg sync.WaitGroup
	results := make([]downloader.DownloadResult, len(entries))

	for i, entry := range entries {
		wg.Add(1)
		sem <- struct{}{}
		go func(idx int, e downloader.VideoEntry) {
			defer wg.Done()
			defer func() { <-sem }()

			res, err := a.DownloadOnlineVideo(e.URL, outputDir, cookieBrowser, e.ID, e.Title)
			if err != nil {
				results[idx] = downloader.DownloadResult{Title: e.Title}
			} else {
				results[idx] = *res
			}
		}(i, entry)
	}
	wg.Wait()

	okCount := 0
	for _, r := range results {
		if r.FilePath != "" {
			okCount++
		}
	}
	runtime.EventsEmit(a.ctx, "download_log", fmt.Sprintf("Hoàn tất tải: %d/%d video thành công.", okCount, len(entries)))
	return results, nil
}

// CancelDownload hủy mọi download đang chạy.
func (a *App) CancelDownload() {
	a.downloadCancelMu.Lock()
	a.isDownloadCancelled = true
	for _, cancel := range a.downloadCancelFuncs {
		cancel()
	}
	a.downloadCancelFuncs = make(map[string]context.CancelFunc)
	a.downloadCancelMu.Unlock()
	runtime.EventsEmit(a.ctx, "download_log", "Đã hủy mọi tải video đang chạy!")
}

// GetDefaultDownloadDir trả về thư mục mặc định để lưu video tải về.
func (a *App) GetDefaultDownloadDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = os.TempDir()
	}
	dir := filepath.Join(home, "Videos", "VideoSplitter_Downloads")
	_ = os.MkdirAll(dir, 0755)
	return dir
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	if err != nil {
		return err
	}
	return out.Sync()
}

// ═══════════════════════════════════════════════════════════════════════════════
// IMAGE DOWNLOADER
// ═══════════════════════════════════════════════════════════════════════════════

// SearchImages tìm kiếm ảnh theo chủ đề từ nguồn được chỉ định.
// source: "duckduckgo" | "pixabay" | "unsplash" | "pexels"
// apiKey: bỏ trống nếu nguồn không cần key (DuckDuckGo)
// maxCount: số ảnh tối đa cần tìm (0 = dùng mặc định 50)
func (a *App) SearchImages(query string, source string, apiKey string, maxCount int) (*imagedownloader.ImageSearchResult, error) {
	runtime.EventsEmit(a.ctx, "image_search_log", fmt.Sprintf("Đang tìm ảnh '%s' từ %s...", query, source))
	result, err := imagedownloader.SearchImages(a.ctx, query, source, apiKey, maxCount)
	if err != nil {
		runtime.EventsEmit(a.ctx, "image_search_log", fmt.Sprintf("Lỗi tìm ảnh: %v", err))
		return nil, err
	}
	runtime.EventsEmit(a.ctx, "image_search_log", fmt.Sprintf("Tìm thấy %d ảnh từ %s!", len(result.Entries), result.Source))
	return result, nil
}

// DownloadImages tải danh sách ảnh về thư mục outputDir (song song 4 luồng).
func (a *App) DownloadImages(entries []imagedownloader.ImageEntry, outputDir string) ([]imagedownloader.ImageDownloadResult, error) {
	if outputDir == "" {
		outputDir = a.GetDefaultImageDownloadDir()
	}
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, fmt.Errorf("không tạo được thư mục: %v", err)
	}

	ctx, cancel := context.WithCancel(a.ctx)
	a.imageDownloadMu.Lock()
	if a.imageDownloadCancel != nil {
		a.imageDownloadCancel() // hủy lần tải trước nếu còn
	}
	a.imageDownloadCancel = cancel
	a.imageDownloadMu.Unlock()

	defer func() {
		a.imageDownloadMu.Lock()
		a.imageDownloadCancel = nil
		a.imageDownloadMu.Unlock()
		cancel()
	}()

	runtime.EventsEmit(a.ctx, "image_search_log", fmt.Sprintf("Bắt đầu tải %d ảnh...", len(entries)))

	results, err := imagedownloader.DownloadImages(ctx, entries, outputDir,
		func(p imagedownloader.ImageDownloadProgress) {
			runtime.EventsEmit(a.ctx, "image_download_progress", map[string]interface{}{
				"id":      p.ID,
				"title":   p.Title,
				"percent": p.Percent,
				"done":    p.Done,
				"total":   p.Total,
			})
		},
	)
	if err != nil {
		return nil, err
	}

	okCount := 0
	for _, r := range results {
		if r.OK {
			okCount++
		}
	}
	runtime.EventsEmit(a.ctx, "image_search_log", fmt.Sprintf("Hoàn tất: %d/%d ảnh tải thành công → %s", okCount, len(results), outputDir))
	runtime.EventsEmit(a.ctx, "image_download_done", map[string]interface{}{
		"ok":        okCount,
		"total":     len(results),
		"outputDir": outputDir,
	})
	return results, nil
}

// CancelImageDownload hủy tải ảnh đang chạy.
func (a *App) CancelImageDownload() {
	a.imageDownloadMu.Lock()
	defer a.imageDownloadMu.Unlock()
	if a.imageDownloadCancel != nil {
		a.imageDownloadCancel()
		a.imageDownloadCancel = nil
	}
	runtime.EventsEmit(a.ctx, "image_search_log", "Đã hủy tải ảnh!")
}

// GetDefaultImageDownloadDir trả về thư mục mặc định lưu ảnh tải về.
func (a *App) GetDefaultImageDownloadDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = os.TempDir()
	}
	dir := filepath.Join(home, "Pictures", "VideoSplitter_Images")
	_ = os.MkdirAll(dir, 0755)
	return dir
}
