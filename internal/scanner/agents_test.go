package scanner

import (
	"testing"
)

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
