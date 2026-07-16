// Package config provides environment-variable overrides for updash retention
// and cleanup policies. Defaults match the previous hardcoded behaviour so
// existing installs stay conservative unless the operator opts in.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Docker prune age defaults (14 days). Homelab scripts often use 168h (7d);
// override via UPDASH_DOCKER_* when tighter retention is desired.
//
// Builder prune is special: age filters often reclaim 0B on busy CI hosts
// because build layers stay "recent". Use UPDASH_DOCKER_BUILDER_MODE=all
// for unfiltered `docker builder prune -af` (safe: unused build cache only).
const (
	DefaultDockerImageMaxAge     = "336h"
	DefaultDockerBuilderMaxAge   = "336h"
	DefaultDockerContainerMaxAge = "336h"

	// Builder prune modes.
	DockerBuilderModeAge = "age" // --filter until=<max age>
	DockerBuilderModeAll = "all" // no until= (builder prune -af)

	DefaultDockerBuilderMode = DockerBuilderModeAge

	DefaultContainerLogMaxMB = 50
	DefaultHostLogMaxDays    = 30
	DefaultDiskPressurePct   = 85
	DefaultDevCacheMaxDays   = 90
	DefaultAIOutputMaxDays   = 7
)

// DockerImageMaxAge is the docker image prune --filter until= value.
func DockerImageMaxAge() string {
	return envOr("UPDASH_DOCKER_IMAGE_MAX_AGE", DefaultDockerImageMaxAge)
}

// DockerBuilderMaxAge is the docker builder prune --filter until= value
// (only used when DockerBuilderMode is "age").
func DockerBuilderMaxAge() string {
	return envOr("UPDASH_DOCKER_BUILDER_MAX_AGE", DefaultDockerBuilderMaxAge)
}

// DockerBuilderMode returns how builder prune is applied:
//   - "age" (default): docker builder prune --filter until=<max age>
//   - "all": docker builder prune -af (no until=; recommended for CI/homelab)
func DockerBuilderMode() string {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("UPDASH_DOCKER_BUILDER_MODE")))
	switch v {
	case "", DockerBuilderModeAge:
		return DockerBuilderModeAge
	case DockerBuilderModeAll, "af", "full", "unfiltered":
		return DockerBuilderModeAll
	default:
		return DockerBuilderModeAge
	}
}

// DockerContainerMaxAge is the docker container prune --filter until= value.
func DockerContainerMaxAge() string {
	return envOr("UPDASH_DOCKER_CONTAINER_MAX_AGE", DefaultDockerContainerMaxAge)
}

// ContainerLogMaxMB is the size threshold for truncating container logs.
func ContainerLogMaxMB() int {
	return envInt("UPDASH_CONTAINER_LOG_MAX_MB", DefaultContainerLogMaxMB)
}

// HostLogMaxDays is the mtime age for host log cleanup.
func HostLogMaxDays() int {
	return envInt("UPDASH_HOST_LOG_MAX_DAYS", DefaultHostLogMaxDays)
}

// DiskPressurePct is the disk-usage percent that triggers aggressive prune.
func DiskPressurePct() int {
	return envInt("UPDASH_DISK_PRESSURE_PCT", DefaultDiskPressurePct)
}

// DevCacheMaxDays is the atime age for maven/gradle (and similar) cache cleanup.
func DevCacheMaxDays() int {
	return envInt("UPDASH_DEV_CACHE_MAX_DAYS", DefaultDevCacheMaxDays)
}

// AIOutputMaxDays is the mtime age for AI tool-output cleanup.
func AIOutputMaxDays() int {
	return envInt("UPDASH_AI_OUTPUT_MAX_DAYS", DefaultAIOutputMaxDays)
}

// EnvDefaults returns a multi-line listing of every UPDASH_* retention var
// with its effective value (env override or built-in default).
func EnvDefaults() string {
	var b strings.Builder
	rows := []struct {
		key, value, note string
	}{
		{"UPDASH_DOCKER_IMAGE_MAX_AGE", DockerImageMaxAge(), "docker image prune until="},
		{"UPDASH_DOCKER_BUILDER_MODE", DockerBuilderMode(), "builder prune: age|all (CI/homelab: all)"},
		{"UPDASH_DOCKER_BUILDER_MAX_AGE", DockerBuilderMaxAge(), "builder prune until= (mode=age only)"},
		{"UPDASH_DOCKER_CONTAINER_MAX_AGE", DockerContainerMaxAge(), "docker container prune until="},
		{"UPDASH_CONTAINER_LOG_MAX_MB", strconv.Itoa(ContainerLogMaxMB()), "container log truncate threshold"},
		{"UPDASH_HOST_LOG_MAX_DAYS", strconv.Itoa(HostLogMaxDays()), "host log mtime age"},
		{"UPDASH_DISK_PRESSURE_PCT", strconv.Itoa(DiskPressurePct()), "disk % for aggressive prune"},
		{"UPDASH_DEV_CACHE_MAX_DAYS", strconv.Itoa(DevCacheMaxDays()), "maven/gradle cache atime age"},
		{"UPDASH_AI_OUTPUT_MAX_DAYS", strconv.Itoa(AIOutputMaxDays()), "AI tool-output mtime age"},
	}
	for _, r := range rows {
		fmt.Fprintf(&b, "%s=%s  # %s\n", r.key, r.value, r.note)
	}
	return b.String()
}

func envOr(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return def
	}
	return n
}
