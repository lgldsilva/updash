package scanner

import (
	"context"
	"strings"

	"github.com/lgldsilva/updash/internal/model"
)

// ApkSource scans Alpine packages via apk.
type ApkSource struct{}

func (s *ApkSource) Category() model.Category { return model.CatApk }
func (s *ApkSource) Label() string            { return "apk" }
func (s *ApkSource) Icon() string             { return "🔷" }

func (s *ApkSource) Scan(ctx context.Context, plat model.PlatformInfo) ([]*model.Item, error) {
	// List installed packages older than the repository version.
	out, err := execCombined(ctx, "apk", "version", "-l", "<")
	if err != nil && len(out) == 0 {
		return []*model.Item{errItem("apk", model.CatApk)}, nil
	}
	return okOrOutdated("apk", model.CatApk, ParseApkVersion(string(out))), nil
}

// ParseApkVersion parses `apk version -l '<'` lines of the form:
//
//	musl-1.2.5-r0 < musl-1.2.5-r1
//
// The "Installed:/Available:" header (contains ':') is skipped.
func ParseApkVersion(output string) []*model.Item {
	var items []*model.Item
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.Contains(line, ":") {
			continue
		}
		parts := strings.SplitN(line, " < ", 2)
		if len(parts) != 2 {
			continue
		}
		name, cur := apkSplitNameVer(strings.TrimSpace(parts[0]))
		_, avail := apkSplitNameVer(strings.TrimSpace(parts[1]))
		items = append(items, &model.Item{
			Name:         name,
			Category:     model.CatApk,
			CurrentVer:   cur,
			AvailableVer: avail,
			Status:       model.StatusOutdated,
		})
	}
	return items
}

// apkSplitNameVer splits "musl-1.2.5-r0" into ("musl", "1.2.5-r0").
// The version starts at the last '-' followed by a digit.
func apkSplitNameVer(pkg string) (string, string) {
	for i := len(pkg) - 1; i > 0; i-- {
		if pkg[i-1] == '-' && pkg[i] >= '0' && pkg[i] <= '9' {
			return pkg[:i-1], pkg[i:]
		}
	}
	return pkg, ""
}
