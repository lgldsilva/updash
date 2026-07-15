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

type agentDef struct {
	name   string
	binary string
	verCmd []string
}

func agentCatalog() []agentDef {
	return []agentDef{
		{"Claude Code", "claude", []string{"claude", flagVersion}},
		{"OpenCode", "opencode", []string{"opencode", flagVersion}},
		{"Grok", "grok", []string{"grok", flagVersion}},
		{"Antigravity", "antigravity", []string{"antigravity", flagVersion}},
		{"Agy", "agy", []string{"agy", flagVersion}},
		{"MimoCode", "mimo", []string{"mimo", flagVersion}},
		{"Codex", "codex", []string{"codex", flagVersion}},
		{"Gemini CLI", "gemini", []string{"gemini", flagVersion}},
		{"Copilot CLI", "copilot", []string{"copilot", flagVersion}},
		{"Crush", "crush", []string{"crush", flagVersion}},
		{"Cursor", "cursor", []string{"cursor", flagVersion}},
	}
}

func (s *AgentSource) Scan(ctx context.Context, plat model.PlatformInfo) ([]*model.Item, error) {
	var items []*model.Item
	for _, a := range agentCatalog() {
		if _, err := exec.LookPath(a.binary); err != nil {
			continue
		}
		items = append(items, probeAgentItem(ctx, plat, a))
	}
	if len(items) == 0 {
		items = append(items, &model.Item{
			Name: "agents", Category: model.CatAgent, Status: model.StatusOK, CurrentVer: "none installed",
		})
	}
	return items, nil
}

func probeAgentItem(ctx context.Context, plat model.PlatformInfo, a agentDef) *model.Item {
	it := &model.Item{Name: a.name, Category: model.CatAgent, Status: model.StatusOK}
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
