package scanner

import (
	"strings"

	"github.com/lgldsilva/updash/internal/model"
)

// Shared status strings (Sonar S1192 — avoid duplicating literals).
const (
	statusUpToDate = "up to date"
	statusError    = "error"
)

// errCauseMaxLen caps a rendered cause so one chatty probe cannot blow up a
// report line.
const errCauseMaxLen = 120

func okItem(name string, cat model.Category) *model.Item {
	return &model.Item{
		Name:       name,
		Category:   cat,
		Status:     model.StatusOK,
		CurrentVer: statusUpToDate,
	}
}

// errItem builds the "check failed" item. The cause is condensed into a single
// line and kept on the item so reports can say *why* the probe failed (a
// missing binary, a bad flag, exec format error), not just that it did.
func errItem(name string, cat model.Category, cause error) *model.Item {
	it := &model.Item{
		Name:       name,
		Category:   cat,
		Status:     model.StatusError,
		CurrentVer: statusError,
	}
	if msg := errCause(cause); msg != "" {
		it.Error = msg
	}
	return it
}

// errCause condenses a failed probe into one display line: the child's stderr
// when it ran and answered, else the Go error string (e.g. "fork/exec
// /usr/bin/x: exec format error"). Empty when there is nothing to say.
func errCause(err error) string {
	if err == nil {
		return ""
	}
	msg := errStderr(err)
	if idx := strings.IndexByte(msg, '\n'); idx >= 0 {
		msg = msg[:idx]
	}
	msg = strings.TrimSpace(msg)
	if runes := []rune(msg); len(runes) > errCauseMaxLen {
		msg = string(runes[:errCauseMaxLen-1]) + "…"
	}
	return msg
}

func infoItem(name string, cat model.Category, current string) *model.Item {
	return &model.Item{
		Name:       name,
		Category:   cat,
		Status:     model.StatusInfo,
		CurrentVer: current,
	}
}

func infoOrOutdated(name string, cat model.Category, items []*model.Item) []*model.Item {
	if len(items) == 0 {
		return []*model.Item{infoItem(name, cat, "inventory only")}
	}
	return items
}

func okOrOutdated(name string, cat model.Category, items []*model.Item) []*model.Item {
	if len(items) == 0 {
		return []*model.Item{okItem(name, cat)}
	}
	return items
}
