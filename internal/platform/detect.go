// Package platform detects the OS and available package managers.
package platform

import (
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/lgldsilva/updash/internal/model"
)

// Detect fills a PlatformInfo by probing the environment.
func Detect() model.PlatformInfo {
	p := model.PlatformInfo{
		OS: runtime.GOOS,
	}

	switch p.OS {
	case "darwin":
		p.Distro = "macos"
		p.HasBrew = has("brew")
		p.HasMAS = has("mas")

	case "linux":
		detectLinuxDistro(&p)
		p.HasApt = has("apt-get")
		p.HasDnf = has("dnf") || has("dnf5")
		p.HasYum = !p.HasDnf && has("yum")
		p.HasZypper = has("zypper")
		p.HasApk = has("apk")
		p.HasPacman = has("pacman")
		p.HasYay = has("yay")
		p.HasFlatpak = has("flatpak")
		p.HasSnap = has("snap")
		p.HasBrew = has("brew")

	case windowsOS:
		p.Distro = windowsOS
		p.HasWinget = has("winget")
		p.HasChoco = has("choco")
		p.HasScoop = has("scoop")
	}

	// Cross-platform tools
	p.HasNpm = has("npm")
	p.HasPnpm = has("pnpm")
	p.HasBun = has("bun")
	p.HasPipx = has("pipx")
	p.HasGo = has("go")
	p.HasGup = has("gup")
	p.HasRustup = has("rustup")
	p.HasCargo = has("cargo")

	// SDKMAN (Linux/macOS)
	if _, err := os.Stat(os.ExpandEnv("$HOME/.sdkman/bin/sdkman-init.sh")); err == nil {
		p.HasSDKMAN = true
	}

	// If HOME is not set, try USERPROFILE (Windows)
	if p.OS == windowsOS {
		home := os.Getenv("USERPROFILE")
		if _, err := os.Stat(home + "\\.sdkman\\bin\\sdkman-init.sh"); err == nil {
			p.HasSDKMAN = true
		}
	}

	p.HasDocker = has("docker")
	p.HasNvm = dirExists(os.ExpandEnv("$HOME/.nvm"))
	p.HasOpenCode = dirExists(os.ExpandEnv("$HOME/.config/opencode"))
	p.HasOmz = dirExists(os.ExpandEnv("$HOME/.oh-my-zsh"))

	// Windows: also check USERPROFILE for nvm-windows
	if p.OS == windowsOS && !p.HasNvm {
		home := os.Getenv("USERPROFILE")
		p.HasNvm = dirExists(home + "\\AppData\\Roaming\\nvm")
	}

	return p
}

func detectLinuxDistro(p *model.PlatformInfo) {
	if b, err := os.ReadFile("/etc/os-release"); err == nil {
		p.Distro = distroFromOSRelease(string(b))
	} else if b, err := os.ReadFile("/etc/lsb-release"); err == nil {
		if strings.Contains(string(b), "Ubuntu") {
			p.Distro = "ubuntu"
		}
	}
}

// distroFromOSRelease maps the contents of /etc/os-release to a distro tag.
// Derived distros must be matched before their parent: CachyOS and Manjaro both
// carry ID_LIKE=arch, so their ID checks come first.
func distroFromOSRelease(content string) string {
	switch {
	case strings.Contains(content, "ID=ubuntu"), strings.Contains(content, "ID_LIKE=ubuntu"):
		return "ubuntu"
	case strings.Contains(content, "ID=manjaro"):
		return "manjaro"
	case strings.Contains(content, "ID=cachyos"):
		return "cachyos"
	case strings.Contains(content, "ID=arch"), strings.Contains(content, "ID_LIKE=arch"):
		return "arch"
	case strings.Contains(content, "ID=debian"), strings.Contains(content, "ID_LIKE=debian"):
		return "debian"
	case strings.Contains(content, "ID=fedora"):
		return "fedora"
	case strings.Contains(content, "ID=opensuse"), strings.Contains(content, "ID_LIKE=suse"), strings.Contains(content, "ID_LIKE=\"suse\""):
		return "opensuse"
	case strings.Contains(content, "ID=alpine"):
		return "alpine"
	default:
		return "linux"
	}
}

// has checks if a command exists in PATH.
func has(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// dirExists checks if a path exists and is a directory.
func dirExists(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.IsDir()
}
