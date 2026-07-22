# updash

**System Update Dashboard** for macOS, Linux, and Windows — one binary for package updates, AI tools, and smart cleanup.

```sh
updash              # interactive TUI
updash --check      # headless scan
updash --all        # update + clean
```

## Install

### Homebrew (macOS / Linux)

```sh
brew install lgldsilva/tap/updash
```

### Scoop (Windows)

```sh
scoop bucket add lgldsilva https://github.com/lgldsilva/scoop-bucket
scoop install updash
```

### Linux packages (.deb / .rpm / .apk)

Download from [GitHub Releases](https://github.com/lgldsilva/updash/releases):

```sh
# Debian / Ubuntu
sudo dpkg -i updash_*_linux_amd64.deb

# RHEL / Fedora
sudo rpm -i updash_*_linux_amd64.rpm

# Alpine
sudo apk add --allow-untrusted updash_*_linux_amd64.apk
```

### Docker

```sh
docker run --rm ghcr.io/lgldsilva/updash:latest --check
```

### From source

```sh
go install github.com/lgldsilva/updash/cmd/updash@latest
# or
make install        # → $HOME/.local/bin/updash
```

### Prebuilt binary (curl)

```sh
./install.sh binary   # downloads latest release with SHA-256 verification
```

### Self-update

```sh
updash --upgrade      # download + verify + replace binary in-place
```

## CLI

| Flag | Description |
|------|-------------|
| `--check`, `-c` | Scan outdated + cleanable |
| `--check --json` | Machine-readable report (cron / monitoring) |
| `--update` | Update outdated items |
| `--clean` | Run cleanup |
| `--all`, `-a` | Update then clean |
| `--only <cat>` | Limit to one source (`brew`, `docker`, `homelab-clean`, …) |
| `--dry-run` | Print plan without executing |
| `--strict` | Non-zero exit if anything remains outdated/cleanable |
| `--skip-password` | Skip sudo-needing batches |
| `--env-defaults` | Print effective `UPDASH_*` retention vars |
| `--upgrade` | Self-update from latest release |

### JSON check (automation)

```sh
updash --check --json | jq '.outdated, .cleanable'
# exit 1 when something is pending:
updash --check --json --strict
```

## What it covers

**Updates:** Homebrew, MAS, apt, pacman/yay, flatpak, snap, winget, chocolatey, scoop, npm (global), OpenCode plugins (`~/.config/opencode`), pipx, Go (`gup`), rustup/cargo, SDKMAN (cleanup), nvm/omz (presence), Docker disk summary, AI agents (Claude, OpenCode, Grok, Codex, Gemini, …), AI infra (ai-memory, semidx, gh extensions, gcloud).

**Cleanup:** brew/apt/go/npm/snap caches, Docker prune (age-filtered images/containers; builder mode configurable), SDKMAN old majors, Antigravity/VS Code extension dupes, Windows TEMP, **homelab retention** (maven/gradle caches, AI tool outputs, host logs, container log truncate, disk-pressure prune).

## Retention env vars

```sh
updash --env-defaults
```

| Variable | Default | Used for |
|----------|---------|----------|
| `UPDASH_DOCKER_IMAGE_MAX_AGE` | `336h` | `docker image prune` |
| `UPDASH_DOCKER_BUILDER_MODE` | `age` | `age` = `until=<max>`; **`all`** = `builder prune -af` (no until) |
| `UPDASH_DOCKER_BUILDER_MAX_AGE` | `336h` | builder prune `until=` (**only when mode=`age`**) |
| `UPDASH_DOCKER_CONTAINER_MAX_AGE` | `336h` | container prune |
| `UPDASH_CONTAINER_LOG_MAX_MB` | `50` | truncate large container logs |
| `UPDASH_HOST_LOG_MAX_DAYS` | `30` | user/host log age |
| `UPDASH_DISK_PRESSURE_PCT` | `85` | aggressive docker prune when disk full |
| `UPDASH_DEV_CACHE_MAX_DAYS` | `90` | maven/gradle cache age |
| `UPDASH_AI_OUTPUT_MAX_DAYS` | `7` | AI tool output/cache age |

### CI / homelab Docker builder

On busy CI/build hosts, `until=` filters often reclaim **0B** because build layers stay "recent". For those machines:

```sh
export UPDASH_DOCKER_BUILDER_MODE=all
# optional: tighten image/container retention
export UPDASH_DOCKER_IMAGE_MAX_AGE=168h
export UPDASH_DOCKER_CONTAINER_MAX_AGE=168h
updash --clean --only docker
```

`mode=all` only drops **unused** build cache (`docker builder prune -af`). Images/containers still honor their age filters. Laptop default remains `age` (conservative).

## Development

```sh
make build
make test          # race tests on all packages
make test-gate     # race + coverage on gate packages (≥90%)
make coverage      # full coverage report
```

Coverage gate packages (`≥90%`): `model`, `config`, `sizefmt`, `cli`, `retention`, `upgrade`.
I/O packages (`scanner`, `tui`, `cleaner`, `updater`, `elevate`, `platform`) are race-tested without the 90% floor.

Architecture notes: see [AGENTS.md](./AGENTS.md). CI: [`.github/workflows/`](./.github/workflows/).

## License

MIT — see [LICENSE](./LICENSE).
