package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// ─── Unified Configuration ──────────────────────────────────────────────────
//
// Centralized configuration management for all modules.
// Replaces scattered config structs with a unified approach.

// UnifiedConfig holds all configuration for the agent system.
type UnifiedConfig struct {
	mu sync.RWMutex

	// Agent configuration
	Agent AgentConfig `json:"agent"`

	// Context configuration
	Context ContextConfig `json:"context"`

	// Memory configuration
	Memory MemoryConfig `json:"memory"`

	// Tool configuration
	Tool ToolConfig `json:"tool"`

	// Compact configuration
	Compact CompactConfig `json:"compact"`

	// LSP configuration
	LSP LSPConfig `json:"lsp"`

	// Plugin configuration
	Plugin PluginConfig `json:"plugin"`

	// Workflow configuration
	Workflow WorkflowConfig `json:"workflow"`
}

// AgentConfig holds agent-specific configuration.
type AgentConfig struct {
	Model            string `json:"model"`
	MaxTurns         int    `json:"max_turns"`
	PermissionMode   string `json:"permission_mode"`
	StreamEnabled    bool   `json:"stream_enabled"`
	ThinkingBudget   int    `json:"thinking_budget"`
}

// ContextConfig holds context management configuration.
type ContextConfig struct {
	MaxContextTokens    int     `json:"max_context_tokens"`
	PressureThreshold   float64 `json:"pressure_threshold"`
	MicroCompactEnabled bool    `json:"micro_compact_enabled"`
}

// MemoryConfig holds memory system configuration.
type MemoryConfig struct {
	GlobalPath    string `json:"global_path"`
	ProjectPath   string `json:"project_path"`
	SessionPath   string `json:"session_path"`
	MaxEntries    int    `json:"max_entries"`
	FTSEnabled    bool   `json:"fts_enabled"`
}

// ToolConfig holds tool system configuration.
type ToolConfig struct {
	MaxToolChars    int  `json:"max_tool_chars"`
	ToolTimeoutMs   int  `json:"tool_timeout_ms"`
	TruncationEnabled bool `json:"truncation_enabled"`
}

// CompactConfig holds compaction configuration.
type CompactConfig struct {
	Threshold       float64 `json:"threshold"`
	KeepRounds      int     `json:"keep_rounds"`
	PruneEnabled    bool    `json:"prune_enabled"`
	AutoContinue    bool    `json:"auto_continue"`
}

// LSPConfig holds LSP configuration.
type LSPConfig struct {
	Enabled    bool     `json:"enabled"`
	Servers    []string `json:"servers"`
	Timeout    int      `json:"timeout"`
}

// PluginConfig holds plugin configuration.
type PluginConfig struct {
	Enabled    bool   `json:"enabled"`
	PluginDir  string `json:"plugin_dir"`
}

// WorkflowConfig holds workflow configuration.
type WorkflowConfig struct {
	Enabled     bool   `json:"enabled"`
	WorkflowDir string `json:"workflow_dir"`
}

// NewUnifiedConfig creates a new unified configuration with defaults.
func NewUnifiedConfig() *UnifiedConfig {
	return &UnifiedConfig{
		Agent: AgentConfig{
			MaxTurns:       90,
			StreamEnabled:  true,
			ThinkingBudget: 10000,
		},
		Context: ContextConfig{
			MaxContextTokens:    200000,
			PressureThreshold:   0.75,
			MicroCompactEnabled: true,
		},
		Memory: MemoryConfig{
			MaxEntries: 100,
			FTSEnabled: true,
		},
		Tool: ToolConfig{
			MaxToolChars:      8000,
			ToolTimeoutMs:     600000,
			TruncationEnabled: true,
		},
		Compact: CompactConfig{
			Threshold:    0.75,
			KeepRounds:   3,
			PruneEnabled: true,
			AutoContinue: true,
		},
		LSP: LSPConfig{
			Enabled: false,
			Timeout: 30,
		},
		Plugin: PluginConfig{
			Enabled: false,
		},
		Workflow: WorkflowConfig{
			Enabled: false,
		},
	}
}

// LoadConfig loads configuration from a file.
func LoadConfig(path string) (*UnifiedConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	config := NewUnifiedConfig()
	if err := json.Unmarshal(data, config); err != nil {
		return nil, err
	}

	return config, nil
}

// SaveConfig saves configuration to a file.
func SaveConfig(config *UnifiedConfig, path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

// MergeConfig merges two configurations, with override taking precedence.
func MergeConfig(base, override *UnifiedConfig) *UnifiedConfig {
	result := NewUnifiedConfig()

	// Agent
	if override.Agent.Model != "" {
		result.Agent.Model = override.Agent.Model
	} else {
		result.Agent.Model = base.Agent.Model
	}
	if override.Agent.MaxTurns > 0 {
		result.Agent.MaxTurns = override.Agent.MaxTurns
	} else {
		result.Agent.MaxTurns = base.Agent.MaxTurns
	}

	// Context
	if override.Context.MaxContextTokens > 0 {
		result.Context.MaxContextTokens = override.Context.MaxContextTokens
	} else {
		result.Context.MaxContextTokens = base.Context.MaxContextTokens
	}

	// Memory
	if override.Memory.GlobalPath != "" {
		result.Memory.GlobalPath = override.Memory.GlobalPath
	} else {
		result.Memory.GlobalPath = base.Memory.GlobalPath
	}

	// Tool
	if override.Tool.MaxToolChars > 0 {
		result.Tool.MaxToolChars = override.Tool.MaxToolChars
	} else {
		result.Tool.MaxToolChars = base.Tool.MaxToolChars
	}

	return result
}
