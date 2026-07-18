# Distribution model

GitHub Releases are the immutable source for every distributable artifact.
The release workflow generates archives, SHA-256 checksums, an SBOM, and native
Linux packages from the same tag.

## Available with every release

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
installed in `~/.local/bin`. Package-manager installs must retain their package
database, signatures, and rollback behavior. `UPDASH_ALLOW_SELF_UPDATE=1` is an
explicit override for a deliberately self-managed install.

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
