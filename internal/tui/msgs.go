// Package tui defines Bubble Tea message types for async operations.
package tui

import (
	"time"

	"github.com/lgldsilva/updash/internal/cleaner"
	"github.com/lgldsilva/updash/internal/elevate"
	"github.com/lgldsilva/updash/internal/model"
	"github.com/lgldsilva/updash/internal/updater"
)

// ScanSourceDoneMsg is sent as each package-manager source finishes scanning.
// Rescan marks summaries from post-update/clean re-probes: they must not
// advance the scan progress counter.
type ScanSourceDoneMsg struct {
	Generation uint64
	Summary    *model.SourceSummary
	IsCleanup  bool
	Rescan     bool
}

// ScanFinishedMsg is sent when all sources have been scanned.
type ScanFinishedMsg struct {
	Generation uint64
	Elapsed    time.Duration
}

// UpdateBatchDoneMsg is sent for each category batch during update.
// Results is nil when the batch hasn't finished yet (progress notification
// only; Source/Items then identify the items entering the batch so the event loop
// can mark them Updating).
type UpdateBatchDoneMsg struct {
	Generation uint64
	Results    []*updater.Result
	Done       int    // accumulated completed count
	Total      int    // total items to update
	Category   string // category label for debug logging (set on start-of-batch only)
	Source     SourceIdentity
	Items      []ItemIdentity // exact operational targets for this batch
}

// UpdateAllDoneMsg is sent when all updates complete.
type UpdateAllDoneMsg struct {
	Generation uint64
	Success    int
	Failed     int
	Total      int
}

// CleanBatchDoneMsg is sent for each category batch during cleanup.
// Results is nil when the batch hasn't finished yet (progress notification
// only; Source/Items then identify the item entering the batch so the event loop
// can mark it Cleaning).
type CleanBatchDoneMsg struct {
	Generation uint64
	Results    []*cleaner.Result
	Done       int
	Total      int
	Category   string // category label for debug logging (set on start-of-batch only)
	Source     SourceIdentity
	Items      []ItemIdentity // exact operational targets for this batch
}

// CleanAllDoneMsg is sent when all cleanup completes.
type CleanAllDoneMsg struct {
	Generation uint64
	BytesFreed int64
	Failed     int // items whose cleanup failed
}

// OutputLineMsg carries a line of subprocess output for the Logs tab.
type OutputLineMsg struct {
	Generation uint64
	Line       string
}

// ElevRequiredMsg requests a sudo password mid-operation (e.g. before MAS
// batch). Wait receives the validated session (or nil when cancelled); the
// channel handoff keeps the worker off State fields entirely.
type ElevRequiredMsg struct {
	Generation uint64
	Reason     string
	Wait       chan *elevate.Session
}

// PasswordResultMsg is sent after validating a sudo password.
type PasswordResultMsg struct {
	Generation uint64
	OK         bool
	Error      string
	Session    *elevate.Session
}

// PasswordlessResultMsg is sent after the async sudo -n probe that decides
// whether a confirmation can proceed without a password dialog.
type PasswordlessResultMsg struct {
	Generation uint64
	OK         bool
}

// TickMsg drives spinner animation while async work runs.
type TickMsg struct{}
