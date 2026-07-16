package config

import (
	"reflect"
	"strings"
	"testing"
)

func TestDockerBuilderPruneArgs_modeAll(t *testing.T) {
	t.Setenv("UPDASH_DOCKER_BUILDER_MODE", "all")
	got := DockerBuilderPruneArgs()
	want := []string{"builder", "prune", "-af"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("DockerBuilderPruneArgs() = %v, want %v", got, want)
	}
	// Must never carry until= in all mode (that was the homelab reclaim-0B bug).
	for _, a := range got {
		if strings.Contains(a, "until=") {
			t.Fatalf("mode=all must not use until= filter, got %v", got)
		}
	}
}

func TestDockerBuilderPruneArgs_modeAge(t *testing.T) {
	t.Setenv("UPDASH_DOCKER_BUILDER_MODE", "age")
	t.Setenv("UPDASH_DOCKER_BUILDER_MAX_AGE", "168h")
	got := DockerBuilderPruneArgs()
	want := []string{"builder", "prune", "--filter", "until=168h", "-f"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("DockerBuilderPruneArgs() = %v, want %v", got, want)
	}
}

func TestDockerBuilderPruneArgs_defaultIsAge(t *testing.T) {
	t.Setenv("UPDASH_DOCKER_BUILDER_MODE", "")
	t.Setenv("UPDASH_DOCKER_BUILDER_MAX_AGE", "")
	got := DockerBuilderPruneArgs()
	want := []string{
		"builder", "prune",
		"--filter", "until=" + DefaultDockerBuilderMaxAge,
		"-f",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("default = %v, want %v", got, want)
	}
}

func TestDockerImagePruneArgs(t *testing.T) {
	t.Setenv("UPDASH_DOCKER_IMAGE_MAX_AGE", "72h")
	got := DockerImagePruneArgs()
	want := []string{"image", "prune", "-a", "--filter", "until=72h", "-f"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestDockerContainerPruneArgs(t *testing.T) {
	t.Setenv("UPDASH_DOCKER_CONTAINER_MAX_AGE", "24h")
	got := DockerContainerPruneArgs()
	want := []string{"container", "prune", "-f", "--filter", "until=24h"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestDockerVolumePruneArgs(t *testing.T) {
	got := DockerVolumePruneArgs()
	want := []string{"volume", "prune", "-f"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestDockerSystemPruneArgs(t *testing.T) {
	t.Setenv("UPDASH_DOCKER_IMAGE_MAX_AGE", "48h")
	got := DockerSystemPruneArgs()
	want := []string{"system", "prune", "-af", "--filter", "until=48h"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestDockerResourceKeepPolicy(t *testing.T) {
	t.Setenv("UPDASH_DOCKER_IMAGE_MAX_AGE", "10h")
	t.Setenv("UPDASH_DOCKER_BUILDER_MAX_AGE", "20h")
	t.Setenv("UPDASH_DOCKER_CONTAINER_MAX_AGE", "30h")

	t.Run("builder age", func(t *testing.T) {
		t.Setenv("UPDASH_DOCKER_BUILDER_MODE", "age")
		got := DockerResourceKeepPolicy("Build Cache")
		if got != "builder mode=age until=20h" {
			t.Fatalf("got %q", got)
		}
	})
	t.Run("builder all", func(t *testing.T) {
		t.Setenv("UPDASH_DOCKER_BUILDER_MODE", "all")
		got := DockerResourceKeepPolicy("build cache")
		if !strings.Contains(got, "mode=all") || !strings.Contains(got, "-af") {
			t.Fatalf("got %q", got)
		}
	})
	t.Run("images", func(t *testing.T) {
		if got := DockerResourceKeepPolicy("Images"); got != "image prune -a until=10h" {
			t.Fatalf("got %q", got)
		}
	})
	t.Run("containers", func(t *testing.T) {
		if got := DockerResourceKeepPolicy("Containers"); got != "container prune until=30h" {
			t.Fatalf("got %q", got)
		}
	})
	t.Run("volumes", func(t *testing.T) {
		if got := DockerResourceKeepPolicy("Local Volumes"); got != "volume prune unused" {
			t.Fatalf("got %q", got)
		}
	})
	t.Run("unknown falls back to system", func(t *testing.T) {
		if got := DockerResourceKeepPolicy("something-else"); got != "system prune until=10h" {
			t.Fatalf("got %q", got)
		}
	})
}
