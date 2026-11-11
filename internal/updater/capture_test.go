package updater

import (
	"bytes"
	"context"
	"os/exec"
	"strings"
	"testing"

	"github.com/lgldsilva/updash/internal/model"
)

const disabledCaskLine = "Warning: Not upgrading flameshot, it is disabled because it does not pass the macOS Gatekeeper check!"

func shPlan(script string) CommandPlan {
	return CommandPlan{Name: "sh", Args: []string{"-c", script}, Scope: CommandScopeExact}
}

func stubBrewStillOutdated(t *testing.T, name string) {
	t.Helper()
	prev := brewOutdatedSet
	brewOutdatedSet = func(context.Context) (map[string]struct{}, error) {
		return map[string]struct{}{name: {}}, nil
	}
	t.Cleanup(func() { brewOutdatedSet = prev })
}

// stubMASProbes lets a MAS upgrade run without sudo and without a host `mas`.
// The outdated probe is a failing shell so the retry loop stops immediately.
func stubMASProbes(t *testing.T) {
	t.Helper()
	prevAttach := attachSubprocessSudo
	attachSubprocessSudo = func(context.Context, *exec.Cmd) (func(), error) {
		return func() {}, nil
	}
	real := commandContext
	commandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		if name == "mas" {
			return real(ctx, "sh", "-c", "exit 1")
		}
		return real(ctx, name, args...)
	}
	t.Cleanup(func() {
		attachSubprocessSudo = prevAttach
		commandContext = real
	})
}

func runModes(live *bytes.Buffer) []struct {
	name string
	opts Options
} {
	return []struct {
		name string
		opts Options
	}{
		{name: "verbose", opts: Options{Verbose: true}},
		{name: "interactive", opts: Options{Interactive: true}},
		{name: "silent", opts: Options{}},
		{name: "tui", opts: Options{Output: live}},
	}
}

func TestBrewModesKeepDisabledCaskText(t *testing.T) {
	skipWithoutShell(t)
	withRealExec(t)
	stubBrewStillOutdated(t, "flameshot")

	var live bytes.Buffer
	script := "printf '%s\n' '" + disabledCaskLine + "' >&2; exit 1"
	for _, mode := range runModes(&live) {
		t.Run(mode.name, func(t *testing.T) {
			live.Reset()
			item := &model.Item{Name: "flameshot", Category: model.CatBrew}
			res := upgradeOneBrewWithPlan(context.Background(), item, shPlan(script), mode.opts)
			assertDisabledCaskCaptured(t, res, mode.opts.Output != nil, live.String())
		})
	}
}

func TestBrewVerboseExitZeroStillShowsDisabledCask(t *testing.T) {
	skipWithoutShell(t)
	withRealExec(t)
	stubBrewStillOutdated(t, "flameshot")

	script := "printf '%s\n' '" + disabledCaskLine + "' >&2"
	item := &model.Item{Name: "flameshot", Category: model.CatBrew}
	res := upgradeOneBrewWithPlan(context.Background(), item, shPlan(script), Options{Verbose: true})
	if res.Success {
		t.Fatal("disabled cask must not be reported as updated")
	}
	if !strings.Contains(res.Output, disabledCaskLine) || !strings.Contains(res.Error, disabledCaskLine) {
		t.Fatalf("output=%q error=%q", res.Output, res.Error)
	}
	fresh := &model.Item{Name: "flameshot", Category: model.CatBrew, Status: model.StatusOutdated}
	if kind, reason := ClassifyItem(fresh, res); kind != KindManualOnly {
		t.Fatalf("kind = %v (%s), want KindManualOnly", kind, reason)
	}
}

func assertDisabledCaskCaptured(t *testing.T, res *Result, streamed bool, live string) {
	t.Helper()
	if res == nil || res.Success {
		t.Fatalf("result = %+v, want a failed upgrade", res)
	}
	if !strings.Contains(res.Output, "Not upgrading") || !strings.Contains(res.Output, "it is disabled") {
		t.Fatalf("output lost the refusal: %q", res.Output)
	}
	if !strings.Contains(res.Error, "brew upgrade falhou") || !strings.Contains(res.Error, disabledCaskLine) {
		t.Fatalf("failure missing excerpt: %q", res.Error)
	}
	fresh := &model.Item{Name: "flameshot", Category: model.CatBrew, Status: model.StatusOutdated}
	if kind, reason := ClassifyItem(fresh, res); kind != KindManualOnly {
		t.Fatalf("kind = %v (%s), want KindManualOnly", kind, reason)
	}
	if streamed && !strings.Contains(live, disabledCaskLine) {
		t.Fatalf("live stream = %q", live)
	}
}

func TestMASModesKeepFailureExcerpt(t *testing.T) {
	skipWithoutShell(t)
	withRealExec(t)
	stubMASProbes(t)

	var live bytes.Buffer
	const line = "Error: Could not update app"
	script := "printf '%s\n' '" + line + "' >&2; exit 1"
	for _, mode := range runModes(&live) {
		t.Run(mode.name, func(t *testing.T) {
			live.Reset()
			item := &model.Item{Name: "Bitwarden", PackageID: "1352778147", Category: model.CatMAS}
			res := upgradeMASAppWithPlan(context.Background(), item, shPlan(script), mode.opts)
			if res.Success || !strings.Contains(res.Output, line) {
				t.Fatalf("output = %q success=%v", res.Output, res.Success)
			}
			if !strings.Contains(res.Error, "mas update falhou") || !strings.Contains(res.Error, line) {
				t.Fatalf("error = %q", res.Error)
			}
			if mode.opts.Output != nil && !strings.Contains(live.String(), line) {
				t.Fatalf("live = %q", live.String())
			}
		})
	}
}

func TestAgentModesReclassifyCaskCollision(t *testing.T) {
	skipWithoutShell(t)
	withRealExec(t)
	link := caskroomSymlink(t)

	var live bytes.Buffer
	plan := CommandPlan{
		Name:  "sh",
		Args:  []string{"-c", `printf '%s\n' 'npm error code EEXIST' "npm error File exists: $1" >&2; exit 1`, "sh", link},
		Scope: CommandScopeExact,
	}
	for _, mode := range runModes(&live) {
		t.Run(mode.name, func(t *testing.T) {
			live.Reset()
			item := &model.Item{
				Name: "Codex", Category: model.CatAgent, PackageID: "@openai/codex", Status: model.StatusOutdated,
			}
			got := executePreparedAgents(context.Background(), []*model.Item{item}, []CommandPlan{plan}, mode.opts)
			if len(got) != 1 {
				t.Fatalf("results = %d", len(got))
			}
			assertCaskCollision(t, got[0], link, mode.opts.Output != nil, live.String())
		})
	}
}

func assertCaskCollision(t *testing.T, res *Result, link string, streamed bool, live string) {
	t.Helper()
	if res == nil || !strings.Contains(res.Output, "File exists: "+link) {
		t.Fatalf("output = %+v", res)
	}
	if !strings.HasPrefix(res.Error, "⊘ ") {
		t.Fatalf("error = %q, want manual-only marker", res.Error)
	}
	fresh := &model.Item{Name: "Codex", Category: model.CatAgent, PackageID: "@openai/codex", Status: model.StatusOutdated}
	kind, reason := ClassifyItem(fresh, res)
	if kind != KindManualOnly {
		t.Fatalf("kind = %v (%s), want KindManualOnly", kind, reason)
	}
	if !strings.Contains(reason, "brew uninstall --cask codex") || !strings.Contains(reason, "npm rm -g @openai/codex") {
		t.Fatalf("reason = %q", reason)
	}
	if streamed && !strings.Contains(live, "File exists:") {
		t.Fatalf("live = %q", live)
	}
}

func TestElevatedUpdateBuffersWhileStreaming(t *testing.T) {
	skipWithoutShell(t)
	withRealExec(t)
	prev := sudoCommand
	// Redirect the seam at a direct exec so the test never invokes sudo.
	sudoCommand = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		return commandContext(ctx, name, args...)
	}
	t.Cleanup(func() { sudoCommand = prev })

	var live bytes.Buffer
	stdout, stderr, err := runElevatedUpdateCmd(context.Background(), Options{Verbose: true, Output: &live}, "sh", "-c", "printf out; printf err >&2")
	if err != nil {
		t.Fatal(err)
	}
	if stdout != "out" || stderr != "err" {
		t.Fatalf("stdout=%q stderr=%q", stdout, stderr)
	}
	if !strings.Contains(live.String(), "out") || !strings.Contains(live.String(), "err") {
		t.Fatalf("live=%q", live.String())
	}
}
