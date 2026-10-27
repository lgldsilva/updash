package retention

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// CurrentLink is the sibling symlink that self-updating CLIs (Codex) point at the
// release in use: <dir>/../current -> <dir>/<release>.
const CurrentLink = "current"

// ReleaseInUse returns the basename of the release to keep in dir: the target of the
// sibling "current" symlink when it points inside dir, otherwise the newest entry by
// mtime. Empty when dir has no entries.
func ReleaseInUse(dir string) (string, error) {
	if keep, ok := currentTarget(dir); ok {
		return keep, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", nil
		}
		return "", err
	}
	var newest string
	var newestMod int64
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		if mod := info.ModTime().UnixNano(); newest == "" || mod > newestMod {
			newest, newestMod = e.Name(), mod
		}
	}
	return newest, nil
}

// currentTarget resolves <dir>/../current; ok only when it lands directly in dir.
func currentTarget(dir string) (string, bool) {
	link := filepath.Join(filepath.Dir(dir), CurrentLink)
	target, err := filepath.EvalSymlinks(link)
	if err != nil {
		return "", false
	}
	realDir, err := filepath.EvalSymlinks(dir)
	if err != nil || filepath.Dir(target) != realDir {
		return "", false
	}
	return filepath.Base(target), true
}

// SupersededReleases lists every entry of dir except the release in use
// (ReleaseInUse). Self-updaters (Codex app-server, Copilot pkg, Grok downloads)
// keep every version they ever installed, a few hundred MB each. A missing dir or
// one with a single entry yields nothing.
func SupersededReleases(dir string) ([]PathCandidate, int64, error) {
	keep, err := ReleaseInUse(dir)
	if err != nil || keep == "" {
		return nil, 0, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, 0, err
	}
	var out []PathCandidate
	var total int64
	for _, e := range entries {
		if e.Name() == keep {
			continue
		}
		p := filepath.Join(dir, e.Name())
		size := dirSize(p)
		out = append(out, PathCandidate{Path: p, Size: size})
		total += size
	}
	return out, total, nil
}
