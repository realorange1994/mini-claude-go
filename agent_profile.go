package main

import (
	"os"
	"path/filepath"
	"strings"
)

// AgentProfile holds the loaded personality and behavioral configuration
// from external files (SOUL.md, USER.md, base_prompt.md).
// These files allow users to customize agent behavior without modifying source code.
type AgentProfile struct {
	Soul        string // agent personality (from SOUL.md)
	UserProfile string // user preferences (from USER.md)
	BasePrompt  string // universal behavioral rules (from base_prompt.md)
}

// Default built-in values (matching openclacky's default_agents/ directory).
// Users can override by placing files in {homeDir}/.claude/.
const (
	defaultSoul = `You are calm, precise, and helpful. You communicate clearly and concisely.
You are honest about uncertainty and ask for clarification when needed.
You take initiative but respect the user's preferences and decisions.`

	defaultBasePrompt = `Always use tools to create, modify, or inspect files — do not just return content in your response.
Keep responses short and concise. Do not narrate internal machinery or justify tool choices.
When the task is done, report the result. Do not append "Is there anything else?"`

	defaultUser = `(No user profile configured yet. To personalize, create ~/.claude/USER.md with your preferences, working style, or any context you want the agent to know.)`
)

// LoadAgentProfile loads SOUL.md, USER.md, and base_prompt.md from the
// user's Claude config directory (~/.claude/). If a file does not exist,
// the built-in default is used instead.
//
// Lookup order for each file:
//   1. {homeDir}/.claude/{filename}  (user override)
//   2. Built-in default              (fallback)
func LoadAgentProfile() *AgentProfile {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		homeDir = ""
	}

	return &AgentProfile{
		Soul:        loadProfileFile(homeDir, "SOUL.md", defaultSoul),
		UserProfile: loadProfileFile(homeDir, "USER.md", defaultUser),
		BasePrompt:  loadProfileFile(homeDir, "base_prompt.md", defaultBasePrompt),
	}
}

// loadProfileFile reads a profile file from {homeDir}/.claude/{filename}.
// Returns the file content (trimmed) or the default value if not found.
func loadProfileFile(homeDir, filename, defaultVal string) string {
	if homeDir == "" {
		return defaultVal
	}
	path := filepath.Join(homeDir, ".claude", filename)
	data, err := os.ReadFile(path)
	if err != nil {
		return defaultVal
	}
	content := strings.TrimSpace(string(data))
	if content == "" {
		return defaultVal
	}
	return content
}

// HasCustomSoul returns true if the user has configured a custom SOUL.md
// (i.e., the soul content differs from the built-in default).
func (p *AgentProfile) HasCustomSoul() bool {
	return p.Soul != defaultSoul
}

// FormatForPrompt returns the profile sections formatted for injection
// into the system prompt. Only includes non-empty sections.
func (p *AgentProfile) FormatForPrompt() string {
	var parts []string

	if p.Soul != "" {
		parts = append(parts, "## Agent Soul (from SOUL.md)\n\n"+p.Soul)
	}
	if p.UserProfile != "" {
		parts = append(parts, "## User Profile (from USER.md)\n\n"+p.UserProfile)
	}
	if p.BasePrompt != "" {
		parts = append(parts, "## Base Rules (from base_prompt.md)\n\n"+p.BasePrompt)
	}

	return strings.Join(parts, "\n\n")
}
