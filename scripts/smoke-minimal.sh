#!/usr/bin/env bash
set -euo pipefail

repo_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
godot_bin=${GODOT_BIN:-godot}
godot_bin=$(command -v "$godot_bin")
export GODOT_PATH="$godot_bin"
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

scene_output=$(PATH="$(dirname "$godot_bin"):$PATH" GDBG_STATE="$state_dir" \
  "$cli_bin" --project-dir "$fixture_dir" test scene res://tests/scene_smoke.tscn -- --gdbg-scene-smoke 2>&1)
grep -F "GDBG_SCENE_TEST_OK" <<<"$scene_output"

PATH="$(dirname "$godot_bin"):$PATH" GDBG_STATE="$state_dir" \
  "$cli_bin" --project-dir "$fixture_dir" run game --detach
test "$(PATH="$(dirname "$godot_bin"):$PATH" GDBG_STATE="$state_dir" \
  "$cli_bin" --project-dir "$fixture_dir" console ping | tail -n 1)" = "pong"
PATH="$(dirname "$godot_bin"):$PATH" GDBG_STATE="$state_dir" \
  "$cli_bin" --project-dir "$fixture_dir" console help ping | grep -F "Check that the game console is responsive"
if headless_capture_output=$(PATH="$(dirname "$godot_bin"):$PATH" GDBG_STATE="$state_dir" \
  "$cli_bin" --project-dir "$fixture_dir" debug screenshot 2>&1); then
  echo "headless screenshot unexpectedly succeeded" >&2
  exit 1
fi
grep -F "Screenshot capture requires a rendered display" <<<"$headless_capture_output"
if legacy_record_output=$(PATH="$(dirname "$godot_bin"):$PATH" GDBG_STATE="$state_dir" \
  "$cli_bin" --project-dir "$fixture_dir" debug record 2>&1); then
  echo "removed debug record unexpectedly succeeded" >&2
  exit 1
fi
grep -F "use 'gdbg record'" <<<"$legacy_record_output"
PATH="$(dirname "$godot_bin"):$PATH" GDBG_STATE="$state_dir" \
  "$cli_bin" --project-dir "$fixture_dir" stop

PATH="$(dirname "$godot_bin"):$PATH" GDBG_STATE="$state_dir" \
  "$cli_bin" --project-dir "$fixture_dir" test integration --suite smoke
PATH="$(dirname "$godot_bin"):$PATH" GDBG_STATE="$state_dir" \
  "$cli_bin" --project-dir "$fixture_dir" test playtest --suite smoke --headless

if [[ "$(uname -s)" == "Darwin" || -n "${DISPLAY:-}" || -n "${WAYLAND_DISPLAY:-}" ]]; then
  PATH="$(dirname "$godot_bin"):$PATH" GDBG_STATE="$state_dir" \
    "$cli_bin" --project-dir "$fixture_dir" record "$fixture_dir/tests/record_smoke.gd" \
    --fps 30 \
    --output "$state_dir/smoke_rec.mp4" \
    --manifest "$state_dir/smoke_manifest.json" \
    --contact-sheet "$state_dir/smoke_sheet.jpg"
  test -s "$state_dir/smoke_rec.mp4"
  test -s "$state_dir/smoke_sheet.jpg"
  grep -F '"total_frames": 20' "$state_dir/smoke_manifest.json"
  grep -F '"missing_draws": 0' "$state_dir/smoke_manifest.json"
fi

echo "GDBG minimal smoke passed"
