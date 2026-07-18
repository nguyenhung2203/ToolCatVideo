//go:build windows
// +build windows

package utils

import (
	"os/exec"
	"syscall"
)

// HideCmdWindow sets the SysProcAttr on Windows to hide the command prompt window and set priority
func HideCmdWindow(cmd *exec.Cmd) {
	if cmd == nil {
		return
	}
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.HideWindow = true
	// Set CREATE_NO_WINDOW (0x08000000) | BELOW_NORMAL_PRIORITY_CLASS (0x00004000)
	// This prevents CPU starvation of Wails WebView2 thread during video encoding/analysis
	cmd.SysProcAttr.CreationFlags = 0x08000000 | 0x00004000
}
