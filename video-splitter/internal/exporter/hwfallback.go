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

	reason := lastFFmpegError(combinedOutput)
	if reason == "" && err != nil {
		reason = err.Error()
	}
	if reason == "" {
		reason = "không rõ nguyên nhân"
	}

	// GPU đã bị tắt từ trước mà vẫn lỗi ở đây → vẫn phải ghi log. Bản cũ return
	// im lặng nên từ clip thứ 4 trở đi nhật ký trống trơn dù vẫn đang lỗi.
	if alreadyDisabled {
		emitLog(fmt.Sprintf("⚠ GPU (%s) vẫn lỗi ở bước %s (đã tắt GPU, đang dùng CPU). Lý do: %s",
			encoder, stage, reason))
		return
	}

	if justDisabled {
		emitLog(fmt.Sprintf("⚠ GPU (%s) lỗi %d lần liên tiếp ở bước %s → CHUYỂN SANG CPU cho toàn bộ phần còn lại. Lý do: %s",
			encoder, failures, stage, reason))
		return
	}
	emitLog(fmt.Sprintf("⚠ GPU (%s) lỗi ở bước %s, đang encode lại bằng CPU (chậm hơn). Lý do: %s",
		encoder, stage, reason))
}

// genericFFmpegLines là các dòng ffmpeg in ra khi BỎ CUỘC, không cho biết vì
// sao. Bản cũ quét từ dưới lên và khớp ngay "Conversion failed!" nên log luôn
// hiện đúng câu vô nghĩa đó, che mất nguyên nhân thật in ở phía trên (ví dụ
// "No capable devices found", "Unknown encoder", "Invalid argument").
var genericFFmpegLines = []string{
	"Conversion failed",
	"Error while filtering",
	"Error opening output file",
	"Error opening output files",
	"Error opening input file",
	"Error opening input files",
	"Task finished with error code",
	"Terminating thread with return code",
}

// specificFFmpegKeywords: dấu hiệu của dòng nói ĐÚNG nguyên nhân.
var specificFFmpegKeywords = []string{
	"No capable devices", "OpenEncodeSessionEx", "InitializeEncoder",
	"Unknown encoder", "Encoder not found", "not supported", "Unsupported",
	"No space left", "Permission denied", "No such file",
	"Invalid argument", "Invalid data found", "Invalid", "invalid",
	"Impossible to convert", "Cannot", "cannot", "Could not", "could not",
	"Unable to", "moov atom not found", "Device creation failed",
	"driver version", "out of memory", "Out of memory",
	"failed", "Failed", "error", "Error", "ERROR",
}

// isGenericFFmpegLine cho biết dòng này chỉ là câu "đã lỗi" chung chung.
func isGenericFFmpegLine(line string) bool {
	for _, g := range genericFFmpegLines {
		if strings.Contains(line, g) {
			return true
		}
	}
	return false
}

// lastFFmpegError lọc ra (các) dòng NÓI ĐÚNG nguyên nhân trong output ffmpeg.
//
// Chiến lược: quét từ dưới lên, BỎ QUA các dòng chung chung ("Conversion
// failed!"...) để tìm dòng lỗi cụ thể. Gom tối đa maxReasonLines dòng cụ thể
// (theo đúng thứ tự gốc) vì nguyên nhân thật hay đi thành cặp — ví dụ
// "[h264_nvenc] OpenEncodeSessionEx failed: out of memory" đứng ngay trước
// "[h264_nvenc] No capable devices found". Chỉ khi không có dòng nào cụ thể
// mới đành dùng dòng chung chung để log không bị rỗng.
func lastFFmpegError(out []byte) string {
	if len(out) == 0 {
		return ""
	}
	const maxReasonLines = 3
	lines := strings.Split(strings.ReplaceAll(string(out), "\r\n", "\n"), "\n")

	var specific []string
	var generic string
	var lastLine string
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		if lastLine == "" {
			lastLine = line
		}
		if isGenericFFmpegLine(line) {
			if generic == "" {
				generic = line
			}
			continue
		}
		for _, kw := range specificFFmpegKeywords {
			if strings.Contains(line, kw) {
				// Chèn đầu để giữ thứ tự xuất hiện gốc (đang quét ngược).
				specific = append([]string{line}, specific...)
				break
			}
		}
		if len(specific) >= maxReasonLines {
			break
		}
	}

	if len(specific) > 0 {
		return truncateLine(strings.Join(specific, " | "))
	}
	if generic != "" {
		return truncateLine(generic)
	}
	return truncateLine(lastLine)
}

func truncateLine(s string) string {
	const maxLen = 240
	r := []rune(s)
	if len(r) <= maxLen {
		return s
	}
	return string(r[:maxLen]) + "…"
}
