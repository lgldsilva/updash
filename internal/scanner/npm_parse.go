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
	var data map[string]npmOutdatedEntry
	if err := json.Unmarshal(out, &data); err != nil || len(data) == 0 {
		return nil
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
	return items
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
