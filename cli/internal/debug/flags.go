// godot-debug-bridge/cli/internal/debug/flags.go
package debug

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Flags holds the parsed command-line options for a `gdbg debug` invocation.
// The Has* fields distinguish "flag absent" from "flag present with value 0",
// which the payload builders rely on (mirroring the old Node client).
type Flags struct {
	Screenshot bool

	Delay    float64
	Count    int
	Interval float64
	Width    int
	Height   int
	Scale    float64
	Seconds  float64
	FPS      float64
	Output   string

	// ProjectDir is accepted for argument compatibility; the native bridge no
	// longer needs the project directory (IPC lives under $GDBG_STATE).
	ProjectDir string

	HasDelay    bool
	HasCount    bool
	HasInterval bool
	HasWidth    bool
	HasHeight   bool
	HasScale    bool
	HasSeconds  bool
	HasFPS      bool
}

// ParseFlags splits args into flags and positional arguments. Numeric parsing
// mirrors the legacy Node client: delay/interval/scale fall back to their
// default on invalid input, while seconds/fps keep NaN so the record validator
// can reject them with a clear error.
func ParseFlags(args []string) (Flags, []string, error) {
	var f Flags
	var positional []string

	i := 0
	for i < len(args) {
		arg := args[i]
		next := func() (string, bool) {
			if i+1 < len(args) {
				return args[i+1], true
			}
			return "", false
		}
		advance := func() {
			i += 2
		}

		switch arg {
		case "--screenshot", "-s":
			f.Screenshot = true
			i++
		case "--delay", "-d":
			v, _ := next()
			parsed := parseFloat(v)
			if math.IsNaN(parsed) {
				parsed = 0
			}
			f.Delay = parsed
			f.HasDelay = true
			advance()
		case "--count", "-c":
			v, _ := next()
			n := parseIntLeading(v)
			if n == 0 {
				n = 1
			}
			f.Count = n
			f.HasCount = true
			advance()
		case "--interval", "-i":
			v, _ := next()
			parsed := parseFloat(v)
			if math.IsNaN(parsed) {
				parsed = 1
			}
			f.Interval = parsed
			f.HasInterval = true
			advance()
		case "--width":
			v, _ := next()
			f.Width = parseIntLeading(v)
			f.HasWidth = true
			advance()
		case "--height":
			v, _ := next()
			f.Height = parseIntLeading(v)
			f.HasHeight = true
			advance()
		case "--scale":
			v, _ := next()
			parsed := parseFloat(v)
			if math.IsNaN(parsed) {
				parsed = 0
			}
			f.Scale = parsed
			f.HasScale = true
			advance()
		case "--seconds":
			v, _ := next()
			f.Seconds = parseFloat(v)
			f.HasSeconds = true
			advance()
		case "--fps":
			v, _ := next()
			f.FPS = parseFloat(v)
			f.HasFPS = true
			advance()
		case "--output":
			v, ok := next()
			if !ok || v == "" || strings.HasPrefix(v, "-") {
				return f, nil, fmt.Errorf("Flag --output requires a .mp4 or .mov path argument")
			}
			f.Output = v
			advance()
		case "--project-dir", "--path", "-p":
			v, ok := next()
			if !ok || v == "" || strings.HasPrefix(v, "-") {
				return f, nil, fmt.Errorf("Flag --project-dir requires a directory argument")
			}
			f.ProjectDir = v
			advance()
		default:
			positional = append(positional, arg)
			i++
		}
	}

	return f, positional, nil
}

// parseFloat mirrors JS Number()/parseFloat: NaN for unparsable input.
func parseFloat(s string) float64 {
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return math.NaN()
	}
	return v
}

// parseIntLeading mirrors JS parseInt(): it parses the leading integer portion
// of the string and returns 0 when nothing parses.
func parseIntLeading(s string) int {
	t := strings.TrimSpace(s)
	i := 0
	neg := false
	if i < len(t) && (t[i] == '-' || t[i] == '+') {
		neg = t[i] == '-'
		i++
	}
	start := i
	for i < len(t) && t[i] >= '0' && t[i] <= '9' {
		i++
	}
	if i == start {
		return 0
	}
	n, err := strconv.Atoi(t[start:i])
	if err != nil {
		return 0
	}
	if neg {
		n = -n
	}
	return n
}
