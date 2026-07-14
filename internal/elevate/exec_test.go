package elevate

import (
	"context"
	"strings"
	"testing"

	"github.com/lgldsilva/updash/internal/model"
)

func TestSudo_WithSession(t *testing.T) {
	sess := NewSession()
	sess.password = "secret"
	sess.valid = true
	ctx := WithSession(context.Background(), sess)

	cmd := Sudo(ctx, "mas", "update", "123")
	if cmd.Args[0] != "sudo" || cmd.Args[1] != "-S" {
		t.Fatalf("args: %v", cmd.Args)
	}
	if !strings.Contains(strings.Join(cmd.Args, " "), "mas") {
		t.Fatalf("expected mas in args: %v", cmd.Args)
	}
}

func TestSudo_Passwordless(t *testing.T) {
	sess := NewSession()
	sess.SetPasswordless()
	ctx := WithSession(context.Background(), sess)
	cmd := Sudo(ctx, "apt-get", "update")
	if len(cmd.Args) < 2 || cmd.Args[1] != "apt-get" {
		t.Fatalf("passwordless sudo args: %v", cmd.Args)
	}
}

func TestItemsNeedElevation(t *testing.T) {
	mac := model.PlatformInfo{OS: "darwin"}
	items := []*model.Item{{Category: model.CatBrew}}
	if ItemsNeedElevation(items, mac, false) {
		t.Fatal("brew should not need elevation")
	}
	items = []*model.Item{{Category: model.CatMAS}}
	if !ItemsNeedElevation(items, mac, false) {
		t.Fatal("mas should need elevation")
	}
}
