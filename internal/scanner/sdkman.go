package scanner

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/lgldsilva/updash/internal/model"
)

// SDKMANSource scans SDKMAN candidates.
type SDKMANSource struct{}

func (s *SDKMANSource) Category() model.Category { return model.CatSDKMAN }
func (s *SDKMANSource) Label() string            { return "SDKMAN" }
func (s *SDKMANSource) Icon() string             { return "☕" }

func (s *SDKMANSource) Scan(ctx context.Context, plat model.PlatformInfo) ([]*model.Item, error) {
	candidatesDir := filepath.Join(os.Getenv("HOME"), ".sdkman", "candidates")
	entries, err := os.ReadDir(candidatesDir)
	if err != nil {
		return []*model.Item{
			{Name: "sdkman", Category: model.CatSDKMAN, Status: model.StatusError, CurrentVer: "not found"},
		}, nil
	}

	var items []*model.Item
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == "current" {
			continue
		}
		items = append(items, scanSDKCandidate(filepath.Join(candidatesDir, entry.Name()), entry.Name())...)
	}

	if len(items) == 0 {
		items = append(items, &model.Item{
			Name: "sdkman", Category: model.CatSDKMAN, Status: model.StatusOK, CurrentVer: "no candidates",
		})
	}
	return items, nil
}

func scanSDKCandidate(verDir, candidate string) []*model.Item {
	installed := listSDKInstalled(verDir)
	if len(installed) == 0 {
		return nil
	}
	var items []*model.Item
	for majorVer, latest := range groupLatestPerMajor(installed) {
		items = append(items, sdkMajorItem(candidate, majorVer, latest, installed))
	}
	return items
}

func listSDKInstalled(verDir string) []string {
	versions, err := os.ReadDir(verDir)
	if err != nil {
		return nil
	}
	var installed []string
	for _, v := range versions {
		if v.IsDir() && v.Name() != "current" {
			installed = append(installed, v.Name())
		}
	}
	return installed
}

func sdkMajorItem(candidate, majorVer, latest string, installed []string) *model.Item {
	totalForMajor := 0
	for _, ver := range installed {
		if getMajorVersion(ver) == majorVer {
			totalForMajor++
		}
	}
	removeCount := totalForMajor - 1
	status := model.StatusOK
	if removeCount > 0 {
		status = model.StatusCleanCandidate
	}
	return &model.Item{
		Name:        fmt.Sprintf("%s %s", candidate, majorVer),
		Category:    model.CatSDKMAN,
		CurrentVer:  latest,
		Status:      status,
		Reclaimable: fmt.Sprintf("%d versions", removeCount),
		KeepPolicy:  "keep latest per major",
		RemoveCount: removeCount,
	}
}

// groupLatestPerMajor groups versions by major version and returns the latest of each.
func groupLatestPerMajor(versions []string) map[string]string {
	result := make(map[string]string)
	for _, ver := range versions {
		major := getMajorVersion(ver)
		if major == "" {
			continue
		}
		existing, ok := result[major]
		if !ok || compareVersions(ver, existing) > 0 {
			result[major] = ver
		}
	}
	return result
}

// getMajorVersion extracts the major version from a version string like "21.0.7-tem".
func getMajorVersion(ver string) string {
	parts := strings.Split(ver, ".")
	if len(parts) == 0 {
		return ""
	}
	re := regexp.MustCompile(`^(\d+)`)
	return re.FindString(parts[0])
}

// compareVersions compares two version strings numerically.
// Returns -1 if a < b, 0 if equal, 1 if a > b.
func compareVersions(a, b string) int {
	aParts := parseVersionParts(a)
	bParts := parseVersionParts(b)
	minLen := len(aParts)
	if len(bParts) < minLen {
		minLen = len(bParts)
	}
	for i := 0; i < minLen; i++ {
		if aParts[i] < bParts[i] {
			return -1
		}
		if aParts[i] > bParts[i] {
			return 1
		}
	}
	switch {
	case len(aParts) < len(bParts):
		return -1
	case len(aParts) > len(bParts):
		return 1
	default:
		return 0
	}
}

// parseVersionParts splits "21.0.7-tem" into [21, 0, 7].
func parseVersionParts(ver string) []int {
	if idx := strings.Index(ver, "-"); idx >= 0 {
		ver = ver[:idx]
	}
	parts := strings.Split(ver, ".")
	var nums []int
	for _, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			continue
		}
		nums = append(nums, n)
	}
	return nums
}

// SDKMANCleanSource provides cleanup scanning for SDKMAN.
type SDKMANCleanSource struct{}

func (s *SDKMANCleanSource) Category() model.Category { return model.CatSDKClean }
func (s *SDKMANCleanSource) Label() string            { return "SDKMAN Cleanup" }
func (s *SDKMANCleanSource) Icon() string             { return "🧹" }

func (s *SDKMANCleanSource) Scan(ctx context.Context, plat model.PlatformInfo) ([]*model.Item, error) {
	src := &SDKMANSource{}
	items, err := src.Scan(ctx, plat)
	if err != nil {
		return items, err
	}
	var cleanItems []*model.Item
	for _, it := range items {
		if it.Status == model.StatusCleanCandidate {
			cleanItems = append(cleanItems, it)
		}
	}
	if len(cleanItems) == 0 {
		cleanItems = append(cleanItems, &model.Item{
			Name: "sdkman", Category: model.CatSDKClean, Status: model.StatusOK, CurrentVer: "nothing to clean",
		})
	}
	return cleanItems, nil
}

func init() {
	_ = sort.Ints
}
