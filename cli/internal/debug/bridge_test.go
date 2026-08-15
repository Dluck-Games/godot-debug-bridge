package debug

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestEncodePayloadAddsProtocolVersionWithoutMutatingCaller(t *testing.T) {
	payload := map[string]any{"cmd": "help", "args": []string{}}
	data, err := encodePayload(payload)
	if err != nil {
		t.Fatal(err)
	}
	if _, mutated := payload["protocol"]; mutated {
		t.Fatal("encodePayload mutated caller payload")
	}
	var encoded map[string]any
	if err := json.Unmarshal(data, &encoded); err != nil {
		t.Fatal(err)
	}
	if encoded["protocol"] != float64(ProtocolVersion) {
		t.Fatalf("protocol = %#v", encoded["protocol"])
	}
}

// respondAfterCommand writes the given result content once the command file
// appears, simulating the game's side of the file IPC.
func respondAfterCommand(t *testing.T, ipcDir, content string) {
	t.Helper()
	go func() {
		cmdPath := filepath.Join(ipcDir, "command")
		resPath := filepath.Join(ipcDir, "result")
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			if _, err := os.Stat(cmdPath); err == nil {
				_ = os.WriteFile(resPath, []byte(content), 0o644)
				return
			}
			time.Sleep(5 * time.Millisecond)
		}
	}()
}

func newTestBridge(t *testing.T) *Bridge {
	t.Helper()
	return NewBridge(t.TempDir())
}

func TestSend_JSONResult(t *testing.T) {
	b := newTestBridge(t)
	b.Log = nil
	respondAfterCommand(t, b.IPCDir, `{"result":"ok"}`)

	got, err := b.Send(map[string]any{"cmd": "pos", "args": []string{}}, 2*time.Second)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	m, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("expected map result, got %T", got)
	}
	if m["result"] != "ok" {
		t.Errorf("result = %v, want ok", m["result"])
	}
}

func TestSend_ArrayJSONResult(t *testing.T) {
	b := newTestBridge(t)
	b.Log = nil
	respondAfterCommand(t, b.IPCDir, `["/tmp/a.png","/tmp/b.png"]`)

	got, err := b.Send(map[string]any{"cmd": "screenshot", "args": map[string]any{}}, 2*time.Second)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	arr, ok := got.([]any)
	if !ok {
		t.Fatalf("expected array result, got %T", got)
	}
	if len(arr) != 2 || arr[0] != "/tmp/a.png" {
		t.Errorf("unexpected array: %v", arr)
	}
}

func TestSend_StringResult(t *testing.T) {
	b := newTestBridge(t)
	b.Log = nil
	respondAfterCommand(t, b.IPCDir, "READY")

	got, err := b.Send(map[string]any{"cmd": "help", "args": []string{}}, 2*time.Second)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if got != "READY" {
		t.Errorf("got %q, want READY", got)
	}
}

func TestSend_InvalidJSONFallsBackToString(t *testing.T) {
	b := newTestBridge(t)
	b.Log = nil
	respondAfterCommand(t, b.IPCDir, `{"result": broken`)

	got, err := b.Send(map[string]any{"cmd": "x"}, 2*time.Second)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if got != `{"result": broken` {
		t.Errorf("got %q, want raw string", got)
	}
}

func TestSend_StringPayloadWrittenVerbatim(t *testing.T) {
	b := newTestBridge(t)
	b.Log = nil
	respondAfterCommand(t, b.IPCDir, "PONG")

	_, err := b.Send("PING", 2*time.Second)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	data, err := os.ReadFile(b.CommandPath())
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "PING" {
		t.Errorf("command file = %q, want PING", data)
	}
}

func TestSend_Timeout(t *testing.T) {
	b := newTestBridge(t)
	b.Log = nil
	b.PollInterval = 10 * time.Millisecond

	start := time.Now()
	_, err := b.Send(map[string]any{"cmd": "pos"}, 250*time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !strings.Contains(err.Error(), "Timeout after") || !strings.Contains(err.Error(), "Is the game running?") {
		t.Errorf("unexpected timeout message: %v", err)
	}
	if elapsed := time.Since(start); elapsed < 200*time.Millisecond {
		t.Errorf("timed out too fast: %v", elapsed)
	}
}

func TestCleanupStale_RemovesCommandAndResultKeepsScript(t *testing.T) {
	b := newTestBridge(t)
	if err := os.MkdirAll(b.IPCDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{b.CommandPath(), b.ResultPath(), b.ScriptPath()} {
		if err := os.WriteFile(p, []byte("stale"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	if err := b.CleanupStale(); err != nil {
		t.Fatalf("CleanupStale: %v", err)
	}

	for _, p := range []string{b.CommandPath(), b.ResultPath()} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("%s should have been cleaned", p)
		}
	}
	if _, err := os.Stat(b.ScriptPath()); err != nil {
		t.Errorf("temp_script.gd should be preserved during stale cleanup: %v", err)
	}
}

func TestSend_ConsumesResultFile(t *testing.T) {
	b := newTestBridge(t)
	b.Log = nil
	respondAfterCommand(t, b.IPCDir, "DONE")

	if _, err := b.Send(map[string]any{"cmd": "x"}, 2*time.Second); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(b.ResultPath()); !os.IsNotExist(err) {
		t.Error("result file should be consumed after read")
	}
}
