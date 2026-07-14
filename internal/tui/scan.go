package tui

import (
	"context"
	"sort"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lgldsilva/updash/internal/model"
	"github.com/lgldsilva/updash/internal/scanner"
)

const maxConcurrentScans = 6

// SetProgram stores the Bubble Tea program for async Send() from background work.
func (s *State) SetProgram(p *tea.Program) {
	s.Program = p
}

// MergeSummary inserts or replaces a source summary by category.
func MergeSummary(list []*model.SourceSummary, sum *model.SourceSummary) []*model.SourceSummary {
	for i, existing := range list {
		if existing.Category == sum.Category && existing.Label == sum.Label {
			list[i] = sum
			return list
		}
	}
	list = append(list, sum)
	sort.Slice(list, func(i, j int) bool {
		if list[i].Category == list[j].Category {
			return list[i].Label < list[j].Label
		}
		return list[i].Category < list[j].Category
	})
	return list
}

// startScan launches a background scan without blocking the Bubble Tea event loop.
func (s *State) startScan() tea.Cmd {
	if s.Scanning || s.Program == nil {
		return nil
	}

	s.Scanning = true
	s.ScanDone = 0
	s.LastSummary = ""
	s.OperationLabel = "system"

	sources := scanner.EnabledSources(s.Platform, true)
	s.ScanTotal = len(sources)

	program := s.Program
	ctx := s.Ctx
	plat := s.Platform

	go func() {
		start := time.Now()
		sem := make(chan struct{}, maxConcurrentScans)
		var wg sync.WaitGroup

		for _, src := range sources {
			wg.Add(1)
			src := src
			go func() {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()

				if ctx.Err() != nil {
					return
				}

				summary := scanner.ScanSource(ctx, src, plat)
				isCleanup := scanner.IsCleanupCategory(summary.Category)

				program.Send(ScanSourceDoneMsg{
					Summary:   summary,
					IsCleanup: isCleanup,
				})
			}()
		}

		wg.Wait()
		program.Send(ScanFinishedMsg{Elapsed: time.Since(start).Round(time.Millisecond)})
	}()

	return TickCmd()
}

// rescanCategory re-probes one package manager and pushes fresh summary to the TUI.
// Uses s.Ctx (not the caller's batch ctx) so rescans still run after batch timeouts cancel.
func (s *State) rescanCategory(_ context.Context, program *tea.Program, cat model.Category, cleanup bool) {
	if program == nil {
		return
	}
	scanCtx := s.Ctx
	if scanCtx == nil {
		scanCtx = context.Background()
	}
	for _, src := range scanner.EnabledSources(s.Platform, cleanup || scanner.IsCleanupCategory(cat)) {
		if src.Category() != cat {
			continue
		}
		if cleanup != scanner.IsCleanupCategory(src.Category()) {
			continue
		}
		summary := scanner.ScanSource(scanCtx, src, s.Platform)
		program.Send(ScanSourceDoneMsg{
			Summary:   summary,
			IsCleanup: scanner.IsCleanupCategory(summary.Category),
		})
	}
}
