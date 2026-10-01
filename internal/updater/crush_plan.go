package updater

import (
	"github.com/lgldsilva/updash/internal/scanner"
)

// agentCrush mirrors the scanner catalog entry name.
const agentCrush = "Crush"

// crushUpgradePlan updates Crush inside the npm prefix that owns the PATH
// binary (frequently a legacy ~/.npm-global). Installing without that prefix
// would drop a second copy into the ambient one — invisible while the legacy
// bin dir keeps winning PATH resolution. Crush layouts that are not
// npm-managed (brew/go builds) have no safe npm target: plan an honest manual
// reinstall instead.
func crushUpgradePlan() CommandPlan {
	path, err := lookPath("crush")
	if err != nil {
		return manualPlan("crush binary not found on PATH")
	}
	if resolved, rerr := evalSymlinks(path); rerr == nil {
		path = resolved
	}
	prefix := scanner.NpmPrefixForPath(path)
	if prefix == "" {
		return manualPlan("crush is not npm-managed; reinstall it with its official installer")
	}
	pkg := "@charmland/crush"
	args := []string{commandInstall, flagGlobal, flagPrefix, prefix, "--allow-scripts=" + pkg, pkg + "@latest"}
	return CommandPlan{Name: npmCommand, Args: args, Scope: CommandScopeExact, Elevated: npmPrefixNeedsSudo(prefix)}
}
