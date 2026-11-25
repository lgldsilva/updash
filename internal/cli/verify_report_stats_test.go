package cli

import (
	"strings"
	"testing"

	"github.com/lgldsilva/updash/internal/model"
	"github.com/lgldsilva/updash/internal/updater"
)

// Regression: the apt scan runs BEFORE the plan's `apt-get update`, so the
// verify rescan can list packages the plan never received. An outdated item
// with no result at all was not attempted and must not be reported as
// "ainda desatualizado após update" nor drive the exit code. A manual-only
// leftover in the same rescan keeps its own bucket.
func TestPrintVerifyReport_UnattemptedItemsAreNotFailures(t *testing.T) {
	docker := &model.Item{Name: "docker-ce", Category: model.CatApt, Status: model.StatusOutdated}
	ghostty := &model.Item{Name: "ghostty", Category: model.CatApt, Status: model.StatusOutdated, CurrentVer: "1.0", AvailableVer: "1.1"}
	sqlite := &model.Item{Name: "sqlite3", Category: model.CatApt, Status: model.StatusOutdated}
	clion := &model.Item{Name: "clion", Category: model.CatBrew, Status: model.StatusOutdated, KeepPolicy: "gerido pelo JetBrains Toolbox"}
	updates := []*model.SourceSummary{{Items: []*model.Item{docker, ghostty, sqlite, clion}}}
	results := []*updater.Result{
		{Item: docker, Success: false, Error: "exit status 100", Output: "E: Unable to fetch …"},
		{Item: clion, Success: false, Error: "⊘ " + clion.KeepPolicy},
	}

	var stats verifyStats
	out := captureStdout(t, func() {
		stats = PrintVerifyReport(updates, results, 2, 2, 0)
	})

	if stats.failed != 1 || stats.remaining != 2 || stats.unattempted != 2 || stats.manual != 1 {
		t.Fatalf("stats=%+v, want failed=1 remaining=2 manual=1 unattempted=2", stats)
	}
	if !strings.Contains(out, "Not attempted") || !strings.Contains(out, "ghostty") || !strings.Contains(out, "run again") {
		t.Fatalf("missing not-attempted group:\n%s", out)
	}
	if strings.Contains(out, "✘ 3 failed") || strings.Contains(out, "3 item(s) still outdated") {
		t.Fatalf("unattempted items leaked into failed/remaining:\n%s", out)
	}
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "ainda desatualizado") && (strings.Contains(line, "ghostty") || strings.Contains(line, "sqlite3")) {
			t.Fatalf("unattempted item reported as failed: %q", line)
		}
	}
	if !strings.Contains(out, "2 not attempted") {
		t.Fatalf("header missing not-attempted count:\n%s", out)
	}
}
