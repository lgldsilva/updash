package tui

import (
	"bytes"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// outputLog streams subprocess lines into the TUI log via OutputLineMsg.
type outputLog struct {
	program *tea.Program
	buf     []byte
}

func newOutputLog(program *tea.Program) *outputLog {
	return &outputLog{program: program}
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
		if line != "" && o.program != nil {
			o.program.Send(OutputLineMsg{Line: line})
		}
	}
	return len(p), nil
}
