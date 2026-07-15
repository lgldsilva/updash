package retention

import (
	"os"
	"path/filepath"
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
