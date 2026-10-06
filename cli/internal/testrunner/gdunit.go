// godot-debug-bridge/cli/internal/testrunner/gdunit.go
package testrunner

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

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
}

// Valid reports whether the gdUnit result is a trustworthy parsed JUnit report.
// A missing/malformed report or a parsed report that declares zero tests is
// never valid, regardless of the engine exit code.
func (r *GdUnitResult) Valid() bool {
	return r != nil && r.Parsed && r.ValidationReason == "" && r.Suites.Tests > 0
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

	args := buildGdUnitArgs(opts, reportsArg)
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
		ExitCode:   exitCode,
		RawOutput:  string(raw),
		RawLogPath: rawLogPath,
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
	if suites.Tests <= 0 {
		result.ValidationReason = "results.xml reported zero tests"
	}

	return result, nil
}

func logStateBase() string {
	return filepath.Join(debug.StateBase(), "logs")
}

func buildGdUnitArgs(opts RunOptions, reportsArg string) []string {
	args := []string{
		"--headless",
		"-s", GdUnit4CmdToolRes,
	}
	args = append(args, buildGdUnitAddArgs(opts.Suite)...)
	args = append(args, "--ignoreHeadlessMode", "--report-directory", reportsArg)
	// GDBG CLI verbose controls our parsed report only. Do not pass --verbose to
	// Godot/gdUnit here; it enables engine exit diagnostics such as StringName
	// orphan reports even when tests themselves are clean.
	return args
}

// buildGdUnitAddArgs constructs the --add arguments for gdunit4 based on suite filter.
// If no suites specified, adds the entire unit test directory.
// Otherwise, adds each suite subdirectory individually.
func buildGdUnitAddArgs(suites []string) []string {
	if len(suites) == 0 {
		return []string{"--add", UnitTestsDirRes}
	}
	var args []string
	for _, s := range suites {
		args = append(args, "--add", UnitTestsDirRes+s+"/")
	}
	return args
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
