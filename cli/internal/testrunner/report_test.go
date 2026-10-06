package testrunner

import (
	"bytes"
	"strings"
	"testing"
)

func TestAllPassedTrustsParsedGdUnitReportOverExitCode(t *testing.T) {
	report := &TestReport{
		GdUnit: &GdUnitResult{
			Parsed:   true,
			ExitCode: 1,
			Suites: JUnitTestSuites{
				Tests:    3,
				Failures: 0,
				Errors:   0,
			},
		},
	}

	if !report.AllPassed() {
		t.Fatal("expected parsed gdUnit report with no failures or errors to pass")
	}
}

func TestAllPassedFailsParsedGdUnitErrors(t *testing.T) {
	report := &TestReport{
		GdUnit: &GdUnitResult{
			Parsed:   true,
			ExitCode: 0,
			Suites: JUnitTestSuites{
				Tests:  3,
				Errors: 1,
			},
		},
	}

	if report.AllPassed() {
		t.Fatal("expected parsed gdUnit report with errors to fail")
	}
}

func TestGdUnitTotalsIncludeErrors(t *testing.T) {
	report := &TestReport{
		GdUnit: &GdUnitResult{
			Parsed: true,
			Suites: JUnitTestSuites{
				Tests:    5,
				Failures: 1,
				Errors:   1,
				Skipped:  1,
			},
		},
	}

	if got := report.TotalPassed(); got != 2 {
		t.Fatalf("TotalPassed() = %d, want 2", got)
	}
	if got := report.TotalFailed(); got != 2 {
		t.Fatalf("TotalFailed() = %d, want 2", got)
	}
}

func TestPlaytestTotalsAndPassStatus(t *testing.T) {
	report := &TestReport{
		Playtest: []PlaytestResult{
			{Name: "night_raid", Passed: true},
			{Name: "smoke", Passed: false},
		},
	}

	if got := report.TotalTests(); got != 2 {
		t.Fatalf("TotalTests() = %d, want 2", got)
	}
	if got := report.TotalPassed(); got != 1 {
		t.Fatalf("TotalPassed() = %d, want 1", got)
	}
	if got := report.TotalFailed(); got != 1 {
		t.Fatalf("TotalFailed() = %d, want 1", got)
	}
	if report.AllPassed() {
		t.Fatal("expected failed playtest to fail report")
	}
	if got := report.ExitCode(); got != 1 {
		t.Fatalf("ExitCode() = %d, want 1", got)
	}
}

func TestPlaytestErrorExitCodeIsPreserved(t *testing.T) {
	report := &TestReport{
		Playtest: []PlaytestResult{
			{Name: "night_raid", Passed: false, ExitCode: 2},
		},
	}

	if got := report.ExitCode(); got != 2 {
		t.Fatalf("ExitCode() = %d, want 2", got)
	}
}

func TestIsSinglePlaytest(t *testing.T) {
	if !IsSinglePlaytest(&TestReport{Playtest: []PlaytestResult{{Name: "input_flow"}}}) {
		t.Fatal("expected single playtest report")
	}
	if IsSinglePlaytest(&TestReport{Playtest: []PlaytestResult{{Name: "a"}, {Name: "b"}}}) {
		t.Fatal("expected multiple playtests to keep the summary report")
	}
	if IsSinglePlaytest(&TestReport{
		GdUnit:   &GdUnitResult{},
		Playtest: []PlaytestResult{{Name: "input_flow"}},
	}) {
		t.Fatal("expected mixed tiers to keep the summary report")
	}
}

func TestRenderASCIISinglePlaytestPrintsSuiteReport(t *testing.T) {
	var buf bytes.Buffer
	RenderASCII(&buf, &TestReport{Playtest: []PlaytestResult{{
		Name:       "input_flow",
		ReportText: "=== PLAYTEST: input_flow ===\nStatus: PASSED\n",
	}}}, false)
	got := buf.String()
	if strings.Contains(got, "GDBG TEST REPORT") {
		t.Fatal("single playtest must not print the summary REPORT banner")
	}
	if !strings.Contains(got, "=== PLAYTEST: input_flow ===") {
		t.Fatalf("missing suite report, got %q", got)
	}
}

func TestCombinedTierExitCodeAndPassing(t *testing.T) {
	cases := []struct {
		name        string
		gdunit      *GdUnitResult
		integration []IntegrationTestResult
		playtest    []PlaytestResult
		wantPassed  bool
		wantExit    int
	}{
		{
			name:        "all tiers pass",
			gdunit:      &GdUnitResult{Parsed: true, Suites: JUnitTestSuites{Tests: 3}},
			integration: []IntegrationTestResult{{Name: "pipeline", Passed: true}},
			playtest:    []PlaytestResult{{Name: "smoke", Passed: true}},
			wantPassed:  true,
			wantExit:    0,
		},
		{
			name:        "unit fails integration passes",
			gdunit:      &GdUnitResult{Parsed: true, Suites: JUnitTestSuites{Tests: 2, Errors: 1}},
			integration: []IntegrationTestResult{{Name: "pipeline", Passed: true}},
			wantPassed:  false,
			wantExit:    1,
		},
		{
			name:        "integration fails playtest passes",
			integration: []IntegrationTestResult{{Name: "pipeline", Passed: false, ExitCode: 1}},
			playtest:    []PlaytestResult{{Name: "smoke", Passed: true}},
			wantPassed:  false,
			wantExit:    1,
		},
		{
			name:       "playtest fails with exit 2",
			playtest:   []PlaytestResult{{Name: "smoke", Passed: false, ExitCode: 2}},
			wantPassed: false,
			wantExit:   2,
		},
		{
			name:       "playtest failure without explicit exit code defaults to 1",
			playtest:   []PlaytestResult{{Name: "smoke", Passed: false}},
			wantPassed: false,
			wantExit:   1,
		},
		{
			name:        "playtest exit 2 takes precedence over integration failure",
			integration: []IntegrationTestResult{{Name: "pipeline", Passed: false, ExitCode: 1}},
			playtest:    []PlaytestResult{{Name: "smoke", Passed: false, ExitCode: 2}},
			wantPassed:  false,
			wantExit:    2,
		},
		{
			name:       "unit errors with playtest pass",
			gdunit:     &GdUnitResult{Parsed: true, Suites: JUnitTestSuites{Tests: 4, Failures: 1}},
			playtest:   []PlaytestResult{{Name: "smoke", Passed: true}},
			wantPassed: false,
			wantExit:   1,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			report := &TestReport{
				GdUnit:      tc.gdunit,
				Integration: tc.integration,
				Playtest:    tc.playtest,
			}
			if got := report.AllPassed(); got != tc.wantPassed {
				t.Fatalf("AllPassed() = %v, want %v", got, tc.wantPassed)
			}
			if got := report.ExitCode(); got != tc.wantExit {
				t.Fatalf("ExitCode() = %d, want %d", got, tc.wantExit)
			}
		})
	}
}

func TestRenderASCIICountsJUnitErrorsAsFailed(t *testing.T) {
	report := &TestReport{
		GdUnit: &GdUnitResult{
			Parsed: true,
			Suites: JUnitTestSuites{
				Tests:    4,
				Failures: 1,
				Errors:   2,
				Skipped:  1,
			},
		},
	}

	if got := report.TotalTests(); got != 4 {
		t.Fatalf("TotalTests() = %d, want 4", got)
	}
	if got := report.TotalPassed(); got != 0 {
		t.Fatalf("TotalPassed() = %d, want 0", got)
	}
	if got := report.TotalFailed(); got != 3 {
		t.Fatalf("TotalFailed() = %d, want 3", got)
	}
	if got := report.TotalSkipped(); got != 1 {
		t.Fatalf("TotalSkipped() = %d, want 1", got)
	}
	if report.AllPassed() {
		t.Fatal("expected JUnit errors to fail the report")
	}

	for _, verbose := range []bool{false, true} {
		var buf bytes.Buffer
		RenderASCII(&buf, report, verbose)
		got := buf.String()
		for _, want := range []string{"Total: 4", "Passed: 0", "Failed: 3", "Skipped: 1"} {
			if !strings.Contains(got, want) {
				t.Fatalf("verbose=%v output missing %q, got %q", verbose, want, got)
			}
		}
		if verbose && !strings.Contains(got, "Subtotal: 4     Passed: 0     Failed: 3     Skipped: 1") {
			t.Fatalf("verbose subtotal must count errors as failures, got %q", got)
		}
	}
}

func TestAllPassedRejectsInvalidGdUnitResults(t *testing.T) {
	cases := []struct {
		name   string
		gdunit *GdUnitResult
	}{
		{
			name:   "missing report",
			gdunit: &GdUnitResult{ExitCode: 0, ValidationReason: "results.xml not found after run"},
		},
		{
			name:   "malformed report",
			gdunit: &GdUnitResult{ExitCode: 0, ValidationReason: "results.xml could not be parsed: unexpected EOF"},
		},
		{
			name:   "zero tests",
			gdunit: &GdUnitResult{Parsed: true, Suites: JUnitTestSuites{Tests: 0}, ValidationReason: "results.xml reported zero tests"},
		},
		{
			name:   "unparsed without explicit reason",
			gdunit: &GdUnitResult{ExitCode: 0},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			report := &TestReport{GdUnit: tc.gdunit}
			if report.AllPassed() {
				t.Fatal("invalid gdUnit result must not report all passed")
			}
			if got := report.ExitCode(); got != 1 {
				t.Fatalf("ExitCode() = %d, want 1", got)
			}
		})
	}
}

func TestRenderASCIIExplainsInvalidGdUnitResult(t *testing.T) {
	const rawLogPath = "logs/gdunit/raw-output.log"
	cases := []struct {
		name   string
		gdunit *GdUnitResult
		reason string
	}{
		{
			name:   "missing report",
			reason: "results.xml not found after run",
		},
		{
			name:   "malformed report",
			reason: "results.xml could not be parsed: unexpected EOF",
		},
		{
			name:   "zero tests",
			reason: "results.xml reported zero tests",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gdunit := &GdUnitResult{
				ExitCode:         137,
				RawOutput:        "engine banner\nlast line before failure\n",
				RawLogPath:       rawLogPath,
				ValidationReason: tc.reason,
			}
			if tc.name == "zero tests" {
				gdunit.Parsed = true
				gdunit.Suites = JUnitTestSuites{Tests: 0}
			}
			report := &TestReport{GdUnit: gdunit}

			if report.AllPassed() {
				t.Fatal("invalid gdUnit result must not report all passed")
			}

			for _, verbose := range []bool{false, true} {
				var buf bytes.Buffer
				RenderASCII(&buf, report, verbose)
				got := buf.String()
				for _, want := range []string{
					tc.reason,
					"engine exit code: 137",
					rawLogPath,
					"last line before failure",
					"SOME TESTS FAILED",
				} {
					if !strings.Contains(got, want) {
						t.Fatalf("verbose=%v output missing %q, got %q", verbose, want, got)
					}
				}
				if strings.Contains(got, "ALL TESTS PASSED") {
					t.Fatalf("verbose=%v must not report success for invalid gdUnit result, got %q", verbose, got)
				}
			}
		})
	}
}
