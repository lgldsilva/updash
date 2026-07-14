package tui

import (
	"context"
	"fmt"

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
	KeySelect
	KeyUpdateSelected
	KeyUpdateAll
	KeyCleanSelected
	KeyCleanAll
	KeyTab
	KeyRefresh
	KeyHelp
	KeyQuit
	KeyConfirm
	KeyCancel
	KeyPasswordSubmit
)

// HandleKey processes a key press and returns an action.
func (s *State) HandleKey(key string) KeyAction {
	if s.ShowPassword {
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

	if s.ShowConfirm {
		switch key {
		case "y", "Y":
			return KeyConfirm
		case "n", "N", "esc":
			s.ShowConfirm = false
			s.ConfirmCmd = nil
			s.PendingUpdateItems = nil
			s.PendingCleanItems = nil
			return KeyCancel
		}
		return KeyNone
	}

	switch key {
	case "up", "k":
		return KeyUp
	case "down", "j":
		return KeyDown
	case " ":
		return KeySelect
	case "u", "U":
		return KeyUpdateSelected
	case "a", "A":
		// A = Update All on Updates tab, Clean All on Cleanup tab
		if s.ActiveTab == model.TabCleanup {
			return KeyCleanAll
		}
		return KeyUpdateAll
	case "c", "C":
		return KeyCleanSelected
	case "1":
		s.ActiveTab = model.TabUpdates
		s.ClampCursor()
		return KeyTab
	case "2":
		s.ActiveTab = model.TabCleanup
		s.ClampCursor()
		return KeyTab
	case "3":
		s.ActiveTab = model.TabLogs
		s.ClampCursor()
		return KeyTab
	case "r", "R":
		return KeyRefresh
	case "?":
		return KeyHelp
	case "q", "Q", "ctrl+c":
		return KeyQuit
	}

	return KeyNone
}

// HandleAction executes a synchronous action and returns an optional async cmd.
// The returned tea.Cmd should be returned from the Bubble Tea Update function.
func (s *State) HandleAction(action KeyAction) tea.Cmd {
	switch action {
	case KeyUp:
		s.moveCursor(-1)
	case KeyDown:
		s.moveCursor(1)
	case KeySelect:
		s.toggleSelection()
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
	items := s.CurrentItems()
	if len(items) == 0 {
		return
	}
	s.Cursor += delta
	if s.Cursor < 0 {
		s.Cursor = 0
	}
	if s.Cursor >= len(items) {
		s.Cursor = len(items) - 1
	}
}

func (s *State) toggleSelection() {
	items := s.CurrentItems()
	if s.Cursor < 0 || s.Cursor >= len(items) {
		return
	}
	item := items[s.Cursor]
	if item.Status == model.StatusOutdated || item.Status == model.StatusCleanCandidate {
		item.Selected = !item.Selected
	}
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

// runUpdateSelected prepares selected items for async update and shows confirm.
func (s *State) runUpdateSelected() {
	items := s.FlattenItems()
	var selected []*model.Item
	for _, it := range items {
		if it.Selected && it.Status == model.StatusOutdated {
			selected = append(selected, it)
		}
	}

	if len(selected) == 0 {
		s.LastSummary = "No items selected — press Space on outdated items first"
		s.AddLog("No items selected for update", false)
		return
	}

	s.ShowConfirm = true
	s.ConfirmMsg = fmt.Sprintf("Update %d selected items?", len(selected))
	s.PendingUpdateItems = selected
	s.ConfirmCmd = func(program *tea.Program) tea.Cmd {
		return s.startUpdateAll(selected, program)
	}
}

// runUpdateAll prepares all outdated items for async update and shows confirm.
func (s *State) runUpdateAll() {
	items := s.FlattenItems()
	var outdated []*model.Item
	for _, it := range items {
		if it.Status == model.StatusOutdated {
			outdated = append(outdated, it)
		}
	}

	if len(outdated) == 0 {
		s.AddLog("Nothing to update", false)
		return
	}

	s.ShowConfirm = true
	s.ConfirmMsg = fmt.Sprintf("Update all %d items?", len(outdated))
	s.PendingUpdateItems = outdated
	s.ConfirmCmd = func(program *tea.Program) tea.Cmd {
		return s.startUpdateAll(outdated, program)
	}
}

// startUpdateAll returns a tea.Cmd that runs updates async.
// Processes each category as a batch and sends progress messages.
func (s *State) startUpdateAll(items []*model.Item, program *tea.Program) tea.Cmd {
	// Prevent double-tap
	if s.Updating {
		return nil
	}

	s.Updating = true
	s.OperationLabel = ""
	s.LastSummary = ""
	s.AddLog(fmt.Sprintf("Starting update of %d items...", len(items)), true)

	// Pre-compute category groups (safe from main thread, not from goroutine)
	groups := groupOutdatedByCategory(s.Summaries, items)

	go func() {
		total := len(items)
		done := 0
		success, failed := 0, 0
		defer func() {
			program.Send(UpdateAllDoneMsg{
				Success: success,
				Failed:  failed,
				Total:   total,
			})
		}()

		for _, group := range groups {
			if len(group.items) == 0 {
				continue
			}

			for _, it := range group.items {
				it.Status = model.StatusUpdating
			}
			catLabel := categoryLabel(s.Summaries, group.category)
			program.Send(UpdateBatchDoneMsg{
				Results:  nil,
				Done:     done,
				Total:    total,
				Category: catLabel,
			})

			batchTimeout := updater.BatchTimeout(group.category)
			cmdCtx, cancel := context.WithTimeout(s.Ctx, batchTimeout)
			needsElev := elevate.CategoryNeedsElevation(group.category, s.Platform)

			if needsElev && group.category == model.CatMAS {
				if err := s.waitForElevation(cmdCtx, program, "Mac App Store updates need your Mac login password"); err != nil {
					cancel()
					done += len(group.items)
					for _, it := range group.items {
						it.Status = model.StatusError
					}
					program.Send(UpdateBatchDoneMsg{
						Results: masElevFailResults(group.items, err),
						Done:    done,
						Total:   total,
					})
					continue
				}
			}

			cmdCtx = elevate.WithSession(cmdCtx, s.ElevSession)
			hasSession := elevate.FromContext(cmdCtx) != nil && elevate.FromContext(cmdCtx).Ready()

			if needsElev && !hasSession && group.category != model.CatMAS {
				program.Send(tea.ExitAltScreen()) //nolint:staticcheck
			}

			opts := updater.SilentOptions()
			opts.Output = newOutputLog(program)
			results := updater.UpdateAllWithOptions(cmdCtx, group.items, opts)
			cancel()

			if needsElev && !hasSession && group.category != model.CatMAS {
				program.Send(tea.EnterAltScreen()) //nolint:staticcheck
			}

			done += len(results)
			for _, r := range results {
				if r.Success {
					success++
				} else {
					failed++
				}
			}
			program.Send(UpdateBatchDoneMsg{
				Results: results,
				Done:    done,
				Total:   total,
			})
		}
	}()

	return TickCmd()
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

// ---------------------------------------------------------------------------
// Cleanup (selected)
// ---------------------------------------------------------------------------

// runCleanSelected prepares selected cleanup items and shows confirm.
func (s *State) runCleanSelected() {
	items := s.FlattenCleanItems()
	var selected []*model.Item
	for _, it := range items {
		if it.Selected && it.Status == model.StatusCleanCandidate {
			selected = append(selected, it)
		}
	}

	if len(selected) == 0 {
		s.LastSummary = "No items selected — press Space on cleanup items first"
		s.AddLog("No items selected for cleanup", false)
		return
	}

	s.ShowConfirm = true
	s.ConfirmMsg = fmt.Sprintf("Clean %d item(s)? This will remove old versions and cache data.", len(selected))
	s.PendingCleanItems = selected
	s.ConfirmCmd = func(program *tea.Program) tea.Cmd {
		return s.startCleanSelected(selected, program)
	}
}

// runCleanAll prepares ALL cleanable items and shows confirm.
func (s *State) runCleanAll() {
	items := s.FlattenCleanItems()
	var all []*model.Item
	for _, it := range items {
		if it.Status == model.StatusCleanCandidate {
			all = append(all, it)
			it.Selected = true
		}
	}

	if len(all) == 0 {
		s.AddLog("Nothing to clean", false)
		return
	}

	s.ShowConfirm = true
	s.ConfirmMsg = fmt.Sprintf("Clean all %d items? This will remove old versions and cache data.", len(all))
	s.PendingCleanItems = all
	s.ConfirmCmd = func(program *tea.Program) tea.Cmd {
		return s.startCleanSelected(all, program)
	}
}

// startCleanSelected returns a tea.Cmd that runs cleanup async.
// Items are processed one-by-one with progress messages.
func (s *State) startCleanSelected(items []*model.Item, program *tea.Program) tea.Cmd {
	// Prevent double-tap
	if s.Cleaning {
		return nil
	}

	s.Cleaning = true
	s.OperationLabel = ""
	s.LastSummary = ""
	s.AddLog(fmt.Sprintf("Starting cleanup of %d items...", len(items)), true)

	go func() {
		total := len(items)
		defer func() {
			program.Send(CleanAllDoneMsg{})
		}()

		for i, it := range items {
			it.Status = model.StatusCleaning
			program.Send(CleanBatchDoneMsg{
				Results:  nil,
				Done:     i,
				Total:    total,
				Category: it.Name,
			})

			cmdCtx, cancel := context.WithTimeout(s.ctxWithElev(), cleaner.ItemTimeout(it))
			needsElev := elevate.ItemNeedsElevation(it)
			hasSession := elevate.FromContext(cmdCtx) != nil && elevate.FromContext(cmdCtx).Ready()

			if needsElev && !hasSession {
				program.Send(tea.ExitAltScreen()) //nolint:staticcheck
			}

			opts := cleaner.SilentOptions()
			opts.Output = newOutputLog(program)
			results := cleaner.CleanAllWithOptions(cmdCtx, []*model.Item{it}, opts)
			cancel()

			if needsElev && !hasSession {
				program.Send(tea.EnterAltScreen()) //nolint:staticcheck
			}

			program.Send(CleanBatchDoneMsg{
				Results: results,
				Done:    i + 1,
				Total:   total,
			})
		}

	}()

	return TickCmd()
}
