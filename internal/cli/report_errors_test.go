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

func TestPrintUpdateSummaryShowsProbeBesideOutdated(t *testing.T) {
	s := &model.SourceSummary{
		Label:    "npm (global)",
		Icon:     "⬡",
		Outdated: 1,
		Items: []*model.Item{
			{Name: "left-pad", Status: model.StatusOutdated, CurrentVer: "1.0.0", AvailableVer: "2.0.0"},
			{Name: "npm prefix /home/u/.npm-global", Status: model.StatusUnverified, CurrentVer: "error", Error: "timeout"},
			{Name: "npm", Status: model.StatusError, CurrentVer: "error", Error: "ls failed"},
		},
	}
	out := captureStdout(t, func() { printUpdateSummary(s) })
	if !strings.Contains(out, "? ⬡ npm prefix /home/u/.npm-global: error — timeout") {
		t.Fatalf("unverified prefix missing:\n%s", out)
	}
	if !strings.Contains(out, "✘ ⬡ npm: error — ls failed") {
		t.Fatalf("error item missing:\n%s", out)
	}
}

func TestShadowNoteIsAffirmative(t *testing.T) {
	shadow := []*model.SourceSummary{{Items: []*model.Item{{
		Name: "pkg", Status: model.StatusInfo, CurrentVer: "1.0.0", Log: "shadowed by active npm prefix copy",
	}}}}
	if hasNonAffirmative(shadow) {
		t.Fatal("shadow note must not count as non-affirmative")
	}
	open := []*model.SourceSummary{{Items: []*model.Item{{Name: "agent", Status: model.StatusInfo}}}}
	if !hasNonAffirmative(open) {
		t.Fatal("info without a log is still non-affirmative")
	}
	out := captureStdout(t, func() {
		if _, _, err := emptyUpdateOutcome(shadow, 0); err != nil {
			t.Fatal(err)
		}
	})
	if strings.Contains(out, "not affirmatively verified") || !strings.Contains(out, "✓ Nothing to update") {
		t.Fatalf("shadow note tripped the noop warning:\n%s", out)
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
