//go:build windows
// +build windows

package utils

import (
	"path/filepath"

	"golang.org/x/sys/windows"
)

// FreeDiskSpace trả về số byte trống người dùng hiện tại được phép ghi vào ổ
// chứa path. Dùng GetDiskFreeSpaceEx với lpFreeBytesAvailableToCaller (KHÔNG
// phải tổng dung lượng trống của ổ) để tôn trọng disk quota nếu có.
//
// path không cần tồn tại — hàm sẽ leo dần lên thư mục cha cho tới khi tìm được
// đường dẫn Windows chấp nhận (hữu ích khi thư mục xuất chưa được tạo).
func FreeDiskSpace(path string) (uint64, error) {
	dir := path
	var lastErr error
	for i := 0; i < 16; i++ {
		if dir == "" {
			break
		}
		p, err := windows.UTF16PtrFromString(dir)
		if err != nil {
			return 0, err
		}
		var freeToCaller, total, totalFree uint64
		err = windows.GetDiskFreeSpaceEx(p, &freeToCaller, &total, &totalFree)
		if err == nil {
			return freeToCaller, nil
		}
		lastErr = err
		parent := filepath.Dir(dir)
		if parent == dir {
			break // đã tới gốc ổ đĩa, không leo được nữa
		}
		dir = parent
	}
	return 0, lastErr
}
