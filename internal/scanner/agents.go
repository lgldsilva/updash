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
	flagVersion  = "--version"
	toolAIMemory = "ai-memory"
	policyManual = "manual reinstall / app update"
)

var semverRE = regexp.MustCompile(`\d+\.\d+\.\d+[a-zA-Z0-9.-]*`)

// parseAgentVersion extracts the first semver-like version from CLI output.
// Handles multi-line, parenthetical comments, and varied formats.
func parseAgentVersion(output string) string {
	firstLine := strings.SplitN(strings.TrimSpace(output), "\n", 2)[0]
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
	last = strings.TrimSuffix(last, ".")
	last = strings.TrimSuffix(last, ",")
	return last
}

// agentUpdateMode describes how an agent can be upgraded.
type agentUpdateMode int

const (
	agentUpdateAuto agentUpdateMode = iota
	agentUpdateManual
)

type agentDef struct {
	name       string
	binary     string
	verCmd     []string
	mode       agentUpdateMode
	npmPackage string // optional: match against `npm outdated -g`
}

func agentCatalog() []agentDef {
	return []agentDef{
		{"Claude Code", "claude", []string{"claude", flagVersion}, agentUpdateAuto, "@anthropic-ai/claude-code"},
		{"OpenCode", "opencode", []string{"opencode", flagVersion}, agentUpdateAuto, ""},
		{"Grok", "grok", []string{"grok", flagVersion}, agentUpdateAuto, ""},
		{"Antigravity", "antigravity", []string{"antigravity", flagVersion}, agentUpdateManual, ""},
		{"Agy", "agy", []string{"agy", flagVersion}, agentUpdateManual, ""},
		{"MimoCode", "mimo", []string{"mimo", flagVersion}, agentUpdateManual, ""},
		{"Codex", "codex", []string{"codex", flagVersion}, agentUpdateAuto, "@openai/codex"},
		{"Gemini CLI", "gemini", []string{"gemini", flagVersion}, agentUpdateAuto, "@google/gemini-cli"},
		{"Copilot CLI", "copilot", []string{"copilot", flagVersion}, agentUpdateAuto, ""},
		{"Crush", "crush", []string{"crush", flagVersion}, agentUpdateManual, ""},
		{"Cursor", "cursor", []string{"cursor", flagVersion}, agentUpdateManual, ""},
	}
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
			{Name: "agents", Category: model.CatAgent, Status: model.StatusOK, CurrentVer: "none installed"},
		}, nil
	}
	if plat.HasNpm {
		applyNpmOutdatedToAgents(ctx, items, catalog)
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
		it.KeepPolicy = policyManual
	}
	if len(a.verCmd) == 0 {
		return it
	}
	if agentSkipVersionProbe(plat, a.binary) {
		it.CurrentVer = "installed"
		return it
	}
	it.CurrentVer = probeAgentVersion(ctx, a.verCmd)
	return it
}

func applyNpmOutdatedToAgents(ctx context.Context, items []*model.Item, catalog []agentDef) {
	out, err := execCombined(ctx, "npm", "outdated", "-g", "--json")
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

// ApplyAgentOutdated marks an agent item outdated when latest differs from current.
// Pure helper for unit tests and npm-merge paths.
func ApplyAgentOutdated(it *model.Item, latest string) {
	if it == nil || latest == "" {
		return
	}
	cur := normalizeAgentVer(it.CurrentVer)
	lat := normalizeAgentVer(latest)
	if cur == "" || cur == "installed" || cur == "none installed" {
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
		return strings.TrimSuffix(m, ".")
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
	return "installed"
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
	case "antigravity", "cursor":
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
	name     string
	binary   string
	category model.Category
	verCmd   []string
}

func aiInfraCatalog() []infraTool {
	return []infraTool{
		{toolAIMemory, toolAIMemory, model.CatAI, []string{toolAIMemory, "version"}},
		{"semidx", "semidx", model.CatAI, []string{"semidx", "version"}},
		{"Gh Extensions", "gh", model.CatGHExt, []string{"gh", "extension", "list"}},
		{"gcloud", "gcloud", model.CatAI, []string{"gcloud", "version", "--format=json"}},
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
			Name: "ai-infra", Category: model.CatAI, Status: model.StatusOK, CurrentVer: "none installed",
		})
	}
	return items, nil
}

func probeInfraItem(ctx context.Context, t infraTool) *model.Item {
	it := &model.Item{Name: t.name, Category: t.category, Status: model.StatusOK}
	if len(t.verCmd) == 0 {
		return it
	}
	out, err := execCommandBudget(ctx, agentProbeTimeout, t.verCmd[0], t.verCmd[1:]...)
	if err != nil {
		return it
	}
	it.CurrentVer = truncateVersionOutput(string(out))
	return it
}

func truncateVersionOutput(v string) string {
	v = strings.TrimSpace(v)
	if len(v) <= 60 {
		return v
	}
	firstLine := strings.SplitN(v, "\n", 2)[0]
	if len(firstLine) <= 60 {
		return firstLine
	}
	return v[:60] + "..."
}
