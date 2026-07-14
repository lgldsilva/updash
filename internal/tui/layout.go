package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

const defaultContentWidth = 72

// contentWidth returns the usable inner width (excluding border + padding).
func (s *State) contentWidth() int {
	if s.Width <= 0 {
		return defaultContentWidth
	}
	inner := s.Width - 8
	if inner < 32 {
		return 32
	}
	return inner
}

// maxListLines estimates how many item rows fit in the terminal.
func (s *State) maxListLines() int {
	if s.Height <= 0 {
		return 40
	}
	lines := s.Height - 12
	if lines < 6 {
		return 6
	}
	return lines
}

// truncatePlain shortens plain text to max visible columns.
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
		w := runewidth.RuneWidth(r)
		if width+w > max-1 {
			break
		}
		b.WriteRune(r)
		width += w
	}
	b.WriteRune('…')
	return b.String()
}

// truncateStyled shortens a lipgloss-rendered string to max visible columns.
func truncateStyled(text string, max int) string {
	if max <= 0 {
		return text
	}
	if lipgloss.Width(text) <= max {
		return text
	}
	return lipgloss.NewStyle().MaxWidth(max).Render(text)
}

// joinRow joins styled segments without nesting Render calls.
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

// frame renders content inside the app border with a stable width.
func (s *State) frame(content string) string {
	w := s.contentWidth()
	style := AppStyle
	if w > 0 {
		style = AppStyle.Width(w + 4).MaxWidth(w + 4)
	}
	return style.Render(content)
}

// wrapFooter splits long footer hints across lines.
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
