package updater

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
)

// The updater runs brew, apt-get, npm and sudo. No test may do that to the
// host running `go test`, so TestMain routes every subprocess built through
// the commandContext/sudoCommand seams to this test binary acting as a fake:
// it records argv and exits 127 like a binary that is not installed. Tests
// that need a successful command stub runUpdateCmd/outputRunner instead.
// lookPath, geteuid, rpmToolName, attachSubprocessSudo and brewOutdatedSet are pinned so planning
// never depends on what the host has installed or who runs the suite.
const fakeExecEnv = "UPDASH_UPDATER_FAKE_EXEC"

func TestMain(m *testing.M) {
	if os.Getenv(fakeExecEnv) == "1" {
		fmt.Fprintf(os.Stderr, "unmocked command in updater tests: %s\n", strings.Join(os.Args[1:], " "))
		os.Exit(127)
	}
	commandContext = fakeCommandContext
	sudoCommand = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		return fakeCommandContext(ctx, "sudo", append([]string{name}, args...)...)
	}
	lookPath = func(file string) (string, error) {
		return "", &exec.Error{Name: file, Err: exec.ErrNotFound}
	}
	geteuid = func() int { return 1000 }
	rpmToolName = func() string { return "dnf" }
	// The real helper probes `sudo -n true` on the host; elevate's own tests
	// cover it (TestAttachSubprocessSudo_*).
	attachSubprocessSudo = func(context.Context, *exec.Cmd) (func(), error) {
		return func() {}, errors.New("sudo attach is not mocked in this test")
	}
	brewOutdatedSet = func(context.Context) (map[string]struct{}, error) {
		return nil, errors.New("brew outdated is not mocked in this test")
	}
	os.Exit(m.Run())
}

type execLog struct {
	mu    sync.Mutex
	argvs [][]string
}

var currentExecLog = &execLog{}

func fakeCommandContext(ctx context.Context, name string, args ...string) *exec.Cmd {
	argv := append([]string{name}, args...)
	currentExecLog.mu.Lock()
	currentExecLog.argvs = append(currentExecLog.argvs, argv)
	currentExecLog.mu.Unlock()
	cmd := exec.CommandContext(ctx, os.Args[0], argv...) //nolint:gosec // test binary re-exec
	cmd.Env = append(os.Environ(), fakeExecEnv+"=1")
	return cmd
}

// recordExec resets the shared exec log for the current test.
func recordExec(t *testing.T) *execLog {
	t.Helper()
	currentExecLog.mu.Lock()
	currentExecLog.argvs = nil
	currentExecLog.mu.Unlock()
	return currentExecLog
}

func (l *execLog) joined() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	var b strings.Builder
	for _, argv := range l.argvs {
		b.WriteString(strings.Join(argv, " "))
		b.WriteByte('\n')
	}
	return b.String()
}

// withRealExec lets one test run real fixture processes (sh, zsh scripts in a
// temp HOME) through the seams. Never use it for a host package manager.
func withRealExec(t *testing.T) {
	t.Helper()
	prevCmd, prevSudo, prevLook := commandContext, sudoCommand, lookPath
	commandContext, lookPath = exec.CommandContext, exec.LookPath
	sudoCommand = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		t.Fatalf("withRealExec does not allow sudo: %s %v", name, args)
		return nil
	}
	t.Cleanup(func() { commandContext, sudoCommand, lookPath = prevCmd, prevSudo, prevLook })
}

func TestFakeExecIsStrict(t *testing.T) {
	log := recordExec(t)
	out, err := outputRunner(context.Background(), "brew", "outdated")
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 127 {
		t.Fatalf("unmocked command must fail like a missing binary: out=%q err=%v", out, err)
	}
	if got := log.joined(); got != "brew outdated\n" {
		t.Fatalf("recorded %q", got)
	}
}
