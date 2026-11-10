package updater

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/lgldsilva/updash/internal/elevate"
	"github.com/lgldsilva/updash/internal/scanner"
)

// Every subprocess the updater starts is built through these seams so tests can
// route them to a fake instead of the host's package managers (see TestMain).
var (
	commandContext = exec.CommandContext
	sudoCommand    = elevate.Sudo
	// brewOutdatedSet re-reads `brew outdated` after an upgrade.
	brewOutdatedSet = scanner.BrewOutdatedSet
	// rpmToolName picks dnf/dnf5/yum from PATH.
	rpmToolName = scanner.RpmToolName
)

// outputRunner runs a command capturing stdout. Used by health/version probes
// (e.g. `opencode --version`). It is a variable so tests can stub sub-process
// execution without spinning up real CLIs.
var outputRunner = func(ctx context.Context, name string, args ...string) ([]byte, error) {
	return commandContext(ctx, name, args...).Output()
}

// npmPrefixRunner reports npm's global prefix. It is separate from
// outputRunner so tests can verify sudo selection without affecting health probes.
var npmPrefixRunner = func(ctx context.Context) ([]byte, error) {
	return commandContext(ctx, npmCommand, "config", "get", "prefix").Output()
}

// geteuid reports the effective user id. Variable so tests can plan for root
// and non-root hosts without changing identity.
var geteuid = os.Geteuid

// lookPath resolves a binary on PATH. Variable so tests can stub it.
var lookPath = exec.LookPath

// evalSymlinks resolves a path to its target. Variable so tests can stub
// prefix resolution (the Crush planner maps the PATH binary back to the npm
// prefix that owns it).
var evalSymlinks = filepath.EvalSymlinks

// runUpdateCmd is the subprocess primitive for the update path: it builds the
// command, wires buffers (or streams to the terminal when Verbose/Interactive),
// runs it, and returns the captured stdout/stderr. It is a variable so tests
// can exercise the update flow (command dispatch + post-update validation)
// without executing real processes — mirroring the scanner/runner.go seam.
//
// Buffering mirrors the cleaner runCmdWithBuilder: verbose or
// interactive runs attach to the terminal, everything else is captured.
var runUpdateCmd = func(ctx context.Context, opts Options, name string, args ...string) (stdout, stderr string, err error) {
	cmd := commandContext(ctx, name, args...)
	var out, er bytes.Buffer
	if opts.Output != nil || opts.Verbose || opts.Interactive {
		opts.ConfigureCmd(cmd)
	} else {
		cmd.Stdout = &out
		cmd.Stderr = &er
	}
	err = runGuarded(cmd, inactivityWindow)
	return out.String(), er.String(), err
}

// runElevatedUpdateCmd is the equivalent subprocess seam for commands that
// require sudo. It receives the unwrapped command so dry-run and execution
// share the same CommandPlan representation.
var runElevatedUpdateCmd = func(ctx context.Context, opts Options, name string, args ...string) (stdout, stderr string, err error) {
	cmd := sudoCommand(ctx, name, args...)
	var out, er bytes.Buffer
	if opts.Output != nil || opts.Verbose || opts.Interactive {
		opts.ConfigureCmd(cmd)
	} else {
		cmd.Stdout = &out
		cmd.Stderr = &er
	}
	err = runGuarded(cmd, inactivityWindow)
	return out.String(), er.String(), err
}
