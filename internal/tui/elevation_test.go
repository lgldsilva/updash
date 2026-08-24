package tui

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lgldsilva/updash/internal/elevate"
	"github.com/lgldsilva/updash/internal/model"
)

func TestCanDeferMASElevation(t *testing.T) {
	s := New()
	s.Platform = model.PlatformInfo{OS: "darwin"}
	items := []*model.Item{
		{Name: "foo", Category: model.CatBrew, Status: model.StatusOutdated},
		{Name: "WhatsApp", Category: model.CatMAS, Status: model.StatusOutdated},
	}
	if !s.canDeferMASElevation(items) {
		t.Fatal("should defer MAS when brew also updates")
	}

	onlyMAS := []*model.Item{{Name: "WhatsApp", Category: model.CatMAS}}
	if !s.canDeferMASElevation(onlyMAS) {
		t.Fatal("should defer for MAS-only batch")
	}

	mixed := []*model.Item{{Name: "pkg", Category: model.CatApt}}
	if s.canDeferMASElevation(mixed) {
		t.Fatal("apt should not defer")
	}
}

func TestCanDeferMASElevation_BrewPkgCaskForcesPrompt(t *testing.T) {
	// Brew PKG casks (Microsoft) sudo-prompt mid-run — they must be primed
	// up-front like apt, not deferred until the MAS batch.
	s := New()
	s.Platform = model.PlatformInfo{OS: "darwin"}
	items := []*model.Item{
		{Name: "microsoft-teams", Category: model.CatBrew, Status: model.StatusOutdated},
		{Name: "WhatsApp", Category: model.CatMAS, Status: model.StatusOutdated},
	}
	if s.canDeferMASElevation(items) {
		t.Fatal("brew PKG cask must force an up-front password prompt")
	}
}

func TestNeedsElevationPrompt_DeferredMAS(t *testing.T) {
	s := New()
	s.Platform = model.PlatformInfo{OS: "darwin"}
	s.PendingUpdateItems = []*model.Item{
		{Name: "foo", Category: model.CatBrew, Status: model.StatusOutdated},
		{Name: "WhatsApp", Category: model.CatMAS, Status: model.StatusOutdated},
	}
	if s.needsElevationPrompt() {
		t.Fatal("should not prompt before brew when MAS can defer")
	}
}

func TestGroupOutdatedByCategory(t *testing.T) {
	summaries := []*model.SourceSummary{
		{
			Category: model.CatBrew,
			Label:    "Homebrew",
			Items: []*model.Item{
				{Name: "a", Status: model.StatusOutdated},
				{Name: "b", Status: model.StatusOK},
			},
		},
		{
			Category: model.CatMAS,
			Label:    "Mac App Store",
			Items:    []*model.Item{{Name: "WhatsApp", Status: model.StatusOutdated}},
		},
	}
	selected := []*model.Item{summaries[0].Items[0], summaries[1].Items[0]}
	groups := groupOutdatedByCategory(summaries, selected)
	if len(groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(groups))
	}
	if groups[0].category != model.CatBrew || len(groups[0].items) != 1 {
		t.Fatalf("brew group: %+v", groups[0])
	}
	if label := groupOutdatedByCategory(summaries, copyItems(summaries[1].Items))[0].label(); label != "Mac App Store" {
		t.Fatalf("label = %q", label)
	}
}

func TestGroupOutdatedByCategory_CopiedItems(t *testing.T) {
	// startUpdateAll gives workers copies so they cannot race the renderer.
	// Grouping those copies by pointer identity against the live summaries used
	// to yield no batches, making Update All silently do nothing.
	summaries := []*model.SourceSummary{{
		Category: model.CatBrew,
		Label:    "Homebrew",
		Items: []*model.Item{{
			Name: "git", Category: model.CatBrew, Status: model.StatusOutdated,
		}},
	}}

	groups := groupOutdatedByCategory(summaries, copyItems(summaries[0].Items))
	if len(groups) != 1 || len(groups[0].items) != 1 {
		t.Fatalf("copied targets must produce one update group, got %+v", groups)
	}
}

func TestGroupOutdatedByCategory_BlocksAmbiguousIdentity(t *testing.T) {
	item := &model.Item{Name: "same", PackageID: "stable-id", Category: model.CatNpm, Status: model.StatusOutdated}
	summaries := []*model.SourceSummary{
		{Category: model.CatNpm, Label: "first", Items: []*model.Item{item}},
		{Category: model.CatNpm, Label: "second", Items: []*model.Item{{Name: "renamed", PackageID: "stable-id", Category: model.CatNpm, Status: model.StatusOutdated}}},
	}
	if groups := groupOutdatedByCategory(summaries, copyItems([]*model.Item{item})); len(groups) != 0 {
		t.Fatalf("ambiguous source identity must be blocked, got %+v", groups)
	}
}

func TestMasElevFailResults(t *testing.T) {
	items := []*model.Item{{Name: "WhatsApp"}}
	results := masElevFailResults(items, errTest("sudo expired"))
	if len(results) != 1 || results[0].Success || results[0].Error == "" {
		t.Fatalf("unexpected results: %+v", results)
	}
}

func TestHandlePasswordOK_MidOperation(t *testing.T) {
	s := New()
	wait := make(chan *elevate.Session, 1)
	s.ElevWait = wait
	sess := elevate.NewSession()
	sess.SetPasswordless()

	done := make(chan struct{})
	go func() {
		got := <-wait
		if got != sess {
			t.Errorf("unexpected session delivered: %v", got)
		}
		close(done)
	}()

	s.HandlePasswordOK(sess, nil)
	<-done
	if s.ElevSession == nil || !s.ElevSession.Ready() {
		t.Fatal("session not stored")
	}
	if s.ElevWait != nil {
		t.Fatal("ElevWait should be cleared after delivery")
	}
}

func TestCancelPassword_UnblocksWaitingWorkerWithNil(t *testing.T) {
	s := New()
	wait := make(chan *elevate.Session, 1)
	s.ElevWait = wait
	s.ConfirmCmd = func(p *tea.Program) tea.Cmd { return nil }

	got := make(chan *elevate.Session, 1)
	go func() { got <- <-wait }()

	s.CancelPassword()
	select {
	case sess := <-got:
		if sess != nil {
			t.Fatalf("cancel must deliver nil, got %v", sess)
		}
	case <-time.After(time.Second):
		t.Fatal("worker was not unblocked by cancel")
	}
	if s.ElevWait != nil {
		t.Fatal("ElevWait should be cleared after cancel")
	}
}

type errTest string

func (e errTest) Error() string { return string(e) }
