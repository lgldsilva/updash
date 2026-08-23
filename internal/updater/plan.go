package updater

import (
	"context"
	"fmt"
	"runtime"

	"github.com/lgldsilva/updash/internal/model"
	"github.com/lgldsilva/updash/internal/scanner"
)

// CommandScope describes how broadly a planned command can affect installed software.
type CommandScope string

const (
	// CommandScopeExact affects only the packages named in Args.
	CommandScopeExact CommandScope = "exact"
	// CommandScopeCategoryGlobal affects a package-manager category once.
	CommandScopeCategoryGlobal CommandScope = "category-global"
	// CommandScopeManual is an intentional no-command/manual action.
	CommandScopeManual CommandScope = "manual"
)

// CommandPlan is a command that can be presented by a dry-run and then executed
// unchanged by the updater. Args never contain a shell fragment.
type CommandPlan struct {
	Name     string
	Args     []string
	Scope    CommandScope
	Elevated bool
	Manual   string
}

// PreparedUpdateBatch is an immutable snapshot of the selected items and the
// command plans approved during preflight. Use it for dry-run and execution so
// environment-sensitive decisions (notably npm elevation) are not recomputed.
type PreparedUpdateBatch struct {
	category model.Category
	items    []*model.Item
	plans    []CommandPlan
}

func (b *PreparedUpdateBatch) Category() model.Category { return b.category }
func (b *PreparedUpdateBatch) Items() []*model.Item     { return append([]*model.Item(nil), b.items...) }
func (b *PreparedUpdateBatch) Plans() []CommandPlan     { return clonePlans(b.plans) }

// PrepareUpdateBatch resolves a batch exactly once. The returned batch may be
// inspected for dry-run/preflight and then passed unchanged to ExecutePreparedBatch.
func PrepareUpdateBatch(ctx context.Context, cat model.Category, items []*model.Item) (*PreparedUpdateBatch, error) {
	selected := validItems(items)
	plans, err := planUpdateCommands(ctx, cat, selected)
	if err != nil {
		return nil, err
	}
	return &PreparedUpdateBatch{category: cat, items: append([]*model.Item(nil), selected...), plans: clonePlans(plans)}, nil
}

func clonePlans(plans []CommandPlan) []CommandPlan {
	cloned := make([]CommandPlan, len(plans))
	for i, plan := range plans {
		cloned[i] = plan
		cloned[i].Args = append([]string(nil), plan.Args...)
	}
	return cloned
}

// PlanUpdateCommands returns the commands necessary to update exactly the
// supplied items. A category-global prerequisite, such as apt metadata refresh,
// is represented explicitly and appears at most once. Empty input is a no-op.
//
// Categories without a safe automatic command return a Manual plan; callers
// can present that reason instead of silently broadening the selection.
func PlanUpdateCommands(cat model.Category, items []*model.Item) ([]CommandPlan, error) {
	return planUpdateCommands(context.Background(), cat, validItems(items))
}

func planUpdateCommands(ctx context.Context, cat model.Category, items []*model.Item) ([]CommandPlan, error) {
	items = validItems(items)
	if len(items) == 0 {
		return nil, nil
	}

	switch cat {
	case model.CatBrew:
		return exactPlans("brew", []string{commandUpgrade, "--greedy"}, items)
	case model.CatMAS:
		plans := make([]CommandPlan, 0, len(items))
		for _, item := range items {
			if item.PackageID == "" {
				plans = append(plans, manualPlan("MAS update requires the App Store ID"))
				continue
			}
			plans = append(plans, CommandPlan{Name: "mas", Args: []string{commandUpdate, item.PackageID}, Scope: CommandScopeExact})
		}
		return plans, nil
	case model.CatApt:
		names, err := planItemNames(items)
		if err != nil {
			return nil, err
		}
		return []CommandPlan{
			{Name: "apt-get", Args: []string{commandUpdate}, Scope: CommandScopeCategoryGlobal, Elevated: true},
			{Name: "apt-get", Args: append([]string{"install", "--only-upgrade", flagYes}, names...), Scope: CommandScopeExact, Elevated: true},
		}, nil
	case model.CatWinget:
		plans := make([]CommandPlan, 0, len(items))
		for _, item := range items {
			if item.PackageID == "" {
				return nil, fmt.Errorf("winget update for %q requires PackageID", item.Name)
			}
			plans = append(plans, CommandPlan{
				Name:  "winget",
				Args:  []string{commandUpgrade, "--exact", "--id", item.PackageID, "--accept-package-agreements", "--accept-source-agreements"},
				Scope: CommandScopeExact,
			})
		}
		return plans, nil
	case model.CatDnf:
		if tool := scanner.RpmToolName(); tool == "yum" {
			return globalPlan(tool, []string{commandUpdate, flagYes}, true)
		}
		return globalPlan(scanner.RpmToolName(), []string{commandUpgrade, "--refresh", flagYes}, true)
	case model.CatZypper:
		return globalPlan("zypper", []string{"--non-interactive", commandUpdate}, true)
	case model.CatApk:
		return globalPlan("apk", []string{commandUpgrade}, false)
	case model.CatPacman:
		if lookPath != nil {
			if _, err := lookPath("yay"); err == nil {
				return globalPlan("yay", []string{"-Syu", "--noconfirm"}, false)
			}
		}
		return globalPlan("pacman", []string{"-Syu", "--noconfirm"}, true)
	case model.CatFlatpak:
		return globalPlan("flatpak", []string{commandUpdate, flagYes}, false)
	case model.CatSnap:
		return globalPlan("snap", []string{"refresh"}, true)
	case model.CatChoco:
		names, err := packageIDsOrNames(items)
		if err != nil {
			return nil, err
		}
		return []CommandPlan{{Name: "choco", Args: append([]string{commandUpgrade}, append(names, flagYes)...), Scope: CommandScopeExact}}, nil
	case model.CatScoop:
		names, err := planItemNames(items)
		if err != nil {
			return nil, err
		}
		return []CommandPlan{{Name: "scoop", Args: append([]string{commandUpdate}, names...), Scope: CommandScopeExact}}, nil
	case model.CatNpm:
		if _, err := planItemNames(items); err != nil {
			return nil, err
		}
		updatable, _ := partitionNpmItems(items)
		if len(updatable) == 0 {
			return []CommandPlan{manualPlan(npmManagedElsewhereNote)}, nil
		}
		args := npmGlobalUpdateArgs(updatable)
		if len(args) == 0 {
			return nil, fmt.Errorf("npm update requires at least one package name")
		}
		if allow := npmAllowScriptsFlag(updatable); allow != "" {
			args = append(args, allow)
		}
		elevated, err := npmGlobalElevation(ctx)
		if err != nil {
			return nil, err
		}
		return []CommandPlan{{Name: npmCommand, Args: args, Scope: CommandScopeExact, Elevated: elevated}}, nil
	case model.CatPnpm:
		return exactPlans("pnpm", []string{commandUpdate, flagGlobal}, items)
	case model.CatBun:
		return exactPlans("bun", []string{commandUpdate, flagGlobal}, items)
	case model.CatPipx:
		return exactPlans("pipx", []string{commandUpgrade}, items)
	case model.CatOpenCodePlugins:
		return globalPlan(npmCommand, []string{commandUpdate, "--prefix", scanner.OpenCodeConfigDir()}, false)
	case model.CatGo:
		return globalPlan("gup", []string{commandUpdate}, false)
	case model.CatRustup:
		return globalPlan("rustup", []string{commandUpdate}, false)
	case model.CatCargo:
		return globalPlan("cargo", []string{"install-update", "-a"}, false)
	case model.CatSDKMAN:
		return bashGlobalPlan("source $HOME/.sdkman/bin/sdkman-init.sh && echo \"Y\" | sdk upgrade", "install bash or run 'sdk upgrade' manually")
	case model.CatNvm:
		if runtime.GOOS == "windows" {
			return []CommandPlan{manualPlan("update nvm-windows via its installer")}, nil
		}
		return bashGlobalPlan("source $HOME/.nvm/nvm.sh && nvm install-latest-npm", "install bash or update nvm manually")
	case model.CatOmz:
		return bashGlobalPlan("source $HOME/.oh-my-zsh/tools/upgrade.sh", "install bash or run the Oh My Zsh upgrade script manually")
	case model.CatGHExt:
		return globalPlan("gh", []string{"extension", commandUpgrade, "--all"}, false)
	case model.CatAgent:
		return agentPlans(items)
	case model.CatAI:
		return infraPlans(items)
	default:
		return []CommandPlan{manualPlan(fmt.Sprintf("no automatic updater for category %s", cat))}, nil
	}
}

func globalPlan(name string, args []string, elevated bool) ([]CommandPlan, error) {
	return []CommandPlan{{Name: name, Args: args, Scope: CommandScopeCategoryGlobal, Elevated: elevated}}, nil
}
func bashGlobalPlan(script, manual string) ([]CommandPlan, error) {
	if _, err := lookPath("bash"); err != nil {
		return []CommandPlan{manualPlan(manual)}, nil
	}
	return globalPlan("bash", []string{"-c", script}, false)
}
func manualPlan(reason string) CommandPlan {
	return CommandPlan{Scope: CommandScopeManual, Manual: reason}
}
func packageIDsOrNames(items []*model.Item) ([]string, error) {
	names := make([]string, 0, len(items))
	for _, item := range items {
		if item.PackageID != "" {
			names = append(names, item.PackageID)
		} else {
			if item.Name == "" {
				return nil, fmt.Errorf("cannot plan update for item without name or package ID")
			}
			names = append(names, item.Name)
		}
	}
	return names, nil
}
func agentPlans(items []*model.Item) ([]CommandPlan, error) {
	plans := make([]CommandPlan, 0, len(items))
	for _, item := range items {
		if cmd := scanner.AgentUpdateCommand(item.Name); len(cmd) > 0 {
			plans = append(plans, CommandPlan{Name: cmd[0], Args: cmd[1:], Scope: CommandScopeExact})
			continue
		}
		reason := item.KeepPolicy
		if reason == "" {
			reason = scanner.AgentKeepPolicy(item.Name)
		}
		if reason == "" {
			reason = "manual reinstall / app update"
		}
		plans = append(plans, manualPlan(reason))
	}
	return plans, nil
}
func infraPlans(items []*model.Item) ([]CommandPlan, error) {
	plans := make([]CommandPlan, 0, len(items))
	for _, item := range items {
		if cmd := scanner.InfraUpdateCommand(item.Name); len(cmd) > 0 {
			plans = append(plans, CommandPlan{Name: cmd[0], Args: cmd[1:], Scope: CommandScopeExact})
		} else {
			plans = append(plans, manualPlan("no auto-update"))
		}
	}
	return plans, nil
}

// PlansRequireWholeCategory tells a caller whether a partial selection must be
// rejected or explicitly expanded before execution.
func PlansRequireWholeCategory(plans []CommandPlan) bool {
	for _, plan := range plans {
		if plan.Scope == CommandScopeCategoryGlobal {
			return true
		}
	}
	return false
}

// PlansRequireElevation reports the preflight decision embedded in the plan.
// CLI/TUI should use this value for password/--skip-password handling rather
// than probing package-manager state again.
func PlansRequireElevation(plans []CommandPlan) bool {
	for _, plan := range plans {
		if plan.Elevated {
			return true
		}
	}
	return false
}

func validItems(items []*model.Item) []*model.Item {
	valid := make([]*model.Item, 0, len(items))
	for _, item := range items {
		if item != nil {
			valid = append(valid, item)
		}
	}
	return valid
}

func planItemNames(items []*model.Item) ([]string, error) {
	names := make([]string, 0, len(items))
	for _, item := range items {
		if item.Name == "" {
			return nil, fmt.Errorf("cannot plan update for item without name")
		}
		names = append(names, item.Name)
	}
	return names, nil
}

func exactPlans(name string, prefix []string, items []*model.Item) ([]CommandPlan, error) {
	names, err := planItemNames(items)
	if err != nil {
		return nil, err
	}
	plans := make([]CommandPlan, 0, len(names))
	for _, itemName := range names {
		args := append([]string{}, prefix...)
		args = append(args, itemName)
		plans = append(plans, CommandPlan{Name: name, Args: args, Scope: CommandScopeExact})
	}
	return plans, nil
}
