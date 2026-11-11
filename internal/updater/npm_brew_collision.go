package updater

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/lgldsilva/updash/internal/model"
)

// npmBrewCollisionDowngrade rewrites a failed npm agent update as manual-only
// when npm could not install because a Homebrew cask already owns the binary
// link ("npm error File exists: /opt/homebrew/bin/codex" pointing into
// Caskroom). No npm run can ever win that collision — brew recreates the link
// on its next upgrade — so picking the origin belongs to the user, not to the
// updater. The item keeps its outdated status and carries the exact commands
// to resolve it.
func npmBrewCollisionDowngrade(item *model.Item, result *Result) *Result {
	if result == nil || result.Success {
		return result
	}
	bin, cask, ok := npmCaskCollision(result.Output)
	if !ok {
		return result
	}
	if cask == "" {
		cask = filepath.Base(bin)
	}
	pkg := item.PackageID
	if pkg == "" {
		pkg = "<pacote>"
	}
	keep := "resolução manual: binário " + bin + " pertence ao cask brew \"" + cask +
		"\" — escolha a origem: brew uninstall --cask " + cask + " (npm/updash gerem) ou npm rm -g " + pkg + " (brew gere)"
	item.KeepPolicy = keep
	item.Status = model.StatusOutdated
	return &Result{
		Item:    item,
		Success: false,
		Error:   "⊘ " + keep,
		Output:  result.Output,
	}
}

// brewCaskPathFromNpmCollision extracts the colliding path from npm's
// "File exists:" error and reports it when it is a symlink into Homebrew's
// Caskroom (covers the /opt/homebrew and /usr/local prefixes).
func brewCaskPathFromNpmCollision(output string) (string, bool) {
	bin, _, ok := npmCaskCollision(output)
	return bin, ok
}

// npmCaskCollision returns the bin path npm could not replace and the cask
// token parsed from the symlink target (.../Caskroom/<token>/...).
func npmCaskCollision(output string) (bin, cask string, ok bool) {
	const marker = "File exists:"
	for _, line := range strings.Split(output, "\n") {
		bin, cask, ok = collisionOnLine(line, marker)
		if ok {
			return bin, cask, true
		}
	}
	return "", "", false
}

func collisionOnLine(line, marker string) (bin, cask string, ok bool) {
	idx := strings.Index(line, marker)
	if idx < 0 {
		return "", "", false
	}
	path := strings.TrimSpace(line[idx+len(marker):])
	if path == "" {
		return "", "", false
	}
	target, err := os.Readlink(path)
	if err != nil || !strings.Contains(target, "/Caskroom/") {
		return "", "", false
	}
	return path, caskToken(target), true
}

// caskToken is the directory immediately under Caskroom. Homebrew recreates
// the bin link from that cask, so the uninstall command has to name it and
// not the binary (the two differ for some casks).
func caskToken(target string) string {
	const marker = "/Caskroom/"
	slash := filepath.ToSlash(target)
	i := strings.Index(slash, marker)
	if i < 0 {
		return ""
	}
	name, _, _ := strings.Cut(slash[i+len(marker):], "/")
	if name == "" || name == "." || name == ".." {
		return ""
	}
	return name
}
