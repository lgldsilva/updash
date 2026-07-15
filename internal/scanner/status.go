package scanner

import "github.com/lgldsilva/updash/internal/model"

// Shared status strings (Sonar S1192 — avoid duplicating literals).
const (
	statusUpToDate = "up to date"
	statusError    = "error"
)

func okItem(name string, cat model.Category) *model.Item {
	return &model.Item{
		Name:       name,
		Category:   cat,
		Status:     model.StatusOK,
		CurrentVer: statusUpToDate,
	}
}

func errItem(name string, cat model.Category) *model.Item {
	return &model.Item{
		Name:       name,
		Category:   cat,
		Status:     model.StatusError,
		CurrentVer: statusError,
	}
}

func okOrOutdated(name string, cat model.Category, items []*model.Item) []*model.Item {
	if len(items) == 0 {
		return []*model.Item{okItem(name, cat)}
	}
	return items
}
