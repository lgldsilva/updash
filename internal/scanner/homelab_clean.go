package scanner

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/lgldsilva/updash/internal/config"
	"github.com/lgldsilva/updash/internal/model"
	"github.com/lgldsilva/updash/internal/retention"
	"github.com/lgldsilva/updash/internal/sizefmt"
)

// HomelabCleanSource scans retention-based cleanup targets (logs, caches, AI outputs).
type HomelabCleanSource struct{}

const policyMtimeDays = "mtime > %dd"

func (s *HomelabCleanSource) Category() model.Category { return model.CatHomelabClean }
func (s *HomelabCleanSource) Label() string            { return "Homelab Cleanup" }
func (s *HomelabCleanSource) Icon() string             { return "🏠" }

// HomelabHome is overridable in tests.
var HomelabHome = func() string {
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	return os.Getenv("USERPROFILE")
}

// DiskUsedPercent is overridable; default probes the home filesystem.
var DiskUsedPercent = diskUsedPercentDefault

func (s *HomelabCleanSource) Scan(ctx context.Context, plat model.PlatformInfo) ([]*model.Item, error) {
	now := time.Now()
	home := HomelabHome()
	var items []*model.Item

	devDays := config.DevCacheMaxDays()
	items = append(items, scanAgeDir(
		"dev-cache:maven",
		filepath.Join(home, ".m2", "repository"),
		devDays,
		now,
		fmt.Sprintf(policyMtimeDays, devDays),
	)...)
	items = append(items, scanAgeDir(
		"dev-cache:gradle",
		filepath.Join(home, ".gradle", "caches"),
		devDays,
		now,
		fmt.Sprintf(policyMtimeDays, devDays),
	)...)
	// dev-cache:projects-builds is an explicit opt-in (UPDASH_DEV_PROJECTS_CLEAN).
	if config.DevProjectsCleanEnabled() {
		items = append(items, scanProjectBuilds(
			ctx,
			config.DevProjectsDir(),
			devDays,
			now,
		)...)
	}

	aiDays := config.AIOutputMaxDays()
	for _, pair := range aiOutputTargets(home) {
		items = append(items, scanAgeDir(
			"ai-output:"+pair.name,
			pair.path,
			aiDays,
			now,
			fmt.Sprintf(policyMtimeDays, aiDays),
		)...)
	}

	logDays := config.HostLogMaxDays()
	for _, pair := range hostLogTargets(home, plat.OS) {
		items = append(items, scanAgeDir(
			"host-logs:"+pair.name,
			pair.path,
			logDays,
			now,
			fmt.Sprintf(policyMtimeDays, logDays),
		)...)
	}

	items = append(items, scanContainerLogItem(plat)...)
	items = append(items, scanDiskPressureItem(plat)...)

	if len(items) == 0 {
		return []*model.Item{
			{Name: "homelab", Category: model.CatHomelabClean, Status: model.StatusOK, CurrentVer: "nothing to clean"},
		}, nil
	}
	return items, nil
}

type namedPath struct {
	name string
	path string
}

func aiOutputTargets(home string) []namedPath {
	return []namedPath{
		{binClaude, filepath.Join(home, ".claude", "debug")},
		{"codex", filepath.Join(home, ".codex", "log")},
		{binOpenCode, filepath.Join(home, ".cache", binOpenCode)},
		{binGrok, filepath.Join(home, ".grok", "sessions")},
	}
}

func hostLogTargets(home, goos string) []namedPath {
	out := []namedPath{
		{"user-state", filepath.Join(home, ".local", "state")},
	}
	if goos == "darwin" || runtime.GOOS == "darwin" {
		out = append(out, namedPath{"library-logs", filepath.Join(home, "Library", "Logs")})
	}
	return out
}

func scanAgeDir(name, dir string, maxDays int, now time.Time, policy string) []*model.Item {
	cands, total, err := retention.CollectOldPaths(dir, maxDays, 1, now)
	if err != nil {
		return []*model.Item{{
			Name:       name,
			Category:   model.CatHomelabClean,
			PackageID:  dir,
			CurrentVer: "scan failed: " + err.Error(),
			Status:     model.StatusUnverified,
			KeepPolicy: policy,
		}}
	}
	if len(cands) == 0 || total <= 0 {
		return nil
	}
	return []*model.Item{{
		Name:         name,
		Category:     model.CatHomelabClean,
		PackageID:    dir,
		CurrentVer:   sizefmt.Format(total),
		Status:       model.StatusCleanCandidate,
		Reclaimable:  sizefmt.Format(total),
		RemoveCount:  len(cands),
		KeepPolicy:   policy,
		AvailableVer: fmt.Sprintf("%d path(s)", len(cands)),
	}}
}

func scanContainerLogItem(plat model.PlatformInfo) []*model.Item {
	if !plat.HasDocker {
		return nil
	}
	maxMB := config.ContainerLogMaxMB()
	return []*model.Item{{
		Name:        "container-logs",
		Category:    model.CatHomelabClean,
		CurrentVer:  fmt.Sprintf("threshold %dMB", maxMB),
		Status:      model.StatusCleanCandidate,
		Reclaimable: "large container logs",
		KeepPolicy:  fmt.Sprintf("truncate logs > %dMB", maxMB),
	}}
}

func scanDiskPressureItem(plat model.PlatformInfo) []*model.Item {
	if !plat.HasDocker {
		return nil
	}
	used := DiskUsedPercent()
	thr := config.DiskPressurePct()
	if !retention.DiskPressureTriggered(used, thr) {
		return nil
	}
	return []*model.Item{{
		Name:        "disk-pressure",
		Category:    model.CatHomelabClean,
		CurrentVer:  fmt.Sprintf("%d%% used", used),
		Status:      model.StatusCleanCandidate,
		Reclaimable: "aggressive docker prune",
		KeepPolicy:  fmt.Sprintf("disk ≥ %d%%", thr),
	}}
}

func diskUsedPercentDefault() int {
	// Best-effort via `df -P` on home; tests override DiskUsedPercent.
	home := HomelabHome()
	if home == "" {
		home = "/"
	}
	out, err := execCommand(context.Background(), "df", "-P", home)
	if err != nil {
		return 0
	}
	return parseDFUsedPercent(string(out))
}

// parseDFUsedPercent parses POSIX `df -P` output and returns Use% as int.
func parseDFUsedPercent(out string) int {
	lines := splitNonEmpty(out)
	if len(lines) < 2 {
		return 0
	}
	// Filesystem 1024-blocks Used Available Capacity Mounted on
	fields := fieldsWS(lines[len(lines)-1])
	if len(fields) < 5 {
		return 0
	}
	capField := fields[4]
	capField = trimSuffix(capField, "%")
	n := 0
	for _, ch := range capField {
		if ch < '0' || ch > '9' {
			return 0
		}
		n = n*10 + int(ch-'0')
	}
	return n
}

func splitNonEmpty(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			line := s[start:i]
			if len(line) > 0 && line[len(line)-1] == '\r' {
				line = line[:len(line)-1]
			}
			if line != "" {
				lines = append(lines, line)
			}
			start = i + 1
		}
	}
	if start < len(s) {
		line := s[start:]
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func fieldsWS(s string) []string {
	var out []string
	field := make([]byte, 0, 16)
	flush := func() {
		if len(field) > 0 {
			out = append(out, string(field))
			field = field[:0]
		}
	}
	for i := 0; i < len(s); i++ {
		if s[i] == ' ' || s[i] == '\t' {
			flush()
			continue
		}
		field = append(field, s[i])
	}
	flush()
	return out
}

func trimSuffix(s, suf string) string {
	if len(s) >= len(suf) && s[len(s)-len(suf):] == suf {
		return s[:len(s)-len(suf)]
	}
	return s
}

func scanProjectBuilds(ctx context.Context, projectsDir string, maxDays int, now time.Time) []*model.Item {
	if projectsDir == "" {
		return nil
	}
	cands, total, partialErrs, err := retention.CollectProjectBuildPaths(ctx, projectsDir, maxDays, now)
	policy := fmt.Sprintf("build dirs always; node_modules idle > %dd", maxDays)
	if err != nil {
		return []*model.Item{{
			Name:       "dev-cache:projects-builds",
			Category:   model.CatHomelabClean,
			PackageID:  projectsDir,
			Status:     model.StatusUnverified,
			CurrentVer: "scan failed: " + err.Error(),
			KeepPolicy: policy,
		}}
	}
	if len(partialErrs) > 0 {
		policy += fmt.Sprintf("; %d unreadable path(s) skipped", len(partialErrs))
	}
	if len(cands) == 0 || total <= 0 {
		if len(partialErrs) > 0 {
			return []*model.Item{{
				Name:       "dev-cache:projects-builds",
				Category:   model.CatHomelabClean,
				PackageID:  projectsDir,
				Status:     model.StatusUnverified,
				CurrentVer: "no candidates; some projects unreadable",
				KeepPolicy: policy,
			}}
		}
		return nil
	}
	return []*model.Item{{
		Name:         "dev-cache:projects-builds",
		Category:     model.CatHomelabClean,
		PackageID:    projectsDir,
		CurrentVer:   sizefmt.Format(total),
		Status:       model.StatusCleanCandidate,
		Reclaimable:  sizefmt.Format(total),
		RemoveCount:  len(cands),
		KeepPolicy:   policy,
		AvailableVer: fmt.Sprintf("%d path(s)", len(cands)),
	}}
}
