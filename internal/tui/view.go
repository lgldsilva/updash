package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/lgldsilva/updash/internal/model"
)

// Render renders the complete TUI view.
func (s *State) Render() string {
	var b strings.Builder

	// Title bar
	b.WriteString(s.renderTitle())
	b.WriteString("\n")

	// Password prompt overrides normal content
	if s.ShowPassword {
		b.WriteString(s.renderPassword())
		b.WriteString("\n")
		b.WriteString(s.renderPasswordFooter())
		return s.frame(b.String())
	}

	// Confirmation dialog overrides normal content
	if s.ShowConfirm {
		b.WriteString(s.renderConfirm())
		b.WriteString("\n")
		b.WriteString(s.renderConfirmFooter())
		return s.frame(b.String())
	}

	// Tabs
	b.WriteString(s.renderTabs())
	b.WriteString("\n")

	// Content
	b.WriteString(s.renderContent())

	// Status line (shows current operation)
	statusLine := s.renderStatusLine()
	if statusLine != "" {
		b.WriteString("\n")
		b.WriteString(statusLine)
	}

	// Footer
	b.WriteString("\n")
	b.WriteString(s.renderFooter())

	return s.frame(b.String())
}

func (s *State) renderTitle() string {
	plat := ""
	switch s.Platform.OS {
	case "darwin":
		plat = "macOS"
	case "windows":
		plat = "Windows"
	default:
		plat = strings.ToUpper(s.Platform.Distro)
	}
	return TitleStyle.Render(fmt.Sprintf(" updash — %s", plat))
}

func (s *State) renderTabs() string {
	tabs := []model.TabID{model.TabUpdates, model.TabCleanup, model.TabLogs}
	var parts []string
	for _, tab := range tabs {
		label := tab.String()
		var count int
		switch tab {
		case model.TabUpdates:
			count = s.TotalOutdated()
		case model.TabCleanup:
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
	var b strings.Builder

	if s.Scanning && len(s.Summaries) == 0 {
		b.WriteString(SpinnerStyle.Render(fmt.Sprintf(" %s Waiting for scan results...", s.spinnerGlyph())))
		b.WriteString("\n\n")
	}

	// Global progress bar when updating
	if s.Updating && s.UpdateTotal > 0 {
		label := ""
		if s.OperationLabel != "" {
			label = " " + s.OperationLabel
		}
		progLine := joinRow(
			SpinnerStyle.Render(s.spinnerGlyph()+" Updating"+label),
			lipgloss.NewStyle().Render("  "),
			s.renderProgressBar(s.UpdateTotal, s.UpdateDone),
			lipgloss.NewStyle().Render(fmt.Sprintf("  %d/%d", s.UpdateDone, s.UpdateTotal)),
		)
		b.WriteString(truncateStyled(progLine, s.contentWidth()))
		b.WriteString("\n\n")
	}

	flatIdx := 0

	for _, summary := range s.Summaries {
		if len(summary.Items) == 0 {
			continue
		}

		// Category header with live progress (reads item statuses, not static counts)
		catLine := s.renderCategoryHeader(summary)
		b.WriteString(catLine)
		b.WriteString("\n")

		if !hasUpdateItems(summary) {
			if summary.Category == model.CatAgent && summary.Total > 0 {
				b.WriteString(s.formatRow(
					joinRow(
						lipgloss.NewStyle().Render("  "),
						ItemOKStyle.Render(fmt.Sprintf("✓ %d installed, up to date", summary.Total)),
					),
					flatIdx,
				))
				b.WriteString("\n")
			}
			continue
		}

		// Items (only actionable — matches CurrentItems() / cursor)
		for _, item := range summary.Items {
			if !isUpdateNavigable(item.Status) {
				continue
			}
			prefix := "  "
			sel := ""

			if s.ActiveTab == model.TabUpdates {
				switch {
				case item.Selected:
					sel = CheckboxStyle.Render("◉")
				case item.Status == model.StatusOutdated:
					sel = "○"
				default:
					sel = " "
				}
			}

			cursor := " "
			if flatIdx == s.Cursor && s.ActiveTab == model.TabUpdates {
				cursor = "▸"
			}

			row := joinRow(
				lipgloss.NewStyle().Render(cursor),
				sel,
				lipgloss.NewStyle().Render(prefix),
				s.renderItemStyled(item),
			)
			b.WriteString(s.formatRow(row, flatIdx))
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
	var b strings.Builder

	if s.Scanning && len(s.CleanItems) == 0 {
		b.WriteString(SpinnerStyle.Render(fmt.Sprintf(" %s Waiting for scan results...", s.spinnerGlyph())))
		b.WriteString("\n\n")
	}

	// Global progress bar when cleaning
	if s.Cleaning && s.CleanTotal > 0 {
		label := ""
		if s.OperationLabel != "" {
			label = " " + s.OperationLabel
		}
		progLine := joinRow(
			SpinnerStyle.Render(s.spinnerGlyph()+" Cleaning"+label),
			lipgloss.NewStyle().Render("  "),
			s.renderProgressBar(s.CleanTotal, s.CleanDone),
			lipgloss.NewStyle().Render(fmt.Sprintf("  %d/%d", s.CleanDone, s.CleanTotal)),
		)
		b.WriteString(truncateStyled(progLine, s.contentWidth()))
		b.WriteString("\n\n")
	}

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
		header := CatLabelStyle.Render(catLine)
		if totalReclaim != "" {
			header = joinRow(header, lipgloss.NewStyle().Render("  [ "), ReclaimStyle.Render(totalReclaim), lipgloss.NewStyle().Render(" ]"))
		}
		b.WriteString(truncateStyled(header, s.contentWidth()))
		b.WriteString("\n")

		// Items (only navigable/cleanable — matches CurrentItems() / cursor)
		for _, item := range summary.Items {
			if !isCleanupNavigable(item.Status) {
				continue
			}

			var sel string
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

			body := s.renderCleanupItemStyled(item)
			row := joinRow(
				lipgloss.NewStyle().Render(cursor),
				sel,
				lipgloss.NewStyle().Render("  "),
				body,
			)
			b.WriteString(s.formatRow(row, flatIdx))
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

	// Show logs in reverse order (newest first), clipped to terminal height
	maxLines := s.maxListLines()
	shown := 0
	for i := len(s.Logs) - 1; i >= 0 && shown < maxLines; i-- {
		entry := s.Logs[i]
		icon := "✓"
		style := LogSuccessStyle
		if !entry.Success {
			icon = "✘"
			style = LogErrorStyle
		}
		line := truncatePlain(fmt.Sprintf(" %s %s", icon, entry.Message), s.contentWidth())
		b.WriteString(style.Render(line))
		b.WriteString("\n")
		shown++
	}
	if len(s.Logs) > maxLines {
		b.WriteString(VerCurrentStyle.Render(fmt.Sprintf(" … %d older entries (scroll N/A)", len(s.Logs)-maxLines)))
		b.WriteString("\n")
	}

	return b.String()
}

// categoryProgress holds live counts computed from item statuses.
type categoryProgress struct {
	ok, outdated, updating, done, errors int
}

// computeCategoryProgress scans items and returns live counts.
func computeCategoryProgress(items []*model.Item) categoryProgress {
	var p categoryProgress
	for _, it := range items {
		switch it.Status {
		case model.StatusOK:
			p.ok++
		case model.StatusOutdated:
			p.outdated++
		case model.StatusUpdating:
			p.updating++
		case model.StatusDone:
			p.done++
		case model.StatusError:
			p.errors++
		}
	}
	return p
}

// renderCategoryHeader builds the progress header for a summary category.
// Always reads live item statuses so counts stay accurate after updates/rescans.
func (s *State) renderCategoryHeader(summary *model.SourceSummary) string {
	prog := computeCategoryProgress(summary.Items)
	total := summary.Total
	if total == 0 {
		total = len(summary.Items)
	}
	done := prog.ok + prog.done
	outdated := prog.outdated
	updating := prog.updating
	errors := prog.errors

	parts := []string{
		CatLabelStyle.Render(fmt.Sprintf(" %s %s  ", summary.Icon, summary.Label)),
		s.renderProgressBar(total, done),
		lipgloss.NewStyle().Foreground(ColorGray).Render(fmt.Sprintf(" %d/%d", done, total)),
	}
	if updating > 0 {
		parts = append(parts, lipgloss.NewStyle().Foreground(ColorGray).Render(" • "), SpinnerStyle.Render(fmt.Sprintf("%d updating", updating)))
	}
	if outdated > 0 {
		parts = append(parts, lipgloss.NewStyle().Foreground(ColorGray).Render(" • "), VerNewStyle.Render(fmt.Sprintf("%d outdated", outdated)))
	}
	if errors > 0 {
		parts = append(parts, lipgloss.NewStyle().Foreground(ColorGray).Render(" • "), ItemErrorStyle.Render(fmt.Sprintf("%d errors", errors)))
	}
	return truncateStyled(joinRow(parts...), s.contentWidth())
}

// formatRow applies cursor highlight and width clamp to a pre-styled row.
func (s *State) formatRow(row string, idx int) string {
	max := s.contentWidth()
	if lipgloss.Width(row) > max {
		row = truncateStyled(row, max)
	}
	if idx == s.Cursor {
		return lipgloss.NewStyle().Background(lipgloss.Color("#2a2a2a")).Render(row)
	}
	return row
}

func (s *State) renderItemStyled(item *model.Item) string {
	name := truncatePlain(item.Name, s.contentWidth()/3)
	bold := lipgloss.NewStyle().Bold(item.Selected)

	switch item.Status {
	case model.StatusOK:
		ver := "✓"
		if item.CurrentVer != "" {
			ver = item.CurrentVer
		}
		return joinRow(bold.Render(name), lipgloss.NewStyle().Render("  "), VerCurrentStyle.Render(ver))
	case model.StatusOutdated:
		cur := item.CurrentVer
		if cur == "" {
			cur = "?"
		}
		avail := item.AvailableVer
		if avail == "" {
			avail = "newer"
		}
		return joinRow(
			lipgloss.NewStyle().Foreground(ColorYellow).Bold(item.Selected).Render(name),
			lipgloss.NewStyle().Render("  "),
			VerCurrentStyle.Render(cur),
			VerArrowStyle.Render(" → "),
			VerNewStyle.Render(avail),
		)
	case model.StatusError:
		return joinRow(name, lipgloss.NewStyle().Render("  "), ItemErrorStyle.Render("✘ "+truncatePlain(item.CurrentVer, 40)))
	case model.StatusUpdating:
		return joinRow(name, lipgloss.NewStyle().Render("  "), SpinnerStyle.Render(s.spinnerGlyph()+" updating..."))
	case model.StatusDone:
		return joinRow(name, lipgloss.NewStyle().Render("  "), ItemOKStyle.Render("✓ updated"))
	default:
		return name
	}
}

func (s *State) renderCleanupItemStyled(item *model.Item) string {
	name := truncatePlain(item.Name, s.contentWidth()/3)
	switch item.Status {
	case model.StatusCleanCandidate:
		parts := []string{lipgloss.NewStyle().Foreground(ColorOrange).Bold(item.Selected).Render(name)}
		if item.CurrentVer != "" {
			parts = append(parts, lipgloss.NewStyle().Render("  "), VerCurrentStyle.Render(item.CurrentVer))
		}
		if item.Reclaimable != "" {
			parts = append(parts, lipgloss.NewStyle().Render("  →  "), ReclaimStyle.Render(item.Reclaimable))
		}
		if item.KeepPolicy != "" {
			parts = append(parts, lipgloss.NewStyle().Render("  ("), VerCurrentStyle.Render(item.KeepPolicy), lipgloss.NewStyle().Render(")"))
		}
		return joinRow(parts...)
	case model.StatusCleaning:
		return joinRow(name, lipgloss.NewStyle().Render("  "), SpinnerStyle.Render(s.spinnerGlyph()+" cleaning..."))
	case model.StatusCleaned:
		return joinRow(name, lipgloss.NewStyle().Render("  "), ItemOKStyle.Render("✓ cleaned"))
	case model.StatusError:
		return joinRow(name, lipgloss.NewStyle().Render("  "), ItemErrorStyle.Render("✘ failed"))
	default:
		return name
	}
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

	filledStr := lipgloss.NewStyle().Foreground(ColorGreen).Render(strings.Repeat("█", filled))
	emptyStr := lipgloss.NewStyle().Foreground(ColorGray).Render(strings.Repeat("░", width-filled))
	return filledStr + emptyStr
}

func (s *State) renderLoading() string {
	var b strings.Builder
	b.WriteString("\n\n")
	b.WriteString(SpinnerStyle.Render(" 🔄 Scanning system..."))
	b.WriteString("\n")
	b.WriteString(VerCurrentStyle.Render(truncatePlain("   Checking package managers, outdated packages, and cleanup candidates", s.contentWidth())))
	b.WriteString("\n")
	if s.Scanning {
		b.WriteString(VerCurrentStyle.Render("   Running scans in parallel for updates + cleanup"))
	}
	b.WriteString("\n\n")
	return s.frame(b.String())
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
			"[A] clean all",
			"[R] refresh",
		}
	case model.TabLogs:
		hints = []string{
			"[R] refresh",
		}
	}

	if sel := s.SelectedCount(); sel > 0 && (s.ActiveTab == model.TabUpdates || s.ActiveTab == model.TabCleanup) {
		hints = append(hints, fmt.Sprintf("[%d selected]", sel))
	}

	hints = append(hints, "[1/2/3] tab", "[Q] quit")
	return wrapFooter(hints, s.contentWidth())
}

// renderStatusLine shows a one-line status of the current async operation.
func (s *State) renderStatusLine() string {
	switch {
	case s.Scanning:
		label := s.OperationLabel
		if label == "" {
			label = "system"
		}
		prog := ""
		if s.ScanTotal > 0 {
			prog = fmt.Sprintf("  %s  %d/%d sources",
				s.renderProgressBar(s.ScanTotal, s.ScanDone),
				s.ScanDone, s.ScanTotal)
		}
		return SpinnerStyle.Render(fmt.Sprintf(" %s Scanning %s%s", s.spinnerGlyph(), label, prog))
	case s.Updating:
		label := s.OperationLabel
		if label == "" {
			label = "packages"
		}
		prog := joinRow(
			SpinnerStyle.Render(s.spinnerGlyph()+" Updating "+label),
			lipgloss.NewStyle().Render("  "),
			s.renderProgressBar(s.UpdateTotal, s.UpdateDone),
			lipgloss.NewStyle().Render(fmt.Sprintf("  %d/%d", s.UpdateDone, s.UpdateTotal)),
		)
		return truncateStyled(prog, s.contentWidth())
	case s.Cleaning:
		label := s.OperationLabel
		if label == "" {
			label = "items"
		}
		prog := joinRow(
			SpinnerStyle.Render(s.spinnerGlyph()+" Cleaning "+label),
			lipgloss.NewStyle().Render("  "),
			s.renderProgressBar(s.CleanTotal, s.CleanDone),
			lipgloss.NewStyle().Render(fmt.Sprintf("  %d/%d", s.CleanDone, s.CleanTotal)),
		)
		return truncateStyled(prog, s.contentWidth())
	case s.LastSummary != "":
		style := ItemOKStyle
		if strings.Contains(s.LastSummary, "failed") {
			style = ItemErrorStyle
		}
		msg := truncatePlain(s.LastSummary+"  ·  [3] Logs  ·  [R] rescan", s.contentWidth())
		return style.Render(" " + msg)
	}
	return ""
}

// renderPassword shows the sudo password prompt (reused across elevated commands).
func (s *State) renderPassword() string {
	var b strings.Builder
	b.WriteString("\n")
	b.WriteString(ConfirmStyle.Render(" 🔐 Administrator password required"))
	b.WriteString("\n\n")
	b.WriteString(" Your Mac login password (for sudo). MAS uses the system sudo cache —\n")
	b.WriteString(" asked right before App Store updates, not during long brew downloads.\n\n")
	masked := strings.Repeat("•", len(s.PasswordInput))
	b.WriteString(ButtonStyle.Render(" ") + masked + "_")
	if s.PasswordError != "" {
		b.WriteString("\n\n")
		b.WriteString(ItemErrorStyle.Render(" ✘ " + s.PasswordError))
	}
	b.WriteString("\n")
	return b.String()
}

func (s *State) renderPasswordFooter() string {
	return FooterStyle.Render("[Enter] submit  [Esc] cancel")
}

// renderConfirm shows the confirmation dialog for destructive actions.
func (s *State) renderConfirm() string {
	var b strings.Builder
	b.WriteString("\n")
	b.WriteString(ConfirmStyle.Render(" ⚠ " + s.ConfirmMsg))
	b.WriteString("\n\n")
	b.WriteString(ButtonStyle.Render(" Y") + "  yes  ")
	b.WriteString(ButtonStyle.Render(" N") + "  no")
	b.WriteString("\n")
	return b.String()
}

// renderConfirmFooter shows key hints during confirmation.
func (s *State) renderConfirmFooter() string {
	return FooterStyle.Render("[Y] yes  [N] no  [Esc] cancel")
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
