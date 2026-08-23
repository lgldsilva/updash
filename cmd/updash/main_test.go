package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/lgldsilva/updash/internal/cli"
	"github.com/lgldsilva/updash/internal/model"
	"github.com/lgldsilva/updash/internal/tui"
)

func TestBubbleModel_IgnoresStaleAsyncMessages(t *testing.T) {
	state := tui.New()
	state.Generation = 4
	state.Updating = true
	state.LastSummary = "new operation"
	m := &bubbleModel{state: state}

	_, _ = m.onUpdateAllDone(tui.UpdateAllDoneMsg{Generation: 3, Success: 1, Total: 1})
	if !state.Updating || state.LastSummary != "new operation" {
		t.Fatalf("stale completion mutated the live operation: %+v", state)
	}
}

func TestExitOnErr_PreservesCLIExitClass(t *testing.T) {
	err := &cli.ExitError{Code: 2, Err: errors.New("untrusted scan")}
	if got := exitOnErr(err); got != 2 {
		t.Fatalf("exitOnErr() = %d, want 2", got)
	}
	if got := exitOnUpdateClean(0, 0, err); got != 2 {
		t.Fatalf("exitOnUpdateClean() = %d, want 2", got)
	}
}

func TestBubbleModel_ScanInfoIsNotLoggedAsSuccess(t *testing.T) {
	state := tui.New()
	state.Summaries = []*model.SourceSummary{{Category: model.CatAI, Label: "AI tools", Items: []*model.Item{{Name: "note", Category: model.CatAI, Status: model.StatusInfo}}}}
	m := &bubbleModel{state: state}
	_, _ = m.onScanFinished(tui.ScanFinishedMsg{})
	if len(state.Logs) == 0 || state.Logs[len(state.Logs)-1].Success {
		t.Fatalf("info-only scan must not emit a success log: %+v", state.Logs)
	}
	if !strings.Contains(strings.ToLower(state.LastSummary), "non-affirmative") {
		t.Fatalf("info-only scan summary must be non-affirmative: %q", state.LastSummary)
	}
}
