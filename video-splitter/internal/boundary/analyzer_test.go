package boundary

import (
	"testing"

	"video-splitter/internal/project"
)

// TestEffectiveScoringConfigByMode khóa bộ tham số chấm điểm riêng cho từng chế độ.
// Giá trị kỳ vọng bám đúng logic trong effectiveScoringConfig (không phải mặc định).
func TestEffectiveScoringConfigByMode(t *testing.T) {
	tests := []struct {
		name              string
		mode              string
		wantVisual        float64
		wantContinuityPen float64
		wantReviewMin     int
	}{
		// Nhanh: nâng visual để hard-cut FFmpeg được giữ, hạ phạt liền mạch, hạ ngưỡng.
		{name: "nhanh", mode: project.ModeFast, wantVisual: 50, wantContinuityPen: 10, wantReviewMin: 25},
		// Tự động (lai): giữ visual mặc định 45, chỉ hạ phạt liền mạch xuống 10, ngưỡng 35.
		{name: "tu dong", mode: project.ModeSmart, wantVisual: 45, wantContinuityPen: 10, wantReviewMin: 35},
		// Kỹ: nâng mạnh visual/layout/audio, hạ phạt liền mạch còn 5, hạ ngưỡng.
		{name: "ky", mode: project.ModePrecise, wantVisual: 55, wantContinuityPen: 5, wantReviewMin: 25},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := project.DefaultConfig()
			cfg.Mode = tt.mode

			weights, reviewMin := effectiveScoringConfig(cfg)
			if weights.VisualChange != tt.wantVisual {
				t.Fatalf("VisualChange = %.0f, muốn %.0f", weights.VisualChange, tt.wantVisual)
			}
			if weights.ContinuityPen != tt.wantContinuityPen {
				t.Fatalf("ContinuityPen = %.0f, muốn %.0f", weights.ContinuityPen, tt.wantContinuityPen)
			}
			if reviewMin != tt.wantReviewMin {
				t.Fatalf("ReviewMinScore = %d, muốn %d", reviewMin, tt.wantReviewMin)
			}
		})
	}
}

// TestMediumVisualOnlyBoundaryByMode: chuyển cảnh chỉ có hình ảnh mức vừa (75/100),
// giọng nói xuyên qua (SpeechBreak=0). Tự động PHẢI loại (tránh cắt nhầm đổi góc quay),
// còn Nhanh và Kỹ giữ lại (ưu tiên không bỏ sót ranh giới).
func TestMediumVisualOnlyBoundaryByMode(t *testing.T) {
	candidate := Candidate{
		Timestamp: 10,
		Signals:   CandidateSignals{VisualChange: 75, SpeechBreak: 0},
		Reason:    "Hard cut chỉ có hình ảnh (mức vừa)",
	}

	tests := []struct {
		name      string
		mode      string
		wantClips int
	}{
		{name: "tu dong loai scene vua", mode: project.ModeSmart, wantClips: 1},
		{name: "nhanh giu hard cut", mode: project.ModeFast, wantClips: 2},
		{name: "ky khong bo sot", mode: project.ModePrecise, wantClips: 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := project.DefaultConfig()
			cfg.Mode = tt.mode
			cfg.MaxClipDuration = 0 // không chia clip dài để cô lập hành vi lọc

			clips := CalculateBoundaries([]Candidate{candidate}, cfg, 30, "")
			if len(clips) != tt.wantClips {
				t.Fatalf("có %d clip, muốn %d", len(clips), tt.wantClips)
			}
			if tt.wantClips == 2 && clips[0].EndTime != candidate.Timestamp {
				t.Fatalf("điểm cắt = %.3f, muốn %.3f", clips[0].EndTime, candidate.Timestamp)
			}
		})
	}
}

// TestSmartKeepsVeryStrongHardCut: hard cut hình ảnh CỰC RÕ (100/100) phải được
// Tự động giữ lại dù giọng nói xuyên qua — đây là tính "lai" của chế độ này.
func TestSmartKeepsVeryStrongHardCut(t *testing.T) {
	cfg := project.DefaultConfig()
	cfg.Mode = project.ModeSmart
	cfg.MaxClipDuration = 0

	candidate := Candidate{
		Timestamp: 15,
		Signals:   CandidateSignals{VisualChange: 100, SpeechBreak: 0},
		Reason:    "Hard cut cực rõ",
	}

	clips := CalculateBoundaries([]Candidate{candidate}, cfg, 30, "")
	if len(clips) != 2 {
		t.Fatalf("Tự động có %d clip, muốn 2 (phải giữ hard cut cực rõ)", len(clips))
	}
}

// TestAudioSoloDoesNotCut: âm thanh đứng MỘT MÌNH (nhịp/drop nhạc nền giữa clip)
// KHÔNG được tạo ranh giới ở BẤT KỲ chế độ nào — kể cả Kỹ. Đây là fix trị gốc hiện
// tượng cắt quá nhạy: nhạc TikTok beat mạnh từng băm một clip thành nhiều mảnh.
func TestAudioSoloDoesNotCut(t *testing.T) {
	for _, mode := range []string{project.ModeFast, project.ModeSmart, project.ModePrecise} {
		t.Run(mode, func(t *testing.T) {
			cfg := project.DefaultConfig()
			cfg.Mode = mode
			cfg.MaxClipDuration = 0

			candidate := Candidate{
				Timestamp: 12.5,
				Signals:   CandidateSignals{AudioChange: 100},
				Reason:    "Đổi nhạc nền (audio đơn độc)",
			}

			clips := CalculateBoundaries([]Candidate{candidate}, cfg, 30, "")
			if len(clips) != 1 {
				t.Fatalf("%s: audio đơn độc tạo %d clip, muốn 1 (không cắt)", mode, len(clips))
			}
		})
	}
}

// TestAudioWithCorroborationCuts: âm thanh KÈM một tín hiệu khác (đổi hình / im lặng)
// vẫn tạo ranh giới bình thường — ranh giới thật trong video nói liên tục luôn có
// khoảng lặng giọng nói hoặc đổi cảnh đi kèm, nên không bị chặn nhầm.
func TestAudioWithCorroborationCuts(t *testing.T) {
	cfg := project.DefaultConfig()
	cfg.Mode = project.ModePrecise
	cfg.MaxClipDuration = 0

	candidate := Candidate{
		Timestamp: 12.5,
		Signals:   CandidateSignals{AudioChange: 100, Silence: 80},
		Reason:    "Đổi âm thanh + khoảng lặng",
	}

	clips := CalculateBoundaries([]Candidate{candidate}, cfg, 30, "")
	if len(clips) != 2 {
		t.Fatalf("Kỹ (audio+silence) có %d clip, muốn 2", len(clips))
	}
	if clips[0].EndTime != candidate.Timestamp {
		t.Fatalf("điểm cắt = %.3f, muốn %.3f", clips[0].EndTime, candidate.Timestamp)
	}
}
