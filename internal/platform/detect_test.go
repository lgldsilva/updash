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

func TestDistroFromOSRelease(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "ubuntu",
			content: "NAME=\"Ubuntu\"\nID=ubuntu\nID_LIKE=debian\nPRETTY_NAME=\"Ubuntu 24.04.5 LTS\"\n",
			want:    "ubuntu",
		},
		{
			name:    "debian",
			content: "PRETTY_NAME=\"Debian GNU/Linux 13 (trixie)\"\nID=debian\n",
			want:    "debian",
		},
		{
			name:    "arch",
			content: "NAME=\"Arch Linux\"\nID=arch\nBUILD_ID=rolling\n",
			want:    "arch",
		},
		{
			// CachyOS ships its own branding hook (cachyos-hooks), which rewrites
			// os-release to ID=cachyos + ID_LIKE=arch.
			name:    "cachyos",
			content: "NAME=\"CachyOS Linux\"\nPRETTY_NAME=\"CachyOS\"\nID=cachyos\nID_LIKE=arch\nBUILD_ID=rolling\n",
			want:    "cachyos",
		},
		{
			// The same machine when that branding never ran: CachyOS keeps the
			// plain Arch os-release, so it is reported as arch. Derived distros
			// are only as good as the ID they ship.
			name:    "cachyos without branding",
			content: "NAME=\"Arch Linux\"\nPRETTY_NAME=\"Arch Linux\"\nID=arch\nBUILD_ID=rolling\n",
			want:    "arch",
		},
		{
			name:    "manjaro",
			content: "NAME=\"Manjaro Linux\"\nID=manjaro\nID_LIKE=arch\n",
			want:    "manjaro",
		},
		{
			name:    "fedora",
			content: "NAME=\"Fedora Linux\"\nID=fedora\n",
			want:    "fedora",
		},
		{
			name:    "opensuse",
			content: "NAME=\"openSUSE Leap\"\nID=\"opensuse-leap\"\nID_LIKE=\"suse\"\n",
			want:    "opensuse",
		},
		{
			name:    "alpine",
			content: "NAME=\"Alpine Linux\"\nID=alpine\n",
			want:    "alpine",
		},
		{
			name:    "unknown id falls back to linux",
			content: "NAME=\"Void\"\nID=void\n",
			want:    "linux",
		},
		{
			name:    "empty content falls back to linux",
			content: "",
			want:    "linux",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := distroFromOSRelease(tt.content); got != tt.want {
				t.Errorf("distroFromOSRelease() = %q, want %q", got, tt.want)
			}
		})
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
