# Distribution model

GitHub Releases are the immutable source for every distributable artifact.
The release workflow generates archives, SHA-256 checksums, an SBOM, and native
Linux packages from the same tag.

## Available with every release

The installer (`install.sh`) and the self-updater share the same staging
discipline: the new binary is written to an unpredictable temporary path
created inside the destination directory (so a pre-planted symlink cannot be
written through, chmod'ed, or moved into place), given the destination's
existing permissions with the executable bit forced on, and then moved in with
an atomic rename. A failed install leaves the current binary untouched.

| Channel | Artifact | Install/update model |
|---|---|---|
| GitHub Releases | `tar.gz` / `zip` + `checksums.txt` | `./install.sh binary` or `updash --upgrade` when installed in `~/.local/bin` |
| Debian/Ubuntu | `.deb` | `apt` / `dpkg` owns updates |
| Fedora/RHEL/openSUSE | `.rpm` | `dnf`, `yum`, or `zypper` owns updates |
| Alpine | `.apk` | `apk` owns updates |
| Arch-compatible repositories | Arch Linux package | `pacman` owns updates |
| AUR | rendered `PKGBUILD` after AUR publication | the AUR helper owns updates |
| Snap Store | `snapcraft.yaml` after Store approval and publication | `snap refresh` owns updates |

The application deliberately skips automatic binary replacement when it is not
installed in `~/.local/bin` (or, on Windows, `%LOCALAPPDATA%\updash`, the Scoop
shims directory, `%USERPROFILE%\bin`, or the Chocolatey bin directory). The
path is resolved through symlinks first, so a `~/.local/bin` symlink that
points into a Homebrew or Snap prefix is still recognised as package-managed
and refused. Package-manager installs must retain their package database,
signatures, and rollback behavior. `UPDASH_ALLOW_SELF_UPDATE=1` is an explicit
override for a deliberately self-managed install.

## Self-update trust model

`updash --upgrade` and the startup auto-upgrade share one fail-closed pipeline.
Every step below must succeed or the installed binary is left untouched.

| Step | Guarantee |
|---|---|
| Transport | TLS 1.2+ with the system trust store; verification is never disabled. `UPDASH_TLS_CA_CERT` only *adds* a CA. |
| Redirects | At most 5 hops, and a redirect from `https` to any plaintext scheme is refused. |
| Download size | Release archive bounded to 64 MiB, `checksums.txt` to 1 MiB, release JSON to 8 MiB — enforced against both the declared `Content-Length` and the streamed body. |
| Integrity | SHA-256 from the release `checksums.txt` is **mandatory**. A missing entry, a digest that is not 64 hex characters, two entries that disagree, or a mismatch all abort the upgrade. A truncated download therefore fails the digest, never the binary swap. |
| Extraction | Total decompressed payload bounded to 192 MiB (tar/zip bomb defense). Members with absolute paths, Windows volume prefixes, or `..` components are rejected; only regular files are candidates, so symlink and hardlink entries can never be installed. |
| Payload | The selected member must carry an ELF / Mach-O / PE magic number — a text or script file named `updash` is refused. An empty payload is refused. |
| Replacement | The verified binary is staged in the destination directory with `O_EXCL` (a pre-planted symlink at the staging path cannot redirect the write), `fsync`ed, given the *current* binary's permissions with the executable bit forced on, and moved into place with an atomic `rename`. The running binary is never truncated in place, and a failed rename removes the staged file and leaves the installation untouched. |
| Windows | `rename` over a locked executable fails, so the current binary is moved to `<name>.old` first and rolled back if the swap fails. The `.old` file is removed on the next startup. |
| Ownership | Package-managed installs are refused before anything is downloaded. |

## Release chain

Artifacts are only produced from a commit that already passed the gated main
CI:

1. `ci.yml` runs the full gate on `main` and, only when every gate is green,
   dispatches `autotag.yml` with the tested SHA.
2. `autotag.yml` refuses to tag when `main` has advanced past that tested SHA,
   computes the next semantic version, pushes the tag, and dispatches
   `release.yml`.
3. `release.yml` re-validates the tag shape (`^v[0-9]+\.[0-9]+\.[0-9]+$`) and
   verifies that the tag commit is **contained in `origin/main`** before
   building anything, so a tag pushed onto an arbitrary commit cannot become a
   release. That verified commit SHA — not the mutable tag name — is exported
   and checked out by build, SBOM, and GoReleaser, so force-moving the tag
   during the build window cannot swap in an unverified commit.
4. A tag that already has a published `checksums.txt` is refused: re-running the
   release would replace artifacts whose digests users already recorded. A
   half-finished run (no `checksums.txt`) still recovers automatically, and a
   deliberate overwrite requires dispatching with `force_rerelease=true`.

Workflow tokens default to `contents: read`; only the publishing job elevates.
Workflow inputs are passed to shell steps through environment variables rather
than `${{ }}` interpolation.

## AUR publication

The AUR is a separate Git repository and needs an AUR account plus an SSH key.
For each tag, render `packaging/aur/PKGBUILD.tmpl` with the release version and
the SHA-256 hashes of the two Linux archives. In the cloned AUR repository run:

```sh
makepkg --printsrcinfo > .SRCINFO
git add PKGBUILD .SRCINFO
git commit -m "updash: v<version>"
git push
```

Publish only after confirming that `updash` is not already provided by an
official Arch repository or a maintained AUR package.

## Snap Store publication

`packaging/snap/snapcraft.yaml` builds amd64 and arm64 snaps. It uses classic
confinement because updash must invoke host package managers; classic snaps
require prior store approval. Register the `updash` name, obtain that approval,
then build and upload the release artifact to the desired channel:

```sh
snapcraft pack --destructive-mode --project-dir packaging/snap
snapcraft upload --release=stable updash_*.snap
```

Store credentials must stay in the Snap Store or GitHub Actions secrets; they
must never be committed to this repository.

## Why Flatpak is intentionally unsupported

Flatpak sandboxes applications and does not grant direct access to host
executables such as `apt`, `pacman`, `snap`, Docker, or AI CLIs. A Flatpak build
would therefore display a dashboard that cannot perform its advertised host
updates and cleanup. Native packages or the verified GitHub binary are the
supported Linux distribution methods.
