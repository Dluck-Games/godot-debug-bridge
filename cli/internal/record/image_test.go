package record

import (
	"bytes"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"
)

func TestResolveOutputSize(t *testing.T) {
	w, h := ResolveOutputSize(1920, 1080, 1280, 0, true)
	if w != 1280 || h != 720 {
		t.Fatalf("got %dx%d, want 1280x720", w, h)
	}
	w, h = ResolveOutputSize(641, 361, 0, 0, true)
	if w != 642 || h != 362 {
		t.Fatalf("even rounding failed: got %dx%d", w, h)
	}
	w, h = ResolveOutputSize(641, 361, 0, 0, false)
	if w != 641 || h != 361 {
		t.Fatalf("no-forceEven failed: got %dx%d", w, h)
	}
}

func TestResizeFramesJPEG_AndContactSheet(t *testing.T) {
	src := [][]byte{
		makeJPEGFrame(t, 120, 80, color.RGBA{R: 220, G: 30, B: 30, A: 255}),
		makeJPEGFrame(t, 120, 80, color.RGBA{R: 30, G: 220, B: 30, A: 255}),
	}

	resized, err := ResizeFramesJPEG(src, 120, 80, 60, 40, ScaleNearest, 85)
	if err != nil {
		t.Fatalf("ResizeFramesJPEG nearest: %v", err)
	}
	for i, b := range resized {
		img, err := jpeg.Decode(bytes.NewReader(b))
		if err != nil {
			t.Fatalf("decode resized %d: %v", i, err)
		}
		if img.Bounds().Dx() != 60 || img.Bounds().Dy() != 40 {
			t.Fatalf("frame %d dims = %dx%d, want 60x40", i, img.Bounds().Dx(), img.Bounds().Dy())
		}
	}

	resizedB, err := ResizeFramesJPEG(src, 120, 80, 60, 40, ScaleBilinear, 85)
	if err != nil || len(resizedB) != 2 {
		t.Fatalf("ResizeFramesJPEG bilinear: err=%v len=%d", err, len(resizedB))
	}

	sheetPath := filepath.Join(t.TempDir(), "sheet.jpg")
	if err := WriteContactSheet(resized, []string{"START 0.00s", "END 1.00s"}, sheetPath); err != nil {
		t.Fatalf("WriteContactSheet: %v", err)
	}
	st, err := os.Stat(sheetPath)
	if err != nil || st.Size() == 0 {
		t.Fatalf("contact sheet missing or empty: err=%v", err)
	}
}
