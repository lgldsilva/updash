package tui

import (
	"testing"

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
	if label := categoryLabel(summaries, model.CatMAS); label != "Mac App Store" {
		t.Fatalf("label = %q", label)
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
	wait := make(chan struct{})
	s.ElevWait = wait
	sess := elevate.NewSession()
	sess.SetPasswordless()

	done := make(chan struct{})
	go func() {
		<-wait
		close(done)
	}()

	cmd := s.HandlePasswordOK(sess, nil)
	if cmd == nil {
		t.Fatal("expected tick cmd")
	}
	<-done
	if s.ElevSession == nil || !s.ElevSession.Ready() {
		t.Fatal("session not stored")
	}
}

type errTest string

func (e errTest) Error() string { return string(e) }
