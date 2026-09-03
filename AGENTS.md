# updash — AGENTS.md

## Build & run

```sh
go build -o updash ./cmd/updash/   # single binary
make check                          # build + --check
make install                        # copies to $HOME/.local/bin/updash
./scripts/validate.sh               # full local gates (build/fmt/test/cover/lint/gosec)
```

Entry modes in `cmd/updash/main.go`:

| Mode | Flag | Notes |
|------|------|--------|
| TUI | (default) | Bubble Tea — Updates / Cleanup / Logs |
| check | `--check` | headless scan; add `--json` for machines |
| update | `--update` | outdated only |
| clean | `--clean` | cleanup candidates |
| all | `--all` | update + clean |
| upgrade | `--upgrade` | GitHub release self-update |
| update-self | `--update-self` | git pull + rebuild (dev) |

## Architecture

```
platform/detect.go   → OS + available package managers (+ HasOpenCode)
scanner/             → Source interface + RunAll() (parallel, max 6 workers)
  ├── brew, apt, mas, winget, … package managers
  ├── opencode.go    → npm outdated --prefix ~/.config/opencode
  ├── agents.go      → AI CLIs + npm global outdated merge
  ├── npm_protected.go → packages owned by another path (excluded from npm batch)
  ├── homelab_clean  → retention cleanups (env-driven)
  └── cleanup.go     → brew/docker/npm/go caches, SDKMAN majors, pacman/yay caches + orphans
updater/             → batch updates (brew/mas/npm/opencode plugins/agents)
  ├── runner.go      → exec seam (outputRunner/runUpdateCmd/lookPath) for tests
  └── opencode_health.go → post-update `opencode --version` validation
cleaner/             → cleanOne + policy.go (age paths, truncate, disk pressure)
cli/                 → headless + JSON report (gate package ≥90% coverage)
tui/                 → Bubble Tea async scan/update/clean
config/env.go        → UPDASH_* retention (wired in cleaner + scanners)
upgrade/             → release self-update on startup
model/types.go       → Item, Category, Status, PlatformInfo
```

## Adding a new package manager source

1. Create `internal/scanner/<name>.go` implementing `Source`
2. Register in `enabledSources()` (`scanner.go`)
3. Add update logic in `internal/updater/updater.go`
4. Add cleanup in `internal/cleaner` if needed
5. Unit-test pure parse helpers; mock `execCombined` / `execCommand` for I/O

## Shell install/staging

`install.sh` and `internal/upgrade` share one staging discipline: stage under an
unpredictable name created inside the destination directory (`mktemp` /
`os.CreateTemp` with `O_EXCL`), apply the destination's existing permissions
with the exec bit forced on (`dest_mode` / `currentBinaryMode`, never a blanket
`0755` over a `0700` install), then atomic rename. A fixed staging path is an
arbitrary-write primitive under `curl | sudo bash`. Regression harnesses:
`scripts/install_test.sh` and `internal/upgrade/hardening_test.go`.

`install.sh` can be sourced with `UPDASH_INSTALL_LIB=1` to call a single
function without running an install (test seam used by `scripts/install_test.sh`).

## Coverage / CI gates

- **COVER_PKGS (≥90%)**: `./internal/model/... ./internal/config/... ./internal/sizefmt/... ./internal/cli/... ./internal/retention/... ./internal/upgrade/...`
- **TEST_IO_PKGS** (race, no floor): scanner, tui, cleaner
- Sonar excludes the same I/O packages from coverage measurement
- Keep complexity low (Sonar S3776); prefer small helpers over large switches
- gosec excludes: `G204,G306,G703,G118`

## Cross-platform quirks

| OS | PMs | Notes |
|---|---|---|
| macOS | brew, mas | brew cask exclusion list in `brew.go` |
| Linux | apt, dnf/yum, zypper, pacman/yay, flatpak, snap | `sudo` for apt/dnf/zypper/pacman/snap; apk when root or sudo |
| Windows | winget, choco, scoop | TEMP cleanup |

- Agent version probes skip Electron CLIs without `DISPLAY` on Linux
- Agent catalog is data-driven (`agentDef` in `agents.go`): add one entry to add an agent; auto agents need `updateCmd` or `npmPackage`
- Agents not installed via global npm get their latest version from the npm registry (`npm view`), so native/brew/pnpm/bun installs are flagged
- semidx (`semidx upgrade --check`), gh extensions and gcloud components are flagged outdated when updates exist — `--update` reaches them headless
- nvm/omz/SDKMAN updates need `bash`; hosts without it get a manual note instead of a failing command
- Manual-only agents use `KeepPolicy` containing `manual` → skipped in CLI update
- OpenCode binary: `opencode upgrade` (single owner); plugins: `npm install --prefix ~/.config/opencode <pkg>@<version>…` (explicit versions — `npm update` cannot move exactly-pinned plugins, e.g. `@opencode-ai/plugin`)
- **OpenCode single-owner / protected npm packages:** the OpenCode binary is updated ONLY by `opencode upgrade` (the agent path). Its npm dist packages (`opencode-ai`, `@opencode-ai/cli`) are in `scanner.ProtectedNpmPackages()` (data-driven, not grep) and are (a) dropped from `NpmSource.Scan` and (b) excluded from the generic `npm update -g` batch in `batchNpmUpgrade`, which targets the remaining outdated globals by explicit name. The OpenCode agent item carries `npmPackage: "opencode-ai"` purely so the existing outdated-detection machinery flags it (its `updateCmd` still wins for the upgrade). After `opencode upgrade`, `ensureOpenCodeHealthy` runs `opencode --version` + checks the launcher exists/executable; a broken stub fails explicitly with the reinstall hint.

## Retention policy (env)

`updash --env-defaults` prints effective values. Defaults:

- Docker ages: `336h` (14d)
- Docker builder mode: `age` (use `UPDASH_DOCKER_BUILDER_MODE=all` on CI/homelab)
- Container log truncate: `50` MB
- Host logs / AI outputs / dev caches: age in days (30 / 7 / 90)
- Disk pressure: prune aggressively when used% ≥ 85
- Project build cleanup (`dev-cache:projects-builds`) is **opt-in**: it is only
  scanned/cleaned with `UPDASH_DEV_PROJECTS_CLEAN=1` (also `true`/`on`/`yes`).
  Root dir: `UPDASH_DEV_PROJECTS_DIR` (fallback `~/Projetos` → `~/Projects`).
  Build dirs (`target`/`build`/`dist`/`.next`/`out`/`.turbo`/`__pycache__`/`.pytest_cache`)
  are always collected; `node_modules` only when the project shows no activity
  newer than `UPDASH_DEV_CACHE_MAX_DAYS` (newest mtime of first-level entries).
  Walk never follows symlinks, never collects the projects root itself, stays
  under the canonicalized root, and reports per-project walk/stat failures as
  partial errors (item `Unverified` when nothing trustworthy was collected).

Homelab clean category: `homelab-clean` (`--only homelab-clean`).

## Self-update trust model (`internal/upgrade`)

Fail-closed; every step must succeed or the installed binary is untouched.
Full table in [docs/DISTRIBUTION.md](./docs/DISTRIBUTION.md).

- TLS 1.2+, system trust store, verification never disabled. `UPDASH_TLS_CA_CERT`
  only *adds* a CA. `checkRedirect` bounds the chain to `maxRedirects` (5) and
  refuses an `https` → plaintext downgrade.
- `httpGetLimited` enforces a per-request ceiling on both the declared
  `Content-Length` and the streamed body. Budgets are the `max*Bytes` vars
  (API 8 MiB, checksums 1 MiB, archive 64 MiB, decompressed 192 MiB) — they are
  vars so tests shrink them instead of building huge fixtures.
- `findChecksum` returns an error for a malformed (non-64-hex) or ambiguous
  digest; a missing entry is an empty result that `downloadReleaseBinary`
  rejects. There is no "install anyway" path.
- `safeArchiveMemberName` rejects absolute paths, Windows volume prefixes, and
  `..` components. Only `tar.TypeReg`/regular zip entries are candidates, so
  symlink/hardlink members can never be installed. `readArchiveMember` shares
  one decompression budget across all members.
- `pickReleaseBinary` requires an ELF/Mach-O/PE magic number even when the
  member is literally named `updash` — a script cannot impersonate the release.
- `replaceRunningBinaryWithOS` refuses an empty payload, stages via
  `os.CreateTemp` (`O_EXCL`, unpredictable name) in the destination directory,
  `Sync`s, applies `currentBinaryMode` (existing perms, exec bit forced), and
  atomically renames. Windows uses the `<name>.old` staging fallback.
- `canSelfUpdate`/`selfUpdateAllowed` resolve symlinks first, then allow only
  `~/.local/bin` (plus documented Windows user dirs). `UPDASH_ALLOW_SELF_UPDATE=1`
  is the only override.

## Release chain

`ci.yml` (all gates green on `main`) → dispatch `autotag.yml` with the tested
SHA → tag → dispatch `release.yml`. `release.yml` re-validates the tag shape,
verifies the tag commit is contained in `origin/main`, **exports that SHA and
pins every downstream checkout to it** (a tag can be force-moved during the
build window), and refuses to overwrite
a release that already published `checksums.txt` unless dispatched with
`force_rerelease=true`. Workflow tokens default to `contents: read`; only the
publishing job elevates. Workflow inputs reach shell steps through `env:`,
never `${{ }}` interpolation inside `run`.

**Builder mode note:** `age` + `until=` frequently reclaims 0B on active build hosts. Prefer `all` there; keep shell `builder prune -af` as belt-and-suspenders if desired.

## Repository

- Public: `github.com/lgldsilva/updash`
- **No direct pushes to `main`** — feature branches + PRs
