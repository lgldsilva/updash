package cli

import (
	"testing"

	"github.com/lgldsilva/updash/internal/model"
	"github.com/lgldsilva/updash/internal/updater"
)

func TestIsSkippedResult(t *testing.T) {
	if !isSkippedResult(&updater.Result{Error: "⊘ senha cancelada"}) {
		t.Fatal("expected skip")
	}
	if isSkippedResult(&updater.Result{Error: "failed"}) {
		t.Fatal("expected not skip")
	}
}

func TestShouldFailExit_strict(t *testing.T) {
	cfg := Config{Strict: true}
	if !shouldFailExit(cfg, verifyStats{remaining: 1}) {
		t.Fatal("strict should fail with remaining")
	}
	if shouldFailExit(Config{}, verifyStats{remaining: 1, failed: 0}) {
		t.Fatal("non-strict should not fail on remaining only")
	}
}

func TestBrewBatchNeedsPassword(t *testing.T) {
	items := []*model.Item{{Name: "microsoft-office"}}
	if !brewBatchNeedsPassword(items) {
		t.Fatal("expected password batch")
	}
	if brewBatchNeedsPassword([]*model.Item{{Name: "telegram"}}) {
		t.Fatal("telegram should not need password batch")
	}
}
