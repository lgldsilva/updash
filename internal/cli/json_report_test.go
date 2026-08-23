package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/lgldsilva/updash/internal/model"
)

func sampleSummaries() (updates, cleanup []*model.SourceSummary) {
	updates = []*model.SourceSummary{
		{
			Category: model.CatBrew,
			Label:    "Homebrew",
			Total:    2,
			Outdated: 1,
			Items: []*model.Item{
				{Name: "git", Category: model.CatBrew, CurrentVer: "2.0", AvailableVer: "2.1", Status: model.StatusOutdated},
				{Name: "curl", Category: model.CatBrew, CurrentVer: "8.0", Status: model.StatusOK},
				nil,
			},
		},
		nil,
		{
			Category: model.CatAgent,
			Label:    "AI Agents",
			Total:    1,
			Items: []*model.Item{
				{
					Name: "OpenCode", Category: model.CatAgent, CurrentVer: "1.0",
					AvailableVer: "1.1", Status: model.StatusOutdated, PackageID: "",
				},
			},
		},
	}
	cleanup = []*model.SourceSummary{
		{
			Category: model.CatHomelabClean,
			Label:    "Homelab Cleanup",
			Items: []*model.Item{
				{
					Name: "dev-cache:maven", Category: model.CatHomelabClean,
					Status: model.StatusCleanCandidate, Reclaimable: "12MB",
					KeepPolicy: "mtime > 90d", PackageID: "/tmp/m2", RemoveCount: 3,
				},
				{Name: "ok", Status: model.StatusOK},
			},
		},
	}
	return updates, cleanup
}

func TestBuildCheckReport(t *testing.T) {
	updates, cleanup := sampleSummaries()
	rep := BuildCheckReport(updates, cleanup)
	if rep.Outdated != 2 {
		t.Fatalf("outdated=%d want 2", rep.Outdated)
	}
	if rep.Cleanable != 1 {
		t.Fatalf("cleanable=%d want 1", rep.Cleanable)
	}
	if len(rep.Updates) != 2 || rep.Updates[0].Name != "git" {
		t.Fatalf("updates=%+v", rep.Updates)
	}
	if len(rep.Cleanup) != 1 || rep.Cleanup[0].RemoveCount != 3 {
		t.Fatalf("cleanup=%+v", rep.Cleanup)
	}
	if len(rep.Sources) < 2 {
		t.Fatalf("sources=%+v", rep.Sources)
	}
}

func TestWriteCheckJSON_and_Format(t *testing.T) {
	updates, cleanup := sampleSummaries()
	var buf bytes.Buffer
	rep := BuildCheckReport(updates, cleanup)
	if err := WriteCheckJSON(&buf, rep); err != nil {
		t.Fatal(err)
	}
	var decoded CheckReport
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Outdated != 2 {
		t.Fatalf("decoded outdated=%d", decoded.Outdated)
	}

	s, err := FormatCheckJSON(updates, cleanup)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(s, `"name": "git"`) {
		t.Fatalf("json=%s", s)
	}
}

func TestExitCodeForCheck(t *testing.T) {
	if ExitCodeForCheck(Config{}, 5, 1) != 0 {
		t.Fatal("non-strict should be 0")
	}
	if ExitCodeForCheck(Config{Strict: true}, 1, 0) != 1 {
		t.Fatal("strict outdated")
	}
	if ExitCodeForCheck(Config{Strict: true}, 0, 2) != 1 {
		t.Fatal("strict cleanable")
	}
	if ExitCodeForCheck(Config{Strict: true}, 0, 0) != 0 {
		t.Fatal("strict clean")
	}
}

func TestCheckReport_ProblemsTakeExitPrecedence(t *testing.T) {
	updates := []*model.SourceSummary{{Category: model.CatNpm, Label: "npm", Items: []*model.Item{{Name: "npm", Category: model.CatNpm, Status: model.StatusError}}}}
	rep := BuildCheckReport(updates, nil)
	if rep.Errors != 1 || len(rep.Problems) != 1 {
		t.Fatalf("report did not retain error: %+v", rep)
	}
	if code := ExitCodeForReport(Config{Strict: true}, rep); code != 2 {
		t.Fatalf("exit code=%d, want 2", code)
	}
}

func TestCheckReport_CountsInfoWithoutTreatingItAsError(t *testing.T) {
	rep := BuildCheckReport([]*model.SourceSummary{{Category: model.CatBun, Items: []*model.Item{{Name: "bun-pkg", Category: model.CatBun, Status: model.StatusInfo}}}}, nil)
	if rep.Info != 1 || rep.Errors != 0 || rep.Unverified != 0 {
		t.Fatalf("report=%+v", rep)
	}
	if ExitCodeForReport(Config{}, rep) != 0 {
		t.Fatal("informational inventory must not be inconclusive")
	}
}

func TestExitCodeForReport_StrictIsLowerPrecedenceThanInconclusive(t *testing.T) {
	rep := CheckReport{Outdated: 1, Errors: 1}
	if got := ExitCodeForReport(Config{Strict: true}, rep); got != 2 {
		t.Fatalf("ExitCodeForReport()=%d, want 2", got)
	}
}

func TestValidateJSONMode(t *testing.T) {
	if err := ValidateJSONMode("check", true); err != nil {
		t.Fatal(err)
	}
	if err := ValidateJSONMode("check", false); err != nil {
		t.Fatal(err)
	}
	if err := ValidateJSONMode("tui", true); err == nil {
		t.Fatal("tui+json should fail")
	}
	if err := ValidateJSONMode("update", true); err == nil {
		t.Fatal("update+json should fail")
	}
}

func TestPrintCheckJSON(t *testing.T) {
	updates, cleanup := sampleSummaries()
	out := captureStdout(t, func() {
		o, c, err := PrintCheckJSON(updates, cleanup)
		if err != nil {
			t.Fatal(err)
		}
		if o != 2 || c != 1 {
			t.Fatalf("o=%d c=%d", o, c)
		}
	})
	if !strings.Contains(out, "outdated") {
		t.Fatalf("stdout=%s", out)
	}
}
