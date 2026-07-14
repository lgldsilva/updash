package scanner

import (
	"context"
	"os/exec"
)

// execCommand is a variable so tests can replace it with a mock.
var execCommand = func(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	return cmd.Output()
}

// execCombined captures stdout+stderr (for actionable error messages).
var execCombined = func(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	return cmd.CombinedOutput()
}
