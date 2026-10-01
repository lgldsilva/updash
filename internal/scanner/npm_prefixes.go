package scanner

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/lgldsilva/updash/internal/model"
)

// npmShadowNote explains why a shadowed legacy-prefix copy is informational
// only: the same package name is already present in the active prefix.
const npmShadowNote = "shadowed by active npm prefix copy"

// npmPrefixLabel is the display name of an unverified per-prefix probe.
const npmPrefixLabel = "npm prefix "

// npmLegacyPrefixCandidates lists well-known non-default global npm prefixes
// (the community-standard ~/.npm-global used for sudo-free installs, typically
// orphaned after a move to nvm). Seam for tests.
var npmLegacyPrefixCandidates = func() []string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return nil
	}
	return []string{filepath.Join(home, ".npm-global")}
}

// legacyNpmPrefixes returns the non-default prefixes worth scanning: roots
// that exist and differ from the active prefix. ok=false means a candidate
// exists but the active prefix could not be probed, so shadow detection would
// be guesswork this run.
func legacyNpmPrefixes(ctx context.Context) (prefixes []string, ok bool) {
	candidates := npmLegacyPrefixCandidates()
	var existing []string
	for _, c := range candidates {
		if npmGlobalRootExists(c) {
			existing = append(existing, c)
		}
	}
	if len(existing) == 0 {
		return nil, true
	}
	active := activeNpmPrefix(ctx)
	if active == "" {
		return nil, false
	}
	for _, c := range existing {
		if !sameDir(c, active) {
			prefixes = append(prefixes, c)
		}
	}
	return prefixes, true
}

// activeNpmPrefix resolves the ambient global prefix (`npm config get
// prefix`). Empty when npm could not answer.
func activeNpmPrefix(ctx context.Context) string {
	out, err := execCommand(ctx, binNpm, "config", "get", "prefix")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// npmGlobalRootExists reports whether prefix holds a global node_modules tree
// (Unix layout <prefix>/lib/node_modules, Windows <prefix>/node_modules).
func npmGlobalRootExists(prefix string) bool {
	if prefix == "" {
		return false
	}
	for _, rel := range []string{filepath.Join("lib", "node_modules"), filepath.Join("node_modules")} {
		if info, err := os.Stat(filepath.Join(prefix, rel)); err == nil && info.IsDir() {
			return true
		}
	}
	return false
}

// sameDir compares two paths after symlink resolution so an ~/.npm-global
// symlink pointing at the active prefix is not scanned twice.
func sameDir(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	if a == b {
		return true
	}
	ra, erra := filepath.EvalSymlinks(a)
	rb, errb := filepath.EvalSymlinks(b)
	if erra == nil && errb == nil {
		return ra == rb
	}
	return filepath.Clean(a) == filepath.Clean(b)
}

// npmLsGlobalVersionsFor lists one prefix's global packages with versions
// (prefix "" = the ambient prefix). A command failure without parsable output
// is an error so callers surface an unverified state instead of a silent
// false-green; an empty-but-valid listing is simply no packages.
func npmLsGlobalVersionsFor(ctx context.Context, prefix string) (map[string]string, error) {
	args := []string{"ls"}
	if prefix != "" {
		args = append(args, "--prefix", prefix)
	}
	args = append(args, flagGlobal, "--json", "--depth=0")
	out, err := execCommand(ctx, binNpm, args...)
	versions := ParseNpmLsGlobalVersions(out)
	if err != nil && len(versions) == 0 {
		return nil, err
	}
	if versions == nil {
		versions = map[string]string{}
	}
	return versions, nil
}

// npmOutdatedBatch checks explicit package names in one prefix. Names must all
// be installed: `npm outdated <name>` stalls on unknown globals, so callers
// derive the list from `npm ls`. Exit code 1 (something is outdated) still
// carries the JSON payload; malformed output is an error.
func npmOutdatedBatch(ctx context.Context, prefix string, names []string) (map[string]npmOutdatedEntry, error) {
	if len(names) == 0 {
		return map[string]npmOutdatedEntry{}, nil
	}
	sorted := append([]string(nil), names...)
	sort.Strings(sorted)
	args := []string{"outdated", flagGlobal, "--json"}
	if prefix != "" {
		args = append(args, "--prefix", prefix)
	}
	args = append(args, sorted...)
	out, err := execCommand(ctx, binNpm, args...)
	if len(out) == 0 {
		if err != nil {
			return nil, err
		}
		return map[string]npmOutdatedEntry{}, nil
	}
	var data map[string]npmOutdatedEntry
	if parseErr := json.Unmarshal(out, &data); parseErr != nil {
		return nil, parseErr
	}
	return data, nil
}

// legacyScan is one non-default prefix's raw inventory, fetched before the
// active-prefix join decides which copies are shadowed.
type legacyScan struct {
	prefix   string
	versions map[string]string
	entries  map[string]npmOutdatedEntry
}

// scanDefaultNpmPrefix inventories the ambient global prefix.
func scanDefaultNpmPrefix(ctx context.Context) (map[string]npmOutdatedEntry, map[string]string, error) {
	versions, err := npmLsGlobalVersionsFor(ctx, "")
	if err != nil {
		return nil, nil, err
	}
	entries, err := npmOutdatedBatch(ctx, "", keysOf(versions))
	if err != nil {
		return nil, nil, err
	}
	return entries, versions, nil
}

// scanLegacyPrefixData discovers and inventories every non-default prefix.
// Probe failures surface as unverified items: an unscanned installation must
// not read as "all good".
func scanLegacyPrefixData(ctx context.Context) (scans []legacyScan, problems []*model.Item) {
	prefixes, ok := legacyNpmPrefixes(ctx)
	if !ok {
		problems = append(problems, unverifiedNpmItem("npm legacy prefix",
			errors.New("could not determine the active npm prefix to compare against")))
		return nil, problems
	}
	for _, prefix := range prefixes {
		versions, err := npmLsGlobalVersionsFor(ctx, prefix)
		if err != nil {
			problems = append(problems, unverifiedNpmItem(npmPrefixLabel+prefix, err))
			continue
		}
		if len(versions) == 0 {
			continue
		}
		entries, err := npmOutdatedBatch(ctx, prefix, keysOf(versions))
		if err != nil {
			problems = append(problems, unverifiedNpmItem(npmPrefixLabel+prefix, err))
			continue
		}
		scans = append(scans, legacyScan{prefix: prefix, versions: versions, entries: entries})
	}
	return scans, problems
}

func unverifiedNpmItem(name string, err error) *model.Item {
	return &model.Item{
		Name:       name,
		Category:   model.CatNpm,
		Status:     model.StatusUnverified,
		CurrentVer: statusError,
		Error:      errCause(err),
	}
}

// itemsWhenActiveScanFailed keeps a failed active scan from hiding legacy
// prefixes. Successful legacy inventories cannot be shadow-joined, so each
// one is reported unverified instead of becoming an update target.
func itemsWhenActiveScanFailed(activeErr error, legacy []legacyScan, problems []*model.Item) []*model.Item {
	items := []*model.Item{errItem(binNpm, model.CatNpm, activeErr)}
	items = append(items, problems...)
	cause := errors.New("active npm prefix scan failed; shadow detection skipped")
	for _, sc := range legacy {
		items = append(items, unverifiedNpmItem(npmPrefixLabel+sc.prefix, cause))
	}
	return items
}

// buildLegacyItems converts one legacy prefix's inventory. A package whose
// name is also present in the active prefix is shadowed information only —
// name presence, not PATH resolution, decides that. Silently refreshing the
// hidden copy would be surprising work. Legacy-only outdated packages are
// normal update targets pinned to their own prefix; fresh legacy-only
// packages stay invisible, matching the active-prefix behaviour.
func buildLegacyItems(sc legacyScan, activeVersions map[string]string) []*model.Item {
	items := make([]*model.Item, 0, len(sc.entries))
	for name, entry := range sc.entries {
		latest := entry.Latest
		if latest == "" {
			latest = entry.Wanted
		}
		if latest == "" {
			continue
		}
		current := entry.Current
		if current == "" {
			current = sc.versions[name]
		}
		if _, shadowed := activeVersions[name]; shadowed {
			items = append(items, &model.Item{
				Name:       name,
				Prefix:     sc.prefix,
				Category:   model.CatNpm,
				CurrentVer: current,
				Status:     model.StatusInfo,
				Log:        npmShadowNote,
			})
			continue
		}
		items = append(items, &model.Item{
			Name:         name,
			Prefix:       sc.prefix,
			Category:     model.CatNpm,
			CurrentVer:   current,
			AvailableVer: latest,
			Status:       model.StatusOutdated,
		})
	}
	sortItemsByName(items)
	return items
}

// npmOutdatedItems converts npm outdated entries into items targeting prefix
// ("" = default). Result is sorted by name for stable output.
func npmOutdatedItems(entries map[string]npmOutdatedEntry, versions map[string]string, prefix string) []*model.Item {
	items := make([]*model.Item, 0, len(entries))
	for name, entry := range entries {
		available := entry.Latest
		if available == "" {
			available = entry.Wanted
		}
		if available == "" {
			continue
		}
		current := entry.Current
		if current == "" {
			current = versions[name]
		}
		items = append(items, &model.Item{
			Name:         name,
			Prefix:       prefix,
			Category:     model.CatNpm,
			CurrentVer:   current,
			AvailableVer: available,
			Status:       model.StatusOutdated,
		})
	}
	sortItemsByName(items)
	return items
}

func sortItemsByName(items []*model.Item) {
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
}

func keysOf(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
