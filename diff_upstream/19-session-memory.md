# Session Memory & Skills

> Session memory, skills, file history, snapshots

## Sections Included
- [##] Line 2705-2735 -- ## 23. Skills System (`skills/loader.go` + `skills/tracker.go`)
- [##] Line 4064-4138 -- ## 12. filehistory.go — 文件历史/快照
- [##] Line 4865-5115 -- ## Part 10: Session Memory System — Deep Comparison
- [##] Line 5116-5360 -- ## Part 11: Skills System — Deep Comparison
- [##] Line 7216-7218 -- ## Go Implementation
- [##] Line 7219-7228 -- ## Upstream Implementation
- [##] Line 7229-7245 -- ## 1. Snapshot Creation — When Snapshots Are Taken
- [##] Line 7246-7271 -- ## 2. Snapshot Storage — In-Memory vs Disk
- [##] Line 7272-7297 -- ## 3. Snapshot Restoration — Undo/Rollback Mechanisms
- [##] Line 7298-7316 -- ## 4. Snapshot Metadata — Timestamps, Annotations
- [##] Line 7317-7333 -- ## 5. Snapshot Listing — Viewing Available Snapshots
- [##] Line 7334-7351 -- ## 6. Diff Generation — Comparing Current vs Snapshot
- [##] Line 7352-7366 -- ## 7. Cross-File Timeline — Go's Timeline Feature
- [##] Line 7367-7383 -- ## 8. Batch Operations — Bulk Snapshot/Restore
- [##] Line 7384-7400 -- ## 9. Tagging System — Snapshot Tags
- [##] Line 7401-7416 -- ## 10. Auto-Snapshot — Before Write/Edit
- [##] Line 7434-7454 -- ## Summary Table: File History
- [##] Line 9090-9273 -- ## Section XXII: Deep Dive - Tool Result Persistence, Cached Microcompact, API Context Management, Cost Tracking (Round 26)
- [##] Line 9611-9656 -- ## 38. Session Memory, Skills, File History, Context References

---

## Content

## 23. Skills System (`skills/loader.go` + `skills/tracker.go`)

**Upstream reference**: `src/skills/loadSkillsDir.ts` + `src/skills/bundled/` + `src/skills/bundledSkills.ts`

### 23.1 Skill loading architecture
- **上游**: `getSkillDirCommands()` (memoized async) loads skills from 5 sources: managed (`policySettings`), user, project, additional (`--add-dir`), legacy commands. Uses `loadMarkdownFilesForSubdir()`, `parseFrontmatter()`, `createSkillCommand()`. Deduplicates by `realpath()`. Supports conditional skills (path-filtered via `ignore` library). Supports dynamic skill discovery from file paths (loadSkillsDir.ts:638-803)
- **Go版**: `Loader` with `Refresh()` scanning builtin + workspace directories synchronously. No memoization, no conditional skills, no dynamic discovery, no deduplication by realpath. Supports `requires`, `extended_requires.bins`, `extended_requires.env` (skills/loader.go:44-83)
- **类型**: 简化 — no conditional skills, no dynamic discovery, no multi-source loading, no deduplication

### 23.2 Frontmatter parsing
- **上游**: Uses `parseFrontmatter()` from `frontmatterParser.ts` with full YAML parsing, supports `allowed-tools`, `argument-hint`, `arguments`, `when_to_use`, `version`, `model`, `disable-model-invocation`, `user-invocable`, `context`, `agent`, `effort`, `shell`, `hooks`, `paths` (loadSkillsDir.ts:185-265)
- **Go版**: Custom `parseFrontmatter()` with hand-rolled YAML parsing (no yaml library). Supports: `name`, `description`, `always`, `available`, `version`, `commands`, `tags`, `requires`, `bins`, `env`, `when_to_use` (skills/loader.go:314-409)
- **类型**: 简化 — fewer frontmatter fields; no `model`, `effort`, `hooks`, `paths`, `allowed-tools`, `arguments`, `shell`

### 23.3 Skill tracking
- **上游**: No explicit `SkillTracker`; skill state managed via React component state and `SkillTool.ts` with `was_discovered` telemetry
- **Go版**: `SkillTracker` with `shownSkills`, `readSkills` (with timestamps), `usedSkills` sets. Supports `ResetPostCompact()` (preserves `shownSkills`, clears `readSkills`), `GetReadSkillNames()` (sorted by read time), `GenerateDiscoveryReminder()`, `RestoreReadSkills()` for resume (skills/tracker.go:13-187)
- **类型**: Go增强 — explicit tracking with post-compact recovery logic; upstream has no equivalent class

### 23.4 Skill tool surface
- **上游**: Skills invoked as slash commands (`/skill-name`); `SkillTool` reads and injects skill content. No `search_skills`, `list_skills`, or `read_skill` as separate LLM tools
- **Go版**: Three dedicated LLM tools: `read_skill`, `list_skills`, `search_skills` (with weighted relevance scoring: name=100/50, tags=30, description=20, when_to_use=15) (tools/skill_tools.go)
- **类型**: Go增强 — LLM can discover and read skills autonomously; upstream relies on user invoking slash commands

### 23.5 MCP skills
- **上游**: `mcpSkillBuilders.ts` + `mcpSkills.ts` register MCP server tools as skills
- **Go版**: No MCP skill integration in the Loader
- **类型**: 缺失 — no MCP-as-skills

---


---

## 12. filehistory.go — 文件历史/快照

### 12.1 架构对比

| Go | Upstream |
|---|---|
| `SnapshotHistory`：每个文件存完整内容副本，JSON 持久化到 `.claude/snapshots/`（`filehistory.go:29-35`） | 上游 `FileHistory`：只存备份文件引用（hash-based），通过 `fileHistoryTrackEdit()` 在编辑前自动备份（`utils/attachments.ts:33-52`） |
| `FileSnapshot` 包含 `Content` 字段，直接存储文件内容（`filehistory.go:17-24`） | 上游 `FileHistoryBackup` 只存 `backupFileName`（hash）和 `version`，内容在备份文件中（`utils/attachments.ts:33-37`） |

### 12.2 快照策略

| Go | Upstream |
|---|---|
| **主动快照**：`TakeSnapshot()` 手动调用，通常在编辑前调用（`filehistory.go:64-66`） | **自动快照**：`fileHistoryMakeSnapshot()` 在每个 turn 后自动调用，备份所有已跟踪文件（`utils/attachments.ts:198-342`） |
| **内容去重**：相同 checksum 跳过新快照（`filehistory.go:90-96`） | **mtime 检查**：`checkOriginFileChanged()` 先比较 stat，再按需比较内容（`utils/attachments.ts:600-634`） |
| 每文件最多 50 个快照，超过则修剪（`filehistory.go:38-39,119-150`） | 最多 100 个快照（`utils/attachments.ts:54`），通过 React state 管理 |
| 按时间裁剪，保留至少 5 个（`filehistory.go:133-149`） | **上游无基于时间的裁剪** — 快照关联 message ID，随会话管理 |

### 12.3 恢复机制

| Go | Upstream |
|---|---|
| `RewindTo(index)`, `RestoreLast()`, `RewindSteps()`, `Checkout(version)` 多种恢复方式（`filehistory.go:155-357`） | 上游 `fileHistoryRewind(messageId)` 按 message ID 恢复整个快照状态（`utils/attachments.ts:347-397`） |
| `restoreInternal()` 按"不同内容版本"回退，跳过 checksum 相同的快照（`filehistory.go:204-281`） | 上游 `applySnapshot()` 逐个文件恢复，只修改有变化的文件（`utils/attachments.ts:537-591`） |
| 恢复前自动保存当前状态（`filehistory.go:169-180`），支持 redo | 上游恢复前不保存当前状态（直接覆盖） |

### 12.4 标签与搜索

| Go | Upstream |
|---|---|
| `AddTag()`, `AnnotateSnapshot()`, `ListTags()`, `SearchTag()`, `RemoveTag()`, `SearchTagAll()` 完整的标签系统（`filehistory.go:565-831`） | **上游无标签系统** — 快照仅关联 message ID |
| 标签存在 snapshot 的 `Description` 字段中（`[tagname]` 格式）（`filehistory.go:592-595`） | — |

### 12.5 历史搜索（diff-based）

| Go | Upstream |
|---|---|
| `Search(filePath, pattern, mode, ignoreCase)` 在连续快照间做行集合 diff（`filehistory.go:886-973`） | 上游 `fileHistoryGetDiffStats()` 使用 `diffLines` 库计算精确 diff 统计（`utils/attachments.ts:414-484`） |
| 使用 set difference（而非真正的 diff），只标记新增/删除的行 | 上游使用 `diffLines` 库，精确计算 insertions/deletions |
| **Go 有 `SearchAdded`, `SearchRemoved`, `SearchChanged` 3种模式** | 上游 `fileHistoryHasAnyChanges()` 只做布尔检查（`utils/attachments.ts:494-531`） |

### 12.6 磁盘持久化

| Go | Upstream |
|---|---|
| 每个快照存为独立 JSON 文件：`{timestamp}_{sanitized_path}.json`（`filehistory.go:1011-1033`） | 备份文件存为 hash 格式：`{sha256hash}@v{version}`，存于 `{configDir}/file-history/{sessionId}/`（`utils/attachments.ts:725-741`） |
| 文件名包含时间戳，通过 glob 模式加载（`filehistory.go:1036-1094`） | 文件名不包含时间戳，通过版本号索引 |
| 加载时回填 `Description` 和 `Checksum`（`filehistory.go:1071-1084`） | **上游无回填逻辑** — 所有元数据在创建时确定 |
| **Go 的快照包含完整文件内容** | 上游只存备份引用，内容在备份文件中（copyFile 硬链接/复制） |

### 12.7 会话恢复

| Go | Upstream |
|---|---|
| **Go 无会话恢复支持** | 上游 `copyFileHistoryForResume()` 在会话恢复时迁移备份文件（`utils/attachments.ts:922-1046`） |
| — | 上游 `fileHistoryRestoreStateFromLog()` 从日志恢复状态（`utils/attachments.ts:888-917`） |

### 12.8 上游独有功能

| Go 缺失 | 上游有 |
|---|---|
| **无 `fileHistoryTrackEdit()` 自动跟踪** | 上游在每次文件编辑前自动创建备份 |
| **无 `notifyVscodeSnapshotFilesUpdated()` VSCode 通知** | 上游通知 VSCode 文件变更（`utils/attachments.ts:1054-1098`） |
| **无 `fileHistoryCanRestore()` 预检查** | 上游 `fileHistoryCanRestore()` 检查是否可恢复（`utils/attachments.ts:399-408`） |

### 12.9 Go 独有功能

| Go 有 | 上游无 |
|---|---|
| `ResolveVersion()` 支持多种版本指定：`v3`, `current`, `last2`, `[tagname]`（`filehistory.go:489-560`） | 上游仅支持 message ID |
| `GetTimeline()` 跨文件时间线（`filehistory.go:843-867`） | 上游无时间线视图 |
| 完整的标签/注释系统 | 上游无标签系统 |

---


---

## Part 10: Session Memory System — Deep Comparison

**Go source**: `session_memory.go` (~1009 lines)
**Upstream sources**:
- `src/services/SessionMemory/sessionMemory.ts` (~503 lines) — extraction orchestration
- `src/services/SessionMemory/sessionMemoryUtils.ts` (~208 lines) — thresholds, state tracking, file I/O
- `src/services/SessionMemory/prompts.ts` (~325 lines) — template, update prompt, truncation
- `src/services/compact/sessionMemoryCompact.ts` (~631 lines) — SM-compact integration
- `src/services/extractMemories/extractMemories.ts` (~616 lines) — auto-memory extraction (separate system)

### 10.1 Memory File Format — 10-Section Template

**Both Go and upstream use the same 10-section markdown template:**

| # | Section | Go | Upstream |
|---|---------|-----|----------|
| 1 | Session Title | ✅ `_A short and distinctive 5-10 word descriptive title...` | ✅ Identical |
| 2 | Current State | ✅ | ✅ |
| 3 | Task Specification | ✅ | ✅ (upstream: "Task specification") |
| 4 | Files and Functions | ✅ | ✅ |
| 5 | Workflow | ✅ | ✅ |
| 6 | Errors & Corrections | ✅ | ✅ |
| 7 | Codebase and System Documentation | ✅ | ✅ |
| 8 | Learnings | ✅ | ✅ |
| 9 | Key Results | ✅ | ✅ (upstream: "Key results") |
| 10 | Worklog | ✅ | ✅ |

**Minor differences**:
- Go: "# Task Specification" (capital S), "# Key Results" (capital R)
- Upstream: "# Task specification" (lowercase s), "# Key results" (lowercase r)
- Upstream `_description_` for Task specification: "Any design decisions or other explanatory context" — no trailing period
- Go: "Any design decisions or explanatory context." — different wording + period
- Upstream Workflow description: "How to interpret their output if not obvious?" — Go omits "if not obvious"

**Template loading**:
- **Go**: Hardcoded constant `defaultSessionMemoryTemplate`, no custom template support
- **Upstream**: `loadSessionMemoryTemplate()` — checks `~/.claude/session-memory/config/template.md` for custom template, falls back to `DEFAULT_SESSION_MEMORY_TEMPLATE`. **Custom template is a user-facing feature Go lacks**.

**Template emptiness detection**:
- **Go**: `IsSessionMemoryTemplateOnly()` — simple `strings.TrimSpace` comparison against hardcoded constant
- **Upstream**: `isSessionMemoryEmpty()` — async, loads custom template via `loadSessionMemoryTemplate()`, then trims and compares. Handles user-customized templates correctly.

### 10.2 LLM Extraction Prompt — Forked Agent

**Go** (`sessionMemoryUpdatePrompt`):
- Single hardcoded prompt function with `fmt.Sprintf` substitution
- Uses `{{notesPath}}` and `{{currentNotes}}` directly as `%s` format args
- Tool name: "edit_file" / "multi_edit" (Go's tool names)
- Section limit: injects `maxTokensPerSection` constant (20000) into prompt
- **No custom prompt support** — no user-overridable prompt file

**Upstream** (`getDefaultUpdatePrompt()` + `buildSessionMemoryUpdatePrompt()`):
- Uses `{{notesPath}}` / `{{currentNotes}}` template variable syntax with `substituteVariables()`
- Tool name: "Edit" (upstream's tool name)
- Section limit: injects `MAX_SECTION_LENGTH` constant (2000) into prompt
- **Custom prompt support**: `loadSessionMemoryPrompt()` checks `~/.claude/session-memory/config/prompt.md`
- **Dynamic section reminders**: `analyzeSectionSizes()` + `generateSectionReminders()` — appends warnings for sections over budget or total over `MAX_TOTAL_SESSION_MEMORY_TOKENS`. This is a **significant feature Go lacks**.

**Key structural difference**: Upstream's `buildSessionMemoryUpdatePrompt` does:
1. Load prompt template (custom or default)
2. Analyze section sizes of current memory
3. Generate reminders for oversized sections (e.g., `"# Current State" is ~3500 tokens (limit: 2000)`)
4. If total exceeds 12000 tokens, adds `CRITICAL: ... exceeds maximum` warning
5. Substitute variables and append reminders

Go's `sessionMemoryUpdatePrompt` only does step 1 (with hardcoded template) and step 5 (variable substitution via fmt.Sprintf).

### 10.3 Token Thresholds for Extraction

| Threshold | Go | Upstream |
|-----------|-----|----------|
| `minimumMessageTokensToInit` | 10000 | 10000 |
| `minimumTokensBetweenUpdate` | 5000 | 5000 |
| `toolCallsBetweenUpdates` | 3 | 3 |

**Values are identical**, but the implementation differs:

**Go** (`ExtractionState.ShouldExtract`):
- Single method `ShouldExtract(currentTokens, hasToolCallsInLastTurn) bool`
- Returns true when: `(initialized AND tokenThreshold AND toolCallThreshold) OR (initialized AND tokenThreshold AND !hasToolCallsInLastTurn)`
- First init check: `currentTokens >= 10000`

**Upstream** (`shouldExtractMemory` in sessionMemory.ts):
- `tokenCountWithEstimation(messages)` — counts tokens from message array (same metric as autocompact)
- Uses module-scoped `lastMemoryMessageUuid` + `countToolCallsSince()` to count tool calls since last extraction
- Same threshold logic but split across: `hasMetInitializationThreshold()`, `hasMetUpdateThreshold()`, `getToolCallsBetweenUpdates()`

**Remote config**:
- **Go**: Hardcoded constants, no remote config
- **Upstream**: `getSessionMemoryRemoteConfig()` — fetches `tengu_sm_config` from GrowthBook, merges with defaults. `initSessionMemoryConfigIfNeeded()` is memoized, runs once per session. Remote values override defaults if positive.

### 10.4 File Persistence and Reading

**Go** (`SessionMemory`):
- Path: `{projectDir}/.claude/session_memory.md`
- **In-memory model**: `entries []MemoryEntry` — Go maintains a structured slice of entries, writes the template format to disk
- `loadFromDisk()` → `parseMarkdownEntries()` — parses both template format and legacy list format
- `flushToDisk()` — atomic write via temp file + rename (avoids Windows locking issues)
- **Background flush loop**: 30-second ticker + final flush on Stop()
- **Dirty flag**: only writes when `dirty == true`

**Upstream** (`sessionMemory.ts` + `sessionMemoryUtils.ts`):
- Path: `getSessionMemoryPath()` → `~/.claude/session_memory.md` (user config home, not project dir)
- **File-only model**: no in-memory structured entries; reads the file each time via `getSessionMemoryContent()`
- `setupSessionMemoryFile()` — creates file with `wx` flag (exclusive create), writes template, then reads via `FileReadTool.call()`
- **No background flush loop** — file is written by the forked agent's Edit tool calls, not by the session memory system itself
- **No dirty tracking** — forked agent directly edits the file

**Critical difference**: Go uses **project-scoped** memory (`{projectDir}/.claude/`), upstream uses **user-scoped** memory (`~/.claude/`). This means:
- Go: different projects have independent session memories
- Upstream: single global session memory file shared across projects

### 10.5 Memory Categories and Structure

**Go** has a dual representation:
1. **Internal categories**: `preference`, `decision`, `state`, `reference`, `test` — used by `MemoryEntry.Category`
2. **Template sections**: 10 markdown sections — used for display and persistence
3. **Mapping**: template sections map to categories (e.g., "Current State" → `state`, "Task Specification" → `decision`, "Files and Functions" → `reference`, "Learnings" → `preference`)

**Per-category limits**:

| Category | Go Max Entries | Go TTL | Upstream |
|----------|---------------|--------|----------|
| state | 20 | 7 days | N/A (no in-memory categories) |
| decision | 30 | 30 days | N/A |
| preference | 20 | 30 days | N/A |
| reference | 50 | 30 days | N/A |
| test | 20 | 30 days | N/A |

**Upstream has NO in-memory category system** — the LLM directly edits the markdown file's sections. The LLM is responsible for organizing content within sections. Go's `MemoryEntry` struct with `Category`/`Content`/`Timestamp`/`Source` fields is a Go-only abstraction.

### 10.6 Deduplication and Merging Logic

**Go**:
- `AddNote()`: exact dedup — if `category + content` matches an existing entry, updates timestamp instead of adding duplicate
- `SaveConclusions()`: checks for exact content match before adding state entries
- `trimCategoryEntriesLocked()`: enforces per-category max by removing oldest entries
- `removeExpiredEntries()`: TTL-based expiry on load
- `ClearStateEntries()`: clears all `state` entries on session start (prevents stale cross-session context)

**Upstream**:
- **No explicit deduplication** — the forked agent edits the markdown file with the Edit tool, so dedup is handled by the LLM's judgment
- **No TTL/expiry** — no automatic entry expiration mechanism
- **No per-category limits** — relies on LLM following the prompt instruction to keep sections under ~2000 tokens
- `truncateSessionMemoryForCompact()`: truncates oversized sections at read time, not at write time
- `isSessionMemoryEmpty()`: detects if the file is still just the template

**Key insight**: Go's approach is **programmatic dedup + TTL + per-category limits**. Upstream's approach is **LLM-guided content management + post-hoc truncation**. Go adds structural guarantees that upstream delegates to the LLM.

### 10.7 SM-Compact Integration

**Go**:
- `LastSummarizedMessageUUID` — tracks UUID of most recently summarized message for incremental compaction
- `GetLastSummarizedMessageUUID()` / `SetLastSummarizedMessageUUID()` — accessors
- `FormatForPromptCompact()` — formats entries with token budgets (20000/section, 60000 total)
- `truncateSessionMemoryForCompact()` — standalone function, truncates per-section and globally
- `ExtractionState.WaitForExtraction()` — waits for in-progress extraction with 60s stale threshold
- **No feature gate** — SM-compact always available

**Upstream** (`sessionMemoryCompact.ts`):
- `lastSummarizedMessageId` — module-scoped in `sessionMemoryUtils.ts`
- `shouldUseSessionMemoryCompaction()` — **feature-gated**: requires both `tengu_session_memory` AND `tengu_sm_compact` GrowthBook flags
- `DEFAULT_SM_COMPACT_CONFIG`: `{ minTokens: 10000, minTextBlockMessages: 5, maxTokens: 40000 }`
- `calculateMessagesToKeepIndex()` — sophisticated algorithm that:
  1. Starts from `lastSummarizedIndex`
  2. Expands backwards to meet `minTokens` and `minTextBlockMessages` minimums
  3. Stops at `maxTokens` hard cap
  4. Adjusts for tool_use/tool_result pairs and shared message.id thinking blocks
  5. Floors at compact boundary to prevent cross-boundary slicing
- `adjustIndexToPreserveAPIInvariants()` — handles streaming message splitting, orphan tool_results
- `createCompactionResultFromSessionMemory()` — builds CompactionResult with truncated memory, boundary marker, hook results
- `trySessionMemoryCompaction()` — full SM-compact flow with feature gates, resumed session handling, threshold checking
- **Remote config**: `initSessionMemoryCompactConfig()` fetches `tengu_sm_compact_config` from GrowthBook
- **Analytics**: logs `tengu_sm_compact_*` events throughout

**Token budget differences**:

| Parameter | Go | Upstream |
|-----------|-----|----------|
| Per-section limit | 20000 tokens | 2000 tokens |
| Total SM limit | 60000 tokens | 12000 tokens |
| SM-compact max kept | N/A | 40000 tokens (configurable) |
| SM-compact min kept | N/A | 10000 tokens (configurable) |

Go uses **10x higher token limits** (20000 vs 2000 per section, 60000 vs 12000 total). The Go code comments explicitly state this is intentional: "Increased to 20000 per section and 60000 total to preserve more context across compaction cycles. 12000 total tokens is insufficient for long coding sessions."

### 10.8 Sensitive Info Filtering

**Go**: No explicit sensitive info filtering. Session memory entries are stored and injected as-is.

**Upstream**: No explicit sensitive info filtering in the session memory system either. Neither system has PII redaction or credential detection in the session memory path. (Upstream's `extractMemories` system has some sanitization via `recursivelySanitizeUnicode()` on MCP skill content, but this is for encoding issues, not secrets.)

### 10.9 Cross-Session Memory Persistence

**Go**:
- Persists to `{projectDir}/.claude/session_memory.md`
- `NewSessionMemory()` → `loadFromDisk()` → `ClearStateEntries()` → `removeExpiredEntries()`
- **State entries are explicitly cleared on new session start** — prevents stale "what's actively being worked on" from bleeding between sessions
- Other categories (decision, preference, reference) survive across sessions with TTL-based expiry
- Background flush loop ensures entries are written periodically

**Upstream**:
- Persists to `~/.claude/session_memory.md` (single global file)
- **No state clearing on new session** — the LLM edits the file directly; "Current State" is just a section the LLM updates
- **No TTL/expiry** — content lives until the LLM replaces it
- **Single file across all projects** — no project-scoped isolation
- The forked agent creates the file with `wx` (exclusive) flag on first use

**Key architectural difference**: Go maintains **project-isolated memory with programmatic lifecycle management** (TTL, state clearing, category limits). Upstream maintains a **single global file with LLM-driven content management** and no programmatic lifecycle.

### 10.10 extractMemories — Separate Auto-Memory System (Upstream Only)

Upstream has a **second memory system** (`extractMemories`) that Go does not implement at all:

- **Location**: `~/.claude/projects/<path>/memory/` — per-project auto-memory directory
- **Format**: Multiple topic-based markdown files (not a single session_memory.md)
- **Trigger**: `handleStopHooks` at end of each query loop (not post-sampling)
- **Feature gate**: `tengu_passport_quail` GrowthBook flag
- **Tool permissions**: `createAutoMemCanUseTool()` — allows Read/Grep/Glob unrestricted, read-only Bash, Edit/Write only for auto-memory paths
- **Turn-based throttling**: `tengu_bramble_lintel` flag (default: every 1 turn)
- **Mutual exclusion**: if main agent wrote memories directly, skip forked extraction
- **Trailing run**: stashes context for a trailing extraction if one is already in progress
- **Team memory**: `feature('TEAMMEM')` — extracts to team memory paths in addition to auto-memory
- **Drain mechanism**: `drainPendingExtraction()` — awaits in-flight extractions before shutdown

**Go has no equivalent** — no auto-memory extraction, no per-project memory directory, no team memory.

### Session Memory Summary

| Aspect | Go | Upstream |
|--------|-----|----------|
| File location | Project-scoped (`{project}/.claude/session_memory.md`) | User-scoped (`~/.claude/session_memory.md`) |
| In-memory model | Structured `[]MemoryEntry` with categories | No in-memory model (file-only) |
| Category system | 5 categories (state/decision/preference/reference/test) | 10-section template (LLM manages content) |
| Dedup | Programmatic (exact match) | LLM-driven |
| Entry expiry | TTL-based (7d state, 30d others) | None |
| Per-category limits | Yes (20-50 entries) | No (LLM-guided) |
| State clearing | On new session start | None |
| Token limits (section/total) | 20000/60000 | 2000/12000 |
| Custom template | No | Yes (`~/.claude/session-memory/config/template.md`) |
| Custom prompt | No | Yes (`~/.claude/session-memory/config/prompt.md`) |
| Section size warnings | No | Yes (dynamic in prompt) |
| Feature gates | None | `tengu_session_memory`, `tengu_sm_compact` |
| Remote config | None | GrowthBook (`tengu_sm_config`, `tengu_sm_compact_config`) |
| Auto-memory (extractMemories) | Not implemented | Full implementation |
| SM-compact algorithm | Simple truncation | Sophisticated index calculation with API invariant preservation |
| Analytics | None | Extensive (`tengu_session_memory_*`, `tengu_sm_compact_*`, `tengu_extract_memories_*`) |
| Background flush | 30s ticker | N/A (forked agent writes directly) |

---


---

## Part 11: Skills System — Deep Comparison

**Go sources**:
- `skills/loader.go` (~597 lines) — skill loading, frontmatter parsing, dependency checking
- `skills/tracker.go` (~187 lines) — skill tracking, progressive discovery, post-compact reset

**Upstream sources**:
- `src/skills/loadSkillsDir.ts` (~1087 lines) — skill loading from directories, frontmatter parsing, dynamic discovery, conditional skills
- `src/skills/mcpSkills.ts` (~143 lines) — MCP-based skill discovery
- `src/skills/mcpSkillBuilders.ts` (~45 lines) — dependency injection for MCP skill builders
- `src/skills/bundledSkills.ts` (~221 lines) — bundled skill registration and file extraction
- `src/utils/frontmatterParser.ts` (~371 lines) — full YAML frontmatter parser
- `src/utils/suggestions/skillUsageTracking.ts` (~56 lines) — skill usage scoring
- `src/utils/skills/skillChangeDetector.ts` — skill change detection (not read)

### 11.1 Skill Loading — Directory Scanning

**Go** (`Loader`):
- `NewLoader(workspace)` — creates loader with `{workspace}/skills` as skill directory
- `SetBuiltinDir(dir)` — optional builtin directory for bundled skills
- `Refresh()` — rescans both builtin and workspace directories, rebuilds index
- `scanDirLocked()` — reads directory entries, skips non-directories, looks for `SKILL.md` in each subdirectory
- **Only supports `skill-name/SKILL.md` directory format**
- Workspace skills override builtin skills with same name (builtin scanned first, `exists` check skips duplicates)

**Upstream** (`getSkillDirCommands`):
- Memoized async function that loads from **5 source hierarchies**:
  1. `managedSkillsDir` — `getManagedFilePath()/.claude/skills` (policy settings)
  2. `userSkillsDir` — `~/.claude/skills` (user settings)
  3. `projectSkillsDirs` — walked up from cwd to home (`getProjectDirsUpToHome('skills', cwd)`)
  4. `additionalDirs` — `--add-dir` paths
  5. `legacyCommandsDir` — `/commands/` directories (deprecated format)
- `loadSkillsFromSkillsDir()` — same `SKILL.md` directory format
- `loadSkillsFromCommandsDir()` — **legacy** format: single `.md` files AND `SKILL.md` directories
- **Bare mode** (`--bare`): skips auto-discovery, only loads `--add-dir` skills
- **Policy locking**: `isRestrictedToPluginOnly('skills')` blocks certain sources
- **Symlink deduplication**: `getFileIdentity()` uses `realpath` to resolve symlinks and dedup by canonical path
- **Managed skills disable**: `CLAUDE_CODE_DISABLE_POLICY_SKILLS` env var
- **Source precedence**: managed → user → project → additional → legacy (first-wins dedup)

**Go has a much simpler loading model**: two directories (builtin + workspace) with no source hierarchy, no policy controls, no symlink dedup, no legacy format, no bare mode.

### 11.2 Frontmatter Parsing

**Go** (`parseFrontmatter` in loader.go):
- **Pure string-based parser** — no YAML library
- Manual line-by-line parsing with indent tracking for multi-line lists
- Handles: `key: value`, `key: [inline, list]`, multi-line `key:\n  - item`
- Supported fields: `name`, `description`, `always`, `available`, `version`, `commands`, `tags`, `requires`, `when_to_use`
- Extended: `extended_requires.bins` (→ `ExtBins`), `extended_requires.env` (→ `ExtEnv`)
- `parseInlineList()` — handles `[a, b, c]` with quote-aware splitting
- `parseBool()` — accepts `true`/`yes`/`True`/`Yes`
- `unquote()` — strips single/double quotes
- **No error reporting** — silently skips unparseable frontmatter

**Upstream** (`parseFrontmatter` in frontmatterParser.ts):
- **Full YAML parser** via `parseYaml()` (js-yaml or equivalent)
- Pre-processing: `quoteProblematicValues()` — auto-quotes values with YAML special chars (`{}[]*&#!|>%@` or `: `) so glob patterns like `**/*.{ts,tsx}` parse correctly
- **Fallback**: if YAML parsing fails, re-tries with quoted values
- **Error logging**: logs warnings with source path on parse failure
- `FrontmatterData` type — extensive field support including:
  - `allowed-tools`, `argument-hint`, `arguments`
  - `when_to_use`, `version`, `model`, `effort`
  - `user-invocable`, `hide-from-slash-command-tool`
  - `hooks` (validated by `HooksSchema`)
  - `context` (inline/fork), `agent`, `paths`, `shell`
  - `skills` (comma-separated skill preloading)
- `coerceDescriptionToString()` — validates description types (string/number/boolean OK, arrays/objects rejected)
- `parseBooleanFrontmatter()` — only accepts `true` or `"true"` (stricter than Go)
- `parseShellFrontmatter()` — validates `bash`/`powershell`, warns on unknown
- `splitPathInFrontmatter()` — brace expansion for glob patterns (`src/*.{ts,tsx}` → `src/*.ts, src/*.tsx`)
- `parseEffortValue()` — validates effort levels
- `parseUserSpecifiedModel()` — model specification parsing

**Key difference**: Go's parser is a minimal string-based implementation covering ~10 fields. Upstream uses a full YAML parser with pre-processing for glob patterns, validation, and ~20+ fields. Go's `parseBool` is more permissive (accepts "yes"/"Yes"), upstream is stricter (only `true`/`"true"`).

### 11.3 Skill Availability — Default Available=true vs Conditional Gating

**Go** (`parseSkillFileLocked`):
- `meta.Available = true` — **defaults to available** when `available:` key is absent
- Only sets `available = false` when frontmatter explicitly has `available: false`
- Dependency checking marks unavailable if:
  - `requires` items not found (file doesn't exist in workspace/builtin, not in PATH, not an env var)
  - `ExtBins` items not found in PATH (with Windows extension handling)
  - `ExtEnv` items not set in environment
- `CheckAvailability(skill)` — simply checks `len(skill.MissingDeps) == 0`

**Upstream** (`parseSkillFrontmatterFields`):
- `user-invocable` defaults: `commands/` → `true`, `skills/` → `true` (changed from historical false default)
- No `available` field — upstream uses `isEnabled?: () => boolean` on `BundledSkillDefinition`
- **Conditional skills** via `paths` frontmatter — skills with `paths` patterns start inactive, only activate when the model touches matching files
- `activateConditionalSkillsForPaths()` — uses `ignore` library for gitignore-style pattern matching
- **Feature gating**: `BundledSkillDefinition.isEnabled` callback — programmatic enable/disable
- **Managed skills disable**: `CLAUDE_CODE_DISABLE_POLICY_SKILLS` env var
- **Plugin-only policy**: `isRestrictedToPluginOnly('skills')`
- **Dynamic discovery gating**: `isSettingSourceEnabled('projectSettings')` check

**Critical difference**: Go uses a simple boolean `available` field with dependency checking. Upstream uses a rich combination of `paths`-based conditional activation, `isEnabled` callbacks, policy restrictions, and source-level gating. Go's `available: true` default means all skills are visible by default unless explicitly disabled or missing dependencies.

### 11.4 Skill Discovery — How Skills Are Presented to the Model

**Go** (`Loader`):
- `BuildSkillsSummary()` — XML-formatted listing: `<skills><skill available="true" always="false"><name>...</name><description>...</description><location>builtin:name</location><requires>...</requires></skill></skills>`
- `ListSkills(filterUnavailable)` — returns `[]SkillInfo` for all or only available skills
- `GetAlwaysSkills()` — returns skills with `always=true && available=true`
- No search/filter mechanism for the model
- No dynamic discovery — skills are loaded once on Refresh()

**Upstream**:
- `estimateSkillFrontmatterTokens()` — estimates token cost of frontmatter-only listing (used for budget management)
- **Dynamic discovery**: `discoverSkillDirsForPaths()` — walks up from file paths to cwd, finds `.claude/skills` directories in nested paths
- `addSkillDirectories()` — loads skills from discovered directories into `dynamicSkills` map
- **Conditional activation**: `activateConditionalSkillsForPaths()` — activates skills whose `paths` patterns match current files
- **Skill change signal**: `skillsLoaded` signal — notifies other modules when skills change (for cache clearing)
- **MCP skills**: `fetchMcpSkillsForClient()` — discovers `skill://` resources from MCP servers, memoized by server name with LRU cache
- **Bundled skills**: `registerBundledSkill()` / `getBundledSkills()` — programmatically registered skills with lazy file extraction
- `getSkillDirCommands()` returns `Command[]` — rich command objects with `getPromptForCommand()`, `allowedTools`, `model`, `effort`, `hooks`, `agent`, `context` (inline/fork)
- **Usage tracking**: `recordSkillUsage()` + `getSkillUsageScore()` — exponential decay scoring (7-day half-life) for skill ranking

**Go lacks**: dynamic discovery, conditional activation, MCP skills, bundled skills with file extraction, usage tracking, skill change signals, and the rich `Command` type system.

### 11.5 Skill Tracking — Read/Shown State Management

**Go** (`SkillTracker`):
- `shownSkills map[string]struct{}` — skills announced in system prompt
- `readSkills map[string]time.Time` — skills read by model (with timestamp for ordering)
- `usedSkills map[string]struct{}` — skills where model performed actions after reading
- `IsNewSkill(name)` — checks if not yet shown
- `MarkShown(name)` / `MarkRead(name)` / `MarkUsed(name)` — state transitions
- `GetUnsentSkills(allSkills)` — returns always-on skills + unshown skills
- `GenerateDiscoveryReminder(allSkills)` — generates hint text for unread skills
- `GetReadSkillNames()` — returns read skills sorted by time (most recent first)
- `ResetPostCompact()` — clears `readSkills` only, preserves `shownSkills` and `usedSkills`
- `RestoreReadSkills(skills)` — restores read state from persisted data (for session resume)

**Upstream**:
- No dedicated `SkillTracker` class — tracking is spread across:
  - `invokedSkills` state in the REPL/command system
  - `skillChangeDetector.ts` — detects when skills change between turns
  - Post-compact cleanup in `postCompactCleanup.ts`
- Skill tracking is **implicit** in the command execution flow rather than a separate tracker object
- `skillUsageTracking.ts` — persistent usage tracking with global config, 7-day half-life scoring
- **No explicit "shown/read/used" state machine** like Go's SkillTracker

**Go's SkillTracker is a Go-specific design** that provides explicit progressive discovery state management. Upstream achieves similar effects through the command execution pipeline and analytics, but without a clean separation into a tracker object.

### 11.6 Skill Content Injection — How Skill Content Enters Context

**Go** (`Loader`):
- `LoadSkill(name)` — returns raw SKILL.md content (including frontmatter)
- `LoadSkillsForContext(names)` — loads multiple skills, strips frontmatter, formats as `### Skill: {name}\n\n{content}` joined by `---`
- `BuildSystemPrompt(names)` — builds `# Active Skills` section with full skill content (including frontmatter)
- **All-or-nothing**: full skill content is injected, no lazy loading

**Upstream** (`createSkillCommand`):
- **Lazy loading**: `Command.getPromptForCommand(args, toolUseContext)` — content is generated on invocation, not at load time
- **Argument substitution**: `substituteArguments()` — replaces `$ARGUMENTS`, `$ARG_NAME` in content
- **Variable expansion**: `${CLAUDE_SKILL_DIR}` → skill's directory, `${CLAUDE_SESSION_ID}` → session ID
- **Shell execution**: `executeShellCommandsInPrompt()` — processes `!`...\`` and ` ```! ` blocks in skill content
- **Base directory prefix**: `Base directory for this skill: {dir}` prepended when `baseDir` is set
- **Fork context**: `context: 'fork'` — skill runs in sub-agent with separate context
- **Model override**: `model` field changes the model used for the skill
- **Effort level**: `effort` field controls thinking effort
- **Allowed tools**: `allowed-tools` restricts which tools the skill can use
- **Hooks**: `hooks` field registers pre/post hooks for the skill

**Major difference**: Go injects raw markdown content. Upstream has a rich content pipeline with argument substitution, variable expansion, shell execution, fork context, model selection, effort control, tool restrictions, and hooks.

### 11.7 Progressive Discovery

**Go** (`SkillTracker`):
- `IsNewSkill(name)` / `MarkShown(name)` — tracks which skills have been announced
- `GetUnsentSkills(allSkills)` — returns always-on + unshown skills
- `GenerateDiscoveryReminder(allSkills)` — generates reminder text like "You have 3 unread skill(s). Use search_skills to find skills..."
- `ResetPostCompact()` — clears read state, preserves shown state (prevents re-announcing ~4K token skill listing)

**Upstream**:
- **Conditional skills with `paths`**: skills start hidden, activate when matching files are touched
- **Dynamic skill discovery**: `discoverSkillDirsForPaths()` finds new skill directories as files are accessed
- **Skill change detection**: `skillChangeDetector.ts` detects new/changed skills between turns
- **Feature-gated discovery**: bundled skills can be gated by `isEnabled()` callbacks
- **No explicit "unread" counter** — upstream relies on the model discovering skills through the Skill tool and the skill listing in the system prompt

**Go's approach is more explicit** — a dedicated tracker with shown/read/used state, reminder generation, and post-compact reset. Upstream's approach is more **declarative** — conditional paths, dynamic directory discovery, and feature gates control what's visible.

### 11.8 MCP Skills

**Go**: **Not implemented**. No MCP skill discovery, no `skill://` resource handling.

**Upstream** (`mcpSkills.ts`):
- `fetchMcpSkillsForClient()` — discovers `skill://` resources from connected MCP servers
- Memoized by server name with LRU cache (size 20)
- Reads each resource, parses frontmatter, creates `Command` objects
- Skill names: `mcp__{normalizedServerName}__{resourcePath}`
- **Security**: MCP skills are treated as untrusted — shell execution (`!` blocks) is disabled in their content
- **Sanitization**: `recursivelySanitizeUnicode()` on content before parsing
- **Builder injection**: `mcpSkillBuilders.ts` provides `registerMCPSkillBuilders()` / `getMCPSkillBuilders()` to avoid circular dependencies

### 11.9 Skill Dependency Checking

**Go** (`Loader`):
- `requires` field — checked against:
  1. Environment variables (all-uppercase + underscore names)
  2. Executables in PATH (`exec.LookPath()`, with Windows extension handling)
  3. Files in workspace or builtin skill directories
- `extended_requires.bins` — checked via `existsInPath()` (with Windows `.exe`/`.cmd`/`.bat` fallback)
- `extended_requires.env` — checked via `os.Getenv()`
- Missing dependencies recorded in `MissingDeps []string` with prefixes: `"Missing: "`, `"CLI: "`, `"ENV: "`
- `CheckAvailability()` — returns `len(MissingDeps) == 0`
- `CheckDependencies(requires)` — public wrapper

**Upstream**:
- **No dependency checking** in the skill loading system
- `allowed-tools` — restricts which tools the skill can use (not a dependency, but a permission)
- `isEnabled()` — programmatic enable/disable for bundled skills
- Dependency management is **delegated to the skill content** — the skill's markdown instructions tell the model what's needed, and the model handles it

**Go has explicit dependency checking** with three types (files, CLI tools, environment variables). Upstream relies on the LLM and `isEnabled()` callbacks.

### Skills System Summary

| Aspect | Go | Upstream |
|--------|-----|----------|
| Skill directories | 2 (builtin + workspace) | 5+ (managed, user, project, additional, legacy) |
| Frontmatter parser | String-based (~10 fields) | Full YAML parser (~20+ fields) |
| `available` field | Yes (defaults true, dependency-gated) | No (uses `isEnabled`, `paths`, policy) |
| Conditional skills | No | Yes (`paths` frontmatter + `ignore` matching) |
| Dynamic discovery | No | Yes (walks up from file paths) |
| MCP skills | No | Yes (`skill://` resource discovery) |
| Bundled skills | No | Yes (programmatic registration + lazy file extraction) |
| Skill tracking | Dedicated `SkillTracker` class | Implicit in command pipeline |
| Content injection | Raw markdown | Rich pipeline (args, variables, shell, fork, model, effort) |
| Dependency checking | Explicit (files, CLI, env vars) | None (delegated to LLM + isEnabled) |
| Usage tracking | No | Yes (7-day half-life scoring, persistent) |
| Custom template/prompt | No | No (not applicable — skills use SKILL.md) |
| Feature gates | None | Multiple (GrowthBook flags, policy) |
| Skill context modes | No | Yes (inline/fork) |
| Hooks in skills | No | Yes (validated by HooksSchema) |
| Shell in skills | No | Yes (bash/powershell, `!` blocks) |
| Analytics | None | Extensive |
| Post-compact reset | Explicit (`ResetPostCompact()`) | Implicit (postCompactCleanup) |
| Code scale | ~784 lines | ~1900+ lines (loadSkillsDir + mcpSkills + bundledSkills + frontmatterParser) |

---


---

## Go Implementation
- `E:\Git\miniClaudeCode-go-github\filehistory.go:1-1095` — Complete implementation


---

## Upstream Implementation
- `E:\Git\claude-code-upstream\src\utils\fileHistory.ts:1-1115` — Main implementation
- `E:\Git\claude-code-upstream\src\utils\sessionStorage.ts:1092,2250-2339,2976-3032,3489-3917,4615-4683` — Session storage integration
- `E:\Git\claude-code-upstream\src\hooks\useFileHistorySnapshotInit.ts:1-24` — React hook for initialization
- `E:\Git\claude-code-upstream\src\components\MessageSelector.tsx:16-434` — UI for snapshot selection
- `E:\Git\claude-code-upstream\src\screens\REPL.tsx:319-326,3746-3750,4531,6254-6257` — Snapshot triggers and rewind UI
- `E:\Git\claude-code-upstream\src\cli\print.ts:303-307,4682-4717` — CLI rewind command
- `E:\Git\claude-code-upstream\src\utils\conversationRecovery.ts:25,465,576` — Conversation recovery with file history
- `E:\Git\claude-code-upstream\src\utils\cleanup.ts:312-342` — File history cleanup


---

## 1. Snapshot Creation — When Snapshots Are Taken

**Go** (filehistory.go:64-114, `TakeSnapshot`, `TakeSnapshotWithDesc`):
- Called explicitly by agent before file edits
- No automatic per-message snapshots
- Dedup: skips if content identical to last snapshot (same checksum) (line 87-96)
- Description field captures context: `"before edit_file"`, `"after write_file"` (line 21)

**Upstream** (fileHistory.ts:198-342, `fileHistoryMakeSnapshot`):
- Called **per message** — after each user turn completes (REPL.tsx:3746-3750, handlePromptSubmit.ts:542-549, QueryEngine.ts:645-653)
- `fileHistoryTrackEdit()` called **before** each file edit (fileHistory.ts:86-193) — captures pre-edit state
- Snapshot tracks ALL modified files since last snapshot (not just the edited one)
- Uses mtime checking to avoid re-backing unchanged files (line 257-269, `checkOriginFileChanged`)
- Controlled by `fileHistoryEnabled()` flag (line 63-78), which checks config and env vars

**Key difference**: Go takes snapshots on-demand before edits. Upstream takes snapshots per-message AND tracks every file edit in real-time. Upstream is much more comprehensive.


---

## 2. Snapshot Storage — In-Memory vs Disk

**Go** (filehistory.go:27-35, 1010-1033, 1036-1094):
- **Hybrid**: in-memory map + disk persistence
- In-memory: `map[string][]FileSnapshot` keyed by absolute path (line 31)
- Disk: `.claude/snapshots/` directory under working directory (line 45)
- Each snapshot = individual JSON file: `{timestamp}_{sanitized_path}.json` (line 1026)
- JSON format: `FileSnapshot{FilePath, Content, Timestamp, Description, Checksum, Deleted}`
- Lazy loading: in-memory first, fall back to disk (line 379, `loadFromDisk`)
- Auto-caching on disk load (line 1087-1091)

**Upstream** (fileHistory.ts:733-741, 748-798):
- **Disk-only for backup content**: `{configDir}/file-history/{sessionId}/{hash}@v{version}` (line 733-741)
- Backup filename: SHA256 hash of file path (first 16 chars) + `@v{version}` (line 725-731)
- Uses `copyFile()` (not read+write) for efficient binary copy (line 778)
- Preserves file permissions (line 786, `chmod`)
- State (snapshot metadata) is in-memory React state (`FileHistoryState`)
- Session storage integration for persistence across restarts (sessionStorage.ts:1092, 2250-2339)

**Key difference**:
- Go: Content stored inline in JSON files (text only, full content in JSON)
- Upstream: Content stored as actual file copies on disk (supports binary, preserves permissions)
- Upstream: Hash-based filenames (not path-based) — more robust for special characters
- Upstream: Per-session storage (`{sessionId}/` directory) — separate per conversation
- Go: Per-working-directory storage (`.claude/snapshots/`)


---

## 3. Snapshot Restoration — Undo/Rollback Mechanisms

**Go** (filehistory.go:155-197, 204-295, 303-357):
- `RewindTo(filePath, index)` — restore to specific snapshot index (line 155)
- `RestoreLast(filePath)` — restore to previous distinct version (line 286)
- `RewindSteps(filePath, steps)` — go back N distinct-content states (line 293)
- `Checkout(filePath, version)` — restore to specific version number (line 303)
- Before restoring: snapshots current state so redo is possible (line 169-180)
- Handles deleted files: restores by deleting if target was empty (line 183-188)
- `restoreInternal()` collapses by unique checksum, skipping tombstones (line 204-281)

**Upstream** (fileHistory.ts:347-397, 537-591, `fileHistoryRewind`, `applySnapshot`):
- `fileHistoryRewind(state, messageId)` — rewind to state at specific message (line 347)
- Restores ALL tracked files simultaneously (not per-file) (line 541-590, `applySnapshot`)
- Before restoring: does NOT create a snapshot of current state (no built-in redo)
- `fileHistoryCanRestore(state, messageId)` — check if restore is possible (line 399)
- `fileHistoryGetDiffStats(state, messageId)` — preview changes before applying (line 414)
- `fileHistoryHasAnyChanges(state, messageId)` — lightweight boolean check (line 494)
- Uses `checkOriginFileChanged()` for stat-based change detection (line 600-634)

**Key difference**:
- Go: Per-file restoration with granular version control (index, steps, checkout)
- Upstream: Full-state restoration (all tracked files) to a message's snapshot
- Go: Auto-snapshots before restore (redo support); Upstream: no auto-snapshot before rewind
- Upstream: Has diff-stats preview; Go: no preview mechanism


---

## 4. Snapshot Metadata — Timestamps, Annotations

**Go** (filehistory.go:17-24, 588-659):
- `FileSnapshot` struct: `FilePath, Content, Timestamp, Description, Checksum, Deleted`
- Tags: `[tagname]` embedded in description (line 564, `AddTag`)
- Annotations: `AnnotateSnapshot(filePath, version, message)` appends with ` | ` separator (line 606)
- FNV-128a checksum for dedup (line 56-58)
- Version numbers: 1-indexed among active (non-deleted) snapshots

**Upstream** (fileHistory.ts:33-52, 39-43):
- `FileHistorySnapshot`: `messageId (UUID), trackedFileBackups, timestamp`
- `FileHistoryBackup`: `backupFileName, version, backupTime`
- No user-facing tags or annotations
- Version numbering is per-file, incrementing (v1, v2, v3...)
- `snapshotSequence` counter for activity signaling (line 51)
- No checksum-based dedup (relies on mtime checking)

**Key difference**: Go has rich metadata (tags, annotations, descriptions, checksums). Upstream is minimal (message linkage, version number, timestamp).


---

## 5. Snapshot Listing — Viewing Available Snapshots

**Go** (filehistory.go:361-380, 467-485, 383-396):
- `ListSnapshots(filePath)` — all snapshots for one file (line 361)
- `ListAllFiles()` — all files with snapshots (line 467)
- `SnapshotCount(filePath)` — count of non-deleted snapshots (line 383)
- `GetTimeline(since)` — cross-file chronological timeline (line 843)
- Loads from memory first, falls back to disk scan

**Upstream**:
- No direct "list snapshots" API
- Snapshots are accessed via `state.snapshots[]` array (in-memory React state)
- `MessageSelector.tsx:142-150` displays diff stats per message in UI
- `fileHistoryCanRestore(state, messageId)` checks if a message has a restorable snapshot (line 399)

**Key difference**: Go has explicit listing APIs with timeline view. Upstream exposes snapshots implicitly through message history UI.


---

## 6. Diff Generation — Comparing Current vs Snapshot

**Go** (filehistory.go:886-973, `Search`):
- Line-level diff via set difference (lines 909-953)
- `Search(filePath, pattern, mode, ignoreCase)` — finds versions where text was added/removed/changed (line 886)
- Modes: `SearchAdded`, `SearchRemoved`, `SearchChanged` (lines 872-876)
- Uses `regexp` for pattern matching in changed lines (line 901)
- Returns `HistorySearchResult{Version, Lines}` per matching version

**Upstream** (fileHistory.ts:414-484, 677-723, `fileHistoryGetDiffStats`, `computeDiffStatsForFile`):
- Uses `diffLines()` from `diff` npm package (line 705)
- Returns `{filesChanged, insertions, deletions}` (line 483)
- `fileHistoryHasAnyChanges()` — boolean-only fast check (line 494)
- `computeDiffStatsForFile()` — per-file line diff stats (line 677)
- Preview before applying rewind (UI shows "N files changed, +X -Y")

**Key difference**: Go's Search is pattern-driven (find versions containing a pattern change). Upstream's diff is count-driven (lines added/removed). Go's approach is more useful for finding when a specific change was made.


---

## 7. Cross-File Timeline — Go's Timeline Feature

**Go** (filehistory.go:833-867, `GetTimeline`):
- Returns all snapshots across ALL files, sorted chronologically (line 843)
- `TimelineEntry{Timestamp, FilePath, Version, Description}`
- Optional `since` parameter for time-filtered queries (line 842)
- Used for session-wide change history

**Upstream**:
- **NO equivalent timeline feature**
- Snapshots are organized by message ID, not chronologically across files
- Session-level view via message history, not file-level timeline

**Key difference**: Only Go has a cross-file timeline. This is a unique Go feature.


---

## 8. Batch Operations — Bulk Snapshot/Restore

**Go** (filehistory.go:399-445):
- `Clear()` — remove all in-memory snapshots (line 399)
- `ClearPath(filePath)` — remove all snapshots for one file (line 406)
- `ClearUnderDir(dir)` — remove all snapshots for files under a directory (line 421)
- `SearchTagAll(tag)` — find tagged versions across ALL files (line 807)
- Bulk tag operations across file tree

**Upstream**:
- `copyFileHistoryForResume()` — batch copy all backups between sessions (line 922-1046)
- Uses hard links with fallback to copy (line 979, 1001)
- Migrates entire snapshot chain for session resume
- No "clear all snapshots" API

**Key difference**: Go has batch clear and cross-file search operations. Upstream has batch copy for session resume.


---

## 9. Tagging System — Snapshot Tags

**Go** (filehistory.go:565-831):
- `AddTag(filePath, tag)` — tag latest snapshot (line 565)
- `ListTags(filePath)` — list all tags for a file (line 668)
- `SearchTag(filePath, tag)` — find versions by tag in one file (line 710)
- `SearchTagAll(tag)` — find tagged versions across all files (line 807)
- `RemoveTag(filePath, version, tag)` — remove tag from specific version (line 737)
- `ResolveVersion(filePath, spec)` — resolve "v3", "last2", "current", or tag name (line 489)
- Tags stored as `[tagname]` in description field

**Upstream**:
- **NO tagging system**
- Snapshots identified only by message ID

**Key difference**: Go has a full tagging system. Upstream has none. This is a major architectural difference.


---

## 10. Auto-Snapshot — Before Write/Edit

**Go**:
- No auto-snapshot mechanism in filehistory.go
- Snapshots taken explicitly by caller before edits

**Upstream** (fileHistory.ts:86-193, `fileHistoryTrackEdit`):
- `fileHistoryTrackEdit()` is called before every file edit
- Creates backup of current file content automatically
- 3-phase approach: (1) check if already tracked, (2) async backup, (3) commit to state (line 100-192)
- Race-condition safe: re-checks tracked status before committing (line 132-192)
- Handles new files (ENOENT = null backup) (line 129, 767-768)
- Dedup: skips if already tracked in current snapshot (line 114-118)

**Key difference**: Upstream has automatic pre-edit tracking built into the edit pipeline. Go relies on explicit calls.


---

## Summary Table: File History

| Feature | Go | Upstream |
|---------|----|----------|
| Snapshot trigger | Explicit call | Per-message + per-edit |
| Content storage | JSON files (inline text) | File copies (binary safe) |
| Storage location | `.claude/snapshots/` | `{configDir}/file-history/{session}/` |
| Per-file restore | Yes (4 methods) | No (all-or-nothing) |
| Redo support | Yes (auto-snapshot before restore) | No |
| Tags | Yes (full system) | No |
| Annotations | Yes | No |
| Timeline | Yes (cross-file) | No |
| Diff preview | Search by pattern | Line count stats |
| Session resume | No | Yes (backup copy between sessions) |
| Checksum dedup | Yes (FNV-128a) | No (mtime-based) |
| Config flag | Always on | `fileCheckpointingEnabled` |
| Permission check | Sensitive dirs list | Tool permission context |


---


---

## Section XXII: Deep Dive - Tool Result Persistence, Cached Microcompact, API Context Management, Cost Tracking (Round 26)

### 34. Tool Result Persistence with ContentReplacementState

**Upstream Files**: `E:\Git\claude-code-upstream\src\utils\toolResultStorage.ts` (1041 lines)
**Go File**: `E:\Git\miniClaudeCode-go-github\agent_loop.go:2184-2205` (`truncateOutput`)

#### 34.1 Per-tool persistence vs in-memory truncation

| Aspect | Go (file:line) | Upstream (file:line) | Type |
|--------|----------------|---------------------|------|
| Large result handling | `truncateOutput()`: keeps 80% head + 20% tail with `[OUTPUT TRUNCATED]` marker (agent_loop.go:2184-2205) | `persistToolResult()`: writes full result to `projectDir/sessionId/tool-results/{id}.txt`, replaces with `<persisted-output>` XML tag containing 2KB preview + file path (toolResultStorage.ts:137-184) | 缺失 |
| Per-tool threshold | Fixed 50000 chars (agent_loop.go:400 `maxToolChars`) | `getPersistenceThreshold()`: GrowthBook override map `tengu_satin_quoll` per-tool, capped by `Math.min(declaredMax, 50000)`. Tools with `Infinity` (Read) opt out entirely (toolResultStorage.ts:55-78) | 缺失 |
| Empty result handling | No special handling | `isToolResultContentEmpty()` injects `({toolName} completed with no output)` to prevent capybara models from emitting `\n\nHuman:` stop sequences (toolResultStorage.ts:250-295) | 缺失 |
| Preview generation | No preview (just truncates) | `generatePreview()`: finds last newline within 2000 bytes to avoid cutting mid-line (toolResultStorage.ts:339-356) | 简化 |
| EEXIST idempotency | N/A (in-memory) | Uses `flag: wx` to skip re-writing on prior turns (toolResultStorage.ts:157-172) | 缺失 |

#### 34.2 Per-message aggregate budget

| Aspect | Go | Upstream (toolResultStorage.ts:769-936) | Type |
|--------|-----|----------------------------------------|------|
| Aggregate budget enforcement | No per-message aggregate check | `enforceToolResultBudget()`: tracks `MAX_TOOL_RESULTS_PER_MESSAGE_CHARS` (200,000). Groups parallel tool results by API-level user message (toolResultStorage.ts:675-692) | 缺失 |
| Wire-level message grouping | N/A | `collectCandidatesByMessage()`: groups by runs of user messages NOT separated by assistant messages (toolResultStorage.ts:600-639) | 缺失 |

#### 34.3 ContentReplacementState for prompt cache stability

| Aspect | Go | Upstream (toolResultStorage.ts:386-463, 960-1012) | Type |
|--------|-----|--------------------------------------------------|------|
| Replacement state | Go re-truncates every turn - different output each time breaks prompt cache | `ContentReplacementState`: `seenIds` Set + `replacements` Map. Once seen, fate is frozen (toolResultStorage.ts:390-412) | 缺失 |
| Resume reconstruction | N/A | `reconstructContentReplacementState()`: loads from transcript records on resume (toolResultStorage.ts:960-988) | 缺失 |
| Subagent fork inheritance | N/A | `reconstructForSubagentResume()`: fills gaps from parent live replacements (toolResultStorage.ts:1001-1012) | 缺失 |
| GrowthBook feature gates | N/A | `tengu_hawthorn_steeple` (enables), `tengu_hawthorn_window` (budget override) (toolResultStorage.ts:447-463) | 缺失 |

#### 34.4 Pre-API budget enforcement

| Aspect | Go | Upstream (query.ts, toolResultStorage.ts:924-936) | Type |
|--------|-----|-------------------------------------------|------|
| Pre-API enforcement | Tool results only truncated at execution time | `applyToolResultBudget()`: called in query loop BEFORE each API call (toolResultStorage.ts:924-936) | 缺失 |

### 35. Cached Microcompact - cache_edits vs client-side clearing

**Upstream Files**: `E:\Git\claude-code-upstream\src\services\compact\cachedMicrocompact.ts` (113 lines), `microCompact.ts` (531 lines)
**Go File**: `E:\Git\miniClaudeCode-go-github\compact.go:2706-2862` (`CachedMicrocompactTracker`)

#### 35.1 Cache editing API vs content mutation

| Aspect | Go (compact.go) | Upstream (cachedMicrocompact.ts, microCompact.ts) | Type |
|--------|-----------------|--------------------------------------------------|------|
| Deletion approach | Go tracks registered tools but does not produce cache_edits | Upstream creates `CacheEditsBlock` with `type: delete_tool_result` entries, sent via `cache_edits` body parameter (microCompact.ts:305-399) | 缺失 |
| Pinned edits for prefix stability | `pinnedEdits []any` declared but never populated (compact.go:2711) | `pinCacheEdits()`: records position of cache_edits blocks (microCompact.ts:111-118) | 缺失 |
| Baseline cache deleted tokens | Go does not track `cache_deleted_input_tokens` | `PendingCacheEdits` includes `baselineCacheDeletedTokens` (microCompact.ts:207-213) | 缺失 |
| Deferred boundary message | Go does not create microcompact boundary messages | Upstream defers boundary message until AFTER API response (microCompact.ts:371-394) | 缺失 |

#### 35.2 Feature gates and model gating

| Aspect | Go | Upstream | Type |
|--------|-----|----------|------|
| Env var gate | `NewCachedMicrocompactTracker()` creates state unconditionally | `isCachedMicrocompactEnabled()`: requires `CLAUDE_CACHED_MICROCOMPACT=1` env var AND `feature(CACHED_MICROCOMPACT)` build flag (cachedMicrocompact.ts:26-28) | 缺失 |
| Model gating | Registers all compactable tools regardless of model | `isModelSupportedForCacheEditing()`: returns true only for `claude-[a-z]+-4[-\d]` (cachedMicrocompact.ts:33-35) | 缺失 |
| Query source routing | All compaction treats every request identically | Cached MC only runs for `isMainThreadSource(querySource)` (microCompact.ts:249-286) | 缺失 |

#### 35.3 Time-based microcompact

| Aspect | Go (compact.go) | Upstream (microCompact.ts:412-530) | Type |
|--------|-----------------|-----------------------------------|------|
| Time-based trigger | Go has no time-based tool result clearing | `maybeTimeBasedMicrocompact()`: fires when gap > configured threshold (default 60min) | 缺失 |
| Config from GrowthBook | `maxTools: 10`, `keepRecent: 5` hardcoded | `getTimeBasedMCConfig()`: reads `{enabled: false, gapThresholdMinutes: 60, keepRecent: 5}` (timeBasedMCConfig.ts) | 缺失 |
| Cache break notification | Go does not track prompt cache hit rates | `notifyCacheDeletion()` called after microcompact (microCompact.ts:362-367, 525-527) | 缺失 |
| Compact warning suppression | Go prints compaction status unconditionally | `suppressCompactWarning()` / `clearCompactWarningSuppression()` (compactWarningState.ts:1-19) | 缺失 |

### 36. API Context Management - per-call vs compact-only

**Upstream File**: `E:\Git\claude-code-upstream\src\services\compact\apiMicrocompact.ts` (151 lines), `claude.ts:1684-1779`
**Go File**: `E:\Git\miniClaudeCode-go-github\compact.go:1603-1611`

#### 36.1 Context management sent with every API call vs only during compact

| Aspect | Go | Upstream (claude.ts:1684-1779) | Type |
|--------|-----|-------------------------------|------|
| When sent | Only during `doCompactLLMCall()` (compact.go:1606) | Sent with EVERY API call via `paramsFromContext()` (claude.ts:1775-1779) | 简化 |
| Strategies configured | Fixed: `clear_tool_uses_20250919` + `clear_thinking_20251015` | Dynamic: `clear_thinking_20251015` with `keep: all` or `keep: {value: 1}` based on idle time (apiMicrocompact.ts:64-150) | 简化 |
| Env var control | No control | `USE_API_CLEAR_TOOL_RESULTS` (default true), `USE_API_CLEAR_TOOL_USES` (default false) (apiMicrocompact.ts:91-96) | 缺失 |
| Thinking clear strategy | Always clears all thinking blocks | `clearAllThinking` logic: when >1h idle, keep only last thinking turn (apiMicrocompact.ts:79-87) | 简化 |

### 37. Cost Tracking and Session Persistence

**Upstream File**: `E:\Git\claude-code-upstream\src\cost-tracker.ts` (324 lines)
**Go File**: `E:\Git\miniClaudeCode-go-github\agent_loop.go:311-325` (token counters only)

#### 37.1 Per-model cost tracking vs raw token counters

| Aspect | Go (agent_loop.go) | Upstream (cost-tracker.ts) | Type |
|--------|--------------------|---------------------------|------|
| Cost calculation | Tracks `totalInputTokens` + `totalOutputTokens`. No USD calculation | `addToTotalSessionCost()`: per-model tracking with `calculateUSDCost()` (cost-tracker.ts:278-323) | 缺失 |
| Advisor cost tracking | No concept of advisor models | Recursive cost tracking for advisor models (cost-tracker.ts:304-321) | 缺失 |
| Session cost persistence | No persistence across sessions | `saveCurrentSessionCosts()`: writes cost state to project config. `restoreCostStateForSession()` restores on resume (cost-tracker.ts:130-175) | 缺失 |
| Per-model aggregation | No model-level breakdown | `formatModelUsage()`: accumulates by canonical model name (cost-tracker.ts:181-226) | 缺失 |
| Lines changed tracking | No tracking | `addToTotalLinesChanged()`, `getTotalLinesAdded()`, `getTotalLinesRemoved()` (cost-tracker.ts:54-56) | 缺失 |
| Fast mode cost attribution | No attribution | Fast mode tracked with `speed: fast` attribute (cost-tracker.ts:287-289) | 缺失 |
| Unknown model cost flag | No handling | `hasUnknownModelCost()`: warns when costs may be inaccurate (cost-tracker.ts:229-233) | 缺失 |
| Web search request tracking | No tracking | `getTotalWebSearchRequests()` (cost-tracker.ts:271) | 缺失 |

### 38. Token Budget Parsing

**Upstream File**: `E:\Git\claude-code-upstream\src\utils\tokenBudget.ts` (74 lines)
**Go File**: N/A

#### 38.1 Token budget syntax parsing

| Aspect | Go | Upstream (tokenBudget.ts) | Type |
|--------|-----|--------------------------|------|
| Budget syntax | No token budget concept | Parses 3 formats: `+500k` (start), `+500k.` (end), `use 2M tokens` (anywhere) (tokenBudget.ts:1-29) | 缺失 |
| Budget continuation message | N/A | `getBudgetContinuationMessage()`: Stopped at 75% of token target... (tokenBudget.ts:66-73) | 缺失 |
| Budget position tracking | N/A | `findTokenBudgetPositions()`: returns start/end positions for stripping (tokenBudget.ts:31-64) | 缺失 |

### 39. Permission System - Deep Dive (Additional Findings)

**Upstream Files**: `E:\Git\claude-code-upstream\src\utils\permissions\permissions.ts` (1509 lines), `PermissionContext.ts` (389 lines), `denialTracking.ts` (46 lines)
**Go File**: `E:\Git\miniClaudeCode-go-github\permissions.go`

#### 39.1 DontAsk mode - entirely missing

| Aspect | Go (permissions.go) | Upstream (permissions.ts:505-517) | Type |
|--------|---------------------|--------------------------------------|------|
| DontAsk mode | Go has `ModeAsk`, `ModeAuto`, `ModeBypass`, `ModePlan` | `dontAsk` mode: converts ALL `ask` to `deny` for headless/non-interactive sessions (permissions.ts:508-517) | 缺失 |

#### 39.2 Auto mode: plan+auto hybrid - missing from Go

| Aspect | Go (permissions.go) | Upstream (permissions.ts:520-524) | Type |
|--------|---------------------|--------------------------------------|------|
| Plan+auto hybrid | Plan mode always read-only | Plan mode can activate auto classifier when `autoModeStateModule.isAutoModeActive()` (permissions.ts:523-524) | 缺失 |

#### 39.3 acceptEdits fast-path - missing from Go

| Aspect | Go | Upstream (permissions.ts:600-656) | Type |
|--------|-----|----------------------------------|------|
| acceptEdits fast-path | No acceptEdits mode | Before classifier, checks if action allowed in `acceptEdits` mode. Auto-approves without classifier call. Skips for Agent and REPL (permissions.ts:600-656) | 缺失 |

#### 39.4 Permission hook infrastructure - Go has no equivalent

| Aspect | Go (permissions.go) | Upstream (PermissionContext.ts:216-263) | Type |
|--------|---------------------|----------------------------------------|------|
| Hook-based permission decisions | `askUserWithWarning()` reads from stdin | `PermissionContext.runHooks()`: iterates `executePermissionRequestHooks()` - hooks can allow/deny/modify before prompt (PermissionContext.ts:216-263) | 缺失 |
| Headless agent hook path | No hook support | `runPermissionRequestHooksForHeadlessAgent()`: hooks allow/deny before auto-deny (permissions.ts:400-471) | 缺失 |

#### 39.5 Permission persistence - Go has no persistent rule management

| Aspect | Go (permissions.go) | Upstream (PermissionContext.ts:139-147, permissions.ts:1343-1493) | Type |
|--------|---------------------|-------------------------------------------------------------|------|
| Rule persistence | `askUserWithWarning()` returns bool only | `handleUserAllow()`: calls `persistPermissionUpdates()` - writes rules to settings files (PermissionContext.ts:291-318) | 缺失 |
| Rule deletion | N/A | `deletePermissionRule()`: deletes from localSettings/userSettings/projectSettings (permissions.ts:1351-1392) | 缺失 |
| Disk sync | N/A | `syncPermissionRulesFromDisk()`: clears all disk-based source:behavior combos before applying new rules (permissions.ts:1441-1493) | 缺失 |
| Managed-only mode | N/A | `shouldAllowManagedPermissionRulesOnly()`: clears all non-policy sources (permissions.ts:1448-1468) | 缺失 |

#### 39.6 Bypass inheritance in plan mode - Go hard-blocks vs upstream inherits

| Aspect | Go (permissions.go:299-306) | Upstream (permissions.ts:1290-1303) | Type |
|--------|---------------------------|--------------------------------------|------|
| Plan mode bypass | `ModePlan` blocks write tools with no exceptions | Plan mode checks `isBypassPermissionsModeAvailable` - inherits bypass if user started with bypass mode (permissions.ts:1290-1303) | 简化 |

#### 39.7 Sandbox auto-allow for ask rules - missing from Go

| Aspect | Go (permissions.go:192-222) | Upstream (permissions.ts:1206-1228) | Type |
|--------|---------------------------|--------------------------------------|------|
| Sandbox auto-allow | `findToolLevelAsk()` always prompts for tool-level ask rules | `canSandboxAutoAllow`: if Bash, sandboxing enabled, `autoAllowBashIfSandboxed` on, command would be sandboxed - ask rule SKIPPED (permissions.ts:1211-1228) | 缺失 |

### 40. Denial Tracking Integration with Classifier

**Upstream Files**: `E:\Git\claude-code-upstream\src\utils\permissions\denialTracking.ts` (46 lines), `permissions.ts:555-948`
**Go File**: `E:\Git\miniClaudeCode-go-github\permissions.go:463-486`

#### 40.1 Denial tracking scope

| Aspect | Go (permissions.go) | Upstream (denialTracking.ts, permissions.ts) | Type |
|--------|---------------------|---------------------------------------------|------|
| Tracking scope | `denialCount` resets on success, fallback at 3; `totalDenialCount` forces interactive at 20 (permissions.go:463-486) | `DenialTrackingState` with `consecutiveDenials` + `totalDenials`. `DENIAL_LIMITS` configurable via GrowthBook (denialTracking.ts:1-45) | Go适配 |
| Local vs global tracking | Single counter on AgentLoop | `context.localDenialTracking` for async subagents, `appState.denialTracking` for main thread (permissions.ts:985-1000) | 缺失 |
| Success resets consecutive | Yes | `recordSuccess()` returns same reference when consecutiveDenials=0 (no-op) (denialTracking.ts:32-38) | Go适配 |
| Denial limit handling | Forces interactive mode | `handleDenialLimitExceeded()`: returns `ask` decision with warning. In headless mode, throws `AbortError` (permissions.ts:1006-1080) | 简化 |

---

*End of Section XXII. Total new table rows: 52 across 7 subsections covering tool result persistence, cached microcompact, API context management, cost tracking, token budget parsing, permission system deep dive, and denial tracking integration.*


---

## 38. Session Memory, Skills, File History, Context References

### Files Compared
- **Go**: `session_memory.go`, `skills/loader.go`, `skills/tracker.go`, `filehistory.go`, `context_references.go`
- **Upstream**: `services/SessionMemory/`, `commands/skills/`, `utils/fileHistory.ts`, `utils/context.ts`

### 38.1 Session Memory

| # | Aspect | Go (`session_memory.go`) | Upstream (`sessionMemory.ts`) | Type |
|---|--------|-------------------------|-----------------------------|------|
| 1 | Section template | Same 10-section format — matching upstream template | 10-section: Title, Current State, Tasks, Files, Workflow, Errors, Docs, Learnings, Results, Worklog | 匹配 |
| 2 | Extraction thresholds | `minimumMessageTokensToInit=10000`, `minimumTokensBetweenUpdate=5000`, `toolCallsBetweenUpdates=3` | Same thresholds | 匹配 |
| 3 | Token budgets | `maxTokensPerSection=20000`, `maxTotalSessionMemoryTokens=60000` | `MAX_SECTION_LENGTH = 2000`, `MAX_TOTAL_SESSION_MEMORY_TOKENS = 12000` | Go适配 (10x higher) |
| 4 | Per-category limits | Per-category max entries (state=20, decision=30, preference=20, reference=50, test=20) with expiration (state=7d, others=30d) | No explicit per-category limits or expiration | Go增强 |
| 5 | File write | Atomic write (tmp + rename), 30-second flush loop | `writeFile` directly | Go适配 (Windows safety) |
| 6 | Wait timeout | 60s stale threshold, 60s extraction wait | `EXTRACTION_WAIT_TIMEOUT_MS = 15000`, `EXTRACTION_STALE_THRESHOLD_MS = 60000` | 简化 (Go: 60s wait vs 15s) |

### 38.2 Skills System

| # | Aspect | Go (`skills/loader.go`, `skills/tracker.go`) | Upstream (`commands/skills/`) | Type |
|---|--------|--------------------------------------|----------------------------|------|
| 1 | Frontmatter parsing | Pure string-based YAML parsing (no yaml library) | Likely uses yaml library | Go适配 |
| 2 | Progressive discovery | `ResetPostCompact` resets `readSkills`, preserves `shownSkills` and `usedSkills` | Same preservation pattern across compacts | 匹配 |
| 3 | Skill tracking | `SkillMeta` with `ExtBins`, `ExtEnv` for extended requires | `requires` with similar semantics | 匹配 |
| 4 | Sorting | `GetReadSkillNames` sorted by read time descending | Sorted by `invokedAt` descending | 匹配 |

### 38.3 File History

| # | Aspect | Go (`filehistory.go`) | Upstream (`fileHistory.ts`) | Type |
|---|--------|----------------------|---------------------------|------|
| 1 | Max snapshots | 50 max, 7-day max age, 5 min keep | `MAX_SNAPSHOTS = 100` | 简化 |
| 2 | Dedup hash | FNV-128a checksum | sha256 hash for backup naming | Go适配 |
| 3 | Version resolution | `RewindTo`, `RestoreLast`, `RewindSteps`, `Checkout` with "v3"/"3", "current"/"latest", "tag names" | `fileHistoryRewind` by messageId only | Go增强 |
| 4 | Tag system | `AddTag`, `AnnotateSnapshot`, `RemoveTag`, `ListTags`, `SearchTag` | No tag system | Go增强 |
| 5 | Storage | JSON metadata files under `.claude/snapshots/` | Individual backup files under `{configDir}/file-history/{sessionId}/` | Go适配 |
| 6 | Timeline/Search | `Timeline` across all files, `Search` by pattern (added/removed/changed lines) | `fileHistoryGetDiffStats` for insertions/deletions | Go增强 |

### 38.4 Context References

| # | Aspect | Go (`context_references.go`) | Upstream (`utils/context.ts`) | Type |
|---|--------|---------------------------|----------------------------|------|
| 1 | Reference types | `@file`, `@folder`, `@diff`, `@staged`, `@git:N`, `@url` with regex pattern matching | Same types handled through attachment system | 匹配 |
| 2 | Token budgets | 25% soft, 50% hard budgets | Managed through attachment system | 匹配 |
| 3 | Sensitive directory blocking | .ssh, .aws, .gnupg blocked | Similar path-based blocking | 匹配 |
| 4 | Email/social exclusion | Manual exclusion (Go lacks lookbehind) | Regex lookbehind | Go适配 |


---

