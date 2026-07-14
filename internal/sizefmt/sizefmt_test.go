package sizefmt

import "testing"

func TestParse(t *testing.T) {
	got, err := Parse("15.3MB")
	if err != nil || got == 0 {
		t.Fatalf("Parse(15.3MB) = %d, %v", got, err)
	}
}

func TestParseBrewFreed(t *testing.T) {
	out := "==> This operation has freed approximately 15.3MB of disk space."
	if got := ParseBrewFreed(out); got == 0 {
		t.Fatal("expected brew parse")
	}
}

func TestFormat(t *testing.T) {
	if Format(0) != "0B" {
		t.Fatal("Format(0) should be 0B")
	}
}
