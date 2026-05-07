package permissions

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// PermissionsConfig represents the permissions section of settings.json.
type PermissionsConfig struct {
	Allow []string `json:"allow"`
	Deny  []string `json:"deny"`
	Ask   []string `json:"ask"`
}

// LoadRulesFromConfig parses permission rules from a PermissionsConfig struct.
func LoadRulesFromConfig(cfg *PermissionsConfig, source string) (*RuleStore, error) {
	store := NewRuleStore()

	if cfg == nil {
		return store, nil
	}

	var lastErr error

	if len(cfg.Deny) > 0 {
		rules, err := ParseRules(cfg.Deny, "deny")
		if err != nil {
			lastErr = err
		} else {
			store.AddRules(source, "deny", rules)
		}
	}

	if len(cfg.Ask) > 0 {
		rules, err := ParseRules(cfg.Ask, "ask")
		if err != nil {
			lastErr = err
		} else {
			store.AddRules(source, "ask", rules)
		}
	}

	if len(cfg.Allow) > 0 {
		rules, err := ParseRules(cfg.Allow, "allow")
		if err != nil {
			lastErr = err
		} else {
			store.AddRules(source, "allow", rules)
		}
	}

	return store, lastErr
}

// LoadRulesFromFile loads permission rules from a settings.json file path.
func LoadRulesFromFile(path string) (*RuleStore, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", err
	}

	// Try to parse as full ClaudeSettings first
	var settings struct {
		Permissions PermissionsConfig `json:"permissions"`
	}
	if err := json.Unmarshal(data, &settings); err != nil {
		// Maybe it's just a permissions section alone
		var perms PermissionsConfig
		if err2 := json.Unmarshal(data, &perms); err2 != nil {
			return nil, "", fmt.Errorf("parse %q: %w (also tried permissions-only: %v)", path, err, err2)
		}
		settings.Permissions = perms
	}

	source := sourceFromPath(path)
	store, err := LoadRulesFromConfig(&settings.Permissions, source)
	return store, source, err
}

// sourceFromPath determines the source label from a settings file path.
// - ~/.claude/settings.json → "userSettings"
// - .claude/settings.json → "projectSettings"
// - .claude/settings.local.json → "localSettings"
// - ~/.claude/settings.local.json → "localSettings"
func sourceFromPath(path string) string {
	dir := filepath.Dir(path)
	file := filepath.Base(path)

	switch file {
	case "settings.json":
		if isHomeDir(dir) {
			return "userSettings"
		}
		return "projectSettings"
	case "settings.local.json":
		return "localSettings"
	}
	return "unknown"
}

// isHomeDir checks if a directory path is the user's home .claude directory.
func isHomeDir(dir string) bool {
	home := ""
	if p := os.Getenv("USERPROFILE"); p != "" {
		home = filepath.Join(p, ".claude")
	}
	if home == "" {
		if p := os.Getenv("HOME"); p != "" {
			home = filepath.Join(p, ".claude")
		}
	}
	if home != "" {
		// Normalize paths for comparison
		absDir, _ := filepath.Abs(dir)
		absHome, _ := filepath.Abs(home)
		return absDir == absHome
	}
	return false
}

// LoadRulesFromAllSources loads rules from all available settings sources.
// Returns a merged RuleStore with rules from all sources combined (additive).
// Sources are loaded in priority order (userSettings first, then projectSettings, etc.).
// Earlier sources take precedence for conflicting rules.
func LoadRulesFromAllSources(projectDir string) *RuleStore {
	var stores []*RuleStore

	// Helper to load from a path and add to stores
	load := func(path string) {
		store, _, err := LoadRulesFromFile(path)
		if err != nil || store == nil {
			return
		}
		stores = append(stores, store)
	}

	// Load from home directory first (lowest priority)
	if homeDir := homeClaudeDir(); homeDir != "" {
		load(filepath.Join(homeDir, "settings.local.json")) // localSettings (lowest priority)
		load(filepath.Join(homeDir, "settings.json"))       // userSettings
	}

	// Load from project directory (highest priority)
	load(filepath.Join(projectDir, ".claude", "settings.local.json")) // localSettings
	load(filepath.Join(projectDir, ".claude", "settings.json"))         // projectSettings

	// Merge all stores (additive, later sources append)
	if len(stores) == 0 {
		return NewRuleStore()
	}
	return MergeRuleStores(stores...)
}

// homeClaudeDir returns the path to ~/.claude, or empty string if undetermined.
func homeClaudeDir() string {
	if p := os.Getenv("USERPROFILE"); p != "" {
		return filepath.Join(p, ".claude")
	}
	if p := os.Getenv("HOME"); p != "" {
		return filepath.Join(p, ".claude")
	}
	return ""
}
