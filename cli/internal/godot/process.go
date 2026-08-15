package godot

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	GamePIDFile   = "gdbg-game.pid"
	EditorPIDFile = "gdbg-editor.pid"
	KillTimeout   = 5 * time.Second
)

func PIDFilePath(projectDir, name string) string {
	return filepath.Join(projectDir, ".godot", name)
}

func WritePID(projectDir, name string, pid int) error {
	path := PIDFilePath(projectDir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(strconv.Itoa(pid)), 0o644)
}

func ReadPID(projectDir, name string) (int, error) {
	data, err := os.ReadFile(PIDFilePath(projectDir, name))
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(string(data)))
}

func RemovePID(projectDir, name string) {
	os.Remove(PIDFilePath(projectDir, name))
}

func IsAlive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	if runtime.GOOS == "windows" {
		out, err := exec.Command("tasklist", "/FI", fmt.Sprintf("PID eq %d", pid), "/NH").Output()
		if err != nil {
			return false
		}
		return strings.Contains(string(out), strconv.Itoa(pid))
	}
	return proc.Signal(syscall.Signal(0)) == nil
}

func KillProcess(pid int) error {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return err
	}

	if runtime.GOOS == "windows" {
		exec.Command("taskkill", "/PID", strconv.Itoa(pid)).Run()
		time.Sleep(KillTimeout)
		if IsAlive(pid) {
			return exec.Command("taskkill", "/F", "/PID", strconv.Itoa(pid)).Run()
		}
		return nil
	}

	// Unix: SIGTERM → wait → SIGKILL
	if err := proc.Signal(syscall.SIGTERM); err != nil {
		return err
	}

	deadline := time.After(KillTimeout)
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-deadline:
			return proc.Signal(syscall.SIGKILL)
		case <-ticker.C:
			if !IsAlive(pid) {
				return nil
			}
		}
	}
}

type DisplayMode int

const (
	DisplayHeadless DisplayMode = iota
	DisplayWindowed
	DisplayFullscreen
)

type LaunchOpts struct {
	ProjectDir  string
	DisplayMode DisplayMode
	Editor      bool
	Detach      bool
	ExtraArgs   []string
	LogFile     *os.File
	// Env holds KEY=VALUE pairs appended to the child's environment
	// (for example GDBG_STATE injected by `gdbg run game`).
	Env []string
}

func Launch(godotBin string, opts LaunchOpts) (*exec.Cmd, error) {
	args := []string{}
	switch opts.DisplayMode {
	case DisplayWindowed:
		args = append(args, "--windowed")
	case DisplayFullscreen:
		args = append(args, "--fullscreen")
	default:
		args = append(args, "--headless")
	}
	if opts.Editor {
		args = append(args, "--editor")
	}
	args = append(args, "--path", opts.ProjectDir)
	if len(opts.ExtraArgs) > 0 {
		args = append(args, "--")
		args = append(args, opts.ExtraArgs...)
	}

	cmd := exec.Command(godotBin, args...)
	cmd.Env = append(os.Environ(), opts.Env...)

	if opts.Detach {
		cmd.SysProcAttr = detachAttr()
		if opts.LogFile != nil {
			cmd.Stdout = opts.LogFile
			cmd.Stderr = opts.LogFile
		} else {
			cmd.Stdout = nil
			cmd.Stderr = nil
		}
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to launch Godot: %w", err)
	}
	return cmd, nil
}

func StopByPIDFile(projectDir, pidFileName string) (int, error) {
	pid, err := ReadPID(projectDir, pidFileName)
	if err != nil {
		return 0, err
	}
	if !IsAlive(pid) {
		RemovePID(projectDir, pidFileName)
		return 0, fmt.Errorf("stale PID file (process %d not running), cleaned up", pid)
	}
	if err := KillProcess(pid); err != nil {
		return pid, err
	}
	RemovePID(projectDir, pidFileName)
	return pid, nil
}
