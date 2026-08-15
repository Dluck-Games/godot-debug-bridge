// godot-debug-bridge/cli/internal/testrunner/paths.go
package testrunner

import "path/filepath"

// Centralized checkout-relative and res:// product paths shared by preflight
// and the command builders. Execution and validation must resolve the same
// paths so the two can never drift apart.

const (
	// GdUnit4CmdToolRes is the gdUnit4 command-line tool invoked for the unit
	// tier.
	GdUnit4CmdToolRes = "res://addons/gdUnit4/bin/GdUnitCmdTool.gd"

	// UnitTestsDirRes is the res:// unit suite root added when no suite filter
	// is selected.
	UnitTestsDirRes = "res://tests/unit/"

	// DebugBridgeIntegrationRunnerRes is the GDBG addon scene that
	// hosts IntegrationTestSuite cases.
	DebugBridgeIntegrationRunnerRes = "res://addons/gdbg/testing/integration_runner.tscn"
)

// gdUnit4PluginCfg is the checkout-relative gdUnit4 addon manifest.
func gdUnit4PluginCfg(projectDir string) string {
	return filepath.Join(projectDir, "addons", "gdUnit4", "plugin.cfg")
}

// gdUnit4CmdTool is the checkout-relative gdUnit4 command-line tool.
func gdUnit4CmdTool(projectDir string) string {
	return filepath.Join(projectDir, "addons", "gdUnit4", "bin", "GdUnitCmdTool.gd")
}

// unitTestsDir is the checkout-relative unit suite directory.
func unitTestsDir(projectDir string) string {
	return filepath.Join(projectDir, "tests", "unit")
}

// debugBridgePluginCfg is the checkout-relative GDBG addon manifest.
func debugBridgePluginCfg(projectDir string) string {
	return filepath.Join(projectDir, "addons", "gdbg", "plugin.cfg")
}

// debugBridgeIntegrationRunner is the checkout-relative GDBG addon
// integration runner scene.
func debugBridgeIntegrationRunner(projectDir string) string {
	return filepath.Join(projectDir, "addons", "gdbg", "testing", "integration_runner.tscn")
}

// debugBridgePlaytestRuntime is the checkout-relative GDBG addon
// automation runtime required by the playtest tier.
func debugBridgePlaytestRuntime(projectDir string) string {
	return filepath.Join(projectDir, "addons", "gdbg", "testing", "test_runtime.gd")
}

// integrationTestsDir is the checkout-relative integration suite directory.
func integrationTestsDir(projectDir string) string {
	return filepath.Join(projectDir, "tests", "integration")
}

// playtestTestsDir is the checkout-relative playtest suite directory.
func playtestTestsDir(projectDir string) string {
	return filepath.Join(projectDir, "tests", "playtest")
}
