package sizefmt

import "testing"

func TestParse(t *testing.T) {
	mb := float64(1024 * 1024)
	gb := float64(1024 * 1024 * 1024)
	tests := []struct {
		in      string
		want    int64
		wantErr bool
	}{
		{"15.3MB", int64(15.3 * mb), false},
		{"11G", 11 * 1024 * 1024 * 1024, false},
		{"3.838GB", int64(3.838 * gb), false},
		{"1024", 1024, false},
		{"100B", 100, false},
		{"0B", 0, false},
		{"", 0, false},
		{"1KiB", 1024, false},
		{"2MiB", 2 * 1024 * 1024, false},
		{"1T", 1024 * 1024 * 1024 * 1024, false},
		{"  512KB  ", 512 * 1024, false},
		{"not-a-size", 0, true},
		{"1XB", 0, true},
	}
	for _, tt := range tests {
		got, err := Parse(tt.in)
		if tt.wantErr {
			if err == nil {
				t.Errorf("Parse(%q) err=nil, want error", tt.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("Parse(%q) unexpected err: %v", tt.in, err)
			continue
		}
		// Allow small float rounding on fractional MB/GB.
		delta := got - tt.want
		if delta < 0 {
			delta = -delta
		}
		if delta > 1024 {
			t.Errorf("Parse(%q) = %d, want ~%d", tt.in, got, tt.want)
		}
	}
}

func TestParseBrewFreed(t *testing.T) {
	tests := []struct {
		name string
		out  string
		want bool // non-zero
	}{
		{"has freed", "==> This operation has freed approximately 15.3MB of disk space.", true},
		{"would free", "Would free approximately 1.2GB of disk space.", true},
		{"freed", "freed approximately 100KB", true},
		{"empty", "", false},
		{"no match", "nothing useful", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseBrewFreed(tt.out)
			if tt.want && got == 0 {
				t.Fatalf("expected non-zero, got 0")
			}
			if !tt.want && got != 0 {
				t.Fatalf("expected 0, got %d", got)
			}
		})
	}
}

func TestParseDockerFreed(t *testing.T) {
	tests := []struct {
		name string
		out  string
		want bool
	}{
		{"ok", "Total reclaimed space: 1.5GB", true},
		{"case", "total reclaimed space: 200MB", true},
		{"empty", "", false},
		{"no match", "deleted: 0", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseDockerFreed(tt.out)
			if tt.want && got == 0 {
				t.Fatalf("expected non-zero")
			}
			if !tt.want && got != 0 {
				t.Fatalf("expected 0, got %d", got)
			}
		})
	}
}

func TestFormat(t *testing.T) {
	tests := []struct {
		n    int64
		want string
	}{
		{0, "0B"},
		{-1, "0B"},
		{500, "500B"},
		{1024, "1KB"},
		{1536, "1.5KB"},
		{1024 * 1024, "1MB"},
		{100 * 1024 * 1024, "100MB"},
		{1024 * 1024 * 1024, "1GB"},
	}
	for _, tt := range tests {
		if got := Format(tt.n); got != tt.want {
			t.Errorf("Format(%d) = %q, want %q", tt.n, got, tt.want)
		}
	}
}
