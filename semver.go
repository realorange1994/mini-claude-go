package main

import (
	"fmt"
	"strconv"
	"strings"
)

// semver version comparison utilities.
// Ported from upstream: src/utils/semver.ts

// semver represents a parsed semantic version.
type semver struct {
	major      int
	minor      int
	patch      int
	prerelease string
}

// parseSemver parses a version string like "1.2.3" or "1.2.3-alpha" into a semver.
func parseSemver(v string) (semver, error) {
	v = strings.TrimSpace(v)

	var prerelease string
	if idx := strings.Index(v, "-"); idx >= 0 {
		prerelease = v[idx+1:]
		v = v[:idx]
	}

	parts := strings.SplitN(v, ".", 3)
	if len(parts) < 3 {
		return semver{}, fmt.Errorf("invalid version: %q", v)
	}

	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return semver{}, fmt.Errorf("invalid major version: %q", parts[0])
	}
	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return semver{}, fmt.Errorf("invalid minor version: %q", parts[1])
	}
	patch, err := strconv.Atoi(parts[2])
	if err != nil {
		return semver{}, fmt.Errorf("invalid patch version: %q", parts[2])
	}

	return semver{major: major, minor: minor, patch: patch, prerelease: prerelease}, nil
}

// compareSemver compares two semver versions.
// Returns 1 if a > b, -1 if a < b, 0 if equal.
func compareSemver(a, b semver) int {
	if a.major != b.major {
		if a.major > b.major {
			return 1
		}
		return -1
	}
	if a.minor != b.minor {
		if a.minor > b.minor {
			return 1
		}
		return -1
	}
	if a.patch != b.patch {
		if a.patch > b.patch {
			return 1
		}
		return -1
	}

	// Pre-release versions have lower precedence than release versions
	if a.prerelease == "" && b.prerelease == "" {
		return 0
	}
	if a.prerelease != "" && b.prerelease == "" {
		return -1 // a is pre-release, b is release → a < b
	}
	if a.prerelease == "" && b.prerelease != "" {
		return 1 // a is release, b is pre-release → a > b
	}

	// Compare pre-release identifiers lexicographically
	if a.prerelease < b.prerelease {
		return -1
	}
	if a.prerelease > b.prerelease {
		return 1
	}
	return 0
}

// Gt returns true if a > b.
func Gt(a, b string) bool {
	va, errA := parseSemver(a)
	vb, errB := parseSemver(b)
	if errA != nil || errB != nil {
		return false
	}
	return compareSemver(va, vb) == 1
}

// Gte returns true if a >= b.
func Gte(a, b string) bool {
	va, errA := parseSemver(a)
	vb, errB := parseSemver(b)
	if errA != nil || errB != nil {
		return false
	}
	return compareSemver(va, vb) >= 0
}

// Lt returns true if a < b.
func Lt(a, b string) bool {
	va, errA := parseSemver(a)
	vb, errB := parseSemver(b)
	if errA != nil || errB != nil {
		return false
	}
	return compareSemver(va, vb) == -1
}

// Lte returns true if a <= b.
func Lte(a, b string) bool {
	va, errA := parseSemver(a)
	vb, errB := parseSemver(b)
	if errA != nil || errB != nil {
		return false
	}
	return compareSemver(va, vb) <= 0
}

// Order compares two version strings.
// Returns 1 if a > b, -1 if a < b, 0 if equal.
func Order(a, b string) int {
	va, errA := parseSemver(a)
	vb, errB := parseSemver(b)
	if errA != nil || errB != nil {
		return 0
	}
	return compareSemver(va, vb)
}

// Satisfies checks if a version satisfies a range specification.
// Supported ranges:
// - Exact: "1.2.3"
// - Caret: "^1.2.3" (compatible with 1.x.x, allows minor/patch bumps)
// - Tilde: "~1.2.3" (allows patch bumps only)
// - Wildcard: "*" (any version)
// - Comparison: ">=1.0.0", ">1.0.0", "<=1.0.0", "<1.0.0"
func Satisfies(version, rangeSpec string) bool {
	v, err := parseSemver(version)
	if err != nil {
		return false
	}

	rangeSpec = strings.TrimSpace(rangeSpec)

	// Wildcard
	if rangeSpec == "*" {
		return true
	}

	// Caret range: ^1.2.3
	if strings.HasPrefix(rangeSpec, "^") {
		base, err := parseSemver(strings.TrimPrefix(rangeSpec, "^"))
		if err != nil {
			return false
		}
		// Compatible with same major version
		if v.major != base.major {
			return false
		}
		return compareSemver(v, base) >= 0
	}

	// Tilde range: ~1.2.3
	if strings.HasPrefix(rangeSpec, "~") {
		base, err := parseSemver(strings.TrimPrefix(rangeSpec, "~"))
		if err != nil {
			return false
		}
		// Compatible with same major.minor version
		if v.major != base.major || v.minor != base.minor {
			return false
		}
		return compareSemver(v, base) >= 0
	}

	// Comparison operators
	if strings.HasPrefix(rangeSpec, ">=") {
		base, err := parseSemver(strings.TrimPrefix(rangeSpec, ">="))
		if err != nil {
			return false
		}
		return compareSemver(v, base) >= 0
	}
	if strings.HasPrefix(rangeSpec, "<=") {
		base, err := parseSemver(strings.TrimPrefix(rangeSpec, "<="))
		if err != nil {
			return false
		}
		return compareSemver(v, base) <= 0
	}
	if strings.HasPrefix(rangeSpec, ">") && !strings.HasPrefix(rangeSpec, ">=") {
		base, err := parseSemver(strings.TrimPrefix(rangeSpec, ">"))
		if err != nil {
			return false
		}
		return compareSemver(v, base) > 0
	}
	if strings.HasPrefix(rangeSpec, "<") && !strings.HasPrefix(rangeSpec, "<=") {
		base, err := parseSemver(strings.TrimPrefix(rangeSpec, "<"))
		if err != nil {
			return false
		}
		return compareSemver(v, base) < 0
	}

	// Exact match
	base, err := parseSemver(rangeSpec)
	if err != nil {
		return false
	}
	return compareSemver(v, base) == 0
}
