package scanner

import (
	"context"
	"encoding/json"

	"github.com/lgldsilva/updash/internal/model"
)

// NpmSource scans npm global packages.
type NpmSource struct{}

func (s *NpmSource) Category() model.Category { return model.CatNpm }
func (s *NpmSource) Label() string            { return "npm (global)" }
func (s *NpmSource) Icon() string             { return "⬡" }

func (s *NpmSource) Scan(ctx context.Context, plat model.PlatformInfo) ([]*model.Item, error) {
	out, err := execCombined(ctx, "npm", "outdated", "-g", "--json")
	if err != nil {
		// npm outdated returns exit code 1 when there are outdated packages
		// but still outputs valid JSON
		if len(out) == 0 {
			msg := err.Error()
			if len(msg) > 120 {
				msg = msg[:120] + "…"
			}
			return []*model.Item{
				{Name: "npm", Category: model.CatNpm, Status: model.StatusError, CurrentVer: msg},
			}, nil
		}
	}

	var data map[string]struct {
		Current string `json:"current"`
		Wanted  string `json:"wanted"`
		Latest  string `json:"latest"`
	}
	if err := json.Unmarshal(out, &data); err != nil || len(data) == 0 {
		return []*model.Item{
			{Name: "npm", Category: model.CatNpm, Status: model.StatusOK, CurrentVer: statusUpToDate},
		}, nil
	}

	var items []*model.Item
	for name, pkg := range data {
		items = append(items, &model.Item{
			Name:         name,
			Category:     model.CatNpm,
			CurrentVer:   pkg.Current,
			AvailableVer: pkg.Latest,
			Status:       model.StatusOutdated,
		})
	}

	return items, nil
}
