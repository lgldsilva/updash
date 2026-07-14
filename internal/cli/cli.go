// Package cli implements headless updash commands.
package cli

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/lgldsilva/updash/internal/cleaner"
	"github.com/lgldsilva/updash/internal/elevate"
	"github.com/lgldsilva/updash/internal/model"
	"github.com/lgldsilva/updash/internal/platform"
	"github.com/lgldsilva/updash/internal/scanner"
	"github.com/lgldsilva/updash/internal/updater"
)

// Config controls headless CLI behaviour.
type Config struct {
	Verbose      bool
	DryRun       bool
	Only         string // category filter, e.g. "brew", "mas"
	Clean        bool   // include cleanup in RunAll
	SkipPassword      bool // skip batches that need sudo instead of macOS dialog
	Strict            bool // exit non-zero if any item remains outdated
	SkipAutoUpgrade   bool // skip release self-update on startup
}

// Scan runs a single full scan and splits update vs cleanup summaries.
func Scan(ctx context.Context) (updates, cleanup []*model.SourceSummary, elapsed time.Duration, err error) {
	plat := platform.Detect()
	start := time.Now()
	all := scanner.RunAll(ctx, plat, true)
	for _, s := range all {
		if scanner.IsCleanupCategory(s.Category) {
			cleanup = append(cleanup, s)
		} else {
			updates = append(updates, s)
		}
	}
	return updates, cleanup, time.Since(start).Round(time.Millisecond), nil
}

// PrintCheck renders scan results to stdout.
func PrintCheck(updates, cleanup []*model.SourceSummary) (outdated, cleanable int) {
	outdated, cleanable, _, _ = printCheckEnhanced(updates, cleanup)
	return outdated, cleanable
}

// RunCheck scans and prints results.
func RunCheck(ctx context.Context) error {
	plat := platformLabel(platform.Detect())
	fmt.Printf("🔍 Scanning %s...\n", plat)
	updates, cleanup, elapsed, err := Scan(ctx)
	if err != nil {
		return err
	}
	PrintCheck(updates, cleanup)
	if elapsed > 0 {
		fmt.Printf("⏱ scan %s\n", elapsed)
	}
	return nil
}

// RunUpdate updates outdated packages.
func RunUpdate(ctx context.Context, cfg Config) (int, int, error) {
	plat := platform.Detect()
	fmt.Printf("🔍 Scanning %s...\n", platformLabel(plat))
	updates, _, _, err := Scan(ctx)
	if err != nil {
		return 0, 0, err
	}

	items := collectOutdated(updates, cfg.Only)
	if len(items) == 0 {
		fmt.Println("✓ Nothing to update")
		return 0, 0, nil
	}

	updatable, manualOnly := partitionUpdatable(items)
	if len(updatable) == 0 && len(manualOnly) == 0 {
		fmt.Println("✓ Nothing to update")
		return 0, 0, nil
	}
	if len(manualOnly) > 0 {
		fmt.Printf("ℹ %d item(s) só atualização manual — não serão tentados\n", len(manualOnly))
	}

	if cfg.DryRun {
		printDryRun("update", updatable)
		return 0, 0, nil
	}

	opts := updater.DefaultOptions()
	if !cfg.Verbose {
		opts.Verbose = false
	}

	fmt.Printf("\n📦 Updating %d item(s)...\n", len(updatable))
	start := time.Now()
	ok, fail, skipped, results := runUpdateBatches(ctx, plat, updates, updatable, opts, cfg)
	results = append(results, manualOnlyResults(manualOnly)...)
	fmt.Printf("\n⏱ update %s — %d ok, %d skipped, %d failed\n",
		time.Since(start).Round(time.Second), ok, skipped, fail)

	fmt.Println("\n🔍 Verifying...")
	updates2, _, _, _ := Scan(ctx)
	stats := PrintVerifyReport(updates2, results, ok, fail, skipped)

	if shouldFailExit(cfg, stats) {
		if fail > 0 {
			return ok, fail, fmt.Errorf("%d update(s) failed", fail)
		}
		return ok, fail, fmt.Errorf("%d item(s) still outdated", stats.remaining)
	}
	return ok, fail, nil
}

// RunClean runs cleanup operations.
func RunClean(ctx context.Context, cfg Config) (int, int, error) {
	plat := platform.Detect()
	fmt.Printf("🔍 Scanning %s...\n", platformLabel(plat))
	_, cleanup, _, err := Scan(ctx)
	if err != nil {
		return 0, 0, err
	}

	items := collectCleanable(cleanup, cfg.Only)
	if len(items) == 0 {
		fmt.Println("✓ Nothing to clean")
		return 0, 0, nil
	}

	if cfg.DryRun {
		printDryRun("clean", items)
		return 0, 0, nil
	}

	opts := cleaner.DefaultOptions()
	if !cfg.Verbose {
		opts.Verbose = false
	}
	ctx = prepareCleanElevation(ctx, plat, items, opts.Interactive)

	fmt.Printf("\n🧹 Cleaning %d item(s)...\n", len(items))
	start := time.Now()
	ok, fail, freed := runCleanBatches(ctx, cleanup, items, opts)
	fmt.Printf("\n⏱ clean %s — %d ok, %d failed", time.Since(start).Round(time.Second), ok, fail)
	if freed > 0 {
		fmt.Printf(", %s freed", cleaner.FormatBytes(freed))
	}
	fmt.Println()
	if fail > 0 {
		return ok, fail, fmt.Errorf("%d clean(s) failed", fail)
	}
	return ok, fail, nil
}

// RunAll updates then cleans.
func RunAll(ctx context.Context, cfg Config) error {
	uok, ufail, err := RunUpdate(ctx, cfg)
	if err != nil && ufail == 0 {
		return err
	}
	cok, cfail, cerr := RunClean(ctx, cfg)
	if cerr != nil && cfail == 0 {
		return cerr
	}
	if ufail > 0 || cfail > 0 {
		return fmt.Errorf("finished with %d update fail(s), %d clean fail(s)", ufail, cfail)
	}
	if uok == 0 && cok == 0 {
		fmt.Println("✓ Everything is up to date and clean!")
	}
	return nil
}

type cleanGroup struct {
	label string
	items []*model.Item
}

func runCleanBatches(ctx context.Context, summaries []*model.SourceSummary, items []*model.Item, opts cleaner.Options) (ok, fail int, freed int64) {
	for _, g := range groupCleanBySummary(summaries, items) {
		fmt.Printf("\n→ %s (%d item(s))\n", g.label, len(g.items))
		for _, it := range g.items {
			detail := it.Name
			if it.Reclaimable != "" {
				detail = fmt.Sprintf("%s (~%s reclaimable)", it.Name, it.Reclaimable)
			}
			fmt.Printf("  • %s\n", detail)

			itemCtx, cancel := context.WithTimeout(ctx, cleaner.ItemTimeout(it))
			r := cleaner.CleanOne(itemCtx, it, opts)
			cancel()

			if r.Success {
				freed += r.BytesFreed
				if r.BytesFreed > 0 {
					fmt.Printf("  ✓ %s (freed %s)\n", it.Name, cleaner.FormatBytes(r.BytesFreed))
				} else {
					fmt.Printf("  ✓ %s (nothing to remove)\n", it.Name)
				}
				ok++
			} else {
				errMsg := r.Error
				if errMsg == "" {
					errMsg = "failed"
				}
				fmt.Printf("  ✘ %s: %s\n", it.Name, errMsg)
				fail++
			}
		}
	}
	return ok, fail, freed
}

func groupCleanBySummary(summaries []*model.SourceSummary, items []*model.Item) []cleanGroup {
	want := make(map[*model.Item]bool, len(items))
	for _, it := range items {
		want[it] = true
	}

	var groups []cleanGroup
	for _, s := range summaries {
		var groupItems []*model.Item
		for _, it := range s.Items {
			if want[it] {
				groupItems = append(groupItems, it)
			}
		}
		if len(groupItems) > 0 {
			groups = append(groups, cleanGroup{
				label: fmt.Sprintf("%s %s", s.Icon, s.Label),
				items: groupItems,
			})
		}
	}
	return groups
}

func runUpdateBatches(
	ctx context.Context,
	plat model.PlatformInfo,
	summaries []*model.SourceSummary,
	items []*model.Item,
	opts updater.Options,
	cfg Config,
) (ok, fail, skipped int, allResults []*updater.Result) {
	nativeItems, normalItems := partitionNativeElevated(plat, items, cfg)
	var elevSession *elevate.Session
	ctx = primeElevationSession(ctx, plat, normalItems, cfg, &elevSession)

	if len(nativeItems) > 0 {
		fmt.Printf("\n→ 🔐 Atualizações privilegiadas (%d item(s))\n", len(nativeItems))
		for _, it := range nativeItems {
			fmt.Printf("  • %s\n", it.Name)
		}
		for _, r := range runNativeElevatedItems(ctx, plat, nativeItems, opts, cfg, &elevSession) {
			allResults = append(allResults, r)
			if r.Success {
				fmt.Printf("  ✓ %s\n", r.Item.Name)
				ok++
			} else if isSkippedResult(r) {
				fmt.Printf("  ⊘ %s: %s\n", r.Item.Name, strings.TrimPrefix(r.Error, "⊘ "))
				skipped++
			} else {
				errMsg := r.Error
				if errMsg == "" {
					errMsg = "failed"
				}
				fmt.Printf("  ✘ %s: %s\n", r.Item.Name, errMsg)
				fail++
			}
		}
	}

	groups := groupByCategory(normalItems)
	cats := sortedCategories(groups)

	for _, cat := range cats {
		groupItems := groups[cat]
		label := categoryLabel(summaries, cat)
		fmt.Printf("\n→ %s (%d item(s))\n", label, len(groupItems))

		batchCtx, cancel := context.WithTimeout(ctx, updater.BatchTimeout(cat))

		var results []*updater.Result
		if cat == model.CatBrew {
			results = runBrewUpdateBatch(batchCtx, groupItems, opts, cfg, &elevSession)
		} else {
			elevCtx := batchCtx
			batchSkipped := false
			skipReason := ""
			elevCtx, batchSkipped, skipReason = ensureCategoryElevation(elevCtx, plat, cat, cfg, &elevSession)
			if batchSkipped {
				results = skipBatchResults(groupItems, skipReason)
			} else {
				results = updater.UpdateCategory(elevCtx, cat, groupItems, opts)
			}
		}
		cancel()

		for _, r := range results {
			allResults = append(allResults, r)
			if r.Success {
				fmt.Printf("  ✓ %s\n", r.Item.Name)
				ok++
			} else if isSkippedResult(r) {
				fmt.Printf("  ⊘ %s: %s\n", r.Item.Name, strings.TrimPrefix(r.Error, "⊘ "))
				skipped++
			} else {
				errMsg := r.Error
				if errMsg == "" {
					errMsg = "failed"
				}
				fmt.Printf("  ✘ %s: %s\n", r.Item.Name, errMsg)
				fail++
			}
		}
	}
	return ok, fail, skipped, allResults
}

func collectOutdated(summaries []*model.SourceSummary, only string) []*model.Item {
	var items []*model.Item
	for _, s := range summaries {
		for _, it := range s.Items {
			if it.Status == model.StatusOutdated && itemMatchesFilter(s, it, only) {
				items = append(items, it)
			}
		}
	}
	return items
}

func collectCleanable(summaries []*model.SourceSummary, only string) []*model.Item {
	var items []*model.Item
	for _, s := range summaries {
		for _, it := range s.Items {
			if it.Status == model.StatusCleanCandidate && itemMatchesFilter(s, it, only) {
				items = append(items, it)
			}
		}
	}
	return items
}

func itemMatchesFilter(s *model.SourceSummary, it *model.Item, only string) bool {
	if only == "" {
		return true
	}
	o := strings.ToLower(strings.TrimSpace(only))
	if strings.EqualFold(string(s.Category), o) {
		return true
	}
	if strings.Contains(strings.ToLower(s.Label), o) {
		return true
	}
	if strings.Contains(strings.ToLower(it.Name), o) {
		return true
	}
	return false
}

func printDryRun(action string, items []*model.Item) {
	fmt.Printf("dry-run: would %s:\n", action)
	for _, it := range items {
		extra := ""
		if it.Reclaimable != "" {
			extra = fmt.Sprintf(" — %s", it.Reclaimable)
		}
		fmt.Printf("  • %s (%s)%s\n", it.Name, it.Category, extra)
	}
}

func groupByCategory(items []*model.Item) map[model.Category][]*model.Item {
	groups := make(map[model.Category][]*model.Item)
	for _, it := range items {
		groups[it.Category] = append(groups[it.Category], it)
	}
	return groups
}

func sortedCategories(groups map[model.Category][]*model.Item) []model.Category {
	cats := make([]model.Category, 0, len(groups))
	for cat := range groups {
		cats = append(cats, cat)
	}
	sort.Slice(cats, func(i, j int) bool { return cats[i] < cats[j] })
	return cats
}

func categoryLabel(summaries []*model.SourceSummary, cat model.Category) string {
	for _, s := range summaries {
		if s.Category == cat {
			return fmt.Sprintf("%s %s", s.Icon, s.Label)
		}
	}
	return string(cat)
}

func prepareCleanElevation(ctx context.Context, plat model.PlatformInfo, items []*model.Item, interactive bool) context.Context {
	if !elevate.ItemsNeedElevation(items, plat, true) {
		return ctx
	}
	if elevate.CanElevateWithoutPassword(ctx) {
		sess := elevate.NewSession()
		sess.SetPasswordless()
		return elevate.WithSession(ctx, sess)
	}
	if interactive {
		fmt.Fprintln(os.Stderr, "ℹ sudo may prompt for your password during cleanup")
	}
	return ctx
}

func platformLabel(p model.PlatformInfo) string {
	switch p.OS {
	case "darwin":
		return "macOS"
	case "windows":
		return "Windows"
	case "linux":
		if p.Distro != "" {
			return p.Distro
		}
		return "linux"
	}
	return "system"
}
