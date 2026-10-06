package updater

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/lgldsilva/updash/internal/model"
	"github.com/lgldsilva/updash/internal/scanner"
)

func openCodeBaseCmd() []string { return scanner.AgentUpdateCommand(agentOpenCode) }

// A detectable, user-owned install upgrades in place with an explicit method:
// that is what stops `opencode upgrade` from prompting.
func TestOpenCodeUpgradePlan_ExplicitMethod(t *testing.T) {
	stubOpenCodeInstall(t, scanner.OpenCodeInstallInfo{
		Method:  scanner.OpenCodeMethodCurl,
		BinPath: "/home/u/.opencode/bin/opencode",
	})
	plan := openCodeUpgradePlan(openCodeBaseCmd(), nil)
	want := []string{"upgrade", "-m", scanner.OpenCodeMethodCurl}
	if plan.Name != "opencode" || !slices.Equal(plan.Args, want) {
		t.Fatalf("plan = %s %v, want opencode %v", plan.Name, plan.Args, want)
	}
	if plan.Elevated || plan.Scope != CommandScopeExact {
		t.Fatalf("unexpected scope/elevation: %+v", plan)
	}
}

// The Manjaro case: /usr/lib/node_modules is not writable, so the update is
// driven by npm with elevation, pinned to the prefix that owns the binary.
func TestOpenCodeUpgradePlan_SystemPrefixFallsBackToElevatedNpm(t *testing.T) {
	stubOpenCodeInstall(t, scanner.OpenCodeInstallInfo{
		Method:       scanner.OpenCodeMethodNpm,
		BinPath:      "/usr/lib/node_modules/opencode-ai/bin/opencode.exe",
		SystemPrefix: true,
		NpmPrefix:    "/usr",
	})
	plan := openCodeUpgradePlan(openCodeBaseCmd(), nil)
	want := []string{"install", "-g", "--prefix", "/usr", "--allow-scripts=opencode-ai", "opencode-ai@latest"}
	if plan.Name != npmCommand || !slices.Equal(plan.Args, want) {
		t.Fatalf("plan = %s %v, want npm %v", plan.Name, plan.Args, want)
	}
	if !plan.Elevated {
		t.Fatal("a system prefix requires elevation")
	}
}

// Unknown method and no npm prefix to fall back to: never run the interactive
// command, report the manual recovery instead.
func TestOpenCodeUpgradePlan_UnknownIsManual(t *testing.T) {
	stubOpenCodeInstall(t, scanner.OpenCodeInstallInfo{
		Method:  scanner.OpenCodeMethodUnknown,
		BinPath: "/somewhere/odd/opencode",
	})
	plan := openCodeUpgradePlan(openCodeBaseCmd(), nil)
	if plan.Scope != CommandScopeManual {
		t.Fatalf("plan = %+v, want manual", plan)
	}
	if !strings.Contains(plan.Manual, opencodeReinstallHint) || !strings.Contains(plan.Manual, "/somewhere/odd/opencode") {
		t.Fatalf("manual reason must name the path and the recovery command: %q", plan.Manual)
	}
}

func TestOpenCodeUpgradePlan_NotInstalledIsManual(t *testing.T) {
	stubOpenCodeInstall(t, scanner.OpenCodeInstallInfo{Method: scanner.OpenCodeMethodUnknown})
	plan := openCodeUpgradePlan(nil, nil)
	if plan.Scope != CommandScopeManual || !strings.Contains(plan.Manual, "not found on PATH") {
		t.Fatalf("plan = %+v, want manual naming the missing binary", plan)
	}
}

// The dry-run must show exactly what execution will run.
func TestPlanUpdateCommands_OpenCodeMatchesExecution(t *testing.T) {
	stubOpenCodeInstall(t, scanner.OpenCodeInstallInfo{
		Method:       scanner.OpenCodeMethodNpm,
		BinPath:      "/usr/lib/node_modules/opencode-ai/bin/opencode",
		SystemPrefix: true,
		NpmPrefix:    "/usr",
	})
	item := &model.Item{Name: agentOpenCode, Category: model.CatAgent}
	plans, err := planUpdateCommands(context.Background(), model.CatAgent, []*model.Item{item})
	if err != nil || len(plans) != 1 {
		t.Fatalf("plans = %v, err = %v", plans, err)
	}
	if plans[0].Name != npmCommand || !plans[0].Elevated {
		t.Fatalf("planned %+v, want the elevated npm fallback", plans[0])
	}
}

// A distro-packaged OpenCode is left to its package manager: writing npm files
// over a pacman/apt-owned tree would corrupt the package database.
func TestOpenCodeUpgradePlan_SystemPackageIsManual(t *testing.T) {
	stubOpenCodeInstall(t, scanner.OpenCodeInstallInfo{
		Method:        scanner.OpenCodeMethodNpm,
		BinPath:       "/usr/lib/node_modules/opencode-ai/bin/opencode",
		SystemPrefix:  true,
		NpmPrefix:     "/usr",
		SystemPackage: "opencode-bin 1.2.3",
	})
	plan := openCodeUpgradePlan(openCodeBaseCmd(), nil)
	if plan.Scope != CommandScopeManual {
		t.Fatalf("plan = %+v, want manual", plan)
	}
	if !strings.Contains(plan.Manual, "opencode-bin 1.2.3") {
		t.Fatalf("manual reason must name the owning package: %q", plan.Manual)
	}
}

func openCodeV2Item(ver string) *model.Item {
	return &model.Item{
		Name: agentOpenCode, Category: model.CatAgent,
		CurrentVer: ver, PackageID: scanner.OpenCodePackageV2,
	}
}

// v1 still treats ~/.local/bin as the curl method. The refusal is v2-only.
func TestOpenCodeUpgradePlan_V1LocalBinStillCurl(t *testing.T) {
	stubOpenCodeInstall(t, scanner.OpenCodeInstallInfo{
		Method:  scanner.OpenCodeMethodCurl,
		BinPath: "/home/u/.local/bin/opencode",
	})
	item := &model.Item{Name: agentOpenCode, CurrentVer: "1.18.4", PackageID: scanner.OpenCodePackageV1}
	plan := openCodeUpgradePlan(openCodeBaseCmd(), item)
	want := []string{"upgrade", "-m", scanner.OpenCodeMethodCurl}
	if plan.Name != "opencode" || !slices.Equal(plan.Args, want) {
		t.Fatalf("plan = %s %v, want opencode %v", plan.Name, plan.Args, want)
	}
}

// A v2 standalone switched in via ~/.local/bin must not be upgraded with
// -m curl: that reinstalls over ~/.opencode/bin/opencode and drops the switch.
func TestOpenCodeUpgradePlan_V2StandaloneDoesNotUseCurl(t *testing.T) {
	stubOpenCodeInstall(t, scanner.OpenCodeInstallInfo{
		Method:  scanner.OpenCodeMethodCurl,
		BinPath: "/home/u/.local/bin/opencode-v2",
	})
	plan := openCodeUpgradePlan(openCodeBaseCmd(), openCodeV2Item("2.0.24"))
	if plan.Scope != CommandScopeManual {
		t.Fatalf("plan = %+v, want manual", plan)
	}
	if strings.Contains(strings.Join(plan.Args, " "), "curl") {
		t.Fatalf("v2 switch must not be planned as curl: %+v", plan)
	}
	if !strings.Contains(plan.Manual, scanner.OpenCodePackageV2) || !strings.Contains(plan.Manual, "opencode-v2") {
		t.Fatalf("manual reason must name the v2 channel and the binary: %q", plan.Manual)
	}
	if strings.Contains(plan.Manual, "opencode-ai") {
		t.Fatalf("v2 manual reason must not point at the v1 package: %q", plan.Manual)
	}
}

// The major alone selects v2, even when PackageID is still the catalog default.
func TestOpenCodeUpgradePlan_V2MajorWithoutPackageID(t *testing.T) {
	stubOpenCodeInstall(t, scanner.OpenCodeInstallInfo{
		Method:  scanner.OpenCodeMethodCurl,
		BinPath: "/home/u/.local/bin/opencode-v2",
	})
	item := &model.Item{Name: agentOpenCode, CurrentVer: "opencode v2.0.24", PackageID: scanner.OpenCodePackageV1}
	plan := openCodeUpgradePlan(openCodeBaseCmd(), item)
	if plan.Scope != CommandScopeManual {
		t.Fatalf("version major must select the v2 plan, got %+v", plan)
	}
}

// A real curl install — the file is the current user's ~/.opencode/bin/opencode,
// not a switch and not another tree with the same suffix — still uses -m curl.
func TestOpenCodeUpgradePlan_V2CanonicalCurl(t *testing.T) {
	t.Setenv("HOME", "/home/u")
	stubOpenCodeInstall(t, scanner.OpenCodeInstallInfo{
		Method:  scanner.OpenCodeMethodCurl,
		BinPath: "/home/u/.opencode/bin/opencode",
	})
	plan := openCodeUpgradePlan(openCodeBaseCmd(), openCodeV2Item("2.0.24"))
	want := []string{"upgrade", "-m", scanner.OpenCodeMethodCurl}
	if plan.Name != "opencode" || !slices.Equal(plan.Args, want) {
		t.Fatalf("plan = %s %v, want opencode %v", plan.Name, plan.Args, want)
	}
}

// Another tree that only ends in /.opencode/bin/opencode is not this user's
// installer. -m curl would still write ~/.opencode/bin/opencode.
func TestOpenCodeUpgradePlan_V2ForeignSuffixIsNotCurl(t *testing.T) {
	t.Setenv("HOME", "/home/u")
	stubOpenCodeInstall(t, scanner.OpenCodeInstallInfo{
		Method:  scanner.OpenCodeMethodCurl,
		BinPath: "/srv/other/.opencode/bin/opencode",
	})
	plan := openCodeUpgradePlan(openCodeBaseCmd(), openCodeV2Item("2.0.24"))
	if plan.Scope != CommandScopeManual {
		t.Fatalf("plan = %+v, want manual", plan)
	}
	if strings.Contains(strings.Join(plan.Args, " "), "curl") {
		t.Fatalf("foreign suffix must not be planned as curl: %+v", plan)
	}
}

func TestOpenCodeUpgradePlan_V2NpmMethod(t *testing.T) {
	stubOpenCodeInstall(t, scanner.OpenCodeInstallInfo{
		Method:  scanner.OpenCodeMethodNpm,
		BinPath: "/home/u/.local/lib/node_modules/@opencode/cli/bin/opencode",
	})
	plan := openCodeUpgradePlan(openCodeBaseCmd(), openCodeV2Item("2.0.24"))
	want := []string{"upgrade", "-m", scanner.OpenCodeMethodNpm}
	if plan.Name != "opencode" || !slices.Equal(plan.Args, want) {
		t.Fatalf("plan = %s %v, want opencode %v", plan.Name, plan.Args, want)
	}
}

func TestOpenCodeUpgradePlan_V2SystemPrefixUsesChannelPackage(t *testing.T) {
	stubOpenCodeInstall(t, scanner.OpenCodeInstallInfo{
		Method:       scanner.OpenCodeMethodNpm,
		BinPath:      "/usr/lib/node_modules/@opencode/cli/bin/opencode",
		SystemPrefix: true,
		NpmPrefix:    "/usr",
	})
	plan := openCodeUpgradePlan(openCodeBaseCmd(), openCodeV2Item("2.0.24"))
	want := []string{"install", "-g", "--prefix", "/usr", "--allow-scripts=@opencode/cli", "@opencode/cli@latest"}
	if plan.Name != npmCommand || !slices.Equal(plan.Args, want) || !plan.Elevated {
		t.Fatalf("plan = %+v, want elevated npm of @opencode/cli", plan)
	}
}
