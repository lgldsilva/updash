package scanner

import (
	"context"
	"sync"

	"github.com/lgldsilva/updash/internal/model"
)

// NpmSource scans npm global packages across every discovered global prefix:
// the ambient one, plus well-known non-default prefixes (e.g. a legacy
// ~/.npm-global kept from before an nvm migration). Packages that only live
// in a non-default prefix are update targets pinned to that prefix; copies
// shadowed by an active-prefix package are informational only. Both halves
// run concurrently: each is a chain of npm invocations whose cold starts sum
// well past acceptable latency on busy hosts.
type NpmSource struct{}

func (s *NpmSource) Category() model.Category { return model.CatNpm }
func (s *NpmSource) Label() string            { return "npm (global)" }
func (s *NpmSource) Icon() string             { return "⬡" }

func (s *NpmSource) Scan(ctx context.Context, plat model.PlatformInfo) ([]*model.Item, error) {
	var (
		wg sync.WaitGroup

		activeEntries  map[string]npmOutdatedEntry
		activeVersions map[string]string
		activeErr      error
		legacy         []legacyScan
		legacyProblems []*model.Item
	)
	wg.Add(2)
	go func() {
		defer wg.Done()
		activeEntries, activeVersions, activeErr = scanDefaultNpmPrefix(ctx)
	}()
	go func() {
		defer wg.Done()
		legacy, legacyProblems = scanLegacyPrefixData(ctx)
	}()
	wg.Wait()
	if activeErr != nil {
		// No usable active inventory, so legacy copies cannot be shadow-joined.
		// Keep the active error and surface each legacy prefix as unverified
		// rather than dropping it or promoting it to an update target.
		return itemsWhenActiveScanFailed(activeErr, legacy, legacyProblems), nil
	}

	items := npmOutdatedItems(activeEntries, activeVersions, "")
	for _, sc := range legacy {
		items = append(items, buildLegacyItems(sc, activeVersions)...)
	}
	items = append(items, legacyProblems...)
	// Drop packages owned by another update path (e.g. opencode-ai is owned by
	// `opencode upgrade`); they must not appear as a second npm update target.
	items = filterProtectedNpm(items)
	if len(items) == 0 {
		return okOrOutdated(binNpm, model.CatNpm, items), nil
	}
	return items, nil
}
