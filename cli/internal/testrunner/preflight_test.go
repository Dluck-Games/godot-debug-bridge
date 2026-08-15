// godot-debug-bridge/cli/internal/testrunner/preflight_test.go
package testrunner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPreflightGdUnit(t *testing.T) {
	cases := []struct {
		name           string
		setup          func(t *testing.T, projectDir string)
		wantErr        bool
		wantSubstrings []string
	}{
		{
			name:    "not installed",
			wantErr: true,
			wantSubstrings: []string{
				"unit", "gdUnit4 addon", "not installed",
				"addons/gdUnit4/plugin.cfg", "install the gdUnit4",
			},
		},
		{
			name: "plugin-only partial",
			setup: func(t *testing.T, projectDir string) {
				writeTestFile(t, projectDir, "addons/gdUnit4/plugin.cfg", "[plugin]")
			},
			wantErr: true,
			wantSubstrings: []string{
				"unit", "gdUnit4 runner", "incomplete",
				"addons/gdUnit4/bin/GdUnitCmdTool.gd",
			},
		},
		{
			name: "runner-only partial",
			setup: func(t *testing.T, projectDir string) {
				writeTestFile(t, projectDir, "addons/gdUnit4/bin/GdUnitCmdTool.gd", "extends SceneTree")
			},
			wantErr: true,
			wantSubstrings: []string{
				"unit", "gdUnit4 addon", "not installed",
				"addons/gdUnit4/plugin.cfg",
			},
		},
		{
			name: "wrong type",
			setup: func(t *testing.T, projectDir string) {
				writeTestDir(t, projectDir, "addons/gdUnit4/plugin.cfg")
			},
			wantErr: true,
			wantSubstrings: []string{
				"unit", "gdUnit4 addon", "wrong type",
				"addons/gdUnit4/plugin.cfg", "regular file",
			},
		},
		{
			name: "tests/unit missing",
			setup: func(t *testing.T, projectDir string) {
				writeTestFile(t, projectDir, "addons/gdUnit4/plugin.cfg", "[plugin]")
				writeTestFile(t, projectDir, "addons/gdUnit4/bin/GdUnitCmdTool.gd", "extends SceneTree")
			},
			wantErr: true,
			wantSubstrings: []string{
				"unit", "unit tests directory", "incomplete", "tests/unit",
			},
		},
		{
			name: "complete",
			setup: func(t *testing.T, projectDir string) {
				writeTestFile(t, projectDir, "addons/gdUnit4/plugin.cfg", "[plugin]")
				writeTestFile(t, projectDir, "addons/gdUnit4/bin/GdUnitCmdTool.gd", "extends SceneTree")
				writeTestDir(t, projectDir, "tests/unit")
			},
			wantErr: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			projectDir := t.TempDir()
			if tc.setup != nil {
				tc.setup(t, projectDir)
			}
			err := Preflight(projectDir, []Tier{TierUnit})
			assertPreflightError(t, err, tc.wantErr, tc.wantSubstrings)
		})
	}
}

func TestPreflightDebugBridge(t *testing.T) {
	cases := []struct {
		name           string
		tiers          []Tier
		setup          func(t *testing.T, projectDir string)
		wantErr        bool
		wantSubstrings []string
	}{
		{
			name:    "not installed",
			tiers:   []Tier{TierIntegration, TierPlaytest},
			wantErr: true,
			wantSubstrings: []string{
				"integration", "GDBG addon", "not installed",
				"addons/gdbg/plugin.cfg",
			},
		},
		{
			name:  "incomplete integration assets",
			tiers: []Tier{TierIntegration},
			setup: func(t *testing.T, projectDir string) {
				writeTestFile(t, projectDir, "addons/gdbg/plugin.cfg", "[plugin]")
			},
			wantErr: true,
			wantSubstrings: []string{
				"integration", "GDBG integration runner", "incomplete",
				"addons/gdbg/testing/integration_runner.tscn",
			},
		},
		{
			name:  "incomplete playtest assets",
			tiers: []Tier{TierPlaytest},
			setup: func(t *testing.T, projectDir string) {
				writeTestFile(t, projectDir, "addons/gdbg/plugin.cfg", "[plugin]")
			},
			wantErr: true,
			wantSubstrings: []string{
				"playtest", "GDBG playtest runtime", "incomplete",
				"addons/gdbg/testing/test_runtime.gd",
			},
		},
		{
			name:  "wrong type",
			tiers: []Tier{TierPlaytest},
			setup: func(t *testing.T, projectDir string) {
				writeTestFile(t, projectDir, "addons/gdbg/plugin.cfg", "[plugin]")
				writeTestDir(t, projectDir, "addons/gdbg/testing/test_runtime.gd")
			},
			wantErr: true,
			wantSubstrings: []string{
				"playtest", "GDBG playtest runtime", "wrong type",
				"addons/gdbg/testing/test_runtime.gd", "regular file",
			},
		},
		{
			name:  "missing tests dirs",
			tiers: []Tier{TierIntegration, TierPlaytest},
			setup: func(t *testing.T, projectDir string) {
				writeTestFile(t, projectDir, "addons/gdbg/plugin.cfg", "[plugin]")
				writeTestFile(t, projectDir, "addons/gdbg/testing/integration_runner.tscn", "[gd_scene]")
				writeTestFile(t, projectDir, "addons/gdbg/testing/test_runtime.gd", "extends Node")
			},
			wantErr: true,
			wantSubstrings: []string{
				"integration", "integration tests directory", "incomplete", "tests/integration",
			},
		},
		{
			name:  "complete",
			tiers: []Tier{TierIntegration, TierPlaytest},
			setup: func(t *testing.T, projectDir string) {
				writeTestFile(t, projectDir, "addons/gdbg/plugin.cfg", "[plugin]")
				writeTestFile(t, projectDir, "addons/gdbg/testing/integration_runner.tscn", "[gd_scene]")
				writeTestFile(t, projectDir, "addons/gdbg/testing/test_runtime.gd", "extends Node")
				writeTestDir(t, projectDir, "tests/integration")
				writeTestDir(t, projectDir, "tests/playtest")
			},
			wantErr: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			projectDir := t.TempDir()
			if tc.setup != nil {
				tc.setup(t, projectDir)
			}
			err := Preflight(projectDir, tc.tiers)
			assertPreflightError(t, err, tc.wantErr, tc.wantSubstrings)
		})
	}
}

func TestPreflightWinsBeforeExecForCombinedTiers(t *testing.T) {
	projectDir := t.TempDir()
	// Unit dependencies are complete so an earlier tier would otherwise run.
	writeTestFile(t, projectDir, "addons/gdUnit4/plugin.cfg", "[plugin]")
	writeTestFile(t, projectDir, "addons/gdUnit4/bin/GdUnitCmdTool.gd", "extends SceneTree")
	writeTestDir(t, projectDir, "tests/unit")
	// Playtest dependencies are missing and the Godot binary does not exist;
	// preflight must report the dependency, not the failed exec.
	godotBin := filepath.Join(t.TempDir(), "definitely-not-godot")

	_, err := Run(godotBin, projectDir, []Tier{TierUnit, TierPlaytest}, RunOptions{})
	if err == nil {
		t.Fatal("expected Run to fail on missing playtest dependency")
	}
	if !strings.Contains(err.Error(), "playtest") || !strings.Contains(err.Error(), "GDBG") {
		t.Fatalf("expected preflight dependency error, got %v", err)
	}
	if strings.Contains(err.Error(), "failed to run gdUnit4") || strings.Contains(err.Error(), "fork/exec") {
		t.Fatalf("preflight must win before exec for combined tiers, got %v", err)
	}
}

func TestRunExecFailsWhenDependenciesComplete(t *testing.T) {
	projectDir := t.TempDir()
	writeTestFile(t, projectDir, "addons/gdUnit4/plugin.cfg", "[plugin]")
	writeTestFile(t, projectDir, "addons/gdUnit4/bin/GdUnitCmdTool.gd", "extends SceneTree")
	writeTestDir(t, projectDir, "tests/unit")
	godotBin := filepath.Join(t.TempDir(), "definitely-not-godot")

	_, err := Run(godotBin, projectDir, []Tier{TierUnit}, RunOptions{})
	if err == nil {
		t.Fatal("expected Run to fail when Godot cannot be executed")
	}
	if strings.Contains(err.Error(), "not installed") || strings.Contains(err.Error(), "incomplete") {
		t.Fatalf("preflight should have passed; expected exec failure, got %v", err)
	}
}

func writeTestFile(t *testing.T, projectDir, rel, content string) {
	t.Helper()
	full := filepath.Join(projectDir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(full), err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
}

func writeTestDir(t *testing.T, projectDir, rel string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(projectDir, filepath.FromSlash(rel)), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", rel, err)
	}
}

func assertPreflightError(t *testing.T, err error, wantErr bool, wantSubstrings []string) {
	t.Helper()
	if !wantErr {
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		return
	}
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	for _, s := range wantSubstrings {
		if !strings.Contains(err.Error(), s) {
			t.Fatalf("error %q missing substring %q", err.Error(), s)
		}
	}
}
