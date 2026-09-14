# GDBG — Godot Debug Bridge

<img src="addons/gdbg/icon.png" alt="GDBG terminal bug icon" width="128" height="128">

> Let agents run, test, and debug Godot games without opening the editor.

GDBG is a CLI-first, headless-friendly toolkit for controlling a running Godot
project from an agent, shell, or CI job. The repository ships one product with
two parts:

- `addons/gdbg/` — the Godot runtime addon: versioned IPC, screenshots, input
  injection, an extensible console-command framework, and integration/playtest
  harnesses.
- `gdbg` — the Go CLI: addon installation, project launch/stop, console and
  debug commands, asset reimport, and unit/integration/playtest execution.

Version 0.1.2 is the current patch release. The protocol and extension APIs are
usable but should still be treated as early-stage interfaces.

## Requirements

- **Godot 4.3 or newer.** GDBG targets the Godot 4.x line. Godot 3.x and Godot
  4.0–4.2 are not supported.
- Full smoke coverage has been verified on **macOS** with **Godot 4.3** and
  **Godot 4.7.1**. Other OSes and intermediate Godot versions are not covered by
  that verification.
- The CLI addon installer relies on `--import`, which was introduced in Godot
  4.3; earlier engines cannot run the automated import step.

The `gdbg` CLI is a **separate required install** for the agent, shell, and CI
workflow. The addon on its own can be enabled and run inside Godot, but it does
not provide a standalone editor control UI — driving a project from an agent or
CI job requires the CLI.

GitHub Releases and CLI installation are the primary distribution channel.
Asset catalogs provide a complementary way to discover and install the Godot
addon. If you install through an asset catalog, follow the manual steps below
and install the CLI separately.

Keep the CLI and addon on the **same release version**. The CLI installer
installs a matching addon, and the addon archive is built from the same release
tag as the CLI.

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
checksum, and install to `~/.local/bin` by default. Set `GDBG_VERSION=0.1.2` to
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
/path/to/addons/gdbg` when developing from a repository checkout. The CLI
installer enables the plugin for you, so no manual plugin enablement is needed
on this path.

### Manual / Asset Library installation

1. Install or extract `addons/gdbg` directly at the project root so the files
   live at `res://addons/gdbg/` with no extra enclosing folder.
2. Open the project in Godot and wait for the import to finish.
3. Open **Project > Project Settings > Plugins** and enable **Godot Debug
   Bridge**.
4. Verify that the three autoloads are registered: `ScreenshotManager`,
   `AIDebugBridge`, and `DebugBridgeTestRuntime`.
5. Close the editor, then install the matching CLI version using the steps in
   [Install the CLI](#install-the-cli). Check that `gdbg version` matches the
   addon's `plugin.cfg` version.

Use `--project-dir` with every example command when the project is not the
current directory. Game-specific console commands are supplied by the host
project; see the [addon host adapter instructions](addons/gdbg/README.md#console-framework-and-host-api).

## Run and control a project

```sh
gdbg --project-dir /path/to/game run game --detach
gdbg --project-dir /path/to/game console help
gdbg --project-dir /path/to/game debug input tap interact
gdbg --project-dir /path/to/game stop game

# Screenshot and recording need a rendered viewport.
gdbg --project-dir /path/to/game run game --windowed --detach
gdbg --project-dir /path/to/game debug screenshot --width 1280
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

Run any project-owned scene directly through Godot in headless mode:

```sh
gdbg --project-dir /path/to/game test scene res://tests/smoke.tscn
gdbg --project-dir /path/to/game test scene res://bench/scenario.tscn -- --json --duration=60
```

`test scene` requires a project-relative `res://` path to a `.tscn` or `.scn`
file inside the selected project. Godot is launched headless, its output is
streamed to the terminal, and arguments after `--` are forwarded to the scene
as Godot user arguments (read them in the scene with
`OS.get_cmdline_user_args()`).

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
scripts/package.sh 0.1.2-dev
```

CI runs Go tests and vet, a clean Godot addon/console smoke test, public-content
checks, and cross-compiles installable macOS, Windows, and Linux archives.
Tagged `v*` builds create GitHub Releases with the CLI archives, addon archive,
and checksums.

## License

GDBG is licensed under the [MIT License](LICENSE).
