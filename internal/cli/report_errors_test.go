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
			{Name: "shadowed", Status: model.StatusInfo, CurrentVer: "1.0.0", Log: "shadowed by active npm prefix copy"},
		},
	}
	out := captureStdout(t, func() { printUpdateSummary(s) })
	if !strings.Contains(out, "? ⬡ npm prefix /home/u/.npm-global: error — timeout") {
		t.Fatalf("unverified prefix missing:\n%s", out)
	}
	if !strings.Contains(out, "✘ ⬡ npm: error — ls failed") {
		t.Fatalf("error item missing:\n%s", out)
	}
	if !strings.Contains(out, "shadowed by active npm prefix copy") {
		t.Fatalf("shadow note missing:\n%s", out)
	}
	printProbeItems(&model.SourceSummary{Items: []*model.Item{nil, {Status: model.StatusOK}}})
}

func TestShadowNoteIsAffirmative(t *testing.T) {
	shadow := []*model.SourceSummary{{Items: []*model.Item{{
		Name: "pkg", Status: model.StatusInfo, CurrentVer: "1.0.0", Log: "shadowed by active npm prefix copy",
	}}}}
	if hasNonAffirmative(shadow) {
		t.Fatal("shadow note must not count as non-affirmative")
	}
	open := []*model.SourceSummary{{Items: []*model.Item{
		nil,
		{Name: "agent", Status: model.StatusInfo},
		{Name: "ok", Status: model.StatusOK},
	}}}
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

func TestItemIsOpenQuestion(t *testing.T) {
	if itemIsOpenQuestion(nil) {
		t.Fatal("nil item is not an open question")
	}
	if !itemIsOpenQuestion(&model.Item{Status: model.StatusError}) {
		t.Fatal("error is an open question")
	}
	if !itemIsOpenQuestion(&model.Item{Status: model.StatusUnverified}) {
		t.Fatal("unverified is an open question")
	}
	if !itemIsOpenQuestion(&model.Item{Status: model.StatusInfo}) {
		t.Fatal("info without a log is an open question")
	}
	if itemIsOpenQuestion(&model.Item{Status: model.StatusInfo, Log: "shadowed by active npm prefix copy"}) {
		t.Fatal("info with a log is a known fact")
	}
	if itemIsOpenQuestion(&model.Item{Status: model.StatusOK}) || itemIsOpenQuestion(&model.Item{Status: model.StatusOutdated}) {
		t.Fatal("ok and outdated are not open questions")
	}
}

func TestHasInformationalIgnoresShadowNotes(t *testing.T) {
	shadow := []*model.SourceSummary{
		nil,
		{Items: []*model.Item{
			nil,
			{Status: model.StatusOK},
			{Status: model.StatusInfo, Log: "shadowed by active npm prefix copy"},
		}},
	}
	if hasInformational(nil, shadow) {
		t.Fatal("shadow notes and empty groups are not informational gaps")
	}
	open := []*model.SourceSummary{{Items: []*model.Item{{Status: model.StatusInfo}}}}
	if !hasInformational(shadow, open) {
		t.Fatal("info without a log is an informational gap")
	}
}

func TestPrintUpdateSummaryShowsShadowNote(t *testing.T) {
	s := &model.SourceSummary{
		Label:    "npm (global)",
		Icon:     "⬡",
		Outdated: 1,
		Items: []*model.Item{
			nil,
			{Name: "left-pad", Status: model.StatusOutdated, CurrentVer: "1.0.0", AvailableVer: "2.0.0"},
			{Name: "fresh", Status: model.StatusOK, CurrentVer: "1.0.0"},
			{Name: "agent", Status: model.StatusInfo, CurrentVer: "9.0.0"},
			{Name: "old-copy", Status: model.StatusInfo, CurrentVer: "1.0.0", Log: "shadowed by active npm prefix copy"},
		},
	}
	out := captureStdout(t, func() { printUpdateSummary(s) })
	if !strings.Contains(out, "ℹ old-copy 1.0.0 (shadowed by active npm prefix copy)") {
		t.Fatalf("shadow note missing:\n%s", out)
	}
	if strings.Contains(out, "agent") {
		t.Fatalf("info without a log must not be printed as a shadow note:\n%s", out)
	}
}

func TestPrintProbeItemsSkipsNilAndNonProbes(t *testing.T) {
	s := &model.SourceSummary{
		Icon: "⬡",
		Items: []*model.Item{
			nil,
			{Name: "left-pad", Status: model.StatusOutdated, CurrentVer: "1.0.0"},
			{Name: "bad", Status: model.StatusError, CurrentVer: "error"},
			{Name: "unk", Status: model.StatusUnverified, CurrentVer: "error", Error: "timeout"},
		},
	}
	out := captureStdout(t, func() { printProbeItems(s) })
	if strings.Contains(out, "left-pad") {
		t.Fatalf("outdated item is not a probe:\n%s", out)
	}
	if !strings.Contains(out, "✘ ⬡ bad: error") || !strings.Contains(out, "? ⬡ unk: error — timeout") {
		t.Fatalf("probe lines missing:\n%s", out)
	}
}

func TestPrintSourceTruthInfoWithAndWithoutLog(t *testing.T) {
	s := &model.SourceSummary{
		Icon:  "⬡",
		Label: "npm (global)",
		Items: []*model.Item{
			nil,
			{Name: "old-copy", Status: model.StatusInfo, CurrentVer: "1.2.3", Log: "shadowed by active npm prefix copy"},
			{Name: "agent", Status: model.StatusInfo, CurrentVer: "9.0.0"},
			{Name: "probe", Status: model.StatusUnverified, CurrentVer: "error", Error: "timeout"},
		},
	}
	out := captureStdout(t, func() { printSourceTruth(s) })
	if !strings.Contains(out, "ℹ ⬡ old-copy: 1.2.3 (shadowed by active npm prefix copy)") {
		t.Fatalf("shadow note missing:\n%s", out)
	}
	if !strings.Contains(out, "ℹ ⬡ agent: freshness not verified") {
		t.Fatalf("unverified freshness missing:\n%s", out)
	}
	if !strings.Contains(out, "? ⬡ npm (global): error — timeout") {
		t.Fatalf("unverified probe missing:\n%s", out)
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
