package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/lgldsilva/updash/internal/model"
	"github.com/mattn/go-runewidth"
)

func sampleBrewState(term int) *State {
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
	return s
}

func expectedBoxWidth(term int) int {
	want := term - layoutSafety()
	minBox := minContentWidth + frameBorderX + framePadX*2
	if want < minBox {
		want = minBox
		if want > term {
			want = term
		}
	}
	if max := maxFrameWidth(); max > 0 && want > max {
		want = max
	}
	return want
}

func lastRune(plain string) rune {
	r := []rune(plain)
	if len(r) == 0 {
		return 0
	}
	return r[len(r)-1]
}

func assertFrameLine(t *testing.T, term, boxW, i, nLines int, line string) {
	t.Helper()
	if line == "" {
		return
	}
	if lipgloss.Width(line) != boxW {
		t.Errorf("term=%d L%d width=%d want %d", term, i, lipgloss.Width(line), boxW)
	}
	plain := stripANSI(line)
	r := lastRune(plain)
	switch {
	case i == 0 && r != '╮':
		t.Errorf("term=%d top last=%q", term, string(r))
	case i == nLines-1 && r != '╯':
		t.Errorf("term=%d bottom last=%q", term, string(r))
	case i > 0 && i < nLines-1 && r != '│':
		t.Errorf("term=%d L%d last=%q want │", term, i, string(r))
	}
}

func assertNoDoubleBorder(t *testing.T, term int, lines []string) {
	t.Helper()
	for _, line := range lines {
		p := stripANSI(line)
		if strings.Contains(p, "││") {
			t.Errorf("term=%d double border in %q", term, p)
		}
		if strings.Contains(p, "clion") || strings.Contains(p, "svt-av1") {
			if n := strings.Count(p, "│"); n != 2 {
				t.Errorf("term=%d content has %d │: %q", term, n, p)
			}
		}
	}
}

func TestRightBorder_alignsWithCorners(t *testing.T) {
	for _, term := range []int{40, 60, 80, 120, 200, 317} {
		s := sampleBrewState(term)
		out := s.Render()
		boxW := s.boxWidth()
		lines := strings.Split(out, "\n")
		if len(lines) < 3 {
			t.Fatalf("term=%d too few lines", term)
		}
		for i, line := range lines {
			assertFrameLine(t, term, boxW, i, len(lines), line)
		}
		assertNoDoubleBorder(t, term, lines)
		if boxW != expectedBoxWidth(term) {
			t.Errorf("term=%d boxW=%d want %d", term, boxW, expectedBoxWidth(term))
		}
	}
}

func TestNoDoubleRightBorder(t *testing.T) {
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
	for i, line := range strings.Split(s.Render(), "\n") {
		p := stripANSI(line)
		if strings.Contains(p, "││") {
			t.Fatalf("L%d has ││: %q", i, p)
		}
		if p == "" {
			continue
		}
		last := lastRune(p)
		if last != '│' && last != '╮' && last != '╯' {
			t.Fatalf("L%d last=%q %q", i, string(last), p)
		}
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
			t.Errorf("iconCell(%q) lipgloss=%d runewidth=%d", icon, lw, rw)
		}
	}
}

func TestGearEmoji_noWidthDisagreement(t *testing.T) {
	s := New()
	s.Width = 200
	s.Height = 40
	s.Ready = true
	s.Platform.OS = "darwin"
	s.Summaries = []*model.SourceSummary{
		{Category: model.CatAI, Icon: "⚙️", Label: "AI Infra", Total: 4,
			Items: []*model.Item{{Name: "b", Status: model.StatusOK}}},
	}
	boxW := s.boxWidth()
	for i, line := range strings.Split(s.Render(), "\n") {
		if line == "" {
			continue
		}
		p := stripANSI(line)
		if rw := runewidth.StringWidth(p); rw != boxW {
			t.Errorf("L%d plainWidth=%d boxW=%d", i, rw, boxW)
		}
	}
}
