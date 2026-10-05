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

func TestParseArgs_UpdateAndCleanIsAll(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		mode    string
		only    string
		dryRun  bool
		verbose bool
	}{
		{name: "update then clean", args: []string{"--update", "--clean"}, mode: "all", verbose: true},
		{name: "clean then update", args: []string{"--clean", "--update"}, mode: "all", verbose: true},
		{name: "update alone", args: []string{"--update"}, mode: "update", verbose: true},
		{name: "clean alone", args: []string{"--clean"}, mode: "clean", verbose: true},
		{name: "later check wins", args: []string{"--update", "--clean", "--check"}, mode: "check", verbose: true},
		{name: "later help wins", args: []string{"--update", "--clean", "--help"}, mode: "help", verbose: true},
		{
			name:    "filters survive",
			args:    []string{"--dry-run", "--only", "npm", "--clean", "--update", "--quiet"},
			mode:    "all",
			only:    "npm",
			dryRun:  true,
			verbose: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mode, cfg, err := parseArgs(tt.args)
			if err != nil {
				t.Fatalf("parseArgs() error = %v", err)
			}
			if mode != tt.mode {
				t.Fatalf("mode = %q, want %q", mode, tt.mode)
			}
			if cfg.Only != tt.only || cfg.DryRun != tt.dryRun || cfg.Verbose != tt.verbose {
				t.Fatalf("cfg = %+v, want only=%q dry=%v verbose=%v", cfg, tt.only, tt.dryRun, tt.verbose)
			}
		})
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
