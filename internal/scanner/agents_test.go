package scanner

import (
	"testing"

	"github.com/lgldsilva/updash/internal/model"
)

func TestApplyAgentOutdated(t *testing.T) {
	it := &model.Item{Name: "Codex", CurrentVer: "0.1.0", Status: model.StatusOK}
	ApplyAgentOutdated(it, "0.2.0")
	if it.Status != model.StatusOutdated || it.AvailableVer != "0.2.0" {
		t.Fatalf("got status=%v avail=%q", it.Status, it.AvailableVer)
	}

	same := &model.Item{Name: "x", CurrentVer: "1.0.0", Status: model.StatusOK}
	ApplyAgentOutdated(same, "1.0.0")
	if same.Status != model.StatusOK {
		t.Fatal("same version should stay OK")
	}

	inst := &model.Item{Name: "y", CurrentVer: "installed", Status: model.StatusOK}
	ApplyAgentOutdated(inst, "2.0.0")
	if inst.Status != model.StatusOutdated {
		t.Fatal("installed + latest should mark outdated")
	}

	ApplyAgentOutdated(nil, "1")
	ApplyAgentOutdated(it, "")
}

func TestNormalizeAgentVer(t *testing.T) {
	if got := normalizeAgentVer("v1.2.3."); got != "1.2.3" {
		t.Fatalf("got %q", got)
	}
	if got := normalizeAgentVer("  0.1.0  "); got != "0.1.0" {
		t.Fatalf("got %q", got)
	}
}

func TestParseAgentVersion(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "claude",
			input: "2.1.209 (Claude Code)",
			want:  "2.1.209",
		},
		{
			name:  "opencode",
			input: "1.17.20",
			want:  "1.17.20",
		},
		{
			name:  "grok",
			input: "grok 0.2.101 (5bc4b5dfadcf)",
			want:  "0.2.101",
		},
		{
			name:  "copilot",
			input: "GitHub Copilot CLI 1.0.70.\nRun 'copilot update' to check for updates.",
			want:  "1.0.70.",
		},
		{
			name:  "cursor_multi_line",
			input: "3.11.13\n3f21b08f0b436a07be29fbfe00b304fa15553350\narm64",
			want:  "3.11.13",
		},
		{
			name:  "gemini",
			input: "0.46.0",
			want:  "0.46.0",
		},
		{
			name:  "crush",
			input: "crush version v0.84.1",
			want:  "0.84.1",
		},
		{
			name:  "mimo",
			input: "0.1.2",
			want:  "0.1.2",
		},
		{
			name:  "agy",
			input: "1.1.2",
			want:  "1.1.2",
		},
		{
			name:  "codex",
			input: "0.144.4",
			want:  "0.144.4",
		},
		{
			name:  "empty",
			input: "",
			want:  "",
		},
		{
			name:  "just_text",
			input: "not a version number here",
			want:  "here", // fallback: last field
		},
		{
			name:  "parenthetical_suffix",
			input: "some-tool (dev-build)",
			want:  "some-tool", // fallback: field before parenthetical
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseAgentVersion(tt.input)
			if got != tt.want {
				t.Errorf("parseAgentVersion(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
