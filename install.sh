#!/usr/bin/env bash
# install.sh — Install updash from source OR a prebuilt GitHub release.
#
# Default (no args): builds from source using the local Go toolchain.
# Pass `binary` to download a prebuilt release with sha256 verification.
#
# Usage:
#   ./install.sh                  # build from source
#   ./install.sh binary           # install latest prebuilt binary from GitHub
#   INSTALL_DIR=/opt/bin ./install.sh binary
set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
INSTALL_DIR="${INSTALL_DIR:-$HOME/.local/bin}"
GITHUB_REPO="${GITHUB_REPO:-lgldsilva/updash}"

# ── Cross-platform helpers ───────────────────────────────────────────────

# Pick sha256 binary: macOS ships shasum, Linux ships sha256sum.
sha256_of() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$1" | awk '{print $1}'
  else
    echo "✘ Neither sha256sum nor shasum found" >&2
    return 1
  fi
}

# Pick uname-style target OS/arch.
detect_target() {
  local goos goarch
  goos=$(uname -s | tr '[:upper:]' '[:lower:]')
  case "$goos" in
    darwin|linux|windows) ;;
    *) echo "✘ Unsupported OS: $goos" >&2; return 1 ;;
  esac
  goarch=$(uname -m)
  case "$goarch" in
    x86_64|amd64)   goarch=amd64 ;;
    aarch64|arm64)  goarch=arm64 ;;
    *) echo "✘ Unsupported architecture: $goarch" >&2; return 1 ;;
  esac
  echo "$goos $goarch"
}

# ── Binary install (prebuilt from GitHub) ─────────────────────────────────

install_from_github_release() {
  echo "📥 Downloading updash from GitHub release..."

  local api_url="https://api.github.com/repos/$GITHUB_REPO"
  local version
  if command -v gh >/dev/null 2>&1; then
    version=$(gh release view --repo "$GITHUB_REPO" --json tagName --jq '.tagName' 2>/dev/null || true)
  fi
  if [ -z "${version:-}" ]; then
    version=$(curl -fsSL "$api_url/releases/latest" | sed -nE 's/.*"tag_name":[[:space:]]*"([^"]+)".*/\1/p' | head -1)
  fi
  if [ -z "${version:-}" ]; then
    echo "✘ Could not determine latest release version" >&2
    exit 1
  fi
  echo "Latest version: $version"

  local goos goarch
  read -r goos goarch < <(detect_target)

  local clean_version="${version#v}"
  local archive_ext="tar.gz"
  local extract_cmd
  if [ "$goos" = "windows" ]; then
    archive_ext="zip"
    extract_cmd="unzip -qo"
  else
    extract_cmd="tar -xzf"
  fi
  local archive_name="updash_${clean_version}_${goos}_${goarch}.${archive_ext}"
  local download_url="https://github.com/$GITHUB_REPO/releases/download/$version/$archive_name"
  local checksums_url="https://github.com/$GITHUB_REPO/releases/download/$version/checksums.txt"

  local tmp_dir
  tmp_dir=$(mktemp -d)
  trap 'rm -rf "$tmp_dir"' EXIT

  echo "Downloading $archive_name..."
  if ! curl -fSL -o "$tmp_dir/$archive_name" "$download_url"; then
    echo "✘ Failed to download $archive_name from $download_url" >&2
    exit 1
  fi

  echo "Downloading checksums.txt..."
  if ! curl -fSL -o "$tmp_dir/checksums.txt" "$checksums_url"; then
    echo "✘ Failed to download checksums.txt from $checksums_url" >&2
    exit 1
  fi

  echo "Verifying SHA256 checksum..."
  local expected actual
  expected=$(grep -F "  $archive_name" "$tmp_dir/checksums.txt" | awk '{print $1}')
  if [ -z "$expected" ]; then
    echo "✘ No checksum entry for $archive_name in checksums.txt" >&2
    exit 1
  fi
  actual=$(sha256_of "$tmp_dir/$archive_name")
  if [ "$expected" != "$actual" ]; then
    echo "✘ Checksum mismatch:" >&2
    echo "  expected: $expected" >&2
    echo "  actual:   $actual" >&2
    exit 1
  fi
  echo "✓ Checksum verified"

  echo "Extracting..."
  ( cd "$tmp_dir" && $extract_cmd "$archive_name" )

  echo "Installing to $INSTALL_DIR/updash..."
  mkdir -p "$INSTALL_DIR"
  install -m 0755 "$tmp_dir/updash" "$INSTALL_DIR/updash"
  if [ -x "$tmp_dir/updash.exe" ]; then
    install -m 0755 "$tmp_dir/updash.exe" "$INSTALL_DIR/updash.exe"
  fi

  echo "✓ Installed to $INSTALL_DIR/updash"
  echo ""
  echo "Make sure $INSTALL_DIR is in your PATH."
}

# ── Source install (default) ──────────────────────────────────────────────

install_from_source() {
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

  echo "🔨 Building updash from source..."
  cd "$REPO_DIR"
  go build -o updash ./cmd/updash/

  mkdir -p "$INSTALL_DIR"
  install -m 0755 updash "$INSTALL_DIR/updash"
  echo "✓ Installed to $INSTALL_DIR/updash"
  echo ""
  echo "Make sure $INSTALL_DIR is in your PATH."
  echo "  Run:  updash           # TUI dashboard"
  echo "  Run:  updash --check   # Headless scan"
  echo "  Run:  updash --all     # Update everything"
}

# ── Entry point ──────────────────────────────────────────────────────────

case "${1:-}" in
  binary) install_from_github_release ;;
  ""|source) install_from_source ;;
  -h|--help)
    cat <<EOF
install.sh — install updash

Usage:
  ./install.sh                  build from source (default)
  ./install.sh source           build from source
  ./install.sh binary           download prebuilt release from GitHub
  ./install.sh --help           show this help

Environment:
  INSTALL_DIR       target directory (default: \$HOME/.local/bin)
  GITHUB_REPO       source GitHub repo (default: lgldsilva/updash)
EOF
    ;;
  *) echo "✘ Unknown subcommand: $1 (try --help)" >&2; exit 1 ;;
esac
