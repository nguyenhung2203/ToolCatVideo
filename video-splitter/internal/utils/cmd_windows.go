//go:build windows
// +build windows

package utils

import (
	"os/exec"
	"syscall"
)

// HideCmdWindow sets the SysProcAttr on Windows to hide the command prompt window
func HideCmdWindow(cmd *exec.Cmd) {
	if cmd == nil {
		return
	}
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.HideWindow = true
}
