package record

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/Dluck-Games/godot-debug-bridge/cli/internal/debug"
	"github.com/Dluck-Games/godot-debug-bridge/cli/internal/godot"
)

const (
	DefaultFPS        = 30
	DefaultSeconds    = 10.0
	DefaultResolution = "1280x720"
	DefaultQuality    = 85
	DefaultTimeout    = 10 * time.Minute
	MaxFPS            = 120
	MaxDefaultSeconds = 300.0
)

var scriptErrorPattern = regexp.MustCompile(`(?m)^(?:\x1b\[[0-9;]*m)*(?:SCRIPT ERROR:|Parse Error:)`)

// Options configures one deterministic recording run.
type Options struct {
	ProjectDir         string
	ScriptPath         string
	Output             string
	FPS                int
	Seconds            float64
	Resolution         string
	Width              int
	Height             int
	ScaleFilter        ScaleFilter
	Scene              string
	Mute               bool
	Codec              Codec
	Quality            int
	Offscreen          string
	ContactSheet       string
	ManifestPath       string
	MaxFrames          int
	Timeout            time.Duration
	NoWindowLock       bool
	AllowDroppedFrames bool
	KeepRaw            bool
	Verbose            bool
	ExtraArgs          []string
}

// Result describes the produced recording artifacts.
type Result struct {
	Output       string
	ContactSheet string
	ManifestPath string
	Manifest     Manifest
	StagingDir   string
}

// Validate checks options and fills defaults before launching Godot.
func (o *Options) Validate() error {
	if o.ProjectDir == "" {
		return errors.New("project directory is required")
	}
	if o.FPS == 0 {
		o.FPS = DefaultFPS
	}
	if o.FPS < 1 || o.FPS > MaxFPS {
		return fmt.Errorf("--fps must be between 1 and %d", MaxFPS)
	}
	if o.Seconds == 0 {
		o.Seconds = DefaultSeconds
	}
	if o.ScriptPath == "" && (o.Seconds <= 0 || o.Seconds > MaxDefaultSeconds) {
		return fmt.Errorf("--seconds must be > 0 and <= %.0f", MaxDefaultSeconds)
	}
	if o.ScriptPath != "" {
		absScript, err := filepath.Abs(o.ScriptPath)
		if err != nil {
			return fmt.Errorf("resolve script path: %w", err)
		}
		info, err := os.Stat(absScript)
		if err != nil || info.IsDir() {
			return fmt.Errorf("recording script not found: %s", absScript)
		}
		o.ScriptPath = absScript
	}
	if o.Resolution == "" {
		o.Resolution = DefaultResolution
	}
	if _, _, err := ParseResolution(o.Resolution); err != nil {
		return err
	}
	if o.ScaleFilter == "" {
		o.ScaleFilter = ScaleBilinear
	}
	if o.Quality == 0 {
		o.Quality = DefaultQuality
	}
	if o.Quality < 1 || o.Quality > 100 {
		return errors.New("--quality must be between 1 and 100")
	}
	if o.Codec == "" {
		o.Codec = CodecAuto
	}
	if o.Offscreen == "" {
		o.Offscreen = "auto"
	}
	switch o.Offscreen {
	case "auto", "force", "off":
	default:
		return fmt.Errorf("invalid --offscreen %q (use auto, force, or off)", o.Offscreen)
	}
	if o.Scene != "" {
		if err := validateProjectScene(o.ProjectDir, o.Scene); err != nil {
			return err
		}
	}
	if o.Timeout <= 0 {
		o.Timeout = DefaultTimeout
	}
	if o.Output == "" {
		o.Output = DefaultOutputPath()
	}
	absOut, err := filepath.Abs(o.Output)
	if err != nil {
		return fmt.Errorf("resolve output path: %w", err)
	}
	ext := strings.ToLower(filepath.Ext(absOut))
	if ext != ".mp4" && ext != ".mov" {
		return fmt.Errorf("--output must end with .mp4 or .mov, got %q", o.Output)
	}
	o.Output = absOut
	if o.ContactSheet != "" {
		absSheet, err := filepath.Abs(o.ContactSheet)
		if err != nil {
			return fmt.Errorf("resolve contact-sheet path: %w", err)
		}
		o.ContactSheet = absSheet
	}
	if o.ManifestPath != "" {
		absManifest, err := filepath.Abs(o.ManifestPath)
		if err != nil {
			return fmt.Errorf("resolve manifest path: %w", err)
		}
		o.ManifestPath = absManifest
	}
	if o.MaxFrames == 0 {
		if o.ScriptPath == "" {
			o.MaxFrames = int(o.Seconds*float64(o.FPS)) + o.FPS*15
		} else {
			o.MaxFrames = o.FPS * 300
		}
	}
	return nil
}

// ParseResolution validates a "<W>x<H>" string.
func ParseResolution(res string) (int, int, error) {
	parts := strings.Split(strings.ToLower(strings.TrimSpace(res)), "x")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("--resolution must be <W>x<H>, got %q", res)
	}
	w, errW := strconv.Atoi(parts[0])
	h, errH := strconv.Atoi(parts[1])
	if errW != nil || errH != nil || w < 16 || h < 16 {
		return 0, 0, fmt.Errorf("invalid --resolution %q", res)
	}
	return w, h, nil
}

// DefaultOutputPath returns a timestamped .mp4 path under $GDBG_STATE/debug/recordings.
func DefaultOutputPath() string {
	now := time.Now()
	name := fmt.Sprintf(
		"recording_%04d%02d%02d_%02d%02d%02d_%03d.mp4",
		now.Year(), int(now.Month()), now.Day(),
		now.Hour(), now.Minute(), now.Second(),
		now.Nanosecond()/int(time.Millisecond),
	)
	return filepath.Join(debug.RecordingsDir(), name)
}

func validateProjectScene(projectDir, sceneRes string) error {
	if !strings.HasPrefix(sceneRes, "res://") {
		return fmt.Errorf("--scene must be a project-relative res:// path, got %q", sceneRes)
	}
	ext := strings.ToLower(filepath.Ext(sceneRes))
	if ext != ".tscn" && ext != ".scn" {
		return fmt.Errorf("--scene must reference a .tscn or .scn file, got %q", sceneRes)
	}
	abs := filepath.Join(projectDir, filepath.FromSlash(strings.TrimPrefix(sceneRes, "res://")))
	rel, err := filepath.Rel(projectDir, abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return fmt.Errorf("--scene %q escapes project directory", sceneRes)
	}
	if _, err := os.Stat(abs); err != nil {
		return fmt.Errorf("--scene file not found: %s", sceneRes)
	}
	return nil
}

// Run executes the full deterministic recording pipeline.
func Run(godotBin string, opts Options) (*Result, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}
	if runtime.GOOS != "darwin" && opts.Codec == CodecH264 {
		return nil, errors.New("built-in --codec h264 is only available on macOS (avconvert); use --codec auto or --codec mjpeg")
	}

	if err := godot.EnsureImportCache(godotBin, opts.ProjectDir); err != nil {
		return nil, fmt.Errorf("import cache failed: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), opts.Timeout)
	defer cancel()

	if !opts.NoWindowLock {
		lock, err := AcquireWindowLock(ctx, DefaultWindowLockPath())
		if err != nil {
			return nil, err
		}
		defer lock.Unlock()
	}

	stagingDir := filepath.Join(debug.StagingDir(), fmt.Sprintf("record-%d", time.Now().UnixNano()))
	if err := os.MkdirAll(stagingDir, 0o755); err != nil {
		return nil, fmt.Errorf("create staging directory: %w", err)
	}
	if !opts.KeepRaw {
		defer os.RemoveAll(stagingDir)
	}

	rawAVI := filepath.Join(stagingDir, "raw.avi")
	eventsPath := filepath.Join(stagingDir, "events.jsonl")
	logPath := filepath.Join(stagingDir, "godot.log")
	assembledMOV := filepath.Join(stagingDir, "assembled.mov")

	godotArgs := BuildGodotArgs(opts, rawAVI, eventsPath)
	if opts.Verbose {
		fmt.Fprintf(os.Stderr, "Recording via Godot: %s %v\n", godotBin, godotArgs)
	}

	logBytes, runErr := runGodotCapture(ctx, godotBin, opts.ProjectDir, godotArgs, logPath, opts.Verbose)
	if runErr != nil {
		return nil, fmt.Errorf("godot recording failed (log: %s): %w\n%s", logPath, runErr, logTail(logBytes, 25))
	}
	if scriptErrorPattern.Match(logBytes) {
		return nil, fmt.Errorf("godot reported GDScript errors during recording (log: %s)\n%s", logPath, logTail(logBytes, 25))
	}

	events, err := ParseEventsFile(eventsPath)
	if err != nil {
		return nil, fmt.Errorf("parse recording timeline (%s): %w", eventsPath, err)
	}
	if events.ErrorMessage != "" {
		return nil, fmt.Errorf("recording director error: %s", events.ErrorMessage)
	}

	avi, err := OpenAVI(rawAVI)
	if err != nil {
		return nil, fmt.Errorf("read recorded movie %s: %w", rawAVI, err)
	}

	outSamples, pcm16, manifestClips, manifestMarks, totalDropped, err := assembleTimeline(avi, events, opts)
	if err != nil {
		return nil, err
	}

	forceEven := opts.Codec == CodecH264 || (opts.Codec == CodecAuto && runtime.GOOS == "darwin")
	targetW, targetH := ResolveOutputSize(avi.Width, avi.Height, opts.Width, opts.Height, forceEven)
	resizedSamples, err := ResizeFramesJPEG(outSamples, avi.Width, avi.Height, targetW, targetH, opts.ScaleFilter, opts.Quality)
	if err != nil {
		return nil, err
	}

	var muxAudio *MuxAudio
	if !opts.Mute && avi.Audio.Present && len(pcm16) > 0 {
		muxAudio = &MuxAudio{
			Channels:   avi.Audio.Channels,
			SampleRate: avi.Audio.SampleRate,
			PCM16:      pcm16,
		}
	}

	muxCfg := MuxConfig{
		FPS:    opts.FPS,
		Width:  targetW,
		Height: targetH,
		Audio:  muxAudio,
	}

	var resolvedCodec string
	if opts.Codec == CodecMJPEG || (opts.Codec == CodecAuto && runtime.GOOS != "darwin") {
		if err := MuxSamples(muxCfg, resizedSamples, opts.Output); err != nil {
			return nil, err
		}
		resolvedCodec = "mjpeg"
	} else {
		if err := MuxSamples(muxCfg, resizedSamples, assembledMOV); err != nil {
			return nil, err
		}
		resolvedCodec, err = TranscodeToH264(assembledMOV, opts.Output, opts.Codec)
		if err != nil {
			return nil, err
		}
	}

	if opts.ContactSheet != "" {
		sheetFrames, sheetLabels := selectContactSheetFrames(resizedSamples, manifestMarks, opts.FPS)
		if err := WriteContactSheet(sheetFrames, sheetLabels, opts.ContactSheet); err != nil {
			return nil, err
		}
	}

	manifest := Manifest{
		Output:          opts.Output,
		FPS:             opts.FPS,
		Width:           targetW,
		Height:          targetH,
		HasAudio:        muxAudio != nil,
		Codec:           resolvedCodec,
		TotalFrames:     len(resizedSamples),
		DurationSeconds: float64(len(resizedSamples)) / float64(opts.FPS),
		MissingDraws:    totalDropped,
		Clips:           manifestClips,
		Marks:           manifestMarks,
	}
	if opts.ManifestPath != "" {
		if err := WriteManifest(opts.ManifestPath, manifest); err != nil {
			return nil, err
		}
	}

	res := &Result{
		Output:       opts.Output,
		ContactSheet: opts.ContactSheet,
		ManifestPath: opts.ManifestPath,
		Manifest:     manifest,
	}
	if opts.KeepRaw {
		res.StagingDir = stagingDir
	}
	return res, nil
}

// BuildGodotArgs constructs the Godot Movie Maker command line for opts.
func BuildGodotArgs(opts Options, rawAVIPath, eventsPath string) []string {
	args := []string{
		"--windowed",
		"--resolution", opts.Resolution,
		"--fixed-fps", strconv.Itoa(opts.FPS),
		"--write-movie", rawAVIPath,
	}
	if opts.MaxFrames > 0 {
		args = append(args, "--quit-after", strconv.Itoa(opts.MaxFrames))
	}
	args = append(args, "--path", opts.ProjectDir)
	if opts.Scene != "" {
		args = append(args, opts.Scene)
	}
	args = append(args, "--",
		"--gdbg-record-manifest="+eventsPath,
		"--gdbg-record-fps="+strconv.Itoa(opts.FPS),
		"--gdbg-record-offscreen="+opts.Offscreen,
	)
	if opts.ScriptPath != "" {
		args = append(args, "--gdbg-record-script="+opts.ScriptPath)
	} else {
		args = append(args, "--gdbg-record-seconds="+strconv.FormatFloat(opts.Seconds, 'f', 3, 64))
	}
	args = append(args, opts.ExtraArgs...)
	return args
}

func runGodotCapture(ctx context.Context, godotBin, projectDir string, args []string, logPath string, verbose bool) ([]byte, error) {
	logFile, err := os.Create(logPath)
	if err != nil {
		return nil, fmt.Errorf("create godot log: %w", err)
	}
	defer logFile.Close()

	cmd := exec.CommandContext(ctx, godotBin, args...)
	cmd.Dir = projectDir
	cmd.Env = withGDBGState(os.Environ())

	var writers []io.Writer
	writers = append(writers, logFile)
	if verbose {
		writers = append(writers, os.Stderr)
	}
	mw := io.MultiWriter(writers...)
	cmd.Stdout = mw
	cmd.Stderr = mw

	runErr := cmd.Run()
	_ = logFile.Sync()
	data, _ := os.ReadFile(logPath)
	if ctx.Err() != nil {
		return data, fmt.Errorf("recording timed out: %w", ctx.Err())
	}
	return data, runErr
}

func withGDBGState(env []string) []string {
	out := make([]string, 0, len(env)+1)
	for _, e := range env {
		if !strings.HasPrefix(e, "GDBG_STATE=") {
			out = append(out, e)
		}
	}
	return append(out, "GDBG_STATE="+debug.StateBase())
}

func assembleTimeline(avi *AVIFile, events *ParsedEvents, opts Options) ([][]byte, []byte, []ManifestClip, []ManifestMark, int, error) {
	availFrames := avi.FrameCount()

	if len(events.Clips) == 0 {
		end := availFrames
		if events.HasFinish && events.FinishFrame > 0 && events.FinishFrame < end {
			end = events.FinishFrame
		}
		if end <= 0 {
			return nil, nil, nil, nil, 0, errors.New("recording produced zero usable frames")
		}
		if events.FinishMissingDraws > 0 && !opts.AllowDroppedFrames {
			return nil, nil, nil, nil, events.FinishMissingDraws, fmt.Errorf("recording dropped %d rendered frames (pass --allow-dropped-frames to keep)", events.FinishMissingDraws)
		}
		frames, err := avi.ReadVideoFrames(0, end)
		if err != nil {
			return nil, nil, nil, nil, 0, err
		}
		var pcm []byte
		if !opts.Mute {
			pcm, err = avi.ReadAudioPCM16(0, end, opts.FPS)
			if err != nil {
				return nil, nil, nil, nil, 0, err
			}
		}
		var marks []ManifestMark
		for _, m := range events.Marks {
			if m.Frame >= 0 && m.Frame < end {
				marks = append(marks, ManifestMark{
					Name:     m.Name,
					ClipID:   m.ClipID,
					RawFrame: m.Frame,
					OutFrame: m.Frame,
					TimeSec:  float64(m.Frame) / float64(opts.FPS),
				})
			}
		}
		return frames, pcm, nil, marks, events.FinishMissingDraws, nil
	}

	var outSamples [][]byte
	var outPCM []byte
	var clips []ManifestClip
	var marks []ManifestMark
	totalMissing := 0

	for _, c := range events.Clips {
		if c.MissingDraws > 0 && !opts.AllowDroppedFrames {
			return nil, nil, nil, nil, c.MissingDraws, fmt.Errorf("clip %q dropped %d rendered frames (pass --allow-dropped-frames to keep)", c.ID, c.MissingDraws)
		}
		totalMissing += c.MissingDraws
		start := c.StartFrame
		end := c.EndFrame
		if start < 0 {
			start = 0
		}
		if end > availFrames {
			end = availFrames
		}
		if end <= start {
			return nil, nil, nil, nil, totalMissing, fmt.Errorf("clip %q has empty frame range [%d, %d) (available=%d)", c.ID, c.StartFrame, c.EndFrame, availFrames)
		}

		frames, err := avi.ReadVideoFrames(start, end)
		if err != nil {
			return nil, nil, nil, nil, totalMissing, fmt.Errorf("clip %q: %w", c.ID, err)
		}
		outStart := len(outSamples)
		outSamples = append(outSamples, frames...)
		outEnd := len(outSamples)

		if !opts.Mute {
			clipPCM, err := avi.ReadAudioPCM16(start, end, opts.FPS)
			if err != nil {
				return nil, nil, nil, nil, totalMissing, fmt.Errorf("clip %q audio: %w", c.ID, err)
			}
			outPCM = append(outPCM, clipPCM...)
		}

		clips = append(clips, ManifestClip{
			ID:            c.ID,
			Title:         c.Title,
			RawStartFrame: start,
			RawEndFrame:   end,
			OutStartFrame: outStart,
			OutEndFrame:   outEnd,
			Frames:        outEnd - outStart,
			MissingDraws:  c.MissingDraws,
		})

		for _, m := range events.Marks {
			if m.Frame >= start && m.Frame < end && (m.ClipID == "" || m.ClipID == c.ID) {
				outFrame := outStart + (m.Frame - start)
				marks = append(marks, ManifestMark{
					Name:     m.Name,
					ClipID:   c.ID,
					RawFrame: m.Frame,
					OutFrame: outFrame,
					TimeSec:  float64(outFrame) / float64(opts.FPS),
				})
			}
		}
	}

	return outSamples, outPCM, clips, marks, totalMissing, nil
}

func selectContactSheetFrames(samples [][]byte, marks []ManifestMark, fps int) ([][]byte, []string) {
	if len(samples) == 0 {
		return nil, nil
	}
	if len(marks) > 0 {
		frames := make([][]byte, 0, len(marks))
		labels := make([]string, 0, len(marks))
		for _, m := range marks {
			idx := m.OutFrame
			if idx < 0 {
				idx = 0
			}
			if idx >= len(samples) {
				idx = len(samples) - 1
			}
			frames = append(frames, samples[idx])
			labels = append(labels, fmt.Sprintf("%s %.2fs", m.Name, float64(idx)/float64(fps)))
		}
		return frames, labels
	}

	maxTiles := 9
	if len(samples) < maxTiles {
		maxTiles = len(samples)
	}
	frames := make([][]byte, 0, maxTiles)
	labels := make([]string, 0, maxTiles)
	for i := 0; i < maxTiles; i++ {
		idx := 0
		if maxTiles > 1 {
			idx = i * (len(samples) - 1) / (maxTiles - 1)
		}
		frames = append(frames, samples[idx])
		labels = append(labels, fmt.Sprintf("#%d %.2fs", idx, float64(idx)/float64(fps)))
	}
	return frames, labels
}

func logTail(data []byte, maxLines int) string {
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) == 0 || (len(lines) == 1 && lines[0] == "") {
		return ""
	}
	if len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
	}
	return "Log tail:\n" + strings.Join(lines, "\n")
}
