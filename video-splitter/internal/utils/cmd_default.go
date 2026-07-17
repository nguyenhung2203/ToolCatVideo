//go:build !windows
// +build !windows

package utils

import "os/exec"

// HideCmdWindow is a no-op on non-Windows platforms
func HideCmdWindow(cmd *exec.Cmd) {
	// No-op
}
