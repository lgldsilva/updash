# updash — AGENTS.md

## Build & run

```sh
go build -o updash ./cmd/updash/   # single binary
make check                          # build + --check
make install                        # copies to $HOME/.local/bin/updash
```

Three entry modes in `cmd/updash/main.go`:
- `updash` — interactive Bubble Tea TUI
- `updash --check` — headless scan, prints outdated + cleanable
- `updash --all` — headless: update everything + cleanup
- `updash --update-self` — `git pull` + rebuild + reinstall

## Architecture

```
platform/detect.go   → detects OS + available package managers
scanner/             → each file = one Source (brew, apt, winget, npm, agents...)
  ├── scanner.go     → Source interface + RunAll() (parallel goroutines)
  ├── brew.go        → brew outdated --greedy --json=v2
  ├── sdkman.go      → reads ~/.sdkman/candidates/ dirs
  ├── agents.go      → exec.LookPath for each AI tool binary
  ├── winget.go      → winget upgrade --json (Windows)
  └── cleanup.go     → cache size scanners (brew, go, npm, docker, Windows TEMP)
updater/             → runs update commands (batched by category)
cleaner/             → runs cleanup with retention policies
tui/                 → Bubble Tea model/view/update (3 tabs: Updates, Cleanup, Logs)
model/types.go       → Item, Category, Status, PlatformInfo, SourceSummary
```

## Adding a new package manager source

1. Create `internal/scanner/<name>.go` implementing `Source` interface (Category, Label, Icon, Scan)
2. Register in `enabledSources()` in `scanner.go`
3. Add update logic in `internal/updater/updater.go` (`updateOne` or `updateBatch`)
4. Add cleanup logic in `internal/cleaner/cleaner.go` if needed

## Cross-platform quirks

| OS | PMs | Notes |
|---|---|---|
| macOS | brew, mas | brew cask exclusion list in `brew.go` (JetBrains Toolbox apps, Microsoft Office, WhatsApp brew → managed elsewhere) |
| Linux | apt, pacman/yay, flatpak, snap | `sudo` required for apt/pacman |
| Windows | winget, choco, scoop | — |

- Platform detection in `platform/detect.go` — probes `exec.LookPath` + dir checks
- `runCmd()` ignores brew/mas exit code 1 (warnings), verifies actual upgrade via post-check
- `mas upgrade` runs via `sudo -S` (needs TTY for password)
- Agent binary scanning uses `exec.LookPath` + `parseAgentVersion()` (semver regex extraction)

## SDKMAN retention policy

`internal/scanner/sdkman.go` → `groupLatestPerMajor()`:
- Groups installed versions by major (e.g. "21" from "21.0.7-tem")
- Keeps only the **latest semver per major line**
- Reports remaining old versions as cleanup candidates (`StatusCleanCandidate`)

## Docker cleanup

- `internal/cleaner/cleaner.go` → `cleanDocker()`
- Age filters via `internal/config` (`UPDASH_DOCKER_*`, default `336h` / 14 days)
- Volumes do NOT support `--filter until` — pruned unconditionally (no filter)
- Volumes use `docker volume prune -f` only
- `updash --env-defaults` prints effective retention env vars

## Brew exclusion list

`externalCasks` in `brew.go` — casks managed by other tools that brew upgrade would fail on:
- JetBrains Toolbox apps (clion, datagrip, goland, intellij-idea-ce, phpstorm, pycharm, etc.)
- Microsoft Office / Auto-Update (needs sudo TTY)
- WhatsApp brew (prefer MAS version)

## Repository

- `github.com/lgldsilva/updash` (Go module path)
- Gitea: `github.com/lgldsilva/updash`
- **No direct pushes to `main`** — ai-standards hooks require feature branches + PRs
- Project has **no tests yet** (0% coverage — gates will fail)
