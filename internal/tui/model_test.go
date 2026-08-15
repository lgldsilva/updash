package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lgldsilva/updash/internal/model"
	"github.com/lgldsilva/updash/internal/updater"
)

func TestNew(t *testing.T) {
	s := New()
	if s == nil {
		t.Fatal("New() returned nil")
	} else {
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

func TestState_PageAndEdgeNavigation(t *testing.T) {
	s := New()
	for i := 0; i < 20; i++ {
		s.Summaries = append(s.Summaries, &model.SourceSummary{
			Category: model.Category(fmt.Sprintf("cat%d", i)),
			Items:    []*model.Item{{Name: fmt.Sprintf("pkg%d", i), Status: model.StatusOutdated}},
		})
	}

	s.HandleAction(KeyEnd)
	if s.Cursor != 19 {
		t.Errorf("End cursor = %d, want 19", s.Cursor)
	}

	s.HandleAction(KeyHome)
	if s.Cursor != 0 {
		t.Errorf("Home cursor = %d, want 0", s.Cursor)
	}

	s.Cursor = 0
	s.HandleAction(KeyPageDown)
	if s.Cursor == 0 {
		t.Error("PageDown should move cursor")
	}

	pos := s.Cursor
	s.HandleAction(KeyPageUp)
	if s.Cursor >= pos {
		t.Errorf("PageUp should move before %d, got %d", pos, s.Cursor)
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

func TestState_QuickSelection(t *testing.T) {
	s := New()
	s.Summaries = []*model.SourceSummary{
		{
			Category: model.CatBrew,
			Items: []*model.Item{
				{Name: "btop", Category: model.CatBrew, Status: model.StatusOutdated},
				{Name: "git", Category: model.CatBrew, Status: model.StatusOutdated},
			},
		},
		{
			Category: model.CatNpm,
			Items: []*model.Item{
				{Name: "npm", Category: model.CatNpm, Status: model.StatusOutdated},
			},
		},
	}

	s.HandleAction(KeySelectAll)
	if s.SelectedCount() != 3 {
		t.Fatalf("select all = %d, want 3", s.SelectedCount())
	}

	s.HandleAction(KeySelectNone)
	if s.SelectedCount() != 0 {
		t.Fatalf("select none = %d, want 0", s.SelectedCount())
	}

	s.Cursor = 0
	s.HandleAction(KeySelectCategory)
	if s.SelectedCount() != 2 {
		t.Fatalf("select category = %d, want 2", s.SelectedCount())
	}

	// Select-category is additive: npm stays selected when cursor moves to it.
	s.Cursor = 2
	s.HandleAction(KeySelectCategory)
	if s.SelectedCount() != 3 {
		t.Fatalf("select second category = %d, want 3", s.SelectedCount())
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

func TestState_FilterItems(t *testing.T) {
	items := []*model.Item{
		{Name: "btop", Category: model.CatBrew},
		{Name: "npm", Category: model.CatNpm},
		{Name: "node", Category: model.CatNvm},
	}

	if got := filterItems(items, ""); len(got) != 3 {
		t.Fatalf("empty filter should keep all, got %d", len(got))
	}
	if got := filterItems(items, "np"); len(got) != 1 || got[0].Name != "npm" {
		t.Fatalf("filter 'np' should match npm, got %+v", got)
	}
	if got := filterItems(items, "brew"); len(got) != 1 || got[0].Name != "btop" {
		t.Fatalf("filter 'brew' should match category, got %+v", got)
	}
	if got := filterItems(items, "xyz"); len(got) != 0 {
		t.Fatalf("filter 'xyz' should match nothing, got %d", len(got))
	}
}

func TestState_FilterApplied(t *testing.T) {
	s := New()
	s.Summaries = []*model.SourceSummary{
		{
			Category: model.CatBrew,
			Items: []*model.Item{
				{Name: "btop", Status: model.StatusOutdated},
				{Name: "git", Status: model.StatusOutdated},
			},
		},
	}
	s.AppliedFilter = "bt"
	items := s.CurrentItems()
	if len(items) != 1 || items[0].Name != "btop" {
		t.Fatalf("applied filter should show btop only, got %+v", items)
	}
}

func TestState_CancelOperation(t *testing.T) {
	s := New()
	s.Scanning = true
	s.Updating = true
	s.HandleAction(KeyCancel)
	if s.Scanning || s.Updating {
		t.Fatal("operation flags should be reset")
	}
	if s.LastSummary != "Cancelled" {
		t.Fatalf("expected Cancelled summary, got %q", s.LastSummary)
	}
}

func TestState_EscCancelsRunningOperation(t *testing.T) {
	// The full keyboard chain: HandleKey must surface KeyCancel while an
	// operation runs, and HandleAction must cancel it. Guards the wiring
	// regression where main.go dropped KeyCancel on the floor.
	s := New()
	s.Scanning = true
	if action := s.HandleKey("esc"); action != KeyCancel {
		t.Fatalf("esc during scan should map to KeyCancel, got %v", action)
	}
	s.HandleAction(KeyCancel)
	if s.Scanning {
		t.Fatal("scan flag should be reset after cancel")
	}
}

func TestState_CtrlCQuitsWithOverlaysOpen(t *testing.T) {
	// ctrl+c must quit even when a dialog swallows every other key.
	overlays := []struct {
		name  string
		setup func(*State)
	}{
		{"password", func(s *State) { s.ShowPassword = true }},
		{"filter", func(s *State) { s.ShowFilter = true }},
		{"confirm", func(s *State) { s.ShowConfirm = true }},
		{"help", func(s *State) { s.ShowHelp = true }},
		{"detail", func(s *State) { s.ShowDetail = true; s.DetailItem = &model.Item{Name: "x"} }},
		{"none", func(s *State) {}},
	}
	for _, o := range overlays {
		t.Run(o.name, func(t *testing.T) {
			s := New()
			o.setup(s)
			if action := s.HandleKey("ctrl+c"); action != KeyQuit {
				t.Fatalf("ctrl+c with %s overlay should map to KeyQuit, got %v", o.name, action)
			}
		})
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

func TestCollectOutdatedItems_SkipsManualOnly(t *testing.T) {
	// The TUI must mirror the CLI's partitionUpdatable: manual-only items
	// (KeepPolicy "manual"/jetbrains/app-store-preferred) never enter an
	// update batch — they used to be updated or counted as failures here.
	s := New()
	s.Summaries = []*model.SourceSummary{
		{Category: model.CatBrew, Label: "Homebrew", Items: []*model.Item{
			{Name: "btop", Status: model.StatusOutdated},
			{Name: "whatsapp", Status: model.StatusOutdated, KeepPolicy: "manual reinstall via Mac App Store"},
			{Name: "up-to-date", Status: model.StatusOK},
		}},
		{Category: model.CatAgent, Label: "Agents", Items: []*model.Item{
			{Name: "Cursor", Status: model.StatusOutdated, KeepPolicy: "⊘ manual reinstall / app update"},
		}},
	}

	got := s.collectOutdatedItems(false)
	if len(got) != 1 || got[0].Name != "btop" {
		t.Fatalf("expected only btop (manual-only skipped), got %+v", got)
	}
	if n := len(s.Logs); n == 0 || !strings.Contains(s.Logs[n-1].Message, "manual-only") {
		t.Fatalf("expected manual-only skip log, got %+v", s.Logs)
	}
}

func TestIsManualOnlyItem(t *testing.T) {
	cases := []struct {
		name string
		item *model.Item
		want bool
	}{
		{"nil", nil, false},
		{"no policy", &model.Item{Name: "btop"}, false},
		{"retention policy", &model.Item{Name: "npm-cache", KeepPolicy: "cache + npx extractions"}, false},
		{"manual agent", &model.Item{Name: "Cursor", KeepPolicy: "⊘ manual reinstall / app update"}, true},
		{"jetbrains cask", &model.Item{Name: "intellij-idea", KeepPolicy: "JetBrains Toolbox manages updates"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsManualOnlyItem(tc.item); got != tc.want {
				t.Errorf("IsManualOnlyItem(%s) = %v, want %v", tc.name, got, tc.want)
			}
		})
	}
}

func TestCountResults_ManualOnlyIsNotFailure(t *testing.T) {
	results := []*updater.Result{
		{Item: &model.Item{Name: "ok"}, Success: true},
		{Item: &model.Item{Name: "cursor", KeepPolicy: "⊘ manual reinstall"}, Success: false, Error: "manual reinstall"},
		{Item: &model.Item{Name: "boom"}, Success: false, Error: "exit 1"},
	}
	ok, fail := countResults(results)
	if ok != 1 || fail != 1 {
		t.Fatalf("expected 1 ok / 1 fail (manual-only skipped), got %d ok / %d fail", ok, fail)
	}
}

func TestLogUpdateResults_ManualOnlyLogsHint(t *testing.T) {
	s := New()
	s.LogUpdateResults([]*updater.Result{
		{Item: &model.Item{Name: "ok"}, Success: true},
		{Item: &model.Item{Name: "cursor", Category: model.CatAgent, KeepPolicy: "⊘ manual reinstall"}, Success: false},
		{Item: &model.Item{Name: "boom"}, Success: false, Error: "kaboom"},
	})
	if len(s.Logs) != 3 {
		t.Fatalf("expected 3 log entries, got %d", len(s.Logs))
	}
	if !strings.Contains(s.Logs[0].Message, "✓ ok: updated") {
		t.Errorf("unexpected first log: %q", s.Logs[0].Message)
	}
	if !strings.Contains(s.Logs[1].Message, "⊘ cursor: manual only") {
		t.Errorf("expected manual hint log, got %q", s.Logs[1].Message)
	}
	if !strings.Contains(s.Logs[2].Message, "✘ boom: kaboom") {
		t.Errorf("unexpected failure log: %q", s.Logs[2].Message)
	}
}

func TestFlattenCleanItems_ShowsErrorOnlySummaries(t *testing.T) {
	// A cleanup source whose scan failed must stay visible on the Cleanup
	// tab — hasCleanupItems used to drop error-only summaries entirely.
	s := New()
	s.CleanItems = []*model.SourceSummary{
		{Category: model.CatCache, Label: "npm Cache", Items: []*model.Item{
			{Name: "npm-cache", Status: model.StatusError, CurrentVer: "scan failed"},
		}},
	}
	items := s.FlattenCleanItems()
	if len(items) != 1 || items[0].Name != "npm-cache" {
		t.Fatalf("expected error item visible on Cleanup tab, got %+v", items)
	}
}

func TestCollectOutdatedItems_RespectsAppliedFilter(t *testing.T) {
	// "U" must not update items hidden by the active "/" filter.
	s := New()
	s.Summaries = []*model.SourceSummary{
		{Category: model.CatBrew, Label: "Homebrew", Items: []*model.Item{
			{Name: "btop", Category: model.CatBrew, Status: model.StatusOutdated},
		}},
		{Category: model.CatNpm, Label: "npm", Items: []*model.Item{
			{Name: "typescript", Category: model.CatNpm, Status: model.StatusOutdated},
		}},
	}
	s.AppliedFilter = "brew"

	got := s.collectOutdatedItems(false)
	if len(got) != 1 || got[0].Name != "btop" {
		t.Fatalf("expected filter to keep only btop, got %+v", got)
	}
}

func TestHandleKey_TabGatesActions(t *testing.T) {
	cases := []struct {
		name string
		tab  model.TabID
		key  string
		want KeyAction
	}{
		{"u on updates", model.TabUpdates, "u", KeyUpdateSelected},
		{"u on cleanup", model.TabCleanup, "u", KeyNone},
		{"c on cleanup", model.TabCleanup, "c", KeyCleanSelected},
		{"c on updates", model.TabUpdates, "c", KeyNone},
		{"a on updates", model.TabUpdates, "a", KeyUpdateAll},
		{"a on cleanup", model.TabCleanup, "a", KeyCleanAll},
		{"a on logs", model.TabLogs, "a", KeyNone},
		{"space on logs", model.TabLogs, " ", KeyNone},
		{"enter on logs", model.TabLogs, "enter", KeyNone},
		{"select-all on logs", model.TabLogs, "*", KeyNone},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := New()
			s.ActiveTab = tc.tab
			if got := s.HandleKey(tc.key); got != tc.want {
				t.Errorf("HandleKey(%q) on %v = %v, want %v", tc.key, tc.tab, got, tc.want)
			}
		})
	}
}

func TestRefuseBusy_BlocksConflictingOps(t *testing.T) {
	s := New()
	s.Updating = true
	if !s.refuseBusy("Update") {
		t.Fatal("refuseBusy should reject while updating")
	}
	if s.ShowConfirm {
		t.Fatal("confirm dialog must not open while busy")
	}
	s.Updating = false
	if s.refuseBusy("Update") {
		t.Fatal("refuseBusy should allow when idle")
	}
}

func TestHandlePasswordless(t *testing.T) {
	t.Run("passwordless proceeds", func(t *testing.T) {
		s := New()
		called := false
		s.ConfirmCmd = func(p *tea.Program) tea.Cmd { called = true; return nil }
		s.HandlePasswordless(true, nil)
		if !called {
			t.Fatal("pending op should run after passwordless OK")
		}
		if s.ElevSession == nil || !s.ElevSession.Ready() {
			t.Fatal("passwordless session should be cached")
		}
	})
	t.Run("needs password opens dialog", func(t *testing.T) {
		s := New()
		called := false
		s.ConfirmCmd = func(p *tea.Program) tea.Cmd { called = true; return nil }
		s.HandlePasswordless(false, nil)
		if called {
			t.Fatal("pending op must not run before the password")
		}
		if !s.ShowPassword {
			t.Fatal("password dialog should open")
		}
		if s.ConfirmCmd == nil {
			t.Fatal("pending op should survive until password or cancel")
		}
	})
}
