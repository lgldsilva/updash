package scanner

import (
	"context"
	"os/exec"
	"testing"

	"github.com/lgldsilva/updash/internal/model"
)

func TestAgentSkipVersionProbe(t *testing.T) {
	plat := model.PlatformInfo{OS: "linux"}
	t.Setenv("DISPLAY", "")
	t.Setenv("WAYLAND_DISPLAY", "")

	if !agentSkipVersionProbe(plat, "cursor") {
		t.Fatal("cursor should skip without display")
	}
	if agentSkipVersionProbe(plat, "claude") {
		t.Fatal("claude should not skip")
	}

	t.Setenv("DISPLAY", ":0")
	if agentSkipVersionProbe(plat, "cursor") {
		t.Fatal("cursor should probe when DISPLAY is set")
	}
}

// Gh Extensions must use CatGHExt so updater hits `gh extension upgrade --all`,
// not updateAIInfra's default "no auto-update".
func TestAIInfraSourceGhExtensionsCategory(t *testing.T) {
	if _, err := exec.LookPath("gh"); err != nil {
		t.Skip("gh not installed")
	}
	src := &AIInfraSource{}
	items, err := src.Scan(context.Background(), model.PlatformInfo{})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	found := false
	for _, it := range items {
		if it.Name != "Gh Extensions" {
			continue
		}
		found = true
		if it.Category != model.CatGHExt {
			t.Errorf("Gh Extensions category = %q, want %q", it.Category, model.CatGHExt)
		}
	}
	if !found {
		t.Fatal("expected Gh Extensions item when gh is on PATH")
	}
}
