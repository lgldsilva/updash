package config

import (
	"strings"
	"testing"
)

func TestDockerAgesDefault(t *testing.T) {
	t.Setenv("UPDASH_DOCKER_IMAGE_MAX_AGE", "")
	t.Setenv("UPDASH_DOCKER_BUILDER_MAX_AGE", "")
	t.Setenv("UPDASH_DOCKER_CONTAINER_MAX_AGE", "")
	t.Setenv("UPDASH_DOCKER_BUILDER_MODE", "")

	if got := DockerImageMaxAge(); got != DefaultDockerImageMaxAge {
		t.Errorf("DockerImageMaxAge() = %q, want %q", got, DefaultDockerImageMaxAge)
	}
	if got := DockerBuilderMaxAge(); got != DefaultDockerBuilderMaxAge {
		t.Errorf("DockerBuilderMaxAge() = %q, want %q", got, DefaultDockerBuilderMaxAge)
	}
	if got := DockerContainerMaxAge(); got != DefaultDockerContainerMaxAge {
		t.Errorf("DockerContainerMaxAge() = %q, want %q", got, DefaultDockerContainerMaxAge)
	}
	if got := DockerBuilderMode(); got != DefaultDockerBuilderMode {
		t.Errorf("DockerBuilderMode() = %q, want %q", got, DefaultDockerBuilderMode)
	}
}

func TestDockerAgesOverride(t *testing.T) {
	t.Setenv("UPDASH_DOCKER_IMAGE_MAX_AGE", "168h")
	t.Setenv("UPDASH_DOCKER_BUILDER_MAX_AGE", "72h")
	t.Setenv("UPDASH_DOCKER_CONTAINER_MAX_AGE", "24h")

	if got := DockerImageMaxAge(); got != "168h" {
		t.Errorf("DockerImageMaxAge() = %q, want 168h", got)
	}
	if got := DockerBuilderMaxAge(); got != "72h" {
		t.Errorf("DockerBuilderMaxAge() = %q, want 72h", got)
	}
	if got := DockerContainerMaxAge(); got != "24h" {
		t.Errorf("DockerContainerMaxAge() = %q, want 24h", got)
	}
}

func TestDockerBuilderMode(t *testing.T) {
	cases := []struct {
		env  string
		want string
	}{
		{"", DockerBuilderModeAge},
		{"age", DockerBuilderModeAge},
		{"AGE", DockerBuilderModeAge},
		{"all", DockerBuilderModeAll},
		{"ALL", DockerBuilderModeAll},
		{"af", DockerBuilderModeAll},
		{"full", DockerBuilderModeAll},
		{"unfiltered", DockerBuilderModeAll},
		{"nonsense", DockerBuilderModeAge},
	}
	for _, tc := range cases {
		t.Setenv("UPDASH_DOCKER_BUILDER_MODE", tc.env)
		if got := DockerBuilderMode(); got != tc.want {
			t.Errorf("DockerBuilderMode(env=%q) = %q, want %q", tc.env, got, tc.want)
		}
	}
}

func TestIntEnvDefaultsAndOverride(t *testing.T) {
	t.Setenv("UPDASH_CONTAINER_LOG_MAX_MB", "")
	if got := ContainerLogMaxMB(); got != DefaultContainerLogMaxMB {
		t.Errorf("ContainerLogMaxMB() = %d, want %d", got, DefaultContainerLogMaxMB)
	}

	t.Setenv("UPDASH_CONTAINER_LOG_MAX_MB", "100")
	if got := ContainerLogMaxMB(); got != 100 {
		t.Errorf("ContainerLogMaxMB() = %d, want 100", got)
	}

	t.Setenv("UPDASH_CONTAINER_LOG_MAX_MB", "not-a-number")
	if got := ContainerLogMaxMB(); got != DefaultContainerLogMaxMB {
		t.Errorf("invalid env should fall back: got %d, want %d", got, DefaultContainerLogMaxMB)
	}

	t.Setenv("UPDASH_CONTAINER_LOG_MAX_MB", "-5")
	if got := ContainerLogMaxMB(); got != DefaultContainerLogMaxMB {
		t.Errorf("negative env should fall back: got %d, want %d", got, DefaultContainerLogMaxMB)
	}
}

func TestRemainingIntLookups(t *testing.T) {
	t.Setenv("UPDASH_HOST_LOG_MAX_DAYS", "14")
	t.Setenv("UPDASH_DISK_PRESSURE_PCT", "90")
	t.Setenv("UPDASH_DEV_CACHE_MAX_DAYS", "30")
	t.Setenv("UPDASH_AI_OUTPUT_MAX_DAYS", "3")

	if HostLogMaxDays() != 14 {
		t.Errorf("HostLogMaxDays() = %d", HostLogMaxDays())
	}
	if DiskPressurePct() != 90 {
		t.Errorf("DiskPressurePct() = %d", DiskPressurePct())
	}
	if DevCacheMaxDays() != 30 {
		t.Errorf("DevCacheMaxDays() = %d", DevCacheMaxDays())
	}
	if AIOutputMaxDays() != 3 {
		t.Errorf("AIOutputMaxDays() = %d", AIOutputMaxDays())
	}
}

func TestEnvDefaultsListing(t *testing.T) {
	t.Setenv("UPDASH_DOCKER_IMAGE_MAX_AGE", "168h")
	out := EnvDefaults()
	if !strings.Contains(out, "UPDASH_DOCKER_IMAGE_MAX_AGE=168h") {
		t.Errorf("EnvDefaults missing override:\n%s", out)
	}
	for _, key := range []string{
		"UPDASH_DOCKER_BUILDER_MODE",
		"UPDASH_DOCKER_BUILDER_MAX_AGE",
		"UPDASH_DOCKER_CONTAINER_MAX_AGE",
		"UPDASH_CONTAINER_LOG_MAX_MB",
		"UPDASH_HOST_LOG_MAX_DAYS",
		"UPDASH_DISK_PRESSURE_PCT",
		"UPDASH_DEV_CACHE_MAX_DAYS",
		"UPDASH_AI_OUTPUT_MAX_DAYS",
	} {
		if !strings.Contains(out, key+"=") {
			t.Errorf("EnvDefaults missing %s:\n%s", key, out)
		}
	}
}
