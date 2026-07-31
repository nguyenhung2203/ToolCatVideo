package boundary

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"video-splitter/internal/media"
	"video-splitter/internal/project"
	"video-splitter/internal/utils"

	"crypto/sha1"
	"encoding/hex"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// CandidateSignals chứa cường độ (0-100) của từng loại tín hiệu do Python phát hiện.
// SpeechBreak (0-100) đo mức độ NGẮT giọng nói tại điểm cắt: 100 = có khoảng lặng
// giọng nói rõ ràng (ranh giới hợp lệ), 0 = giọng nói xuyên qua liền mạch (nên phạt).
type CandidateSignals struct {
	VisualChange int `json:"visual_change"`
	BlackFrame   int `json:"black_frame"`
	Silence      int `json:"silence"`
	LayoutChange int `json:"layout_change"`
	AudioChange  int `json:"audio_change"`
	SpeechBreak  int `json:"speech_break"`
}

// Candidate đại diện cho một điểm cắt tiềm năng nhận từ Python
type Candidate struct {
	Timestamp  float64          `json:"timestamp"`
	Confidence int              `json:"confidence"`
	Signals    CandidateSignals `json:"signals"`
	Reason     string           `json:"reason"`
}

// AnalyzerResult kết quả trả về từ Python
type AnalyzerResult struct {
	Status     string      `json:"status"`
	ProxyPath  string      `json:"proxy_path"`
	AudioPath  string      `json:"audio_path"`
	Candidates []Candidate `json:"candidates"`
	Message    string      `json:"message"`
}

// scoredBoundary lưu timestamp kèm thông tin điểm/tín hiệu để dựng clip sau này.
type scoredBoundary struct {
	timestamp  float64
	score      int      // Boundary Score đã tính (0-100)
	confidence int      // = score, alias cho rõ nghĩa khi gán vào clip
	tier       string   // auto / review
	reason     string   // lý do (từ Python)
	signals    []string // danh sách tín hiệu đã phát hiện
}

// AnalyzeVideo gọi script Python và đọc kết quả cũng như tiến độ.
// totalDuration là tổng thời lượng video nguồn (giây) — cần để đặt EndTime cho
// clip cuối TRƯỚC khi chia clip quá dài; nếu <=0 sẽ để app.go cập nhật sau.
// proxyPath và audioPath giữ cho backward compat nhưng Python worker giờ chạy
// trực tiếp trên sourcePath — không cần proxy/audio riêng.
func AnalyzeVideo(ctx context.Context, sourcePath, proxyPath, audioPath string, pythonExe string, cfg project.AnalyzerConfig, totalDuration float64) ([]project.Clip, error) {
	mode := cfg.Mode
	if mode == "" {
		mode = project.ModeSmart
	}

	analyzeArgs := []string{
		"--proxy", proxyPath,
		"--audio", audioPath,
		"--source", sourcePath,
		"--ffmpeg", utils.GetBinPath("ffmpeg"),
		"--mode", mode,
		"--scene-threshold", fmt.Sprintf("%.1f", cfg.SceneThreshold),
		"--silence-db", fmt.Sprintf("%.0f", cfg.SilenceThreshold),
		"--silence-duration", fmt.Sprintf("%.2f", cfg.SilenceDuration),
	}

	// Chỉ báo worker khi video KHÔNG có audio để nó bỏ nhánh phân tích âm thanh
	// (tránh tạo/đọc WAV rỗng). VFR được worker xử lý gốc qua pts_time thật của
	// showinfo (không cần cờ); start_time không cần vì refine dùng hệ 0-based
	// nhất quán với lệnh cắt (-ss không -copyts). Probe rẻ (1 lần ffprobe).
	if vi, err := media.GetVideoInfo(sourcePath); err == nil && vi != nil {
		if !vi.HasAudio {
			analyzeArgs = append(analyzeArgs, "--no-audio")
		}
	}

	// Quyết định exe để chạy Python worker:
	var exePath string
	var cmdArgs []string

	// Kiểm tra xem script python dev có tồn tại không
	scriptPath := utils.GetWorkerScript()
	scriptExists := false
	if _, err := os.Stat(scriptPath); err == nil {
		scriptExists = true
	}

	// Thử chạy bằng Python script trước nếu tồn tại script (để dev/test tiện lợi)
	useScript := false
	if scriptExists {
		checkCmd := exec.Command(pythonExe, "--version")
		utils.HideCmdWindow(checkCmd)
		if err := checkCmd.Run(); err == nil {
			useScript = true
		}
	}

	if useScript {
		exePath = pythonExe
		cmdArgs = append([]string{scriptPath}, analyzeArgs...)
	} else if workerExe := utils.GetWorkerExe(); workerExe != "" {
		exePath = workerExe
		cmdArgs = analyzeArgs
	} else {
		exePath = pythonExe
		cmdArgs = append([]string{scriptPath}, analyzeArgs...)
	}

	// CommandContext để có thể hủy; Cancel kill cả cây tiến trình (worker + ffmpeg con)
	cmd := exec.CommandContext(ctx, exePath, cmdArgs...)
	utils.HideCmdWindow(cmd)
	cmd.Env = append(os.Environ(), "PYTHONUTF8=1", "PYTHONIOENCODING=utf-8")
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		killCmd := exec.Command("taskkill", "/F", "/T", "/PID", strconv.Itoa(cmd.Process.Pid))
		utils.HideCmdWindow(killCmd)
		return killCmd.Run()
	}

	// Hứng luồng stderr để hiển thị thông tin lỗi chi tiết khi sập
	var stderrBuf bytes.Buffer
	cmd.Stderr = &stderrBuf

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("lỗi khởi tạo stdout pipe: %v", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("lỗi khởi động python worker: %v", err)
	}

	var jsonOutput strings.Builder
	scanner := bufio.NewScanner(stdout)
	// Tăng kích thước buffer của scanner lên tối đa 10MB để tránh bị sập/ deadlock khi nhận chuỗi JSON kết quả rất dài từ video lớn
	scanner.Buffer(make([]byte, 64*1024), 10*1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "PROGRESS:") {
			parts := strings.Split(line, ":")
			if len(parts) == 2 {
				prog, _ := strconv.Atoi(parts[1])
				// Map progress từ python (0-100) vào khoảng (15-90) để dành đoạn cuối cho Bước 3
				realProg := 15 + (prog * 75 / 100)
				runtime.EventsEmit(ctx, "analyze_progress", map[string]interface{}{"path": sourcePath, "progress": realProg})
			}
		} else if strings.HasPrefix(line, "DETECTOR_DONE:") {
			// Python báo đã hoàn thành một thuật toán phân tích, hiện log chi tiết
			detectorInfo := strings.TrimPrefix(line, "DETECTOR_DONE:")
			detParts := strings.SplitN(detectorInfo, "|", 2)
			if len(detParts) == 2 {
				runtime.EventsEmit(ctx, "analyze_log", fmt.Sprintf("  ✓ %s: tìm thấy %s điểm", detParts[0], detParts[1]))
			}
		} else if strings.HasPrefix(line, "STATUS_LOG:") {
			// Python gửi log trạng thái chi tiết thời gian thực khi đang quét
			statusMsg := strings.TrimPrefix(line, "STATUS_LOG:")
			runtime.EventsEmit(ctx, "analyze_log", statusMsg)
		} else {
			// Thu thập json output
			jsonOutput.WriteString(line)
		}
	}

	// Đọc Wait trước nhưng chỉ xử lý sau khi kiểm tra nội dung JSON
	waitErr := cmd.Wait()

	var result AnalyzerResult
	unmarshalErr := json.Unmarshal([]byte(jsonOutput.String()), &result)

	// Ghi debug log để theo dõi kết quả thô từ Python worker
	_ = os.WriteFile(filepath.Join(os.TempDir(), "video-splitter-debug-json.log"), []byte(jsonOutput.String()), 0644)
	_ = os.WriteFile(filepath.Join(os.TempDir(), "video-splitter-debug-stderr.log"), stderrBuf.Bytes(), 0644)

	// Nếu Python có thông điệp lỗi tự cấu trúc thành công, ưu tiên hiển thị lỗi này
	if unmarshalErr == nil && result.Status == "error" && result.Message != "" {
		return nil, fmt.Errorf("lỗi từ python worker: %s", result.Message)
	}

	// Nếu Python sập hệ thống (exit code != 0), đính kèm stderr chi tiết
	if waitErr != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		stderrDetail := strings.TrimSpace(stderrBuf.String())
		if stderrDetail != "" {
			return nil, fmt.Errorf("python worker kết thúc có lỗi: %v\nChi tiết lỗi (Traceback):\n%s", waitErr, stderrDetail)
		}
		return nil, fmt.Errorf("python worker kết thúc có lỗi: %v", waitErr)
	}

	// Lỗi giải mã JSON (nếu Python in ra chuỗi rác hoặc sập đột ngột mà không in JSON)
	if unmarshalErr != nil {
		stderrDetail := strings.TrimSpace(stderrBuf.String())
		if stderrDetail != "" {
			return nil, fmt.Errorf("lỗi parse json từ python: %v, output: %s\nChi tiết stderr:\n%s", unmarshalErr, jsonOutput.String(), stderrDetail)
		}
		return nil, fmt.Errorf("lỗi parse json từ python: %v, output: %s", unmarshalErr, jsonOutput.String())
	}

	if result.Status != "success" {
		return nil, fmt.Errorf("python worker returned error: %s", result.Message)
	}

	return CalculateBoundaries(result.Candidates, cfg, totalDuration, sourcePath), nil
}

// boundaryScore tính Boundary Score cho một candidate theo công thức có trọng số:
//
//	Score = Σ (tín hiệu bật × trọng số tương ứng) - Continuity Penalty
//
// Continuity Penalty áp dụng khi có dấu hiệu liền mạch: chỉ có chuyển cảnh hình ảnh
// (đổi camera) mà KHÔNG kèm im lặng / đổi âm thanh / màn hình đen — tức nhiều khả năng
// đây là chuyển cảnh minh họa trong cùng một video ngắn, không phải ranh giới thật.
//
// Trả về điểm đã kẹp trong [0, 100] và danh sách tín hiệu đã phát hiện.
// COOccurBonus: điểm thưởng khi có từ 3 họ tín hiệu độc lập cùng bật tại một điểm.
// Nhiều tín hiệu độc lập đồng thời → khả năng cao là ranh giới thật, không phải nhiễu.
const COOccurBonus = 10.0

// norm chuẩn hoá cường độ tín hiệu (0-100 từ Python) về hệ số [0,1] để nhân trọng số.
func norm(intensity int) float64 {
	v := float64(intensity) / 100.0
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func boundaryScore(c Candidate, w project.SignalWeights) (int, []string) {
	var score float64
	var signals []string

	// Tính điểm theo CƯỜNG ĐỘ (không nhị phân on/off): mỗi tín hiệu đóng góp
	// trọng số × mức độ mạnh thực tế. Một scene-change mờ nhạt (intensity 20)
	// đóng góp ít hơn hẳn một hard-cut rõ (intensity 95).
	nVisual := norm(c.Signals.VisualChange)
	nBlack := norm(c.Signals.BlackFrame)
	nSilence := norm(c.Signals.Silence)
	nLayout := norm(c.Signals.LayoutChange)
	nAudio := norm(c.Signals.AudioChange)

	families := 0
	if nVisual > 0 {
		score += w.VisualChange * nVisual
		signals = append(signals, "visual")
		families++
	}
	if nBlack > 0 {
		score += w.BlackFrame * nBlack
		signals = append(signals, "black")
		families++
	}
	if nSilence > 0 {
		score += w.Silence * nSilence
		signals = append(signals, "silence")
		families++
	}
	if nLayout > 0 {
		score += w.LayoutChange * nLayout
		signals = append(signals, "layout")
		families++
	}

	// Audio (đổi đặc trưng nhạc/môi trường) CHỈ được tính khi có ít nhất một tín hiệu
	// KHÁC xác nhận (đổi hình/đen/layout/im lặng). Audio đứng MỘT MÌNH gần như luôn là
	// nhịp/drop của nhạc nền giữa clip — không phải ranh giới thật — nên nếu cho nó tự
	// cắt sẽ băm một clip thành nhiều mảnh (đặc biệt ở chế độ Kỹ, nơi trọng số audio
	// đủ vượt ngưỡng). Yêu cầu corroboration là cách trị gốc hiện tượng cắt quá nhạy
	// do nhạc mà không làm mất ranh giới thật (ranh giới thật luôn kèm đổi hình/đen).
	if nAudio > 0 {
		if families > 0 {
			score += w.AudioChange * nAudio
			signals = append(signals, "audio")
			families++
		} else {
			signals = append(signals, "audio-solo-bỏ")
		}
	}

	// Co-occurrence bonus: ≥3 họ tín hiệu độc lập cùng bật → ranh giới rất đáng tin.
	if families >= 3 {
		score += COOccurBonus
		signals = append(signals, "cooccur")
	}

	// Continuity Penalty (đo thật qua SpeechBreak): chỉ áp khi có chuyển cảnh hình ảnh
	// mà KHÔNG có màn hình đen. Mức phạt tỉ lệ nghịch với SpeechBreak — giọng nói xuyên
	// qua điểm cắt càng liền mạch (SpeechBreak thấp) thì phạt càng mạnh (nhiều khả năng
	// chỉ là đổi góc quay trong cùng một clip, không phải ranh giới thật).
	// SpeechBreak = 0 khi không có dữ liệu audio (no-audio / fast mode) → giữ hành vi
	// bảo thủ: nếu không có bất kỳ dấu hiệu ngắt mạch audio nào, vẫn phạt như cũ.
	hasVisual := nVisual > 0
	hasBlack := nBlack > 0
	hasAudioBreak := nSilence > 0 || nAudio > 0
	if hasVisual && !hasBlack {
		if hasAudioBreak {
			// Có tín hiệu audio: phạt theo mức độ liền mạch giọng nói.
			penaltyFactor := 1.0 - norm(c.Signals.SpeechBreak)
			if penaltyFactor > 0 {
				score -= w.ContinuityPen * penaltyFactor
				signals = append(signals, "continuity-penalty")
			}
		} else {
			// Không có tín hiệu audio nào (visual đơn độc) → phạt toàn phần như cũ.
			score -= w.ContinuityPen
			signals = append(signals, "continuity-penalty")
		}
	}

	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}
	return int(score), signals
}

// tierFor phân loại điểm ranh giới theo hai ngưỡng cấu hình được.
func tierFor(score int, cfg project.AnalyzerConfig) string {
	if score >= cfg.AutoAcceptScore {
		return project.TierAuto
	}
	if score >= cfg.ReviewMinScore {
		return project.TierReview
	}
	return project.TierReject
}

// effectiveScoringConfig tạo bộ chấm điểm nội bộ theo chế độ mà không sửa cấu hình
// người dùng đã lưu.
//
// LƯU Ý — ngữ nghĩa 3 chế độ đã được sửa lại cho khớp với TÊN của chúng. Bản trước
// mọi phép điều chỉnh đều một chiều theo hướng NỚI LỎNG (visual chỉ kéo lên, phạt
// liền mạch chỉ kẹp xuống, ngưỡng chỉ hạ xuống), nên không chế độ nào thắt lại được.
// Hệ quả: "Kỹ" là chế độ CẮT NHIỀU NHẤT chứ không phải chính xác nhất — nó vừa hạ
// ngưỡng, vừa giảm phạt liền mạch còn 5, trong khi phía Python còn hạ cả ngưỡng phát
// hiện. Người dùng bấm "Kỹ" tưởng cắt cẩn thận hơn, thực tế nó băm vụn nhất.
//
// Ngữ nghĩa mới:
//
//	Nhanh : bảo thủ nhất — chỉ hard cut / màn hình đen thật rõ. Ít clip, chạy nhanh.
//	Tự động: cân bằng.
//	Kỹ    : quét dày candidate (phía Python) nhưng chấp nhận NGHIÊM hơn — ít clip mà
//	        đúng, vị trí cắt chuẩn từng khung. Không phải "nhiều clip hơn".
//
// lenWeight là trọng số độ lệch chiều dài dùng cho bộ chọn ranh giới tối ưu:
// càng cao thì càng ưu tiên clip đều nhau quanh TargetClipDuration.
func effectiveScoringConfig(cfg project.AnalyzerConfig) (w project.SignalWeights, reviewMin int, lenWeight float64) {
	w = cfg.Weights
	reviewMin = cfg.ReviewMinScore
	lenWeight = 1.0

	switch cfg.Mode {
	case project.ModeFast:
		// Nhanh chỉ có tín hiệu thô từ FFmpeg (scene 0.35, black, silence) — không
		// có refine, không có layout. Giữ phạt liền mạch ĐỦ MẠNH và nâng ngưỡng để
		// chỉ ranh giới thật rõ được cắt; đây là chế độ đánh đổi độ phủ lấy tốc độ.
		if reviewMin < 45 {
			reviewMin = 45
		}
		// Ít candidate → dựa nhiều hơn vào mốc độ dài để clip không dài ngắn thất thường.
		lenWeight = 1.4
	case project.ModeSmart:
		// Cân bằng: nới phạt liền mạch một chút so với mặc định vì đã có SpeechBreak
		// đo thật, không cần phạt toàn phần.
		if w.ContinuityPen > 12 {
			w.ContinuityPen = 12
		}
	case project.ModePrecise:
		// Kỹ: NÂNG ngưỡng và TĂNG phạt liền mạch (ngược hẳn bản cũ). Python đã sinh
		// candidate dày và refine về fps gốc, nên việc của Go là chọn lọc nghiêm —
		// nếu đây cũng nới lỏng thì hai tầng cùng chiều sẽ cho ra clip vụn.
		if reviewMin < 40 {
			reviewMin = 40
		}
		if w.ContinuityPen < 18 {
			w.ContinuityPen = 18
		}
		// Tin vào tín hiệu hơn là vào mốc độ dài, vì candidate đã được tinh chỉnh kỹ.
		lenWeight = 0.8
	}

	return w, reviewMin, lenWeight
}

// selectBoundaries chọn TẬP ranh giới tối ưu bằng quy hoạch động thay vì duyệt tham
// lam từng điểm.
//
// Vì sao cần: bản cũ quyết định từng điểm cắt ĐỘC LẬP ("điểm này có vượt ngưỡng
// không?") rồi gom cụm tham lam trong cửa sổ MinClipDuration. Cách đó không có khái
// niệm "clip nên dài bao nhiêu", nên gặp ranh giới đạt điểm ở giây thứ 6 là cắt ngay
// — chính là hiện tượng 1 đoạn nội dung bị băm thành 2-3 khúc.
//
// Cách làm: tối thiểu hoá tổng chi phí trên TOÀN video
//
//	cost(đoạn a→b) = lenWeight × ((độ dài − target)/target)² − score(b)/100
//	ràng buộc      : MinClipDuration ≤ độ dài ≤ MaxClipDuration
//
// Nhờ xét toàn cục, thuật toán sẵn sàng BỎ QUA ranh giới mạnh ở giây 6 để chọn ranh
// giới yếu hơn ở giây 33 nếu tổng chi phí thấp hơn. Đây là bài toán segmentation kinh
// điển (cùng dạng Knuth-Plass dùng ngắt dòng văn bản), giải bằng DP O(n²) trên số
// ranh giới ứng viên — với vài trăm điểm là tức thời.
//
// Trả về danh sách ranh giới đã chọn, theo thứ tự thời gian.
func selectBoundaries(scored []scoredBoundary, cfg project.AnalyzerConfig, totalDuration float64, lenWeight float64) []scoredBoundary {
	target := cfg.TargetClipDuration
	// target <= 0 → người dùng tắt tính năng: quay về gom cụm tham lam như cũ.
	if target <= 0 || totalDuration <= 0 || len(scored) == 0 {
		return greedyCluster(scored, cfg)
	}

	minD := cfg.MinClipDuration
	maxD := cfg.MaxClipDuration
	if maxD <= 0 {
		maxD = totalDuration
	}
	// Cấu hình vô lý (min > max) → không DP được, trả về tham lam cho an toàn.
	if minD > maxD {
		return greedyCluster(scored, cfg)
	}

	// Node 0 = đầu video (t=0); node 1..n = các ranh giới ứng viên; node n+1 = cuối video.
	n := len(scored)
	times := make([]float64, n+2)
	times[0] = 0
	for i, b := range scored {
		times[i+1] = b.timestamp
	}
	times[n+1] = totalDuration

	inf := math.Inf(1)
	// best[j] = tổng chi phí tối thiểu để cắt đoạn [0, times[j]] với j là điểm cắt.
	best := make([]float64, n+2)
	prev := make([]int, n+2)
	for j := range best {
		best[j] = inf
		prev[j] = -1
	}
	best[0] = 0

	// segCost tính chi phí một đoạn từ node i tới node j. Chi phí LUÔN >= 0.
	//
	// Điểm ranh giới vào công thức dưới dạng HÌNH PHẠT chất lượng (1 - score/100), không
	// phải phần thưởng (-score/100). Khác biệt này quyết định cả thuật toán: nếu là phần
	// thưởng thì mỗi nhát cắt được cộng tới -1.0 điểm lợi, trong khi một clip 10s lệch mốc
	// 30s chỉ bị phạt 0.44 — DP sẽ luôn thấy "cắt thêm" là có lợi và băm vụn video, đúng
	// hiện tượng cần trị. Với hình phạt, thêm một đoạn là thêm một khoản chi phí >= 0 nên
	// nhát cắt phải TỰ BIỆN MINH bằng việc giảm độ lệch chiều dài của các đoạn quanh nó.
	segCost := func(i, j int) float64 {
		length := times[j] - times[i]
		dev := (length - target) / target
		cost := lenWeight * dev * dev
		// Node cuối (kết thúc video) không do ranh giới nào tạo ra → không xét chất lượng.
		if j <= n {
			cost += 1.0 - float64(scored[j-1].score)/100.0
		}
		return cost
	}

	for j := 1; j <= n+1; j++ {
		for i := 0; i < j; i++ {
			if best[i] == inf {
				continue
			}
			length := times[j] - times[i]
			if length < minD {
				continue
			}
			// Đoạn cuối được phép ngắn hơn min (phần dư của video) nhưng vẫn phải
			// tôn trọng max; bước gộp clip ngắn ở sau sẽ xử lý phần dư.
			if length > maxD {
				continue
			}
			if c := best[i] + segCost(i, j); c < best[j] {
				best[j] = c
				prev[j] = i
			}
		}
	}

	// Nếu không tới được node cuối (ví dụ video ngắn hơn min, hoặc mọi đoạn khả thi
	// đều vượt max vì thiếu ranh giới), lùi về tham lam rồi để splitLongClips lo.
	if best[n+1] == inf {
		return greedyCluster(scored, cfg)
	}

	// Truy vết ngược, bỏ node 0 và node cuối vì chúng không phải ranh giới thật.
	var picked []scoredBoundary
	for j := prev[n+1]; j > 0; j = prev[j] {
		picked = append(picked, scored[j-1])
	}
	// Đảo lại thành thứ tự thời gian tăng dần.
	for l, r := 0, len(picked)-1; l < r; l, r = l+1, r-1 {
		picked[l], picked[r] = picked[r], picked[l]
	}
	return picked
}

// greedyCluster là hành vi gom cụm cũ: giữ ranh giới điểm cao nhất trong mỗi cửa sổ
// MinClipDuration. Dùng khi tắt TargetClipDuration hoặc khi DP không tìm được lời giải.
func greedyCluster(scored []scoredBoundary, cfg project.AnalyzerConfig) []scoredBoundary {
	var filtered []scoredBoundary
	for _, b := range scored {
		if len(filtered) == 0 {
			filtered = append(filtered, b)
			continue
		}
		last := &filtered[len(filtered)-1]
		if b.timestamp-last.timestamp < cfg.MinClipDuration {
			if b.score > last.score {
				*last = b
			}
		} else {
			filtered = append(filtered, b)
		}
	}
	return filtered
}

// filterByProminence chỉ giữ ranh giới TRỘI HƠN HẲN vùng lân cận, thay vì so với một
// ngưỡng tuyệt đối.
//
// Vì sao cần: MV ca nhạc, gameplay hay video edit nhanh có chuyển cảnh liên tục nên
// điểm nào cũng "cao" theo thang tuyệt đối — ngưỡng cố định không phân biệt được đâu
// là ranh giới nội dung, đâu là nhịp dựng bình thường. So tương đối với lân cận thì
// bộ lọc tự thích nghi với từng loại video: video ít chuyển cảnh giữ gần như mọi
// điểm, video băm liên tục chỉ giữ các đỉnh thật.
//
// Điều kiện giữ: score >= mean(lân cận) + k × std(lân cận), hoặc là điểm cao nhất
// trong cửa sổ. Ranh giới điểm rất cao (>= AutoAcceptScore) luôn được giữ.
func filterByProminence(scored []scoredBoundary, windowSec float64, k float64, keepAbove int) []scoredBoundary {
	// Dưới 4 điểm thì thống kê lân cận vô nghĩa — giữ nguyên.
	if len(scored) < 4 || windowSec <= 0 {
		return scored
	}

	var out []scoredBoundary
	for i, b := range scored {
		if b.score >= keepAbove {
			out = append(out, b)
			continue
		}

		// Thu thập điểm của các ranh giới khác trong cửa sổ ±windowSec.
		var sum, sumSq float64
		count := 0
		isLocalMax := true
		for j, o := range scored {
			if j == i || abs(o.timestamp-b.timestamp) > windowSec {
				continue
			}
			sum += float64(o.score)
			sumSq += float64(o.score) * float64(o.score)
			count++
			if o.score > b.score {
				isLocalMax = false
			}
		}

		// Không có lân cận → điểm đứng một mình, không phải nhiễu dày đặc.
		if count == 0 || isLocalMax {
			out = append(out, b)
			continue
		}

		mean := sum / float64(count)
		variance := sumSq/float64(count) - mean*mean
		if variance < 0 {
			variance = 0
		}
		std := math.Sqrt(variance)
		if float64(b.score) >= mean+k*std {
			out = append(out, b)
		}
	}
	return out
}

// CalculateBoundaries tính Boundary Score cho từng candidate, loại bỏ điểm dưới ngưỡng,
// gom cụm điểm gần nhau (giữ điểm mạnh nhất), rồi dựng danh sách clip kèm confidence/tier/reason.
// sourcePath dùng để snap boundary sang keyframe gần nhất (tối ưu cho stream-copy).
func CalculateBoundaries(candidates []Candidate, cfg project.AnalyzerConfig, totalDuration float64, sourcePath string) []project.Clip {
	w, reviewMin, lenWeight := effectiveScoringConfig(cfg)
	tierCfg := cfg
	tierCfg.ReviewMinScore = reviewMin

	// === BƯỚC 1: Tính Boundary Score. CHỈ tier auto được dùng làm điểm cắt ===
	// Trước đây bước này chỉ loại tier reject, nên tier review ("cần người duyệt")
	// cũng thành điểm cắt thật — ngưỡng cắt thực tế tụt từ AutoAcceptScore về
	// ReviewMinScore và mọi điểm còn ngờ ngợ đều bị cắt, dù không có UI nào để duyệt.
	// Nay tier review chỉ được giữ làm ứng viên dự phòng cho bước chia clip quá dài
	// (splitLongClips), nơi cắt tại ranh giới yếu vẫn tốt hơn cắt cứng theo thời gian.
	var scored []scoredBoundary
	for _, c := range candidates {
		score, signals := boundaryScore(c, w)
		if tierFor(score, tierCfg) != project.TierAuto {
			continue
		}
		scored = append(scored, scoredBoundary{
			timestamp:  c.Timestamp,
			score:      score,
			confidence: score,
			tier:       project.TierAuto,
			reason:     c.Reason,
			signals:    signals,
		})
	}

	sort.Slice(scored, func(i, j int) bool {
		return scored[i].timestamp < scored[j].timestamp
	})

	// === BƯỚC 1.5: Snap boundary sang keyframe gần nhất (nếu biết sourcePath) ===
	// Chỉ Tự động snap nhẹ sang I-frame để tối ưu stream-copy mà không kéo lệch
	// ranh giới đáng kể. Nhanh giữ nguyên timestamp scene FFmpeg; Kỹ giữ nguyên
	// timestamp refine ở fps gốc và lúc xuất sẽ re-encode để cắt đúng từng khung.
	snapLimit := 0.3
	switch cfg.Mode {
	case project.ModeFast, project.ModePrecise:
		snapLimit = 0.0
	}

	if sourcePath != "" && snapLimit > 0 {
		if kf, err := media.LoadAllKeyframes(sourcePath); err == nil && len(kf) > 0 {
			for i := range scored {
				snapped := media.FindNearestKeyframeCached(kf, scored[i].timestamp)
				if abs(snapped-scored[i].timestamp) < snapLimit {
					scored[i].timestamp = snapped
				}
			}
		} else {
			// Fallback nếu LoadAllKeyframes bị lỗi: dùng FindNearestKeyframe từng cái như cũ
			for i := range scored {
				snapped := media.FindNearestKeyframe(sourcePath, scored[i].timestamp)
				if abs(snapped-scored[i].timestamp) < snapLimit {
					scored[i].timestamp = snapped
				}
			}
		}
	}

	// Lọc bỏ ranh giới quá gần đầu hoặc cuối video sau khi đã snap
	var validScored []scoredBoundary
	for _, b := range scored {
		if b.timestamp < cfg.MinClipDuration {
			continue
		}
		if totalDuration > 0 && totalDuration-b.timestamp < cfg.MinClipDuration {
			continue
		}
		validScored = append(validScored, b)
	}
	scored = validScored

	// === BƯỚC 2a: Lọc theo độ trội so với lân cận ===
	// Cửa sổ lấy theo mốc độ dài mong muốn (tối thiểu 15s): trong phạm vi một clip,
	// chỉ những đỉnh thật mới đáng làm ranh giới. Ranh giới rất mạnh (>= autoAccept + 20)
	// luôn được giữ để không bỏ sót chuyển cảnh hiển nhiên.
	promWindow := cfg.TargetClipDuration / 2
	if promWindow < 15 {
		promWindow = 15
	}
	scored = filterByProminence(scored, promWindow, 0.5, cfg.AutoAcceptScore+20)

	// === BƯỚC 2b: Chọn TẬP ranh giới tối ưu toàn cục (DP) thay vì gom cụm tham lam ===
	filtered := selectBoundaries(scored, cfg, totalDuration, lenWeight)

	// === BƯỚC 3: Dựng danh sách clip sơ bộ. Ranh giới BẮT ĐẦU clip mang tier/score
	// của boundary tạo ra nó (boundary tại StartTime). ===
	var clips []project.Clip
	start := 0.0
	// Clip đầu tiên bắt đầu tại 0 (không do boundary nào tạo) → tin cậy tuyệt đối.
	startMeta := scoredBoundary{tier: project.TierAuto, confidence: 100, reason: "Đầu video", signals: []string{"start"}}

	for _, b := range filtered {
		clips = append(clips, buildClip(start, b.timestamp, startMeta))
		start = b.timestamp
		startMeta = b
	}
	// Clip cuối: nếu biết tổng thời lượng thì đặt EndTime ngay để bước chia
	// clip quá dài có thể xử lý luôn (nếu chưa biết, app.go sẽ cập nhật sau).
	last := buildClip(start, totalDuration, startMeta)
	if totalDuration <= 0 {
		last.EndTime = -1
		last.Duration = -1
	}
	clips = append(clips, last)

	// === BƯỚC 4: Chia clip quá dài (> MaxClipDuration) ===
	// Với video liên tục (vlog/gameplay) AI thường không tìm được ranh giới rõ,
	// dẫn tới một clip dài bằng cả video. Ta chia nó thành nhiều đoạn: ưu tiên
	// cắt tại candidate mạnh gần mốc mong muốn; nếu không có, cắt cứng theo thời gian.
	if cfg.MaxClipDuration > 0 {
		clips = splitLongClips(clips, candidates, cfg, w)
	}

	// === BƯỚC 5: Gộp clip quá ngắn (< MinClipDuration) với clip liền kề ===
	if len(clips) > 1 {
		var merged []project.Clip
		merged = append(merged, clips[0])
		for i := 1; i < len(clips); i++ {
			lastMerged := &merged[len(merged)-1]
			curr := clips[i]
			if curr.Duration > 0 && curr.Duration < cfg.MinClipDuration {
				lastMerged.EndTime = curr.EndTime
				lastMerged.Duration = lastMerged.EndTime - lastMerged.StartTime
			} else if lastMerged.Duration > 0 && lastMerged.Duration < cfg.MinClipDuration {
				curr.StartTime = lastMerged.StartTime
				curr.Duration = curr.EndTime - curr.StartTime
				merged[len(merged)-1] = curr
			} else {
				merged = append(merged, curr)
			}
		}
		clips = merged
	}

	// === BƯỚC 6: Đánh lại Index và ID ===
	var hashKey string
	if sourcePath != "" {
		h := sha1.New()
		h.Write([]byte(sourcePath))
		hashKey = hex.EncodeToString(h.Sum(nil))[:8]
	} else {
		hashKey = "default"
	}
	for i := range clips {
		clips[i].ID = fmt.Sprintf("clip_%s_%d", hashKey, i+1)
		clips[i].Index = i + 1
	}

	return clips
}

// buildClip dựng một clip từ [start, end] và metadata của boundary bắt đầu clip.
func buildClip(start, end float64, meta scoredBoundary) project.Clip {
	reason := meta.reason
	if reason == "" {
		reason = "—"
	}
	return project.Clip{
		StartTime:  start,
		EndTime:    end,
		Duration:   end - start,
		Status:     "pending",
		Confidence: meta.confidence,
		Tier:       meta.tier,
		Reason:     reason,
		Signals:    meta.signals,
	}
}

// splitLongClips chia mọi clip dài hơn MaxClipDuration thành nhiều đoạn.
// Mỗi lần cắt, tìm candidate mạnh gần mốc "start + MaxClipDuration"; nếu không có
// candidate phù hợp (video liên tục), cắt cứng đúng tại mốc thời gian đó. Nhờ vậy
// video dài luôn được chia đều thay vì trả về một clip duy nhất.
func splitLongClips(clips []project.Clip, candidates []Candidate, cfg project.AnalyzerConfig, w project.SignalWeights) []project.Clip {
	// Bước chia dùng mốc MONG MUỐN, không dùng mốc TỐI ĐA. Trước đây mỗi nhát cắt đặt
	// tại start+MaxClipDuration nên clip tự chia luôn dài sát trần (60s) — lệch hẳn so
	// với độ dài người dùng thực sự muốn. Chỉ khi tắt target mới quay về dùng max.
	step := cfg.TargetClipDuration
	if step <= 0 || step > cfg.MaxClipDuration {
		step = cfg.MaxClipDuration
	}

	var out []project.Clip
	for _, clip := range clips {
		// Bỏ qua clip chưa biết EndTime (totalDuration<=0, app.go sẽ xử lý).
		if clip.EndTime <= 0 || clip.Duration <= cfg.MaxClipDuration {
			out = append(out, clip)
			continue
		}

		start := clip.StartTime
		startMeta := scoredBoundary{
			tier: clip.Tier, confidence: clip.Confidence,
			reason: clip.Reason, signals: clip.Signals,
		}
		for clip.EndTime-start > cfg.MaxClipDuration {
			target := start + step
			cut := findSplitPointNear(candidates, start, clip.EndTime, target, cfg, w)
			if cut <= start {
				cut = target // không có candidate → cắt cứng theo thời gian
			}
			// Không để nhát cắt vượt trần (khi step=max và không tìm được candidate gần).
			if cut > start+cfg.MaxClipDuration {
				cut = start + cfg.MaxClipDuration
			}
			out = append(out, buildClip(start, cut, startMeta))
			start = cut
			startMeta = scoredBoundary{
				tier: project.TierReview, confidence: cfg.ReviewMinScore,
				reason: "Chia tự động (clip quá dài)", signals: []string{"auto-split"},
			}
		}
		// Đoạn còn lại.
		out = append(out, buildClip(start, clip.EndTime, startMeta))
	}
	return out
}

// findSplitPointNear tìm candidate mạnh nhất nằm gần mốc target trong [start, end],
// đảm bảo hai đoạn tạo ra vẫn >= MinClipDuration. Trả về 0 nếu không có ứng viên.
func findSplitPointNear(candidates []Candidate, start, end, target float64, cfg project.AnalyzerConfig, w project.SignalWeights) float64 {
	bestScore := -1
	bestTimestamp := 0.0

	// Cửa sổ tìm kiếm quanh mốc mong muốn. Lấy theo target (mốc thực tế đang nhắm) chứ
	// không theo MaxClipDuration: khi target nhỏ hơn max nhiều, cửa sổ tính theo max sẽ
	// rộng quá và kéo nhát cắt lệch xa mốc.
	window := (target - start) * 0.3
	if window <= 0 {
		window = cfg.MaxClipDuration * 0.3
	}

	for _, c := range candidates {
		if c.Timestamp <= start+cfg.MinClipDuration || c.Timestamp >= end-cfg.MinClipDuration {
			continue // giữ hai đoạn >= MinClipDuration
		}
		if abs(c.Timestamp-target) > window {
			continue
		}
		// Dùng trọng số ĐÃ hiệu chỉnh theo chế độ, không phải cfg.Weights thô — nếu không
		// thì điểm ở đây lệch hẳn so với điểm dùng ở bước chọn ranh giới chính.
		score, _ := boundaryScore(c, w)
		// Ưu tiên điểm cao và gần mốc target.
		proximity := int(window - abs(c.Timestamp-target))
		totalScore := score + proximity
		if totalScore > bestScore {
			bestScore = totalScore
			bestTimestamp = c.Timestamp
		}
	}

	return bestTimestamp
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
