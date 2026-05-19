# Transcript

> Transcript recording, playback, resume

## Sections Included
- [##] Line 2668-2683 -- ## 21. Transcript Building (`transcript_builder.go`)
- [##] Line 2736-2751 -- ## 24. Transcript Serialization (`transcript/transcript.go`)
- [##] Line 6184-6554 -- ## 12. Transcript & Resume System
- [##] Line 9704-9750 -- ## 40. Transcript, Sub-Agent, Forked Agent, Tools Interface

---

## Content

## 21. Transcript Building (`transcript_builder.go`)

**Upstream reference**: No direct equivalent; upstream uses `transcriptSearch.ts` for searching transcripts

### 21.1 Compact transcript for classification
- **上游**: No dedicated compact transcript builder; the auto-mode classifier receives raw messages or uses `extractTextContent()` for text extraction
- **Go版**: `BuildCompactTranscript()` builds a compact text representation of the conversation for the auto-mode classifier: user messages (truncated 500 chars), tool names + compact input, tool results (truncated 100 chars) with `isAskUserApproval()` detection. Security: assistant text is explicitly excluded to prevent agent from influencing classifier (transcript_builder.go:14-69)
- **类型**: Go增强 — dedicated compact format for classifier; upstream has no equivalent

### 21.2 AskUserQuestion approval detection
- **上游**: No explicit approval detection in transcript
- **Go版**: `isAskUserApproval()` detects affirmative responses from AskUserQuestion tool results (13 affirmation keywords) and marks them as "USER EXPLICITLY APPROVED" in the compact transcript (transcript_builder.go:150-175)
- **类型**: Go增强 — enables classifier to distinguish approved vs defaulted actions

---


---

## 24. Transcript Serialization (`transcript/transcript.go`)

**Upstream reference**: `src/utils/sessionStorage.ts` (transcript recording) + `src/utils/transcriptSearch.ts` (search)

### 24.1 Transcript format
- **上游**: Transcripts stored as JSONL in `~/.claude/projects/{hash}/sessions/` with rich message types. `recordSidechainTranscript()` for sub-agent transcripts. `transcriptSearch.ts` provides regex search over transcripts
- **Go版**: `Writer`/`Reader` for JSONL transcript files in `.claude/transcripts/`. Entry types: `user`, `assistant`, `tool_use`, `tool_result`, `error`, `system`, `compact`, `summary`. Immediate flush after each write via `f.Sync()` (transcript/transcript.go:15-194)
- **类型**: 简化 — basic JSONL format without rich message types, no transcript search

### 24.2 Crash safety
- **上游**: Transcripts written incrementally during streaming; partial messages may exist
- **Go版**: `writeOne()` opens file in append mode, writes JSON line, calls `f.Sync()`. Reader discards last truncated line (common after Ctrl+C/crash) (transcript/transcript.go:123-140, 167-194)
- **类型**: 等价 — both handle crash safety; Go adds explicit Sync()

---


---

## 12. Transcript & Resume System

**Go files**: `transcript/transcript.go`, `transcript_builder.go`, `agent_loop.go:491-642,644-806`
**Upstream files**: `utils/sessionStorage.ts`, `utils/conversationRecovery.ts`, `utils/sessionStoragePortable.ts`, `services/compact/compact.ts`, `types/logs.ts`, `utils/messages.ts`, `assistant/sessionDiscovery.ts`, `assistant/sessionHistory.ts`, `history.ts`, `utils/listSessionsImpl.ts`

### 12.1 Transcript File Format

**Go** (`transcript/transcript.go:1-195`):
- Simple JSONL format: one `Entry` struct per line
- Each entry is a flat JSON object with these fields:
  ```go
  type Entry struct {
      Type      string         `json:"type"`                 // user, assistant, tool_use, tool_result, error, system, compact, summary
      Content   string         `json:"content,omitempty"`
      ToolName  string         `json:"tool_name,omitempty"`
      ToolArgs  map[string]any `json:"tool_args,omitempty"`
      ToolID    string         `json:"tool_id,omitempty"`
      Timestamp time.Time      `json:"timestamp"`
      Model     string         `json:"model,omitempty"`
      Error     string         `json:"error,omitempty"`
  }
  ```
- File location: `.claude/transcripts/<sessionID>.jsonl` where sessionID = `YYYYMMDD-HHMMSS`
- One file per session, always appended to
- No parentUuid chain, no UUID-per-message, no branching

**Upstream** (`types/logs.ts:1-318`, `utils/sessionStorage.ts:101-106`):
- JSONL format with a rich, discriminated-union `Entry` type: 19+ entry types
- Transcript messages (`TranscriptMessage` = `SerializedMessage & {parentUuid, isSidechain, ...}`) contain:
  - `uuid: UUID` — unique per message
  - `parentUuid: UUID | null` — forms a linked list / tree (DAG) enabling branching
  - `isSidechain: boolean` — marks sub-agent transcripts
  - `sessionId`, `cwd`, `userType`, `entrypoint`, `timestamp`, `version`, `gitBranch`, `slug`
  - `logicalParentUuid` — preserved across compact boundaries for session breaks
  - `agentId`, `teamName`, `agentName`, `agentColor`, `promptId`
- Non-transcript metadata entries (18 types): `summary`, `custom-title`, `ai-title`, `last-prompt`, `task-summary`, `tag`, `agent-name`, `agent-color`, `agent-setting`, `pr-link`, `mode`, `worktree-state`, `content-replacement`, `file-history-snapshot`, `attribution-snapshot`, `speculation-accept`, `marble-origami-commit`, `marble-origami-snapshot`
- File location: `<projectsDir>/<sanitizedProjectPath>/<sessionId>.jsonl` where sessionId is a UUID
- Sub-agent transcripts stored in `<projectsDir>/<sanitizedProjectPath>/<sessionId>/subagents/agent-<agentId>.jsonl`
- Supports multiple leaves (branching conversations); `buildConversationChain` walks parentUuid to find the main chain

**DIFF**: Go uses a flat, linear JSONL with 8 entry types; upstream uses a DAG-based JSONL with 19+ entry types including per-message UUID, parentUuid chains, sidechain support, and extensive metadata entries. Go stores transcripts in `.claude/transcripts/`; upstream stores in a project-scoped directory with UUID filenames.

### 12.2 Entry Types

| Feature | Go | Upstream |
|---------|-----|----------|
| `user` | ✅ `transcript/transcript.go:67-69` | ✅ `types/logs.ts:221-231` — `TranscriptMessage` with `type: 'user'`, full `Message` payload including `permissionMode`, `isMeta`, `isCompactSummary`, tool_result blocks |
| `assistant` | ✅ `transcript/transcript.go:71-73` — `content` string + `model` | ✅ `TranscriptMessage` with `type: 'assistant'`, full `Message` payload including thinking blocks, tool_use blocks, `stop_reason`, `model` |
| `tool_use` | ✅ `transcript/transcript.go:75-78` — separate entry with `tool_id`, `tool_name`, `tool_args` | ✅ Embedded inside assistant message as `tool_use` content blocks (not a separate entry type) |
| `tool_result` | ✅ `transcript/transcript.go:80-82` — separate entry with `tool_id`, `tool_name`, `content` | ✅ Embedded inside user message as `tool_result` content blocks (not a separate entry type) |
| `compact` | ✅ `transcript/transcript.go:97-106` — dedicated `type: "compact"` entry with `trigger` and `pre_compact_tokens` | ✅ Represented as `system` message with `subtype: 'compact_boundary'` (`messages.ts:4617-4630`) — not a separate entry type. Has `compactMetadata` with `trigger`, `preTokens`, `userContext`, `messagesSummarized`, `preCompactDiscoveredTools`, `preservedSegment` |
| `summary` | ✅ `transcript/transcript.go:109-111` — `type: "summary"` with `content` string | ✅ `type: 'summary'` with `leafUuid` and `summary` string (`types/logs.ts:55-59`) |
| `system` | ✅ `transcript/transcript.go:92-94` — `type: "system"` with `content` | ✅ `type: 'system'` messages with `subtype` field, `level`, `uuid` — many subtypes (`compact_boundary`, `microcompact_boundary`, `info`, `api_error`, etc.) |
| `error` | ✅ `transcript/transcript.go:87-89` — `type: "error"` with `error` string | ❌ No dedicated error entry; errors embedded in system messages or tool results |
| `custom-title` | ❌ | ✅ `types/logs.ts:61-65` |
| `ai-title` | ❌ | ✅ `types/logs.ts:75-79` |
| `last-prompt` | ❌ | ✅ `types/logs.ts:81-84` |
| `task-summary` | ❌ | ✅ `types/logs.ts:93-98` — periodic fork-generated summary for `claude ps` |
| `tag` | ❌ | ✅ `types/logs.ts:100-104` |
| `agent-name/color/setting` | ❌ | ✅ `types/logs.ts:106-122` |
| `pr-link` | ❌ | ✅ `types/logs.ts:127-135` |
| `mode` | ❌ | ✅ `types/logs.ts:137-141` — `coordinator`/`normal` |
| `worktree-state` | ❌ | ✅ `types/logs.ts:167-171` |
| `content-replacement` | ❌ | ✅ `types/logs.ts:181-186` — for prompt cache stability on resume |
| `file-history-snapshot` | ❌ | ✅ `types/logs.ts:188-193` |
| `attribution-snapshot` | ❌ | ✅ `types/logs.ts:208-219` — Claude character contribution tracking |
| `marble-origami-commit` | ❌ | ✅ `types/logs.ts:255-269` — context collapse commit entries |
| `marble-origami-snapshot` | ❌ | ✅ `types/logs.ts:282-295` — staged queue + spawn state |
| `speculation-accept` | ❌ | ✅ `types/logs.ts:233-237` |
| `progress` | ❌ (removed from current Entry type) | Legacy entries bridged in `sessionStorage.ts:3624-3641` |

**DIFF**: Go has 8 flat entry types; upstream has 19+ entry types. Crucially, Go stores `tool_use` and `tool_result` as separate entries while upstream embeds them as content blocks within `assistant` and `user` messages respectively. Go's `compact` is a dedicated type; upstream uses `system` with `subtype: 'compact_boundary'`. Upstream has extensive metadata entries (titles, tags, PR links, worktree state, content replacements, attribution, etc.) that Go lacks entirely.

### 12.3 Transcript Writing

**Go** (`transcript/transcript.go:56-151`):
- `Writer.Write(entry)` — acquires mutex, sets timestamp, opens file in `O_APPEND|O_CREATE|O_WRONLY`, marshals JSON, writes line + `\n`, calls `f.Sync()` (fsync for crash safety)
- Immediate flush on every write: opens and closes the file per write (`writeOne`, line 123-140)
- `Flush()` is a no-op (already immediate)
- Writing points in agent_loop:
  - `NewAgentLoop`: writes `system` entry with model/mode config (`agent_loop.go:382`)
  - `Run()`: writes `user` entry on input (`agent_loop.go:919`)
  - After streaming: writes `assistant` entry (`agent_loop.go:1130`)
  - During tool execution: writes `tool_use` (`agent_loop.go:2239`) and `tool_result` (`agent_loop.go:2428`)
  - After compaction: writes `compact` + `summary` entries (`agent_loop.go:3892-3894`, `4074-4076`, `4182-4184`)

**Upstream** (`utils/sessionStorage.ts:993-1280`):
- `ProjectStore.insertMessageChain(messages)` — assigns parentUuid chain, materializes session file on first user/assistant, writes each message via `appendEntry`
- `appendEntry(entry)` — buffers until session file materialized, then enqueues async write via `enqueueWrite`
- `appendEntryToFile(path, entry)` — appends JSONL line with file locking (`sessionStorage.ts:2573`)
- Writes are asynchronous with a write queue and batching
- Compact boundary gets `parentUuid: null` (chain break), with `logicalParentUuid` preserving the chain (`sessionStorage.ts:1040-1041`)
- Metadata entries written at various points: summary on compaction, custom-title on `/rename`, tag on `/tag`, agent-setting on resume, etc.
- Dedup: `shouldSkipPersistence()` checks UUID set to avoid writing messages already on disk

**DIFF**: Go does synchronous, per-write fsync (crash-safe but slower); upstream uses async write queue with file locking. Go opens/closes file per write; upstream keeps a write queue. Go has no parentUuid chain; upstream assigns UUIDs and builds a parentUuid DAG. Go has no dedup mechanism; upstream dedup-skips already-persisted UUIDs. Upstream writes `parentUuid: null` on compact boundaries (chain break) with `logicalParentUuid` preserving chain; Go has no equivalent.

### 12.4 Transcript Reading / Replay Logic

**Go** (`transcript/transcript.go:158-195`, `agent_loop.go:644-746`):
- `Reader.ReadAll()` — reads entire file line-by-line, JSON unmarshals each line into `Entry`, discards corrupt last line (truncated writes)
- Scanner buffer: 1MB max line size (`scanner.Buffer(make([]byte, 1024*1024), 1024*1024)`)
- `rebuildContextFromTranscript(entries, cfg)`:
  1. Iterates entries sequentially
  2. Groups consecutive `tool_use` entries into one assistant message
  3. Groups consecutive `tool_result` entries into one user message
  4. On `compact` entry: flushes pending tool state, adds compact boundary to context
  5. On `summary` entry: adds summary as user-role message
  6. On `system`/`error` entries: skips them
  7. Final validation: `ValidateToolPairing()` and `FixRoleAlternation()` fix orphaned tools and consecutive same-role messages
- No concept of compact-boundary-aware skipping — reads ALL entries, relying on compact/summary entries to correctly partition the history
- No pre-boundary truncation optimization for large files

**Upstream** (`utils/sessionStorage.ts:3473-3800`, `utils/conversationRecovery.ts:154-252`):
- `loadTranscriptFile(filePath)`:
  1. For files > 5MB (`SKIP_PRECOMPACT_THRESHOLD`), uses `readTranscriptForLoad()` which does a forward chunked read that:
     - Skips attribution-snapshot lines at the fd level
     - Truncates at compact boundaries (discards pre-boundary data)
     - Recovers pre-boundary metadata via `scanPreBoundaryMetadata()`
  2. For smaller files, reads entire file
  3. Optionally runs `walkChainBeforeParse()` to skip dead fork branches before JSONL parse
  4. Parses JSONL into `Entry` array, categorizing by type into maps
  5. Bridges legacy progress entries
  6. `buildConversationChain(messages, leafMessage)` walks parentUuid from newest leaf to root, recovers orphaned parallel tool results
  7. Computes leaf UUIDs (branch endpoints)
- `deserializeMessagesWithInterruptDetection()`:
  1. Migrates legacy attachment types
  2. Strips invalid `permissionMode` values
  3. `filterUnresolvedToolUses()` — removes assistant messages with unmatched tool_uses
  4. `filterOrphanedThinkingOnlyMessages()` — removes thinking-only assistant messages
  5. `filterWhitespaceOnlyAssistantMessages()` — removes whitespace-only assistants
  6. `detectTurnInterruption()` — 3-way detection: `none`, `interrupted_prompt`, `interrupted_turn`
  7. Transforms `interrupted_turn` → `interrupted_prompt` by appending synthetic "Continue from where you left off."
  8. Appends synthetic assistant sentinel after last user message for API validity
- `loadConversationForResume(source, sourceJsonlFile)`:
  1. Resolves source: `undefined` (most recent), `string` (session ID or .jsonl path), `LogOption` (already loaded)
  2. Loads messages via `loadMessageLogs()` / `loadMessagesFromJsonlPath()`
  3. Calls `restoreSkillStateFromMessages()` before deserialization
  4. Deserializes with interrupt detection
  5. Processes session start hooks
  6. Returns messages + metadata (file history, attribution, content replacements, session metadata)

**DIFF**: Go reads entire file linearly and replays sequentially; upstream uses a DAG-based chain walk from leaf to root with compact-boundary-aware skipping for large files. Go has no interrupt detection; upstream detects `interrupted_turn` and `interrupted_prompt`, auto-injecting "Continue from where you left off." Go has no pre-boundary optimization; upstream truncates at compact boundaries for files > 5MB. Go has no filtering of unresolved tool uses or orphaned thinking messages; upstream filters all of these. Go has no concept of "turn interruption" state.

### 12.5 Resume Flow — Context Reconstruction

**Go** (`agent_loop.go:491-642`):
1. `NewAgentLoopFromTranscript(cfg, registry, useStream, transcriptPath, continueTranscript)`:
   - Reads all entries via `transcript.NewReader(transcriptPath).ReadAll()`
   - Calls `rebuildContextFromTranscript(entries, cfg)` to construct `ConversationContext`
   - If `continueTranscript`: creates `Writer` from existing file path (appends)
   - If NOT `continueTranscript`: creates new session ID + new transcript file, writes system + "Resumed from..." entries
   - Restores skill state via `restoreSkillStateFromEntries(skillTracker, entries)`
   - Initializes `prevTurnTokens` from rebuilt context to prevent false reactive compact
   - Returns fully constructed AgentLoop

**Upstream** (`utils/conversationRecovery.ts:459-600`, `main.tsx:4485-4515`):
1. `loadConversationForResume(source, sourceJsonlFile)`:
   - Resolves source to get `LogOption` or raw messages
   - If lite log, loads full messages
   - Copies plan for resume, copies file history
   - Restores skill state from `invoked_skills` attachments in messages
   - Deserializes with interrupt detection
   - Processes session start hooks
   - Returns rich object: messages + turnInterruptionState + fileHistorySnapshots + attributionSnapshots + contentReplacements + contextCollapseCommits + contextCollapseSnapshot + sessionId + agent metadata (name, color, setting, customTitle, tag, mode, worktreeSession, prNumber, prUrl, prRepository, fullPath)
2. `processResumedConversation(result, options, resumeContext)` in `main.tsx`:
   - Switches session ID to resumed session
   - Re-uses session file path (continues writing to same JSONL)
   - Restores agent definition, coordinator mode, worktree state
   - Restores attribution state from snapshots
   - Re-applies content replacements for prompt cache stability
   - Restores context collapse state from commits + snapshot
   - Re-appends session metadata (title, tag, agent setting, mode, PR link, etc.)
3. **Session ID reuse**: On resume, upstream reuses the original session's UUID, continuing to append to the same `.jsonl` file

**DIFF**: Go's `continueTranscript` flag controls whether to append to the original file or start a new one; upstream always continues the same session file (reuses session UUID). Go returns a flat `ConversationContext`; upstream returns a rich object with 15+ metadata fields. Upstream restores file history, attribution state, content replacements, context collapse state, and agent metadata on resume; Go only restores skill state. Upstream detects turn interruption and auto-continues; Go has no such detection.

### 12.6 Compact Boundary Handling During Resume

**Go** (`agent_loop.go:704-725`):
- `rebuildContextFromTranscript` handles `compact` entries:
  - Flushes pending tool uses and tool results (orphaned by compaction)
  - Extracts `pre_compact_tokens` from `ToolArgs`
  - Calls `ctx.AddCompactBoundary(CompactTriggerAuto, preTokens)`
  - `summary` entries are added as user-role messages via `ctx.AddSummary(entry.Content)`
- All entries (including pre-compact) are replayed, but compact boundary truncates the in-memory message list
- No optimization for skipping pre-boundary entries during file read

**Upstream** (`utils/sessionStorage.ts:3473-3800`, `utils/sessionStoragePortable.ts:717-800`):
- Compact boundary is `system` + `subtype: 'compact_boundary'`
- For files > 5MB: `readTranscriptForLoad()` does a single forward chunked read:
  - Finds the last compact boundary via byte-level scan for `"compact_boundary"`
  - Truncates all pre-boundary bytes, keeping only post-boundary data
  - Recovers session-scoped metadata from pre-boundary region via `scanPreBoundaryMetadata()`
  - Preserves `preservedSegment` messages that compact chose to keep
- In `loadTranscriptFile()`, compact boundary:
  - Clears `contextCollapseCommits` and `contextCollapseSnapshot` (pre-boundary commits reference missing messages)
  - Sets `parentUuid: null` on boundary messages (chain break)
  - `logicalParentUuid` preserves the actual parent for session break detection
- `buildConversationChain` walks from leaf; compact boundary with `parentUuid: null` becomes the chain root, so pre-compact messages are never traversed

**DIFF**: Go replays all entries including pre-compact ones, relying on compact/summary entries to partition history in-memory; upstream physically truncates pre-boundary data at read time for large files. Go has no pre-boundary skip optimization; upstream has a sophisticated chunked-read + metadata-recovery pipeline. Upstream has `preservedSegment` support for keeping selected messages across compact boundaries; Go has no equivalent. Upstream's compact boundary breaks the parentUuid chain; Go has no chain to break.

### 12.7 Session Discovery — Finding Sessions for Resume

**Go** (`main.go:515-599`):
- `findTranscript(target)` resolves:
  - `"last"` → most recent `.jsonl` file in `.claude/transcripts/`
  - Number (1-indexed) → nth most recent transcript
  - Exact filename or filename + `.jsonl` extension
- `loadTranscriptList()` — reads `.claude/transcripts/` directory, filters `.jsonl` files, sorts by modification time (newest first)
- `listTranscripts()` — prints numbered list of available transcripts
- CLI: `--resume <target>` or `/resume <target>` or `/resume last`
- Single-project scope: only transcripts in `.claude/transcripts/` under current directory

**Upstream** (`utils/listSessionsImpl.ts`, `utils/sessionStorage.ts`, `assistant/sessionDiscovery.ts`):
- **Local session discovery** (`listSessionsImpl.ts`):
  - Scans `<projectsDir>/<sanitizedProjectPath>/` for `*.jsonl` files
  - Two-phase: stat-only candidate pass (cheap), then head/tail read for metadata
  - Extracts: sessionId, summary, lastModified, fileSize, customTitle, firstPrompt, gitBranch, cwd, tag, createdAt
  - Filters sidechain sessions, metadata-only sessions
  - Supports pagination (`limit`, `offset`)
  - Cross-project: `loadAllProjectsMessageLogs()` scans all project directories
  - Worktree support: includes sessions from git worktree paths
- **Cloud session discovery** (`assistant/sessionDiscovery.ts`):
  - `discoverAssistantSessions()` — fetches from Anthropic CCR API (`/v1/sessions`)
  - Filters by status: `idle`, `working`, `waiting`
  - Returns `AssistantSession[]` with id, title, status, created_at
- **Session history** (`assistant/sessionHistory.ts`):
  - `fetchLatestEvents(ctx)` — fetches newest page of events via CCR API
  - `fetchOlderEvents(ctx, beforeId)` — paginates to older events
  - Uses OAuth + anthropic-beta headers
  - Returns `HistoryPage` with `events: SDKMessage[]`, `firstId`, `hasMore`
- **Resume picker UI**: Full interactive session picker with search, filtering, session metadata display

**DIFF**: Go has simple directory listing with number/last/filename lookup, single-project scope; upstream has multi-project scanning, head/tail metadata extraction, cloud CCR API integration, pagination, worktree support, and an interactive picker UI. Upstream supports session search and filtering by tag/title/summary; Go has no search. Upstream has cloud session discovery for assistant mode; Go has no cloud integration.

### 12.8 Session History — Navigation Between Sessions

**Go** (`main.go:558-570`, `history.ts`):
- `listTranscripts()` — prints numbered list of `.jsonl` files sorted by mtime
- No persistent history file for prompt inputs
- No cross-session navigation beyond the numbered list
- Resume hint on exit: prints `To resume this session: --resume <id>`

**Upstream** (`history.ts`, `commands/history/history.ts`):
- `history.jsonl` — global prompt history stored in `~/.claude/history.jsonl`
- Each entry: `{display, pastedContents, timestamp, project, sessionId}`
- Supports pasted content (text and images) with hash-based dedup for large pastes
- Paste store: `~/.claude/paste-store/<hash>.txt` for large content
- `getHistory()` — project-scoped, current session first, then other sessions
- `getTimestampedHistory()` — deduped by display text, for Ctrl+R search
- `removeLastFromHistory()` — undo on auto-restore-from-interrupt
- Async flush with lock-based file writes
- Cleanup registration for process exit

**DIFF**: Go has no persistent prompt history; upstream has a full JSONL-based prompt history with paste support, project scoping, session-scoped ordering, Ctrl+R search, and cleanup handling. Go's session listing is just directory contents; upstream's session picker shows summaries, titles, tags, and timestamps.

### 12.9 Transcript Compaction Markers

**Go** (`transcript/transcript.go:97-106`, `agent_loop.go:3890-3894`):
- `WriteCompact(trigger, preCompactTokens)` — writes `type: "compact"` entry
- Content: `"Compacted conversation (trigger: <trigger>, <N> tokens compressed)"`
- `ToolArgs`: `{"pre_compact_tokens": N}`
- Triggers: `"compact_context"` (manual), `"sm-compact"` (sub-message), `"auto"` (reactive)
- `WriteSummary(content)` — writes `type: "summary"` entry with the LLM-generated summary text
- Written as: compact entry immediately followed by summary entry
- On replay, compact entry triggers `ctx.AddCompactBoundary()`, summary entry triggers `ctx.AddSummary()`

**Upstream** (`utils/messages.ts:4609-4630`, `services/compact/compact.ts:602-628`):
- Compact boundary is a `SystemCompactBoundaryMessage`: `{type: 'system', subtype: 'compact_boundary', compactMetadata: {trigger, preTokens, userContext, messagesSummarized, preCompactDiscoveredTools, preservedSegment}}`
- Boundary message gets `parentUuid: null` (chain break) with `logicalParentUuid` preserving the actual parent
- Summary is a `UserMessage` with `isCompactSummary: true`, `isVisibleInTranscriptOnly: true`
- The boundary + summary are written as regular messages in `insertMessageChain`, maintaining the parentUuid chain
- Additional metadata entries: `marble-origami-commit` and `marble-origami-snapshot` for context collapse state
- `ContentReplacementEntry` records content blocks replaced with stubs for prompt cache stability
- `compactMetadata.preservedSegment` identifies messages kept across the boundary
- `compactMetadata.preCompactDiscoveredTools` carries loaded tool state

**DIFF**: Go uses a simple `compact` + `summary` entry pair; upstream uses a `system` message with `compact_boundary` subtype + a `user` message with `isCompactSummary` flag. Upstream's compact metadata is much richer (trigger, preTokens, userContext, messagesSummarized, discovered tools, preserved segment). Upstream writes the boundary as a chain-breaking message; Go writes flat entries. Upstream has context collapse commit/snapshot entries and content replacement entries; Go has none of these.

### 12.10 Error Recovery During Resume — Malformed Transcript Handling

**Go** (`transcript/transcript.go:176-194`, `agent_loop.go:648-746`):
- `Reader.ReadAll()`: if JSON unmarshal fails for a line, it's kept as `lastBadLine` but the last bad line is silently discarded (`_ = lastBadLine`)
- If the file can't be opened, returns the OS error
- `rebuildContextFromTranscript()`:
  - Handles unknown entry types via default case (no-op for `system`/`error`)
  - `ctx.ValidateToolPairing()` — fixes orphaned tool_use without matching tool_result
  - `ctx.FixRoleAlternation()` — fixes consecutive same-role messages (breaks Anthropic API)
- If `NewAgentLoopFromTranscript()` fails, `main.go:131-133` catches and falls back to new session:
  ```go
  fmt.Fprintf(os.Stderr, "[!] Resume failed: %v\n", err)
  fmt.Fprintln(os.Stderr, "[*] Starting a new session instead")
  ```

**Upstream** (`utils/conversationRecovery.ts:154-252`, `utils/sessionStorage.ts:3626-3700`):
- `loadTranscriptFile()`: wraps entire parse in try/catch; returns empty maps on failure
- `parseJSONL()`: skips malformed lines during parsing
- Legacy progress entry bridging: `progressBridge` map resolves parentUuid across legacy progress entries that are no longer in the type union
- `deserializeMessagesWithInterruptDetection()`:
  - `migrateLegacyAttachmentTypes()` — transforms `new_file` → `file`, `new_directory` → `directory`
  - Strips invalid `permissionMode` values from deserialized user messages
  - `filterUnresolvedToolUses()` — removes assistant messages with unmatched tool_uses + any synthetic messages that follow them
  - `filterOrphanedThinkingOnlyMessages()` — removes thinking-only assistant messages that would cause API errors
  - `filterWhitespaceOnlyAssistantMessages()` — removes whitespace-only assistants from cancelled streams
  - Detects turn interruption and auto-continues
- `checkResumeConsistency()` — validates the loaded messages for consistency
- `isTerminalToolResult()` — handles brief-mode sessions where the last message legitimately ends on a tool_result
- Comprehensive error logging via `logError()`
- Process session start hooks on resume (can inject additional context)

**DIFF**: Go has basic malformed-line discarding and post-replay validation (tool pairing + role alternation fixes); upstream has extensive error recovery including legacy format migration, invalid field stripping, unresolved tool use filtering, orphaned thinking message removal, whitespace filtering, interrupt detection, consistency checks, and brief-mode edge case handling. Upstream migrates legacy attachment types; Go has no legacy format handling.

### 12.11 Continue vs New Session — Go's `continueTranscript` vs Upstream's Approach

**Go** (`agent_loop.go:491-548`):
- `continueTranscript` parameter in `NewAgentLoopFromTranscript()`:
  - `true`: `NewWriterFromExisting(transcriptPath)` — appends to original `.jsonl` file, session ID extracted from filename
  - `false`: creates new session ID (`YYYYMMDD-HHMMSS`), creates new `.jsonl` file in `.claude/transcripts/`, writes system entry with model/mode, writes `"Resumed from <path> (<N> messages restored)"` user entry
- Resume always continues: `main.go:139` always passes `continueTranscript=true`
- CLI: `--resume <target>` or `/resume <target>`
- No `--continue` flag for "most recent session"; `--resume last` achieves same effect
- No `--fork-session` option

**Upstream** (`cli/print.ts:472-474,5061-5064`, `main.tsx:4485-4515,5308-5338`):
- `--continue`: loads most recent session, switches session ID to resumed session's UUID, continues writing to same `.jsonl` file. Skips live background/daemon sessions. No `continueTranscript` parameter — always continues.
- `--resume <sessionId|jsonl-path>`: loads specific session by ID or `.jsonl` file path. Switches session ID. If `.jsonl` path, uses `loadMessagesFromJsonlPath()`.
- `--fork-session`: loads session but creates a NEW session ID and transcript file. The forked session starts from the resumed messages but diverges from there.
- `--resume-session-at <uuid>`: resumes at a specific message within the session (time travel)
- Session ID reuse: on `--continue`/`--resume`, `switchSession()` changes the active session ID to the resumed one, so all subsequent writes go to the original file
- `processResumedConversation()` handles the full pipeline:
  - Coordinator mode restoration
  - Agent definition restoration
  - Worktree state restoration
  - Attribution state restoration
  - Context collapse state restoration
  - Content replacement replay
  - Session metadata re-append (title, tag, agent setting, mode, PR link)
- `--rewind-files` option for resume: restores files to their pre-session state

**DIFF**: Go has a simple `continueTranscript` boolean that determines whether to append to original or create new file; upstream always continues the same file on resume (session UUID reuse) and has `--fork-session` for the "new file" case. Upstream has `--resume-session-at` for time-travel resume; Go has no equivalent. Upstream has `--continue` (most recent) as a distinct concept from `--resume <id>`; Go uses `--resume last`. Upstream has `--rewind-files`; Go has no equivalent. Upstream's resume pipeline restores 10+ types of state; Go only restores conversation context + skill state.

### 12.12 Summary of Architectural Differences

| Aspect | Go | Upstream |
|--------|-----|----------|
| **Data model** | Flat linear JSONL entries | DAG-based entries with UUID + parentUuid chain |
| **Entry types** | 8 types (user, assistant, tool_use, tool_result, compact, summary, system, error) | 19+ types including metadata-only entries |
| **Tool representation** | Separate `tool_use`/`tool_result` entries | Embedded as content blocks within user/assistant messages |
| **Branching** | Not supported | Full DAG with leaf detection, chain walking, orphan recovery |
| **Sub-agents** | No separate transcript | Sidechain transcripts with `agentId`, separate `.jsonl` files |
| **File location** | `.claude/transcripts/<timestamp>.jsonl` | `<projectsDir>/<projectPath>/<uuid>.jsonl` + subagents/ |
| **Session ID** | `YYYYMMDD-HHMMSS` timestamp | UUID v4 |
| **Crash safety** | Per-write fsync | Async write queue with file locking |
| **Compact boundary** | Dedicated `compact` entry type | `system` + `subtype: 'compact_boundary'` with rich metadata |
| **Pre-boundary skip** | None — reads entire file | Truncates at boundary for files > 5MB |
| **Interrupt detection** | None | 3-way: none / interrupted_prompt / interrupted_turn |
| **State restoration** | Conversation context + skill state | 15+ fields: context, skills, file history, attribution, content replacements, context collapse, worktree, agent metadata, PR links |
| **Session discovery** | Directory listing of `.jsonl` files | Multi-project scan + cloud CCR API + interactive picker |
| **Prompt history** | None | Global `history.jsonl` with paste store |
| **Fork/branch** | `continueTranscript=false` creates new file | `--fork-session` with proper chain fork |
| **Time travel** | Not supported | `--resume-session-at <uuid>` |
| **Legacy format** | No migration | Attachment type migration, progress bridging, invalid field stripping |

---

# Part 7: Prompt Caching Economics — Deep Comparison

**Research date: 2026-05-11**


---

## 40. Transcript, Sub-Agent, Forked Agent, Tools Interface

### Files Compared
- **Go**: `transcript/transcript.go`, `agent_sub.go`, `forked_agent.go`, `tools/base.go`
- **Upstream**: `services/sessionTranscript/`, `utils/forkedAgent.ts`, `Tool.ts`

### 40.1 Transcript System

| # | Aspect | Go (`transcript/transcript.go`) | Upstream (`sessionTranscript.ts`) | Type |
|---|--------|-------------------------------|----------------------------------|------|
| 1 | Format | JSONL with `Entry` struct (type, content, tool_name, tool_args, tool_id, timestamp, model, error) | JSONL with segments — similar format | 匹配 |
| 2 | Flush strategy | Immediate disk flush (open+write+sync per entry) for crash safety | Async segment writing | Go适配 |
| 3 | Truncated line handling | `Reader` discards truncated last lines | Similar handling | 匹配 |
| 4 | Entry types | user, assistant, tool_use, tool_result, error, system, compact, summary | Similar types | 匹配 |

### 40.2 Sub-Agent System

| # | Aspect | Go (`agent_sub.go`) | Upstream (`forkedAgent.ts`) | Type |
|---|--------|--------------------|---------------------------|------|
| 1 | Agent types | General, Explore, Plan, Verify, Fork with tool restrictions | `AgentDefinition` with agentType | 匹配 |
| 2 | Disallowed tools | agent, task_output, plan_approval, task_create/update/list/get/stop, send_message | Similar disallowed tools | 匹配 |
| 3 | Fork mode | `ShouldAvoidPermissionPrompts=true`, `MaxOutputTokens=8000`, always auto mode | Same settings | 匹配 |
| 4 | Context filtering | `filterEntriesForFork` removes ToolUseContent, CompactBoundaryContent, AttachmentContent | Same filtering pattern | 匹配 |
| 5 | Spawning | Async with independent cancellation context | Async spawning with background task management | 匹配 |

### 40.3 Forked Agent

| # | Aspect | Go (`forked_agent.go`) | Upstream (`forkedAgent.ts`) | Type |
|---|--------|-----------------------|---------------------------|------|
| 1 | Cache-safe params | `CacheSafeParams`: systemPrompt, model, tools, messages, thinking config | `CacheSafeParams`: systemPrompt, userContext, systemContext, toolUseContext, forkContextMessages | Go适配 |
| 2 | Execution loop | Re-implements query loop with direct registry calls | Delegates to `query()` which handles full loop | 简化 |
| 3 | Skip parent messages | `SkipParentMessages` option for lightweight forks | No equivalent — always includes parent messages | Go增强 |
| 4 | Retry | `retryForkedCall`: 3 retries with exponential backoff | Relies on `query()`'s `withRetry` | 简化 |
| 5 | Output truncation | 50K char limit | Truncated via query loop | 匹配 |

### 40.4 Tool Interface & Registry

| # | Aspect | Go (`tools/base.go:205-211`) | Upstream (`Tool.ts`, `buildTool()`) | Type |
|---|--------|--------------------------|-----------------------------------|------|
| 1 | Tool interface | 5 methods: Name(), Description(), InputSchema(), CheckPermissions(), Execute() + optional ContextTool | ~20+ lifecycle hooks: call, validateInput, isConcurrencySafe, isReadOnly, renderToolUseMessage, renderToolResultMessage, getActivityDescription, getToolUseSummary, etc. | 简化 |
| 2 | Input schema | Returns `map[string]any` (not Zod) | Zod strict input/output schemas | 简化 |
| 3 | Missing hooks | No: renderToolUseMessage, renderToolResultMessage, renderToolUseErrorMessage, extractSearchText, toAutoClassifierInput, preparePermissionMatcher, isSearchOrReadCommand, getPath, backfillObservableInput, getActivityDescription, getToolUseSummary | All present in upstream | 缺失 |
| 4 | Permission result | `PermissionResult` with Behavior enum + ClassifierApprovable, IsBypassImmune, MatchedRule | `{behavior, message, suggestions}` — suggestions for rule-based auto-allow | Go适配 (has unique features, missing suggestions) |
| 5 | Output validation | No output schema validation — input validation via `ValidateParams` (required + enum only) | Zod strict output schemas validated before return | 简化 |

---


---

