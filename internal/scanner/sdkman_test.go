package scanner

import (
	"testing"
)

func TestGetMajorVersion(t *testing.T) {
	tests := []struct {
		version string
		want    string
	}{
		{"11.0.25-tem", "11"},
		{"17.0.13-tem", "17"},
		{"21.0.7-tem", "21"},
		{"25.0.2-tem", "25"},
		{"8.11.1", "8"},
		{"9.4.1", "9"},
		{"3.9.5", "3"},
		{"1.10.13", "1"},
		{"invalid", ""},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			if got := getMajorVersion(tt.version); got != tt.want {
				t.Errorf("getMajorVersion(%q) = %q, want %q", tt.version, got, tt.want)
			}
		})
	}
}

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"1.0.0", "2.0.0", -1},
		{"2.0.0", "1.0.0", 1},
		{"1.0.0", "1.0.0", 0},
		{"21.0.7", "21.0.5", 1},
		{"21.0.5", "21.0.7", -1},
		{"8.14.4", "8.14.1", 1},
		{"8.14.1", "8.14.4", -1},
		{"8.14.4", "9.4.1", -1},
		{"9.4.1", "8.14.4", 1},
		{"11.0.25-tem", "11.0.24-tem", 1},
		{"3.9.5", "3.9.5", 0},
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

func TestParseVersionParts(t *testing.T) {
	tests := []struct {
		version string
		want    []int
	}{
		{"21.0.7-tem", []int{21, 0, 7}},
		{"21.0.7", []int{21, 0, 7}},
		{"8.14.4", []int{8, 14, 4}},
		{"3.9.5", []int{3, 9, 5}},
		{"1.10.13", []int{1, 10, 13}},
	}

	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			got := parseVersionParts(tt.version)
			if len(got) != len(tt.want) {
				t.Fatalf("parseVersionParts(%q) = %v, want %v", tt.version, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("parseVersionParts(%q) = %v, want %v", tt.version, got, tt.want)
				}
			}
		})
	}
}

func TestGroupLatestPerMajor(t *testing.T) {
	tests := []struct {
		name     string
		versions []string
		want     map[string]string // major -> latest version
	}{
		{
			name:     "java_multiple",
			versions: []string{"11.0.25-tem", "17.0.13-tem", "21.0.5-tem", "21.0.6-tem", "21.0.7-tem", "25.0.2-tem"},
			want:     map[string]string{"11": "11.0.25-tem", "17": "17.0.13-tem", "21": "21.0.7-tem", "25": "25.0.2-tem"},
		},
		{
			name:     "gradle_multiple",
			versions: []string{"8.11.1", "8.13", "8.14", "8.14.1", "8.14.4", "9.4.1"},
			want:     map[string]string{"8": "8.14.4", "9": "9.4.1"},
		},
		{
			name:     "single_per_major",
			versions: []string{"3.9.5", "1.10.13"},
			want:     map[string]string{"3": "3.9.5", "1": "1.10.13"},
		},
		{
			name:     "empty",
			versions: []string{},
			want:     map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := groupLatestPerMajor(tt.versions)
			if len(got) != len(tt.want) {
				t.Fatalf("groupLatestPerMajor(%v) = %v, want %v", tt.versions, got, tt.want)
			}
			for major, expected := range tt.want {
				if got[major] != expected {
					t.Errorf("groupLatestPerMajor(%v)[%s] = %q, want %q", tt.versions, major, got[major], expected)
				}
			}
		})
	}
}
