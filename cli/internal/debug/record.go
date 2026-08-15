// godot-debug-bridge/cli/internal/debug/record.go
package debug

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	// DefaultRecordSeconds matches the old client's default recording length.
	DefaultRecordSeconds = 10
	// DefaultRecordFPS matches the old client's default frame rate.
	DefaultRecordFPS = 4
	// DefaultRecordWidth/Height are applied when no resize option is given.
	DefaultRecordWidth  = 1920
	DefaultRecordHeight = 1080
	// MaxRecordSeconds and MaxRecordFPS bound the accepted recording options.
	MaxRecordSeconds = 300
	MaxRecordFPS     = 12
	// RecordBatchFrameLimit caps how many screenshot frames are requested per
	// bridge round trip.
	RecordBatchFrameLimit = 20
	// recordExtraTimeout is added to the batch timeout beyond the screenshot
	// formula to leave headroom for the game to write each frame.
	recordExtraTimeout = 5 * time.Second
)

// RecordOptions is the validated plan for one record invocation.
type RecordOptions struct {
	Seconds    float64
	FPS        int
	Output     string // empty means "use the default under $GDBG_STATE/debug/recordings"
	Screenshot map[string]any
}

// BuildRecordOpts validates record flags and fills defaults, mirroring the old
// client's bounds: seconds 1..300, integer fps 1..12, output ending in .mp4 or
// .mov, and a default 1920x1080 frame size when no resize option is given.
func BuildRecordOpts(f Flags) (RecordOptions, error) {
	seconds := float64(DefaultRecordSeconds)
	if f.HasSeconds {
		seconds = f.Seconds
	}
	if math.IsNaN(seconds) || seconds < 1 || seconds > MaxRecordSeconds {
		return RecordOptions{}, fmt.Errorf("record --seconds must be between 1 and %d", MaxRecordSeconds)
	}

	fps := float64(DefaultRecordFPS)
	if f.HasFPS {
		fps = f.FPS
	}
	if math.IsNaN(fps) || fps < 1 || fps > MaxRecordFPS || fps != math.Trunc(fps) {
		return RecordOptions{}, fmt.Errorf("record --fps must be between 1 and %d", MaxRecordFPS)
	}

	var output string
	if f.Output != "" {
		ext := strings.ToLower(filepath.Ext(f.Output))
		if ext != ".mp4" && ext != ".mov" {
			return RecordOptions{}, fmt.Errorf("record --output must end with .mp4 or .mov")
		}
		output = f.Output
	}

	return RecordOptions{
		Seconds:    seconds,
		FPS:        int(fps),
		Output:     output,
		Screenshot: recordScreenshotOpts(f),
	}, nil
}

// recordScreenshotOpts builds the screenshot args for frame capture, defaulting
// to 1920x1080 when no resize option is present.
func recordScreenshotOpts(f Flags) map[string]any {
	opts := map[string]any{}
	if f.HasDelay {
		opts["delay"] = f.Delay
	}
	if f.HasWidth {
		opts["width"] = f.Width
	}
	if f.HasHeight {
		opts["height"] = f.Height
	}
	if f.HasScale {
		opts["scale"] = f.Scale
	}
	if !f.HasWidth && !f.HasHeight && !f.HasScale {
		opts["width"] = DefaultRecordWidth
		opts["height"] = DefaultRecordHeight
	}
	return opts
}

// Sender is the injectable bridge call used by the record loop. The default
// implementation wraps Bridge.Send.
type Sender func(payload map[string]any, timeout time.Duration) (any, error)

// DefaultRecordOutput is the timestamped default .mp4 path under
// $GDBG_STATE/debug/recordings.
func DefaultRecordOutput() string {
	name := fmt.Sprintf("ai_recording_%s.mp4", recordTimestamp())
	return filepath.Join(RecordingsDir(), name)
}

func recordTimestamp() string {
	now := time.Now()
	return fmt.Sprintf(
		"%04d%02d%02d_%02d%02d%02d_%03d",
		now.Year(), int(now.Month()), now.Day(),
		now.Hour(), now.Minute(), now.Second(),
		now.Nanosecond()/int(time.Millisecond),
	)
}

// ExecuteRecord captures screenshot frames in batches of up to
// RecordBatchFrameLimit, stages them under $GDBG_STATE/debug/staging, encodes
// them in-process to mp4/mov, then removes the staging directory. It returns
// the recorded output path (as `result`), matching the old client's shape.
func ExecuteRecord(send Sender, opts RecordOptions) (any, error) {
	output := opts.Output
	if output == "" {
		output = DefaultRecordOutput()
	}
	absOutput, err := filepath.Abs(output)
	if err != nil {
		return nil, fmt.Errorf("cannot resolve output path %s: %w", output, err)
	}
	if err := os.MkdirAll(filepath.Dir(absOutput), 0o755); err != nil {
		return nil, fmt.Errorf("cannot create output directory: %w", err)
	}

	totalFrames := int(math.Ceil(opts.Seconds * float64(opts.FPS)))
	if totalFrames < 1 {
		totalFrames = 1
	}
	interval := 1.0 / float64(opts.FPS)

	staging := filepath.Join(StagingDir(), fmt.Sprintf("record-%d", time.Now().UnixNano()))
	if err := os.MkdirAll(staging, 0o755); err != nil {
		return nil, fmt.Errorf("cannot create staging directory: %w", err)
	}
	defer os.RemoveAll(staging)

	var frames []string
	remaining := totalFrames
	firstBatch := true
	for remaining > 0 {
		count := RecordBatchFrameLimit
		if remaining < count {
			count = remaining
		}

		args := map[string]any{}
		for k, v := range opts.Screenshot {
			args[k] = v
		}
		args["count"] = count
		args["interval"] = interval
		if !firstBatch {
			delete(args, "delay")
		}
		firstBatch = false

		delay := 0.0
		if d, ok := args["delay"].(float64); ok {
			delay = d
		}
		timeout := time.Duration((delay + float64(count-1)*interval) * float64(time.Second))
		timeout += DefaultTimeout + recordExtraTimeout

		result, err := send(map[string]any{"cmd": "screenshot", "args": args}, timeout)
		if err != nil {
			return nil, err
		}
		batchFrames, err := NormalizeScreenshotPaths(result)
		if err != nil {
			return nil, err
		}
		if len(batchFrames) != count {
			return nil, fmt.Errorf("record expected %d frames but captured %d", count, len(batchFrames))
		}
		for _, frame := range batchFrames {
			staged, err := stageRecordFrame(staging, frame, len(frames))
			if err != nil {
				return nil, err
			}
			frames = append(frames, staged)
		}
		remaining -= count
	}

	if err := EncodeFramesToVideo(frames, absOutput, opts.FPS); err != nil {
		return nil, err
	}
	return map[string]any{"result": absOutput}, nil
}

// stageRecordFrame copies a captured frame into the staging directory with a
// zero-padded sequence number so the encoder sees frames in capture order.
func stageRecordFrame(stagingDir, source string, index int) (string, error) {
	ext := filepath.Ext(source)
	if ext == "" {
		ext = ".png"
	}
	dest := filepath.Join(stagingDir, fmt.Sprintf("%05d%s", index, ext))
	data, err := os.ReadFile(source)
	if err != nil {
		return "", fmt.Errorf("cannot read captured frame %s: %w", source, err)
	}
	if err := os.WriteFile(dest, data, 0o644); err != nil {
		return "", fmt.Errorf("cannot stage frame %s: %w", dest, err)
	}
	return dest, nil
}
