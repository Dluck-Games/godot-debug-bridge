package record

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func writeTestPNG(t *testing.T, dir, name string, w, h int, shade uint8) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: shade, G: shade, B: shade, A: 255})
		}
	}
	p := filepath.Join(dir, name)
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	return p
}

func makeJPEGFrame(t *testing.T, w, h int, c color.RGBA) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

type rawBox struct {
	typ     string
	offset  int
	payload []byte
}

func topBoxes(data []byte) ([]rawBox, error) {
	var out []rawBox
	off := 0
	for off+8 <= len(data) {
		size := binary.BigEndian.Uint32(data[off : off+4])
		typ := string(data[off+4 : off+8])
		total := uint64(size)
		hdr := 8
		if size == 1 {
			if off+16 > len(data) {
				return nil, errors.New("truncated 64-bit box")
			}
			total = binary.BigEndian.Uint64(data[off+8 : off+16])
			hdr = 16
		}
		if total < uint64(hdr) || off+int(total) > len(data) {
			return nil, errors.New("box out of range")
		}
		out = append(out, rawBox{typ: typ, offset: off, payload: data[off+hdr : off+int(total)]})
		off += int(total)
	}
	return out, nil
}

func findBox(data []byte, typ string) ([]byte, bool) {
	boxes, err := topBoxes(data)
	if err != nil {
		return nil, false
	}
	for _, b := range boxes {
		if b.typ == typ {
			return b.payload, true
		}
		sub := b.payload
		if b.typ == "stsd" && len(sub) >= 8 {
			sub = sub[8:]
		}
		if p, ok := findBox(sub, typ); ok {
			return p, true
		}
	}
	return nil, false
}

func checkPlayableMedia(t *testing.T, path string, wantAudio bool) {
	t.Helper()
	if ffprobe, err := exec.LookPath("ffprobe"); err == nil {
		out, err := exec.Command(ffprobe, "-v", "error",
			"-show_entries", "stream=codec_type,codec_name,width,height,sample_rate,channels",
			"-of", "csv=p=0", path).Output()
		if err != nil {
			t.Fatalf("ffprobe rejected %s: %v\n%s", path, err, out)
		}
		text := string(out)
		if !strings.Contains(text, "video") {
			t.Errorf("ffprobe missing video stream: %q", text)
		}
		if wantAudio && !strings.Contains(text, "audio") {
			t.Errorf("ffprobe missing audio stream: %q", text)
		}
	}
	if ffmpeg, err := exec.LookPath("ffmpeg"); err == nil {
		out, err := exec.Command(ffmpeg, "-v", "error", "-i", path, "-f", "null", "-").CombinedOutput()
		if err != nil {
			t.Fatalf("ffmpeg could not decode %s: %v\n%s", path, err, out)
		}
	}
}

func TestEncodeFramesToVideo_Faststart(t *testing.T) {
	dir := t.TempDir()
	frames := []string{
		writeTestPNG(t, dir, "f0.png", 64, 48, 200),
		writeTestPNG(t, dir, "f1.png", 64, 48, 120),
		writeTestPNG(t, dir, "f2.png", 64, 48, 40),
	}
	for _, ext := range []string{".mp4", ".mov"} {
		out := filepath.Join(t.TempDir(), "rec"+ext)
		if err := EncodeFramesToVideo(frames, out, 4); err != nil {
			t.Fatalf("EncodeFramesToVideo(%s): %v", ext, err)
		}
		data, err := os.ReadFile(out)
		if err != nil {
			t.Fatal(err)
		}
		boxes, err := topBoxes(data)
		if err != nil {
			t.Fatalf("topBoxes(%s): %v", ext, err)
		}
		if len(boxes) != 3 || boxes[0].typ != "ftyp" || boxes[1].typ != "moov" || boxes[2].typ != "mdat" {
			t.Fatalf("faststart box order wrong for %s: %v", ext, boxes)
		}
		checkPlayableMedia(t, out, false)
	}
}

func TestEncodeFramesToVideo_EdgeCases(t *testing.T) {
	dir := t.TempDir()
	single := []string{writeTestPNG(t, dir, "only.png", 16, 16, 150)}
	out := filepath.Join(t.TempDir(), "one.mp4")
	if err := EncodeFramesToVideo(single, out, 2); err != nil {
		t.Fatalf("single frame: %v", err)
	}
	checkPlayableMedia(t, out, false)

	if err := EncodeFramesToVideo(nil, filepath.Join(t.TempDir(), "empty.mp4"), 4); err == nil {
		t.Fatal("expected error for empty frames")
	}
	if err := EncodeFramesToVideo(single, filepath.Join(t.TempDir(), "bad.avi"), 4); err == nil || !strings.Contains(err.Error(), ".mp4 or .mov") {
		t.Fatalf("expected extension error, got %v", err)
	}
	inconsistent := []string{
		writeTestPNG(t, dir, "wide.png", 64, 48, 100),
		writeTestPNG(t, dir, "tall.png", 48, 64, 100),
	}
	if err := EncodeFramesToVideo(inconsistent, filepath.Join(t.TempDir(), "mismatch.mp4"), 4); err == nil || !strings.Contains(err.Error(), "inconsistent dimensions") {
		t.Fatalf("expected dimension error, got %v", err)
	}
}

func TestMuxSamples_WithAudioAndTranscode(t *testing.T) {
	const (
		w          = 160
		h          = 120
		fps        = 30
		sampleRate = 48000
		channels   = 2
		nFrames    = 30
	)
	video := make([][]byte, 0, nFrames)
	for i := 0; i < nFrames; i++ {
		video = append(video, makeJPEGFrame(t, w, h, color.RGBA{R: uint8(i * 8), G: 180, B: 60, A: 255}))
	}
	totalSamples := (nFrames * sampleRate) / fps
	pcm := make([]byte, totalSamples*channels*2)
	for i := 0; i < totalSamples; i++ {
		val := int16(math.Sin(2*math.Pi*440*float64(i)/float64(sampleRate)) * 8000)
		binary.LittleEndian.PutUint16(pcm[i*4:], uint16(val))
		binary.LittleEndian.PutUint16(pcm[i*4+2:], uint16(val))
	}

	movPath := filepath.Join(t.TempDir(), "assembled.mov")
	err := MuxSamples(MuxConfig{
		FPS:    fps,
		Width:  w,
		Height: h,
		Audio: &MuxAudio{
			Channels:   channels,
			SampleRate: sampleRate,
			PCM16:      pcm,
		},
	}, video, movPath)
	if err != nil {
		t.Fatalf("MuxSamples: %v", err)
	}
	data, err := os.ReadFile(movPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := findBox(data, "sowt"); !ok {
		t.Fatal("missing sowt audio sample entry")
	}
	checkPlayableMedia(t, movPath, true)

	if runtime.GOOS == "darwin" {
		mp4Path := filepath.Join(t.TempDir(), "final.mp4")
		codec, err := TranscodeToH264(movPath, mp4Path, CodecH264)
		if err != nil {
			t.Fatalf("TranscodeToH264: %v", err)
		}
		if codec != "h264" {
			t.Fatalf("codec = %q, want h264", codec)
		}
		checkPlayableMedia(t, mp4Path, true)
		if ffprobe, err := exec.LookPath("ffprobe"); err == nil {
			out, err := exec.Command(ffprobe, "-v", "error", "-count_frames",
				"-select_streams", "v:0",
				"-show_entries", "stream=codec_name,width,height,r_frame_rate,avg_frame_rate,nb_read_frames",
				"-of", "csv=p=0", mp4Path).Output()
			if err != nil {
				t.Fatalf("ffprobe h264 details: %v", err)
			}
			t.Logf("avconvert h264+aac stream info: %s", strings.TrimSpace(string(out)))
			fields := strings.Split(strings.TrimSpace(string(out)), ",")
			if len(fields) < 6 || fields[0] != "h264" || fields[1] != "160" || fields[2] != "120" || fields[5] != "30" {
				t.Fatalf("unexpected avconvert stream fields: %v", fields)
			}
		}

		// Also verify mute (video-only) transcode on darwin.
		movMute := filepath.Join(t.TempDir(), "mute.mov")
		if err := MuxSamples(MuxConfig{FPS: fps, Width: w, Height: h}, video, movMute); err != nil {
			t.Fatalf("MuxSamples mute: %v", err)
		}
		mp4Mute := filepath.Join(t.TempDir(), "mute.mp4")
		codecMute, err := TranscodeToH264(movMute, mp4Mute, CodecH264)
		if err != nil {
			t.Fatalf("TranscodeToH264 mute: %v", err)
		}
		if codecMute != "h264" {
			t.Fatalf("mute codec = %q, want h264", codecMute)
		}
		checkPlayableMedia(t, mp4Mute, false)
	}
}
