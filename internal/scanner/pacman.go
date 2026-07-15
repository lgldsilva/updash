package scanner

import (
	"context"
	"strings"

	"github.com/lgldsilva/updash/internal/model"
)

// PacmanSource scans Arch/Manjaro packages.
type PacmanSource struct{}

func (s *PacmanSource) Category() model.Category { return model.CatPacman }
func (s *PacmanSource) Label() string            { return "pacman" }
func (s *PacmanSource) Icon() string             { return "🐧" }

func (s *PacmanSource) Scan(ctx context.Context, plat model.PlatformInfo) ([]*model.Item, error) {
	if plat.HasYay {
		return s.scanYay(ctx)
	}
	return s.scanPacman(ctx)
}

func (s *PacmanSource) scanYay(ctx context.Context) ([]*model.Item, error) {
	out, err := execCommand(ctx, "yay", "-Qua")
	if err != nil {
		return []*model.Item{errItem("yay", model.CatPacman)}, nil
	}
	return okOrOutdated("yay", model.CatPacman, parsePacmanArrowLines(string(out), true)), nil
}

func (s *PacmanSource) scanPacman(ctx context.Context) ([]*model.Item, error) {
	out, err := execCommand(ctx, "pacman", "-Qu")
	if err != nil {
		return []*model.Item{errItem("pacman", model.CatPacman)}, nil
	}
	return okOrOutdated("pacman", model.CatPacman, parsePacmanArrowLines(string(out), false)), nil
}

// parsePacmanArrowLines parses "name 1.0 -> 2.0" or "repo/name 1.0 -> 2.0" lines.
func parsePacmanArrowLines(output string, stripRepo bool) []*model.Item {
	var items []*model.Item
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || !strings.Contains(line, "->") {
			continue
		}
		if it := parsePacmanArrowLine(line, stripRepo); it != nil {
			items = append(items, it)
		}
	}
	return items
}

func parsePacmanArrowLine(line string, stripRepo bool) *model.Item {
	parts := strings.Split(line, " -> ")
	if len(parts) < 2 {
		return nil
	}
	left := strings.Fields(parts[0])
	if len(left) < 1 {
		return nil
	}
	name := left[0]
	if stripRepo {
		if idx := strings.Index(name, "/"); idx >= 0 {
			name = name[idx+1:]
		}
	}
	cur := ""
	if len(left) >= 2 {
		cur = left[1]
	}
	return &model.Item{
		Name:         name,
		Category:     model.CatPacman,
		CurrentVer:   cur,
		AvailableVer: strings.TrimSpace(parts[1]),
		Status:       model.StatusOutdated,
	}
}
