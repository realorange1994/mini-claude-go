package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// FeatureFlag is a boolean feature flag with a name.
type FeatureFlag struct {
	Name        string `json:"-"`
	Enabled     bool   `json:"enabled"`
	Description string `json:"description,omitempty"`
}

// FeatureFlagStore persists feature flags to a JSON file in .claude/.
type FeatureFlagStore struct {
	mu   sync.Mutex
	file string
	flags map[string]FeatureFlag
}

// NewFeatureFlagStore creates a store backed by .claude/feature_flags.json.
func NewFeatureFlagStore() *FeatureFlagStore {
	file := filepath.Join(".claude", "feature_flags.json")
	flags := make(map[string]FeatureFlag)

	data, err := os.ReadFile(file)
	if err == nil {
		_ = json.Unmarshal(data, &flags)
	}

	store := &FeatureFlagStore{file: file, flags: flags}
	// Set names for deserialized flags
	for name, f := range flags {
		f.Name = name
		store.flags[name] = f
	}
	return store
}

// Enabled checks if a feature flag is enabled.
func (s *FeatureFlagStore) Enabled(name string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if f, ok := s.flags[name]; ok {
		return f.Enabled
	}
	return false
}

// Enable sets a feature flag to enabled.
func (s *FeatureFlagStore) Enable(name string, description string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.flags[name] = FeatureFlag{
		Name:        name,
		Enabled:     true,
		Description: description,
	}
	s.save()
}

// Disable sets a feature flag to disabled.
func (s *FeatureFlagStore) Disable(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if f, ok := s.flags[name]; ok {
		f.Enabled = false
		s.flags[name] = f
		s.save()
	}
}

// List returns all registered flags.
func (s *FeatureFlagStore) List() []FeatureFlag {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]FeatureFlag, 0, len(s.flags))
	for _, f := range s.flags {
		result = append(result, f)
	}
	return result
}

func (s *FeatureFlagStore) save() {
	data, err := json.MarshalIndent(s.flags, "", "  ")
	if err != nil {
		return
	}
	os.WriteFile(s.file, data, 0o644)
}

// handleFeature handles the /feature slash command.
func handleFeature(agent *AgentLoop, args []string) {
	if agent == nil || agent.featureFlags == nil {
		fmt.Println("Feature flags not available.")
		return
	}
	store := agent.featureFlags

	if len(args) == 0 {
		// List all flags
		flags := store.List()
		if len(flags) == 0 {
			fmt.Println("No feature flags configured.")
			return
		}
		fmt.Println("Feature Flags:")
		for _, f := range flags {
			status := "OFF"
			if f.Enabled {
				status = "ON"
			}
			desc := ""
			if f.Description != "" {
				desc = fmt.Sprintf(" — %s", f.Description)
			}
			fmt.Printf("  %s: %s%s\n", f.Name, status, desc)
		}
		return
	}

	switch args[0] {
	case "list", "ls":
		flags := store.List()
		for _, f := range flags {
			status := "OFF"
			if f.Enabled {
				status = "ON"
			}
			fmt.Printf("  %s: %s\n", f.Name, status)
		}
	case "enable":
		if len(args) < 2 {
			fmt.Println("Usage: /feature enable <name> [description]")
			return
		}
		desc := ""
		if len(args) > 2 {
			desc = args[2]
		}
		store.Enable(args[1], desc)
		fmt.Printf("Feature %s enabled.\n", args[1])
	case "disable":
		if len(args) < 2 {
			fmt.Println("Usage: /feature disable <name>")
			return
		}
		store.Disable(args[1])
		fmt.Printf("Feature %s disabled.\n", args[1])
	default:
		fmt.Printf("Unknown feature command: %s\n", args[0])
		fmt.Println("Usage: /feature [list|enable <name>|disable <name>]")
	}
}