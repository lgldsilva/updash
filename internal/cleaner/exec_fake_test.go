package cleaner

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
)

// The cleaner prunes Docker images, uninstalls SDKs and runs sudo. No test may
// do that to the host running `go test`, so TestMain routes every command
// through this test binary acting as a fake: it records argv, prints
// UPDASH_CLEANER_FAKE_STDOUT, and exits 1 when argv contains
// UPDASH_CLEANER_FAKE_FAIL (or the command is literally "false").
const (
	fakeExecEnv   = "UPDASH_CLEANER_FAKE_EXEC"
	fakeStdoutEnv = "UPDASH_CLEANER_FAKE_STDOUT"
	fakeFailEnv   = "UPDASH_CLEANER_FAKE_FAIL"
)

func TestMain(m *testing.M) {
	if os.Getenv(fakeExecEnv) == "1" {
		os.Exit(runFakeCommand(os.Args[1:]))
	}
	// Cleanup removes paths under HOME (~/.npm/_npx, editor extensions,
	// SDKMAN). A test that forgets t.Setenv("HOME") must hit a sandbox.
	home, err := os.MkdirTemp("", "updash-cleaner-home-")
	if err != nil {
		panic(err)
	}
	if err := os.Setenv("HOME", home); err != nil {
		panic(err)
	}
	commandContext = fakeCommandContext
	sudoCommand = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		return fakeCommandContext(ctx, "sudo", append([]string{name}, args...)...)
	}
	code := m.Run()
	_ = os.RemoveAll(home)
	os.Exit(code)
}

func runFakeCommand(argv []string) int {
	fmt.Print(os.Getenv(fakeStdoutEnv))
	joined := strings.Join(argv, " ")
	if fail := os.Getenv(fakeFailEnv); fail != "" && strings.Contains(joined, fail) {
		return 1
	}
	if len(argv) > 0 && argv[0] == "false" {
		return 1
	}
	return 0
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

func (l *execLog) calls() [][]string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([][]string(nil), l.argvs...)
}

func (l *execLog) joined() string {
	var b strings.Builder
	for _, argv := range l.calls() {
		b.WriteString(strings.Join(argv, " "))
		b.WriteByte('\n')
	}
	return b.String()
}
