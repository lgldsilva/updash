package scanner

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/lgldsilva/updash/internal/model"
)

func TestPacmanScan_CombinesCheckupdatesAndYay(t *testing.T) {
	enableMocks()
	defer disableMocks()

	// Official repos come from checkupdates, AUR from `yay -Qua`; the old
	// implementation only ran the latter and missed repo updates entirely.
	setMock("checkupdates", nil, "btop 1.3.0 -> 1.5.0\ngit 2.42.0 -> 2.45.0", nil)
	setMock("yay", []string{"-Qua"}, "aur/yay 12.3.0 -> 12.4.0", nil)

	src := &PacmanSource{}
	items, err := src.Scan(context.Background(), model.PlatformInfo{HasYay: true})
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("expected 3 combined items, got %d: %+v", len(items), items)
	}
	names := map[string]bool{}
	for _, it := range items {
		if it.Status != model.StatusOutdated {
			t.Errorf("item %s not outdated: %+v", it.Name, it)
		}
		names[it.Name] = true
	}
	for _, want := range []string{"btop", "git", "yay"} {
		if !names[want] {
			t.Errorf("missing combined item %q", want)
		}
	}
}

func TestPacmanScan_CheckupdatesFailureTolerated(t *testing.T) {
	enableMocks()
	defer disableMocks()

	// checkupdates missing (pacman-contrib not installed) must not fail the
	// source: the AUR probe alone still reports its updates.
	setMock("checkupdates", nil, "", errors.New("executable file not found"))
	setMock("yay", []string{"-Qua"}, "aur/yay 12.3.0 -> 12.4.0", nil)

	src := &PacmanSource{}
	items, err := src.Scan(context.Background(), model.PlatformInfo{HasYay: true})
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}
	if len(items) != 1 || items[0].Name != "yay" || items[0].Status != model.StatusOutdated {
		t.Fatalf("expected the AUR update only, got %+v", items)
	}
}

func TestPacmanScan_NoUpdatesAnywhereIsOK(t *testing.T) {
	enableMocks()
	defer disableMocks()

	setMock("checkupdates", nil, "", errors.New("exit status 2 (no updates)"))
	setMock("yay", []string{"-Qua"}, "", nil)

	src := &PacmanSource{}
	items, _ := src.Scan(context.Background(), model.PlatformInfo{HasYay: true})
	if len(items) != 1 || items[0].Status != model.StatusOK {
		t.Errorf("expected OK, got %+v", items)
	}
}

func TestPacmanScan_DedupesOverlappingSources(t *testing.T) {
	items := dedupePacmanItems([]*model.Item{
		{Name: "btop", Category: model.CatPacman, Status: model.StatusOutdated},
		{Name: "yay", Category: model.CatPacman, Status: model.StatusOutdated},
		{Name: "btop", Category: model.CatPacman, Status: model.StatusOutdated},
	})
	if len(items) != 2 {
		t.Fatalf("dedupe failed: %+v", items)
	}
}

func TestPacmanCleanSource_YayCacheAndOrphans(t *testing.T) {
	home := t.TempDir()
	yayCache := filepath.Join(home, ".cache", "yay")
	if err := os.MkdirAll(yayCache, 0o755); err != nil {
		t.Fatal(err)
	}
	oldHome := os.Getenv("HOME")
	_ = os.Setenv("HOME", home)
	t.Cleanup(func() { _ = os.Setenv("HOME", oldHome) })

	enableMocks()
	defer disableMocks()
	setMock("du", []string{"-sh", yayCache}, "640M\t"+yayCache, nil)
	setMock("pacman", []string{"-Qtdq"}, "leftover-lib\nstale-tool\n", nil)

	src := &PacmanCleanSource{}
	items, err := src.Scan(context.Background(), model.PlatformInfo{HasYay: true, HasPacman: true})
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	byName := map[string]*model.Item{}
	for _, it := range items {
		byName[it.Name] = it
	}

	yayItem, ok := byName["yay-cache"]
	if !ok {
		t.Fatalf("missing yay-cache item: %+v", items)
	}
	if yayItem.Status != model.StatusCleanCandidate || yayItem.CurrentVer != "640M" {
		t.Errorf("yay-cache item wrong: %+v", yayItem)
	}

	orphans, ok := byName["pacman-orphans"]
	if !ok {
		t.Fatalf("missing pacman-orphans item: %+v", items)
	}
	if orphans.Status != model.StatusCleanCandidate || orphans.RemoveCount != 2 {
		t.Errorf("orphans item wrong: %+v", orphans)
	}
}

func TestPacmanCleanSource_DuPartialPermissionErrorStillSizes(t *testing.T) {
	// du exits 1 when it cannot read root-owned subdirs (pacman download-*
	// temp dirs) but still prints the total — the item must stay a candidate.
	home := t.TempDir()
	yayCache := filepath.Join(home, ".cache", "yay")
	if err := os.MkdirAll(yayCache, 0o755); err != nil {
		t.Fatal(err)
	}
	oldHome := os.Getenv("HOME")
	_ = os.Setenv("HOME", home)
	t.Cleanup(func() { _ = os.Setenv("HOME", oldHome) })

	enableMocks()
	defer disableMocks()
	setMock("du", []string{"-sh", yayCache}, "19G\t"+yayCache+"\n", errors.New("exit status 1"))
	setMock("pacman", []string{"-Qtdq"}, "", nil)

	src := &PacmanCleanSource{}
	items, _ := src.Scan(context.Background(), model.PlatformInfo{HasYay: true})
	for _, it := range items {
		if it.Name == nameYayCache {
			if it.Status != model.StatusCleanCandidate || it.CurrentVer != "19G" {
				t.Fatalf("partial du error must not demote the item: %+v", it)
			}
			return
		}
	}
	t.Fatal("yay-cache item missing")
}

func TestPacmanCleanSource_NoOrphansNoItem(t *testing.T) {
	enableMocks()
	defer disableMocks()
	setMock("pacman", []string{"-Qtdq"}, "", nil)

	src := &PacmanCleanSource{}
	items, _ := src.Scan(context.Background(), model.PlatformInfo{HasPacman: true})
	for _, it := range items {
		if it.Name == "pacman-orphans" {
			t.Fatalf("orphan item must not appear when none exist: %+v", items)
		}
	}
}
