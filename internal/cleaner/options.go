package cleaner

import (
	"os/exec"

	"github.com/lgldsilva/updash/internal/updater"
)

// Options controls how cleanup commands are executed (shared with updater).
type Options = updater.Options

// DefaultOptions returns sensible CLI defaults (live output on TTY).
func DefaultOptions() Options {
	return updater.DefaultOptions()
}

// SilentOptions buffers output (TUI).
func SilentOptions() Options {
	return updater.SilentOptions()
}

func configureCmd(o Options, cmd *exec.Cmd) {
	o.ConfigureCmd(cmd)
}
