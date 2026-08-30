package updater

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lgldsilva/updash/internal/model"
)

func TestPlanUpdateCommandsOmz_UsesNonInteractiveZshAndAbsoluteScript(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	originalLookPath := lookPath
	t.Cleanup(func() { lookPath = originalLookPath })
	lookPath = func(name string) (string, error) {
		if name == "zsh" {
			return "/usr/bin/zsh", nil
		}
		return originalLookPath(name)
	}

	plans, err := PlanUpdateCommands(model.CatOmz, []*model.Item{{Name: "omz"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(plans) != 1 {
		t.Fatalf("plans = %#v, want one command", plans)
	}

	wantScript := filepath.Join(home, ".oh-my-zsh", "tools", "upgrade.sh")
	plan := plans[0]
	if plan.Name != "zsh" || len(plan.Args) != 2 || plan.Args[0] != "-f" || plan.Args[1] != wantScript {
		t.Fatalf("OMZ plan = %#v, want zsh -f %q", plan, wantScript)
	}
	if plan.Scope != CommandScopeCategoryGlobal || plan.Elevated {
		t.Fatalf("OMZ plan scope/elevation = %#v, want category-global and non-elevated", plan)
	}
	if !filepath.IsAbs(plan.Args[1]) {
		t.Fatalf("script path is not absolute: %q", plan.Args[1])
	}
	if strings.Contains(strings.Join(append([]string{plan.Name}, plan.Args...), " "), "bash -c") {
		t.Fatalf("OMZ plan must not use bash -c: %#v", plan)
	}
}

func TestPlanUpdateCommandsOmz_FallsBackToManualWithoutZsh(t *testing.T) {
	originalLookPath := lookPath
	t.Cleanup(func() { lookPath = originalLookPath })
	lookPath = func(name string) (string, error) {
		if name == "zsh" {
			return "", errors.New("zsh unavailable")
		}
		return originalLookPath(name)
	}

	plans, err := PlanUpdateCommands(model.CatOmz, []*model.Item{{Name: "omz"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(plans) != 1 || plans[0].Scope != CommandScopeManual || plans[0].Manual == "" {
		t.Fatalf("OMZ fallback plan = %#v, want a manual plan", plans)
	}
}

func TestExecutePreparedBatchOmz_DoesNotLoadZshrcOrRunInteractive(t *testing.T) {
	if _, err := exec.LookPath("zsh"); err != nil {
		t.Skip("zsh is not installed")
	}

	home := t.TempDir()
	t.Setenv("HOME", home)
	marker := filepath.Join(home, "zshrc-loaded")
	t.Setenv("UPDASH_ZSHRC_MARKER", marker)

	if err := os.WriteFile(filepath.Join(home, ".zshrc"), []byte("print -r -- loaded > \"$UPDASH_ZSHRC_MARKER\"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	scriptDir := filepath.Join(home, ".oh-my-zsh", "tools")
	if err := os.MkdirAll(scriptDir, 0700); err != nil {
		t.Fatal(err)
	}
	script := "if [[ -o interactive ]]; then print -u2 interactive; exit 1; fi\nprint -r -- executed\n"
	if err := os.WriteFile(filepath.Join(scriptDir, "upgrade.sh"), []byte(script), 0600); err != nil {
		t.Fatal(err)
	}

	item := &model.Item{Name: "omz", Category: model.CatOmz}
	batch, err := PrepareUpdateBatch(context.Background(), model.CatOmz, []*model.Item{item})
	if err != nil {
		t.Fatal(err)
	}
	results := ExecutePreparedBatch(context.Background(), batch, SilentOptions())
	if len(results) != 1 || !results[0].Success {
		t.Fatalf("OMZ update failed: %#v", results)
	}
	if !strings.Contains(results[0].Output, "executed") {
		t.Fatalf("unexpected output: %q", results[0].Output)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf(".zshrc was loaded; marker stat error = %v", err)
	}
}
