package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"miniclaudecode-go/mcp"
	"miniclaudecode-go/permissions"
	"miniclaudecode-go/skills"
	"miniclaudecode-go/tools"
)

// MicrolispSources embeds all microlisp source files (plaintext, no compression).
// Note: go:embed does NOT include *_test.go files — use MicrolispTestSources for those.
//
//go:embed microlisp/*.go
var MicrolispSources embed.FS

// MicrolispTestSources embeds all microlisp *_test.go test files.
// Go's embed excludes test files from regular embed patterns, so we use
// a separate directive with an explicit glob.
//
//go:embed microlisp/*_test.go
var MicrolispTestSources embed.FS

// LispTestData embeds all Common Lisp test files from microlisp/testdata/.
// These provide practical Lisp syntax examples for the lisp_guide tool.
//
//go:embed microlisp/testdata/*.lisp
var LispTestData embed.FS

// PermissionMode defines the permission checking strategy.
type PermissionMode string

const (
	ModeAsk    PermissionMode = "ask"
	ModeAuto   PermissionMode = "auto"
	ModePlan   PermissionMode = "plan"
	ModeBypass PermissionMode = "bypass"
)

// Config holds all runtime configuration.
type Config struct {
	Model                         string
	APIKey                        string
	BaseURL                       string
	MaxTurns                      int
	MaxContextMsgs                int
	PermissionMode                PermissionMode
	PrePlanMode                   PermissionMode // remembers the mode before entering plan mode (for ExitPlanMode to restore)
	AllowedCommands               []string
	DeniedPatterns                []string
	ProjectDir                    string
	MCPManager                    *mcp.Manager
	SkillLoader                   *skills.Loader
	SkillTracker                  *skills.SkillTracker
	FileHistory                   *SnapshotHistory
	AutoCompactEnabled            bool
	AutoCompactThreshold          float64
	AutoCompactBuffer             int
	MaxCompactOutputTokens        int
	MicroCompactEnabled           bool
	MicroCompactKeepRecent        int
	MicroCompactPlaceholder       string
	MicroCompactMinCharCount      int // minimum chars in a tool result to consider clearing; preserves small results
	MicroCompactGapMinutes        int // time gap (minutes) since last assistant message before triggering microcompact; 0 = disabled (run every turn)
	PostCompactRecoverFiles       bool
	PostCompactMaxFiles           int
	PostCompactMaxFileChars       int // legacy char-based budget (deprecated, use PostCompactMaxFileTokens)
	PostCompactMaxSkillChars      int // legacy char-based budget (deprecated, use PostCompactMaxSkillTokens)
	PostCompactMaxTotalSkillChars int // legacy char-based budget (deprecated, use PostCompactMaxTotalSkillTokens)
	// Token-based budgets for post-compact recovery (upstream uses tokens, not chars)
	PostCompactMaxFileTokens       int // default 50000 (matches upstream POST_COMPACT_TOKEN_BUDGET)
	PostCompactMaxTokensPerFile    int // default 5000 (matches upstream POST_COMPACT_MAX_TOKENS_PER_FILE)
	PostCompactMaxSkillTokens      int // default 1250 (~5K chars / 4)
	PostCompactMaxTotalSkillTokens int // default 6250 (~25K chars / 4)
	PostCompactHistorySnipCount    int
	SessionMemory                  *SessionMemory
	// Reactive compaction: trigger compaction when token count spikes above
	// this threshold between turns, even if not exceeding the max buffer.
	ReactiveCompactEnabled   bool
	ReactiveCompactThreshold int // default 5000 tokens
	// Partial compaction: directional compaction settings
	PartialCompactEnabled bool
	cachedPrompt          *CachedSystemPrompt
	// Sub-agent settings
	SubAgentMaxTurns int  // max turns for sub-agent loops (default 0 = no limit, matching Claude Code)
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
	// MaxOutputTokens controls the max_tokens parameter sent to the API.
	// Default: 16384 (main agent), 8000 (sub-agents matching Claude's CAPPED_DEFAULT_MAX_TOKENS).
	// When a response hits the max_tokens ceiling, the agent automatically
	// escalates to EscalatedMaxOutputTokens (64000) for the next request.
	MaxOutputTokens int
	// EscalatedMaxOutputTokens is the fallback max_tokens used when the
	// default cap is hit (matching Claude's ESCALATED_MAX_TOKENS = 64,000).
	EscalatedMaxOutputTokens int
	// RuleStore holds permission rules loaded from settings files.
	// May be nil if no settings files with permissions section are found.
	RuleStore *permissions.RuleStore
	// SubscriptionType identifies the subscriber tier for rate-limit gating.
	// Values: "claude_ai", "enterprise", "api", "unknown" (default).
	// "claude_ai" subscribers have hard usage limits; 429 retries are skipped.
	// "enterprise"/"api" subscribers get retried with exponential backoff.
	SubscriptionType string
	// QuerySource identifies the source of the query (e.g., "repl_main_thread", "sdk", "agent:task-xxx").
	// Used by RunPostCompactCleanup to guard main-thread-only state clears — subagents
	// share module-level state and must not clobber the main thread's caches.
	querySource string
	// HookManager holds registered compact hooks (pre/post compact).
	Hooks *HookManager
	// ThinkingBudgetTokens enables extended thinking with a budget (min 1024).
	// When > 0, the API request includes a thinking configuration.
	ThinkingBudgetTokens int
	// EffortLevel controls model selection for effort-based routing.
	// Values: "" (default), "fast" (use cheaper/faster model), "high" (use premium model with thinking)
	EffortLevel string
	// Runtime behavior settings from config file (takes priority over env vars)
	PreferNonStreaming      bool
	TelemetryDisabled       bool
	ExitAfterStopDelay      time.Duration
	SessionID               string
	DefaultOpusModel        string
	DefaultSonnetModel      string
	DefaultHaikuModel       string
	MaxContextTokens        int64
	MaxOutputTokensOverride int64
	GitBashPath             string
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
	Permissions permissions.PermissionsConfig `json:"permissions"`
	// Runtime behavior settings
	Runtime struct {
		EffortLevel        string `json:"effort_level"`
		PreferNonStreaming *bool  `json:"prefer_non_streaming"`
		ExitAfterStopDelay string `json:"exit_after_stop_delay"`
		TelemetryDisabled  *bool  `json:"telemetry_disabled"`
		SessionID          string `json:"session_id"`
	} `json:"runtime"`
	// Default model overrides (empty = use hard-coded defaults)
	Models struct {
		DefaultOpusModel   string `json:"default_opus_model"`
		DefaultSonnetModel string `json:"default_sonnet_model"`
		DefaultHaikuModel  string `json:"default_haiku_model"`
	} `json:"models"`
	// Token limits (0 = use model's natural limit)
	Tokens struct {
		MaxContextTokens int64 `json:"max_context_tokens"`
		MaxOutputTokens  int64 `json:"max_output_tokens"`
	} `json:"tokens"`
	// Tool-specific settings
	Tools struct {
		GitBashPath string `json:"git_bash_path"`
	} `json:"tools"`
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
			// Apply new runtime/model/token/tool settings
			cfg.applySettings(s)
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
				// Apply home settings for any still-empty runtime fields
				cfg.applySettingsFallback(s)
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

	// Load permission rules from settings files (project + home)
	cfg.RuleStore = permissions.LoadRulesFromAllSources(projectDir)

	// Start MCP servers if any
	if servers := mcpMgr.ListServers(); len(servers) > 0 {
		if err := mcpMgr.StartAll(context.Background()); err != nil {
			fmt.Fprintf(os.Stderr, "MCP start error: %v\n", err)
		}
	}
	cfg.MCPManager = mcpMgr
	cfg.Hooks = NewHookManager()

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

	// Env var fallback (lowest priority, below config file, above defaults)
	cfg.applyEnvFallback()

	// Return found if any config was loaded
	if cfg.APIKey != "" || cfg.Model != "" || len(mcpMgr.ListServers()) > 0 {
		// Resolve model alias (e.g. "sonnet" → "claude-sonnet-4-20250514")
		if cfg.Model != "" {
			if resolved, ok := ResolveModelAlias(cfg.Model); ok {
				cfg.Model = resolved
			}
		}
		return cfg, true
	}
	return cfg, false
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		Model:          "",
		MaxTurns:       0, // unlimited
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
		AutoCompactEnabled:            true,
		AutoCompactThreshold:          0.75,
		AutoCompactBuffer:             13000,
		MaxCompactOutputTokens:        8192,
		MicroCompactEnabled:           true,
		MicroCompactKeepRecent:        5,
		MicroCompactPlaceholder:       "[Old tool result content cleared]",
		MicroCompactMinCharCount:      2000, // only clear results >= 2000 chars; preserve small useful results
		MicroCompactGapMinutes:        60,   // trigger microcompact when gap > 60 min since last assistant; matches server cache TTL; 0 = disabled
		PostCompactRecoverFiles:       true,
		PostCompactMaxFiles:           5,
		PostCompactMaxFileChars:       50000, // legacy, use PostCompactMaxFileTokens
		PostCompactMaxSkillChars:      5000,  // legacy, use PostCompactMaxSkillTokens
		PostCompactMaxTotalSkillChars: 25000, // legacy, use PostCompactMaxTotalSkillTokens
		// Token-based budgets (upstream-compatible)
		PostCompactMaxFileTokens:       50000, // matches upstream POST_COMPACT_TOKEN_BUDGET
		PostCompactMaxTokensPerFile:    5000,  // matches upstream POST_COMPACT_MAX_TOKENS_PER_FILE
		PostCompactMaxSkillTokens:      1250,  // ~5K chars at 4 chars/token
		PostCompactMaxTotalSkillTokens: 6250,  // ~25K chars at 4 chars/token
		PostCompactHistorySnipCount:    3,
		ReactiveCompactEnabled:         true,
		ReactiveCompactThreshold:       5000,
		PartialCompactEnabled:          true,
		cachedPrompt:                   NewCachedSystemPrompt(),
		SubAgentMaxTurns:               0,
		SubAgentEnabled:                true,
		AutoClassifierEnabled:          true,
		AutoClassifierMaxTokens:        128,
		AutoDenialLimit:                3,
		MaxOutputTokens:                16384,
		EscalatedMaxOutputTokens:       64000,
		Hooks:                          NewHookManager(),
		// Runtime defaults (convention over configuration)
		// Model defaults left empty here — hard-coded defaults live in model_aliases.go
		// package-level vars. Only non-empty when explicitly set in settings.json.
		DefaultOpusModel:   "",
		DefaultSonnetModel: "",
		DefaultHaikuModel:  "",
	}
}

// applySettings transfers settings from a ClaudeSettings struct to Config.
// Used for project-level config — always overwrites.
func (cfg *Config) applySettings(s ClaudeSettings) {
	if s.Runtime.EffortLevel != "" {
		cfg.EffortLevel = s.Runtime.EffortLevel
	}
	if s.Runtime.PreferNonStreaming != nil {
		cfg.PreferNonStreaming = *s.Runtime.PreferNonStreaming
	}
	if s.Runtime.ExitAfterStopDelay != "" {
		if d, err := time.ParseDuration(s.Runtime.ExitAfterStopDelay); err == nil {
			cfg.ExitAfterStopDelay = d
		}
	}
	if s.Runtime.TelemetryDisabled != nil {
		cfg.TelemetryDisabled = *s.Runtime.TelemetryDisabled
	}
	if s.Runtime.SessionID != "" {
		cfg.SessionID = s.Runtime.SessionID
	}
	if s.Models.DefaultOpusModel != "" {
		cfg.DefaultOpusModel = s.Models.DefaultOpusModel
	}
	if s.Models.DefaultSonnetModel != "" {
		cfg.DefaultSonnetModel = s.Models.DefaultSonnetModel
	}
	if s.Models.DefaultHaikuModel != "" {
		cfg.DefaultHaikuModel = s.Models.DefaultHaikuModel
	}
	if s.Tokens.MaxContextTokens > 0 {
		cfg.MaxContextTokens = s.Tokens.MaxContextTokens
	}
	if s.Tokens.MaxOutputTokens > 0 {
		cfg.MaxOutputTokensOverride = s.Tokens.MaxOutputTokens
	}
	if s.Tools.GitBashPath != "" {
		cfg.GitBashPath = s.Tools.GitBashPath
	}
}

// applySettingsFallback transfers settings from a ClaudeSettings struct to Config
// only if the corresponding field is still empty. Used for home-level config fallback.
func (cfg *Config) applySettingsFallback(s ClaudeSettings) {
	if cfg.EffortLevel == "" && s.Runtime.EffortLevel != "" {
		cfg.EffortLevel = s.Runtime.EffortLevel
	}
	if cfg.SessionID == "" && s.Runtime.SessionID != "" {
		cfg.SessionID = s.Runtime.SessionID
	}
	if cfg.DefaultOpusModel == "" && s.Models.DefaultOpusModel != "" {
		cfg.DefaultOpusModel = s.Models.DefaultOpusModel
	}
	if cfg.DefaultSonnetModel == "" && s.Models.DefaultSonnetModel != "" {
		cfg.DefaultSonnetModel = s.Models.DefaultSonnetModel
	}
	if cfg.DefaultHaikuModel == "" && s.Models.DefaultHaikuModel != "" {
		cfg.DefaultHaikuModel = s.Models.DefaultHaikuModel
	}
	if cfg.GitBashPath == "" && s.Tools.GitBashPath != "" {
		cfg.GitBashPath = s.Tools.GitBashPath
	}
	if cfg.MaxContextTokens == 0 && s.Tokens.MaxContextTokens > 0 {
		cfg.MaxContextTokens = s.Tokens.MaxContextTokens
	}
	if cfg.MaxOutputTokensOverride == 0 && s.Tokens.MaxOutputTokens > 0 {
		cfg.MaxOutputTokensOverride = s.Tokens.MaxOutputTokens
	}
}

// applyEnvFallback reads CLAUDE_CODE_* env vars as a last-resort fallback
// (below config file settings, above hard-coded defaults).
func (cfg *Config) applyEnvFallback() {
	// Runtime behavior (category 3)
	if cfg.EffortLevel == "" {
		if v := os.Getenv("CLAUDE_CODE_EFFORT_LEVEL"); v == "fast" {
			cfg.EffortLevel = "fast"
		}
	}
	if !cfg.PreferNonStreaming {
		if v := os.Getenv("CLAUDE_CODE_PREFER_NON_STREAMING"); v != "" && v != "0" && v != "false" {
			cfg.PreferNonStreaming = true
		}
	}
	if cfg.ExitAfterStopDelay == 0 {
		if v := os.Getenv("CLAUDE_CODE_EXIT_AFTER_STOP_DELAY"); v != "" {
			if d, err := time.ParseDuration(v); err == nil {
				cfg.ExitAfterStopDelay = d
			} else {
				if ms, err := strconv.ParseInt(v, 10, 64); err == nil && ms > 0 {
					cfg.ExitAfterStopDelay = time.Duration(ms) * time.Millisecond
				}
			}
		}
	}
	if !cfg.TelemetryDisabled {
		if v := os.Getenv("CLAUDE_CODE_TELEMETRY_DISABLED"); v == "1" || v == "true" {
			cfg.TelemetryDisabled = true
		}
	}
	if cfg.SessionID == "" {
		if sid := os.Getenv("CLAUDE_SESSION_ID"); sid != "" {
			cfg.SessionID = sid
		}
	}
	// Model defaults (category 4)
	if cfg.DefaultOpusModel == "" {
		if m := os.Getenv("CLAUDE_DEFAULT_OPUS_MODEL"); m != "" {
			cfg.DefaultOpusModel = m
		}
	}
	if cfg.DefaultSonnetModel == "" {
		if m := os.Getenv("CLAUDE_DEFAULT_SONNET_MODEL"); m != "" {
			cfg.DefaultSonnetModel = m
		}
	}
	if cfg.DefaultHaikuModel == "" {
		if m := os.Getenv("CLAUDE_DEFAULT_HAIKU_MODEL"); m != "" {
			cfg.DefaultHaikuModel = m
		}
	}
	// Token limits (category 4)
	if cfg.MaxContextTokens == 0 {
		if v := os.Getenv("CLAUDE_CODE_MAX_CONTEXT_TOKENS"); v != "" {
			if val, err := strconv.ParseInt(v, 10, 64); err == nil && val > 0 {
				cfg.MaxContextTokens = val
			}
		}
	}
	if cfg.MaxOutputTokensOverride == 0 {
		if v := os.Getenv("CLAUDE_CODE_MAX_OUTPUT_TOKENS"); v != "" {
			if val, err := strconv.ParseInt(v, 10, 64); err == nil && val > 0 {
				cfg.MaxOutputTokensOverride = val
			}
		}
	}
	// Tool-specific (category 5)
	if cfg.GitBashPath == "" {
		if p := os.Getenv("CLAUDE_CODE_GIT_BASH_PATH"); p != "" {
			cfg.GitBashPath = p
		}
	}
}

// DefaultRegistry creates and populates a registry with all built-in tools.
func DefaultRegistry() *tools.Registry {
	r := tools.NewRegistry()
	r.Register(&tools.ExecTool{})
	r.Register(tools.NewFileReadTool(r))
	r.Register(tools.NewFileWriteTool(r))
	r.Register(tools.NewFileEditTool(r))
	r.Register(&tools.GlobTool{})
	r.Register(&tools.GrepTool{})
	r.Register(tools.NewMultiEditTool(r))
	r.Register(&tools.ListDirTool{})
	r.Register(&tools.ExaSearchTool{})
	r.Register(&tools.WebSearchTool{})
	r.Register(&tools.WebFetchTool{})
	r.Register(&tools.RuntimeInfoTool{})
	r.Register(&tools.ProcessTool{})
	r.Register(&tools.FileOpsTool{})
	r.Register(&tools.GitTool{})
	r.Register(tools.NewNotebookEditTool(r))
	r.Register(&tools.SystemTool{})
	r.Register(&tools.TerminalTool{})
	r.Register(&tools.LispEvalTool{})
	r.Register(&tools.LispExecTool{})
	r.Register(&tools.LispToolsTool{})
	r.Register(tools.NewFileEncodingTool(r))
	// ToolSearchTool's Registry field is nil here; it is set by the agent loop
	// after the registry is fully populated.
	r.Register(&tools.BriefTool{})
	r.Register(&tools.ToolSearchTool{})
	RegisterCronTools(r)
	return r
}

// RegisterCronTools adds cron management tools to the registry.
func RegisterCronTools(r *tools.Registry) {
	r.Register(&CronCreateTool{})
	r.Register(&CronDeleteTool{})
	r.Register(&CronListTool{})
}

// RegisterMCPAndSkills adds MCP and skills tools to the registry using the loaded config.
func RegisterMCPAndSkills(r *tools.Registry, cfg *Config) {
	r.Register(&tools.ListMCPTools{Manager: cfg.MCPManager})
	r.Register(&tools.MCPToolCaller{Manager: cfg.MCPManager})
	r.Register(&tools.MCPServerStatus{Manager: cfg.MCPManager})
	// Create a sub-FS rooted at microlisp/ so .go files can be read by basename.
	microlispFS, _ := fs.Sub(MicrolispSources, "microlisp")
	// Create a sub-FS for *_test.go files.
	testSourceFS, _ := fs.Sub(MicrolispTestSources, "microlisp")
	// Create a sub-FS rooted at microlisp/testdata/ so .lisp files can be read by basename.
	testdataFS, _ := fs.Sub(LispTestData, "microlisp/testdata")
	r.Register(&tools.LispGuideTool{SourceFS: microlispFS, TestSourceFS: testSourceFS, TestDataFS: testdataFS})
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
