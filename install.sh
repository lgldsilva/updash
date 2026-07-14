#!/usr/bin/env bash
# install.sh — Build and install updash on any machine (macOS / Linux)
set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
INSTALL_DIR="${INSTALL_DIR:-$HOME/.local/bin}"

# ── Check Go ─────────────────────────────────────────────────────────────────
if ! command -v go &>/dev/null; then
  echo "✘ Go is not installed. Installing via brew/apt/pacman..."
  case "$(uname -s)" in
    Darwin) brew install go ;;
    Linux)
      if command -v apt-get &>/dev/null; then
        sudo apt-get update && sudo apt-get install -y golang-go
      elif command -v pacman &>/dev/null; then
        sudo pacman -S --noconfirm go
      else
        echo "✘ Please install Go manually: https://go.dev/dl/"
        exit 1
      fi
      ;;
  esac
fi

# ── Build ────────────────────────────────────────────────────────────────────
echo "🔨 Building updash..."
cd "$REPO_DIR"
go build -o updash ./cmd/updash/

# ── Install ──────────────────────────────────────────────────────────────────
mkdir -p "$INSTALL_DIR"
cp updash "$INSTALL_DIR/updash"
chmod +x "$INSTALL_DIR/updash"

echo "✓ Installed to $INSTALL_DIR/updash"
echo ""
echo "Make sure $INSTALL_DIR is in your PATH."
echo "  Run:  updash           # TUI dashboard"
echo "  Run:  updash --check   # Headless scan"
echo "  Run:  updash --all     # Update everything"
