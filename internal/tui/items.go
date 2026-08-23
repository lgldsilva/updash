package tui

import (
	"context"

	"github.com/lgldsilva/updash/internal/cleaner"
	"github.com/lgldsilva/updash/internal/elevate"
	"github.com/lgldsilva/updash/internal/model"
	"github.com/lgldsilva/updash/internal/updater"
)

// Concurrency model: background goroutines (update/clean workers) only ever
// touch item COPIES and a workerEnv snapshot; the Bubble Tea event loop is
// the single writer of live summary items, applying worker results back via
// the ...Results messages. This keeps Render() reads race-free without
// locks — the classic Bubble Tea pattern.

// workerEnv snapshots the State fields a background goroutine needs. The
// goroutine must never read State directly: the event loop mutates it
// (MergeSummary, cancelOperation, HandlePasswordOK).
type workerEnv struct {
	ctx        context.Context
	sess       *elevate.Session
	platform   model.PlatformInfo
	generation uint64
}

// snapshotEnv captures the worker environment. Call from the event loop
// before spawning the goroutine.
func (s *State) snapshotEnv() workerEnv {
	return workerEnv{
		ctx:        s.Ctx,
		sess:       s.ElevSession,
		platform:   s.Platform,
		generation: s.Generation,
	}
}

// ctxWithElev returns the worker context with a ready session attached.
func (env workerEnv) ctxWithElev() context.Context {
	if env.sess != nil && env.sess.Ready() {
		return elevate.WithSession(env.ctx, env.sess)
	}
	return env.ctx
}

// withSession returns a copy of the env with the session replaced (e.g.
// after a mid-run elevation grant).
func (env workerEnv) withSession(sess *elevate.Session) workerEnv {
	env.sess = sess
	return env
}

// copyItems returns copies for a background batch. model.Item has no
// reference fields, so a struct copy is deep enough: workers mutate the
// copies freely while the event loop renders the originals.
func copyItems(items []*model.Item) []*model.Item {
	out := make([]*model.Item, len(items))
	for i, it := range items {
		c := *it
		out[i] = &c
	}
	return out
}

// SourceIdentity identifies where an item was observed. Its category is the
// scanner source category, which can differ from the item's operational
// category (for example a gh extension reported by the AI source).
type SourceIdentity struct {
	Category model.Category
	Label    string
}

// ItemIdentity identifies the package-manager operation target. PackageID is
// preferred over Name so display-name changes cannot redirect an operation.
type ItemIdentity struct {
	Category  model.Category
	PackageID string
	Name      string
}

func itemIdentity(it *model.Item) ItemIdentity {
	if it == nil {
		return ItemIdentity{}
	}
	return ItemIdentity{Category: it.Category, PackageID: it.PackageID, Name: it.Name}
}

func itemIdentityFor(it *model.Item, defaultCategory model.Category) ItemIdentity {
	id := itemIdentity(it)
	if id.Category == "" {
		id.Category = defaultCategory
	}
	return id
}

func sameItemIdentity(left, right ItemIdentity) bool {
	if left.Category != right.Category {
		return false
	}
	if left.PackageID != "" || right.PackageID != "" {
		return left.PackageID != "" && left.PackageID == right.PackageID
	}
	return left.Name != "" && left.Name == right.Name
}

func itemIdentityKey(item ItemIdentity) string {
	if item.PackageID != "" {
		return string(item.Category) + "\x00id\x00" + item.PackageID
	}
	return string(item.Category) + "\x00name\x00" + item.Name
}

// findLiveItem locates one unambiguous live counterpart. PackageID is stable
// across display-name changes; name is only used when no source-specific ID
// exists. Multiple matches are deliberately rejected rather than updating the
// wrong package.
func findLiveItem(summaries []*model.SourceSummary, source SourceIdentity, item ItemIdentity) (*model.Item, bool) {
	var found *model.Item
	for _, sum := range summaries {
		if (source.Category != "" && sum.Category != source.Category) || (source.Label != "" && sum.Label != source.Label) {
			continue
		}
		for _, live := range sum.Items {
			if !sameItemIdentity(itemIdentityFor(live, sum.Category), item) {
				continue
			}
			if found != nil {
				return nil, false
			}
			found = live
		}
	}
	return found, found != nil
}

func itemIdentities(items []*model.Item) []ItemIdentity {
	identities := make([]ItemIdentity, len(items))
	for i, it := range items {
		identities[i] = itemIdentity(it)
	}
	return identities
}

// ApplyUpdateResults copies worker-mutated fields (Status/Log/CurrentVer)
// from result items back onto the live summary items. Event loop only.
func (s *State) ApplyUpdateResults(results []*updater.Result) {
	s.ApplyUpdateResultsForSource(results, SourceIdentity{})
}

// ApplyUpdateResultsForSource applies results only when their source/item
// identity resolves exactly once. It returns ambiguous results so the event
// loop can surface a useful warning instead of silently mutating a row.
func (s *State) ApplyUpdateResultsForSource(results []*updater.Result, source SourceIdentity) (ambiguous int) {
	for _, r := range results {
		if r.Item == nil {
			continue
		}
		live, ok := findLiveItem(s.Summaries, source, itemIdentity(r.Item))
		if !ok {
			ambiguous++
			continue
		}
		live.Status = r.Item.Status
		live.Log = r.Item.Log
		if r.Item.CurrentVer != "" {
			live.CurrentVer = r.Item.CurrentVer
		}
	}
	return ambiguous
}

// ApplyCleanResults copies worker-mutated fields (Status/Freed) from result
// items back onto the live cleanup items. Event loop only.
func (s *State) ApplyCleanResults(results []*cleaner.Result) {
	s.ApplyCleanResultsForSource(results, SourceIdentity{})
}

func (s *State) ApplyCleanResultsForSource(results []*cleaner.Result, source SourceIdentity) (ambiguous int) {
	for _, r := range results {
		if r.Item == nil {
			continue
		}
		live, ok := findLiveItem(s.CleanItems, source, itemIdentity(r.Item))
		if !ok {
			ambiguous++
			continue
		}
		live.Status = r.Item.Status
		live.Freed = r.Item.Freed
	}
	return ambiguous
}

// MarkItemsUpdating flips live items to StatusUpdating when a batch starts,
// so the spinner state is visible even though workers run on copies.
// Event loop only.
func (s *State) MarkItemsUpdating(source SourceIdentity, items []ItemIdentity) {
	for _, item := range items {
		if it, ok := findLiveItem(s.Summaries, source, item); ok && it.Status == model.StatusOutdated {
			it.Status = model.StatusUpdating
		}
	}
}

// MarkItemCleaning flips one live cleanup item to StatusCleaning.
// Event loop only.
func (s *State) MarkItemCleaning(source SourceIdentity, item ItemIdentity) {
	if it, ok := findLiveItem(s.CleanItems, source, item); ok && it.Status == model.StatusCleanCandidate {
		it.Status = model.StatusCleaning
	}
}
