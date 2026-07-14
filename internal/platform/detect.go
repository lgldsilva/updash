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
		p.HasPacman = has("pacman")
		p.HasYay = has("yay")
		p.HasFlatpak = has("flatpak")
		p.HasSnap = has("snap")
		p.HasBrew = has("brew")

	case "windows":
		p.Distro = "windows"
		p.HasWinget = has("winget")
		p.HasChoco = has("choco")
		p.HasScoop = has("scoop")
	}

	// Cross-platform tools
	p.HasNpm = has("npm")
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
	if p.OS == "windows" {
		home := os.Getenv("USERPROFILE")
		if _, err := os.Stat(home + "\\.sdkman\\bin\\sdkman-init.sh"); err == nil {
			p.HasSDKMAN = true
		}
	}

	p.HasDocker = has("docker")
	p.HasNvm = dirExists(os.ExpandEnv("$HOME/.nvm"))
	p.HasOmz = dirExists(os.ExpandEnv("$HOME/.oh-my-zsh"))

	// Windows: also check USERPROFILE for nvm-windows
	if p.OS == "windows" && !p.HasNvm {
		home := os.Getenv("USERPROFILE")
		p.HasNvm = dirExists(home + "\\AppData\\Roaming\\nvm")
	}

	return p
}

func detectLinuxDistro(p *model.PlatformInfo) {
	if b, err := os.ReadFile("/etc/os-release"); err == nil {
		content := string(b)
		switch {
		case strings.Contains(content, "ID=ubuntu"), strings.Contains(content, "ID_LIKE=ubuntu"):
			p.Distro = "ubuntu"
		case strings.Contains(content, "ID=manjaro"):
			p.Distro = "manjaro"
		case strings.Contains(content, "ID=arch"), strings.Contains(content, "ID_LIKE=arch"):
			p.Distro = "arch"
		case strings.Contains(content, "ID=debian"), strings.Contains(content, "ID_LIKE=debian"):
			p.Distro = "debian"
		case strings.Contains(content, "ID=fedora"):
			p.Distro = "fedora"
		default:
			p.Distro = "linux"
		}
	} else if b, err := os.ReadFile("/etc/lsb-release"); err == nil {
		if strings.Contains(string(b), "Ubuntu") {
			p.Distro = "ubuntu"
		}
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
