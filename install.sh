#!/usr/bin/env bash
set -euo pipefail

REPO="https://github.com/geniobot/mini-agent/archive/refs/heads/main.tar.gz"
BINARY="mini-agent"
VERSION="v2.6.1"
INSTALL_DIR="${PREFIX:-$HOME/.local/bin}"

# ── colors (disabled when NO_COLOR is set or stdout is not a terminal) ────────
if [ -z "${NO_COLOR:-}" ] && [ "${TERM:-dumb}" != "dumb" ]; then
  CYN='\033[96m'
  BLU='\033[94m'
  TEL='\033[36m'
  GRN='\033[92m'
  DIM='\033[2m'
  RST='\033[0m'
else
  CYN='' BLU='' TEL='' GRN='' DIM='' RST=''
fi

p() { printf "%b\n" "$@"; }

# ── dependency check ──────────────────────────────────────────────────────────
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

printf "${DIM}Downloading...${RST} "
curl -fsSL "$REPO" -o "$TMP/src.tar.gz"
tar -xz -C "$TMP" -f "$TMP/src.tar.gz"
printf "${GRN}done${RST}\n"

SRCDIR=$(find "$TMP" -maxdepth 1 -mindepth 1 -type d | head -1)
if [ -z "$SRCDIR" ] || [ ! -f "$SRCDIR/go.mod" ]; then
  echo "error: could not find go.mod in extracted archive" >&2
  exit 1
fi

# ── build ─────────────────────────────────────────────────────────────────────
printf "${DIM}Building...${RST}    "
(cd "$SRCDIR" && go build -ldflags "-s -w" -o "$TMP/$BINARY" ./cmd/$BINARY)
printf "${GRN}done${RST}\n"

# ── install ───────────────────────────────────────────────────────────────────
printf "${DIM}Installing...${RST}  "
mkdir -p "$INSTALL_DIR"
install -m755 "$TMP/$BINARY" "$INSTALL_DIR/$BINARY"
printf "${GRN}done${RST}\n"

# ── config ────────────────────────────────────────────────────────────────────
CONFIG_DIR="$HOME/.mini-agent"
CONFIG_FILE="$CONFIG_DIR/config.yaml"
CONFIG_URL="https://raw.githubusercontent.com/geniobot/mini-agent/main/config.yaml"
printf "${DIM}Config...${RST}      "
mkdir -p "$CONFIG_DIR"
if [ ! -f "$CONFIG_FILE" ]; then
  curl -fsSL "$CONFIG_URL" -o "$CONFIG_FILE"
  printf "${GRN}done${RST}\n"
else
  printf "${DIM}kept existing${RST}\n"
fi

# ── banner ────────────────────────────────────────────────────────────────────
SEP="${DIM}  ──────────────────────────────────────────────────${RST}"

echo ""
p "${CYN}  ███╗   ███╗██╗███╗   ██╗██╗${RST}"
p "${CYN}  ████╗ ████║██║████╗  ██║██║${RST}"
p "${CYN}  ██╔████╔██║██║██╔██╗ ██║██║${RST}"
p "${CYN}  ██║╚██╔╝██║██║██║╚██╗██║██║${RST}"
p "${CYN}  ██║ ╚═╝ ██║██║██║ ╚████║██║${RST}"
p "${CYN}  ╚═╝     ╚═╝╚═╝╚═╝  ╚═══╝╚═╝${RST}"
p "${BLU}   █████╗  ██████╗ ███████╗███╗   ██╗████████╗${RST}"
p "${BLU}  ██╔══██╗██╔════╝ ██╔════╝████╗  ██║╚══██╔══╝${RST}"
p "${BLU}  ███████║██║  ███╗█████╗  ██╔██╗ ██║   ██║${RST}"
p "${BLU}  ██╔══██║██║   ██║██╔══╝  ██║╚██╗██║   ██║${RST}"
p "${BLU}  ██║  ██║╚██████╔╝███████╗██║ ╚████║   ██║${RST}"
p "${BLU}  ╚═╝  ╚═╝ ╚═════╝ ╚══════╝╚═╝  ╚═══╝   ╚═╝${RST}"
echo ""
p "${DIM}  local · lightweight · fast                      ${VERSION}${RST}"
p "$SEP"
p "  ${GRN}✓${RST}  installed ${TEL}mini-agent ${VERSION}${RST}"
p "     ${DIM}→ $INSTALL_DIR/$BINARY${RST}"
p "$SEP"
echo ""
p "  ${TEL}◆${RST}  Getting started"
echo ""
p "     ${DIM}1. Pull a model (if you haven't)${RST}"
p "        ollama pull qwen2.5-coder:1.5b"
echo ""
p "     ${DIM}2. Run${RST}"
p "        mini-agent"
echo ""
p "$SEP"
p "  ${TEL}◆${RST}  Useful commands inside mini-agent"
echo ""
p "     ${DIM}/run <goal>${RST}   run a task autonomously"
p "     ${DIM}/model <name>${RST} switch model without restarting"
p "     ${DIM}/status${RST}       show token usage and config"
p "     ${DIM}/help${RST}         list all commands"
p "     ${DIM}/exit${RST}         quit"
echo ""
p "$SEP"
p "  ${TEL}◆${RST}  Uninstall"
echo ""
p "     rm $INSTALL_DIR/$BINARY"
p "     rm -rf ~/.mini-agent    ${DIM}# config + session history${RST}"
echo ""
p "$SEP"
echo ""

# ── PATH check ────────────────────────────────────────────────────────────────
case ":$PATH:" in
  *":$INSTALL_DIR:"*) ;;
  *)
    p "  ${GRN}!${RST}  ${DIM}$INSTALL_DIR is not in your PATH.${RST}"
    p "     Add this to ${DIM}~/.bashrc${RST} or ${DIM}~/.profile${RST} and restart your shell:"
    echo ""
    p "     export PATH=\"\$HOME/.local/bin:\$PATH\""
    echo ""
    ;;
esac
