package scanner

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// pnpm refuses to run any global command ("outdated -g", "update -g") when its
// global bin directory is not on PATH:
//
//	[ERROR] The configured global bin directory "…/pnpm/bin" is not in PATH
//	Run "pnpm setup" to update your shell configuration.
//
// That is a shell-configuration problem on the machine, but it made updash
// report the whole pnpm source as an error — and an inconclusive scan blocks
// every update, not just pnpm. Put the global directory on the child's PATH so
// the probe answers regardless of how the user's shell is configured.

// PnpmGlobalDirs returns the candidate pnpm global directories that exist on
// this machine, most specific first. Empty when pnpm has no global home.
func PnpmGlobalDirs() []string {
	var candidates []string
	add := func(paths ...string) {
		for _, p := range paths {
			if p != "" {
				candidates = append(candidates, p, filepath.Join(p, "bin"))
			}
		}
	}
	add(os.Getenv("PNPM_HOME"))
	if runtime.GOOS == "windows" {
		add(joinIfSet(os.Getenv("LOCALAPPDATA"), "pnpm"))
	} else {
		add(joinIfSet(os.Getenv("XDG_DATA_HOME"), "pnpm"))
		if home, err := os.UserHomeDir(); err == nil {
			add(filepath.Join(home, ".local", "share", "pnpm"))
			if runtime.GOOS == "darwin" {
				add(filepath.Join(home, "Library", "pnpm"))
			}
		}
	}

	seen := make(map[string]bool, len(candidates))
	dirs := make([]string, 0, len(candidates))
	for _, dir := range candidates {
		if seen[dir] {
			continue
		}
		seen[dir] = true
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			dirs = append(dirs, dir)
		}
	}
	return dirs
}

func joinIfSet(base string, elem ...string) string {
	if base == "" {
		return ""
	}
	return filepath.Join(append([]string{base}, elem...)...)
}

// EnsurePnpmPath returns env with the pnpm global directories appended to PATH
// (and PNPM_HOME set when it is missing). A nil env starts from the process
// environment. Returns env unchanged when pnpm has no global home here.
func EnsurePnpmPath(env []string) []string {
	dirs := PnpmGlobalDirs()
	if len(dirs) == 0 {
		return env
	}
	if env == nil {
		env = os.Environ()
	}

	out := make([]string, 0, len(env)+1)
	pathSet, homeSet := false, false
	for _, kv := range env {
		key, value, ok := strings.Cut(kv, "=")
		switch {
		case ok && strings.EqualFold(key, "PATH"):
			pathSet = true
			out = append(out, key+"="+appendMissingPaths(value, dirs))
		case ok && key == "PNPM_HOME":
			homeSet = homeSet || value != ""
			out = append(out, kv)
		default:
			out = append(out, kv)
		}
	}
	if !pathSet {
		out = append(out, "PATH="+strings.Join(dirs, string(os.PathListSeparator)))
	}
	if !homeSet {
		out = append(out, "PNPM_HOME="+dirs[0])
	}
	return out
}

// appendMissingPaths adds dirs that the PATH value does not already contain.
func appendMissingPaths(path string, dirs []string) string {
	sep := string(os.PathListSeparator)
	present := make(map[string]bool)
	for _, entry := range strings.Split(path, sep) {
		present[entry] = true
	}
	for _, dir := range dirs {
		if !present[dir] {
			path += sep + dir
			present[dir] = true
		}
	}
	return path
}
