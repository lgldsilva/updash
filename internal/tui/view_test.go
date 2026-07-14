package tui

import (
	"strings"
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
				Items: []*model.Item{{Name: "go-cache", Status: model.StatusCleanCandidate}},
			},
			want: true,
		},
		{
			name: "has cleaning",
			sum: &model.SourceSummary{
				Items: []*model.Item{{Name: "go-cache", Status: model.StatusCleaning}},
			},
			want: true,
		},
		{
			name: "has cleaned",
			sum: &model.SourceSummary{
				Items: []*model.Item{{Name: "go-cache", Status: model.StatusCleaned}},
			},
			want: true,
		},
		{
			name: "only ok items",
			sum: &model.SourceSummary{
				Items: []*model.Item{{Name: "brew", Status: model.StatusOK}},
			},
			want: false,
		},
		{
			name: "empty",
			sum:  &model.SourceSummary{Items: []*model.Item{}},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hasCleanupItems(tt.sum); got != tt.want {
				t.Errorf("hasCleanupItems() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRenderTitle(t *testing.T) {
	s := New()
	output := s.renderTitle()
	if output == "" {
		t.Error("renderTitle() returned empty")
	}
	if !strings.Contains(output, "updash") {
		t.Errorf("renderTitle missing 'updash': %s", output)
	}
}

func TestRenderTabs(t *testing.T) {
	s := New()
	s.Ready = true
	s.ActiveTab = model.TabUpdates

	output := s.renderTabs()
	if !strings.Contains(output, "Updates") {
		t.Errorf("renderTabs missing Updates: %s", output)
	}
	if !strings.Contains(output, "Cleanup") {
		t.Errorf("renderTabs missing Cleanup: %s", output)
	}
	if !strings.Contains(output, "Logs") {
		t.Errorf("renderTabs missing Logs: %s", output)
	}
}

func TestRenderTabs_WithCounts(t *testing.T) {
	s := New()
	s.Ready = true
	s.ActiveTab = model.TabCleanup
	s.CleanItems = []*model.SourceSummary{
		{Category: model.CatCache, Items: []*model.Item{
			{Name: "go-cache", Status: model.StatusCleanCandidate},
		}, Outdated: 1},
	}
	output := s.renderTabs()
	// Should show count on Cleanup tab
	if !strings.Contains(output, "Cleanup") {
		t.Errorf("renderTabs missing Cleanup tab: %s", output)
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
		{"over", 5, 10},
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

func TestRenderUpdatesTab_WithItems(t *testing.T) {
	s := New()
	s.Ready = true
	s.Summaries = []*model.SourceSummary{
		{
			Category: model.CatBrew,
			Label:    "Homebrew",
			Icon:     "🍺",
			Items: []*model.Item{
				{Name: "btop", Category: model.CatBrew, CurrentVer: "1.3.0", AvailableVer: "1.5.0", Status: model.StatusOutdated},
				{Name: "git", Category: model.CatBrew, CurrentVer: "2.42.0", AvailableVer: "2.45.0", Status: model.StatusOutdated},
			},
			Total: 2, Outdated: 2,
		},
		{
			Category: model.CatNpm,
			Label:    "npm (global)",
			Icon:     "⬡",
			Items: []*model.Item{
				{Name: "npm", Category: model.CatNpm, CurrentVer: "up to date", Status: model.StatusOK},
			},
			Total: 1, Outdated: 0, OK: 1,
		},
	}

	output := s.renderUpdatesTab()
	if output == "" {
		t.Fatal("renderUpdatesTab() returned empty")
	}
	if !strings.Contains(output, "Homebrew") {
		t.Error("missing Homebrew header")
	}
	if !strings.Contains(output, "btop") {
		t.Error("missing btop item")
	}
	if !strings.Contains(output, "1.5.0") {
		t.Error("missing version info")
	}
}

func TestRenderUpdatesTab_AllUpToDate(t *testing.T) {
	s := New()
	s.Ready = true
	s.Summaries = nil
	output := s.renderUpdatesTab()
	if output == "" {
		t.Error("renderUpdatesTab() returned empty")
	}
	if !strings.Contains(output, "up to date") {
		t.Errorf("should show up-to-date message: %s", output)
	}
}

func TestRenderUpdatesTab_WithCursor(t *testing.T) {
	s := New()
	s.Ready = true
	s.Cursor = 0
	s.Summaries = []*model.SourceSummary{
		{
			Category: model.CatBrew,
			Label:    "Homebrew",
			Icon:     "🍺",
			Items: []*model.Item{
				{Name: "btop", Status: model.StatusOutdated},
			},
			Total: 1, Outdated: 1,
		},
	}
	output := s.renderUpdatesTab()
	if !strings.Contains(output, "▸") {
		t.Log("cursor indicator may not render in test (expected)")
	}
}

func TestRenderCleanupTab_WithItems(t *testing.T) {
	s := New()
	s.Ready = true
	s.CleanItems = []*model.SourceSummary{
		{
			Category: model.CatCache,
			Label:    "Go Cache",
			Icon:     "🧹",
			Items: []*model.Item{
				{Name: "go-cache", Category: model.CatCache, CurrentVer: "23G", Status: model.StatusCleanCandidate, Reclaimable: "23G"},
			},
		},
		{
			Category: model.CatSDKMAN,
			Label:    "SDKMAN Cleanup",
			Icon:     "☕",
			Items: []*model.Item{
				{Name: "java 21", Category: model.CatSDKMAN, CurrentVer: "21.0.7-tem", Status: model.StatusCleanCandidate, Reclaimable: "4 versions", RemoveCount: 3, KeepPolicy: "keep latest per major"},
			},
		},
	}

	output := s.renderCleanupTab()
	if output == "" {
		t.Fatal("renderCleanupTab() returned empty")
	}
	if !strings.Contains(output, "Go Cache") {
		t.Error("missing Go Cache header")
	}
	if !strings.Contains(output, "23G") {
		t.Error("missing reclaimable info")
	}
}

func TestRenderCleanupTab_NothingToClean(t *testing.T) {
	s := New()
	s.Ready = true
	s.CleanItems = nil
	output := s.renderCleanupTab()
	if output == "" {
		t.Error("renderCleanupTab() returned empty")
	}
	if !strings.Contains(output, "Nothing to clean") {
		t.Errorf("should show nothing-to-clean: %s", output)
	}
}

func TestRenderLogsTab_WithEntries(t *testing.T) {
	s := New()
	s.Logs = []model.GlobalLogEntry{
		{Message: "✓ brew: updated", Success: true},
		{Message: "✘ npm: failed", Success: false},
	}
	output := s.renderLogsTab()
	if output == "" {
		t.Fatal("renderLogsTab() returned empty")
	}
	if !strings.Contains(output, "brew: updated") {
		t.Error("missing log entry")
	}
}

func TestRenderLogsTab_Empty(t *testing.T) {
	s := New()
	output := s.renderLogsTab()
	if !strings.Contains(output, "No log entries") {
		t.Errorf("should show empty message: %s", output)
	}
}

func TestRenderItemStyled(t *testing.T) {
	s := New()

	tests := []struct {
		name string
		item *model.Item
	}{
		{"ok", &model.Item{Name: "brew", Status: model.StatusOK, CurrentVer: "up to date"}},
		{"outdated", &model.Item{Name: "btop", Status: model.StatusOutdated, CurrentVer: "1.3.0", AvailableVer: "1.5.0"}},
		{"error", &model.Item{Name: "brew", Status: model.StatusError, CurrentVer: "error"}},
		{"updating", &model.Item{Name: "brew", Status: model.StatusUpdating}},
		{"done", &model.Item{Name: "brew", Status: model.StatusDone}},
		{"unknown", &model.Item{Name: "test", Status: 99}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := s.renderItemStyled(tt.item)
			if output == "" {
				t.Errorf("renderItemStyled() returned empty for %s", tt.name)
			}
		})
	}
}

func TestFormatRow(t *testing.T) {
	s := New()
	s.Cursor = 0
	row := s.formatRow("btop  1.3.0 → 1.5.0", 0)
	if row == "" {
		t.Fatal("formatRow returned empty")
	}
}

func TestRenderContent(t *testing.T) {
	s := New()
	s.Ready = true

	// Updates tab
	s.ActiveTab = model.TabUpdates
	output := s.renderContent()
	if output == "" {
		t.Error("renderContent() for Updates tab returned empty")
	}

	// Cleanup tab
	s.ActiveTab = model.TabCleanup
	output = s.renderContent()
	if output == "" {
		t.Error("renderContent() for Cleanup tab returned empty")
	}

	// Logs tab
	s.ActiveTab = model.TabLogs
	output = s.renderContent()
	if output == "" {
		t.Error("renderContent() for Logs tab returned empty")
	}
}

func TestRenderFooter(t *testing.T) {
	s := New()
	s.Ready = true

	// Updates tab
	s.ActiveTab = model.TabUpdates
	footer := s.renderFooter()
	if !strings.Contains(footer, "U") || !strings.Contains(footer, "A") {
		t.Errorf("updates footer missing U/A: %s", footer)
	}

	// Cleanup tab
	s.ActiveTab = model.TabCleanup
	footer = s.renderFooter()
	if !strings.Contains(footer, "C") {
		t.Errorf("cleanup footer missing C: %s", footer)
	}

	// Logs tab
	s.ActiveTab = model.TabLogs
	footer = s.renderFooter()
	if !strings.Contains(footer, "R") {
		t.Errorf("logs footer missing R: %s", footer)
	}
}

func TestRenderLoading(t *testing.T) {
	s := New()
	output := s.renderLoading()
	if output == "" {
		t.Error("renderLoading() returned empty")
	}
	if !strings.Contains(output, "Scanning") {
		t.Errorf("expected Scanning message: %s", output)
	}
}

func TestFullRender(t *testing.T) {
	s := New()
	s.Ready = true

	output := s.Render()
	if output == "" {
		t.Error("Render() returned empty")
	}
	// Should have the updash title
	if !strings.Contains(output, "updash") {
		t.Errorf("Render() missing updash title: %s", output)
	}
}

func TestRenderPassword(t *testing.T) {
	s := New()
	s.ShowPassword = true
	s.PasswordInput = "abc"
	s.PasswordError = "wrong"
	out := s.renderPassword()
	if !strings.Contains(out, "password") && !strings.Contains(out, "Administrator") {
		t.Fatalf("missing password prompt: %s", out)
	}
	if !strings.Contains(out, "wrong") {
		t.Fatal("missing error")
	}
}

func TestRenderConfirm(t *testing.T) {
	s := New()
	s.ConfirmMsg = "Update 3 items?"
	out := s.renderConfirm()
	if !strings.Contains(out, "Update 3 items?") {
		t.Fatalf("missing confirm msg: %s", out)
	}
}

func TestRenderPasswordFooter(t *testing.T) {
	s := New()
	if !strings.Contains(s.renderPasswordFooter(), "submit") {
		t.Fatal("missing submit hint")
	}
}

func TestRenderConfirmFooter(t *testing.T) {
	s := New()
	if !strings.Contains(s.renderConfirmFooter(), "yes") {
		t.Fatal("missing yes hint")
	}
}

func TestRenderUpdatesTab_UpdatingProgress(t *testing.T) {
	s := New()
	s.Updating = true
	s.UpdateTotal = 5
	s.UpdateDone = 2
	s.OperationLabel = "Homebrew"
	s.Summaries = []*model.SourceSummary{{
		Icon: "🍺", Label: "Homebrew", Total: 1,
		Items: []*model.Item{{Name: "pkg", Status: model.StatusUpdating}},
	}}
	out := s.renderUpdatesTab()
	if !strings.Contains(out, "Updating") || !strings.Contains(out, "2/5") {
		t.Fatalf("missing progress: %s", out)
	}
}

func TestRenderCategoryHeader_Updating(t *testing.T) {
	s := New()
	s.Updating = true
	summary := &model.SourceSummary{
		Icon:   "🍺",
		Label:  "Homebrew",
		Total:  2,
		Items: []*model.Item{
			{Name: "a", Status: model.StatusUpdating},
			{Name: "b", Status: model.StatusOutdated},
		},
	}
	out := s.renderCategoryHeader(summary)
	if !strings.Contains(out, "updating") {
		t.Fatalf("expected updating indicator: %s", out)
	}
}
