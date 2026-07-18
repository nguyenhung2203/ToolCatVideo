package media

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
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
		"-select_streams", "v:0",
		"-show_entries", "stream=width,height,r_frame_rate,duration,time_base",
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
	
	if len(probeResult.Streams) == 0 {
		return nil, fmt.Errorf("no video stream found")
	}
	
	stream := probeResult.Streams[0]
	
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
		args = append(args, "-vf", fmt.Sprintf("scale=-2:240,fps=%s", fpsStr))
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
		hwDecodeArgs = []string{"-hwaccel", "cuda", "-hwaccel_output_format", "nv12"}
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

	// CPU encoder fallback
	cmdArgs := buildCmdArgs("libx264", []string{"-preset", "ultrafast", "-crf", "30"}, nil)
	return runWithProgress(cmdArgs)
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
		"-vn", // bỏ video
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

// FindNearestKeyframe tìm I-frame (keyframe) gần nhất trước hoặc sau timeSec.
// Dùng ffprobe -read_intervals để quét một cửa sổ ±windowSec quanh timeSec,
// lọc chỉ lấy key_frame=1, rồi chọn frame có PTS gần nhất.
//
// Hữu ích cho stream-copy: cắt tại keyframe tránh artifact (xanh lá, glitch)
// ở đầu clip mà không cần re-encode.
//
// Trả về timestamp keyframe gần nhất. Nếu không tìm được, trả về timeSec gốc.
func FindNearestKeyframe(filePath string, timeSec float64) float64 {
	windowSec := 3.0 // quét ±3 giây
	start := timeSec - windowSec
	if start < 0 {
		start = 0
	}
	end := timeSec + windowSec

	// ffprobe -read_intervals START%END : quét đoạn [start, end]
	// -select_streams v:0 : chỉ video stream đầu
	// -show_frames : liệt kê frame
	// -show_entries frame=pts_time,key_frame : chỉ lấy 2 trường
	// -of csv=p=0 : output CSV gọn
	interval := fmt.Sprintf("%.3f%%%.3f", start, end)
	cmdArgs := []string{
		"-v", "error",
		"-read_intervals", interval,
		"-select_streams", "v:0",
		"-show_frames",
		"-show_entries", "frame=pts_time,key_frame",
		"-of", "csv=p=0",
		filePath,
	}

	cmd := exec.Command(utils.GetBinPath("ffprobe"), cmdArgs...)
	utils.HideCmdWindow(cmd)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return timeSec // fallback: giữ nguyên timestamp
	}

	bestTs := timeSec
	bestDist := windowSec + 1 // bắt đầu lớn hơn window

	scanner := bufio.NewScanner(&out)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		// Format: pts_time,key_frame  (VD: "12.345,1")
		parts := strings.SplitN(line, ",", 2)
		if len(parts) != 2 {
			continue
		}
		isKey := strings.TrimSpace(parts[1])
		if isKey != "1" {
			continue // bỏ qua frame không phải keyframe
		}
		pts, err := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
		if err != nil {
			continue
		}
		dist := pts - timeSec
		if dist < 0 {
			dist = -dist
		}
		if dist < bestDist {
			bestDist = dist
			bestTs = pts
		}
	}

	return bestTs
}

