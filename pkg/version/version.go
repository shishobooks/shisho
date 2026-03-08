package version

import (
	"strconv"
	"strings"
)

// Version is the application version, set at build time via ldflags.
// Example: go build -ldflags "-X github.com/shishobooks/shisho/pkg/version.Version=1.0.0".
var Version = "dev"

// IsCompatible returns true if the current Version satisfies the given
// minimum version requirement. Returns true if either version is empty,
// "dev", or not parseable as semver (permissive by default).
func IsCompatible(minVersion string) bool {
	if minVersion == "" || Version == "" || Version == "dev" {
		return true
	}

	currentParts := parseSemver(Version)
	minParts := parseSemver(minVersion)
	if currentParts == nil || minParts == nil {
		return true // Can't parse, assume compatible
	}

	for i := 0; i < 3; i++ {
		if currentParts[i] > minParts[i] {
			return true
		}
		if currentParts[i] < minParts[i] {
			return false
		}
	}
	return true // Equal
}

// parseSemver extracts [major, minor, patch] from a semver string.
// Returns nil if the string cannot be parsed.
func parseSemver(s string) []int {
	// Strip leading "v" if present
	s = strings.TrimPrefix(s, "v")
	// Strip anything after a hyphen (pre-release)
	if idx := strings.Index(s, "-"); idx >= 0 {
		s = s[:idx]
	}

	parts := strings.Split(s, ".")
	if len(parts) < 1 || len(parts) > 3 {
		return nil
	}

	result := make([]int, 3)
	for i, part := range parts {
		n, err := strconv.Atoi(part)
		if err != nil {
			return nil
		}
		result[i] = n
	}
	return result
}
