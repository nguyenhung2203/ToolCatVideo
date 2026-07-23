package exporter

import (
	"context"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
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

// needsReencode kiểm tra xem clip có bất kỳ filter/chỉnh sửa nào cần re-encode hay không.
// Nếu tất cả filter đều tắt (tỷ lệ, màu, tốc độ, text, watermark, nhạc nền, flip...),
// ta có thể dùng stream-copy (-c copy) để cắt nhanh gấp 5-20 lần.
func needsReencode(e project.EditOps) bool {
	if e.Aspect.Enabled {
		return true
	}
	if e.Color.Enabled {
		return true
	}
	if e.Speed > 0 && e.Speed != 1.0 {
		return true
	}
	if e.HFlip {
		return true
	}
	if len(e.Texts) > 0 {
		for _, t := range e.Texts {
			if strings.TrimSpace(t.Content) != "" {
				return true
			}
		}
	}
	if e.Watermark.Enabled && e.Watermark.ImgPath != "" {
		return true
	}
	if e.Audio.MusicPath != "" || len(e.Audio.MusicTracks) > 0 {
		return true
	}
	if e.Audio.Mute {
		return true
	}
	if e.Audio.Volume > 0 && e.Audio.Volume != 1.0 {
		return true
	}
	if e.Audio.FadeIn > 0 || e.Audio.FadeOut > 0 {
		return true
	}
	// Nhóm xào nấu chống trùng lặp.
	if e.ZoomPan.Enabled || e.Crop.Enabled || e.Rotate.Enabled || e.Noise.Enabled {
		return true
	}
	if e.Pitch != 0 {
		return true
	}
	if e.Subtitle.Enabled && e.Subtitle.Path != "" {
		return true
	}
	// TrimStart/TrimEnd và StripMeta KHÔNG cần re-encode: xử lý riêng qua -ss/-t và
	// -map_metadata -1 ngay cả khi stream-copy (xem CutVideo).
	return false
}

// cutVideoStreamCopy cắt video bằng stream-copy (không re-encode).
// Ưu điểm: nhanh gấp 5-20 lần, không mất chất lượng (generation loss).
// Nhược điểm: chỉ chính xác tới keyframe gần nhất (có thể lệch 0-0.5s ở đầu clip).
//
// Kỹ thuật: Đặt -ss TRƯỚC -i (input seeking) để ffmpeg seek đến keyframe gần nhất
// rồi bỏ packet thừa khi mux. -avoid_negative_ts make_zero đảm bảo PTS bắt đầu từ 0.
func cutVideoStreamCopy(ctx context.Context, inputPath string, startTime, dur float64, outputPath string, stripMeta bool) error {
	args := []string{
		"-y",
		"-ss", fmt.Sprintf("%.3f", startTime),
		"-i", inputPath,
		"-t", fmt.Sprintf("%.3f", dur),
		"-c", "copy",
	}
	if stripMeta {
		// Xóa toàn bộ metadata (title/encoder/creation_time) — né đối chiếu metadata
		// của FB/TikTok. Vẫn stream-copy được vì chỉ bỏ tag, không đụng khung hình.
		args = append(args, "-map_metadata", "-1")
	}
	args = append(args,
		"-avoid_negative_ts", "make_zero",
		"-movflags", "+faststart",
		outputPath,
	)
	cmd := exec.CommandContext(ctx, utils.GetBinPath("ffmpeg"), args...)
	utils.HideCmdWindow(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg stream-copy error: %v, output: %s", err, string(out))
	}
	return nil
}

// cutVideoReencode cắt video có re-encode (chậm nhưng frame-accurate và hỗ trợ filter).
func cutVideoReencode(ctx context.Context, inputPath string, clip project.Clip, outputPath string, preset string, crf int, threads int, hwAccel string) error {
	dur := clip.EndTime - clip.StartTime
	if dur <= 0 {
		dur = clip.Duration
	}
	if dur <= 0 {
		return fmt.Errorf("clip %s có thời lượng không hợp lệ", clip.ID)
	}
	e := clip.Edit

	// Trim ngẫu nhiên đầu/cuối (chống trùng: đổi độ dài + hash). Dịch điểm seek sang
	// phải TrimStart giây và giảm thời lượng cả hai đầu. Giữ lại tối thiểu 0.5s để
	// không tạo clip rỗng khi khoảng trim vô tình lớn hơn clip.
	effStart := clip.StartTime
	if e.TrimStart > 0 {
		effStart += e.TrimStart
		dur -= e.TrimStart
	}
	if e.TrimEnd > 0 {
		dur -= e.TrimEnd
	}
	if dur < 0.5 {
		// Khoảng trim quá lớn so với clip → bỏ trim, giữ nguyên clip gốc.
		effStart = clip.StartTime
		dur = clip.EndTime - clip.StartTime
		if dur <= 0 {
			dur = clip.Duration
		}
	}

	// File text tạm cho drawtext (tránh phải escape nội dung Unicode/ký tự đặc biệt).
	var tmpFiles []string
	defer func() {
		for _, f := range tmpFiles {
			_ = os.Remove(f)
		}
	}()

	// Xử lý nhạc nền (đơn hoặc ghép nhiều bài)
	musicPathToUse := e.Audio.MusicPath
	if len(e.Audio.MusicTracks) > 0 {
		if len(e.Audio.MusicTracks) == 1 {
			musicPathToUse = e.Audio.MusicTracks[0]
		} else {
			mergedMusic, err := mergeMusicTracks(ctx, e.Audio.MusicTracks)
			if err != nil {
				return err
			}
			if mergedMusic != "" {
				musicPathToUse = mergedMusic
				tmpFiles = append(tmpFiles, mergedMusic) // tự động xóa khi chạy xong
			}
		}
	}

	wmIdx := -1
	if e.Watermark.Enabled && e.Watermark.ImgPath != "" {
		wmIdx = 1
	}
	musicIdx := -1
	if musicPathToUse != "" {
		if wmIdx >= 0 {
			musicIdx = 2
		} else {
			musicIdx = 1
		}
	}

	vOut, aOut, complexParts, textFiles, err := buildGraph(e, dur, wmIdx, musicIdx)
	if err != nil {
		return err
	}
	tmpFiles = append(tmpFiles, textFiles...)

	// Chỉ sử dụng Hardware Decoding (giải mã phần cứng) cho Nvidia CUDA.
	// Đối với Intel QSV và AMD, việc giải mã bằng CPU kết hợp với filter (chạy trên CPU)
	// rồi chuyển sang GPU mã hóa (encode) sẽ nhanh hơn nhiều, tránh thắt nút cổ chai do copy bộ nhớ GPU <-> CPU.
	var hwDecodeArgs []string
	if hwAccel == "nvidia" {
		hwDecodeArgs = []string{"-hwaccel", "cuda", "-hwaccel_output_format", "nv12"}
	}

	buildArgs := func(vcodec string, extraArgs []string, hwDecArgs []string) []string {
		args := []string{"-y", "-accurate_seek", "-fflags", "+genpts"}
		args = append(args, hwDecArgs...)
		args = append(args, "-ss", fmt.Sprintf("%.3f", effStart), "-i", inputPath)
		if e.Watermark.Enabled && e.Watermark.ImgPath != "" {
			args = append(args, "-i", e.Watermark.ImgPath)
		}
		if musicPathToUse != "" {
			if e.Audio.MusicLoop {
				args = append(args, "-stream_loop", "-1") // Lặp vô hạn nhạc nền
			}
			args = append(args, "-i", musicPathToUse)
		}
		if len(complexParts) > 0 {
			args = append(args, "-filter_complex", strings.Join(complexParts, ";"))
			args = append(args, "-map", vOut)
			if aOut != "" {
				args = append(args, "-map", aOut)
			}
		}
		args = append(args,
			"-t", fmt.Sprintf("%.3f", dur),
			"-c:v", vcodec,
		)
		args = append(args, extraArgs...)
		args = append(args,
			"-pix_fmt", "yuv420p",
			"-c:a", "aac",
			"-b:a", "128k",
			"-avoid_negative_ts", "make_zero",
			"-movflags", "+faststart",
		)
		if e.StripMeta {
			// Xóa toàn bộ metadata (title/encoder/creation_time) — FB/TikTok đọc các
			// trường này để nhận diện nguồn. -map_metadata -1 bỏ mọi metadata kế thừa.
			args = append(args, "-map_metadata", "-1")
		}
		if threads > 0 {
			args = append(args, "-threads", strconv.Itoa(threads))
		}
		if musicIdx >= 0 {
			args = append(args, "-shortest")
		}
		args = append(args, outputPath)
		return args
	}

	encoder, encoderArgs := getEncoderParams(hwAccel, preset, crf)

	// Try GPU encoder first (if not CPU)
	if encoder != "libx264" {
		cmdArgs := buildArgs(encoder, encoderArgs, hwDecodeArgs)
		cmd := exec.CommandContext(ctx, utils.GetBinPath("ffmpeg"), cmdArgs...)
		utils.HideCmdWindow(cmd)
		if _, err := cmd.CombinedOutput(); err == nil {
			return nil
		}
		// If fails, clean up partial output and fall back to CPU
		_ = os.Remove(outputPath)
	}

	// CPU fallback
	cmdArgs := buildArgs("libx264", []string{"-preset", preset, "-crf", strconv.Itoa(crf)}, nil)
	cmd := exec.CommandContext(ctx, utils.GetBinPath("ffmpeg"), cmdArgs...)
	utils.HideCmdWindow(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg cut error (CPU fallback): %v, output: %s", err, string(out))
	}
	return nil
}

// CutVideo cắt một clip từ startTime đến endTime và áp các thao tác chỉnh sửa
// trong clip.Edit.
//
// Chiến lược tối ưu tốc độ:
//   - Nếu clip KHÔNG có bất kỳ filter nào (tỷ lệ, màu, tốc độ, text, watermark...),
//     dùng stream-copy (-c copy) → nhanh gấp 5-20 lần, không mất chất lượng.
//   - Nếu stream-copy thất bại (codec không tương thích, container lỗi), tự động
//     fallback sang re-encode.
//   - Nếu clip CÓ filter → re-encode bằng libx264 với preset/crf cấu hình.
func CutVideo(ctx context.Context, inputPath string, clip project.Clip, outputPath string, preset string, crf int, threads int, hwAccel string, mode string) error {
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

	// Không cần filter → stream-copy (nhanh gấp nhiều lần)
	// Chỉ sử dụng stream-copy nếu không phải chế độ Precise (Kỹ), vì Precise yêu cầu chính xác tuyệt đối từng khung hình (bắt buộc re-encode).
	if mode != "precise" && !needsReencode(clip.Edit) {
		// Trim đầu/cuối vẫn áp được ở stream-copy (chỉ dịch -ss / giảm -t).
		scStart := clip.StartTime
		scDur := dur
		if clip.Edit.TrimStart > 0 {
			scStart += clip.Edit.TrimStart
			scDur -= clip.Edit.TrimStart
		}
		if clip.Edit.TrimEnd > 0 {
			scDur -= clip.Edit.TrimEnd
		}
		if scDur < 0.5 {
			// Trim quá lớn → bỏ trim, giữ nguyên clip.
			scStart = clip.StartTime
			scDur = dur
		}
		err := cutVideoStreamCopy(ctx, inputPath, scStart, scDur, outputPath, clip.Edit.StripMeta)
		if err == nil {
			return nil
		}
		// Stream-copy thất bại → fallback sang re-encode
		_ = os.Remove(outputPath)
	}

	return cutVideoReencode(ctx, inputPath, clip, outputPath, preset, crf, threads, hwAccel)
}

func getEncoderParams(hwAccel string, preset string, crf int) (encoder string, encoderArgs []string) {
	switch hwAccel {
	case "nvidia":
		encoder = "h264_nvenc"
		nvPreset := "p3"
		switch preset {
		case "ultrafast", "superfast", "veryfast", "faster", "fast":
			nvPreset = "p1" // Tốc độ tối đa cho Nvidia NVENC
		case "medium":
			nvPreset = "p3"
		case "slow", "slower":
			nvPreset = "p5"
		case "veryslow":
			nvPreset = "p7"
		}
		encoderArgs = []string{"-preset", nvPreset}
	case "intel":
		encoder = "h264_qsv"
		qsvPreset := "fast"
		if preset == "ultrafast" || preset == "superfast" || preset == "veryfast" || preset == "faster" || preset == "fast" {
			qsvPreset = "veryfast" // Tốc độ tối đa cho Intel QSV
		} else if preset == "slow" || preset == "slower" || preset == "veryslow" {
			qsvPreset = "slow"
		}
		encoderArgs = []string{"-preset", qsvPreset}
	case "amd":
		encoder = "h264_amf"
		amfQuality := "speed"
		if preset == "slow" || preset == "slower" || preset == "veryslow" {
			amfQuality = "quality"
		}
		encoderArgs = []string{"-quality", amfQuality}
	default:
		encoder = "libx264"
		cpuPreset := preset
		if preset == "fast" {
			cpuPreset = "superfast" // CPU fallback: tự động đổi sang superfast để nhanh hơn
		} else if preset == "faster" {
			cpuPreset = "ultrafast"
		}
		encoderArgs = []string{"-preset", cpuPreset, "-crf", strconv.Itoa(crf)}
	}
	return
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

	// === Nhóm xào nấu hình học (đặt TRƯỚC aspect để aspect chuẩn hóa khung cuối) ===

	// Cắt rìa: bỏ p% mỗi mép rồi scale lại về đúng kích thước cũ (nội dung không đổi
	// nhưng mọi pixel dịch → lệch bố cục so bản gốc). scale dùng iw/ih SAU crop nên
	// /(1-2p) đưa về đúng kích thước ban đầu.
	if e.Crop.Enabled && e.Crop.Percent > 0 && e.Crop.Percent < 0.45 {
		p := e.Crop.Percent
		keep := 1 - 2*p
		vSteps = append(vSteps,
			fmt.Sprintf("crop=iw*%s:ih*%s:iw*%s:ih*%s", trimFloat(keep), trimFloat(keep), trimFloat(p), trimFloat(p)),
			fmt.Sprintf("scale=iw/%s:ih/%s", trimFloat(keep), trimFloat(keep)))
	}

	// Xoay nhẹ vài độ + zoom bù (scale 1.10 rồi crop về giữa) để che góc đen.
	// Góc nhỏ (≤3°) thì 10% zoom là dư để lấp viền.
	if e.Rotate.Enabled && e.Rotate.Degrees != 0 {
		vSteps = append(vSteps,
			fmt.Sprintf("rotate=%s*PI/180:fillcolor=black", trimFloat(e.Rotate.Degrees)),
			"scale=iw*1.10:ih*1.10",
			"crop=iw/1.10:ih/1.10")
	}

	// Zoom tĩnh (Ken Burns dạng an toàn): scale lên Z rồi crop cửa sổ về kích thước cũ.
	// Không dùng filter zoompan vì nó reset PTS/fps → giật và lệch tiếng khi concat.
	// Dir chỉ dịch tâm cửa sổ crop (tĩnh), không pan động.
	if e.ZoomPan.Enabled && e.ZoomPan.Zoom > 1.0 {
		z := e.ZoomPan.Zoom
		x, y := "(in_w-out_w)/2", "(in_h-out_h)/2" // giữa
		switch e.ZoomPan.Dir {
		case "left":
			x = "0"
		case "right":
			x = "in_w-out_w"
		case "up":
			y = "0"
		case "down":
			y = "in_h-out_h"
		}
		vSteps = append(vSteps,
			fmt.Sprintf("scale=iw*%s:ih*%s", trimFloat(z), trimFloat(z)),
			fmt.Sprintf("crop=iw/%s:ih/%s:%s:%s", trimFloat(z), trimFloat(z), x, y))
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

	// Hạt nhiễu (grain) — đổi đặc trưng nén để né fingerprint. all=cường độ toàn khung.
	if e.Noise.Enabled && e.Noise.Strength > 0 {
		s := e.Noise.Strength
		if s > 60 {
			s = 60
		}
		vSteps = append(vSteps, fmt.Sprintf("noise=alls=%d:allf=t", s))
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

	// Phụ đề burn-in (hardsub) — render sau drawtext để nằm trên khung kích thước cuối.
	if e.Subtitle.Enabled && strings.TrimSpace(e.Subtitle.Path) != "" {
		vSteps = append(vSteps, subtitleFilter(e.Subtitle))
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
	// Đổi cao độ giọng (pitch) theo semitone — né audio fingerprint mà vẫn nghe tự nhiên.
	// rubberband đổi pitch mà KHÔNG đổi tốc độ; hệ số pitch = 2^(semitone/12).
	if e.Pitch != 0 {
		factor := math.Pow(2, e.Pitch/12.0)
		aSteps = append(aSteps, "rubberband=pitch="+trimFloat(factor))
	}
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

// subtitleFilter dựng filter subtitles= (hardsub qua libass) từ SubtitleOp.
// Dùng force_style để chỉnh font/màu/viền/lề. Màu ASS dạng &HBBGGRR (BGR, không RGB).
// Đường dẫn cần escape 2 lớp: ffFilterPath cho filtergraph (':' của ổ đĩa), và ở đây
// bọc trong dấu ' vì filename có thể chứa khoảng trắng.
func subtitleFilter(s project.SubtitleOp) string {
	size := s.FontSize
	if size <= 0 {
		size = 24
	}
	primary := s.FontColor
	if primary == "" {
		primary = "&Hffffff" // trắng
	}
	outline := s.OutlineCol
	if outline == "" {
		outline = "&H000000" // đen
	}
	marginV := s.MarginV
	if marginV < 0 {
		marginV = 0
	}
	// Tên font ưu tiên có sẵn trên Windows + phủ dấu tiếng Việt.
	style := fmt.Sprintf(
		"FontName=Arial,Fontsize=%d,PrimaryColour=%s,OutlineColour=%s,Outline=1,Shadow=0,MarginV=%d",
		size, primary, outline, marginV)
	return fmt.Sprintf("subtitles='%s':force_style='%s'", ffSubPath(s.Path), style)
}

// ffSubPath escape đường dẫn file phụ đề cho option subtitles=. Ngoài đổi '\'→'/'
// và escape ':' ổ đĩa như ffFilterPath, còn phải escape '\' còn lại cho libass.
func ffSubPath(p string) string {
	p = filepath.ToSlash(p)
	p = strings.ReplaceAll(p, "\\", "/")
	p = strings.ReplaceAll(p, ":", "\\:")
	return p
}

// ConcatClips ghép nhiều file video (đã cắt/chỉnh) thành một file duy nhất.
// transitionType == "" hoặc transitionDur <= 0 → nối cứng (concat, nhanh).
// Ngược lại → dùng xfade (video) + acrossfade (audio) tại mọi mối nối, thời lượng
// transition đồng nhất. Các input được chuẩn hóa về cùng khung/fps của clip đầu.
func ConcatClips(inputFiles []string, outputPath, transitionType string, transitionDur float64, preset string, crf int, hwAccel string) error {
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
		return concatDemuxer(inputFiles, outputPath, preset, crf, hwAccel)
	}
	return concatXfade(inputFiles, outputPath, transitionType, transitionDur, preset, crf, hwAccel)
}

// concatDemuxer nối cứng bằng concat filter (re-encode, an toàn với input cùng codec).
func concatDemuxer(inputFiles []string, outputPath, preset string, crf int, hwAccel string) error {
	// Chuẩn hóa về khung/fps của clip đầu để concat filter không lỗi lệch kích thước.
	w, h, fps := probeFrame(inputFiles[0])
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

	buildArgs := func(vcodec string, extraArgs []string) []string {
		cmdArgs := []string{"-y"}
		for _, f := range inputFiles {
			cmdArgs = append(cmdArgs, "-i", f)
		}
		cmdArgs = append(cmdArgs,
			"-filter_complex", filter,
			"-map", "[vout]", "-map", "[aout]",
			"-c:v", vcodec,
		)
		cmdArgs = append(cmdArgs, extraArgs...)
		cmdArgs = append(cmdArgs,
			"-pix_fmt", "yuv420p", "-c:a", "aac", "-b:a", "128k",
			"-movflags", "+faststart", outputPath)
		return cmdArgs
	}

	encoder, encoderArgs := getEncoderParams(hwAccel, preset, crf)
	if encoder != "libx264" {
		cmdArgs := buildArgs(encoder, encoderArgs)
		cmd := exec.Command(utils.GetBinPath("ffmpeg"), cmdArgs...)
		utils.HideCmdWindow(cmd)
		if err := cmd.Run(); err == nil {
			return nil
		}
		_ = os.Remove(outputPath)
	}

	// Fallback to CPU
	cmdArgs := buildArgs("libx264", []string{"-preset", preset, "-crf", strconv.Itoa(crf)})
	cmd := exec.Command(utils.GetBinPath("ffmpeg"), cmdArgs...)
	utils.HideCmdWindow(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg concat error (CPU fallback): %v, output: %s", err, string(out))
	}
	return nil
}

// concatXfade nối các clip với hiệu ứng chuyển cảnh xfade/acrossfade đồng nhất.
func concatXfade(inputFiles []string, outputPath, transitionType string, td float64, preset string, crf int, hwAccel string) error {
	w, h, fps := probeFrame(inputFiles[0])
	durs := make([]float64, len(inputFiles))
	for i, f := range inputFiles {
		if info, e := media.GetVideoInfo(f); e == nil && info != nil {
			durs[i] = info.Duration
		}
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

	buildArgs := func(vcodec string, extraArgs []string) []string {
		cmdArgs := []string{"-y"}
		for _, f := range inputFiles {
			cmdArgs = append(cmdArgs, "-i", f)
		}
		cmdArgs = append(cmdArgs,
			"-filter_complex", filter,
			"-map", prevV, "-map", prevA,
			"-c:v", vcodec,
		)
		cmdArgs = append(cmdArgs, extraArgs...)
		cmdArgs = append(cmdArgs,
			"-pix_fmt", "yuv420p", "-c:a", "aac", "-b:a", "128k",
			"-movflags", "+faststart", outputPath)
		return cmdArgs
	}

	encoder, encoderArgs := getEncoderParams(hwAccel, preset, crf)
	if encoder != "libx264" {
		cmdArgs := buildArgs(encoder, encoderArgs)
		cmd := exec.Command(utils.GetBinPath("ffmpeg"), cmdArgs...)
		utils.HideCmdWindow(cmd)
		if err := cmd.Run(); err == nil {
			return nil
		}
		_ = os.Remove(outputPath)
	}

	// Fallback to CPU
	cmdArgs := buildArgs("libx264", []string{"-preset", preset, "-crf", strconv.Itoa(crf)})
	cmd := exec.Command(utils.GetBinPath("ffmpeg"), cmdArgs...)
	utils.HideCmdWindow(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg xfade error (CPU fallback): %v, output: %s", err, string(out))
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

// PrependThumbnailIntro ghép một đoạn intro (ảnh thumbnail đứng yên introDur giây,
// audio IM LẶNG) vào ĐẦU clip video, xuất ra outputPath. Toàn bộ được re-encode để
// intro và clip đồng nhất khung/fps/codec (concat filter yêu cầu điều này).
//
// Khung/fps chuẩn hóa theo chính clip (probeFrame) nên intro luôn khớp tỷ lệ clip.
// Audio intro là anullsrc (im lặng) — theo lựa chọn "im lặng khi hiện thumbnail".
// Audio clip giữ nguyên. Nếu clip không có audio track, dùng nhánh fallback nối
// silent cho cả clip để concat không lỗi.
func PrependThumbnailIntro(ctx context.Context, clipPath, imagePath, outputPath string, introDur float64, preset string, crf int, hwAccel string) error {
	if introDur <= 0 {
		introDur = 2.0
	}
	if preset == "" {
		preset = "fast"
	}
	if crf <= 0 || crf > 51 {
		crf = 23
	}

	w, h, fps := probeFrame(clipPath)

	// Clip có audio hay không → quyết định cách map audio.
	clipHasAudio := false
	if info, err := media.GetVideoInfo(clipPath); err == nil && info != nil {
		clipHasAudio = info.HasAudio
	}

	scalePad := fmt.Sprintf("scale=%d:%d:force_original_aspect_ratio=decrease,pad=%d:%d:(ow-iw)/2:(oh-ih)/2,setsar=1,fps=%s", w, h, w, h, fps)

	var filter string
	if clipHasAudio {
		// [0]=ảnh loop, [1]=silent audio intro, [2]=clip. Nối intro(v+silent) + clip(v+a).
		filter = fmt.Sprintf(
			"[0:v]%s[iv];[2:v]%s[cv];[iv][1:a][cv][2:a]concat=n=2:v=1:a=1[vout][aout]",
			scalePad, scalePad)
	} else {
		// Clip không audio → silent cho cả intro lẫn clip để track audio đồng nhất.
		filter = fmt.Sprintf(
			"[0:v]%s[iv];[2:v]%s[cv];[iv][1:a][cv][3:a]concat=n=2:v=1:a=1[vout][aout]",
			scalePad, scalePad)
	}

	buildArgs := func(vcodec string, extraArgs []string) []string {
		args := []string{
			"-y",
			"-loop", "1", "-t", fmt.Sprintf("%.3f", introDur), "-i", imagePath,
			"-f", "lavfi", "-t", fmt.Sprintf("%.3f", introDur), "-i", "anullsrc=channel_layout=stereo:sample_rate=44100",
			"-i", clipPath,
		}
		if !clipHasAudio {
			// Input [3]: silent audio phủ toàn clip (dài dư cũng được, concat cắt theo video).
			args = append(args, "-f", "lavfi", "-i", "anullsrc=channel_layout=stereo:sample_rate=44100")
		}
		args = append(args,
			"-filter_complex", filter,
			"-map", "[vout]", "-map", "[aout]",
			"-c:v", vcodec,
		)
		args = append(args, extraArgs...)
		args = append(args,
			"-pix_fmt", "yuv420p", "-c:a", "aac", "-b:a", "128k",
			"-movflags", "+faststart", outputPath)
		return args
	}

	encoder, encoderArgs := getEncoderParams(hwAccel, preset, crf)
	if encoder != "libx264" {
		cmd := exec.CommandContext(ctx, utils.GetBinPath("ffmpeg"), buildArgs(encoder, encoderArgs)...)
		utils.HideCmdWindow(cmd)
		if err := cmd.Run(); err == nil {
			return nil
		}
		_ = os.Remove(outputPath)
	}

	cmd := exec.CommandContext(ctx, utils.GetBinPath("ffmpeg"), buildArgs("libx264", []string{"-preset", preset, "-crf", strconv.Itoa(crf)})...)
	utils.HideCmdWindow(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg prepend intro error: %v, output: %s", err, string(out))
	}
	return nil
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

// mergeMusicTracks ghép nối tiếp nhiều bài nhạc nền thành một file MP3 duy nhất.
func mergeMusicTracks(ctx context.Context, tracks []string) (string, error) {
	if len(tracks) == 0 {
		return "", nil
	}
	if len(tracks) == 1 {
		return tracks[0], nil
	}

	// Đảm bảo thư mục bin chứa ffmpeg hoạt động
	ffmpegPath := utils.GetBinPath("ffmpeg")

	// Tạo file nhạc ghép tạm
	tempDir := os.TempDir()
	mergedPath := filepath.Join(tempDir, fmt.Sprintf("merged_bg_music_%d.mp3", time.Now().UnixNano()))

	args := []string{"-y"}
	for _, t := range tracks {
		args = append(args, "-i", t)
	}

	// Xây dựng filter_complex concat
	var filterInputs []string
	for i := range tracks {
		filterInputs = append(filterInputs, fmt.Sprintf("[%d:a]", i))
	}
	concatFilter := strings.Join(filterInputs, "") + fmt.Sprintf("concat=n=%d:v=0:a=1[outa]", len(tracks))
	args = append(args, "-filter_complex", concatFilter, "-map", "[outa]", "-c:a", "libmp3lame", "-b:a", "192k", mergedPath)

	cmd := exec.CommandContext(ctx, ffmpegPath, args...)
	utils.HideCmdWindow(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("lỗi ghép nhạc nền: %v, output: %s", err, string(out))
	}
	return mergedPath, nil
}
