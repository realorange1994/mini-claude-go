# Error Handling & Retry

> Errors, retry logic, rate limiting

## Sections Included
- [##] Line 2498-2538 -- ## 16. Error Types & Classification (`error_types.go`)
- [##] Line 2539-2559 -- ## 17. Agent Progress Tracking (`agent_progress.go`)
- [##] Line 5361-5627 -- ## 10. Retry, Rate-Limiting & Normalization -- Deep Algorithmic Comparison
- [##] Line 5907-6183 -- ## ERROR CLASSIFICATION AND ERROR TYPE SYSTEM: Go vs Upstream TypeScript
- [##] Line 9941-10029 -- ## 43. Error Handling, Work Tasks, Agent Progress, CLI Entry Points

---

## Content

## 16. Error Types & Classification (`error_types.go`)

**Upstream reference**: `src/utils/errors.ts` (error class taxonomy) + `@ant/model-provider` (API error utils re-exported via `src/services/api/errorUtils.ts`)

### 16.1 Error taxonomy approach
- **上游**: Flat error class hierarchy — `ClaudeError`, `AbortError`, `ConfigParseError`, `ShellError`, `TeleportOperationError`, `TelemetrySafeError` etc. Each is a distinct class with typed fields. API error classification is delegated to `@ant/model-provider` (external package). Axios errors classified by `classifyAxiosError()` into `auth|timeout|network|http|other` (src/utils/errors.ts:197-238)
- **Go版**: 15-category `ErrorClass` enum (`ECRetryable`, `ECNonRetryable`, `ECContextOverflow`, `ECToolPairing`, `ECRateLimit`, `ECBilling`, `ECModelNotFound`, `ECPayloadTooLarge`, `ECOverloaded`, `ECTimeout`, `ECFormatError`, `ECAuth`, `ECThinkingSig`, `ECLongContextTier`, `ECUnknown`) with `ClassifyResult` struct carrying recovery hints (`Retryable`, `Compress`, `RotateKey`, `Fallback`, `RetryAfter`) (error_types.go:17-45)
- **类型**: Go增强 — Go version has a far richer error taxonomy with explicit recovery strategies per error class; upstream relies on a simpler class hierarchy with recovery logic scattered across callers

### 16.2 Error classification pipeline
- **上游**: No centralized classifier; error handling is distributed: API errors via `@ant/model-provider`, filesystem errors via `isFsInaccessible()`, axios errors via `classifyAxiosError()`, abort errors via `isAbortError()`
- **Go版**: Single `classifyError()` function with priority-ordered pipeline: (1) status code → (2) error code from body → (3) message pattern matching → (4) server disconnect heuristic → (5) transport error heuristics → (6) fallback unknown. Sub-classifiers `classify400()`, `classify402()` handle ambiguous status codes (error_types.go:147-373)
- **类型**: Go增强 — centralized pipeline with deterministic priority ordering; upstream lacks equivalent

### 16.3 Context overflow detection
- **上游**: Relies on API error messages; no explicit context overflow pattern matching in `errors.ts`
- **Go版**: 26+ context overflow patterns including Chinese language patterns ("超过最大长度", "上下文长度"), plus `parsePromptTooLongTokenGap()` to extract actual vs max token counts from error messages, plus heuristics: 400 + large session (60% context) → `ECContextOverflow` (error_types.go:88-100, 467-480)
- **类型**: Go增强 — pattern library + token gap parser + heuristic classification for ambiguous 400s

### 16.4 Billing vs rate limit disambiguation
- **上游**: No explicit disambiguation; `classifyAxiosError()` maps 401/403 → auth, no billing handling
- **Go版**: Explicit `billingPatterns` (9 patterns), `rateLimitPatterns` (13 patterns), `usageLimitPatterns` (4 patterns) with `usageLimitTransientSignals` to disambiguate permanent billing exhaustion from transient usage limits. `classify402()` disambiguates 402 as billing vs rate limit (error_types.go:62-86, 327-338)
- **类型**: Go增强 — upstream has no billing/rate-limit disambiguation

### 16.5 Error pattern libraries
- **上游**: Minimal — only `getErrnoCode()` for filesystem errors and `classifyAxiosError()` for HTTP
- **Go版**: 8 pattern libraries: `billingPatterns`, `rateLimitPatterns`, `usageLimitPatterns`, `contextOverflowPatterns`, `modelNotFoundPatterns`, `authPatterns`, `serverDisconnectPatterns`, `networkErrorPatterns`, `serverErrorPatterns`, `transportErrorTypes` (error_types.go:62-144)
- **类型**: Go增强 — comprehensive pattern libraries absent in upstream

### 16.6 Telemetry-safe error handling
- **上游**: `TelemetrySafeError_I_VERIFIED_THIS_IS_NOT_CODE_OR_FILEPATHS` class with separate `telemetryMessage` field (src/utils/errors.ts:93-101)
- **Go版**: No equivalent — error messages are passed as-is without sanitization for telemetry
- **类型**: 缺失 — no telemetry-safe error filtering

### 16.7 Stack trace truncation
- **上游**: `shortErrorStack(e, maxFrames=5)` extracts error message + top N stack frames for tool_result contexts (src/utils/errors.ts:161-171)
- **Go版**: No equivalent — Go uses `truncateStr()` for message length but has no stack frame extraction
- **类型**: 缺失 — Go stack traces are typically shorter but no intentional truncation for LLM context savings

---


---

## 17. Agent Progress Tracking (`agent_progress.go`)

**Upstream reference**: `src/utils/task/sdkProgress.ts` + `src/components/tasks/` (React-based task UI)

### 17.1 Progress reporting model
- **上游**: React-based component tree (`Ink` framework) renders task progress as TUI components. `sdkProgress.ts` provides SDK-compatible progress reporting. Progress is rendered via React state updates
- **Go版**: `subAgentProgressWriter` — an `io.Writer` that intercepts sub-agent output, suppresses `[THINK]` lines, extracts tool names from `[+]`/`[ERR]`/`[TIMEOUT]` lines, and writes condensed single-line progress updates using `\r\x1b[K` (carriage return + clear-to-end-line) (agent_progress.go:19-139)
- **类型**: Go适配 — Go version adapts the concept to a streaming Writer model instead of React components

### 17.2 Progress data model
- **上游**: Rich `AppState` with React component tree, multiple UI modes (inline, card, expandable)
- **Go版**: Minimal: `toolCount`, `totalTokens`, `lastToolName`, `description` — single-line status like `[agent: description] Running exec · 3 tool uses · 1500 tokens` (agent_progress.go:23-27, 112-139)
- **类型**: 简化 — no expandable UI, no tool duration tracking, no progress percentage

### 17.3 ANSI stripping
- **上游**: Uses `chalk`/`strip-ansi` npm packages
- **Go版**: Custom `stripAnsi()` that handles `\x1b` + terminator chars `m/K/J/H/L` (agent_progress.go:185-202)
- **类型**: 简化 — handles common escape codes but not full ANSI sequence spec

---


---

## 10. Retry, Rate-Limiting & Normalization -- Deep Algorithmic Comparison

**Go files analyzed:**
- `retry_utils.go` -- Jittered exponential backoff
- `rate_limit.go` -- Rate-limit state tracking, 429 handling, header parsing
- `normalize.go` -- Message normalization for API (KV cache reuse)
- `error_types.go` -- 15-category error classifier
- `agent_loop.go:1902-2018` -- Non-streaming fallback retry loop
- `prompt_caching.go` -- Cache_control breakpoint injection

**Upstream files analyzed:**
- `src/services/api/withRetry.ts` -- Retry orchestrator (AsyncGenerator-based)
- `src/services/api/errors.ts` -- Error classification + 429 handling
- `src/services/api/claude.ts:3149-3248` -- addCacheBreakpoints
- `src/services/api/claude.ts:350-365` -- getCacheControl (1h TTL gating)
- `src/utils/messages.ts:2018-2404` -- normalizeMessagesForAPI
- `src/utils/messages.ts:5197-5543` -- ensureToolResultPairing
- `src/services/rateLimitMessages.ts` -- User-facing rate limit messages
- `src/services/rateLimitMocking.ts` -- Mock rate limit facade

---

### 10.1 Retry -- Backoff Algorithm

| Aspect | Go (`retry_utils.go`) | Upstream (`withRetry.ts`) |
|--------|----------------------|--------------------------|
| **Algorithm** | Jittered exponential: `base * 2^(attempt-1) + uniform(0, jitterRatio * delay)` | Exponential: `min(500 * 2^(attempt-1), maxDelayMs) + random(0, 0.25 * baseDelay)` |
| **Base delay** | 5s (`retry_utils.go:23`) | 500ms (`withRetry.ts:55`) |
| **Max delay** | 120s (`retry_utils.go:24`) | 32s default (`withRetry.ts:533`); 5min in persistent mode (`withRetry.ts:96`) |
| **Jitter ratio** | 0.5 (50% of computed delay) (`retry_utils.go:25`) | 0.25 (25% of base delay, NOT of capped delay) (`withRetry.ts:547`) |
| **Overflow guard** | Caps exponent at 63 to prevent `1 << 63` overflow (`retry_utils.go:35-36`) | `Math.min(base * 2^n, maxDelayMs)` prevents unbounded growth (`withRetry.ts:543-544`) |
| **Retry-After override** | Not in backoff itself; handled separately in `agent_loop.go:1910-1912` (uses `rateLimitState.RetryDelay()`) | Directly in `getRetryDelay`: if `retryAfterHeader` present, returns `seconds * 1000` ignoring backoff (`withRetry.ts:535-539`) |
| **Persistent mode** | None -- always bounded by maxRetries | `CLAUDE_CODE_UNATTENDED_RETRY`: retries 429/529 indefinitely with 5-min max backoff, 6-hour reset cap, 30s heartbeat yields (`withRetry.ts:96-98`) |

**Key difference:** Go's base delay is **10x larger** (5s vs 500ms), max delay is **3.75x larger** (120s vs 32s), and jitter is **2x larger** (50% vs 25%). Go was designed for proxy/third-party providers with longer cooldown windows; upstream was designed for first-party API with faster recovery.

---

### 10.2 Retry -- Transient Error Classification

| Error Category | Go (`error_types.go`) | Upstream (`withRetry.ts:shouldRetry`) |
|---------------|----------------------|--------------------------------------|
| **Classifier approach** | 15-category `ErrorClass` enum with priority-ordered pipeline (`error_types.go:15-33`) | Inline boolean checks in `shouldRetry()` (`withRetry.ts:696-787`) |
| **429 Rate Limit** | `ECRateLimit`, retryable=true (`error_types.go:213-218`) | Conditional: `!isClaudeAISubscriber() || isEnterpriseSubscriber()` -- subscribers don't retry 429 (`withRetry.ts:767-769`) |
| **529 Overloaded** | `ECOverloaded`, retryable=true (`error_types.go:228-229`) | Checks `x-should-retry` header; respects `false` except for Ant+5xx (`withRetry.ts:746-750`) |
| **Overloaded in message body** | Pattern-matched via `serverErrorPatterns` (`error_types.go:129-132`) | Explicit `"type":"overloaded_error"` in message body (`withRetry.ts:610-620, 722-724`) |
| **401 Auth** | `ECAuth`, NOT retryable (`error_types.go:179-184`) | Retryable: clears API key cache + forces client reconnect (`withRetry.ts:773-776, 234-251`) |
| **403 Auth** | `ECAuth`, NOT retryable (`error_types.go:186-195`) | Conditional: CCR mode retries (`withRetry.ts:712-717`); OAuth revoked retries (`withRetry.ts:779-781`); otherwise not retryable |
| **408 Timeout** | Not explicitly handled; falls to `ECTimeout` via pattern (`error_types.go:315`) | Explicitly retryable (`withRetry.ts:760`) |
| **409 Conflict** | Not handled | Retryable (`withRetry.ts:763`) |
| **413 Payload Too Large** | `ECPayloadTooLarge`, retryable with compress=true (`error_types.go:209-210`) | Not retryable -- creates error message and stops |
| **500 Server Error** | `ECRetryable` (`error_types.go:224-225`) | Retryable via `status >= 500` (`withRetry.ts:784`); Ant users can override `x-should-retry: false` (`withRetry.ts:746-750`) |
| **Connection errors** | Pattern-matched via `networkErrorPatterns`/`serverDisconnectPatterns` (`error_types.go:114-127`) | `APIConnectionError` instance check (`withRetry.ts:753-755`) |
| **Context overflow** | `ECContextOverflow`, NOT retryable (compress=true) (`error_types.go:243-244`) | Not retryable; triggers reactive compact |
| **Tool pairing** | `ECToolPairing`, NOT retryable (`error_types.go:248-249`) | Not retried; creates error message |
| **Bedrock auth** | Not handled (generic 403) | Separate `isBedrockAuthError` + `clearAwsCredentialsCache` (`withRetry.ts:631-656`) |
| **Vertex auth** | Not handled | Separate `isVertexAuthError` + `clearGcpCredentialsCache` (`withRetry.ts:660-694`) |
| **x-should-retry header** | Not parsed | Parsed and respected (`withRetry.ts:732-750`) |
| **Max tokens overflow** | Not handled in classifier | `parseMaxTokensContextOverflowError`: adjusts `max_tokens` and retries (`withRetry.ts:388-427, 727-729`) |
| **Fast mode 429/529** | Not applicable | Dedicated short-retry vs cooldown path with model switching (`withRetry.ts:267-305`) |

**Key difference:** Go uses a single classifier returning `{Retryable: bool}`; upstream's `shouldRetry` has **subscriber-gated logic** where ClaudeAI subscribers don't retry 429 but enterprise subscribers do. Go always retries 429. Go doesn't parse `x-should-retry`, doesn't handle Bedrock/Vertex auth recovery, and doesn't have fast-mode fallback.

---

### 10.3 Retry -- Max Retry Counts

| Scenario | Go | Upstream |
|----------|-----|----------|
| **Default max retries** | 9 (10 total attempts) (`agent_loop.go:1904`) | 10 (`withRetry.ts:52`) |
| **Configurable** | No env var | `CLAUDE_CODE_MAX_RETRIES` env var (`withRetry.ts:790-794`) |
| **529-specific cap** | None (529 is just `ECOverloaded`, retries until maxRetries) | `MAX_529_RETRIES = 3` before Opus-to-Sonnet fallback or throw (`withRetry.ts:54, 335-365`) |
| **Persistent (unattended)** | None | Infinite with 6-hour reset cap (`withRetry.ts:97, 467-506`) |

---

### 10.4 Retry -- Delay Calculation

| Feature | Go | Upstream |
|---------|-----|----------|
| **Backoff formula** | `min(5s * 2^(attempt-1), 120s) + rand(0, 50% * delay)` | `min(500ms * 2^(attempt-1), 32s) + rand(0, 25% * baseDelay)` |
| **Retry-After** | Proactive: `rateLimitState.RetryDelay()` checked BEFORE backoff; uses min(rlim, delay*3) (`agent_loop.go:1910-1912`) | Override: if `retry-after` header present, it completely replaces backoff calculation (`withRetry.ts:535-539`) |
| **Rate limit reset delay** | `RemainingSecondsNow() * 1.1` (10% safety margin) (`rate_limit.go:128`) | `getRateLimitResetDelayMs`: parses `anthropic-ratelimit-unified-reset` unix timestamp, caps at 6hr (`withRetry.ts:814-822`) |
| **Persistent mode delay** | N/A | Windowed: min(resetDelay, 5min exponential, 6hr cap); chunked sleeps with 30s heartbeat (`withRetry.ts:433-506`) |
| **Fast mode delay** | N/A | Short (<20s) retry-after -> wait and retry at fast speed; long retry-after -> 30-min cooldown to standard speed (`withRetry.ts:284-304, 799-801`) |

**Key difference:** Go integrates rate limit state **proactively** (check before sleeping, take shorter delay); upstream integrates it **reactively** (header overrides backoff). Go's `RetryDelay()` uses time-decay from captured headers; upstream's `getRateLimitResetDelayMs` uses a server-provided unix timestamp.

---

### 10.5 Retry -- Rate Limit Integration

| Feature | Go | Upstream |
|---------|-----|----------|
| **State tracking** | `RateLimitState` struct with `sync.RWMutex`; 4 buckets (requests/min, requests/hr, tokens/min, tokens/hr) (`rate_limit.go:49-58`) | No equivalent struct -- rate limit data is extracted per-error from response headers (`errors.ts:469-534`) |
| **State sharing** | Shared across API calls via `AgentLoop.rateLimitState` pointer; `Update()` merges new data (`rate_limit.go:132-152`) | Per-request: headers parsed from each error's `APIError.headers` |
| **Proactive delay** | `RetryDelay()` scans all buckets; if any has `Remaining <= 0`, returns `RemainingSecondsNow() * 1.1` (`rate_limit.go:105-129`) | `getRateLimitResetDelayMs` only used in persistent mode; normal mode relies on `retry-after` header |
| **Most constrained bucket** | `MostConstrainedBucket()` returns bucket with highest usage % for diagnostics (`rate_limit.go:75-101`) | No equivalent |
| **Header format parsed** | `x-ratelimit-*` (OpenRouter/OCI/OpenAI-compatible format) (`rate_limit.go:156-169`) | `anthropic-ratelimit-unified-*` (first-party Anthropic format) (`errors.ts:471-534`) |

**Key difference:** Go tracks rate limit state **persistently** across requests and uses it to proactively delay retries. Upstream only reads rate limit headers from error responses and doesn't maintain persistent state. Go supports proxy provider headers; upstream supports Anthropic's unified rate limit headers.

---

### 10.6 Retry -- Consecutive 500/529 Detection

| Feature | Go | Upstream |
|---------|-----|----------|
| **500 detection** | `consecutive500s` counter; triggers compaction after **5** consecutive 500s (`agent_loop.go:1996`) | No consecutive 500 detection |
| **529 detection** | Treated as `ECOverloaded`, no special counter | `consecutive529Errors` counter; triggers **Opus-to-Sonnet fallback** after `MAX_529_RETRIES = 3` (`withRetry.ts:54, 335`) |
| **Heuristic purpose** | Proxy providers return 500 for context overflow; 5-strike triggers compact (`agent_loop.go:1988-1992`) | API returns 529 for overloaded; 3-strike triggers model fallback for non-subscribers (`withRetry.ts:331-365`) |
| **Reset condition** | Reset to 0 on any non-500 error (`agent_loop.go:2005`) | Reset to 0 on any non-529 response |
| **Fallback action** | `context.TruncateHistory()` + return `context_length_exceeded` error (`agent_loop.go:1997-1999`) | Throw `FallbackTriggeredError(originalModel, fallbackModel)` (`withRetry.ts:347-349`) |
| **Initial count seeding** | No | `initialConsecutive529Errors` option to pre-seed from streaming fallback (`withRetry.ts:141`) |

**Key difference:** Go's 5-strike-500 heuristic is a **proxy workaround** (generic 500 = likely context overflow); upstream's 3-strike-529 is an **API capacity management** strategy (overloaded = switch models). These are fundamentally different problems being solved.

---

### 10.7 Rate Limiting -- Header Parsing & State

| Feature | Go (`rate_limit.go`) | Upstream (`errors.ts`, `claude.ts`) |
|---------|---------------------|-------------------------------------|
| **Headers parsed** | `x-ratelimit-limit-requests`, `x-ratelimit-remaining-requests`, `x-ratelimit-reset-requests`, `x-ratelimit-limit-requests-1h`, `x-ratelimit-limit-tokens`, `x-ratelimit-remaining-tokens`, `x-ratelimit-reset-tokens`, `x-ratelimit-reset-tokens-1h` (`rate_limit.go:156-169`) | `anthropic-ratelimit-unified-status`, `anthropic-ratelimit-unified-overage-status`, `anthropic-ratelimit-unified-representative-claim`, `anthropic-ratelimit-unified-reset`, `anthropic-ratelimit-unified-overage-reset`, `anthropic-ratelimit-unified-overage-disabled-reason` (`errors.ts:471-514`) |
| **Header family** | OpenRouter/OCI/OpenAI-compatible | Anthropic first-party unified |
| **State structure** | `RateLimitState{RequestsMin, RequestsHour, TokensMin, TokensHour}` with mutex (`rate_limit.go:49-58`) | Inline object construction from error headers per-request (`errors.ts:482-514`) |
| **Time tracking** | `CapturedAt` timestamp per bucket; `RemainingSecondsNow()` adjusts for elapsed time (`rate_limit.go:40-47`) | Unix timestamp from `anthropic-ratelimit-unified-reset` header (`errors.ts:489-494, withRetry.ts:815-821`) |
| **Usage display** | Rich terminal display with ASCII progress bars, warnings at 80%+ (`rate_limit.go:354-447`) | User-facing messages via `rateLimitMessages.ts` (subscription-aware, model-specific) |
| **Model-specific limits** | None | `rateLimitType` field: `seven_day`, `five_hour`, `seven_day_opus`, `seven_day_sonnet` (`rateLimitMessages.ts:201-254`) |
| **Overage tracking** | None | `overageStatus`: `allowed`, `allowed_warning`, `rejected`; `overageResetsAt` timestamp (`errors.ts:475-510`) |

**Key difference:** Go parses proxy-compatible `x-ratelimit-*` headers into a persistent, thread-safe state object. Upstream parses Anthropic-specific `anthropic-ratelimit-unified-*` headers per-error with subscription/overage awareness. Go has no concept of overage or model-specific rate limit windows.

---

### 10.8 Rate Limiting -- 429 Handling

| Feature | Go | Upstream |
|---------|-----|----------|
| **429 retry decision** | Always retryable via `ECRateLimit` (`error_types.go:213-218`) | Gated: `!isClaudeAISubscriber() || isEnterpriseSubscriber()` (`withRetry.ts:767-769`) |
| **Retry-After parsing** | Not parsed from 429 error directly; `ParseRateLimitHeaders` handles `retry-after` from HTTP response (`rate_limit.go:194-207`) | `getRetryAfter` extracts from `error.headers['retry-after']` (`withRetry.ts:519-528`) |
| **Proactive delay** | `rateLimitState.RetryDelay()` returns delay based on exhausted buckets (`rate_limit.go:105-129`) | No proactive delay; relies on server's `retry-after` header |
| **Overage flow** | Not handled | `anthropic-ratelimit-unified-overage-status` drives: `allowed_warning` -> show warning; `rejected` -> error with model-specific messaging (`errors.ts:475-534`) |
| **Extra usage required** | Not handled | Special case for "Extra usage is required for long context" 429 (`errors.ts:540-547`) |
| **Mock 429** | Not implemented | `/mock-limits` command for Ant testing (`rateLimitMocking.ts:45-132`) |

**Key difference:** Go treats all 429s equally and always retries. Upstream has a sophisticated 429 decision tree based on subscription type, overage status, and rate limit type.

---

### 10.9 Rate Limiting -- State Sharing Across API Calls

| Feature | Go | Upstream |
|---------|-----|----------|
| **State persistence** | `AgentLoop.rateLimitState` -- shared pointer, survives across calls | No persistent state -- re-parsed from each error |
| **State update** | `rateLimitState.Update(newState)` merges new bucket data; only overwrites if `Limit > 0` (`rate_limit.go:132-152`) | No update mechanism |
| **Thread safety** | `sync.RWMutex` protects all reads/writes (`rate_limit.go:51, 77, 107, 133`) | Single-threaded Node.js -- no mutex needed |
| **Cross-request influence** | `RetryDelay()` uses accumulated state to decide if current request should wait (`agent_loop.go:1910`) | No cross-request influence |
| **Display** | `FormatRateLimitDisplay()` for detailed view; `FormatRateLimitCompact()` for status bar (`rate_limit.go:354-447`) | Footer warnings via `rateLimitMessages.ts` |

---

### 10.10 Normalization -- Role Alternation Enforcement

| Feature | Go (`normalize.go`) | Upstream (`messages.ts:2018-2404`) |
|---------|---------------------|-----------------------------------|
| **Strategy** | None -- Go only normalizes tool_use/tool_result content, not message order | Merges consecutive user messages (`mergeUserMessages`) (`messages.ts:2216-2224`); merges consecutive assistant messages by same `message.id` (`messages.ts:2284-2298`); prepends synthetic user message if first message is assistant (`compact.ts:288-291`) |
| **Attachment reordering** | Not handled | `reorderAttachmentsForAPI` bubbles attachments up to first non-tool-result user message (`messages.ts:2025-2030`) |
| **Virtual message stripping** | Not handled | Strips `isVirtual` messages (REPL inner calls) (`messages.ts:2028-2030`) |
| **System-to-User conversion** | Not handled | `system` local_command messages converted to user messages (`messages.ts:2107-2121`) |

**Key difference:** Go does NO role alternation enforcement. Upstream has extensive multi-pass normalization to ensure user/assistant alternation, including merging, conversion, and synthetic message insertion.

---

### 10.11 Normalization -- Tool Pairing Validation

| Feature | Go (`normalize.go`) | Upstream (`messages.ts:5197-5543`) |
|---------|---------------------|-----------------------------------|
| **Dedicated function** | None -- Go only sorts input keys, doesn't validate pairing | `ensureToolResultPairing()` -- 350-line function (`messages.ts:5212-5543`) |
| **Forward direction** | Not checked | Inserts synthetic `tool_result` with placeholder text for orphaned `tool_use` blocks (`messages.ts:5201`) |
| **Reverse direction** | Not checked | Strips `tool_result` blocks referencing non-existent `tool_use` blocks (`messages.ts:5202`) |
| **Duplicate tool_use ID** | Not checked | Cross-message dedup via `allSeenToolUseIds` Set; strips duplicates (`messages.ts:5226, 5307-5315`) |
| **Duplicate tool_result** | Not checked | Detects and strips duplicate `tool_result` for same `tool_use_id` (`messages.ts:5361-5381`) |
| **Server-side tool_use** | Not checked | Strips orphaned `server_tool_use`/`mcp_tool_use` without matching results (`messages.ts:5317-5324`) |
| **Strict mode** | None | `getStrictToolResultPairing()`: throws instead of repairing (for HFI training data) (`messages.ts:5206-5210, 5521-5527`) |
| **Orphaned tool_result at start** | Not checked | If first message is user with tool_results but no preceding assistant, strips tool_results; inserts placeholder text if message would be empty (`messages.ts:5234-5278`) |

**Key difference:** Go has zero tool pairing validation. Upstream has comprehensive bidirectional validation with strict mode, cross-message dedup, and server-side tool use handling. This is the largest normalization gap.

---

### 10.12 Normalization -- Empty Message Handling

| Feature | Go (`normalize.go`) | Upstream (`messages.ts`) |
|---------|---------------------|--------------------------|
| **Whitespace-only assistant** | Not handled | `filterWhitespaceOnlyAssistantMessages()` -- removes assistants with only whitespace text blocks (`messages.ts:4949-4998`) |
| **Empty assistant content** | Not handled | `ensureNonEmptyAssistantContent()` -- inserts placeholder for non-final empty assistants (`messages.ts:5012-5015`) |
| **Orphaned thinking-only** | Not handled | `filterOrphanedThinkingOnlyMessages()` -- removes thinking-only assistants from failed streaming retries (`messages.ts:2345`) |
| **Trailing thinking** | Not handled | `filterTrailingThinkingFromLastAssistant()` -- removes thinking blocks from last assistant (`messages.ts:2355-2356`) |

**Key difference:** Go has no empty/whitespace message filtering. Upstream has a multi-pass pipeline: strip thinking -> filter whitespace -> ensure non-empty -> merge adjacent users.

---

### 10.13 Normalization -- Image/Media Content Handling

| Feature | Go (`normalize.go`) | Upstream (`messages.ts`) |
|---------|---------------------|--------------------------|
| **Image validation** | Not handled in normalization | `validateImagesForAPI()` -- validates all images within API size limits before sending (`messages.ts:2401`) |
| **PDF error stripping** | Not handled | `stripTargets` map: PDF/image errors -> strip corresponding document/image blocks from preceding meta user messages (`messages.ts:2033-2083`) |
| **Error tool_result sanitization** | Not handled | `sanitizeErrorToolResultContent()` -- strips images from `is_error: true` tool_results (API rejects images in error results) (`messages.ts:1913, 2377`) |
| **Image resize errors** | Not handled | `ImageSizeError`/`ImageResizeError` caught before API call (`errors.ts:448-452`) |
| **Many-image dimension errors** | Not handled | Specific 400 handler for `image dimensions exceed` + `many-image` (`errors.ts:625-639`) |

**Key difference:** Go normalizes whitespace in tool_result text but doesn't handle images/media at all. Upstream has extensive image validation, error-triggered stripping, and dimension limit handling.

---

### 10.14 Normalization -- Cache Control Injection

| Feature | Go (`prompt_caching.go`) | Upstream (`claude.ts`) |
|---------|--------------------------|----------------------|
| **Strategy** | "4 breakpoints": system prompt + last 3 non-system messages (`prompt_caching.go:11-52`) | "1 marker": exactly one `cache_control` on the last (or second-to-last for skipCacheWrite) message (`claude.ts:3175-3176`) |
| **Placement** | `applyCacheMarker`: on tool role at message level; on user/assistant at last content block (`prompt_caching.go:55-95`) | `userMessageToMessageParam`/`assistantMessageToMessageParam`: on last content block of the marker message (`claude.ts:593-609`) |
| **TTL** | "5m" default, "1h" option (`prompt_caching.go:116, 24-25`) | Dynamic: `should1hCacheTTL()` gated by subscriber status + overage + GrowthBook allowlist (`claude.ts:350-365, 396-416`) |
| **Cache scope** | None | `scope?: 'global'` for static system prompt parts (`claude.ts:364`) |
| **Cache edits** | `injectCacheEdits` -- re-inserts pinned edits after message rebuild (`agent_loop.go:1974`) | `addCacheBreakpoints` inserts `cached_mc_edits` blocks with deletion references; pins them for future requests (`claude.ts:3194-3248`) |
| **Skip cache write** | Not supported | Shifts marker to second-to-last message for fire-and-forget forks (`claude.ts:3175`) |
| **Static/dynamic split** | `FormatBoundaryCachedSystemPrompt` -- splits at `SYSTEM_PROMPT_STATIC_BOUNDARY`, uses different cache scopes (`prompt_caching.go:180-218`) | No equivalent -- uses GrowthBook-gated TTL and scope instead |

**Key difference:** Go uses **4 cache breakpoints** (matching the older Anthropic API model); upstream uses **1 breakpoint** (matching Mycro's turn-to-turn eviction model). Go's approach wastes KV pages on intermediate positions that Mycro would free. Upstream also has `cache_edits`/`cache_reference` for cached MC (model context) management -- Go has no equivalent.

---

### 10.15 Normalization -- Map Key Sorting for Deterministic Output

| Feature | Go (`normalize.go`) | Upstream |
|---------|---------------------|----------|
| **tool_use input sorting** | `sortMapKeys()` -- recursively sorts JSON keys alphabetically in `tool_use.input` (`normalize.go:90-108, 139-155`) | No equivalent -- `normalizeMessagesForAPI` normalizes tool inputs via `normalizeToolInputForAPI` (strips provider-specific fields) but doesn't sort keys |
| **tool_result whitespace** | `normalizeWhitespace()` -- collapses 3+ consecutive blank lines into 1, trims trailing whitespace per line, removes trailing blank lines (`normalize.go:175-201`) | No equivalent |
| **Full JSON normalization** | `NormalizeJSONBytes()` -- sorts keys in entire JSON payload (`normalize.go:204-216`) | No equivalent |
| **Purpose** | KV cache reuse: identical logical content produces identical API payload (`normalize.go:7-8`) | Not needed -- upstream relies on Anthropic's prefix cache which is byte-exact |

**Key difference:** Go's normalization is specifically designed for **cache hit optimization** on proxy/third-party providers. Sorting JSON keys and normalizing whitespace ensures identical logical content produces byte-identical payloads. Upstream doesn't need this because Anthropic's first-party API handles cache matching internally.

---

### 10.16 Summary of Critical Gaps

| Gap | Severity | Description |
|-----|----------|-------------|
| **Tool pairing validation** | CRITICAL | Go has NO `ensureToolResultPairing`. Orphaned tool_use/tool_result will cause API 400 errors. Upstream has 350-line dedicated function. |
| **Role alternation enforcement** | CRITICAL | Go doesn't merge consecutive user messages. API rejects multiple user messages in a row on Bedrock. |
| **Empty message filtering** | HIGH | Go doesn't filter whitespace-only or empty assistant messages, risking API 400. |
| **429 subscriber gating** | HIGH | Go always retries 429; upstream conditionally skips based on subscription type. May cause excessive retries for ClaudeAI subscribers. |
| **529 model fallback** | HIGH | Go has no Opus-to-Sonnet fallback on repeated 529. Upstream falls back after 3 consecutive 529s. |
| **Cache breakpoint count** | HIGH | Go uses 4 breakpoints; upstream uses 1. Go's extra 3 breakpoints waste KV pages on Mycro-based infrastructure. |
| **Overage tracking** | MEDIUM | Go has no overage status/flow. Upstream tracks `allowed_warning`/`rejected` overage states. |
| **Image/media validation** | MEDIUM | Go doesn't validate image sizes or strip images from error tool_results. |
| **x-should-retry header** | LOW | Go doesn't parse this header; upstream respects it. |
| **Persistent retry mode** | LOW | Go doesn't have unattended/persistent retry. Niche feature for long-running agents. |
| **Fast mode retry** | LOW | Go doesn't have fast-mode-aware retry with cache preservation. |
| **Auth retry (401/403)** | LOW | Go marks auth as non-retryable; upstream retries with credential refresh. |

---


---

## ERROR CLASSIFICATION AND ERROR TYPE SYSTEM: Go vs Upstream TypeScript

### 1. Error Type Taxonomy -- 15 Go Categories vs Upstream Classification

**Go** (`E:\Git\miniClaudeCode-go-github\error_types.go:17-33`): Defines a flat `ErrorClass` enum with 15 categories:

| Go ErrorClass | Purpose |
|---|---|
| `ECRetryable` | Generic retryable (network, 5xx) |
| `ECNonRetryable` | Generic non-retryable (4xx) |
| `ECContextOverflow` | Context too large -- compress |
| `ECToolPairing` | 2013 tool pairing broken |
| `ECRateLimit` | 429 -- backoff + retry |
| `ECBilling` | 402/credit exhausted |
| `ECModelNotFound` | Model doesn't exist -- fallback |
| `ECPayloadTooLarge` | 413 -- compress prompt |
| `ECOverloaded` | 503/529 -- provider overloaded |
| `ECTimeout` | Connection/read timeout |
| `ECFormatError` | 400 bad request |
| `ECAuth` | 401/403 -- rotate credential |
| `ECThinkingSig` | Thinking block signature invalid (defined but UNUSED in classifyError) |
| `ECLongContextTier` | 429 + "extra usage" + "long context" (defined but UNUSED in classifyError) |
| `ECUnknown` | Unclassifiable |

**Upstream** (`E:\Git\claude-code-upstream\src\services\api\errors.ts:968-1164`): `classifyAPIError()` returns 25+ string categories:

| Upstream Classification | Source Location |
|---|---|
| `aborted` | errors.ts:970 |
| `api_timeout` | errors.ts:975-981 |
| `repeated_529` | errors.ts:984-988 |
| `capacity_off_switch` | errors.ts:992-997 |
| `rate_limit` | errors.ts:1000 |
| `server_overload` | errors.ts:1005-1011 |
| `prompt_too_long` | errors.ts:1014-1021 |
| `pdf_too_large` | errors.ts:1024-1029 |
| `pdf_password_protected` | errors.ts:1031-1036 |
| `image_too_large` | errors.ts:1039-1056 |
| `tool_use_mismatch` | errors.ts:1059-1067 |
| `unexpected_tool_result` | errors.ts:1069-1075 |
| `duplicate_tool_use_id` | errors.ts:1077-1083 |
| `invalid_model` | errors.ts:1086-1092 |
| `credit_balance_low` | errors.ts:1095-1102 |
| `invalid_api_key` | errors.ts:1105-1110 |
| `token_revoked` | errors.ts:1113-1118 |
| `oauth_org_not_allowed` | errors.ts:1120-1128 |
| `auth_error` | errors.ts:1131-1136 |
| `bedrock_model_access` | errors.ts:1139-1145 |
| `server_error` (5xx) | errors.ts:1150 |
| `client_error` (4xx) | errors.ts:1151 |
| `ssl_cert_error` | errors.ts:1157-1158 |
| `connection_error` | errors.ts:1160 |
| `unknown` | errors.ts:1163 |

**Key differences:**
- Go has 15 ErrorClass constants; upstream has 25+ string labels via `classifyAPIError()`.
- Go's `ECThinkingSig` and `ECLongContextTier` are defined in the enum (`error_types.go:30-31`) but never returned by `classifyError()` -- dead code.
- Upstream has PDF-specific errors (`pdf_too_large`, `pdf_password_protected`) that Go lacks entirely.
- Upstream has `ssl_cert_error` classification; Go has none.
- Upstream has `capacity_off_switch` and `repeated_529` as named categories; Go maps these to generic `ECRetryable`/`ECOverloaded`.
- Upstream distinguishes `tool_use_mismatch`, `unexpected_tool_result`, `duplicate_tool_use_id`; Go lumps all as `ECToolPairing`.
- Go's `ClassifyResult` carries action flags (`Compress`, `RotateKey`, `Fallback`, `RetryAfter`); upstream's `classifyAPIError()` returns only a string label (for analytics/telemetry). Upstream's *action* decisions are in `shouldRetry()` and `getAssistantMessageFromError()`, not in the classifier.

### 2. Pattern Matching Database -- Regex-based Classification

**Go** (`error_types.go:61-144`): Uses string-array `matchesAny()` for 11+ pattern groups:
- `billingPatterns`: 10 patterns
- `rateLimitPatterns`: 14 patterns
- `usageLimitPatterns`: 4 patterns + 6 `usageLimitTransientSignals` for disambiguation
- `contextOverflowPatterns`: 24 patterns (including Chinese patterns)
- `modelNotFoundPatterns`: 8 patterns
- `authPatterns`: 9 patterns
- `serverDisconnectPatterns`: 7 patterns
- `networkErrorPatterns`: 12 patterns
- `serverErrorPatterns`: 4 patterns
- `transportErrorTypes`: 13 patterns
- `statusCodeRegex`: 3-digit extraction regex

**Upstream** (`errors.ts:425-934`): `getAssistantMessageFromError()` uses a cascade of type guards and `.includes()` checks:
- `is529Error()`: checks `error.status === 529` OR overloaded_error in message
- `isPromptTooLongMessage()`: "prompt is too long" prefix match
- `isMediaSizeError()`: "image exceeds" + "maximum" OR regex for PDF pages
- `parseMaxTokensContextOverflowError()`: regex parsing input/max token counts
- Token gap: `parsePromptTooLongTokenCounts()` with regex

**Key differences:**
- Go has 100+ total substring patterns across 11 groups; upstream has ~20 distinct checks.
- Go includes Chinese patterns; upstream is English-only.
- Go uses a single regex for HTTP status extraction; upstream relies on `APIError.status` property from the SDK.
- Go's `matchesAny` is O(n*m) substring search; upstream uses `.includes()` chains in a priority-ordered if/else cascade.
- Go's `parsePromptTooLongTokenGap()` and upstream's `getPromptTooLongTokenGap()` use nearly identical regexes.
- Go has no `parseMaxTokensContextOverflowError()` equivalent -- Go does not parse the input/max token gap for context adjustment.

### 3. Context Overflow Detection

**Go** (`agent_loop.go:1075-1094` and `agent_loop.go:1988-2001`):
- **Run loop** (`agent_loop.go:1075`): `isContextLengthError()` via `matchesAny()` against 24 patterns. Escalating recovery: TruncateHistory -> AggressiveTruncateHistory -> MinimumHistory, with `maxContextRecovery` cap.
- **Non-streaming fallback** (`agent_loop.go:1988-2001`): Uses a `consecutive500s` counter heuristic -- after 5 consecutive 500 errors, assumes context overflow and triggers compaction, returning `context_length_exceeded`.
- Status code 400 + large session > 40% context length or > 80K tokens -> `ECContextOverflow` (`error_types.go:367-370`)
- Server disconnect + large session (>60% context or >120K tokens) -> `ECContextOverflow` (`error_types.go:296-301`)

**Upstream** (`query.ts:1112-1229` and `withRetry.ts:388-426`):
- **No consecutive 500 heuristic.** Instead uses `parseMaxTokensContextOverflowError()` to detect specific 400 error and adjusts `maxTokensOverride` for the next attempt.
- **Reactive compact**: After 400 PTL error is withheld, tries `contextCollapse.recoverFromOverflow()` (drain staged context), then `reactiveCompact.tryReactiveCompact()` (summarize).
- **Multi-tier recovery**: collapse drain -> reactive compact -> surface error. Each is single-shot.
- Upstream has `max_output_tokens` recovery: if capped 8k default was used, escalates to 64k (`query.ts:1235-1267`), then multi-turn recovery messages (`query.ts:1270-1299`).

**Key differences:**
- Go: 5 consecutive 500s heuristic (coarse, works for any 500 cascade); upstream: precise 400 token-count parsing (requires specific error format).
- Go's recovery is truncation-based (Truncate/AggressiveTruncate/MinimumHistory); upstream's is compaction-based (collapse drain, reactive compact).
- Go has no `max_output_tokens` escalation path; upstream escalates 8k->64k->multi-turn recovery.
- Go's `isContextLengthError()` checks 24 substring patterns; upstream checks for "prompt is too long" prefix only.
- Go's `parsePromptTooLongTokenGap()` regex exists but is not used for auto-recovery; upstream's `getPromptTooLongTokenGap()` drives reactive compact token-targeting.

### 4. Rate Limit Detection -- Header Parsing

**Go** (`E:\Git\miniClaudeCode-go-github\rate_limit.go:49-264`):
- `RateLimitState` struct with 4 buckets: `RequestsMin`, `RequestsHour`, `TokensMin`, `TokensHour`
- Parses `x-ratelimit-*` headers (third-party provider format: OpenRouter, Nous Portal, OpenAI-compatible)
- Proactive delay: `rateLimitState.RetryDelay()` called BEFORE backoff; uses `min(rlim, delay*3)` (`agent_loop.go:1506-1510`)
- On 429 classification: `ECRateLimit` with `RotateKey=true`, `Fallback=true`
- Usage limit disambiguation: checks for "try again", "resets at" transient signals to distinguish rate limit vs billing

**Upstream** (`errors.ts:465-534` and `withRetry.ts:519-822`):
- ClaudeAI subscription rate limits: `anthropic-ratelimit-unified-representative-claim` header (five_hour, seven_day, seven_day_opus)
- `anthropic-ratelimit-unified-overage-status` header (allowed, allowed_warning, rejected)
- `anthropic-ratelimit-unified-overage-reset`, `anthropic-ratelimit-unified-reset` for reset timestamps
- `anthropic-ratelimit-unified-overage-disabled-reason` for rejection reasons
- `getRateLimitErrorMessage()` generates tiered messages based on limits object
- Fast mode cooldown on 429/529 with `triggerFastModeCooldown()` (`fastMode.ts:217-235`)
- `retry-after` header bypasses backoff entirely (`withRetry.ts:535-539`)
- `x-should-retry` header respected (`withRetry.ts:732-751`)

**Key differences:**
- Go tracks third-party rate limit buckets (requests/tokens per min/hr); upstream tracks ClaudeAI subscription quotas (5hr/7day windows).
- Go's rate limit state is shared across API calls via `AgentLoop.rateLimitState` pointer; upstream has no persistent rate limit state (parsed per-error).
- Go has no fast mode, so no fast mode cooldown on rate limits.
- Upstream has sophisticated overage/PAYG handling; Go has none.
- Upstream has `UNATTENDED_RETRY` persistent mode (indefinite retries with heartbeat); Go has bounded retries only.
- Go's `usageLimitPatterns` disambiguation (transient vs hard) is a rough proxy for upstream's overage-status header.

### 5. Tool Pairing Errors -- 2013 Error Handling

**Go** (`agent_loop.go:1038-1046, 1537-1548, 1963-1978` and `context.go:1470-1580`):
- Detects "2013" or "tool call result does not follow tool call" in error message
- Repairs: `ValidateToolPairing()` (removes orphaned tool_results), `FixRoleAlternation()` (merges consecutive same-role messages)
- Injects corrective hint to model: "A tool call result was not properly paired..."
- `ValidateToolPairing()` is two-pass: collect tool_use IDs, then remove orphaned tool_results
- Called proactively BEFORE `BuildMessages()` on every API call (`agent_loop.go:1479-1480`)

**Upstream** (`utils/messages.ts:5198-5538` and `claude.ts:1321-1324`):
- `ensureToolResultPairing()`: comprehensive pre-send validation on every API call
- Handles both directions: inserts synthetic `tool_result` blocks for missing results AND strips orphaned `tool_result` blocks
- Cross-message tool_use ID deduplication with `allSeenToolUseIds` Set
- Handles server-side tools: strips orphaned `server_tool_use` and `mcp_tool_use` blocks
- Strict mode: throws on mismatch for HFI training data collection
- Called at `claude.ts:1324` before building API payload
- On 400 error with tool_use mismatch: returns user-facing message with `/rewind` suggestion (`errors.ts:667-705`)
- Dedicated `logToolUseToolResultMismatch()` for Statsig telemetry (`errors.ts:222-382`)

**Key differences:**
- Go removes orphaned tool_results only; upstream also inserts synthetic tool_results for missing results (bidirectional repair).
- Go detects 2013 errors via substring match; upstream catches the specific API message about tool_use/tool_result mismatch.
- Go's `FixRoleAlternation()` merges consecutive same-role messages; upstream's `normalizeMessagesForAPI()` handles this during message normalization.
- Upstream has cross-message deduplication; Go validates within the flat entry list.
- Go has no telemetry for tool pairing repairs; upstream logs to Statsig.
- Upstream handles server_tool_use and mcp_tool_use orphans; Go does not.
- Upstream has strict mode for training data; Go has no equivalent.

### 6. Interrupt Detection -- atomic.Bool vs AbortController

**Go** (`agent_loop.go:293, 793-799, 857-890`):
- `interrupted atomic.Bool` flag set by Ctrl+C handler
- `interruptCtx(baseCtx, timeout)` wraps context with timeout + interrupt watcher goroutine (100ms ticker polling `IsInterrupted()`)
- `IsInterrupted()` checked at turn start, after tool execution, in tool goroutines
- `SetInterrupted(false)` resets after handling

**Upstream** (`query.ts:707, 1062, 1093, 1532, 1548`):
- `toolUseContext.abortController` -- AbortController with `signal.aborted` and `signal.reason`
- Reasons tracked: 'interrupt', 'streaming_fallback', etc.
- `signal.reason !== 'interrupt'` gates whether to emit interruption message (query.ts:1093, 1548)
- `createChildAbortController` for nested operations
- Propagates parent abort to child via controller hierarchy

**Key differences:**
- Go uses polling (100ms ticker in goroutine); upstream uses event-driven AbortController.
- Upstream tracks abort *reason* ('interrupt' vs others); Go has boolean-only.
- Upstream has hierarchical abort controllers for nested operations; Go uses flat `context.WithCancel`.
- Go's interrupt watcher goroutine creates one goroutine per API call; upstream's AbortController is event-based with no extra goroutines.
- Go's `interruptCtx` combines timeout + interrupt in one wrapper; upstream separates timeout from abort.

### 7. Stream Stall Detection

**Go** (`streaming.go:710-823`):
- `StreamAdapter` has `stallTimeoutMs` and `startupMs` fields with dynamic timeouts
- Stall detection goroutine with buffered channel (`stallReset chan struct{}, 16`)
- Dynamic timeouts: Local=300s stall/600s startup; >100K tokens=300s/360s; >50K=240s/300s; Default=300s/600s
- `stallCount` tracked (reset on each event), force-closes stream and cancels context on timeout
- On stall error in Run loop: progressive recovery (TruncateHistory -> AggressiveTruncateHistory -> MinimumHistory)

**Upstream** (`claude.ts:1924-2023`):
- **Streaming idle timeout watchdog**: `CLAUDE_ENABLE_STREAM_WATCHDOG` env var (opt-in), defaults 90s idle timeout, 45s warning
- `setTimeout`-based watchdog actively kills hung streams (not passive detection)
- `STALL_THRESHOLD_MS = 30_000` for logging stalls (passive, logging-only, does NOT kill)
- `stallCount` and `totalStallTime` tracked for telemetry via `tengu_streaming_stall` event
- Stall logging fires only *after* first event (avoids counting TTFB)

**Key differences:**
- Go has always-on, aggressive stall detection with automatic recovery; upstream's watchdog is env-gated and upstream's stall detection is telemetry-only.
- Go's timeouts are 240-300s; upstream's watchdog default is 90s (when enabled).
- Go has dynamic timeout adjustment based on token count and provider type; upstream has fixed configurable timeout.
- Go's stall recovery involves truncation; upstream's watchdog just aborts with no automatic recovery in the stream layer.
- Upstream has a warning phase (half timeout); Go does not.
- Go returns `fmt.Errorf("stream stalled: %w", err)` which the Run loop catches and recovers from; upstream logs and aborts without higher-level recovery.

### 8. Recovery Hints -- "Try X to resolve" Hints

**Go** (`agent_loop.go:1035, 1044, 1050, 1072`):
- `ClassifyResult` has action flags: `Compress`, `RotateKey`, `Fallback`
- Recovery hints injected as user messages:
  - Model confusion: "ERROR: Your previous response was malformed. Do NOT output tool syntax as text." (`agent_loop.go:1035`)
  - Tool pairing: "A tool call result was not properly paired with its call..." (`agent_loop.go:1044`)
  - Truncated arguments: "ERROR: Your tool call arguments was cut off due to length limits..." (`agent_loop.go:1050`)
  - Truncation continuation via `injectTruncationContinuation()` (`agent_loop.go:1072`)
- Warning suppression: suppresses warnings after first recovery attempt (`agent_loop.go:1055-1067`)

**Upstream** (`query.ts:1270-1275`):
- `max_output_tokens` recovery: "Output token limit hit. Resume directly -- no apology, no recap..." (`query.ts:1273-1274`)
- User-facing messages via `getAssistantMessageFromError()`:
  - Rate limit: specific messages based on quota headers, overage status
  - Auth: "Please run /login" or "Fix external API key"
  - Model: "Run /model to pick a different model"
  - PDF: "Try reading the file a different way" or "Double press esc"
- Withhold-and-recover pattern: errors withheld from stream until recovery exhausts (`query.ts:834-862`)
- Stop hooks skipped on API errors to prevent death spirals (`query.ts:1305-1311`)

**Key differences:**
- Go's `ClassifyResult` has structured action flags (`Compress`, `RotateKey`, `Fallback`); upstream has no equivalent structured classification result.
- Upstream's error messages are user-facing and contextual (CLI vs SDK mode); Go's recovery hints are model-directed.
- Upstream has withhold-and-recover: PTL and media errors withheld until recovery path is determined; Go surfaces errors immediately and reacts.
- Upstream has max_output_tokens multi-turn recovery with escalating prompts; Go has none.
- Go injects corrective hints to the *model*; upstream presents recovery options to the *user*.
- Both have death spiral prevention: Go via `maxContextRecovery` cap, upstream via `hasAttemptedReactiveCompact` guard and stop hook skip.

### 9. Chinese Error Message Patterns (Go-Specific)

**Go** (`error_types.go:98-100`):
- `contextOverflowPatterns` includes Chinese: "超过最大长度" (exceeds max length), "上下文长度" (context length), "max input token", "input token"
- Go is the only codebase with non-English error pattern matching
- This enables Go to classify errors from Chinese-speaking providers (domestic LLM APIs like DeepSeek, Qwen, etc.)
- No equivalent in upstream TypeScript (English-only)

### 10. Error Propagation Through Layers

**Go error propagation chain:**
1. **Streaming layer** (`streaming.go`): `StreamAdapter.Process()` returns `error` from handler. Stall detection goroutine force-closes stream -> `fmt.Errorf("stream stalled: %w", err)`
2. **API retry layer** (`agent_loop.go:1576-1684`): `callWithRetryAndFallback()` -- tries `tryStreamOnce()`, checks `isTransientError()`, `isContextLengthError()`, "model confused", "2013", "stream stalled". Falls back to `callWithNonStreamingFallback()`.
3. **Run loop** (`agent_loop.go:1024-1096`): `Run()` -- handles errors from `callWithRetryAndFallback()` and `callWithNonStreamingOnly()`. Progressive recovery: TruncateHistory -> AggressiveTruncateHistory -> MinimumHistory. Returns string error to caller.
4. **Tool layer** (`tools/exec_tool.go`): Tool errors wrapped as `ToolResult{Output: fmt.Sprintf("Error: %v", err), IsError: true}`. Passed back to API as tool_result blocks.

**Upstream error propagation chain:**
1. **Streaming layer** (`claude.ts:1924-2200`): `queryModelWithStreaming()` yields messages. Watchdog aborts on idle. Stalls logged. Errors yielded as synthetic assistant messages via `getAssistantMessageFromError()`.
2. **Retry layer** (`withRetry.ts:170-517`): `withRetry<T>()` async generator. Yields `SystemAPIErrorMessage` for retry waits. Throws `CannotRetryError` on exhaustion. Handles 529 counter, fast mode cooldown, OAuth refresh, persistent retry.
3. **Query loop** (`query.ts:1002-1349`): Catch block yields error message, returns `{ reason: 'model_error' }`. Post-stream recovery: check withheld PTL/media errors -> collapse drain -> reactive compact -> max_output_tokens recovery -> surface.
4. **Tool layer**: `executeTool()` returns errors as tool_result content with `is_error: true`. Abort controller checked mid-tool.

**Key differences:**
- Go's errors are string-based (`errMsg := err.Error()` + substring matching); upstream uses typed errors (`APIError`, `APIConnectionError`, etc.).
- Go has 4 layers: streaming -> retry -> Run loop -> tool; upstream has 4 layers: streaming -> withRetry -> queryLoop -> tool.
- Go's Run loop is the central error recovery coordinator; upstream's query loop handles recovery after streaming completes.
- Upstream's `withRetry` is an async generator that yields retry status messages to stdout; Go's retry is a simple for-loop with `time.Sleep`.
- Go's error classification (`classifyError`) is called at the retry layer; upstream's `classifyAPIError` is for analytics only, not for retry decisions (which use `shouldRetry`).
- Upstream has error "withholding": API errors yielded as synthetic assistant messages but withheld from SDK users until recovery is determined; Go has no withholding -- errors are surfaced immediately.
- Upstream's `FallbackTriggeredError` switches models on 529 exhaustion; Go has no model fallback.

---


---

## 43. Error Handling, Work Tasks, Agent Progress, CLI Entry Points

### Files Compared
- **Go**: `error_types.go`, `agent_task.go`, `work_task.go`, `agent_progress.go`, `main.go`
- **Upstream**: `utils/errors.ts`, `services/api/errors.ts`, `utils/errorLogSink.ts`, `tasks/`, `utils/activityManager.ts`, `main.tsx`

### 43.1 Error Handling System

| # | Aspect | Go (`error_types.go`) | Upstream (`errors.ts`, `api/errors.ts`) | Type |
|---|--------|----------------------|----------------------------------------|------|
| 1 | Error type taxonomy | 15-category `ErrorClass` enum with recovery hints (line 15-33) | 6 named classes: ClaudeError, MalformedCommandError, AbortError, ConfigParseError, ShellError, TelemetrySafeError (`errors.ts:3-101`) | Go增强 |
| 2 | Abort error detection | No abort error type | `isAbortError()` checks AbortError, APIUserAbortError, DOMException (`errors.ts:27-33`) | 缺失 |
| 3 | API error classification | `classifyError()` with 15-class taxonomy, status codes, pattern matching, recovery hints (line 155-324) | `classifyAPIError()` returns analytics tag strings (20+ categories) (`api/errors.ts:968-1164`) | Go适配 |
| 4 | API error → user message | No user-facing message formatter | `getAssistantMessageFromError()` converts errors to user-facing `AssistantMessage` (`api/errors.ts:425-934`) | 缺失 |
| 5 | Retry categorization | `ClassifyResult` with `Retryable/Compress/RotateKey/Fallback` (line 36-45) | `categorizeRetryableAPIError()` returns `SDKAssistantMessageError` (`api/errors.ts:1166-1185`) | Go增强 |
| 6 | Config parse errors | No ConfigParseError | `ConfigParseError` with filePath + defaultConfig fields (`errors.ts:39-49`) | 缺失 |
| 7 | Shell errors | No dedicated ShellError | `ShellError` with stdout/stderr/code/interrupted (`errors.ts:51-61`) | 缺失 |
| 8 | Telemetry-safe errors | No dual-message design | `TelemetrySafeError` dual-message design (`errors.ts:93-101`) | 缺失 |
| 9 | Error normalization | No `toError`/`errorMessage`/`shortErrorStack` | `toError()`, `errorMessage()`, `getErrnoCode()`, `isENOENT()`, `shortErrorStack()` (`errors.ts:111-171`) | 缺失 |
| 10 | FS error helpers | No FS error helpers | `isFsInaccessible()` covering ENOENT/EACCES/EPERM/ENOTDIR/ELOOP (`errors.ts:186-195`) | 缺失 |
| 11 | Error log sink | No file-based error logging | File-based JSONL error logging with Sentry integration (`errorLogSink.ts:1-240`) | 缺失 |
| 12 | Error IDs | No numeric error IDs | 346 numeric error IDs for production tracking (`constants/errorIds.ts:1-16`) | 缺失 |
| 13 | Prompt token gap parsing | `parsePromptTooLongTokenGap()` (line 467-480) | `parsePromptTooLongTokenCounts()` + `getPromptTooLongTokenGap()` (`api/errors.ts:85-96`) | 修正 |

### 43.2 Work/Task System

| # | Aspect | Go (`agent_task.go`, `work_task.go`) | Upstream (`tasks/`) | Type |
|---|--------|--------------------------------------|---------------------|------|
| 1 | Task types | Single unified `TaskState` with `SubagentType` field (agent_task.go:18-26) | 7 types: local_bash, local_agent, remote_agent, in_process_teammate, local_workflow, monitor_mcp, dream (`Task.ts:6-13`) | 简化 |
| 2 | Task ID generation | Uses agentID passed from caller (no ID generator) | Prefix-based random IDs (8 crypto bytes, type-specific prefixes) (`Task.ts:79-106`) | 简化 |
| 3 | Task state fields | `TaskState` with Model/SubagentType/TranscriptPath/ToolsUsed/DurationMs/CancelFunc/Process (agent_task.go:46-65) | `TaskStateBase` with id/type/status/description/toolUseId/startTime/endTime/totalPausedMs/outputFile/outputOffset/notified (`Task.ts:45-57`) | Go增强 |
| 4 | Progress tracking | No progress tracker | `ProgressTracker`, `updateProgressFromMessage()`, `getProgressUpdate()` (`LocalAgentTask.tsx:46-156`) | 缺失 |
| 5 | Background/foreground agents | No foreground/background distinction | `registerAgentForeground()`, `backgroundAgentTask()`, `unregisterAgentForeground()` with signal promises (`LocalAgentTask.tsx:645-808`) | 缺失 |
| 6 | Agent summary updates | No agent summary updates | `updateAgentSummary()` with SDK progress emission (`LocalAgentTask.tsx:458-508`) | 缺失 |
| 7 | Work tasks (LLM TODOs) | `WorkTaskStore` with task creation, dependency graphs, cycle detection (work_task.go:1-276) | No work task store | Go增强 |
| 8 | Cycle detection | `wouldCreateCycle()` BFS over both Blocks/BlockedBy edges (work_task.go:242-264) | No equivalent | Go增强 |
| 9 | Dream tasks | No dream task type | DreamTask type (`tasks/DreamTask/`) | 缺失 |
| 10 | Remote agent tasks | No remote agent task type | RemoteAgentTask (`tasks/RemoteAgentTask/`) | 缺失 |
| 11 | In-process teammate tasks | No teammate task type | InProcessTeammateTask (`tasks/InProcessTeammateTask/`) | 缺失 |
| 12 | Workflow tasks | No workflow task type | LocalWorkflowTask (`tasks/LocalWorkflowTask/`) | 缺失 |
| 13 | Monitor MCP tasks | No MCP monitor task type | MonitorMcpTask (`tasks/MonitorMcpTask/`) | 缺失 |

### 43.3 Agent Progress

| # | Aspect | Go (`agent_progress.go`) | Upstream (`activityManager.ts`, `LocalAgentTask.tsx`) | Type |
|---|--------|--------------------------|------------------------------------------------------|------|
| 1 | Activity manager | `subAgentProgressWriter` — terminal-only progress writer, no user activity tracking (line 1-216) | `ActivityManager` with user/CLI activity tracking, operation deduplication, timeout-based activity detection (`activityManager.ts:1-164`) | 简化 |
| 2 | User vs CLI activity | No user activity tracking | `recordUserActivity()` with 5s timeout (`activityManager.ts:60-81`) | 缺失 |
| 3 | CLI operation tracking | No CLI operation tracking | `startCLIActivity()`/`endCLIActivity()` with deduplication (`activityManager.ts:86-125`) | 缺失 |
| 4 | SDK progress emission | No SDK progress emission | `emitTaskProgress()` for VS Code subagent panel (`LocalAgentTask.tsx:496-507`) | 缺失 |
| 5 | Progress token updates | `UpdateTokens()` for live token display (agent_progress.go:150-155) | `updateProgressFromMessage()` tracking input/output tokens separately (`LocalAgentTask.tsx:100-144`) | 简化 |

### 43.4 CLI Entry Points

| # | Aspect | Go (`main.go`) | Upstream (`main.tsx`) | Type |
|---|--------|---------------|----------------------|------|
| 1 | CLI entry point | `main()` with flag parsing, config priority chain, agent initialization, one-shot/REPL dispatch (line 19-188) | 244KB React-based TUI with full terminal UI, command palette, vim mode (`main.tsx:1-6800+`) | 简化 |
| 2 | Permission modes | 4 modes: ask/auto/bypass/plan (line 386-393) | Same 4 modes + SDK permission flow with hooks race | 简化 |
| 3 | Transcript resume | `findTranscript()` by number/name/last (line 516-555) | Resume via teleport, worktree, session state restoration | 简化 |
| 4 | Slash commands | `/quit /tools /mode /help /compact /partialcompact /clear /resume /agents` (line 363-456) | 30+ commands including `/login /mock-limits /config` etc. (`commands.ts:1-700+`) | 简化 |
| 5 | Ctrl+C handling | Atomic timestamp, double-press exit, signal + channel dual detection (line 191-218) | React-based interrupt handling via AppState | Go适配 |
| 6 | Structured IO / SDK protocol | No structured IO | `StructuredIO` class with control requests, permission prompts, hooks, elicitation, MCP messaging (`structuredIO.ts:1-863`) | 缺失 |
| 7 | Bridge protocol | No bridge protocol | CCR (Claude Code Remote) bridge with WebSocket, JWT auth, session management (`bridge/`) | 缺失 |

### 43.5 Remaining Upstream Modules (Not in Go)

| # | Aspect | Go | Upstream | Type |
|---|--------|----|----------|------|
| 1 | Analytics/telemetry | No analytics | Statsig, Datadog, event tracking, A/B testing (`services/analytics/`) | 缺失 |
| 2 | OAuth | No OAuth | OAuth flow, token management, organization auth (`services/oauth/`) | 缺失 |
| 3 | Settings sync | No settings sync | Cloud settings synchronization (`services/settingsSync/`) | 缺失 |
| 4 | Team memory sync | No team memory sync | Shared team memory (`services/teamMemorySync/`) | 缺失 |
| 5 | Remote managed settings | No managed settings | Admin-managed settings (`services/remoteManagedSettings/`) | 缺失 |
| 6 | Skill learning | No skill learning | Skill learning and improvement (`services/skillLearning/`) | 缺失 |
| 7 | Skill search | No skill search | Skill discovery and search (`services/skillSearch/`) | 缺失 |
| 8 | LSP integration | No LSP | Language Server Protocol integration (`services/lsp/`) | 缺失 |
| 9 | Auto Dream | No auto dream | Automatic background planning/dreaming (`services/autoDream/`) | 缺失 |
| 10 | Voice | No voice | Voice input, STT streaming, keyterm detection (`services/voice.ts`) | 缺失 |
| 11 | ACP protocol | No ACP | Agent Communication Protocol (`services/acp/`) | 缺失 |
| 12 | Coordinator | No coordinator | Multi-agent coordination panel (`coordinator/`) | 缺失 |
| 13 | Vim mode | No vim mode | Full vim keybindings, modes, navigation (`vim/`) | 缺失 |
| 14 | Sandboxing | No sandboxing | Containerized execution, network permissions (`sandbox/`) | 缺失 |
| 15 | SSH/remote | No SSH | SSH remote execution, remote environments (`ssh/`, `remote/`) | 缺失 |
| 16 | Plugins | No plugins | Plugin system (`plugins/`) | 缺失 |
| 17 | Proactive features | No proactive features | Proactive suggestions, notifications (`proactive/`) | 缺失 |
| 18 | VCR (record/replay) | No VCR | API response recording/replaying for testing (`services/vcr.ts`) | 缺失 |
| 19 | Diagnostic tracking | No diagnostic tracking | Diagnostic event collection (`services/diagnosticTracking.ts`) | 缺失 |
| 20 | Rate limit mocking | No rate limit mocking | `/mock-limits` command for testing (`services/mockRateLimits.ts`) | 缺失 |


---

