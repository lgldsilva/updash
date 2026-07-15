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
func (s *RustupSource) Label() string            { return "rustup" }
func (s *RustupSource) Icon() string             { return "🦀" }

func (s *RustupSource) Scan(ctx context.Context, plat model.PlatformInfo) ([]*model.Item, error) {
	out, err := execCommand(ctx, "rustup", "check")
	if err != nil {
		return []*model.Item{errItem("rustup", model.CatRustup)}, nil
	}
	if items := parseRustupCheck(string(out)); len(items) > 0 {
		return items, nil
	}
	return []*model.Item{okItem("rustup", model.CatRustup)}, nil
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
func (s *CargoSource) Label() string            { return "cargo" }
func (s *CargoSource) Icon() string             { return "🦀" }

func (s *CargoSource) Scan(ctx context.Context, plat model.PlatformInfo) ([]*model.Item, error) {
	if _, err := exec.LookPath("cargo-install-update"); err != nil {
		return []*model.Item{
			{Name: "cargo", Category: model.CatCargo, Status: model.StatusOK, CurrentVer: "not installed"},
		}, nil
	}
	return []*model.Item{
		{Name: "cargo", Category: model.CatCargo, Status: model.StatusOK, CurrentVer: "cargo-install-update available"},
	}, nil
}
