package main

import (
	"os"
	"path/filepath"
	"testing"
)

// ─── DefaultConfig ───────────────────────────────────────────────────────────

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Model != "" {
		t.Errorf("default model should be empty, got %q", cfg.Model)
	}
	if cfg.MaxTurns != 90 {
		t.Errorf("expected MaxTurns=90, got %d", cfg.MaxTurns)
	}
	if cfg.MaxContextMsgs != 100 {
		t.Errorf("expected MaxContextMsgs=100, got %d", cfg.MaxContextMsgs)
	}
	if cfg.PermissionMode != ModeAsk {
		t.Errorf("expected PermissionMode=ask, got %s", cfg.PermissionMode)
	}
	if !cfg.AutoCompactEnabled {
		t.Error("AutoCompact should be enabled by default")
	}
	if !cfg.MicroCompactEnabled {
		t.Error("MicroCompact should be enabled by default")
	}
	if !cfg.SubAgentEnabled {
		t.Error("SubAgent should be enabled by default")
	}
	if !cfg.AutoClassifierEnabled {
		t.Error("AutoClassifier should be enabled by default")
	}
	if cfg.MaxOutputTokens != 16384 {
		t.Errorf("expected MaxOutputTokens=16384, got %d", cfg.MaxOutputTokens)
	}
	if cfg.EscalatedMaxOutputTokens != 64000 {
		t.Errorf("expected EscalatedMaxOutputTokens=64000, got %d", cfg.EscalatedMaxOutputTokens)
	}
	if cfg.Hooks == nil {
		t.Error("Hooks should not be nil in default config")
	}
	if cfg.cachedPrompt == nil {
		t.Error("cachedPrompt should not be nil in default config")
	}
}

func TestDefaultConfigDefaults(t *testing.T) {
	cfg := DefaultConfig()
	// Check compaction defaults
	if cfg.AutoCompactThreshold != 0.75 {
		t.Errorf("expected AutoCompactThreshold=0.75, got %f", cfg.AutoCompactThreshold)
	}
	if cfg.AutoCompactBuffer != 13000 {
		t.Errorf("expected AutoCompactBuffer=13000, got %d", cfg.AutoCompactBuffer)
	}
	if cfg.MicroCompactKeepRecent != 5 {
		t.Errorf("expected MicroCompactKeepRecent=5, got %d", cfg.MicroCompactKeepRecent)
	}
	if cfg.MicroCompactMinCharCount != 2000 {
		t.Errorf("expected MicroCompactMinCharCount=2000, got %d", cfg.MicroCompactMinCharCount)
	}
	if cfg.PostCompactMaxFiles != 5 {
		t.Errorf("expected PostCompactMaxFiles=5, got %d", cfg.PostCompactMaxFiles)
	}
	// Token-based budgets
	if cfg.PostCompactMaxFileTokens != 12500 {
		t.Errorf("expected PostCompactMaxFileTokens=12500, got %d", cfg.PostCompactMaxFileTokens)
	}
	if cfg.PostCompactMaxSkillTokens != 1250 {
		t.Errorf("expected PostCompactMaxSkillTokens=1250, got %d", cfg.PostCompactMaxSkillTokens)
	}
	if cfg.PostCompactMaxTotalSkillTokens != 6250 {
		t.Errorf("expected PostCompactMaxTotalSkillTokens=6250, got %d", cfg.PostCompactMaxTotalSkillTokens)
	}
}

func TestDefaultConfigAllowedCommands(t *testing.T) {
	cfg := DefaultConfig()
	if len(cfg.AllowedCommands) == 0 {
		t.Error("AllowedCommands should not be empty")
	}
	// Check common commands are present
	found := false
	for _, c := range cfg.AllowedCommands {
		if c == "ls" {
			found = true
			break
		}
	}
	if !found {
		t.Error("AllowedCommands should include 'ls'")
	}
}

func TestDefaultConfigDeniedPatterns(t *testing.T) {
	cfg := DefaultConfig()
	if len(cfg.DeniedPatterns) == 0 {
		t.Error("DeniedPatterns should not be empty")
	}
	found := false
	for _, p := range cfg.DeniedPatterns {
		if p == "rm -rf /" {
			found = true
			break
		}
	}
	if !found {
		t.Error("DeniedPatterns should include 'rm -rf /'")
	}
}

// ─── PermissionMode constants ────────────────────────────────────────────────

func TestPermissionModeConstants(t *testing.T) {
	if ModeAsk != "ask" {
		t.Errorf("ModeAsk = %q, want 'ask'", ModeAsk)
	}
	if ModeAuto != "auto" {
		t.Errorf("ModeAuto = %q, want 'auto'", ModeAuto)
	}
	if ModePlan != "plan" {
		t.Errorf("ModePlan = %q, want 'plan'", ModePlan)
	}
	if ModeBypass != "bypass" {
		t.Errorf("ModeBypass = %q, want 'bypass'", ModeBypass)
	}
}

// ─── DefaultRegistry ─────────────────────────────────────────────────────────

func TestDefaultRegistry(t *testing.T) {
	r := DefaultRegistry()
	if r == nil {
		t.Fatal("DefaultRegistry should not return nil")
	}
	tools := r.AllTools()
	if len(tools) == 0 {
		t.Error("DefaultRegistry should have tools")
	}
	// Check key tools are registered
	names := make(map[string]bool)
	for _, t := range tools {
		names[t.Name()] = true
	}
	for _, name := range []string{"exec", "read_file", "write_file", "edit_file", "glob", "grep"} {
		if !names[name] {
			t.Errorf("DefaultRegistry should include tool %q", name)
		}
	}
}

// ─── Config.Close ─────────────────────────────────────────────────────────────

func TestConfigClose(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Close()
	// Should not panic
}

// ─── LoadConfigFromFile with temp config files ────────────────────────────────

func TestLoadConfigFromFileMissingProject(t *testing.T) {
	dir := t.TempDir()
	// No settings files exist in the project dir
	cfg, _ := LoadConfigFromFile(dir)
	// found may be true if home directory has config files
	// Just verify MCPManager is initialized
	if cfg.MCPManager == nil {
		t.Error("MCPManager should always be initialized")
	}
}

func TestLoadConfigFromProjectSettings(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".claude"), 0755)
	settingsJSON := `{
		"env": {
			"ANTHROPIC_AUTH_TOKEN": "sk-test-key",
			"ANTHROPIC_BASE_URL": "https://api.test.com",
			"ANTHROPIC_MODEL": "claude-test-model"
		},
		"mcp": {"servers": {}},
		"permissions": {"allowedCommands": [], "deniedPatterns": []}
	}`
	err := os.WriteFile(filepath.Join(dir, ".claude", "settings.json"), []byte(settingsJSON), 0644)
	if err != nil {
		t.Fatalf("failed to write settings: %v", err)
	}

	cfg, found := LoadConfigFromFile(dir)
	if !found {
		t.Fatal("should return found=true when settings.json exists")
	}
	if cfg.APIKey != "sk-test-key" {
		t.Errorf("expected APIKey=sk-test-key, got %q", cfg.APIKey)
	}
	if cfg.BaseURL != "https://api.test.com" {
		t.Errorf("expected BaseURL=https://api.test.com, got %q", cfg.BaseURL)
	}
	if cfg.Model != "claude-test-model" {
		t.Errorf("expected Model=claude-test-model, got %q", cfg.Model)
	}
}

func TestLoadConfigFromMCPConfig(t *testing.T) {
	dir := t.TempDir()
	mcpJSON := `{
		"mcpServers": {
			"test-server": {
				"command": "python",
				"args": ["-m", "mcp_server"],
				"env": {"TEST": "1"}
			}
		}
	}`
	err := os.WriteFile(filepath.Join(dir, ".mcp.json"), []byte(mcpJSON), 0644)
	if err != nil {
		t.Fatalf("failed to write mcp config: %v", err)
	}

	cfg, found := LoadConfigFromFile(dir)
	if !found {
		t.Fatal("should return found=true when .mcp.json exists")
	}
	servers := cfg.MCPManager.ListServers()
	if len(servers) != 1 {
		t.Errorf("expected 1 MCP server, got %d", len(servers))
	}
	if servers[0] != "test-server" {
		t.Errorf("expected server name 'test-server', got %q", servers[0])
	}
}

func TestLoadConfigFromMCPConfigRemote(t *testing.T) {
	dir := t.TempDir()
	mcpJSON := `{
		"mcpServers": {
			"remote-server": {
				"url": "https://example.com/mcp",
				"env": {"API_KEY": "secret"}
			}
		}
	}`
	err := os.WriteFile(filepath.Join(dir, ".mcp.json"), []byte(mcpJSON), 0644)
	if err != nil {
		t.Fatalf("failed to write mcp config: %v", err)
	}

	cfg, _ := LoadConfigFromFile(dir)
	servers := cfg.MCPManager.ListServers()
	if len(servers) != 1 {
		t.Errorf("expected 1 MCP server, got %d", len(servers))
	}
	if servers[0] != "remote-server" {
		t.Errorf("expected server name 'remote-server', got %q", servers[0])
	}
}

func TestLoadConfigFallback(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".claude"), 0755)
	// Create a malformed settings.json that should still be handled gracefully
	settingsJSON := `{"invalid json`
	err := os.WriteFile(filepath.Join(dir, ".claude", "settings.json"), []byte(settingsJSON), 0644)
	if err != nil {
		t.Fatalf("failed to write settings: %v", err)
	}

	// Should not panic
	cfg, _ := LoadConfigFromFile(dir)
	// found depends on home directory settings
	if cfg.MCPManager == nil {
		t.Error("MCPManager should still be initialized")
	}
}

func TestHomeClaudeDir(t *testing.T) {
	dir := homeClaudeDir()
	// Should return non-empty on Windows (USERPROFILE) or Unix (HOME)
	if dir == "" {
		t.Log("homeClaudeDir returned empty — no HOME or USERPROFILE set")
	}
}
