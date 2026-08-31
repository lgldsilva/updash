package updater

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/lgldsilva/updash/internal/model"
)

// Two flatpak failures are recoverable by re-running the same update with a
// different flag, and both were hit on a real machine:
//
//   - "Decompressed delta part exceeds configured limit of N bytes" — a broken
//     static delta on the remote; downloading the full objects works.
//   - "Flatpak system operation Deploy not allowed for user" — a system-wide
//     installation being updated without an authorising session (ssh, cron, a
//     desktop without polkit); the same command succeeds elevated.
const (
	flatpakDeltaMarker      = "delta part exceeds configured limit"
	flatpakNotAllowedMarker = "not allowed for user"
	flatpakNoDeltasFlag     = "--no-static-deltas"
	// flatpakMaxRetries bounds the recovery: one attempt per known cause.
	flatpakMaxRetries = 2
)

// executePreparedFlatpak runs the flatpak update and retries it once per known
// recoverable failure, so a broken delta or a missing polkit session does not
// leave every flatpak app outdated.
func executePreparedFlatpak(ctx context.Context, items []*model.Item, plans []CommandPlan, opts Options) []*Result {
	if len(plans) != 1 {
		return executePreparedPlans(ctx, items, plans, opts)
	}

	plan := plans[0]
	result := runCommandPlan(ctx, items[0], plan, opts)
	var output strings.Builder
	output.WriteString(result.Output)

	for attempt := 0; attempt < flatpakMaxRetries && !result.Success; attempt++ {
		next, reason, ok := flatpakRetryPlan(plan, result.Output+result.Error)
		if !ok {
			break
		}
		plan = next
		fmt.Fprintf(&output, "\nupdash: %s — retrying: %s\n", reason, commandText(plan))
		result = runCommandPlan(ctx, items[0], plan, opts)
		output.WriteString(result.Output)
	}

	result.Output = output.String()
	items[0].Log = result.Output
	return batchMarkAll(items, result)
}

// flatpakRetryPlan maps a failed flatpak run to the next plan worth trying.
// Pure: it only inspects the command output.
func flatpakRetryPlan(plan CommandPlan, output string) (CommandPlan, string, bool) {
	lower := strings.ToLower(output)
	switch {
	case strings.Contains(lower, flatpakDeltaMarker) && !slices.Contains(plan.Args, flatpakNoDeltasFlag):
		next := plan
		next.Args = append(append([]string(nil), plan.Args...), flatpakNoDeltasFlag)
		return next, "flathub served a broken static delta", true
	case strings.Contains(lower, flatpakNotAllowedMarker) && !plan.Elevated:
		next := plan
		next.Args = append([]string(nil), plan.Args...)
		next.Elevated = true
		return next, "system-wide install needs elevation (no authorising session)", true
	default:
		return CommandPlan{}, "", false
	}
}

// commandText renders a plan the way the user would type it.
func commandText(plan CommandPlan) string {
	parts := append([]string{plan.Name}, plan.Args...)
	if plan.Elevated {
		parts = append([]string{"sudo"}, parts...)
	}
	return strings.Join(parts, " ")
}
