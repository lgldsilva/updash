package config

import "strings"

// Shared docker CLI fragments (Sonar go:S1192 — avoid duplicated string literals).
const (
	dockerFilterFlag = "--filter"
	dockerUntilPref  = "until="
)

func untilFilter(maxAge string) []string {
	return []string{dockerFilterFlag, dockerUntilPref + maxAge}
}

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
	args := []string{"builder", "prune"}
	args = append(args, untilFilter(DockerBuilderMaxAge())...)
	return append(args, "-f")
}

// DockerImagePruneArgs returns args for unused-image prune with age filter.
func DockerImagePruneArgs() []string {
	args := []string{"image", "prune", "-a"}
	args = append(args, untilFilter(DockerImageMaxAge())...)
	return append(args, "-f")
}

// DockerContainerPruneArgs returns args for stopped-container prune with age filter.
func DockerContainerPruneArgs() []string {
	args := []string{"container", "prune", "-f"}
	return append(args, untilFilter(DockerContainerMaxAge())...)
}

// DockerVolumePruneArgs returns args for unused volume prune (no age filter).
func DockerVolumePruneArgs() []string {
	return []string{"volume", "prune", "-f"}
}

// DockerSystemPruneArgs returns args for system prune with image age filter.
func DockerSystemPruneArgs() []string {
	args := []string{"system", "prune", "-af"}
	return append(args, untilFilter(DockerImageMaxAge())...)
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
		return "builder mode=age " + dockerUntilPref + DockerBuilderMaxAge()
	case strings.Contains(typ, "image"):
		return "image prune -a " + dockerUntilPref + DockerImageMaxAge()
	case strings.Contains(typ, "container"):
		return "container prune " + dockerUntilPref + DockerContainerMaxAge()
	case strings.Contains(typ, "volume"):
		return "volume prune unused"
	default:
		return "system prune " + dockerUntilPref + DockerImageMaxAge()
	}
}
