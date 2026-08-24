package tui

import (
	"sync"
	"testing"

	"github.com/lgldsilva/updash/internal/cleaner"
	"github.com/lgldsilva/updash/internal/model"
	"github.com/lgldsilva/updash/internal/updater"
)

func seedSummaries(n int) (*State, []*model.Item) {
	s := New()
	items := make([]*model.Item, 0, n)
	summary := &model.SourceSummary{Category: model.CatNpm, Label: "npm"}
	for i := 0; i < n; i++ {
		it := &model.Item{
			Name:     "pkg-" + string(rune('a'+i%26)) + string(rune('0'+i/26)),
			Category: model.CatNpm,
			Status:   model.StatusOutdated,
		}
		items = append(items, it)
		summary.Items = append(summary.Items, it)
	}
	s.Summaries = []*model.SourceSummary{summary}
	return s, items
}

func TestCopyItems_Isolation(t *testing.T) {
	s, items := seedSummaries(3)
	copies := copyItems(items)
	copies[0].Status = model.StatusDone
	copies[0].Log = "mutated"
	if items[0].Status != model.StatusOutdated || items[0].Log != "" {
		t.Fatal("worker mutation leaked into live items")
	}
	if s.Summaries[0].Items[0].Status != model.StatusOutdated {
		t.Fatal("summary must render pre-batch state")
	}
}

func TestApplyUpdateResults(t *testing.T) {
	s, items := seedSummaries(2)
	copies := copyItems(items)
	copies[0].Status = model.StatusDone
	copies[0].Log = "upgraded"
	copies[0].CurrentVer = "2.0.0"
	copies[1].Status = model.StatusError

	s.ApplyUpdateResults([]*updater.Result{
		{Item: copies[0], Success: true},
		{Item: copies[1], Success: false},
	})

	if items[0].Status != model.StatusDone || items[0].Log != "upgraded" || items[0].CurrentVer != "2.0.0" {
		t.Fatalf("apply-back failed: %+v", items[0])
	}
	if items[1].Status != model.StatusError {
		t.Fatalf("error status not applied: %+v", items[1])
	}
}

func TestApplyUpdateResults_SurvivesRescan(t *testing.T) {
	// After a rescan swaps the summary, apply-back must still find the live
	// item by (Category, Name) — no disconnected pointers.
	s, items := seedSummaries(1)
	fresh := &model.Item{Name: items[0].Name, Category: model.CatNpm, Status: model.StatusOutdated}
	s.Summaries = MergeSummary(s.Summaries, &model.SourceSummary{
		Category: model.CatNpm, Label: "npm", Items: []*model.Item{fresh},
	})

	done := copyItems(items)[0]
	done.Status = model.StatusDone
	s.ApplyUpdateResults([]*updater.Result{{Item: done, Success: true}})

	if fresh.Status != model.StatusDone {
		t.Fatalf("apply-back must target post-rescan items, got %+v", fresh)
	}
}

func TestApplyUpdateResultsForSource_UsesStableIdentityAndBlocksCollisions(t *testing.T) {
	s := New()
	first := &model.Item{Name: "first name", PackageID: "pkg-id", Category: model.CatNpm, Status: model.StatusOutdated}
	second := &model.Item{Name: "second name", PackageID: "pkg-id", Category: model.CatNpm, Status: model.StatusOutdated}
	s.Summaries = []*model.SourceSummary{
		{Category: model.CatNpm, Label: "first", Items: []*model.Item{first}},
		{Category: model.CatNpm, Label: "second", Items: []*model.Item{second}},
	}

	result := &model.Item{Name: "renamed", PackageID: "pkg-id", Category: model.CatNpm, Status: model.StatusDone}
	if ambiguous := s.ApplyUpdateResultsForSource([]*updater.Result{{Item: result, Success: true}}, SourceIdentity{Category: model.CatNpm, Label: "first"}); ambiguous != 0 {
		t.Fatalf("source-qualified result unexpectedly ambiguous: %d", ambiguous)
	}
	if first.Status != model.StatusDone || second.Status != model.StatusOutdated {
		t.Fatalf("source-qualified result updated wrong row: first=%v second=%v", first.Status, second.Status)
	}

	second.Status = model.StatusOutdated
	first.Status = model.StatusOutdated
	if ambiguous := s.ApplyUpdateResultsForSource([]*updater.Result{{Item: result, Success: true}}, SourceIdentity{}); ambiguous != 1 {
		t.Fatalf("unqualified duplicate must be blocked, ambiguous=%d", ambiguous)
	}
	if first.Status != model.StatusOutdated || second.Status != model.StatusOutdated {
		t.Fatal("ambiguous result must not mutate either row")
	}
}

func TestApplyCleanResults(t *testing.T) {
	s := New()
	it := &model.Item{Name: "brew-cache", Category: model.CatCache, Status: model.StatusCleanCandidate}
	s.CleanItems = []*model.SourceSummary{{
		Category: model.CatCache, Label: "Homebrew Cache", Items: []*model.Item{it},
	}}
	done := &model.Item{Name: it.Name, Category: it.Category, Status: model.StatusCleaned, Freed: "13M"}

	s.ApplyCleanResults([]*cleaner.Result{{Item: done, Success: true, BytesFreed: 13 << 20}})

	if it.Status != model.StatusCleaned || it.Freed != "13M" {
		t.Fatalf("clean apply-back failed: %+v", it)
	}
}

func TestMarkItemsUpdating(t *testing.T) {
	s, items := seedSummaries(2)
	s.MarkItemsUpdating(SourceIdentity{Category: model.CatNpm, Label: "npm"}, []ItemIdentity{itemIdentity(items[0]), {Category: model.CatNpm, Name: "missing"}})
	if items[0].Status != model.StatusUpdating {
		t.Fatal("live item should be marked updating")
	}
	if items[1].Status != model.StatusOutdated {
		t.Fatal("untouched item must stay outdated")
	}
}

func TestResolveSource_SeparatesSourceAndOperationalCategories(t *testing.T) {
	item := &model.Item{Name: "gh-ext", PackageID: "owner/ext", Category: model.CatGHExt, Status: model.StatusOutdated}
	summaries := []*model.SourceSummary{{
		Category: model.CatAI,
		Label:    "AI tools",
		Items:    []*model.Item{item},
	}}

	source, ok := resolveSource(summaries, copyItems([]*model.Item{item})[0])
	if !ok || source.Category != model.CatAI || source.Label != "AI tools" {
		t.Fatalf("source identity must be independent from operational category: source=%+v ok=%v", source, ok)
	}
}

// TestWorkersVsRender_NoRace exercises the concurrency model under -race,
// mirroring production exactly: each round's batch gets its own copies
// (workers never touch live items or a previous round's copies), and a
// single "event loop" goroutine consumes result messages, applies them and
// renders. Pre-refactor, workers mutated the very items Render() reads.
func TestWorkersVsRender_NoRace(t *testing.T) {
	s, items := seedSummaries(24)
	// Pool copies: workers copy from this pool, never from live items.
	pool := copyItems(items)

	results := make(chan []*updater.Result, 4)
	var workers sync.WaitGroup
	for w := 0; w < 4; w++ {
		workers.Add(1)
		go func(chunk []*model.Item) {
			defer workers.Done()
			for round := 0; round < 25; round++ {
				batch := copyItems(chunk)
				for _, it := range batch {
					it.Status = model.StatusDone
					it.Log = "ok"
				}
				out := make([]*updater.Result, len(batch))
				for i, it := range batch {
					out[i] = &updater.Result{Item: it, Success: true}
				}
				// Handoff through the channel is the happens-before edge;
				// the worker never touches these copies again.
				results <- out
			}
		}(pool[w*6 : w*6+6])
	}

	go func() {
		workers.Wait()
		close(results)
	}()

	eventLoopDone := make(chan struct{})
	go func() {
		defer close(eventLoopDone)
		for batch := range results {
			s.ApplyUpdateResults(batch)
			_ = s.Render()
		}
	}()
	<-eventLoopDone
}
