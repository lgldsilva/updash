package elevate

import (
	"testing"

	"github.com/lgldsilva/updash/internal/model"
)

func TestCategoryNeedsElevation(t *testing.T) {
	mac := model.PlatformInfo{OS: "darwin", HasYay: false}
	linuxYay := model.PlatformInfo{OS: "linux", HasYay: true}
	linuxPacman := model.PlatformInfo{OS: "linux", HasYay: false}
	win := model.PlatformInfo{OS: "windows"}

	cases := []struct {
		cat  model.Category
		plat model.PlatformInfo
		want bool
	}{
		{model.CatMAS, mac, true},
		{model.CatApt, linuxPacman, true},
		{model.CatPacman, linuxYay, false},
		{model.CatPacman, linuxPacman, true},
		{model.CatBrew, mac, false},
		{model.CatWinget, win, false},
	}

	for _, tc := range cases {
		got := CategoryNeedsElevation(tc.cat, tc.plat)
		if got != tc.want {
			t.Errorf("CategoryNeedsElevation(%s, %s) = %v, want %v",
				tc.cat, tc.plat.OS, got, tc.want)
		}
	}
}

func TestItemNeedsElevation(t *testing.T) {
	apt := &model.Item{Category: model.CatCache, Name: "apt cache"}
	snap := &model.Item{Category: model.CatCache, Name: "snap retention"}
	brew := &model.Item{Category: model.CatCache, Name: "brew cache"}

	if !ItemNeedsElevation(apt) || !ItemNeedsElevation(snap) {
		t.Fatal("apt/snap cleanup should need elevation")
	}
	if ItemNeedsElevation(brew) {
		t.Fatal("brew cleanup should not need elevation")
	}
}
