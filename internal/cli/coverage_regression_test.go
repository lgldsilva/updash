package cli

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/lgldsilva/updash/internal/model"
	"github.com/lgldsilva/updash/internal/updater"
)

func TestExitErrorContractAndPrecedence(t *testing.T) {
	cause := errors.New("scan failed")
	for _, tc := range []struct {
		name string
		err  error
		want int
	}{
		{name: "nil", err: nil, want: 0},
		{name: "ordinary", err: cause, want: 1},
		{name: "exit one", err: &ExitError{Code: 1, Err: cause}, want: 1},
		{name: "exit two wrapped", err: errors.Join(errors.New("context"), &ExitError{Code: 2, Err: cause}), want: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := ExitCode(tc.err); got != tc.want {
				t.Fatalf("ExitCode()=%d, want %d", got, tc.want)
			}
		})
	}

	exitErr := &ExitError{Code: 2, Err: cause}
	if got := exitErr.Error(); got != cause.Error() {
		t.Fatalf("Error()=%q, want %q", got, cause)
	}
	if !errors.Is(exitErr, cause) || exitErr.Unwrap() != cause {
		t.Fatal("ExitError must preserve its wrapped cause")
	}
}

func TestCountsIgnoreOtherStatuses(t *testing.T) {
	summaries := []*model.SourceSummary{
		{Items: []*model.Item{
			{Name: "outdated", Status: model.StatusOutdated},
			{Name: "clean", Status: model.StatusCleanCandidate},
			{Name: "info", Status: model.StatusInfo},
			nil,
		}},
	}
	if got := countOutdated(summaries); got != 1 {
		t.Fatalf("countOutdated=%d, want 1", got)
	}
	if got := countCleanable(summaries); got != 1 {
		t.Fatalf("countCleanable=%d, want 1", got)
	}
}

func TestAppendProblemsDoesNotDoubleCountItemStatuses(t *testing.T) {
	withItems := &model.SourceSummary{
		Category:   model.CatNpm,
		Label:      "npm",
		ErrorCount: 1,
		Unverified: 1,
		Items: []*model.Item{
			{Name: "failed probe", Category: model.CatNpm, Status: model.StatusError},
			{Name: "uncertain probe", Category: model.CatNpm, Status: model.StatusUnverified},
		},
	}
	withoutItems := &model.SourceSummary{
		Category:   model.CatPnpm,
		Label:      "pnpm",
		ErrorCount: 2,
		Unverified: 3,
	}
	var problems []ReportItem
	errs, unverified := appendProblems(&problems, []*model.SourceSummary{withItems, nil}, []*model.SourceSummary{withoutItems})
	if errs != 3 || unverified != 4 {
		t.Fatalf("appendProblems() errors=%d unverified=%d, want 3 and 4", errs, unverified)
	}
	if len(problems) != 4 {
		t.Fatalf("problems=%+v, want two item and two summary entries", problems)
	}
	if !summaryHasStatus(withItems, model.StatusError) || !summaryHasStatus(withItems, model.StatusUnverified) {
		t.Fatal("summaryHasStatus must find concrete problem items")
	}
	if summaryHasStatus(withoutItems, model.StatusError) {
		t.Fatal("summaryHasStatus must not infer status from summary counters")
	}
}

func TestScanForConfigOnlyUsesExactAvailableCategory(t *testing.T) {
	restoreHooks(t)
	detectPlatform = func() model.PlatformInfo { return model.PlatformInfo{OS: "linux", HasPnpm: true} }
	var got []model.Category
	runScannerFiltered = func(_ context.Context, _ model.PlatformInfo, includeCleanup bool, categories []model.Category) []*model.SourceSummary {
		if includeCleanup {
			t.Fatal("update scan must not include cleanup sources")
		}
		got = append(got, categories...)
		return []*model.SourceSummary{{Category: model.CatPnpm, Items: []*model.Item{{Name: "pkg", Status: model.StatusOutdated}}}}
	}

	updates, cleanup, _, err := scanForConfig(context.Background(), Config{Only: "PNPM"}, false)
	if err != nil || len(updates) != 1 || len(cleanup) != 0 {
		t.Fatalf("updates=%+v cleanup=%+v err=%v", updates, cleanup, err)
	}
	if len(got) != 1 || got[0] != model.CatPnpm {
		t.Fatalf("filtered categories=%v, want [pnpm]", got)
	}
}

func TestScanForConfigRejectsUnavailableOnlyBeforeScanning(t *testing.T) {
	restoreHooks(t)
	detectPlatform = func() model.PlatformInfo { return model.PlatformInfo{OS: "linux"} }
	called := false
	runScannerAll = func(context.Context, model.PlatformInfo, bool) []*model.SourceSummary {
		called = true
		return nil
	}

	_, _, _, err := scanForConfig(context.Background(), Config{Only: "not-a-source"}, true)
	if called {
		t.Fatal("invalid --only must fail before invoking a scanner")
	}
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 2 || ExitCode(err) != 2 {
		t.Fatalf("err=%v, want ExitError code 2", err)
	}
}

func TestRunCheckInconclusivePrecedesStrictFindings(t *testing.T) {
	restoreHooks(t)
	fakeScan([]*model.SourceSummary{{
		Category: model.CatNpm,
		Label:    "npm",
		Items: []*model.Item{
			{Name: "old", Category: model.CatNpm, Status: model.StatusOutdated},
			{Name: "unknown", Category: model.CatNpm, Status: model.StatusUnverified},
		},
	}}, nil)

	err := RunCheck(context.Background(), Config{Strict: true})
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || ExitCode(err) != 2 {
		t.Fatalf("RunCheck err=%v, want inconclusive code 2", err)
	}
}

func TestRunUpdateDryRunPrintsExactAndGlobalPlans(t *testing.T) {
	restoreHooks(t)
	apt := &model.Item{Name: "curl", Category: model.CatApt, Status: model.StatusOutdated}
	pnpm := &model.Item{Name: "eslint", Category: model.CatPnpm, Status: model.StatusOutdated}
	fakeScan([]*model.SourceSummary{
		{Category: model.CatApt, Label: "APT", Items: []*model.Item{apt}},
		{Category: model.CatPnpm, Label: "pnpm", Items: []*model.Item{pnpm}},
	}, nil)

	out := captureStdout(t, func() {
		ok, fail, err := RunUpdate(context.Background(), Config{DryRun: true})
		if err != nil || ok != 0 || fail != 0 {
			t.Fatalf("ok=%d fail=%d err=%v", ok, fail, err)
		}
	})
	for _, want := range []string{
		"[category-global] sudo apt-get update",
		"[exact] sudo apt-get install --only-upgrade -y curl",
		"[exact] pnpm update -g eslint",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("dry-run missing %q:\n%s", want, out)
		}
	}
}

func TestPartitionConclusiveKeepsSiblingsInSameSource(t *testing.T) {
	src := &model.SourceSummary{
		Category: model.CatAI,
		Label:    "AI Infra",
		Items: []*model.Item{
			{Name: "gcloud", Category: model.CatAI, Status: model.StatusError, CurrentVer: "error"},
			{Name: "semidx", Category: model.CatAI, Status: model.StatusOutdated},
			{Name: "ai-memory", Category: model.CatAI, Status: model.StatusOK},
		},
	}
	ok, skipped := partitionConclusive([]*model.SourceSummary{src})
	if len(ok) != 1 || len(skipped) != 1 {
		t.Fatalf("ok=%d skipped=%d", len(ok), len(skipped))
	}
	if len(ok[0].Items) != 2 || ok[0].ErrorCount != 0 {
		t.Fatalf("keep = %+v", ok[0])
	}
	if len(skipped[0].Items) != 1 || skipped[0].Items[0].Name != "gcloud" {
		t.Fatalf("skip = %+v", skipped[0])
	}
}

func TestRunUpdateSkipsBrokenItemKeepsSiblingsInSameSource(t *testing.T) {
	restoreHooks(t)
	gcloud := &model.Item{Name: "gcloud", Category: model.CatAI, Status: model.StatusError, CurrentVer: "error"}
	semidx := &model.Item{Name: "semidx", Category: model.CatAI, Status: model.StatusOutdated}
	src := &model.SourceSummary{Category: model.CatAI, Label: "AI Infra", Items: []*model.Item{gcloud, semidx}}
	verified := &model.SourceSummary{Category: model.CatAI, Label: "AI Infra", Items: []*model.Item{
		{Name: "gcloud", Category: model.CatAI, Status: model.StatusError, CurrentVer: "error"},
		{Name: "semidx", Category: model.CatAI, Status: model.StatusOK},
	}}
	calls := 0
	runScannerAll = func(context.Context, model.PlatformInfo, bool) []*model.SourceSummary {
		calls++
		if calls == 1 {
			return []*model.SourceSummary{src}
		}
		return []*model.SourceSummary{verified}
	}
	detectPlatform = func() model.PlatformInfo { return model.PlatformInfo{OS: "linux"} }
	executePreparedBatch = func(_ context.Context, batch *updater.PreparedUpdateBatch, _ updater.Options) []*updater.Result {
		if batch.Category() != model.CatAI || len(batch.Items()) != 1 || batch.Items()[0].Name != "semidx" {
			t.Fatalf("must only update semidx, got cat=%s items=%+v", batch.Category(), batch.Items())
		}
		return []*updater.Result{{Item: semidx, Success: true}}
	}

	out := captureStdout(t, func() {
		ok, fail, err := RunUpdate(context.Background(), Config{})
		var exitErr *ExitError
		if !errors.As(err, &exitErr) || ExitCode(err) != 2 {
			t.Fatalf("ok=%d fail=%d err=%v, want partial ExitError code 2", ok, fail, err)
		}
		if ok != 1 || fail != 0 {
			t.Fatalf("ok=%d fail=%d, want the conclusive sibling updated", ok, fail)
		}
	})
	if !strings.Contains(out, "skipped") || !strings.Contains(out, "gcloud") || !strings.Contains(out, "semidx") {
		t.Fatalf("output missing skip/update evidence:\n%s", out)
	}
}

func TestRunUpdateSkipsErrorSourceAndUpdatesConclusiveItems(t *testing.T) {
	restoreHooks(t)
	tool := &model.Item{Name: "semidx", Category: model.CatAI, Status: model.StatusOutdated}
	broken := &model.SourceSummary{
		Category: model.CatBun,
		Label:    "bun (global)",
		Items:    []*model.Item{{Name: "bun", Category: model.CatBun, Status: model.StatusError, CurrentVer: "error"}},
	}
	conclusive := &model.SourceSummary{Category: model.CatAI, Label: "AI Infra", Items: []*model.Item{tool}}
	verified := &model.SourceSummary{Category: model.CatAI, Label: "AI Infra", Items: []*model.Item{{Name: "semidx", Category: model.CatAI, Status: model.StatusOK}}}
	calls := 0
	runScannerAll = func(context.Context, model.PlatformInfo, bool) []*model.SourceSummary {
		calls++
		if calls == 1 {
			return []*model.SourceSummary{broken, conclusive}
		}
		return []*model.SourceSummary{broken, verified}
	}
	detectPlatform = func() model.PlatformInfo { return model.PlatformInfo{OS: "linux"} }
	executePreparedBatch = func(_ context.Context, batch *updater.PreparedUpdateBatch, _ updater.Options) []*updater.Result {
		if batch.Category() != model.CatAI {
			t.Fatalf("must only update the conclusive category, got %s", batch.Category())
		}
		return []*updater.Result{{Item: tool, Success: true}}
	}

	out := captureStdout(t, func() {
		ok, fail, err := RunUpdate(context.Background(), Config{})
		var exitErr *ExitError
		if !errors.As(err, &exitErr) || ExitCode(err) != 2 {
			t.Fatalf("ok=%d fail=%d err=%v, want partial ExitError code 2", ok, fail, err)
		}
		if ok != 1 || fail != 0 {
			t.Fatalf("ok=%d fail=%d, want the conclusive update applied", ok, fail)
		}
	})
	for _, want := range []string{"skipped", "bun (bun)", "semidx"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}
}

func TestRunUpdateProceedsDespiteUnverifiedVerification(t *testing.T) {
	restoreHooks(t)
	item := &model.Item{Name: "eslint", Category: model.CatPnpm, Status: model.StatusOutdated}
	first := &model.SourceSummary{Category: model.CatPnpm, Label: "pnpm", Items: []*model.Item{item}}
	calls := 0
	runScannerAll = func(context.Context, model.PlatformInfo, bool) []*model.SourceSummary {
		calls++
		if calls == 1 {
			return []*model.SourceSummary{first}
		}
		return []*model.SourceSummary{{Category: model.CatPnpm, Label: "pnpm", Unverified: 1}}
	}
	detectPlatform = func() model.PlatformInfo { return model.PlatformInfo{OS: "linux"} }
	executePreparedBatch = func(_ context.Context, batch *updater.PreparedUpdateBatch, _ updater.Options) []*updater.Result {
		if batch.Category() != model.CatPnpm || len(batch.Items()) != 1 {
			t.Fatalf("unexpected update batch: cat=%s items=%+v", batch.Category(), batch.Items())
		}
		return []*updater.Result{{Item: item, Success: true}}
	}

	out := captureStdout(t, func() {
		ok, fail, err := RunUpdate(context.Background(), Config{})
		var exitErr *ExitError
		if !errors.As(err, &exitErr) || ExitCode(err) != 2 || calls != 2 {
			t.Fatalf("ok=%d fail=%d err=%v calls=%d, want updated item plus partial ExitError after two scans", ok, fail, err, calls)
		}
		if ok != 1 || fail != 0 {
			t.Fatalf("ok=%d fail=%d, want the conclusive update applied", ok, fail)
		}
	})
	if !strings.Contains(out, "skipped") || !strings.Contains(out, "pnpm") {
		t.Fatalf("partial update must name the skipped source:\n%s", out)
	}
}

func TestRunUpdateExecutesThePreparedBatchWithoutReplanning(t *testing.T) {
	restoreHooks(t)
	item := &model.Item{Name: "eslint", Category: model.CatPnpm, Status: model.StatusOutdated}
	first := &model.SourceSummary{Category: model.CatPnpm, Label: "pnpm", Items: []*model.Item{item}}
	scans := 0
	runScannerAll = func(context.Context, model.PlatformInfo, bool) []*model.SourceSummary {
		scans++
		if scans == 1 {
			return []*model.SourceSummary{first}
		}
		return []*model.SourceSummary{{Category: model.CatPnpm, Label: "pnpm", Items: []*model.Item{{Name: item.Name, Category: item.Category, Status: model.StatusOK}}}}
	}
	detectPlatform = func() model.PlatformInfo { return model.PlatformInfo{OS: "linux"} }

	prepares := 0
	var prepared, executed *updater.PreparedUpdateBatch
	prepareUpdateBatch = func(ctx context.Context, cat model.Category, items []*model.Item) (*updater.PreparedUpdateBatch, error) {
		prepares++
		if prepares > 1 {
			t.Fatal("the CLI must not prepare an already approved batch again")
		}
		batch, err := updater.PrepareUpdateBatch(ctx, cat, items)
		if err != nil {
			t.Fatal(err)
		}
		prepared = batch
		return batch, nil
	}
	executePreparedBatch = func(_ context.Context, batch *updater.PreparedUpdateBatch, _ updater.Options) []*updater.Result {
		executed = batch
		if batch != prepared {
			t.Fatal("executor received a different batch than the one approved during preflight")
		}
		return []*updater.Result{{Item: item, Success: true}}
	}

	ok, fail, err := RunUpdate(context.Background(), Config{})
	if err != nil || ok != 1 || fail != 0 || prepares != 1 || executed != prepared || scans != 2 {
		t.Fatalf("ok=%d fail=%d err=%v prepares=%d scans=%d prepared=%p executed=%p", ok, fail, err, prepares, scans, prepared, executed)
	}
}

func TestRunAllInfoOnlyUsesTruthfulNoopWording(t *testing.T) {
	restoreHooks(t)
	fakeScan([]*model.SourceSummary{{
		Category: model.CatBun,
		Items:    []*model.Item{{Name: "typescript", Category: model.CatBun, Status: model.StatusInfo}},
	}}, nil)

	out := captureStdout(t, func() {
		if err := RunAll(context.Background(), Config{}); err != nil {
			t.Fatal(err)
		}
	})
	if strings.Contains(out, "✓ Everything is up to date and clean!") || !strings.Contains(out, "not affirmatively verified") {
		t.Fatalf("output=%q", out)
	}
}

func TestRunAllSkipsInconclusiveScanWithoutMutation(t *testing.T) {
	restoreHooks(t)
	calls := 0
	runScannerAll = func(context.Context, model.PlatformInfo, bool) []*model.SourceSummary {
		calls++
		return []*model.SourceSummary{{Category: model.CatNpm, Label: "npm", ErrorCount: 1}}
	}
	detectPlatform = func() model.PlatformInfo { return model.PlatformInfo{OS: "linux"} }
	updateCategory = func(context.Context, model.Category, []*model.Item, updater.Options) []*updater.Result {
		t.Fatal("inconclusive sources must be skipped, never updated")
		return nil
	}

	out := captureStdout(t, func() {
		err := RunAll(context.Background(), Config{})
		var exitErr *ExitError
		if !errors.As(err, &exitErr) || ExitCode(err) != 2 {
			t.Fatalf("err=%v, want partial ExitError code 2", err)
		}
	})
	if calls != 2 {
		t.Fatalf("calls=%d, want update scan plus clean scan", calls)
	}
	if !strings.Contains(out, "skipped") || !strings.Contains(out, "npm") {
		t.Fatalf("partial run must name the skipped source:\n%s", out)
	}
}

func TestMaybePartialAndEmptyUpdateOutcome(t *testing.T) {
	if err := maybePartial(0); err != nil {
		t.Fatalf("maybePartial(0)=%v, want nil", err)
	}
	if ExitCode(maybePartial(2)) != 2 {
		t.Fatal("maybePartial must be ExitError code 2")
	}

	out := captureStdout(t, func() {
		ok, fail, err := emptyUpdateOutcome(nil, 3)
		if ok != 0 || fail != 0 || ExitCode(err) != 2 {
			t.Fatalf("ok=%d fail=%d err=%v", ok, fail, err)
		}
	})
	if strings.Contains(out, "Nothing to update") || strings.Contains(out, "not affirmatively verified") {
		t.Fatalf("partial empty update must not print a noop summary:\n%s", out)
	}

	info := []*model.SourceSummary{{Items: []*model.Item{{Name: "x", Status: model.StatusInfo}}}}
	out = captureStdout(t, func() {
		ok, fail, err := emptyUpdateOutcome(info, 0)
		if err != nil || ok != 0 || fail != 0 {
			t.Fatalf("ok=%d fail=%d err=%v", ok, fail, err)
		}
	})
	if strings.Contains(out, "✓ Nothing to update") || !strings.Contains(out, "not affirmatively verified") {
		t.Fatalf("output=%q", out)
	}

	out = captureStdout(t, func() {
		if _, _, err := emptyUpdateOutcome(nil, 0); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(out, "✓ Nothing to update") {
		t.Fatalf("output=%q", out)
	}
}

func TestUpdateOutcomeErrPrecedence(t *testing.T) {
	failed := updateOutcomeErr(Config{}, verifyStats{failed: 2, remaining: 3}, 4)
	if failed == nil || isPartialErr(failed) || !strings.Contains(failed.Error(), "update(s) failed") {
		t.Fatalf("classified failures must win over partial skip: %v", failed)
	}
	strict := updateOutcomeErr(Config{Strict: true}, verifyStats{remaining: 1}, 4)
	if strict == nil || isPartialErr(strict) || !strings.Contains(strict.Error(), "still outdated") {
		t.Fatalf("strict remaining must win over partial skip: %v", strict)
	}
	if !isPartialErr(updateOutcomeErr(Config{}, verifyStats{}, 1)) {
		t.Fatal("clean stats with skips must stay Code 2")
	}
	if err := updateOutcomeErr(Config{}, verifyStats{}, 0); err != nil {
		t.Fatalf("clean success: %v", err)
	}
}
