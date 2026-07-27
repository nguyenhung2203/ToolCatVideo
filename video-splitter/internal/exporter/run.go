package exporter

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
	"video-splitter/internal/utils"
)

// ProgressFunc báo phần trăm hoàn thành (0..1) của MỘT lệnh ffmpeg đang chạy.
// Dùng để thanh tiến độ nhích liên tục trong lúc encode một clip dài, thay vì
// đứng im ở 0% rồi nhảy thẳng lên 100% khi clip xong.
type ProgressFunc func(frac float64)

// Nhịp báo tiến độ: ffmpeg đẩy out_time rất dày (mỗi frame), gửi hết ra frontend
// sẽ ngập event. Chặn theo cả thời gian lẫn mức thay đổi để vừa mượt vừa nhẹ.
const (
	progressMinInterval = 200 * time.Millisecond
	progressMinDelta    = 0.004 // 0.4%
)

// runFFmpeg chạy ffmpeg, trả về log (stderr, kèm stdout khi không đo tiến độ) để
// phía gọi trích nguyên nhân lỗi.
//
// totalDur > 0 và onProgress != nil → thêm "-progress pipe:1" để đọc mốc thời gian
// đã encode (out_time_us) và quy ra phần trăm. totalDur là thời lượng của FILE RA
// (đã chia tốc độ nếu có speed), không phải thời lượng nguồn.
func runFFmpeg(ctx context.Context, args []string, totalDur float64, onProgress ProgressFunc) ([]byte, error) {
	useProgress := totalDur > 0 && onProgress != nil

	full := args
	if useProgress {
		// Cờ toàn cục nên đặt trước mọi tham số khác. -nostats bỏ dòng stats rối
		// (đã có -progress), tránh lẫn vào stderr khi đọc nguyên nhân lỗi.
		full = append([]string{"-nostats", "-progress", "pipe:1"}, args...)
	}

	cmd := exec.CommandContext(ctx, utils.GetBinPath("ffmpeg"), full...)
	utils.HideCmdWindow(cmd)

	var logBuf bytes.Buffer
	cmd.Stderr = &logBuf

	if !useProgress {
		cmd.Stdout = &logBuf
		err := cmd.Run()
		return logBuf.Bytes(), err
	}

	pipe, err := cmd.StdoutPipe()
	if err != nil {
		// Không mở được pipe → chạy kiểu thường, mất tiến độ nhưng vẫn xuất được.
		cmd.Stdout = &logBuf
		if rerr := cmd.Run(); rerr != nil {
			return logBuf.Bytes(), rerr
		}
		onProgress(1)
		return logBuf.Bytes(), nil
	}
	if err := cmd.Start(); err != nil {
		return logBuf.Bytes(), err
	}

	lastFrac := -1.0
	lastEmit := time.Time{}
	scanner := bufio.NewScanner(pipe)
	for scanner.Scan() {
		us, ok := parseProgressMicros(scanner.Text())
		if !ok {
			continue
		}
		frac := us / 1e6 / totalDur
		if frac < 0 {
			frac = 0
		}
		// Không bao giờ báo 100% từ đây: 100% chỉ khi tiến trình kết thúc OK
		// (còn bước ghi moov atom / faststart sau khi encode xong frame cuối).
		if frac > 0.99 {
			frac = 0.99
		}
		if frac-lastFrac < progressMinDelta || time.Since(lastEmit) < progressMinInterval {
			continue
		}
		lastFrac = frac
		lastEmit = time.Now()
		onProgress(frac)
	}
	werr := cmd.Wait()
	if werr == nil {
		onProgress(1)
	}
	return logBuf.Bytes(), werr
}

// parseProgressMicros đọc mốc thời gian (micro-giây) từ một dòng của -progress.
// ffmpeg in cả out_time_us và out_time_ms, nhưng out_time_ms thực chất cũng mang
// giá trị micro-giây (lỗi lịch sử của ffmpeg) nên xử lý như nhau.
func parseProgressMicros(line string) (float64, bool) {
	line = strings.TrimSpace(line)
	var raw string
	switch {
	case strings.HasPrefix(line, "out_time_us="):
		raw = strings.TrimPrefix(line, "out_time_us=")
	case strings.HasPrefix(line, "out_time_ms="):
		raw = strings.TrimPrefix(line, "out_time_ms=")
	default:
		return 0, false
	}
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "N/A" {
		return 0, false
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

// ffmpegFailure dựng thông báo lỗi ĐỌC ĐƯỢC cho một bước ffmpeg thất bại và đẩy
// luôn ra nhật ký hoạt động.
//
// Vì sao cần: trước đây lỗi trả về là cả nghìn dòng log ffmpeg dán vào message
// (frontend cắt còn một mẩu vô nghĩa), hoặc chỉ còn dòng chung chung
// "Conversion failed!" — người dùng không biết vì sao clip lỗi. Ở đây ta lấy các
// dòng lỗi CỤ THỂ (xem lastFFmpegError) và nói rõ bước nào, encoder nào.
func ffmpegFailure(stage, encoder string, err error, out []byte) error {
	reason := lastFFmpegError(out)
	if reason == "" && err != nil {
		reason = err.Error()
	}
	if reason == "" {
		reason = "không rõ nguyên nhân"
	}
	exit := ""
	if err != nil {
		exit = fmt.Sprintf(" [%v]", err)
	}
	emitLog(fmt.Sprintf("❌ ffmpeg lỗi ở bước %s (encoder %s)%s: %s", stage, encoder, exit, reason))
	return fmt.Errorf("bước %s (encoder %s) lỗi: %s", stage, encoder, reason)
}
