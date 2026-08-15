// godot-debug-bridge/cli/cmd/test_scene.go
package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Dluck-Games/godot-debug-bridge/cli/internal/debug"
	"github.com/Dluck-Games/godot-debug-bridge/cli/internal/godot"
)

var testSceneCmd = &cobra.Command{
	Use:   "scene <res://scene> [-- <godot user args...>]",
	Short: "Run a single project-owned scene headless",
	Long: `Run one scene from the selected project in headless mode.

The scene must be a project-relative res:// path to a .tscn or .scn file
inside the selected project. Godot is discovered via GODOT_PATH or the
platform default locations, the import cache is ensured, stdout/stderr are
streamed to the terminal, and GDBG_STATE is injected into the child process.

Arguments after '--' are forwarded to Godot as user arguments; the scene
reads them with OS.get_cmdline_user_args().

Examples:
  gdbg --project-dir /game test scene res://tests/smoke.tscn
  gdbg --project-dir /game test scene res://bench/scenario.tscn -- --json --duration=60`,
	Args:         cobra.ArbitraryArgs,
	SilenceUsage: true,
	RunE:         testSceneRun,
}

func init() {
	testCmd.AddCommand(testSceneCmd)
}

func testSceneRun(cmd *cobra.Command, args []string) error {
	sceneRes, userArgs, err := splitSceneArgs(args, cmd.ArgsLenAtDash())
	if err != nil {
		return err
	}

	projectDir, err := resolveProject()
	if err != nil {
		return err
	}

	if _, err := resolveScenePath(projectDir, sceneRes); err != nil {
		return err
	}

	godotBin, err := godot.FindBinary()
	if err != nil {
		return err
	}

	if err := godot.EnsureImportCache(godotBin, projectDir); err != nil {
		return err
	}

	return runTestScene(godotBin, projectDir, sceneRes, userArgs)
}

// runTestScene launches Godot with the scene, streaming stdio and injecting
// GDBG_STATE. A nonzero Godot exit is returned as an error so the CLI exits
// with a nonzero status.
func runTestScene(godotBin, projectDir, sceneRes string, userArgs []string) error {
	child := exec.Command(godotBin, buildSceneGodotArgs(sceneRes, userArgs)...)
	child.Dir = projectDir
	child.Stdin = os.Stdin
	child.Stdout = os.Stdout
	child.Stderr = os.Stderr
	child.Env = withGDBGState(os.Environ())
	return child.Run()
}

// splitSceneArgs separates the single scene path from the Godot user arguments
// that follow '--'. dashIndex is cmd.ArgsLenAtDash(): the number of arguments
// before the '--' separator, or -1 when there is no separator.
func splitSceneArgs(args []string, dashIndex int) (string, []string, error) {
	if len(args) == 0 {
		return "", nil, fmt.Errorf("scene path required; use e.g. 'gdbg test scene res://tests/smoke.tscn'")
	}
	if dashIndex == -1 {
		if len(args) > 1 {
			return "", nil, fmt.Errorf("unexpected extra arguments %v; pass Godot user arguments after '--'", args[1:])
		}
		return args[0], nil, nil
	}
	if dashIndex == 0 {
		return "", nil, fmt.Errorf("scene path required before '--'")
	}
	if dashIndex != 1 {
		return "", nil, fmt.Errorf("unexpected extra arguments %v before '--'; provide a single scene path", args[1:dashIndex])
	}
	return args[0], args[1:], nil
}

// validateSceneRes checks that a scene reference is a project-relative res://
// path to a .tscn or .scn file.
func validateSceneRes(sceneArg string) error {
	if !strings.HasPrefix(sceneArg, "res://") {
		return fmt.Errorf("scene path must be a project-relative res:// path, got %q", sceneArg)
	}
	ext := strings.ToLower(filepath.Ext(sceneArg))
	if ext != ".tscn" && ext != ".scn" {
		return fmt.Errorf("scene path must reference a .tscn or .scn file, got %q", sceneArg)
	}
	return nil
}

// resolveScenePath maps a res:// scene reference to an absolute path inside
// projectDir, rejecting paths that escape the project and missing files.
// Traversal is detected before extension validation so escaping paths are
// always reported as escapes, never as merely invalid scene files.
func resolveScenePath(projectDir, sceneArg string) (string, error) {
	if !strings.HasPrefix(sceneArg, "res://") {
		return "", fmt.Errorf("scene path must be a project-relative res:// path, got %q", sceneArg)
	}
	abs := filepath.Join(projectDir, filepath.FromSlash(strings.TrimPrefix(sceneArg, "res://")))
	if !isWithinProject(projectDir, abs) {
		return "", fmt.Errorf("scene path %q escapes the project directory", sceneArg)
	}
	ext := strings.ToLower(filepath.Ext(sceneArg))
	if ext != ".tscn" && ext != ".scn" {
		return "", fmt.Errorf("scene path must reference a .tscn or .scn file, got %q", sceneArg)
	}
	if _, err := os.Stat(abs); err != nil {
		return "", fmt.Errorf("scene file not found: %s", sceneArg)
	}
	return abs, nil
}

// isWithinProject reports whether target is located inside base.
func isWithinProject(base, target string) bool {
	rel, err := filepath.Rel(base, target)
	if err != nil {
		return false
	}
	return rel != ".." &&
		!strings.HasPrefix(rel, ".."+string(filepath.Separator)) &&
		!filepath.IsAbs(rel)
}

// buildSceneGodotArgs assembles the Godot command line: headless by default,
// the scene path, then a '--' separator followed by the user arguments.
func buildSceneGodotArgs(sceneRes string, userArgs []string) []string {
	args := []string{"--headless", sceneRes}
	if len(userArgs) > 0 {
		args = append(args, "--")
		args = append(args, userArgs...)
	}
	return args
}

// withGDBGState returns env with a single authoritative GDBG_STATE entry from
// debug.StateBase, replacing any inherited value.
func withGDBGState(env []string) []string {
	out := make([]string, 0, len(env)+1)
	for _, entry := range env {
		if !strings.HasPrefix(entry, "GDBG_STATE=") {
			out = append(out, entry)
		}
	}
	return append(out, "GDBG_STATE="+debug.StateBase())
}
