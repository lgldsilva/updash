package updater

import (
	"testing"

	"github.com/lgldsilva/updash/internal/model"
)

func TestClassifyItem_microsoftPassword(t *testing.T) {
	it := &model.Item{
		Name:       "microsoft-office",
		Category:   model.CatBrew,
		KeepPolicy: "PKG Microsoft — precisa de senha de admin no Terminal",
	}
	kind, _ := ClassifyItem(it, nil)
	if kind != KindNeedsPassword {
		t.Fatalf("kind = %v, want KindNeedsPassword", kind)
	}
}

func TestClassifyItem_jetbrainsManual(t *testing.T) {
	it := &model.Item{
		Name:       "clion",
		Category:   model.CatBrew,
		KeepPolicy: "gerido pelo JetBrains Toolbox",
	}
	kind, _ := ClassifyItem(it, nil)
	if kind != KindManualOnly {
		t.Fatalf("kind = %v, want KindManualOnly", kind)
	}
}

func TestSuggestCommand_mas(t *testing.T) {
	it := &model.Item{Category: model.CatMAS, PackageID: "310633997"}
	if got := SuggestCommand(it); got != "mas update 310633997" {
		t.Fatalf("got %q", got)
	}
}
