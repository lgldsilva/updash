package updater

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestExplainBrewUpgradeFailure_microsoftTimeout(t *testing.T) {
	msg := explainBrewUpgradeFailure("microsoft-office", "", context.DeadlineExceeded, true)
	if !strings.Contains(msg, "microsoft-office") {
		t.Fatalf("missing package name: %q", msg)
	}
	if !strings.Contains(msg, "Terminal") || !strings.Contains(msg, "brew upgrade") {
		t.Fatalf("missing actionable command: %q", msg)
	}
}

func TestExplainBrewUpgradeFailure_sudoInOutput(t *testing.T) {
	msg := explainBrewUpgradeFailure("foo", "Error: Need sudo password", nil, false)
	if !strings.Contains(msg, "administrador") {
		t.Fatalf("expected admin hint: %q", msg)
	}
}

func TestExplainBrewUpgradeFailure_stillOutdatedWithNote(t *testing.T) {
	msg := explainBrewUpgradeFailure("microsoft-auto-update", "", nil, false)
	if !strings.Contains(msg, "PKG Microsoft") {
		t.Fatalf("expected microsoft note: %q", msg)
	}
}

func TestExplainBrewUpgradeFailure_excerptInEveryOutcome(t *testing.T) {
	warning := "Warning: Not upgrading flameshot, it is disabled because it does not pass the macOS Gatekeeper check!"
	still := explainBrewUpgradeFailure("flameshot", warning, nil, false)
	if !strings.Contains(still, "ainda desatualizado") || !strings.Contains(still, "it is disabled") {
		t.Fatalf("still-outdated message lost the excerpt: %q", still)
	}
	failed := explainBrewUpgradeFailure("iterm2", "Error: Download failed", errors.New("exit status 1"), false)
	if !strings.Contains(failed, "brew upgrade falhou") || !strings.Contains(failed, "Download failed") {
		t.Fatalf("failure message lost the excerpt: %q", failed)
	}
	bare := explainBrewUpgradeFailure("wget", "   ", nil, false)
	if strings.Contains(bare, "—") {
		t.Fatalf("blank output should not add an excerpt: %q", bare)
	}
}
