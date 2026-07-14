package scanner

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/lgldsilva/updash/internal/model"
)

type flatpakRef struct {
	Name          string `json:"name"`
	ApplicationID string `json:"application_id"`
	Version       string `json:"version"`
	Branch        string `json:"branch"`
}

// FlatpakSource scans Flatpak applications.
type FlatpakSource struct{}

func (s *FlatpakSource) Category() model.Category { return model.CatFlatpak }
func (s *FlatpakSource) Label() string            { return "Flatpak" }
func (s *FlatpakSource) Icon() string             { return "📦" }

func (s *FlatpakSource) Scan(ctx context.Context, plat model.PlatformInfo) ([]*model.Item, error) {
	out, err := execCommand(ctx, "flatpak", "remote-ls", "--app", "--updates", "--json")
	if err == nil {
		return parseFlatpakJSONUpdates(ctx, out)
	}

	// Older/newer flatpak builds may still support update --dry-run.
	return scanFlatpakDryRun(ctx)
}

func parseFlatpakJSONUpdates(ctx context.Context, out []byte) ([]*model.Item, error) {
	trimmed := strings.TrimSpace(string(out))
	if trimmed == "" || trimmed == "[]" {
		return []*model.Item{
			{Name: "flatpak", Category: model.CatFlatpak, Status: model.StatusOK, CurrentVer: "up to date"},
		}, nil
	}

	var updates []flatpakRef
	if err := json.Unmarshal(out, &updates); err != nil {
		return []*model.Item{
			{Name: "flatpak", Category: model.CatFlatpak, Status: model.StatusError, CurrentVer: "parse error"},
		}, nil
	}

	if len(updates) == 0 {
		return []*model.Item{
			{Name: "flatpak", Category: model.CatFlatpak, Status: model.StatusOK, CurrentVer: "up to date"},
		}, nil
	}

	installed := flatpakInstalledVersions(ctx)
	var items []*model.Item
	for _, upd := range updates {
		id := upd.ApplicationID
		if id == "" {
			continue
		}
		cur := installed[id]
		if cur == "" {
			cur = "installed"
		}
		avail := upd.Version
		if avail == "" {
			avail = "update available"
		}
		items = append(items, &model.Item{
			Name:         id,
			Category:     model.CatFlatpak,
			CurrentVer:   cur,
			AvailableVer: avail,
			Status:       model.StatusOutdated,
		})
	}

	if len(items) == 0 {
		return []*model.Item{
			{Name: "flatpak", Category: model.CatFlatpak, Status: model.StatusOK, CurrentVer: "up to date"},
		}, nil
	}

	return items, nil
}

func flatpakInstalledVersions(ctx context.Context) map[string]string {
	out, err := execCommand(ctx, "flatpak", "list", "--app", "--json")
	if err != nil {
		return nil
	}
	var installed []flatpakRef
	if err := json.Unmarshal(out, &installed); err != nil {
		return nil
	}
	vers := make(map[string]string, len(installed))
	for _, ref := range installed {
		if ref.ApplicationID != "" && ref.Version != "" {
			vers[ref.ApplicationID] = ref.Version
		}
	}
	return vers
}

func scanFlatpakDryRun(ctx context.Context) ([]*model.Item, error) {
	out, err := execCombined(ctx, "flatpak", "update", "--dry-run")
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		if len(msg) > 120 {
			msg = msg[:120] + "…"
		}
		return []*model.Item{
			{Name: "flatpak", Category: model.CatFlatpak, Status: model.StatusError, CurrentVer: msg},
		}, nil
	}

	output := string(out)
	if strings.Contains(output, "Nothing to do") || strings.Contains(output, "Nothing to update") || strings.Contains(output, "No updates") {
		return []*model.Item{
			{Name: "flatpak", Category: model.CatFlatpak, Status: model.StatusOK, CurrentVer: "up to date"},
		}, nil
	}

	lines := strings.Split(output, "\n")
	var items []*model.Item
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.Contains(line, ".") && strings.Contains(line, "stable") && strings.Contains(line, "org.") {
			fields := strings.Fields(line)
			if len(fields) >= 5 {
				items = append(items, &model.Item{
					Name:         fields[1],
					Category:     model.CatFlatpak,
					CurrentVer:   fields[2],
					AvailableVer: fields[3],
					Status:       model.StatusOutdated,
				})
			}
		}
	}

	if len(items) == 0 {
		items = append(items, &model.Item{
			Name:         "flatpak",
			Category:     model.CatFlatpak,
			Status:       model.StatusOutdated,
			CurrentVer:   "updates pending",
			AvailableVer: "run flatpak update",
		})
	}

	return items, nil
}
