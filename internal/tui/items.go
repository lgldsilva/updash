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
	ctx       context.Context
	summaries []*model.SourceSummary
	sess      *elevate.Session
	platform  model.PlatformInfo
}

// snapshotEnv captures the worker environment. Call from the event loop
// before spawning the goroutine.
func (s *State) snapshotEnv() workerEnv {
	return workerEnv{
		ctx:       s.Ctx,
		summaries: s.Summaries,
		sess:      s.ElevSession,
		platform:  s.Platform,
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

// findLiveItem locates the live counterpart of a worker-mutated copy by
// (Category, Name) in the summaries.
func findLiveItem(summaries []*model.SourceSummary, cat model.Category, name string) *model.Item {
	for _, sum := range summaries {
		if sum.Category != cat {
			continue
		}
		for _, it := range sum.Items {
			if it.Name == name {
				return it
			}
		}
	}
	return nil
}

// itemNames returns the batch item names for start-of-batch messages.
func itemNames(items []*model.Item) []string {
	names := make([]string, len(items))
	for i, it := range items {
		names[i] = it.Name
	}
	return names
}

// ApplyUpdateResults copies worker-mutated fields (Status/Log/CurrentVer)
// from result items back onto the live summary items. Event loop only.
func (s *State) ApplyUpdateResults(results []*updater.Result) {
	for _, r := range results {
		if r.Item == nil {
			continue
		}
		live := findLiveItem(s.Summaries, r.Item.Category, r.Item.Name)
		if live == nil {
			continue
		}
		live.Status = r.Item.Status
		live.Log = r.Item.Log
		if r.Item.CurrentVer != "" {
			live.CurrentVer = r.Item.CurrentVer
		}
	}
}

// ApplyCleanResults copies worker-mutated fields (Status/Freed) from result
// items back onto the live cleanup items. Event loop only.
func (s *State) ApplyCleanResults(results []*cleaner.Result) {
	for _, r := range results {
		if r.Item == nil {
			continue
		}
		live := findLiveItem(s.CleanItems, r.Item.Category, r.Item.Name)
		if live == nil {
			continue
		}
		live.Status = r.Item.Status
		live.Freed = r.Item.Freed
	}
}

// MarkItemsUpdating flips live items to StatusUpdating when a batch starts,
// so the spinner state is visible even though workers run on copies.
// Event loop only.
func (s *State) MarkItemsUpdating(cat model.Category, names []string) {
	for _, name := range names {
		if it := findLiveItem(s.Summaries, cat, name); it != nil && it.Status == model.StatusOutdated {
			it.Status = model.StatusUpdating
		}
	}
}

// MarkItemCleaning flips one live cleanup item to StatusCleaning.
// Event loop only.
func (s *State) MarkItemCleaning(cat model.Category, name string) {
	if it := findLiveItem(s.CleanItems, cat, name); it != nil && it.Status == model.StatusCleanCandidate {
		it.Status = model.StatusCleaning
	}
}
