package elevate

import (
	"strings"

	"github.com/lgldsilva/updash/internal/model"
)

// CategoryNeedsElevation reports whether updating items in this category may
// require privileged access on the current platform.
func CategoryNeedsElevation(cat model.Category, plat model.PlatformInfo) bool {
	if plat.OS == "windows" {
		// User-scope package managers; admin elevation is out of scope for now.
		return false
	}
	switch cat {
	case model.CatMAS, model.CatApt, model.CatSnap:
		return true
	case model.CatPacman:
		// yay runs without sudo; plain pacman needs it.
		return !plat.HasYay
	default:
		return false
	}
}

// ItemNeedsElevation reports whether cleaning this item may require sudo.
func ItemNeedsElevation(item *model.Item) bool {
	if item.Category != model.CatCache {
		return false
	}
	name := strings.ToLower(item.Name)
	return strings.HasPrefix(name, "apt") || strings.HasPrefix(name, "snap")
}

// ItemsNeedElevation returns true if any item in the batch needs elevation.
func ItemsNeedElevation(items []*model.Item, plat model.PlatformInfo, cleanup bool) bool {
	for _, it := range items {
		if cleanup {
			if ItemNeedsElevation(it) {
				return true
			}
		} else if CategoryNeedsElevation(it.Category, plat) {
			return true
		}
	}
	return false
}
