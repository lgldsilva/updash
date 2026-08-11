package scanner

import (
	"context"
	"time"

	"github.com/lgldsilva/updash/internal/model"
)

const (
	defaultSourceTimeout = 45 * time.Second
	brewSourceTimeout    = 90 * time.Second
	aptSourceTimeout     = 20 * time.Second
	// AI Agents (~10 CLI probes, several Node.js cold-starts) and OpenCode
	// Plugins (full npm dependency-tree registry check) are cheap on an
	// idle machine but RunAll fires every source concurrently — on a
	// busy multi-service host (e.g. a homelab box also running backups,
	// a media server, queue workers) that contention alone can push a
	// normally-sub-5s probe well past 45s.
	agentSourceTimeout = 90 * time.Second
)

// EnabledSources returns scanners for the current platform (exported for TUI orchestration).
func EnabledSources(plat model.PlatformInfo, includeCleanup bool) []Source {
	return enabledSources(plat, includeCleanup)
}

// IsCleanupCategory reports whether a category belongs on the Cleanup tab.
func IsCleanupCategory(cat model.Category) bool {
	switch cat {
	case model.CatCache, model.CatSDKMAN, model.CatSDKClean,
		model.CatDockerClean, model.CatVSCodeClean, model.CatHomelabClean:
		return true
	default:
		return false
	}
}

// SourceTimeout returns a per-source scan budget.
func SourceTimeout(cat model.Category) time.Duration {
	switch cat {
	case model.CatBrew:
		return brewSourceTimeout
	case model.CatApt:
		return aptSourceTimeout
	case model.CatAgent, model.CatOpenCodePlugins:
		return agentSourceTimeout
	default:
		return defaultSourceTimeout
	}
}

// ScanSource runs one source with timeout and builds its summary.
func ScanSource(ctx context.Context, src Source, plat model.PlatformInfo) *model.SourceSummary {
	timeout := SourceTimeout(src.Category())
	scanCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	type scanResult struct {
		items []*model.Item
		err   error
	}
	ch := make(chan scanResult, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		items, err := src.Scan(scanCtx, plat)
		select {
		case ch <- scanResult{items: items, err: err}:
		case <-scanCtx.Done():
		}
	}()

	select {
	case r := <-ch:
		return buildSummary(src, r.items, r.err)
	case <-scanCtx.Done():
		// Cancel the context and wait a short grace period so the worker
		// goroutine observes cancellation before the next test swaps the
		// global exec mocks. This avoids data-race reports in tests that
		// exercise ScanSource with tight budgets.
		cancel()
		const grace = 250 * time.Millisecond
		graceCtx, graceCancel := context.WithTimeout(ctx, grace)
		defer graceCancel()
		select {
		case <-done:
		case <-graceCtx.Done():
		}
		return buildSummary(src, []*model.Item{{
			Name:       src.Label(),
			Category:   src.Category(),
			Status:     model.StatusError,
			CurrentVer: "scan timed out",
		}}, scanCtx.Err())
	}
}

func buildSummary(src Source, items []*model.Item, err error) *model.SourceSummary {
	summary := &model.SourceSummary{
		Category: src.Category(),
		Label:    src.Label(),
		Icon:     src.Icon(),
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
	return summary
}
