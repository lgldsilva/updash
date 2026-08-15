// Package tui implements the Bubble Tea TUI for updash.
package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

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
	ShowHelp     bool
	ShowConfirm  bool // confirmation dialog for destructive actions
	ShowPassword bool // sudo password prompt (reused across elevated batches)

	// Confirmation state
	ConfirmMsg    string
	ConfirmAction func()
	ConfirmCmd    func(program *tea.Program) tea.Cmd // replaces ConfirmAction for async ops

	// Filter state
	ShowFilter    bool
	FilterInput   string
	AppliedFilter string

	// Detail view state
	ShowDetail bool
	DetailItem *model.Item

	// Pending items for async operations (cleared after confirm/cancel)
	PendingUpdateItems []*model.Item
	PendingCleanItems  []*model.Item

	// Elevation (sudo password cached for the session)
	PasswordInput string
	PasswordError string
	ElevSession   *elevate.Session
	ElevWait      chan *elevate.Session // receives the validated session for a waiting worker (nil on cancel)

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

	// Build info (shown in title bar)
	Version   string
	LatestTag string
}

// New creates a new TUI state.
func New() *State {
	return NewWithVersion("", "")
}

// NewWithVersion creates TUI state with build/release tags for the title bar.
func NewWithVersion(version, latest string) *State {
	ctx, cancel := context.WithCancel(context.Background())
	plat := platform.Detect()

	return &State{
		Platform:  plat,
		ActiveTab: model.TabUpdates,
		Ctx:       ctx,
		Cancel:    cancel,
		Width:     80,
		Height:    24,
		Version:   version,
		LatestTag: latest,
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

// flattenSummaries returns navigable items from a slice of summaries using the
// supplied predicates. Extracted to keep update and cleanup flatteners in sync.
func flattenSummaries(
	summaries []*model.SourceSummary,
	has func(*model.SourceSummary) bool,
	navigable func(model.Status) bool,
	filter string,
) []*model.Item {
	var items []*model.Item
	for _, summary := range summaries {
		if !has(summary) {
			continue
		}
		for _, it := range summary.Items {
			if navigable(it.Status) {
				items = append(items, it)
			}
		}
	}
	return filterItems(items, filter)
}

// FlattenUpdateItems returns update-tab navigable items (hides up-to-date noise like agent inventory).
func (s *State) FlattenUpdateItems() []*model.Item {
	return flattenSummaries(s.Summaries, hasUpdateItems, isUpdateNavigable, s.AppliedFilter)
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
	return flattenSummaries(s.CleanItems, hasCleanupItems, isCleanupNavigable, s.AppliedFilter)
}

// filterItems keeps items whose name or category matches the filter (case-insensitive).
// An empty filter keeps every item.
func filterItems(items []*model.Item, filter string) []*model.Item {
	if filter == "" {
		return items
	}
	f := strings.ToLower(filter)
	out := make([]*model.Item, 0, len(items))
	for _, it := range items {
		if strings.Contains(strings.ToLower(it.Name), f) ||
			strings.Contains(strings.ToLower(string(it.Category)), f) {
			out = append(out, it)
		}
	}
	return out
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

// TotalCleanable returns total cleanable items from live item statuses
// (summary.Outdated goes stale after rescans swap summaries).
func (s *State) TotalCleanable() int {
	count := 0
	for _, summary := range s.CleanItems {
		count += countStatus(summary.Items, model.StatusCleanCandidate)
	}
	return count
}

// AddLog adds a log entry.
func (s *State) AddLog(msg string, success bool) {
	s.Logs = append(s.Logs, model.GlobalLogEntry{
		Timestamp: time.Now().Format("15:04:05"),
		Message:   msg,
		Success:   success,
	})
	if len(s.Logs) > 100 {
		s.Logs = s.Logs[len(s.Logs)-100:]
	}
}

func (s *State) pendingItemsForConfirm() ([]*model.Item, bool) {
	if len(s.PendingUpdateItems) > 0 {
		return s.PendingUpdateItems, false
	}
	return s.PendingCleanItems, true
}

// needsElevationPrompt reports whether the pending confirmation needs a sudo
// password before starting. Excludes the passwordless sudo probe (which can
// block for seconds) — that runs as a tea.Cmd, see ConsumeConfirmCmd.
func (s *State) needsElevationPrompt() bool {
	items, cleanup := s.pendingItemsForConfirm()
	if len(items) == 0 {
		return false
	}
	if !elevate.ItemsNeedElevation(items, s.Platform, cleanup) {
		return false
	}
	if s.elevationSessionReady() {
		return false
	}
	if !cleanup && s.canDeferMASElevation(items) {
		return false
	}
	return true
}

func (s *State) elevationSessionReady() bool {
	return s.ElevSession != nil && s.ElevSession.Ready()
}

// canDeferMASElevation returns true when only MAS items need sudo and other
// update batches can run first without a password prompt. Brew PKG casks
// (Microsoft) also sudo-prompt mid-run, so they must be primed up-front too.
func (s *State) canDeferMASElevation(items []*model.Item) bool {
	needsMAS := false
	for _, it := range items {
		if !elevate.ItemNeedsUpdateElevation(it, s.Platform) {
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

// waitForElevation blocks the worker until sudo is ready or ctx is
// cancelled. It prompts via ElevRequiredMsg carrying the wait channel; the
// event loop delivers the validated session (or nil on cancel) through
// that channel — the channel handoff keeps the worker off State fields.
func waitForElevation(ctx context.Context, program *tea.Program, reason string, sess *elevate.Session) (*elevate.Session, error) {
	if sess != nil && sess.Ready() {
		return sess, nil
	}
	wait := make(chan *elevate.Session, 1)
	program.Send(ElevRequiredMsg{Reason: reason, Wait: wait})
	select {
	case got := <-wait:
		if got == nil || !got.Ready() {
			return nil, fmt.Errorf("sudo password required")
		}
		return got, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// HandlePasswordOK stores the session and resumes a blocked operation if needed.
func (s *State) HandlePasswordOK(session *elevate.Session, program *tea.Program) tea.Cmd {
	s.ElevSession = session
	if s.ElevWait != nil {
		s.ShowPassword = false
		s.PasswordInput = ""
		s.PasswordError = ""
		s.deliverElevation(session)
		return nil // spinner animation runs on the single tick chain (see onTick)
	}
	return s.ConsumeConfirmAfterPassword(program)
}

// deliverElevation hands a validated session (or nil on cancel) to the
// waiting worker. The channel is buffered, so this never blocks.
func (s *State) deliverElevation(sess *elevate.Session) {
	if s.ElevWait != nil {
		s.ElevWait <- sess
		s.ElevWait = nil
	}
}

// ConsumeConfirmCmd returns the pending async cmd and clears confirm state.
// When a sudo password may be needed, the sudo -n probe runs as a tea.Cmd
// (Bubble Tea executes cmds in their own goroutine) — it can block for
// seconds on a black-holed network and must never freeze the render loop.
// Call only from the Bubble Tea event loop (not from goroutines).
func (s *State) ConsumeConfirmCmd(program *tea.Program) tea.Cmd {
	if s.ConfirmCmd == nil {
		return nil
	}
	if s.needsElevationPrompt() {
		s.ShowConfirm = false
		return checkPasswordlessElevation
	}
	return s.finishConfirm(program)
}

// checkPasswordlessElevation probes sudo -n off the event loop.
func checkPasswordlessElevation() tea.Msg {
	return PasswordlessResultMsg{OK: elevate.CanElevateWithoutPassword(context.Background())}
}

// HandlePasswordless resumes the pending confirmation after the sudo -n
// probe: passwordless sudo continues straight away, otherwise the password
// dialog opens.
func (s *State) HandlePasswordless(ok bool, program *tea.Program) tea.Cmd {
	if ok {
		sess := elevate.NewSession()
		sess.SetPasswordless()
		s.ElevSession = sess
		return s.finishConfirm(program)
	}
	s.ShowPassword = true
	s.PasswordInput = ""
	s.PasswordError = ""
	return nil // spinner animation runs on the single tick chain (see onTick)
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
	s.deliverElevation(nil)
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
