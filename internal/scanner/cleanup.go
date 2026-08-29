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
	"github.com/lgldsilva/updash/internal/sizefmt"
)

const (
	verNoCache      = "no cache"
	nameBrewCache   = "brew-cache"
	nameAptCache    = "apt-cache"
	nameGoCache     = "go-cache"
	nameNpmCache    = "npm-cache"
	nameWindowsTemp = "win-temp"
	errInspectCache = "unable to inspect cache"
)

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
		return []*model.Item{infoItem(nameBrewCache, model.CatCache, errInspectCache)}, nil
	} else if os.IsNotExist(err) {
		// Linux: /home/user/.cache/Homebrew
		cacheDir = filepath.Join(home, ".cache", "Homebrew")
		if _, err := os.Stat(cacheDir); err != nil {
			if !os.IsNotExist(err) {
				return []*model.Item{infoItem(nameBrewCache, model.CatCache, errInspectCache)}, nil
			}
			return []*model.Item{
				{Name: nameBrewCache, Category: model.CatCache, Status: model.StatusOK, CurrentVer: verNoCache},
			}, nil
		}
	}

	size := "0B"
	if sizeOut, err := execCommand(ctx, binDu, flagDuShort, cacheDir); err == nil {
		if strings.TrimSpace(string(sizeOut)) == "" {
			return []*model.Item{infoItem(nameBrewCache, model.CatCache, errInspectCache)}, nil
		}
		size = firstField(sizeOut, size)
	} else {
		return []*model.Item{infoItem(nameBrewCache, model.CatCache, errInspectCache)}, nil
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
		return []*model.Item{infoItem(nameAptCache, model.CatCache, errInspectCache)}, nil
	}
	out, err := execCommand(ctx, binDu, flagDuShort, "/var/cache/apt")
	if err != nil {
		return []*model.Item{infoItem(nameAptCache, model.CatCache, errInspectCache)}, nil
	}
	if strings.TrimSpace(string(out)) == "" {
		return []*model.Item{infoItem(nameAptCache, model.CatCache, errInspectCache)}, nil
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
		return []*model.Item{infoItem(nameGoCache, model.CatCache, errInspectCache)}, nil
	}
	if strings.TrimSpace(string(sizeOut)) == "" {
		return []*model.Item{infoItem(nameGoCache, model.CatCache, errInspectCache)}, nil
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
			return []*model.Item{infoItem(nameNpmCache, model.CatCache, errInspectCache)}, nil
		}
		return []*model.Item{
			{Name: nameNpmCache, Category: model.CatCache, Status: model.StatusOK, CurrentVer: verNoCache},
		}, nil
	}

	totalOut, err := execCommand(ctx, binDu, flagDuShort, cacheDir)
	if err != nil {
		return []*model.Item{infoItem(nameNpmCache, model.CatCache, errInspectCache)}, nil
	}
	if strings.TrimSpace(string(totalOut)) == "" {
		return []*model.Item{infoItem(nameNpmCache, model.CatCache, errInspectCache)}, nil
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
	_, err := os.Stat(s.ExtDir)
	if err != nil {
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

	// Group by publisher.name and find duplicates
	type extInfo struct {
		name    string
		version string
	}
	extMap := make(map[string][]extInfo)

	// Regex: publisher.name-version-arch
	re := regexp.MustCompile(`^([a-zA-Z0-9_.-]+)-(\d+\.\d+\.\d+)(?:-.+)?$`)

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		m := re.FindStringSubmatch(entry.Name())
		if m == nil {
			continue
		}
		fullName := m[1]
		version := m[2]
		extMap[fullName] = append(extMap[fullName], extInfo{name: entry.Name(), version: version})
	}

	var items []*model.Item
	for extName, versions := range extMap {
		if len(versions) <= 1 {
			continue // no duplicates
		}

		// Sort by version descending
		sort.Slice(versions, func(i, j int) bool {
			return compareVersions(versions[i].version, versions[j].version) > 0
		})

		removeCount := len(versions) - 1
		items = append(items, &model.Item{
			Name:        fmt.Sprintf("ext: %s", extName),
			Category:    model.CatVSCodeClean,
			CurrentVer:  versions[0].version, // latest kept
			Reclaimable: fmt.Sprintf("%d old version(s)", removeCount),
			RemoveCount: removeCount,
			KeepPolicy:  "keep latest",
			Status:      model.StatusCleanCandidate,
		})
	}

	if len(items) == 0 {
		items = append(items, &model.Item{
			Name: s.LabelName, Category: model.CatVSCodeClean, Status: model.StatusOK, CurrentVer: "no duplicates",
		})
	}

	return items, nil
}

// WindowsTempSource scans Windows temporary files for cleanup.
type WindowsTempSource struct{}

func (s *WindowsTempSource) Category() model.Category { return model.CatCache }
func (s *WindowsTempSource) Label() string            { return "Windows TEMP" }
func (s *WindowsTempSource) Icon() string             { return cleanupIcon }

func (s *WindowsTempSource) Scan(ctx context.Context, plat model.PlatformInfo) ([]*model.Item, error) {
	out, err := execCommand(ctx, "cmd", "/c", "dir %TEMP% /s /a:-d /w 2>nul | findstr /b \"Total\"")
	if err != nil {
		return []*model.Item{infoItem(nameWindowsTemp, model.CatCache, "unable to scan")}, nil
	}

	size := strings.TrimSpace(string(out))
	if size == "" {
		return []*model.Item{infoItem(nameWindowsTemp, model.CatCache, "unable to scan")}, nil
	}

	return []*model.Item{
		{
			Name:        nameWindowsTemp,
			Category:    model.CatCache,
			CurrentVer:  size + " (TEMP)",
			Status:      model.StatusCleanCandidate,
			Reclaimable: size,
		},
	}, nil
}
