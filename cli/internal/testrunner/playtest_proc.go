//go:build !windows

package testrunner

import "os/exec"

// applyPlaytestDisplayAttr is a no-op on non-Windows hosts. Background
// playtests still pass --playtest-background so the game moves the window
// off-screen and marks it unfocusable.
func applyPlaytestDisplayAttr(_ *exec.Cmd, _ RunOptions) {}
