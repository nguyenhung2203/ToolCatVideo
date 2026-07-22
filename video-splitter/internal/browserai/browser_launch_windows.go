//go:build windows
// +build windows

package browserai

// launchHidden launches Chrome on Windows with CREATE_NO_WINDOW so no CMD
// flashes appear when running as a packaged .exe (wails build).
// go-rod's default launcher.Launch() sets CREATE_NEW_PROCESS_GROUP but NOT
// CREATE_NO_WINDOW, which causes a brief black console window on every start.

import (
	"bufio"
	"fmt"
	"net"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/go-rod/rod/lib/launcher"
)

const (
	createNoWindow = 0x08000000
	createNewPG    = 0x00000200
)

// launchBrowserHidden prepares a launcher l (already configured with Bin,
// Headless, UserDataDir, flags …) and launches Chrome with a hidden console
// window.  It returns the CDP ws URL the same way launcher.Launch() does.
//
// Internally it:
//  1. Disables leakless (which adds its own helper process we can't control)
//  2. Calls l.FormatArgs() to obtain the final Chrome CLI flags
//  3. Builds an exec.Cmd with CREATE_NO_WINDOW | CREATE_NEW_PROCESS_GROUP
//  4. Waits for Chrome to print its ws:// debug URL on stderr/stdout
func launchBrowserHidden(l *launcher.Launcher) (string, error) {
	// Disable leakless so launcher doesn't wrap chrome in a helper subprocess
	l.Leakless(false)

	// go-rod stores the Chrome binary path under the "rod-bin" flag
	bin := l.Get("rod-bin")
	if bin == "" {
		return "", fmt.Errorf("launchBrowserHidden: Chrome binary path not set in launcher")
	}

	args := l.FormatArgs()

	cmd := exec.Command(bin, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: createNoWindow | createNewPG,
	}

	// Chrome prints its debug port line on stderr.
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return "", fmt.Errorf("stderr pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("start chrome: %w", err)
	}

	// Read lines from stderr/stdout until we see the DevTools listening line.
	urlCh := make(chan string, 1)
	scan := func(r *bufio.Scanner) {
		for r.Scan() {
			line := r.Text()
			if strings.Contains(line, "DevTools listening on") {
				// Line looks like: DevTools listening on ws://127.0.0.1:PORT/...
				idx := strings.Index(line, "ws://")
				if idx >= 0 {
					select {
					case urlCh <- line[idx:]:
					default:
					}
				}
			}
		}
	}
	go scan(bufio.NewScanner(stderr))
	go scan(bufio.NewScanner(stdout))

	select {
	case wsURL := <-urlCh:
		return resolveDebugURL(wsURL)
	case <-time.After(30 * time.Second):
		_ = cmd.Process.Kill()
		return "", fmt.Errorf("timeout waiting for Chrome DevTools URL")
	}
}

// resolveDebugURL converts a raw ws:// URL to the http://host:port form that
// rod.Browser.ControlURL expects (rod uses the /json/version endpoint).
func resolveDebugURL(wsURL string) (string, error) {
	// wsURL example: ws://127.0.0.1:PORT/devtools/browser/UUID
	wsURL = strings.TrimSpace(wsURL)
	wsURL = strings.TrimPrefix(wsURL, "ws://")
	// keep only host:port
	hostPort := wsURL
	if idx := strings.Index(hostPort, "/"); idx >= 0 {
		hostPort = hostPort[:idx]
	}
	host, port, err := net.SplitHostPort(hostPort)
	if err != nil {
		return "", fmt.Errorf("parse host:port from %q: %w", wsURL, err)
	}
	return fmt.Sprintf("http://%s:%s", host, port), nil
}
