package testrunner

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestPlaytestGodotArgsUsesProductionMainSceneImplicitly(t *testing.T) {
	args := playtestGodotArgs("proj", "smoke", "out", "frames", RunOptions{})
	assertArgSequence(t, args, []string{"--path", "proj"})
	for _, arg := range args {
		if strings.HasSuffix(arg, ".tscn") {
			t.Fatalf("playtest must omit the explicit scene (application/run/main_scene is the production entry): %v", args)
		}
		if strings.Contains(arg, "scenes/main.tscn") {
			t.Fatalf("playtest must not hardcode res://scenes/main.tscn: %v", args)
		}
	}
}

func TestPlaytestLogStateUsesGDBGState(t *testing.T) {
	stateRoot := t.TempDir()
	t.Setenv("GDBG_STATE", stateRoot)
	if got, want := playtestLogStateBase(), filepath.Join(stateRoot, "logs"); got != want {
		t.Fatalf("playtestLogStateBase() = %q, want %q", got, want)
	}
}

func TestPlaytestGodotArgsBackgroundByDefault(t *testing.T) {
	args := playtestGodotArgs("proj", "input_flow", "out", "frames", RunOptions{})
	if args[0] != "--windowed" {
		t.Fatalf("display = %q, want --windowed", args[0])
	}
	assertArgSequence(t, args, []string{"--playtest=input_flow"})
	assertArgSequence(t, args, []string{"--playtest-background"})
	for _, arg := range args {
		if arg == "--headless" {
			t.Fatalf("default playtest must not pass --headless: %v", args)
		}
	}
}

func TestPlaytestGodotArgsHeadlessSilent(t *testing.T) {
	args := playtestGodotArgs("proj", "input_flow", "out", "frames", RunOptions{Headless: true})
	if args[0] != "--headless" {
		t.Fatalf("display = %q, want --headless", args[0])
	}
	assertArgSequence(t, args, []string{"--playtest=input_flow"})
	for _, arg := range args {
		if arg == "--windowed" || arg == "--record" || arg == "--playtest-background" {
			t.Fatalf("headless playtest must not pass windowed/record/background: %v", args)
		}
	}
}

func TestPlaytestGodotArgsWindowedForeground(t *testing.T) {
	args := playtestGodotArgs("proj", "input_flow", "out", "frames", RunOptions{Windowed: true})
	if args[0] != "--windowed" {
		t.Fatalf("display = %q, want --windowed", args[0])
	}
	for _, arg := range args {
		if arg == "--headless" || arg == "--playtest-background" {
			t.Fatalf("foreground playtest must not pass headless/background: %v", args)
		}
	}
}

func TestShouldTailPlaytestTimeline(t *testing.T) {
	cases := []struct {
		name string
		opts RunOptions
		want bool
	}{
		{name: "verbose disables tail", opts: RunOptions{Verbose: true}, want: false},
		{name: "non-verbose keeps tail", opts: RunOptions{}, want: true},
		{name: "headless non-verbose keeps tail", opts: RunOptions{Headless: true}, want: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := shouldTailPlaytestTimeline(tc.opts); got != tc.want {
				t.Fatalf("shouldTailPlaytestTimeline() = %v, want %v", got, tc.want)
			}
		})
	}
}
