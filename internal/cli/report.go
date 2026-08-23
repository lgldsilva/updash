package cli

import (
	"fmt"
	"strings"

	"github.com/lgldsilva/updash/internal/model"
	"github.com/lgldsilva/updash/internal/updater"
)

type verifyStats struct {
	updated   int
	skipped   int
	manual    int
	failed    int
	remaining int
}

// PrintCheck renders scan results; returns outdated and cleanable counts.
func printCheckEnhanced(updates, cleanup []*model.SourceSummary) (outdated, cleanable, needsSudo, manualOnly int) {
	fmt.Println("\n📦 Updates:")
	for _, s := range updates {
		o, ns, mo := printUpdateSummary(s)
		outdated += o
		needsSudo += ns
		manualOnly += mo
	}

	fmt.Println("\n🧹 Cleanup:")
	for _, s := range cleanup {
		cleanable += printCleanupSummary(s)
	}

	printCheckFooterWithTruth(outdated, cleanable, needsSudo, manualOnly, hasNonAffirmative(updates, cleanup))
	return outdated, cleanable, needsSudo, manualOnly
}

func hasNonAffirmative(groups ...[]*model.SourceSummary) bool {
	for _, summaries := range groups {
		for _, s := range summaries {
			if s != nil {
				for _, it := range s.Items {
					if it != nil && (it.Status == model.StatusError || it.Status == model.StatusUnverified || it.Status == model.StatusInfo) {
						return true
					}
				}
			}
		}
	}
	return false
}

func printUpdateSummary(s *model.SourceSummary) (outdated, needsSudo, manualOnly int) {
	if s.Category == model.CatAgent || s.Category == model.CatOpenCodePlugins {
		return printAgentSummary(s)
	}
	if s.Outdated == 0 {
		printSourceTruth(s)
		return 0, 0, 0
	}
	fmt.Printf("  %s %s: %d outdated\n", s.Icon, s.Label, s.Outdated)
	for _, it := range s.Items {
		if it.Status != model.StatusOutdated {
			continue
		}
		printOutdatedLine(it)
		outdated++
		countScanHints(it, &needsSudo, &manualOnly)
	}
	return outdated, needsSudo, manualOnly
}

func printSourceTruth(s *model.SourceSummary) {
	for _, it := range s.Items {
		if it == nil {
			continue
		}
		switch it.Status {
		case model.StatusError:
			fmt.Printf("  ✘ %s %s: %s\n", s.Icon, s.Label, it.CurrentVer)
		case model.StatusUnverified:
			fmt.Printf("  ? %s %s: %s\n", s.Icon, s.Label, it.CurrentVer)
		case model.StatusInfo:
			fmt.Printf("  ℹ %s %s: freshness not verified\n", s.Icon, it.Name)
		}
	}
}

func printAgentSummary(s *model.SourceSummary) (outdated, needsSudo, manualOnly int) {
	agentsOut := 0
	for _, it := range s.Items {
		if it.Status == model.StatusOutdated {
			agentsOut++
		}
	}
	fmt.Printf("  %s %s: %d installed (%d outdated)\n", s.Icon, s.Label, len(s.Items), agentsOut)
	for _, it := range s.Items {
		if it.Status == model.StatusOutdated {
			printOutdatedLine(it)
			outdated++
			countScanHints(it, &needsSudo, &manualOnly)
		} else if it.CurrentVer != "" {
			switch it.Status {
			case model.StatusOK:
				fmt.Printf("    ✓ %s  %s\n", it.Name, it.CurrentVer)
			case model.StatusInfo:
				fmt.Printf("    ℹ %s  %s (freshness not verified)\n", it.Name, it.CurrentVer)
			case model.StatusUnverified:
				fmt.Printf("    ? %s  %s (verification failed)\n", it.Name, it.CurrentVer)
			case model.StatusError:
				fmt.Printf("    ✘ %s  %s\n", it.Name, it.CurrentVer)
			}
		}
	}
	return outdated, needsSudo, manualOnly
}

func printCleanupSummary(s *model.SourceSummary) int {
	count := 0
	for _, it := range s.Items {
		if it.Status == model.StatusCleanCandidate {
			count++
		}
	}
	if count == 0 {
		return 0
	}
	reclaim := s.Reclaimable
	if reclaim == "" {
		reclaim = fmt.Sprintf("%d item(s)", count)
	}
	fmt.Printf("  %s %s: %s\n", s.Icon, s.Label, reclaim)
	return count
}

func printCheckFooter(outdated, cleanable, needsSudo, manualOnly int) {
	printCheckFooterWithTruth(outdated, cleanable, needsSudo, manualOnly, false)
}

func printCheckFooterWithTruth(outdated, cleanable, needsSudo, manualOnly int, nonAffirmative bool) {
	if outdated == 0 && cleanable == 0 {
		if nonAffirmative {
			fmt.Println("\n⚠ No pending updates found, but some sources were not affirmatively verified.")
		} else {
			fmt.Println("\n✓ Everything is up to date!")
		}
		return
	}
	fmt.Printf("\n%d outdated", outdated)
	if needsSudo > 0 {
		fmt.Printf(" · %d need sudo", needsSudo)
	}
	if manualOnly > 0 {
		fmt.Printf(" · %d manual-only", manualOnly)
	}
	if cleanable > 0 {
		fmt.Printf(" · %d cleanable", cleanable)
	}
	fmt.Println()
}

func printOutdatedLine(it *model.Item) {
	extra := ""
	if it.KeepPolicy != "" {
		extra = fmt.Sprintf("  (%s)", it.KeepPolicy)
	}
	cur := it.CurrentVer
	if cur == "" {
		cur = "?"
	}
	avail := it.AvailableVer
	if avail == "" {
		avail = "newer"
	}
	fmt.Printf("    • %s  %s → %s%s\n", it.Name, cur, avail, extra)
}

func countScanHints(it *model.Item, needsSudo, manualOnly *int) {
	kind, _ := updater.ClassifyItem(it, nil)
	switch kind {
	case updater.KindNeedsPassword:
		*needsSudo++
	case updater.KindManualOnly:
		*manualOnly++
	}
}

// PrintVerifyReport summarizes update results and remaining outdated items.
func PrintVerifyReport(
	updates []*model.SourceSummary,
	results []*updater.Result,
	ok, fail, skipped int,
) verifyStats {
	resultByItem := indexResults(results)
	stats := verifyStats{updated: ok, skipped: skipped, failed: fail}

	printVerifyHeader(ok, fail, skipped)

	needPass, manual, failed, other := classifyRemaining(updates, resultByItem, &stats)
	if stats.remaining == 0 {
		fmt.Println("\n✓ Verified — nothing outdated remains")
		return stats
	}
	stats.manual = len(manual)

	fmt.Printf("\n⚠ %d item(s) still outdated:\n", stats.remaining)
	printVerifyGroup("Need password / Terminal", needPass, resultByItem)
	printVerifyGroup("Manual update only", manual, resultByItem)
	printVerifyGroup("Failed", failed, resultByItem)
	printVerifyGroup("Others", other, resultByItem)
	return stats
}

// itemKey identifies an item across two independent scan passes, whose
// *model.Item pointers never match (each Scan() allocates fresh objects).
type itemKey struct {
	category model.Category
	name     string
}

func keyOf(it *model.Item) itemKey {
	return itemKey{category: it.Category, name: it.Name}
}

func indexResults(results []*updater.Result) map[itemKey]*updater.Result {
	m := make(map[itemKey]*updater.Result, len(results))
	for _, r := range results {
		if r != nil && r.Item != nil {
			m[keyOf(r.Item)] = r
		}
	}
	return m
}

func printVerifyHeader(ok, fail, skipped int) {
	fmt.Println("\n📋 Report:")
	fmt.Printf("  ✓ %d updated", ok)
	if skipped > 0 {
		fmt.Printf(" · ⊘ %d skipped (password/cancelled)", skipped)
	}
	if fail > 0 {
		fmt.Printf(" · ✘ %d failed", fail)
	}
	fmt.Println()
}

func classifyRemaining(
	updates []*model.SourceSummary,
	resultByItem map[itemKey]*updater.Result,
	stats *verifyStats,
) (needPass, manual, failed, other []*model.Item) {
	for _, s := range updates {
		for _, it := range s.Items {
			if it.Status != model.StatusOutdated {
				continue
			}
			stats.remaining++
			kind, _ := updater.ClassifyItem(it, resultByItem[keyOf(it)])
			switch kind {
			case updater.KindNeedsPassword:
				needPass = append(needPass, it)
			case updater.KindManualOnly:
				manual = append(manual, it)
			case updater.KindFailed:
				failed = append(failed, it)
			default:
				other = append(other, it)
			}
		}
	}
	return needPass, manual, failed, other
}

func printVerifyGroup(title string, items []*model.Item, results map[itemKey]*updater.Result) {
	if len(items) == 0 {
		return
	}
	fmt.Printf("\n  %s:\n", title)
	for _, it := range items {
		res := results[keyOf(it)]
		_, reason := updater.ClassifyItem(it, res)
		if reason == "" && res != nil {
			reason = res.Error
		}
		if reason == "" && it.KeepPolicy != "" {
			reason = it.KeepPolicy
		}
		cmd := updater.SuggestCommand(it)
		line := fmt.Sprintf("    • %s", it.Name)
		if reason != "" {
			line += " — " + reason
		}
		fmt.Println(line)
		if cmd != "" {
			fmt.Printf("      → %s\n", cmd)
		}
	}
}

// shouldFailExit returns whether CLI should exit non-zero.
func shouldFailExit(cfg Config, stats verifyStats) bool {
	if cfg.Strict && stats.remaining > 0 {
		return true
	}
	return stats.failed > 0
}

func isSkippedResult(r *updater.Result) bool {
	return r != nil && strings.HasPrefix(r.Error, "⊘ ")
}
