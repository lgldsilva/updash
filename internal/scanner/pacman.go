package scanner

import (
	"context"
	"strings"

	"github.com/lgldsilva/updash/internal/model"
)

// PacmanSource scans Arch/Manjaro packages.
type PacmanSource struct{}

func (s *PacmanSource) Category() model.Category { return model.CatPacman }
func (s *PacmanSource) Label() string            { return binPacman }
func (s *PacmanSource) Icon() string             { return "🐧" }

func (s *PacmanSource) Scan(ctx context.Context, plat model.PlatformInfo) ([]*model.Item, error) {
	if plat.HasYay {
		return s.scanYay(ctx)
	}
	return s.scanPacman(ctx)
}

func (s *PacmanSource) scanYay(ctx context.Context) ([]*model.Item, error) {
	// `yay -Qua` restricts the check to AUR packages only, so official-repo
	// (core/extra/multilib) upgrades would go unnoticed. Combine both probes:
	// checkupdates for official repos (copies the sync db, so no root and no
	// db lock needed) and `yay -Qua` for AUR foreign packages.
	items := pacmanOfficialRepoUpdates(ctx)

	out, err := execCommand(ctx, binYay, "-Qua")
	items = append(items, parsePacmanArrowLines(string(out), true)...)
	items = dedupePacmanItems(items)
	if err != nil {
		// yay exits non-zero on benign states (no updates on some versions,
		// foreign-repo quirks). Parse whatever it printed: arrow lines are
		// updates, blank output means no updates, anything else is a real error.
		if len(items) > 0 {
			return okOrOutdated(binYay, model.CatPacman, items), nil
		}
		if len(strings.TrimSpace(string(out))) == 0 {
			return okOrOutdated(binYay, model.CatPacman, nil), nil
		}
		return []*model.Item{errItem(binYay, model.CatPacman)}, nil
	}
	return okOrOutdated(binYay, model.CatPacman, items), nil
}

// pacmanOfficialRepoUpdates lists pending official-repo upgrades via
// checkupdates (pacman-contrib). It exits 2 with empty output when nothing is
// pending and may be absent on minimal installs — both are "no items", never
// an error: the AUR probe is the authoritative signal for this source.
func pacmanOfficialRepoUpdates(ctx context.Context) []*model.Item {
	out, err := execCommand(ctx, binCheckupdates)
	if err != nil && len(out) == 0 {
		return nil
	}
	return parsePacmanArrowLines(string(out), false)
}

func dedupePacmanItems(items []*model.Item) []*model.Item {
	seen := make(map[string]struct{}, len(items))
	out := items[:0]
	for _, it := range items {
		if _, dup := seen[it.Name]; dup {
			continue
		}
		seen[it.Name] = struct{}{}
		out = append(out, it)
	}
	return out
}

func (s *PacmanSource) scanPacman(ctx context.Context) ([]*model.Item, error) {
	out, err := execCommand(ctx, binPacman, "-Qu")
	if err != nil {
		return []*model.Item{errItem(binPacman, model.CatPacman)}, nil
	}
	return okOrOutdated(binPacman, model.CatPacman, parsePacmanArrowLines(string(out), false)), nil
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
