package elevate

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"
)

// No test may authenticate against the host's sudo: a wrong password lands
// in auth.log and, with pam_faillock, can lock the account running
// `go test`. TestMain routes commandContext to this test binary acting as a
// fake sudo: `-n` succeeds only when the test sets fakeNopasswdEnv, and
// `-S` accepts exactly fakeSudoPassword on stdin. Anything that is not sudo
// fails like a missing binary.
const (
	fakeExecEnv      = "UPDASH_ELEVATE_FAKE_EXEC"
	fakeNopasswdEnv  = "UPDASH_ELEVATE_FAKE_NOPASSWD"
	fakeSudoPassword = "correct-horse-battery"
)

func TestMain(m *testing.M) {
	if os.Getenv(fakeExecEnv) == "1" {
		os.Exit(runFakeSudo(os.Args[1:]))
	}
	commandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		cmd := exec.CommandContext(ctx, os.Args[0], append([]string{name}, args...)...) //nolint:gosec // test binary re-exec
		cmd.Env = append(os.Environ(), fakeExecEnv+"=1")
		return cmd
	}
	os.Exit(m.Run())
}

func runFakeSudo(argv []string) int {
	if len(argv) == 0 || argv[0] != sudoBin {
		fmt.Fprintf(os.Stderr, "unmocked command in elevate tests: %v\n", argv)
		return 127
	}
	args := argv[1:]
	switch {
	case slices.Contains(args, "-n"):
		if os.Getenv(fakeNopasswdEnv) == "1" {
			return 0
		}
		fmt.Fprintln(os.Stderr, "sudo: a password is required")
		return 1
	case slices.Contains(args, "-S"):
		line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		if strings.TrimSuffix(line, "\n") == fakeSudoPassword {
			return 0
		}
		fmt.Fprintln(os.Stderr, "Sorry, try again.")
		return 1
	}
	return 0
}

// hostSudo configures the fake for a host with or without NOPASSWD sudo.
func hostSudo(t *testing.T, nopasswd bool) {
	t.Helper()
	v := ""
	if nopasswd {
		v = "1"
	}
	t.Setenv(fakeNopasswdEnv, v)
}

// sudoArgv strips the fake-exec test binary from a command built by Sudo.
func sudoArgv(cmd *exec.Cmd) []string { return cmd.Args[1:] }
