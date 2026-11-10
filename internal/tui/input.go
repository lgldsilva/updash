package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/lgldsilva/updash/internal/model"
)

// HandleKey processes a key press and returns an action.
func (s *State) HandleKey(key string) KeyAction {
	// Ctrl+C always quits, even with a dialog/overlay open — otherwise the
	// single-char handlers below would swallow it and trap the user.
	if key == "ctrl+c" {
		return KeyQuit
	}
	if action, handled := s.handleOverlayKey(key); handled {
		return action
	}
	if action, handled := s.handleContextKey(key); handled {
		return action
	}
	if tab, ok := tabKeys[key]; ok {
		s.ActiveTab = tab
		s.ClampCursor()
		return KeyTab
	}
	if action, ok := keyActions[key]; ok && s.actionAllowedOnTab(action) {
		return action
	}
	return KeyNone
}

// handleOverlayKey routes keys to whichever overlay owns input. An open
// overlay consumes every key, so handled is true whenever one is showing.
func (s *State) handleOverlayKey(key string) (action KeyAction, handled bool) {
	switch {
	case s.ShowHelp:
		// Any key dismisses the help overlay; Esc is the explicit close.
		return KeyHelp, true
	case s.ShowDetail:
		// Enter/Esc close the detail view; other keys are ignored.
		if key == "enter" || key == "esc" {
			return KeyDetail, true
		}
		return KeyNone, true
	case s.ShowFilter:
		return s.handleFilterKey(key), true
	case s.ShowPassword:
		return s.handlePasswordKey(key), true
	case s.ShowConfirm:
		return s.handleConfirmKey(key), true
	}
	return KeyNone, false
}

// handleContextKey resolves keys whose meaning depends on state: Esc cancels
// a running operation or clears the filter, and A is Update All on Updates,
// Clean All on Cleanup and a no-op on Logs.
func (s *State) handleContextKey(key string) (action KeyAction, handled bool) {
	switch key {
	case "esc":
		if s.Scanning || s.Updating || s.Cleaning {
			return KeyCancel, true
		}
		if s.AppliedFilter != "" {
			return KeyFilterCancel, true
		}
	case "a", "A":
		switch s.ActiveTab {
		case model.TabCleanup:
			return KeyCleanAll, true
		case model.TabUpdates:
			return KeyUpdateAll, true
		default:
			return KeyNone, true
		}
	}
	return KeyNone, false
}

// actionAllowedOnTab reports whether the action applies to the active tab.
// Update keys belong to Updates, clean keys to Cleanup, and item
// selection/detail make no sense on Logs — acting on invisible items from
// another tab is never what the user meant.
func (s *State) actionAllowedOnTab(action KeyAction) bool {
	switch s.ActiveTab {
	case model.TabLogs:
		switch action {
		case KeyUpdateSelected, KeyUpdateAll, KeyCleanSelected, KeyCleanAll,
			KeySelect, KeySelectAll, KeySelectNone, KeySelectCategory, KeyDetail:
			return false
		}
	case model.TabUpdates:
		if action == KeyCleanSelected || action == KeyCleanAll {
			return false
		}
	case model.TabCleanup:
		if action == KeyUpdateSelected {
			return false
		}
	}
	return true
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
		s.PendingUpdateBatches = nil
		s.PendingUpdateRequiresElevation = false
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
	case KeyRefresh:
		return s.runRefresh()
	case KeyPasswordSubmit:
		return s.submitPassword()
	}
	if run, ok := syncActions[action]; ok {
		run(s)
	}
	return nil
}

// syncActions are the actions that only mutate State (no async tea.Cmd).
var syncActions = map[KeyAction]func(*State){
	KeyUp:             func(s *State) { s.moveCursor(-1) },
	KeyDown:           func(s *State) { s.moveCursor(1) },
	KeyPageUp:         func(s *State) { s.moveCursor(-s.pageSize()) },
	KeyPageDown:       func(s *State) { s.moveCursor(s.pageSize()) },
	KeyHome:           func(s *State) { s.moveCursorTo(0) },
	KeyEnd:            func(s *State) { s.moveCursorTo(len(s.CurrentItems()) - 1) },
	KeySelect:         (*State).toggleSelection,
	KeySelectAll:      func(s *State) { s.setSelectionAll(true) },
	KeySelectNone:     func(s *State) { s.setSelectionAll(false) },
	KeySelectCategory: func(s *State) { s.setSelectionCategory(true) },
	KeyDetail:         (*State).toggleDetail,
	KeyUpdateSelected: unlessBusy("Update", (*State).runUpdateSelected),
	KeyUpdateAll:      unlessBusy("Update all", (*State).runUpdateAll),
	KeyCleanSelected:  unlessBusy("Clean", (*State).runCleanSelected),
	KeyCleanAll:       unlessBusy("Clean all", (*State).runCleanAll),
	KeyHelp:           func(s *State) { s.ShowHelp = !s.ShowHelp },
	KeyFilter:         (*State).toggleFilter,
	KeyFilterSubmit:   (*State).applyFilter,
	KeyFilterCancel:   (*State).clearFilter,
	KeyCancel:         (*State).cancelIfBusy,
}

// unlessBusy runs an update/clean action only when no operation is running;
// refuseBusy reports the refusal in the status line.
func unlessBusy(label string, run func(*State)) func(*State) {
	return func(s *State) {
		if !s.refuseBusy(label) {
			run(s)
		}
	}
}

func (s *State) toggleFilter() {
	s.ShowFilter = !s.ShowFilter
	if s.ShowFilter {
		s.FilterInput = s.AppliedFilter
	}
}

func (s *State) applyFilter() {
	s.AppliedFilter = s.FilterInput
	s.ShowFilter = false
	s.Cursor = 0
	s.ClampCursor()
}

func (s *State) clearFilter() {
	s.FilterInput = ""
	s.AppliedFilter = ""
	s.ShowFilter = false
	s.Cursor = 0
	s.ClampCursor()
}

func (s *State) cancelIfBusy() {
	if s.Scanning || s.Updating || s.Cleaning {
		s.cancelOperation()
	}
}
