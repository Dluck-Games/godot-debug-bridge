package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Dluck-Games/godot-debug-bridge/cli/internal/record"
)

func resetRecordFlags() {
	flagRecOutput = ""
	flagRecFPS = record.DefaultFPS
	flagRecSeconds = record.DefaultSeconds
	flagRecResolution = record.DefaultResolution
	flagRecWidth = 0
	flagRecHeight = 0
	flagRecScaleFilter = string(record.ScaleBilinear)
	flagRecScene = ""
	flagRecMute = false
	flagRecCodec = string(record.CodecAuto)
	flagRecQuality = record.DefaultQuality
	flagRecOffscreen = "auto"
	flagRecContactSheet = ""
	flagRecManifest = ""
	flagRecMaxFrames = 0
	flagRecTimeout = record.DefaultTimeout
	flagRecNoWindowLock = false
	flagRecAllowDroppedFrames = false
	flagRecKeepRaw = false
}

func TestBuildRecordOptions_ScriptAndExtraArgs(t *testing.T) {
	resetRecordFlags()
	proj := t.TempDir()
	script := filepath.Join(t.TempDir(), "director.gd")
	if err := os.WriteFile(script, []byte("extends Node\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	flagRecFPS = 60
	flagRecOutput = filepath.Join(t.TempDir(), "demo.mp4")

	opts, err := buildRecordOptions(proj, []string{script, "--skip-menu", "--seed=7"}, 1)
	if err != nil {
		t.Fatalf("buildRecordOptions: %v", err)
	}
	if opts.ScriptPath != script || opts.FPS != 60 {
		t.Fatalf("unexpected opts: %+v", opts)
	}
	if len(opts.ExtraArgs) != 2 || opts.ExtraArgs[0] != "--skip-menu" {
		t.Fatalf("ExtraArgs = %v", opts.ExtraArgs)
	}
}

func TestBuildRecordOptions_RejectsExtraPositionalBeforeDash(t *testing.T) {
	resetRecordFlags()
	proj := t.TempDir()
	script := filepath.Join(t.TempDir(), "director.gd")
	if err := os.WriteFile(script, []byte("extends Node\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := buildRecordOptions(proj, []string{script, "extra"}, -1); err == nil || !strings.Contains(err.Error(), "after '--'") {
		t.Fatalf("expected extra args error, got %v", err)
	}
	if _, err := buildRecordOptions(proj, []string{script, "extra", "--skip-menu"}, 2); err == nil || !strings.Contains(err.Error(), "before '--'") {
		t.Fatalf("expected before-dash error, got %v", err)
	}
}
