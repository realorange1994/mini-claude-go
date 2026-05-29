// Package agent provides the SDK factory for creating agent sessions.
// Aligned to pi's sdk.ts createAgentSession().
package agent

import (
	"fmt"
	"os"
	"strings"

	"miniclaudecode-go/pkg/core/auth"
	"miniclaudecode-go/pkg/core/config"
	"miniclaudecode-go/pkg/core/modelregistry"
	"miniclaudecode-go/pkg/core/resourceloader"
	"miniclaudecode-go/pkg/core/session"
)

// CreateSessionOptions configures how a session is created.
// Aligned to pi's CreateAgentSessionOptions.
type CreateSessionOptions struct {
	// Working directory. Default: os.Getwd()
	Cwd string
	// Global config dir. Default: ~/.miniclaude/
	AgentDir string

	// Auth storage for credentials. Default: auth.Create(agentDir/auth.json)
	AuthStorage *auth.AuthStorage
	// Model registry. Default: modelregistry.NewRegistry() + LoadFromJSON
	ModelRegistry *modelregistry.Registry

	// Model ID to use. Can be a short alias like "sonnet", "opus", "haiku".
	// Default: from settings, then "claude-sonnet-4-20250514"
	Model string
	// Thinking level. Default: from settings, then "medium" (clamped to model capabilities)
	ThinkingLevel ThinkingLevel
	// Max turns per message loop. 0 = unlimited (default 100)
	MaxTurns int
	// Max tokens per LLM response. Default: 8192
	MaxTokens int
	// Enable streaming output
	Streaming bool

	// Tool names to enable. Default: built-in tools
	Tools []string
	// Suppress default built-in tools
	NoTools bool
	// Initial user message (one-shot mode)
	Message string

	// API key override (skips settings/env resolution)
	APIKey string
	// Base URL override (skips settings/env resolution)
	BaseURL string

	// Resource loader. Default: resourceloader.New(cwd, agentDir)
	ResourceLoader *resourceloader.ResourceLoader

	// Continue previous session (restore messages + model)
	ContinueSession bool
}

// CreateSessionResult is returned by CreateSession.
type CreateSessionResult struct {
	Session *AgentSession
	Model   modelregistry.ModelInfo
	// Warning if model was resolved from a fallback
	ModelFallback string
	// Non-fatal warnings encountered during session creation
	Warnings []string
	// Auth storage used by this session
	AuthStorage *auth.AuthStorage
}

// CreateSession creates a fully configured agent session.
// It resolves the model, API key, settings, and tools from the
// provided options, falling back to env vars and defaults.
// Aligned to pi's createAgentSession().
func CreateSession(opts CreateSessionOptions) (*CreateSessionResult, error) {
	// Resolve working directory
	cwd := opts.Cwd
	if cwd == "" {
		cwd, _ = os.Getwd()
	}

	// Resolve agent dir
	agentDir := opts.AgentDir
	if agentDir == "" {
		home, _ := os.UserHomeDir()
		agentDir = fmt.Sprintf("%s/.miniclaude", home)
	}

	// Load settings
	var warnings []string
	settingsMgr := config.NewSettingsManager(agentDir, fmt.Sprintf("%s/.miniclaude", cwd))
	if err := settingsMgr.Load(); err != nil {
		w := fmt.Sprintf("settings load error: %v", err)
		warnings = append(warnings, w)
		fmt.Fprintf(os.Stderr, "[!] %s\n", w)
	}
	merged := settingsMgr.Merged()

	// Create auth storage
	authStorage := opts.AuthStorage
	if authStorage == nil {
		authPath := fmt.Sprintf("%s/auth.json", agentDir)
		authStorage = auth.Create(authPath)
	}

	// Create model registry
	modelReg := opts.ModelRegistry
	if modelReg == nil {
		modelReg = modelregistry.NewRegistry()
		modelsPath := fmt.Sprintf("%s/models.json", agentDir)
		if err := modelReg.LoadFromJSON(modelsPath); err != nil {
			fmt.Fprintf(os.Stderr, "[!] models.json load error: %v\n", err)
		}
	}

	// Create resource loader
	rl := opts.ResourceLoader
	if rl == nil {
		rl = resourceloader.New(cwd, agentDir)
	}

	// Resolve model
	modelID := opts.Model
	if modelID == "" {
		modelID = merged.Model
	}
	if modelID == "" {
		modelID = "claude-sonnet-4-20250514"
	}

	modelInfo, err := modelReg.Resolve(modelID)
	if err != nil {
		return nil, fmt.Errorf("resolve model %q: %w", modelID, err)
	}

	// Resolve API key
	apiKey := opts.APIKey
	if apiKey == "" {
		apiKey = merged.APIKey
	}
	if apiKey == "" {
		// Try auth storage
		providerKey := modelInfo.Provider
		if providerKey == "" {
			providerKey = "anthropic"
		}
		if cred, ok := authStorage.GetCredential(providerKey); ok && cred.Key != "" {
			apiKey = cred.Key
		}
	}
	if apiKey == "" {
		apiKey, err = modelReg.ResolveAPIKey(modelInfo)
		if err != nil {
			// Provide helpful auth guidance
			guidance := auth.GetAuthGuidance(modelInfo.Provider)
			return nil, fmt.Errorf("resolve API key: %w\n%s", err, auth.FormatAuthHelp(guidance))
		}
	}

	// Resolve base URL
	baseURL := opts.BaseURL
	if baseURL == "" {
		baseURL = merged.BaseURL
	}
	if baseURL == "" {
		baseURL = modelInfo.BaseURL
	}
	// Ensure URL ends with /v1/messages for Anthropic-compatible APIs
	if baseURL != "" && !strings.HasSuffix(baseURL, "/v1/messages") {
		baseURL = strings.TrimSuffix(baseURL, "/") + "/v1/messages"
	}

	// Resolve max tokens
	maxTokens := opts.MaxTokens
	if maxTokens == 0 {
		maxTokens = merged.MaxTokens
	}
	if maxTokens == 0 {
		maxTokens = 8192
	}

	// Resolve max turns
	maxTurns := opts.MaxTurns
	if maxTurns == 0 {
		maxTurns = merged.MaxTurns
	}
	if maxTurns == 0 {
		maxTurns = 100
	}

	// Resolve thinking level
	thinkingLevel := opts.ThinkingLevel
	if thinkingLevel == "" {
		thinkingLevel = ThinkingLevelMedium
	}
	// Clamp to model capabilities
	thinkingLevel = ClampThinkingLevel(thinkingLevel, modelInfo.Reasoning)

	// Session path
	sessionPath := fmt.Sprintf("%s/sessions", agentDir)

	// Resolve tool names
	var toolNames []string
	if opts.Tools != nil {
		toolNames = opts.Tools
	} else if opts.NoTools {
		toolNames = []string{}
	} else {
		toolNames = []string{"Read", "Bash", "Edit", "Write"}
	}

	// Build agent config
	agentConfig := AgentConfig{
		Model:          modelInfo.ID,
		Cwd:            cwd,
		MaxTurns:       maxTurns,
		CompactAfter:   merged.CompactAfter,
		AutoCompact:    merged.AutoCompact,
		EnableStreaming: opts.Streaming,
		SessionPath:    sessionPath,
		SelectedTools:  toolNames,
		ResourceLoader: rl,
	}

	if agentConfig.CompactAfter == 0 {
		agentConfig.CompactAfter = 50
	}

	// Create runtime and session
	runtime, err := NewAgentSessionRuntime(agentConfig)
	if err != nil {
		return nil, fmt.Errorf("create runtime: %w", err)
	}

	sess, err := runtime.NewSession(modelInfo.ID, cwd)
	if err != nil {
		return nil, fmt.Errorf("create session: %w", err)
	}

	// Set thinking level
	sess.SetThinkingLevel(thinkingLevel)

	// Wire LLM client
	llmClient := NewHTTPClient(HTTPClientConfig{
		BaseURL:      baseURL,
		APIKey:       apiKey,
		DefaultModel: modelInfo.ID,
		MaxTokens:    maxTokens,
	})
	sess.SetLLMClient(llmClient)

	// Streaming callback
	if opts.Streaming {
		sess.SetStreamCallback(func(text string) {
			fmt.Fprint(os.Stderr, text)
		})
	}

	// Session CWD validation
	sessionFile := sess.session.GetSessionFile()
	sessionCwd := sess.session.GetCwd()
	if err := session.AssertSessionCwdExists(sessionFile, sessionCwd, cwd); err != nil {
		warnings = append(warnings, err.Error())
	}

	return &CreateSessionResult{
		Session:       sess,
		Model:         modelInfo,
		ModelFallback: "",
		Warnings:      warnings,
		AuthStorage:   authStorage,
	}, nil
}
