// Package updater executes update commands for selected items.
package updater

import (
	"bytes"
	"context"
	"errors"
	"fmt"
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
		batchResult := updateBatch(batchCtx, cat, groups[cat], opts)
		cancel()
		results = append(results, batchResult...)
	}
	return results
}

// UpdateCategory updates one category batch (used by CLI for per-step progress).
func UpdateCategory(ctx context.Context, cat model.Category, items []*model.Item, opts Options) []*Result {
	return updateBatch(ctx, cat, items, opts)
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
		groups[it.Category] = append(groups[it.Category], it)
	}
	return groups
}

// updateBatch processes a group of items of the same category.
func updateBatch(ctx context.Context, cat model.Category, items []*model.Item, opts Options) []*Result {
	switch cat {
	case model.CatBrew:
		return batchBrewUpgrade(ctx, items, opts)
	case model.CatMAS:
		return batchMASUpgrade(ctx, items, opts)
	case model.CatApt:
		return batchAptUpgrade(ctx, items, opts)
	case model.CatPacman:
		return batchPacmanUpgrade(ctx, items, opts)
	case model.CatWinget:
		return batchWingetUpgrade(ctx, items, opts)
	case model.CatChoco:
		return batchChocoUpgrade(ctx, items, opts)
	case model.CatScoop:
		return batchScoopUpgrade(ctx, items, opts)
	case model.CatNpm:
		return batchNpmUpgrade(ctx, items, opts)
	case model.CatOpenCodePlugins:
		return batchOpenCodePlugins(ctx, items, opts)
	case model.CatPipx:
		return batchPipxUpgrade(ctx, items, opts)
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

	cmd := exec.CommandContext(itemCtx, "brew", "upgrade", "--greedy", item.Name)
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
	item.CurrentVer = truncatePlainDiagnosis(result.Error)
	item.Log = output
	return result
}

func truncatePlainDiagnosis(msg string) string {
	if len(msg) <= 72 {
		return msg
	}
	return msg[:72] + "…"
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
	args := []string{"update"}
	if item.PackageID != "" {
		args = append(args, item.PackageID)
	}

	cmd := exec.CommandContext(ctx, "mas", args...)
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
	if err != nil {
		item.CurrentVer = truncatePlainDiagnosis(result.Error)
	}
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

// batchAptUpgrade runs apt-get dist-upgrade.
func batchAptUpgrade(ctx context.Context, items []*model.Item, opts Options) []*Result {
	for _, it := range items {
		it.Status = model.StatusUpdating
	}

	cmds := [][]string{
		{"apt-get", "update"},
		{"apt-get", "dist-upgrade", "-y"},
	}

	var allOutput strings.Builder
	var lastErr error
	for _, args := range cmds {
		cmd := elevate.Sudo(ctx, args[0], args[1:]...)
		var out []byte
		var err error
		if opts.Verbose || opts.Interactive {
			opts.ConfigureCmd(cmd)
			err = cmd.Run()
		} else {
			out, err = cmd.CombinedOutput()
			allOutput.Write(out)
		}
		if err != nil {
			lastErr = err
			fmt.Fprintf(&allOutput, "error: %s\n", err)
		}
	}

	results := make([]*Result, len(items))
	success := lastErr == nil
	for i, it := range items {
		results[i] = &Result{
			Item:    it,
			Success: success,
			Output:  allOutput.String(),
		}
		if success {
			it.Status = model.StatusDone
		} else {
			it.Status = model.StatusError
			results[i].Error = lastErr.Error()
		}
	}

	return results
}

// batchPacmanUpgrade runs yay/pacman -Syu.
func batchPacmanUpgrade(ctx context.Context, items []*model.Item, opts Options) []*Result {
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
	for _, it := range items {
		it.Status = model.StatusUpdating
	}
	cmd := exec.CommandContext(ctx, "winget", "upgrade", "--all",
		"--accept-package-agreements", "--accept-source-agreements")
	return batchMarkAll(items, runCmdWithBuilder(ctx, items[0], cmd, opts))
}

func batchChocoUpgrade(ctx context.Context, items []*model.Item, opts Options) []*Result {
	for _, it := range items {
		it.Status = model.StatusUpdating
	}
	cmd := exec.CommandContext(ctx, "choco", "upgrade", "all", "-y")
	return batchMarkAll(items, runCmdWithBuilder(ctx, items[0], cmd, opts))
}

func batchScoopUpgrade(ctx context.Context, items []*model.Item, opts Options) []*Result {
	for _, it := range items {
		it.Status = model.StatusUpdating
	}
	cmd := exec.CommandContext(ctx, "scoop", "update", "*")
	return batchMarkAll(items, runCmdWithBuilder(ctx, items[0], cmd, opts))
}

func batchNpmUpgrade(ctx context.Context, items []*model.Item, opts Options) []*Result {
	for _, it := range items {
		it.Status = model.StatusUpdating
	}
	cmd := npmUpdateCmd(ctx)
	return batchMarkAll(items, runCmdWithBuilder(ctx, items[0], cmd, opts))
}

// npmUpdateCmd runs global npm update; uses sudo when prefix is system-wide (/usr).
func npmUpdateCmd(ctx context.Context) *exec.Cmd {
	if npmGlobalNeedsSudo(ctx) {
		return elevate.Sudo(ctx, "npm", "update", "-g")
	}
	return exec.CommandContext(ctx, "npm", "update", "-g")
}

func npmGlobalNeedsSudo(ctx context.Context) bool {
	if elevate.CanElevateWithoutPassword(ctx) || elevate.FromContext(ctx) != nil {
		out, err := exec.CommandContext(ctx, "npm", "config", "get", "prefix").Output()
		if err != nil {
			return elevate.CanElevateWithoutPassword(ctx)
		}
		prefix := strings.TrimSpace(string(out))
		return strings.HasPrefix(prefix, "/usr")
	}
	return false
}

func batchPipxUpgrade(ctx context.Context, items []*model.Item, opts Options) []*Result {
	for _, it := range items {
		it.Status = model.StatusUpdating
	}
	cmd := exec.CommandContext(ctx, "pipx", "upgrade-all")
	return batchMarkAll(items, runCmdWithBuilder(ctx, items[0], cmd, opts))
}

func batchMarkAll(items []*model.Item, single *Result) []*Result {
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

// updateOne runs the appropriate update command for a single item.
func updateOne(ctx context.Context, item *model.Item, opts Options) *Result {
	item.Status = model.StatusUpdating

	switch item.Category {
	case model.CatFlatpak:
		return runCmd(ctx, item, opts, "flatpak", "update", "-y")
	case model.CatSnap:
		return runElevatedCmd(ctx, item, opts, "snap", "refresh")
	case model.CatGo:
		return runCmd(ctx, item, opts, "gup", "update")
	case model.CatRustup:
		return runCmd(ctx, item, opts, "rustup", "update")
	case model.CatCargo:
		return runCmd(ctx, item, opts, "cargo", "install-update", "-a")
	case model.CatSDKMAN:
		return runSDKMANUpgrade(ctx, item, opts)
	case model.CatNvm:
		return runCmd(ctx, item, opts, "bash", "-c", "source $HOME/.nvm/nvm.sh && nvm install-latest-npm")
	case model.CatOmz:
		return runCmd(ctx, item, opts, "bash", "-c", "source $HOME/.oh-my-zsh/tools/upgrade.sh")
	case model.CatAgent:
		return updateAgent(ctx, item, opts)
	case model.CatGHExt:
		return runCmd(ctx, item, opts, "gh", "extension", "upgrade", "--all")
	case model.CatAI:
		return updateAIInfra(ctx, item, opts)
	default:
		return &Result{
			Item:    item,
			Success: false,
			Error:   fmt.Sprintf("no updater for category %s", item.Category),
		}
	}
}

func runElevatedCmd(ctx context.Context, item *model.Item, opts Options, name string, args ...string) *Result {
	return runCmdWithBuilder(ctx, item, elevate.Sudo(ctx, name, args...), opts)
}

func runCmd(ctx context.Context, item *model.Item, opts Options, name string, args ...string) *Result {
	return runCmdWithBuilder(ctx, item, exec.CommandContext(ctx, name, args...), opts)
}

func runCmdWithBuilder(ctx context.Context, item *model.Item, cmd *exec.Cmd, opts Options) *Result {
	var stdout, stderr bytes.Buffer
	if opts.Verbose || opts.Interactive {
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
	bashCmd := `
		source $HOME/.sdkman/bin/sdkman-init.sh
		echo "Y" | sdk upgrade
	`
	return runCmd(ctx, item, opts, "bash", "-c", bashCmd)
}

func updateAgent(ctx context.Context, item *model.Item, opts Options) *Result {
	switch {
	case strings.Contains(item.Name, "Claude"):
		return runCmd(ctx, item, opts, "claude", "update")
	case strings.Contains(item.Name, "OpenCode"):
		return runCmd(ctx, item, opts, "opencode", "upgrade")
	case strings.Contains(item.Name, "Grok"):
		return runCmd(ctx, item, opts, "grok", "update")
	case strings.Contains(item.Name, "Gemini"):
		return runCmd(ctx, item, opts, "gemini", "update")
	case strings.Contains(item.Name, "Codex"):
		return updateAgentViaNpm(ctx, item, opts, "@openai/codex")
	case strings.Contains(item.Name, "Copilot"):
		return runCmd(ctx, item, opts, "copilot", "update")
	default:
		reason := item.KeepPolicy
		if reason == "" {
			reason = "manual reinstall / app update"
		}
		item.Status = model.StatusOutdated
		return &Result{
			Item:    item,
			Success: false,
			Error:   "⊘ " + reason,
			Output:  fmt.Sprintf("%s: %s", item.Name, reason),
		}
	}
}

func updateAgentViaNpm(ctx context.Context, item *model.Item, opts Options, pkg string) *Result {
	if item.PackageID != "" {
		pkg = item.PackageID
	}
	return runCmd(ctx, item, opts, "npm", "install", "-g", pkg+"@latest")
}

// batchOpenCodePlugins updates local plugins under ~/.config/opencode.
func batchOpenCodePlugins(ctx context.Context, items []*model.Item, opts Options) []*Result {
	if len(items) == 0 {
		return nil
	}
	dir := scanner.OpenCodeConfigDir()
	for _, it := range items {
		it.Status = model.StatusUpdating
	}
	cmd := exec.CommandContext(ctx, "npm", "update", "--prefix", dir)
	return batchMarkAll(items, runCmdWithBuilder(ctx, items[0], cmd, opts))
}

func updateAIInfra(ctx context.Context, item *model.Item, opts Options) *Result {
	switch {
	case strings.Contains(item.Name, "ai-memory"):
		return runCmd(ctx, item, opts, "ai-memory", "upgrade")
	case strings.Contains(item.Name, "semidx"):
		return runCmd(ctx, item, opts, "semidx", "upgrade")
	case strings.Contains(item.Name, "gcloud"):
		return runCmd(ctx, item, opts, "gcloud", "components", "update", "--quiet")
	default:
		return &Result{
			Item:    item,
			Success: true,
			Output:  fmt.Sprintf("%s: no auto-update", item.Name),
		}
	}
}
