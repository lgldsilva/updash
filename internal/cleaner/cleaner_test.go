package cleaner

import (
	"testing"
)

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"1.0.0", "2.0.0", -1},
		{"2.0.0", "1.0.0", 1},
		{"1.0.0", "1.0.0", 0},
		{"0.27.2", "0.27.4", -1},
		{"0.27.4", "0.27.2", 1},
		{"0.6.0", "0.4.0", 1},
		{"0.4.0", "0.6.0", -1},
		{"1.54.0", "1.55.0", -1},
		{"1.55.0", "1.54.0", 1},
	}

	for _, tt := range tests {
		name := tt.a + "_vs_" + tt.b
		t.Run(name, func(t *testing.T) {
			got := compareVersions(tt.a, tt.b)
			if (got < 0 && tt.want >= 0) || (got > 0 && tt.want <= 0) || (got == 0 && tt.want != 0) {
				t.Errorf("compareVersions(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestExtractVersion(t *testing.T) {
	tests := []struct {
		dirName string
		want    string
	}{
		{"vscjava.vscode-java-dependency-0.27.2-universal", "0.27.2"},
		{"vscjava.vscode-java-dependency-0.27.4-universal", "0.27.4"},
		{"llvm-vs-code-extensions.vscode-clangd-0.6.0-universal", "0.6.0"},
		{"ms-python.python-2026.4.0-universal", "2026.4.0"},
		{"plain-name", ""},
	}

	for _, tt := range tests {
		t.Run(tt.dirName, func(t *testing.T) {
			got := extractVersion(tt.dirName)
			if got != tt.want {
				t.Errorf("extractVersion(%q) = %q, want %q", tt.dirName, got, tt.want)
			}
		})
	}
}

func TestGetMajorVersion(t *testing.T) {
	tests := []struct {
		ver  string
		want string
	}{
		{"21.0.7-tem", "21"},
		{"8.14.4", "8"},
		{"3.9.5", "3"},
		{"abc", ""},
	}

	for _, tt := range tests {
		t.Run(tt.ver, func(t *testing.T) {
			got := getMajorVersion(tt.ver)
			if got != tt.want {
				t.Errorf("getMajorVersion(%q) = %q, want %q", tt.ver, got, tt.want)
			}
		})
	}
}

func TestParseVersionParts(t *testing.T) {
	tests := []struct {
		ver  string
		want []int
	}{
		{"21.0.7-tem", []int{21, 0, 7}},
		{"8.14.4", []int{8, 14, 4}},
		{"0.27.2", []int{0, 27, 2}},
		{"1.2.3", []int{1, 2, 3}},
	}

	for _, tt := range tests {
		t.Run(tt.ver, func(t *testing.T) {
			got := parseVersionParts(tt.ver)
			if len(got) != len(tt.want) {
				t.Fatalf("parseVersionParts(%q) = %v, want %v", tt.ver, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("parseVersionParts(%q) = %v, want %v", tt.ver, got, tt.want)
				}
			}
		})
	}
}
