package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Dluck-Games/godot-debug-bridge/cli/internal/debug"
	"github.com/spf13/cobra"
)

func TestPerfCapture(t *testing.T) {
	t.Setenv("GDBG_STATE", t.TempDir())
	dir := debug.IPCDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	errCh := make(chan error, 1)
	go func() {
		for _, op := range []string{"start", "stop"} {
			var request struct {
				Cmd  string
				Args map[string]string
			}
			deadline := time.Now().Add(3 * time.Second)
			for {
				b, err := os.ReadFile(filepath.Join(dir, "command"))
				if err == nil && json.Unmarshal(b, &request) == nil {
					break
				}
				if time.Now().After(deadline) {
					errCh <- fmt.Errorf("waiting for %s", op)
					return
				}
				time.Sleep(time.Millisecond)
			}
			if request.Cmd != "perf" || request.Args["op"] != op || (op == "start" && request.Args["prefix"] != "ui/") || (op == "stop" && !filepath.IsAbs(request.Args["out"])) {
				errCh <- fmt.Errorf("unexpected request %+v", request)
				return
			}
			os.Remove(filepath.Join(dir, "command"))
			if err := os.WriteFile(filepath.Join(dir, "result"), []byte(`{"result":{"frames":4}}`), 0600); err != nil {
				errCh <- err
				return
			}
		}
		errCh <- nil
	}()
	cmd := &cobra.Command{Use: "capture"}
	cmd.SetContext(context.Background())
	cmd.Flags().String("prefix", "ui/", "")
	cmd.Flags().String("out", "capture.json", "")
	cmd.Flags().Float64("duration", 0.001, "")
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	if err := runPerf(cmd, nil); err != nil {
		t.Fatal(err)
	}
	if err := <-errCh; err != nil {
		t.Fatal(err)
	}
	if buf.String() != "{\"frames\":4}\n" {
		t.Fatal(buf.String())
	}
}
func TestPerfInvalidDuration(t *testing.T) {
	cmd := &cobra.Command{Use: "capture"}
	cmd.Flags().Float64("duration", -1, "")
	if runPerf(cmd, nil) == nil {
		t.Fatal("accepted negative duration")
	}
}
