// godot-debug-bridge/cli/internal/debug/run.go
package debug

import (
	"errors"
	"fmt"
	"io"
	"time"
)

// Run executes a native `gdbg debug` invocation. It prints all user-visible
// output to stdout/stderr and returns a non-nil error whenever the invocation
// should exit non-zero (usage errors, removed commands, bridge failures, and
// game-reported failures). Callers that do not want cobra to echo the returned
// error again should silence error printing on their command.
func Run(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		fmt.Fprint(stdout, HelpText)
		return nil
	}
	if isRecordHelp(args) {
		fmt.Fprint(stdout, RecordHelpText)
		return nil
	}

	command := args[0]
	flags, positional, err := ParseFlags(args[1:])
	if err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return err
	}

	route := ResolveCommand(command, positional, flags)
	switch route.Type {
	case RouteError, RouteMigrate:
		fmt.Fprintf(stderr, "Error: %s\n", route.Message)
		return errors.New(route.Message)

	}

	var result any
	switch route.Type {
	case RouteBridge:
		result, err = runBridgeCommand(route, flags)
	case RouteScript:
		result, err = ExecuteScript(NewBridge(IPCDir()), route.Path)
	case RouteRecord:
		result, err = ExecuteRecord(bridgeSender(), *route.Options)
	default:
		err = fmt.Errorf("unhandled debug route %q", route.Type)
	}
	if err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return err
	}

	formatted := FormatResult(result)
	for _, line := range formatted.Stdout {
		fmt.Fprintln(stdout, line)
	}
	for _, line := range formatted.Stderr {
		fmt.Fprintln(stderr, line)
	}
	if IsFailureResult(result) {
		return errors.New("gdbg debug command failed")
	}
	return nil
}

func isRecordHelp(args []string) bool {
	if len(args) != 2 {
		return false
	}
	if args[0] == "record" && (args[1] == "--help" || args[1] == "-h") {
		return true
	}
	if args[0] == "help" && args[1] == "record" {
		return true
	}
	return false
}

// runBridgeCommand sends a bridge payload and picks the timeout by command
// kind, mirroring the old client: screenshots get the delay/count/interval
// extended timeout, everything else gets the default.
func runBridgeCommand(route Route, flags Flags) (any, error) {
	timeout := DefaultTimeout
	if cmd, ok := route.Payload["cmd"].(string); ok && cmd == "screenshot" {
		timeout = CalculateTimeout(flags)
	}
	return NewBridge(IPCDir()).Send(route.Payload, timeout)
}

// bridgeSender adapts Bridge.Send to the Sender signature used by record.
func bridgeSender() Sender {
	return func(payload map[string]any, timeout time.Duration) (any, error) {
		return NewBridge(IPCDir()).Send(payload, timeout)
	}
}
