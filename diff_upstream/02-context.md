# Context Management

> Context window, context references, entry system

## Sections Included
- [##] Line 3710-3810 -- ## Part 1: Context Management
- [##] Line 3894-3956 -- ## 8. context.go — 上下文管理（ConversationContext + ToolStateTracker）
- [##] Line 4139-4214 -- ## 13. context_references.go — @context 展开
- [##] Line 7007-7009 -- ## Go Implementation
- [##] Line 7010-7018 -- ## Upstream Implementation
- [##] Line 7019-7046 -- ## 1. Supported Reference Types
- [##] Line 7047-7073 -- ## 2. File Expansion — Reading and Injecting File Content
- [##] Line 7074-7095 -- ## 3. Folder Expansion — Directory Listing with Content
- [##] Line 7096-7112 -- ## 4. Diff Expansion — Git Diff Injection
- [##] Line 7113-7123 -- ## 5. Staged Expansion — Git Staged Changes
- [##] Line 7124-7136 -- ## 6. Git Log Expansion — Commit History
- [##] Line 7137-7154 -- ## 7. URL Expansion — Web Content Fetching
- [##] Line 7155-7174 -- ## 8. Token Budget for Expansion
- [##] Line 7175-7192 -- ## 9. Safety Checks — Path Validation, Size Limits
- [##] Line 7193-7215 -- ## 10. Error Handling — Missing Files, Failed URLs
- [##] Line 7417-7433 -- ## Summary Table: Context References
- [##] Line 9351-9406 -- ## 34. Context.go Deep Dive — Entry System & API Message Building

---

## Content

## Part 1: Context Management

### 1. Entry Type System

| Go (`context.go`) | Upstream (`src/types/message.ts`, `src/utils/messages.ts`) |
|---|---|
| `EntryContent` sealed interface with: `TextContent`, `ToolUseContent`, `ToolResultContent`, `CompactBoundaryContent`, `SummaryContent`, `AttachmentContent` | Rich discriminated union `Message` type with `type` field: `user`, `assistant`, `system`, `progress`, `attachment`, `system_informational`, `system_compact_boundary`, `system_away_summary`, `system_agents_killed`, `system_api_error`, `system_api_metrics`, `system_microcompact_boundary`, `system_permission_retry`, `system_scheduled_task_fire`, `system_stop_hook_summary`, `system_turn_duration`, `system_thinking`, `system_bridge_status`, `system_memory_saved`, `system_local_command`, `tombstone`, `hook_result`, `grouped_tool_use`, `collapsible` |
| Single `conversationEntry` struct wraps `EntryContent` + `role` + `summarized bool` | Each message type has distinct interface with `uuid: UUID`, `timestamp`, `origin`, `isMeta`, `isCompactSummary`, `isVisibleInTranscriptOnly`, `message.id`, `parentUuid`, and many type-specific fields |
| Compact boundary is a `CompactBoundaryContent` struct with `Trigger`, `PreCompactTokens`, `UUID`, `PreCompactDiscoveredTools`, `PreservedSegment` | `SystemCompactBoundaryMessage` with `compactMetadata` containing `preCompactDiscoveredTools`, `preservedSegment` (`headUuid`, `anchorUuid`, `tailUuid`), `userFeedback`, `messagesSummarized` |
| Summary is `SummaryContent` (string) | Summary is a `UserMessage` with `isCompactSummary: true`, `isVisibleInTranscriptOnly: true`, and optional `summarizeMetadata` |
| Attachment is `AttachmentContent` (string) | `AttachmentMessage<Attachment>` with typed attachments: `file_content`, `invoked_skills`, `plan_mode`, `plan_file_reference`, `task_status`, `agent_listing_delta`, `deferred_tools_delta`, `mcp_instructions_delta`, etc. |
| **Gap**: Go has no `ProgressMessage`, `ThinkingMessage`, `TombstoneMessage`, `HookResultMessage`, `SystemAPIErrorMessage`, `SystemAgentsKilledMessage` | Upstream has many system message types for UI state tracking |

### 2. Token Estimation

| Go | Upstream |
|---|---|
| `EstimateTokens(string)` in `tokenizer.go` — uses `DetectContentType` + fixed ratios (code: 3 chars/token, natural: 4 chars/token, JSON: 3.5 chars/token), applies 4/3 safety margin in `EstimatedTokens()` | **Two-tier system**: (1) `countTokensWithAPI()` — calls `anthropic.beta.messages.countTokens` API for exact counts. (2) `roughTokenCountEstimation()` — fallback using `content.length / bytesPerToken` (default 4 bytes/token, 2 for JSON). |
| `EstimatedTokens()` is content-type-aware: walks each entry type, classifies text, sums with appropriate ratios | `tokenCountWithEstimation()` is smarter: walks messages backwards, finds last assistant with real `usage` data, uses `input_tokens + cache_tokens + output_tokens` as anchor point, then estimates the *delta* messages between that anchor and end |
| 4/3 margin applied uniformly | `estimateMessageTokens()` in `microCompact.ts` also applies 4/3 pad (`Math.ceil(totalTokens * 4/3)`) |
| **No API token counting** — Go never calls the count-tokens API | Upstream can call `anthropic.beta.messages.countTokens` for exact counts, falls back to rough estimation when provider (Bedrock/Vertex) doesn't support it |
| Go estimates per-content-type using heuristic classification | Upstream also has `bytesPerTokenForFileType()` which uses 2 bytes/token for JSON/JSONL/JSONC |

### 3. BuildMessages() / API Message Construction

| Go | Upstream |
|---|---|
| `BuildMessages()` scans backwards for last `CompactBoundaryContent`, takes `entries[boundaryIdx:]`, converts each to `anthropic.MessageParam` | `getMessagesAfterCompactBoundary()` uses `findLastCompactBoundaryIndex()`, slices from boundary, then optionally applies `projectSnippedView()` (HISTORY_SNIP feature) |
| Compact boundaries are **not** sent to API (`continue` in switch) | Compact boundaries are **not** sent to API (same behavior) |
| Merges consecutive same-role messages after conversion | `normalizeMessagesForAPI()` handles role merging, tool pairing, thinking block merging by `message.id` |
| System prompt is a separate `c.systemPrompt` field | System prompt composed via `asSystemPrompt()` from `getSystemContext()`, `getUserContext()`, CLAUDE.md, tool lists |
| **Gap**: Go has no `normalizeMessagesForAPI()` — no tool search field stripping, no incomplete tool call filtering | Upstream's `normalizeMessagesForAPI()` strips `caller` from tool_use, strips `tool_reference` from tool_result, handles `defer_loading` tools |
| **Gap**: Go has no `ensureToolResultPairing()` — instead has `ValidateToolPairing()` | Upstream's `ensureToolResultPairing()` in `claude.ts` is the authoritative tool repair function |

### 4. Truncation

| Go | Upstream |
|---|---|
| Three levels: `TruncateHistory()` (keep first + last 10), `AggressiveTruncateHistory()` (first + last 5), `MinimumHistory()` (first + last 2) | No equivalent naive truncation functions. Upstream relies on compaction + message selector UI |
| All three are compact-boundary-aware via `truncateWithBoundary()` | Upstream has `truncateHeadForPTLRetry()` — drops oldest API-round groups when compact request hits prompt-too-long (last-resort escape hatch) |
| Go's truncation can orphan tool results, fixed by `ValidateToolPairing()` | Upstream uses `adjustIndexToPreserveAPIInvariants()` to prevent splitting tool pairs when calculating preserved tail |
| `truncateIfNeeded()` fires on every message add when over `MaxContextMsgs` | Upstream has no message-count-based truncation — relies on token-based auto-compact |

### 5. CompactContext — Degradation Phases

| Go | Upstream |
|---|---|
| **4-phase degradation**: Phase 1: `Compact()` (round-based, keeps last N rounds). Phase 2: `SmartCompact()` (turn-based, keeps first 2 + last 2 turns). Phase 3: `SelectiveCompact()` (clears readable tool outputs). Phase 4: Hard truncate | **No degradation chain.** Single strategy: `compactConversation()` sends messages to an LLM summarizer. If it fails (prompt-too-long), retries with `truncateHeadForPTLRetry()`. |
| Go compacts **locally** using algorithmic strategies (no LLM call) | Upstream compacts by **calling the API** — sends full conversation + summary prompt to a forked agent (`runForkedAgent` with cache sharing) or streaming fallback |
| Go has no LLM-assisted summarization | Upstream's `streamCompactSummary()` uses `queryModelWithStreaming()` or cache-sharing forked agent to generate AI summary |
| Upstream also has **session memory compaction** (`trySessionMemoryCompaction()`) as a first strategy before legacy compact | Go has no session memory compaction equivalent |
| Go's compact is **synchronous and deterministic** | Upstream's compact is **async, can fail, has retry loops** (PTL retry up to 3x, streaming retry up to 2x) |

### 6. MicroCompactEntries

| Go | Upstream |
|---|---|
| `MicroCompactEntries()` — mutates messages in-place, replaces old tool result text with placeholder `"[Old tool result content cleared]"`. Dedup-aware, whitelist-only for compactable tools, size-threshold-based (>= 2000 chars) | **Two paths**: (1) **Cached microcompact** — uses server-side `cache_edits` API to delete tool results without invalidating cache prefix. (2) **Time-based microcompact** — when gap since last assistant exceeds threshold, content-clears old tool results directly |
| Keep window: `keepRecent` most recent tool results preserved | Cached MC: count-based `triggerThreshold` (configurable via GrowthBook), keeps `keepRecent` most recent |
| Tool name lookup via two-pass scan | Tool name lookup via `collectCompactableToolIds()` — single pass over assistant messages |
| Go has no time-based trigger | Upstream time-based: `evaluateTimeBasedTrigger()` checks gap vs `gapThresholdMinutes` (configurable) |
| Go mutates message content directly | Cached MC does **NOT** mutate messages — adds `cache_reference` and `cache_edits` at API layer. Time-based MC mutates content directly (cache is already cold) |

### 7. KeepRecentMessagesAdaptive

| Go | Upstream |
|---|---|
| `KeepRecentMessagesAdaptive(minTokens, minTextMsgs, maxTokens)` — walks backwards from boundary, accumulates tokens until min constraints met, stops at max cap | `calculateMessagesToKeepIndex()` in `sessionMemoryCompact.ts` — starts from `lastSummarizedMessageId`, expands backwards to meet `minTokens`, `minTextBlockMessages`, stops at `maxTokens` |
| Skips entries marked `summarized` (incremental compaction) | Uses `lastSummarizedMessageId` from session memory module — same concept as Go's `summarized` field |
| `adjustForToolPairing()` ensures tool_use/tool_result pairing | `adjustIndexToPreserveAPIInvariants()` — same purpose, more sophisticated (also handles thinking blocks sharing `message.id`) |
| Updates `lastSummarizedIndex` and `compactedEntryCount` for incremental tracking | Uses `setLastSummarizedMessageId()` and `getLastSummarizedMessageId()` in `sessionMemoryUtils.js` |
| Config: default `minTokens=1000`, `minTextMsgs=4`, `maxTokens=10000` | Default config: `minTokens=10000`, `minTextBlockMessages=5`, `maxTokens=40000` (remotely configurable via GrowthBook `tengu_sm_compact_config`) |
| **Very similar logic** — Go's implementation closely mirrors upstream's `calculateMessagesToKeepIndex` | Same algorithm with configurable thresholds |

### 8. ConversationEntry.summarized Field

| Go | Upstream |
|---|---|
| `conversationEntry.summarized bool` — marks entries already included in a previous compaction summary. `KeepRecentMessagesAdaptive` skips summarized entries | `lastSummarizedMessageId: UUID | undefined` — tracks the UUID of the last message included in a compaction summary. Stored in `sessionMemoryUtils.js` module-level state. |
| `lastSummarizedIndex int` — index boundary for incremental compaction | No index — uses UUID lookup: `messages.findIndex(msg => msg.uuid === lastSummarizedMessageId)` |
| `compactedEntryCount int` — count of entries before boundary that were already summarized | Not explicitly tracked — derived from `messagesToKeep.length` |
| Go marks kept entries as `summarized = true` after each `KeepRecentMessagesAdaptive` call | Upstream sets `setLastSummarizedMessageId(keep[keep.length-1].uuid)` after compaction |
| `summarized` is per-entry in-memory state | `lastSummarizedMessageId` is module-level global state (persisted across turns) |

### Context Management — Summary Divergence Table

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

---


---

## 8. context.go — 上下文管理（ConversationContext + ToolStateTracker）

### 8.1 架构与数据模型

| Go | Upstream |
|---|---|
| `ConversationContext` + `ToolStateTracker` 独立结构体，使用 `sync.RWMutex` 保护（`context.go:289`, `context.go:104`） | Upstream 使用 React `useState` + `appState` 全局状态管理，通过 `useReplBridge` 和 `AppStateStore` 协调 |
| `EntryContent` 为密封接口，5种子类型：`TextContent`, `ToolUseContent`, `ToolResultContent`, `CompactBoundaryContent`, `SummaryContent`, `AttachmentContent`（`context.go:29-85`） | Upstream 使用 `Message` union type（`types/message.ts`），通过 `type` 字段区分：`user`, `assistant`, `compact_boundary`, `attachment` 等 |

### 8.2 Compaction（上下文压缩）

#### 8.2.1 上游 Compact 架构
- **上游**：`src/services/compact/compact.ts` — 主 compact 流程，调用 `runForkedAgent()` 生成摘要
- **上游**：`src/services/compact/sessionMemoryCompact.ts` — Session Memory compact（SM-compact），基于 `lastSummarizedMessageId` 增量压缩
- **上游**：`src/services/compact/autoCompact.ts` — 自动压缩调度

| Go | Upstream |
|---|---|
| 4阶段退化链：`Compact` → `SmartCompact` → `SelectiveCompact` → `Hard truncate`（`context.go:666-716`） | 上游使用 Session Memory 作为持久化摘要（`sessionMemoryCompact.ts`），辅以 forked agent 生成摘要（`compact.ts`） |
| `CompactContext()` 在 `entriesToCompactionMessages()` 中序列化，调用 `Compact()` 后反序列化回 `conversationEntry` | 上游直接在 `Message[]` 上操作，通过 `buildPostCompactMessages()` 构建后压缩消息，无序列化/反序列化往返 |
| 使用 `lastSummarizedIndex` 和 `compactedEntryCount` 跟踪增量压缩进度（`context.go:1075`） | 上游使用 `lastSummarizedMessageId`（UUID 精确匹配）跟踪已摘要消息 |
| `entriesToCompactionMessages()` 在 `CompactBoundaryContent` 处丢弃所有之前的消息（`context.go:779`） | 上游使用 `getMessagesAfterCompactBoundary()` 跳过边界前的消息（`utils/messages.ts`） |
| **Go 缺失：无 Session Memory 持久化摘要** | 上游 `trySessionMemoryCompaction()` 读写 `.claude/SESSION_MEMORY.md`，跨会话复用上下文记忆 |
| **Go 缺失：无 forked agent 生成摘要** | 上游通过 `runForkedAgent()` 调用 LLM 生成摘要（`compact.ts`），Go 版本无此机制 |

#### 8.2.2 边界标记与 PreservedSegment

| Go | Upstream |
|---|---|
| `CompactBoundaryContent` 含 `UUID`, `PreservedSegment`（`context.go:49-72`），用于链重链接 | 上游 `createCompactBoundaryMessage()` + `annotateBoundaryWithPreservedSegment()` 功能相同（`compact.ts`） |
| `KeepRecentMessagesAdaptive()` 按 token 预算（minTokens/minTextMsgs/maxTokens）动态计算保留范围（`context.go:1040-1145`） | 上游 `calculateMessagesToKeepIndex()` + `adjustIndexToPreserveAPIInvariants()` 按 token 预算计算（`sessionMemoryCompact.ts:324-397`） |
| `adjustForToolPairing()` 向前搜索补全 `tool_use/tool_result` 对（`context.go:1196-1269`） | 上游 `adjustIndexToPreserveAPIInvariants()` 做相同调整，还处理 streaming thinking block（`sessionMemoryCompact.ts:232-314`） |

### 8.3 ToolStateTracker（工具状态跟踪）

| Go | Upstream |
|---|---|
| `ToolStateTracker` 使用 `epoch` 计数器区分 fresh/stale 项（`context.go:104-120`） | 上游使用 `FileReadStateCache`（`fileStateCache.ts`），跟踪已读文件 + 搜索模式 |
| `OnCompaction()` 递增 epoch，使之前所有项变为 stale（`context.go:181-185`） | 上游通过 `cloneFileStateCache()` 在 fork 时克隆缓存，隔离子代理状态（`forkedAgent.ts:383`） |
| `BuildSessionStateNote()` 生成纯文本摘要注入 system prompt（`context.go:220-279`） | 上游将状态作为独立 system context 字段，不拼接为文本 |
| `FileUnmodified()` 通过 mtime 检查文件是否被修改（`context.go:145-157`） | 上游使用 `FileReadState` 跟踪 `contentHash` + `stat` 比较 |
| **Go 缺失：无 `discoveredSkillNames` 跟踪** | 上游子代理上下文跟踪 `discoveredSkillNames`（`forkedAgent.ts:390`），用于 `was_discovered` 遥测 |
| **Go 缺失：无 `contentReplacementState`** | 上游使用 `ContentReplacementState` 确保 tool result replacement 的 UUID 一致性（`toolResultStorage.ts`） |

### 8.4 MicroCompact

| Go | Upstream |
|---|---|
| `MicroCompactEntries()` 清除旧 tool result 内容（`context.go:1285-1409`），有白名单/去重/大小阈值 | 上游 `microCompact.ts` 有 `estimateMessageTokens()`，使用类似策略清除旧 tool results |
| 清除时保留 `ToolUseID`（`context.go:1389-1395`）| 上游使用 `contentReplacementState` 跟踪被替换的工具结果，确保 prompt cache 命中 |
| **Go 使用硬编码 `compactableToolNames`** | 上游通过 `getDynamicConfig` 动态配置可清除工具列表 |

### 8.5 Truncate 与 ValidateToolPairing

| Go | Upstream |
|---|---|
| `ValidateToolPairing()` 处理 orphaned tool_results 和 orphaned tool_uses（`context.go:1470-1567`） | 上游 `normalizeMessagesForAPI()` + `ensureToolResultPairing()` 做相同工作 |
| 插入合成 error result 处理缺失的 tool_result（`context.go:1544-1557`） | 上游在 `claude.ts` 中 `ensureToolResultPairing` 做相同处理 |
| `FixRoleAlternation()` 合并连续同角色消息（`context.go:1573-1643`） | 上游 `normalizeMessagesForAPI()` 处理相同问题 |
| `truncateWithBoundary()` 在 truncation 时保留 compaction boundary（`context.go:626-655`） | 上游通过 `getMessagesAfterCompactBoundary()` 做类似处理 |

---


---

## 13. context_references.go — @context 展开

### 13.1 架构对比

| Go | Upstream |
|---|---|
| **文本预处理模式**：`PreprocessContextReferences()` 在发送前解析 `@` 引用，将内容注入消息（`context_references.go:150-233`） | **UI附件模式**：上游通过 `extractAtMentionedFiles()` 提取 `@` 文件名，通过 `processAtMentionedFiles()` 生成 `Attachment[]`，作为独立附件附加（`utils/attachments.ts:1911-1981`） |
| 正则解析：`@file:path`, `@folder:path`, `@diff`, `@staged`, `@git:N`, `@url:url`（`context_references.go:63`） | 上游正则仅提取简单 `@filename`（`utils/attachments.ts:2790-2812`），不支持 `@file:`/`@folder:` 前缀语法 |
| 解析结果直接拼接为 Markdown 文本块注入消息 | 解析结果作为结构化 `Attachment` 对象（`{type: 'file', path, content}`） |

### 13.2 支持的引用类型

| 引用类型 | Go | Upstream |
|---|---|---|
| `@file:path` | 支持，读取文件内容（`context_references.go:266-375`） | 支持，通过 `generateFileAttachment()`（`utils/attachments.ts`） |
| `@file:path:10-50`（行范围） | 支持，带行号显示（`context_references.go:327-367`） | 支持，`parseAtMentionedFileLines()` 解析行范围（`utils/attachments.ts`） |
| `@folder:path` | 支持，递归目录树（`context_references.go:378-409`） | 支持，`readdir()` 列出目录（`utils/attachments.ts:1934-1958`） |
| `@diff`（未暂存变更） | 支持，调用 `git diff`（`context_references.go:412-460`） | **上游无文本 `@diff` 支持** — diff 通过 UI 工具调用 |
| `@staged`（已暂存变更） | 支持，调用 `git diff --staged`（`context_references.go:244-245`） | **上游无文本 `@staged` 支持** |
| `@git:N`（最近 N 次提交） | 支持，调用 `git log -N -p`（`context_references.go:246-257`） | **上游无文本 `@git:N` 支持** |
| `@url:url` | 支持，HTTP 请求获取网页内容（`context_references.go:463-518`） | **上游无 `@url` 支持** |
| `@"quoted path"` | 支持（`context_references.go:116`） | 支持（`utils/attachments.ts:2790`） |

### 13.3 安全机制

| Go | Upstream |
|---|---|
| **敏感目录阻止**：`.ssh`, `.aws`, `.gnupg`, `.kube`, `.docker`, `.azure` 等（`context_references.go:66-69`） | **上游使用 `isFileReadDenied()`** 通过权限规则控制（`utils/attachments.ts:1926-1929`） |
| **路径遍历保护**：`ensurePathAllowed()` 检查路径必须在 CWD 内（`context_references.go:531-560`） | 上游通过 `expandPath()` 展开路径，但不强制 CWD 限制 |
| 软/硬 token 预算：50% 硬拒绝，25% 警告（`context_references.go:179-206`） | 上游无 token 预算限制 — 附件通过独立 UI 渲染 |
| **文件缓存**：`fileCache` 全局 map 避免重复读取（`context_references.go:49-56`） | 上游使用 `FileReadStateCache` 做类似缓存 |
| 文件大小限制：10MB 上限（`context_references.go:286-289`） | 上游通过 `FileReadTool` 有文件大小限制 |
| 行数限制：1000 行（`context_references.go:18,298`） | 上游通过 `generateFileAttachment()` 有行数限制 |

### 13.4 HTML 处理

| Go | Upstream |
|---|---|
| `extractHTMLContent()` 使用正则提取网页内容，移除 script/style 块（`context_references.go:789-831`） | **上游使用浏览器抓取**（Playwright/puppeteer）获取页面内容，非简单正则提取 |
| 支持提取 `<article>`, `<main>`, `<body>` 内容 | 上游通过浏览器渲染获取完整 DOM |
| `extractHTMLTitle()` 提取 `<title>`（`context_references.go:834-841`） | 上游通过浏览器获取页面 title |

### 13.5 代码围栏语言检测

| Go | Upstream |
|---|---|
| `codeFenceLanguage()` 映射 20+ 种文件扩展名到 Markdown 语言（`context_references.go:673-704`） | 上游通过 `generateFileAttachment()` 做类似语言检测 |

### 13.6 目录树渲染

| Go | Upstream |
|---|---|
| `buildFolderListing()` 递归目录树，最大深度 3，最多 200 条目（`context_references.go:707-757`） | 上游 `readdir()` 列出目录，最多 1000 条目，无树形缩进（`utils/attachments.ts:1936-1958`） |
| 跳过隐藏文件/目录（`context_references.go:743-745`） | 上游不跳过隐藏文件 |

### 13.7 Go 独有功能

| Go 有 | 上游无 |
|---|---|
| `@diff`/`@staged`/`@git:N` git 引用展开 | 上游通过独立工具/命令访问 git 信息 |
| `@url:url` 网页抓取 | 上游无 URL 展开 |
| Token 预算门控（软/硬限制） | 上游附件无 token 预算 |
| 行范围指定（`@file:path:10-50`）在预处理阶段解析 | 上游行范围在附件生成时处理 |
| 错误注入为上下文块（`context_references.go:166-171`） | 上游通过 UI 显示错误 |

### 13.8 上游独有功能

| 上游有 | Go 缺失 |
|---|---|
| MCP 资源引用（`@server:uri`）| Go 无 MCP 资源引用展开 |
| Agent 引用（`@agent-name`）| Go 无 agent 引用展开 |
| Skill 引用（`/skill` 命令） | Go 通过其他方式处理 |
| VSCode IDE 集成（打开文件通知）| Go 为纯 CLI，无 IDE 集成 |

---


---

## Go Implementation
- `E:\Git\miniClaudeCode-go-github\context_references.go:1-841` — Complete standalone file


---

## Upstream Implementation
- `E:\Git\claude-code-upstream\src\utils\attachments.ts:1-4023` — Main attachment/mention processing
- `E:\Git\claude-code-upstream\src\utils\readFileInRange.ts` — File range reading with size limits
- `E:\Git\claude-code-upstream\src\utils\fileStateCache.ts` — File state caching
- `E:\Git\claude-code-upstream\src\utils\file.ts:544-553` — `isFileWithinReadSizeLimit()`
- `E:\Git\claude-code-upstream\src\utils\messages.ts:3640` — Truncated file message rendering
- `E:\Git\claude-code-upstream\src\components\src\utils\fileHistory.ts` — Stub (auto-generated type)
- `E:\Git\claude-code-upstream\src\types\src\utils\fileHistory.ts` — Stub (auto-generated type)


---

## 1. Supported Reference Types

**Go** (context_references.go:28-29):
```go
type ContextReference struct {
    Kind string // "file", "folder", "diff", "staged", "git", "url"
}
```
Go supports exactly 6 types: `@file`, `@folder`, `@diff`, `@staged`, `@git:N`, `@url`.

**Upstream** (attachments.ts:440-700, Attachment union type):
The upstream does NOT have a structured `@reference` parsing system. Instead, it uses a single catch-all regex for `@` mentions and then classifies the target via stat:
- **File**: `@file.txt`, `@file.txt#L10-20` (attachments.ts:2783-2816, `extractAtMentionedFiles`)
- **Directory**: Same `@` syntax, detected via `stat.isDirectory()` (attachments.ts:1931-1958)
- **MCP Resource**: `@server:uri` (attachments.ts:2818-2826, `extractMcpResourceMentions`)
- **Agent mention**: `@agent-<type>` (attachments.ts:2828-2854, `extractAgentMentions`)
- **No equivalent**: `@diff`, `@staged`, `@git`, `@url` as standalone reference types

**Key difference**: Go has explicit keyword-based reference types (`@diff`, `@staged`, `@git:N`, `@url`). The upstream handles none of these as @-references. Git diff/staged info would be retrieved via the Bash tool or FileReadTool. URL fetching would be done via a web search tool or MCP resource.

**Line references**:
- Go: `@file:path:10-50` (context_references.go:118-123, colon-separated range)
- Upstream: `@file.txt#L10-20` (attachments.ts:2867, GitHub-style hash-L notation)

**Regex comparison**:
- Go (line 63): `@(?:diff|staged\b|(?:file|folder|git|url):(?:[^"]+|\S+))` — structured, requires keyword prefix
- Upstream (line 2790-2791): `(^|\s)@"([^"]+)"` and `(^|\s)@([^\s]+)\b` — free-form, any non-whitespace after @


---

## 2. File Expansion — Reading and Injecting File Content

**Go** (context_references.go:266-375, `expandFileReference`):
- Uses `os.Stat` for existence/size check (line 273)
- Stream-reads with `bufio.Scanner`, 1MB max line buffer (line 626)
- Binary detection via null-byte check in first 16 lines (line 307, `isBinaryLines`)
- Content wrapped in markdown code fences with language detection (line 374)
- Line ranges with 1-indexed `start-end` notation
- Returns formatted block: `## @file:path (N tokens)\n```lang\ncontent\n```
- Token estimation: `len(text) / 4` (line 372)

**Upstream** (attachments.ts:3046-3225, `generateFileAttachment`):
- Calls `FileReadTool.call()` for actual reading (line 3204)
- Uses `isFileWithinReadSizeLimit()` with `getDefaultFileReadingLimits().maxSizeBytes` (line 3073-3077)
- PDF special handling: large PDFs (>threshold pages) become `PDFReferenceAttachment` instead of inline (line 3012-3044, `tryGetPDFReference`)
- Already-read file optimization: checks `fileState` cache + mtime to avoid re-reading (line 3103-3145)
- Truncation fallback: if `MaxFileReadTokenExceededError` or `FileTooLargeError`, reads first `MAX_LINES_TO_READ` lines (line 3175-3179)
- Returns structured `FileAttachment` type with `truncated` flag (line 3184-3190)
- Deny rules: checks `isFileReadDenied()` against tool permission context (line 3067)

**Key differences**:
- Go: Direct file I/O with inline regex-based HTML extraction for URLs
- Upstream: Tool-based (`FileReadTool`) with structured attachment types
- Upstream has PDF-aware handling; Go has no PDF special treatment
- Upstream has "already_read_file" optimization to avoid resending unchanged files; Go caches in-memory but always injects
- Go estimates tokens as `len/4`; upstream uses context window-based budgeting


---

## 3. Folder Expansion — Directory Listing with Content

**Go** (context_references.go:378-409, `expandFolderReference`):
- Uses `os.ReadDir` with depth limit of 3 (constant `MaxFolderDepth`, line 21)
- Recursive tree builder `buildFolderListingRecursive` (line 724-757)
- Entry limit of 200 files (line 405)
- Hidden files skipped (line 743)
- Returns tree format: `path/` then indented `- subdir/` or `- file`
- No content of files shown — names only

**Upstream** (attachments.ts:1931-1958, inline in `processAtMentionedFiles`):
- Single-level `readdir` (no recursion) (line 1936)
- Entry limit of 1000 (line 1939, `MAX_DIR_ENTRIES`)
- Hidden files NOT skipped (no filter)
- Returns flat list: filenames joined with `\n`
- If over limit: adds `… and N more entries` (line 1943)

**Key differences**:
- Go: Recursive tree with depth 3, max 200 entries, hides dotfiles
- Upstream: Flat listing, max 1000 entries, shows all files
- Go has richer formatting (indentation, trailing `/` for dirs)


---

## 4. Diff Expansion — Git Diff Injection

**Go** (context_references.go:243-244, 412-460, `expandGitReference`):
- `@diff` maps to `git diff` (no `--staged` flag)
- Checks if in git repo via `git rev-parse --is-inside-work-tree` (line 414)
- Sets `GIT_TERMINAL_PROMPT=0` to prevent credential prompts (line 416)
- Wraps output in ```diff code fence (line 459)
- Empty diff: `"(working tree is clean -- no unstaged changes)"` (line 443)

**Upstream**:
- **NO equivalent `@diff` reference type**
- Git diff must be obtained via Bash tool (`git diff`)
- However, `fileHistoryGetDiffStats` (fileHistory.ts:414-484) computes line-level diff stats using the `diff` npm package's `diffLines()` (line 705)
- The `useGitDiffStats` comment in fileHistory.ts:50 suggests UI-level diff stat display

**Key difference**: Go has first-class `@diff` keyword. Upstream requires tool invocation.


---

## 5. Staged Expansion — Git Staged Changes

**Go** (context_references.go:244-245):
- `@staged` maps to `git diff --staged`
- Same git check/prompt protection as @diff
- Empty: `"(nothing staged -- no staged changes to commit)"` (line 444)

**Upstream**:
- **NO equivalent `@staged` reference type**
- Would require explicit Bash tool call


---

## 6. Git Log Expansion — Commit History

**Go** (context_references.go:247-257):
- `@git:N` shows last N commits with `-p` (patch/diff included)
- N clamped: minimum 1, maximum 10 (lines 250-255)
- Command: `git log -N -p`
- Empty: `"(no commits found in this repository)"` (line 447)

**Upstream**:
- **NO equivalent `@git` reference type**
- Commit attribution is tracked separately (utils/commitAttribution.ts)
- Git log requires Bash tool


---

## 7. URL Expansion — Web Content Fetching

**Go** (context_references.go:463-518, `expandURLReference`):
- HTTP/HTTPS only (line 466)
- 30-second timeout (line 471)
- Max 10 redirects (line 473)
- Max 500KB response body (`MaxURLSize`, line 24)
- Custom User-Agent: `miniClaudeCode/1.0` (line 484)
- HTML content extraction: removes `<script>`/`<style>`, extracts `<article>`/`<main>`/`<body>`, strips tags, decodes entities (lines 789-841)
- Title extraction for context hint (line 511, `extractHTMLTitle`)

**Upstream**:
- **NO equivalent `@url` reference type**
- Web content via `web_fetch` MCP tool or search tools
- No inline URL fetching in the attachment system

**Key difference**: Go has built-in URL fetching with HTML parsing. Upstream relies on MCP tools.


---

## 8. Token Budget for Expansion

**Go** (context_references.go:178-207):
- **Hard limit**: 50% of context length (line 179)
- **Soft limit**: 25% of context length (line 180)
- If exceeds hard limit: `Blocked: true`, expansion refused entirely
- If exceeds soft limit: warning appended, but expansion still happens
- Token estimation: `len(block) / 4` chars-to-tokens (line 174)
- Returns `ContextReferenceResult` with `Blocked`, `InjectedTokens`, `Warnings` fields

**Upstream** (attachments.ts:3175-3190, plus FileReadTool):
- No aggregate budget for all @-mentions
- Per-file limits via `getDefaultFileReadingLimits().maxSizeBytes` (line 3076)
- `MAX_LINES_TO_READ` for truncation fallback (line 3175)
- Context window awareness in token usage attachments (line 3841-3849)
- No hard/soft limit gates that block entire expansion
- Context window size retrieved via `getContextWindowForModel()` (line 2763, 3969)

**Key difference**: Go has aggregate token budget enforcement (hard block at 50%, soft warning at 25%). Upstream relies on per-file size limits with no aggregate check.


---

## 9. Safety Checks — Path Validation, Size Limits

**Go** (context_references.go:528-560, `ensurePathAllowed`):
- Sensitive directories blocklist: `.ssh`, `.aws`, `.gnupg`, `.kube`, `.docker`, `.azure`, `.config/gh`, `.config/git` (lines 66-69)
- Path traversal protection: path must be within CWD (line 555)
- File size limit: 10MB hard reject (line 286-288)
- Binary file detection (line 307)
- Line limit: 1000 lines default (`MaxLineLimit`, line 18)

**Upstream** (attachments.ts + utilities):
- Permission system: `isFileReadDenied()` checks deny rules from `toolPermissionContext` (line 3067)
- File size limit: `isFileWithinReadSizeLimit()` with `getDefaultFileReadingLimits().maxSizeBytes` (line 3073-3077)
- `pathInAllowedWorkingPath()` in permissions/filesystem (line 134) — CWD restriction
- No sensitive directory blocklist (handled by permission system)
- No explicit binary file detection (handled by FileReadTool)

**Key difference**: Go has an explicit sensitive directories list; upstream uses a configurable permission system. Both enforce CWD restriction and file size limits.


---

## 10. Error Handling — Missing Files, Failed URLs

**Go** (context_references.go:266-518):
- Missing file: `"File not found: path" (hint: " (permission denied)")` (line 279)
- Is directory: `"path is a directory, use @folder: instead"` (line 282)
- Too large: `"file is too large (N bytes, max 10 MB)"` (line 288)
- Binary: `"binary files are not supported"` (line 308)
- Git errors: captured via `CombinedOutput()` with context-specific messages (lines 417-434)
- URL errors: `"Fetch failed: error"` or `"Fetch failed with status 404"` (lines 488-493)
- Errors injected as markdown blocks in context (line 168): `## @ref (error)\nerror message`

**Upstream** (attachments.ts:3191-3224):
- Missing file: returns `null`, logs analytics event `tengu_at_mention_extracting_filename_error` (line 2876, 3192-3193)
- Size exceeded: catches `MaxFileReadTokenExceededError` / `FileTooLargeError`, falls back to truncated read (line 3213-3217)
- Error handling is silent (returns `null`) with analytics logging
- No inline error messages injected into context

**Key difference**: Go injects error messages directly into the context so the model understands what went wrong. Upstream silently returns `null` with analytics tracking — the model never sees the error reason.

---

# Part N+2: File History — Go vs Upstream


---

## Summary Table: Context References

| Feature | Go | Upstream |
|---------|----|----------|
| `@file` | Yes, with line ranges | Yes, with #L notation |
| `@folder` | Yes, recursive tree | Yes, flat listing |
| `@diff` | Yes, `git diff` | No |
| `@staged` | Yes, `git diff --staged` | No |
| `@git:N` | Yes, `git log -N -p` | No |
| `@url` | Yes, with HTML parsing | No |
| `@agent` | No | Yes |
| `@mcp:resource` | No | Yes |
| Token budget gates | Hard 50%, soft 25% | Per-file size limits only |
| Sensitive dirs blocklist | Yes (8 dirs) | No (permission system) |
| Error injection | Inline in context | Silent (analytics only) |
| PDF handling | No | Yes (reference attachment) |


---

## 34. Context.go Deep Dive — Entry System & API Message Building

### Files Compared
- **Go**: `E:\Git\miniClaudeCode-go-github\context.go` (~1643 lines)
- **Upstream**: `E:\Git\claude-code-upstream\src\` messages.ts, types/message.ts, tokens.ts, context.ts, microCompact.ts

### 34.1 Entry/Content Type System

| # | Aspect | Go (`context.go`) | Upstream (`types/message.ts`) | Type |
|---|--------|-------------------|-------------------------------|------|
| 1 | Message metadata | `conversationEntry` has `role`, `content`, `summarized` (line 282-286). No UUID, timestamp, parentUuid, isMeta, isCompactSummary | `SystemCompactBoundaryMessage` has UUID, timestamp, compactMetadata with preCompactTokens, preservedSegment (headUuid, anchorUuid, tailUuid), userFeedback, messagesSummarized | 简化 |
| 2 | Compact boundary metadata | `CompactBoundaryContent` (line 49-73) has UUID, PreCompactTokens, PreCompactDiscoveredTools, PreservedSegment — but no `userFeedback` or `messagesSummarized` | `compactMetadata` carries `userFeedback` for human feedback preservation and `messagesSummarized` count | 简化 |
| 3 | Rich message types | 5 variants: TextContent, ToolUseContent, ToolResultContent, CompactBoundaryContent, SummaryContent, AttachmentContent | 8+ types: UserMessage, AssistantMessage, SystemMessage, AttachmentMessage, HookResultMessage, SystemCompactBoundaryMessage, SystemMicrocompactBoundaryMessage, ProgressMessage | 简化 |

### 34.2 Token Estimation Algorithm

| # | Aspect | Go (`context.go`) | Upstream (`tokens.ts`) | Type |
|---|--------|-------------------|------------------------|------|
| 1 | Base estimation | `EstimateTokens()`: `len(text) / 4` (line 37-42). Upstream uses `/ 3.5` — Go is ~14% less conservative | `roughTokenCountEstimation(text)`: `text.length / 3.5` with 4/3 padding | 简化 |
| 2 | Content-type-aware | `EstimateContentTokens()`: code=3.5, json=3.0, tool_use=3.0+10, tool_result=3.0+5, default=4.0. Uses `DetectContentType()` heuristics (`func `, `var `, `class `) | No content-type differentiation. Uses uniform 3.5 ratio. Relies on TypeScript block types, not content inspection | Go适配 |
| 3 | Image/document tokens | No explicit image/document estimation. Images stripped before compaction (line 2613) | `IMAGE_MAX_TOKEN_SIZE = 2000` for any image/document block (`tokens.ts:460-471`) | 简化 |
| 4 | 4/3 padding | `math.Ceil(rawTotal * 4.0 / 3.0)` (line 368) — matches upstream | `Math.ceil(totalTokens * (4/3))` (`microCompact.ts:164-204`) | 匹配 |

### 34.3 BuildMessages() — Entries to API Message Params

| # | Aspect | Go (`context.go:473-546`) | Upstream (`messages.ts`) | Type |
|---|--------|--------------------------|--------------------------|------|
| 1 | Thinking/redacted_thinking | NOT handled — no thinking block or redacted_thinking support | Handled — merges streaming-chunk messages by `message.id`, preserves thinking blocks | 缺失 |
| 2 | Image/document blocks | NOT handled — stripped before compaction | Handled — strips errored images/documents, replaces with text | 简化 |
| 3 | Attachment/hook_result/progress messages | NOT handled | Handled — attachment messages, hook_result messages, progress messages | 缺失 |
| 4 | Tool pairing validation | `ValidateToolPairing()` (line 1470-1567): 3 passes — collect IDs, remove orphans, insert synthetic results | `ensureToolPairing()` in `normalizeMessagesForAPI()`: ~150 lines, handles thinking blocks sharing message.id | 简化 |
| 5 | Same-role merging | `FixRoleAlternation()` (line 1573-1643): type-aware merging (TextContent+TextContent, SummaryContent+TextContent, etc.) | `normalizeMessagesForAPI()` enforces user/assistant alternation, merges consecutive same-role messages | 匹配 |

### 34.4 CompactContext() — Multi-Phase Degradation Chain

| # | Aspect | Go (`context.go:666-716`) | Upstream (`compact.ts`) | Type |
|---|--------|--------------------------|--------------------------|------|
| 1 | Degradation phases | 4 phases: Compact (round-based) → SmartCompact (turn-based) → SelectiveCompact (clear readable) → AggressiveTruncateHistory | LLM-driven compaction via forked agent or streaming fallback with PTL retry loop | Go适配 |
| 2 | Hooks | No pre/post compact hooks in context.go | `executePreCompactHooks()`, `executePostCompactHooks()`, `processSessionStartHooks()` | 缺失 |
| 3 | Skill/plan attachment | No skill attachment re-injection, no plan mode attachment | `createSkillAttachmentIfNeeded()` (25K budget), `createPlanAttachmentIfNeeded()` | 缺失 |
| 4 | Delta re-announcement | No delta tools/agents/MCP re-declaration | `getDeferredToolsDeltaAttachment()`, `getAgentListingDeltaAttachment()`, `getMcpInstructionsDeltaAttachment()` | 缺失 |

### 34.5 Microcompact Entry Clearing

| # | Aspect | Go (`context.go:1285-1409`) | Upstream (`microCompact.ts`) | Type |
|---|--------|--------------------------|--------------------------|------|
| 1 | Time-based microcompact | `MicroCompactEntries()`: gap-based clearing with dedup, whitelist, size-threshold (minCharCount=2000) | `maybeTimeBasedMicrocompact()`: gap > 60min threshold (from GrowthBook `tengu_slate_heron`), content-clears with `TIME_BASED_MC_CLEARED_MESSAGE` | Go适配 (Go more sophisticated on dedup/whitelist) |
| 2 | GrowthBook remote config | Hardcoded defaults | `gapThresholdMinutes` from GrowthBook remote config | 缺失 |
| 3 | Thinking block clearing | No thinking block clearing | `clear_thinking_20251015` with `keep: all` or `keep: {value: 1}` | 缺失 |

### 34.6 TruncateHistory Methods

| # | Aspect | Go (`context.go:576-655`) | Upstream | Type |
|---|--------|--------------------------|----------|------|
| 1 | Truncation levels | 3 levels: TruncateHistory(keep 10), AggressiveTruncateHistory(keep 5), MinimumHistory(keep 2). All use `truncateWithBoundary()` | No direct `truncateHistory` method — handled via reactive compact, context collapse, `truncateHeadForPTLRetry()` | Go适配 (standalone API for CLI fallback) |


---

