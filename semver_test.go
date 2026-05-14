package main

import (
	"testing"
)

// Ported from upstream: src/utils/__tests__/semver.test.ts
// Tests for gt, gte, lt, lte, satisfies, order

func TestGt(t *testing.T) {
	tests := []struct {
		name string
		a    string
		b    string
		want bool
	}{
		{"returns true when a > b", "2.0.0", "1.0.0", true},
		{"returns false when a < b", "1.0.0", "2.0.0", false},
		{"returns false when equal", "1.0.0", "1.0.0", false},
		{"returns false for 0.0.0 vs 0.0.0", "0.0.0", "0.0.0", false},
		{"release is greater than pre-release", "1.0.0", "1.0.0-alpha", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Gt(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("Gt(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestGte(t *testing.T) {
	tests := []struct {
		name string
		a    string
		b    string
		want bool
	}{
		{"returns true when a > b", "2.0.0", "1.0.0", true},
		{"returns true when equal", "1.0.0", "1.0.0", true},
		{"returns false when a < b", "1.0.0", "2.0.0", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Gte(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("Gte(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestLt(t *testing.T) {
	tests := []struct {
		name string
		a    string
		b    string
		want bool
	}{
		{"returns true when a < b", "1.0.0", "2.0.0", true},
		{"returns false when a > b", "2.0.0", "1.0.0", false},
		{"returns false when equal", "1.0.0", "1.0.0", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Lt(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("Lt(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestLte(t *testing.T) {
	tests := []struct {
		name string
		a    string
		b    string
		want bool
	}{
		{"returns true when a < b", "1.0.0", "2.0.0", true},
		{"returns true when equal", "1.0.0", "1.0.0", true},
		{"returns false when a > b", "2.0.0", "1.0.0", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Lte(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("Lte(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestSatisfies(t *testing.T) {
	tests := []struct {
		name    string
		version string
		range_  string
		want    bool
	}{
		{"matches exact version", "1.2.3", "1.2.3", true},
		{"matches range >=1.0.0", "1.2.3", ">=1.0.0", true},
		{"does not match out-of-range version", "0.9.0", ">=1.0.0", false},
		{"matches caret range ^1.0.0", "1.2.3", "^1.0.0", true},
		{"does not match major bump in caret", "2.0.0", "^1.0.0", false},
		{"matches tilde range ~1.2.3", "1.2.5", "~1.2.3", true},
		{"matches wildcard range *", "2.0.0", "*", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Satisfies(tt.version, tt.range_)
			if got != tt.want {
				t.Errorf("Satisfies(%q, %q) = %v, want %v", tt.version, tt.range_, got, tt.want)
			}
		})
	}
}

func TestOrder(t *testing.T) {
	tests := []struct {
		name string
		a    string
		b    string
		want int
	}{
		{"returns 1 when a > b", "2.0.0", "1.0.0", 1},
		{"returns -1 when a < b", "1.0.0", "2.0.0", -1},
		{"returns 0 when equal", "1.0.0", "1.0.0", 0},
		{"compares patch versions", "1.0.1", "1.0.0", 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Order(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("Order(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
		})
	}
}
