package exporter

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"video-splitter/internal/project"

	// decode JPEG/PNG cho chế độ card "image"
	_ "image/jpeg"
)

// uiPreviewFrameW là chiều rộng (px) của khung preview trong tab Chỉnh sửa. Mọi giá trị
// người dùng nhập bằng px trên preview (cỡ chữ, bo góc, độ dày viền) phải nhân
// frameW/uiPreviewFrameW khi xuất để trông đúng như lúc chỉnh.
const uiPreviewFrameW = 500.0

// renderCardPNG dựng card/banner thành 1 ảnh PNG (straight alpha) đúng kích thước pixel
// của card trong khung xuất, khớp với preview CSS ở RemixScenarioPage.vue. Trả về đường
// dẫn file PNG tạm (caller tự thêm vào danh sách dọn dẹp) hoặc lỗi.
//
// Vì sao render bằng Go thay vì filtergraph ffmpeg: gradient + bo góc + viền (solid/
// dashed/2 màu/glow) + opacity + sọc chéo rất khó dựng đúng bằng gradients/geq/drawbox;
// vẽ pixel bằng Go cho kiểm soát chính xác và gộp cả 3 chế độ (preset/color/image) về
// một đường overlay duy nhất.
func renderCardPNG(c project.CardOp, frameW, frameH int) (string, error) {
	if frameW <= 0 || frameH <= 0 {
		return "", fmt.Errorf("kích thước khung không hợp lệ: %dx%d", frameW, frameH)
	}

	// Kích thước card theo % khung (Width/Height là phần trăm 0..100).
	cw := int(math.Round(float64(frameW) * clampFloat(c.Width, 5, 100) / 100.0))
	ch := int(math.Round(float64(frameH) * clampFloat(c.Height, 3, 100) / 100.0))
	if cw < 2 {
		cw = 2
	}
	if ch < 2 {
		ch = 2
	}

	// Hệ số quy đổi px của preview (~khung rộng 500px) sang khung xuất, để độ dày viền,
	// bán kính bo góc, chu kỳ sọc trông tương đương preview.
	uiScale := float64(frameW) / uiPreviewFrameW
	if uiScale < 1 {
		uiScale = 1
	}

	img := image.NewNRGBA(image.Rect(0, 0, cw, ch))

	mode := strings.ToLower(strings.TrimSpace(c.Mode))
	switch mode {
	case "image":
		if err := fillCardImage(img, c.ImgPath, cw, ch); err != nil {
			return "", err
		}
	case "color":
		fillLinearGradient(img, cw, ch, 135, []gradStop{
			{0, colorFromHex(c.Color, 15, 23, 42), 1},
			{1, colorFromHex(c.Color2, 30, 41, 59), 1},
		})
		drawSolidBorder(img, cw, ch, maxInt(1, int(math.Round(1.5*uiScale))), rgb{255, 255, 255}, 0.35)
	default:
		fillPresetBackground(img, cw, ch, strings.ToLower(strings.TrimSpace(c.Preset)), uiScale)
	}

	// Bo góc: kẹp bán kính ≤ nửa cạnh ngắn để không méo. borderRadius (px UI) * uiScale.
	radius := int(math.Round(float64(c.BorderRadius) * uiScale))
	if maxR := minInt(cw, ch) / 2; radius > maxR {
		radius = maxR
	}
	if radius > 0 {
		applyRoundedAlpha(img, cw, ch, radius)
	}

	// Opacity toàn phần tử: CSS áp opacity lên cả card → nhân dồn vào alpha (đã có sẵn
	// alpha riêng của từng preset). Kẹp [0.05, 1] giống slider UI.
	op := clampFloat(c.Opacity, 0.05, 1)
	if op < 1 {
		multiplyAlpha(img, op)
	}

	return writeCardPNG(img)
}

// === Preset backgrounds (màu trích đúng từ CSS .card-preview-overlay.preset-*) ===

func fillPresetBackground(img *image.NRGBA, cw, ch int, preset string, uiScale float64) {
	switch preset {
	case "gradient-purple":
		fillLinearGradient(img, cw, ch, 135, []gradStop{
			{0, rgb{147, 51, 234}, 0.85},
			{1, rgb{219, 39, 119}, 0.85},
		})
		drawSolidBorder(img, cw, ch, maxInt(1, int(math.Round(1.5*uiScale))), rgb{255, 255, 255}, 0.4)
	case "gold":
		fillLinearGradient(img, cw, ch, 135, []gradStop{
			{0, rgb{217, 119, 6}, 0.9},
			{0.5, rgb{245, 158, 11}, 0.9},
			{1, rgb{180, 83, 9}, 0.9},
		})
		drawSolidBorder(img, cw, ch, maxInt(1, int(math.Round(1.5*uiScale))), rgb{254, 240, 138}, 1)
	case "ribbon":
		fillLinearGradient(img, cw, ch, 90, []gradStop{
			{0, rgb{220, 38, 38}, 0.95},
			{1, rgb{185, 28, 28}, 0.95},
		})
		// Viền trên/dưới khác màu tạo hiệu ứng nổi; không có viền trái/phải.
		bw := maxInt(1, int(math.Round(2*uiScale)))
		drawHorizontalEdge(img, cw, ch, bw, true, rgb{252, 165, 165})  // top highlight
		drawHorizontalEdge(img, cw, ch, bw, false, rgb{127, 29, 29})   // bottom shadow
	case "vintage":
		fillSolid(img, cw, ch, rgb{254, 243, 199}, 1)
		drawDashedBorder(img, cw, ch, maxInt(1, int(math.Round(2*uiScale))), rgb{180, 83, 9}, uiScale)
	case "neon":
		fillSolid(img, cw, ch, rgb{0, 0, 0}, 0.8)
		drawGlowBorder(img, cw, ch, maxInt(1, int(math.Round(2*uiScale))), rgb{6, 182, 212}, uiScale)
	case "stripes":
		fillDiagonalStripes(img, cw, ch, uiScale)
		drawSolidBorder(img, cw, ch, maxInt(1, int(math.Round(1*uiScale))), rgb{255, 255, 255}, 0.25)
	default: // glass (và mọi preset lạ đã bỏ khỏi UI → fallback glass)
		fillSolid(img, cw, ch, rgb{15, 23, 42}, 0.75)
		drawSolidBorder(img, cw, ch, maxInt(1, int(math.Round(1.5*uiScale))), rgb{56, 189, 248}, 0.6)
	}
}

// === Chế độ image: scale-cover ảnh vào khung card ===

func fillCardImage(dst *image.NRGBA, imgPath string, cw, ch int) error {
	if strings.TrimSpace(imgPath) == "" {
		return fmt.Errorf("chế độ card 'image' nhưng chưa có đường dẫn ảnh")
	}
	f, err := os.Open(imgPath)
	if err != nil {
		return fmt.Errorf("không mở được ảnh card: %w", err)
	}
	defer f.Close()
	src, _, err := image.Decode(f)
	if err != nil {
		return fmt.Errorf("không giải mã được ảnh card: %w", err)
	}
	sb := src.Bounds()
	sw, sh := sb.Dx(), sb.Dy()
	if sw <= 0 || sh <= 0 {
		return fmt.Errorf("ảnh card rỗng")
	}
	// object-fit: cover → scale theo cạnh lớn hơn để phủ kín, phần thừa bị cắt.
	scale := math.Max(float64(cw)/float64(sw), float64(ch)/float64(sh))
	dispW := float64(sw) * scale
	dispH := float64(sh) * scale
	offX := (dispW - float64(cw)) / 2
	offY := (dispH - float64(ch)) / 2
	for y := 0; y < ch; y++ {
		for x := 0; x < cw; x++ {
			srcX := int((float64(x) + offX) / scale)
			srcY := int((float64(y) + offY) / scale)
			if srcX < 0 {
				srcX = 0
			}
			if srcY < 0 {
				srcY = 0
			}
			if srcX >= sw {
				srcX = sw - 1
			}
			if srcY >= sh {
				srcY = sh - 1
			}
			r, g, b, a := src.At(sb.Min.X+srcX, sb.Min.Y+srcY).RGBA()
			dst.SetNRGBA(x, y, colorNRGBA(uint8(r>>8), uint8(g>>8), uint8(b>>8), float64(a>>8)/255.0))
		}
	}
	return nil
}

// === Helpers vẽ ===

type rgb struct{ r, g, b uint8 }

type gradStop struct {
	pos float64 // 0..1
	c   rgb
	a   float64 // 0..1
}

// fillSolid đổ nền đơn sắc với alpha cho trước.
func fillSolid(img *image.NRGBA, cw, ch int, c rgb, a float64) {
	for y := 0; y < ch; y++ {
		for x := 0; x < cw; x++ {
			img.SetNRGBA(x, y, colorNRGBA(c.r, c.g, c.b, a))
		}
	}
}

// fillLinearGradient đổ gradient tuyến tính theo góc CSS (deg). 0deg = hướng lên,
// 90deg = sang phải, 135deg = xuống phải. Nội suy màu + alpha giữa các stop.
func fillLinearGradient(img *image.NRGBA, cw, ch int, angleDeg float64, stops []gradStop) {
	if len(stops) == 0 {
		return
	}
	rad := angleDeg * math.Pi / 180.0
	dx := math.Sin(rad)
	dy := -math.Cos(rad)
	// Projection của 4 góc để chuẩn hóa t về [0,1].
	corners := [][2]float64{{0, 0}, {float64(cw), 0}, {0, float64(ch)}, {float64(cw), float64(ch)}}
	minP, maxP := math.Inf(1), math.Inf(-1)
	for _, p := range corners {
		v := p[0]*dx + p[1]*dy
		minP = math.Min(minP, v)
		maxP = math.Max(maxP, v)
	}
	span := maxP - minP
	if span == 0 {
		span = 1
	}
	for y := 0; y < ch; y++ {
		for x := 0; x < cw; x++ {
			proj := (float64(x)*dx + float64(y)*dy - minP) / span
			c, a := sampleStops(stops, proj)
			img.SetNRGBA(x, y, colorNRGBA(c.r, c.g, c.b, a))
		}
	}
}

// sampleStops nội suy màu + alpha tại vị trí t (0..1) trong danh sách stop đã sắp tăng dần.
func sampleStops(stops []gradStop, t float64) (rgb, float64) {
	if t <= stops[0].pos {
		return stops[0].c, stops[0].a
	}
	if t >= stops[len(stops)-1].pos {
		last := stops[len(stops)-1]
		return last.c, last.a
	}
	for i := 1; i < len(stops); i++ {
		if t <= stops[i].pos {
			a0, a1 := stops[i-1], stops[i]
			span := a1.pos - a0.pos
			f := 0.0
			if span > 0 {
				f = (t - a0.pos) / span
			}
			return rgb{
				lerp8(a0.c.r, a1.c.r, f),
				lerp8(a0.c.g, a1.c.g, f),
				lerp8(a0.c.b, a1.c.b, f),
			}, a0.a + (a1.a-a0.a)*f
		}
	}
	last := stops[len(stops)-1]
	return last.c, last.a
}

// fillDiagonalStripes vẽ sọc chéo 45° xen kẽ 2 màu slate, khớp stripes CSS
// (repeating-linear-gradient 45deg, chu kỳ 20px ở preview → *uiScale khi xuất).
func fillDiagonalStripes(img *image.NRGBA, cw, ch int, uiScale float64) {
	stripe := 10.0 * uiScale // mỗi dải dày 10px preview
	if stripe < 1 {
		stripe = 1
	}
	period := stripe * 2
	cA, cB := rgb{30, 41, 59}, rgb{15, 23, 42}
	for y := 0; y < ch; y++ {
		for x := 0; x < cw; x++ {
			// Chiếu lên hướng 45° (x+y), lấy phần dư theo chu kỳ.
			d := math.Mod(float64(x+y), period)
			if d < 0 {
				d += period
			}
			c := cA
			if d >= stripe {
				c = cB
			}
			img.SetNRGBA(x, y, colorNRGBA(c.r, c.g, c.b, 0.95))
		}
	}
}

// drawSolidBorder vẽ viền đặc bao quanh (source-over lên nền hiện có).
func drawSolidBorder(img *image.NRGBA, cw, ch, width int, c rgb, a float64) {
	if width <= 0 {
		return
	}
	for y := 0; y < ch; y++ {
		for x := 0; x < cw; x++ {
			if x < width || x >= cw-width || y < width || y >= ch-width {
				blendPixel(img, x, y, c, a)
			}
		}
	}
}

// drawHorizontalEdge vẽ dải viền trên (top=true) hoặc dưới (top=false).
func drawHorizontalEdge(img *image.NRGBA, cw, ch, width int, top bool, c rgb) {
	if width <= 0 {
		return
	}
	for i := 0; i < width; i++ {
		y := i
		if !top {
			y = ch - 1 - i
		}
		if y < 0 || y >= ch {
			continue
		}
		for x := 0; x < cw; x++ {
			blendPixel(img, x, y, c, 1)
		}
	}
}

// drawDashedBorder vẽ viền nét đứt (vintage) — bật/tắt theo đoạn dọc chu vi.
func drawDashedBorder(img *image.NRGBA, cw, ch, width int, c rgb, uiScale float64) {
	if width <= 0 {
		return
	}
	dash := int(math.Round(8 * uiScale)) // độ dài nét + khoảng trống ~8px preview mỗi bên
	if dash < 2 {
		dash = 2
	}
	on := func(pos int) bool { return (pos/dash)%2 == 0 }
	for y := 0; y < ch; y++ {
		for x := 0; x < cw; x++ {
			isBorder := x < width || x >= cw-width || y < width || y >= ch-width
			if !isBorder {
				continue
			}
			// Vị trí dọc chu vi xấp xỉ: cạnh trên/dưới theo x, trái/phải theo y.
			var pos int
			if y < width || y >= ch-width {
				pos = x
			} else {
				pos = y
			}
			if on(pos) {
				blendPixel(img, x, y, c, 1)
			}
		}
	}
}

// drawGlowBorder vẽ viền cyan (neon) + vài lớp glow mờ dần vào trong để xấp xỉ box-shadow.
func drawGlowBorder(img *image.NRGBA, cw, ch, width int, c rgb, uiScale float64) {
	// Glow trong: các vòng mờ dần từ viền vào tâm.
	glowLayers := maxInt(3, int(math.Round(6*uiScale)))
	for g := glowLayers; g >= 1; g-- {
		inset := width + g
		alpha := 0.5 * (1 - float64(g)/float64(glowLayers+1))
		for y := 0; y < ch; y++ {
			for x := 0; x < cw; x++ {
				if x < inset || x >= cw-inset || y < inset || y >= ch-inset {
					// chỉ tô đúng "khung" tại độ dày inset (không tô đặc bên trong)
					if x < width || x >= cw-width || y < width || y >= ch-width {
						continue // để lớp viền đặc vẽ sau
					}
					blendPixel(img, x, y, c, alpha)
				}
			}
		}
	}
	// Viền đặc ngoài cùng.
	drawSolidBorder(img, cw, ch, width, c, 1)
}

// applyRoundedAlpha bo 4 góc: pixel ngoài đường bo → alpha 0. Anti-alias nhẹ ở mép.
func applyRoundedAlpha(img *image.NRGBA, cw, ch, radius int) {
	r := float64(radius)
	// Tâm cung của 4 góc.
	centers := [4][2]float64{
		{r, r},                             // TL
		{float64(cw) - r, r},               // TR
		{r, float64(ch) - r},               // BL
		{float64(cw) - r, float64(ch) - r}, // BR
	}
	for y := 0; y < ch; y++ {
		for x := 0; x < cw; x++ {
			var cx, cy float64
			inCorner := false
			fx, fy := float64(x)+0.5, float64(y)+0.5
			if fx < r && fy < r {
				cx, cy, inCorner = centers[0][0], centers[0][1], true
			} else if fx > float64(cw)-r && fy < r {
				cx, cy, inCorner = centers[1][0], centers[1][1], true
			} else if fx < r && fy > float64(ch)-r {
				cx, cy, inCorner = centers[2][0], centers[2][1], true
			} else if fx > float64(cw)-r && fy > float64(ch)-r {
				cx, cy, inCorner = centers[3][0], centers[3][1], true
			}
			if !inCorner {
				continue
			}
			dist := math.Hypot(fx-cx, fy-cy)
			if dist <= r-0.5 {
				continue // trong vùng bo → giữ nguyên
			}
			idx := img.PixOffset(x, y)
			if dist >= r+0.5 {
				img.Pix[idx+3] = 0 // ngoài hẳn → trong suốt
				continue
			}
			// Mép: nội suy anti-alias.
			cov := (r + 0.5) - dist // 0..1
			cur := float64(img.Pix[idx+3])
			img.Pix[idx+3] = uint8(cur * clampFloat(cov, 0, 1))
		}
	}
}

// multiplyAlpha nhân toàn bộ kênh alpha với hệ số (áp opacity toàn card).
func multiplyAlpha(img *image.NRGBA, factor float64) {
	for i := 3; i < len(img.Pix); i += 4 {
		img.Pix[i] = uint8(float64(img.Pix[i]) * factor)
	}
}

// blendPixel ghi màu nguồn (straight alpha) lên pixel đích theo source-over.
func blendPixel(img *image.NRGBA, x, y int, c rgb, a float64) {
	if x < 0 || y < 0 || x >= img.Rect.Dx() || y >= img.Rect.Dy() {
		return
	}
	idx := img.PixOffset(x, y)
	dr := float64(img.Pix[idx]) / 255.0
	dg := float64(img.Pix[idx+1]) / 255.0
	db := float64(img.Pix[idx+2]) / 255.0
	da := float64(img.Pix[idx+3]) / 255.0
	sr := float64(c.r) / 255.0
	sg := float64(c.g) / 255.0
	sb := float64(c.b) / 255.0
	outA := a + da*(1-a)
	if outA <= 0 {
		img.Pix[idx], img.Pix[idx+1], img.Pix[idx+2], img.Pix[idx+3] = 0, 0, 0, 0
		return
	}
	outR := (sr*a + dr*da*(1-a)) / outA
	outG := (sg*a + dg*da*(1-a)) / outA
	outB := (sb*a + db*da*(1-a)) / outA
	img.Pix[idx] = uint8(math.Round(outR * 255))
	img.Pix[idx+1] = uint8(math.Round(outG * 255))
	img.Pix[idx+2] = uint8(math.Round(outB * 255))
	img.Pix[idx+3] = uint8(math.Round(outA * 255))
}

func colorNRGBA(r, g, b uint8, a float64) color.NRGBA {
	return color.NRGBA{R: r, G: g, B: b, A: uint8(math.Round(clampFloat(a, 0, 1) * 255))}
}

// writeCardPNG ghi ảnh card ra file PNG tạm (pattern giống writeTempText).
func writeCardPNG(img *image.NRGBA) (string, error) {
	dir := filepath.Join(os.TempDir(), "video-splitter", "card")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	f, err := os.CreateTemp(dir, "card_*.png")
	if err != nil {
		return "", err
	}
	path := f.Name()
	if err := png.Encode(f, img); err != nil {
		_ = f.Close()
		_ = os.Remove(path)
		return "", err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(path)
		return "", err
	}
	return path, nil
}

// colorFromHex parse "#rrggbb" (hoặc "rrggbb"); lỗi thì dùng màu mặc định r,g,b.
func colorFromHex(hex string, dr, dg, db uint8) rgb {
	h := strings.TrimPrefix(strings.TrimSpace(hex), "#")
	if len(h) != 6 {
		return rgb{dr, dg, db}
	}
	v, err := strconv.ParseUint(h, 16, 32)
	if err != nil {
		return rgb{dr, dg, db}
	}
	return rgb{uint8(v >> 16), uint8(v >> 8), uint8(v)}
}

func lerp8(a, b uint8, f float64) uint8 {
	return uint8(math.Round(float64(a) + (float64(b)-float64(a))*clampFloat(f, 0, 1)))
}

func clampFloat(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
