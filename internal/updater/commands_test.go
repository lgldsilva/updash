package updater

import (
	"strings"
	"testing"

	"github.com/lgldsilva/updash/internal/model"
)

func TestShellUpdateCommand_brew(t *testing.T) {
	cmd := ShellUpdateCommand(&model.Item{Name: "microsoft-office", Category: model.CatBrew})
	if !strings.Contains(cmd, "brew upgrade --greedy") || !strings.Contains(cmd, "microsoft-office") {
		t.Fatalf("unexpected: %q", cmd)
	}
}

func TestBuildElevatedShellScript_markers(t *testing.T) {
	script := BuildElevatedShellScript([]*model.Item{
		{Name: "microsoft-office", Category: model.CatBrew},
	}, "/usr/bin:/bin")
	if !strings.Contains(script, "UPDASH_OK") || !strings.Contains(script, "microsoft-office") {
		t.Fatalf("missing ok marker: %q", script)
	}
}
