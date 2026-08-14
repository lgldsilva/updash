package scanner

import (
	"context"
	"os/exec"
	"strings"

	"github.com/lgldsilva/updash/internal/model"
)

// RustupSource scans Rust toolchains.
type RustupSource struct{}

func (s *RustupSource) Category() model.Category { return model.CatRustup }
func (s *RustupSource) Label() string            { return binRustup }
func (s *RustupSource) Icon() string             { return "🦀" }

func (s *RustupSource) Scan(ctx context.Context, plat model.PlatformInfo) ([]*model.Item, error) {
	out, err := execCommand(ctx, binRustup, "check")
	if err != nil {
		return []*model.Item{errItem(binRustup, model.CatRustup)}, nil
	}
	if items := parseRustupCheck(string(out)); len(items) > 0 {
		return items, nil
	}
	return []*model.Item{okItem(binRustup, model.CatRustup)}, nil
}

func parseRustupCheck(output string) []*model.Item {
	if !strings.Contains(output, "out of date") && !strings.Contains(output, "Update available") {
		return nil
	}
	var items []*model.Item
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if it := parseRustupCheckLine(strings.TrimSpace(line)); it != nil {
			items = append(items, it)
		}
	}
	return items
}

func parseRustupCheckLine(line string) *model.Item {
	parts := strings.Fields(line)
	if len(parts) < 2 {
		return nil
	}
	switch {
	case strings.Contains(line, "out of date"), strings.Contains(line, "Update available"):
		return &model.Item{Name: parts[0], Category: model.CatRustup, Status: model.StatusOutdated}
	case strings.Contains(line, "is up to date"):
		return &model.Item{
			Name: parts[0], Category: model.CatRustup,
			CurrentVer: statusUpToDate, Status: model.StatusOK,
		}
	default:
		return nil
	}
}

// CargoSource scans cargo-installed tools via cargo-install-update.
type CargoSource struct{}

func (s *CargoSource) Category() model.Category { return model.CatCargo }
func (s *CargoSource) Label() string            { return binCargo }
func (s *CargoSource) Icon() string             { return "🦀" }

func (s *CargoSource) Scan(ctx context.Context, plat model.PlatformInfo) ([]*model.Item, error) {
	if _, err := exec.LookPath("cargo-install-update"); err != nil {
		return []*model.Item{
			{Name: binCargo, Category: model.CatCargo, Status: model.StatusOK, CurrentVer: "not installed"},
		}, nil
	}

	out, err := execCommand(ctx, "cargo-install-update", "-l")
	if err != nil {
		return []*model.Item{errItem(binCargo, model.CatCargo)}, nil
	}

	items := parseCargoInstallUpdate(string(out))
	if len(items) == 0 {
		return []*model.Item{okItem(binCargo, model.CatCargo)}, nil
	}
	return items, nil
}

// parseCargoInstallUpdate parses the output of `cargo-install-update -l`.
// Typical output:
//
//	Package       Installed  Latest  Needs update
//	cargo-edit    0.11.0     0.11.1  Yes
//	cargo-watch   0.18.0     0.18.0  No
func parseCargoInstallUpdate(output string) []*model.Item {
	var items []*model.Item
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(strings.ToLower(line), "package") {
			continue
		}
		if it := parseCargoInstallUpdateLine(line); it != nil {
			items = append(items, it)
		}
	}
	return items
}

func parseCargoInstallUpdateLine(line string) *model.Item {
	parts := strings.Fields(line)
	if len(parts) < 3 {
		return nil
	}
	name := parts[0]
	current := parts[1]
	latest := parts[2]
	needsUpdate := false
	if len(parts) >= 4 {
		needsUpdate = strings.EqualFold(parts[3], "yes") || strings.EqualFold(parts[3], "true")
	} else {
		needsUpdate = current != latest
	}
	if needsUpdate {
		return &model.Item{
			Name:         name,
			Category:     model.CatCargo,
			CurrentVer:   current,
			AvailableVer: latest,
			Status:       model.StatusOutdated,
		}
	}
	return &model.Item{
		Name:       name,
		Category:   model.CatCargo,
		CurrentVer: current,
		Status:     model.StatusOK,
	}
}
