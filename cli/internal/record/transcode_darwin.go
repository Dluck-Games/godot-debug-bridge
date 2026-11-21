//go:build darwin

package record

import (
	"fmt"
	"os"
	"os/exec"
)

// TranscodeToH264 uses macOS built-in /usr/bin/avconvert to compress an
// intermediate faststart MOV into compact H.264/AAC MP4/MOV with zero
// third-party tools. Returns the resolved codec name ("h264" or "mjpeg").
func TranscodeToH264(sourceMOV, outputPath string, mode Codec) (string, error) {
	if mode == CodecMJPEG {
		if err := copyFile(sourceMOV, outputPath); err != nil {
			return "", err
		}
		return "mjpeg", nil
	}

	avconvertPath, err := exec.LookPath("/usr/bin/avconvert")
	if err != nil {
		avconvertPath, err = exec.LookPath("avconvert")
	}
	if err != nil {
		if mode == CodecH264 {
			return "", fmt.Errorf("avconvert not found for --codec h264: %w", err)
		}
		if copyErr := copyFile(sourceMOV, outputPath); copyErr != nil {
			return "", copyErr
		}
		return "mjpeg", nil
	}

	presets := []string{"Preset3840x2160", "PresetHighestQuality"}
	var lastErr error
	for _, preset := range presets {
		_ = os.Remove(outputPath)
		cmd := exec.Command(
			avconvertPath,
			"--source", sourceMOV,
			"--output", outputPath,
			"--preset", preset,
			"--replace",
		)
		out, runErr := cmd.CombinedOutput()
		if runErr == nil {
			if st, statErr := os.Stat(outputPath); statErr == nil && st.Size() > 0 {
				return "h264", nil
			}
			lastErr = fmt.Errorf("avconvert (%s) produced empty output", preset)
		} else {
			lastErr = fmt.Errorf("avconvert (%s): %w: %s", preset, runErr, string(out))
		}
	}

	if mode == CodecH264 {
		return "", lastErr
	}
	if copyErr := copyFile(sourceMOV, outputPath); copyErr != nil {
		return "", copyErr
	}
	return "mjpeg", nil
}
