// Package model defines the core data types for updash.
package model

// Category groups update/cleanup sources.
type Category string

const (
	CatBrew    Category = "brew"
	CatMAS     Category = "mas"
	CatApt     Category = "apt"
	CatPacman  Category = "pacman"
	CatFlatpak Category = "flatpak"
	CatSnap    Category = "snap"

	// Windows package managers
	CatWinget Category = "winget"
	CatChoco  Category = "choco"
	CatScoop  Category = "scoop"

	CatNpm         Category = "npm"
	CatPipx        Category = "pipx"
	CatGo          Category = "go"
	CatRustup      Category = "rustup"
	CatCargo       Category = "cargo"
	CatSDKMAN      Category = "sdkman"
	CatDocker      Category = "docker"
	CatWatchtower  Category = "watchtower"
	CatCloud       Category = "cloud" // gcloud, etc.
	CatAI          Category = "ai"    // ai-memory, semidx, ai-standards
	CatAgent       Category = "agent" // claude, opencode, grok, agy...
	CatGHExt       Category = "gh-ext"
	CatNvm         Category = "nvm"
	CatOmz         Category = "oh-my-zsh"
	CatCache       Category = "cache"
	CatSDKClean    Category = "sdkman-clean"
	CatVSCodeClean Category = "vscode-clean"
	CatDockerClean Category = "docker-clean"
)

// Status represents the current state of an item.
type Status int

const (
	StatusPending        Status = iota // not yet checked
	StatusOK                           // up to date
	StatusOutdated                     // update available
	StatusError                        // check failed
	StatusUpdating                     // update in progress
	StatusDone                         // updated successfully
	StatusCleanCandidate               // remove candidate (cleanup)
	StatusCleaning                     // cleanup in progress
	StatusCleaned                      // cleaned successfully
)

func (s Status) String() string {
	switch s {
	case StatusPending:
		return "pending"
	case StatusOK:
		return "ok"
	case StatusOutdated:
		return "outdated"
	case StatusError:
		return "error"
	case StatusUpdating:
		return "updating"
	case StatusDone:
		return "done"
	case StatusCleanCandidate:
		return "clean-candidate"
	case StatusCleaning:
		return "cleaning"
	case StatusCleaned:
		return "cleaned"
	default:
		return "unknown"
	}
}

// Item represents a single updatable/cleanable entity.
type Item struct {
	Name         string   // display name (e.g., "btop", "OpenCode")
	Category     Category // group
	CurrentVer   string   // installed version ("" if N/A)
	AvailableVer string   // version available ("" if up to date)
	Status       Status
	Selected     bool   // marked by user for action
	Log          string // output from update/clean operation

	// Cleanup-specific fields
	Reclaimable string // human-readable reclaimable info ("4 versões" / "13 GB")
	KeepPolicy  string // retention policy ("keep latest per major")
	RemoveCount int    // number of items that would be removed
}

// SourceSummary is the aggregated state for one category/source.
type SourceSummary struct {
	Category    Category
	Label       string // display name ("Homebrew", "SDKMAN", etc.)
	Items       []*Item
	Total       int
	Outdated    int
	OK          int
	ErrorCount  int
	Reclaimable string // total reclaimable for cleanup items
	Icon        string // emoji/icon for the category
}

// TabID identifies the current dashboard tab.
type TabID int

const (
	TabUpdates TabID = iota
	TabCleanup
	TabLogs
)

func (t TabID) String() string {
	switch t {
	case TabUpdates:
		return "Updates"
	case TabCleanup:
		return "Cleanup"
	case TabLogs:
		return "Logs"
	default:
		return "?"
	}
}

// PlatformInfo holds detected OS and available package managers.
type PlatformInfo struct {
	OS      string // "darwin", "linux"
	Distro  string // "ubuntu", "manjaro", "macos"
	HasBrew bool
	HasMAS  bool

	// Linux package managers
	HasApt     bool
	HasPacman  bool
	HasYay     bool
	HasFlatpak bool
	HasSnap    bool

	// Windows package managers
	HasWinget bool
	HasChoco  bool
	HasScoop  bool

	HasNpm    bool
	HasPipx   bool
	HasGo     bool
	HasGup    bool
	HasRustup bool
	HasCargo  bool
	HasSDKMAN bool
	HasDocker bool
	HasNvm    bool
	HasOmz    bool
}

// GlobalLogEntry stores one line of the session log.
type GlobalLogEntry struct {
	Timestamp string
	Message   string
	Success   bool
}

// TabUpdateMsg is sent when the TUI should refresh.
type TabUpdateMsg struct{}
