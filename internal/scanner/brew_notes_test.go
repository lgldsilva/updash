package scanner

import "testing"

func TestBrewUpgradeNote_microsoft(t *testing.T) {
	if got := BrewUpgradeNote("microsoft-office"); got == "" {
		t.Fatal("expected note for microsoft-office")
	}
}

func TestBrewUpgradeNote_normalCask(t *testing.T) {
	if got := BrewUpgradeNote("telegram"); got != "" {
		t.Fatalf("telegram should have no note, got %q", got)
	}
}
