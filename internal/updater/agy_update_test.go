package updater

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/lgldsilva/updash/internal/model"
	"github.com/lgldsilva/updash/internal/scanner"
)

func TestUpdateAgent_AgyUsesGuardedCommand(t *testing.T) {
	prev := runUpdateCmd
	t.Cleanup(func() { runUpdateCmd = prev })

	var got []string
	hadDeadline := false
	runUpdateCmd = func(ctx context.Context, opts Options, name string, args ...string) (string, string, error) {
		_, hadDeadline = ctx.Deadline()
		got = append([]string{name}, args...)
		return "✓ You are already on the latest version.", "", nil
	}

	it := &model.Item{Name: "Agy", Category: model.CatAgent, Status: model.StatusInfo, CurrentVer: "1.2.16"}
	res := updateAgent(context.Background(), it, SilentOptions())
	if !res.Success || !hadDeadline {
		t.Fatalf("success=%v deadline=%v err=%q", res.Success, hadDeadline, res.Error)
	}
	if !slices.Equal(got, []string{"agy", "update"}) {
		t.Fatalf("cmd = %v, want [agy update]", got)
	}
	if !strings.Contains(res.Output, "agy update: "+scanner.AgyUpdateCurrent) {
		t.Fatalf("output = %q", res.Output)
	}
	if it.Status != model.StatusDone {
		t.Fatalf("status = %v, want done", it.Status)
	}
}

func TestUpdateAgent_AgySuccessfulInstall(t *testing.T) {
	prev := runUpdateCmd
	t.Cleanup(func() { runUpdateCmd = prev })
	runUpdateCmd = func(context.Context, Options, string, ...string) (string, string, error) {
		return "✓ Update successful!", "", nil
	}
	it := &model.Item{Name: "Agy", Category: model.CatAgent, Status: model.StatusInfo, CurrentVer: "1.2.13"}
	res := updateAgent(context.Background(), it, SilentOptions())
	if !res.Success || !strings.Contains(res.Output, "agy update: "+scanner.AgyUpdateUpdated) {
		t.Fatalf("%+v", res)
	}
}

func TestUpdateAgent_AgyUnknownZeroExitStillSucceeds(t *testing.T) {
	prev := runUpdateCmd
	t.Cleanup(func() { runUpdateCmd = prev })
	runUpdateCmd = func(context.Context, Options, string, ...string) (string, string, error) {
		return "⟳ Downloading update...", "", nil
	}
	it := &model.Item{Name: "Agy", Category: model.CatAgent, Status: model.StatusInfo, CurrentVer: "1.2.16"}
	res := updateAgent(context.Background(), it, SilentOptions())
	if !res.Success || res.Output != "⟳ Downloading update..." {
		t.Fatalf("unrecognized zero exit must stay success without a fake label: %+v", res)
	}
}

func TestUpdateAgent_AgyNonZeroIsFailure(t *testing.T) {
	prev := runUpdateCmd
	t.Cleanup(func() { runUpdateCmd = prev })
	runUpdateCmd = func(context.Context, Options, string, ...string) (string, string, error) {
		return "", "boom", errors.New("exit status 1")
	}
	it := &model.Item{Name: "Agy", Category: model.CatAgent, Status: model.StatusInfo, CurrentVer: "1.2.16"}
	res := updateAgent(context.Background(), it, SilentOptions())
	if res.Success || it.Status != model.StatusError {
		t.Fatalf("non-zero agy update must fail: %+v status=%v", res, it.Status)
	}
}
