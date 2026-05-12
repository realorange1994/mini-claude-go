# P0 — Critical Gaps

> Must-fix: capability-breaking or security-critical gaps
> Updated: 2026-05-12 (Audit Round 5-8 results applied)

These gaps cause API errors, incorrect behavior, or security vulnerabilities. Fix before any feature work.

---

## P0-1: Tool Pairing Validation [DONE — AUDIT: PARTIAL]

| Field | Value |
|-------|-------|
| Gap type | 缺失 |
| Severity | CRITICAL |
| Source | 01-core-agent-loop.md §A.2 |
| Round | 1 Committed (PARTIAL) |
| Upstream | `src/utils/messages.ts` — `ensureToolResultPairing()` |
| REPL | N/A — core agent logic |

**Audit note**: Go has `EnsureToolResultPairing` in `normalize.go` but upstream's defense is layered: JSONL-format transcript building provides injection resistance, while Go uses text-based dedup. Upstream dedup logic spans multiple functions (`ensureToolResultPairing`, `normalizeMessagesForAPI`).

---

## P0-2: Role Alternation Enforcement [DONE — AUDIT: PARTIAL]

| Field | Value |
|-------|-------|
| Gap type | 缺失 |
| Severity | CRITICAL |
| Source | 01-core-agent-loop.md §A.1 |
| Round | 1 Committed (PARTIAL) |
| Upstream | `src/utils/messages.ts` — `normalizeMessagesForAPI()` role alternation logic |
| REPL | N/A — core agent logic |

**Audit note**: Go has `EnforceRoleAlternation` in `normalize.go`. Upstream handles this as part of `normalizeMessagesForAPI()` which merges consecutive user/assistant messages. Go's approach is standalone function — conceptually similar but not integrated into same pipeline.

---

## P0-3: Empty Message Filtering [DONE — AUDIT: PARTIAL]

| Field | Value |
|-------|-------|
| Gap type | 缺失 |
| Severity | HIGH |
| Source | 01-core-agent-loop.md §A.3 |
| Round | 1 Committed (PARTIAL) |
| Upstream | `src/utils/messages.ts` — empty message strip in `normalizeMessagesForAPI()` |
| REPL | N/A — core agent logic |

**Audit note**: Go has `FilterEmptyMessages` in `normalize.go`. Upstream strips empty messages as part of its normalization pipeline. Go's implementation is separate — covers whitespace-only, orphaned thinking, trailing thinking. May need to check if upstream has additional edge cases.

---

## P0-4: Cache Breakpoint Optimization (4→1) [DONE — AUDIT: PARTIAL]

| Field | Value |
|-------|-------|
| Gap type | 简化 |
| Severity | HIGH |
| Source | 03-system-prompt.md §A.3 |
| Round | 4 Committed (PARTIAL) |
| Upstream | `src/services/api/claude.ts` — `addCacheBreakpoints()`, breakpoint insertion logic |
| REPL | N/A — core agent logic |

**Audit note**: Go changed from 4→1 breakpoint, added `CacheBreakpointConfig`. Upstream's breakpoint logic is more nuanced: it dynamically places breakpoints based on message count and type. Go's `maxBP` is always 1, making the system prompt breakpoint code in `prompt_caching.go` unreachable dead code.

---

## P0-5: Classifier Fail-Closed on Stage 2 [DONE — AUDIT: PARTIAL]

| Field | Value |
|-------|-------|
| Gap type | Bug |
| Severity | HIGH (security) |
| Source | 03-system-prompt.md §C.5 |
| Round | 3 Committed (PARTIAL) |
| Upstream | `src/utils/permissions/yoloClassifier.ts` — JSONL format, fail-closed |
| REPL | N/A — core agent logic |

**Audit note**: Fabricated `escapeContentInjection` removed. Replaced with upstream-matching architectural defense: (1) assistant text exclusion from transcript, (2) `<transcript>` XML tag wrapping, (3) JSONL transcript option via `BuildJSONLTranscript()`. Fail-closed logic confirmed correct: classifier blocks on stage 2 errors.

---

## P0-6: Multi-Edit Multiple Match Check [DONE — AUDIT: PASS]

| Field | Value |
|-------|-------|
| Gap type | Bug |
| Severity | HIGH |
| Source | 02-tools.md §C.2 |
| Round | 2 Committed (PASS) |
| Upstream | `src/tools/edit.ts` — multiple occurrence detection |
| REPL | N/A — core tool logic |

**Audit note**: Go's `countOccurrences()` and multiple match error when `replace_all=false` correctly matches upstream behavior.

---

## P0-7: Context Window Per-Model Resolution [DONE — AUDIT: PARTIAL]

| Field | Value |
|-------|-------|
| Gap type | 缺失 |
| Severity | HIGH |
| Source | 04-api-client.md §A.1 |
| Round | 9 Committed (PARTIAL) |
| Upstream | Model capabilities via API, model.ts |
| REPL | N/A — core agent logic |

**Audit note**: Go has `ModelCapabilitiesCache` with `/v1/models` fetching. Correct model IDs should be verified (upstream uses `claude-opus-4-5-20251101`, `claude-haiku-4-5-20251001`).

---

## P0-8: Post-Compact Memory Loss [DONE — AUDIT: PARTIAL]

| Field | Value |
|-------|-------|
| Gap type | 缺失 |
| Severity | CRITICAL |
| Source | 01-core-agent-loop.md §B.3, 03-compact §B |
| Status | DONE |
| Affected files | `agent_loop.go`, `compact.go` |
| Upstream | `src/services/compact/compact.ts:522-543` — `cacheToObject`, `createPostCompactFileAttachments`; `src/services/compact/buildPostCompactMessages.ts` |
| REPL | N/A — core agent logic |

**Audit note**: Go now has full snapshot-then-clear pattern via `PostCompactRecovery` and `buildPreCompactFileSnapshot`. File recovery uses `registry.GetCachedFileContent()` (cached content preferred over disk re-read), with budgets matching upstream (5 files, 5000 tokens/file, 50000 total). SM-compact preserves recent 10K-40K tokens via `KeepRecentMessagesAdaptive`. Conclusion extraction has 15 patterns. Structured metadata in `entriesToSummaryTextForMessagesParams` includes user previews (1000 chars), edit details, and error collection. Skill, plan, tools, MCP, agent, todo, and session memory recovery all implemented. **Remaining gap**: LLM compact prompt doesn't include "full code snippets" instruction (upstream prompt.ts:70), but this is minor since the LLM sees the actual conversation.

---

## P0-9: Streaming Tool Executor [DONE — AUDIT: PARTIAL]

| Field | Value |
|-------|-------|
| Gap type | 缺失 |
| Severity | CRITICAL |
| Source | 01-core-agent-loop.md §A.5 |
| Round | 19 Committed (PARTIAL) |
| Affected files | `streaming_executor.go`, `streaming.go`, `agent_loop.go` |
| Upstream | `src/services/tools/StreamingToolExecutor.ts`, `src/services/api/claude.ts` |
| REPL | N/A — core agent logic |

**Audit note**: Implemented pipelined tool execution via `StreamingToolExecutor`. When a tool's content block finishes during streaming (ChunkTypeBlockStop), the executor dispatches it immediately. Concurrency-safe tools (read_file, glob, grep, web_search, etc.) run in parallel; non-safe tools (exec, edit_file, write_file, etc.) are serialized. Results are collected in-order and injected into context after stream completion. The `CollectHandler` now has a `toolCallDoneCh` channel that emits tool call indices on block stop events. The `tryStreamOnce()` method accepts optional executor parameters. The Run loop creates an executor per turn and waits for pipelined results before falling back to synchronous execution. **Remaining gap**: Upstream's executor has more sophisticated features (Bash error cascading with sibling abort, discard mechanism for streaming fallback, progress message streaming, per-tool abort controllers). Go's implementation is simpler — no sibling abort, no discard, no progress streaming. These are P1-level refinements.

---

## P0-10: Orphaned Tool Result Backfill [DONE — AUDIT: PASS]

| Field | Value |
|-------|-------|
| Gap type | Bug |
| Severity | CRITICAL |
| Source | 01-core-agent-loop.md §A.6 |
| Round | 19 Committed (PASS) |
| Affected files | `context.go` |
| Upstream | Transcript resume logic, orphaned tool result detection |
| REPL | N/A — core agent logic |

**Audit note**: `ValidateToolPairing()` Pass 2 now backfills orphaned tool_results with synthetic tool_use blocks instead of discarding them. Fully-orphaned entries (all results lack matching tool_use) are preserved in-place with a synthetic `assistant` entry containing `ToolUseContent` injected before them, using `inferToolNameFromResult()` heuristics to name the tool. Mixed entries (valid + orphaned results) keep only valid results and drop orphans without backfill (consistent with upstream's approach of not fabricating context for partial orphans). All 3 `TestValidateToolPairing*` tests pass.

---

## P0-11: Stop Hooks [DONE — AUDIT: PASS]

| Field | Value |
|-------|-------|
| Gap type | 缺失 |
| Severity | CRITICAL |
| Source | 01-core-agent-loop.md §A.7 |
| Round | 18 Committed (PASS) |
| Affected files | `agent_loop.go`, `hooks.go` |
| Upstream | `src/utils/hooks.ts` — `HookStop` event type, fired at agent loop exit |
| REPL | N/A — core agent logic |

**Audit note**: `HookStop` constant added to `hooks.go`. `fireStopHook()` helper fires at all 7 exit points of `Run()`: cancelled_by_parent, interrupted (3 locations), stream_stalled, context_length_exceeded, and completed (normal exit). Metadata includes reason, model, turns, interrupted flag, and cumulative token counts. `IterationBudget.Consumed()` method added for turn count reporting.

---

## P0-12: Token Budget Tracking [DONE — AUDIT: PARTIAL]

| Field | Value |
|-------|-------|
| Gap type | 缺失 |
| Severity | HIGH |
| Source | 01-core-agent-loop.md §A.8, 04-api-client.md §A.3 |
| Round | 18 Committed (PARTIAL) |
| Affected files | `agent_loop.go`, `compact.go`, `streaming.go` |
| Upstream | `src/services/api/claude.ts` — usage tracking; `src/services/compact/` — budget calculation |
| REPL | N/A — core agent logic |

**Audit note**: Added `lastAPIInputTokens`/`lastAPIOutputTokens` to store exact token counts from the most recent API response. Added `totalCacheCreationTokens`/`totalCacheReadTokens` for separate cache token tracking. Added `RemainingTokenBudget()` method that prefers exact API input tokens over heuristic estimates. `fireStopHook` now includes all token fields. The compaction trigger (`Compactor.ShouldCompact()`) still uses content-type-aware heuristic estimation rather than exact API token counts — this is because compaction decisions happen BEFORE the next API call, so there are no exact tokens to use. Upstream uses a tokenizer library for per-message estimation; Go's `EstimateContentTokens()` with 4/3 safety margin is a reasonable approximation but not exact.

---

## P0-13: Permission Path Safety Fixes [DONE — AUDIT: PASS]

| Field | Value |
|-------|-------|
| Gap type | Bug |
| Severity | HIGH (security) |
| Source | 03-system-prompt.md §C.11 |
| Round | 18 Committed (PASS) |
| Affected files | `permissions/internal_paths.go` |
| Upstream | Path safety validation in permissions module |
| REPL | N/A — core agent logic |

**Audit note**: All `strings.Contains()` path checks replaced with precise `hasPathPrefix()` (HasPrefix + separator) and `hasPathComponent()` (full separator-bounded component match). Added `hasSuspiciousColon()` for Windows ADS attack prevention (allows drive-letter colon, rejects other colons). Added `isSymlinkEscape()` using `filepath.EvalSymlinks` for symlink escape detection. Both `IsInternalEditablePath` and `IsInternalReadablePath` now reject ADS colon paths. Path-specific fixes: `isInSessionMemoryDir`, `isInToolResultsDir`, `isInBundledSkillsDir` now require `~/.claude/projects/` or `/tmp/claude-*/` prefix + component match. `isAgentMemoryPath` requires `~/.claude/projects/` prefix. `isInProjectTempDir` validates first component after temp dir starts with `claude-`. 20+ unit tests added for path traversal edge cases.

---

## Summary Table

| # | Gap | Audit | Status | Effort |
|---|-----|-------|--------|--------|
| P0-1 | Tool pairing validation | PARTIAL | DONE | Large |
| P0-2 | Role alternation | PARTIAL | DONE | Medium |
| P0-3 | Empty message filtering | PARTIAL | DONE | Small |
| P0-4 | Cache breakpoint 4→1 | PARTIAL | DONE | Small |
| P0-5 | Classifier fail-closed | PARTIAL | DONE | Small |
| P0-6 | Multi-edit match check | PASS | DONE | Small |
| P0-7 | Per-model context window | PARTIAL | DONE | Medium |
| P0-8 | Post-compact memory loss | PARTIAL | DONE | Large |
| P0-9 | Streaming tool executor | PARTIAL | DONE | Large |
| P0-10 | Orphaned tool result backfill | PASS | DONE | Medium |
| P0-11 | Stop hooks | PASS | DONE | Small |
| P0-12 | Token budget tracking | PARTIAL | DONE | Medium |
| P0-13 | Permission path safety | PASS | DONE | Medium |

## Audit Legend

| Status | Meaning |
|--------|---------|
| **PASS** | Implementation matches upstream design |
| **PARTIAL** | Exists but incomplete or deviates from upstream |
| **FAIL** | Fabricated/stub code, needs rewrite |
| **—** | Not yet audited |
