package debug

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// writeTestPNG writes a solid-color PNG frame file.
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

type rawBox struct {
	typ  string
	data []byte
}

func rawBoxes(data []byte) ([]rawBox, error) {
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
		} else if size == 0 {
			total = uint64(len(data) - off)
		}
		if total < uint64(hdr) || off+int(total) > len(data) {
			return nil, fmt.Errorf("box %q out of range", typ)
		}
		out = append(out, rawBox{typ: typ, data: data[off+hdr : off+int(total)]})
		off += int(total)
	}
	return out, nil
}

func findBoxPayload(data []byte, typ string) ([]byte, bool) {
	boxes, err := rawBoxes(data)
	if err != nil {
		return nil, false
	}
	for _, b := range boxes {
		if b.typ == typ {
			return b.data, true
		}
		if payload, ok := findBoxPayload(b.data, typ); ok {
			return payload, true
		}
	}
	return nil, false
}

func be16(b []byte) uint16 { return binary.BigEndian.Uint16(b) }
func be32(b []byte) uint32 { return binary.BigEndian.Uint32(b) }

// verifyRecording checks the container structure and sample tables of a muxed
// MJPEG file.
func verifyRecording(t *testing.T, out string, width, height, frames int, majorBrand string) {
	t.Helper()
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 {
		t.Fatal("recording file is empty")
	}

	ftyp, ok := findBoxPayload(data, "ftyp")
	if !ok {
		t.Fatal("missing ftyp box")
	}
	if len(ftyp) < 4 || string(ftyp[:4]) != majorBrand {
		t.Errorf("ftyp major brand = %q, want %q", string(ftyp[:4]), majorBrand)
	}

	mdat, ok := findBoxPayload(data, "mdat")
	if !ok {
		t.Fatal("missing mdat box")
	}
	if len(mdat) == 0 {
		t.Fatal("mdat is empty")
	}
	if !bytes.HasPrefix(mdat, []byte{0xFF, 0xD8}) {
		t.Error("mdat does not start with JPEG SOI marker")
	}

	stsd, ok := findBoxPayload(data, "stsd")
	if !ok {
		t.Fatal("missing stsd box")
	}
	// stsd: verflags(4) entry_count(4) then the sample entry box.
	if len(stsd) < 16 {
		t.Fatalf("stsd too small: %d bytes", len(stsd))
	}
	if string(stsd[12:16]) != "jpeg" {
		t.Errorf("sample entry type = %q, want jpeg", string(stsd[12:16]))
	}
	// Sample entry payload: reserved(6) dref(2) predef(2) reserved(2) predef3(12) width(2) height(2)
	if gotW, gotH := be16(stsd[40:42]), be16(stsd[42:44]); int(gotW) != width || int(gotH) != height {
		t.Errorf("sample entry dimensions = %dx%d, want %dx%d", gotW, gotH, width, height)
	}

	stsz, ok := findBoxPayload(data, "stsz")
	if !ok {
		t.Fatal("missing stsz box")
	}
	if len(stsz) < 12 {
		t.Fatalf("stsz too small: %d bytes", len(stsz))
	}
	if got := int(be32(stsz[8:12])); got != frames {
		t.Errorf("stsz sample count = %d, want %d", got, frames)
	}

	stts, ok := findBoxPayload(data, "stts")
	if !ok {
		t.Fatal("missing stts box")
	}
	if len(stts) < 12 || int(be32(stts[8:12])) != frames {
		t.Errorf("stts sample count mismatch: %v", stts)
	}

	stco, ok := findBoxPayload(data, "stco")
	if !ok {
		t.Fatal("missing stco box")
	}
	if len(stco) < 12 {
		t.Fatalf("stco too small: %d bytes", len(stco))
	}
	chunkOffset := int(be32(stco[8:12]))
	if chunkOffset+2 > len(data) {
		t.Fatalf("chunk offset %d out of range", chunkOffset)
	}
	if !bytes.Equal(data[chunkOffset:chunkOffset+2], []byte{0xFF, 0xD8}) {
		t.Error("chunk offset does not point at a JPEG SOI marker")
	}
}

// checkPlayable runs ffprobe (and ffmpeg decode) when available; skipped
// otherwise. This is a verification aid only — the encoder itself never shells
// out.
func checkPlayable(t *testing.T, out string) {
	t.Helper()
	ffprobe, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Skip("ffprobe not available; skipping player check")
	}
	probe, err := exec.Command(ffprobe, "-v", "error",
		"-show_entries", "stream=codec_name,width,height",
		"-of", "csv=p=0", out).Output()
	if err != nil {
		t.Fatalf("ffprobe rejected recording: %v\n%s", err, probe)
	}
	if s := strings.TrimSpace(string(probe)); !strings.Contains(s, "mjpeg") {
		t.Errorf("ffprobe stream = %q, want mjpeg", s)
	}

	if ffmpeg, err := exec.LookPath("ffmpeg"); err == nil {
		decode, err := exec.Command(ffmpeg, "-v", "error", "-i", out, "-f", "null", "-").CombinedOutput()
		if err != nil {
			t.Fatalf("ffmpeg could not decode recording: %v\n%s", err, decode)
		}
	}
}

func TestEncodeFramesToVideo_MP4(t *testing.T) {
	dir := t.TempDir()
	frames := []string{
		writeTestPNG(t, dir, "f0.png", 64, 48, 200),
		writeTestPNG(t, dir, "f1.png", 64, 48, 120),
		writeTestPNG(t, dir, "f2.png", 64, 48, 40),
	}
	out := filepath.Join(t.TempDir(), "rec.mp4")
	if err := EncodeFramesToVideo(frames, out, 4); err != nil {
		t.Fatalf("EncodeFramesToVideo: %v", err)
	}
	verifyRecording(t, out, 64, 48, 3, "isom")
	checkPlayable(t, out)
}

func TestEncodeFramesToVideo_MOV(t *testing.T) {
	dir := t.TempDir()
	frames := []string{
		writeTestPNG(t, dir, "f0.png", 32, 32, 100),
		writeTestPNG(t, dir, "f1.png", 32, 32, 200),
	}
	out := filepath.Join(t.TempDir(), "rec.mov")
	if err := EncodeFramesToVideo(frames, out, 1); err != nil {
		t.Fatalf("EncodeFramesToVideo: %v", err)
	}
	verifyRecording(t, out, 32, 32, 2, "qt  ")
	checkPlayable(t, out)
}

func TestEncodeFramesToVideo_SingleFrame(t *testing.T) {
	dir := t.TempDir()
	frames := []string{writeTestPNG(t, dir, "only.png", 16, 16, 150)}
	out := filepath.Join(t.TempDir(), "one.mp4")
	if err := EncodeFramesToVideo(frames, out, 2); err != nil {
		t.Fatalf("EncodeFramesToVideo: %v", err)
	}
	verifyRecording(t, out, 16, 16, 1, "isom")
	checkPlayable(t, out)
}

func TestEncodeFramesToVideo_NoFrames(t *testing.T) {
	out := filepath.Join(t.TempDir(), "x.mp4")
	if err := EncodeFramesToVideo(nil, out, 4); err == nil {
		t.Fatal("expected error for empty frame list")
	}
}

func TestEncodeFramesToVideo_BadExtension(t *testing.T) {
	dir := t.TempDir()
	frames := []string{writeTestPNG(t, dir, "f0.png", 8, 8, 50)}
	out := filepath.Join(t.TempDir(), "rec.avi")
	if err := EncodeFramesToVideo(frames, out, 4); err == nil || !strings.Contains(err.Error(), ".mp4 or .mov") {
		t.Errorf("expected extension error, got %v", err)
	}
}

func TestEncodeFramesToVideo_InconsistentDimensions(t *testing.T) {
	dir := t.TempDir()
	frames := []string{
		writeTestPNG(t, dir, "wide.png", 64, 48, 100),
		writeTestPNG(t, dir, "tall.png", 48, 64, 100),
	}
	out := filepath.Join(t.TempDir(), "bad.mp4")
	if err := EncodeFramesToVideo(frames, out, 4); err == nil || !strings.Contains(err.Error(), "inconsistent dimensions") {
		t.Errorf("expected dimension error, got %v", err)
	}
}
