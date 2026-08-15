// godot-debug-bridge/cli/cmd/stop.go
package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Dluck-Games/godot-debug-bridge/cli/internal/godot"
)

var stopCmd = &cobra.Command{
	Use:       "stop [game|editor]",
	Short:     "Stop Godot processes",
	Long:      "Stop game, editor, or all Godot processes. Reads PID files from .godot/ directory.",
	Args:      cobra.MaximumNArgs(1),
	ValidArgs: []string{"game", "editor"},
	RunE:      stopRun,
}

func init() {
	rootCmd.AddCommand(stopCmd)
}

func stopRun(cmd *cobra.Command, args []string) error {
	projectDir, err := resolveProject()
	if err != nil {
		return err
	}

	target := "all"
	if len(args) > 0 {
		target = args[0]
	}

	stopped := 0

	if target == "game" || target == "all" {
		pid, err := godot.StopByPIDFile(projectDir, godot.GamePIDFile)
		if err != nil {
			if flagVerbose {
				fmt.Fprintf(cmd.ErrOrStderr(), "game: %v\n", err)
			}
		} else {
			fmt.Printf("Stopped game (PID %d)\n", pid)
			stopped++
		}
	}

	if target == "editor" || target == "all" {
		pid, err := godot.StopByPIDFile(projectDir, godot.EditorPIDFile)
		if err != nil {
			if flagVerbose {
				fmt.Fprintf(cmd.ErrOrStderr(), "editor: %v\n", err)
			}
		} else {
			fmt.Printf("Stopped editor (PID %d)\n", pid)
			stopped++
		}
	}

	if stopped == 0 {
		fmt.Println("No Godot process found")
	}

	return nil
}
