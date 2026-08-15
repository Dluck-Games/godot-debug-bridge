# Architecture

**Status: pre-alpha / repository scaffold**

GDBG is organized around a CLI-first control path:

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

The CLI is the entry point for automation and external control. The runtime
addon is the in-game integration boundary. The game remains the authority for
its own runtime state and behavior.

Editor integration is not a current core goal. The communication transport
between the CLI and runtime addon is intentionally undecided at this stage.
