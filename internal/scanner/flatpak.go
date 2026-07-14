package scanner

import (
	"context"
	"strings"

	"github.com/lgldsilva/updash/internal/model"
)

// FlatpakSource scans Flatpak applications.
type FlatpakSource struct{}

func (s *FlatpakSource) Category() model.Category { return model.CatFlatpak }
func (s *FlatpakSource) Label() string            { return "Flatpak" }
func (s *FlatpakSource) Icon() string             { return "📦" }

func (s *FlatpakSource) Scan(ctx context.Context, plat model.PlatformInfo) ([]*model.Item, error) {
	out, err := execCommand(ctx, "flatpak", "update", "--dry-run")
	if err != nil {
		return []*model.Item{
			{Name: "flatpak", Category: model.CatFlatpak, Status: model.StatusError, CurrentVer: "error"},
		}, nil
	}

	output := string(out)
	if strings.Contains(output, "Nothing to do") || strings.Contains(output, "No updates") {
		return []*model.Item{
			{Name: "flatpak", Category: model.CatFlatpak, Status: model.StatusOK, CurrentVer: "up to date"},
		}, nil
	}

	// Parse the dry-run output to find updates
	lines := strings.Split(output, "\n")
	var items []*model.Item
	for _, line := range lines {
		line = strings.TrimSpace(line)
		// Look for lines like: "1.    org.gnome.Platform     3.38     4.0      stable"
		if strings.Contains(line, ".") && strings.Contains(line, "stable") && strings.Contains(line, "org.") {
			fields := strings.Fields(line)
			if len(fields) >= 5 {
				name := fields[1]
				cur := fields[2]
				avail := fields[3]
				items = append(items, &model.Item{
					Name:         name,
					Category:     model.CatFlatpak,
					CurrentVer:   cur,
					AvailableVer: avail,
					Status:       model.StatusOutdated,
				})
			}
		}
	}

	if len(items) == 0 {
		// Updates available but couldn't parse individually
		items = append(items, &model.Item{
			Name:         "flatpak",
			Category:     model.CatFlatpak,
			Status:       model.StatusOutdated,
			CurrentVer:   "updates pending",
			AvailableVer: "run flatpak update",
		})
	}

	return items, nil
}
