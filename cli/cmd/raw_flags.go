package cmd

import (
	"fmt"
	"strconv"
	"strings"
)

// consumeRawPersistentFlags handles root flags that Cobra deliberately leaves
// in the argument list for raw forwarding commands. Only leading flags are
// consumed, so project-defined command arguments such as "--path" remain
// untouched once the actual debug/console command has started.
func consumeRawPersistentFlags(args []string) ([]string, error) {
	for len(args) > 0 {
		arg := args[0]
		switch {
		case arg == "--project-dir" || arg == "--path":
			if len(args) < 2 {
				return nil, fmt.Errorf("%s requires a value", arg)
			}
			flagProjectDir = args[1]
			args = args[2:]
		case strings.HasPrefix(arg, "--project-dir="):
			flagProjectDir = strings.TrimPrefix(arg, "--project-dir=")
			args = args[1:]
		case strings.HasPrefix(arg, "--path="):
			flagProjectDir = strings.TrimPrefix(arg, "--path=")
			args = args[1:]
		case arg == "--verbose" || arg == "-v":
			flagVerbose = true
			args = args[1:]
		case strings.HasPrefix(arg, "--verbose=") || strings.HasPrefix(arg, "-v="):
			value := strings.SplitN(arg, "=", 2)[1]
			parsed, err := strconv.ParseBool(value)
			if err != nil {
				return nil, fmt.Errorf("invalid verbose value %q", value)
			}
			flagVerbose = parsed
			args = args[1:]
		default:
			return args, nil
		}
	}
	return args, nil
}
