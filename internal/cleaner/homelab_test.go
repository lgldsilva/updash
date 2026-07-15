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
