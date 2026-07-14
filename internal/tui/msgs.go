// Package tui defines Bubble Tea message types for async operations.
package tui

import (
	"time"

	"github.com/lgldsilva/updash/internal/cleaner"
	"github.com/lgldsilva/updash/internal/elevate"
	"github.com/lgldsilva/updash/internal/model"
	"github.com/lgldsilva/updash/internal/updater"
)

// ScanResultMsg is sent when a background scan completes (legacy batch result).
type ScanResultMsg struct {
	Summaries      []*model.SourceSummary
	CleanSummaries []*model.SourceSummary
	Elapsed        time.Duration
}

// ScanSourceDoneMsg is sent as each package-manager source finishes scanning.
type ScanSourceDoneMsg struct {
	Summary   *model.SourceSummary
	IsCleanup bool
}

// ScanFinishedMsg is sent when all sources have been scanned.
type ScanFinishedMsg struct {
	Elapsed time.Duration
}

// ErrMsg is sent when a background operation fails.
type ErrMsg struct {
	Error error
}

// UpdateBatchDoneMsg is sent for each category batch during update.
// Results is nil when the batch hasn't finished yet (progress notification only).
type UpdateBatchDoneMsg struct {
	Results  []*updater.Result
	Done     int    // accumulated completed count
	Total    int    // total items to update
	Category string // category label for debug logging (set on start-of-batch only)
}

// UpdateAllDoneMsg is sent when all updates complete.
type UpdateAllDoneMsg struct {
	Success int
	Failed  int
	Total   int
}

// CleanBatchDoneMsg is sent for each category batch during cleanup.
// Results is nil when the batch hasn't finished yet (progress notification only).
type CleanBatchDoneMsg struct {
	Results  []*cleaner.Result
	Done     int
	Total    int
	Category string // category label for debug logging (set on start-of-batch only)
}

// CleanAllDoneMsg is sent when all cleanup completes.
type CleanAllDoneMsg struct{}

// OutputLineMsg carries a line of subprocess output for the Logs tab.
type OutputLineMsg struct {
	Line string
}

// ElevRequiredMsg requests a sudo password mid-operation (e.g. before MAS batch).
type ElevRequiredMsg struct {
	Reason string
}

// PasswordResultMsg is sent after validating a sudo password.
type PasswordResultMsg struct {
	OK      bool
	Error   string
	Session *elevate.Session
}

// TickMsg drives spinner animation while async work runs.
type TickMsg struct{}
