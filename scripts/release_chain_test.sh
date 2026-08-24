#!/usr/bin/env bash
# Regression harness for the release workflow's supply-chain guards.
#
# The release must be built from the commit that was verified against
# origin/main, not from the tag name. Build + tests + SBOM take minutes, and a
# tag can be force-moved in that window (TOCTOU), so re-resolving the tag in a
# later job would publish artifacts from an unverified commit.
# The assertions below match GitHub Actions "${{ }}" expressions as literal
# text; the single quotes are deliberate.
# shellcheck disable=SC2016
set -uo pipefail

root=$(cd "$(dirname "$0")/.." && pwd)
wf="${1:-$root/.github/workflows/release.yml}"

FAIL=0
pass() { printf '  ✓ %s\n' "$1"; }
fail() { printf '  ✘ %s\n' "$1" >&2; FAIL=1; }

if grep -qF 'sha: ${{ steps.verify.outputs.sha }}' "$wf"; then
  pass "resolve-tag exports the verified commit SHA"
else
  fail "resolve-tag does not export a verified SHA output"
fi

if grep -qF 'merge-base --is-ancestor' "$wf"; then
  pass "the tag commit is checked for containment in origin/main"
else
  fail "no origin/main containment check found"
fi

if grep -qF 'ref: ${{ needs.resolve-tag.outputs.tag }}' "$wf"; then
  fail "a job still checks out the mutable tag name instead of the verified SHA"
else
  pass "no job checks out the mutable tag name"
fi

refs=$(grep -cF 'ref: ${{ needs.resolve-tag.outputs.sha }}' "$wf")
if [ "$refs" -ge 3 ]; then
  pass "build-test, sbom and release-artifacts all pin the verified SHA ($refs)"
else
  fail "only $refs checkout(s) pin the verified SHA, expected at least 3"
fi

if grep -qF "grep -qE '^v[0-9]+\\.[0-9]+\\.[0-9]+$'" "$wf"; then
  pass "the tag shape is validated with an anchored semver regex"
else
  fail "the tag shape check is not an anchored v<major>.<minor>.<patch> regex"
fi

if grep -qF 'GORELEASER_CURRENT_TAG' "$wf"; then
  pass "GoReleaser is given the tag explicitly (HEAD is a detached SHA)"
else
  fail "GoReleaser would have to re-derive the tag from a detached checkout"
fi

if awk '/^permissions:/{f=1;next} f&&/^  contents: read/{print;exit}' "$wf" | grep -q .; then
  pass "the default workflow token is contents: read"
else
  fail "the default workflow token is not contents: read"
fi

if [ "$FAIL" -eq 0 ]; then
  echo "release chain pins the verified commit"
else
  echo "release chain regressions detected" >&2
fi
exit "$FAIL"
