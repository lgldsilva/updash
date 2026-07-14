package updater

import (
	"strings"

	"github.com/lgldsilva/updash/internal/model"
	"github.com/lgldsilva/updash/internal/scanner"
)

// ItemKind classifies why an item did not update.
type ItemKind int

const (
	KindOK ItemKind = iota
	KindNeedsPassword
	KindManualOnly
	KindFailed
)

// ClassifyItem returns the outcome bucket and a human-readable reason.
func ClassifyItem(item *model.Item, result *Result) (ItemKind, string) {
	if result != nil && result.Success {
		return KindOK, ""
	}

	errMsg := ""
	out := ""
	if result != nil {
		errMsg = result.Error
		out = result.Output
	}

	if item.KeepPolicy != "" && isManualOnlyNote(item.KeepPolicy) {
		return KindManualOnly, item.KeepPolicy
	}

	combined := strings.ToLower(errMsg + " " + out)
	if OutputNeedsPassword(combined) || itemNeedsPasswordHint(item) {
		reason := errMsg
		if reason == "" {
			reason = item.KeepPolicy
		}
		if reason == "" {
			reason = "precisa de senha de administrador"
		}
		return KindNeedsPassword, reason
	}

	if errMsg != "" {
		return KindFailed, errMsg
	}
	if note := scanner.BrewUpgradeNote(item.Name); note != "" {
		return KindManualOnly, note
	}
	return KindFailed, "ainda desatualizado após update"
}

// OutputNeedsPassword reports sudo/password hints in command output or errors.
func OutputNeedsPassword(s string) bool {
	s = strings.ToLower(s)
	return strings.Contains(s, "sudo") ||
		strings.Contains(s, "password") ||
		strings.Contains(s, "administrator") ||
		strings.Contains(s, "tty") ||
		strings.Contains(s, "killed") ||
		strings.Contains(s, "signal")
}

func itemNeedsPasswordHint(item *model.Item) bool {
	if item.Category == model.CatMAS {
		return true
	}
	note := scanner.BrewUpgradeNote(item.Name)
	return strings.Contains(note, "senha") || strings.Contains(note, "admin")
}

func isManualOnlyNote(note string) bool {
	n := strings.ToLower(note)
	return strings.Contains(n, "jetbrains") ||
		strings.Contains(n, "toolbox") ||
		strings.Contains(n, "app store") && strings.Contains(n, "prefira")
}

// SuggestCommand returns a manual command for an outdated item.
func SuggestCommand(item *model.Item) string {
	switch item.Category {
	case model.CatMAS:
		if item.PackageID != "" {
			return "mas update " + item.PackageID
		}
		return "mas update <app-id>"
	case model.CatBrew:
		return "brew upgrade --greedy " + item.Name
	default:
		return ""
	}
}
