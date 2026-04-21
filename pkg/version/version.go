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
	return isVersionCompatible(Version, minVersion)
}

// isVersionCompatible returns true if currentVersion satisfies the given
// minimum version requirement. Extracted for testability (avoids mutating global).
func isVersionCompatible(currentVersion, minVersion string) bool {
	if minVersion == "" || currentVersion == "" || currentVersion == "dev" {
		return true
	}

	currentParts := parseSemver(currentVersion)
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

// Compare returns -1 if a < b, 0 if a == b, 1 if a > b using semver
// comparison on major.minor.patch only. Prerelease suffixes (anything after
// "-") are stripped before comparing, so "1.0.0-rc1" and "1.0.0" compare
// equal. Returns 0 if either input is unparseable.
func Compare(a, b string) int {
	aParts := parseSemver(a)
	bParts := parseSemver(b)
	if aParts == nil || bParts == nil {
		return 0
	}
	for i := 0; i < 3; i++ {
		if aParts[i] < bParts[i] {
			return -1
		}
		if aParts[i] > bParts[i] {
			return 1
		}
	}
	return 0
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
