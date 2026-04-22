package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"miniclaudecode-go/mcp"
	"miniclaudecode-go/skills"
	"miniclaudecode-go/tools"
)

// PermissionMode defines the permission checking strategy.
type PermissionMode string

const (
	ModeAsk  PermissionMode = "ask"
	ModeAuto PermissionMode = "auto"
	ModePlan PermissionMode = "plan"
)

// Config holds all runtime configuration.
type Config struct {
	Model           string
	APIKey          string
	BaseURL         string
	MaxTurns        int
	MaxContextMsgs  int
	PermissionMode  PermissionMode
	AllowedCommands []string
	DeniedPatterns  []string
	ProjectDir      string
	MCPManager      *mcp.Manager
	SkillLoader     *skills.Loader
}

// MCPServerConfig holds the configuration for a single MCP server.
type MCPServerConfig struct {
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env"`
}

// ClaudeSettings represents the settings.json format used by Claude CLI.
type ClaudeSettings struct {
	Env struct {
		AnthropicAuthToken string `json:"ANTHROPIC_AUTH_TOKEN"`
		AnthropicBaseURL   string `json:"ANTHROPIC_BASE_URL"`
		AnthropicModel     string `json:"ANTHROPIC_MODEL"`
		AnthropicSonnet    string `json:"ANTHROPIC_DEFAULT_SONNET_MODEL"`
		AnthropicOpus      string `json:"ANTHROPIC_DEFAULT_OPUS_MODEL"`
		AnthropicHaiku     string `json:"ANTHROPIC_DEFAULT_HAIKU_MODEL"`
		AnthropicReasoning string `json:"ANTHROPIC_REASONING_MODEL"`
	} `json:"env"`
	MCP struct {
		Servers map[string]MCPServerConfig `json:"servers"`
	} `json:"mcp"`
}

// LoadConfigFromFile loads config from .claude/settings.json in the project root.
func LoadConfigFromFile(projectDir string) (cfg Config, found bool) {
	settingsPath := filepath.Join(projectDir, ".claude", "settings.json")
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		return Config{}, false
	}

	var s ClaudeSettings
	if err := json.Unmarshal(data, &s); err != nil {
		return Config{}, false
	}

	cfg = Config{
		APIKey:     s.Env.AnthropicAuthToken,
		BaseURL:    s.Env.AnthropicBaseURL,
		Model:      s.Env.AnthropicModel,
		ProjectDir: projectDir,
	}
	if cfg.Model == "" {
		cfg.Model = s.Env.AnthropicSonnet
	}
	if cfg.Model == "" {
		cfg.Model = s.Env.AnthropicOpus
	}

	// Initialize MCP manager
	mcpMgr := mcp.NewManager()
	for name, srv := range s.MCP.Servers {
		mcpMgr.Register(name, srv.Command, srv.Args, srv.Env)
	}
	if len(s.MCP.Servers) > 0 {
		if err := mcpMgr.StartAll(context.Background()); err != nil {
			// Log error but don't fail - MCP is optional
			fmt.Fprintf(os.Stderr, "MCP start error: %v\n", err)
		}
	}
	cfg.MCPManager = mcpMgr

	// Initialize skill loader - check binary directory first, then workspace
	loader := skills.NewLoader(projectDir)
	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		builtinSkills := filepath.Join(exeDir, "skills")
		if _, err := os.Stat(builtinSkills); err == nil {
			loader.SetBuiltinDir(builtinSkills)
		}
	}
	_ = loader.Refresh()
	cfg.SkillLoader = loader

	return cfg, cfg.APIKey != ""
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		Model:          "claude-sonnet-4-20250514",
		MaxTurns:       30,
		MaxContextMsgs: 100,
		PermissionMode: ModeAsk,
		AllowedCommands: []string{
			"ls", "cat", "head", "tail", "wc", "find", "grep", "rg",
			"git status", "git diff", "git log", "git branch",
			"python", "python3", "pip", "npm", "node",
			"echo", "pwd", "which", "env", "date",
		},
		DeniedPatterns: []string{
			"rm -rf /", "rm -rf ~", "sudo rm",
			"git push --force", "git reset --hard",
			"> /dev/sda", "mkfs", "dd if=",
		},
	}
}

// DefaultRegistry creates and populates a registry with all built-in tools.
func DefaultRegistry() *tools.Registry {
	r := tools.NewRegistry()
	r.Register(&tools.ExecTool{})
	r.Register(&tools.FileReadTool{})
	r.Register(&tools.FileWriteTool{})
	r.Register(&tools.FileEditTool{})
	r.Register(&tools.GlobTool{})
	r.Register(&tools.GrepTool{})
	r.Register(&tools.MultiEditTool{})
	r.Register(&tools.ListDirTool{})
	r.Register(&tools.WebSearchTool{})
	r.Register(&tools.WebFetchTool{})
	r.Register(&tools.RuntimeInfoTool{})
	r.Register(&tools.ImageReadTool{})
	r.Register(&tools.ProcessTool{})
	r.Register(&tools.FileOpsTool{})
	return r
}

// RegisterMCPAndSkills adds MCP and skills tools to the registry using the loaded config.
func RegisterMCPAndSkills(r *tools.Registry, cfg *Config) {
	r.Register(&tools.ListMCPTools{Manager: cfg.MCPManager})
	r.Register(&tools.MCPToolCaller{Manager: cfg.MCPManager})
	r.Register(&tools.MCPServerStatus{Manager: cfg.MCPManager})
	r.Register(&tools.ReadSkillTool{Loader: cfg.SkillLoader})
	r.Register(&tools.ListSkillsTool{Loader: cfg.SkillLoader})
}

// Close cleans up resources held by the config (MCP servers, etc).
func (cfg *Config) Close() {
	if cfg.MCPManager != nil {
		cfg.MCPManager.StopAll()
	}
}
