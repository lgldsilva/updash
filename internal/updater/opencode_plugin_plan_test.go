package updater

import (
	"slices"
	"strings"
	"testing"

	"github.com/lgldsilva/updash/internal/model"
	"github.com/lgldsilva/updash/internal/scanner"
)

// A plugin pinned exactly in ~/.config/opencode/package.json never moves via
// `npm update`; the plan must install the flagged versions explicitly.
func TestOpencodePluginPlans_InstallsPinnedVersions(t *testing.T) {
	items := []*model.Item{
		{Name: "@opencode-ai/plugin", Category: model.CatOpenCodePlugins, AvailableVer: "1.18.27"},
		{Name: "@ai-sdk/google", Category: model.CatOpenCodePlugins, AvailableVer: "4.0.63"},
	}
	plans, err := opencodePluginPlans(items)
	if err != nil {
		t.Fatal(err)
	}
	if len(plans) != 1 {
		t.Fatalf("plans = %v", plans)
	}
	p := plans[0]
	if p.Name != "npm" || p.Scope != CommandScopeExact {
		t.Fatalf("plan = %+v", p)
	}
	want := []string{"install", "--prefix", scanner.OpenCodeConfigDir(),
		"@opencode-ai/plugin@1.18.27", "@ai-sdk/google@4.0.63"}
	if !slices.Equal(p.Args, want) {
		t.Fatalf("args = %v, want %v", p.Args, want)
	}
}

// Without a known target version there is nothing to install explicitly —
// fall back to the historical `npm update` behaviour.
func TestOpencodePluginPlans_FallsBackToNpmUpdate(t *testing.T) {
	items := []*model.Item{
		{Name: "@opencode-ai/plugin", Category: model.CatOpenCodePlugins},
	}
	plans, err := opencodePluginPlans(items)
	if err != nil {
		t.Fatal(err)
	}
	if len(plans) != 1 || plans[0].Scope != CommandScopeCategoryGlobal {
		t.Fatalf("plans = %v", plans)
	}
	if strings.Join(plans[0].Args, " ") != "update --prefix "+scanner.OpenCodeConfigDir() {
		t.Fatalf("args = %v", plans[0].Args)
	}
}
