package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// Layout debug is enabled with UPDASH_DEBUG_LAYOUT=1.
// Logs go to: $UPDASH_DEBUG_LAYOUT_FILE or ~/.cache/updash/layout-debug.log
//
// Every WindowSize and every Render that produces an overflowing line is recorded
// so we can see real terminal sizes vs computed chrome.

var (
	layoutLogOnce sync.Once
	layoutLogMu   sync.Mutex
	layoutLogPath string
	layoutLogN    int // frames logged (cap spam)
)

func layoutDebugEnabled() bool {
	v := os.Getenv("UPDASH_DEBUG_LAYOUT")
	return v == "1" || v == "true" || v == "yes"
}

func layoutLogFile() string {
	if p := os.Getenv("UPDASH_DEBUG_LAYOUT_FILE"); p != "" {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join(os.TempDir(), "updash-layout-debug.log")
	}
	return filepath.Join(home, ".cache", "updash", "layout-debug.log")
}

func layoutLog(format string, args ...any) {
	if !layoutDebugEnabled() {
		return
	}
	layoutLogOnce.Do(func() {
		layoutLogPath = layoutLogFile()
		_ = os.MkdirAll(filepath.Dir(layoutLogPath), 0o750)
		// truncate on first open of process (owner-only log file)
		// Path is under ~/.cache or explicit UPDASH_DEBUG_LAYOUT_FILE (debug only).
		f, err := os.OpenFile(layoutLogPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600) // #nosec G304 -- debug log path from env/home
		if err == nil {
			_, _ = fmt.Fprintf(f, "# updash layout debug started %s\n", time.Now().Format(time.RFC3339))
			_, _ = fmt.Fprintf(f, "# UPDASH_DEBUG_LAYOUT=1  file=%s\n", layoutLogPath)
			_, _ = fmt.Fprintf(f, "# columns: ts | event | details\n")
			_ = f.Close()
		}
	})
	layoutLogMu.Lock()
	defer layoutLogMu.Unlock()
	f, err := os.OpenFile(layoutLogPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600) // #nosec G304 -- debug log path from env/home
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()
	_, _ = fmt.Fprintf(f, "%s | %s\n", time.Now().Format("15:04:05.000"), fmt.Sprintf(format, args...))
}

// layoutLogAlways writes even without UPDASH_DEBUG_LAYOUT (used for overflow only).
func layoutLogAlways(format string, args ...any) {
	path := layoutLogFile()
	_ = os.MkdirAll(filepath.Dir(path), 0o750)
	layoutLogMu.Lock()
	defer layoutLogMu.Unlock()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600) // #nosec G304 -- debug log path from env/home
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()
	_, _ = fmt.Fprintf(f, "%s | OVERFLOW | %s\n", time.Now().Format("15:04:05.000"), fmt.Sprintf(format, args...))
}

// LogWindowSize records a tea.WindowSizeMsg for diagnostics.
func (s *State) LogWindowSize(w, h int) {
	if !layoutDebugEnabled() {
		return
	}
	layoutLog("WindowSize raw=%dx%d termW=%d boxW=%d contentW=%d safety=%d maxFrame=%d metrics=%+v",
		w, h, s.termWidth(), s.boxWidth(), s.contentWidth(),
		layoutSafety(), maxFrameWidth(),
		s.metrics(),
	)
}

// frameLineStats summarizes widths for debug logging.
type frameLineStats struct {
	maxW, over int
	samples    []string
}

func collectFrameStats(out string, term int) frameLineStats {
	var st frameLineStats
	for i, line := range strings.Split(out, "\n") {
		w := lipgloss.Width(line)
		if w > st.maxW {
			st.maxW = w
		}
		if w <= term {
			continue
		}
		st.over++
		if len(st.samples) >= 5 {
			continue
		}
		plain := stripANSIForLog(line)
		if len(plain) > 100 {
			plain = plain[:100] + "…"
		}
		st.samples = append(st.samples, fmt.Sprintf("L%d w=%d %q", i, w, plain))
	}
	return st
}

func shouldLogFrame(debug bool, over int) bool {
	if !debug && over == 0 {
		return false
	}
	if !debug {
		return true // overflow without debug still logs
	}
	layoutLogN++
	if layoutLogN <= 200 {
		return true
	}
	if layoutLogN == 201 {
		layoutLog("suppress further frame logs (cap 200); overflows still recorded")
	}
	return over > 0
}

// debugFrame logs frame metrics and any line that exceeds the terminal width.
func (s *State) debugFrame(stage string, term, cw, boxW int, out string) {
	debug := layoutDebugEnabled()
	st := collectFrameStats(out, term)
	if !shouldLogFrame(debug, st.over) {
		return
	}
	msg := fmt.Sprintf("%s term=%d cw=%d boxW=%d maxLine=%d overLines=%d state.Width=%d state.Height=%d",
		stage, term, cw, boxW, st.maxW, st.over, s.Width, s.Height)
	if debug {
		layoutLog("%s", msg)
		for _, sample := range st.samples {
			layoutLog("  %s", sample)
		}
	}
	if st.over > 0 {
		layoutLogAlways("%s samples=%v", msg, st.samples)
	}
}

func hasOverflow(out string, term int) bool {
	if term <= 0 {
		return false
	}
	for _, line := range strings.Split(out, "\n") {
		if lipgloss.Width(line) > term {
			return true
		}
	}
	return false
}

func stripANSIForLog(s string) string {
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
		if c == '\n' || c == '\r' {
			b.WriteByte(' ')
			continue
		}
		b.WriteByte(c)
	}
	return b.String()
}
