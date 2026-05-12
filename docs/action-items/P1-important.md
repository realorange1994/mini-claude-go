# P1 — Important Gaps

> Should-fix: significant functionality gaps and quality improvements
> Updated: 2026-05-12 (Audit Round 5-16 results applied)

These gaps limit capabilities or cause degraded behavior but don't break core functionality.

---

## P1-1: Cost Tracking with Per-Model USD Pricing [DONE — AUDIT: PARTIAL]

| Field | Value |
|-------|-------|
| Gap type | 缺失 |
| Severity | HIGH |
| Source | 04-api-client.md §B.1 |
| Round | 5 Committed (PARTIAL) |
| Upstream | `src/utils/modelCost.ts` — model pricing tiers, cost calculation |
| REPL | N/A — core agent logic |

**Audit note**: Go has `CostTracker` with 6 pricing tiers. Upstream uses different model IDs: `claude-opus-4-5-20251101` vs Go's `claude-opus-4-5-20250610`. Model IDs need verification against current API.

---

## P1-2: Reactive Compaction with Token-Gap Parsing [DONE — AUDIT: PASS]

| Field | Value |
|-------|-------|
| Gap type | 缺失 |
| Severity | HIGH |
| Source | 01-core-agent-loop.md §B.1 |
| Round | 6 Committed (PASS) |
| Upstream | `src/services/compact/reactiveCompact.ts` — **stubbed in upstream** |
| REPL | N/A — core agent logic |

**Audit note**: Go's `reactiveCompact` with token-gap detection is actually **superior** to upstream since upstream stubs reactive compaction. Go correctly detects context overflow from API errors and triggers compaction.

---

## P1-3: 529 Model Fallback + 429 Subscriber Gating [DONE — AUDIT: PARTIAL]

| Field | Value |
|-------|-------|
| Gap type | 缺失 |
| Severity | HIGH |
| Source | 04-api-client.md §B.2 |
| Round | 7 Committed (PARTIAL) |
| Upstream | `src/services/api/withRetry.ts` — `FallbackTriggeredError`, 529/429 handling |
| REPL | N/A — core agent logic |

**Audit note**: Go has `FallbackTriggeredError`, `is529Error`, `shouldRetry429`. Upstream has more sophisticated retry logic with multiple fallback targets and subscriber-type detection from API responses.

---

## P1-4: Model Alias Resolution [DONE — AUDIT: PASS]

| Field | Value |
|-------|-------|
| Gap type | 缺失 |
| Severity | MEDIUM |
| Source | 04-api-client.md §B.3 |
| Round | 8 Rewritten (PASS) |
| Upstream | `src/utils/model/model.ts` — `parseUserSpecifiedModel()`, `[1m]` suffix handling |
| REPL | N/A — core agent logic |

**Audit note**: Rewritten to match upstream. Go now has full `[1m]` suffix support via `ResolveModelAlias()`, `has1mContext()`, `modelSupports1M()`, `GetModelForAPI()`, and beta header construction via `BuildBetaHeaders()` + `FormatBetaHeader()`. All API call sites use `GetModelForAPI()` to strip `[1m]` before sending to API. Legacy model remap and tier-based defaults implemented.

---

## P1-5: Cache Break Detection + Pinned Edits [DONE — AUDIT: PASS]

| Field | Value |
|-------|-------|
| Gap type | 缺失 |
| Severity | MEDIUM |
| Source | 03-system-prompt.md §A.4 |
| Round | 10 Rewritten (PASS) |
| Upstream | `src/services/api/promptCacheBreakDetection.ts` (670+ lines) — 12+ change categories; `src/services/compact/cachedMicrocompact.ts` — pinned cache edits |
| REPL | N/A — core agent logic |

**Audit note**: Rewritten to match upstream. (1) `CacheBreakDetector` now tracks 12 change categories (`CacheChangeToolResult`, `CacheChangeCompaction`, `CacheChangeSystemPrompt`, etc.) with per-category weights, replacing the simple 20% heuristic. (2) `ApplyPinnedCacheEdits` now actually sets `cache_control` on pinned tool_result blocks using `anthropic.NewCacheControlEphemeralParam()`, replacing the `_ = msg` stub. (3) `RecordChange()` API allows callers to track specific changes between API calls for category-based break prediction.

---

## P1-6: Classifier Improvements [DONE — AUDIT: PASS]

| Field | Value |
|-------|-------|
| Gap type | 简化 |
| Severity | MEDIUM |
| Source | 03-system-prompt.md §C |
| Round | 11 Rewritten (PASS) |
| Upstream | `src/utils/permissions/yoloClassifier.ts` — JSONL format, XML output, content injection defense |
| REPL | N/A — core agent logic |

**Audit note**: Fabricated `escapeContentInjection` removed. Replaced with proper architectural defense matching upstream: (1) assistant text exclusion (primary defense, matching yoloClassifier.ts:346-348), (2) `<transcript>` XML tag wrapping (matching yoloClassifier.ts:766-769), (3) `BuildJSONLTranscript()` for optional JSONL mode (matching yoloClassifier.ts:378-426). JSON escaping prevents role-spoofing.

---

## P1-7: Skill Content Pipeline [DONE — AUDIT: PARTIAL]

| Field | Value |
|-------|-------|
| Gap type | 缺失 |
| Severity | MEDIUM |
| Source | 03-system-prompt.md §D |
| Round | 12 Committed (PARTIAL) |
| Upstream | Skill expansion in prompt building |
| REPL | N/A — core agent logic |

**Audit note**: Go has `ExpandSkillContent`, `ExpandSkillVariables`, `MCPSkillDiscovery` in `skills/loader.go`. Basic structure is present but may be missing some variable expansion patterns and MCP skill discovery depth.

---

## P1-8: Hook System Expansion [DONE — AUDIT: PASS]

| Field | Value |
|-------|-------|
| Gap type | 缺失 |
| Severity | MEDIUM |
| Source | 03-system-prompt.md §E |
| Round | 13 Committed (PASS) |
| Upstream | `src/utils/hooks.ts` (~5200 lines) — full hook lifecycle |
| REPL | N/A — core agent logic |

**Audit note**: All 16 hook type constants are now invoked in the agent loop. Previously 9 were never called. Fixed by wiring: `HookPreToolUse`/`HookPostToolUse` (around `executeTool()`), `HookPreAssistantMessage`/`HookPostAssistantMessage` (around assistant response processing), `HookOnNotification` (in `InjectNotifications()`), `HookOnSubagent` (in `SpawnSubAgent()`), `HookOnFork` (in fork mode detection), `HookOnResume` (in `NewAgentLoopFromTranscript()`). Note: Go uses custom event names (e.g., `pre_tool_use` vs upstream's `PreToolUse`) — the naming convention differs but the semantics match.

---

## P1-9: Normalization Pipeline Enhancements [DONE — AUDIT: PARTIAL]

| Field | Value |
|-------|-------|
| Gap type | 缺失 |
| Severity | MEDIUM |
| Source | 01-core-agent-loop.md §A.4 |
| Round | 14 Committed (PARTIAL) |
| Upstream | `src/utils/messages.ts` — `normalizeMessagesForAPI()` |
| REPL | N/A — core agent logic |

**Audit note**: Go has `ReorderAttachmentsForAPI`, `ValidateImagesForAPI`, `StripImagesFromErrorToolResults`, `StripVirtualMessages` in `normalize.go`. Basic structure matches upstream but implementation details may differ.

---

## P1-10: Transcript DAG [DONE — AUDIT: PARTIAL]

| Field | Value |
|-------|-------|
| Gap type | 缺失 |
| Severity | MEDIUM |
| Source | 07-architecture.md §A.1 |
| Round | 15 Committed (PARTIAL) |
| Upstream | Transcript UUID, parent chain in conversation storage |
| REPL | N/A — core agent logic |

**Audit note**: Go has UUID, ParentUUID, Subtype, Metadata fields, `DetectInterruptType`. Basic DAG structure present but parent chain rewriting and metadata completeness may need verification.

---

## P1-11: Agent Tool Improvements [DONE — AUDIT: PARTIAL]

| Field | Value |
|-------|-------|
| Gap type | 缺失 |
| Severity | MEDIUM |
| Source | 02-tools.md §D |
| Round | 16 Committed (PARTIAL) |
| Upstream | `src/utils/agent.ts` — sync/async execution; `src/utils/worktree.ts` (1517 lines) — worktree isolation; handoff classifier |
| REPL | N/A — core agent logic |

**Audit note**: Go has `AgentModeSync`, `SpawnSyncFunc`, `SetupWorktree` (minimal), `ClassifyHandoff` (simple pattern matching). Upstream's worktree isolation is 1517 lines with full git worktree management. Go's is minimal. Handoff classifier uses simple patterns vs upstream's more sophisticated approach.

---

## P1-12: Post-Compact Recovery Chain [DONE — partial]

| Field | Value |
|-------|-------|
| Gap type | 简化 |
| Severity | MEDIUM |
| Source | 01-core-agent-loop.md §B.2 |
| Round | 6 Committed (partial) |
| Upstream | `src/services/compact/buildPostCompactMessages.ts` |
| REPL | N/A — core agent logic |

**Note**: Basic recovery chain exists but file attachment injection is incomplete. See P0-8 for full fix.

---

## P1-13: SM-Compact Token Retention [DONE — AUDIT: PASS]

| Field | Value |
|-------|-------|
| Gap type | 简化 |
| Severity | HIGH |
| Source | 01-core-agent-loop.md §B.4 |
| Affected files | `agent_loop.go` |
| Upstream | `src/services/compact/sessionMemoryCompact.ts:57-61` — `minTokens=10000`, `maxTokens=40000`, `minTextBlockMessages=5` |
| REPL | N/A — core agent logic |

**Audit note**: Go's `KeepRecentMessagesAdaptive(10_000, 5, 40_000)` matches upstream exactly.

---

## P1-14: LLM Compaction Summary Quality [DONE — AUDIT: PASS]

| Field | Value |
|-------|-------|
| Gap type | 简化 |
| Severity | HIGH |
| Source | 01-core-agent-loop.md §B.5 |
| Affected files | `compact.go` |
| Upstream | `src/services/compact/prompt.ts:70` — "include full code snippets where applicable"; 9-section structured output |
| REPL | N/A — core agent logic |

**Audit note**: Compaction prompt includes "include full code snippets where applicable", "Preserving code snippets, function signatures, and file edits", and 9-section structured output format matching upstream.

---

## P1-15: Non-LLM Compaction Metadata Enhancement [DONE — AUDIT: PASS]

| Field | Value |
|-------|-------|
| Gap type | 简化 |
| Severity | MEDIUM |
| Source | 01-core-agent-loop.md §B.6 |
| Affected files | `compact.go` |
| Upstream | Non-LLM compaction metadata in compact.ts |
| REPL | N/A — core agent logic |

**Audit note**: `entriesToSummaryTextForMessagesParams` includes: user message previews at 1000 chars, first 10 error messages in full, edit operation details, and structured conclusion extraction.

---

## P1-16: Tool Output Structured Format [DONE — AUDIT: PASS]

| Field | Value |
|-------|-------|
| Gap type | 缺失 |
| Severity | MEDIUM |
| Source | 02-tools.md §A.1 |
| Round | 20 Committed (PASS) |
| Affected files | `tools/base.go` |
| Upstream | `output_type: "text"|"error"` in tool result, `ToolResult` type |
| REPL | N/A — core tool logic |

**Audit note**: Go has `ToolResult` struct with `Output` (string), `IsError` (bool), and `Metadata` (ToolResultMetadata with ToolName, ExitCode, DurationMs, OutputLines, Truncated). When sent to the API, `IsError` maps to `anthropic.ToolResultBlockParam.IsError` which corresponds to upstream's `output_type: "error"`. Helper functions `ToolResultOK()` and `ToolResultError()` provide clean construction. Metadata supports tool-specific data like file paths and line counts.

---

## P1-17: Exec Tool Safety Improvements [DONE — AUDIT: PASS]

| Field | Value |
|-------|-------|
| Gap type | 简化 |
| Severity | MEDIUM |
| Source | 02-tools.md §A.2 |
| Round | 20 Committed (PASS) |
| Affected files | `tools/exec_tool.go` |
| Upstream | `src/tools/bash.ts` — timeout, output truncation, background process |
| REPL | REPL-relevant — exec tool is primary REPL interaction |

**Audit note**: All upstream safety features are implemented: (1) per-command timeout (2min default, configurable up to 10min), (2) working directory validation with expandPath, (3) output truncation at 30K chars with `[N lines truncated]` notice, (4) `run_in_background` parameter with BackgroundTaskCallback, (5) TimeoutCallback for auto-backgrounding timed-out processes, (6) command substitution detection (detectCommandSubstitution), (7) glob/brace expansion detection in destructive commands, (8) path validation for deletion commands, (9) redirect target validation, (10) UNC path blocking for SMB/WebDAV credential leakage, (11) deny regex patterns for destructive commands, (12) compound command splitting with quote awareness, (13) safe wrapper stripping, (14) safe variable whitelist for ${} expansion.

---

## P1-18: File Read Tool Enhancements [DONE — AUDIT: PARTIAL]

| Field | Value |
|-------|-------|
| Gap type | 简化 |
| Severity | MEDIUM |
| Source | 02-tools.md §A.3 |
| Round | 20 Committed (PARTIAL) |
| Affected files | `tools/file_read.go` |
| Upstream | `src/tools/read.ts` — image, PDF, notebook reading |
| REPL | REPL-relevant — file reading is primary REPL interaction |

**Audit note**: File read already has: line range (offset+limit), file size limit (256KB), auto line number display (cat -n format), notebook reading (.ipynb), binary file detection (magic bytes), read dedup (FileUnchangedStub). **Remaining gap**: upstream supports image reading as base64 content blocks and PDF reading with page ranges — Go rejects images and PDFs with clear error messages. Adding image/PDF support would require image processing libraries or external tools.

---

## P1-19: File Write Tool Safety [DONE — AUDIT: PARTIAL]

| Field | Value |
|-------|-------|
| Gap type | 简化 |
| Severity | MEDIUM |
| Source | 02-tools.md §A.4 |
| Round | 20 Committed (PARTIAL) |
| Affected files | `tools/file_write.go` |
| Upstream | `src/tools/write.ts` — must-read-first check |
| REPL | REPL-relevant — file writing is primary REPL interaction |

**Audit note**: Most upstream safety features implemented: (1) must-read-first check via `CheckFileStale()` (concurrent modification detection), (2) parent directory creation with `os.MkdirAll`, (3) file size limit (10MB), (4) UNC path blocking, (5) disk sync after write, (6) registry tracking via `MarkFileReadWithContent`. **Remaining gap**: upstream has write confirmation for files > 1MB (asks user before writing large files) — Go doesn't have this confirmation step.

---

## P1-20: Grep/Glob Tool Alignment [DONE — AUDIT: PARTIAL]

| Field | Value |
|-------|-------|
| Gap type | 简化 |
| Severity | MEDIUM |
| Source | 02-tools.md §A.5 |
| Round | 20 Committed (PARTIAL) |
| Affected files | `tools/grep_tool.go`, `tools/glob_tool.go` |
| Upstream | `src/tools/grep.ts`, `src/tools/glob.ts` — context lines, multiline, pagination |
| REPL | REPL-relevant — search tools are primary REPL interaction |

**Audit note**: Grep has: `-A`/`-B`/`-C` context lines, multiline mode, `head_limit`/`offset` pagination, `output_mode` (content/files_with_matches/count), `type` language filter, `glob` file filter, `max_depth`, `max_filesize`, case-insensitive, fixed_strings. Glob has: modification-time sorting, `head_limit`, `excludes` patterns. **Remaining gap**: Glob lacks `type` parameter for file type filtering (upstream has this). Minor gap.

---

## P1-21: Git Tool Enhancements

| Field | Value |
|-------|-------|
| Gap type | 简化 |
| Severity | MEDIUM |
| Source | 02-tools.md §A.6 |
| Status | NEW |
| Affected files | `tools/git_tool.go` |
| Upstream | `src/tools/bash.ts` — git commands; `src/utils/git.ts` — git utilities |
| REPL | REPL-relevant — git operations are primary REPL interaction |

**Problem**: Go's git tool is basic compared to upstream. Missing:
- `git diff` with staged/unstaged/branch comparison
- `git log` with format options
- `git blame` support
- `git stash` operations
- Branch management (create, switch, delete)
- PR creation via `gh` CLI

**Action items**:
1. Add `git diff` with staged/unstaged/branch comparison
2. Add `git log` with format options
3. Add `git blame` support
4. Add `git stash` operations
5. Add branch management commands
6. Add PR creation via `gh` CLI

---

## P1-22: Notebook Edit Tool

| Field | Value |
|-------|-------|
| Gap type | 缺失 |
| Severity | MEDIUM |
| Source | 02-tools.md §A.7 |
| Status | NEW |
| Affected files | New: `tools/notebook_tool.go` |
| Upstream | `src/tools/notebook.ts` — cell-level operations |
| REPL | N/A — tool implementation |

**Problem**: Upstream has a dedicated `NotebookEdit` tool for Jupyter notebooks (.ipynb). Go has no notebook support at all.

**Action items**:
1. Create `NotebookEdit` tool with cell-level operations
2. Support cell types: code, markdown
3. Support edit modes: replace, insert, delete
4. Support cell ID targeting

---

## P1-23: System Prompt Dynamic Sections

| Field | Value |
|-------|-------|
| Gap type | 简化 |
| Severity | MEDIUM |
| Source | 03-system-prompt.md §A.1 |
| Status | NEW |
| Affected files | `system_prompt.go` |
| Upstream | `src/services/claude.ts` — dynamic prompt building; `src/utils/systemPrompt.ts` |
| REPL | N/A — core agent logic |

**Problem**: Go's system prompt is mostly static. Upstream dynamically includes/excludes sections based on:
- Available tools (only list tools that are registered)
- Current permission mode (different instructions for ask/auto/plan)
- Active MCP servers (list available MCP tools)
- Active skills (list available skills)
- Git status (branch, dirty state)
- Project context (CLAUDE.md content)

**Action items**:
1. Add dynamic tool listing in system prompt
2. Add permission-mode-specific instructions
3. Add MCP server/tool listing
4. Add skill listing
5. Add git status section
6. Add CLAUDE.md content injection

---

## P1-24: Permission Rule Engine

| Field | Value |
|-------|-------|
| Gap type | 简化 |
| Severity | MEDIUM |
| Source | 03-system-prompt.md §C.1 |
| Status | NEW |
| Affected files | `permissions/` |
| Upstream | `src/utils/permissions/` — glob patterns, rule priority, settings hierarchy |
| REPL | N/A — core agent logic |

**Problem**: Go's permission system uses simple allow/deny lists. Upstream has a full rule engine with:
- Glob patterns for command matching
- Rule priority and override
- Per-tool permission rules
- Settings hierarchy (global < project < worktree < session)
- Rule inheritance and merge

**Action items**:
1. Add glob pattern matching for command rules
2. Add rule priority system
3. Add per-tool permission rules
4. Add settings hierarchy with merge
5. Add rule inheritance

---

## P1-25: API Client Beta Headers [DONE — AUDIT: PASS]

| Field | Value |
|-------|-------|
| Gap type | 缺失 |
| Severity | MEDIUM |
| Source | 04-api-client.md §A.2 |
| Affected files | `beta_headers.go`, `agent_loop.go` |
| Upstream | `src/services/api/claude.ts` — `anthropic-beta` header construction |
| REPL | N/A — core agent logic |

**Audit note**: Full implementation with 8 beta header constants, `BuildBetaHeaders()` dynamically constructs headers based on model and env vars, `agent_loop.go` sends `anthropic-beta` header on all API requests.

---

## P1-26: Error Classification System [DONE — AUDIT: PARTIAL]

| Field | Value |
|-------|-------|
| Gap type | 缺失 |
| Severity | MEDIUM |
| Source | 04-api-client.md §A.4 |
| Round | 20 Committed (PARTIAL) |
| Affected files | `streaming.go`, `error_classify.go` |
| Upstream | `src/services/api/withRetry.ts`, streaming error handling |
| REPL | N/A — core agent logic |

**Audit note**: Implemented `ErrorClass` enum with 15 categories (`Unauthorized`, `RateLimit`, `Overloaded`, `ServerOverloaded`, `ContentPolicy`, `ContextLengthExceeded`, `PromptTooLong`, `ModelNotSubscribed`, `ServerError`, `RateLimitCandidate`, `CachePoisoned`, `NetworkError`, `Timeout`, `Unknown`). `ClassifyResult()` carries recovery hints: `Retryable`, `Compress`, `RotateKey`, `Fallback`, `RetryAfter`. **Remaining gap**: upstream also has partial stream recovery (reconnect on disconnect), stream interruption detection, and connection timeout with fallback — these are not yet implemented.

---

## P1-27: Transcript Resume Enhancements

| Field | Value |
|-------|-------|
| Gap type | 简化 |
| Severity | MEDIUM |
| Source | 07-architecture.md §A.2 |
| Status | NEW |
| Affected files | `transcript/` |
| Upstream | Transcript resume with time-travel, fork, rewind |
| REPL | REPL-relevant — resume is a REPL feature |

**Problem**: Go's transcript resume is basic. Upstream has:
- Time-travel resume (`--resume-session-at`)
- Fork session support
- Rewind files on resume
- Session metadata (model, cost, duration)
- Cloud session discovery

**Action items**:
1. Add `--resume-session-at` for time-travel resume
2. Add `--fork-session` support
3. Add file rewind on resume
4. Add session metadata recording

---

## P1-28: Error Classification System [DONE — AUDIT: PASS]

| Field | Value |
|-------|-------|
| Gap type | 简化 |
| Severity | MEDIUM |
| Source | 07-architecture.md §A.3 |
| Round | 20 Committed (PASS) |
| Affected files | `error_types.go` |
| Upstream | Error severity levels, categories, retry strategies |
| REPL | N/A — core agent logic |

**Audit note**: Full implementation with 15-category `ErrorClass` enum (retryable, non_retryable, context_overflow, tool_pairing, rate_limit, billing, model_not_found, payload_too_large, overloaded, timeout, format_error, auth, thinking_signature, long_context_tier, unknown). `ClassifyResult` carries recovery hints: `Retryable`, `Compress`, `RotateKey`, `Fallback`, `RetryAfter`. Error pattern matching covers billing, rate limit, usage limit, overload, auth, model not found, and payload too large patterns.

---

## P1-29: Context Reference Expansion

| Field | Value |
|-------|-------|
| Gap type | 缺失 |
| Severity | MEDIUM |
| Source | 07-architecture.md §A.4 |
| Status | NEW |
| Affected files | `context_references.go` |
| Upstream | `src/utils/contextReferences.ts` — @folder, @diff, @staged, @gitlog, @url |
| REPL | REPL-relevant — @-references are a REPL feature |

**Problem**: Go's @-reference expansion is incomplete. Missing:
- `@folder` expansion (directory listing with content)
- `@diff` expansion (git diff injection)
- `@staged` expansion (git staged changes)
- `@gitlog` expansion (commit history)
- `@url` expansion (web content fetching)
- Token budget for expansion (upstream limits to 50K tokens)

**Action items**:
1. Add `@folder` expansion
2. Add `@diff` expansion
3. Add `@staged` expansion
4. Add `@gitlog` expansion
5. Add `@url` expansion
6. Add token budget for expansion (50K tokens)

---

## P1-30: File History Snapshots

| Field | Value |
|-------|-------|
| Gap type | 简化 |
| Severity | MEDIUM |
| Source | 07-architecture.md §A.5 |
| Status | NEW |
| Affected files | `filehistory.go` |
| Upstream | `src/utils/fileHistory.ts` — auto-snapshot, diff generation, tagging |
| REPL | N/A — core agent logic |

**Problem**: Go's file history is basic. Upstream has:
- Auto-snapshot before write/edit
- Diff generation (current vs snapshot)
- Batch operations (bulk snapshot/restore)
- Tagging system
- Cross-file timeline
- Snapshot metadata (timestamps, annotations)

**Action items**:
1. Add auto-snapshot before write/edit
2. Add diff generation
3. Add batch operations
4. Add tagging system
5. Add cross-file timeline
6. Add snapshot metadata

---

## P1-31: MCP Tool Schema Validation

| Field | Value |
|-------|-------|
| Gap type | 缺失 |
| Severity | MEDIUM |
| Source | 03-system-prompt.md §E.2 |
| Status | NEW |
| Affected files | `mcp/` |
| Upstream | MCP tool input schema validation |
| REPL | N/A — core agent logic |

**Problem**: Go doesn't validate MCP tool schemas against the JSON Schema spec. Upstream validates tool input schemas and provides helpful error messages when parameters don't match. This can cause silent failures or confusing errors.

**Action items**:
1. Add JSON Schema validation for MCP tool inputs
2. Add helpful error messages for schema mismatches
3. Add schema caching for performance

---

## P1-32: Sub-Agent Context Isolation

| Field | Value |
|-------|-------|
| Gap type | 简化 |
| Severity | MEDIUM |
| Source | 02-tools.md §D.2 |
| Status | NEW |
| Affected files | `tools/agent_tool.go` |
| Upstream | `src/utils/agent.ts` — context isolation; `src/utils/worktree.ts` — state isolation |
| REPL | N/A — core agent logic |

**Problem**: Sub-agents share module-level state with the main agent, which can cause state clobbering. Upstream isolates sub-agent context completely. Go's `ShouldAvoidPermissionPrompts` flag exists but other shared state (caches, registries) is not isolated.

**Action items**:
1. Audit all shared state between main agent and sub-agents
2. Add context isolation for sub-agent caches
3. Add sub-agent-specific registry instances
4. Add guard for main-thread-only state clears

---

## Summary Table

| # | Gap | Audit | Status | Effort | REPL |
|---|-----|-------|--------|--------|------|
| P1-1 | Cost tracking | PARTIAL | DONE | Medium | N/A |
| P1-2 | Reactive compaction | PASS | DONE | Medium | N/A |
| P1-3 | 529/429 handling | PARTIAL | DONE | Medium | N/A |
| P1-4 | Model aliases | PASS | DONE | Small | N/A |
| P1-5 | Cache break detection | PASS | DONE | Large | N/A |
| P1-6 | Classifier improvements | PASS | DONE | Medium | N/A |
| P1-7 | Skill content pipeline | PARTIAL | DONE | Medium | N/A |
| P1-8 | Hook system expansion | PASS | DONE | Large | N/A |
| P1-9 | Normalization pipeline | PARTIAL | DONE | Large | N/A |
| P1-10 | Transcript DAG | PARTIAL | DONE | Medium | N/A |
| P1-11 | Agent tool improvements | PARTIAL | DONE | Large | N/A |
| P1-12 | Post-compact recovery (partial) | PARTIAL | DONE | Medium | N/A |
| P1-13 | SM-compact token retention | — | DONE | Medium | N/A |
| P1-14 | LLM compaction summary quality | — | DONE | Medium | N/A |
| P1-15 | Non-LLM compaction metadata | — | DONE | Small | N/A |
| P1-16 | Tool output structured format | PASS | DONE | Medium | N/A |
| P1-17 | Exec tool safety | PASS | DONE | Medium | REPL |
| P1-18 | File read enhancements | PARTIAL | DONE | Medium | REPL |
| P1-19 | File write safety | PARTIAL | DONE | Small | REPL |
| P1-20 | Grep/Glob alignment | PARTIAL | DONE | Medium | REPL |
| P1-21 | Git tool enhancements | — | NEW | Medium | REPL |
| P1-22 | Notebook edit tool | — | NEW | Medium | N/A |
| P1-23 | System prompt dynamic sections | — | NEW | Medium | N/A |
| P1-24 | Permission rule engine | — | NEW | Large | N/A |
| P1-25 | API client beta headers | — | DONE | Small | N/A |
| P1-26 | Error classification system | PARTIAL | DONE | Medium | N/A |
| P1-27 | Transcript resume | — | NEW | Medium | REPL |
| P1-28 | Error classification system | PASS | DONE | Medium | N/A |
| P1-29 | Context reference expansion | — | NEW | Medium | REPL |
| P1-30 | File history snapshots | — | NEW | Medium | N/A |
| P1-31 | MCP tool schema validation | — | NEW | Small | N/A |
| P1-32 | Sub-agent context isolation | — | NEW | Medium | N/A |

## Audit Legend

| Status | Meaning |
|--------|---------|
| **PASS** | Implementation matches upstream design |
| **PARTIAL** | Exists but incomplete or deviates from upstream |
| **FAIL** | Fabricated/stub code, needs rewrite |
| **—** | Not yet audited |
| **REWRITE** | Failed audit, must be redone |

## REPL Tag Legend

| Tag | Meaning |
|-----|---------|
| **N/A** | Core agent logic — must replicate upstream exactly |
| **REPL** | REPL-relevant — reference upstream but adapt for CLI |
