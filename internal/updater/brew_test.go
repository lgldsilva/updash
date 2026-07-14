package updater

import (
	"context"
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
