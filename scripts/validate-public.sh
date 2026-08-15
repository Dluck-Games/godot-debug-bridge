#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${repo_root}"

required=(
  LICENSE
  README.md
  addons/gdbg/LICENSE
  addons/gdbg/README.md
  addons/gdbg/plugin.cfg
  addons/gdbg/plugin.gd
  addons/gdbg/ai_debug_bridge.gd
  addons/gdbg/console_bridge_host.gd
  addons/gdbg/console/console_registry.gd
  addons/gdbg/testing/test_host.gd
  addons/gdbg/testing/integration_runner.tscn
  cli/go.mod
  cli/main.go
  protocol/v1.md
  examples/minimal/project.godot
  scripts/install.sh
  scripts/install.ps1
  scripts/package.sh
  scripts/smoke-minimal.sh
)

for path in "${required[@]}"; do
  test -f "${path}" || { echo "missing required file: ${path}" >&2; exit 1; }
done

if rg -n -i '\bGOL\b|GOL[A-Za-z_]|God of Lego|gol-tools|gol-project|GOL_|addons/debug_bridge|GDAI|SceneWire|SceneDriver|/Users/' \
  . --glob '!.git/**' --glob '!scripts/validate-public.sh'; then
  echo "public repository contains project-specific or local-only content" >&2
  exit 1
fi

unformatted=$(gofmt -l $(rg --files cli -g '*.go'))
if [[ -n "${unformatted}" ]]; then
  echo "unformatted Go files:" >&2
  echo "${unformatted}" >&2
  exit 1
fi

(cd cli && go test ./... && go vet ./...)
