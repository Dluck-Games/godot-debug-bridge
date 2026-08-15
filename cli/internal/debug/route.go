package debug

// RouteType discriminates how a gdbg debug invocation is handled.
type RouteType string

const (
	RouteBridge  RouteType = "bridge"
	RouteRecord  RouteType = "record"
	RouteScript  RouteType = "script"
	RouteError   RouteType = "error"
	RouteMigrate RouteType = "migrate"
)

type Route struct {
	Type    RouteType
	Payload map[string]any
	Path    string
	Options *RecordOptions
	Message string
}

// ResolveCommand handles bridge-owned commands locally and forwards every
// other command unchanged to the project's DebugBridgeHost. GDBG deliberately
// does not reserve game vocabulary: projects own and extend their commands.
func ResolveCommand(command string, positional []string, f Flags) Route {
	switch command {
	case "screenshot":
		return Route{Type: RouteBridge, Payload: map[string]any{
			"cmd": "screenshot", "args": ScreenshotOpts(f),
		}}
	case "record":
		if len(positional) > 0 {
			return Route{Type: RouteError, Message: "record does not accept positional arguments"}
		}
		opts, err := BuildRecordOpts(f)
		if err != nil {
			return Route{Type: RouteError, Message: err.Error()}
		}
		return Route{Type: RouteRecord, Options: &opts}
	case "fetch":
		if len(positional) == 0 {
			return Route{Type: RouteError, Message: "capture_id required"}
		}
		return Route{Type: RouteBridge, Payload: map[string]any{
			"cmd": "fetch", "args": map[string]any{"capture_id": positional[0]},
		}}
	case "script":
		if len(positional) == 0 {
			return Route{Type: RouteError, Message: "script file required"}
		}
		return Route{Type: RouteScript, Path: positional[0]}
	case "input":
		return inputRoute(positional)
	case "boot":
		return Route{Type: RouteMigrate, Message: "gdbg debug boot was removed — use 'gdbg run game'"}
	case "teardown":
		return Route{Type: RouteMigrate, Message: "gdbg debug teardown was removed — use 'gdbg stop'"}
	case "reimport":
		return Route{Type: RouteMigrate, Message: "gdbg debug reimport was removed — use 'gdbg reimport'"}
	}

	actualCommand := command
	actualArgs := positional
	if command == "console" {
		if len(positional) == 0 {
			return Route{Type: RouteError, Message: "console command required"}
		}
		actualCommand = positional[0]
		actualArgs = positional[1:]
	}
	payload := map[string]any{"cmd": actualCommand, "args": actualArgs}
	if f.Screenshot {
		payload["screenshot"] = ScreenshotOpts(f)
	}
	return Route{Type: RouteBridge, Payload: payload}
}
