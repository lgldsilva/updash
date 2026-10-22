package updater

import "github.com/lgldsilva/updash/internal/model"

// Name of the rustup manager row from `rustup check` (first field of the line).
const rustupManagerName = "rustup"

// rustupPlans builds ordered update commands for the selected rustup items.
// The manager row (Name == "rustup") needs `rustup self update`; toolchains
// use a single category-global `rustup update`. When both are selected,
// self-update runs first so the manager is refreshed before toolchains.
func rustupPlans(items []*model.Item) ([]CommandPlan, error) {
	var hasManager, hasToolchain bool
	for _, it := range items {
		if it == nil {
			continue
		}
		if it.Name == rustupManagerName {
			hasManager = true
			continue
		}
		hasToolchain = true
	}
	var plans []CommandPlan
	if hasManager {
		self, err := globalPlan(rustupManagerName, []string{"self", commandUpdate}, false)
		if err != nil {
			return nil, err
		}
		plans = append(plans, self...)
	}
	if hasToolchain {
		toolchains, err := globalPlan(rustupManagerName, []string{commandUpdate}, false)
		if err != nil {
			return nil, err
		}
		plans = append(plans, toolchains...)
	}
	return plans, nil
}
