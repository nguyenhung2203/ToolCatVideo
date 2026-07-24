//go:build !windows
// +build !windows

package browserai

import (
	"os/exec"

	"github.com/go-rod/rod/lib/launcher"
)

// launchBrowserHidden on non-Windows platforms just delegates to the normal
// launcher.Launch() — hiding the window is only needed on Windows. Trả về cmd=nil
// vì launcher tự quản lý tiến trình; session sẽ fallback về launcher.Kill().
func launchBrowserHidden(l *launcher.Launcher) (string, *exec.Cmd, error) {
	url, err := l.Launch()
	return url, nil, err
}

// killOrphanChromeForProfile là no-op ngoài Windows: launcher.Launch() tự quản lý
// vòng đời tiến trình nên không có Chrome mồ côi kiểu handoff cần dọn thủ công.
func killOrphanChromeForProfile(profileDir string) {}
