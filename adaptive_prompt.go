package main

import (
	"strings"
)

// detectTaskMode infers the task type from the user's latest message.
// This matches openclacky's adaptive prompting pattern: detect the nature
// of the work and inject mode-specific instructions to improve output quality.
//
// Returns one of: "debug", "refactor", "create", "search", "general"
func detectTaskMode(message string) string {
	msg := strings.ToLower(message)

	// Debug: looking for bugs, errors, failures, stack traces
	for _, kw := range []string{
		"why does", "why is", "what's wrong", "what is wrong", "fix the bug",
		"fix this", "error:", "panic:", "failed to", "doesn't work",
		"not working", "crash", "stack trace", "exception", "segmentation fault",
		"debug", "troubleshoot", "diagnose",
	} {
		if strings.Contains(msg, kw) {
			return "debug"
		}
	}

	// Refactor: restructuring existing code
	for _, kw := range []string{
		"refactor", "restructure", "reorganize", "clean up", "simplify",
		"extract", "rename", "move the function", "extract method",
		"rename variable", "decompose", "abstraction", "design pattern",
	} {
		if strings.Contains(msg, kw) {
			return "refactor"
		}
	}

	// Create: building something new from scratch
	for _, kw := range []string{
		"create a", "write a", "build a", "implement a", "make a",
		"new file", "new function", "new struct", "new package",
		"generate a", "add a new",
	} {
		if strings.Contains(msg, kw) {
			return "create"
		}
	}

	// Search: looking for information in the codebase
	for _, kw := range []string{
		"find", "where is", "show me", "list all", "search",
		"what files", "which file", "does this repo", "does this code",
		"explain", "how does", "how do", "what does",
	} {
		if strings.Contains(msg, kw) {
			return "search"
		}
	}

	return "general"
}

// adaptiveTaskInstructions returns mode-specific instructions for the detected task type.
// These instructions are injected as a user message at the start of each turn,
// matching openclacky's layered prompt composition.
func adaptiveTaskInstructions(mode string) string {
	switch mode {
	case "debug":
		return "You are in debugging mode. Follow a systematic approach: 1) Reproduce the issue 2) Form a hypothesis about the root cause 3) Use targeted tool calls (Read, Grep) to gather evidence 4) Confirm or reject your hypothesis 5) Apply the minimal fix 6) Verify the fix works. Avoid making unrelated changes or jumping to conclusions."
	case "refactor":
		return "You are in refactoring mode. Preserve existing behavior — do not change functionality. Make small, incremental changes. After each change, verify the code still compiles and tests pass. Document any intentional API changes. Prefer composition over duplication."
	case "create":
		return "You are in creation mode. Before writing new code, outline the design and key types/functions. Prefer starting with a minimal working implementation, then iterate. Follow existing project conventions for naming, structure, and error handling. Add tests alongside implementation."
	case "search":
		return "You are in search/exploration mode. Use targeted searches (Grep, Glob, Read) to find relevant code. Summarize findings concisely with file:line references. Avoid reading entire files when a focused search would work. Build a mental model before answering."
	default:
		return ""
	}
}

// InjectAdaptiveInstructions detects the task mode from the latest user message
// and injects mode-specific instructions into the conversation context.
// Injected with SystemInjectedPrefix so cache break detection can skip it.
func (c *ConversationContext) InjectAdaptiveInstructions(latestUserMessage string) {
	mode := detectTaskMode(latestUserMessage)
	instructions := adaptiveTaskInstructions(mode)
	if instructions != "" {
		c.mu.Lock()
		defer c.mu.Unlock()
		c.entries = append(c.entries, conversationEntry{
			role:    "user",
			content: TextContent(SystemInjectedPrefix + "[Task mode: " + mode + "] " + instructions),
		})
	}
}
