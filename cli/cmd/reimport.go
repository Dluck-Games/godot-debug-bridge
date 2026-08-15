// godot-debug-bridge/cli/cmd/reimport.go
package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/Dluck-Games/godot-debug-bridge/cli/internal/godot"
)

var flagEditorReimport bool

var reimportCmd = &cobra.Command{
	Use:   "reimport",
	Short: "Reimport assets and generate UIDs",
	Long:  "Kill running Godot, clear import cache, run headless import, and generate missing .uid files. With --editor, uses the Godot editor for scanning instead of headless mode.",
	RunE:  reimportRun,
}

func init() {
	reimportCmd.Flags().BoolVar(&flagEditorReimport, "editor", false, "use Godot editor for reimport (launch, scan, close)")
	rootCmd.AddCommand(reimportCmd)
}

func reimportRun(cmd *cobra.Command, args []string) error {
	projectDir, err := resolveProject()
	if err != nil {
		return err
	}

	godotBin, err := godot.FindBinary()
	if err != nil {
		return err
	}

	// Stop all running Godot processes
	fmt.Fprintln(os.Stderr, "Stopping running Godot processes...")
	godot.StopByPIDFile(projectDir, godot.GamePIDFile)
	godot.StopByPIDFile(projectDir, godot.EditorPIDFile)

	// Clear import cache
	if err := godot.ClearImportCache(projectDir); err != nil {
		return fmt.Errorf("failed to clear import cache: %w", err)
	}

	if flagEditorReimport {
		return reimportViaEditor(godotBin, projectDir)
	}
	return reimportHeadless(godotBin, projectDir)
}

func reimportHeadless(godotBin, projectDir string) error {
	fmt.Fprintln(os.Stderr, "Running headless import...")
	if err := godot.RunImport(godotBin, projectDir); err != nil {
		return fmt.Errorf("headless import failed: %w", err)
	}

	count, err := godot.GenerateMissingUIDs(godotBin, projectDir)
	if err != nil {
		return fmt.Errorf("UID generation failed: %w", err)
	}

	removed, err := godot.CleanOrphanedUIDs(projectDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: orphan UID cleanup failed: %v\n", err)
	}

	fmt.Fprintln(os.Stderr, "Reimport complete.")
	if count > 0 {
		fmt.Fprintf(os.Stderr, "  Generated UIDs for %d files\n", count)
	}
	if len(removed) > 0 {
		fmt.Fprintf(os.Stderr, "  Cleaned %d orphaned UIDs\n", len(removed))
	}
	return nil
}

func reimportViaEditor(godotBin, projectDir string) error {
	fmt.Fprintln(os.Stderr, "Launching editor for reimport scan...")

	proc, err := godot.Launch(godotBin, godot.LaunchOpts{
		ProjectDir: projectDir,
		Editor:     true,
	})
	if err != nil {
		return err
	}
	pid := proc.Process.Pid

	// Poll for .godot/imported/ to be repopulated
	importedDir := filepath.Join(projectDir, ".godot", "imported")
	timeout := 120 * time.Second
	deadline := time.After(timeout)
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	scanned := false
	for {
		select {
		case <-deadline:
			fmt.Fprintln(os.Stderr, "Timeout waiting for editor scan, killing editor...")
			godot.KillProcess(pid)
			return fmt.Errorf("editor reimport timed out after %s", timeout)
		case <-ticker.C:
			entries, err := os.ReadDir(importedDir)
			if err == nil && len(entries) > 0 {
				scanned = true
			}
			if scanned {
				// Give it a moment to finish writing
				time.Sleep(2 * time.Second)
				fmt.Fprintln(os.Stderr, "Editor scan complete, closing editor...")
				godot.KillProcess(pid)
				fmt.Fprintln(os.Stderr, "Reimport via editor complete.")
				return nil
			}
		}
	}
}
