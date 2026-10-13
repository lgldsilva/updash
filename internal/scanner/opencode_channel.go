package scanner

import (
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/lgldsilva/updash/internal/model"
)

// npm distribution channels for the OpenCode CLI. v1 publishes opencode-ai;
// v2 publishes @opencode/cli. The PATH binary's major selects the channel so
// a preserved v1 install cannot hide a v2 release (or the reverse).
const (
	OpenCodePackageV1 = "opencode-ai"
	OpenCodePackageV2 = "@opencode/cli"
)

// OpenCodeMajor returns the leading semver component of an OpenCode version
// string (0 when it is missing or not numeric). Accepts the raw CLI line
// (`opencode v2.0.24`) as well as a bare version.
func OpenCodeMajor(version string) int {
	v := normalizeAgentVer(version)
	if v == "" {
		return 0
	}
	head, _, _ := strings.Cut(v, ".")
	n, err := strconv.Atoi(head)
	if err != nil || n < 0 {
		return 0
	}
	return n
}

// OpenCodeChannelPackage is the npm package whose latest version matches the
// installed major. Anything below 2 stays on the v1 channel, including an
// empty version (the catalog default, used when the probe did not run).
func OpenCodeChannelPackage(version string) string {
	if OpenCodeMajor(version) >= 2 {
		return OpenCodePackageV2
	}
	return OpenCodePackageV1
}

// openCodeUserHome resolves the current user's home. Tests replace it so the
// curl path does not depend on the machine running the suite.
var openCodeUserHome = os.UserHomeDir

// OpenCodeCanonicalCurl reports whether path is exactly the current user's
// curl installer file, $HOME/.opencode/bin/opencode (or opencode.exe).
// Equality is against that path, not a suffix: another tree that happens to
// end in /.opencode/bin/opencode is a different install. `opencode upgrade
// -m curl` always writes the current user's installer file, so a suffix
// match would replace a version switch that lives elsewhere.
//
// The comparison is case-sensitive except on Windows, where the filesystem
// is not. Symlinks are not followed here: OpenCodeInstall already resolved
// the binary, and a switch that points at this path must stay non-canonical.
func OpenCodeCanonicalCurl(path string) bool {
	if path == "" {
		return false
	}
	home, err := openCodeUserHome()
	if err != nil || home == "" {
		return false
	}
	got := filepath.Clean(path)
	base := filepath.Clean(filepath.Join(home, ".opencode", "bin", "opencode"))
	return sameInstallPath(got, base) || sameInstallPath(got, base+".exe")
}

func sameInstallPath(a, b string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

// applyOpenCodeChannel points the OpenCode agent item at the npm channel
// that matches the version just probed. The catalog npmPackage stays the v1
// default; PackageID on the item is what freshness and the updater read.
func applyOpenCodeChannel(it *model.Item) {
	if it == nil || it.Status == model.StatusUnverified {
		return
	}
	it.PackageID = OpenCodeChannelPackage(it.CurrentVer)
}
