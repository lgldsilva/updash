package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

func TestEmoji_vs_border(t *testing.T) {
	term := 80
	s := New()
	s.Width = term
	// Simulate frame top vs line with emoji
	top := "╭" + strings.Repeat("─", term-2) + "╮"
	emojiLine := "🍺 Homebrew"
	cw := term - 4 - 1                         // border2+pad2+safety1
	body := " " + fitLine(emojiLine, cw) + " " // rough
	// actual frame path
	out := s.frame("🍺 Homebrew test\nplain line without emoji")
	for i, line := range strings.Split(out, "\n") {
		if line == "" {
			continue
		}
		p := stripANSI(line)
		fmt.Printf("L%d w=%d rw=%d last=%q first=%q\n", i, lipgloss.Width(line), runewidth.StringWidth(p), string([]rune(p)[len([]rune(p))-1]), string([]rune(p)[0]))
	}
	_ = top
	_ = body
}
