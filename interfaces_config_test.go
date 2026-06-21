package main

import (
	"path/filepath"
	"testing"
)

// ─── Interface Tests ────────────────────────────────────────────────────────

func TestInterfaces_Defined(t *testing.T) {
	// Verify interfaces are defined (compile-time check)
	// AgentRunner, ContextManager, MemoryStore, etc. are available
}

// ─── Unified Config Tests ───────────────────────────────────────────────────

func TestUnifiedConfig_New(t *testing.T) {
	config := NewUnifiedConfig()
	if config == nil {
		t.Error("expected non-nil config")
	}
	if config.Agent.MaxTurns != 90 {
		t.Errorf("expected 90 max turns, got %d", config.Agent.MaxTurns)
	}
	if config.Context.MaxContextTokens != 200000 {
		t.Errorf("expected 200000 max context tokens, got %d", config.Context.MaxContextTokens)
	}
}

func TestUnifiedConfig_LoadSave(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	config := NewUnifiedConfig()
	config.Agent.Model = "claude-sonnet-4-20250514"
	config.Agent.MaxTurns = 50

	// Save
	err := SaveConfig(config, path)
	if err != nil {
		t.Fatalf("save failed: %v", err)
	}

	// Load
	loaded, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}

	if loaded.Agent.Model != "claude-sonnet-4-20250514" {
		t.Errorf("expected 'claude-sonnet-4-20250514', got %q", loaded.Agent.Model)
	}
	if loaded.Agent.MaxTurns != 50 {
		t.Errorf("expected 50, got %d", loaded.Agent.MaxTurns)
	}
}

func TestUnifiedConfig_Merge(t *testing.T) {
	base := NewUnifiedConfig()
	base.Agent.Model = "base-model"
	base.Agent.MaxTurns = 90

	override := NewUnifiedConfig()
	override.Agent.Model = "override-model"

	result := MergeConfig(base, override)

	if result.Agent.Model != "override-model" {
		t.Errorf("expected 'override-model', got %q", result.Agent.Model)
	}
	if result.Agent.MaxTurns != 90 {
		t.Errorf("expected 90, got %d", result.Agent.MaxTurns)
	}
}

func TestUnifiedConfig_Load_NonExistent(t *testing.T) {
	_, err := LoadConfig("/nonexistent/config.json")
	if err == nil {
		t.Error("expected error for non-existent file")
	}
}

func TestUnifiedConfig_Defaults(t *testing.T) {
	config := NewUnifiedConfig()

	if !config.Context.MicroCompactEnabled {
		t.Error("expected micro compact enabled by default")
	}
	if !config.Memory.FTSEnabled {
		t.Error("expected FTS enabled by default")
	}
	if !config.Compact.PruneEnabled {
		t.Error("expected prune enabled by default")
	}
	if !config.Compact.AutoContinue {
		t.Error("expected auto continue enabled by default")
	}
	if !config.Tool.TruncationEnabled {
		t.Error("expected truncation enabled by default")
	}
}
