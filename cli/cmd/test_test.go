package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/Dluck-Games/godot-debug-bridge/cli/internal/testrunner"
)

// helper to create a cobra.Command with known flag state.
// Resets package-level flag vars to avoid leakage.
func newTestCmd(allSet, recordSet bool, suiteVal string) *cobra.Command {
	flagRecord = recordSet
	flagHeadless = false
	flagPlaytestWindowed = false
	flagSuite = suiteVal
	cmd := &cobra.Command{}
	cmd.Flags().Bool("all", false, "")
	cmd.Flags().Bool("record", false, "")
	cmd.Flags().Bool("headless", false, "")
	cmd.Flags().Bool("windowed", false, "")
	cmd.Flags().String("suite", "", "")
	if allSet {
		cmd.Flags().Set("all", "true")
	}
	if recordSet {
		cmd.Flags().Set("record", "true")
	}
	if suiteVal != "" {
		cmd.Flags().Set("suite", suiteVal)
	}
	return cmd
}

func mkdirAll(t *testing.T, dir, path string) {
	t.Helper()
	full := filepath.Join(dir, path)
	if err := os.MkdirAll(full, 0o755); err != nil {
		t.Fatalf("MkdirAll %s: %v", path, err)
	}
}

func writeFile(t *testing.T, dir, path string) {
	t.Helper()
	full := filepath.Join(dir, path)
	os.MkdirAll(filepath.Dir(full), 0o755)
	if err := os.WriteFile(full, nil, 0o644); err != nil {
		t.Fatalf("WriteFile %s: %v", path, err)
	}
}

// fixtureProject creates a temp dir with typical suite layout.
func fixtureProject(t *testing.T, suites ...string) string {
	t.Helper()
	dir := t.TempDir()
	for _, s := range suites {
		mkdirAll(t, dir, "tests/unit/"+s)
		mkdirAll(t, dir, "tests/integration/"+s)
		writeFile(t, dir, "tests/playtest/playtest_"+s+".gd")
	}
	return dir
}

// ---- Tier parsing (uses --suite all to skip existence check) ----

func TestResolveTestSelection_SingleTierAll(t *testing.T) {
	cmd := newTestCmd(false, false, "all")
	tiers, opts, err := resolveTestSelection(cmd, []string{"unit"}, "/fake")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tiers) != 1 || tiers[0] != testrunner.TierUnit {
		t.Errorf("expected [unit], got %v", tiers)
	}
	if opts.Suite != nil {
		t.Errorf("expected nil suites, got %v", opts.Suite)
	}
}

func TestResolveTestSelection_SingleTierWithSuite(t *testing.T) {
	dir := fixtureProject(t, "system")
	cmd := newTestCmd(false, false, "system")
	tiers, opts, err := resolveTestSelection(cmd, []string{"unit"}, dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tiers) != 1 || tiers[0] != testrunner.TierUnit {
		t.Errorf("expected [unit], got %v", tiers)
	}
	if len(opts.Suite) != 1 || opts.Suite[0] != "system" {
		t.Errorf("expected [system], got %v", opts.Suite)
	}
}

func TestResolveTestSelection_MultiTierComma(t *testing.T) {
	cmd := newTestCmd(false, false, "all")
	tiers, _, err := resolveTestSelection(cmd, []string{"unit,integration"}, "/fake")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tiers) != 2 || tiers[0] != testrunner.TierUnit || tiers[1] != testrunner.TierIntegration {
		t.Errorf("expected [unit integration], got %v", tiers)
	}
}

func TestResolveTestSelection_MultiTierWithSpaces(t *testing.T) {
	cmd := newTestCmd(false, false, "all")
	tiers, _, err := resolveTestSelection(cmd, []string{"playtest  ,  unit"}, "/fake")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tiers) != 2 || tiers[0] != testrunner.TierUnit || tiers[1] != testrunner.TierPlaytest {
		t.Errorf("expected [unit playtest] sorted, got %v", tiers)
	}
}

func TestResolveTestSelection_InvalidTier(t *testing.T) {
	cmd := newTestCmd(false, false, "all")
	_, _, err := resolveTestSelection(cmd, []string{"bogus"}, "/fake")
	if err == nil || !strings.Contains(err.Error(), "unknown test tier") {
		t.Fatalf("expected unknown tier error, got %v", err)
	}
}

func TestResolveTestSelection_InvalidTierInList(t *testing.T) {
	cmd := newTestCmd(false, false, "all")
	_, _, err := resolveTestSelection(cmd, []string{"unit,bogus"}, "/fake")
	if err == nil || !strings.Contains(err.Error(), "unknown test tier") {
		t.Fatalf("expected unknown tier error, got %v", err)
	}
}

func TestResolveTestSelection_ThreeTiers(t *testing.T) {
	cmd := newTestCmd(false, false, "all")
	tiers, _, err := resolveTestSelection(cmd, []string{"unit,integration,playtest"}, "/fake")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tiers) != 3 {
		t.Errorf("expected 3 tiers, got %d: %v", len(tiers), tiers)
	}
}

// ---- --all rejection ----

func TestResolveTestSelection_AllFlagRejected(t *testing.T) {
	cmd := newTestCmd(true, false, "all")
	_, _, err := resolveTestSelection(cmd, []string{}, "/fake")
	if err == nil {
		t.Fatal("expected error for --all")
	}
	if !strings.Contains(err.Error(), "--all was removed") {
		t.Fatalf("expected migration error, got %v", err)
	}
}

func TestResolveTestSelection_AllFlagWithTierRejected(t *testing.T) {
	cmd := newTestCmd(true, false, "all")
	_, _, err := resolveTestSelection(cmd, []string{"unit,integration"}, "/fake")
	if err == nil {
		t.Fatal("expected error for --all")
	}
	if !strings.Contains(err.Error(), "--all was removed") {
		t.Fatalf("expected migration error, got %v", err)
	}
}

// ---- --suite all expansion ----

func TestResolveTestSelection_SuiteAllUnit(t *testing.T) {
	cmd := newTestCmd(false, false, "all")
	tiers, opts, err := resolveTestSelection(cmd, []string{"unit"}, "/fake")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tiers) != 1 || tiers[0] != testrunner.TierUnit {
		t.Errorf("expected [unit], got %v", tiers)
	}
	if opts.Suite != nil {
		t.Errorf("expected nil (all) suites, got %v", opts.Suite)
	}
}

func TestResolveTestSelection_SuiteAllPlaytest(t *testing.T) {
	cmd := newTestCmd(false, false, "all")
	tiers, opts, err := resolveTestSelection(cmd, []string{"playtest"}, "/fake")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tiers) != 1 || tiers[0] != testrunner.TierPlaytest {
		t.Errorf("expected [playtest], got %v", tiers)
	}
	if opts.Suite != nil {
		t.Errorf("expected nil (all) suites, got %v", opts.Suite)
	}
}

func TestResolveTestSelection_SuiteAllMultiTier(t *testing.T) {
	cmd := newTestCmd(false, false, "all")
	tiers, opts, err := resolveTestSelection(cmd, []string{"unit,integration"}, "/fake")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tiers) != 2 {
		t.Errorf("expected 2 tiers, got %d: %v", len(tiers), tiers)
	}
	if opts.Suite != nil {
		t.Errorf("expected nil (all) suites, got %v", opts.Suite)
	}
}

// ---- --suite required ----

func TestResolveTestSelection_NoSuite(t *testing.T) {
	cmd := newTestCmd(false, false, "")
	_, _, err := resolveTestSelection(cmd, []string{"unit"}, "/fake")
	if err == nil || !strings.Contains(err.Error(), "--suite is required") {
		t.Fatalf("expected --suite required error, got %v", err)
	}
}

func TestResolveTestSelection_NoSuitePlaytest(t *testing.T) {
	cmd := newTestCmd(false, false, "")
	_, _, err := resolveTestSelection(cmd, []string{"playtest"}, "/fake")
	if err == nil || !strings.Contains(err.Error(), "--suite is required") {
		t.Fatalf("expected --suite required error, got %v", err)
	}
}

func TestResolveTestSelection_NoTiersEmptyArgs(t *testing.T) {
	cmd := newTestCmd(false, false, "system")
	_, _, err := resolveTestSelection(cmd, []string{""}, "/fake")
	if err == nil || !strings.Contains(err.Error(), "no tiers specified") {
		t.Fatalf("expected no tiers error, got %v", err)
	}
}

func TestResolveTestSelection_NoTiersNilArgs(t *testing.T) {
	cmd := newTestCmd(false, false, "system")
	_, _, err := resolveTestSelection(cmd, nil, "/fake")
	if err == nil || !strings.Contains(err.Error(), "no tiers specified") {
		t.Fatalf("expected no tiers error, got %v", err)
	}
}

// ---- --record without playtest ----

func TestResolveTestSelection_RecordWithoutPlaytest(t *testing.T) {
	cmd := newTestCmd(false, true, "all")
	_, _, err := resolveTestSelection(cmd, []string{"unit"}, "/fake")
	if err == nil || !strings.Contains(err.Error(), "--record requires the playtest tier") {
		t.Fatalf("expected --record tier error, got %v", err)
	}
}

func TestResolveTestSelection_RecordWithPlaytest(t *testing.T) {
	cmd := newTestCmd(false, true, "all")
	tiers, opts, err := resolveTestSelection(cmd, []string{"playtest"}, "/fake")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !opts.Record {
		t.Error("expected record=true")
	}
	if len(tiers) != 1 || tiers[0] != testrunner.TierPlaytest {
		t.Errorf("expected [playtest], got %v", tiers)
	}
}

func TestResolveTestSelection_RecordWithMultiTierIncludingPlaytest(t *testing.T) {
	cmd := newTestCmd(false, true, "all")
	tiers, opts, err := resolveTestSelection(cmd, []string{"unit,playtest"}, "/fake")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !opts.Record {
		t.Error("expected record=true")
	}
	if len(tiers) != 2 {
		t.Errorf("expected 2 tiers, got %d: %v", len(tiers), tiers)
	}
}

// ---- Suite existence validation ----

func TestResolveTestSelection_SuiteExistsInUnit(t *testing.T) {
	dir := fixtureProject(t, "system")
	cmd := newTestCmd(false, false, "system")
	_, _, err := resolveTestSelection(cmd, []string{"unit"}, dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResolveTestSelection_SuiteExistsInPlaytest(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "tests/playtest/playtest_smoke.gd")
	cmd := newTestCmd(false, false, "smoke")
	_, _, err := resolveTestSelection(cmd, []string{"playtest"}, dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResolveTestSelection_SuiteExistsInMultiTier(t *testing.T) {
	dir := fixtureProject(t, "system")
	cmd := newTestCmd(false, false, "system")
	_, _, err := resolveTestSelection(cmd, []string{"unit,integration"}, dir)
	if err != nil {
		t.Fatalf("expected suite to be found in at least unit tier, got %v", err)
	}
}

func TestResolveTestSelection_SuiteInNoTier(t *testing.T) {
	dir := fixtureProject(t, "system")
	cmd := newTestCmd(false, false, "bogus")
	_, _, err := resolveTestSelection(cmd, []string{"unit,integration"}, dir)
	if err == nil {
		t.Fatal("expected error for suite in no tier")
	}
	if !strings.Contains(err.Error(), "not found in any of the listed tiers") {
		t.Fatalf("expected 'not found' error, got %v", err)
	}
}

func TestResolveTestSelection_SuiteInNoTierPlaytest(t *testing.T) {
	dir := fixtureProject(t, "system")
	cmd := newTestCmd(false, false, "bogus")
	_, _, err := resolveTestSelection(cmd, []string{"playtest"}, dir)
	if err == nil {
		t.Fatal("expected error for suite in no tier")
	}
	if !strings.Contains(err.Error(), "not found in any of the listed tiers") {
		t.Fatalf("expected 'not found' error, got %v", err)
	}
}

// ---- Tier deduplication ----

func TestResolveTestSelection_DedupAndSortTiers(t *testing.T) {
	cmd := newTestCmd(false, false, "all")
	tiers, _, err := resolveTestSelection(cmd, []string{"playtest,unit,playtest"}, "/fake")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tiers) != 2 || tiers[0] != testrunner.TierUnit || tiers[1] != testrunner.TierPlaytest {
		t.Errorf("expected deduped sorted [unit playtest], got %v", tiers)
	}
}

// ---- parseTierList ----

func TestParseTierList_Dedup(t *testing.T) {
	tiers, err := parseTierList([]string{"unit", "unit", "integration"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tiers) != 2 {
		t.Errorf("expected 2 tiers after dedup, got %d: %v", len(tiers), tiers)
	}
}

func TestParseTierList_EmptyAfterTrim(t *testing.T) {
	// Simulate what strings.Split(", ,", ",") produces after trimming.
	_, err := parseTierList([]string{"", " ", ""})
	if err == nil || !strings.Contains(err.Error(), "no valid tier names") {
		t.Fatalf("expected empty tiers error, got %v", err)
	}
}

func TestParseTierList_Invalid(t *testing.T) {
	_, err := parseTierList([]string{"nope"})
	if err == nil || !strings.Contains(err.Error(), "unknown test tier") {
		t.Fatalf("expected unknown tier error, got %v", err)
	}
}

// ---- parseSuiteList ----

func TestParseSuiteList_NormalizesCase(t *testing.T) {
	suites := parseSuiteList("System,Gameplay")
	if len(suites) != 2 || suites[0] != "system" || suites[1] != "gameplay" {
		t.Errorf("expected [system gameplay], got %v", suites)
	}
}

func TestParseSuiteList_TrimsWhitespace(t *testing.T) {
	suites := parseSuiteList(" system , gameplay ")
	if len(suites) != 2 || suites[0] != "system" || suites[1] != "gameplay" {
		t.Errorf("expected [system gameplay], got %v", suites)
	}
}

func TestParseSuiteList_Empty(t *testing.T) {
	if parseSuiteList("") != nil {
		t.Error("expected nil for empty input")
	}
}

func TestParseSuiteList_OnlyCommas(t *testing.T) {
	suites := parseSuiteList(",,")
	if len(suites) != 0 {
		t.Errorf("expected empty slice, got %v", suites)
	}
}

func TestResolveTestSelection_HeadlessWithoutPlaytest(t *testing.T) {
	cmd := newTestCmd(false, false, "all")
	flagHeadless = true
	_, _, err := resolveTestSelection(cmd, []string{"unit"}, "/fake")
	if err == nil || !strings.Contains(err.Error(), "--headless requires the playtest tier") {
		t.Fatalf("expected --headless tier error, got %v", err)
	}
}

func TestResolveTestSelection_HeadlessWithPlaytest(t *testing.T) {
	cmd := newTestCmd(false, false, "all")
	flagHeadless = true
	_, opts, err := resolveTestSelection(cmd, []string{"playtest"}, "/fake")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !opts.Headless {
		t.Error("expected Headless=true")
	}
	if opts.Windowed {
		t.Error("expected Windowed=false")
	}
}

func TestResolveTestSelection_WindowedWithoutPlaytest(t *testing.T) {
	cmd := newTestCmd(false, false, "all")
	flagPlaytestWindowed = true
	_, _, err := resolveTestSelection(cmd, []string{"unit"}, "/fake")
	if err == nil || !strings.Contains(err.Error(), "--windowed requires the playtest tier") {
		t.Fatalf("expected --windowed tier error, got %v", err)
	}
}

func TestResolveTestSelection_WindowedWithPlaytest(t *testing.T) {
	cmd := newTestCmd(false, false, "all")
	flagPlaytestWindowed = true
	_, opts, err := resolveTestSelection(cmd, []string{"playtest"}, "/fake")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !opts.Windowed {
		t.Error("expected Windowed=true")
	}
	if opts.Headless {
		t.Error("expected Headless=false")
	}
}

func TestResolveTestSelection_HeadlessWithRecord(t *testing.T) {
	cmd := newTestCmd(false, true, "all")
	flagHeadless = true
	_, _, err := resolveTestSelection(cmd, []string{"playtest"}, "/fake")
	if err == nil || !strings.Contains(err.Error(), "--headless cannot be combined with --record") {
		t.Fatalf("expected headless+record error, got %v", err)
	}
}

func TestResolveTestSelection_HeadlessWithWindowed(t *testing.T) {
	cmd := newTestCmd(false, false, "all")
	flagHeadless = true
	flagPlaytestWindowed = true
	_, _, err := resolveTestSelection(cmd, []string{"playtest"}, "/fake")
	if err == nil || !strings.Contains(err.Error(), "--headless cannot be combined with --windowed") {
		t.Fatalf("expected headless+windowed error, got %v", err)
	}
}
