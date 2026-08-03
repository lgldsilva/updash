package scanner

import (
	"context"
	"os/exec"
	"time"
)

const agentProbeTimeout = 5 * time.Second

// infraLatestTimeout is the budget for network-bound freshness probes
// (semidx release check, gcloud components) — larger than local version probes.
const infraLatestTimeout = 10 * time.Second

// execCommandBudget runs a command with a per-invocation timeout (in addition to ctx).
func execCommandBudget(ctx context.Context, budget time.Duration, name string, args ...string) ([]byte, error) {
	if budget <= 0 {
		return execCommand(ctx, name, args...)
	}
	cmdCtx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()
	cmd := exec.CommandContext(cmdCtx, name, args...)
	return cmd.Output()
}
