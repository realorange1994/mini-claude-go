package main

import (
	"testing"
)

func TestResolveModelAliasSonnet(t *testing.T) {
	resolved, ok := ResolveModelAlias("sonnet")
	if !ok {
		t.Fatal("expected alias to resolve")
	}
	if resolved != "claude-sonnet-4-20250514" {
		t.Fatalf("expected claude-sonnet-4-20250514, got %s", resolved)
	}
}

func TestResolveModelAliasOpus(t *testing.T) {
	resolved, ok := ResolveModelAlias("opus")
	if !ok {
		t.Fatal("expected alias to resolve")
	}
	if resolved != "claude-opus-4-20250514" {
		t.Fatalf("expected claude-opus-4-20250514, got %s", resolved)
	}
}

func TestResolveModelAliasHaiku(t *testing.T) {
	resolved, ok := ResolveModelAlias("haiku")
	if !ok {
		t.Fatal("expected alias to resolve")
	}
	if resolved != "claude-haiku-4-5-20250610" {
		t.Fatalf("expected claude-haiku-4-5-20250610, got %s", resolved)
	}
}

func TestResolveModelAliasCaseInsensitive(t *testing.T) {
	resolved, ok := ResolveModelAlias("OPUS")
	if !ok {
		t.Fatal("expected alias to resolve case-insensitively")
	}
	if resolved != "claude-opus-4-20250514" {
		t.Fatalf("expected claude-opus-4-20250514, got %s", resolved)
	}

	resolved2, ok2 := ResolveModelAlias("Sonnet")
	if !ok2 {
		t.Fatal("expected Sonnet alias to resolve case-insensitively")
	}
	if resolved2 != "claude-sonnet-4-20250514" {
		t.Fatalf("expected claude-sonnet-4-20250514, got %s", resolved2)
	}
}

func TestResolveModelAliasFullID(t *testing.T) {
	resolved, ok := ResolveModelAlias("claude-opus-4-20250514")
	if ok {
		t.Fatal("expected full model ID not to be an alias")
	}
	if resolved != "claude-opus-4-20250514" {
		t.Fatalf("expected unchanged, got %s", resolved)
	}
}

func TestResolveModelAliasLegacyRemap(t *testing.T) {
	resolved, ok := ResolveModelAlias("claude-3-opus-20240229")
	if !ok {
		t.Fatal("expected legacy model ID to resolve")
	}
	if resolved != "claude-opus-4-20250514" {
		t.Fatalf("expected claude-opus-4-20250514, got %s", resolved)
	}

	resolved2, ok2 := ResolveModelAlias("claude-3-5-sonnet-20240620")
	if !ok2 {
		t.Fatal("expected legacy sonnet model ID to resolve")
	}
	if resolved2 != "claude-sonnet-4-20250514" {
		t.Fatalf("expected claude-sonnet-4-20250514, got %s", resolved2)
	}
}

func TestResolveModelAliasVariantForms(t *testing.T) {
	tests := []struct {
		alias    string
		expected string
	}{
		{"sonnet4", "claude-sonnet-4-20250514"},
		{"sonnet-4", "claude-sonnet-4-20250514"},
		{"sonnet3.5", "claude-3-5-sonnet-20241022"},
		{"sonnet-3.5", "claude-3-5-sonnet-20241022"},
		{"opus4", "claude-opus-4-20250514"},
		{"opus-4", "claude-opus-4-20250514"},
		{"opus4.5", "claude-opus-4-5-20250610"},
		{"opus-4.5", "claude-opus-4-5-20250610"},
		{"haiku4.5", "claude-haiku-4-5-20250610"},
		{"haiku-4.5", "claude-haiku-4-5-20250610"},
		{"best", "claude-opus-4-20250514"},
		{"fast", "claude-sonnet-4-20250514"},
	}
	for _, tt := range tests {
		resolved, ok := ResolveModelAlias(tt.alias)
		if !ok {
			t.Errorf("alias %q: expected to resolve", tt.alias)
			continue
		}
		if resolved != tt.expected {
			t.Errorf("alias %q: expected %s, got %s", tt.alias, tt.expected, resolved)
		}
	}
}

func TestGetDefaultModel(t *testing.T) {
	tests := []struct {
		subscription string
		expected     string
	}{
		{"enterprise", "claude-opus-4-20250514"},
		{"claude_ai", "claude-sonnet-4-20250514"},
		{"api", "claude-sonnet-4-20250514"},
		{"unknown", "claude-sonnet-4-20250514"},
		{"", "claude-sonnet-4-20250514"},
	}
	for _, tt := range tests {
		got := GetDefaultModel(tt.subscription)
		if got != tt.expected {
			t.Errorf("GetDefaultModel(%q): expected %s, got %s", tt.subscription, tt.expected, got)
		}
	}
}

func TestConfigModelAliasResolution(t *testing.T) {
	// Verify that ResolveModelAlias works on a Config-like model string
	cfg := Config{Model: "sonnet"}
	if resolved, ok := ResolveModelAlias(cfg.Model); ok {
		cfg.Model = resolved
	}
	if cfg.Model != "claude-sonnet-4-20250514" {
		t.Fatalf("expected Config.Model to be resolved to claude-sonnet-4-20250514, got %s", cfg.Model)
	}

	// Full model IDs should remain unchanged
	cfg2 := Config{Model: "claude-opus-4-20250514"}
	if resolved, ok := ResolveModelAlias(cfg2.Model); ok {
		cfg2.Model = resolved
	}
	if cfg2.Model != "claude-opus-4-20250514" {
		t.Fatalf("expected full model ID to remain unchanged, got %s", cfg2.Model)
	}
}
