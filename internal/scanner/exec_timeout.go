package scanner

import (
	"context"
	"time"
)

const agentProbeTimeout = 5 * time.Second

// registryLatestTimeout is the budget for registry lookups (`npm view <pkg>
// version`, or a custom latest command). Unlike a local --version probe, these
// pay a full network round trip — an npm view on a healthy link hovers around
// 4s, so the local 5s budget leaves no headroom: one slow round trip reads as
// "verification failed" and an unverified source blocks every update.
const registryLatestTimeout = 15 * time.Second

// infraLatestTimeout is the budget for network-bound freshness probes
// (semidx release check, gcloud components) — larger than local version probes.
const infraLatestTimeout = 10 * time.Second

// execCommandBudget runs a command with a per-invocation timeout (in addition to ctx).
// Delegates to execCommand so tests can mock it uniformly.
func execCommandBudget(ctx context.Context, budget time.Duration, name string, args ...string) ([]byte, error) {
	if budget <= 0 {
		return execCommand(ctx, name, args...)
	}
	cmdCtx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()
	return execCommand(cmdCtx, name, args...)
}
