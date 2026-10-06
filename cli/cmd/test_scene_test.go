// godot-debug-bridge/cli/cmd/test_scene_test.go
package cmd

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Dluck-Games/godot-debug-bridge/cli/internal/debug"
)

func TestValidateSceneRes(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{name: "valid tscn", input: "res://tests/smoke.tscn"},
		{name: "valid scn", input: "res://scenes/level.scn"},
		{name: "valid nested", input: "res://a/b/c.scn"},
		{name: "missing res prefix", input: "tests/smoke.tscn", want: "res://"},
		{name: "uppercase scheme", input: "RES://tests/smoke.tscn", want: "res://"},
		{name: "wrong extension", input: "res://tests/smoke.gd", want: ".tscn or .scn"},
		{name: "no extension", input: "res://tests/smoke", want: ".tscn or .scn"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateSceneRes(tc.input)
			if tc.want == "" {
				if err != nil {
					t.Fatalf("validateSceneRes(%q) = %v, want nil", tc.input, err)
				}
				return
			}
			if err == nil {
				t.Fatalf("validateSceneRes(%q) = nil, want error containing %q", tc.input, tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("validateSceneRes(%q) error = %q, want substring %q", tc.input, err, tc.want)
			}
		})
	}
}

func TestResolveScenePath(t *testing.T) {
	project := t.TempDir()
	if err := os.MkdirAll(filepath.Join(project, "tests"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "tests", "smoke.tscn"), []byte("[gd_scene load_steps=1 format=3]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "tests", "smoke.scn"), []byte("[]"), 0o644); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name string
		arg  string
		want string
	}{
		{name: "tscn", arg: "res://tests/smoke.tscn", want: filepath.Join(project, "tests", "smoke.tscn")},
		{name: "scn", arg: "res://tests/smoke.scn", want: filepath.Join(project, "tests", "smoke.scn")},
		{name: "dot segment cleaned", arg: "res://tests/../tests/smoke.tscn", want: filepath.Join(project, "tests", "smoke.tscn")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveScenePath(project, tc.arg)
			if err != nil {
				t.Fatalf("resolveScenePath(%q) = %v", tc.arg, err)
			}
			if got != tc.want {
				t.Fatalf("resolveScenePath(%q) = %q, want %q", tc.arg, got, tc.want)
			}
		})
	}
}

func TestResolveScenePathRejectsTraversal(t *testing.T) {
	project := t.TempDir()
	for _, arg := range []string{
		"res://../outside.tscn",
		"res://tests/../../esc.tscn",
		"res://..",
		"res://../",
	} {
		t.Run(arg, func(t *testing.T) {
			_, err := resolveScenePath(project, arg)
			if err == nil {
				t.Fatalf("resolveScenePath(%q) = nil error, want traversal rejection", arg)
			}
			if !strings.Contains(err.Error(), "escape") {
				t.Fatalf("resolveScenePath(%q) error = %q, want escape rejection", arg, err)
			}
		})
	}
}

func TestResolveScenePathMissingFile(t *testing.T) {
	project := t.TempDir()
	_, err := resolveScenePath(project, "res://tests/missing.tscn")
	if err == nil {
		t.Fatal("resolveScenePath returned nil error for a missing scene")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("resolveScenePath error = %q, want not-found error", err)
	}
}

func TestSplitSceneArgs(t *testing.T) {
	cases := []struct {
		name      string
		args      []string
		dashIndex int
		wantScene string
		wantUser  []string
		wantErr   string
	}{
		{name: "scene only", args: []string{"res://tests/smoke.tscn"}, dashIndex: -1, wantScene: "res://tests/smoke.tscn"},
		{name: "user args after dash", args: []string{"res://tests/smoke.tscn", "--json", "--duration=60"}, dashIndex: 1, wantScene: "res://tests/smoke.tscn", wantUser: []string{"--json", "--duration=60"}},
		{name: "empty user args after separator", args: []string{"res://tests/smoke.tscn"}, dashIndex: 1, wantScene: "res://tests/smoke.tscn", wantUser: []string{}},
		{name: "no scene", args: nil, dashIndex: -1, wantErr: "scene path required"},
		{name: "dash first", args: []string{"--json"}, dashIndex: 0, wantErr: "before '--'"},
		{name: "extra positional", args: []string{"res://a.tscn", "res://b.tscn"}, dashIndex: -1, wantErr: "unexpected extra arguments"},
		{name: "extra before dash", args: []string{"res://a.tscn", "res://b.tscn", "--json"}, dashIndex: 2, wantErr: "before '--'"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			scene, user, err := splitSceneArgs(tc.args, tc.dashIndex)
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("splitSceneArgs(%v) = nil error, want %q", tc.args, tc.wantErr)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("splitSceneArgs(%v) error = %q, want substring %q", tc.args, err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("splitSceneArgs(%v) = %v", tc.args, err)
			}
			if scene != tc.wantScene {
				t.Fatalf("splitSceneArgs(%v) scene = %q, want %q", tc.args, scene, tc.wantScene)
			}
			if !reflect.DeepEqual(user, tc.wantUser) {
				t.Fatalf("splitSceneArgs(%v) user args = %v, want %v", tc.args, user, tc.wantUser)
			}
		})
	}
}

func TestBuildSceneGodotArgs(t *testing.T) {
	cases := []struct {
		name     string
		scene    string
		userArgs []string
		verbose  bool
		want     []string
	}{
		{
			name:  "headless by default",
			scene: "res://tests/smoke.tscn",
			want:  []string{"--headless", "res://tests/smoke.tscn"},
		},
		{
			name:     "forwards user args after dash dash",
			scene:    "res://tests/smoke.tscn",
			userArgs: []string{"--json", "--duration=60"},
			want:     []string{"--headless", "res://tests/smoke.tscn", "--", "--json", "--duration=60"},
		},
		{
			name:    "engine verbose before scene",
			scene:   "res://tests/smoke.tscn",
			verbose: true,
			want:    []string{"--headless", "--verbose", "res://tests/smoke.tscn"},
		},
		{
			name:     "engine verbose before scene and before separator",
			scene:    "res://tests/smoke.tscn",
			userArgs: []string{"--json", "--duration=60"},
			verbose:  true,
			want:     []string{"--headless", "--verbose", "res://tests/smoke.tscn", "--", "--json", "--duration=60"},
		},
		{
			name:     "user verbose after separator stays a user arg",
			scene:    "res://tests/smoke.tscn",
			userArgs: []string{"--verbose", "--json"},
			verbose:  false,
			want:     []string{"--headless", "res://tests/smoke.tscn", "--", "--verbose", "--json"},
		},
		{
			name:     "engine and user verbose remain distinct",
			scene:    "res://tests/smoke.tscn",
			userArgs: []string{"--verbose"},
			verbose:  true,
			want:     []string{"--headless", "--verbose", "res://tests/smoke.tscn", "--", "--verbose"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := buildSceneGodotArgs(tc.scene, tc.userArgs, tc.verbose)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("buildSceneGodotArgs(%q, %v, %v) = %v, want %v", tc.scene, tc.userArgs, tc.verbose, got, tc.want)
			}
		})
	}
}

func TestWithGDBGState(t *testing.T) {
	stateRoot := t.TempDir()
	t.Setenv("GDBG_STATE", stateRoot)
	t.Setenv("XDG_STATE_HOME", "/must-not-win")

	env := withGDBGState([]string{"PATH=/usr/bin", "GDBG_STATE=stale"})
	want := "GDBG_STATE=" + debug.StateBase()
	count := 0
	for _, entry := range env {
		if strings.HasPrefix(entry, "GDBG_STATE=") {
			count++
			if entry != want {
				t.Fatalf("GDBG_STATE = %q, want %q", entry, want)
			}
		}
	}
	if count != 1 {
		t.Fatalf("GDBG_STATE entries = %d, want exactly 1: %v", count, env)
	}
}
