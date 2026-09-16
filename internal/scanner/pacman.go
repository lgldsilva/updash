package scanner

import (
	"context"
	"errors"
	"os/exec"
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
	official, probe := pacmanOfficialRepoUpdates(ctx)
	out, err := execCommand(ctx, binYay, "-Qua")
	return combineYayScan(official, probe, out, err), nil
}

func combineYayScan(official []*model.Item, probe *model.Item, out []byte, err error) []*model.Item {
	aur := parsePacmanArrowLines(string(out), true)
	items := dedupePacmanItems(append(append([]*model.Item{}, official...), aur...))
	if probe != nil {
		items = append(items, probe)
	}
	if err != nil && len(aur) == 0 && len(strings.TrimSpace(string(out))) != 0 {
		items = append(items, errItem(binYay, model.CatPacman, err))
	}
	return okOrOutdated(binYay, model.CatPacman, items)
}

// pacmanOfficialRepoUpdates lists pending official-repo upgrades via
// checkupdates (pacman-contrib). Exit 2 + empty stdout is "none pending".
// A missing binary falls back to `pacman -Qu`. Any other empty failure is a
// probe item (never StatusOK): claiming "up to date" would skip `-Syu`.
func pacmanOfficialRepoUpdates(ctx context.Context) ([]*model.Item, *model.Item) {
	out, err := execCommand(ctx, binCheckupdates)
	switch {
	case err == nil:
		return parsePacmanArrowLines(string(out), false), nil
	case isNonePending(err, out):
		return nil, nil
	case isMissingBinary(err):
		return pacmanQuOfficial(ctx)
	case len(out) > 0:
		return parsePacmanArrowLines(string(out), false), nil
	default:
		return nil, errItem(binPacman, model.CatPacman, err)
	}
}

func pacmanQuOfficial(ctx context.Context) ([]*model.Item, *model.Item) {
	out, err := execCommand(ctx, binPacman, "-Qu")
	if err == nil || len(out) > 0 {
		return parsePacmanArrowLines(string(out), false), nil
	}
	it := &model.Item{
		Name:       binPacman,
		Category:   model.CatPacman,
		Status:     model.StatusUnverified,
		CurrentVer: "checkupdates missing",
		Error:      errCause(err),
	}
	return nil, it
}

func isNonePending(err error, out []byte) bool {
	if err == nil || len(out) != 0 {
		return false
	}
	if isExitCode(err, 2) {
		return true
	}
	msg := err.Error()
	return msg == "exit status 2" || strings.HasPrefix(msg, "exit status 2 ")
}

func isMissingBinary(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, exec.ErrNotFound) {
		return true
	}
	var pe *exec.Error
	if errors.As(err, &pe) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "executable file not found")
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
		return []*model.Item{errItem(binPacman, model.CatPacman, err)}, nil
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
