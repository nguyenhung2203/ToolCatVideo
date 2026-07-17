package main

import (
	"context"
	"crypto/rand"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"math"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"video-splitter/internal/boundary"
	"video-splitter/internal/exporter"
	"video-splitter/internal/media"
	"video-splitter/internal/project"
	"video-splitter/internal/storage"

	"github.com/wailsapp/wails/v2/pkg/runtime"
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
	cancelFuncs       map[string]context.CancelFunc
	cancelMu          sync.Mutex
	exportCancelFuncs map[string]context.CancelFunc
	exportCancelMu    sync.Mutex
	isExportCancelled bool
	streamPort        int
	streamToken       string
	store             *storage.Store
}

// NewApp creates a new App application struct
func NewApp() *App {
	return &App{}
}

// startup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.cancelFuncs = make(map[string]context.CancelFunc)
	a.exportCancelFuncs = make(map[string]context.CancelFunc)
	a.isExportCancelled = false

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

// GetVideoInfo lấy thông tin của video qua ffprobe
func (a *App) GetVideoInfo(filePath string) (*project.VideoInfo, error) {
	return media.GetVideoInfo(filePath)
}

// GetDefaultConfig trả về cấu hình mặc định cho frontend
func (a *App) GetDefaultConfig() project.AnalyzerConfig {
	return project.DefaultConfig()
}

// Analyze chạy pipeline phân tích (Proxy -> Audio -> Python Worker -> Boundary Score)
func (a *App) Analyze(sourcePath string, cfg project.AnalyzerConfig) ([]project.Clip, error) {
	if cfg.Mode == "fixed" {
		return a.analyzeFixed(sourcePath, cfg)
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
	needAudio := cfg.Mode == project.ModePrecise
	if needAudio {
		runtime.EventsEmit(a.ctx, "analyze_log", "Bước 1/3: Đang tối ưu hóa video (tạo proxy 240p & trích xuất âm thanh song song)...")
	} else {
		runtime.EventsEmit(a.ctx, "analyze_log", "Bước 1/3: Đang tối ưu hóa video (tạo proxy 240p)...")
	}
	runtime.EventsEmit(a.ctx, "analyze_progress", map[string]interface{}{"path": sourcePath, "progress": 5})

	var wg sync.WaitGroup
	var errProxy, errAudio error

	wg.Add(1)
	go func() {
		defer wg.Done()
		proxyFPS := strconv.Itoa(cfg.ProxyFPS)
		errProxy = media.GenerateProxy(ctx, sourcePath, proxyPath, proxyFPS)
	}()

	if needAudio {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errAudio = media.ExtractAudio(ctx, sourcePath, audioPath)
		}()
	}

	wg.Wait()

	if errProxy != nil {
		return nil, fmt.Errorf("lỗi tạo proxy: %v", errProxy)
	}
	if needAudio && errAudio != nil {
		return nil, fmt.Errorf("lỗi tạo audio: %v", errAudio)
	}

	// Đọc tổng thời lượng
	info, _ := media.GetVideoInfo(sourcePath)
	totalDuration := 0.0
	if info != nil {
		totalDuration = info.Duration
	}

	// 2. Chạy Python worker trên proxy (240p) và audio WAV (nếu có)
	runtime.EventsEmit(a.ctx, "analyze_log", fmt.Sprintf("Bước 2/3: Đang phân tích chuyển cảnh (scene=%.1f, minClip=%.0fs, maxClip=%.0fs)...", cfg.SceneThreshold, cfg.MinClipDuration, cfg.MaxClipDuration))
	runtime.EventsEmit(a.ctx, "analyze_progress", map[string]interface{}{"path": sourcePath, "progress": 15})
	
	passedAudioPath := ""
	if needAudio {
		passedAudioPath = audioPath
	}
	clips, err := boundary.AnalyzeVideo(ctx, sourcePath, proxyPath, passedAudioPath, "python", cfg, totalDuration)
	if err != nil {
		return nil, fmt.Errorf("lỗi phân tích: %v", err)
	}

	// An toàn: cập nhật EndTime clip cuối nếu analyzer chưa biết.
	if info != nil && len(clips) > 0 && clips[len(clips)-1].EndTime <= 0 {
		clips[len(clips)-1].EndTime = info.Duration
		clips[len(clips)-1].Duration = info.Duration - clips[len(clips)-1].StartTime
	}

	// 3. Tạo thumbnails cho từng clip song song (hạn chế 8 luồng ffmpeg đồng thời để tránh làm nghẽn CPU)
	runtime.EventsEmit(a.ctx, "analyze_log", fmt.Sprintf("Bước 3/3: Đang trích xuất %d ảnh xem trước (song song)...", len(clips)))
	thumbDir := filepath.Join(workDir, "thumbnails")
	_ = os.MkdirAll(thumbDir, 0755)

	type thumbJob struct {
		index int
		start bool
	}

	numJobs := len(clips) * 2
	jobsChan := make(chan thumbJob, numJobs)
	for i := 0; i < len(clips); i++ {
		jobsChan <- thumbJob{index: i, start: true}
		jobsChan <- thumbJob{index: i, start: false}
	}
	close(jobsChan)

	numWorkers := 8
	if numWorkers > numJobs {
		numWorkers = numJobs
	}

	var wgThumbs sync.WaitGroup
	var mu sync.Mutex
	wgThumbs.Add(numWorkers)

	for w := 0; w < numWorkers; w++ {
		go func() {
			defer wgThumbs.Done()
			for job := range jobsChan {
				i := job.index
				if job.start {
					thumbPath := filepath.Join(thumbDir, fmt.Sprintf("thumb_%s_%d.jpg", clips[i].ID, i))
					_ = media.ExtractFrame(ctx, sourcePath, clips[i].StartTime, thumbPath)
					mu.Lock()
					clips[i].Thumbnail = thumbPath
					mu.Unlock()
				} else {
					if clips[i].EndTime > clips[i].StartTime {
						endThumbPath := filepath.Join(thumbDir, fmt.Sprintf("thumbend_%s_%d.jpg", clips[i].ID, i))
						endAt := clips[i].EndTime - 0.1
						if endAt < clips[i].StartTime {
							endAt = clips[i].StartTime
						}
						_ = media.ExtractFrame(ctx, sourcePath, endAt, endThumbPath)
						mu.Lock()
						clips[i].ThumbEnd = endThumbPath
						mu.Unlock()
					}
				}
			}
		}()
	}

	wgThumbs.Wait()

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
func (a *App) ExportClips(projectName string, sourcePath string, clips []project.Clip, outDir string, cfg project.AnalyzerConfig, jobs int) ([]ExportResult, error) {
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return nil, fmt.Errorf("không tạo được thư mục xuất: %v", err)
	}
	if jobs <= 0 {
		jobs = 2
	}
	if jobs > 8 {
		jobs = 8
	}

	a.exportCancelMu.Lock()
	a.isExportCancelled = false
	a.exportCancelFuncs = make(map[string]context.CancelFunc)
	a.exportCancelMu.Unlock()

	results := make([]ExportResult, len(clips))
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

			err := exporter.CutVideo(clipCtx, sourcePath, clip, outPath, cfg.ExportPreset, cfg.ExportCRF)
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
			results[i] = res

			mu.Lock()
			done++
			runtime.EventsEmit(a.ctx, "export_progress", map[string]any{
				"done": done, "total": len(clips), "clipId": clip.ID, "ok": res.OK,
			})
			statusW := statusWord(res.OK)
			if clipCtx.Err() != nil {
				statusW = "ĐÃ DỪNG"
			}
			runtime.EventsEmit(a.ctx, "export_log", fmt.Sprintf("Xuất %d/%d: Clip #%d %s",
				done, len(clips), clip.Index, statusW))
			mu.Unlock()
		}(i, clip)
	}
	wg.Wait()

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
		if err := exporter.CutVideo(context.Background(), sourcePath, clip, p, cfg.ExportPreset, cfg.ExportCRF); err != nil {
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
	if err := exporter.ConcatClips(parts, outPath, transType, transDur, cfg.ExportPreset, cfg.ExportCRF); err != nil {
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
