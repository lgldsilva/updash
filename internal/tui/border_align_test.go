package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/lgldsilva/updash/internal/model"
	"github.com/mattn/go-runewidth"
)

func TestRightBorder_alignsWithCorners(t *testing.T) {
	for _, term := range []int{40, 60, 80, 120, 200, 317} {
		s := New()
		s.Width = term
		s.Height = 40
		s.Ready = true
		s.Version = "test"
		s.Platform.OS = "darwin"
		s.Summaries = []*model.SourceSummary{{
			Category: model.CatBrew, Icon: "🍺", Label: "Homebrew", Total: 3,
			Items: []*model.Item{
				{Name: "clion", CurrentVer: "1.0", AvailableVer: "2.0", Status: model.StatusOutdated, KeepPolicy: "toolbox"},
				{Name: "svt-av1", CurrentVer: "4.1.0", AvailableVer: "4.2.0", Status: model.StatusOutdated, Selected: true},
			},
		}}
		s.Cursor = 1
		out := s.Render()
		boxW := s.boxWidth()
		lines := strings.Split(out, "\n")
		if len(lines) < 3 {
			t.Fatalf("term=%d too few lines", term)
		}
		for i, line := range lines {
			if line == "" {
				continue
			}
			w := lipgloss.Width(line)
			if w != boxW {
				t.Errorf("term=%d boxW=%d L%d width=%d plain=%q", term, boxW, i, w, stripANSI(line))
			}
			plain := stripANSI(line)
			runes := []rune(plain)
			if len(runes) == 0 {
				continue
			}
			r := runes[len(runes)-1]
			switch {
			case i == 0:
				if r != '╮' {
					t.Errorf("term=%d top last=%q", term, string(r))
				}
			case i == len(lines)-1:
				if r != '╯' {
					t.Errorf("term=%d bottom last=%q", term, string(r))
				}
			default:
				if r != '│' {
					t.Errorf("term=%d L%d last=%q want │", term, i, string(r))
				}
			}
		}
		// Exactly 2 │ on content rows (left+right) — no stray mid-line border
		for _, line := range lines {
			p := stripANSI(line)
			if strings.Contains(p, "clion") || strings.Contains(p, "svt-av1") {
				if n := strings.Count(p, "│"); n != 2 {
					t.Errorf("term=%d content has %d │ (stray border?): %q", term, n, p)
				}
			}
		}
		// No double right border (the bug from padding after │)
		for _, line := range lines {
			p := stripANSI(line)
			if strings.Contains(p, "││") {
				t.Errorf("term=%d double border ││ in %q", term, p)
			}
		}
		// box fills terminal minus safety (unless UPDASH_MAX_WIDTH set)
		want := term - layoutSafety()
		if want < minContentWidth+frameBorderX+framePadX*2 {
			want = minContentWidth + frameBorderX + framePadX*2
			if want > term {
				want = term
			}
		}
		if maxFrameWidth() > 0 && want > maxFrameWidth() {
			want = maxFrameWidth()
		}
		if boxW != want {
			t.Errorf("term=%d boxW=%d want %d", term, boxW, want)
		}
	}
}

func TestNoDoubleRightBorder(t *testing.T) {
	// Regression: fitLine/ensureBox used to pad AFTER │ then re-append │ → ││
	s := New()
	s.Width = 80
	s.Height = 24
	s.Ready = true
	s.Platform.OS = "darwin"
	s.Summaries = []*model.SourceSummary{{
		Category: model.CatBrew, Icon: "🍺", Label: "Homebrew", Total: 1,
		Items: []*model.Item{
			{Name: "x", CurrentVer: "1", AvailableVer: "2", Status: model.StatusOutdated},
		},
	}}
	out := s.Render()
	for i, line := range strings.Split(out, "\n") {
		p := stripANSI(line)
		if strings.Contains(p, "││") {
			t.Fatalf("L%d has ││: %q", i, p)
		}
		if p == "" {
			continue
		}
		// last rune must be a single border glyph
		r := []rune(p)
		last := r[len(r)-1]
		if last != '│' && last != '╮' && last != '╯' {
			t.Fatalf("L%d last=%q %q", i, string(last), p)
		}
		// must not end with "│ " (space after border)
		if strings.HasSuffix(p, "│ ") || strings.HasSuffix(p, "╮ ") || strings.HasSuffix(p, "╯ ") {
			t.Fatalf("L%d space after border: %q", i, p)
		}
	}
}

func TestIconCell_width2(t *testing.T) {
	for _, icon := range []string{"🍺", "🤖", "⚙", "⚙️", "x", ""} {
		c := iconCell(icon)
		rw := runewidth.StringWidth(c)
		lw := lipgloss.Width(c)
		if rw != 2 {
			t.Errorf("iconCell(%q)=%q runewidth=%d", icon, c, rw)
		}
		if lw != rw {
			t.Errorf("iconCell(%q)=%q lipgloss=%d runewidth=%d (disagree → border drift)", icon, c, lw, rw)
		}
	}
}

func TestGearEmoji_noWidthDisagreement(t *testing.T) {
	// Regression: "⚙️" was lipgloss=2 runewidth=1 → right border 1 col early
	s := New()
	s.Width = 200
	s.Height = 40
	s.Ready = true
	s.Platform.OS = "darwin"
	s.Summaries = []*model.SourceSummary{
		{Category: model.CatAI, Icon: "⚙️", Label: "AI Infra", Total: 4,
			Items: []*model.Item{{Name: "b", Status: model.StatusOK}}},
	}
	out := s.Render()
	boxW := s.boxWidth()
	for i, line := range strings.Split(out, "\n") {
		if line == "" {
			continue
		}
		p := stripANSI(line)
		if rw := runewidth.StringWidth(p); rw != boxW {
			t.Errorf("L%d plainWidth=%d boxW=%d %q", i, rw, boxW, p[:min(60, len(p))])
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
