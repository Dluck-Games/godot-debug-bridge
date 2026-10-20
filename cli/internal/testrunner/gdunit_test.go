package testrunner

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildGdUnitArgsDoesNotEnableGodotVerboseForCliVerbose(t *testing.T) {
	args := buildGdUnitArgs(t.TempDir(), RunOptions{
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

func TestFinalizeGdUnitResultFlagsMissingReport(t *testing.T) {
	dir := t.TempDir()
	raw := []byte("Godot Engine v4.x\nno JUnit report produced\n")

	result, err := finalizeGdUnitResult(dir, 0, raw)
	if err != nil {
		t.Fatalf("finalizeGdUnitResult() error = %v", err)
	}
	if result.Parsed {
		t.Fatal("absent results.xml must not be marked parsed")
	}
	if result.Valid() {
		t.Fatal("absent results.xml must not be a valid result")
	}
	if result.ValidationReason == "" {
		t.Fatal("absent results.xml must record a validation reason")
	}
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0", result.ExitCode)
	}
	assertRawLogPersisted(t, result.RawLogPath, raw)
}

func TestFinalizeGdUnitResultFlagsMalformedXML(t *testing.T) {
	dir := t.TempDir()
	writeGdUnitTestFile(t, filepath.Join(dir, "results.xml"), "<testsuites><testsuite")
	raw := []byte("partially written report")

	result, err := finalizeGdUnitResult(dir, 0, raw)
	if err != nil {
		t.Fatalf("finalizeGdUnitResult() error = %v", err)
	}
	if result.Parsed {
		t.Fatal("malformed XML must not be marked parsed")
	}
	if result.Valid() {
		t.Fatal("malformed XML must not be a valid result")
	}
	if !strings.Contains(result.ValidationReason, "parse") {
		t.Fatalf("ValidationReason = %q, want it to mention parsing", result.ValidationReason)
	}
	assertRawLogPersisted(t, result.RawLogPath, raw)
}

func TestFinalizeGdUnitResultFlagsZeroTests(t *testing.T) {
	dir := t.TempDir()
	writeGdUnitTestFile(t, filepath.Join(dir, "results.xml"),
		`<?xml version="1.0" encoding="UTF-8"?><testsuites tests="0" failures="0" errors="0" skipped="0" time="0.0"></testsuites>`)
	raw := []byte("engine ran but discovered no tests")

	result, err := finalizeGdUnitResult(dir, 0, raw)
	if err != nil {
		t.Fatalf("finalizeGdUnitResult() error = %v", err)
	}
	if !result.Parsed {
		t.Fatal("well-formed zero-test XML must still be marked parsed")
	}
	if result.Valid() {
		t.Fatal("zero-test report must not be a valid result")
	}
	if !strings.Contains(result.ValidationReason, "zero") {
		t.Fatalf("ValidationReason = %q, want it to mention zero tests", result.ValidationReason)
	}
	assertRawLogPersisted(t, result.RawLogPath, raw)
}

func TestFinalizeGdUnitResultKeepsValidJUnit(t *testing.T) {
	dir := t.TempDir()
	writeGdUnitTestFile(t, filepath.Join(dir, "results.xml"),
		`<?xml version="1.0" encoding="UTF-8"?>
<testsuites tests="2" failures="0" errors="0" skipped="0" time="0.02">
  <testsuite name="res://tests/unit/example/test_example.gd" tests="2" failures="0" errors="0" skipped="0" time="0.02">
    <testcase name="test_alpha" classname="Example" time="0.01"/>
    <testcase name="test_beta" classname="Example" time="0.01"/>
  </testsuite>
</testsuites>`)
	raw := []byte("2 tests passed")

	result, err := finalizeGdUnitResult(dir, 0, raw)
	if err != nil {
		t.Fatalf("finalizeGdUnitResult() error = %v", err)
	}
	if !result.Parsed || !result.Valid() {
		t.Fatalf("valid JUnit report must be parsed and valid, got parsed=%v valid=%v reason=%q",
			result.Parsed, result.Valid(), result.ValidationReason)
	}
	if result.ValidationReason != "" {
		t.Fatalf("valid JUnit report must not record a validation reason, got %q", result.ValidationReason)
	}
	if result.Suites.Tests != 2 {
		t.Fatalf("Suites.Tests = %d, want 2", result.Suites.Tests)
	}
	if len(result.Suites.TestSuites) != 1 || len(result.Suites.TestSuites[0].Cases) != 2 {
		t.Fatalf("unexpected suite/case parse: %+v", result.Suites)
	}
	if got := result.Suites.TestSuites[0].Cases[0].Name; got != "test_alpha" {
		t.Fatalf("first case name = %q, want test_alpha", got)
	}
	assertRawLogPersisted(t, result.RawLogPath, raw)
}

func TestFinalizeGdUnitResultRetainsNonzeroEngineExit(t *testing.T) {
	dir := t.TempDir()
	writeGdUnitTestFile(t, filepath.Join(dir, "results.xml"),
		`<?xml version="1.0" encoding="UTF-8"?><testsuites tests="1" failures="0" errors="0" skipped="0" time="0.01"><testsuite name="s" tests="1" failures="0" errors="0" skipped="0" time="0.01"><testcase name="test_ok" classname="S" time="0.01"/></testsuite></testsuites>`)
	raw := []byte("engine exited nonzero after tests")

	result, err := finalizeGdUnitResult(dir, 137, raw)
	if err != nil {
		t.Fatalf("finalizeGdUnitResult() error = %v", err)
	}
	if result.ExitCode != 137 {
		t.Fatalf("ExitCode = %d, want 137 (actual engine exit must be retained)", result.ExitCode)
	}
	if !result.Valid() {
		t.Fatalf("valid parsed JUnit must remain valid even when engine exit is nonzero, reason=%q", result.ValidationReason)
	}
	assertRawLogPersisted(t, result.RawLogPath, raw)
}

func writeGdUnitTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func assertRawLogPersisted(t *testing.T, path string, want []byte) {
	t.Helper()
	if path == "" {
		t.Fatal("raw log path must be set")
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read raw log %s: %v", path, err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("raw log not byte-for-byte identical:\n got %q\nwant %q", got, want)
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

func TestFinalizeGdUnitResultRecordsScriptLoadFailures(t *testing.T) {
	dir := t.TempDir()
	writeGdUnitTestFile(t, filepath.Join(dir, "results.xml"),
		`<testsuites tests="1" failures="0" errors="0" skipped="0"><testsuite name="test_good" tests="1"/></testsuites>`)
	raw := []byte(strings.Join([]string{
		"Scanning for test suites in: res://tests/unit/",
		`SCRIPT ERROR: Parse Error: Identifier "nope" not declared in the current scope.`,
		"          at: GDScript::reload (res://tests/unit/broken/test_broken.gd:5)",
		"          GDScript backtrace (most recent call first):",
		`ERROR: Failed to load script "res://tests/unit/broken/test_broken.gd" with error "Parse error".`,
		"   at: load (modules/gdscript/gdscript_resource_format.cpp:46)",
		`ERROR: Failed to load script "res://tests/unit/broken/test_broken.gd" with error "Parse error".`,
		`ERROR: Failed to load script "res://scripts/helper.gd" with error "Parse error".`,
		"SCRIPT ERROR: Compile Error: Failed to compile depended scripts.",
		"          at: GDScript::reload (res://tests/unit/dep/test_dep.gd:0)",
		`ERROR: Failed to load script "res://tests/unit/dep/test_dep.gd" with error "Compilation failed".`,
		"",
	}, "\n"))

	result, err := finalizeGdUnitResult(dir, 0, raw)
	if err != nil {
		t.Fatalf("finalizeGdUnitResult() error = %v", err)
	}
	if !result.Valid() {
		t.Fatal("a parsed report with tests must stay valid; load failures are reported separately")
	}
	if len(result.LoadFailures) != 2 {
		t.Fatalf("LoadFailures = %+v, want the two unit scripts once each", result.LoadFailures)
	}
	broken := result.LoadFailures[0]
	if broken.Path != "res://tests/unit/broken/test_broken.gd" || broken.Reason != "Parse error" {
		t.Fatalf("first load failure = %+v", broken)
	}
	if len(broken.Errors) != 1 || !strings.Contains(broken.Errors[0], `Identifier "nope"`) || !strings.HasSuffix(broken.Errors[0], "(line 5)") {
		t.Fatalf("broken.Errors = %q, want the attributed parse error with its line", broken.Errors)
	}
	dep := result.LoadFailures[1]
	if dep.Path != "res://tests/unit/dep/test_dep.gd" || dep.Reason != "Compilation failed" || len(dep.Errors) != 1 {
		t.Fatalf("second load failure = %+v", dep)
	}

	report := &TestReport{GdUnit: result}
	if report.AllPassed() {
		t.Fatal("unloadable unit scripts must fail the run")
	}
	if got := report.TotalFailed(); got != 2 {
		t.Fatalf("TotalFailed() = %d, want 2", got)
	}
	if got := report.TotalTests(); got != 3 {
		t.Fatalf("TotalTests() = %d, want 3", got)
	}
	var out bytes.Buffer
	RenderASCII(&out, report, false)
	for _, want := range []string{"failed to load", "res://tests/unit/broken/test_broken.gd (Parse error)", `Identifier "nope"`, "SOME TESTS FAILED"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("report output missing %q:\n%s", want, out.String())
		}
	}
}

func TestBuildGdUnitAddArgsRootSuiteAddsTopLevelFiles(t *testing.T) {
	projectDir := t.TempDir()
	for _, rel := range []string{"tests/unit/test_beta.gd", "tests/unit/test_alpha.gd", "tests/unit/system/test_nested.gd", "tests/unit/notes.txt"} {
		full := filepath.Join(projectDir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		writeGdUnitTestFile(t, full, "extends GdUnitTestSuite")
	}

	got := buildGdUnitAddArgs(projectDir, []string{RootSuite, "system"})
	want := []string{"--add", UnitTestsDirRes + "test_alpha.gd", "--add", UnitTestsDirRes + "test_beta.gd", "--add", UnitTestsDirRes + "system/"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("root suite args = %v, want %v", got, want)
	}
	if all := buildGdUnitAddArgs(projectDir, nil); strings.Join(all, " ") != "--add "+UnitTestsDirRes {
		t.Fatalf("--suite all must add the whole unit directory, got %v", all)
	}
	if !UnitSuiteExists(projectDir, RootSuite) || !UnitSuiteExists(projectDir, "system") || UnitSuiteExists(projectDir, "missing") {
		t.Fatal("suite existence must name root and first-level directories only")
	}
	if UnitSuiteExists(t.TempDir(), RootSuite) {
		t.Fatal("root must not exist without top-level unit test files")
	}
}

func TestVerdictSumsSuiteErrorsOmittedFromRoot(t *testing.T) {
	dir := t.TempDir()
	// gdUnit4 writes no `errors` attribute on <testsuites>; only the suites carry it.
	writeGdUnitTestFile(t, filepath.Join(dir, "results.xml"),
		`<?xml version="1.0" encoding="UTF-8"?>
<testsuites tests="3" failures="1" skipped="0" time="0.02">
  <testsuite name="test_drop" tests="2" failures="0" errors="1" skipped="0" time="0.01">
    <testcase name="test_ok" classname="test_drop" time="0.01"/>
    <testcase name="test_boom" classname="test_drop" time="0.00">
      <error message="ERROR: res://tests/unit/test_drop.gd:9" type="ABORT"><![CDATA[
Godot Runtime Error !
Error: 'Trying to return a previously freed instance.'
]]></error>
    </testcase>
  </testsuite>
  <testsuite name="test_math" tests="1" failures="1" errors="0" skipped="0" time="0.01">
    <testcase name="test_sum" classname="test_math" time="0.01">
      <failure message="FAILED: res://tests/unit/math/test_math.gd:12" type="FAILURE">Expecting: '3' but was '4'</failure>
    </testcase>
  </testsuite>
</testsuites>`)

	result, err := finalizeGdUnitResult(dir, gdUnitExitFailures, []byte("Exit code: 100\n"))
	if err != nil {
		t.Fatalf("finalizeGdUnitResult() error = %v", err)
	}
	v := result.Verdict()
	if v.Passed() {
		t.Fatal("errored cases must fail the verdict even though the root omits errors")
	}
	if v.Tests != 3 || v.Failures != 1 || v.Errors != 1 {
		t.Fatalf("counts = %+v, want tests 3 failures 1 errors 1", v.GdUnitCounts)
	}
	if len(v.Reasons) != 1 || !strings.Contains(v.Reasons[0], "1 failed and 1 errored") {
		t.Fatalf("Reasons = %q, want exactly the case-count reason (exit 100 agrees with the report)", v.Reasons)
	}
	report := &TestReport{GdUnit: result}
	if report.TotalFailed() != 2 || report.TotalPassed() != 1 {
		t.Fatalf("TotalFailed/TotalPassed = %d/%d, want 2/1", report.TotalFailed(), report.TotalPassed())
	}
	for _, verbose := range []bool{false, true} {
		var out bytes.Buffer
		RenderASCII(&out, report, verbose)
		for _, want := range []string{
			"Unit tier failed:",
			"x test_drop.test_boom",
			"ERROR: res://tests/unit/test_drop.gd:9",
			"Trying to return a previously freed instance",
			"x test_math.test_sum",
			"Expecting: '3' but was '4'",
			result.RawLogPath,
			"SOME TESTS FAILED",
		} {
			if !strings.Contains(out.String(), want) {
				t.Fatalf("verbose=%v output missing %q:\n%s", verbose, want, out.String())
			}
		}
	}
}

func TestVerdictFailsWhenExitCodeDisagreesWithReport(t *testing.T) {
	clean := JUnitTestSuites{Tests: 1, TestSuites: []JUnitTestSuite{{Name: "s", Tests: 1, Cases: []JUnitTestCase{{Name: "test_ok"}}}}}

	disagree := &GdUnitResult{Parsed: true, ExitCode: gdUnitExitFailures, Suites: clean}
	v := disagree.Verdict()
	if v.Passed() || len(v.Reasons) != 1 || !strings.Contains(v.Reasons[0], "disagree") {
		t.Fatalf("exit 100 with a clean report must fail with the disagreement reason, got %q", v.Reasons)
	}
	var out bytes.Buffer
	RenderASCII(&out, &TestReport{GdUnit: disagree}, false)
	if !strings.Contains(out.String(), "disagree") || !strings.Contains(out.String(), "engine exit code: 100") {
		t.Fatalf("report must say the exit code and report disagree:\n%s", out.String())
	}

	crashed := &GdUnitResult{Parsed: true, ExitCode: 139, Suites: clean}
	if v := crashed.Verdict(); v.Passed() || !strings.Contains(v.Reasons[0], "exited with code 139") {
		t.Fatalf("an abnormal engine exit must fail, got %q", v.Reasons)
	}
	warned := &GdUnitResult{Parsed: true, ExitCode: gdUnitExitWarning, Suites: clean}
	if !warned.Verdict().Passed() {
		t.Fatal("gdUnit4 exit 101 (orphan warning) must not fail a clean report")
	}
}

func TestVerdictHintsStaleClassCache(t *testing.T) {
	raw := strings.Join([]string{
		`SCRIPT ERROR: Parse Error: Identifier "WorldCommands" not declared in the current scope.`,
		"          at: GDScript::reload (res://tests/unit/world/test_commands.gd:12)",
		"",
	}, "\n")
	hinted := &GdUnitResult{ValidationReason: "results.xml reported zero tests", RawOutput: raw}
	if !hinted.Verdict().StaleClassCacheHint {
		t.Fatal("unresolved identifier in a res://tests/ script must raise the stale class cache hint")
	}
	var out bytes.Buffer
	RenderASCII(&out, &TestReport{GdUnit: hinted}, false)
	if !strings.Contains(out.String(), "gdbg reimport") {
		t.Fatalf("hint line missing:\n%s", out.String())
	}
	elsewhere := &GdUnitResult{RawOutput: strings.ReplaceAll(raw, "res://tests/", "res://scripts/")}
	if elsewhere.Verdict().StaleClassCacheHint {
		t.Fatal("errors outside res://tests/ must not raise the hint")
	}
}
