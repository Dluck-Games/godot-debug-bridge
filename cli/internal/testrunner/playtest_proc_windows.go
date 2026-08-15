//go:build windows

package testrunner

import (
	"os/exec"
	"syscall"
)

// applyPlaytestDisplayAttr hides the Godot process window on Windows for the
// default background playtest so it cannot steal foreground focus. Headless
// has no window; --windowed is an explicit foreground watch mode.
func applyPlaytestDisplayAttr(cmd *exec.Cmd, opts RunOptions) {
	if opts.Headless || opts.Windowed {
		return
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
}
