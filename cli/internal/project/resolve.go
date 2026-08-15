package project

import (
	"errors"
	"os"
	"path/filepath"
)

func ResolveProjectDir(explicit string) (string, error) {
	if explicit != "" {
		if hasProjectGodot(explicit) {
			return filepath.Abs(explicit)
		}
		return "", errors.New("no project.godot at " + explicit)
	}

	if !isInteractive() {
		return "", errors.New("non-interactive mode requires --project-dir or GDBG_PROJECT_DIR")
	}

	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return walkUp(cwd)
}

func walkUp(dir string) (string, error) {
	current, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	for {
		if hasProjectGodot(current) {
			return current, nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", errors.New("project.godot not found in current directory tree")
		}
		current = parent
	}
}

func hasProjectGodot(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, "project.godot"))
	return err == nil
}

func isInteractive() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}
