package scanner

import (
	"context"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/lgldsilva/updash/internal/model"
)

// AgentSource scans AI coding assistant tools.
type AgentSource struct{}

func (s *AgentSource) Category() model.Category { return model.CatAgent }
func (s *AgentSource) Label() string            { return "AI Agents" }
func (s *AgentSource) Icon() string             { return "🤖" }

const (
	toolAIMemory     = "ai-memory"
	policyManual     = "manual reinstall / app update"
	verNoneInstalled = "none installed"
)

var semverRE = regexp.MustCompile(`\d+\.\d+\.\d+[a-zA-Z0-9.-]*`)

// parseAgentVersion extracts the first semver-like version from CLI output.
// Handles multi-line, parenthetical comments, and varied formats.
func parseAgentVersion(output string) string {
	firstLine := strings.SplitN(strings.TrimSpace(output), scannerNL, 2)[0]
	if m := semverRE.FindString(firstLine); m != "" {
		return m
	}
	parts := strings.Fields(firstLine)
	if len(parts) == 0 {
		return firstLine
	}
	last := parts[len(parts)-1]
	if strings.HasPrefix(last, "(") || strings.HasPrefix(last, "[") {
		if len(parts) >= 2 {
			return parts[len(parts)-2]
		}
	}
	last = strings.TrimSuffix(last, versionDot)
	last = strings.TrimSuffix(last, ",")
	return last
}

// agentUpdateMode describes how an agent can be upgraded.
type agentUpdateMode int

const (
	agentUpdateAuto agentUpdateMode = iota
	agentUpdateManual
)

// agentDef is the single data-driven description of one AI coding assistant:
// how to probe its version, how to learn the latest release, and how to
// upgrade it. Adding an agent = adding one entry here (no updater changes).
type agentDef struct {
	name       string
	binary     string
	verCmd     []string
	mode       agentUpdateMode
	keepPolicy string   // manual-mode reason shown to the user ("" = policyManual)
	npmPackage string   // npm package name: drives registry latest lookup + npm-based update
	updateCmd  []string // explicit upgrade command (required for auto mode without npmPackage)
	latestCmd  []string // optional: command whose stdout holds the latest version
}

func agentCatalog() []agentDef {
	return []agentDef{
		{name: "Claude Code", binary: binClaude, verCmd: []string{binClaude, flagVersion}, mode: agentUpdateAuto, npmPackage: "@anthropic-ai/claude-code", updateCmd: []string{binClaude, cmdUpdate}},
		{name: "OpenCode", binary: binOpenCode, verCmd: []string{binOpenCode, flagVersion}, mode: agentUpdateAuto, updateCmd: []string{binOpenCode, cmdUpgrade}},
		{name: "Grok", binary: binGrok, verCmd: []string{binGrok, flagVersion}, mode: agentUpdateAuto, updateCmd: []string{binGrok, cmdUpdate}},
		{name: "Antigravity", binary: binAntigravity, verCmd: []string{binAntigravity, flagVersion}, mode: agentUpdateManual},
		{name: "Agy", binary: "agy", verCmd: []string{"agy", flagVersion}, mode: agentUpdateManual},
		{name: "MimoCode", binary: "mimo", verCmd: []string{"mimo", flagVersion}, mode: agentUpdateManual},
		{name: "Codex", binary: "codex", verCmd: []string{"codex", flagVersion}, mode: agentUpdateAuto, npmPackage: "@openai/codex", updateCmd: npmGlobalInstallCmd("@openai/codex")},
		{name: "Gemini CLI", binary: binGemini, verCmd: []string{binGemini, flagVersion}, mode: agentUpdateAuto, npmPackage: "@google/gemini-cli", updateCmd: []string{binGemini, cmdUpdate}},
		{name: "Copilot CLI", binary: binCopilot, verCmd: []string{binCopilot, flagVersion}, mode: agentUpdateAuto, updateCmd: []string{binCopilot, cmdUpdate}},
		{name: "Crush", binary: "crush", verCmd: []string{"crush", flagVersion}, mode: agentUpdateManual},
		{name: "Cursor", binary: binCursor, verCmd: []string{binCursor, flagVersion}, mode: agentUpdateManual},
		{name: binPi, binary: binPi, verCmd: []string{binPi, flagVersion}, mode: agentUpdateAuto, npmPackage: "@earendil-works/pi-coding-agent", updateCmd: npmGlobalInstallCmd("@earendil-works/pi-coding-agent")},
		{name: "Qwen Code", binary: "qwen", verCmd: []string{"qwen", flagVersion}, mode: agentUpdateAuto, npmPackage: "@qwen-code/qwen-code", updateCmd: npmGlobalInstallCmd("@qwen-code/qwen-code")},
		{name: "Aider", binary: "aider", verCmd: []string{"aider", flagVersion}, mode: agentUpdateManual, keepPolicy: "pipx upgrade aider (or pip install -U aider)"},
		{name: "Amazon Q", binary: "q", verCmd: []string{"q", flagVersion}, mode: agentUpdateManual, keepPolicy: "q doctor / installer re-run"},
		{name: "Windsurf", binary: binWindsurf, verCmd: []string{binWindsurf, flagVersion}, mode: agentUpdateManual},
	}
}

// lookupAgentDef returns the catalog entry matching an item name.
func lookupAgentDef(name string) (agentDef, bool) {
	for _, a := range agentCatalog() {
		if a.name == name {
			return a, true
		}
	}
	return agentDef{}, false
}

// AgentUpdateCommand returns the upgrade command for an auto-update agent
// (nil for manual/unknown agents). Data-driven: mirrors agentCatalog.
func AgentUpdateCommand(name string) []string {
	a, ok := lookupAgentDef(name)
	if !ok || a.mode != agentUpdateAuto {
		return nil
	}
	if len(a.updateCmd) > 0 {
		return a.updateCmd
	}
	if a.npmPackage != "" {
		return npmGlobalInstallCmd(a.npmPackage)
	}
	return nil
}

// npmGlobalInstallCmd builds a global npm install for one agent package.
// npm >= 12 blocks install scripts by default unless allowScripts covers
// the package (RFC npm/rfcs#868), silently skipping postinstall — which
// breaks wrapper packages such as @anthropic-ai/claude-code ("native binary
// not installed"). Allow scripts explicitly for the package being
// installed; older npm versions ignore the unknown config key.
func npmGlobalInstallCmd(pkg string) []string {
	return []string{binNpm, cmdInstall, flagGlobal, "--allow-scripts=" + pkg, pkg + "@latest"}
}

// AgentKeepPolicy returns the manual-mode reason for an agent ("" = auto).
func AgentKeepPolicy(name string) string {
	a, ok := lookupAgentDef(name)
	if !ok || a.mode != agentUpdateManual {
		return ""
	}
	if a.keepPolicy != "" {
		return a.keepPolicy
	}
	return policyManual
}

func (s *AgentSource) Scan(ctx context.Context, plat model.PlatformInfo) ([]*model.Item, error) {
	var items []*model.Item
	catalog := agentCatalog()
	for _, a := range catalog {
		if _, err := exec.LookPath(a.binary); err != nil {
			continue
		}
		items = append(items, probeAgentItem(ctx, plat, a))
	}
	if len(items) == 0 {
		return []*model.Item{
			{Name: "agents", Category: model.CatAgent, Status: model.StatusOK, CurrentVer: verNoneInstalled},
		}, nil
	}
	if plat.HasNpm {
		applyNpmOutdatedToAgents(ctx, items, catalog)
		resolveRegistryLatest(ctx, items, catalog)
	}
	return items, nil
}

func probeAgentItem(ctx context.Context, plat model.PlatformInfo, a agentDef) *model.Item {
	it := &model.Item{
		Name:     a.name,
		Category: model.CatAgent,
		Status:   model.StatusOK,
	}
	if a.npmPackage != "" {
		it.PackageID = a.npmPackage
	}
	if a.mode == agentUpdateManual {
		if a.keepPolicy != "" {
			it.KeepPolicy = a.keepPolicy
		} else {
			it.KeepPolicy = policyManual
		}
	}
	if len(a.verCmd) == 0 {
		return it
	}
	if agentSkipVersionProbe(plat, a.binary) {
		it.CurrentVer = statusInstalled
		return it
	}
	it.CurrentVer = probeAgentVersion(ctx, a.verCmd)
	return it
}

// npmInstalledPackages lists globally npm-installed package names (depth 0).
func npmInstalledPackages(ctx context.Context) map[string]bool {
	out, err := execCombined(ctx, binNpm, "ls", flagGlobal, "--json", "--depth=0")
	installed := ParseNpmLsGlobal(out)
	if err != nil && len(installed) == 0 {
		return nil
	}
	return installed
}

func applyNpmOutdatedToAgents(ctx context.Context, items []*model.Item, catalog []agentDef) {
	out, err := execCombined(ctx, binNpm, "outdated", flagGlobal, "--json")
	if err != nil && len(out) == 0 {
		return
	}
	latestByPkg := ParseNpmOutdatedMap(out)
	if len(latestByPkg) == 0 {
		return
	}
	npmByName := make(map[string]string, len(catalog))
	for _, a := range catalog {
		if a.npmPackage != "" {
			npmByName[a.name] = a.npmPackage
		}
	}
	for _, it := range items {
		pkg := it.PackageID
		if pkg == "" {
			pkg = npmByName[it.Name]
		}
		if pkg == "" {
			continue
		}
		if latest, ok := latestByPkg[pkg]; ok {
			ApplyAgentOutdated(it, latest)
		}
	}
}

// resolveRegistryLatest flags agents whose npm package is NOT installed via
// global npm (native installer, brew, pnpm/bun store) by asking the registry
// for the latest version. Agents already handled by `npm outdated -g` or
// without an npmPackage are skipped.
func resolveRegistryLatest(ctx context.Context, items []*model.Item, catalog []agentDef) {
	installed := npmInstalledPackages(ctx)
	defByName := make(map[string]agentDef, len(catalog))
	for _, a := range catalog {
		defByName[a.name] = a
	}
	for _, it := range items {
		if it.Status == model.StatusOutdated {
			continue // already flagged by the npm-outdated merge
		}
		a, ok := defByName[it.Name]
		if !ok || a.npmPackage == "" || installed[a.npmPackage] {
			continue
		}
		latest := registryLatest(ctx, a)
		if latest != "" {
			ApplyAgentOutdated(it, latest)
		}
	}
}

// registryLatest returns the newest published version of the agent's npm
// package: an explicit latestCmd wins, otherwise `npm view <pkg> version`
// (which honours the user's .npmrc registry/proxy).
func registryLatest(ctx context.Context, a agentDef) string {
	if len(a.latestCmd) > 0 {
		out, err := execCommandBudget(ctx, agentProbeTimeout, a.latestCmd[0], a.latestCmd[1:]...)
		if err == nil {
			return parseAgentVersion(string(out))
		}
		return ""
	}
	out, err := execCommandBudget(ctx, agentProbeTimeout, binNpm, "view", a.npmPackage, "version")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// ApplyAgentOutdated marks an agent item outdated when latest differs from current.
// Pure helper for unit tests and npm-merge paths.
func ApplyAgentOutdated(it *model.Item, latest string) {
	if it == nil || latest == "" {
		return
	}
	cur := normalizeAgentVer(it.CurrentVer)
	lat := normalizeAgentVer(latest)
	if cur == "" || cur == statusInstalled || cur == verNoneInstalled {
		it.AvailableVer = lat
		it.Status = model.StatusOutdated
		return
	}
	if cur == lat {
		return
	}
	it.AvailableVer = lat
	it.Status = model.StatusOutdated
}

func normalizeAgentVer(v string) string {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "v")
	v = strings.TrimSuffix(v, ".")
	if m := semverRE.FindString(v); m != "" {
		return strings.TrimSuffix(m, versionDot)
	}
	return v
}

func probeAgentVersion(ctx context.Context, verCmd []string) string {
	out, err := execCommandBudget(ctx, agentProbeTimeout, verCmd[0], verCmd[1:]...)
	if err == nil {
		return parseAgentVersion(string(out))
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		if v := parseAgentVersion(string(exitErr.Stderr)); v != "" {
			return v
		}
	}
	return statusInstalled
}

// agentSkipVersionProbe avoids Electron/GUI CLIs that hang without a display (common over SSH).
func agentSkipVersionProbe(plat model.PlatformInfo, binary string) bool {
	if plat.OS != "linux" {
		return false
	}
	if os.Getenv("DISPLAY") != "" || os.Getenv("WAYLAND_DISPLAY") != "" {
		return false
	}
	switch binary {
	case binAntigravity, binCursor, binWindsurf:
		return true
	default:
		return false
	}
}

// AIInfraSource scans AI infrastructure tools.
type AIInfraSource struct{}

func (s *AIInfraSource) Category() model.Category { return model.CatAI }
func (s *AIInfraSource) Label() string            { return "AI Infra" }
func (s *AIInfraSource) Icon() string             { return "⚙️" }

type infraTool struct {
	name      string
	binary    string
	category  model.Category
	verCmd    []string
	latestCmd []string // optional freshness probe (see infraLatestMode)
	latest    infraLatestMode
}

// infraLatestMode describes how to interpret a latestCmd's output.
type infraLatestMode int

const (
	infraLatestNone     infraLatestMode = iota // informational only
	infraLatestNonEmpty                        // any output = update available
	infraLatestSemidx                          // "latest: vX" + "update is available"
	infraLatestGhExt                           // gh extension list update-marker column
)

func aiInfraCatalog() []infraTool {
	return []infraTool{
		{name: toolAIMemory, binary: toolAIMemory, category: model.CatAI, verCmd: []string{toolAIMemory, flagVersion}},
		{name: binSemidx, binary: binSemidx, category: model.CatAI, verCmd: []string{binSemidx, flagVersion},
			latestCmd: []string{binSemidx, cmdUpgrade, "--check"}, latest: infraLatestSemidx},
		{name: "Gh Extensions", binary: binGh, category: model.CatGHExt, verCmd: []string{binGh, "extension", cmdList},
			latestCmd: []string{binGh, "extension", cmdList}, latest: infraLatestGhExt},
		{name: binGcloud, binary: binGcloud, category: model.CatAI, verCmd: []string{binGcloud, "version", "--format=json"},
			latestCmd: []string{binGcloud, "components", cmdList, "--only-filter-updates-available", "--format=value(id)"}, latest: infraLatestNonEmpty},
	}
}

func (s *AIInfraSource) Scan(ctx context.Context, plat model.PlatformInfo) ([]*model.Item, error) {
	var items []*model.Item
	for _, t := range aiInfraCatalog() {
		if _, err := exec.LookPath(t.binary); err != nil {
			continue
		}
		items = append(items, probeInfraItem(ctx, t))
	}
	if len(items) == 0 {
		items = append(items, &model.Item{
			Name: "ai-infra", Category: model.CatAI, Status: model.StatusOK, CurrentVer: verNoneInstalled,
		})
	}
	return items, nil
}

func probeInfraItem(ctx context.Context, t infraTool) *model.Item {
	it := &model.Item{Name: t.name, Category: t.category, Status: model.StatusOK}
	if len(t.verCmd) > 0 {
		out, err := execCommandBudget(ctx, agentProbeTimeout, t.verCmd[0], t.verCmd[1:]...)
		if err == nil {
			it.CurrentVer = truncateVersionOutput(string(out))
		}
		// A failed version probe must not skip the freshness check below.
	}
	if len(t.latestCmd) > 0 {
		applyInfraLatest(ctx, it, t)
	}
	return it
}

// applyInfraLatest runs the freshness probe and flags the item outdated.
func applyInfraLatest(ctx context.Context, it *model.Item, t infraTool) {
	out, err := execCommandBudget(ctx, infraLatestTimeout, t.latestCmd[0], t.latestCmd[1:]...)
	if err != nil && len(out) == 0 {
		return // probe unavailable — stay informational
	}
	latest, hasUpdate := parseInfraLatest(t.latest, string(out))
	if !hasUpdate {
		return
	}
	it.Status = model.StatusOutdated
	if latest != "" {
		it.AvailableVer = latest
	}
}

// InfraUpdateCommand returns the upgrade command for an AI-infra tool
// (nil = no auto-update). Data-driven counterpart of aiInfraCatalog.
func InfraUpdateCommand(name string) []string {
	switch name {
	case toolAIMemory:
		return []string{toolAIMemory, cmdUpgrade}
	case binSemidx:
		return []string{binSemidx, cmdUpgrade}
	case binGcloud:
		return []string{binGcloud, "components", cmdUpdate, "--quiet"}
	default:
		return nil
	}
}

// parseInfraLatest interprets a latestCmd output per mode.
func parseInfraLatest(mode infraLatestMode, out string) (string, bool) {
	switch mode {
	case infraLatestNonEmpty:
		return "", strings.TrimSpace(out) != ""
	case infraLatestSemidx:
		if !strings.Contains(out, "update is available") {
			return "", false
		}
		for _, line := range strings.Split(out, scannerNL) {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "latest:") {
				return strings.TrimSpace(strings.TrimPrefix(line, "latest:")), true
			}
		}
		return "", true
	case infraLatestGhExt:
		// Newer gh adds an "Update available" column; older ones have none.
		for _, line := range strings.Split(out, scannerNL) {
			fields := strings.Fields(line)
			if len(fields) >= 4 && strings.Contains(strings.Join(fields[3:], " "), "Update") {
				return "", true
			}
		}
		return "", false
	default:
		return "", false
	}
}

func truncateVersionOutput(v string) string {
	v = strings.TrimSpace(v)
	if len(v) <= 60 {
		return v
	}
	firstLine := strings.SplitN(v, scannerNL, 2)[0]
	if len(firstLine) <= 60 {
		return firstLine
	}
	return v[:60] + "..."
}
