package retention

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// vscodeManifest is the subset of a VS Code-family extensions.json entry
// (VS Code, Antigravity, other forks) that names the installed directory.
type vscodeManifest struct {
	RelativeLocation string `json:"relativeLocation"`
	Location         struct {
		Path string `json:"path"`
	} `json:"location"`
}

// VSCodeActiveExtensionDirs returns the directory names the editor records as
// installed in extDir/extensions.json. ok is false when there is no manifest.
// A manifest that exists but cannot be read or parsed is an error: cleanup
// must fail closed rather than guess which directory is live.
func VSCodeActiveExtensionDirs(extDir string) (dirs map[string]bool, ok bool, err error) {
	data, err := os.ReadFile(filepath.Join(extDir, "extensions.json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	var entries []vscodeManifest
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, false, fmt.Errorf("parse extensions.json: %w", err)
	}
	dirs = make(map[string]bool, len(entries))
	for _, e := range entries {
		if name := manifestDirName(e); name != "" {
			dirs[name] = true
		}
	}
	return dirs, true, nil
}

func manifestDirName(e vscodeManifest) string {
	if e.RelativeLocation != "" {
		return filepath.Base(e.RelativeLocation)
	}
	if p := strings.TrimRight(e.Location.Path, `/\`); p != "" {
		return filepath.Base(p)
	}
	return ""
}

// KeepExtensionDirs picks which of one extension's directories (sorted newest
// first) cleanup keeps: every directory the manifest marks active, or the
// newest when none is. After a downgrade the active one is older than an
// obsolete newer copy the editor will delete itself.
func KeepExtensionDirs(newestFirst []string, active map[string]bool) []string {
	if len(newestFirst) == 0 {
		return nil
	}
	var keep []string
	for _, d := range newestFirst {
		if active[d] {
			keep = append(keep, d)
		}
	}
	if len(keep) == 0 {
		keep = []string{newestFirst[0]}
	}
	return keep
}
