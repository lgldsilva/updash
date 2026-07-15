package updater

import (
	"context"
	"strings"
	"testing"

	"github.com/lgldsilva/updash/internal/model"
)

func TestUpdateAgent_manualOnly(t *testing.T) {
	it := &model.Item{
		Name: "Cursor", Category: model.CatAgent,
		KeepPolicy: "manual reinstall / app update",
		Status:     model.StatusOutdated,
	}
	res := updateAgent(context.Background(), it, SilentOptions())
	if res.Success {
		t.Fatal("manual agent must not report success")
	}
	if !strings.HasPrefix(res.Error, "⊘ ") {
		t.Fatalf("error=%q", res.Error)
	}
	if it.Status != model.StatusOutdated {
		t.Fatalf("status=%v", it.Status)
	}
}

func TestUpdateAgent_defaultManualNote(t *testing.T) {
	it := &model.Item{Name: "Crush", Category: model.CatAgent}
	res := updateAgent(context.Background(), it, SilentOptions())
	if res.Success || !strings.Contains(res.Error, "manual") {
		t.Fatalf("%+v", res)
	}
}
