package upgrade

import "testing"

func TestShouldAutoUpgrade(t *testing.T) {
	if !ShouldAutoUpgrade("v1.0.0", false) {
		t.Fatal("expected auto upgrade enabled")
	}
	if ShouldAutoUpgrade("v1.0.0", true) {
		t.Fatal("skip flag should disable")
	}
}

func TestFormatBuild(t *testing.T) {
	got := FormatBuild("841d04d")
	if got == "" || got == "dev" {
		t.Fatalf("FormatBuild = %q", got)
	}
}

func TestModeSkipsStartupUpgrade(t *testing.T) {
	if !ModeSkipsStartupUpgrade("upgrade") {
		t.Fatal("upgrade mode should skip")
	}
	if !ModeSkipsStartupUpgrade("env-defaults") {
		t.Fatal("env-defaults mode should skip")
	}
	if ModeSkipsStartupUpgrade("tui") {
		t.Fatal("tui mode should not skip")
	}
	for _, m := range []string{"check", "update", "clean", "all"} {
		if !ModeSkipsStartupUpgrade(m) {
			t.Fatalf("headless mode %q must skip startup upgrade (automation output stays parseable)", m)
		}
	}
}
