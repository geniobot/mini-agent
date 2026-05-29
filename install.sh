#!/usr/bin/env bash
set -euo pipefail

REPO="https://github.com/geniobot/mini-agent/archive/refs/heads/main.tar.gz"
BINARY="mini-agent"
INSTALL_DIR="${PREFIX:-/usr/local}/bin"

# ── dependency check ────────────────────────────────────────────────────────
if ! command -v go &>/dev/null; then
  echo "error: Go 1.22+ is required — https://go.dev/dl/" >&2
  exit 1
fi

if ! command -v curl &>/dev/null; then
  echo "error: curl is required" >&2
  exit 1
fi

# ── download ─────────────────────────────────────────────────────────────────
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

printf 'Downloading... '
curl -fsSL "$REPO" | tar -xz -C "$TMP"
echo "done"

# ── build ────────────────────────────────────────────────────────────────────
printf 'Building...    '
cd "$TMP/mini-agent-main"
go build -ldflags "-s -w" -o "$TMP/$BINARY" ./cmd/$BINARY
echo "done"

# ── install ──────────────────────────────────────────────────────────────────
printf "Installing to %s... " "$INSTALL_DIR"
mkdir -p "$INSTALL_DIR"
if [ -w "$INSTALL_DIR" ]; then
  install -m755 "$TMP/$BINARY" "$INSTALL_DIR/$BINARY"
else
  sudo install -m755 "$TMP/$BINARY" "$INSTALL_DIR/$BINARY"
fi
echo "done"

echo ""
echo "  ✓  mini-agent installed"
echo "     run: mini-agent"
echo ""
echo "  First time? Set up a config:"
echo "     mkdir -p ~/.mini-agent"
echo "     curl -fsSL https://raw.githubusercontent.com/geniobot/mini-agent/main/config.yaml -o ~/.mini-agent/config.yaml"
echo ""
