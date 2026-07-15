package scanner

import (
	"context"
	"strings"

	"github.com/lgldsilva/updash/internal/model"
)

// GoSource scans Go tools via gup (preferred) or lists GOPATH tools.
type GoSource struct{}

func (s *GoSource) Category() model.Category { return model.CatGo }
func (s *GoSource) Label() string            { return "Go tools" }
func (s *GoSource) Icon() string             { return "🔷" }

func (s *GoSource) Scan(ctx context.Context, plat model.PlatformInfo) ([]*model.Item, error) {
	// If gup is available, use it to list outdated
	if plat.HasGup {
		return s.scanGup(ctx)
	}

	// Otherwise, just list installed tools in GOPATH/bin
	gopathBytes, err := execCommand(ctx, "go", "env", "GOPATH")
	if err != nil {
		return []*model.Item{
			{Name: "go", Category: model.CatGo, Status: model.StatusError, CurrentVer: "error"},
		}, nil
	}
	gopath := strings.TrimSpace(string(gopathBytes))

	// List Go binaries
	out, err := execCommand(ctx, "ls", gopath+"/bin")
	if err != nil {
		return []*model.Item{okItem("go", model.CatGo)}, nil
	}

	names := strings.Fields(string(out))
	if len(names) == 0 {
		return []*model.Item{okItem("go", model.CatGo)}, nil
	}

	var items []*model.Item
	for _, name := range names {
		items = append(items, &model.Item{
			Name:     name,
			Category: model.CatGo,
			Status:   model.StatusOK,
		})
	}

	return items, nil
}

func (s *GoSource) scanGup(ctx context.Context) ([]*model.Item, error) {
	out, err := execCommand(ctx, "gup", "update", "--dry-run")
	if err != nil {
		return []*model.Item{okItem("go", model.CatGo)}, nil
	}

	output := string(out)
	if strings.Contains(output, statusUpToDate) || output == "" {
		return []*model.Item{okItem("go", model.CatGo)}, nil
	}

	lines := strings.Split(strings.TrimSpace(output), "\n")
	var items []*model.Item
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.Contains(line, "Checking") || strings.Contains(line, "Dry-run") {
			continue
		}
		if strings.Contains(line, "->") {
			parts := strings.Split(line, "->")
			name := strings.TrimSpace(parts[0])
			avail := strings.TrimSpace(parts[1])
			items = append(items, &model.Item{
				Name:         name,
				Category:     model.CatGo,
				AvailableVer: avail,
				Status:       model.StatusOutdated,
			})
		}
	}

	if len(items) == 0 {
		items = append(items, okItem("go", model.CatGo))
	}

	return items, nil
}
