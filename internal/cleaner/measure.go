package cleaner

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/lgldsilva/updash/internal/model"
	"github.com/lgldsilva/updash/internal/sizefmt"
)

// FormatBytes renders a byte count in human-readable form.
func FormatBytes(n int64) string {
	return sizefmt.Format(n)
}

func parseOutputFreed(item *model.Item, output string) int64 {
	switch {
	case strings.HasPrefix(item.Name, "brew"):
		return sizefmt.ParseBrewFreed(output)
	case item.Category == model.CatDockerClean:
		return sizefmt.ParseDockerFreed(output)
	default:
		return 0
	}
}

// measurePaths returns total byte size of existing paths (du -sk, 1024-byte blocks).
func measurePaths(ctx context.Context, paths []string) int64 {
	var total int64
	for _, p := range paths {
		if p == "" {
			continue
		}
		if _, err := os.Stat(p); err != nil {
			continue
		}
		out, err := exec.CommandContext(ctx, "du", "-sk", p).Output()
		if err != nil {
			continue
		}
		fields := strings.Fields(string(out))
		if len(fields) == 0 {
			continue
		}
		kb, err := strconv.ParseInt(fields[0], 10, 64)
		if err != nil {
			continue
		}
		total += kb * 1024
	}
	return total
}

func cacheMeasurePaths(item *model.Item) []string {
	home := os.Getenv("HOME")
	switch {
	case strings.HasPrefix(item.Name, "brew"):
		for _, p := range []string{
			filepath.Join(home, "Library", "Caches", "Homebrew"),
			filepath.Join(home, ".cache", "Homebrew"),
		} {
			if _, err := os.Stat(p); err == nil {
				return []string{p}
			}
		}
	case strings.HasPrefix(item.Name, "go"):
		out, err := exec.Command("go", "env", "GOCACHE").Output()
		if err == nil {
			if p := strings.TrimSpace(string(out)); p != "" {
				return []string{p}
			}
		}
	case strings.HasPrefix(item.Name, "npm"):
		base := filepath.Join(home, ".npm")
		var paths []string
		for _, sub := range []string{"_cacache", "_npx"} {
			p := filepath.Join(base, sub)
			if _, err := os.Stat(p); err == nil {
				paths = append(paths, p)
			}
		}
		return paths
	case strings.HasPrefix(item.Name, "apt"):
		return []string{"/var/cache/apt"}
	}
	return nil
}

func computeBytesFreed(ctx context.Context, item *model.Item, output string, before int64) int64 {
	after := measurePaths(ctx, cacheMeasurePaths(item))
	fromOutput := parseOutputFreed(item, output)

	freed := fromOutput
	if delta := before - after; delta > freed {
		freed = delta
	}
	if freed < 0 {
		return 0
	}
	return freed
}
