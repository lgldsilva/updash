package updater

import (
	"context"
	"os/exec"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/lgldsilva/updash/internal/model"
)

func withCrushBinary(t *testing.T, path, resolved string) {
	t.Helper()
	prevLook, prevEval := lookPath, evalSymlinks
	lookPath = func(file string) (string, error) {
		if file == "crush" {
			return path, nil
		}
		return "", exec.ErrNotFound
	}
	evalSymlinks = func(p string) (string, error) { return resolved, nil }
	t.Cleanup(func() { lookPath, evalSymlinks = prevLook, prevEval })
}

// Crush installed under a user-owned legacy prefix must be updated THERE, not
// duplicated into the ambient prefix.
func TestCrushUpgradePlan_NpmPrefix(t *testing.T) {
	withCrushBinary(t, "/usr/local/bin/crush", "/home/u/.npm-global/lib/node_modules/@charmland/crush/run-crush.js")
	plan := crushUpgradePlan()
	if plan.Scope == CommandScopeManual {
		t.Fatalf("expected a command plan, got manual: %+v", plan)
	}
	if plan.Name != npmCommand || plan.Elevated {
		t.Fatalf("plan = %+v, want plain npm", plan)
	}
	want := []string{commandInstall, flagGlobal, flagPrefix, "/home/u/.npm-global", "--allow-scripts=@charmland/crush", "@charmland/crush@latest"}
	if !slices.Equal(plan.Args, want) {
		t.Fatalf("args = %v, want %v", plan.Args, want)
	}
}

// A binary outside any npm layout has no safe npm target: plan an honest
// manual reinstall instead of shadowing it with an npm copy.
func TestCrushUpgradePlan_NotNpmManaged(t *testing.T) {
	withCrushBinary(t, "/opt/crush/bin/crush", "/opt/crush/bin/crush")
	plan := crushUpgradePlan()
	if plan.Scope != CommandScopeManual || plan.Manual == "" {
		t.Fatalf("want manual plan for non-npm layout, got %+v", plan)
	}
}

func TestCrushUpgradePlan_BinaryMissing(t *testing.T) {
	prev := lookPath
	lookPath = func(string) (string, error) { return "", exec.ErrNotFound }
	t.Cleanup(func() { lookPath = prev })
	if plan := crushUpgradePlan(); plan.Scope != CommandScopeManual {
		t.Fatalf("want manual plan when crush is not found, got %+v", plan)
	}
}

// agentPlans must route Crush through the prefix-aware hook (never through
// the generic npm-managed fallback).
func TestAgentPlans_CrushUsesPrefixHook(t *testing.T) {
	withCrushBinary(t, "/usr/local/bin/crush", "/home/u/.npm-global/lib/node_modules/@charmland/crush/run-crush.js")
	plans, err := agentPlans([]*model.Item{{Name: "Crush", Category: model.CatAgent, Status: model.StatusOutdated}})
	if err != nil {
		t.Fatal(err)
	}
	if len(plans) != 1 || plans[0].Scope == CommandScopeManual {
		t.Fatalf("agentPlans(Crush) = %+v, want the prefix-aware npm plan", plans)
	}
	if !slices.Contains(plans[0].Args, "/home/u/.npm-global") {
		t.Fatalf("plan args lost the resolved prefix: %v", plans[0].Args)
	}
}

func TestGroupNpmByPrefix(t *testing.T) {
	items := []*model.Item{
		{Name: "z", Prefix: "/b"},
		{Name: "a"},
		{Name: "y", Prefix: "/a"},
		{Name: "b"},
		nil,
	}
	groups := groupNpmByPrefix(items)
	if len(groups) != 3 {
		t.Fatalf("groups = %d, want 3", len(groups))
	}
	if groups[0][0].Prefix != "" || groups[0][0].Name != "a" || groups[0][1].Name != "b" {
		t.Fatalf("ambient group must come first in order: %+v", groups[0])
	}
	if groups[1][0].Prefix != "/a" || groups[2][0].Prefix != "/b" {
		t.Fatalf("legacy groups must follow lexicographically: %+v / %+v", groups[1], groups[2])
	}
}

func TestNpmPrefixNeedsSudo(t *testing.T) {
	for _, tc := range []struct {
		prefix string
		want   bool
	}{
		{"/usr", true},
		{"/usr/local", true},
		{"/home/u/.npm-global", false},
		{"/opt/homebrew", false},
		{"", false},
	} {
		if got := npmPrefixNeedsSudo(tc.prefix); got != tc.want {
			t.Errorf("npmPrefixNeedsSudo(%q) = %v, want %v", tc.prefix, got, tc.want)
		}
	}
}

func TestNpmGroupElevation(t *testing.T) {
	// Explicit prefixes are judged by path — no npm probe must run.
	ctx := context.Background()
	legacyGroup := []*model.Item{{Name: "p", Prefix: "/home/u/.npm-global"}}
	if elevated, err := npmGroupElevation(ctx, legacyGroup); err != nil || elevated {
		t.Fatalf("legacy group elevation = %v, %v; want false, nil", elevated, err)
	}
	systemGroup := []*model.Item{{Name: "p", Prefix: "/usr/local"}}
	if elevated, err := npmGroupElevation(ctx, systemGroup); err != nil || !elevated {
		t.Fatalf("/usr group elevation = %v, %v; want true, nil", elevated, err)
	}

	// The ambient group keeps the `npm config get prefix` probe.
	prev := npmPrefixRunner
	t.Cleanup(func() { npmPrefixRunner = prev })
	npmPrefixRunner = func(context.Context) ([]byte, error) { return []byte("/storage/nvm/versions/node/v24/bin\n"), nil }
	if elevated, err := npmGroupElevation(ctx, []*model.Item{{Name: "p"}}); err != nil || elevated {
		t.Fatalf("ambient user-owned prefix elevation = %v, %v; want false, nil", elevated, err)
	}
	npmPrefixRunner = func(context.Context) ([]byte, error) { return []byte("/usr\n"), nil }
	if elevated, err := npmGroupElevation(ctx, []*model.Item{{Name: "p"}}); err != nil || !elevated {
		t.Fatalf("ambient /usr prefix elevation = %v, %v; want true, nil", elevated, err)
	}
}

// One plan per prefix group, ambient first, with per-group elevation.
func TestNpmPrefixGroupPlans(t *testing.T) {
	prev := npmPrefixRunner
	t.Cleanup(func() { npmPrefixRunner = prev })
	npmPrefixRunner = func(context.Context) ([]byte, error) { return []byte("/usr\n"), nil }

	items := []*model.Item{
		{Name: "legacy-pkg", Prefix: "/home/u/.npm-global"},
		{Name: "sys-pkg"},
	}
	plans, err := npmPrefixGroupPlans(context.Background(), items)
	if err != nil {
		t.Fatal(err)
	}
	if len(plans) != 2 {
		t.Fatalf("plans = %d, want 2", len(plans))
	}
	if !plans[0].Elevated || plans[1].Elevated {
		t.Fatalf("elevation = %v / %v, want true / false (ambient /usr first, legacy second)", plans[0].Elevated, plans[1].Elevated)
	}
	if !reflect.DeepEqual(plans[1].Args, []string{commandUpdate, flagGlobal, flagPrefix, "/home/u/.npm-global", "legacy-pkg", "--allow-scripts=legacy-pkg"}) {
		t.Fatalf("legacy args = %v", plans[1].Args)
	}
	if !strings.Contains(strings.Join(plans[0].Args, " "), "sys-pkg") {
		t.Fatalf("ambient args = %v", plans[0].Args)
	}
}

// Execution must zip each plan back onto its own prefix group: a legacy
// failure must not mark the ambient group, and vice versa.
func TestExecutePreparedNpm_MultiPrefixGroups(t *testing.T) {
	prevRun := runUpdateCmd
	t.Cleanup(func() { runUpdateCmd = prevRun })
	var ran []string
	runUpdateCmd = func(ctx context.Context, opts Options, name string, args ...string) (string, string, error) {
		ran = append(ran, strings.Join(args, " "))
		if slices.Contains(args, "boom-pkg") {
			return "", "npm ERR", context.DeadlineExceeded
		}
		return "updated", "", nil
	}

	items := []*model.Item{
		{Name: "boom-pkg", Prefix: "/legacy", Category: model.CatNpm, Status: model.StatusOutdated},
		{Name: "ok-pkg", Category: model.CatNpm, Status: model.StatusOutdated},
	}
	plans := []CommandPlan{
		{Name: npmCommand, Args: []string{commandUpdate, flagGlobal, "ok-pkg"}, Scope: CommandScopeExact},
		{Name: npmCommand, Args: []string{commandUpdate, flagGlobal, flagPrefix, "/legacy", "boom-pkg"}, Scope: CommandScopeExact},
	}
	results := executePreparedNpm(context.Background(), items, plans, SilentOptions())

	if len(ran) != 2 {
		t.Fatalf("ran = %v, want one command per group", ran)
	}
	byName := map[string]*Result{}
	for _, r := range results {
		byName[r.Item.Name] = r
	}
	if !byName["ok-pkg"].Success || byName["boom-pkg"].Success {
		t.Fatalf("results = ok:%v boom:%v, want success/failure", byName["ok-pkg"].Success, byName["boom-pkg"].Success)
	}
}

func TestBatchNpmUpgrade_MultiPrefixGroups(t *testing.T) {
	prevRun, prevElev, prevPrefix := runUpdateCmd, runElevatedUpdateCmd, npmPrefixRunner
	t.Cleanup(func() { runUpdateCmd, runElevatedUpdateCmd, npmPrefixRunner = prevRun, prevElev, prevPrefix })
	// CI's ambient npm prefix is /usr, which would elevate that group and
	// leave runUpdateCmd. Pin a user prefix so both groups stay unelevated.
	npmPrefixRunner = func(context.Context) ([]byte, error) {
		return []byte("/home/u/.nvm/versions/node/v24\n"), nil
	}
	runElevatedUpdateCmd = func(ctx context.Context, opts Options, name string, args ...string) (string, string, error) {
		t.Errorf("user-owned prefixes must not elevate: %s %v", name, args)
		return "", "", context.DeadlineExceeded
	}
	var ran []string
	runUpdateCmd = func(ctx context.Context, opts Options, name string, args ...string) (string, string, error) {
		ran = append(ran, strings.Join(args, " "))
		return "updated", "", nil
	}

	items := []*model.Item{
		{Name: "legacy-pkg", Prefix: "/legacy", Category: model.CatNpm, Status: model.StatusOutdated},
		{Name: "ok-pkg", Category: model.CatNpm, Status: model.StatusOutdated},
	}
	results := batchNpmUpgrade(context.Background(), items, SilentOptions())
	if len(results) != 2 || len(ran) != 2 {
		t.Fatalf("results=%d ran=%v, want 2/2", len(results), len(ran))
	}
	if !slices.ContainsFunc(ran, func(cmd string) bool { return strings.Contains(cmd, flagPrefix+" /legacy") }) {
		t.Fatalf("legacy command lost its prefix: %v", ran)
	}
}

// Declined elevation skips the /usr prefix and still updates the user prefix.
func TestExecuteNpmSkippingElevated_RunsUserPrefix(t *testing.T) {
	prevRun, prevElev := runUpdateCmd, runElevatedUpdateCmd
	t.Cleanup(func() { runUpdateCmd, runElevatedUpdateCmd = prevRun, prevElev })
	var ran []string
	runUpdateCmd = func(ctx context.Context, opts Options, name string, args ...string) (string, string, error) {
		ran = append(ran, strings.Join(args, " "))
		return "updated", "", nil
	}
	runElevatedUpdateCmd = func(ctx context.Context, opts Options, name string, args ...string) (string, string, error) {
		t.Errorf("elevated npm must not run when elevation is skipped: %s %v", name, args)
		return "", "", context.DeadlineExceeded
	}

	user := &model.Item{Name: "user-pkg", Prefix: "/home/u/.npm-global", Category: model.CatNpm, Status: model.StatusOutdated}
	sys := &model.Item{Name: "sys-pkg", Prefix: "/usr", Category: model.CatNpm, Status: model.StatusOutdated}
	batch := &PreparedUpdateBatch{
		category: model.CatNpm,
		items:    []*model.Item{sys, user},
		plans: []CommandPlan{
			{Name: npmCommand, Args: []string{commandUpdate, flagGlobal, flagPrefix, "/home/u/.npm-global", "user-pkg"}, Scope: CommandScopeExact},
			{Name: npmCommand, Args: []string{commandUpdate, flagGlobal, flagPrefix, "/usr", "sys-pkg"}, Scope: CommandScopeExact, Elevated: true},
		},
	}
	if !PlansHaveUnelevatedWork(batch.Plans()) {
		t.Fatal("user prefix must count as unelevated work")
	}
	results := ExecuteNpmSkippingElevated(context.Background(), batch, SilentOptions(), "administrator password skipped")
	if len(ran) != 1 || !strings.Contains(ran[0], "/home/u/.npm-global") {
		t.Fatalf("ran = %v, want only the user prefix", ran)
	}
	byName := map[string]*Result{}
	for _, r := range results {
		byName[r.Item.Name] = r
	}
	if !byName["user-pkg"].Success || byName["user-pkg"].Item.Status != model.StatusDone {
		t.Fatalf("user prefix: %+v", byName["user-pkg"])
	}
	skipped := byName["sys-pkg"]
	if skipped.Success || !strings.HasPrefix(skipped.Error, "⊘ ") || skipped.Item.Status != model.StatusOutdated {
		t.Fatalf("system prefix must be skipped: %+v status=%v", skipped, skipped.Item.Status)
	}
}
