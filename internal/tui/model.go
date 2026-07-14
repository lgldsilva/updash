// Package tui implements the Bubble Tea TUI for updash.
package tui

import (
	"context"

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
	Scanning    bool
	Updating    bool
	Cleaning    bool
	Ready       bool
	ShowHelp    bool
	ShowConfirm bool // confirmation dialog for destructive actions

	// Confirmation state
	ConfirmMsg    string
	ConfirmAction func()

	// Logs
	Logs []model.GlobalLogEntry

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
		Ready:     false,
	}
}

// FlattenItems returns all items from all summaries as a flat slice.
func (s *State) FlattenItems() []*model.Item {
	var items []*model.Item
	for _, summary := range s.Summaries {
		items = append(items, summary.Items...)
	}
	return items
}

// FlattenCleanItems returns all cleanup items.
func (s *State) FlattenCleanItems() []*model.Item {
	var items []*model.Item
	for _, summary := range s.CleanItems {
		items = append(items, summary.Items...)
	}
	return items
}

// CurrentItems returns the flat list for the active tab.
func (s *State) CurrentItems() []*model.Item {
	if s.ActiveTab == model.TabCleanup {
		return s.FlattenCleanItems()
	}
	return s.FlattenItems()
}

// TotalOutdated returns total outdated/cleanable items.
func (s *State) TotalOutdated() int {
	count := 0
	for _, summary := range s.Summaries {
		count += summary.Outdated
	}
	return count
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
