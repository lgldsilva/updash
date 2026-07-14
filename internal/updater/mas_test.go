package updater

import "testing"

func TestNormalizeMASName(t *testing.T) {
	// U+200E LEFT-TO-RIGHT MARK often prefixes mas app names
	got := normalizeMASName("\u200eWhatsApp")
	if got != "WhatsApp" {
		t.Fatalf("got %q", got)
	}
}