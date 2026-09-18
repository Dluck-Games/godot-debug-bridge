package godot

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func EnsureImportCache(godotBin, projectDir string) error {
	fmt.Fprintf(os.Stderr, "Syncing imported assets with headless import...\n")
	return RunImport(godotBin, projectDir)
}

func RunImport(godotBin, projectDir string) error {
	cmd := exec.Command(godotBin, "--headless", "--import", "--path", projectDir)
	output, err := cmd.CombinedOutput()
	_, _ = os.Stderr.Write(output)
	if err != nil {
		return err
	}
	if marker := importFailureMarker(string(output)); marker != "" {
		return fmt.Errorf("Godot import reported script compilation failure (%s)", marker)
	}
	return nil
}

func importFailureMarker(output string) string {
	for _, marker := range []string{"SCRIPT ERROR:", "Parse Error:", "Compile Error:", "Failed to load script"} {
		if strings.Contains(output, marker) {
			return strings.TrimSuffix(marker, ":")
		}
	}
	return ""
}

func ClearImportCache(projectDir string) error {
	importedDir := filepath.Join(projectDir, ".godot", "imported")
	if _, err := os.Stat(importedDir); os.IsNotExist(err) {
		return nil
	}
	fmt.Fprintf(os.Stderr, "Clearing import cache...\n")
	return os.RemoveAll(importedDir)
}

var uidExtensions = map[string]bool{
	".gd":   true,
	".tscn": true,
	".tres": true,
}

var skipDirs = map[string]bool{
	".godot":       true,
	"node_modules": true,
	".git":         true,
}

func FindMissingUIDs(projectDir string) ([]string, error) {
	var missing []string
	err := filepath.WalkDir(projectDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && skipDirs[d.Name()] {
			return filepath.SkipDir
		}
		if d.IsDir() {
			return nil
		}
		ext := filepath.Ext(d.Name())
		if uidExtensions[ext] {
			uidPath := path + ".uid"
			if _, err := os.Stat(uidPath); os.IsNotExist(err) {
				rel, _ := filepath.Rel(projectDir, path)
				missing = append(missing, rel)
			}
		}
		return nil
	})
	return missing, err
}

func GenerateMissingUIDs(godotBin, projectDir string) (int, error) {
	missing, err := FindMissingUIDs(projectDir)
	if err != nil {
		return 0, err
	}
	if len(missing) == 0 {
		return 0, nil
	}
	fmt.Fprintf(os.Stderr, "Found %d files missing UIDs, running import to generate...\n", len(missing))
	if err := RunImport(godotBin, projectDir); err != nil {
		return 0, err
	}
	return len(missing), nil
}

func CleanOrphanedUIDs(projectDir string) ([]string, error) {
	var removed []string
	err := filepath.WalkDir(projectDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && skipDirs[d.Name()] {
			return filepath.SkipDir
		}
		if d.IsDir() {
			return nil
		}
		if strings.HasSuffix(d.Name(), ".uid") {
			sourcePath := strings.TrimSuffix(path, ".uid")
			if _, err := os.Stat(sourcePath); os.IsNotExist(err) {
				os.Remove(path)
				rel, _ := filepath.Rel(projectDir, path)
				removed = append(removed, rel)
			}
		}
		return nil
	})
	return removed, err
}
