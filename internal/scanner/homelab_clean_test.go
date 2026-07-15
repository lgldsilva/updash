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
