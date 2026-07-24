package media

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"video-splitter/internal/project"
	"video-splitter/internal/utils"
)

// DetectGPU tự động phát hiện loại GPU hỗ trợ mã hóa phần cứng trên máy người dùng.
// Thử lần lượt NVIDIA → Intel → AMD. Trả về "nvidia", "intel", "amd" hoặc "none".
// Kết quả được cache lại sau lần gọi đầu tiên để không phải thử lại nhiều lần.
var cachedGPU *string

func DetectGPU() string {
	if cachedGPU != nil {
		return *cachedGPU
	}

	ffmpegPath := utils.GetBinPath("ffmpeg")

	// Thử NVIDIA NVENC
	cmd := exec.Command(ffmpegPath, "-y", "-f", "lavfi", "-i", "nullsrc=s=64x64:d=0.1", "-c:v", "h264_nvenc", "-preset", "p1", "-f", "null", "-")
	utils.HideCmdWindow(cmd)
	if err := cmd.Run(); err == nil {
		result := "nvidia"
		cachedGPU = &result
		return result
	}

	// Thử Intel QSV
	cmd = exec.Command(ffmpegPath, "-y", "-f", "lavfi", "-i", "nullsrc=s=64x64:d=0.1", "-c:v", "h264_qsv", "-preset", "veryfast", "-f", "null", "-")
	utils.HideCmdWindow(cmd)
	if err := cmd.Run(); err == nil {
		result := "intel"
		cachedGPU = &result
		return result
	}

	// Thử AMD AMF
	cmd = exec.Command(ffmpegPath, "-y", "-f", "lavfi", "-i", "nullsrc=s=64x64:d=0.1", "-c:v", "h264_amf", "-quality", "speed", "-f", "null", "-")
	utils.HideCmdWindow(cmd)
	if err := cmd.Run(); err == nil {
		result := "amd"
		cachedGPU = &result
		return result
	}

	result := "none"
	cachedGPU = &result
	return result
}

// GetVideoInfo sử dụng ffprobe để lấy metadata của video.
// Đọc duration từ stream trước; nếu stream không có (nhiều MP4) thì fallback sang
// format duration để đảm bảo luôn có thời lượng chính xác.
func GetVideoInfo(filePath string) (*project.VideoInfo, error) {
	cmdArgs := []string{
		"-v", "error",
		"-show_entries", "stream=codec_type,width,height,r_frame_rate,duration,time_base",
		"-show_entries", "format=duration",
		"-of", "json",
		filePath,
	}

	cmd := exec.Command(utils.GetBinPath("ffprobe"), cmdArgs...)
	utils.HideCmdWindow(cmd)
	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		return nil, fmt.Errorf("ffprobe error: %v, stderr: %s", err, stderr.String())
	}

	var probeResult struct {
		Streams []struct {
			CodecType  string `json:"codec_type"`
			Width      int    `json:"width"`
			Height     int    `json:"height"`
			RFrameRate string `json:"r_frame_rate"`
			Duration   string `json:"duration"`
			TimeBase   string `json:"time_base"`
		} `json:"streams"`
		Format struct {
			Duration string `json:"duration"`
		} `json:"format"`
	}

	if err := json.Unmarshal(out.Bytes(), &probeResult); err != nil {
		return nil, fmt.Errorf("failed to parse ffprobe json: %v", err)
	}

	// Tách video stream đầu tiên + kiểm tra có audio stream không.
	var stream *struct {
		CodecType  string `json:"codec_type"`
		Width      int    `json:"width"`
		Height     int    `json:"height"`
		RFrameRate string `json:"r_frame_rate"`
		Duration   string `json:"duration"`
		TimeBase   string `json:"time_base"`
	}
	hasAudio := false
	for i := range probeResult.Streams {
		s := &probeResult.Streams[i]
		switch s.CodecType {
		case "video":
			if stream == nil {
				stream = s
			}
		case "audio":
			hasAudio = true
		}
	}

	if stream == nil {
		return nil, fmt.Errorf("no video stream found")
	}

	// Tính fps từ dạng phân số "30000/1001"
	fps := 0.0
	if strings.Contains(stream.RFrameRate, "/") {
		parts := strings.Split(stream.RFrameRate, "/")
		if len(parts) == 2 {
			num, _ := strconv.ParseFloat(parts[0], 64)
			den, _ := strconv.ParseFloat(parts[1], 64)
			if den != 0 {
				fps = num / den
			}
		}
	} else {
		fps, _ = strconv.ParseFloat(stream.RFrameRate, 64)
	}

	// Ưu tiên duration từ stream; fallback sang format duration nếu stream không có.
	duration, _ := strconv.ParseFloat(stream.Duration, 64)
	if duration <= 0 {
		duration, _ = strconv.ParseFloat(probeResult.Format.Duration, 64)
	}

	var sizeByte int64
	fileInfo, err := os.Stat(filePath)
	if err == nil {
		sizeByte = fileInfo.Size()
	}

	info := &project.VideoInfo{
		SourcePath: filePath,
		Duration:   duration,
		Width:      stream.Width,
		Height:     stream.Height,
		FPS:        fps,
		TimeBase:   stream.TimeBase,
		SizeByte:   sizeByte,
		HasAudio:   hasAudio,
	}

	return info, nil
}

// GenerateProxy tạo một video proxy phân giải thấp để phân tích nhanh.
// Dùng filter fps=N (thay vì -r N) để giữ nguyên PTS → timestamp trên proxy
// khớp chính xác với video gốc, tránh scenedetect/layout bị lệch thời gian.
//
// totalDuration: thời lượng video gốc (giây), dùng để tính % tiến độ.
// progressFn: callback nhận phần trăm tiến độ (0-100), có thể nil nếu không cần.
func GenerateProxy(ctx context.Context, inputPath string, outputPath string, fpsStr string, hwAccel string, totalDuration float64, progressFn func(percent int)) error {
	if fpsStr == "" {
		fpsStr = "15"
	}
	
	// Determine the encoder based on hwAccel
	var encoder string
	var encoderArgs []string
	switch hwAccel {
	case "nvidia":
		encoder = "h264_nvenc"
		encoderArgs = []string{"-preset", "p1"}
	case "intel":
		encoder = "h264_qsv"
		encoderArgs = []string{"-preset", "veryfast"}
	case "amd":
		encoder = "h264_amf"
		encoderArgs = []string{"-quality", "speed"}
	default:
		encoder = "libx264"
		encoderArgs = []string{"-preset", "ultrafast", "-crf", "30"}
	}

	buildCmdArgs := func(vcodec string, extraArgs []string, hwDecodeArgs []string) []string {
		args := []string{"-y"} // overwrite
		// Thêm tham số giải mã bằng GPU (hardware decode) trước -i để tăng tốc đọc video gốc
		args = append(args, hwDecodeArgs...)
		args = append(args, "-i", inputPath)
		args = append(args, "-vf", fmt.Sprintf("scale=320:180,fps=%s", fpsStr))
		args = append(args, "-c:v", vcodec)
		args = append(args, extraArgs...)
		args = append(args,
			"-an",       // bỏ audio
			"-fps_mode", "vfr", // giữ VFR để PTS không bị force lại
			outputPath,
		)
		return args
	}

	// Xác định tham số giải mã bằng GPU (hardware decode) để tăng tốc Bước 1
	var hwDecodeArgs []string
	switch hwAccel {
	case "nvidia":
		// Không dùng -hwaccel_output_format nv12 — filter scale= không xử lý được nv12 từ CUDA
		// Dùng -hwaccel cuda đơn thuần: decode bằng GPU, tự đổi về CPU memory cho filter
		hwDecodeArgs = []string{"-hwaccel", "cuda"}
	case "intel":
		hwDecodeArgs = []string{"-hwaccel", "qsv"}
	case "amd":
		hwDecodeArgs = []string{"-hwaccel", "auto"}
	}

	// runWithProgress chạy FFmpeg và parse stderr để báo tiến độ thời gian thực
	runWithProgress := func(cmdArgs []string) error {
		cmd := exec.CommandContext(ctx, utils.GetBinPath("ffmpeg"), cmdArgs...)
		utils.HideCmdWindow(cmd)

		stderrPipe, err := cmd.StderrPipe()
		if err != nil {
			return fmt.Errorf("lỗi tạo stderr pipe: %v", err)
		}

		if err := cmd.Start(); err != nil {
			return fmt.Errorf("lỗi khởi động ffmpeg: %v", err)
		}

		// Custom split function để split theo cả \n và \r (vì FFmpeg tiến độ dùng \r)
		splitFn := func(data []byte, atEOF bool) (advance int, token []byte, err error) {
			if atEOF && len(data) == 0 {
				return 0, nil, nil
			}
			for i := 0; i < len(data); i++ {
				if data[i] == '\r' || data[i] == '\n' {
					return i + 1, data[:i], nil
				}
			}
			if atEOF {
				return len(data), data, nil
			}
			return 0, nil, nil
		}

		// Parse stderr theo thời gian thực để trích xuất tiến độ
		var lastStderr strings.Builder
		scanner := bufio.NewScanner(stderrPipe)
		// Tăng buffer size lên 1MB đề phòng dòng log cực dài
		buf := make([]byte, 64*1024)
		scanner.Buffer(buf, 1024*1024)
		scanner.Split(splitFn)

		lastPercent := 0
		for scanner.Scan() {
			line := scanner.Text()
			lastStderr.WriteString(line + "\n")

			// FFmpeg ghi tiến độ dạng: "frame= 100 fps=30 ... time=00:03:22.43 ..."
			if progressFn != nil && totalDuration > 0 && strings.Contains(line, "time=") {
				if idx := strings.Index(line, "time="); idx >= 0 {
					timeStr := line[idx+5:]
					if spaceIdx := strings.IndexAny(timeStr, " \t"); spaceIdx > 0 {
						timeStr = timeStr[:spaceIdx]
					}
					if secs := parseFFmpegTime(timeStr); secs > 0 {
						pct := int(secs / totalDuration * 100)
						if pct > 100 {
							pct = 100
						}
						if pct > lastPercent {
							lastPercent = pct
							progressFn(pct)
						}
					}
				}
			}
		}

		if scanErr := scanner.Err(); scanErr != nil {
			// Ghi nhận lỗi quét ra log nếu có
			fmt.Printf("[FFmpeg Scan Log Error]: %v\n", scanErr)
		}

		if err := cmd.Wait(); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return fmt.Errorf("ffmpeg proxy error: %v, detail: %s", err, strings.TrimSpace(lastStderr.String()))
		}
		return nil
	}

	// Try GPU encoder first (if not CPU)
	if encoder != "libx264" {
		cmdArgs := buildCmdArgs(encoder, encoderArgs, hwDecodeArgs)
		if err := runWithProgress(cmdArgs); err == nil {
			return nil
		}
		// If fails, we fall back to CPU
	}

	// CPU encoder fallback. We still preserve hardware decoding (hwDecodeArgs) if available
	// to offload decoding to GPU (e.g. cuda/qsv) even if encoding (nvenc) fails.
	if len(hwDecodeArgs) > 0 {
		cmdArgs := buildCmdArgs("libx264", []string{"-preset", "ultrafast", "-crf", "30"}, hwDecodeArgs)
		if err := runWithProgress(cmdArgs); err == nil {
			return nil
		}
	}

	// absolute fallback: CPU decoding and CPU encoding
	cmdArgsFallback := buildCmdArgs("libx264", []string{"-preset", "ultrafast", "-crf", "30"}, nil)
	return runWithProgress(cmdArgsFallback)
}

// parseFFmpegTime chuyển đổi chuỗi thời gian FFmpeg "HH:MM:SS.ms" thành giây.
func parseFFmpegTime(s string) float64 {
	s = strings.TrimSpace(s)
	// Xử lý giá trị âm hoặc N/A
	if s == "" || s == "N/A" || strings.HasPrefix(s, "-") {
		return 0
	}
	parts := strings.Split(s, ":")
	if len(parts) != 3 {
		return 0
	}
	h, _ := strconv.ParseFloat(parts[0], 64)
	m, _ := strconv.ParseFloat(parts[1], 64)
	sec, _ := strconv.ParseFloat(parts[2], 64)
	return h*3600 + m*60 + sec
}

// ExtractAudio trích xuất âm thanh thành file .wav cho worker Python phân tích
func ExtractAudio(ctx context.Context, inputPath string, outputPath string) error {
	cmdArgs := []string{
		"-y",
		"-i", inputPath,
		"-vn",             // bỏ video
		"-map", "0:a:0?",  // lấy TƯỜNG MINH track audio đầu tiên (video có thể nhiều track); '?' = không lỗi nếu thiếu
		"-acodec", "pcm_s16le",
		"-ar", "16000",
		"-ac", "1", // mono để dễ phân tích silence/energy
		outputPath,
	}
	
	cmd := exec.CommandContext(ctx, utils.GetBinPath("ffmpeg"), cmdArgs...)
	utils.HideCmdWindow(cmd)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("ffmpeg extract audio error: %v, detail: %s", err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

// ExtractFrame trích xuất 1 khung hình tại giây ss để làm thumbnail
func ExtractFrame(ctx context.Context, inputPath string, timeSec float64, outputPath string) error {
	cmdArgs := []string{
		"-y",
		"-ss", fmt.Sprintf("%.3f", timeSec),
		"-i", inputPath,
		"-vframes", "1",
		"-f", "image2",
		"-q:v", "5", // chất lượng ảnh (2-31, 5 là vừa phải)
		outputPath,
	}
	
	cmd := exec.CommandContext(ctx, utils.GetBinPath("ffmpeg"), cmdArgs...)
	utils.HideCmdWindow(cmd)
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("ffmpeg extract frame error: %v", err)
	}
	return nil
}

// IsBlackOrDarkImage giải mã file ảnh và tính độ sáng trung bình (Luminance) để xác định xem khung hình có bị đen/tối không
func IsBlackOrDarkImage(imagePath string) (bool, error) {
	f, err := os.Open(imagePath)
	if err != nil {
		return false, err
	}
	defer f.Close()

	img, _, err := image.Decode(f)
	if err != nil {
		return false, err
	}

	bounds := img.Bounds()
	width, height := bounds.Max.X - bounds.Min.X, bounds.Max.Y - bounds.Min.Y
	if width <= 0 || height <= 0 {
		return true, nil
	}

	var totalLum float64
	var darkPixels int
	var sampleCount int

	step := 4
	if width > 1000 || height > 1000 {
		step = 8
	}

	for y := bounds.Min.Y; y < bounds.Max.Y; y += step {
		for x := bounds.Min.X; x < bounds.Max.X; x += step {
			r, g, b, _ := img.At(x, y).RGBA()
			r8, g8, b8 := float64(r>>8), float64(g>>8), float64(b>>8)
			
			lum := 0.299*r8 + 0.587*g8 + 0.114*b8
			totalLum += lum
			sampleCount++

			if lum < 20.0 {
				darkPixels++
			}
		}
	}

	if sampleCount == 0 {
		return true, nil
	}

	avgLum := totalLum / float64(sampleCount)
	darkRatio := float64(darkPixels) / float64(sampleCount)

	if avgLum < 18.0 || (darkRatio > 0.85 && avgLum < 30.0) {
		return true, nil
	}

	return false, nil
}

// ExtractClearFrame cắt 1 khung hình đẹp, tự động bỏ qua các khung hình bị đen/tối.
// startOffset là mốc BẮT ĐẦU clip trong video gốc (giây): mọi timestamp trích frame
// đều tính TỪ startOffset trở đi, để mỗi clip lấy frame trong ĐÚNG đoạn của nó. Nếu
// bỏ qua (0) thì trích từ đầu video — khiến các clip cùng độ dài ra frame giống hệt.
func ExtractClearFrame(ctx context.Context, inputPath string, startOffset float64, duration float64, outputPath string) (string, error) {
	var timestamps []float64

	if duration > 0 {
		timestamps = []float64{
			startOffset + duration*0.30,
			startOffset + duration*0.50,
			startOffset + duration*0.70,
			startOffset + duration*0.15,
			startOffset + 0.5,
		}
	} else {
		timestamps = []float64{startOffset + 1.0, startOffset + 2.0, startOffset + 0.5}
	}

	tmpPath := outputPath + ".tmp.jpg"
	defer func() {
		_ = os.Remove(tmpPath)
	}()

	var lastErr error
	for _, ts := range timestamps {
		if ts < 0 {
			ts = 0.5
		}
		err := ExtractFrame(ctx, inputPath, ts, tmpPath)
		if err != nil {
			lastErr = err
			continue
		}

		isDark, errDark := IsBlackOrDarkImage(tmpPath)
		if errDark == nil && !isDark {
			_ = os.Remove(outputPath)
			errMove := os.Rename(tmpPath, outputPath)
			if errMove == nil {
				fmt.Printf("[ExtractClearFrame] Trích xuất khung hình rõ nét tại %.2fs cho %s\n", ts, outputPath)
				return outputPath, nil
			}
		}
	}

	fallbackTs := startOffset + 0.5
	if duration > 0 {
		fallbackTs = startOffset + duration*0.5
	}
	errFallback := ExtractFrame(ctx, inputPath, fallbackTs, outputPath)
	if errFallback != nil && lastErr != nil {
		return "", lastErr
	}
	return outputPath, nil
}

// LoadAllKeyframes dùng ffprobe để đọc toàn bộ danh sách keyframe pts_time của video.
func LoadAllKeyframes(filePath string) ([]float64, error) {
	cmdArgs := []string{
		"-v", "error",
		"-select_streams", "v:0",
		"-skip_frame", "nokey",
		"-show_entries", "frame=pts_time",
		"-of", "csv=p=0",
		filePath,
	}

	cmd := exec.Command(utils.GetBinPath("ffprobe"), cmdArgs...)
	utils.HideCmdWindow(cmd)
	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ffprobe error: %v, stderr: %s", err, stderr.String())
	}

	var keyframes []float64
	scanner := bufio.NewScanner(&out)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		pts, err := strconv.ParseFloat(line, 64)
		if err == nil {
			keyframes = append(keyframes, pts)
		}
	}

	return keyframes, nil
}

// FindNearestKeyframeCached tìm keyframe gần nhất với timeSec từ danh sách keyframe đã load sẵn.
func FindNearestKeyframeCached(keyframes []float64, timeSec float64) float64 {
	if len(keyframes) == 0 {
		return timeSec
	}

	idx := sort.Search(len(keyframes), func(i int) bool {
		return keyframes[i] >= timeSec
	})

	if idx == 0 {
		return keyframes[0]
	}
	if idx == len(keyframes) {
		return keyframes[len(keyframes)-1]
	}

	prev := keyframes[idx-1]
	curr := keyframes[idx]
	if (timeSec - prev) < (curr - timeSec) {
		return prev
	}
	return curr
}

// FindNearestKeyframe tìm I-frame (keyframe) gần nhất trước hoặc sau timeSec.
func FindNearestKeyframe(filePath string, timeSec float64) float64 {
	kf, err := LoadAllKeyframes(filePath)
	if err != nil {
		return timeSec
	}
	return FindNearestKeyframeCached(kf, timeSec)
}

