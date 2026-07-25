package exporter

import (
	"fmt"
	"strings"
	"sync"
)

// ==========================================================================
// THEO DÕI GPU → CPU FALLBACK
//
// Trước đây mọi chỗ encode đều thử GPU trước, nếu ffmpeg trả lỗi thì nuốt
// hoàn toàn (`if err == nil { return nil }`) rồi âm thầm chạy lại bằng CPU.
// Hai vấn đề khi dùng lâu dài:
//
//  1. Không ai biết GPU đã ngừng hoạt động. Người dùng chỉ thấy "xuất chậm
//     bất thường" mà không có manh mối gì trong log.
//  2. Mỗi clip phải trả giá một lần khởi chạy ffmpeg thất bại. Với hàng trăm
//     clip (và driver NVENC giới hạn số session đồng thời) đây là lãng phí
//     lớn: ffmpeg phải mở file, dựng filtergraph, mới lỗi ở bước mở encoder.
//
// Giải pháp: nhớ trạng thái theo từng encoder trong một phiên chạy app.
//   - Lỗi lần đầu → log chi tiết (dòng lỗi thật của ffmpeg) rồi fallback CPU.
//   - Lỗi liên tiếp tới ngưỡng → tắt encoder đó cho các lần sau, đi thẳng CPU.
//   - Thành công → reset bộ đếm (lỗi lẻ do tranh chấp session không nên
//     làm tắt GPU vĩnh viễn).
// ==========================================================================

// gpuFailureThreshold: số lần lỗi LIÊN TIẾP trước khi bỏ hẳn encoder GPU.
// Để >1 vì lỗi đầu tiên thường là tranh chấp session (nhiều clip song song),
// không phải GPU thực sự không dùng được.
const gpuFailureThreshold = 3

type gpuEncoderState struct {
	consecutiveFailures int
	disabled            bool
	everSucceeded       bool
}

var (
	gpuStateMu sync.Mutex
	gpuStates  = map[string]*gpuEncoderState{}

	logMu   sync.Mutex
	logFunc func(string)
)

// SetLogFunc gắn hàm ghi log của tầng app (thường là runtime.EventsEmit lên
// thanh trạng thái) để các thông báo fallback hiện ra cho người dùng thấy.
// Truyền nil để tắt.
func SetLogFunc(fn func(string)) {
	logMu.Lock()
	logFunc = fn
	logMu.Unlock()
}

func emitLog(msg string) {
	logMu.Lock()
	fn := logFunc
	logMu.Unlock()
	if fn != nil {
		fn(msg)
	}
}

// gpuEncoderDisabled cho biết có nên BỎ QUA hẳn lần thử GPU hay không.
func gpuEncoderDisabled(encoder string) bool {
	if encoder == "" || encoder == "libx264" {
		return false
	}
	gpuStateMu.Lock()
	defer gpuStateMu.Unlock()
	st := gpuStates[encoder]
	return st != nil && st.disabled
}

// noteGPUSuccess reset bộ đếm lỗi sau một lần encode GPU thành công.
func noteGPUSuccess(encoder string) {
	if encoder == "" || encoder == "libx264" {
		return
	}
	gpuStateMu.Lock()
	defer gpuStateMu.Unlock()
	st := gpuStates[encoder]
	if st == nil {
		st = &gpuEncoderState{}
		gpuStates[encoder] = st
	}
	st.consecutiveFailures = 0
	st.everSucceeded = true
}

// noteGPUFailure ghi log lỗi GPU và tăng bộ đếm; khi vượt ngưỡng thì tắt
// encoder cho phần còn lại của phiên. stage là tên bước ("cắt clip",
// "ghép intro"...) để log đọc được. combinedOutput là output của ffmpeg.
func noteGPUFailure(encoder, stage string, err error, combinedOutput []byte) {
	if encoder == "" || encoder == "libx264" {
		return
	}
	gpuStateMu.Lock()
	st := gpuStates[encoder]
	if st == nil {
		st = &gpuEncoderState{}
		gpuStates[encoder] = st
	}
	st.consecutiveFailures++
	failures := st.consecutiveFailures
	justDisabled := false
	if !st.disabled && failures >= gpuFailureThreshold {
		st.disabled = true
		justDisabled = true
	}
	alreadyDisabled := st.disabled && !justDisabled
	gpuStateMu.Unlock()

	if alreadyDisabled {
		return
	}

	reason := lastFFmpegError(combinedOutput)
	if reason == "" && err != nil {
		reason = err.Error()
	}
	if reason == "" {
		reason = "không rõ nguyên nhân"
	}

	if justDisabled {
		emitLog(fmt.Sprintf("⚠ GPU (%s) lỗi %d lần liên tiếp ở bước %s → CHUYỂN SANG CPU cho toàn bộ phần còn lại. Lý do: %s",
			encoder, failures, stage, reason))
		return
	}
	emitLog(fmt.Sprintf("⚠ GPU (%s) lỗi ở bước %s, đang encode lại bằng CPU (chậm hơn). Lý do: %s",
		encoder, stage, reason))
}

// lastFFmpegError lọc ra dòng có ý nghĩa nhất trong output của ffmpeg. ffmpeg
// in rất nhiều banner (version/configuration/Stream #...), dòng lỗi thật
// thường nằm ở cuối và chứa các từ khóa dưới đây.
func lastFFmpegError(out []byte) string {
	if len(out) == 0 {
		return ""
	}
	lines := strings.Split(strings.ReplaceAll(string(out), "\r\n", "\n"), "\n")
	keywords := []string{
		"error", "Error", "ERROR",
		"Cannot", "cannot", "failed", "Failed",
		"Invalid", "invalid", "No capable devices",
		"OpenEncodeSessionEx", "not supported", "Unknown encoder",
		"Impossible", "Conversion failed",
	}
	// Quét từ dưới lên: dòng lỗi cuối cùng mới là nguyên nhân dừng.
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		for _, kw := range keywords {
			if strings.Contains(line, kw) {
				return truncateLine(line)
			}
		}
	}
	// Không khớp từ khóa nào → lấy dòng cuối không rỗng.
	for i := len(lines) - 1; i >= 0; i-- {
		if line := strings.TrimSpace(lines[i]); line != "" {
			return truncateLine(line)
		}
	}
	return ""
}

func truncateLine(s string) string {
	const maxLen = 240
	r := []rune(s)
	if len(r) <= maxLen {
		return s
	}
	return string(r[:maxLen]) + "…"
}
