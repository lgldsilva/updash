package scanner

import (
	"context"
	"os"
	"path/filepath"

	"github.com/lgldsilva/updash/internal/model"
)

// OpenCodeSource scans local npm plugins under ~/.config/opencode.
type OpenCodeSource struct{}

const nameOpenCodePlugins = "opencode-plugins"

func (s *OpenCodeSource) Category() model.Category { return model.CatOpenCodePlugins }
func (s *OpenCodeSource) Label() string            { return "OpenCode Plugins" }
func (s *OpenCodeSource) Icon() string             { return "🔌" }

// OpenCodeConfigDir returns the OpenCode config directory (overridable in tests).
var OpenCodeConfigDir = func() string {
	return filepath.Join(os.Getenv("HOME"), ".config", "opencode")
}

func (s *OpenCodeSource) Scan(ctx context.Context, plat model.PlatformInfo) ([]*model.Item, error) {
	dir := OpenCodeConfigDir()
	pkgJSON := filepath.Join(dir, "package.json")
	if _, err := os.Stat(pkgJSON); err != nil {
		return []*model.Item{
			{Name: nameOpenCodePlugins, Category: model.CatOpenCodePlugins, Status: model.StatusOK, CurrentVer: "no package.json"},
		}, nil
	}

	out, err := execCombined(ctx, "npm", "outdated", "--prefix", dir, "--json")
	if err != nil && len(out) == 0 {
		return []*model.Item{
			{Name: nameOpenCodePlugins, Category: model.CatOpenCodePlugins, Status: model.StatusError, CurrentVer: "npm outdated failed"},
		}, nil
	}

	items := ParseNpmOutdatedJSON(out, model.CatOpenCodePlugins)
	return okOrOutdated(nameOpenCodePlugins, model.CatOpenCodePlugins, items), nil
}
