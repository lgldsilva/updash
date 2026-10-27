package retention

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"
)

// release creates dir/name with a file of size bytes and the given mtime.
func release(t *testing.T, dir, name string, size int, mod time.Time) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Join(p, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(p, "bin", "tool"), make([]byte, size), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(p, mod, mod); err != nil {
		t.Fatal(err)
	}
	return p
}

func names(c []PathCandidate) []string {
	out := make([]string, len(c))
	for i, x := range c {
		out[i] = filepath.Base(x.Path)
	}
	sort.Strings(out)
	return out
}

func TestSupersededReleasesKeepsCurrentSymlinkTarget(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "releases")
	t0 := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	release(t, dir, "0.163.0", 10, t0)
	cur := release(t, dir, "0.164.0", 20, t0.Add(time.Hour))
	// newest by mtime is NOT the one in use: current must win
	release(t, dir, "0.165.0-rc", 30, t0.Add(2*time.Hour))
	if err := os.Symlink(cur, filepath.Join(root, CurrentLink)); err != nil {
		t.Fatal(err)
	}
	got, total, err := SupersededReleases(dir)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"0.163.0", "0.165.0-rc"}; !equal(names(got), want) {
		t.Fatalf("got %v want %v", names(got), want)
	}
	if total != 40 {
		t.Fatalf("total %d want 40", total)
	}
}

func TestSupersededReleasesWithoutCurrentKeepsNewest(t *testing.T) {
	dir := t.TempDir()
	t0 := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	release(t, dir, "grok-1.0.67", 5, t0)
	release(t, dir, "grok-1.0.70", 5, t0.Add(time.Hour))
	got, _, err := SupersededReleases(dir)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"grok-1.0.67"}; !equal(names(got), want) {
		t.Fatalf("got %v want %v", names(got), want)
	}
}

func TestSupersededReleasesIgnoresCurrentOutsideDir(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "releases")
	t0 := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	release(t, dir, "a", 1, t0)
	release(t, dir, "b", 1, t0.Add(time.Hour))
	elsewhere := release(t, root, "other", 1, t0)
	if err := os.Symlink(elsewhere, filepath.Join(root, CurrentLink)); err != nil {
		t.Fatal(err)
	}
	got, _, err := SupersededReleases(dir)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"a"}; !equal(names(got), want) {
		t.Fatalf("current outside dir must fall back to newest: got %v", names(got))
	}
}

func TestSupersededReleasesNothingToDo(t *testing.T) {
	for name, dir := range map[string]string{
		"missing": filepath.Join(t.TempDir(), "nope"),
		"empty":   t.TempDir(),
	} {
		got, total, err := SupersededReleases(dir)
		if err != nil || len(got) != 0 || total != 0 {
			t.Fatalf("%s: got %v %d %v", name, got, total, err)
		}
	}
	single := t.TempDir()
	release(t, single, "only", 3, time.Now())
	if got, _, _ := SupersededReleases(single); len(got) != 0 {
		t.Fatalf("single release must be kept: %v", got)
	}
}

func TestReleaseInUseUnreadableDir(t *testing.T) {
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ReleaseInUse(file); err == nil {
		t.Fatal("a regular file is not a releases dir")
	}
	if _, _, err := SupersededReleases(file); err == nil {
		t.Fatal("error must propagate")
	}
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
