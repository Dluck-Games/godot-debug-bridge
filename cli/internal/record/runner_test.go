package record

import (
	"bytes"
	"encoding/binary"
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseEventsFile_AndAssembleTimeline(t *testing.T) {
	eventsPath := filepath.Join(t.TempDir(), "events.jsonl")
	content := strings.Join([]string{
		`{"type":"start","fps":30,"offscreen":"force"}`,
		`{"type":"clip_begin","id":"intro","title":"Intro Clip","frame":2,"missing_draws":0}`,
		`{"type":"mark","name":"mid","clip_id":"intro","frame":3}`,
		`{"type":"clip_end","id":"intro","frame":5,"missing_draws":0}`,
		`{"type":"clip_begin","id":"action","frame":7,"missing_draws":0}`,
		`{"type":"clip_end","id":"action","frame":9,"missing_draws":0}`,
		`{"type":"finish","frame":10,"missing_draws":0,"exit_code":0}`,
	}, "\n")
	if err := os.WriteFile(eventsPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	events, err := ParseEventsFile(eventsPath)
	if err != nil {
		t.Fatalf("ParseEventsFile: %v", err)
	}
	if len(events.Clips) != 2 || len(events.Marks) != 1 || !events.HasFinish {
		t.Fatalf("unexpected parsed events: %+v", events)
	}

	aviPath := filepath.Join(t.TempDir(), "dummy.avi")
	avi := writeDummyMJPEGAVI(t, aviPath, 64, 48, 30, 10)

	samples, _, clips, marks, dropped, err := assembleTimeline(avi, events, Options{FPS: 30, Mute: true})
	if err != nil {
		t.Fatalf("assembleTimeline: %v", err)
	}
	if dropped != 0 || len(samples) != 5 {
		t.Fatalf("samples=%d dropped=%d, want 5/0", len(samples), dropped)
	}
	if len(clips) != 2 || clips[0].OutStartFrame != 0 || clips[0].OutEndFrame != 3 || clips[1].OutStartFrame != 3 || clips[1].OutEndFrame != 5 {
		t.Fatalf("clips = %+v", clips)
	}
	if len(marks) != 1 || marks[0].OutFrame != 1 {
		t.Fatalf("marks = %+v", marks)
	}
}

func TestAssembleTimeline_FailsOnDroppedFrames(t *testing.T) {
	aviPath := filepath.Join(t.TempDir(), "dummy.avi")
	avi := writeDummyMJPEGAVI(t, aviPath, 64, 48, 30, 5)
	events := &ParsedEvents{
		Clips: []RawClip{{ID: "bad", StartFrame: 0, EndFrame: 4, MissingDraws: 2}},
	}
	if _, _, _, _, _, err := assembleTimeline(avi, events, Options{FPS: 30, Mute: true}); err == nil || !strings.Contains(err.Error(), "dropped 2 rendered frames") {
		t.Fatalf("expected dropped frame error, got %v", err)
	}
	samples, _, _, _, dropped, err := assembleTimeline(avi, events, Options{FPS: 30, Mute: true, AllowDroppedFrames: true})
	if err != nil || len(samples) != 4 || dropped != 2 {
		t.Fatalf("AllowDroppedFrames failed: err=%v len=%d dropped=%d", err, len(samples), dropped)
	}
}

func TestOptionsValidate_AndBuildGodotArgs(t *testing.T) {
	proj := t.TempDir()
	script := filepath.Join(t.TempDir(), "director.gd")
	if err := os.WriteFile(script, []byte("extends Node\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GDBG_STATE", t.TempDir())

	opts := Options{
		ProjectDir: proj,
		ScriptPath: script,
		ExtraArgs:  []string{"--skip-menu"},
	}
	if err := opts.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if opts.FPS != DefaultFPS || opts.Resolution != DefaultResolution || opts.Offscreen != "auto" {
		t.Fatalf("defaults wrong: %+v", opts)
	}
	args := BuildGodotArgs(opts, "/tmp/raw.avi", "/tmp/events.jsonl")
	joined := strings.Join(args, " ")
	for _, want := range []string{"--windowed", "--resolution 1280x720", "--fixed-fps 30", "--write-movie /tmp/raw.avi", "--gdbg-record-manifest=/tmp/events.jsonl", "--gdbg-record-script=", "--skip-menu"} {
		if !strings.Contains(joined, want) {
			t.Errorf("BuildGodotArgs missing %q in %q", want, joined)
		}
	}
	if strings.Contains(joined, "--audio-driver") {
		t.Errorf("BuildGodotArgs should not force Dummy audio driver: %q", joined)
	}
}

func writeDummyMJPEGAVI(t *testing.T, path string, w, h, fps, count int) *AVIFile {
	t.Helper()
	avih := make([]byte, 56)
	binary.LittleEndian.PutUint32(avih[0:4], uint32(1_000_000/fps))
	binary.LittleEndian.PutUint32(avih[16:20], uint32(count))
	binary.LittleEndian.PutUint32(avih[24:28], 1)
	binary.LittleEndian.PutUint32(avih[32:36], uint32(w))
	binary.LittleEndian.PutUint32(avih[36:40], uint32(h))

	strlV := riffList("LIST", "strl", append(
		riffChunk("strh", makeAVIStreamHeader("vids", "MJPG", 1, uint32(fps), uint32(count))),
		riffChunk("strf", makeAVIBitmapInfo(w, h))...,
	))
	var hdrlBody bytes.Buffer
	hdrlBody.Write(riffChunk("avih", avih))
	hdrlBody.Write(strlV)
	hdrl := riffList("LIST", "hdrl", hdrlBody.Bytes())

	var movi bytes.Buffer
	for i := 0; i < count; i++ {
		jpg := makeJPEGFrame(t, w, h, color.RGBA{R: uint8(i * 20), G: 100, B: 200, A: 255})
		movi.Write(riffChunk("00dc", jpg))
	}

	var raw bytes.Buffer
	raw.Write(riffList("RIFF", "AVI ", append(hdrl, riffList("LIST", "movi", movi.Bytes())...)))
	if err := os.WriteFile(path, raw.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	avi, err := OpenAVI(path)
	if err != nil {
		t.Fatalf("OpenAVI dummy: %v", err)
	}
	return avi
}
