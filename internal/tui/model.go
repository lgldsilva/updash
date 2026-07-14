// Package tui implements the Bubble Tea TUI for updash.
package tui

import (
	"context"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lgldsilva/updash/internal/elevate"
	"github.com/lgldsilva/updash/internal/model"
	"github.com/lgldsilva/updash/internal/platform"
)

// State holds all UI state.
type State struct {
	// Platform
	Platform model.PlatformInfo

	// Active tab
	ActiveTab model.TabID

	// Scan results
	Summaries  []*model.SourceSummary
	CleanItems []*model.SourceSummary

	// Selection
	Cursor     int // current cursor position across all items
	TotalItems int // total items across all sources

	// Status
	Scanning     bool
	Updating     bool
	Cleaning     bool
	Ready        bool
	ShowHelp     bool
	ShowConfirm  bool // confirmation dialog for destructive actions
	ShowPassword bool // sudo password prompt (reused across elevated batches)

	// Confirmation state
	ConfirmMsg    string
	ConfirmAction func()
	ConfirmCmd    func(program *tea.Program) tea.Cmd // replaces ConfirmAction for async ops

	// Pending items for async operations (cleared after confirm/cancel)
	PendingUpdateItems []*model.Item
	PendingCleanItems  []*model.Item

	// Elevation (sudo password cached for the session)
	PasswordInput string
	PasswordError string
	ElevSession   *elevate.Session
	ElevWait      chan struct{} // closed when mid-operation password is validated

	// Progress tracking (written only from event loop)
	UpdateTotal  int
	UpdateDone   int
	UpdateErrors int
	CleanTotal   int
	CleanDone    int

	// Live operation feedback
	OperationLabel string // e.g. "Homebrew" during batch update
	LastSummary    string // shown after update/clean completes
	SpinnerFrame   int    // animated spinner index

	// Logs
	Logs []model.GlobalLogEntry

	// Bubble Tea program (for background Send; set from main after NewProgram)
	Program *tea.Program

	// Scan progress
	ScanTotal int
	ScanDone  int

	// Context for cancellation
	Ctx    context.Context
	Cancel context.CancelFunc

	// Window dimensions
	Width  int
	Height int

	// Error state
	Error string
}

// New creates a new TUI state.
func New() *State {
	ctx, cancel := context.WithCancel(context.Background())
	plat := platform.Detect()

	return &State{
		Platform:  plat,
		ActiveTab: model.TabUpdates,
		Ctx:       ctx,
		Cancel:    cancel,
		Ready:     true,
		Width:     80,
		Height:    24,
	}
}

// isUpdateNavigable reports items shown and selectable on the Updates tab.
func isUpdateNavigable(status model.Status) bool {
	switch status {
	case model.StatusOutdated, model.StatusUpdating, model.StatusError, model.StatusDone:
		return true
	default:
		return false
	}
}

// hasUpdateItems reports whether a summary has actionable rows on the Updates tab.
func hasUpdateItems(summary *model.SourceSummary) bool {
	for _, it := range summary.Items {
		if isUpdateNavigable(it.Status) {
			return true
		}
	}
	return false
}

// countLiveOutdated returns outdated items from live statuses (not stale summary.Outdated).
func countLiveOutdated(items []*model.Item) int {
	n := 0
	for _, it := range items {
		if it.Status == model.StatusOutdated {
			n++
		}
	}
	return n
}

// FlattenItems returns all items from all summaries as a flat slice.
func (s *State) FlattenItems() []*model.Item {
	var items []*model.Item
	for _, summary := range s.Summaries {
		items = append(items, summary.Items...)
	}
	return items
}

// FlattenUpdateItems returns update-tab navigable items (hides up-to-date noise like agent inventory).
func (s *State) FlattenUpdateItems() []*model.Item {
	var items []*model.Item
	for _, summary := range s.Summaries {
		if !hasUpdateItems(summary) {
			continue
		}
		for _, it := range summary.Items {
			if isUpdateNavigable(it.Status) {
				items = append(items, it)
			}
		}
	}
	return items
}

// isCleanupNavigable reports items shown and selectable on the Cleanup tab.
func isCleanupNavigable(status model.Status) bool {
	switch status {
	case model.StatusCleanCandidate, model.StatusCleaning, model.StatusCleaned, model.StatusError:
		return true
	default:
		return false
	}
}

// FlattenCleanItems returns cleanup items visible in the Cleanup tab (same order as render).
func (s *State) FlattenCleanItems() []*model.Item {
	var items []*model.Item
	for _, summary := range s.CleanItems {
		if !hasCleanupItems(summary) {
			continue
		}
		for _, it := range summary.Items {
			if isCleanupNavigable(it.Status) {
				items = append(items, it)
			}
		}
	}
	return items
}

// CurrentItems returns the flat navigable list for the active tab.
func (s *State) CurrentItems() []*model.Item {
	if s.ActiveTab == model.TabCleanup {
		return s.FlattenCleanItems()
	}
	return s.FlattenUpdateItems()
}

// ClampCursor keeps the cursor within the current tab's item list.
func (s *State) ClampCursor() {
	items := s.CurrentItems()
	if len(items) == 0 {
		s.Cursor = 0
		return
	}
	if s.Cursor < 0 {
		s.Cursor = 0
	}
	if s.Cursor >= len(items) {
		s.Cursor = len(items) - 1
	}
}

// SelectedCount returns how many items are selected on the active tab.
func (s *State) SelectedCount() int {
	n := 0
	for _, it := range s.CurrentItems() {
		if it.Selected {
			n++
		}
	}
	return n
}

// TotalOutdated returns total outdated items from live item statuses.
func (s *State) TotalOutdated() int {
	count := 0
	for _, summary := range s.Summaries {
		count += countLiveOutdated(summary.Items)
	}
	return count
}

// TotalScanErrors counts scan failures across Updates and Cleanup tabs.
func (s *State) TotalScanErrors() int {
	var n int
	for _, summary := range s.Summaries {
		n += countStatus(summary.Items, model.StatusError)
	}
	for _, summary := range s.CleanItems {
		n += countStatus(summary.Items, model.StatusError)
	}
	return n
}

func countStatus(items []*model.Item, want model.Status) int {
	n := 0
	for _, it := range items {
		if it.Status == want {
			n++
		}
	}
	return n
}

// LogScanErrors writes per-source scan failures to the Logs tab.
func (s *State) LogScanErrors() {
	logScanErrors := func(summaries []*model.SourceSummary) {
		for _, sum := range summaries {
			for _, it := range sum.Items {
				if it.Status != model.StatusError {
					continue
				}
				detail := it.CurrentVer
				if detail == "" {
					detail = "scan failed"
				}
				s.AddLog(fmt.Sprintf("✘ %s %s — %s: %s", sum.Icon, sum.Label, it.Name, detail), false)
			}
		}
	}
	logScanErrors(s.Summaries)
	logScanErrors(s.CleanItems)
}

// TotalCleanable returns total cleanable items.
func (s *State) TotalCleanable() int {
	count := 0
	for _, summary := range s.CleanItems {
		count += summary.Outdated
	}
	return count
}

// AddLog adds a log entry.
func (s *State) AddLog(msg string, success bool) {
	s.Logs = append(s.Logs, model.GlobalLogEntry{
		Timestamp: "now",
		Message:   msg,
		Success:   success,
	})
	if len(s.Logs) > 100 {
		s.Logs = s.Logs[len(s.Logs)-100:]
	}
}

// ctxWithElev returns the state context with a cached elevation session attached.
func (s *State) ctxWithElev() context.Context {
	if s.ElevSession != nil && s.ElevSession.Ready() {
		return elevate.WithSession(s.Ctx, s.ElevSession)
	}
	return s.Ctx
}

func (s *State) pendingItemsForConfirm() ([]*model.Item, bool) {
	if len(s.PendingUpdateItems) > 0 {
		return s.PendingUpdateItems, false
	}
	return s.PendingCleanItems, true
}

// needsElevationPrompt reports whether the user must enter a sudo password
// before starting the pending operation. MAS-only elevation is deferred until
// the MAS batch runs so sudo credentials stay fresh after long brew runs.
func (s *State) needsElevationPrompt() bool {
	items, cleanup := s.pendingItemsForConfirm()
	if len(items) == 0 {
		return false
	}
	if !elevate.ItemsNeedElevation(items, s.Platform, cleanup) {
		return false
	}
	if s.elevationReady() {
		return false
	}
	if !cleanup && s.canDeferMASElevation(items) {
		return false
	}
	return true
}

func (s *State) elevationReady() bool {
	if s.ElevSession != nil && s.ElevSession.Ready() {
		return true
	}
	if elevate.CanElevateWithoutPassword(s.Ctx) {
		s.ElevSession = elevate.NewSession()
		s.ElevSession.SetPasswordless()
		return true
	}
	return false
}

// canDeferMASElevation returns true when only MAS items need sudo and other
// update batches can run first without a password prompt.
func (s *State) canDeferMASElevation(items []*model.Item) bool {
	needsMAS := false
	for _, it := range items {
		if !elevate.CategoryNeedsElevation(it.Category, s.Platform) {
			continue
		}
		if it.Category == model.CatMAS {
			needsMAS = true
			continue
		}
		return false
	}
	return needsMAS
}

// waitForElevation blocks the caller until sudo is ready or ctx is cancelled.
// Must run from a background goroutine; prompts via ElevRequiredMsg.
func (s *State) waitForElevation(ctx context.Context, program *tea.Program, reason string) error {
	if s.elevationReady() {
		return nil
	}
	wait := make(chan struct{})
	s.ElevWait = wait
	program.Send(ElevRequiredMsg{Reason: reason})
	select {
	case <-wait:
		if s.elevationReady() {
			return nil
		}
		return fmt.Errorf("sudo password required")
	case <-ctx.Done():
		return ctx.Err()
	}
}

// HandlePasswordOK stores the session and resumes a blocked operation if needed.
func (s *State) HandlePasswordOK(session *elevate.Session, program *tea.Program) tea.Cmd {
	s.ElevSession = session
	if s.ElevWait != nil {
		s.ShowPassword = false
		s.PasswordInput = ""
		s.PasswordError = ""
		s.signalElevation()
		return TickCmd()
	}
	return s.ConsumeConfirmAfterPassword(program)
}

// signalElevation unblocks a goroutine waiting in waitForElevation.
func (s *State) signalElevation() {
	if s.ElevWait != nil {
		close(s.ElevWait)
		s.ElevWait = nil
	}
}

// ConsumeConfirmCmd returns the pending async cmd and clears confirm state.
// If elevation is required, shows the password prompt instead.
// Call only from the Bubble Tea event loop (not from goroutines).
func (s *State) ConsumeConfirmCmd(program *tea.Program) tea.Cmd {
	if s.ConfirmCmd == nil {
		return nil
	}
	if s.needsElevationPrompt() {
		s.ShowPassword = true
		s.PasswordInput = ""
		s.PasswordError = ""
		s.ShowConfirm = false
		return nil
	}
	return s.finishConfirm(program)
}

// ConsumeConfirmAfterPassword runs the pending cmd after password validation.
func (s *State) ConsumeConfirmAfterPassword(program *tea.Program) tea.Cmd {
	s.ShowPassword = false
	s.PasswordInput = ""
	s.PasswordError = ""
	return s.finishConfirm(program)
}

func (s *State) finishConfirm(program *tea.Program) tea.Cmd {
	if s.ConfirmCmd == nil {
		return nil
	}
	cmd := s.ConfirmCmd(program)
	s.ConfirmCmd = nil
	s.PendingUpdateItems = nil
	s.PendingCleanItems = nil
	s.ShowConfirm = false
	return cmd
}

// CancelPassword clears the password prompt and pending operation.
func (s *State) CancelPassword() {
	s.ShowPassword = false
	s.PasswordInput = ""
	s.PasswordError = ""
	s.signalElevation()
	s.ConfirmCmd = nil
	s.PendingUpdateItems = nil
	s.PendingCleanItems = nil
}

// ClearElevation wipes cached sudo credentials.
func (s *State) ClearElevation() {
	if s.ElevSession != nil {
		s.ElevSession.Clear()
	}
	s.ElevSession = nil
	s.PasswordInput = ""
}
