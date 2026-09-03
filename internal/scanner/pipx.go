package scanner

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/lgldsilva/updash/internal/model"
)

// PipxSource scans pipx-installed packages.
type PipxSource struct{}

func (s *PipxSource) Category() model.Category { return model.CatPipx }
func (s *PipxSource) Label() string            { return binPipx }
func (s *PipxSource) Icon() string             { return "🐍" }

func (s *PipxSource) Scan(ctx context.Context, plat model.PlatformInfo) ([]*model.Item, error) {
	// pipx list --json gives installed packages with versions
	out, err := execCommand(ctx, binPipx, cmdList, "--json")
	if err != nil {
		return []*model.Item{
			{Name: binPipx, Category: model.CatPipx, Status: model.StatusError, CurrentVer: "error"},
		}, nil
	}

	output := string(out)
	if !strings.Contains(output, "venvs") {
		return []*model.Item{{Name: binPipx, Category: model.CatPipx, Status: model.StatusError, CurrentVer: "invalid pipx JSON"}}, nil
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
		return []*model.Item{{Name: binPipx, Category: model.CatPipx, Status: model.StatusError, CurrentVer: "invalid pipx JSON"}}, nil
	}

	var items []*model.Item
	for name, venv := range data.Venvs {
		items = append(items, &model.Item{
			Name:       name,
			Category:   model.CatPipx,
			CurrentVer: venv.Metadata.ItemVersion,
			Status:     model.StatusInfo,
		})
	}

	// `pipx list` only reports installed versions, so probe PyPI per package
	// through the venv's own pip (`pipx runpip <pkg> list --outdated`). A probe
	// that fails leaves the item informational — an affirmative claim it never
	// verified would be worse than none.
	for _, it := range items {
		out, err := execCommandBudget(ctx, pipxProbeTimeout, binPipx, "runpip", it.Name, "list", "--outdated", "--format=json")
		if err != nil {
			continue
		}
		latest := parsePipxOutdated(string(out), it.Name)
		if latest == "" {
			it.Status = model.StatusOK
			continue
		}
		it.AvailableVer = latest
		it.Status = model.StatusOutdated
	}

	if len(items) == 0 {
		items = append(items, &model.Item{Name: binPipx, Category: model.CatPipx, Status: model.StatusInfo, CurrentVer: statusUpToDate})
	}

	return items, nil
}

const pipxProbeTimeout = 20 * time.Second

// parsePipxOutdated extracts latest_version for the named package from
// `pip list --outdated --format=json` output. A package absent from the list
// is up to date (or pip could not reach the index); both mean "no item".
func parsePipxOutdated(output, name string) string {
	var rows []struct {
		Name      string `json:"name"`
		LatestVer string `json:"latest_version"`
	}
	if err := json.Unmarshal([]byte(output), &rows); err != nil {
		return ""
	}
	for _, r := range rows {
		if strings.EqualFold(r.Name, name) {
			return r.LatestVer
		}
	}
	return ""
}
