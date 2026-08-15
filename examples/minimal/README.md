# Minimal example

This project shows the smallest host-side console extension: a
`DebugBridgeConsoleHostBase` autoload plus `ping` and `echo` command modules. It
also includes a generic `DebugBridgeTestHostBase` adapter and one integration /
playtest smoke suite using opaque scenario dictionaries.

From this directory, with a source checkout of GDBG:

```sh
gdbg --project-dir . addon install --source ../../addons/gdbg
gdbg --project-dir . run game --detach
gdbg console ping
gdbg console echo hello from GDBG
gdbg --project-dir . stop game
gdbg --project-dir . test integration --suite smoke
gdbg --project-dir . test playtest --suite smoke --headless
```

The `addon install` command copies the addon, enables the plugin, and runs a
headless Godot import. Release users can omit `--source` to install the addon
matching their CLI release.
