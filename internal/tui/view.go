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
	b.WriteString(tuiBlankLine)

	// Password prompt overrides normal content
	if s.ShowPassword {
		b.WriteString(s.renderPassword())
		b.WriteString(tuiNewline)
		b.WriteString(s.renderPasswordFooter())
		return s.frame(b.String())
	}

	// Confirmation dialog overrides normal content
	if s.ShowConfirm {
		b.WriteString(s.renderConfirm())
		b.WriteString(tuiNewline)
		b.WriteString(s.renderConfirmFooter())
		return s.frame(b.String())
	}

	// Help screen overlay
	if s.ShowHelp {
		b.WriteString(s.renderHelp())
		b.WriteString(tuiNewline)
		b.WriteString(s.renderHelpFooter())
		return s.frame(b.String())
	}

	// Filter input bar
	if s.ShowFilter {
		b.WriteString(s.renderFilter())
		b.WriteString(tuiNewline)
	}

	// Detail view overlay
	if s.ShowDetail {
		b.WriteString(s.renderDetail())
		b.WriteString(tuiNewline)
		b.WriteString(s.renderDetailFooter())
		return s.frame(b.String())
	}

	// Tabs (space-separated without MarginRight)
	b.WriteString(s.renderTabs())
	b.WriteString(tuiBlankLine)

	// Content
	b.WriteString(s.renderContent())

	// Status line (shows current operation)
	statusLine := s.renderStatusLine()
	if statusLine != "" {
		b.WriteString(tuiNewline)
		b.WriteString(statusLine)
	}

	// Footer
	b.WriteString(tuiNewline)
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
			out = append(out, tuiSpace)
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

// itemTabConfig parameterizes the shared update/cleanup tab rendering.
type itemTabConfig struct {
	summaries   []*model.SourceSummary
	scanEmpty   bool
	opActive    bool
	opVerb      string
	opTotal     int
	opDone      int
	hasItems    func(*model.SourceSummary) bool
	renderEmpty func() string
	header      func(*model.SourceSummary) string
	writeItems  func(*strings.Builder, *model.SourceSummary, int) int
}

func (s *State) renderItemTab(cfg itemTabConfig) string {
	var b strings.Builder
	s.writeScanWait(&b, cfg.scanEmpty)
	s.writeOpProgress(&b, cfg.opActive, cfg.opVerb, cfg.opTotal, cfg.opDone)

	flatIdx := 0
	firstCat := true
	for _, summary := range cfg.summaries {
		if len(summary.Items) == 0 {
			continue
		}
		if cfg.hasItems != nil && !cfg.hasItems(summary) {
			continue
		}
		if !firstCat {
			b.WriteString(tuiNewline)
		}
		firstCat = false
		b.WriteString(cfg.header(summary))
		b.WriteString(tuiNewline)
		flatIdx = cfg.writeItems(&b, summary, flatIdx)
	}

	if cfg.renderEmpty != nil {
		b.WriteString(cfg.renderEmpty())
	}
	return b.String()
}

func (s *State) renderUpdatesTab() string {
	empty := ""
	updateProblems := scanProblemCount(s.Summaries)
	updateNonAffirmative := scanNonAffirmativeCount(s.Summaries)
	switch {
	case s.TotalOutdated() == 0 && updateProblems > 0:
		empty = tuiNewlineIndent + ItemErrorStyle.Render("⚠ No outdated packages — source state is inconclusive (see Logs tab)") + tuiNewline
	case s.TotalOutdated() == 0 && updateNonAffirmative > 0:
		empty = tuiNewlineIndent + VerCurrentStyle.Render("ⓘ No outdated packages — source state is informational, not verified") + tuiNewline
	case s.TotalOutdated() == 0 && updateNonAffirmative == 0 && !s.Scanning:
		empty = tuiNewlineIndent + ItemOKStyle.Render("✓ All packages are up to date") + tuiNewline
	}
	return s.renderItemTab(itemTabConfig{
		summaries:   s.Summaries,
		scanEmpty:   len(s.Summaries) == 0,
		opActive:    s.Updating && s.UpdateTotal > 0,
		opVerb:      "Updating",
		opTotal:     s.UpdateTotal,
		opDone:      s.UpdateDone,
		renderEmpty: func() string { return empty },
		header:      s.renderCategoryHeader,
		writeItems: func(b *strings.Builder, summary *model.SourceSummary, flatIdx int) int {
			if !hasUpdateItems(summary) {
				s.writeAgentUpToDate(b, summary)
				return flatIdx
			}
			return s.writeUpdateItems(b, summary, flatIdx)
		},
	})
}

func (s *State) renderCleanupTab() string {
	empty := ""
	cleanupProblems := scanProblemCount(s.CleanItems)
	cleanupNonAffirmative := scanNonAffirmativeCount(s.CleanItems)
	if s.TotalCleanable() == 0 && cleanupProblems > 0 {
		empty = tuiNewlineIndent + ItemErrorStyle.Render("⚠ Nothing to clean — source state is inconclusive (see Logs tab)") + tuiNewline
	} else if s.TotalCleanable() == 0 && cleanupNonAffirmative > 0 {
		empty = tuiNewlineIndent + VerCurrentStyle.Render("ⓘ No cleanup candidates — source inventory is informational, not verified") + tuiNewline
	} else if s.TotalCleanable() == 0 && !s.Scanning {
		empty = tuiNewlineIndent + ItemOKStyle.Render("✓ Nothing to clean") + tuiNewline
	}
	return s.renderItemTab(itemTabConfig{
		summaries:   s.CleanItems,
		scanEmpty:   len(s.CleanItems) == 0,
		opActive:    s.Cleaning && s.CleanTotal > 0,
		opVerb:      "Cleaning",
		opTotal:     s.CleanTotal,
		opDone:      s.CleanDone,
		hasItems:    hasCleanupItems,
		renderEmpty: func() string { return empty },
		header:      s.renderCleanupCategoryHeader,
		writeItems: func(b *strings.Builder, summary *model.SourceSummary, flatIdx int) int {
			return s.writeCleanupItems(b, summary, flatIdx)
		},
	})
}

func scanProblemCount(summaries []*model.SourceSummary) int {
	var problems int
	for _, summary := range summaries {
		problems += countStatus(summary.Items, model.StatusError)
		problems += countStatus(summary.Items, model.StatusUnverified)
	}
	return problems
}

func scanNonAffirmativeCount(summaries []*model.SourceSummary) int {
	return scanProblemCount(summaries) + countSummariesStatus(summaries, model.StatusInfo)
}

func (s *State) writeScanWait(b *strings.Builder, waiting bool) {
	if !s.Scanning || !waiting {
		return
	}
	b.WriteString(SpinnerStyle.Render(fmt.Sprintf(" %s Waiting for scan results...", s.spinnerGlyph())))
	b.WriteString(tuiBlankLine)
}

func (s *State) writeOpProgress(b *strings.Builder, active bool, verb string, total, done int) {
	if !active {
		return
	}
	label := ""
	if s.OperationLabel != "" {
		label = tuiSpace + s.OperationLabel
	}
	progLine := joinRow(
		SpinnerStyle.Render(s.spinnerGlyph()+tuiSpace+verb+label),
		lipgloss.NewStyle().Render(tuiIndent),
		s.renderProgressBar(total, done),
		lipgloss.NewStyle().Render(fmt.Sprintf(fmtProgressCount, done, total)),
	)
	b.WriteString(truncateStyled(progLine, s.contentWidth()))
	b.WriteString(tuiBlankLine)
}

func (s *State) writeAgentUpToDate(b *strings.Builder, summary *model.SourceSummary) {
	if summary.Total == 0 || !summaryItemsAffirmativelyOK(summary.Items) {
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
	indent := strings.Repeat(tuiSpace, 4)
	b.WriteString(joinRow(
		lipgloss.NewStyle().Render(indent),
		ItemOKStyle.Render(msg),
	))
	b.WriteString(tuiNewline)
}

func summaryItemsAffirmativelyOK(items []*model.Item) bool {
	if len(items) == 0 {
		return false
	}
	for _, item := range items {
		if item == nil || item.Status != model.StatusOK {
			return false
		}
	}
	return true
}

func (s *State) writeUpdateItems(b *strings.Builder, summary *model.SourceSummary, flatIdx int) int {
	return s.writeItems(b, summary, flatIdx, model.TabUpdates, isUpdateNavigable, s.updateCheckbox, s.renderItemStyled)
}

func (s *State) writeItems(
	b *strings.Builder,
	summary *model.SourceSummary,
	flatIdx int,
	tab model.TabID,
	isNavigable func(model.Status) bool,
	checkbox func(*model.Item) string,
	renderItem func(*model.Item) string,
) int {
	for _, item := range summary.Items {
		if !isNavigable(item.Status) {
			continue
		}
		gutter := fmt.Sprintf("%s %s ", s.rowCursor(flatIdx, tab), checkbox(item))
		row := joinRow(lipgloss.NewStyle().Render(gutter), renderItem(item))
		b.WriteString(s.formatRow(row, flatIdx))
		b.WriteString(tuiNewline)
		flatIdx++
	}
	return flatIdx
}

func checkboxSymbol(selected, selectable bool) string {
	switch {
	case selected:
		return CheckboxStyle.Render("◉")
	case selectable:
		return "○"
	default:
		return tuiSpace
	}
}

func (s *State) updateCheckbox(item *model.Item) string {
	if s.ActiveTab != model.TabUpdates {
		return tuiSpace
	}
	return checkboxSymbol(item.Selected, item.Status == model.StatusOutdated)
}

func (s *State) rowCursor(flatIdx int, tab model.TabID) string {
	if flatIdx == s.Cursor && s.ActiveTab == tab {
		return "▸"
	}
	return tuiSpace
}

func (s *State) renderCleanupCategoryHeader(summary *model.SourceSummary) string {
	label := padRight(iconCell(summary.Icon)+tuiSpace+summary.Label, s.metrics().catLabel)
	header := CatLabelStyle.Render(label)
	if totalReclaim := sumReclaimable(summary); totalReclaim != "" {
		header = joinRow(header, lipgloss.NewStyle().Render(tuiIndent), ReclaimStyle.Render(totalReclaim))
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
	return s.writeItems(b, summary, flatIdx, model.TabCleanup, isCleanupNavigable, s.cleanupCheckbox, s.renderCleanupItemStyled)
}

func (s *State) cleanupCheckbox(item *model.Item) string {
	return checkboxSymbol(item.Selected, item.Status == model.StatusCleanCandidate)
}

func (s *State) renderLogsTab() string {
	var b strings.Builder

	if len(s.Logs) == 0 {
		b.WriteString(tuiNewlineIndent)
		b.WriteString(VerCurrentStyle.Render("No log entries yet"))
		b.WriteString(tuiNewline)
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
		b.WriteString(tuiNewline)
		shown++
	}
	if len(s.Logs) > maxLines {
		b.WriteString(VerCurrentStyle.Render(fmt.Sprintf(" … %d older entries (scroll N/A)", len(s.Logs)-maxLines)))
		b.WriteString(tuiNewline)
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
	label := padRight(iconCell(summary.Icon)+tuiSpace+summary.Label, m.catLabel)
	count := padLeft(fmt.Sprintf("%d/%d", done, total), 5)

	parts := []string{
		CatLabelStyle.Render(label),
		lipgloss.NewStyle().Render(tuiSpace),
		s.renderProgressBar(total, done),
		lipgloss.NewStyle().Foreground(ColorGray).Render(tuiSpace + count),
	}
	if updating > 0 {
		parts = append(parts, lipgloss.NewStyle().Foreground(ColorGray).Render(tuiBulletGap), SpinnerStyle.Render(fmt.Sprintf("%d updating", updating)))
	}
	if outdated > 0 {
		parts = append(parts, lipgloss.NewStyle().Foreground(ColorGray).Render(tuiBulletGap), VerNewStyle.Render(fmt.Sprintf("%d outdated", outdated)))
	}
	if errors > 0 {
		parts = append(parts, lipgloss.NewStyle().Foreground(ColorGray).Render(tuiBulletGap), ItemErrorStyle.Render(fmt.Sprintf("%d errors", errors)))
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
			lipgloss.NewStyle().Render(tuiIndent),
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
			lipgloss.NewStyle().Render(tuiIndent),
			VerCurrentStyle.Render(padRight(cur, m.ver)),
			VerArrowStyle.Render(" → "),
			VerNewStyle.Render(padRight(avail, m.ver)),
		}
		if item.KeepPolicy != "" && m.note > 0 {
			parts = append(parts,
				lipgloss.NewStyle().Render(tuiIndent),
				VerCurrentStyle.Render(truncatePlain("("+item.KeepPolicy+")", m.note)),
			)
		}
		return joinRow(parts...)
	case model.StatusError:
		return joinRow(
			bold.Render(namePlain),
			lipgloss.NewStyle().Render(tuiIndent),
			ItemErrorStyle.Render("✘ "+truncatePlain(item.CurrentVer, m.ver*2)),
		)
	case model.StatusUpdating:
		return joinRow(
			bold.Render(namePlain),
			lipgloss.NewStyle().Render(tuiIndent),
			SpinnerStyle.Render(s.spinnerGlyph()+" updating..."),
		)
	case model.StatusDone:
		return joinRow(
			bold.Render(namePlain),
			lipgloss.NewStyle().Render(tuiIndent),
			ItemOKStyle.Render("✓ updated"),
		)
	case model.StatusInfo:
		return joinRow(bold.Render(namePlain), lipgloss.NewStyle().Render(tuiIndent), VerCurrentStyle.Render("ⓘ "+truncatePlain(item.CurrentVer, m.ver*2)))
	case model.StatusUnverified:
		return joinRow(bold.Render(namePlain), lipgloss.NewStyle().Render(tuiIndent), ItemErrorStyle.Render("⚠ unverified — "+truncatePlain(item.CurrentVer, m.ver*2)))
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
			parts = append(parts, lipgloss.NewStyle().Render(tuiIndent), VerCurrentStyle.Render(padRight(item.CurrentVer, m.ver)))
		}
		if item.Reclaimable != "" {
			// reclaim takes part of the note budget
			parts = append(parts, lipgloss.NewStyle().Render("  →  "), ReclaimStyle.Render(truncatePlain(item.Reclaimable, maxInt(8, m.note/2))))
		}
		if item.KeepPolicy != "" && m.note > 0 {
			parts = append(parts, lipgloss.NewStyle().Render(tuiIndent), VerCurrentStyle.Render(truncatePlain("("+item.KeepPolicy+")", m.note)))
		}
		return joinRow(parts...)
	case model.StatusCleaning:
		return joinRow(namePlain, lipgloss.NewStyle().Render(tuiIndent), SpinnerStyle.Render(s.spinnerGlyph()+" cleaning..."))
	case model.StatusCleaned:
		msg := "✓ cleaned"
		if item.Freed != "" && item.Freed != "0B" {
			msg = "✓ freed " + item.Freed
		} else if item.Freed == "0B" {
			msg = "✓ nothing removed"
		}
		return joinRow(namePlain, lipgloss.NewStyle().Render(tuiIndent), ItemOKStyle.Render(msg))
	case model.StatusError:
		return joinRow(namePlain, lipgloss.NewStyle().Render(tuiIndent), ItemErrorStyle.Render("✘ failed"))
	case model.StatusInfo:
		return joinRow(namePlain, lipgloss.NewStyle().Render(tuiIndent), VerCurrentStyle.Render("ⓘ "+truncatePlain(item.CurrentVer, m.ver*2)))
	case model.StatusUnverified:
		return joinRow(namePlain, lipgloss.NewStyle().Render(tuiIndent), ItemErrorStyle.Render("⚠ unverified — "+truncatePlain(item.CurrentVer, m.ver*2)))
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
		return lipgloss.NewStyle().Foreground(ColorGray).Render("[" + strings.Repeat(tuiHorizontal, width) + "]")
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
		mid = lipgloss.NewStyle().Foreground(ColorGray).Render(strings.Repeat(tuiHorizontal, width))
	default:
		mid = lipgloss.NewStyle().Foreground(ColorGreen).Render(strings.Repeat("█", filled)) +
			lipgloss.NewStyle().Foreground(ColorGray).Render(strings.Repeat(tuiHorizontal, width-filled))
	}
	return open + mid + closeB
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

	if s.AppliedFilter != "" {
		hints = append(hints, fmt.Sprintf("[filter: %s]", s.AppliedFilter))
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
			lipgloss.NewStyle().Render(tuiIndent),
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
			lipgloss.NewStyle().Render(tuiIndent),
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
		return style.Render(tuiSpace + msg)
	}
	return ""
}

// renderPassword shows the sudo password prompt (reused across elevated commands).
// renderDialogHeader returns the common top portion of a full-screen dialog.
func renderDialogHeader(icon, title string) string {
	var b strings.Builder
	b.WriteString(tuiNewline)
	b.WriteString(ConfirmStyle.Render(fmt.Sprintf(" %s %s", icon, title)))
	b.WriteString(tuiBlankLine)
	return b.String()
}

// renderDialogFooter renders a one-line footer hint with the standard style.
func renderDialogFooter(hint string) string {
	return FooterStyle.Render(hint)
}

func (s *State) renderPassword() string {
	var b strings.Builder
	b.WriteString(renderDialogHeader("🔐", "Administrator password required"))
	b.WriteString(" Your Mac login password (for sudo). MAS uses the system sudo cache —\n")
	b.WriteString(" asked right before App Store updates, not during long brew downloads.\n\n")
	masked := strings.Repeat("•", len(s.PasswordInput))
	b.WriteString(ButtonStyle.Render(tuiSpace) + masked + "_")
	if s.PasswordError != "" {
		b.WriteString(tuiBlankLine)
		b.WriteString(ItemErrorStyle.Render(" ✘ " + s.PasswordError))
	}
	b.WriteString(tuiNewline)
	return b.String()
}

func (s *State) renderPasswordFooter() string {
	return renderDialogFooter("[Enter] submit  [Esc] cancel")
}

// renderConfirm shows the confirmation dialog for destructive actions.
func (s *State) renderConfirm() string {
	var b strings.Builder
	b.WriteString(renderDialogHeader("⚠", strings.SplitN(s.ConfirmMsg, "\n", 2)[0]))
	lines := strings.Split(s.ConfirmMsg, "\n")
	for i, line := range lines {
		if i == 0 {
			continue // rendered in header
		}
		b.WriteString(VerCurrentStyle.Render(truncatePlain(line, s.contentWidth()-4)))
		b.WriteString(tuiNewline)
	}
	b.WriteString(tuiNewline)
	b.WriteString(ButtonStyle.Render(" Y") + "  yes  ")
	b.WriteString(ButtonStyle.Render(" N") + "  no")
	b.WriteString(tuiNewline)
	return b.String()
}

// renderConfirmFooter shows key hints during confirmation.
func (s *State) renderConfirmFooter() string {
	return renderDialogFooter("[Y] yes  [N] no  [Esc] cancel")
}

// hasCleanupItems checks if a summary has any cleanup candidates.
// StatusError is included so scan failures stay visible on the Cleanup tab
// (isCleanupNavigable also lists them — the two must stay in sync).
func hasCleanupItems(s *model.SourceSummary) bool {
	for _, it := range s.Items {
		switch it.Status {
		case model.StatusCleanCandidate, model.StatusCleaning, model.StatusCleaned, model.StatusError, model.StatusInfo, model.StatusUnverified:
			return true
		}
	}
	return false
}

// helpBinding is one keyboard shortcut shown on the help screen.
type helpBinding struct {
	keys string
	desc string
}

// helpSections returns the help content grouped by context.
func helpSections() [][]helpBinding {
	return [][]helpBinding{
		{
			{keys: "↑ / k, ↓ / j", desc: "Move cursor up/down"},
			{keys: "PgUp / PgDown", desc: "Jump one page"},
			{keys: "Home / End", desc: "Jump to first/last item"},
		},
		{
			{keys: "1 / 2 / 3", desc: "Switch tab (Updates / Cleanup / Logs)"},
			{keys: "Space", desc: "Toggle selection"},
			{keys: "* / -", desc: "Select all / none in current tab"},
			{keys: ".", desc: "Select all in current category"},
		},
		{
			{keys: "U", desc: "Update selected items"},
			{keys: "A", desc: "Update all (Updates tab) / Clean all (Cleanup tab)"},
			{keys: "C", desc: "Clean selected items"},
		},
		{
			{keys: "/", desc: "Filter items by name"},
			{keys: "Enter", desc: "Show item details / output"},
			{keys: "Esc", desc: "Cancel filter, dialog, or running operation"},
			{keys: "R", desc: "Refresh scan"},
		},
		{
			{keys: "Q / Ctrl+C", desc: "Quit updash"},
		},
	}
}

func (s *State) renderHelp() string {
	var b strings.Builder
	b.WriteString(renderDialogHeader("⌨", "Keyboard shortcuts"))

	cw := s.contentWidth()
	for _, section := range helpSections() {
		for _, binding := range section {
			keyLine := ButtonStyle.Render(padRight(binding.keys, 18))
			descLine := VerCurrentStyle.Render(truncatePlain(binding.desc, cw-22))
			b.WriteString(joinRow(lipgloss.NewStyle().Render(tuiIndent), keyLine, lipgloss.NewStyle().Render(tuiIndent), descLine))
			b.WriteString(tuiNewline)
		}
		b.WriteString(tuiNewline)
	}
	return b.String()
}

func (s *State) renderHelpFooter() string {
	return renderDialogFooter("[Esc] close help")
}

func (s *State) renderFilter() string {
	prompt := "/ " + s.FilterInput + "_"
	return ConfirmStyle.Render(truncatePlain(prompt, s.contentWidth()-2))
}

func (s *State) renderDetail() string {
	if s.DetailItem == nil {
		return ""
	}
	it := s.DetailItem
	var b strings.Builder
	b.WriteString(renderDialogHeader("📋", it.Name))

	cw := s.contentWidth() - 4
	rows := []struct{ label, value string }{
		{"Category", string(it.Category)},
		{"Status", it.Status.String()},
		{"Current", it.CurrentVer},
		{"Available", it.AvailableVer},
	}
	if it.Reclaimable != "" {
		rows = append(rows, struct{ label, value string }{"Reclaimable", it.Reclaimable})
	}
	if it.KeepPolicy != "" {
		rows = append(rows, struct{ label, value string }{"Policy", it.KeepPolicy})
	}
	if it.PackageID != "" {
		rows = append(rows, struct{ label, value string }{"Package ID", it.PackageID})
	}

	for _, r := range rows {
		if r.value == "" {
			continue
		}
		line := fmt.Sprintf("  %s: %s", r.label, r.value)
		b.WriteString(VerCurrentStyle.Render(truncatePlain(line, cw)))
		b.WriteString(tuiNewline)
	}

	if it.Log != "" {
		b.WriteString(tuiNewline)
		b.WriteString(VerCurrentStyle.Render("  Last output:"))
		b.WriteString(tuiNewline)
		for _, line := range strings.Split(it.Log, "\n") {
			if strings.TrimSpace(line) == "" {
				continue
			}
			b.WriteString(VerCurrentStyle.Render(truncatePlain("    "+line, cw)))
			b.WriteString(tuiNewline)
		}
	}
	return b.String()
}

func (s *State) renderDetailFooter() string {
	return renderDialogFooter("[Enter/Esc] close details")
}
