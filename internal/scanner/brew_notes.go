package scanner

import "strings"

// BrewUpgradeNote explains why a brew cask may not auto-update in headless/TUI mode.
// Empty string means no special constraint.
func BrewUpgradeNote(name string) string {
	switch name {
	case "microsoft-office", "microsoft-auto-update":
		return "PKG Microsoft — precisa de senha de admin no Terminal"
	case "whatsapp":
		return "cask brew — prefira atualizar pela Mac App Store"
	case "clion", "datagrip", "goland", "intellij-idea-ce", "phpstorm",
		"pycharm", "pycharm-ce", "fleet", "rubymine", "webstorm":
		return "gerido pelo JetBrains Toolbox"
	default:
		if strings.HasPrefix(name, "microsoft-") {
			return "PKG Microsoft — precisa de senha de admin no Terminal"
		}
		return ""
	}
}

// BrewNeedsSudoPrime reports casks whose installers call sudo internally (e.g. Microsoft PKG).
func BrewNeedsSudoPrime(name string) bool {
	note := strings.ToLower(BrewUpgradeNote(name))
	return strings.Contains(note, "senha") || strings.Contains(note, "admin")
}
