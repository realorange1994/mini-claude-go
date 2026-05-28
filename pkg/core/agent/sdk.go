// Package agent provides the SDK factory for creating agent sessions.
// Aligned to pi's sdk.ts createAgentSession().
package agent

import (
	"fmt"
	"os"

	"miniclaudecode-go/pkg/core/config"
	"miniclaudecode-go/pkg/core/modelregistry"
)

// CreateSessionOptions configures how a session is created.
type CreateSessionOptions struct {
	// Working directory. Default: os.Getwd()
	Cwd string
	// Global config dir. Default: ~/.miniclaude/
	AgentDir string

	// Model ID to use. Can be a short alias like "sonnet", "opus", "haiku".
	// Default: from settings, then "claude-sonnet-4-20250514"
	Model string
	// Max turns per message loop. 0 = unlimited (default 100)
	MaxTurns int
	// Max tokens per LLM response. Default: 8192
	MaxTokens int
	// Enable streaming output
	Streaming bool

	// Tool names to enable. Default: built-in tools
	Tools []string
	// Initial user message (one-shot mode)
	Message string

	// API key override (skips settings/env resolution)
	APIKey string
	// Base URL override (skips settings/env resolution)
	BaseURL string
}

// CreateSessionResult is returned by CreateSession.
type CreateSessionResult struct {
	Session *AgentSession
	Model   modelregistry.ModelInfo
	// Warning if model was resolved from a fallback
	ModelFallback string
	// Non-fatal warnings encountered during session creation
	Warnings []string
}

// CreateSession creates a fully configured agent session.
// It resolves the model, API key, settings, and tools from the
// provided options, falling back to env vars and defaults.
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
		// Settings load failure is not fatal, but record it
		w := fmt.Sprintf("settings load error: %v", err)
		warnings = append(warnings, w)
		fmt.Fprintf(os.Stderr, "[!] %s\n", w)
	}
	merged := settingsMgr.Merged()

	// Create model registry
	modelReg := modelregistry.NewRegistry()
	modelsPath := fmt.Sprintf("%s/models.json", agentDir)
	if err := modelReg.LoadFromJSON(modelsPath); err != nil {
		// Not fatal — built-in models still available
		fmt.Fprintf(os.Stderr, "[!] models.json load error: %v\n", err)
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
		apiKey, err = modelReg.ResolveAPIKey(modelInfo)
		if err != nil {
			return nil, fmt.Errorf("resolve API key: %w", err)
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
	if baseURL != "" && !endsWith(baseURL, "/v1/messages") {
		baseURL = trimSuffix(baseURL, "/") + "/v1/messages"
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

	// Session path
	sessionPath := fmt.Sprintf("%s/sessions", agentDir)

	// Build agent config
	agentConfig := AgentConfig{
		Model:        modelInfo.ID,
		Cwd:          cwd,
		MaxTurns:     maxTurns,
		CompactAfter: merged.CompactAfter,
		AutoCompact:  merged.AutoCompact,
		EnableStreaming: opts.Streaming,
		SessionPath:  sessionPath,
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

	return &CreateSessionResult{
		Session:       sess,
		Model:         modelInfo,
		ModelFallback: "",
		Warnings:      warnings,
	}, nil
}

// helper string functions (avoid importing strings just for these)
func endsWith(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}

func trimSuffix(s, suffix string) string {
	if endsWith(s, suffix) {
		return s[:len(s)-len(suffix)]
	}
	return s
}