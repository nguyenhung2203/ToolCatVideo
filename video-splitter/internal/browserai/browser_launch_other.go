//go:build !windows
// +build !windows

package browserai

import "github.com/go-rod/rod/lib/launcher"

// launchBrowserHidden on non-Windows platforms just delegates to the normal
// launcher.Launch() — hiding the window is only needed on Windows.
func launchBrowserHidden(l *launcher.Launcher) (string, error) {
	return l.Launch()
}
