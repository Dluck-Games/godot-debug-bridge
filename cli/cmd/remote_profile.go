package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/Dluck-Games/godot-debug-bridge/cli/internal/debug"
	"github.com/Dluck-Games/godot-debug-bridge/cli/internal/godot"
	"github.com/Dluck-Games/godot-debug-bridge/cli/internal/record"
	"github.com/Dluck-Games/godot-debug-bridge/cli/internal/remoteprof"
	"github.com/spf13/cobra"
)

func profilerPIDPath() string { return filepath.Join(debug.DebugRoot(), "remote_profiler.pid") }
func init() {
	serve := &cobra.Command{Use: "remote-serve", Short: "Collect one Godot remote profiler session on loopback", Args: cobra.NoArgs, RunE: serveProfile}
	serve.Flags().Int("port", 0, "TCP port (0 selects a free port)")
	serve.Flags().String("out", "", "output JSON file")
	serve.MarkFlagRequired("out")
	serve.Flags().String("ready-file", "", "publish the listening port to this file")
	serve.Flags().Bool("track-game", false, "register collector PID for stop game")
	serve.Flags().MarkHidden("track-game")
	perfCmd.AddCommand(serve)
}
func serveProfile(cmd *cobra.Command, _ []string) error {
	port, _ := cmd.Flags().GetInt("port")
	out, _ := cmd.Flags().GetString("out")
	ready, _ := cmd.Flags().GetString("ready-file")
	track, _ := cmd.Flags().GetBool("track-game")
	if out == "" || port < 0 || port > 65535 {
		return errors.New("invalid output path or port")
	}
	ctx, cancel := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if track {
		pidPath := profilerPIDPath()
		if err := registerProfiler(ctx, pidPath); err != nil {
			return fmt.Errorf("register game profiler: %w", err)
		}
		defer os.Remove(pidPath)
	}
	return remoteprof.Serve(ctx, port, out, func(port int) error {
		if ready == "" {
			fmt.Fprintf(cmd.ErrOrStderr(), "Listening on 127.0.0.1:%d\n", port)
			return nil
		}
		return os.WriteFile(ready, []byte(strconv.Itoa(port)), 0600)
	})
}

// Serialize stale-file recovery so two starters cannot unlink each other's registration.
func clearStaleProfiler(ctx context.Context, path string) error {
	lock, err := record.AcquireWindowLock(ctx, path+".lock")
	if err != nil {
		return err
	}
	defer lock.Unlock()
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid <= 0 {
		return errors.New("invalid remote profiler PID file")
	}
	if godot.IsAlive(pid) {
		return fmt.Errorf("remote profiler is already running (PID %d)", pid)
	}
	return os.Remove(path)
}

func registerProfiler(ctx context.Context, path string) error {
	const flags = os.O_WRONLY | os.O_CREATE | os.O_EXCL
	f, err := os.OpenFile(path, flags, 0600)
	if os.IsExist(err) {
		if err := clearStaleProfiler(ctx, path); err != nil {
			return err
		}
		f, err = os.OpenFile(path, flags, 0600)
	}
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(f, os.Getpid())
	if err = errors.Join(err, f.Close()); err != nil {
		os.Remove(path)
	}
	return err
}

type profileProcess struct {
	cmd        *exec.Cmd
	done       chan error
	engineArgs []string
}

func startProfiler(out string, detach bool) (*profileProcess, error) {
	if out == "" {
		return &profileProcess{}, nil
	}
	if err := os.MkdirAll(debug.DebugRoot(), 0755); err != nil {
		return nil, err
	}
	dir, err := os.MkdirTemp(debug.DebugRoot(), "profiler-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	ready := filepath.Join(dir, "port")
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	args := []string{"perf", "remote-serve", "--port", "0", "--out", out, "--ready-file", ready}
	if detach {
		args = append(args, "--track-game")
	}
	cmd := exec.Command(exe, args...)
	log, err := os.OpenFile(filepath.Join(debug.DebugRoot(), "remote_profiler.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return nil, err
	}
	defer log.Close()
	cmd.Stderr = log
	godot.DetachCommand(cmd)
	if err = cmd.Start(); err != nil {
		return nil, err
	}
	p := &profileProcess{cmd: cmd, done: make(chan error, 1)}
	go func() { p.done <- cmd.Wait() }()
	ticker := time.NewTicker(15 * time.Millisecond)
	defer ticker.Stop()
	timeout := time.After(5 * time.Second)
	for {
		select {
		case err := <-p.done:
			return nil, fmt.Errorf("profiler exited before ready (%v), see %s", err, log.Name())
		case <-timeout:
			p.interrupt()
			<-p.done
			return nil, errors.New("profiler readiness timeout")
		case <-ticker.C:
			b, err := os.ReadFile(ready)
			if err == nil {
				port, err := strconv.Atoi(string(b))
				if err == nil && port > 0 {
					p.engineArgs = []string{"--remote-debug", fmt.Sprintf("tcp://127.0.0.1:%d", port)}
					return p, nil
				}
			}
		}
	}
}
func (p *profileProcess) interrupt() {
	if err := p.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		p.cmd.Process.Kill()
	}
}
func (p *profileProcess) finish(gameErr error) error {
	if p.cmd == nil {
		return gameErr
	}
	if gameErr == nil {
		select {
		case err := <-p.done:
			return err
		case <-time.After(2 * time.Second):
		}
	}
	p.interrupt()
	return errors.Join(gameErr, <-p.done)
}

// The collector removes its registration after closing and renaming the JSON.
func waitProfiler() error {
	path := profilerPIDPath()
	// Some games exit before opening the debugger socket. Give EOF time to
	// drain first, then ask a still-waiting collector to publish its report.
	start := time.Now()
	deadline := start.Add(3 * time.Second)
	signaled := false
	for {
		b, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
		if err == nil && pid > 0 {
			if !godot.IsAlive(pid) {
				ctx, cancel := context.WithDeadline(context.Background(), deadline)
				defer cancel()
				return clearStaleProfiler(ctx, path)
			}
			if !signaled && time.Since(start) >= 2*time.Second {
				if proc, err := os.FindProcess(pid); err == nil {
					if err := proc.Signal(syscall.SIGTERM); err != nil && godot.IsAlive(pid) {
						if err := godot.KillProcess(pid); err != nil {
							return err
						}
					}
				}
				signaled = true
				continue
			}
		}
		if time.Now().After(deadline) {
			return errors.New("timed out waiting for remote profiler report")
		}
		time.Sleep(25 * time.Millisecond)
	}
}
