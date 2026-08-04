package scanner

import (
	"context"
	"strings"

	"github.com/lgldsilva/updash/internal/model"
)

// ZypperSource scans openSUSE/SLE packages via zypper.
type ZypperSource struct{}

func (s *ZypperSource) Category() model.Category { return model.CatZypper }
func (s *ZypperSource) Label() string            { return binZypper }
func (s *ZypperSource) Icon() string             { return "🦎" }

func (s *ZypperSource) Scan(ctx context.Context, plat model.PlatformInfo) ([]*model.Item, error) {
	// list-updates exits 100-103 when updates of various kinds are available,
	// so parse the table regardless of exit code and only fail on empty output.
	out, err := execCombined(ctx, binZypper, "--quiet", "--non-interactive", "list-updates")
	if err != nil && len(out) == 0 {
		return []*model.Item{errItem(binZypper, model.CatZypper)}, nil
	}
	return okOrOutdated(binZypper, model.CatZypper, ParseZypperListUpdates(string(out))), nil
}

// ParseZypperListUpdates parses `zypper --quiet list-updates` table rows:
//
//	S | Repository | Name | Current Version | Available Version | Arch
//	v | repo       | bash | 5.1.8-1.1       | 5.1.8-2.1         | x86_64
//
// Only rows marked "v" (update available) become items.
func ParseZypperListUpdates(output string) []*model.Item {
	var items []*model.Item
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "v |") {
			continue
		}
		fields := strings.Split(line, "|")
		if len(fields) < 6 {
			continue
		}
		items = append(items, &model.Item{
			Name:         strings.TrimSpace(fields[2]),
			Category:     model.CatZypper,
			CurrentVer:   strings.TrimSpace(fields[3]),
			AvailableVer: strings.TrimSpace(fields[4]),
			Status:       model.StatusOutdated,
		})
	}
	return items
}
