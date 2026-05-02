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
	MicroCompactEnabled    bool
	MicroCompactKeepRecent int
	MicroCompactPlaceholder string
	PostCompactRecoverFiles    bool
	PostCompactMaxFiles        int
	PostCompactMaxFileChars    int
	PostCompactMaxSkillChars   int
	PostCompactMaxTotalSkillChars int
	PostCompactHistorySnipCount   int
	SessionMemory           *SessionMemory
	// Reactive compaction: trigger compaction when token count spikes above
	// this threshold between turns, even if not exceeding the max buffer.
	ReactiveCompactEnabled    bool
	ReactiveCompactThreshold  int // default 5000 tokens
	// Partial compaction: directional compaction settings
	PartialCompactEnabled bool
	cachedPrompt           *CachedSystemPrompt
	// Sub-agent settings
	SubAgentMaxTurns int  // max turns for sub-agent loops (default 200, matching Claude fork agent)
	SubAgentEnabled  bool // enable/disable the agent tool (default true)
	// Auto mode classifier settings
	AutoClassifierEnabled   bool   // enable LLM classifier in auto mode (default true)
	AutoClassifierModel     string // model for classifier (default: same as main model)
	AutoClassifierMaxTokens int    // max tokens for classifier response (default 128)
	AutoDenialLimit         int    // consecutive denials before fallback (default 3)
	// ShouldAvoidPermissionPrompts, when true, causes any tool that would
	// normally trigger an interactive permission prompt to be auto-denied
	// instead. Sub-agents set this to true so they never block on user input.
	ShouldAvoidPermissionPrompts bool
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
	} `json:"env"`
	MCP struct {
		Servers map[string]MCPServerConfig `json:"servers"`
	} `json:"mcp"`
}

// homeClaudeDir returns the path to ~/.claude, or empty string if undetermined.
func homeClaudeDir() string {
	// Windows: USERPROFILE
	if p := os.Getenv("USERPROFILE"); p != "" {
		return filepath.Join(p, ".claude")
	}
	// Unix: HOME
	if p := os.Getenv("HOME"); p != "" {
		return filepath.Join(p, ".claude")
	}
	return ""
}

// LoadConfigFromFile loads config from .claude/settings.json and .mcp.json in the project root.
// Falls back to ~/.claude/settings.json and ~/.mcp.json when project-level config is missing.
func LoadConfigFromFile(projectDir string) (cfg Config, found bool) {
	// Initialize MCP manager first
	mcpMgr := mcp.NewManager()
	cfg.ProjectDir = projectDir

	// Load from project-level settings.json
	settingsPath := filepath.Join(projectDir, ".claude", "settings.json")
	if data, err := os.ReadFile(settingsPath); err == nil {
		var s ClaudeSettings
		if err := json.Unmarshal(data, &s); err == nil {
			cfg.APIKey = s.Env.AnthropicAuthToken
			cfg.BaseURL = s.Env.AnthropicBaseURL
			cfg.Model = s.Env.AnthropicModel
			// Legacy: also load MCP servers from settings.json
			for name, srv := range s.MCP.Servers {
				mcpMgr.Register(name, srv.Command, srv.Args, srv.Env)
			}
		} else {
			fmt.Fprintf(os.Stderr, "[WARN] Failed to parse settings.json: %v\n", err)
		}
	}

	// Fallback: load from home directory ~/.claude/settings.json
	// Fill in values that are still empty after project-level loading
	if homeDir := homeClaudeDir(); homeDir != "" {
		homeSettingsPath := filepath.Join(homeDir, "settings.json")
		if data, err := os.ReadFile(homeSettingsPath); err == nil {
			var s ClaudeSettings
			if err := json.Unmarshal(data, &s); err == nil {
				if cfg.APIKey == "" {
					cfg.APIKey = s.Env.AnthropicAuthToken
				}
				if cfg.BaseURL == "" {
					cfg.BaseURL = s.Env.AnthropicBaseURL
				}
				if cfg.Model == "" {
					cfg.Model = s.Env.AnthropicModel
				}
				// Load MCP servers from home settings only if none loaded yet
				if len(mcpMgr.ListServers()) == 0 {
					for name, srv := range s.MCP.Servers {
						mcpMgr.Register(name, srv.Command, srv.Args, srv.Env)
					}
				}
			} else {
				fmt.Fprintf(os.Stderr, "[WARN] Failed to parse home settings.json: %v\n", err)
			}
		}
	}

	// Load MCP config from project-level .mcp.json (Claude Code compatible format)
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

	// Fallback: load MCP config from home directory ~/.claude/.mcp.json
	// Only if no MCP servers were loaded from project-level config
	if len(mcpMgr.ListServers()) == 0 {
		if homeDir := homeClaudeDir(); homeDir != "" {
			homeMCPPath := filepath.Join(homeDir, ".mcp.json")
			if mcpData, err := os.ReadFile(homeMCPPath); err == nil {
				var mcpCfg MCPConfigFile
				if err := json.Unmarshal(mcpData, &mcpCfg); err == nil {
					if len(mcpMgr.ListServers()) == 0 {
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
					}
				} else {
					fmt.Fprintf(os.Stderr, "[WARN] Failed to parse home .mcp.json: %v\n", err)
				}
			}
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
		AutoCompactEnabled:     true,
		AutoCompactThreshold:   0.75,
		AutoCompactBuffer:      13000,
		MaxCompactOutputTokens: 8192,
		MicroCompactEnabled:    true,
		MicroCompactKeepRecent: 5,
		MicroCompactPlaceholder: "[Old tool result content cleared]",
		PostCompactRecoverFiles:       true,
		PostCompactMaxFiles:           5,
		PostCompactMaxFileChars:       50000,
		PostCompactMaxSkillChars:      5000,
		PostCompactMaxTotalSkillChars: 25000,
		PostCompactHistorySnipCount:   3,
		ReactiveCompactEnabled:    true,
		ReactiveCompactThreshold:  5000,
		PartialCompactEnabled:     true,
		cachedPrompt: NewCachedSystemPrompt(),
		SubAgentMaxTurns:          200,
		SubAgentEnabled:           true,
		AutoClassifierEnabled:   true,
		AutoClassifierMaxTokens: 128,
		AutoDenialLimit:         3,
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
	// ToolSearchTool's Registry field is nil here; it is set by the agent loop
	// after the registry is fully populated.
	r.Register(&tools.BriefTool{})
	r.Register(&tools.ToolSearchTool{})
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

// RegisterMemoryTools adds memory tools to the registry using the SessionMemory instance.
func RegisterMemoryTools(r *tools.Registry, sm *SessionMemory) {
	if sm == nil {
		return
	}
	r.Register(&tools.MemoryAddTool{
		OnAdd: func(category, content, source string) {
			sm.AddNote(category, content, source)
		},
	})
	r.Register(&tools.MemorySearchTool{
		OnSearch: func(query string) []tools.MemorySearchResult {
			notes := sm.SearchNotes(query)
			results := make([]tools.MemorySearchResult, len(notes))
			for i, n := range notes {
				results[i] = tools.MemorySearchResult{Category: n.Category, Content: n.Content}
			}
			return results
		},
	})
}

// Close cleans up resources held by the config (MCP servers, etc).
func (cfg *Config) Close() {
	if cfg.MCPManager != nil {
		cfg.MCPManager.StopAll()
	}
	if cfg.SessionMemory != nil {
		cfg.SessionMemory.Stop()
	}
}
