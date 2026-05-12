# 05 — Services: OAuth, Analytics, Telemetry & Settings

> Authentication, observability, feature flags, remote settings, MCP, upstream utilities

## Overview

The upstream has extensive service infrastructure for authentication, analytics, telemetry, feature flags, and remote configuration. Go has almost none of these.

---

## 1. OAuth/PKCE Authentication

| # | Aspect | Go | Upstream | Gap |
|---|--------|-----|----------|-----|
| 1 | Multi-source auth resolution | env only: `ANTHROPIC_API_KEY` → `ANTHROPIC_AUTH_TOKEN` (`main.go:67-78`) | 5 sources with precedence: env → file descriptor → apiKeyHelper → keychain → config (`auth.ts:227-349`) | 简化 |
| 2 | OAuth 2.0 PKCE flow | **Not implemented** — API key only | `OAuthService`, code verifier/challenge (`services/oauth/index.ts`) | 缺失 |
| 3 | OAuth token storage | **Not implemented** | `saveOAuthTokensIfNeeded`, `getClaudeAIOAuthTokens` (`auth.ts:1189-1296`) | 缺失 |
| 4 | OAuth token refresh with lock-based dedup | **Not implemented** | `checkAndRefreshOAuthTokenIfNeeded` (`auth.ts:1423-1558`) | 缺失 |
| 5 | macOS Keychain integration | **Not implemented** | `getApiKeyFromConfigOrMacOSKeychain`, `saveApiKey` (`auth.ts:1047-1187`) | 缺失 |
| 6 | AWS auth refresh (STS) | **Not implemented** | `awsAuthRefresh` + `awsCredentialExport` (`auth.ts:604-808`) | 缺失 |
| 7 | GCP auth refresh | **Not implemented** | `gcpAuthRefresh` + credential check (`auth.ts:813-1010`) | 缺失 |
| 8 | apiKeyHelper (exec helper with SWR cache) | **Not implemented** | `apiKeyHelper` settings with TTL, trust guard (`auth.ts:356-603`) | 缺失 |
| 9 | Subscription type detection | **Not implemented** | `getSubscriptionType`: max/pro/enterprise/team (`auth.ts:1658-1673`) | 缺失 |
| 10 | Rate limit tier detection | **Not implemented** | Rate limit tier from auth (`auth.ts:1698-1708`) | 缺失 |
| 11 | Auth token source tracking | **Not implemented** | `getAuthTokenSource`: env/FD/helper/keychain/oauth (`auth.ts:154-207`) | 缺失 |
| 12 | 3P auth detection (Bedrock/Vertex/Foundry) | **Not implemented** | `isAnthropicAuthEnabled` (`auth.ts:100-150`) | 缺失 |
| 13 | Auth Bearer header construction | Simple `"Bearer "+apiKey` (`agent_loop.go:367`) | OAuth-aware, `x-api-key` vs `Authorization` | 简化 |
| 14 | Logout | Not applicable | `logout()` clears tokens + keychain | 缺失 |
| 15 | Org membership | None | `getOrganizationMember()` check for org-based features | 缺失 |
| 16 | Auth status | None | `getAuthStatus()` with tier detection (free/pro/enterprise) | 缺失 |
| 17 | Max session enforcement | None | `isMaxSession()` checks usage limits for free-tier users | 缺失 |
| 18 | C4E/GDPR restrict | None | `isC4EUser()`, `isGdprRestricted()` for compliance | 缺失 |
| 19 | OAuth client initialization | **Not implemented** | `src/services/oauth/client.ts:1-200` — full OAuth client | 缺失 |
| 20 | PKCE code verifier/challenge | **Not implemented** | `src/services/oauth/client.ts:47-105` | 缺失 |
| 21 | Auth URL builder | **Not implemented** | `src/services/oauth/client.ts:46-105` — Claude.ai / Console | 缺失 |
| 22 | Token exchange (code → tokens) | **Not implemented** | `src/services/oauth/client.ts:107-144` | 缺失 |
| 23 | Token refresh with scope expansion | **Not implemented** | `src/services/oauth/client.ts:146-200` | 缺失 |
| 24 | OAuth profile retrieval | **Not implemented** | `src/services/oauth/getOauthProfile.ts` | 缺失 |
| 25 | Auth code listener | **Not implemented** | `src/services/oauth/auth-code-listener.ts` — localhost HTTP server | 缺失 |
| 26 | OAuth crypto (state, code_verifier) | **Not implemented** | `src/services/oauth/crypto.ts` | 缺失 |
| 27 | Multi-account / orgUUID support | **Not implemented** | `src/services/oauth/client.ts:53-91` | 缺失 |
| 28 | SSO / magic_link / Google login | **Not implemented** | `src/services/oauth/client.ts:99-102` | 缺失 |

**Impact**: **Critical** — OAuth is the primary auth method for claude-code. Without it, Go is limited to API-key users only.

[diff_upstream/23-oauth-auth.md §46.4] [diff_upstream/28-services.md §52.2]

---

## 2. Analytics & Telemetry

| # | Aspect | Go | Upstream | Gap |
|---|--------|----|----------|-----|
| 1 | OpenTelemetry instrumentation | Not implemented | `utils/telemetry/instrumentation.ts:87-706` — `bootstrapTelemetry`, `initializeTelemetry` | 缺失 |
| 2 | OTel event logging | Not implemented | `utils/telemetry/events.ts:21-75` — `logOTelEvent` with event sequence, prompt ID, workspace | 缺失 |
| 3 | OTel session tracing | Not implemented | `utils/telemetry/sessionTracing.ts:1-928` — `startInteractionSpan`, `startLLMRequestSpan`, `startToolSpan` | 缺失 |
| 4 | OTel telemetry attributes | Not implemented | `utils/telemetry/telemetryAttributes.ts:29-71` — user.id, session.id, organization.id, terminal.type | 缺失 |
| 5 | BigQuery internal metrics exporter | Not implemented | `utils/telemetry/bigqueryExporter.ts:40-252` | 缺失 |
| 6 | Perfetto tracing (Chrome Trace Event format) | Not implemented | `utils/telemetry/perfettoTracing.ts` | 缺失 |
| 7 | 1st-party event logger (batched) | Not implemented | `services/analytics/firstPartyEventLogger.ts:1-451` — `logEventTo1P`, `initialize1PEventLogging`, event sampling | 缺失 |
| 8 | Analytics event routing sink (Datadog + 1P fanout) | Not implemented | `services/analytics/index.ts:1-80+` — `logEvent`, `AnalyticsSink` | 缺失 |
| 9 | Analytics config | Not implemented | `services/analytics/config.ts:19-27` — `isAnalyticsDisabled`, privacy gating | 缺失 |
| 10 | Per-sink analytics killswitch | Not implemented | `services/analytics/sinkKillswitch.ts:18-25` — GrowthBook-driven | 缺失 |
| 11 | Event metadata enrichment | Not implemented | `services/analytics/metadata.ts` — model, session, env, betas | 缺失 |
| 12 | Privacy level system | Not implemented | `utils/privacyLevel.ts:17-44` — default / no-telemetry / essential-traffic | 缺失 |
| 13 | Langfuse tracing integration | Not implemented | `services/langfuse/client.ts:13-40+` | 缺失 |
| 14 | User prompt redaction | Not implemented | `utils/telemetry/events.ts:17-19` — `redactIfDisabled`, `OTEL_LOG_USER_PROMPTS` | 缺失 |
| 15 | Telemetry shutdown + flush | Not implemented | `utils/telemetry/instrumentation.ts:658-753` | 缺失 |
| 16 | Analytics event queue (pre-sink) | N/A | `src/services/analytics/index.ts:81-123` | 缺失 |
| 17 | `AnalyticsSink` interface | N/A | `src/services/analytics/index.ts:72-78` — `logEvent`/`logEventAsync` | 缺失 |
| 18 | Datadog event tracking | N/A | `src/services/analytics/sink.ts:63-67` | 缺失 |
| 19 | 1P event logging | N/A | `src/services/analytics/sink.ts:71` | 缺失 |
| 20 | Event sampling configuration | N/A | `src/services/analytics/sink.ts:49-61` | 缺失 |
| 21 | PII-tagged proto fields stripping | N/A | `src/services/analytics/index.ts:45-58` — `_PROTO_*` fields stripped | 缺失 |
| 22 | `AnalyticsMetadata` verification type | N/A | `src/services/analytics/index.ts:19-33` | 缺失 |
| 23 | Custom analytics events | None | `recordAuthEvent()`, `recordToolCallEvent()`, `recordSessionEvent()` etc. | 缺失 |
| 24 | Error reporting | Print to stderr only | Structured error reporting to Sentry with severity levels | 缺失 |
| 25 | Session analytics | None | Session start/end, duration, model used, tools called | 缺失 |
| 26 | Usage analytics | None | Per-command usage tracking for product decisions | 缺失 |
| 27 | Statsig analytics | None | `logApiRequestEnqueued()`, `logApiRequestReturned()` | 缺失 |
| 28 | `CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC` | Not applicable | Telemetry opt-out env var | N/A |

**Impact**: No observability. Cannot diagnose production issues, track errors, or understand usage patterns.

[diff_upstream/22-analytics-telemetry.md §46.1] [diff_upstream/28-services.md §52.5]

---

## 3. Feature Flags (GrowthBook)

| # | Aspect | Go | Upstream | Gap |
|---|--------|----|----------|-----|
| 1 | Feature flag system | None | Full GrowthBook SDK integration | 缺失 |
| 2 | GrowthBook client initialization | **Not implemented** | `getGrowthBookClient`, `initializeGrowthBook` (`growthbook.ts:565-700`) | 缺失 |
| 3 | Flag count | 0 | 20+ flags: `modelContext`, `microcompact`, `proactiveCompaction`, `reactiveCompact`, `thinkingBudget`, `cacheTTL`, `skillBundles`, `autoMemory`, etc. | 缺失 |
| 4 | Flag evaluation | None | `isFeatureEnabled()` with attribute-based targeting | 缺失 |
| 5 | Experiment enrollment | None | A/B test enrollment with forced variations | 缺失 |
| 6 | Cache invalidation | None | Periodic + on-change refresh of flag values | 缺失 |
| 7 | Sticky evaluation | None | `stickyBucketService` for consistent user experience | 缺失 |
| 8 | Remote eval payload processing | **Not implemented** | `processRemoteEvalPayload` (`growthbook.ts:329-394`) | 缺失 |
| 9 | GrowthBook disk cache sync | **Not implemented** | `syncRemoteEvalToDisk` (`growthbook.ts:407-417`) | 缺失 |
| 10 | Periodic refresh (20min ant / 6h external) | **Not implemented** | `setupPeriodicGrowthBookRefresh` (`growthbook.ts:1119-1231`) | 缺失 |
| 11 | Cached flag reads | **Not implemented** — all features hardcoded | `getFeatureValue_CACHED_MAY_BE_STALE` (`growthbook.ts:819-870`) | 缺失 |
| 12 | Local gate defaults (70+ feature defaults) | **Not implemented** | `LOCAL_GATE_DEFAULTS` (`growthbook.ts:434-478`) | 缺失 |
| 13 | Env-var overrides for features | **Not implemented** | `CLAUDE_INTERNAL_FC_OVERRIDES` (`growthbook.ts:167-202`) | 缺失 |
| 14 | Experiment exposure logging | **Not implemented** | `logExposureForFeature` with dedup (`growthbook.ts:296-314`) | 缺失 |
| 15 | Post-auth refresh | **Not implemented** | `refreshGrowthBookAfterAuthChange` — destroy + recreate client (`growthbook.ts:1050-1089`) | 缺失 |
| 16 | Dynamic config support | **Not implemented** | `getDynamicConfig_CACHED_MAY_BE_STALE`, `getDynamicConfig_BLOCKS_ON_INIT` (`growthbook.ts:1243-1262`) | 缺失 |

**Impact**: No remote feature gating. All features must be hardcoded or config-file-based.

[diff_upstream/24-feature-flags.md §46.3]

---

## 4. Remote Settings

| # | Aspect | Go | Upstream | Gap |
|---|--------|-----|----------|-----|
| 1 | Local settings file loading | `config.go:147-281` — `.claude/settings.json` | `src/services/remoteManagedSettings/index.ts:147-280` | 简化 |
| 2 | Home directory fallback | `config.go:171-197` — `~/.claude/settings.json` | `src/services/remoteManagedSettings/index.ts:1-200` | 简化 |
| 3 | Remote API fetch for enterprise settings | **Not implemented** | `src/services/remoteManagedSettings/index.ts:102-200` | 缺失 |
| 4 | Checksum-based settings validation | **Not implemented** | SHA-256 checksum validation (`src/services/remoteManagedSettings/index.ts:131-137`) | 缺失 |
| 5 | Background polling | **Not implemented** | 1h interval background polling (`src/services/remoteManagedSettings/index.ts:54-57`) | 缺失 |
| 6 | Loading promise / wait-for-initialization | **Not implemented** | Initialization promise blocks until first settings loaded (`src/services/remoteManagedSettings/index.ts:77-99,155-159`) | 缺失 |
| 7 | Security check for tampered settings | **Not implemented** | `securityCheck.tsx` | 缺失 |
| 8 | Sync cache with eligibility gating | **Not implemented** | `syncCache.ts` | 缺失 |
| 9 | OAuth + API key dual auth for settings endpoint | **Not implemented** | Dual auth for settings fetch (`src/services/remoteManagedSettings/index.ts:166-200`) | 缺失 |
| 10 | Fail-open behavior | **Not implemented** | Fails open on API error (`src/services/remoteManagedSettings/index.ts:1-13`) | 缺失 |
| 11 | Max thinking budget | Not set | Remote-configurable `maxBudgetTokens` | 缺失 |
| 12 | Max output tokens | Not set | Remote-configurable `maxOutputTokens` | 缺失 |
| 13 | Model config | Hardcoded | Remote model configurations (context windows, pricing, betas) | 缺失 |
| 14 | Organization settings | None | Org-level settings with merge precedence | 缺失 |

[diff_upstream/28-services.md §52.3]

---

## 5. MCP (Model Context Protocol)

| Aspect | Go | Upstream | Gap |
|--------|-----|----------|-----|
| Transport types | 1 (`stdio` only) | 7+: `stdio`, `sse`, `streamableHttp`, `websocket`, `openapi`, `anthropic`, `remote` | 缺失 |
| Auto-approval | Not supported | `autoApprove` for read-only MCP tools | 缺失 |
| Health monitoring | Not implemented | `McpHealthMonitor` with periodic health checks | 缺失 |
| Circuit breaker | Not implemented | Breaker on consecutive MCP failures | 缺失 |
| Per-agent MCP | Not supported | Each agent can have different MCP servers | 缺失 |
| Permission scoping | Not supported | MCP tools restricted by permission mode | 缺失 |
| Output schema enforcement | Not implemented | `outputSchema` validated against JSON Schema | 缺失 |
| Sampling handler | Not implemented | `McpSamplingHandler` for server-initiated LLM calls | 缺失 |
| Resource subscription | Not implemented | `resources_subscribe` handler | 缺失 |
| Discovery protocol | Basic `list_tools` | `list_tools` + `list_resources` + `list_prompts` + `skill://` | 简化 |
| Logging handler | Not implemented | `logging/setLevel` handler | 缺失 |
| Tool annotations | Not supported | `toolAnnotations` for hints (title, readOnlyHint, openWorldHint) | 缺失 |
| Concurrency control | Sequential | `maxConcurrentConnections` for parallel setup | 缺失 |

**Action**: Add SSE and streamableHttp transport types. Add health monitoring. Add output schema enforcement.

---

## 6. Conversation Recovery

| # | Aspect | Go | Upstream | Gap |
|---|--------|------|----------|-----|
| 1 | Transcript resume entry point | `main.go:128-158` (`findTranscript`, `NewAgentLoopFromTranscript`) | `conversationRecovery.ts:459-600` (`loadConversationForResume`) | Go适配 |
| 2 | JSONL transcript loading | `main.go:579-599` (`loadTranscriptList`) | `conversationRecovery.ts:419-443` (`loadMessagesFromJsonlPath`) | 简化 |
| 3 | Message deserialization | Go: no explicit deserialization | `conversationRecovery.ts:154-252` (`deserializeMessagesWithInterruptDetection`) | 缺失 |
| 4 | Legacy attachment migration | Not implemented | `conversationRecovery.ts:77-132` (`migrateLegacyAttachmentTypes`) | 缺失 |
| 5 | Unresolved tool use filtering | Go: relies on `ValidateToolPairing` at truncation time | `conversationRecovery.ts:187-189` (`filterUnresolvedToolUses`) | 简化 |
| 6 | Orphaned thinking message filtering | Not implemented | `conversationRecovery.ts:194-196` (`filterOrphanedThinkingOnlyMessages`) | 缺失 |
| 7 | Whitespace-only assistant message filtering | Not implemented | `conversationRecovery.ts:200-202` (`filterWhitespaceOnlyAssistantMessages`) | 缺失 |
| 8 | Turn interruption detection | Go: Ctrl+C sets `SetInterrupted(true)`; no message-level detection | `conversationRecovery.ts:272-333` (`detectTurnInterruption`) | 简化 |
| 9 | Synthetic assistant sentinel insertion | Not implemented | `conversationRecovery.ts:231-245` (appends `NO_RESPONSE_REQUESTED`) | 缺失 |
| 10 | Auto-continue for interrupted turns | Not implemented | `conversationRecovery.ts:210-224` (injects "Continue from where you left off.") | 缺失 |
| 11 | Terminal tool result detection (brief mode) | Not implemented | `conversationRecovery.ts:348-375` (`isTerminalToolResult`) | 缺失 |
| 12 | Skill state restoration from messages | Go: skills restored via CLAUDE.md re-injection | `conversationRecovery.ts:384-406` (`restoreSkillStateFromMessages`) | Go适配 |
| 13 | Session metadata carry-over | Not implemented | `conversationRecovery.ts:572-595` — returns 12+ metadata fields: agentName, agentColor, mode, tag | 缺失 |
| 14 | File history snapshot copying | Go: FileHistory created fresh on startup (`main.go:103`) | `conversationRecovery.ts:553` (`copyFileHistoryForResume`) | 简化 |
| 15 | Plan copying for resume | Not implemented | `conversationRecovery.ts:548-550` (`copyPlanForResume`) | 缺失 |
| 16 | Context collapse commits/snapshots | Not implemented | `conversationRecovery.ts:568-570` (carries `contextCollapseCommits`, `contextCollapseSnapshot`) | 缺失 |
| 17 | Session start hooks on resume | Not implemented | `conversationRecovery.ts:568` (`processSessionStartHooks`) | 缺失 |
| 18 | Resume consistency check | Not implemented | `conversationRecovery.ts:556` (`checkResumeConsistency`) | 缺失 |
| 19 | Cross-directory resume via .jsonl path | `main.go:541-554` (accepts filename or number) | `conversationRecovery.ts:516-522` (`sourceJsonlFile` branch) | Go适配 |
| 20 | Live session skipping | Not implemented | `conversationRecovery.ts:494-515` (`listAllLiveSessions`, skips active bg/daemon) | 缺失 |

[diff_upstream/28-services.md §51.1]

---

## 7. File Read / Cache

| # | Aspect | Go | Upstream | Gap |
|---|--------|-----|----------|-----|
| 1 | Basic file reading with line numbers | `file_read.go:66-264` (cat -n format) | `fileRead.ts:100-102` (`readFileSync`) | Go适配 |
| 2 | Offset/limit partial reads | `file_read.go:130-164` | `fileRead.ts:75-98` (`readFileSyncWithMetadata`) | Go适配 |
| 3 | UTF-16 LE BOM detection | `file_read.go:191-194` | `fileRead.ts:33-34` (`detectEncodingForResolvedPath`) | Go适配 |
| 4 | UTF-8 BOM stripping | `file_read.go:200-202` | `fileRead.ts:37-43` | Go适配 |
| 5 | CRLF normalization | `file_read.go:198` | `fileRead.ts:94` (`replaceAll('\r\n', '\n')`) | Go适配 |
| 6 | Line ending detection (CRLF vs LF) | Not implemented | `fileRead.ts:51-66` (`detectLineEndingsForString`) | 缺失 |
| 7 | File encoding stored alongside content | Not stored | `fileRead.ts:75-98` — returns `{content, encoding, lineEndings}` | 缺失 |
| 8 | File read dedup | `file_read.go:174-182` (`FileUnchangedStub`) | `fileReadCache.ts:14-96` — mtime-based cache with LRU eviction | Go适配 |
| 9 | Read cache with mtime invalidation | `file_read.go:174-182` (single-entry per-file) | `fileReadCache.ts:14-96` (`FileReadCache` class, Map-based, max 1000 entries) | 简化 |
| 10 | FileStateCache with LRUCache | Registry tracks `(epoch, mtime)` per file (`context.go:86-89`) | `fileStateCache.ts:30-137` (`FileStateCache` with LRUCache, 100 entries, 25MB size limit) | 简化 |
| 11 | Cache serialization/dump/load | Not implemented | `fileStateCache.ts:97-103` (`dump()`, `load()`) | 缺失 |
| 12 | Cache merge (two caches) | Not implemented | `fileStateCache.ts:140-153` (`mergeFileStateCaches`) | 缺失 |
| 13 | Cache clone | Not implemented | `fileStateCache.ts:133-137` (`cloneFileStateCache`) | 缺失 |
| 14 | isPartialView tracking | `isPartial` flag in Registry (`file_read.go:254-260`) | `fileStateCache.ts:9-14` (`isPartialView` in `FileState` type) | Go适配 |
| 15 | Path normalization for cache keys | `filepath.Abs` in `context.go:126` | `fileStateCache.ts:52-67` (`normalize(key)` via `path.normalize`) | Go适配 |
| 16 | Symlink handling | Not explicitly handled | `fileRead.ts:83-85` (logs symlink resolution) | 简化 |
| 17 | Binary file rejection | `file_read.go:103-108` (extension) + `113-123` (magic bytes) | Extension check + magic bytes in separate module | Go增强 |
| 18 | File size limit (256KB full read) | `file_read.go:13, 168-170` | Upstream: configurable via settings | 简化 |

[diff_upstream/28-services.md §51.2]

---

## 8. Diff Rendering

| # | Aspect | Go | Upstream | Gap |
|---|--------|----|----------|-----|
| 1 | Structured patch generation | Not implemented (direct string replacement) | `diff.ts:81-114` (`getPatchFromContents` using `structuredPatch` from `diff` library) | 缺失 |
| 2 | Display patch with edits applied | Not implemented | `diff.ts:128-177` (`getPatchForDisplay`) | 缺失 |
| 3 | Context lines (3-line) | Not implemented | `diff.ts:9` (`CONTEXT_LINES = 3`) | 缺失 |
| 4 | Diff timeout | Not implemented | `diff.ts:10` (`DIFF_TIMEOUT_MS = 5000`) | 缺失 |
| 5 | Line change counting | Not implemented | `diff.ts:49-79` (`countLinesChanged`) | 缺失 |
| 6 | Hunk line number adjustment | Not implemented | `diff.ts:17-27` (`adjustHunkLineNumbers`) | 缺失 |
| 7 | Whitespace-ignoring diff | Not implemented | `diff.ts:85, 91` (`ignoreWhitespace` option) | 缺失 |
| 8 | Single-hunk mode | Not implemented | `diff.ts:86, 103` (`singleHunk` option) | 缺失 |
| 9 | Ampersand/dollar escaping for diff | Not implemented | `diff.ts:31-41` (`escapeForDiff`, `unescapeFromDiff`) | 缺失 |
| 10 | Leading tabs-to-spaces conversion | Not implemented | `diff.ts:140` (`convertLeadingTabsToSpaces`) | 缺失 |
| 11 | Analytics integration (LOC counter, events) | Not implemented | `diff.ts:70-78` (`addToTotalLinesChanged`, `logEvent`) | 缺失 |

[diff_upstream/28-services.md §51.3]

---

## 9. Context Analysis

| # | Aspect | Go | Upstream | Gap |
|---|--------|----|----------|-----|
| 1 | Context window resolution | Uses model token limits from SDK | `context.ts:56-103` (`getContextWindowForModel`) with 1M detection, beta flags | 简化 |
| 2 | 1M context support detection | Not implemented | `context.ts:36-54` (`has1mContext`, `modelSupports1M`) | 缺失 |
| 3 | Context override env var | Not implemented | `context.ts:64-71` (`CLAUDE_CODE_MAX_CONTEXT_TOKENS`) | 缺失 |
| 4 | Context usage percentage | Go: estimated tokens via `EstimatedTokens()` (`context.go:309-369`) | `context.ts:123-155` (`calculateContextPercentages`) | Go适配 |
| 5 | Max output tokens per model | Hardcoded or SDK default | `context.ts:160-224` (`getModelMaxOutputTokens`) with per-model defaults | 简化 |
| 6 | Thinking budget calculation | Not implemented | `context.ts:227-235` (`getMaxThinkingTokensForModel`) | 缺失 |
| 7 | 1M context disable flag | Not implemented | `context.ts:32-34` (`is1mContextDisabled` via `CLAUDE_CODE_DISABLE_1M_CONTEXT`) | 缺失 |
| 8 | Sonnet 1M experiment treatment | Not applicable | `context.ts:93-117` (`getSonnet1mExpTreatmentEnabled`) | 缺失 |

[diff_upstream/28-services.md §51.4]

---

## 10. Idle Timeout & Graceful Shutdown

### 10.1 Idle Timeout

| # | Aspect | Go | Upstream | Gap |
|---|--------|-----|----------|-----|
| 1 | SDK-mode idle auto-exit | Not implemented | `idleTimeout.ts:11-53` (`createIdleTimeoutManager` with `CLAUDE_CODE_EXIT_AFTER_STOP_DELAY`) | 缺失 |
| 2 | Idle detection function | Go: todo list idle reminder nudges model | `idleTimeout.ts:8` (`isIdle: () => boolean` callback) | Go适配 |
| 3 | Configurable idle delay | Not implemented | `idleTimeout.ts:16-18` (parses `CLAUDE_CODE_EXIT_AFTER_STOP_DELAY` env var) | 缺失 |
| 4 | Graceful shutdown on idle exit | Go: Ctrl+C handler in `main.go` | `idleTimeout.ts:40` (`gracefulShutdownSync()`) | 简化 |
| 5 | Streaming stall detection watchdog | `streaming.go:787-823` (goroutine-based) | Upstream: watchdog in `claude.ts` with `CLAUDE_ENABLE_STREAM_WATCHDOG` env | Go适配 |
| 6 | TodoWrite idle reminder | `agent_loop.go:1003-1011`, `todo_write.go:75-77` (`BuildIdleReminder`) | Upstream: separate TodoList module with verification nudge | Go增强 |

### 10.2 Graceful Shutdown

| # | Aspect | Go | Upstream | Gap |
|---|--------|----|----------|-----|
| 1 | Signal handling (SIGINT) | `main.go:197-218` (SIGINT via `signal.Notify`, Ctrl+C double-press exit) | `gracefulShutdown.ts:239-250` (`process.on('SIGINT')`) | Go适配 |
| 2 | SIGTERM handling | Not implemented | `gracefulShutdown.ts:251-254` (`process.on('SIGTERM')`, exit code 143) | 缺失 |
| 3 | SIGHUP handling | Not implemented | `gracefulShutdown.ts:255-259` (`process.on('SIGHUP')`, exit code 129) | 缺失 |
| 4 | Orphaned process detection | Not implemented | `gracefulShutdown.ts:264-279` (30s interval check on stdin/stdout writability) | 缺失 |
| 5 | Terminal mode cleanup | Not implemented | `gracefulShutdown.ts:42-119` (`cleanupTerminalModes`: Kitty keyboard, bracketed paste, cursor, alt screen, iTerm2 progress) | 缺失 |
| 6 | Resume hint printing | `main.go:488-498` (`printResumeHint`) | `gracefulShutdown.ts:127-167` (`printResumeHint` with session ID/title) | Go适配 |
| 7 | Session persistence on shutdown | Transcript flushed via `agent.Close()` | `gracefulShutdown.ts:426-450` (`runCleanupFunctions` with 2s timeout) | 简化 |
| 8 | SessionEnd hooks execution | Not implemented | `gracefulShutdown.ts:455-463` (`executeSessionEndHooks` with budget) | 缺失 |
| 9 | Analytics flush on shutdown | Not implemented | `gracefulShutdown.ts:487-494` (Datadog, 1P event logger, Sentry, 500ms cap) | 缺失 |
| 10 | Failsafe timer | Not implemented | `gracefulShutdown.ts:397-409` (`Math.max(5000, sessionEndTimeoutMs + 3500)`) | 缺失 |
| 11 | Force exit with fallback | `os.Exit(0)` on double Ctrl+C | `gracefulShutdown.ts:176-215` (`forceExit`: process.exit → SIGKILL fallback) | 简化 |
| 12 | Uncaught exception handler | Not implemented | `gracefulShutdown.ts:284-293` (`process.on('uncaughtException')`) | 缺失 |
| 13 | Unhandled rejection handler | Not implemented | `gracefulShutdown.ts:296-316` (`process.on('unhandledRejection')`) | 缺失 |
| 14 | Shutdown state machine | Simple atomic `SetInterrupted` flag | `gracefulShutdown.ts:344-347` (`shutdownInProgress`, `pendingShutdown`, `failsafeTimer`) | 简化 |
| 15 | Cache eviction hint on shutdown | Not implemented | `gracefulShutdown.ts:474-482` (`tengu_cache_eviction_hint` event) | 缺失 |
| 16 | Sync shutdown variant | Not implemented | `gracefulShutdown.ts:319-342` (`gracefulShutdownSync`) | 缺失 |

[diff_upstream/28-services.md §51.5, §51.6]

---

## 11. Cleanup & Cron

### 11.1 Cleanup

| # | Aspect | Go | Upstream | Gap |
|---|--------|----|----------|-----|
| 1 | Cleanup function registry | Not implemented | `cleanupRegistry.ts:7-25` (`registerCleanup`, `runCleanupFunctions` Set-based registry) | 缺失 |
| 2 | Periodic message file cleanup | Not implemented | `cleanup.ts:93-132` (`cleanupOldMessageFiles`) | 缺失 |
| 3 | Session file cleanup (30-day cutoff) | Not implemented | `cleanup.ts:155-258` (`cleanupOldSessionFiles`) | 缺失 |
| 4 | MCP log cleanup | Not implemented | `cleanup.ts:102-129` (mcp-logs-* directories) | 缺失 |
| 5 | Plan file cleanup | Not implemented | `cleanup.ts:300-303` (`cleanupOldPlanFiles`) | 缺失 |
| 6 | File history backup cleanup | Not implemented | `cleanup.ts:305-348` (`cleanupOldFileHistoryBackups`) | 缺失 |
| 7 | Session env dir cleanup | Not implemented | `cleanup.ts:350-388` (`cleanupOldSessionEnvDirs`) | 缺失 |
| 8 | Debug log cleanup | Not implemented | `cleanup.ts:396-429` (`cleanupOldDebugLogs`, preserves 'latest' symlink) | 缺失 |
| 9 | NPM cache cleanup (Anthropic packages) | Not implemented | `cleanup.ts:438-535` (`cleanupNpmCacheForAnthropicPackages`, once/day with lock) | 缺失 |
| 10 | Version cleanup (throttled) | Not implemented | `cleanup.ts:543-573` (`cleanupOldVersionsThrottled`) | 缺失 |
| 11 | Image cache cleanup | Not implemented | `cleanup.ts:593` (`cleanupOldImageCaches`) | 缺失 |
| 12 | Paste store cleanup | Not implemented | `cleanup.ts:594` (`cleanupOldPastes`) | 缺失 |
| 13 | Stale agent worktree cleanup | Not implemented | `cleanup.ts:595-598` (`cleanupStaleAgentWorktrees`) | 缺失 |
| 14 | Configurable cleanup period | Not implemented | `cleanup.ts:23-31` (`cleanupPeriodDays` setting, default 30) | 缺失 |
| 15 | Post-compact cleanup | `agent_loop.go:3077-3087`, `compact.go:2863-2866` | upstream `postCompactCleanup.ts` | Go适配 |

### 11.2 Cron/Scheduling

| # | Aspect | Go | Upstream | Gap |
|---|--------|----|----------|-----|
| 1 | Cron expression parsing | Not implemented | `cron.ts:83-101` (`parseCronExpression` for 5-field cron) | 缺失 |
| 2 | Next cron run computation | Not implemented | `cron.ts:119-181` (`computeNextCronRun` with minute-by-minute walk, 366-day bound) | 缺失 |
| 3 | Cron-to-human readable | Not implemented | `cron.ts:218-308` (`cronToHuman`: "Every day at 3pm", "Weekdays at 9am") | 缺失 |
| 4 | Cron scheduler (start/stop) | Not implemented | `cronScheduler.ts:142-531` (`createCronScheduler`) | 缺失 |
| 5 | File-backed cron tasks | Not implemented | `cronTasks.ts` (read/write `scheduled_tasks.json`) | 缺失 |
| 6 | Session-only cron tasks | Not implemented | bootstrap state `getSessionCronTasks` / `removeSessionCronTasks` | 缺失 |
| 7 | Missed task detection on startup | Not implemented | `cronScheduler.ts:195-227` (`findMissedTasks`, `buildMissedTaskNotification`) | 缺失 |
| 8 | Recurring task support | Not implemented | `cronScheduler.ts:315-324` (reschedule from `now` with jitter) | 缺失 |
| 9 | Jittered scheduling | Not implemented | `cronJitterConfig.ts` (configurable jitter window for load shedding) | 缺失 |
| 10 | Scheduler lock (multi-session) | Not implemented | `cronTasksLock.ts` (`tryAcquireSchedulerLock`, `releaseSchedulerLock`) | 缺失 |
| 11 | File watching for cron changes | Not implemented | `cronScheduler.ts:440-454` (chokidar watch on `scheduled_tasks.json`) | 缺失 |
| 12 | Task aging / expiration | Not implemented | `cronScheduler.ts:302-313` (`isRecurringTaskAged`, `recurringMaxAgeMs`) | 缺失 |

[diff_upstream/28-services.md §51.7, §51.8]

---

## 12. Output Formatting

| # | Aspect | Go | Upstream | Gap |
|---|--------|----|----------|-----|
| 1 | File size formatting (KB/MB/GB) | Not implemented (raw byte counts) | `format.ts:9-23` (`formatFileSize`) | 简化 |
| 2 | Duration formatting | `time.Duration` default `String()` (e.g., "2m30s") | `format.ts:34-95` (`formatDuration` with d/h/m/s breakdown, `hideTrailingZeros`, `mostSignificantOnly`) | 简化 |
| 3 | Short seconds formatting | Not implemented | `format.ts:30-32` (`formatSecondsShort`: "1.2s" with decimal) | 缺失 |
| 4 | Number formatting (compact notation) | Not implemented | `format.ts:124-131` (`formatNumber`: "1.3K", "900") | 缺失 |
| 5 | Token formatting | Direct `fmt.Sprintf("%d", count)` | `format.ts:133-135` (`formatTokens`: "1.3K") | 简化 |
| 6 | Relative time formatting | Not implemented | `format.ts:144-184` (`formatRelativeTime`: "5m ago", "in 2h") | 缺失 |
| 7 | Log metadata formatting | Custom formatting in REPL output | `format.ts:203-236` (`formatLogMetadata`: combines time, size, branch, tag, PR) | 简化 |
| 8 | Timezone-aware formatting | Not implemented | `format.ts:240, 286` (`showTimezone` option, `getTimeZone()`) | 缺失 |
| 9 | Intl.NumberFormat caching | Not implemented | `format.ts:97-122` (caches formatters for performance) | 缺失 |

[diff_upstream/28-services.md §51.9]

---

## 13. Shell Integration

| # | Aspect | Go | Upstream | Gap |
|---|--------|----|----------|-----|
| 1 | Basic command execution | `exec_tool.go:244-487` (`execToolExecute` via `exec.Command`) | `Shell.ts:181-442` (`exec` via `spawn`) | Go适配 |
| 2 | Shell detection/selection | `exec_tool.go:267-279` (PowerShell > bash > cmd on Windows) | `Shell.ts:73-137` (`findSuitableShell`: zsh/bash detection with `which`, `CLAUDE_CODE_SHELL` override) | 简化 |
| 3 | Shell provider abstraction | Not implemented (hardcodes shell path per OS) | `Shell.ts:139-159` (`getShellConfig`, `createBashShellProvider`, `createPowerShellProvider`) | 简化 |
| 4 | CWD tracking via `pwd -P` | `exec_tool.go:282-287` (uses `os.Getwd()` or `working_dir` param) | `Shell.ts:385-421` (writes cwd to temp file via `pwd -P > path`, reads back, updates state) | 简化 |
| 5 | CWD recovery when directory deleted | Not implemented | `Shell.ts:222-238` (realpath check, fallback to originalCwd) | 缺失 |
| 6 | Sandbox support | Not implemented | `Shell.ts:259-273` (`SandboxManager.wrapWithSandbox`, bwrap on Linux) | 缺失 |
| 7 | Abort signal integration | `exec_tool.go:434-441` (Go context cancellation kills process) | `Shell.ts:241-243, 316` (AbortSignal passed to spawn, handled in wrapSpawn) | Go适配 |
| 8 | Process group management | `exec_tool_unix.go:10-16` (setpgid for Unix), `exec_tool_windows.go` (no-op) | `Shell.ts:334` (`detached: provider.detached`, tree-kill in wrapSpawn) | Go适配 |
| 9 | Output file-based streaming | `exec_tool.go:301-334` (stdout/stderr pipes, readLimited) | `Shell.ts:289-313` (O_APPEND file fd for stdout+stderr interleaving) | Go适配 |
| 10 | Pipe mode (real-time stdout callbacks) | Not implemented | `Shell.ts:284-368` (`onStdout` callback via pipe mode) | 缺失 |
| 11 | Timeout handling | 2-min default, 10-min max (`exec_tool.go:251-265`) | `DEFAULT_TIMEOUT = 30 * 60 * 1000` (`Shell.ts:44`) | Go适配 |
| 12 | Environment snapshot | Inherited env + `subprocessEnv()` pattern | `Shell.ts:317-328` (`subprocessEnv()`, CLAUDEDECODE, SHELL, GIT_EDITOR) | Go适配 |
| 13 | Windows console window hiding | Not implemented | `Shell.ts:336` (`windowsHide: true`) | 缺失 |
| 14 | Session ID injection in env | Not implemented | `Shell.ts:325` (`CLAUDE_CODE_SESSION_ID`) | 缺失 |
| 15 | Auto-backgrounding on timeout | `exec_tool.go:352-433` (`TimeoutCallback` registers as background task) | `Shell.ts:385-421` (result.then for cwd update, cleanup) | Go增强 |
| 16 | Compound command splitting | `exec_tool.go:1247-1311` (`splitCompoundCommand`) | Upstream: handled in shell provider | Go适配 |
| 17 | Safe wrapper stripping | `exec_tool.go:1337-1421` (`stripSafeWrappers`: timeout, nice, sudo, env) | Upstream: in ShellCommand.ts | Go适配 |
| 18 | Read-only command detection | `exec_tool.go:1437-1544` (`isReadOnlyCommand`) | Upstream: in ShellCommand.ts | Go增强 |
| 19 | Destructive command warnings | `exec_tool.go:1547-1621` (`isDestructiveCommand`) | Upstream: in ShellCommand.ts | Go适配 |
| 20 | Permission check (deny patterns) | `exec_tool.go:116-194` (deny patterns, UNC, command substitution, expansion, paths) | Shell.ts: permission checks via ShellCommand.ts | Go增强 |
| 21 | Command substitution detection | `exec_tool.go:619-680` (`detectCommandSubstitution`) | Upstream: in ShellCommand.ts | Go增强 |
| 22 | Glob/brace expansion detection | `exec_tool.go:710-790` (`detectExpansion`) | Upstream: in ShellCommand.ts | Go增强 |
| 23 | Redirect target validation | `exec_tool.go:954-1089` (`validateRedirectTargets`) | Upstream: in ShellCommand.ts | Go增强 |
| 24 | CWD change analytics | Not implemented | `Shell.ts:467-473` (`logEvent('tengu_shell_set_cwd')`) | 缺失 |
| 25 | Session env cache invalidation | Not implemented | `Shell.ts:408` (`invalidateSessionEnvCache()`) | 缺失 |

[diff_upstream/28-services.md §51.10]

---

## 14. Langfuse Tracing

| # | Aspect | Go | Upstream | Gap |
|---|--------|----|----------|-----|
| 1 | Langfuse OpenTelemetry initialization | N/A | `src/services/langfuse/client.ts:1-72` | 缺失 |
| 2 | Trace creation (agent-run) | N/A | `src/services/langfuse/tracing.ts:18-52` | 缺失 |
| 3 | LLM observation recording (model, tokens, cost) | N/A | `src/services/langfuse/tracing.ts:64-139` | 缺失 |
| 4 | Tool observation recording (input/output/error) | N/A | `src/services/langfuse/tracing.ts:141-195` | 缺失 |
| 5 | Tool batch span for concurrent tools | N/A | `src/services/langfuse/tracing.ts:197-200` | 缺失 |
| 6 | Sub-agent trace creation | N/A | `src/services/langfuse/index.ts:2` | 缺失 |
| 7 | PII sanitization (tool input/output/global) | N/A | `src/services/langfuse/sanitize.ts` | 缺失 |
| 8 | Message conversion to Langfuse format | N/A | `src/services/langfuse/convert.ts` | 缺失 |
| 9 | Graceful shutdown with forceFlush | N/A | `src/services/langfuse/client.ts:60-72` | 缺失 |
| 10 | Session ID / user ID propagation | N/A | `src/services/langfuse/tracing.ts:111-118` | 缺失 |

[diff_upstream/28-services.md §52.4]

---

## 15. Doctor Diagnostics

| # | Aspect | Go | Upstream | Gap |
|---|--------|----|----------|-----|
| 1 | Installation type detection (npm/bundled/native) | N/A | `src/utils/doctorDiagnostic.ts:86-148` | 缺失 |
| 2 | Version detection & validation | N/A | `src/utils/doctorDiagnostic.ts:1-200` | 缺失 |
| 3 | Multiple installation detection | N/A | `src/utils/doctorDiagnostic.ts:62` | 缺失 |
| 4 | Package manager detection (brew/winget/npm/etc) | N/A | `src/utils/doctorDiagnostic.ts:22-32` | 缺失 |
| 5 | Ripgrep status check (working/mode/path) | N/A | `src/utils/doctorDiagnostic.ts:34,66-70` | 缺失 |
| 6 | Auto-update permission check | N/A | `src/utils/doctorDiagnostic.ts:5-60` | 缺失 |
| 7 | Shell config path detection | N/A | `src/utils/doctorDiagnostic.ts:39-42` | 缺失 |
| 8 | Warning generation with fix suggestions | N/A | `src/utils/doctorDiagnostic.ts:63` | 缺失 |
| 9 | Doctor command UI (React JSX) | N/A | `src/commands/doctor/doctor.tsx:1-7` | 缺失 |
| 10 | DISABLE_DOCTOR_COMMAND env gate | N/A | `src/commands/doctor/index.ts:7` | 缺失 |

[diff_upstream/28-services.md §52.6]

---

## 16. Bridge/Teleport

| # | Aspect | Go | Upstream | Gap |
|---|--------|----|----------|-----|
| 1 | Remote session bridge protocol | N/A | `src/bridge/bridgeMain.ts:141-200` | 缺失 |
| 2 | Bridge API client (work polling, heartbeats) | N/A | `src/bridge/bridgeApi.ts` | 缺失 |
| 3 | Session spawner (child claude processes) | `agent_loop.go` (local sub-agents only) | `src/bridge/sessionRunner.ts` | 缺失 |
| 4 | Worktree creation for sessions | N/A | `src/bridge/bridgeMain.ts:24` | 缺失 |
| 5 | JWT token refresh scheduler | N/A | `src/bridge/jwtUtils.ts:36` | 缺失 |
| 6 | Capacity wake (session completion signaling) | N/A | `src/bridge/capacityWake.ts:34` | 缺失 |
| 7 | Multi-session spawn (up to 32 parallel) | N/A | `src/bridge/bridgeMain.ts:85-99` | 缺失 |
| 8 | Poll loop with backoff configuration | N/A | `src/bridge/bridgeMain.ts:61-81,109` | 缺失 |
| 9 | Permission callback relay | N/A | `src/bridge/bridgePermissionCallbacks.ts` | 缺失 |
| 10 | Session title management | N/A | `src/bridge/bridgeMain.ts:190` | 缺失 |
| 11 | Bridge UI / status display | N/A | `src/bridge/bridgeUI.ts:33` | 缺失 |
| 12 | Inbound message/attachment handling | N/A | `src/bridge/inboundMessages.ts`, `src/bridge/inboundAttachments.ts` | 缺失 |
| 13 | Remote interrupt handling | N/A | `src/bridge/remoteInterruptHandling.ts` | 缺失 |
| 14 | ReplBridge (local REPL to bridge) | N/A | `src/bridge/replBridge.ts` | 缺失 |
| 15 | Flush gate (output batching) | N/A | `src/bridge/flushGate.ts` | 缺失 |

[diff_upstream/28-services.md §52.7]

---

## 17. Sentry Error Reporting

| # | Aspect | Go | Upstream | Gap |
|---|--------|----|----------|-----|
| 1 | Sentry SDK initialization (DSN-based) | N/A | `src/utils/sentry.ts:19-85` | 缺失 |
| 2 | Exception capture with context | N/A | `src/utils/sentry.ts:91-106` | 缺失 |
| 3 | Tag setting for error grouping | N/A | `src/utils/sentry.ts:112-122` | 缺失 |
| 4 | User context for error attribution | N/A | `src/utils/sentry.ts:128-138` | 缺失 |
| 5 | Sensitive header stripping (auth, cookies) | N/A | `src/utils/sentry.ts:43-58` | 缺失 |
| 6 | Ignored error patterns (ECONNREFUSED, aborts) | N/A | `src/utils/sentry.ts:64-75` | 缺失 |
| 7 | Graceful shutdown with flush | N/A | `src/utils/sentry.ts:144-155` | 缺失 |
| 8 | Performance transaction filtering | N/A | `src/utils/sentry.ts:77-80` | 缺失 |

[diff_upstream/28-services.md §52.8]

---

## 18. API Service Deep Dive

| # | Aspect | Go | Upstream | Gap |
|---|--------|----|----------|-----|
| 1 | Anthropic SDK streaming client | `agent_loop.go:350-450` | `src/services/api/claude.ts:771-799` | 简化 |
| 2 | Message construction & tool schema | `agent_loop.go:500-600` | `src/services/api/claude.ts:579-700` | 简化 |
| 3 | Chunk-type streaming dispatch | `streaming.go:125-176` | `src/services/api/claude.ts:836-895` | 适配 |
| 4 | CollectHandler for assembling streamed deltas | `streaming.go:59-115` | `src/services/api/claude.ts:1-100` (message types) | 适配 |
| 5 | TerminalHandler with [THINK] display | `streaming.go:317-467` | `src/services/api/claude.ts:1-100` | Go适配 |
| 6 | StreamBus pub/sub for chunk events | `streaming.go:568-621` | N/A (upstream uses generators) | Go增强 |
| 7 | StreamProgress TTFB/throughput metrics | `streaming.go:628-681` | `src/services/api/claude.ts:1-100` | Go适配 |
| 8 | DeltasState for retry safety | `streaming.go:687-700` | `src/services/api/claude.ts:1-100` | 适配 |
| 9 | Stall detection with dynamic timeouts | `streaming.go:733-823` | `src/services/api/claude.ts:1-100` | Go适配 |
| 10 | Beta header management | `prompt_caching.go:1-200` (basic scope) | `src/services/api/claude.ts:140-146` | 简化 |
| 11 | Model fallback on error | N/A (Go has no model fallback) | `src/services/api/claude.ts:700-701` (`fallbackModel`) | 缺失 |
| 12 | Advisory/advisor tool integration | N/A | `src/services/api/claude.ts:153-158` | 缺失 |
| 13 | Task budgets (`output_config.task_budget`) | N/A | `src/services/api/claude.ts:471-493` | 缺失 |
| 14 | Effort params configuration | N/A | `src/services/api/claude.ts:432-458` | 缺失 |
| 15 | Fast mode beta header | N/A | `src/services/api/claude.ts:141,174-178` | 缺失 |
| 16 | 1h prompt cache TTL with GrowthBook gating | `prompt_caching.go:1-200` (basic scope) | `src/services/api/claude.ts:368-426` | 简化 |
| 17 | Tool deferral (LSP pending init) | N/A | `src/services/api/claude.ts:805-812` | 缺失 |
| 18 | Message normalization for API | `normalize.go:1-200` | `src/services/api/claude.ts:74-94` | 适配 |
| 19 | Tool result pairing enforcement | N/A | `src/services/api/claude.ts:79` | 缺失 |
| 20 | Cost calculation per request | N/A | `src/services/api/claude.ts:182` | 缺失 |
| 21 | Session activity tracking (start/stop) | N/A | `src/services/api/claude.ts:211-213` | 缺失 |
| 22 | Fingerprint computation for requests | N/A | `src/services/api/claude.ts:74` | 缺失 |
| 23 | Extra body params (`CLAUDE_CODE_EXTRA_BODY`) | N/A | `src/services/api/claude.ts:278-323` | 缺失 |
| 24 | Cache break detection | N/A | `src/services/api/claude.ts:254-256` | 缺失 |
| 25 | withRetry with 529 handling | `retry_utils.go:1-80` | `src/services/api/claude.ts:257-263` | 简化 |
| 26 | Non-streaming fallback path | N/A | `src/services/api/claude.ts:837-899` | 缺失 |
| 27 | Bedrock adapter integration | N/A | `src/services/api/claude.ts:104-105` | 缺失 |
| 28 | MCP instructions delta support | N/A | `src/services/api/claude.ts:181` | 缺失 |
| 29 | Deferred tools support | N/A | `src/services/api/claude.ts:190-193` | 缺失 |
| 30 | API metadata (`user_id`, `session_id`, `account_uuid`) | N/A | `src/services/api/claude.ts:495-520` | 缺失 |
| 31 | Client request ID header tracking | N/A | `src/services/api/claude.ts:237` | 缺失 |
| 32 | Cache editing / AFK mode / thinking clear latches | N/A | `src/services/api/claude.ts:120-134` | 缺失 |
| 33 | Structured outputs beta header | N/A | `src/services/api/claude.ts:144,163` | 缺失 |
| 34 | Context management beta header | N/A | `src/services/api/claude.ts:139` | 缺失 |

[diff_upstream/28-services.md §52.1]

---

## Cross-References

- API client auth: [04-api-client.md](04-api-client.md) §1
- Permission scoping: [03-system-prompt.md](03-system-prompt.md) §3
- Feature flag usage in compaction: [01-core-agent-loop.md](01-core-agent-loop.md) §3
