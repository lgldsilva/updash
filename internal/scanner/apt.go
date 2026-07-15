package scanner

import (
	"context"
	"strings"

	"github.com/lgldsilva/updash/internal/model"
)

// AptSource scans Debian/Ubuntu packages via apt.
type AptSource struct{}

func (s *AptSource) Category() model.Category { return model.CatApt }
func (s *AptSource) Label() string            { return "apt" }
func (s *AptSource) Icon() string             { return "🐧" }

func (s *AptSource) Scan(ctx context.Context, plat model.PlatformInfo) ([]*model.Item, error) {
	// Fast scan only — apt-get update is slow and may block on sudo.
	// Package lists are refreshed during apt upgrade, not here.
	out, err := execCommand(ctx, "apt", "list", "--upgradable", "-q")
	if err != nil {
		return []*model.Item{
			{Name: "apt", Category: model.CatApt, Status: model.StatusError, CurrentVer: "error"},
		}, nil
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) <= 1 { // first line is "Listing..."
		return []*model.Item{
			{Name: "apt", Category: model.CatApt, Status: model.StatusOK, CurrentVer: statusUpToDate},
		}, nil
	}

	var items []*model.Item
	for _, line := range lines[1:] {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Format: "pkg_name/stable 1.2.3 amd64 [upgradable from: 1.2.2]"
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		name := parts[0]
		if idx := strings.Index(name, "/"); idx >= 0 {
			name = name[:idx]
		}
		avail := parts[1]
		cur := ""
		if idx := strings.Index(line, "from:"); idx >= 0 {
			rest := line[idx+5:]
			rest = strings.TrimSpace(rest)
			rest = strings.TrimSuffix(rest, "]")
			cur = rest
		}

		items = append(items, &model.Item{
			Name:         name,
			Category:     model.CatApt,
			CurrentVer:   cur,
			AvailableVer: avail,
			Status:       model.StatusOutdated,
		})
	}

	if len(items) == 0 {
		items = append(items, &model.Item{
			Name: "apt", Category: model.CatApt, Status: model.StatusOK, CurrentVer: statusUpToDate,
		})
	}

	return items, nil
}
