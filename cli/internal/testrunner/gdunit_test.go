package testrunner

import (
	"path/filepath"
	"testing"
)

func TestBuildGdUnitArgsDoesNotEnableGodotVerboseForCliVerbose(t *testing.T) {
	args := buildGdUnitArgs(RunOptions{
		Verbose: true,
		Suite:   []string{"system"},
	}, "../logs/gdunit")

	for _, arg := range args {
		if arg == "--verbose" {
			t.Fatal("CLI verbose must not enable Godot/gdUnit verbose output")
		}
	}

	assertArgSequence(t, args, []string{"-s", GdUnit4CmdToolRes})
	assertArgSequence(t, args, []string{"--add", UnitTestsDirRes + "system/"})
	assertArgSequence(t, args, []string{"--ignoreHeadlessMode", "--report-directory", "../logs/gdunit"})
}

func TestGdUnitLogStateUsesGDBGState(t *testing.T) {
	stateRoot := t.TempDir()
	t.Setenv("GDBG_STATE", stateRoot)
	if got, want := logStateBase(), filepath.Join(stateRoot, "logs"); got != want {
		t.Fatalf("logStateBase() = %q, want %q", got, want)
	}
}

func assertArgSequence(t *testing.T, args []string, want []string) {
	t.Helper()
	for i := 0; i <= len(args)-len(want); i++ {
		matched := true
		for j := range want {
			if args[i+j] != want[j] {
				matched = false
				break
			}
		}
		if matched {
			return
		}
	}
	t.Fatalf("args %v do not contain sequence %v", args, want)
}
