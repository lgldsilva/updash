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
	out, err := execCommand(ctx, binFlatpak, "remote-ls", "--app", "--updates", "--json")
	if err == nil {
		return parseFlatpakJSONUpdates(ctx, out)
	}

	// Older/newer flatpak builds may still support update --dry-run.
	return scanFlatpakDryRun(ctx)
}

func parseFlatpakJSONUpdates(ctx context.Context, out []byte) ([]*model.Item, error) {
	trimmed := strings.TrimSpace(string(out))
	if trimmed == "" || trimmed == "[]" {
		return []*model.Item{okItem(binFlatpak, model.CatFlatpak)}, nil
	}

	var updates []flatpakRef
	if err := json.Unmarshal(out, &updates); err != nil {
		return []*model.Item{
			{Name: binFlatpak, Category: model.CatFlatpak, Status: model.StatusError, CurrentVer: "parse error"},
		}, nil
	}

	if len(updates) == 0 {
		return []*model.Item{okItem(binFlatpak, model.CatFlatpak)}, nil
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
			cur = statusInstalled
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
		return []*model.Item{okItem(binFlatpak, model.CatFlatpak)}, nil
	}

	return items, nil
}

func flatpakInstalledVersions(ctx context.Context) map[string]string {
	out, err := execCommand(ctx, binFlatpak, cmdList, "--app", "--json")
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
	out, err := execCombined(ctx, binFlatpak, cmdUpdate, "--dry-run")
	if err != nil {
		return dryRunErrorItem(out, err), nil
	}

	output := string(out)
	if flatpakNoUpdate(output) {
		return []*model.Item{okItem(binFlatpak, model.CatFlatpak)}, nil
	}

	var items []*model.Item
	for _, line := range strings.Split(output, scannerNL) {
		if item := parseFlatpakUpdateLine(line); item != nil {
			items = append(items, item)
		}
	}

	if len(items) == 0 {
		items = append(items, &model.Item{
			Name:         binFlatpak,
			Category:     model.CatFlatpak,
			Status:       model.StatusOutdated,
			CurrentVer:   "updates pending",
			AvailableVer: "run flatpak update",
		})
	}

	return items, nil
}

func dryRunErrorItem(out []byte, err error) []*model.Item {
	msg := strings.TrimSpace(string(out))
	if msg == "" {
		msg = err.Error()
	}
	if len(msg) > 120 {
		msg = msg[:120] + "…"
	}
	return []*model.Item{{
		Name: binFlatpak, Category: model.CatFlatpak, Status: model.StatusError, CurrentVer: msg,
	}}
}

func flatpakNoUpdate(output string) bool {
	return strings.Contains(output, "Nothing to do") ||
		strings.Contains(output, "Nothing to update") ||
		strings.Contains(output, "No updates")
}

func parseFlatpakUpdateLine(line string) *model.Item {
	line = strings.TrimSpace(line)
	if !strings.Contains(line, ".") || !strings.Contains(line, "stable") || !strings.Contains(line, "org.") {
		return nil
	}
	fields := strings.Fields(line)
	if len(fields) < 5 {
		return nil
	}
	return &model.Item{
		Name:         fields[1],
		Category:     model.CatFlatpak,
		CurrentVer:   fields[2],
		AvailableVer: fields[3],
		Status:       model.StatusOutdated,
	}
}
