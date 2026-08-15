# GDBG — Godot Debug Bridge

> Let agents run, test, and debug Godot games without opening the editor.

**Status: pre-alpha / repository scaffold**

GDBG is being designed as a CLI-first, headless-friendly bridge for agent-driven
Godot development. This repository currently defines only the intended project
layout; it does not yet ship a working runtime, CLI, or stable protocol.

GDBG will consist of two parts:

- **Godot addon** — connects the bridge to a running game.
- **CLI** — starts, controls, observes, and debugs games from an agent, shell, or
  CI environment.

The core workflow is intended to work without opening the Godot Editor. Editor
integration is not part of the current core scope.

## Repository layout

- `addons/gdbg/` — Godot runtime addon scaffold.
- `cli/` — future `gdbg` command-line application.
- `protocol/` — future versioned CLI/runtime protocol documentation.
- `docs/` — architecture and product documentation.
- `examples/` — future example Godot projects.

## License

GDBG is licensed under the MIT License.
