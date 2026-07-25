//go:build !windows
// +build !windows

package utils

// FreeDiskSpace: trên nền tảng không phải Windows trả về 0 kèm err=nil, nghĩa là
// "không xác định được". Phía gọi phải coi 0 là BỎ QUA kiểm tra, không phải "đầy ổ".
func FreeDiskSpace(path string) (uint64, error) {
	return 0, nil
}
