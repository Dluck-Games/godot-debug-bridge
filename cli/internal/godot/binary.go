// godot-debug-bridge/cli/internal/godot/binary.go
package godot

import (
	"errors"
	"os"
	"os/exec"
	"runtime"
)

func FindBinary() (string, error) {
	if p := os.Getenv("GODOT_PATH"); p != "" {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}

	for _, p := range platformPaths() {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}

	if p, err := exec.LookPath("godot"); err == nil {
		return p, nil
	}

	return "", errors.New("Godot not found. Set GODOT_PATH or install Godot")
}

func platformPaths() []string {
	switch runtime.GOOS {
	case "darwin":
		return []string{
			"/Applications/Godot.app/Contents/MacOS/Godot",
		}
	case "windows":
		home := os.Getenv("USERPROFILE")
		return []string{
			`C:\Program Files (x86)\Steam\steamapps\common\Godot Engine\godot.windows.opt.tools.64.exe`,
			`C:\Program Files (x86)\Steam\steamapps\common\Godot Engine\godot.exe`,
			home + `\.local\bin\godot.bat`,
			`C:\Program Files\Godot\Godot.exe`,
			`C:\Program Files (x86)\Godot\Godot.exe`,
		}
	default:
		return nil
	}
}
