package boundary

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"video-splitter/internal/media"
	"video-splitter/internal/project"
	"video-splitter/internal/utils"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// CandidateSignals chứa cường độ (0-100) của từng loại tín hiệu do Python phát hiện.
type CandidateSignals struct {
	VisualChange int `json:"visual_change"`
	BlackFrame   int `json:"black_frame"`
	Silence      int `json:"silence"`
	LayoutChange int `json:"layout_change"`
	AudioChange  int `json:"audio_change"`
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
		"--source", sourcePath,
		"--ffmpeg", utils.GetBinPath("ffmpeg"),
		"--mode", mode,
		"--scene-threshold", fmt.Sprintf("%.1f", cfg.SceneThreshold),
		"--silence-db", fmt.Sprintf("%.0f", cfg.SilenceThreshold),
		"--silence-duration", fmt.Sprintf("%.2f", cfg.SilenceDuration),
	}

	// Ưu tiên worker.exe đã đóng gói (PyInstaller) — user không cần cài Python.
	// Fallback: chạy script bằng python (khi dev).
	var exePath string
	var cmdArgs []string
	if workerExe := utils.GetWorkerExe(); workerExe != "" {
		exePath = workerExe
		cmdArgs = analyzeArgs
	} else {
		exePath = pythonExe
		cmdArgs = append([]string{utils.GetWorkerScript()}, analyzeArgs...)
	}

	// CommandContext để có thể hủy; Cancel kill cả cây tiến trình (worker + ffmpeg con)
	cmd := exec.CommandContext(ctx, exePath, cmdArgs...)
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		// Trên Windows, taskkill /T kill cả tiến trình con (ffmpeg do python spawn)
		return exec.Command("taskkill", "/F", "/T", "/PID", strconv.Itoa(cmd.Process.Pid)).Run()
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("lỗi khởi tạo stdout pipe: %v", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("lỗi khởi động python worker: %v", err)
	}

	var jsonOutput strings.Builder
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "PROGRESS:") {
			parts := strings.Split(line, ":")
			if len(parts) == 2 {
				prog, _ := strconv.Atoi(parts[1])
				// Map progress từ python (0-100) vào khoảng còn lại (15-95)
				realProg := 15 + (prog * 80 / 100)
				runtime.EventsEmit(ctx, "analyze_progress", map[string]interface{}{"path": sourcePath, "progress": realProg})
			}
		} else {
			// Thu thập json output
			jsonOutput.WriteString(line)
		}
	}

	if err := cmd.Wait(); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("python worker kết thúc có lỗi: %v", err)
	}

	runtime.EventsEmit(ctx, "analyze_progress", map[string]interface{}{"path": sourcePath, "progress": 95})

	var result AnalyzerResult
	if err := json.Unmarshal([]byte(jsonOutput.String()), &result); err != nil {
		return nil, fmt.Errorf("lỗi parse json từ python: %v, output: %s", err, jsonOutput.String())
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
func boundaryScore(c Candidate, w project.SignalWeights) (int, []string) {
	var score float64
	var signals []string

	hasVisual := c.Signals.VisualChange > 0
	hasBlack := c.Signals.BlackFrame > 0
	hasSilence := c.Signals.Silence > 0
	hasLayout := c.Signals.LayoutChange > 0
	hasAudio := c.Signals.AudioChange > 0

	if hasVisual {
		score += w.VisualChange
		signals = append(signals, "visual")
	}
	if hasBlack {
		score += w.BlackFrame
		signals = append(signals, "black")
	}
	if hasSilence {
		score += w.Silence
		signals = append(signals, "silence")
	}
	if hasLayout {
		score += w.LayoutChange
		signals = append(signals, "layout")
	}
	if hasAudio {
		score += w.AudioChange
		signals = append(signals, "audio")
	}

	// Continuity Penalty: có chuyển cảnh hình ảnh nhưng không có bất kỳ dấu hiệu
	// ngắt mạch nào (im lặng / đổi âm thanh / màn hình đen) → nhiều khả năng chỉ là
	// đổi góc quay trong cùng một video. Trừ điểm để tránh cắt nhầm.
	audioBreak := hasSilence || hasAudio
	if hasVisual && !audioBreak && !hasBlack {
		score -= w.ContinuityPen
		signals = append(signals, "continuity-penalty")
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

// CalculateBoundaries tính Boundary Score cho từng candidate, loại bỏ điểm dưới ngưỡng,
// gom cụm điểm gần nhau (giữ điểm mạnh nhất), rồi dựng danh sách clip kèm confidence/tier/reason.
// sourcePath dùng để snap boundary sang keyframe gần nhất (tối ưu cho stream-copy).
func CalculateBoundaries(candidates []Candidate, cfg project.AnalyzerConfig, totalDuration float64, sourcePath string) []project.Clip {
	w := cfg.Weights

	// === BƯỚC 1: Tính Boundary Score, loại điểm dưới ngưỡng review (tier reject) ===
	var scored []scoredBoundary
	for _, c := range candidates {
		score, signals := boundaryScore(c, w)
		tier := tierFor(score, cfg)
		if tier == project.TierReject {
			continue // dưới ReviewMinScore → không dùng làm ranh giới
		}
		scored = append(scored, scoredBoundary{
			timestamp:  c.Timestamp,
			score:      score,
			confidence: score,
			tier:       tier,
			reason:     c.Reason,
			signals:    signals,
		})
	}

	sort.Slice(scored, func(i, j int) bool {
		return scored[i].timestamp < scored[j].timestamp
	})

	// === BƯỚC 1.5: Snap boundary sang keyframe gần nhất (nếu biết sourcePath) ===
	// Điều này đảm bảo điểm cắt nằm tại I-frame, giúp stream-copy không bị artifact
	// (xanh lá, glitch) ở đầu clip. Chỉ snap khi lệch < 1s để giữ ý định phân tích.
	if sourcePath != "" {
		for i := range scored {
			snapped := media.FindNearestKeyframe(sourcePath, scored[i].timestamp)
			if abs(snapped-scored[i].timestamp) < 1.0 {
				scored[i].timestamp = snapped
			}
		}
	}

	// === BƯỚC 2: Gom cụm — giữ boundary điểm cao nhất trong mỗi cửa sổ MinClipDuration ===
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
		clips = splitLongClips(clips, candidates, cfg)
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
	for i := range clips {
		clips[i].ID = fmt.Sprintf("clip_%d", i+1)
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
func splitLongClips(clips []project.Clip, candidates []Candidate, cfg project.AnalyzerConfig) []project.Clip {
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
			target := start + cfg.MaxClipDuration
			cut := findSplitPointNear(candidates, start, clip.EndTime, target, cfg)
			if cut <= start {
				cut = target // không có candidate → cắt cứng theo thời gian
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
func findSplitPointNear(candidates []Candidate, start, end, target float64, cfg project.AnalyzerConfig) float64 {
	bestScore := -1
	bestTimestamp := 0.0

	for _, c := range candidates {
		if c.Timestamp <= start+cfg.MinClipDuration || c.Timestamp >= end-cfg.MinClipDuration {
			continue // giữ hai đoạn >= MinClipDuration
		}
		// Chỉ xét candidate quanh mốc mong muốn (±30% MaxClipDuration) để đoạn đều nhau.
		window := cfg.MaxClipDuration * 0.3
		if abs(c.Timestamp-target) > window {
			continue
		}
		score, _ := boundaryScore(c, cfg.Weights)
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
