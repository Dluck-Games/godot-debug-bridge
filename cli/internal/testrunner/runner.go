// godot-debug-bridge/cli/internal/testrunner/runner.go
package testrunner

import (
	"fmt"
	"os"
)

type Tier int

const (
	TierUnit Tier = iota + 1
	TierIntegration
	TierPlaytest
)

// RunOptions controls test execution behavior.
type RunOptions struct {
	Record           bool
	Verbose          bool
	Headless         bool     // playtest: Godot --headless, no window
	Windowed         bool     // playtest: foreground window (may take focus)
	Suite            []string // empty = all suites (maps to CLI --suite all)
	PlaytestFrameDir string   // optional explicit screenshot/recording frame directory
}

func ParseTier(s string) (Tier, error) {
	switch s {
	case "unit":
		return TierUnit, nil
	case "integration":
		return TierIntegration, nil
	case "playtest":
		return TierPlaytest, nil
	default:
		return 0, fmt.Errorf("unknown test tier %q (use 'unit', 'integration', or 'playtest')", s)
	}
}

func tierOrder() []Tier {
	return []Tier{TierUnit, TierIntegration, TierPlaytest}
}

func Run(godotBin, projectDir string, tiers []Tier, opts RunOptions) (*TestReport, error) {
	// Fail fast: validate every selected tier's dependencies before launching
	// any Godot process, so a missing later tier can never be masked by an
	// earlier tier that already ran.
	if err := Preflight(projectDir, tiers); err != nil {
		return nil, err
	}

	report := &TestReport{}

	// Ensure tiers run in the canonical order: unit → integration → playtest.
	tierSet := make(map[Tier]bool, len(tiers))
	for _, t := range tiers {
		tierSet[t] = true
	}
	numTiers := len(tierSet)

	step := 1
	for _, t := range tierOrder() {
		if !tierSet[t] {
			continue
		}
		switch t {
		case TierUnit:
			if opts.Verbose {
				fmt.Fprintf(os.Stderr, "> [%d/%d] gdUnit4 unit tests...\n\n", step, numTiers)
			}
			result, err := RunGdUnit(godotBin, projectDir, opts)
			if err != nil {
				return nil, fmt.Errorf("gdUnit4 failed: %w", err)
			}
			report.GdUnit = result
		case TierIntegration:
			if opts.Verbose {
				fmt.Fprintf(os.Stderr, "\n> [%d/%d] IntegrationTestSuite integration tests...\n\n", step, numTiers)
			}
			results, err := RunIntegrationTests(godotBin, projectDir, opts)
			if err != nil {
				return nil, fmt.Errorf("integration tests failed: %w", err)
			}
			report.Integration = results
		case TierPlaytest:
			if opts.Verbose {
				fmt.Fprintf(os.Stderr, "\n> [%d/%d] Automated playtests...\n\n", step, numTiers)
			}
			results, err := RunPlaytest(godotBin, projectDir, opts)
			if err != nil {
				return nil, fmt.Errorf("playtest failed: %w", err)
			}
			report.Playtest = results
		}
		step++
	}

	return report, nil
}
