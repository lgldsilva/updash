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
	out, err := execCommand(ctx, "scoop", "status")
	if err != nil {
		return []*model.Item{errItem("scoop", model.CatScoop)}, nil
	}
	output := string(out)
	if strings.Contains(output, "Everything is ok") {
		return []*model.Item{okItem("scoop", model.CatScoop)}, nil
	}
	return okOrOutdated("scoop", model.CatScoop, parseScoopStatus(output)), nil
}

// parseScoopStatus parses "appName: cur (latest: new)" lines after the updates header.
func parseScoopStatus(output string) []*model.Item {
	var items []*model.Item
	inUpdates := false
	for _, raw := range strings.Split(output, "\n") {
		line := strings.TrimSpace(raw)
		switch {
		case strings.Contains(line, "Updates are available"):
			inUpdates = true
			continue
		case !inUpdates, line == "", strings.Contains(line, "WARN"), strings.Contains(line, "ERROR"):
			continue
		case strings.HasPrefix(line, "'"), !strings.Contains(line, "latest:"):
			continue
		}
		if it := parseScoopUpdateLine(line); it != nil {
			items = append(items, it)
		}
	}
	return items
}

func parseScoopUpdateLine(line string) *model.Item {
	parts := strings.SplitN(line, ":", 2)
	if len(parts) < 2 {
		return nil
	}
	name := strings.TrimSpace(parts[0])
	verParts := strings.SplitN(parts[1], "(", 2)
	cur := strings.TrimSpace(verParts[0])
	avail := ""
	if len(verParts) >= 2 {
		avail = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(verParts[1], "latest: "), ")"))
	}
	return &model.Item{
		Name: name, Category: model.CatScoop,
		CurrentVer: cur, AvailableVer: avail, Status: model.StatusOutdated,
	}
}
