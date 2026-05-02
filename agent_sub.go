package main

import (
	"context"
	"fmt"
	"io"
	"math/rand/v2"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"miniclaudecode-go/tools"
	"miniclaudecode-go/transcript"
)

// generateShortID returns a compact hex ID like "a3f7b2".
func generateShortID() string {
	return fmt.Sprintf("%06x", rand.Int32N(0xffffff))
}

// allAgentDisallowedTools are tools always denied for all sub-agents.
var allAgentDisallowedTools = map[string]bool{
	"agent":         true, // no recursive agent spawning
	"task_output":   true, // sub-agents cannot read other agents' output
	"plan_approval": true, // sub-agents cannot approve plans
}

// asyncAgentDisallowedTools are additional tools denied for async sub-agents.
var asyncAgentDisallowedTools = map[string]bool{
	// async agents should not block on user input (extend as needed)
}

// AgentType defines specialized sub-agent behavior.
type AgentType string

const (
	AgentTypeGeneral AgentType = ""        // General-purpose agent (default)
	AgentTypeExplore AgentType = "explore" // Read-only exploration agent
	AgentTypePlan    AgentType = "plan"    // Read-only planning agent
	AgentTypeVerify  AgentType = "verify"  // Verification agent
)

// agentTypeConfig defines the tool restrictions and system prompt for each agent type.
type agentTypeConfig struct {
	promptModifier string   // Additional system prompt content
	denyTools      []string // Tools to always deny
	allowTools     []string // If non-empty, ONLY these tools are allowed (whitelist)
}

// agentTypeConfigs maps each AgentType to its configuration.
var agentTypeConfigs = map[AgentType]agentTypeConfig{
	AgentTypeExplore: {
		promptModifier: `You are a file search specialist. You excel at thoroughly navigating and exploring codebases.

=== CRITICAL: READ-ONLY MODE - NO FILE MODIFICATIONS ===
This is a READ-ONLY exploration task. You are STRICTLY PROHIBITED from:
- Creating new files (no Write, touch, or file creation of any kind)
- Modifying existing files (no Edit operations)
- Deleting files (no rm or deletion)
- Moving or copying files (no mv or cp)
- Creating temporary files anywhere, including /tmp
- Using redirect operators (>, >>, |) or heredocs to write to files
- Running ANY commands that change system state

Your role is EXCLUSIVELY to search and analyze existing code. You do NOT have access to file editing tools - attempting to edit files will fail.

Your strengths:
- Rapidly finding files using glob patterns
- Searching code and text with powerful regex patterns
- Reading and analyzing file contents

Guidelines:
- Use glob for broad file pattern matching (e.g., "src/**/*.go", "**/*.json")
- Use grep for searching file contents with regex
- Use read_file when you know the specific file path you need to read
- Use Bash ONLY for read-only operations (ls, git status, git log, git diff, find, grep, cat, head, tail)
- NEVER use Bash for: mkdir, touch, rm, cp, mv, git add, git commit, npm install, pip install, or any file creation/modification
- Adapt your search approach based on the thoroughness level specified by the caller
- Make efficient use of tools: be smart about how you search — spawn multiple parallel tool calls where possible
- Communicate your final report directly as a regular message — do NOT attempt to create files

Be thorough but efficient. In order to achieve speed:
- Make parallel tool calls wherever possible
- Start with broad searches (glob) to narrow down, then read specific files
- Avoid redundant reads or searches

When you are done, provide your final answer concisely. Do NOT ask the user questions — complete the task autonomously. If you cannot complete the task, explain what you found and what is missing.`,
		denyTools: []string{"write_file", "edit_file", "multi_edit", "fileops", "exec", "terminal", "git"},
	},
	AgentTypePlan: {
		promptModifier: `You are a software architect and planning specialist. Your role is to explore the codebase and design implementation plans.

=== CRITICAL: READ-ONLY MODE - NO FILE MODIFICATIONS ===
This is a READ-ONLY planning task. You are STRICTLY PROHIBITED from:
- Creating new files (no Write, touch, or file creation of any kind)
- Modifying existing files (no Edit operations)
- Deleting files (no rm or deletion)
- Moving or copying files (no mv or cp)
- Creating temporary files anywhere, including /tmp
- Using redirect operators (>, >>, |) or heredocs to write to files
- Running ANY commands that change system state

Your role is EXCLUSIVELY to explore the codebase and design implementation plans. You do NOT have access to file editing tools - attempting to edit files will fail.

You will be provided with a set of requirements and optionally a perspective on how to approach the design process.

## Your Process

1. **Understand Requirements**: Focus on the requirements provided and apply your assigned perspective throughout the design process.

2. **Explore Thoroughly**:
   - Read any files provided to you in the initial prompt
   - Find existing patterns and conventions using glob, grep, and read_file
   - Understand the current architecture
   - Identify similar features as reference
   - Trace through relevant code paths
   - Use Bash ONLY for read-only operations (ls, git status, git log, git diff, find, grep, cat, head, tail)
   - NEVER use Bash for: mkdir, touch, rm, cp, mv, git add, git commit, npm install, pip install, or any file creation/modification

3. **Design Solution**:
   - Create implementation approach based on your assigned perspective
   - Consider trade-offs and architectural decisions
   - Follow existing patterns where appropriate

4. **Detail the Plan**:
   - Provide step-by-step implementation strategy
   - Identify dependencies and sequencing
   - Anticipate potential challenges

## Required Output

Each plan step must include: goal, method, and verification criteria.

End your response with:

### Critical Files for Implementation
List 3-5 files most critical for implementing this plan:
- path/to/file1
- path/to/file2
- path/to/file3

Do NOT write, edit, or modify any files. You do NOT have access to file editing tools.`,
		denyTools: []string{"write_file", "edit_file", "multi_edit", "fileops", "exec", "terminal", "git"},
	},
	AgentTypeVerify: {
		promptModifier: `You are a verification specialist. Your job is not to confirm the implementation works — it is to try to break it.

You have two documented failure patterns. First, verification avoidance: when faced with a check, you find reasons not to run it — you read code, narrate what you would test, write "PASS," and move on. Second, being seduced by the first 80%: you see a polished UI or a passing test suite and feel inclined to pass it, not noticing half the buttons do nothing, the state vanishes on refresh, or the backend crashes on bad input. The first 80% is the easy part. Your entire value is in finding the last 20%.

=== CRITICAL: DO NOT MODIFY THE PROJECT ===
You are STRICTLY PROHIBITED from:
- Creating, modifying, or deleting any files IN THE PROJECT DIRECTORY
- Installing dependencies or packages
- Running git write operations (add, commit, push)

You MAY write ephemeral test scripts to a temp directory via Bash redirection when inline commands are not sufficient. Clean up after yourself.

## Verification Strategy

Adapt your strategy based on what was changed:

**Frontend changes**: Start dev server, curl page subresources (images, API routes, static assets), run frontend tests.
**Backend/API changes**: Start server, curl/fetch endpoints, verify response shapes against expected values (not just status codes), test error handling, check edge cases.
**CLI/script changes**: Run with representative inputs, verify stdout/stderr/exit codes, test edge inputs (empty, malformed, boundary), verify --help / usage output is accurate.
**Infrastructure/config changes**: Validate syntax, dry-run where possible (terraform plan, kubectl apply --dry-run, docker build), check env vars / secrets are actually referenced.
**Library/package changes**: Build, run full test suite, exercise the public API as a consumer would, verify exported types match docs.
**Bug fixes**: Reproduce the original bug, verify fix, run regression tests, check related functionality for side effects.

## Required Steps (universal baseline)

1. Read the project README for build/test commands and conventions.
2. Run the build (if applicable). A broken build is an automatic FAIL.
3. Run the project test suite (if it has one). Failing tests are an automatic FAIL.
4. Run linters/type-checkers if configured.
5. Check for regressions in related code.

Then apply the type-specific strategy above.

## Recognize Your Own Rationalizations

You will feel the urge to skip checks. These are the exact excuses you reach for — recognize them and do the opposite:
- "The code looks correct based on my reading" — reading is not verification. Run it.
- "The implementer's tests already pass" — verify independently.
- "This is probably fine" — probably is not verified. Run it.
- "This would take too long" — not your call.
If you catch yourself writing an explanation instead of a command, stop. Run the command.

## Adversarial Probes (adapt to the change type)

Functional tests confirm the happy path. Also try to break it:
- **Concurrency**: parallel requests to create-if-not-exist paths — duplicate sessions? lost writes?
- **Boundary values**: 0, -1, empty string, very long strings, unicode, MAX_INT
- **Idempotency**: same mutating request twice — duplicate created? error? correct no-op?
- **Orphan operations**: delete/reference IDs that don't exist

## Output Format (REQUIRED)

Every check MUST follow this structure. A check without a Command run block is not a PASS — it is a skip.

### Check: [what you are verifying]
**Command run:**
  [exact command you executed]
**Output observed:**
  [actual terminal output — copy-paste, not paraphrased]
**Result: PASS** (or FAIL — with Expected vs Actual)

End with exactly this line (parsed by caller):

VERDICT: PASS
or
VERDICT: FAIL
or
VERDICT: PARTIAL

PARTIAL is for environmental limitations only (no test framework, tool unavailable, server can not start). If you can run the check, you must decide PASS or FAIL.

- **FAIL**: include what failed, exact error output, reproduction steps.
- **PARTIAL**: what was verified, what could not be and why, what the implementer should know.`,
		denyTools: []string{"write_file", "edit_file", "multi_edit", "fileops"},
	},
}

// ParseAgentType converts a string to an AgentType.
func ParseAgentType(s string) AgentType {
	return AgentType(s)
}

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
	description string,
	prompt string,
	subagentType string,
	model string,
	runInBackground bool,
	allowedTools []string,
	disallowedTools []string,
	inheritContext bool,
	parentMessages []map[string]any,
) (agentID string, result string, errText string, toolsUsed int, durationMs int64) {
	start := time.Now()

	// Convert string subagentType to AgentType
	agentType := ParseAgentType(subagentType)

	// Build child config and registry
	childCfg := a.buildSubAgentConfig(model)
	childRegistry := a.buildSubAgentRegistry(agentType, allowedTools, disallowedTools, runInBackground)

	// Build child system prompt (includes agent type specialization)
	childSysPrompt := buildSubAgentSystemPrompt(childRegistry, childCfg, agentType)

	// Create task in task store with compact hex ID
	taskID := fmt.Sprintf("agent-%s", generateShortID())
	if a.taskStore != nil {
		a.taskStore.CreateTask(taskID, prompt, model, subagentType)
	}

	// Register agent name in parent's name registry
	if a.agentNameRegistry != nil {
		if shortName := extractAgentName(prompt); shortName != "" {
			a.agentNameRegistry[shortName] = taskID
		}
	}

	if runInBackground {
		a.activeSubAgents.Add(1)

		// Capture parent context for fork mode
		var parentEntries []conversationEntry
		if inheritContext && a.context != nil {
			a.context.mu.RLock()
			parentEntries = make([]conversationEntry, len(a.context.entries))
			copy(parentEntries, a.context.entries)
			a.context.mu.RUnlock()
		}

		// Create independent cancellable context for async sub-agent
		asyncCtx, asyncCancel := context.WithCancel(context.Background())

		// Store cancel func in task state for external cancellation via CancelSubAgent
		if a.taskStore != nil {
			if task := a.taskStore.GetTask(taskID); task != nil {
				task.CancelFunc = asyncCancel
			}
		}

		// Create task in the new AgentTaskStore (with output capture)
		var bgTask *tools.AgentTask
		if a.agentTaskStore != nil {
			bgTask = a.agentTaskStore.CreateWithID(taskID, description, subagentType, prompt, model)
			bgTask.CancelFunc = asyncCancel
		}

		// Launch background goroutine with independent cancellation
		go func() {
			defer a.activeSubAgents.Add(-1)
			defer asyncCancel() // ensure context is released when done


			if a.taskStore != nil {
				a.taskStore.UpdateStatus(taskID, TaskStatusRunning)
			}

			if bgTask != nil {
					bgTask.SetStatus(tools.TaskRunning)
			}

			childLoop, err := a.createChildAgentLoop(childCfg, childRegistry)
			if err != nil {
				if a.taskStore != nil {
					a.taskStore.FailTask(taskID, fmt.Sprintf("failed to create: %v", err))
				}
				if bgTask != nil {
					bgTask.SetStatus(tools.TaskFailed)
				}
				a.EnqueueAgentNotification(taskID, "failed", "", "", 0, 0)
				return
			}
			defer childLoop.Close()

			// Set agentOutput for the created child loop (override with task buffer writer)
			if bgTask != nil {
				childLoop.agentOutput = &taskOutputWriter{task: bgTask}
			} else {
				childLoop.agentOutput = io.Discard
			}

			// Store the child's transcript path in the task state
			if a.taskStore != nil {
				if task := a.taskStore.GetTask(taskID); task != nil {
					task.SetTranscriptPath(childLoop.TranscriptPath())
				}
			}
			if bgTask != nil {
				bgTask.SetTranscriptPath(childLoop.TranscriptPath())
			}

			// Wire async cancellation context into child loop
			childLoop.cancelCtx = asyncCtx
			childLoop.cancelFunc = asyncCancel

			childLoop.context.SetSystemPrompt(childSysPrompt)

			// Apply fork mode with cloned entries (filtered same as sync path)
			if inheritContext && len(parentEntries) > 0 {
				filtered := filterEntriesForFork(parentEntries)
				childLoop.context.mu.Lock()
				for _, entry := range filtered {
					childLoop.context.entries = append(childLoop.context.entries, entry)
				}
				childLoop.context.mu.Unlock()
			}

			childResult := childLoop.Run(prompt)

			// If Run returned empty, try to recover partial results from conversation context
			if childResult == "" {
				childResult = childLoop.getPartialResult()
			}

			// Capture final result into the task's output buffer
			if bgTask != nil {
				bgTask.WriteOutput(childResult)
					bgTask.SetToolsInfo(int(childLoop.budget.consumed.Load()), time.Since(start).Milliseconds())
			}

			turnsUsed := int(childLoop.budget.consumed.Load())
			dur := time.Since(start).Milliseconds()

			if a.taskStore != nil {
				a.taskStore.CompleteTask(taskID, childResult, turnsUsed, dur)
			}
			if bgTask != nil {
					bgTask.SetStatus(tools.TaskCompleted)
			}
			a.EnqueueAgentNotification(taskID, "completed", childResult, childLoop.TranscriptPath(), turnsUsed, dur)
		}()

		return taskID, fmt.Sprintf("Agent launched in background.\n\nagentId: %s\nStatus: async_launched", taskID), "", 0, time.Since(start).Milliseconds()
	}

	// ─── Synchronous path ────────────────────────────────────────────────
	a.activeSubAgents.Add(1)
	defer a.activeSubAgents.Add(-1)

	if a.taskStore != nil {
		a.taskStore.UpdateStatus(taskID, TaskStatusRunning)
	}

	childLoop, err := a.createChildAgentLoop(childCfg, childRegistry)
	if err != nil {
		if a.taskStore != nil {
			a.taskStore.FailTask(taskID, err.Error())
		}
		return taskID, "", fmt.Sprintf("failed to create sub-agent: %v", err), 0, time.Since(start).Milliseconds()
	}
	defer childLoop.Close()

	// Store the child's transcript path in the task state
	if a.taskStore != nil {
		if task := a.taskStore.GetTask(taskID); task != nil {
			task.SetTranscriptPath(childLoop.TranscriptPath())
		}
	}

	childLoop.context.SetSystemPrompt(childSysPrompt)

	if inheritContext {
		a.cloneContextForFork(childLoop)
	}

	result = childLoop.Run(prompt)

	// If Run returned empty, try to recover partial results from conversation context
	if result == "" {
		result = childLoop.getPartialResult()
	}

	turnsUsed := int(childLoop.budget.consumed.Load())
	durationMs = time.Since(start).Milliseconds()

	if a.taskStore != nil {
		a.taskStore.CompleteTask(taskID, result, turnsUsed, durationMs)
	}

	return taskID, result, "", turnsUsed, durationMs
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
// only the tools the child is allowed to use. Filtering is applied in layers:
//
// Layer 1: allAgentDisallowedTools — always denied for all sub-agents
// Layer 2: asyncAgentDisallowedTools — additionally denied for async sub-agents
// Layer 3: agent type specific denyTools
// Layer 4: explicit disallowedTools from the caller
//
// After filtering, if allowedTools (whitelist) is provided, only those tools
// are included. A wildcard "*" in allowedTools means "all non-disallowed tools".
func (a *AgentLoop) buildSubAgentRegistry(agentType AgentType, allowedTools, disallowedTools []string, runInBackground bool) *tools.Registry {
	childRegistry := tools.NewRegistry()

	disallowed := make(map[string]bool)

	// Layer 1: global disallowed — always applied
	for t := range allAgentDisallowedTools {
		disallowed[t] = true
	}

	// Layer 2: async-specific disallowed
	if runInBackground {
		for t := range asyncAgentDisallowedTools {
			disallowed[t] = true
		}
	}

	// Layer 3: agent type specific deny list
	if typeConfig, ok := agentTypeConfigs[agentType]; ok {
		for _, t := range typeConfig.denyTools {
			disallowed[t] = true
		}
	}

	// Layer 4: explicit disallowed from the caller
	for _, t := range disallowedTools {
		disallowed[t] = true
	}

	// Build allowed (whitelist) set
	hasAllowed := len(allowedTools) > 0
	allowed := make(map[string]bool)
	wildcardAllowed := false
	for _, t := range allowedTools {
		if t == "*" {
			wildcardAllowed = true
			continue
		}
		allowed[t] = true
	}

	// Copy tools from parent registry
	for _, tool := range a.registry.AllTools() {
		name := tool.Name()

		// Skip disallowed tools
		if disallowed[name] {
			continue
		}

		// If explicit whitelist is provided, only include allowed tools
		if hasAllowed && !wildcardAllowed && !allowed[name] {
			continue
		}

		childRegistry.Register(tool)
	}

	return childRegistry
}

// buildSubAgentSystemPrompt creates a system prompt for the child agent.
// For Explore/Plan agents, CLAUDE.md and gitStatus are omitted for efficiency
// (saves ~5-15 Gtok/week and ~1-3 Gtok/week respectively).
func buildSubAgentSystemPrompt(registry *tools.Registry, cfg Config, agentType AgentType) string {
	toolList := buildToolList(registry)

	wd, _ := os.Getwd()
	// Get shell information
	shell := os.Getenv("SHELL")
	if shell == "" {
		// Windows detection: check for PowerShell
		if strings.Contains(strings.ToLower(os.Getenv("PSModulePath")), "powershell") {
			shell = "powershell"
		} else {
			shell = "bash"
		}
	}
	envInfo := fmt.Sprintf("%s / %s / %s / %s", runtime.GOOS, shell, runtime.Version(), runtime.GOARCH)

	// Determine if we should omit CLAUDE.md and gitStatus (Explore/Plan agents)
	omitClaudeMd := agentType == AgentTypeExplore || agentType == AgentTypePlan
	omitGitStatus := agentType == AgentTypeExplore || agentType == AgentTypePlan

	var sb strings.Builder

	// Apply agent type specific prompt modifier FIRST (identity + behavioral constraints)
	if typeConfig, ok := agentTypeConfigs[agentType]; ok {
		sb.WriteString(typeConfig.promptModifier)
		sb.WriteString("\n\n")
	}

	// Environment section
	sb.WriteString("## Environment\n")
	sb.WriteString(fmt.Sprintf("- Working directory: %s\n", wd))
	sb.WriteString(fmt.Sprintf("- OS: %s\n", envInfo))
	sb.WriteString(fmt.Sprintf("- Model: %s\n\n", cfg.Model))

	// Permission mode
	sb.WriteString(fmt.Sprintf("## Permission Mode: %s\n\n", strings.ToUpper(string(cfg.PermissionMode))))

	// For Explore/Plan agents, add efficiency optimization note
	if omitClaudeMd && omitGitStatus {
		sb.WriteString("## Note\n")
		sb.WriteString("CLAUDE.md and gitStatus are omitted for efficiency.\n\n")
	}

	// Available tools section with structured format
	sb.WriteString("## Available Tools\n\n")
	sb.WriteString("You have access to the following tools. Use them to accomplish your task.\n\n")
	sb.WriteString(toolList)
	sb.WriteString("\n\n")

	// Output format section
	sb.WriteString("## Output Format\n")
	sb.WriteString("- Share file paths as absolute paths (never relative).\n")
	sb.WriteString("- Avoid emojis — plain text communication only.\n")
	sb.WriteString("- Do not use a colon before tool calls.\n")
	sb.WriteString("- Do NOT ask the user questions — you must complete the task autonomously.\n")
	sb.WriteString("- When done, provide your final answer concisely.\n")
	sb.WriteString("- If you cannot complete the task, explain what you found and what is missing.\n\n")

	// Operational notes
	sb.WriteString("## Operational Notes\n")
	sb.WriteString("- Agent threads always have their cwd reset between bash calls — only use absolute file paths.\n")
	sb.WriteString("- Be thorough but efficient — avoid redundant reads or searches.\n\n")

	// Security section for sub-agents
	sb.WriteString("## Security\n")
	sb.WriteString("- You are a sub-agent with limited access.\n")
	sb.WriteString("- Do not attempt to modify system configuration or security settings.\n")
	sb.WriteString("- If you encounter sensitive data, report it but do not store it.\n")
	sb.WriteString("- Follow the principle of least privilege.\n")

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

	// Wire auto mode classifier if enabled
	if cfg.AutoClassifierEnabled && cfg.PermissionMode == ModeAuto {
		classifierModel := cfg.AutoClassifierModel
		if classifierModel == "" {
			classifierModel = cfg.Model
		}
		apiKey := cfg.APIKey
		if apiKey == "" {
			apiKey = os.Getenv("ANTHROPIC_API_KEY")
		}
		classifier := NewAutoModeClassifier(apiKey, cfg.BaseURL, classifierModel)
		child.gate.WithClassifier(classifier)
		child.gate.WithTranscriptSource(child.context)
	}

	return child, nil
}

// filterEntriesForFork filters parent conversation entries for use in a child agent's
// forked context. It returns a copy of the entries with:
//   - The last ToolUseContent entry removed (the agent tool that triggered this child
//     has no corresponding tool_result in the child context, so including it would
//     cause an API error for missing tool_result).
//   - CompactBoundaryContent and AttachmentContent entries removed (not meaningful in
//     the child's context).
//   - All other entries (TextContent, ToolUseContent, ToolResultContent, SummaryContent)
//     preserved as-is so the child can reference the parent's prior work.
func filterEntriesForFork(entries []conversationEntry) []conversationEntry {
	// Find the last ToolUseContent entry index (the agent tool that triggered this child)
	lastToolUseIdx := -1
	for i, entry := range entries {
		if _, ok := entry.content.(ToolUseContent); ok {
			lastToolUseIdx = i
		}
	}

	filtered := make([]conversationEntry, 0, len(entries))
	for i, entry := range entries {
		// Skip the last ToolUseContent (agent tool) -- the child has no tool_result for it
		if i == lastToolUseIdx {
			continue
		}

		switch entry.content.(type) {
		case TextContent, ToolUseContent, ToolResultContent, SummaryContent:
			filtered = append(filtered, conversationEntry{
				role:    entry.role,
				content: entry.content,
			})
		case CompactBoundaryContent, AttachmentContent:
			// Skip compact boundaries and attachments in fork mode
			continue
		default:
			continue
		}
	}
	return filtered
}

// cloneContextForFork clones the parent's conversation context into the child agent.
// Tool results are preserved as-is so the child can reference the parent's work.
// The last ToolUseContent (the agent tool that triggered this child) is NOT
// copied, because the child does not have a corresponding tool_result for it.
func (a *AgentLoop) cloneContextForFork(childLoop *AgentLoop) {
	a.context.mu.RLock()
	filtered := filterEntriesForFork(a.context.entries)
	a.context.mu.RUnlock()

	childLoop.context.mu.Lock()
	defer childLoop.context.mu.Unlock()
	childLoop.context.entries = append(childLoop.context.entries, filtered...)
}

// SendMessageToSubAgent sends a message to a running sub-agent or returns its status.
func (a *AgentLoop) SendMessageToSubAgent(agentID string, message string) (string, string) {
	if a.taskStore != nil {
		if task := a.taskStore.GetTask(agentID); task != nil {
			if task.IsTerminal() {
				return fmt.Sprintf("Agent %s has completed.\nStatus: %d\nResult: %s",
					agentID, task.Status, task.Result), ""
			}
			if message != "" {
				task.AddPendingMessage(message)
				return fmt.Sprintf("Message queued for agent %s", agentID), ""
			}
			return fmt.Sprintf("Agent %s is still running.\nStatus: %d", agentID, task.Status), ""
		}
	}
	return "", fmt.Sprintf("agent %s not found", agentID)
}

// GetSubAgentStatus returns the status of a sub-agent task.
func (a *AgentLoop) GetSubAgentStatus(agentID string) string {
	if a.taskStore != nil {
		if task := a.taskStore.GetTask(agentID); task != nil {
			return fmt.Sprintf("Agent: %s\nStatus: %d\nStarted: %s\nTools used: %d",
				task.ID, task.Status, task.StartTime.Format(time.RFC3339), task.ToolsUsed)
		}
	}
	return fmt.Sprintf("Agent %s: not found", agentID)
}

// GetSubAgentOutput retrieves the output of a sub-agent task, optionally
// blocking until the task completes. This is the callback wired to TaskOutputTool.
// For bash background tasks (with OutputFile), it reads output from the disk file.
func (a *AgentLoop) GetSubAgentOutput(agentID string, block bool, timeout time.Duration) (string, string) {
	if a.taskStore == nil {
		return "", "task store not available"
	}

	task := a.taskStore.GetTask(agentID)
	if task == nil {
		return "", fmt.Sprintf("agent %s not found", agentID)
	}

	if task.IsTerminal() {
		result := formatTaskResult(task)
		// For bash tasks with output file, append the file contents
		if task.OutputFile != "" && task.Result == "" {
			if data, err := os.ReadFile(task.OutputFile); err == nil {
				result += fmt.Sprintf("\n\n--- Output from %s ---\n%s", task.OutputFile, string(data))
			}
		}
		return result, ""
	}

	if block {
		// Poll until task completes or timeout
		deadline := time.Now().Add(timeout)
		for time.Now().Before(deadline) {
			time.Sleep(500 * time.Millisecond)
			if task.IsTerminal() {
				result := formatTaskResult(task)
				if task.OutputFile != "" && task.Result == "" {
					if data, err := os.ReadFile(task.OutputFile); err == nil {
						result += fmt.Sprintf("\n\n--- Output from %s ---\n%s", task.OutputFile, string(data))
					}
				}
				return result, ""
			}
		}
		return fmt.Sprintf("Agent: %s\nStatus: %d (still running after timeout)", task.ID, task.Status), ""
	}

	return fmt.Sprintf("Agent: %s\nStatus: %d (still running)", task.ID, task.Status), ""
}

// formatTaskResult formats a task's result for display.
func formatTaskResult(task *TaskState) string {
	result := fmt.Sprintf("Agent: %s\nStatus: %d\nResult: %s", task.ID, task.Status, task.Result)
	if tp := task.GetTranscriptPath(); tp != "" {
		result += fmt.Sprintf("\nTranscriptPath: %s", tp)
	}
	return result
}

// getPartialResult extracts the last assistant text from the conversation
// context as a partial result when the agent's Run returns empty.
func (a *AgentLoop) getPartialResult() string {
	if a.context == nil {
		return ""
	}
	a.context.mu.RLock()
	defer a.context.mu.RUnlock()

	// Get the last assistant message with text content
	for i := len(a.context.entries) - 1; i >= 0; i-- {
		if a.context.entries[i].role == "assistant" {
			if text, ok := a.context.entries[i].content.(TextContent); ok && string(text) != "" {
				return string(text)
			}
		}
	}
	return ""
}

// CancelSubAgent cancels a running sub-agent by agent ID.
// It calls the cancel function stored in the task state and marks the task as killed.
func (a *AgentLoop) CancelSubAgent(agentID string) {
	if a.taskStore == nil && a.agentTaskStore == nil {
		return
	}

	// Kill in the new AgentTaskStore
	if a.agentTaskStore != nil {
		a.agentTaskStore.Kill(agentID)
	}

	// Also kill in the legacy TaskStore for backward compatibility
	if a.taskStore != nil {
		task := a.taskStore.GetTask(agentID)
		if task == nil || task.IsTerminal() {
			return
		}
		if task.CancelFunc != nil {
			task.CancelFunc()
		}
		a.taskStore.UpdateStatus(agentID, TaskStatusKilled)
	}
}

// StopBackgroundTask forcibly stops a running background task (async sub-agent or bash task).
// It kills the OS process if one is tracked, or cancels the context for async agents,
// then marks the task as killed. Returns an error if the task is not found or not running.
func (a *AgentLoop) StopBackgroundTask(taskID string) error {
	// Try the new AgentTaskStore first
	if a.agentTaskStore != nil {
		task := a.agentTaskStore.Get(taskID)
		if task != nil {
			if task.IsTerminal() {
				return fmt.Errorf("task %s is not running (status: %s)", taskID, task.Status)
			}
			a.agentTaskStore.Kill(taskID)
			return nil
		}
	}

	// Fall back to legacy TaskStore for bash background tasks
	if a.taskStore == nil {
		return fmt.Errorf("task store not available")
	}
	task := a.taskStore.GetTask(taskID)
	if task == nil {
		return fmt.Errorf("task %s not found", taskID)
	}
	if task.IsTerminal() {
		return fmt.Errorf("task %s is not running (status: %d)", taskID, task.Status)
	}

	a.taskStore.KillTask(taskID)
	return nil
}

// resolveAgentID resolves a name or agent ID to an agent ID.
// If the input matches a registered name, returns the corresponding agent ID.
// Otherwise, returns the input as-is (assumed to be an agent ID directly).
func (a *AgentLoop) resolveAgentID(nameOrID string) string {
	if a.agentNameRegistry != nil {
		if agentID, ok := a.agentNameRegistry[nameOrID]; ok {
			return agentID
		}
	}
	return nameOrID
}

// ResumeAsyncAgent creates a new AgentLoop from a completed async task's transcript.
// The caller is responsible for managing the returned agent (calling Run, Close, etc).
// Returns an error if the task is not found, has no transcript path, or the transcript
// cannot be read.
func (a *AgentLoop) ResumeAsyncAgent(taskID string) (*AgentLoop, error) {
	if a.taskStore == nil {
		return nil, fmt.Errorf("task store not available")
	}

	task := a.taskStore.GetTask(taskID)
	if task == nil {
		return nil, fmt.Errorf("agent %s not found", taskID)
	}

	transcriptPath := task.GetTranscriptPath()
	if transcriptPath == "" {
		return nil, fmt.Errorf("agent %s has no transcript path", taskID)
	}

	// Use the parent agent's config and registry to create the resumed agent
	// from the stored transcript.
	resumedAgent, err := NewAgentLoopFromTranscript(
		a.config,
		a.registry,
		a.useStream,
		transcriptPath,
		false, // start a new session transcript for the resumed agent
	)
	if err != nil {
		return nil, fmt.Errorf("failed to resume agent from transcript: %w", err)
	}

	return resumedAgent, nil
}

// extractAgentName extracts a short name from the prompt/description string.
// It returns the first word if it looks like a valid identifier, or "" otherwise.
func extractAgentName(description string) string {
	description = strings.TrimSpace(description)
	if description == "" {
		return ""
	}
	// Take first word as name (if it looks like an identifier)
	parts := strings.Fields(description)
	if len(parts) == 0 {
		return ""
	}
	name := parts[0]
	// Validate: must be alphanumeric with hyphens/underscores, max 32 chars
	if len(name) > 32 {
		return ""
	}
	for _, c := range name {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_') {
			return ""
		}
	}
	return name
}

// taskOutputWriter is an io.Writer that captures output into an AgentTask's
// buffer. Used as the agentOutput writer for background sub-agents to avoid
// process-level os.Stdout/os.Stderr redirection (which would block the main REPL).
type taskOutputWriter struct {
	task *tools.AgentTask
}

func (w *taskOutputWriter) Write(p []byte) (int, error) {
	w.task.WriteOutput(string(p))
	return len(p), nil
}

