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
func GenerateProxy(ctx context.Context, inputPath string, outputPath string, fpsStr string) error {
	if fpsStr == "" {
		fpsStr = "15"
	}
	// Giảm xuống 240p + fps=N filter: drop/duplicate frame nhưng giữ PTS gốc.
	cmdArgs := []string{
		"-y", // overwrite
		"-i", inputPath,
		"-vf", fmt.Sprintf("scale=-2:240,fps=%s", fpsStr),
		"-c:v", "libx264",
		"-preset", "ultrafast",
		"-crf", "30",
		"-an", // bỏ audio
		"-fps_mode", "vfr", // giữ VFR để PTS không bị force lại
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
		return fmt.Errorf("ffmpeg proxy error: %v, detail: %s", err, strings.TrimSpace(stderr.String()))
	}
	return nil
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

