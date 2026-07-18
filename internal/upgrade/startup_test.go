package upgrade

import (
	"errors"
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
	if ModeSkipsStartupUpgrade("tui") {
		t.Fatal("tui mode should not skip")
	}
	for _, m := range []string{"check", "update", "clean", "all"} {
		if !ModeSkipsStartupUpgrade(m) {
			t.Fatalf("headless mode %q must skip startup upgrade (automation output stays parseable)", m)
		}
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

// stubSelfUpdateDeps swaps the OS-level helpers used by canSelfUpdate.
func stubSelfUpdateDeps(t *testing.T, executable, home string, execErr, symlinkErr, homeErr error) {
	t.Helper()
	oldExec, oldSymlink, oldHome := osExecutable, evalSymlinks, osUserHomeDir
	t.Cleanup(func() { osExecutable, evalSymlinks, osUserHomeDir = oldExec, oldSymlink, oldHome })
	osExecutable = func() (string, error) { return executable, execErr }
	evalSymlinks = func(p string) (string, error) { return p, symlinkErr }
	osUserHomeDir = func() (string, error) { return home, homeErr }
}

func TestCanSelfUpdate(t *testing.T) {
	t.Run("env override wins without touching the filesystem", func(t *testing.T) {
		t.Setenv("UPDASH_ALLOW_SELF_UPDATE", "1")
		stubSelfUpdateDeps(t, "", "", errors.New("nope"), errors.New("nope"), errors.New("nope"))
		if !canSelfUpdate() {
			t.Fatal("UPDASH_ALLOW_SELF_UPDATE=1 must allow self-update")
		}
	})

	t.Run("user-local install allows self-update", func(t *testing.T) {
		home := t.TempDir()
		stubSelfUpdateDeps(t, filepath.Join(home, ".local", "bin", "updash"), home, nil, nil, nil)
		if !canSelfUpdate() {
			t.Fatal("binary in ~/.local/bin must allow self-update")
		}
	})

	t.Run("system install is package-managed", func(t *testing.T) {
		stubSelfUpdateDeps(t, "/usr/bin/updash", t.TempDir(), nil, nil, nil)
		if canSelfUpdate() {
			t.Fatal("binary in /usr/bin must not self-update")
		}
	})

	t.Run("executable resolution failure denies self-update", func(t *testing.T) {
		stubSelfUpdateDeps(t, "", t.TempDir(), errors.New("no exe"), nil, nil)
		if canSelfUpdate() {
			t.Fatal("must deny when os.Executable fails")
		}
	})

	t.Run("symlink resolution failure denies self-update", func(t *testing.T) {
		stubSelfUpdateDeps(t, "/tmp/updash", t.TempDir(), nil, errors.New("broken link"), nil)
		if canSelfUpdate() {
			t.Fatal("must deny when EvalSymlinks fails")
		}
	})

	t.Run("home resolution failure denies self-update", func(t *testing.T) {
		stubSelfUpdateDeps(t, "/tmp/updash", "", nil, nil, errors.New("no home"))
		if canSelfUpdate() {
			t.Fatal("must deny when UserHomeDir fails")
		}
	})
}
