package updater

import (
	"context"
	"strings"
	"testing"

	"github.com/lgldsilva/updash/internal/model"
)

func TestBatchBrewUpgrade_skipsManagedExternally(t *testing.T) {
	items := []*model.Item{
		{Name: "microsoft-office", Category: model.CatBrew, Status: model.StatusOutdated},
		{Name: "microsoft-auto-update", Category: model.CatBrew, Status: model.StatusOutdated},
	}

	results := batchBrewUpgrade(context.Background(), items, SilentOptions())
	if len(results) != 2 {
		t.Fatalf("len(results) = %d, want 2", len(results))
	}
	for i, r := range results {
		if r == nil || r.Success {
			t.Fatalf("item %d should be skipped: %+v", i, r)
		}
		if !strings.Contains(r.Error, "skipped") {
			t.Fatalf("item %d error = %q, want skip message", i, r.Error)
		}
		if items[i].Status != model.StatusOutdated {
			t.Fatalf("item %d status = %v, want outdated", i, items[i].Status)
		}
	}
}
