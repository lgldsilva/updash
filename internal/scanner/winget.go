package scanner

import (
	"context"
	"encoding/json"
	"os/exec"
	"strings"

	"github.com/lgldsilva/updash/internal/model"
)

// WingetSource scans Windows packages via winget.
type WingetSource struct{}

func (s *WingetSource) Category() model.Category { return model.CatWinget }
func (s *WingetSource) Label() string            { return "winget" }
func (s *WingetSource) Icon() string             { return "🪟" }

// wingetUpgradeJSON maps the `winget upgrade --json` output.
type wingetUpgradeJSON struct {
	Upgrades []wingetPkg `json:"upgrades"`
}

type wingetPkg struct {
	Name    string `json:"name"`
	ID      string `json:"id"`
	Version string `json:"version"`
	NewVer  string `json:"newVersion"`
}

func (s *WingetSource) Scan(ctx context.Context, plat model.PlatformInfo) ([]*model.Item, error) {
	cmd := exec.CommandContext(ctx, "winget", "upgrade", "--json")
	out, err := cmd.Output()
	if err != nil {
		// winget may exit non-zero when there are upgrades
		if len(out) == 0 {
			return []*model.Item{
				{Name: "winget", Category: model.CatWinget, Status: model.StatusError, CurrentVer: "error"},
			}, nil
		}
	}

	// Parse JSON — winget's --json may wrap in multiple lines
	output := strings.TrimSpace(string(out))

	// Find the JSON object in the output (sometimes there's preamble text)
	braceIdx := strings.Index(output, "{")
	if braceIdx >= 0 {
		output = output[braceIdx:]
	}

	var data wingetUpgradeJSON
	if err := json.Unmarshal([]byte(output), &data); err != nil || len(data.Upgrades) == 0 {
		// Also try direct upgrade list
		return scanWingetText(ctx)
	}

	var items []*model.Item
	for _, p := range data.Upgrades {
		name := p.Name
		if name == "" {
			name = p.ID
		}
		items = append(items, &model.Item{
			Name:         name,
			Category:     model.CatWinget,
			CurrentVer:   p.Version,
			AvailableVer: p.NewVer,
			Status:       model.StatusOutdated,
		})
	}

	if len(items) == 0 {
		items = append(items, &model.Item{
			Name: "winget", Category: model.CatWinget, Status: model.StatusOK, CurrentVer: "up to date",
		})
	}

	return items, nil
}

// scanWingetText parses `winget upgrade` text output as fallback.
func scanWingetText(ctx context.Context) ([]*model.Item, error) {
	cmd := exec.CommandContext(ctx, "winget", "upgrade")
	out, err := cmd.Output()
	if err != nil {
		return []*model.Item{
			{Name: "winget", Category: model.CatWinget, Status: model.StatusOK, CurrentVer: "up to date"},
		}, nil
	}

	lines := strings.Split(string(out), "\n")
	var items []*model.Item
	inTable := false

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "Name") && strings.Contains(line, "Id") && strings.Contains(line, "Version") {
			inTable = true
			continue
		}
		if !inTable || line == "" || strings.HasPrefix(line, "---") {
			continue
		}
		// End of table
		if strings.Contains(line, "upgrades available") {
			break
		}

		// Parse table line: "Name  Id  Version  Available  Source"
		fields := strings.Fields(line)
		if len(fields) >= 4 {
			name := fields[0]
			cur := fields[2]
			avail := fields[3]
			items = append(items, &model.Item{
				Name:         name,
				Category:     model.CatWinget,
				CurrentVer:   cur,
				AvailableVer: avail,
				Status:       model.StatusOutdated,
			})
		}
	}

	if len(items) == 0 {
		items = append(items, &model.Item{
			Name: "winget", Category: model.CatWinget, Status: model.StatusOK, CurrentVer: "up to date",
		})
	}

	return items, nil
}
