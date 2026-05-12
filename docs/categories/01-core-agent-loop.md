# 01 — Core Agent Loop

> Agent loop, context management, compaction, streaming, recovery

## Overview

The core agent loop is Go's central orchestration layer. The upstream has a significantly more sophisticated pipeline with multi-phase normalization, reactive compaction, and richer recovery strategies.

---

## 1. Agent Loop — Turn Execution

| Aspect | Go | Upstream | Gap |
|--------|-----|----------|-----|
| Turn structure | Single `Run()` loop with streaming + tool execution | `QueryEngine` with streaming, tool execution, speculation, and post-stream recovery | 简化 |
| Streaming fallback | `callWithRetryAndFallback` -> `callWithNonStreamingFallback` | Streaming only (no non-streaming fallback) | 差异 |
| Interrupt detection | `atomic.Bool` polling (100ms ticker goroutine) | `AbortController` with event-driven abort + reason tracking | 简化 |
| Abort reason tracking | Boolean only (`IsInterrupted()`) | Reason enum: `'interrupt'`, `'streaming_fallback'`, etc. | 缺失 |
| Hierarchical abort | `context.WithCancel` (flat) | `createChildAbortController` for nested operations | 缺失 |

**Action**: Add abort reason tracking. Consider replacing polling with event-driven cancellation.

---

## 2. Context Management — Conversation Context

| Aspect | Go | Upstream | Gap |
|--------|-----|----------|-----|
| Context structure | `ConversationContext` with `[]Entry` list | Rich context with `fileHistory`, `attribution`, `contentReplacements`, `contextCollapse` state | 简化 |
| Compact boundary | `CompactBoundary` marks truncation point | `system` message with `subtype: 'compact_boundary'` + rich metadata (trigger, preTokens, userContext, messagesSummarized, discoveredTools, preservedSegment) | 简化 |
| Preserved segments | Not supported | `preservedSegment` keeps selected messages across compact boundaries | 缺失 |
| Post-compact skill reset | Explicit `ResetPostCompact()` | `postCompactCleanup` with skill change detection | Go适配 |

**Action**: Add `preservedSegment` support to compact boundaries.

---

## 3. Compaction — When and How

| Aspect | Go | Upstream | Gap |
|--------|-----|----------|-----|
| Triggers | Manual (`/compact`), auto (token threshold), reactive (consecutive 500s) | Manual, auto, reactive (PTL error), proactive (microcompact) | 简化 |
| Reactive compact | 5-strike consecutive-500 heuristic -> truncate | `reactiveCompact.tryReactiveCompact()` with precise token targeting | 简化 |
| Microcompact | Time-based content replacement + cached MC | Time-based + cached MC + GrowthBook gating + model support check | 简化 |
| Proactive compaction | Not supported | `contextCollapse` system with staged queue, speculative pre-emption | 缺失 |
| Compaction summary | `WriteSummary(content)` | Summary as `user` message with `isCompactSummary: true`, preserves tool state | 差异 |

**Key difference**: Go's 5-strike-500 heuristic is a proxy workaround (generic 500 = likely context overflow). Upstream's reactive compact uses precise 400 token-count parsing. Go's recovery is truncation-based; upstream's is compaction-based.

**Action**: Replace consecutive-500 heuristic with precise token-gap parsing for reactive compact.

---

## 4. Streaming — Event Processing

| Aspect | Go | Upstream | Gap |
|--------|-----|----------|-----|
| Stream adapter | `StreamAdapter` with event routing | `queryModelWithStreaming()` async generator | 差异 |
| Stall detection | Always-on, 240-300s dynamic timeout, auto-recovery | Env-gated watchdog (90s), telemetry-only stall logging | Go增强 |
| Dynamic timeouts | Yes: local=300s/600s, >100K tokens=300s/360s, >50K=240s/300s | Fixed configurable timeout | Go增强 |
| Stream stall recovery | Progressive: TruncateHistory -> AggressiveTruncate -> MinimumHistory | Abort only, no automatic recovery in stream layer | Go增强 |
| Warning phase | None | Half-timeout warning before abort | 缺失 |

**Go strength**: Stall detection is more aggressive and has automatic recovery. Upstream's is opt-in and passive.

---

## 5. Recovery — Error Handling in the Loop

| Aspect | Go | Upstream | Gap |
|--------|-----|----------|-----|
| Progressive recovery | TruncateHistory -> AggressiveTruncateHistory -> MinimumHistory | Collapse drain -> reactive compact -> surface error | 差异 |
| `max_output_tokens` recovery | Not supported | 8K -> 64K escalation -> multi-turn recovery messages | 缺失 |
| Error withholding | Not supported (errors surfaced immediately) | Withhold PTL/media errors until recovery path determined | 缺失 |
| Death spiral prevention | `maxContextRecovery` cap | `hasAttemptedReactiveCompact` guard + stop hook skip | 差异 |
| Recovery hints | Model-directed: "Your previous response was malformed..." | User-facing: "Run /model to pick a different model" | 差异 |

**Action**: Add `max_output_tokens` escalation path. Consider error withholding for PTL/media errors.

---

## 6. Post-Compact Recovery — Deep Comparison

| Aspect | Go | Upstream | Gap |
|--------|-----|----------|-----|
| Detection | Pattern matching: "prompt is too long" substring | Pattern matching + `parseMaxTokensContextOverflowError()` token count parsing | 简化 |
| Token gap parsing | `parsePromptTooLongTokenGap()` exists but unused for auto-recovery | `getPromptTooLongTokenGap()` drives reactive compact token targeting | 简化 |
| Recovery tiers | Single path: truncation with escalating severity | Multi-tier: collapse drain -> reactive compact -> max_tokens escalation -> surface | 缺失 |
| State restoration | Skill state only | 15+ fields: skills, file history, attribution, content replacements, context collapse, worktree, agent metadata | 缺失 |

**Action**: Implement multi-tier recovery with collapse drain and reactive compact.

---

## Cross-References

- Streaming retry: [04-api-client.md](04-api-client.md) §2
- Error classification: [07-architecture.md](07-architecture.md) §3
- Message normalization: [07-architecture.md](07-architecture.md) §4
- Compaction cache economics: [04-api-client.md](04-api-client.md) §5
- Context references: [08-enhancements.md](08-enhancements.md) §1

---
---

# Detailed Comparison

## Section A: Agent Loop Core

> Source: [diff_upstream/01-core-agent-loop.md]

Detailed analysis of the agent loop orchestration, covering loop structure, API calls, tool execution, interrupt handling, budget management, hooks, and Go-original features.

### A.1 Main Loop Structure

**Intro**: The fundamental architecture difference is that upstream's `query()` is an async generator (`async function*`) yielding events to the REPL/UI in real-time, while Go's `Run()` is a blocking synchronous call returning a final string.

#### A.1.1 Loop Architecture

| Aspect | Go | Upstream | Type |
|--------|-----|----------|------|
| Loop pattern | `for a.budget.Consume()` linear loop. `IterationBudget` atomic counter with Consume/Refund/GraceCall (agent_loop.go:32-73) | Generator `queryLoop` with State struct, 7 `continue` sites for state transitions (query.ts L1161, L1211, L1267, L1298, L1351, L1387, L1774) | Go适配 |
| Turn tracking | No `turnCount` variable -- budget serves as counter | `turnCount` checked against `maxTurns` | 简化 |
| Terminal reasons | Returns final text string directly | Yields Terminal reason: 'completed', 'aborted_streaming', 'aborted_tools', 'prompt_too_long', 'max_turns', 'stop_hook_prevented' | 简化 |
| Grace call | `GraceCall()` after budget exhausted -- one more text-only call (line 1245) | No equivalent -- yields terminal reason immediately | Go增强 |

#### A.1.2 Loop Continuation Logic

- **Upstream**: 7 distinct `continue` paths, each setting `state = { ... }` with a `transition` reason: `collapse_drain_retry`, `reactive_compact_retry`, `max_output_tokens_recovery`, `max_output_tokens_escalate`, `stop_hook_blocking`, `token_budget_continuation`, `next_turn`. Each carries different tracking state (query.ts:1161, 1211, 1297, 1352, 1387, 1774).
- **Go**: Single `continue` path on error recovery (context errors, stream stalled, model confused). No structured transition reasons -- just `continue` with counter increment. Grace call (`budget.GraceCall()`) is a separate post-loop path (agent_loop.go:893-1266).
- **Type**: 简化 -- Go has 1-2 continue paths vs upstream's 7 labeled transitions with rich state carryover.

#### A.1.3 Turn Counting

- **Upstream**: `turnCount` on State struct, incremented each turn: `const nextTurnCount = turnCount + 1` (query.ts:1726). Passed to state carryover.
- **Go**: `IterationBudget` with `atomic.Int32` consumed counter. No explicit turn counter in state. `maxTurns` checked via `budget.Consume()` (agent_loop.go:44-53).
- **Type**: Go适配 -- Go uses budget-based iteration counting instead of explicit turn counting.

### A.2 API Calls

#### A.2.1 Model Call Interface

| Aspect | Go | Upstream | Type |
|--------|-----|----------|------|
| Call pattern | Direct SDK call `a.client.Messages.New(ctx, params)` or `a.client.Messages.NewStreaming(ctx, params)` | `deps.callModel({ messages, systemPrompt, thinkingConfig, tools, signal, options })` -- async generator wrapped by `withRetry()` | 简化 |
| Retry | Manual retry loop (`for attempt := 0; attempt <= maxRetries; attempt++`) | `withRetry()` generator (withRetry.ts:170-400+) | 简化 |
| Model selection | Config-based, no runtime switching | `getRuntimeMainLoopModel()` with plan-mode 200k fallback | 简化 |

#### A.2.2 Retry Framework

- **Upstream**: `withRetry()` generator handles: 529 errors with foreground-only retry, 429 rate limits with `Retry-After` headers, fast mode cooldown (`triggerFastModeCooldown`), OAuth 401 token refresh, Bedrock/Vertex auth errors, ECONNRESET/EPIPE stale connection recovery, persistent retry mode (`CLAUDE_CODE_UNATTENDED_RETRY`), `CannotRetryError`/`FallbackTriggeredError` typed errors, MAX_529_RETRIES=3 cap, mock rate limit support for ant employees.
- **Go**: Manual retry loop with `jitteredBackoff(attempt)` and `rateLimitState.RetryDelay()` preference. Handles: transient errors (5xx, network), 2013 tool pairing repair, rate limit headers. Missing: fast mode cooldown, OAuth refresh, persistent retry, 529 foreground-only gating, mock rate limits (agent_loop.go:1501-1569, 1612-1684).
- **Type**: 简化 -- Go retry is basic; missing fast mode, OAuth, persistent, and error-type classification.

#### A.2.3 Model Fallback

- **Upstream**: `fallbackModel` parameter + `onStreamingFallback` callback. When `FallbackTriggeredError` is thrown during streaming, the loop catches it, yields tombstones for orphaned messages, discards the streaming executor, switches to fallback model, strips thinking signatures, and retries with `attemptWithFallback = true` (query.ts:941-998).
- **Go**: **No model fallback support**. No `FallbackTriggeredError` equivalent. No streaming fallback path in `callWithRetryAndFallback()`. The `fallbackModel` config field exists but is never used in the agent loop (agent_loop.go:1576-1685).
- **Type**: 缺失 -- Go has zero model fallback capability despite having the config field.

### A.3 Streaming (Agent Loop Perspective)

#### A.3.1 Streaming vs Non-Streaming

| Aspect | Go | Upstream | Type |
|--------|-----|----------|------|
| Mode | Two modes: `callWithRetryAndFallback()` (streaming) vs `callWithNonStreamingOnly()` (non-streaming). Streaming has fallback to non-streaming | Streaming only. Non-streaming is not an option in the main loop | Go适配 |
| Delta state tracking | `DeltasState`: None -> clean retry, ToolInFlight -> retry with warning, TextOnly -> non-streaming fallback | No delta-state tracking -- uses error type switching | Go适配 |
| Consecutive-500 heuristic | 5 consecutive 500s triggers compaction (proxy context overflow detection) | No equivalent heuristic | Go增强 |

#### A.3.2 Streaming Tool Executor

- **Upstream**: `StreamingToolExecutor` (StreamingToolExecutor.ts:1-400+): tools execute as they stream in. Concurrency-safe tools run in parallel; non-concurrent tools execute exclusively. Results buffered and emitted in order. Supports `addTool()`, `getCompletedResults()`, `getRemainingResults()`, `discard()`. Progress messages stored separately and yielded immediately. Langfuse batch span tracking.
- **Go**: **No streaming tool executor**. All tools collected after full response, then executed concurrently via `executeToolCallsConcurrent()` (agent_loop.go:1165, 2082-2182). No order-preserving execution, no concurrency safety flags, no progress message handling.
- **Type**: 缺失 -- Go executes tools in a batch after the model finishes, not incrementally during streaming.

### A.4 Tool Parallelism and Safety

#### A.4.1 Concurrent Tool Execution

| Aspect | Go | Upstream | Type |
|--------|-----|----------|------|
| Concurrency model | ALL tools run concurrently in goroutines -- no safety partitioning | Partitioned by `isConcurrencySafe()`: read-only concurrent, write serial. Max concurrency 10 | 简化 |
| Streaming tool executor | No streaming tool executor -- tools run after full API response | `StreamingToolExecutor` -- tools execute as they stream in (feature gate `tengu_streaming_tool_execution2`) | 缺失 |
| Langfuse tracing | No tracing | Batch span for all tools in a turn | 缺失 |
| Panic recovery | Per-tool panic recovery | No panic recovery (JS/TS doesn't have panics) | Go适配 |
| Permission pre-checks | Sequential permission checks before concurrent execution to avoid concurrent stdin reads | Permission checks integrated with `runTools()` | Go适配 |
| Context modifiers | No context modifiers | Context modifiers queued and applied after each batch | 缺失 |

#### A.4.2 Permission Pre-Check

- **Go**: Pre-checks all permissions sequentially before concurrent execution (`a.gate.Check(tool, input)`), then concurrent execution skips permission check via `executeSingleToolApproved()` (agent_loop.go:2106-2122, 2214-2216).
- **Upstream**: `canUseTool` callback invoked per-tool during execution via `runToolUse` or `streamingToolExecutor`. Permission context flows through tool execution.
- **Type**: Go增强 -- Go's pre-check avoids concurrent stdin reads in ask mode, which upstream doesn't explicitly protect against.

### A.5 Tool Result Persistence

#### A.5.1 Large Tool Result Persistence

- **Upstream**: `applyToolResultBudget()` enforces per-message budget on aggregate tool result size. `persistToolResult()` writes large results to disk (`projectDir/sessionId/tool-results/<id>.txt`), replaces content with `<persisted-output>` XML tag. GrowthBook override map for per-tool thresholds. `EEXIST` idempotency check. Preview generation (2000 bytes) (toolResultStorage.ts:1-180+).
- **Go**: **No tool result persistence**. All results truncated to `maxToolChars` (default 50000) via `truncateOutput()` with 80/20 head-tail split (agent_loop.go:2185-2204). No disk persistence, no XML tags, no per-tool thresholds.
- **Type**: 简化 -- Go truncates instead of persisting to disk.

#### A.5.2 Tool Result Budget

- **Upstream**: `applyToolResultBudget()` runs BEFORE microcompact, enforcing aggregate size limits across all tool results in a message. Persists replacement state for resume (query.ts:412-437).
- **Go**: **Missing** -- No equivalent of `applyToolResultBudget`. Tool results are truncated individually but not budgeted collectively.
- **Type**: 缺失

### A.6 Tool Use Summary

- **Upstream**: `generateToolUseSummary()` fires async (Haiku ~1s) during model streaming, consumed at next turn via `pendingToolUseSummary` promise. Includes tool name, input, output, last assistant text. Subagents skip (query.ts:1458-1529).
- **Go**: **Missing** -- No tool use summary generation. No async Haiku call to summarize tool results.
- **Type**: 缺失

### A.7 Interrupt Handling

#### A.7.1 Interrupt Mechanism

| Aspect | Go | Upstream | Type |
|--------|-----|----------|------|
| Cancellation model | `interruptCtx` wraps base context with timeout, polls `IsInterrupted()` atomic flag every 100ms | `AbortController.signal` -- standard AbortController pattern | Go适配 |
| Dual cancellation | interrupt flag + `cancelCtx` (for sub-agent kill from parent) | Single AbortController | Go增强 |
| Submit-interrupt distinction | No distinction -- always shows interruption message | Checks abort reason: 'interrupt' vs others -- skips message for submit-interrupts | 简化 |
| MCP cleanup | No MCP cleanup on interrupt | `CHICAGO_MCP` cleanup on interrupt | 缺失 |

#### A.7.2 Orphaned Tool Result Backfill

- **Upstream**: `yieldMissingToolResultBlocks()` generates synthetic error `tool_result` blocks for orphaned `tool_use` blocks on fallback, error, or abort (query.ts:126-152, 747-771, 947-954, 1031, 1072-1076). Streaming executor's `getRemainingResults()` generates synthetic tool_results for in-progress tools on abort (StreamingToolExecutor.ts).
- **Go**: **Missing** -- No orphaned tool_result backfill. If streaming fails mid-tool-call, orphaned tool_use blocks are not repaired before retry. `ValidateToolPairing()` handles some cases but not during streaming fallback (agent_loop.go:1539-1550 repairs on 2013 error only).
- **Type**: 简化 -- Go handles orphaned tool_results only on API 2013 error, not on streaming fallback or abort.

### A.8 Context Error Recovery

#### A.8.1 Recovery Strategy

| Aspect | Go | Upstream | Type |
|--------|-----|----------|------|
| Recovery tiers | 3-phase counter: contextErrors<=1->TruncateHistory, <=2->AggressiveTruncateHistory, >=3->MinimumHistory | Context collapse drain (`recoverFromOverflow()`), reactive compact (strip media), max_output_tokens recovery (escalate to 64k) | 简化 |
| Context collapse | No context collapse mechanism | `contextCollapse.recoverFromOverflow()` -- stages collapse commits | 缺失 |
| Reactive compact for media | No media-size-specific recovery | `reactiveCompact.tryReactiveCompact()` -- strips oversized media/images | 缺失 |
| Token escalation recovery | Escalates to `EscalatedMaxOutputTokens`, injects recovery message each hit -- no limit counter | `MAX_OUTPUT_TOKENS_RECOVERY_LIMIT = 3`, gate `tengu_otk_slot_v1`, escalate to 64k | 简化 |

#### A.8.2 Context Length Escalation

- **Upstream**: `ESCALATED_MAX_TOKENS = 64000` -- single-shot escalation when max_output_tokens hit with capped default. `maxOutputTokensOverride` set to 64k, state continued. On subsequent hit, inject recovery message (query.ts:1242-1298).
- **Go**: `currentMaxTokens` (atomic.Int64) escalated from `cfg.MaxOutputTokens` to `cfg.EscalatedMaxOutputTokens` on `finish_reason == "max_tokens"`. Recovery message injected if already at escalated level (agent_loop.go:1766-1775, 1943-1950, 1870-1877).
- **Type**: Go适配 -- Go implements the same escalation pattern but with configurable thresholds instead of hardcoded 64k.

### A.9 Notification Draining

- **Upstream**: Command queue with agentId scoping. `getCommandsByMaxPriority(sleepRan ? 'later' : 'next')` filters by `agentId` and `mode`. Main thread drains `agentId===undefined`, subagents drain their own `agentId`. Slash commands excluded (query.ts:1613-1690).
- **Go**: `notificationChan` (buffered channel, 64 slots). `DrainNotifications()` drains all pending. `InjectNotifications()` wraps as `[System: ...]` user message. `drainPendingMessagesFunc` for parent-agent messages. Between-turn drain after tool execution (agent_loop.go:229-275, 1209-1231).
- **Type**: 简化 -- Go's channel-based approach works but lacks the upstream's agentId scoping, priority ordering, and sleep-based timing.

### A.10 Budget and Turn Management

#### A.10.1 Turn Budget

- **Upstream**: `maxTurns` checked at `nextTurnCount > maxTurns`, yields `max_turns_reached` attachment, returns `{ reason: 'max_turns' }` (query.ts:1752-1758). Also `task_budget` (API output_config.task_budget) tracked across compaction boundaries (query.ts:196-200, 551-557).
- **Go**: `IterationBudget` with `Consume()` / `Refund()` / `GraceCall()`. Grace call allows one extra turn to get final text-only answer. Post-loop grace call forces text-only response (agent_loop.go:32-73, 1245-1258).
- **Type**: Go增强 -- Go's grace call mechanism (one extra text-only turn after budget exhaustion) is unique to Go and not present in upstream.

#### A.10.2 Token Budget

- **Upstream**: `TOKEN_BUDGET` feature gate with `createBudgetTracker()`, `checkTokenBudget()`, `incrementBudgetContinuationCount()`. Continuation with diminishing returns detection (query.ts:1355-1402).
- **Go**: **Missing** -- No token budget tracking or continuation logic.
- **Type**: 缺失

### A.11 Token Tracking

#### A.11.1 Token Counters

- **Upstream**: Extensive tracking: `input_tokens`, `output_tokens`, `cache_creation_input_tokens`, `cache_read_input_tokens`, `cache_deleted_input_tokens`. `tokenCountWithEstimation()` for pre-API estimates. `doesMostRecentAssistantMessageExceed200k()` for plan mode fallback (query.ts:85-88, 920-928).
- **Go**: `totalInputTokens` and `totalOutputTokens` (atomic.Int64) accumulated via `recordTokenUsage()`. No cache token tracking. No token estimation (agent_loop.go:311-327).
- **Type**: 简化 -- Go tracks only basic input/output tokens.

#### A.11.2 Prev-Turn Token Tracking (Reactive Compact)

- **Upstream**: `AutoCompactTrackingState` with `compacted`, `turnCounter`, `turnId`, `consecutiveFailures`. Tracked on State struct, reset on each compact (query.ts:51-60, 564-569, 1770-1771).
- **Go**: `prevTurnTokens` int field. `CheckReactiveCompact(currentTokens, prevTurnTokens, threshold)` detects spikes. Updated each turn (agent_loop.go:296, 963-976).
- **Type**: 简化 -- Go has simpler reactive compact tracking without the structured state object.

### A.12 Session Memory Extraction

| Aspect | Go (agent_loop.go:4306-4385) | Upstream (stopHooks.ts:148-164) | Type |
|--------|------------------------------|----------------------------------|------|
| Extraction approach | Forked agent with restricted canUseTool (only edit_file on session_memory.md), `ExtractionState` tracks thresholds/tool counts/tokens | Fire-and-forget hook `executeExtractMemories()`, gated on `EXTRACT_MEMORIES` feature flag | Go适配 (more elaborate) |
| Trigger | `extractionState.ShouldExtract()` at L1177-1182 | Called in stop hooks after each turn | Go适配 |
| Wait timeout | 15s timeout in SM-compact (L3950-3952) | `drainPendingExtraction` in print.ts for -p/SDK mode | Go适配 |

**Key difference**: Go triggers extraction mid-turn via goroutine instead of at loop end via stop hooks. The Go implementation is more elaborate, using a forked agent with tool restrictions.

### A.13 Todo List Injection

- **Upstream**: Todo list not present in upstream (no TodoWrite tool equivalent). Task management handled differently (session memory, work items via other mechanisms).
- **Go**: `TodoWriteTool` + `TodoList` struct. `BuildReminder()` injected into system prompt at init and every turn. `IncrementTurn()` + `BuildIdleReminder()` nudges model after 10+ idle turns (agent_loop.go:92-96, 163-164, 663-667, 996-1011).
- **Type**: Go增强 -- Todo list management is a Go-original feature not present in upstream.

### A.14 Auto Classifier

- **Upstream**: `yoloClassifier.ts` -- separate module for auto-mode permission classification. `auto_mode` querySource for security classifier. `bash_classifier` for ant-only bash classification. `permissions.ts` integrates classifier decision.
- **Go**: `AutoModeClassifier` wired to `PermissionGate`. Uses separate API call with configurable `AutoClassifierModel`. `SetClaudeMd()` for project instructions. `WithTranscriptSource()` for context (agent_loop.go:423-437, 583-591).
- **Type**: Go适配 -- Go implements auto classifier as a standalone component wired to the gate, matching the upstream's functional pattern but with different architecture.

### A.15 Hooks

#### A.15.1 Pre-Compact Hooks

- **Upstream**: `buildPostCompactMessages()` runs hooks via `compactConversation()`. Hooks include: extractMemories, autoDream, skillImprovement, magicDocs, promptSuggestion, sessionMemory. `executePostSamplingHooks()` fires after model response (query.ts:1046-1056).
- **Go**: `HookManager` with `ExecutePreCompactHooks()` returning `PreCompactInput` and `CustomInstructions`. Manual hook execution in `ForceCompact()` (agent_loop.go:1314-1319). No `executePostSamplingHooks` equivalent.
- **Type**: 简化 -- Go has pre-compact hooks but missing post-sampling hooks and the rich hook ecosystem.

#### A.15.2 Stop Hooks

- **Upstream**: `handleStopHooks()` yields blocking errors, prevents continuation, executes stop-failure hooks. `executeStopFailureHooks()` on API errors (query.ts:1314-1322, 1309-1311, 1040-1043).
- **Go**: **Missing** -- No stop hooks or stop-failure hooks in the agent loop.
- **Type**: 缺失

### A.16 Transcript Writing

- **Upstream**: Transcript writing handled by SDK/REPL layer. Messages yielded through generator are serialized by upstream infrastructure. VCR fixtures for testing.
- **Go**: `transcript.Writer` with JSONL format. `WriteUser()`, `WriteAssistant()`, `WriteToolUse()`, `WriteToolResult()`, `Flush()`, `Close()`. `NewAgentLoopFromTranscript()` for session resume with `rebuildContextFromTranscript()` (agent_loop.go:378-382, 494-642).
- **Type**: Go适配 -- Go implements its own transcript layer since there's no REPL/SDK infrastructure to delegate to.

### A.17 @Context Expansion

- **Upstream**: `getAttachmentMessages()` processes attachments including file references, memory prefetches, skill prefetches. `prependUserContext()` adds `<system-reminder>` with CLAUDE.md, git status. `processUserInput` handles slash commands and bash commands (query.ts:1627-1637, api.ts:447-472).
- **Go**: `PreprocessContextReferences()` expands `@file:`, `@diff`, `@commit:` etc. into inline content. `modelContextWindow()` sizing for expansion limits. `Expanded`/`Blocked` flags with warnings (agent_loop.go:901-915).
- **Type**: Go适配 -- Go implements @-expansion as a preprocessing step before the loop, while upstream handles it through attachment processing within the loop.

### A.18 Consecutive Empty Response Handling

- **Upstream**: No equivalent -- upstream's streaming model always yields something (text, tool_use, or error). Empty responses are handled by the stream completing with `stop_reason`.
- **Go**: `consecutiveEmptyResponses` counter with `maxEmptyResponses=3`. Detects thinking-only responses (no text, no tool_calls), injects hint, gives up after 3 attempts (agent_loop.go:928-929, 1107-1121).
- **Type**: Go增强 -- Go handles the edge case of thinking-only responses which upstream doesn't encounter due to its streaming architecture.

### A.19 Skill Tracking

- **Upstream**: `skillPrefetch.startSkillDiscoveryPrefetch()` async discovery during streaming. `collectSkillDiscoveryPrefetch()` consumed at turn end. Skill state tracked via messages.
- **Go**: Manual tracking in Run loop: checks `toolCalls` for `read_skill`, calls `skillTracker.MarkRead()` + `MarkUsed()`. `restoreSkillStateFromEntries()` on transcript resume (agent_loop.go:1151-1163, 748-790).
- **Type**: 简化 -- Go's synchronous skill tracking vs upstream's async prefetch/consume pattern.

### A.20 Conclusion Extraction

- **Upstream**: No equivalent -- upstream relies on session memory extraction via forked agent for durable memory.
- **Go**: `extractConclusions()` with 15+ regex patterns scanning assistant text for code structure, file semantics, task progress, bug/fix, error, and discovery conclusions. Recorded in `toolStateTracker` (agent_loop.go:821-855).
- **Type**: Go增强 -- Regex-based conclusion extraction is Go-original, providing lightweight session state without the overhead of upstream's forked-agent extraction.

### A.21 Post-Compact Recovery

- **Upstream**: `runPostCompactCleanup()` re-injects critical context (recent file edits, CLAUDE.md). Hook results appended. Memory injection (postCompactCleanup.ts).
- **Go**: `PostCompactRecovery()` with `RunPostCompactCleanup()` internally. `injectTruncationContinuation()` for truncation recovery. File freshness tracking via `toolStateTracker.MarkFileFresh()` (agent_loop.go:1338-1343, 1363-1368, 1442-1449).
- **Type**: Go适配 -- Go implements similar post-compact recovery with its own `PostCompactRecovery` function.

### A.22 Cache Edits Injection

- **Upstream**: `CachedMCEditsBlock` insertion in messages array. Pinned edits re-inserted at original positions. Edit deduplication with `seenDeleteRefs`. `cache_reference` on tool_results before last `cache_control` (claude.ts:3214-3294).
- **Go**: `injectCacheEdits()` from `CachedMicrocompactTracker`. Registers compactable tool_use IDs. `MarkSentToAPI()` prevents double-send (agent_loop.go:1487, 1548, 1592, 1933-1937, 1968-1974).
- **Type**: Go适配 -- Go implements a simplified cache_edits mechanism via `CachedMicrocompactTracker`.

### A.23 Preflight Compression

- **Upstream**: Session resume handled by `ResumeConversation.tsx` with `sessionStorage` and `conversationRecovery.ts`. No explicit preflight compression in the query loop.
- **Go**: `preflightThreshold = 100000` tokens. On resume with >100k tokens, tries compaction up to 3 times before entering the main loop (agent_loop.go:932-940).
- **Type**: Go增强 -- Go's preflight compression on session resume is unique.

### A.24 Grace Eviction Ticker

- **Upstream**: Background task cleanup handled by separate infrastructure (session runner, bridge messaging). No grace eviction ticker in the query loop.
- **Go**: Background goroutine with `time.NewTicker(10s)` calling `taskStore.CleanupEvicted()`. Runs independently of the main loop (agent_loop.go:439-453, 594-608).
- **Type**: Go增强 -- Go's grace eviction ticker is a unique addition for managing completed background tasks.

### A.25 Plan Mode Tools

- **Upstream**: Plan mode handled via `toolPermissionContext.mode` and `getRuntimeMainLoopModel()` plan-mode fallback. No dedicated EnterPlanMode/ExitPlanMode tools.
- **Go**: `EnterPlanModeTool` and `ExitPlanModeTool` with `GetMode`/`SetMode`/`GetPrePlanMode` callbacks. Plan mode transitions managed through tools (agent_loop.go:104-118).
- **Type**: Go增强 -- Dedicated plan mode tools are Go-original.

### A.26 Agent Management Tools

- **Upstream**: Sub-agent lifecycle managed through `runForkedAgent()` (forkedAgent.ts) and `LocalAgentTask`. No agent_list/agent_get/agent_kill tools exposed to the model.
- **Go**: `AgentListTool`, `AgentGetTool`, `AgentKillTool` wired to `AgentTaskStore`. Model can query and kill running sub-agents (agent_loop.go:157-165).
- **Type**: Go增强 -- Exposing agent management as tools to the model is Go-original.

### A.27 Summary: Agent Loop Key Differences

#### Critical Gaps (correctness/behavior differences)
1. **No streaming tool executor** -- Go collects all tool calls then executes; upstream streams tools in during model response
2. **No model fallback during streaming** -- Go's fallbackModel is never used in the loop
3. **No orphaned tool_result backfill** -- Missing on streaming fallback and abort (only handled on API 2013 error)
4. **No tool result persistence** -- Truncation-only vs upstream's disk persistence with XML tags
5. **No tool use summary** -- Missing async Haiku summary of tool results
6. **No stop hooks** -- Missing handleStopHooks and executeStopFailureHooks
7. **No token budget tracking** -- Missing TOKEN_BUDGET feature

#### Behavioral Differences
8. **Generator vs blocking** -- Upstream yields events in real-time; Go's Run() blocks until completion
9. **Single vs layered recovery** -- Go uses one-size-fits-all truncation; upstream has collapse -> reactive compact -> max_tokens escalation -> recovery message
10. **Batch vs streaming tool execution** -- Go executes all tools after response; upstream executes as they stream
11. **No concurrency safety per tool** -- Go runs all tools concurrently; upstream respects isConcurrencySafe flag

#### Go-Original Features (enhancements)
12. **Grace call mechanism** -- One extra text-only turn after budget exhaustion
13. **Conclusion extraction** -- Regex-based session state extraction
14. **Todo list injection** -- TodoWrite tool with idle reminders
15. **Preflight compression** -- Auto-compact on session resume >100k tokens
16. **Grace eviction ticker** -- Background task cleanup
17. **Agent management tools** -- agent_list/get/kill exposed to model
18. **Plan mode tools** -- EnterPlanMode/ExitPlanMode as tools
19. **Permission pre-check** -- Sequential pre-check avoids concurrent stdin reads

#### Architecture Differences
20. **State machine vs local variables** -- Upstream uses structured State with 7 transition reasons; Go uses local counters and simple continue
21. **Composable retry vs inline retry** -- Upstream's withRetry() generator vs Go's manual for-loop
22. **Atomic bool vs AbortController** -- Go's interrupted flag vs upstream's AbortController with reason tracking

### A.28 Agent Loop Deep Dive (Section 35)

#### Compaction Integration in Loop

| Aspect | Go (agent_loop.go:3832-4239) | Upstream (query.ts:440-586) | Type |
|--------|-----------------------------|-----------------------------|------|
| Compaction layers | 3 phases: time-based MC -> SM-compact -> LLM-compact. Fallback to truncation | 4 layers: snip compact -> microcompact -> context collapse -> auto-compact | Go适配 (different architecture) |
| SM-compact | Unique to Go -- session memory file as summary | SM-compact in upstream (sessionMemoryCompact.ts) but called differently | 匹配 (both have SM-compact) |
| Post-compact recovery | `PostCompactRecovery()` (L2819-3090): extensive file/tool/agent/MCP/skill/session-memory recovery | `buildPostCompactMessages()`: boundaryMarker, summaryMessages, messagesToKeep, attachments, hookResults | 匹配 (both extensive) |
| Cooldown | `ShouldCompact()` checks 25% token growth (L4007-4011) | Consecutive failures tracked (L579-586) | Go适配 |
| Prefetch | No memory/skill prefetch | `startRelevantMemoryPrefetch()`, `startSkillDiscoveryPrefetch()` parallel during streaming | 缺失 |

#### Hooks Execution

| Aspect | Go (agent_loop.go:3056-3088, 3927-3937) | Upstream (stopHooks.ts:65-481) | Type |
|--------|------------------------------------------|----------------------------------|------|
| Hook types | Pre/post compact hooks only | Stop hooks, teammate idle, task completed, prompt suggestion, auto-dream, extract memories, job classifier | 简化 |
| Hook input/output | `PreCompactInput{Trigger, CustomInstructions}`, `PostCompactInput{Trigger, CompactSummary, RecoveredFiles}` | Comprehensive hook pipeline with MCP cleanup, job state classification, skill activation | 简化 |

---

## Section B: Context Management

> Source: [diff_upstream/02-context.md]

Detailed analysis of conversation context, entry types, token estimation, message building, compaction strategies, microcompact, and @context reference expansion.

### B.1 Entry Type System

| Go (`context.go`) | Upstream (`types/message.ts`, `utils/messages.ts`) |
|---|---|
| `EntryContent` sealed interface with: `TextContent`, `ToolUseContent`, `ToolResultContent`, `CompactBoundaryContent`, `SummaryContent`, `AttachmentContent` | Rich discriminated union `Message` type with `type` field: `user`, `assistant`, `system`, `progress`, `attachment`, `system_informational`, `system_compact_boundary`, `system_away_summary`, `system_agents_killed`, `system_api_error`, `system_api_metric`, `system_microcompact_boundary`, `system_permission_retry`, `system_scheduled_task_fire`, `system_stop_hook_summary`, `system_turn_duration`, `system_thinking`, `system_bridge_status`, `system_memory_saved`, `system_local_command`, `tombstone`, `hook_result`, `grouped_tool_use`, `collapsible` |
| Single `conversationEntry` struct wraps `EntryContent` + `role` + `summarized bool` | Each message type has distinct interface with `uuid: UUID`, `timestamp`, `origin`, `isMeta`, `isCompactSummary`, `isVisibleInTranscriptOnly`, `message.id`, `parentUuid`, and many type-specific fields |
| Compact boundary is a `CompactBoundaryContent` struct with `Trigger`, `PreCompactTokens`, `UUID`, `PreCompactDiscoveredTools`, `PreservedSegment` | `SystemCompactBoundaryMessage` with `compactMetadata` containing `preCompactDiscoveredTools`, `preservedSegment` (`headUuid`, `anchorUuid`, `tailUuid`), `userFeedback`, `messagesSummarized` |
| Summary is `SummaryContent` (string) | Summary is a `UserMessage` with `isCompactSummary: true`, `isVisibleInTranscriptOnly: true`, and optional `summarizeMetadata` |
| Attachment is `AttachmentContent` (string) | `AttachmentMessage<Attachment>` with typed attachments: `file_content`, `invoked_skills`, `plan_mode`, `plan_file_reference`, `task_status`, `agent_listing_delta`, `deferred_tools_delta`, `mcp_instructions_delta`, etc. |
| **Gap**: No `ProgressMessage`, `ThinkingMessage`, `TombstoneMessage`, `HookResultMessage`, `SystemAPIErrorMessage`, `SystemAgentsKilledMessage` | Upstream has many system message types for UI state tracking |

### B.2 Token Estimation

| Go | Upstream |
|---|---|
| `EstimateTokens(string)` in `tokenizer.go` -- uses `DetectContentType` + fixed ratios (code: 3 chars/token, natural: 4 chars/token, JSON: 3.5 chars/token), applies 4/3 safety margin in `EstimatedTokens()` | **Two-tier system**: (1) `countTokensWithAPI()` -- calls `anthropic.beta.messages.countTokens` API for exact counts. (2) `roughTokenCountEstimation()` -- fallback using `content.length / bytesPerToken` (default 4 bytes/token, 2 for JSON). |
| `EstimatedTokens()` is content-type-aware: walks each entry type, classifies text, sums with appropriate ratios | `tokenCountWithEstimation()` is smarter: walks messages backwards, finds last assistant with real `usage` data, uses `input_tokens + cache_tokens + output_tokens` as anchor point, then estimates the *delta* messages between that anchor and end |
| 4/3 margin applied uniformly | `estimateMessageTokens()` in `microCompact.ts` also applies 4/3 pad (`Math.ceil(totalTokens * 4/3)`) |
| **No API token counting** -- Go never calls the count-tokens API | Upstream can call `anthropic.beta.messages.countTokens` for exact counts, falls back to rough estimation when provider (Bedrock/Vertex) doesn't support it |

### B.3 BuildMessages() / API Message Construction

| Go | Upstream |
|---|---|
| `BuildMessages()` scans backwards for last `CompactBoundaryContent`, takes `entries[boundaryIdx:]`, converts each to `anthropic.MessageParam` | `getMessagesAfterCompactBoundary()` uses `findLastCompactBoundaryIndex()`, slices from boundary, then optionally applies `projectSnippedView()` (HISTORY_SNIP feature) |
| Compact boundaries are **not** sent to API (`continue` in switch) | Compact boundaries are **not** sent to API (same behavior) |
| Merges consecutive same-role messages after conversion | `normalizeMessagesForAPI()` handles role merging, tool pairing, thinking block merging by `message.id` |
| System prompt is a separate `c.systemPrompt` field | System prompt composed via `asSystemPrompt()` from `getSystemContext()`, `getUserContext()`, CLAUDE.md, tool lists |
| **Gap**: No `normalizeMessagesForAPI()` -- no tool search field stripping, no incomplete tool call filtering | Upstream's `normalizeMessagesForAPI()` strips `caller` from tool_use, strips `tool_reference` from tool_result, handles `defer_loading` tools |
| **Gap**: No `ensureToolResultPairing()` -- instead has `ValidateToolPairing()` | Upstream's `ensureToolResultPairing()` in `claude.ts` is the authoritative tool repair function |

### B.4 Truncation

| Go | Upstream |
|---|---|
| Three levels: `TruncateHistory()` (keep first + last 10), `AggressiveTruncateHistory()` (first + last 5), `MinimumHistory()` (first + last 2) | No equivalent naive truncation functions. Upstream relies on compaction + message selector UI |
| All three are compact-boundary-aware via `truncateWithBoundary()` | Upstream has `truncateHeadForPTLRetry()` -- drops oldest API-round groups when compact request hits prompt-too-long (last-resort escape hatch) |
| Go's truncation can orphan tool results, fixed by `ValidateToolPairing()` | Upstream uses `adjustIndexToPreserveAPIInvariants()` to prevent splitting tool pairs when calculating preserved tail |
| `truncateIfNeeded()` fires on every message add when over `MaxContextMsgs` | Upstream has no message-count-based truncation -- relies on token-based auto-compact |

### B.5 CompactContext -- Degradation Phases

| Go | Upstream |
|---|---|
| **4-phase degradation**: Phase 1: `Compact()` (round-based, keeps last N rounds). Phase 2: `SmartCompact()` (turn-based, keeps first 2 + last 2 turns). Phase 3: `SelectiveCompact()` (clears readable tool outputs). Phase 4: Hard truncate | **No degradation chain.** Single strategy: `compactConversation()` sends messages to an LLM summarizer. If it fails (prompt-too-long), retries with `truncateHeadForPTLRetry()`. |
| Go compacts **locally** using algorithmic strategies (no LLM call) | Upstream compacts by **calling the API** -- sends full conversation + summary prompt to a forked agent (`runForkedAgent` with cache sharing) or streaming fallback |
| Go has no LLM-assisted summarization | Upstream's `streamCompactSummary()` uses `queryModelWithStreaming()` or cache-sharing forked agent to generate AI summary |
| Upstream also has **session memory compaction** (`trySessionMemoryCompaction()`) as a first strategy before legacy compact | Go has no session memory compaction equivalent |
| Go's compact is **synchronous and deterministic** | Upstream's compact is **async, can fail, has retry loops** (PTL retry up to 3x, streaming retry up to 2x) |

### B.6 MicroCompactEntries

| Go | Upstream |
|---|---|
| `MicroCompactEntries()` -- mutates messages in-place, replaces old tool result text with placeholder `"[Old tool result content cleared]"`. Dedup-aware, whitelist-only for compactable tools, size-threshold-based (>= 2000 chars) | **Two paths**: (1) **Cached microcompact** -- uses server-side `cache_edits` API to delete tool results without invalidating cache prefix. (2) **Time-based microcompact** -- when gap since last assistant exceeds threshold, content-clears old tool results directly |
| Keep window: `keepRecent` most recent tool results preserved | Cached MC: count-based `triggerThreshold` (configurable via GrowthBook), keeps `keepRecent` most recent |
| Tool name lookup via two-pass scan | Tool name lookup via `collectCompactableToolIds()` -- single pass over assistant messages |
| Go has no time-based trigger | Upstream time-based: `evaluateTimeBasedTrigger()` checks gap vs `gapThresholdMinutes` (configurable) |
| Go mutates message content directly | Cached MC does **NOT** mutate messages -- adds `cache_reference` and `cache_edits` at API layer. Time-based MC mutates content directly (cache is already cold) |

### B.7 KeepRecentMessagesAdaptive

| Go | Upstream |
|---|---|
| `KeepRecentMessagesAdaptive(minTokens, minTextMsgs, maxTokens)` -- walks backwards from boundary, accumulates tokens until min constraints met, stops at max cap | `calculateMessagesToKeepIndex()` in `sessionMemoryCompact.ts` -- starts from `lastSummarizedMessageId`, expands backwards to meet `minTokens`, `minTextBlockMessages`, stops at `maxTokens` |
| Skips entries marked `summarized` (incremental compaction) | Uses `lastSummarizedMessageId` from session memory module -- same concept as Go's `summarized` field |
| `adjustForToolPairing()` ensures tool_use/tool_result pairing | `adjustIndexToPreserveAPIInvariants()` -- same purpose, more sophisticated (also handles thinking blocks sharing `message.id`) |
| Updates `lastSummarizedIndex` and `compactedEntryCount` for incremental tracking | Uses `setLastSummarizedMessageId()` and `getLastSummarizedMessageId()` in `sessionMemoryUtils.js` |
| Config: default `minTokens=1000`, `minTextMsgs=4`, `maxTokens=10000` | Default config: `minTokens=10000`, `minTextBlockMessages=5`, `maxTokens=40000` (remotely configurable via GrowthBook `tengu_sm_compact_config`) |
| **Very similar logic** -- Go's implementation closely mirrors upstream's `calculateMessagesToKeepIndex` | Same algorithm with configurable thresholds |

### B.8 ConversationEntry.summarized Field

| Go | Upstream |
|---|---|
| `conversationEntry.summarized bool` -- marks entries already included in a previous compaction summary. `KeepRecentMessagesAdaptive` skips summarized entries | `lastSummarizedMessageId: UUID \| undefined` -- tracks the UUID of the last message included in a compaction summary. Stored in `sessionMemoryUtils.js` module-level state. |
| `lastSummarizedIndex int` -- index boundary for incremental compaction | No index -- uses UUID lookup: `messages.findIndex(msg => msg.uuid === lastSummarizedMessageId)` |
| `compactedEntryCount int` -- count of entries before boundary that were already summarized | Not explicitly tracked -- derived from `messagesToKeep.length` |
| Go marks kept entries as `summarized = true` after each `KeepRecentMessagesAdaptive` call | Upstream sets `setLastSummarizedMessageId(keep[keep.length-1].uuid)` after compaction |
| `summarized` is per-entry in-memory state | `lastSummarizedMessageId` is module-level global state (persisted across turns) |

### B.9 Context Management -- Summary Divergence Table

| # | Divergence | Impact |
|---|---|---|
| 1 | **Go uses algorithmic compaction (4-phase degradation); upstream uses LLM summarization** | Go's compact is faster and free but produces less coherent summaries. Upstream's AI summaries preserve narrative context. |
| 2 | **Go has no session memory compaction** | Missing upstream's `trySessionMemoryCompaction()` first-strategy that uses structured session memory instead of LLM summary. |
| 3 | **Go has no cached microcompact (cache_edits API)** | Upstream can delete old tool results server-side without cache invalidation. Go must mutate content, causing cache miss on every microcompact. |
| 4 | **Go has no `normalizeMessagesForAPI()`** | Missing tool search field stripping, deferred tool filtering, and incomplete tool call handling that upstream does before every API call. |
| 5 | **Go's token estimation is heuristic-only** | No API-based exact counting. May over/under-estimate context usage compared to upstream's `tokenCountWithEstimation()` which anchors on real API usage data. |
| 6 | **Go lacks time-based microcompact trigger** | Upstream clears stale tool results when idle gap exceeds threshold. Go only does count-based clearing. |
| 7 | **Go's compact is synchronous; upstream is async with retries** | Go's compact can fail catastrophically (no retry). Upstream has PTL retry (3x), streaming retry (2x), and fallback paths. |
| 8 | **Go has no `stripReinjectedAttachments()`** | Upstream strips skill_discovery/skill_listing attachments before compact (re-injected post-compact anyway). Go doesn't. |
| 9 | **Go has no post-compact hooks** | Missing `executePreCompactHooks()` and `executePostCompactHooks()` which upstream uses for extensibility. |
| 10 | **Go's `ToolStateTracker` epoch-based vs upstream's `readFileState.clear()`** | Go tracks staleness per-compaction via epoch; upstream simply clears file state and re-reads post-compact via attachments. |

### B.10 context.go Deep Dive -- Architecture & Data Model

#### Architecture

| Go | Upstream |
|---|---|
| `ConversationContext` + `ToolStateTracker` independent structs, protected by `sync.RWMutex` (`context.go:289`, `context.go:104`) | Upstream uses React `useState` + `appState` global state management, via `useReplBridge` and `AppStateStore` |

#### Compaction Details

| Go | Upstream |
|---|---|
| `CompactContext()` serializes via `entriesToCompactionMessages()`, calls `Compact()`, deserializes back to `conversationEntry` | Upstream operates directly on `Message[]` via `buildPostCompactMessages()`, no serialize/deserialize round-trip |
| `entriesToCompactionMessages()` discards all messages before `CompactBoundaryContent` (`context.go:779`) | Upstream uses `getMessagesAfterCompactBoundary()` to skip pre-boundary messages (`utils/messages.ts`) |
| **Go missing**: No Session Memory persistent summary | Upstream `trySessionMemoryCompaction()` reads/writes `.claude/SESSION_MEMORY.md` for cross-session memory reuse |
| **Go missing**: No forked agent summary generation | Upstream generates summaries via `runForkedAgent()` (compact.ts) |

#### Boundary Markers and PreservedSegment

| Go | Upstream |
|---|---|
| `CompactBoundaryContent` has `UUID`, `PreservedSegment` (`context.go:49-72`) for chain re-linking | Upstream `createCompactBoundaryMessage()` + `annotateBoundaryWithPreservedSegment()` equivalent (compact.ts) |
| `KeepRecentMessagesAdaptive()` computes keep range by token budget (minTokens/minTextMsgs/maxTokens) (`context.go:1040-1145`) | Upstream `calculateMessagesToKeepIndex()` + `adjustIndexToPreserveAPIInvariants()` by token budget (sessionMemoryCompact.ts:324-397) |
| `adjustForToolPairing()` forward-searches to complete `tool_use/tool_result` pairs (`context.go:1196-1269`) | Upstream `adjustIndexToPreserveAPIInvariants()` does same, also handles streaming thinking blocks (sessionMemoryCompact.ts:232-314) |

#### ToolStateTracker

| Go | Upstream |
|---|---|
| Uses `epoch` counter to distinguish fresh/stale items (`context.go:104-120`) | Upstream uses `FileReadStateCache` (fileStateCache.ts) tracking read files + search patterns |
| `OnCompaction()` increments epoch, making all prior items stale (`context.go:181-185`) | Upstream clones cache via `cloneFileStateCache()` on fork, isolating sub-agent state (forkedAgent.ts:383) |
| `BuildSessionStateNote()` generates plain-text summary injected into system prompt (`context.go:220-279`) | Upstream puts state as independent system context fields, not concatenated as text |
| `FileUnmodified()` checks file modification via mtime (`context.go:145-157`) | Upstream uses `FileReadState` tracking `contentHash` + stat comparison |
| **Go missing**: No `discoveredSkillNames` tracking | Upstream tracks `discoveredSkillNames` in sub-agent context (forkedAgent.ts:390) |
| **Go missing**: No `contentReplacementState` | Upstream uses `ContentReplacementState` for UUID consistency in tool result replacement (toolResultStorage.ts) |

#### MicroCompact

| Go | Upstream |
|---|---|
| `MicroCompactEntries()` clears old tool result content (`context.go:1285-1409`), with whitelist/dedup/size threshold | Upstream `microCompact.ts` has `estimateMessageTokens()`, similar strategy for clearing old tool results |
| Preserves `ToolUseID` when clearing (`context.go:1389-1395`) | Upstream uses `contentReplacementState` to track replaced tool results for prompt cache hit |
| **Go uses hardcoded `compactableToolNames`** | Upstream configures clearable tool list dynamically via `getDynamicConfig` |

#### Truncate and ValidateToolPairing

| Go | Upstream |
|---|---|
| `ValidateToolPairing()` handles orphaned tool_results and tool_uses (`context.go:1470-1567`) | Upstream `normalizeMessagesForAPI()` + `ensureToolResultPairing()` does same work |
| Inserts synthetic error results for missing tool_results (`context.go:1544-1557`) | Upstream `ensureToolResultPairing` in `claude.ts` does same |
| `FixRoleAlternation()` merges consecutive same-role messages (`context.go:1573-1643`) | Upstream `normalizeMessagesForAPI()` handles same issue |

### B.11 @Context Reference Expansion (context_references.go)

#### Architecture Comparison

| Go | Upstream |
|---|---|
| **Text preprocessing mode**: `PreprocessContextReferences()` parses `@` references before sending, injects content into message (`context_references.go:150-233`) | **UI attachment mode**: Upstream extracts `@` filenames via `extractAtMentionedFiles()`, generates `Attachment[]`, attaches as independent objects (`utils/attachments.ts:1911-1981`) |
| Regex parses: `@file:path`, `@folder:path`, `@diff`, `@staged`, `@git:N`, `@url:url` (`context_references.go:63`) | Upstream regex extracts simple `@filename` only (`utils/attachments.ts:2790-2812`), no `@file:`/`@folder:` prefix syntax |
| Parsed results concatenated as Markdown text blocks injected into message | Parsed results as structured `Attachment` objects (`{type: 'file', path, content}`) |

#### Supported Reference Types

| Reference Type | Go | Upstream |
|---|---|---|
| `@file:path` | Yes, reads file content (`context_references.go:266-375`) | Yes, via `generateFileAttachment()` (`utils/attachments.ts`) |
| `@file:path:10-50` (line range) | Yes, with line numbers (`context_references.go:327-367`) | Yes, `parseAtMentionedFileLines()` parses line range (`utils/attachments.ts`) |
| `@folder:path` | Yes, recursive directory tree (`context_references.go:378-409`) | Yes, `readdir()` lists directory (`utils/attachments.ts:1934-1958`) |
| `@diff` (uncommitted changes) | Yes, calls `git diff` (`context_references.go:412-460`) | **No text `@diff` support** -- diff via UI tool call |
| `@staged` (staged changes) | Yes, calls `git diff --staged` (`context_references.go:244-245`) | **No `@staged` support** |
| `@git:N` (last N commits) | Yes, calls `git log -N -p` (`context_references.go:246-257`) | **No `@git:N` support** |
| `@url:url` | Yes, HTTP fetch for web content (`context_references.go:463-518`) | **No `@url` support** |
| `@"quoted path"` | Yes (`context_references.go:116`) | Yes (`utils/attachments.ts:2790`) |

#### Safety Mechanisms

| Go | Upstream |
|---|---|
| **Sensitive directory blocklist**: `.ssh`, `.aws`, `.gnupg`, `.kube`, `.docker`, `.azure`, `.config/gh`, `.config/git` (`context_references.go:66-69`) | **Upstream uses `isFileReadDenied()`** via permission rules (`utils/attachments.ts:1926-1929`) |
| **Path traversal protection**: `ensurePathAllowed()` checks path must be within CWD (`context_references.go:531-560`) | Upstream expands via `expandPath()` but doesn't enforce CWD restriction |
| Soft/hard token budget: 50% hard reject, 25% warning (`context_references.go:179-206`) | Upstream no token budget limits -- attachments via independent UI rendering |
| **File caching**: `fileCache` global map avoids repeated reads (`context_references.go:49-56`) | Upstream uses `FileReadStateCache` for similar caching |
| File size limit: 10MB max (`context_references.go:286-289`) | Upstream `FileReadTool` has file size limit |
| Line limit: 1000 lines default (`context_references.go:18,298`) | Upstream `generateFileAttachment()` has line limit |

#### HTML Processing

| Go | Upstream |
|---|---|
| `extractHTMLContent()` uses regex to extract web content, removes script/style blocks (`context_references.go:789-831`) | **Upstream uses browser scraping** (Playwright/puppeteer) for page content, not simple regex extraction |
| Extracts `<article>`, `<main>`, `<body>` content | Upstream gets full DOM via browser rendering |
| `extractHTMLTitle()` extracts `<title>` (`context_references.go:834-841`) | Upstream gets page title via browser |

#### Go-Only Features

| Go Has | Upstream Lacks |
|---|---|
| `@diff`/`@staged`/`@git:N` git reference expansion | Upstream accesses git info via separate tools/commands |
| `@url:url` web fetching | Upstream no URL expansion |
| Token budget gating (soft/hard limits) | Upstream attachments no token budget |
| Line range spec (`@file:path:10-50`) parsed at preprocessing stage | Upstream line ranges handled during attachment generation |
| Errors injected as context blocks (`context_references.go:166-171`) | Upstream displays errors via UI |

#### Upstream-Only Features

| Upstream Has | Go Lacks |
|---|---|
| MCP resource references (`@server:uri`) | Go no MCP resource expansion |
| Agent references (`@agent-name`) | Go no agent reference expansion |
| Skill references (`/skill` command) | Go handles via other means |
| VSCode IDE integration (file open notifications) | Go is pure CLI, no IDE integration |

### B.12 Context.go Deep Dive (Section 34)

#### Entry/Content Type System

| Aspect | Go (`context.go`) | Upstream (`types/message.ts`) | Type |
|--------|-------------------|-------------------------------|------|
| Message metadata | `conversationEntry` has `role`, `content`, `summarized` (line 282-286). No UUID, timestamp, parentUuid, isMeta, isCompactSummary | `SystemCompactBoundaryMessage` has UUID, timestamp, compactMetadata with preCompactTokens, preservedSegment (headUuid, anchorUuid, tailUuid), userFeedback, messagesSummarized | 简化 |
| Compact boundary metadata | `CompactBoundaryContent` (line 49-73) has UUID, PreCompactTokens, PreCompactDiscoveredTools, PreservedSegment -- but no `userFeedback` or `messagesSummarized` | `compactMetadata` carries `userFeedback` for human feedback preservation and `messagesSummarized` count | 简化 |
| Rich message types | 5 variants: TextContent, ToolUseContent, ToolResultContent, CompactBoundaryContent, SummaryContent, AttachmentContent | 8+ types: UserMessage, AssistantMessage, SystemMessage, AttachmentMessage, HookResultMessage, SystemCompactBoundaryMessage, SystemMicrocompactBoundaryMessage, ProgressMessage | 简化 |

#### Token Estimation Algorithm

| Aspect | Go (`context.go`) | Upstream (`tokens.ts`) | Type |
|--------|-------------------|------------------------|------|
| Base estimation | `EstimateTokens()`: `len(text) / 4` (line 37-42). Upstream uses `/ 3.5` -- Go is ~14% less conservative | `roughTokenCountEstimation(text)`: `text.length / 3.5` with 4/3 padding | 简化 |
| Content-type-aware | `EstimateContentTokens()`: code=3.5, json=3.0, tool_use=3.0+10, tool_result=3.0+5, default=4.0. Uses `DetectContentType()` heuristics (`func `, `var `, `class `) | No content-type differentiation. Uses uniform 3.5 ratio. Relies on TypeScript block types, not content inspection | Go适配 |
| 4/3 padding | `math.Ceil(rawTotal * 4.0 / 3.0)` (line 368) -- matches upstream | `Math.ceil(totalTokens * (4/3))` (`microCompact.ts:164-204`) | 匹配 |

#### BuildMessages() -- Entries to API Message Params

| Aspect | Go (`context.go:473-546`) | Upstream (`messages.ts`) | Type |
|--------|--------------------------|--------------------------|------|
| Thinking/redacted_thinking | NOT handled -- no thinking block or redacted_thinking support | Handled -- merges streaming-chunk messages by `message.id`, preserves thinking blocks | 缺失 |
| Image/document blocks | NOT handled -- stripped before compaction | Handled -- strips errored images/documents, replaces with text | 简化 |
| Attachment/hook_result/progress messages | NOT handled | Handled -- attachment messages, hook_result messages, progress messages | 缺失 |
| Tool pairing validation | `ValidateToolPairing()` (line 1470-1567): 3 passes -- collect IDs, remove orphans, insert synthetic results | `ensureToolPairing()` in `normalizeMessagesForAPI()`: ~150 lines, handles thinking blocks sharing message.id | 简化 |
| Same-role merging | `FixRoleAlternation()` (line 1573-1643): type-aware merging (TextContent+TextContent, SummaryContent+TextContent, etc.) | `normalizeMessagesForAPI()` enforces user/assistant alternation, merges consecutive same-role messages | 匹配 |

#### CompactContext() -- Multi-Phase Degradation Chain

| Aspect | Go (`context.go:666-716`) | Upstream (`compact.ts`) | Type |
|--------|--------------------------|--------------------------|------|
| Degradation phases | 4 phases: Compact (round-based) -> SmartCompact (turn-based) -> SelectiveCompact (clear readable) -> AggressiveTruncateHistory | LLM-driven compaction via forked agent or streaming fallback with PTL retry loop | Go适配 |
| Hooks | No pre/post compact hooks in context.go | `executePreCompactHooks()`, `executePostCompactHooks()`, `processSessionStartHooks()` | 缺失 |
| Skill/plan attachment | No skill attachment re-injection, no plan mode attachment | `createSkillAttachmentIfNeeded()` (25K budget), `createPlanAttachmentIfNeeded()` | 缺失 |
| Delta re-announcement | No delta tools/agents/MCP re-declaration | `getDeferredToolsDeltaAttachment()`, `getAgentListingDeltaAttachment()`, `getMcpInstructionsDeltaAttachment()` | 缺失 |

#### Microcompact Entry Clearing

| Aspect | Go (`context.go:1285-1409`) | Upstream (`microCompact.ts`) | Type |
|--------|--------------------------|--------------------------|------|
| Time-based microcompact | `MicroCompactEntries()`: gap-based clearing with dedup, whitelist, size-threshold (minCharCount=2000) | `maybeTimeBasedMicrocompact()`: gap > 60min threshold (from GrowthBook `tengu_slate_heron`), content-clears with `TIME_BASED_MC_CLEARED_MESSAGE` | Go适配 (Go more sophisticated on dedup/whitelist) |
| GrowthBook remote config | Hardcoded defaults | `gapThresholdMinutes` from GrowthBook remote config | 缺失 |
| Thinking block clearing | No thinking block clearing | `clear_thinking_20251015` with `keep: all` or `keep: {value: 1}` | 缺失 |

#### TruncateHistory Methods

| Aspect | Go (`context.go:576-655`) | Upstream | Type |
|--------|--------------------------|----------|------|
| Truncation levels | 3 levels: TruncateHistory(keep 10), AggressiveTruncateHistory(keep 5), MinimumHistory(keep 2). All use `truncateWithBoundary()` | No direct `truncateHistory` method -- handled via reactive compact, context collapse, `truncateHeadForPTLRetry()` | Go适配 (standalone API for CLI fallback) |

---

## Section C: Streaming

> Source: [diff_upstream/04-streaming.md]

Detailed analysis of streaming implementation, SSE handling, event processing, stall detection, and all streaming-adjacent subsystems including prompt caching, system prompt construction, hooks, session memory, retry utilities, rate limits, and normalization.

### C.1 Streaming Implementation (streaming.go)

**Intro**: Go implements a full custom streaming layer (1006 lines) with typed SDK events, chunk accumulation, stall detection, and terminal display handling. The upstream handles streaming in claude.ts (~3500+ lines total) with manual content block tracking.

#### C.1.1 SSE Parsing Approach

| Aspect | Go | Upstream TS |
|--------|-----|-------------|
| Parser | SDK typed union: `anthropic.MessageStreamEventUnion` via `ssestream.Stream` | Raw SSE events with `part.type` string matching |
| Event dispatch | `switch e := event.AsAny().(type)` on typed SDK unions (line 836) | `switch (part.type)` on string enum (line 2035) |
| Content block tracking | N/A -- delegates to SDK typed content block | Manual `contentBlocks[part.index]` array for accumulating blocks across deltas (line 2054) |

**Finding**: Go relies on the SDK's type system to provide fully typed events, while upstream manually tracks content blocks by index and accumulates deltas.

#### C.1.2 Event Types Handled

| Event Type | Go (line) | Upstream TS (line) | Notes |
|------------|-----------|-------------------|-------|
| `message_start` | 837 (no-op) | 2036 (captures usage, research, ttft) | Upstream captures `research` field for internal use; Go ignores |
| `content_block_start` | 839-853 (tool_use only) | 2051-2107 (tool_use, server_tool_use, text, thinking) | **Go misses**: server_tool_use, text, thinking block starts |
| `content_block_delta` | 855-867 | 2109-2225 | **Go handles**: text, input_json, thinking. **Upstream additionally handles**: citations_delta, signature_delta, connector_text_delta |
| `content_block_stop` | 869-873 | 2227-2268 | Both handle equivalently |
| `message_delta` | 875-890 | 2270-2350 | Upstream additionally handles refusal messages, max_tokens errors, context_window_exceeded |
| `message_stop` | Not explicitly handled | 2352 (no-op) | Minor difference |

#### C.1.3 Thinking/Reasoning Block Handling

| Aspect | Go | Upstream TS |
|--------|-----|-------------|
| Thinking delta | Yes -- `anthropic.ThinkingDelta` at streaming.go:945-951 | Yes -- `case 'thinking_delta'` at line 2204 |
| Thinking block start | **No** -- not handled in content_block_start switch | Yes -- initializes `thinking: ''` and `signature: ''` at line 2086-2093 |
| Thinking signature (`signature_delta`) | **No** | Yes -- captured at line 2183-2202 |
| Redacted thinking | **No** -- completely missing | Handled -- `contentBlock.type !== 'redacted_thinking'` exclusion at line 653 |

**Gap**: Go has no support for redacted thinking blocks or thinking signature deltas. Redacted thinking content from the API would be silently dropped.

#### C.1.4 Cache Creation/Hit Events

| Aspect | Go | Upstream TS |
|--------|-----|-------------|
| Token usage tracking | Basic: input_tokens, output_tokens at streaming.go:880-889 | Full: input_tokens, output_tokens, cache_creation_input_tokens, cache_read_input_tokens at lines 3015-3110 |
| Cache creation tokens | **No** | Tracked in `updateUsage` function (line 3000+) |
| Cache read tokens | **No** | Tracked in `updateUsage` function |
| cache_creation.ephemeral_1h/5m | **No** | Tracked at lines 3042-3049 |
| Cache break detection | **No** | `checkResponseForCacheBreak` at line 2441 |

**Gap**: Go's `Usage` struct (streaming.go:44-47) only tracks input/output tokens, missing all cache-related token counters.

#### C.1.5 Streaming Error Recovery

| Aspect | Go | Upstream TS |
|--------|-----|-------------|
| Stall detection | Yes -- goroutine with configurable timeouts at streaming.go:788-823 | Yes -- idle timeout watchdog with performance.now() at lines 1960-2000 |
| Stall timeouts | 300s default stall, 600s startup (lines 777-784) | STALL_THRESHOLD_MS constant |
| Partial tool call recovery | `ClearPartialToolCall()`, `HasPartialToolCall()`, `HasTruncatedToolArgs()` at lines 212-267 | Stream abort + retry with `withRetry` |
| Model fallback | **No** | `fallbackModel` option with `onStreamingFallback` callback (lines 700-701, 2532) |
| Retry framework | Manual -- caller handles retry | `withRetry()` with CannotRetryError, FallbackTriggeredError (line 257-263) |
| Context window exceeded handling | **No** | Yields error message at lines 2336-2349 |
| Refusal handling | **No** | `getErrorMessageIfRefusal` at line 2315 |
| Stream completion validation | Basic -- checks stream.Err() | Validates `partialMessage` and `stopReason` at lines 2407-2421, triggers non-streaming fallback |

#### C.1.6 Rate Limit Header Parsing

| Aspect | Go | Upstream TS |
|--------|-----|-------------|
| Response header parsing | **No** | `extractQuotaStatusFromHeaders` at line 2457 |
| Rate limit bucket tracking | **No** | `currentLimits`, `extractQuotaStatusFromError` (line 100-101) |
| Bedrock adapter headers | **No** | `bedrockAdapter.parseHeaders` at line 2464 |

#### C.1.7 Additional Upstream Events Not in Go

| Event/Delta | Upstream Location | Description |
|-------------|-------------------|-------------|
| `server_tool_use` | claude.ts:2059-2073 | Server-side tool calls (advisor, etc.) |
| `advisor_tool_result` | claude.ts:2100-2104 | Advisor tool result tracking |
| `connector_text_delta` | claude.ts:2122-2137 | Connector text streaming |
| `citations_delta` | claude.ts:2140-2141 | Citation streaming |
| `signature_delta` | claude.ts:2183-2202 | Thinking block signature |
| `redacted_thinking` | claude.ts:653 | Redacted thinking blocks |

### C.2 Streaming Features and Enhancements

#### C.2.1 DeltasState Tracking vs Upstream

- **Upstream**: `withRetry.ts` tracks `deltasState` conceptually inside `query.ts` -- a `previousDeltas` field tracks whether text/tool deltas have been emitted. The retry loop checks this to decide if retry is safe. `query.ts:~1800` has `hasEmittedDeltas` tracking.
- **Go**: `streaming.go:693-699` defines `DeltasState` enum (`none`, `text_only`, `tool_in_flight`) and `trackDeltaState()` at line 991-1005. This is an **explicit Go enhancement** -- upstream relies on scattered booleans rather than a formal state machine.
- **Type**: Go增强

#### C.2.2 Tool-Use-As-Text Detection

- **Upstream**: `claude.ts` has no explicit detection of the model echoing tool_use JSON as text. Relies on the model not doing this, or SDK parsing handling it.
- **Go**: `streaming.go:131-146` implements heuristic detection: when a text chunk contains >=2 of 3 structural markers (`"type":"tool_use"`, `"id":"`, `"name":"`), it flags `toolUseAsText=true` and discards the chunk. This prevents the model from "getting stuck" outputting raw tool JSON.
- **Type**: Go增强

#### C.2.3 Think Filter State Machine

- **Upstream**: `claude.ts` does not have a `<thinking>` tag filter for terminal display. Thinking blocks handled at API level via `thinking_delta` events and `redacted_thinking` blocks.
- **Go**: `streaming.go:300-397` implements `ThinkFilterState` -- a 4-state machine (`ThinkNormal`, `ThinkInTag`, `ThinkInBlock`, `ThinkClosing`) that wraps `<thinking>...</thinking>` and `<｜begin▁of▁sentence｜>...<｜end▁of▁sentence｜>` blocks in ANSI dim escape codes. This is a **terminal display feature** not present in upstream.
- **Type**: Go增强

#### C.2.4 TerminalHandler Tool Arg Summary

- **Upstream**: Tool call display handled by React components (`AssistantToolUseMessage.tsx`, `BashPermissionRequest.tsx`) with rich formatting, progress indicators, and expandable detail.
- **Go**: `streaming.go:470-562` implements `TerminalHandler.flushToolCall()` and `toolArgSummary()` -- a compact single-line summary for each tool call (e.g., file path for file_read, command for exec, pattern for grep). No upstream equivalent since upstream uses a GUI.
- **Type**: Go增强

#### C.2.5 StreamBus Pub/Sub

- **Upstream**: No equivalent. Uses React state/context for distributing stream events to UI components.
- **Go**: `streaming.go:569-621` implements `StreamBus` -- a goroutine-safe pub/sub with buffered channels (capacity 100), non-blocking publish, and auto-drop on full subscribers. Needed because Go lacks React's reactivity model.
- **Type**: Go适配

#### C.2.6 StreamProgress (TTFB + Throughput)

- **Upstream**: TTFB tracked via `ttftMs = Date.now() - start` at `claude.ts:2038`. Throughput not explicitly tracked.
- **Go**: `streaming.go:627-685` implements `StreamProgress` with `RecordFirstByte()`, `RecordTokens(n)`, `TTFB()`, and `Throughput()` -- thread-safe with mutex. Exposed via `StreamSnapshot`.
- **Type**: Go增强

#### C.2.7 CollectHandler vs Upstream Stream Accumulation

- **Upstream**: `claude.ts` accumulates streaming content in `partialMessage` object and `contentBlocks[]` array. Usage tracked separately. Final result is the full API response message.
- **Go**: `streaming.go:59-298` implements `CollectHandler` which collects text, tool calls, thinking, usage, and error state into separate fields. `StreamResultFrom()` at line 96-115 assembles the final `StreamResult` with `Completed` and `FinishReason` fields. `AsParsedResponse()` at line 271-298 provides a SDK-independent representation. This is a **cleaner abstraction** than upstream's raw `partialMessage` approach.
- **Type**: Go增强

### C.3 Prompt Caching

#### C.3.1 1h TTL Eligibility Gating

- **Upstream**: `claude.ts:385-420` gates 1h TTL on: (1) Bedrock users with `ENABLE_PROMPT_CACHING_1H_BEDROCK` env var, (2) ant users, (3) ClaudeAI subscribers NOT in overage. Eligibility latched session-stable. GrowthBook allowlist controls which `querySource` patterns get 1h.
- **Go**: `prompt_caching.go:24-26` uses simple string check `if ttl == "1h"` with no eligibility gating, no GrowthBook, no subscriber checks, no session latching.
- **Type**: 简化

#### C.3.2 Streaming Layer (Section 39 Comparison)

| Aspect | Go (`streaming.go`) | Upstream (`utils/stream.ts`, `claude.ts`) | Type |
|--------|--------------------|-----------------------------------------|------|
| Architecture | Full custom streaming layer (1006 lines): chunk types, StreamBus pub/sub, CollectHandler, TerminalHandler, StreamAdapter | Simple async iterator queue pattern (76 lines) -- delegates to SDK | Go适配 (more elaborate) |
| Stall detection | Dynamic timeouts (300s/600s), goroutine-based | Activity signals (`sendSessionActivitySignal`, `setSDKStatus`) -- no explicit stall detection | Go适配 |
| Tool detection | `toolUseAsText` requires 2-of-3 structural markers | `ensureToolPairing` in `claude.ts` | 简化 |

### C.4 System Prompt

#### C.4.1 Prompt Construction Architecture

- **Upstream**: `systemPrompt.ts:41-123` implements `buildEffectiveSystemPrompt()` with 5 priority levels: (0) override system prompt -> replaces all; (1) coordinator system prompt; (2) agent system prompt (proactive mode appends, otherwise replaces); (3) custom system prompt; (4) default system prompt. `appendSystemPrompt` always appended (except override). Cached/uncached parts separated via `systemPromptSection()` and `DANGEROUS_uncachedSystemPromptSection()`.
- **Go**: `system_prompt.go:225-355` implements `BuildSystemPrompt()` with simpler model: static part (environment, tools, rules) + dynamic part (mode, project instructions, skills). No coordinator mode, no override, no proactive mode, no agent priority system. Cached version `GetOrBuild()` uses `staticDirty`/`dynamicDirty` flags (line 404).
- **Type**: 简化

#### C.4.2 Section Content Differences

- **Upstream**: `prompts.ts` builds system prompt from dozens of `systemPromptSection()` calls, including: environment info, git context, tool descriptions, rules, dangerous patterns, output style, MCP tool descriptions, scratchpad instructions, agent instructions, skill instructions, plan mode, poor mode, memory injection, todo injection. Conditionally included based on feature flags, user type, and query source.
- **Go**: `system_prompt.go:23-170` uses a single large template string `systemPromptTemplateStatic` with `%s` placeholders. Dynamic template `systemPromptTemplateDynamic` has `%s` for mode, mode description, project instructions, skills, and context retention count. Far fewer conditional sections.
- **Type**: 简化

#### C.4.3 Git Context

- **Upstream**: `prompts.ts` includes git context via `getIsGit()` (boolean) and `getCurrentWorktreeSession()` for worktree info. Git status details not in system prompt.
- **Go**: `system_prompt.go:347` calls `tools.GetGitContext()` which returns detailed git context (branch, remote, status). Injected as `%s` placeholder in static template.
- **Type**: Go增强

#### C.4.4 Output Style System

- **Upstream**: `prompts.ts:29` imports `getOutputStyleConfig` and supports multiple output styles (Explanatory, Learning, Custom) injected into system prompt. Each style has its own system prompt section.
- **Go**: `system_prompt.go` has no output style system. Only the default prompt.
- **Type**: 缺失

#### C.4.5 Memory Injection Point

- **Upstream**: Memory (session memory) injected into system prompt via `systemPromptSection('memory', ...)` which calls `getSessionMemoryContent()`.
- **Go**: `system_prompt.go:239-241` correctly does NOT inject session memory into system prompt during normal conversation. It is only used during compaction as a user message (SM-compact), matching upstream's behavior.
- **Type**: 匹配

#### C.4.6 Skill System

- **Upstream**: Skills managed via `SkillTool` and `skillSearch/` module. Skill instructions loaded dynamically and injected as tool descriptions.
- **Go**: `system_prompt.go:244-333` has its own skill system with `SkillTracker`, `GetAlwaysSkills()`, `GetUnsentSkills()`, `BuildSystemPrompt()`, and `BuildSkillsSummary()`. Skills injected in three ways: (1) always-on skills in system prompt, (2) "New This Turn" section with 4000-char budget, (3) skills summary for previously-shown skills.
- **Type**: Go适配

#### C.4.7 Todo List Injection

- **Upstream**: `prompts.ts` injects todo list items into system prompt via `systemPromptSection('todo', ...)`. Todo section refreshed every turn.
- **Go**: `system_prompt.go` does not inject todo list into system prompt. The `TodoWrite` tool description mentions tracking, but there's no dynamic injection of current todo items.
- **Type**: 缺失

### C.5 Auto Classifier

#### C.5.1 Two-Stage Classification

- **Upstream**: `yoloClassifier.ts` uses a single API call with extended thinking (budget_tokens up to 10240). The model produces a `thinking` block then a `tool_use` block with the classification. Uses `sideQuery()` for the API call which shares the parent's prompt cache.
- **Go**: `auto_classifier.go:654-846` implements explicit two-stage classification: Stage 1 (fast, 2112 max_tokens) for quick allow/block, Stage 2 (thinking, 6144 max_tokens) with richer analysis prompt when Stage 1 blocks. This is a **Go-specific design** -- the two-stage approach reduces latency for the common "allow" case.
- **Type**: Go增强

#### C.5.2 Whitelist Granularity

- **Upstream**: `yoloClassifier.ts` does not have an explicit whitelist. Uses `bashClassifier.ts` for bash command classification with regex patterns for dangerous commands. Tool-level allow/deny done by the permission system.
- **Go**: `auto_classifier.go:48-124` has explicit `AUTO_MODE_SAFE_TOOLS`, `SAFE_GIT_OPERATIONS`, `SAFE_PROCESS_OPERATIONS`, `SAFE_FILEOPS_OPERATIONS`, and `SAFE_EXEC_PREFIXES` whitelists with 40+ safe tool names and 25+ safe command prefixes. Also has `DANGEROUS_EXEC_PATTERNS` with 20+ dangerous patterns. `IsAutoAllowlisted()` (line 533) checks these before calling the LLM classifier.
- **Type**: Go增强

#### C.5.3 Path Validation for Removal

- **Upstream**: `pathValidation.ts` has `isDangerousRemovalPath()` checking: root `/`, home dir `~`, direct children of root, wildcard `*`/`/*`, Windows drive roots and protected dirs (Windows, Users, Program Files). Part of permission pipeline.
- **Go**: `auto_classifier.go:307-384` implements `isDangerousRemovalPath()` with equivalent checks plus Windows-specific `C:\Windows`, `C:\Users`, `C:\Program Files` detection using regex. Also has `extractRemovalPaths()` and `checkDangerousRemovalPaths()` for parsing rm commands. Integrated directly into the classifier.
- **Type**: Go适配

#### C.5.4 Fail-Open vs Fail-Closed

- **Upstream**: `yoloClassifier.ts` fails-closed when unavailable (returns "deny" by default). Uses `CannotRetryError` on classifier API failures.
- **Go**: `auto_classifier.go:621-627` fails-closed when classifier is unavailable (returns `Allow: false`). However, `callStage2()` at line 823-827 **fails-open** on Stage 2 API error: `Allow: true, Reason: "classifier unavailable (stage 2 error); action allowed by default"`. This is a security difference.
- **Type**: 差异

#### C.5.5 Cache Design

- **Upstream**: `yoloClassifier.ts` uses `lastClassifierRequests` in bootstrap state -- a simple map keyed by tool name + input hash. No explicit TTL; cache invalidated on compaction via `clearClassifierCache()`.
- **Go**: `auto_classifier.go:36-43` implements `cacheEntry` with explicit `expiresAt` (5-minute TTL). `cacheKey()` at line 1139-1173 generates specialized keys per tool type (exec by command prefix, git by operation, fileops by operation+path, etc.). `ClearCache()` called post-compaction.
- **Type**: Go增强

### C.6 Hooks

#### C.6.1 Event Types

- **Upstream**: `hooks.ts` (1000+ lines) supports 20+ hook event types: `PreToolUse`, `PostToolUse`, `PostToolUseFailure`, `PreCompact`, `PostCompact`, `SessionStart`, `SessionEnd`, `Setup`, `Stop`, `StopFailure`, `SubagentStart`, `SubagentStop`, `TeammateIdle`, `TaskCreated`, `TaskCompleted`, `ConfigChange`, `CwdChanged`, `FileChanged`, `InstructionsLoaded`, `UserPromptSubmit`, `PermissionRequest`, `Elicitation`, `StatusLine`, `FileSuggestion`, `PermissionDenied`, `Notification`. Hooks run as shell commands with JSON input on stdin and JSON output on stdout.
- **Go**: `hooks.go` (182 lines) only supports `PreCompact` and `PostCompact` hook events. No shell command execution -- hooks are Go function callbacks (`PreCompactHandler`, `PostCompactHandler`). No JSON stdin/stdout protocol. No timeout configuration per hook (uses fixed 5s default).
- **Type**: 简化

#### C.6.2 Shell Command Execution

- **Upstream**: `hooks.ts:185+` executes hooks as child processes (`spawn`) with configurable shell (detected per platform: bash, PowerShell, Git Bash on Windows). Hook input serialized to JSON, piped via stdin. Hook output parsed from stdout with `hookJSONOutputSchema`. Supports sync (blocking) and async (background) hooks. Timeout is `TOOL_HOOK_EXECUTION_TIMEOUT_MS = 10 * 60 * 1000` (10 minutes).
- **Go**: `hooks.go` hooks are Go functions, not shell commands. No child process execution, no JSON serialization, no shell detection, no async hooks.
- **Type**: 缺失

#### C.6.3 Hook Matcher System

- **Upstream**: `hooks.ts` supports `HookMatcher` with tool name patterns, event type filtering, and `PluginHookMatcher`/`SkillHookMatcher` variants. Hooks configurable via `settings.json` `hooks` field with `matcher` objects. `getSessionHooks()` provides session-derived hooks.
- **Go**: `hooks.go` has no matcher system. Hooks are registered programmatically by name.
- **Type**: 缺失

#### C.6.4 HTTP Hook Support

- **Upstream**: `execHttpHook.ts` supports HTTP-based hooks that POST to a URL instead of running shell commands. Used for webhooks and server-side integrations.
- **Go**: No HTTP hook support.
- **Type**: 缺失

#### C.6.5 Session-End Hook Timeout

- **Upstream**: `hooks.ts:176-183` has `SESSION_END_HOOK_TIMEOUT_MS_DEFAULT = 1500` with env var override `CLAUDE_CODE_SESSIONEND_HOOKS_TIMEOUT_MS`. Session-end hooks must be fast since they block shutdown.
- **Go**: `hooks.go:113` uses `5 * time.Second` default for all hooks, no special session-end handling.
- **Type**: 简化

### C.7 Session Memory

#### C.7.1 Extraction Mechanism

- **Upstream**: `sessionMemory.ts:140+` uses `runForkedAgent()` which creates a sub-agent that shares the parent's prompt cache. The sub-agent reads the current session memory file and uses `Edit` tool to update it. Extraction triggered by `registerPostSamplingHook()` (post-sampling hook system). Forked agent runs asynchronously, shares cache via `createCacheSafeParams()`.
- **Go**: `session_memory.go:830-901` has `sessionMemoryUpdatePrompt()` and `createMemoryFileCanUseTool()` -- prompt and tool filter for forked extraction agent. Actual extraction call happens in `agent_loop.go`. The prompt template at line 842-879 matches upstream's `getDefaultUpdatePrompt()` from `prompts.ts:43-81` almost verbatim.
- **Type**: Go适配

#### C.7.2 Token Budget Constants

- **Upstream**: `prompts.ts:8-9` defines `MAX_SECTION_LENGTH = 2000` and `MAX_TOTAL_SESSION_MEMORY_TOKENS = 12000`. The 10-section template is exactly 2000 tokens per section and 12000 total.
- **Go**: `session_memory.go:54-55` defines `maxTokensPerSection = 20000` and `maxTotalSessionMemoryTokens = 60000` -- **5x the upstream limits**. Comment says: "Increased to 20000 per section and 60000 total to preserve more context across compaction cycles. 12000 total tokens is insufficient for long coding sessions."
- **Type**: Go增强

#### C.7.3 Entry Expiration

- **Upstream**: `sessionMemoryUtils.ts` does not have explicit entry expiration. Entries persist until overwritten by forked agent. State entries implicitly refreshed each extraction.
- **Go**: `session_memory.go:59-66` has explicit entry expiration: `entryExpirationState = 7 days`, `entryExpirationOther = 30 days`. `removeExpiredEntries()` called on load. Per-category max entries (`maxStateEntries = 20`, `maxDecisionEntries = 30`, etc.) prevent unbounded growth.
- **Type**: Go增强

#### C.7.4 Custom Template Loading

- **Upstream**: `prompts.ts:86-101` implements `loadSessionMemoryTemplate()` which reads a custom template from `~/.claude/session-memory/config/template.md`. Falls back to `DEFAULT_SESSION_MEMORY_TEMPLATE` if file not found.
- **Go**: `session_memory.go:427-429` always returns `defaultSessionMemoryTemplate` -- no custom template file loading. `IsSessionMemoryTemplateOnly()` at line 435-437 checks if content equals the default template.
- **Type**: 缺失

#### C.7.5 GrowthBook Remote Config

- **Upstream**: `sessionMemory.ts:82-94` checks `getFeatureValue_CACHED_MAY_BE_STALE('tengu_session_memory', false)` for feature gate and `getDynamicConfig_CACHED_MAY_BE_STALE('tengu_sm_config', {})` for remote configuration of thresholds. Changeable server-side without client updates.
- **Go**: `session_memory.go:910-914` uses hardcoded constants (`minimumMessageTokensToInit = 10000`, `minimumTokensBetweenUpdate = 5000`, `toolCallsBetweenUpdates = 3`). No remote config.
- **Type**: 简化

#### C.7.6 Feature Gate

- **Upstream**: `sessionMemory.ts:82-83` has `isSessionMemoryGateEnabled()` which checks GrowthBook feature gate `tengu_session_memory`. If the gate is off, session memory is completely disabled.
- **Go**: No feature gate -- session memory is always enabled when the feature is configured.
- **Type**: 简化

### C.8 Retry Utilities

#### C.8.1 withRetry Architecture

- **Upstream**: `withRetry.ts:170-517` implements `withRetry<T>()` as an `AsyncGenerator<SystemAPIErrorMessage, T>` -- yields status messages during retries while waiting. Handles: 529 overloaded errors, 429 rate limits, 401/403 auth errors, OAuth token refresh, Bedrock/GCP credential refresh, max_tokens context overflow adjustment, fast mode fallback, persistent retry for unattended sessions, model fallback with `FallbackTriggeredError`. 12+ error classification functions (`is529Error`, `isOAuthTokenRevokedError`, `isBedrockAuthError`, `isVertexAuthError`, `isFastModeNotEnabledError`, etc.).
- **Go**: `retry_utils.go` (71 lines) only provides `jitteredBackoff()` -- a standalone utility computing `min(base * 2^(attempt-1), maxDelay) + jitter`. No error classification, no retry loop, no auth handling, no model fallback. The actual retry logic is in `agent_loop.go` using `jitteredBackoff()` as one component.
- **Type**: 简化

#### C.8.2 Backoff Parameters

- **Upstream**: `withRetry.ts:52-56` uses `BASE_DELAY_MS = 500`, `DEFAULT_MAX_RETRIES = 10`, `MAX_529_RETRIES = 3`. `getRetryDelay()` computes `baseDelay * 2^(attempt-1) + random(0, 0.25 * baseDelay)` capped at `maxDelayMs = 32000` (32 seconds). Respects `retry-after` header.
- **Go**: `retry_utils.go:22-46` uses `baseDelay = 5s`, `maxDelay = 120s`, `jitterRatio = 0.5`. Computes `min(5s * 2^(attempt-1), 120s) + random(0, 0.5 * delay)`. No `retry-after` header handling. No `MAX_529_RETRIES` concept.
- **Type**: 简化

#### C.8.3 Persistent Retry (Unattended Sessions)

- **Upstream**: `withRetry.ts:96-104` implements persistent retry for unattended sessions (`CLAUDE_CODE_UNATTENDED_RETRY` env var). Retries 429/529 indefinitely with `PERSISTENT_MAX_BACKOFF_MS = 5 * 60 * 1000` (5 min cap) and `PERSISTENT_RESET_CAP_MS = 6 * 60 * 60 * 1000` (6 hour cap). Uses chunked sleep with `HEARTBEAT_INTERVAL_MS = 30_000` for keep-alive yields. Resets attempt counter.
- **Go**: No persistent retry mode.
- **Type**: 缺失

#### C.8.4 Fast Mode Cooldown

- **Upstream**: `withRetry.ts:267-305` implements fast mode cooldown on 429/529: if `retry-after` < 20s, retry immediately with fast mode still active. Otherwise, enter cooldown (switch to standard speed) with `DEFAULT_FAST_MODE_FALLBACK_HOLD_MS = 30 minutes` and `MIN_COOLDOWN_MS = 10 minutes`. Also handles fast mode rejection (org doesn't have fast mode) with permanent disable.
- **Go**: No fast mode concept, no cooldown logic.
- **Type**: 缺失

#### C.8.5 Model Fallback on 529

- **Upstream**: `withRetry.ts:327-365` tracks `consecutive529Errors` and after `MAX_529_RETRIES = 3`, triggers `FallbackTriggeredError` which switches to `fallbackModel` (e.g., Opus -> Sonnet). Logs `tengu_api_opus_fallback_triggered` analytics event.
- **Go**: No model fallback mechanism.
- **Type**: 缺失

#### C.8.6 Context Overflow Auto-Adjustment

- **Upstream**: `withRetry.ts:388-427` detects `max_tokens` context overflow errors (status 400, message "input length and `max_tokens` exceed context limit"). Parses `inputTokens + maxTokens > contextLimit` from error message. Adjusts `retryContext.maxTokensOverride` to fit within available context minus 1000 safety buffer. Ensures minimum `FLOOR_OUTPUT_TOKENS = 3000`.
- **Go**: No context overflow handling in retry logic. Handled separately in `agent_loop.go` when `stop_reason == "max_tokens"`.
- **Type**: 简化

#### C.8.7 Connection Error Handling

- **Upstream**: `withRetry.ts:112-118` detects stale connections (`ECONNRESET`, `EPIPE`) and disables keep-alive on retry via `disableKeepAlive()`. Reconnects with fresh client.
- **Go**: No connection error detection or keep-alive management.
- **Type**: 缺失

### C.9 Rate Limit

#### C.9.1 Architecture Difference

- **Upstream**: Rate limit tracking primarily server-side (ClaudeAI subscription limits). `ClaudeAILimits` with: `status` (`allowed`/`allowed_warning`/`rejected`), `rateLimitType` (`seven_day`/`five_hour`/`seven_day_opus`/`seven_day_sonnet`/`overage`), `utilization`, `resetsAt`, `isUsingOverage`, `overageStatus`, `overageResetsAt`. Limits extracted from API response headers and error messages.
- **Go**: `rate_limit.go` implements generic `RateLimitState` with 4 buckets: `RequestsMin`, `RequestsHour`, `TokensMin`, `TokensHour` -- each with `Limit`, `Remaining`, `ResetSeconds`. Parsed from `x-ratelimit-*` headers. This is a **third-party provider** rate limit tracker (Nous Portal / OpenRouter / OpenAI-compatible), not ClaudeAI subscription limits.
- **Type**: Go适配

#### C.9.2 Overage Handling

- **Upstream**: `rateLimitMessages.ts` has full overage support: `isUsingOverage`, `overageStatus` (`allowed`/`allowed_warning`/`rejected`), `overageResetsAt`, `overageDisabledReason` (`out_of_credits`). Messages like "You're now using extra usage" and "You're out of extra usage".
- **Go**: No overage concept -- rate_limit.go tracks bucket-style limits (RPM/RPH/TPM/TPH) without subscription/overage semantics.
- **Type**: 缺失

#### C.9.3 Display Formatting

- **Upstream**: Rate limit display via React components (`RateLimitMessage.tsx`, `StatusLine.tsx`, `Usage.tsx`) with rich formatting, progress bars, and upsell commands (`/upgrade`, `/extra-usage`).
- **Go**: `rate_limit.go:289-447` implements terminal-friendly formatting: `FormatRateLimitDisplay()` with ASCII progress bars (`bar()`), `bucketLine()` with usage percentage, `fmtCount()` for human-friendly numbers (8.0M, 33.6K), `fmtSeconds()` for duration, `FormatRateLimitCompact()` for one-line status bars.
- **Type**: Go适配

#### C.9.4 Warning Threshold

- **Upstream**: `rateLimitMessages.ts:69` warns when `utilization >= 0.7` (70%). Team/Enterprise with overage enabled skip warnings.
- **Go**: `rate_limit.go:404` warns when `UsagePct() >= 80` (80%). No subscription-type awareness.
- **Type**: 简化

#### C.9.5 Retry-After Header

- **Upstream**: `withRetry.ts:519-528` parses `retry-after` header from error response. Supports both integer seconds and HTTP-date formats.
- **Go**: `rate_limit.go:194-206` checks for `retry-after` header as fallback when no `x-ratelimit-*` headers exist. `RetryDelay()` at line 105-129 computes delay from bucket reset times with 10% safety margin.
- **Type**: Go适配

### C.10 Normalize

#### C.10.1 JSON Key Sorting vs Upstream

- **Upstream**: `normalizeMessagesForAPI` does NOT sort JSON keys in tool_use input. Instead, it normalizes tool input via `normalizeToolInputForAPI()` which strips specific fields (like `plan` from ExitPlanModeV2) based on the tool's schema. This is tool-aware normalization, not generic key sorting.
- **Go**: `normalize.go:91-108` sorts ALL JSON keys alphabetically via `sortMapKeys()` recursively. This is a cache-friendly normalization that makes identical logical content produce identical API payloads, but it's unaware of tool-specific field semantics.
- **Type**: 差异

#### C.10.2 Tool Input Normalization

- **Upstream**: `messages.ts:2240-2275` normalizes tool input per-tool: `normalizeToolInputForAPI(tool, input)` strips tool-specific fields like `plan` from ExitPlanModeV2, `caller` when tool search is disabled. Also canonicalizes tool names via `toolMatchesName()`.
- **Go**: `normalize.go` has no per-tool normalization. Only generic JSON key sorting and whitespace normalization.
- **Type**: 缺失

#### C.10.3 Tool Search / Tool Reference Handling

- **Upstream**: `messages.ts:2132-2213` handles `tool_reference` blocks in user messages: when tool search is NOT enabled, strips all tool_reference blocks. When enabled, strips only blocks for non-existent tools. Also handles `TOOL_REFERENCE_TURN_BOUNDARY` injection to prevent stop sequence sampling.
- **Go**: `normalize.go` has no tool_reference handling whatsoever.
- **Type**: 缺失

#### C.10.4 PDF/Image Error Stripping

- **Upstream**: `messages.ts:2033-2083` builds a targeted strip map: when a synthetic API error message is found (PDF too large, password protected, invalid; image too large; request too large), it walks backward to find the preceding meta user message and strips the corresponding document/image blocks. This prevents re-sending problematic content on every subsequent API call.
- **Go**: `normalize.go` has no error-triggered content stripping.
- **Type**: 缺失

#### C.10.5 Consecutive Message Merging

- **Upstream**: `messages.ts:2216-2229` merges consecutive user messages into a single message via `mergeUserMessages()`. Also merges consecutive assistant messages with the same `message.id` via `mergeAssistantMessages()`. Required for Bedrock compatibility.
- **Go**: `normalize.go` does not merge consecutive messages. Message ordering and role alternation is assumed to be correct from the caller.
- **Type**: 缺失

#### C.10.6 Normalize (Section 39 Comparison)

| Aspect | Go (`normalize.go`) | Upstream (`normalization.ts`, `messages.ts`) | Type |
|--------|--------------------|----------------------------------------|------|
| JSON key sorting | `recursive sortMapKeys` and `sortValueKeys` for deep normalization | Similar recursive approach in `normalizeMessagesForAPI` | 匹配 |
| Whitespace cleanup | `normalizeWhitespace`: 3+ blank lines -> 1, trim trailing | Focuses on API message structure normalization | Go适配 |
| Tool result whitespace | Collapses whitespace in tool_result content | Same in `normalizeMessagesForAPI` | 匹配 |

### C.11 Prompt Caching (Section 39 Comparison)

| Aspect | Go (`prompt_caching.go`) | Upstream (`promptCaching.ts`) | Type |
|--------|------------------------|----------------------------|------|
| Breakpoint strategy | 4 breakpoints: system + last 3 non-system messages | `cache_control` on individual `TextBlockParam` objects | Go适配 |
| System prompt caching | `FormatBoundaryCachedSystemPrompt`: static/dynamic split, global scope for static, ephemeral for dynamic | `cache_control` blocks on system prompt segments | Go适配 |
| SDK adaptation | Converts `MessageParam` to maps, applies caching, converts back | Sets `cache_control` directly on typed objects | Go适配 |

### C.12 Streaming API Layer -- Supplementary Deep Dive (Section 41)

#### C.12.1 Streaming Event Model & Stall Detection

| Aspect | Go (`streaming.go`) | Upstream (`claude.ts`) | Type |
|--------|--------------------|-----------------------|------|
| Event model | Flat `StreamChunk` with `ChunkType` enum; no nested content_block_start/delta/stop (line 35-41) | Full Anthropic SSE event model with message_start/content_block_start/content_block_delta/content_block_stop/message_delta/message_stop switch/case (line 2035-2354) | 简化 |
| Stall detection analytics | Simple goroutine timer, 300s/600s, single stall count (line 788-823) | Detailed: logs `tengu_streaming_stall` events with analytics, stall_count accumulation across stream lifetime (line 2000-2022) | 简化 |
| Stream idle watchdog | Single timer goroutine | Dual watchdog: `streamIdleAborted` + `streamWatchdogFiredAt`, non-streaming fallback after abort, `tengu_stream_loop_exited_after_watchdog` analytics (line 2363-2392) | 缺失 |
| Non-streaming fallback | No non-streaming fallback mechanism | `executeNonStreamingRequest` fallback with `CLAUDE_CODE_DISABLE_NONSTREAMING_FALLBACK` flag, 529-error budget carry-over, `tengu_streaming_fallback_to_non_streaming` analytics (line 2536-2661) | 缺失 |
| Thinking block streaming | Basic `ThinkingDelta` text only (line 931-954) | Thinking block with `signature` field, `signature_delta` handling, connector_text support (line 2086-2094, 2183-2203) | 简化 |
| Max tokens/context exceeded handling | Not present | Yields `createAssistantAPIErrorMessage` for `max_tokens` and `model_context_window_exceeded` (line 2323-2349) | 缺失 |
| Stream resource release | Simple `stream.Close()` and `cancel()` in handler wrapper | `releaseStreamResources()` in finally block, fallback cost tracking, `tengu_api_success` logging with costUSD (line 2876-2897) | 简化 |

#### C.12.2 Cache Breakpoints & System Prompt Blocks

| Aspect | Go (`prompt_caching.go`) | Upstream (`claude.ts`) | Type |
|--------|-------------------------|-----------------------|------|
| Cache breakpoint insertion | 4-breakpoint system: system + last 3 non-system messages (line 17-52) | Full `addCacheBreakpoints` with `skipCacheWrite` for fork agents, `cache_edits` dedup, pinned edits re-insertion, `cache_reference` on tool_result blocks, conditional `insertBlockAfterToolResults` (line 3149-3298) | 简化 |
| System prompt blocks | Single `FormatCachedSystemPrompt` returning one block (line 158-170) | `buildSystemPromptBlocks` via `splitSysPromptPrefix` with per-block `cache_scope` (global vs org) and `querySource`-aware cache control (line 3300-3324) | 简化 |
| Advisor/server_tool_use | Not present | `server_tool_use` content block, `advisor` tool tracking, `tengu_advisor_tool_call`/`tengu_advisor_tool_interrupted` events (line 2059-2074, 2100-2106) | 缺失 |
| Langfuse integration | Not present | `recordLLMObservation` for Langfuse tracing (line 2926-2941) | 缺失 |
| Prompt cache break detection | Not present | Calls `checkResponseForCacheBreak` after stream completion when `PROMPT_CACHE_BREAK_DETECTION` feature enabled (line 2440-2449) | 缺失 |
| Quota status extraction | Not present | `extractQuotaStatusFromHeaders`, bedrock adapter parsing (line 2455-2469) | 缺失 |
| Cumulative usage tracking | Not present -- `Usage` is simple `InputTokens/OutputTokens` | Handles `cache_creation_input_tokens`, `cache_read_input_tokens`, `server_tool_use`, `cache_creation` (ephemeral_1h/5m), `cache_deleted_input_tokens`, `inference_geo`, `iterations`, `speed` (line 3010-3123) | 缺失 |
| `normalizeContentFromAPI` | Not present -- constructs messages from `CollectHandler` | Normalizes content blocks with tool name lookups, agent ID context (line 2252-2256) | 简化 |

#### C.12.3 Tool Result Storage (Missing Module)

| Aspect | Go | Upstream (`toolResultStorage.ts`) | Type |
|--------|----|-----------------------------------|------|
| Entire tool result storage module | Not present -- no equivalent Go file | Complete disk persistence system (900+ lines): `persistToolResult`, `getToolResultsDir`, `buildLargeToolResultMessage`, `enforceToolResultBudget`, `ContentReplacementState`, `reconstructContentReplacementState` | 缺失 |
| `<persisted-output>` XML tag | Not present | Wraps large results with XML tags for model to reference file path (line 30-31, 192-198) | 缺失 |
| Tool results directory | Not present | `getToolResultsDir()` = `projectDir/sessionId/tool-results/` (line 97-128) | 缺失 |
| Persistence threshold | Not present | `getPersistenceThreshold` with GrowthBook `tengu_satin_quoll` flag override (line 43-78) | 缺失 |
| Preview generation | Not present | `generatePreview` cuts at last newline within limit (line 339-356) | 缺失 |
| ContentReplacementState | Not present | `seenIds` Set + `replacements` Map for prompt cache stability across turns (line 390-412) | 缺失 |
| Message-level budget enforcement | Not present | `enforceToolResultBudget` with candidate partitioning (mustReapply/frozen/fresh) (line 769-909) | 缺失 |
| Resume reconstruction | Not present | `reconstructContentReplacementState`, `reconstructForSubagentResume` (line 960-1012) | 缺失 |
| Empty tool result handling | Not present | `isToolResultContentEmpty` guard, injects "({toolName} completed with no output)" (line 287-295) | 缺失 |
| Persistence analytics | Not present | `tengu_tool_result_persisted`, `tengu_tool_result_persisted_message_budget`, `tengu_message_level_tool_result_budget_enforced` events (line 323-330, 875-902) | 缺失 |

#### C.12.4 Compact System -- Supplementary API Microcompact

| Aspect | Go (`compact.go`) | Upstream (`apiMicrocompact.ts`, `microCompact.ts`) | Type |
|--------|-------------------|------------------------------------------------------|------|
| API context management config | Hardcoded `context_management` with `clear_tool_uses_20250919` + `clear_thinking_20251015` (line 1606-1611) | Configurable `getAPIContextManagement` with env var toggles (`USE_API_CLEAR_TOOL_RESULTS`, `USE_API_CLEAR_TOOL_USES`), `API_MAX_INPUT_TOKENS`, `API_TARGET_INPUT_TOKENS`, `exclude_tools`, `clear_at_least` thresholds (apiMicrocompact.ts:64-150) | 简化 |
| Thinking block clearing | Always keeps 'all' thinking blocks (line 1609) | Conditional: `clearAllThinking` (>1h idle) keeps only 1 thinking turn, otherwise keeps 'all' (apiMicrocompact.ts:82-87) | 简化 |
| Compact warning suppression | Not present | `suppressCompactWarning`/`clearCompactWarningSuppression` (compactWarningState.ts:1-19) | 缺失 |
| Cache deletion notification | Not present | `notifyCacheDeletion` from `promptCacheBreakDetection.ts` (microCompact.ts:21) | 缺失 |
| Pinned/edits separation | `GetCacheEditsBlock()` merges consume+pin (line 2802-2841) | Separate `consumePendingCacheEdits` (clears pending) and `getPinnedCacheEdits` (returns pinned) (microCompact.ts:88-94, 97-100) | 简化 |

### C.13 Rate Limit (Section 39 Comparison)

| Aspect | Go (`rate_limit.go`) | Upstream (`withRetry.ts`) | Type |
|--------|--------------------|--------------------------|------|
| Bucket tracking | 4 `RateLimitBucket` types: RequestsMin, RequestsHour, TokensMin, TokensHour | Tracks via HTTP headers in retry loop -- no explicit buckets | Go适配 |
| Header format | Parses `x-ratelimit-*` (Nous/OpenRouter compatible) | Parses `retry-after`, `anthropic-ratelimit-unified-reset` | Go适配 |
| Retry delay | `MostConstrainedBucket` + `RetryDelay` with 10% safety margin | `getRetryDelay` with exponential backoff + jitter | Go适配 |
| UI display | ASCII progress bar | `RateLimitWarningNotification` in screens | Go适配 |

### C.14 Retry Utils (Section 39 Comparison)

| Aspect | Go (`retry_utils.go`) | Upstream (`withRetry.ts`) | Type |
|--------|---------------------|--------------------------|------|
| Scope | 71 lines -- just `jitteredBackoff` function | 823 lines -- full retry loop with error classification, OAuth refresh, AWS/GCP handling | 简化 |
| Backoff formula | base=5s, max=120s, jitterRatio=0.5 | base=500ms, max=32s, jitter=25% | 匹配 (concept) |
| Missing features | No persistent retry, fast mode fallback, 529 handling, OAuth refresh, AWS/GCP credential handling | All present | 缺失 |
