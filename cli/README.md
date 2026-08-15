# `gdbg` CLI

The CLI starts and stops Godot, installs the runtime addon, performs debug IPC,
forwards project console commands, reimports assets, and runs unit,
integration, and playtest tiers.

Build and verify:

```sh
go test ./...
go vet ./...
go build -o gdbg .
```

Important commands:

- `gdbg addon install` — install/enable `addons/gdbg` and run headless import.
- `gdbg run game --detach` / `gdbg stop game` — manage the game process.
- `gdbg console ...` — forward a game-defined console command.
- `gdbg debug ...` — screenshot, record, input, script, or console IPC.
- `gdbg test ...` — run gdUnit4, GDBG integration, and GDBG playtest tiers.
- `gdbg reimport` — rebuild Godot imports and UID sidecars.

Project resolution uses `--project-dir`, then `GDBG_PROJECT_DIR`, then walks up
from an interactive current directory. Runtime state uses `GDBG_STATE`,
`XDG_STATE_HOME/gdbg`, or `~/.local/state/gdbg` in that order.

The default game launch is headless. Console, script, input, and process control
work in that mode; screenshot and recording require `gdbg run game --windowed`.
