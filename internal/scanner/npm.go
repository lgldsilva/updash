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
	out, err := execCommand(ctx, binNpm, "outdated", flagGlobal, "--json")
	if err != nil {
		// npm outdated returns exit code 1 when there are outdated packages
		// but still outputs valid JSON on stdout
		if len(out) == 0 {
			msg := errStderr(err)
			if len(msg) > 120 {
				msg = msg[:120] + "…"
			}
			return []*model.Item{
				{Name: binNpm, Category: model.CatNpm, Status: model.StatusError, CurrentVer: msg},
			}, nil
		}
	}

	items, parseErr := parseNpmOutdatedJSON(out, model.CatNpm)
	if parseErr != nil {
		return []*model.Item{errItem(binNpm, model.CatNpm)}, nil
	}
	// Drop packages owned by another update path (e.g. opencode-ai is owned by
	// `opencode upgrade`); they must not appear as a second npm update target.
	items = filterProtectedNpm(items)
	return okOrOutdated(binNpm, model.CatNpm, items), nil
}
