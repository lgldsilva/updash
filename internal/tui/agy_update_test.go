package tui

import (
	"strings"
	"testing"

	"github.com/lgldsilva/updash/internal/model"
	"github.com/lgldsilva/updash/internal/scanner"
)

func TestCollectOutdatedItems_IncludesAgyOnUpdateAll(t *testing.T) {
	s := New()
	agy := &model.Item{
		Name: "Agy", Category: model.CatAgent, Status: model.StatusInfo,
		CurrentVer: "1.2.16", KeepPolicy: scanner.AgentUpdateCheckNote("Agy"),
	}
	grok := &model.Item{Name: "Grok", Category: model.CatAgent, Status: model.StatusInfo, CurrentVer: "1.0.0"}
	note := &model.Item{Name: "note", Category: model.CatAI, Status: model.StatusInfo, CurrentVer: "1"}
	btop := &model.Item{Name: "btop", Category: model.CatBrew, Status: model.StatusOutdated}
	s.Summaries = []*model.SourceSummary{
		{Category: model.CatAgent, Label: "Agents", Items: []*model.Item{agy, grok}},
		{Category: model.CatAI, Label: "AI", Items: []*model.Item{note}},
		{Category: model.CatBrew, Label: "Homebrew", Items: []*model.Item{btop}},
	}

	if IsManualOnlyItem(agy) {
		t.Fatal("agy check note must not be manual-only")
	}
	got := s.collectOutdatedItems(false)
	if len(got) != 2 || got[0] != agy || got[1] != btop {
		t.Fatalf("update-all = %v", namesOf(got))
	}

	agy.Selected = true
	selected := s.collectOutdatedItems(true)
	if len(selected) != 0 {
		t.Fatalf("selection must not pull an info row: %v", namesOf(selected))
	}
}

func namesOf(items []*model.Item) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.Name
	}
	return out
}

func TestRenderItem_AgyInfoShowsUpdateNote(t *testing.T) {
	s := New()
	s.Width = 160
	s.Height = 40
	item := &model.Item{
		Name: "Agy", Category: model.CatAgent, Status: model.StatusInfo,
		CurrentVer: "1.2.16", KeepPolicy: scanner.AgentUpdateCheckNote("Agy"),
	}
	out := s.renderItemStyled(item)
	if !strings.Contains(out, "checked on update") || !strings.Contains(out, "1.2.16") {
		t.Fatalf("row = %s", out)
	}
	s.DetailItem = item
	detail := s.renderDetail()
	if !strings.Contains(detail, "agy update") {
		t.Fatalf("detail = %s", detail)
	}
}
