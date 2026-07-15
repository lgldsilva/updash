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
	_ = ctx
	now := time.Now()
	home := HomelabHome()
	var items []*model.Item

	items = append(items, scanAgeDir(
		"dev-cache:maven",
		filepath.Join(home, ".m2", "repository"),
		config.DevCacheMaxDays(),
		now,
		fmt.Sprintf("atime/mtime > %dd", config.DevCacheMaxDays()),
	)...)
	items = append(items, scanAgeDir(
		"dev-cache:gradle",
		filepath.Join(home, ".gradle", "caches"),
		config.DevCacheMaxDays(),
		now,
		fmt.Sprintf("mtime > %dd", config.DevCacheMaxDays()),
	)...)

	for _, pair := range aiOutputTargets(home) {
		items = append(items, scanAgeDir(
			"ai-output:"+pair.name,
			pair.path,
			config.AIOutputMaxDays(),
			now,
			fmt.Sprintf("mtime > %dd", config.AIOutputMaxDays()),
		)...)
	}

	for _, pair := range hostLogTargets(home, plat.OS) {
		items = append(items, scanAgeDir(
			"host-logs:"+pair.name,
			pair.path,
			config.HostLogMaxDays(),
			now,
			fmt.Sprintf("mtime > %dd", config.HostLogMaxDays()),
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
		{"claude", filepath.Join(home, ".claude", "debug")},
		{"codex", filepath.Join(home, ".codex", "log")},
		{"opencode", filepath.Join(home, ".cache", "opencode")},
		{"grok", filepath.Join(home, ".grok", "sessions")},
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
	if err != nil || len(cands) == 0 || total <= 0 {
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
