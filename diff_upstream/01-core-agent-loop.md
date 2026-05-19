# Core Agent Loop

> Agent loop orchestration (sections 7, 35, 7.27)

## Document Preamble

# Upstream Comparison: Go vs TypeScript (claude-code-upstream)

**Go source**: `E:\Git\miniClaudeCode-go-github`
**Upstream source**: `E:\Git\claude-code-upstream`
**Date**: 2026-05-12

---



---

## Sections Included
- [##] Line 9-56 -- ## 目录 / Table of Contents
- [##] Line 1064-1313 -- ## 7. Agent Loop Core (`agent_loop.go`)
- [##] Line 1314-1347 -- ## 7.27 Summary: Agent Loop Key Differences
- [##] Line 9407-9485 -- ## 35. Agent Loop Deep Dive — Loop Structure, Retry, Tool Execution

---

## Content

## 目录 / Table of Contents

| # | Section | Go File(s) | Key Findings |
|---|---------|------------|-------------|
| 1 | Streaming Implementation | streaming.go | Go uses SDK typed events; upstream manually tracks content blocks. Go misses server_tool_use, text, thinking block starts |
| 2 | Message Normalization | normalize.go | Go only fixes role alternation; upstream has multi-layer pipeline (tool pairing, empty msg, image handling) |
| 3 | Prompt Caching | prompt_caching.go | Go has basic cache breakpoints; upstream has cache_edits for incremental updates across API calls |
| 4 | System Prompt | system_prompt.go | Go builds string prompt; upstream has CachedSystemPrompt with lazy rebuild + dynamic injection |
| 5 | MCP Implementation | mcp/client.go | Go has stdio only; upstream has stdio+SSE+HTTP. Go missing capabilities detection, elicitation, roots |
| 6 | Test Coverage | *_test.go | Go stronger in rate-limit/retry/caching/error tests; upstream stronger in utility/schema/transport tests |
| 7 | Agent Loop Core | agent_loop.go | Go for-loop vs upstream async generator; missing model fallback, streaming tool executor, task_budget, yieldMissingToolResultBlocks |
| 8 | Compaction System | compact.go | Go has SM-compact/LLM-compact/selective; upstream has contextCollapse, snipCompact, applyToolResultBudget. Post-compact recovery largely aligned |
| 9 | Tool Interface & Registry | tools/base.go | Go Tool interface vs upstream Tool class; Go has coercion, upstream has validateInput/checkPermissions |
| 10 | Exec/Bash Tool | tools/exec_tool.go | Go has process tree kill; upstream has streaming output, shell detection, working dir handling |
| 11 | File Read Tool | tools/file_read.go | Go has binary detection + CRLF handling; upstream has image support, PDF extraction, line range in params |
| 12 | File Edit Tool | tools/file_edit.go | Go single edit; upstream has multi-edit with array edits + dryRun |
| 13 | File Write Tool | tools/file_write.go | Go has auto-snapshot; upstream has createIfNotExists flag |
| 14 | Config System | config.go | Go loads from .claude/settings.json; upstream has richer settings precedence (project/user/enterprise) |
| 15 | Permission System | permissions.go | Go has PermissionGate; upstream has interactive prompts, deny rules, auto-strip, plan mode restrictions |
| 12 | Permissions Submodule (6 files) | permissions/*.go | Go has simplified auto-strip (Bash/Exec only, no PS/Agent), weaker internal path checks (Contains vs startsWith), custom glob matcher vs upstream ignore library, no multi-symlink path checking, no skill-scope permissions, no persistent rule management |
| 13 | Hooks System | hooks.go | Go supports only PreCompact/PostCompact (Go callbacks); upstream has 20+ hook types (PreToolUse, PostToolUse, SessionStart, Stop, etc.) with shell commands, SDK callbacks, HTTP hooks, matcher system, parallel execution, prompt interaction |
| 14-31 | Remaining Modules | Various | See individual sections for error_types, agent_progress, agent_task, forked_agent, file_history, transcript, work_task, skills, sub-agent, tools/*, permissions/* |
| 28 | Search/Listing Tools | grep/glob/list_dir | Go has native grep fallback; upstream has smarter output formatting and head_limit |
| 29 | PostCompactRecovery | agent_loop.go | Largely aligned; Go missing contextCollapse, snipCompact, applyToolResultBudget |
| 30 | Context Management | context.go | Go has 6 content types; upstream has richer API message building |
| 31 | Sub-Agent | agent_sub.go | Go has goroutine-based; upstream has AgentTool with SDK stream support |
| 32 | Entry Points | main.go | Go headless CLI; upstream has full TUI (Ink/React), headless, agent SDK, multiple entry points |
| 33 | Round 28 Tool Diff | file_read.go, file_write.go, file_edit.go, exec_tool.go | 35 new differences: PDF pages param, token budget, CRLF preservation, multi-edit support, heredoc handling, sed simulation, output accumulation |
| 34 | Context.go Deep Dive | context.go | Entry system, token estimation, BuildMessages, CompactContext degradation chain, microcompact entry clearing, truncation |
| 35 | Agent Loop Deep Dive | agent_loop.go | Loop structure, retry logic, tool execution concurrency, interrupt handling, context error recovery, session memory extraction, hooks |
| 36 | Compact.go Deep Dive | compact.go | LLM compaction flow, token estimation, micro-compact, post-compact recovery missing attachments, compactor struct, boundary metadata |
| 37 | Config/Permissions/System Prompt | config.go, permissions.go, system_prompt.go, auto_classifier.go, hooks.go | Config centralization, XML classifier missing, denial tracking, system prompt caching |
| 38 | Session Memory/Skills/File History | session_memory.go, skills/, filehistory.go, context_references.go | Session memory thresholds, skill progressive discovery, file history tag system, context reference expansion |
| 39 | Streaming/Normalize/Prompt Caching | streaming.go, normalize.go, prompt_caching.go, rate_limit.go, retry_utils.go | Custom streaming architecture, rate limit bucket tracking, retry utils scope |
| 40 | Transcript/Sub-Agent/Forked Agent/Tools | transcript.go, agent_sub.go, forked_agent.go, tools/base.go | JSONL transcript, forked agent re-implementation, tool interface lifecycle hooks |
| 41 | Streaming API Layer Supplementary | streaming.go, prompt_caching.go | SSE event model, cache breakpoints, tool result storage (entire module missing), API microcompact config |
| 42 | Tools Deep Dive | file_read.go, file_write.go, file_edit.go, exec_tool.go, grep/glob, agent_tool.go, web_fetch/search | 95 differences: image/PDF support, LSP integration, skill discovery, worktree isolation, agent teams, notebook editing, web search API |
| 43 | Error Handling, Work Tasks, CLI | error_types.go, agent_task.go, work_task.go, agent_progress.go, main.go | 15-category error taxonomy, work task dependency graphs, agent progress tracking, remaining 20+ upstream modules |
| 44 | Hooks, MCP, Permissions, Cost | hooks.go, mcp/client.go, permissions.go, cost-tracker.ts | 25+ hook event types missing, 7 MCP transport types, permission rule sources, cost tracking |
| 45 | Remaining Modules Sweep | main.go, retry_utils.go, normalize.go, streaming.go, forked_agent.go, work_task.go | 29 differences: CLI simplification, error pattern matching, JSON key sorting, rate limit buckets, transcript builder |
| 46 | Analytics/Telemetry/Cost/Flags/Auth/Settings | (no Go equivalents) | 87 differences: entire telemetry system missing, cost tracking, GrowthBook feature flags, OAuth PKCE, 5-source settings |
| 47 | TUI/UI/Print/Session Management | (headless CLI) | 30 differences: no Ink/React TUI, no virtual message list, no command palette, 9 vs 30+ slash commands, no session sharing |
| 48 | API Client, Beta Headers, Model Management | api_client.go, config.go, betas.ts, model.ts | 58 differences: 15+ beta constants missing, per-model beta aggregation, model aliases, context windows, structured outputs, provider differentiation |
| 49 | Go-Specific Enhancements & Adaptations | work_task.go, file_history_tools.go, error_types.go, normalize.go, rate_limit.go, streaming.go, retry_utils.go, main.go | 87 differences: Go does 52 enhancements + 25 adaptations in error classification, file history, rate limits, streaming architecture, retry utilities, concurrency |
| 50 | Cross-Cutting Architectural Patterns | agent_loop.go, context.go, file_lock_unix.go, error_types.go | Concurrency model (goroutines vs async/await), state management, testing patterns, platform abstraction, error propagation |

---


---

## 7. Agent Loop Core (`agent_loop.go`)

### Files Compared
- **Go**: `E:\Git\miniClaudeCode-go-github\agent_loop.go` (4385 lines)
- **Upstream**: `E:\Git\claude-code-upstream\src\query.ts` (~1777 lines, main query loop), `E:\Git\claude-code-upstream\src\services\api\withRetry.ts` (~600+ lines), `E:\Git\claude-code-upstream\src\services\tools\StreamingToolExecutor.ts` (~400+ lines), `E:\Git\claude-code-upstream\src\services\tools\toolExecution.ts` (~800+ lines), `E:\Git\claude-code-upstream\src\services\compact\autoCompact.ts` (~200+ lines), `E:\Git\claude-code-upstream\src\services\extractMemories\extractMemories.ts` (~200+ lines)

### 7.1 Main Loop Structure

### 7.1.1 Loop Architecture
- **上游**: `query()` is a generator function (`async function*`) yielding `StreamEvent | Message | TombstoneMessage | ToolUseSummaryMessage` to the REPL/UI in real-time. The outer `query()` wraps `queryLoop()` which is the actual `while(true)` loop. State is carried between iterations via a `State` struct that's reassigned at 7+ `continue` sites (query.ts:222-282, query.ts:284-1776).
- **Go版**: `Run()` is a plain synchronous `for a.budget.Consume()` loop returning a single `string` (finalText). No generator, no streaming yield to caller. State is carried in local variables (`contextErrors`, `consecutiveEmptyResponses`) and the `ConversationContext` mutable object (agent_loop.go:893-1266).
- **类型**: 简化 — Go's `Run()` is a blocking call that returns only after the loop completes, whereas upstream's generator yields events incrementally for live UI updates.

### 7.1.2 Loop Continuation Logic
- **上游**: 7 distinct `continue` paths, each setting `state = { ... }` with a `transition` reason: `collapse_drain_retry`, `reactive_compact_retry`, `max_output_tokens_recovery`, `max_output_tokens_escalate`, `stop_hook_blocking`, `token_budget_continuation`, `next_turn`. Each carries different tracking state (query.ts:1161, 1211, 1297, 1352, 1387, 1774).
- **Go版**: Single `continue` path on error recovery (context errors, stream stalled, model confused). No structured transition reasons -- just `continue` with counter increment. Grace call (`budget.GraceCall()`) is a separate post-loop path (agent_loop.go:1245).
- **类型**: 简化 — Go has 1-2 continue paths vs upstream's 7 labeled transitions with rich state carryover.

### 7.1.3 Turn Counting
- **上游**: `turnCount` on State struct, incremented each turn: `const nextTurnCount = turnCount + 1` (query.ts:1726). Passed to state carryover.
- **Go版**: `IterationBudget` with `atomic.Int32` consumed counter. No explicit turn counter in state. `maxTurns` checked via `budget.Consume()` (agent_loop.go:44-53).
- **类型**: Go适配 — Go uses budget-based iteration counting instead of explicit turn counting.

### 7.2 API Calls

### 7.2.1 Model Call Interface
- **上游**: `deps.callModel({ messages, systemPrompt, thinkingConfig, tools, signal, options })` — an async generator yielding messages/events (query.ts:702-910). Wrapped by `withRetry()` for retry logic (withRetry.ts:170+). Model selection via `getRuntimeMainLoopModel()` with plan-mode 200k fallback (query.ts:615-621).
- **Go版**: Direct SDK call `a.client.Messages.New(ctx, params)` (non-streaming) or `a.client.Messages.NewStreaming(ctx, params)` (streaming). No `withRetry` wrapper; retry loop is manual (`for attempt := 0; attempt <= maxRetries; attempt++`) (agent_loop.go:1503, 1612).
- **类型**: 简化 — Go inlines retry logic instead of using a composable retry framework.

### 7.2.2 Retry Framework
- **上游**: `withRetry()` generator (withRetry.ts:170-400+) handles: 529 errors with foreground-only retry, 429 rate limits with `Retry-After` headers, fast mode cooldown (`triggerFastModeCooldown`), OAuth 401 token refresh, Bedrock/Vertex auth errors, ECONNRESET/EPIPE stale connection recovery, persistent retry mode (`CLAUDE_CODE_UNATTENDED_RETRY`), `CannotRetryError`/`FallbackTriggeredError` typed errors, MAX_529_RETRIES=3 cap, mock rate limit support for ant employees.
- **Go版**: Manual retry loop with `jitteredBackoff(attempt)` and `rateLimitState.RetryDelay()` preference. Handles: transient errors (5xx, network), 2013 tool pairing repair, rate limit headers. Missing: fast mode cooldown, OAuth refresh, persistent retry, 529 foreground-only gating, mock rate limits (agent_loop.go:1501-1569, 1612-1684).
- **类型**: 简化 — Go retry is basic; missing fast mode, OAuth, persistent, and error-type classification.

### 7.2.3 Model Fallback
- **上游**: `fallbackModel` parameter + `onStreamingFallback` callback. When `FallbackTriggeredError` is thrown during streaming, the loop catches it, yields tombstones for orphaned messages, discards the streaming executor, switches to fallback model, strips thinking signatures, and retries with `attemptWithFallback = true` (query.ts:941-998).
- **Go版**: **No model fallback support**. No `FallbackTriggeredError` equivalent. No streaming fallback path in `callWithRetryAndFallback()` (agent_loop.go:1576-1685). The `fallbackModel` config field exists but is never used in the agent loop.
- **类型**: 缺失 — Go has zero model fallback capability despite having the config field.

### 7.3 Streaming

### 7.3.1 Streaming vs Non-Streaming
- **上游**: Always streaming via `deps.callModel()`. Non-streaming is not an option in the main loop. Streaming is the only mode.
- **Go版**: Two distinct modes controlled by `a.useStream`: `callWithRetryAndFallback()` (streaming) vs `callWithNonStreamingOnly()` (non-streaming). Streaming has fallback to non-streaming on persistent failure (agent_loop.go:1018-1023, 1682-1684).
- **类型**: Go适配 — Go supports both streaming and non-streaming; upstream is streaming-only.

### 7.3.2 Streaming Tool Executor
- **上游**: `StreamingToolExecutor` (StreamingToolExecutor.ts:1-400+): tools execute as they stream in. Concurrency-safe tools run in parallel; non-concurrent tools execute exclusively. Results buffered and emitted in order. Supports `addTool()`, `getCompletedResults()`, `getRemainingResults()`, `discard()`. Progress messages stored separately and yielded immediately. Langfuse batch span tracking.
- **Go版**: **No streaming tool executor**. All tools collected after full response, then executed concurrently via `executeToolCallsConcurrent()` (agent_loop.go:1165, 2082-2182). No order-preserving execution, no concurrency safety flags, no progress message handling.
- **类型**: 缺失 — Go executes tools in a batch after the model finishes, not incrementally during streaming.

### 7.4 Tool Parallelism and Safety

### 7.4.1 Concurrent Tool Execution
- **上游**: `StreamingToolExecutor` checks `isConcurrencySafe` per tool definition. Non-concurrent tools (e.g., file write) execute exclusively. Concurrency-safe tools run in parallel via `Promise.all()` (StreamingToolExecutor.ts:180-250).
- **Go版**: All tools run concurrently via goroutines (`go func()`), results collected on channel and sorted by original index. No concurrency safety check. Permission pre-check done sequentially to avoid concurrent stdin reads in ask mode (agent_loop.go:2098-2164).
- **类型**: 简化 — Go's concurrent execution lacks the upstream's concurrency-safety gate per tool.

### 7.4.2 Permission Pre-Check
- **上游**: `canUseTool` callback invoked per-tool during execution via `runToolUse` or `streamingToolExecutor`. Permission context flows through tool execution.
- **Go版**: Pre-checks all permissions sequentially before concurrent execution (`a.gate.Check(tool, input)`), then concurrent execution skips permission check via `executeSingleToolApproved()` (agent_loop.go:2106-2122, 2214-2216).
- **类型**: Go增强 — Go's pre-check avoids concurrent stdin reads in ask mode, which upstream doesn't explicitly protect against.

### 7.5 Tool Result Persistence

### 7.5.1 Large Tool Result Persistence
- **上游**: `applyToolResultBudget()` enforces per-message budget on aggregate tool result size. `persistToolResult()` writes large results to disk (`projectDir/sessionId/tool-results/<id>.txt`), replaces content with `<persisted-output>` XML tag. GrowthBook override map for per-tool thresholds. `EEXIST` idempotency check. Preview generation (2000 bytes) (toolResultStorage.ts:1-180+).
- **Go版**: **No tool result persistence**. All results truncated to `maxToolChars` (default 50000) via `truncateOutput()` with 80/20 head-tail split (agent_loop.go:2185-2204). No disk persistence, no XML tags, no per-tool thresholds.
- **类型**: 简化 — Go truncates instead of persisting to disk.

### 7.5.2 Tool Result Budget
- **上游**: `applyToolResultBudget()` runs BEFORE microcompact, enforcing aggregate size limits across all tool results in a message. Persists replacement state for resume (query.ts:412-437).
- **Go版**: **Missing** — No equivalent of `applyToolResultBudget`. Tool results are truncated individually but not budgeted collectively.
- **类型**: 缺失

### 7.6 Tool Use Summary

### 7.6.1 Summary Generation
- **上游**: `generateToolUseSummary()` fires async (Haiku ~1s) during model streaming, consumed at next turn via `pendingToolUseSummary` promise. Includes tool name, input, output, last assistant text. Subagents skip (query.ts:1458-1529).
- **Go版**: **Missing** — No tool use summary generation. No async Haiku call to summarize tool results.
- **类型**: 缺失

### 7.7 Interrupt Handling

### 7.7.1 Interrupt Mechanism
- **上游**: `toolUseContext.abortController` — AbortController with reason tracking (`'interrupt'`, `'streaming_fallback'`, etc.). `createChildAbortController` for nested operations. `signal.aborted` checked at multiple points. `signal.reason !== 'interrupt'` gates the interruption message (query.ts:707, 886, 1062, 1093, 1532, 1548).
- **Go版**: `a.interrupted` (atomic.Bool) + `a.cancelCtx` (context.Context). `interruptCtx()` wraps base context with timeout + interrupt watcher goroutine polling 100ms ticker. `a.IsInterrupted()` checked at turn start, after tool execution, and within tool goroutines (agent_loop.go:793-890, 954, 1234, 2136).
- **类型**: Go适配 — Go uses atomic bool + context instead of AbortController; functionally equivalent.

### 7.7.2 Orphaned Tool Result Backfill
- **上游**: `yieldMissingToolResultBlocks()` generates synthetic error `tool_result` blocks for orphaned `tool_use` blocks on fallback, error, or abort (query.ts:126-152, 747-771, 947-954, 1031, 1072-1076). Streaming executor's `getRemainingResults()` generates synthetic tool_results for in-progress tools on abort (StreamingToolExecutor.ts).
- **Go版**: **Missing** — No orphaned tool_result backfill. If streaming fails mid-tool-call, orphaned tool_use blocks are not repaired before retry. `ValidateToolPairing()` handles some cases but not during streaming fallback (agent_loop.go:1539-1550 repairs on 2013 error only).
- **类型**: 简化 — Go handles orphaned tool_results only on API 2013 error, not on streaming fallback or abort.

### 7.8 Context Error Recovery

### 7.8.1 Recovery Strategy
- **上游**: Multi-layered recovery: (1) collapse drain first (cheap, keeps granular context), (2) reactive compact (full summary), (3) max_output_tokens escalation (8k -> 64k single-shot), (4) max_output_tokens recovery (inject "resume directly" message, up to 3 retries). Each path gates on `isWithheldPromptTooLong` / `isWithheldMediaSizeError` / `isWithheldMaxOutputTokens` (query.ts:1117-1303).
- **Go版**: 3-phase recovery: Phase 1 `TruncateHistory()`, Phase 2 `AggressiveTruncateHistory()`, Phase 3 `MinimumHistory()`. Single counter `contextErrors` with `maxContextRecovery=3`. Both "stream stalled" and "context length exceeded" trigger same recovery. Recovery via `injectTruncationContinuation()` (agent_loop.go:1024-1095).
- **类型**: 简化 — Go has one-size-fits-all truncation recovery; upstream has layered recovery with distinct strategies for PTL, media, and max_tokens.

### 7.8.2 Context Length Escalation
- **上游**: `ESCALATED_MAX_TOKENS = 64000` — single-shot escalation when max_output_tokens hit with capped default. `maxOutputTokensOverride` set to 64k, state continued. On subsequent hit, inject recovery message (query.ts:1242-1298).
- **Go版**: `currentMaxTokens` (atomic.Int64) escalated from `cfg.MaxOutputTokens` to `cfg.EscalatedMaxOutputTokens` on `finish_reason == "max_tokens"`. Recovery message injected if already at escalated level (agent_loop.go:1766-1775, 1943-1950, 1870-1877).
- **类型**: Go适配 — Go implements the same escalation pattern but with configurable thresholds instead of hardcoded 64k.

### 7.9 Notification Draining

### 7.9.1 Sub-Agent Notifications
- **上游**: Command queue with agentId scoping. `getCommandsByMaxPriority(sleepRan ? 'later' : 'next')` filters by `agentId` and `mode`. Main thread drains `agentId===undefined`, subagents drain their own `agentId`. Slash commands excluded (query.ts:1613-1690).
- **Go版**: `notificationChan` (buffered channel, 64 slots). `DrainNotifications()` drains all pending. `InjectNotifications()` wraps as `[System: ...]` user message. `drainPendingMessagesFunc` for parent-agent messages. Between-turn drain after tool execution (agent_loop.go:229-275, 1209-1231).
- **类型**: 简化 — Go's channel-based approach works but lacks the upstream's agentId scoping, priority ordering, and sleep-based timing.

### 7.10 Budget and Turn Management

### 7.10.1 Turn Budget
- **上游**: `maxTurns` checked at `nextTurnCount > maxTurns`, yields `max_turns_reached` attachment, returns `{ reason: 'max_turns' }` (query.ts:1752-1758). Also `task_budget` (API output_config.task_budget) tracked across compaction boundaries (query.ts:196-200, 551-557).
- **Go版**: `IterationBudget` with `Consume()` / `Refund()` / `GraceCall()`. Grace call allows one extra turn to get final text-only answer. Post-loop grace call forces text-only response (agent_loop.go:32-73, 1245-1258).
- **类型**: Go增强 — Go's grace call mechanism (one extra text-only turn after budget exhaustion) is unique to Go and not present in upstream.

### 7.10.2 Token Budget
- **上游**: `TOKEN_BUDGET` feature gate with `createBudgetTracker()`, `checkTokenBudget()`, `incrementBudgetContinuationCount()`. Continuation with diminishing returns detection (query.ts:1355-1402).
- **Go版**: **Missing** — No token budget tracking or continuation logic.
- **类型**: 缺失

### 7.11 Token Tracking

### 7.11.1 Token Counters
- **上游**: Extensive tracking: `input_tokens`, `output_tokens`, `cache_creation_input_tokens`, `cache_read_input_tokens`, `cache_deleted_input_tokens`. `tokenCountWithEstimation()` for pre-API estimates. `doesMostRecentAssistantMessageExceed200k()` for plan mode fallback (query.ts:85-88, 920-928).
- **Go版**: `totalInputTokens` and `totalOutputTokens` (atomic.Int64) accumulated via `recordTokenUsage()`. No cache token tracking. No token estimation (agent_loop.go:311-327).
- **类型**: 简化 — Go tracks only basic input/output tokens.

### 7.11.2 Prev-Turn Token Tracking (Reactive Compact)
- **上游**: `AutoCompactTrackingState` with `compacted`, `turnCounter`, `turnId`, `consecutiveFailures`. Tracked on State struct, reset on each compact (query.ts:51-60, 564-569, 1770-1771).
- **Go版**: `prevTurnTokens` int field. `CheckReactiveCompact(currentTokens, prevTurnTokens, threshold)` detects spikes. Updated each turn (agent_loop.go:296, 963-976).
- **类型**: 简化 — Go has simpler reactive compact tracking without the structured state object.

### 7.12 Session Memory Extraction

### 7.12.1 Memory Extraction
- **上游**: `extractMemories` runs once at end of complete query loop (no tool calls) via `handleStopHooks` in stopHooks.ts. Uses forked agent pattern (`runForkedAgent`) sharing parent's prompt cache. `initExtractMemories()` returns closure with state. Combined extract+write or extract-only modes (extractMemories.ts:1-80+).
- **Go版**: `extractionState` tracks tool call count and turn count. `ShouldExtract(currentTokens, hasToolCalls)` checks thresholds. `runSessionMemoryExtraction()` spawns goroutine with forked agent call. Extraction triggered mid-turn, not at loop end (agent_loop.go:1168-1182).
- **类型**: 简化 — Go triggers extraction mid-turn via goroutine instead of at loop end via stop hooks.

### 7.13 Todo List Injection

### 7.13.1 Todo List in System Prompt
- **上游**: Todo list not present in upstream (no TodoWrite tool equivalent). Task management handled differently (session memory, work items via other mechanisms).
- **Go版**: `TodoWriteTool` + `TodoList` struct. `BuildReminder()` injected into system prompt at init and every turn. `IncrementTurn()` + `BuildIdleReminder()` nudges model after 10+ idle turns (agent_loop.go:92-96, 163-164, 663-667, 996-1011).
- **类型**: Go增强 — Todo list management is a Go-original feature not present in upstream.

### 7.14 Auto Classifier

### 7.14.1 Auto Mode Classification
- **上游**: `yoloClassifier.ts` — separate module for auto-mode permission classification. `auto_mode` querySource for security classifier. `bash_classifier` for ant-only bash classification. `permissions.ts` integrates classifier decision (yoloClassifier.ts, permissions.ts).
- **Go版**: `AutoModeClassifier` wired to `PermissionGate`. Uses separate API call with configurable `AutoClassifierModel`. `SetClaudeMd()` for project instructions. `WithTranscriptSource()` for context (agent_loop.go:423-437, 583-591).
- **类型**: Go适配 — Go implements auto classifier as a standalone component wired to the gate, matching the upstream's functional pattern but with different architecture.

### 7.15 Hooks

### 7.15.1 Pre-Compact Hooks
- **上游**: `buildPostCompactMessages()` runs hooks via `compactConversation()`. Hooks include: extractMemories, autoDream, skillImprovement, magicDocs, promptSuggestion, sessionMemory. `executePostSamplingHooks()` fires after model response (query.ts:1046-1056).
- **Go版**: `HookManager` with `ExecutePreCompactHooks()` returning `PreCompactInput` and `CustomInstructions`. Manual hook execution in `ForceCompact()` (agent_loop.go:1314-1319). No `executePostSamplingHooks` equivalent.
- **类型**: 简化 — Go has pre-compact hooks but missing post-sampling hooks and the rich hook ecosystem.

### 7.15.2 Stop Hooks
- **上游**: `handleStopHooks()` yields blocking errors, prevents continuation, executes stop-failure hooks. `executeStopFailureHooks()` on API errors (query.ts:1314-1322, 1309-1311, 1040-1043).
- **Go版**: **Missing** — No stop hooks or stop-failure hooks in the agent loop.
- **类型**: 缺失

### 7.16 Transcript Writing

### 7.16.1 Transcript Management
- **上游**: Transcript writing handled by SDK/REPL layer. Messages yielded through generator are serialized by upstream infrastructure. VCR fixtures for testing.
- **Go版**: `transcript.Writer` with JSONL format. `WriteUser()`, `WriteAssistant()`, `WriteToolUse()`, `WriteToolResult()`, `Flush()`, `Close()`. `NewAgentLoopFromTranscript()` for session resume with `rebuildContextFromTranscript()` (agent_loop.go:378-382, 494-642).
- **类型**: Go适配 — Go implements its own transcript layer since there's no REPL/SDK infrastructure to delegate to.

### 7.17 @Context Expansion

### 7.17.1 Context Reference Expansion
- **上游**: `getAttachmentMessages()` processes attachments including file references, memory prefetches, skill prefetches. `prependUserContext()` adds `<system-reminder>` with CLAUDE.md, git status. `processUserInput` handles slash commands and bash commands (query.ts:1627-1637, api.ts:447-472).
- **Go版**: `PreprocessContextReferences()` expands `@file:`, `@diff`, `@commit:` etc. into inline content. `modelContextWindow()` sizing for expansion limits. `Expanded`/`Blocked` flags with warnings (agent_loop.go:901-915).
- **类型**: Go适配 — Go implements @-expansion as a preprocessing step before the loop, while upstream handles it through attachment processing within the loop.

### 7.18 Consecutive Empty Response Handling

### 7.18.1 Thinking-Only Response Detection
- **上游**: No equivalent -- upstream's streaming model always yields something (text, tool_use, or error). Empty responses are handled by the stream completing with `stop_reason`.
- **Go版**: `consecutiveEmptyResponses` counter with `maxEmptyResponses=3`. Detects thinking-only responses (no text, no tool_calls), injects hint, gives up after 3 attempts (agent_loop.go:928-929, 1107-1121).
- **类型**: Go增强 — Go handles the edge case of thinking-only responses which upstream doesn't encounter due to its streaming architecture.

### 7.19 Skill Tracking

### 7.19.1 Skill Read/Used Tracking
- **上游**: `skillPrefetch.startSkillDiscoveryPrefetch()` async discovery during streaming. `collectSkillDiscoveryPrefetch()` consumed at turn end. Skill state tracked via messages.
- **Go版**: Manual tracking in Run loop: checks `toolCalls` for `read_skill`, calls `skillTracker.MarkRead()` + `MarkUsed()`. `restoreSkillStateFromEntries()` on transcript resume (agent_loop.go:1151-1163, 748-790).
- **类型**: 简化 — Go's synchronous skill tracking vs upstream's async prefetch/consume pattern.

### 7.20 Conclusion Extraction

### 7.20.1 Session State Extraction
- **上游**: No equivalent -- upstream relies on session memory extraction via forked agent for durable memory.
- **Go版**: `extractConclusions()` with 15+ regex patterns scanning assistant text for code structure, file semantics, task progress, bug/fix, error, and discovery conclusions. Recorded in `toolStateTracker` (agent_loop.go:821-855).
- **类型**: Go增强 — Regex-based conclusion extraction is Go-original, providing lightweight session state without the overhead of upstream's forked-agent extraction.

### 7.21 Post-Compact Recovery

### 7.21.1 Context Re-injection After Compaction
- **上游**: `runPostCompactCleanup()` re-injects critical context (recent file edits, CLAUDE.md). Hook results appended. Memory injection (postCompactCleanup.ts).
- **Go版**: `PostCompactRecovery()` with `RunPostCompactCleanup()` internally. `injectTruncationContinuation()` for truncation recovery. File freshness tracking via `toolStateTracker.MarkFileFresh()` (agent_loop.go:1338-1343, 1363-1368, 1442-1449).
- **类型**: Go适配 — Go implements similar post-compact recovery with its own `PostCompactRecovery` function.

### 7.22 Cache Edits Injection

### 7.22.1 cache_edits Block
- **上游**: `CachedMCEditsBlock` insertion in messages array. Pinned edits re-inserted at original positions. Edit deduplication with `seenDeleteRefs`. `cache_reference` on tool_results before last `cache_control` (claude.ts:3214-3294).
- **Go版**: `injectCacheEdits()` from `CachedMicrocompactTracker`. Registers compactable tool_use IDs. `MarkSentToAPI()` prevents double-send (agent_loop.go:1487, 1548, 1592, 1933-1937, 1968-1974).
- **类型**: Go适配 — Go implements a simplified cache_edits mechanism via `CachedMicrocompactTracker`.

### 7.23 Preflight Compression

### 7.23.1 Session Resume Compression
- **上游**: Session resume handled by `ResumeConversation.tsx` with `sessionStorage` and `conversationRecovery.ts`. No explicit preflight compression in the query loop.
- **Go版**: `preflightThreshold = 100000` tokens. On resume with >100k tokens, tries compaction up to 3 times before entering the main loop (agent_loop.go:932-940).
- **类型**: Go增强 — Go's preflight compression on session resume is unique.

### 7.24 Grace Eviction Ticker

### 7.24.1 Background Task Cleanup
- **上游**: Background task cleanup handled by separate infrastructure (session runner, bridge messaging). No grace eviction ticker in the query loop.
- **Go版**: Background goroutine with `time.NewTicker(10s)` calling `taskStore.CleanupEvicted()`. Runs independently of the main loop (agent_loop.go:439-453, 594-608).
- **类型**: Go增强 — Go's grace eviction ticker is a unique addition for managing completed background tasks.

### 7.25 Plan Mode Tools

### 7.25.1 Plan Mode Management
- **上游**: Plan mode handled via `toolPermissionContext.mode` and `getRuntimeMainLoopModel()` plan-mode fallback. No dedicated EnterPlanMode/ExitPlanMode tools.
- **Go版**: `EnterPlanModeTool` and `ExitPlanModeTool` with `GetMode`/`SetMode`/`GetPrePlanMode` callbacks. Plan mode transitions managed through tools (agent_loop.go:104-118).
- **类型**: Go增强 — Dedicated plan mode tools are Go-original.

### 7.26 Agent Management Tools

### 7.26.1 Sub-Agent Lifecycle
- **上游**: Sub-agent lifecycle managed through `runForkedAgent()` (forkedAgent.ts) and `LocalAgentTask`. No agent_list/agent_get/agent_kill tools exposed to the model.
- **Go版**: `AgentListTool`, `AgentGetTool`, `AgentKillTool` wired to `AgentTaskStore`. Model can query and kill running sub-agents (agent_loop.go:157-165).
- **类型**: Go增强 — Exposing agent management as tools to the model is Go-original.

---


---

## 7.27 Summary: Agent Loop Key Differences

### Critical Gaps (correctness/behavior differences)
1. **No streaming tool executor** -- Go collects all tool calls then executes; upstream streams tools in during model response
2. **No model fallback during streaming** -- Go's fallbackModel is never used in the loop
3. **No orphaned tool_result backfill** -- Missing on streaming fallback and abort (only handled on API 2013 error)
4. **No tool result persistence** -- Truncation-only vs upstream's disk persistence with XML tags
5. **No tool use summary** -- Missing async Haiku summary of tool results
6. **No stop hooks** -- Missing handleStopHooks and executeStopFailureHooks
7. **No token budget tracking** -- Missing TOKEN_BUDGET feature

### Behavioral Differences
8. **Generator vs blocking** -- Upstream yields events in real-time; Go's Run() blocks until completion
9. **Single vs layered recovery** -- Go uses one-size-fits-all truncation; upstream has collapse -> reactive compact -> max_tokens escalation -> recovery message
10. **Batch vs streaming tool execution** -- Go executes all tools after response; upstream executes as they stream
11. **No concurrency safety per tool** -- Go runs all tools concurrently; upstream respects isConcurrencySafe flag

### Go-Original Features (enhancements)
12. **Grace call mechanism** -- One extra text-only turn after budget exhaustion
13. **Conclusion extraction** -- Regex-based session state extraction
14. **Todo list injection** -- TodoWrite tool with idle reminders
15. **Preflight compression** -- Auto-compact on session resume >100k tokens
16. **Grace eviction ticker** -- Background task cleanup
17. **Agent management tools** -- agent_list/get/kill exposed to model
18. **Plan mode tools** -- EnterPlanMode/ExitPlanMode as tools
19. **Permission pre-check** -- Sequential pre-check avoids concurrent stdin reads

### Architecture Differences
20. **State machine vs local variables** -- Upstream uses structured State with 7 transition reasons; Go uses local counters and simple continue
21. **Composable retry vs inline retry** -- Upstream's withRetry() generator vs Go's manual for-loop
22. **Atomic bool vs AbortController** -- Go's interrupted flag vs upstream's AbortController with reason tracking

---


---

## 35. Agent Loop Deep Dive — Loop Structure, Retry, Tool Execution

### Files Compared
- **Go**: `E:\Git\miniClaudeCode-go-github\agent_loop.go` (4386 lines)
- **Upstream**: `E:\Git\claude-code-upstream\src\query.ts` (1777 lines), `toolOrchestration.ts`, `stopHooks.ts`, `tokenBudget.ts`

### 35.1 Main Loop Structure

| # | Aspect | Go (`agent_loop.go`) | Upstream (`query.ts`) | Type |
|---|--------|---------------------|----------------------|------|
| 1 | Loop pattern | `for a.budget.Consume()` linear loop. `IterationBudget` atomic counter with Consume/Refund/GraceCall (line 32-73) | Generator `queryLoop` with State struct (line 311-322), 7 `continue` sites for state transitions (L1161, L1211, L1267, L1298, L1351, L1387, L1774) | Go适配 |
| 2 | Turn tracking | No turnCount variable — budget serves as counter | `turnCount` checked against `maxTurns` at L1752 | 简化 |
| 3 | Terminal reasons | Returns final text string directly | Yields Terminal reason: 'completed', 'aborted_streaming', 'aborted_tools', 'prompt_too_long', 'max_turns', 'stop_hook_prevented' | 简化 |
| 4 | Grace call | `GraceCall()` after budget exhausted — one more text-only call (line 1245) | No equivalent — yields terminal reason immediately | Go增强 |

### 35.2 API Call Retry Logic

| # | Aspect | Go (`agent_loop.go:1576-1685`) | Upstream (`query.ts:696-1000`) | Type |
|---|--------|-------------------------------|-------------------------------|------|
| 1 | Retry mechanism | Explicit 10-retry with `jitteredBackoff` + rate-limit header override (line 1616-1619) | `attemptWithFallback` pattern — inner streaming loop, outer catch for `FallbackTriggeredError` | Go适配 |
| 2 | Delta state tracking | `DeltasState`: None → clean retry, ToolInFlight → retry with warning, TextOnly → non-streaming fallback (line 1660-1674) | No delta-state tracking — uses error type switching | Go适配 |
| 3 | Model fallback | No fallback model concept | `fallbackModel` switching on `FallbackTriggeredError`, strip signature blocks, yield system warning | 缺失 |
| 4 | Consecutive-500 heuristic | 5 consecutive 500s triggers compaction (proxy context overflow detection) | No equivalent heuristic | Go增强 |
| 5 | Non-streaming fallback | `callWithNonStreamingFallback` (line 1903-2017): 10 retries + consecutive-500 detection | Falls back to non-streaming via `withRetry` | 匹配 |

### 35.3 Tool Call Execution

| # | Aspect | Go (`agent_loop.go:2082-2182`) | Upstream (`query.ts:1427-1455`, `toolOrchestration.ts`) | Type |
|---|--------|-------------------------------|------------------------------------------------------|------|
| 1 | Concurrency model | ALL tools run concurrently in goroutines — no safety partitioning (line 2133-2164) | Partitioned by `isConcurrencySafe()`: read-only concurrent, write serial (line 108-129). Max concurrency 10 | 简化 |
| 2 | Streaming tool executor | No streaming tool executor — tools run after full API response | `StreamingToolExecutor` — tools execute as they stream in (feature gate `tengu_streaming_tool_execution2`) | 缺失 |
| 3 | Langfuse tracing | No tracing | Batch span for all tools in a turn (line 27-32) | 缺失 |
| 4 | Panic recovery | Per-tool panic recovery (line 2336-2345) | No panic recovery (JS/TS doesn't have panics) | Go适配 |
| 5 | Permission pre-checks | Sequential permission checks before concurrent execution to avoid concurrent stdin reads (line 2098-2123) | Permission checks integrated with `runTools()` | Go适配 |
| 6 | Context modifiers | No context modifiers | Context modifiers queued and applied after each batch (line 43-75) | 缺失 |

### 35.4 Interrupt/Cancellation Handling

| # | Aspect | Go (`agent_loop.go:857-890`) | Upstream (`query.ts:1062-1098`) | Type |
|---|--------|-----------------------------|-------------------------------|------|
| 1 | Cancellation model | `interruptCtx` wraps base context with timeout, polls `IsInterrupted()` atomic flag every 100ms | `AbortController.signal` — standard AbortController pattern | Go适配 |
| 2 | Dual cancellation | interrupt flag + `cancelCtx` (for sub-agent kill from parent) | Single AbortController | Go增强 |
| 3 | Submit-interrupt distinction | No distinction — always shows interruption message | Checks abort reason: 'interrupt' vs others — skips message for submit-interrupts | 简化 |
| 4 | MCP cleanup | No MCP cleanup on interrupt | `CHICAGO_MCP` cleanup on interrupt (line 1080-1089) | 缺失 |

### 35.5 Context Error Recovery

| # | Aspect | Go (`agent_loop.go:925, 1075-1094`) | Upstream (`query.ts:1117-1230`) | Type |
|---|--------|-----------------------------------|-------------------------------|------|
| 1 | Recovery strategy | 3-phase counter: contextErrors≤1→TruncateHistory, ≤2→AggressiveTruncateHistory, ≥3→MinimumHistory | Context collapse drain (`recoverFromOverflow()`), reactive compact (strip media), max_output_tokens recovery (escalate to 64k) | 简化 |
| 2 | Context collapse | No context collapse mechanism | `contextCollapse.recoverFromOverflow()` — stages collapse commits | 缺失 |
| 3 | Reactive compact for media | No media-size-specific recovery | `reactiveCompact.tryReactiveCompact()` — strips oversized media/images | 缺失 |
| 4 | Token escalation recovery | Escalates to `EscalatedMaxOutputTokens`, injects recovery message each hit — no limit counter | `MAX_OUTPUT_TOKENS_RECOVERY_LIMIT = 3`, gate `tengu_otk_slot_v1`, escalate to 64k | 简化 |

### 35.6 Session Memory Extraction

| # | Aspect | Go (`agent_loop.go:4306-4385`) | Upstream (`stopHooks.ts:148-164`) | Type |
|---|--------|-------------------------------|----------------------------------|------|
| 1 | Extraction approach | Forked agent with restricted canUseTool (only edit_file on session_memory.md), `ExtractionState` tracks thresholds/tool counts/tokens | Fire-and-forget hook `executeExtractMemories()`, gated on `EXTRACT_MEMORIES` feature flag | Go适配 (more elaborate) |
| 2 | Trigger | `extractionState.ShouldExtract()` at L1177-1182 | Called in stop hooks after each turn | Go适配 |
| 3 | Wait timeout | 15s timeout in SM-compact (L3950-3952) | `drainPendingExtraction` in print.ts for -p/SDK mode | Go适配 |

### 35.7 Hooks Execution

| # | Aspect | Go (`agent_loop.go:3056-3088, 3927-3937`) | Upstream (`stopHooks.ts:65-481`) | Type |
|---|--------|------------------------------------------|----------------------------------|------|
| 1 | Hook types | Pre/post compact hooks only | Stop hooks, teammate idle, task completed, prompt suggestion, auto-dream, extract memories, job classifier | 简化 |
| 2 | Hook input/output | `PreCompactInput{Trigger, CustomInstructions}`, `PostCompactInput{Trigger, CompactSummary, RecoveredFiles}` | Comprehensive hook pipeline with MCP cleanup, job state classification, skill activation | 简化 |

### 35.8 Compaction Integration in Loop

| # | Aspect | Go (`agent_loop.go:3832-4239`) | Upstream (`query.ts:440-586`) | Type |
|---|--------|-------------------------------|-----------------------------|------|
| 1 | Compaction layers | 3 phases: time-based MC → SM-compact → LLM-compact. Fallback to truncation | 4 layers: snip compact → microcompact → context collapse → auto-compact | Go适配 (different architecture) |
| 2 | SM-compact | Unique to Go — session memory file as summary | SM-compact in upstream (`sessionMemoryCompact.ts`) but called differently | 匹配 (both have SM-compact) |
| 3 | Post-compact recovery | `PostCompactRecovery()` (L2819-3090): extensive file/tool/agent/MCP/skill/session-memory recovery | `buildPostCompactMessages()`: boundaryMarker, summaryMessages, messagesToKeep, attachments, hookResults | 匹配 (both extensive) |
| 4 | Cooldown | `ShouldCompact()` checks 25% token growth (L4007-4011) | Consecutive failures tracked (L579-586) | Go适配 |
| 5 | Prefetch | No memory/skill prefetch | `startRelevantMemoryPrefetch()`, `startSkillDiscoveryPrefetch()` parallel during streaming | 缺失 |


---

