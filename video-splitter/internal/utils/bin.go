package utils

import (
	"os"
	"path/filepath"
)

// GetBinPath trả về đường dẫn tới file thực thi. 
// Nếu file tồn tại trong thư mục ./bin/ (chạy local portable), nó sẽ trả về đường dẫn đó.
// Nếu không, nó sẽ trả về tên gốc để sử dụng từ %PATH%.
func GetBinPath(name string) string {
	// Kiểm tra xem ứng dụng đang chạy ở đâu
	ex, err := os.Executable()
	if err != nil {
		return name
	}
	appDir := filepath.Dir(ex)
	
	// Thử tìm directly cùng cấp với file executable (khi đã build portable)
	directPath := filepath.Join(appDir, name+".exe")
	if _, err := os.Stat(directPath); err == nil {
		return directPath
	}

	// Thử tìm trong thư mục bin cùng cấp với file executable (khi đã build)
	localPath := filepath.Join(appDir, "bin", name+".exe")
	if _, err := os.Stat(localPath); err == nil {
		return localPath
	}
	
	// Thử tìm trong thư mục bin từ root project (khi đang chạy wails dev)
	devPath := filepath.Join("bin", name+".exe")
	if _, err := os.Stat(devPath); err == nil {
		return devPath
	}

	return name // Fallback về %PATH%
}

// GetWorkerScript trả về đường dẫn tới script Python worker.
// Ưu tiên thư mục cạnh file executable (khi build), sau đó thư mục dev.
func GetWorkerScript() string {
	const rel = "python_worker/main.py"

	ex, err := os.Executable()
	if err == nil {
		appDir := filepath.Dir(ex)
		localPath := filepath.Join(appDir, "python_worker", "main.py")
		if _, err := os.Stat(localPath); err == nil {
			return localPath
		}
	}

	return rel // Fallback: chạy wails dev từ thư mục gốc project
}

// GetFontPath trả về đường dẫn tới một file font TTF dùng cho filter drawtext.
// drawtext cần font TTF/OTF thật (không dùng được woff2), nên thứ tự ưu tiên:
//  1. font đóng gói cạnh executable (bin/font.ttf) — đảm bảo portable khi build.
//  2. font trong bin/ của thư mục dev.
//  3. Arial của Windows (fallback khi chạy dev chưa bundle font).
// Trả về "" nếu không tìm thấy — caller nên bỏ qua drawtext để tránh lỗi ffmpeg.
func GetFontPath() string {
	var candidates []string
	if ex, err := os.Executable(); err == nil {
		appDir := filepath.Dir(ex)
		candidates = append(candidates,
			filepath.Join(appDir, "bin", "font.ttf"),
			filepath.Join(appDir, "font.ttf"),
		)
	}
	candidates = append(candidates,
		filepath.Join("bin", "font.ttf"),
	)
	if win := os.Getenv("WINDIR"); win != "" {
		candidates = append(candidates,
			filepath.Join(win, "Fonts", "arial.ttf"),
			filepath.Join(win, "Fonts", "segoeui.ttf"),
		)
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// GetWorkerExe trả về đường dẫn tới worker đã đóng gói (PyInstaller) nếu có.
// Trả về "" nếu không tìm thấy — khi đó caller nên fallback sang chạy script bằng python.
func GetWorkerExe() string {
	ex, err := os.Executable()
	if err != nil {
		return ""
	}
	appDir := filepath.Dir(ex)

	candidates := []string{
		filepath.Join(appDir, "bin", "worker.exe"),
		filepath.Join(appDir, "worker.exe"),
		filepath.Join("bin", "worker.exe"),
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// GetYtDlpPath trả về đường dẫn tới yt-dlp.exe dùng để tải video online.
// Ưu tiên portable (cạnh executable / bin/), sau đó fallback sang %PATH%.
func GetYtDlpPath() string {
	return GetBinPath("yt-dlp")
}
