// godot-debug-bridge/cli/internal/testrunner/integration_args_test.go
package testrunner

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBuildIntegrationGodotArgs(t *testing.T) {
	cases := []struct {
		name       string
		projectDir string
		resPath    string
		want       []string
	}{
		{
			name:       "gameplay suite",
			projectDir: "/proj",
			resPath:    "res://tests/integration/gameplay/test_flow.gd",
			want: []string{
				"--headless",
				"--path", "/proj",
				"--fixed-fps", "60",
				DebugBridgeIntegrationRunnerRes,
				"--",
				"--integration=res://tests/integration/gameplay/test_flow.gd",
			},
		},
		{
			name:       "root-level suite",
			projectDir: "/proj",
			resPath:    "res://tests/integration/test_combat.gd",
			want: []string{
				"--headless",
				"--path", "/proj",
				"--fixed-fps", "60",
				DebugBridgeIntegrationRunnerRes,
				"--",
				"--integration=res://tests/integration/test_combat.gd",
			},
		},
		{
			name:       "nested project path",
			projectDir: "/work/projects/game",
			resPath:    "res://tests/integration/ai/test_steering.gd",
			want: []string{
				"--headless",
				"--path", "/work/projects/game",
				"--fixed-fps", "60",
				DebugBridgeIntegrationRunnerRes,
				"--",
				"--integration=res://tests/integration/ai/test_steering.gd",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := buildIntegrationGodotArgs(tc.projectDir, tc.resPath)
			if len(got) != len(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Fatalf("arg[%d] = %q, want %q (full args: %v)", i, got[i], tc.want[i], got)
				}
			}
			for _, arg := range got {
				if arg == "res://scenes/tests/integration.tscn" {
					t.Fatalf("legacy project integration runner must not be used: %v", got)
				}
			}
		})
	}
}

func TestDiscoverIntegrationTestSuitesRootSuite(t *testing.T) {
	projectDir := t.TempDir()
	dir := filepath.Join(projectDir, "tests", "integration")
	suite := "extends IntegrationTestSuite\n"
	for rel, content := range map[string]string{
		"test_combat.gd":        suite,
		"helper.gd":             "extends Node\n",
		"gameplay/test_flow.gd": suite,
	} {
		full := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	got, err := discoverIntegrationTestSuites(dir, []string{RootSuite})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || filepath.Base(got[0]) != "test_combat.gd" {
		t.Fatalf("root suite = %v, want only the top-level test_combat.gd", got)
	}
	if !IntegrationSuiteExists(projectDir, RootSuite) || !IntegrationSuiteExists(projectDir, "gameplay") {
		t.Fatal("root and first-level directories must be discoverable")
	}
	if IntegrationSuiteExists(t.TempDir(), RootSuite) {
		t.Fatal("root must not exist without top-level integration suites")
	}
}
