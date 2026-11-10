package updater

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/lgldsilva/updash/internal/model"
	"github.com/lgldsilva/updash/internal/scanner"
)

const defaultPlanTimeout = 30 * time.Second

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

// Subset returns a prepared batch for the requested items without invoking
// the planner again. It is used when execution partitions one preflight batch
// (for example, Brew items that do and do not need native elevation).
func (b *PreparedUpdateBatch) Subset(items []*model.Item) (*PreparedUpdateBatch, error) {
	if b == nil {
		return nil, fmt.Errorf("cannot subset a nil prepared update batch")
	}
	selected := validItems(items)
	if len(selected) == 0 {
		return &PreparedUpdateBatch{category: b.category}, nil
	}
	if len(b.items) != len(b.plans) {
		return nil, fmt.Errorf("prepared update batch has mismatched items and plans")
	}

	positions := make(map[*model.Item]int, len(b.items))
	for i, item := range b.items {
		positions[item] = i
	}
	selectedPlans := make([]CommandPlan, 0, len(selected))
	for _, item := range selected {
		position, ok := positions[item]
		if !ok {
			return nil, fmt.Errorf("item %q is not part of the prepared update batch", item.Name)
		}
		selectedPlans = append(selectedPlans, b.plans[position])
	}
	return &PreparedUpdateBatch{
		category: b.category,
		items:    append([]*model.Item(nil), selected...),
		plans:    clonePlans(selectedPlans),
	}, nil
}

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
	ctx, cancel := context.WithTimeout(context.Background(), defaultPlanTimeout)
	defer cancel()
	return planUpdateCommands(ctx, cat, validItems(items))
}

func planUpdateCommands(ctx context.Context, cat model.Category, items []*model.Item) ([]CommandPlan, error) {
	plans, err := planCategory(ctx, cat, items)
	if err == nil && geteuid() == 0 {
		// Root needs no sudo, and root containers (Alpine, Debian, Fedora)
		// often do not ship it: "sudo apt-get" would fail with "not found".
		for i := range plans {
			plans[i].Elevated = false
		}
	}
	return plans, err
}

func planCategory(ctx context.Context, cat model.Category, items []*model.Item) ([]CommandPlan, error) {
	items = validItems(items)
	if len(items) == 0 {
		return nil, nil
	}

	if plan, ok := categoryPlanners[cat]; ok {
		return plan(ctx, items)
	}
	return []CommandPlan{manualPlan(fmt.Sprintf("no automatic updater for category %s", cat))}, nil
}

func globalPlan(name string, args []string, elevated bool) ([]CommandPlan, error) {
	return []CommandPlan{{Name: name, Args: args, Scope: CommandScopeCategoryGlobal, Elevated: elevated}}, nil
}

// opencodePluginPlans builds the OpenCode plugin update. `npm update` only
// moves packages within their package.json range, so a plugin pinned exactly
// (wanted == current) never upgrades and stays flagged forever. Install the
// flagged versions explicitly instead; fall back to `npm update` when an item
// has no known target version.
func opencodePluginPlans(items []*model.Item) ([]CommandPlan, error) {
	dir := scanner.OpenCodeConfigDir()
	var pinned []string
	unversioned := 0
	for _, it := range items {
		if it == nil {
			continue
		}
		if it.AvailableVer == "" {
			unversioned++
			continue
		}
		pinned = append(pinned, it.Name+"@"+it.AvailableVer)
	}
	if len(pinned) == 0 {
		return globalPlan(npmCommand, []string{commandUpdate, flagPrefix, dir}, false)
	}
	args := append([]string{commandInstall, flagPrefix, dir}, pinned...)
	plans := []CommandPlan{{Name: npmCommand, Args: args, Scope: CommandScopeExact}}
	if unversioned == 0 {
		return plans, nil
	}
	fallback, err := globalPlan(npmCommand, []string{commandUpdate, flagPrefix, dir}, false)
	if err != nil {
		return nil, err
	}
	return append(plans, fallback...), nil
}
func bashGlobalPlan(script, manual string) ([]CommandPlan, error) {
	if _, err := lookPath("bash"); err != nil {
		return []CommandPlan{manualPlan(manual)}, nil
	}
	return globalPlan("bash", []string{"-c", script}, false)
}

func omzGlobalPlan() ([]CommandPlan, error) {
	if _, err := lookPath("zsh"); err != nil {
		return []CommandPlan{manualPlan("install zsh or run the Oh My Zsh upgrade script manually")}, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolve home directory for Oh My Zsh: %w", err)
	}
	scriptPath, err := filepath.Abs(filepath.Join(home, ".oh-my-zsh", "tools", "upgrade.sh"))
	if err != nil {
		return nil, fmt.Errorf("resolve Oh My Zsh updater path: %w", err)
	}
	// -f disables zsh startup files. Passing the script as argv also avoids a
	// shell fragment that could source the caller's .zshrc or attach tmux.
	return globalPlan("zsh", []string{"-f", scriptPath}, false)
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
		// OpenCode resolves its own install method and prompts when it fails;
		// updash decides the method (or the npm fallback) up front instead.
		if item.Name == agentOpenCode {
			plans = append(plans, openCodeUpgradePlan(scanner.AgentUpdateCommand(item.Name), item))
			continue
		}
		// Crush lives in whatever npm prefix owns the PATH binary; updating it
		// there avoids installing a shadowing second copy into the ambient one.
		if item.Name == agentCrush {
			plans = append(plans, crushUpgradePlan())
			continue
		}
		cmd := scanner.AgentUpdateCommand(item.Name)
		if len(cmd) > 0 {
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

// PlansHaveUnelevatedWork reports whether declining elevation can still leave
// a real command to run. Manual plans are not work. npm uses this so a
// user-owned prefix is updated when a /usr prefix in the same batch needs sudo.
func PlansHaveUnelevatedWork(plans []CommandPlan) bool {
	for _, plan := range plans {
		if plan.Scope != CommandScopeManual && !plan.Elevated {
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
