package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

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
) context.Context {
	if !itemsNeedPasswordElevation(items, plat) {
		return ctx
	}

	if elevate.CanElevateWithoutPassword(ctx) {
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
	if plat.OS == "darwin" && elevate.NativeMacAuthAvailable() {
		return ctx
	}

	s, err := elevate.PromptMacPasswordSession(ctx,
		"O updash precisa da sua senha de administrador para concluir as atualizações")
	if err != nil {
		switch {
		case errors.Is(err, elevate.ErrDialogCancelled):
			fmt.Fprintln(os.Stderr, "⊘ Senha cancelada — pacotes que precisam de admin serão ignorados")
		case errors.Is(err, elevate.ErrDialogUnavailable):
			fmt.Fprintln(os.Stderr, "⊘ Diálogo de senha indisponível — pacotes que precisam de admin serão ignorados")
		default:
			fmt.Fprintf(os.Stderr, "⊘ Senha inválida: %v — pacotes que precisam de admin serão ignorados\n", err)
		}
		return ctx
	}

	*sess = s
	return elevate.WithSession(ctx, s)
}

func itemsNeedPasswordElevation(items []*model.Item, plat model.PlatformInfo) bool {
	for _, it := range items {
		if it.Category == model.CatBrew && brewItemNeedsPassword(it) {
			return true
		}
		if elevate.CategoryNeedsElevation(it.Category, plat) {
			return true
		}
	}
	return false
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
			return ctx, true, fmt.Sprintf("sudo expirou: %v", err)
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
			return ctx, true, fmt.Sprintf("sudo expirou: %v", err)
		}
		return elevate.WithSession(ctx, *sess), false, ""
	}
	return ctx, true, elevationSkipReason(cfg)
}

func elevationSkipReason(cfg Config) string {
	if cfg.SkipPassword {
		return "precisa de senha de administrador — remova --skip-password para abrir o diálogo do macOS"
	}
	return "atualização cancelada — senha não informada"
}

func brewItemNeedsPassword(it *model.Item) bool {
	note := scanner.BrewUpgradeNote(it.Name)
	return note != "" && containsPasswordNote(note)
}

func brewBatchNeedsPassword(items []*model.Item) bool {
	for _, it := range items {
		if brewItemNeedsPassword(it) {
			return true
		}
	}
	return false
}

func containsPasswordNote(note string) bool {
	n := strings.ToLower(note)
	return strings.Contains(n, "senha") || strings.Contains(n, "admin")
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
		results = append(results, updater.UpdateCategory(ctx, model.CatBrew, plain, opts)...)
	}
	if len(password) > 0 {
		passCtx, skipped, reason := ensureBrewPassword(ctx, password, cfg, sess)
		if skipped {
			results = append(results, skipBatchResults(password, reason)...)
		} else {
			results = append(results, updater.UpdateCategory(passCtx, model.CatBrew, password, opts)...)
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

func manualOnlyResults(items []*model.Item) []*updater.Result {
	results := make([]*updater.Result, len(items))
	for i, it := range items {
		reason := it.KeepPolicy
		if reason == "" {
			reason = "só atualização manual"
		}
		results[i] = &updater.Result{
			Item:    it,
			Success: false,
			Error:   "⊘ " + reason,
		}
	}
	return results
}

func skipBatchResults(items []*model.Item, reason string) []*updater.Result {
	results := make([]*updater.Result, len(items))
	for i, it := range items {
		it.Status = model.StatusOutdated
		results[i] = &updater.Result{
			Item:    it,
			Success: false,
			Error:   "⊘ " + reason,
		}
	}
	return results
}
