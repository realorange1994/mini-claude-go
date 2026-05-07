package permissions

import (
	"fmt"
	"strings"
)

// DANGEROUS_SHELL_PATTERNS lists shell commands that should not be auto-allowed
// in auto mode. Matches upstream's CROSS_PLATFORM_CODE_EXEC + DANGEROUS_BASH_PATTERNS.
var DANGEROUS_SHELL_PATTERNS = []string{
	// CROSS_PLATFORM_CODE_EXEC
	"python", "python3", "python2", "node", "deno", "tsx", "ruby", "perl",
	"php", "lua", "npx", "bunx", "npm run", "yarn run", "pnpm run",
	"bun run", "bash", "sh", "ssh",
	// BASH additions
	"zsh", "fish", "eval", "exec", "env", "xargs", "sudo",
}

// IsDangerousAllowRule checks if a parsed allow rule matches a dangerous shell pattern.
// Used by StripDangerousAllowRules to strip dangerous permissions in auto mode.
func IsDangerousAllowRule(rule *ParsedRule) bool {
	if rule.Behavior != "allow" {
		return false
	}

	// Only applies to Bash and Exec tools
	toolName := strings.ToLower(rule.ToolName)
	if toolName != "bash" && toolName != "exec" {
		return false
	}

	content := rule.Content
	if rule.IsToolLevel() {
		// Tool-level allow rule for Bash/Exec is always dangerous
		return true
	}

	// Check if content matches any dangerous pattern
	for _, pattern := range DANGEROUS_SHELL_PATTERNS {
		if matchesDangerousPattern(content, pattern) {
			return true
		}
	}

	return false
}

// matchesDangerousPattern checks if content matches a dangerous pattern.
// Pattern shapes: "cmd", "cmd:*", "cmd*", "cmd *", "cmd -*"
func matchesDangerousPattern(content, pattern string) bool {
	content = strings.ToLower(content)
	pattern = strings.ToLower(pattern)

	// Exact match: "python"
	if content == pattern {
		return true
	}

	// Prefix syntax: "python:*"
	prefixRule := pattern + ":*"
	if content == prefixRule {
		return true
	}

	// Trailing wildcard: "python*"
	wildcardRule := pattern + "*"
	if content == wildcardRule {
		return true
	}

	// Space wildcard: "python *"
	spaceRule := pattern + " *"
	if content == spaceRule {
		return true
	}

	// Flag wildcard: "python -*"
	flagRule := pattern + " -*"
	if content == flagRule {
		return true
	}

	// Content starts with pattern followed by space: "python script.py"
	if strings.HasPrefix(content, pattern+" ") {
		return true
	}

	// Content starts with pattern followed by colon: "python:script.py"
	if strings.HasPrefix(content, pattern+":") {
		return true
	}

	return false
}

// StripDangerousAllowRules removes dangerous allow rules from the store and
// returns them as a stash map for later restoration.
// The returned map is keyed by "source|allow" with the stripped rules.
func (s *RuleStore) StripDangerousAllowRules() map[string][]*ParsedRule {
	s.mu.Lock()
	defer s.mu.Unlock()

	stash := make(map[string][]*ParsedRule)
	for key, rules := range s.rules {
		parts := strings.SplitN(key, "|", 2)
		if len(parts) != 2 || parts[1] != "allow" {
			continue
		}

		var kept []*ParsedRule
		for _, rule := range rules {
			if IsDangerousAllowRule(rule) {
				stash[key] = append(stash[key], rule)
			} else {
				kept = append(kept, rule)
			}
		}
		s.rules[key] = kept
	}

	// Rebuild index
	s.indexByTool = make(map[string][]ruleEntry)
	for key, rules := range s.rules {
		behavior := ""
		if parts := strings.SplitN(key, "|", 2); len(parts) == 2 {
			behavior = parts[1]
		}
		for _, rule := range rules {
			if rule.IsToolLevel() {
				source := ""
				if parts := strings.SplitN(key, "|", 2); len(parts) == 2 {
					source = parts[0]
				}
				s.indexByTool[rule.ToolName] = append(s.indexByTool[rule.ToolName], ruleEntry{
					Rule:     rule,
					Source:   source,
					Behavior: behavior,
				})
			}
		}
	}

	return stash
}

// RestoreStrippedRules puts stripped rules back into the store.
func (s *RuleStore) RestoreStrippedRules(stash map[string][]*ParsedRule) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for key, rules := range stash {
		s.rules[key] = append(s.rules[key], rules...)
	}

	// Rebuild index
	s.indexByTool = make(map[string][]ruleEntry)
	for key, rules := range s.rules {
		behavior := ""
		source := ""
		if parts := strings.SplitN(key, "|", 2); len(parts) == 2 {
			behavior = parts[1]
			source = parts[0]
		}
		for _, rule := range rules {
			if rule.IsToolLevel() {
				s.indexByTool[rule.ToolName] = append(s.indexByTool[rule.ToolName], ruleEntry{
					Rule:     rule,
					Source:   source,
					Behavior: behavior,
				})
			}
		}
	}
}

// StrippedRulesSummary returns a human-readable summary of stripped rules.
func StrippedRulesSummary(stash map[string][]*ParsedRule) string {
	if len(stash) == 0 {
		return ""
	}

	var parts []string
	for key, rules := range stash {
		for _, rule := range rules {
			parts = append(parts, fmt.Sprintf("%s: %s", key, FormatRule(rule)))
		}
	}
	return "Stripped dangerous allow rules: " + strings.Join(parts, ", ")
}
