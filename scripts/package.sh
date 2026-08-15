#!/usr/bin/env bash
set -euo pipefail

version="${1#v}"
commit="${2:-none}"
build_date="${3:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}"
script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "${script_dir}/.." && pwd)"
dist_dir="${4:-${repo_root}/dist}"
build_tmp="$(mktemp -d)"
trap 'rm -rf "${build_tmp}"' EXIT

if [[ -z "${version}" ]]; then
  echo "usage: scripts/package.sh <version> [commit] [date] [dist-dir]" >&2
  exit 2
fi

mkdir -p "${dist_dir}"

targets=(
  darwin/amd64
  darwin/arm64
  windows/amd64
  windows/arm64
  linux/amd64
  linux/arm64
)

for target in "${targets[@]}"; do
  goos="${target%/*}"
  goarch="${target#*/}"
  package_dir="${build_tmp}/${goos}-${goarch}"
  mkdir -p "${package_dir}"
  binary="gdbg"
  if [[ "${goos}" == "windows" ]]; then
    binary="gdbg.exe"
  fi
  (
    cd "${repo_root}/cli"
    CGO_ENABLED=0 GOOS="${goos}" GOARCH="${goarch}" go build \
      -trimpath \
      -ldflags "-s -w -X main.version=v${version} -X main.commit=${commit} -X main.date=${build_date}" \
      -o "${package_dir}/${binary}" .
  )
  cp "${repo_root}/LICENSE" "${package_dir}/LICENSE"
  archive_base="gdbg-v${version}-${goos}-${goarch}"
  if [[ "${goos}" == "windows" ]]; then
    archive="${dist_dir}/${archive_base}.zip"
    rm -f "${archive}"
    (cd "${package_dir}" && zip -q "${archive}" "${binary}" LICENSE)
  else
    archive="${dist_dir}/${archive_base}.tar.gz"
    tar -czf "${archive}" -C "${package_dir}" "${binary}" LICENSE
  fi
done

addon_archive="${dist_dir}/gdbg-addon-v${version}.zip"
rm -f "${addon_archive}"
(cd "${repo_root}" && zip -q -r "${addon_archive}" addons/gdbg -x '*.DS_Store')

(
  cd "${dist_dir}"
  rm -f checksums.txt
  for artifact in gdbg-*; do
    if command -v sha256sum >/dev/null 2>&1; then
      sha256sum "${artifact}"
    else
      shasum -a 256 "${artifact}"
    fi
  done | LC_ALL=C sort > checksums.txt
)

echo "Packaged GDBG v${version} in ${dist_dir}"
