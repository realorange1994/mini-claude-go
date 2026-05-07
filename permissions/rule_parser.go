package permissions

import (
	"fmt"
	"strings"
)

// Legacy tool name aliases (renamed in our codebase vs upstream).
var legacyAliases = map[string]string{
	"Task":            "Agent",
	"KillShell":       "TaskStop",
	"AgentOutputTool": "TaskOutput",
	"BashOutputTool":  "TaskOutput",
}

// ParsedRule represents a parsed permission rule string like "Bash(git:*)".
type ParsedRule struct {
	ToolName string // e.g., "Bash", "Edit", "mcp__server1"
	Content  string // e.g., "git:*", ".env", "" (empty = tool-level rule)
	Behavior string // "allow", "deny", "ask"
}

// ParseRule parses a rule string into a ParsedRule.
// Format: "ToolName" or "ToolName(content)".
// Examples:
//   - "Bash" → tool-level rule (matches entire tool)
//   - "Bash(git:*)" → content-specific rule
//   - "Bash(*)" or "Bash()" → tool-level rule (wildcard collapses)
func ParseRule(ruleString string) (*ParsedRule, error) {
	if ruleString == "" {
		return nil, fmt.Errorf("empty rule string")
	}

	// Find the opening paren (if any)
	openIdx := strings.Index(ruleString, "(")
	if openIdx == -1 {
		// No content: tool-level rule
		toolName := resolveAlias(strings.TrimSpace(ruleString))
		return &ParsedRule{
			ToolName: toolName,
			Content:  "",
			Behavior: "", // caller must set
		}, nil
	}

	// Must have closing paren
	closeIdx := strings.LastIndex(ruleString, ")")
	if closeIdx == -1 || closeIdx < openIdx {
		return nil, fmt.Errorf("unmatched parentheses in rule %q", ruleString)
	}

	toolName := strings.TrimSpace(ruleString[:openIdx])
	content := ruleString[openIdx+1 : closeIdx]

	// Resolve aliases
	toolName = resolveAlias(toolName)

	// Empty content or wildcard-only content → tool-level rule
	if content == "" || content == "*" {
		return &ParsedRule{
			ToolName: toolName,
			Content:  "",
			Behavior: "", // caller must set
		}, nil
	}

	// Unescape \( and \) inside content
	content = unescapeParens(content)

	return &ParsedRule{
		ToolName: toolName,
		Content:  content,
		Behavior: "", // caller must set
	}, nil
}

// resolveAlias resolves legacy tool name aliases.
func resolveAlias(name string) string {
	if resolved, ok := legacyAliases[name]; ok {
		return resolved
	}
	return name
}

// unescapeParens converts \( to ( and \) to ) inside content strings.
func unescapeParens(s string) string {
	s = strings.ReplaceAll(s, "\\(", "(")
	s = strings.ReplaceAll(s, "\\)", ")")
	return s
}

// ParseRules parses a list of rule strings with the given behavior.
func ParseRules(rules []string, behavior string) ([]*ParsedRule, error) {
	result := make([]*ParsedRule, 0, len(rules))
	for _, ruleStr := range rules {
		rule, err := ParseRule(ruleStr)
		if err != nil {
			return nil, fmt.Errorf("parse rule %q: %w", ruleStr, err)
		}
		rule.Behavior = behavior
		result = append(result, rule)
	}
	return result, nil
}

// IsToolLevel returns true if this is a tool-level rule (no content filter).
func (r *ParsedRule) IsToolLevel() bool {
	return r.Content == ""
}

// ToolMatches checks if a tool name matches this rule.
// Handles MCP server-level matching:
//   - "mcp__server1" matches "mcp__server1__tool1"
//   - "mcp__server1__*" matches all tools from server1
func (r *ParsedRule) ToolMatches(toolName string) bool {
	if !r.IsToolLevel() {
		return false // content-specific rules don't match at tool level
	}

	if r.ToolName == toolName {
		return true
	}

	// MCP server-level matching
	return mcpServerMatches(r.ToolName, toolName)
}

// mcpServerMatches checks if a rule toolName matches a full tool name for MCP tools.
func mcpServerMatches(ruleName, toolName string) bool {
	// Both must be MCP-style names
	if !strings.HasPrefix(ruleName, "mcp__") || !strings.HasPrefix(toolName, "mcp__") {
		return false
	}

	// ruleName "mcp__server1" matches "mcp__server1__tool1"
	if strings.HasPrefix(toolName, ruleName+"__") {
		return true
	}

	// ruleName "mcp__server1__*" matches all tools from server1
	ruleName = strings.TrimSuffix(ruleName, "__*")
	if strings.HasPrefix(toolName, ruleName+"__") {
		return true
	}

	return false
}

// ContentMatches checks if a content string matches this rule's content pattern.
// Supports glob-style patterns: git:*, *.env, etc.
func (r *ParsedRule) ContentMatches(content string) bool {
	if r.IsToolLevel() {
		return false
	}
	return globMatch(r.Content, content)
}

// globMatch is a simple glob matcher supporting *, ?, and character classes.
// This is a simplified version matching upstream's pattern matching.
func globMatch(pattern, text string) bool {
	// Exact match
	if pattern == text {
		return true
	}

	// Prefix pattern: "git:*" matches "git status", "git commit", etc.
	// Format: "prefix*" matches anything starting with prefix
	if strings.HasSuffix(pattern, "*") && !strings.HasSuffix(pattern, "\\*") {
		prefix := strings.TrimSuffix(pattern, "*")
		prefix = strings.TrimSuffix(prefix, "\\") // unescape
		return strings.HasPrefix(text, prefix)
	}

	// Suffix pattern: "*.env" matches "foo.env", "bar.env"
	if strings.HasPrefix(pattern, "*") && len(pattern) > 1 {
		suffix := pattern[1:]
		suffix = strings.TrimPrefix(suffix, ".") // allow .env shorthand
		return strings.HasSuffix(text, suffix) || strings.HasSuffix(text, "."+suffix)
	}

	// Contains pattern: "*pattern*" matches if text contains pattern
	if strings.HasPrefix(pattern, "*") && strings.HasSuffix(pattern, "*") && len(pattern) > 2 {
		mid := pattern[1 : len(pattern)-1]
		mid = strings.TrimSuffix(mid, "\\*") // unescape
		return strings.Contains(text, mid)
	}

	// Wildcard (*) matches any sequence
	return wildcardMatch(pattern, text)
}

// wildcardMatch performs glob-style matching with *.
func wildcardMatch(pattern, text string) bool {
	// Simple recursive DP matching
	if pattern == "*" {
		return true
	}
	if pattern == "" {
		return text == ""
	}
	if text == "" {
		return pattern == "" || pattern == "*"
	}

	// Try matching first character
	if pattern[0] == '*' {
		// * matches anything, then try remaining pattern on remaining text
		return wildcardMatch(pattern[1:], text) || wildcardMatch(pattern, text[1:])
	}
	if pattern[0] == '?' || pattern[0] == text[0] {
		return wildcardMatch(pattern[1:], text[1:])
	}
	return false
}
