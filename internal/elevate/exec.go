package elevate

import (
	"context"
	"os/exec"
	"strings"
)

// Sudo builds a sudo command for name+args, feeding the cached password when
// a validated session is present in ctx. Without a session it runs plain sudo
// (may prompt on a real TTY in headless mode).
func Sudo(ctx context.Context, name string, args ...string) *exec.Cmd {
	sess := FromContext(ctx)
	sudoArgs := append([]string{name}, args...)

	if sess != nil && sess.passwordless {
		return exec.CommandContext(ctx, "sudo", sudoArgs...)
	}
	if sess != nil && sess.valid {
		cmd := exec.CommandContext(ctx, "sudo", append([]string{"-S", "-p", ""}, sudoArgs...)...)
		cmd.Stdin = strings.NewReader(sess.password + "\n")
		return cmd
	}

	return exec.CommandContext(ctx, "sudo", sudoArgs...)
}
