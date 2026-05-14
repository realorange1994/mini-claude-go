package main

import (
	"os"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Additional env utils tests ported from upstream: src/utils/__tests__/envUtils.test.ts
// These supplement the existing isEnvTruthy tests in beta_headers_env_test.go.
// ---------------------------------------------------------------------------

// IsEnvDefinedFalsy checks if an environment variable is explicitly set to a
// falsy value ("0", "false", "no", "off"). Returns false if the variable is
// unset or set to a truthy value.
// Upstream: isEnvDefinedFalsy() in envUtils.ts
func IsEnvDefinedFalsy(key string) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	return v == "0" || v == "false" || v == "no" || v == "off"
}

// ParseEnvBool parses a boolean environment variable with a default fallback.
// Returns the default value if the variable is unset.
// Upstream: parseEnvBool() in envUtils.ts
func ParseEnvBool(key string, defaultVal bool) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	if v == "" {
		return defaultVal
	}
	return v == "1" || v == "true" || v == "yes" || v == "on"
}

// GetAWSRegion returns the AWS region from environment variables.
// Checks AWS_REGION first, then AWS_DEFAULT_REGION.
// Upstream: getAWSRegion() in envUtils.ts
func GetAWSRegion() string {
	if r := os.Getenv("AWS_REGION"); r != "" {
		return r
	}
	return os.Getenv("AWS_DEFAULT_REGION")
}

// TestIsEnvDefinedFalsyTruthyValues tests that truthy values are NOT considered defined-falsy.
func TestIsEnvDefinedFalsyTruthyValues(t *testing.T) {
	tests := []struct {
		name  string
		value string
	}{
		{"1", "1"},
		{"true", "true"},
		{"yes", "yes"},
		{"on", "on"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := "TEST_ENV_DEFINED_FALSY_TRUTHY_" + tt.name
			os.Setenv(key, tt.value)
			defer os.Unsetenv(key)
			if IsEnvDefinedFalsy(key) {
				t.Errorf("IsEnvDefinedFalsy(%q) with value %q = true, want false", key, tt.value)
			}
		})
	}
}

// TestIsEnvDefinedFalsyFalsyValues tests that falsy values ARE considered defined-falsy.
func TestIsEnvDefinedFalsyFalsyValues(t *testing.T) {
	tests := []struct {
		name  string
		value string
	}{
		{"0", "0"},
		{"false", "false"},
		{"FALSE", "FALSE"},
		{"no", "no"},
		{"off", "off"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := "TEST_ENV_DEFINED_FALSY_FALSY_" + tt.name
			os.Setenv(key, tt.value)
			defer os.Unsetenv(key)
			if !IsEnvDefinedFalsy(key) {
				t.Errorf("IsEnvDefinedFalsy(%q) with value %q = false, want true", key, tt.value)
			}
		})
	}
}

// TestIsEnvDefinedFalsyMissingKey tests that an unset key returns false.
func TestIsEnvDefinedFalsyMissingKey(t *testing.T) {
	key := "TEST_ENV_DEFINED_FALSY_MISSING_XYZ_12345"
	os.Unsetenv(key)
	if IsEnvDefinedFalsy(key) {
		t.Errorf("IsEnvDefinedFalsy(%q) = true for missing key, want false", key)
	}
}

// TestIsEnvDefinedFalsyEmptyString tests that an empty string is NOT defined-falsy.
func TestIsEnvDefinedFalsyEmptyString(t *testing.T) {
	key := "TEST_ENV_DEFINED_FALSY_EMPTY"
	os.Setenv(key, "")
	defer os.Unsetenv(key)
	if IsEnvDefinedFalsy(key) {
		t.Errorf("IsEnvDefinedFalsy(%q) with empty string = true, want false", key)
	}
}

// TestIsEnvDefinedFalsyMixedCase tests case-insensitive matching.
func TestIsEnvDefinedFalsyMixedCase(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  bool
	}{
		{"FaLsE", "FaLsE", true},
		{"No", "No", true},
		{"OfF", "OfF", true},
		{"tRuE", "tRuE", false},
		{"YeS", "YeS", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := "TEST_ENV_DEFINED_FALSY_MIXED_" + tt.name
			os.Setenv(key, tt.value)
			defer os.Unsetenv(key)
			if got := IsEnvDefinedFalsy(key); got != tt.want {
				t.Errorf("IsEnvDefinedFalsy(%q) with value %q = %v, want %v", key, tt.value, got, tt.want)
			}
		})
	}
}

// TestIsEnvDefinedFalsyWhitespaceHandling tests that surrounding whitespace is trimmed.
func TestIsEnvDefinedFalsyWhitespaceHandling(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  bool
	}{
		{"spaced false", " false ", true},
		{"spaced 0", " 0 ", true},
		{"spaced true", " true ", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := "TEST_ENV_DEFINED_FALSY_WS_" + tt.name
			os.Setenv(key, tt.value)
			defer os.Unsetenv(key)
			if got := IsEnvDefinedFalsy(key); got != tt.want {
				t.Errorf("IsEnvDefinedFalsy(%q) with value %q = %v, want %v", key, tt.value, got, tt.want)
			}
		})
	}
}

// TestParseEnvBool tests ParseEnvBool with defaults and overrides.
func TestParseEnvBool(t *testing.T) {
	t.Run("returns default when unset", func(t *testing.T) {
		key := "TEST_PARSE_BOOL_UNSET"
		os.Unsetenv(key)
		if got := ParseEnvBool(key, true); got != true {
			t.Errorf("ParseEnvBool(unset, true) = %v, want true", got)
		}
		if got := ParseEnvBool(key, false); got != false {
			t.Errorf("ParseEnvBool(unset, false) = %v, want false", got)
		}
	})

	t.Run("overrides default with truthy value", func(t *testing.T) {
		key := "TEST_PARSE_BOOL_TRUE"
		os.Setenv(key, "true")
		defer os.Unsetenv(key)
		if got := ParseEnvBool(key, false); got != true {
			t.Errorf("ParseEnvBool(true, false) = %v, want true", got)
		}
	})

	t.Run("overrides default with falsy value", func(t *testing.T) {
		key := "TEST_PARSE_BOOL_FALSE"
		os.Setenv(key, "false")
		defer os.Unsetenv(key)
		if got := ParseEnvBool(key, true); got != false {
			t.Errorf("ParseEnvBool(false, true) = %v, want false", got)
		}
	})

	t.Run("empty string returns default", func(t *testing.T) {
		key := "TEST_PARSE_BOOL_EMPTY"
		os.Setenv(key, "")
		defer os.Unsetenv(key)
		if got := ParseEnvBool(key, true); got != true {
			t.Errorf("ParseEnvBool(empty, true) = %v, want true", got)
		}
	})
}

// TestGetAWSRegion tests AWS region resolution from environment.
func TestGetAWSRegion(t *testing.T) {
	t.Run("returns AWS_REGION when set", func(t *testing.T) {
		os.Setenv("AWS_REGION", "us-west-2")
		os.Unsetenv("AWS_DEFAULT_REGION")
		defer os.Unsetenv("AWS_REGION")
		if got := GetAWSRegion(); got != "us-west-2" {
			t.Errorf("GetAWSRegion() = %q, want us-west-2", got)
		}
	})

	t.Run("falls back to AWS_DEFAULT_REGION", func(t *testing.T) {
		os.Unsetenv("AWS_REGION")
		os.Setenv("AWS_DEFAULT_REGION", "eu-west-1")
		defer os.Unsetenv("AWS_DEFAULT_REGION")
		if got := GetAWSRegion(); got != "eu-west-1" {
			t.Errorf("GetAWSRegion() = %q, want eu-west-1", got)
		}
	})

	t.Run("AWS_REGION takes precedence over AWS_DEFAULT_REGION", func(t *testing.T) {
		os.Setenv("AWS_REGION", "ap-southeast-1")
		os.Setenv("AWS_DEFAULT_REGION", "us-east-1")
		defer os.Unsetenv("AWS_REGION")
		defer os.Unsetenv("AWS_DEFAULT_REGION")
		if got := GetAWSRegion(); got != "ap-southeast-1" {
			t.Errorf("GetAWSRegion() = %q, want ap-southeast-1", got)
		}
	})

	t.Run("returns empty string when neither is set", func(t *testing.T) {
		os.Unsetenv("AWS_REGION")
		os.Unsetenv("AWS_DEFAULT_REGION")
		if got := GetAWSRegion(); got != "" {
			t.Errorf("GetAWSRegion() = %q, want empty string", got)
		}
	})
}
