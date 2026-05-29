// Package telemetry provides install telemetry detection.
// Aligned to pi's telemetry.ts.
package telemetry

import (
	"os"
	"strings"
)

// IsInstallTelemetryEnabled checks whether install telemetry is enabled.
// It checks the PI_TELEMETRY environment variable first, returning true if set
// to "1", "true", or "yes" (case-insensitive).
func IsInstallTelemetryEnabled() bool {
	val := os.Getenv("PI_TELEMETRY")
	if val == "" {
		return false
	}
	return isTruthyEnvFlag(val)
}

// isTruthyEnvFlag checks if an env value represents "true".
func isTruthyEnvFlag(val string) bool {
	val = strings.TrimSpace(strings.ToLower(val))
	return val == "1" || val == "true" || val == "yes"
}
