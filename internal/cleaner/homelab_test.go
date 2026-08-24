package cleaner

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lgldsilva/updash/internal/model"
)

func TestCleanHomelab_agePaths(t *testing.T) {
	root := t.TempDir()
	oldChild := filepath.Join(root, "stale")
	if err := os.Mkdir(oldChild, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(oldChild, "x"), []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	oldTime := time.Now().AddDate(0, 0, -40)
	if err := os.Chtimes(oldChild, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}

	it := &model.Item{
		Name:      "dev-cache:maven",
		Category:  model.CatHomelabClean,
		PackageID: root,
	}
	// ageDaysForHomelab uses config.DevCacheMaxDays() (90). Force via direct cleanAgePaths.
	res := cleanAgePaths(it, 14)
	if !res.Success {
		t.Fatalf("res=%+v", res)
	}
	if _, err := os.Stat(oldChild); !os.IsNotExist(err) {
		t.Fatal("stale path should be removed")
	}
}

func TestCleanHomelab_missingPath(t *testing.T) {
	it := &model.Item{Name: "dev-cache:maven", Category: model.CatHomelabClean}
	res := cleanOne(context.Background(), it, SilentOptions())
	if res.Success {
		t.Fatal("expected failure without PackageID")
	}
}

func TestAgeDaysForHomelab(t *testing.T) {
	if ageDaysForHomelab("dev-cache:x") <= 0 {
		t.Fatal("dev-cache")
	}
	if ageDaysForHomelab("ai-output:x") <= 0 {
		t.Fatal("ai-output")
	}
	if ageDaysForHomelab("host-logs:x") <= 0 {
		t.Fatal("host-logs")
	}
	if ageDaysForHomelab("other") != 30 {
		t.Fatal("default")
	}
}

func TestCleanHomelab_defaultBranch(t *testing.T) {
	it := &model.Item{Name: "unknown-thing", Category: model.CatHomelabClean}
	res := cleanHomelab(context.Background(), it, SilentOptions())
	if !res.Success {
		t.Fatalf("%+v", res)
	}
}

func TestCleanContainerLogs_noDocker(t *testing.T) {
	// Uses real docker if present; should still succeed.
	it := &model.Item{Name: "container-logs", Category: model.CatHomelabClean}
	res := cleanContainerLogs(context.Background(), it, SilentOptions())
	if !res.Success {
		t.Fatalf("%+v", res)
	}
}

// --- dev-cache:projects-builds (opt-in) ---

func mkCleanerProj(t *testing.T, root, name string, files map[string]string) string {
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

func ageCleanerTree(t *testing.T, root string, mt time.Time) {
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

func projectsBuildsFixture(t *testing.T) (projects, staleNM, activeNM string) {
	t.Helper()
	projects = t.TempDir()
	old := time.Now().AddDate(0, 0, -120)

	stale := mkCleanerProj(t, projects, "stale", map[string]string{
		"node_modules/dep/index.js": "module-bytes",
		"target/app.bin":            "bin-bytes",
	})
	ageCleanerTree(t, stale, old)
	staleNM = filepath.Join(stale, "node_modules")

	active := mkCleanerProj(t, projects, "active", map[string]string{
		"node_modules/dep/index.js": "module-bytes",
		"src/main.go":               "package main",
	})
	ageCleanerTree(t, active, old)
	recent := time.Now().AddDate(0, 0, -1)
	if err := os.Chtimes(filepath.Join(active, "src"), recent, recent); err != nil {
		t.Fatal(err)
	}
	activeNM = filepath.Join(active, "node_modules")
	return projects, staleNM, activeNM
}

func TestCleanProjectBuilds_optIn(t *testing.T) {
	projects, staleNM, activeNM := projectsBuildsFixture(t)
	t.Setenv("UPDASH_DEV_PROJECTS_CLEAN", "1")

	it := &model.Item{
		Name:      "dev-cache:projects-builds",
		Category:  model.CatHomelabClean,
		PackageID: projects,
	}
	res := cleanOne(context.Background(), it, SilentOptions())
	if !res.Success {
		t.Fatalf("res=%+v", res)
	}
	if _, err := os.Stat(staleNM); !os.IsNotExist(err) {
		t.Fatal("stale node_modules should be removed")
	}
	if _, err := os.Stat(activeNM); err != nil {
		t.Fatal("recently-active node_modules must be protected")
	}
}

func TestCleanProjectBuilds_optOutIsNoOp(t *testing.T) {
	projects, staleNM, _ := projectsBuildsFixture(t)
	t.Setenv("UPDASH_DEV_PROJECTS_CLEAN", "")

	it := &model.Item{
		Name:      "dev-cache:projects-builds",
		Category:  model.CatHomelabClean,
		PackageID: projects,
	}
	res := cleanOne(context.Background(), it, SilentOptions())
	if !res.Success {
		t.Fatalf("res=%+v", res)
	}
	if it.Status != model.StatusInfo {
		t.Fatalf("opt-out no-op must be informational (non-affirmative), got %v", it.Status)
	}
	if _, err := os.Stat(staleNM); err != nil {
		t.Fatal("opt-out must never remove project build paths")
	}
}

func TestCleanProjectBuilds_missingPath(t *testing.T) {
	t.Setenv("UPDASH_DEV_PROJECTS_CLEAN", "1")
	it := &model.Item{Name: "dev-cache:projects-builds", Category: model.CatHomelabClean}
	res := cleanOne(context.Background(), it, SilentOptions())
	if res.Success {
		t.Fatal("expected failure without PackageID")
	}
}

func TestCleanProjectBuilds_canceledCtx(t *testing.T) {
	projects, _, _ := projectsBuildsFixture(t)
	t.Setenv("UPDASH_DEV_PROJECTS_CLEAN", "1")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	it := &model.Item{
		Name:      "dev-cache:projects-builds",
		Category:  model.CatHomelabClean,
		PackageID: projects,
	}
	res := cleanOne(ctx, it, SilentOptions())
	if res.Success {
		t.Fatal("canceled context must fail the cleanup")
	}
	if _, err := os.Stat(projects); err != nil {
		t.Fatal("projects root must remain")
	}
}

func TestCleanProjectBuilds_rejectsUnsafeRoot(t *testing.T) {
	// UPDASH_DEV_PROJECTS_DIR=$HOME must never turn every home subdirectory
	// into a "project": the retention layer is fail-closed on unsafe roots.
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("UPDASH_DEV_PROJECTS_CLEAN", "1")
	proj := mkCleanerProj(t, home, "proj", map[string]string{"target/app.bin": "bin-bytes"})
	ageCleanerTree(t, proj, time.Now().AddDate(0, 0, -120))

	it := &model.Item{
		Name:      "dev-cache:projects-builds",
		Category:  model.CatHomelabClean,
		PackageID: home,
	}
	res := cleanOne(context.Background(), it, SilentOptions())
	if res.Success {
		t.Fatal("unsafe projects root must fail closed")
	}
	if _, err := os.Stat(filepath.Join(proj, "target", "app.bin")); err != nil {
		t.Fatal("nothing may be removed when the projects root is unsafe")
	}
}
