package scanner

import (
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
