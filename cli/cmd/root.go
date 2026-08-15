package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/Dluck-Games/godot-debug-bridge/cli/internal/project"
)

var (
	flagProjectDir string
	flagVerbose    bool
)

var rootCmd = &cobra.Command{
	Use:   "gdbg",
	Short: "Godot Debug Bridge CLI",
	Long:  "Run, control, observe, test, and debug Godot projects without opening the editor.",
	RunE: func(cmd *cobra.Command, args []string) error {
		if cmd.Flags().Changed("windowed") {
			flagWindowed = true
		}
		if cmd.Flags().Changed("fullscreen") {
			flagFullscreen = true
		}
		if cmd.Flags().Changed("detach") {
			flagDetach = true
		}
		return runGameCmd.RunE(runGameCmd, args)
	},
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.PersistentFlags().StringVar(&flagProjectDir, "path", "", "path to Godot project directory (deprecated; use --project-dir)")
	rootCmd.PersistentFlags().StringVar(&flagProjectDir, "project-dir", "", "path to Godot project directory")
	rootCmd.PersistentFlags().BoolVarP(&flagVerbose, "verbose", "v", false, "verbose output")
	rootCmd.Flags().BoolVar(&flagWindowed, "windowed", false, "run game in windowed mode (shortcut for 'gdbg run game --windowed')")
	rootCmd.Flags().BoolVar(&flagFullscreen, "fullscreen", false, "run game in fullscreen mode (shortcut for 'gdbg run game --fullscreen')")
	rootCmd.Flags().BoolVar(&flagDetach, "detach", false, "run game in background, log to file, return immediately (shortcut for 'gdbg run game --detach')")
	rootCmd.MarkFlagsMutuallyExclusive("windowed", "fullscreen")
}

func resolveProject() (string, error) {
	explicit := flagProjectDir
	if explicit == "" {
		explicit = os.Getenv("GDBG_PROJECT_DIR")
	}
	p, err := project.ResolveProjectDir(explicit)
	if err != nil {
		return "", fmt.Errorf("cannot find Godot project: %w\nSet --project-dir or GDBG_PROJECT_DIR, or run from within the project tree in an interactive terminal", err)
	}
	if flagVerbose {
		fmt.Fprintf(os.Stderr, "Project: %s\n", p)
	}
	return p, nil
}
