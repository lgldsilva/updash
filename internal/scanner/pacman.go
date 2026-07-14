package scanner

import (
	"context"
	"os/exec"
	"strings"

	"github.com/lgldsilva/updash/internal/model"
)

// PacmanSource scans Arch/Manjaro packages.
type PacmanSource struct{}

func (s *PacmanSource) Category() model.Category { return model.CatPacman }
func (s *PacmanSource) Label() string            { return "pacman" }
func (s *PacmanSource) Icon() string             { return "🐧" }

func (s *PacmanSource) Scan(ctx context.Context, plat model.PlatformInfo) ([]*model.Item, error) {
	// Check if yay is available (AUR helper)
	if plat.HasYay {
		return s.scanYay(ctx)
	}
	return s.scanPacman(ctx)
}

func (s *PacmanSource) scanYay(ctx context.Context) ([]*model.Item, error) {
	// yay -Qua: list AUR + repo updates
	cmd := exec.CommandContext(ctx, "yay", "-Qua")
	out, err := cmd.Output()
	if err != nil {
		return []*model.Item{
			{Name: "yay", Category: model.CatPacman, Status: model.StatusError, CurrentVer: "error"},
		}, nil
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) == 0 || (len(lines) == 1 && lines[0] == "") {
		return []*model.Item{
			{Name: "yay", Category: model.CatPacman, Status: model.StatusOK, CurrentVer: "up to date"},
		}, nil
	}

	var items []*model.Item
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Format: "repo/pkg_name 1.2.3 -> 1.2.4"
		// or "aur/pkg_name 1.2.3 -> 1.2.4-1"
		if strings.Contains(line, "->") {
			parts := strings.Split(line, " -> ")
			left := strings.Fields(parts[0])
			avail := strings.TrimSpace(parts[1])
			name := ""
			cur := ""
			if len(left) >= 1 {
				fullName := left[0]
				if idx := strings.Index(fullName, "/"); idx >= 0 {
					name = fullName[idx+1:]
				} else {
					name = fullName
				}
			}
			if len(left) >= 2 {
				cur = left[1]
			}
			items = append(items, &model.Item{
				Name:         name,
				Category:     model.CatPacman,
				CurrentVer:   cur,
				AvailableVer: avail,
				Status:       model.StatusOutdated,
			})
		}
	}

	if len(items) == 0 {
		items = append(items, &model.Item{
			Name: "yay", Category: model.CatPacman, Status: model.StatusOK, CurrentVer: "up to date",
		})
	}

	return items, nil
}

func (s *PacmanSource) scanPacman(ctx context.Context) ([]*model.Item, error) {
	// pacman -Qu: list repo updates
	cmd := exec.CommandContext(ctx, "pacman", "-Qu")
	out, err := cmd.Output()
	if err != nil {
		return []*model.Item{
			{Name: "pacman", Category: model.CatPacman, Status: model.StatusError, CurrentVer: "error"},
		}, nil
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) == 0 || (len(lines) == 1 && lines[0] == "") {
		return []*model.Item{
			{Name: "pacman", Category: model.CatPacman, Status: model.StatusOK, CurrentVer: "up to date"},
		}, nil
	}

	var items []*model.Item
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Format: "pkg_name 1.2.3 -> 1.2.4"
		if strings.Contains(line, "->") {
			parts := strings.Split(line, " -> ")
			left := strings.Fields(parts[0])
			avail := strings.TrimSpace(parts[1])
			name := ""
			cur := ""
			if len(left) >= 1 {
				name = left[0]
			}
			if len(left) >= 2 {
				cur = left[1]
			}
			items = append(items, &model.Item{
				Name:         name,
				Category:     model.CatPacman,
				CurrentVer:   cur,
				AvailableVer: avail,
				Status:       model.StatusOutdated,
			})
		}
	}

	if len(items) == 0 {
		items = append(items, &model.Item{
			Name: "pacman", Category: model.CatPacman, Status: model.StatusOK, CurrentVer: "up to date",
		})
	}

	return items, nil
}
