package tui

import (
	"fmt"

	"github.com/lgldsilva/updash/internal/cleaner"
	"github.com/lgldsilva/updash/internal/model"
	"github.com/lgldsilva/updash/internal/scanner"
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
	KeyTab
	KeyRefresh
	KeyHelp
	KeyQuit
	KeyConfirm
	KeyCancel
)

// HandleKey processes a key press and updates state.
func (s *State) HandleKey(key string) KeyAction {
	if s.ShowConfirm {
		switch {
		case key == "y" || key == "Y":
			s.ShowConfirm = false
			if s.ConfirmAction != nil {
				s.ConfirmAction()
			}
			return KeyConfirm
		case key == "n" || key == "N" || key == "esc":
			s.ShowConfirm = false
			return KeyCancel
		}
		return KeyNone
	}

	switch {
	case key == "up" || key == "k":
		return KeyUp
	case key == "down" || key == "j":
		return KeyDown
	case key == " ":
		return KeySelect
	case key == "u" || key == "U":
		return KeyUpdateSelected
	case key == "a" || key == "A":
		return KeyUpdateAll
	case key == "c" || key == "C":
		return KeyCleanSelected
	case key == "1":
		s.ActiveTab = model.TabUpdates
		s.Cursor = 0
		return KeyTab
	case key == "2":
		s.ActiveTab = model.TabCleanup
		s.Cursor = 0
		return KeyTab
	case key == "3":
		s.ActiveTab = model.TabLogs
		s.Cursor = 0
		return KeyTab
	case key == "r" || key == "R":
		return KeyRefresh
	case key == "?":
		return KeyHelp
	case key == "q" || key == "Q" || key == "ctrl+c":
		return KeyQuit
	}

	return KeyNone
}

// HandleAction executes an action and returns whether to redraw.
func (s *State) HandleAction(action KeyAction) {
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
	case KeyRefresh:
		s.runRefresh()
	case KeyHelp:
		s.ShowHelp = !s.ShowHelp
	}
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
	// Only toggle if the item is actionable
	if item.Status == model.StatusOutdated || item.Status == model.StatusCleanCandidate {
		item.Selected = !item.Selected
	}
}

func (s *State) runUpdateSelected() {
	items := s.FlattenItems()
	var selected []*model.Item
	for _, it := range items {
		if it.Selected && it.Status == model.StatusOutdated {
			selected = append(selected, it)
		}
	}

	if len(selected) == 0 {
		s.AddLog("No items selected for update", false)
		return
	}

	s.Updating = true
	s.AddLog(fmt.Sprintf("Updating %d selected items...", len(selected)), true)

	results := updater.UpdateAll(s.Ctx, selected)
	s.processUpdateResults(results)
	s.Updating = false
}

func (s *State) runUpdateAll() {
	items := s.FlattenItems()
	var allOutdated []*model.Item
	for _, it := range items {
		if it.Status == model.StatusOutdated {
			allOutdated = append(allOutdated, it)
		}
	}

	if len(allOutdated) == 0 {
		s.AddLog("Nothing to update", false)
		return
	}

	// Confirmation dialog
	s.ShowConfirm = true
	s.ConfirmMsg = fmt.Sprintf("Update all %d items?", len(allOutdated))
	s.ConfirmAction = func() {
		s.Updating = true
		s.AddLog(fmt.Sprintf("Updating all %d items...", len(allOutdated)), true)
		results := updater.UpdateAll(s.Ctx, allOutdated)
		s.processUpdateResults(results)
		s.Updating = false
	}
}

func (s *State) runCleanSelected() {
	items := s.FlattenCleanItems()
	var selected []*model.Item
	for _, it := range items {
		if it.Selected && it.Status == model.StatusCleanCandidate {
			selected = append(selected, it)
		}
	}

	if len(selected) == 0 {
		s.AddLog("No items selected for cleanup", false)
		return
	}

	// Confirmation dialog
	s.ShowConfirm = true
	s.ConfirmMsg = fmt.Sprintf("Clean %d item(s)? This will remove old versions and cache data.", len(selected))
	s.ConfirmAction = func() {
		s.Cleaning = true
		s.AddLog(fmt.Sprintf("Cleaning %d items...", len(selected)), true)
		results := cleaner.CleanAll(s.Ctx, selected)
		s.processCleanResults(results)
		s.Cleaning = false
	}
}

func (s *State) processUpdateResults(results []*updater.Result) {
	for _, r := range results {
		if r.Success {
			s.AddLog(fmt.Sprintf("✓ %s: updated", r.Item.Name), true)
		} else {
			s.AddLog(fmt.Sprintf("✘ %s: %s", r.Item.Name, r.Error), false)
		}
	}
}

func (s *State) processCleanResults(results []*cleaner.Result) {
	for _, r := range results {
		if r.Success {
			s.AddLog(fmt.Sprintf("✓ %s: cleaned", r.Item.Name), true)
		} else {
			s.AddLog(fmt.Sprintf("✘ %s: %s", r.Item.Name, r.Error), false)
		}
	}
}

func (s *State) runRefresh() {
	s.Scanning = true
	s.Ready = false

	go func() {
		// Run scan
		summaries := scanner.RunAll(s.Ctx, s.Platform, false)
		cleanSummaries := scanner.RunAll(s.Ctx, s.Platform, true)

		// Update state
		s.Summaries = summaries
		s.CleanItems = cleanSummaries
		s.Scanning = false
		s.Ready = true
		s.Cursor = 0

		s.AddLog(fmt.Sprintf("Scan complete: %d outdated, %d cleanable",
			s.TotalOutdated(), s.TotalCleanable()), true)
	}()
}
