package sizefmt

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var (
	brewFreedRE   = regexp.MustCompile(`(?i)(?:would free|has freed|freed)\s+approximately\s+([0-9.]+\s*[KMGT]?i?B)`)
	dockerFreedRE = regexp.MustCompile(`(?i)total reclaimed space:\s*([0-9.]+\s*[KMGT]?i?B)`)
	humanSizeRE   = regexp.MustCompile(`(?i)^([0-9]+(?:\.[0-9]+)?)\s*([KMGT]?i?B?)$`)
)

// Format renders a byte count in human-readable form (base 1024).
func Format(n int64) string {
	if n <= 0 {
		return "0B"
	}
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%dB", n)
	}
	div, exp := int64(unit), 0
	for v := n / unit; v >= unit; v /= unit {
		div *= unit
		exp++
	}
	val := float64(n) / float64(div)
	suffix := []string{"KB", "MB", "GB", "TB", "PB"}[exp]
	if val >= 100 || val == float64(int64(val)) {
		return fmt.Sprintf("%.0f%s", val, suffix)
	}
	return fmt.Sprintf("%.1f%s", val, suffix)
}

// Parse converts strings like "15.3MB", "11G", or "3.838GB" to bytes.
func Parse(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" || strings.EqualFold(s, "0B") {
		return 0, nil
	}
	m := humanSizeRE.FindStringSubmatch(strings.ReplaceAll(s, " ", ""))
	if m == nil {
		return 0, fmt.Errorf("unrecognized size %q", s)
	}
	val, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return 0, err
	}
	unit := strings.ToUpper(m[2])
	if unit == "" || unit == "B" {
		return int64(val), nil
	}
	if strings.HasSuffix(unit, "IB") {
		unit = unit[:len(unit)-2]
	} else {
		unit = strings.TrimSuffix(unit, "B")
	}
	mult := int64(1)
	switch unit {
	case "K":
		mult = 1024
	case "M":
		mult = 1024 * 1024
	case "G":
		mult = 1024 * 1024 * 1024
	case "T":
		mult = 1024 * 1024 * 1024 * 1024
	default:
		return 0, fmt.Errorf("unrecognized unit in %q", s)
	}
	return int64(val * float64(mult)), nil
}

// ParseBrewFreed extracts the freed/would-free size from brew cleanup output.
func ParseBrewFreed(output string) int64 {
	m := brewFreedRE.FindStringSubmatch(output)
	if len(m) < 2 {
		return 0
	}
	n, err := Parse(m[1])
	if err != nil {
		return 0
	}
	return n
}

// ParseDockerFreed extracts reclaimed space from docker prune output.
func ParseDockerFreed(output string) int64 {
	m := dockerFreedRE.FindStringSubmatch(output)
	if len(m) < 2 {
		return 0
	}
	n, err := Parse(m[1])
	if err != nil {
		return 0
	}
	return n
}
