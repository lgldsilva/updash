package updater

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/lgldsilva/updash/internal/model"
)

func skipWithoutShell(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell not available")
	}
}

// The regression: a command that draws a prompt and then waits forever on a
// stdin it will never get must be killed, not left to burn the batch budget.
func TestRunGuarded_KillsSilentCommand(t *testing.T) {
	skipWithoutShell(t)
	prevDelay := stallWaitDelay
	stallWaitDelay = 100 * time.Millisecond
	t.Cleanup(func() { stallWaitDelay = prevDelay })

	var out bytes.Buffer
	// sleep stands in for the real hang: a prompt drawn on stdout while the
	// child waits on input it can never receive (updash gives it no stdin).
	cmd := exec.Command("sh", "-c", `printf 'Install anyways?'; sleep 30`)
	cmd.Stdout = &out

	start := time.Now()
	err := runGuarded(cmd, 150*time.Millisecond)

	var inactive *inactivityError
	if err == nil || !asInactivity(err, &inactive) {
		t.Fatalf("err = %v, want an inactivity error", err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("watchdog took %s to fire", elapsed)
	}
	if !strings.Contains(inactive.Error(), "interactive prompt") {
		t.Fatalf("error must explain the likely cause: %q", inactive.Error())
	}
	// Output produced before the stall is still forwarded to the caller.
	if !strings.Contains(out.String(), "Install anyways?") {
		t.Fatalf("captured output = %q", out.String())
	}
}

// A slow but talkative command (the normal case for package managers) is never
// killed, however long it runs relative to the window.
func TestRunGuarded_KeepsChattyCommandAlive(t *testing.T) {
	skipWithoutShell(t)
	var out bytes.Buffer
	cmd := exec.Command("sh", "-c", `for i in 1 2 3 4 5 6; do printf 'tick\n'; sleep 0.05; done`)
	cmd.Stdout = &out

	if err := runGuarded(cmd, 200*time.Millisecond); err != nil {
		t.Fatalf("chatty command was killed: %v", err)
	}
	if strings.Count(out.String(), "tick") != 6 {
		t.Fatalf("output lost: %q", out.String())
	}
}

func TestRunGuarded_DisabledWindowRunsPlainly(t *testing.T) {
	skipWithoutShell(t)
	var out bytes.Buffer
	cmd := exec.Command("sh", "-c", "printf done")
	cmd.Stdout = &out
	if err := runGuarded(cmd, 0); err != nil {
		t.Fatalf("err = %v", err)
	}
	if out.String() != "done" {
		t.Fatalf("output = %q", out.String())
	}
}

func TestRunGuarded_PropagatesExitError(t *testing.T) {
	skipWithoutShell(t)
	cmd := exec.Command("sh", "-c", "exit 3")
	err := runGuarded(cmd, time.Minute)
	var exitErr *exec.ExitError
	if err == nil || !asExit(err, &exitErr) || exitErr.ExitCode() != 3 {
		t.Fatalf("err = %v, want exit status 3", err)
	}
}

func TestRunGuarded_StartFailure(t *testing.T) {
	cmd := exec.Command("updash-no-such-binary-xyz")
	if err := runGuarded(cmd, time.Minute); err == nil {
		t.Fatal("expected a start failure")
	}
}

func TestWithNonInteractiveEnv(t *testing.T) {
	got := withNonInteractiveEnv([]string{"PATH=/bin", "CI=0"})
	if !slices.Contains(got, "PATH=/bin") {
		t.Fatalf("existing env dropped: %v", got)
	}
	// A value the caller already set wins; the rest are added.
	if !slices.Contains(got, "CI=0") || slices.Contains(got, "CI=1") {
		t.Fatalf("caller-provided CI overwritten: %v", got)
	}
	for _, want := range []string{"NONINTERACTIVE=1", "DEBIAN_FRONTEND=noninteractive", "NO_COLOR=1"} {
		if !slices.Contains(got, want) {
			t.Fatalf("missing %q in %v", want, got)
		}
	}
	// TERM is never touched: rewriting it breaks brew/pacman progress output.
	for _, kv := range got {
		if strings.HasPrefix(kv, "TERM=") && !slices.Contains(os.Environ(), kv) {
			t.Fatalf("TERM was rewritten: %q", kv)
		}
	}
}

func TestWithNonInteractiveEnv_NilInheritsProcessEnv(t *testing.T) {
	t.Setenv("UPDASH_ENV_MARKER", "1")
	got := withNonInteractiveEnv(nil)
	if !slices.Contains(got, "UPDASH_ENV_MARKER=1") {
		t.Fatalf("process env not inherited: %v", got)
	}
}

// updash never hands a child the terminal's stdin; that is what makes the
// watchdog necessary and must not regress.
func TestPrepareUpdateCmd_LeavesStdinDetached(t *testing.T) {
	cmd := exec.Command("true")
	prepareUpdateCmd(cmd)
	if cmd.Stdin != nil {
		t.Fatalf("Stdin = %v, want nil (/dev/null)", cmd.Stdin)
	}
	prepareUpdateCmd(nil) // must not panic
}

// A stuck agent is reported as an error the user can act on, and does not
// leave the item stuck in "updating".
func TestUpdateAgent_StalledCommandIsReported(t *testing.T) {
	skipWithoutShell(t)
	prevWindow, prevDelay := inactivityWindow, stallWaitDelay
	inactivityWindow, stallWaitDelay = 100*time.Millisecond, 100*time.Millisecond
	t.Cleanup(func() { inactivityWindow, stallWaitDelay = prevWindow, prevDelay })

	item := &model.Item{Name: "Grok", Category: model.CatAgent, Status: model.StatusOutdated}
	plan := CommandPlan{Name: "sh", Args: []string{"-c", "printf 'Install anyways?'; sleep 30"}, Scope: CommandScopeExact}
	res := runCommandPlan(context.Background(), item, plan, SilentOptions())

	if res.Success || item.Status != model.StatusError {
		t.Fatalf("stalled command reported as success: %+v", res)
	}
	if !strings.Contains(res.Error, "interactive prompt") {
		t.Fatalf("error = %q, want the interactive-prompt diagnosis", res.Error)
	}
}

func asInactivity(err error, target **inactivityError) bool {
	e, ok := err.(*inactivityError)
	if ok {
		*target = e
	}
	return ok
}

func asExit(err error, target **exec.ExitError) bool {
	e, ok := err.(*exec.ExitError)
	if ok {
		*target = e
	}
	return ok
}

func TestActivityWriter(t *testing.T) {
	var sink bytes.Buffer
	w := newActivityWriter(&sink)
	w.last = time.Now().Add(-time.Hour)
	if w.idle() < time.Minute {
		t.Fatal("idle must grow while nothing is written")
	}
	if n, err := w.Write([]byte("hello")); err != nil || n != 5 {
		t.Fatalf("Write = %d, %v", n, err)
	}
	if sink.String() != "hello" {
		t.Fatalf("forwarded %q", sink.String())
	}
	if w.idle() > time.Second {
		t.Fatal("a write must reset the idle timer")
	}
	if n, err := newActivityWriter(nil).Write([]byte("dropped")); err != nil || n != 7 {
		t.Fatalf("nil destination must swallow writes: %d, %v", n, err)
	}
}

func TestTickForAndCommandLine(t *testing.T) {
	if got := tickFor(time.Minute); got != 15*time.Second {
		t.Fatalf("tickFor(1m) = %s, want 15s", got)
	}
	if got := tickFor(time.Millisecond); got != 10*time.Millisecond {
		t.Fatalf("tickFor clamps to a floor, got %s", got)
	}
	if got := commandLine(exec.Command("echo", "hi")); !strings.HasSuffix(got, "echo hi") {
		t.Fatalf("commandLine = %q", got)
	}
	if got := commandLine(nil); got != "" {
		t.Fatalf("commandLine(nil) = %q", got)
	}
}
