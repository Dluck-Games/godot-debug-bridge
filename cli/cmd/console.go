package cmd

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Dluck-Games/godot-debug-bridge/cli/internal/debug"
)

var consoleCmd = &cobra.Command{
	Use:                "console <command> [args...]",
	Short:              "Run a project-defined console command in the game",
	DisableFlagParsing: true,
	SilenceErrors:      true,
	SilenceUsage:       true,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
			fmt.Fprintln(cmd.OutOrStdout(), "Usage: gdbg console <command> [args...] [--screenshot ...]")
			fmt.Fprintln(cmd.OutOrStdout(), "Commands are defined by the Godot project's DebugBridgeHost modules.")
			if len(args) == 0 {
				return errors.New("console command required")
			}
			return nil
		}
		forwarded := append([]string{"console"}, args...)
		return debug.Run(forwarded, cmd.OutOrStdout(), cmd.ErrOrStderr())
	},
}

func init() {
	rootCmd.AddCommand(consoleCmd)
}
