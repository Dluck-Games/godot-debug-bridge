// godot-debug-bridge/cli/cmd/run_test.go
package cmd

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// respondLikeGame simulates the game-side AIDebugBridge: once a command file
// appears under ipcDir it writes a result file, then returns.
func respondLikeGame(t *testing.T, ipcDir string) {
	t.Helper()
	go func() {
		cmdPath := filepath.Join(ipcDir, "command")
		resPath := filepath.Join(ipcDir, "result")
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			if _, err := os.Stat(cmdPath); err == nil {
				_ = os.WriteFile(resPath, []byte(`{"result":"ok"}`), 0o644)
				return
			}
			time.Sleep(5 * time.Millisecond)
		}
	}()
}

// respondStably simulates a fully responsive game-side AIDebugBridge: it keeps
// answering every command file that appears, so each probe round trip succeeds.
func respondStably(t *testing.T, ipcDir string) {
	t.Helper()
	go func() {
		cmdPath := filepath.Join(ipcDir, "command")
		resPath := filepath.Join(ipcDir, "result")
		deadline := time.Now().Add(30 * time.Second)
		for time.Now().Before(deadline) {
			if _, err := os.Stat(cmdPath); err == nil {
				_ = os.WriteFile(resPath, []byte(`{"result":"ok"}`), 0o644)
			}
			time.Sleep(5 * time.Millisecond)
		}
	}()
}

func TestPollReadiness_StaleDirWithoutBridge(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if pollReadiness(dir, 500*time.Millisecond) {
		t.Fatal("readiness must not succeed on a pre-existing IPC directory with no live bridge")
	}
}

func TestPollReadiness_SingleResponseDoesNotReportReady(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// The bridge answers exactly one probe, then goes silent — the transient
	// response that can land just before synchronous project startup blocks the
	// game main thread. One success must not report ready.
	respondLikeGame(t, dir)
	if pollReadiness(dir, 3*time.Second) {
		t.Fatal("readiness must not report ready after a single transient response")
	}
}

func TestPollReadiness_TwoConsecutiveRoundTripsReportReady(t *testing.T) {
	dir := t.TempDir()
	// Pre-create the directory too, proving readiness comes from the command/
	// result round trips and not from directory existence alone.
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	respondStably(t, dir)
	if !pollReadiness(dir, 5*time.Second) {
		t.Fatal("readiness should succeed once two consecutive probes succeed")
	}
}

func TestReadinessTimeoutIs90s(t *testing.T) {
	if gameReadyTimeout != 90*time.Second {
		t.Fatalf("gameReadyTimeout = %s, want 90s", gameReadyTimeout)
	}
}

func TestReadinessStabilizeIntervalInRange(t *testing.T) {
	if readinessStabilizeInterval < 500*time.Millisecond || readinessStabilizeInterval > 1*time.Second {
		t.Fatalf("readinessStabilizeInterval = %s, want between 500ms and 1s", readinessStabilizeInterval)
	}
}

func TestLogStateBaseUsesGDBGState(t *testing.T) {
	stateRoot := t.TempDir()
	t.Setenv("GDBG_STATE", stateRoot)
	t.Setenv("XDG_STATE_HOME", "/must-not-win")
	if got, want := logStateBase(), filepath.Join(stateRoot, "logs"); got != want {
		t.Fatalf("logStateBase() = %q, want %q", got, want)
	}
}
