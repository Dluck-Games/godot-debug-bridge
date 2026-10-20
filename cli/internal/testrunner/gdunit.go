// godot-debug-bridge/cli/internal/testrunner/gdunit.go
package testrunner

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Dluck-Games/godot-debug-bridge/cli/internal/debug"
)

type JUnitTestSuites struct {
	XMLName    xml.Name         `xml:"testsuites"`
	Tests      int              `xml:"tests,attr"`
	Failures   int              `xml:"failures,attr"`
	Errors     int              `xml:"errors,attr"`
	Skipped    int              `xml:"skipped,attr"`
	Time       float64          `xml:"time,attr"`
	TestSuites []JUnitTestSuite `xml:"testsuite"`
}

type JUnitTestSuite struct {
	Name     string          `xml:"name,attr"`
	Tests    int             `xml:"tests,attr"`
	Failures int             `xml:"failures,attr"`
	Errors   int             `xml:"errors,attr"`
	Skipped  int             `xml:"skipped,attr"`
	Time     float64         `xml:"time,attr"`
	Cases    []JUnitTestCase `xml:"testcase"`
}

type JUnitTestCase struct {
	Name      string        `xml:"name,attr"`
	ClassName string        `xml:"classname,attr"`
	Time      float64       `xml:"time,attr"`
	Failure   *JUnitFailure `xml:"failure"`
	Error     *JUnitError   `xml:"error"`
}

type JUnitFailure struct {
	Message string `xml:"message,attr"`
	Type    string `xml:"type,attr"`
	Body    string `xml:",chardata"`
}

type JUnitError struct {
	Message string `xml:"message,attr"`
	Type    string `xml:"type,attr"`
	Body    string `xml:",chardata"`
}

type GdUnitResult struct {
	Suites    JUnitTestSuites
	ExitCode  int
	Parsed    bool
	RawOutput string // captured gdUnit4 stdout+stderr for failure diagnostics

	// RawLogPath is the persisted location of the complete, unfiltered child
	// stdout+stderr capture written under the per-state gdunit report dir.
	RawLogPath string

	// ValidationReason is non-empty when the result is not trustworthy (missing
	// results.xml, malformed/unreadable XML, or a parsed report with zero
	// tests). It carries the explicit reason the unit tier cannot be trusted.
	ValidationReason string

	// LoadFailures lists unit scripts Godot could not parse or compile. gdUnit4
	// skips such scripts silently and its JUnit report never mentions them, so
	// they are recovered from the engine output and each counts as a failure.
	LoadFailures []GdUnitLoadFailure
}

// GdUnitLoadFailure is one unit script that failed to load before any of its
// tests could run.
type GdUnitLoadFailure struct {
	Path   string   // res:// path of the script
	Reason string   // Godot's load error, e.g. "Parse error"
	Errors []string // SCRIPT ERROR messages Godot attributed to Path
}

// Valid reports whether the gdUnit result is a trustworthy parsed JUnit report.
// A missing/malformed report or a parsed report that declares zero tests is
// never valid, regardless of the engine exit code.
func (r *GdUnitResult) Valid() bool {
	return r != nil && r.Parsed && r.ValidationReason == "" && junitCounts(&r.Suites).Tests > 0
}

// GdUnitCaseFailure is one failed or errored test case together with the
// message gdUnit4 wrote for it, so the reader knows where to fix.
type GdUnitCaseFailure struct {
	Suite   string
	Case    string
	Kind    string // "failure" (assertion) or "error" (runtime error)
	Message string // <failure message> / <error message>
	Body    string // element text, e.g. the assertion detail or the Godot error
}

// GdUnitCounts are the JUnit totals aggregated from every <testsuite> element
// and its <testcase> children. gdUnit4 writes `errors` only on <testsuite>,
// never on the root <testsuites>, so every count takes the largest of the
// root attribute, the summed suite attributes and the counted case markers;
// no single omission can hide a failure.
type GdUnitCounts struct {
	Tests, Failures, Errors, Skipped int
	Cases                            []GdUnitCaseFailure
}

func junitCounts(s *JUnitTestSuites) GdUnitCounts {
	var sumTests, sumFailures, sumErrors, sumSkipped, caseFailures, caseErrors int
	var cases []GdUnitCaseFailure
	for _, suite := range s.TestSuites {
		sumTests += suite.Tests
		sumFailures += suite.Failures
		sumErrors += suite.Errors
		sumSkipped += suite.Skipped
		for _, tc := range suite.Cases {
			switch {
			case tc.Error != nil:
				caseErrors++
				cases = append(cases, GdUnitCaseFailure{suite.Name, tc.Name, "error", tc.Error.Message, strings.TrimSpace(tc.Error.Body)})
			case tc.Failure != nil:
				caseFailures++
				cases = append(cases, GdUnitCaseFailure{suite.Name, tc.Name, "failure", tc.Failure.Message, strings.TrimSpace(tc.Failure.Body)})
			}
		}
	}
	return GdUnitCounts{
		Tests:    max(s.Tests, sumTests),
		Failures: max(s.Failures, sumFailures, caseFailures),
		Errors:   max(s.Errors, sumErrors, caseErrors),
		Skipped:  max(s.Skipped, sumSkipped),
		Cases:    cases,
	}
}

// gdUnit4 exit codes (GdUnitTestSessionRunner.gd RETURN_* constants). 100 is
// the only code that reports test failures; 101 warns about orphan nodes and
// is not a failure; 103/104 are engine-side refusals to run at all.
const (
	gdUnitExitSuccess             = 0
	gdUnitExitFailures            = 100
	gdUnitExitWarning             = 101
	gdUnitExitHeadlessUnsupported = 103
	gdUnitExitGodotUnsupported    = 104
)

// GdUnitVerdict is the single pass/fail decision for the unit tier. Every
// failure mode (untrustworthy report, unloadable scripts, failed or errored
// cases, engine exit code) contributes one reason; an empty Reasons is a pass.
type GdUnitVerdict struct {
	GdUnitCounts
	Reasons []string
	// StaleClassCacheHint is set when scripts under res://tests/ hit unresolved
	// identifiers or types, which a stale class cache typically causes.
	StaleClassCacheHint bool
}

func (v GdUnitVerdict) Passed() bool { return len(v.Reasons) == 0 }

// Verdict decides pass/fail for the unit tier from the parsed report, the
// recovered load failures and the gdUnit4 exit code.
func (r *GdUnitResult) Verdict() GdUnitVerdict {
	v := GdUnitVerdict{GdUnitCounts: junitCounts(&r.Suites)}
	if !r.Valid() {
		reason := r.ValidationReason
		if reason == "" {
			reason = "no trustworthy JUnit report was produced"
		}
		v.Reasons = append(v.Reasons, "gdUnit result invalid: "+reason)
	}
	if n := len(r.LoadFailures); n > 0 {
		v.Reasons = append(v.Reasons, fmt.Sprintf("%d unit script(s) failed to load (parse or compile error)", n))
	}
	if v.Failures+v.Errors > 0 {
		v.Reasons = append(v.Reasons, fmt.Sprintf("%d failed and %d errored test case(s)", v.Failures, v.Errors))
	}
	if reason := gdUnitExitReason(r.ExitCode, v.Failures+v.Errors > 0); reason != "" {
		v.Reasons = append(v.Reasons, reason)
	}
	v.StaleClassCacheHint = hasStaleClassCacheErrors(r.RawOutput)
	return v
}

// gdUnitExitReason explains a gdUnit4 exit code that the parsed report does
// not already account for. A failure code with zero counted failures means the
// report and the exit code disagree, which is itself a failure.
func gdUnitExitReason(code int, countedFailures bool) string {
	switch code {
	case gdUnitExitSuccess, gdUnitExitWarning:
		return ""
	case gdUnitExitFailures:
		if countedFailures {
			return ""
		}
		return "gdUnit4 exited with code 100 (test failures or errors) but results.xml counts none: the report and the exit code disagree"
	case gdUnitExitHeadlessUnsupported:
		return "gdUnit4 exited with code 103: headless mode is not supported by this gdUnit4 version"
	case gdUnitExitGodotUnsupported:
		return "gdUnit4 exited with code 104: this Godot version is not supported by gdUnit4"
	default:
		return fmt.Sprintf("the Godot engine exited with code %d before gdUnit4 could finish", code)
	}
}

// rawOutputLogName is the generic file name used to persist the complete
// gdUnit4 child output for evidence, independent of any test tier naming.
const rawOutputLogName = "raw-output.log"

func RunGdUnit(godotBin, projectDir string, opts RunOptions) (*GdUnitResult, error) {
	// Recreate the report directory from scratch. Stale artefacts from a prior
	// run must never be mistaken for this run's evidence, and our own cleanup /
	// create failures must surface instead of being silently ignored.
	reportsDir := filepath.Join(logStateBase(), "gdunit")
	if err := os.RemoveAll(reportsDir); err != nil {
		return nil, fmt.Errorf("cannot clear gdUnit report directory %s: %w", reportsDir, err)
	}
	if err := os.MkdirAll(reportsDir, 0o755); err != nil {
		return nil, fmt.Errorf("cannot create gdUnit report directory %s: %w", reportsDir, err)
	}
	reportsArg, err := filepath.Rel(projectDir, reportsDir)
	if err != nil {
		return nil, fmt.Errorf("cannot resolve gdUnit report directory: %w", err)
	}
	reportsArg = filepath.ToSlash(reportsArg)

	args := buildGdUnitArgs(projectDir, opts, reportsArg)
	cmd := exec.Command(godotBin, args...)
	cmd.Dir = projectDir

	var outputBuf bytes.Buffer
	cmd.Stdout = &outputBuf
	cmd.Stderr = &outputBuf

	runErr := cmd.Run()
	exitCode := 0
	if runErr != nil {
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			return nil, fmt.Errorf("failed to run gdUnit4: %w", runErr)
		}
	}

	return finalizeGdUnitResult(reportsDir, exitCode, outputBuf.Bytes())
}

// finalizeGdUnitResult captures the child's exit code and complete output,
// persists the raw capture, then reads and validates the JUnit report. It
// performs no process execution so every post-execution path can be exercised
// deterministically in tests with a temp directory.
func finalizeGdUnitResult(reportsDir string, exitCode int, raw []byte) (*GdUnitResult, error) {
	// Persist the complete, unfiltered child output next to the reports so an
	// invalid or partial run still leaves diagnosable evidence on disk.
	rawLogPath := filepath.Join(reportsDir, rawOutputLogName)
	if err := os.WriteFile(rawLogPath, raw, 0o644); err != nil {
		return nil, fmt.Errorf("cannot write gdUnit raw output log %s: %w", rawLogPath, err)
	}

	result := &GdUnitResult{
		ExitCode:     exitCode,
		RawOutput:    string(raw),
		RawLogPath:   rawLogPath,
		LoadFailures: parseScriptLoadFailures(string(raw)),
	}

	// Find and parse results.xml. Missing or malformed reports are explicit
	// failures, never a silent pass.
	xmlPath := findResultsXML(reportsDir)
	if xmlPath == "" {
		result.ValidationReason = "results.xml not found after run"
		return result, nil
	}

	suites, err := parseJUnitXML(xmlPath)
	if err != nil {
		result.ValidationReason = fmt.Sprintf("results.xml could not be parsed: %v", err)
		return result, nil
	}

	result.Suites = *suites
	result.Parsed = true
	if junitCounts(suites).Tests <= 0 {
		result.ValidationReason = "results.xml reported zero tests"
	}

	return result, nil
}

func logStateBase() string {
	return filepath.Join(debug.StateBase(), "logs")
}

var (
	scriptLoadFailureRe = regexp.MustCompile(`Failed to load script "(res://[^"]+)" with error "([^"]*)"`)
	scriptErrorAtRe     = regexp.MustCompile(`^\s*at: \S+ \((res://[^:)]+):(\d+)\)`)
)

// parseScriptLoadFailures recovers unit scripts under UnitTestsDirRes that
// Godot reported as "Failed to load script" while gdUnit4 scanned them. The
// SCRIPT ERROR lines Godot prints just before are attached by their "at:"
// location so the report names the parse/compile error, not only the file.
func parseScriptLoadFailures(raw string) []GdUnitLoadFailure {
	lines := strings.Split(raw, "\n")
	errorsByPath := map[string][]string{}
	seen := map[string]bool{}
	var failures []GdUnitLoadFailure
	for i, line := range lines {
		msg := strings.TrimSpace(line)
		if strings.HasPrefix(msg, "SCRIPT ERROR: ") && i+1 < len(lines) {
			if m := scriptErrorAtRe.FindStringSubmatch(lines[i+1]); m != nil {
				errorsByPath[m[1]] = append(errorsByPath[m[1]],
					fmt.Sprintf("%s (line %s)", strings.TrimPrefix(msg, "SCRIPT ERROR: "), m[2]))
			}
			continue
		}
		m := scriptLoadFailureRe.FindStringSubmatch(line)
		if m == nil || !strings.HasPrefix(m[1], UnitTestsDirRes) || seen[m[1]] {
			continue
		}
		seen[m[1]] = true
		failures = append(failures, GdUnitLoadFailure{Path: m[1], Reason: m[2], Errors: errorsByPath[m[1]]})
	}
	return failures
}

// staleClassCacheMarkers are the Godot parse errors raised when a class_name
// is missing from the class cache, typically after adding or renaming scripts
// without an import.
var staleClassCacheMarkers = []string{"not declared in the current scope", "Could not find type"}

// hasStaleClassCacheErrors reports whether the engine output attributes an
// unresolved identifier or type to a script under res://tests/.
func hasStaleClassCacheErrors(raw string) bool {
	lines := strings.Split(raw, "\n")
	for i := 0; i+1 < len(lines); i++ {
		marked := false
		for _, marker := range staleClassCacheMarkers {
			if strings.Contains(lines[i], marker) {
				marked = true
				break
			}
		}
		if !marked {
			continue
		}
		if m := scriptErrorAtRe.FindStringSubmatch(lines[i+1]); m != nil && strings.HasPrefix(m[1], TestsDirRes) {
			return true
		}
	}
	return false
}

func buildGdUnitArgs(projectDir string, opts RunOptions, reportsArg string) []string {
	args := []string{
		"--headless",
		"-s", GdUnit4CmdToolRes,
	}
	args = append(args, buildGdUnitAddArgs(projectDir, opts.Suite)...)
	args = append(args, "--ignoreHeadlessMode", "--report-directory", reportsArg)
	// GDBG CLI verbose controls our parsed report only. Do not pass --verbose to
	// Godot/gdUnit here; it enables engine exit diagnostics such as StringName
	// orphan reports even when tests themselves are clean.
	return args
}

// buildGdUnitAddArgs constructs the --add arguments for gdUnit4 from the suite
// filter. No filter adds the whole unit directory, exactly as a raw gdUnit4
// run does. A named suite adds its first-level directory; the root suite adds
// each script directly under tests/unit/ (gdUnit4 --add accepts files too).
func buildGdUnitAddArgs(projectDir string, suites []string) []string {
	if len(suites) == 0 {
		return []string{"--add", UnitTestsDirRes}
	}
	var args []string
	for _, s := range suites {
		if s == RootSuite {
			for _, f := range rootUnitSuiteFiles(projectDir) {
				args = append(args, "--add", UnitTestsDirRes+f)
			}
			continue
		}
		args = append(args, "--add", UnitTestsDirRes+s+"/")
	}
	return args
}

// rootUnitSuiteFiles lists the scripts directly under tests/unit/, in name
// order. They form the root suite.
func rootUnitSuiteFiles(projectDir string) []string {
	entries, err := os.ReadDir(unitTestsDir(projectDir))
	if err != nil {
		return nil
	}
	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".gd") {
			files = append(files, e.Name())
		}
	}
	return files
}

// UnitSuiteExists reports whether suite names a first-level directory under
// tests/unit/, or the root suite when scripts exist directly under it.
func UnitSuiteExists(projectDir, suite string) bool {
	if suite == RootSuite {
		return len(rootUnitSuiteFiles(projectDir)) > 0
	}
	stat, err := os.Stat(filepath.Join(unitTestsDir(projectDir), suite))
	return err == nil && stat.IsDir()
}

func findResultsXML(reportsDir string) string {
	var found string
	filepath.WalkDir(reportsDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.Name() == "results.xml" && !containsSubstring(path, "unittest-") {
			found = path
		}
		return nil
	})
	return found
}

func containsSubstring(s, sub string) bool {
	return len(s) >= len(sub) && findSubstring(s, sub)
}

func findSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func parseJUnitXML(path string) (*JUnitTestSuites, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var suites JUnitTestSuites
	if err := xml.Unmarshal(data, &suites); err != nil {
		return nil, err
	}
	return &suites, nil
}
