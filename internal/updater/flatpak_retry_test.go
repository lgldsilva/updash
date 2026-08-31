package updater

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/lgldsilva/updash/internal/model"
)

func flatpakPlan() CommandPlan {
	return CommandPlan{Name: "flatpak", Args: []string{"update", "-y"}, Scope: CommandScopeCategoryGlobal}
}

func TestFlatpakRetryPlan(t *testing.T) {
	deltaFail := "Error: While pulling app/org.qbittorrent.qBittorrent/x86_64/stable from remote flathub: Decompressed delta part exceeds configured limit of 19030406 bytes"
	next, reason, ok := flatpakRetryPlan(flatpakPlan(), deltaFail)
	if !ok || !slices.Contains(next.Args, flatpakNoDeltasFlag) || next.Elevated {
		t.Fatalf("delta failure must retry without static deltas: %+v (%q)", next, reason)
	}

	// Same failure again with the flag already applied: no further retry.
	if _, _, ok := flatpakRetryPlan(next, deltaFail); ok {
		t.Fatal("must not retry the same flag twice")
	}

	denied := "Error: Flatpak system operation Deploy not allowed for user"
	elevated, _, ok := flatpakRetryPlan(next, denied)
	if !ok || !elevated.Elevated || !slices.Contains(elevated.Args, flatpakNoDeltasFlag) {
		t.Fatalf("permission failure must retry elevated, keeping earlier flags: %+v", elevated)
	}
	if _, _, ok := flatpakRetryPlan(elevated, denied); ok {
		t.Fatal("must not retry elevation twice")
	}

	// An unrelated failure is not retried.
	if _, _, ok := flatpakRetryPlan(flatpakPlan(), "Error: no space left on device"); ok {
		t.Fatal("unknown failures must not be retried")
	}

	// The original plan is never mutated in place.
	base := flatpakPlan()
	if _, _, _ = flatpakRetryPlan(base, deltaFail); slices.Contains(base.Args, flatpakNoDeltasFlag) {
		t.Fatal("retry must not mutate the original plan")
	}
}

// End-to-end: the broken delta then the permission denial, recovering on the
// elevated retry — the exact sequence seen on the real machine.
func TestExecutePreparedFlatpak_RecoversThroughBothRetries(t *testing.T) {
	var dispatched []string
	prevRun, prevElevated := runUpdateCmd, runElevatedUpdateCmd
	runUpdateCmd = func(_ context.Context, _ Options, name string, args ...string) (string, string, error) {
		dispatched = append(dispatched, strings.Join(append([]string{name}, args...), " "))
		if slices.Contains(args, flatpakNoDeltasFlag) {
			return "", "Error: Flatpak system operation Deploy not allowed for user", errors.New("exit status 1")
		}
		return "", "Error: Decompressed delta part exceeds configured limit of 19030406 bytes", errors.New("exit status 1")
	}
	runElevatedUpdateCmd = func(_ context.Context, _ Options, name string, args ...string) (string, string, error) {
		dispatched = append(dispatched, "sudo "+strings.Join(append([]string{name}, args...), " "))
		return "Updates complete.", "", nil
	}
	t.Cleanup(func() { runUpdateCmd, runElevatedUpdateCmd = prevRun, prevElevated })

	item := &model.Item{Name: "org.qbittorrent.qBittorrent", Category: model.CatFlatpak}
	results := executePreparedFlatpak(context.Background(), []*model.Item{item}, []CommandPlan{flatpakPlan()}, SilentOptions())

	if len(results) != 1 || !results[0].Success || item.Status != model.StatusDone {
		t.Fatalf("expected recovery, got %+v (status %v)", results[0], item.Status)
	}
	want := []string{
		"flatpak update -y",
		"flatpak update -y " + flatpakNoDeltasFlag,
		"sudo flatpak update -y " + flatpakNoDeltasFlag,
	}
	if !slices.Equal(dispatched, want) {
		t.Fatalf("dispatched %v, want %v", dispatched, want)
	}
	// The user can see why the command changed.
	if !strings.Contains(results[0].Output, "retrying") || !strings.Contains(results[0].Output, "Updates complete.") {
		t.Fatalf("output must explain the retries and keep the final output: %q", results[0].Output)
	}
}

// A failure with no known cause is reported as-is, without extra attempts.
func TestExecutePreparedFlatpak_UnknownFailureIsNotRetried(t *testing.T) {
	calls := 0
	prevRun := runUpdateCmd
	runUpdateCmd = func(context.Context, Options, string, ...string) (string, string, error) {
		calls++
		return "", "Error: no space left on device", errors.New("exit status 1")
	}
	t.Cleanup(func() { runUpdateCmd = prevRun })

	item := &model.Item{Name: "org.example.App", Category: model.CatFlatpak}
	results := executePreparedFlatpak(context.Background(), []*model.Item{item}, []CommandPlan{flatpakPlan()}, SilentOptions())
	if calls != 1 {
		t.Fatalf("calls = %d, want a single attempt", calls)
	}
	if results[0].Success {
		t.Fatal("unknown failure must stay a failure")
	}
}

// A malformed batch falls back to the generic path instead of panicking.
func TestExecutePreparedFlatpak_FallsBackOnPlanMismatch(t *testing.T) {
	items := []*model.Item{{Name: "a", Category: model.CatFlatpak}, {Name: "b", Category: model.CatFlatpak}}
	results := executePreparedFlatpak(context.Background(), items, nil, SilentOptions())
	if len(results) != len(items) {
		t.Fatalf("results = %d, want %d", len(results), len(items))
	}
}

func TestCommandText(t *testing.T) {
	if got := commandText(flatpakPlan()); got != "flatpak update -y" {
		t.Fatalf("got %q", got)
	}
	elevated := flatpakPlan()
	elevated.Elevated = true
	if got := commandText(elevated); got != "sudo flatpak update -y" {
		t.Fatalf("got %q", got)
	}
}
