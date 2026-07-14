package scanner

import (
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
