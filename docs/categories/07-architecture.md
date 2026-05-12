# 07 — Architecture: Cross-Cutting Patterns, Concurrency & State

> Error classification, retry/rate-limiting, message normalization, compaction, transcript system, state management, cross-cutting concerns

## Overview

Cross-cutting architectural patterns that span multiple subsystems. These are foundational to the system's reliability, performance, and correctness.

---

## 1. Error Classification

[diff_upstream/06-error-handling.md]

### 1.1 Taxonomy Approach

| Aspect | Go (`error_types.go`) | Upstream (`errors.ts`) | Gap |
|--------|---------------------|----------------------|-----|
| Classification approach | 15-category `ErrorClass` enum with priority-ordered pipeline | 25+ string labels via `classifyAPIError()` + `shouldRetry()` boolean checks | 简化 |
| Classifier result | `ClassifyResult{Retryable, Compress, RotateKey, Fallback, RetryAfter}` | String label only; action decisions in `shouldRetry()` and `getAssistantMessageFromError()` | 差异 |
| Dead code | `ECThinkingSig` and `ECLongContextTier` defined but never returned | N/A | **Bug** |
| PDF-specific errors | None | `pdf_too_large`, `pdf_password_protected` | 缺失 |
| SSL cert error | None | `ssl_cert_error` | 缺失 |
| Capacity switches | Generic `ECRetryable` | `capacity_off_switch` named category | 简化 |
| Tool error granularity | Single `ECToolPairing` | `tool_use_mismatch`, `unexpected_tool_result`, `duplicate_tool_use_id` | 简化 |
| Chinese patterns | Yes — `超过最大长度`, `上下文长度` | English only | **Go增强** |

**Action**: Remove dead code (`ECThinkingSig`, `ECLongContextTier`). Add PDF-specific errors. Split `ECToolPairing` into granular categories.

### 1.2 Classification Pipeline

- **Go**: Single `classifyError()` function with priority-ordered pipeline: (1) status code → (2) error code from body → (3) message pattern matching → (4) server disconnect heuristic → (5) transport error heuristics → (6) fallback unknown. Sub-classifiers `classify400()`, `classify402()` handle ambiguous status codes.
- **Upstream**: No centralized classifier; error handling is distributed: API errors via `@ant/model-provider`, filesystem errors via `isFsInaccessible()`, axios errors via `classifyAxiosError()`, abort errors via `isAbortError()`.

**Go strength**: Centralized pipeline with deterministic priority ordering; upstream lacks equivalent.

### 1.3 Error Pattern Libraries

| Pattern Group | Go Count | Upstream | Notes |
|---------------|----------|----------|-------|
| `billingPatterns` | 10 | None | Go增强 |
| `rateLimitPatterns` | 14 | None | Go增强 |
| `usageLimitPatterns` | 4 + 6 transient signals | None | Disambiguates permanent billing vs transient usage limits |
| `contextOverflowPatterns` | 24 (incl. Chinese) | ~1 ("prompt is too long") | Go增强 |
| `modelNotFoundPatterns` | 8 | None | Go增强 |
| `authPatterns` | 9 | None | Go增强 |
| `serverDisconnectPatterns` | 7 | None | Go增强 |
| `networkErrorPatterns` | 12 | None | Go增强 |
| `transportErrorTypes` | 13 | 1 (`APIConnectionTimeoutError`) | Go增强 |

### 1.4 Missing Upstream Error Features

| Feature | Status | Impact |
|---------|--------|--------|
| `TelemetrySafeError` dual-message design | 缺失 | Error messages passed as-is without sanitization for telemetry |
| `shortErrorStack(e, maxFrames=5)` | 缺失 | No stack frame extraction for LLM context savings |
| `getAssistantMessageFromError()` | 缺失 | No user-facing message formatter from API errors |
| `parseMaxTokensContextOverflowError()` | 缺失 | Cannot adjust max_tokens and retry on overflow |

---

## 2. Retry & Rate Limiting

[diff_upstream/06-error-handling.md]

### 2.1 Backoff Algorithm

| Aspect | Go (`retry_utils.go`) | Upstream (`withRetry.ts`) | Gap |
|--------|----------------------|--------------------------|-----|
| **Algorithm** | Jittered exponential: `base * 2^(attempt-1) + uniform(0, jitterRatio * delay)` | Exponential: `min(500 * 2^(attempt-1), maxDelayMs) + random(0, 0.25 * baseDelay)` |
| Base delay | 5s | 500ms | 差异 (10x) |
| Max delay | 120s | 32s default, 5min persistent | 差异 |
| Jitter ratio | 0.5 (50% of computed delay) | 0.25 (25% of base delay, NOT capped delay) | 差异 |
| Overflow guard | Caps exponent at 63 | `Math.min(base * 2^n, maxDelayMs)` | 匹配 |
| Persistent retry | None | `CLAUDE_CODE_UNATTENDED_RETRY` — infinite with 6h cap, 30s heartbeat | 缺失 |
| Fast mode retry | Not applicable | Short vs long retry-after paths with model switching | 缺失 |

**Key difference**: Go's base delay is 10x larger (5s vs 500ms), max delay is 3.75x larger (120s vs 32s), and jitter is 2x larger (50% vs 25%). Go was designed for proxy/third-party providers with longer cooldown windows; upstream was designed for first-party API with faster recovery.

### 2.2 Transient Error Classification

| Error Category | Go | Upstream | Key Difference |
|---------------|-----|----------|----------------|
| 429 Rate Limit | Always retryable via `ECRateLimit` | Gated: `!isClaudeAISubscriber() \|\| isEnterpriseSubscriber()` | Go always retries; upstream conditionally skips for subscribers |
| 401 Auth | NOT retryable | Retryable with credential refresh | 缺失 |
| 403 Auth | NOT retryable | Conditional: CCR retries; OAuth revoked retries | 缺失 |
| 529 Overloaded | Always retryable | 3-strike → Opus-to-Sonnet fallback | 缺失 |
| 413 Payload Too Large | Retryable with compress=true | Not retryable | 差异 |
| Bedrock/Vertex auth | Not handled | Separate auth error + credential cache clear | 缺失 |
| x-should-retry header | Not parsed | Parsed and respected | 缺失 |

### 2.3 Consecutive 500/529 Detection

| Feature | Go | Upstream |
|---------|-----|----------|
| 500 detection | `consecutive500s` counter; triggers compaction after **5** consecutive 500s | No consecutive 500 detection |
| 529 detection | Treated as `ECOverloaded`, no special counter | `consecutive529Errors` counter; triggers **Opus-to-Sonnet fallback** after 3 |
| Purpose | Proxy workaround (generic 500 = likely context overflow) | API capacity management (overloaded = switch models) |
| Fallback action | `TruncateHistory()` + return `context_length_exceeded` | Throw `FallbackTriggeredError(originalModel, fallbackModel)` |

### 2.4 Rate Limit Header Parsing & State

| Aspect | Go | Upstream | Gap |
|--------|-----|----------|-----|
| Rate limit header format | `x-ratelimit-*` (proxy/OpenRouter format) | `anthropic-ratelimit-unified-*` (first-party format) | Go适配 |
| Overage tracking | None | `allowed`/`allowed_warning`/`rejected` status tracking | 缺失 |
| Model-specific limits | None | `rateLimitType`: seven_day, five_hour, seven_day_opus, seven_day_sonnet | 缺失 |
| Rate limit state persistence | Shared across API calls via pointer | Per-request only (no persistent state) | **Go增强** |
| Most constrained bucket | `MostConstrainedBucket()` returns highest-usage bucket | None | **Go增强** |
| Proactive delay | `calculateProactiveDelay()` with sliding window | No proactive delay | **Go增强** |
| ASCII progress bar | Yes | None | **Go增强** |

**Go strength**: Persistent rate limit state with proactive delay calculation. Upstream only reads rate limit headers per-error.

---

## 3. Message Normalization

[diff_upstream/05-normalize.md]

### 3.1 Scope Comparison

| Capability | Go | Upstream TS | Gap |
|-----------|-----|-------------|-----|
| JSON key sorting | Yes (recursive `sortMapKeys`) | No (SDK handles) | **Go增强** |
| Tool result whitespace | Yes (normalizeWhitespace) | Yes (via content normalization) | 匹配 |
| Role alternation enforcement | **No** | Yes — `mergeAdjacentUserMessages` | **缺失 (CRITICAL)** |
| Tool pairing validation | **No** | Extensive — `ensureToolResultPairing` (~350 lines) | **缺失 (CRITICAL)** |
| Empty message handling | **No** | Yes — `ensureNonEmptyAssistantContent`, `filterWhitespaceOnlyAssistantMessages` | **缺失 (HIGH)** |
| Image/media validation | **No** | Yes — `validateImagesForAPI` | 缺失 |
| Orphaned tool result stripping | **No** | Yes — in `ensureToolResultPairing` | 缺失 |
| Thinking block filtering | **No** | Yes — `filterOrphanedThinkingOnlyMessages`, `filterTrailingThinkingFromLastAssistant` | 缺失 |
| Content block type stripping | **No** | Yes — strips document/image blocks on error | 缺失 |
| Error tool result sanitization | **No** | Yes — `sanitizeErrorToolResultContent` | 缺失 |
| Attachment reordering | **No** | Yes — `reorderAttachmentsForAPI` | 缺失 |
| Virtual message stripping | **No** | Yes — `.filter(m => !m.isVirtual)` | 缺失 |

### 3.2 Normalization Pipeline

**Go** (single-pass, targeted):
```
NormalizeAPIMessages -> normalizeMessage -> normalizeAssistantMessage / normalizeUserMessage
  -> sortMapKeys (tool_use input JSON)
  -> normalizeWhitespace (tool_result text)
```

**Upstream** (multi-pass pipeline in `normalizeMessagesForAPI`, ~75 lines):
```
reorderAttachmentsForAPI
  -> filter virtual messages
  -> strip targets (images/docs on error)
  -> tool_reference handling
  -> merge consecutive user/assistant messages
  -> filterOrphanedThinkingOnlyMessages
  -> filterTrailingThinkingFromLastAssistant
  -> filterWhitespaceOnlyAssistantMessages
  -> ensureNonEmptyAssistantContent
  -> smooshSystemReminderSiblings
  -> sanitizeErrorToolResultContent
  -> append message ID tags
  -> validateImagesForAPI
```

**Gap**: Go's normalization is narrowly scoped to cache-friendly JSON/whitespace normalization. It lacks the extensive role alternation, tool pairing, content filtering, and validation pipeline that upstream runs before every API call.

### 3.3 Tool Pairing Validation — Critical Gap

| Feature | Go | Upstream |
|---------|-----|----------|
| Forward direction (insert synthetic tool_result for orphaned tool_use) | **No** | Yes |
| Reverse direction (strip orphaned tool_result) | **No** | Yes |
| Duplicate tool_use ID dedup | **No** | Yes (cross-message `allSeenToolUseIds` Set) |
| Server-side tool_use handling | **No** | Yes (strips orphaned `server_tool_use`/`mcp_tool_use`) |
| Strict mode (throw on mismatch) | **No** | Yes (for HFI training data) |

**Action**: Implement `ensureToolResultPairing()`. Implement role alternation enforcement. Add empty message filtering.

---

## 4. Compaction System

[diff_upstream/03-compact.md]

### 4.1 Architecture — Monolithic vs Multi-File

| Aspect | Go | Upstream |
|--------|-----|----------|
| Code organization | Single `compact.go` (2893 lines) | ~10 files: `compact.ts` (1712 lines), `prompt.ts` (375), `microCompact.ts` (531), `cachedMicrocompact.ts` (113), `sessionMemoryCompact.ts` (631), `autoCompact.ts` (352), etc. |
| LLM compact flow | Direct streaming API call, no forked-agent cache sharing | First attempts `runForkedAgent` to share prompt cache, falls back to streaming |
| Custom/hook instructions | No custom instruction injection path | `mergeHookInstructions()` merges user + hook instructions |
| PARTIAL_COMPACT_UP_TO_PROMPT | Missing (uses same prompt for both directions) | Distinct prompt with "Work Completed" / "Context for Continuing Work" sections |

### 4.2 Post-Compact Recovery

| Recovery Type | Go | Upstream | Gap |
|---------------|-----|----------|-----|
| File re-read (5 files, 50K budget) | No | `createPostCompactFileAttachments()` | 缺失 |
| Plan attachment preservation | No | `createPlanAttachmentIfNeeded()` | 缺失 |
| Skill content recovery (25K budget) | No | `createSkillAttachmentIfNeeded()` | 缺失 |
| Plan mode recovery | No | `createPlanModeAttachmentIfNeeded()` | 缺失 |
| Async agent status | No | `createAsyncAgentAttachmentsIfNeeded()` | 缺失 |
| Delta tools/MCP re-declaration | No | `getDeferredToolsDeltaAttachment()`, `getMcpInstructionsDeltaAttachment()` | 缺失 |
| Session-start hooks | No | `processSessionStartHooks('compact')` | 缺失 |
| File staleness detection (mtime) | No | Compares mtime, falls back to `compact_file_reference` | 缺失 |

**Critical architectural divergence**: Go preserves recent messages after full compact; upstream does not — only summary + attachments form the new context. Go's post-compact context is always larger.

### 4.3 Go-Unique Compaction Features

| Feature | Description | Type |
|---------|-------------|------|
| Anti-thrashing (savings-ratio) | Skip if last 2 compactions each saved <10% | Go增强 |
| Cooldown | Skip if tokens haven't grown 25% since last compaction | Go增强 |
| SmartCompact (head+tail preservation) | Keeps first N and last M turns, collapses middle with OmissionMarker | Go增强 |
| FNV-1a tool result dedup | Hash-based deduplication of identical tool results | Go增强 |
| `truncateLargeToolArgs` | Pre-pruning: truncates string values in tool_use input to 2000 chars | Go增强 |
| `redactSensitiveText` | Redacts API keys, passwords, secrets before compaction | Go增强 |
| Iterative summary | `{previous_summary}` placeholder enables updating rather than re-generating | Go增强 |
| `context_management` API | Sends `clear_tool_uses_20250919` + `clear_thinking_20251015` | Go增强 |
| Archive to disk | Writes omitted rounds to timestamped JSON files | Go增强 |
| Reactive compact | Token spike detection with threshold-based trigger | Go增强 |

### 4.4 Cache Breakpoint Placement

| Aspect | Go | Upstream | Gap |
|--------|-----|----------|-----|
| Marker count | Up to 4 | Exactly 1 | 简化 |
| Rationale | Anthropic allows up to 4 breakpoints | Mycro's turn-to-turn eviction frees local KV pages — extra markers waste pages | 简化 |
| skipCacheWrite | Not supported | Shifts marker to second-to-last for fire-and-forget forks | 缺失 |
| Scope | Not supported | `scope: 'global'\|'org'` | 缺失 |
| Cache break detection | None | Two-phase system: `recordPromptState()` pre-call + `checkResponseForCacheBreak()` post-call with 5% drop threshold | 缺失 |
| Cache edit injection | `injectCacheEdits()` but no pinning/re-insertion | Full pinned edits lifecycle with deduplication | 缺失 |

**Action**: Reduce from 4 to 1 cache marker. This wastes KV pages on intermediate positions.

---

## 5. Transcript System

[diff_upstream/20-transcript.md]

### 5.1 Data Model

| Aspect | Go | Upstream | Gap |
|--------|-----|----------|-----|
| Data model | Flat linear JSONL entries | DAG-based with UUID + parentUuid chain | 简化 |
| Entry types | 8 (user, assistant, tool_use, tool_result, compact, summary, system, error) | 19+ (includes metadata: custom-title, tag, agent-name, pr-link, mode, worktree-state, attribution-snapshot, etc.) | 简化 |
| Tool representation | Separate `tool_use`/`tool_result` entries | Embedded as content blocks within user/assistant messages | 差异 |
| Branching | Not supported | Full DAG with leaf detection, chain walking, orphan recovery | 缺失 |
| Compact boundary | Dedicated `compact` entry type | `system` + `subtype: 'compact_boundary'` with rich metadata | 简化 |
| Pre-boundary skip | Reads entire file | Truncates at boundary for files > 5MB | 缺失 |
| Interrupt detection | None | 3-way: none / interrupted_prompt / interrupted_turn | 缺失 |
| Crash safety | Per-write fsync (immediate) | Async write queue with file locking | 差异 |
| Dedup | None | UUID set to avoid writing messages already on disk | 缺失 |

### 5.2 Resume Flow — Context Reconstruction

| Aspect | Go | Upstream | Gap |
|--------|-----|----------|-----|
| Resume method | `continueTranscript` boolean: append to original or create new | Always continues same session file (session UUID reuse); `--fork-session` for new file | 简化 |
| State restoration | Conversation context + skill state only | 15+ fields: context, skills, file history, attribution, content replacements, context collapse, worktree, agent metadata, PR links | 缺失 |
| Interrupt detection | None | Detects `interrupted_turn`, auto-injects "Continue from where you left off." | 缺失 |
| Pre-boundary optimization | None (reads all entries) | Truncates at compact boundaries for files > 5MB | 缺失 |
| Legacy format migration | None | Attachment type migration, progress bridging, invalid field stripping | 缺失 |

### 5.3 Session Discovery

| Aspect | Go | Upstream | Gap |
|--------|-----|----------|-----|
| Session lookup | Directory listing with number/last/filename | Multi-project scan + cloud CCR API + interactive picker | 简化 |
| Pagination | None | Supported | 缺失 |
| Cloud session discovery | None | Anthropic CCR API (`/v1/sessions`) | 缺失 |
| Prompt history | None | Global `history.jsonl` with paste store, Ctrl+R search | 缺失 |
| Worktree support | None | Includes sessions from git worktree paths | 缺失 |

### 5.4 Compact Boundary Handling

| Aspect | Go | Upstream |
|--------|-----|----------|
| Entry type | Dedicated `compact` + `summary` pair | `system` + `subtype: 'compact_boundary'` + user message with `isCompactSummary: true` |
| Metadata | Simple: trigger + pre_compact_tokens | Rich: trigger, preTokens, userContext, messagesSummarized, preCompactDiscoveredTools, preservedSegment |
| Chain handling | No chain (flat entries) | `parentUuid: null` on boundary (chain break) + `logicalParentUuid` preserving actual parent |
| Large file optimization | None | Forward chunked read truncates at boundary for files > 5MB |

---

## 6. Cache Breakpoint Placement

See Compaction System §4.4 above for full comparison.

---

## 7. Error Propagation

| Aspect | Go | Upstream | Gap |
|--------|-----|----------|-----|
| Propagation layers | 4: streaming → retry → Run loop → tool | 4: streaming → withRetry → queryLoop → tool | 匹配 |
| Error format | String-based (`errMsg.Error()` + substring) | Typed errors (`APIError`, `APIConnectionError`, etc.) | 简化 |
| Error withholding | Immediate surface | Withhold PTL/media errors until recovery determined | 缺失 |
| Model recovery hints | Model-directed: "Do NOT output tool syntax as text" | User-facing: "Try reading the file a different way" | 差异 |
| Death spiral prevention | `maxContextRecovery` cap | `hasAttemptedReactiveCompact` guard + stop hook skip | 差异 |
| `FallbackTriggeredError` | Not implemented | Switches models on 529 exhaustion | 缺失 |

---

## 8. Concurrency & State Management

| Aspect | Go | Upstream | Gap |
|--------|-----|----------|-----|
| Goroutines | Per-stream stall watcher, per-tool execution goroutines | Async generators, Promises, setTimeout-based watchdog | Go适配 |
| Mutex usage | `sync.RWMutex` for rate limit state, agent store | Single-threaded Node.js — no mutex needed | Go适配 |
| State sharing | Pointer-based state sharing (`rateLimitState`, `ctx`) | Object references, React context, Zustand stores | 差异 |
| Process groups | Build-tagged files (unix/windows) | Runtime platform detection in single file | **Go增强** |
| Pipe management | Explicit: keeps pipes open for backgrounding | Integrated with LocalShellTask infrastructure | 差异 |
| Cancellation | `context.Context` + `atomic.Bool` for interrupted | `AbortController` with `signal.aborted` + reason tracking | Go适配 |
| Budget tracking | `IterationBudget` with `atomic.Int32` CAS loop | `createBudgetTracker()` with API beta header | Go适配 |

**Go strengths**: Build-tagged platform code, explicit pipe management, persistent state sharing.

---

## 9. Cross-Cutting Concerns

[diff_upstream/27-cross-cutting.md]

### 9.1 Critical Gaps (may cause correctness issues)

1. **Redacted thinking blocks** — Go has no handling; API responses with redacted thinking will lose content
2. **Thinking signature** — Go doesn't capture or preserve `signature_delta` for thinking blocks
3. **Single vs multiple cache markers** — Go's 4-breakpoint strategy conflicts with upstream's documented KVCC behavior
4. **Tool result pairing** — Go lacks `ensureToolResultPairing`; malformed sequences may cause API 400s
5. **Role alternation** — Go doesn't merge consecutive user messages; may cause API rejections

### 9.2 Missing Features (may cause degraded behavior)

6. Model fallback on errors
7. Rate limit quota status / throttle header parsing
8. Cache edits / cache_reference / KVCC optimization
9. Cache scope (`global`/`org`)
10. Refusal handling detection in streaming
11. Image validation before API send
12. Orphaned thinking filtering

### 9.3 Gap Type Distribution

| Type | Count | Description |
|------|-------|-------------|
| **缺失** | 17 | Feature exists in upstream but not in Go |
| **简化** | 16 | Feature exists but with reduced scope/logic |
| **Go增强** | 14 | Feature exists in Go but not in upstream |
| **Go适配** | 9 | Different implementation due to Go/terminal constraints |
| **差异** | 2 | Different approach to the same problem |
| **匹配** | 1 | Behavior matches upstream |

### 9.4 Top 10 Critical Gaps

| # | Gap | Severity | Why It Matters |
|---|-----|----------|----------------|
| 1 | **No OS-level sandbox** | CRITICAL | All commands run without Seatbelt (macOS), Bubblewrap (Linux), or kernel-level isolation |
| 2 | **No network restrictions** | CRITICAL | No domain allow/deny lists, no Unix socket blocking — data exfiltration risk |
| 3 | **No model fallback on overload** | CRITICAL | `fallbackModel` field exists but never used; on 429/529, session retries same model or dies |
| 4 | **No tool result pairing enforcement** | HIGH | Malformed sequences may cause API 400 errors that terminate session |
| 5 | **No streaming tool execution** | HIGH | Go batch-executes tools; upstream executes as they stream in |
| 6 | **No redacted thinking block handling** | HIGH | Context corruption on re-submission |
| 7 | **No role alternation enforcement** | HIGH | API requires strict user/assistant alternation — violations cause 400 |
| 8 | **No MCP reconnection/auth** | HIGH | Failed MCP connections require full process restart |
| 9 | **No multi-provider API routing** | HIGH | Locked to Anthropic first-party API only |
| 10 | **No tool result persistence / disk spillover** | MEDIUM-HIGH | Truncates tool results with no recovery on context overflow |

### 9.5 Implementation Priority

| Priority | Feature | Impact | Effort | Rationale |
|----------|---------|--------|--------|-----------|
| **P0** | Tool result pairing | HIGH | LOW | ~200 lines; prevents API 400 crashes |
| **P0** | Role alternation enforcement | HIGH | LOW | ~150 lines; prevents API 400 rejections |
| **P0** | Redacted thinking block handling | HIGH | LOW | ~100 lines; prevent context corruption |
| **P1** | Model fallback on 429/529 | HIGH | MEDIUM | ~300 lines; major availability improvement |
| **P1** | MCP reconnection with backoff | HIGH | MEDIUM | ~400 lines; current behavior unacceptable for production |
| **P1** | Streaming tool execution | HIGH | HIGH | Major refactor; high latency impact |
| **P2** | Basic sandbox (macOS Seatbelt) | HIGH | HIGH | Platform-specific but critical for security |
| **P2** | Network restrictions | HIGH | MEDIUM | ~500 lines; at HTTP transport layer |
| **P3** | Multi-provider API routing | HIGH | VERY HIGH | Fundamental architecture change |
| **P3** | Cost tracking with USD calculation | MEDIUM | LOW | ~300 lines |

### 9.6 Key Go Enhancements Not in Upstream

1. **Two-stage auto classifier** — faster allow decisions
2. **Tool-use-as-text detection** — prevents model stuck patterns
3. **Think filter state machine** — terminal display feature
4. **@context expansion** — `@file:`, `@folder:`, `@diff:`, `@staged:`, `@git:`, `@url:` syntax
5. **StreamBus pub/sub** — event distribution for Go architecture
6. **Explicit DeltasState** — formal state machine for retry safety
7. **Session memory expiration** — prevents unbounded growth
8. **Rate limit display formatting** — terminal-friendly ASCII display
9. **5x session memory token budget** — 60K vs 12K total tokens
10. **Classifier whitelist with 40+ safe tools** — avoids unnecessary LLM calls

### 9.7 Architectural Differences

| Dimension | Go | Upstream TypeScript |
|-----------|-----|---------------------|
| **Language & Runtime** | Compiled Go binary, zero runtime dependencies | Node.js/Bun runtime, 140+ npm packages |
| **Deployment** | Single static binary (~10-20MB) | npm package or compiled exe (~100MB+ with embedded resources) |
| **State Management** | Mutex-guarded structs | React context + Ink component state + `AbortController` |
| **Streaming Model** | Blocking `Run()` — agent loop blocks until completion | Generator-based — `yield*` events in real-time |
| **Error Recovery** | Escalating truncation | Compaction-based recovery (collapse drain → reactive compact) |
| **Retry Architecture** | Inline for-loop with proactive rate-limit checks | Composable `withRetry()` generator with subscriber-gated logic |
| **Transcript Format** | Flat JSONL, 8 entry types, synchronous fsync | DAG-based JSONL, 19+ entry types, async write queue |
| **Rate Limiting** | Proactive — `RetryDelay()` checks before sleeping | Reactive — `retry-after` header overrides backoff |
| **Tool Execution** | Batch — all tools after model response completes | Streaming — tools execute as they stream in |

---

## Cross-References

- Agent loop recovery: [01-core-agent-loop.md](01-core-agent-loop.md) §5
- Tool pairing: [02-tools.md](02-tools.md) §3
- Retry/rate-limit details: [04-api-client.md](04-api-client.md) §7
- Go enhancements: [08-enhancements.md](08-enhancements.md)
- Testing patterns: [09-testing.md](09-testing.md)
