package debug

import (
	"path/filepath"
	"testing"
)

func clearStateEnv(t *testing.T) {
	t.Helper()
	t.Setenv("GDBG_STATE", "")
	t.Setenv("XDG_STATE_HOME", "")
}

func TestStateBase_GDBGStateEnvWins(t *testing.T) {
	clearStateEnv(t)
	t.Setenv("GDBG_STATE", "/custom/state")
	t.Setenv("XDG_STATE_HOME", "/should/not/win")
	t.Setenv("HOME", "/should/not/win")

	if got := StateBase(); got != "/custom/state" {
		t.Errorf("StateBase = %q, want /custom/state", got)
	}
}

func TestStateBase_XDGStateHome(t *testing.T) {
	clearStateEnv(t)
	t.Setenv("XDG_STATE_HOME", "/xdg")

	want := filepath.Join("/xdg", "gdbg")
	if got := StateBase(); got != want {
		t.Errorf("StateBase = %q, want %q", got, want)
	}
}

func TestStateBase_DefaultHome(t *testing.T) {
	clearStateEnv(t)
	t.Setenv("HOME", "/fake/home")

	want := filepath.Join("/fake/home", ".local", "state", "gdbg")
	if got := StateBase(); got != want {
		t.Errorf("StateBase = %q, want %q", got, want)
	}
}

func TestDebugSubdirs(t *testing.T) {
	clearStateEnv(t)
	t.Setenv("GDBG_STATE", "/gs")

	if got := DebugRoot(); got != filepath.Join("/gs", "debug") {
		t.Errorf("DebugRoot = %q", got)
	}
	if got := IPCDir(); got != filepath.Join("/gs", "debug", "ipc") {
		t.Errorf("IPCDir = %q", got)
	}
	if got := ScreenshotsDir(); got != filepath.Join("/gs", "debug", "screenshots") {
		t.Errorf("ScreenshotsDir = %q", got)
	}
	if got := RecordingsDir(); got != filepath.Join("/gs", "debug", "recordings") {
		t.Errorf("RecordingsDir = %q", got)
	}
	if got := ScriptsDir(); got != filepath.Join("/gs", "debug", "scripts") {
		t.Errorf("ScriptsDir = %q", got)
	}
	if got := StagingDir(); got != filepath.Join("/gs", "debug", "staging") {
		t.Errorf("StagingDir = %q", got)
	}
}
