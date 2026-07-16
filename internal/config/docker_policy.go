package config

import "strings"

// DockerBuilderPruneArgs returns the docker subcommand args for builder prune
// (everything after "docker"). Pure policy — no I/O — so unit tests can assert
// exact CLI without talking to a daemon.
//
//	mode=all → builder prune -af
//	mode=age → builder prune --filter until=<max> -f
func DockerBuilderPruneArgs() []string {
	if DockerBuilderMode() == DockerBuilderModeAll {
		return []string{"builder", "prune", "-af"}
	}
	return []string{
		"builder", "prune",
		"--filter", "until=" + DockerBuilderMaxAge(),
		"-f",
	}
}

// DockerImagePruneArgs returns args for unused-image prune with age filter.
func DockerImagePruneArgs() []string {
	return []string{
		"image", "prune", "-a",
		"--filter", "until=" + DockerImageMaxAge(),
		"-f",
	}
}

// DockerContainerPruneArgs returns args for stopped-container prune with age filter.
func DockerContainerPruneArgs() []string {
	return []string{
		"container", "prune", "-f",
		"--filter", "until=" + DockerContainerMaxAge(),
	}
}

// DockerVolumePruneArgs returns args for unused volume prune (no age filter).
func DockerVolumePruneArgs() []string {
	return []string{"volume", "prune", "-f"}
}

// DockerSystemPruneArgs returns args for system prune with image age filter.
func DockerSystemPruneArgs() []string {
	return []string{
		"system", "prune", "-af",
		"--filter", "until=" + DockerImageMaxAge(),
	}
}

// DockerResourceKeepPolicy is the human-readable prune policy for a
// `docker system df` type string (images, build cache, containers, …).
// Used in scan KeepPolicy so operators see what clean will actually run.
func DockerResourceKeepPolicy(typ string) string {
	typ = strings.ToLower(typ)
	switch {
	case strings.Contains(typ, "build"):
		if DockerBuilderMode() == DockerBuilderModeAll {
			return "builder mode=all (prune -af, unused cache only)"
		}
		return "builder mode=age until=" + DockerBuilderMaxAge()
	case strings.Contains(typ, "image"):
		return "image prune -a until=" + DockerImageMaxAge()
	case strings.Contains(typ, "container"):
		return "container prune until=" + DockerContainerMaxAge()
	case strings.Contains(typ, "volume"):
		return "volume prune unused"
	default:
		return "system prune until=" + DockerImageMaxAge()
	}
}
