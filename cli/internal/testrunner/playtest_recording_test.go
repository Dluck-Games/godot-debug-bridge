// godot-debug-bridge/cli/internal/testrunner/playtest_recording_test.go
package testrunner

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeFramePNG writes a solid-shade 32x24 PNG named name into dir. Distinct
// shades let tests assert that the recorder frames were encoded in order and
// that checkpoint screenshots were excluded.
func writeFramePNG(t *testing.T, dir, name string, shade uint8) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 32, 24))
	for y := 0; y < 24; y++ {
		for x := 0; x < 32; x++ {
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
}

// findBox returns the payload of the first box of typ found by recursively
// walking the MP4 box tree in data.
func findBox(data []byte, typ string) ([]byte, bool) {
	for off := 0; off+8 <= len(data); {
		size := binary.BigEndian.Uint32(data[off : off+4])
		hdr := 8
		total := uint64(size)
		if size == 0 {
			total = uint64(len(data) - off)
		}
		if int(total) < hdr || off+int(total) > len(data) {
			return nil, false
		}
		if string(data[off+4:off+8]) == typ {
			return data[off+hdr : off+int(total)], true
		}
		if payload, ok := findBox(data[off+hdr:off+int(total)], typ); ok {
			return payload, true
		}
		off += int(total)
	}
	return nil, false
}

// sampleCount reads the stsz sample-count entry from a muxed recording.
func sampleCount(data []byte) (int, bool) {
	stsz, ok := findBox(data, "stsz")
	if !ok || len(stsz) < 12 {
		return 0, false
	}
	return int(binary.BigEndian.Uint32(stsz[8:12])), true
}

// jpegSamples splits the mdat payload into its individual JPEG samples by
// scanning JPEG SOI/EOI markers, so each recorder frame can be decoded.
func jpegSamples(mdat []byte) ([][]byte, error) {
	var samples [][]byte
	start := -1
	for i := 0; i+1 < len(mdat); i++ {
		if mdat[i] == jpegSOI[0] && mdat[i+1] == jpegSOI[1] {
			start = i
		} else if mdat[i] == jpegEOI[0] && mdat[i+1] == jpegEOI[1] && start >= 0 {
			samples = append(samples, mdat[start:i+2])
			start = -1
		}
	}
	if len(samples) == 0 {
		return nil, errors.New("no jpeg samples in mdat")
	}
	return samples, nil
}

var (
	jpegSOI = []byte{0xFF, 0xD8}
	jpegEOI = []byte{0xFF, 0xD9}
)

// jpegCenterShade decodes a JPEG sample and returns the shade at its center
// pixel (flattened onto an 8-bit value).
func jpegCenterShade(sample []byte) (uint8, error) {
	img, err := jpeg.Decode(bytes.NewReader(sample))
	if err != nil {
		return 0, err
	}
	b := img.Bounds()
	r, _, _, _ := img.At(b.Min.X+b.Dx()/2, b.Min.Y+b.Dy()/2).RGBA()
	return uint8(r >> 8), nil
}

// shadeClose reports whether two shades are equal within JPEG re-encode noise.
func shadeClose(a, b uint8) bool {
	d := int(a) - int(b)
	if d < 0 {
		d = -d
	}
	return d <= 10
}

// TestEncodePlaytestRecording_EncodesOnlyRecorderFrames proves that when a
// frame directory contains both checkpoint screenshots and continuous recorder
// frames, only the three recorder samples are encoded in lexical order.
func TestEncodePlaytestRecording_EncodesOnlyRecorderFrames(t *testing.T) {
	dir := t.TempDir()
	// Checkpoint screenshots that must NOT be substituted for a recording.
	writeFramePNG(t, dir, "checkpoint_hub.png", 90)
	writeFramePNG(t, dir, "checkpoint_city.png", 100)
	writeFramePNG(t, dir, "checkpoint_boss.png", 110)
	// Continuous recorder samples, intentionally distinct and well separated so
	// the decoded shades confirm both exclusion and ordering.
	shades := []uint8{210, 130, 30}
	writeFramePNG(t, dir, "frame_00000.png", shades[0])
	writeFramePNG(t, dir, "frame_00001.png", shades[1])
	writeFramePNG(t, dir, "frame_00002.png", shades[2])

	out := filepath.Join(t.TempDir(), "recording.mp4")
	if err := encodePlaytestRecording(dir, out); err != nil {
		t.Fatalf("encodePlaytestRecording: %v", err)
	}

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read recording: %v", err)
	}
	if n, ok := sampleCount(data); !ok || n != 3 {
		t.Fatalf("stsz sample count = %v (ok=%v), want 3 (checkpoints leaked)", n, ok)
	}

	mdat, ok := findBox(data, "mdat")
	if !ok {
		t.Fatal("missing mdat box")
	}
	samples, err := jpegSamples(mdat)
	if err != nil {
		t.Fatalf("split mdat: %v", err)
	}
	if len(samples) != 3 {
		t.Fatalf("decoded %d jpeg samples, want 3", len(samples))
	}
	for i, s := range samples {
		got, err := jpegCenterShade(s)
		if err != nil {
			t.Fatalf("decode sample %d: %v", i, err)
		}
		if !shadeClose(got, shades[i]) {
			t.Errorf("frame %d shade = %d, want ~%d (checkpoint leaked or wrong order)", i, got, shades[i])
		}
	}
}

// TestEncodePlaytestRecording_FailsClosedWithoutRecorderFrames proves that when
// only checkpoint screenshots exist and no frame_*.png is present, encoding
// returns the no-recorded-frames error and leaves no recording behind.
func TestEncodePlaytestRecording_FailsClosedWithoutRecorderFrames(t *testing.T) {
	dir := t.TempDir()
	writeFramePNG(t, dir, "checkpoint_hub.png", 90)
	writeFramePNG(t, dir, "checkpoint_city.png", 100)
	writeFramePNG(t, dir, "checkpoint_boss.png", 110)

	out := filepath.Join(t.TempDir(), "recording.mp4")
	err := encodePlaytestRecording(dir, out)
	if err == nil {
		t.Fatal("expected error when no frame_*.png present, got nil")
	}
	if !strings.Contains(err.Error(), "no recorded playtest frames") {
		t.Errorf("error = %q, want substring %q", err.Error(), "no recorded playtest frames")
	}
	if _, statErr := os.Stat(out); statErr == nil {
		t.Error("recording file was created despite missing recorder frames")
	}
}

// TestEncodePlaytestRecording_PreservesZeroPaddedOrder proves that recorder
// frames are encoded in lexical (capture) order even when the filesystem
// returns them out of order: frame_00000 precedes frame_00001 and frame_00010.
func TestEncodePlaytestRecording_PreservesZeroPaddedOrder(t *testing.T) {
	dir := t.TempDir()
	// Write out of lexical order; glob+sort must restore capture order.
	writeFramePNG(t, dir, "frame_00010.png", 30)
	writeFramePNG(t, dir, "frame_00001.png", 130)
	writeFramePNG(t, dir, "frame_00000.png", 210)
	want := []uint8{210, 130, 30}

	out := filepath.Join(t.TempDir(), "recording.mp4")
	if err := encodePlaytestRecording(dir, out); err != nil {
		t.Fatalf("encodePlaytestRecording: %v", err)
	}

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read recording: %v", err)
	}
	if n, ok := sampleCount(data); !ok || n != 3 {
		t.Fatalf("stsz sample count = %v (ok=%v), want 3", n, ok)
	}
	mdat, ok := findBox(data, "mdat")
	if !ok {
		t.Fatal("missing mdat box")
	}
	samples, err := jpegSamples(mdat)
	if err != nil {
		t.Fatalf("split mdat: %v", err)
	}
	for i, s := range samples {
		got, err := jpegCenterShade(s)
		if err != nil {
			t.Fatalf("decode sample %d: %v", i, err)
		}
		if !shadeClose(got, want[i]) {
			t.Errorf("frame %d shade = %d, want ~%d (wrong lexical order)", i, got, want[i])
		}
	}
}
