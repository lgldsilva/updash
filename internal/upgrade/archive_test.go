package upgrade

import "testing"

func TestArchiveName(t *testing.T) {
	tests := []struct {
		tag, goos, goarch, want string
	}{
		{"v0.1.8", "darwin", "arm64", "updash_0.1.8_darwin_arm64.tar.gz"},
		{"0.1.8", "linux", "amd64", "updash_0.1.8_linux_amd64.tar.gz"},
		{"v1.0.0", "windows", "amd64", "updash_1.0.0_windows_amd64.zip"},
	}
	for _, tt := range tests {
		if got := archiveName(tt.tag, tt.goos, tt.goarch); got != tt.want {
			t.Errorf("archiveName(%q, %q, %q) = %q, want %q", tt.tag, tt.goos, tt.goarch, got, tt.want)
		}
	}
}
