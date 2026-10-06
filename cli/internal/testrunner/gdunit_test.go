package testrunner

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
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
