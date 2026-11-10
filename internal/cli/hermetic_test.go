package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/lgldsilva/updash/internal/cleaner"
	"github.com/lgldsilva/updash/internal/elevate"
	"github.com/lgldsilva/updash/internal/model"
	"github.com/lgldsilva/updash/internal/updater"
)

// The cli hooks default to the real scanner, cleaner, updater and sudo. A test
// that forgets to stub one must fail loudly instead of scanning, pruning,
// upgrading or probing sudo on the host running `go test`, so TestMain swaps
// every side-effecting hook for a panic (or a host-independent default).
func TestMain(m *testing.M) {
	detectPlatform = func() model.PlatformInfo { return model.PlatformInfo{OS: "linux", Distro: "ubuntu"} }
	runScannerAll = func(context.Context, model.PlatformInfo, bool) []*model.SourceSummary {
		panic(unmocked("runScannerAll"))
	}
	runScannerFiltered = func(context.Context, model.PlatformInfo, bool, []model.Category) []*model.SourceSummary {
		panic(unmocked("runScannerFiltered"))
	}
	runScannerFilteredForCleanup = func(context.Context, model.PlatformInfo, []model.Category) []*model.SourceSummary {
		panic(unmocked("runScannerFilteredForCleanup"))
	}
	cleanOneFn = func(context.Context, *model.Item, cleaner.Options) *cleaner.Result { panic(unmocked("cleanOneFn")) }
	updateCategory = func(context.Context, model.Category, []*model.Item, updater.Options) []*updater.Result {
		panic(unmocked("updateCategory"))
	}
	executePreparedBatch = func(context.Context, *updater.PreparedUpdateBatch, updater.Options) []*updater.Result {
		panic(unmocked("executePreparedBatch"))
	}
	executeNpmSkippingElevated = func(context.Context, *updater.PreparedUpdateBatch, updater.Options, string) []*updater.Result {
		panic(unmocked("executeNpmSkippingElevated"))
	}
	// The real npm planner asks the host npm for its global prefix.
	planUpdateCommands = func(cat model.Category, items []*model.Item) ([]updater.CommandPlan, error) {
		if cat == model.CatNpm {
			return []updater.CommandPlan{{Name: "npm", Args: []string{"install", "-g"}, Scope: updater.CommandScopeExact}}, nil
		}
		return updater.PlanUpdateCommands(cat, items)
	}
	canElevateNP = func(context.Context) bool { return false }
	nativeMacAvail = func() bool { return false }
	promptMacSess = func(context.Context, string) (*elevate.Session, error) {
		return nil, errors.New("native macOS prompt is not mocked in this test")
	}
	stdinIsTTYFn = func() bool { return false }
	os.Exit(m.Run())
}

func unmocked(hook string) string {
	return fmt.Sprintf("cli test reached the real %s hook; stub it", hook)
}

// withRealUpdater lets an integration test drive the real updater against
// fixture binaries it put on PATH (the agy tests). Never use it without one.
func withRealUpdater(t *testing.T) {
	t.Helper()
	restoreHooks(t)
	updateCategory = updater.UpdateCategory
	executePreparedBatch = updater.ExecutePreparedBatch
	executeNpmSkippingElevated = updater.ExecuteNpmSkippingElevated
}

// stubFailedClean makes every cleanup fail without touching the host.
func stubFailedClean() {
	cleanOneFn = func(_ context.Context, it *model.Item, _ cleaner.Options) *cleaner.Result {
		return &cleaner.Result{Item: it, Error: "cleanup failed (stub)"}
	}
}

func TestPrepareCleanElevation_passwordlessOrInteractive(t *testing.T) {
	restoreHooks(t)
	items := []*model.Item{{Name: "apt-cache", Category: model.CatCache}}
	linux := model.PlatformInfo{OS: "linux"}

	canElevateNP = func(context.Context) bool { return true }
	sess := elevate.FromContext(prepareCleanElevation(context.Background(), linux, items, false))
	if sess == nil || !sess.Ready() {
		t.Fatal("NOPASSWD host must attach a passwordless session")
	}

	canElevateNP = func(context.Context) bool { return false }
	for _, interactive := range []bool{false, true} {
		ctx := context.Background()
		if got := prepareCleanElevation(ctx, linux, items, interactive); elevate.FromContext(got) != nil {
			t.Fatalf("interactive=%v: a password host must not get a session", interactive)
		}
	}
}
