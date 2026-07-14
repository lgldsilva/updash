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
	os.MkdirAll(javaDir, 0755)
	os.Mkdir(filepath.Join(javaDir, "11.0.25-tem"), 0755)
	os.Mkdir(filepath.Join(javaDir, "17.0.13-tem"), 0755)
	os.Mkdir(filepath.Join(javaDir, "21.0.5-tem"), 0755)
	os.Mkdir(filepath.Join(javaDir, "21.0.7-tem"), 0755)
	os.Mkdir(filepath.Join(javaDir, "current"), 0755)

	// Create Gradle versions
	gradleDir := filepath.Join(sdkmanDir, "gradle")
	os.MkdirAll(gradleDir, 0755)
	os.Mkdir(filepath.Join(gradleDir, "8.14.1"), 0755)
	os.Mkdir(filepath.Join(gradleDir, "8.14.4"), 0755)
	os.Mkdir(filepath.Join(gradleDir, "9.4.1"), 0755)
	os.Mkdir(filepath.Join(gradleDir, "current"), 0755)

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
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

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
	os.MkdirAll(tmpDir, 0755)
	os.Mkdir(filepath.Join(tmpDir, "vscjava.vscode-java-dependency-0.27.2-universal"), 0755)
	os.Mkdir(filepath.Join(tmpDir, "vscjava.vscode-java-dependency-0.27.4-universal"), 0755)
	os.Mkdir(filepath.Join(tmpDir, "ms-python.python-2026.4.0-universal"), 0755)

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
