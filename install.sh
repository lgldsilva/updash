#!/usr/bin/env bash
# install.sh — Install updash from GitHub releases, or build from a local checkout.
#
# Default mode: download the latest prebuilt release for your OS/arch from
#   https://github.com/lgldsilva/updash/releases/latest, verify SHA-256
#   against checksums.txt, install to $INSTALL_DIR.
#
# Source mode (--from-source): build from the local checkout the script
#   lives in. Requires `go` in PATH (auto-installed if missing).
#
# Either way, the binary lands at $INSTALL_DIR/updash (default ~/.local/bin).
set -euo pipefail

REPO="lgldsilva/updash"
GITHUB_API="https://api.github.com/repos/${REPO}/releases/latest"
GITHUB_DL="https://github.com/${REPO}/releases/download"

INSTALL_DIR="${INSTALL_DIR:-$HOME/.local/bin}"

usage() {
  cat <<'EOF'
updash installer — install the System Update Dashboard from GitHub.

Usage: install.sh [options]

Options:
  --from-source           Build from a local checkout instead of downloading
                          a release binary. The script must live inside the
                          git working tree (go.mod + cmd/updash/ present).
  --version vX.Y.Z        Install a specific release (default: latest).
  --help                  Show this help and exit.

Environment:
  INSTALL_DIR=...                Target bin directory (default: ~/.local/bin).
  UPDASH_VERSION=...             Same as --version.
  UPDASH_INSTALL_FROM_SOURCE=1   Same as --from-source.

Examples:
  ./install.sh                                 # latest release for this OS/arch
  UPDASH_VERSION=v0.6.1 ./install.sh           # pin a version
  ./install.sh --from-source                   # build from current checkout
  curl -fsSL https://raw.githubusercontent.com/lgldsilva/updash/main/install.sh | bash
EOF
}

log()  { printf '%s\n' "$*"; }
warn() { printf '⚠ %s\n' "$*" >&2; }
die()  { printf '✘ %s\n' "$*" >&2; exit 1; }

_UPDASH_TMP=""
_cleanup_tmp() { [ -n "$_UPDASH_TMP" ] && [ -d "$_UPDASH_TMP" ] && rm -rf "$_UPDASH_TMP"; }

# ── Arg parsing ────────────────────────────────────────────────────────────
MODE="binary"
PIN_VERSION=""
while [ $# -gt 0 ]; do
  case "$1" in
    --from-source) MODE="source" ;;
    --version)     PIN_VERSION="${2:-}"; [ -n "$PIN_VERSION" ] || die "--version requires a value"; shift ;;
    --version=*)   PIN_VERSION="${1#*=}" ;;
    -h|--help)     usage; exit 0 ;;
    *)             die "unknown argument: $1 (use --help)" ;;
  esac
  shift
done
[ -n "${UPDASH_VERSION:-}" ] && [ -z "$PIN_VERSION" ] && PIN_VERSION="$UPDASH_VERSION"
[ "${UPDASH_INSTALL_FROM_SOURCE:-}" = "1" ] && MODE="source"

# ── Helpers ───────────────────────────────────────────────────────────────
require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    die "required command '$1' not found in PATH"
  fi
}

sha256_file() {
  local f="$1"
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$f" | awk '{print $1}'
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$f" | awk '{print $1}'
  elif command -v openssl >/dev/null 2>&1; then
    openssl dgst -sha256 "$f" | awk '{print $NF}'
  else
    die "no SHA-256 tool found (need sha256sum, shasum, or openssl)"
  fi
}

detect_platform() {
  local uos umach
  uos="$(uname -s)"
  umach="$(uname -m)"

  case "$uos" in
    Darwin) GOOS="darwin" ;;
    Linux)  GOOS="linux" ;;
    *)      die "unsupported OS: $uos (supported: darwin, linux)" ;;
  esac

  case "$umach" in
    x86_64|amd64)   GOARCH="amd64" ;;
    aarch64|arm64)  GOARCH="arm64" ;;
    *)              die "unsupported arch: $umach (supported: amd64, arm64)" ;;
  esac

  EXT="tar.gz"
}

install_go() {
  if command -v go >/dev/null 2>&1; then return 0; fi
  log "→ Go not found; installing via system package manager…"
  case "$(uname -s)" in
    Darwin)
      command -v brew >/dev/null 2>&1 || die "brew not found; install Go from https://go.dev/dl/"
      brew install go
      ;;
    Linux)
      if command -v apt-get >/dev/null 2>&1; then
        sudo apt-get update && sudo apt-get install -y golang-go
      elif command -v pacman >/dev/null 2>&1; then
        sudo pacman -S --noconfirm go
      elif command -v dnf >/dev/null 2>&1; then
        sudo dnf install -y golang
      else
        die "install Go manually: https://go.dev/dl/"
      fi
      ;;
  esac
  command -v go >/dev/null 2>&1 || die "Go install did not produce a 'go' binary in PATH"
}

# ── Mode: source (build from local checkout) ──────────────────────────────
install_from_source() {
  local repo_dir
  repo_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

  if [ ! -f "$repo_dir/go.mod" ] || [ ! -d "$repo_dir/cmd/updash" ]; then
    die "--from-source needs a local checkout (go.mod + cmd/updash/ not found at $repo_dir)"
  fi

  log "→ Building from source: $repo_dir"
  install_go

  ( cd "$repo_dir" && go build -o updash ./cmd/updash/ )
  install_binary "$repo_dir/updash" "source build"
}

# ── Mode: binary (download release from GitHub) ───────────────────────────
install_from_release() {
  detect_platform
  require_cmd curl

  local tag
  if [ -n "$PIN_VERSION" ]; then
    tag="$PIN_VERSION"
  else
    log "→ Querying latest release from GitHub…"
    local body
    body="$(curl -fsSL "$GITHUB_API")" || die "failed to fetch $GITHUB_API"
    tag="$(printf '%s' "$body" \
      | grep -o '"tag_name"[[:space:]]*:[[:space:]]*"[^"]*"' \
      | head -n1 \
      | sed -E 's/.*"([^"]+)".*/\1/')"
    [ -n "$tag" ] || die "could not parse tag_name from GitHub release"
  fi

  local ver="${tag#v}"
  local archive="updash_${ver}_${GOOS}_${GOARCH}.${EXT}"
  local url_archive="${GITHUB_DL}/${tag}/${archive}"
  local url_checksums="${GITHUB_DL}/${tag}/checksums.txt"

  local tmp
  _UPDASH_TMP="$(mktemp -d -t updash.XXXXXX)" || die "mktemp failed"
  local tmp="$_UPDASH_TMP"
  trap _cleanup_tmp EXIT

  log "→ Downloading $archive ($tag) for ${GOOS}/${GOARCH}…"
  curl -fsSL -o "$tmp/$archive" "$url_archive" \
    || die "download failed: $url_archive"
  curl -fsSL -o "$tmp/checksums.txt" "$url_checksums" \
    || die "download failed: $url_checksums"

  local expected actual
  expected="$(grep -E "  ${archive}\$" "$tmp/checksums.txt" | awk '{print $1}' | head -n1)"
  [ -n "$expected" ] || die "no checksum entry found for $archive in checksums.txt"
  actual="$(sha256_file "$tmp/$archive")"
  if [ "$expected" != "$actual" ]; then
    die "sha256 mismatch for $archive: expected $expected, got $actual"
  fi
  log "✓ sha256 verified"

  case "$EXT" in
    tar.gz) require_cmd tar;   tar -xzf "$tmp/$archive" -C "$tmp" ;;
    zip)    require_cmd unzip; unzip -o -q "$tmp/$archive" -d "$tmp" ;;
  esac
  local bin="updash"
  [ -f "$tmp/${bin}.exe" ] && bin="${bin}.exe"
  [ -f "$tmp/$bin" ] || die "binary 'updash' not found in archive"

  install_binary "$tmp/$bin" "$tag"
}

# ── Common install step ───────────────────────────────────────────────────
install_binary() {
  local src="$1" label="$2"
  mkdir -p "$INSTALL_DIR" || die "could not create $INSTALL_DIR"

  local stage="$INSTALL_DIR/.updash.install.tmp"
  cp "$src" "$stage" || die "copy to $stage failed"
  chmod 0755 "$stage"   || die "chmod $stage failed"
  mv "$stage" "$INSTALL_DIR/updash" || die "install to $INSTALL_DIR/updash failed"

  log ""
  log "✓ Installed updash ($label) → $INSTALL_DIR/updash"

  if ! command -v updash >/dev/null 2>&1; then
    log ""
    log "  Note: $INSTALL_DIR is not in your PATH."
    log "  Add it (zsh):   echo 'export PATH=\"$INSTALL_DIR:\$PATH\"' >> ~/.zshrc"
    log "  Add it (bash):  echo 'export PATH=\"$INSTALL_DIR:\$PATH\"' >> ~/.bashrc"
  fi
  log ""
  log "  Try:  updash version    # shows build/arch"
  log "        updash --check    # headless scan"
  log ""
  warn "Note: internal/upgrade defaults still point at the old Gitea host."
  warn "      Run with UPDASH_SKIP_AUTO_UPGRADE=1 to silence the startup"
  warn "      upgrade-check until that migration lands."
}

# ── Dispatch ──────────────────────────────────────────────────────────────
case "$MODE" in
  source) install_from_source ;;
  binary) install_from_release ;;
  *)      die "internal error: bad MODE=$MODE" ;;
esac
