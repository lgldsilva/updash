package retention

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestIsOlderThan(t *testing.T) {
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	old := now.AddDate(0, 0, -10)
	if !IsOlderThan(old, 7, now) {
		t.Fatal("10d old should exceed 7d retention")
	}
	if IsOlderThan(old, 14, now) {
		t.Fatal("10d old should be within 14d retention")
	}
	if IsOlderThan(old, 0, now) {
		t.Fatal("maxDays 0 disables age")
	}
}

func TestDiskPressureTriggered(t *testing.T) {
	if !DiskPressureTriggered(90, 85) {
		t.Fatal("90 >= 85")
	}
	if DiskPressureTriggered(80, 85) {
		t.Fatal("80 < 85")
	}
	if DiskPressureTriggered(100, 0) {
		t.Fatal("threshold 0 disables")
	}
}

func TestCollectOldPaths_and_Remove(t *testing.T) {
	root := t.TempDir()
	oldDir := filepath.Join(root, "old")
	newDir := filepath.Join(root, "new")
	if err := os.Mkdir(oldDir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(newDir, 0o750); err != nil {
		t.Fatal(err)
	}
	oldFile := filepath.Join(oldDir, "a.txt")
	if err := os.WriteFile(oldFile, []byte("hello-world"), 0o600); err != nil {
		t.Fatal(err)
	}
	newFile := filepath.Join(newDir, "b.txt")
	if err := os.WriteFile(newFile, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	// Force mtimes: old = 30 days ago, new = now
	now := time.Now()
	oldTime := now.AddDate(0, 0, -30)
	if err := os.Chtimes(oldDir, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(oldFile, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(newDir, now, now); err != nil {
		t.Fatal(err)
	}

	cands, total, err := CollectOldPaths(root, 14, 1, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(cands) != 1 || filepath.Base(cands[0].Path) != "old" {
		t.Fatalf("cands=%v total=%d", cands, total)
	}
	if total <= 0 {
		t.Fatalf("expected positive size, got %d", total)
	}

	paths := []string{cands[0].Path}
	freed, errs := RemovePaths(paths)
	if len(errs) != 0 {
		t.Fatalf("errs=%v", errs)
	}
	if freed <= 0 {
		t.Fatalf("freed=%d", freed)
	}
	if _, err := os.Stat(oldDir); !os.IsNotExist(err) {
		t.Fatal("old dir should be removed")
	}
	if _, err := os.Stat(newDir); err != nil {
		t.Fatal("new dir should remain")
	}
}

func TestCollectOldPaths_missing(t *testing.T) {
	cands, total, err := CollectOldPaths(filepath.Join(t.TempDir(), "nope"), 7, 1, time.Now())
	if err != nil || len(cands) != 0 || total != 0 {
		t.Fatalf("missing: cands=%v total=%d err=%v", cands, total, err)
	}
}

func TestTruncateFileIfOver(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "log.txt")
	if err := os.WriteFile(p, []byte("0123456789abcdef"), 0o600); err != nil {
		t.Fatal(err)
	}
	ok, before, err := TruncateFileIfOver(p, 8)
	if err != nil || !ok || before != 16 {
		t.Fatalf("ok=%v before=%d err=%v", ok, before, err)
	}
	fi, err := os.Stat(p)
	if err != nil || fi.Size() != 0 {
		t.Fatalf("size=%v err=%v", fi.Size(), err)
	}
	ok, _, err = TruncateFileIfOver(p, 8)
	if err != nil || ok {
		t.Fatalf("already small: ok=%v err=%v", ok, err)
	}
	ok, _, err = TruncateFileIfOver(p, 0)
	if err != nil || ok {
		t.Fatalf("disabled: ok=%v err=%v", ok, err)
	}
	// directory is a no-op
	ok, _, err = TruncateFileIfOver(dir, 1)
	if err != nil || ok {
		t.Fatalf("dir: ok=%v err=%v", ok, err)
	}
	// missing file
	if _, _, err := TruncateFileIfOver(filepath.Join(dir, "nope"), 1); err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestCollectOldPaths_depthZero(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "f"), []byte("xx"), 0o600); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	old := now.AddDate(0, 0, -10)
	if err := os.Chtimes(filepath.Join(root, "f"), old, old); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(root, old, old); err != nil {
		t.Fatal(err)
	}
	cands, total, err := CollectOldPaths(root, 5, 0, now)
	if err != nil || len(cands) != 1 || total <= 0 {
		t.Fatalf("cands=%v total=%d err=%v", cands, total, err)
	}
	// fresh root should not qualify
	if err := os.Chtimes(root, now, now); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(filepath.Join(root, "f"), now, now); err != nil {
		t.Fatal(err)
	}
	cands, total, err = CollectOldPaths(root, 5, 0, now)
	if err != nil || len(cands) != 0 || total != 0 {
		t.Fatalf("fresh: cands=%v total=%d err=%v", cands, total, err)
	}
}

func TestRemovePaths_errors(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "gone")
	freed, errs := RemovePaths([]string{missing})
	if freed != 0 || len(errs) != 1 {
		t.Fatalf("freed=%d errs=%v", freed, errs)
	}

	// file remove success path
	dir := t.TempDir()
	f := filepath.Join(dir, "one.txt")
	if err := os.WriteFile(f, []byte("abc"), 0o600); err != nil {
		t.Fatal(err)
	}
	freed, errs = RemovePaths([]string{f})
	if len(errs) != 0 || freed != 3 {
		t.Fatalf("freed=%d errs=%v", freed, errs)
	}
}

func TestCollectOldPaths_fileChildren(t *testing.T) {
	root := t.TempDir()
	f := filepath.Join(root, "old.log")
	if err := os.WriteFile(f, []byte("logdata"), 0o600); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	old := now.AddDate(0, 0, -20)
	if err := os.Chtimes(f, old, old); err != nil {
		t.Fatal(err)
	}
	cands, total, err := CollectOldPaths(root, 7, 1, now)
	if err != nil || len(cands) != 1 || total != 7 {
		t.Fatalf("cands=%v total=%d err=%v", cands, total, err)
	}
}

func TestCollectOldPaths_protectsRecentDescendant(t *testing.T) {
	root := t.TempDir()
	oldDir := filepath.Join(root, "old-parent")
	if err := os.MkdirAll(filepath.Join(oldDir, "nested"), 0o750); err != nil {
		t.Fatal(err)
	}
	old := time.Now().AddDate(0, 0, -30)
	for _, name := range []string{"old-parent", "old-parent/nested", "old-parent/nested/old.txt"} {
		path := filepath.Join(root, name)
		if name == "old-parent/nested/old.txt" {
			if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		if err := os.Chtimes(path, old, old); err != nil {
			t.Fatal(err)
		}
	}
	recent := time.Now().AddDate(0, 0, -1)
	recentFile := filepath.Join(oldDir, "nested", "recent.txt")
	if err := os.WriteFile(recentFile, []byte("recent"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(recentFile, recent, recent); err != nil {
		t.Fatal(err)
	}

	cands, total, err := CollectOldPaths(root, 14, 1, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(cands) != 0 || total != 0 {
		t.Fatalf("recent descendant must protect parent: cands=%v total=%d", cands, total)
	}
}

func TestDirSizeCtxReturnsCancellation(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "file"), []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := dirSizeCtx(ctx, root); !errors.Is(err, context.Canceled) {
		t.Fatalf("dirSizeCtx error=%v, want context.Canceled", err)
	}
}

// --- CollectProjectBuildPaths ---

// mkProj scaffolds root/<name>/<children...> with the given file sizes and
// returns the project path.
func mkProj(t *testing.T, root, name string, files map[string]string) string {
	t.Helper()
	proj := filepath.Join(root, name)
	for rel, content := range files {
		p := filepath.Join(proj, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return proj
}

func ageTree(t *testing.T, root string, mt time.Time) {
	t.Helper()
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		return os.Chtimes(p, mt, mt)
	})
	if err != nil {
		t.Fatal(err)
	}
}

func collectPaths(cands []PathCandidate) []string {
	out := make([]string, len(cands))
	for i, c := range cands {
		out[i] = c.Path
	}
	return out
}

func hasPathSuffix(paths []string, suffix string) bool {
	for _, p := range paths {
		if strings.HasSuffix(filepath.ToSlash(p), suffix) {
			return true
		}
	}
	return false
}

func TestCollectProjectBuildPaths_recencyProtectsActiveProject(t *testing.T) {
	root := t.TempDir()
	now := time.Now()
	old := now.AddDate(0, 0, -120)

	// Fully stale project: node_modules eligible.
	stale := mkProj(t, root, "stale-proj", map[string]string{
		"node_modules/dep/index.js": "module-bytes",
		"target/app.bin":            "bin-bytes",
		"src/main.go":               "package main",
	})
	ageTree(t, stale, old)

	// Project dir mtime is old, but a first-level entry (src/) is recent:
	// node_modules must be protected even though the root dir looks stale.
	active := mkProj(t, root, "active-proj", map[string]string{
		"node_modules/dep/index.js": "module-bytes",
		"build/out.js":              "bundle-bytes",
		"src/main.go":               "package main",
	})
	ageTree(t, active, old)
	recent := now.AddDate(0, 0, -1)
	if err := os.Chtimes(filepath.Join(active, "src"), recent, recent); err != nil {
		t.Fatal(err)
	}

	cands, total, partialErrs, err := CollectProjectBuildPaths(context.Background(), root, 90, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(partialErrs) != 0 {
		t.Fatalf("unexpected partial errors: %v", partialErrs)
	}
	paths := collectPaths(cands)
	if !hasPathSuffix(paths, "stale-proj/node_modules") {
		t.Fatalf("stale node_modules not collected: %v", paths)
	}
	if !hasPathSuffix(paths, "stale-proj/target") {
		t.Fatalf("stale target not collected: %v", paths)
	}
	if !hasPathSuffix(paths, "active-proj/build") {
		t.Fatalf("build dir must always be collected: %v", paths)
	}
	if hasPathSuffix(paths, "active-proj/node_modules") {
		t.Fatalf("node_modules of recently-active project must be protected: %v", paths)
	}
	if total <= 0 {
		t.Fatalf("total=%d", total)
	}
}

func TestCollectProjectBuildPaths_symlinkEscape(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink privileges")
	}
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "big.bin"), []byte("outside-bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	old := now.AddDate(0, 0, -120)

	// node_modules is a symlink pointing outside the projects root.
	proj := mkProj(t, root, "link-proj", map[string]string{"src/main.go": "package main"})
	if err := os.Symlink(outside, filepath.Join(proj, "node_modules")); err != nil {
		t.Fatal(err)
	}
	ageTree(t, proj, old)

	// A symlinked project entry must not be walked at all.
	real := mkProj(t, root, "real-proj", map[string]string{"target/app.bin": "bin-bytes"})
	ageTree(t, real, old)
	if err := os.Symlink(real, filepath.Join(root, "alias-proj")); err != nil {
		t.Fatal(err)
	}

	cands, _, partialErrs, err := CollectProjectBuildPaths(context.Background(), root, 90, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(partialErrs) != 0 {
		t.Fatalf("unexpected partial errors: %v", partialErrs)
	}
	canon, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range cands {
		if !strings.HasPrefix(c.Path, canon+string(os.PathSeparator)) {
			t.Fatalf("candidate escapes projects root: %s", c.Path)
		}
		if strings.HasPrefix(c.Path, outside) {
			t.Fatalf("candidate points outside root via symlink: %s", c.Path)
		}
	}
	if len(cands) == 0 {
		t.Fatal("expected at least real-proj/target")
	}
}

func TestCollectProjectBuildPaths_neverCollectsWalkRoot(t *testing.T) {
	root := t.TempDir()
	now := time.Now()
	old := now.AddDate(0, 0, -120)
	// A project literally named "target" must not be collected wholesale.
	proj := mkProj(t, root, "target", map[string]string{"app.bin": "bin-bytes"})
	ageTree(t, proj, old)

	cands, _, _, err := CollectProjectBuildPaths(context.Background(), root, 90, now)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range cands {
		if c.Path == proj || c.Path == root {
			t.Fatalf("walk root collected: %s", c.Path)
		}
	}
}

func TestCollectProjectBuildPaths_missingRoot(t *testing.T) {
	cands, total, partialErrs, err := CollectProjectBuildPaths(context.Background(), filepath.Join(t.TempDir(), "nope"), 90, time.Now())
	if err != nil || len(cands) != 0 || total != 0 || len(partialErrs) != 0 {
		t.Fatalf("cands=%v total=%d partial=%v err=%v", cands, total, partialErrs, err)
	}
}

func TestCollectProjectBuildPaths_contextCanceled(t *testing.T) {
	root := t.TempDir()
	mkProj(t, root, "proj", map[string]string{"target/app.bin": "bin-bytes"})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, _, err := CollectProjectBuildPaths(ctx, root, 90, time.Now())
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestCollectProjectBuildPaths_partialErrors(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permission-based walk errors are not reliable as root")
	}
	root := t.TempDir()
	now := time.Now()
	old := now.AddDate(0, 0, -120)

	good := mkProj(t, root, "good-proj", map[string]string{"target/app.bin": "bin-bytes"})
	ageTree(t, good, old)

	bad := mkProj(t, root, "bad-proj", map[string]string{"locked/inner/x.txt": "x"})
	locked := filepath.Join(bad, "locked")
	if err := os.Chmod(locked, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o750) })

	cands, _, partialErrs, err := CollectProjectBuildPaths(context.Background(), root, 90, now)
	if err != nil {
		t.Fatalf("partial walk failures must not abort collection: %v", err)
	}
	if !hasPathSuffix(collectPaths(cands), "good-proj/target") {
		t.Fatalf("good project must still be collected: %v", cands)
	}
	if len(partialErrs) == 0 {
		t.Fatal("walk errors must be reported as partial failures, not swallowed")
	}
}

func TestCollectProjectBuildPaths_unreadableRoot(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permission-based errors are not reliable as root")
	}
	root := t.TempDir()
	if err := os.Chmod(root, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(root, 0o750) })
	_, _, _, err := CollectProjectBuildPaths(context.Background(), root, 90, time.Now())
	if err == nil {
		t.Fatal("unreadable root must surface a fatal error")
	}
}

func TestCollectProjectBuildPaths_rootNotDir(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(f, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	cands, total, partialErrs, err := CollectProjectBuildPaths(context.Background(), f, 90, time.Now())
	if err != nil || len(cands) != 0 || total != 0 || len(partialErrs) != 0 {
		t.Fatalf("cands=%v total=%d partial=%v err=%v", cands, total, partialErrs, err)
	}
}

func TestCollectProjectBuildPaths_skipsDotfilesAndFiles(t *testing.T) {
	root := t.TempDir()
	now := time.Now()
	old := now.AddDate(0, 0, -120)

	// Dotfile project and plain files at the top level are ignored.
	hidden := mkProj(t, root, ".hidden", map[string]string{"target/app.bin": "bin-bytes"})
	ageTree(t, hidden, old)
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte("n"), 0o600); err != nil {
		t.Fatal(err)
	}

	visible := mkProj(t, root, "proj", map[string]string{
		// Empty build dirs are skipped (nothing to reclaim) but still prune
		// the descent; .git is never entered.
		".git/objects/ab/cdef": "obj-bytes",
	})
	if err := os.MkdirAll(filepath.Join(visible, "dist"), 0o750); err != nil {
		t.Fatal(err)
	}
	ageTree(t, visible, old)

	cands, _, partialErrs, err := CollectProjectBuildPaths(context.Background(), root, 90, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(partialErrs) != 0 {
		t.Fatalf("unexpected partial errors: %v", partialErrs)
	}
	if len(cands) != 0 {
		t.Fatalf("expected no candidates, got %v", collectPaths(cands))
	}
}

func TestCollectProjectBuildPaths_unreadableProjectFailsSafe(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permission-based errors are not reliable as root")
	}
	root := t.TempDir()
	now := time.Now()
	old := now.AddDate(0, 0, -120)

	proj := mkProj(t, root, "locked-proj", map[string]string{
		"node_modules/dep/index.js": "module-bytes",
	})
	ageTree(t, proj, old)
	if err := os.Chmod(proj, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(proj, 0o750) })

	cands, _, partialErrs, err := CollectProjectBuildPaths(context.Background(), root, 90, now)
	if err != nil {
		t.Fatalf("one unreadable project must not abort collection: %v", err)
	}
	if hasPathSuffix(collectPaths(cands), "locked-proj/node_modules") {
		t.Fatal("unreadable project recency is unknown: node_modules must be kept")
	}
	if len(partialErrs) == 0 {
		t.Fatal("unreadable project must surface as a partial error")
	}
}

func TestCollectProjectBuildPaths_rejectsShallowRoot(t *testing.T) {
	// A root with a single path component ("/projects") would treat every
	// top-level directory as a project — refuse it even though it does not
	// exist here (validation must happen before/independent of Stat).
	shallow := string(os.PathSeparator) + "single"
	cands, _, _, err := CollectProjectBuildPaths(context.Background(), shallow, 90, time.Now())
	if err == nil {
		t.Fatalf("expected error for shallow root %q, got cands=%v", shallow, cands)
	}
	if len(cands) != 0 {
		t.Fatalf("nothing may be collected from %q: %v", shallow, cands)
	}
}

func TestCollectProjectBuildPaths_rejectsFilesystemRoot(t *testing.T) {
	cands, _, _, err := CollectProjectBuildPaths(context.Background(), string(os.PathSeparator), 90, time.Now())
	if err == nil {
		t.Fatalf("expected error for filesystem root, got cands=%v", cands)
	}
	if len(cands) != 0 {
		t.Fatalf("nothing may be collected from /: %v", cands)
	}
}

func TestCollectProjectBuildPaths_rejectsUserHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	// Without the guard, this stale project directly under HOME would be collected.
	proj := mkProj(t, home, "proj", map[string]string{"target/app.bin": "bin-bytes"})
	ageTree(t, proj, time.Now().AddDate(0, 0, -120))

	cands, _, _, err := CollectProjectBuildPaths(context.Background(), home, 90, time.Now())
	if err == nil {
		t.Fatalf("expected error when projects root == user home, got cands=%v", cands)
	}
	if len(cands) != 0 {
		t.Fatalf("nothing may be collected when root is the user home: %v", collectPaths(cands))
	}
	if _, statErr := os.Stat(filepath.Join(proj, "target", "app.bin")); statErr != nil {
		t.Fatal("guard must be fail-closed: nothing removed")
	}
}

func TestLatestProjectMtime_missingPath(t *testing.T) {
	gone := filepath.Join(t.TempDir(), "gone")
	var p []string
	if _, err := latestProjectMtime(gone, &p); err == nil {
		t.Fatal("expected error for missing project path")
	}
	// Fail-safe: undetermined recency means "active" (keep node_modules)
	// and must be recorded as a partial error, never silently stale.
	var partial []string
	if projectStale(gone, 90, time.Now(), &partial) {
		t.Fatal("undetermined recency must not classify a project as stale")
	}
	if len(partial) == 0 {
		t.Fatal("undetermined recency must surface as a partial error")
	}
}
