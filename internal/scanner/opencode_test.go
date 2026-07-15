package scanner

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/lgldsilva/updash/internal/model"
)

func TestOpenCodeSource_meta(t *testing.T) {
	s := &OpenCodeSource{}
	if s.Category() != model.CatOpenCodePlugins || s.Label() == "" || s.Icon() == "" {
		t.Fatalf("meta: cat=%s label=%s icon=%s", s.Category(), s.Label(), s.Icon())
	}
}

func TestOpenCodeSource_Scan_noPackageJSON(t *testing.T) {
	dir := t.TempDir()
	old := OpenCodeConfigDir
	OpenCodeConfigDir = func() string { return dir }
	t.Cleanup(func() { OpenCodeConfigDir = old })

	items, err := (&OpenCodeSource{}).Scan(context.Background(), model.PlatformInfo{})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Status != model.StatusOK {
		t.Fatalf("items=%v", items)
	}
	if items[0].CurrentVer != "no package.json" {
		t.Fatalf("ver=%q", items[0].CurrentVer)
	}
}

func TestOpenCodeSource_Scan_outdated(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"name":"x"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	oldDir := OpenCodeConfigDir
	OpenCodeConfigDir = func() string { return dir }
	t.Cleanup(func() { OpenCodeConfigDir = oldDir })

	oldExec := execCombined
	execCombined = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		return []byte(`{"plugin-a":{"current":"1.0.0","wanted":"1.1.0","latest":"1.2.0"}}`), nil
	}
	t.Cleanup(func() { execCombined = oldExec })

	items, err := (&OpenCodeSource{}).Scan(context.Background(), model.PlatformInfo{HasNpm: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Name != "plugin-a" || items[0].Status != model.StatusOutdated {
		t.Fatalf("items=%+v", items)
	}
	if items[0].AvailableVer != "1.2.0" {
		t.Fatalf("avail=%q", items[0].AvailableVer)
	}
}

func TestOpenCodeSource_Scan_upToDate(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	oldDir := OpenCodeConfigDir
	OpenCodeConfigDir = func() string { return dir }
	t.Cleanup(func() { OpenCodeConfigDir = oldDir })

	oldExec := execCombined
	execCombined = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		return []byte(`{}`), nil
	}
	t.Cleanup(func() { execCombined = oldExec })

	items, err := (&OpenCodeSource{}).Scan(context.Background(), model.PlatformInfo{})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Status != model.StatusOK {
		t.Fatalf("items=%+v", items)
	}
}
