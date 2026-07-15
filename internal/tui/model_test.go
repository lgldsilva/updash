package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lgldsilva/updash/internal/elevate"
	"github.com/lgldsilva/updash/internal/model"
)

func TestNew(t *testing.T) {
	s := New()
	if s == nil {
		t.Fatal("New() returned nil")
	}
	if s.Platform.OS == "" {
		t.Error("Platform.OS must be set")
	}
	if s.ActiveTab != model.TabUpdates {
		t.Errorf("ActiveTab = %v, want TabUpdates", s.ActiveTab)
	}
	if s.Ctx == nil {
		t.Error("Ctx must not be nil")
	}
}

func TestState_AddLog(t *testing.T) {
	s := New()
	s.AddLog("test message", true)
	if len(s.Logs) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(s.Logs))
	}
	if s.Logs[0].Message != "test message" {
		t.Errorf("Log message = %q, want %q", s.Logs[0].Message, "test message")
	}
	if !s.Logs[0].Success {
		t.Error("expected success = true")
	}

	// Test log cap at 100
	for i := 0; i < 150; i++ {
		s.AddLog("log", true)
	}
	if len(s.Logs) > 100 {
		t.Errorf("logs exceeded 100: %d", len(s.Logs))
	}
}

func TestState_TotalOutdated(t *testing.T) {
	s := New()
	s.Summaries = []*model.SourceSummary{
		{
			Category: model.CatBrew,
			Items: []*model.Item{
				{Name: "pkg1", Status: model.StatusOutdated},
				{Name: "pkg2", Status: model.StatusOK},
			},
			Outdated: 1,
		},
		{
			Category: model.CatNpm,
			Items: []*model.Item{
				{Name: "npm1", Status: model.StatusOutdated},
			},
			Outdated: 1,
		},
	}

	if got := s.TotalOutdated(); got != 2 {
		t.Errorf("TotalOutdated = %d, want 2", got)
	}

	// Stale summary.Outdated must not inflate the tab badge.
	s.Summaries[0].Outdated = 99
	if got := s.TotalOutdated(); got != 2 {
		t.Errorf("TotalOutdated with stale summary = %d, want 2", got)
	}
}

func TestState_FlattenUpdateItems_hidesUpToDateAgents(t *testing.T) {
	s := New()
	s.Summaries = []*model.SourceSummary{
		{
			Category: model.CatAgent,
			Items: []*model.Item{
				{Name: "Claude", Status: model.StatusOK},
				{Name: "Grok", Status: model.StatusOK},
			},
		},
		{
			Category: model.CatBrew,
			Items: []*model.Item{
				{Name: "btop", Status: model.StatusOutdated},
				{Name: "neovim", Status: model.StatusOK},
			},
		},
	}
	items := s.FlattenUpdateItems()
	if len(items) != 1 || items[0].Name != "btop" {
		t.Fatalf("FlattenUpdateItems = %+v, want only btop", items)
	}
}

func TestState_TotalCleanable(t *testing.T) {
	s := New()
	s.CleanItems = []*model.SourceSummary{
		{
			Category: model.CatCache,
			Items: []*model.Item{
				{Name: "go-cache", Status: model.StatusCleanCandidate},
				{Name: "npm-cache", Status: model.StatusOK},
			},
			Outdated: 1,
		},
	}

	if got := s.TotalCleanable(); got != 1 {
		t.Errorf("TotalCleanable = %d, want 1", got)
	}
}

func TestState_CurrentItems(t *testing.T) {
	s := New()

	// Updates tab
	s.ActiveTab = model.TabUpdates
	s.Summaries = []*model.SourceSummary{
		{
			Category: model.CatBrew,
			Items:    []*model.Item{{Name: "brew-pkg", Status: model.StatusOutdated}},
		},
	}
	items := s.CurrentItems()
	if len(items) != 1 {
		t.Fatalf("expected 1 item in updates tab, got %d", len(items))
	}
	if items[0].Name != "brew-pkg" {
		t.Errorf("item name = %q, want %q", items[0].Name, "brew-pkg")
	}

	// Cleanup tab — only navigable clean candidates (not hidden OK-only summaries)
	s.ActiveTab = model.TabCleanup
	s.CleanItems = []*model.SourceSummary{
		{
			Category: model.CatDockerClean,
			Items:    []*model.Item{{Name: "docker", Status: model.StatusOK, CurrentVer: "nothing to clean"}},
		},
		{
			Category: model.CatCache,
			Items: []*model.Item{
				{Name: "npm-cache", Status: model.StatusOK, CurrentVer: "no cache"},
				{Name: "go-cache", Status: model.StatusCleanCandidate},
			},
		},
	}
	items = s.CurrentItems()
	if len(items) != 1 {
		t.Fatalf("expected 1 navigable cleanup item, got %d", len(items))
	}
	if items[0].Name != "go-cache" {
		t.Errorf("item name = %q, want %q", items[0].Name, "go-cache")
	}
}

func TestState_CleanupToggleMatchesCursor(t *testing.T) {
	s := New()
	s.ActiveTab = model.TabCleanup
	s.Ready = true
	s.CleanItems = []*model.SourceSummary{
		{
			Category: model.CatDockerClean,
			Items:    []*model.Item{{Name: "docker", Status: model.StatusOK}},
		},
		{
			Category: model.CatCache,
			Items: []*model.Item{
				{Name: "go-cache", Status: model.StatusCleanCandidate},
				{Name: "npm-cache", Status: model.StatusCleanCandidate},
			},
		},
	}

	s.Cursor = 0
	s.HandleAction(KeySelect)
	if !s.CleanItems[1].Items[0].Selected {
		t.Fatal("cursor 0 should toggle go-cache")
	}

	s.Cursor = 1
	s.HandleAction(KeySelect)
	if !s.CleanItems[1].Items[1].Selected {
		t.Fatal("cursor 1 should toggle npm-cache")
	}
}

func TestState_HandleKey(t *testing.T) {
	s := New()

	// Tab switching
	if action := s.HandleKey("1"); action != KeyTab {
		t.Errorf("HandleKey('1') = %v, want KeyTab", action)
	}
	if s.ActiveTab != model.TabUpdates {
		t.Errorf("ActiveTab should be Updates")
	}

	if action := s.HandleKey("2"); action != KeyTab {
		t.Errorf("HandleKey('2') = %v, want KeyTab", action)
	}
	if s.ActiveTab != model.TabCleanup {
		t.Errorf("ActiveTab should be Cleanup")
	}

	if action := s.HandleKey("3"); action != KeyTab {
		t.Errorf("HandleKey('3') = %v, want KeyTab", action)
	}
	if s.ActiveTab != model.TabLogs {
		t.Errorf("ActiveTab should be Logs")
	}
}

func TestState_HandleActions(t *testing.T) {
	s := New()
	s.Summaries = []*model.SourceSummary{
		{
			Category: model.CatBrew,
			Items: []*model.Item{
				{Name: "pkg1", Status: model.StatusOutdated},
				{Name: "pkg2", Status: model.StatusOK},
				{Name: "pkg3", Status: model.StatusOutdated},
			},
		},
	}
	s.Ready = true

	// Navigate down
	s.HandleAction(KeyDown)
	if s.Cursor != 1 {
		t.Errorf("Cursor after KeyDown = %d, want 1", s.Cursor)
	}

	// Navigate down again (pkg2 is StatusOK and hidden from the updates list)
	s.HandleAction(KeyDown)
	if s.Cursor != 1 {
		t.Errorf("Cursor after second KeyDown = %d, want 1", s.Cursor)
	}

	// Navigate up
	s.HandleAction(KeyUp)
	if s.Cursor != 0 {
		t.Errorf("Cursor after KeyUp = %d, want 0", s.Cursor)
	}

	// Boundary: navigate up past 0
	s.HandleAction(KeyUp)
	s.HandleAction(KeyUp)
	s.HandleAction(KeyUp)
	if s.Cursor != 0 {
		t.Errorf("Cursor should not go below 0, got %d", s.Cursor)
	}

	// Toggle selection on an outdated item
	s.Cursor = 0
	s.HandleAction(KeySelect)
	if !s.Summaries[0].Items[0].Selected {
		t.Error("outdated item should be selected after toggle")
	}

	// Toggle again (deselect)
	s.HandleAction(KeySelect)
	if s.Summaries[0].Items[0].Selected {
		t.Error("item should be deselected after second toggle")
	}
}

func TestState_SelectedCount(t *testing.T) {
	s := New()
	s.Summaries = []*model.SourceSummary{{
		Items: []*model.Item{
			{Selected: true, Status: model.StatusOutdated},
			{Selected: false, Status: model.StatusOutdated},
			{Selected: true, Status: model.StatusOutdated},
		},
	}}
	if got := s.SelectedCount(); got != 2 {
		t.Fatalf("SelectedCount = %d, want 2", got)
	}
}

func TestState_ClampCursor_Empty(t *testing.T) {
	s := New()
	s.Cursor = 5
	s.ClampCursor()
	if s.Cursor != 0 {
		t.Fatalf("cursor = %d, want 0", s.Cursor)
	}
}

func TestState_FlattenItems(t *testing.T) {
	s := New()
	s.Summaries = []*model.SourceSummary{
		{Items: []*model.Item{{Name: "a"}, {Name: "b"}}},
		{Items: []*model.Item{{Name: "c"}}},
	}
	if len(s.FlattenItems()) != 3 {
		t.Fatalf("flatten = %d", len(s.FlattenItems()))
	}
}

func TestState_CtxWithElev(t *testing.T) {
	s := New()
	s.ElevSession = elevate.NewSession()
	s.ElevSession.SetPasswordless()
	ctx := s.ctxWithElev()
	if elevate.FromContext(ctx) == nil {
		t.Fatal("expected session in context")
	}
}

func TestState_CancelPasswordAndClearElevation(t *testing.T) {
	s := New()
	s.ShowPassword = true
	s.PasswordInput = "x"
	s.PasswordError = "err"
	s.ConfirmCmd = func(p *tea.Program) tea.Cmd { return nil }
	s.PendingUpdateItems = []*model.Item{{Name: "a"}}
	s.PendingCleanItems = []*model.Item{{Name: "b"}}
	s.CancelPassword()
	if s.ShowPassword || s.PasswordInput != "" || s.PasswordError != "" {
		t.Fatal("password state not cleared")
	}
	if s.ConfirmCmd != nil || s.PendingUpdateItems != nil || s.PendingCleanItems != nil {
		t.Fatal("pending ops not cleared")
	}
	s.ClearElevation()
	if s.ElevSession != nil {
		t.Fatal("elev session should stay nil")
	}
}

func TestState_ConsumeConfirmCmd_nil(t *testing.T) {
	s := New()
	if cmd := s.ConsumeConfirmCmd(nil); cmd != nil {
		t.Fatal("expected nil cmd when ConfirmCmd is nil")
	}
	called := false
	s.ConfirmCmd = func(p *tea.Program) tea.Cmd {
		called = true
		return nil
	}
	s.ShowConfirm = true
	_ = s.ConsumeConfirmCmd(nil)
	// Either password prompt or finishConfirm; both are success if no panic.
	_ = called
	_ = s.ConsumeConfirmAfterPassword(nil)
	s.CancelPassword()
}

func TestState_ClampCursorAndLogScanErrors(t *testing.T) {
	s := New()
	s.Cursor = 99
	s.ClampCursor()
	if s.Cursor < 0 {
		t.Fatal("cursor negative")
	}
	s.Summaries = []*model.SourceSummary{
		{Category: model.CatBrew, ErrorCount: 1, Items: []*model.Item{{Name: "x", Status: model.StatusError, CurrentVer: "fail"}}},
	}
	s.LogScanErrors()
}
