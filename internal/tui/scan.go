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
// startScan launches a background scan without blocking the Bubble Tea event loop.
//
// Divergence from scanner.RunAll is intentional: the TUI needs one message
// per source for incremental progress, so it orchestrates the sources itself
// (bounded by maxConcurrentScans) instead of the CLI's unbounded RunAll.
func (s *State) startScan() tea.Cmd {
	// A rescan while an update/clean batch is running would swap the very
	// summaries those workers are mutating — refuse it with a visible hint.
	if s.Scanning || s.Updating || s.Cleaning || s.Program == nil {
		if s.Updating || s.Cleaning {
			s.LastSummary = "⚠ Refresh skipped — operation in progress (Esc cancels)"
			s.AddLog("Refresh ignored: another operation is running", false)
		}
		return nil
	}

	s.Scanning = true
	generation := s.nextGeneration()
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

				summary := scanner.ScanSource(ctx, src, plat)
				isCleanup := scanner.IsCleanupCategory(summary.Category)

				program.Send(ScanSourceDoneMsg{
					Generation: generation,
					Summary:    summary,
					IsCleanup:  isCleanup,
				})
			}()
		}

		wg.Wait()
		program.Send(ScanFinishedMsg{Generation: generation, Elapsed: time.Since(start).Round(time.Millisecond)})
	}()

	return nil // spinner animation runs on the single tick chain (see onTick)
}

// rescanCategory re-probes one package manager and pushes a fresh summary
// to the TUI. Runs on worker goroutines: it must only use the caller's
// snapshot (ctx/plat), never State — the event loop may be swapping fields.
func rescanCategory(ctx context.Context, plat model.PlatformInfo, program *tea.Program, generation uint64, cat model.Category, cleanup bool) {
	if program == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	for _, src := range scanner.EnabledSources(plat, cleanup || scanner.IsCleanupCategory(cat)) {
		if src.Category() != cat {
			continue
		}
		if cleanup != scanner.IsCleanupCategory(src.Category()) {
			continue
		}
		summary := scanner.ScanSource(ctx, src, plat)
		program.Send(ScanSourceDoneMsg{
			Generation: generation,
			Summary:    summary,
			IsCleanup:  scanner.IsCleanupCategory(summary.Category),
			Rescan:     true,
		})
	}
}
