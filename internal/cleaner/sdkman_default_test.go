package cleaner

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lgldsilva/updash/internal/model"
)

func mkSDKCandidate(t *testing.T, home, candidate, current string, versions ...string) {
	t.Helper()
	base := filepath.Join(home, ".sdkman", "candidates", candidate)
	for _, v := range versions {
		if err := os.MkdirAll(filepath.Join(base, v), 0o750); err != nil {
			t.Fatal(err)
		}
	}
	if current != "" {
		if err := os.Symlink(filepath.Join(base, current), filepath.Join(base, "current")); err != nil {
			t.Fatal(err)
		}
	}
}

// Regression: `sdk default java 21.0.1-tem` with a newer 21.x installed must
// never uninstall the user's default JDK.
func TestCleanSDKMAN_KeepsDefaultVersion(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	mkSDKCandidate(t, home, "java", "21.0.1-tem", "21.0.1-tem", "21.0.2-tem", "21.0.3-tem")
	log := recordExec(t)

	item := &model.Item{Name: "java 21", Category: model.CatSDKMAN, CurrentVer: "21.0.3-tem"}
	r := cleanSDKMAN(context.Background(), item, SilentOptions())
	if !r.Success {
		t.Fatalf("expected success, got %+v", r)
	}
	got := log.joined()
	if !strings.Contains(got, "sdk uninstall java 21.0.2-tem") {
		t.Fatalf("expected 21.0.2-tem uninstall, got %q", got)
	}
	for _, kept := range []string{"21.0.1-tem", "21.0.3-tem"} {
		if strings.Contains(got, "uninstall java "+kept) {
			t.Fatalf("must keep %s, calls: %q", kept, got)
		}
	}
}

// Regression: an item without CurrentVer (stale/crafted) removed every
// version of the major. Fail closed instead.
func TestCleanSDKMAN_RefusesWithoutKeepVersion(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	mkSDKCandidate(t, home, "java", "", "21.0.1-tem", "21.0.2-tem")
	log := recordExec(t)

	item := &model.Item{Name: "java 21", Category: model.CatSDKMAN}
	r := cleanSDKMAN(context.Background(), item, SilentOptions())
	if r.Success || !strings.Contains(r.Error, "version to keep") {
		t.Fatalf("expected refusal, got %+v", r)
	}
	if n := len(log.calls()); n != 0 {
		t.Fatalf("expected no commands, got %v", log.calls())
	}
}

func TestCleanSDKMAN_UninstallFailureStopsAndReports(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	mkSDKCandidate(t, home, "java", "", "21.0.1-tem", "21.0.2-tem")
	recordExec(t)
	t.Setenv(fakeFailEnv, "sdk uninstall")

	item := &model.Item{Name: "java 21", Category: model.CatSDKMAN, CurrentVer: "21.0.2-tem"}
	r := cleanSDKMAN(context.Background(), item, SilentOptions())
	if r.Success || item.Status != model.StatusError || !strings.Contains(r.Error, "21.0.1-tem") {
		t.Fatalf("expected failure naming the version, got %+v", r)
	}
}

// Git Bash/MSYS copies `current` instead of linking it: the default is
// unknowable, so nothing may be uninstalled.
func TestCleanSDKMAN_RefusesWhenCurrentIsNotASymlink(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	mkSDKCandidate(t, home, "java", "", "21.0.1-tem", "21.0.3-tem", "current")
	log := recordExec(t)

	item := &model.Item{Name: "java 21", Category: model.CatSDKMAN, CurrentVer: "21.0.3-tem"}
	r := cleanSDKMAN(context.Background(), item, SilentOptions())
	if r.Success || !strings.Contains(r.Error, "sdk default") {
		t.Fatalf("expected refusal, got %+v", r)
	}
	if n := len(log.calls()); n != 0 {
		t.Fatalf("expected no commands, got %v", log.calls())
	}
}
