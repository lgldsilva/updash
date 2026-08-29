package cli

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/lgldsilva/updash/internal/elevate"
	"github.com/lgldsilva/updash/internal/model"
	"github.com/lgldsilva/updash/internal/updater"
)

// shouldUseNativeMacAuth reports whether items should use the macOS system auth sheet.
func shouldUseNativeMacAuth(plat model.PlatformInfo, items []*model.Item, cfg Config) bool {
	if plat.OS != "darwin" || cfg.SkipPassword || !nativeMacAvail() {
		return false
	}
	return itemsNeedPasswordElevation(items, plat)
}

func partitionNativeElevated(plat model.PlatformInfo, items []*model.Item, cfg Config) (native, normal []*model.Item) {
	if !shouldUseNativeMacAuth(plat, items, cfg) {
		return nil, items
	}
	for _, it := range items {
		if itemNeedsNativeElevation(it, plat) {
			native = append(native, it)
		} else {
			normal = append(normal, it)
		}
	}
	return native, normal
}

func itemNeedsNativeElevation(it *model.Item, plat model.PlatformInfo) bool {
	if it.Category == model.CatBrew && brewItemNeedsPassword(it) {
		return true
	}
	return elevate.CategoryNeedsElevation(it.Category, plat)
}

// runNativeElevatedItems primes sudo via the macOS native auth sheet, then runs
// brew/mas as the logged-in user (Homebrew refuses to run as root).
func runNativeElevatedItems(
	ctx context.Context,
	plat model.PlatformInfo,
	items []*model.Item,
	opts updater.Options,
	cfg Config,
	sess **elevate.Session,
	preparedBatches ...map[model.Category]*updater.PreparedUpdateBatch,
) []*updater.Result {
	if !stdinIsTTYFn() {
		fmt.Fprintln(os.Stderr, "⚠ Run in Terminal.app (not a pipe/CI) for the native macOS dialog to appear")
	}
	fmt.Println("ℹ macOS will ask for authorization in the native system dialog (lock icon)")
	fmt.Println("ℹ After that, brew/mas run as your user with cached sudo")

	if err := primeMacSudo(ctx); err != nil {
		if errors.Is(err, elevate.ErrDialogCancelled) {
			fmt.Fprintln(os.Stderr, "⊘ Authorization cancelled — privileged packages skipped")
			return skipBatchResults(items, "authorization cancelled in the macOS dialog")
		}
		fmt.Fprintf(os.Stderr, "⊘ Native authorization failed: %v\n", err)
		return nativeElevatedFailAll(items, "", err)
	}

	// sudo -v succeeded — reuse normal updater paths with a passwordless session.
	if *sess == nil || !(*sess).Ready() {
		s := elevate.NewSession()
		s.SetPasswordless()
		*sess = s
	}
	ctx = elevate.WithSession(ctx, *sess)
	return executeNativeElevatedGroups(ctx, plat, items, opts, cfg, sess, firstPreparedBatch(preparedBatches))
}

func firstPreparedBatch(batches []map[model.Category]*updater.PreparedUpdateBatch) map[model.Category]*updater.PreparedUpdateBatch {
	if len(batches) == 0 {
		return nil
	}
	return batches[0]
}

func executeNativeElevatedGroups(
	ctx context.Context,
	plat model.PlatformInfo,
	items []*model.Item,
	opts updater.Options,
	cfg Config,
	sess **elevate.Session,
	prepared map[model.Category]*updater.PreparedUpdateBatch,
) []*updater.Result {
	var results []*updater.Result
	groups := groupByCategory(items)
	for _, cat := range sortedCategories(groups) {
		results = append(results, executeNativeElevatedCategory(ctx, plat, cat, groups[cat], prepared, opts, cfg, sess)...)
	}
	return results
}

func executeNativeElevatedCategory(
	ctx context.Context,
	plat model.PlatformInfo,
	cat model.Category,
	items []*model.Item,
	prepared map[model.Category]*updater.PreparedUpdateBatch,
	opts updater.Options,
	cfg Config,
	sess **elevate.Session,
) []*updater.Result {
	if batch := prepared[cat]; batch != nil {
		subset, err := batch.Subset(items)
		if err != nil {
			return updatePlanErrorResults(items, err)
		}
		return executePreparedBatch(ctx, subset, opts)
	}
	if cat == model.CatBrew {
		return updateCategory(ctx, cat, items, opts)
	}
	elevCtx, skipped, reason := ensureCategoryElevation(ctx, plat, cat, cfg, sess)
	if skipped {
		return skipBatchResults(items, reason)
	}
	return updateCategory(elevCtx, cat, items, opts)
}

func stdinIsTTY() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

func nativeElevatedFailAll(items []*model.Item, output string, err error) []*updater.Result {
	results := make([]*updater.Result, len(items))
	msg := err.Error()
	for i, it := range items {
		results[i] = &updater.Result{
			Item:    it,
			Success: false,
			Error:   msg,
			Output:  output,
		}
		it.Status = model.StatusError
	}
	return results
}
