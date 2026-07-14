package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
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
