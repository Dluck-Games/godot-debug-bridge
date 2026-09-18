package godot

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestMain re-executes the test binary as the fake importer when invoked with
// Godot's argument vector, which the Go test flag parser would otherwise reject.
func TestMain(m *testing.M) {
	if os.Getenv(fakeGodotHelperEnv) == "1" {
		os.Exit(helperExitCode(os.Getenv(fakeGodotArgsEnv), os.Args[1:]))
	}
	os.Exit(m.Run())
}

// helperExitCode records the invocation arguments and reports the configured
// exit status. The helper runs before flag parsing, so it must not use the test
// framework.
func helperExitCode(argsFile string, args []string) int {
	if argsFile != "" {
		if err := os.WriteFile(argsFile, []byte(strings.Join(args, "\n")), 0o644); err != nil {
			return 1
		}
	}
	code, err := strconv.Atoi(os.Getenv(fakeGodotExitEnv))
	if err != nil {
		return 0
	}
	return code
}

func TestImportFailureMarkerRejectsGodotScriptErrors(t *testing.T) {
	for _, output := range []string{
		"SCRIPT ERROR: Parse Error: unexpected token",
		"SCRIPT ERROR: Compile Error: failed dependency",
		"ERROR: Failed to load script res://main.gd",
	} {
		if importFailureMarker(output) == "" {
			t.Fatalf("expected failure marker for %q", output)
		}
	}
}

func TestImportFailureMarkerAllowsOrdinaryImportOutput(t *testing.T) {
	if marker := importFailureMarker("Godot Engine v4\n[DONE] first_scan_filesystem\n"); marker != "" {
		t.Fatalf("unexpected marker %q", marker)
	}
}

// fakeGodotArgsFile is set by the helper process branch of TestMain-style
// subprocess re-execution so the fake importer can record its arguments.
const (
	fakeGodotHelperEnv = "GDBG_FAKE_GODOT_HELPER"
	fakeGodotArgsEnv   = "GDBG_FAKE_GODOT_ARGS_FILE"
	fakeGodotExitEnv   = "GDBG_FAKE_GODOT_EXIT_CODE"
)

// fakeGodotExecutable re-executes the current test binary as a stand-in for the
// Godot binary. This is portable across supported OSes because it does not rely
// on shell scripts or platform-specific script interpreters.
func fakeGodotExecutable(t *testing.T) string {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("resolve test executable: %v", err)
	}
	return exe
}

// TestRunImportPassesHeadlessImportArgs asserts the exact argument vector the
// importer is invoked with.
func TestRunImportPassesHeadlessImportArgs(t *testing.T) {
	argsFile := filepath.Join(t.TempDir(), "args.txt")
	t.Setenv(fakeGodotHelperEnv, "1")
	t.Setenv(fakeGodotArgsEnv, argsFile)

	projectDir := t.TempDir()
	if err := RunImport(fakeGodotExecutable(t), projectDir); err != nil {
		t.Fatalf("RunImport returned error: %v", err)
	}

	raw, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("fake importer did not record args: %v", err)
	}
	got := strings.Split(strings.TrimSpace(string(raw)), "\n")
	want := []string{"--headless", "--import", "--path", projectDir}
	if len(got) != len(want) {
		t.Fatalf("args = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("args = %v, want %v", got, want)
		}
	}
}

// TestEnsureImportCacheInvokesImporterWithExistingCacheButStaleImport covers the
// regression: a .godot directory already exists (tracked source + .import
// merged from another worktree) while the imported output it points at is
// absent. The importer must still run.
func TestEnsureImportCacheInvokesImporterWithExistingCacheButStaleImport(t *testing.T) {
	projectDir := t.TempDir()
	godotDir := filepath.Join(projectDir, ".godot")
	if err := os.MkdirAll(filepath.Join(godotDir, "imported"), 0o755); err != nil {
		t.Fatalf("seed .godot: %v", err)
	}
	// A tracked .import file pointing at an absent ctex artifact.
	importFile := filepath.Join(projectDir, "icon.png.import")
	if err := os.WriteFile(importFile, []byte("[remap]\npath=\"res://.godot/imported/icon.png-abc.ctex\"\n"), 0o644); err != nil {
		t.Fatalf("seed .import: %v", err)
	}

	argsFile := filepath.Join(t.TempDir(), "args.txt")
	t.Setenv(fakeGodotHelperEnv, "1")
	t.Setenv(fakeGodotArgsEnv, argsFile)

	if err := EnsureImportCache(fakeGodotExecutable(t), projectDir); err != nil {
		t.Fatalf("EnsureImportCache returned error: %v", err)
	}

	raw, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("importer was not invoked despite existing .godot with absent ctex: %v", err)
	}
	if !strings.Contains(string(raw), "--import") {
		t.Fatalf("recorded args = %q, want --import", string(raw))
	}
}

// TestEnsureImportCacheInvokesImporterWhenSourceChangesWhileCacheExists asserts
// the importer runs again on a subsequent call after a source edit, even though
// the .godot directory is present.
func TestEnsureImportCacheInvokesImporterWhenSourceChangesWhileCacheExists(t *testing.T) {
	projectDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(projectDir, ".godot", "imported"), 0o755); err != nil {
		t.Fatalf("seed .godot: %v", err)
	}
	source := filepath.Join(projectDir, "main.gd")
	if err := os.WriteFile(source, []byte("extends Node\n"), 0o644); err != nil {
		t.Fatalf("seed source: %v", err)
	}

	argsFile := filepath.Join(t.TempDir(), "args.txt")
	t.Setenv(fakeGodotHelperEnv, "1")
	t.Setenv(fakeGodotArgsEnv, argsFile)

	if err := EnsureImportCache(fakeGodotExecutable(t), projectDir); err != nil {
		t.Fatalf("first EnsureImportCache: %v", err)
	}
	first, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("importer not invoked on first call: %v", err)
	}
	if err := os.Remove(argsFile); err != nil {
		t.Fatalf("reset args file: %v", err)
	}

	if err := os.WriteFile(source, []byte("extends Node\nfunc _ready():\n\tpass\n"), 0o644); err != nil {
		t.Fatalf("edit source: %v", err)
	}
	if err := EnsureImportCache(fakeGodotExecutable(t), projectDir); err != nil {
		t.Fatalf("second EnsureImportCache: %v", err)
	}
	second, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("importer not invoked after source change: %v", err)
	}
	if string(first) != string(second) {
		t.Fatalf("args differ between runs: first=%q second=%q", first, second)
	}
}

// TestEnsureImportCachePropagatesImportFailure asserts import errors abort
// before any game launch.
func TestEnsureImportCachePropagatesImportFailure(t *testing.T) {
	projectDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(projectDir, ".godot"), 0o755); err != nil {
		t.Fatalf("seed .godot: %v", err)
	}
	argsFile := filepath.Join(t.TempDir(), "args.txt")
	t.Setenv(fakeGodotHelperEnv, "1")
	t.Setenv(fakeGodotArgsEnv, argsFile)
	t.Setenv(fakeGodotExitEnv, "1")

	if err := EnsureImportCache(fakeGodotExecutable(t), projectDir); err == nil {
		t.Fatal("expected import failure to propagate, got nil")
	}
	if _, err := os.Stat(argsFile); err != nil {
		t.Fatalf("importer was not invoked: %v", err)
	}
}
