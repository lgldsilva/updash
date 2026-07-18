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
| upgrade | `--upgrade` | Gitea release self-update |
| update-self | `--update-self` | git pull + rebuild (dev) |

## Architecture

```
platform/detect.go   → OS + available package managers (+ HasOpenCode)
scanner/             → Source interface + RunAll() (parallel)
  ├── brew, apt, mas, winget, … package managers
  ├── opencode.go    → npm outdated --prefix ~/.config/opencode
  ├── agents.go      → AI CLIs + npm global outdated merge
  ├── homelab_clean  → retention cleanups (env-driven)
  └── cleanup.go     → brew/docker/npm/go caches, SDKMAN majors
updater/             → batch updates (brew/mas/npm/opencode plugins/agents)
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

## Coverage / CI gates

- **COVER_PKGS (≥90%)**: `./internal/model/... ./internal/config/... ./internal/sizefmt/... ./internal/cli/... ./internal/retention/...`
- **TEST_IO_PKGS** (race, no floor): scanner, tui, cleaner
- Sonar excludes the same I/O packages from coverage measurement
- Keep complexity low (Sonar S3776); prefer small helpers over large switches
- gosec excludes: `G204,G306,G703,G118`

## Cross-platform quirks

| OS | PMs | Notes |
|---|---|---|
| macOS | brew, mas | brew cask exclusion list in `brew.go` |
| Linux | apt, pacman/yay, flatpak, snap | `sudo` for apt/pacman/snap |
| Windows | winget, choco, scoop | TEMP cleanup |

- Agent version probes skip Electron CLIs without `DISPLAY` on Linux
- Manual-only agents use `KeepPolicy` containing `manual` → skipped in CLI update
- OpenCode binary: `opencode upgrade`; plugins: `npm update --prefix ~/.config/opencode`

## Retention policy (env)

`updash --env-defaults` prints effective values. Defaults:

- Docker ages: `336h` (14d)
- Docker builder mode: `age` (use `UPDASH_DOCKER_BUILDER_MODE=all` on CI/homelab)
- Container log truncate: `50` MB
- Host logs / AI outputs / dev caches: age in days (30 / 7 / 90)
- Disk pressure: prune aggressively when used% ≥ 85

Homelab clean category: `homelab-clean` (`--only homelab-clean`).

**Builder mode note:** `age` + `until=` frequently reclaims 0B on active build hosts. Prefer `all` there; keep shell `builder prune -af` as belt-and-suspenders if desired.

## Repository

- Public: `github.com/lgldsilva/updash`
- **No direct pushes to `main`** — feature branches + PRs
