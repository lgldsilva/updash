package scanner

import (
	"context"
	"strings"

	"github.com/lgldsilva/updash/internal/model"
)

// MASource scans Mac App Store apps via `mas`.
type MASource struct{}

func (s *MASource) Category() model.Category { return model.CatMAS }
func (s *MASource) Label() string            { return "Mac App Store" }
func (s *MASource) Icon() string             { return "📱" }

func (s *MASource) Scan(ctx context.Context, plat model.PlatformInfo) ([]*model.Item, error) {
	// mas list shows installed apps
	// mas outdated shows available updates
	out, err := execCommand(ctx, "mas", "outdated")
	if err != nil {
		return []*model.Item{
			{Name: "mas", Category: model.CatMAS, Status: model.StatusError, CurrentVer: "error"},
		}, nil
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) == 0 || (len(lines) == 1 && lines[0] == "") {
		return []*model.Item{
			{Name: "mas", Category: model.CatMAS, Status: model.StatusOK, CurrentVer: "up to date"},
		}, nil
	}

	var items []*model.Item
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Format: "1234567890 AppName (1.0.0 -> 2.0.0)"
		// or maybe just "1234567890  AppName"
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		appID := parts[0]
		name := strings.Join(parts[1:], " ")
		cur := ""
		avail := ""

		// Try to extract versions from "(cur -> avail)"
		if idx := strings.Index(name, "("); idx >= 0 {
			verPart := name[idx:]
			name = strings.TrimSpace(name[:idx])
			verPart = strings.TrimPrefix(verPart, "(")
			verPart = strings.TrimSuffix(verPart, ")")
			if arrow := strings.Index(verPart, "->"); arrow >= 0 {
				cur = strings.TrimSpace(verPart[:arrow])
				avail = strings.TrimSpace(verPart[arrow+2:])
			}
		}

		items = append(items, &model.Item{
			Name:         name,
			PackageID:    appID,
			Category:     model.CatMAS,
			CurrentVer:   cur,
			AvailableVer: avail,
			Status:       model.StatusOutdated,
		})
	}

	if len(items) == 0 {
		items = append(items, &model.Item{
			Name: "mas", Category: model.CatMAS, Status: model.StatusOK, CurrentVer: "up to date",
		})
	}

	return items, nil
}
