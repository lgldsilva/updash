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
			items, err := s.Scan(ctx, plat)
			summary := &model.SourceSummary{
				Category: s.Category(),
				Label:    s.Label(),
				Icon:     s.Icon(),
				Items:    items,
			}
			if err != nil {
				summary.ErrorCount = 1
			}
			for _, it := range items {
				summary.Total++
				switch it.Status {
				case model.StatusOutdated, model.StatusCleanCandidate:
					summary.Outdated++
				case model.StatusOK:
					summary.OK++
				case model.StatusError:
					summary.ErrorCount++
				}
			if it.Reclaimable != "" && it.Reclaimable != "0 versions" {
				if summary.Reclaimable != "" {
					summary.Reclaimable += " + " + it.Reclaimable
				} else {
					summary.Reclaimable = it.Reclaimable
				}
			}
			}
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
	var src []Source

	// macOS
	if plat.HasBrew {
		src = append(src, &BrewSource{})
	}
	if plat.HasMAS {
		src = append(src, &MASource{})
	}

	// Linux
	if plat.HasApt {
		src = append(src, &AptSource{})
	}
	if plat.HasPacman || plat.HasYay {
		src = append(src, &PacmanSource{})
	}
	if plat.HasFlatpak {
		src = append(src, &FlatpakSource{})
	}
	if plat.HasSnap {
		src = append(src, &SnapSource{})
	}

	// Windows
	if plat.HasWinget {
		src = append(src, &WingetSource{})
	}
	if plat.HasChoco {
		src = append(src, &ChocoSource{})
	}
	if plat.HasScoop {
		src = append(src, &ScoopSource{})
	}
	if plat.HasNpm {
		src = append(src, &NpmSource{})
	}
	if plat.HasPipx {
		src = append(src, &PipxSource{})
	}
	if plat.HasGo {
		src = append(src, &GoSource{})
	}
	if plat.HasRustup {
		src = append(src, &RustupSource{})
	}
	if plat.HasCargo {
		src = append(src, &CargoSource{})
	}
	if plat.HasDocker {
		src = append(src, &DockerSource{})
	}
	if plat.HasNvm {
		src = append(src, &NvmSource{})
	}
	if plat.HasOmz {
		src = append(src, &OmzSource{})
	}

	// AI agents — probe always
	src = append(src, &AgentSource{})
	src = append(src, &AIInfraSource{})

	// Cleanup sources
	if includeCleanup {
		if plat.HasBrew {
			src = append(src, &BrewCleanSource{})
		}
		if plat.HasApt {
			src = append(src, &AptCleanSource{})
		}
		if plat.HasSDKMAN {
			src = append(src, &SDKMANSource{})
		}
		if plat.HasDocker {
			src = append(src, &DockerCleanSource{})
		}
		if plat.HasGo {
			src = append(src, &GoCleanSource{})
		}
		if plat.HasNpm {
			src = append(src, &NpmCleanSource{})
		}
		if plat.HasSnap {
			src = append(src, &SnapCleanSource{})
		}

		// Windows cache cleanup
		if plat.OS == "windows" {
			src = append(src, &WindowsTempSource{})
		}

		home := os.Getenv("HOME")
		src = append(src, &VSCodeCleanSource{
			LabelName: "Antigravity Ext",
			ExtDir:    home + "/.antigravity/extensions",
		})
		src = append(src, &VSCodeCleanSource{
			LabelName: "Antigravity IDE Ext",
			ExtDir:    home + "/.antigravity-ide/extensions",
		})
	}

	return src
}
