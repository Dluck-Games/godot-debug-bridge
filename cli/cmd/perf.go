package cmd

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/Dluck-Games/godot-debug-bridge/cli/internal/debug"
	"github.com/Dluck-Games/godot-debug-bridge/cli/internal/perf"
	"github.com/spf13/cobra"
)

var perfCmd = &cobra.Command{Use: "perf", Short: "Sample and compare game performance"}

func init() {
	for op, description := range map[string]string{
		"start":   "Start frame sampling",
		"status":  "Show sampler status",
		"stop":    "Stop and summarize sampling",
		"capture": "Sample for a fixed duration",
	} {
		cmd := &cobra.Command{Use: op, Args: cobra.NoArgs, Short: description, RunE: runPerf}
		if op == "start" || op == "capture" {
			cmd.Flags().String("prefix", "", "custom monitor name prefix")
		}
		if op == "stop" || op == "capture" {
			cmd.Flags().String("out", "", "write summary JSON to this file")
		}
		if op == "capture" {
			cmd.Flags().Float64("duration", 0, "sample duration in seconds")
			cmd.MarkFlagRequired("duration")
		}
		perfCmd.AddCommand(cmd)
	}
	diff := &cobra.Command{Use: "diff <before.json> <after.json>", Args: cobra.ExactArgs(2), Short: "Compare performance summaries", RunE: runPerfDiff}
	diff.Flags().Bool("json", false, "print JSON")
	perfCmd.AddCommand(diff)
	rootCmd.AddCommand(perfCmd)
}

func perfRequest(op, prefix, out string) (map[string]any, error) {
	args := map[string]any{"op": op}
	if prefix != "" {
		args["prefix"] = prefix
	}
	if out != "" {
		path, err := filepath.Abs(out)
		if err != nil {
			return nil, err
		}
		args["out"] = path
	}
	bridge := debug.NewBridge(debug.IPCDir())
	if !flagVerbose {
		bridge.Log = nil
	}
	raw, err := bridge.Send(map[string]any{"cmd": "perf", "args": args}, debug.DefaultTimeout)
	if err != nil {
		return nil, err
	}
	env, _ := raw.(map[string]any)
	if env["status"] == "error" {
		return nil, fmt.Errorf("perf %s: %v", op, env["message"])
	}
	result, ok := env["result"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("unexpected perf reply: %v", raw)
	}
	return result, nil
}

func runPerf(cmd *cobra.Command, _ []string) error {
	prefix, _ := cmd.Flags().GetString("prefix")
	out, _ := cmd.Flags().GetString("out")
	op := cmd.Name()
	if op == "capture" {
		seconds, _ := cmd.Flags().GetFloat64("duration")
		duration := time.Duration(seconds * float64(time.Second))
		if math.IsNaN(seconds) || math.IsInf(seconds, 0) || seconds <= 0 || duration <= 0 {
			return fmt.Errorf("duration must be a positive, finite number of seconds within timer range")
		}
		ctx, cancel := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
		defer cancel()
		if _, err := perfRequest("start", prefix, ""); err != nil {
			return err
		}
		select {
		case <-ctx.Done():
		case <-time.After(duration):
		}
		op = "stop"
		prefix = ""
	}
	result, err := perfRequest(op, prefix, out)
	if err != nil {
		return err
	}
	return json.NewEncoder(cmd.OutOrStdout()).Encode(result)
}

func runPerfDiff(cmd *cobra.Command, args []string) error {
	rows, err := perf.Diff(args[0], args[1])
	if err != nil {
		return err
	}
	asJSON, _ := cmd.Flags().GetBool("json")
	if asJSON {
		return json.NewEncoder(cmd.OutOrStdout()).Encode(rows)
	}
	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "METRIC\tBEFORE\tAFTER\tDELTA")
	format := func(v *float64) string {
		if v == nil {
			return "-"
		}
		return strconv.FormatFloat(*v, 'g', 6, 64)
	}
	for _, r := range rows {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", r.Metric, format(r.Before), format(r.After), format(r.Delta))
	}
	return w.Flush()
}
