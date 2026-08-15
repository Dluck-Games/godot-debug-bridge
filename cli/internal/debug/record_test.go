package debug

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBuildRecordOpts_Defaults(t *testing.T) {
	opts, err := BuildRecordOpts(Flags{})
	if err != nil {
		t.Fatalf("BuildRecordOpts: %v", err)
	}
	if opts.Seconds != 10 || opts.FPS != 4 || opts.Output != "" {
		t.Errorf("opts = %+v", opts)
	}
	if opts.Screenshot["width"] != 1920 || opts.Screenshot["height"] != 1080 {
		t.Errorf("default screenshot opts = %v", opts.Screenshot)
	}
}

func TestBuildRecordOpts_PreservesResizeOpts(t *testing.T) {
	f, _, err := ParseFlags([]string{"--scale", "2"})
	if err != nil {
		t.Fatal(err)
	}
	opts, err := BuildRecordOpts(f)
	if err != nil {
		t.Fatal(err)
	}
	if opts.Screenshot["scale"] != 2.0 {
		t.Errorf("screenshot opts = %v", opts.Screenshot)
	}
	if _, hasWidth := opts.Screenshot["width"]; hasWidth {
		t.Error("width should not be defaulted when scale is set")
	}
}

func TestBuildRecordOpts_Validation(t *testing.T) {
	cases := []struct {
		args []string
		msg  string
	}{
		{[]string{"--seconds", "0"}, "seconds"},
		{[]string{"--seconds", "301"}, "seconds"},
		{[]string{"--seconds", "abc"}, "seconds"},
		{[]string{"--fps", "0"}, "fps"},
		{[]string{"--fps", "13"}, "fps"},
		{[]string{"--fps", "1.5"}, "fps"},
		{[]string{"--output", "/tmp/x.txt"}, ".mp4 or .mov"},
	}
	for _, c := range cases {
		f, _, err := ParseFlags(c.args)
		if err != nil {
			t.Fatalf("ParseFlags(%v): %v", c.args, err)
		}
		_, err = BuildRecordOpts(f)
		if err == nil {
			t.Errorf("args %v: expected validation error", c.args)
			continue
		}
		if !strings.Contains(err.Error(), c.msg) {
			t.Errorf("args %v: error %q missing %q", c.args, err, c.msg)
		}
	}
}

func TestBuildRecordOpts_AcceptsMov(t *testing.T) {
	f, _, err := ParseFlags([]string{"--output", "/tmp/play.mov"})
	if err != nil {
		t.Fatal(err)
	}
	opts, err := BuildRecordOpts(f)
	if err != nil {
		t.Fatalf("BuildRecordOpts: %v", err)
	}
	if opts.Output != "/tmp/play.mov" {
		t.Errorf("output = %q", opts.Output)
	}
}

// fakeFrameSender returns frame paths from a prepared pool, count per call.
func fakeFrameSender(allFrames []string, batchSizes *[]int, countPtr *int) Sender {
	return func(payload map[string]any, timeout time.Duration) (any, error) {
		args, _ := payload["args"].(map[string]any)
		count := 0
		switch v := args["count"].(type) {
		case int:
			count = v
		case float64:
			count = int(v)
		}
		if *countPtr+count > len(allFrames) {
			return nil, fmt.Errorf("fake sender ran out of frames")
		}
		batch := allFrames[*countPtr : *countPtr+count]
		*countPtr += count
		*batchSizes = append(*batchSizes, count)
		result := make([]any, len(batch))
		for i, p := range batch {
			result[i] = p
		}
		return map[string]any{"result": result}, nil
	}
}

func makeFramePool(t *testing.T, n int) []string {
	t.Helper()
	dir := t.TempDir()
	var frames []string
	for i := 0; i < n; i++ {
		frames = append(frames, writeTestPNG(t, dir, fmt.Sprintf("frame_%02d.png", i), 64, 48, uint8(i*5)))
	}
	return frames
}

func TestExecuteRecord_DefaultOutputAndStagingCleanup(t *testing.T) {
	stateRoot := t.TempDir()
	t.Setenv("GDBG_STATE", stateRoot)

	pool := makeFramePool(t, 50)
	var batchSizes []int
	captured := 0
	send := fakeFrameSender(pool, &batchSizes, &captured)

	opts := RecordOptions{
		Seconds:    10,
		FPS:        4, // 40 frames -> two batches of 20
		Screenshot: map[string]any{"width": 64, "height": 48},
	}
	result, err := ExecuteRecord(send, opts)
	if err != nil {
		t.Fatalf("ExecuteRecord: %v", err)
	}

	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("result type %T", result)
	}
	out, ok := m["result"].(string)
	if !ok {
		t.Fatalf("result missing output path: %#v", m)
	}
	wantDir := filepath.Join(stateRoot, "debug", "recordings")
	if filepath.Dir(out) != wantDir {
		t.Errorf("output dir = %s, want %s", filepath.Dir(out), wantDir)
	}
	fi, err := os.Stat(out)
	if err != nil {
		t.Fatalf("output missing: %v", err)
	}
	if fi.Size() == 0 {
		t.Error("recording is empty")
	}

	if captured != 40 {
		t.Errorf("captured %d frames, want 40", captured)
	}
	if len(batchSizes) != 2 || batchSizes[0] != 20 || batchSizes[1] != 20 {
		t.Errorf("batch sizes = %v, want [20 20]", batchSizes)
	}

	// Staging must have no leftover frame files.
	staging := filepath.Join(stateRoot, "debug", "staging")
	entries, err := os.ReadDir(staging)
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("read staging: %v", err)
	}
	for _, e := range entries {
		t.Errorf("staging leftover: %s", e.Name())
	}

	checkPlayable(t, out)
}

func TestExecuteRecord_ExplicitOutput(t *testing.T) {
	t.Setenv("GDBG_STATE", t.TempDir())

	pool := makeFramePool(t, 10)
	var batchSizes []int
	captured := 0
	send := fakeFrameSender(pool, &batchSizes, &captured)

	explicit := filepath.Join(t.TempDir(), "clip.mov")
	opts := RecordOptions{
		Seconds:    2,
		FPS:        2, // 4 frames, single batch
		Output:     explicit,
		Screenshot: map[string]any{"width": 64, "height": 48},
	}
	result, err := ExecuteRecord(send, opts)
	if err != nil {
		t.Fatalf("ExecuteRecord: %v", err)
	}
	m := result.(map[string]any)
	if got := m["result"]; got != explicit {
		t.Errorf("result = %v, want %v", got, explicit)
	}
	if captured != 4 {
		t.Errorf("captured %d frames, want 4", captured)
	}
	fi, err := os.Stat(explicit)
	if err != nil {
		t.Fatalf("explicit output missing: %v", err)
	}
	if fi.Size() == 0 {
		t.Error("recording is empty")
	}
	verifyRecording(t, explicit, 64, 48, 4, "qt  ")
	checkPlayable(t, explicit)
}

func TestExecuteRecord_CapturesFrameCountMismatch(t *testing.T) {
	t.Setenv("GDBG_STATE", t.TempDir())

	pool := makeFramePool(t, 5)
	send := func(payload map[string]any, timeout time.Duration) (any, error) {
		// Always return one fewer frame than requested.
		args, _ := payload["args"].(map[string]any)
		count := 0
		switch v := args["count"].(type) {
		case int:
			count = v
		case float64:
			count = int(v)
		}
		if count > len(pool) {
			count = len(pool)
		}
		result := make([]any, count-1)
		for i := range result {
			result[i] = pool[i]
		}
		return map[string]any{"result": result}, nil
	}

	opts := RecordOptions{Seconds: 1, FPS: 2, Screenshot: map[string]any{"width": 64, "height": 48}}
	_, err := ExecuteRecord(send, opts)
	if err == nil || !strings.Contains(err.Error(), "expected 2 frames but captured 1") {
		t.Errorf("expected frame count mismatch error, got %v", err)
	}
}
