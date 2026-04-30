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
	Model                  string
	APIKey                 string
	BaseURL                string
	MaxTurns               int
	MaxContextMsgs         int
	PermissionMode         PermissionMode
	AllowedCommands        []string
	DeniedPatterns         []string
	ProjectDir             string
	MCPManager             *mcp.Manager
	SkillLoader            *skills.Loader
	SkillTracker           *skills.SkillTracker
	FileHistory            *SnapshotHistory
	AutoCompactEnabled     bool
	AutoCompactThreshold   float64
	AutoCompactBuffer      int
	MaxCompactOutputTokens int
	cachedPrompt           *CachedSystemPrompt
}

// MCPServerConfig holds the configuration for a single MCP server.
type MCPServerConfig struct {
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env"`
	URL     string            `json:"url"`
}

// MCPConfigFile represents the .mcp.json format used by Claude Code.
type MCPConfigFile struct {
	MCPServers map[string]MCPConfigEntry `json:"mcpServers"`
}

type MCPConfigEntry struct {
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env"`
	URL     string            `json:"url"`
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

// LoadConfigFromFile loads config from .claude/settings.json and .mcp.json in the project root.
func LoadConfigFromFile(projectDir string) (cfg Config, found bool) {
	// Initialize MCP manager first
	mcpMgr := mcp.NewManager()

	// Load from settings.json
	settingsPath := filepath.Join(projectDir, ".claude", "settings.json")
	if data, err := os.ReadFile(settingsPath); err == nil {
		var s ClaudeSettings
		if err := json.Unmarshal(data, &s); err == nil {
			cfg = Config{
				APIKey:     s.Env.AnthropicAuthToken,
				BaseURL:    s.Env.AnthropicBaseURL,
				Model:      s.Env.AnthropicModel,
				ProjectDir: projectDir,
			}
			// Legacy: also load MCP servers from settings.json
			for name, srv := range s.MCP.Servers {
				mcpMgr.Register(name, srv.Command, srv.Args, srv.Env)
			}
		} else {
			fmt.Fprintf(os.Stderr, "[WARN] Failed to parse settings.json: %v\n", err)
		}
	}

	// Load MCP config from .mcp.json (Claude Code compatible format)
	mcpPath := filepath.Join(projectDir, ".mcp.json")
	if mcpData, err := os.ReadFile(mcpPath); err == nil {
		var mcpCfg MCPConfigFile
		if err := json.Unmarshal(mcpData, &mcpCfg); err == nil {
			for name, entry := range mcpCfg.MCPServers {
				if entry.URL != "" {
					mcpMgr.RegisterRemote(name, entry.URL, entry.Env)
				} else if entry.Command != "" {
					args := entry.Args
					if args == nil {
						args = []string{}
					}
					mcpMgr.Register(name, entry.Command, args, entry.Env)
				}
			}
		} else {
			fmt.Fprintf(os.Stderr, "[WARN] Failed to parse .mcp.json: %v\n", err)
		}
	}

	// Start MCP servers if any
	if servers := mcpMgr.ListServers(); len(servers) > 0 {
		if err := mcpMgr.StartAll(context.Background()); err != nil {
			fmt.Fprintf(os.Stderr, "MCP start error: %v\n", err)
		}
	}
	cfg.MCPManager = mcpMgr

	// Initialize skill loader -- check binary directory first, then workspace
	loader := skills.NewLoader(projectDir)
	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		builtinSkills := filepath.Join(exeDir, "skills")
		if _, err := os.Stat(builtinSkills); err == nil {
			loader.SetBuiltinDir(builtinSkills)
		}
	}
	if err := loader.Refresh(); err != nil {
		fmt.Fprintf(os.Stderr, "[WARN] Failed to refresh skills: %v\n", err)
	}
	cfg.SkillLoader = loader
	cfg.SkillTracker = skills.NewSkillTracker()
	cfg.cachedPrompt = NewCachedSystemPrompt()

	// Return found if any config was loaded
	if cfg.APIKey != "" || cfg.Model != "" || len(mcpMgr.ListServers()) > 0 {
		return cfg, true
	}
	return cfg, false
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		Model:          "",
		MaxTurns:       90,
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
		cachedPrompt: NewCachedSystemPrompt(),
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
	r.Register(&tools.ExaSearchTool{})
	r.Register(&tools.WebSearchTool{})
	r.Register(&tools.WebFetchTool{})
	r.Register(&tools.RuntimeInfoTool{})
	r.Register(&tools.ProcessTool{})
	r.Register(&tools.FileOpsTool{})
	r.Register(&tools.GitTool{})
	r.Register(&tools.SystemTool{})
	r.Register(&tools.TerminalTool{})
	return r
}

// RegisterMCPAndSkills adds MCP and skills tools to the registry using the loaded config.
func RegisterMCPAndSkills(r *tools.Registry, cfg *Config) {
	r.Register(&tools.ListMCPTools{Manager: cfg.MCPManager})
	r.Register(&tools.MCPToolCaller{Manager: cfg.MCPManager})
	r.Register(&tools.MCPServerStatus{Manager: cfg.MCPManager})
	r.Register(&tools.ReadSkillTool{Loader: cfg.SkillLoader})
	r.Register(&tools.ListSkillsTool{Loader: cfg.SkillLoader})
	r.Register(&tools.SearchSkillsTool{Loader: cfg.SkillLoader})
}

// Close cleans up resources held by the config (MCP servers, etc).
func (cfg *Config) Close() {
	if cfg.MCPManager != nil {
		cfg.MCPManager.StopAll()
	}
}
