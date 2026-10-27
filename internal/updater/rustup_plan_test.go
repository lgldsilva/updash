package updater

import (
	"reflect"
	"testing"

	"github.com/lgldsilva/updash/internal/model"
)

func TestPlanUpdateCommands_RustupSelfUpdateAndToolchains(t *testing.T) {
	toolchain := &model.Item{Name: "stable-x86_64-unknown-linux-gnu", Category: model.CatRustup}
	manager := &model.Item{Name: "rustup", Category: model.CatRustup}

	tests := []struct {
		name  string
		items []*model.Item
		want  []CommandPlan
	}{
		{
			name:  "rustup-only",
			items: []*model.Item{manager},
			want: []CommandPlan{
				{Name: "rustup", Args: []string{"self", "update"}, Scope: CommandScopeCategoryGlobal},
			},
		},
		{
			name:  "toolchain-only",
			items: []*model.Item{toolchain},
			want: []CommandPlan{
				{Name: "rustup", Args: []string{"update"}, Scope: CommandScopeCategoryGlobal},
			},
		},
		{
			name:  "both-self-update-first",
			items: []*model.Item{toolchain, manager},
			want: []CommandPlan{
				{Name: "rustup", Args: []string{"self", "update"}, Scope: CommandScopeCategoryGlobal},
				{Name: "rustup", Args: []string{"update"}, Scope: CommandScopeCategoryGlobal},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := PlanUpdateCommands(model.CatRustup, tt.items)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("PlanUpdateCommands() = %#v, want %#v", got, tt.want)
			}
		})
	}
}
