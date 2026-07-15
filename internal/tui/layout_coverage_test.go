package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/lgldsilva/updash/internal/model"
)

func TestLayoutSafety_env(t *testing.T) {
	t.Setenv("UPDASH_LAYOUT_SAFETY", "3")
	if layoutSafety() != 3 {
		t.Fatalf("want 3 got %d", layoutSafety())
	}
	t.Setenv("UPDASH_LAYOUT_SAFETY", "nope")
	if layoutSafety() != defaultLayoutSafety {
		t.Fatalf("invalid should fallback default")
	}
	t.Setenv("UPDASH_LAYOUT_SAFETY", "")
	if layoutSafety() != defaultLayoutSafety {
		t.Fatalf("empty should default")
	}
}

func TestMaxFrameWidth_env(t *testing.T) {
	t.Setenv("UPDASH_MAX_WIDTH", "100")
	if maxFrameWidth() != 100 {
		t.Fatalf("want 100 got %d", maxFrameWidth())
	}
	t.Setenv("UPDASH_MAX_WIDTH", "bad")
	if maxFrameWidth() != defaultMaxFrame {
		t.Fatalf("invalid fallback")
	}
	t.Setenv("UPDASH_MAX_WIDTH", "")
	if maxFrameWidth() != defaultMaxFrame {
		t.Fatalf("empty default")
	}
}

func TestBoxWidth_capAndTiny(t *testing.T) {
	s := New()
	s.Width = 300
	t.Setenv("UPDASH_MAX_WIDTH", "90")
	t.Setenv("UPDASH_LAYOUT_SAFETY", "0")
	if s.boxWidth() != 90 {
		t.Fatalf("capped boxW=%d", s.boxWidth())
	}
	s.Width = 10 // smaller than min content+chrome
	t.Setenv("UPDASH_MAX_WIDTH", "0")
	t.Setenv("UPDASH_LAYOUT_SAFETY", "0")
	w := s.boxWidth()
	if w < 1 || w > s.Width {
		// may expand min beyond term then clamp to term
		if w != s.Width && w < minContentWidth {
			t.Fatalf("tiny boxW=%d term=%d", w, s.Width)
		}
	}
}

func TestPadLeft_and_joinRow_empty(t *testing.T) {
	if padLeft("ab", 5) != "   ab" {
		t.Fatalf("padLeft=%q", padLeft("ab", 5))
	}
	got := padLeft("abcdef", 3)
	if plainWidth(got) != 3 {
		t.Fatalf("padLeft truncate width=%d %q", plainWidth(got), got)
	}
	if joinRow() != "" {
		t.Fatal("empty join")
	}
	if joinRow("", "x", "") != "x" {
		t.Fatalf("join filter")
	}
}

func TestPaintFrameLine_and_edge(t *testing.T) {
	border := lipgloss.NewStyle().Foreground(ColorCyan)
	line := paintFrameLine("hello", 40, border)
	if plainWidth(line) != 40 {
		t.Fatalf("paintFrameLine width=%d", plainWidth(line))
	}
	if !strings.HasPrefix(stripANSI(line), "│") || !strings.HasSuffix(stripANSI(line), "│") {
		t.Fatalf("borders: %q", stripANSI(line))
	}
	// tiny
	tiny := paintFrameLine("x", 2, border)
	if plainWidth(tiny) < 1 {
		t.Fatal("tiny frame")
	}
	top := paintEdgeRow("╭", "─", "╮", 40, border)
	if plainWidth(top) != 40 || !strings.HasSuffix(stripANSI(top), "╮") {
		t.Fatalf("top=%q w=%d", stripANSI(top), plainWidth(top))
	}
	bot := paintEdgeRow("╰", "─", "╯", 2, border)
	if plainWidth(bot) < 1 {
		t.Fatal("bot tiny")
	}
}

func TestFitLine_newlineAndOver(t *testing.T) {
	got := fitLine("hello\nworld", 5)
	if strings.Contains(got, "\n") {
		t.Fatalf("newline leaked: %q", got)
	}
	if plainWidth(got) != 5 {
		t.Fatalf("width %d", plainWidth(got))
	}
	over := fitLine(strings.Repeat("x", 50), 10)
	if plainWidth(over) != 10 {
		t.Fatalf("over width %d", plainWidth(over))
	}
}

func TestTruncateStyled_and_plain(t *testing.T) {
	styled := lipgloss.NewStyle().Bold(true).Render(strings.Repeat("y", 40))
	got := truncateStyled(styled, 8)
	if visibleWidth(got) > 8 {
		t.Fatalf("truncateStyled too wide %d", visibleWidth(got))
	}
	if truncatePlain("", 5) != "" {
		t.Fatal("empty")
	}
	if truncatePlain("hi", 0) != "" {
		t.Fatal("max0")
	}
	if padRight("z", 0) != "" || padLeft("z", 0) != "" {
		t.Fatal("pad0")
	}
}

func TestLayoutDebug_writesLog(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "layout.log")
	t.Setenv("UPDASH_DEBUG_LAYOUT", "1")
	t.Setenv("UPDASH_DEBUG_LAYOUT_FILE", logPath)

	if !layoutDebugEnabled() {
		t.Fatal("debug should be on")
	}
	if layoutLogFile() != logPath {
		t.Fatalf("path %s", layoutLogFile())
	}

	// Force path used by log writers (Once may already have run in process)
	layoutLogPath = logPath
	// ensure file exists for append
	_ = os.WriteFile(logPath, []byte("# test\n"), 0o600)

	layoutLog("hello %s", "world")
	layoutLogAlways("overflow-test")

	s := New()
	s.Width = 80
	s.Height = 24
	s.LogWindowSize(80, 24)
	s.debugFrame("test", 80, 76, 80, "short")
	// overflow path
	s.debugFrame("ov", 10, 8, 10, strings.Repeat("Z", 50))

	if !hasOverflow(strings.Repeat("a", 20), 10) {
		t.Fatal("expected overflow")
	}
	if hasOverflow("ok", 10) {
		t.Fatal("no overflow")
	}

	styled := "\x1b[31mred\x1b[0m"
	if stripANSIForLog(styled) != "red" {
		t.Fatalf("strip=%q", stripANSIForLog(styled))
	}
	_ = stripANSIForLog("plain\nline\r")

	// layoutLog with debug off is a no-op
	t.Setenv("UPDASH_DEBUG_LAYOUT", "0")
	layoutLog("should-skip")
}

func TestMetrics_narrowAndWide(t *testing.T) {
	n := New()
	n.Width = 40
	t.Setenv("UPDASH_LAYOUT_SAFETY", "0")
	t.Setenv("UPDASH_MAX_WIDTH", "0")
	mn := n.metrics()
	if mn.name < 8 || mn.ver < 6 {
		t.Fatalf("narrow metrics %+v", mn)
	}
	w := New()
	w.Width = 200
	mw := w.metrics()
	if mw.note > 48 {
		t.Fatalf("note should cap at 48 got %d", mw.note)
	}
	if mw.width <= mn.width {
		t.Fatalf("wide content width should exceed narrow: n=%d w=%d", mn.width, mw.width)
	}
}

func TestTermWidth_default(t *testing.T) {
	s := New()
	s.Width = 0
	if s.termWidth() != defaultTermWidth {
		t.Fatalf("default term %d", s.termWidth())
	}
	if s.maxListLines() < 6 {
		t.Fatal("maxListLines")
	}
	s.Height = 5
	if s.maxListLines() != 6 {
		t.Fatalf("min height lines %d", s.maxListLines())
	}
	s.Height = 50
	if s.maxListLines() < 6 {
		t.Fatal("tall")
	}
}

func TestFrame_withWideContent(t *testing.T) {
	s := New()
	s.Width = 100
	s.Height = 30
	s.Ready = true
	s.Platform.OS = "darwin"
	s.Summaries = []*model.SourceSummary{{
		Category: model.CatBrew, Icon: "🍺", Label: "Homebrew", Total: 2,
		Items: []*model.Item{
			{Name: strings.Repeat("longname", 5), CurrentVer: strings.Repeat("1.", 20),
				AvailableVer: strings.Repeat("2.", 20), Status: model.StatusOutdated,
				KeepPolicy: strings.Repeat("policy ", 30)},
		},
	}}
	out := s.frame(s.renderUpdatesTab())
	boxW := s.boxWidth()
	for _, line := range strings.Split(out, "\n") {
		if line == "" {
			continue
		}
		if plainWidth(line) != boxW {
			t.Fatalf("width %d want %d %q", plainWidth(line), boxW, stripANSI(line)[:min(50, len(stripANSI(line)))])
		}
	}
}
