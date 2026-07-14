package tui

import (
	"testing"

	"github.com/lgldsilva/updash/internal/model"
)

func TestMergeSummary_InsertAndReplace(t *testing.T) {
	brew := &model.SourceSummary{Category: model.CatBrew, Label: "Homebrew", Total: 1}
	list := MergeSummary(nil, brew)
	if len(list) != 1 {
		t.Fatalf("len = %d", len(list))
	}

	updated := &model.SourceSummary{Category: model.CatBrew, Label: "Homebrew", Total: 5}
	list = MergeSummary(list, updated)
	if len(list) != 1 || list[0].Total != 5 {
		t.Fatalf("replace failed: %+v", list)
	}

	mas := &model.SourceSummary{Category: model.CatMAS, Label: "Mac App Store", Total: 2}
	list = MergeSummary(list, mas)
	if len(list) != 2 {
		t.Fatalf("insert failed: len=%d", len(list))
	}
}

func TestStartScan_Idempotent(t *testing.T) {
	s := New()
	if s.startScan() != nil {
		t.Fatal("startScan without program should return nil")
	}
	s.Program = nil
	s.Scanning = true
	if s.startScan() != nil {
		t.Fatal("startScan while scanning should return nil")
	}
}