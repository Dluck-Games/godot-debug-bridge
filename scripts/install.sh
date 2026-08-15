#!/bin/sh
set -eu

repository="Dluck-Games/godot-debug-bridge"
requested_version="${GDBG_VERSION:-latest}"
install_dir="${GDBG_INSTALL_DIR:-${HOME}/.local/bin}"

if [ "${requested_version}" = "latest" ]; then
  requested_version=$(curl -fsSL -H 'Accept: application/vnd.github+json' \
    "https://api.github.com/repos/${repository}/releases/latest" \
    | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' \
    | head -n 1)
fi
version=${requested_version#v}
if [ -z "${version}" ]; then
  echo "Could not resolve a GDBG release version." >&2
  exit 1
fi

case "$(uname -s)" in
  Darwin) platform=darwin ;;
  Linux) platform=linux ;;
  *) echo "Unsupported operating system: $(uname -s)" >&2; exit 1 ;;
esac

case "$(uname -m)" in
  arm64|aarch64) architecture=arm64 ;;
  x86_64|amd64) architecture=amd64 ;;
  *) echo "Unsupported architecture: $(uname -m)" >&2; exit 1 ;;
esac

asset="gdbg-v${version}-${platform}-${architecture}.tar.gz"
base_url="https://github.com/${repository}/releases/download/v${version}"
install_tmp=$(mktemp -d)
trap 'rm -rf "${install_tmp}"' EXIT HUP INT TERM

curl -fsSL "${base_url}/${asset}" -o "${install_tmp}/${asset}"
curl -fsSL "${base_url}/checksums.txt" -o "${install_tmp}/checksums.txt"
expected=$(awk -v name="${asset}" '$2 == name {print $1}' "${install_tmp}/checksums.txt")
if [ -z "${expected}" ]; then
  echo "No checksum published for ${asset}." >&2
  exit 1
fi
if command -v sha256sum >/dev/null 2>&1; then
  actual=$(sha256sum "${install_tmp}/${asset}" | awk '{print $1}')
else
  actual=$(shasum -a 256 "${install_tmp}/${asset}" | awk '{print $1}')
fi
if [ "${actual}" != "${expected}" ]; then
  echo "Checksum verification failed for ${asset}." >&2
  exit 1
fi

tar -xzf "${install_tmp}/${asset}" -C "${install_tmp}"
mkdir -p "${install_dir}"
install -m 0755 "${install_tmp}/gdbg" "${install_dir}/gdbg"

echo "Installed gdbg v${version} to ${install_dir}/gdbg"
case ":${PATH}:" in
  *":${install_dir}:"*) ;;
  *) echo "Add ${install_dir} to PATH to run gdbg." ;;
esac
