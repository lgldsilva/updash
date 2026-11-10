package elevate

import (
	"context"
	"os/exec"
	"strings"
)

// commandContext builds every subprocess this package starts (sudo,
// osascript). Tests swap it for a fake sudo so no test ever authenticates
// against the host (see TestMain).
var commandContext = exec.CommandContext

// Sudo builds a sudo command for name+args, feeding the cached password when
// a validated session is present in ctx. Without a session it runs plain sudo
// (may prompt on a real TTY in headless mode).
func Sudo(ctx context.Context, name string, args ...string) *exec.Cmd {
	sess := FromContext(ctx)
	sudoArgs := append([]string{name}, args...)

	if sess != nil && sess.passwordless {
		return commandContext(ctx, sudoBin, sudoArgs...)
	}
	if sess != nil && sess.valid {
		cmd := commandContext(ctx, sudoBin, append([]string{"-S", "-p", ""}, sudoArgs...)...)
		cmd.Stdin = strings.NewReader(sess.password + "\n")
		return cmd
	}

	return commandContext(ctx, sudoBin, sudoArgs...)
}
