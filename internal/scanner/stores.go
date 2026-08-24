package scanner

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/lgldsilva/updash/internal/model"
)

// PnpmSource scans pnpm global packages.
type PnpmSource struct{}

func (s *PnpmSource) Category() model.Category { return model.CatPnpm }
func (s *PnpmSource) Label() string            { return "pnpm (global)" }
func (s *PnpmSource) Icon() string             { return "📦" }

func (s *PnpmSource) Scan(ctx context.Context, plat model.PlatformInfo) ([]*model.Item, error) {
	// stdout only: pnpm writes deprecation/funding warnings to stderr, and
	// merging them corrupts the JSON (see the execCombined doc in runner.go).
	out, err := execCommand(ctx, binPnpm, "outdated", flagGlobal, "--json")
	if err != nil && len(out) == 0 {
		return []*model.Item{errItem(binPnpm, model.CatPnpm)}, nil
	}
	items, parseErr := parsePnpmOutdatedGlobal(out)
	if parseErr != nil {
		return []*model.Item{errItem(binPnpm, model.CatPnpm)}, nil
	}
	return okOrOutdated(binPnpm, model.CatPnpm, items), nil
}

// pnpmOutdatedEntry is one package from `pnpm outdated --json`.
type pnpmOutdatedEntry struct {
	Current string `json:"current"`
	Wanted  string `json:"wanted"`
	Latest  string `json:"latest"`
}

// ParsePnpmOutdatedGlobal converts `pnpm outdated -g --json` into items.
// The global-dir row (path key, empty metadata) and up-to-date entries are skipped.
func ParsePnpmOutdatedGlobal(out []byte) []*model.Item {
	items, _ := parsePnpmOutdatedGlobal(out)
	return items
}

func parsePnpmOutdatedGlobal(out []byte) ([]*model.Item, error) {
	var data map[string]pnpmOutdatedEntry
	if err := json.Unmarshal(out, &data); err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, nil
	}
	var items []*model.Item
	for name, pkg := range data {
		if pkg.Latest == "" || strings.HasPrefix(name, "/") {
			continue
		}
		if pkg.Current != "" && pkg.Current == pkg.Latest {
			continue
		}
		items = append(items, &model.Item{
			Name:         name,
			Category:     model.CatPnpm,
			CurrentVer:   pkg.Current,
			AvailableVer: pkg.Latest,
			Status:       model.StatusOutdated,
		})
	}
	return items, nil
}

// BunSource lists bun global packages (presence + versions; bun has no
// reliable outdated check, so items stay informational unless bun itself
// reports an update during `bun update -g`).
type BunSource struct{}

func (s *BunSource) Category() model.Category { return model.CatBun }
func (s *BunSource) Label() string            { return "bun (global)" }
func (s *BunSource) Icon() string             { return "🍞" }

func (s *BunSource) Scan(ctx context.Context, plat model.PlatformInfo) ([]*model.Item, error) {
	out, err := execCombined(ctx, binBun, "pm", "ls", flagGlobal)
	if err != nil && len(out) == 0 {
		return []*model.Item{errItem(binBun, model.CatBun)}, nil
	}
	return okOrOutdated(binBun, model.CatBun, ParseBunPmLsGlobal(string(out))), nil
}

// ParseBunPmLsGlobal parses `bun pm ls -g` tree rows like:
//
//	/home/user/.bun/install/global/node_modules (2)
//	├── @anthropic-ai/claude-code@1.2.3
//	└── typescript@5.5.4
func ParseBunPmLsGlobal(output string) []*model.Item {
	var items []*model.Item
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if idx := strings.Index(line, "── "); idx >= 0 {
			line = strings.TrimSpace(line[idx+len("── "):])
		} else {
			continue
		}
		name, ver := splitNameAt(line)
		if name == "" {
			continue
		}
		items = append(items, &model.Item{
			Name:       name,
			Category:   model.CatBun,
			CurrentVer: ver,
			Status:     model.StatusInfo,
		})
	}
	return items
}

// splitNameAt splits "pkg@version" at the LAST '@' (scoped names start with '@').
func splitNameAt(s string) (string, string) {
	idx := strings.LastIndex(s, "@")
	if idx <= 0 {
		return s, ""
	}
	return s[:idx], strings.TrimPrefix(s[idx+1:], " ")
}
