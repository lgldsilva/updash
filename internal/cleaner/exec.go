package cleaner

import (
	"os/exec"

	"github.com/lgldsilva/updash/internal/elevate"
)

// Exec seams. Every cleanup command is built through these so tests can swap
// them for a recorder; no test may prune, uninstall or sudo on the host.
var (
	commandContext = exec.CommandContext
	sudoCommand    = elevate.Sudo
)
