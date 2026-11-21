package record

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
)

// AVIStreamChunk identifies one frame or audio chunk inside an AVI file.
type AVIStreamChunk struct {
	Offset int64
	Size   int64
}

// AVIAudioFormat describes an uncompressed PCM audio stream inside an AVI.
type AVIAudioFormat struct {
	Present       bool
	StreamIndex   int
	FormatTag     uint16
	Channels      int
	SampleRate    int
	BitsPerSample int
	BlockAlign    int
}

// AVIFile holds the parsed metadata and chunk index of a RIFF AVI file
// produced by Godot's MovieWriterMJPEG.
type AVIFile struct {
	Path        string
	Width       int
	Height      int
	FPS         int
	VideoStream int
	Audio       AVIAudioFormat

	VideoChunks []AVIStreamChunk
	AudioChunks []audioChunkEntry

	audioBytesPerSample int
	totalAudioSamples   int64
}

type audioChunkEntry struct {
	Offset      int64
	Size        int64
	StartSample int64
	SampleCount int64
}

// OpenAVI scans the RIFF headers and movi chunk index of path without loading
// frame payloads into memory. Supports standard AVI and OpenDML AVIX chunks.
func OpenAVI(path string) (*AVIFile, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open avi: %w", err)
	}
	defer f.Close()

	st, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat avi: %w", err)
	}
	fileSize := st.Size()
	if fileSize < 12 {
		return nil, errors.New("avi file too small")
	}

	avi := &AVIFile{
		Path:        path,
		VideoStream: -1,
		Audio:       AVIAudioFormat{StreamIndex: -1},
	}

	var offset int64
	streamIndex := 0
	var currentStreamType string

	for offset+12 <= fileSize {
		hdr, err := readChunkHeader(f, offset)
		if err != nil {
			break
		}
		if hdr.FourCC != "RIFF" {
			next := offset + 8 + paddedSize(hdr.Size)
			if next <= offset {
				break
			}
			offset = next
			continue
		}

		var formType [4]byte
		if _, err := readAtFull(f, formType[:], offset+8); err != nil {
			return nil, fmt.Errorf("read RIFF form type: %w", err)
		}
		form := string(formType[:])
		riffEnd := offset + 8 + int64(hdr.Size)
		if riffEnd > fileSize {
			riffEnd = fileSize
		}

		if form == "AVI " || form == "AVIX" {
			if err := scanRIFFContainer(f, offset+12, riffEnd, avi, &streamIndex, &currentStreamType); err != nil {
				return nil, err
			}
		}

		next := offset + 8 + paddedSize(hdr.Size)
		if next <= offset {
			break
		}
		offset = next
	}

	if len(avi.VideoChunks) == 0 {
		return nil, errors.New("avi contains no video frames")
	}
	if avi.Width <= 0 || avi.Height <= 0 {
		return nil, fmt.Errorf("avi has invalid video dimensions %dx%d", avi.Width, avi.Height)
	}
	if avi.FPS <= 0 {
		avi.FPS = 30
	}
	return avi, nil
}

type chunkHeader struct {
	FourCC string
	Size   uint32
}

func readChunkHeader(r io.ReaderAt, offset int64) (chunkHeader, error) {
	var buf [8]byte
	if _, err := readAtFull(r, buf[:], offset); err != nil {
		return chunkHeader{}, err
	}
	return chunkHeader{
		FourCC: string(buf[:4]),
		Size:   binary.LittleEndian.Uint32(buf[4:8]),
	}, nil
}

func readAtFull(r io.ReaderAt, dst []byte, offset int64) (int, error) {
	total := 0
	for total < len(dst) {
		n, err := r.ReadAt(dst[total:], offset+int64(total))
		total += n
		if err != nil {
			if err == io.EOF && total == len(dst) {
				return total, nil
			}
			return total, err
		}
	}
	return total, nil
}

func paddedSize(size uint32) int64 {
	s := int64(size)
	if s%2 == 1 {
		s++
	}
	return s
}

func scanRIFFContainer(f io.ReaderAt, start, end int64, avi *AVIFile, streamIndex *int, currentStreamType *string) error {
	pos := start
	for pos+8 <= end {
		hdr, err := readChunkHeader(f, pos)
		if err != nil {
			return nil
		}
		dataStart := pos + 8
		dataEnd := dataStart + int64(hdr.Size)
		if dataEnd > end {
			dataEnd = end
		}

		switch hdr.FourCC {
		case "LIST":
			if dataStart+4 > end {
				return nil
			}
			var listType [4]byte
			if _, err := readAtFull(f, listType[:], dataStart); err != nil {
				return err
			}
			lt := string(listType[:])
			if lt == "strl" {
				*currentStreamType = ""
				if err := scanRIFFContainer(f, dataStart+4, dataEnd, avi, streamIndex, currentStreamType); err != nil {
					return err
				}
				*streamIndex++
			} else if lt == "hdrl" || lt == "movi" || lt == "rec " {
				if err := scanRIFFContainer(f, dataStart+4, dataEnd, avi, streamIndex, currentStreamType); err != nil {
					return err
				}
			}
		case "avih":
			if err := parseAVIH(f, dataStart, dataEnd-dataStart, avi); err != nil {
				return err
			}
		case "strh":
			st, err := parseSTRH(f, dataStart, dataEnd-dataStart, *streamIndex, avi)
			if err != nil {
				return err
			}
			*currentStreamType = st
		case "strf":
			if err := parseSTRF(f, dataStart, dataEnd-dataStart, *streamIndex, *currentStreamType, avi); err != nil {
				return err
			}
		default:
			if isVideoChunk(hdr.FourCC, avi.VideoStream) {
				avi.VideoChunks = append(avi.VideoChunks, AVIStreamChunk{
					Offset: dataStart,
					Size:   dataEnd - dataStart,
				})
			} else if avi.Audio.Present && isAudioChunk(hdr.FourCC, avi.Audio.StreamIndex) {
				sz := dataEnd - dataStart
				sampleCount := int64(0)
				if avi.audioBytesPerSample > 0 {
					sampleCount = sz / int64(avi.audioBytesPerSample)
				}
				avi.AudioChunks = append(avi.AudioChunks, audioChunkEntry{
					Offset:      dataStart,
					Size:        sz,
					StartSample: avi.totalAudioSamples,
					SampleCount: sampleCount,
				})
				avi.totalAudioSamples += sampleCount
			}
		}

		next := pos + 8 + paddedSize(hdr.Size)
		if next <= pos {
			break
		}
		pos = next
	}
	return nil
}

func isVideoChunk(fourCC string, videoStream int) bool {
	if len(fourCC) != 4 {
		return false
	}
	idx, ok := parseStreamDigits(fourCC[:2])
	if !ok {
		return false
	}
	if videoStream >= 0 && idx != videoStream {
		return false
	}
	return fourCC[2:] == "dc" || fourCC[2:] == "db"
}

func isAudioChunk(fourCC string, audioStream int) bool {
	if len(fourCC) != 4 {
		return false
	}
	idx, ok := parseStreamDigits(fourCC[:2])
	if !ok || idx != audioStream {
		return false
	}
	return fourCC[2:] == "wb"
}

func parseStreamDigits(s string) (int, bool) {
	if len(s) != 2 || s[0] < '0' || s[0] > '9' || s[1] < '0' || s[1] > '9' {
		return 0, false
	}
	return int(s[0]-'0')*10 + int(s[1]-'0'), true
}

func parseAVIH(f io.ReaderAt, offset, size int64, avi *AVIFile) error {
	if size < 40 {
		return nil
	}
	buf := make([]byte, 40)
	if _, err := readAtFull(f, buf, offset); err != nil {
		return fmt.Errorf("read avih: %w", err)
	}
	usPerFrame := binary.LittleEndian.Uint32(buf[0:4])
	if usPerFrame > 0 && avi.FPS == 0 {
		avi.FPS = int(math.Round(1_000_000.0 / float64(usPerFrame)))
	}
	w := int(binary.LittleEndian.Uint32(buf[32:36]))
	h := int(binary.LittleEndian.Uint32(buf[36:40]))
	if w > 0 && h > 0 {
		avi.Width = w
		avi.Height = h
	}
	return nil
}

func parseSTRH(f io.ReaderAt, offset, size int64, streamIdx int, avi *AVIFile) (string, error) {
	if size < 32 {
		return "", nil
	}
	buf := make([]byte, 32)
	if _, err := readAtFull(f, buf, offset); err != nil {
		return "", fmt.Errorf("read strh: %w", err)
	}
	fccType := string(buf[0:4])
	scale := binary.LittleEndian.Uint32(buf[20:24])
	rate := binary.LittleEndian.Uint32(buf[24:28])
	if fccType == "vids" {
		avi.VideoStream = streamIdx
		if scale > 0 && rate > 0 {
			avi.FPS = int(math.Round(float64(rate) / float64(scale)))
		}
	} else if fccType == "auds" {
		avi.Audio.StreamIndex = streamIdx
	}
	return fccType, nil
}

func parseSTRF(f io.ReaderAt, offset, size int64, streamIdx int, streamType string, avi *AVIFile) error {
	switch streamType {
	case "vids":
		if size < 12 {
			return nil
		}
		buf := make([]byte, 12)
		if _, err := readAtFull(f, buf, offset); err != nil {
			return fmt.Errorf("read vids strf: %w", err)
		}
		w := int(int32(binary.LittleEndian.Uint32(buf[4:8])))
		h := int(int32(binary.LittleEndian.Uint32(buf[8:12])))
		if h < 0 {
			h = -h
		}
		if w > 0 && h > 0 {
			avi.Width = w
			avi.Height = h
		}
	case "auds":
		if size < 16 {
			return nil
		}
		readLen := size
		if readLen > 40 {
			readLen = 40
		}
		buf := make([]byte, readLen)
		if _, err := readAtFull(f, buf, offset); err != nil {
			return fmt.Errorf("read auds strf: %w", err)
		}
		tag := binary.LittleEndian.Uint16(buf[0:2])
		channels := int(binary.LittleEndian.Uint16(buf[2:4]))
		sampleRate := int(binary.LittleEndian.Uint32(buf[4:8]))
		blockAlign := int(binary.LittleEndian.Uint16(buf[12:14]))
		bits := int(binary.LittleEndian.Uint16(buf[14:16]))
		if tag == 0xFFFE && len(buf) >= 26 {
			tag = binary.LittleEndian.Uint16(buf[24:26])
		}
		if channels > 0 && sampleRate > 0 && (tag == 1 || tag == 3) && (bits == 8 || bits == 16 || bits == 24 || bits == 32) {
			avi.Audio = AVIAudioFormat{
				Present:       true,
				StreamIndex:   streamIdx,
				FormatTag:     tag,
				Channels:      channels,
				SampleRate:    sampleRate,
				BitsPerSample: bits,
				BlockAlign:    blockAlign,
			}
			avi.audioBytesPerSample = channels * (bits / 8)
		}
	}
	return nil
}

// FrameCount returns the number of indexed video frames.
func (a *AVIFile) FrameCount() int {
	return len(a.VideoChunks)
}

// ReadVideoFrames reads the raw JPEG payloads for frame indices [start, end).
// Zero-byte placeholder chunks repeat the previous frame's JPEG payload.
func (a *AVIFile) ReadVideoFrames(start, end int) ([][]byte, error) {
	if start < 0 || end > len(a.VideoChunks) || start >= end {
		return nil, fmt.Errorf("invalid video frame range [%d, %d) for %d frames", start, end, len(a.VideoChunks))
	}
	f, err := os.Open(a.Path)
	if err != nil {
		return nil, fmt.Errorf("open avi: %w", err)
	}
	defer f.Close()

	frames := make([][]byte, 0, end-start)
	var lastNonEmpty []byte
	if start > 0 && a.VideoChunks[start].Size == 0 {
		for idx := start - 1; idx >= 0; idx-- {
			if a.VideoChunks[idx].Size > 0 {
				buf := make([]byte, a.VideoChunks[idx].Size)
				if _, err := readAtFull(f, buf, a.VideoChunks[idx].Offset); err != nil {
					return nil, err
				}
				lastNonEmpty = buf
				break
			}
		}
	}

	for i := start; i < end; i++ {
		c := a.VideoChunks[i]
		if c.Size == 0 {
			if len(lastNonEmpty) == 0 {
				return nil, fmt.Errorf("avi frame %d is empty with no prior frame", i)
			}
			dup := make([]byte, len(lastNonEmpty))
			copy(dup, lastNonEmpty)
			frames = append(frames, dup)
			continue
		}
		buf := make([]byte, c.Size)
		if _, err := readAtFull(f, buf, c.Offset); err != nil {
			return nil, fmt.Errorf("read avi frame %d: %w", i, err)
		}
		lastNonEmpty = buf
		frames = append(frames, buf)
	}
	return frames, nil
}

// ReadAudioPCM16 extracts 16-bit signed little-endian PCM matching video
// frames [startFrame, endFrame) at fps. Returns nil if the AVI has no audio.
func (a *AVIFile) ReadAudioPCM16(startFrame, endFrame, fps int) ([]byte, error) {
	if !a.Audio.Present || a.audioBytesPerSample == 0 || len(a.AudioChunks) == 0 {
		return nil, nil
	}
	if fps <= 0 {
		fps = a.FPS
	}
	if startFrame < 0 || endFrame <= startFrame || fps <= 0 {
		return nil, nil
	}

	startSample := int64(startFrame) * int64(a.Audio.SampleRate) / int64(fps)
	endSample := int64(endFrame) * int64(a.Audio.SampleRate) / int64(fps)
	wantSamples := endSample - startSample
	if wantSamples <= 0 {
		return nil, nil
	}

	outBytes := int(wantSamples) * a.Audio.Channels * 2
	out := make([]byte, outBytes) // zero-padded beyond EOF if AVI audio is shorter

	f, err := os.Open(a.Path)
	if err != nil {
		return nil, fmt.Errorf("open avi for audio: %w", err)
	}
	defer f.Close()

	srcBps := a.Audio.BitsPerSample / 8
	frameStride := a.audioBytesPerSample

	for _, chunk := range a.AudioChunks {
		chunkEnd := chunk.StartSample + chunk.SampleCount
		if chunkEnd <= startSample {
			continue
		}
		if chunk.StartSample >= endSample {
			break
		}

		overlapStart := maxInt64(chunk.StartSample, startSample)
		overlapEnd := minInt64(chunkEnd, endSample)
		if overlapEnd <= overlapStart {
			continue
		}

		localStart := overlapStart - chunk.StartSample
		localCount := overlapEnd - overlapStart
		readOff := chunk.Offset + localStart*int64(frameStride)
		raw := make([]byte, localCount*int64(frameStride))
		if _, err := readAtFull(f, raw, readOff); err != nil {
			return nil, fmt.Errorf("read avi audio chunk: %w", err)
		}

		dstOffset := int(overlapStart-startSample) * a.Audio.Channels * 2
		convertPCMToS16LE(out[dstOffset:], raw, int(localCount)*a.Audio.Channels, a.Audio.FormatTag, srcBps)
	}

	return out, nil
}

func convertPCMToS16LE(dst, src []byte, totalScalarSamples int, formatTag uint16, srcBytesPerScalar int) {
	switch {
	case formatTag == 1 && srcBytesPerScalar == 2:
		copy(dst, src[:totalScalarSamples*2])
	case formatTag == 1 && srcBytesPerScalar == 4:
		// 32-bit signed LE PCM -> upper 16 bits.
		for i := 0; i < totalScalarSamples; i++ {
			dst[i*2] = src[i*4+2]
			dst[i*2+1] = src[i*4+3]
		}
	case formatTag == 1 && srcBytesPerScalar == 3:
		// 24-bit signed LE PCM -> upper 16 bits.
		for i := 0; i < totalScalarSamples; i++ {
			dst[i*2] = src[i*3+1]
			dst[i*2+1] = src[i*3+2]
		}
	case formatTag == 1 && srcBytesPerScalar == 1:
		// 8-bit unsigned PCM -> signed 16-bit LE.
		for i := 0; i < totalScalarSamples; i++ {
			s := int16(src[i]-128) << 8
			binary.LittleEndian.PutUint16(dst[i*2:], uint16(s))
		}
	case formatTag == 3 && srcBytesPerScalar == 4:
		// 32-bit IEEE float -> signed 16-bit LE.
		for i := 0; i < totalScalarSamples; i++ {
			bits := binary.LittleEndian.Uint32(src[i*4:])
			v := math.Float32frombits(bits)
			if v > 1.0 {
				v = 1.0
			} else if v < -1.0 {
				v = -1.0
			}
			s := int16(math.Round(float64(v) * 32767.0))
			binary.LittleEndian.PutUint16(dst[i*2:], uint16(s))
		}
	}
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func minInt64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}
