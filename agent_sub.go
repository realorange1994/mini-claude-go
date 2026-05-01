package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"miniclaudecode-go/tools"
	"miniclaudecode-go/transcript"
)

// subAgentResult holds the output of a sub-agent execution.
type subAgentResult struct {
	result     string
	errText    string
	toolsUsed  int
	durationMs int64
}

// SpawnSubAgent creates and runs a child AgentLoop with isolated context.
// This is the callback wired to AgentTool.SpawnFunc.
func (a *AgentLoop) SpawnSubAgent(
	prompt string,
	subagentType string,
	model string,
	runInBackground bool,
	allowedTools []string,
	disallowedTools []string,
	inheritContext bool,
	parentMessages []map[string]any,
) (result string, errText string, toolsUsed int, durationMs int64) {
	start := time.Now()

	a.activeSubAgents.Add(1)
	defer a.activeSubAgents.Add(-1)

	// Build child config (inherits parent settings, overrides child-specific ones)
	childCfg := a.buildSubAgentConfig(model)

	// Build child registry (filter tools)
	childRegistry := a.buildSubAgentRegistry(allowedTools, disallowedTools)

	// Create the child AgentLoop directly (reuses parent's client, API key, etc.)
	childLoop, err := a.createChildAgentLoop(childCfg, childRegistry)
	if err != nil {
		return "", fmt.Sprintf("failed to create sub-agent: %v", err), 0, time.Since(start).Milliseconds()
	}
	defer childLoop.Close()

	// Build child system prompt
	childSysPrompt := buildSubAgentSystemPrompt(childRegistry, childCfg, subagentType)
	childLoop.context.SetSystemPrompt(childSysPrompt)

	// Run the child loop
	result = childLoop.Run(prompt)

	// Calculate usage (turns consumed as proxy for tool usage)
	turnsUsed := int(childLoop.budget.consumed.Load())
	durationMs = time.Since(start).Milliseconds()

	return result, "", turnsUsed, durationMs
}

// buildSubAgentConfig creates a Config for the child agent by copying the parent's config
// and overriding child-specific fields.
func (a *AgentLoop) buildSubAgentConfig(model string) Config {
	childCfg := a.config // copy by value

	// Override model if specified
	if model != "" {
		childCfg.Model = model
	}

	// Limit child agent turns
	if childCfg.SubAgentMaxTurns > 0 {
		childCfg.MaxTurns = childCfg.SubAgentMaxTurns
	} else {
		childCfg.MaxTurns = 50 // sensible default for sub-agents
	}

	// Disable session memory for sub-agents (they don't need to persist notes)
	childCfg.SessionMemory = nil

	// Disable reactive compaction (sub-agents have short lifetimes)
	childCfg.ReactiveCompactEnabled = false

	// Clear cached prompt so it rebuilds with child's tool set
	childCfg.cachedPrompt = nil

	return childCfg
}

// buildSubAgentRegistry creates a new Registry for the child agent containing
// only the tools the child is allowed to use.
func (a *AgentLoop) buildSubAgentRegistry(allowedTools, disallowedTools []string) *tools.Registry {
	childRegistry := tools.NewRegistry()

	// Build disallowed set (always includes "agent")
	disallowed := make(map[string]bool)
	for _, t := range disallowedTools {
		disallowed[t] = true
	}

	// Build allowed set
	hasAllowed := len(allowedTools) > 0
	allowed := make(map[string]bool)
	for _, t := range allowedTools {
		allowed[t] = true
	}

	// Copy tools from parent registry
	for _, tool := range a.registry.AllTools() {
		name := tool.Name()

		// Skip disallowed tools
		if disallowed[name] {
			continue
		}

		// Skip tools not in allowed set (if explicit whitelist is provided)
		if hasAllowed && !allowed[name] {
			continue
		}

		childRegistry.Register(tool)
	}

	return childRegistry
}

// buildSubAgentSystemPrompt creates a system prompt for the child agent.
func buildSubAgentSystemPrompt(registry *tools.Registry, cfg Config, subagentType string) string {
	// Build a tailored system prompt for the sub-agent
	toolList := buildToolList(registry)

	wd, _ := os.Getwd()
	envInfo := fmt.Sprintf("%s / %s / %s", os.Getenv("GOOS"), "go", os.Getenv("GOARCH"))
	if envInfo == " / go / " {
		envInfo = fmt.Sprintf("%s / go / amd64", os.Getenv("GOOS"))
	}

	// Build a simpler system prompt for sub-agents
	var sb strings.Builder
	sb.WriteString("You are a specialized AI assistant running as a sub-agent.\n\n")
	sb.WriteString(fmt.Sprintf("## Environment\n- Working directory: %s\n- OS: %s\n- Model: %s\n\n", wd, envInfo, cfg.Model))

	// Mode description
	sb.WriteString(fmt.Sprintf("## Permission Mode: %s\n\n", strings.ToUpper(string(cfg.PermissionMode))))

	// Tool list
	sb.WriteString("## Available Tools\n\n")
	sb.WriteString("You have access to the following tools. Use them to accomplish your task.\n\n")
	sb.WriteString(toolList)
	sb.WriteString("\n")

	// Instructions
	sb.WriteString("## Instructions\n\n")
	sb.WriteString("1. Complete the task as described in the user message.\n")
	sb.WriteString("2. You have your own isolated context - start fresh for this task.\n")
	sb.WriteString("3. Use tools as needed. Be thorough but efficient.\n")
	sb.WriteString("4. When done, provide your final answer as text.\n")

	if subagentType != "" {
		sb.WriteString(fmt.Sprintf("\n## Specialization: %s\n\n", subagentType))
		sb.WriteString(fmt.Sprintf("You are specialized in: %s. Apply this expertise to the task.\n", subagentType))
	}

	return sb.String()
}

// createChildAgentLoop creates a new AgentLoop for a child, reusing the parent's
// HTTP client and API configuration.
func (a *AgentLoop) createChildAgentLoop(cfg Config, registry *tools.Registry) (*AgentLoop, error) {
	maxTurns := cfg.MaxTurns
	if maxTurns <= 0 {
		maxTurns = 50
	}

	// Create child context
	ctx := NewConversationContext(cfg)

	// Create a sub-agent transcript (separate from parent)
	sessionID := time.Now().Format("20060102-150405-sub")
	transcriptDir := filepath.Join(".claude", "transcripts", "sub-agents")
	tw := transcript.NewWriter(sessionID, filepath.Join(transcriptDir, sessionID+".jsonl"))
	_ = tw.WriteSystem(fmt.Sprintf("sub-agent: model=%s, mode=%s", cfg.Model, cfg.PermissionMode))

	child := &AgentLoop{
		config:       cfg,
		registry:     registry,
		gate:         NewPermissionGate(&cfg),
		context:      ctx,
		client:       a.client, // reuse parent's HTTP client
		snapshots:    cfg.FileHistory,
		transcript:   tw,
		skillTracker: nil, // sub-agents don't track skills
		compactor:    NewCompactor(),
		useStream:    a.useStream,
		maxToolChars: a.maxToolChars,
		toolTimeout:  a.toolTimeout,
		maxTurns:     maxTurns,
		budget:       NewIterationBudget(maxTurns),
	}
	child.gate = NewPermissionGate(&child.config)

	return child, nil
}

