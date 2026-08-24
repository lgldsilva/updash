// Package updater executes update commands for selected items.
package updater

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/lgldsilva/updash/internal/elevate"
	"github.com/lgldsilva/updash/internal/model"
	"github.com/lgldsilva/updash/internal/scanner"
)

// Result holds the outcome of an update operation.
type Result struct {
	Item    *model.Item
	Success bool
	Output  string
	Error   string
}

// UpdateAll runs update commands for the given items (silent/buffered — for TUI).
func UpdateAll(ctx context.Context, items []*model.Item) []*Result {
	return UpdateAllWithOptions(ctx, items, SilentOptions())
}

// UpdateAllWithOptions runs updates with the given execution options.
func UpdateAllWithOptions(ctx context.Context, items []*model.Item, opts Options) []*Result {
	groups := groupByCategory(items)
	results := make([]*Result, 0, len(items))
	for _, cat := range sortedCategories(groups) {
		batchCtx, cancel := withBatchTimeout(ctx, cat)
		batchResult := UpdateCategory(batchCtx, cat, groups[cat], opts)
		cancel()
		results = append(results, batchResult...)
	}
	return results
}

// UpdateCategory updates one category batch (used by CLI for per-step progress).
func UpdateCategory(ctx context.Context, cat model.Category, items []*model.Item, opts Options) []*Result {
	prepared, err := PrepareUpdateBatch(ctx, cat, items)
	if err != nil {
		return failedBatch(validItems(items), err)
	}
	return ExecutePreparedBatch(ctx, prepared, opts)
}

// ExecutePreparedBatch executes exactly the plans captured by
// PrepareUpdateBatch. It never invokes the planner again.
func ExecutePreparedBatch(ctx context.Context, batch *PreparedUpdateBatch, opts Options) []*Result {
	if batch == nil || len(batch.items) == 0 {
		return nil
	}
	items, plans := batch.items, batch.plans
	switch batch.category {
	case model.CatNpm:
		return executePreparedNpm(ctx, items, plans, opts)
	case model.CatApt:
		return executePreparedApt(ctx, items, plans, opts)
	case model.CatWinget, model.CatPnpm, model.CatBun, model.CatPipx:
		return runExactPlans(ctx, items, plans, opts)
	default:
		// Legacy specialized paths retain their post-update verification. New
		// callers that need strict plan identity should use the prepared routes
		// above; npm is the environment-sensitive path that must never replan.
		return updateBatch(ctx, batch.category, items, opts)
	}
}

func executePreparedNpm(ctx context.Context, items []*model.Item, plans []CommandPlan, opts Options) []*Result {
	updatable, protected := partitionNpmItems(items)
	results := make([]*Result, 0, len(items))
	for _, item := range protected {
		item.Status, item.Log = model.StatusOK, npmManagedElsewhereNote
		results = append(results, &Result{Item: item, Success: true, Output: npmManagedElsewhereNote})
	}
	if len(updatable) == 0 {
		return results
	}
	if len(plans) != 1 {
		return append(results, failedBatch(updatable, fmt.Errorf("invalid prepared npm plan"))...)
	}
	return append(results, batchMarkAll(updatable, runCommandPlan(ctx, updatable[0], plans[0], opts))...)
}

func executePreparedApt(ctx context.Context, items []*model.Item, plans []CommandPlan, opts Options) []*Result {
	var output strings.Builder
	for _, plan := range plans {
		result := runCommandPlan(ctx, items[0], plan, opts)
		output.WriteString(result.Output)
		if !result.Success {
			return failedBatchWithOutput(items, result.Error, output.String())
		}
	}
	return batchMarkAll(items, &Result{Success: true, Output: output.String()})
}

func sortedCategories(groups map[model.Category][]*model.Item) []model.Category {
	cats := make([]model.Category, 0, len(groups))
	for cat := range groups {
		cats = append(cats, cat)
	}
	sort.Slice(cats, func(i, j int) bool { return cats[i] < cats[j] })
	return cats
}

// groupByCategory organizes items by their category.
func groupByCategory(items []*model.Item) map[model.Category][]*model.Item {
	groups := make(map[model.Category][]*model.Item)
	for _, it := range items {
		if it == nil {
			continue
		}
		groups[it.Category] = append(groups[it.Category], it)
	}
	return groups
}

// updateBatch processes a group of items of the same category.
func updateBatch(ctx context.Context, cat model.Category, items []*model.Item, opts Options) []*Result {
	if len(items) == 0 {
		return nil
	}
	switch cat {
	case model.CatBrew:
		return batchBrewUpgrade(ctx, items, opts)
	case model.CatMAS:
		return batchMASUpgrade(ctx, items, opts)
	case model.CatApt:
		return batchAptUpgrade(ctx, items, opts)
	case model.CatDnf:
		return batchPlannedCategory(ctx, cat, items, opts)
	case model.CatZypper:
		return batchPlannedCategory(ctx, cat, items, opts)
	case model.CatApk:
		return batchPlannedCategory(ctx, cat, items, opts)
	case model.CatPacman:
		return batchPlannedCategory(ctx, cat, items, opts)
	case model.CatWinget:
		return batchWingetUpgrade(ctx, items, opts)
	case model.CatChoco:
		return batchPlannedCategory(ctx, cat, items, opts)
	case model.CatScoop:
		return batchPlannedCategory(ctx, cat, items, opts)
	case model.CatNpm:
		return batchNpmUpgrade(ctx, items, opts)
	case model.CatPnpm:
		return batchPnpmUpgrade(ctx, items, opts)
	case model.CatBun:
		return batchBunUpgrade(ctx, items, opts)
	case model.CatOpenCodePlugins:
		return batchPlannedCategory(ctx, cat, items, opts)
	case model.CatPipx:
		return batchPipxUpgrade(ctx, items, opts)
	case model.CatFlatpak, model.CatSnap, model.CatGo, model.CatRustup, model.CatCargo,
		model.CatSDKMAN, model.CatNvm, model.CatOmz, model.CatGHExt:
		return batchPlannedCategory(ctx, cat, items, opts)
	case model.CatAgent, model.CatAI:
		return batchSequential(ctx, items, opts)
	default:
		return batchSequential(ctx, items, opts)
	}
}

func batchSequential(ctx context.Context, items []*model.Item, opts Options) []*Result {
	results := make([]*Result, len(items))
	for i, it := range items {
		results[i] = updateOne(ctx, it, opts)
	}
	return results
}

// batchBrewUpgrade upgrades each selected brew package individually and diagnoses failures.
// Never runs a bare "brew upgrade --greedy" (that would touch unrelated outdated casks).
func batchBrewUpgrade(ctx context.Context, items []*model.Item, opts Options) []*Result {
	results := make([]*Result, len(items))
	for i, it := range items {
		results[i] = upgradeOneBrew(ctx, it, opts)
	}
	return results
}

func upgradeOneBrew(ctx context.Context, item *model.Item, opts Options) *Result {
	item.Status = model.StatusUpdating

	itemCtx, cancel := context.WithTimeout(ctx, BrewItemTimeout(item.Name))
	defer cancel()

	cmd := exec.CommandContext(itemCtx, "brew", commandUpgrade, "--greedy", item.Name)
	if scanner.BrewNeedsSudoPrime(item.Name) && !opts.Interactive {
		cleanup, err := elevate.AttachSubprocessSudo(itemCtx, cmd)
		if err != nil {
			item.Status = model.StatusError
			return &Result{
				Item:    item,
				Success: false,
				Error:   err.Error() + " — informe a senha de admin no diálogo do updash",
			}
		}
		defer cleanup()
	}

	var stdout, stderr bytes.Buffer
	if opts.Output != nil {
		opts.ConfigureCmd(cmd)
	} else if opts.Verbose || opts.Interactive {
		opts.ConfigureCmd(cmd)
	} else {
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
	}

	runErr := cmd.Run()
	output := stdout.String() + stderr.String()
	timedOut := errors.Is(itemCtx.Err(), context.DeadlineExceeded)

	stillOutdated, verifyErr := brewVerifyAfterUpgrade(ctx)
	if verifyErr != nil {
		msg := fmt.Sprintf("could not verify brew upgrade: %v", verifyErr)
		item.Status = model.StatusError
		item.Log = output
		return &Result{Item: item, Success: false, Error: msg, Output: output}
	}

	_, still := stillOutdated[item.Name]
	result := &Result{Item: item, Output: output}

	if !still && runErr == nil {
		result.Success = true
		item.Status = model.StatusOK
		item.AvailableVer = ""
		item.Log = output
		return result
	}

	result.Success = false
	result.Error = explainBrewUpgradeFailure(item.Name, output, runErr, timedOut)
	item.Status = model.StatusError
	item.Log = output
	return result
}

// batchMASUpgrade updates each MAS app individually and verifies via mas outdated.
// mas manages its own sudo (see mas README) — do not wrap it in elevate.Sudo.
func batchMASUpgrade(ctx context.Context, items []*model.Item, opts Options) []*Result {
	results := make([]*Result, len(items))
	for i, it := range items {
		it.Status = model.StatusUpdating
		results[i] = upgradeMASApp(ctx, it, opts)
	}

	return results
}

func upgradeMASApp(ctx context.Context, item *model.Item, opts Options) *Result {
	plans, planErr := PlanUpdateCommands(model.CatMAS, []*model.Item{item})
	if planErr != nil || len(plans) != 1 {
		if planErr == nil {
			planErr = fmt.Errorf("invalid MAS update plan")
		}
		return failedBatch([]*model.Item{item}, planErr)[0]
	}
	plan := plans[0]
	if plan.Scope == CommandScopeManual {
		return manualAgentResult(item, plan.Manual)
	}
	cmd := exec.CommandContext(ctx, plan.Name, plan.Args...)
	if !opts.Interactive {
		cleanup, err := elevate.AttachSubprocessSudo(ctx, cmd)
		if err != nil {
			item.Status = model.StatusError
			return &Result{
				Item:    item,
				Success: false,
				Error:   err.Error() + " — informe a senha de admin no diálogo do updash",
			}
		}
		defer cleanup()
	}
	var stdout, stderr bytes.Buffer
	if opts.Output != nil {
		opts.ConfigureCmd(cmd)
	} else if opts.Verbose || opts.Interactive {
		opts.ConfigureCmd(cmd)
	} else {
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
	}

	err := cmd.Run()
	output := stdout.String() + stderr.String()
	stillOutdated := masStillOutdatedWithRetry(ctx, item)

	result := &Result{Item: item, Output: output}
	if err == nil && !stillOutdated {
		result.Success = true
		item.Status = model.StatusOK
		item.AvailableVer = ""
		return result
	}

	if err == nil {
		item.Status = model.StatusOutdated
	} else {
		item.Status = model.StatusError
	}
	result.Success = false
	result.Error = explainMasFailure(item.Name, item.PackageID, output, err)
	return result
}

type masOutdatedEntry struct {
	id   string
	name string
}

func masOutdatedEntries(ctx context.Context) []masOutdatedEntry {
	out, err := exec.CommandContext(ctx, "mas", "outdated").Output()
	if err != nil {
		return nil
	}
	var entries []masOutdatedEntry
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		name := strings.Join(parts[1:], " ")
		if idx := strings.Index(name, "("); idx >= 0 {
			name = strings.TrimSpace(name[:idx])
		}
		entries = append(entries, masOutdatedEntry{id: parts[0], name: name})
	}
	return entries
}

func brewVerifyAfterUpgrade(ctx context.Context) (map[string]struct{}, error) {
	delays := []time.Duration{0, 2 * time.Second, 5 * time.Second}
	var lastErr error
	for _, d := range delays {
		if d > 0 {
			select {
			case <-ctx.Done():
				if lastErr != nil {
					return nil, lastErr
				}
				return scanner.BrewOutdatedSet(ctx)
			case <-time.After(d):
			}
		}
		set, err := scanner.BrewOutdatedSet(ctx)
		if err == nil {
			return set, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

func masStillOutdatedWithRetry(ctx context.Context, item *model.Item) bool {
	delays := []time.Duration{0, 3 * time.Second, 8 * time.Second}
	for _, d := range delays {
		if d > 0 {
			select {
			case <-ctx.Done():
				return masStillOutdated(ctx, item)
			case <-time.After(d):
			}
		}
		if !masStillOutdated(ctx, item) {
			return false
		}
	}
	return true
}

func masStillOutdated(ctx context.Context, item *model.Item) bool {
	wantName := normalizeMASName(item.Name)
	for _, entry := range masOutdatedEntries(ctx) {
		if item.PackageID != "" && entry.id == item.PackageID {
			return true
		}
		if normalizeMASName(entry.name) == wantName {
			return true
		}
	}
	return false
}

// normalizeMASName strips invisible Unicode marks (e.g. RTL) from mas app names.
func normalizeMASName(s string) string {
	var b strings.Builder
	for _, r := range strings.TrimSpace(s) {
		if r == '\u200e' || r == '\u200f' || r == '\ufeff' {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// batchAptUpgrade refreshes package metadata then upgrades only the selection.
func batchAptUpgrade(ctx context.Context, items []*model.Item, opts Options) []*Result {
	if len(items) == 0 {
		return nil
	}
	for _, it := range items {
		it.Status = model.StatusUpdating
	}

	plans, planErr := PlanUpdateCommands(model.CatApt, items)
	if planErr != nil {
		return failedBatch(items, planErr)
	}

	var allOutput strings.Builder
	for _, plan := range plans {
		result := runCommandPlan(ctx, items[0], plan, opts)
		allOutput.WriteString(result.Output)
		if !result.Success {
			if result.Error != "" {
				fmt.Fprintf(&allOutput, "error: %s\n", result.Error)
			}
			return failedBatchWithOutput(items, result.Error, allOutput.String())
		}
	}
	return batchMarkAll(items, &Result{Success: true, Output: allOutput.String()})
}

// batchDnfUpgrade runs dnf upgrade (or yum update fallback) for all items.
func batchDnfUpgrade(ctx context.Context, items []*model.Item, opts Options) []*Result {
	if len(items) == 0 {
		return nil
	}
	for _, it := range items {
		it.Status = model.StatusUpdating
	}

	tool := scanner.RpmToolName()
	var args []string
	switch tool {
	case "yum":
		args = []string{commandUpdate, flagYes}
	default:
		args = []string{commandUpgrade, "--refresh", flagYes}
	}
	cmd := elevate.Sudo(ctx, tool, args...)
	return batchMarkAll(items, runCmdWithBuilder(ctx, items[0], cmd, opts))
}

// batchZypperUpgrade runs a non-interactive zypper update for all items.
func batchZypperUpgrade(ctx context.Context, items []*model.Item, opts Options) []*Result {
	if len(items) == 0 {
		return nil
	}
	for _, it := range items {
		it.Status = model.StatusUpdating
	}
	cmd := elevate.Sudo(ctx, "zypper", "--non-interactive", commandUpdate)
	return batchMarkAll(items, runCmdWithBuilder(ctx, items[0], cmd, opts))
}

// batchApkUpgrade runs apk upgrade; sudo only when not already root
// (Alpine containers often run as root without sudo installed).
func batchApkUpgrade(ctx context.Context, items []*model.Item, opts Options) []*Result {
	if len(items) == 0 {
		return nil
	}
	for _, it := range items {
		it.Status = model.StatusUpdating
	}
	var cmd *exec.Cmd
	if os.Geteuid() == 0 {
		cmd = exec.CommandContext(ctx, "apk", commandUpgrade)
	} else {
		cmd = elevate.Sudo(ctx, "apk", commandUpgrade)
	}
	return batchMarkAll(items, runCmdWithBuilder(ctx, items[0], cmd, opts))
}

// batchPacmanUpgrade runs yay/pacman -Syu.
func batchPacmanUpgrade(ctx context.Context, items []*model.Item, opts Options) []*Result {
	if len(items) == 0 {
		return nil
	}
	for _, it := range items {
		it.Status = model.StatusUpdating
	}

	var cmd *exec.Cmd
	if _, err := exec.LookPath("yay"); err == nil {
		cmd = exec.CommandContext(ctx, "yay", "-Syu", "--noconfirm")
	} else {
		cmd = elevate.Sudo(ctx, "pacman", "-Syu", "--noconfirm")
	}

	var stdout, stderr bytes.Buffer
	if opts.Verbose || opts.Interactive {
		opts.ConfigureCmd(cmd)
	} else {
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
	}
	err := cmd.Run()

	output := stdout.String() + stderr.String()
	success := err == nil
	results := make([]*Result, len(items))
	for i, it := range items {
		results[i] = &Result{
			Item:    it,
			Success: success,
			Output:  output,
		}
		if success {
			it.Status = model.StatusDone
		} else {
			it.Status = model.StatusError
			if err != nil {
				results[i].Error = err.Error()
			}
		}
	}

	return results
}

func batchWingetUpgrade(ctx context.Context, items []*model.Item, opts Options) []*Result {
	if len(items) == 0 {
		return nil
	}
	for _, it := range items {
		it.Status = model.StatusUpdating
	}
	plans, err := PlanUpdateCommands(model.CatWinget, items)
	if err != nil {
		return failedBatch(items, err)
	}
	return runExactPlans(ctx, items, plans, opts)
}

func wingetUpgradeArgs(items []*model.Item) []string {
	args := make([]string, 0, len(items)*2)
	for _, it := range items {
		if it == nil || it.PackageID == "" {
			continue
		}
		args = append(args, "--exact", "--id", it.PackageID)
	}
	return args
}

func batchChocoUpgrade(ctx context.Context, items []*model.Item, opts Options) []*Result {
	if len(items) == 0 {
		return nil
	}
	for _, it := range items {
		it.Status = model.StatusUpdating
	}
	args := append([]string{commandUpgrade}, chocoPackageNames(items)...)
	args = append(args, flagYes)
	cmd := exec.CommandContext(ctx, "choco", args...)
	return batchMarkAll(items, runCmdWithBuilder(ctx, items[0], cmd, opts))
}

func chocoPackageNames(items []*model.Item) []string {
	if len(items) == 0 {
		return []string{"all"}
	}
	names := make([]string, 0, len(items))
	for _, it := range items {
		name := it.Name
		if it.PackageID != "" {
			name = it.PackageID
		}
		names = append(names, name)
	}
	return names
}

func batchScoopUpgrade(ctx context.Context, items []*model.Item, opts Options) []*Result {
	if len(items) == 0 {
		return nil
	}
	for _, it := range items {
		it.Status = model.StatusUpdating
	}
	args := append([]string{commandUpdate}, scoopPackageNames(items)...)
	cmd := exec.CommandContext(ctx, "scoop", args...)
	return batchMarkAll(items, runCmdWithBuilder(ctx, items[0], cmd, opts))
}

func scoopPackageNames(items []*model.Item) []string {
	if len(items) == 0 {
		return []string{"*"}
	}
	names := make([]string, 0, len(items))
	for _, it := range items {
		names = append(names, it.Name)
	}
	return names
}

// npmManagedElsewhereNote explains why a protected npm item is skipped here.
const npmManagedElsewhereNote = "managed by opencode upgrade (single owner)"

func batchNpmUpgrade(ctx context.Context, items []*model.Item, opts Options) []*Result {
	for _, it := range items {
		it.Status = model.StatusUpdating
	}
	updatable, protected := partitionNpmItems(items)
	results := make([]*Result, 0, len(items))
	// Protected packages are owned by another path (opencode upgrade); never
	// let the generic npm batch touch them.
	for _, it := range protected {
		it.Status = model.StatusOK
		it.Log = npmManagedElsewhereNote
		results = append(results, &Result{Item: it, Success: true, Output: npmManagedElsewhereNote})
	}
	if len(updatable) == 0 {
		return results
	}
	plans, err := PlanUpdateCommands(model.CatNpm, updatable)
	if err != nil || len(plans) != 1 {
		if err == nil {
			err = fmt.Errorf("invalid npm update plan")
		}
		return append(results, failedBatch(updatable, err)...)
	}
	return append(results, batchMarkAll(updatable, runCommandPlan(ctx, updatable[0], plans[0], opts))...)
}

// partitionNpmItems splits a batch into packages the generic npm update may
// touch and protected ones owned by another update path. Pure (no I/O).
func partitionNpmItems(items []*model.Item) (updatable, protected []*model.Item) {
	for _, it := range items {
		if it == nil {
			continue
		}
		if scanner.IsProtectedNpmPackage(it.Name) {
			protected = append(protected, it)
		} else {
			updatable = append(updatable, it)
		}
	}
	return updatable, protected
}

// npmGlobalUpdateArgs builds the explicit package list for `npm update -g`:
// only the non-protected, deduplicated names. Targeting names instead of a bare
// `npm update -g` is what keeps protected packages out and matches the
// per-package model used by brew. Pure (no I/O).
func npmGlobalUpdateArgs(items []*model.Item) []string {
	seen := make(map[string]bool, len(items))
	names := make([]string, 0, len(items))
	for _, it := range items {
		if it == nil || it.Name == "" || seen[it.Name] {
			continue
		}
		seen[it.Name] = true
		names = append(names, it.Name)
	}
	if len(names) == 0 {
		return nil
	}
	return append([]string{commandUpdate, flagGlobal}, names...)
}

// npmUpdateCmd builds the global npm update command for the given (already
// filtered, non-protected) items: `npm update -g <names...> --allow-scripts=…`
// (with sudo when the global prefix is system-wide).
//
// npm >= 12 blocks dependency install scripts by default unless the package is
// covered by an allowScripts policy (RFC npm/rfcs#868): the install "succeeds"
// while postinstall is silently skipped, leaving a placeholder stub instead of
// the native binary. Pass --allow-scripts covering the names being updated so
// their lifecycle scripts still run; older npm versions ignore the key.
// npmAllowScriptsFlag builds the --allow-scripts flag covering every
// package name in the batch (deduplicated, empties dropped). Returns ""
// when there is nothing to allow.
func npmAllowScriptsFlag(items []*model.Item) string {
	seen := make(map[string]bool, len(items))
	names := make([]string, 0, len(items))
	for _, it := range items {
		if it == nil || it.Name == "" || seen[it.Name] {
			continue
		}
		seen[it.Name] = true
		names = append(names, it.Name)
	}
	if len(names) == 0 {
		return ""
	}
	return "--allow-scripts=" + strings.Join(names, ",")
}

func npmGlobalNeedsSudo(ctx context.Context) bool {
	elevated, err := npmGlobalElevation(ctx)
	return err == nil && elevated
}

// npmGlobalElevation determines the privilege requirement from npm's actual
// global prefix. It intentionally fails closed: sudo readiness is not evidence
// of where npm installs packages.
func npmGlobalElevation(ctx context.Context) (bool, error) {
	out, err := npmPrefixRunner(ctx)
	if err != nil {
		return false, fmt.Errorf("determine npm global prefix: %w", err)
	}
	prefix := strings.TrimSpace(string(out))
	if prefix == "" {
		return false, fmt.Errorf("determine npm global prefix: empty prefix")
	}
	return strings.HasPrefix(prefix, "/usr"), nil
}

// batchPnpmUpgrade updates all pnpm global packages.
func batchPnpmUpgrade(ctx context.Context, items []*model.Item, opts Options) []*Result {
	return batchPlannedExact(ctx, model.CatPnpm, items, opts)
}

// batchBunUpgrade updates all bun global packages.
func batchBunUpgrade(ctx context.Context, items []*model.Item, opts Options) []*Result {
	return batchPlannedExact(ctx, model.CatBun, items, opts)
}

func batchPipxUpgrade(ctx context.Context, items []*model.Item, opts Options) []*Result {
	return batchPlannedExact(ctx, model.CatPipx, items, opts)
}

func batchMarkAll(items []*model.Item, single *Result) []*Result {
	if len(items) == 0 || single == nil {
		return nil
	}
	results := make([]*Result, len(items))
	for i, it := range items {
		results[i] = &Result{
			Item:    it,
			Success: single.Success,
			Output:  single.Output,
			Error:   single.Error,
		}
		if single.Success {
			it.Status = model.StatusDone
		} else {
			it.Status = model.StatusError
		}
	}
	return results
}

func batchPlannedExact(ctx context.Context, cat model.Category, items []*model.Item, opts Options) []*Result {
	if len(items) == 0 {
		return nil
	}
	for _, item := range items {
		item.Status = model.StatusUpdating
	}
	plans, err := PlanUpdateCommands(cat, items)
	if err != nil {
		return failedBatch(items, err)
	}
	return runExactPlans(ctx, items, plans, opts)
}

func batchPlannedCategory(ctx context.Context, cat model.Category, items []*model.Item, opts Options) []*Result {
	plans, err := PlanUpdateCommands(cat, items)
	if err != nil {
		return failedBatch(items, err)
	}
	if len(plans) == 0 {
		return nil
	}
	if len(plans) == len(items) {
		return runExactPlans(ctx, items, plans, opts)
	}
	if len(plans) == 1 {
		return batchMarkAll(items, runCommandPlan(ctx, items[0], plans[0], opts))
	}
	return failedBatch(items, fmt.Errorf("internal command plan does not match selected items"))
}

func runExactPlans(ctx context.Context, items []*model.Item, plans []CommandPlan, opts Options) []*Result {
	if len(items) != len(plans) {
		return failedBatch(items, fmt.Errorf("internal command plan does not match selected items"))
	}
	results := make([]*Result, len(items))
	for i, plan := range plans {
		results[i] = runCommandPlan(ctx, items[i], plan, opts)
	}
	return results
}

func runCommandPlan(ctx context.Context, item *model.Item, plan CommandPlan, opts Options) *Result {
	if plan.Scope == CommandScopeManual {
		return manualAgentResult(item, plan.Manual)
	}
	if plan.Elevated {
		return runElevatedCmd(ctx, item, opts, plan.Name, plan.Args...)
	}
	return runCmd(ctx, item, opts, plan.Name, plan.Args...)
}

func failedBatch(items []*model.Item, err error) []*Result {
	return failedBatchWithOutput(items, err.Error(), "")
}

func failedBatchWithOutput(items []*model.Item, errText, output string) []*Result {
	results := make([]*Result, len(items))
	for i, item := range items {
		item.Status = model.StatusError
		results[i] = &Result{Item: item, Error: errText, Output: output}
	}
	return results
}

// updateOne runs the appropriate update command for a single item.
func updateOne(ctx context.Context, item *model.Item, opts Options) *Result {
	item.Status = model.StatusUpdating

	switch item.Category {
	case model.CatFlatpak:
		return updatePlannedOne(ctx, item, opts)
	case model.CatSnap:
		return updatePlannedOne(ctx, item, opts)
	case model.CatGo:
		return updatePlannedOne(ctx, item, opts)
	case model.CatRustup:
		return updatePlannedOne(ctx, item, opts)
	case model.CatCargo:
		return updatePlannedOne(ctx, item, opts)
	case model.CatSDKMAN:
		return runSDKMANUpgrade(ctx, item, opts)
	case model.CatNvm:
		if runtime.GOOS == "windows" {
			// nvm-windows is a different product; it has no install-latest-npm.
			return manualAgentResult(item, "update nvm-windows via its installer")
		}
		return runBashScript(ctx, item, opts,
			"source $HOME/.nvm/nvm.sh && nvm install-latest-npm",
			"install bash or update nvm manually")
	case model.CatOmz:
		return runBashScript(ctx, item, opts,
			"source $HOME/.oh-my-zsh/tools/upgrade.sh",
			"install bash or run the Oh My Zsh upgrade script manually")
	case model.CatAgent:
		return updateAgent(ctx, item, opts)
	case model.CatGHExt:
		return updatePlannedOne(ctx, item, opts)
	case model.CatAI:
		return updateAIInfra(ctx, item, opts)
	default:
		return updatePlannedOne(ctx, item, opts)
	}
}

func updatePlannedOne(ctx context.Context, item *model.Item, opts Options) *Result {
	plans, err := PlanUpdateCommands(item.Category, []*model.Item{item})
	if err != nil || len(plans) != 1 {
		if err == nil {
			err = fmt.Errorf("invalid command plan for category %s", item.Category)
		}
		return &Result{Item: item, Error: err.Error()}
	}
	return runCommandPlan(ctx, item, plans[0], opts)
}

func runElevatedCmd(ctx context.Context, item *model.Item, opts Options, name string, args ...string) *Result {
	stdout, stderr, err := runElevatedUpdateCmd(ctx, opts, name, args...)
	result := &Result{Item: item}
	if err != nil {
		result.Error = err.Error()
		result.Output = stderr + stdout
		item.Status = model.StatusError
		item.Log = result.Output
		return result
	}
	result.Success = true
	result.Output = stdout
	item.Status = model.StatusDone
	item.Log = result.Output
	return result
}

func runCmd(ctx context.Context, item *model.Item, opts Options, name string, args ...string) *Result {
	stdout, stderr, err := runUpdateCmd(ctx, opts, name, args...)
	result := &Result{Item: item}
	if err != nil {
		result.Success = false
		result.Error = err.Error()
		result.Output = stderr + stdout
		item.Status = model.StatusError
		item.Log = result.Output
	} else {
		result.Success = true
		result.Output = stdout
		item.Status = model.StatusDone
		item.Log = result.Output
	}
	return result
}

func runCmdWithBuilder(ctx context.Context, item *model.Item, cmd *exec.Cmd, opts Options) *Result {
	var stdout, stderr bytes.Buffer
	// opts.Output (TUI streaming) takes priority over buffering, same as brew.
	if opts.Output != nil || opts.Verbose || opts.Interactive {
		opts.ConfigureCmd(cmd)
	} else {
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
	}

	err := cmd.Run()
	result := &Result{Item: item}

	if err != nil {
		result.Success = false
		result.Error = err.Error()
		result.Output = stderr.String() + stdout.String()
		item.Status = model.StatusError
		item.Log = result.Output
	} else {
		result.Success = true
		result.Output = stdout.String()
		item.Status = model.StatusDone
		item.Log = result.Output
	}

	return result
}

func runSDKMANUpgrade(ctx context.Context, item *model.Item, opts Options) *Result {
	script := `
		source $HOME/.sdkman/bin/sdkman-init.sh
		echo "Y" | sdk upgrade
	`
	return runBashScript(ctx, item, opts, script, "install bash or run 'sdk upgrade' manually")
}

// runBashScript runs a shell-script update, degrading to a manual note when
// bash is unavailable (e.g. bare Windows hosts).
func runBashScript(ctx context.Context, item *model.Item, opts Options, script, manualNote string) *Result {
	if _, err := exec.LookPath("bash"); err != nil {
		return manualAgentResult(item, manualNote)
	}
	return runCmd(ctx, item, opts, "bash", "-c", script)
}

// manualAgentResult marks an item as skipped-manual without running anything.
func manualAgentResult(item *model.Item, reason string) *Result {
	item.Status = model.StatusOutdated
	return &Result{
		Item:    item,
		Success: false,
		Error:   "⊘ " + reason,
		Output:  fmt.Sprintf("%s: %s", item.Name, reason),
	}
}

func updateAgent(ctx context.Context, item *model.Item, opts Options) *Result {
	if cmd := scanner.AgentUpdateCommand(item.Name); len(cmd) > 0 {
		res := runCmd(ctx, item, opts, cmd[0], cmd[1:]...)
		switch item.Name {
		case agentClaudeCode:
			return ensureClaudeNativeBinary(ctx, item, res, cmd, opts)
		case agentOpenCode:
			return ensureOpenCodeHealthy(ctx, item, res)
		}
		return res
	}
	reason := item.KeepPolicy
	if reason == "" {
		reason = scanner.AgentKeepPolicy(item.Name)
	}
	if reason == "" {
		reason = "manual reinstall / app update"
	}
	return manualAgentResult(item, reason)
}

func updateAIInfra(ctx context.Context, item *model.Item, opts Options) *Result {
	if cmd := scanner.InfraUpdateCommand(item.Name); len(cmd) > 0 {
		return runCmd(ctx, item, opts, cmd[0], cmd[1:]...)
	}
	return &Result{
		Item:    item,
		Success: true,
		Output:  fmt.Sprintf("%s: no auto-update", item.Name),
	}
}
