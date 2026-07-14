package updater

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// explainMasFailure turns mas errors into an actionable message for the TUI/CLI.
func explainMasFailure(itemName, packageID, output string, err error) string {
	if err == nil {
		return fmt.Sprintf(
			"%s ainda desatualizado — download da App Store pode estar em andamento",
			itemName,
		)
	}

	combined := strings.ToLower(output + " " + err.Error())
	killed := strings.Contains(combined, "killed") || strings.Contains(combined, "signal")

	if errors.Is(err, context.DeadlineExceeded) || killed {
		cmd := masSuggestCommand(packageID)
		return fmt.Sprintf(
			"%s: mas interrompido (sudo expirou ou precisa de Terminal) — tente de novo com senha ou rode: %s",
			itemName, cmd,
		)
	}

	if strings.Contains(combined, "sudo") || strings.Contains(combined, "password") ||
		strings.Contains(combined, "tty") {
		return fmt.Sprintf(
			"%s: precisa da senha de administrador do Mac — confirme a senha e tente de novo, ou rode: %s",
			itemName, masSuggestCommand(packageID),
		)
	}

	trimmed := strings.TrimSpace(output)
	if len(trimmed) > 160 {
		trimmed = trimmed[:160] + "…"
	}
	if trimmed != "" {
		return fmt.Sprintf("%s: mas update falhou: %v — %s", itemName, err, trimmed)
	}
	return fmt.Sprintf("%s: mas update falhou: %v", itemName, err)
}

func masSuggestCommand(packageID string) string {
	if packageID != "" {
		return "mas update " + packageID
	}
	return "mas update <app-id>"
}
