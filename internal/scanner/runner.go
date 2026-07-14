package scanner

import (
	"bytes"
	"context"
	"os/exec"
)

// execCommand is a variable so tests can replace it with a mock.
// Real implementation: runs the command and returns stdout.
var execCommand = func(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	return cmd.Output()
}

// execCombinedOutput is like execCommand but returns both stdout and stderr.
var execCombinedOutput = func(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	return cmd.CombinedOutput()
}

// execStdoutStderr runs the command and returns stdout and stderr separately.
var execStdoutStderr = func(ctx context.Context, name string, args ...string) (stdout, stderr []byte, err error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err = cmd.Run()
	return outBuf.Bytes(), errBuf.Bytes(), err
}
