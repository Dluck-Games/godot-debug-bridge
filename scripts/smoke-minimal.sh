#!/usr/bin/env bash
set -euo pipefail

repo_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
godot_bin=${GODOT_BIN:-godot}
fixture_dir=$(mktemp -d "${TMPDIR:-/tmp}/gdbg-minimal.XXXXXXXX")
state_dir=$(mktemp -d "${TMPDIR:-/tmp}/gdbg-state.XXXXXXXX")
cli_bin="$fixture_dir/gdbg"

cleanup() {
  GDBG_STATE="$state_dir" "$cli_bin" --project-dir "$fixture_dir" stop >/dev/null 2>&1 || true
  rm -rf "$fixture_dir" "$state_dir"
}
trap cleanup EXIT

(cd "$repo_dir/cli" && go build -o "$cli_bin" .)
cp -R "$repo_dir/examples/minimal/." "$fixture_dir"

PATH="$(dirname "$godot_bin"):$PATH" GDBG_STATE="$state_dir" \
  "$cli_bin" --project-dir "$fixture_dir" addon install \
  --source "$repo_dir/addons/gdbg"
PATH="$(dirname "$godot_bin"):$PATH" GDBG_STATE="$state_dir" \
  "$godot_bin" --headless --path "$fixture_dir" -- --gdbg-smoke

PATH="$(dirname "$godot_bin"):$PATH" GDBG_STATE="$state_dir" \
  "$cli_bin" --project-dir "$fixture_dir" run game --detach
test "$(PATH="$(dirname "$godot_bin"):$PATH" GDBG_STATE="$state_dir" \
  "$cli_bin" --project-dir "$fixture_dir" console ping | tail -n 1)" = "pong"
PATH="$(dirname "$godot_bin"):$PATH" GDBG_STATE="$state_dir" \
  "$cli_bin" --project-dir "$fixture_dir" console help ping | grep -F "Check that the game console is responsive"
PATH="$(dirname "$godot_bin"):$PATH" GDBG_STATE="$state_dir" \
  "$cli_bin" --project-dir "$fixture_dir" stop

PATH="$(dirname "$godot_bin"):$PATH" GDBG_STATE="$state_dir" \
  "$cli_bin" --project-dir "$fixture_dir" test integration --suite smoke
PATH="$(dirname "$godot_bin"):$PATH" GDBG_STATE="$state_dir" \
  "$cli_bin" --project-dir "$fixture_dir" test playtest --suite smoke --headless

echo "GDBG minimal smoke passed"
