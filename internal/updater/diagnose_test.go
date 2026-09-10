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

func TestClassifyItem_manualPolicy(t *testing.T) {
	it := &model.Item{
		Name: "Cursor", Category: model.CatAgent,
		KeepPolicy: "manual reinstall / app update",
	}
	kind, _ := ClassifyItem(it, nil)
	if kind != KindManualOnly {
		t.Fatalf("kind=%v", kind)
	}
}

func TestClassifyItem_brewDisabledCask(t *testing.T) {
	it := &model.Item{Name: "flameshot", Category: model.CatBrew}
	result := &Result{
		Success: false,
		Error:   "flameshot ainda desatualizado após brew upgrade (verifique manualmente: brew upgrade --greedy flameshot)",
		Output:  "Warning: Not upgrading flameshot, it is disabled because it does not pass the macOS Gatekeeper check! It was disabled on 2026-09-01.",
	}
	kind, reason := ClassifyItem(it, result)
	if kind != KindManualOnly {
		t.Fatalf("kind = %v, want KindManualOnly", kind)
	}
	if reason == "" {
		t.Fatal("want a non-empty manual-only reason")
	}
}

func TestSuggestCommand_agentsAndPlugins(t *testing.T) {
	cases := []struct {
		it   *model.Item
		want string
	}{
		{&model.Item{Category: model.CatOpenCodePlugins}, "npm update --prefix ~/.config/opencode"},
		{&model.Item{Category: model.CatAgent, Name: "OpenCode"}, "opencode upgrade"},
		{&model.Item{Category: model.CatAgent, Name: "Claude Code"}, "claude update"},
		{&model.Item{Category: model.CatAgent, Name: "Codex"}, "npm install -g --allow-scripts=@openai/codex @openai/codex@latest"},
		{&model.Item{Category: model.CatAgent, Name: "Copilot CLI"}, "copilot update"},
		{&model.Item{Category: model.CatAgent, Name: "Cursor"}, ""},
	}
	for _, tc := range cases {
		if got := SuggestCommand(tc.it); got != tc.want {
			t.Fatalf("%s: got %q want %q", tc.it.Name, got, tc.want)
		}
	}
}
