package updater

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/lgldsilva/updash/internal/model"
)

func TestRunUpdateCmd_StreamsToConfiguredOutput(t *testing.T) {
	var output bytes.Buffer
	stdout, stderr, err := runUpdateCmd(context.Background(), Options{Output: &output}, "sh", "-c", "printf streamed")
	if err != nil {
		t.Fatal(err)
	}
	if stdout != "" || stderr != "" {
		t.Fatalf("streamed command returned buffered output stdout=%q stderr=%q", stdout, stderr)
	}
	if output.String() != "streamed" {
		t.Fatalf("streamed output = %q, want %q", output.String(), "streamed")
	}
}

func TestOutputNeedsPassword_DoesNotTreatTerminationAsPasswordFailure(t *testing.T) {
	for _, output := range []string{"signal: killed", "process killed", "signal: terminated"} {
		if OutputNeedsPassword(output) {
			t.Errorf("OutputNeedsPassword(%q) = true, want false", output)
		}
	}
}

func TestBatchPnpmUpgrade_ExecutesOnlySelectedPackages(t *testing.T) {
	original := runUpdateCmd
	t.Cleanup(func() { runUpdateCmd = original })
	var calls [][]string
	runUpdateCmd = func(_ context.Context, _ Options, name string, args ...string) (string, string, error) {
		calls = append(calls, append([]string{name}, args...))
		return "updated", "", nil
	}

	items := []*model.Item{{Name: "one", Category: model.CatPnpm}, {Name: "two", Category: model.CatPnpm}}
	results := batchPnpmUpgrade(context.Background(), items, SilentOptions())
	if len(results) != 2 || !results[0].Success || !results[1].Success {
		t.Fatalf("results = %#v", results)
	}
	want := [][]string{{"pnpm", "update", "-g", "one"}, {"pnpm", "update", "-g", "two"}}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("commands = %#v, want %#v", calls, want)
	}
}

func TestBatchAptUpgrade_StopsWhenRefreshFails(t *testing.T) {
	original := runElevatedUpdateCmd
	t.Cleanup(func() { runElevatedUpdateCmd = original })
	var calls [][]string
	runElevatedUpdateCmd = func(_ context.Context, _ Options, name string, args ...string) (string, string, error) {
		calls = append(calls, append([]string{name}, args...))
		return "", "refresh failed", errors.New("refresh failed")
	}

	items := []*model.Item{{Name: "curl", Category: model.CatApt}}
	results := batchAptUpgrade(context.Background(), items, SilentOptions())
	if len(results) != 1 || results[0].Success {
		t.Fatalf("results = %#v", results)
	}
	want := [][]string{{"apt-get", "update"}}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("commands = %#v, want %#v", calls, want)
	}
}

func TestNpmGlobalNeedsSudo_InspectsSystemPrefixBeforeElevationIsReady(t *testing.T) {
	original := npmPrefixRunner
	t.Cleanup(func() { npmPrefixRunner = original })
	npmPrefixRunner = func(context.Context) ([]byte, error) { return []byte("/usr/local\n"), nil }
	if !npmGlobalNeedsSudo(context.Background()) {
		t.Fatal("system npm prefix must request elevation")
	}
}

func TestPlanUpdateCommands_TargetsOnlySelectedPackages(t *testing.T) {
	tests := []struct {
		category model.Category
		items    []*model.Item
		want     []CommandPlan
	}{
		{
			category: model.CatPnpm,
			items:    []*model.Item{{Name: "one"}, {Name: "two"}},
			want: []CommandPlan{
				{Name: "pnpm", Args: []string{"update", "-g", "one"}, Scope: CommandScopeExact},
				{Name: "pnpm", Args: []string{"update", "-g", "two"}, Scope: CommandScopeExact},
			},
		},
		{
			category: model.CatBun,
			items:    []*model.Item{{Name: "one"}},
			want:     []CommandPlan{{Name: "bun", Args: []string{"update", "-g", "one"}, Scope: CommandScopeExact}},
		},
		{
			category: model.CatPipx,
			items:    []*model.Item{{Name: "one"}, {Name: "two"}},
			want: []CommandPlan{
				{Name: "pipx", Args: []string{"upgrade", "one"}, Scope: CommandScopeExact},
				{Name: "pipx", Args: []string{"upgrade", "two"}, Scope: CommandScopeExact},
			},
		},
	}
	for _, tt := range tests {
		t.Run(string(tt.category), func(t *testing.T) {
			got, err := PlanUpdateCommands(tt.category, tt.items)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("PlanUpdateCommands() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestPlanUpdateCommands_CoversAutomaticAndManualCategories(t *testing.T) {
	tests := []struct {
		name  string
		cat   model.Category
		item  *model.Item
		scope CommandScope
		argv  []string
	}{
		{"brew", model.CatBrew, &model.Item{Name: "git"}, CommandScopeExact, []string{"brew", "upgrade", "--greedy", "git"}},
		{"mas", model.CatMAS, &model.Item{Name: "Xcode", PackageID: "497799835"}, CommandScopeExact, []string{"mas", "update", "497799835"}},
		{"dnf", model.CatDnf, &model.Item{Name: "git"}, CommandScopeCategoryGlobal, nil},
		{"flatpak", model.CatFlatpak, &model.Item{Name: "org.example.App"}, CommandScopeCategoryGlobal, []string{"flatpak", "update", "-y"}},
		{"snap", model.CatSnap, &model.Item{Name: "core"}, CommandScopeCategoryGlobal, []string{"snap", "refresh"}},
		{"choco", model.CatChoco, &model.Item{Name: "git"}, CommandScopeExact, []string{"choco", "upgrade", "git", "-y"}},
		{"agent manual", model.CatAgent, &model.Item{Name: "Cursor"}, CommandScopeManual, nil},
		{"cleanup manual", model.CatCache, &model.Item{Name: "cache"}, CommandScopeManual, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plans, err := PlanUpdateCommands(tt.cat, []*model.Item{tt.item})
			if err != nil || len(plans) != 1 {
				t.Fatalf("plans=%#v err=%v", plans, err)
			}
			if plans[0].Scope != tt.scope {
				t.Fatalf("scope=%q want %q", plans[0].Scope, tt.scope)
			}
			if tt.argv != nil && !reflect.DeepEqual(append([]string{plans[0].Name}, plans[0].Args...), tt.argv) {
				t.Fatalf("argv=%v want %v", append([]string{plans[0].Name}, plans[0].Args...), tt.argv)
			}
		})
	}
}

func TestPlanUpdateCommands_AptRefreshesOnceThenTargetsSelection(t *testing.T) {
	items := []*model.Item{{Name: "curl"}, {Name: "git"}}
	got, err := PlanUpdateCommands(model.CatApt, items)
	if err != nil {
		t.Fatal(err)
	}
	want := []CommandPlan{
		{Name: "apt-get", Args: []string{"update"}, Scope: CommandScopeCategoryGlobal, Elevated: true},
		{Name: "apt-get", Args: []string{"install", "--only-upgrade", "-y", "curl", "git"}, Scope: CommandScopeExact, Elevated: true},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("PlanUpdateCommands() = %#v, want %#v", got, want)
	}
}

func TestPlanUpdateCommands_EmptyAndMissingIDsDoNotBroadenScope(t *testing.T) {
	for _, category := range []model.Category{model.CatBrew, model.CatWinget, model.CatFlatpak, model.CatAgent} {
		plans, err := PlanUpdateCommands(category, nil)
		if err != nil || len(plans) != 0 {
			t.Fatalf("%s empty plans=%#v err=%v", category, plans, err)
		}
	}
	plans, err := PlanUpdateCommands(model.CatMAS, []*model.Item{{Name: "Xcode"}})
	if err != nil || len(plans) != 1 || plans[0].Scope != CommandScopeManual {
		t.Fatalf("MAS missing ID plans=%#v err=%v", plans, err)
	}
	if _, err := PlanUpdateCommands(model.CatWinget, []*model.Item{{Name: "Notepad++"}}); err == nil {
		t.Fatal("winget must reject a missing PackageID")
	}
	if _, err := PlanUpdateCommands(model.CatChoco, []*model.Item{{Name: ""}}); err == nil {
		t.Fatal("choco must reject an empty exact target")
	}
}

func TestUpgradeMASApp_MissingIDIsManualAndDoesNotExecute(t *testing.T) {
	item := &model.Item{Name: "Xcode", Category: model.CatMAS}
	result := upgradeMASApp(context.Background(), item, SilentOptions())
	if result.Success || !strings.Contains(result.Error, "App Store ID") {
		t.Fatalf("result=%#v", result)
	}
}

func TestPlansRequireWholeCategory(t *testing.T) {
	global, _ := PlanUpdateCommands(model.CatFlatpak, []*model.Item{{Name: "app"}})
	if !PlansRequireWholeCategory(global) {
		t.Fatal("flatpak global plan must require whole-category confirmation")
	}
	exact, _ := PlanUpdateCommands(model.CatBrew, []*model.Item{{Name: "git"}})
	if PlansRequireWholeCategory(exact) {
		t.Fatal("exact brew plan must permit a partial selection")
	}
}

func TestPlansRequireElevation_UsesEmbeddedNpmPreflight(t *testing.T) {
	original := npmPrefixRunner
	t.Cleanup(func() { npmPrefixRunner = original })
	npmPrefixRunner = func(context.Context) ([]byte, error) { return []byte("/usr/local\n"), nil }
	plans, err := PlanUpdateCommands(model.CatNpm, []*model.Item{{Name: "example"}})
	if err != nil || !PlansRequireElevation(plans) {
		t.Fatalf("plans=%#v err=%v", plans, err)
	}
}

func TestPlanNpm_FailsClosedWhenPrefixCannotBeDetermined(t *testing.T) {
	original := npmPrefixRunner
	t.Cleanup(func() { npmPrefixRunner = original })
	npmPrefixRunner = func(context.Context) ([]byte, error) { return nil, errors.New("npm unavailable") }
	if _, err := PlanUpdateCommands(model.CatNpm, []*model.Item{{Name: "example"}}); err == nil {
		t.Fatal("npm plan must fail closed when global prefix cannot be determined")
	}
}

func TestPlanNpm_RejectsMixedEmptyAndValidNames(t *testing.T) {
	if _, err := PlanUpdateCommands(model.CatNpm, []*model.Item{{Name: "valid"}, {Name: ""}}); err == nil {
		t.Fatal("npm plan must reject every empty item before deduplication")
	}
}

func TestPreparedNpmBatch_PlansOnceAndExecutesStoredPlan(t *testing.T) {
	originalPrefix := npmPrefixRunner
	originalElevated := runElevatedUpdateCmd
	t.Cleanup(func() { npmPrefixRunner = originalPrefix; runElevatedUpdateCmd = originalElevated })
	prefixCalls := 0
	npmPrefixRunner = func(context.Context) ([]byte, error) { prefixCalls++; return []byte("/usr/local\n"), nil }
	var executed []string
	runElevatedUpdateCmd = func(_ context.Context, _ Options, name string, args ...string) (string, string, error) {
		executed = append([]string{name}, args...)
		return "ok", "", nil
	}

	batch, err := PrepareUpdateBatch(context.Background(), model.CatNpm, []*model.Item{{Name: "example"}})
	if err != nil {
		t.Fatal(err)
	}
	if prefixCalls != 1 || !PlansRequireElevation(batch.Plans()) {
		t.Fatalf("preflight calls=%d plans=%#v", prefixCalls, batch.Plans())
	}
	results := ExecutePreparedBatch(context.Background(), batch, SilentOptions())
	if len(results) != 1 || !results[0].Success {
		t.Fatalf("results=%#v", results)
	}
	if prefixCalls != 1 {
		t.Fatalf("executor replanned npm prefix: %d calls", prefixCalls)
	}
	want := []string{"npm", "update", "-g", "example", "--allow-scripts=example"}
	if !reflect.DeepEqual(executed, want) {
		t.Fatalf("executed=%v want=%v", executed, want)
	}
}

func TestUpdateCategory_GlobalPlanExecutesOnce(t *testing.T) {
	original := runUpdateCmd
	t.Cleanup(func() { runUpdateCmd = original })
	var calls [][]string
	runUpdateCmd = func(_ context.Context, _ Options, name string, args ...string) (string, string, error) {
		calls = append(calls, append([]string{name}, args...))
		return "ok", "", nil
	}
	items := []*model.Item{{Name: "first", Category: model.CatFlatpak}, {Name: "second", Category: model.CatFlatpak}}
	results := UpdateCategory(context.Background(), model.CatFlatpak, items, SilentOptions())
	if len(results) != 2 || !results[0].Success || !results[1].Success {
		t.Fatalf("results=%#v", results)
	}
	want := [][]string{{"flatpak", "update", "-y"}}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls=%#v want=%#v", calls, want)
	}
}

func TestPlanUpdateCommands_EveryKnownCategoryHasAnExplicitScope(t *testing.T) {
	categories := []model.Category{
		model.CatBrew, model.CatMAS, model.CatApt, model.CatDnf, model.CatZypper, model.CatApk, model.CatPacman, model.CatFlatpak, model.CatSnap,
		model.CatWinget, model.CatChoco, model.CatScoop, model.CatNpm, model.CatPnpm, model.CatBun, model.CatPipx, model.CatGo, model.CatRustup,
		model.CatCargo, model.CatSDKMAN, model.CatDocker, model.CatWatchtower, model.CatCloud, model.CatAI, model.CatAgent, model.CatGHExt,
		model.CatNvm, model.CatOpenCodePlugins, model.CatOmz, model.CatCache, model.CatSDKClean, model.CatVSCodeClean, model.CatDockerClean, model.CatHomelabClean,
	}
	for _, category := range categories {
		t.Run(string(category), func(t *testing.T) {
			plans, err := PlanUpdateCommands(category, []*model.Item{{Name: "example", PackageID: "example.id"}})
			if err != nil || len(plans) == 0 {
				t.Fatalf("plans=%#v err=%v", plans, err)
			}
			for _, plan := range plans {
				if plan.Scope == "" {
					t.Fatalf("plan has no scope: %#v", plan)
				}
			}
		})
	}
}

func TestUpdateCategory_EmptyGlobalCategoriesAreNoops(t *testing.T) {
	categories := []model.Category{
		model.CatApt,
		model.CatDnf,
		model.CatZypper,
		model.CatApk,
		model.CatPacman,
		model.CatWinget,
		model.CatChoco,
		model.CatScoop,
		model.CatPnpm,
		model.CatBun,
		model.CatPipx,
	}
	for _, category := range categories {
		t.Run(string(category), func(t *testing.T) {
			got := UpdateCategory(context.Background(), category, nil, SilentOptions())
			if len(got) != 0 {
				t.Fatalf("UpdateCategory(%s, nil) = %d results, want no-op", category, len(got))
			}
		})
	}
}

func TestDirectGlobalBatches_EmptyInputIsNoop(t *testing.T) {
	batches := []struct {
		name string
		run  func(context.Context, []*model.Item, Options) []*Result
	}{
		{"dnf", batchDnfUpgrade},
		{"zypper", batchZypperUpgrade},
		{"apk", batchApkUpgrade},
		{"pacman", batchPacmanUpgrade},
		{"choco", batchChocoUpgrade},
		{"scoop", batchScoopUpgrade},
	}
	for _, batch := range batches {
		t.Run(batch.name, func(t *testing.T) {
			if got := batch.run(context.Background(), nil, SilentOptions()); len(got) != 0 {
				t.Fatalf("got %d results, want no-op", len(got))
			}
		})
	}
}
