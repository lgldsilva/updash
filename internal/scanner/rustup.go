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
	cmd := exec.CommandContext(ctx, "rustup", "check")
	out, err := cmd.Output()
	if err != nil {
		return []*model.Item{
			{Name: "rustup", Category: model.CatRustup, Status: model.StatusError, CurrentVer: "error"},
		}, nil
	}

	output := string(out)
	if strings.Contains(output, "out of date") || strings.Contains(output, "Update available") {
		// Parse lines like "rustc 1.84.0-x86_64-unknown-linux-gnu is out of date"
		lines := strings.Split(strings.TrimSpace(output), "\n")
		var items []*model.Item
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.Contains(line, "out of date") || strings.Contains(line, "Update available") {
				parts := strings.Fields(line)
				if len(parts) >= 2 {
					items = append(items, &model.Item{
						Name:     parts[0],
						Category: model.CatRustup,
						Status:   model.StatusOutdated,
					})
				}
			} else if strings.Contains(line, "is up to date") {
				parts := strings.Fields(line)
				if len(parts) >= 2 {
					items = append(items, &model.Item{
						Name:       parts[0],
						Category:   model.CatRustup,
						CurrentVer: "up to date",
						Status:     model.StatusOK,
					})
				}
			}
		}
		if len(items) > 0 {
			return items, nil
		}
	}

	return []*model.Item{
		{Name: "rustup", Category: model.CatRustup, Status: model.StatusOK, CurrentVer: "up to date"},
	}, nil
}

// CargoSource scans cargo-installed tools via cargo-install-update.
type CargoSource struct{}

func (s *CargoSource) Category() model.Category { return model.CatCargo }
func (s *CargoSource) Label() string            { return "cargo" }
func (s *CargoSource) Icon() string             { return "🦀" }

func (s *CargoSource) Scan(ctx context.Context, plat model.PlatformInfo) ([]*model.Item, error) {
	// cargo-install-update depends on external binary
	if _, err := exec.LookPath("cargo-install-update"); err != nil {
		return []*model.Item{
			{Name: "cargo", Category: model.CatCargo, Status: model.StatusOK, CurrentVer: "not installed"},
		}, nil
	}

	return []*model.Item{
		{Name: "cargo", Category: model.CatCargo, Status: model.StatusOK, CurrentVer: "cargo-install-update available"},
	}, nil
}
