package tui

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lgldsilva/updash/internal/cleaner"
	"github.com/lgldsilva/updash/internal/elevate"
	"github.com/lgldsilva/updash/internal/model"
	"github.com/lgldsilva/updash/internal/updater"
)

// KeyAction handles a key press and returns a command.
type KeyAction int

const (
	KeyNone KeyAction = iota
	KeyUp
	KeyDown
	KeyPageUp
	KeyPageDown
	KeyHome
	KeyEnd
	KeySelect
	KeySelectAll
	KeySelectNone
	KeySelectCategory
	KeyDetail
	KeyUpdateSelected
	KeyUpdateAll
	KeyCleanSelected
	KeyCleanAll
	KeyTab
	KeyRefresh
	KeyHelp
	KeyFilter
	KeyFilterSubmit
	KeyFilterCancel
	KeyQuit
	KeyConfirm
	KeyCancel
	KeyPasswordSubmit
)

var keyActions = map[string]KeyAction{
	"up": KeyUp, "k": KeyUp,
	"down": KeyDown, "j": KeyDown,
	"pgup":   KeyPageUp,
	"pgdown": KeyPageDown,
	"home":   KeyHome,
	"end":    KeyEnd,
	" ":      KeySelect,
	"*":      KeySelectAll,
	"-":      KeySelectNone,
	".":      KeySelectCategory,
	"enter":  KeyDetail,
	"u":      KeyUpdateSelected, "U": KeyUpdateSelected,
	"c": KeyCleanSelected, "C": KeyCleanSelected,
	"r": KeyRefresh, "R": KeyRefresh,
	"?": KeyHelp,
	"/": KeyFilter,
	"q": KeyQuit, "Q": KeyQuit, "ctrl+c": KeyQuit,
}

var tabKeys = map[string]model.TabID{
	"1": model.TabUpdates,
	"2": model.TabCleanup,
	"3": model.TabLogs,
}

// HandleKey processes a key press and returns an action.
func (s *State) HandleKey(key string) KeyAction {
	if s.ShowHelp {
		// Any key dismisses the help overlay; Esc is the explicit close.
		return KeyHelp
	}
	if s.ShowDetail {
		// Enter/Esc close the detail view; other keys are ignored.
		if key == "enter" || key == "esc" {
			return KeyDetail
		}
		return KeyNone
	}
	if s.ShowFilter {
		return s.handleFilterKey(key)
	}
	if s.ShowPassword {
		return s.handlePasswordKey(key)
	}
	if s.ShowConfirm {
		return s.handleConfirmKey(key)
	}
	if key == "esc" && (s.Scanning || s.Updating || s.Cleaning) {
		return KeyCancel
	}
	if key == "esc" && s.AppliedFilter != "" {
		return KeyFilterCancel
	}
	if key == "?" {
		return KeyHelp
	}
	if key == "/" {
		return KeyFilter
	}
	if key == "a" || key == "A" {
		// A = Update All on Updates tab, Clean All on Cleanup tab.
		if s.ActiveTab == model.TabCleanup {
			return KeyCleanAll
		}
		return KeyUpdateAll
	}
	if tab, ok := tabKeys[key]; ok {
		s.ActiveTab = tab
		s.ClampCursor()
		return KeyTab
	}
	if action, ok := keyActions[key]; ok {
		return action
	}
	return KeyNone
}

func (s *State) handlePasswordKey(key string) KeyAction {
	switch key {
	case "enter":
		return KeyPasswordSubmit
	case "esc":
		s.CancelPassword()
		return KeyCancel
	case "backspace":
		if len(s.PasswordInput) > 0 {
			s.PasswordInput = s.PasswordInput[:len(s.PasswordInput)-1]
		}
		return KeyNone
	default:
		if len(key) == 1 {
			s.PasswordInput += key
		}
		return KeyNone
	}
}

func (s *State) handleConfirmKey(key string) KeyAction {
	switch key {
	case "y", "Y":
		return KeyConfirm
	case "n", "N", "esc":
		s.ShowConfirm = false
		s.ConfirmCmd = nil
		s.PendingUpdateItems = nil
		s.PendingCleanItems = nil
		return KeyCancel
	default:
		return KeyNone
	}
}

func (s *State) handleFilterKey(key string) KeyAction {
	switch key {
	case "enter":
		return KeyFilterSubmit
	case "esc":
		return KeyFilterCancel
	case "backspace":
		if len(s.FilterInput) > 0 {
			s.FilterInput = s.FilterInput[:len(s.FilterInput)-1]
		}
		return KeyNone
	default:
		if len(key) == 1 {
			s.FilterInput += key
		}
		return KeyNone
	}
}

// HandleAction executes a synchronous action and returns an optional async cmd.
// The returned tea.Cmd should be returned from the Bubble Tea Update function.
func (s *State) HandleAction(action KeyAction) tea.Cmd {
	switch action {
	case KeyUp:
		s.moveCursor(-1)
	case KeyDown:
		s.moveCursor(1)
	case KeyPageUp:
		s.moveCursor(-s.pageSize())
	case KeyPageDown:
		s.moveCursor(s.pageSize())
	case KeyHome:
		s.moveCursorTo(0)
	case KeyEnd:
		s.moveCursorTo(len(s.CurrentItems()) - 1)
	case KeySelect:
		s.toggleSelection()
	case KeySelectAll:
		s.setSelectionAll(true)
	case KeySelectNone:
		s.setSelectionAll(false)
	case KeySelectCategory:
		s.setSelectionCategory(true)
	case KeyDetail:
		s.toggleDetail()
	case KeyUpdateSelected:
		s.runUpdateSelected()
	case KeyUpdateAll:
		s.runUpdateAll()
	case KeyCleanSelected:
		s.runCleanSelected()
	case KeyCleanAll:
		s.runCleanAll()
	case KeyRefresh:
		return s.runRefresh()
	case KeyHelp:
		s.ShowHelp = !s.ShowHelp
	case KeyFilter:
		s.ShowFilter = !s.ShowFilter
		if s.ShowFilter {
			s.FilterInput = s.AppliedFilter
		}
	case KeyFilterSubmit:
		s.AppliedFilter = s.FilterInput
		s.ShowFilter = false
		s.Cursor = 0
		s.ClampCursor()
	case KeyFilterCancel:
		s.FilterInput = ""
		s.AppliedFilter = ""
		s.ShowFilter = false
		s.Cursor = 0
		s.ClampCursor()
	case KeyCancel:
		if s.Scanning || s.Updating || s.Cleaning {
			s.cancelOperation()
		}
	case KeyPasswordSubmit:
		return s.submitPassword()
	}
	return nil
}

// submitPassword validates the sudo password without blocking the event loop.
func (s *State) submitPassword() tea.Cmd {
	pw := s.PasswordInput
	s.PasswordInput = ""
	program := s.Program
	ctx := s.Ctx

	if program == nil {
		return nil
	}

	go func() {
		sess := elevate.NewSession()
		if err := sess.Validate(ctx, pw); err != nil {
			program.Send(PasswordResultMsg{OK: false, Error: err.Error()})
			return
		}
		program.Send(PasswordResultMsg{OK: true, Session: sess})
	}()

	return TickCmd()
}

func (s *State) moveCursor(delta int) {
	s.moveCursorTo(s.Cursor + delta)
}

func (s *State) moveCursorTo(pos int) {
	items := s.CurrentItems()
	if len(items) == 0 {
		s.Cursor = 0
		return
	}
	if pos < 0 {
		pos = 0
	}
	if pos >= len(items) {
		pos = len(items) - 1
	}
	s.Cursor = pos
}

func (s *State) pageSize() int {
	n := s.maxListLines() - 2
	if n < 3 {
		return 3
	}
	return n
}

func isSelectableStatus(status model.Status) bool {
	return status == model.StatusOutdated || status == model.StatusCleanCandidate
}

func (s *State) toggleSelection() {
	items := s.CurrentItems()
	if s.Cursor < 0 || s.Cursor >= len(items) {
		return
	}
	item := items[s.Cursor]
	if isSelectableStatus(item.Status) {
		item.Selected = !item.Selected
	}
}

func (s *State) setSelectionAll(selected bool) {
	for _, it := range s.CurrentItems() {
		if isSelectableStatus(it.Status) {
			it.Selected = selected
		}
	}
}

func (s *State) setSelectionCategory(selected bool) {
	items := s.CurrentItems()
	if s.Cursor < 0 || s.Cursor >= len(items) {
		return
	}
	cat := items[s.Cursor].Category
	for _, it := range items {
		if it.Category == cat && isSelectableStatus(it.Status) {
			it.Selected = selected
		}
	}
}

func (s *State) toggleDetail() {
	if s.ShowDetail {
		s.ShowDetail = false
		s.DetailItem = nil
		return
	}
	items := s.CurrentItems()
	if s.Cursor < 0 || s.Cursor >= len(items) {
		return
	}
	s.DetailItem = items[s.Cursor]
	s.ShowDetail = true
}

// cancelOperation cancels the current scan/update/clean context and resets
// the running flags so the UI stops showing spinners.
func (s *State) cancelOperation() {
	if s.Cancel != nil {
		s.Cancel()
	}
	s.Scanning = false
	s.Updating = false
	s.Cleaning = false
	s.OperationLabel = ""
	s.LastSummary = "Cancelled"
	s.AddLog("Operation cancelled by user", false)

	// Recreate context for subsequent operations.
	ctx, cancel := context.WithCancel(context.Background())
	s.Ctx = ctx
	s.Cancel = cancel
}

// ---------------------------------------------------------------------------
// Scan (refresh)
// ---------------------------------------------------------------------------

// runRefresh starts a non-blocking background scan (see scan.go).
func (s *State) runRefresh() tea.Cmd {
	return s.startScan()
}

// ---------------------------------------------------------------------------
// Update (selected + all)
// ---------------------------------------------------------------------------

// collectOutdatedItems returns all outdated items, or only selected ones when selectedOnly is true.
func (s *State) collectOutdatedItems(selectedOnly bool) []*model.Item {
	var out []*model.Item
	for _, it := range s.FlattenItems() {
		if it.Status != model.StatusOutdated {
			continue
		}
		if selectedOnly && !it.Selected {
			continue
		}
		out = append(out, it)
	}
	return out
}

// showUpdateConfirm prepares the confirmation dialog for an update batch.
func (s *State) showUpdateConfirm(items []*model.Item) {
	s.ShowConfirm = true
	s.ConfirmMsg = confirmMessage("Update", items)
	s.PendingUpdateItems = items
	s.ConfirmCmd = func(program *tea.Program) tea.Cmd {
		return s.startUpdateAll(items, program)
	}
}

// runUpdateSelected prepares selected items for async update and shows confirm.
func (s *State) runUpdateSelected() {
	selected := s.collectOutdatedItems(true)
	if len(selected) == 0 {
		s.LastSummary = "No items selected — press Space on outdated items first"
		s.AddLog("No items selected for update", false)
		return
	}
	s.showUpdateConfirm(selected)
}

// runUpdateAll prepares all outdated items for async update and shows confirm.
func (s *State) runUpdateAll() {
	outdated := s.collectOutdatedItems(false)
	if len(outdated) == 0 {
		s.AddLog("Nothing to update", false)
		return
	}
	s.showUpdateConfirm(outdated)
}

// startUpdateAll returns a tea.Cmd that runs updates async.
// Processes each category as a batch and sends progress messages.
func (s *State) startUpdateAll(items []*model.Item, program *tea.Program) tea.Cmd {
	if s.Updating {
		return nil
	}
	s.Updating = true
	s.OperationLabel = ""
	s.LastSummary = ""
	s.AddLog(fmt.Sprintf("Starting update of %d items...", len(items)), true)
	groups := groupOutdatedByCategory(s.Summaries, items)

	go func() {
		total := len(items)
		done, success, failed := 0, 0, 0
		defer func() {
			program.Send(UpdateAllDoneMsg{Success: success, Failed: failed, Total: total})
		}()
		for _, group := range groups {
			if len(group.items) == 0 {
				continue
			}
			ok, fail, n := s.runUpdateGroup(group, done, total, program)
			success += ok
			failed += fail
			done += n
			s.rescanCategory(s.Ctx, program, group.category, false)
		}
	}()
	return TickCmd()
}

func (s *State) runUpdateGroup(group categoryGroup, done, total int, program *tea.Program) (ok, fail, n int) {
	for _, it := range group.items {
		it.Status = model.StatusUpdating
	}
	program.Send(UpdateBatchDoneMsg{
		Results: nil, Done: done, Total: total,
		Category: categoryLabel(s.Summaries, group.category),
	})

	cmdCtx, cancel := context.WithTimeout(s.Ctx, updater.BatchTimeout(group.category))
	defer cancel()

	if results, skipped := s.tryMASElevation(group, cmdCtx, program); skipped {
		done += len(group.items)
		program.Send(UpdateBatchDoneMsg{Results: results, Done: done, Total: total})
		return 0, len(group.items), len(group.items)
	}

	results := s.execUpdateBatch(group, cmdCtx, program)
	for _, r := range results {
		if r.Success {
			ok++
		} else {
			fail++
		}
	}
	program.Send(UpdateBatchDoneMsg{Results: results, Done: done + len(results), Total: total})
	return ok, fail, len(results)
}

func (s *State) tryMASElevation(group categoryGroup, cmdCtx context.Context, program *tea.Program) (results []*updater.Result, skipped bool) {
	needsElev := elevate.CategoryNeedsElevation(group.category, s.Platform)
	if !needsElev || group.category != model.CatMAS {
		return nil, false
	}
	if err := s.waitForElevation(cmdCtx, program, "Mac App Store updates need your Mac login password"); err != nil {
		for _, it := range group.items {
			it.Status = model.StatusError
		}
		return masElevFailResults(group.items, err), true
	}
	return nil, false
}

func (s *State) execUpdateBatch(group categoryGroup, cmdCtx context.Context, program *tea.Program) []*updater.Result {
	needsElev := elevate.CategoryNeedsElevation(group.category, s.Platform)
	cmdCtx = elevate.WithSession(cmdCtx, s.ElevSession)
	hasSession := elevate.FromContext(cmdCtx) != nil && elevate.FromContext(cmdCtx).Ready()
	leaveAlt := needsElev && !hasSession && group.category != model.CatMAS
	if leaveAlt {
		program.Send(tea.ExitAltScreen()) //nolint:staticcheck
	}
	opts := updater.SilentOptions()
	opts.Output = newOutputLog(program)
	results := updater.UpdateAllWithOptions(cmdCtx, group.items, opts)
	if leaveAlt {
		program.Send(tea.EnterAltScreen()) //nolint:staticcheck
	}
	return results
}

// categoryGroup holds items grouped by category.
type categoryGroup struct {
	category model.Category
	items    []*model.Item
}

// categoryLabel returns the human-readable label for a category from summaries.
func categoryLabel(summaries []*model.SourceSummary, cat model.Category) string {
	for _, s := range summaries {
		if s.Category == cat {
			return s.Label
		}
	}
	return string(cat)
}

// groupOutdatedByCategory groups items by their source category, using the
// summaries structure so the goroutine doesn't need to read State directly.
func groupOutdatedByCategory(summaries []*model.SourceSummary, items []*model.Item) []categoryGroup {
	// Build fast lookup
	need := make(map[*model.Item]bool, len(items))
	for _, it := range items {
		need[it] = true
	}

	var groups []categoryGroup
	for _, summary := range summaries {
		var groupItems []*model.Item
		for _, it := range summary.Items {
			if need[it] {
				groupItems = append(groupItems, it)
			}
		}
		if len(groupItems) > 0 {
			groups = append(groups, categoryGroup{
				category: summary.Category,
				items:    groupItems,
			})
		}
	}
	return groups
}

func masElevFailResults(items []*model.Item, err error) []*updater.Result {
	msg := err.Error()
	results := make([]*updater.Result, len(items))
	for i, it := range items {
		results[i] = &updater.Result{Item: it, Success: false, Error: msg}
	}
	return results
}

// confirmMessage builds a confirmation dialog text that lists the first
// few items so the user knows exactly what will be touched.
func confirmMessage(verb string, items []*model.Item) string {
	if len(items) == 0 {
		return fmt.Sprintf("%s 0 items?", verb)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s %d item(s)?\n\n", verb, len(items))
	const maxVisible = 5
	for i, it := range items {
		if i >= maxVisible {
			fmt.Fprintf(&b, "… and %d more\n", len(items)-maxVisible)
			break
		}
		fmt.Fprintf(&b, "  • %s  (%s)\n", it.Name, it.Category)
	}
	return strings.TrimSpace(b.String())
}

// ---------------------------------------------------------------------------
// Cleanup (selected)
// ---------------------------------------------------------------------------

// collectCleanCandidates returns all cleanable items, selecting them when selectAll is true.
func (s *State) collectCleanCandidates(selectAll bool) []*model.Item {
	var out []*model.Item
	for _, it := range s.FlattenCleanItems() {
		if it.Status != model.StatusCleanCandidate {
			continue
		}
		if selectAll {
			it.Selected = true
		}
		out = append(out, it)
	}
	return out
}

// showCleanConfirm prepares the confirmation dialog for a cleanup batch.
func (s *State) showCleanConfirm(items []*model.Item) {
	s.ShowConfirm = true
	s.ConfirmMsg = confirmMessage("Clean", items)
	s.PendingCleanItems = items
	s.ConfirmCmd = func(program *tea.Program) tea.Cmd {
		return s.startCleanSelected(items, program)
	}
}

// runCleanSelected prepares selected cleanup items and shows confirm.
func (s *State) runCleanSelected() {
	selected := s.collectCleanCandidates(false)
	var matched []*model.Item
	for _, it := range selected {
		if it.Selected {
			matched = append(matched, it)
		}
	}
	if len(matched) == 0 {
		s.LastSummary = "No items selected — press Space on cleanup items first"
		s.AddLog("No items selected for cleanup", false)
		return
	}
	s.showCleanConfirm(matched)
}

// runCleanAll prepares ALL cleanable items and shows confirm.
func (s *State) runCleanAll() {
	all := s.collectCleanCandidates(true)
	if len(all) == 0 {
		s.AddLog("Nothing to clean", false)
		return
	}
	s.showCleanConfirm(all)
}

// startCleanSelected returns a tea.Cmd that runs cleanup async.
// Items are processed one-by-one with progress messages.
func (s *State) startCleanSelected(items []*model.Item, program *tea.Program) tea.Cmd {
	if s.Cleaning {
		return nil
	}
	s.Cleaning = true
	s.OperationLabel = ""
	s.LastSummary = ""
	s.AddLog(fmt.Sprintf("Starting cleanup of %d items...", len(items)), true)

	go func() {
		total := len(items)
		var totalFreed int64
		defer func() {
			program.Send(CleanAllDoneMsg{BytesFreed: totalFreed})
		}()
		for i, it := range items {
			totalFreed += s.cleanOneItem(it, i, total, program)
			s.rescanCategory(s.Ctx, program, it.Category, true)
		}
	}()
	return TickCmd()
}

func (s *State) cleanOneItem(it *model.Item, i, total int, program *tea.Program) int64 {
	it.Status = model.StatusCleaning
	program.Send(CleanBatchDoneMsg{
		Results: nil, Done: i, Total: total, Category: it.Name,
	})

	cmdCtx, cancel := context.WithTimeout(s.ctxWithElev(), cleaner.ItemTimeout(it))
	needsElev := elevate.ItemNeedsElevation(it)
	hasSession := elevate.FromContext(cmdCtx) != nil && elevate.FromContext(cmdCtx).Ready()
	leaveAlt := needsElev && !hasSession
	if leaveAlt {
		program.Send(tea.ExitAltScreen()) //nolint:staticcheck
	}
	opts := cleaner.SilentOptions()
	opts.Output = newOutputLog(program)
	results := cleaner.CleanAllWithOptions(cmdCtx, []*model.Item{it}, opts)
	cancel()
	if leaveAlt {
		program.Send(tea.EnterAltScreen()) //nolint:staticcheck
	}

	var freed int64
	for _, r := range results {
		if r.Success {
			freed += r.BytesFreed
		}
	}
	program.Send(CleanBatchDoneMsg{Results: results, Done: i + 1, Total: total})
	return freed
}
