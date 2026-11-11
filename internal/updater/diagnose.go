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

const disabledCaskReason = "cask desabilitado no brew — nada a atualizar até reativação (brew outdated segue listando)"

// ClassifyItem returns the outcome bucket and a human-readable reason.
func ClassifyItem(item *model.Item, result *Result) (ItemKind, string) {
	if result != nil && result.Success {
		return KindOK, ""
	}
	errMsg, out := resultTexts(result)
	if reason, ok := manualOnlyReason(item, errMsg); ok {
		return KindManualOnly, reason
	}
	if kind, reason, ok := failedUpdateKind(item, errMsg, out); ok {
		return kind, reason
	}
	return KindFailed, "ainda desatualizado após update"
}

func resultTexts(result *Result) (errMsg, out string) {
	if result == nil {
		return "", ""
	}
	return result.Error, result.Output
}

// manualOnlyReason reports a choice the updater will not make for the user.
// The note lives on the item when the scan already knew (Toolbox, manual
// agents) and on the result when a later run discovered it (npm vs cask).
// A rescan allocates a fresh Item, so the result has to carry the note too.
func manualOnlyReason(item *model.Item, errMsg string) (string, bool) {
	if item.KeepPolicy != "" && isManualOnlyNote(item.KeepPolicy) {
		return item.KeepPolicy, true
	}
	return skippedManualReason(errMsg)
}

func skippedManualReason(errMsg string) (string, bool) {
	if !strings.HasPrefix(errMsg, "⊘ ") {
		return "", false
	}
	reason := strings.TrimSpace(strings.TrimPrefix(errMsg, "⊘ "))
	if reason == "" || !isManualOnlyNote(reason) {
		return "", false
	}
	return reason, true
}

func failedUpdateKind(item *model.Item, errMsg, out string) (ItemKind, string, bool) {
	combined := strings.ToLower(errMsg + " " + out)
	if OutputNeedsPassword(combined) || itemNeedsPasswordHint(item) {
		return KindNeedsPassword, passwordReason(item, errMsg), true
	}
	// Homebrew refuses to upgrade casks it disabled upstream (Gatekeeper /
	// notarization failures) but keeps listing them in `brew outdated
	// --greedy`, so the item can never turn green on its own.
	if brewDisabledCask(combined) {
		return KindManualOnly, disabledCaskReason, true
	}
	if errMsg != "" {
		return KindFailed, errMsg, true
	}
	if note := scanner.BrewUpgradeNote(item.Name); note != "" {
		return KindManualOnly, note, true
	}
	return 0, "", false
}

func passwordReason(item *model.Item, errMsg string) string {
	if errMsg != "" {
		return errMsg
	}
	if item.KeepPolicy != "" {
		return item.KeepPolicy
	}
	return "precisa de senha de administrador"
}

func brewDisabledCask(combined string) bool {
	return strings.Contains(combined, "not upgrading") && strings.Contains(combined, "it is disabled")
}

// OutputNeedsPassword reports sudo/password hints in command output or errors.
func OutputNeedsPassword(s string) bool {
	s = strings.ToLower(s)
	return strings.Contains(s, "sudo") ||
		strings.Contains(s, "password") ||
		strings.Contains(s, "administrator") ||
		strings.Contains(s, "tty")
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
		strings.Contains(n, "manual") ||
		(strings.Contains(n, "app store") && strings.Contains(n, "prefira"))
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
	case model.CatOpenCodePlugins:
		return "npm update --prefix ~/.config/opencode"
	case model.CatAgent:
		return suggestAgentCommand(item.Name)
	default:
		return ""
	}
}

func suggestAgentCommand(name string) string {
	if name != agentCrush {
		if cmd := scanner.AgentUpdateCommand(name); len(cmd) > 0 {
			return strings.Join(cmd, " ")
		}
	}
	switch {
	case strings.Contains(name, "Claude"):
		return "claude update"
	case strings.Contains(name, "OpenCode"):
		return "opencode upgrade"
	case strings.Contains(name, "Grok"):
		return "grok update"
	case strings.Contains(name, "Gemini"):
		return "gemini update"
	case strings.Contains(name, "Codex"):
		return "npm install -g --allow-scripts=@openai/codex @openai/codex@latest"
	case strings.Contains(name, "MimoCode"):
		return "mimo upgrade"
	case strings.Contains(name, "Crush"):
		return suggestCrushCommand()
	case strings.Contains(name, "Copilot"):
		return "npm install -g --allow-scripts=@github/copilot @github/copilot@latest"
	default:
		return ""
	}
}

// suggestCrushCommand repeats the prefix-aware plan. A layout that is not
// npm-managed stays on the manual note instead of an unscoped npm install,
// which would create a second copy in the ambient prefix.
func suggestCrushCommand() string {
	plan := crushUpgradePlan()
	if plan.Scope != CommandScopeExact {
		return plan.Manual
	}
	return strings.TrimSpace(plan.Name + " " + strings.Join(plan.Args, " "))
}
