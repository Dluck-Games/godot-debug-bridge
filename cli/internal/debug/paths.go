// godot-debug-bridge/cli/internal/debug/paths.go
package debug

import (
	"os"
	"path/filepath"
)

// StateBase returns the absolute GDBG_STATE root shared by the CLI and the game.
//
// Resolution order (formal injected variable is GDBG_STATE):
//  1. $GDBG_STATE (authoritative — `gdbg run` injects this into the game process)
//  2. $XDG_STATE_HOME/gdbg
//  3. ~/.local/state/gdbg
//
// The CLI and the game must resolve the same root so the file IPC under
// $GDBG_STATE/debug/ipc stays in one place. Godot user:// is never used.
func StateBase() string {
	base := ""
	switch {
	case os.Getenv("GDBG_STATE") != "":
		base = os.Getenv("GDBG_STATE")
	case os.Getenv("XDG_STATE_HOME") != "":
		base = filepath.Join(os.Getenv("XDG_STATE_HOME"), "gdbg")
	default:
		home := userHomeDir()
		if home == "" {
			base = filepath.Join(".local", "state", "gdbg")
		} else {
			base = filepath.Join(home, ".local", "state", "gdbg")
		}
	}
	abs, err := filepath.Abs(base)
	if err != nil {
		return filepath.Clean(base)
	}
	return abs
}

// DebugRoot is the debug workspace under $GDBG_STATE.
func DebugRoot() string { return filepath.Join(StateBase(), "debug") }

// IPCDir holds the file IPC triple: command / result / temp_script.gd.
func IPCDir() string { return filepath.Join(DebugRoot(), "ipc") }

// ScreenshotsDir is the authoritative destination for screenshot PNGs.
func ScreenshotsDir() string { return filepath.Join(DebugRoot(), "screenshots") }

// RecordingsDir is the default destination for record output.
func RecordingsDir() string { return filepath.Join(DebugRoot(), "recordings") }

// ScriptsDir is where agent-dropped temporary .gd scripts live.
func ScriptsDir() string { return filepath.Join(DebugRoot(), "scripts") }

// StagingDir holds frames before encoding; leftover files are cleaned after record.
func StagingDir() string { return filepath.Join(DebugRoot(), "staging") }

func userHomeDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return home
}
