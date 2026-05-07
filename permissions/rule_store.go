package permissions

import (
	"fmt"
	"strings"
	"sync"
)

// Source priority order (lower index = higher priority).
// For merging: additive (later sources add to earlier, no override).
var ruleSources = []string{
	"userSettings",    // ~/.claude/settings.json
	"projectSettings", // .claude/settings.json
	"localSettings",   // .claude/settings.local.json
	"flagSettings",    // feature flag overrides
	"policySettings",  // managed/enterprise policy
	"cliArg",         // CLI --allowed-tools/--disallowed-tools
	"command",        // command-specific
	"session",        // session-specific
}

// RuleStore stores permission rules by source and behavior.
type RuleStore struct {
	mu sync.Mutex
	// rules: key = "source|behavior", value = sorted rule list
	rules map[string][]*ParsedRule
	// indexByTool: toolName → list of (rule, source, behavior) for fast lookup
	indexByTool map[string][]ruleEntry
}

type ruleEntry struct {
	Rule     *ParsedRule
	Source   string
	Behavior string
}

func NewRuleStore() *RuleStore {
	return &RuleStore{
		rules:       make(map[string][]*ParsedRule),
		indexByTool: make(map[string][]ruleEntry),
	}
}

// AddRules adds parsed rules from a source with a given behavior.
func (s *RuleStore) AddRules(source, behavior string, rules []*ParsedRule) {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := source + "|" + behavior
	s.rules[key] = append(s.rules[key], rules...)

	// Update index
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

// HasDenyRule returns true if there's a tool-level deny rule for this tool.
func (s *RuleStore) HasDenyRule(toolName string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	entries := s.indexByTool[toolName]
	for _, e := range entries {
		if e.Behavior == "deny" {
			return true
		}
	}
	return false
}

// HasAskRule returns true if there's a tool-level ask rule for this tool.
func (s *RuleStore) HasAskRule(toolName string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	entries := s.indexByTool[toolName]
	for _, e := range entries {
		if e.Behavior == "ask" {
			return true
		}
	}
	return false
}

// HasAllowRule returns true if there's a tool-level allow rule for this tool.
func (s *RuleStore) HasAllowRule(toolName string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	entries := s.indexByTool[toolName]
	for _, e := range entries {
		if e.Behavior == "allow" {
			return true
		}
	}
	return false
}

// FindContentRule finds a content-specific rule matching toolName + content + behavior.
// Returns the matched ParsedRule or nil.
func (s *RuleStore) FindContentRule(toolName, content, behavior string) *ParsedRule {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Search all rules for matching content
	for key, rules := range s.rules {
		parts := strings.SplitN(key, "|", 2)
		if len(parts) != 2 || parts[1] != behavior {
			continue
		}
		for _, rule := range rules {
			if rule.ToolName == toolName && !rule.IsToolLevel() && rule.ContentMatches(content) {
				return rule
			}
		}
	}
	return nil
}

// GetAllRules returns all rules with the given behavior from all sources.
func (s *RuleStore) GetAllRules(behavior string) []*ParsedRule {
	s.mu.Lock()
	defer s.mu.Unlock()

	var result []*ParsedRule
	for key, rules := range s.rules {
		parts := strings.SplitN(key, "|", 2)
		if len(parts) == 2 && parts[1] == behavior {
			result = append(result, rules...)
		}
	}
	return result
}

// GetRulesForTool returns all rules (from all sources) applicable to a tool.
func (s *RuleStore) GetRulesForTool(toolName string) []*ParsedRule {
	s.mu.Lock()
	defer s.mu.Unlock()

	var result []*ParsedRule
	for _, rules := range s.rules {
		for _, rule := range rules {
			if rule.IsToolLevel() && rule.ToolMatches(toolName) {
				result = append(result, rule)
			}
		}
	}
	return result
}

// MergeRuleStores creates a new RuleStore with all rules from all input stores combined.
// Additive merge: rules from all stores are kept (no override).
func MergeRuleStores(stores ...*RuleStore) *RuleStore {
	merged := NewRuleStore()
	for _, store := range stores {
		store.mu.Lock()
		for key, rules := range store.rules {
			merged.rules[key] = append(merged.rules[key], rules...)
		}
		for toolName, entries := range store.indexByTool {
			merged.indexByTool[toolName] = append(merged.indexByTool[toolName], entries...)
		}
		store.mu.Unlock()
	}
	return merged
}

// Clone creates a deep copy of the RuleStore.
func (s *RuleStore) Clone() *RuleStore {
	s.mu.Lock()
	defer s.mu.Unlock()

	clone := &RuleStore{
		rules:       make(map[string][]*ParsedRule),
		indexByTool: make(map[string][]ruleEntry),
	}
	for key, rules := range s.rules {
		cp := make([]*ParsedRule, len(rules))
		copy(cp, rules)
		clone.rules[key] = cp
	}
	for toolName, entries := range s.indexByTool {
		cp := make([]ruleEntry, len(entries))
		copy(cp, entries)
		clone.indexByTool[toolName] = cp
	}
	return clone
}

// SourceForRule returns the source string for a rule (used for debugging/logging).
func (s *RuleStore) SourceForRule(rule *ParsedRule) string {
	s.mu.Lock()
	defer s.mu.Unlock()

	for key, rules := range s.rules {
		for _, r := range rules {
			if r == rule {
				parts := strings.SplitN(key, "|", 2)
				if len(parts) == 2 {
					return parts[0]
				}
			}
		}
	}
	return ""
}

// FormatRule formats a ParsedRule back to its string representation.
func FormatRule(r *ParsedRule) string {
	if r.IsToolLevel() {
		return r.ToolName
	}
	return fmt.Sprintf("%s(%s)", r.ToolName, r.Content)
}
