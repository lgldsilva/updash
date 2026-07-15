package scanner

import (
	"context"

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

	items := ParseNpmOutdatedJSON(out, model.CatNpm)
	return okOrOutdated("npm", model.CatNpm, items), nil
}
