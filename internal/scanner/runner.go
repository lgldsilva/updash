package scanner

import (
	"context"
	"errors"
	"os/exec"
	"runtime"
	"strings"
	"syscall"
)

// execCommand is a variable so tests can replace it with a mock.
var execCommand = func(ctx context.Context, name string, args ...string) ([]byte, error) {
	return spawn(ctx, nil, name, args, false)
}

// execCommandEnv is execCommand with an explicit environment, for tools that
// need a corrected PATH (see EnsurePnpmPath). Variable so tests can mock it.
var execCommandEnv = func(ctx context.Context, env []string, name string, args ...string) ([]byte, error) {
	return spawn(ctx, env, name, args, true)
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
	return spawn(ctx, nil, name, args, true)
}

// spawn runs the command, retrying through the POSIX shell when the kernel
// refuses the target with ENOEXEC. A shebang-less script is not a machine
// binary, so execve rejects it outright — pnpm ships exactly that between
// `npm install -g pnpm` and its install script swapping in the native binary
// (an install blocked by script allow-lists leaves the placeholder in place).
// POSIX shells and glibc's execvp retry such a file under sh; Go does not, and
// without the retry the source reports an error instead of probing the tool.
// Windows has no sh (and pnpm never ships the placeholder there), so the
// retry is unix-only.
func spawn(ctx context.Context, env []string, name string, args []string, combine bool) ([]byte, error) {
	out, err := runCmd(ctx, env, name, args, combine)
	if err == nil || runtime.GOOS == "windows" || !errors.Is(err, syscall.ENOEXEC) {
		return out, err
	}
	// Hand the script a real path: placeholder-style scripts derive their own
	// directory from $0 (pnpm's walks symlinks from it). The retry reuses the
	// probe's own fixed name and argument list — no new input, no quoting.
	target := name
	if abs, lookErr := exec.LookPath(name); lookErr == nil {
		target = abs
	}
	shArgs := make([]string, len(args)+1)
	shArgs[0] = target
	copy(shArgs[1:], args)
	return runCmd(ctx, env, binSh, shArgs, combine)
}

func runCmd(ctx context.Context, env []string, name string, args []string, combine bool) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = env
	if combine {
		return cmd.CombinedOutput()
	}
	return cmd.Output()
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
