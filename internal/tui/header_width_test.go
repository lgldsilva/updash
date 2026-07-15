package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/lgldsilva/updash/internal/model"
	"github.com/mattn/go-runewidth"
)

func TestCategoryHeader_frameAlignment(t *testing.T) {
	s := New()
	s.Width = 200
	s.Height = 40
	s.Ready = true
	s.Platform.OS = "darwin"
	s.Summaries = []*model.SourceSummary{
		{Category: model.CatAgent, Icon: "🤖", Label: "AI Agents", Total: 10,
			Items: []*model.Item{{Name: "a", Status: model.StatusOK}}},
		{Category: model.CatAI, Icon: "⚙️", Label: "AI Infra", Total: 4,
			Items: []*model.Item{{Name: "b", Status: model.StatusOK}}},
		{Category: model.CatBrew, Icon: "🍺", Label: "Homebrew", Total: 9,
			Items: []*model.Item{{Name: "c", CurrentVer: "1", AvailableVer: "2", Status: model.StatusOutdated}}},
	}
	out := s.Render()
	boxW := s.boxWidth()
	fmt.Printf("term=%d boxW=%d cw=%d\n", s.Width, boxW, s.contentWidth())
	for i, line := range strings.Split(out, "\n") {
		if line == "" {
			continue
		}
		p := stripANSI(line)
		lw := lipgloss.Width(line)
		rw := runewidth.StringWidth(p)
		ok := "OK"
		if lw != boxW || rw != boxW {
			ok = "BAD"
		}
		// show ends
		end := p
		if len([]rune(p)) > 30 {
			r := []rune(p)
			end = string(r[len(r)-15:])
		}
		start := p
		if len([]rune(p)) > 40 {
			start = string([]rune(p)[:40])
		}
		if strings.Contains(p, "AI Agents") || strings.Contains(p, "AI Infra") || strings.Contains(p, "Homebrew") || i < 3 || i > len(strings.Split(out, "\n"))-4 {
			fmt.Printf("L%02d %s lip=%d rw=%d box=%d | %q … %q\n", i, ok, lw, rw, boxW, start, end)
		}
		if strings.Contains(p, "││") {
			t.Errorf("double pipe L%d", i)
		}
	}
}
