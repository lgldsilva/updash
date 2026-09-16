package cleaner

import (
	"reflect"
	"testing"
)

func TestPacmanCleanCmdForItem(t *testing.T) {
	cases := []struct {
		name string
		want []string
	}{
		{"pacman-cache", []string{"paccache", "-rk2"}},
		{"pacman-orphans", []string{"bash", "-c",
			`orphans=$(pacman -Qtdq); if [ -n "$orphans" ]; then echo "$orphans" | pacman -Rns - --noconfirm; else echo "no orphans"; fi`}},
		{"yay-cache", []string{"yay", "-Sc", "--aur", "--noconfirm"}},
		{"brew-cache", nil},
	}
	for _, tc := range cases {
		got := pacmanCleanCmdForItem(tc.name)
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestPacmanCleanCmdForItem_OrphansOnlyTargetsOrphans(t *testing.T) {
	// The orphans branch must win over the broader "pacman" prefix (which
	// would run paccache and never remove the orphans).
	got := pacmanCleanCmdForItem("pacman-orphans")
	if len(got) == 0 || got[0] != "bash" {
		t.Fatalf("orphans must route to the removal script, got %v", got)
	}
}

func TestPacmanCleanNeedsRoot(t *testing.T) {
	if pacmanCleanNeedsRoot("yay-cache") {
		t.Fatal("yay-cache is user-owned and must not be elevated")
	}
	if !pacmanCleanNeedsRoot("pacman-cache") {
		t.Fatal("pacman-cache needs root")
	}
	if !pacmanCleanNeedsRoot("pacman-orphans") {
		t.Fatal("pacman-orphans needs root")
	}
}
