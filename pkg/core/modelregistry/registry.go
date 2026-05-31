// Package modelregistry manages model definitions and API key resolution.
// Aligned to pi's model-registry.ts.
package modelregistry

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
)

// ModelInfo describes a model that can be used by the agent.
type ModelInfo struct {
	ID           string  `json:"id"`
	Name         string  `json:"name,omitempty"`
	Provider     string  `json:"provider"`
	BaseURL      string  `json:"baseUrl"`
	APIKey       string  `json:"apiKey,omitempty"` // resolved at runtime
	MaxTokens    int     `json:"maxTokens"`
	ContextWindow int   `json:"contextWindow"`
	Reasoning    bool    `json:"reasoning,omitempty"`
	InputPrice   float64 `json:"inputPrice,omitempty"`  // per 1M tokens
	OutputPrice  float64 `json:"outputPrice,omitempty"` // per 1M tokens
}

// ProviderConfig describes a provider in models.json.
type ProviderConfig struct {
	Name     string        `json:"name,omitempty"`
	BaseURL  string        `json:"baseUrl,omitempty"`
	APIKey   string        `json:"apiKey,omitempty"`
	API      string        `json:"api,omitempty"`
	Models   []ModelDef    `json:"models,omitempty"`
}

// ModelDef is a model definition from models.json.
type ModelDef struct {
	ID            string  `json:"id"`
	Name          string  `json:"name,omitempty"`
	BaseURL       string  `json:"baseUrl,omitempty"`
	MaxTokens     int     `json:"maxTokens,omitempty"`
	ContextWindow int     `json:"contextWindow,omitempty"`
	Reasoning     bool    `json:"reasoning,omitempty"`
	InputPrice    float64 `json:"inputPrice,omitempty"`
	OutputPrice   float64 `json:"outputPrice,omitempty"`
}

// ModelsConfig is the top-level structure of models.json.
type ModelsConfig struct {
	Providers map[string]ProviderConfig `json:"providers"`
}

// Registry manages available models and resolves API keys.
type Registry struct {
	mu     sync.RWMutex
	models []ModelInfo
	// API key sources: provider -> apiKey
	providerKeys map[string]string
	loadError    error
}

// NewRegistry creates an empty model registry.
func NewRegistry() *Registry {
	return &Registry{
		models:       builtInModels(),
		providerKeys: make(map[string]string),
	}
}

// LoadFromJSON loads custom models from a models.json file.
// Built-in models are kept; custom models override on provider+id conflict.
func (r *Registry) LoadFromJSON(path string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // no custom models file is fine
		}
		r.loadError = err
		return err
	}

	// Strip // comments (simple line-comment stripper)
	cleaned := stripComments(string(data))

	var config ModelsConfig
	if err := json.Unmarshal([]byte(cleaned), &config); err != nil {
		r.loadError = fmt.Errorf("parse models.json: %w", err)
		return r.loadError
	}

	// Parse custom models
	var custom []ModelInfo
	for provider, pc := range config.Providers {
		if pc.APIKey != "" {
			r.providerKeys[provider] = pc.APIKey
		}
		for _, md := range pc.Models {
			baseURL := md.BaseURL
			if baseURL == "" {
				baseURL = pc.BaseURL
			}
			maxTokens := md.MaxTokens
			if maxTokens == 0 {
				maxTokens = 16384
			}
			ctxWindow := md.ContextWindow
			if ctxWindow == 0 {
				ctxWindow = 128000
			}
			name := md.Name
			if name == "" {
				name = md.ID
			}
			custom = append(custom, ModelInfo{
				ID:            md.ID,
				Name:          name,
				Provider:      provider,
				BaseURL:       baseURL,
				MaxTokens:     maxTokens,
				ContextWindow: ctxWindow,
				Reasoning:     md.Reasoning,
				InputPrice:    md.InputPrice,
				OutputPrice:   md.OutputPrice,
			})
		}
	}

	// Merge: custom overrides built-in on provider+id match
	merged := make([]ModelInfo, 0, len(r.models)+len(custom))
	merged = append(merged, r.models...)
	for _, cm := range custom {
		replaced := false
		for i, m := range merged {
			if m.Provider == cm.Provider && m.ID == cm.ID {
				merged[i] = cm
				replaced = true
				break
			}
		}
		if !replaced {
			merged = append(merged, cm)
		}
	}
	r.models = merged
	r.loadError = nil
	return nil
}

// All returns all registered models.
func (r *Registry) All() []ModelInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]ModelInfo, len(r.models))
	copy(out, r.models)
	return out
}

// Available returns models that have auth configured.
func (r *Registry) Available() []ModelInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []ModelInfo
	for _, m := range r.models {
		if r.hasAuth(m.Provider) {
			out = append(out, m)
		}
	}
	return out
}

// Find looks up a model by provider and ID.
func (r *Registry) Find(provider, id string) (ModelInfo, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, m := range r.models {
		if m.Provider == provider && m.ID == id {
			return m, true
		}
	}
	return ModelInfo{}, false
}

// FindByID looks up a model by ID across all providers.
func (r *Registry) FindByID(id string) (ModelInfo, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, m := range r.models {
		if m.ID == id {
			return m, true
		}
	}
	return ModelInfo{}, false
}

// ResolveAPIKey resolves the API key for a model.
// Priority: model's APIKey > provider key from models.json > environment variable.
func (r *Registry) ResolveAPIKey(m ModelInfo) (string, error) {
	if m.APIKey != "" {
		return m.APIKey, nil
	}
	r.mu.RLock()
	pk := r.providerKeys[m.Provider]
	r.mu.RUnlock()
	if pk != "" {
		// If it looks like an env var reference, resolve it
		if strings.HasPrefix(pk, "$") {
			envKey := pk[1:]
			v := os.Getenv(envKey)
			if v == "" {
				return "", fmt.Errorf("env var %s not set for provider %s", envKey, m.Provider)
			}
			return v, nil
		}
		return pk, nil
	}
	// Fallback to well-known env vars
	switch m.Provider {
	case "anthropic":
		if v := os.Getenv("ANTHROPIC_API_KEY"); v != "" {
			return v, nil
		}
		if v := os.Getenv("ANTHROPIC_AUTH_TOKEN"); v != "" {
			return v, nil
		}
	case "bedrock":
		// Bedrock uses AWS credentials — not a simple API key
	case "vertex":
		// Vertex uses Google credentials
	default:
		// Generic: try PROVIDER_API_KEY
		if v := os.Getenv(strings.ToUpper(m.Provider) + "_API_KEY"); v != "" {
			return v, nil
		}
	}
	return "", fmt.Errorf("no API key found for provider %s", m.Provider)
}

// HasAuth checks if a provider has auth configured.
func (r *Registry) hasAuth(provider string) bool {
	if r.providerKeys[provider] != "" {
		return true
	}
	switch provider {
	case "anthropic":
		return os.Getenv("ANTHROPIC_API_KEY") != "" || os.Getenv("ANTHROPIC_AUTH_TOKEN") != ""
	default:
		return os.Getenv(strings.ToUpper(provider)+"_API_KEY") != ""
	}
}

// LoadError returns any error from loading models.json.
func (r *Registry) LoadError() error {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.loadError
}

// Register adds a model dynamically (e.g., from extensions).
func (r *Registry) Register(m ModelInfo) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, existing := range r.models {
		if existing.Provider == m.Provider && existing.ID == m.ID {
			r.models[i] = m
			return
		}
	}
	r.models = append(r.models, m)
}

// Unregister removes a model by provider and ID.
func (r *Registry) Unregister(provider, id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i, m := range r.models {
		if m.Provider == provider && m.ID == id {
			r.models = append(r.models[:i], r.models[i+1:]...)
			return
		}
	}
}

// DefaultModelPerProvider maps provider IDs to their default model IDs.
// Aligned to TS model-resolver.ts defaultModelPerProvider (updated to latest).
var DefaultModelPerProvider = map[string]string{
	"anthropic":              "claude-opus-4-7",
	"amazon-bedrock":         "us.anthropic.claude-opus-4-6-v1",
	"openai":                 "gpt-5.4",
	"azure-openai-responses": "gpt-5.4",
	"openai-codex":           "gpt-5.5",
	"deepseek":               "deepseek-v4-pro",
	"google":                 "gemini-3.1-pro-preview",
	"google-vertex":          "gemini-3.1-pro-preview",
	"github-copilot":          "gpt-5.4",
	"openrouter":             "moonshotai/kimi-k2.6",
	"vercel-ai-gateway":      "zai/glm-5.1",
	"xai":                    "grok-4.20-0309-reasoning",
	"groq":                   "openai/gpt-oss-120b",
	"cerebras":               "zai-glm-4.7",
	"zai":                    "glm-5.1",
	"mistral":                "devstral-medium-latest",
	"minimax":                "MiniMax-M2.7",
	"minimax-cn":             "MiniMax-M2.7",
	"moonshotai":             "kimi-k2.6",
	"moonshotai-cn":          "kimi-k2.6",
	"huggingface":            "moonshotai/Kimi-K2.6",
	"fireworks":              "accounts/fireworks/models/kimi-k2p6",
	"together":               "moonshotai/Kimi-K2.6",
	"opencode":               "kimi-k2.6",
	"opencode-go":            "kimi-k2.6",
	"kimi-coding":            "kimi-for-coding",
	"cloudflare-workers-ai":  "@cf/moonshotai/kimi-k2.6",
	"cloudflare-ai-gateway":   "workers-ai/@cf/moonshotai/kimi-k2.6",
	"xiaomi":                 "mimo-v2.5-pro",
	"xiaomi-token-plan-cn":   "mimo-v2.5-pro",
	"xiaomi-token-plan-ams":  "mimo-v2.5-pro",
	"xiaomi-token-plan-sgp":  "mimo-v2.5-pro",
	// Legacy aliases for backward compatibility
	"azure":       "gpt-5.4",
	"vertex":      "gemini-3.1-pro-preview",
	"bedrock":     "us.anthropic.claude-opus-4-6-v1",
	"litellm":     "claude-opus-4-7",
	"ollama":      "llama3",
	"lm-studio":   "local-model",
	"cohere":      "command-a-06-2025",
	"perplexity":  "sonar-pro",
}

// ProviderDisplayNames maps provider IDs to human-readable display names.
// Aligned to TS provider-display-names.ts.
var ProviderDisplayNames = map[string]string{
	"anthropic":             "Anthropic",
	"openai":                "OpenAI",
	"openrouter":            "OpenRouter",
	"google":                "Google",
	"google-vertex-ai":      "Google Vertex AI",
	"azure-openai":          "Azure OpenAI",
	"aws-bedrock":           "AWS Bedrock",
	"gemini":                "Gemini",
	"ollama":                "Ollama",
	"litellm":               "LiteLLM",
	"vercel-ai-sdk":         "Vercel AI SDK",
	"together-ai":           "Together AI",
	"groq":                  "Groq",
	"mistral":               "Mistral",
	"cohere":                "Cohere",
	"deepseek":              "DeepSeek",
	"opencode":              "OpenCode",
	"cloudflare-workers-ai": "Cloudflare Workers AI",
	"xai":                   "xAI",
	"perplexity":            "Perplexity",
	"fireworks":             "Fireworks",
	"lm-studio":             "LM Studio",
	"nebius":                "Nebius",
	"replicate":             "Replicate",
	"bedrock":               "Amazon Bedrock",
	"vertex":                "Google Vertex AI",
	"azure":                 "Azure OpenAI",
	"cloudflare":            "Cloudflare",
	"together":              "Together AI",
	"vercel":                "Vercel",
	"bedrock-converse":      "AWS Bedrock Converse",
	"kimi-coding":           "Kimi Coding",
}

// GetProviderDisplayName returns a human-readable name for a provider.
func GetProviderDisplayName(providerID string) string {
	if name, ok := ProviderDisplayNames[providerID]; ok {
		return name
	}
	return providerID
}

// GetDefaultModelForProvider returns the default model ID for a provider.
// Returns empty string if no default is known.
func GetDefaultModelForProvider(providerID string) string {
	return DefaultModelPerProvider[providerID]
}

// RegisterProvider adds all models from a provider dynamically (e.g., from extensions).
func (r *Registry) RegisterProvider(provider string, models []ModelDef, baseURL string, apiKey string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if apiKey != "" {
		r.providerKeys[provider] = apiKey
	}
	for _, md := range models {
		// Skip if already registered
		exists := false
		for _, m := range r.models {
			if m.Provider == provider && m.ID == md.ID {
				exists = true
				break
			}
		}
		if exists {
			continue
		}
		bURL := md.BaseURL
		if bURL == "" {
			bURL = baseURL
		}
		maxTokens := md.MaxTokens
		if maxTokens == 0 {
			maxTokens = 8192
		}
		ctxWindow := md.ContextWindow
		if ctxWindow == 0 {
			ctxWindow = 200000
		}
		name := md.Name
		if name == "" {
			name = md.ID
		}
		r.models = append(r.models, ModelInfo{
			ID:            md.ID,
			Name:          name,
			Provider:      provider,
			BaseURL:       bURL,
			APIKey:        apiKey,
			MaxTokens:     maxTokens,
			ContextWindow: ctxWindow,
			Reasoning:     md.Reasoning,
			InputPrice:    md.InputPrice,
			OutputPrice:   md.OutputPrice,
		})
	}
}

// Refresh rebuilds models from a models.json file, replacing all previously loaded models.
func (r *Registry) Refresh(path string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Keep built-in models, clear loaded ones
	r.models = builtInModels()
	r.providerKeys = make(map[string]string)

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		r.loadError = err
		return err
	}

	cleaned := stripComments(string(data))
	var config ModelsConfig
	if err := json.Unmarshal([]byte(cleaned), &config); err != nil {
		r.loadError = fmt.Errorf("parse models.json: %w", err)
		return r.loadError
	}

	for provider, pc := range config.Providers {
		if pc.APIKey != "" {
			r.providerKeys[provider] = pc.APIKey
		}
		for _, md := range pc.Models {
			baseURL := md.BaseURL
			if baseURL == "" {
				baseURL = pc.BaseURL
			}
			maxTokens := md.MaxTokens
			if maxTokens == 0 {
				maxTokens = 16384
			}
			ctxWindow := md.ContextWindow
			if ctxWindow == 0 {
				ctxWindow = 128000
			}
			name := md.Name
			if name == "" {
				name = md.ID
			}
			r.models = append(r.models, ModelInfo{
				ID:            md.ID,
				Name:          name,
				Provider:      provider,
				BaseURL:       baseURL,
				MaxTokens:     maxTokens,
				ContextWindow: ctxWindow,
				Reasoning:     md.Reasoning,
				InputPrice:    md.InputPrice,
				OutputPrice:   md.OutputPrice,
			})
		}
	}
	r.loadError = nil
	return nil
}

// builtInModels returns the default Anthropic models.
func builtInModels() []ModelInfo {
	return []ModelInfo{
		{
			ID: "claude-sonnet-4-20250514", Name: "Claude Sonnet 4",
			Provider: "anthropic", BaseURL: "https://api.anthropic.com",
			MaxTokens: 8192, ContextWindow: 200000,
			InputPrice: 3, OutputPrice: 15,
		},
		{
			ID: "claude-opus-4-20250514", Name: "Claude Opus 4",
			Provider: "anthropic", BaseURL: "https://api.anthropic.com",
			MaxTokens: 8192, ContextWindow: 200000,
			InputPrice: 15, OutputPrice: 75,
		},
		{
			ID: "claude-haiku-3-5-20241022", Name: "Claude Haiku 3.5",
			Provider: "anthropic", BaseURL: "https://api.anthropic.com",
			MaxTokens: 8192, ContextWindow: 200000,
			InputPrice: 0.80, OutputPrice: 4,
		},
	}
}

// stripComments removes // line comments from JSON, preserving strings.
func stripComments(input string) string {
	var b strings.Builder
	inString := false
	escape := false
	i := 0
	for i < len(input) {
		c := input[i]
		if escape {
			b.WriteByte(c)
			escape = false
			i++
			continue
		}
		if c == '\\' && inString {
			b.WriteByte(c)
			escape = true
			i++
			continue
		}
		if c == '"' {
			inString = !inString
			b.WriteByte(c)
			i++
			continue
		}
		if !inString && c == '/' && i+1 < len(input) && input[i+1] == '/' {
			// Skip until newline
			for i < len(input) && input[i] != '\n' {
				i++
			}
			continue
		}
		b.WriteByte(c)
		i++
	}
	return b.String()
}
