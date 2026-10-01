package updater

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/lgldsilva/updash/internal/model"
)

func TestUpdateAgent_manualOnly(t *testing.T) {
	it := &model.Item{
		Name: "Cursor", Category: model.CatAgent,
		KeepPolicy: "manual reinstall / app update",
		Status:     model.StatusOutdated,
	}
	res := updateAgent(context.Background(), it, SilentOptions())
	if res.Success {
		t.Fatal("manual agent must not report success")
	}
	if !strings.HasPrefix(res.Error, "⊘ ") {
		t.Fatalf("error=%q", res.Error)
	}
	if it.Status != model.StatusOutdated {
		t.Fatalf("status=%v", it.Status)
	}
}

// Windsurf has no update channel: the default manual note applies and no
// command may run.
func TestUpdateAgent_defaultManualNote(t *testing.T) {
	prevRun := runUpdateCmd
	t.Cleanup(func() { runUpdateCmd = prevRun })
	runUpdateCmd = func(ctx context.Context, opts Options, name string, args ...string) (string, string, error) {
		t.Fatalf("manual agent must not run %s %v", name, args)
		return "", "", nil
	}
	it := &model.Item{Name: "Windsurf", Category: model.CatAgent}
	res := updateAgent(context.Background(), it, SilentOptions())
	if res.Success || !strings.Contains(res.Error, "manual") {
		t.Fatalf("%+v", res)
	}
}

// Crush's agent update targets the npm prefix that owns the PATH binary.
func TestUpdateAgent_crushPrefixAware(t *testing.T) {
	withCrushBinary(t, "/usr/local/bin/crush", "/home/u/.npm-global/lib/node_modules/@charmland/crush/run-crush.js")
	prevRun := runUpdateCmd
	t.Cleanup(func() { runUpdateCmd = prevRun })
	var got []string
	runUpdateCmd = func(ctx context.Context, opts Options, name string, args ...string) (string, string, error) {
		got = append([]string{name}, args...)
		return "added 1 package", "", nil
	}
	it := &model.Item{Name: "Crush", Category: model.CatAgent, Status: model.StatusOutdated, CurrentVer: "0.86.0"}
	res := updateAgent(context.Background(), it, SilentOptions())
	if !res.Success {
		t.Fatalf("crush update failed: %+v", res)
	}
	want := []string{"npm", "install", "-g", "--prefix", "/home/u/.npm-global",
		"--allow-scripts=@charmland/crush", "@charmland/crush@latest"}
	if !slices.Equal(got, want) {
		t.Fatalf("crush cmd = %v, want %v", got, want)
	}
	if it.Status != model.StatusDone {
		t.Fatalf("status = %v, want done", it.Status)
	}
}
