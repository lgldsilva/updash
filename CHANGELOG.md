# Changelog

All notable changes to this project are documented here.
Format follows [Keep a Changelog](https://keepachangelog.com/); versions follow [Semantic Versioning](https://semver.org/).

## [Unreleased]

### Added
- Homebrew tap (`brew install lgldsilva/tap/updash`)
- Scoop bucket (`scoop install updash`)
- Linux packages: `.deb`, `.rpm`, `.apk` via GoReleaser nfpms
- Docker image published to `ghcr.io/lgldsilva/updash` (multi-arch: amd64 + arm64)
- SLSA L3 provenance attestation on release artifacts
- Coverage gate (≥90%) expanded to `upgrade`, `updater`, `elevate` packages
- `make test-gate` and `make coverage` targets

## [0.6.1] - 2026-07-18

### Added
- Docker builder prune mode `age|all` + safe self-upgrade extract
- OpenCode plugins scanning, homelab retention cleanup, agent updates, `--json` output
- Build version banner on startup with auto-upgrade from release
- CLI modes: headless updates with verify report, macOS native sudo dialog
- Brew failure diagnostics instead of silent skip
- TUI rescan after update with post-upgrade verification
- Auto-upgrade from configurable release URL (Gitea/GitHub compatible)

### Fixed
- GoReleaser archive name matching on self-update
- Flatpak 1.18 compatibility + clearer TUI scan errors
- Manjaro scan hang on GUI agent CLIs
- Brew upgrade: only selected packages, skip Microsoft casks
- Flaky `TestNpmScan` (map iteration order + mutex)
- Various SonarQube smells (S1192, S3776, S107) and gosec findings

## [0.6.0] - 2026-07-16

### Added
- Docker builder prune mode `age|all` + safe self-upgrade extract

### Fixed
- Extract docker filter/until constants (S1192)
- Drop deprecated `tar.TypeRegA` (staticcheck SA1019)

## [0.5.0] - 2026-07-15

### Added
- OpenCode plugins, homelab retention cleanup, agent updates, `--json`

## [0.4.6] - 2026-07-15

### Fixed
- Sonar security hotspots (Dockerfile + TLS)
- Gitleaks history false positive

## [0.4.5] - 2026-07-15

### Fixed
- Sonar S1192/S3776 smells across scanner, cli, tui
- Enforce 90% coverage on gate packages

## [0.4.4] - 2026-07-15

### Changed
- Lower Sonar cognitive complexity in CLI batches and reports

## [0.4.3] - 2026-07-15

### Fixed
- Sonar new_violations and CRITICAL smells
- Reduce cognitive complexity and string duplication

## [0.4.2] - 2026-07-15

### Fixed
- Deflake `EnsureSudoReady_NoSession` under CI `-race`

## [0.4.1] - 2026-07-15

### Fixed
- Stable full-width TUI frame and column layout

## [0.4.0] - 2026-07-15

### Added
- Initial public release structure

## [0.3.0] - 2026-07-15

### Added
- Sonar Community PR scans via ephemeral project

## [0.2.0] - 2026-07-15

### Added
- Env retention defaults and gh-ext update wiring

### Fixed
- Local validate gates (lint, gosec, coverage)

## [0.1.0] - 2026-07-14

### Added
- Initial release: TUI dashboard, multi-source scanner, headless CLI modes

[Unreleased]: https://github.com/lgldsilva/updash/compare/v0.6.1...HEAD
[0.6.1]: https://github.com/lgldsilva/updash/compare/v0.6.0...v0.6.1
[0.6.0]: https://github.com/lgldsilva/updash/compare/v0.5.1...v0.6.0
[0.5.0]: https://github.com/lgldsilva/updash/compare/v0.4.6...v0.5.0
[0.4.6]: https://github.com/lgldsilva/updash/compare/v0.4.5...v0.4.6
[0.4.5]: https://github.com/lgldsilva/updash/compare/v0.4.4...v0.4.5
[0.4.4]: https://github.com/lgldsilva/updash/compare/v0.4.3...v0.4.4
[0.4.3]: https://github.com/lgldsilva/updash/compare/v0.4.2...v0.4.3
[0.4.2]: https://github.com/lgldsilva/updash/compare/v0.4.1...v0.4.2
[0.4.1]: https://github.com/lgldsilva/updash/compare/v0.4.0...v0.4.1
[0.4.0]: https://github.com/lgldsilva/updash/compare/v0.3.0...v0.4.0
[0.3.0]: https://github.com/lgldsilva/updash/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/lgldsilva/updash/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/lgldsilva/updash/releases/tag/v0.1.0
