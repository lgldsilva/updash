// Package scanner detects outdated packages and cleanup candidates.
package scanner

import (
	"context"
	"os"
	"sort"
	"sync"

	"github.com/lgldsilva/updash/internal/model"
)

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
	sources := enabledSources(plat, includeCleanup)

	var mu sync.Mutex
	results := make([]*model.SourceSummary, 0, len(sources))
	var wg sync.WaitGroup

	for _, src := range sources {
		wg.Add(1)
		s := src
		go func() {
			defer wg.Done()
			summary := ScanSource(ctx, s, plat)
			mu.Lock()
			results = append(results, summary)
			mu.Unlock()
		}()
	}

	wg.Wait()

	sort.Slice(results, func(i, j int) bool {
		return results[i].Category < results[j].Category
	})

	return results
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
