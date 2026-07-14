package scanner

import (
	"context"
	"strings"

	"github.com/lgldsilva/updash/internal/model"
)

// ScoopSource scans Windows packages via Scoop.
type ScoopSource struct{}

func (s *ScoopSource) Category() model.Category { return model.CatScoop }
func (s *ScoopSource) Label() string            { return "Scoop" }
func (s *ScoopSource) Icon() string             { return "🪣" }

func (s *ScoopSource) Scan(ctx context.Context, plat model.PlatformInfo) ([]*model.Item, error) {
	// scoop status shows outdated packages
	out, err := execCommand(ctx, "scoop", "status")
	if err != nil {
		return []*model.Item{
			{Name: "scoop", Category: model.CatScoop, Status: model.StatusError, CurrentVer: "error"},
		}, nil
	}

	output := string(out)
	if strings.Contains(output, "Everything is ok") {
		return []*model.Item{
			{Name: "scoop", Category: model.CatScoop, Status: model.StatusOK, CurrentVer: "up to date"},
		}, nil
	}

	// Parse scoop status output
	// Format: "  appName: 1.2.3 (latest: 1.3.0)"
	lines := strings.Split(output, "\n")
	var items []*model.Item
	inUpdates := false

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "Updates are available") {
			inUpdates = true
			continue
		}
		if !inUpdates || line == "" {
			continue
		}
		if strings.Contains(line, "WARN") || strings.Contains(line, "ERROR") {
			continue
		}
		if strings.HasPrefix(line, "'") {
			continue
		}

		// Parse "appName: currentVer (latest: newVer)"
		if strings.Contains(line, "latest:") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) < 2 {
				continue
			}
			name := strings.TrimSpace(parts[0])

			rest := parts[1]
			verParts := strings.SplitN(rest, "(", 2)
			cur := strings.TrimSpace(verParts[0])

			avail := ""
			if len(verParts) >= 2 {
				avail = strings.TrimPrefix(verParts[1], "latest: ")
				avail = strings.TrimSuffix(avail, ")")
				avail = strings.TrimSpace(avail)
			}

			items = append(items, &model.Item{
				Name:         name,
				Category:     model.CatScoop,
				CurrentVer:   cur,
				AvailableVer: avail,
				Status:       model.StatusOutdated,
			})
		}
	}

	if len(items) == 0 {
		items = append(items, &model.Item{
			Name: "scoop", Category: model.CatScoop, Status: model.StatusOK, CurrentVer: "up to date",
		})
	}

	return items, nil
}
