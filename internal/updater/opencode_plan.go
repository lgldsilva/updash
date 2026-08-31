package updater

import (
	"fmt"

	"github.com/lgldsilva/updash/internal/scanner"
)

// openCodeNpmPackage is the npm package backing the OpenCode CLI.
const openCodeNpmPackage = "opencode-ai"

// openCodeInstall resolves the local OpenCode installation. Variable so tests
// can describe an installation without touching the machine.
var openCodeInstall = scanner.OpenCodeInstall

// openCodeUpgradePlan decides how OpenCode gets updated on this machine.
//
// `opencode upgrade` prompts ("opencode is installed to … and may be managed by
// a package manager / Install anyways?") whenever it cannot detect its own
// install method. updash runs update commands without a stdin, so that prompt
// used to hang the whole run until the 20-minute batch timeout. Passing the
// method explicitly removes the prompt; when the binary lives in a directory
// the user cannot write to, `opencode upgrade` cannot succeed at all, so the
// update is driven by an elevated npm install pinned to that same prefix.
func openCodeUpgradePlan(base []string) CommandPlan {
	info := openCodeInstall()

	// A distro-owned binary belongs to its package manager; updash updates that
	// category separately and must not write over the package's files.
	if info.SystemPackage != "" {
		return manualPlan(fmt.Sprintf("OpenCode at %s is owned by the system package manager (%s) — update it there",
			info.BinPath, info.SystemPackage))
	}

	if info.SystemPrefix && info.NpmPrefix != "" {
		args := []string{"install", flagGlobal, "--prefix", info.NpmPrefix,
			"--allow-scripts=" + openCodeNpmPackage, openCodeNpmPackage + "@latest"}
		return CommandPlan{Name: npmCommand, Args: args, Scope: CommandScopeExact, Elevated: true}
	}

	if info.Method == scanner.OpenCodeMethodUnknown || len(base) == 0 {
		return manualPlan(openCodeManualReason(info))
	}

	args := append(append([]string{}, base[1:]...), "-m", info.Method)
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
