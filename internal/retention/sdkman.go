package retention

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// SDKMANDefaultVersion returns the version the candidate's `current` symlink
// points to (the user's `sdk default`). Cleanup must never remove it, even
// when a newer version of the same major is installed.
//
// known is false when `current` exists but is not a readable symlink (Git
// Bash/MSYS without winsymlinks copies the directory instead of linking):
// the default cannot be identified, so callers must not clean that
// candidate. No `current` at all is known with an empty version.
func SDKMANDefaultVersion(candidateDir string) (version string, known bool) {
	link := filepath.Join(candidateDir, "current")
	target, err := os.Readlink(link)
	if err == nil {
		return filepath.Base(strings.TrimRight(target, `/\`)), true
	}
	if _, statErr := os.Lstat(link); errors.Is(statErr, os.ErrNotExist) {
		return "", true
	}
	return "", false
}
