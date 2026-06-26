package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"

	"miniclaudecode-go/mcp"
	"miniclaudecode-go/tools"
)

// trigger and compactSummary are passed to post-compact hooks.
func (a *AgentLoop) PostCompactRecovery(trigger HookTrigger, compactSummary string) []string {
	// Reset cache break detector baseline — compaction invalidates all cached prefixes.
	a.cacheBreakDetector.ResetBaseline()
	// Guard against false-positive break detection on next API call after compaction.
	a.cacheBreakDetector.MarkPostCompaction()
	// File content recovery — optional, skipped when PostCompactRecoverFiles is false.
	// All other recovery steps (tools, agents, todo, session memory) always run.
	var recoveredPaths []string

	// --- File content recovery (optional) ---
	// Implements upstream's snapshot-then-clear pattern:
	// 1. Snapshot all cached file content from registry.filesRead BEFORE compaction clears it
	// 2. After compaction, re-inject the most recently accessed files as attachments
	// 3. Prefer cached content (what the model saw) over disk re-read (may have changed)
	if a.config.PostCompactRecoverFiles && a.registry != nil {
		maxFiles := a.config.PostCompactMaxFiles
		if maxFiles <= 0 {
			maxFiles = 5
		}
		// Use token-based budget (upstream-compatible), fall back to char-based if not set
		maxFileTokens := a.config.PostCompactMaxFileTokens
		if maxFileTokens <= 0 {
			// Legacy char-based fallback: convert chars to approximate tokens
			maxFileChars := a.config.PostCompactMaxFileChars
			if maxFileChars <= 0 {
				maxFileChars = 50000
			}
			maxFileTokens = maxFileChars / 4
		}
		// Per-file token cap (matches upstream POST_COMPACT_MAX_TOKENS_PER_FILE = 5000)
		maxTokensPerFile := a.config.PostCompactMaxTokensPerFile
		if maxTokensPerFile <= 0 {
			maxTokensPerFile = 5000
		}

		// Collect file paths already visible in preserved messages (after boundary).
		// These are files whose read results survived compaction, so re-injecting
		// them would be redundant. Matches upstream's collectReadToolFilePaths.
		preservedReadPaths := collectReadToolFilePaths(a.context)

		paths := a.registry.GetRecentlyReadFiles(maxFiles)
		totalTokens := 0
		filesRecovered := 0

		for _, path := range paths {
			// Expand the normalized path back to a real path
			realPath := path
			if !filepath.IsAbs(realPath) {
				realPath = filepath.Join(a.config.ProjectDir, realPath)
			}

			// Skip plan files and memory files (CLAUDE.md, etc.)
			if shouldExcludeFromPostCompactRestore(realPath, a.config.ProjectDir) {
				continue
			}

			// Skip files already visible in the preserved message tail
			if preservedReadPaths != nil && preservedReadPaths[realPath] {
				continue
			}

			// Snapshot-then-clear pattern: prefer cached content from registry
			// (what the model actually saw) over disk re-read.
			// This matches upstream's generateFileAttachment using readFileState.content.
			var content string
			if cached := a.registry.GetCachedFileContent(path); cached != "" {
				content = cached
			} else {
				// Fallback: re-read from disk if no cached content
				data, err := os.ReadFile(realPath)
				if err != nil {
					continue // file may have been deleted
				}
				content = string(data)
			}

			// Per-file token truncation (matches upstream POST_COMPACT_MAX_TOKENS_PER_FILE)
			contentTokens := EstimateTokens(content)
			if contentTokens > maxTokensPerFile {
				charLimit := maxTokensPerFile * 4
				if charLimit < len(content) {
					content = content[:charLimit] + "\n... [truncated for compaction; use Read to get full content]"
					contentTokens = EstimateTokens(content)
				}
			}

			// Total budget check (matches upstream POST_COMPACT_TOKEN_BUDGET = 50000)
			if totalTokens+contentTokens > maxFileTokens {
				break
			}

			attachment := fmt.Sprintf("[Post-compact file recovery: %s -- ALREADY IN CONTEXT, DO NOT RE-READ]\n## IMPORTANT: This file was recovered from cache. Do NOT call Read or read_file again on this file unless you believe its content has changed.\n## Re-reading wastes tokens and causes compaction churn.\n```\n%s\n```", path, content)
			a.context.AddAttachment(attachment)
			totalTokens += contentTokens
			filesRecovered++
			recoveredPaths = append(recoveredPaths, path)

			// Re-mark file as read so edit checks still work
			a.registry.MarkFileRead(path)
		}

		if filesRecovered > 0 {
			a.logDebug("[post-compact] Recovered %d files (~%d tokens)\n", filesRecovered, totalTokens)
		}
	}

	// --- Skill content recovery ---
	if a.skillTracker != nil && a.config.SkillLoader != nil {
		// Use token-based budgets (upstream-compatible), fall back to char-based if not set
		maxSkillTokens := a.config.PostCompactMaxSkillTokens
		if maxSkillTokens <= 0 {
			maxSkillChars := a.config.PostCompactMaxSkillChars
			if maxSkillChars <= 0 {
				maxSkillChars = 5000
			}
			maxSkillTokens = maxSkillChars / 4
		}
		maxTotalSkillTokens := a.config.PostCompactMaxTotalSkillTokens
		if maxTotalSkillTokens <= 0 {
			maxTotalSkillChars := a.config.PostCompactMaxTotalSkillChars
			if maxTotalSkillChars <= 0 {
				maxTotalSkillChars = 25000
			}
			maxTotalSkillTokens = maxTotalSkillChars / 4
		}

		readSkills := a.skillTracker.GetReadSkillNames()
		totalSkillTokens := 0
		skillsRecovered := 0

		for _, name := range readSkills {
			content := a.config.SkillLoader.LoadSkill(name)
			if content == "" {
				continue
			}

			contentTokens := EstimateTokens(content)
			if contentTokens > maxSkillTokens {
				// Truncate per-skill: approximate char limit from token budget
				charLimit := maxSkillTokens * 4
				if charLimit < len(content) {
					content = content[:charLimit] + "\n\n[... skill content truncated for compaction; use Read on the skill path if you need the full text]"
					contentTokens = EstimateTokens(content)
				}
			}

			if totalSkillTokens+contentTokens > maxTotalSkillTokens {
				break
			}

			attachment := fmt.Sprintf("[Post-compact skill recovery: %s]\n%s", name, content)
			a.context.AddAttachment(attachment)
			totalSkillTokens += contentTokens
			skillsRecovered++
		}

		if skillsRecovered > 0 {
			a.logDebug("[post-compact] Recovered %d skills (~%d tokens)\n", skillsRecovered, totalSkillTokens)
		}
	}

	// --- Plan file recovery ---
	// Re-inject the current plan file if one exists, so the model knows
	// what it was working on and what to do next.
	planAttachment := buildPostCompactPlanAttachment(a.config.ProjectDir)
	if planAttachment != "" {
		a.context.AddAttachment(planAttachment)
		a.logDebug("[post-compact] Recovered plan file\n")
	}

	// --- Plan mode recovery ---
	// If in plan mode, remind the model to continue planning without executing.
	if a.config.PermissionMode == ModePlan {
		a.context.AddAttachment("## Plan Mode Active\n\nYou are in plan mode. Do NOT execute any tools without first presenting your plan to the user and getting their approval. Continue planning — do not execute.")
		a.logDebug("[post-compact] Plan mode reminder injected\n")
	}

	// --- Tools re-announcement (delta-based) ---
	// After compaction the model loses all tool-use history. Re-announce the
	// tool inventory so the model knows what capabilities are available.
	// Only announce tools NOT already visible in the preserved message tail,
	// matching upstream's getDeferredToolsDeltaAttachment pattern.
	// Compute once and share with MCP/Agent announcements.
	preservedToolNames := collectUsedToolNamesInPreservedMessages(a.context)
	if a.registry != nil {
		toolsAttachment := a.buildPostCompactToolsAnnouncement(preservedToolNames)
		if toolsAttachment != "" {
			a.context.AddAttachment(toolsAttachment)
			a.logDebug("[post-compact] Re-announced tool inventory (delta)\n")
		} else {
			a.logDebug("[post-compact] All tools already visible in preserved tail, skipping re-announcement\n")
		}
	}

	// --- MCP tools re-announcement (delta-based) ---
	// Re-announce available MCP servers and their tools after compaction.
	// Only announce MCP servers NOT already visible in the preserved message tail
	// (via mcp_call_tool usage), matching upstream's getMcpInstructionsDeltaAttachment.
	if a.config.MCPManager != nil {
		mcpAttachment := a.buildPostCompactMCPAnnouncement(preservedToolNames)
		if mcpAttachment != "" {
			a.context.AddAttachment(mcpAttachment)
			a.logDebug("[post-compact] Re-announced MCP tools (delta)\n")
		}
	}

	// --- Agent listing re-announcement (delta-based) ---
	// Re-announce active background sub-agents after compaction so the model
	// doesn't lose track of running tasks.
	// Only announce agents NOT already visible in the preserved message tail,
	// matching upstream's getAgentListingDeltaAttachment.
	if a.agentTaskStore != nil {
		agentAttachment := a.buildPostCompactAgentAnnouncement(preservedToolNames)
		if agentAttachment != "" {
			a.context.AddAttachment(agentAttachment)
			a.logDebug("[post-compact] Re-announced background agents (delta)\n")
		}
	}

	// --- Todo/Task recovery ---
	// Re-inject task state by scanning transcript for task_create, task_update,
	// and TodoWrite tool calls. This survives compact since the transcript persists.
	taskAttachment := buildTaskRecoveryAttachment(a.context)
	if taskAttachment != "" {
		a.context.AddAttachment(taskAttachment)
		a.logDebug("[post-compact] Task/Todo state recovered\n")
	}

	// --- Session Memory Recovery ---
	// Re-inject session memory after compaction. Session memory contains
	// user-defined notes that must survive context compaction.
	// Uses per-section truncation matching upstream's truncateSessionMemoryForCompact.
	if a.config.SessionMemory != nil {
		smContent := a.config.SessionMemory.FormatForPromptCompact()
		if smContent != "" {
			attachment := fmt.Sprintf("<session_memory>\n%s\n</session_memory>", smContent)
			a.context.AddAttachment(attachment)
			a.logDebug("[post-compact] Session memory recovered\n")
		}
	}

	// --- Post-compact hooks ---
	// Execute registered post-compact hooks. These can inject additional content
	// into the prompt or display user messages. Matches upstream's executePostCompactHooks.
	if a.hooks != nil {
		hookInput := PostCompactInput{
			Trigger:        trigger,
			CompactSummary: compactSummary,
			RecoveredFiles: recoveredPaths,
		}
		hookResult, hookErr := a.hooks.ExecutePostCompactHooks(hookInput)
		if hookErr != nil {
			a.out("[hook] PostCompact error: %v\n", hookErr)
		}
		if hookResult.UserMessage != "" {
			a.out("%s\n", strings.TrimPrefix(hookResult.UserMessage, "\n"))
		}
		if hookResult.Attachment != "" {
			a.context.AddAttachment(hookResult.Attachment)
		}
	}

	// --- Post-compact cleanup ---
	// Clear caches and tracking state that were invalidated by compaction.
	// This matches upstream's runPostCompactCleanup() in postCompactCleanup.ts.

	// Re-inject the in-memory todo list into the system prompt so the model
	// sees its task list. The in-memory TodoList survives compaction; this
	// ensures the reminder is always included after compaction, regardless
	// of which recovery steps ran above.
	a.injectTodoReminder()

	a.RunPostCompactCleanup()

	return recoveredPaths
}

// RunPostCompactCleanup clears caches and tracking state after compaction.
// Call this after PostCompactRecovery in every compaction path.
// This prevents stale references (e.g. file history pointing to deleted messages,
// skill tracker with compacted-away state) from corrupting subsequent turns.
//
// Subagents (agent:*) run in the same process and share module-level state
// with the main thread. Only reset main-thread module-level state for main-thread
// compacts — subagent compaction must not clobber the main thread's state.
func (a *AgentLoop) RunPostCompactCleanup() {
	// Guard: subagents share module-level state with the main thread.
	// Skip clears that would corrupt the main thread when a subagent compacts.
	isMainThread := a.config.querySource == "" || strings.HasPrefix(a.config.querySource, "repl_main_thread") || a.config.querySource == "sdk"

	// Clear skill discovery state: after compaction, the system prompt is rebuilt
	// and should re-announce all skills as "new". The skill content is re-injected
	// via post-compact attachments, so shown/read state should reset.
	// Preserves usedSkills since "used" is a durable fact about the conversation.
	if a.skillTracker != nil {
		a.skillTracker.ResetPostCompact()
	}

	// Invalidate cached system prompt so it rebuilds fresh with post-compact state.
	// The cache was built before compaction and references pre-compact entries.
	if a.config.cachedPrompt != nil {
		a.config.cachedPrompt.MarkDirty()
	}

	// Save tool state conclusions to session memory BEFORE clearing.
	// Conclusions represent the agent's accumulated knowledge about work done;
	// if not saved, they are permanently lost after compaction.
	if a.toolStateTracker != nil && a.config.SessionMemory != nil {
		conclusions := a.toolStateTracker.GetConclusions()
		if len(conclusions) > 0 {
			a.config.SessionMemory.SaveConclusions(conclusions)
		}
		a.toolStateTracker.ClearConclusions()
	}


	// Classifier and permission state — stale decisions may reference
	// compacted messages, so clear them to force re-evaluation.
	a.gate.ResetPostCompact()

	// ─── Missing state clears matching upstream's runPostCompactCleanup() ───────

	// Clear speculative checks (bash permission evaluations) — stale decisions
	// may reference compacted-away messages. Main-thread only since subagents
	// share the same module-level permission state.
	if isMainThread {
		clearSpeculativeChecks()
	}

	// Clear beta tracing state (analytics) — after compaction, tracing context
	// is invalidated and should be reset.
	clearBetaTracingState()

	// Clear session messages cache (transcript/CLI display) — prevents stale
	// references to pre-compact messages. Main-thread only.
	if isMainThread {
		clearSessionMessagesCache()
	}

	// Sweep file content cache (commit attribution) — invalidate cached file
	// snapshots that reference compacted content. Only if COMMIT_ATTRIBUTION
	// feature is active.
	if isMainThread {
		sweepFileContentCache()
	}

	// Reset MCP instructions delta tracking — after compaction, the post-compact
	// announcement re-declares visible servers, so per-turn delta state must reset.
	a.announcedMCPServers = make(map[string]bool)

	// Reset beta header memoization and session latch — after compaction,
	// recompute headers from current env vars and model configuration.
	a.betaHeadersLatched = nil
	ClearBetaHeaderCache()

	// Inject active task status after compaction — ensures the LLM knows
	// what tasks are still pending even after context is compressed.
	// This prevents "task amnesia" in long-running sessions.
	a.injectTaskStatusAfterCompact()
}

// injectTaskStatusAfterCompact injects a system message listing active tasks
// after compaction, so the LLM can continue working on them.
// Shows hierarchical task tree with subtasks, priorities, dependencies, and time tracking.
func (a *AgentLoop) injectTaskStatusAfterCompact() {
	if a.workTaskStore == nil {
		return
	}

	store := a.workTaskStore
	activeTasks := store.ListActiveTasks()
	if len(activeTasks) == 0 {
		return
	}

	var sb strings.Builder
	sb.WriteString("## Task Context After Compaction\n\n")

	// Section 1: Active task tree with rich info
	sb.WriteString("### Active Tasks\n\n")
	topTasks := store.ListTopLevelTasks()
	if len(topTasks) > 0 {
		for _, t := range topTasks {
			if t.Status == WorkTaskDeleted || t.Status == WorkTaskCompleted {
				continue
			}
			sb.WriteString(formatTaskLine(t, 0))
			subtasks := store.ListSubtasks(t.ID)
			for _, st := range subtasks {
				if st.Status != WorkTaskDeleted && st.Status != WorkTaskCompleted {
					sb.WriteString(formatTaskLine(st, 1))
				}
			}
		}
	} else {
		// No hierarchy, show flat list
		for _, t := range activeTasks {
			sb.WriteString(formatTaskLine(t, 0))
		}
	}

	// Section 2: Blocked tasks with reasons
	blockedTasks := store.ListBlockedTasks()
	if len(blockedTasks) > 0 {
		sb.WriteString("\n### Blocked Tasks\n\n")
		for _, t := range blockedTasks {
			blockers := store.GetBlockers(t.ID)
			blockerInfo := make([]string, len(blockers))
			for i, b := range blockers {
				blockerInfo[i] = fmt.Sprintf("#%s (%s)", b.ID, b.Subject)
			}
			sb.WriteString(fmt.Sprintf("- #%s: %s — waiting on: %s\n",
				t.ID, t.Subject, strings.Join(blockerInfo, ", ")))
		}
	}

	// Section 3: Ready tasks (can start now)
	readyTasks := store.GetReadyTasks()
	if len(readyTasks) > 0 {
		sb.WriteString("\n### Ready to Start\n\n")
		for _, t := range readyTasks {
			pri := string(t.Priority)
			if pri == "" {
				pri = "medium"
			}
			sb.WriteString(fmt.Sprintf("- [#%s] %s (%s priority)\n", t.ID, t.Subject, pri))
		}
	}

	// Section 4: Priority distribution
	priStats := store.PriorityStats()
	if priStats[PriorityCritical] > 0 || priStats[PriorityHigh] > 0 {
		sb.WriteString("\n### Priority Summary\n\n")
		if priStats[PriorityCritical] > 0 {
			sb.WriteString(fmt.Sprintf("- Critical: %d task(s)\n", priStats[PriorityCritical]))
		}
		if priStats[PriorityHigh] > 0 {
			sb.WriteString(fmt.Sprintf("- High: %d task(s)\n", priStats[PriorityHigh]))
		}
	}

	// Section 5: Time tracking summary
	var overdueTasks []*WorkTask
	var longRunningTasks []*WorkTask
	for _, t := range activeTasks {
		if t.IsOverdue(30 * time.Minute) {
			overdueTasks = append(overdueTasks, t)
		}
		if t.GetActiveTime() > 10*time.Minute {
			longRunningTasks = append(longRunningTasks, t)
		}
	}

	if len(overdueTasks) > 0 || len(longRunningTasks) > 0 {
		sb.WriteString("\n### Time Alerts\n\n")
		for _, t := range overdueTasks {
			sb.WriteString(fmt.Sprintf("- #%s: %s — OVERDUE (active for %s)\n",
				t.ID, t.Subject, t.GetActiveTime().Round(time.Minute)))
		}
		for _, t := range longRunningTasks {
			sb.WriteString(fmt.Sprintf("- #%s: %s — long-running (%s active)\n",
				t.ID, t.Subject, t.GetActiveTime().Round(time.Minute)))
		}
	}

	// Section 6: Execution plan hint
	groups := store.GetExecutionGroups()
	if len(groups) > 1 {
		sb.WriteString("\n### Execution Order\n\n")
		sb.WriteString(fmt.Sprintf("Tasks can be parallelized into %d groups:\n", len(groups)))
		for i, group := range groups {
			var ids []string
			for _, t := range group {
				ids = append(ids, fmt.Sprintf("#%s", t.ID))
			}
			sb.WriteString(fmt.Sprintf("  Group %d: %s\n", i+1, strings.Join(ids, ", ")))
		}
	}

	sb.WriteString("\nContinue working on these tasks. Mark done when complete. Do not re-create existing tasks.\n")

	a.context.AddAttachment(SystemInjectedPrefix + "\n" + sb.String())
	a.logDebug("\n[post-compact] Injected enhanced task context\n")
}

// formatTaskLine returns a formatted task line with priority and status icons.
func formatTaskLine(t *WorkTask, depth int) string {
	indent := strings.Repeat("  ", depth)

	// Status icon
	statusIcon := "[ ]"
	switch t.Status {
	case WorkTaskInProgress:
		statusIcon = "[>]"
	case WorkTaskBlocked:
		statusIcon = "[!]"
	case WorkTaskCompleted:
		statusIcon = "[x]"
	case WorkTaskCancelled:
		statusIcon = "[-]"
	}

	// Priority indicator
	pri := ""
	switch t.Priority {
	case PriorityCritical:
		pri = " !!!"
	case PriorityHigh:
		pri = " !!"
	case PriorityLow:
		pri = " (low)"
	}

	// Tags
	tags := ""
	if len(t.Tags) > 0 {
		tags = fmt.Sprintf(" [%s]", strings.Join(t.Tags, ","))
	}

	// Time info
	timeInfo := ""
	if t.Status == WorkTaskInProgress && t.GetActiveTime() > 0 {
		timeInfo = fmt.Sprintf(" (%s)", t.GetActiveTime().Round(time.Second))
	}

	return fmt.Sprintf("%s%s #%s: %s%s%s%s\n",
		indent, statusIcon, t.ID, t.Subject, pri, tags, timeInfo)
}

// buildPostCompactToolsAnnouncement re-announces available tools after compaction.
// Only tools NOT already visible in the preserved message tail are announced,
// avoiding redundant re-injection. The model loses tool-use history during
// compaction; this reminds it of tools it hasn't used in the preserved tail.
func (a *AgentLoop) buildPostCompactToolsAnnouncement(preservedToolNames map[string]bool) string {
	var sb strings.Builder
	sb.WriteString("## Tools Available After Compaction\n\n")
	sb.WriteString("The following tools are available. Use them as needed.\n\n")

	// Collect native, MCP, and skill tools, skipping those already visible
	// in the preserved message tail (to avoid redundant re-announcement).
	var nativeTools []string
	var mcpTools []string
	var skillTools []string
	for _, t := range a.registry.AllTools() {
		name := t.Name()
		// Skip tools already used in the preserved message tail
		if preservedToolNames != nil && preservedToolNames[name] {
			continue
		}
		desc := t.Description()
		entry := fmt.Sprintf("- **%s**: %s", name, desc)
		switch name {
		case "mcp_call_tool", "mcp_server_status":
			mcpTools = append(mcpTools, entry)
		case "search_skills", "read_skill", "list_skills":
			skillTools = append(skillTools, entry)
		default:
			nativeTools = append(nativeTools, entry)
		}
	}

	if len(nativeTools) > 0 {
		sb.WriteString("### Core Tools\n")
		for _, t := range nativeTools {
			sb.WriteString(t + "\n")
		}
		sb.WriteString("\n")
	}
	if len(mcpTools) > 0 {
		sb.WriteString("### MCP Tools\n")
		for _, t := range mcpTools {
			sb.WriteString(t + "\n")
		}
		sb.WriteString("\n")
	}
	if len(skillTools) > 0 {
		sb.WriteString("### Skill Tools\n")
		for _, t := range skillTools {
			sb.WriteString(t + "\n")
		}
		sb.WriteString("\n")
	}

	result := strings.TrimRight(sb.String(), "\n")
	// If all tools were already visible, return empty to skip the attachment
	if strings.HasSuffix(result, "Use them as needed.\n\n") {
		return ""
	}
	return result
}

// buildPostCompactPlanAttachment reads the most recent plan file from .claude/plan/
// and returns it as an attachment. Matches upstream's createPlanAttachmentIfNeeded.
func buildPostCompactPlanAttachment(projectDir string) string {
	planDir := filepath.Join(projectDir, ".claude", "plan")
	info, err := os.Stat(planDir)
	if err != nil || !info.IsDir() {
		return ""
	}

	entries, err := os.ReadDir(planDir)
	if err != nil {
		return ""
	}

	// Find the most recently modified .md file
	var newestPath string
	var newestTime int64
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(strings.ToLower(name), ".md") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		modTime := info.ModTime().Unix()
		if modTime > newestTime {
			newestTime = modTime
			newestPath = filepath.Join(planDir, entry.Name())
		}
	}

	if newestPath == "" {
		return ""
	}

	data, err := os.ReadFile(newestPath)
	if err != nil {
		return ""
	}

	content := string(data)
	return fmt.Sprintf("A plan file exists from plan mode at: %s\n\nPlan contents:\n\n%s\n\nIf this plan is relevant to the current work and not already complete, continue working on it.", newestPath, content)
}

// buildPostCompactMCPAnnouncement re-announces MCP servers and tools after compaction.
// Includes per-server instructions from the MCP initialize response.
// Delta-aware: only announces servers whose tools are NOT already visible in the
// preserved message tail, matching upstream's getMcpInstructionsDeltaAttachment.
func (a *AgentLoop) buildPostCompactMCPAnnouncement(preservedToolNames map[string]bool) string {
	mgr := a.config.MCPManager
	servers := mgr.ListServers()
	if len(servers) == 0 {
		return ""
	}

	// Collect MCP server names from preserved tool names.
	// MCP servers are identified by their tools (e.g., mcp_call_tool with server param,
	// or specific MCP tool names like mcp_server_status).
	preservedMCPServers := make(map[string]bool)
	for name := range preservedToolNames {
		if strings.HasPrefix(name, "mcp_") || name == "mcp_call_tool" {
			preservedMCPServers[name] = true
		}
	}

	// Filter servers to only those not already visible in preserved tail.
	// A server is "visible" if any of its tools have been used in the preserved messages.
	var serversToAnnounce []string
	serverTools := make(map[string][]mcp.Tool)
	serverInstructions := mgr.AllServerInstructions()
	for _, tws := range mgr.AllToolsWithServer() {
		serverTools[tws.Server] = append(serverTools[tws.Server], tws.Tool)
	}
	for _, server := range servers {
		// Check if this server's tools were used in preserved messages.
		// A server is visible if any of its tools appear in preservedToolNames.
		seen := false
		for _, tool := range serverTools[server] {
			if preservedToolNames[tool.Name] {
				seen = true
				break
			}
		}
		if seen {
			continue // skip — already visible in preserved tail
		}
		serversToAnnounce = append(serversToAnnounce, server)
	}

	if len(serversToAnnounce) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("## MCP Servers After Compaction\n\n")
	sb.WriteString("The following MCP servers are connected. Use list_mcp_tools to discover their tools, or call mcp_call_tool directly.\n\n")

	for _, server := range serversToAnnounce {
		status := mgr.GetServerStatus(server)
		tools := serverTools[server]
		statusIcon := "●"
		if status != "connected" {
			statusIcon = "○"
		}
		sb.WriteString(fmt.Sprintf("%s **%s** [%s] (%d tools)\n", statusIcon, server, status, len(tools)))
		for _, tool := range tools {
			desc := tool.Description
			if len(desc) > 80 {
				desc = desc[:80] + "..."
			}
			sb.WriteString(fmt.Sprintf("  - %s: %s\n", tool.Name, desc))
		}
		// Inject per-server instructions if available
		if instr, ok := serverInstructions[server]; ok && instr != "" {
			sb.WriteString(fmt.Sprintf("\n  **Usage instructions for %s:**\n  %s\n", server, instr))
		}
	}

	return strings.TrimRight(sb.String(), "\n")
}

// buildTaskRecoveryAttachment scans conversation entries for task_create, task_update,
// and TodoWrite tool calls, extracts the most recent task state, and formats it as
// an attachment for post-compact recovery. Matches upstream's extractTodosFromTranscript.
func buildTaskRecoveryAttachment(ctx *ConversationContext) string {
	entries := ctx.Entries()

	// Scan backward for task tool calls and collect task state
	type taskState struct {
		id          string
		subject     string
		status      string
		description string
	}
	latestTasks := make(map[string]taskState) // keyed by task ID
	var todoItems []string                    // TodoWrite items

	for i := len(entries) - 1; i >= 0; i-- {
		entry := entries[i]
		tu, ok := entry.content.(ToolUseContent)
		if !ok {
			continue
		}
		for _, block := range tu {
			// Extract tool_use block info
			jsonBytes, _ := json.Marshal(block)
			var parsed struct {
				Type  string `json:"type"`
				Name  string `json:"name"`
				Input struct {
					Subject     string `json:"subject"`
					Description string `json:"description"`
					Status      string `json:"status"`
					TaskId      string `json:"taskId"`
					ID          string `json:"id"`
					Todos       []struct {
						Content string `json:"content"`
						Status  string `json:"status"`
						Subject string `json:"subject"`
					} `json:"todos"`
				} `json:"input"`
			}
			if json.Unmarshal(jsonBytes, &parsed) != nil || parsed.Type != "tool_use" {
				continue
			}

			switch parsed.Name {
			case "task_create":
				id := parsed.Input.TaskId
				if id == "" {
					id = parsed.Input.ID
				}
				if id != "" {
					if _, exists := latestTasks[id]; !exists {
						latestTasks[id] = taskState{
							id:          id,
							subject:     parsed.Input.Subject,
							status:      "pending",
							description: parsed.Input.Description,
						}
					}
				}
			case "task_update":
				id := parsed.Input.TaskId
				if id == "" {
					id = parsed.Input.ID
				}
				if id != "" {
					if existing, ok := latestTasks[id]; ok {
						if parsed.Input.Status != "" {
							existing.status = parsed.Input.Status
						}
						if parsed.Input.Subject != "" {
							existing.subject = parsed.Input.Subject
						}
						latestTasks[id] = existing
					} else {
						latestTasks[id] = taskState{
							id:      id,
							subject: parsed.Input.Subject,
							status:  parsed.Input.Status,
						}
					}
				}
			case "TodoWrite":
				if len(parsed.Input.Todos) > 0 && len(todoItems) == 0 {
					for _, t := range parsed.Input.Todos {
						status := t.Status
						if status == "" {
							status = "pending"
						}
						content := t.Content
						if content == "" {
							content = t.Subject
						}
						icon := "O" // pending
						if status == "in_progress" {
							icon = "◐"
						} else if status == "completed" {
							icon = "●"
						}
						todoItems = append(todoItems, fmt.Sprintf("%s %s [%s]", icon, content, status))
					}
				}
			}
		}
	}

	var sb strings.Builder

	// Format task items
	if len(latestTasks) > 0 {
		sb.WriteString("## Tasks (recovered from transcript)\n\n")
		for _, t := range latestTasks {
			icon := "O"
			if t.status == "in_progress" {
				icon = "◐"
			} else if t.status == "completed" {
				icon = "●"
			}
			sb.WriteString(fmt.Sprintf("%s [%s] %s\n", icon, t.id, t.subject))
			if t.description != "" {
				sb.WriteString(fmt.Sprintf("  %s\n", t.description))
			}
		}
	}

	// Format todo items
	if len(todoItems) > 0 {
		if sb.Len() > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString("## Todo List (recovered from transcript)\n\n")
		for _, item := range todoItems {
			sb.WriteString(item + "\n")
		}
		sb.WriteString("\nUse task_list, task_update, or TodoWrite to manage these items.\n")
	}

	return sb.String()
}

// injectTodoReminder re-injects the in-memory todo list into the conversation
// as a system-injected user message.
// Called after compaction when the todo reminder entry may have been dropped.
// The TodoList survives compaction in memory; this just ensures
// it's visible to the model.
func (a *AgentLoop) injectTodoReminder() {
	reminder := a.todoList.BuildReminder()
	if reminder == "" {
		return
	}
	a.context.InjectTodoReminder(reminder)
}

// buildPostCompactAgentAnnouncement re-announces active and completed-but-unretrieved
// background sub-agents after compaction. Matches upstream's createAsyncAgentAttachmentsIfNeeded
// which includes all agents with retrieved==false (running + completed but not yet collected).
func (a *AgentLoop) buildPostCompactAgentAnnouncement(preservedToolNames map[string]bool) string {
	tasks := a.agentTaskStore.List()
	if len(tasks) == 0 {
		return ""
	}

	var active []*tools.AgentTask
	var completedUnretrieved []*tools.AgentTask
	for _, t := range tasks {
		if t.Status == tools.TaskRunning || t.Status == tools.TaskPending {
			active = append(active, t)
		} else if t.Status == tools.TaskCompleted && !t.Notified {
			// Completed but results not yet retrieved by the user/main agent.
			// Include these to prevent the model from re-spawning the same task.
			completedUnretrieved = append(completedUnretrieved, t)
		}
	}

	if len(active) == 0 && len(completedUnretrieved) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("## Background Agents After Compaction\n\n")

	if len(active) > 0 {
		sb.WriteString("The following sub-agents are running. Do NOT spawn duplicates for the same task.\n\n")
		for _, t := range active {
			sb.WriteString(fmt.Sprintf("- agentId: %s, status: %s, description: %s\n", t.ID, t.Status, t.Description))
			if !t.StartTime.IsZero() {
				dur := time.Since(t.StartTime)
				sb.WriteString(fmt.Sprintf("  (running for %s)\n", dur.Round(time.Second)))
			}
		}
	}

	if len(completedUnretrieved) > 0 {
		sb.WriteString("\nThe following sub-agents completed but results have not been retrieved. Check their output before spawning duplicates.\n\n")
		for _, t := range completedUnretrieved {
			outputInfo := ""
			if t.OutputFile != "" {
				outputInfo = fmt.Sprintf(", output: %s", t.OutputFile)
			}
			sb.WriteString(fmt.Sprintf("- agentId: %s, status: completed, description: %s%s\n", t.ID, t.Description, outputInfo))
		}
	}

	return strings.TrimRight(sb.String(), "\n")
}

// buildPreCompactFileSnapshot generates a snapshot of recently read files' content
// from the registry's cache, for injection into the compaction summary.
// This matches upstream's post-compact file attachment restoration, but embedded
// directly in the summary text for the non-LLM compact path.
//
// Parameters match upstream defaults:
//   - maxFiles: maximum number of files to include (upstream POST_COMPACT_MAX_FILES_TO_RESTORE = 5, but we use more for the summary)
//   - maxTokensPerFile: per-file token cap (upstream POST_COMPACT_MAX_TOKENS_PER_FILE = 5000)
//   - maxTotalTokens: total token budget (upstream POST_COMPACT_TOKEN_BUDGET = 50000)
func (a *AgentLoop) buildPreCompactFileSnapshot(maxFiles int, maxTokensPerFile int, maxTotalTokens int) string {
	if a.registry == nil {
		return ""
	}

	// Collect all recently read files sorted by recency
	type fileEntry struct {
		path    string
		content string
		tokens  int
	}
	var entries []fileEntry
	for _, path := range a.registry.GetRecentlyReadFiles(maxFiles) {
		cached := a.registry.GetCachedFileContent(path)
		if cached == "" {
			continue
		}
		tokens := EstimateTokens(cached)
		if tokens > maxTokensPerFile {
			charLimit := maxTokensPerFile * 4
			if charLimit < len(cached) {
				truncated := cached[:charLimit] + "\n... [truncated for compaction; use Read to get full content]"
				tokens = EstimateTokens(truncated)
				entries = append(entries, fileEntry{path: path, content: truncated, tokens: tokens})
			} else {
				entries = append(entries, fileEntry{path: path, content: cached, tokens: tokens})
			}
		} else {
			entries = append(entries, fileEntry{path: path, content: cached, tokens: tokens})
		}
	}

	if len(entries) == 0 {
		return ""
	}

	var sb strings.Builder
	totalTokens := 0
	fileCount := 0
	sb.WriteString("## File Contents at Compaction Time\n")
	for _, e := range entries {
		if totalTokens+e.tokens > maxTotalTokens {
			break
		}
		sb.WriteString(fmt.Sprintf("\n### %s\n```go\n%s\n```", e.path, e.content))
		totalTokens += e.tokens
		fileCount++
	}

	if fileCount == 0 {
		return ""
	}
	sb.WriteString(fmt.Sprintf("\n\n[Snapshot of %d recently read files, ~%d tokens]\n", fileCount, totalTokens))
	return sb.String()
}

// buildStructuredGoalBlock extracts goal state from in-memory structures.
// Unlike upstream's LLM-based goal extraction (probabilistic), this uses
// deterministic structured state that survives compaction intact.
// Returns the goal block text and whether any content was generated.
func (a *AgentLoop) buildStructuredGoalBlock(messages []anthropic.MessageParam) (string, bool) {
	var sb strings.Builder

	// 1. Pending Tasks from TodoList — the model's primary "what's left to do"
	if a.todoList != nil {
		pending := a.todoList.GetPendingTasks()
		if len(pending) > 0 {
			sb.WriteString("## Pending Tasks\n")
			for i, item := range pending {
				status := string(item.Status)
				active := ""
				if item.ActiveForm != "" {
					active = " (" + item.ActiveForm + ")"
				}
				sb.WriteString(fmt.Sprintf("%d. [%s] %s%s\n", i+1, status, item.Content, active))
			}
		}
	}

	// 2. Completed Tasks — the anti-replay guard. Tells the model what's done
	// so it doesn't re-execute completed work after compaction.
	if a.todoList != nil {
		completed := a.todoList.GetCompletedTasks()
		if len(completed) > 0 {
			sb.WriteString("\n## Completed Work (DO NOT RE-EXECUTE)\n")
			for _, item := range completed {
				active := ""
				if item.ActiveForm != "" {
					active = " (" + item.ActiveForm + ")"
				}
				sb.WriteString(fmt.Sprintf("- %s%s\n", item.Content, active))
			}
		}
	}

	// 3. Active task from toolStateTracker — what the model claims it's doing
	if a.toolStateTracker != nil {
		activeTask := a.toolStateTracker.GetActiveTask()
		if activeTask != "" {
			sb.WriteString(fmt.Sprintf("\n## Current Work\nActive task: %s\n", activeTask))
		}
	}

	// 4. In-progress task from TodoList — cross-reference with tracker
	if a.todoList != nil {
		inProgress := a.todoList.GetInProgressTask()
		if inProgress != "" {
			sb.WriteString(fmt.Sprintf("Currently working on: %s\n", inProgress))
		}
	}

	// 5. ToolStateTracker conclusions — key findings the agent claimed
	if a.toolStateTracker != nil {
		conclusions := a.toolStateTracker.GetConclusions()
		if len(conclusions) > 0 {
			sb.WriteString("\n## Key Findings\n")
			for _, c := range conclusions {
				sb.WriteString(fmt.Sprintf("- %s\n", c))
			}
		}
	}

	// 6. Error memory — errors encountered, extracted structurally from tool results
	if len(messages) > 0 {
		errorMem := a.extractErrorMemory(messages)
		if len(errorMem) > 0 {
			sb.WriteString("\n## Errors Encountered\n")
			for _, e := range errorMem {
				sb.WriteString(fmt.Sprintf("- %s\n", e))
			}
		}
	}

	return sb.String(), sb.Len() > 0
}

// extractErrorMemory extracts error messages from tool results structurally.
// Unlike upstream's "Errors and fixes" section (LLM-extracted), this uses
// deterministic regex-based extraction from tool_result content.
func (a *AgentLoop) extractErrorMemory(messages []anthropic.MessageParam) []string {
	seen := make(map[string]bool)
	var errors []string
	for _, msg := range messages {
		for _, block := range msg.Content {
			if block.OfToolResult == nil {
				continue
			}
			for _, tc := range block.OfToolResult.Content {
				if tc.OfText == nil {
					continue
				}
				text := tc.OfText.Text
				for _, line := range strings.Split(text, "\n") {
					line = strings.TrimSpace(line)
					if line == "" {
						continue
					}
					if strings.Contains(strings.ToLower(line), "error") {
						if len(line) > 200 {
							line = line[:200] + "..."
						}
						if !seen[line] {
							seen[line] = true
							errors = append(errors, line)
						}
					}
				}
			}
		}
	}
	// Keep only the most recent errors (upstream only keeps what LLM remembers)
	if len(errors) > 10 {
		errors = errors[len(errors)-10:]
	}
	return errors
}

// buildCompactSummaryMessage generates a structured summary message for the non-LLM
// compact path, matching upstream's getCompactUserSummaryMessage format.
// It uses deterministic structured state from TodoList + toolStateTracker for
// goal preservation (surpassing upstream's purely LLM-based goal extraction).
//
// messages and recentToolCalls are the pre-compacted conversation data.
// If messages is nil, BuildMessages() is called (backwards compat).
func (a *AgentLoop) buildCompactSummaryMessage(preTokens int, messages []anthropic.MessageParam, recentToolCalls []string) string {
	var sb strings.Builder

	// Preamble matching upstream's getCompactUserSummaryMessage
	sb.WriteString("This session is being continued from a previous conversation that ran out of context. The summary below covers the earlier portion of the conversation.\n\n")
	sb.WriteString(fmt.Sprintf("[compact: %d tokens compressed]\n", preTokens))

	// Include structured metadata from the full conversation before compaction.
	// This ensures the model sees an explicit inventory of files, tool calls,
	// and user messages — matching the LLM compact path's structured output.
	if messages == nil {
		messages = a.context.BuildMessages()
	}
	structuredMeta := entriesToSummaryTextForMessagesParams(messages)
	if structuredMeta != "" {
		sb.WriteString("\n## Structured context from compacted messages:\n")
		sb.WriteString(structuredMeta)
	}

	// Inject file content snapshot — the model's most recently read files with
	// their actual content at read time. This is the key anti-amnesia mechanism
	// matching upstream's post-compact file attachment restoration.
	// Uses registry cached content (what the model saw) not disk re-read.
	fileSnapshot := a.buildPreCompactFileSnapshot(10, 5000, 50000)
	if fileSnapshot != "" {
		sb.WriteString("\n")
		sb.WriteString(fileSnapshot)
	}

	// Inject the structured goal block — the key anti-amnesia mechanism.
	// This replaces upstream's purely LLM-based goal extraction with
	// deterministic structured state from TodoList + toolStateTracker.
	if goalBlock, hasContent := a.buildStructuredGoalBlock(messages); hasContent {
		sb.WriteString("\n")
		sb.WriteString(goalBlock)
	}

	sb.WriteString("\n(compact truncated the conversation — recent messages are preserved verbatim below)\n")

	// Anti-replay directive: explicit rules to prevent re-execution
	sb.WriteString("\n## Rules After Compaction\n")
	sb.WriteString("1. DO NOT re-execute any task listed in \"Completed Work\" — those are done.\n")
	sb.WriteString("2. Start from the first item in \"Pending Tasks\" that you have not yet completed.\n")

	// Transcript path for detail recovery.
	if tp := a.TranscriptPath(); tp != "" {
		sb.WriteString(fmt.Sprintf("3. If unsure what to do next, read the transcript at: %s.\n", tp))
	}
	sb.WriteString("4. Do NOT ask the user what to work on — you already know.\n")

	if tp := a.TranscriptPath(); tp != "" {
		sb.WriteString(fmt.Sprintf("\nIf you need specific details from before compaction (like exact code snippets, error messages, or content you generated), read the full transcript at: %s\n", tp))
	}

	sb.WriteString("\nRecent messages are preserved verbatim.\n\n")
	sb.WriteString("Continue the conversation from where it left off without asking the user any further questions. Resume directly — do not acknowledge the summary, do not recap what was happening, do not preface with \"I'll continue\" or similar. Pick up the last task as if the break never happened.")

	return sb.String()
}

// injectTruncationContinuation adds a CompactBoundary + summary after truncation-based
// recovery. Without this, the model receives truncated context with no directive to
// continue, causing it to re-execute old instructions or ask what to do.
// This matches the boundary+summary pattern used by the LLM compact path.
func (a *AgentLoop) injectTruncationContinuation(preTokens int) {
	if a.context == nil {
		return
	}
	a.context.AddCompactBoundary(CompactTriggerAuto, preTokens)
	summaryContent := a.buildCompactSummaryMessage(preTokens, nil, nil)
	a.context.AddSummary(summaryContent)
}

// collectRecentReadFiles returns a list of file paths from the most recent
// read_file tool calls in the conversation. This is used by the compression
// prompt to tell the LLM which files to preserve during compaction, preventing
// the read-compact-reread thrashing cycle.
func (a *AgentLoop) collectRecentReadFiles() []string {
	if a.context == nil {
		return nil
	}
	entries := a.context.Entries()
	var files []string
	seen := make(map[string]bool)
	// Walk backwards through entries, collecting read_file paths from the last 10 turns.
	turnCount := 0
	for i := len(entries) - 1; i >= 0 && turnCount < 10; i-- {
		entry := entries[i]
		if entry.role == "assistant" {
			turnCount++
		}
		if blocks, ok := entry.content.(ToolUseContent); ok {
			for _, b := range blocks {
				if b.OfToolUse == nil || b.OfToolUse.Name != "read_file" {
					continue
				}
				inputMap, _ := b.OfToolUse.Input.(map[string]any)
				if inputMap == nil {
					continue
				}
				if path, ok := inputMap["file_path"].(string); ok && path != "" && !seen[path] {
					seen[path] = true
					files = append(files, path)
				}
			}
		}
	}
	return files
}

// extractRecentToolCallsForSummary returns a list of recent tool call descriptions
// for inclusion in compaction summaries. This helps the model understand what
// work was done before compaction truncated the conversation history.
func (a *AgentLoop) extractRecentToolCallsForSummary(n int) []string {
	if a.context == nil {
		return nil
	}
	entries := a.context.Entries()
	var results []string
	count := 0
	for i := len(entries) - 1; i >= 0 && count < n; i-- {
		entry := entries[i]
		if blocks, ok := entry.content.(ToolUseContent); ok {
			for j := len(blocks) - 1; j >= 0 && count < n; j-- {
				if b := blocks[j].OfToolUse; b != nil {
					inputMap, _ := b.Input.(map[string]any)
					desc := b.Name
					if inputMap != nil {
						if pathVal, ok := inputMap["path"].(string); ok {
							desc += " " + pathVal
						} else if cmdVal, ok := inputMap["command"].(string); ok {
							shortCmd := cmdVal
							if len(shortCmd) > 80 {
								shortCmd = shortCmd[:80] + "..."
							}
							desc += " " + shortCmd
						} else if queryVal, ok := inputMap["query"].(string); ok {
							desc += " " + queryVal
						} else if promptVal, ok := inputMap["prompt"].(string); ok {
							short := promptVal
							if len(short) > 80 {
								short = short[:80] + "..."
							}
							desc += " " + short
						}
					}
					results = append([]string{desc}, results...)
					count++
				}
			}
		}
	}
	return results
}

// InjectRunningAgentStatus adds attachments for sub-agents that are still
// running in the background. This prevents the model from spawning duplicate
// agents after compaction. Matches upstream's createAsyncAgentAttachmentsIfNeeded.
func (a *AgentLoop) InjectRunningAgentStatus() {
	if a.agentTaskStore == nil {
		return
	}
	tasks := a.agentTaskStore.ListByStatus(tools.TaskRunning)
	for _, task := range tasks {
		statusLine := fmt.Sprintf(
			"[task_status] taskId: %s, type: local_agent, description: %s, status: running\nThis agent is still running in the background. Do NOT spawn a duplicate agent for this task.",
			task.ID, task.Description,
		)
		a.context.AddAttachment(statusLine)
	}
}

// runPerTurnMicroCompact executes lightweight micro-compact at the start of each
// turn to prevent old tool results from accumulating between full compactions.
// This matches upstream's per-turn microCompact call in query.ts.
//
// Progressive compression (MiMo-Code pattern):
//   - Pressure 0 (ratio < 0.50): no action
//   - Pressure 1 (ratio 0.50-0.70): soft-trim large outputs (keep head+tail)
//   - Pressure 2 (ratio >= 0.70): hard-clear old results (persist to disk)
func (a *AgentLoop) runPerTurnMicroCompact() {
	if !a.config.MicroCompactEnabled {
		return
	}

	keepRecent := a.config.MicroCompactKeepRecent
	if keepRecent <= 0 {
		keepRecent = 5
	}

	// Determine pressure level based on token ratio
	pressure := 0
	if a.compactor != nil {
		ctxMax := a.compactor.CompactThreshold() // 75% of context window
		if ctxMax > 0 {
			estimatedTokens := a.context.EstimatedTokens()
			ratio := float64(estimatedTokens) / float64(ctxMax)
			switch {
			case ratio >= 0.93: // 0.70 / 0.75 ≈ 0.93 of threshold
				pressure = 2
			case ratio >= 0.67: // 0.50 / 0.75 ≈ 0.67 of threshold
				pressure = 1
			}
		}
	}

	switch {
	case pressure >= 2:
		// Level 2: Hard-clear old tool results (persist to disk)
		cleared := a.context.MicroCompactEntries(keepRecent, a.config.MicroCompactPlaceholder, a.config.MicroCompactMinCharCount)
		if cleared > 0 {
			a.logDebug("\n[per-turn-micro-compact] Level 2 hard-clear: cleared %d old tool results\n", cleared)
		}

	case pressure >= 1:
		// Level 1: Soft-trim large tool outputs (keep head + tail)
		trimmed := a.context.SoftTrimEntries(keepRecent)
		if trimmed > 0 {
			a.logDebug("\n[per-turn-micro-compact] Level 1 soft-trim: trimmed %d large tool results\n", trimmed)
		}

	default:
		// Pressure 0: no action needed
	}
}

// tryCompaction attempts LLM-driven compaction, falling back to truncation.
// When session memory exists and has content, uses SM-compact (API-free compaction)
// to skip the LLM call and use session memory as the summary directly.

// decidePostResponseFold implements DeepSeek-Reasonix's multi-tier fold decision.
// After each API response, checks if token usage exceeds fold thresholds and
// decides the appropriate compaction strategy:
//   - ratio > FORCE_SUMMARY_THRESHOLD (0.80): exit with summary
//   - ratio > HISTORY_FOLD_AGGRESSIVE_THRESHOLD (0.78): aggressive fold (10% tail)
//   - ratio > HISTORY_FOLD_THRESHOLD (0.75): normal fold (20% tail)
func (a *AgentLoop) decidePostResponseFold(promptTokens int, ctxMax int) {
	if promptTokens <= 0 || ctxMax <= 0 {
		return
	}
	ratio := float64(promptTokens) / float64(ctxMax)

	if ratio > FORCE_SUMMARY_THRESHOLD {
		// Above 80%: exit with summary instead of folding
		a.logDebug("[multi-tier-fold] ratio=%.2f > %.2f (FORCE_SUMMARY): exiting with summary\n",
			ratio, FORCE_SUMMARY_THRESHOLD)
		// The loop will naturally complete; the user can see the summary.
		return
	}
	if ratio > HISTORY_FOLD_AGGRESSIVE_THRESHOLD {
		// Above 78%: aggressive fold with 10% tail budget
		a.logDebug("[multi-tier-fold] ratio=%.2f > %.2f (AGGRESSIVE): folding with 10%% tail\n",
			ratio, HISTORY_FOLD_AGGRESSIVE_THRESHOLD)
		a.tryCompaction()
		a.RunPostCompactCleanup()
		return
	}
	if ratio > HISTORY_FOLD_THRESHOLD {
		// Above 75%: normal fold with 20% tail budget
		a.logDebug("[multi-tier-fold] ratio=%.2f > %.2f (NORMAL): folding with 20%% tail\n",
			ratio, HISTORY_FOLD_THRESHOLD)
		a.tryCompaction()
		a.RunPostCompactCleanup()
		return
	}
	// Below 75%: no fold needed
}

// tryCompaction:
// When session memory exists and has content, uses SM-compact (API-free compaction)
// to skip the LLM call and use session memory as the summary directly.
func (a *AgentLoop) tryCompaction() {
	a.logDebug("[compact] tryCompaction called: est_tokens=%d\n", a.context.EstimatedTokens())

	// Phase 0: Micro-compact — progressive compression (MiMo-Code pattern)
	// Time-based trigger: only fire when gap since last assistant > threshold (default 60 min).
	// Pressure levels: 0=no action, 1=soft-trim (keep head+tail), 2=hard-clear (persist to disk)
	if a.config.MicroCompactEnabled && a.context.ShouldTimeBasedMicroCompact(a.config.MicroCompactGapMinutes) {
		keepRecent := a.config.MicroCompactKeepRecent
		if keepRecent <= 0 {
			keepRecent = 5
		}

		// Determine pressure level
		pressure := 0
		if a.compactor != nil {
			ctxMax := a.compactor.CompactThreshold()
			if ctxMax > 0 {
				estimatedTokens := a.context.EstimatedTokens()
				ratio := float64(estimatedTokens) / float64(ctxMax)
				switch {
				case ratio >= 0.93:
					pressure = 2
				case ratio >= 0.67:
					pressure = 1
				}
			}
		}

		switch {
		case pressure >= 2:
			// Level 2: Hard-clear old tool results (persist to disk)
			cleared := a.context.MicroCompactEntries(keepRecent, a.config.MicroCompactPlaceholder, a.config.MicroCompactMinCharCount)
			if cleared > 0 {
				a.logDebug("\n[micro-compact] Level 2 hard-clear: cleared %d old tool results\n", cleared)
			}

		case pressure >= 1:
			// Level 1: Soft-trim large tool outputs (keep head + tail)
			trimmed := a.context.SoftTrimEntries(keepRecent)
			if trimmed > 0 {
				a.logDebug("\n[micro-compact] Level 1 soft-trim: trimmed %d large tool results\n", trimmed)
			}

		default:
			// Pressure 0: no action needed
		}
	}

	if a.compactor == nil {
		preTokens := a.context.EstimatedTokens()
		// Capture messages and recent tool calls BEFORE compaction clears entries.
		// buildCompactSummaryMessage needs the full conversation context to generate
		// a useful summary; after CompactContext(), entries are truncated and the
		// summary would show "0 conversation turns with 0 tool calls".
		preCompactMessages := a.context.BuildMessages()
		preCompactToolCalls := a.extractRecentToolCallsForSummary(5)
		if a.context.CompactContext() {
			// CompactContext truncates messages but doesn't add a continuation directive.
			// Without one, the model sees an incomplete conversation and re-executes
			// historical instructions instead of continuing. Inject boundary + summary
			// with continuation directive, matching the SM-compact and LLM-compact paths.
			a.context.AddCompactBoundary(CompactTriggerAuto, preTokens)

			// Build a structured summary matching upstream's getCompactUserSummaryMessage
			// format. Without this, the model sees a bare "[compact: N tokens]" and
			// re-executes historical instructions instead of continuing.
			summaryContent := a.buildCompactSummaryMessage(preTokens, preCompactMessages, preCompactToolCalls)
			// DeepSeek-Reasonix pattern: prepend pinned content that must survive compaction
			if a.foldSummaryPin != nil {
				a.foldSummaryPin.SetSystemPrompt(a.context.SystemPrompt())
				pinPrompt := a.foldSummaryPin.BuildPinPrompt()
				if pinPrompt != "" {
					summaryContent = pinPrompt + "\n\n" + summaryContent
				}
			}
			a.context.AddSummary(summaryContent)

			if a.toolStateTracker != nil {
				a.toolStateTracker.OnCompaction()
			}
			a.InjectRunningAgentStatus()

			// Persist compaction boundary and summary to transcript for resume support.
			// Without this, --resume replays the full un-compacted history.
			if a.transcript != nil {
				_ = a.transcript.WriteCompact("compact_context", preTokens)
				_ = a.transcript.WriteSummary(summaryContent)
			}

			// Post-compact recovery: re-inject recently read files
			recoveredPaths := a.PostCompactRecovery(HookTriggerAuto, summaryContent)
			if a.toolStateTracker != nil {
				for _, path := range recoveredPaths {
					a.toolStateTracker.MarkFileFresh(path)
				}
				// NOTE: RunPostCompactCleanup() (called from PostCompactRecovery above)
				// already saves conclusions to session memory and clears them.
				// Do NOT save/clear conclusions here to avoid double-operation.
			}

			// Keep recent messages — preserve actual message objects with tool structure intact.
			// Use adaptive token-based calculation matching SM-compact (10K min, 5 text msgs, 40K max)
			// instead of fixed count. Fixed count is too small for large tool results and too
			// large for small text messages.
			a.context.KeepRecentMessagesAdaptive(10_000, 5, 40_000)
			a.context.ValidateToolPairing()
			a.context.FixRoleAlternation()

			// Calculate post-compact token count for cooldown tracking.
			// Without this, postCompactTokens stays 0 and the cooldown check never activates,
			// causing the next turn to immediately try compaction again even if it just ran.
			postCompactMessages := a.context.BuildMessages()
			postCompactTokens := estimateMessageParamsTokens(postCompactMessages)
			a.compactor.SetPostCompactTokens(postCompactTokens)

			// Auto-continue after compaction (MiMo-Code 1C)
			// Inject a synthetic user message so the agent continues working
			// instead of stopping and waiting for user input.
			a.context.AddUserMessage("Continue if you have next steps, or stop and ask for clarification if you are unsure how to proceed.")
		}
		return
	}

	// Execute pre-compact hooks before any compaction action.
	// Hooks can inject custom instructions that affect the compaction summary.
	var preCompactInst string
	if a.hooks != nil {
		hookInput := PreCompactInput{
			Trigger:            HookTriggerAuto,
			CustomInstructions: "",
		}
		if hookResult, err := a.hooks.ExecutePreCompactHooks(hookInput); err == nil {
			preCompactInst = hookResult.CustomInstructions
		}
	}

	// Phase 1: SM-compact — use session memory as summary instead of calling LLM API.
	// This is the preferred path when memory is available: saves an LLM API call
	// and leverages incrementally collected session memory as the context summary.
	// Uses the full structured session_memory.md content (10-section template) rather
	// than the flattened FormatForPromptCompact output — the structured template
	// matches upstream's getSessionMemoryContent() behavior and provides much richer
	// context for post-compact recovery.
	if a.config.SessionMemory != nil {
		// Wait for any in-progress session memory extraction to complete.
		// Matching upstream's waitForSessionMemoryExtraction() in trySessionMemoryCompaction.
		// If extraction is stale (>60s old) or timed out, proceed anyway.
		if a.extractionState != nil {
			a.extractionState.WaitForExtraction(15 * time.Second)
		}

		var smContent string

		// Try reading the actual session_memory.md file (structured 10-section template)
		memoryPath := filepath.Join(a.config.ProjectDir, ".claude", "session_memory.md")
		if data, err := os.ReadFile(memoryPath); err == nil {
			content := strings.TrimSpace(string(data))
			// Only use if content has actual user-written content (not just the template)
			if content != "" && !IsSessionMemoryTemplateOnly(content) {
				smContent = content
			}
		}

		// Fall back to the flattened FormatForPromptCompact if file doesn't exist or is empty
		if smContent == "" {
			smContent = a.config.SessionMemory.FormatForPromptCompact()
		}

		if smContent != "" {
			a.trySMCompact(smContent, preCompactInst)
			// Mark system prompt dirty after compaction
			if a.config.cachedPrompt != nil {
				a.config.cachedPrompt.MarkDirty()
			}
			return
		}
	}

	// Phase 2: LLM-driven compaction (existing path)
	a.tryLLMCompaction(preCompactInst)

	// Mark system prompt dirty after compaction
	if a.config.cachedPrompt != nil {
		a.config.cachedPrompt.MarkDirty()
	}
}

// trySMCompact performs compaction using session memory as the summary,
// skipping the LLM API call entirely. Inspired by the official Claude Code
// SM-compact mechanism (sessionMemoryCompact.ts).
func (a *AgentLoop) trySMCompact(sessionMemoryContent string, preCompactInst string) {
	messages := a.context.BuildMessages()
	preTokens := estimateMessageParamsTokens(messages)

	if !a.compactor.ShouldCompact(messages) {
		return // Not enough tokens to justify compaction
	}

	// Cooldown: skip if tokens haven't grown 50% since last compaction
	// (handled inside ShouldCompact, but double-check pre-compact tokens)
	a.compactor.mu.Lock()
	postTokens := a.compactor.postCompactTokens
	a.compactor.mu.Unlock()
	if postTokens > 0 {
		cooldownThreshold := postTokens + postTokens/2
		if preTokens < cooldownThreshold {
			return // Still in cooldown period
		}
	}

	// Advance compaction epoch BEFORE clearing context — marks all tracked items as stale.
	// After this point, items from the previous epoch are marked "cleared from context".
	if a.toolStateTracker != nil {
		a.toolStateTracker.OnCompaction()
	}

	// Clear stale state entries from session memory — the old state context is no
	// longer valid after compaction, and the new compaction will write fresh state.
	if a.config.SessionMemory != nil {
		a.config.SessionMemory.ClearStateEntries()
	}

	// Build structured metadata from the messages being compacted.
	// This ensures the model sees an explicit inventory of files, tool calls,
	// and user messages even when session memory only has high-level notes.
	// Matches upstream's structured_meta injection in do_compact_llm_call.
	structuredMeta := entriesToSummaryTextForMessagesParams(messages)
	if structuredMeta != "" {
		structuredMeta = "\n\n## Structured context from compacted messages:\n" + structuredMeta
	}

	// Write structured worklog and error entries to session memory before compaction.
	// This populates the previously empty Worklog and Errors sections in session_memory.md
	// with concrete entries extracted from the messages being compacted.
	if a.config.SessionMemory != nil && structuredMeta != "" {
		worklogEntries := extractWorklogFromStructuredMeta(structuredMeta)
		for _, entry := range worklogEntries {
			a.config.SessionMemory.AddNote("worklog", entry, "auto")
		}
		errorEntries := extractErrorsFromMessagesParams(messages)
		for _, entry := range errorEntries {
			a.config.SessionMemory.AddNote("error", entry, "auto")
		}
	}

	// Inject file content snapshot — reduced size (5 files, 20K total)
	// to avoid bloating the summary. The model can re-read files it needs
	// after compaction via PostCompactRecovery.
	fileSnapshot := a.buildPreCompactFileSnapshot(5, 5000, 20000)
	if fileSnapshot != "" {
		fileSnapshot = "\n" + fileSnapshot
	}

	// Format the session memory as a compact summary
	// Cap session memory content at ~40K tokens to prevent context overflow.
	// Matches upstream's DEFAULT_SM_COMPACT_CONFIG.maxTokens = 40_000.
	// Uses per-section truncation (truncateSessionMemoryForCompact) to preserve
	// section headers while truncating oversized sections.
	const maxSessionMemoryTokens = 40_000
	smTokens := EstimateTokens(sessionMemoryContent)
	smContentForSummary := sessionMemoryContent
	if smTokens > maxSessionMemoryTokens {
		smContentForSummary = truncateSessionMemoryForCompact(sessionMemoryContent, maxSessionMemoryTokens)
		a.logDebug("\n[sm-compact] Session memory truncated: %d tokens -> %d token limit\n", smTokens, maxSessionMemoryTokens)
	}

	boundaryText := fmt.Sprintf("[SM-compact: %d tokens compressed, session memory used as summary]", preTokens)
	// Match upstream's getCompactUserSummaryMessage: add transcript path for
	// detail recovery, recentMessagesPreserved notice, and continuation instruction.
	summaryContent := "This session is being continued from a previous conversation that ran out of context. The summary below covers the earlier portion of the conversation.\n\n" + boundaryText + "\n\n" + smContentForSummary + structuredMeta + fileSnapshot
	if tp := a.TranscriptPath(); tp != "" {
		summaryContent += fmt.Sprintf("\n\nIf you need specific details from before compaction (like exact code snippets, error messages, or content you generated), read the full transcript at: %s", tp)
	}

	// Inject deterministic goal block — pending/completed tasks, current work,
	// key findings, and errors. SM-compact uses session memory as the summary
	// which has no explicit task tracking, so the structured goal block is the
	// model's only signal for what's pending vs done.
	if goalBlock, hasContent := a.buildStructuredGoalBlock(messages); hasContent {
		summaryContent += "\n\n" + goalBlock
	}

	summaryContent += "\n\nRecent messages are preserved verbatim.\n\nContinue the conversation from where it left off without asking the user any further questions. Resume directly — do not acknowledge the summary, do not recap what was happening, do not preface with \"I'll continue\" or similar. Pick up the last task as if the break never happened."

	// Anti-replay directive: explicit rules to prevent re-execution
	summaryContent += "\n\n## Rules After Compaction\n"
	summaryContent += "1. DO NOT re-execute any task listed in \"Completed Work\" — those are done.\n"
	summaryContent += "2. Start from the first item in \"Pending Tasks\" that you have not yet completed.\n"
	if tp := a.TranscriptPath(); tp != "" {
		summaryContent += fmt.Sprintf("3. If unsure what to do next, read the transcript at: %s.\n", tp)
	}
	summaryContent += "4. Do NOT ask the user what to work on — you already know.\n"

	if preCompactInst != "" {
		summaryContent += "\n\n## Custom instructions for this compaction:\n" + preCompactInst
	}

	a.logDebug("\n[sm-compact] Using session memory as summary (%d tokens -> ~%d tokens)\n",
		preTokens, EstimateTokens(summaryContent)+6)

	// Inject boundary + summary into context
	a.context.AddCompactBoundary(CompactTriggerSMCompact, preTokens)
	a.context.AddSummary(summaryContent)

	// Persist compaction boundary and summary to transcript for resume support.
	if a.transcript != nil {
		_ = a.transcript.WriteCompact("sm-compact", preTokens)
		_ = a.transcript.WriteSummary(summaryContent)
	}

	// Update session memory with compaction state
	if a.config.SessionMemory != nil {
		a.config.SessionMemory.AddNote("state", fmt.Sprintf("Compaction (sm-compact): %d tokens compressed", preTokens), "auto")
	}

	// Phase 2: Keep recent messages — preserve with tool structure intact.
	// Run BEFORE post-compact recovery so attachments appear AFTER kept messages.
	// KeepRecentMessagesAdaptive uses token-based adaptive calculation instead of fixed count,
	// matching upstream's calculateMessagesToKeepIndex:
	//   - minTokens: enough context for recovery (~10K tokens)
	//   - minTextMsgs: ensure at least 5 text messages are visible post-compact
	//   - maxTokens: cap tail at ~40K to avoid bloating context with tool results
	a.context.KeepRecentMessagesAdaptive(10_000, 5, 40_000)

	// Fix message structure after KeepRecentMessages: remove orphaned tool_results
	// (whose tool_use was in the summarized portion) and merge consecutive same-role
	// messages. Without this, the API returns error 2013 for invalid message structure.
	a.context.ValidateToolPairing()
	a.context.FixRoleAlternation()

	// Phase 3: Post-compact recovery — re-inject critical context
	recoveredPaths := a.PostCompactRecovery(HookTriggerSM, summaryContent)

	// Phase 3b: Inject running agent status so model doesn't spawn duplicates
	a.InjectRunningAgentStatus()

	// Mark recovered files as fresh (content is back in context).
	// Also mark ALL tracked files as fresh if no specific recovery was done
	// (the summary now contains the distilled knowledge).
	// RunPostCompactCleanup (called from PostCompactRecovery) handles ClearConclusions.
	if a.toolStateTracker != nil {
		for _, path := range recoveredPaths {
			a.toolStateTracker.MarkFileFresh(path)
		}
	}

	// Calculate real post-compact token count for cooldown
	actualMessages := a.context.BuildMessages()
	actualPostTokens := estimateMessageParamsTokens(actualMessages)
	a.compactor.SetPostCompactTokens(actualPostTokens)

	// Post-compact threshold check: if SM-compact didn't reduce tokens below
	// the autocompact threshold, fall back to LLM compaction. This matches
	// upstream's autoCompactThreshold check in trySessionMemoryCompaction.
	compactThreshold := a.compactor.CompactThreshold()
	if actualPostTokens >= compactThreshold {
		a.logDebug("\n[sm-compact] Post-compact tokens (%d) still above threshold (%d), falling back to LLM compaction\n",
			actualPostTokens, compactThreshold)
		// Undo SM-compact and fall back to LLM compaction.
		// tryLLMCompaction will re-check ShouldCompact and may skip if tokens
		// are too low, which is the correct behavior (context pressure is gone).
		a.tryLLMCompaction("")
		return
	}

	// Track lastSummarizedMessageUUID for incremental SM-compact.
	// The compaction boundary is inserted before summary, so the boundary's
	// UUID marks the end of the summarized portion. Subsequent compactions
	// will only compact forward from this point.
	// Mirrors upstream's setLastSummarizedMessageId() called after compaction.
	if a.config.SessionMemory != nil {
		a.config.SessionMemory.SetLastSummarizedMessageUUID(a.context.LastCompactBoundaryUUID())
	}
}

// tryLLMCompaction performs LLM-driven compaction using the insert-then-compress
// pattern from openclacky: inject a compression instruction into the existing
// conversation, make a single API call (reusing cached system prompt + tools),
// parse the <topics> + <summary> response, and rebuild context.
// Returns true if compaction was performed.
func (a *AgentLoop) tryLLMCompaction(preCompactInst string) {
	messages := a.context.BuildMessages()

	// Advance compression level for progressive summarization.
	// Higher levels produce shorter summaries, preventing summary bloat
	// across multiple compactions. Inspired by openclacky's hierarchical
	// summarization (Level 1=full, 2=concise, 3=minimal, 4+=ultra-minimal).
	level := a.context.NextCompressionLevel()

	// Chunk archival: archive pre-compaction messages to disk for
	// on-demand recall. Matches openclacky's chunk archival pattern.
	var chunkPath string
	var prevChunksIndex string
	if a.config.ProjectDir != "" {
		chunkMsgs := messagesToChunkMessages(messages)
		chunkPath = archiveChunkMessages(a.config.ProjectDir, level, chunkMsgs)
		// Discover existing chunks and build index for the summary
		prevChunksIndex = tools.BuildPreviousChunksIndex(
			tools.ListChunks(a.config.ProjectDir, ""))
	}

	// ─── Insert-then-compress (openclacky pattern) ────────────────────────────
	// Instead of making a separate API call with its own prompt, inject the
	// compression instruction into the conversation messages and make the API
	// call with the instruction already embedded. This reuses the existing
	// prompt cache prefix (system prompt + tools + prior messages are cached,
	// only the instruction text itself is new tokens).
	//
	// The instruction is built and appended as the final user message:

	// Collect recent file reads so the compression instruction tells LLM to preserve them.
	recentFiles := a.collectRecentReadFiles()
	compressPrompt := buildCompressionPrompt(level, recentFiles)
	finalMsgs := make([]anthropic.MessageParam, len(messages)+1)
	copy(finalMsgs, messages)
	finalMsgs[len(messages)] = anthropic.MessageParam{
		Role: anthropic.MessageParamRoleUser,
		Content: []anthropic.ContentBlockParamUnion{
			{OfText: &anthropic.TextBlockParam{Text: SystemInjectedPrefix + compressPrompt}},
		},
	}

	// Call the inline compaction API (no additional prompt appended — the
	// instruction is already in the messages as the final user message).
	result, err := doInlineCompactLLMCall(finalMsgs, a.config.Model, a.config.APIKey, a.config.BaseURL, a.context.SystemPrompt(), a.TranscriptPath(), level)

	if err != nil || result.Summary == "" {
		// Fall back to the existing separate-call path
		a.logDebug("\n[Compaction] inline compaction failed or empty, falling back to separate call\n")
		summary, performed := a.compactor.Compact(messages, a.config.Model, a.config.APIKey, a.config.BaseURL, a.context.SystemPrompt(), a.TranscriptPath())
		if performed && summary != "" {
			// Augment summary with chunk path reference and previous chunks index
			if chunkPath != "" {
				summary = augmentSummaryWithChunk(summary, chunkPath)
			}
			if prevChunksIndex != "" {
				summary += prevChunksIndex
			}
			// Advance compaction epoch BEFORE clearing context — marks all tracked items as stale.
			if a.toolStateTracker != nil {
				a.toolStateTracker.OnCompaction()
			}
			// Clear stale state entries from session memory - the old state context is no
			// longer valid after compaction, and the new compaction will write fresh state.
			if a.config.SessionMemory != nil {
				a.config.SessionMemory.ClearStateEntries()
			}

			// Inject boundary marker and summary into context
			preTokens := a.context.EstimatedTokens()

			// Capture discovered tool names before compaction — the summary doesn't
			// preserve tool_reference blocks, so post-compact recovery needs this
			// to keep sending already-loaded deferred tool schemas to the API.
			discoveredTools := collectDiscoveredToolNames(a.context)

			a.context.AddCompactBoundary(CompactTriggerAuto, preTokens,
				func(bc *CompactBoundaryContent) {
					if len(discoveredTools) > 0 {
						bc.PreCompactDiscoveredTools = discoveredTools
					}
				},
			)
			a.context.AddSummary(summary)

			// Anti-replay rules: injected as a separate system message so they
			// survive compaction even when the LLM summary is compressed further.
			// This matches the SM-compact and truncation paths which include the
			// same rules.
			a.context.AddAntiReplayRules()

			// Inject structured goal block — deterministic task state from TodoList
			// + toolStateTracker. Without this, the LLM-generated summary may lack
			// explicit completed/pending task distinction.
			if goalBlock, hasContent := a.buildStructuredGoalBlock(messages); hasContent {
				a.context.AddGoalBlock(goalBlock)
			}

			if preCompactInst != "" {
				a.context.AddSummary("\n\n## Custom instructions for this compaction:\n" + preCompactInst)
			}

			// Persist compaction boundary and summary to transcript for resume support.
			// Without this, --resume replays the full un-compacted history.
			if a.transcript != nil {
				_ = a.transcript.WriteCompact("auto", preTokens)
				_ = a.transcript.WriteSummary(summary)
			}

			// Save compaction state to session memory — store a compact token count marker,
			// not the full summary text (which bloats session memory for future sessions).
			if a.config.SessionMemory != nil {
				a.config.SessionMemory.AddNote("state", fmt.Sprintf("Compaction (auto): %d tokens compressed", preTokens), "auto")
				// Track lastSummarizedMessageUUID for incremental SM-compact.
				// Subsequent compactions will only compact forward from this point.
				a.config.SessionMemory.SetLastSummarizedMessageUUID(a.context.LastCompactBoundaryUUID())
			}

			// Phase 2: Keep recent messages — preserve with tool structure intact.
			// Run BEFORE post-compact recovery so attachments appear AFTER kept messages.
			// Use adaptive token-based calculation (10K min, 5 text msgs, 40K max)
			// matching SM-compact path. Fixed count is too small for large tool results
			// and too large for small text messages.
			a.context.KeepRecentMessagesAdaptive(10_000, 5, 40_000)

			// Fix message structure after KeepRecentMessages: remove orphaned tool_results
			// (whose tool_use was in the summarized portion) and merge consecutive same-role
			// messages. Without this, the API returns error 2013 for invalid message structure.
			a.context.ValidateToolPairing()
			a.context.FixRoleAlternation()

			// Phase 3: Post-compact recovery — re-inject critical context
			recoveredPaths := a.PostCompactRecovery(HookTriggerAuto, summary)

			// Phase 3b: Inject running agent status so model doesn't spawn duplicates
			a.InjectRunningAgentStatus()

			// Mark recovered files as fresh (content is back in context).
			// NOTE: RunPostCompactCleanup() (called from PostCompactRecovery above)
			// already saves conclusions to session memory and clears them.
			// The outer conditional that checked len(recoveredPaths) was removed —
			// conclusions should ALWAYS be saved before clearing, regardless of
			// whether files were recovered (conclusions contain semantic knowledge
			// that file content doesn't capture, like "the bug was in line 42").
			if a.toolStateTracker != nil {
				for _, path := range recoveredPaths {
					a.toolStateTracker.MarkFileFresh(path)
				}
			}

			// Rebuild messages from the actual context (summary + attachments + any tail entries)
			// and calculate the real post-compact token count for cooldown.
			actualMessages := a.context.BuildMessages()
			postTokens := estimateMessageParamsTokens(actualMessages)
			a.compactor.SetPostCompactTokens(postTokens)
			return
		}

		// LLM compaction was not performed (not needed or disabled).
		// Do NOT fall through to CompactContext() -- the LLM compactor's
		// ShouldCompact() check already determined that compaction isn't needed.
		return
	}

	// ─── Inline compaction succeeded ──────────────────────────────────────────
	summary := result.Summary
	topics := result.Topics

	// Augment summary with chunk path reference and previous chunks index
	if chunkPath != "" {
		summary = augmentSummaryWithChunk(summary, chunkPath)
	}
	if prevChunksIndex != "" {
		summary += prevChunksIndex
	}

	// Advance compaction epoch BEFORE clearing context — marks all tracked items as stale.
	if a.toolStateTracker != nil {
		a.toolStateTracker.OnCompaction()
	}
	// Clear stale state entries from session memory - the old state context is no
	// longer valid after compaction, and the new compaction will write fresh state.
	if a.config.SessionMemory != nil {
		a.config.SessionMemory.ClearStateEntries()
	}

	// Inject boundary marker and summary into context
	preTokens := a.context.EstimatedTokens()

	// Capture discovered tool names before compaction — the summary doesn't
	// preserve tool_reference blocks, so post-compact recovery needs this
	// to keep sending already-loaded deferred tool schemas to the API.
	discoveredTools := collectDiscoveredToolNames(a.context)

	a.context.AddCompactBoundary(CompactTriggerAuto, preTokens,
		func(bc *CompactBoundaryContent) {
			if len(discoveredTools) > 0 {
				bc.PreCompactDiscoveredTools = discoveredTools
			}
		},
	)

	// Use CompressedSummaryContent instead of raw SummaryContent — this preserves
	// topics metadata and chunk path for on-demand recall.
	a.context.AddCompressedSummary(summary, topics, chunkPath)

	// Anti-replay rules: injected as a separate system message so they
	// survive compaction even when the LLM summary is compressed further.
	a.context.AddAntiReplayRules()

	// Inject structured goal block — deterministic task state from TodoList
	// + toolStateTracker. Without this, the LLM-generated summary may lack
	// explicit completed/pending task distinction.
	if goalBlock, hasContent := a.buildStructuredGoalBlock(messages); hasContent {
		a.context.AddGoalBlock(goalBlock)
	}

	if preCompactInst != "" {
		a.context.AddSummary("\n\n## Custom instructions for this compaction:\n" + preCompactInst)
	}

	// Persist compaction boundary and summary to transcript for resume support.
	// Without this, --resume replays the full un-compacted history.
	if a.transcript != nil {
		_ = a.transcript.WriteCompact("auto", preTokens)
		_ = a.transcript.WriteSummary(summary)
	}

	// Save compaction state to session memory
	if a.config.SessionMemory != nil {
		a.config.SessionMemory.AddNote("state", fmt.Sprintf("Compaction (auto, inline): %d tokens compressed", preTokens), "auto")
		a.config.SessionMemory.SetLastSummarizedMessageUUID(a.context.LastCompactBoundaryUUID())
	}

	// Phase 2: Keep recent messages
	a.context.KeepRecentMessagesAdaptive(10_000, 5, 40_000)

	// Fix message structure after KeepRecentMessages
	a.context.ValidateToolPairing()
	a.context.FixRoleAlternation()

	// Phase 3: Post-compact recovery — re-inject critical context
	recoveredPaths := a.PostCompactRecovery(HookTriggerAuto, summary)

	// Phase 3b: Inject running agent status so model doesn't spawn duplicates
	a.InjectRunningAgentStatus()

	// Mark recovered files as fresh
	if a.toolStateTracker != nil {
		for _, path := range recoveredPaths {
			a.toolStateTracker.MarkFileFresh(path)
		}
	}

	// Rebuild messages and calculate post-compact token count
	actualMessages := a.context.BuildMessages()
	postTokens := estimateMessageParamsTokens(actualMessages)
	a.compactor.SetPostCompactTokens(postTokens)

	fmt.Fprintf(os.Stderr, "\n[Compaction] inline: %d messages compressed, topics=[%s]\n",
		len(messages), topics)
}

// messagesToChunkMessages converts API message params into simplified
// ChunkMessage format for archival to disk.
func messagesToChunkMessages(messages []anthropic.MessageParam) []tools.ChunkMessage {
	var result []tools.ChunkMessage
	for _, msg := range messages {
		role := string(msg.Role)
		var content strings.Builder
		for _, block := range msg.Content {
			if block.OfText != nil {
				content.WriteString(block.OfText.Text)
			} else if block.OfToolResult != nil {
				for _, cb := range block.OfToolResult.Content {
					if cb.OfText != nil {
						content.WriteString(cb.OfText.Text)
					}
				}
			} else if block.OfToolUse != nil {
				fmt.Fprintf(&content, "_Tool call: %s_", block.OfToolUse.Name)
			}
		}
		text := content.String()
		if text == "" {
			continue
		}
		result = append(result, tools.ChunkMessage{
			Role:    role,
			Content: text,
		})
	}
	return result
}

// archiveChunkMessages writes conversation messages to a chunk .md file
// on disk and returns the path. Returns empty string on error or if
// projectDir is empty. Matches openclacky's chunk archival pattern.
func archiveChunkMessages(projectDir string, level int, msgs []tools.ChunkMessage) string {
	if projectDir == "" || len(msgs) == 0 {
		return ""
	}
	// Use the tool-results directory's parent as the base for chunks
	chunkDir := filepath.Join(projectDir, "chunks")
	chunkIndex := tools.NextChunkIndex(chunkDir, "")
	path, err := tools.ArchiveChunk(chunkDir, "", chunkIndex, level, "", msgs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "\n[WARN] Chunk archival failed: %v\n", err)
		return ""
	}
	return path
}

// augmentSummaryWithChunk appends a chunk path reference to the summary,
// enabling on-demand recall via file_reader. Matches openclacky's pattern.
func augmentSummaryWithChunk(summary, chunkPath string) string {
	return summary + fmt.Sprintf("\n\n---\nCurrent chunk archived at: %s\nUse file_reader to recall details from this chunk.", chunkPath)
}

// isLocalEndpoint detects if the base URL points to a local provider.
func isLocalEndpoint(baseURL string) bool {
	lower := strings.ToLower(baseURL)
	return strings.Contains(lower, "localhost") ||
		strings.Contains(lower, "127.0.0.1") ||
		strings.Contains(lower, "0.0.0.0") ||
		strings.Contains(lower, "::1") ||
		strings.HasSuffix(lower, ".local") ||
		strings.Contains(lower, "://localhost:")
}

// estimateMessageTokens roughly estimates token count (~4 chars per token).
func estimateMessageTokens(messages []anthropic.MessageParam) int {
	totalChars := 0
	for _, msg := range messages {
		for _, block := range msg.Content {
			if block.OfText != nil {
				totalChars += len(block.OfText.Text)
			}
		}
	}
	return totalChars / 4
}

// runSessionMemoryExtraction runs a forked agent to update session_memory.md.
// It captures the parent's cache-safe params and uses a restricted canUseTool
// (only edit_file on the session memory file). Runs asynchronously in a goroutine.
func (a *AgentLoop) runSessionMemoryExtraction() {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(os.Stderr, "[session-memory] extraction panic: %v\n", r)
			// Clear in-progress flag even on panic
			if a.extractionState != nil {
				a.extractionState.MarkExtracted(int64(a.context.EstimatedTokens()))
			}
		}
	}()

	// Mark extraction as in-progress so SM-compact can wait for it.
	// This matches upstream's waitForSessionMemoryExtraction pattern.
	if a.extractionState != nil {
		a.extractionState.MarkExtractionInProgress()
	}

	sm := a.config.SessionMemory
	if sm == nil {
		return
	}

	// Capture cache-safe params from the current state
	messages := a.context.BuildMessages()
	messages = NormalizeAPIMessages(messages)
	cacheParams := CaptureCacheSafeParams(
		a.context.SystemPrompt(),
		a.config.Model,
		a.registry,
		messages,
	)

	// Read current session memory content
	memoryPath := filepath.Join(a.config.ProjectDir, ".claude", "session_memory.md")
	currentContent, _ := os.ReadFile(memoryPath)
	if len(currentContent) == 0 {
		currentContent = []byte(defaultSessionMemoryTemplate)
	}

	// Build the extraction prompt
	prompt := sessionMemoryUpdatePrompt(memoryPath, string(currentContent))
	forkMessages := []anthropic.MessageParam{
		anthropic.MessageParam{
			Role:    anthropic.MessageParamRoleUser,
			Content: []anthropic.ContentBlockParamUnion{{OfText: &anthropic.TextBlockParam{Text: prompt}}},
		},
	}

	// Run forked agent
	cfg := ForkedAgentConfig{
		CacheSafeParams:    cacheParams,
		ForkMessages:       forkMessages,
		CanUseTool:         createMemoryFileCanUseTool(memoryPath),
		MaxTokens:          8192,
		QuerySource:        "session_memory",
		MaxTurns:           5,
		Registry:           a.registry,
		ProjectDir:         a.config.ProjectDir,
		SkipParentMessages: true,
		Client:             a.client,
	}

	_, err := RunForkedAgent(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[session-memory] extraction error: %v\n", err)
		// Clear in-progress flag on error so SM-compact does not block forever.
		if a.extractionState != nil {
			a.extractionState.MarkExtracted(int64(a.context.EstimatedTokens()))
		}
		return
	}

	// Mark extraction complete and record token count
	if a.extractionState != nil {
		a.extractionState.MarkExtracted(int64(a.context.EstimatedTokens()))
	}
}

// runPhase4Extraction extracts user instructions and discoveries from the
// conversation context and saves them to session memory. This runs alongside
// the existing session memory extraction to capture structured knowledge.
func (a *AgentLoop) runPhase4Extraction() {
	sm := a.config.SessionMemory
	if sm == nil {
		return
	}

	// Build conversation messages from context
	entries := a.context.Entries()
	var messages []ConversationMessage
	for _, entry := range entries {
		role := ""
		content := ""
		switch v := entry.content.(type) {
		case TextContent:
			role = entry.role
			content = string(v)
		case SummaryContent:
			role = "assistant"
			content = string(v)
		}
		if role != "" && content != "" {
			messages = append(messages, ConversationMessage{
				Role:    role,
				Content: content,
			})
		}
	}

	if len(messages) == 0 {
		return
	}

	// Extract and save
	count := sm.ExtractAndSave(messages)
	if count > 0 {
		a.logDebug("[phase4] Extracted %d items (instructions/discoveries)\n", count)
	}
}

