package debug

import (
	"math"
	"reflect"
	"strings"
	"testing"
)

func TestParseFlags_Empty(t *testing.T) {
	f, positional, err := ParseFlags(nil)
	if err != nil {
		t.Fatalf("ParseFlags: %v", err)
	}
	if len(positional) != 0 {
		t.Errorf("positional = %v, want empty", positional)
	}
	if !reflect.DeepEqual(f, Flags{}) {
		t.Errorf("flags = %+v, want zero value", f)
	}
}

func TestParseFlags_KeepsPositional(t *testing.T) {
	f, positional, err := ParseFlags([]string{"heal", "full"})
	if err != nil {
		t.Fatalf("ParseFlags: %v", err)
	}
	if !reflect.DeepEqual(positional, []string{"heal", "full"}) {
		t.Errorf("positional = %v", positional)
	}
	if f.Screenshot {
		t.Error("unexpected screenshot flag")
	}
}

func TestParseFlags_ScreenshotVariants(t *testing.T) {
	f, positional, err := ParseFlags([]string{
		"heal", "full", "--screenshot", "--delay", "2", "--count", "5",
		"--interval", "0.5", "--width", "1920", "--height", "1080", "--scale", "2",
	})
	if err != nil {
		t.Fatalf("ParseFlags: %v", err)
	}
	if !f.Screenshot || !f.HasDelay || !f.HasCount || !f.HasInterval || !f.HasWidth || !f.HasHeight || !f.HasScale {
		t.Errorf("presence flags wrong: %+v", f)
	}
	if f.Delay != 2 || f.Count != 5 || f.Interval != 0.5 || f.Width != 1920 || f.Height != 1080 || f.Scale != 2 {
		t.Errorf("values wrong: %+v", f)
	}
	if !reflect.DeepEqual(positional, []string{"heal", "full"}) {
		t.Errorf("positional = %v", positional)
	}

	// Short forms.
	f2, p2, err := ParseFlags([]string{"-s", "-d", "3", "-c", "4", "-i", "1.5"})
	if err != nil {
		t.Fatalf("ParseFlags: %v", err)
	}
	if !f2.Screenshot || f2.Delay != 3 || f2.Count != 4 || f2.Interval != 1.5 {
		t.Errorf("short flags wrong: %+v", f2)
	}
	if len(p2) != 0 {
		t.Errorf("positional = %v", p2)
	}
}

func TestParseFlags_RecordFlags(t *testing.T) {
	f, positional, err := ParseFlags([]string{"--seconds", "10", "--fps", "4", "--output", "/tmp/play.mp4", "--width", "1280"})
	if err != nil {
		t.Fatalf("ParseFlags: %v", err)
	}
	if f.Seconds != 10 || f.FPS != 4 || f.Output != "/tmp/play.mp4" || f.Width != 1280 {
		t.Errorf("record flags wrong: %+v", f)
	}
	if len(positional) != 0 {
		t.Errorf("positional = %v", positional)
	}

	// Invalid numerics stay NaN (rejected later by BuildRecordOpts).
	f2, _, err := ParseFlags([]string{"--seconds", "abc", "--fps", "bad"})
	if err != nil {
		t.Fatalf("ParseFlags: %v", err)
	}
	if !math.IsNaN(f2.Seconds) || !math.IsNaN(f2.FPS) {
		t.Errorf("expected NaN for invalid numerics: %+v", f2)
	}

	// Non-integer fps is preserved for the validator to reject.
	f3, _, err := ParseFlags([]string{"--fps", "1.5"})
	if err != nil {
		t.Fatalf("ParseFlags: %v", err)
	}
	if f3.FPS != 1.5 {
		t.Errorf("fps = %v, want 1.5", f3.FPS)
	}
}

func TestParseFlags_InvalidDelayFallsBackZero(t *testing.T) {
	f, _, err := ParseFlags([]string{"--delay", "abc"})
	if err != nil {
		t.Fatalf("ParseFlags: %v", err)
	}
	if !f.HasDelay || f.Delay != 0 {
		t.Errorf("delay = %v (has=%v), want 0", f.Delay, f.HasDelay)
	}
}

func TestParseFlags_InvalidCountFallsBackOne(t *testing.T) {
	f, _, err := ParseFlags([]string{"--count", "abc"})
	if err != nil {
		t.Fatalf("ParseFlags: %v", err)
	}
	if !f.HasCount || f.Count != 1 {
		t.Errorf("count = %v (has=%v), want 1", f.Count, f.HasCount)
	}
}

func TestParseFlags_OutputRequiresArgument(t *testing.T) {
	for _, args := range [][]string{{"--output"}, {"--output", "--seconds", "5"}, {"record", "--output"}} {
		_, _, err := ParseFlags(args)
		if err == nil {
			t.Errorf("expected error for args %v", args)
			continue
		}
		if !strings.Contains(err.Error(), ".mp4 or .mov") {
			t.Errorf("unexpected error for %v: %v", args, err)
		}
	}
}

func TestParseFlags_ProjectDirForms(t *testing.T) {
	for _, form := range []string{"--project-dir", "--path", "-p"} {
		f, _, err := ParseFlags([]string{form, "/proj"})
		if err != nil {
			t.Fatalf("ParseFlags(%s): %v", form, err)
		}
		if f.ProjectDir != "/proj" {
			t.Errorf("%s: projectDir = %q", form, f.ProjectDir)
		}
	}

	if _, _, err := ParseFlags([]string{"--path"}); err == nil {
		t.Error("expected error for bare --path")
	}
}
