package main

import (
	"os"
	"testing"
)

// TestIsEnvTruthyTruthyValues tests that all recognized truthy values return true.
// Mirrors upstream: envUtils.test.ts "isEnvTruthy returns true for truthy values"
func TestIsEnvTruthyTruthyValues(t *testing.T) {
	tests := []struct {
		name  string
		value string
	}{
		{"1", "1"},
		{"true", "true"},
		{"TRUE", "TRUE"},
		{"True", "True"},
		{"yes", "yes"},
		{"YES", "YES"},
		{"Yes", "Yes"},
		{"on", "on"},
		{"ON", "ON"},
		{"On", "On"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := "TEST_ENV_TRUTHY_" + tt.name
			os.Setenv(key, tt.value)
			defer os.Unsetenv(key)
			if !isEnvTruthy(key) {
				t.Errorf("isEnvTruthy(%q) with value %q = false, want true", key, tt.value)
			}
		})
	}
}

// TestIsEnvTruthyFalsyValues tests that non-truthy values return false.
// Mirrors upstream: envUtils.test.ts "isEnvTruthy returns false for falsy values"
func TestIsEnvTruthyFalsyValues(t *testing.T) {
	tests := []struct {
		name  string
		value string
	}{
		{"0", "0"},
		{"false", "false"},
		{"FALSE", "FALSE"},
		{"no", "no"},
		{"NO", "NO"},
		{"off", "off"},
		{"OFF", "OFF"},
		{"empty", ""},
		{"random", "maybe"},
		{"2", "2"},
		{"yes_but", "yes please"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := "TEST_ENV_FALSY_" + tt.name
			os.Setenv(key, tt.value)
			defer os.Unsetenv(key)
			if isEnvTruthy(key) {
				t.Errorf("isEnvTruthy(%q) with value %q = true, want false", key, tt.value)
			}
		})
	}
}

// TestIsEnvTruthyMissingKey tests that a missing environment variable returns false.
// Mirrors upstream: envUtils.test.ts "isEnvTruthy returns false for unset variable"
func TestIsEnvTruthyMissingKey(t *testing.T) {
	key := "TEST_ENV_MISSING_KEY_XYZ_12345"
	os.Unsetenv(key)
	if isEnvTruthy(key) {
		t.Errorf("isEnvTruthy(%q) = true for missing key, want false", key)
	}
}

// TestIsEnvTruthyWhitespaceHandling tests that surrounding whitespace is trimmed.
// Mirrors upstream: envUtils.test.ts "isEnvTruthy trims whitespace"
func TestIsEnvTruthyWhitespaceHandling(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  bool
	}{
		{"spaced true", " true ", true},
		{"tabbed true", "\ttrue\t", true},
		{"newline true", "\ntrue\n", true},
		{"spaced 1", " 1 ", true},
		{"spaced yes", " yes ", true},
		{"spaced on", " on ", true},
		{"spaced false", " false ", false},
		{"spaced 0", " 0 ", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := "TEST_ENV_WS_" + tt.name
			os.Setenv(key, tt.value)
			defer os.Unsetenv(key)
			if got := isEnvTruthy(key); got != tt.want {
				t.Errorf("isEnvTruthy(%q) with value %q = %v, want %v", key, tt.value, got, tt.want)
			}
		})
	}
}

// TestIsEnvTruthyMixedCase tests various mixed-case representations.
// Mirrors upstream: envUtils.test.ts "isEnvTruthy handles mixed case"
func TestIsEnvTruthyMixedCase(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  bool
	}{
		{"tRuE", "tRuE", true},
		{"YeS", "YeS", true},
		{"oN", "oN", true},
		{"FaLsE", "FaLsE", false},
		{"No", "No", false},
		{"OfF", "OfF", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := "TEST_ENV_MIXED_" + tt.name
			os.Setenv(key, tt.value)
			defer os.Unsetenv(key)
			if got := isEnvTruthy(key); got != tt.want {
				t.Errorf("isEnvTruthy(%q) with value %q = %v, want %v", key, tt.value, got, tt.want)
			}
		})
	}
}

// TestIsEnvTruthyDoesNotAffectOtherVars tests that checking one key doesn't affect others.
func TestIsEnvTruthyDoesNotAffectOtherVars(t *testing.T) {
	key1 := "TEST_ENV_ISOLATE_A"
	key2 := "TEST_ENV_ISOLATE_B"
	os.Setenv(key1, "true")
	os.Setenv(key2, "0")
	defer os.Unsetenv(key1)
	defer os.Unsetenv(key2)

	if !isEnvTruthy(key1) {
		t.Errorf("isEnvTruthy(%q) = false, want true", key1)
	}
	if isEnvTruthy(key2) {
		t.Errorf("isEnvTruthy(%q) = true, want false", key2)
	}
}
