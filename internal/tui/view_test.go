package tui

import (
	"testing"

	"github.com/lgldsilva/updash/internal/model"
)

func TestHasCleanupItems(t *testing.T) {
	tests := []struct {
		name string
		sum  *model.SourceSummary
		want bool
	}{
		{
			name: "has clean candidate",
			sum: &model.SourceSummary{
				Items: []*model.Item{
					{Name: "go-cache", Status: model.StatusCleanCandidate},
				},
			},
			want: true,
		},
		{
			name: "has cleaning",
			sum: &model.SourceSummary{
				Items: []*model.Item{
					{Name: "go-cache", Status: model.StatusCleaning},
				},
			},
			want: true,
		},
		{
			name: "has cleaned",
			sum: &model.SourceSummary{
				Items: []*model.Item{
					{Name: "go-cache", Status: model.StatusCleaned},
				},
			},
			want: true,
		},
		{
			name: "only ok items",
			sum: &model.SourceSummary{
				Items: []*model.Item{
					{Name: "brew", Status: model.StatusOK},
				},
			},
			want: false,
		},
		{
			name: "empty items",
			sum:  &model.SourceSummary{Items: []*model.Item{}},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hasCleanupItems(tt.sum)
			if got != tt.want {
				t.Errorf("hasCleanupItems() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRenderLoading(t *testing.T) {
	s := New()
	output := s.renderLoading()
	if output == "" {
		t.Error("renderLoading() returned empty string")
	}
	if len(output) < 10 {
		t.Errorf("renderLoading() too short: %d chars", len(output))
	}
}

func TestRenderTitle(t *testing.T) {
	s := New()
	output := s.renderTitle()
	if output == "" {
		t.Error("renderTitle() returned empty string")
	}
}

func TestRenderTabs(t *testing.T) {
	s := New()
	s.Ready = true

	// Updates tab - no outdated items
	s.Summaries = []*model.SourceSummary{}
	output := s.renderTabs()
	if !contains(output, "Updates") {
		t.Errorf("renderTabs() should include Updates tab")
	}
	if !contains(output, "Cleanup") {
		t.Errorf("renderTabs() should include Cleanup tab")
	}
	if !contains(output, "Logs") {
		t.Errorf("renderTabs() should include Logs tab")
	}

	// Cleanup tab - with count
	s.ActiveTab = model.TabCleanup
	s.CleanItems = []*model.SourceSummary{
		{Category: model.CatCache, Items: []*model.Item{
			{Name: "go-cache", Status: model.StatusCleanCandidate},
		}, Outdated: 1},
	}
	output = s.renderTabs()
	if !contains(output, "Cleanup (1)") {
		t.Errorf("renderTabs() should show count on Cleanup tab")
	}
}

func TestRenderProgressBar(t *testing.T) {
	s := New()

	tests := []struct {
		name  string
		total int
		done  int
	}{
		{"full", 10, 10},
		{"half", 10, 5},
		{"empty", 10, 0},
		{"zero total", 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bar := s.renderProgressBar(tt.total, tt.done)
			if bar == "" {
				t.Error("renderProgressBar() returned empty")
			}
		})
	}
}

func TestRenderFooter(t *testing.T) {
	s := New()
	s.Ready = true

	// Updates tab footer
	s.ActiveTab = model.TabUpdates
	updatesFooter := s.renderFooter()
	if !contains(updatesFooter, "U") || !contains(updatesFooter, "A") {
		t.Errorf("updates footer missing U/A keys: %s", updatesFooter)
	}

	// Cleanup tab footer
	s.ActiveTab = model.TabCleanup
	cleanupFooter := s.renderFooter()
	if !contains(cleanupFooter, "C") {
		t.Errorf("cleanup footer missing C key: %s", cleanupFooter)
	}

	// Logs tab footer
	s.ActiveTab = model.TabLogs
	logsFooter := s.renderFooter()
	if logsFooter == "" {
		t.Error("logs footer empty")
	}
}

// TestFlattenItemsEdgeCases tests edge cases in item flattening.
func TestFlattenItemsEdgeCases(t *testing.T) {
	s := New()
	s.Ready = true

	// Empty lists
	if items := s.FlattenItems(); len(items) != 0 {
		t.Errorf("FlattenItems() on empty summaries = %d items", len(items))
	}
	if items := s.FlattenCleanItems(); len(items) != 0 {
		t.Errorf("FlattenCleanItems() on empty summaries = %d items", len(items))
	}

	// With items
	s.Summaries = []*model.SourceSummary{
		{Category: model.CatBrew, Items: []*model.Item{
			{Name: "pkg1"}, {Name: "pkg2"},
		}},
	}
	if items := s.FlattenItems(); len(items) != 2 {
		t.Errorf("FlattenItems() = %d items, want 2", len(items))
	}
}

func TestState_LogCap(t *testing.T) {
	s := New()
	for i := 0; i < 150; i++ {
		s.AddLog("entry", true)
	}
	if len(s.Logs) > 100 {
		t.Errorf("logs exceeded 100 cap: %d", len(s.Logs))
	}
	if s.Logs[len(s.Logs)-1].Message != "entry" {
		t.Error("last log entry should be 'entry'")
	}
}

func TestState_HandleKey_ConfirmDialog(t *testing.T) {
	s := New()
	s.ShowConfirm = true
	s.ConfirmMsg = "Test?"

	// Y should confirm
	if action := s.HandleKey("y"); action != KeyConfirm {
		t.Errorf("HandleKey('y') in confirm = %v, want KeyConfirm", action)
	}
	if s.ShowConfirm {
		t.Error("ShowConfirm should be false after confirming")
	}

	// N should cancel
	s.ShowConfirm = true
	if action := s.HandleKey("n"); action != KeyCancel {
		t.Errorf("HandleKey('n') in confirm = %v, want KeyCancel", action)
	}
	if s.ShowConfirm {
		t.Error("ShowConfirm should be false after cancelling")
	}
}

func TestState_RunUpdateSelected_NoSelection(t *testing.T) {
	s := New()
	s.Ready = true
	s.Summaries = []*model.SourceSummary{
		{Category: model.CatBrew, Items: []*model.Item{
			{Name: "pkg1", Status: model.StatusOutdated},
		}},
	}

	// No items selected, should not panic
	s.runUpdateSelected()
	if len(s.Logs) > 0 && !s.Logs[len(s.Logs)-1].Success {
		// Expected: "No items selected for update"
	}
}

// contains checks if a string contains a substring.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && containsStr(s, substr)
}

// containsStr is a simple contains helper.
func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
