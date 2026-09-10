package cli

import (
	"testing"

	"github.com/lgldsilva/updash/internal/model"
	"github.com/lgldsilva/updash/internal/updater"
)

// The failed count that drives the exit decision must come from the
// classification of what REMAINS outdated — not from the raw execution
// counter. A manual-only leftover (Toolbox-managed cask, disabled cask,
// package-origin conflict) is not a failed update.
func TestPrintVerifyReportCountsOnlyClassifiedFailures(t *testing.T) {
	manual := &model.Item{
		Name: "clion", Category: model.CatBrew, Status: model.StatusOutdated,
		KeepPolicy: "gerido pelo JetBrains Toolbox",
	}
	broken := &model.Item{Name: "brokentool", Category: model.CatBrew, Status: model.StatusOutdated}
	updates := []*model.SourceSummary{
		{Items: []*model.Item{manual, broken}},
	}
	results := []*updater.Result{
		{Item: manual, Success: false, Error: "clion ainda desatualizado após brew upgrade"},
		{Item: broken, Success: false, Error: "brew upgrade falhou: exit status 1", Output: "Error: download failed"},
	}

	var stats verifyStats
	captureStdout(t, func() {
		stats = PrintVerifyReport(updates, results, 1, 2, 0)
	})
	if stats.failed != 1 {
		t.Fatalf("stats.failed = %d, want 1 (only the non-manual leftover)", stats.failed)
	}
	if stats.manual != 1 {
		t.Fatalf("stats.manual = %d, want 1", stats.manual)
	}
	if stats.remaining != 2 {
		t.Fatalf("stats.remaining = %d, want 2", stats.remaining)
	}
}
