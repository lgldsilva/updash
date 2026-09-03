package scanner

import (
	"context"
	"errors"
	"testing"

	"github.com/lgldsilva/updash/internal/model"
)

var errProbeFailed = errors.New("index unreachable")

func TestPipxScan_MarksOutdatedPackages(t *testing.T) {
	enableMocks()
	defer disableMocks()

	out := `{"venvs": {"poetry": {"metadata": {"item_version": "1.8.0"}}, "ruff": {"metadata": {"item_version": "0.6.0"}}}}`
	setMock("pipx", []string{"list", "--json"}, out, nil)
	setMock("pipx", []string{"runpip", "poetry", "list", "--outdated", "--format=json"},
		`[{"name": "poetry", "version": "1.8.0", "latest_version": "1.8.3"}]`, nil)
	setMock("pipx", []string{"runpip", "ruff", "list", "--outdated", "--format=json"}, `[]`, nil)

	src := &PipxSource{}
	items, err := src.Scan(context.Background(), model.PlatformInfo{})
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %+v", items)
	}
	byName := map[string]*model.Item{}
	for _, it := range items {
		byName[it.Name] = it
	}
	if p := byName["poetry"]; p == nil || p.Status != model.StatusOutdated || p.AvailableVer != "1.8.3" {
		t.Errorf("poetry should be outdated with available 1.8.3: %+v", p)
	}
	if r := byName["ruff"]; r == nil || r.Status != model.StatusOK {
		t.Errorf("ruff should be confirmed up-to-date: %+v", r)
	}
}

func TestPipxScan_ProbeFailureLeavesInfo(t *testing.T) {
	enableMocks()
	defer disableMocks()

	out := `{"venvs": {"poetry": {"metadata": {"item_version": "1.8.0"}}}}`
	setMock("pipx", []string{"list", "--json"}, out, nil)
	// Simulate an unreachable index / broken venv: the probe errors and the
	// item must stay informational, never an unverified "outdated".
	setMock("pipx", []string{"runpip", "poetry", "list", "--outdated", "--format=json"}, "", errProbeFailed)

	src := &PipxSource{}
	items, err := src.Scan(context.Background(), model.PlatformInfo{})
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}
	if len(items) != 1 || items[0].Status != model.StatusInfo || items[0].Status == model.StatusOutdated {
		t.Fatalf("probe failure must leave informational status: %+v", items)
	}
}

func TestParsePipxOutdated(t *testing.T) {
	out := `[{"name": "Poetry", "version": "1.8.0", "latest_version": "1.8.3"}, {"name": "pip", "version": "24.0", "latest_version": "24.1"}]`
	if got := parsePipxOutdated(out, "poetry"); got != "1.8.3" {
		t.Errorf("case-insensitive name match failed: %q", got)
	}
	if got := parsePipxOutdated(out, "pipx"); got != "" {
		t.Errorf("absent package must be empty: %q", got)
	}
	if got := parsePipxOutdated("{", "poetry"); got != "" {
		t.Errorf("invalid JSON must be empty: %q", got)
	}
}
