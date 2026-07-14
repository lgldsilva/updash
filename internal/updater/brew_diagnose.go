package updater

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/lgldsilva/updash/internal/scanner"
)

// explainBrewUpgradeFailure turns brew output + process error into an actionable message.
func explainBrewUpgradeFailure(name, output string, err error, timedOut bool) string {
	combined := strings.ToLower(output)
	if err != nil {
		combined += " " + strings.ToLower(err.Error())
	}

	if timedOut || errors.Is(err, context.DeadlineExceeded) {
		if note := scanner.BrewUpgradeNote(name); note != "" {
			return fmt.Sprintf(
				"timeout (%s): %s — rode no Terminal: brew upgrade --greedy %s",
				name, note, name,
			)
		}
		if strings.Contains(combined, "microsoft") {
			return fmt.Sprintf(
				"timeout: instalador Microsoft aguardando senha de admin — rode no Terminal: brew upgrade --greedy %s",
				name,
			)
		}
		return fmt.Sprintf(
			"timeout: brew upgrade demorou demais (pode estar aguardando senha ou download) — rode no Terminal: brew upgrade --greedy %s",
			name,
		)
	}

	if strings.Contains(combined, "sudo") || strings.Contains(combined, "password") ||
		strings.Contains(combined, "administrator") {
		return fmt.Sprintf(
			"precisa de senha de administrador — rode no Terminal: brew upgrade --greedy %s",
			name,
		)
	}

	if note := scanner.BrewUpgradeNote(name); note != "" {
		if err != nil {
			return fmt.Sprintf("%s — %s (brew: %v)", name, note, err)
		}
		return fmt.Sprintf("%s ainda desatualizado — %s", name, note)
	}

	if err != nil {
		trimmed := strings.TrimSpace(output)
		if len(trimmed) > 200 {
			trimmed = trimmed[:200] + "…"
		}
		if trimmed != "" {
			return fmt.Sprintf("brew upgrade falhou: %v — %s", err, trimmed)
		}
		return fmt.Sprintf("brew upgrade falhou: %v", err)
	}

	return fmt.Sprintf(
		"%s ainda desatualizado após brew upgrade (verifique manualmente: brew upgrade --greedy %s)",
		name, name,
	)
}
