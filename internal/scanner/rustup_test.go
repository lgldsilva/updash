package scanner

import (
	"errors"
	"testing"

	"github.com/lgldsilva/updash/internal/model"
)

func TestParseCargoInstallUpdate(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  int
		first string
	}{
		{
			name: "header plus outdated and up-to-date crates",
			input: `Package       Installed  Latest  Needs update
			cargo-edit    0.11.0     0.11.1  Yes
			cargo-watch   0.18.0     0.18.0  No
			`,
			want:  2,
			first: "cargo-edit",
		},
		{
			name:  "empty output",
			input: "",
			want:  0,
			first: "",
		},
		{
			name: "crate name containing package substring",
			input: `Package       Installed  Latest  Needs update
			mypackage-utils 1.0.0      1.1.0   Yes
			`,
			want:  1,
			first: "mypackage-utils",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			items := parseCargoInstallUpdate(tt.input)
			if len(items) != tt.want {
				t.Fatalf("got %d items, want %d", len(items), tt.want)
			}
			if tt.want > 0 && items[0].Name != tt.first {
				t.Fatalf("first item name = %q, want %q", items[0].Name, tt.first)
			}
		})
	}
}

func TestParseCargoInstallUpdateLine(t *testing.T) {
	tests := []struct {
		name string
		line string
		want *model.Item
	}{
		{
			name: "outdated crate",
			line: "cargo-edit    0.11.0     0.11.1  Yes",
			want: &model.Item{
				Name:         "cargo-edit",
				Category:     model.CatCargo,
				CurrentVer:   "0.11.0",
				AvailableVer: "0.11.1",
				Status:       model.StatusOutdated,
			},
		},
		{
			name: "up-to-date crate",
			line: "cargo-watch   0.18.0     0.18.0  No",
			want: &model.Item{
				Name:       "cargo-watch",
				Category:   model.CatCargo,
				CurrentVer: "0.18.0",
				Status:     model.StatusOK,
			},
		},
		{
			name: "needs update inferred from version mismatch",
			line: "ripgrep       13.0.0     14.0.0",
			want: &model.Item{
				Name:         "ripgrep",
				Category:     model.CatCargo,
				CurrentVer:   "13.0.0",
				AvailableVer: "14.0.0",
				Status:       model.StatusOutdated,
			},
		},
		{
			name: "malformed line",
			line: "only-two-fields",
			want: nil,
		},
		{
			name: "separator line",
			line: "---",
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseCargoInstallUpdateLine(tt.line)
			if tt.want == nil {
				if got != nil {
					t.Fatalf("expected nil, got %+v", got)
				}
				return
			}
			if got == nil {
				t.Fatalf("expected item, got nil")
			}
			if got.Name != tt.want.Name || got.CurrentVer != tt.want.CurrentVer ||
				got.AvailableVer != tt.want.AvailableVer || got.Status != tt.want.Status {
				t.Fatalf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

// `rustup check` exits 100 when an update is available while printing a valid
// report; the source must be reported as outdated, not errored — an errored
// source blocks every update.
func TestRustupScan_NonZeroExitWithValidOutput(t *testing.T) {
	enableMocks()
	defer disableMocks()
	setMock(binRustup, []string{"check"},
		"stable-x86_64-unknown-linux-gnu - Update available: 1.97.0 -> 1.98.0\nrustup - up to date : 1.29.0",
		errors.New("exit status 100"))

	items, err := (&RustupSource{}).Scan(t.Context(), model.PlatformInfo{})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) == 0 || items[0].Status != model.StatusOutdated {
		t.Fatalf("items = %+v, want an outdated toolchain", items)
	}
}

// A real failure (no parseable output) is still an error.
func TestRustupScan_FailureWithoutOutputIsError(t *testing.T) {
	enableMocks()
	defer disableMocks()
	setMock(binRustup, []string{"check"}, "", errors.New("rustup: command failed"))

	items, err := (&RustupSource{}).Scan(t.Context(), model.PlatformInfo{})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Status != model.StatusError {
		t.Fatalf("items = %+v, want an error item", items)
	}
}

// rustup 1.29 lowercased the phrase ("update available"); older releases wrote
// "Update available". Both must be recognised, and the trailing "up to date :"
// line for rustup itself must not be mistaken for a toolchain update.
func TestParseRustupCheck_CaseInsensitivePhrasing(t *testing.T) {
	out := "stable-x86_64-unknown-linux-gnu - update available: 1.97.0 (2d8144b78) -> 1.98.0 (88d9e12ae)\nrustup - up to date : 1.29.0"
	items := parseRustupCheck(out)
	if len(items) != 2 {
		t.Fatalf("items = %+v, want the toolchain and rustup itself", items)
	}
	if items[0].Status != model.StatusOutdated || items[0].Name != "stable-x86_64-unknown-linux-gnu" {
		t.Fatalf("toolchain not flagged outdated: %+v", items[0])
	}
	if items[1].Status != model.StatusOK {
		t.Fatalf("rustup itself must be OK: %+v", items[1])
	}
	if got := parseRustupCheck("stable - Update available: 1.0 -> 1.1"); len(got) != 1 || got[0].Status != model.StatusOutdated {
		t.Fatalf("legacy capitalised phrasing broke: %+v", got)
	}
}
