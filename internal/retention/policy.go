package retention

import (
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// IsOlderThan reports whether modTime is older than maxDays relative to now.
// maxDays <= 0 means "never older" (retain everything).
func IsOlderThan(modTime time.Time, maxDays int, now time.Time) bool {
	if maxDays <= 0 {
		return false
	}
	cutoff := now.AddDate(0, 0, -maxDays)
	return modTime.Before(cutoff)
}

// DiskPressureTriggered reports whether used percent is at or above the threshold.
func DiskPressureTriggered(usedPct, threshold int) bool {
	if threshold <= 0 {
		return false
	}
	return usedPct >= threshold
}

// PathCandidate is a file/dir selected for age-based cleanup.
type PathCandidate struct {
	Path string
	Size int64
}

// CollectOldPaths walks root (max depth) and returns entries whose mtime is older
// than maxDays. Skips missing roots. Depth 0 means root itself only; depth 1
// means immediate children (typical for cache dirs).
func CollectOldPaths(root string, maxDays, maxDepth int, now time.Time) ([]PathCandidate, int64, error) {
	info, err := os.Stat(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, 0, nil
		}
		return nil, 0, err
	}
	if maxDepth <= 0 {
		if !IsOlderThan(info.ModTime(), maxDays, now) {
			return nil, 0, nil
		}
		sz := dirSize(root)
		return []PathCandidate{{Path: root, Size: sz}}, sz, nil
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, 0, err
	}
	var out []PathCandidate
	var total int64
	for _, e := range entries {
		p := filepath.Join(root, e.Name())
		fi, err := e.Info()
		if err != nil {
			continue
		}
		if !IsOlderThan(fi.ModTime(), maxDays, now) {
			continue
		}
		sz := int64(0)
		if e.IsDir() {
			sz = dirSize(p)
		} else {
			sz = fi.Size()
		}
		out = append(out, PathCandidate{Path: p, Size: sz})
		total += sz
	}
	return out, total, nil
}

func dirSize(root string) int64 {
	var total int64
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		fi, err := d.Info()
		if err != nil {
			return nil
		}
		total += fi.Size()
		return nil
	})
	return total
}

// RemovePaths deletes each path (file or directory). Returns freed bytes best-effort.
func RemovePaths(paths []string) (freed int64, errs []string) {
	for _, p := range paths {
		fi, err := os.Stat(p)
		if err != nil {
			errs = append(errs, p+": "+err.Error())
			continue
		}
		var sz int64
		if fi.IsDir() {
			sz = dirSize(p)
		} else {
			sz = fi.Size()
		}
		if err := os.RemoveAll(p); err != nil {
			errs = append(errs, p+": "+err.Error())
			continue
		}
		freed += sz
	}
	return freed, errs
}

// TruncateFileIfOver writes empty content when size exceeds maxBytes.
// maxBytes <= 0 disables truncation.
func TruncateFileIfOver(path string, maxBytes int64) (truncated bool, before int64, err error) {
	if maxBytes <= 0 {
		return false, 0, nil
	}
	fi, err := os.Stat(path)
	if err != nil {
		return false, 0, err
	}
	if fi.IsDir() {
		return false, 0, nil
	}
	before = fi.Size()
	if before <= maxBytes {
		return false, before, nil
	}
	if err := os.Truncate(path, 0); err != nil {
		return false, before, err
	}
	return true, before, nil
}
