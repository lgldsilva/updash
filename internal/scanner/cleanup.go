package scanner

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/lgldsilva/updash/internal/config"
	"github.com/lgldsilva/updash/internal/model"
	"github.com/lgldsilva/updash/internal/retention"
	"github.com/lgldsilva/updash/internal/sizefmt"
)

const (
	verNoCache              = "no cache"
	nameGoCache             = "go-cache"
	nameNpmCache            = "npm-cache"
	nameBrewCache           = "brew-cache"
	nameAptCache            = "apt-cache"
	namePacmanCache         = "pacman-cache"
	namePacmanOrphans       = "pacman-orphans"
	nameYayCache            = "yay-cache"
	nameWinTemp             = "win-temp"
	msgUnableToInspectCache = "unable to inspect cache"
	msgUnableToScan         = "unable to scan"
)

func unverifiedCache(name string) []*model.Item {
	return []*model.Item{infoItem(name, model.CatCache, msgUnableToInspectCache)}
}

// firstField returns the first whitespace-separated field of out, or
// fallback when out has none. Guards `du` calls that succeed with an
// empty stdout — indexing Fields()[0] blindly would panic inside the scan
// goroutine and take the whole process down.
func firstField(out []byte, fallback string) string {
	fields := strings.Fields(string(out))
	if len(fields) == 0 {
		return fallback
	}
	return fields[0]
}

// --- Brew Cleanup ---

type BrewCleanSource struct{}

func (s *BrewCleanSource) Category() model.Category { return model.CatCache }
func (s *BrewCleanSource) Label() string            { return "Homebrew Cache" }
func (s *BrewCleanSource) Icon() string             { return cleanupIcon }

func (s *BrewCleanSource) Scan(ctx context.Context, plat model.PlatformInfo) ([]*model.Item, error) {
	// Estimate cache size
	home := os.Getenv("HOME")
	cacheDir := filepath.Join(home, "Library", "Caches", "Homebrew")
	if _, err := os.Stat(cacheDir); err != nil && !os.IsNotExist(err) {
		return unverifiedCache(nameBrewCache), nil
	} else if os.IsNotExist(err) {
		// Linux: /home/user/.cache/Homebrew
		cacheDir = filepath.Join(home, ".cache", "Homebrew")
		if _, err := os.Stat(cacheDir); err != nil {
			if !os.IsNotExist(err) {
				return unverifiedCache(nameBrewCache), nil
			}
			return []*model.Item{
				{Name: nameBrewCache, Category: model.CatCache, Status: model.StatusOK, CurrentVer: verNoCache},
			}, nil
		}
	}

	size := "0B"
	if sizeOut, err := execCommand(ctx, binDu, flagDuShort, cacheDir); err == nil {
		if strings.TrimSpace(string(sizeOut)) == "" {
			return unverifiedCache(nameBrewCache), nil
		}
		size = firstField(sizeOut, size)
	} else {
		return unverifiedCache(nameBrewCache), nil
	}

	reclaimable := "~0B"
	if _, err := exec.LookPath(binBrew); err == nil {
		if dryOut, err := execCommand(ctx, binBrew, "cleanup", "-n", "-s"); err == nil {
			if n := sizefmt.ParseBrewFreed(string(dryOut)); n > 0 {
				reclaimable = sizefmt.Format(n)
			}
		}
	}

	return []*model.Item{
		{
			Name:        nameBrewCache,
			Category:    model.CatCache,
			CurrentVer:  size,
			Status:      model.StatusCleanCandidate,
			Reclaimable: reclaimable,
			KeepPolicy:  "old versions; active downloads kept",
		},
	}, nil
}

// --- Apt Cleanup ---

type AptCleanSource struct{}

func (s *AptCleanSource) Category() model.Category { return model.CatCache }
func (s *AptCleanSource) Label() string            { return "apt Cache" }
func (s *AptCleanSource) Icon() string             { return cleanupIcon }

func (s *AptCleanSource) Scan(ctx context.Context, plat model.PlatformInfo) ([]*model.Item, error) {
	if _, statErr := os.Stat("/var/cache/apt"); statErr != nil {
		if os.IsNotExist(statErr) {
			return []*model.Item{{Name: nameAptCache, Category: model.CatCache, Status: model.StatusOK, CurrentVer: verNoCache}}, nil
		}
		return unverifiedCache(nameAptCache), nil
	}
	out, err := execCommand(ctx, binDu, flagDuShort, "/var/cache/apt")
	if err != nil {
		return unverifiedCache(nameAptCache), nil
	}
	if strings.TrimSpace(string(out)) == "" {
		return unverifiedCache(nameAptCache), nil
	}
	size := firstField(out, "0B")
	return []*model.Item{
		{
			Name:        nameAptCache,
			Category:    model.CatCache,
			CurrentVer:  size,
			Status:      model.StatusCleanCandidate,
			Reclaimable: size,
		},
	}, nil
}

// --- Pacman Cleanup ---

type PacmanCleanSource struct{}

func (s *PacmanCleanSource) Category() model.Category { return model.CatCache }
func (s *PacmanCleanSource) Label() string            { return "Pacman Cache" }
func (s *PacmanCleanSource) Icon() string             { return cleanupIcon }

func (s *PacmanCleanSource) Scan(ctx context.Context, plat model.PlatformInfo) ([]*model.Item, error) {
	var items []*model.Item
	if it := s.scanPacmanCache(ctx); it != nil {
		items = append(items, it)
	}
	if it := s.scanYayCache(ctx, plat); it != nil {
		items = append(items, it)
	}
	if it := s.scanOrphans(ctx); it != nil {
		items = append(items, it)
	}
	return items, nil
}

// scanPacmanCache sizes /var/cache/pacman/pkg. Cleaning runs `paccache -rk2`
// (pacman-contrib), so without paccache the item is informational only.
func (s *PacmanCleanSource) scanPacmanCache(ctx context.Context) *model.Item {
	if _, err := os.Stat("/var/cache/pacman/pkg"); err != nil {
		if os.IsNotExist(err) {
			return &model.Item{Name: namePacmanCache, Category: model.CatCache, Status: model.StatusOK, CurrentVer: verNoCache}
		}
		return infoItem(namePacmanCache, model.CatCache, msgUnableToInspectCache)
	}
	if _, err := exec.LookPath("paccache"); err != nil {
		return &model.Item{Name: namePacmanCache, Category: model.CatCache, Status: model.StatusInfo, CurrentVer: "install pacman-contrib (paccache) to clean"}
	}
	// du exits 1 on unreadable subdirs (e.g. pacman's root-owned
	// download-* temp dirs during an active update) but still prints the
	// total on stdout — trust the output whenever it is non-empty.
	out, _ := execCommand(ctx, binDu, flagDuShort, "/var/cache/pacman/pkg")
	if strings.TrimSpace(string(out)) == "" {
		return infoItem(namePacmanCache, model.CatCache, msgUnableToInspectCache)
	}
	size := firstField(out, "0B")
	return &model.Item{
		Name:        namePacmanCache,
		Category:    model.CatCache,
		CurrentVer:  size,
		Status:      model.StatusCleanCandidate,
		Reclaimable: size,
		KeepPolicy:  "keep 2 latest versions (paccache -rk2)",
	}
}

// scanYayCache sizes the AUR build cache (~/.cache/yay). Only present as an
// item when yay is installed and the cache directory exists.
func (s *PacmanCleanSource) scanYayCache(ctx context.Context, plat model.PlatformInfo) *model.Item {
	if !plat.HasYay {
		return nil
	}
	cacheDir := filepath.Join(os.Getenv("HOME"), ".cache", "yay")
	if _, err := os.Stat(cacheDir); err != nil {
		if os.IsNotExist(err) {
			return &model.Item{Name: nameYayCache, Category: model.CatCache, Status: model.StatusOK, CurrentVer: verNoCache}
		}
		return infoItem(nameYayCache, model.CatCache, msgUnableToInspectCache)
	}
	out, _ := execCommand(ctx, binDu, flagDuShort, cacheDir)
	if strings.TrimSpace(string(out)) == "" {
		return infoItem(nameYayCache, model.CatCache, msgUnableToInspectCache)
	}
	size := firstField(out, "0B")
	return &model.Item{
		Name:        nameYayCache,
		Category:    model.CatCache,
		CurrentVer:  size,
		Status:      model.StatusCleanCandidate,
		Reclaimable: size,
		KeepPolicy:  "AUR build cache (yay -Sc --aur)",
	}
}

// scanOrphans detects orphan packages (installed as deps, no longer required)
// via `pacman -Qtdq`. Only surfaces an item when orphans exist.
func (s *PacmanCleanSource) scanOrphans(ctx context.Context) *model.Item {
	out, err := execCommand(ctx, binPacman, "-Qtdq")
	if err != nil {
		return nil // query failure is benign — nothing claimable
	}
	var count int
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if strings.TrimSpace(line) != "" {
			count++
		}
	}
	if count == 0 {
		return nil
	}
	return &model.Item{
		Name:        namePacmanOrphans,
		Category:    model.CatCache,
		CurrentVer:  fmt.Sprintf("%d package(s)", count),
		Status:      model.StatusCleanCandidate,
		RemoveCount: count,
		KeepPolicy:  "pacman -Rns (orphaned deps)",
	}
}

// --- Docker Cleanup ---

type DockerCleanSource struct{}

func (s *DockerCleanSource) Category() model.Category { return model.CatDockerClean }
func (s *DockerCleanSource) Label() string            { return "Docker Cleanup" }
func (s *DockerCleanSource) Icon() string             { return cleanupIcon }

func (s *DockerCleanSource) Scan(ctx context.Context, plat model.PlatformInfo) ([]*model.Item, error) {
	out, err := execCommand(ctx, binDocker, "system", "df", "--format", "{{.Type}}\t{{.Size}}\t{{.Reclaimable}}")
	if err != nil {
		return []*model.Item{infoItem(binDocker, model.CatDockerClean, "daemon not running")}, nil
	}

	var items []*model.Item
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	for _, line := range lines {
		fields := strings.Split(line, "\t")
		if len(fields) >= 3 {
			typ := strings.ToLower(fields[0])
			size := fields[1]
			reclaim := fields[2]
			if reclaim != "0B" {
				items = append(items, &model.Item{
					Name:        "docker " + typ,
					Category:    model.CatDockerClean,
					CurrentVer:  size,
					Reclaimable: reclaim,
					Status:      model.StatusCleanCandidate,
					KeepPolicy:  config.DockerResourceKeepPolicy(typ),
				})
			}
		}
	}

	if len(items) == 0 {
		items = append(items, &model.Item{
			Name: binDocker, Category: model.CatDockerClean, Status: model.StatusOK, CurrentVer: "nothing to clean",
		})
	}

	return items, nil
}

// --- Go Cleanup ---

type GoCleanSource struct{}

func (s *GoCleanSource) Category() model.Category { return model.CatCache }
func (s *GoCleanSource) Label() string            { return "Go Cache" }
func (s *GoCleanSource) Icon() string             { return cleanupIcon }

func (s *GoCleanSource) Scan(ctx context.Context, plat model.PlatformInfo) ([]*model.Item, error) {
	out, err := execCommand(ctx, binGo, "env", "GOCACHE")
	if err != nil {
		return []*model.Item{infoItem(nameGoCache, model.CatCache, "unable to determine cache path")}, nil
	}
	cacheDir := strings.TrimSpace(string(out))
	if cacheDir == "" {
		return []*model.Item{infoItem(nameGoCache, model.CatCache, "unable to determine cache path")}, nil
	}

	sizeOut, err := execCommand(ctx, binDu, flagDuShort, cacheDir)
	if err != nil {
		return unverifiedCache(nameGoCache), nil
	}
	if strings.TrimSpace(string(sizeOut)) == "" {
		return unverifiedCache(nameGoCache), nil
	}
	size := firstField(sizeOut, "0B")

	return []*model.Item{
		{
			Name:        nameGoCache,
			Category:    model.CatCache,
			CurrentVer:  size,
			Status:      model.StatusCleanCandidate,
			Reclaimable: size,
			KeepPolicy:  "build cache only",
		},
	}, nil
}

// --- npm Cleanup ---

type NpmCleanSource struct{}

func (s *NpmCleanSource) Category() model.Category { return model.CatCache }
func (s *NpmCleanSource) Label() string            { return "npm Cache" }
func (s *NpmCleanSource) Icon() string             { return cleanupIcon }

func (s *NpmCleanSource) Scan(ctx context.Context, plat model.PlatformInfo) ([]*model.Item, error) {
	home := os.Getenv("HOME")
	cacheDir := filepath.Join(home, ".npm")
	_, err := os.Stat(cacheDir)
	if err != nil {
		if !os.IsNotExist(err) {
			return unverifiedCache(nameNpmCache), nil
		}
		return []*model.Item{
			{Name: nameNpmCache, Category: model.CatCache, Status: model.StatusOK, CurrentVer: verNoCache},
		}, nil
	}

	totalOut, err := execCommand(ctx, binDu, flagDuShort, cacheDir)
	if err != nil {
		return unverifiedCache(nameNpmCache), nil
	}
	if strings.TrimSpace(string(totalOut)) == "" {
		return unverifiedCache(nameNpmCache), nil
	}
	total := firstField(totalOut, "0B")

	var reclaimBytes int64
	subErrors := 0
	for _, sub := range []string{"_cacache", "_npx"} {
		subDir := filepath.Join(cacheDir, sub)
		if out, err := execCommand(ctx, binDu, "-sk", subDir); err == nil {
			if strings.TrimSpace(string(out)) == "" {
				subErrors++
				continue
			}
			kb, _ := strconv.ParseInt(firstField(out, "0"), 10, 64)
			reclaimBytes += kb * 1024
		} else {
			subErrors++
		}
	}
	reclaimable := sizefmt.Format(reclaimBytes)
	if reclaimBytes == 0 && subErrors > 0 {
		return []*model.Item{infoItem(nameNpmCache, model.CatCache, total+" (cache details incomplete)")}, nil
	}
	if reclaimBytes == 0 {
		return []*model.Item{
			{Name: nameNpmCache, Category: model.CatCache, Status: model.StatusOK, CurrentVer: total},
		}, nil
	}

	return []*model.Item{
		{
			Name:        nameNpmCache,
			Category:    model.CatCache,
			CurrentVer:  total,
			Status:      model.StatusCleanCandidate,
			Reclaimable: reclaimable,
			KeepPolicy:  "cache + npx extractions",
		},
	}, nil
}

// --- Snap Cleanup ---

type SnapCleanSource struct{}

func (s *SnapCleanSource) Category() model.Category { return model.CatCache }
func (s *SnapCleanSource) Label() string            { return "Snap (retain=2)" }
func (s *SnapCleanSource) Icon() string             { return cleanupIcon }

func (s *SnapCleanSource) Scan(ctx context.Context, plat model.PlatformInfo) ([]*model.Item, error) {
	// Check if snap is available
	_, err := exec.LookPath(binSnap)
	if err != nil {
		return []*model.Item{infoItem(binSnap, model.CatCache, "not installed")}, nil
	}

	return []*model.Item{
		{
			Name:       "snap-retain",
			Category:   model.CatCache,
			Status:     model.StatusCleanCandidate,
			KeepPolicy: "keep 2 revisions",
		},
	}, nil
}

// --- VSCode Extension Cleanup ---

type VSCodeCleanSource struct {
	LabelName string
	ExtDir    string
}

func (s *VSCodeCleanSource) Category() model.Category { return model.CatVSCodeClean }
func (s *VSCodeCleanSource) Label() string            { return s.LabelName }
func (s *VSCodeCleanSource) Icon() string             { return cleanupIcon }

func (s *VSCodeCleanSource) Scan(ctx context.Context, plat model.PlatformInfo) ([]*model.Item, error) {
	if _, err := os.Stat(s.ExtDir); err != nil {
		if !os.IsNotExist(err) {
			return []*model.Item{infoItem(s.LabelName, model.CatVSCodeClean, "unable to inspect extensions")}, nil
		}
		return []*model.Item{
			{Name: s.LabelName, Category: model.CatVSCodeClean, Status: model.StatusOK, CurrentVer: "no extensions"},
		}, nil
	}
	entries, err := os.ReadDir(s.ExtDir)
	if err != nil {
		return []*model.Item{infoItem(s.LabelName, model.CatVSCodeClean, "unable to inspect extensions")}, nil
	}
	active, hasManifest, err := retention.VSCodeActiveExtensionDirs(s.ExtDir)
	if err != nil {
		// Without a readable manifest we cannot tell the live install from an
		// obsolete one; offer nothing rather than risk the active version.
		return []*model.Item{infoItem(s.LabelName, model.CatVSCodeClean, "unable to read extensions.json")}, nil
	}
	items := vscodeDuplicateItems(groupExtensionDirs(entries), active, hasManifest)
	if len(items) == 0 {
		items = append(items, &model.Item{
			Name: s.LabelName, Category: model.CatVSCodeClean, Status: model.StatusOK, CurrentVer: "no duplicates",
		})
	}
	return items, nil
}

// extDirVersion is one installed directory of an extension.
type extDirVersion struct {
	dir     string
	version string
}

// extDirPattern matches publisher.name-version[-platform] directory names.
var extDirPattern = regexp.MustCompile(`^([a-zA-Z0-9_.-]+)-(\d+\.\d+\.\d+)(?:-.+)?$`)

// groupExtensionDirs groups extension directories by publisher.name, each
// group sorted newest first.
func groupExtensionDirs(entries []os.DirEntry) map[string][]extDirVersion {
	groups := make(map[string][]extDirVersion)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if m := extDirPattern.FindStringSubmatch(entry.Name()); m != nil {
			groups[m[1]] = append(groups[m[1]], extDirVersion{dir: entry.Name(), version: m[2]})
		}
	}
	for _, versions := range groups {
		sort.Slice(versions, func(i, j int) bool {
			return compareVersions(versions[i].version, versions[j].version) > 0
		})
	}
	return groups
}

// vscodeDuplicateItems reports one cleanup candidate per extension with more
// than one directory, keeping exactly what the cleaner keeps.
func vscodeDuplicateItems(groups map[string][]extDirVersion, active map[string]bool, hasManifest bool) []*model.Item {
	policy := "keep latest"
	if hasManifest {
		policy = "keep active (extensions.json)"
	}
	var items []*model.Item
	for extName, versions := range groups {
		if len(versions) <= 1 {
			continue
		}
		dirs := make([]string, len(versions))
		byDir := make(map[string]string, len(versions))
		for i, v := range versions {
			dirs[i] = v.dir
			byDir[v.dir] = v.version
		}
		keep := retention.KeepExtensionDirs(dirs, active)
		removeCount := len(versions) - len(keep)
		if removeCount == 0 {
			continue
		}
		items = append(items, &model.Item{
			Name:        fmt.Sprintf("ext: %s", extName),
			Category:    model.CatVSCodeClean,
			CurrentVer:  byDir[keep[0]],
			Reclaimable: fmt.Sprintf("%d old version(s)", removeCount),
			RemoveCount: removeCount,
			KeepPolicy:  policy,
			Status:      model.StatusCleanCandidate,
		})
	}
	return items
}

// WindowsTempSource scans Windows temporary files for cleanup.
type WindowsTempSource struct{}

func (s *WindowsTempSource) Category() model.Category { return model.CatCache }
func (s *WindowsTempSource) Label() string            { return "Windows TEMP" }
func (s *WindowsTempSource) Icon() string             { return cleanupIcon }

func (s *WindowsTempSource) Scan(ctx context.Context, plat model.PlatformInfo) ([]*model.Item, error) {
	out, err := execCommand(ctx, "cmd", "/c", "dir %TEMP% /s /a:-d /w 2>nul | findstr /b \"Total\"")
	if err != nil {
		return []*model.Item{infoItem(nameWinTemp, model.CatCache, msgUnableToScan)}, nil
	}

	size := strings.TrimSpace(string(out))
	if size == "" {
		return []*model.Item{infoItem(nameWinTemp, model.CatCache, msgUnableToScan)}, nil
	}

	return []*model.Item{
		{
			Name:        nameWinTemp,
			Category:    model.CatCache,
			CurrentVer:  size + " (TEMP)",
			Status:      model.StatusCleanCandidate,
			Reclaimable: size,
		},
	}, nil
}
