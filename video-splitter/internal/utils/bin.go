package utils

import (
	"os"
	"path/filepath"
	"strings"
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
		
		// Trong chế độ phát triển (Wails dev), exe thường chứa "-dev".
		// Ta luôn ưu tiên chạy trực tiếp script trong workspace gốc (Cwd) để dev/test tức thì
		if strings.Contains(strings.ToLower(filepath.Base(ex)), "-dev") {
			if _, err := os.Stat(rel); err == nil {
				return rel
			}
		}

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

// fontFamilyFiles map tên font family (khớp dropdown UI) sang tên file .ttf trong
// thư mục Fonts của Windows. drawtext cần đường dẫn file thật (không nhận tên family
// như libass), nên phải quy đổi. Danh sách này khớp các font mặc định luôn có sẵn
// trên Windows 10/11 để tránh chọn font rồi không render được.
var fontFamilyFiles = map[string]string{
	"Arial":           "arial.ttf",
	"Times New Roman": "times.ttf",
	"Tahoma":          "tahoma.ttf",
	"Verdana":         "verdana.ttf",
	"Georgia":         "georgia.ttf",
	"Calibri":         "calibri.ttf",
	"Comic Sans MS":   "comic.ttf",
	"Impact":          "impact.ttf",
	"Trebuchet MS":    "trebuc.ttf",
	"Courier New":     "cour.ttf",
	"Segoe UI":        "segoeui.ttf",
}

// GetFontPathForFamily trả về đường dẫn file .ttf cho một font family (dùng cho
// drawtext của TextOp). Nếu family rỗng hoặc không nằm trong danh sách hỗ trợ, hoặc
// file font không tồn tại trên máy, fallback về GetFontPath() (font mặc định).
func GetFontPathForFamily(family string) string {
	family = strings.TrimSpace(family)
	if family != "" {
		if file, ok := fontFamilyFiles[family]; ok {
			if win := os.Getenv("WINDIR"); win != "" {
				p := filepath.Join(win, "Fonts", file)
				if _, err := os.Stat(p); err == nil {
					return p
				}
			}
		}
	}
	return GetFontPath()
}

// SupportedFontFamilies trả về danh sách tên font family hỗ trợ (cho UI dropdown +
// kiểm tra hợp lệ). Chỉ liệt kê font thực sự có file trên máy để UI không hiện font
// dùng không được.
func SupportedFontFamilies() []string {
	var out []string
	win := os.Getenv("WINDIR")
	for _, fam := range []string{"Arial", "Times New Roman", "Tahoma", "Verdana", "Georgia", "Calibri", "Comic Sans MS", "Impact", "Trebuchet MS", "Courier New", "Segoe UI"} {
		if win == "" {
			out = append(out, fam)
			continue
		}
		if _, err := os.Stat(filepath.Join(win, "Fonts", fontFamilyFiles[fam])); err == nil {
			out = append(out, fam)
		}
	}
	return out
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
