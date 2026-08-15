package cmd

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Dluck-Games/godot-debug-bridge/cli/internal/addon"
	"github.com/Dluck-Games/godot-debug-bridge/cli/internal/godot"
)

var (
	flagAddonSource     string
	flagAddonVersion    string
	flagAddonForce      bool
	flagAddonSkipImport bool
)

var addonCmd = &cobra.Command{
	Use:   "addon",
	Short: "Manage the GDBG Godot addon",
}

var addonInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Install or update addons/gdbg in a Godot project",
	RunE:  addonInstallRun,
}

func init() {
	addonInstallCmd.Flags().StringVar(&flagAddonSource, "source", "", "local addons/gdbg source directory (for repository/source installs)")
	addonInstallCmd.Flags().StringVar(&flagAddonVersion, "version", "", "release version to install (default: CLI version or latest release)")
	addonInstallCmd.Flags().BoolVar(&flagAddonForce, "force", false, "replace an existing addons/gdbg directory")
	addonInstallCmd.Flags().BoolVar(&flagAddonSkipImport, "skip-import", false, "do not run Godot headless import after installation")
	addonCmd.AddCommand(addonInstallCmd)
	rootCmd.AddCommand(addonCmd)
}

func addonInstallRun(cmd *cobra.Command, args []string) error {
	projectDir, err := resolveProject()
	if err != nil {
		return err
	}
	version := strings.TrimSpace(flagAddonVersion)
	if version == "" {
		version = versionStr
	}
	result, err := addon.Install(cmd.Context(), addon.Options{
		ProjectDir: projectDir,
		SourceDir:  flagAddonSource,
		Version:    version,
		Force:      flagAddonForce,
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Installed GDBG addon (%s) to %s\n", result.Version, result.AddonDir)
	if result.ProjectChanged {
		fmt.Fprintln(cmd.OutOrStdout(), "Enabled res://addons/gdbg/plugin.cfg in project.godot")
	}
	if flagAddonSkipImport {
		return nil
	}
	godotBin, err := godot.FindBinary()
	if err != nil {
		return fmt.Errorf("addon installed, but Godot import could not start: %w", err)
	}
	if err := godot.RunImport(godotBin, projectDir); err != nil {
		return fmt.Errorf("addon installed, but Godot import failed: %w", err)
	}
	// Headless editor shutdown can run EditorPlugin._exit_tree(), which removes
	// autoloads registered during import. Reassert the product runtime contract
	// after import so subsequent editor-free launches are deterministic.
	if _, err := addon.EnsureAutoloads(filepath.Join(projectDir, "project.godot")); err != nil {
		return fmt.Errorf("addon imported, but runtime autoloads could not be finalized: %w", err)
	}
	fmt.Fprintln(cmd.OutOrStdout(), "Godot import completed; GDBG runtime autoloads are registered")
	return nil
}
