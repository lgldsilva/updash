// Package cli implements headless updash commands.
package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/lgldsilva/updash/internal/cleaner"
	"github.com/lgldsilva/updash/internal/elevate"
	"github.com/lgldsilva/updash/internal/model"
	"github.com/lgldsilva/updash/internal/scanner"
	"github.com/lgldsilva/updash/internal/updater"
)

// Config controls headless CLI behaviour.
type Config struct {
	Verbose         bool
	DryRun          bool
	Only            string // category filter, e.g. "brew", "mas"
	Clean           bool   // include cleanup in RunAll
	SkipPassword    bool   // skip batches that need sudo instead of macOS dialog
	Strict          bool   // exit non-zero if any item remains outdated
	SkipAutoUpgrade bool   // skip release self-update on startup
	JSON            bool   // machine-readable --check output
}

// ExitError carries the process class without changing the public command
// functions' error-returning API. cmd/updash maps it to the documented code.
type ExitError struct {
	Code int
	Err  error
}

func (e *ExitError) Error() string { return e.Err.Error() }
func (e *ExitError) Unwrap() error { return e.Err }

// ExitCode classifies CLI errors with the documented 2 > 1 > 0 precedence.
func ExitCode(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *ExitError
	if errors.As(err, &exitErr) && exitErr.Code == 2 {
		return 2
	}
	return 1
}

// Scan runs a single full scan and splits update vs cleanup summaries.
func Scan(ctx context.Context) (updates, cleanup []*model.SourceSummary, elapsed time.Duration, err error) {
	plat := detectPlatform()
	start := time.Now()
	all := runScannerAll(ctx, plat, true)
	for _, s := range all {
		if scanner.IsCleanupCategory(s.Category) {
			cleanup = append(cleanup, s)
		} else {
			updates = append(updates, s)
		}
	}
	return updates, cleanup, time.Since(start).Round(time.Millisecond), nil
}

func scanForConfig(ctx context.Context, cfg Config, includeCleanup bool, cleanupOnlyArg ...bool) (updates, cleanup []*model.SourceSummary, elapsed time.Duration, err error) {
	cleanupOnly := len(cleanupOnlyArg) > 0 && cleanupOnlyArg[0]
	plat := detectPlatform()
	var categories []model.Category
	if strings.TrimSpace(cfg.Only) != "" {
		cat, ok := scanner.CanonicalCategory(plat, includeCleanup, cfg.Only)
		if !ok {
			return nil, nil, 0, &ExitError{Code: 2, Err: fmt.Errorf("invalid or unavailable --only category: %q", cfg.Only)}
		}
		categories = []model.Category{cat}
		cfg.Only = string(cat)
	}
	start := time.Now()
	var all []*model.SourceSummary
	if len(categories) == 0 {
		all = runScannerAll(ctx, plat, includeCleanup)
	} else {
		if cleanupOnly {
			all = runScannerFilteredForCleanup(ctx, plat, categories)
		} else {
			all = runScannerFiltered(ctx, plat, includeCleanup, categories)
		}
	}
	for _, s := range all {
		if scanner.IsCleanupCategory(s.Category) {
			cleanup = append(cleanup, s)
		} else {
			updates = append(updates, s)
		}
	}
	return updates, cleanup, time.Since(start).Round(time.Millisecond), nil
}

func requireConclusive(summaries ...[]*model.SourceSummary) error {
	for _, group := range summaries {
		if hasInconclusive(group) {
			return &ExitError{Code: 2, Err: fmt.Errorf("scan contains errors or unverified sources")}
		}
	}
	return nil
}

// PrintCheck renders scan results to stdout.
func PrintCheck(updates, cleanup []*model.SourceSummary) (outdated, cleanable int) {
	outdated, cleanable, _, _ = printCheckEnhanced(updates, cleanup)
	return outdated, cleanable
}

const (
	msgScanning = "🔍 Scanning %s...\n"
	msgItemFail = "  ✘ %s: %s\n"
)

// RunCheck scans and prints results.
func RunCheck(ctx context.Context, cfg Config) error {
	updates, cleanup, elapsed, err := scanForConfig(ctx, cfg, true, false)
	if err != nil {
		return err
	}
	// --only applies to --check as well (it was silently ignored before).
	updates = filterCheckResults(updates, cfg.Only)
	cleanup = filterCheckResults(cleanup, cfg.Only)
	if cfg.JSON {
		rep := BuildCheckReport(updates, cleanup)
		rep.Platform = platformLabel(detectPlatform())
		if elapsed > 0 {
			rep.ElapsedMS = elapsed.Milliseconds()
		}
		if err := WriteCheckJSON(os.Stdout, rep); err != nil {
			return err
		}
		if code := ExitCodeForReport(cfg, rep); code != 0 {
			if code == 2 {
				return &ExitError{Code: 2, Err: fmt.Errorf("scan contains errors or unverified sources")}
			}
			return fmt.Errorf("%d outdated, %d cleanable", rep.Outdated, rep.Cleanable)
		}
		return nil
	}
	fmt.Printf(msgScanning, platformLabel(detectPlatform()))
	PrintCheck(updates, cleanup)
	if elapsed > 0 {
		fmt.Printf("⏱ scan %s\n", elapsed)
	}
	if code := ExitCodeForSummaries(cfg, updates, cleanup); code != 0 {
		if code == 2 {
			return &ExitError{Code: 2, Err: fmt.Errorf("scan contains errors or unverified sources")}
		}
		return fmt.Errorf("strict: remaining outdated/cleanable items")
	}
	return nil
}

func countOutdated(summaries []*model.SourceSummary) int {
	n := 0
	for _, s := range summaries {
		for _, it := range s.Items {
			if it != nil && it.Status == model.StatusOutdated {
				n++
			}
		}
	}
	return n
}

func countCleanable(summaries []*model.SourceSummary) int {
	n := 0
	for _, s := range summaries {
		for _, it := range s.Items {
			if it != nil && it.Status == model.StatusCleanCandidate {
				n++
			}
		}
	}
	return n
}

// RunUpdate updates outdated packages.
func RunUpdate(ctx context.Context, cfg Config) (int, int, error) {
	plat := detectPlatform()
	fmt.Printf(msgScanning, platformLabel(plat))
	updates, _, _, err := scanForConfig(ctx, cfg, false, false)
	if err != nil {
		return 0, 0, err
	}
	if err := requireConclusive(updates); err != nil {
		return 0, 0, err
	}

	items := collectOutdated(updates, cfg.Only)
	if len(items) == 0 {
		if hasInformational(updates) {
			fmt.Println("ℹ No update selected; installed inventory was not affirmatively verified")
		} else {
			fmt.Println("✓ Nothing to update")
		}
		return 0, 0, nil
	}

	updatable, manualOnly := partitionUpdatable(items)
	if len(updatable) == 0 && len(manualOnly) == 0 {
		fmt.Println("✓ Nothing to update")
		return 0, 0, nil
	}
	if len(manualOnly) > 0 {
		fmt.Printf("ℹ %d item(s) require manual update — skipping\n", len(manualOnly))
	}
	prepared, err := prepareBatches(ctx, updatable)
	if err != nil {
		return 0, 0, err
	}

	if cfg.DryRun {
		printPreparedDryRun(prepared)
		return 0, 0, nil
	}

	opts := updater.DefaultOptions()
	if !cfg.Verbose {
		opts.Verbose = false
	}

	fmt.Printf("\n📦 Updating %d item(s)...\n", len(updatable))
	start := time.Now()
	ok, fail, skipped, results := runUpdateBatches(ctx, plat, updates, updatable, prepared, opts, cfg)
	results = append(results, manualOnlyResults(manualOnly)...)
	fmt.Printf("\n⏱ update %s — %d ok, %d skipped, %d failed\n",
		time.Since(start).Round(time.Second), ok, skipped, fail)

	fmt.Println("\n🔍 Verifying...")
	updates2, _, _, verifyErr := scanForConfig(ctx, cfg, false, false)
	if verifyErr != nil {
		return ok, fail, verifyErr
	}
	if hasInconclusive(updates2) {
		return ok, fail, &ExitError{Code: 2, Err: fmt.Errorf("post-update scan contains errors or unverified sources")}
	}
	stats := PrintVerifyReport(updates2, results, ok, fail, skipped)

	if shouldFailExit(cfg, stats) {
		if fail > 0 {
			return ok, fail, fmt.Errorf("%d update(s) failed", fail)
		}
		return ok, fail, fmt.Errorf("%d item(s) still outdated", stats.remaining)
	}
	return ok, fail, nil
}

func prepareBatches(ctx context.Context, items []*model.Item) (map[model.Category]*updater.PreparedUpdateBatch, error) {
	batches := make(map[model.Category]*updater.PreparedUpdateBatch)
	for cat, group := range groupByCategory(items) {
		batch, err := prepareUpdateBatch(ctx, cat, group)
		if err != nil {
			return nil, fmt.Errorf("plan update commands for %s: %w", cat, err)
		}
		batches[cat] = batch
	}
	return batches, nil
}

func printPreparedDryRun(batches map[model.Category]*updater.PreparedUpdateBatch) {
	fmt.Println("dry-run: planned update commands:")
	for _, cat := range sortedPreparedCategories(batches) {
		prepared := batches[cat]
		plans := prepared.Plans()
		for _, plan := range plans {
			prefix := ""
			if plan.Elevated {
				prefix = "sudo "
			}
			if plan.Manual != "" {
				fmt.Printf("  • %s: manual — %s\n", cat, plan.Manual)
				continue
			}
			fmt.Printf("  • [%s] %s%s %s\n", plan.Scope, prefix, plan.Name, strings.Join(plan.Args, " "))
		}
	}
}

func sortedPreparedCategories(batches map[model.Category]*updater.PreparedUpdateBatch) []model.Category {
	cats := make([]model.Category, 0, len(batches))
	for cat := range batches {
		cats = append(cats, cat)
	}
	sort.Slice(cats, func(i, j int) bool { return cats[i] < cats[j] })
	return cats
}

// RunClean runs cleanup operations.
func RunClean(ctx context.Context, cfg Config) (int, int, error) {
	plat := detectPlatform()
	fmt.Printf(msgScanning, platformLabel(plat))
	updates, cleanup, _, err := scanForConfig(ctx, cfg, true, true)
	if err != nil {
		return 0, 0, err
	}
	if err := requireConclusive(updates, cleanup); err != nil {
		return 0, 0, err
	}

	items := collectCleanable(cleanup, cfg.Only)
	if len(items) == 0 {
		if hasInformational(updates, cleanup) {
			fmt.Println("ℹ No cleanup selected; installed inventory was not affirmatively verified")
		} else {
			fmt.Println("✓ Nothing to clean")
		}
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

func hasInformational(groups ...[]*model.SourceSummary) bool {
	for _, summaries := range groups {
		for _, summary := range summaries {
			if summary != nil {
				for _, item := range summary.Items {
					if item != nil && item.Status == model.StatusInfo {
						return true
					}
				}
			}
		}
	}
	return false
}

// RunAll updates then cleans.
func RunAll(ctx context.Context, cfg Config) error {
	updates, cleanup, _, err := scanForConfig(ctx, cfg, true, false)
	if err != nil {
		return err
	}
	if hasInconclusive(updates) || hasInconclusive(cleanup) {
		return &ExitError{Code: 2, Err: fmt.Errorf("scan contains errors or unverified sources")}
	}
	uok, ufail, err := RunUpdate(ctx, cfg)
	if err != nil {
		return err
	}
	cok, cfail, cerr := RunClean(ctx, cfg)
	if cerr != nil && cfail == 0 {
		return cerr
	}
	if ufail > 0 || cfail > 0 {
		return fmt.Errorf("finished with %d update fail(s), %d clean fail(s)", ufail, cfail)
	}
	if uok == 0 && cok == 0 && !hasInformational(updates, cleanup) {
		fmt.Println("✓ Everything is up to date and clean!")
	} else if uok == 0 && cok == 0 {
		fmt.Println("ℹ No action selected; installed inventory was not affirmatively verified")
	}
	return nil
}

func hasInconclusive(summaries []*model.SourceSummary) bool {
	for _, summary := range summaries {
		if summary == nil {
			continue
		}
		if summary.ErrorCount > 0 || summary.Unverified > 0 {
			return true
		}
		for _, item := range summary.Items {
			if item != nil && (item.Status == model.StatusError || item.Status == model.StatusUnverified) {
				return true
			}
		}
	}
	return false
}

type cleanGroup struct {
	label string
	items []*model.Item
}

func runCleanBatches(ctx context.Context, summaries []*model.SourceSummary, items []*model.Item, opts cleaner.Options) (ok, fail int, freed int64) {
	for _, g := range groupCleanBySummary(summaries, items) {
		fmt.Printf("\n→ %s (%d item(s))\n", g.label, len(g.items))
		for _, it := range g.items {
			printCleanItemDetail(it)
			o, f, bytes := runOneClean(ctx, it, opts)
			ok += o
			fail += f
			freed += bytes
		}
	}
	return ok, fail, freed
}

func printCleanItemDetail(it *model.Item) {
	detail := it.Name
	if it.Reclaimable != "" {
		detail = fmt.Sprintf("%s (~%s reclaimable)", it.Name, it.Reclaimable)
	}
	if it.KeepPolicy != "" {
		detail = fmt.Sprintf("%s  [%s]", detail, it.KeepPolicy)
	}
	fmt.Printf("  • %s\n", detail)
}

func runOneClean(ctx context.Context, it *model.Item, opts cleaner.Options) (ok, fail int, freed int64) {
	itemCtx, cancel := context.WithTimeout(ctx, cleaner.ItemTimeout(it))
	r := cleanOneFn(itemCtx, it, opts)
	cancel()
	if !r.Success {
		errMsg := r.Error
		if errMsg == "" {
			errMsg = "failed"
		}
		fmt.Printf(msgItemFail, it.Name, errMsg)
		return 0, 1, 0
	}
	if r.BytesFreed > 0 {
		fmt.Printf("  ✓ %s (freed %s)\n", it.Name, formatBytesFn(r.BytesFreed))
	} else {
		fmt.Printf("  ✓ %s (nothing to remove)\n", it.Name)
	}
	return 1, 0, r.BytesFreed
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

// updateBatchEnv groups shared state for update-section helpers (keeps
// function parameter counts under Sonar S107).
type updateBatchEnv struct {
	plat        model.PlatformInfo
	summaries   []*model.SourceSummary
	prepared    map[model.Category]*updater.PreparedUpdateBatch
	opts        updater.Options
	cfg         Config
	elevSession **elevate.Session
}

func runUpdateBatches(
	ctx context.Context,
	plat model.PlatformInfo,
	summaries []*model.SourceSummary,
	items []*model.Item,
	prepared map[model.Category]*updater.PreparedUpdateBatch,
	opts updater.Options,
	cfg Config,
) (ok, fail, skipped int, allResults []*updater.Result) {
	nativeItems, normalItems := partitionNativeElevated(plat, items, cfg)
	var elevSession *elevate.Session
	ctx = primeElevationSessionForBatches(ctx, plat, prepared, cfg, &elevSession)

	env := updateBatchEnv{
		plat:        plat,
		summaries:   summaries,
		prepared:    prepared,
		opts:        opts,
		cfg:         cfg,
		elevSession: &elevSession,
	}

	o, f, sk, res := runNativeUpdateSection(ctx, env, nativeItems)
	ok, fail, skipped = o, f, sk
	allResults = append(allResults, res...)

	groups := groupByCategory(normalItems)
	for _, cat := range sortedCategories(groups) {
		o, f, sk, res := runCategoryUpdateSection(ctx, env, cat, groups[cat])
		ok += o
		fail += f
		skipped += sk
		allResults = append(allResults, res...)
	}
	return ok, fail, skipped, allResults
}

func tallyUpdateResults(results []*updater.Result) (ok, fail, skipped int) {
	for _, r := range results {
		switch {
		case r.Success:
			fmt.Printf("  ✓ %s\n", r.Item.Name)
			ok++
		case isSkippedResult(r):
			fmt.Printf("  ⊘ %s: %s\n", r.Item.Name, strings.TrimPrefix(r.Error, "⊘ "))
			skipped++
		default:
			errMsg := r.Error
			if errMsg == "" {
				errMsg = "failed"
			}
			fmt.Printf(msgItemFail, r.Item.Name, errMsg)
			fail++
		}
	}
	return ok, fail, skipped
}

func runNativeUpdateSection(
	ctx context.Context,
	env updateBatchEnv,
	nativeItems []*model.Item,
) (ok, fail, skipped int, results []*updater.Result) {
	if len(nativeItems) == 0 {
		return 0, 0, 0, nil
	}
	fmt.Printf("\n→ 🔐 Privileged updates (%d item(s))\n", len(nativeItems))
	for _, it := range nativeItems {
		fmt.Printf("  • %s\n", it.Name)
	}
	results = runNativeElevatedItems(ctx, env.plat, nativeItems, env.opts, env.cfg, env.elevSession)
	ok, fail, skipped = tallyUpdateResults(results)
	return ok, fail, skipped, results
}

func runCategoryUpdateSection(
	ctx context.Context,
	env updateBatchEnv,
	cat model.Category,
	groupItems []*model.Item,
) (ok, fail, skipped int, results []*updater.Result) {
	label := categoryLabel(env.summaries, cat)
	fmt.Printf("\n→ %s (%d item(s))\n", label, len(groupItems))

	batchCtx, cancel := context.WithTimeout(ctx, updater.BatchTimeout(cat))
	defer cancel()

	if cat == model.CatBrew {
		results = runBrewUpdateBatch(batchCtx, groupItems, env.opts, env.cfg, env.elevSession)
	} else {
		prepared := env.prepared[cat]
		if prepared == nil {
			results = updatePlanErrorResults(groupItems, fmt.Errorf("missing prepared update batch"))
		} else if !updater.PlansRequireElevation(prepared.Plans()) {
			results = executePreparedBatch(batchCtx, prepared, env.opts)
		} else {
			elevCtx, batchSkipped, skipReason := ensurePlannedElevation(batchCtx, updater.PlansRequireElevation(prepared.Plans()), env.cfg, env.elevSession)
			if batchSkipped {
				results = skipBatchResults(groupItems, skipReason)
			} else {
				results = executePreparedBatch(elevCtx, prepared, env.opts)
			}
		}
	}
	ok, fail, skipped = tallyUpdateResults(results)
	return ok, fail, skipped, results
}

func updatePlanErrorResults(items []*model.Item, err error) []*updater.Result {
	results := make([]*updater.Result, 0, len(items))
	for _, item := range items {
		results = append(results, &updater.Result{Item: item, Error: err.Error()})
	}
	return results
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
	if it != nil {
		if strings.EqualFold(string(it.Category), o) {
			return true
		}
		if strings.Contains(strings.ToLower(it.Name), o) {
			return true
		}
	}
	if strings.Contains(strings.ToLower(s.Label), o) {
		return true
	}
	return false
}

// filterCheckResults applies --only to scan summaries: a summary matching
// by category/label is kept whole, otherwise only matching items survive
// and emptied summaries are dropped.
func filterCheckResults(summaries []*model.SourceSummary, only string) []*model.SourceSummary {
	if strings.TrimSpace(only) == "" {
		return summaries
	}
	var out []*model.SourceSummary
	for _, s := range summaries {
		if itemMatchesFilter(s, nil, only) {
			out = append(out, s)
			continue
		}
		var kept []*model.Item
		for _, it := range s.Items {
			if itemMatchesFilter(s, it, only) {
				kept = append(kept, it)
			}
		}
		if len(kept) == 0 {
			continue
		}
		clone := *s
		clone.Items = kept
		out = append(out, &clone)
	}
	return out
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
	if canElevateNP(ctx) {
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
