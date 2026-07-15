package tui

import (
	"os"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

// Manual frame geometry — every painted line is EXACTLY boxW cells:
//
//	│ + pad + content(cw) + pad + │   == boxW
//	1 +  1  +     cw      +  1  + 1
//
//	cw = boxW - 4
//
// boxW = min(termWidth - safety, maxFrameWidth)
// safety shrinks the whole box (avoids scrollbar / off-by-one at the window edge).
// maxFrameWidth keeps ultra-wide terminals from stretching a 300-col empty void.
const (
	defaultTermWidth    = 80
	minContentWidth     = 20
	framePadX           = 1
	frameBorderX        = 2
	defaultLayoutSafety = 1 // cols reserved at window edge (scrollbar)
	// 0 = use full terminal (minus safety). Set UPDASH_MAX_WIDTH to cap.
	defaultMaxFrame = 0
)

func layoutSafety() int {
	v := os.Getenv("UPDASH_LAYOUT_SAFETY")
	if v == "" {
		return defaultLayoutSafety
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return defaultLayoutSafety
	}
	return n
}

// maxFrameWidth caps the app box on ultra-wide terminals.
// 0 = no cap. Override with UPDASH_MAX_WIDTH=N.
func maxFrameWidth() int {
	v := os.Getenv("UPDASH_MAX_WIDTH")
	if v == "" {
		return defaultMaxFrame
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return defaultMaxFrame
	}
	return n
}

func (s *State) termWidth() int {
	if s.Width <= 0 {
		return defaultTermWidth
	}
	return s.Width
}

// boxWidth is the full painted width of the frame (including borders).
func (s *State) boxWidth() int {
	term := s.termWidth()
	w := term - layoutSafety()
	if w < minContentWidth+frameBorderX+framePadX*2 {
		w = minContentWidth + frameBorderX + framePadX*2
		if w > term {
			w = term
		}
	}
	if max := maxFrameWidth(); max > 0 && w > max {
		w = max
	}
	return w
}

// contentWidth is columns available inside pad+border for content text.
func (s *State) contentWidth() int {
	cw := s.boxWidth() - frameBorderX - framePadX*2
	if cw < minContentWidth {
		return minContentWidth
	}
	return cw
}

func (s *State) maxListLines() int {
	if s.Height <= 0 {
		return 40
	}
	lines := s.Height - 14
	if lines < 6 {
		return 6
	}
	return lines
}

type layoutMetrics struct {
	width    int
	gutter   int
	catLabel int
	bar      int
	name     int
	ver      int
	note     int
}

func (s *State) metrics() layoutMetrics {
	w := s.contentWidth()
	m := layoutMetrics{width: w, gutter: 4}

	m.catLabel = clampInt(w*16/100, 10, 20)
	maxBar := w - m.catLabel - 24
	if maxBar < 6 {
		maxBar = 6
	}
	m.bar = clampInt(w*10/100, 6, maxBar)
	if m.bar > 24 {
		m.bar = 24
	}

	const seps = 7
	flex := w - m.gutter - seps
	if flex < 16 {
		flex = 16
	}

	m.name = clampInt(flex*30/100, 8, 28)
	m.ver = clampInt(flex*20/100, 6, 22)
	used := m.name + m.ver*2
	if used > flex {
		for used > flex && m.ver > 6 {
			m.ver--
			used = m.name + m.ver*2
		}
		for used > flex && m.name > 8 {
			m.name--
			used = m.name + m.ver*2
		}
	}
	m.note = flex - used
	if m.note < 0 {
		m.note = 0
	}
	// Cap note so it doesn't dominate; remaining stays as trailing blank in the row
	if m.note > 48 {
		m.note = 48
	}
	return m
}

func clampInt(v, lo, hi int) int {
	if hi < lo {
		hi = lo
	}
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// visibleWidth is the display cell count we trust for layout.
//
// lipgloss and runewidth disagree on some emoji (e.g. "⚙️" = U+2699+FE0F:
// lipgloss=2, runewidth=1). macOS terminals typically follow the smaller
// advance, so using lipgloss alone made the right border sit 1 col early.
// We take max(lipgloss, runewidth) so we never under-allocate, and normalize
// icons via iconCell() so both metrics agree.
func visibleWidth(s string) int {
	a := lipgloss.Width(s)
	b := runewidth.StringWidth(stripANSIForLog(s))
	if a > b {
		return a
	}
	return b
}

// plainWidth is runewidth of ANSI-stripped text (terminal-like).
func plainWidth(s string) int {
	return runewidth.StringWidth(stripANSIForLog(s))
}

func truncatePlain(text string, max int) string {
	if max <= 0 {
		return ""
	}
	if runewidth.StringWidth(text) <= max {
		return text
	}
	if max <= 1 {
		return "…"
	}
	var b strings.Builder
	width := 0
	for _, r := range text {
		rw := runewidth.RuneWidth(r)
		if rw < 0 {
			rw = 1
		}
		if width+rw > max-1 {
			break
		}
		b.WriteRune(r)
		width += rw
	}
	b.WriteRune('…')
	return b.String()
}

func padRight(text string, width int) string {
	if width <= 0 {
		return ""
	}
	text = truncatePlain(text, width)
	pad := width - runewidth.StringWidth(text)
	if pad <= 0 {
		return text
	}
	return text + strings.Repeat(" ", pad)
}

func padLeft(text string, width int) string {
	if width <= 0 {
		return ""
	}
	text = truncatePlain(text, width)
	pad := width - runewidth.StringWidth(text)
	if pad <= 0 {
		return text
	}
	return strings.Repeat(" ", pad) + text
}

// iconCell normalizes category/item icons to exactly 2 display cells and
// strips emoji variation selectors (U+FE0F) that make lipgloss≠runewidth.
func iconCell(icon string) string {
	if icon == "" {
		return "  "
	}
	// Strip VS15/VS16, ZWJ, BOM — keep the base glyph only
	var b strings.Builder
	for _, r := range icon {
		switch {
		case r >= 0xFE00 && r <= 0xFE0F: // variation selectors
			continue
		case r == 0x200D || r == 0xFEFF: // ZWJ / BOM
			continue
		case r == 0x20E3: // combining enclosing keycap
			continue
		default:
			b.WriteRune(r)
		}
	}
	icon = b.String()
	// Prefer a single base emoji/symbol; pad to 2 cells with space
	w := runewidth.StringWidth(icon)
	lw := lipgloss.Width(icon)
	// If metrics still disagree, drop to ASCII fallback for that icon
	if w != lw {
		icon = "?"
		w = 1
	}
	switch {
	case w == 2 && lw == 2:
		return icon
	case w >= 2:
		return truncatePlain(icon, 2)
	case w == 1:
		return icon + " "
	default:
		return "  "
	}
}

func truncateStyled(text string, max int) string {
	if max <= 0 {
		return ""
	}
	if visibleWidth(text) <= max {
		return text
	}
	out := strings.TrimRight(lipgloss.NewStyle().MaxWidth(max).Render(text), "\n")
	if visibleWidth(out) <= max {
		return out
	}
	return truncatePlain(stripANSIForLog(text), max)
}

// fitLine forces a line to exactly max visible columns (ANSI-aware).
func fitLine(line string, max int) string {
	if max <= 0 {
		return ""
	}
	if i := strings.IndexByte(line, '\n'); i >= 0 {
		line = line[:i]
	}
	w := visibleWidth(line)
	switch {
	case w > max:
		return truncateStyled(line, max)
	case w < max:
		return line + strings.Repeat(" ", max-w)
	default:
		return line
	}
}

func joinRow(parts ...string) string {
	filtered := make([]string, 0, len(parts))
	for _, p := range parts {
		if p != "" {
			filtered = append(filtered, p)
		}
	}
	if len(filtered) == 0 {
		return ""
	}
	return lipgloss.JoinHorizontal(lipgloss.Left, filtered...)
}

// paintFrameLine builds one side row: │ + mid(inner) + │ with EXACTLY boxW cells.
//
// Mid is padded/truncated by plainWidth (runewidth), not lipgloss.Style.Width,
// so emoji quirks cannot leave the right border 1 col early.
// Spaces are only inserted INSIDE mid — never after the final │.
func paintFrameLine(body string, boxW int, border lipgloss.Style) string {
	if boxW < 3 {
		return "│"
	}
	inner := boxW - 2

	// Truncate/pad using plain width (matches macOS terminal advance better)
	mid := body
	for plainWidth(mid) > inner {
		// shrink by chopping plain text; keep as much ANSI as truncateStyled can
		mid = truncateStyled(mid, plainWidth(mid)-1)
		if plainWidth(mid) > inner {
			mid = truncatePlain(stripANSIForLog(mid), inner)
		}
	}
	if gap := inner - plainWidth(mid); gap > 0 {
		mid += strings.Repeat(" ", gap)
	}
	// Final plain-width check
	if plainWidth(mid) != inner {
		mid = padRight(truncatePlain(stripANSIForLog(body), inner), inner)
	}

	left := border.Render("│")
	right := border.Render("│")
	line := left + mid + right

	// Prefer plain width (terminal-like); lipgloss may still disagree on edge glyphs
	if plainWidth(line) == boxW {
		return line
	}
	// Absolute plain fallback
	return "│" + padRight(truncatePlain(stripANSIForLog(body), inner), inner) + "│"
}

// paintEdgeRow paints top or bottom border of exact boxW (no trailing pad after ╮/╯).
func paintEdgeRow(left, fill, right string, boxW int, border lipgloss.Style) string {
	if boxW < 2 {
		return border.Render(left)
	}
	n := boxW - 2
	if n < 0 {
		n = 0
	}
	raw := left + strings.Repeat(fill, n) + right
	line := border.Render(raw)
	if visibleWidth(line) == boxW {
		return line
	}
	// Unstyled exact
	return raw
}

// frame draws a rounded box of width boxWidth() spanning (almost) the full terminal.
// Right border is always the LAST cell — never "│ " or "││".
func (s *State) frame(content string) string {
	boxW := s.boxWidth()
	cw := boxW - frameBorderX - framePadX*2
	if cw < 1 {
		cw = 1
	}

	border := lipgloss.NewStyle().Foreground(ColorCyan)
	pad := strings.Repeat(" ", framePadX)
	innerW := boxW - 2

	var b strings.Builder
	b.WriteString(paintEdgeRow("╭", "─", "╮", boxW, border))
	b.WriteByte('\n')

	empty := strings.Repeat(" ", innerW)
	b.WriteString(paintFrameLine(empty, boxW, border))
	b.WriteByte('\n')

	for _, raw := range strings.Split(content, "\n") {
		// Content only (no borders here). fitLine must never run on a full frame row.
		line := fitLine(raw, cw)
		if visibleWidth(line) != cw {
			line = padRight(truncatePlain(stripANSIForLog(raw), cw), cw)
		}
		body := pad + line + pad
		b.WriteString(paintFrameLine(body, boxW, border))
		b.WriteByte('\n')
	}

	b.WriteString(paintFrameLine(empty, boxW, border))
	b.WriteByte('\n')
	b.WriteString(paintEdgeRow("╰", "─", "╯", boxW, border))

	out := b.String()
	s.debugFrame("frame", s.termWidth(), cw, boxW, out)
	return out
}

func wrapFooter(hints []string, maxWidth int) string {
	if maxWidth <= 0 {
		return FooterStyle.Render(strings.Join(hints, " · "))
	}
	var lines []string
	var current strings.Builder
	for i, hint := range hints {
		sep := ""
		if current.Len() > 0 {
			sep = " · "
		}
		candidate := current.String() + sep + hint
		if current.Len() > 0 && runewidth.StringWidth(candidate) > maxWidth {
			lines = append(lines, FooterStyle.Render(current.String()))
			current.Reset()
			current.WriteString(hint)
		} else if current.Len() == 0 {
			current.WriteString(hint)
		} else {
			current.WriteString(sep)
			current.WriteString(hint)
		}
		if i == len(hints)-1 && current.Len() > 0 {
			lines = append(lines, FooterStyle.Render(current.String()))
		}
	}
	return strings.Join(lines, "\n")
}
