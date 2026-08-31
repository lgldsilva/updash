package scanner

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// OpenCode install methods, mirroring `opencode upgrade --method` choices.
// The upgrade command only prompts ("Install anyways?") when it cannot detect
// the method itself; passing -m explicitly is what keeps the run headless.
const (
	OpenCodeMethodCurl    = "curl"
	OpenCodeMethodNpm     = "npm"
	OpenCodeMethodPnpm    = "pnpm"
	OpenCodeMethodBun     = "bun"
	OpenCodeMethodBrew    = "brew"
	OpenCodeMethodUnknown = "unknown"
)

// OpenCodeInstallInfo describes how the local opencode binary was installed.
//
// SystemPrefix means the binary lives in a directory the current user cannot
// write to (typically /usr/lib/node_modules on a distro package or a
// `sudo npm i -g`), so `opencode upgrade` would fail even with -m: the update
// has to be driven by npm with elevation instead.
type OpenCodeInstallInfo struct {
	Method       string
	BinPath      string
	SystemPrefix bool
	NpmPrefix    string
	// SystemPackage is the distro package that owns the binary ("" when none).
	// A binary owned by pacman/apt/rpm must be updated by that package manager;
	// writing over it with npm would corrupt the package database.
	SystemPackage string
}

// systemPackageProbeTimeout caps the ownership query.
const systemPackageProbeTimeout = 10 * time.Second

// Seams so tests can describe an installation without touching the machine.
var (
	openCodeLookPath     = exec.LookPath
	openCodeEvalSymlinks = filepath.EvalSymlinks
	openCodeDirWritable  = dirWritable
	openCodePackageOwner = systemPackageOwner
)

// OpenCodeInstall resolves the opencode launcher and classifies it.
// An unresolvable binary yields an unknown method with an empty path.
func OpenCodeInstall() OpenCodeInstallInfo {
	path, err := openCodeLookPath(binOpenCode)
	if err != nil || path == "" {
		return OpenCodeInstallInfo{Method: OpenCodeMethodUnknown}
	}
	if resolved, err := openCodeEvalSymlinks(path); err == nil && resolved != "" {
		path = resolved
	}
	info := OpenCodeInstallInfo{
		Method:    ClassifyOpenCodePath(path),
		BinPath:   path,
		NpmPrefix: NpmPrefixForPath(path),
	}
	info.SystemPrefix = !openCodeDirWritable(filepath.Dir(path))
	if info.SystemPrefix {
		info.SystemPackage = openCodePackageOwner(path)
	}
	return info
}

// ClassifyOpenCodePath maps an installed opencode path to its install method.
// Pure: the ordering mirrors opencode's own Installation.method().
func ClassifyOpenCodePath(path string) string {
	p := strings.ToLower(filepath.ToSlash(path))
	switch {
	case p == "":
		return OpenCodeMethodUnknown
	case strings.Contains(p, "/.opencode/bin/"), strings.Contains(p, "/.local/bin/"):
		return OpenCodeMethodCurl
	case strings.Contains(p, "/.bun/"), strings.Contains(p, "/bun/install/global/"):
		return OpenCodeMethodBun
	case strings.Contains(p, "/pnpm/"), strings.Contains(p, "/.pnpm/"):
		return OpenCodeMethodPnpm
	case strings.Contains(p, "/homebrew/"), strings.Contains(p, "/cellar/"):
		return OpenCodeMethodBrew
	case strings.Contains(p, "/node_modules/"):
		return OpenCodeMethodNpm
	default:
		return OpenCodeMethodUnknown
	}
}

// NpmPrefixForPath derives the npm --prefix that owns a path inside a global
// node_modules tree ("" when the path is not npm-managed). Deriving it from the
// binary keeps an elevated `npm install -g` writing where the current copy
// already lives, instead of wherever root's own npm config happens to point.
//
// Unix layout: <prefix>/lib/node_modules/<pkg>/…
// Windows layout: <prefix>/node_modules/<pkg>/…
func NpmPrefixForPath(path string) string {
	parts := strings.Split(filepath.ToSlash(path), "/")
	idx := -1
	for i, part := range parts {
		if strings.EqualFold(part, "node_modules") {
			idx = i
			break
		}
	}
	if idx <= 0 {
		return ""
	}
	if idx >= 2 && strings.EqualFold(parts[idx-1], "lib") {
		idx--
	}
	prefix := strings.Join(parts[:idx], "/")
	if prefix == "" {
		return "/"
	}
	return filepath.FromSlash(prefix)
}

// dirWritable reports whether the current user can create files in dir.
// It probes with a temp file (removed immediately) instead of guessing from
// uid/permission bits, which get root, ACLs and group ownership wrong.
func dirWritable(dir string) bool {
	if dir == "" {
		return false
	}
	f, err := os.CreateTemp(dir, ".updash-write-probe-*")
	if err != nil {
		return false
	}
	name := f.Name()
	_ = f.Close()
	_ = os.Remove(name)
	return true
}

// systemPackageQueries maps a distro package manager to the query that names
// the package owning a file. Only the first available tool is consulted.
var systemPackageQueries = []struct {
	tool string
	args func(path string) []string
}{
	{"pacman", func(p string) []string { return []string{"-Qo", p} }},
	{"dpkg", func(p string) []string { return []string{"-S", p} }},
	{"rpm", func(p string) []string { return []string{"-qf", p} }},
	{"apk", func(p string) []string { return []string{"info", "--who-owns", p} }},
}

// systemPackageOwner returns the distro package owning path, or "" when the
// file is unowned (or no package manager can be consulted). A non-zero exit
// means "not owned by any package", which is the common case.
func systemPackageOwner(path string) string {
	ctx, cancel := context.WithTimeout(context.Background(), systemPackageProbeTimeout)
	defer cancel()
	for _, q := range systemPackageQueries {
		if _, err := openCodeLookPath(q.tool); err != nil {
			continue
		}
		out, err := execCommand(ctx, q.tool, q.args(path)...)
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(out))
	}
	return ""
}
