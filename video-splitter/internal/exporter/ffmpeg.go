package exporter

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"video-splitter/internal/media"
	"video-splitter/internal/project"
	"video-splitter/internal/utils"
)

// Kích thước khung mục tiêu cho từng tỷ lệ (chuẩn video social).
var aspectTargets = map[string][2]int{
	"9:16": {1080, 1920},
	"1:1":  {1080, 1080},
	"16:9": {1920, 1080},
}

// CutVideo cắt một clip từ startTime đến endTime và áp các thao tác chỉnh sửa
// trong clip.Edit. Luôn re-encode để frame-accurate và cho phép áp filter.
//
// Cách cắt: dùng "-ss <start>" TRƯỚC "-i" (input seeking). ffmpeg hiện đại seek
// chính xác tới frame (giải mã & bỏ frame thừa từ keyframe gần nhất) nên vừa nhanh
// vừa đúng, đồng thời làm cho input bắt đầu ~0s → filter time (drawtext/afade) là
// thời gian tương đối trong clip, khớp với TextOp/AudioOp. "-t <duration>" giới hạn
// độ dài đầu ra. Các input phụ (watermark, nhạc nền) thêm sau, không bị -ss ảnh hưởng.
func CutVideo(ctx context.Context, inputPath string, clip project.Clip, outputPath string, preset string, crf int) error {
	if preset == "" {
		preset = "fast"
	}
	if crf <= 0 || crf > 51 {
		crf = 23
	}

	dur := clip.EndTime - clip.StartTime
	if dur <= 0 {
		dur = clip.Duration
	}
	if dur <= 0 {
		return fmt.Errorf("clip %s có thời lượng không hợp lệ", clip.ID)
	}
	e := clip.Edit

	// File text tạm cho drawtext (tránh phải escape nội dung Unicode/ký tự đặc biệt).
	// Dọn sau khi ffmpeg chạy xong.
	var tmpFiles []string
	defer func() {
		for _, f := range tmpFiles {
			_ = os.Remove(f)
		}
	}()

	// === Dựng danh sách input ===
	// -accurate_seek: đảm bảo seek chính xác tới frame (không chỉ keyframe) khi kết
	// hợp input seeking. -fflags +genpts: tạo lại PTS liên tục, tránh lỗi PTS gián đoạn
	// khi video có B-frames hoặc PTS không đều — gây lệch filter_complex + setpts.
	args := []string{"-y", "-accurate_seek", "-fflags", "+genpts", "-ss", fmt.Sprintf("%.3f", clip.StartTime), "-i", inputPath}
	nextIdx := 1

	wmIdx := -1
	if e.Watermark.Enabled && e.Watermark.ImgPath != "" {
		args = append(args, "-i", e.Watermark.ImgPath)
		wmIdx = nextIdx
		nextIdx++
	}
	musicIdx := -1
	if e.Audio.MusicPath != "" {
		args = append(args, "-i", e.Audio.MusicPath)
		musicIdx = nextIdx
		nextIdx++
	}

	// === Dựng filter graph ===
	vOut, aOut, complexParts, textFiles, err := buildGraph(e, dur, wmIdx, musicIdx)
	if err != nil {
		return err
	}
	tmpFiles = append(tmpFiles, textFiles...)

	if len(complexParts) > 0 {
		args = append(args, "-filter_complex", strings.Join(complexParts, ";"))
		args = append(args, "-map", vOut)
		if aOut != "" {
			args = append(args, "-map", aOut)
		}
	}

	args = append(args,
		"-t", fmt.Sprintf("%.3f", dur),
		"-c:v", "libx264",
		"-preset", preset,
		"-crf", strconv.Itoa(crf),
		"-pix_fmt", "yuv420p",
		"-c:a", "aac",
		"-b:a", "128k",
		"-avoid_negative_ts", "make_zero",
		"-movflags", "+faststart",
	)
	if musicIdx >= 0 {
		args = append(args, "-shortest") // giữ đầu ra dài bằng video, không kéo theo nhạc thừa
	}
	args = append(args, outputPath)

	cmd := exec.CommandContext(ctx, utils.GetBinPath("ffmpeg"), args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg cut error: %v, output: %s", err, string(out))
	}
	return nil
}

// buildGraph dựng toàn bộ filter_complex cho một clip. Trả về nhãn map video/audio,
// các đoạn filter (nối bằng ';'), và danh sách file text tạm đã tạo cho drawtext.
// Nếu không có thao tác nào cần filter, trả về complexParts rỗng (caller map thẳng).
func buildGraph(e project.EditOps, dur float64, wmIdx, musicIdx int) (vOut, aOut string, parts []string, textFiles []string, err error) {
	// --- Nhánh VIDEO ---
	vSteps := []string{"setpts=PTS-STARTPTS"}

	// Tốc độ.
	if e.Speed > 0 && e.Speed != 1.0 {
		vSteps[0] = fmt.Sprintf("setpts=(PTS-STARTPTS)/%s", trimFloat(e.Speed))
	}

	// Lật ngang video (chế cháo chống bản quyền).
	if e.HFlip {
		vSteps = append(vSteps, "hflip")
	}

	// Tỷ lệ khung.
	blurGraph := ""
	if e.Aspect.Enabled {
		if t, ok := aspectTargets[e.Aspect.Ratio]; ok {
			w, h := t[0], t[1]
			switch e.Aspect.Mode {
			case "pad":
				vSteps = append(vSteps,
					fmt.Sprintf("scale=%d:%d:force_original_aspect_ratio=decrease", w, h),
					fmt.Sprintf("pad=%d:%d:(ow-iw)/2:(oh-ih)/2:black", w, h))
			case "blur":
				blurGraph = fmt.Sprintf(
					"split=2[bg][fg];"+
						"[bg]scale=%d:%d:force_original_aspect_ratio=increase,crop=%d:%d,gblur=sigma=20[bgb];"+
						"[fg]scale=%d:%d:force_original_aspect_ratio=decrease[fgs];"+
						"[bgb][fgs]overlay=(W-w)/2:(H-h)/2",
					w, h, w, h, w, h)
			default: // crop
				vSteps = append(vSteps,
					fmt.Sprintf("scale=%d:%d:force_original_aspect_ratio=increase", w, h),
					fmt.Sprintf("crop=%d:%d", w, h))
			}
		}
	}

	// Màu.
	if e.Color.Enabled {
		if eq := colorEq(e.Color); eq != "" {
			vSteps = append(vSteps, eq)
		}
		if p := colorPreset(e.Color.Preset); p != "" {
			vSteps = append(vSteps, p)
		}
	}

	// Text overlay (drawtext) — mỗi TextOp một bộ lọc, dùng textfile để an toàn Unicode.
	fontPath := utils.GetFontPath()
	for i, t := range e.Texts {
		if strings.TrimSpace(t.Content) == "" || fontPath == "" {
			continue
		}
		txtFile, ferr := writeTempText(t.Content, i)
		if ferr != nil {
			return "", "", nil, textFiles, ferr
		}
		textFiles = append(textFiles, txtFile)
		vSteps = append(vSteps, drawText(t, fontPath, txtFile))
	}

	// Ghép nhánh video tuyến tính.
	linearV := strings.Join(vSteps, ",")
	vLabel := "[vbase]"
	if blurGraph != "" {
		parts = append(parts, fmt.Sprintf("[0:v]%s,%s%s", linearV, blurGraph, vLabel))
	} else {
		parts = append(parts, fmt.Sprintf("[0:v]%s%s", linearV, vLabel))
	}

	// Watermark overlay (input phụ): scale + opacity rồi overlay lên video.
	if wmIdx >= 0 {
		scale := 1.0
		if e.Watermark.Scale > 0 {
			scale = e.Watermark.Scale
		}
		opacity := 1.0
		if e.Watermark.Opacity > 0 {
			opacity = e.Watermark.Opacity
		}
		x := e.Watermark.X
		if x == "" {
			x = "W-w-20"
		}
		y := e.Watermark.Y
		if y == "" {
			y = "H-h-20"
		}
		parts = append(parts, fmt.Sprintf(
			"[%d:v]scale=iw*%s:-1,format=rgba,colorchannelmixer=aa=%s[wm]",
			wmIdx, trimFloat(scale), trimFloat(opacity)))
		parts = append(parts, fmt.Sprintf("%s[wm]overlay=%s:%s[vout]", vLabel, x, y))
		vOut = "[vout]"
	} else {
		vOut = vLabel
	}

	// --- Nhánh AUDIO ---
	aSteps := []string{"asetpts=PTS-STARTPTS"}
	if e.Speed > 0 && e.Speed != 1.0 {
		for _, tp := range atempoChain(e.Speed) {
			aSteps = append(aSteps, "atempo="+trimFloat(tp))
		}
	}
	a := e.Audio
	if a.Mute {
		aSteps = append(aSteps, "volume=0")
	} else if a.Volume > 0 && a.Volume != 1.0 {
		aSteps = append(aSteps, "volume="+trimFloat(a.Volume))
	}
	// Fade tính theo thời lượng đầu ra (đã chia tốc độ nếu có).
	outDur := dur
	if e.Speed > 0 && e.Speed != 1.0 {
		outDur = dur / e.Speed
	}
	if a.FadeIn > 0 {
		aSteps = append(aSteps, fmt.Sprintf("afade=t=in:st=0:d=%s", trimFloat(a.FadeIn)))
	}
	if a.FadeOut > 0 && outDur > 0 {
		st := outDur - a.FadeOut
		if st < 0 {
			st = 0
		}
		aSteps = append(aSteps, fmt.Sprintf("afade=t=out:st=%s:d=%s", trimFloat(st), trimFloat(a.FadeOut)))
	}
	parts = append(parts, fmt.Sprintf("[0:a]%s[abase]", strings.Join(aSteps, ",")))

	// Nhạc nền: chỉnh âm lượng + fade rồi trộn với tiếng gốc.
	if musicIdx >= 0 {
		mSteps := []string{"asetpts=PTS-STARTPTS"}
		mv := 1.0
		if a.MusicVolume > 0 {
			mv = a.MusicVolume
		}
		if mv != 1.0 {
			mSteps = append(mSteps, "volume="+trimFloat(mv))
		}
		if a.FadeIn > 0 {
			mSteps = append(mSteps, fmt.Sprintf("afade=t=in:st=0:d=%s", trimFloat(a.FadeIn)))
		}
		if a.FadeOut > 0 && outDur > 0 {
			st := outDur - a.FadeOut
			if st < 0 {
				st = 0
			}
			mSteps = append(mSteps, fmt.Sprintf("afade=t=out:st=%s:d=%s", trimFloat(st), trimFloat(a.FadeOut)))
		}
		parts = append(parts, fmt.Sprintf("[%d:a]%s[mus]", musicIdx, strings.Join(mSteps, ",")))
		parts = append(parts, "[abase][mus]amix=inputs=2:duration=first:dropout_transition=0[aout]")
		aOut = "[aout]"
	} else {
		aOut = "[abase]"
	}

	return vOut, aOut, parts, textFiles, nil
}

// drawText dựng một filter drawtext từ TextOp.
func drawText(t project.TextOp, fontPath, txtFile string) string {
	color := t.Color
	if color == "" {
		color = "white"
	}
	size := t.FontSize
	if size <= 0 {
		size = 48
	}
	x := t.X
	if x == "" {
		x = "(w-text_w)/2"
	}
	y := t.Y
	if y == "" {
		y = "h-text_h-80"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "drawtext=fontfile=%s:textfile=%s:fontcolor=%s:fontsize=%d:x=%s:y=%s",
		ffFilterPath(fontPath), ffFilterPath(txtFile), color, size, x, y)
	if t.BgBox {
		b.WriteString(":box=1:boxcolor=black@0.5:boxborderw=10")
	}
	if t.StartTime > 0 || t.EndTime > 0 {
		end := t.EndTime
		if end <= 0 {
			end = 1e9 // tới cuối clip
		}
		fmt.Fprintf(&b, ":enable='between(t,%s,%s)'", trimFloat(t.StartTime), trimFloat(end))
	}
	return b.String()
}

// ConcatClips ghép nhiều file video (đã cắt/chỉnh) thành một file duy nhất.
// transitionType == "" hoặc transitionDur <= 0 → nối cứng (concat, nhanh).
// Ngược lại → dùng xfade (video) + acrossfade (audio) tại mọi mối nối, thời lượng
// transition đồng nhất. Các input được chuẩn hóa về cùng khung/fps của clip đầu.
func ConcatClips(inputFiles []string, outputPath, transitionType string, transitionDur float64, preset string, crf int) error {
	if len(inputFiles) == 0 {
		return fmt.Errorf("không có clip để ghép")
	}
	if preset == "" {
		preset = "fast"
	}
	if crf <= 0 || crf > 51 {
		crf = 23
	}
	if len(inputFiles) == 1 {
		return copyFile(inputFiles[0], outputPath)
	}

	useTransition := transitionType != "" && transitionDur > 0
	if !useTransition {
		return concatDemuxer(inputFiles, outputPath, preset, crf)
	}
	return concatXfade(inputFiles, outputPath, transitionType, transitionDur, preset, crf)
}

// concatDemuxer nối cứng bằng concat filter (re-encode, an toàn với input cùng codec).
func concatDemuxer(inputFiles []string, outputPath, preset string, crf int) error {
	// Chuẩn hóa về khung/fps của clip đầu để concat filter không lỗi lệch kích thước.
	w, h, fps := probeFrame(inputFiles[0])
	args := []string{"-y"}
	for _, f := range inputFiles {
		args = append(args, "-i", f)
	}
	var parts []string
	var concatIn strings.Builder
	for i := range inputFiles {
		parts = append(parts, fmt.Sprintf(
			"[%d:v]scale=%d:%d:force_original_aspect_ratio=decrease,pad=%d:%d:(ow-iw)/2:(oh-ih)/2,setsar=1,fps=%s[v%d]",
			i, w, h, w, h, fps, i))
		concatIn.WriteString(fmt.Sprintf("[v%d][%d:a]", i, i))
	}
	filter := strings.Join(parts, ";") + ";" +
		concatIn.String() + fmt.Sprintf("concat=n=%d:v=1:a=1[vout][aout]", len(inputFiles))

	args = append(args,
		"-filter_complex", filter,
		"-map", "[vout]", "-map", "[aout]",
		"-c:v", "libx264", "-preset", preset, "-crf", strconv.Itoa(crf),
		"-pix_fmt", "yuv420p", "-c:a", "aac", "-b:a", "128k",
		"-movflags", "+faststart", outputPath)

	cmd := exec.Command(utils.GetBinPath("ffmpeg"), args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg concat error: %v, output: %s", err, string(out))
	}
	return nil
}

// concatXfade nối các clip với hiệu ứng chuyển cảnh xfade/acrossfade đồng nhất.
func concatXfade(inputFiles []string, outputPath, transitionType string, td float64, preset string, crf int) error {
	w, h, fps := probeFrame(inputFiles[0])
	durs := make([]float64, len(inputFiles))
	for i, f := range inputFiles {
		if info, e := media.GetVideoInfo(f); e == nil && info != nil {
			durs[i] = info.Duration
		}
	}

	args := []string{"-y"}
	for _, f := range inputFiles {
		args = append(args, "-i", f)
	}

	var parts []string
	// Chuẩn hóa từng input.
	for i := range inputFiles {
		parts = append(parts, fmt.Sprintf(
			"[%d:v]scale=%d:%d:force_original_aspect_ratio=decrease,pad=%d:%d:(ow-iw)/2:(oh-ih)/2,setsar=1,fps=%s,format=yuv420p[v%d]",
			i, w, h, w, h, fps, i))
		parts = append(parts, fmt.Sprintf("[%d:a]aresample=async=1[a%d]", i, i))
	}

	// Chuỗi xfade video: offset lũy tiến = tổng độ dài đã ghép - transition.
	prevV := "[v0]"
	prevA := "[a0]"
	running := durs[0]
	for i := 1; i < len(inputFiles); i++ {
		offset := running - td
		if offset < 0 {
			offset = 0
		}
		vlab := fmt.Sprintf("[vx%d]", i)
		alab := fmt.Sprintf("[ax%d]", i)
		parts = append(parts, fmt.Sprintf("%s[v%d]xfade=transition=%s:duration=%s:offset=%s%s",
			prevV, i, transitionType, trimFloat(td), trimFloat(offset), vlab))
		parts = append(parts, fmt.Sprintf("%s[a%d]acrossfade=d=%s%s",
			prevA, i, trimFloat(td), alab))
		prevV = vlab
		prevA = alab
		running = running + durs[i] - td
	}

	filter := strings.Join(parts, ";")
	args = append(args,
		"-filter_complex", filter,
		"-map", prevV, "-map", prevA,
		"-c:v", "libx264", "-preset", preset, "-crf", strconv.Itoa(crf),
		"-pix_fmt", "yuv420p", "-c:a", "aac", "-b:a", "128k",
		"-movflags", "+faststart", outputPath)

	cmd := exec.Command(utils.GetBinPath("ffmpeg"), args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg xfade error: %v, output: %s", err, string(out))
	}
	return nil
}

// VerifyOutput dùng ffprobe kiểm tra file đầu ra: tồn tại, đọc được, và thời lượng
// nằm trong dung sai so với kỳ vọng. Trả về thời lượng thực và lỗi (nil nếu đạt).
func VerifyOutput(outputPath string, expectedDur, tolerance float64) (float64, error) {
	info, err := media.GetVideoInfo(outputPath)
	if err != nil {
		return 0, fmt.Errorf("không đọc được file đầu ra: %v", err)
	}
	if info.Duration <= 0 {
		return 0, fmt.Errorf("file đầu ra có thời lượng 0")
	}
	if expectedDur > 0 && tolerance > 0 {
		diff := info.Duration - expectedDur
		if diff < 0 {
			diff = -diff
		}
		if diff > tolerance {
			return info.Duration, fmt.Errorf("lệch thời lượng %.2fs (thực %.2fs / kỳ vọng %.2fs)", diff, info.Duration, expectedDur)
		}
	}
	return info.Duration, nil
}

// probeFrame lấy width/height/fps của một file để chuẩn hóa concat/xfade.
// Trả về mặc định 1080x1920@30 nếu không đọc được.
func probeFrame(path string) (w, h int, fps string) {
	w, h, fps = 1080, 1920, "30"
	if info, err := media.GetVideoInfo(path); err == nil && info != nil {
		if info.Width > 0 && info.Height > 0 {
			w, h = info.Width, info.Height
		}
		if info.FPS > 0 {
			fps = strconv.FormatFloat(info.FPS, 'f', -1, 64)
		}
	}
	return
}

// writeTempText ghi nội dung text ra file tạm UTF-8 cho drawtext textfile=.
func writeTempText(content string, idx int) (string, error) {
	dir := filepath.Join(os.TempDir(), "video-splitter", "drawtext")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	f, err := os.CreateTemp(dir, fmt.Sprintf("text_%d_*.txt", idx))
	if err != nil {
		return "", err
	}
	defer f.Close()
	if _, err := f.WriteString(content); err != nil {
		return "", err
	}
	return f.Name(), nil
}

// ffFilterPath escape đường dẫn Windows để dùng trong filter option (fontfile/textfile):
// đổi '\' -> '/' và escape ':' của ổ đĩa thành '\:'.
func ffFilterPath(p string) string {
	p = filepath.ToSlash(p)
	p = strings.ReplaceAll(p, ":", "\\:")
	return p
}

// copyFile sao chép file (dùng khi ghép chỉ có 1 clip).
func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0644)
}

// colorEq dựng filter eq từ ColorOp.
func colorEq(c project.ColorOp) string {
	var parts []string
	if c.Brightness != 0 {
		parts = append(parts, "brightness="+trimFloat(c.Brightness))
	}
	if c.Contrast != 0 {
		parts = append(parts, "contrast="+trimFloat(1+c.Contrast))
	}
	if c.Saturation > 0 && c.Saturation != 1.0 {
		parts = append(parts, "saturation="+trimFloat(c.Saturation))
	}
	if len(parts) == 0 {
		return ""
	}
	return "eq=" + strings.Join(parts, ":")
}

// colorPreset trả về filter tương ứng preset màu (rỗng nếu không có).
func colorPreset(preset string) string {
	switch preset {
	case "bw":
		return "hue=s=0"
	case "vivid":
		return "eq=saturation=1.4:contrast=1.1"
	case "warm":
		return "colorbalance=rs=0.10:gs=0.02:bs=-0.08"
	case "cool":
		return "colorbalance=rs=-0.08:gs=0.00:bs=0.12"
	case "vintage":
		return "curves=preset=vintage"
	}
	return ""
}

// atempoChain phân rã hệ số tốc độ thành chuỗi atempo hợp lệ (mỗi bộ lọc chỉ nhận 0.5–2.0).
func atempoChain(speed float64) []float64 {
	var out []float64
	s := speed
	for s > 2.0 {
		out = append(out, 2.0)
		s /= 2.0
	}
	for s < 0.5 {
		out = append(out, 0.5)
		s /= 0.5
	}
	out = append(out, s)
	return out
}

// trimFloat format float gọn (bỏ số 0 thừa) để lệnh ffmpeg sạch.
func trimFloat(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}
