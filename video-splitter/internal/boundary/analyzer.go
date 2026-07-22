package boundary

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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
	if nAudio > 0 {
		score += w.AudioChange * nAudio
		signals = append(signals, "audio")
		families++
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
	// Mục đích: đảm bảo điểm cắt nằm tại I-frame để stream-copy không bị artifact.
	// Quy tắc snap theo mode:
	//   - fast:    snap tối đa 2.0s (độ chính xác thấp hơn, ưu tiên tốc độ xuất)
	//   - smart:   snap tối đa 0.3s — refine trên source (fps gốc) đã cho điểm cắt
	//     chính xác ~1/fps; chỉ snap nhẹ để bám I-frame gần nhất, KHÔNG kéo lệch >0.3s
	//     phá lại độ chính xác vừa refine.
	//   - precise: KHÔNG snap — AI đã phát hiện chính xác đến mili-giây, giữ nguyên
	snapLimit := 0.3
	switch cfg.Mode {
	case project.ModeFast:
		snapLimit = 2.0
	case project.ModePrecise:
		snapLimit = 0.0 // tắt snap hoàn toàn cho precise
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
