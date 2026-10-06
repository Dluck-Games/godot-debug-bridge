// godot-debug-bridge/cli/internal/testrunner/playtest.go
package testrunner

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Dluck-Games/godot-debug-bridge/cli/internal/debug"
)

const playtestFPS = 4
const playtestTimelineFile = "timeline.log"

// playtestSuiteAddonPath is the canonical res://-relative base script of the
// playtest tier. A quoted `extends` that resolves to exactly this file is the
// accepted base of a playtest suite hierarchy.
const playtestSuiteAddonPath = "addons/gdbg/testing/automation_playtest_suite.gd"

// playtestExtendsDepthLimit bounds recursive parent resolution so a broken or
// hostile project cannot cause unbounded work. Real playtest hierarchies are
// one or two levels deep.
const playtestExtendsDepthLimit = 16

// playtestSuiteAliases are direct class-name `extends` targets accepted as
// playtest bases without recursive resolution.
var playtestSuiteAliases = map[string]struct{}{
	"AutomationPlayTestSuite":  {},
	"DebugBridgePlaytestSuite": {},
}

type PlaytestResult struct {
	Name          string
	Passed        bool
	ExitCode      int
	ReportPath    string
	RecordingPath string
	ReportText    string
}

func RunPlaytest(godotBin, projectDir string, opts RunOptions) ([]PlaytestResult, error) {
	playtestDir := playtestTestsDir(projectDir)
	configs, err := discoverPlaytests(projectDir, playtestDir, opts.Suite)
	if err != nil {
		return nil, err
	}
	// Fail closed when discovery selects nothing. This includes a filter that
	// matched an existing but non-playtest script and the unfiltered case of an
	// empty suite directory. Refuse before any engine launch or output
	// directory creation so an empty selection is never reported as success.
	if len(configs) == 0 {
		if len(opts.Suite) > 0 {
			return nil, fmt.Errorf("no playtest suites matched %s in %s", strings.Join(opts.Suite, ", "), playtestDir)
		}
		return nil, fmt.Errorf("no playtest suites found in %s", playtestDir)
	}

	var results []PlaytestResult
	for _, configPath := range configs {
		name := playtestName(configPath)
		if opts.Verbose {
			fmt.Fprintf(os.Stderr, "  %s ... ", name)
		}

		result, err := runOnePlaytest(godotBin, projectDir, name, opts)
		if err != nil {
			return nil, err
		}
		results = append(results, result)

		if opts.Verbose {
			if result.Passed {
				fmt.Fprintln(os.Stderr, "PASS")
			} else {
				fmt.Fprintln(os.Stderr, "FAIL")
			}
		} else if !result.Passed {
			fmt.Fprintf(os.Stderr, "  [FAIL] %s\n", name)
		}
	}

	return results, nil
}

func runOnePlaytest(godotBin, projectDir, name string, opts RunOptions) (PlaytestResult, error) {
	outputDir := filepath.Join(playtestLogStateBase(), "playtest", name)
	frameDir := filepath.Join(outputDir, "frames")
	if opts.PlaytestFrameDir != "" {
		frameDir = opts.PlaytestFrameDir
	}
	if err := os.RemoveAll(outputDir); err != nil {
		return PlaytestResult{}, fmt.Errorf("clean playtest output: %w", err)
	}
	if err := os.MkdirAll(frameDir, 0o755); err != nil {
		return PlaytestResult{}, fmt.Errorf("create playtest output: %w", err)
	}
	// The game's AIDebugBridge starts as an autoload. Give each playtest process
	// its own state root before launch so it cannot consume command files queued
	// by an interactive debug session sharing the normal GDBG_STATE.
	playtestStateRoot := filepath.Join(outputDir, "state")
	if err := os.MkdirAll(playtestStateRoot, 0o755); err != nil {
		return PlaytestResult{}, fmt.Errorf("create isolated playtest state: %w", err)
	}

	logPath := filepath.Join(outputDir, "godot.log")
	logFile, err := os.Create(logPath)
	if err != nil {
		return PlaytestResult{}, fmt.Errorf("create playtest log: %w", err)
	}
	defer logFile.Close()

	args := playtestGodotArgs(projectDir, name, outputDir, frameDir, opts)

	cmd := exec.Command(godotBin, args...)
	cmd.Dir = projectDir
	cmd.Env = isolatedPlaytestEnv(playtestStateRoot)
	applyPlaytestDisplayAttr(cmd, opts)
	var output bytes.Buffer
	var writer io.Writer = io.MultiWriter(logFile, &output)
	if opts.Verbose {
		writer = io.MultiWriter(logFile, &output, os.Stderr)
	}
	cmd.Stdout = writer
	cmd.Stderr = writer

	fmt.Fprintf(os.Stdout, "running playtest %s\n", name)

	timelinePath := filepath.Join(outputDir, playtestTimelineFile)
	var stopTail chan struct{}
	var tailWG sync.WaitGroup
	if shouldTailPlaytestTimeline(opts) {
		stopTail = make(chan struct{})
		tailWG.Add(1)
		go func() {
			defer tailWG.Done()
			tailPlaytestTimeline(timelinePath, os.Stdout, stopTail)
		}()
	}

	err = cmd.Run()
	if stopTail != nil {
		close(stopTail)
		tailWG.Wait()
	}
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			return PlaytestResult{}, fmt.Errorf("run Godot playtest: %w", err)
		}
	}

	reportPath := filepath.Join(outputDir, "report.txt")
	reportText := ""
	if data, readErr := os.ReadFile(reportPath); readErr == nil {
		reportText = string(data)
	} else if exitCode == 0 {
		exitCode = 2
		reportText = "Missing playtest report: " + reportPath
	}

	result := PlaytestResult{
		Name:       name,
		Passed:     exitCode == 0,
		ExitCode:   exitCode,
		ReportPath: reportPath,
		ReportText: reportText,
	}

	if opts.Record && result.Passed {
		recordingPath := filepath.Join(outputDir, "recording.mp4")
		if err := encodePlaytestRecording(frameDir, recordingPath); err != nil {
			return PlaytestResult{}, err
		}
		result.RecordingPath = recordingPath
	}

	return result, nil
}

// shouldTailPlaytestTimeline reports whether the CLI should tail timeline.log
// while the playtest runs. Verbose mode already streams Godot stdout/stderr,
// which carry the same timeline lines, so tailing would duplicate each line.
func shouldTailPlaytestTimeline(opts RunOptions) bool {
	return !opts.Verbose
}

func playtestGodotArgs(projectDir, name, outputDir, frameDir string, opts RunOptions) []string {
	display := "--windowed"
	if opts.Headless {
		display = "--headless"
	}
	args := []string{
		display,
		"--path", projectDir,
		"--",
		"--playtest=" + name,
		"--playtest-output-dir=" + outputDir,
		"--playtest-frame-dir=" + frameDir,
	}
	if !opts.Headless && !opts.Windowed {
		args = append(args, "--playtest-background")
	}
	if opts.Record {
		args = append(args, "--record")
	}
	return args
}

func tailPlaytestTimeline(path string, w io.Writer, stop <-chan struct{}) {
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	var (
		file    *os.File
		offset  int64
		partial []byte
	)
	open := func() {
		if file != nil {
			return
		}
		f, err := os.Open(path)
		if err != nil {
			return
		}
		file = f
	}
	drain := func(final bool) {
		open()
		if file == nil {
			return
		}
		info, err := file.Stat()
		if err != nil || info.Size() <= offset {
			if final && len(partial) > 0 {
				fmt.Fprintln(w, strings.TrimRight(string(partial), "\r"))
				partial = nil
			}
			return
		}
		buf := make([]byte, info.Size()-offset)
		n, err := file.ReadAt(buf, offset)
		if n > 0 {
			offset += int64(n)
			partial = append(partial, buf[:n]...)
		}
		if err != nil && err != io.EOF {
			return
		}
		for {
			i := bytes.IndexByte(partial, '\n')
			if i < 0 {
				break
			}
			line := strings.TrimRight(string(partial[:i]), "\r")
			partial = partial[i+1:]
			if line != "" {
				fmt.Fprintln(w, line)
			}
		}
		if final && len(partial) > 0 {
			fmt.Fprintln(w, strings.TrimRight(string(partial), "\r"))
			partial = nil
		}
	}
	for {
		select {
		case <-stop:
			drain(true)
			if file != nil {
				file.Close()
			}
			return
		case <-ticker.C:
			drain(false)
		}
	}
}

func isolatedPlaytestEnv(stateRoot string) []string {
	env := make([]string, 0, len(os.Environ())+1)
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, "GDBG_STATE=") {
			env = append(env, entry)
		}
	}
	return append(env, "GDBG_STATE="+stateRoot)
}

func discoverPlaytests(projectDir, dir string, suiteFilter []string) ([]string, error) {
	var configs []string
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".gd") {
			return nil
		}
		if !isAutomationPlaytest(projectDir, path) {
			return nil
		}
		if len(suiteFilter) > 0 {
			name := playtestName(path)
			matched := false
			for _, suite := range suiteFilter {
				if name == suite {
					matched = true
					break
				}
			}
			if !matched {
				return nil
			}
		}
		configs = append(configs, path)
		return nil
	})
	sort.Strings(configs)
	return configs, err
}

func playtestLogStateBase() string {
	return filepath.Join(debug.StateBase(), "logs")
}

// isAutomationPlaytest reports whether the .gd file at path belongs to the
// playtest tier by parsing its real `extends` declaration instead of matching
// raw line substrings. Comments never count as type evidence.
func isAutomationPlaytest(projectDir, path string) bool {
	return playtestExtendsAccepted(projectDir, path, make(map[string]bool), 0)
}

// playtestExtendsAccepted resolves path's parent chain. Direct suite aliases
// and the canonical addon base are accepted; quoted res:// parents are resolved
// recursively inside projectDir. Cycles, missing or outside-root parents, and
// non-playtest parents are rejected.
func playtestExtendsAccepted(projectDir, path string, visited map[string]bool, depth int) bool {
	if depth > playtestExtendsDepthLimit {
		return false
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	if visited[abs] {
		return false
	}
	visited[abs] = true

	parent, ok := parseExtendsDeclaration(path)
	if !ok {
		return false
	}
	if !parent.quoted {
		_, ok := playtestSuiteAliases[parent.value]
		return ok
	}
	resolved, ok := resolvePlaytestParent(projectDir, parent.value)
	if !ok {
		return false
	}
	if resolved == canonicalPlaytestBase(projectDir) {
		return true
	}
	return playtestExtendsAccepted(projectDir, resolved, visited, depth+1)
}

// canonicalPlaytestBase is the absolute path of the accepted addon base script.
func canonicalPlaytestBase(projectDir string) string {
	abs, err := filepath.Abs(filepath.Join(projectDir, filepath.FromSlash(playtestSuiteAddonPath)))
	if err != nil {
		return ""
	}
	return abs
}

// resolvePlaytestParent converts a quoted res:// parent declaration into an
// absolute path, rejecting anything that escapes the registered project root.
func resolvePlaytestParent(projectDir, resPath string) (string, bool) {
	if !strings.HasPrefix(resPath, "res://") {
		return "", false
	}
	rel := strings.TrimPrefix(resPath, "res://")
	if rel == "" {
		return "", false
	}
	root, err := filepath.Abs(projectDir)
	if err != nil {
		return "", false
	}
	resolved, err := filepath.Abs(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		return "", false
	}
	relCheck, err := filepath.Rel(root, resolved)
	if err != nil || relCheck == ".." || strings.HasPrefix(relCheck, ".."+string(filepath.Separator)) {
		return "", false
	}
	return resolved, true
}

// playtestExtendsSpec is the parsed parent of an `extends` declaration.
type playtestExtendsSpec struct {
	quoted bool
	value  string
}

// parseExtendsDeclaration returns the parent declared by the script's first
// real `extends` statement. Comment-only and blank lines are ignored, and any
// inline comment after the declaration is stripped without scanning inside
// quoted strings.
func parseExtendsDeclaration(path string) (playtestExtendsSpec, bool) {
	f, err := os.Open(path)
	if err != nil {
		return playtestExtendsSpec{}, false
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if !strings.HasPrefix(line, "extends") {
			continue
		}
		if len(line) > len("extends") && !isPlaytestSpaceByte(line[len("extends")]) {
			continue
		}
		rest := stripPlaytestInlineComment(strings.TrimSpace(line[len("extends"):]))
		rest = strings.TrimSpace(rest)
		if rest == "" {
			return playtestExtendsSpec{}, false
		}
		if rest[0] == '"' || rest[0] == '\'' {
			quote := rest[0]
			end := strings.IndexByte(rest[1:], quote)
			if end < 0 {
				return playtestExtendsSpec{}, false
			}
			return playtestExtendsSpec{quoted: true, value: rest[1 : 1+end]}, true
		}
		fields := strings.Fields(rest)
		if len(fields) == 0 {
			return playtestExtendsSpec{}, false
		}
		return playtestExtendsSpec{value: fields[0]}, true
	}
	return playtestExtendsSpec{}, false
}

// stripPlaytestInlineComment removes a trailing `#` comment while leaving the
// contents of quoted strings untouched.
func stripPlaytestInlineComment(s string) string {
	var quote byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		if quote != 0 {
			if c == quote {
				quote = 0
			}
			continue
		}
		switch c {
		case '"', '\'':
			quote = c
		case '#':
			return s[:i]
		}
	}
	return s
}

func isPlaytestSpaceByte(c byte) bool {
	return c == ' ' || c == '\t'
}

func playtestName(path string) string {
	name := strings.TrimSuffix(filepath.Base(path), ".gd")
	return strings.TrimPrefix(name, "playtest_")
}

// encodePlaytestRecording muxes the captured playtest frames into recording.mp4
// with the same in-process pure-Go encoder used by `gdbg debug record`. No
// external encoder or Swift runtime is involved.
func encodePlaytestRecording(frameDir, outputPath string) error {
	// Only continuous recorder frames named frame_*.png are valid samples,
	// sorted in zero-padded lexical (capture) order. Checkpoint screenshots
	// produced by the game (e.g. checkpoint_*.png) must never be substituted
	// for a recording; if no frame_* samples exist, fail closed.
	framePaths, err := filepath.Glob(filepath.Join(frameDir, "frame_*.png"))
	if err != nil {
		return err
	}
	sort.Strings(framePaths)
	if len(framePaths) == 0 {
		return fmt.Errorf("no recorded playtest frames found in %s", frameDir)
	}

	if err := debug.EncodeFramesToVideo(framePaths, outputPath, playtestFPS); err != nil {
		return fmt.Errorf("encode playtest recording: %w", err)
	}
	return nil
}
