package scanner

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
)

func TestClassifyOpenCodePath(t *testing.T) {
	cases := []struct {
		path string
		want string
	}{
		{"", OpenCodeMethodUnknown},
		{"/home/u/.opencode/bin/opencode", OpenCodeMethodCurl},
		{"/home/u/.local/bin/opencode", OpenCodeMethodCurl},
		{"/home/u/.bun/install/global/node_modules/opencode-ai/bin/opencode", OpenCodeMethodBun},
		{"/home/u/.local/share/pnpm/global/5/node_modules/opencode-ai/bin/opencode", OpenCodeMethodPnpm},
		{"/opt/homebrew/bin/opencode", OpenCodeMethodBrew},
		{"/usr/local/Cellar/opencode/1.2.3/bin/opencode", OpenCodeMethodBrew},
		{"/usr/lib/node_modules/opencode-ai/bin/opencode.exe", OpenCodeMethodNpm},
		{"/home/u/.local/lib/node_modules/opencode-ai/bin/opencode", OpenCodeMethodNpm},
		{"/some/random/place/opencode", OpenCodeMethodUnknown},
	}
	for _, tc := range cases {
		if got := ClassifyOpenCodePath(tc.path); got != tc.want {
			t.Errorf("ClassifyOpenCodePath(%q) = %q, want %q", tc.path, got, tc.want)
		}
	}
}

func TestNpmPrefixForPath(t *testing.T) {
	cases := []struct {
		path string
		want string
	}{
		{"/usr/lib/node_modules/opencode-ai/bin/opencode", "/usr"},
		{"/home/u/.local/lib/node_modules/opencode-ai/bin/opencode", "/home/u/.local"},
		{"C:/Users/u/AppData/Roaming/npm/node_modules/opencode-ai/bin/opencode", "C:/Users/u/AppData/Roaming/npm"},
		{"/home/u/.opencode/bin/opencode", ""},
		{"node_modules/opencode-ai/bin/opencode", ""},
	}
	for _, tc := range cases {
		want := filepath.FromSlash(tc.want)
		if got := NpmPrefixForPath(tc.path); got != want {
			t.Errorf("NpmPrefixForPath(%q) = %q, want %q", tc.path, got, want)
		}
	}
}

// A path under a global node_modules whose directory is not writable is the
// Manjaro case: `opencode upgrade` cannot succeed there.
func TestOpenCodeInstall_SystemPrefix(t *testing.T) {
	stubOpenCodeLookup(t, "/usr/lib/node_modules/opencode-ai/bin/opencode", nil)
	prev := openCodeDirWritable
	openCodeDirWritable = func(string) bool { return false }
	t.Cleanup(func() { openCodeDirWritable = prev })

	got := OpenCodeInstall()
	if got.Method != OpenCodeMethodNpm || !got.SystemPrefix {
		t.Fatalf("got %+v, want npm install under a system prefix", got)
	}
	if got.NpmPrefix != filepath.FromSlash("/usr") {
		t.Fatalf("NpmPrefix = %q, want /usr", got.NpmPrefix)
	}
}

func TestOpenCodeInstall_UserOwnedCurl(t *testing.T) {
	stubOpenCodeLookup(t, "/home/u/.opencode/bin/opencode", nil)
	prev := openCodeDirWritable
	openCodeDirWritable = func(string) bool { return true }
	t.Cleanup(func() { openCodeDirWritable = prev })

	got := OpenCodeInstall()
	if got.Method != OpenCodeMethodCurl || got.SystemPrefix || got.NpmPrefix != "" {
		t.Fatalf("got %+v, want a writable curl install", got)
	}
}

func TestOpenCodeInstall_NotOnPath(t *testing.T) {
	stubOpenCodeLookup(t, "", errors.New("not found"))
	got := OpenCodeInstall()
	if got.Method != OpenCodeMethodUnknown || got.BinPath != "" {
		t.Fatalf("got %+v, want unknown/empty", got)
	}
}

// The classification follows the resolved target, matching opencode's own
// process.execPath-based detection: a launcher symlinked into node_modules is
// an npm install even when the link itself lives in ~/.opencode/bin.
func TestOpenCodeInstall_FollowsSymlink(t *testing.T) {
	stubOpenCodeLookup(t, "/home/u/.opencode/bin/opencode", nil)
	prevEval, prevWritable := openCodeEvalSymlinks, openCodeDirWritable
	openCodeEvalSymlinks = func(string) (string, error) {
		return "/home/u/.local/lib/node_modules/opencode-ai/bin/opencode", nil
	}
	openCodeDirWritable = func(string) bool { return true }
	t.Cleanup(func() { openCodeEvalSymlinks, openCodeDirWritable = prevEval, prevWritable })

	if got := OpenCodeInstall(); got.Method != OpenCodeMethodNpm {
		t.Fatalf("got %+v, want npm", got)
	}
}

func TestDirWritable(t *testing.T) {
	if !dirWritable(t.TempDir()) {
		t.Fatal("a temp dir must be writable")
	}
	if dirWritable("") {
		t.Fatal("empty dir must not be writable")
	}
	if dirWritable(filepath.Join(t.TempDir(), "missing")) {
		t.Fatal("missing dir must not be writable")
	}
	if runtime.GOOS != "windows" && os.Geteuid() != 0 {
		if dirWritable("/proc/self") {
			t.Fatal("a read-only system dir must not be writable")
		}
	}
}

// stubOpenCodeLookup pins the launcher path resolution (and disables symlink
// resolution) so classification can be tested without touching the machine.
func stubOpenCodeLookup(t *testing.T, path string, err error) {
	t.Helper()
	prevLook, prevEval := openCodeLookPath, openCodeEvalSymlinks
	openCodeLookPath = func(string) (string, error) { return path, err }
	openCodeEvalSymlinks = func(p string) (string, error) { return p, nil }
	t.Cleanup(func() { openCodeLookPath, openCodeEvalSymlinks = prevLook, prevEval })
}

func TestNpmPrefixForPath_RootLevel(t *testing.T) {
	if got := NpmPrefixForPath("/node_modules/opencode-ai/bin/opencode"); got != string(filepath.Separator) {
		t.Fatalf("got %q, want the filesystem root", got)
	}
}

// A binary owned by the distro package manager must be reported as such: it is
// updated by that package manager, never by npm writing over its files.
func TestOpenCodeInstall_SystemPackageOwned(t *testing.T) {
	stubOpenCodeLookup(t, "/usr/lib/node_modules/opencode-ai/bin/opencode", nil)
	prevWritable, prevOwner := openCodeDirWritable, openCodePackageOwner
	openCodeDirWritable = func(string) bool { return false }
	openCodePackageOwner = func(string) string { return "opencode-bin 1.2.3" }
	t.Cleanup(func() { openCodeDirWritable, openCodePackageOwner = prevWritable, prevOwner })

	if got := OpenCodeInstall(); got.SystemPackage != "opencode-bin 1.2.3" {
		t.Fatalf("got %+v, want the owning package recorded", got)
	}
}

// A writable install is never probed for package ownership.
func TestOpenCodeInstall_NoOwnershipProbeWhenWritable(t *testing.T) {
	stubOpenCodeLookup(t, "/home/u/.opencode/bin/opencode", nil)
	prevWritable, prevOwner := openCodeDirWritable, openCodePackageOwner
	openCodeDirWritable = func(string) bool { return true }
	openCodePackageOwner = func(string) string {
		t.Fatal("ownership must not be probed for a writable install")
		return ""
	}
	t.Cleanup(func() { openCodeDirWritable, openCodePackageOwner = prevWritable, prevOwner })
	_ = OpenCodeInstall()
}

func TestSystemPackageOwner(t *testing.T) {
	prevLook, prevExec := openCodeLookPath, execCommand
	t.Cleanup(func() { openCodeLookPath, execCommand = prevLook, prevExec })

	// No package manager available: nothing claims the file.
	openCodeLookPath = func(string) (string, error) { return "", errors.New("nope") }
	if got := systemPackageOwner("/usr/bin/opencode"); got != "" {
		t.Fatalf("got %q, want empty", got)
	}

	// First available tool answers.
	openCodeLookPath = func(name string) (string, error) {
		if name == "pacman" {
			return "/usr/bin/pacman", nil
		}
		return "", errors.New("nope")
	}
	var asked []string
	execCommand = func(_ context.Context, name string, args ...string) ([]byte, error) {
		asked = append([]string{name}, args...)
		return []byte("/usr/bin/opencode is owned by opencode-bin 1.2.3\n"), nil
	}
	got := systemPackageOwner("/usr/bin/opencode")
	if got != "/usr/bin/opencode is owned by opencode-bin 1.2.3" {
		t.Fatalf("got %q", got)
	}
	if !slices.Equal(asked, []string{"pacman", "-Qo", "/usr/bin/opencode"}) {
		t.Fatalf("queried %v", asked)
	}

	// A non-zero exit means the file belongs to no package.
	execCommand = func(context.Context, string, ...string) ([]byte, error) {
		return nil, errors.New("No package owns /usr/bin/opencode")
	}
	if got := systemPackageOwner("/usr/bin/opencode"); got != "" {
		t.Fatalf("got %q, want empty for an unowned file", got)
	}
}
