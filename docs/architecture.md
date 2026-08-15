# Architecture

```text
agent / shell / CI
        |
        v
     gdbg CLI
        |
        v
GDBG runtime addon
        |
        v
running Godot game
```

The CLI owns process management, package installation, test orchestration,
local artifact paths, and the client side of protocol version 1. The addon owns
the runtime IPC loop, screenshots, input injection, console dispatch, and the
game-agnostic test harnesses.

Game code stays behind two explicit adapters:

- `DebugBridgeHostBase` or `DebugBridgeConsoleHostBase` provides console
  commands, optional completion, and project input-service delegation.
- `DebugBridgeTestHostBase` provides typed fixture construction and production
  lifecycle operations used by integration and playtest suites.

Protocol version 1 is serialized file IPC under `GDBG_STATE`. That transport is
an implementation decision for 0.1, not a promise that later major protocol
versions cannot add another transport.

The addon contains an `EditorPlugin` only to register settings and autoloads.
Interactive editor integration is not a core goal: `gdbg addon install` enables
the plugin and invokes Godot's headless import path, and normal run/test/debug
flows do not require opening the editor UI.
