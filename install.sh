#!/usr/bin/env bash
set -euo pipefail

REPO="https://github.com/geniobot/mini-agent/archive/refs/heads/main.tar.gz"
BINARY="mini-agent"
INSTALL_DIR="${PREFIX:-$HOME/.local/bin}"

# ── dependency check ─────────────────────────────────────────────────────────
if ! command -v go &>/dev/null; then
  echo "error: Go 1.22+ is required — https://go.dev/dl/" >&2
  exit 1
fi

if ! command -v curl &>/dev/null; then
  echo "error: curl is required" >&2
  exit 1
fi

# ── download ──────────────────────────────────────────────────────────────────
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

printf 'Downloading... '
curl -fsSL "$REPO" -o "$TMP/src.tar.gz"
tar -xz -C "$TMP" -f "$TMP/src.tar.gz"
echo "done"

# Find the extracted directory (works regardless of branch/tag naming)
SRCDIR=$(find "$TMP" -maxdepth 1 -mindepth 1 -type d | head -1)

if [ -z "$SRCDIR" ] || [ ! -f "$SRCDIR/go.mod" ]; then
  echo "error: could not find go.mod in extracted archive" >&2
  exit 1
fi

# ── build ─────────────────────────────────────────────────────────────────────
printf 'Building...    '
(cd "$SRCDIR" && go build -ldflags "-s -w" -o "$TMP/$BINARY" ./cmd/$BINARY)
echo "done"

# ── install ───────────────────────────────────────────────────────────────────
printf "Installing to %s... " "$INSTALL_DIR"
mkdir -p "$INSTALL_DIR"
install -m755 "$TMP/$BINARY" "$INSTALL_DIR/$BINARY"
echo "done"

# Warn if the install directory is not in PATH
case ":$PATH:" in
  *":$INSTALL_DIR:"*) ;;
  *) echo ""
     echo "  note: $INSTALL_DIR is not in your PATH."
     echo "  Add this to ~/.bashrc or ~/.profile:"
     echo "    export PATH=\"\$HOME/.local/bin:\$PATH\""
     ;;
esac

echo ""
echo "  mini-agent installed. Run: mini-agent"
echo ""
echo "  First time? Set up a config:"
echo "    mkdir -p ~/.mini-agent"
echo "    curl -fsSL https://raw.githubusercontent.com/geniobot/mini-agent/main/config.yaml \\"
echo "         -o ~/.mini-agent/config.yaml"
echo ""
