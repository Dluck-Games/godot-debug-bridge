//go:build !darwin

package record

import (
	"fmt"
)

// TranscodeToH264 falls back to the pure-Go MJPEG(+PCM) container when built-in
// OS H.264 transcoding is unavailable, or fails explicitly if --codec h264 was
// forced on a non-macOS host.
func TranscodeToH264(sourceMOV, outputPath string, mode Codec) (string, error) {
	if mode == CodecH264 {
		return "", fmt.Errorf("built-in --codec h264 is only available on macOS (avconvert); use --codec auto or --codec mjpeg")
	}
	if err := copyFile(sourceMOV, outputPath); err != nil {
		return "", err
	}
	return "mjpeg", nil
}
