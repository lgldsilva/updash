package scanner

import (
	"context"
	"strings"

	"github.com/lgldsilva/updash/internal/model"
)

// SnapSource scans Snap packages.
type SnapSource struct{}

func (s *SnapSource) Category() model.Category { return model.CatSnap }
func (s *SnapSource) Label() string            { return "Snap" }
func (s *SnapSource) Icon() string             { return "📦" }

func (s *SnapSource) Scan(ctx context.Context, plat model.PlatformInfo) ([]*model.Item, error) {
	// snap refresh --list shows available updates
	out, err := execCommand(ctx, "snap", "refresh", "--list")
	if err != nil {
		return []*model.Item{
			{Name: "snap", Category: model.CatSnap, Status: model.StatusError, CurrentVer: "error"},
		}, nil
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) <= 1 {
		return []*model.Item{
			{Name: "snap", Category: model.CatSnap, Status: model.StatusOK, CurrentVer: statusUpToDate},
		}, nil
	}

	var items []*model.Item
	header := true
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || header {
			header = false
			continue
		}
		// Format: "Name    Version   Rev   Tracking  Publisher  Notes"
		// Just extract the name and version
		fields := strings.Fields(line)
		if len(fields) >= 4 {
			name := fields[0]
			// The second field is the new version, but we don't have "current" easily
			items = append(items, &model.Item{
				Name:         name,
				Category:     model.CatSnap,
				CurrentVer:   "?",
				AvailableVer: fields[1],
				Status:       model.StatusOutdated,
			})
		}
	}

	if len(items) == 0 {
		items = append(items, &model.Item{
			Name: "snap", Category: model.CatSnap, Status: model.StatusOK, CurrentVer: statusUpToDate,
		})
	}

	return items, nil
}
