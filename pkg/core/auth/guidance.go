// Package auth provides authentication guidance and help messages.
// Aligned to pi's auth-guidance.ts.
package auth

import "fmt"

// AuthGuidance provides help text for authentication setup.
type AuthGuidance struct {
	ProviderName string
	ProviderID   string
	Instructions []string
	DocURL       string
}

// GetAuthGuidance returns authentication guidance for a provider.
func GetAuthGuidance(providerID string) AuthGuidance {
	guidances := map[string]AuthGuidance{
		"anthropic": {
			ProviderName: "Anthropic",
			ProviderID:   "anthropic",
			Instructions: []string{
				"Set the ANTHROPIC_API_KEY environment variable",
				"Or run: miniClaudeCode auth login --provider anthropic",
			},
			DocURL: "https://docs.anthropic.com/en/docs/initial-setup",
		},
		"openai": {
			ProviderName: "OpenAI",
			ProviderID:   "openai",
			Instructions: []string{
				"Set the OPENAI_API_KEY environment variable",
				"Or run: miniClaudeCode auth login --provider openai",
			},
			DocURL: "https://platform.openai.com/api-keys",
		},
		"google": {
			ProviderName: "Google",
			ProviderID:   "google",
			Instructions: []string{
				"Set the GOOGLE_API_KEY environment variable",
				"Or set up Google Cloud credentials with Application Default Credentials",
			},
			DocURL: "https://ai.google.dev/gemini-api/docs/api-key",
		},
		"aws-bedrock": {
			ProviderName: "AWS Bedrock",
			ProviderID:   "aws-bedrock",
			Instructions: []string{
				"Set AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY environment variables",
				"Or configure AWS credentials via ~/.aws/credentials",
				"Or use AWS IAM role-based authentication",
			},
			DocURL: "https://docs.aws.amazon.com/bedrock/latest/userguide/security_iam.html",
		},
		"azure-openai": {
			ProviderName: "Azure OpenAI",
			ProviderID:   "azure-openai",
			Instructions: []string{
				"Set AZURE_OPENAI_API_KEY environment variable",
				"Set AZURE_OPENAI_ENDPOINT environment variable",
			},
			DocURL: "https://learn.microsoft.com/en-us/azure/ai-services/openai/",
		},
	}

	if guidance, ok := guidances[providerID]; ok {
		return guidance
	}

	return AuthGuidance{
		ProviderName: providerID,
		ProviderID:   providerID,
		Instructions: []string{
			fmt.Sprintf("Set the %s_API_KEY environment variable", toEnvName(providerID)),
			"Or check the provider documentation for authentication options",
		},
		DocURL: "",
	}
}

// FormatAuthHelp formats authentication guidance as a help message.
func FormatAuthHelp(guidance AuthGuidance) string {
	msg := fmt.Sprintf("Authentication required for %s\n", guidance.ProviderName)
	msg += "\nOptions:\n"
	for i, instr := range guidance.Instructions {
		msg += fmt.Sprintf("  %d. %s\n", i+1, instr)
	}
	if guidance.DocURL != "" {
		msg += fmt.Sprintf("\nDocumentation: %s\n", guidance.DocURL)
	}
	return msg
}

// FormatAuthError formats an authentication error message.
func FormatAuthError(providerID string, err error) string {
	guidance := GetAuthGuidance(providerID)
	msg := fmt.Sprintf("Authentication failed for %s: %v\n", guidance.ProviderName, err)
	msg += FormatAuthHelp(guidance)
	return msg
}

// toEnvName converts a provider ID to an environment variable name prefix.
func toEnvName(providerID string) string {
	// e.g. "aws-bedrock" -> "AWS_BEDROCK"
	result := make([]byte, 0, len(providerID)*2)
	for i, c := range providerID {
		if c == '-' || c == '_' || c == '.' {
			result = append(result, '_')
			continue
		}
		if c >= 'a' && c <= 'z' {
			result = append(result, byte(c-'a'+'A'))
		} else {
			result = append(result, byte(c))
		}
		// Insert underscore before uppercase transitions (for camelCase)
		_ = i
	}
	return string(result)
}