package record

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Dluck-Games/godot-debug-bridge/cli/internal/godot"
)

const windowLockPollInterval = 200 * time.Millisecond

// DefaultWindowLockPath returns the per-host GUI window mutex directory path.
func DefaultWindowLockPath() string {
	return filepath.Join(os.TempDir(), "gdbg-window.lock")
}

// WindowLock guards exclusive GUI window usage across concurrent gdbg record
// invocations on the same machine.
type WindowLock struct {
	dir string
}

// AcquireWindowLock acquires an atomic directory lock at lockDir, cleaning up
// stale locks left behind by dead PIDs.
func AcquireWindowLock(ctx context.Context, lockDir string) (*WindowLock, error) {
	if lockDir == "" {
		return &WindowLock{}, nil
	}
	if err := os.MkdirAll(filepath.Dir(lockDir), 0o755); err != nil {
		return nil, fmt.Errorf("create lock parent dir: %w", err)
	}
	pidPath := filepath.Join(lockDir, "pid")
	myPID := strconv.Itoa(os.Getpid())

	for {
		err := os.Mkdir(lockDir, 0o755)
		if err == nil {
			_ = os.WriteFile(pidPath, []byte(myPID), 0o644)
			return &WindowLock{dir: lockDir}, nil
		}
		if !os.IsExist(err) {
			return nil, fmt.Errorf("acquire window lock %s: %w", lockDir, err)
		}

		// Clean stale lock if owner PID is no longer alive.
		if data, readErr := os.ReadFile(pidPath); readErr == nil {
			if ownerPID, convErr := strconv.Atoi(strings.TrimSpace(string(data))); convErr == nil {
				if ownerPID > 0 && !godot.IsAlive(ownerPID) {
					_ = os.RemoveAll(lockDir)
					continue
				}
			}
		} else if os.IsNotExist(readErr) {
			// Give the lock creator a brief window to write its pid file before treating as stale.
			if info, statErr := os.Stat(lockDir); statErr == nil && time.Since(info.ModTime()) > 2*time.Second {
				_ = os.RemoveAll(lockDir)
				continue
			}
		}

		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("timed out waiting for window lock %s: %w", lockDir, ctx.Err())
		case <-time.After(windowLockPollInterval):
		}
	}
}

// Unlock releases the window lock directory.
func (l *WindowLock) Unlock() {
	if l == nil || l.dir == "" {
		return
	}
	_ = os.RemoveAll(l.dir)
	l.dir = ""
}
