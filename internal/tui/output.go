package tui

import (
	"bytes"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// outputLog streams subprocess lines into the TUI log via OutputLineMsg.
type outputLog struct {
	program    *tea.Program
	buf        []byte
	generation uint64
}

func newOutputLog(program *tea.Program, generations ...uint64) *outputLog {
	var generation uint64
	if len(generations) > 0 {
		generation = generations[0]
	}
	return &outputLog{program: program, generation: generation}
}

func (o *outputLog) Write(p []byte) (int, error) {
	o.buf = append(o.buf, p...)
	for {
		idx := bytes.IndexByte(o.buf, '\n')
		if idx < 0 {
			break
		}
		line := strings.TrimSpace(string(o.buf[:idx]))
		o.buf = o.buf[idx+1:]
		o.sendLine(line)
	}
	return len(p), nil
}

// Flush emits a final partial line exactly once. Commands often finish
// without a trailing newline, and leaving it buffered hides the last error.
func (o *outputLog) Flush() {
	line := strings.TrimSpace(string(o.buf))
	o.buf = nil
	o.sendLine(line)
}

func (o *outputLog) sendLine(line string) {
	if line != "" && o.program != nil {
		o.program.Send(OutputLineMsg{Generation: o.generation, Line: line})
	}
}
