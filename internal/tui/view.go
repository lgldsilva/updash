package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/lgldsilva/updash/internal/model"
)

// Render renders the complete TUI view.
func (s *State) Render() string {
	if !s.Ready {
		return s.renderLoading()
	}

	var b strings.Builder

	// Title bar
	b.WriteString(s.renderTitle())
	b.WriteString("\n")

	// Tabs
	b.WriteString(s.renderTabs())
	b.WriteString("\n")

	// Content
	b.WriteString(s.renderContent())

	// Footer
	b.WriteString("\n")
	b.WriteString(s.renderFooter())

	return AppStyle.Render(b.String())
}

func (s *State) renderTitle() string {
	plat := strings.ToUpper(s.Platform.Distro)
	if s.Platform.OS == "darwin" {
		plat = "macOS"
	}
	return TitleStyle.Render(fmt.Sprintf(" updash — %s", plat))
}

func (s *State) renderTabs() string {
	tabs := []model.TabID{model.TabUpdates, model.TabCleanup, model.TabLogs}
	var parts []string
	for _, tab := range tabs {
		label := tab.String()
		count := 0
		if tab == model.TabUpdates {
			count = s.TotalOutdated()
		} else if tab == model.TabCleanup {
			count = s.TotalCleanable()
		}

		if count > 0 {
			label = fmt.Sprintf("%s (%d)", label, count)
		}

		if tab == s.ActiveTab {
			parts = append(parts, ActiveTabStyle.Render(label))
		} else {
			parts = append(parts, InactiveTabStyle.Render(label))
		}
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, parts...)
}

func (s *State) renderContent() string {
	switch s.ActiveTab {
	case model.TabUpdates:
		return s.renderUpdatesTab()
	case model.TabCleanup:
		return s.renderCleanupTab()
	case model.TabLogs:
		return s.renderLogsTab()
	default:
		return "unknown tab"
	}
}

func (s *State) renderUpdatesTab() string {
	if s.Scanning {
		return SpinnerStyle.Render(" Scanning...")
	}

	var b strings.Builder
	flatIdx := 0

	for _, summary := range s.Summaries {
		if len(summary.Items) == 0 {
			continue
		}

		// Category header with progress bar
		catLine := fmt.Sprintf(" %s %s  ", summary.Icon, summary.Label)
		catLine += s.renderProgressBar(summary.Total, summary.Total-summary.Outdated-summary.ErrorCount)
		catLine += fmt.Sprintf("  %d/%d ok", summary.Total-summary.Outdated-summary.ErrorCount, summary.Total)
		if summary.Outdated > 0 {
			catLine += fmt.Sprintf(" • %s outdated", VerNewStyle.Render(fmt.Sprintf("%d", summary.Outdated)))
		}
		b.WriteString(CatLabelStyle.Render(catLine))
		b.WriteString("\n")

		// Items
		for _, item := range summary.Items {
			prefix := "  "
			sel := " "

			if s.ActiveTab == model.TabUpdates {
				sel = " "
				if item.Selected {
					sel = CheckboxStyle.Render("◉")
				} else if item.Status == model.StatusOutdated {
					sel = "○"
				} else {
					sel = " "
				}
			}

			cursor := " "
			if flatIdx == s.Cursor && s.ActiveTab == model.TabUpdates {
				cursor = "▸"
			}

			line := fmt.Sprintf("%s%s%s ", cursor, sel, prefix)
			itemLine := s.renderItem(item, flatIdx)
			styled := s.applyItemStyle(line+itemLine, item, flatIdx)
			b.WriteString(styled)
			b.WriteString("\n")

			flatIdx++
		}
	}

	if s.TotalOutdated() == 0 && !s.Scanning {
		b.WriteString("\n ")
		b.WriteString(ItemOKStyle.Render("✓ All packages are up to date"))
		b.WriteString("\n")
	}

	return b.String()
}

func (s *State) renderCleanupTab() string {
	if s.Scanning {
		return SpinnerStyle.Render(" Scanning...")
	}

	var b strings.Builder
	flatIdx := 0

	for _, summary := range s.CleanItems {
		if len(summary.Items) == 0 {
			continue
		}
		// Skip summaries that have no cleanup candidates
		if !hasCleanupItems(summary) {
			continue
		}

		// Category header
		catLine := fmt.Sprintf(" %s %s", summary.Icon, summary.Label)
		totalReclaim := ""
		for _, it := range summary.Items {
			if it.Reclaimable != "" && it.Reclaimable != "0 versions" {
				if totalReclaim != "" {
					totalReclaim += " + "
				}
				totalReclaim += it.Reclaimable
			}
		}
		if totalReclaim != "" {
			catLine += fmt.Sprintf("  [ %s ]", ReclaimStyle.Render(totalReclaim))
		}
		b.WriteString(CatLabelStyle.Render(catLine))
		b.WriteString("\n")

		// Items
		for _, item := range summary.Items {
			sel := " "
			if item.Selected {
				sel = CheckboxStyle.Render("◉")
			} else if item.Status == model.StatusCleanCandidate {
				sel = "○"
			} else {
				sel = " "
			}

			cursor := " "
			if flatIdx == s.Cursor && s.ActiveTab == model.TabCleanup {
				cursor = "▸"
			}

			line := fmt.Sprintf("%s%s  ", cursor, sel)

			// Item name + version
			itemStr := item.Name
			if item.CurrentVer != "" {
				itemStr += fmt.Sprintf("  %s", item.CurrentVer)
			}
			if item.Reclaimable != "" {
				itemStr += fmt.Sprintf("  →  %s", ReclaimStyle.Render(item.Reclaimable))
			}
			if item.KeepPolicy != "" {
				itemStr += fmt.Sprintf("  (%s)", VerCurrentStyle.Render(item.KeepPolicy))
			}

			styled := s.applyItemStyle(line+itemStr, item, flatIdx)
			b.WriteString(styled)
			b.WriteString("\n")

			flatIdx++
		}
	}

	if s.TotalCleanable() == 0 && !s.Scanning {
		b.WriteString("\n ")
		b.WriteString(ItemOKStyle.Render("✓ Nothing to clean"))
		b.WriteString("\n")
	}

	return b.String()
}

func (s *State) renderLogsTab() string {
	var b strings.Builder

	if len(s.Logs) == 0 {
		b.WriteString("\n ")
		b.WriteString(VerCurrentStyle.Render("No log entries yet"))
		b.WriteString("\n")
		return b.String()
	}

	// Show logs in reverse order (newest first)
	for i := len(s.Logs) - 1; i >= 0; i-- {
		entry := s.Logs[i]
		icon := "✓"
		style := LogSuccessStyle
		if !entry.Success {
			icon = "✘"
			style = LogErrorStyle
		}
		line := fmt.Sprintf(" %s %s", icon, entry.Message)
		b.WriteString(style.Render(line))
		b.WriteString("\n")
	}

	return b.String()
}

func (s *State) renderItem(item *model.Item, idx int) string {
	switch item.Status {
	case model.StatusOK:
		if item.CurrentVer != "" {
			return fmt.Sprintf("%s  %s", item.Name, VerCurrentStyle.Render(item.CurrentVer))
		}
		return fmt.Sprintf("%s  %s", item.Name, VerCurrentStyle.Render("✓"))
	case model.StatusOutdated:
		cur := item.CurrentVer
		if cur == "" {
			cur = "?"
		}
		avail := item.AvailableVer
		if avail == "" {
			avail = "newer"
		}
		return fmt.Sprintf("%s  %s → %s",
			item.Name,
			VerCurrentStyle.Render(cur),
			VerNewStyle.Render(avail),
		)
	case model.StatusError:
		return fmt.Sprintf("%s  %s", item.Name, ItemErrorStyle.Render("✘ "+item.CurrentVer))
	case model.StatusUpdating:
		return fmt.Sprintf("%s  %s", item.Name, SpinnerStyle.Render("⟳ updating..."))
	case model.StatusDone:
		return fmt.Sprintf("%s  %s", item.Name, ItemOKStyle.Render("✓ updated"))
	case model.StatusCleanCandidate:
		return fmt.Sprintf("%s  %s  →  %s", item.Name, VerCurrentStyle.Render(item.CurrentVer), ReclaimStyle.Render(item.Reclaimable))
	case model.StatusCleaning:
		return fmt.Sprintf("%s  %s", item.Name, SpinnerStyle.Render("⟳ cleaning..."))
	case model.StatusCleaned:
		return fmt.Sprintf("%s  %s", item.Name, ItemOKStyle.Render("✓ cleaned"))
	default:
		return item.Name
	}
}

func (s *State) applyItemStyle(text string, item *model.Item, idx int) string {
	style := lipgloss.NewStyle()

	// Cursor highlight
	if idx == s.Cursor {
		style = style.Background(lipgloss.Color("#333333"))
	}

	// Color by status
	switch item.Status {
	case model.StatusOK:
		style = style.Foreground(ColorGray)
	case model.StatusOutdated:
		if s.ActiveTab == model.TabUpdates {
			style = style.Foreground(ColorYellow)
		}
	case model.StatusError:
		style = style.Foreground(ColorRed)
	case model.StatusUpdating, model.StatusCleaning:
		style = style.Foreground(ColorCyan)
	case model.StatusDone, model.StatusCleaned:
		style = style.Foreground(ColorGreen)
	case model.StatusCleanCandidate:
		style = style.Foreground(ColorOrange)
	}

	// Selected items get bold
	if item.Selected {
		style = style.Bold(true)
	}

	return style.Render(text)
}

func (s *State) renderProgressBar(total, done int) string {
	if total == 0 {
		return BarFilled.Render(strings.Repeat("█", 10))
	}

	width := 10
	filled := (done * width) / total
	if filled > width {
		filled = width
	}

	filledStr := strings.Repeat("█", filled)
	emptyStr := strings.Repeat("░", width-filled)

	bar := BarFilled.Render(filledStr) + BarEmpty.Render(emptyStr)
	return bar
}

func (s *State) renderLoading() string {
	var b strings.Builder
	b.WriteString("\n\n")
	b.WriteString(SpinnerStyle.Render(" 🔄 Scanning system..."))
	b.WriteString("\n")
	b.WriteString(VerCurrentStyle.Render("   Detecting package managers, outdated packages, and cleanup candidates"))
	b.WriteString("\n\n")
	return AppStyle.Render(b.String())
}

func (s *State) renderFooter() string {
	var hints []string

	switch s.ActiveTab {
	case model.TabUpdates:
		hints = []string{
			"[↑↓] navigate",
			"[Space] toggle",
			"[U] update selected",
			"[A] update all",
			"[R] refresh",
		}
	case model.TabCleanup:
		hints = []string{
			"[↑↓] navigate",
			"[Space] toggle",
			"[C] clean selected",
			"[R] refresh",
		}
	case model.TabLogs:
		hints = []string{
			"[R] refresh",
		}
	}

	hints = append(hints, "[1/2/3] tab", "[Q] quit")
	return FooterStyle.Render(strings.Join(hints, "  ·  "))
}

// hasCleanupItems checks if a summary has any cleanup candidates.
func hasCleanupItems(s *model.SourceSummary) bool {
	for _, it := range s.Items {
		if it.Status == model.StatusCleanCandidate || it.Status == model.StatusCleaning || it.Status == model.StatusCleaned {
			return true
		}
	}
	return false
}
