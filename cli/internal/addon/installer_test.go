package addon

import (
	"archive/zip"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func newProjectAndSource(t *testing.T) (string, string) {
	t.Helper()
	projectDir := t.TempDir()
	writeFile(t, filepath.Join(projectDir, "project.godot"), "[application]\n\nconfig/name=\"Fixture\"\n")
	sourceDir := filepath.Join(t.TempDir(), "gdbg")
	writeFile(t, filepath.Join(sourceDir, "plugin.cfg"), "[plugin]\nname=\"Godot Debug Bridge\"\n")
	writeFile(t, filepath.Join(sourceDir, "plugin.gd"), "@tool\nextends EditorPlugin\n")
	return projectDir, sourceDir
}

func TestInstallFromSourceEnablesPlugin(t *testing.T) {
	projectDir, sourceDir := newProjectAndSource(t)
	result, err := Install(context.Background(), Options{ProjectDir: projectDir, SourceDir: sourceDir})
	if err != nil {
		t.Fatal(err)
	}
	if result.Version != "source" || !result.ProjectChanged {
		t.Fatalf("result = %#v", result)
	}
	if _, err := os.Stat(filepath.Join(projectDir, "addons", "gdbg", "plugin.cfg")); err != nil {
		t.Fatal(err)
	}
	project, err := os.ReadFile(filepath.Join(projectDir, "project.godot"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(project), PluginResourcePath) != 1 {
		t.Fatalf("project.godot = %s", project)
	}
	for _, autoload := range requiredAutoloads {
		if strings.Count(string(project), autoload.Name) != 1 || !strings.Contains(string(project), autoload.Path) {
			t.Fatalf("project.godot missing autoload %#v: %s", autoload, project)
		}
	}
}

func TestEnsureAutoloadsRejectsNameConflict(t *testing.T) {
	projectFile := filepath.Join(t.TempDir(), "project.godot")
	writeFile(t, projectFile, "[autoload]\n\nAIDebugBridge=\"*res://custom/bridge.gd\"\n")
	if _, err := EnsureAutoloads(projectFile); err == nil || !strings.Contains(err.Error(), "points elsewhere") {
		t.Fatalf("expected autoload conflict, got %v", err)
	}
}

func uidProjectFile(t *testing.T) string {
	t.Helper()
	projectDir := t.TempDir()
	projectFile := filepath.Join(projectDir, "project.godot")
	var autoloads strings.Builder
	autoloads.WriteString("[autoload]\n\n")
	for i, required := range requiredAutoloads {
		uid := fmt.Sprintf("uid://uid%d", i)
		writeFile(t, filepath.Join(projectDir, filepath.FromSlash(strings.TrimPrefix(required.Path, "res://")))+".uid", uid+"\n")
		autoloads.WriteString(required.Name + `="*` + uid + `"` + "\n")
	}
	writeFile(t, projectFile, autoloads.String())
	return projectFile
}

func TestEnsureAutoloadsAcceptsMatchingUIDReferences(t *testing.T) {
	projectFile := uidProjectFile(t)
	before, err := os.ReadFile(projectFile)
	if err != nil {
		t.Fatal(err)
	}
	changed, err := EnsureAutoloads(projectFile)
	if err != nil {
		t.Fatalf("expected UID autoloads to be accepted, got %v", err)
	}
	if changed {
		t.Fatal("expected UID-based autoloads to be left unchanged")
	}
	after, err := os.ReadFile(projectFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatalf("project.godot changed:\nbefore=%s\nafter=%s", before, after)
	}
}

func TestEnsureAutoloadsRejectsUnknownUID(t *testing.T) {
	projectFile := uidProjectFile(t)
	data, err := os.ReadFile(projectFile)
	if err != nil {
		t.Fatal(err)
	}
	corrupted := strings.Replace(string(data), "uid://uid0", "uid://unknown", 1)
	writeFile(t, projectFile, corrupted)
	if _, err := EnsureAutoloads(projectFile); err == nil || !strings.Contains(err.Error(), "points elsewhere") {
		t.Fatalf("expected conflict for unknown UID, got %v", err)
	}
}

func TestEnsureAutoloadsRejectsReplacedSidecar(t *testing.T) {
	for _, sidecar := range []string{"missing", "", "uid://\n", "not-a-uid\n"} {
		projectFile := uidProjectFile(t)
		target := filepath.Join(filepath.Dir(projectFile), filepath.FromSlash(strings.TrimPrefix(requiredAutoloads[0].Path, "res://"))) + ".uid"
		if sidecar != "missing" {
			writeFile(t, target, sidecar)
		} else if err := os.Remove(target); err != nil {
			t.Fatal(err)
		}
		if _, err := EnsureAutoloads(projectFile); err == nil || !strings.Contains(err.Error(), "points elsewhere") {
			t.Fatalf("sidecar %q: expected conflict, got %v", sidecar, err)
		}
	}
}

func TestInstallRequiresForceAndReplacesExistingAddon(t *testing.T) {
	projectDir, sourceDir := newProjectAndSource(t)
	writeFile(t, filepath.Join(projectDir, "addons", "gdbg", "old.txt"), "old")
	if _, err := Install(context.Background(), Options{ProjectDir: projectDir, SourceDir: sourceDir}); err == nil || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("expected --force error, got %v", err)
	}
	if _, err := Install(context.Background(), Options{ProjectDir: projectDir, SourceDir: sourceDir, Force: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(projectDir, "addons", "gdbg", "old.txt")); !os.IsNotExist(err) {
		t.Fatalf("old file still exists: %v", err)
	}
}

func TestEnablePluginPreservesExistingPlugins(t *testing.T) {
	projectFile := filepath.Join(t.TempDir(), "project.godot")
	writeFile(t, projectFile, "[editor_plugins]\n\nenabled=PackedStringArray(\"res://addons/example/plugin.cfg\")\n")
	changed, err := EnablePlugin(projectFile)
	if err != nil || !changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	data, _ := os.ReadFile(projectFile)
	content := string(data)
	if !strings.Contains(content, "res://addons/example/plugin.cfg") || !strings.Contains(content, PluginResourcePath) {
		t.Fatalf("project.godot = %s", content)
	}
	changed, err = EnablePlugin(projectFile)
	if err != nil || changed {
		t.Fatalf("second call changed=%v err=%v", changed, err)
	}
}

func TestExtractZipRejectsTraversal(t *testing.T) {
	archivePath := filepath.Join(t.TempDir(), "bad.zip")
	archive, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(archive)
	entry, err := writer.Create("../escape.txt")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = entry.Write([]byte("escape"))
	_ = writer.Close()
	_ = archive.Close()
	if err := extractZip(archivePath, filepath.Join(t.TempDir(), "out")); err == nil {
		t.Fatal("expected traversal archive to be rejected")
	}
}
