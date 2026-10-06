package updater

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/lgldsilva/updash/internal/model"
)

// partitionNpmItems must separate protected packages (owned by another path)
// from the generic npm update batch.
func TestPartitionNpmItems(t *testing.T) {
	items := []*model.Item{
		{Name: "left-pad", Category: model.CatNpm},
		{Name: "opencode-ai", Category: model.CatNpm},
		{Name: "@opencode-ai/cli", Category: model.CatNpm},
		{Name: "@opencode/cli", Category: model.CatNpm},
		{Name: "@charmland/crush", Category: model.CatNpm},
		{Name: "react", Category: model.CatNpm},
	}
	updatable, protected := partitionNpmItems(items)
	if len(updatable) != 2 || len(protected) != 4 {
		t.Fatalf("split = %d updatable / %d protected, want 2/4", len(updatable), len(protected))
	}
	for _, it := range protected {
		if it.Name == "left-pad" || it.Name == "react" {
			t.Errorf("non-protected package %q leaked into protected list", it.Name)
		}
	}
}

func TestNpmUpdateArgs_BuildsAndDedups(t *testing.T) {
	items := []*model.Item{
		{Name: "left-pad"},
		{Name: "left-pad"}, // duplicate
		{Name: "react"},
		{Name: ""}, // ignored
	}
	got := npmUpdateArgs("", items)
	want := []string{commandUpdate, flagGlobal, "left-pad", "react"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("npmUpdateArgs = %v, want %v", got, want)
	}
}

func TestNpmUpdateArgs_PinsExplicitPrefix(t *testing.T) {
	items := []*model.Item{{Name: "left-pad", Prefix: "/home/u/.npm-global"}}
	got := npmUpdateArgs("/home/u/.npm-global", items)
	want := []string{commandUpdate, flagGlobal, flagPrefix, "/home/u/.npm-global", "left-pad"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("npmUpdateArgs = %v, want %v", got, want)
	}
}

func TestNpmUpdateArgs_EmptyNeverBuildsBareGlobalUpdate(t *testing.T) {
	if got := npmUpdateArgs("", nil); len(got) != 0 {
		t.Fatalf("npmUpdateArgs(nil) = %v, want no command", got)
	}
	if got := npmUpdateArgs("", []*model.Item{{Name: ""}}); len(got) != 0 {
		t.Fatalf("npmUpdateArgs(empty name) = %v, want no command", got)
	}
}

// The composed pipeline (partition -> args) must never emit a protected name.
func TestNpmPipeline_ExcludesProtectedFromArgs(t *testing.T) {
	items := []*model.Item{
		{Name: "left-pad"},
		{Name: "opencode-ai"},
		{Name: "react"},
		{Name: "@opencode-ai/cli"},
		{Name: "@opencode/cli"},
	}
	updatable, _ := partitionNpmItems(items)
	args := npmUpdateArgs("", updatable)
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "left-pad") || !strings.Contains(joined, "react") {
		t.Errorf("non-protected names missing from args: %v", args)
	}
	if strings.Contains(joined, "opencode-ai") || strings.Contains(joined, "@opencode-ai/cli") || strings.Contains(joined, "@opencode/cli") {
		t.Errorf("protected package leaked into npm update args: %v", args)
	}
}

// When every item is protected, batchNpmUpgrade must NOT invoke npm at all —
// the items are owned by opencode upgrade. (No real subprocess runs here: the
// empty-updatable branch returns before building any command.)
func TestBatchNpmUpgrade_SkipsWhenOnlyProtected(t *testing.T) {
	items := []*model.Item{
		{Name: "opencode-ai", Category: model.CatNpm, Status: model.StatusOutdated},
		{Name: "@opencode-ai/cli", Category: model.CatNpm, Status: model.StatusOutdated},
		{Name: "@opencode/cli", Category: model.CatNpm, Status: model.StatusOutdated},
	}
	results := batchNpmUpgrade(context.Background(), items, SilentOptions())
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	for i, it := range items {
		if it.Status != model.StatusOK {
			t.Errorf("item[%d] %q status = %v, want OK (managed elsewhere)", i, it.Name, it.Status)
		}
		if !results[i].Success {
			t.Errorf("result[%d] should be success (managed elsewhere): %+v", i, results[i])
		}
		if results[i].Output != npmManagedElsewhereNote {
			t.Errorf("result[%d] output = %q, want %q", i, results[i].Output, npmManagedElsewhereNote)
		}
	}
}
