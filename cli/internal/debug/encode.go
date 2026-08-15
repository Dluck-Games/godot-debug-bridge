// godot-debug-bridge/cli/internal/debug/encode.go
//
// Pure-Go in-process recording encoder (documented decision, spec decision
// record #4 / open question #1).
//
// The record feature historically shelled out to a Swift/AVFoundation encoder
// shipped in the deleted ai-debug package. This file replaces that with a
// genuinely pure-Go implementation: every frame is decoded, flattened onto an
// opaque white background, re-encoded as a baseline JPEG (JFIF APP0), and then
// muxed into a QuickTime-compatible MP4/MOV container.
//
// Container layout (classic MJPEG-in-MOV, as produced by QuickTime itself):
//
//	ftyp  brand isom (mp4) or "qt  " (mov)
//	mdat  concatenated JPEG samples (single chunk)
//	moov  mvhd + trak(tkhd, mdia(mdhd, hdlr, minf(vmhd, dinf, stbl)))
//	      stbl carries a single 'jpeg' VisualSampleEntry plus stts/stsc/stsz/stco.
//
// Every box is written by hand with big-endian 32-bit sizes. No Node.js,
// Swift, ffmpeg, or subprocess encoder is involved, and there are no new
// dependencies — the CLI stays a single Go binary. Output is decodable by
// AVFoundation/QuickTime Player, VLC, and ffmpeg (verified in tests via
// ffprobe when present).
package debug

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	_ "image/png" // register the PNG decoder used by image.Decode for screenshot frames
	"os"
	"path/filepath"
	"strings"
)

// EncodeFramesToVideo encodes the given image frame files into output (.mp4 or
// .mov) at the given frame rate, entirely in-process. All frames must share the
// same pixel dimensions; the encoder derives the container dimensions from the
// first frame.
func EncodeFramesToVideo(framePaths []string, output string, fps int) error {
	if len(framePaths) == 0 {
		return errors.New("cannot encode recording: no frames captured")
	}
	if fps <= 0 {
		return fmt.Errorf("cannot encode recording: invalid fps %d", fps)
	}
	ext := strings.ToLower(filepath.Ext(output))
	if ext != ".mp4" && ext != ".mov" {
		return fmt.Errorf("recording output must end with .mp4 or .mov: %s", output)
	}

	var samples [][]byte
	width, height := 0, 0
	for i, p := range framePaths {
		jpg, w, h, err := frameToJPEG(p)
		if err != nil {
			return fmt.Errorf("cannot encode frame %d (%s): %w", i, p, err)
		}
		if width == 0 {
			width, height = w, h
		} else if w != width || h != height {
			return fmt.Errorf("recording frames have inconsistent dimensions: %dx%d vs %dx%d", width, height, w, h)
		}
		samples = append(samples, jpg)
	}

	if err := writeMJPEGMP4(samples, output, fps, width, height); err != nil {
		return err
	}
	return nil
}

// frameToJPEG decodes an image file (PNG or JPEG) and returns it as a baseline
// JPEG, ready to be muxed as an MJPEG sample.
func frameToJPEG(path string) ([]byte, int, int, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, 0, err
	}
	defer f.Close()

	img, _, err := image.Decode(f)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("cannot decode image: %w", err)
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 {
		return nil, 0, 0, fmt.Errorf("empty image bounds %dx%d", w, h)
	}

	// JPEG has no alpha channel; flatten transparency onto white so transparent
	// pixels do not come out black.
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(dst, dst.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	draw.Draw(dst, dst.Bounds(), img, b.Min, draw.Over)

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: 90}); err != nil {
		return nil, 0, 0, fmt.Errorf("cannot encode JPEG: %w", err)
	}
	return buf.Bytes(), w, h, nil
}

// ---------------------------------------------------------------------------
// MP4/MOV box writer
// ---------------------------------------------------------------------------

// writeMJPEGMP4 writes the final container: ftyp, then mdat (all JPEG samples
// as a single chunk), then moov with the sample tables. moov comes last so the
// stco chunk offset (ftyp size + 8-byte mdat header) is a constant.
func writeMJPEGMP4(samples [][]byte, output string, fps, width, height int) error {
	ftyp := buildFtyp(filepath.Ext(output))
	chunkOffset := uint32(len(ftyp) + 8) // mdat payload starts after ftyp + mdat header
	moov := buildMoov(fps, width, height, samples, chunkOffset)

	out, err := os.Create(output)
	if err != nil {
		return fmt.Errorf("cannot create recording file: %w", err)
	}
	defer out.Close()

	if _, err := out.Write(ftyp); err != nil {
		return fmt.Errorf("cannot write ftyp: %w", err)
	}
	if err := writeMdat(out, samples); err != nil {
		return fmt.Errorf("cannot write mdat: %w", err)
	}
	if _, err := out.Write(moov); err != nil {
		return fmt.Errorf("cannot write moov: %w", err)
	}
	return out.Close()
}

func buildFtyp(ext string) []byte {
	major := "isom"
	compat := []string{"isom", "mp42", "mp41"}
	if strings.EqualFold(ext, ".mov") {
		major = "qt  "
		compat = []string{"qt  ", "mp42"}
	}
	p := &bytes.Buffer{}
	p.WriteString(major)
	p.Write(u32buf(0)) // minor version
	for _, c := range compat {
		p.WriteString(c)
	}
	buf := &bytes.Buffer{}
	appendBox(buf, "ftyp", p.Bytes())
	return buf.Bytes()
}

func writeMdat(w *os.File, samples [][]byte) error {
	var payloadSize uint64
	for _, s := range samples {
		payloadSize += uint64(len(s))
	}
	var header [8]byte
	binary.BigEndian.PutUint32(header[:4], uint32(payloadSize+8))
	copy(header[4:], "mdat")
	if _, err := w.Write(header[:]); err != nil {
		return err
	}
	for _, s := range samples {
		if _, err := w.Write(s); err != nil {
			return err
		}
	}
	return nil
}

func buildMoov(fps, width, height int, samples [][]byte, chunkOffset uint32) []byte {
	n := uint32(len(samples))
	timescale := uint32(fps)

	moov := &bytes.Buffer{}

	// mvhd (movie header)
	mvhd := &bytes.Buffer{}
	mvhd.Write([]byte{0, 0, 0, 0}) // version/flags
	mvhd.Write(u32buf(0))          // creation time
	mvhd.Write(u32buf(0))          // modification time
	mvhd.Write(u32buf(timescale))  // timescale = fps
	mvhd.Write(u32buf(n))          // duration in timescale units
	mvhd.Write(u32buf(0x00010000)) // rate 1.0
	mvhd.Write(u16buf(0x0100))     // volume 1.0
	mvhd.Write(make([]byte, 2))    // reserved
	mvhd.Write(make([]byte, 8))    // reserved
	mvhd.Write(identityMatrix())   // 36 bytes
	mvhd.Write(make([]byte, 24))   // predefined
	mvhd.Write(u32buf(2))          // next track id
	appendBox(moov, "mvhd", mvhd.Bytes())

	// tkhd (track header)
	tkhd := &bytes.Buffer{}
	tkhd.Write([]byte{0, 0, 0, 7}) // version/flags: enabled, in movie, in preview
	tkhd.Write(u32buf(0))          // creation time
	tkhd.Write(u32buf(0))          // modification time
	tkhd.Write(u32buf(1))          // track id
	tkhd.Write(u32buf(0))          // reserved
	tkhd.Write(u32buf(n))          // duration
	tkhd.Write(make([]byte, 8))    // reserved
	tkhd.Write(u16buf(0))          // layer
	tkhd.Write(u16buf(0))          // alternate group
	tkhd.Write(u16buf(0x0100))     // volume
	tkhd.Write(make([]byte, 2))    // reserved
	tkhd.Write(identityMatrix())   // 36 bytes
	tkhd.Write(u32buf(uint32(width) << 16))
	tkhd.Write(u32buf(uint32(height) << 16))

	// mdhd (media header)
	mdhd := &bytes.Buffer{}
	mdhd.Write([]byte{0, 0, 0, 0}) // version/flags
	mdhd.Write(u32buf(0))          // creation time
	mdhd.Write(u32buf(0))          // modification time
	mdhd.Write(u32buf(timescale))  // media timescale = fps
	mdhd.Write(u32buf(n))          // media duration
	mdhd.Write(u16buf(0))          // language
	mdhd.Write(u16buf(0))          // predefined

	// hdlr (handler)
	hdlr := &bytes.Buffer{}
	hdlr.Write([]byte{0, 0, 0, 0}) // version/flags
	hdlr.Write(u32buf(0))          // predefined
	hdlr.WriteString("vide")       // handler type
	hdlr.Write(make([]byte, 12))   // reserved
	hdlr.WriteString("VideoHandler\x00")

	// vmhd (video media header)
	vmhd := &bytes.Buffer{}
	vmhd.Write([]byte{0, 0, 0, 1}) // version/flags
	vmhd.Write(u16buf(0))          // graphics mode
	vmhd.Write(make([]byte, 6))    // opcolor

	// dref -> url (self-contained)
	urlBox := &bytes.Buffer{}
	urlBox.Write(u32buf(12)) // url box size
	urlBox.WriteString("url ")
	urlBox.Write(u32buf(1)) // flags: self-contained
	dref := &bytes.Buffer{}
	dref.Write([]byte{0, 0, 0, 0}) // version/flags
	dref.Write(u32buf(1))          // entry count
	dref.Write(urlBox.Bytes())
	dinf := &bytes.Buffer{}
	appendBox(dinf, "dref", dref.Bytes())

	// stsd (sample description: single jpeg visual sample entry)
	stsd := &bytes.Buffer{}
	stsd.Write([]byte{0, 0, 0, 0}) // version/flags
	stsd.Write(u32buf(1))          // entry count
	stsd.Write(buildVisualSampleEntry(width, height))

	// stts (time to sample: uniform 1 time unit per frame)
	stts := &bytes.Buffer{}
	stts.Write([]byte{0, 0, 0, 0}) // version/flags
	stts.Write(u32buf(1))          // entry count
	stts.Write(u32buf(n))          // sample count
	stts.Write(u32buf(1))          // sample delta

	// stsc (sample to chunk: all frames in one chunk)
	stsc := &bytes.Buffer{}
	stsc.Write([]byte{0, 0, 0, 0}) // version/flags
	stsc.Write(u32buf(1))          // entry count
	stsc.Write(u32buf(1))          // first chunk
	stsc.Write(u32buf(n))          // samples per chunk
	stsc.Write(u32buf(1))          // sample description index

	// stsz (sample sizes)
	stsz := &bytes.Buffer{}
	stsz.Write([]byte{0, 0, 0, 0}) // version/flags
	stsz.Write(u32buf(0))          // uniform sample size (0 = listed)
	stsz.Write(u32buf(n))          // sample count
	for _, s := range samples {
		stsz.Write(u32buf(uint32(len(s))))
	}

	// stco (chunk offset)
	stco := &bytes.Buffer{}
	stco.Write([]byte{0, 0, 0, 0}) // version/flags
	stco.Write(u32buf(1))          // entry count
	stco.Write(u32buf(chunkOffset))

	// Assemble the box tree.
	stbl := &bytes.Buffer{}
	appendBox(stbl, "stsd", stsd.Bytes())
	appendBox(stbl, "stts", stts.Bytes())
	appendBox(stbl, "stsc", stsc.Bytes())
	appendBox(stbl, "stsz", stsz.Bytes())
	appendBox(stbl, "stco", stco.Bytes())

	minf := &bytes.Buffer{}
	appendBox(minf, "vmhd", vmhd.Bytes())
	appendBox(minf, "dinf", dinf.Bytes())
	appendBox(minf, "stbl", stbl.Bytes())

	mdia := &bytes.Buffer{}
	appendBox(mdia, "mdhd", mdhd.Bytes())
	appendBox(mdia, "hdlr", hdlr.Bytes())
	appendBox(mdia, "minf", minf.Bytes())

	trak := &bytes.Buffer{}
	appendBox(trak, "tkhd", tkhd.Bytes())
	appendBox(trak, "mdia", mdia.Bytes())

	moovPayload := &bytes.Buffer{}
	appendBox(moovPayload, "mvhd", mvhd.Bytes())
	appendBox(moovPayload, "trak", trak.Bytes())

	buf := &bytes.Buffer{}
	appendBox(buf, "moov", moovPayload.Bytes())
	return buf.Bytes()
}

// buildVisualSampleEntry returns the 'jpeg' VisualSampleEntry box (78-byte
// payload plus the 8-byte box header) used by MJPEG tracks. No avcC/colr
// children are required for JPEG.
func buildVisualSampleEntry(width, height int) []byte {
	fields := &bytes.Buffer{}
	fields.Write(make([]byte, 6))  // reserved
	fields.Write(u16buf(1))        // data reference index
	fields.Write(u16buf(0))        // predefined
	fields.Write(make([]byte, 2))  // reserved
	fields.Write(make([]byte, 12)) // predefined[3]
	fields.Write(u16buf(uint16(width)))
	fields.Write(u16buf(uint16(height)))
	fields.Write(u32buf(0x00480000)) // horizontal resolution (72 dpi)
	fields.Write(u32buf(0x00480000)) // vertical resolution (72 dpi)
	fields.Write(u32buf(0))          // reserved
	fields.Write(u16buf(1))          // frame count
	var cname [32]byte               // compressor name (Pascal string)
	name := []byte("Photo - JPEG")
	cname[0] = byte(len(name))
	copy(cname[1:], name)
	fields.Write(cname[:])
	fields.Write(u16buf(24))     // depth (24-bit)
	fields.Write(u16buf(0xFFFF)) // predefined

	buf := &bytes.Buffer{}
	appendBox(buf, "jpeg", fields.Bytes())
	return buf.Bytes()
}

// identityMatrix is the 36-byte ISO identity transformation matrix.
func identityMatrix() []byte {
	return []byte{
		0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x40, 0x00, 0x00, 0x00,
	}
}

// appendBox writes a size-prefixed box (size + type + payload) to buf.
func appendBox(buf *bytes.Buffer, boxType string, payload []byte) {
	buf.Write(u32buf(uint32(len(payload) + 8)))
	buf.WriteString(boxType)
	buf.Write(payload)
}

func u32buf(v uint32) []byte {
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, v)
	return b
}

func u16buf(v uint16) []byte {
	b := make([]byte, 2)
	binary.BigEndian.PutUint16(b, v)
	return b
}
