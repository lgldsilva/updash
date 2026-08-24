package cli

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/lgldsilva/updash/internal/elevate"
	"github.com/lgldsilva/updash/internal/model"
	"github.com/lgldsilva/updash/internal/scanner"
	"github.com/lgldsilva/updash/internal/updater"
)

// primeElevationSession prompts once per run when any item needs sudo (MAS, Microsoft
// brew PKG, apt, etc.) and stores the session for all later batches.
func primeElevationSession(
	ctx context.Context,
	plat model.PlatformInfo,
	items []*model.Item,
	cfg Config,
	sess **elevate.Session,
	neededOverride ...bool,
) context.Context {
	needed := false
	if len(neededOverride) > 0 {
		needed = neededOverride[0]
	} else {
		needed = plannedItemsNeedElevation(items)
	}
	if !needed {
		return ctx
	}

	if canElevateNP(ctx) {
		if *sess == nil || !(*sess).Ready() {
			s := elevate.NewSession()
			s.SetPasswordless()
			*sess = s
		}
		return elevate.WithSession(ctx, *sess)
	}

	if *sess != nil && (*sess).Ready() {
		_ = (*sess).Refresh(ctx)
		return elevate.WithSession(ctx, *sess)
	}

	if cfg.SkipPassword {
		return ctx
	}

	// On macOS, brew/MAS use the system authorization sheet (see runNativeElevatedItems).
	if plat.OS == "darwin" && nativeMacAvail() {
		return ctx
	}

	s, err := promptMacSess(ctx,
		"updash needs your administrator password to complete the updates")
	if err != nil {
		switch {
		case errors.Is(err, elevate.ErrDialogCancelled):
			fmt.Fprintln(os.Stderr, "⊘ Password cancelled — admin-required packages will be skipped")
		case errors.Is(err, elevate.ErrDialogUnavailable):
			fmt.Fprintln(os.Stderr, "⊘ Password dialog unavailable — admin-required packages will be skipped")
		default:
			fmt.Fprintf(os.Stderr, "⊘ Invalid password: %v — admin-required packages will be skipped\n", err)
		}
		return ctx
	}

	*sess = s
	return elevate.WithSession(ctx, s)
}

func plannedItemsNeedElevation(items []*model.Item) bool {
	for cat, group := range groupByCategory(items) {
		plans, err := updater.PlanUpdateCommands(cat, group)
		if err == nil && updater.PlansRequireElevation(plans) {
			return true
		}
	}
	return false
}

func primeElevationSessionForBatches(ctx context.Context, plat model.PlatformInfo, batches map[model.Category]*updater.PreparedUpdateBatch, cfg Config, sess **elevate.Session) context.Context {
	for _, batch := range batches {
		if updater.PlansRequireElevation(batch.Plans()) {
			return primeElevationSessionWithNeed(ctx, plat, true, cfg, sess)
		}
	}
	return ctx
}

func primeElevationSessionWithNeed(ctx context.Context, plat model.PlatformInfo, needed bool, cfg Config, sess **elevate.Session) context.Context {
	if !needed {
		return ctx
	}
	// Delegate to the existing session path with a synthetic elevated item is
	// avoided; its remaining behavior is intentionally shared below.
	return primeElevationSession(ctx, plat, nil, cfg, sess, true)
}

func itemsNeedPasswordElevation(items []*model.Item, plat model.PlatformInfo) bool {
	return elevate.ItemsNeedElevation(items, plat, false)
}

// ensureCategoryElevation attaches the run-wide session for a category batch.
// When skipped is true, reason explains why the batch should not run elevated.
func ensureCategoryElevation(
	ctx context.Context,
	plat model.PlatformInfo,
	cat model.Category,
	cfg Config,
	sess **elevate.Session,
) (context.Context, bool, string) {
	if !elevate.CategoryNeedsElevation(cat, plat) {
		if *sess != nil && (*sess).Ready() {
			return elevate.WithSession(ctx, *sess), false, ""
		}
		return ctx, false, ""
	}

	if *sess != nil && (*sess).Ready() {
		if err := (*sess).Refresh(ctx); err != nil {
			return ctx, true, fmt.Sprintf("sudo expired: %v", err)
		}
		return elevate.WithSession(ctx, *sess), false, ""
	}

	return ctx, true, elevationSkipReason(cfg)
}

// ensurePlannedElevation follows the command plan rather than inferring
// privilege from a category. This keeps --skip-password truthful for commands
// such as npm whose elevation depends on the configured installation prefix.
func ensurePlannedElevation(ctx context.Context, needed bool, cfg Config, sess **elevate.Session) (context.Context, bool, string) {
	if !needed {
		return ctx, false, ""
	}
	if *sess != nil && (*sess).Ready() {
		if err := (*sess).Refresh(ctx); err != nil {
			return ctx, true, fmt.Sprintf("sudo expired: %v", err)
		}
		return elevate.WithSession(ctx, *sess), false, ""
	}
	return ctx, true, elevationSkipReason(cfg)
}

// ensureBrewPassword attaches the run-wide session for brew PKG casks (Microsoft, etc.).
func ensureBrewPassword(
	ctx context.Context,
	items []*model.Item,
	cfg Config,
	sess **elevate.Session,
) (context.Context, bool, string) {
	if !brewBatchNeedsPassword(items) {
		return ctx, false, ""
	}
	if *sess != nil && (*sess).Ready() {
		if err := (*sess).Refresh(ctx); err != nil {
			return ctx, true, fmt.Sprintf("sudo expired: %v", err)
		}
		return elevate.WithSession(ctx, *sess), false, ""
	}
	return ctx, true, elevationSkipReason(cfg)
}

func elevationSkipReason(cfg Config) string {
	if cfg.SkipPassword {
		return "needs administrator password — remove --skip-password to open the macOS dialog"
	}
	return "update cancelled — password not provided"
}

func brewItemNeedsPassword(it *model.Item) bool {
	// PKG casks (Microsoft, etc.) carry an upgrade note mentioning admin/senha.
	return scanner.BrewNeedsSudoPrime(it.Name)
}

func brewBatchNeedsPassword(items []*model.Item) bool {
	for _, it := range items {
		if brewItemNeedsPassword(it) {
			return true
		}
	}
	return false
}

func runBrewUpdateBatch(
	ctx context.Context,
	items []*model.Item,
	opts updater.Options,
	cfg Config,
	sess **elevate.Session,
) []*updater.Result {
	var plain, password []*model.Item
	for _, it := range items {
		if brewItemNeedsPassword(it) {
			password = append(password, it)
		} else {
			plain = append(plain, it)
		}
	}

	var results []*updater.Result
	if len(plain) > 0 {
		results = append(results, updateCategory(ctx, model.CatBrew, plain, opts)...)
	}
	if len(password) > 0 {
		passCtx, skipped, reason := ensureBrewPassword(ctx, password, cfg, sess)
		if skipped {
			results = append(results, skipBatchResults(password, reason)...)
		} else {
			results = append(results, updateCategory(passCtx, model.CatBrew, password, opts)...)
		}
	}
	return results
}

func partitionUpdatable(items []*model.Item) (updatable, manual []*model.Item) {
	for _, it := range items {
		if it.KeepPolicy != "" {
			if kind, _ := updater.ClassifyItem(it, nil); kind == updater.KindManualOnly {
				manual = append(manual, it)
				continue
			}
		}
		updatable = append(updatable, it)
	}
	return updatable, manual
}

func skipResult(it *model.Item, reason string) *updater.Result {
	return &updater.Result{
		Item:    it,
		Success: false,
		Error:   "⊘ " + reason,
	}
}

func manualOnlyResults(items []*model.Item) []*updater.Result {
	results := make([]*updater.Result, len(items))
	for i, it := range items {
		reason := it.KeepPolicy
		if reason == "" {
			reason = "manual update only"
		}
		results[i] = skipResult(it, reason)
	}
	return results
}

func skipBatchResults(items []*model.Item, reason string) []*updater.Result {
	results := make([]*updater.Result, len(items))
	for i, it := range items {
		it.Status = model.StatusOutdated
		results[i] = skipResult(it, reason)
	}
	return results
}
