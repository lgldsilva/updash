package scanner

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lgldsilva/updash/internal/model"
)

func TestHomelabCleanSource_meta(t *testing.T) {
	s := &HomelabCleanSource{}
	if s.Category() != model.CatHomelabClean || s.Label() == "" || s.Icon() == "" {
		t.Fatalf("meta broken")
	}
}

func TestParseDFUsedPercent(t *testing.T) {
	out := `Filesystem     1024-blocks      Used Available Capacity Mounted on
/dev/disk3s1s1   971350180  15000000 900000000      87% /
`
	if got := parseDFUsedPercent(out); got != 87 {
		t.Fatalf("got %d", got)
	}
	if parseDFUsedPercent("bad") != 0 {
		t.Fatal("bad input")
	}
}

func TestHomelabCleanSource_Scan_ageDirs(t *testing.T) {
	home := t.TempDir()
	oldHome := HomelabHome
	HomelabHome = func() string { return home }
	t.Cleanup(func() { HomelabHome = oldHome })

	// Force disk pressure off
	oldDisk := DiskUsedPercent
	DiskUsedPercent = func() int { return 10 }
	t.Cleanup(func() { DiskUsedPercent = oldDisk })

	m2 := filepath.Join(home, ".m2", "repository", "old-art")
	if err := os.MkdirAll(m2, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(m2, "x.jar"), []byte("jar-bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	// age the child directory past default 90d
	old := time.Now().AddDate(0, 0, -120)
	if err := os.Chtimes(m2, old, old); err != nil {
		t.Fatal(err)
	}

	// no docker → no container-logs / disk-pressure
	items, err := (&HomelabCleanSource{}).Scan(context.Background(), model.PlatformInfo{})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, it := range items {
		if it.Name == "dev-cache:maven" && it.Status == model.StatusCleanCandidate {
			found = true
			if it.PackageID == "" {
				t.Fatal("expected PackageID path")
			}
		}
	}
	if !found {
		t.Fatalf("expected maven clean candidate, got %+v", items)
	}
}

func TestHomelabCleanSource_diskPressure(t *testing.T) {
	home := t.TempDir()
	oldHome := HomelabHome
	HomelabHome = func() string { return home }
	t.Cleanup(func() { HomelabHome = oldHome })

	oldDisk := DiskUsedPercent
	DiskUsedPercent = func() int { return 99 }
	t.Cleanup(func() { DiskUsedPercent = oldDisk })

	items, err := (&HomelabCleanSource{}).Scan(context.Background(), model.PlatformInfo{HasDocker: true})
	if err != nil {
		t.Fatal(err)
	}
	var pressure, clog bool
	for _, it := range items {
		if it.Name == "disk-pressure" {
			pressure = true
		}
		if it.Name == "container-logs" {
			clog = true
		}
	}
	if !pressure || !clog {
		t.Fatalf("pressure=%v container-logs=%v items=%+v", pressure, clog, items)
	}
}

func TestScanAgeDir_empty(t *testing.T) {
	if got := scanAgeDir("x", filepath.Join(t.TempDir(), "missing"), 7, time.Now(), "p"); len(got) != 0 {
		t.Fatalf("got %v", got)
	}
}

// --- dev-cache:projects-builds (opt-in) ---

func mkProjTree(t *testing.T, root, name string, files map[string]string) string {
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

func ageProjTree(t *testing.T, root string, mt time.Time) {
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

// stubHomelabHome isolates the scan from the real HOME and disk probe.
func stubHomelabHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	oldHome := HomelabHome
	HomelabHome = func() string { return home }
	t.Cleanup(func() { HomelabHome = oldHome })
	oldDisk := DiskUsedPercent
	DiskUsedPercent = func() int { return 10 }
	t.Cleanup(func() { DiskUsedPercent = oldDisk })
	return home
}

func findItem(items []*model.Item, name string) *model.Item {
	for _, it := range items {
		if it.Name == name {
			return it
		}
	}
	return nil
}

func TestHomelabCleanSource_projectsBuilds_optOutByDefault(t *testing.T) {
	stubHomelabHome(t)
	projects := t.TempDir()
	proj := mkProjTree(t, projects, "stale", map[string]string{
		"node_modules/dep/index.js": "module-bytes",
		"target/app.bin":            "bin-bytes",
	})
	ageProjTree(t, proj, time.Now().AddDate(0, 0, -120))

	t.Setenv("UPDASH_DEV_PROJECTS_CLEAN", "")
	t.Setenv("UPDASH_DEV_PROJECTS_DIR", projects)

	items, err := (&HomelabCleanSource{}).Scan(context.Background(), model.PlatformInfo{})
	if err != nil {
		t.Fatal(err)
	}
	if it := findItem(items, "dev-cache:projects-builds"); it != nil {
		t.Fatalf("projects-builds must be opt-in, got %+v", it)
	}
}

func TestHomelabCleanSource_projectsBuilds_optIn(t *testing.T) {
	stubHomelabHome(t)
	projects := t.TempDir()
	proj := mkProjTree(t, projects, "stale", map[string]string{
		"node_modules/dep/index.js": "module-bytes",
		"target/app.bin":            "bin-bytes",
	})
	ageProjTree(t, proj, time.Now().AddDate(0, 0, -120))

	t.Setenv("UPDASH_DEV_PROJECTS_CLEAN", "1")
	t.Setenv("UPDASH_DEV_PROJECTS_DIR", projects)

	items, err := (&HomelabCleanSource{}).Scan(context.Background(), model.PlatformInfo{})
	if err != nil {
		t.Fatal(err)
	}
	it := findItem(items, "dev-cache:projects-builds")
	if it == nil {
		t.Fatalf("expected projects-builds item with opt-in, got %+v", items)
	}
	if it.Status != model.StatusCleanCandidate || it.PackageID != projects {
		t.Fatalf("item=%+v", it)
	}
	if it.RemoveCount != 2 {
		t.Fatalf("RemoveCount=%d, want 2 (node_modules + target)", it.RemoveCount)
	}
}

func TestHomelabCleanSource_projectsBuilds_protectsRecentDescendants(t *testing.T) {
	stubHomelabHome(t)
	projects := t.TempDir()
	proj := mkProjTree(t, projects, "active", map[string]string{
		"node_modules/dep/index.js": "module-bytes",
		"build/out.js":              "bundle-bytes",
		"src/main.go":               "package main",
	})
	ageProjTree(t, proj, time.Now().AddDate(0, 0, -120))
	// src/ was touched recently: node_modules must survive even though the
	// project dir mtime is old.
	recent := time.Now().AddDate(0, 0, -1)
	if err := os.Chtimes(filepath.Join(proj, "src"), recent, recent); err != nil {
		t.Fatal(err)
	}

	t.Setenv("UPDASH_DEV_PROJECTS_CLEAN", "1")
	t.Setenv("UPDASH_DEV_PROJECTS_DIR", projects)

	items, err := (&HomelabCleanSource{}).Scan(context.Background(), model.PlatformInfo{})
	if err != nil {
		t.Fatal(err)
	}
	it := findItem(items, "dev-cache:projects-builds")
	if it == nil {
		t.Fatalf("expected projects-builds item (build dir), got %+v", items)
	}
	if it.RemoveCount != 1 {
		t.Fatalf("RemoveCount=%d, want 1 (build only; node_modules protected)", it.RemoveCount)
	}
}

func TestHomelabCleanSource_projectsBuilds_rejectsHomeAsRoot(t *testing.T) {
	home := stubHomelabHome(t)
	t.Setenv("HOME", home)
	proj := mkProjTree(t, home, "stale", map[string]string{"target/app.bin": "bin-bytes"})
	ageProjTree(t, proj, time.Now().AddDate(0, 0, -120))

	t.Setenv("UPDASH_DEV_PROJECTS_CLEAN", "1")
	t.Setenv("UPDASH_DEV_PROJECTS_DIR", home)

	items, err := (&HomelabCleanSource{}).Scan(context.Background(), model.PlatformInfo{})
	if err != nil {
		t.Fatal(err)
	}
	it := findItem(items, "dev-cache:projects-builds")
	if it == nil {
		t.Fatalf("unsafe root must surface as an unverified item, got %+v", items)
	}
	if it.Status != model.StatusUnverified {
		t.Fatalf("unsafe root must not produce an affirmative candidate: %+v", it)
	}
	if it.RemoveCount != 0 {
		t.Fatalf("nothing may be selected for removal: %+v", it)
	}
}
