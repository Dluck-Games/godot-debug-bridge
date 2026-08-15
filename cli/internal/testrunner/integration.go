// godot-debug-bridge/cli/internal/testrunner/integration.go
package testrunner

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type IntegrationTestResult struct {
	Name     string
	Passed   bool
	ExitCode int
}

func RunIntegrationTests(godotBin, projectDir string, opts RunOptions) ([]IntegrationTestResult, error) {
	integrationDir := integrationTestsDir(projectDir)
	suites, err := discoverIntegrationTestSuites(integrationDir, opts.Suite)
	if err != nil {
		return nil, err
	}

	var results []IntegrationTestResult
	for _, suitePath := range suites {
		name := strings.TrimSuffix(filepath.Base(suitePath), ".gd")
		relPath, _ := filepath.Rel(projectDir, suitePath)
		resPath := "res://" + filepath.ToSlash(relPath)

		if opts.Verbose {
			fmt.Fprintf(os.Stderr, "  %s ... ", name)
		}

		cmd := exec.Command(godotBin, buildIntegrationGodotArgs(projectDir, resPath)...)
		cmd.Dir = projectDir

		var output bytes.Buffer
		if opts.Verbose {
			cmd.Stdout = os.Stderr
			cmd.Stderr = os.Stderr
		} else {
			cmd.Stdout = &output
			cmd.Stderr = &output
		}

		err := cmd.Run()
		exitCode := 0
		passed := true
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				exitCode = exitErr.ExitCode()
			}
			passed = false
		}

		if opts.Verbose {
			if passed {
				fmt.Fprintln(os.Stderr, "PASS")
			} else {
				fmt.Fprintln(os.Stderr, "FAIL")
			}
		} else if !passed {
			if output.Len() > 0 {
				fmt.Fprint(os.Stderr, output.String())
			}
			fmt.Fprintf(os.Stderr, "  [FAIL] %s\n", name)
		}

		results = append(results, IntegrationTestResult{
			Name:     name,
			Passed:   passed,
			ExitCode: exitCode,
		})
	}

	return results, nil
}

// buildIntegrationGodotArgs builds the Godot invocation that runs a single
// integration case through the GDBG addon runner scene.
func buildIntegrationGodotArgs(projectDir, resPath string) []string {
	return []string{
		"--headless",
		"--path", projectDir,
		"--fixed-fps", "60",
		DebugBridgeIntegrationRunnerRes,
		"--",
		"--integration=" + resPath,
	}
}

func discoverIntegrationTestSuites(dir string, suiteFilter []string) ([]string, error) {
	var suites []string
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".gd") {
			return nil
		}
		if isIntegrationTestSuite(path) {
			// Apply suite filter
			if len(suiteFilter) > 0 {
				rel, _ := filepath.Rel(dir, path)
				matched := false
				for _, s := range suiteFilter {
					// Match by subdirectory prefix: "gameplay/test_flow.gd" matches suite "gameplay"
					if strings.HasPrefix(filepath.ToSlash(rel), s+"/") {
						matched = true
						break
					}
					// Match by filename prefix: "test_combat.gd" matches suite "combat"
					if strings.HasPrefix(filepath.Base(rel), "test_"+s) {
						matched = true
						break
					}
				}
				if !matched {
					return nil
				}
			}
			suites = append(suites, path)
		}
		return nil
	})
	return suites, err
}

func isIntegrationTestSuite(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "extends IntegrationTestSuite") ||
			strings.HasPrefix(line, "extends DebugBridgeIntegrationTestSuite") ||
			strings.Contains(line, "addons/gdbg/testing/integration_test_suite.gd") {
			return true
		}
		// Only bail early if we see a non-GDBG integration-suite extends line;
		// class_name may appear before extends, so keep scanning.
		if strings.HasPrefix(line, "extends ") {
			break
		}
	}
	return false
}
