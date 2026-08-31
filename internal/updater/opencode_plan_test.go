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
	plan := openCodeUpgradePlan(openCodeBaseCmd())
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
	plan := openCodeUpgradePlan(openCodeBaseCmd())
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
	plan := openCodeUpgradePlan(openCodeBaseCmd())
	if plan.Scope != CommandScopeManual {
		t.Fatalf("plan = %+v, want manual", plan)
	}
	if !strings.Contains(plan.Manual, opencodeReinstallHint) || !strings.Contains(plan.Manual, "/somewhere/odd/opencode") {
		t.Fatalf("manual reason must name the path and the recovery command: %q", plan.Manual)
	}
}

func TestOpenCodeUpgradePlan_NotInstalledIsManual(t *testing.T) {
	stubOpenCodeInstall(t, scanner.OpenCodeInstallInfo{Method: scanner.OpenCodeMethodUnknown})
	plan := openCodeUpgradePlan(nil)
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
	plan := openCodeUpgradePlan(openCodeBaseCmd())
	if plan.Scope != CommandScopeManual {
		t.Fatalf("plan = %+v, want manual", plan)
	}
	if !strings.Contains(plan.Manual, "opencode-bin 1.2.3") {
		t.Fatalf("manual reason must name the owning package: %q", plan.Manual)
	}
}
