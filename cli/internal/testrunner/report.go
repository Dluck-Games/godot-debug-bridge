// godot-debug-bridge/cli/internal/testrunner/report.go
package testrunner

import (
	"fmt"
	"io"
	"os"
	"strings"
)

type TestReport struct {
	GdUnit      *GdUnitResult
	Integration []IntegrationTestResult
	Playtest    []PlaytestResult
}

func (r *TestReport) TotalTests() int {
	total := 0
	if r.GdUnit != nil && r.GdUnit.Parsed {
		total += r.GdUnit.Suites.Tests
	}
	total += len(r.Integration)
	total += len(r.Playtest)
	return total
}

func (r *TestReport) TotalPassed() int {
	passed := 0
	if r.GdUnit != nil && r.GdUnit.Parsed {
		passed += r.GdUnit.Suites.Tests - r.GdUnit.Suites.Failures - r.GdUnit.Suites.Errors - r.GdUnit.Suites.Skipped
	}
	for _, sc := range r.Integration {
		if sc.Passed {
			passed++
		}
	}
	for _, pt := range r.Playtest {
		if pt.Passed {
			passed++
		}
	}
	return passed
}

func (r *TestReport) TotalFailed() int {
	failed := 0
	if r.GdUnit != nil && r.GdUnit.Parsed {
		failed += r.GdUnit.Suites.Failures + r.GdUnit.Suites.Errors
	}
	for _, sc := range r.Integration {
		if !sc.Passed {
			failed++
		}
	}
	for _, pt := range r.Playtest {
		if !pt.Passed {
			failed++
		}
	}
	return failed
}

func (r *TestReport) TotalSkipped() int {
	if r.GdUnit != nil && r.GdUnit.Parsed {
		return r.GdUnit.Suites.Skipped
	}
	return 0
}

func (r *TestReport) AllPassed() bool {
	if r.GdUnit != nil {
		if r.GdUnit.Parsed {
			if r.GdUnit.Suites.Failures+r.GdUnit.Suites.Errors > 0 {
				return false
			}
		} else if r.GdUnit.ExitCode != 0 {
			return false
		}
	}
	for _, sc := range r.Integration {
		if !sc.Passed {
			return false
		}
	}
	for _, pt := range r.Playtest {
		if !pt.Passed {
			return false
		}
	}
	return true
}

func (r *TestReport) ExitCode() int {
	if r.AllPassed() {
		return 0
	}
	for _, pt := range r.Playtest {
		if pt.ExitCode == 2 {
			return 2
		}
	}
	return 1
}

const bar = "================================================================"

func RenderASCII(w io.Writer, report *TestReport, verbose bool) {
	if IsSinglePlaytest(report) {
		RenderSinglePlaytest(w, report.Playtest[0])
		return
	}
	if verbose {
		renderVerbose(w, report)
	} else {
		renderSimplified(w, report)
	}
}

func IsSinglePlaytest(report *TestReport) bool {
	return report != nil && report.GdUnit == nil && len(report.Integration) == 0 && len(report.Playtest) == 1
}

func RenderSinglePlaytest(w io.Writer, pt PlaytestResult) {
	fmt.Fprintln(w)
	text := strings.TrimRight(pt.ReportText, "\n")
	if text != "" {
		fmt.Fprintln(w, text)
		fmt.Fprintln(w)
	}
	if pt.RecordingPath != "" {
		fmt.Fprintf(w, "recording: %s\n", pt.RecordingPath)
	}
}

// renderSimplified shows only failures + summary totals.
// If all tests pass, only the summary line is shown.
func renderSimplified(w io.Writer, report *TestReport) {
	fmt.Fprintln(w)
	fmt.Fprintln(w, bar)
	fmt.Fprintln(w, "                       GDBG TEST REPORT")
	fmt.Fprintln(w, bar)

	hasFailures := !report.AllPassed()

	if hasFailures {
		// Show failure details
		if report.GdUnit != nil && report.GdUnit.Parsed {
			var failures []string
			for _, suite := range report.GdUnit.Suites.TestSuites {
				for _, tc := range suite.Cases {
					if tc.Failure != nil || tc.Error != nil {
						failures = append(failures, fmt.Sprintf("  x %s.%s", suite.Name, tc.Name))
					}
				}
			}
			if len(failures) > 0 {
				fmt.Fprintln(w)
				fmt.Fprintln(w, "  Unit Failures:")
				for _, f := range failures {
					fmt.Fprintln(w, f)
				}
			}
		} else if report.GdUnit != nil && !report.GdUnit.Parsed {
			fmt.Fprintln(w)
			fmt.Fprintln(w, "  (no JUnit report found)")
			// Dump last 30 lines of raw output for diagnosis
			if report.GdUnit.RawOutput != "" {
				dumpRawOutputTail(report.GdUnit.RawOutput)
			}
		}

		// Integration failures
		var scFails []string
		for _, sc := range report.Integration {
			if !sc.Passed {
				scFails = append(scFails, sc.Name)
			}
		}
		if len(scFails) > 0 {
			fmt.Fprintln(w)
			fmt.Fprintln(w, "  Integration Failures:")
			for _, name := range scFails {
				fmt.Fprintf(w, "  x %s\n", name)
			}
		}

		var playtestFails []string
		for _, pt := range report.Playtest {
			if !pt.Passed {
				playtestFails = append(playtestFails, pt.Name)
			}
		}
		if len(playtestFails) > 0 {
			fmt.Fprintln(w)
			fmt.Fprintln(w, "  Playtest Failures:")
			for _, name := range playtestFails {
				fmt.Fprintf(w, "  x %s\n", name)
			}
		}
	}

	// Summary totals
	fmt.Fprintln(w)
	fmt.Fprintln(w, bar)
	fmt.Fprintln(w)
	fmt.Fprintf(w, "  Total: %-5d  Passed: %-5d  Failed: %-5d  Skipped: %-5d\n",
		report.TotalTests(), report.TotalPassed(), report.TotalFailed(), report.TotalSkipped())
	fmt.Fprintln(w)

	if report.AllPassed() {
		fmt.Fprintln(w, "  RESULT: ALL TESTS PASSED")
	} else {
		fmt.Fprintln(w, "  RESULT: SOME TESTS FAILED")
	}

	fmt.Fprintln(w)
	fmt.Fprintln(w, bar)
}

// renderVerbose shows the full detailed output (original behavior).
func renderVerbose(w io.Writer, report *TestReport) {
	fmt.Fprintln(w)
	fmt.Fprintln(w, bar)
	fmt.Fprintln(w, "                       GDBG TEST REPORT")
	fmt.Fprintln(w, bar)

	// gdUnit4 section
	if report.GdUnit != nil {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "  gdUnit4 (Unit)")
		fmt.Fprintln(w, "  "+strings.Repeat("─", 28))
		fmt.Fprintln(w)

		if report.GdUnit.Parsed {
			fmt.Fprintf(w, "  %-38s %5s %5s %5s %7s\n", "Suite", "Tests", "Pass", "Fail", "Time")
			fmt.Fprintf(w, "  %-38s %5s %5s %5s %7s\n",
				strings.Repeat("─", 38), strings.Repeat("─", 5),
				strings.Repeat("─", 5), strings.Repeat("─", 5),
				strings.Repeat("─", 7))

			var failures []string
			totalTime := 0.0

			for _, suite := range report.GdUnit.Suites.TestSuites {
				pass := suite.Tests - suite.Failures - suite.Errors
				mark := "  "
				if suite.Failures > 0 || suite.Errors > 0 {
					mark = "<<"
				}
				name := suite.Name
				if len(name) > 38 {
					name = name[:38]
				}
				fmt.Fprintf(w, "  %-38s %5d %5d %5d %6.1fs %s\n",
					name, suite.Tests, pass, suite.Failures+suite.Errors, suite.Time, mark)
				totalTime += suite.Time

				for _, tc := range suite.Cases {
					if tc.Failure != nil || tc.Error != nil {
						failures = append(failures, fmt.Sprintf("    x %s.%s", suite.Name, tc.Name))
					}
				}
			}

			gdPass := report.GdUnit.Suites.Tests - report.GdUnit.Suites.Failures - report.GdUnit.Suites.Errors - report.GdUnit.Suites.Skipped
			fmt.Fprintf(w, "\n  Subtotal: %-5d Passed: %-5d Failed: %-5d Skipped: %-5d\n",
				report.GdUnit.Suites.Tests, gdPass, report.GdUnit.Suites.Failures+report.GdUnit.Suites.Errors, report.GdUnit.Suites.Skipped)
			fmt.Fprintf(w, "  Duration: %.1fs\n", totalTime)

			if len(failures) > 0 {
				fmt.Fprintln(w)
				fmt.Fprintln(w, "  Failures:")
				for _, f := range failures {
					fmt.Fprintln(w, f)
				}
			}
		} else {
			fmt.Fprintln(w, "  (no JUnit report found)")
		}
	}

	// Integration section
	if report.Integration != nil {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "  IntegrationTestSuite")
		fmt.Fprintln(w, "  "+strings.Repeat("─", 28))
		fmt.Fprintln(w)

		if len(report.Integration) > 0 {
			for _, sc := range report.Integration {
				status := "PASS"
				if !sc.Passed {
					status = "FAIL"
				}
				fmt.Fprintf(w, "  [%s] %s\n", status, sc.Name)
			}
		} else {
			fmt.Fprintln(w, "  (no integration tests found)")
		}
	}

	if report.Playtest != nil {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "  Automated Playtest")
		fmt.Fprintln(w, "  "+strings.Repeat("─", 28))
		fmt.Fprintln(w)

		if len(report.Playtest) > 0 {
			for _, pt := range report.Playtest {
				status := "PASS"
				if !pt.Passed {
					status = "FAIL"
				}
				fmt.Fprintf(w, "  [%s] %s\n", status, pt.Name)
				if pt.ReportPath != "" {
					fmt.Fprintf(w, "       report: %s\n", pt.ReportPath)
				}
				if pt.RecordingPath != "" {
					fmt.Fprintf(w, "       recording: %s\n", pt.RecordingPath)
				}
			}
		} else {
			fmt.Fprintln(w, "  (no automated playtests found)")
		}
	}

	// Grand totals
	fmt.Fprintln(w)
	fmt.Fprintln(w, bar)
	fmt.Fprintln(w)
	fmt.Fprintf(w, "  Total: %-5d  Passed: %-5d  Failed: %-5d  Skipped: %-5d\n",
		report.TotalTests(), report.TotalPassed(), report.TotalFailed(), report.TotalSkipped())
	fmt.Fprintln(w)

	if report.AllPassed() {
		fmt.Fprintln(w, "  RESULT: ALL TESTS PASSED")
	} else {
		fmt.Fprintln(w, "  RESULT: SOME TESTS FAILED")
	}

	fmt.Fprintln(w)
	fmt.Fprintln(w, bar)
}

// dumpRawOutputTail prints the last ~30 lines of raw gdunit4 output to stderr
// for diagnosis when no JUnit report was found.
func dumpRawOutputTail(raw string) {
	lines := strings.Split(raw, "\n")
	start := max(len(lines)-30, 0)
	fmt.Fprintln(os.Stderr, "  Last output lines:")
	for _, line := range lines[start:] {
		fmt.Fprintf(os.Stderr, "  %s\n", line)
	}
}
