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

# Fail-closed transport for every download: https only (no plaintext even
# after a redirect), TLS 1.2+, bounded redirects, and a size ceiling so a
# hostile or broken host cannot stream an unbounded body onto the disk.
CURL_SECURE_OPTS=(
  --proto "=https" --proto-redir "=https" --tlsv1.2 --location
  --max-redirs 5 --max-time 300 --retry 2 --fail --silent --show-error
)
CURL_MAX_ARCHIVE_BYTES=67108864   # 64 MiB
CURL_MAX_API_BYTES=8388608        # 8 MiB
CURL_MAX_CHECKSUM_BYTES=1048576   # 1 MiB
CURL_MAX_EXTRACTED_BYTES=201326592 # 192 MiB

INSTALL_DIR="${INSTALL_DIR:-$HOME/.local/bin}"

usage() {
  cat <<'EOF'
updash installer — install the System Update Dashboard from GitHub.

Usage: install.sh [binary|options]

Options:
  --from-source           Build from a local checkout instead of downloading
                          a release binary. The script must live inside the
                          git working tree (go.mod + cmd/updash/ present).
  --version vX.Y.Z        Install a specific release (default: latest).
  --help                  Show this help and exit.

The optional positional argument "binary" selects the default release-binary
mode explicitly. "--from-source" selects source mode.

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
_UPDASH_STAGE=""
# One cleanup entry point: install_binary registers its own trap, and a second
# EXIT trap would otherwise replace (not add to) the download-tempdir cleanup.
_cleanup_tmp() {
  [ -n "$_UPDASH_STAGE" ] && rm -f "$_UPDASH_STAGE"
  _UPDASH_STAGE=""
  [ -n "$_UPDASH_TMP" ] && [ -d "$_UPDASH_TMP" ] && rm -rf "$_UPDASH_TMP"
  return 0
}
_cleanup_stage() { [ -n "$_UPDASH_STAGE" ] && rm -f "$_UPDASH_STAGE"; _UPDASH_STAGE=""; return 0; }

# ── Arg parsing ────────────────────────────────────────────────────────────
MODE="binary"
PIN_VERSION=""
while [ $# -gt 0 ]; do
  case "$1" in
    binary)         MODE="binary" ;;
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
    body="$(curl "${CURL_SECURE_OPTS[@]}" --max-filesize "$CURL_MAX_API_BYTES" "$GITHUB_API")" \
      || die "failed to fetch $GITHUB_API"
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

  _UPDASH_TMP="$(mktemp -d -t updash.XXXXXX)" || die "mktemp failed"
  local tmp="$_UPDASH_TMP"
  trap _cleanup_tmp EXIT

  log "→ Downloading $archive ($tag) for ${GOOS}/${GOARCH}…"
  download_limited "$url_archive" "$tmp/$archive" "$CURL_MAX_ARCHIVE_BYTES" \
    || die "download failed: $url_archive"
  download_limited "$url_checksums" "$tmp/checksums.txt" "$CURL_MAX_CHECKSUM_BYTES" \
    || die "download failed: $url_checksums"

  local expected actual
  expected="$(checksum_for "$tmp/checksums.txt" "$archive")" || die "$expected"
  actual="$(sha256_file "$tmp/$archive" | tr 'A-F' 'a-f')"
  if [ "$expected" != "$actual" ]; then
    die "sha256 mismatch for $archive: expected $expected, got $actual"
  fi
  log "✓ sha256 verified"

  case "$EXT" in
    tar.gz) require_cmd tar; validate_tar_members "$tmp/$archive" ;;
    zip)    require_cmd unzip; validate_zip_members "$tmp/$archive" ;;
  esac
  local bin="updash"
  if ! archive_member_exists "$tmp/$archive" "$EXT" "$bin"; then
    bin="${bin}.exe"
  fi
  archive_member_exists "$tmp/$archive" "$EXT" "$bin" || die "binary 'updash' not found in archive"
  extract_archive_member "$tmp/$archive" "$EXT" "$bin" "$tmp/$bin" \
    || die "could not extract binary 'updash' from archive"
  require_executable_image "$tmp/$bin"

  install_binary "$tmp/$bin" "$tag"
}

# checksum_for <manifest> <filename> — echoes the single lowercase SHA-256
# published for <filename>, or exits non-zero with a message on stdout.
# The filename is compared as a literal field, never interpolated into a
# regex: the dots in "updash_1.2.3_linux_amd64.tar.gz" would otherwise be
# wildcards and could match a different entry.
checksum_for() {
  local manifest="$1" want="$2" entries count status
  if entries="$(awk -v want="$want" '
    $2 == want {
      found=1
      if (length($1) != 64 || $1 !~ /^[0-9a-fA-F]+$/) {
        bad=1
      } else {
        print tolower($1)
      }
    }
    END {
      if (bad) exit 2
      if (!found) exit 3
    }
  ' "$manifest")"; then
    status=0
  else
    status=$?
  fi
  case "$status" in
    2)
      printf 'malformed sha256 entry for %s in checksums.txt' "$want"
      return 1
      ;;
    3)
      printf 'no sha256 entry found for %s in checksums.txt' "$want"
      return 1
      ;;
  esac
  if [ -z "$entries" ]; then
    printf 'no sha256 entry found for %s in checksums.txt' "$want"
    return 1
  fi
  count="$(printf '%s\n' "$entries" | wc -l | tr -d ' ')"
  if [ "$count" != "1" ]; then
    printf 'conflicting checksum entries for %s in checksums.txt' "$want"
    return 1
  fi
  printf '%s' "$entries"
}

# download_limited <url> <destination> <max-bytes> — cap the streamed body,
# including responses without a Content-Length header. The extra byte lets us
# distinguish an exactly-at-limit response from one that exceeded the budget.
download_limited() {
  local url="$1" dest="$2" limit="$3" status size
  require_cmd head
  rm -f "$dest"
  set +e
  curl "${CURL_SECURE_OPTS[@]}" --max-filesize "$limit" "$url" \
    | head -c "$((limit + 1))" >"$dest"
  status=$?
  set -e
  size="$(wc -c <"$dest" | tr -d ' ')"
  if [ "$size" -gt "$limit" ]; then
    rm -f "$dest"
    printf 'response exceeds %s-byte limit: %s' "$limit" "$url" >&2
    return 1
  fi
  if [ "$status" -ne 0 ]; then
    rm -f "$dest"
    return "$status"
  fi
}

validate_archive_member_name() {
  local member="$1" normalized part
  [ -n "$member" ] || die "archive contains an unnamed member"
  case "$member" in
    /*|\\*|[A-Za-z]:*) die "absolute path in archive: $member" ;;
  esac
  normalized="${member//\\//}"
  IFS='/' read -r -a parts <<<"$normalized"
  for part in "${parts[@]}"; do
    [ "$part" != ".." ] || die "path traversal in archive: $member"
  done
}

validate_tar_members() {
  local archive="$1" names listing line type
  names="$(tar -tzf "$archive")" || die "could not list tar archive"
  while IFS= read -r line; do
    [ -n "$line" ] || continue
    validate_archive_member_name "$line"
  done <<<"$names"

  listing="$(tar -tvzf "$archive")" || die "could not inspect tar archive members"
  while IFS= read -r line; do
    [ -n "$line" ] || continue
    type="${line:0:1}"
    case "$type" in
      -|d) ;;
      *) die "non-regular archive member is not allowed: $line" ;;
    esac
  done <<<"$listing"
}

validate_zip_members() {
  local archive="$1" names listing
  names="$(unzip -Z1 "$archive")" || die "could not list zip archive"
  while IFS= read -r line; do
    [ -n "$line" ] || continue
    validate_archive_member_name "$line"
  done <<<"$names"

  listing="$(unzip -Z -l "$archive")" || die "could not inspect zip archive members"
  if ! awk '
    $1 ~ /^[dl-][rwx-]{9}$/ {
      seen=1
      type=substr($1, 1, 1)
      if (type != "-" && type != "d") exit 1
    }
    END { if (!seen) exit 2 }
  ' <<<"$listing"; then
    die "zip archive contains an unsupported or unrecognized member type"
  fi
}

archive_member_exists() {
  local archive="$1" ext="$2" member="$3"
  case "$ext" in
    tar.gz) tar -tzf "$archive" | grep -Fx -- "$member" >/dev/null ;;
    zip)    unzip -Z1 "$archive" | grep -Fx -- "$member" >/dev/null ;;
    *)      return 1 ;;
  esac
}

extract_archive_member() {
  local archive="$1" ext="$2" member="$3" dest="$4" status size
  require_cmd head
  rm -f "$dest"
  set +e
  case "$ext" in
    tar.gz) tar -xOzf "$archive" "$member" | head -c "$((CURL_MAX_EXTRACTED_BYTES + 1))" >"$dest" ;;
    zip)    unzip -p "$archive" "$member" | head -c "$((CURL_MAX_EXTRACTED_BYTES + 1))" >"$dest" ;;
    *)      set -e; return 1 ;;
  esac
  status=$?
  set -e
  size="$(wc -c <"$dest" | tr -d ' ')"
  if [ "$size" -gt "$CURL_MAX_EXTRACTED_BYTES" ]; then
    rm -f "$dest"
    printf 'archive member exceeds %s-byte limit: %s' "$CURL_MAX_EXTRACTED_BYTES" "$member" >&2
    return 1
  fi
  if [ "$status" -ne 0 ]; then
    rm -f "$dest"
    return "$status"
  fi
}

require_executable_image() {
  local file="$1" magic
  require_cmd od
  magic="$(od -An -tx1 -N4 "$file" | tr -d '[:space:]' | tr 'A-F' 'a-f')"
  case "$magic" in
    7f454c46|4d5a*|feedface|feedfacf|cefaedfe|cffaedfe|cafebabe|bebafeca|cafebabf|bfbafeca) ;;
    *) die "archive payload is not an ELF, Mach-O, or PE executable" ;;
  esac
}

# ── Common install step ───────────────────────────────────────────────────
# dest_mode echoes the permissions to install with: the current mode of the
# destination with the owner-execute bit forced on, or 0755 for a fresh
# install. A destination deliberately restricted to 0700 must not be widened.
dest_mode() {
  local dest="$1" mode=""
  [ -f "$dest" ] || { printf '0755'; return 0; }
  if mode="$(stat -f '%Lp' "$dest" 2>/dev/null)" && [ -n "$mode" ]; then
    :
  elif mode="$(stat -c '%a' "$dest" 2>/dev/null)" && [ -n "$mode" ]; then
    :
  else
    printf '0755'
    return 0
  fi
  # Pad to four digits and force u+x; the installed file must be runnable.
  printf '0%03o' "$(( 8#$mode | 0100 ))"
}

install_binary() {
  local src="$1" label="$2"
  mkdir -p "$INSTALL_DIR" || die "could not create $INSTALL_DIR"

  local dest="$INSTALL_DIR/updash"
  local mode
  mode="$(dest_mode "$dest")"

  # Stage under an unpredictable name created by mktemp (which fails rather
  # than reusing an existing path). A fixed name such as
  # ".updash.install.tmp" is an arbitrary-write primitive: a pre-planted
  # symlink there would be written through by cp, chmod'ed, and then moved
  # into place as the "installed binary" — as root under `curl | sudo bash`.
  local stage
  stage="$(mktemp "$INSTALL_DIR/.updash.install.XXXXXX")" || die "could not stage in $INSTALL_DIR"
  _UPDASH_STAGE="$stage"
  trap _cleanup_tmp EXIT INT TERM

  cat "$src" >"$stage" 2>/dev/null || { _cleanup_stage; die "copy of $src failed"; }
  chmod "$mode" "$stage" || { _cleanup_stage; die "chmod $stage failed"; }
  mv "$stage" "$dest" || { _cleanup_stage; die "install to $dest failed"; }
  _UPDASH_STAGE=""

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
}

# ── Dispatch ──────────────────────────────────────────────────────────────
# UPDASH_INSTALL_LIB=1 lets scripts/install_test.sh source this file and call a
# single function without performing an install (same idea as the exec seams in
# the Go packages).
if [ "${UPDASH_INSTALL_LIB:-}" = "1" ]; then
  # shellcheck disable=SC2317 # reached only when this file is executed, not sourced
  return 0 2>/dev/null || exit 0
fi

case "$MODE" in
  source) install_from_source ;;
  binary) install_from_release ;;
  *)      die "internal error: bad MODE=$MODE" ;;
esac
