package tools

import (
	"strings"
)

// HandoffClassification represents the result of classifying sub-agent output
// before returning it to the parent agent.
type HandoffClassification struct {
	Safe     bool   `json:"safe"`
	Reason   string `json:"reason,omitempty"`
	Filtered string `json:"filtered,omitempty"` // filtered version if not safe
}

// ClassifyHandoff reviews sub-agent output before returning to parent.
// This is a lightweight check — not a full permission classification.
// It detects potential secrets/credentials and excessively long outputs.
func ClassifyHandoff(output string) HandoffClassification {
	// Secret pattern detection — common API key/token prefixes
	secretPatterns := []string{
		"sk-ant-api03-",  // Anthropic API key prefix
		"sk-ant-tool03-", // Anthropic tool key prefix
		"sk-proj-",       // OpenAI project key
		"AKIA",           // AWS access key prefix
		"ghp_",           // GitHub personal access token
		"gho_",           // GitHub OAuth token
		"ghs_",           // GitHub server-to-server token
		"xoxb-",          // Slack bot token
		"xoxp-",          // Slack user token
		"-----BEGIN PRIVATE KEY-----",
		"-----BEGIN RSA PRIVATE KEY-----",
	}

	for _, pattern := range secretPatterns {
		if strings.Contains(output, pattern) {
			return HandoffClassification{
				Safe:     false,
				Reason:   "output contains potential secret/credential pattern: " + pattern,
				Filtered: "[REDACTED: output contained potential secrets]",
			}
		}
	}

	// Length check: if output > 50000 chars, suggest truncation
	if len(output) > 50000 {
		return HandoffClassification{
			Safe:   true,
			Reason: "output very long, consider truncation",
		}
	}

	return HandoffClassification{Safe: true}
}

// SanitizeHandoffOutput returns the output if safe, or the filtered version
// if the handoff classification detected issues.
func SanitizeHandoffOutput(output string) (string, bool) {
	class := ClassifyHandoff(output)
	if !class.Safe {
		return class.Filtered, false
	}
	return output, true
}
