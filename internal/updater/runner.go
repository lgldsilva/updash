package updater

import (
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
// command, runs it, and returns the captured stdout/stderr. Output is always
// buffered and, when a terminal or TUI writer is configured, streamed there
// at the same time. It is a variable so tests can exercise the update flow
// (command dispatch + post-update validation) without executing real
// processes — mirroring the scanner/runner.go seam.
var runUpdateCmd = func(ctx context.Context, opts Options, name string, args ...string) (stdout, stderr string, err error) {
	return captureUpdateCmd(opts, commandContext(ctx, name, args...))
}

// runElevatedUpdateCmd is the equivalent subprocess seam for commands that
// require sudo. It receives the unwrapped command so dry-run and execution
// share the same CommandPlan representation. Output follows the same
// buffer-and-stream policy as runUpdateCmd.
var runElevatedUpdateCmd = func(ctx context.Context, opts Options, name string, args ...string) (stdout, stderr string, err error) {
	return captureUpdateCmd(opts, sudoCommand(ctx, name, args...))
}

// captureUpdateCmd runs cmd with the shared output policy: live stream when
// one is configured, and a full stdout/stderr buffer in every run mode.
func captureUpdateCmd(opts Options, cmd *exec.Cmd) (stdout, stderr string, err error) {
	out, er := bufferCommandOutput(opts, cmd)
	err = runGuarded(cmd, inactivityWindow)
	return out.String(), er.String(), err
}
