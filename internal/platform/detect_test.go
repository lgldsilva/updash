package platform

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestDetect_OS(t *testing.T) {
	p := Detect()
	if p.OS == "" {
		t.Error("Detect() OS should not be empty")
	}

	switch p.OS {
	case "darwin":
		if p.Distro != "macos" {
			t.Errorf("darwin distro = %q, want %q", p.Distro, "macos")
		}
	case "linux":
		if p.Distro == "" {
			t.Error("linux distro should not be empty")
		}
	case "windows":
		if p.Distro != "windows" {
			t.Errorf("windows distro = %q, want %q", p.Distro, "windows")
		}
	default:
		t.Errorf("unexpected OS: %s", p.OS)
	}
}

func TestHasFunction(t *testing.T) {
	// Go binary should always be findable via LookPath
	if !has("go") {
		t.Error("has('go') should be true (Go is in PATH)")
	}

	// This binary almost certainly doesn't exist
	if has("this-command-should-not-exist-xyz789") {
		t.Error("has() returned true for non-existent command")
	}
}

func TestDirExists(t *testing.T) {
	tmpDir := t.TempDir()

	// Existing dir
	if !dirExists(tmpDir) {
		t.Error("dirExists() should be true for existing temp dir")
	}

	// Non-existing dir
	if dirExists(filepath.Join(tmpDir, "nonexistent")) {
		t.Error("dirExists() should be false for non-existing dir")
	}

	// File is not a dir
	tmpFile := filepath.Join(tmpDir, "testfile")
	if err := os.WriteFile(tmpFile, []byte("data"), 0644); err != nil {
		t.Fatalf("cannot create test file: %v", err)
	}
	if dirExists(tmpFile) {
		t.Error("dirExists() should be false for a file")
	}
}

func TestDetect_HasToolFlags(t *testing.T) {
	p := Detect()

	// Go should always be available (it's how we're running this test)
	if !p.HasGo {
		t.Error("HasGo should be true (running via Go test)")
	}

	// Platform-specific checks
	switch p.OS {
	case "darwin":
		// Homebrew is common on macOS dev machines
		if p.HasNpm != has("npm") {
			t.Error("HasNpm inconsistent with has('npm')")
		}
	case "linux":
		// npm may or may not be installed
		if p.HasDocker != has("docker") {
			t.Error("HasDocker inconsistent with has('docker')")
		}
	case "windows":
		_ = runtime.GOOS // used for build constraint
	}
}
