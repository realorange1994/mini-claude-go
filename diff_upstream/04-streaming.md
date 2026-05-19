# Streaming

> Streaming response handling, SSE, stream bus

## Sections Included
- [##] Line 57-144 -- ## 1. Streaming Implementation
- [###] Line 2140-2145 -- ### 15.1 Streaming — DeltasState Tracking vs Upstream
- [###] Line 2146-2151 -- ### 15.2 Streaming — Tool-Use-As-Text Detection
- [###] Line 2152-2157 -- ### 15.3 Streaming — Think Filter State Machine
- [###] Line 2158-2163 -- ### 15.4 Streaming — TerminalHandler Tool Arg Summary
- [###] Line 2164-2169 -- ### 15.5 Streaming — StreamBus Pub/Sub
- [###] Line 2170-2175 -- ### 15.6 Streaming — StreamProgress (TTFB + Throughput)
- [###] Line 2194-2199 -- ### 15.10 Prompt Caching — 1h TTL Eligibility Gating
- [###] Line 2200-2205 -- ### 15.11 System Prompt — Prompt Construction Architecture
- [###] Line 2206-2211 -- ### 15.12 System Prompt — Section Content Differences
- [###] Line 2212-2217 -- ### 15.13 Auto Classifier — Two-Stage Classification
- [###] Line 2218-2223 -- ### 15.14 Auto Classifier — Whitelist Granularity
- [###] Line 2224-2229 -- ### 15.15 Auto Classifier — Path Validation for Removal
- [###] Line 2230-2235 -- ### 15.16 Auto Classifier — Fail-Open vs Fail-Closed
- [###] Line 2236-2241 -- ### 15.17 Auto Classifier — Cache Design
- [###] Line 2242-2247 -- ### 15.18 Hooks — Event Types
- [###] Line 2248-2253 -- ### 15.19 Hooks — Shell Command Execution
- [###] Line 2254-2259 -- ### 15.20 Hooks — Hook Matcher System
- [###] Line 2260-2265 -- ### 15.21 Hooks — HTTP Hook Support
- [###] Line 2266-2271 -- ### 15.22 Hooks — Session-End Hook Timeout
- [###] Line 2272-2277 -- ### 15.23 Session Memory — Extraction Mechanism
- [###] Line 2278-2283 -- ### 15.24 Session Memory — Token Budget Constants
- [###] Line 2284-2289 -- ### 15.25 Session Memory — Entry Expiration
- [###] Line 2290-2295 -- ### 15.26 Session Memory — Custom Template Loading
- [###] Line 2296-2301 -- ### 15.27 Session Memory — GrowthBook Remote Config
- [###] Line 2302-2307 -- ### 15.28 Session Memory — Feature Gate
- [###] Line 2308-2313 -- ### 15.29 Retry Utilities — withRetry Architecture
- [###] Line 2314-2319 -- ### 15.30 Retry Utilities — Backoff Parameters
- [###] Line 2320-2325 -- ### 15.31 Retry Utilities — Persistent Retry (Unattended Sessions)
- [###] Line 2326-2331 -- ### 15.32 Retry Utilities — Fast Mode Cooldown
- [###] Line 2332-2337 -- ### 15.33 Retry Utilities — Model Fallback on 529
- [###] Line 2338-2343 -- ### 15.34 Retry Utilities — Context Overflow Auto-Adjustment
- [###] Line 2344-2349 -- ### 15.35 Retry Utilities — Connection Error Handling
- [###] Line 2350-2355 -- ### 15.36 Rate Limit — Architecture Difference
- [###] Line 2356-2361 -- ### 15.37 Rate Limit — Overage Handling
- [###] Line 2362-2367 -- ### 15.38 Rate Limit — Display Formatting
- [###] Line 2368-2373 -- ### 15.39 Rate Limit — Warning Threshold
- [###] Line 2374-2379 -- ### 15.40 Rate Limit — Retry-After Header
- [###] Line 2380-2385 -- ### 15.41 Context References — No Upstream Equivalent
- [###] Line 2386-2391 -- ### 15.42 Context References — Reference Pattern Regex
- [###] Line 2392-2397 -- ### 15.43 Context References — Security Protections
- [###] Line 2398-2403 -- ### 15.44 Context References — Token Budget Guardrails
- [###] Line 2404-2409 -- ### 15.45 Normalize — JSON Key Sorting vs Upstream
- [###] Line 2410-2415 -- ### 15.46 Normalize — Tool Input Normalization
- [###] Line 2416-2421 -- ### 15.47 Normalize — Tool Search / Tool Reference Handling
- [###] Line 2422-2427 -- ### 15.48 Normalize — PDF/Image Error Stripping
- [###] Line 2428-2433 -- ### 15.49 Normalize — Consecutive Message Merging
- [###] Line 2434-2439 -- ### 15.50 Streaming — CollectHandler vs Upstream Stream Accumulation
- [###] Line 2440-2445 -- ### 15.51 System Prompt — Git Context
- [###] Line 2446-2451 -- ### 15.52 System Prompt — Output Style System
- [###] Line 2452-2457 -- ### 15.53 System Prompt — Memory Injection Point
- [###] Line 2458-2463 -- ### 15.54 System Prompt — Skill System
- [###] Line 2464-2471 -- ### 15.55 System Prompt — Todo List Injection
- [##] Line 9657-9703 -- ## 39. Streaming, Normalize, Prompt Caching, Rate Limit, Retry Utils
- [##] Line 9751-9808 -- ## 41. Streaming API Layer — Supplementary Deep Dive

---

## Content

## 1. Streaming Implementation

### Files Compared
- **Go**: `E:\Git\miniClaudeCode-go-github\streaming.go` (1006 lines)
- **Upstream**: `E:\Git\claude-code-upstream\src\services\api\claude.ts` (stream loop at lines ~2000-2500, ~3500+ lines total)

### 1.1 SSE Parsing Approach

| Aspect | Go | Upstream TS |
|--------|-----|-------------|
| Parser | SDK typed union: `anthropic.MessageStreamEventUnion` via `ssestream.Stream` | Raw SSE events with `part.type` string matching |
| Event dispatch | `switch e := event.AsAny().(type)` on typed SDK unions (line 836) | `switch (part.type)` on string enum (line 2035) |
| Content block tracking | N/A -- delegates to SDK typed content block | Manual `contentBlocks[part.index]` array for accumulating blocks across deltas (line 2054) |

**Finding**: Go relies on the SDK's type system to provide fully typed events, while upstream manually tracks content blocks by index and accumulates deltas. Both approaches handle the same core events but with different abstraction levels.

### 1.2 Event Types Handled

| Event Type | Go (line) | Upstream TS (line) | Notes |
|------------|-----------|-------------------|-------|
| `message_start` | 837 (no-op) | 2036 (captures usage, research, ttft) | Upstream captures `research` field for internal use; Go ignores |
| `content_block_start` | 839-853 (tool_use only) | 2051-2107 (tool_use, server_tool_use, text, thinking) | **Go misses**: server_tool_use, text, thinking block starts |
| `content_block_delta` | 855-867 | 2109-2225 | **Go handles**: text, input_json, thinking. **Upstream additionally handles**: citations_delta, signature_delta, connector_text_delta |
| `content_block_stop` | 869-873 | 2227-2268 | Both handle equivalently |
| `message_delta` | 875-890 | 2270-2350 | Upstream additionally handles refusal messages, max_tokens errors, context_window_exceeded |
| `message_stop` | Not explicitly handled | 2352 (no-op) | Minor difference |

### 1.3 Thinking/Reasoning Block Handling

| Aspect | Go | Upstream TS |
|--------|-----|-------------|
| Thinking delta | Yes -- `anthropic.ThinkingDelta` at streaming.go:945-951 | Yes -- `case 'thinking_delta'` at line 2204 |
| Thinking block start | **No** -- not handled in content_block_start switch | Yes -- initializes `thinking: ''` and `signature: ''` at line 2086-2093 |
| Thinking signature (`signature_delta`) | **No** | Yes -- captured at line 2183-2202 |
| Redacted thinking | **No** -- completely missing | Handled -- `contentBlock.type !== 'redacted_thinking'` exclusion at line 653 |

**Gap**: Go has no support for redacted thinking blocks or thinking signature deltas. This means redacted thinking content from the API would be silently dropped.

### 1.4 Cache Creation/Hit Events

| Aspect | Go | Upstream TS |
|--------|-----|-------------|
| Token usage tracking | Basic: input_tokens, output_tokens at streaming.go:880-889 | Full: input_tokens, output_tokens, cache_creation_input_tokens, cache_read_input_tokens at lines 3015-3110 |
| Cache creation tokens | **No** | Tracked in `updateUsage` function (line 3000+) |
| Cache read tokens | **No** | Tracked in `updateUsage` function |
| cache_creation.ephemeral_1h/5m | **No** | Tracked at lines 3042-3049 |
| Cache break detection | **No** | `checkResponseForCacheBreak` at line 2441 |

**Gap**: Go's `Usage` struct (streaming.go:44-47) only tracks input/output tokens, missing all cache-related token counters that upstream tracks extensively.

### 1.5 Streaming Error Recovery

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

**Gap**: Go lacks model fallback support, refusal handling, and the sophisticated retry framework with fallback model switching that upstream provides.

### 1.6 Rate Limit Header Parsing

| Aspect | Go | Upstream TS |
|--------|-----|-------------|
| Response header parsing | **No** | `extractQuotaStatusFromHeaders` at line 2457 |
| Rate limit bucket tracking | **No** | `currentLimits`, `extractQuotaStatusFromError` (line 100-101) |
| Bedrock adapter headers | **No** | `bedrockAdapter.parseHeaders` at line 2464 |

**Gap**: Go has no rate limit header parsing or quota status tracking.

### 1.7 Additional Upstream Events Not in Go

| Event/Delta | Upstream Location | Description |
|-------------|-------------------|-------------|
| `server_tool_use` | claude.ts:2059-2073 | Server-side tool calls (advisor, etc.) |
| `advisor_tool_result` | claude.ts:2100-2104 | Advisor tool result tracking |
| `connector_text_delta` | claude.ts:2122-2137 | Connector text streaming |
| `citations_delta` | claude.ts:2140-2141 | Citation streaming |
| `signature_delta` | claude.ts:2183-2202 | Thinking block signature |
| `redacted_thinking` | claude.ts:653 | Redacted thinking blocks |

---


---

### 15.1 Streaming — DeltasState Tracking vs Upstream

- **上游**: `withRetry.ts` tracks `deltasState` conceptually inside `query.ts` — a `previousDeltas` field on the API call context tracks whether text/tool deltas have been emitted. The retry loop checks this to decide if retry is safe. `query.ts:~1800` has `hasEmittedDeltas` tracking.
- **Go版**: `streaming.go:693-699` defines `DeltasState` enum (`none`, `text_only`, `tool_in_flight`) and `trackDeltaState()` at line 991-1005. This is an **explicit Go enhancement** — upstream relies on scattered booleans in query.ts rather than a formal state machine.
- **类型**: Go增强


---

### 15.2 Streaming — Tool-Use-As-Text Detection

- **上游**: `claude.ts` has no explicit detection of the model echoing tool_use JSON as text. Relies on the model not doing this, or the SDK parsing handling it.
- **Go版**: `streaming.go:131-146` implements heuristic detection: when a text chunk contains ≥2 of 3 structural markers (`"type":"tool_use"`, `"id":"`, `"name":"`), it flags `toolUseAsText=true` and discards the chunk. This prevents the model from "getting stuck" outputting raw tool JSON instead of making actual tool calls.
- **类型**: Go增强


---

### 15.3 Streaming — Think Filter State Machine

- **上游**: `claude.ts` does not have a `<thinking>` tag filter for terminal display. Thinking blocks are handled at the API level via `thinking_delta` events and `redacted_thinking` blocks.
- **Go版**: `streaming.go:300-397` implements `ThinkFilterState` — a 4-state machine (`ThinkNormal`, `ThinkInTag`, `ThinkInBlock`, `ThinkClosing`) that wraps `<thinking>...</thinking>` and `<think>...</think>` blocks in ANSI dim escape codes. This is a **terminal display feature** not present in upstream (which uses React components for rendering).
- **类型**: Go增强


---

### 15.4 Streaming — TerminalHandler Tool Arg Summary

- **上游**: Tool call display is handled by React components (`AssistantToolUseMessage.tsx`, `BashPermissionRequest.tsx`) with rich formatting, progress indicators, and expandable detail.
- **Go版**: `streaming.go:470-562` implements `TerminalHandler.flushToolCall()` and `toolArgSummary()` — a compact single-line summary for each tool call (e.g., file path for file_read, command for exec, pattern for grep). No upstream equivalent since upstream uses a GUI.
- **类型**: Go增强


---

### 15.5 Streaming — StreamBus Pub/Sub

- **上游**: No equivalent. Upstream uses React state/context for distributing stream events to UI components.
- **Go版**: `streaming.go:569-621` implements `StreamBus` — a goroutine-safe pub/sub with buffered channels (capacity 100), non-blocking publish, and auto-drop on full subscribers. This is needed because Go lacks React's reactivity model.
- **类型**: Go适配


---

### 15.6 Streaming — StreamProgress (TTFB + Throughput)

- **上游**: TTFB tracked via `ttftMs = Date.now() - start` at `claude.ts:2038`. Throughput not explicitly tracked.
- **Go版**: `streaming.go:627-685` implements `StreamProgress` with `RecordFirstByte()`, `RecordTokens(n)`, `TTFB()`, and `Throughput()` — thread-safe with mutex. Exposed via `StreamSnapshot`.
- **类型**: Go增强


---

### 15.10 Prompt Caching — 1h TTL Eligibility Gating

- **上游**: `claude.ts:385-420` gates 1h TTL on: (1) Bedrock users with `ENABLE_PROMPT_CACHING_1H_BEDROCK` env var, (2) ant users, (3) ClaudeAI subscribers NOT in overage. Eligibility is latched session-stable to prevent mid-session TTL flips. GrowthBook allowlist controls which `querySource` patterns get 1h (e.g., `["repl_main_thread*", "sdk", "agent:*"]`).
- **Go版**: `prompt_caching.go:24-26` uses simple string check `if ttl == "1h"` with no eligibility gating, no GrowthBook, no subscriber checks, no session latching.
- **类型**: 简化


---

### 15.11 System Prompt — Prompt Construction Architecture

- **上游**: `systemPrompt.ts:41-123` implements `buildEffectiveSystemPrompt()` with 5 priority levels: (0) override system prompt → replaces all; (1) coordinator system prompt; (2) agent system prompt (proactive mode appends, otherwise replaces); (3) custom system prompt; (4) default system prompt. `appendSystemPrompt` always appended (except override). The system prompt is assembled from `systemPromptSection()` and `DANGEROUS_uncachedSystemPromptSection()` helpers that separate cached vs uncached parts.
- **Go版**: `system_prompt.go:225-355` implements `BuildSystemPrompt()` with a simpler model: static part (environment, tools, rules) + dynamic part (mode, project instructions, skills). No coordinator mode, no override, no proactive mode, no agent priority system. The cached version `GetOrBuild()` at line 404 uses `staticDirty`/`dynamicDirty` flags.
- **类型**: 简化


---

### 15.12 System Prompt — Section Content Differences

- **上游**: `prompts.ts` builds the system prompt from dozens of `systemPromptSection()` calls, including: environment info, git context, tool descriptions (from individual tool modules), rules, dangerous patterns, output style, MCP tool descriptions, scratchpad instructions, agent instructions, skill instructions, plan mode, poor mode, memory injection, todo injection, and more. Sections are conditionally included based on feature flags, user type, and query source.
- **Go版**: `system_prompt.go:23-170` uses a single large template string `systemPromptTemplateStatic` with `%s` placeholders for model name, env info, working dir, time, git context, and tool list. The dynamic template `systemPromptTemplateDynamic` has `%s` for mode, mode description, project instructions, skills, and context retention count. Far fewer conditional sections.
- **类型**: 简化


---

### 15.13 Auto Classifier — Two-Stage Classification

- **上游**: `yoloClassifier.ts` uses a single API call with extended thinking (budget_tokens up to 10240). The model produces a `thinking` block then a `tool_use` block with the classification. Uses `sideQuery()` for the API call which shares the parent's prompt cache. No explicit two-stage approach.
- **Go版**: `auto_classifier.go:654-846` implements explicit two-stage classification: Stage 1 (fast, 2112 max_tokens) for quick allow/block, Stage 2 (thinking, 6144 max_tokens) with richer analysis prompt when Stage 1 blocks. This is a **Go-specific design** — the two-stage approach reduces latency for the common "allow" case.
- **类型**: Go增强


---

### 15.14 Auto Classifier — Whitelist Granularity

- **上游**: `yoloClassifier.ts` does not have an explicit whitelist. Instead, it uses `bashClassifier.ts` for bash command classification with regex patterns for dangerous commands. Tool-level allow/deny is done by the permission system (`permissions.ts`), not the classifier. The classifier only runs for tools not already allowed by permission rules.
- **Go版**: `auto_classifier.go:48-124` has explicit `AUTO_MODE_SAFE_TOOLS`, `SAFE_GIT_OPERATIONS`, `SAFE_PROCESS_OPERATIONS`, `SAFE_FILEOPS_OPERATIONS`, and `SAFE_EXEC_PREFIXES` whitelists with 40+ safe tool names and 25+ safe command prefixes. Also has `DANGEROUS_EXEC_PATTERNS` with 20+ dangerous patterns. The `IsAutoAllowlisted()` function at line 533 checks these before calling the LLM classifier.
- **类型**: Go增强


---

### 15.15 Auto Classifier — Path Validation for Removal

- **上游**: `pathValidation.ts` has `isDangerousRemovalPath()` which checks: root `/`, home dir `~`, direct children of root, wildcard `*`/`/*`, Windows drive roots and protected dirs (Windows, Users, Program Files). This runs as part of the permission pipeline.
- **Go版**: `auto_classifier.go:307-384` implements `isDangerousRemovalPath()` with equivalent checks plus Windows-specific `C:\Windows`, `C:\Users`, `C:\Program Files` detection using regex. Also has `extractRemovalPaths()` and `checkDangerousRemovalPaths()` for parsing rm commands. Integrated directly into the classifier rather than a separate permission module.
- **类型**: Go适配


---

### 15.16 Auto Classifier — Fail-Open vs Fail-Closed

- **上游**: `yoloClassifier.ts` fails-closed when unavailable (returns "deny" by default). Uses `CannotRetryError` on classifier API failures.
- **Go版**: `auto_classifier.go:621-627` fails-closed when classifier is unavailable (returns `Allow: false`). However, `callStage2()` at line 823-827 **fails-open** on Stage 2 API error: `Allow: true, Reason: "classifier unavailable (stage 2 error); action allowed by default"`. This is a security difference — upstream is consistently fail-closed.
- **类型**: 差异


---

### 15.17 Auto Classifier — Cache Design

- **上游**: `yoloClassifier.ts` uses `lastClassifierRequests` in bootstrap state — a simple map keyed by tool name + input hash. No explicit TTL; cache is invalidated on compaction via `clearClassifierCache()`.
- **Go版**: `auto_classifier.go:36-43` implements `cacheEntry` with explicit `expiresAt` (5-minute TTL). `cacheKey()` at line 1139-1173 generates specialized keys per tool type (exec by command prefix, git by operation, fileops by operation+path, etc.). `ClearCache()` called post-compaction.
- **类型**: Go增强


---

### 15.18 Hooks — Event Types

- **上游**: `hooks.ts` (1000+ lines) supports 20+ hook event types: `PreToolUse`, `PostToolUse`, `PostToolUseFailure`, `PreCompact`, `PostCompact`, `SessionStart`, `SessionEnd`, `Setup`, `Stop`, `StopFailure`, `SubagentStart`, `SubagentStop`, `TeammateIdle`, `TaskCreated`, `TaskCompleted`, `ConfigChange`, `CwdChanged`, `FileChanged`, `InstructionsLoaded`, `UserPromptSubmit`, `PermissionRequest`, `Elicitation`, `StatusLine`, `FileSuggestion`, `PermissionDenied`, `Notification`. Hooks run as shell commands with JSON input on stdin and JSON output on stdout.
- **Go版**: `hooks.go` (182 lines) only supports `PreCompact` and `PostCompact` hook events. No shell command execution — hooks are Go function callbacks (`PreCompactHandler`, `PostCompactHandler`). No JSON stdin/stdout protocol. No timeout configuration per hook (uses fixed 5s default).
- **类型**: 简化


---

### 15.19 Hooks — Shell Command Execution

- **上游**: `hooks.ts:185+` executes hooks as child processes (`spawn`) with configurable shell (detected per platform: bash, PowerShell, Git Bash on Windows). Hook input is serialized to JSON and piped via stdin. Hook output is parsed from stdout with `hookJSONOutputSchema`. Supports sync (blocking) and async (background) hooks. Timeout is `TOOL_HOOK_EXECUTION_TIMEOUT_MS = 10 * 60 * 1000` (10 minutes).
- **Go版**: `hooks.go` hooks are Go functions, not shell commands. No child process execution, no JSON serialization, no shell detection, no async hooks.
- **类型**: 缺失


---

### 15.20 Hooks — Hook Matcher System

- **上游**: `hooks.ts` supports `HookMatcher` with tool name patterns, event type filtering, and `PluginHookMatcher`/`SkillHookMatcher` variants. Hooks can be configured via `settings.json` `hooks` field with `matcher` objects. `getSessionHooks()` provides session-derived hooks.
- **Go版**: `hooks.go` has no matcher system. Hooks are registered programmatically by name.
- **类型**: 缺失


---

### 15.21 Hooks — HTTP Hook Support

- **上游**: `execHttpHook.ts` supports HTTP-based hooks that POST to a URL instead of running shell commands. Used for webhooks and server-side integrations.
- **Go版**: No HTTP hook support.
- **类型**: 缺失


---

### 15.22 Hooks — Session-End Hook Timeout

- **上游**: `hooks.ts:176-183` has `SESSION_END_HOOK_TIMEOUT_MS_DEFAULT = 1500` with env var override `CLAUDE_CODE_SESSIONEND_HOOKS_TIMEOUT_MS`. Session-end hooks must be fast since they block shutdown.
- **Go版**: `hooks.go:113` uses `5 * time.Second` default for all hooks, no special session-end handling.
- **类型**: 简化


---

### 15.23 Session Memory — Extraction Mechanism

- **上游**: `sessionMemory.ts:140+` uses `runForkedAgent()` which creates a sub-agent that shares the parent's prompt cache. The sub-agent reads the current session memory file and uses `Edit` tool to update it. Extraction is triggered by `registerPostSamplingHook()` (post-sampling hook system). The forked agent runs asynchronously and shares cache via `createCacheSafeParams()`.
- **Go版**: `session_memory.go:830-901` has `sessionMemoryUpdatePrompt()` and `createMemoryFileCanUseTool()` — the prompt and tool filter for a forked extraction agent. The actual extraction call happens in `agent_loop.go`. The prompt template at line 842-879 matches upstream's `getDefaultUpdatePrompt()` from `prompts.ts:43-81` almost verbatim.
- **类型**: Go适配


---

### 15.24 Session Memory — Token Budget Constants

- **上游**: `prompts.ts:8-9` defines `MAX_SECTION_LENGTH = 2000` and `MAX_TOTAL_SESSION_MEMORY_TOKENS = 12000`. The 10-section template is exactly 2000 tokens per section and 12000 total.
- **Go版**: `session_memory.go:54-55` defines `maxTokensPerSection = 20000` and `maxTotalSessionMemoryTokens = 60000` — **5x and 5x the upstream limits**. The comment says "Increased to 20000 per section and 60000 total to preserve more context across compaction cycles. 12000 total tokens is insufficient for long coding sessions."
- **类型**: Go增强


---

### 15.25 Session Memory — Entry Expiration

- **上游**: `sessionMemoryUtils.ts` does not have explicit entry expiration. Entries persist until overwritten by the forked agent. State entries are implicitly refreshed each extraction.
- **Go版**: `session_memory.go:59-66` has explicit entry expiration: `entryExpirationState = 7 days`, `entryExpirationOther = 30 days`. `removeExpiredEntries()` is called on load. Per-category max entries (`maxStateEntries = 20`, `maxDecisionEntries = 30`, etc.) prevent unbounded growth.
- **类型**: Go增强


---

### 15.26 Session Memory — Custom Template Loading

- **上游**: `prompts.ts:86-101` implements `loadSessionMemoryTemplate()` which reads a custom template from `~/.claude/session-memory/config/template.md`. Falls back to `DEFAULT_SESSION_MEMORY_TEMPLATE` if file not found.
- **Go版**: `session_memory.go:427-429` always returns `defaultSessionMemoryTemplate` — no custom template file loading. `IsSessionMemoryTemplateOnly()` at line 435-437 checks if content equals the default template.
- **类型**: 缺失


---

### 15.27 Session Memory — GrowthBook Remote Config

- **上游**: `sessionMemory.ts:82-94` checks `getFeatureValue_CACHED_MAY_BE_STALE('tengu_session_memory', false)` for feature gate and `getDynamicConfig_CACHED_MAY_BE_STALE('tengu_sm_config', {})` for remote configuration of thresholds. These can be changed server-side without client updates.
- **Go版**: `session_memory.go:910-914` uses hardcoded constants (`minimumMessageTokensToInit = 10000`, `minimumTokensBetweenUpdate = 5000`, `toolCallsBetweenUpdates = 3`). No remote config.
- **类型**: 简化


---

### 15.28 Session Memory — Feature Gate

- **上游**: `sessionMemory.ts:82-83` has `isSessionMemoryGateEnabled()` which checks GrowthBook feature gate `tengu_session_memory`. If the gate is off, session memory is completely disabled.
- **Go版**: No feature gate — session memory is always enabled when the feature is configured.
- **类型**: 简化


---

### 15.29 Retry Utilities — withRetry Architecture

- **上游**: `withRetry.ts:170-517` implements `withRetry<T>()` as an `AsyncGenerator<SystemAPIErrorMessage, T>` — it yields status messages during retries while waiting. Handles: 529 overloaded errors, 429 rate limits, 401/403 auth errors, OAuth token refresh, Bedrock/GCP credential refresh, max_tokens context overflow adjustment, fast mode fallback, persistent retry for unattended sessions, model fallback with `FallbackTriggeredError`. 12+ error classification functions (`is529Error`, `isOAuthTokenRevokedError`, `isBedrockAuthError`, `isVertexAuthError`, `isFastModeNotEnabledError`, etc.).
- **Go版**: `retry_utils.go` (71 lines) only provides `jitteredBackoff()` — a standalone utility that computes `min(base * 2^(attempt-1), maxDelay) + jitter`. No error classification, no retry loop, no auth handling, no model fallback. The actual retry logic is in `agent_loop.go` which uses `jitteredBackoff()` as one component.
- **类型**: 简化


---

### 15.30 Retry Utilities — Backoff Parameters

- **上游**: `withRetry.ts:52-56` uses `BASE_DELAY_MS = 500`, `DEFAULT_MAX_RETRIES = 10`, `MAX_529_RETRIES = 3`. `getRetryDelay()` computes `baseDelay * 2^(attempt-1) + random(0, 0.25 * baseDelay)` capped at `maxDelayMs = 32000` (32 seconds). Respects `retry-after` header.
- **Go版**: `retry_utils.go:22-46` uses `baseDelay = 5s`, `maxDelay = 120s`, `jitterRatio = 0.5`. Computes `min(5s * 2^(attempt-1), 120s) + random(0, 0.5 * delay)`. No `retry-after` header handling. No `MAX_529_RETRIES` concept.
- **类型**: 简化


---

### 15.31 Retry Utilities — Persistent Retry (Unattended Sessions)

- **上游**: `withRetry.ts:96-104` implements persistent retry for unattended sessions (`CLAUDE_CODE_UNATTENDED_RETRY` env var). Retries 429/529 indefinitely with `PERSISTENT_MAX_BACKOFF_MS = 5 * 60 * 1000` (5 min cap) and `PERSISTENT_RESET_CAP_MS = 6 * 60 * 60 * 1000` (6 hour cap). Uses chunked sleep with `HEARTBEAT_INTERVAL_MS = 30_000` for keep-alive yields. Resets attempt counter to prevent loop termination.
- **Go版**: No persistent retry mode.
- **类型**: 缺失


---

### 15.32 Retry Utilities — Fast Mode Cooldown

- **上游**: `withRetry.ts:267-305` implements fast mode cooldown on 429/529: if `retry-after` < 20s, retry immediately with fast mode still active. Otherwise, enter cooldown (switch to standard speed) with `DEFAULT_FAST_MODE_FALLBACK_HOLD_MS = 30 minutes` and `MIN_COOLDOWN_MS = 10 minutes`. Also handles fast mode rejection (org doesn't have fast mode) with permanent disable.
- **Go版**: No fast mode concept, no cooldown logic.
- **类型**: 缺失


---

### 15.33 Retry Utilities — Model Fallback on 529

- **上游**: `withRetry.ts:327-365` tracks `consecutive529Errors` and after `MAX_529_RETRIES = 3`, triggers `FallbackTriggeredError` which switches to `fallbackModel` (e.g., Opus → Sonnet). Logs `tengu_api_opus_fallback_triggered` analytics event.
- **Go版**: No model fallback mechanism.
- **类型**: 缺失


---

### 15.34 Retry Utilities — Context Overflow Auto-Adjustment

- **上游**: `withRetry.ts:388-427` detects `max_tokens` context overflow errors (status 400, message "input length and `max_tokens` exceed context limit"). Parses `inputTokens + maxTokens > contextLimit` from error message. Adjusts `retryContext.maxTokensOverride` to fit within available context minus 1000 safety buffer. Ensures minimum `FLOOR_OUTPUT_TOKENS = 3000`.
- **Go版**: No context overflow handling in retry logic. Handled separately in `agent_loop.go` when `stop_reason == "max_tokens"`.
- **类型**: 简化


---

### 15.35 Retry Utilities — Connection Error Handling

- **上游**: `withRetry.ts:112-118` detects stale connections (`ECONNRESET`, `EPIPE`) and disables keep-alive on retry via `disableKeepAlive()`. Reconnects with fresh client.
- **Go版**: No connection error detection or keep-alive management.
- **类型**: 缺失


---

### 15.36 Rate Limit — Architecture Difference

- **上游**: Rate limit tracking is primarily server-side (ClaudeAI subscription limits). `claudeAiLimits.ts` defines `ClaudeAILimits` with: `status` (`allowed`/`allowed_warning`/`rejected`), `rateLimitType` (`seven_day`/`five_hour`/`seven_day_opus`/`seven_day_sonnet`/`overage`), `utilization`, `resetsAt`, `isUsingOverage`, `overageStatus`, `overageResetsAt`. `rateLimitMessages.ts` generates user-facing messages based on subscription type and limit type. Limits are extracted from API response headers and error messages.
- **Go版**: `rate_limit.go` implements a generic `RateLimitState` with 4 buckets: `RequestsMin`, `RequestsHour`, `TokensMin`, `TokensHour` — each with `Limit`, `Remaining`, `ResetSeconds`. Parsed from `x-ratelimit-*` headers. This is a **third-party provider** rate limit tracker (Nous Portal / OpenRouter / OpenAI-compatible), not ClaudeAI subscription limits.
- **类型**: Go适配


---

### 15.37 Rate Limit — Overage Handling

- **上游**: `rateLimitMessages.ts` has full overage support: `isUsingOverage`, `overageStatus` (`allowed`/`allowed_warning`/`rejected`), `overageResetsAt`, `overageDisabledReason` (`out_of_credits`). Messages like "You're now using extra usage" and "You're out of extra usage".
- **Go版**: No overage concept — rate_limit.go tracks bucket-style limits (RPM/RPH/TPM/TPH) without subscription/overage semantics.
- **类型**: 缺失


---

### 15.38 Rate Limit — Display Formatting

- **上游**: Rate limit display is via React components (`RateLimitMessage.tsx`, `StatusLine.tsx`, `Usage.tsx`) with rich formatting, progress bars, and upsell commands (`/upgrade`, `/extra-usage`).
- **Go版**: `rate_limit.go:289-447` implements terminal-friendly formatting: `FormatRateLimitDisplay()` with ASCII progress bars (`bar()`), `bucketLine()` with usage percentage, `fmtCount()` for human-friendly numbers (8.0M, 33.6K), `fmtSeconds()` for duration, `FormatRateLimitCompact()` for one-line status bars.
- **类型**: Go适配


---

### 15.39 Rate Limit — Warning Threshold

- **上游**: `rateLimitMessages.ts:69` warns when `utilization >= 0.7` (70%). Team/Enterprise with overage enabled skip warnings.
- **Go版**: `rate_limit.go:404` warns when `UsagePct() >= 80` (80%). No subscription-type awareness.
- **类型**: 简化


---

### 15.40 Rate Limit — Retry-After Header

- **上游**: `withRetry.ts:519-528` parses `retry-after` header from error response. Supports both integer seconds and HTTP-date formats.
- **Go版**: `rate_limit.go:194-206` checks for `retry-after` header as fallback when no `x-ratelimit-*` headers exist. `RetryDelay()` at line 105-129 computes delay from bucket reset times with 10% safety margin.
- **类型**: Go适配


---

### 15.41 Context References — No Upstream Equivalent

- **上游**: There is NO equivalent to `context_references.go` in upstream. Upstream handles file/folder/git references through the attachment system (`attachments.ts`) and tool calls — the user must use tool calls to read files, not `@file:` syntax.
- **Go版**: `context_references.go` (841 lines) implements a complete `@context` expansion system: `@file:path`, `@folder:path`, `@diff`, `@staged`, `@git:N`, `@url:url` — parsed via `ParseContextReferences()`, expanded via `PreprocessContextReferences()`, with token budget guardrails (25% soft limit, 50% hard limit), sensitive directory protection, path traversal prevention, file caching, binary detection, HTML content extraction, and line range support.
- **类型**: Go增强


---

### 15.42 Context References — Reference Pattern Regex

- **上游**: N/A
- **Go版**: `context_references.go:63` uses `@(?:(?P<simple>diff|staged)\b|(?P<kind>file|folder|git|url):(?P<value>"[^"]+"|\S+))` regex. Go's `regexp` doesn't support lookbehind, so email exclusion (`user@domain.com`) is done manually in `ParseContextReferences()` via `isWordChar()` check on the character before `@`.
- **类型**: Go适配


---

### 15.43 Context References — Security Protections

- **上游**: N/A
- **Go版**: `context_references.go:66-69` lists sensitive directories (`.ssh`, `.aws`, `.gnupg`, `.kube`, `.docker`, `.azure`, `.config/gh`, `.config/git`). `ensurePathAllowed()` at line 531-560 blocks: (1) paths in sensitive dirs, (2) path traversal outside CWD. `@url:` validates http/https only, limits redirects to 10, limits response to 500KB, sets 30s timeout.
- **类型**: Go增强


---

### 15.44 Context References — Token Budget Guardrails

- **上游**: N/A
- **Go版**: `context_references.go:178-207` implements two-level token budget: soft limit (25% of contextLength) — warns but allows; hard limit (50% of contextLength) — blocks injection entirely. `InjectedTokens` estimated as `len(block) / 4` chars-per-token.
- **类型**: Go增强


---

### 15.45 Normalize — JSON Key Sorting vs Upstream

- **上游**: `normalizeMessagesForAPI` does NOT sort JSON keys in tool_use input. Instead, it normalizes tool input via `normalizeToolInputForAPI()` which strips specific fields (like `plan` from ExitPlanModeV2) based on the tool's schema. This is tool-aware normalization, not generic key sorting.
- **Go版**: `normalize.go:91-108` sorts ALL JSON keys alphabetically via `sortMapKeys()` recursively. This is a cache-friendly normalization that makes identical logical content produce identical API payloads, but it's unaware of tool-specific field semantics.
- **类型**: 差异


---

### 15.46 Normalize — Tool Input Normalization

- **上游**: `messages.ts:2240-2275` normalizes tool input per-tool: `normalizeToolInputForAPI(tool, input)` strips tool-specific fields like `plan` from ExitPlanModeV2, `caller` when tool search is disabled. Also canonicalizes tool names via `toolMatchesName()`.
- **Go版**: `normalize.go` has no per-tool normalization. Only generic JSON key sorting and whitespace normalization.
- **类型**: 缺失


---

### 15.47 Normalize — Tool Search / Tool Reference Handling

- **上游**: `messages.ts:2132-2213` handles `tool_reference` blocks in user messages: when tool search is NOT enabled, strips all tool_reference blocks. When enabled, strips only blocks for non-existent tools. Also handles `TOOL_REFERENCE_TURN_BOUNDARY` injection to prevent stop sequence sampling.
- **Go版**: `normalize.go` has no tool_reference handling whatsoever.
- **类型**: 缺失


---

### 15.48 Normalize — PDF/Image Error Stripping

- **上游**: `messages.ts:2033-2083` builds a targeted strip map: when a synthetic API error message is found (PDF too large, password protected, invalid; image too large; request too large), it walks backward to find the preceding meta user message and strips the corresponding document/image blocks. This prevents re-sending problematic content on every subsequent API call.
- **Go版**: `normalize.go` has no error-triggered content stripping.
- **类型**: 缺失


---

### 15.49 Normalize — Consecutive Message Merging

- **上游**: `messages.ts:2216-2229` merges consecutive user messages into a single message via `mergeUserMessages()`. Also merges consecutive assistant messages with the same `message.id` via `mergeAssistantMessages()`. This is required for Bedrock compatibility.
- **Go版**: `normalize.go` does not merge consecutive messages. Message ordering and role alternation is assumed to be correct from the caller.
- **类型**: 缺失


---

### 15.50 Streaming — CollectHandler vs Upstream Stream Accumulation

- **上游**: `claude.ts` accumulates streaming content in `partialMessage` object (a raw API response message) and `contentBlocks[]` array. Usage is tracked separately. The final result is the full API response message.
- **Go版**: `streaming.go:59-298` implements `CollectHandler` which collects text, tool calls, thinking, usage, and error state into separate fields. `StreamResultFrom()` at line 96-115 assembles the final `StreamResult` with `Completed` and `FinishReason` fields. `AsParsedResponse()` at line 271-298 provides a SDK-independent representation. This is a **cleaner abstraction** than upstream's raw `partialMessage` approach.
- **类型**: Go增强


---

### 15.51 System Prompt — Git Context

- **上游**: `prompts.ts` includes git context via `getIsGit()` (boolean) and `getCurrentWorktreeSession()` for worktree info. Git status details are not in the system prompt.
- **Go版**: `system_prompt.go:347` calls `tools.GetGitContext()` which returns detailed git context (branch, remote, status). This is injected as a `%s` placeholder in the static template.
- **类型**: Go增强


---

### 15.52 System Prompt — Output Style System

- **上游**: `prompts.ts:29` imports `getOutputStyleConfig` and supports multiple output styles (Explanatory, Learning, Custom) injected into the system prompt. Each style has its own system prompt section.
- **Go版**: `system_prompt.go` has no output style system. Only the default prompt.
- **类型**: 缺失


---

### 15.53 System Prompt — Memory Injection Point

- **上游**: Memory (session memory) is injected into the system prompt via `systemPromptSection('memory', ...)` which calls `getSessionMemoryContent()`. The memory section is part of the dynamic content after the boundary.
- **Go版**: `system_prompt.go:239-241` explicitly comments: "Session memory is NOT injected into the system prompt during normal conversation. It is only used during compaction as a user message (SM-compact), matching upstream's behavior." This is correct — upstream also injects memory as a user message during SM-compact, not in the system prompt during normal turns.
- **类型**: 匹配


---

### 15.54 System Prompt — Skill System

- **上游**: Skills are managed via `SkillTool` and `skillSearch/` module. Skill instructions are loaded dynamically and injected as tool descriptions. `getSkillToolCommands()` provides slash command definitions.
- **Go版**: `system_prompt.go:244-333` has its own skill system with `SkillTracker`, `GetAlwaysSkills()`, `GetUnsentSkills()`, `BuildSystemPrompt()`, and `BuildSkillsSummary()`. Skills are injected in three ways: (1) always-on skills in system prompt, (2) "New This Turn" section with 4000-char budget, (3) skills summary for previously-shown skills.
- **类型**: Go适配


---

### 15.55 System Prompt — Todo List Injection

- **上游**: `prompts.ts` injects todo list items into the system prompt via `systemPromptSection('todo', ...)`. The todo section is refreshed every turn.
- **Go版**: `system_prompt.go` does not inject todo list into the system prompt. The `TodoWrite` tool description mentions tracking, but there's no dynamic injection of current todo items.
- **类型**: 缺失

---


---

## 39. Streaming, Normalize, Prompt Caching, Rate Limit, Retry Utils

### Files Compared
- **Go**: `streaming.go`, `normalize.go`, `prompt_caching.go`, `rate_limit.go`, `retry_utils.go`
- **Upstream**: `utils/stream.ts`, `services/api/claude.ts`, `services/mcp/normalization.ts`, `services/api/promptCaching.ts`, `services/api/withRetry.ts`

### 39.1 Streaming Layer

| # | Aspect | Go (`streaming.go`) | Upstream (`utils/stream.ts`, `claude.ts`) | Type |
|---|--------|--------------------|-----------------------------------------|------|
| 1 | Architecture | Full custom streaming layer (1006 lines): chunk types, StreamBus pub/sub, CollectHandler, TerminalHandler, StreamAdapter | Simple async iterator queue pattern (76 lines) — delegates to SDK | Go适配 (more elaborate) |
| 2 | Stall detection | Dynamic timeouts (300s/600s), goroutine-based | Activity signals (`sendSessionActivitySignal`, `setSDKStatus`) — no explicit stall detection | Go适配 |
| 3 | Tool detection | `toolUseAsText` requires 2-of-3 structural markers | `ensureToolPairing` in `claude.ts` | 简化 |

### 39.2 Normalize

| # | Aspect | Go (`normalize.go`) | Upstream (`normalization.ts`, `messages.ts`) | Type |
|---|--------|--------------------|----------------------------------------|------|
| 1 | JSON key sorting | `recursive sortMapKeys` and `sortValueKeys` for deep normalization | Similar recursive approach in `normalizeMessagesForAPI` | 匹配 |
| 2 | Whitespace cleanup | `normalizeWhitespace`: 3+ blank lines → 1, trim trailing | Focuses on API message structure normalization | Go适配 |
| 3 | Tool result whitespace | Collapses whitespace in tool_result content | Same in `normalizeMessagesForAPI` | 匹配 |

### 39.3 Prompt Caching

| # | Aspect | Go (`prompt_caching.go`) | Upstream (`promptCaching.ts`) | Type |
|---|--------|------------------------|----------------------------|------|
| 1 | Breakpoint strategy | 4 breakpoints: system + last 3 non-system messages | `cache_control` on individual `TextBlockParam` objects | Go适配 |
| 2 | System prompt caching | `FormatBoundaryCachedSystemPrompt`: static/dynamic split, global scope for static, ephemeral for dynamic | `cache_control` blocks on system prompt segments | Go适配 |
| 3 | SDK adaptation | Converts `MessageParam` to maps, applies caching, converts back | Sets `cache_control` directly on typed objects | Go适配 |

### 39.4 Rate Limit

| # | Aspect | Go (`rate_limit.go`) | Upstream (`withRetry.ts`) | Type |
|---|--------|--------------------|--------------------------|------|
| 1 | Bucket tracking | 4 `RateLimitBucket` types: RequestsMin, RequestsHour, TokensMin, TokensHour | Tracks via HTTP headers in retry loop — no explicit buckets | Go适配 |
| 2 | Header format | Parses `x-ratelimit-*` (Nous/OpenRouter compatible) | Parses `retry-after`, `anthropic-ratelimit-unified-reset` | Go适配 |
| 3 | Retry delay | `MostConstrainedBucket` + `RetryDelay` with 10% safety margin | `getRetryDelay` with exponential backoff + jitter | Go适配 |
| 4 | UI display | ASCII progress bar | `RateLimitWarningNotification` in screens | Go适配 |

### 39.5 Retry Utils

| # | Aspect | Go (`retry_utils.go`) | Upstream (`withRetry.ts`) | Type |
|---|--------|---------------------|--------------------------|------|
| 1 | Scope | 71 lines — just `jitteredBackoff` function | 823 lines — full retry loop with error classification, OAuth refresh, AWS/GCP handling | 简化 |
| 2 | Backoff formula | base=5s, max=120s, jitterRatio=0.5 | base=500ms, max=32s, jitter=25% | 匹配 (concept) |
| 3 | Missing features | No persistent retry, fast mode fallback, 529 handling, OAuth refresh, AWS/GCP credential handling | All present | 缺失 |


---

## 41. Streaming API Layer — Supplementary Deep Dive

### Files Compared
- **Go**: `streaming.go`, `prompt_caching.go`
- **Upstream**: `claude.ts` (SSE streaming, cache breakpoints, usage tracking), `toolResultStorage.ts`

### 41.1 Streaming Event Model & Stall Detection

| # | Aspect | Go (`streaming.go`) | Upstream (`claude.ts`) | Type |
|---|--------|--------------------|-----------------------|------|
| 1 | Event model | Flat `StreamChunk` with `ChunkType` enum; no nested content_block_start/delta/stop (line 35-41) | Full Anthropic SSE event model with message_start/content_block_start/content_block_delta/content_block_stop/message_delta/message_stop switch/case (line 2035-2354) | 简化 |
| 2 | Stall detection analytics | Simple goroutine timer, 300s/600s, single stall count (line 788-823) | Detailed: logs `tengu_streaming_stall` events with analytics, stall_count accumulation across stream lifetime (line 2000-2022) | 简化 |
| 3 | Stream idle watchdog | Single timer goroutine | Dual watchdog: `streamIdleAborted` + `streamWatchdogFiredAt`, non-streaming fallback after abort, `tengu_stream_loop_exited_after_watchdog` analytics (line 2363-2392) | 缺失 |
| 4 | Non-streaming fallback | No non-streaming fallback mechanism | `executeNonStreamingRequest` fallback with `CLAUDE_CODE_DISABLE_NONSTREAMING_FALLBACK` flag, 529-error budget carry-over, `tengu_streaming_fallback_to_non_streaming` analytics (line 2536-2661) | 缺失 |
| 5 | Thinking block streaming | Basic `ThinkingDelta` text only (line 931-954) | Thinking block with `signature` field, `signature_delta` handling, connector_text support (line 2086-2094, 2183-2203) | 简化 |
| 6 | Max tokens/context exceeded handling | Not present | Yields `createAssistantAPIErrorMessage` for `max_tokens` and `model_context_window_exceeded` (line 2323-2349) | 缺失 |
| 7 | Stream resource release | Simple `stream.Close()` and `cancel()` in handler wrapper | `releaseStreamResources()` in finally block, fallback cost tracking, `tengu_api_success` logging with costUSD (line 2876-2897) | 简化 |

### 41.2 Cache Breakpoints & System Prompt Blocks

| # | Aspect | Go (`prompt_caching.go`) | Upstream (`claude.ts`) | Type |
|---|--------|-------------------------|-----------------------|------|
| 1 | Cache breakpoint insertion | 4-breakpoint system: system + last 3 non-system messages (line 17-52) | Full `addCacheBreakpoints` with `skipCacheWrite` for fork agents, `cache_edits` dedup, pinned edits re-insertion, `cache_reference` on tool_result blocks, conditional `insertBlockAfterToolResults` (line 3149-3298) | 简化 |
| 2 | System prompt blocks | Single `FormatCachedSystemPrompt` returning one block (line 158-170) | `buildSystemPromptBlocks` via `splitSysPromptPrefix` with per-block `cacheScope` (global vs org) and `querySource`-aware cache control (line 3300-3324) | 简化 |
| 3 | Advisor/server_tool_use | Not present | `server_tool_use` content block, `advisor` tool tracking, `tengu_advisor_tool_call`/`tengu_advisor_tool_interrupted` events (line 2059-2074, 2100-2106) | 缺失 |
| 4 | Langfuse integration | Not present | `recordLLMObservation` for Langfuse tracing (line 2926-2941) | 缺失 |
| 5 | Prompt cache break detection | Not present | Calls `checkResponseForCacheBreak` after stream completion when `PROMPT_CACHE_BREAK_DETECTION` feature enabled (line 2440-2449) | 缺失 |
| 6 | Quota status extraction | Not present | `extractQuotaStatusFromHeaders`, bedrock adapter parsing (line 2455-2469) | 缺失 |
| 7 | Cumulative usage tracking | Not present — `Usage` is simple `InputTokens/OutputTokens` | Handles `cache_creation_input_tokens`, `cache_read_input_tokens`, `server_tool_use`, `cache_creation` (ephemeral_1h/5m), `cache_deleted_input_tokens`, `inference_geo`, `iterations`, `speed` (line 3010-3123) | 缺失 |
| 8 | `normalizeContentFromAPI` | Not present — constructs messages from `CollectHandler` | Normalizes content blocks with tool name lookups, agent ID context (line 2252-2256) | 简化 |

### 41.3 Tool Result Storage (Missing Module)

| # | Aspect | Go | Upstream (`toolResultStorage.ts`) | Type |
|---|--------|----|-----------------------------------|------|
| 1 | Entire tool result storage module | Not present — no equivalent Go file | Complete disk persistence system (900+ lines): `persistToolResult`, `getToolResultsDir`, `buildLargeToolResultMessage`, `enforceToolResultBudget`, `ContentReplacementState`, `reconstructContentReplacementState` | 缺失 |
| 2 | `<persisted-output>` XML tag | Not present | Wraps large results with XML tags for model to reference file path (line 30-31, 192-198) | 缺失 |
| 3 | Tool results directory | Not present | `getToolResultsDir()` = `projectDir/sessionId/tool-results/` (line 97-128) | 缺失 |
| 4 | Persistence threshold | Not present | `getPersistenceThreshold` with GrowthBook `tengu_satin_quoll` flag override (line 43-78) | 缺失 |
| 5 | Preview generation | Not present | `generatePreview` cuts at last newline within limit (line 339-356) | 缺失 |
| 6 | ContentReplacementState | Not present | `seenIds` Set + `replacements` Map for prompt cache stability across turns (line 390-412) | 缺失 |
| 7 | Message-level budget enforcement | Not present | `enforceToolResultBudget` with candidate partitioning (mustReapply/frozen/fresh) (line 769-909) | 缺失 |
| 8 | Resume reconstruction | Not present | `reconstructContentReplacementState`, `reconstructForSubagentResume` (line 960-1012) | 缺失 |
| 9 | Empty tool result handling | Not present | `isToolResultContentEmpty` guard, injects "({toolName} completed with no output)" (line 287-295) | 缺失 |
| 10 | Persistence analytics | Not present | `tengu_tool_result_persisted`, `tengu_tool_result_persisted_message_budget`, `tengu_message_level_tool_result_budget_enforced` events (line 323-330, 875-902) | 缺失 |

### 41.4 Compact System — Supplementary API Microcompact

| # | Aspect | Go (`compact.go`) | Upstream (`apiMicrocompact.ts`, `microCompact.ts`) | Type |
|---|--------|-------------------|------------------------------------------------------|------|
| 1 | API context management config | Hardcoded `context_management` with `clear_tool_uses_20250919` + `clear_thinking_20251015` (line 1606-1611) | Configurable `getAPIContextManagement` with env var toggles (`USE_API_CLEAR_TOOL_RESULTS`, `USE_API_CLEAR_TOOL_USES`), `API_MAX_INPUT_TOKENS`, `API_TARGET_INPUT_TOKENS`, `exclude_tools`, `clear_at_least` thresholds (apiMicrocompact.ts:64-150) | 简化 |
| 2 | Thinking block clearing | Always keeps 'all' thinking blocks (line 1609) | Conditional: `clearAllThinking` (>1h idle) keeps only 1 thinking turn, otherwise keeps 'all' (apiMicrocompact.ts:82-87) | 简化 |
| 3 | Compact warning suppression | Not present | `suppressCompactWarning`/`clearCompactWarningSuppression` (compactWarningState.ts:1-19) | 缺失 |
| 4 | Cache deletion notification | Not present | `notifyCacheDeletion` from `promptCacheBreakDetection.ts` (microCompact.ts:21) | 缺失 |
| 5 | Pinned/edits separation | `GetCacheEditsBlock()` merges consume+pin (line 2802-2841) | Separate `consumePendingCacheEdits` (clears pending) and `getPinnedCacheEdits` (returns pinned) (microCompact.ts:88-94, 97-100) | 简化 |

---


---

