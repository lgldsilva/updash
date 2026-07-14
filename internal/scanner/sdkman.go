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
	home := os.Getenv("HOME")
	candidatesDir := home + "/.sdkman/candidates"

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
		candidate := entry.Name()

		// Read versions for this candidate
		verDir := filepath.Join(candidatesDir, candidate)
		versions, err := os.ReadDir(verDir)
		if err != nil {
			continue
		}

		var installed []string
		for _, v := range versions {
			if v.IsDir() && v.Name() != "current" {
				installed = append(installed, v.Name())
			}
		}

		if len(installed) == 0 {
			continue
		}

		// Group by major version and find latest per major
		latestPerMajor := groupLatestPerMajor(installed)

		// Create items for each major version group
		for majorVer, latest := range latestPerMajor {
			// Count how many versions total in this major group vs latest
			totalForMajor := 0
			for _, ver := range installed {
				if getMajorVersion(ver) == majorVer {
					totalForMajor++
				}
			}

			removeCount := totalForMajor - 1
			reclaimable := fmt.Sprintf("%d versions", removeCount)

			status := model.StatusOK
			if removeCount > 0 {
				status = model.StatusCleanCandidate
			}

			name := fmt.Sprintf("%s %s", candidate, majorVer)
			items = append(items, &model.Item{
				Name:        name,
				Category:    model.CatSDKMAN,
				CurrentVer:  latest,
				Status:      status,
				Reclaimable: reclaimable,
				KeepPolicy:  "keep latest per major",
				RemoveCount: removeCount,
			})
		}
	}

	if len(items) == 0 {
		items = append(items, &model.Item{
			Name: "sdkman", Category: model.CatSDKMAN, Status: model.StatusOK, CurrentVer: "no candidates",
		})
	}

	return items, nil
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
	// Try to parse numeric part
	n := parts[0]
	// Handle cases like "8.11.1" -> "8"
	re := regexp.MustCompile(`^(\d+)`)
	m := re.FindString(n)
	return m
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

	if len(aParts) < len(bParts) {
		return -1
	}
	if len(aParts) > len(bParts) {
		return 1
	}
	return 0
}

// parseVersionParts splits "21.0.7-tem" into [21, 0, 7].
func parseVersionParts(ver string) []int {
	// Remove trailing identifier after "-"
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
	// Reuse the SDKMAN source logic but filter to cleanup candidates only
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
	// Ensure sort is used (import side effect)
	_ = sort.Ints
}
