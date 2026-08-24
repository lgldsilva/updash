package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/lgldsilva/updash/internal/model"
)

// CheckReport is the machine-readable form of --check (and --check --json).
type CheckReport struct {
	Platform   string         `json:"platform,omitempty"`
	Outdated   int            `json:"outdated"`
	Cleanable  int            `json:"cleanable"`
	Errors     int            `json:"errors"`
	Unverified int            `json:"unverified"`
	Info       int            `json:"info"`
	Problems   []ReportItem   `json:"problems,omitempty"`
	Updates    []ReportItem   `json:"updates"`
	Cleanup    []ReportItem   `json:"cleanup"`
	Sources    []ReportSource `json:"sources,omitempty"`
	ElapsedMS  int64          `json:"elapsed_ms,omitempty"`
}

// ReportItem is one outdated or cleanable entity.
type ReportItem struct {
	Name        string `json:"name"`
	Category    string `json:"category"`
	Current     string `json:"current,omitempty"`
	Available   string `json:"available,omitempty"`
	Status      string `json:"status"`
	Reclaimable string `json:"reclaimable,omitempty"`
	KeepPolicy  string `json:"keep_policy,omitempty"`
	PackageID   string `json:"package_id,omitempty"`
	RemoveCount int    `json:"remove_count,omitempty"`
}

// ReportSource summarizes one scanner category.
type ReportSource struct {
	Category   string `json:"category"`
	Label      string `json:"label"`
	Outdated   int    `json:"outdated"`
	Total      int    `json:"total"`
	Kind       string `json:"kind"` // "update" or "cleanup"
	Errors     int    `json:"errors"`
	Unverified int    `json:"unverified"`
	Info       int    `json:"info"`
}

// BuildCheckReport aggregates scan results into a stable JSON structure.
func BuildCheckReport(updates, cleanup []*model.SourceSummary) CheckReport {
	rep := CheckReport{
		Updates:  make([]ReportItem, 0),
		Cleanup:  make([]ReportItem, 0),
		Problems: make([]ReportItem, 0),
		Sources:  make([]ReportSource, 0),
	}
	rep.Outdated = appendStatusItems(&rep.Updates, &rep.Sources, updates, model.StatusOutdated, "update")
	rep.Cleanable = appendStatusItems(&rep.Cleanup, &rep.Sources, cleanup, model.StatusCleanCandidate, "cleanup")
	rep.Errors, rep.Unverified = appendProblems(&rep.Problems, updates, cleanup)
	rep.Info = countStatus(model.StatusInfo, updates, cleanup)
	return rep
}

func countStatus(status model.Status, groups ...[]*model.SourceSummary) int {
	n := 0
	for _, summaries := range groups {
		for _, s := range summaries {
			if s != nil {
				for _, it := range s.Items {
					if it != nil && it.Status == status {
						n++
					}
				}
			}
		}
	}
	return n
}

func appendProblems(dest *[]ReportItem, groups ...[]*model.SourceSummary) (errors, unverified int) {
	for _, summaries := range groups {
		for _, s := range summaries {
			if s == nil {
				continue
			}
			for _, it := range s.Items {
				if it == nil {
					continue
				}
				switch it.Status {
				case model.StatusError:
					errors++
					*dest = append(*dest, itemToReport(it))
				case model.StatusUnverified:
					unverified++
					*dest = append(*dest, itemToReport(it))
				}
			}
			// A source error without an error item is still a reportable problem.
			if s.ErrorCount > 0 && !summaryHasStatus(s, model.StatusError) {
				errors += s.ErrorCount
				*dest = append(*dest, ReportItem{Name: s.Label, Category: string(s.Category), Status: model.StatusError.String()})
			}
			if s.Unverified > 0 && !summaryHasStatus(s, model.StatusUnverified) {
				unverified += s.Unverified
				*dest = append(*dest, ReportItem{Name: s.Label, Category: string(s.Category), Status: model.StatusUnverified.String()})
			}
		}
	}
	return errors, unverified
}

func summaryHasStatus(s *model.SourceSummary, status model.Status) bool {
	for _, it := range s.Items {
		if it != nil && it.Status == status {
			return true
		}
	}
	return false
}

// appendStatusItems collects items matching status from summaries into dest and sources.
func appendStatusItems(
	dest *[]ReportItem,
	sources *[]ReportSource,
	summaries []*model.SourceSummary,
	status model.Status,
	kind string,
) int {
	total := 0
	for _, s := range summaries {
		n := collectMatching(dest, s, status)
		if s == nil {
			continue
		}
		*sources = append(*sources, ReportSource{
			Category:   string(s.Category),
			Label:      s.Label,
			Outdated:   n,
			Total:      s.Total,
			Kind:       kind,
			Errors:     s.ErrorCount,
			Unverified: s.Unverified,
			Info:       s.Info,
		})
		total += n
	}
	return total
}

func collectMatching(dest *[]ReportItem, s *model.SourceSummary, status model.Status) int {
	if s == nil {
		return 0
	}
	n := 0
	for _, it := range s.Items {
		if it == nil || it.Status != status {
			continue
		}
		n++
		*dest = append(*dest, itemToReport(it))
	}
	return n
}

func itemToReport(it *model.Item) ReportItem {
	return ReportItem{
		Name:        it.Name,
		Category:    string(it.Category),
		Current:     it.CurrentVer,
		Available:   it.AvailableVer,
		Status:      it.Status.String(),
		Reclaimable: it.Reclaimable,
		KeepPolicy:  it.KeepPolicy,
		PackageID:   it.PackageID,
		RemoveCount: it.RemoveCount,
	}
}

// WriteCheckJSON encodes the report as pretty JSON to w.
func WriteCheckJSON(w io.Writer, rep CheckReport) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(rep)
}

// PrintCheckJSON writes the check report to stdout.
func PrintCheckJSON(updates, cleanup []*model.SourceSummary) (outdated, cleanable int, err error) {
	rep := BuildCheckReport(updates, cleanup)
	if err := WriteCheckJSON(os.Stdout, rep); err != nil {
		return 0, 0, err
	}
	return rep.Outdated, rep.Cleanable, nil
}

// FormatCheckJSON returns the report as a JSON string (tests / embedding).
func FormatCheckJSON(updates, cleanup []*model.SourceSummary) (string, error) {
	rep := BuildCheckReport(updates, cleanup)
	b, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b) + "\n", nil
}

// ExitCodeForCheck returns process exit code for --check under --strict.
func ExitCodeForCheck(cfg Config, outdated, cleanable int) int {
	if cfg.Strict && (outdated > 0 || cleanable > 0) {
		return 1
	}
	return 0
}

// ExitCodeForReport applies inconclusive scan precedence over strict findings.
func ExitCodeForReport(cfg Config, rep CheckReport) int {
	if rep.Errors > 0 || rep.Unverified > 0 {
		return 2
	}
	return ExitCodeForCheck(cfg, rep.Outdated, rep.Cleanable)
}

func ExitCodeForSummaries(cfg Config, updates, cleanup []*model.SourceSummary) int {
	return ExitCodeForReport(cfg, BuildCheckReport(updates, cleanup))
}

// ValidateJSONMode reports whether --json is only valid with check (or alone defaults).
func ValidateJSONMode(mode string, json bool) error {
	if !json {
		return nil
	}
	switch mode {
	case "check", "tui":
		// tui+json is invalid; caller should force check or error
		if mode == "tui" {
			return fmt.Errorf("--json requires --check")
		}
		return nil
	default:
		return fmt.Errorf("--json is only supported with --check")
	}
}
