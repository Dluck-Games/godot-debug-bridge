package record

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	_ "image/png" // PNG decoding for EncodeFramesToVideo
	"os"
	"path/filepath"
	"strings"
)

const (
	movieTimescale uint32 = 60000
	largeFileLimit uint64 = 0xE0000000
)

// MuxAudio holds 16-bit signed little-endian PCM audio to mux as an uncompressed
// QuickTime 'sowt' sound track.
type MuxAudio struct {
	Channels   int
	SampleRate int
	PCM16      []byte
}

// MuxConfig configures an MJPEG(+PCM) MP4/MOV mux operation.
type MuxConfig struct {
	FPS    int
	Width  int
	Height int
	Audio  *MuxAudio
}

// EncodeFramesToVideo encodes PNG or JPEG frame files into output (.mp4 or
// .mov) at fps entirely in-process with faststart moov placement.
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

	samples := make([][]byte, 0, len(framePaths))
	width, height := 0, 0
	for i, p := range framePaths {
		jpg, w, h, err := imageFileToJPEG(p, 90)
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

	return MuxSamples(MuxConfig{
		FPS:    fps,
		Width:  width,
		Height: height,
	}, samples, output)
}

func imageFileToJPEG(path string, quality int) ([]byte, int, int, error) {
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

	// Flatten alpha onto white so transparent UI frames never turn black.
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(dst, dst.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	draw.Draw(dst, dst.Bounds(), img, b.Min, draw.Over)

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: quality}); err != nil {
		return nil, 0, 0, fmt.Errorf("cannot encode JPEG: %w", err)
	}
	return buf.Bytes(), w, h, nil
}

// MuxSamples writes JPEG video samples (and optional 16-bit LE PCM audio) into
// a faststart MP4/MOV container (`ftyp` -> `moov` -> `mdat`).
func MuxSamples(cfg MuxConfig, videoSamples [][]byte, output string) error {
	if len(videoSamples) == 0 {
		return errors.New("cannot mux recording: no video samples")
	}
	if cfg.FPS <= 0 {
		return fmt.Errorf("cannot mux recording: invalid fps %d", cfg.FPS)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 {
		return fmt.Errorf("cannot mux recording: invalid dimensions %dx%d", cfg.Width, cfg.Height)
	}
	ext := strings.ToLower(filepath.Ext(output))
	if ext != ".mp4" && ext != ".mov" {
		return fmt.Errorf("recording output must end with .mp4 or .mov: %s", output)
	}

	var audio *MuxAudio
	if cfg.Audio != nil && len(cfg.Audio.PCM16) > 0 && cfg.Audio.Channels > 0 && cfg.Audio.SampleRate > 0 {
		bytesPerFrame := cfg.Audio.Channels * 2
		aligned := (len(cfg.Audio.PCM16) / bytesPerFrame) * bytesPerFrame
		if aligned > 0 {
			audio = &MuxAudio{
				Channels:   cfg.Audio.Channels,
				SampleRate: cfg.Audio.SampleRate,
				PCM16:      cfg.Audio.PCM16[:aligned],
			}
		}
	}

	var videoBytes uint64
	for _, s := range videoSamples {
		videoBytes += uint64(len(s))
	}
	var audioBytes uint64
	if audio != nil {
		audioBytes = uint64(len(audio.PCM16))
	}
	payloadBytes := videoBytes + audioBytes
	use64 := payloadBytes >= largeFileLimit
	mdatHeaderSize := uint64(8)
	if use64 {
		mdatHeaderSize = 16
	}

	ftyp := buildFtyp(ext)
	// First pass calculates exact moov byte size with placeholder chunk offsets.
	placeholderMoov := buildMoov(cfg.FPS, cfg.Width, cfg.Height, videoSamples, audio, 0, 0, use64)
	videoChunkOffset := uint64(len(ftyp)) + uint64(len(placeholderMoov)) + mdatHeaderSize
	audioChunkOffset := videoChunkOffset + videoBytes
	if !use64 && audioChunkOffset > uint64(^uint32(0)) {
		use64 = true
		mdatHeaderSize = 16
		placeholderMoov = buildMoov(cfg.FPS, cfg.Width, cfg.Height, videoSamples, audio, 0, 0, true)
		videoChunkOffset = uint64(len(ftyp)) + uint64(len(placeholderMoov)) + mdatHeaderSize
		audioChunkOffset = videoChunkOffset + videoBytes
	}

	moov := buildMoov(cfg.FPS, cfg.Width, cfg.Height, videoSamples, audio, videoChunkOffset, audioChunkOffset, use64)
	if len(moov) != len(placeholderMoov) {
		return fmt.Errorf("internal mux error: moov length drifted (%d vs %d)", len(moov), len(placeholderMoov))
	}

	if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
		return fmt.Errorf("cannot create output directory: %w", err)
	}
	out, err := os.Create(output)
	if err != nil {
		return fmt.Errorf("cannot create recording file: %w", err)
	}
	bw := bufio.NewWriterSize(out, 1<<20)

	writeErr := func() error {
		if _, err := bw.Write(ftyp); err != nil {
			return fmt.Errorf("write ftyp: %w", err)
		}
		if _, err := bw.Write(moov); err != nil {
			return fmt.Errorf("write moov: %w", err)
		}
		if err := writeMdatHeader(bw, payloadBytes, use64); err != nil {
			return fmt.Errorf("write mdat header: %w", err)
		}
		for _, s := range videoSamples {
			if _, err := bw.Write(s); err != nil {
				return fmt.Errorf("write video sample: %w", err)
			}
		}
		if audio != nil {
			if _, err := bw.Write(audio.PCM16); err != nil {
				return fmt.Errorf("write audio samples: %w", err)
			}
		}
		if err := bw.Flush(); err != nil {
			return fmt.Errorf("flush output: %w", err)
		}
		return out.Close()
	}()
	if writeErr != nil {
		_ = out.Close()
		_ = os.Remove(output)
		return writeErr
	}
	return nil
}

func buildFtyp(ext string) []byte {
	major := "isom"
	compat := []string{"isom", "iso2", "mp41"}
	minor := uint32(0x00000200)
	if strings.EqualFold(ext, ".mov") {
		major = "qt  "
		compat = []string{"qt  "}
	}
	p := &bytes.Buffer{}
	p.WriteString(major)
	p.Write(u32buf(minor))
	for _, c := range compat {
		p.WriteString(c)
	}
	buf := &bytes.Buffer{}
	appendBox(buf, "ftyp", p.Bytes())
	return buf.Bytes()
}

func writeMdatHeader(w *bufio.Writer, payloadBytes uint64, use64 bool) error {
	if !use64 {
		var hdr [8]byte
		binary.BigEndian.PutUint32(hdr[0:4], uint32(payloadBytes+8))
		copy(hdr[4:8], "mdat")
		_, err := w.Write(hdr[:])
		return err
	}
	var hdr [16]byte
	binary.BigEndian.PutUint32(hdr[0:4], 1)
	copy(hdr[4:8], "mdat")
	binary.BigEndian.PutUint64(hdr[8:16], payloadBytes+16)
	_, err := w.Write(hdr[:])
	return err
}

func buildMoov(fps, width, height int, videoSamples [][]byte, audio *MuxAudio, videoOffset, audioOffset uint64, use64 bool) []byte {
	nFrames := uint32(len(videoSamples))
	videoMovieDuration := uint32((uint64(nFrames) * uint64(movieTimescale)) / uint64(fps))

	var audioFrameCount uint32
	var audioMovieDuration uint32
	if audio != nil {
		bytesPerFrame := audio.Channels * 2
		audioFrameCount = uint32(len(audio.PCM16) / bytesPerFrame)
		audioMovieDuration = uint32((uint64(audioFrameCount) * uint64(movieTimescale)) / uint64(audio.SampleRate))
	}

	movieDuration := videoMovieDuration
	if audioMovieDuration > movieDuration {
		movieDuration = audioMovieDuration
	}
	nextTrackID := uint32(2)
	if audio != nil {
		nextTrackID = 3
	}

	mvhd := &bytes.Buffer{}
	mvhd.Write([]byte{0, 0, 0, 0})
	mvhd.Write(u32buf(0))
	mvhd.Write(u32buf(0))
	mvhd.Write(u32buf(movieTimescale))
	mvhd.Write(u32buf(movieDuration))
	mvhd.Write(u32buf(0x00010000))
	mvhd.Write(u16buf(0x0100))
	mvhd.Write(make([]byte, 2))
	mvhd.Write(make([]byte, 8))
	mvhd.Write(identityMatrix())
	mvhd.Write(make([]byte, 24))
	mvhd.Write(u32buf(nextTrackID))

	moovPayload := &bytes.Buffer{}
	appendBox(moovPayload, "mvhd", mvhd.Bytes())
	moovPayload.Write(buildVideoTrack(1, fps, width, height, videoMovieDuration, videoSamples, videoOffset, use64))
	if audio != nil {
		moovPayload.Write(buildAudioTrack(2, audioMovieDuration, audio, audioFrameCount, audioOffset, use64))
	}

	buf := &bytes.Buffer{}
	appendBox(buf, "moov", moovPayload.Bytes())
	return buf.Bytes()
}

func buildVideoTrack(trackID uint32, fps, width, height int, movieDuration uint32, samples [][]byte, chunkOffset uint64, use64 bool) []byte {
	n := uint32(len(samples))
	timescale := uint32(fps)

	tkhd := &bytes.Buffer{}
	tkhd.Write([]byte{0, 0, 0, 7})
	tkhd.Write(u32buf(0))
	tkhd.Write(u32buf(0))
	tkhd.Write(u32buf(trackID))
	tkhd.Write(u32buf(0))
	tkhd.Write(u32buf(movieDuration))
	tkhd.Write(make([]byte, 8))
	tkhd.Write(u16buf(0))
	tkhd.Write(u16buf(0))
	tkhd.Write(u16buf(0)) // video volume = 0
	tkhd.Write(make([]byte, 2))
	tkhd.Write(identityMatrix())
	tkhd.Write(u32buf(uint32(width) << 16))
	tkhd.Write(u32buf(uint32(height) << 16))

	mdhd := &bytes.Buffer{}
	mdhd.Write([]byte{0, 0, 0, 0})
	mdhd.Write(u32buf(0))
	mdhd.Write(u32buf(0))
	mdhd.Write(u32buf(timescale))
	mdhd.Write(u32buf(n))
	mdhd.Write(u16buf(0))
	mdhd.Write(u16buf(0))

	hdlr := buildHdlr("vide", "VideoHandler\x00")

	vmhd := &bytes.Buffer{}
	vmhd.Write([]byte{0, 0, 0, 1})
	vmhd.Write(u16buf(0))
	vmhd.Write(make([]byte, 6))

	stsd := &bytes.Buffer{}
	stsd.Write([]byte{0, 0, 0, 0})
	stsd.Write(u32buf(1))
	stsd.Write(buildVisualSampleEntry(width, height))

	stts := buildStts(n, 1)
	stsc := buildSingleChunkStsc(n)

	stsz := &bytes.Buffer{}
	stsz.Write([]byte{0, 0, 0, 0})
	stsz.Write(u32buf(0))
	stsz.Write(u32buf(n))
	for _, s := range samples {
		stsz.Write(u32buf(uint32(len(s))))
	}

	stbl := &bytes.Buffer{}
	appendBox(stbl, "stsd", stsd.Bytes())
	appendBox(stbl, "stts", stts)
	appendBox(stbl, "stsc", stsc)
	appendBox(stbl, "stsz", stsz.Bytes())
	appendChunkOffsetBox(stbl, chunkOffset, use64)

	minf := &bytes.Buffer{}
	appendBox(minf, "vmhd", vmhd.Bytes())
	appendBox(minf, "dinf", buildSelfContainedDinf())
	appendBox(minf, "stbl", stbl.Bytes())

	mdia := &bytes.Buffer{}
	appendBox(mdia, "mdhd", mdhd.Bytes())
	appendBox(mdia, "hdlr", hdlr)
	appendBox(mdia, "minf", minf.Bytes())

	trak := &bytes.Buffer{}
	appendBox(trak, "tkhd", tkhd.Bytes())
	appendBox(trak, "mdia", mdia.Bytes())

	buf := &bytes.Buffer{}
	appendBox(buf, "trak", trak.Bytes())
	return buf.Bytes()
}

func buildAudioTrack(trackID, movieDuration uint32, audio *MuxAudio, sampleCount uint32, chunkOffset uint64, use64 bool) []byte {
	tkhd := &bytes.Buffer{}
	tkhd.Write([]byte{0, 0, 0, 7})
	tkhd.Write(u32buf(0))
	tkhd.Write(u32buf(0))
	tkhd.Write(u32buf(trackID))
	tkhd.Write(u32buf(0))
	tkhd.Write(u32buf(movieDuration))
	tkhd.Write(make([]byte, 8))
	tkhd.Write(u16buf(0))
	tkhd.Write(u16buf(0))
	tkhd.Write(u16buf(0x0100))
	tkhd.Write(make([]byte, 2))
	tkhd.Write(identityMatrix())
	tkhd.Write(u32buf(0))
	tkhd.Write(u32buf(0))

	mdhd := &bytes.Buffer{}
	mdhd.Write([]byte{0, 0, 0, 0})
	mdhd.Write(u32buf(0))
	mdhd.Write(u32buf(0))
	mdhd.Write(u32buf(uint32(audio.SampleRate)))
	mdhd.Write(u32buf(sampleCount))
	mdhd.Write(u16buf(0))
	mdhd.Write(u16buf(0))

	hdlr := buildHdlr("soun", "SoundHandler\x00")

	smhd := &bytes.Buffer{}
	smhd.Write([]byte{0, 0, 0, 0})
	smhd.Write(u16buf(0)) // balance
	smhd.Write(u16buf(0)) // reserved

	stsd := &bytes.Buffer{}
	stsd.Write([]byte{0, 0, 0, 0})
	stsd.Write(u32buf(1))
	stsd.Write(buildAudioSampleEntry(audio.Channels, audio.SampleRate))

	stts := buildStts(sampleCount, 1)
	stsc := buildSingleChunkStsc(sampleCount)

	// Uniform PCM frame size (channels * 2 bytes per 16-bit sample).
	stsz := &bytes.Buffer{}
	stsz.Write([]byte{0, 0, 0, 0})
	stsz.Write(u32buf(uint32(audio.Channels * 2)))
	stsz.Write(u32buf(sampleCount))

	stbl := &bytes.Buffer{}
	appendBox(stbl, "stsd", stsd.Bytes())
	appendBox(stbl, "stts", stts)
	appendBox(stbl, "stsc", stsc)
	appendBox(stbl, "stsz", stsz.Bytes())
	appendChunkOffsetBox(stbl, chunkOffset, use64)

	minf := &bytes.Buffer{}
	appendBox(minf, "smhd", smhd.Bytes())
	appendBox(minf, "dinf", buildSelfContainedDinf())
	appendBox(minf, "stbl", stbl.Bytes())

	mdia := &bytes.Buffer{}
	appendBox(mdia, "mdhd", mdhd.Bytes())
	appendBox(mdia, "hdlr", hdlr)
	appendBox(mdia, "minf", minf.Bytes())

	trak := &bytes.Buffer{}
	appendBox(trak, "tkhd", tkhd.Bytes())
	appendBox(trak, "mdia", mdia.Bytes())

	buf := &bytes.Buffer{}
	appendBox(buf, "trak", trak.Bytes())
	return buf.Bytes()
}

func buildHdlr(handlerType, name string) []byte {
	hdlr := &bytes.Buffer{}
	hdlr.Write([]byte{0, 0, 0, 0})
	hdlr.Write(u32buf(0))
	hdlr.WriteString(handlerType)
	hdlr.Write(make([]byte, 12))
	hdlr.WriteString(name)
	return hdlr.Bytes()
}

func buildSelfContainedDinf() []byte {
	urlBox := &bytes.Buffer{}
	urlBox.Write(u32buf(12))
	urlBox.WriteString("url ")
	urlBox.Write(u32buf(1))

	dref := &bytes.Buffer{}
	dref.Write([]byte{0, 0, 0, 0})
	dref.Write(u32buf(1))
	dref.Write(urlBox.Bytes())

	dinf := &bytes.Buffer{}
	appendBox(dinf, "dref", dref.Bytes())
	return dinf.Bytes()
}

func buildStts(sampleCount, delta uint32) []byte {
	stts := &bytes.Buffer{}
	stts.Write([]byte{0, 0, 0, 0})
	stts.Write(u32buf(1))
	stts.Write(u32buf(sampleCount))
	stts.Write(u32buf(delta))
	return stts.Bytes()
}

func buildSingleChunkStsc(sampleCount uint32) []byte {
	stsc := &bytes.Buffer{}
	stsc.Write([]byte{0, 0, 0, 0})
	stsc.Write(u32buf(1))
	stsc.Write(u32buf(1))
	stsc.Write(u32buf(sampleCount))
	stsc.Write(u32buf(1))
	return stsc.Bytes()
}

func appendChunkOffsetBox(stbl *bytes.Buffer, offset uint64, use64 bool) {
	b := &bytes.Buffer{}
	b.Write([]byte{0, 0, 0, 0})
	b.Write(u32buf(1))
	if use64 {
		b.Write(u64buf(offset))
		appendBox(stbl, "co64", b.Bytes())
	} else {
		b.Write(u32buf(uint32(offset)))
		appendBox(stbl, "stco", b.Bytes())
	}
}

func buildVisualSampleEntry(width, height int) []byte {
	fields := &bytes.Buffer{}
	fields.Write(make([]byte, 6))
	fields.Write(u16buf(1))
	fields.Write(u16buf(0))
	fields.Write(make([]byte, 2))
	fields.Write(make([]byte, 12))
	fields.Write(u16buf(uint16(width)))
	fields.Write(u16buf(uint16(height)))
	fields.Write(u32buf(0x00480000))
	fields.Write(u32buf(0x00480000))
	fields.Write(u32buf(0))
	fields.Write(u16buf(1))
	var cname [32]byte
	name := []byte("Photo - JPEG")
	cname[0] = byte(len(name))
	copy(cname[1:], name)
	fields.Write(cname[:])
	fields.Write(u16buf(24))
	fields.Write(u16buf(0xFFFF))

	buf := &bytes.Buffer{}
	appendBox(buf, "jpeg", fields.Bytes())
	return buf.Bytes()
}

func buildAudioSampleEntry(channels, sampleRate int) []byte {
	fields := &bytes.Buffer{}
	fields.Write(make([]byte, 6))
	fields.Write(u16buf(1))                // data_reference_index
	fields.Write(u16buf(0))                // version
	fields.Write(u16buf(0))                // revision_level
	fields.Write(u32buf(0))                // vendor
	fields.Write(u16buf(uint16(channels))) // channelcount
	fields.Write(u16buf(16))               // samplesize
	fields.Write(u16buf(0))                // compression_id
	fields.Write(u16buf(0))                // packet_size
	fields.Write(u32buf(uint32(sampleRate) << 16))

	buf := &bytes.Buffer{}
	appendBox(buf, "sowt", fields.Bytes())
	return buf.Bytes()
}

func identityMatrix() []byte {
	return []byte{
		0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x40, 0x00, 0x00, 0x00,
	}
}

func appendBox(buf *bytes.Buffer, boxType string, payload []byte) {
	buf.Write(u32buf(uint32(len(payload) + 8)))
	buf.WriteString(boxType)
	buf.Write(payload)
}

func u64buf(v uint64) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, v)
	return b
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
