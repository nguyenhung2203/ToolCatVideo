package project

type VideoInfo struct {
	SourcePath string
	Duration   float64
	Width      int
	Height     int
	FPS        float64
	TimeBase   string
	SizeByte   int64 `json:"sizeByte"`
}

// Boundary tier: mức phân loại điểm ranh giới theo độ tin cậy.
const (
	TierAuto   = "auto"   // >= AutoAcceptScore: tự động chấp nhận
	TierReview = "review" // trong khoảng [ReviewMinScore, AutoAcceptScore): cần người dùng duyệt
	TierReject = "reject" // < ReviewMinScore: loại bỏ
)

// Các chế độ phân tích.
const (
	ModeFast    = "fast"    // Tách nhanh: chủ yếu màn hình đen + im lặng
	ModeSmart   = "smart"   // Tách thông minh (mặc định): kết hợp hình ảnh + âm thanh + layout
	ModePrecise = "precise" // Chính xác cao: thêm phổ âm thanh, lấy mẫu dày hơn
	ModeFixed   = "fixed"   // Cắt đều theo thời lượng cố định
)

type Clip struct {
	ID         string   `json:"id"`
	Index      int      `json:"index"`
	StartTime  float64  `json:"startTime"`
	EndTime    float64  `json:"endTime"`
	Duration   float64  `json:"duration"`
	Status     string   `json:"status"` // pending, processing, completed, error
	Thumbnail  string   `json:"thumbnail"`
	ThumbEnd   string   `json:"thumbEnd"`   // thumbnail ngay trước điểm cắt cuối clip
	Confidence int      `json:"confidence"` // điểm tin cậy của ranh giới BẮT ĐẦU clip (0-100)
	Tier       string   `json:"tier"`       // auto / review / reject
	Reason     string   `json:"reason"`     // lý do xác định ranh giới
	Signals    []string `json:"signals"`    // các tín hiệu đã phát hiện
	Edit       EditOps  `json:"edit"`       // các thao tác chỉnh sửa áp cho clip khi xuất
}

// === EDIT OPERATIONS ===
// EditOps chứa toàn bộ thao tác chỉnh sửa áp cho một clip lúc xuất. Mỗi nhóm
// có thể tắt (giá trị zero = không áp) để export builder chỉ dựng filter khi cần.
// Đây là điểm bản lề cho các giai đoạn edit (tỷ lệ, màu, tốc độ, text, audio, transition).
type EditOps struct {
	Aspect      AspectOp     `json:"aspect"`      // chuyển tỷ lệ khung (9:16 / 1:1 / 16:9)
	Color       ColorOp      `json:"color"`       // chỉnh màu + preset filter
	Speed       float64      `json:"speed"`       // hệ số tốc độ (1.0 = giữ nguyên; >1 nhanh, <1 chậm)
	HFlip       bool         `json:"hflip"`       // lật ngang video (tránh bản quyền)
	Texts       []TextOp     `json:"texts"`       // overlay chữ / phụ đề
	Watermark   WatermarkOp  `json:"watermark"`   // overlay logo / watermark ảnh
	Audio       AudioOp      `json:"audio"`       // âm lượng, nhạc nền, fade
	Transition  TransitionOp `json:"transition"`  // hiệu ứng chuyển vào đầu clip (khi ghép)
}

// AspectOp mô tả cách chuyển tỷ lệ khung hình.
type AspectOp struct {
	Enabled bool   `json:"enabled"`
	Ratio   string `json:"ratio"` // "9:16" / "1:1" / "16:9"
	Mode    string `json:"mode"`  // "crop" (cắt) / "pad" (viền đen) / "blur" (nền mờ)
}

// ColorOp chỉnh màu cơ bản + preset filter.
type ColorOp struct {
	Enabled    bool    `json:"enabled"`
	Brightness float64 `json:"brightness"` // -1.0 .. 1.0 (0 = giữ nguyên)
	Contrast   float64 `json:"contrast"`   // -2.0 .. 2.0 (1 = giữ nguyên trong ffmpeg eq; ta chuẩn hóa 0 = giữ nguyên)
	Saturation float64 `json:"saturation"` // 0 .. 3.0 (1 = giữ nguyên)
	Preset     string  `json:"preset"`     // "" / "warm" / "cool" / "vivid" / "bw" / "vintage"
}

// TextOp mô tả một overlay chữ (tiêu đề, phụ đề, caption).
type TextOp struct {
	Content   string  `json:"content"`
	FontSize  int     `json:"fontSize"`
	Color     string  `json:"color"`     // mã màu, ví dụ "white", "#ffcc00"
	X         string  `json:"x"`         // biểu thức vị trí ffmpeg ("(w-text_w)/2" = giữa ngang)
	Y         string  `json:"y"`         // biểu thức vị trí ffmpeg
	StartTime float64 `json:"startTime"` // thời điểm hiện chữ (giây, trong clip); 0 = từ đầu
	EndTime   float64 `json:"endTime"`   // thời điểm ẩn chữ; 0 = tới cuối clip
	BgBox     bool    `json:"bgBox"`     // nền hộp mờ phía sau chữ cho dễ đọc
}

// WatermarkOp overlay một ảnh logo / watermark.
type WatermarkOp struct {
	Enabled  bool    `json:"enabled"`
	ImgPath  string  `json:"imgPath"`
	X        string  `json:"x"`
	Y        string  `json:"y"`
	Opacity  float64 `json:"opacity"` // 0..1
	Scale    float64 `json:"scale"`   // hệ số scale so với gốc (1 = giữ nguyên)
}

// AudioOp điều chỉnh âm thanh clip.
type AudioOp struct {
	Volume       float64 `json:"volume"`       // hệ số âm lượng (1 = giữ nguyên)
	Mute         bool    `json:"mute"`         // tắt tiếng gốc
	MusicPath    string  `json:"musicPath"`    // đường dẫn nhạc nền; "" = không
	MusicVolume  float64 `json:"musicVolume"`  // âm lượng nhạc nền (1 = giữ nguyên)
	FadeIn       float64 `json:"fadeIn"`       // fade in (giây)
	FadeOut      float64 `json:"fadeOut"`      // fade out (giây)
}

// TransitionOp hiệu ứng chuyển cảnh vào đầu clip (dùng khi ghép nhiều clip).
type TransitionOp struct {
	Type     string  `json:"type"`     // "" / "fade" / "slide" / "wipe" ...
	Duration float64 `json:"duration"` // thời lượng transition (giây)
}

// DefaultEditOps trả về EditOps trung tính (không áp thao tác nào).
func DefaultEditOps() EditOps {
	return EditOps{
		Speed: 1.0,
		Color: ColorOp{Saturation: 1.0},
		Audio: AudioOp{Volume: 1.0, MusicVolume: 1.0},
	}
}

// === PROJECT (lưu SQLite) ===
// Project là một phiên làm việc trên một video nguồn: metadata + config + danh sách clip.
// Lưu để mở lại app không mất việc.
type Project struct {
	ID         string         `json:"id"`
	SourcePath string         `json:"sourcePath"`
	Name       string         `json:"name"`
	Duration   float64        `json:"duration"`
	Width      int            `json:"width"`
	Height     int            `json:"height"`
	FPS        float64        `json:"fps"`
	Status     string         `json:"status"` // new / analyzed / exported
	Config     AnalyzerConfig `json:"config"`
	Clips      []Clip         `json:"clips"`
	CreatedAt  int64          `json:"createdAt"` // Unix epoch giây
	UpdatedAt  int64          `json:"updatedAt"`
}

// SignalWeights chứa trọng số cho từng loại tín hiệu khi tính Boundary Score.
// Có thể cấu hình để điều chỉnh theo dữ liệu thực tế.
type SignalWeights struct {
	VisualChange   float64 `json:"visualChange"`   // chuyển cảnh nội dung
	BlackFrame     float64 `json:"blackFrame"`     // màn hình đen / fade
	Silence        float64 `json:"silence"`        // khoảng im lặng
	LayoutChange   float64 `json:"layoutChange"`   // thay đổi bố cục / tỷ lệ vùng nội dung
	AudioChange    float64 `json:"audioChange"`    // thay đổi phổ / môi trường âm thanh
	ContinuityPen  float64 `json:"continuityPen"`  // phạt khi có dấu hiệu liền mạch (giọng nói, nhạc nền tiếp tục)
}

// AnalyzerConfig chứa tham số cấu hình cho thuật toán phân tích chuyển cảnh
type AnalyzerConfig struct {
	Mode             string        `json:"mode"`             // fast / smart / precise — mặc định smart
	SceneThreshold   float64       `json:"sceneThreshold"`   // Ngưỡng nhạy scene detection (ContentDetector) — mặc định 27.0
	MinClipDuration  float64       `json:"minClipDuration"`  // Độ dài tối thiểu 1 clip (giây) — mặc định 5.0
	MaxClipDuration  float64       `json:"maxClipDuration"`  // Độ dài tối đa 1 clip (giây), vượt thì tự chia — mặc định 120.0
	AutoAcceptScore  int           `json:"autoAcceptScore"`  // Ngưỡng điểm tự động chấp nhận ranh giới — mặc định 85
	ReviewMinScore   int           `json:"reviewMinScore"`   // Ngưỡng điểm tối thiểu để cần duyệt — mặc định 60
	SilenceThreshold float64       `json:"silenceThreshold"` // Ngưỡng dB cho silence — mặc định -30
	SilenceDuration  float64       `json:"silenceDuration"`  // Thời lượng tối thiểu silence (giây) — mặc định 0.5
	ProxyFPS         int           `json:"proxyFPS"`         // FPS proxy video — mặc định 15
	Weights          SignalWeights `json:"weights"`          // Trọng số từng tín hiệu
	ExportPreset     string        `json:"exportPreset"`     // FFmpeg preset (ultrafast/fast/medium) — mặc định "fast"
	ExportCRF        int           `json:"exportCRF"`        // Chất lượng xuất (0-51, thấp = tốt hơn) — mặc định 23
}

// DefaultWeights trả về trọng số tín hiệu mặc định.
// Đã hiệu chỉnh lại so với phiên bản cũ: tăng trọng số visual/silence,
// giảm continuity penalty để không loại nhầm quá nhiều điểm cắt hợp lệ.
//
// Ví dụ tính điểm với trọng số mới:
//   Visual only (không liền mạch): 40 → review
//   Visual + Silence:              40 + 30 = 70 → auto accept ✓
//   Visual + Black:                40 + 35 = 75 → auto accept ✓
//   Silence + Black:               30 + 35 = 65 → auto accept ✓
//   Visual only (liền mạch):       40 - 15 = 25 → vẫn review (trước đây bị reject!)
func DefaultWeights() SignalWeights {
	return SignalWeights{
		VisualChange:  55,
		BlackFrame:    35,
		Silence:       30,
		LayoutChange:  25,
		AudioChange:   20,
		ContinuityPen: 5,
	}
}

// DefaultConfig trả về cấu hình mặc định
func DefaultConfig() AnalyzerConfig {
	return AnalyzerConfig{
		Mode:             ModeSmart,
		SceneThreshold:   15.0,
		MinClipDuration:  5.0,
		MaxClipDuration:  120.0,
		AutoAcceptScore:  50, // visual-only scene change (55 - 5 = 50) sẽ được auto-accept
		ReviewMinScore:   30, // giảm từ 60: giữ lại nhiều candidate hơn để không sót
		SilenceThreshold: -30,
		SilenceDuration:  0.5,
		ProxyFPS:         15,
		Weights:          DefaultWeights(),
		ExportPreset:     "fast",
		ExportCRF:        23,
	}
}

