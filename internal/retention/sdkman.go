package retention

import (
	"os"
	"path/filepath"
	"strings"
)

// SDKMANDefaultVersion returns the version the candidate's `current` symlink
// points to (the user's `sdk default`), or "" when there is no such link.
// Cleanup must never remove it, even when a newer version of the same major
// is installed.
func SDKMANDefaultVersion(candidateDir string) string {
	target, err := os.Readlink(filepath.Join(candidateDir, "current"))
	if err != nil {
		return ""
	}
	return filepath.Base(strings.TrimRight(target, `/\`))
}
