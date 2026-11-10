package updater

import (
	"context"
	"testing"

	"github.com/lgldsilva/updash/internal/model"
)

// apk needs root to lock its database. The elevation layer primes sudo for the
// apk category, so a non-root plan must actually run under sudo; root (common
// in Alpine containers without sudo installed) must not.
func TestPlanUpdateCommands_ApkElevatesOnlyWhenNotRoot(t *testing.T) {
	for _, tc := range []struct {
		name     string
		euid     int
		elevated bool
	}{
		{"non-root", 1000, true},
		{"root", 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			prev := geteuid
			t.Cleanup(func() { geteuid = prev })
			geteuid = func() int { return tc.euid }

			plans, err := planUpdateCommands(context.Background(), model.CatApk, []*model.Item{{Name: "apk-tools"}})
			if err != nil || len(plans) != 1 {
				t.Fatalf("plans=%v err=%v", plans, err)
			}
			if plans[0].Name != "apk" || plans[0].Elevated != tc.elevated {
				t.Fatalf("plan = %+v, want apk with Elevated=%v", plans[0], tc.elevated)
			}
		})
	}
}

// Root needs no sudo for any category, and Debian/Fedora/Arch containers that
// run as root often lack sudo entirely ("sudo: not found").
func TestPlanUpdateCommands_RootNeverPlansSudo(t *testing.T) {
	prev := geteuid
	t.Cleanup(func() { geteuid = prev })
	geteuid = func() int { return 0 }
	for _, cat := range []model.Category{model.CatApt, model.CatDnf, model.CatZypper, model.CatPacman, model.CatSnap, model.CatApk} {
		plans, err := planUpdateCommands(context.Background(), cat, []*model.Item{{Name: "pkg"}})
		if err != nil || len(plans) == 0 {
			t.Fatalf("%s: plans=%v err=%v", cat, plans, err)
		}
		if PlansRequireElevation(plans) {
			t.Errorf("%s as root plans sudo: %+v", cat, plans)
		}
	}
}
