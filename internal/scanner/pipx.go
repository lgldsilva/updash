package scanner

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/lgldsilva/updash/internal/model"
)

// PipxSource scans pipx-installed packages.
type PipxSource struct{}

func (s *PipxSource) Category() model.Category { return model.CatPipx }
func (s *PipxSource) Label() string            { return "pipx" }
func (s *PipxSource) Icon() string             { return "🐍" }

func (s *PipxSource) Scan(ctx context.Context, plat model.PlatformInfo) ([]*model.Item, error) {
	// pipx list --json gives installed packages with versions
	out, err := execCommand(ctx, "pipx", "list", "--json")
	if err != nil {
		return []*model.Item{
			{Name: "pipx", Category: model.CatPipx, Status: model.StatusError, CurrentVer: "error"},
		}, nil
	}

	output := string(out)
	if !strings.Contains(output, "venvs") {
		return []*model.Item{
			{Name: "pipx", Category: model.CatPipx, Status: model.StatusOK, CurrentVer: "up to date"},
		}, nil
	}

	// Parse JSON manually since structure is nested
	var data struct {
		Venvs map[string]struct {
			Metadata struct {
				ItemVersion string `json:"item_version"`
			} `json:"metadata"`
		} `json:"venvs"`
	}
	if err := json.Unmarshal([]byte(output), &data); err != nil {
		return []*model.Item{
			{Name: "pipx", Category: model.CatPipx, Status: model.StatusOK, CurrentVer: "up to date"},
		}, nil
	}

	var items []*model.Item
	for name, venv := range data.Venvs {
		items = append(items, &model.Item{
			Name:       name,
			Category:   model.CatPipx,
			CurrentVer: venv.Metadata.ItemVersion,
			Status:     model.StatusOK, // We'll mark all as OK; upgrade-all is simple
		})
	}

	if len(items) == 0 {
		items = append(items, &model.Item{
			Name: "pipx", Category: model.CatPipx, Status: model.StatusOK, CurrentVer: "up to date",
		})
	}

	return items, nil
}
