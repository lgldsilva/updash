package scanner

import (
	"context"
	"testing"

	"github.com/lgldsilva/updash/internal/model"
)

func TestProtectedNpmPackages(t *testing.T) {
	protected := ProtectedNpmPackages()
	for _, name := range []string{"opencode-ai", "@opencode-ai/cli"} {
		if _, ok := protected[name]; !ok {
			t.Errorf("ProtectedNpmPackages() missing %q", name)
		}
		if !IsProtectedNpmPackage(name) {
			t.Errorf("IsProtectedNpmPackage(%q) = false, want true", name)
		}
	}
	if IsProtectedNpmPackage("left-pad") {
		t.Error("IsProtectedNpmPackage(left-pad) = true, want false")
	}
}

// NpmSource.Scan must drop packages owned by another update path (opencode-ai
// is owned by `opencode upgrade`) so the dashboard shows a single owner and the
// generic npm batch can never target them.
func TestNpmScan_DropsProtected(t *testing.T) {
	enableMocks()
	defer disableMocks()

	setMock("npm", []string{"outdated", "-g", "--json"}, `{
		"opencode-ai": {"current":"1.18.0","wanted":"1.18.16","latest":"1.18.16"},
		"@opencode-ai/cli": {"current":"0.1.0","wanted":"0.2.0","latest":"0.2.0"},
		"left-pad": {"current":"1.0.0","wanted":"1.3.0","latest":"1.3.0"}
	}`, nil)

	src := &NpmSource{}
	items, err := src.Scan(context.Background(), model.PlatformInfo{})
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	names := make(map[string]bool, len(items))
	for _, it := range items {
		names[it.Name] = true
	}
	for _, n := range []string{"opencode-ai", "@opencode-ai/cli"} {
		if names[n] {
			t.Errorf("protected package %q must be dropped from npm scan", n)
		}
	}
	if !names["left-pad"] {
		t.Error("non-protected package left-pad must remain in npm scan")
	}
}
