package scanner

import (
	"context"
	"strings"

	"github.com/lgldsilva/updash/internal/model"
)

// ChocoSource scans Windows packages via Chocolatey.
type ChocoSource struct{}

func (s *ChocoSource) Category() model.Category { return model.CatChoco }
func (s *ChocoSource) Label() string            { return "Chocolatey" }
func (s *ChocoSource) Icon() string             { return "🍫" }

func (s *ChocoSource) Scan(ctx context.Context, plat model.PlatformInfo) ([]*model.Item, error) {
	// choco outdated returns a list of outdated packages
	out, err := execCommand(ctx, "choco", "outdated", "--no-color", "--limit-output")
	if err != nil {
		return []*model.Item{
			{Name: "choco", Category: model.CatChoco, Status: model.StatusError, CurrentVer: "error"},
		}, nil
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	var items []*model.Item

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "Chocolatey") || strings.HasPrefix(line, "Outdated") {
			continue
		}
		// Format: "name|currentVer|availableVer|source"
		parts := strings.Split(line, "|")
		if len(parts) >= 3 {
			items = append(items, &model.Item{
				Name:         strings.TrimSpace(parts[0]),
				Category:     model.CatChoco,
				CurrentVer:   strings.TrimSpace(parts[1]),
				AvailableVer: strings.TrimSpace(parts[2]),
				Status:       model.StatusOutdated,
			})
		}
	}

	if len(items) == 0 {
		items = append(items, &model.Item{
			Name: "choco", Category: model.CatChoco, Status: model.StatusOK, CurrentVer: "up to date",
		})
	}

	return items, nil
}
