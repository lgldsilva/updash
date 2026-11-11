// Package updater executes update commands for selected items.
package updater

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/lgldsilva/updash/internal/elevate"
	"github.com/lgldsilva/updash/internal/model"
	"github.com/lgldsilva/updash/internal/scanner"
)

// attachSubprocessSudo installs SUDO_ASKPASS for a child that calls sudo.
// Tests replace it so privileged brew/mas runs can be observed without sudo.
var attachSubprocessSudo = elevate.AttachSubprocessSudo

// Result holds the outcome of an update operation.
type Result struct {
	Item    *model.Item
	Success bool
	Output  string
	Error   string
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
		return executeSequentialPlans(ctx, items, plans, opts)
	case model.CatRustup:
		return executeSequentialPlans(ctx, items, plans, opts)
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
	return runPreparedNpm(ctx, items, plans, opts, "")
}

// ExecuteNpmSkippingElevated runs a prepared npm batch. When reason is
// non-empty, elevated prefix groups are skipped (their results start with
// "⊘ ") and every other group still runs. An empty reason runs every plan.
// Protected packages are marked success before either path.
func ExecuteNpmSkippingElevated(ctx context.Context, batch *PreparedUpdateBatch, opts Options, reason string) []*Result {
	if batch == nil || len(batch.items) == 0 {
		return nil
	}
	return runPreparedNpm(ctx, batch.items, batch.plans, opts, reason)
}

func runPreparedNpm(ctx context.Context, items []*model.Item, plans []CommandPlan, opts Options, skipElevatedReason string) []*Result {
	updatable, protected := partitionNpmItems(items)
	results := make([]*Result, 0, len(items))
	for _, item := range protected {
		item.Status, item.Log = model.StatusOK, npmManagedElsewhereNote
		results = append(results, &Result{Item: item, Success: true, Output: npmManagedElsewhereNote})
	}
	if len(updatable) == 0 {
		return results
	}
	// Plans and groups are zipped in groupNpmByPrefix order: one command per
	// prefix, each result pinned back to the items it targeted.
	groups := groupNpmByPrefix(updatable)
	if len(plans) != len(groups) {
		return append(results, failedBatch(updatable, fmt.Errorf("invalid prepared npm plan"))...)
	}
	for i, group := range groups {
		if skipElevatedReason != "" && plans[i].Elevated {
			results = append(results, skippedElevatedResults(group, skipElevatedReason)...)
			continue
		}
		results = append(results, batchMarkAll(group, runCommandPlan(ctx, group[0], plans[i], opts))...)
	}
	return results
}

func skippedElevatedResults(items []*model.Item, reason string) []*Result {
	msg := "⊘ " + reason
	results := make([]*Result, len(items))
	for i, it := range items {
		if it != nil {
			it.Status = model.StatusOutdated
		}
		results[i] = &Result{Item: it, Error: msg}
	}
	return results
}

// executeSequentialPlans runs every plan in order against the batch, fail-fast,
// then marks all items successful. Used when one category emits multiple
// category-global steps whose count need not match len(items) (apt refresh+
// install, rustup self update + toolchain update).
func executeSequentialPlans(ctx context.Context, items []*model.Item, plans []CommandPlan, opts Options) []*Result {
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

// annotateIdempotentAgentResult labels a zero-exit update that has no
// freshness channel. The exit code already decided success; an unrecognized
// payload stays a success so "already current" cannot become inconclusive.
func annotateIdempotentAgentResult(item *model.Item, result *Result) *Result {
	if result == nil || item == nil || !result.Success || !scanner.AgentUpdatesWithoutFreshness(item.Name) {
		return result
	}
	kind := scanner.ClassifyAgyUpdateOutput(result.Output)
	if kind == "" {
		return result
	}
	label := strings.Join(scanner.AgentUpdateCommand(item.Name), " ") + ": " + kind
	if result.Output != "" {
		result.Output = label + "\n" + result.Output
	} else {
		result.Output = label
	}
	item.Log = result.Output
	return result
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
			results[i] = annotateIdempotentAgentResult(item, npmBrewCollisionDowngrade(item, result))
		}
	}
	return results
}

// usePrivilegedAskpass wires SUDO_ASKPASS when needed. Interactive CLI runs
// use the same helper as headless ones: Homebrew executes each privileged
// cask step on its own PTY, so a terminal sudo ticket does not cover the
// next launchctl/pkg sudo. A passwordless session installs nothing.
func usePrivilegedAskpass(ctx context.Context, cmd *exec.Cmd, needed bool) (func(), error) {
	if cmd == nil || !needed {
		return func() {}, nil
	}
	return attachSubprocessSudo(ctx, cmd)
}

func upgradeOneBrewWithPlan(ctx context.Context, item *model.Item, plan CommandPlan, opts Options) *Result {
	item.Status = model.StatusUpdating
	if plan.Scope == CommandScopeManual {
		return manualAgentResult(item, plan.Manual)
	}

	itemCtx, cancel := context.WithTimeout(ctx, BrewItemTimeout(item.Name))
	defer cancel()

	cmd := commandContext(itemCtx, plan.Name, plan.Args...)
	cleanup, err := usePrivilegedAskpass(itemCtx, cmd, scanner.BrewNeedsSudoPrime(item.Name))
	if err != nil {
		item.Status = model.StatusError
		return &Result{
			Item:    item,
			Success: false,
			Error:   err.Error() + " — informe a senha de admin no diálogo do updash",
		}
	}
	defer cleanup()

	stdout, stderr := bufferCommandOutput(opts, cmd)

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

func upgradeMASAppWithPlan(ctx context.Context, item *model.Item, plan CommandPlan, opts Options) *Result {
	if plan.Scope == CommandScopeManual {
		return manualAgentResult(item, plan.Manual)
	}
	cmd := commandContext(ctx, plan.Name, plan.Args...)
	cleanup, err := usePrivilegedAskpass(ctx, cmd, true)
	if err != nil {
		item.Status = model.StatusError
		return &Result{
			Item:    item,
			Success: false,
			Error:   err.Error() + " — informe a senha de admin no diálogo do updash",
		}
	}
	defer cleanup()
	stdout, stderr := bufferCommandOutput(opts, cmd)

	err = cmd.Run()
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
	out, err := commandContext(ctx, "mas", "outdated").Output()
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
				return brewOutdatedSet(ctx)
			case <-time.After(d):
			}
		}
		set, err := brewOutdatedSet(ctx)
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

// npmManagedElsewhereNote explains why a protected npm item is skipped here.
const npmManagedElsewhereNote = "managed by its agent updater (single owner)"

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

// bufferCommandOutput streams child output to the terminal or TUI writer when
// one is configured and always keeps a buffer of both streams. Diagnosis
// reads that text: without the buffer a verbose run reduced every failure to
// a bare "exit status 1" and hid refusals such as disabled casks.
func bufferCommandOutput(opts Options, cmd *exec.Cmd) (stdout, stderr *bytes.Buffer) {
	var out, er bytes.Buffer
	opts.ConfigureCmd(cmd)
	// ConfigureCmd points both streams at one TUI writer. exec only shares a
	// single copy goroutine when Stdout and Stderr are the identical Writer,
	// and wireCapture has to wrap each one, so the shared sink is serialized.
	if cmd.Stdout != nil && cmd.Stdout == cmd.Stderr {
		shared := &lockedWriter{w: cmd.Stdout}
		cmd.Stdout = shared
		cmd.Stderr = shared
	}
	wireCapture(cmd, &out, &er)
	return &out, &er
}

// lockedWriter serializes writes. The shared TUI/terminal sink is not safe
// for the two goroutines exec uses to copy stdout and stderr.
type lockedWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (w *lockedWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.w == nil {
		return len(p), nil
	}
	return w.w.Write(p)
}

// wireCapture tees whatever live writer ConfigureCmd attached into the
// buffers. A nil writer means buffer-only (silent runs).
func wireCapture(cmd *exec.Cmd, stdout, stderr *bytes.Buffer) {
	if cmd.Stdout != nil {
		cmd.Stdout = io.MultiWriter(cmd.Stdout, stdout)
	} else {
		cmd.Stdout = stdout
	}
	if cmd.Stderr != nil {
		cmd.Stderr = io.MultiWriter(cmd.Stderr, stderr)
	} else {
		cmd.Stderr = stderr
	}
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
