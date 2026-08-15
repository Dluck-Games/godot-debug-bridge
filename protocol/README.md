# GDBG protocol

The CLI and runtime addon currently implement protocol version 1. The protocol
is intentionally small and transport-specific; compatibility changes are
documented in versioned files in this directory.

- [Version 1](v1.md) — file IPC, command/result envelopes, and bridge-owned
  operations shipped in GDBG 0.1.0.

New clients add a numeric `protocol` field to JSON commands. The 0.1 runtime
accepts a missing field as version 1 so pre-versioned local clients remain
compatible. Unsupported explicit versions fail with a structured error.
