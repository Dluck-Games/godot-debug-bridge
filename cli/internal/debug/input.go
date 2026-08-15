// godot-debug-bridge/cli/internal/debug/input.go
package debug

import (
	"fmt"
	"math"
	"slices"
	"strings"
)

// inputOps are the supported input injection operations, forwarded verbatim to
// the in-game AIDebugBridge input command.
var inputOps = []string{"actions", "press", "release", "tap", "hold"}

// inputRoute builds the bridge payload (or an error route) for the `input`
// debug command: `gdbg debug input <op> [action] [duration]`.
func inputRoute(positional []string) Route {
	if len(positional) == 0 {
		return Route{Type: RouteError, Message: "input op required"}
	}

	op := positional[0]
	if !slices.Contains(inputOps, op) {
		return Route{
			Type:    RouteError,
			Message: fmt.Sprintf("Unknown input op: %s. Valid: %s", op, strings.Join(inputOps, ", ")),
		}
	}

	action := ""
	if len(positional) > 1 {
		action = positional[1]
	}
	if op != "actions" && action == "" {
		return Route{
			Type:    RouteError,
			Message: fmt.Sprintf("input %s requires an action", op),
		}
	}

	args := map[string]any{"op": op}
	if action != "" {
		args["action"] = action
	}
	if op == "hold" && len(positional) > 2 {
		d := parseFloat(positional[2])
		if math.IsNaN(d) {
			d = 0
		}
		args["duration"] = d
	}

	return Route{
		Type:    RouteBridge,
		Payload: map[string]any{"cmd": "input", "args": args},
	}
}
