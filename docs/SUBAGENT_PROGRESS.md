# Go Version Sub-Agent Architecture — Progress & Analysis

## Overview

This document tracks the implementation progress of the advanced sub-agent system for `miniClaudeCode-go`.

## Research Baseline

Claude Code's official Agent sub-agent implementation (`AgentTool.tsx` from `@anthropics/claude-code`) provides these features:

### Core Architecture
- **AgentSpawnFunc callback**: separates tool definition from agent lifecycle
- **Normal mode**: fresh context, isolated conversation history
- **Fork mode**: inherits parent's full conversation + rendered system prompt (prompt cache optimization)
- **Sync vs Async**: sync blocks parent; async launches goroutine and returns immediately

### Lifecycle Management
- **AppState.tasks**: `Map<agentId, LocalAgentTaskState>` keyed by `agentId`
- **LocalAgentTaskState**: id, status (pending/running/completed/failed/killed), result, startTime, lastResultUpdate, abortController
- **30s grace eviction**: background tasks auto-evicted 30s after completion
- **AbortController**: sync agents share parent's context; async agents get independent controllers

### Result Feedback
- **`<task-notification>` XML injection**: async agents push completion XML to command queue
- **enqueueAgentNotification equivalent**: formats `<task-notification>agentId</agent-notification>` with status
- **Auto-injection**: parent LLM sees result as user message on next turn
- **SendMessage tool**: queue messages to running agents via `pendingMessages[]`; auto-resumes stopped agents from disk transcript

### Prompt Engineering
- **Dynamic prompt assembly**: `getSystemPrompt()` → `enhanceSystemPromptWithEnvDetails()` → env info + notes + skill guidance
- **Agent definitions**: each type has getSystemPrompt(), tools, disallowedTools, model, permissionMode, maxTurns
- **Specialized types**: general-purpose, Explore (read-only), Plan (read-only), verification, statusline-setup

---

## Implementation Status

### Completed Features

| Feature | File | Status |
|---------|------|--------|
| AgentTool struct + InputSchema | `tools/agent_tool.go` | ✅ Done |
| SpawnFunc callback pattern | `tools/agent_tool.go` | ✅ Done |
| Sync sub-agent execution | `agent_sub.go` | ✅ Done |
| Async sub-agent (background goroutine) | `agent_sub.go` | ✅ Done |
| Task state tracking (TaskStore) | `agent_task.go` | ✅ Done |
| Result notification injection | `agent_loop.go` | ✅ Done |
| Tool filtering (no recursive agent) | `agent_sub.go` | ✅ Done |
| Specialized Agent types (Explore, Plan, Verify) | `agent_sub.go` | ✅ Done |
| Fork mode (inherit parent context) | `agent_sub.go` | ✅ Done |
| SendMessage tool | `tools/send_message_tool.go` | ✅ Done |
| Notification drain in REPL | `main.go` | ✅ Done |
| buildSubAgentConfig | `agent_sub.go` | ✅ Done |
| buildSubAgentRegistry (with AgentType) | `agent_sub.go` | ✅ Done |
| buildSubAgentSystemPrompt (with AgentType) | `agent_sub.go` | ✅ Done |
| createChildAgentLoop | `agent_sub.go` | ✅ Done |
| registerAgentTool() in NewAgentLoop | `agent_loop.go` | ✅ Done |
| registerSendMessageTool() | `agent_loop.go` | ✅ Done |
| activeSubAgents atomic counter | `agent_loop.go` | ✅ Done |
| cloneContextForFork | `agent_sub.go` | ✅ Done |
| SendMessageToSubAgent | `agent_sub.go` | ✅ Done |
| GetSubAgentStatus | `agent_sub.go` | ✅ Done |
| EnqueueAgentNotification | `agent_loop.go` | ✅ Done |
| DrainNotifications | `agent_loop.go` | ✅ Done |

### Not Yet Implemented

| Feature | Priority | Complexity | Status |
|---------|----------|------------|--------|
| AbortController (cancel running agents) | MEDIUM | LOW | ✅ Done |
| 30s grace eviction for completed tasks | LOW | LOW | ✅ Done |
| AgentSummaryService (periodic progress) | LOW | HIGH | 🔲 Pending |
| Notification injection into LLM context (not just REPL) | MEDIUM | MEDIUM | 🔲 Pending |
| Resume stopped agents from disk transcript | LOW | HIGH | 🔲 Pending |

---

## File Structure

```
E:\Git\miniClaudeCode-go-github/
├── agent_sub.go              # Sub-agent engine (SpawnSubAgent, fork, agent types)
├── agent_task.go             # Task state tracking (TaskStore, TaskState, TaskStatus)
├── agent_task_test.go        # Tests for TaskStore
├── agent_loop.go             # AgentLoop (taskStore, notificationChan, registerAgentTool)
├── context.go                # ConversationContext (EntryContent types)
├── config.go                 # Config (SubAgentMaxTurns, SubAgentEnabled)
├── main.go                   # REPL (DrainNotifications before prompt)
└── tools/
    ├── agent_tool.go         # AgentTool (SpawnFunc callback, InputSchema, Execute)
    └── send_message_tool.go  # SendMessageTool (query status, queue messages)
```

---

## Architecture Decisions

### Decision 1: SpawnFunc Callback Pattern
- `tools.AgentTool` defines `AgentSpawnFunc` callback
- `AgentLoop.SpawnSubAgent` implements the callback
- Registered in `NewAgentLoop()` via `registerAgentTool()`
- **Why**: Avoids circular dependency between `tools/` and `main` packages

### Decision 2: TaskStore in AgentLoop
- `TaskStore` uses `sync.RWMutex` for thread-safe concurrent access
- `TaskState` uses per-task `sync.Mutex` for fine-grained locking
- Stored in `AgentLoop.taskStore` field
- **Why**: Simple, no external dependencies, works with goroutines

### Decision 3: Notification Channel
- Buffered channel (capacity 10) in `AgentLoop.notificationChan`
- `EnqueueAgentNotification()` pushes formatted `<task-notification>` XML
- `DrainNotifications()` called in REPL loop before each prompt
- **Why**: Non-blocking, goroutine-safe, simple to integrate

### Decision 4: Agent Type System
- `AgentType` string constants: `""`, `"explore"`, `"plan"`, `"verify"`
- `agentTypeConfigs` map defines per-type prompt modifiers and tool deny lists
- `ParseAgentType()` converts string to `AgentType`
- `buildSubAgentRegistry()` applies type-specific tool restrictions
- **Why**: Declarative, easy to add new types, no complex inheritance

### Decision 5: Fork Mode via cloneContextForFork
- Clones parent's `conversationEntry` slice
- Replaces `ToolResultContent` with identical placeholder for cache stability
- Skips `CompactBoundaryContent` and `AttachmentContent`
- **Why**: Matches Claude Code's approach for prompt cache optimization

### Decision 6: Async Execution in SpawnSubAgent
- Goroutine launched inside `SpawnSubAgent` (not in `tools/agent_tool.go`)
- Goroutine has access to parent's `taskStore` and `notificationChan`
- Returns immediately with task ID for async; blocks for sync
- **Why**: Goroutine needs parent references; tools package can't access AgentLoop

---

## Comparison with Deepseek Prototype

The Deepseek prototype at `E:\Git\miniClaudeCode-go` uses a different architecture:

| Aspect | Deepseek (go) | Our Version (go-github) |
|--------|---------------|------------------------|
| Package structure | `subagent/` package (4 files) | `agent_sub.go` + `agent_task.go` in main |
| Execution engine | Independent mini loop in `executor.go` | Reuses full `AgentLoop` instance |
| Task management | `TaskRegistry` + `Manager` | `TaskStore` (simpler) |
| Tool access | `ToolInfo`/`ToolExecutor` interfaces | Direct `tools.Registry` copy |
| Cancel support | `Manager.RegisterCancel/KillAll` | Not yet implemented |
| Notification | `BuildNotificationXML` (not connected) | `EnqueueAgentNotification` (connected) |
| Build status | Has compile errors | Builds and tests pass |

**Key advantage of our approach**: Reusing the full `AgentLoop` means sub-agents get all features (compaction, streaming, permission gate, etc.) for free. The Deepseek approach reimplements a minimal agent loop that lacks these features.

---

## Open Questions

1. **AbortController**: Should we add `context.Context` cancellation for running sub-agents? The Deepseek prototype has `Manager.RegisterCancel()` but we don't have this yet.

2. **Notification injection into LLM context**: Currently notifications are only displayed in the REPL. Should they also be injected as user messages so the LLM can act on them?

3. **Grace eviction**: Should completed tasks be auto-removed from TaskStore after 30s? This prevents memory leaks in long sessions.

---

*Last updated: 2026-05-01*
