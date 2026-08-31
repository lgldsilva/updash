// Package updater executes update commands for selected items.
package updater

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
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
	case model.CatBrew:
		return executePreparedBrew(ctx, items, plans, opts)
	case model.CatMAS:
		return executePreparedMAS(ctx, items, plans, opts)
	case model.CatNpm:
		return executePreparedNpm(ctx, items, plans, opts)
	case model.CatApt:
		return executePreparedApt(ctx, items, plans, opts)
	case model.CatAgent:
		return executePreparedAgents(ctx, items, plans, opts)
	case model.CatFlatpak:
		return executePreparedFlatpak(ctx, items, plans, opts)
	default:
		return executePreparedPlans(ctx, items, plans, opts)
	}
}

func executePreparedBrew(ctx context.Context, items []*model.Item, plans []CommandPlan, opts Options) []*Result {
	if len(items) != len(plans) {
		return failedBatch(items, fmt.Errorf("internal brew plan does not match selected items"))
	}
	results := make([]*Result, len(items))
	for i, plan := range plans {
		results[i] = upgradeOneBrewWithPlan(ctx, items[i], plan, opts)
	}
	return results
}

func executePreparedMAS(ctx context.Context, items []*model.Item, plans []CommandPlan, opts Options) []*Result {
	if len(items) != len(plans) {
		return failedBatch(items, fmt.Errorf("internal MAS plan does not match selected items"))
	}
	results := make([]*Result, len(items))
	for i, plan := range plans {
		results[i] = upgradeMASAppWithPlan(ctx, items[i], plan, opts)
	}
	return results
}

func executePreparedPlans(ctx context.Context, items []*model.Item, plans []CommandPlan, opts Options) []*Result {
	if len(plans) == len(items) {
		return runExactPlans(ctx, items, plans, opts)
	}
	if len(plans) == 1 {
		return batchMarkAll(items, runCommandPlan(ctx, items[0], plans[0], opts))
	}
	return failedBatch(items, fmt.Errorf("internal command plan does not match selected items"))
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

func executePreparedAgents(ctx context.Context, items []*model.Item, plans []CommandPlan, opts Options) []*Result {
	if len(items) != len(plans) {
		return failedBatch(items, fmt.Errorf("internal agent plan does not match selected items"))
	}
	results := make([]*Result, len(items))
	for i, item := range items {
		plan := plans[i]
		if plan.Scope == CommandScopeManual {
			results[i] = manualAgentResult(item, plan.Manual)
			continue
		}
		result := runAgentPlan(ctx, item, plan, opts)
		updateCmd := append([]string{plan.Name}, plan.Args...)
		switch item.Name {
		case agentClaudeCode:
			results[i] = ensureClaudeNativeBinary(ctx, item, result, updateCmd, opts)
		case agentOpenCode:
			results[i] = ensureOpenCodeHealthy(ctx, item, result)
		default:
			results[i] = result
		}
	}
	return results
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

func upgradeOneBrewWithPlan(ctx context.Context, item *model.Item, plan CommandPlan, opts Options) *Result {
	item.Status = model.StatusUpdating
	if plan.Scope == CommandScopeManual {
		return manualAgentResult(item, plan.Manual)
	}

	itemCtx, cancel := context.WithTimeout(ctx, BrewItemTimeout(item.Name))
	defer cancel()

	cmd := exec.CommandContext(itemCtx, plan.Name, plan.Args...)
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

	// No inactivity watchdog here: brew already has a per-item timeout, and a
	// long silent step (a .pkg installer) is expected. The zero window still
	// applies the shared non-interactive environment.
	runErr := runGuarded(cmd, 0)
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

func upgradeMASApp(ctx context.Context, item *model.Item, opts Options) *Result {
	plans, err := planUpdateCommands(ctx, model.CatMAS, []*model.Item{item})
	if err != nil || len(plans) != 1 {
		if err == nil {
			err = fmt.Errorf("invalid MAS update plan")
		}
		return failedBatch([]*model.Item{item}, err)[0]
	}
	return upgradeMASAppWithPlan(ctx, item, plans[0], opts)
}

func upgradeMASAppWithPlan(ctx context.Context, item *model.Item, plan CommandPlan, opts Options) *Result {
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

	plans, planErr := planUpdateCommands(ctx, model.CatApt, items)
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
	plans, err := planUpdateCommands(ctx, model.CatNpm, updatable)
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
	plans, err := planUpdateCommands(ctx, cat, items)
	if err != nil {
		return failedBatch(items, err)
	}
	return runExactPlans(ctx, items, plans, opts)
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

// runAgentPlan runs one agent plan under its own timeout so a single stuck
// agent cannot exhaust the shared CatAgent batch budget.
func runAgentPlan(ctx context.Context, item *model.Item, plan CommandPlan, opts Options) *Result {
	itemCtx, cancel := context.WithTimeout(ctx, AgentItemTimeout())
	defer cancel()
	return runCommandPlan(itemCtx, item, plan, opts)
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

	err := runGuarded(cmd, inactivityWindow)
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

// updateAgent updates a single agent through the same planner the prepared
// batch uses, so a dry-run and an execution can never disagree on the command.
func updateAgent(ctx context.Context, item *model.Item, opts Options) *Result {
	plans, err := agentPlans([]*model.Item{item})
	if err != nil || len(plans) != 1 {
		if err == nil {
			err = fmt.Errorf("invalid agent update plan for %q", item.Name)
		}
		item.Status = model.StatusError
		return &Result{Item: item, Error: err.Error()}
	}
	return executePreparedAgents(ctx, []*model.Item{item}, plans, opts)[0]
}
