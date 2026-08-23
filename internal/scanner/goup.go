package scanner

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
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
	gopathBytes, err := execCommand(ctx, binGo, "env", "GOPATH")
	if err != nil {
		return []*model.Item{
			{Name: binGo, Category: model.CatGo, Status: model.StatusError, CurrentVer: "error"},
		}, nil
	}
	gopath := strings.TrimSpace(string(gopathBytes))

	// List Go binaries using native Go APIs so this works on Windows too.
	return scanGoBinInventory(filepath.Join(gopath, "bin"), os.ReadDir), nil
}

func scanGoBinInventory(dir string, readDir func(string) ([]os.DirEntry, error)) []*model.Item {
	entries, err := readDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []*model.Item{{Name: binGo, Category: model.CatGo, Status: model.StatusInfo, CurrentVer: "no Go tools installed"}}
		}
		return []*model.Item{{Name: binGo, Category: model.CatGo, Status: model.StatusError, CurrentVer: "unable to read Go tools"}}
	}
	if len(entries) == 0 {
		return []*model.Item{{Name: binGo, Category: model.CatGo, Status: model.StatusInfo, CurrentVer: "no Go tools installed"}}
	}
	var items []*model.Item
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if runtime.GOOS == "windows" && strings.HasSuffix(name, ".exe") {
			name = strings.TrimSuffix(name, ".exe")
		}
		items = append(items, &model.Item{
			Name:     name,
			Category: model.CatGo,
			Status:   model.StatusInfo,
		})
	}

	return items
}

func (s *GoSource) scanGup(ctx context.Context) ([]*model.Item, error) {
	out, err := execCommand(ctx, "gup", cmdUpdate, "--dry-run")
	if err != nil {
		return []*model.Item{{Name: binGo, Category: model.CatGo, Status: model.StatusError, CurrentVer: "gup check failed"}}, nil
	}

	output := string(out)
	if strings.Contains(strings.ToLower(output), statusUpToDate) || strings.Contains(strings.ToLower(output), "nothing to update") || output == "" {
		return []*model.Item{okItem(binGo, model.CatGo)}, nil
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
		items = append(items, &model.Item{Name: binGo, Category: model.CatGo, Status: model.StatusUnverified, CurrentVer: "unrecognized gup output"})
	}

	return items, nil
}
