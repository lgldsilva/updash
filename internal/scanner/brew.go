package scanner

import (
	"context"
	"encoding/json"

	"github.com/lgldsilva/updash/internal/model"
)

// BrewSource scans Homebrew formulas and casks.
type BrewSource struct{}

func (s *BrewSource) Category() model.Category { return model.CatBrew }
func (s *BrewSource) Label() string            { return "Homebrew" }
func (s *BrewSource) Icon() string             { return "🍺" }

// externalCasks lists casks that are managed by other tools (Toolbox, MAS, etc.)
// or require sudo TTY — excluded from brew scanner to avoid false failures.
var externalCasks = map[string]bool{
	// JetBrains Toolbox
	"clion": true, "datagrip": true, "goland": true,
	"intellij-idea-ce": true, "phpstorm": true,
	"pycharm": true, "pycharm-ce": true,
	"fleet": true, "rubymine": true, "webstorm": true,
	// MAS apps that also have brew casks (prefer MAS)
	"whatsapp": true,
	// Microsoft apps needing sudo TTY
	"microsoft-office":      true,
	"microsoft-auto-update": true,
}

// isManagedExternally returns true if the cask is managed by another tool.
func isManagedExternally(name string) bool {
	return externalCasks[name]
}

// brewOutdatedJSON maps the --json=v2 output.
type brewOutdatedJSON struct {
	Formulae []brewPkg `json:"formulae"`
	Casks    []brewPkg `json:"casks"`
}

type brewPkg struct {
	Name              string   `json:"name"`
	InstalledVersions []string `json:"installed_versions"`
	CurrentVersion    string   `json:"current_version"`
	Pinned            bool     `json:"pinned"`
}

func (s *BrewSource) Scan(ctx context.Context, plat model.PlatformInfo) ([]*model.Item, error) {
	out, err := execCommand(ctx, "brew", "outdated", "--greedy", "--json=v2")
	if err != nil {
		return []*model.Item{
			{Name: "brew", Category: model.CatBrew, Status: model.StatusError, CurrentVer: "error checking"},
		}, nil
	}

	var data brewOutdatedJSON
	if err := json.Unmarshal(out, &data); err != nil {
		return []*model.Item{
			{Name: "brew", Category: model.CatBrew, Status: model.StatusError, CurrentVer: "parse error"},
		}, nil
	}

	total := len(data.Formulae) + len(data.Casks)
	if total == 0 {
		return []*model.Item{
			{Name: "brew", Category: model.CatBrew, Status: model.StatusOK, CurrentVer: statusUpToDate},
		}, nil
	}

	var items []*model.Item
	for _, p := range data.Formulae {
		cur := ""
		if len(p.InstalledVersions) > 0 {
			cur = p.InstalledVersions[0]
		}
		items = append(items, &model.Item{
			Name:         p.Name,
			Category:     model.CatBrew,
			CurrentVer:   cur,
			AvailableVer: p.CurrentVersion,
			Status:       model.StatusOutdated,
		})
	}
	for _, p := range data.Casks {
		cur := ""
		if len(p.InstalledVersions) > 0 {
			cur = p.InstalledVersions[0]
		}
		it := &model.Item{
			Name:         p.Name,
			Category:     model.CatBrew,
			CurrentVer:   cur,
			AvailableVer: p.CurrentVersion,
			Status:       model.StatusOutdated,
		}
		if note := BrewUpgradeNote(p.Name); note != "" {
			it.KeepPolicy = note
		}
		items = append(items, it)
	}

	return items, nil
}

// BrewOutdatedSet returns outdated brew formula/cask names (same source as BrewSource scan).
func BrewOutdatedSet(ctx context.Context) (map[string]struct{}, error) {
	out, err := execCommand(ctx, "brew", "outdated", "--greedy", "--json=v2")
	if err != nil {
		return nil, err
	}
	var data brewOutdatedJSON
	if err := json.Unmarshal(out, &data); err != nil {
		return nil, err
	}
	set := make(map[string]struct{}, len(data.Formulae)+len(data.Casks))
	for _, p := range data.Formulae {
		set[p.Name] = struct{}{}
	}
	for _, p := range data.Casks {
		set[p.Name] = struct{}{}
	}
	return set, nil
}
