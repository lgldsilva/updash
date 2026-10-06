package scanner

import (
	"context"
	"os"
	"testing"

	"github.com/lgldsilva/updash/internal/model"
)

func TestOpenCodeMajorAndChannel(t *testing.T) {
	cases := []struct {
		version string
		major   int
		pkg     string
	}{
		{version: "1.18.4", major: 1, pkg: OpenCodePackageV1},
		{version: "opencode v2.0.24", major: 2, pkg: OpenCodePackageV2},
		{version: "v2.0.24", major: 2, pkg: OpenCodePackageV2},
		{version: "2.0.0-beta.1", major: 2, pkg: OpenCodePackageV2},
		{version: "", major: 0, pkg: OpenCodePackageV1},
		{version: "not-a-version", major: 0, pkg: OpenCodePackageV1},
	}
	for _, tc := range cases {
		t.Run(tc.version, func(t *testing.T) {
			if got := OpenCodeMajor(tc.version); got != tc.major {
				t.Errorf("OpenCodeMajor(%q) = %d, want %d", tc.version, got, tc.major)
			}
			if got := OpenCodeChannelPackage(tc.version); got != tc.pkg {
				t.Errorf("OpenCodeChannelPackage(%q) = %q, want %q", tc.version, got, tc.pkg)
			}
		})
	}
}

func TestOpenCodeCanonicalCurl(t *testing.T) {
	t.Setenv("HOME", "/home/u")
	if !OpenCodeCanonicalCurl("/home/u/.opencode/bin/opencode") {
		t.Fatal("the current user's installer path must be canonical curl")
	}
	if !OpenCodeCanonicalCurl("/home/u/.opencode/bin/opencode.exe") {
		t.Fatal("the current user's opencode.exe installer path must be canonical curl")
	}
	for _, path := range []string{
		"/home/u/.local/bin/opencode-v2",
		"/home/u/.local/bin/opencode",
		"/home/u/.opencode/bin/opencode-v1",
		"/srv/other/.opencode/bin/opencode",
		"/home/u/.OpenCode/bin/opencode",
		"",
	} {
		if OpenCodeCanonicalCurl(path) {
			t.Errorf("OpenCodeCanonicalCurl(%q) = true, want false", path)
		}
	}
}

func TestOpenCodeCanonicalCurl_NoHome(t *testing.T) {
	prev := openCodeUserHome
	openCodeUserHome = func() (string, error) { return "", os.ErrNotExist }
	t.Cleanup(func() { openCodeUserHome = prev })
	if OpenCodeCanonicalCurl("/home/u/.opencode/bin/opencode") {
		t.Fatal("missing home must not treat any path as the curl install")
	}
}

func TestApplyOpenCodeChannel(t *testing.T) {
	v2 := &model.Item{Name: "OpenCode", Status: model.StatusInfo, PackageID: OpenCodePackageV1, CurrentVer: "2.0.24"}
	applyOpenCodeChannel(v2)
	if v2.PackageID != OpenCodePackageV2 {
		t.Fatalf("v2 PackageID = %q, want %s", v2.PackageID, OpenCodePackageV2)
	}
	v1 := &model.Item{Name: "OpenCode", Status: model.StatusInfo, CurrentVer: "1.18.4"}
	applyOpenCodeChannel(v1)
	if v1.PackageID != OpenCodePackageV1 {
		t.Fatalf("v1 PackageID = %q, want %s", v1.PackageID, OpenCodePackageV1)
	}
	broken := &model.Item{Status: model.StatusUnverified, PackageID: OpenCodePackageV1, CurrentVer: "2.0.24"}
	applyOpenCodeChannel(broken)
	if broken.PackageID != OpenCodePackageV1 {
		t.Fatal("unverified probe must keep the catalog package")
	}
}

// v2's `opencode --version` prints a name prefix. The probe must keep 2.0.24
// and select @opencode/cli, not the catalog default opencode-ai.
func TestProbeOpenCode_VersionPrefixSelectsV2Channel(t *testing.T) {
	enableMocks()
	defer disableMocks()
	setMock(binOpenCode, []string{flagVersion}, "opencode v2.0.24\n", nil)

	def, ok := lookupAgentDef("OpenCode")
	if !ok {
		t.Fatal("OpenCode missing from catalog")
	}
	it := probeAgentItem(context.Background(), model.PlatformInfo{}, def)
	if it.CurrentVer != "2.0.24" {
		t.Fatalf("CurrentVer = %q, want 2.0.24", it.CurrentVer)
	}
	if it.PackageID != OpenCodePackageV2 {
		t.Fatalf("PackageID = %q, want %s", it.PackageID, OpenCodePackageV2)
	}
	if it.Status == model.StatusUnverified {
		t.Fatalf("prefixed version must probe cleanly: %+v", it)
	}
}
