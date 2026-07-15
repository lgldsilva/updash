#!/usr/bin/env bash
# validate.sh — Full local validation gate (8 gates + Sonar preview opcional).
# Exit code: 0 = all green, 1 = something failed.
# Run manually:  ./scripts/validate.sh
# As git hook:  ln -sf ../../scripts/validate.sh .git/hooks/pre-push
#
# Sonar preview: opcional. Pula se SONAR_TOKEN não estiver setado.
#   SONAR_TOKEN=sqa_xxx ./scripts/validate.sh
#   ou export SONAR_TOKEN=sqa_xxx; ./scripts/validate.sh
#
# Se o gate falhar, veja os logs:
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

# ── 8 gates obrigatórios ──────────────────────────────────────────────────

step "Build"           go build ./...
step "Format"          bash -c 'gofmt -l . | test ! -s /dev/stdin'
step "Vet"             go vet ./...
step "Tests (race)"    go test -race -count=1 -coverprofile=/tmp/updash-cov.out \
                         ./internal/model/... ./internal/scanner/... \
                         ./internal/tui/... ./internal/cleaner/... \
                         ./internal/config/... ./internal/sizefmt/... 2>&1 | tail -1
step "Coverage ≥70%"   bash -c 'pct=$(go tool cover -func=/tmp/updash-cov.out | awk '\''/^total:/ {gsub("%","",$3); print $3}'\''); awk -v p="$pct" -v min=70 "BEGIN{ exit (p+0 < min+0) }"'
step "Lint (0 issues)" go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2 run ./...
step "gosec"           go run github.com/securego/gosec/v2/cmd/gosec@v2.27.1 -quiet -exclude=G204,G306,G703,G118 ./...
step "Vulncheck"       bash -c 'go run golang.org/x/vuln/cmd/govulncheck@latest ./... 2>&1 | grep -q "Your code is affected by 0 vulnerabilities"'

echo ""

# ── Sonar preview (opcional — requer SONAR_TOKEN) ─────────────────────────

if [ -n "${SONAR_TOKEN:-}" ]; then
  echo "  SonarQube preview (ephemeral project)..."
  # Same CE workaround as CI: temp project → report → delete.
  if PROJECT_KEY="updash-preview-$(date +%s)" \
     PROJECT_NAME="updash-preview" \
     EPHEMERAL=1 \
     COVERAGE_FILE=/tmp/updash-cov.out \
     REPORT_FILE=/tmp/updash-sonar-report.txt \
     ./scripts/sonar-ephemeral.sh &>/tmp/updash-sonar.log; then
    echo -e "  SonarQube gate              ${GREEN}✓ PASSED${NC}"
  else
    echo -e "  SonarQube gate              ${RED}✘ FAILED${NC}"
    grep -E 'quality_gate|status:|✘|✓' /tmp/updash-sonar.log | tail -20 \
      || tail -15 /tmp/updash-sonar.log
    FAIL=1
  fi
else
  echo "  SonarQube gate              ${GRAY}SKIPPED (set SONAR_TOKEN)${NC}"
fi

echo ""
if [ "$FAIL" -eq 0 ]; then
  echo -e "${GREEN}✅ ALL GATES PASSED${NC}"
else
  echo -e "${RED}❌ SOME GATES FAILED — check /tmp/updash-validate.log${NC}"
fi
exit "$FAIL"
