package upgrade

import (
	"path/filepath"
	"testing"
)

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
	if ModeSkipsStartupUpgrade("check") {
		t.Fatal("check mode should not skip")
	}
}

func TestSelfUpdateAllowed(t *testing.T) {
	home := t.TempDir()
	if !selfUpdateAllowed(filepath.Join(home, ".local", "bin", "updash"), home) {
		t.Fatal("expected a user-installed binary to allow self-update")
	}
	if selfUpdateAllowed("/usr/bin/updash", home) {
		t.Fatal("system package binaries must be updated by their package manager")
	}
}
