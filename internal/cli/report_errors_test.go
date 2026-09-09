package cli

import (
	"strings"
	"testing"

	"github.com/lgldsilva/updash/internal/model"
)

func TestPrintSourceTruthShowsErrorCause(t *testing.T) {
	s := &model.SourceSummary{
		Category: model.CatPnpm,
		Label:    "pnpm (global)",
		Icon:     "📦",
		Items: []*model.Item{{
			Name:       "pnpm",
			Category:   model.CatPnpm,
			Status:     model.StatusError,
			CurrentVer: "error",
			Error:      "fork/exec /home/u/.npm-global/bin/pnpm: exec format error",
		}},
	}
	out := captureStdout(t, func() { printSourceTruth(s) })
	if !strings.Contains(out, "error — fork/exec /home/u/.npm-global/bin/pnpm: exec format error") {
		t.Fatalf("cause missing from report:\n%s", out)
	}
}

func TestPrintSourceTruthWithoutCauseKeepsLegacyLine(t *testing.T) {
	s := &model.SourceSummary{
		Category: model.CatPnpm,
		Label:    "pnpm (global)",
		Icon:     "📦",
		Items: []*model.Item{{
			Name:       "pnpm",
			Category:   model.CatPnpm,
			Status:     model.StatusError,
			CurrentVer: "error",
		}},
	}
	out := captureStdout(t, func() { printSourceTruth(s) })
	if !strings.Contains(out, "✘ 📦 pnpm (global): error\n") {
		t.Fatalf("legacy line changed:\n%s", out)
	}
	if strings.Contains(out, " — ") {
		t.Fatalf("unexpected cause separator:\n%s", out)
	}
}

func TestCheckJSONIncludesErrorCause(t *testing.T) {
	updates := []*model.SourceSummary{{
		Category: model.CatPnpm,
		Label:    "pnpm (global)",
		Icon:     "📦",
		Items: []*model.Item{{
			Name:       "pnpm",
			Category:   model.CatPnpm,
			Status:     model.StatusError,
			CurrentVer: "error",
			Error:      "fork/exec pnpm: exec format error",
		}},
	}}
	rep, err := FormatCheckJSON(updates, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rep, `"error": "fork/exec pnpm: exec format error"`) {
		t.Fatalf("error field missing from JSON:\n%s", rep)
	}
}
