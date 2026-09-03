// Package scanner detects outdated packages and cleanup candidates.
package scanner

import (
	"context"
	"os"
	"sort"
	"strings"
	"sync"

	"github.com/lgldsilva/updash/internal/model"
)

const maxConcurrentSources = 6

// A Source scans one category of packages.
type Source interface {
	// Category returns the category key.
	Category() model.Category
	// Label returns the human-readable name.
	Label() string
	// Icon returns a single-character icon.
	Icon() string
	// Scan probes the system and returns items.
	Scan(ctx context.Context, plat model.PlatformInfo) ([]*model.Item, error)
}

// RunAll scans every applicable source in parallel and returns summaries.
func RunAll(ctx context.Context, plat model.PlatformInfo, includeCleanup bool) []*model.SourceSummary {
	return RunAllFiltered(ctx, plat, includeCleanup, nil)
}

// RunAllFiltered scans only selected source categories. Empty categories keeps
// RunAll's historical behavior, while the worker limit avoids an unbounded
// burst of external commands on a full scan.
func RunAllFiltered(ctx context.Context, plat model.PlatformInfo, includeCleanup bool, categories []model.Category) []*model.SourceSummary {
	return runAllFiltered(ctx, plat, includeCleanup, categories, false)
}

// RunAllFilteredForCleanup restricts logical aliases such as "brew" and
// "docker" to their cleanup sources, instead of also launching their update
// scanners. It is used by --clean --only.
func RunAllFilteredForCleanup(ctx context.Context, plat model.PlatformInfo, categories []model.Category) []*model.SourceSummary {
	return runAllFiltered(ctx, plat, true, categories, true)
}

func runAllFiltered(ctx context.Context, plat model.PlatformInfo, includeCleanup bool, categories []model.Category, cleanupOnly bool) []*model.SourceSummary {
	sources := filterSources(enabledSources(plat, includeCleanup), categories, cleanupOnly)

	var mu sync.Mutex
	results := make([]*model.SourceSummary, 0, len(sources))
	var wg sync.WaitGroup
	jobs := make(chan Source)
	workers := maxConcurrentSources
	if len(sources) < workers {
		workers = len(sources)
	}
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for s := range jobs {
				summary := ScanSource(ctx, s, plat)
				mu.Lock()
				results = append(results, summary)
				mu.Unlock()
			}
		}()
	}

	for _, src := range sources {
		jobs <- src
	}
	close(jobs)

	wg.Wait()

	sort.Slice(results, func(i, j int) bool {
		return results[i].Category < results[j].Category
	})

	return results
}

func filterSources(sources []Source, categories []model.Category, cleanupOnly bool) []Source {
	if len(categories) == 0 {
		return sources
	}
	want := make(map[model.Category]struct{}, len(categories))
	for _, cat := range categories {
		want[cat] = struct{}{}
	}
	out := make([]Source, 0, len(categories))
	for _, src := range sources {
		if sourceMatchesSelection(src, want, cleanupOnly) {
			out = append(out, src)
		}
	}
	return out
}

func sourceMatchesSelection(src Source, want map[model.Category]struct{}, cleanupOnly bool) bool {
	if cleanupOnly {
		switch src.(type) {
		case *BrewCleanSource:
			_, ok := want[model.CatBrew]
			return ok
		case *DockerCleanSource:
			_, ok := want[model.CatDocker]
			return ok
		}
		if !IsCleanupCategory(src.Category()) {
			return false
		}
		_, ok := want[src.Category()]
		return ok
	}
	if _, ok := want[src.Category()]; ok {
		return true
	}
	if _, ok := want[model.CatGHExt]; ok {
		_, isAI := src.(*AIInfraSource)
		return isAI
	}
	return false
}

// CanonicalCategory resolves an exact case-insensitive source category for
// this host. Labels and package names are deliberately not aliases.
func CanonicalCategory(plat model.PlatformInfo, includeCleanup bool, raw string) (model.Category, bool) {
	want := model.Category(strings.ToLower(strings.TrimSpace(raw)))
	if want == model.CatGHExt {
		for _, src := range enabledSources(plat, includeCleanup) {
			if _, ok := src.(*AIInfraSource); ok {
				return want, true
			}
		}
	}
	if includeCleanup && (want == model.CatBrew || want == model.CatDocker) {
		for _, src := range enabledSources(plat, includeCleanup) {
			if want == model.CatBrew {
				if _, ok := src.(*BrewCleanSource); ok {
					return want, true
				}
			}
			if want == model.CatDocker {
				if _, ok := src.(*DockerCleanSource); ok {
					return want, true
				}
			}
		}
	}
	for _, src := range enabledSources(plat, includeCleanup) {
		if strings.EqualFold(string(src.Category()), string(want)) {
			return src.Category(), true
		}
	}
	return "", false
}

// enabledSources returns scanners for the current platform.
func enabledSources(plat model.PlatformInfo, includeCleanup bool) []Source {
	src := appendPlatformSources(nil, plat)
	src = appendLanguageSources(src, plat)
	src = append(src, &AgentSource{}, &AIInfraSource{})
	if includeCleanup {
		src = appendCleanupSources(src, plat)
	}
	return src
}

func appendPlatformSources(src []Source, plat model.PlatformInfo) []Source {
	type cond struct {
		ok  bool
		src Source
	}
	for _, c := range []cond{
		{plat.HasBrew, &BrewSource{}},
		{plat.HasMAS, &MASource{}},
		{plat.HasApt, &AptSource{}},
		{plat.HasDnf || plat.HasYum, &RpmSource{}},
		{plat.HasZypper, &ZypperSource{}},
		{plat.HasApk, &ApkSource{}},
		{plat.HasPacman || plat.HasYay, &PacmanSource{}},
		{plat.HasFlatpak, &FlatpakSource{}},
		{plat.HasSnap, &SnapSource{}},
		{plat.HasWinget, &WingetSource{}},
		{plat.HasChoco, &ChocoSource{}},
		{plat.HasScoop, &ScoopSource{}},
	} {
		if c.ok {
			src = append(src, c.src)
		}
	}
	return src
}

func appendLanguageSources(src []Source, plat model.PlatformInfo) []Source {
	type cond struct {
		ok  bool
		src Source
	}
	for _, c := range []cond{
		{plat.HasNpm, &NpmSource{}},
		{plat.HasPnpm, &PnpmSource{}},
		{plat.HasBun, &BunSource{}},
		{plat.HasPipx, &PipxSource{}},
		{plat.HasGo, &GoSource{}},
		{plat.HasRustup, &RustupSource{}},
		{plat.HasCargo, &CargoSource{}},
		{plat.HasDocker, &DockerSource{}},
		{plat.HasNvm, &NvmSource{}},
		{plat.HasOpenCode && plat.HasNpm, &OpenCodeSource{}},
		{plat.HasOmz, &OmzSource{}},
	} {
		if c.ok {
			src = append(src, c.src)
		}
	}
	return src
}

func appendCleanupSources(src []Source, plat model.PlatformInfo) []Source {
	type cond struct {
		ok  bool
		src Source
	}
	for _, c := range []cond{
		{plat.HasBrew, &BrewCleanSource{}},
		{plat.HasApt, &AptCleanSource{}},
		{plat.HasSDKMAN, &SDKMANSource{}},
		{plat.HasDocker, &DockerCleanSource{}},
		{plat.HasGo, &GoCleanSource{}},
		{plat.HasNpm, &NpmCleanSource{}},
		{plat.HasSnap, &SnapCleanSource{}},
		{plat.HasPacman || plat.HasYay, &PacmanCleanSource{}},
		{plat.OS == "windows", &WindowsTempSource{}},
	} {
		if c.ok {
			src = append(src, c.src)
		}
	}
	// Homelab retention cleanups (logs, maven/gradle, AI outputs, disk pressure).
	src = append(src, &HomelabCleanSource{})
	home := os.Getenv("HOME")
	src = append(src,
		&VSCodeCleanSource{LabelName: "Antigravity Ext", ExtDir: home + "/.antigravity/extensions"},
		&VSCodeCleanSource{LabelName: "Antigravity IDE Ext", ExtDir: home + "/.antigravity-ide/extensions"},
	)
	return src
}
