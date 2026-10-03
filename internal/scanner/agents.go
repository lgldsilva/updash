package scanner

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"

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
// An auto agent with updateCmd but no freshness channel (no latestCmd and no
// npmPackage) is still upgraded by running that command: it is an idempotent
// remote check, so --update runs it even when the scan cannot mark the item
// outdated. See AgentUpdatesWithoutFreshness.
type agentDef struct {
	name       string
	binary     string
	verCmd     []string
	mode       agentUpdateMode
	keepPolicy string   // manual-mode reason shown to the user ("" = policyManual)
	npmPackage string   // npm package name: drives registry latest lookup + npm-based update
	updateCmd  []string // explicit upgrade command (required for auto mode without npmPackage)
	latestCmd  []string // optional: command whose stdout holds the latest version
	// latestJSONKey switches latestCmd parsing from generic semver extraction
	// to reading a top-level JSON string field (e.g. grok update --check
	// --json → "latestVersion").
	latestJSONKey string
}

func agentCatalog() []agentDef {
	return []agentDef{
		{name: "Claude Code", binary: binClaude, verCmd: []string{binClaude, flagVersion}, mode: agentUpdateAuto, npmPackage: "@anthropic-ai/claude-code", updateCmd: []string{binClaude, cmdUpdate}},
		{name: "OpenCode", binary: binOpenCode, verCmd: []string{binOpenCode, flagVersion}, mode: agentUpdateAuto, npmPackage: "opencode-ai", updateCmd: []string{binOpenCode, cmdUpgrade}},
		// Grok ships as a native binary updated by its own subcommand; its
		// `update --check --json` probe is the freshness channel (no npm
		// package exists to ask the registry about).
		{name: "Grok", binary: binGrok, verCmd: []string{binGrok, flagVersion}, mode: agentUpdateAuto, updateCmd: []string{binGrok, cmdUpdate},
			latestCmd: []string{binGrok, cmdUpdate, "--check", "--json"}, latestJSONKey: "latestVersion"},
		{name: "Antigravity", binary: binAntigravity, verCmd: []string{binAntigravity, flagVersion}, mode: agentUpdateManual},
		// Agy is the standalone Antigravity CLI (~/.local/bin/agy), not the
		// AUR IDE package. `agy update` is non-interactive and idempotent
		// (already-current exits 0), but there is no check-only flag and
		// `agy changelog` prints the version embedded in the binary. With no
		// freshness channel, --update runs `agy update` even while the scan
		// stays "freshness not verified". Today Agy is the only such agent.
		{name: "Agy", binary: "agy", verCmd: []string{"agy", flagVersion}, mode: agentUpdateAuto, updateCmd: []string{"agy", "update"}},
		// MiMo Code (Xiaomi) distributes a native binary whose `mimo upgrade`
		// self-updater mirrors the curl-installer layout; @mimo-ai/cli is the
		// published package used only as the freshness source.
		{name: "MimoCode", binary: "mimo", verCmd: []string{"mimo", flagVersion}, mode: agentUpdateAuto, npmPackage: "@mimo-ai/cli", updateCmd: []string{"mimo", cmdUpgrade}},
		{name: "Codex", binary: "codex", verCmd: []string{"codex", flagVersion}, mode: agentUpdateAuto, npmPackage: "@openai/codex", updateCmd: npmGlobalInstallCmd("@openai/codex")},
		{name: "Gemini CLI", binary: binGemini, verCmd: []string{binGemini, flagVersion}, mode: agentUpdateAuto, npmPackage: "@google/gemini-cli", updateCmd: []string{binGemini, cmdUpdate}},
		// Copilot CLI rides the same npm channel as the generic @github/copilot
		// item (no duplicate updater): npm metadata flags staleness, the
		// npm-managed fallback installs the new version.
		{name: "Copilot CLI", binary: binCopilot, verCmd: []string{binCopilot, flagVersion}, mode: agentUpdateAuto, npmPackage: "@github/copilot"},
		// Crush is npm-published (@charmland/crush) but frequently installed
		// into a non-default prefix; the updater resolves the owning prefix at
		// plan time (agentPlans hook).
		{name: "Crush", binary: "crush", verCmd: []string{"crush", flagVersion}, mode: agentUpdateAuto, npmPackage: "@charmland/crush"},
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

// AgentUpdatesWithoutFreshness reports an auto agent whose updateCmd is an
// idempotent remote check and that has neither latestCmd nor an npm package.
// --update and TUI update-all run that command even when the scan could not
// mark the item outdated. The item stays informational ("freshness not
// verified"); a successful version probe is not an unverified source.
func AgentUpdatesWithoutFreshness(name string) bool {
	a, ok := lookupAgentDef(name)
	if !ok || a.mode != agentUpdateAuto {
		return false
	}
	return len(a.updateCmd) > 0 && len(a.latestCmd) == 0 && a.npmPackage == ""
}

// AgentUpdateCheckNote is the scan hint for an agent updated without a
// freshness channel ("" otherwise). The wording must not contain "manual":
// that substring marks an item manual-only and would skip the update.
func AgentUpdateCheckNote(name string) string {
	if !AgentUpdatesWithoutFreshness(name) {
		return ""
	}
	cmd := AgentUpdateCommand(name)
	if len(cmd) == 0 {
		return ""
	}
	return "checked on update: " + strings.Join(cmd, " ")
}

const (
	// AgyUpdateUpdated labels `agy update` stdout that installed a release.
	AgyUpdateUpdated = "updated"
	// AgyUpdateCurrent labels stdout that reports the installed build is current.
	AgyUpdateCurrent = "current"
)

// ClassifyAgyUpdateOutput labels `agy update` stdout. The process exit code
// still decides success: a zero exit stays a success even when the text
// matches neither phrase, so an idempotent update is never inconclusive.
func ClassifyAgyUpdateOutput(out string) string {
	switch {
	case strings.Contains(out, "Update successful"):
		return AgyUpdateUpdated
	case strings.Contains(out, "already on the latest version"):
		return AgyUpdateCurrent
	default:
		return ""
	}
}

func (s *AgentSource) Scan(ctx context.Context, plat model.PlatformInfo) ([]*model.Item, error) {
	catalog := agentCatalog()
	var installed []agentDef
	for _, a := range catalog {
		if _, err := exec.LookPath(a.binary); err == nil {
			installed = append(installed, a)
		}
	}
	if len(installed) == 0 {
		return []*model.Item{infoItem("agents", model.CatAgent, verNoneInstalled)}, nil
	}
	items := probeAgentsConcurrently(ctx, plat, installed)
	if plat.HasNpm {
		installedVersions := npmInstalledVersions(ctx)
		if err := applyNpmOutdatedToAgents(ctx, items, catalog, installedVersions); err != nil {
			// A failed or unreadable `npm outdated` is not "nothing is
			// outdated". Leave managed agents unverified instead of OK.
			markNpmManagedUnverified(items, installedVersions, err)
		} else {
			// npm omits up-to-date packages from `npm outdated`, so a managed
			// agent absent from a successful merge is affirmatively fresh.
			markNpmManagedFresh(items, installedVersions)
		}
		resolveRegistryLatestFrom(ctx, items, catalog, nameSet(installedVersions))
	}
	return items, nil
}

// probeAgentsConcurrently probes every installed agent's version in
// parallel. Each probe already has its own budget (agentProbeTimeout); a
// serial loop over ~10 CLIs — several of them Node-based with a slow cold
// start — sums well past the 90s per-source scan timeout on modest
// hardware. Probing is independent per tool, so there's nothing to
// serialize for.
func probeAgentsConcurrently(ctx context.Context, plat model.PlatformInfo, installed []agentDef) []*model.Item {
	items := make([]*model.Item, len(installed))
	var wg sync.WaitGroup
	for i, a := range installed {
		wg.Add(1)
		go func(i int, a agentDef) {
			defer wg.Done()
			items[i] = probeAgentItem(ctx, plat, a)
		}(i, a)
	}
	wg.Wait()
	return items
}

func probeAgentItem(ctx context.Context, plat model.PlatformInfo, a agentDef) *model.Item {
	it := &model.Item{
		Name:     a.name,
		Category: model.CatAgent,
		Status:   model.StatusInfo,
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
	var ok bool
	it.CurrentVer, ok = probeAgentVersionOK(ctx, a.verCmd)
	if !ok {
		it.Status = model.StatusUnverified
		return it
	}
	// Version answered: freshness may still be unknown, but the item is not
	// an unverified source. Record how --update will check it.
	if note := AgentUpdateCheckNote(a.name); note != "" {
		it.KeepPolicy = note
	}
	return it
}

// npmInstalledVersions maps the globally npm-installed package names to their
// installed versions (depth 0). stdout only: npm ls --json output must not be
// corrupted by stderr warnings (see the execCombined doc in runner.go).
func npmInstalledVersions(ctx context.Context) map[string]string {
	out, err := execCommand(ctx, binNpm, "ls", flagGlobal, "--json", "--depth=0")
	versions := ParseNpmLsGlobalVersions(out)
	if err != nil && len(versions) == 0 {
		return nil
	}
	return versions
}

func nameSet(versions map[string]string) map[string]bool {
	set := make(map[string]bool, len(versions))
	for name := range versions {
		set[name] = true
	}
	return set
}

// markNpmManagedFresh affirms agents whose npm package is installed and was
// not flagged by a successful npm-outdated merge: their installed version
// equals the latest npm knows about.
func markNpmManagedFresh(items []*model.Item, installedVersions map[string]string) {
	for _, it := range items {
		if it == nil || it.Status != model.StatusInfo || it.PackageID == "" {
			continue
		}
		if _, managed := installedVersions[it.PackageID]; managed {
			it.Status = model.StatusOK
		}
	}
}

// markNpmManagedUnverified records that the npm-outdated probe could not be
// trusted. Only StatusInfo rows are touched: a version probe that already
// failed stays unverified with its own cause.
func markNpmManagedUnverified(items []*model.Item, installedVersions map[string]string, cause error) {
	msg := errCause(cause)
	if msg == "" {
		msg = "npm outdated probe failed"
	}
	for _, it := range items {
		if it == nil || it.Status != model.StatusInfo || it.PackageID == "" {
			continue
		}
		if _, managed := installedVersions[it.PackageID]; !managed {
			continue
		}
		it.Status = model.StatusUnverified
		it.Error = msg
	}
}

// applyNpmOutdatedToAgents merges `npm outdated -g` (batched by explicit name)
// into the agent items. The batch is restricted to packages npm actually
// manages: `npm outdated <name>` stalls on names that are not installed.
// A nil error means the payload was valid JSON (including an empty object):
// absence from that map is "not outdated". A non-nil error means the probe
// failed or the payload could not be parsed.
func applyNpmOutdatedToAgents(ctx context.Context, items []*model.Item, catalog []agentDef, installedVersions map[string]string) error {
	pkgs := managedAgentPackages(items, catalog, installedVersions)
	if len(pkgs) == 0 {
		return nil
	}
	latestByPkg, err := npmOutdatedLatestFor(ctx, pkgs)
	if err != nil {
		return err
	}
	applyLatestToAgents(items, catalog, latestByPkg)
	return nil
}

func managedAgentPackages(items []*model.Item, catalog []agentDef, installedVersions map[string]string) []string {
	npmByName := agentNpmNames(catalog)
	pkgs := make([]string, 0, len(items))
	seen := make(map[string]bool, len(items))
	for _, it := range items {
		pkg := agentItemPackage(it, npmByName)
		if pkg == "" || seen[pkg] {
			continue
		}
		if _, managed := installedVersions[pkg]; !managed {
			continue
		}
		seen[pkg] = true
		pkgs = append(pkgs, pkg)
	}
	return pkgs
}

func agentNpmNames(catalog []agentDef) map[string]string {
	npmByName := make(map[string]string, len(catalog))
	for _, a := range catalog {
		if a.npmPackage != "" {
			npmByName[a.name] = a.npmPackage
		}
	}
	return npmByName
}

func agentItemPackage(it *model.Item, npmByName map[string]string) string {
	if it == nil {
		return ""
	}
	if it.PackageID != "" {
		return it.PackageID
	}
	return npmByName[it.Name]
}

// npmOutdatedLatestFor runs one explicit-name outdated probe. npm exits 1
// when something is outdated and still prints JSON; that is success. An empty
// body with a command error, or a body that is not JSON, is a failed probe.
func npmOutdatedLatestFor(ctx context.Context, pkgs []string) (map[string]string, error) {
	sort.Strings(pkgs)
	args := append([]string{"outdated", flagGlobal, "--json"}, pkgs...)
	out, err := execCommand(ctx, binNpm, args...)
	if err != nil && strings.TrimSpace(string(out)) == "" {
		return nil, err
	}
	return parseNpmOutdatedLatest(out)
}

func applyLatestToAgents(items []*model.Item, catalog []agentDef, latestByPkg map[string]string) {
	npmByName := agentNpmNames(catalog)
	for _, it := range items {
		pkg := agentItemPackage(it, npmByName)
		if latest, ok := latestByPkg[pkg]; ok {
			ApplyAgentOutdated(it, latest)
		}
	}
}

// resolveRegistryLatest flags agents whose npm package is NOT installed via
// global npm (native installer, brew, pnpm/bun store) by asking the registry
// for the latest version. Agents already handled by `npm outdated -g` or
// without a freshness channel are skipped.
func resolveRegistryLatest(ctx context.Context, items []*model.Item, catalog []agentDef) {
	resolveRegistryLatestFrom(ctx, items, catalog, nameSet(npmInstalledVersions(ctx)))
}

// resolveRegistryLatestFrom is resolveRegistryLatest with a pre-fetched
// global-npm package set (allows callers to overlap it with other npm work).
// Registry probes fan out bounded-parallel: they are network-bound with a
// per-probe budget, and a serial loop over many non-npm agents sums several
// seconds of avoidable latency.
func resolveRegistryLatestFrom(ctx context.Context, items []*model.Item, catalog []agentDef, installed map[string]bool) {
	defByName := make(map[string]agentDef, len(catalog))
	for _, a := range catalog {
		defByName[a.name] = a
	}
	type probeTarget struct {
		it *model.Item
		a  agentDef
	}
	targets := make([]probeTarget, 0, len(items))
	for _, it := range items {
		if it.Status == model.StatusOutdated || it.Status == model.StatusOK || it.Status == model.StatusUnverified {
			continue // npm merge already decided, or the version probe failed
		}
		a, ok := defByName[it.Name]
		if !ok {
			continue
		}
		// latestCmd is a self-contained freshness probe (e.g. `grok update
		// --check --json`): it needs no npm package and ignores npm state.
		if len(a.latestCmd) > 0 {
			targets = append(targets, probeTarget{it: it, a: a})
			continue
		}
		if a.npmPackage == "" || installed[a.npmPackage] {
			continue
		}
		targets = append(targets, probeTarget{it: it, a: a})
	}
	probeBounded(len(targets), registryProbeConcurrency, func(i int) {
		t := targets[i]
		if latest := registryLatest(ctx, t.a); latest != "" {
			ApplyAgentOutdated(t.it, latest)
		} else if t.it.Status != model.StatusUnverified {
			// The probe could not confirm a version: do not leave the item
			// claiming an affirmative status it never verified.
			t.it.Status = model.StatusUnverified
		}
	})
}

// registryLatest returns the newest published version of the agent: an
// explicit latestCmd wins, otherwise `npm view <pkg> version` (which honours
// the user's .npmrc registry/proxy).
func registryLatest(ctx context.Context, a agentDef) string {
	if len(a.latestCmd) > 0 {
		out, err := execCommandBudget(ctx, registryLatestTimeout, a.latestCmd[0], a.latestCmd[1:]...)
		if err != nil {
			return ""
		}
		return parseAgentLatest(a, string(out))
	}
	out, err := execCommandBudget(ctx, registryLatestTimeout, binNpm, "view", a.npmPackage, "version")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// parseAgentLatest interprets a latestCmd payload: a JSON probe
// (latestJSONKey set) reads the key's string value, the generic semver
// extraction handles everything else.
func parseAgentLatest(a agentDef, out string) string {
	if a.latestJSONKey == "" {
		return parseAgentVersion(out)
	}
	return jsonStringValue(out, a.latestJSONKey)
}

// jsonStringValue extracts a top-level string field from a JSON object
// ("" when the payload is not JSON or the key is absent).
func jsonStringValue(out, key string) string {
	var data map[string]json.RawMessage
	if err := json.Unmarshal([]byte(out), &data); err != nil {
		return ""
	}
	var value string
	if err := json.Unmarshal(data[key], &value); err != nil {
		return ""
	}
	return value
}

// ApplyAgentOutdated marks an agent item outdated when latest differs from current.
// Pure helper for unit tests and npm-merge paths.
func ApplyAgentOutdated(it *model.Item, latest string) {
	if it == nil || latest == "" || it.Status == model.StatusUnverified {
		// Unverified means the current version is unknown (often the
		// "installed" placeholder). A registry or npm latest must not turn
		// that into a false outdated row.
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
		// The freshness channel confirmed the installed version is current:
		// upgrade the probe-only info state to an affirmative OK.
		if it.Status == model.StatusInfo {
			it.Status = model.StatusOK
		}
		return
	}
	if compareAgentVersions(cur, lat) >= 0 {
		return
	}
	it.AvailableVer = lat
	it.Status = model.StatusOutdated
}

// compareAgentVersions compares numeric semver cores. Unknown formats return
// -1 so that a newly discovered version remains visible instead of hidden.
func compareAgentVersions(current, latest string) int {
	parse := func(v string) ([]int, bool) {
		core := strings.SplitN(v, "-", 2)[0]
		parts := strings.Split(core, ".")
		if len(parts) != 3 {
			return nil, false
		}
		out := make([]int, 3)
		for i, p := range parts {
			n, err := strconv.Atoi(p)
			if err != nil {
				return nil, false
			}
			out[i] = n
		}
		return out, true
	}
	a, oka := parse(current)
	b, okb := parse(latest)
	if !oka || !okb {
		return -1
	}
	for i := range a {
		if a[i] < b[i] {
			return -1
		}
		if a[i] > b[i] {
			return 1
		}
	}
	return 0
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

func probeAgentVersionOK(ctx context.Context, verCmd []string) (string, bool) {
	out, err := execCommandBudget(ctx, agentProbeTimeout, verCmd[0], verCmd[1:]...)
	if err == nil {
		return parseAgentVersion(string(out)), true
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		if v := parseAgentVersion(string(exitErr.Stderr)); v != "" {
			return v, true
		}
	}
	return statusInstalled, false
}

// agentSkipVersionProbe avoids Electron/GUI CLIs whose "--version" launches
// the full app instead of printing a version and exiting.
//
// Antigravity has no working version flag at all: `antigravity --version`
// starts the Electron shell, language server, and host bridge regardless of
// whether a display is attached, and never returns on its own — every probe
// burns the full per-agent timeout. Cursor and Windsurf, by contrast, only
// exhibit that behavior headless (common over SSH, e.g. no DISPLAY); with a
// display attached they answer --version normally, so they stay gated on that.
func agentSkipVersionProbe(plat model.PlatformInfo, binary string) bool {
	if binary == binAntigravity {
		return true
	}
	if plat.OS != "linux" {
		return false
	}
	if os.Getenv("DISPLAY") != "" || os.Getenv("WAYLAND_DISPLAY") != "" {
		return false
	}
	switch binary {
	case binCursor, binWindsurf:
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
			// --only-filter-updates-available was removed from modern gcloud;
			// the generic --filter is the supported equivalent (empty output
			// = nothing to update).
			latestCmd: []string{binGcloud, "components", cmdList, `--filter=status="Update Available"`, "--format=value(id)"}, latest: infraLatestNonEmpty},
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
			Name: "ai-infra", Category: model.CatAI, Status: model.StatusInfo, CurrentVer: verNoneInstalled,
		})
	}
	return items, nil
}

func probeInfraItem(ctx context.Context, t infraTool) *model.Item {
	it := &model.Item{Name: t.name, Category: t.category, Status: model.StatusInfo}
	if len(t.verCmd) > 0 {
		out, err := execCommandBudget(ctx, agentProbeTimeout, t.verCmd[0], t.verCmd[1:]...)
		if err == nil {
			it.CurrentVer = truncateVersionOutput(string(out))
		} else {
			it.Status = model.StatusUnverified
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
		it.Status = model.StatusUnverified
		return
	}
	latest, hasUpdate := parseInfraLatest(t.latest, string(out))
	if !hasUpdate {
		if it.Status != model.StatusUnverified {
			it.Status = model.StatusOK
		}
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
