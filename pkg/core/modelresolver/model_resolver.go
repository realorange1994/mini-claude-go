// Package modelresolver provides model resolution, pattern matching,
// and initial selection aligned to pi's model-resolver.ts.
package modelresolver

import (
	"fmt"
	"regexp"
	"strings"

	"miniclaudecode-go/pkg/core/agent"
	"miniclaudecode-go/pkg/core/modelregistry"
)

// defaultModelPerProvider maps provider IDs to default model IDs.
var defaultModelPerProvider = map[string]string{
	"anthropic":               "claude-opus-4-20250514",
	"openai":                  "gpt-4o",
	"google":                  "gemini-2.5-pro",
	"openrouter":              "anthropic/claude-sonnet-4",
	"deepseek":                "deepseek-chat",
	"aws-bedrock":             "us.anthropic.claude-sonnet-4-20250514-v1:0",
	"azure-openai":            "gpt-4o",
	"xai":                     "grok-3",
	"mistral":                 "mistral-large-latest",
	"minimax":                 "MiniMax-M2",
	"moonshotai":              "kimi-k2.6",
	"together-ai":             "moonshotai/Kimi-K2.6",
	"groq":                    "llama-3.3-70b-versatile",
	"cohere":                  "command-r-plus",
	"fireworks":               "accounts/fireworks/models/claude-sonnet-4",
	"vercel-ai-sdk":           "anthropic/claude-sonnet-4",
	"cloudflare-workers-ai":   "@cf/meta/llama-3.3-70b-instruct",
	"cloudflare-ai-gateway":   "workers-ai/@cf/meta/llama-3.3-70b-instruct",
	"perplexity":              "sonar-reasoning",
	"hyperbolic":              "meta-llama/Meta-Llama-3.1-405B",
	"novita-ai":               "meta-llama/llama-3.1-405b",
}

// datePattern matches model IDs ending with a date suffix like -20250514
var datePattern = regexp.MustCompile(`-\d{8}$`)

// isAlias checks if a model ID looks like an alias (no date suffix).
func isAlias(id string) bool {
	if strings.HasSuffix(id, "-latest") {
		return true
	}
	return !datePattern.MatchString(id)
}

// ScopedModel pairs a model with an optional thinking level.
type ScopedModel struct {
	Model         modelregistry.ModelInfo
	ThinkingLevel agent.ThinkingLevel // empty if not specified
}

// ParsedModelResult is the result of parsing a model pattern.
type ParsedModelResult struct {
	Model         modelregistry.ModelInfo
	Found         bool
	ThinkingLevel agent.ThinkingLevel
	Warning       string
}

// isValidThinkingLevel checks if a string is a valid thinking level.
func isValidThinkingLevel(s string) bool {
	for _, l := range agent.ValidThinkingLevels() {
		if string(l) == s {
			return true
		}
	}
	return false
}

// findExactModelReferenceMatch finds an exact match for a model reference.
// Supports "provider/id" format and bare id (rejects ambiguous bare id matches).
func findExactModelReferenceMatch(ref string, models []modelregistry.ModelInfo) modelregistry.ModelInfo {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return modelregistry.ModelInfo{}
	}
	norm := strings.ToLower(ref)

	// Try canonical match: "provider/id"
	canonical := make([]modelregistry.ModelInfo, 0)
	for _, m := range models {
		if strings.ToLower(m.Provider+"/"+m.ID) == norm {
			canonical = append(canonical, m)
		}
	}
	if len(canonical) == 1 {
		return canonical[0]
	}

	// Try provider/id with slashes
	if slash := strings.Index(ref, "/"); slash != -1 {
		prov := strings.TrimSpace(ref[:slash])
		id := strings.TrimSpace(ref[slash+1:])
		if prov != "" && id != "" {
			provMatches := make([]modelregistry.ModelInfo, 0)
			for _, m := range models {
				if strings.EqualFold(m.Provider, prov) && strings.EqualFold(m.ID, id) {
					provMatches = append(provMatches, m)
				}
			}
			if len(provMatches) == 1 {
				return provMatches[0]
			}
		}
	}

	// Try bare id match (only accept if exactly one)
	idMatches := make([]modelregistry.ModelInfo, 0)
	for _, m := range models {
		if strings.EqualFold(m.ID, norm) {
			idMatches = append(idMatches, m)
		}
	}
	if len(idMatches) == 1 {
		return idMatches[0]
	}

	return modelregistry.ModelInfo{}
}

// tryMatchModel tries to match a pattern to a model.
func tryMatchModel(pattern string, models []modelregistry.ModelInfo) modelregistry.ModelInfo {
	exact := findExactModelReferenceMatch(pattern, models)
	if exact.ID != "" {
		return exact
	}

	// Partial matching
	var matches []modelregistry.ModelInfo
	for _, m := range models {
		lowerID := strings.ToLower(m.ID)
		lowerName := strings.ToLower(m.Name)
		lowerPattern := strings.ToLower(pattern)
		if strings.Contains(lowerID, lowerPattern) || strings.Contains(lowerName, lowerPattern) {
			matches = append(matches, m)
		}
	}

	if len(matches) == 0 {
		return modelregistry.ModelInfo{}
	}

	// Separate aliases and dated versions
	var aliases, dated []modelregistry.ModelInfo
	for _, m := range matches {
		if isAlias(m.ID) {
			aliases = append(aliases, m)
		} else {
			dated = append(dated, m)
		}
	}

	if len(aliases) > 0 {
		// Sort descending, pick first
		return aliases[0]
	}
	if len(dated) > 0 {
		return dated[0]
	}
	return matches[0]
}

// buildFallbackModel creates a fallback model for a provider with a custom ID.
func buildFallbackModel(provider, modelID string, models []modelregistry.ModelInfo) modelregistry.ModelInfo {
	var providerModels []modelregistry.ModelInfo
	for _, m := range models {
		if m.Provider == provider {
			providerModels = append(providerModels, m)
		}
	}
	if len(providerModels) == 0 {
		return modelregistry.ModelInfo{}
	}

	base := providerModels[0]
	if defaultID, ok := defaultModelPerProvider[provider]; ok {
		for _, m := range providerModels {
			if m.ID == defaultID {
				base = m
				break
			}
		}
	}

	return modelregistry.ModelInfo{
		ID:            modelID,
		Name:          modelID,
		Provider:      provider,
		BaseURL:       base.BaseURL,
		MaxTokens:     base.MaxTokens,
		ContextWindow: base.ContextWindow,
		Reasoning:     base.Reasoning,
	}
}

// parseModelPattern parses a model pattern string to extract model and thinking level.
// Handles "model:thinking" suffix patterns.
func parseModelPattern(pattern string, models []modelregistry.ModelInfo) ParsedModelResult {
	// Try exact match first
	match := tryMatchModel(pattern, models)
	if match.ID != "" {
		return ParsedModelResult{Model: match, Found: true}
	}

	// No match - try splitting on last colon
	colonIdx := strings.LastIndex(pattern, ":")
	if colonIdx == -1 {
		return ParsedModelResult{}
	}

	prefix := pattern[:colonIdx]
	suffix := pattern[colonIdx+1:]

	if isValidThinkingLevel(suffix) {
		result := parseModelPattern(prefix, models)
		if result.Found {
			if result.Warning == "" {
				result.ThinkingLevel = agent.ThinkingLevel(suffix)
			}
			return result
		}
		return result
	}

	// Invalid suffix - recurse on prefix
	result := parseModelPattern(prefix, models)
	if result.Found {
		result.Warning = fmt.Sprintf("Invalid thinking level %q in pattern %q. Using default instead.", suffix, pattern)
	}
	return result
}

// ResolveModelScope resolves model patterns (including glob patterns) to actual models.
func ResolveModelScope(patterns []string, reg *modelregistry.Registry) ([]ScopedModel, []string) {
	available := reg.Available()
	var scoped []ScopedModel
	var warnings []string

	for _, pattern := range patterns {
		// Check for glob characters
		if strings.ContainsAny(pattern, "*?[") {
			colonIdx := strings.LastIndex(pattern, ":")
			globPattern := pattern
			var thinkingLevel agent.ThinkingLevel

			if colonIdx != -1 {
				suffix := pattern[colonIdx+1:]
				if isValidThinkingLevel(suffix) {
					thinkingLevel = agent.ThinkingLevel(suffix)
					globPattern = pattern[:colonIdx]
				}
			}

			// Simple glob matching (no minimatch, basic wildcard)
			for _, m := range available {
				fullID := m.Provider + "/" + m.ID
				if matchGlob(globPattern, fullID) || matchGlob(globPattern, m.ID) {
					// Avoid duplicates
					dup := false
					for _, s := range scoped {
						if s.Model.Provider == m.Provider && s.Model.ID == m.ID {
							dup = true
							break
						}
					}
					if !dup {
						scoped = append(scoped, ScopedModel{Model: m, ThinkingLevel: thinkingLevel})
					}
				}
			}
			continue
		}

		result := parseModelPattern(pattern, available)
		if result.Warning != "" {
			warnings = append(warnings, result.Warning)
		}
		if !result.Found {
			warnings = append(warnings, fmt.Sprintf("No models match pattern %q", pattern))
			continue
		}

		// Avoid duplicates
		dup := false
		for _, s := range scoped {
			if s.Model.Provider == result.Model.Provider && s.Model.ID == result.Model.ID {
				dup = true
				break
			}
		}
		if !dup {
			scoped = append(scoped, ScopedModel{Model: result.Model, ThinkingLevel: result.ThinkingLevel})
		}
	}

	return scoped, warnings
}

// matchGlob performs basic glob matching with * and ? wildcards.
func matchGlob(pattern, s string) bool {
	if pattern == "*" {
		return true
	}
	if pattern == s {
		return true
	}

	// Simple single-star matching: "prefix*" or "*suffix" or "*middle*"
	if !strings.Contains(pattern, "*") && !strings.Contains(pattern, "?") {
		return false
	}

	// Convert glob pattern to regex (basic conversion)
	rePattern := "^"
	for _, c := range pattern {
		switch c {
		case '*':
			rePattern += ".*"
		case '?':
			rePattern += "."
		case '.', '+', '(', ')', '{', '}', '[', ']', '|', '\\', '^', '$':
			rePattern += "\\" + string(c)
		default:
			rePattern += string(c)
		}
	}
	rePattern += "$"

	matched, _ := regexp.MatchString("(?i)"+rePattern, s)
	return matched
}

// ResolveCliModelResult is the result of resolving a CLI model flag.
type ResolveCliModelResult struct {
	Model         modelregistry.ModelInfo
	Found         bool
	ThinkingLevel agent.ThinkingLevel
	Warning       string
	Error         string
}

// ResolveCliModel resolves a single model from CLI flags.
// Supports --provider <provider> --model <pattern> and --model <provider>/<pattern>.
func ResolveCliModel(cliProvider, cliModel string, reg *modelregistry.Registry) ResolveCliModelResult {
	if cliModel == "" {
		return ResolveCliModelResult{}
	}

	available := reg.All()
	if len(available) == 0 {
		return ResolveCliModelResult{
			Error: "No models available. Check your installation or add models to models.json.",
		}
	}

	// Build canonical provider lookup
	providerMap := make(map[string]string)
	for _, m := range available {
		providerMap[strings.ToLower(m.Provider)] = m.Provider
	}

	provider := ""
	if cliProvider != "" {
		if p, ok := providerMap[strings.ToLower(cliProvider)]; ok {
			provider = p
		} else {
			return ResolveCliModelResult{
				Error: fmt.Sprintf("Unknown provider %q. Use --list-models to see available providers/models.", cliProvider),
			}
		}
	}

	pattern := cliModel
	inferredProvider := false

	if provider == "" {
		if slash := strings.Index(cliModel, "/"); slash != -1 {
			maybeProv := cliModel[:slash]
			if canonical, ok := providerMap[strings.ToLower(maybeProv)]; ok {
				provider = canonical
				pattern = cliModel[slash+1:]
				inferredProvider = true
			}
		}
	}

	// If no provider inferred, try exact match without inference
	if provider == "" {
		norm := strings.ToLower(cliModel)
		for _, m := range available {
			if strings.EqualFold(m.ID, norm) || strings.ToLower(m.Provider+"/"+m.ID) == norm {
				return ResolveCliModelResult{Model: m, Found: true}
			}
		}
	}

	// If both --provider and --model provider/pattern provided, strip prefix
	if cliProvider != "" && provider != "" {
		prefix := provider + "/"
		if strings.HasPrefix(strings.ToLower(cliModel), strings.ToLower(prefix)) {
			pattern = cliModel[len(prefix):]
		}
	}

	// Find candidates
	var candidates []modelregistry.ModelInfo
	if provider != "" {
		for _, m := range available {
			if m.Provider == provider {
				candidates = append(candidates, m)
			}
		}
	} else {
		candidates = available
	}

	result := parseModelPattern(pattern, candidates)
	if result.Found {
		return ResolveCliModelResult{
			Model:         result.Model,
			Found:         true,
			ThinkingLevel: result.ThinkingLevel,
			Warning:       result.Warning,
		}
	}

	// If provider was inferred but no match, try full input against all models
	if inferredProvider {
		norm := strings.ToLower(cliModel)
		for _, m := range available {
			if strings.EqualFold(m.ID, norm) || strings.ToLower(m.Provider+"/"+m.ID) == norm {
				return ResolveCliModelResult{Model: m, Found: true}
			}
		}
		// Try parseModelPattern on full input
		fallback := parseModelPattern(cliModel, available)
		if fallback.Found {
			return ResolveCliModelResult{
				Model:         fallback.Model,
				Found:         true,
				ThinkingLevel: fallback.ThinkingLevel,
				Warning:       fallback.Warning,
			}
		}
	}

	// Build fallback model for provider
	if provider != "" {
		fm := buildFallbackModel(provider, pattern, available)
		if fm.ID != "" {
			warn := result.Warning
			if warn == "" {
				warn = fmt.Sprintf("Model %q not found for provider %q. Using custom model id.", pattern, provider)
			}
			return ResolveCliModelResult{
				Model:         fm,
				Found:         true,
				Warning:       warn,
			}
		}
	}

	display := cliModel
	if provider != "" {
		display = provider + "/" + pattern
	}
	return ResolveCliModelResult{
		Error:   fmt.Sprintf("Model %q not found. Use --list-models to see available models.", display),
		Warning: result.Warning,
	}
}

// InitialModelResult is the result of finding the initial model.
type InitialModelResult struct {
	Model           modelregistry.ModelInfo
	Found           bool
	ThinkingLevel   agent.ThinkingLevel
	FallbackMessage string
}

// FindInitialModel finds the initial model with priority:
// 1. CLI args
// 2. Scoped models (if not continuing)
// 3. Saved defaults from settings
// 4. First available model with valid API key
func FindInitialModel(opts FindInitialModelOpts) InitialModelResult {
	// 1. CLI args
	if opts.CliProvider != "" && opts.CliModel != "" {
		resolved := ResolveCliModel(opts.CliProvider, opts.CliModel, opts.ModelRegistry)
		if resolved.Error != "" {
			return InitialModelResult{
				FallbackMessage: resolved.Error,
			}
		}
		if resolved.Found {
			return InitialModelResult{Model: resolved.Model, Found: true, ThinkingLevel: agent.ThinkingLevelMedium}
		}
	}

	// 2. Scoped models
	if len(opts.ScopedModels) > 0 && !opts.IsContinuing {
		sm := opts.ScopedModels[0]
		tl := sm.ThinkingLevel
		if tl == "" {
			tl = opts.DefaultThinkingLevel
		}
		if tl == "" {
			tl = agent.ThinkingLevelMedium
		}
		return InitialModelResult{Model: sm.Model, Found: true, ThinkingLevel: tl}
	}

	// 3. Saved defaults
	if opts.DefaultProvider != "" && opts.DefaultModelID != "" {
		if m, ok := opts.ModelRegistry.Find(opts.DefaultProvider, opts.DefaultModelID); ok {
			tl := opts.DefaultThinkingLevel
			if tl == "" {
				tl = agent.ThinkingLevelMedium
			}
			return InitialModelResult{Model: m, Found: true, ThinkingLevel: tl}
		}
	}

	// 4. First available model
	available := opts.ModelRegistry.Available()
	if len(available) > 0 {
		// Try known provider defaults
		for provider, defaultID := range defaultModelPerProvider {
			for _, m := range available {
				if m.Provider == provider && m.ID == defaultID {
					return InitialModelResult{Model: m, Found: true, ThinkingLevel: agent.ThinkingLevelMedium}
				}
			}
		}
		// Use first available
		return InitialModelResult{Model: available[0], Found: true, ThinkingLevel: agent.ThinkingLevelMedium}
	}

	return InitialModelResult{ThinkingLevel: agent.ThinkingLevelMedium}
}

// FindInitialModelOpts configures FindInitialModel.
type FindInitialModelOpts struct {
	CliProvider        string
	CliModel           string
	ScopedModels       []ScopedModel
	IsContinuing       bool
	DefaultProvider    string
	DefaultModelID     string
	DefaultThinkingLevel agent.ThinkingLevel
	ModelRegistry      *modelregistry.Registry
}

// RestoreModelFromSession restores model from session with fallback.
func RestoreModelFromSession(savedProvider, savedModelID string, currentModel modelregistry.ModelInfo, reg *modelregistry.Registry) (modelregistry.ModelInfo, string, bool) {
	restored, ok := reg.Find(savedProvider, savedModelID)
	if !ok {
		// Model no longer exists
		fallbackMsg := fmt.Sprintf("Could not restore model %s/%s (model no longer exists).", savedProvider, savedModelID)
		return currentModel, fallbackMsg, currentModel.ID != ""
	}

	// Check auth
	if _, ok2 := reg.Find(savedProvider, savedModelID); ok2 {
		// Try to resolve API key
		if _, err := reg.ResolveAPIKey(restored); err != nil {
			fallbackMsg := fmt.Sprintf("Could not restore model %s/%s (no auth configured).", savedProvider, savedModelID)
			return fallbackModel(savedProvider, savedModelID, currentModel, reg), fallbackMsg, currentModel.ID != ""
		}
	}

	return restored, "", false
}

// fallbackModel returns a fallback model when restore fails.
func fallbackModel(savedProvider, savedModelID string, currentModel modelregistry.ModelInfo, reg *modelregistry.Registry) modelregistry.ModelInfo {
	if currentModel.ID != "" {
		return currentModel
	}
	available := reg.Available()
	if len(available) > 0 {
		return available[0]
	}
	return modelregistry.ModelInfo{}
}