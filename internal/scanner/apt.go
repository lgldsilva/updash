package scanner

import (
	"context"
	"os/exec"
	"strings"

	"github.com/lgldsilva/updash/internal/model"
)

// AptSource scans Debian/Ubuntu packages via apt.
type AptSource struct{}

func (s *AptSource) Category() model.Category { return model.CatApt }
func (s *AptSource) Label() string            { return "apt" }
func (s *AptSource) Icon() string             { return "🐧" }

func (s *AptSource) Scan(ctx context.Context, plat model.PlatformInfo) ([]*model.Item, error) {
	// First update package lists (required for apt list --upgradable)
	update := exec.CommandContext(ctx, "sudo", "apt-get", "update")
	_ = update.Run() // ignore errors, proceed anyway

	// List upgradable packages
	out, err := execCommand(ctx, "apt", "list", "--upgradable", "-q")
	if err != nil {
		return []*model.Item{
			{Name: "apt", Category: model.CatApt, Status: model.StatusError, CurrentVer: "error"},
		}, nil
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) <= 1 { // first line is "Listing..."
		return []*model.Item{
			{Name: "apt", Category: model.CatApt, Status: model.StatusOK, CurrentVer: "up to date"},
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
			Name: "apt", Category: model.CatApt, Status: model.StatusOK, CurrentVer: "up to date",
		})
	}

	return items, nil
}
