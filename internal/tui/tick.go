package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

const spinnerInterval = 120 * time.Millisecond

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// spinnerGlyph returns the current animated spinner character.
func (s *State) spinnerGlyph() string {
	if len(spinnerFrames) == 0 {
		return "⟳"
	}
	return spinnerFrames[s.SpinnerFrame%len(spinnerFrames)]
}

// TickCmd schedules the next spinner frame.
func TickCmd() tea.Cmd {
	return tea.Tick(spinnerInterval, func(time.Time) tea.Msg {
		return TickMsg{}
	})
}

// NeedsSpinner reports whether the UI should animate a spinner.
func (s *State) NeedsSpinner() bool {
	return s.Scanning || s.Updating || s.Cleaning
}

// AdvanceSpinner advances the spinner frame counter.
func (s *State) AdvanceSpinner() {
	s.SpinnerFrame = (s.SpinnerFrame + 1) % len(spinnerFrames)
}
