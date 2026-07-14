// Package updater executes update commands for selected items.
package updater

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"sync"

	"github.com/lgldsilva/updash/internal/model"
)

// Result holds the outcome of an update operation.
type Result struct {
	Item    *model.Item
	Success bool
	Output  string
	Error   string
}

// UpdateAll runs update commands for the given items.
// Items of the same category that support batching are grouped into a single command.
func UpdateAll(ctx context.Context, items []*model.Item) []*Result {
	// Group by category for batch updates
	groups := groupByCategory(items)
	results := make([]*Result, 0, len(items))

	// Process each group
	for cat, groupItems := range groups {
		batchResult := updateBatch(ctx, cat, groupItems)
		results = append(results, batchResult...)
	}

	return results
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
func updateBatch(ctx context.Context, cat model.Category, items []*model.Item) []*Result {
	switch cat {
	case model.CatBrew:
		// Single brew upgrade --greedy for all
		return batchBrewUpgrade(ctx, items)
	case model.CatMAS:
		// Single mas upgrade for all
		return batchMASUpgrade(ctx, items)
	case model.CatApt:
		return batchAptUpgrade(ctx, items)
	case model.CatPacman:
		return batchPacmanUpgrade(ctx, items)
	default:
		// Run each item individually in parallel
		var wg sync.WaitGroup
		results := make([]*Result, len(items))
		for i, it := range items {
			wg.Add(1)
			go func(idx int, item *model.Item) {
				defer wg.Done()
				results[idx] = updateOne(ctx, item)
			}(i, it)
		}
		wg.Wait()
		return results
	}
}

// batchBrewUpgrade runs brew upgrade --greedy.
// Even if brew exits non-zero (warnings, validations), we verify which items
// were actually upgraded by re-checking brew outdated after the run.
func batchBrewUpgrade(ctx context.Context, items []*model.Item) []*Result {
	// Mark all as updating
	for _, it := range items {
		it.Status = model.StatusUpdating
	}

	// Run brew upgrade --greedy
	cmd := exec.CommandContext(ctx, "brew", "upgrade", "--greedy")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	_ = cmd.Run() // ignore exit code — we verify results below

	output := stdout.String() + stderr.String()

	// Check what's still outdated after the upgrade
	stillOutdated := brewOutdatedNames(ctx)

	results := make([]*Result, len(items))
	for i, it := range items {
		wasUpgraded := true
		for _, name := range stillOutdated {
			if name == it.Name {
				wasUpgraded = false
				break
			}
		}

		results[i] = &Result{Item: it, Output: output}
		if wasUpgraded {
			results[i].Success = true
			it.Status = model.StatusDone
		} else {
			results[i].Success = false
			results[i].Error = "still outdated after brew upgrade (needs manual fix or Toolbox)"
			it.Status = model.StatusError
		}
	}

	return results
}

// brewOutdatedNames returns the set of outdated brew packages after an upgrade.
func brewOutdatedNames(ctx context.Context) []string {
	out, err := exec.CommandContext(ctx, "brew", "outdated", "--greedy").Output()
	if err != nil {
		return nil
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	var names []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			names = append(names, line)
		}
	}
	return names
}

// batchMASUpgrade runs mas upgrade for all.
// Uses sudo -S for TTY-less environments (reads password via stdin pipe).
func batchMASUpgrade(ctx context.Context, items []*model.Item) []*Result {
	for _, it := range items {
		it.Status = model.StatusUpdating
	}

	// mas upgrade needs sudo to install apps
	cmd := exec.CommandContext(ctx, "sudo", "-S", "mas", "upgrade")
	cmd.Stdin = strings.NewReader("") // empty pipe to avoid sudo hang
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
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
			results[i].Error = fmt.Sprintf("mas upgrade: %v (sudo may need TTY)", err)
		}
	}

	return results
}

// batchAptUpgrade runs apt-get dist-upgrade.
func batchAptUpgrade(ctx context.Context, items []*model.Item) []*Result {
	for _, it := range items {
		it.Status = model.StatusUpdating
	}

	cmds := [][]string{
		{"sudo", "apt-get", "update"},
		{"sudo", "apt-get", "dist-upgrade", "-y"},
	}

	var allOutput strings.Builder
	var lastErr error
	for _, args := range cmds {
		cmd := exec.CommandContext(ctx, args[0], args[1:]...)
		out, err := cmd.CombinedOutput()
		allOutput.Write(out)
		if err != nil {
			lastErr = err
			allOutput.WriteString(fmt.Sprintf("error: %s\n", err))
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
func batchPacmanUpgrade(ctx context.Context, items []*model.Item) []*Result {
	for _, it := range items {
		it.Status = model.StatusUpdating
	}

	var cmd *exec.Cmd
	if _, err := exec.LookPath("yay"); err == nil {
		cmd = exec.CommandContext(ctx, "yay", "-Syu", "--noconfirm")
	} else {
		cmd = exec.CommandContext(ctx, "sudo", "pacman", "-Syu", "--noconfirm")
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
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
			results[i].Error = err.Error()
		}
	}

	return results
}

// updateOne runs the appropriate update command for a single item.
func updateOne(ctx context.Context, item *model.Item) *Result {
	item.Status = model.StatusUpdating

	switch item.Category {
	case model.CatFlatpak:
		return runCmd(ctx, item, "flatpak", "update", "-y")

	// Windows updaters
	case model.CatWinget:
		return runCmd(ctx, item, "winget", "upgrade", "--all", "--accept-package-agreements", "--accept-source-agreements")

	case model.CatChoco:
		// choco upgrade all -y
		return runCmd(ctx, item, "choco", "upgrade", "all", "-y")

	case model.CatScoop:
		// scoop update * updates all apps; scoop update itself first
		return runCmd(ctx, item, "scoop", "update", "*")
	case model.CatSnap:
		return runCmd(ctx, item, "sudo", "snap", "refresh")
	case model.CatNpm:
		return runCmd(ctx, item, "npm", "update", "-g")
	case model.CatPipx:
		return runCmd(ctx, item, "pipx", "upgrade-all")
	case model.CatGo:
		return runCmd(ctx, item, "gup", "update")
	case model.CatRustup:
		return runCmd(ctx, item, "rustup", "update")
	case model.CatCargo:
		return runCmd(ctx, item, "cargo", "install-update", "-a")
	case model.CatSDKMAN:
		return runSDKMANUpgrade(ctx, item)
	case model.CatNvm:
		return runCmd(ctx, item, "bash", "-c", "source $HOME/.nvm/nvm.sh && nvm install-latest-npm")
	case model.CatOmz:
		return runCmd(ctx, item, "bash", "-c", "source $HOME/.oh-my-zsh/tools/upgrade.sh")
	case model.CatAgent:
		return updateAgent(ctx, item)
	case model.CatGHExt:
		return runCmd(ctx, item, "gh", "extension", "upgrade", "--all")
	case model.CatAI:
		return updateAIInfra(ctx, item)
	default:
		return &Result{
			Item:    item,
			Success: false,
			Error:   fmt.Sprintf("no updater for category %s", item.Category),
		}
	}
}

func runCmd(ctx context.Context, item *model.Item, name string, args ...string) *Result {
	cmd := exec.CommandContext(ctx, name, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

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

func runSDKMANUpgrade(ctx context.Context, item *model.Item) *Result {
	bashCmd := `
		source $HOME/.sdkman/bin/sdkman-init.sh
		echo "Y" | sdk upgrade
	`
	return runCmd(ctx, item, "bash", "-c", bashCmd)
}

func updateAgent(ctx context.Context, item *model.Item) *Result {
	switch {
	case strings.Contains(item.Name, "Claude"):
		return runCmd(ctx, item, "claude", "update")
	case strings.Contains(item.Name, "Grok"):
		return runCmd(ctx, item, "grok", "update")
	case strings.Contains(item.Name, "Gemini"):
		return runCmd(ctx, item, "gemini", "update")
	default:
		return &Result{
			Item:    item,
			Success: true,
			Output:  fmt.Sprintf("%s: auto-update or manual reinstall needed", item.Name),
		}
	}
}

func updateAIInfra(ctx context.Context, item *model.Item) *Result {
	switch {
	case strings.Contains(item.Name, "ai-memory"):
		return runCmd(ctx, item, "ai-memory", "upgrade")
	case strings.Contains(item.Name, "semidx"):
		return runCmd(ctx, item, "semidx", "upgrade")
	case strings.Contains(item.Name, "gcloud"):
		return runCmd(ctx, item, "gcloud", "components", "update", "--quiet")
	default:
		return &Result{
			Item:    item,
			Success: true,
			Output:  fmt.Sprintf("%s: no auto-update", item.Name),
		}
	}
}
