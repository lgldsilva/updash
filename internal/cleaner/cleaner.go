// Package cleaner executes smart cleanup operations (retention policies, cache cleaning).
package cleaner

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/lgldsilva/updash/internal/elevate"
	"github.com/lgldsilva/updash/internal/model"
)

// Result holds the outcome of a cleanup operation.
type Result struct {
	Item       *model.Item
	Success    bool
	Output     string
	Error      string
	BytesFreed int64 // disk space reclaimed when measurable
}

// CleanAll runs cleanup operations for the given items (silent/buffered — for TUI).
func CleanAll(ctx context.Context, items []*model.Item) []*Result {
	return CleanAllWithOptions(ctx, items, SilentOptions())
}

// CleanAllWithOptions runs cleanup with the given execution options.
func CleanAllWithOptions(ctx context.Context, items []*model.Item, opts Options) []*Result {
	var results []*Result
	for _, it := range items {
		itemCtx, cancel := context.WithTimeout(ctx, ItemTimeout(it))
		results = append(results, CleanOne(itemCtx, it, opts))
		cancel()
	}
	return results
}

// CleanOne cleans a single item.
func CleanOne(ctx context.Context, item *model.Item, opts Options) *Result {
	return cleanOne(ctx, item, opts)
}

func cleanOne(ctx context.Context, item *model.Item, opts Options) *Result {
	item.Status = model.StatusCleaning

	switch item.Category {
	case model.CatCache:
		return cleanCache(ctx, item, opts)
	case model.CatSDKMAN, model.CatSDKClean:
		return cleanSDKMAN(ctx, item, opts)
	case model.CatDockerClean:
		return cleanDocker(ctx, item, opts)
	case model.CatVSCodeClean:
		return cleanVSCodeExt(ctx, item, opts)
	default:
		return &Result{
			Item:    item,
			Success: false,
			Error:   fmt.Sprintf("no cleaner for category %s", item.Category),
		}
	}
}

// cleanCache handles general cache cleanup.
func cleanCache(ctx context.Context, item *model.Item, opts Options) *Result {
	switch {
	case strings.HasPrefix(item.Name, "brew"):
		return runCmd(ctx, item, opts, "brew", "cleanup", "-s")
	case strings.HasPrefix(item.Name, "apt"):
		return runMultiElevatedCmd(ctx, item, opts,
			[]string{"apt-get", "autoremove", "-y"},
			[]string{"apt-get", "autoclean"},
		)
	case strings.HasPrefix(item.Name, "go"):
		return runCmd(ctx, item, opts, "go", "clean", "-cache")
	case strings.HasPrefix(item.Name, "npm"):
		return cleanNpm(ctx, item, opts)
	case strings.HasPrefix(item.Name, "snap"):
		return runElevatedCmd(ctx, item, opts, "snap", "set", "system", "refresh.retain=2")
	case strings.HasPrefix(item.Name, "win"):
		return cleanWindowsCache(ctx, item, opts)
	default:
		return &Result{
			Item:    item,
			Success: true,
			Output:  fmt.Sprintf("%s: cleaned", item.Name),
		}
	}
}

// cleanSDKMAN removes old SDKMAN versions, keeping only the latest per major.
func cleanSDKMAN(ctx context.Context, item *model.Item, opts Options) *Result {
	home := os.Getenv("HOME")
	candidatesDir := filepath.Join(home, ".sdkman", "candidates")

	// Parse item name: "java 21" -> candidate=java, major=21
	parts := strings.Fields(item.Name)
	if len(parts) < 2 {
		return &Result{
			Item:    item,
			Success: false,
			Error:   fmt.Sprintf("cannot parse SDKMAN item: %s", item.Name),
		}
	}
	candidate := parts[0]
	major := parts[1]

	// List installed versions for this candidate
	verDir := filepath.Join(candidatesDir, candidate)
	entries, err := os.ReadDir(verDir)
	if err != nil {
		return &Result{
			Item:    item,
			Success: false,
			Error:   err.Error(),
		}
	}

	// Find which ones to remove
	var removals []string
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == "current" {
			continue
		}
		ver := entry.Name()
		if getMajorVersion(ver) == major && ver != item.CurrentVer {
			removals = append(removals, ver)
		}
	}

	if len(removals) == 0 {
		return &Result{
			Item:    item,
			Success: true,
			Output:  fmt.Sprintf("%s %s: nothing to remove", candidate, major),
		}
	}

	// Remove each old version via sdk uninstall
	var allOutput strings.Builder
	for _, ver := range removals {
		// Sanitize inputs to prevent command injection
		safeCandidate := sanitizeIdent(candidate)
		safeVer := sanitizeIdent(ver)
		cmd := exec.CommandContext(ctx, "bash", "-c",
			fmt.Sprintf("source $HOME/.sdkman/bin/sdkman-init.sh && sdk uninstall %s %s", safeCandidate, safeVer))
		var hadError bool
		if opts.Verbose || opts.Interactive {
			configureCmd(opts, cmd)
			fmt.Fprintf(&allOutput, "removing %s %s...\n", candidate, ver)
			if err := cmd.Run(); err != nil {
				hadError = true
				fmt.Fprintf(&allOutput, "error: %s\n", err)
			}
		} else {
			out, err := cmd.CombinedOutput()
			fmt.Fprintf(&allOutput, "removed %s %s\n", candidate, ver)
			allOutput.Write(out)
			if err != nil {
				hadError = true
				fmt.Fprintf(&allOutput, "error: %s\n", err)
			}
		}
		if hadError {
			item.Status = model.StatusError
			return &Result{
				Item:    item,
				Success: false,
				Error:   fmt.Sprintf("sdk uninstall %s %s failed", candidate, ver),
				Output:  allOutput.String(),
			}
		}
	}

	item.Status = model.StatusCleaned
	return &Result{
		Item:    item,
		Success: true,
		Output:  allOutput.String(),
	}
}

// cleanDocker prunes Docker resources.
// Uses --filter "until=336h" (14 days) to avoid removing recent images/containers.
func cleanDocker(ctx context.Context, item *model.Item, opts Options) *Result {
	switch {
	case strings.Contains(item.Name, "images"):
		return runCmd(ctx, item, opts, "docker", "image", "prune", "-a", "--filter", "until=336h", "-f")
	case strings.Contains(item.Name, "builder") || strings.Contains(item.Name, "build"):
		return runCmd(ctx, item, opts, "docker", "builder", "prune", "--filter", "until=336h", "-f")
	case strings.Contains(item.Name, "container"):
		return runCmd(ctx, item, opts, "docker", "container", "prune", "-f", "--filter", "until=336h")
	case strings.Contains(item.Name, "volume"):
		return runCmd(ctx, item, opts, "docker", "volume", "prune", "-f")
	default:
		return runCmd(ctx, item, opts, "docker", "system", "prune", "-af", "--filter", "until=336h")
	}
}

// cleanVSCodeExt removes old versions of VS Code extensions.
func cleanVSCodeExt(ctx context.Context, item *model.Item, opts Options) *Result {
	// Determine the extensions directory from the item name or context
	var candidates []string
	home := os.Getenv("HOME")
	for _, dir := range []string{
		filepath.Join(home, ".antigravity", "extensions"),
		filepath.Join(home, ".antigravity-ide", "extensions"),
	} {
		if _, err := os.Stat(dir); err == nil {
			candidates = append(candidates, dir)
		}
	}

	if len(candidates) == 0 {
		return &Result{
			Item:    item,
			Success: true,
			Output:  "no extension directories found",
		}
	}

	// Parse "ext: publisher.name" from item.Name
	extName := strings.TrimPrefix(item.Name, "ext: ")
	extName = strings.TrimSpace(extName)

	if extName == "" {
		return &Result{
			Item:    item,
			Success: false,
			Error:   "cannot parse extension name",
		}
	}

	var allOutput strings.Builder
	for _, extDir := range candidates {
		entries, err := os.ReadDir(extDir)
		if err != nil {
			continue
		}

		re := regexp.MustCompile(fmt.Sprintf(`^%s-(\d+\.\d+\.\d+)(?:-.+)?$`, regexp.QuoteMeta(extName)))

		var versions []string
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			m := re.FindStringSubmatch(entry.Name())
			if m != nil {
				versions = append(versions, entry.Name())
			}
		}

		// Sort descending to keep latest
		sort.Slice(versions, func(i, j int) bool {
			return compareVersions(extractVersion(versions[i]), extractVersion(versions[j])) > 0
		})

		if len(versions) <= 1 {
			continue
		}

		// Remove all but first (latest)
		for _, old := range versions[1:] {
			oldPath := filepath.Join(extDir, old)
			err := os.RemoveAll(oldPath)
			if err != nil {
				fmt.Fprintf(&allOutput, "error removing %s: %s\n", old, err)
			} else {
				fmt.Fprintf(&allOutput, "removed %s\n", old)
			}
		}
	}

	var hadError bool
	if allOutput.Len() > 0 && strings.Contains(allOutput.String(), "error removing") {
		hadError = true
	}
	if hadError {
		item.Status = model.StatusError
		return &Result{
			Item:    item,
			Success: false,
			Error:   "some extension versions could not be removed",
			Output:  allOutput.String(),
		}
	}

	item.Status = model.StatusCleaned
	return &Result{
		Item:    item,
		Success: true,
		Output:  allOutput.String(),
	}
}

// --- Helpers ---

func runElevatedCmd(ctx context.Context, item *model.Item, opts Options, name string, args ...string) *Result {
	return runCmdWithBuilder(ctx, item, elevate.Sudo(ctx, name, args...), opts)
}

func runCmd(ctx context.Context, item *model.Item, opts Options, name string, args ...string) *Result {
	return runCmdWithBuilder(ctx, item, exec.CommandContext(ctx, name, args...), opts)
}

func runCmdWithBuilder(ctx context.Context, item *model.Item, cmd *exec.Cmd, opts Options) *Result {
	paths := cacheMeasurePaths(item)
	before := measurePaths(ctx, paths)
	result := executeCmd(ctx, item, cmd, opts)
	attachFreed(ctx, item, result, before)
	return result
}

func executeCmd(ctx context.Context, item *model.Item, cmd *exec.Cmd, opts Options) *Result {
	var stdout, stderr bytes.Buffer
	if opts.Verbose || opts.Interactive {
		configureCmd(opts, cmd)
	} else {
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
	}

	err := cmd.Run()
	result := &Result{Item: item}

	if err != nil {
		result.Success = false
		result.Error = err.Error()
		result.Output = stderr.String() + stdout.String()
		item.Status = model.StatusError
	} else {
		result.Success = true
		result.Output = stdout.String()
		item.Status = model.StatusCleaned
	}

	return result
}

func attachFreed(ctx context.Context, item *model.Item, result *Result, before int64) {
	if !result.Success {
		return
	}
	result.BytesFreed = computeBytesFreed(ctx, item, result.Output, before)
	if result.BytesFreed > 0 {
		item.Freed = FormatBytes(result.BytesFreed)
	} else {
		item.Freed = "0B"
	}
}

// cleanNpm clears the npm content cache and stale npx extraction dirs under ~/.npm.
func cleanNpm(ctx context.Context, item *model.Item, opts Options) *Result {
	paths := cacheMeasurePaths(item)
	before := measurePaths(ctx, paths)

	result := executeCmd(ctx, item, exec.CommandContext(ctx, "npm", "cache", "clean", "--force"), opts)
	if result.Success {
		var npxOut strings.Builder
		npxDir := filepath.Join(os.Getenv("HOME"), ".npm", "_npx")
		entries, err := os.ReadDir(npxDir)
		if err == nil {
			for _, entry := range entries {
				p := filepath.Join(npxDir, entry.Name())
				if rmErr := os.RemoveAll(p); rmErr != nil {
					fmt.Fprintf(&npxOut, "npx remove %s: %v\n", entry.Name(), rmErr)
				}
			}
		}
		if npxOut.Len() > 0 {
			result.Output += npxOut.String()
		}
	}

	attachFreed(ctx, item, result, before)
	return result
}

func runMultiElevatedCmd(ctx context.Context, item *model.Item, opts Options, cmds ...[]string) *Result {
	var allOutput strings.Builder
	var lastErr error
	for _, args := range cmds {
		cmd := elevate.Sudo(ctx, args[0], args[1:]...)
		if opts.Verbose || opts.Interactive {
			configureCmd(opts, cmd)
			if err := cmd.Run(); err != nil {
				lastErr = err
				fmt.Fprintf(&allOutput, "error: %s\n", err)
			}
		} else {
			out, err := cmd.CombinedOutput()
			allOutput.Write(out)
			if err != nil {
				lastErr = err
				fmt.Fprintf(&allOutput, "error: %s\n", err)
			}
		}
	}
	result := &Result{
		Item:   item,
		Output: allOutput.String(),
	}
	if lastErr != nil {
		result.Success = false
		result.Error = lastErr.Error()
		item.Status = model.StatusError
	} else {
		result.Success = true
		item.Status = model.StatusCleaned
	}
	return result
}

func getMajorVersion(ver string) string {
	re := regexp.MustCompile(`^(\d+)`)
	m := re.FindString(ver)
	return m
}

func extractVersion(dirName string) string {
	re := regexp.MustCompile(`-(\d+\.\d+\.\d+)`)
	m := re.FindStringSubmatch(dirName)
	if len(m) >= 2 {
		return m[1]
	}
	return ""
}

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

func parseVersionParts(ver string) []int {
	if idx := strings.Index(ver, "-"); idx >= 0 {
		ver = ver[:idx]
	}
	parts := strings.Split(ver, ".")
	var nums []int
	for _, p := range parts {
		var n int
		if _, err := fmt.Sscanf(p, "%d", &n); err != nil {
			n = 0
		}
		nums = append(nums, n)
	}
	return nums
}

// cleanWindowsCache handles Windows temp/cache cleanup.
func cleanWindowsCache(ctx context.Context, item *model.Item, opts Options) *Result {
	switch {
	case strings.Contains(item.Name, "temp") || strings.Contains(item.Name, "TEMP"):
		return runCmd(ctx, item, opts, "cmd", "/c", "del /q /s %TEMP%\\* >nul 2>&1")
	case strings.Contains(item.Name, "npm"):
		return runCmd(ctx, item, opts, "npm", "cache", "clean", "--force")
	default:
		return runCmd(ctx, item, opts, "cmd", "/c", "echo No Windows cleaner defined")
	}
}

// sanitizeIdent strips characters that could be used for command injection.
// Allows: alphanumeric, dots, hyphens, underscores.
func sanitizeIdent(s string) string {
	var safe strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' || r == '-' || r == '_' {
			safe.WriteRune(r)
		}
	}
	return safe.String()
}
