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

var semverRE = regexp.MustCompile(`\d+\.\d+\.\d+[a-zA-Z0-9.-]*`)

// parseAgentVersion extracts the first semver-like version from CLI output.
// Handles multi-line, parenthetical comments, and varied formats.
func parseAgentVersion(output string) string {
	// Take first line only to avoid multi-line issues
	firstLine := strings.SplitN(strings.TrimSpace(output), "\n", 2)[0]

	// Try to find a semver pattern anywhere in the line
	m := semverRE.FindString(firstLine)
	if m != "" {
		return m
	}

	// Fallback: use the last whitespace-delimited token that's not obviously wrong
	parts := strings.Fields(firstLine)
	if len(parts) > 0 {
		last := parts[len(parts)-1]
		// Skip parenthetical tokens
		if strings.HasPrefix(last, "(") || strings.HasPrefix(last, "[") {
			if len(parts) >= 2 {
				return parts[len(parts)-2]
			}
		}
		last = strings.TrimSuffix(last, ".")
		last = strings.TrimSuffix(last, ",")
		return last
	}

	return firstLine
}

func (s *AgentSource) Scan(ctx context.Context, plat model.PlatformInfo) ([]*model.Item, error) {
	var items []*model.Item

	// Check each agent tool
	agents := []struct {
		name   string
		binary string
		verCmd []string // command to get version
	}{
		{"Claude Code", "claude", []string{"claude", "--version"}},
		{"OpenCode", "opencode", []string{"opencode", "--version"}},
		{"Grok", "grok", []string{"grok", "--version"}},
		{"Antigravity", "antigravity", []string{"antigravity", "--version"}},
		{"Agy", "agy", []string{"agy", "--version"}},
		{"MimoCode", "mimo", []string{"mimo", "--version"}},
		{"Codex", "codex", []string{"codex", "--version"}},
		{"Gemini CLI", "gemini", []string{"gemini", "--version"}},
		{"Copilot CLI", "copilot", []string{"copilot", "--version"}},
		{"Crush", "crush", []string{"crush", "--version"}},
		{"Cursor", "cursor", []string{"cursor", "--version"}},
	}

	for _, a := range agents {
		// Check if binary exists
		if _, err := exec.LookPath(a.binary); err != nil {
			continue // skip, not installed
		}

		it := &model.Item{
			Name:     a.name,
			Category: model.CatAgent,
			Status:   model.StatusOK,
		}

		if len(a.verCmd) > 0 {
			if agentSkipVersionProbe(plat, a.binary) {
				it.CurrentVer = "installed"
			} else {
				out, err := execCommandBudget(ctx, agentProbeTimeout, a.verCmd[0], a.verCmd[1:]...)
				if err == nil {
					it.CurrentVer = parseAgentVersion(string(out))
				} else {
					// Some tools output version to stderr
					if exitErr, ok := err.(*exec.ExitError); ok {
						it.CurrentVer = parseAgentVersion(string(exitErr.Stderr))
					}
					if it.CurrentVer == "" {
						it.CurrentVer = "installed"
					}
				}
			}
		}

		items = append(items, it)
	}

	if len(items) == 0 {
		items = append(items, &model.Item{
			Name: "agents", Category: model.CatAgent, Status: model.StatusOK, CurrentVer: "none installed",
		})
	}

	return items, nil
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

func (s *AIInfraSource) Scan(ctx context.Context, plat model.PlatformInfo) ([]*model.Item, error) {
	var items []*model.Item

	tools := []struct {
		name   string
		binary string
		verCmd []string
	}{
		{"ai-memory", "ai-memory", []string{"ai-memory", "version"}},
		{"semidx", "semidx", []string{"semidx", "version"}},
		{"Gh Extensions", "gh", []string{"gh", "extension", "list"}},
		{"gcloud", "gcloud", []string{"gcloud", "version", "--format=json"}},
	}

	for _, t := range tools {
		_, err := exec.LookPath(t.binary)
		if err != nil {
			continue
		}

		it := &model.Item{
			Name:     t.name,
			Category: model.CatAI,
			Status:   model.StatusOK,
		}

		if len(t.verCmd) > 0 {
			out, err := execCommandBudget(ctx, agentProbeTimeout, t.verCmd[0], t.verCmd[1:]...)
			if err == nil {
				v := strings.TrimSpace(string(out))
				if len(v) > 60 {
					// Try to extract just the version line
					firstLine := strings.SplitN(v, "\n", 2)[0]
					if len(firstLine) <= 60 {
						v = firstLine
					} else {
						v = v[:60] + "..."
					}
				}
				it.CurrentVer = v
			}
		}

		items = append(items, it)
	}

	if len(items) == 0 {
		items = append(items, &model.Item{
			Name: "ai-infra", Category: model.CatAI, Status: model.StatusOK, CurrentVer: "none installed",
		})
	}

	return items, nil
}
