package scanner

import (
	"context"
	"testing"

	"github.com/lgldsilva/updash/internal/model"
)

func TestParseNpmLsGlobal(t *testing.T) {
	out := []byte(`{
  "dependencies": {
    "@anthropic-ai/claude-code": {"version": "1.0.34"},
    "@earendil-works/pi-coding-agent": {"version": "0.9.1"}
  }
}`)
	m := ParseNpmLsGlobal(out)
	if !m["@anthropic-ai/claude-code"] || !m["@earendil-works/pi-coding-agent"] {
		t.Fatalf("missing entries: %v", m)
	}
	if m["not-installed"] {
		t.Fatal("unexpected entry")
	}
	if got := ParseNpmLsGlobal([]byte("not json")); got != nil {
		t.Fatalf("want nil on bad json, got %v", got)
	}
	if got := ParseNpmLsGlobal([]byte(`{}`)); got != nil {
		t.Fatalf("want nil on empty deps, got %v", got)
	}
}

func TestAgentCatalogConsistency(t *testing.T) {
	for _, a := range agentCatalog() {
		if a.name == "" || a.binary == "" {
			t.Errorf("catalog entry missing name/binary: %+v", a)
		}
		if a.mode == agentUpdateAuto && len(a.updateCmd) == 0 && a.npmPackage == "" {
			t.Errorf("auto agent %q needs updateCmd or npmPackage", a.name)
		}
		if AgentUpdateCommand(a.name) == nil && a.mode == agentUpdateAuto {
			t.Errorf("AgentUpdateCommand(%q) = nil for auto agent", a.name)
		}
		if a.mode == agentUpdateManual && AgentKeepPolicy(a.name) == "" {
			t.Errorf("manual agent %q must expose a keep policy", a.name)
		}
	}
}

func TestAgentUpdateCommand(t *testing.T) {
	cmd := AgentUpdateCommand("pi")
	if len(cmd) == 0 || cmd[0] != "npm" {
		t.Fatalf("pi update cmd = %v", cmd)
	}
	cmd = AgentUpdateCommand("Claude Code")
	if len(cmd) == 0 || cmd[0] != "claude" {
		t.Fatalf("claude update cmd = %v", cmd)
	}
	if cmd := AgentUpdateCommand("Cursor"); cmd != nil {
		t.Fatalf("manual agent must have nil update cmd, got %v", cmd)
	}
	if cmd := AgentUpdateCommand("no-such-agent"); cmd != nil {
		t.Fatalf("unknown agent must have nil update cmd, got %v", cmd)
	}
}

// OpenCode owns an npmPackage (opencode-ai) so the existing outdated-detection
// machinery flags it, but its explicit updateCmd must still win so the upgrade
// runs `opencode upgrade` (single owner) — never a generic npm install.
func TestAgentUpdateCommand_OpenCode(t *testing.T) {
	cmd := AgentUpdateCommand("OpenCode")
	if len(cmd) != 2 || cmd[0] != "opencode" || cmd[1] != "upgrade" {
		t.Fatalf("OpenCode update cmd = %v, want [opencode upgrade]", cmd)
	}
}

// When opencode-ai shows up in `npm outdated -g`, the OpenCode agent item must
// be flagged outdated — that is what makes `opencode upgrade` actually fire
// under --update (single owner), instead of the npm path touching it.
func TestOpenCodeFlaggedOutdatedViaNpm(t *testing.T) {
	enableMocks()
	defer disableMocks()

	setMock("npm", []string{"outdated", "-g", "--json"},
		`{"opencode-ai":{"current":"1.18.0","wanted":"1.18.16","latest":"1.18.16"}}`, nil)

	items := []*model.Item{{
		Name: "OpenCode", Category: model.CatAgent,
		Status: model.StatusOK, PackageID: "opencode-ai", CurrentVer: "1.18.0",
	}}
	applyNpmOutdatedToAgents(context.Background(), items, agentCatalog())

	if items[0].Status != model.StatusOutdated || items[0].AvailableVer != "1.18.16" {
		t.Fatalf("OpenCode not flagged outdated via opencode-ai: %+v", items[0])
	}
}

func TestAgentKeepPolicy(t *testing.T) {
	if AgentKeepPolicy("Cursor") == "" {
		t.Fatal("Cursor must have a keep policy")
	}
	if got := AgentKeepPolicy("Aider"); got == "" || got == policyManual {
		t.Fatalf("Aider should carry its pipx note, got %q", got)
	}
	if AgentKeepPolicy("pi") != "" {
		t.Fatal("auto agent must not expose a keep policy")
	}
}

func TestParseInfraLatest(t *testing.T) {
	if _, ok := parseInfraLatest(infraLatestNonEmpty, "component-a\n"); !ok {
		t.Error("non-empty mode: want update")
	}
	if _, ok := parseInfraLatest(infraLatestNonEmpty, "  \n"); ok {
		t.Error("non-empty mode: blank output must mean up to date")
	}

	semidxOut := "current: 0.46.0\nlatest:  v0.48.1\nan update is available — run `semidx upgrade`.\n"
	latest, ok := parseInfraLatest(infraLatestSemidx, semidxOut)
	if !ok || latest != "v0.48.1" {
		t.Fatalf("semidx mode: latest=%q ok=%v", latest, ok)
	}
	if _, ok := parseInfraLatest(infraLatestSemidx, "current: 0.46.0\nlatest: v0.46.0\nalready up to date\n"); ok {
		t.Error("semidx mode: up-to-date output must not flag")
	}

	ghOut := "gh copilot\tgithub/gh-copilot\tv1.2.0\tUpdate available\n"
	if _, ok := parseInfraLatest(infraLatestGhExt, ghOut); !ok {
		t.Error("gh mode: update marker column must flag")
	}
	if _, ok := parseInfraLatest(infraLatestGhExt, "gh copilot\tgithub/gh-copilot\tv1.2.0\n"); ok {
		t.Error("gh mode: no marker column must not flag")
	}
}

func TestInfraUpdateCommand(t *testing.T) {
	if cmd := InfraUpdateCommand("semidx"); len(cmd) == 0 || cmd[1] != "upgrade" {
		t.Fatalf("semidx cmd = %v", cmd)
	}
	if cmd := InfraUpdateCommand("gcloud"); len(cmd) == 0 || cmd[1] != "components" {
		t.Fatalf("gcloud cmd = %v", cmd)
	}
	if cmd := InfraUpdateCommand("unknown"); cmd != nil {
		t.Fatalf("unknown infra tool must be nil, got %v", cmd)
	}
}

func TestApplyAgentOutdatedFromRegistry(t *testing.T) {
	// Native-installed agent: probe returned a version, registry is newer.
	it := &model.Item{Name: "pi", Category: model.CatAgent, Status: model.StatusOK, CurrentVer: "0.9.1"}
	ApplyAgentOutdated(it, "0.9.4")
	if it.Status != model.StatusOutdated || it.AvailableVer != "0.9.4" {
		t.Fatalf("item not flagged: %+v", it)
	}
	// Same version → stays OK.
	it2 := &model.Item{Name: "pi", Category: model.CatAgent, Status: model.StatusOK, CurrentVer: "0.9.4"}
	ApplyAgentOutdated(it2, "0.9.4")
	if it2.Status != model.StatusOK {
		t.Fatalf("same version must stay ok: %+v", it2)
	}
}
