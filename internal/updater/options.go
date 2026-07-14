package updater

import (
	"io"
	"os"
	"os/exec"
)

// Options controls how update commands are executed.
type Options struct {
	// Verbose attaches child process stdout/stderr to the terminal.
	Verbose bool
	// Interactive is true when stdin is a TTY (sudo prompts, brew progress).
	Interactive bool
	// Output captures child stdout/stderr (e.g. TUI log streaming).
	Output io.Writer
}

// DefaultOptions returns sensible CLI defaults.
func DefaultOptions() Options {
	return Options{
		Verbose:     true,
		Interactive: isTerminal(os.Stdin),
	}
}

// SilentOptions buffers all output (TUI / tests).
func SilentOptions() Options {
	return Options{}
}

func isTerminal(f *os.File) bool {
	if f == nil {
		return false
	}
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

// ConfigureCmd wires stdout/stderr for an update command.
func (o Options) ConfigureCmd(cmd *exec.Cmd) {
	switch {
	case o.Output != nil:
		cmd.Stdout = o.Output
		cmd.Stderr = o.Output
	case o.Verbose || o.Interactive:
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}
}
