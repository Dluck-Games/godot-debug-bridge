// godot-debug-bridge/cli/cmd/test.go
package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Dluck-Games/godot-debug-bridge/cli/internal/godot"
	"github.com/Dluck-Games/godot-debug-bridge/cli/internal/testrunner"
)

var (
	flagAll              bool // only registered to catch usage and emit migration error
	flagRecord           bool
	flagHeadless         bool
	flagPlaytestWindowed bool
	flagSuite            string
	flagPlaytestFrameDir string
)

var testCmd = &cobra.Command{
	Use:   "test <tiers> --suite <suites> [--record] [--headless] [--windowed] [--verbose]",
	Short: "Run test suites by tier",
	Long: `Run gdUnit4 unit tests, IntegrationTestSuite integration tests, and automated playtests.

Tiers (positional, comma-separable): one or more of 'unit', 'integration', 'playtest'.
Suites (--suite, required): comma-separated suite names, or 'all' for every discovered suite.

Playtest display: default is background (window exists for recording, does not take focus).
Use --headless for no window, or --windowed to watch in the foreground.

Dependencies:
  unit requires the gdUnit4 addon (addons/gdUnit4/plugin.cfg and bin/GdUnitCmdTool.gd).
  integration and playtest require the GDBG addon testing assets (addons/gdbg/).

Examples:
  gdbg test unit --suite system           # unit tests for the 'system' suite
  gdbg test unit,integration --suite gameplay  # matching unit + integration suites
  gdbg test unit,integration --suite all  # all unit + integration suites
  gdbg test playtest --suite smoke        # playtest 'smoke' suite (background)
  gdbg test playtest --suite all          # all playtest suites
  gdbg test unit,integration,playtest --suite all   # all tiers, all suites
  gdbg test playtest --suite smoke --record         # playtest with recording (background)
  gdbg test playtest --suite smoke --headless       # playtest with no window
  gdbg test playtest --suite smoke --windowed       # playtest in a foreground window`,
	Args: cobra.MaximumNArgs(1),
	RunE: testRun,
}

func init() {
	rootCmd.AddCommand(testCmd)
	testCmd.Flags().BoolVar(&flagAll, "all", false, "REMOVED: use tiers positional + --suite all instead")
	testCmd.Flags().Lookup("all").Hidden = true
	testCmd.Flags().BoolVar(&flagRecord, "record", false, "record automated playtest video (only valid with playtest)")
	testCmd.Flags().BoolVar(&flagHeadless, "headless", false, "run playtest with no window (only valid with playtest; incompatible with --record and --windowed)")
	testCmd.Flags().BoolVar(&flagPlaytestWindowed, "windowed", false, "show the playtest window in the foreground (only valid with playtest; incompatible with --headless)")
	testCmd.Flags().StringVar(&flagSuite, "suite", "", "comma-separated suite names to run (e.g. gameplay,render,system), or 'all' for all discovered suites")
	testCmd.Flags().StringVar(&flagPlaytestFrameDir, "playtest-frame-dir", "", "explicit directory for automated playtest frames")
	testCmd.MarkFlagsMutuallyExclusive("headless", "windowed")
}

func testRun(cmd *cobra.Command, args []string) error {
	// Catch legacy --all before anything else so the migration message is always shown.
	if cmd.Flags().Changed("all") {
		return fmt.Errorf("--all was removed. Use 'gdbg test unit,integration --suite all' (old 'gdbg test --all') or 'gdbg test playtest --suite all' (old 'gdbg test playtest --all')")
	}

	projectDir, err := resolveProject()
	if err != nil {
		return err
	}

	tiers, opts, err := resolveTestSelection(cmd, args, projectDir)
	if err != nil {
		return err
	}

	// Fail fast on missing addon dependencies before locating or launching
	// Godot, so the CLI reports an actionable addon error even when Godot is
	// unavailable.
	if err := testrunner.Preflight(projectDir, tiers); err != nil {
		return err
	}

	godotBin, err := godot.FindBinary()
	if err != nil {
		return err
	}

	report, err := testrunner.Run(godotBin, projectDir, tiers, opts)
	if err != nil {
		return err
	}

	testrunner.RenderASCII(os.Stdout, report, opts.Verbose)

	if !report.AllPassed() {
		os.Exit(report.ExitCode())
	}
	return nil
}

func resolveTestSelection(cmd *cobra.Command, args []string, projectDir string) ([]testrunner.Tier, testrunner.RunOptions, error) {
	// Reject --all if present.
	if cmd.Flags().Changed("all") {
		return nil, testrunner.RunOptions{},
			fmt.Errorf("--all was removed. Use 'gdbg test unit,integration --suite all' (old 'gdbg test --all') or 'gdbg test playtest --suite all' (old 'gdbg test playtest --all')")
	}

	// Parse tiers from positional args.
	if len(args) == 0 || args[0] == "" {
		return nil, testrunner.RunOptions{}, fmt.Errorf("no tiers specified; use e.g. 'gdbg test unit --suite system' or 'gdbg test unit,integration --suite all'\n\nUsage:\n  gdbg test <tiers> --suite <suites> [--record] [--verbose]")
	}

	tierNames := strings.Split(args[0], ",")
	tiers, err := parseTierList(tierNames)
	if err != nil {
		return nil, testrunner.RunOptions{}, err
	}

	// --suite is required.
	suiteArg := strings.TrimSpace(flagSuite)
	if suiteArg == "" {
		return nil, testrunner.RunOptions{}, fmt.Errorf("--suite is required; use e.g. '--suite system' or '--suite all'")
	}

	// --suite all → empty filter (runners interpret nil/empty as "all").
	var suites []string
	if suiteArg != "all" {
		suites = parseSuiteList(suiteArg)
		if len(suites) == 0 {
			return nil, testrunner.RunOptions{}, fmt.Errorf("--suite must be 'all' or a comma-separated list of suite names")
		}
	}

	// Validate each named suite exists in at least one tier.
	if len(suites) > 0 {
		if err := validateSuiteExistence(projectDir, tiers, suites); err != nil {
			return nil, testrunner.RunOptions{}, err
		}
	}

	// --record only valid with playtest.
	hasPlaytest := false
	for _, t := range tiers {
		if t == testrunner.TierPlaytest {
			hasPlaytest = true
			break
		}
	}
	if flagRecord && !hasPlaytest {
		return nil, testrunner.RunOptions{}, fmt.Errorf("--record requires the playtest tier")
	}
	if flagHeadless && !hasPlaytest {
		return nil, testrunner.RunOptions{}, fmt.Errorf("--headless requires the playtest tier")
	}
	if flagPlaytestWindowed && !hasPlaytest {
		return nil, testrunner.RunOptions{}, fmt.Errorf("--windowed requires the playtest tier")
	}
	if flagHeadless && flagPlaytestWindowed {
		return nil, testrunner.RunOptions{}, fmt.Errorf("--headless cannot be combined with --windowed")
	}
	if flagHeadless && flagRecord {
		return nil, testrunner.RunOptions{}, fmt.Errorf("--headless cannot be combined with --record (recording needs a window)")
	}
	if flagPlaytestFrameDir != "" && !hasPlaytest {
		return nil, testrunner.RunOptions{}, fmt.Errorf("--playtest-frame-dir requires the playtest tier")
	}

	return tiers, testrunner.RunOptions{
		Record:           flagRecord,
		Verbose:          flagVerbose,
		Headless:         flagHeadless,
		Windowed:         flagPlaytestWindowed,
		Suite:            suites,
		PlaytestFrameDir: flagPlaytestFrameDir,
	}, nil
}

// validateSuiteExistence checks that each named suite exists in at least one
// of the listed tiers. For unit/integration: tests/<tier>/<suite>/ directory.
// For playtest: tests/playtest/playtest_<suite>.gd file.
func validateSuiteExistence(projectDir string, tiers []testrunner.Tier, suites []string) error {
	for _, suite := range suites {
		found := false
		for _, tier := range tiers {
			var candidates []string
			switch tier {
			case testrunner.TierUnit:
				candidates = []string{filepath.Join(projectDir, "tests", "unit", suite)}
			case testrunner.TierIntegration:
				candidates = []string{
					filepath.Join(projectDir, "tests", "integration", suite),
					filepath.Join(projectDir, "tests", "integration", "test_"+suite+".gd"),
				}
			case testrunner.TierPlaytest:
				candidates = []string{filepath.Join(projectDir, "tests", "playtest", "playtest_"+suite+".gd")}
			}
			for _, path := range candidates {
				if stat, err := os.Stat(path); err == nil {
					wantDir := tier == testrunner.TierUnit || (tier == testrunner.TierIntegration && filepath.Ext(path) == "")
					if stat.IsDir() == wantDir {
						found = true
						break
					}
				}
			}
			if found {
				break
			}
		}
		if !found {
			tierNames := make([]string, len(tiers))
			for i, t := range tiers {
				tierNames[i] = tierShortName(t)
			}
			return fmt.Errorf("suite %q not found in any of the listed tiers (%s)", suite, strings.Join(tierNames, ", "))
		}
	}
	return nil
}

func tierShortName(t testrunner.Tier) string {
	switch t {
	case testrunner.TierUnit:
		return "unit"
	case testrunner.TierIntegration:
		return "integration"
	case testrunner.TierPlaytest:
		return "playtest"
	}
	return "unknown"
}

// parseTierList parses a cleaned slice of tier name strings into Tier values.
// Deduplicates and returns an error for any unknown tier name.
func parseTierList(names []string) ([]testrunner.Tier, error) {
	seen := make(map[testrunner.Tier]bool)
	var tiers []testrunner.Tier
	for _, name := range names {
		name = strings.TrimSpace(strings.ToLower(name))
		if name == "" {
			continue
		}
		t, err := testrunner.ParseTier(name)
		if err != nil {
			return nil, err
		}
		if !seen[t] {
			seen[t] = true
			tiers = append(tiers, t)
		}
	}
	if len(tiers) == 0 {
		return nil, fmt.Errorf("no valid tier names provided")
	}
	// Sort into canonical order: unit → integration → playtest.
	sort.Slice(tiers, func(i, j int) bool { return tiers[i] < tiers[j] })
	return tiers, nil
}

// parseSuiteList splits a comma-separated suite string into a cleaned slice.
// Returns nil if the input is empty.
func parseSuiteList(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	var out []string
	for _, p := range parts {
		p = strings.TrimSpace(strings.ToLower(p))
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
