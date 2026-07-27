package boundary

import (
	"testing"

	"video-splitter/internal/project"
)

// TestEffectiveScoringConfigByMode khóa bộ tham số chấm điểm riêng cho từng chế độ.
//
// Ngữ nghĩa đã được sửa lại cho khớp với TÊN chế độ. Bản cũ mọi phép điều chỉnh đều
// một chiều theo hướng NỚI LỎNG nên "Kỹ" là chế độ cắt nhiều nhất chứ không phải
// chính xác nhất. Nay: Nhanh và Kỹ đều THẮT ngưỡng, Tự động ở giữa.
func TestEffectiveScoringConfigByMode(t *testing.T) {
	tests := []struct {
		name              string
		mode              string
		wantContinuityPen float64
		wantReviewMin     int
		wantLenWeight     float64
	}{
		// Nhanh: chỉ có tín hiệu thô từ FFmpeg → nâng ngưỡng lên 45, giữ phạt mặc định,
		// và dựa nhiều hơn vào mốc độ dài (ít candidate nên cần mốc để clip đều).
		{name: "nhanh", mode: project.ModeFast, wantContinuityPen: 15, wantReviewMin: 45, wantLenWeight: 1.4},
		// Tự động: cân bằng — chỉ nới phạt liền mạch xuống 12 vì đã có SpeechBreak đo thật.
		{name: "tu dong", mode: project.ModeSmart, wantContinuityPen: 12, wantReviewMin: 35, wantLenWeight: 1.0},
		// Kỹ: NÂNG ngưỡng 40 và TĂNG phạt liền mạch lên 18 (ngược hẳn bản cũ), tin vào
		// tín hiệu hơn mốc độ dài vì candidate đã refine về fps gốc.
		{name: "ky", mode: project.ModePrecise, wantContinuityPen: 18, wantReviewMin: 40, wantLenWeight: 0.8},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := project.DefaultConfig()
			cfg.Mode = tt.mode

			weights, reviewMin, lenWeight := effectiveScoringConfig(cfg)
			if weights.ContinuityPen != tt.wantContinuityPen {
				t.Fatalf("ContinuityPen = %.0f, muốn %.0f", weights.ContinuityPen, tt.wantContinuityPen)
			}
			if reviewMin != tt.wantReviewMin {
				t.Fatalf("ReviewMinScore = %d, muốn %d", reviewMin, tt.wantReviewMin)
			}
			if lenWeight != tt.wantLenWeight {
				t.Fatalf("lenWeight = %.1f, muốn %.1f", lenWeight, tt.wantLenWeight)
			}
		})
	}
}

// TestOnlyAutoTierCuts là test quan trọng nhất của lần sửa này: ranh giới tier review
// (điểm nằm giữa ReviewMinScore và AutoAcceptScore) KHÔNG được tự cắt.
//
// Bản cũ chỉ loại tier reject nên ngưỡng cắt thực tế tụt từ AutoAcceptScore (60) về
// ReviewMinScore (35) — mọi điểm "còn ngờ ngợ" đều bị cắt dù không có UI nào để duyệt.
// Đây là gốc rễ của hiện tượng cắt quá nhạy.
func TestOnlyAutoTierCuts(t *testing.T) {
	cfg := project.DefaultConfig()
	cfg.Mode = project.ModeSmart
	cfg.MaxClipDuration = 0
	cfg.TargetClipDuration = 0

	// Visual 100 đơn độc, giọng nói xuyên qua: 45 - 12 = 33 → dưới cả reviewMin 35.
	// Visual 100 + Silence 40: 45 + 12 - 12 = 45 → tier review (35..60) → KHÔNG cắt.
	reviewOnly := Candidate{
		Timestamp: 30,
		Signals:   CandidateSignals{VisualChange: 100, Silence: 40, SpeechBreak: 0},
		Reason:    "Ranh giới mức trung bình (tier review)",
	}

	score, _ := boundaryScore(reviewOnly, mustWeights(t, cfg))
	if score < cfg.ReviewMinScore || score >= cfg.AutoAcceptScore {
		t.Fatalf("điểm = %d, cần nằm trong [%d, %d) để test đúng ý nghĩa tier review",
			score, cfg.ReviewMinScore, cfg.AutoAcceptScore)
	}

	clips := CalculateBoundaries([]Candidate{reviewOnly}, cfg, 90, "")
	if len(clips) != 1 {
		t.Fatalf("tier review tạo %d clip, muốn 1 (không được tự cắt)", len(clips))
	}
}

// TestStrongBoundaryStillCuts: ranh giới đủ mạnh (đạt AutoAcceptScore) vẫn phải cắt.
// Bảo vệ chiều ngược lại của TestOnlyAutoTierCuts — thắt ngưỡng không được làm
// mất luôn những ranh giới hiển nhiên.
func TestStrongBoundaryStillCuts(t *testing.T) {
	cfg := project.DefaultConfig()
	cfg.Mode = project.ModeSmart
	cfg.MaxClipDuration = 0
	cfg.TargetClipDuration = 0

	// Visual 100 + Black 100 + Silence 100 = 45+35+30 +10 (cooccur) → kẹp 100, không phạt
	// vì có black frame.
	strong := Candidate{
		Timestamp: 30,
		Signals:   CandidateSignals{VisualChange: 100, BlackFrame: 100, Silence: 100, SpeechBreak: 100},
		Reason:    "Chuyển cảnh rõ ràng: đổi hình + màn đen + im lặng",
	}

	clips := CalculateBoundaries([]Candidate{strong}, cfg, 90, "")
	if len(clips) != 2 {
		t.Fatalf("ranh giới mạnh tạo %d clip, muốn 2", len(clips))
	}
	if clips[0].EndTime != strong.Timestamp {
		t.Fatalf("điểm cắt = %.3f, muốn %.3f", clips[0].EndTime, strong.Timestamp)
	}
}

// TestAudioSoloDoesNotCut: âm thanh đứng MỘT MÌNH (nhịp/drop nhạc nền giữa clip)
// KHÔNG được tạo ranh giới ở BẤT KỲ chế độ nào — kể cả Kỹ.
func TestAudioSoloDoesNotCut(t *testing.T) {
	for _, mode := range []string{project.ModeFast, project.ModeSmart, project.ModePrecise} {
		t.Run(mode, func(t *testing.T) {
			cfg := project.DefaultConfig()
			cfg.Mode = mode
			cfg.MaxClipDuration = 0
			cfg.TargetClipDuration = 0

			candidate := Candidate{
				Timestamp: 30,
				Signals:   CandidateSignals{AudioChange: 100},
				Reason:    "Đổi nhạc nền (audio đơn độc)",
			}

			clips := CalculateBoundaries([]Candidate{candidate}, cfg, 90, "")
			if len(clips) != 1 {
				t.Fatalf("%s: audio đơn độc tạo %d clip, muốn 1 (không cắt)", mode, len(clips))
			}
		})
	}
}

// TestDPPrefersTargetDuration là test cho bộ chọn ranh giới tối ưu (DP).
//
// Kịch bản đúng hiện tượng người dùng báo: có ranh giới MẠNH ở giây thứ 10 và một
// ranh giới cũng mạnh ở giây 30. Với target 30s, thuật toán phải BỎ QUA điểm giây 10
// (cắt ở đó tạo clip 10s vụn) và chọn giây 30. Bản cũ duyệt tham lam nên cắt cả hai
// → ra 3 clip 10s/20s/30s, chính là "1 clip ngắn thành 2-3 khúc".
func TestDPPrefersTargetDuration(t *testing.T) {
	cfg := project.DefaultConfig()
	cfg.Mode = project.ModeSmart
	cfg.MinClipDuration = 8
	cfg.MaxClipDuration = 60
	cfg.TargetClipDuration = 30

	strong := CandidateSignals{VisualChange: 100, BlackFrame: 100, Silence: 100, SpeechBreak: 100}
	candidates := []Candidate{
		{Timestamp: 10, Signals: strong, Reason: "Ranh giới sớm (nên bỏ qua)"},
		{Timestamp: 30, Signals: strong, Reason: "Ranh giới đúng mốc mong muốn"},
	}

	clips := CalculateBoundaries(candidates, cfg, 60, "")
	if len(clips) != 2 {
		t.Fatalf("có %d clip, muốn 2 (bỏ qua ranh giới giây 10 để clip đều 30s)", len(clips))
	}
	if clips[0].EndTime != 30 {
		t.Fatalf("điểm cắt = %.1f, muốn 30 (mốc mong muốn)", clips[0].EndTime)
	}
}

// TestNoClipBelowMinDuration: sau toàn bộ pipeline, không clip nào được ngắn hơn
// MinClipDuration (trừ trường hợp video gốc vốn ngắn hơn thế).
func TestNoClipBelowMinDuration(t *testing.T) {
	cfg := project.DefaultConfig()
	cfg.Mode = project.ModeSmart
	cfg.MinClipDuration = 8
	cfg.MaxClipDuration = 60
	cfg.TargetClipDuration = 30

	strong := CandidateSignals{VisualChange: 100, BlackFrame: 100, Silence: 100, SpeechBreak: 100}
	// Chuỗi ranh giới mạnh dày đặc mỗi 2 giây — kiểu MV/gameplay edit nhanh.
	var candidates []Candidate
	for ts := 2.0; ts < 118; ts += 2 {
		candidates = append(candidates, Candidate{Timestamp: ts, Signals: strong, Reason: "edit nhanh"})
	}

	clips := CalculateBoundaries(candidates, cfg, 120, "")
	for _, c := range clips {
		if c.Duration < cfg.MinClipDuration {
			t.Fatalf("clip #%d dài %.2fs, dưới mức tối thiểu %.1fs", c.Index, c.Duration, cfg.MinClipDuration)
		}
	}
}

// TestLongVideoSplitsAtTargetNotMax: clip tự chia (video liên tục không có ranh giới)
// phải chia quanh mốc MONG MUỐN, không phải sát trần MaxClipDuration. Bản cũ đặt mỗi
// nhát cắt tại start+MaxClipDuration nên clip tự chia luôn dài 60s.
func TestLongVideoSplitsAtTargetNotMax(t *testing.T) {
	cfg := project.DefaultConfig()
	cfg.Mode = project.ModeSmart
	cfg.MinClipDuration = 8
	cfg.MaxClipDuration = 60
	cfg.TargetClipDuration = 30

	// Không có candidate nào → toàn bộ video là một clip, buộc splitLongClips xử lý.
	clips := CalculateBoundaries(nil, cfg, 300, "")
	if len(clips) < 2 {
		t.Fatalf("video 300s không có ranh giới ra %d clip, muốn nhiều clip tự chia", len(clips))
	}
	for _, c := range clips {
		if c.Duration > cfg.MaxClipDuration+0.01 {
			t.Fatalf("clip #%d dài %.2fs, vượt trần %.1fs", c.Index, c.Duration, cfg.MaxClipDuration)
		}
	}
	// Nhát cắt đầu phải ở mốc mong muốn 30s, không phải 60s.
	if clips[0].Duration > cfg.TargetClipDuration+0.01 {
		t.Fatalf("clip đầu dài %.2fs, muốn ~%.0fs (mốc mong muốn, không phải trần)",
			clips[0].Duration, cfg.TargetClipDuration)
	}
}

// TestProminenceFiltersDenseNoise: trong vùng có rất nhiều ranh giới điểm ngang nhau
// (MV ca nhạc, gameplay), bộ lọc độ trội phải giảm số điểm giữ lại — ngưỡng tuyệt đối
// không phân biệt được đâu là ranh giới nội dung, đâu là nhịp dựng bình thường.
func TestProminenceFiltersDenseNoise(t *testing.T) {
	var dense []scoredBoundary
	for ts := 0.0; ts < 60; ts += 2 {
		dense = append(dense, scoredBoundary{timestamp: ts, score: 50})
	}
	// Một đỉnh thật trội hơn hẳn.
	dense = append(dense, scoredBoundary{timestamp: 61, score: 95})

	out := filterByProminence(dense, 15, 0.5, 90)
	if len(out) >= len(dense) {
		t.Fatalf("lọc độ trội giữ %d/%d điểm, phải giảm bớt điểm nhiễu dày đặc", len(out), len(dense))
	}
	// Đỉnh 95 (>= keepAbove 90) luôn phải được giữ.
	found := false
	for _, b := range out {
		if b.timestamp == 61 {
			found = true
		}
	}
	if !found {
		t.Fatal("đỉnh mạnh (95) bị lọc mất, phải luôn được giữ")
	}
}

// mustWeights lấy trọng số đã hiệu chỉnh theo chế độ để test tính điểm khớp với
// điểm mà CalculateBoundaries thực sự dùng.
func mustWeights(t *testing.T, cfg project.AnalyzerConfig) project.SignalWeights {
	t.Helper()
	w, _, _ := effectiveScoringConfig(cfg)
	return w
}
