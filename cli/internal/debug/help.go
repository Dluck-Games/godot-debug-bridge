package debug

const HelpText = `gdbg debug - control a running Godot project through GDBG

Usage:
  gdbg debug <command> [args...] [flags]
  gdbg debug console <project-command> [args...] [flags]

Bridge commands:
  screenshot                       Capture the current viewport (windowed game)
  record                           Record viewport frames (windowed game)
  fetch <capture_id>               Retrieve an asynchronous screenshot result
  script <file.gd>                 Execute a GDScript file in the running game
  input actions                    List injectable input actions
  input press <action>             Press and hold an input action
  input release <action>           Release an input action
  input tap <action>               Press for one frame, then release
  input hold <action> <seconds>    Hold an action for a duration

All other commands are forwarded unchanged to the project's DebugBridgeHost.
Games define their own command modules with the console framework in
addons/gdbg/console/. Run 'gdbg debug help' to ask the running game for its
available commands.

Screenshot and recording require a rendered viewport. Start the game with
'gdbg run game --windowed' before using those commands; console and input work
with the default headless launch.

Screenshot and recording flags:
  -d, --delay <seconds>            Wait before the first capture
  -c, --count <N>                  Number of screenshots
  -i, --interval <seconds>         Time between captures
  --width <pixels>                 Resize to a target width
  --height <pixels>                Resize to a target height
  --scale <factor>                 Resize by a viewport scale factor
  --seconds <seconds>              Recording duration (1-300)
  --fps <frames>                   Recording frame rate (1-12)
  --output <path.mp4|path.mov>     Recording output path
  -s, --screenshot                 Attach a screenshot to a console command

Examples:
  gdbg debug help
  gdbg debug screenshot --width 1920 --height 1080
  gdbg debug record --seconds 10 --fps 4
  gdbg debug input tap interact
  gdbg debug console my-command value
  gdbg debug script inspect_state.gd
`

const RecordHelpText = `gdbg debug record - record the Godot viewport

Usage:
  gdbg debug record [--seconds N] [--fps N] [--output path.mp4|path.mov] [--width N] [--height N] [--scale N]

Requires a running game with the GDBG runtime addon. Frames are encoded by the
gdbg binary; no external encoder is required.
`
