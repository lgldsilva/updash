#!/usr/bin/env bash
# Regression harness: a failed command on the left side of a pipeline must
# make validate.sh fail, rather than being hidden by tail's successful exit.
set -euo pipefail

root=$(cd "$(dirname "$0")/.." && pwd)
tmp=$(mktemp -d "${TMPDIR:-/tmp}/updash-validate-test.XXXXXX")
trap 'rm -rf "$tmp"' EXIT
mkdir -p "$tmp/bin" "$tmp/logs"
printf '%s\n' '#!/usr/bin/env bash' 'if [ "${1:-}" = test ]; then echo "diagnostic before failure"; echo "FAIL"; exit 1; fi' 'exit 0' >"$tmp/bin/go"
chmod +x "$tmp/bin/go"

set +e
PATH="$tmp/bin:$PATH" UPDASH_VALIDATE_LOG_DIR="$tmp/logs" bash "$root/scripts/validate.sh" >"$tmp/output" 2>&1
status=$?
set -e

if [ "$status" -ne 1 ]; then
  cat "$tmp/output"
  echo "validate.sh returned $status; expected 1" >&2
  exit 1
fi
if ! grep -q 'Tests I/O (race).*✘' "$tmp/output"; then
  cat "$tmp/output"
  echo "pipeline failure was not reported by Tests I/O gate" >&2
  exit 1
fi
io_log=$(find "$tmp/logs" -name '*tests-io-race*.log' -print -quit)
if [ -z "$io_log" ] || ! grep -q 'diagnostic before failure' "$io_log"; then
  cat "$tmp/output"
  if [ -n "$io_log" ]; then
    cat "$io_log"
  fi
  echo "gate log lost the failing command diagnostic" >&2
  exit 1
fi
echo "validate.sh propagates pipeline failures"
