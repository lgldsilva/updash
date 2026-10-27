package updater

import (
	"fmt"

	"github.com/lgldsilva/updash/internal/model"
	"github.com/lgldsilva/updash/internal/scanner"
)

// openCodeNpmPackage is the v1 npm package. v2 uses scanner.OpenCodePackageV2.
const openCodeNpmPackage = scanner.OpenCodePackageV1

// openCodeInstall resolves the local OpenCode installation. Variable so tests
// can describe an installation without touching the machine.
var openCodeInstall = scanner.OpenCodeInstall

// openCodeUpgradePlan decides how OpenCode gets updated on this machine.
// The installed major selects the channel: v1 keeps `opencode upgrade -m
// <method>` (or an elevated npm install of opencode-ai); v2 follows
// @opencode/cli and refuses `-m curl` unless the resolved binary is the
// curl installer's own file.
func openCodeUpgradePlan(base []string, item *model.Item) CommandPlan {
	if openCodeItemV2(item) {
		return openCodeV2UpgradePlan(base)
	}
	return openCodeV1UpgradePlan(base)
}

func openCodeItemV2(item *model.Item) bool {
	if item == nil {
		return false
	}
	if item.PackageID == scanner.OpenCodePackageV2 {
		return true
	}
	return scanner.OpenCodeMajor(item.CurrentVer) >= 2
}

// openCodeV1UpgradePlan is the original headless plan. `opencode upgrade`
// prompts whenever it cannot detect its install method; passing -m removes
// the prompt. A non-writable global prefix is driven by elevated npm pinned
// to that prefix.
func openCodeV1UpgradePlan(base []string) CommandPlan {
	info := openCodeInstall()
	if plan, ok := openCodeOwnedOrPrefixPlan(info, openCodeNpmPackage); ok {
		return plan
	}
	if info.Method == scanner.OpenCodeMethodUnknown || len(base) == 0 {
		return manualPlan(openCodeManualReason(info))
	}
	return openCodeMethodPlan(base, info.Method)
}

// openCodeV2UpgradePlan updates the @opencode/cli channel. `-m curl` is only
// safe when the resolved path is ~/.opencode/bin/opencode itself. A v2
// standalone switched in from elsewhere (the v1 classifier calls ~/.local/bin
// "curl") must not take that flag: the v2 installer writes over
// ~/.opencode/bin/opencode and replaces the switch.
func openCodeV2UpgradePlan(base []string) CommandPlan {
	info := openCodeInstall()
	if plan, ok := openCodeOwnedOrPrefixPlan(info, scanner.OpenCodePackageV2); ok {
		return plan
	}
	if info.Method == scanner.OpenCodeMethodUnknown || len(base) == 0 {
		return manualPlan(openCodeV2ManualReason(info))
	}
	if info.Method == scanner.OpenCodeMethodCurl && !scanner.OpenCodeCanonicalCurl(info.BinPath) {
		return manualPlan(openCodeV2ManualReason(info))
	}
	return openCodeMethodPlan(base, info.Method)
}

// openCodeOwnedOrPrefixPlan handles the two cases shared by both channels:
// a distro-owned binary, and an npm prefix the user cannot write.
func openCodeOwnedOrPrefixPlan(info scanner.OpenCodeInstallInfo, pkg string) (CommandPlan, bool) {
	if info.SystemPackage != "" {
		return manualPlan(fmt.Sprintf("OpenCode at %s is owned by the system package manager (%s) — update it there",
			info.BinPath, info.SystemPackage)), true
	}
	if info.SystemPrefix && info.NpmPrefix != "" {
		args := []string{"install", flagGlobal, "--prefix", info.NpmPrefix,
			"--allow-scripts=" + pkg, pkg + "@latest"}
		return CommandPlan{Name: npmCommand, Args: args, Scope: CommandScopeExact, Elevated: true}, true
	}
	return CommandPlan{}, false
}

func openCodeMethodPlan(base []string, method string) CommandPlan {
	args := append(append([]string{}, base[1:]...), "-m", method)
	return CommandPlan{Name: base[0], Args: args, Scope: CommandScopeExact}
}

// openCodeManualReason explains why no safe automatic command exists, always
// with the recovery command the user can run themselves.
func openCodeManualReason(info scanner.OpenCodeInstallInfo) string {
	where := info.BinPath
	if where == "" {
		where = "not found on PATH"
	}
	return fmt.Sprintf("unknown OpenCode install method (%s) — run `%s`", where, opencodeReinstallHint)
}

// openCodeV2ManualReason refuses the v1 curl method. That command reinstalls
// over ~/.opencode/bin/opencode, which is how a version switch (v2 binary
// behind the opencode name, v1 kept as opencode-v1) gets replaced by a stale
// copy.
func openCodeV2ManualReason(info scanner.OpenCodeInstallInfo) string {
	where := info.BinPath
	if where == "" {
		where = "not found on PATH"
	}
	return fmt.Sprintf("OpenCode v2 at %s is a standalone binary on the %s channel; `opencode upgrade -m curl` would replace the version switch — update that binary in place",
		where, scanner.OpenCodePackageV2)
}
