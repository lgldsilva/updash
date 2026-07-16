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
	verNoCache   = "no cache"
	nameGoCache  = "go-cache"
	nameNpmCache = "npm-cache"
)

// --- Brew Cleanup ---

type BrewCleanSource struct{}

func (s *BrewCleanSource) Category() model.Category { return model.CatCache }
func (s *BrewCleanSource) Label() string            { return "Homebrew Cache" }
func (s *BrewCleanSource) Icon() string             { return "🧹" }

func (s *BrewCleanSource) Scan(ctx context.Context, plat model.PlatformInfo) ([]*model.Item, error) {
	// Estimate cache size
	home := os.Getenv("HOME")
	cacheDir := filepath.Join(home, "Library", "Caches", "Homebrew")
	if _, err := os.Stat(cacheDir); err != nil {
		// Linux: /home/user/.cache/Homebrew
		cacheDir = filepath.Join(home, ".cache", "Homebrew")
		if _, err := os.Stat(cacheDir); err != nil {
			return []*model.Item{
				{Name: "brew-cache", Category: model.CatCache, Status: model.StatusOK, CurrentVer: verNoCache},
			}, nil
		}
	}

	sizeOut, _ := execCommand(ctx, "du", "-sh", cacheDir)
	size := strings.TrimSpace(strings.Fields(string(sizeOut))[0])

	reclaimable := "~0B"
	if _, err := exec.LookPath("brew"); err == nil {
		if dryOut, err := execCommand(ctx, "brew", "cleanup", "-n", "-s"); err == nil {
			if n := sizefmt.ParseBrewFreed(string(dryOut)); n > 0 {
				reclaimable = sizefmt.Format(n)
			}
		}
	}

	return []*model.Item{
		{
			Name:        "brew-cache",
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
func (s *AptCleanSource) Icon() string             { return "🧹" }

func (s *AptCleanSource) Scan(ctx context.Context, plat model.PlatformInfo) ([]*model.Item, error) {
	out, err := execCommand(ctx, "du", "-sh", "/var/cache/apt")
	if err != nil {
		return []*model.Item{
			{Name: "apt-cache", Category: model.CatCache, Status: model.StatusOK, CurrentVer: verNoCache},
		}, nil
	}
	size := strings.TrimSpace(strings.Fields(string(out))[0])
	return []*model.Item{
		{
			Name:        "apt-cache",
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
func (s *DockerCleanSource) Icon() string             { return "🧹" }

func (s *DockerCleanSource) Scan(ctx context.Context, plat model.PlatformInfo) ([]*model.Item, error) {
	out, err := execCommand(ctx, "docker", "system", "df", "--format", "{{.Type}}\t{{.Size}}\t{{.Reclaimable}}")
	if err != nil {
		return []*model.Item{
			{Name: "docker", Category: model.CatDockerClean, Status: model.StatusOK, CurrentVer: "daemon not running"},
		}, nil
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
					KeepPolicy:  dockerCleanKeepPolicy(typ),
				})
			}
		}
	}

	if len(items) == 0 {
		items = append(items, &model.Item{
			Name: "docker", Category: model.CatDockerClean, Status: model.StatusOK, CurrentVer: "nothing to clean",
		})
	}

	return items, nil
}

// dockerCleanKeepPolicy documents the prune policy that clean will apply.
func dockerCleanKeepPolicy(typ string) string {
	switch {
	case strings.Contains(typ, "build"):
		if config.DockerBuilderMode() == config.DockerBuilderModeAll {
			return "builder mode=all (prune -af, unused cache only)"
		}
		return "builder mode=age until=" + config.DockerBuilderMaxAge()
	case strings.Contains(typ, "image"):
		return "image prune -a until=" + config.DockerImageMaxAge()
	case strings.Contains(typ, "container"):
		return "container prune until=" + config.DockerContainerMaxAge()
	case strings.Contains(typ, "volume"):
		return "volume prune unused"
	default:
		return "system prune until=" + config.DockerImageMaxAge()
	}
}

// --- Go Cleanup ---

type GoCleanSource struct{}

func (s *GoCleanSource) Category() model.Category { return model.CatCache }
func (s *GoCleanSource) Label() string            { return "Go Cache" }
func (s *GoCleanSource) Icon() string             { return "🧹" }

func (s *GoCleanSource) Scan(ctx context.Context, plat model.PlatformInfo) ([]*model.Item, error) {
	out, err := execCommand(ctx, "go", "env", "GOCACHE")
	if err != nil {
		return []*model.Item{
			{Name: nameGoCache, Category: model.CatCache, Status: model.StatusOK, CurrentVer: "error"},
		}, nil
	}
	cacheDir := strings.TrimSpace(string(out))

	sizeOut, err := execCommand(ctx, "du", "-sh", cacheDir)
	if err != nil {
		return []*model.Item{
			{Name: nameGoCache, Category: model.CatCache, Status: model.StatusOK, CurrentVer: verNoCache},
		}, nil
	}
	size := strings.TrimSpace(strings.Fields(string(sizeOut))[0])

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
func (s *NpmCleanSource) Icon() string             { return "🧹" }

func (s *NpmCleanSource) Scan(ctx context.Context, plat model.PlatformInfo) ([]*model.Item, error) {
	home := os.Getenv("HOME")
	cacheDir := filepath.Join(home, ".npm")
	_, err := os.Stat(cacheDir)
	if err != nil {
		return []*model.Item{
			{Name: nameNpmCache, Category: model.CatCache, Status: model.StatusOK, CurrentVer: verNoCache},
		}, nil
	}

	totalOut, err := execCommand(ctx, "du", "-sh", cacheDir)
	if err != nil {
		return []*model.Item{
			{Name: nameNpmCache, Category: model.CatCache, Status: model.StatusOK, CurrentVer: verNoCache},
		}, nil
	}
	total := strings.TrimSpace(strings.Fields(string(totalOut))[0])

	var reclaimBytes int64
	for _, sub := range []string{"_cacache", "_npx"} {
		subDir := filepath.Join(cacheDir, sub)
		if out, err := execCommand(ctx, "du", "-sk", subDir); err == nil {
			kb, _ := strconv.ParseInt(strings.Fields(string(out))[0], 10, 64)
			reclaimBytes += kb * 1024
		}
	}
	reclaimable := sizefmt.Format(reclaimBytes)
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
func (s *SnapCleanSource) Icon() string             { return "🧹" }

func (s *SnapCleanSource) Scan(ctx context.Context, plat model.PlatformInfo) ([]*model.Item, error) {
	// Check if snap is available
	_, err := exec.LookPath("snap")
	if err != nil {
		return []*model.Item{
			{Name: "snap", Category: model.CatCache, Status: model.StatusOK, CurrentVer: "not installed"},
		}, nil
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
func (s *VSCodeCleanSource) Icon() string             { return "🧹" }

func (s *VSCodeCleanSource) Scan(ctx context.Context, plat model.PlatformInfo) ([]*model.Item, error) {
	_, err := os.Stat(s.ExtDir)
	if err != nil {
		return []*model.Item{
			{Name: s.LabelName, Category: model.CatVSCodeClean, Status: model.StatusOK, CurrentVer: "no extensions"},
		}, nil
	}

	entries, err := os.ReadDir(s.ExtDir)
	if err != nil {
		return []*model.Item{
			{Name: s.LabelName, Category: model.CatVSCodeClean, Status: model.StatusOK, CurrentVer: "error reading"},
		}, nil
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
func (s *WindowsTempSource) Icon() string             { return "🧹" }

func (s *WindowsTempSource) Scan(ctx context.Context, plat model.PlatformInfo) ([]*model.Item, error) {
	out, err := execCommand(ctx, "cmd", "/c", "dir %TEMP% /s /a:-d /w 2>nul | findstr /b \"Total\"")
	if err != nil {
		return []*model.Item{
			{Name: "win-temp", Category: model.CatCache, Status: model.StatusOK, CurrentVer: "unable to scan"},
		}, nil
	}

	size := strings.TrimSpace(string(out))
	if size == "" {
		size = "?"
	}

	return []*model.Item{
		{
			Name:        "win-temp",
			Category:    model.CatCache,
			CurrentVer:  size + " (TEMP)",
			Status:      model.StatusCleanCandidate,
			Reclaimable: size,
		},
	}, nil
}
