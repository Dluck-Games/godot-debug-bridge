package cmd

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/Dluck-Games/godot-debug-bridge/cli/internal/godot"
	"github.com/Dluck-Games/godot-debug-bridge/cli/internal/record"
)

var (
	flagRecOutput             string
	flagRecFPS                int
	flagRecSeconds            float64
	flagRecResolution         string
	flagRecWidth              int
	flagRecHeight             int
	flagRecScaleFilter        string
	flagRecScene              string
	flagRecMute               bool
	flagRecCodec              string
	flagRecQuality            int
	flagRecOffscreen          string
	flagRecContactSheet       string
	flagRecManifest           string
	flagRecMaxFrames          int
	flagRecTimeout            time.Duration
	flagRecNoWindowLock       bool
	flagRecAllowDroppedFrames bool
	flagRecKeepRaw            bool
)

var recordCmd = &cobra.Command{
	Use:   "record [director.gd] [flags] [-- <game-args...>]",
	Short: "Record deterministic video via Godot Movie Maker",
	Long: `Run a project scene under Godot Movie Maker (--fixed-fps + --write-movie)
with an optional external GDScript director, zero-drop offscreen rendering,
clip trimming, pure-Go container muxing, and built-in H.264/AAC transcoding on macOS.

Requires no ffmpeg, Python, or third-party media tools.

Examples:
  gdbg record --seconds 5 --fps 30 -o demo.mp4 -- --skip-menu
  gdbg record director.gd --fps 60 --width 1280 --height 720 -o demo.mp4 --contact-sheet keyframes.jpg -- --skip-menu`,
	Args:         cobra.ArbitraryArgs,
	SilenceUsage: true,
	RunE:         recordRun,
}

func init() {
	rootCmd.AddCommand(recordCmd)
	f := recordCmd.Flags()
	f.StringVarP(&flagRecOutput, "output", "o", "", "output .mp4 or .mov path (default: $GDBG_STATE/debug/recordings/...)")
	f.IntVar(&flagRecFPS, "fps", record.DefaultFPS, "fixed recording frame rate (1-120)")
	f.Float64Var(&flagRecSeconds, "seconds", record.DefaultSeconds, "recording duration in seconds when no director script is given")
	f.StringVar(&flagRecResolution, "resolution", record.DefaultResolution, "Godot window resolution <W>x<H>")
	f.IntVar(&flagRecWidth, "width", 0, "optional output width (preserves aspect if --height is omitted)")
	f.IntVar(&flagRecHeight, "height", 0, "optional output height (preserves aspect if --width is omitted)")
	f.StringVar(&flagRecScaleFilter, "scale-filter", string(record.ScaleBilinear), "resize filter: bilinear or nearest")
	f.StringVar(&flagRecScene, "scene", "", "optional res:// scene to launch instead of project main_scene")
	f.BoolVar(&flagRecMute, "mute", false, "omit the audio track from the output file")
	f.StringVar(&flagRecCodec, "codec", string(record.CodecAuto), "output video codec: auto (H.264 on macOS, MJPEG otherwise), h264, or mjpeg")
	f.IntVar(&flagRecQuality, "quality", record.DefaultQuality, "JPEG quality when resizing frames (1-100)")
	f.StringVar(&flagRecOffscreen, "offscreen", "auto", "offscreen capture mode: auto, force, or off")
	f.StringVar(&flagRecContactSheet, "contact-sheet", "", "optional output JPEG path for keyframe contact sheet")
	f.StringVar(&flagRecManifest, "manifest", "", "optional output JSON path for clip/mark manifest")
	f.IntVar(&flagRecMaxFrames, "max-frames", 0, "safety cap passed to Godot --quit-after (0 = automatic)")
	f.DurationVar(&flagRecTimeout, "timeout", record.DefaultTimeout, "maximum wall-clock duration for the Godot recording run")
	f.BoolVar(&flagRecNoWindowLock, "no-window-lock", false, "do not acquire the host-wide GUI window lock")
	f.BoolVar(&flagRecAllowDroppedFrames, "allow-dropped-frames", false, "do not fail if offscreen frame verification detects missing draws")
	f.BoolVar(&flagRecKeepRaw, "keep-raw", false, "keep the staging directory (raw.avi, godot.log, events.jsonl)")
}

func recordRun(cmd *cobra.Command, args []string) error {
	projectDir, err := resolveProject()
	if err != nil {
		return err
	}

	opts, err := buildRecordOptions(projectDir, args, cmd.ArgsLenAtDash())
	if err != nil {
		return err
	}

	godotBin, err := godot.FindBinary()
	if err != nil {
		return err
	}

	res, err := record.Run(godotBin, opts)
	if err != nil {
		return err
	}

	printRecordSummary(res)
	return nil
}

func buildRecordOptions(projectDir string, args []string, dashIndex int) (record.Options, error) {
	var scriptPath string
	var extraArgs []string

	if dashIndex == -1 {
		if len(args) > 1 {
			return record.Options{}, fmt.Errorf("unexpected extra arguments %v; pass game arguments after '--'", args[1:])
		}
		if len(args) == 1 {
			scriptPath = args[0]
		}
	} else {
		before := args[:dashIndex]
		if len(before) > 1 {
			return record.Options{}, fmt.Errorf("unexpected extra arguments %v before '--'; pass at most one director script path", before[1:])
		}
		if len(before) == 1 {
			scriptPath = before[0]
		}
		extraArgs = args[dashIndex:]
	}

	filter, err := record.ParseScaleFilter(flagRecScaleFilter)
	if err != nil {
		return record.Options{}, err
	}
	codec, err := record.ParseCodec(flagRecCodec)
	if err != nil {
		return record.Options{}, err
	}

	opts := record.Options{
		ProjectDir:         projectDir,
		ScriptPath:         scriptPath,
		Output:             flagRecOutput,
		FPS:                flagRecFPS,
		Seconds:            flagRecSeconds,
		Resolution:         flagRecResolution,
		Width:              flagRecWidth,
		Height:             flagRecHeight,
		ScaleFilter:        filter,
		Scene:              flagRecScene,
		Mute:               flagRecMute,
		Codec:              codec,
		Quality:            flagRecQuality,
		Offscreen:          flagRecOffscreen,
		ContactSheet:       flagRecContactSheet,
		ManifestPath:       flagRecManifest,
		MaxFrames:          flagRecMaxFrames,
		Timeout:            flagRecTimeout,
		NoWindowLock:       flagRecNoWindowLock,
		AllowDroppedFrames: flagRecAllowDroppedFrames,
		KeepRaw:            flagRecKeepRaw,
		Verbose:            flagVerbose,
		ExtraArgs:          extraArgs,
	}
	if err := opts.Validate(); err != nil {
		return record.Options{}, err
	}
	return opts, nil
}

func printRecordSummary(res *record.Result) {
	m := res.Manifest
	audioLabel := "none"
	if m.HasAudio {
		if m.Codec == "h264" {
			audioLabel = "aac"
		} else {
			audioLabel = "pcm"
		}
	}
	fmt.Printf("Recorded: %s (%dx%d @ %d fps, %d frames, %.2fs, codec=%s, audio=%s, clips=%d)\n",
		res.Output, m.Width, m.Height, m.FPS, m.TotalFrames, m.DurationSeconds, m.Codec, audioLabel, len(m.Clips))
	if res.ContactSheet != "" {
		fmt.Printf("Contact sheet: %s\n", res.ContactSheet)
	}
	if res.ManifestPath != "" {
		fmt.Printf("Manifest: %s\n", res.ManifestPath)
	}
	if res.StagingDir != "" {
		fmt.Printf("Staging kept: %s\n", res.StagingDir)
	}
}
