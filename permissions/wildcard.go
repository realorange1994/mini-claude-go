package permissions

import (
	"regexp"
	"strings"
)

// ─── Wildcard Pattern Matching (MiMo-Code pattern) ──────────────────────────

// MatchWildcard checks if a string matches a wildcard pattern.
// Supports * (matches any sequence) and ? (matches single char).
// Special: trailing " *" makes the suffix optional (for bash commands).
func MatchWildcard(pattern, s string) bool {
	if pattern == "*" {
		return true
	}

	// Handle trailing " *" pattern (MiMo-Code bash command matching)
	// "git *" matches both "git" and "git checkout main"
	if strings.HasSuffix(pattern, " *") {
		prefix := strings.TrimSuffix(pattern, " *")
		if s == prefix {
			return true
		}
		// Continue with normal matching for the full pattern
	}

	// Convert wildcard pattern to regex
	regex := WildcardToRegex(pattern)
	matched, err := regexp.MatchString(regex, s)
	if err != nil {
		return false
	}
	return matched
}

// WildcardToRegex converts a wildcard pattern to a regex pattern.
func WildcardToRegex(pattern string) string {
	var sb strings.Builder
	sb.WriteString("^")
	for i := 0; i < len(pattern); i++ {
		ch := pattern[i]
		switch ch {
		case '*':
			sb.WriteString(".*")
		case '?':
			sb.WriteString(".")
		case '.', '+', '(', ')', '[', ']', '{', '}', '^', '$', '|', '\\':
			sb.WriteString("\\")
			sb.WriteByte(ch)
		default:
			sb.WriteByte(ch)
		}
	}
	sb.WriteString("$")
	return sb.String()
}

// MatchPermission checks if a tool+resource combination matches a permission rule.
// MiMo-Code pattern: both tool name and resource pattern must match.
func MatchPermission(toolName, resource, ruleTool, ruleResource string) bool {
	return MatchWildcard(ruleTool, toolName) && MatchWildcard(ruleResource, resource)
}

// EvaluatePermission evaluates permission rules using last-match-wins semantics.
// MiMo-Code pattern: findLast matching rule, default to "ask".
func EvaluatePermission(toolName, resource string, rules []PermissionRule) string {
	// Find last matching rule
	for i := len(rules) - 1; i >= 0; i-- {
		rule := rules[i]
		if MatchPermission(toolName, resource, rule.Tool, rule.Resource) {
			return rule.Action
		}
	}
	// Default: ask
	return "ask"
}

// PermissionRule represents a permission rule with wildcard matching.
type PermissionRule struct {
	Tool     string `json:"tool"`     // wildcard pattern for tool name
	Resource string `json:"resource"` // wildcard pattern for resource
	Action   string `json:"action"`   // "allow", "deny", "ask"
}

// IsToolDisabled checks if a tool is completely disabled by a deny rule.
// MiMo-Code pattern: tool is disabled if it matches a deny rule with pattern "*".
func IsToolDisabled(toolName string, rules []PermissionRule) bool {
	for _, rule := range rules {
		if rule.Action == "deny" && rule.Resource == "*" && MatchWildcard(rule.Tool, toolName) {
			return true
		}
	}
	return false
}

// EditToolGroup defines tools that share the "edit" permission group.
// MiMo-Code pattern: edit: "deny" covers all write tools.
var EditToolGroup = []string{
	"write_file", "edit_file", "multi_edit", "apply_patch",
}

// IsInEditGroup checks if a tool is in the edit permission group.
func IsInEditGroup(toolName string) bool {
	for _, t := range EditToolGroup {
		if t == toolName {
			return true
		}
	}
	return false
}

// FilterDisabledTools removes disabled tools from a tool list.
// MiMo-Code pattern: tools with deny + pattern "*" are completely removed.
func FilterDisabledTools(tools []string, rules []PermissionRule) []string {
	var result []string
	for _, tool := range tools {
		if !IsToolDisabled(tool, rules) {
			result = append(result, tool)
		}
	}
	return result
}
