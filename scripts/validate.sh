#!/usr/bin/env bash
# validate.sh — full local validation gate. Every gate keeps its own log and
# its real exit status controls this script's final status.
set -uo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
NC='\033[0m'
FAIL=0
GATE_NO=0
LOG_DIR="${UPDASH_VALIDATE_LOG_DIR:-}"
if [ -z "$LOG_DIR" ]; then
  LOG_DIR=$(mktemp -d "${TMPDIR:-/tmp}/updash-validate.XXXXXX")
fi
mkdir -p "$LOG_DIR"

step() {
  local label="$1"
  shift
  local log
  GATE_NO=$((GATE_NO + 1))
  log=$(printf '%s/gate-%02d-%s.log' "$LOG_DIR" "$GATE_NO" "$(tr '[:upper:] ' '[:lower:]-' <<<"$label" | tr -cd '[:alnum:]-')")
  printf '  %-30s' "$label"
  if "$@" >"$log" 2>&1; then
    printf '%b\n' "${GREEN}✓${NC}"
  else
    printf '%b  %s\n' "${RED}✘${NC}" "$log"
    FAIL=1
  fi
}

echo "🔍 Full validation — $(date)"
echo "   logs: $LOG_DIR"
echo

step "Build" go build ./...
step "Cross-build" bash -o pipefail -c 'for t in darwin/amd64 darwin/arm64 windows/amd64 windows/arm64 linux/arm64; do GOOS=${t%/*} GOARCH=${t#*/} go build ./... || exit 1; done'
step "Format" bash -o pipefail -c 'gofmt -l . | test ! -s /dev/stdin'
step "Vet" go vet ./...
step "Tests I/O (race)" go test -race -count=1 ./internal/scanner/... ./internal/tui/... ./internal/cleaner/... ./internal/updater/... ./internal/elevate/... ./internal/platform/...
step "Tests gate+cover" go test -race -count=1 -coverprofile="$LOG_DIR/coverage.out" ./internal/model/... ./internal/config/... ./internal/sizefmt/... ./internal/cli/... ./internal/retention/... ./internal/upgrade/...
step "Coverage >=90%" bash -o pipefail -c 'pct=$(go tool cover -func="$1" | awk '\''/^total:/ {gsub("%","",$3); print $3}'\''); test -n "$pct"; printf "coverage=%s%%\n" "$pct"; awk -v p="$pct" -v min="${COVERAGE_MIN:-90}" "BEGIN { exit (p+0 < min+0) }"' _ "$LOG_DIR/coverage.out"
step "Lint (0 issues)" bash -o pipefail -c 'GOTOOLCHAIN=go1.26.6 go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2 run ./...'
step "gosec" bash -o pipefail -c 'GOTOOLCHAIN=go1.26.6 go run github.com/securego/gosec/v2/cmd/gosec@v2.27.1 -quiet -exclude=G204,G306,G703,G118 ./...'
step "Shell/CI regressions" bash -o pipefail -c 'bash "$(dirname "$0")/install_test.sh" && bash "$(dirname "$0")/release_chain_test.sh"' "$0"
step "Vulncheck" bash -o pipefail -c 'GOTOOLCHAIN=go1.26.6 go run golang.org/x/vuln/cmd/govulncheck@latest ./...'

echo
if [ "$FAIL" -eq 0 ]; then
  printf '%b\n' "${GREEN}✅ ALL GATES PASSED${NC}"
else
  printf '%b\n' "${RED}❌ SOME GATES FAILED — inspect $LOG_DIR${NC}"
fi
exit "$FAIL"
