package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/lgldsilva/updash/internal/model"
	"github.com/mattn/go-runewidth"
)

func TestTruncatePlain(t *testing.T) {
	got := truncatePlain("hello world", 8)
	if lipgloss.Width(got) > 8 {
		t.Fatalf("truncatePlain too wide: %q", got)
	}
	if !strings.HasSuffix(got, "…") {
		t.Fatalf("expected ellipsis, got %q", got)
	}
}

func TestWrapFooter(t *testing.T) {
	hints := []string{
		"[↑↓] navigate",
		"[Space] toggle",
		"[U] update selected",
		"[A] update all",
		"[R] refresh",
		"[1/2/3] tab",
		"[Q] quit",
	}
	out := wrapFooter(hints, 40)
	if out == "" {
		t.Fatal("wrapFooter returned empty")
	}
	if !strings.Contains(out, "navigate") {
		t.Fatalf("missing content: %q", out)
	}
}

func TestFrameUsesWidth(t *testing.T) {
	s := New()
	s.Width = 60
	out := s.frame("hello")
	if out == "" {
		t.Fatal("frame returned empty")
	}
}

func TestFrame_neverExceedsTerminalWidth(t *testing.T) {
	// Wide content + emoji: every painted line must be EXACTLY boxW cells.
	for _, term := range []int{60, 80, 100, 120, 317} {
		s := New()
		s.Width = term
		s.Height = 30
		s.Ready = true
		s.Version = "0.3.0"
		s.LatestTag = "v0.3.0"
		s.Platform.OS = "darwin"
		s.Summaries = []*model.SourceSummary{
			{
				Category: model.CatBrew, Icon: "🍺", Label: "Homebrew", Total: 11,
				Items: []*model.Item{
					{
						Name: "microsoft-office", CurrentVer: "16.110.26062818",
						AvailableVer: "16.111.26071325", Status: model.StatusOutdated,
						KeepPolicy: "PKG Microsoft — precisa de senha de admin no Terminal",
					},
					{
						Name: "google-chrome", CurrentVer: "150.0.7871.115",
						AvailableVer: "150.0.7871.125", Status: model.StatusOutdated,
					},
					{
						Name: "clion", CurrentVer: "2023.1.3,231.9011.31",
						AvailableVer: "2026.1.4,261.26222.59", Status: model.StatusOutdated,
						KeepPolicy: "gerido pelo JetBrains Toolbox",
						Selected:   true,
					},
				},
			},
		}
		s.Cursor = 2
		out := s.Render()
		boxW := s.boxWidth()
		for i, line := range strings.Split(out, "\n") {
			if line == "" {
				continue
			}
			w := lipgloss.Width(line)
			if w != boxW {
				t.Fatalf("term=%d boxW=%d line %d width=%d\nplain=%q",
					term, boxW, i, w, stripANSI(line))
			}
		}
		first := stripANSI(strings.Split(out, "\n")[0])
		if !strings.HasPrefix(first, "╭") || !strings.HasSuffix(strings.TrimRight(first, " "), "╮") {
			t.Fatalf("term=%d bad top border: %q", term, first)
		}
	}
}

func TestFitLine_exactWidth(t *testing.T) {
	in := "🍺 Homebrew  hello"
	for _, max := range []int{10, 20, 40} {
		got := fitLine(in, max)
		if visibleWidth(got) != max {
			t.Fatalf("fitLine max=%d got width=%d %q", max, visibleWidth(got), stripANSI(got))
		}
	}
	// Already-styled
	styled := CatLabelStyle.Render(" 🍺 Homebrew ")
	got := fitLine(styled, 16)
	if visibleWidth(got) != 16 {
		t.Fatalf("styled fitLine width=%d", visibleWidth(got))
	}
}

func stripANSI(s string) string {
	var b strings.Builder
	inEsc := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == 0x1b {
			inEsc = true
			continue
		}
		if inEsc {
			if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') {
				inEsc = false
			}
			continue
		}
		b.WriteByte(c)
	}
	return b.String()
}

func TestPadRightLeft(t *testing.T) {
	got := padRight("git", 8)
	if runewidth.StringWidth(got) != 8 {
		t.Fatalf("padRight width=%d want 8 (%q)", runewidth.StringWidth(got), got)
	}
	got = padLeft("12", 5)
	if runewidth.StringWidth(got) != 5 || !strings.HasSuffix(got, "12") {
		t.Fatalf("padLeft=%q", got)
	}
	// truncation
	got = padRight("abcdefghij", 5)
	if runewidth.StringWidth(got) != 5 {
		t.Fatalf("truncate padRight width=%d", runewidth.StringWidth(got))
	}
}

func TestCategoryHeader_fixedColumns(t *testing.T) {
	s := New()
	s.Width = 100
	h1 := s.renderCategoryHeader(&model.SourceSummary{
		Icon: "🍺", Label: "Homebrew", Total: 11,
		Items: []*model.Item{
			{Status: model.StatusOutdated}, {Status: model.StatusOutdated},
		},
	})
	h2 := s.renderCategoryHeader(&model.SourceSummary{
		Icon: "🤖", Label: "AI", Total: 4,
		Items: []*model.Item{
			{Status: model.StatusOK}, {Status: model.StatusOK},
			{Status: model.StatusOK}, {Status: model.StatusOK},
		},
	})
	// Both headers should contain bracketed bars of equal structure
	if !strings.Contains(h1, "[") || !strings.Contains(h2, "[") {
		t.Fatalf("missing bars:\n%s\n%s", h1, h2)
	}
	if !strings.Contains(h1, "outdated") {
		t.Fatalf("expected outdated count: %s", h1)
	}
}

func TestRenderItemStyled_versionColumns(t *testing.T) {
	s := New()
	s.Width = 120
	a := s.renderItemStyled(&model.Item{
		Name: "clion", CurrentVer: "1.0", AvailableVer: "2.0",
		Status: model.StatusOutdated, KeepPolicy: "toolbox",
	})
	b := s.renderItemStyled(&model.Item{
		Name: "google-chrome", CurrentVer: "150.0.1", AvailableVer: "150.0.2",
		Status: model.StatusOutdated,
	})
	// Both should contain arrow; names padded so arrow appears after fixed name col
	if !strings.Contains(a, "→") || !strings.Contains(b, "→") {
		t.Fatalf("missing arrows:\n%s\n%s", a, b)
	}
	m := s.metrics()
	if lipgloss.Width(a) < m.name || lipgloss.Width(b) < m.name {
		t.Fatalf("rows too narrow: %d %d (name col %d)", lipgloss.Width(a), lipgloss.Width(b), m.name)
	}
}

func TestMetrics_scalesWithWidth(t *testing.T) {
	narrow := New()
	narrow.Width = 48
	wide := New()
	wide.Width = 160

	n := narrow.metrics()
	w := wide.metrics()

	if n.width >= w.width {
		t.Fatalf("narrow width %d should be < wide %d", n.width, w.width)
	}
	if w.name <= n.name && w.ver <= n.ver && w.bar <= n.bar {
		t.Fatalf("wide metrics should grow: narrow=%+v wide=%+v", n, w)
	}
	// bars and names stay within clamps
	if w.bar > 28 || n.bar < 6 {
		t.Fatalf("bar out of clamp: n=%d w=%d", n.bar, w.bar)
	}
	if w.name > 36 || n.name < 8 {
		t.Fatalf("name out of clamp: n=%d w=%d", n.name, w.name)
	}
	// columns must never exceed content width
	itemBudget := n.gutter + n.name + n.ver*2 + 7 + n.note
	if itemBudget > n.width+1 { // +1 tolerance
		t.Fatalf("narrow item budget %d > width %d", itemBudget, n.width)
	}
}

func TestFormatRow_padsToContentWidth(t *testing.T) {
	s := New()
	s.Width = 80
	s.Cursor = 0
	row := s.formatRow("short", 0)
	if lipgloss.Width(row) != s.contentWidth() {
		t.Fatalf("padded row width=%d want %d", lipgloss.Width(row), s.contentWidth())
	}
}
