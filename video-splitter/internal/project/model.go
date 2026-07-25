package project

type VideoInfo struct {
	SourcePath string
	Duration   float64
	Width      int
	Height     int
	FPS        float64
	TimeBase   string
	SizeByte   int64 `json:"sizeByte"`
	// HasAudio: có ít nhất 1 audio stream. Dùng để bỏ nhánh phân tích audio khi video
	// không tiếng (tránh trích WAV rỗng). VFR/rotation/start_time KHÔNG cần field riêng:
	// refine-source dùng pts_time thật (đúng cho VFR), ffmpeg tự autorotate, và toàn bộ
	// timeline phân tích lẫn cắt đều 0-based nên start_time≠0 không gây lệch.
	HasAudio bool `json:"hasAudio"`
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
	ID           string   `json:"id"`
	Index        int      `json:"index"`
	StartTime    float64  `json:"startTime"`
	EndTime      float64  `json:"endTime"`
	Duration     float64  `json:"duration"`
	Status       string   `json:"status"` // pending, processing, completed, error
	Thumbnail    string   `json:"thumbnail"`
	ThumbEnd     string   `json:"thumbEnd"`     // thumbnail ngay trước điểm cắt cuối clip
	Confidence   int      `json:"confidence"`   // điểm tin cậy của ranh giới BẮT ĐẦU clip (0-100)
	Tier         string   `json:"tier"`         // auto / review / reject
	Reason       string   `json:"reason"`       // lý do xác định ranh giới
	Signals      []string `json:"signals"`      // các tín hiệu đã phát hiện
	Edit         EditOps  `json:"edit"`         // các thao tác chỉnh sửa áp cho clip khi xuất
	ExportedPath string   `json:"exportedPath"` // đường dẫn video đã xuất để phát ngay

	// Segments: biên gốc các clip con của một CLIP GHÉP (dải liên tục trên timeline).
	// Rỗng = clip thường. Có phần tử = clip ghép: StartTime=seg[0].start, EndTime=seg[last].end.
	// Giữ lại để HỦY GHÉP (tách về từng clip con). Khi xuất chỉ cắt một lần theo [StartTime,EndTime].
	Segments []ClipSegment `json:"segments"`
}

// ClipSegment: một đoạn con của clip ghép (biên gốc trước khi ghép), để hủy ghép được.
type ClipSegment struct {
	StartTime  float64 `json:"startTime"`
	EndTime    float64 `json:"endTime"`
	Confidence int     `json:"confidence"`
	Tier       string  `json:"tier"`
	Reason     string  `json:"reason"`
	ThumbEnd   string  `json:"thumbEnd"`
}

// === EDIT OPERATIONS ===
// EditOps chứa toàn bộ thao tác chỉnh sửa áp cho một clip lúc xuất. Mỗi nhóm
// có thể tắt (giá trị zero = không áp) để export builder chỉ dựng filter khi cần.
// Đây là điểm bản lề cho các giai đoạn edit (tỷ lệ, màu, tốc độ, text, audio, transition).
type EditOps struct {
	Aspect     AspectOp     `json:"aspect"`     // chuyển tỷ lệ khung (9:16 / 1:1 / 16:9)
	Color      ColorOp      `json:"color"`      // chỉnh màu + preset filter
	Speed      float64      `json:"speed"`      // hệ số tốc độ (1.0 = giữ nguyên; >1 nhanh, <1 chậm)
	HFlip      bool         `json:"hflip"`      // lật ngang video (tránh bản quyền)
	Texts      []TextOp     `json:"texts"`      // overlay chữ / phụ đề
	Watermark  WatermarkOp  `json:"watermark"`  // overlay logo / watermark ảnh
	Card       CardOp       `json:"card"`       // overlay khung nền card / banner
	Audio      AudioOp      `json:"audio"`      // âm lượng, nhạc nền, fade
	Transition TransitionOp `json:"transition"` // hiệu ứng chuyển vào đầu clip (khi ghép)

	// === Nhóm "xào nấu" chống trùng lặp (né fingerprint FB/TikTok/YouTube) ===
	ZoomPan   ZoomPanOp  `json:"zoomPan"`   // Ken Burns: phóng to + dịch chậm suốt clip
	Crop      CropOp     `json:"crop"`      // cắt rìa % rồi scale lại (lệch bố cục pixel)
	Rotate    RotateOp   `json:"rotate"`    // xoay nhẹ vài độ + zoom bù lấp góc đen
	Noise     NoiseOp    `json:"noise"`     // thêm hạt grain (đổi đặc trưng nén)
	TrimStart float64    `json:"trimStart"` // cắt bỏ N giây ĐẦU clip (0 = không)
	TrimEnd   float64    `json:"trimEnd"`   // cắt bỏ N giây CUỐI clip (0 = không)
	Pitch     float64    `json:"pitch"`     // đổi cao độ giọng theo semitone (0 = giữ nguyên, ±1..±3)
	Subtitle  SubtitleOp `json:"subtitle"`  // burn phụ đề SRT vào video
	StripMeta bool       `json:"stripMeta"` // xóa toàn bộ metadata (title/encoder/creation_time)
}

// ZoomPanOp mô tả hiệu ứng Ken Burns (phóng to + dịch chuyển chậm) để đổi từng khung
// hình, né fingerprint mạnh hơn lật ngang.
type ZoomPanOp struct {
	Enabled bool    `json:"enabled"`
	Zoom    float64 `json:"zoom"` // hệ số zoom cuối clip (VD 1.08 = phóng to 8%)
	Dir     string  `json:"dir"`  // hướng dịch: "in" / "out" / "left" / "right" / "up" / "down"
}

// CropOp cắt bỏ một tỷ lệ mép quanh khung rồi scale lại full, làm lệch bố cục pixel.
type CropOp struct {
	Enabled bool    `json:"enabled"`
	Percent float64 `json:"percent"` // % cắt mỗi mép (VD 0.04 = cắt 4% quanh viền)
}

// RotateOp xoay khung một góc nhỏ (độ) + zoom bù để không lộ góc đen.
type RotateOp struct {
	Enabled bool    `json:"enabled"`
	Degrees float64 `json:"degrees"` // góc xoay (VD 1.5 hoặc -1.5)
}

// NoiseOp thêm hạt nhiễu nhẹ lên khung hình.
type NoiseOp struct {
	Enabled  bool `json:"enabled"`
	Strength int  `json:"strength"` // cường độ nhiễu (ffmpeg alls, khoảng 5..30)
}

// SubtitleOp burn phụ đề từ file .srt/.ass vào video (hardsub).
type SubtitleOp struct {
	Enabled    bool   `json:"enabled"`
	Path       string `json:"path"`       // đường dẫn file .srt / .ass
	FontSize   int    `json:"fontSize"`   // cỡ chữ (mặc định 24)
	FontColor  string `json:"fontColor"`  // màu chữ dạng "&HBBGGRR" hoặc tên; rỗng = trắng
	OutlineCol string `json:"outlineCol"` // màu viền; rỗng = đen
	MarginV    int    `json:"marginV"`    // lề dưới (px) — đẩy phụ đề lên/xuống
	Font       string `json:"font"`       // tên font family (VD "Arial"); rỗng = Arial mặc định

	// Vị trí neo tâm tự do theo tỷ lệ khung đầu ra (0..1). HasCustomPosition=false
	// giữ nguyên cách burn phụ đề cũ bằng MarginV để tương thích preset/project đã lưu.
	PositionX         float64 `json:"positionX"`
	PositionY         float64 `json:"positionY"`
	HasCustomPosition bool    `json:"hasCustomPosition"`

	// Tự nghe tạo phụ đề (dùng cho KỊCH BẢN áp cho video tương lai): khi AutoGen=true
	// và Path rỗng, lúc xuất frontend sẽ nghe từng clip (Whisper) ra .srt rồi điền Path.
	// SourceLang="auto" tự nhận diện; TargetLang rỗng = giữ gốc, khác = dịch (Gemini).
	AutoGen    bool   `json:"autoGen"`
	SourceLang string `json:"sourceLang"`
	TargetLang string `json:"targetLang"`
}

// AspectOp mô tả cách chuyển tỷ lệ khung hình.
type AspectOp struct {
	Enabled bool    `json:"enabled"`
	Ratio   string  `json:"ratio"` // "9:16" / "1:1" / "16:9"
	Mode    string  `json:"mode"`  // "crop" (cắt) / "pad" (viền đen) / "blur" (nền mờ)
	PanX    float64 `json:"panX"`  // vị trí pan X (0..1, 0.5 = giữa tâm)
	PanY    float64 `json:"panY"`  // vị trí pan Y (0..1, 0.5 = giữa tâm)
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
	Font      string  `json:"font"`      // tên font family (VD "Arial", "Times New Roman"); rỗng = mặc định
	X         string  `json:"x"`         // biểu thức vị trí ffmpeg ("(w-text_w)/2" = giữa ngang)
	Y         string  `json:"y"`         // biểu thức vị trí ffmpeg
	StartTime float64 `json:"startTime"` // thời điểm hiện chữ (giây, trong clip); 0 = từ đầu
	EndTime   float64 `json:"endTime"`   // thời điểm ẩn chữ; 0 = tới cuối clip
	BgBox     bool    `json:"bgBox"`     // nền hộp mờ phía sau chữ cho dễ đọc
}

// WatermarkOp overlay một ảnh logo / watermark.
type WatermarkOp struct {
	Enabled   bool    `json:"enabled"`
	ImgPath   string  `json:"imgPath"`
	X         string  `json:"x"`
	Y         string  `json:"y"`
	Opacity   float64 `json:"opacity"`   // 0..1
	Scale     float64 `json:"scale"`     // hệ số scale so với gốc (1 = giữ nguyên)
	StartTime float64 `json:"startTime"` // thời điểm bắt đầu hiện watermark (giây, trong clip); 0 = từ đầu
	EndTime   float64 `json:"endTime"`   // thời điểm ẩn watermark; 0 = tới cuối clip
}

// CardOp overlay một khung nền card / banner đệm cho text.
type CardOp struct {
	Enabled       bool    `json:"enabled"`
	Mode          string  `json:"mode"`   // "preset" / "image"
	Preset        string  `json:"preset"` // "glass" / "gradient-purple" / "gold" / "ribbon" / "vintage" / "neon" / "stripes"
	ImgPath       string  `json:"imgPath"`
	Color         string  `json:"color"`
	Color2        string  `json:"color2"`
	Opacity       float64 `json:"opacity"`
	X             string  `json:"x"`
	Y             string  `json:"y"`
	Width         float64 `json:"width"`
	Height        float64 `json:"height"`
	BorderRadius  int     `json:"borderRadius"`
	StartTime     float64 `json:"startTime"`
	EndTime       float64 `json:"endTime"`
	CardAboveText bool    `json:"cardAboveText"` // true = đè lên chữ; false = chữ đè lên card (mặc định)
}

// AudioOp điều chỉnh âm thanh clip.
type AudioOp struct {
	Volume      float64  `json:"volume"`      // hệ số âm lượng (1 = giữ nguyên)
	Mute        bool     `json:"mute"`        // tắt tiếng gốc
	MusicPath   string   `json:"musicPath"`   // đường dẫn nhạc nền; "" = không (file nhạc đầu tiên hoặc đơn)
	MusicVolume float64  `json:"musicVolume"` // âm lượng nhạc nền (1 = giữ nguyên)
	FadeIn      float64  `json:"fadeIn"`      // fade in (giây)
	FadeOut     float64  `json:"fadeOut"`     // fade out (giây)
	MusicLoop   bool     `json:"musicLoop"`   // tự động lặp nhạc nền nếu ngắn hơn video
	MusicTracks []string `json:"musicTracks"` // danh sách nhiều file nhạc nền để ghép nối tiếp
}

// TransitionOp hiệu ứng chuyển cảnh vào đầu clip (dùng khi ghép nhiều clip).
type TransitionOp struct {
	Type     string  `json:"type"`     // "" / "fade" / "slide" / "wipe" ...
	Duration float64 `json:"duration"` // thời lượng transition (giây)
}

// DefaultEditOps trả về EditOps trung tính (không áp thao tác nào).
func DefaultEditOps() EditOps {
	return EditOps{
		Speed:    1.0,
		Color:    ColorOp{Saturation: 1.0},
		Audio:    AudioOp{Volume: 1.0, MusicVolume: 1.0},
		ZoomPan:  ZoomPanOp{Zoom: 1.08, Dir: "in"},
		Crop:     CropOp{Percent: 0.04},
		Rotate:   RotateOp{Degrees: 1.5},
		Noise:    NoiseOp{Strength: 12},
		Subtitle: SubtitleOp{FontSize: 24, MarginV: 40},
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
	VisualChange  float64 `json:"visualChange"`  // chuyển cảnh nội dung
	BlackFrame    float64 `json:"blackFrame"`    // màn hình đen / fade
	Silence       float64 `json:"silence"`       // khoảng im lặng
	LayoutChange  float64 `json:"layoutChange"`  // thay đổi bố cục / tỷ lệ vùng nội dung
	AudioChange   float64 `json:"audioChange"`   // thay đổi phổ / môi trường âm thanh
	ContinuityPen float64 `json:"continuityPen"` // phạt khi có dấu hiệu liền mạch (giọng nói, nhạc nền tiếp tục)
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
	ProxyFPS         int           `json:"proxyFPS"`         // FPS proxy video — mặc định 4
	Weights          SignalWeights `json:"weights"`          // Trọng số từng tín hiệu
	ExportPreset     string        `json:"exportPreset"`     // FFmpeg preset (ultrafast/fast/medium) — mặc định "fast"
	ExportCRF        int           `json:"exportCRF"`        // Chất lượng xuất (0-51, thấp = tốt hơn) — mặc định 23
	HardwareAccel    string        `json:"hardwareAccel"`    // acceleration method: none / nvidia / intel / amd — mặc định none
	Prompt           string        `json:"prompt"`           // Ý tưởng / chủ đề thiết kế cho video này
}

// DefaultWeights trả về trọng số tín hiệu mặc định.
// Đã hiệu chỉnh lại so với phiên bản cũ: tăng trọng số visual/silence,
// giảm continuity penalty để không loại nhầm quá nhiều điểm cắt hợp lệ.
//
// Ví dụ tính điểm với trọng số mới:
//
//	Visual only (không liền mạch): 40 → review
//	Visual + Silence:              40 + 30 = 70 → auto accept ✓
//	Visual + Black:                40 + 35 = 75 → auto accept ✓
//	Silence + Black:               30 + 35 = 65 → auto accept ✓
//	Visual only (liền mạch):       40 - 15 = 25 → vẫn review (trước đây bị reject!)
func DefaultWeights() SignalWeights {
	return SignalWeights{
		VisualChange:  45,
		BlackFrame:    35,
		Silence:       30,
		LayoutChange:  25,
		AudioChange:   20,
		ContinuityPen: 15,
	}
}

// DefaultConfig trả về cấu hình mặc định
func DefaultConfig() AnalyzerConfig {
	return AnalyzerConfig{
		Mode:             ModeSmart,
		SceneThreshold:   25.0, // bảo thủ hơn 20: bớt bắt chuyển cảnh yếu → ít điểm rác
		MinClipDuration:  3.0,  // clip tối thiểu 3s: chặn điểm cắt dày đặc (không ai làm clip 1s)
		MaxClipDuration:  60.0,
		AutoAcceptScore:  60, // visual-only scene change (45 - 10 = 35) sẽ được xếp vào review candidate, không bị auto-accept tràn lan
		ReviewMinScore:   35,
		SilenceThreshold: -30,
		SilenceDuration:  0.5,
		ProxyFPS:         4,
		Weights:          DefaultWeights(),
		ExportPreset:     "fast",
		ExportCRF:        23,
		HardwareAccel:    "none",
	}
}
