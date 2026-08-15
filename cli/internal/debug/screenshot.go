// godot-debug-bridge/cli/internal/debug/screenshot.go
package debug

import (
	"errors"
	"fmt"
	"math"
	"time"
)

// ScreenshotOpts builds the bridge screenshot args from parsed flags. Only
// explicitly provided options are included, mirroring the old client.
func ScreenshotOpts(f Flags) map[string]any {
	opts := map[string]any{}
	if f.HasDelay {
		opts["delay"] = f.Delay
	}
	if f.HasCount {
		opts["count"] = f.Count
	}
	if f.HasInterval {
		opts["interval"] = f.Interval
	}
	if f.HasWidth {
		opts["width"] = f.Width
	}
	if f.HasHeight {
		opts["height"] = f.Height
	}
	if f.HasScale {
		opts["scale"] = f.Scale
	}
	return opts
}

// CalculateTimeout bounds a screenshot bridge round trip:
// delay + (count-1)*interval + default timeout.
func CalculateTimeout(f Flags) time.Duration {
	delay := 0.0
	if f.HasDelay {
		delay = f.Delay
	}
	count := 1
	if f.HasCount {
		count = f.Count
	}
	interval := 1.0
	if f.HasInterval {
		interval = f.Interval
	}
	secs := delay + math.Max(0, float64(count-1))*interval + DefaultTimeout.Seconds()
	return time.Duration(secs * float64(time.Second))
}

// NormalizeScreenshotPaths extracts the frame path list from a screenshot
// bridge result. Accepts a single path string or a list of paths. A failure
// result is surfaced as an error with a readable message.
func NormalizeScreenshotPaths(result any) ([]string, error) {
	if IsFailureResult(result) {
		msg := "screenshot capture failed"
		switch v := result.(type) {
		case string:
			msg = v
		case map[string]any:
			if m, ok := v["message"].(string); ok && m != "" {
				msg = m
			}
		}
		return nil, errors.New(msg)
	}

	m, ok := result.(map[string]any)
	if !ok {
		return nil, errors.New("screenshot command did not return frame paths")
	}
	switch v := m["result"].(type) {
	case string:
		return []string{v}, nil
	case []any:
		out := make([]string, 0, len(v))
		for _, e := range v {
			out = append(out, fmt.Sprint(e))
		}
		return out, nil
	}
	return nil, errors.New("screenshot command did not return frame paths")
}
