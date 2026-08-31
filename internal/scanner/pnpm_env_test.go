package scanner

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestPnpmGlobalDirs_UsesPnpmHomeAndSkipsMissing(t *testing.T) {
	home := t.TempDir()
	bin := filepath.Join(home, "bin")
	if err := os.Mkdir(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PNPM_HOME", home)
	t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "missing"))

	dirs := PnpmGlobalDirs()
	if !slices.Contains(dirs, home) || !slices.Contains(dirs, bin) {
		t.Fatalf("dirs = %v, want the pnpm home and its bin", dirs)
	}
	for _, dir := range dirs {
		if strings.Contains(dir, "missing") {
			t.Fatalf("non-existent dir returned: %q", dir)
		}
	}
}

func TestPnpmGlobalDirs_NoHomeIsEmpty(t *testing.T) {
	t.Setenv("PNPM_HOME", filepath.Join(t.TempDir(), "nope"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "nope"))
	t.Setenv("HOME", t.TempDir())
	t.Setenv("LOCALAPPDATA", filepath.Join(t.TempDir(), "nope"))
	if dirs := PnpmGlobalDirs(); len(dirs) != 0 {
		t.Fatalf("dirs = %v, want none", dirs)
	}
}

func TestEnsurePnpmPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("PNPM_HOME", home)
	sep := string(os.PathListSeparator)

	got := EnsurePnpmPath([]string{"PATH=/bin", "FOO=bar"})
	var path string
	for _, kv := range got {
		if strings.HasPrefix(kv, "PATH=") {
			path = strings.TrimPrefix(kv, "PATH=")
		}
	}
	if !strings.HasPrefix(path, "/bin"+sep) || !strings.Contains(path, home) {
		t.Fatalf("PATH = %q, want /bin plus the pnpm home", path)
	}
	if !slices.Contains(got, "FOO=bar") {
		t.Fatalf("unrelated env dropped: %v", got)
	}
	if !slices.Contains(got, "PNPM_HOME="+home) {
		t.Fatalf("PNPM_HOME not preserved/added: %v", got)
	}

	// An env without PATH gets one; an already-complete PATH is left alone.
	if got := EnsurePnpmPath([]string{}); !slices.ContainsFunc(got, func(kv string) bool {
		return strings.HasPrefix(kv, "PATH=") && strings.Contains(kv, home)
	}) {
		t.Fatalf("missing PATH not added: %v", got)
	}
	twice := EnsurePnpmPath(EnsurePnpmPath([]string{"PATH=/bin"}))
	for _, kv := range twice {
		if strings.HasPrefix(kv, "PATH=") && strings.Count(kv, home) != 1 {
			t.Fatalf("PATH entry duplicated: %q", kv)
		}
	}
}

func TestEnsurePnpmPath_NoGlobalHomeIsUnchanged(t *testing.T) {
	t.Setenv("PNPM_HOME", filepath.Join(t.TempDir(), "nope"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "nope"))
	t.Setenv("HOME", t.TempDir())
	t.Setenv("LOCALAPPDATA", filepath.Join(t.TempDir(), "nope"))
	env := []string{"PATH=/bin"}
	if got := EnsurePnpmPath(env); !slices.Equal(got, env) {
		t.Fatalf("env changed without a pnpm home: %v", got)
	}
}

func TestJoinIfSet(t *testing.T) {
	if got := joinIfSet("", "pnpm"); got != "" {
		t.Fatalf("got %q, want empty", got)
	}
	if got := joinIfSet("/a", "pnpm"); got != filepath.Join("/a", "pnpm") {
		t.Fatalf("got %q", got)
	}
}
