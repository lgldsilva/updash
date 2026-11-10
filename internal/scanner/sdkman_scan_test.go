package scanner

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/lgldsilva/updash/internal/model"
)

func TestSDKMANSource_Scan_WithTempDirs(t *testing.T) {
	// Create a temporary SDKMAN-like directory structure
	tmpDir := t.TempDir()
	sdkmanDir := filepath.Join(tmpDir, ".sdkman", "candidates")

	// Create Java versions
	javaDir := filepath.Join(sdkmanDir, "java")
	_ = os.MkdirAll(javaDir, 0755)
	_ = os.Mkdir(filepath.Join(javaDir, "11.0.25-tem"), 0755)
	_ = os.Mkdir(filepath.Join(javaDir, "17.0.13-tem"), 0755)
	_ = os.Mkdir(filepath.Join(javaDir, "21.0.5-tem"), 0755)
	_ = os.Mkdir(filepath.Join(javaDir, "21.0.7-tem"), 0755)
	_ = os.Symlink("21.0.7-tem", filepath.Join(javaDir, "current"))

	// Create Gradle versions
	gradleDir := filepath.Join(sdkmanDir, "gradle")
	_ = os.MkdirAll(gradleDir, 0755)
	_ = os.Mkdir(filepath.Join(gradleDir, "8.14.1"), 0755)
	_ = os.Mkdir(filepath.Join(gradleDir, "8.14.4"), 0755)
	_ = os.Mkdir(filepath.Join(gradleDir, "9.4.1"), 0755)
	_ = os.Symlink("9.4.1", filepath.Join(gradleDir, "current"))

	// Verify structure
	entries, err := os.ReadDir(sdkmanDir)
	if err != nil {
		t.Fatalf("cannot read sdkman dir: %v", err)
	}
	t.Logf("SDKMAN candidates: %d entries", len(entries))
	for _, e := range entries {
		t.Logf("  candidate: %s (dir: %v)", e.Name(), e.IsDir())
	}

	// Save original HOME and override to point to tmpDir
	origHome := os.Getenv("HOME")
	_ = os.Setenv("HOME", tmpDir)
	defer func() { _ = os.Setenv("HOME", origHome) }()

	src := &SDKMANSource{}
	ctx := context.Background()

	items, err := src.Scan(ctx, model.PlatformInfo{})
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	t.Logf("Got %d items:", len(items))
	for _, it := range items {
		t.Logf("  %s: current=%q status=%s reclaim=%q", it.Name, it.CurrentVer, it.Status, it.Reclaimable)
	}

	// Should have at least: java 11, 17, 21, gradle 8, 9 = 5 items
	if len(items) < 4 {
		t.Fatalf("expected at least 4 items, got %d", len(items))
	}

	// Count cleanup candidates
	cleanCount := 0
	for _, it := range items {
		if it.Status == model.StatusCleanCandidate {
			cleanCount++
		}
	}

	if cleanCount < 1 {
		t.Errorf("expected at least 1 cleanup candidate (java 21 or gradle 8), got %d", cleanCount)
	}
}

func TestVSCodeCleanSource_Scan_WithTempDirs(t *testing.T) {
	tmpDir := t.TempDir()

	// Create VS Code-like extension dir with duplicate versions
	_ = os.MkdirAll(tmpDir, 0755)
	_ = os.Mkdir(filepath.Join(tmpDir, "vscjava.vscode-java-dependency-0.27.2-universal"), 0755)
	_ = os.Mkdir(filepath.Join(tmpDir, "vscjava.vscode-java-dependency-0.27.4-universal"), 0755)
	_ = os.Mkdir(filepath.Join(tmpDir, "ms-python.python-2026.4.0-universal"), 0755)

	// Verify structure
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("cannot read ext dir: %v", err)
	}
	t.Logf("Extensions: %d entries", len(entries))

	src := &VSCodeCleanSource{
		LabelName: "Test Ext",
		ExtDir:    tmpDir,
	}
	ctx := context.Background()

	items, err := src.Scan(ctx, model.PlatformInfo{})
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	t.Logf("Got %d items:", len(items))
	for _, it := range items {
		t.Logf("  %s: status=%s remove=%d", it.Name, it.Status, it.RemoveCount)
	}

	// Should find 1 duplicate (vscjava.vscode-java-dependency)
	found := false
	for _, it := range items {
		if it.RemoveCount > 0 {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected to find a duplicate extension, but none found")
	}
}

// Regression: the SDKMAN default (`current`) must not be counted as removable.
func TestSDKMANScan_DefaultVersionNotCountedAsRemovable(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	javaDir := filepath.Join(home, ".sdkman", "candidates", "java")
	for _, v := range []string{"21.0.1-tem", "21.0.2-tem", "21.0.3-tem"} {
		if err := os.MkdirAll(filepath.Join(javaDir, v), 0o750); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(filepath.Join(javaDir, "21.0.1-tem"), filepath.Join(javaDir, "current")); err != nil {
		t.Fatal(err)
	}
	items, err := (&SDKMANSource{}).Scan(context.Background(), model.PlatformInfo{})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("want 1 item, got %d", len(items))
	}
	if items[0].RemoveCount != 1 || items[0].Reclaimable != "1 versions" {
		t.Fatalf("want 1 removable (default kept), got count=%d reclaim=%q", items[0].RemoveCount, items[0].Reclaimable)
	}
}

func TestSDKMANScan_CopiedCurrentOffersNoCleanup(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	javaDir := filepath.Join(home, ".sdkman", "candidates", "java")
	for _, v := range []string{"21.0.1-tem", "21.0.3-tem", "current"} {
		if err := os.MkdirAll(filepath.Join(javaDir, v), 0o750); err != nil {
			t.Fatal(err)
		}
	}
	items, err := (&SDKMANSource{}).Scan(context.Background(), model.PlatformInfo{})
	if err != nil || len(items) != 1 {
		t.Fatalf("items=%+v err=%v", items, err)
	}
	if it := items[0]; it.Status == model.StatusCleanCandidate || it.RemoveCount != 0 {
		t.Fatalf("unknown default must not be a clean candidate: %+v", it)
	}
}

func TestVSCodeCleanSource_Scan_RespectsActiveManifest(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"a.b-3.0.0", "a.b-2.0.0", "a.b-1.0.0"} {
		if err := os.Mkdir(filepath.Join(dir, n), 0o750); err != nil {
			t.Fatal(err)
		}
	}
	manifest := `[{"identifier":{"id":"a.b"},"version":"2.0.0","relativeLocation":"a.b-2.0.0"}]`
	if err := os.WriteFile(filepath.Join(dir, "extensions.json"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	items, err := (&VSCodeCleanSource{LabelName: "T", ExtDir: dir}).Scan(context.Background(), model.PlatformInfo{})
	if err != nil || len(items) != 1 {
		t.Fatalf("items=%+v err=%v", items, err)
	}
	if it := items[0]; it.CurrentVer != "2.0.0" || it.RemoveCount != 2 {
		t.Fatalf("want active 2.0.0 kept and 2 removable, got current=%q remove=%d", it.CurrentVer, it.RemoveCount)
	}
}

func TestVSCodeCleanSource_Scan_CorruptManifestIsInfo(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"a.b-2.0.0", "a.b-1.0.0"} {
		if err := os.Mkdir(filepath.Join(dir, n), 0o750); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "extensions.json"), []byte("{broken"), 0o600); err != nil {
		t.Fatal(err)
	}
	items, _ := (&VSCodeCleanSource{LabelName: "T", ExtDir: dir}).Scan(context.Background(), model.PlatformInfo{})
	if len(items) != 1 || items[0].Status == model.StatusCleanCandidate {
		t.Fatalf("unreadable manifest must not offer cleanup: %+v", items)
	}
}
