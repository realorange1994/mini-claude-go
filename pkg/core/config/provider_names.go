// Package config provides configuration constants.
package config

// BUILT_IN_PROVIDER_DISPLAY_NAMES maps provider IDs to human-readable names.
// Aligned to pi's provider-display-names.ts.
var BUILT_IN_PROVIDER_DISPLAY_NAMES = map[string]string{
	"anthropic":              "Anthropic",
	"openai":                 "OpenAI",
	"openrouter":             "OpenRouter",
	"google":                 "Google",
	"google-vertex-ai":       "Google Vertex AI",
	"azure-openai":           "Azure OpenAI",
	"aws-bedrock":            "AWS Bedrock",
	"aws-bedrock-converse":   "AWS Bedrock Converse",
	"gemini":                 "Gemini",
	"ollama":                 "Ollama",
	"litellm":                "LiteLLM",
	"vercel-ai-sdk":          "Vercel AI SDK",
	"together-ai":            "Together AI",
	"groq":                   "Groq",
	"mistral":                "Mistral",
	"cohere":                 "Cohere",
	"deepseek":               "DeepSeek",
	"opencode":               "OpenCode",
	"cloudflare-workers-ai":  "Cloudflare Workers AI",
	"cloudflare-ai-gateway":  "Cloudflare AI Gateway",
	"xai":                    "xAI",
	"perplexity":             "Perplexity",
	"fireworks":              "Fireworks",
	"hyperbolic":             "Hyperbolic",
	"novita-ai":              "Novita AI",
	"voyage-ai":              "Voyage AI",
	"voyage":                 "Voyage",
	"lm-studio":              "LM Studio",
	"nebius":                 "Nebius",
	"replicate":              "Replicate",
	"replicate-ai":           "Replicate AI",
	"cerenai":                "Cerenai",
}

// GetProviderDisplayName returns a human-readable name for a provider ID.
func GetProviderDisplayName(providerID string) string {
	if name, ok := BUILT_IN_PROVIDER_DISPLAY_NAMES[providerID]; ok {
		return name
	}
	return providerID
}