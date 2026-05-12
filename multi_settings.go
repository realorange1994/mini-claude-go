package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// SettingsLevel represents the precedence level of a settings source.
// Higher levels override lower levels.
type SettingsLevel int

const (
	SettingsDefault  SettingsLevel = iota // built-in defaults
	SettingsGlobal                        // ~/.claude/settings.json
	SettingsProject                       // .claude/settings.json (project root)
	SettingsWorktree                      // .claude/settings.local.json (worktree-specific)
	SettingsSession                       // runtime session overrides (highest priority)
)

func (l SettingsLevel) String() string {
	switch l {
	case SettingsDefault:
		return "default"
	case SettingsGlobal:
		return "global"
	case SettingsProject:
		return "project"
	case SettingsWorktree:
		return "worktree"
	case SettingsSession:
		return "session"
	default:
		return "unknown"
	}
}

// SettingsFile represents a single settings file at a specific level.
type SettingsFile struct {
	Level    SettingsLevel      `json:"level"`
	Path     string             `json:"path"`
	Values   map[string]any     `json:"values"`
	Loaded   bool               `json:"loaded"`
}

// MultiSourceSettings implements the 5-level settings hierarchy.
type MultiSourceSettings struct {
	sources []SettingsFile // ordered by level (lowest to highest)
}

// NewMultiSourceSettings creates a settings manager with all 5 levels.
func NewMultiSourceSettings(projectDir string) *MultiSourceSettings {
	ms := &MultiSourceSettings{
		sources: make([]SettingsFile, 5),
	}

	// Level 0: defaults (no file, applied programmatically)
	ms.sources[0] = SettingsFile{Level: SettingsDefault, Path: "", Values: make(map[string]any)}

	// Level 1: global (~/.claude/settings.json)
	if home := homeClaudeDir(); home != "" {
		ms.sources[1] = SettingsFile{Level: SettingsGlobal, Path: filepath.Join(home, "settings.json")}
	}

	// Level 2: project (.claude/settings.json)
	if projectDir != "" {
		ms.sources[2] = SettingsFile{Level: SettingsProject, Path: filepath.Join(projectDir, ".claude", "settings.json")}
	}

	// Level 3: worktree (.claude/settings.local.json)
	if projectDir != "" {
		ms.sources[3] = SettingsFile{Level: SettingsWorktree, Path: filepath.Join(projectDir, ".claude", "settings.local.json")}
	}

	// Level 4: session (runtime only, no file)
	ms.sources[4] = SettingsFile{Level: SettingsSession, Path: "", Values: make(map[string]any)}

	// Load all files
	for i := range ms.sources {
		if ms.sources[i].Path != "" {
			ms.loadFile(i)
		}
	}

	return ms
}

// loadFile reads a settings file into the source at index i.
func (ms *MultiSourceSettings) loadFile(i int) {
	src := &ms.sources[i]
	data, err := os.ReadFile(src.Path)
	if err != nil {
		src.Loaded = false
		return
	}
	var values map[string]any
	if err := json.Unmarshal(data, &values); err != nil {
		src.Loaded = false
		return
	}
	src.Values = values
	src.Loaded = true
}

// Get returns the effective value for a key, respecting precedence.
// Higher levels override lower levels.
func (ms *MultiSourceSettings) Get(key string) (any, bool) {
	// Search from highest to lowest priority
	for i := len(ms.sources) - 1; i >= 0; i-- {
		if v, ok := ms.sources[i].Values[key]; ok {
			return v, true
		}
	}
	return nil, false
}

// GetString returns a string value for a key.
func (ms *MultiSourceSettings) GetString(key string) string {
	if v, ok := ms.Get(key); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// GetInt returns an int value for a key.
func (ms *MultiSourceSettings) GetInt(key string) int {
	if v, ok := ms.Get(key); ok {
		switch n := v.(type) {
		case float64:
			return int(n)
		case int:
			return n
		}
	}
	return 0
}

// GetBool returns a bool value for a key.
func (ms *MultiSourceSettings) GetBool(key string) bool {
	if v, ok := ms.Get(key); ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}

// SetSession sets a session-level override (highest priority).
func (ms *MultiSourceSettings) SetSession(key string, value any) {
	ms.sources[SettingsSession].Values[key] = value
}

// Merged returns the fully merged settings map (all levels applied).
func (ms *MultiSourceSettings) Merged() map[string]any {
	result := make(map[string]any)
	for _, src := range ms.sources {
		for k, v := range src.Values {
			result[k] = v
		}
	}
	return result
}

// SourceOf returns which level provides the effective value for a key.
func (ms *MultiSourceSettings) SourceOf(key string) SettingsLevel {
	for i := len(ms.sources) - 1; i >= 0; i-- {
		if _, ok := ms.sources[i].Values[key]; ok {
			return ms.sources[i].Level
		}
	}
	return SettingsDefault
}

// Sources returns all loaded settings sources.
func (ms *MultiSourceSettings) Sources() []SettingsFile {
	return ms.sources
}

// handleSettings handles the /settings slash command.
func handleSettings(args []string) {
	projectDir, _ := os.Getwd()
	ms := NewMultiSourceSettings(projectDir)

	if len(args) == 0 {
		// Show merged settings with source info
		merged := ms.Merged()
		if len(merged) == 0 {
			fmt.Println("No settings configured.")
			return
		}
		fmt.Println("Effective Settings:")
		for k, v := range merged {
			source := ms.SourceOf(k)
			fmt.Printf("  %s = %v (from %s)\n", k, v, source)
		}
		return
	}

	switch args[0] {
	case "sources":
		fmt.Println("Settings Sources:")
		for _, src := range ms.Sources() {
			status := "not loaded"
			if src.Loaded {
				status = "loaded"
			}
			if src.Path == "" {
				status = "built-in"
			}
			count := len(src.Values)
			fmt.Printf("  [%s] %s — %d keys (%s)\n", src.Level, src.Path, count, status)
		}
	case "get":
		if len(args) < 2 {
			fmt.Println("Usage: /settings get <key>")
			return
		}
		if v, ok := ms.Get(args[1]); ok {
			source := ms.SourceOf(args[1])
			fmt.Printf("%s = %v (from %s)\n", args[1], v, source)
		} else {
			fmt.Printf("%s: not set\n", args[1])
		}
	case "set":
		if len(args) < 3 {
			fmt.Println("Usage: /settings set <key> <value>")
			return
		}
		ms.SetSession(args[1], args[2])
		fmt.Printf("Set %s = %v (session-level override)\n", args[1], args[2])
	default:
		fmt.Printf("Unknown settings command: %s\n", args[0])
		fmt.Println("Usage: /settings [sources|get <key>|set <key> <value>]")
	}
}