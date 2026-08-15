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
}

func RunGdUnit(godotBin, projectDir string, opts RunOptions) (*GdUnitResult, error) {
	// Clean reports
	reportsDir := filepath.Join(logStateBase(), "gdunit")
	os.RemoveAll(reportsDir)
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

	err = cmd.Run()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			return nil, fmt.Errorf("failed to run gdUnit4: %w", err)
		}
	}

	result := &GdUnitResult{ExitCode: exitCode, RawOutput: outputBuf.String()}

	// Find and parse results.xml
	xmlPath := findResultsXML(reportsDir)
	if xmlPath != "" {
		suites, err := parseJUnitXML(xmlPath)
		if err == nil {
			result.Suites = *suites
			result.Parsed = true
		}
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
