package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/lgldsilva/updash/internal/model"
)

const (
	fmtProgressCount = "  %d/%d"
	hintRefresh      = "[R] refresh"
)

// Render renders the complete TUI view.
func (s *State) Render() string {
	var b strings.Builder

	// Title bar + blank line (explicit spacing; no style margins)
	b.WriteString(s.renderTitle())
	b.WriteString("\n\n")

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

	// Tabs (space-separated without MarginRight)
	b.WriteString(s.renderTabs())
	b.WriteString("\n\n")

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
	ver := s.Version
	if ver == "" {
		ver = "dev"
	}
	title := fmt.Sprintf(" updash %s — %s", ver, plat)
	if s.LatestTag != "" {
		title += fmt.Sprintf(" · latest %s", s.LatestTag)
	}
	return TitleStyle.Render(title)
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
	// Explicit single-space gap between tabs (no MarginRight on styles)
	return lipgloss.JoinHorizontal(lipgloss.Top, interleaveSpaces(parts)...)
}

func interleaveSpaces(parts []string) []string {
	if len(parts) <= 1 {
		return parts
	}
	out := make([]string, 0, len(parts)*2-1)
	for i, p := range parts {
		if i > 0 {
			out = append(out, " ")
		}
		out = append(out, p)
	}
	return out
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
	s.writeScanWait(&b, len(s.Summaries) == 0)
	s.writeOpProgress(&b, s.Updating && s.UpdateTotal > 0, "Updating", s.UpdateTotal, s.UpdateDone)

	flatIdx := 0
	firstCat := true
	for _, summary := range s.Summaries {
		if len(summary.Items) == 0 {
			continue
		}
		if !firstCat {
			b.WriteString("\n")
		}
		firstCat = false
		b.WriteString(s.renderCategoryHeader(summary))
		b.WriteString("\n")
		if !hasUpdateItems(summary) {
			s.writeAgentUpToDate(&b, summary)
			continue
		}
		flatIdx = s.writeUpdateItems(&b, summary, flatIdx)
	}

	if s.TotalOutdated() == 0 && !s.Scanning {
		b.WriteString("\n ")
		b.WriteString(ItemOKStyle.Render("✓ All packages are up to date"))
		b.WriteString("\n")
	}
	return b.String()
}

func (s *State) writeScanWait(b *strings.Builder, waiting bool) {
	if !s.Scanning || !waiting {
		return
	}
	b.WriteString(SpinnerStyle.Render(fmt.Sprintf(" %s Waiting for scan results...", s.spinnerGlyph())))
	b.WriteString("\n\n")
}

func (s *State) writeOpProgress(b *strings.Builder, active bool, verb string, total, done int) {
	if !active {
		return
	}
	label := ""
	if s.OperationLabel != "" {
		label = " " + s.OperationLabel
	}
	progLine := joinRow(
		SpinnerStyle.Render(s.spinnerGlyph()+" "+verb+label),
		lipgloss.NewStyle().Render("  "),
		s.renderProgressBar(total, done),
		lipgloss.NewStyle().Render(fmt.Sprintf(fmtProgressCount, done, total)),
	)
	b.WriteString(truncateStyled(progLine, s.contentWidth()))
	b.WriteString("\n\n")
}

func (s *State) writeAgentUpToDate(b *strings.Builder, summary *model.SourceSummary) {
	if summary.Total == 0 {
		return
	}
	var msg string
	switch summary.Category {
	case model.CatAgent:
		msg = fmt.Sprintf("✓ %d installed, up to date", summary.Total)
	case model.CatOpenCodePlugins:
		msg = "✓ plugins up to date"
	default:
		return
	}
	indent := strings.Repeat(" ", 4)
	b.WriteString(joinRow(
		lipgloss.NewStyle().Render(indent),
		ItemOKStyle.Render(msg),
	))
	b.WriteString("\n")
}

func (s *State) writeUpdateItems(b *strings.Builder, summary *model.SourceSummary, flatIdx int) int {
	for _, item := range summary.Items {
		if !isUpdateNavigable(item.Status) {
			continue
		}
		gutter := fmt.Sprintf("%s %s ", s.rowCursor(flatIdx, model.TabUpdates), s.updateCheckbox(item))
		row := joinRow(lipgloss.NewStyle().Render(gutter), s.renderItemStyled(item))
		b.WriteString(s.formatRow(row, flatIdx))
		b.WriteString("\n")
		flatIdx++
	}
	return flatIdx
}

func (s *State) updateCheckbox(item *model.Item) string {
	if s.ActiveTab != model.TabUpdates {
		return " "
	}
	switch {
	case item.Selected:
		return CheckboxStyle.Render("◉")
	case item.Status == model.StatusOutdated:
		return "○"
	default:
		return " "
	}
}

func (s *State) rowCursor(flatIdx int, tab model.TabID) string {
	if flatIdx == s.Cursor && s.ActiveTab == tab {
		return "▸"
	}
	return " "
}

func (s *State) renderCleanupTab() string {
	var b strings.Builder
	s.writeScanWait(&b, len(s.CleanItems) == 0)
	s.writeOpProgress(&b, s.Cleaning && s.CleanTotal > 0, "Cleaning", s.CleanTotal, s.CleanDone)

	flatIdx := 0
	firstCat := true
	for _, summary := range s.CleanItems {
		if len(summary.Items) == 0 || !hasCleanupItems(summary) {
			continue
		}
		if !firstCat {
			b.WriteString("\n")
		}
		firstCat = false
		b.WriteString(s.renderCleanupCategoryHeader(summary))
		b.WriteString("\n")
		flatIdx = s.writeCleanupItems(&b, summary, flatIdx)
	}

	if s.TotalCleanable() == 0 && !s.Scanning {
		b.WriteString("\n ")
		b.WriteString(ItemOKStyle.Render("✓ Nothing to clean"))
		b.WriteString("\n")
	}
	return b.String()
}

func (s *State) renderCleanupCategoryHeader(summary *model.SourceSummary) string {
	label := padRight(iconCell(summary.Icon)+" "+summary.Label, s.metrics().catLabel)
	header := CatLabelStyle.Render(label)
	if totalReclaim := sumReclaimable(summary); totalReclaim != "" {
		header = joinRow(header, lipgloss.NewStyle().Render("  "), ReclaimStyle.Render(totalReclaim))
	}
	return truncateStyled(header, s.contentWidth())
}

func sumReclaimable(summary *model.SourceSummary) string {
	var parts []string
	for _, it := range summary.Items {
		if it.Reclaimable != "" && it.Reclaimable != "0 versions" {
			parts = append(parts, it.Reclaimable)
		}
	}
	return strings.Join(parts, " + ")
}

func (s *State) writeCleanupItems(b *strings.Builder, summary *model.SourceSummary, flatIdx int) int {
	for _, item := range summary.Items {
		if !isCleanupNavigable(item.Status) {
			continue
		}
		gutter := fmt.Sprintf("%s %s ", s.rowCursor(flatIdx, model.TabCleanup), s.cleanupCheckbox(item))
		row := joinRow(lipgloss.NewStyle().Render(gutter), s.renderCleanupItemStyled(item))
		b.WriteString(s.formatRow(row, flatIdx))
		b.WriteString("\n")
		flatIdx++
	}
	return flatIdx
}

func (s *State) cleanupCheckbox(item *model.Item) string {
	switch {
	case item.Selected:
		return CheckboxStyle.Render("◉")
	case item.Status == model.StatusCleanCandidate:
		return "○"
	default:
		return " "
	}
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
// Layout scales with terminal width via metrics().
func (s *State) renderCategoryHeader(summary *model.SourceSummary) string {
	m := s.metrics()
	prog := computeCategoryProgress(summary.Items)
	total := summary.Total
	if total == 0 {
		total = len(summary.Items)
	}
	done := prog.ok + prog.done
	outdated := prog.outdated
	updating := prog.updating
	errors := prog.errors

	// iconCell keeps emoji at exactly 2 cols so the right border doesn't drift
	label := padRight(iconCell(summary.Icon)+" "+summary.Label, m.catLabel)
	count := padLeft(fmt.Sprintf("%d/%d", done, total), 5)

	parts := []string{
		CatLabelStyle.Render(label),
		lipgloss.NewStyle().Render(" "),
		s.renderProgressBar(total, done),
		lipgloss.NewStyle().Foreground(ColorGray).Render(" " + count),
	}
	if updating > 0 {
		parts = append(parts, lipgloss.NewStyle().Foreground(ColorGray).Render("  ·  "), SpinnerStyle.Render(fmt.Sprintf("%d updating", updating)))
	}
	if outdated > 0 {
		parts = append(parts, lipgloss.NewStyle().Foreground(ColorGray).Render("  ·  "), VerNewStyle.Render(fmt.Sprintf("%d outdated", outdated)))
	}
	if errors > 0 {
		parts = append(parts, lipgloss.NewStyle().Foreground(ColorGray).Render("  ·  "), ItemErrorStyle.Render(fmt.Sprintf("%d errors", errors)))
	}
	return truncateStyled(joinRow(parts...), m.width)
}

// formatRow clamps/pads a row to the full content width and applies cursor highlight.
// Always returns a string of exactly contentWidth cells so the manual frame stays square.
func (s *State) formatRow(row string, idx int) string {
	max := s.contentWidth()
	row = fitLine(row, max)
	if idx == s.Cursor {
		// Re-fit after background style — some terminals measure bold/bg differently
		row = lipgloss.NewStyle().Background(lipgloss.Color("#2a2a2a")).Render(row)
		row = fitLine(row, max)
	}
	return row
}

func (s *State) renderItemStyled(item *model.Item) string {
	m := s.metrics()
	namePlain := padRight(item.Name, m.name)
	bold := lipgloss.NewStyle().Bold(item.Selected)

	switch item.Status {
	case model.StatusOK:
		ver := "✓"
		if item.CurrentVer != "" {
			ver = item.CurrentVer
		}
		return joinRow(
			bold.Render(namePlain),
			lipgloss.NewStyle().Render("  "),
			VerCurrentStyle.Render(padRight(ver, m.ver)),
		)
	case model.StatusOutdated:
		cur := item.CurrentVer
		if cur == "" {
			cur = "?"
		}
		avail := item.AvailableVer
		if avail == "" {
			avail = "newer"
		}
		parts := []string{
			lipgloss.NewStyle().Foreground(ColorYellow).Bold(item.Selected).Render(namePlain),
			lipgloss.NewStyle().Render("  "),
			VerCurrentStyle.Render(padRight(cur, m.ver)),
			VerArrowStyle.Render(" → "),
			VerNewStyle.Render(padRight(avail, m.ver)),
		}
		if item.KeepPolicy != "" && m.note > 0 {
			parts = append(parts,
				lipgloss.NewStyle().Render("  "),
				VerCurrentStyle.Render(truncatePlain("("+item.KeepPolicy+")", m.note)),
			)
		}
		return joinRow(parts...)
	case model.StatusError:
		return joinRow(
			bold.Render(namePlain),
			lipgloss.NewStyle().Render("  "),
			ItemErrorStyle.Render("✘ "+truncatePlain(item.CurrentVer, m.ver*2)),
		)
	case model.StatusUpdating:
		return joinRow(
			bold.Render(namePlain),
			lipgloss.NewStyle().Render("  "),
			SpinnerStyle.Render(s.spinnerGlyph()+" updating..."),
		)
	case model.StatusDone:
		return joinRow(
			bold.Render(namePlain),
			lipgloss.NewStyle().Render("  "),
			ItemOKStyle.Render("✓ updated"),
		)
	default:
		return bold.Render(namePlain)
	}
}

func (s *State) renderCleanupItemStyled(item *model.Item) string {
	m := s.metrics()
	namePlain := padRight(item.Name, m.name)

	switch item.Status {
	case model.StatusCleanCandidate:
		parts := []string{lipgloss.NewStyle().Foreground(ColorOrange).Bold(item.Selected).Render(namePlain)}
		if item.CurrentVer != "" {
			parts = append(parts, lipgloss.NewStyle().Render("  "), VerCurrentStyle.Render(padRight(item.CurrentVer, m.ver)))
		}
		if item.Reclaimable != "" {
			// reclaim takes part of the note budget
			parts = append(parts, lipgloss.NewStyle().Render("  →  "), ReclaimStyle.Render(truncatePlain(item.Reclaimable, maxInt(8, m.note/2))))
		}
		if item.KeepPolicy != "" && m.note > 0 {
			parts = append(parts, lipgloss.NewStyle().Render("  "), VerCurrentStyle.Render(truncatePlain("("+item.KeepPolicy+")", m.note)))
		}
		return joinRow(parts...)
	case model.StatusCleaning:
		return joinRow(namePlain, lipgloss.NewStyle().Render("  "), SpinnerStyle.Render(s.spinnerGlyph()+" cleaning..."))
	case model.StatusCleaned:
		msg := "✓ cleaned"
		if item.Freed != "" && item.Freed != "0B" {
			msg = "✓ freed " + item.Freed
		} else if item.Freed == "0B" {
			msg = "✓ nothing removed"
		}
		return joinRow(namePlain, lipgloss.NewStyle().Render("  "), ItemOKStyle.Render(msg))
	case model.StatusError:
		return joinRow(namePlain, lipgloss.NewStyle().Render("  "), ItemErrorStyle.Render("✘ failed"))
	default:
		return namePlain
	}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func (s *State) renderProgressBar(total, done int) string {
	width := s.metrics().bar
	if total <= 0 {
		// Neutral empty bar (unknown total)
		return lipgloss.NewStyle().Foreground(ColorGray).Render("[" + strings.Repeat("─", width) + "]")
	}

	filled := (done * width) / total
	if filled > width {
		filled = width
	}
	if done > 0 && filled == 0 {
		filled = 1 // show a sliver when progress has started
	}

	// Bracketed bar so empty and full states are equally wide and readable
	open := lipgloss.NewStyle().Foreground(ColorGray).Render("[")
	closeB := lipgloss.NewStyle().Foreground(ColorGray).Render("]")
	var mid string
	switch filled {
	case width:
		mid = lipgloss.NewStyle().Foreground(ColorGreen).Render(strings.Repeat("█", width))
	case 0:
		mid = lipgloss.NewStyle().Foreground(ColorGray).Render(strings.Repeat("─", width))
	default:
		mid = lipgloss.NewStyle().Foreground(ColorGreen).Render(strings.Repeat("█", filled)) +
			lipgloss.NewStyle().Foreground(ColorGray).Render(strings.Repeat("─", width-filled))
	}
	return open + mid + closeB
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
			hintRefresh,
		}
	case model.TabCleanup:
		hints = []string{
			"[↑↓] navigate",
			"[Space] toggle",
			"[C] clean selected",
			"[A] clean all",
			hintRefresh,
		}
	case model.TabLogs:
		hints = []string{
			hintRefresh,
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
			lipgloss.NewStyle().Render(fmt.Sprintf(fmtProgressCount, s.UpdateDone, s.UpdateTotal)),
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
			lipgloss.NewStyle().Render(fmt.Sprintf(fmtProgressCount, s.CleanDone, s.CleanTotal)),
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
