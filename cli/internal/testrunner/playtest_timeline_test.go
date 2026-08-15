package testrunner

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTailPlaytestTimelineWritesNewLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "timeline.log")
	stop := make(chan struct{})
	var out bytes.Buffer
	done := make(chan struct{})
	go func() {
		tailPlaytestTimeline(path, &out, stop)
		close(done)
	}()

	time.Sleep(80 * time.Millisecond)
	if err := os.WriteFile(path, []byte("[playtest:demo] start seed=1 timeout=60s\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(out.String(), "start seed=1") {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !strings.Contains(out.String(), "[playtest:demo] start seed=1 timeout=60s") {
		close(stop)
		<-done
		t.Fatalf("missing live start line, got %q", out.String())
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		close(stop)
		<-done
		t.Fatal(err)
	}
	if _, err := f.WriteString("[playtest:demo] 0.9s  PASS  player_reached_npc\n"); err != nil {
		f.Close()
		close(stop)
		<-done
		t.Fatal(err)
	}
	f.Close()
	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(out.String(), "PASS  player_reached_npc") {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	close(stop)
	<-done
	if !strings.Contains(out.String(), "PASS  player_reached_npc") {
		t.Fatalf("missing live pass line, got %q", out.String())
	}
}
