package record

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Codec selects the output video encoding mode.
type Codec string

const (
	CodecAuto  Codec = "auto"
	CodecH264  Codec = "h264"
	CodecMJPEG Codec = "mjpeg"
)

// ParseCodec validates a codec flag value.
func ParseCodec(s string) (Codec, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "auto":
		return CodecAuto, nil
	case "h264":
		return CodecH264, nil
	case "mjpeg":
		return CodecMJPEG, nil
	default:
		return "", fmt.Errorf("invalid codec %q (use auto, h264, or mjpeg)", s)
	}
}

type rawEvent struct {
	Type         string `json:"type"`
	ID           string `json:"id"`
	Title        string `json:"title"`
	Name         string `json:"name"`
	ClipID       string `json:"clip_id"`
	Frame        int    `json:"frame"`
	ProcessFrame int    `json:"process_frame"`
	PostDraw     int    `json:"post_draw"`
	MissingDraws int    `json:"missing_draws"`
	ExitCode     int    `json:"exit_code"`
	FPS          int    `json:"fps"`
	Offscreen    string `json:"offscreen"`
	Message      string `json:"message"`
}

// RawClip represents one raw AVI frame slice `[StartFrame, EndFrame)`.
type RawClip struct {
	ID           string
	Title        string
	StartFrame   int
	EndFrame     int
	MissingDraws int
}

// RawMark represents a marker at a raw AVI frame index.
type RawMark struct {
	Name   string
	ClipID string
	Frame  int
}

// ParsedEvents is the structured timeline parsed from events.jsonl.
type ParsedEvents struct {
	FPS                int
	Offscreen          string
	Clips              []RawClip
	Marks              []RawMark
	HasFinish          bool
	FinishFrame        int
	FinishMissingDraws int
	ExitCode           int
	ErrorMessage       string
}

// ManifestClip describes one assembled clip in the final video.
type ManifestClip struct {
	ID            string `json:"id"`
	Title         string `json:"title,omitempty"`
	RawStartFrame int    `json:"raw_start_frame"`
	RawEndFrame   int    `json:"raw_end_frame"`
	OutStartFrame int    `json:"out_start_frame"`
	OutEndFrame   int    `json:"out_end_frame"`
	Frames        int    `json:"frames"`
	MissingDraws  int    `json:"missing_draws"`
}

// ManifestMark describes a marker mapped to its output frame index.
type ManifestMark struct {
	Name     string  `json:"name"`
	ClipID   string  `json:"clip_id,omitempty"`
	RawFrame int     `json:"raw_frame"`
	OutFrame int     `json:"out_frame"`
	TimeSec  float64 `json:"time_seconds"`
}

// Manifest is the machine-readable metadata emitted for a completed recording.
type Manifest struct {
	Output          string         `json:"output"`
	FPS             int            `json:"fps"`
	Width           int            `json:"width"`
	Height          int            `json:"height"`
	HasAudio        bool           `json:"has_audio"`
	Codec           string         `json:"codec"`
	TotalFrames     int            `json:"total_frames"`
	DurationSeconds float64        `json:"duration_seconds"`
	MissingDraws    int            `json:"missing_draws"`
	Clips           []ManifestClip `json:"clips,omitempty"`
	Marks           []ManifestMark `json:"marks,omitempty"`
}

// ParseEventsFile reads the JSONL event log produced by DebugBridgeRecordSession.
func ParseEventsFile(path string) (*ParsedEvents, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open events manifest: %w", err)
	}
	defer f.Close()

	parsed := &ParsedEvents{}
	var openClip *RawClip
	var openClipStartMissing int

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var ev rawEvent
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			return nil, fmt.Errorf("parse event line %q: %w", line, err)
		}
		switch ev.Type {
		case "start":
			parsed.FPS = ev.FPS
			parsed.Offscreen = ev.Offscreen
		case "clip_begin":
			if openClip != nil {
				openClip.EndFrame = ev.Frame
				openClip.MissingDraws = maxInt(0, ev.MissingDraws-openClipStartMissing)
				parsed.Clips = append(parsed.Clips, *openClip)
			}
			c := RawClip{
				ID:         ev.ID,
				Title:      ev.Title,
				StartFrame: ev.Frame,
			}
			openClip = &c
			openClipStartMissing = ev.MissingDraws
		case "clip_end":
			if openClip != nil {
				openClip.EndFrame = ev.Frame
				openClip.MissingDraws = maxInt(0, ev.MissingDraws-openClipStartMissing)
				parsed.Clips = append(parsed.Clips, *openClip)
				openClip = nil
			}
		case "mark":
			parsed.Marks = append(parsed.Marks, RawMark{
				Name:   ev.Name,
				ClipID: ev.ClipID,
				Frame:  ev.Frame,
			})
		case "finish":
			if openClip != nil {
				openClip.EndFrame = ev.Frame
				openClip.MissingDraws = maxInt(0, ev.MissingDraws-openClipStartMissing)
				parsed.Clips = append(parsed.Clips, *openClip)
				openClip = nil
			}
			parsed.HasFinish = true
			parsed.FinishFrame = ev.Frame
			parsed.FinishMissingDraws = ev.MissingDraws
			parsed.ExitCode = ev.ExitCode
		case "error":
			parsed.ErrorMessage = ev.Message
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan events manifest: %w", err)
	}
	return parsed, nil
}

// WriteManifest writes m as indented JSON to path.
func WriteManifest(path string, m Manifest) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create manifest directory: %w", err)
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("encode manifest: %w", err)
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("create destination directory: %w", err)
	}
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open source file %s: %w", src, err)
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("create destination file %s: %w", dst, err)
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		_ = os.Remove(dst)
		return fmt.Errorf("copy file to %s: %w", dst, err)
	}
	return out.Close()
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
