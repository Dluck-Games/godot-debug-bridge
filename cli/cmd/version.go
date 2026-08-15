package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	versionStr string
	commitStr  string
	dateStr    string
)

func SetVersionInfo(version, commit, date string) {
	versionStr = version
	commitStr = commit
	dateStr = date
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("gdbg %s (%s) built %s\n", versionStr, commitStr, dateStr)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
