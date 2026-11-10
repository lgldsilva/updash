package scanner

import (
	"fmt"
	"testing"

	"github.com/lgldsilva/updash/internal/model"
)

// TestEnabledSourcesNoDuplicates guards against duplicate source registration
// (regression test for the double PacmanSource entry).
func TestEnabledSourcesNoDuplicates(t *testing.T) {
	plat := model.PlatformInfo{
		HasPacman: true,
		HasBrew:   true,
		HasNpm:    true,
	}
	srcs := enabledSources(plat, false)
	seen := make(map[string]int)
	for _, s := range srcs {
		seen[fmt.Sprintf("%T", s)]++
	}
	for typ, n := range seen {
		if n > 1 {
			t.Fatalf("duplicate source registered %dx: %s", n, typ)
		}
	}
}
