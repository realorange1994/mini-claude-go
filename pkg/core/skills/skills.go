// Package skills provides a skill registry aligned to pi's skills.ts.
// Skills are prompt templates that inject domain-specific instructions
// into the system prompt when activated.
package skills

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// Skill represents a named prompt template that can be activated.
type Skill struct {
	// Name is the unique identifier, e.g. "commit", "review-pr"
	Name string `json:"name"`
	// Description is a one-line summary shown in UI
	Description string `json:"description"`
	// Prompt is the full prompt text injected into system prompt
	Prompt string `json:"prompt"`
	// Type categorizes the skill: "builtin", "extension", "user"
	Type string `json:"type"`
	// SourceFile is where the skill was loaded from (empty for builtins)
	SourceFile string `json:"sourceFile,omitempty"`
}

// Registry manages available skills.
type Registry struct {
	mu     sync.RWMutex
	skills map[string]Skill
}

// NewRegistry creates a skill registry with built-in skills.
func NewRegistry() *Registry {
	r := &Registry{
		skills: make(map[string]Skill),
	}
	for _, s := range builtinSkills() {
		r.skills[s.Name] = s
	}
	return r
}

// Register adds or replaces a skill.
func (r *Registry) Register(s Skill) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.skills[s.Name] = s
}

// Unregister removes a skill by name.
func (r *Registry) Unregister(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.skills, name)
}

// Get returns a skill by name.
func (r *Registry) Get(name string) (Skill, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.skills[name]
	return s, ok
}

// All returns all skills sorted by name.
func (r *Registry) All() []Skill {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Skill, 0, len(r.skills))
	for _, s := range r.skills {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	return out
}

// ActivePrompt returns the concatenated prompt text for the given skill names.
// Skills that don't exist are silently skipped.
func (r *Registry) ActivePrompt(names []string) string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var parts []string
	for _, name := range names {
		if s, ok := r.skills[name]; ok {
			parts = append(parts, s.Prompt)
		}
	}
	return strings.Join(parts, "\n\n")
}

// LoadFromDir loads user-defined skills from a directory.
// Each skill is a JSON file: { "name": "...", "description": "...", "prompt": "..." }
func (r *Registry) LoadFromDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read skills dir: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var s Skill
		if err := json.Unmarshal(data, &s); err != nil {
			continue
		}
		if s.Name == "" || s.Prompt == "" {
			continue
		}
		s.Type = "user"
		s.SourceFile = path
		r.Register(s)
	}
	return nil
}

// builtinSkills returns the default built-in skills.
func builtinSkills() []Skill {
	return []Skill{
		{
			Name:        "commit",
			Description: "Generate a git commit message from staged changes",
			Type:        "builtin",
			Prompt: `When asked to commit, follow these steps:
1. Run "git status" and "git diff --cached" to understand the changes
2. Draft a concise commit message that explains the "why" not the "what"
3. Use the Conventional Commits format (feat:, fix:, refactor:, etc.)
4. Run "git commit" with the message
5. Do NOT push unless explicitly asked`,
		},
		{
			Name:        "review-pr",
			Description: "Review a pull request and provide feedback",
			Type:        "builtin",
			Prompt: `When asked to review a PR, follow these steps:
1. Fetch the PR diff using "gh pr diff <number>"
2. Analyze the changes for correctness, style, and potential issues
3. Provide actionable feedback organized by file
4. Highlight security concerns, performance issues, and missing tests
5. Summarize overall assessment`,
		},
		{
			Name:        "test",
			Description: "Write tests for the specified code",
			Type:        "builtin",
			Prompt: `When asked to write tests, follow these steps:
1. Read the source code to understand what needs testing
2. Write tests using the project's existing test framework
3. Cover edge cases and error paths, not just happy paths
4. Follow existing test patterns in the project
5. Run the tests to verify they pass`,
		},
		{
			Name:        "debug",
			Description: "Debug an issue by systematic investigation",
			Type:        "builtin",
			Prompt: `When asked to debug an issue, follow these steps:
1. Reproduce the issue first if possible
2. Read the relevant source code and trace the execution path
3. Check logs, error messages, and recent changes
4. Form a hypothesis and test it
5. Propose a fix and verify it resolves the issue`,
		},
		{
			Name:        "refactor",
			Description: "Refactor code while preserving behavior",
			Type:        "builtin",
			Prompt: `When asked to refactor, follow these principles:
1. Do not change behavior — only structure
2. Make small, incremental changes
3. Run tests after each change
4. Prefer composition over inheritance
5. Remove dead code rather than commenting it out`,
		},
		{
			Name:        "explain",
			Description: "Explain code or concepts in detail",
			Type:        "builtin",
			Prompt: `When asked to explain code or concepts:
1. Start with a high-level summary
2. Break down into logical sections
3. Use analogies to familiar concepts when helpful
4. Include concrete examples
5. Point out non-obvious details and gotchas`,
		},
	}
}
