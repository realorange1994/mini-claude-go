// Package httpconfig provides HTTP client configuration constants
// and utilities aligned to pi's http-dispatcher.ts.
package httpconfig

import (
	"fmt"
	"time"
)

const (
	// DefaultHttpIdleTimeoutMs is the default HTTP idle timeout (5 minutes).
	DefaultHttpIdleTimeoutMs = 300000
	// MinHttpIdleTimeoutMs is the minimum HTTP idle timeout (30 seconds).
	MinHttpIdleTimeoutMs = 30000
	// MaxHttpIdleTimeoutMs is the maximum HTTP idle timeout (5 minutes).
	MaxHttpIdleTimeoutMs = 300000
	// HttpIdleTimeoutDisabled disables the HTTP idle timeout.
	HttpIdleTimeoutDisabled = 0
)

// HttpIdleTimeoutChoices returns available HTTP idle timeout options in ms.
func HttpIdleTimeoutChoices() []int {
	return []int{30000, 60000, 120000, 300000, 0}
}

// ParseHttpIdleTimeoutMs parses an HTTP idle timeout value.
// Returns the value if valid (0-300000ms), or 0 (disabled) for invalid input.
func ParseHttpIdleTimeoutMs(value interface{}) (int, error) {
	switch v := value.(type) {
	case int:
		if v < 0 {
			return 0, nil
		}
		return v, nil
	case float64:
		if v < 0 {
			return 0, nil
		}
		return int(v), nil
	case string:
		if v == "" || v == "disabled" {
			return 0, nil
		}
		var d time.Duration
		var err error
		switch v {
		case "30s":
			d = 30 * time.Second
		case "1m":
			d = time.Minute
		case "2m":
			d = 2 * time.Minute
		case "5m":
			d = 5 * time.Minute
		case "disabled":
			d = 0
		default:
			d, err = time.ParseDuration(v)
			if err != nil {
				return DefaultHttpIdleTimeoutMs, nil
			}
		}
		return int(d.Milliseconds()), nil
	default:
		return DefaultHttpIdleTimeoutMs, nil
	}
}

// FormatHttpIdleTimeoutMs formats an idle timeout value as a human-readable label.
func FormatHttpIdleTimeoutMs(timeoutMs int) string {
	if timeoutMs == 0 {
		return "disabled"
	}
	d := time.Duration(timeoutMs) * time.Millisecond
	if d < time.Minute {
		return fmt.Sprintf("%.0fs", d.Seconds())
	}
	if d < time.Hour {
		return fmt.Sprintf("%.0fm", d.Minutes())
	}
	return d.String()
}
