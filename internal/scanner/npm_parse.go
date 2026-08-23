package scanner

import (
	"encoding/json"

	"github.com/lgldsilva/updash/internal/model"
)

// npmOutdatedEntry is one package from `npm outdated --json`.
type npmOutdatedEntry struct {
	Current string `json:"current"`
	Wanted  string `json:"wanted"`
	Latest  string `json:"latest"`
}

// ParseNpmOutdatedJSON converts `npm outdated --json` output into Item list.
// Empty or "{}" yields no items (caller adds an OK placeholder when needed).
func ParseNpmOutdatedJSON(out []byte, cat model.Category) []*model.Item {
	items, _ := parseNpmOutdatedJSON(out, cat)
	return items
}

func parseNpmOutdatedJSON(out []byte, cat model.Category) ([]*model.Item, error) {
	var data map[string]npmOutdatedEntry
	if err := json.Unmarshal(out, &data); err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, nil
	}
	items := make([]*model.Item, 0, len(data))
	for name, pkg := range data {
		avail := pkg.Latest
		if avail == "" {
			avail = pkg.Wanted
		}
		items = append(items, &model.Item{
			Name:         name,
			Category:     cat,
			CurrentVer:   pkg.Current,
			AvailableVer: avail,
			Status:       model.StatusOutdated,
		})
	}
	return items, nil
}

// npmLsGlobal is the shape of `npm ls -g --json --depth=0`.
type npmLsGlobal struct {
	Dependencies map[string]struct {
		Version string `json:"version"`
	} `json:"dependencies"`
}

// ParseNpmLsGlobal returns the set of globally installed npm package names.
func ParseNpmLsGlobal(out []byte) map[string]bool {
	var data npmLsGlobal
	if err := json.Unmarshal(out, &data); err != nil || len(data.Dependencies) == 0 {
		return nil
	}
	m := make(map[string]bool, len(data.Dependencies))
	for name := range data.Dependencies {
		m[name] = true
	}
	return m
}

// ParseNpmOutdatedMap returns package name → latest version from npm outdated JSON.
func ParseNpmOutdatedMap(out []byte) map[string]string {
	var data map[string]npmOutdatedEntry
	if err := json.Unmarshal(out, &data); err != nil || len(data) == 0 {
		return nil
	}
	m := make(map[string]string, len(data))
	for name, pkg := range data {
		avail := pkg.Latest
		if avail == "" {
			avail = pkg.Wanted
		}
		if avail != "" {
			m[name] = avail
		}
	}
	return m
}
