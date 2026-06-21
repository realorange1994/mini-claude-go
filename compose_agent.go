package main

import (
	"sync"
)

// ─── Compose Agent (MiMo-Code 5) ───────────────────────────────────────────
//
// A dedicated agent that orchestrates multi-step workflows using
// specialized compose skills (brainstorm, debug, TDD, ask).
//
// MiMo-Code source: agent/agent.ts (177-191)

// ComposeMode represents the compose agent's mode.
type ComposeMode string

const (
	ComposeModeBuild ComposeMode = "build"  // direct execution
	ComposeModePlan  ComposeMode = "plan"   // read-only reasoning
	ComposeModeCompose ComposeMode = "compose" // workflow orchestration
)

// ComposeAgent provides workflow orchestration capabilities.
type ComposeAgent struct {
	mu          sync.Mutex
	mode        ComposeMode
	skills      []string
	currentStep int
}

// NewComposeAgent creates a new compose agent.
func NewComposeAgent() *ComposeAgent {
	return &ComposeAgent{
		mode: ComposeModeCompose,
		skills: []string{
			"brainstorm",
			"debug",
			"tdd",
			"ask",
		},
	}
}

// GetMode returns the current compose mode.
func (a *ComposeAgent) GetMode() ComposeMode {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.mode
}

// SetMode sets the compose mode.
func (a *ComposeAgent) SetMode(mode ComposeMode) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.mode = mode
}

// GetSkills returns the available compose skills.
func (a *ComposeAgent) GetSkills() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.skills
}

// ShouldUseCompose returns true if the compose agent should handle the request.
func (a *ComposeAgent) ShouldUseCompose(message string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.mode == ComposeModeCompose
}

// BuildComposePrompt builds a prompt for the compose agent.
func BuildComposePrompt(message string) string {
	return "You are in compose mode. Use compose skills (brainstorm, debug, tdd, ask) to orchestrate this workflow.\n\n" + message
}
