package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/lgldsilva/updash/internal/elevate"
	"github.com/lgldsilva/updash/internal/model"
	"github.com/lgldsilva/updash/internal/scanner"
	"github.com/lgldsilva/updash/internal/updater"
)

// ensureCategoryElevation prepares sudo for a category batch.
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

	if elevate.CanElevateWithoutPassword(ctx) {
		if *sess == nil || !(*sess).Ready() {
			s := elevate.NewSession()
			s.SetPasswordless()
			*sess = s
		}
		return elevate.WithSession(ctx, *sess), false, ""
	}

	if *sess != nil && (*sess).Ready() {
		if cat == model.CatMAS {
			_ = (*sess).Refresh(ctx)
		}
		return elevate.WithSession(ctx, *sess), false, ""
	}

	if cfg.SkipPassword {
		return ctx, true, "precisa de senha de administrador — remova --skip-password para abrir o diálogo do macOS"
	}

	reason := elevationPrompt(cat)
	s, err := elevate.PromptMacPasswordSession(ctx, reason)
	if errors.Is(err, elevate.ErrDialogCancelled) {
		return ctx, true, "atualização cancelada — senha não informada"
	}
	if errors.Is(err, elevate.ErrDialogUnavailable) {
		return ctx, true, "diálogo de senha indisponível nesta plataforma"
	}
	if err != nil {
		return ctx, true, fmt.Sprintf("senha inválida: %v", err)
	}

	*sess = s
	return elevate.WithSession(ctx, s), false, ""
}

// ensureBrewPassword primes sudo for brew PKG casks (Microsoft, etc.) that need admin.
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
		_ = (*sess).Refresh(ctx)
		return elevate.WithSession(ctx, *sess), false, ""
	}
	if elevate.CanElevateWithoutPassword(ctx) {
		if *sess == nil || !(*sess).Ready() {
			s := elevate.NewSession()
			s.SetPasswordless()
			*sess = s
		}
		return elevate.WithSession(ctx, *sess), false, ""
	}
	if cfg.SkipPassword {
		return ctx, true, "PKG brew precisa de senha de admin — remova --skip-password para o diálogo do macOS"
	}
	s, err := elevate.PromptMacPasswordSession(ctx, "Pacotes brew (ex.: Microsoft Office) precisam da sua senha de administrador")
	if errors.Is(err, elevate.ErrDialogCancelled) {
		return ctx, true, "atualização cancelada — senha não informada"
	}
	if errors.Is(err, elevate.ErrDialogUnavailable) {
		return ctx, true, "diálogo de senha indisponível nesta plataforma"
	}
	if err != nil {
		return ctx, true, fmt.Sprintf("senha inválida: %v", err)
	}
	*sess = s
	return elevate.WithSession(ctx, s), false, ""
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

func elevationPrompt(cat model.Category) string {
	switch cat {
	case model.CatMAS:
		return "Apps da Mac App Store (mas) precisam da sua senha de administrador do Mac"
	case model.CatApt:
		return "Atualizações apt precisam da sua senha de administrador"
	case model.CatSnap:
		return "Atualizações snap precisam da sua senha de administrador"
	default:
		return "Esta operação precisa da sua senha de administrador do Mac"
	}
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
