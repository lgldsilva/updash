package updater

import (
	"bytes"
	"context"
	"os/exec"
)

// outputRunner runs a command capturing stdout. Used by health/version probes
// (e.g. `opencode --version`). It is a variable so tests can stub sub-process
// execution without spinning up real CLIs.
var outputRunner = func(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).Output()
}

// lookPath resolves a binary on PATH. Variable so tests can stub it.
var lookPath = exec.LookPath

// runUpdateCmd is the subprocess primitive for the update path: it builds the
// command, wires buffers (or streams to the terminal when Verbose/Interactive),
// runs it, and returns the captured stdout/stderr. It is a variable so tests
// can exercise the update flow (command dispatch + post-update validation)
// without executing real processes — mirroring the scanner/runner.go seam.
//
// The buffer/stream behaviour matches runCmdWithBuilder exactly: verbose or
// interactive runs attach to the terminal, everything else is captured.
var runUpdateCmd = func(ctx context.Context, opts Options, name string, args ...string) (stdout, stderr string, err error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var out, er bytes.Buffer
	if opts.Verbose || opts.Interactive {
		opts.ConfigureCmd(cmd)
	} else {
		cmd.Stdout = &out
		cmd.Stderr = &er
	}
	err = cmd.Run()
	return out.String(), er.String(), err
}
