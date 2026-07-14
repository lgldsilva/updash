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
func printCheckEnhanced(updates, cleanup []*model.SourceSummary) (outdated, cleanable, needsPassword, manualOnly int) {
	fmt.Println("\n📦 Updates:")
	for _, s := range updates {
		if s.Category == model.CatAgent {
			fmt.Printf("  %s %s:", s.Icon, s.Label)
			agentsOut := 0
			for _, it := range s.Items {
				if it.Status == model.StatusOutdated {
					agentsOut++
				}
			}
			fmt.Printf(" %d installed (%d outdated)\n", len(s.Items), agentsOut)
			for _, it := range s.Items {
				if it.Status == model.StatusOutdated {
					printOutdatedLine(it)
					outdated++
					countScanHints(it, &needsPassword, &manualOnly)
				} else if it.CurrentVer != "" {
					fmt.Printf("    ✓ %s  %s\n", it.Name, it.CurrentVer)
				}
			}
			continue
		}

		if s.Outdated > 0 {
			fmt.Printf("  %s %s: %d outdated\n", s.Icon, s.Label, s.Outdated)
			for _, it := range s.Items {
				if it.Status == model.StatusOutdated {
					printOutdatedLine(it)
					outdated++
					countScanHints(it, &needsPassword, &manualOnly)
				}
			}
		}
	}

	fmt.Println("\n🧹 Cleanup:")
	for _, s := range cleanup {
		count := 0
		for _, it := range s.Items {
			if it.Status == model.StatusCleanCandidate {
				count++
			}
		}
		if count > 0 {
			reclaim := s.Reclaimable
			if reclaim == "" {
				reclaim = fmt.Sprintf("%d item(s)", count)
			}
			fmt.Printf("  %s %s: %s\n", s.Icon, s.Label, reclaim)
			cleanable += count
		}
	}

	if outdated == 0 && cleanable == 0 {
		fmt.Println("\n✓ Everything is up to date!")
	} else {
		fmt.Printf("\n%d outdated", outdated)
		if needsPassword > 0 {
			fmt.Printf(" · %d precisam senha", needsPassword)
		}
		if manualOnly > 0 {
			fmt.Printf(" · %d só manual", manualOnly)
		}
		if cleanable > 0 {
			fmt.Printf(" · %d cleanable", cleanable)
		}
		fmt.Println()
	}
	return outdated, cleanable, needsPassword, manualOnly
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

func countScanHints(it *model.Item, needsPassword, manualOnly *int) {
	kind, _ := updater.ClassifyItem(it, nil)
	switch kind {
	case updater.KindNeedsPassword:
		*needsPassword++
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
	resultByItem := make(map[*model.Item]*updater.Result, len(results))
	for _, r := range results {
		if r != nil && r.Item != nil {
			resultByItem[r.Item] = r
		}
	}

	stats := verifyStats{updated: ok, skipped: skipped, failed: fail}

	fmt.Println("\n📋 Relatório:")
	fmt.Printf("  ✓ %d atualizados", ok)
	if skipped > 0 {
		fmt.Printf(" · ⊘ %d ignorados (senha/cancelado)", skipped)
	}
	if fail > 0 {
		fmt.Printf(" · ✘ %d falharam", fail)
	}
	fmt.Println()

	var needPass, manual, failed, other []*model.Item
	for _, s := range updates {
		for _, it := range s.Items {
			if it.Status != model.StatusOutdated {
				continue
			}
			stats.remaining++
			r := resultByItem[it]
			kind, reason := updater.ClassifyItem(it, r)
			switch kind {
			case updater.KindNeedsPassword:
				needPass = append(needPass, it)
				_ = reason
			case updater.KindManualOnly:
				manual = append(manual, it)
			case updater.KindFailed:
				failed = append(failed, it)
			default:
				other = append(other, it)
			}
		}
	}

	if stats.remaining == 0 {
		fmt.Println("\n✓ Tudo verificado — nada outdated restante")
		return stats
	}

	stats.manual = len(manual)

	fmt.Printf("\n⚠ %d item(s) ainda outdated:\n", stats.remaining)
	printVerifyGroup("Precisam senha / Terminal", needPass, resultByItem)
	printVerifyGroup("Só atualização manual", manual, resultByItem)
	printVerifyGroup("Falharam", failed, resultByItem)
	printVerifyGroup("Outros", other, resultByItem)

	return stats
}

func printVerifyGroup(title string, items []*model.Item, results map[*model.Item]*updater.Result) {
	if len(items) == 0 {
		return
	}
	fmt.Printf("\n  %s:\n", title)
	for _, it := range items {
		_, reason := updater.ClassifyItem(it, results[it])
		if reason == "" && results[it] != nil {
			reason = results[it].Error
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
