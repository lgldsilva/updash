package updater

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/lgldsilva/updash/internal/elevate"
	"github.com/lgldsilva/updash/internal/model"
)

func TestUsePrivilegedAskpass_passwordlessSkipsAskpass(t *testing.T) {
	sess := elevate.NewSession()
	sess.SetPasswordless()
	ctx := elevate.WithSession(context.Background(), sess)
	cmd := exec.Command("brew", "upgrade", "--greedy", "microsoft-office")
	cleanup, err := usePrivilegedAskpass(ctx, cmd, true)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	if path := sudoAskpassFrom(cmd.Env); path != "" {
		t.Fatalf("NOPASSWD session created SUDO_ASKPASS=%s", path)
	}
}

func TestUsePrivilegedAskpass_plainBrewDoesNotAttach(t *testing.T) {
	prev := attachSubprocessSudo
	attachSubprocessSudo = func(context.Context, *exec.Cmd) (func(), error) {
		t.Fatal("packages that do not need sudo must not install askpass")
		return nil, nil
	}
	t.Cleanup(func() { attachSubprocessSudo = prev })

	cleanup, err := usePrivilegedAskpass(context.Background(), exec.Command("brew"), false)
	if err != nil {
		t.Fatal(err)
	}
	cleanup()
}

func TestInteractivePrivilegedUpdatesInstallAskpass(t *testing.T) {
	// The seam stands in for elevate.AttachSubprocessSudo so the test does not
	// exec brew/mas. TestInstallAskpass covers the real SUDO_ASKPASS helper.
	var commands []*exec.Cmd
	prev := attachSubprocessSudo
	attachSubprocessSudo = func(_ context.Context, cmd *exec.Cmd) (func(), error) {
		cmd.Env = append(os.Environ(), "SUDO_ASKPASS=/tmp/updash-askpass")
		commands = append(commands, cmd)
		return func() {}, errors.New("stop before exec")
	}
	t.Cleanup(func() { attachSubprocessSudo = prev })

	opts := Options{Interactive: true}
	brewItem := &model.Item{Name: "microsoft-office", Category: model.CatBrew}
	brewRes := upgradeOneBrewWithPlan(context.Background(), brewItem, CommandPlan{
		Name:  "brew",
		Args:  []string{"upgrade", "--greedy", "microsoft-office"},
		Scope: CommandScopeExact,
	}, opts)
	if brewRes.Success || !strings.Contains(brewRes.Error, "stop before exec") {
		t.Fatalf("brew result: %+v", brewRes)
	}

	masItem := &model.Item{Name: "Bitwarden", PackageID: "1352778147", Category: model.CatMAS}
	masRes := upgradeMASAppWithPlan(context.Background(), masItem, CommandPlan{
		Name:  "mas",
		Args:  []string{"update", "1352778147"},
		Scope: CommandScopeExact,
	}, opts)
	if masRes.Success || !strings.Contains(masRes.Error, "stop before exec") {
		t.Fatalf("mas result: %+v", masRes)
	}

	if len(commands) != 2 {
		t.Fatalf("askpass calls = %d", len(commands))
	}
	assertAskpassCommand(t, commands[0], "brew upgrade --greedy microsoft-office")
	assertAskpassCommand(t, commands[1], "mas update 1352778147")
}

func assertAskpassCommand(t *testing.T, cmd *exec.Cmd, want string) {
	t.Helper()
	got := strings.Join(cmd.Args, " ")
	if got != want {
		t.Fatalf("command = %q, want %q", got, want)
	}
	if sudoAskpassFrom(cmd.Env) == "" {
		t.Fatalf("%s left the command without SUDO_ASKPASS", want)
	}
}

func sudoAskpassFrom(env []string) string {
	for _, entry := range env {
		if strings.HasPrefix(entry, "SUDO_ASKPASS=") {
			return strings.TrimPrefix(entry, "SUDO_ASKPASS=")
		}
	}
	return ""
}
