package scanner

import (
	"context"
	"os/exec"
	"strings"
)

// execCommand is a variable so tests can replace it with a mock.
var execCommand = func(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	return cmd.Output()
}

// execCommandEnv is execCommand with an explicit environment, for tools that
// need a corrected PATH (see EnsurePnpmPath). Variable so tests can mock it.
var execCommandEnv = func(ctx context.Context, env []string, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = env
	return cmd.Output()
}

// execCombined captures stdout+stderr (for actionable error messages).
// Do NOT use this for commands whose stdout must be parsed as JSON (e.g.
// `npm ... --json`): npm routinely writes "npm warn"/"npm notice" lines to
// stderr (deprecation notices, funding nags, blocked install scripts), and
// merging them into stdout corrupts the JSON — json.Unmarshal then fails
// silently and callers treat the parse error as "nothing outdated". Use
// execCommand (stdout only) for those, and errStderr(err) to still surface
// the stderr text on a hard failure.
var execCombined = func(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	return cmd.CombinedOutput()
}

// errStderr returns the failed command's stderr text when available
// (cmd.Output() populates *exec.ExitError.Stderr), falling back to err.Error().
func errStderr(err error) string {
	if exitErr, ok := err.(*exec.ExitError); ok {
		if msg := strings.TrimSpace(string(exitErr.Stderr)); msg != "" {
			return msg
		}
	}
	return err.Error()
}
