// godot-debug-bridge/cli/internal/debug/script.go
package debug

import (
	"fmt"
	"os"
)

// ExecuteScript stages the given .gd file as temp_script.gd inside the IPC
// directory and asks the game to run it. The game reads the script from the
// same absolute path, so no user:// alias is involved.
func ExecuteScript(b *Bridge, scriptPath string) (any, error) {
	if _, err := os.Stat(scriptPath); err != nil {
		return nil, fmt.Errorf("Script not found: %s", scriptPath)
	}

	if err := b.ensureDir(); err != nil {
		return nil, err
	}

	content, err := os.ReadFile(scriptPath)
	if err != nil {
		return nil, fmt.Errorf("cannot read script %s: %w", scriptPath, err)
	}
	if err := os.WriteFile(b.ScriptPath(), content, 0o644); err != nil {
		return nil, fmt.Errorf("cannot stage script at %s: %w", b.ScriptPath(), err)
	}

	return b.Send(map[string]any{"cmd": "script"}, ScriptTimeout)
}
