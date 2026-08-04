package scanner

import (
	"reflect"
	"testing"
)

func TestNpmGlobalInstallCmd(t *testing.T) {
	want := []string{
		"npm", "install", "-g",
		"--allow-scripts=@anthropic-ai/claude-code",
		"@anthropic-ai/claude-code@latest",
	}
	if got := npmGlobalInstallCmd("@anthropic-ai/claude-code"); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestAgentUpdateCommand_npmAgentsAllowScripts(t *testing.T) {
	// npm >= 12 blocks install scripts by default (RFC npm/rfcs#868); every
	// npm-driven agent update must carry an explicit --allow-scripts entry.
	for _, name := range []string{"Codex", "pi", "Qwen Code"} {
		a, ok := lookupAgentDef(name)
		if !ok || a.npmPackage == "" {
			t.Fatalf("%s: expected catalog entry with npmPackage", name)
		}
		cmd := AgentUpdateCommand(name)
		wantFlag := "--allow-scripts=" + a.npmPackage
		found := false
		for _, arg := range cmd {
			if arg == wantFlag {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("%s: update cmd %v missing %q", name, cmd, wantFlag)
		}
	}
}
