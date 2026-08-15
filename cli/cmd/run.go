// godot-debug-bridge/cli/cmd/run.go
package cmd

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/Dluck-Games/godot-debug-bridge/cli/internal/debug"
	"github.com/Dluck-Games/godot-debug-bridge/cli/internal/godot"
)

var (
	flagWindowed   bool
	flagFullscreen bool
	flagForce      bool
	flagDetach     bool
)

const (
	// gameReadyTimeout bounds how long `gdbg run game` waits for the game's
	// AIDebugBridge to answer a round trip before declaring startup failure.
	// Large project startup can take 40-50s, so a 30s bound produced false timeouts.
	gameReadyTimeout = 90 * time.Second
	// readinessStabilizeInterval is the deliberate pause between the two
	// consecutive round trips that must both succeed before readiness is
	// declared. One transient probe can land in the window just before
	// synchronous project startup blocks the game main thread; requiring a second
	// probe that far apart proves the bridge is stably responsive.
	readinessStabilizeInterval = 500 * time.Millisecond
)

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Run game or editor",
}

var runGameCmd = &cobra.Command{
	Use:   "game [-- godot-args...]",
	Short: "Run the game (headless by default)",
	Long: `Smart boot: kill existing → ensure import → launch → poll readiness.

Display modes (mutually exclusive):
  (default)     Headless — no window, lowest resource usage (for AI agents via gdbg debug)
  --windowed    Windowed — launch with a GUI window
  --fullscreen  Fullscreen — launch in fullscreen mode

Detached mode:
  --detach      Launch in background, redirect output to log file, return immediately.
                Designed for AI agents whose Bash tool blocks on streaming stdout.

Pass arbitrary game arguments after --:
  gdbg run game --windowed -- --my-game-option`,
	Args: cobra.ArbitraryArgs,
	RunE: runGame,
}

var runEditorCmd = &cobra.Command{
	Use:   "editor",
	Short: "Open the Godot editor",
	RunE:  runEditor,
}

func init() {
	runGameCmd.Flags().BoolVar(&flagWindowed, "windowed", false, "launch with GUI window instead of headless")
	runGameCmd.Flags().BoolVar(&flagFullscreen, "fullscreen", false, "launch in fullscreen mode")
	runGameCmd.Flags().BoolVar(&flagForce, "force", false, "skip duplicate-process check, allow multiple instances")
	runGameCmd.Flags().BoolVar(&flagDetach, "detach", false, "launch in background, log to file only, return immediately (for AI agents)")
	runGameCmd.MarkFlagsMutuallyExclusive("windowed", "fullscreen")

	runCmd.AddCommand(runGameCmd, runEditorCmd)
	rootCmd.AddCommand(runCmd)
}

func runGame(cmd *cobra.Command, args []string) error {
	projectDir, err := resolveProject()
	if err != nil {
		return err
	}

	godotBin, err := godot.FindBinary()
	if err != nil {
		return err
	}

	// Duplicate check
	if !flagForce {
		pid, err := godot.ReadPID(projectDir, godot.GamePIDFile)
		if err == nil && godot.IsAlive(pid) {
			return fmt.Errorf("Game already running (PID %d). Use --force to start another or 'gdbg stop game' first.", pid)
		}
		if err == nil {
			godot.RemovePID(projectDir, godot.GamePIDFile)
		}
	}

	// Ensure import cache
	if err := godot.EnsureImportCache(godotBin, projectDir); err != nil {
		return fmt.Errorf("import cache failed: %w", err)
	}

	godotArgs := []string{}
	switch {
	case flagFullscreen:
		godotArgs = append(godotArgs, "--fullscreen")
	case flagWindowed:
		godotArgs = append(godotArgs, "--windowed")
	default:
		godotArgs = append(godotArgs, "--headless")
	}
	godotArgs = append(godotArgs, "--path", projectDir)

	gameArgs := append([]string(nil), args...)
	if len(gameArgs) > 0 {
		godotArgs = append(godotArgs, "--")
		godotArgs = append(godotArgs, gameArgs...)
	}

	if flagVerbose {
		fmt.Fprintf(os.Stderr, "Launching: %s %v\n", godotBin, godotArgs)
	}

	// Inject the authoritative state root so the game's AIDebugBridge and debug
	// artifacts resolve to the same $GDBG_STATE/debug layout as the CLI, instead
	// of probing home directories or using user:// paths.
	bridgeEnv := []string{"GDBG_STATE=" + debug.StateBase()}
	if flagVerbose {
		fmt.Fprintf(os.Stderr, "Injecting: %s\n", bridgeEnv[0])
	}

	if flagDetach {
		return runGameDetached(godotBin, godotArgs, projectDir, bridgeEnv)
	}
	return runGameAttached(godotBin, godotArgs, projectDir, bridgeEnv)
}

func logStateBase() string {
	return filepath.Join(debug.StateBase(), "logs")
}

func runGameAttached(godotBin string, godotArgs []string, projectDir string, bridgeEnv []string) error {
	logDir := filepath.Join(logStateBase(), "game")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return fmt.Errorf("cannot create log directory: %w", err)
	}
	logName := fmt.Sprintf("game-%s.log", time.Now().Format("20060102-150405"))
	logFile, err := os.Create(filepath.Join(logDir, logName))
	if err != nil {
		return fmt.Errorf("cannot create log file: %w", err)
	}
	defer logFile.Close()

	proc := newStreamingProcess(godotBin, godotArgs, logFile, bridgeEnv)
	if err := proc.Start(); err != nil {
		return fmt.Errorf("failed to launch Godot: %w", err)
	}

	if err := godot.WritePID(projectDir, godot.GamePIDFile, proc.Pid()); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not write PID file: %v\n", err)
	}

	ready := pollReadiness(debug.IPCDir(), gameReadyTimeout)
	if ready {
		fmt.Printf("Game started (PID %d)\n", proc.Pid())
	} else {
		fmt.Printf("Game launched (PID %d) but readiness poll timed out\n", proc.Pid())
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	exitCh := make(chan error, 1)
	go func() { exitCh <- proc.Wait() }()

	select {
	case err := <-exitCh:
		godot.RemovePID(projectDir, godot.GamePIDFile)
		if err != nil {
			return fmt.Errorf("Godot exited with error: %w", err)
		}
		return nil
	case sig := <-sigCh:
		fmt.Fprintf(os.Stderr, "\nReceived %s, stopping game...\n", sig)
		godot.KillProcess(proc.Pid())
		godot.RemovePID(projectDir, godot.GamePIDFile)
		return nil
	}
}

func runGameDetached(godotBin string, godotArgs []string, projectDir string, bridgeEnv []string) error {
	logDir := filepath.Join(logStateBase(), "game")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return fmt.Errorf("cannot create log directory: %w", err)
	}
	logName := fmt.Sprintf("game-%s.log", time.Now().Format("20060102-150405"))
	logPath := filepath.Join(logDir, logName)

	logFile, err := os.Create(logPath)
	if err != nil {
		return fmt.Errorf("cannot create log file: %w", err)
	}

	displayMode := godot.DisplayHeadless
	if flagWindowed {
		displayMode = godot.DisplayWindowed
	} else if flagFullscreen {
		displayMode = godot.DisplayFullscreen
	}

	proc, err := godot.Launch(godotBin, godot.LaunchOpts{
		ProjectDir:  projectDir,
		DisplayMode: displayMode,
		Detach:      true,
		ExtraArgs:   extractPassthroughArgs(godotArgs),
		LogFile:     logFile,
		Env:         bridgeEnv,
	})
	if err != nil {
		logFile.Close()
		return fmt.Errorf("failed to launch Godot: %w", err)
	}

	pid := proc.Process.Pid
	if err := godot.WritePID(projectDir, godot.GamePIDFile, pid); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not write PID file: %v\n", err)
	}

	ready := pollReadiness(debug.IPCDir(), gameReadyTimeout)

	switch {
	case ready:
		fmt.Printf("Game started (PID %d, detached)\n", pid)
	default:
		fmt.Printf("Game launched (PID %d, detached) — readiness poll timed out\n", pid)
	}
	fmt.Printf("Log: %s\n", logPath)

	return nil
}

func extractPassthroughArgs(godotArgs []string) []string {
	for i, a := range godotArgs {
		if a == "--" && i+1 < len(godotArgs) {
			return godotArgs[i+1:]
		}
	}
	return nil
}

func runEditor(cmd *cobra.Command, args []string) error {
	projectDir, err := resolveProject()
	if err != nil {
		return err
	}

	godotBin, err := godot.FindBinary()
	if err != nil {
		return err
	}

	proc, err := godot.Launch(godotBin, godot.LaunchOpts{
		ProjectDir:  projectDir,
		Editor:      true,
		DisplayMode: godot.DisplayHeadless,
		Detach:      true,
	})
	if err != nil {
		return err
	}

	pid := proc.Process.Pid
	if err := godot.WritePID(projectDir, godot.EditorPIDFile, pid); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not write PID file: %v\n", err)
	}

	fmt.Printf("Editor started (PID %d)\n", pid)
	return nil
}

// streamingProcess wraps exec.Cmd to tee stdout/stderr to both terminal and log file.
type streamingProcess struct {
	cmd *exec.Cmd
}

func newStreamingProcess(bin string, args []string, logFile *os.File, extraEnv []string) *streamingProcess {
	cmd := exec.Command(bin, args...)
	cmd.Env = append(os.Environ(), extraEnv...)
	cmd.Stdout = io.MultiWriter(os.Stdout, logFile)
	cmd.Stderr = io.MultiWriter(os.Stderr, logFile)
	return &streamingProcess{cmd: cmd}
}

func (p *streamingProcess) Start() error { return p.cmd.Start() }
func (p *streamingProcess) Wait() error  { return p.cmd.Wait() }
func (p *streamingProcess) Pid() int     { return p.cmd.Process.Pid }

// pollReadiness waits until the game's AIDebugBridge answers two consecutive
// round trips over the command/result file IPC under $GDBG_STATE/debug/ipc,
// separated by a short stabilization interval. A single transient response —
// which can land just before synchronous project startup blocks the game main
// thread — is not enough; any failed probe resets the consecutive-success
// count. A pre-existing IPC directory alone is NOT sufficient: readiness
// requires an actual bridge response, so a stale directory from an earlier run
// cannot pass for a live game.
func pollReadiness(ipcDir string, timeout time.Duration) bool {
	probe := debug.NewBridge(ipcDir)
	probe.Log = nil
	payload := map[string]any{"cmd": "help", "args": []string{}}

	// Each attempt is a full command/result round trip; any reply — even a
	// game-reported error — proves the bridge is live. The attempt bound keeps
	// one slow probe from consuming the whole readiness window.
	attempt := 1 * time.Second
	deadline := time.Now().Add(timeout)
	consecutive := 0
	for time.Now().Before(deadline) {
		if _, err := probe.Send(payload, attempt); err == nil {
			consecutive++
			if consecutive >= 2 {
				return true
			}
			// Pause before the second required round trip so the pair is
			// meaningfully separated in time; cap the pause at the deadline so
			// the overall readiness window is never overshot.
			pause := readinessStabilizeInterval
			if rem := time.Until(deadline); rem < pause {
				pause = rem
			}
			time.Sleep(pause)
			continue
		}
		consecutive = 0
	}
	return false
}
