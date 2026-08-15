// godot-debug-bridge/cli/internal/testrunner/preflight.go
package testrunner

import (
	"fmt"
	"os"
	"path/filepath"
)

// Preflight validates the dependencies of every selected tier before any Godot
// process is launched. It only inspects regular files and directories on disk;
// it never inspects editor plugin enable state and never falls back to
// alternative paths. Errors name the tier, dependency, project-relative path
// and the install or update action to take.
func Preflight(projectDir string, tiers []Tier) error {
	seen := make(map[Tier]bool, len(tiers))
	for _, t := range tiers {
		if seen[t] {
			continue
		}
		seen[t] = true
		switch t {
		case TierUnit:
			if err := preflightUnit(projectDir); err != nil {
				return err
			}
		case TierIntegration:
			if err := preflightIntegration(projectDir); err != nil {
				return err
			}
		case TierPlaytest:
			if err := preflightPlaytest(projectDir); err != nil {
				return err
			}
		}
	}
	return nil
}

func preflightUnit(projectDir string) error {
	return runPreflightChecks(projectDir, []preflightRequirement{
		{
			tier:    "unit",
			dep:     "gdUnit4 addon",
			rel:     "addons/gdUnit4/plugin.cfg",
			isDir:   false,
			install: "install the gdUnit4 v4 addon into addons/gdUnit4/",
			update:  "reinstall gdUnit4 so addons/gdUnit4/plugin.cfg is a regular file",
		},
		{
			tier:    "unit",
			dep:     "gdUnit4 runner",
			rel:     "addons/gdUnit4/bin/GdUnitCmdTool.gd",
			isDir:   false,
			install: "update gdUnit4 to a version that ships addons/gdUnit4/bin/GdUnitCmdTool.gd",
			update:  "update gdUnit4 so addons/gdUnit4/bin/GdUnitCmdTool.gd is a regular file",
		},
		{
			tier:    "unit",
			dep:     "unit tests directory",
			rel:     "tests/unit",
			isDir:   true,
			install: "create tests/unit/ and place your unit test suites there",
			update:  "tests/unit must be a directory",
		},
	})
}

func preflightIntegration(projectDir string) error {
	return runPreflightChecks(projectDir, []preflightRequirement{
		{
			tier:    "integration",
			dep:     "GDBG addon",
			rel:     "addons/gdbg/plugin.cfg",
			isDir:   false,
			install: "install the GDBG addon into addons/gdbg/",
			update:  "reinstall the GDBG addon so addons/gdbg/plugin.cfg is a regular file",
		},
		{
			tier:    "integration",
			dep:     "GDBG integration runner",
			rel:     "addons/gdbg/testing/integration_runner.tscn",
			isDir:   false,
			install: "update the GDBG addon to a version that ships addons/gdbg/testing/integration_runner.tscn",
			update:  "update the GDBG addon so addons/gdbg/testing/integration_runner.tscn is a regular file",
		},
		{
			tier:    "integration",
			dep:     "integration tests directory",
			rel:     "tests/integration",
			isDir:   true,
			install: "create tests/integration/ and place your integration suites there",
			update:  "tests/integration must be a directory",
		},
	})
}

func preflightPlaytest(projectDir string) error {
	return runPreflightChecks(projectDir, []preflightRequirement{
		{
			tier:    "playtest",
			dep:     "GDBG addon",
			rel:     "addons/gdbg/plugin.cfg",
			isDir:   false,
			install: "install the GDBG addon into addons/gdbg/",
			update:  "reinstall the GDBG addon so addons/gdbg/plugin.cfg is a regular file",
		},
		{
			tier:    "playtest",
			dep:     "GDBG playtest runtime",
			rel:     "addons/gdbg/testing/test_runtime.gd",
			isDir:   false,
			install: "update the GDBG addon to a version that ships addons/gdbg/testing/test_runtime.gd",
			update:  "update the GDBG addon so addons/gdbg/testing/test_runtime.gd is a regular file",
		},
		{
			tier:    "playtest",
			dep:     "playtest tests directory",
			rel:     "tests/playtest",
			isDir:   true,
			install: "create tests/playtest/ and place your playtest suites there",
			update:  "tests/playtest must be a directory",
		},
	})
}

// preflightRequirement is a single on-disk dependency: either a regular file or
// a directory, expressed as a project-relative slash-separated path.
type preflightRequirement struct {
	tier    string // tier name used in error messages ("unit", ...)
	dep     string // human-readable dependency name
	rel     string // project-relative path
	isDir   bool   // true when the dependency must be a directory
	install string // guidance when the path is absent
	update  string // guidance when the path exists with the wrong type
}

func runPreflightChecks(projectDir string, reqs []preflightRequirement) error {
	for i, req := range reqs {
		info, err := os.Stat(filepath.Join(projectDir, filepath.FromSlash(req.rel)))
		if err != nil {
			if os.IsNotExist(err) {
				category := "not installed"
				if i > 0 {
					category = "incomplete"
				}
				return fmt.Errorf("%s: %s %s: missing %s — %s", req.tier, req.dep, category, req.rel, req.install)
			}
			return fmt.Errorf("%s: %s cannot be checked at %s: %v", req.tier, req.dep, req.rel, err)
		}
		if req.isDir && !info.IsDir() {
			return fmt.Errorf("%s: %s wrong type: %s exists but is not a directory — %s", req.tier, req.dep, req.rel, req.update)
		}
		if !req.isDir && !info.Mode().IsRegular() {
			return fmt.Errorf("%s: %s wrong type: %s exists but is not a regular file — %s", req.tier, req.dep, req.rel, req.update)
		}
	}
	return nil
}
