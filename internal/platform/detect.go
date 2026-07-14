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

	// Distro detection
	switch p.OS {
	case "darwin":
		p.Distro = "macos"
		if has("sw_vers") {
			if b, err := exec.Command("sw_vers", "-productName").Output(); err == nil {
				v := strings.TrimSpace(string(b))
				if strings.Contains(v, "macOS") || strings.Contains(v, "Mac OS") {
					p.Distro = "macos"
				}
			}
		}
	case "linux":
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

	// Package manager probes (order: platform-specific first)
	if p.OS == "darwin" {
		p.HasBrew = has("brew")
		p.HasMAS = has("mas")
	}
	if p.OS == "linux" {
		p.HasApt = has("apt-get")
		p.HasPacman = has("pacman")
		p.HasYay = has("yay")
		p.HasFlatpak = has("flatpak")
		p.HasSnap = has("snap")
		p.HasBrew = has("brew") // brew can be on Linux too
	}

	// Cross-platform tools
	p.HasNpm = has("npm")
	p.HasPipx = has("pipx")
	p.HasGo = has("go")
	p.HasGup = has("gup")
	p.HasRustup = has("rustup")
	p.HasCargo = has("cargo")

	// SDKMAN
	if _, err := os.Stat(os.ExpandEnv("$HOME/.sdkman/bin/sdkman-init.sh")); err == nil {
		p.HasSDKMAN = true
	}

	p.HasDocker = has("docker")
	p.HasNvm = has("nvm") || dirExists(os.ExpandEnv("$HOME/.nvm"))
	p.HasOmz = dirExists(os.ExpandEnv("$HOME/.oh-my-zsh"))

	return p
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
