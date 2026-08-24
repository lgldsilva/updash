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

The self-update path is **fail-closed**: it verifies the release archive
against the SHA-256 published in `checksums.txt` and refuses to install when
the digest is missing, malformed, ambiguous, or does not match. TLS
verification is never disabled — a self-signed mirror is supported only by
adding its CA through `UPDASH_TLS_CA_CERT`, and a redirect that downgrades
`https` to plaintext is refused. Downloads and archive extraction are bounded
(64 MiB archive, 192 MiB decompressed) and archive members with absolute or
`..` paths are rejected. The new binary is staged in the install directory,
fsynced, and swapped in with an atomic rename that preserves the current file
permissions; the running binary is never truncated in place. `install.sh`
stages the same way, so neither path can be redirected by a symlink planted at
a predictable staging name.

Release binaries and native Linux packages are published on GitHub. See the
[distribution model](docs/DISTRIBUTION.md) for `.deb`, `.rpm`, `.apk`, Arch,
AUR, and Snap availability. `updash --upgrade` is reserved for a binary
installed in `~/.local/bin` (plus the documented Windows user locations);
package-manager installations are detected and left alone so that their manager
keeps ownership of updates, signatures, and rollback.

| Variable | Purpose |
|----------|---------|
| `UPDASH_UPDATE_API` | Release API base URL (GitHub-compatible mirror) |
| `UPDASH_UPDATE_URL` | Release asset download prefix |
| `UPDASH_UPDATE_TOKEN` | Token for a private releases host |
| `UPDASH_TLS_CA_CERT` | Extra CA certificate for a self-signed host |
| `UPDASH_ALLOW_SELF_UPDATE` | `1` overrides the package-manager ownership guard |
| `UPDASH_SKIP_AUTO_UPGRADE` | `1` disables the startup upgrade check |

## CLI

| Flag | Description |
|------|-------------|
| `--check`, `-c` | Scan outdated + cleanable |
| `--check --json` | Machine-readable report (cron / monitoring) |
| `--update` | Update outdated items |
| `--clean` | Run cleanup |
| `--all`, `-a` | Update then clean |
| `--only <cat>` | Limit the run to one canonical category (see below) |
| `--dry-run` | Print plan without executing |
| `--strict` | Non-zero exit if anything remains outdated/cleanable |
| `--skip-password` | Skip sudo-needing batches |
| `--env-defaults` | Print effective `UPDASH_*` retention vars |
| `--upgrade` | Self-update from latest release |

### `--only <category>`

`--only` takes exactly one **canonical category** (case-insensitive) and is
validated **before** anything is scanned or executed: an unknown category, or
one that is not available on this host / in this mode, exits `2` without
touching the system. It filters `--check`, `--update`, `--clean`, and `--all`
alike, and a filtered mutable run can never widen its scope back to the other
sources.

```sh
updash --check --only brew
updash --update --only npm
updash --clean  --only docker
updash --clean  --only homelab-clean
updash --only nope        # exit 2: invalid or unavailable --only category
```

### Exit codes

| Code | Meaning |
|------|---------|
| `0` | Everything the run could verify is fine |
| `1` | Operational failure, or `--strict` with something still outdated/cleanable |
| `2` | The scan is **inconclusive** (errors or unverified sources), or the invocation is invalid |

Precedence is `2 > 1 > 0`. Exit `2` wins over `--strict`, because an
inconclusive scan cannot honestly answer "is anything pending?". When `--json`
is used, the report is written to stdout **before** the non-zero exit, so
automation always gets a parseable document.

Mutable modes (`--update`, `--clean`, `--all`) preflight every included source
first: if any of them is inconclusive, nothing is executed and the run exits
`2`. `--all` also refuses to start cleanup when the post-update verification
is inconclusive.

### Status semantics

| Status | Meaning |
|--------|---------|
| `ok` | Affirmatively verified as up to date |
| `outdated` | An update is available |
| `error` | The check failed |
| `unverified` | The source could not establish a trustworthy state — **blocks mutation** |
| `info` | Informational only; no affirmative freshness check was possible |

`ok` is reserved for affirmative verification. Inventory-only sources (a
package manager that can list what is installed but not what is current) report
`info`, never `ok`, and the CLI/TUI never print success copy for an
`info`-only summary. `unverified` and `error` are counted separately in the
JSON report (`unverified`, `errors`, `info`) and both drive exit code `2`.

### JSON check (automation)

```sh
updash --check --json | jq '.outdated, .cleanable'
# exit 1 when something is pending:
updash --check --json --strict
# exit 2 when the scan itself is inconclusive:
updash --check --json | jq '.errors, .unverified, .info'
```

## What it covers

**Updates:** Homebrew, MAS, apt, dnf/yum, zypper, pacman/yay, flatpak, snap, winget, chocolatey, scoop, npm (global), pnpm (global), bun (global), OpenCode plugins (`~/.config/opencode`), pipx, Go (`gup`), rustup/cargo, SDKMAN (cleanup), nvm/omz (presence), Docker disk summary, AI agents (Claude, OpenCode, Grok, Codex, Gemini, pi, Qwen, …), AI infra (ai-memory, semidx, gh extensions, gcloud).

Agent outdated detection merges `npm outdated -g` with a direct npm-registry latest lookup, so agents installed via native installer, Homebrew, pnpm or bun are flagged too.

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
| `UPDASH_DEV_PROJECTS_CLEAN` | *(unset)* | **Opt-in** switch for project build cleanup |
| `UPDASH_DEV_PROJECTS_DIR` | `~/Projetos` → `~/Projects` | Root scanned for project build dirs |

### Project build cleanup (opt-in)

The `dev-cache:projects-builds` item is **off by default**. It is only scanned
and only cleaned when `UPDASH_DEV_PROJECTS_CLEAN` is `1`/`true`/`on`/`yes`; the
cleaner independently refuses the item without it, so an opt-out run removes
nothing.

```sh
export UPDASH_DEV_PROJECTS_CLEAN=1
export UPDASH_DEV_PROJECTS_DIR="$HOME/Projetos"
updash --check --only homelab-clean
```

Build directories (`target`, `build`, `dist`, `.next`, `out`, `.turbo`,
`__pycache__`, `.pytest_cache`) are always collected. `node_modules` is only
collected when the project shows **no activity newer than**
`UPDASH_DEV_CACHE_MAX_DAYS`, measured as the newest mtime among the project
directory and its first-level entries.

Guardrails: the walk never follows symlinks, never collects the projects root
itself, stays inside the canonicalized root, and rejects a root that is the
filesystem root, the user home, or shallower than two path components. Per
project walk/stat failures are reported as partial errors and the project is
treated as active; when nothing trustworthy could be collected the item is
`unverified`, which blocks mutation and exits `2`.

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
