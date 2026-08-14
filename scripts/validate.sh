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
# Platform-specific files (*_darwin.go, Windows paths) are otherwise never
# compiled on a Linux dev box — mirror the CI cross-build job.
step "Cross-build"     bash -c 'for t in darwin/amd64 darwin/arm64 windows/amd64 windows/arm64 linux/arm64; do GOOS=${t%/*} GOARCH=${t#*/} go build ./... || exit 1; done'
step "Format"          bash -c 'gofmt -l . | test ! -s /dev/stdin'
step "Vet"             go vet ./...
# I/O packages: race-tested, not folded into the 90% gate (see .github/workflows/ci.yml).
step "Tests I/O (race)" go test -race -count=1 \
                         ./internal/scanner/... ./internal/tui/... \
                         ./internal/cleaner/... ./internal/updater/... \
                         ./internal/elevate/... ./internal/platform/... 2>&1 | tail -1
step "Tests gate+cover" go test -race -count=1 -coverprofile=/tmp/updash-cov.out \
                         ./internal/model/... ./internal/config/... \
                         ./internal/sizefmt/... ./internal/cli/... \
                         ./internal/retention/... ./internal/upgrade/... 2>&1 | tail -1
step "Coverage ≥90%"   bash -c 'pct=$(go tool cover -func=/tmp/updash-cov.out | awk '\''/^total:/ {gsub("%","",$3); print $3}'\''); echo "  coverage=${pct}%"; awk -v p="$pct" -v min=90 "BEGIN{ exit (p+0 < min+0) }"'
step "Lint (0 issues)" bash -c 'GOTOOLCHAIN=go1.26.6 go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2 run ./...'
step "gosec"           bash -c 'GOTOOLCHAIN=go1.26.6 go run github.com/securego/gosec/v2/cmd/gosec@v2.27.1 -quiet -exclude=G204,G306,G703,G118 ./...'
step "Vulncheck"       bash -c 'GOTOOLCHAIN=go1.26.6 go run golang.org/x/vuln/cmd/govulncheck@latest ./... 2>&1 | grep -q "No vulnerabilities found"'

echo ""
if [ "$FAIL" -eq 0 ]; then
  echo -e "${GREEN}✅ ALL GATES PASSED${NC}"
else
  echo -e "${RED}❌ SOME GATES FAILED — check /tmp/updash-validate.log${NC}"
fi
exit "$FAIL"
