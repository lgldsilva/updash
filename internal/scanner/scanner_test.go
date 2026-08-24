package scanner

import (
	"context"
	"testing"

	"github.com/lgldsilva/updash/internal/model"
)

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

func TestRunAllFiltered_OnlyRunsMatchingCategory(t *testing.T) {
	plat := model.PlatformInfo{OS: "linux", HasNpm: true, HasPnpm: true}
	results := RunAllFiltered(t.Context(), plat, false, []model.Category{model.CatNpm})
	for _, result := range results {
		if result.Category != model.CatNpm {
			t.Fatalf("unfiltered source ran: %s", result.Category)
		}
	}
}

func TestCanonicalCategory_RoutesEmittedAndCleanupCategories(t *testing.T) {
	plat := model.PlatformInfo{OS: "linux", HasBrew: true, HasDocker: true}
	cases := map[string]model.Category{
		"gh-ext": model.CatGHExt,
		"BReW":   model.CatBrew,
		"docker": model.CatDocker,
	}
	for raw, want := range cases {
		got, ok := CanonicalCategory(plat, true, raw)
		if !ok || got != want {
			t.Fatalf("CanonicalCategory(%q)=(%q,%v), want (%q,true)", raw, got, ok, want)
		}
	}
}

func TestFilterSources_CleanupAliasSelectsOnlyPhysicalCleaner(t *testing.T) {
	sources := []Source{&BrewSource{}, &BrewCleanSource{}, &DockerCleanSource{}, &AIInfraSource{}}
	got := filterSources(sources, []model.Category{model.CatBrew}, true)
	if len(got) != 1 {
		t.Fatalf("got %d sources, want one", len(got))
	}
	if _, ok := got[0].(*BrewCleanSource); !ok {
		t.Fatalf("selected %T, want BrewCleanSource", got[0])
	}
	got = filterSources(sources, []model.Category{model.CatGHExt}, false)
	if len(got) != 1 {
		t.Fatalf("gh-ext got %d sources, want one", len(got))
	}
	if _, ok := got[0].(*AIInfraSource); !ok {
		t.Fatalf("selected %T, want AIInfraSource", got[0])
	}
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
		&OpenCodeSource{},
		&OmzSource{},
		&AgentSource{},
		&AIInfraSource{},
		&BrewCleanSource{},
		&AptCleanSource{},
		&DockerCleanSource{},
		&GoCleanSource{},
		&NpmCleanSource{},
		&SnapCleanSource{},
		&HomelabCleanSource{},
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

func TestEnabledSources_NoDuplicateCategories(t *testing.T) {
	// A pacman-capable host must register PacmanSource exactly once: a
	// duplicated entry double-scans and double-counts items in --check
	// --json (the TUI masks it via MergeSummary, the CLI does not).
	// The invariant is the (Category, Label) pair — the two VSCode clean
	// sources intentionally share a category with distinct labels.
	plat := model.PlatformInfo{
		OS:        "linux",
		Distro:    "arch",
		HasPacman: true,
		HasYay:    true,
		HasNpm:    true,
	}

	counts := map[string]int{}
	pacman := 0
	for _, s := range enabledSources(plat, true) {
		key := string(s.Category()) + "/" + s.Label()
		counts[key]++
		if s.Category() == model.CatPacman {
			pacman++
		}
	}
	for key, n := range counts {
		if n > 1 {
			t.Errorf("source %s registered %d times, expected 1", key, n)
		}
	}
	if pacman != 1 {
		t.Errorf("expected exactly one pacman source, got %d", pacman)
	}
}
