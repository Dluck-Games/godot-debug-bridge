# GDBG — Godot Debug Bridge

> Let agents run, test, and debug Godot games without opening the editor.

GDBG is a CLI-first, headless-friendly toolkit for controlling a running Godot
project from an agent, shell, or CI job. The repository ships one product with
two parts:

- `addons/gdbg/` — the Godot runtime addon: versioned IPC, screenshots, input
  injection, an extensible console-command framework, and integration/playtest
  harnesses.
- `gdbg` — the Go CLI: addon installation, project launch/stop, console and
  debug commands, asset reimport, and unit/integration/playtest execution.

Version 0.1.0 is the initial public release. The protocol and extension APIs
are usable but should still be treated as early-stage interfaces.

## Install the CLI

macOS and Linux:

```sh
curl -fsSL https://raw.githubusercontent.com/Dluck-Games/godot-debug-bridge/main/scripts/install.sh | sh
```

Windows PowerShell:

```powershell
irm https://raw.githubusercontent.com/Dluck-Games/godot-debug-bridge/main/scripts/install.ps1 | iex
```

Both installers download the latest GitHub Release artifact, verify its SHA-256
checksum, and install to `~/.local/bin` by default. Set `GDBG_VERSION=0.1.0` to
pin a version or `GDBG_INSTALL_DIR` to select another destination.

To build the current source instead:

```sh
git clone https://github.com/Dluck-Games/godot-debug-bridge.git
cd godot-debug-bridge/cli
go install .
```

## Install the addon

From any directory, point GDBG at a Godot project:

```sh
gdbg --project-dir /path/to/game addon install
```

This installs the addon matching the CLI release, adds
`res://addons/gdbg/plugin.cfg` to `project.godot`, and runs a complete headless
Godot import so the GDBG autoloads are registered. It does not require opening
the Godot Editor. Use `--force` to replace an existing addon or `--source
/path/to/addons/gdbg` when developing from a repository checkout.

## Run and control a project

```sh
gdbg --project-dir /path/to/game run game --detach
gdbg console help
gdbg debug input tap interact
gdbg --project-dir /path/to/game stop game

# Screenshot and recording need a rendered viewport.
gdbg --project-dir /path/to/game run game --windowed --detach
gdbg debug screenshot --width 1280
```

The default launch is headless and supports console, script, input, and process
control. Screenshot and recording commands require `run game --windowed`; a
headless capture request fails immediately with an actionable error.

The addon owns transport-level operations. Game-specific commands are supplied
by the project through `DebugBridgeConsoleHostBase` command modules and are
forwarded unchanged by both `gdbg console` and `gdbg debug`. See the
[minimal example](examples/minimal/README.md) and the
[addon host API](addons/gdbg/README.md#console-framework-and-host-api).

## Test projects

```sh
gdbg --project-dir /path/to/game test unit --suite all
gdbg --project-dir /path/to/game test integration --suite all
gdbg --project-dir /path/to/game test playtest --suite all --headless
```

The unit tier requires gdUnit4 at `addons/gdUnit4/`. GDBG checks for
`plugin.cfg` and `bin/GdUnitCmdTool.gd` before launching Godot and returns an
actionable error when gdUnit4 is not installed. Integration and playtest tiers
use the framework in `addons/gdbg/testing/`; the game supplies typed data and
lifecycle behavior through a `DebugBridgeTestHostBase` adapter.

## State and protocol

The CLI injects `GDBG_STATE` into launched games. Version 1 uses serialized file
IPC under `$GDBG_STATE/debug/ipc`; screenshots, recordings, reports, and logs
use sibling state directories. See the [version 1 protocol](protocol/v1.md).

The transport is local and unauthenticated. Console commands and staged
GDScript execute with the game's authority; only enable GDBG in environments
where local processes are trusted.

## Development

```sh
scripts/validate-public.sh
scripts/package.sh 0.1.0-dev
```

CI runs Go tests and vet, a clean Godot addon/console smoke test, public-content
checks, and cross-compiles installable macOS, Windows, and Linux archives.
Tagged `v*` builds create GitHub Releases with the CLI archives, addon archive,
and checksums.

## License

GDBG is licensed under the [MIT License](LICENSE).
