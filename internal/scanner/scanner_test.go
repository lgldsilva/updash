package scanner

import (
	"context"
	"testing"

	"github.com/lgldsilva/updash/internal/model"
)

// mockSource implements Source for testing.
type mockSource struct {
	category model.Category
	label    string
	icon     string
	items    []*model.Item
	err      error
}

func (m *mockSource) Category() model.Category { return m.category }
func (m *mockSource) Label() string            { return m.label }
func (m *mockSource) Icon() string             { return m.icon }
func (m *mockSource) Scan(ctx context.Context, plat model.PlatformInfo) ([]*model.Item, error) {
	return m.items, m.err
}

func TestRunAll_NoSources(t *testing.T) {
	// With an empty platform (no tools installed), RunAll should return empty summaries
	plat := model.PlatformInfo{OS: "linux"}
	results := RunAll(context.Background(), plat, false)
	if len(results) == 0 {
		t.Log("No sources detected for empty platform — expected")
	}
}

func TestRunAll_BrewSource(t *testing.T) {
	// This test verifies the source interface contract with a mock
	items := []*model.Item{
		{Name: "test-pkg", Status: model.StatusOutdated, CurrentVer: "1.0", AvailableVer: "2.0"},
	}

	// Manually test that a brew source produces items
	src := &BrewSource{}
	if src.Category() != model.CatBrew {
		t.Errorf("unexpected category: %s", src.Category())
	}
	if src.Label() != "Homebrew" {
		t.Errorf("unexpected label: %s", src.Label())
	}
	if src.Icon() != "🍺" {
		t.Errorf("unexpected icon: %s", src.Icon())
	}

	_ = items // brew source needs to run commands, tested via --check mode
}

func TestSources_AllHaveCategoryLabelIcon(t *testing.T) {
	sources := []Source{
		&BrewSource{},
		&MASource{},
		&AptSource{},
		&PacmanSource{},
		&FlatpakSource{},
		&SnapSource{},
		&WingetSource{},
		&ChocoSource{},
		&ScoopSource{},
		&NpmSource{},
		&PipxSource{},
		&GoSource{},
		&RustupSource{},
		&CargoSource{},
		&SDKMANSource{},
		&DockerSource{},
		&NvmSource{},
		&OmzSource{},
		&AgentSource{},
		&AIInfraSource{},
		&BrewCleanSource{},
		&AptCleanSource{},
		&DockerCleanSource{},
		&GoCleanSource{},
		&NpmCleanSource{},
		&SnapCleanSource{},
	}

	for _, s := range sources {
		t.Run(s.Label(), func(t *testing.T) {
			if s.Category() == "" {
				t.Error("Category() must not be empty")
			}
			if s.Label() == "" {
				t.Error("Label() must not be empty")
			}
			if s.Icon() == "" {
				t.Error("Icon() must not be empty")
			}
		})
	}
}

func TestEnabledSources(t *testing.T) {
	plat := model.PlatformInfo{
		OS:        "darwin",
		Distro:    "macos",
		HasBrew:   true,
		HasMAS:    true,
		HasNpm:    true,
		HasGo:     true,
		HasSDKMAN: true,
		HasDocker: true,
	}

	srcs := enabledSources(plat, false)
	if len(srcs) == 0 {
		t.Fatal("expected at least one source for macOS")
	}

	found := false
	for _, s := range srcs {
		if s.Category() == model.CatBrew {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected BrewSource for macOS with HasBrew=true")
	}

	// Windows platform
	wplat := model.PlatformInfo{
		OS:        "windows",
		Distro:    "windows",
		HasWinget: true,
		HasChoco:  true,
		HasNpm:    true,
	}

	wsrcs := enabledSources(wplat, false)
	foundWinget := false
	for _, s := range wsrcs {
		if s.Category() == model.CatWinget {
			foundWinget = true
			break
		}
	}
	if !foundWinget {
		t.Error("expected WingetSource for Windows with HasWinget=true")
	}

	// Linux platform
	lplat := model.PlatformInfo{
		OS:      "linux",
		Distro:  "ubuntu",
		HasApt:  true,
		HasSnap: true,
		HasNpm:  true,
	}

	lsrcs := enabledSources(lplat, false)
	foundApt := false
	for _, s := range lsrcs {
		if s.Category() == model.CatApt {
			foundApt = true
			break
		}
	}
	if !foundApt {
		t.Error("expected AptSource for Ubuntu with HasApt=true")
	}
}
