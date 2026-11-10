package updater

import (
	"context"
	"fmt"
	"runtime"

	"github.com/lgldsilva/updash/internal/model"
)

// categoryPlanner plans the update commands for one category. items is
// already validated and non-empty.
type categoryPlanner func(ctx context.Context, items []*model.Item) ([]CommandPlan, error)

// categoryPlanners is the single dispatch table for update planning; a
// category missing here gets a Manual plan. Planners whose command depends on
// the host (euid, PATH) evaluate that at call time, never at init.
var categoryPlanners = map[model.Category]categoryPlanner{
	model.CatBrew:            exactPlanner("brew", commandUpgrade, "--greedy"),
	model.CatMAS:             planMAS,
	model.CatApt:             planApt,
	model.CatWinget:          planWinget,
	model.CatDnf:             planDnf,
	model.CatZypper:          globalPlanner("zypper", true, "--non-interactive", commandUpdate),
	model.CatApk:             planApk,
	model.CatPacman:          planPacman,
	model.CatFlatpak:         globalPlanner("flatpak", false, commandUpdate, flagYes),
	model.CatSnap:            globalPlanner("snap", true, "refresh"),
	model.CatChoco:           planChoco,
	model.CatScoop:           planScoop,
	model.CatNpm:             planNpm,
	model.CatPnpm:            exactPlanner("pnpm", commandUpdate, flagGlobal),
	model.CatBun:             exactPlanner("bun", commandUpdate, flagGlobal),
	model.CatPipx:            globalPlanner("pipx", false, "upgrade-all"),
	model.CatOpenCodePlugins: itemsOnly(opencodePluginPlans),
	model.CatGo:              globalPlanner("gup", false, commandUpdate),
	model.CatRustup:          itemsOnly(rustupPlans),
	model.CatCargo:           globalPlanner("cargo", false, "install-update", "-a"),
	model.CatSDKMAN:          planSDKMAN,
	model.CatNvm:             planNvm,
	model.CatOmz:             func(context.Context, []*model.Item) ([]CommandPlan, error) { return omzGlobalPlan() },
	model.CatGHExt:           globalPlanner("gh", false, "extension", commandUpgrade, "--all"),
	model.CatAgent:           itemsOnly(agentPlans),
	model.CatAI:              itemsOnly(infraPlans),
}

func exactPlanner(name string, args ...string) categoryPlanner {
	return func(_ context.Context, items []*model.Item) ([]CommandPlan, error) {
		return exactPlans(name, args, items)
	}
}

func globalPlanner(name string, elevated bool, args ...string) categoryPlanner {
	return func(context.Context, []*model.Item) ([]CommandPlan, error) {
		return globalPlan(name, args, elevated)
	}
}

func itemsOnly(plan func([]*model.Item) ([]CommandPlan, error)) categoryPlanner {
	return func(_ context.Context, items []*model.Item) ([]CommandPlan, error) {
		return plan(items)
	}
}

func planMAS(_ context.Context, items []*model.Item) ([]CommandPlan, error) {
	plans := make([]CommandPlan, 0, len(items))
	for _, item := range items {
		if item.PackageID == "" {
			plans = append(plans, manualPlan("MAS update requires the App Store ID"))
			continue
		}
		plans = append(plans, CommandPlan{Name: "mas", Args: []string{commandUpdate, item.PackageID}, Scope: CommandScopeExact})
	}
	return plans, nil
}

func planApt(_ context.Context, items []*model.Item) ([]CommandPlan, error) {
	names, err := planItemNames(items)
	if err != nil {
		return nil, err
	}
	return []CommandPlan{
		{Name: "apt-get", Args: []string{commandUpdate}, Scope: CommandScopeCategoryGlobal, Elevated: true},
		{Name: "apt-get", Args: append([]string{"install", "--only-upgrade", flagYes}, names...), Scope: CommandScopeExact, Elevated: true},
	}, nil
}

func planWinget(_ context.Context, items []*model.Item) ([]CommandPlan, error) {
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
}

func planDnf(context.Context, []*model.Item) ([]CommandPlan, error) {
	if tool := rpmToolName(); tool == "yum" {
		return globalPlan(tool, []string{commandUpdate, flagYes}, true)
	}
	return globalPlan(rpmToolName(), []string{commandUpgrade, "--refresh", flagYes}, true)
}

// planApk: apk locks a root-owned database; planUpdateCommands drops sudo
// when already root.
func planApk(context.Context, []*model.Item) ([]CommandPlan, error) {
	return globalPlan("apk", []string{commandUpgrade}, true)
}

func planPacman(context.Context, []*model.Item) ([]CommandPlan, error) {
	if lookPath != nil {
		if _, err := lookPath("yay"); err == nil {
			return globalPlan("yay", []string{"-Syu", "--noconfirm"}, false)
		}
	}
	return globalPlan("pacman", []string{"-Syu", "--noconfirm"}, true)
}

func planChoco(_ context.Context, items []*model.Item) ([]CommandPlan, error) {
	names, err := packageIDsOrNames(items)
	if err != nil {
		return nil, err
	}
	return []CommandPlan{{Name: "choco", Args: append([]string{commandUpgrade}, append(names, flagYes)...), Scope: CommandScopeExact}}, nil
}

func planScoop(_ context.Context, items []*model.Item) ([]CommandPlan, error) {
	names, err := planItemNames(items)
	if err != nil {
		return nil, err
	}
	return []CommandPlan{{Name: "scoop", Args: append([]string{commandUpdate}, names...), Scope: CommandScopeExact}}, nil
}

func planNpm(ctx context.Context, items []*model.Item) ([]CommandPlan, error) {
	if _, err := planItemNames(items); err != nil {
		return nil, err
	}
	updatable, _ := partitionNpmItems(items)
	if len(updatable) == 0 {
		return []CommandPlan{manualPlan(npmManagedElsewhereNote)}, nil
	}
	return npmPrefixGroupPlans(ctx, updatable)
}

func planSDKMAN(context.Context, []*model.Item) ([]CommandPlan, error) {
	return bashGlobalPlan("source $HOME/.sdkman/bin/sdkman-init.sh && echo \"Y\" | sdk upgrade", "install bash or run 'sdk upgrade' manually")
}

func planNvm(context.Context, []*model.Item) ([]CommandPlan, error) {
	if runtime.GOOS == "windows" {
		return []CommandPlan{manualPlan("update nvm-windows via its installer")}, nil
	}
	return bashGlobalPlan("source $HOME/.nvm/nvm.sh && nvm install-latest-npm", "install bash or update nvm manually")
}
