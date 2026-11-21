package record

import (
	"bytes"
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"testing"
)

func makeAVIStreamHeader(fccType, fccHandler string, scale, rate, length uint32) []byte {
	b := make([]byte, 56)
	copy(b[0:4], fccType)
	copy(b[4:8], fccHandler)
	binary.LittleEndian.PutUint32(b[20:24], scale)
	binary.LittleEndian.PutUint32(b[24:28], rate)
	binary.LittleEndian.PutUint32(b[32:36], length)
	return b
}

func makeAVIBitmapInfo(width, height int) []byte {
	b := make([]byte, 40)
	binary.LittleEndian.PutUint32(b[0:4], 40)
	binary.LittleEndian.PutUint32(b[4:8], uint32(width))
	binary.LittleEndian.PutUint32(b[8:12], uint32(height))
	binary.LittleEndian.PutUint16(b[12:14], 1)
	binary.LittleEndian.PutUint16(b[14:16], 24)
	copy(b[16:20], "MJPG")
	return b
}

func makeWaveFormatEx(tag uint16, channels, sampleRate, bits int) []byte {
	b := make([]byte, 16)
	blockAlign := channels * (bits / 8)
	binary.LittleEndian.PutUint16(b[0:2], tag)
	binary.LittleEndian.PutUint16(b[2:4], uint16(channels))
	binary.LittleEndian.PutUint32(b[4:8], uint32(sampleRate))
	binary.LittleEndian.PutUint32(b[8:12], uint32(sampleRate*blockAlign))
	binary.LittleEndian.PutUint16(b[12:14], uint16(blockAlign))
	binary.LittleEndian.PutUint16(b[14:16], uint16(bits))
	return b
}

func riffChunk(fourCC string, payload []byte) []byte {
	var buf bytes.Buffer
	buf.WriteString(fourCC)
	var sz [4]byte
	binary.LittleEndian.PutUint32(sz[:], uint32(len(payload)))
	buf.Write(sz[:])
	buf.Write(payload)
	if len(payload)%2 == 1 {
		buf.WriteByte(0)
	}
	return buf.Bytes()
}

func riffList(tag, listType string, body []byte) []byte {
	var p bytes.Buffer
	p.WriteString(listType)
	p.Write(body)
	return riffChunk(tag, p.Bytes())
}

func TestOpenAVI_AndSliceFramesWithPCM32(t *testing.T) {
	const (
		width      = 64
		height     = 48
		fps        = 30
		sampleRate = 48000
		channels   = 2
	)
	samplesPerFrame := sampleRate / fps // 1600 stereo samples per frame

	avih := make([]byte, 56)
	binary.LittleEndian.PutUint32(avih[0:4], 1_000_000/fps)
	binary.LittleEndian.PutUint32(avih[16:20], 4)
	binary.LittleEndian.PutUint32(avih[24:28], 2)
	binary.LittleEndian.PutUint32(avih[32:36], width)
	binary.LittleEndian.PutUint32(avih[36:40], height)

	strlV := riffList("LIST", "strl", append(
		riffChunk("strh", makeAVIStreamHeader("vids", "MJPG", 1, fps, 4)),
		riffChunk("strf", makeAVIBitmapInfo(width, height))...,
	))
	strlA := riffList("LIST", "strl", append(
		riffChunk("strh", makeAVIStreamHeader("auds", "", 1, sampleRate, uint32(4*samplesPerFrame))),
		riffChunk("strf", makeWaveFormatEx(1, channels, sampleRate, 32))...,
	))
	var hdrlBody bytes.Buffer
	hdrlBody.Write(riffChunk("avih", avih))
	hdrlBody.Write(strlV)
	hdrlBody.Write(strlA)
	hdrl := riffList("LIST", "hdrl", hdrlBody.Bytes())

	makeAudioChunk := func(frameIdx int) []byte {
		buf := make([]byte, samplesPerFrame*channels*4)
		val := int32((frameIdx + 1) * 0x01000000)
		for i := 0; i < samplesPerFrame*channels; i++ {
			binary.LittleEndian.PutUint32(buf[i*4:], uint32(val))
		}
		return buf
	}

	frame0 := []byte{0xFF, 0xD8, 0x00, 0xFF, 0xD9} // 5 bytes (odd length tests RIFF padding!)
	frame1 := []byte{0xFF, 0xD8, 0x11, 0x22, 0xFF, 0xD9}
	frame3 := []byte{0xFF, 0xD8, 0x33, 0x44, 0xFF, 0xD9}

	var movi1 bytes.Buffer
	movi1.Write(riffChunk("00dc", frame0))
	movi1.Write(riffChunk("01wb", makeAudioChunk(0)))
	movi1.Write(riffChunk("00dc", frame1))
	movi1.Write(riffChunk("01wb", makeAudioChunk(1)))

	// Put frames 2 (empty duplicate) and 3 inside an OpenDML AVIX continuation RIFF chunk.
	var movi2 bytes.Buffer
	movi2.Write(riffChunk("00dc", nil)) // repeats frame1
	movi2.Write(riffChunk("01wb", makeAudioChunk(2)))
	movi2.Write(riffChunk("00dc", frame3))
	movi2.Write(riffChunk("01wb", makeAudioChunk(3)))

	var aviBytes bytes.Buffer
	aviBytes.Write(riffList("RIFF", "AVI ", append(hdrl, riffList("LIST", "movi", movi1.Bytes())...)))
	aviBytes.Write(riffList("RIFF", "AVIX", riffList("LIST", "movi", movi2.Bytes())))

	aviPath := filepath.Join(t.TempDir(), "test.avi")
	if err := os.WriteFile(aviPath, aviBytes.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	avi, err := OpenAVI(aviPath)
	if err != nil {
		t.Fatalf("OpenAVI: %v", err)
	}
	if avi.Width != width || avi.Height != height || avi.FPS != fps {
		t.Fatalf("metadata = %dx%d @ %dfps", avi.Width, avi.Height, avi.FPS)
	}
	if !avi.Audio.Present || avi.Audio.SampleRate != sampleRate || avi.Audio.Channels != channels || avi.Audio.BitsPerSample != 32 {
		t.Fatalf("audio = %+v", avi.Audio)
	}
	if avi.FrameCount() != 4 {
		t.Fatalf("FrameCount = %d, want 4", avi.FrameCount())
	}

	frames, err := avi.ReadVideoFrames(1, 4)
	if err != nil {
		t.Fatalf("ReadVideoFrames: %v", err)
	}
	if len(frames) != 3 {
		t.Fatalf("got %d frames, want 3", len(frames))
	}
	if !bytes.Equal(frames[0], frame1) || !bytes.Equal(frames[1], frame1) || !bytes.Equal(frames[2], frame3) {
		t.Fatalf("unexpected frame payloads: %x", frames)
	}

	pcm, err := avi.ReadAudioPCM16(1, 3, fps)
	if err != nil {
		t.Fatalf("ReadAudioPCM16: %v", err)
	}
	wantBytes := 2 * samplesPerFrame * channels * 2
	if len(pcm) != wantBytes {
		t.Fatalf("pcm len = %d, want %d", len(pcm), wantBytes)
	}
	// Frame 1 had int32 0x02000000 -> int16 0x0200; Frame 2 had 0x03000000 -> int16 0x0300.
	firstSample := int16(binary.LittleEndian.Uint16(pcm[0:2]))
	secondFrameStart := samplesPerFrame * channels * 2
	secondSample := int16(binary.LittleEndian.Uint16(pcm[secondFrameStart : secondFrameStart+2]))
	if firstSample != 0x0200 || secondSample != 0x0300 {
		t.Fatalf("PCM samples = (%#x, %#x), want (0x200, 0x300)", firstSample, secondSample)
	}
}

func TestConvertPCMToS16LE_Float32(t *testing.T) {
	src := make([]byte, 8)
	binary.LittleEndian.PutUint32(src[0:4], math.Float32bits(0.5))
	binary.LittleEndian.PutUint32(src[4:8], math.Float32bits(-1.0))
	dst := make([]byte, 4)
	convertPCMToS16LE(dst, src, 2, 3, 4)
	s0 := int16(binary.LittleEndian.Uint16(dst[0:2]))
	s1 := int16(binary.LittleEndian.Uint16(dst[2:4]))
	if s0 < 16380 || s0 > 16385 || s1 != -32767 {
		t.Fatalf("float32->s16le = (%d, %d)", s0, s1)
	}
}
