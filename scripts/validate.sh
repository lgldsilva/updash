#!/usr/bin/env bash
# validate.sh — Full local validation gate (8 gates).
# Exit code: 0 = all green, 1 = something failed.
# Run manually:  ./scripts/validate.sh
#
# If a gate fails, see the logs:
#   cat /tmp/updash-validate.log

set -uo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
GRAY='\033[0;90m'
NC='\033[0m'
FAIL=0

step() {
  local label="$1"; shift
  printf "  %-30s" "$label"
  if "$@" &>/tmp/updash-validate.log; then
    echo -e "${GREEN}✓${NC}"
  else
    echo -e "${RED}✘${NC}"
    FAIL=1
  fi
}

echo "🔍 Full validation — $(date)"
echo ""

# ── 8 required gates ──────────────────────────────────────────────────────

step "Build"           go build ./...
step "Format"          bash -c 'gofmt -l . | test ! -s /dev/stdin'
step "Vet"             go vet ./...
# I/O packages: race-tested, not folded into the 90% gate (see .github/workflows/ci.yml).
step "Tests I/O (race)" go test -race -count=1 \
                         ./internal/scanner/... ./internal/tui/... \
                         ./internal/cleaner/... 2>&1 | tail -1
step "Tests gate+cover" go test -race -count=1 -coverprofile=/tmp/updash-cov.out \
                         ./internal/model/... ./internal/config/... \
                         ./internal/sizefmt/... ./internal/cli/... \
                         ./internal/retention/... 2>&1 | tail -1
step "Coverage ≥90%"   bash -c 'pct=$(go tool cover -func=/tmp/updash-cov.out | awk '\''/^total:/ {gsub("%","",$3); print $3}'\''); echo "  coverage=${pct}%"; awk -v p="$pct" -v min=90 "BEGIN{ exit (p+0 < min+0) }"'
step "Lint (0 issues)" go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2 run ./...
step "gosec"           go run github.com/securego/gosec/v2/cmd/gosec@v2.27.1 -quiet -exclude=G204,G306,G703,G118 ./...
step "Vulncheck"       bash -c 'go run golang.org/x/vuln/cmd/govulncheck@latest ./... 2>&1 | grep -q "Your code is affected by 0 vulnerabilities"'

echo ""
if [ "$FAIL" -eq 0 ]; then
  echo -e "${GREEN}✅ ALL GATES PASSED${NC}"
else
  echo -e "${RED}❌ SOME GATES FAILED — check /tmp/updash-validate.log${NC}"
fi
exit "$FAIL"
