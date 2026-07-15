package cli

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/lgldsilva/updash/internal/cleaner"
	"github.com/lgldsilva/updash/internal/elevate"
	"github.com/lgldsilva/updash/internal/model"
	"github.com/lgldsilva/updash/internal/updater"
)

func restoreHooks(t *testing.T) {
	t.Helper()
	od, os, oc, ou, op, oe, on, opm, of, oi := detectPlatform, runScannerAll, cleanOneFn, updateCategory, primeMacSudo, canElevateNP, nativeMacAvail, promptMacSess, formatBytesFn, stdinIsTTYFn
	t.Cleanup(func() {
		detectPlatform, runScannerAll, cleanOneFn, updateCategory = od, os, oc, ou
		primeMacSudo, canElevateNP, nativeMacAvail, promptMacSess = op, oe, on, opm
		formatBytesFn, stdinIsTTYFn = of, oi
	})
}

func fakeScan(updates, cleanup []*model.SourceSummary) {
	runScannerAll = func(ctx context.Context, plat model.PlatformInfo, withCleanup bool) []*model.SourceSummary {
		var all []*model.SourceSummary
		all = append(all, updates...)
		if withCleanup {
			all = append(all, cleanup...)
		}
		return all
	}
	detectPlatform = func() model.PlatformInfo { return model.PlatformInfo{OS: "linux", Distro: "ubuntu"} }
}

func TestScanSplitsCategories(t *testing.T) {
	restoreHooks(t)
	up := &model.SourceSummary{Category: model.CatBrew, Label: "Brew", Items: []*model.Item{{Name: "git", Status: model.StatusOutdated}}}
	cl := &model.SourceSummary{Category: model.CatCache, Label: "Cache", Items: []*model.Item{{Name: "npm-cache", Status: model.StatusCleanCandidate}}}
	fakeScan([]*model.SourceSummary{up}, []*model.SourceSummary{cl})

	updates, cleanup, elapsed, err := Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(updates) != 1 || len(cleanup) != 1 {
		t.Fatalf("updates=%d cleanup=%d", len(updates), len(cleanup))
	}
	if elapsed < 0 {
		t.Fatal("elapsed")
	}
}

func TestRunCheck(t *testing.T) {
	restoreHooks(t)
	fakeScan([]*model.SourceSummary{
		{Category: model.CatBrew, Icon: "🍺", Label: "Brew", Outdated: 0, Items: nil},
	}, nil)
	out := captureStdout(t, func() {
		if err := RunCheck(context.Background()); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(out, "Scanning") {
		t.Fatalf("%q", out)
	}
}

func TestRunUpdate_emptyAndDryRun(t *testing.T) {
	restoreHooks(t)
	// Nothing outdated
	fakeScan([]*model.SourceSummary{
		{Category: model.CatBrew, Label: "Brew", Items: []*model.Item{{Name: "ok", Status: model.StatusOK}}},
	}, nil)
	out := captureStdout(t, func() {
		ok, fail, err := RunUpdate(context.Background(), Config{})
		if err != nil || ok != 0 || fail != 0 {
			t.Fatalf("ok=%d fail=%d err=%v", ok, fail, err)
		}
	})
	if !strings.Contains(out, "Nothing to update") {
		t.Fatalf("%q", out)
	}

	// Dry-run with outdated
	item := &model.Item{Name: "git", Status: model.StatusOutdated, Category: model.CatBrew}
	fakeScan([]*model.SourceSummary{
		{Category: model.CatBrew, Label: "Brew", Items: []*model.Item{item}},
	}, nil)
	out = captureStdout(t, func() {
		ok, fail, err := RunUpdate(context.Background(), Config{DryRun: true})
		if err != nil || ok != 0 || fail != 0 {
			t.Fatalf("ok=%d fail=%d err=%v", ok, fail, err)
		}
	})
	if !strings.Contains(out, "dry-run") || !strings.Contains(out, "git") {
		t.Fatalf("%q", out)
	}
}

func TestRunUpdate_withBatches(t *testing.T) {
	restoreHooks(t)
	item := &model.Item{Name: "ripgrep", Status: model.StatusOutdated, Category: model.CatBrew}
	sum := &model.SourceSummary{Category: model.CatBrew, Icon: "🍺", Label: "Brew", Items: []*model.Item{item}}
	fakeScan([]*model.SourceSummary{sum}, nil)
	// After update, verify scan shows clean
	call := 0
	runScannerAll = func(ctx context.Context, plat model.PlatformInfo, withCleanup bool) []*model.SourceSummary {
		call++
		if call == 1 {
			return []*model.SourceSummary{sum}
		}
		return []*model.SourceSummary{{
			Category: model.CatBrew, Items: []*model.Item{{Name: "ripgrep", Status: model.StatusOK, Category: model.CatBrew}},
		}}
	}
	updateCategory = func(ctx context.Context, cat model.Category, items []*model.Item, opts updater.Options) []*updater.Result {
		return []*updater.Result{{Item: items[0], Success: true}}
	}
	canElevateNP = func(ctx context.Context) bool { return true }

	out := captureStdout(t, func() {
		ok, fail, err := RunUpdate(context.Background(), Config{SkipPassword: true})
		if err != nil || ok != 1 || fail != 0 {
			t.Fatalf("ok=%d fail=%d err=%v", ok, fail, err)
		}
	})
	if !strings.Contains(out, "Updating") {
		t.Fatalf("%q", out)
	}
}

func TestRunUpdate_strictRemaining(t *testing.T) {
	restoreHooks(t)
	item := &model.Item{Name: "git", Status: model.StatusOutdated, Category: model.CatNpm}
	sum := &model.SourceSummary{Category: model.CatNpm, Label: "npm", Items: []*model.Item{item}}
	fakeScan([]*model.SourceSummary{sum}, nil)
	updateCategory = func(ctx context.Context, cat model.Category, items []*model.Item, opts updater.Options) []*updater.Result {
		return []*updater.Result{{Item: items[0], Success: false, Error: "nope"}}
	}
	// verify still outdated
	runScannerAll = func(ctx context.Context, plat model.PlatformInfo, withCleanup bool) []*model.SourceSummary {
		return []*model.SourceSummary{sum}
	}
	detectPlatform = func() model.PlatformInfo { return model.PlatformInfo{OS: "linux"} }

	captureStdout(t, func() {
		_, fail, err := RunUpdate(context.Background(), Config{Strict: true})
		if err == nil {
			t.Fatal("expected error")
		}
		if fail != 1 {
			t.Fatalf("fail=%d", fail)
		}
	})
}

func TestRunClean_emptyDryRunAndSuccess(t *testing.T) {
	restoreHooks(t)
	fakeScan(nil, []*model.SourceSummary{
		{Category: model.CatCache, Label: "Cache", Items: nil},
	})
	out := captureStdout(t, func() {
		ok, fail, err := RunClean(context.Background(), Config{})
		if err != nil || ok != 0 || fail != 0 {
			t.Fatalf("ok=%d fail=%d err=%v", ok, fail, err)
		}
	})
	if !strings.Contains(out, "Nothing to clean") {
		t.Fatalf("%q", out)
	}

	cleanItem := &model.Item{Name: "npm-cache", Status: model.StatusCleanCandidate, Category: model.CatCache}
	fakeScan(nil, []*model.SourceSummary{
		{Category: model.CatCache, Icon: "🧹", Label: "Cache", Items: []*model.Item{cleanItem}},
	})
	out = captureStdout(t, func() {
		ok, fail, err := RunClean(context.Background(), Config{DryRun: true})
		if err != nil || ok != 0 || fail != 0 {
			t.Fatalf("%v", err)
		}
	})
	if !strings.Contains(out, "dry-run") {
		t.Fatalf("%q", out)
	}

	// Real clean path with mocked cleaner
	fakeScan(nil, []*model.SourceSummary{
		{Category: model.CatCache, Icon: "🧹", Label: "Cache", Items: []*model.Item{cleanItem}},
	})
	cleanOneFn = func(ctx context.Context, item *model.Item, opts cleaner.Options) *cleaner.Result {
		return &cleaner.Result{Item: item, Success: true, BytesFreed: 1024}
	}
	formatBytesFn = func(n int64) string { return "1KB" }
	canElevateNP = func(ctx context.Context) bool { return false }

	out = captureStdout(t, func() {
		ok, fail, err := RunClean(context.Background(), Config{})
		if err != nil || ok != 1 || fail != 0 {
			t.Fatalf("ok=%d fail=%d err=%v", ok, fail, err)
		}
	})
	if !strings.Contains(out, "freed") {
		t.Fatalf("%q", out)
	}
}

func TestRunClean_fail(t *testing.T) {
	restoreHooks(t)
	cleanItem := &model.Item{Name: "x", Status: model.StatusCleanCandidate, Category: model.CatCache}
	fakeScan(nil, []*model.SourceSummary{
		{Category: model.CatCache, Label: "Cache", Items: []*model.Item{cleanItem}},
	})
	cleanOneFn = func(ctx context.Context, item *model.Item, opts cleaner.Options) *cleaner.Result {
		return &cleaner.Result{Item: item, Success: false, Error: ""}
	}
	captureStdout(t, func() {
		_, fail, err := RunClean(context.Background(), Config{})
		if err == nil || fail != 1 {
			t.Fatalf("fail=%d err=%v", fail, err)
		}
	})
}

func TestRunAll(t *testing.T) {
	restoreHooks(t)
	// Nothing to do
	fakeScan(nil, nil)
	out := captureStdout(t, func() {
		if err := RunAll(context.Background(), Config{}); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(out, "up to date and clean") && !strings.Contains(out, "Nothing") {
		t.Fatalf("%q", out)
	}
}

func TestRunOneClean_successPaths(t *testing.T) {
	restoreHooks(t)
	cleanOneFn = func(ctx context.Context, item *model.Item, opts cleaner.Options) *cleaner.Result {
		return &cleaner.Result{Item: item, Success: true, BytesFreed: 0}
	}
	out := captureStdout(t, func() {
		ok, fail, freed := runOneClean(context.Background(), &model.Item{Name: "a"}, cleaner.Options{})
		if ok != 1 || fail != 0 || freed != 0 {
			t.Fatalf("ok=%d fail=%d freed=%d", ok, fail, freed)
		}
	})
	if !strings.Contains(out, "nothing to remove") {
		t.Fatalf("%q", out)
	}

	cleanOneFn = func(ctx context.Context, item *model.Item, opts cleaner.Options) *cleaner.Result {
		return &cleaner.Result{Item: item, Success: true, BytesFreed: 2048}
	}
	formatBytesFn = func(n int64) string { return "2KB" }
	out = captureStdout(t, func() {
		ok, _, freed := runOneClean(context.Background(), &model.Item{Name: "b"}, cleaner.Options{})
		if ok != 1 || freed != 2048 {
			t.Fatalf("ok=%d freed=%d", ok, freed)
		}
	})
	if !strings.Contains(out, "2KB") {
		t.Fatalf("%q", out)
	}
}

func TestRunNativeElevatedItems(t *testing.T) {
	restoreHooks(t)
	var sess *elevate.Session
	items := []*model.Item{
		{Name: "microsoft-office", Category: model.CatBrew},
		{Name: "app", Category: model.CatMAS},
	}
	stdinIsTTYFn = func() bool { return false }

	// cancelled
	primeMacSudo = func(ctx context.Context) error { return elevate.ErrDialogCancelled }
	res := runNativeElevatedItems(context.Background(), model.PlatformInfo{OS: "darwin"}, items, updater.Options{}, Config{}, &sess)
	if len(res) != 2 || !strings.Contains(res[0].Error, "⊘") {
		t.Fatalf("%+v", res)
	}

	// other error
	primeMacSudo = func(ctx context.Context) error { return errors.New("boom") }
	res = runNativeElevatedItems(context.Background(), model.PlatformInfo{OS: "darwin"}, items, updater.Options{}, Config{}, &sess)
	if len(res) != 2 || res[0].Error != "boom" {
		t.Fatalf("%+v", res)
	}

	// success
	primeMacSudo = func(ctx context.Context) error { return nil }
	updateCategory = func(ctx context.Context, cat model.Category, items []*model.Item, opts updater.Options) []*updater.Result {
		out := make([]*updater.Result, len(items))
		for i, it := range items {
			out[i] = &updater.Result{Item: it, Success: true}
		}
		return out
	}
	stdinIsTTYFn = func() bool { return true }
	sess = nil
	res = runNativeElevatedItems(context.Background(), model.PlatformInfo{OS: "darwin"}, items, updater.Options{}, Config{}, &sess)
	if len(res) != 2 || !res[0].Success || sess == nil {
		t.Fatalf("res=%+v sess=%v", res, sess)
	}
}

func TestRunNativeUpdateSection_withItems(t *testing.T) {
	restoreHooks(t)
	primeMacSudo = func(ctx context.Context) error { return elevate.ErrDialogCancelled }
	stdinIsTTYFn = func() bool { return true }
	var sess *elevate.Session
	env := updateBatchEnv{
		plat:        model.PlatformInfo{OS: "darwin"},
		cfg:         Config{},
		elevSession: &sess,
	}
	items := []*model.Item{{Name: "app", Category: model.CatMAS}}
	out := captureStdout(t, func() {
		_, _, skipped, _ := runNativeUpdateSection(context.Background(), env, items)
		if skipped != 1 {
			t.Fatalf("skipped=%d", skipped)
		}
	})
	if !strings.Contains(out, "privilegiadas") {
		t.Fatalf("%q", out)
	}
}

func TestPrimeElevationSession_passwordlessAndPrompt(t *testing.T) {
	restoreHooks(t)
	ctx := context.Background()
	var sess *elevate.Session
	items := []*model.Item{{Name: "curl", Category: model.CatApt}}

	canElevateNP = func(context.Context) bool { return true }
	got := primeElevationSession(ctx, model.PlatformInfo{OS: "linux"}, items, Config{}, &sess)
	if elevate.FromContext(got) == nil || sess == nil {
		t.Fatal("expected passwordless session")
	}

	// darwin native defers to native sheet
	sess = nil
	canElevateNP = func(context.Context) bool { return false }
	nativeMacAvail = func() bool { return true }
	got = primeElevationSession(ctx, model.PlatformInfo{OS: "darwin"}, items, Config{}, &sess)
	if got != ctx {
		t.Fatal("darwin native should defer")
	}

	// prompt errors
	nativeMacAvail = func() bool { return false }
	for _, e := range []error{elevate.ErrDialogCancelled, elevate.ErrDialogUnavailable, errors.New("bad")} {
		promptMacSess = func(ctx context.Context, reason string) (*elevate.Session, error) {
			return nil, e
		}
		got = primeElevationSession(ctx, model.PlatformInfo{OS: "linux"}, items, Config{}, &sess)
		if got != ctx {
			t.Fatalf("err %v should leave ctx", e)
		}
	}

	// prompt success
	s := elevate.NewSession()
	s.SetPasswordless()
	promptMacSess = func(ctx context.Context, reason string) (*elevate.Session, error) {
		return s, nil
	}
	got = primeElevationSession(ctx, model.PlatformInfo{OS: "linux"}, items, Config{}, &sess)
	if elevate.FromContext(got) != s {
		t.Fatal("expected prompted session")
	}
}

func TestRunBrewUpdateBatch_plain(t *testing.T) {
	restoreHooks(t)
	updateCategory = func(ctx context.Context, cat model.Category, items []*model.Item, opts updater.Options) []*updater.Result {
		return []*updater.Result{{Item: items[0], Success: true}}
	}
	var sess *elevate.Session
	res := runBrewUpdateBatch(context.Background(), []*model.Item{{Name: "wget", Category: model.CatBrew}}, updater.Options{}, Config{}, &sess)
	if len(res) != 1 || !res[0].Success {
		t.Fatalf("%+v", res)
	}
}

func TestRunCategoryUpdateSection_updatePath(t *testing.T) {
	restoreHooks(t)
	updateCategory = func(ctx context.Context, cat model.Category, items []*model.Item, opts updater.Options) []*updater.Result {
		return []*updater.Result{{Item: items[0], Success: true}}
	}
	var sess *elevate.Session
	env := updateBatchEnv{
		plat: model.PlatformInfo{OS: "linux"},
		summaries: []*model.SourceSummary{
			{Category: model.CatNpm, Icon: "📦", Label: "npm"},
		},
		elevSession: &sess,
	}
	out := captureStdout(t, func() {
		ok, _, _, _ := runCategoryUpdateSection(context.Background(), env, model.CatNpm, []*model.Item{
			{Name: "left-pad", Category: model.CatNpm},
		})
		if ok != 1 {
			t.Fatalf("ok=%d", ok)
		}
	})
	if !strings.Contains(out, "npm") {
		t.Fatalf("%q", out)
	}
}

func TestPrepareCleanElevation_passwordless(t *testing.T) {
	restoreHooks(t)
	canElevateNP = func(context.Context) bool { return true }
	ctx := context.Background()
	got := prepareCleanElevation(ctx, model.PlatformInfo{OS: "linux"}, []*model.Item{
		{Name: "apt-cache", Category: model.CatCache},
	}, false)
	if elevate.FromContext(got) == nil {
		t.Fatal("expected session")
	}
}

func TestShouldUseNativeMacAuth_available(t *testing.T) {
	restoreHooks(t)
	nativeMacAvail = func() bool { return true }
	if !shouldUseNativeMacAuth(model.PlatformInfo{OS: "darwin"}, []*model.Item{
		{Name: "app", Category: model.CatMAS},
	}, Config{}) {
		t.Fatal("expected true")
	}
}

func TestRunUpdateBatches_linux(t *testing.T) {
	restoreHooks(t)
	updateCategory = func(ctx context.Context, cat model.Category, items []*model.Item, opts updater.Options) []*updater.Result {
		return []*updater.Result{{Item: items[0], Success: true}}
	}
	canElevateNP = func(context.Context) bool { return true }
	out := captureStdout(t, func() {
		ok, _, _, _ := runUpdateBatches(
			context.Background(),
			model.PlatformInfo{OS: "linux"},
			[]*model.SourceSummary{{Category: model.CatNpm, Icon: "n", Label: "npm"}},
			[]*model.Item{{Name: "x", Category: model.CatNpm}},
			updater.Options{},
			Config{},
		)
		if ok != 1 {
			t.Fatalf("ok=%d", ok)
		}
	})
	_ = out
}
