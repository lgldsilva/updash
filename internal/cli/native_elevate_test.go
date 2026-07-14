//go:build darwin

package cli

import (
	"testing"

	"github.com/lgldsilva/updash/internal/model"
)

func TestPartitionNativeElevated_darwin(t *testing.T) {
	plat := model.PlatformInfo{OS: "darwin"}
	items := []*model.Item{
		{Name: "telegram", Category: model.CatBrew},
		{Name: "microsoft-office", Category: model.CatBrew},
		{Name: "app", Category: model.CatMAS},
	}
	native, normal := partitionNativeElevated(plat, items, Config{})
	if len(native) != 2 {
		t.Fatalf("expected 2 native, got %d", len(native))
	}
	if len(normal) != 1 || normal[0].Name != "telegram" {
		t.Fatalf("expected telegram normal, got %+v", normal)
	}
}
