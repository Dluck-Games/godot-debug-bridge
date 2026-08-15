// godot-debug-bridge/cli/internal/debug/bridge.go
package debug

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	// ProtocolVersion is the current CLI/runtime JSON envelope version.
	ProtocolVersion = 1
	// DefaultTimeout bounds a regular bridge command round trip.
	DefaultTimeout = 10 * time.Second
	// ScriptTimeout is longer because GDScript compilation can take a while.
	ScriptTimeout = 30 * time.Second
	// PollInterval is how often the result file is checked.
	PollInterval = 100 * time.Millisecond
)

// Bridge is the file IPC client. The protocol shape is unchanged from the old
// ai-debug bridge: the CLI writes a JSON (or raw string) payload to `command`,
// the game writes its reply to `result`, and the CLI polls for it. The only
// change is the root directory, which now lives under $GDBG_STATE/debug/ipc
// instead of Godot userdata.
type Bridge struct {
	IPCDir       string
	PollInterval time.Duration
	Log          io.Writer // optional sink for the "Sending: ..." debug line
}

// NewBridge returns a Bridge rooted at ipcDir with sane defaults.
func NewBridge(ipcDir string) *Bridge {
	return &Bridge{
		IPCDir:       ipcDir,
		PollInterval: PollInterval,
		Log:          os.Stderr,
	}
}

// CommandPath is the file the CLI writes the payload to.
func (b *Bridge) CommandPath() string { return filepath.Join(b.IPCDir, "command") }

// ResultPath is the file the game writes its reply to.
func (b *Bridge) ResultPath() string { return filepath.Join(b.IPCDir, "result") }

// ScriptPath is the temp_script.gd the script command stages before sending.
func (b *Bridge) ScriptPath() string { return filepath.Join(b.IPCDir, "temp_script.gd") }

func (b *Bridge) ensureDir() error {
	if err := os.MkdirAll(b.IPCDir, 0o755); err != nil {
		return fmt.Errorf("cannot create IPC directory %s: %w", b.IPCDir, err)
	}
	return nil
}

// CleanupStale removes leftover command/result files from a previous run.
// temp_script.gd is deliberately NOT removed here: script commands write it
// before the command file, and deleting it early could break an in-flight script.
func (b *Bridge) CleanupStale() error {
	for _, p := range []string{b.CommandPath(), b.ResultPath()} {
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("cannot clean stale file %s: %w", p, err)
		}
	}
	return nil
}

// Send writes payload to the command file and polls for the result file until
// timeout. JSON-like results are parsed to their native shape (map or slice);
// anything else is returned as a raw string. The result file is consumed
// (deleted) once read, matching the polling protocol.
func (b *Bridge) Send(payload any, timeout time.Duration) (any, error) {
	if err := b.ensureDir(); err != nil {
		return nil, err
	}
	if err := b.CleanupStale(); err != nil {
		return nil, err
	}

	data, err := encodePayload(payload)
	if err != nil {
		return nil, err
	}

	if err := os.WriteFile(b.CommandPath(), data, 0o644); err != nil {
		return nil, fmt.Errorf("cannot write command file: %w", err)
	}
	if b.Log != nil {
		fmt.Fprintf(b.Log, "Sending: %s\n", data)
	}

	if b.PollInterval <= 0 {
		b.PollInterval = PollInterval
	}

	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(b.PollInterval)
	defer ticker.Stop()

	for time.Now().Before(deadline) {
		raw, err := os.ReadFile(b.ResultPath())
		if err == nil {
			_ = os.Remove(b.ResultPath())
			return parseResult(raw), nil
		}
		<-ticker.C
	}

	return nil, fmt.Errorf("Timeout after %s. Is the game running?", timeout.Round(time.Second))
}

func encodePayload(payload any) ([]byte, error) {
	if text, ok := payload.(string); ok {
		return []byte(text), nil
	}
	if envelope, ok := payload.(map[string]any); ok {
		versioned := make(map[string]any, len(envelope)+1)
		for key, value := range envelope {
			versioned[key] = value
		}
		if _, exists := versioned["protocol"]; !exists {
			versioned["protocol"] = ProtocolVersion
		}
		payload = versioned
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("cannot encode command payload: %w", err)
	}
	return encoded, nil
}

// parseResult returns the JSON-decoded value for JSON payloads and the raw
// string otherwise, mirroring the old bridge's result handling.
func parseResult(raw []byte) any {
	trimmed := strings.TrimSpace(string(raw))
	if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
		var v any
		if err := json.Unmarshal(raw, &v); err == nil {
			return v
		}
	}
	return string(raw)
}
