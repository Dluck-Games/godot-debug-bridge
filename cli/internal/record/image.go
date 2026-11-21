package record

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"sync"
)

// ScaleFilter controls JPEG frame resizing interpolation.
type ScaleFilter string

const (
	ScaleBilinear ScaleFilter = "bilinear"
	ScaleNearest  ScaleFilter = "nearest"
)

// ParseScaleFilter validates a scale filter flag string.
func ParseScaleFilter(s string) (ScaleFilter, error) {
	switch s {
	case "", "bilinear":
		return ScaleBilinear, nil
	case "nearest":
		return ScaleNearest, nil
	default:
		return "", fmt.Errorf("invalid scale filter %q (use bilinear or nearest)", s)
	}
}

// ResolveOutputSize computes target output dimensions given source dimensions
// and optional requested width/height. When forceEven is true, dimensions are
// rounded to even integers >= 2 (required by H.264 yuv420p).
func ResolveOutputSize(srcW, srcH, reqW, reqH int, forceEven bool) (int, int) {
	w, h := srcW, srcH
	switch {
	case reqW > 0 && reqH > 0:
		w, h = reqW, reqH
	case reqW > 0:
		w = reqW
		h = int(math.Round(float64(srcH) * float64(reqW) / float64(srcW)))
	case reqH > 0:
		h = reqH
		w = int(math.Round(float64(srcW) * float64(reqH) / float64(srcH)))
	}
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	if forceEven {
		if w%2 != 0 {
			w++
		}
		if h%2 != 0 {
			h++
		}
	}
	return w, h
}

// ResizeFramesJPEG resizes JPEG frames in parallel to targetW x targetH. If
// source dimensions already match, raw JPEG bytes are returned untouched.
func ResizeFramesJPEG(frames [][]byte, srcW, srcH, targetW, targetH int, filter ScaleFilter, quality int) ([][]byte, error) {
	if srcW == targetW && srcH == targetH {
		return frames, nil
	}
	if quality <= 0 || quality > 100 {
		quality = 85
	}
	out := make([][]byte, len(frames))
	errs := make([]error, len(frames))

	workers := runtime.NumCPU()
	if workers < 1 {
		workers = 1
	}
	if workers > len(frames) {
		workers = len(frames)
	}

	var next int
	var mu sync.Mutex
	var wg sync.WaitGroup
	wg.Add(workers)
	for w := 0; w < workers; w++ {
		go func() {
			defer wg.Done()
			for {
				mu.Lock()
				idx := next
				next++
				mu.Unlock()
				if idx >= len(frames) {
					return
				}
				resized, err := ResizeJPEG(frames[idx], targetW, targetH, filter, quality)
				out[idx] = resized
				errs[idx] = err
			}
		}()
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			return nil, fmt.Errorf("resize frame %d: %w", i, err)
		}
	}
	return out, nil
}

// ResizeJPEG decodes a JPEG payload, resizes it to targetW x targetH, and
// re-encodes as baseline JPEG.
func ResizeJPEG(data []byte, targetW, targetH int, filter ScaleFilter, quality int) ([]byte, error) {
	img, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decode jpeg: %w", err)
	}
	b := img.Bounds()
	if b.Dx() == targetW && b.Dy() == targetH {
		return data, nil
	}
	src := toNRGBA(img)
	var dst *image.RGBA
	if filter == ScaleNearest {
		dst = scaleNearest(src, targetW, targetH)
	} else {
		dst = scaleBilinear(src, targetW, targetH)
	}
	var out bytes.Buffer
	if err := jpeg.Encode(&out, dst, &jpeg.Options{Quality: quality}); err != nil {
		return nil, fmt.Errorf("encode jpeg: %w", err)
	}
	return out.Bytes(), nil
}

func toNRGBA(img image.Image) *image.RGBA {
	if r, ok := img.(*image.RGBA); ok && r.Rect.Min == (image.Point{}) {
		return r
	}
	b := img.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(dst, dst.Bounds(), img, b.Min, draw.Src)
	return dst
}

func scaleNearest(src *image.RGBA, targetW, targetH int) *image.RGBA {
	srcW := src.Bounds().Dx()
	srcH := src.Bounds().Dy()
	dst := image.NewRGBA(image.Rect(0, 0, targetW, targetH))
	for y := 0; y < targetH; y++ {
		sy := (y * srcH) / targetH
		if sy >= srcH {
			sy = srcH - 1
		}
		srcRow := sy * src.Stride
		dstRow := y * dst.Stride
		for x := 0; x < targetW; x++ {
			sx := (x * srcW) / targetW
			if sx >= srcW {
				sx = srcW - 1
			}
			si := srcRow + sx*4
			di := dstRow + x*4
			dst.Pix[di+0] = src.Pix[si+0]
			dst.Pix[di+1] = src.Pix[si+1]
			dst.Pix[di+2] = src.Pix[si+2]
			dst.Pix[di+3] = 255
		}
	}
	return dst
}

func scaleBilinear(src *image.RGBA, targetW, targetH int) *image.RGBA {
	srcW := src.Bounds().Dx()
	srcH := src.Bounds().Dy()
	dst := image.NewRGBA(image.Rect(0, 0, targetW, targetH))
	if srcW == 1 || srcH == 1 {
		return scaleNearest(src, targetW, targetH)
	}
	scaleX := float64(srcW) / float64(targetW)
	scaleY := float64(srcH) / float64(targetH)
	for y := 0; y < targetH; y++ {
		fy := (float64(y)+0.5)*scaleY - 0.5
		if fy < 0 {
			fy = 0
		}
		y0 := int(fy)
		y1 := y0 + 1
		if y1 >= srcH {
			y0 = srcH - 1
			y1 = srcH - 1
		}
		wy := fy - float64(y0)
		wy0 := 1.0 - wy
		row0 := y0 * src.Stride
		row1 := y1 * src.Stride
		dstRow := y * dst.Stride

		for x := 0; x < targetW; x++ {
			fx := (float64(x)+0.5)*scaleX - 0.5
			if fx < 0 {
				fx = 0
			}
			x0 := int(fx)
			x1 := x0 + 1
			if x1 >= srcW {
				x0 = srcW - 1
				x1 = srcW - 1
			}
			wx := fx - float64(x0)
			wx0 := 1.0 - wx

			w00 := wx0 * wy0
			w10 := wx * wy0
			w01 := wx0 * wy
			w11 := wx * wy

			i00 := row0 + x0*4
			i10 := row0 + x1*4
			i01 := row1 + x0*4
			i11 := row1 + x1*4

			di := dstRow + x*4
			dst.Pix[di+0] = uint8(math.Round(float64(src.Pix[i00+0])*w00 + float64(src.Pix[i10+0])*w10 + float64(src.Pix[i01+0])*w01 + float64(src.Pix[i11+0])*w11))
			dst.Pix[di+1] = uint8(math.Round(float64(src.Pix[i00+1])*w00 + float64(src.Pix[i10+1])*w10 + float64(src.Pix[i01+1])*w01 + float64(src.Pix[i11+1])*w11))
			dst.Pix[di+2] = uint8(math.Round(float64(src.Pix[i00+2])*w00 + float64(src.Pix[i10+2])*w10 + float64(src.Pix[i01+2])*w01 + float64(src.Pix[i11+2])*w11))
			dst.Pix[di+3] = 255
		}
	}
	return dst
}

// WriteContactSheet writes a multi-panel JPEG overview of selected frames with
// short ASCII captions beneath each tile.
func WriteContactSheet(frames [][]byte, labels []string, outputPath string) error {
	if len(frames) == 0 {
		return errors.New("cannot create contact sheet: no frames")
	}
	const (
		thumbW  = 480
		labelH  = 28
		pad     = 12
		maxCols = 3
	)

	firstImg, err := jpeg.Decode(bytes.NewReader(frames[0]))
	if err != nil {
		return fmt.Errorf("decode contact sheet sample: %w", err)
	}
	b := firstImg.Bounds()
	thumbH := int(math.Round(float64(b.Dy()) * float64(thumbW) / float64(b.Dx())))
	if thumbH < 60 {
		thumbH = 60
	}

	cols := maxCols
	if len(frames) == 1 {
		cols = 1
	} else if len(frames) == 2 || len(frames) == 4 {
		cols = 2
	}
	rows := (len(frames) + cols - 1) / cols
	sheetW := pad + cols*(thumbW+pad)
	sheetH := pad + rows*(thumbH+labelH+pad)

	sheet := image.NewRGBA(image.Rect(0, 0, sheetW, sheetH))
	draw.Draw(sheet, sheet.Bounds(), image.NewUniform(color.RGBA{R: 20, G: 24, B: 31, A: 255}), image.Point{}, draw.Src)

	for i, raw := range frames {
		img, err := jpeg.Decode(bytes.NewReader(raw))
		if err != nil {
			return fmt.Errorf("decode contact sheet frame %d: %w", i, err)
		}
		scaled := scaleBilinear(toNRGBA(img), thumbW, thumbH)
		col := i % cols
		row := i / cols
		x0 := pad + col*(thumbW+pad)
		y0 := pad + row*(thumbH+labelH+pad)
		rect := image.Rect(x0, y0, x0+thumbW, y0+thumbH)
		draw.Draw(sheet, rect, scaled, image.Point{}, draw.Src)

		if i < len(labels) && labels[i] != "" {
			drawBitmapText(sheet, x0+4, y0+thumbH+7, labels[i], color.RGBA{R: 235, G: 240, B: 248, A: 255}, 2)
		}
	}

	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return fmt.Errorf("create contact sheet directory: %w", err)
	}
	f, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("create contact sheet: %w", err)
	}
	defer f.Close()
	return jpeg.Encode(f, sheet, &jpeg.Options{Quality: 90})
}

// drawBitmapText renders compact 5x7 ASCII glyphs for contact-sheet labels with
// zero font-file dependencies.
func drawBitmapText(dst *image.RGBA, x, y int, text string, col color.RGBA, scale int) {
	cx := x
	maxX := dst.Bounds().Max.X - 6*scale
	for _, r := range text {
		if cx > maxX {
			break
		}
		glyph, ok := bitmapFont5x7[r]
		if !ok {
			if r >= 'a' && r <= 'z' {
				glyph = bitmapFont5x7[r-'a'+'A']
				ok = true
			}
		}
		if ok {
			for row := 0; row < 7; row++ {
				bits := glyph[row]
				for c := 0; c < 5; c++ {
					if (bits & (1 << uint(4-c))) != 0 {
						for sy := 0; sy < scale; sy++ {
							for sx := 0; sx < scale; sx++ {
								px := cx + c*scale + sx
								py := y + row*scale + sy
								if image.Pt(px, py).In(dst.Bounds()) {
									dst.SetRGBA(px, py, col)
								}
							}
						}
					}
				}
			}
		}
		cx += 6 * scale
	}
}

var bitmapFont5x7 = map[rune][7]uint8{
	' ': {0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
	'-': {0x00, 0x00, 0x00, 0x1F, 0x00, 0x00, 0x00},
	'_': {0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x1F},
	'.': {0x00, 0x00, 0x00, 0x00, 0x00, 0x0C, 0x0C},
	':': {0x00, 0x0C, 0x0C, 0x00, 0x0C, 0x0C, 0x00},
	'/': {0x01, 0x02, 0x02, 0x04, 0x08, 0x08, 0x10},
	'#': {0x0A, 0x0A, 0x1F, 0x0A, 0x1F, 0x0A, 0x0A},
	'0': {0x0E, 0x11, 0x13, 0x15, 0x19, 0x11, 0x0E},
	'1': {0x04, 0x0C, 0x04, 0x04, 0x04, 0x04, 0x0E},
	'2': {0x0E, 0x11, 0x01, 0x02, 0x04, 0x08, 0x1F},
	'3': {0x1F, 0x02, 0x04, 0x02, 0x01, 0x11, 0x0E},
	'4': {0x02, 0x06, 0x0A, 0x12, 0x1F, 0x02, 0x02},
	'5': {0x1F, 0x10, 0x1E, 0x01, 0x01, 0x11, 0x0E},
	'6': {0x06, 0x08, 0x10, 0x1E, 0x11, 0x11, 0x0E},
	'7': {0x1F, 0x01, 0x02, 0x04, 0x08, 0x08, 0x08},
	'8': {0x0E, 0x11, 0x11, 0x0E, 0x11, 0x11, 0x0E},
	'9': {0x0E, 0x11, 0x11, 0x0F, 0x01, 0x02, 0x0C},
	'A': {0x0E, 0x11, 0x11, 0x1F, 0x11, 0x11, 0x11},
	'B': {0x1E, 0x11, 0x11, 0x1E, 0x11, 0x11, 0x1E},
	'C': {0x0E, 0x11, 0x10, 0x10, 0x10, 0x11, 0x0E},
	'D': {0x1C, 0x12, 0x11, 0x11, 0x11, 0x12, 0x1C},
	'E': {0x1F, 0x10, 0x10, 0x1E, 0x10, 0x10, 0x1F},
	'F': {0x1F, 0x10, 0x10, 0x1E, 0x10, 0x10, 0x10},
	'G': {0x0E, 0x11, 0x10, 0x17, 0x11, 0x11, 0x0F},
	'H': {0x11, 0x11, 0x11, 0x1F, 0x11, 0x11, 0x11},
	'I': {0x0E, 0x04, 0x04, 0x04, 0x04, 0x04, 0x0E},
	'J': {0x07, 0x02, 0x02, 0x02, 0x02, 0x12, 0x0C},
	'K': {0x11, 0x12, 0x14, 0x18, 0x14, 0x12, 0x11},
	'L': {0x10, 0x10, 0x10, 0x10, 0x10, 0x10, 0x1F},
	'M': {0x11, 0x1B, 0x15, 0x15, 0x11, 0x11, 0x11},
	'N': {0x11, 0x11, 0x19, 0x15, 0x13, 0x11, 0x11},
	'O': {0x0E, 0x11, 0x11, 0x11, 0x11, 0x11, 0x0E},
	'P': {0x1E, 0x11, 0x11, 0x1E, 0x10, 0x10, 0x10},
	'Q': {0x0E, 0x11, 0x11, 0x11, 0x15, 0x12, 0x0D},
	'R': {0x1E, 0x11, 0x11, 0x1E, 0x14, 0x12, 0x11},
	'S': {0x0F, 0x10, 0x10, 0x0E, 0x01, 0x01, 0x1E},
	'T': {0x1F, 0x04, 0x04, 0x04, 0x04, 0x04, 0x04},
	'U': {0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x0E},
	'V': {0x11, 0x11, 0x11, 0x11, 0x11, 0x0A, 0x04},
	'W': {0x11, 0x11, 0x11, 0x15, 0x15, 0x1B, 0x11},
	'X': {0x11, 0x11, 0x0A, 0x04, 0x0A, 0x11, 0x11},
	'Y': {0x11, 0x11, 0x0A, 0x04, 0x04, 0x04, 0x04},
	'Z': {0x1F, 0x01, 0x02, 0x04, 0x08, 0x10, 0x1F},
}
