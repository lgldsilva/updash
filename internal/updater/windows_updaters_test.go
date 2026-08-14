package updater

import (
	"testing"

	"github.com/lgldsilva/updash/internal/model"
)

func TestWingetUpgradeArgs(t *testing.T) {
	tests := []struct {
		name  string
		items []*model.Item
		want  []string
	}{
		{
			name:  "empty falls back to --all",
			items: nil,
			want:  []string{"--all"},
		},
		{
			name: "uses PackageID when present",
			items: []*model.Item{
				{Name: "Git", PackageID: "Git.Git"},
			},
			want: []string{"--exact", "--id", "Git.Git"},
		},
		{
			name: "falls back to Name when PackageID empty",
			items: []*model.Item{
				{Name: "Notepad++"},
			},
			want: []string{"--exact", "--id", "Notepad++"},
		},
		{
			name: "multiple items",
			items: []*model.Item{
				{Name: "a", PackageID: "A.A"},
				{Name: "b"},
			},
			want: []string{"--exact", "--id", "A.A", "--exact", "--id", "b"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := wingetUpgradeArgs(tt.items)
			if len(got) != len(tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("arg[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestChocoPackageNames(t *testing.T) {
	tests := []struct {
		name  string
		items []*model.Item
		want  []string
	}{
		{name: "empty", items: nil, want: []string{"all"}},
		{name: "by name", items: []*model.Item{{Name: "git"}}, want: []string{"git"}},
		{name: "by PackageID", items: []*model.Item{{Name: "x", PackageID: "y"}}, want: []string{"y"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := chocoPackageNames(tt.items)
			if len(got) != len(tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("arg[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestScoopPackageNames(t *testing.T) {
	tests := []struct {
		name  string
		items []*model.Item
		want  []string
	}{
		{name: "empty", items: nil, want: []string{"*"}},
		{name: "by name", items: []*model.Item{{Name: "git"}}, want: []string{"git"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := scoopPackageNames(tt.items)
			if len(got) != len(tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("arg[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}
