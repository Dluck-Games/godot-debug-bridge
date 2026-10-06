package testrunner

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

// newPlaytestFixtureProject builds a generic temporary Godot project used by
// the playtest discovery regressions. Accepted and rejected script fixtures
// live side by side so each case exercises the real resolver boundaries rather
// than a mock.
func newPlaytestFixtureProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		// Canonical addon base script.
		"addons/gdbg/testing/automation_playtest_suite.gd": "extends Node\n",
		// Non-playtest helper scripts outside the playtest directory.
		"scripts/fixture_base.gd":    "extends \"res://addons/gdbg/testing/automation_playtest_suite.gd\"\n",
		"scripts/fixture_top.gd":     "extends \"res://scripts/fixture_base.gd\"\n",
		"scripts/fixture_cycle_a.gd": "extends \"res://scripts/fixture_cycle_b.gd\"\n",
		"scripts/fixture_cycle_b.gd": "extends \"res://scripts/fixture_cycle_a.gd\"\n",
		// Accepted playtest scripts.
		"tests/playtest/playtest_alias_a.gd":   "extends AutomationPlayTestSuite\n",
		"tests/playtest/playtest_alias_b.gd":   "extends DebugBridgePlaytestSuite\n",
		"tests/playtest/playtest_basepath.gd":  "extends \"res://addons/gdbg/testing/automation_playtest_suite.gd\"\n",
		"tests/playtest/playtest_one_level.gd": "extends \"res://scripts/fixture_base.gd\"\n",
		"tests/playtest/playtest_chain.gd":     "extends \"res://scripts/fixture_top.gd\"\n",
		"tests/playtest/playtest_inline.gd":    "extends \"res://scripts/fixture_top.gd\" # derived suite\n",
		// Rejected playtest scripts.
		"tests/playtest/playtest_nonplaytest.gd":    "extends Node\n",
		"tests/playtest/playtest_missing_parent.gd": "extends \"res://scripts/fixture_missing.gd\"\n",
		"tests/playtest/playtest_escape.gd":         "extends \"res://../outside.gd\"\n",
		"tests/playtest/playtest_comment_spoof.gd":  "# extends AutomationPlayTestSuite\n# extends \"res://addons/gdbg/testing/automation_playtest_suite.gd\"\nextends Node\n",
		"tests/playtest/playtest_cycle.gd":          "extends \"res://scripts/fixture_cycle_a.gd\"\n",
		"tests/playtest/playtest_alias_prefix.gd":   "extends AutomationPlayTestSuiteExtra\n",
	}
	for rel, body := range files {
		writeTestScript(t, root, rel, body)
	}
	return root
}

func writeTestScript(t *testing.T, root, rel, body string) string {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir for %s: %v", rel, err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
	return path
}

func TestPlaytestDiscoveryAcceptsRealPlaytestExtensions(t *testing.T) {
	root := newPlaytestFixtureProject(t)
	cases := []struct {
		name string
		rel  string
	}{
		{"direct AutomationPlayTestSuite alias", "tests/playtest/playtest_alias_a.gd"},
		{"direct DebugBridgePlaytestSuite alias", "tests/playtest/playtest_alias_b.gd"},
		{"canonical addon base path", "tests/playtest/playtest_basepath.gd"},
		{"one-level quoted project parent", "tests/playtest/playtest_one_level.gd"},
		{"two-level quoted project parent chain", "tests/playtest/playtest_chain.gd"},
		{"quoted parent with inline comment", "tests/playtest/playtest_inline.gd"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(root, filepath.FromSlash(tc.rel))
			if !isAutomationPlaytest(root, path) {
				t.Fatalf("expected %s to be detected as a playtest", tc.rel)
			}
		})
	}
}

func TestPlaytestDiscoveryRejectsNonPlaytestAndSpoofs(t *testing.T) {
	root := newPlaytestFixtureProject(t)
	cases := []struct {
		name string
		rel  string
	}{
		{"non-playtest base", "tests/playtest/playtest_nonplaytest.gd"},
		{"missing quoted parent", "tests/playtest/playtest_missing_parent.gd"},
		{"parent escaping project root", "tests/playtest/playtest_escape.gd"},
		{"addon path only in comment", "tests/playtest/playtest_comment_spoof.gd"},
		{"cyclic inheritance", "tests/playtest/playtest_cycle.gd"},
		{"alias prefix is not a real alias", "tests/playtest/playtest_alias_prefix.gd"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(root, filepath.FromSlash(tc.rel))
			if isAutomationPlaytest(root, path) {
				t.Fatalf("expected %s to be rejected as a playtest", tc.rel)
			}
		})
	}
}

func TestPlaytestDiscoveryUnfilteredFindsOnlyAcceptedScripts(t *testing.T) {
	root := newPlaytestFixtureProject(t)
	got, err := discoverPlaytests(root, playtestTestsDir(root), nil)
	if err != nil {
		t.Fatalf("discoverPlaytests returned error: %v", err)
	}
	want := []string{
		filepath.Join(root, filepath.FromSlash("tests/playtest/playtest_alias_a.gd")),
		filepath.Join(root, filepath.FromSlash("tests/playtest/playtest_alias_b.gd")),
		filepath.Join(root, filepath.FromSlash("tests/playtest/playtest_basepath.gd")),
		filepath.Join(root, filepath.FromSlash("tests/playtest/playtest_chain.gd")),
		filepath.Join(root, filepath.FromSlash("tests/playtest/playtest_inline.gd")),
		filepath.Join(root, filepath.FromSlash("tests/playtest/playtest_one_level.gd")),
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("discoverPlaytests() = %v, want %v", got, want)
	}
}

func TestPlaytestDiscoverySuiteFilterSelectsDerivedFixtureOnly(t *testing.T) {
	root := newPlaytestFixtureProject(t)
	got, err := discoverPlaytests(root, playtestTestsDir(root), []string{"chain"})
	if err != nil {
		t.Fatalf("discoverPlaytests returned error: %v", err)
	}
	want := []string{
		filepath.Join(root, filepath.FromSlash("tests/playtest/playtest_chain.gd")),
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("filtered discoverPlaytests() = %v, want %v", got, want)
	}
}

func TestPlaytestDiscoverySuiteFilterRejectsNonPlaytestName(t *testing.T) {
	root := newPlaytestFixtureProject(t)
	got, err := discoverPlaytests(root, playtestTestsDir(root), []string{"nonplaytest"})
	if err != nil {
		t.Fatalf("discoverPlaytests returned error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected no selection for a non-playtest suite name, got %v", got)
	}
}

func TestRunPlaytestNoSelectionFailsBeforeEngine(t *testing.T) {
	cases := []struct {
		name  string
		suite []string
		files map[string]string
	}{
		{
			name:  "unfiltered empty suite directory",
			files: map[string]string{},
		},
		{
			name: "unfiltered existing non-playtest script",
			files: map[string]string{
				"tests/playtest/playtest_unsupported.gd": "extends Node\n",
			},
		},
		{
			name:  "explicit filter names existing non-playtest script",
			suite: []string{"unsupported"},
			files: map[string]string{
				"tests/playtest/playtest_unsupported.gd": "extends Node\n",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			if err := os.MkdirAll(playtestTestsDir(root), 0o755); err != nil {
				t.Fatalf("mkdir playtest dir: %v", err)
			}
			for rel, body := range tc.files {
				writeTestScript(t, root, rel, body)
			}
			stateRoot := t.TempDir()
			t.Setenv("GDBG_STATE", stateRoot)

			results, err := RunPlaytest(filepath.Join(root, "missing-godot"), root, RunOptions{Suite: tc.suite})
			if err == nil {
				t.Fatal("expected a descriptive error when discovery selects no suites")
			}
			if results != nil {
				t.Fatalf("expected nil results, got %v", results)
			}
			if !strings.Contains(err.Error(), "no playtest suites") {
				t.Fatalf("error %q must describe the empty selection", err)
			}
			// The guard must run before any engine launch or output-directory
			// creation, so no playtest log directory may exist.
			if _, statErr := os.Stat(filepath.Join(stateRoot, "logs", "playtest")); !os.IsNotExist(statErr) {
				t.Fatalf("expected no output directory before engine launch, stat err = %v", statErr)
			}
		})
	}
}

func TestPlaytestSelectedSuccessSyntheticEngine(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("synthetic engine fixture is a POSIX shell script")
	}
	root := t.TempDir()
	writeTestScript(t, root, "tests/playtest/playtest_smoke.gd",
		"extends \"res://addons/gdbg/testing/automation_playtest_suite.gd\"\n")
	fakeGodot := writeSyntheticGodot(t)
	stateRoot := t.TempDir()
	t.Setenv("GDBG_STATE", stateRoot)

	results, err := RunPlaytest(fakeGodot, root, RunOptions{Headless: true, Suite: []string{"smoke"}})
	if err != nil {
		t.Fatalf("RunPlaytest returned error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected one selected result, got %d", len(results))
	}
	if !results[0].Passed {
		t.Fatalf("expected synthetic playtest to pass, got %+v", results[0])
	}
	if results[0].Name != "smoke" {
		t.Fatalf("result name = %q, want smoke", results[0].Name)
	}
	if !strings.Contains(results[0].ReportText, "PASSED") {
		t.Fatalf("report text = %q, want synthetic PASSED report", results[0].ReportText)
	}
}

// writeSyntheticGodot writes a tiny shell executable that records the
// --playtest-output-dir value and produces a passing report. It never invokes
// the real engine, imports assets, or builds anything.
func writeSyntheticGodot(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "synthetic-godot")
	script := "#!/bin/sh\n" +
		"out=\"\"\n" +
		"for arg in \"$@\"; do\n" +
		"  case \"$arg\" in\n" +
		"    --playtest-output-dir=*) out=\"${arg#--playtest-output-dir=}\" ;;\n" +
		"  esac\n" +
		"done\n" +
		"mkdir -p \"$out\"\n" +
		"printf 'Status: PASSED\\n' > \"$out/report.txt\"\n" +
		"exit 0\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write synthetic godot: %v", err)
	}
	return path
}
