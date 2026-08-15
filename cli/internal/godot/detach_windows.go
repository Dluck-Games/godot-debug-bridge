//go:build windows

package godot

import "syscall"

func detachAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{CreationFlags: 0x00000008} // CREATE_NO_WINDOW
}
