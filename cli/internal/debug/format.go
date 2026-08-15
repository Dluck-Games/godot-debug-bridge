// godot-debug-bridge/cli/internal/debug/format.go
package debug

import (
	"fmt"
	"strings"
)

// Formatted is the split stdout/stderr rendering of a bridge result.
type Formatted struct {
	Stdout []string
	Stderr []string
}

// FormatResult renders a bridge result the same way the old client did:
//   - raw strings print verbatim
//   - a `result` field prints its value (arrays element by element)
//   - `capture_id` and pending progress print as status lines
//   - an error status prints to stderr
func FormatResult(result any) Formatted {
	if s, ok := result.(string); ok {
		return Formatted{Stdout: []string{s}}
	}

	m, ok := result.(map[string]any)
	if !ok {
		return Formatted{}
	}

	var out Formatted
	if v, exists := m["result"]; exists && v != nil {
		if arr, isArr := v.([]any); isArr {
			for _, e := range arr {
				out.Stdout = append(out.Stdout, fmt.Sprint(e))
			}
		} else {
			out.Stdout = append(out.Stdout, fmt.Sprint(v))
		}
	}

	if id, exists := m["capture_id"]; exists {
		out.Stdout = append(out.Stdout, "capture_id:"+fmt.Sprint(id))
	}

	if status, isStr := m["status"].(string); isStr && status == "pending" {
		progress := "unknown"
		if p, exists := m["progress"]; exists {
			progress = fmt.Sprint(p)
		}
		out.Stdout = append(out.Stdout, "status:pending progress:"+progress)
	}

	if status, isStr := m["status"].(string); isStr && status == "error" {
		if msg, exists := m["message"]; exists {
			out.Stderr = append(out.Stderr, "Error: "+fmt.Sprint(msg))
		}
	}

	return out
}

// IsFailureResult reports whether a bridge result represents a failure so the
// caller can exit non-zero instead of reporting success.
func IsFailureResult(result any) bool {
	if s, ok := result.(string); ok {
		for _, prefix := range []string{"TIMEOUT", "CRASH", "ERROR"} {
			if strings.HasPrefix(s, prefix) {
				return true
			}
		}
		return false
	}
	m, ok := result.(map[string]any)
	if !ok {
		return false
	}
	status, _ := m["status"].(string)
	return status == "error"
}
