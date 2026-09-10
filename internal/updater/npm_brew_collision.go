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
	caskPath, ok := brewCaskPathFromNpmCollision(result.Output)
	if !ok {
		return result
	}
	cask := filepath.Base(caskPath)
	pkg := item.PackageID
	if pkg == "" {
		pkg = "<pacote>"
	}
	keep := "resolução manual: binário " + caskPath + " pertence ao cask brew \"" + cask +
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
	const marker = "File exists:"
	for _, line := range strings.Split(output, "\n") {
		idx := strings.Index(line, marker)
		if idx < 0 {
			continue
		}
		path := strings.TrimSpace(line[idx+len(marker):])
		if path == "" {
			continue
		}
		target, err := os.Readlink(path)
		if err == nil && strings.Contains(target, "/Caskroom/") {
			return path, true
		}
	}
	return "", false
}
