#!/usr/bin/env bash
# Regression harness for install.sh's staging step.
#
# The installer stages the new binary inside $INSTALL_DIR before the atomic
# rename. A predictable staging path is an arbitrary-write primitive: with
# `curl | sudo bash` and INSTALL_DIR=/usr/local/bin, a pre-planted symlink at
# that path would be written through, chmod'ed, and then moved into place as
# the installed "binary". This is the shell twin of the Go-side
# TestReplaceRunningBinaryWithOS_doesNotFollowStagedSymlink.
set -uo pipefail

root=$(cd "$(dirname "$0")/.." && pwd)
work=$(mktemp -d "${TMPDIR:-/tmp}/updash-install-test.XXXXXX")
trap 'rm -rf "$work"' EXIT

FAIL=0
pass() { printf '  ✓ %s\n' "$1"; }
# mode_of echoes the octal permissions of a path (BSD stat, then GNU stat).
mode_of() { stat -f '%Lp' "$1" 2>/dev/null || stat -c '%a' "$1" 2>/dev/null; }
fail() { printf '  ✘ %s\n' "$1" >&2; FAIL=1; }

# run_install <install_dir> <src> — calls install_binary in a subshell.
run_install() {
  (
    set +e
    # shellcheck disable=SC2030 # the subshell is exactly the isolation wanted
    export UPDASH_INSTALL_LIB=1 INSTALL_DIR="$1"
    local src="$2"
    # Clear positional parameters: install.sh parses "$@" when sourced.
    set --
    # shellcheck disable=SC1090 # sourced by absolute path built at runtime
    . "$root/install.sh"
    install_binary "$src" "test" >/dev/null 2>&1
  )
}

# ── 1. pre-planted staging symlink must not be followed ───────────────────
dir="$work/case1"; mkdir -p "$dir/bin"
victim="$dir/victim"
printf 'untouched' >"$victim"
chmod 0600 "$victim"
printf 'new-binary' >"$work/src1"
ln -s "$victim" "$dir/bin/.updash.install.tmp"

run_install "$dir/bin" "$work/src1"

if [ "$(cat "$victim")" = "untouched" ]; then
  pass "staging did not write through the planted symlink"
else
  fail "victim file was overwritten through the staging symlink"
fi
victim_mode=$(mode_of "$victim")
if [ "$victim_mode" = "600" ]; then
  pass "staging did not chmod the symlink target"
else
  fail "victim permissions were changed to $victim_mode"
fi
if [ -e "$dir/bin/updash" ] && [ -L "$dir/bin/updash" ]; then
  fail "installed path is a symlink pointing outside the install dir"
else
  pass "installed path is not a symlink"
fi
if [ -f "$dir/bin/updash" ] && [ "$(cat "$dir/bin/updash")" != "new-binary" ]; then
  fail "installed file has unexpected content"
fi

# ── 2. an existing install must not have its permissions widened ──────────
dir="$work/case2"; mkdir -p "$dir/bin"
printf 'old' >"$dir/bin/updash"
chmod 0700 "$dir/bin/updash"
printf 'new-binary' >"$work/src2"

if ! run_install "$dir/bin" "$work/src2"; then
  fail "install into an existing 0700 destination failed"
fi
mode=$(mode_of "$dir/bin/updash")
if [ "$mode" = "700" ]; then
  pass "existing 0700 permissions preserved"
else
  fail "permissions widened to $mode"
fi
if [ "$(cat "$dir/bin/updash")" != "new-binary" ]; then
  fail "existing destination was not replaced"
fi

# ── 3. a fresh install is executable (0755) ───────────────────────────────
dir="$work/case3"; mkdir -p "$dir/bin"
printf 'new-binary' >"$work/src3"
if ! run_install "$dir/bin" "$work/src3"; then
  fail "fresh install failed"
fi
mode=$(mode_of "$dir/bin/updash")
if [ "$mode" = "755" ]; then
  pass "fresh install is 0755"
else
  fail "fresh install mode is $mode, want 755"
fi

# ── 4. no staging debris is left behind ───────────────────────────────────
debris=$(find "$work" -name '.updash.install*' -not -type l 2>/dev/null)
if [ -z "$debris" ]; then
  pass "no staging debris left behind"
else
  fail "staging debris left: $debris"
fi

# ── 5. a failed copy must not destroy the current install ─────────────────
dir="$work/case5"; mkdir -p "$dir/bin"
printf 'still-here' >"$dir/bin/updash"
chmod 0755 "$dir/bin/updash"
run_install "$dir/bin" "$work/does-not-exist"
if [ "$(cat "$dir/bin/updash")" = "still-here" ]; then
  pass "a failed install leaves the current binary intact"
else
  fail "current binary was damaged by a failed install"
fi

# ── 6. checksum lookup treats the filename as a literal ───────────────────
# run_lib <fn> <args...> — call one install.sh function in a subshell.
run_lib() {
  (
    set +e
    # shellcheck disable=SC2031 # each call runs in its own isolated subshell
    export UPDASH_INSTALL_LIB=1
    local fn="$1"; shift
    local args=("$@")
    set --
    # shellcheck disable=SC1090 # sourced by absolute path built at runtime
    . "$root/install.sh"
    "$fn" "${args[@]}"
  )
}

manifest="$work/checksums.txt"
good=$(printf 'a%.0s' $(seq 1 64))
other=$(printf 'b%.0s' $(seq 1 64))
{
  printf '%s  updash_1.0.0_linux_amd64.tar.gz
' "$good"
  # A dot treated as a regex wildcard would let this line match the entry above.
  printf '%s  updash_1.0.0_linux_amd64Xtar.gz
' "$other"
  printf 'not-a-digest  updash_9.9.9_linux_amd64.tar.gz
'
} >"$manifest"

got=$(run_lib checksum_for "$manifest" "updash_1.0.0_linux_amd64.tar.gz")
if [ "$got" = "$good" ]; then
  pass "checksum lookup matches the filename literally"
else
  fail "checksum lookup returned '$got', want the exact entry"
fi

if run_lib checksum_for "$manifest" "updash_9.9.9_linux_amd64.tar.gz" >/dev/null 2>&1; then
  fail "a malformed digest was accepted"
else
  pass "a malformed digest is rejected"
fi

printf '%s  updash_2.0.0_linux_amd64.tar.gz
%s  updash_2.0.0_linux_amd64.tar.gz
'   "$good" "$other" >"$work/conflict.txt"
if run_lib checksum_for "$work/conflict.txt" "updash_2.0.0_linux_amd64.tar.gz" >/dev/null 2>&1; then
  fail "conflicting checksum entries were accepted"
else
  pass "conflicting checksum entries are rejected"
fi

if [ "$FAIL" -eq 0 ]; then
  echo "install.sh staging is safe"
else
  echo "install.sh staging regressions detected" >&2
fi
exit "$FAIL"
