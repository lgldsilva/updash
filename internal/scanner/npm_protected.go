package scanner

import "github.com/lgldsilva/updash/internal/model"

// ProtectedNpmPackages are global npm packages owned by another updash update
// path, not by the generic `npm update -g` batch. The OpenCode binary is owned
// by `opencode upgrade` (its agent updateCmd wins over this npmPackage), so its
// npm distribution packages must never be touched here — updating them without
// the agent path's validation is exactly how a broken launcher stub gets left
// behind.
//
// This is the single, data-driven source of truth (not grep): add a package
// name here when a new agent owns its own npm upgrade.
func ProtectedNpmPackages() map[string]struct{} {
	return map[string]struct{}{
		"opencode-ai":      {},
		"@opencode-ai/cli": {},
	}
}

// IsProtectedNpmPackage reports whether name is owned by another update path.
func IsProtectedNpmPackage(name string) bool {
	_, ok := ProtectedNpmPackages()[name]
	return ok
}

// filterProtectedNpm drops items whose package name is owned by another update
// path so the npm category shows a single owner (the agent row).
func filterProtectedNpm(items []*model.Item) []*model.Item {
	if len(items) == 0 {
		return items
	}
	filtered := items[:0]
	for _, it := range items {
		if it != nil && IsProtectedNpmPackage(it.Name) {
			continue
		}
		filtered = append(filtered, it)
	}
	return filtered
}
