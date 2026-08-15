// godot-debug-bridge/cli/cmd/debug.go
package cmd

import (
	"github.com/spf13/cobra"

	"github.com/Dluck-Games/godot-debug-bridge/cli/internal/debug"
)

var debugCmd = &cobra.Command{
	Use:   "debug [command] [args...]",
	Short: "Debug a running game via the native Go AI Debug Bridge",
	Long: `Send commands to a running game through the file-based AI Debug Bridge under
$GDBG_STATE/debug/ipc. Supports screenshots, console commands, eval, script
injection, input, and performance profiling. Runs entirely in-process with no
Node.js or external debug package required.`,
	DisableFlagParsing: true,
	// Errors are printed by the debug package with exact formatting; keep cobra
	// from echoing them a second time.
	SilenceErrors: true,
	SilenceUsage:  true,
	RunE:          debugRun,
}

func init() {
	rootCmd.AddCommand(debugCmd)
}

func debugRun(cmd *cobra.Command, args []string) error {
	return debug.Run(args, cmd.OutOrStdout(), cmd.ErrOrStderr())
}
