package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"unicode"

	"github.com/lgldsilva/updash/internal/elevate"
	"github.com/lgldsilva/updash/internal/model"
	"github.com/lgldsilva/updash/internal/updater"
)

// shouldUseNativeMacAuth reports whether items should use the macOS system auth sheet.
func shouldUseNativeMacAuth(plat model.PlatformInfo, items []*model.Item, cfg Config) bool {
	if plat.OS != "darwin" || cfg.SkipPassword || !elevate.NativeMacAuthAvailable() {
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
) []*updater.Result {
	if !stdinIsTTY() {
		fmt.Fprintln(os.Stderr, "⚠ Rode no Terminal.app (não em pipe/CI) para o diálogo nativo do macOS aparecer")
	}
	fmt.Println("ℹ O macOS vai pedir autorização no diálogo nativo do sistema (ícone de cadeado)")
	fmt.Println("ℹ Depois disso, brew/mas rodam como seu usuário com sudo em cache")

	if err := elevate.PrimeMacOSUserSudo(ctx); err != nil {
		if errors.Is(err, elevate.ErrDialogCancelled) {
			fmt.Fprintln(os.Stderr, "⊘ Autorização cancelada — pacotes privilegiados ignorados")
			return skipBatchResults(items, "autorização cancelada no diálogo do macOS")
		}
		fmt.Fprintf(os.Stderr, "⊘ Falha na autorização nativa: %v\n", err)
		return nativeElevatedFailAll(items, "", err)
	}

	// sudo -v succeeded — reuse normal updater paths with a passwordless session.
	if *sess == nil || !(*sess).Ready() {
		s := elevate.NewSession()
		s.SetPasswordless()
		*sess = s
	}
	ctx = elevate.WithSession(ctx, *sess)

	var results []*updater.Result
	groups := groupByCategory(items)
	for _, cat := range sortedCategories(groups) {
		if cat == model.CatBrew {
			results = append(results, updater.UpdateCategory(ctx, cat, groups[cat], opts)...)
			continue
		}
		elevCtx, skipped, reason := ensureCategoryElevation(ctx, plat, cat, cfg, sess)
		if skipped {
			results = append(results, skipBatchResults(groups[cat], reason)...)
		} else {
			results = append(results, updater.UpdateCategory(elevCtx, cat, groups[cat], opts)...)
		}
	}
	return results
}

func normalizeItemKey(name string) string {
	return strings.TrimSpace(strings.Map(func(r rune) rune {
		if unicode.Is(unicode.Cf, r) || unicode.Is(unicode.Cc, r) {
			return -1
		}
		return r
	}, name))
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
