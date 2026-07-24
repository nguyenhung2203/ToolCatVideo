// Package subtitle lo việc tạo phụ đề tự động: gọi Python worker (faster-whisper)
// để nghe tiếng ra segment có timestamp, rồi dựng file .srt để burn vào clip.
//
// Việc DỊCH sang ngôn ngữ khác (Gemini) nằm ở app.go vì cần API key + HTTP; package
// này chỉ lo transcribe + dựng/ghi SRT (thuần, dễ test, không phụ thuộc wails runtime).
package subtitle

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

	"video-splitter/internal/utils"
)

// Segment là một câu phụ đề với mốc thời gian (giây).
type Segment struct {
	Start float64 `json:"start"`
	End   float64 `json:"end"`
	Text  string  `json:"text"`
}

// transcribeResult khớp JSON worker in ra ở nhánh --transcribe.
type transcribeResult struct {
	Status   string    `json:"status"`
	Language string    `json:"language"`
	Segments []Segment `json:"segments"`
	Message  string    `json:"message"`
}

// ProgressFn nhận (phần trăm 0-100, thông điệp trạng thái). msg rỗng nếu chỉ cập nhật %.
type ProgressFn func(pct int, msg string)

// Transcribe gọi worker nghe tiếng trong mediaPath (audio hoặc video).
// Nếu clipStart/clipEnd > 0 và clipEnd>clipStart → chỉ nghe đoạn đó (timestamp 0-based
// theo đoạn). language="auto" để tự nhận diện. Trả segments + ngôn ngữ đã nhận diện.
func Transcribe(ctx context.Context, mediaPath, language, model, modelDir string,
	clipStart, clipEnd float64, onProgress ProgressFn) ([]Segment, string, error) {

	if language == "" {
		language = "auto"
	}
	if model == "" {
		model = "small"
	}

	args := []string{
		"--transcribe", mediaPath,
		"--language", language,
		"--whisper-model", model,
		"--ffmpeg", utils.GetBinPath("ffmpeg"),
	}
	if modelDir != "" {
		args = append(args, "--model-dir", modelDir)
	}
	if clipEnd > clipStart {
		args = append(args,
			"--clip-start", strconv.FormatFloat(clipStart, 'f', 3, 64),
			"--clip-end", strconv.FormatFloat(clipEnd, 'f', 3, 64))
	}

	// Quyết định exe: ưu tiên script dev (nếu có Python), fallback worker.exe đóng gói.
	// Cùng logic với boundary.AnalyzeVideo để hành vi nhất quán dev/build.
	exePath, cmdArgs := resolveWorkerCommand(args)

	cmd := exec.CommandContext(ctx, exePath, cmdArgs...)
	utils.HideCmdWindow(cmd)
	cmd.Env = append(os.Environ(), "PYTHONUTF8=1", "PYTHONIOENCODING=utf-8")
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		kill := exec.Command("taskkill", "/F", "/T", "/PID", strconv.Itoa(cmd.Process.Pid))
		utils.HideCmdWindow(kill)
		return kill.Run()
	}

	var stderrBuf bytes.Buffer
	cmd.Stderr = &stderrBuf

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, "", fmt.Errorf("lỗi tạo stdout pipe: %v", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, "", fmt.Errorf("lỗi khởi động worker phụ đề: %v", err)
	}

	var jsonOut strings.Builder
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 64*1024), 10*1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "PROGRESS:"):
			if onProgress != nil {
				if p, e := strconv.Atoi(strings.TrimPrefix(line, "PROGRESS:")); e == nil {
					onProgress(p, "")
				}
			}
		case strings.HasPrefix(line, "STATUS_LOG:"):
			if onProgress != nil {
				onProgress(-1, strings.TrimPrefix(line, "STATUS_LOG:"))
			}
		case strings.HasPrefix(line, "DETECTOR_DONE:"):
			// không dùng cho transcribe
		default:
			jsonOut.WriteString(line)
		}
	}

	waitErr := cmd.Wait()

	var res transcribeResult
	unmarshalErr := json.Unmarshal([]byte(jsonOut.String()), &res)

	if unmarshalErr == nil && res.Status == "error" && res.Message != "" {
		return nil, "", fmt.Errorf("%s", res.Message)
	}
	if waitErr != nil {
		if ctx.Err() != nil {
			return nil, "", ctx.Err()
		}
		detail := strings.TrimSpace(stderrBuf.String())
		if detail != "" {
			return nil, "", fmt.Errorf("worker phụ đề lỗi: %v\n%s", waitErr, detail)
		}
		return nil, "", fmt.Errorf("worker phụ đề lỗi: %v", waitErr)
	}
	if unmarshalErr != nil {
		return nil, "", fmt.Errorf("lỗi parse JSON phụ đề: %v, output: %s", unmarshalErr, jsonOut.String())
	}
	if res.Status != "success" {
		return nil, "", fmt.Errorf("worker phụ đề trả lỗi: %s", res.Message)
	}

	return res.Segments, res.Language, nil
}

// resolveWorkerCommand chọn cách chạy worker: script Python dev (nếu có python) hoặc
// worker.exe đóng gói. Trả về (exePath, args đầy đủ).
func resolveWorkerCommand(analyzeArgs []string) (string, []string) {
	scriptPath := utils.GetWorkerScript()
	scriptExists := false
	if _, err := os.Stat(scriptPath); err == nil {
		scriptExists = true
	}

	if scriptExists {
		check := exec.Command("python", "--version")
		utils.HideCmdWindow(check)
		if err := check.Run(); err == nil {
			return "python", append([]string{scriptPath}, analyzeArgs...)
		}
	}
	if workerExe := utils.GetWorkerExe(); workerExe != "" {
		return workerExe, analyzeArgs
	}
	return "python", append([]string{scriptPath}, analyzeArgs...)
}

// SliceForClip lọc các segment giao với [start,end] và dịch mốc về 0-based theo clip
// (trừ start). Segment vượt biên được kẹp về [0, end-start]. Dùng cho chế độ "nghe cả
// video 1 lần" rồi cắt phụ đề theo từng clip.
func SliceForClip(all []Segment, start, end float64) []Segment {
	var out []Segment
	dur := end - start
	for _, s := range all {
		// Bỏ segment nằm hoàn toàn ngoài clip.
		if s.End <= start || s.Start >= end {
			continue
		}
		ns := Segment{Start: s.Start - start, End: s.End - start, Text: s.Text}
		if ns.Start < 0 {
			ns.Start = 0
		}
		if dur > 0 && ns.End > dur {
			ns.End = dur
		}
		if ns.End > ns.Start {
			out = append(out, ns)
		}
	}
	return out
}

// AdjustForOutput biến đổi mốc segment (đang 0-based theo đầu clip GỐC) về đúng
// dòng thời gian của FILE ĐÃ XUẤT khi clip bị trim đầu và/hoặc đổi tốc độ:
//   - trimStart: clip xuất bắt đầu từ (StartTime+trimStart) nên phải trừ trimStart.
//   - speed: setpts nén thời gian (speed>1 làm clip ngắn lại) nên chia cho speed.
// Công thức: out = (t - trimStart) / speed. Segment nằm trọn trong vùng bị trim đầu
// (End <= trimStart) bị loại; segment vắt qua biên được kẹp về 0. Không có trim/speed
// (trimStart=0, speed=1) thì trả về nguyên trạng.
func AdjustForOutput(segs []Segment, trimStart, speed float64) []Segment {
	if speed <= 0 {
		speed = 1.0
	}
	if trimStart <= 0 && speed == 1.0 {
		return segs
	}
	var out []Segment
	for _, s := range segs {
		st := (s.Start - trimStart) / speed
		en := (s.End - trimStart) / speed
		if en <= 0 {
			continue // nằm trọn trong phần đầu đã trim → không xuất hiện
		}
		if st < 0 {
			st = 0
		}
		if en > st {
			out = append(out, Segment{Start: st, End: en, Text: s.Text})
		}
	}
	return out
}

// ToSRT dựng nội dung file SRT từ segments (đã 0-based). shift trừ thêm khỏi mọi mốc
// (thường 0 vì SliceForClip đã dịch mốc). Bỏ segment có mốc âm sau khi shift.
func ToSRT(segments []Segment, shift float64) string {
	var b strings.Builder
	idx := 1
	for _, s := range segments {
		st := s.Start - shift
		en := s.End - shift
		if en <= 0 {
			continue
		}
		if st < 0 {
			st = 0
		}
		text := strings.TrimSpace(s.Text)
		if text == "" {
			continue
		}
		fmt.Fprintf(&b, "%d\n%s --> %s\n%s\n\n", idx, srtTime(st), srtTime(en), text)
		idx++
	}
	return b.String()
}

// srtTime format giây → "HH:MM:SS,mmm" cho SRT.
func srtTime(sec float64) string {
	if sec < 0 {
		sec = 0
	}
	total := int(sec)
	ms := int((sec - float64(total)) * 1000)
	h := total / 3600
	m := (total % 3600) / 60
	s := total % 60
	return fmt.Sprintf("%02d:%02d:%02d,%03d", h, m, s, ms)
}

// WriteSRTFile ghi nội dung SRT ra file UTF-8 trong dir với tên theo clipID.
// Trả về đường dẫn file đã ghi.
func WriteSRTFile(dir, clipID, content string) (string, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	path := fmt.Sprintf("%s/%s.srt", strings.TrimRight(dir, `/\`), clipID)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return "", err
	}
	return path, nil
}
