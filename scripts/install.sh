#!/usr/bin/env bash
# Install the latest Goop release binary from GitHub.
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/Macho0x/Goop/main/scripts/install.sh | bash
#   GOOP_INSTALL_DIR=/usr/local/bin ./scripts/install.sh
set -euo pipefail

REPO="${GOOP_REPO:-Macho0x/Goop}"
INSTALL_DIR="${GOOP_INSTALL_DIR:-${HOME}/.local/bin}"

os="$(uname -s | tr '[:upper:]' '[:lower:]')"
arch="$(uname -m)"
case "$arch" in
  x86_64|amd64) arch="amd64" ;;
  aarch64|arm64) arch="arm64" ;;
  *)
    echo "error: unsupported architecture: $arch" >&2
    exit 1
    ;;
esac

case "$os" in
  linux)
    asset="goop-linux-amd64"
    if [[ "$arch" != "amd64" ]]; then
      echo "error: Linux releases currently ship amd64 only (got $arch)" >&2
      exit 1
    fi
    ;;
  darwin)
    asset="goop-darwin-${arch}"
    ;;
  mingw*|msys*|cygwin*|windows)
    echo "error: use the .exe from GitHub Releases on Windows" >&2
    exit 1
    ;;
  *)
    echo "error: unsupported OS: $os" >&2
    exit 1
    ;;
esac

api="https://api.github.com/repos/${REPO}/releases/latest"
echo "→ resolving latest release for ${REPO}…"
json="$(curl -fsSL "$api")"
tag="$(printf '%s' "$json" | sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' | head -1)"
url="$(printf '%s' "$json" | sed -n "s|.*\"browser_download_url\": *\"\\([^\"]*/${asset}\\)\"|\\1|p" | head -1)"

if [[ -z "$url" ]]; then
  echo "error: could not find asset ${asset} in latest release ${tag:-?}" >&2
  exit 1
fi

mkdir -p "$INSTALL_DIR"
tmp="$(mktemp)"
trap 'rm -f "$tmp"' EXIT
echo "→ downloading ${url}"
curl -fsSL -o "$tmp" "$url"
chmod +x "$tmp"
dest="${INSTALL_DIR}/goop"
mv "$tmp" "$dest"
trap - EXIT

echo "installed ${tag} → ${dest}"
if ! command -v goop >/dev/null 2>&1; then
  echo "note: add ${INSTALL_DIR} to your PATH, e.g.:"
  echo "  export PATH=\"${INSTALL_DIR}:\$PATH\""
fi
"$dest" version || true
