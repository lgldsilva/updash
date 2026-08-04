package updater

import (
	"testing"

	"github.com/lgldsilva/updash/internal/model"
)

func TestNpmAllowScriptsFlag(t *testing.T) {
	items := []*model.Item{
		{Name: "@anthropic-ai/claude-code"},
		{Name: "opencode-ai"},
		{Name: "@anthropic-ai/claude-code"}, // duplicate
		{Name: ""},                          // empty
		nil,                                 // defensive
	}
	want := "--allow-scripts=@anthropic-ai/claude-code,opencode-ai"
	if got := npmAllowScriptsFlag(items); got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	if got := npmAllowScriptsFlag(nil); got != "" {
		t.Fatalf("empty batch must yield no flag, got %q", got)
	}
	if got := npmAllowScriptsFlag([]*model.Item{{}}); got != "" {
		t.Fatalf("blank-only batch must yield no flag, got %q", got)
	}
}

func TestClaudeStubMarker(t *testing.T) {
	stubOutput := `Error: claude native binary not installed.

Either postinstall did not run (--ignore-scripts, some pnpm configs)`
	if !containsStubMarker(stubOutput) {
		t.Fatal("stub output must be recognized")
	}
	if containsStubMarker("2.1.220 (Claude Code)") {
		t.Fatal("healthy version output must not match the stub marker")
	}
}
