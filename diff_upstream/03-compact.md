# Compaction System

> Compaction, cache, post-compact recovery

## Sections Included
- [##] Line 203-263 -- ## 3. Prompt Caching
- [##] Line 1348-1628 -- ## compact.go vs Upstream Compaction System
- [###] Line 2176-2181 -- ### 15.7 Prompt Caching — Boundary Splitting vs Upstream
- [###] Line 2182-2187 -- ### 15.8 Prompt Caching — Global Cache Scope Conditionality
- [###] Line 2188-2193 -- ### 15.9 Prompt Caching — Cache Break Detection
- [##] Line 3404-3709 -- ## PostCompactRecovery System — Deep Comparison
- [##] Line 6555-6572 -- ## 7.1 Cache Breakpoint Placement
- [##] Line 6573-6594 -- ## 7.2 Cache Edit Injection
- [##] Line 6595-6602 -- ## 7.3 Cache Sharing Across Forked Agents
- [##] Line 6603-6617 -- ## 7.4 Cache Invalidation on Micro-Compact
- [##] Line 6618-6626 -- ## 7.5 Cache Creation/Read Token Tracking
- [##] Line 6627-6635 -- ## 7.6 Pinned Cache Edits - Upstream's Always-Cached Blocks
- [##] Line 6636-6658 -- ## 7.7 Cache Break Detection - Upstream Only
- [##] Line 9486-9561 -- ## 36. Compact.go Deep Dive — Prompt, Retry, Post-Compact Recovery

---

## Content

## 3. Prompt Caching

### Files Compared
- **Go**: `E:\Git\miniClaudeCode-go-github\prompt_caching.go` (219 lines)
- **Upstream**: `E:\Git\claude-code-upstream\src\services\api\claude.ts` (cache control at lines 325-673, 3147-3324) + `E:\Git\claude-code-upstream\src\utils\api.ts` (`splitSysPromptPrefix` at line 321)

### 3.1 Cache Breakpoint Placement Strategy

| Aspect | Go | Upstream TS |
|--------|-----|-------------|
| Strategy | System prompt + last 3 non-system messages (4 total) | **Exactly 1 marker** per request (line 3164-3175) |
| System prompt caching | Yes -- first message if role="system" (line 31) | Via `buildSystemPromptBlocks` / `splitSysPromptPrefix` with cacheScope |
| Message-level markers | Multiple (up to 4) | Single marker at last message (or second-to-last for skipCacheWrite) |
| Rationale | "Reduces input token costs by ~75%" | Anthropic's local-attention KV pages: multiple markers protect second-to-last locals unnecessarily (lines 3164-3174) |

**Finding**: Go's 4-breakpoint strategy is fundamentally different from upstream's single-marker approach. Upstream explicitly explains (line 3164-3174) that multiple markers cause KV page retention issues with Mycro's turn-to-turn eviction.

### 3.2 Cache Control Scope

| Scope | Go | Upstream TS |
|-------|-----|-------------|
| Ephemeral | Yes -- `{"type": "ephemeral"}` (line 23) | Yes -- `{type: 'ephemeral'}` |
| 1h TTL | Yes -- conditional `"ttl": "1h"` (line 24-26) | Yes -- `should1hCacheTTL()` with GrowthBook gating (lines 385-420) |
| Global scope | **No** | Yes -- `scope: 'global'` for static content (line 392) |
| Org scope | **No** | Yes -- `scope: 'org'` for per-session content (line 353) |
| Eligibility gating | **No** | GrowthBook allowlist + subscriber status (lines 368-420) |

**Gap**: Go has no concept of `scope: 'global'` or `scope: 'org'` cache scopes, only `ephemeral`. This means Go cannot leverage cross-session cache persistence.

### 3.3 Cache Edit Injection

| Capability | Go | Upstream TS |
|-----------|-----|-------------|
| cache_edits blocks | **No** | Yes -- `CachedMCEditsBlock` insertion (lines 3214-3248) |
| cache_reference on tool_results | **No** | Yes -- added to tool_results before last cache_control (lines 3250-3294) |
| Pinned edits re-insertion | **No** | Yes -- `getPinnedCacheEdits` at original positions (lines 3214-3225) |
| Edit deduplication | **No** | Yes -- `deduplicateEdits` with `seenDeleteRefs` (lines 3202-3211) |
| Cache break detection | **No** | Yes -- `checkResponseForCacheBreak` (line 2441) |

**Gap**: Go has no cache edit support whatsoever. Upstream's cache_editing feature enables sophisticated KVCC (Key-Value Cache Compression) with per-block deletions and re-insertions.

### 3.4 System Prompt Caching

| Aspect | Go | Upstream TS |
|--------|-----|-------------|
| Approach | Single `cache_control` on first system message (line 31-34) | Multi-block via `splitSysPromptPrefix` with cacheScope: null/org/global |
| Static/dynamic boundary | Yes -- `SYSTEM_PROMPT_STATIC_BOUNDARY` = `"<!-- STATIC_PROMPT_END -->"` (line 21) | Yes -- `SYSTEM_PROMPT_DYNAMIC_BOUNDARY` = `"__SYSTEM_PROMPT_DYNAMIC_BOUNDARY__"` |
| Boundary cache split | `FormatBoundaryCachedSystemPrompt` -- global for static, ephemeral for dynamic (lines 172-218) | `splitSysPromptPrefix` -- up to 4 blocks with attribution/prefix/static/dynamic (lines 362-402) |
| Attribution header handling | **No** | Yes -- `x-anthropic-billing-header` as separate block with `cacheScope: null` |
| CLI_SYSPROMPT_PREFIX recognition | **No** | Yes -- cached blocks identified by `CLI_SYSPROMPT_PREFIXES` set |

### 3.5 Cache Miss Handling

| Capability | Go | Upstream TS |
|-----------|-----|-------------|
| Cache hit/miss tracking | **No** -- no cache token counters | Yes -- tracks cache_creation_input_tokens, cache_read_input_tokens |
| Cache break detection | **No** | Yes -- `promptCacheBreakDetection.ts` with full hash comparison |
| Fallback on cache miss | N/A | Non-streaming fallback, model fallback |

---


---

## compact.go vs Upstream Compaction System

*Compared: `E:\Git\miniClaudeCode-go-github\compact.go` against `E:\Git\claude-code-upstream\src\services\compact\*.ts`, `src\utils\tokens.ts`, `src\utils\contextAnalysis.ts`, `src\commands\force-snip.ts`*

### 1.1 Architecture — Monolithic vs Multi-File
- **上游**: Compaction logic spread across ~10 files: `compact.ts` (1712 lines, LLM compaction + post-compact recovery), `prompt.ts` (375 lines, prompts + formatCompactSummary), `microCompact.ts` (531 lines, cached + time-based MC), `cachedMicrocompact.ts` (113 lines, cache_edits state), `sessionMemoryCompact.ts` (631 lines, SM-compact algorithm), `autoCompact.ts` (352 lines, trigger logic + circuit breaker), `reactiveCompact.ts` (stub), `snipCompact.ts` (stub), `grouping.ts` (64 lines, API-round grouping), `postCompactCleanup.ts` (78 lines, cache/state reset), `timeBasedMCConfig.ts` (44 lines, GrowthBook config)
- **Go版**: All compaction logic in single `compact.go` (2893 lines): token estimation, round grouping, rule-based compact, LLM compact, SM-compact, partial compact, selective compact, smart compact, micro-compact, cached MC tracker, context window tracking, image stripping, sensitive redaction, reactive compact, archive I/O
- **类型**: Go适配

### 1.2 LLM Compaction Prompt — PARTIAL_COMPACT_UP_TO_PROMPT Missing
- **上游**: Three prompt variants: `BASE_COMPACT_PROMPT` (full conversation), `PARTIAL_COMPACT_PROMPT` (from direction), `PARTIAL_COMPACT_UP_TO_PROMPT` (up_to direction — "Context for Continuing Work" section replaces "Current Work"/"Optional Next Step") (`prompt.ts:208-267`)
- **Go版**: Two prompt variants: `baseCompactPrompt` (line 949), `partialCompactPrompt` (line 1035). No `PARTIAL_COMPACT_UP_TO_PROMPT` equivalent — up_to direction uses the same partial prompt as "from"
- **类型**: 缺失

### 1.3 LLM Compaction Prompt — Custom Instructions / Hook Instructions
- **上游**: `getCompactPrompt(customInstructions)` and `getPartialCompactPrompt(customInstructions, direction)` both accept custom instructions; `executePreCompactHooks` can inject `newCustomInstructions`; `mergeHookInstructions` merges user + hook instructions (`compact.ts:377-385`, `prompt.ts:274-303`)
- **Go版**: `doCompactLLMCall` (line 1530) uses hardcoded prompts with no custom/hook instruction injection path
- **类型**: 缺失

### 1.4 LLM Compaction Flow — Forked Agent for Cache Sharing
- **上游**: `streamCompactSummary` first attempts `runForkedAgent` to share main thread's prompt cache (system prompt + tools + context prefix) (`compact.ts:1140-1402`). Falls back to regular streaming on failure. GrowthBook-gated `tengu_compact_cache_prefix` (default true for 3P)
- **Go版**: `doCompactLLMCall` (line 1530) makes a direct streaming API call with no forked-agent / cache-sharing path
- **类型**: 缺失

### 1.5 LLM Compaction Flow — Streaming Retry with getRetryDelay
- **上游**: `streamCompactSummary` retries with `getRetryDelay(attempt)` (exponential + jitter) up to `MAX_COMPACT_STREAMING_RETRIES=2`, gated by `tengu_compact_streaming_retry` GrowthBook flag (`compact.ts:1255-1395`)
- **Go版**: `doCompactLLMCallWithRetry` (line 1676) retries up to `MAX_COMPACT_STREAMING_RETRIES=2` with quadratic backoff (5×attempt² seconds). No GrowthBook gate — always retries on transient errors
- **类型**: Go适配

### 1.6 LLM Compaction Flow — ToolSearchTool in Compact Request
- **上游**: When `isToolSearchEnabled`, includes `ToolSearchTool` + MCP tools (with `defer_loading: true`) in the compact request so tool results get counted but don't consume context tokens (`compact.ts:1269-1294`)
- **Go版**: Compact request sends only `anthropic.MessageNewParams` with `Thinking: disabled` and `context_management` edits. No ToolSearchTool or MCP tools in compact request
- **类型**: 缺失

### 1.7 LLM Compaction Flow — Image Stripping
- **上游**: `stripImagesFromMessages` replaces `image`/`document` blocks in user messages (including nested inside tool_result) with `[image]`/`[document]` text markers (`compact.ts:149-204`). Also `stripReinjectedAttachments` removes `skill_discovery`/`skill_listing` attachments
- **Go版**: `stripImages` (line 2613) uses regex to replace base64 image data and image URLs with `[image content stripped]` in text blocks, plus replaces document blocks. Works on text content rather than structured blocks. No `stripReinjectedAttachments` equivalent
- **类型**: Go适配

### 1.8 formatCompactSummary — <analysis>/<summary> Tag Processing
- **上游**: `formatCompactSummary` strips `<analysis>` block, extracts `<summary>` content and replaces tags with `Summary:\n` header, collapses extra whitespace (`prompt.ts:311-335`)
- **Go版**: `extractSummaryFromCompactOutput` (line 1980) strips `<analysis>` block with regex, extracts `<summary>` inner content. Returns just the content without `Summary:\n` header prefix. No extra-whitespace collapsing
- **类型**: Go适配

### 1.9 Summary Message — getCompactUserSummaryMessage
- **上游**: `getCompactUserSummaryMessage` builds session-continuation message with: formatted summary, optional transcript path, `recentMessagesPreserved` notice, and `suppressFollowUpQuestions` continuation directive. Proactive-mode awareness (`prompt.ts:337-375`)
- **Go版**: `doCompactLLMCall` (line 1636-1643) manually builds the summary message with: boundary text, transcript path, `recentMessagesPreserved`, and continuation directive. No proactive-mode awareness. Slightly different phrasing
- **类型**: Go适配

### 1.10 Session Memory Compaction (SM-compact) Algorithm
- **上游**: `trySessionMemoryCompaction` in `sessionMemoryCompact.ts`: checks GrowthBook flags (`tengu_session_memory` + `tengu_sm_compact`), initializes remote config, waits for `waitForSessionMemoryExtraction`, checks `isSessionMemoryEmpty`, computes `calculateMessagesToKeepIndex` (minTokens=10K, minTextBlockMessages=5, maxTokens=40K), `adjustIndexToPreserveAPIInvariants` (tool pairs + thinking blocks with same message.id), creates `CompactionResult` with `createCompactionResultFromSessionMemory`, runs `processSessionStartHooks`, truncates oversized sections via `truncateSessionMemoryForCompact` (`sessionMemoryCompact.ts:1-631`)
- **Go版**: No SM-compact implementation in compact.go. The `CompactTriggerSMCompact` constant exists (line 877) but no algorithm. SM-compact logic referenced in `PartialCompact` via `conclusions` parameter and `entriesToSummaryTextForMessagesParams` but no full SM-compact flow
- **类型**: 缺失

### 1.11 SM-compact Config — Remote GrowthBook
- **上游**: `initSessionMemoryCompactConfig` fetches `tengu_sm_compact_config` from GrowthBook with `minTokens`/`minTextBlockMessages`/`maxTokens` thresholds, validated to prevent zero overrides (`sessionMemoryCompact.ts:102-130`)
- **Go版**: No remote config. `CachedMicrocompactTracker` has hardcoded `maxTools=10`, `keepRecent=5` (line 2725-2726)
- **类型**: 缺失

### 1.12 SM-compact — adjustIndexToPreserveAPIInvariants (Thinking Blocks)
- **上游**: Two-step adjustment: (1) tool_use/tool_result pair preservation, (2) assistant messages sharing same `message.id` (streaming splits) are kept together so `normalizeMessagesForAPI` can merge thinking blocks. Walks backwards to find matching IDs (`sessionMemoryCompact.ts:232-314`)
- **Go版**: `adjustPivotForToolPairs` (line 2172) handles tool pairs only. No equivalent for thinking-block merging / streaming message.id grouping
- **类型**: 缺失

### 1.13 Micro-compact — Count-Based Cached MC
- **上游**: `cachedMicrocompactPath` registers tool results per user message, tracks `toolOrder`, deletes when `active.length > TRIGGER_THRESHOLD(10)`, keeps last `KEEP_RECENT(5)`, creates `CacheEditsBlock` with `delete_tool_result` edits. State includes `pinnedEdits` for re-sending at original positions. `consumePendingCacheEdits`/`pinCacheEdits` manage lifecycle (`microCompact.ts:305-399`, `cachedMicrocompact.ts:1-113`)
- **Go版**: `CachedMicrocompactTracker` (line 2706) mirrors the core logic: `registeredTools`, `toolOrder`, `deletedRefs`, `maxTools=10`, `keepRecent=5`. `GetCacheEditsBlock` (line 2802) creates same `cache_edits`/`delete_tool_result` structure. `toolsSentToAPI` flag prevents double-sending. No `pinnedEdits` re-send mechanism
- **类型**: 简化

### 1.14 Micro-compact — Cached MC Pinned Edits Re-Send
- **上游**: `pinCacheEdits(userMessageIndex, block)` stores edits at their original position; `getPinnedCacheEdits()` returns all previously-pinned edits that must be re-sent in subsequent API calls for cache hits (`microCompact.ts:100-118`)
- **Go版**: `pinnedEdits` field exists on `CachedMicrocompactTracker` (line 2711) as `[]any` but is never populated or read — no pin/re-send logic
- **类型**: 缺失

### 1.15 Micro-compact — Time-Based Trigger
- **上游**: `maybeTimeBasedMicrocompact` fires when gap since last assistant message exceeds `gapThresholdMinutes` (default 60, GrowthBook `tengu_slate_heron`). Content-clears old compactable tool results with `TIME_BASED_MC_CLEARED_MESSAGE = '[Old tool result content cleared]'`. Resets `cachedMCState` after firing. Notifies `promptCacheBreakDetection` (`microCompact.ts:412-530`, `timeBasedMCConfig.ts:1-44`)
- **Go版**: No time-based micro-compact trigger. `ResetForTimeBasedMC` (line 2859) exists as a stub but is never called since the time-based trigger is not implemented
- **类型**: 缺失

### 1.16 Micro-compact — Prompt Cache Break Detection Integration
- **上游**: Both cached and time-based MC paths call `notifyCacheDeletion(querySource)` or `notifyCompaction(querySource, agentId)` to suppress false-positive cache break alerts (`microCompact.ts:366-367, 525-527`, `autoCompact.ts:302-304`)
- **Go版**: No cache break detection integration
- **类型**: 缺失

### 1.17 Micro-compact — Main-Thread Source Filtering
- **上游**: `isMainThreadSource` uses prefix-match (`startsWith('repl_main_thread')`) to exclude subagents (session_memory, prompt_suggestion) from cached MC and time-based MC. Also filters output-style variants correctly (`microCompact.ts:249-251, 431`)
- **Go版**: No source-based filtering in `CachedMicrocompactTracker`. All callers share the same tracker regardless of agent/source
- **类型**: 缺失

### 1.18 Token Estimation — API Usage vs Heuristic
- **上游**: `tokenCountWithEstimation` (primary): uses last API response's `usage` (input_tokens + cache_creation + cache_read + output), then adds `roughTokenCountEstimationForMessages` for messages added since. Handles parallel tool call splits by walking back to first sibling with same `message.id`. `roughTokenCountEstimation` uses `bytesPerToken=4` default. `estimateMessageTokens` in microCompact applies `4/3` padding (`tokens.ts:241-276`, `microCompact.ts:164-205`)
- **Go版**: `EstimateTokens` (line 37) uses `math.Ceil(len(text) / 4)`. `EstimateContentTokens` (line 51) uses content-type-specific ratios. `estimateMessageParamsTokens` (line 1177) applies `4/3` padding. No API-usage-based estimation — purely heuristic. No parallel tool call split handling
- **类型**: 简化

### 1.19 Token Estimation — Thinking/Redacted Thinking Blocks
- **上游**: `estimateMessageTokens` counts `thinking` text, `redacted_thinking` data, `tool_use` name+input separately. Images/documents get `IMAGE_MAX_TOKEN_SIZE=2000` each (`microCompact.ts:138-205`)
- **Go版**: `estimateMessageParamsTokens` (line 1177) and `estimateSingleMessageTokens` (line 1949) handle `OfText`, `OfToolUse`, `OfToolResult` only. No thinking/redacted_thinking block estimation. No image/document token estimation
- **类型**: 缺失

### 1.20 Context Window Tracking — Effective Window Calculation
- **上游**: `getEffectiveContextWindowSize` reserves `MAX_OUTPUT_TOKENS_FOR_SUMMARY=20_000` from context window; `getAutoCompactThreshold` subtracts `AUTOCOMPACT_BUFFER_TOKENS=13_000` from effective window; `getContextWindowForModel` uses model-specific sizes; env override `CLAUDE_CODE_AUTO_COMPACT_WINDOW` (`autoCompact.ts:33-49, 72-91`)
- **Go版**: `ContextWindowTracker.EffectiveWindow` (line 1155) reserves 20K for output. `CompactThreshold` (line 1332) applies `compactThreshold * maxTokens`. `modelContextWindow` (line 1138) supports `[1m]` suffix and Sonnet-4/Opus-4 detection for 1M context, defaults to 200K. No env override for auto-compact window
- **类型**: Go适配

### 1.21 Auto-compact — Circuit Breaker
- **上游**: `MAX_CONSECUTIVE_AUTOCOMPACT_FAILURES=3` — after 3 consecutive failures, stops retrying. `AutoCompactTrackingState` tracks `consecutiveFailures`, `compacted`, `turnCounter`, `turnId` (`autoCompact.ts:60-70, 258-265`)
- **Go版**: `Compactor.maxLLMCompactFailures=3` (line 1300) — after 3 failures, permanently disables LLM auto-compact (`c.disabled = true`). Other compaction paths (SM-compact, PartialCompact, truncation) remain available but LLM path is permanently disabled, not just circuit-broken per-session
- **类型**: Go适配

### 1.22 Auto-compact — Feature Flag Gating
- **上游**: Multiple feature gates: `DISABLE_COMPACT`, `DISABLE_AUTO_COMPACT`, `autoCompactEnabled` in user config, `tengu_cobalt_raccoon` (reactive-only mode suppresses proactive), `CONTEXT_COLLAPSE` feature suppresses autocompact when collapse is active, `marble_origami` querySource exclusion (`autoCompact.ts:147-223`)
- **Go版**: `Compactor.disabled` flag only (set after 3 failures). No `DISABLE_AUTO_COMPACT`, no user config, no reactive-only mode, no context-collapse suppression, no querySource exclusion
- **类型**: 缺失

### 1.23 Auto-compact — Recompaction Info / Turn Tracking
- **上游**: `RecompactionInfo` tracks `isRecompactionInChain`, `turnsSincePreviousCompact`, `previousCompactTurnId`, `autoCompactThreshold`, `querySource`. Used in `tengu_compact` analytics event to disambiguate loops (`compact.ts:320-327`, `autoCompact.ts:279-285`)
- **Go版**: No recompaction info or turn tracking
- **类型**: 缺失

### 1.24 Auto-compact — shouldAutoCompact snipTokensFreed
- **上游**: `shouldAutoCompact(messages, model, querySource, snipTokensFreed)` subtracts `snipTokensFreed` from token count — snip removes messages but surviving assistant's usage still reflects pre-snip context, so this compensates (`autoCompact.ts:160-239`)
- **Go版**: No snip compensation in `ShouldCompact`
- **类型**: 缺失

### 1.25 Anti-Thrashing — Savings-Ratio vs None
- **上游**: No explicit anti-thrashing in autoCompact — relies on threshold + buffer tokens to prevent re-triggering. `truePostCompactTokenCount` and `willRetriggerNextTurn` in analytics help detect thrashing patterns post-hoc
- **Go版**: `Compactor.ShouldCompact` (line 1314) has cooldown: skip if tokens haven't grown 25% since last compaction. `Compactor.Compact` (line 1366) has savings-ratio anti-thrashing: skip if last 2 compactions each saved <10%. This is a Go-original enhancement not present upstream
- **类型**: Go增强

### 1.26 Post-compact Recovery — File Attachments
- **上游**: `createPostCompactFileAttachments` re-reads up to 5 recently-accessed files with `POST_COMPACT_TOKEN_BUDGET=50_000` and `POST_COMPACT_MAX_TOKENS_PER_FILE=5_000`. Skips files already in preserved messages via `collectReadToolFilePaths` (dedup). Skips `FILE_UNCHANGED_STUB` results. Excludes plan files and claude.md files (`compact.ts:1421-1470`)
- **Go版**: No post-compact file re-read/attachment mechanism
- **类型**: 缺失

### 1.27 Post-compact Recovery — Plan Attachment
- **上游**: `createPlanAttachmentIfNeeded` reads plan file content and includes as `plan_file_reference` attachment. Preserves plan mode via `createPlanModeAttachmentIfNeeded` (`compact.ts:1476-1566`)
- **Go版**: No plan attachment preservation
- **类型**: 缺失

### 1.28 Post-compact Recovery — Skill Attachment
- **上游**: `createSkillAttachmentIfNeeded` includes invoked skills sorted by recency, per-skill truncation to `POST_COMPACT_MAX_TOKENS_PER_SKILL=5_000`, budget `POST_COMPACT_SKILLS_TOKEN_BUDGET=25_000` (`compact.ts:1500-1540`)
- **Go版**: No skill attachment preservation
- **类型**: 缺失

### 1.29 Post-compact Recovery — Agent Task Status
- **上游**: `createAsyncAgentAttachmentsIfNeeded` includes running/completed async agent tasks as `task_status` attachments so model knows about background work (`compact.ts:1574-1605`)
- **Go版**: No async agent attachment
- **类型**: 缺失

### 1.30 Post-compact Recovery — Delta Tools/Agents/MCP Re-declaration
- **上游**: After compaction, re-announces: `getDeferredToolsDeltaAttachment(tools, model, [], {callSite: 'compact_full'})` for tools, `getAgentListingDeltaAttachment(context, [])` for agents, `getMcpInstructionsDeltaAttachment(mcpClients, tools, model, [])` for MCP. Empty history → diff against nothing → full announcement (`compact.ts:571-589`)
- **Go版**: No delta re-declaration after compaction
- **类型**: 缺失

### 1.31 Post-compact Recovery — preCompactDiscoveredTools
- **上游**: `extractDiscoveredToolNames(messages)` finds tool_reference blocks in messages being compacted; stores in `boundaryMarker.compactMetadata.preCompactDiscoveredTools` so post-compact schema filter keeps sending those tool schemas (`compact.ts:610-615`)
- **Go版**: No tool discovery state preservation across compaction
- **类型**: 缺失

### 1.32 Post-compact Cleanup — runPostCompactCleanup
- **上游**: `runPostCompactCleanup(querySource)` resets: microcompact state, context collapse state (main-thread only), `getUserContext` cache, `getMemoryFiles` cache, system prompt sections, classifier approvals, speculative checks, beta tracing state, commit attribution file cache, session messages cache. Intentionally NOT resetting `sentSkillNames` (`postCompactCleanup.ts:1-78`)
- **Go版**: Stub no-op functions: `clearSpeculativeChecks` (line 2873), `clearBetaTracingState` (line 2879), `clearSessionMessagesCache` (line 2885), `sweepFileContentCache` (line 2891). No context-collapse reset, no memory-files cache reset, no classifier-approval reset, no system-prompt-sections reset, no getUserContext cache reset
- **类型**: 缺失

### 1.33 Hooks — Pre-compact and Post-compact
- **上游**: `executePreCompactHooks({trigger, customInstructions})` can inject `newCustomInstructions` and `userDisplayMessage`. `executePostCompactHooks({trigger, compactSummary})` can provide `userDisplayMessage`. `processSessionStartHooks('compact', {model})` runs after compaction for CLAUDE.md re-injection (`compact.ts:416-428, 596-598, 727-733`)
- **Go版**: No pre/post-compact hooks. No session-start hooks after compaction
- **类型**: 缺失

### 1.34 Compaction Result — CompactionResult Structure
- **上游**: `CompactionResult` includes: `boundaryMarker` (SystemCompactBoundaryMessage), `summaryMessages` (UserMessage[]), `attachments` (AttachmentMessage[]), `hookResults` (HookResultMessage[]), `messagesToKeep?`, `userDisplayMessage?`, `preCompactTokenCount?`, `postCompactTokenCount?`, `truePostCompactTokenCount?`, `compactionUsage?` (`compact.ts:303-314`)
- **Go版**: `CompactionResult` (line 122) includes: `Messages`, `OmittedCount`, `KeptCount`, `TokensBefore`, `TokensAfter`, `TokensSaved`, `CompactionRatio`, `ArchivePath`, `CompactionTrigger`. `CompactionResultLLM` (line 1210) includes: `BoundaryText`, `SummaryText`, `PreCompactTokens`, `PostCompactTokens`. No attachments, hook results, boundary markers, usage metrics
- **类型**: 简化

### 1.35 Compact Boundary — Relink Metadata / Preserved Segment
- **上游**: `annotateBoundaryWithPreservedSegment` adds `compactMetadata.preservedSegment` with `headUuid`, `anchorUuid`, `tailUuid` for disk-chain relinking after compaction. Essential for session storage to maintain parent-child links (`compact.ts:353-371`)
- **Go版**: No preserved-segment relink metadata on boundary markers
- **类型**: 缺失

### 1.36 Compact Boundary — createCompactBoundaryMessage
- **上游**: `createCompactBoundaryMessage(trigger, preCompactTokenCount, lastMessageUuid, userFeedback?, messagesSummarized?)` creates a structured `SystemCompactBoundaryMessage` with `compactMetadata` including trigger, token count, timestamps, discovered tools, preserved segment (`compact.ts:602-615`)
- **Go版**: `buildCompactBoundaryText` (line 1635) creates a simple text string `"[Previous conversation summary (%d tokens compressed)]"`. No structured metadata, no UUID tracking, no discovered tools
- **类型**: 简化

### 1.37 PTL Retry — truncateHeadForPTLRetry
- **上游**: `truncateHeadForPTLRetry` groups messages by API round (`groupMessagesByApiRound`), calculates token gap from error response, drops oldest groups to cover gap. 20% fallback for unparseable gaps. Prepends synthetic `PTL_RETRY_MARKER` user message if first remaining message is assistant (`compact.ts:247-295`)
- **Go版**: `compactConversationLLM` PTL loop (line 1418) groups by round, drops oldest rounds. Uses `dropFraction` based on parsed token gap or 20% fallback. No synthetic PTL_RETRY_MARKER prepended. Never drops more than half
- **类型**: Go适配

### 1.38 PTL Retry — API-round Grouping Algorithm
- **上游**: `groupMessagesByApiRound` (grouping.ts) uses `message.id` boundary: new group when assistant message has different `id` from previous assistant. Streaming chunks with same id stay in one group. Handles interleaved tool_results between streaming chunks (`grouping.ts:22-63`)
- **Go版**: `groupMessagesByRound` (line 174) uses role-based grouping: new group on "user" role. `groupMessageParamsByRound` (line 842) same pattern. No `message.id` boundary detection
- **类型**: 简化

### 1.39 Selective Compact — Tool-Level Filtering
- **上游**: No standalone "selective compact" — micro-compact handles tool-result clearing via `COMPACTABLE_TOOLS` set (Read, Shell, Grep, Glob, WebSearch, WebFetch, Edit, Write). Only clears tool results, not entire rounds (`microCompact.ts:41-50`)
- **Go版**: `SelectiveCompact` (line 559) operates at round level, replacing entire message content with placeholder for rounds using compactable tools. `defaultCompactableTools` (line 823) includes: read_file, glob, grep, list_dir, web_fetch, web_search. Different tool names and different granularity (round-level vs tool-result-level)
- **类型**: Go增强

### 1.40 Smart Compact — Turn-Based Head+Tail Preservation
- **上游**: No equivalent — upstream uses LLM summary or SM-compact, not head+tail preservation with omission marker
- **Go版**: `SmartCompact` (line 608) groups messages into conversational turns, keeps first N and last M turns, collapses middle with `OmissionMarker`. This is a Go-original lightweight compaction strategy
- **类型**: Go增强

### 1.41 FNV-1a Dedup — Tool Result Deduplication
- **上游**: No FNV-1a dedup. Cached micro-compact uses `delete_tool_result` cache edits to remove old tool results server-side. No content-based deduplication
- **Go版**: `dedupToolResults` (line 1769) hashes tool result content with FNV-1a (32-bit), replaces duplicate results with `[duplicate result, see tool_use_id XXX]` reference. Skips error results. This is a Go-original pre-pruning step
- **类型**: Go增强

### 1.42 Pre-pruning — Truncate Large Tool Args
- **上游**: No tool-use argument truncation before compaction
- **Go版**: `truncateLargeToolArgs` (line 1857) truncates string values in tool_use input maps to `maxArgChars` (2000). Go-original pre-pruning step
- **类型**: Go增强

### 1.43 Pre-pruning — Sensitive Info Redaction
- **上游**: No sensitive info redaction before compaction
- **Go版**: `redactSensitiveText` (line 2601) redacts patterns matching `api_key`, `password`, `secret`, `token`, `credential`, `auth`, `private_key`, `access_key` with `[REDACTED]`. Pre-compiled regex patterns. Go-original security enhancement
- **类型**: Go增强

### 1.44 Context Management — Server-Side context_management API
- **上游**: No `context_management` edits in compact API call. Relies on `cache_edits` (via cached micro-compact) for server-side tool result deletion. No `clear_tool_uses_20250919` or `clear_thinking_20251015`
- **Go版**: `doCompactLLMCall` (line 1606) includes `context_management` with `clear_tool_uses_20250919` + `clear_tool_inputs: true` and `clear_thinking_20251015` in the API request. This is a newer API feature not yet adopted upstream
- **类型**: Go增强

### 1.45 Iterative Summary — Previous Summary Update
- **上游**: No iterative summary — each compaction generates a fresh summary from the full conversation. Previous summaries are not fed back as input
- **Go版**: `iterativeCompactPrompt` (line 1098) accepts `{previous_summary}` placeholder. `Compactor.lastSummary` (line 1288) stores previous summary. `compactConversationLLM` (line 1376) passes `c.lastSummary` to enable iterative updates. Go-original optimization to reduce compaction token usage
- **类型**: Go增强

### 1.46 Reactive Compact — Threshold-Based Token Spike Detection
- **上游**: `reactiveCompact.ts` is a stub (auto-generated). `tryReactiveCompact`, `reactiveCompactOnPromptTooLong`, `isReactiveOnlyMode` all return no-op
- **Go版**: `CheckReactiveCompact` (line 2556) compares current vs previous token count; if delta exceeds threshold (default 5000), returns `ReactiveCompactResult` with trigger info. Functional but simple implementation
- **类型**: Go增强

### 1.47 Snip Compact — /force-snip Command
- **上游**: `/force-snip` command (`commands/force-snip.ts`) inserts a `snip_boundary` system message with `snipMetadata.removedUuids`. `snipCompactIfNeeded` processes boundaries on next query cycle. `snipCompact.ts` is a stub in external builds (`force-snip.ts:1-59`)
- **Go版**: No snip/force-snip mechanism
- **类型**: 缺失

### 1.48 Context Collapse — Granular Context Management
- **上游**: `contextCollapse/index.ts` provides `applyCollapsesIfNeeded`, `recoverFromOverflow`, `isWithheldPromptTooLong`. 90% commit / 95% blocking-spawn flow. Suppresses autocompact when active. External build is a stub (`contextCollapse/index.ts:1-76`)
- **Go版**: No context collapse mechanism
- **类型**: 缺失

### 1.49 Compaction Analytics — tengu_compact Event
- **上游**: `tengu_compact` event includes: preCompactTokenCount, postCompactTokenCount, truePostCompactTokenCount, autoCompactThreshold, willRetriggerNextTurn, isAutoCompact, querySource, queryChainId, queryDepth, isRecompactionInChain, turnsSincePreviousCompact, previousCompactTurnId, compactionInputTokens, compactionOutputTokens, compactionCacheReadTokens, compactionCacheCreationTokens, compactionTotalTokens, promptCacheSharingEnabled, plus full analyzeContext breakdown (`compact.ts:654-699`)
- **Go版**: `fmt.Fprintf(os.Stderr, ...)` prints basic stats (message count, tokens saved). No analytics events
- **类型**: 缺失

### 1.50 buildPostCompactMessages — Consistent Ordering
- **上游**: `buildPostCompactMessages` ensures consistent message order: boundaryMarker → summaryMessages → messagesToKeep → attachments → hookResults (`compact.ts:334-342`)
- **Go版**: No consistent post-compact message builder. Each path builds its own message list
- **类型**: 缺失

### 1.51 markPostCompaction — Bootstrap State
- **上游**: `markPostCompaction()` sets bootstrap state flag used by other modules. `reAppendSessionMetadata` preserves custom title/tag for `--resume` display. `writeSessionTranscriptSegment` writes pre-compact transcript for KAIROS feature (`compact.ts:708-721`)
- **Go版**: No post-compaction state marking or session metadata re-appending
- **类型**: 缺失

### 1.52 Error Handling — addErrorNotificationIfNeeded
- **上游**: Shows error notification for manual /compact failures, suppresses for auto-compact (confusing when eventual success follows). Differentiates `ERROR_MESSAGE_USER_ABORT` and `ERROR_MESSAGE_NOT_ENOUGH_MESSAGES` (`compact.ts:1112-1127`)
- **Go版**: `fmt.Fprintf(os.Stderr, ...)` for all errors. No notification system, no auto/manual differentiation
- **类型**: Go适配

### 1.53 Compaction — Compact Can-Use-Tool Denial
- **上游**: `createCompactCanUseTool()` returns deny-all function — compact fork should never use tools. Applied to forked agent path (`compact.ts:1129-1138`)
- **Go版**: Compact API call has no tools in request (only `MessageNewParams`). Implicit denial via absence of tools
- **类型**: Go适配

### 1.54 Archive — Disk Persistence
- **上游**: No archive-to-disk of omitted messages. Session transcript written separately for KAIROS feature
- **Go版**: `archiveRounds` (line 469) writes omitted rounds to timestamped JSON files in `ArchiveDir`. `LoadArchive`/`ListArchives` (lines 504-542) for reading archived context. Go-original feature for context recovery
- **类型**: Go增强

### 1.55 Role Alternation Fixing
- **上游**: `normalizeMessagesForAPI` in `utils/messages.ts` handles role alternation for API compliance as part of message normalization before every API call, not specifically compaction-related
- **Go版**: `ConversationContext.FixRoleAlternation()` is called after `PartialCompact` (line 2151) to ensure user/assistant alternation in the rebuilt message list
- **类型**: Go适配

---


---

### 15.7 Prompt Caching — Boundary Splitting vs Upstream

- **上游**: `prompts.ts:108-117` uses `SYSTEM_PROMPT_DYNAMIC_BOUNDARY = '__SYSTEM_PROMPT_DYNAMIC_BOUNDARY__'` as a string marker injected into the system prompt array. The boundary is conditionally included only when `shouldUseGlobalCacheScope()` is true (`prompts.ts:662-663`). Content BEFORE the boundary gets `scope: 'global'`, content AFTER gets standard ephemeral.
- **Go版**: `system_prompt.go:21` uses `SYSTEM_PROMPT_STATIC_BOUNDARY = "<!-- STATIC_PROMPT_END -->"` — an HTML comment marker. `prompt_caching.go:180-218` splits at this boundary with `FormatBoundaryCachedSystemPrompt()`. The Go version ALWAYS injects the boundary (no conditional gating).
- **类型**: 简化


---

### 15.8 Prompt Caching — Global Cache Scope Conditionality

- **上游**: `claude.ts:1230-1253` has sophisticated `globalCacheStrategy` logic: `'system_prompt'` (when no MCP tools rendered), `'tool_based'` (when MCP tools present — can't globally cache since tools are per-user), or `'none'`. Also checks `needsToolBasedCacheMarker` which gates on `filteredTools.some(t => t.isMcp === true && !willDefer(t))`.
- **Go版**: `prompt_caching.go:191` always applies global marker to static content, with no MCP tool detection or conditional strategy switching.
- **类型**: 简化


---

### 15.9 Prompt Caching — Cache Break Detection

- **上游**: `promptCacheBreakDetection.ts` (728 lines) implements a two-phase detection system: (1) `recordPromptState()` hashes system/tool state pre-call, tracking 13 change categories (systemPromptChanged, toolSchemasChanged, modelChanged, fastModeChanged, cacheControlChanged, globalCacheStrategyChanged, betasChanged, autoModeActive, overageChanged, cachedMCChanged, effortChanged, extraBodyChanged, per-tool schema hashes). (2) `checkResponseForCacheBreak()` compares cache_read_tokens drop >5% against previous call and correlates with pending changes. Also detects TTL expiration (5min vs 1h) based on time gaps. Writes `.diff` files for debugging.
- **Go版**: No cache break detection at all.
- **类型**: 缺失


---

## PostCompactRecovery System — Deep Comparison

**Go source**: `agent_loop.go:2814–3615`, `context.go:48–74`, `compact.go:871–877`, `hooks.go:17–40`, `session_memory.go:338–460`
**Upstream source**: `services/compact/compact.ts:126–1711`, `services/compact/postCompactCleanup.ts:1–77`, `services/compact/sessionMemoryCompact.ts:1–600`, `services/compact/autoCompact.ts:1–351`, `services/compact/prompt.ts:1–372`, `utils/attachments.ts:1472–1602,3046–3204`, `utils/messages.ts:4609–4634`, `utils/conversationRecovery.ts:384–406`, `utils/memory/types.ts:1–12`, `services/SessionMemory/prompts.ts:256–296`

---

### 1. File Attachment Recovery

| Aspect | Upstream (TypeScript) | Go |
|---|---|---|
| **Entry point** | `createPostCompactFileAttachments()` (`compact.ts:1421`) | `PostCompactRecovery()` file section (`agent_loop.go:2829–2919`) |
| **Max files** | `POST_COMPACT_MAX_FILES_TO_RESTORE = 5` (`compact.ts:126`) | `PostCompactMaxFiles` config, default 5 (`agent_loop.go:2831`) |
| **Total token budget** | `POST_COMPACT_TOKEN_BUDGET = 50_000` (`compact.ts:127`) | `PostCompactMaxFileTokens` config, default 12,500 (50K chars / 4) (`agent_loop.go:2835–2843`) |
| **Per-file token cap** | `POST_COMPACT_MAX_TOKENS_PER_FILE = 5_000` (`compact.ts:128`) | `PostCompactMaxTokensPerFile` config, default 5,000 (`agent_loop.go:2846`) |
| **Source of file content** | `readFileState` cache (`preCompactReadFileState = cacheToObject(context.readFileState)` at `compact.ts:522`), then `generateFileAttachment()` (`attachments.ts:3046`) which uses `existingFileState.content` for already-read files, or `FileReadTool.call()` for disk reads | Registry's `GetCachedFileContent()` (cached content the model actually saw), falling back to `os.ReadFile()` for disk re-read (`agent_loop.go:2880–2889`) |
| **Staleness check** | `generateFileAttachment()` compares `existingFileState.timestamp` against `mtimeMs` from disk; if file was modified since read, returns `compact_file_reference` (path-only) instead of full content (`attachments.ts:3109–3141`) | **MISSING** — no mtime/staleness check. Uses cached content blindly, or reads current disk content as fallback. No detection of file changes between read and compact. |
| **Sort order** | By `timestamp` descending (most recently read first) (`compact.ts:1437`) | By `GetRecentlyReadFiles()` order (assumed recency-based) (`agent_loop.go:2855`) |
| **Token budget enforcement** | Post-hoc: generates all attachments, then filters by `roughTokenCountEstimation(jsonStringify(result))` against `POST_COMPACT_TOKEN_BUDGET` (`compact.ts:1458–1469`) | Pre-emptive: estimates tokens per file with `EstimateTokens(content)`, breaks loop when `totalTokens+contentTokens > maxFileTokens` (`agent_loop.go:2901–2903`) |
| **Attachment format** | Structured `AttachmentMessage<Attachment>` with typed sub-types (`file`, `compact_file_reference`, `already_read_file`) | Plain text string: `[Post-compact file recovery: <path>]\n<content>` (`agent_loop.go:2906`) |
| **Preserved-path dedup** | `collectReadToolFilePaths(preservedMessages)` returns `Set<string>` of `expandPath()`-normalized paths (`compact.ts:1616–1661`) | `collectReadToolFilePaths(ctx)` returns `map[string]bool` of raw paths (`agent_loop.go:2611–2677`) |
| **File-read dedup (stubs)** | Scans for `FILE_UNCHANGED_STUB`-prefixed tool_results, collects their `tool_use_id`s, then skips tool_use blocks with those IDs (`compact.ts:1622–1631`) | Same logic: identifies stub tool_use_ids, skips them when collecting file paths (`agent_loop.go:2629–2649`) |
| **Feature gate** | Always runs (no gate) | Gated by `PostCompactRecoverFiles` config (`agent_loop.go:2829`) |
| **Re-mark as read** | Not explicitly re-marked (readFileState is cleared but attachments carry content) | `a.registry.MarkFileRead(path)` after each recovered file (`agent_loop.go:2913`) |

**Key differences**:
- Upstream does staleness detection (mtime check) and falls back to a lightweight `compact_file_reference` attachment when files have changed. Go always injects full content regardless of staleness.
- Upstream token-budgets on the *serialized message* (`jsonStringify(result)`), Go budgets on *content alone* (`EstimateTokens(content)`). Upstream's approach accounts for JSON overhead; Go's may underestimate.
- Upstream uses structured typed attachments (AttachmentMessage); Go uses plain text strings injected via `AddAttachment()`.

---

### 2. Skill Content Recovery

| Aspect | Upstream (TypeScript) | Go |
|---|---|---|
| **Entry point** | `createSkillAttachmentIfNeeded()` (`compact.ts:1500`) | Skill recovery section in `PostCompactRecovery()` (`agent_loop.go:2921–2974`) |
| **Source** | `getInvokedSkillsForAgent(agentId)` — per-agent scoped (`compact.ts:1503`) | `a.skillTracker.GetReadSkillNames()` + `a.config.SkillLoader.LoadSkill(name)` (`agent_loop.go:2941,2946`) |
| **Per-skill token cap** | `POST_COMPACT_MAX_TOKENS_PER_SKILL = 5_000` (`compact.ts:133`) | `PostCompactMaxSkillTokens` config, default 1,250 (5K chars / 4) (`agent_loop.go:2924–2929`) |
| **Total skill budget** | `POST_COMPACT_SKILLS_TOKEN_BUDGET = 25_000` (`compact.ts:134`) | `PostCompactMaxTotalSkillTokens` config, default 6,250 (25K chars / 4) (`agent_loop.go:2932–2938`) |
| **Sort order** | Most-recently-invoked first (`sort((a, b) => b.invokedAt - a.invokedAt)`) (`compact.ts:1513`) | `GetReadSkillNames()` order (undefined sort) |
| **Truncation** | `truncateToTokens(content, POST_COMPACT_MAX_TOKENS_PER_SKILL)` — char budget = `maxTokens * 4 - SKILL_TRUNCATION_MARKER.length`, preserves head (`compact.ts:1518,1672–1678`) | Per-skill: `charLimit = maxSkillTokens * 4`, truncates at char boundary with marker (`agent_loop.go:2954–2958`) |
| **Truncation marker** | `'\n\n[... skill content truncated for compaction; use Read on the skill path if you need the full text]'` (`compact.ts:1663`) | Same: `'\n\n[... skill content truncated for compaction; use Read on the skill path if you need the full text]'` (`agent_loop.go:2956`) |
| **Attachment format** | Structured `{ type: 'invoked_skills', skills: [...] }` (`compact.ts:1536–1539`) | Plain text: `[Post-compact skill recovery: <name>]\n<content>` (`agent_loop.go:2965`) |
| **Agent scoping** | Per-agent: `getInvokedSkillsForAgent(agentId)` filters by `agentId` (`compact.ts:1503`) | **MISSING** — no agent scoping; all read skills are re-injected regardless of agent context |
| **Resume recovery** | `restoreSkillStateFromMessages()` walks transcript for `invoked_skills` attachments and re-registers them via `addInvokedSkill()` (`conversationRecovery.ts:384–406`) | **MISSING** — no equivalent resume recovery for skill state |

**Key differences**:
- Upstream scopes skills per-agent (subagents only see their own invoked skills). Go injects all skills for all agents.
- Upstream sorts by `invokedAt` (most recent first) for budget pressure. Go has no explicit sort.
- Upstream has `restoreSkillStateFromMessages()` for session resume; Go does not persist or restore skill state across sessions.
- Upstream's `truncateToTokens` subtracts marker length from char budget to stay within token limit; Go does not subtract marker length.

---

### 3. Plan File Recovery

| Aspect | Upstream (TypeScript) | Go |
|---|---|---|
| **Entry point** | `createPlanAttachmentIfNeeded(agentId)` (`compact.ts:1476`) | `buildPostCompactPlanAttachment(projectDir)` (`agent_loop.go:3280`) |
| **Plan content source** | `getPlan(agentId)` — centralized plan getter (`compact.ts:1479`) | Scans `.claude/plan/` directory for most recently modified `.md` file (`agent_loop.go:3281–3324`) |
| **Plan file path** | `getPlanFilePath(agentId)` — centralized path getter (`compact.ts:1485`) | Manually computed: `filepath.Join(projectDir, ".claude", "plan")` (`agent_loop.go:3281`) |
| **Agent scoping** | Yes — `agentId` parameter selects per-agent plan (`compact.ts:1478`) | **MISSING** — no agent scoping; always uses project-level plan |
| **Attachment format** | Structured `{ type: 'plan_file_reference', planFilePath, planContent }` (`compact.ts:1487–1491`) | Plain text string: `"A plan file exists from plan mode at: <path>\n\nPlan contents:\n\n<content>\n\nIf this plan is relevant..."` (`agent_loop.go:3324`) |
| **When injected** | In `compactConversation()` and `partialCompactConversation()` — after file/agent attachments, before delta tools (`compact.ts:549,943`) | In `PostCompactRecovery()` — after file/skill recovery, before tools/MCP (`agent_loop.go:2979–2983`) |
| **SM-compact** | Yes — `createPlanAttachmentIfNeeded(agentId)` at `sessionMemoryCompact.ts:484` | Yes — same `buildPostCompactPlanAttachment()` call in `PostCompactRecovery()` |

**Key differences**:
- Upstream uses centralized `getPlan(agentId)` / `getPlanFilePath(agentId)` for per-agent plan support. Go hardcodes directory scanning.
- Upstream returns structured attachment; Go returns plain text with instructional prompt.
- Upstream's plan path is agent-scoped; Go's is always project-level.

---

### 4. Plan Mode Recovery

| Aspect | Upstream (TypeScript) | Go |
|---|---|---|
| **Entry point** | `createPlanModeAttachmentIfNeeded(context)` (`compact.ts:1548`) | Plan mode section in `PostCompactRecovery()` (`agent_loop.go:2985–2990`) |
| **Check** | `appState.toolPermissionContext.mode !== 'plan'` (`compact.ts:1552`) | `a.config.PermissionMode == ModePlan` (`agent_loop.go:2987`) |
| **Attachment format** | Structured `{ type: 'plan_mode', reminderType: 'full', isSubAgent, planFilePath, planExists }` (`compact.ts:1559–1565`) | Plain text: `"## Plan Mode Active\n\nYou are in plan mode..."` (`agent_loop.go:2988`) |
| **Sub-agent awareness** | Yes — `isSubAgent: !!context.agentId` (`compact.ts:1563`) | **MISSING** — no sub-agent distinction |
| **Plan file reference** | Yes — `planFilePath` and `planExists` included in attachment (`compact.ts:1563–1564`) | **MISSING** — plan mode attachment does not reference plan file |
| **Async** | `async` function (`compact.ts:1548`) | Synchronous |

**Key differences**:
- Upstream includes `isSubAgent` flag and `planFilePath`/`planExists` metadata in the structured attachment. Go uses a static text string without these details.
- Upstream is async (reads app state); Go checks a synchronous config field.

---

### 5. Tool Re-declaration (Delta Tools)

| Aspect | Upstream (TypeScript) | Go |
|---|---|---|
| **Entry point** | `getDeferredToolsDeltaAttachment(tools, model, messages, scanContext)` (`attachments.ts:1472`) | `buildPostCompactToolsAnnouncement(preservedToolNames)` (`agent_loop.go:3220`) |
| **Mechanism** | Differential: scans `messages` for prior `deferred_tools_delta` attachments, computes diff of current tools vs already-announced tools. Only announces the delta. (`attachments.ts:1489`) | Delta-based but coarser: collects tool names from preserved message tail via `collectUsedToolNamesInPreservedMessages()`, then lists all tools NOT in the preserved set. (`agent_loop.go:2997–3007`) |
| **Feature gate** | `isDeferredToolsDeltaEnabled()` + `isToolSearchEnabledOptimistic()` + `modelSupportsToolReference(model)` + `isToolSearchToolAvailable(tools)` — 4 gate checks (`attachments.ts:1478–1488`) | **MISSING** — no feature gates; always announces tools when `a.registry != nil` |
| **Compact-specific** | Passes `scanContext: { callSite: 'compact_full' }` or `'compact_partial'` for analytics (`compact.ts:575,965`) | **MISSING** — no call-site analytics |
| **Full compact** | Passes `[]` (empty messages) → diff against nothing → announces full set (`compact.ts:571–578`) | Collects `preservedToolNames` from entries after boundary → skips those already visible (`agent_loop.go:2998–2999`) |
| **Partial compact** | Passes `messagesToKeep` → diff against those → only announces tools in summarized portion (`compact.ts:961–968`) | **PARTIAL** — `preservedToolNames` uses boundary-based scanning, not partial-compact-aware |
| **Attachment format** | Structured `{ type: 'deferred_tools_delta', ...delta }` with typed fields | Plain text: `"## Tools Available After Compaction\n\n..."` with markdown list (`agent_loop.go:3222–3275`) |
| **Tool categorization** | Single delta attachment with added/removed tools | Categorizes into Core/MCP/Skill tools with markdown headers (`agent_loop.go:3238–3268`) |

**Key differences**:
- Upstream's delta mechanism is precise: it reconstructs the announced set from prior `deferred_tools_delta` attachments in the transcript, computing a true diff. Go's approach only considers whether a tool *name* appeared in the preserved tail's tool_use blocks — this is a coarser heuristic that misses tools announced but not yet invoked.
- Upstream has 4 feature gates; Go has none.
- Upstream tracks added/removed separately; Go just lists available tools.
- Upstream passes call-site context for analytics; Go does not.

---

### 6. MCP Re-declaration (Delta MCP)

| Aspect | Upstream (TypeScript) | Go |
|---|---|---|
| **Entry point** | `getMcpInstructionsDeltaAttachment(mcpClients, tools, model, messages)` (`attachments.ts:1576`) | `buildPostCompactMCPAnnouncement(preservedToolNames)` (`agent_loop.go:3331`) |
| **Mechanism** | Differential: reconstructs announced MCP servers from prior `mcp_instructions_delta` attachments, computes diff. Includes Chrome ToolSearch client-side instructions. (`attachments.ts:1587–1601`) | Delta-based: checks if any tools from a server appear in `preservedToolNames`, skips those servers. (`agent_loop.go:3357–3370`) |
| **Feature gate** | `isMcpInstructionsDeltaEnabled()` (`attachments.ts:1582`) | **MISSING** — no feature gate |
| **Chrome MCP** | Synthesizes `CLAUDE_IN_CHROME_MCP_SERVER_NAME` with `CHROME_TOOL_SEARCH_INSTRUCTIONS` as client-side instruction (`attachments.ts:1593–1596`) | **MISSING** — no Chrome MCP concept |
| **Full compact** | Passes `[]` → announces all MCP servers (`compact.ts:582–589`) | Announces servers whose tools are NOT in preserved tail (`agent_loop.go:3331–3401`) |
| **Partial compact** | Passes `messagesToKeep` → diff against those (`compact.ts:972–979`) | Same boundary-based scanning |
| **Attachment format** | Structured `{ type: 'mcp_instructions_delta', ...delta }` | Plain text: `"## MCP Servers After Compaction\n\n..."` with server status and tool lists (`agent_loop.go:3377–3401`) |
| **Server instructions** | Included via delta mechanism | Included via `mgr.AllServerInstructions()` — per-server usage instructions injected as text (`agent_loop.go:3395–3398`) |

**Key differences**:
- Upstream reconstructs the announced set from prior delta attachments for true diff; Go uses tool-name-presence heuristic.
- Upstream includes Chrome MCP virtual server; Go does not.
- Upstream has feature gate; Go does not.
- Both inject per-server instructions.

---

### 7. Agent Listing Recovery

| Aspect | Upstream (TypeScript) | Go |
|---|---|---|
| **Entry point** | `createAsyncAgentAttachmentsIfNeeded(context)` (`compact.ts:1574`) for running agents; `getAgentListingDeltaAttachment(toolUseContext, messages)` (`attachments.ts:1507`) for agent type listing | `InjectRunningAgentStatus()` (`agent_loop.go:3818`) for running agents; `buildPostCompactAgentAnnouncement(preservedToolNames)` (`agent_loop.go:3567`) for post-compact announcement |
| **Running agents** | Filters `appState.tasks` for `type === 'local_agent'`, excludes `retrieved`, `pending`, and `context.agentId` (self). Returns structured `{ type: 'task_status', taskId, taskType, description, status, deltaSummary, outputFilePath }` (`compact.ts:1578–1604`) | Lists tasks from `agentTaskStore.List()` filtered by `TaskRunning`. Returns plain text: `[task_status] taskId: <id>, type: local_agent, description: <desc>, status: running` (`agent_loop.go:3822–3829`) |
| **Completed unretrieved** | Yes — agents with `status !== 'running'` but `retrieved === false` are included with `deltaSummary` (error or progress) and `outputFilePath` (`compact.ts:1578–1604`) | Yes — `completedUnretrieved` tracked separately in `buildPostCompactAgentAnnouncement()` with output file info (`agent_loop.go:3575–3583,3601–3614`) |
| **Agent type listing** | `getAgentListingDeltaAttachment()` — reconstructs announced agent types from prior `agent_listing_delta` attachments, computes added/removed diff (`attachments.ts:1507–1573`) | **MISSING** — no agent type listing/delta mechanism |
| **Attachment format** | Structured `{ type: 'task_status', ... }` and `{ type: 'agent_listing_delta', ... }` | Plain text strings |
| **Delta mechanism** | `agent_listing_delta`: reconstructs from prior attachments, computes addedTypes/removedTypes (`attachments.ts:1540–1556`) | No delta — announces all active/completed agents |

**Key differences**:
- Upstream has both task-status attachments AND agent-type-listing delta attachments. Go only has task-status.
- Upstream's task_status includes `deltaSummary` (progress summary for running, error for failed); Go's `InjectRunningAgentStatus()` only includes description.
- Upstream excludes `pending` agents and the current agent (self); Go only lists `TaskRunning` in `InjectRunningAgentStatus()` but includes pending in `buildPostCompactAgentAnnouncement()`.
- Upstream has true delta tracking for agent types; Go has none.

---

### 8. Task/Todo Recovery

| Aspect | Upstream (TypeScript) | Go |
|---|---|---|
| **Entry point** | Part of `getAttachmentMessages()` — scans for `TodoWrite`, `TaskCreate`, `TaskUpdate` tool uses and reconstructs task state | `buildTaskRecoveryAttachment(ctx)` (`agent_loop.go:3407`) |
| **Mechanism** | Uses `listTasks()` / `isTodoV2Enabled()` from `utils/tasks.ts` — reads from in-memory task store | Scans conversation entries backward for `task_create`, `task_update`, `TodoWrite` tool calls, extracts most recent state per task ID (`agent_loop.go:3420–3509`) |
| **V1 vs V2** | Supports both TodoWrite (V1) and TaskCreate/TaskUpdate (V2) via `isTodoV2Enabled()` gate | Handles both `task_create`/`task_update` (V2) and `TodoWrite` (V1) (`agent_loop.go:3449–3507`) |
| **Attachment format** | Structured `{ type: 'todo_list', list, isV2 }` or `{ type: 'task_list', tasks }` | Plain text: `"## Tasks (recovered from transcript)\n\n..."` and `"## Todo List (recovered from transcript)\n\n..."` (`agent_loop.go:3515–3539`) |
| **Todo reminder injection** | Via `getAttachmentMessages()` in the normal per-turn flow | `injectTodoReminder()` — appends in-memory todo list to system prompt after compaction (`agent_loop.go:3545–3562`) |
| **Dedup** | Controlled via per-turn attachment injection (skips if already present) | `injectTodoReminder()` checks `strings.Contains(currentPrompt, fullReminder)` to avoid duplicates (`agent_loop.go:3558`) |

**Key differences**:
- Upstream reads from in-memory task store (authoritative source); Go scans transcript (may miss in-memory-only tasks or include stale states).
- Upstream uses structured typed attachments; Go uses plain text.
- Go adds `injectTodoReminder()` to system prompt post-compact; upstream handles it in the normal attachment flow.

---

### 9. Session Memory Recovery

| Aspect | Upstream (TypeScript) | Go |
|---|---|---|
| **Post-compact injection** | **NOT injected as a separate post-compact attachment**. Session memory is used as the *summary itself* in SM-compact (`sessionMemoryCompact.ts:462–468`), or not injected at all in LLM-compact (the summary replaces the conversation context). | Injected as a separate attachment: `<session_memory>\n...\n</session_memory>` via `a.config.SessionMemory.FormatForPromptCompact()` (`agent_loop.go:3047–3053`) |
| **SM-compact usage** | `truncateSessionMemoryForCompact(sessionMemory)` → per-section truncation with `MAX_SECTION_LENGTH = 2000` tokens per section → used as summary content (`sessionMemoryCompact.ts:462`, `prompts.ts:256–296`) | Same: `truncateSessionMemoryForCompact(sessionMemoryContent, maxSessionMemoryTokens)` with `maxSectionTokens = 2000` per section (`session_memory.go:447–448`, `agent_loop.go:4050`) |
| **Token cap** | `DEFAULT_SM_COMPACT_CONFIG.maxTokens = 40_000` (`sessionMemoryCompact.ts:57`) | `maxSessionMemoryTokens = 40_000` (`agent_loop.go:4046`) |
| **In LLM-compact** | Session memory is NOT separately injected; the LLM-generated summary replaces the context | Session memory IS injected as a separate post-compact attachment (`agent_loop.go:3047–3053`) |
| **Truncation marker** | `wasTruncated` flag → appends note about `memoryPath` for full content (`sessionMemoryCompact.ts:471–474`) | Logs truncation (`agent_loop.go:4051`); no path reference appended to summary |

**Key differences**:
- **Architectural**: Upstream does NOT inject session memory as a post-compact attachment in LLM-compact — the summary is the replacement. Go injects it as an additional attachment, which means session memory content appears *twice* in the context (once in the summary, once as the attachment). This is a significant divergence.
- In SM-compact, both use session memory as the summary, with matching per-section truncation (2K tokens/section).
- Upstream appends a note about where to find the full session memory file when truncated; Go does not.

---

### 10. Hook Execution

| Aspect | Upstream (TypeScript) | Go |
|---|---|---|
| **Pre-compact hooks** | `executePreCompactHooks({ trigger, customInstructions }, signal)` (`compact.ts:417–423`) — merges hook instructions with user instructions via `mergeHookInstructions()` (`compact.ts:424`) | `a.hooks.ExecutePreCompactHooks(PreCompactInput{Trigger, CustomInstructions})` — called before compaction in manual path (`agent_loop.go:1316`) |
| **Post-compact hooks** | `executePostCompactHooks({ trigger, compactSummary }, signal)` (`compact.ts:727–733`) — returns `userDisplayMessage` combined with pre-compact display message | `a.hooks.ExecutePostCompactHooks(PostCompactInput{Trigger, CompactSummary, RecoveredFiles})` (`agent_loop.go:3060–3075`) — also passes `RecoveredFiles` which upstream does not |
| **Session-start hooks** | `processSessionStartHooks('compact', { model })` (`compact.ts:596–598`) — returns `HookResultMessage[]` included in result | **MISSING** — no session-start hook execution after compaction |
| **Hook result handling** | `hookResult.newCustomInstructions` merged into compact prompt; `userDisplayMessage` shown to user; `hookMessages` included in result as `HookResultMessage[]` (`compact.ts:424,735–741`) | `hookResult.UserMessage` printed; `hookResult.Attachment` added as attachment (`agent_loop.go:3069–3074`) |
| **Progress callbacks** | `context.onCompactProgress?.({ type: 'hooks_start', hookType: 'pre_compact'/'post_compact'/'session_start' })` and `{ type: 'compact_start'/'compact_end' }` (`compact.ts:411–413,591–593,723–725,764`) | **MISSING** — no progress callbacks |

**Key differences**:
- Upstream has 3 hook phases (pre-compact, session-start, post-compact); Go has 2 (pre-compact, post-compact). Session-start hooks are missing in Go.
- Upstream's post-compact hooks do NOT receive `RecoveredFiles`; Go's do.
- Upstream has progress callbacks for UI feedback; Go has none.
- Upstream merges hook instructions with custom instructions for the compact prompt; Go separates them.

---

### 11. History Snip (Recent Message Preservation)

| Aspect | Upstream (TypeScript) | Go |
|---|---|---|
| **Full compact** | `messagesToKeep` is empty (not passed, defaults to `[]`) — no messages preserved after full compact. The boundary + summary + attachments + hookMessages form the new context. (`compact.ts:338`) | `KeepRecentMessagesAdaptive(10_000, 5, 40_000)` — preserves recent messages with adaptive token-based calculation (`agent_loop.go:3912,4201`) |
| **SM-compact** | `messagesToKeep` = messages after `lastSummarizedMessageId`, filtered by boundary and adjusted by `calculateMessagesToKeepIndex()` with `minTokens=10_000`, `minTextBlockMessages=5`, `maxTokens=40_000` (`sessionMemoryCompact.ts:57–59,571–581`) | Same adaptive calculation: `KeepRecentMessagesAdaptive(10_000, 5, 40_000)` (`agent_loop.go:4091`) |
| **Partial compact** | `messagesToKeep` = either prefix or suffix of `allMessages` depending on direction (`compact.ts:794–804`) | `KeepRecentMessages(keepCount)` with fixed count for partial compact (`agent_loop.go:1456–1460`) |
| **Message structure fixup** | Not needed (messagesToKeep are preserved as-is in the result) | `ValidateToolPairing()` + `FixRoleAlternation()` after keeping messages (`agent_loop.go:3913–3914,4096–4097,4206–4207`) |
| **Manual /compact** | No messages preserved (full compact) | `KeepRecentMessages(keepCount)` with default count 8 (`agent_loop.go:1456–1460`) |

**Key differences**:
- **Major architectural divergence**: Upstream full compact preserves NO recent messages — only the summary + attachments. Go preserves recent messages after every compact type. This means Go's post-compact context is always larger than upstream's.
- Upstream's SM-compact calculates `messagesToKeep` using `calculateMessagesToKeepIndex()` which expands from `lastSummarizedMessageId` forward. Go's `KeepRecentMessagesAdaptive()` truncates from the tail backward.
- Upstream's partial compact preserves the opposite half of the conversation; Go uses fixed-count tail preservation.

---

### 12. Boundary Marker Format and UUID Tracking

| Aspect | Upstream (TypeScript) | Go |
|---|---|---|
| **Message type** | `SystemCompactBoundaryMessage` with `type: 'system'`, `subtype: 'compact_boundary'` (`messages.ts:4615–4633`) | `CompactBoundaryContent` struct with `Trigger`, `PreCompactTokens`, `UUID`, `PreCompactDiscoveredTools`, `PreservedSegment` (`context.go:49–64`) |
| **UUID** | `randomUUID()` assigned to boundary marker (`messages.ts:4623`) | `UUID` field, generated via `generateUUID()` in `AddCompactBoundary()` (`context.go:858–865`) |
| **Metadata** | `compactMetadata: { trigger, preTokens, userContext, messagesSummarized }` (`messages.ts:4624–4628`) | `Trigger`, `PreCompactTokens` fields; `userContext` and `messagesSummarized` **MISSING** |
| **Discovered tools** | `compactMetadata.preCompactDiscoveredTools` — sorted array of tool names (`compact.ts:612–615`) | `PreCompactDiscoveredTools` — same concept (`context.go:58–59`) |
| **Preserved segment** | `compactMetadata.preservedSegment: { headUuid, anchorUuid, tailUuid }` — chain relinking metadata (`compact.ts:362–369`) | `PreservedSegment *PreservedSegment` with `HeadUUID`, `AnchorUUID`, `TailUUID` (`context.go:66–72`) |
| **Logical parent** | `logicalParentUuid: lastPreCompactMessageUuid` (`messages.ts:4630–4632`) | **MISSING** — no `logicalParentUuid` equivalent |
| **Trigger values** | `'manual' \| 'auto'` (`messages.ts:4610`) | `CompactTriggerAuto`, `CompactTriggerManual`, `CompactTriggerSMCompact` (`compact.go:875–877`) — Go has extra `SMCompact` trigger |
| **Annotation** | `annotateBoundaryWithPreservedSegment()` — patches boundary with relink info (`compact.ts:353–371`) | Applied via option funcs in `AddCompactBoundary()` (`agent_loop.go:4168–4173`) |

**Key differences**:
- Upstream includes `userContext` and `messagesSummarized` in boundary metadata; Go does not.
- Upstream includes `logicalParentUuid` for chain walking; Go does not.
- Go has an extra `CompactTriggerSMCompact` trigger type not present in upstream (upstream uses 'auto' for SM-compact).
- Both support preserved segment chain relinking with the same UUID triplet.

---

### 13. Summary Message Format and Content Requirements

| Aspect | Upstream (TypeScript) | Go |
|---|---|---|
| **Prompt** | `getCompactPrompt(customInstructions)` = `NO_TOOLS_PREAMBLE + BASE_COMPACT_PROMPT + customInstructions + NO_TOOLS_TRAILER` (`prompt.ts:293–302`). 9-section structured summary with `<analysis>` drafting scratchpad. | `buildCompactSummaryMessage()` — Go-specific structured summary builder (`agent_loop.go:2828`). Uses structured template with sections matching upstream's format. |
| **Format** | `formatCompactSummary(summary)` — strips `<analysis>` block, replaces `<summary>` tags with `Summary:\n<content>` (`prompt.ts:311–335`) | No XML tag stripping — Go's summary builder outputs plain text directly |
| **Summary wrapper** | `getCompactUserSummaryMessage(summary, suppressFollowUpQuestions, transcriptPath, recentMessagesPreserved)` (`prompt.ts:337–372`): wraps with `"This session is being continued from a previous conversation..."`, adds transcript path, `"Recent messages are preserved verbatim."`, and continuation directive | Same structure: `"This session is being continued..."` + transcript path + `"Recent messages are preserved verbatim."` + continuation directive (`agent_loop.go:4057–4061`) |
| **Continuation directive** | `"Continue the conversation from where it left off without asking the user any further questions. Resume directly — do not acknowledge the summary..."` (`prompt.ts:358–359`). Proactive mode adds autonomous continuation. | Same text (`agent_loop.go:4061`). No proactive mode variant. |
| **SM-compact summary** | Uses session memory as summary content with `truncateSessionMemoryForCompact()`. Adds truncation notice with `memoryPath` (`sessionMemoryCompact.ts:462–474`) | Same: uses `FormatForPromptCompact()` or raw file, with `truncateSessionMemoryForCompact()` (`agent_loop.go:4046–4052`) |
| **LLM-compact** | Actual LLM generates the summary via `streamCompactSummary()` with 20K max output tokens (`prompt.ts:19–26`, `autoCompact.ts:30`) | LLM compactor calls `a.compactor.Compact()` which generates the summary via API call (`agent_loop.go:4148`) |
| **Non-LLM fallback** | No non-LLM fallback — if compaction fails, it throws | `buildCompactSummaryMessage()` generates a structured template-based summary without LLM (`agent_loop.go:2828`) |
| **Custom instructions** | Merged with hook instructions via `mergeHookInstructions()` (`compact.ts:424–427`) | Appended as `"\n\n## Custom instructions for this compaction:\n" + preCompactInst` (`agent_loop.go:4063`) |

**Key differences**:
- Upstream's compact prompt includes a `NO_TOOLS_PREAMBLE` ("CRITICAL: Respond with TEXT ONLY...") and `NO_TOOLS_TRAILER` to prevent tool calls during summary generation; Go does not have this preamble/trailer pattern.
- Upstream uses `<analysis>`/`<summary>` XML tags with `formatCompactSummary()` post-processing; Go outputs plain text directly.
- Upstream has no non-LLM fallback for summary generation; Go does (template-based).
- Upstream merges custom instructions with hook instructions; Go appends custom instructions separately.

---

### 14. Compact Trigger Types and Their Handling

| Aspect | Upstream (TypeScript) | Go |
|---|---|---|
| **Trigger types** | `'manual' \| 'auto'` (`messages.ts:4610`) | `CompactTriggerAuto`, `CompactTriggerManual`, `CompactTriggerSMCompact` (`compact.go:875–877`) |
| **Manual (/compact)** | `compactConversation()` with `isAutoCompact=false`, `suppressFollowUpQuestions=false` (`compact.ts:391,536–543`). Pre-compact hooks with `trigger: 'manual'`. No messages preserved. | Same flow but with `KeepRecentMessages()` after compaction (`agent_loop.go:1456–1460`) |
| **Auto** | `autoCompactIfNeeded()` → `trySessionMemoryCompaction()` first, then `compactConversation()` with `isAutoCompact=true`, `suppressFollowUpQuestions=true` (`autoCompact.ts:287–321`). Pre-compact hooks with `trigger: 'auto'`. | `tryCompaction()` → SM-compact first, then `tryLLMCompaction()` (`agent_loop.go:3835–3864`). |
| **SM-compact** | Not a separate trigger — uses `trigger: 'auto'` in boundary marker. `trySessionMemoryCompaction()` returns `CompactionResult` with type='auto' boundary (`sessionMemoryCompact.ts:447–448`). `runPostCompactCleanup(querySource)` called separately (`autoCompact.ts:297`). | Separate `CompactTriggerSMCompact` trigger type. Dedicated SM-compact path in `trySMCompaction()` (`agent_loop.go:4030–4142`). |
| **Partial compact** | `partialCompactConversation()` with `trigger: 'manual'`, `direction: 'up_to' \| 'from'` (`compact.ts:776–979`). Preserves opposite half of messages. | `ForcePartialCompact()` with `PartialCompactDirection` (`agent_loop.go:1397–1462`). Preserves tail with fixed count. |
| **Reactive compact** | `reactiveCompact.ts` — separate module for post-PTL compaction | `ReactiveCompactEnabled` config flag with inline reactive logic (`agent_loop.go:1471`) |
| **Circuit breaker** | `MAX_CONSECUTIVE_AUTOCOMPACT_FAILURES = 3` — stops retrying after 3 consecutive failures (`autoCompact.ts:70`) | **MISSING** — no circuit breaker |
| **Post-compact threshold check** | `willRetriggerNextTurn` computed in event data (`compact.ts:660–663`). Not used to fall back. | SM-compact checks if `actualPostTokens >= compactThreshold` and falls back to LLM compaction (`agent_loop.go:4124–4128`) |
| **Post-compact cleanup** | `runPostCompactCleanup(querySource)` called from `autoCompactIfNeeded()` after successful compaction (`autoCompact.ts:297,326`) | `RunPostCompactCleanup()` called from within `PostCompactRecovery()` (`agent_loop.go:3087`) |

**Key differences**:
- Upstream has only 2 trigger types (manual/auto); Go has 3 (manual/auto/SM-compact).
- Upstream's SM-compact uses 'auto' trigger; Go distinguishes it with a separate type.
- Go has a unique post-compact threshold check that can fall back from SM-compact to LLM-compact; upstream does not.
- Upstream has a circuit breaker (3 consecutive failures); Go does not.
- Upstream calls `runPostCompactCleanup()` from the auto-compact orchestrator; Go calls it from within `PostCompactRecovery()`.

---

### Summary of Critical Divergences

| # | Divergence | Impact |
|---|---|---|
| 1 | **Go preserves recent messages after full compact; upstream does not** | Go's context is always larger post-compact; upstream relies solely on summary + attachments. This is the most significant architectural difference. |
| 2 | **Go has no file staleness detection** | After compaction, Go may inject outdated file content. Upstream detects mtime changes and falls back to `compact_file_reference` (path-only). |
| 3 | **Go injects session memory as separate post-compact attachment** | In LLM-compact, session memory content appears twice (once in the summary, once as the attachment). Upstream does not inject session memory separately. |
| 4 | **Go has no session-start hook execution after compaction** | Missing `processSessionStartHooks('compact')` which upstream uses to restore CLAUDE.md and other context. |
| 5 | **Go has no circuit breaker for consecutive auto-compact failures** | Upstream stops retrying after 3 failures; Go may loop indefinitely. |
| 6 | **Go's delta tool/MCP announcement uses coarse heuristic** | Go checks tool_name presence in preserved tail; upstream reconstructs from prior delta attachments. Go may re-announce tools that were announced but not yet invoked. |
| 7 | **Go has no `restoreSkillStateFromMessages()` for resume** | Skill state is lost on session resume; upstream restores it from transcript attachments. |
| 8 | **Go has no agent type listing delta** | Upstream tracks added/removed agent types via `agent_listing_delta`; Go has no equivalent. |
| 9 | **Go uses plain text strings for all attachments** | Upstream uses structured typed `AttachmentMessage<Attachment>` objects. Go's approach loses type information and makes programmatic processing harder. |
| 10 | **Go's boundary marker missing `userContext`, `messagesSummarized`, `logicalParentUuid`** | Less metadata for debugging and chain reconstruction. |
| 11 | **Go's plan mode recovery missing sub-agent awareness and plan file reference** | Upstream includes `isSubAgent` and `planFilePath`/`planExists`; Go uses static text. |
| 12 | **Go's skill recovery has no agent scoping** | All skills injected for all agents; upstream scopes by `agentId`. |

---


---

## 7.1 Cache Breakpoint Placement

| Aspect | Go (`prompt_caching.go`) | Upstream (`claude.ts:addCacheBreakpoints`) |
|--------|----------|-----------|
| **Strategy** | 4-breakpoint: system prompt + last 3 non-system messages | Single-marker: exactly 1 `cache_control` per request |
| **Location** | `ApplyPromptCaching()` lines 17-52 | `addCacheBreakpoints()` `claude.ts:3149-3297` |
| **Marker count** | Up to 4 `cache_control` markers (system + last N non-system, capped at 4-total) | Exactly 1 marker at `messages[markerIndex]` (line 3175) |
| **Marker index** | System (index 0 if system role) + last N non-system | `skipCacheWrite ? messages.length-2 : messages.length-1` (line 3175) |
| **Rationale** | "system_and_3 caching strategy" - Anthropic allows up to 4 breakpoints, Go uses all of them | "Exactly one message-level cache_control marker per request. Mycro's turn-to-turn eviction frees local-attention KV pages at any cached prefix position NOT in cache_store_int_token_boundaries. With two markers the second-to-last position is protected and its locals survive an extra turn even though nothing will ever resume from there." (lines 3164-3174) |
| **skipCacheWrite** | Not implemented | Shifts marker to second-to-last message (line 3175) - "fire-and-forget forks don't leave their own tail in the KVCC" |
| **System prompt caching** | `cacheMessageParams()` line 123: `params.System[0].CacheControl = anthropic.CacheControlEphemeralParam{}` | System-level caching handled separately via `system` array with `cache_control` on each TextBlockParam |
| **TTL support** | `ttl` parameter in `ApplyPromptCaching` (5m default, 1h option) and `FormatBoundaryCachedSystemPrompt` | `getCacheControl({querySource})` returns scope+TTL based on query source and GrowthBook gating (1h for eligible users) |
| **Scope support** | Not implemented - no `global`/`org` cache scope | `getCacheControl()` returns `{type:'ephemeral', scope:'global'|'org', ttl:'5m'|'1h'}` based on eligibility |

### Key Difference: 4-breakpoint vs single-marker

Go places up to 4 cache_control markers, upstream places exactly 1. Upstream's comment (lines 3164-3174) explains that multiple markers cause Mycro (Anthropic's KV cache manager) to protect intermediate positions that won't be resumed from, wasting KV pages. Go's 4-breakpoint strategy may work with older API behavior but could waste cache resources on current infrastructure.


---

## 7.2 Cache Edit Injection

| Aspect | Go (`agent_loop.go`) | Upstream (`claude.ts` + `microCompact.ts`) |
|--------|----------|-----------|
| **Function** | `injectCacheEdits()` at `agent_loop.go:3096` | `addCacheBreakpoints()` lines 3227-3248 + `cachedMicrocompactPath()` in `microCompact.ts:305-399` |
| **Mechanism** | Injects `cache_edits` block into last user message via JSON marshal/unmarshal round-trip | `insertBlockAfterToolResults()` adds cache_edits block after tool_result blocks in user messages |
| **Edit type** | `{type: "cache_edits", edits: [{type: "delete_tool_result", tool_use_id: id}]}` | `{type: 'cache_edits', edits: [{type: 'delete', cache_reference: id}]}` |
| **Field name** | `tool_use_id` | `cache_reference` - upstream uses `cache_reference` field, not `tool_use_id` |
| **Pin tracking** | `CachedMicrocompactTracker.pinnedEdits` field exists but not populated (line 2711: `pinnedEdits []any`) | `pinCacheEdits(userMessageIndex, block)` in `microCompact.ts:111-118` stores `{userMessageIndex, block}` for re-insertion |
| **Pin re-insertion** | Not implemented | `addCacheBreakpoints()` lines 3213-3225: re-inserts all previously-pinned cache_edits at their original user message positions |
| **Deduplication** | Not implemented | `deduplicateEdits()` lines 3202-3210: prevents duplicate cache_reference deletions across blocks |
| **cache_reference tagging** | Not implemented | Lines 3250-3295: adds `cache_reference: tool_use_id` to tool_result blocks strictly before the last cache_control marker |
| **API layer vs local** | Local JSON manipulation then converted back to `MessageParam` | API-layer only - local messages are never modified; cache_edits/cache_reference are added at the `addCacheBreakpoints` transformation step |

### Key Difference: cache_reference vs tool_use_id

Upstream's cache editing API uses `cache_reference` field on tool_result blocks (added at line 3288-3289) plus `{type: 'delete', cache_reference: string}` edit entries. Go uses `{type: 'delete_tool_result', tool_use_id: string}` without adding `cache_reference` to tool_result blocks. This is a protocol difference - upstream likely requires `cache_reference` for the server to identify which cached prefix entries to delete.

### Key Difference: Pinning and re-insertion

Upstream pins cache_edits to specific user message positions and re-inserts them on subsequent calls so the server sees a consistent prefix with the same edits. Go's `CachedMicrocompactTracker.pinnedEdits` field exists but is never populated (declared as `[]any`), meaning Go doesn't maintain edit continuity across turns.


---

## 7.3 Cache Sharing Across Forked Agents

| Aspect | Go | Upstream (`forkedAgent.ts`) |
|--------|-----|-----------|
| **CacheSafeParams** | Not implemented | `CacheSafeParams` type at `forkedAgent.ts:57-68`: carries `systemPrompt`, `userContext`, `systemContext`, `toolUseContext`, `forkContextMessages` |
| **Save/restore** | Not implemented | `saveCacheSafeParams()` / `getLastCacheSafeParams()` at `forkedAgent.ts:73-80` - slot written after each turn so post-turn forks share the main loop's prompt cache |
| **Fork cache hit** | Forked agents make fresh API calls with no cache sharing | Forked agents inherit parent's `CacheSafeParams` to guarantee cache hits - "share identical cache-critical params with the parent to guarantee prompt cache hits" (line 5) |


---

## 7.4 Cache Invalidation on Micro-Compact

| Aspect | Go (`context.go`) | Upstream (`microCompact.ts`) |
|--------|----------|-----------|
| **MicroCompact type** | `MicroCompactEntries()` at `context.go:1285` - local content replacement | Two paths: time-based (content-clear, line 446) + cached (cache_edits, line 305) |
| **Cache invalidation** | Local content replacement inherently invalidates cache (changed text = different prefix hash) | Time-based MC: content-clears AND resets cachedMCState + notifies cache break detection (lines 517-527). Cached MC: uses cache_edits API - no local content change, no cache invalidation |
| **Cached microcompact** | `CachedMicrocompactTracker` in `compact.go:2706-2861` - tracks deletions, generates cache_edits block | `cachedMicrocompactPath()` in `microCompact.ts:305-399` - same concept with GrowthBook gating and proper pinning |
| **Feature gating** | Always enabled if cache_edits block is non-nil | Gated by `feature('CACHED_MICROCOMPACT')` + `isCachedMicrocompactEnabled()` (env var) + `isModelSupportedForCacheEditing(model)` (Claude 4.x only) + `isMainThreadSource(querySource)` |
| **Post-MC state reset** | `ResetForTimeBasedMC()` at `compact.go:2859` resets tracker | `resetMicrocompactState()` at `microCompact.ts:130-135` resets state + clears pending edits |
| **Compaction notification** | Not implemented | `notifyCompaction(querySource)` in `promptCacheBreakDetection.ts:689-698` resets `prevCacheReadTokens` to null so the next call isn't false-positive detected as a break |

### Key Difference: Cached microcompact model support

Upstream gates cached microcompact on `isModelSupportedForCacheEditing(model)` which requires `claude-[a-z]+-4[-\d]` regex match (`cachedMCConfig.ts:33`). Go doesn't check model compatibility - it would attempt cache_edits on any model, potentially causing API errors on models that don't support the feature.


---

## 7.5 Cache Creation/Read Token Tracking

| Aspect | Go | Upstream |
|--------|-----|----------|
| **cache_creation_input_tokens** | Not tracked | Used in `promptCacheBreakDetection.ts:441` (passed to `checkResponseForCacheBreak`) and logged in `tengu_api_success` analytics |
| **cache_read_input_tokens** | Not tracked | Used as primary metric for cache break detection: `prevCacheRead - cacheReadTokens > 5% drop` triggers break investigation (lines 485-493) |
| **Token delta tracking** | Not implemented | `prevCacheReadTokens` stored per source in `previousStateBySource` map (line 65) |
| **Analytics** | Not implemented | `logEvent('tengu_prompt_cache_break', {...})` at line 590 with 20+ fields including per-tool schema changes, beta header diffs, effort changes |


---

## 7.6 Pinned Cache Edits - Upstream's Always-Cached Blocks

| Aspect | Go | Upstream |
|--------|-----|----------|
| **Pin data structure** | `pinnedEdits []any` in `CachedMicrocompactTracker` (compact.go:2711) - declared but never populated | `PinnedCacheEdits = { userMessageIndex: number, block: CacheEditsBlock }` in `cachedMicrocompact.ts:14-17` |
| **Pin function** | Not implemented | `pinCacheEdits(userMessageIndex, block)` in `microCompact.ts:111-118` |
| **Re-insertion on next call** | Not implemented | `addCacheBreakpoints()` lines 3213-3225: iterates `pinnedEdits` and re-inserts each block at its original `userMessageIndex` position |
| **Purpose** | N/A | Ensures previously-deleted tool results stay deleted across turns without re-registering. The server's cache prefix includes the cache_edits, so re-sending them maintains prefix consistency. |


---

## 7.7 Cache Break Detection - Upstream Only

Go has **no cache break detection**. Upstream has a sophisticated two-phase system:

| Aspect | Implementation |
|--------|---------------|
| **Phase 1 (pre-call)** | `recordPromptState()` at `promptCacheBreakDetection.ts:247-430` - hashes system prompt, tool schemas, cache_control, betas, model, effort, extraBody, and stores `pendingChanges` |
| **Phase 2 (post-call)** | `checkResponseForCacheBreak()` at line 437-666 - compares `cacheReadTokens` vs previous; if >5% drop AND >2000 token absolute drop, reports break with root cause |
| **Tracked fields** | 12 fields: systemHash, toolsHash, cacheControlHash, model, fastMode, globalCacheStrategy, betas, autoModeActive, isUsingOverage, cachedMCEnabled, effortValue, extraBodyHash |
| **Per-tool hashing** | When aggregate tool hash changes, computes per-tool hashes to identify which specific tool's schema changed (lines 369-378) |
| **TTL detection** | Checks time since last assistant message; labels breaks as "possible 5min TTL expiry" or "possible 1h TTL expiry" (lines 571-588) |
| **Diff output** | Writes unified diff of system prompt + tool schemas to temp file for debugging (lines 708-727) |
| **Source tracking** | Per-querySource tracking with `MAX_TRACKED_SOURCES=10` cap and LRU eviction (lines 107-108) |
| **Deletion awareness** | `notifyCacheDeletion()` suppresses false positives when cached MC intentionally drops cache reads (lines 673-682) |
| **Compaction awareness** | `notifyCompaction()` resets `prevCacheReadTokens` to null after compaction (lines 689-698) |
| **Analytics event** | `tengu_prompt_cache_break` with 20+ fields logged to BigQuery for monitoring |

---

# Part 8: Auto Classifier - Deep Comparison

**Research date: 2026-05-11**


---

## 36. Compact.go Deep Dive — Prompt, Retry, Post-Compact Recovery

### Files Compared
- **Go**: `E:\Git\miniClaudeCode-go-github\compact.go` (~2862 lines)
- **Upstream**: `E:\Git\claude-code-upstream\src\services\compact\compact.ts`, `microCompact.ts`, `cachedMicrocompact.ts`, `sessionMemoryCompact.ts`

### 36.1 LLM Compaction Flow

| # | Aspect | Go (`compact.go`) | Upstream (`compact.ts`) | Type |
|---|--------|-------------------|------------------------|------|
| 1 | Forked-agent compact | `doCompactLLMCall()` direct streaming API call — always creates new cache entry | `streamCompactSummary()` uses `runForkedAgent()` reusing main conversation's prompt cache. Falls back to streaming | 缺失 |
| 2 | Custom instructions | No `customInstructions` parameter in `doCompactLLMCall` (line 1530) | `mergeHookInstructions()` appends hook instructions after user instructions | 缺失 |
| 3 | Iterative summary | `iterativeCompactPrompt` with `{previous_summary}` placeholder (line 1098). `Compactor.lastSummary` stores previous (line 1289) | No iterative summary — re-summarizes full conversation from scratch via forked agent | Go适配 (risk of summary drift) |
| 4 | Summary formatting | `extractSummaryFromCompactOutput()` strips tags, returns inner text (line 1980-1994) | `formatCompactSummary()` replaces `<summary>` with `Summary:\n` header, collapses whitespace | 简化 |
| 5 | PARTIAL_COMPACT_UP_TO_PROMPT | Uses same `partialCompactPrompt` for both directions | Distinct `PARTIAL_COMPACT_UP_TO_PROMPT` with "Work Completed" and "Context for Continuing Work" sections | 缺失 |
| 6 | context_management API | Injects `clear_tool_uses_20250919`, `clear_thinking_20251015` in compact API call (line 1606-1611) | No `context_management` edits in compact call — forked-agent inherits parent's tool set | Go适配 |
| 7 | PTL retry | 3 retries, groups by user-role boundary, drops oldest rounds (line 1416-1520) | 3 retries, groups by `message.id` boundary, prepends `PTL_RETRY_MARKER` for assistant-first protection | 简化 |
| 8 | Thinking config | `ThinkingConfigDisabledParam` (line 1597-1599) | `thinkingConfig: { type: 'disabled' }` | 匹配 |

### 36.2 Token Estimation & Grouping

| # | Aspect | Go (`compact.go`) | Upstream | Type |
|---|--------|-------------------|----------|------|
| 1 | Grouping algorithm | `groupMessagesByRound()`: new group on each "user" role message (line 174-233) | Groups by `message.id` boundary — all streaming chunks from same API response grouped together | 简化 (fundamental difference) |
| 2 | Base estimation | `EstimateTokens()`: `len(text) / 4` (line 37-42) | `roughTokenCountEstimation()`: `text.length / 3.5` | 简化 |

### 36.3 Micro-Compact

| # | Aspect | Go (`compact.go:2706-2862`) | Upstream (`cachedMicrocompact.ts`) | Type |
|---|--------|--------------------------|-----------------------------------|------|
| 1 | Model gating | No model gate — applied to all models | `isModelSupportedForCacheEditing()`: only Claude 4.x models (`/claude-[a-z]+-4[-\d]/`) | 缺失 |
| 2 | Environment gate | No env gate — always enabled | `isCachedMicrocompactEnabled()`: requires `CLAUDE_CACHED_MICROCOMPACT=1` or feature flag | 缺失 |
| 3 | Pinned edits | `pinnedEdits []any` declared but never populated or used | `pinnedEdits: PinnedCacheEdits[]` — re-sent at original positions for cache consistency | 缺失 |
| 4 | Data structure | `map[string]bool` for registeredTools/deletedRefs | `Set<string>` — functionally equivalent | Go适配 |
| 5 | Compactable tools | `{read_file, exec, edit_file, write_file, multi_edit, grep, glob, web_fetch, web_search}` (line 2674-2684) | `{FileRead, Shell, Grep, Glob, WebSearch, WebFetch, FileEdit, FileWrite}` | 简化 |

### 36.4 Post-Compact Recovery (Missing Attachments)

| # | Aspect | Go | Upstream (`compact.ts:1421+`) | Type |
|---|--------|----|-------------------------------|------|
| 1 | File re-read | No file re-read after compaction | `createPostCompactFileAttachments()`: re-reads up to 5 files from readFileState, 50K token budget, 5K per file | 缺失 |
| 2 | Plan recovery | No plan file recovery | `createPlanAttachmentIfNeeded()` | 缺失 |
| 3 | Skill recovery | No skill re-injection | `createSkillAttachmentIfNeeded()`: 25K budget, 5K per skill | 缺失 |
| 4 | Plan mode recovery | No plan mode attachment | `createPlanModeAttachmentIfNeeded()` | 缺失 |
| 5 | Delta tools | No delta re-announcement | `getDeferredToolsDeltaAttachment()`, `getAgentListingDeltaAttachment()`, `getMcpInstructionsDeltaAttachment()` | 缺失 |
| 6 | Session start hooks | No post-compact session start hooks | `processSessionStartHooks('compact', ...)` | 缺失 |
| 7 | Post-compact hooks | No hooks | `executePostCompactHooks()` | 缺失 |
| 8 | Pre-compact hooks | No hooks | `executePreCompactHooks()` | 缺失 |
| 9 | Post-compact cleanup | 4 no-op stubs: clearSpeculativeChecks, clearBetaTracingState, clearSessionMessagesCache, sweepFileContentCache (line 2873-2894) | `runPostCompactCleanup()`: resets microcompact, context collapse, getUserContext, getMemoryFiles, system prompt, classifier, etc. | 简化 (stubs) |

### 36.5 Compactor Struct & Auto-Compact

| # | Aspect | Go (`compact.go:1280-1409`) | Upstream (`autoCompact.ts`) | Type |
|---|--------|--------------------------|--------------------------|------|
| 1 | Anti-thrashing | Tracks `lastCompactSavings`, skips if last 2 compactions each saved <10% | No equivalent | Go增强 |
| 2 | Cooldown | Skips if tokens haven't grown 25% since last compaction | No cooldown | Go增强 |
| 3 | Permanent disable | Disables LLM auto-compact after 3 failures (permanent) | Circuit breaker per-turn state that resets | Go适配 (more aggressive) |
| 4 | Missing env/config | No `DISABLE_COMPACT`, `DISABLE_AUTO_COMPACT`, `CLAUDE_AUTOCOMPACT_PCT_OVERRIDE` env support | All env vars supported | 缺失 |
| 5 | Query source guards | No guards for subagent/session_memory compact prevention | `querySource` guards prevent recursion | 缺失 |

### 36.6 Boundary Metadata

| # | Aspect | Go (`compact.go:2115-2119`) | Upstream | Type |
|---|--------|--------------------------|----------|------|
| 1 | Compact boundary | `CompactBoundaryContent` has Trigger, PreCompactTokens, UUID | `compactMetadata` with preCompactDiscoveredTools, preservedSegment (headUuid, anchorUuid, tailUuid), userFeedback, messagesSummarized | 简化 |
| 2 | Disk chain reconstruction | No preservedSegment tracking | `preservedSegment` metadata used by session loader to chain UUIDs on disk after compaction | 缺失 |
| 3 | Deferred tools | No preCompactDiscoveredTools | `preCompactDiscoveredTools` carries loaded deferred tool state across compaction | 缺失 |

### 36.7 Image/Document Stripping

| # | Aspect | Go (`compact.go:2613-2664`) | Upstream | Type |
|---|--------|--------------------------|----------|------|
| 1 | Stripping approach | Regex-based: replaces base64 `data:image/...` and image URLs in text content | Structured block level: replaces `image`/`document` content blocks with text | Go适配 (different approach) |
| 2 | Attachment stripping | No skill_discovery/skill_listing attachment stripping | `stripReinjectedAttachments()` removes experimental skill attachments | 缺失 |
| 3 | Sensitive info redaction | `redactSensitiveText()`: regex for API keys, passwords, tokens, credentials (line 2577-2609) | No explicit redaction — relies on API safety | Go增强 (but may degrade summary quality) |


---

