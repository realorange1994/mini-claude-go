# TUI & UI

> Terminal UI, rendering, components

## Sections Included
- [##] Line 8908-9089 -- ## Section XXI: TUI Rendering, Input Handling, Notification System, and Conversation Branching (2026-05-12)
- [##] Line 10279-10336 -- ## 47. TUI/UI System, Print/Rendering, Session Management
- [##] Line 11061-11290 -- ## 53. Upstream Component System vs Go Headless CLI

---

## Content

## Section XXI: TUI Rendering, Input Handling, Notification System, and Conversation Branching (2026-05-12)

### 1. Notification / Toast Queue System

### Files Compared
- **Go**: No dedicated notification system — uses `fmt.Fprintf(os.Stderr, ...)` for all status output
- **Upstream**: `src/context/notifications.tsx` (312 lines), `src/components/PromptInput/Notifications.tsx`

### 1.1 Priority Queue Architecture

| Aspect | Go (file:line) | Upstream (file:line) | Type |
|--------|----------------|---------------------|------|
| Notification queue | **None** — `fmt.Fprintf(os.Stderr, ...)` for all output (`context.go:686-715` uses Fprintf for compact messages) | Priority queue with 4 levels: `immediate`, `high`, `medium`, `low` (`notifications.tsx:6`) | 缺失 |
| Current + queue dual slot | **None** | `current: Notification | null` + `queue: Notification[]` dual-slot model (`notifications.tsx:56-60`) | 缺失 |
| Auto-timeout with cleanup | **None** — messages printed to stderr persist in terminal scrollback | `setTimeout` with `DEFAULT_TIMEOUT_MS = 8000` per notification; `currentTimeoutId` tracked for cleanup (`notifications.tsx:41-44`) | 缺失 |
| Immediate priority | **None** | `immediate` priority preempts current notification, clears existing timeout, re-queues displaced current notification (`notifications.tsx:98-151`) | 缺失 |
| Fold / merge | **None** | `fold?: (accumulator, incoming) => Notification` — notifications with same key merge like `Array.reduce()`. Applied to both current and queued (`notifications.tsx:22-24, 157-217`) | 缺失 |
| Invalidation cascades | **None** | `invalidates?: string[]` — new notification can remove queued/displayed notifications by key (`notifications.tsx:15`) | 缺失 |
| Deduplication | **None** | Prevents duplicate keys in queue and current slot (`notifications.tsx:220-224`) | 缺失 |
| React hook integration | **None** | `useNotifications()` returns `addNotification` / `removeNotification` callbacks (`notifications.tsx:46-48`) | 缺失 |
| Text + JSX notifications | **None** | `TextNotification` (string) + `JSXNotification` (React.ReactNode) union type (`notifications.tsx:27-34`) | 缺失 |
| Esc-to-clear hint | **None** — double-press Esc not handled | `addNotification({ key: 'escape-again-to-clear', text: 'Esc again to clear', priority: 'immediate', timeoutMs: 1000 })` (`useTextInput.ts:131-137`) | 缺失 |

**Gap**: Go has no notification/toast system. Every status message is a permanent `fmt.Fprintf` to stderr. Upstream's queue provides transient, priority-ordered, deduplicated, mergeable notifications that auto-dismiss. The `immediate` priority enables UI patterns like "Esc again to clear" that are impossible in Go's permanent-output model.

---

### 2. Input Handling — Line Editing and Keybinding

### Files Compared
- **Go**: `main.go:250-460` (REPL loop with `bufio.NewReader(os.Stdin).ReadString('\n')`)
- **Upstream**: `src/hooks/useTextInput.ts` (530 lines), `src/utils/Cursor.ts` (~400 lines), `src/components/PromptInput/PromptInput.tsx` (~1200 lines)

### 2.1 Core Input Model

| Aspect | Go (file:line) | Upstream (file:line) | Type |
|--------|----------------|---------------------|------|
| Input reading | `bufio.NewReader(os.Stdin).ReadString('\n')` — raw line read with no editing (`main.go:252-254`) | `useTextInput()` hook with `Cursor` class — grapheme-aware cursor with line/offset tracking (`useTextInput.ts:74-530`) | 缺失 |
| Cursor abstraction | **None** — no cursor concept | `Cursor.fromText(value, columns, offset)` with grapheme segmentation, wrapped-line positioning, viewport scrolling (`Cursor.ts:1+`) | 缺失 |
| Line editing | **None** — user must use terminal's native line editing | Full readline: Ctrl+A/E (start/end), Ctrl+B/F (left/right), Ctrl+K (kill-to-end), Ctrl+U (kill-to-start), Ctrl+W (kill-word-back), Ctrl+H (delete-token-before), Meta+B/F (prev/next word), Meta+D (delete-word-after), Ctrl+Y (yank), Meta+Y (yank-pop) (`useTextInput.ts:225-246`) | 缺失 |
| Kill ring | **None** | 10-entry kill ring with `pushToKillRing(text, 'prepend'|'append')`, consecutive kill accumulation, `yankPop()` cycling (`Cursor.ts:15-72`) | 缺失 |
| Double-press detection | **None** — single Ctrl+C immediately cancels input | `useDoublePress()` for Ctrl+C (clear input on 2nd press), Esc (clear input on 2nd press with notification hint), Ctrl+D on empty input (exit on 2nd press) (`useTextInput.ts:109-169`) | 缺失 |
| Multiline input | **None** — single line only | Backslash+Enter for explicit newline (`useTextInput.ts:250-256`), Shift+Enter / Meta+Enter for newline (`useTextInput.ts:258-260`), Apple Terminal modifier detection (`useTextInput.ts:264-266`) | 缺失 |
| Image paste | **None** | `onImagePaste` callback with base64, mediaType, filename, dimensions, sourcePath (`useTextInput.ts:57-63`) | 缺失 |
| Ghost text / typeahead | **None** | `inlineGhostText` with `insertPosition` + `dim` function for inline suggestion rendering (`useTextInput.ts:507-510`) | 缺失 |
| SSH-coalesced Enter handling | **None** | Detects "o\r" coalesced input from slow SSH links; strips trailing \r and submits (`useTextInput.ts:491-500`) | 缺失 |
| DEL character filtering | **None** | Filters `\x7f` DEL chars that interfere with backspace in SSH/tmux (`useTextInput.ts:443-466`) | 缺失 |
| Input mode character | **None** | `isInputModeCharacter(input)` — mode prefix chars (e.g., `!` for bash mode) positioned before cursor (`useTextInput.ts:405-406`) | 缺失 |
| Keybinding system | **None** | `useKeybindings()` / `useKeybinding()` with `KeybindingProviderSetup`, `CommandKeybindingHandlers`, `defaultBindings.ts` (`PromptInput.tsx:67-72`) | 缺失 |
| History search | **None** — only up/down arrow with `useArrowKeyHistory` | `useHistorySearch()` with search box, `useTypeahead()` with typeahead dropdown (`PromptInput.tsx:59,65`) | 缺失 |
| Input buffer for slow terminals | **None** | `useInputBuffer()` — debounces rapid keystrokes on slow terminals (`PromptInput.tsx:61`) | 缺失 |

**Gap**: Go's REPL uses raw `bufio.Reader` with no line editing, no kill ring, no keybinding system, no multiline input, and no image paste. Upstream's `useTextInput` + `Cursor` provides a full terminal editor experience inside the React/Ink TUI, with grapheme-aware cursor movement, readline keybindings, kill/yank ring, double-press guards, and SSH terminal compatibility workarounds.

---

### 3. TUI Rendering — Tool Result and Agent Progress

### Files Compared
- **Go**: `agent_progress.go` (216 lines) — `subAgentProgressWriter`
- **Upstream**: `src/components/AgentProgressLine.tsx` (105 lines), `src/components/Message.tsx` (527 lines), `src/components/messages/UserToolResultMessage/` (4 sub-components)

### 3.1 Agent Progress Display

| Aspect | Go (file:line) | Upstream (file:line) | Type |
|--------|----------------|---------------------|------|
| Agent progress type | `subAgentProgressWriter` struct with `description`, `toolCount`, `totalTokens`, `lastToolName` (`agent_progress.go:19-27`) | `AgentProgressLine` React component with 16 props: `agentType`, `description`, `name`, `descriptionColor`, `taskDescription`, `toolUseCount`, `tokens`, `color`, `isLast`, `isResolved`, `isError`, `isAsync`, `shouldAnimate`, `lastToolInfo`, `hideType` (`AgentProgressLine.tsx:6-22`) | 简化 |
| Tree-style rendering | Flat ANSI: `"\r\x1b[K[agent: desc] Running tool · N tool uses · M tokens"` (`agent_progress.go:117-139`) | Tree chars: `├─` / `└─` with nested `⎿` status line (`AgentProgressLine.tsx:41,98-100`) | 简化 |
| Resolved state | **None** — always shows "Running" | `isResolved` toggles between "Running {tool}" → "Done" or "Running in the background" (`AgentProgressLine.tsx:45-53`) | 缺失 |
| Error state | **None** | `isError` prop for error rendering (`AgentProgressLine.tsx:19`) | 缺失 |
| Async/background tracking | **None** | `isAsync && isResolved` → "Running in the background" status, hides tool count/tokens (`AgentProgressLine.tsx:42,49-51,88-94`) | 缺失 |
| Color-coded agent type | **None** — plain text | `backgroundColor={color}` badge for agent type label with `inverseText` color (`AgentProgressLine.tsx:69-73`) | 缺失 |
| Animation control | **None** | `shouldAnimate` prop for spinner/animation state (`AgentProgressLine.tsx:19`) | 缺失 |
| Token formatting | Raw integer: `fmt.Sprintf("%d tokens", w.totalTokens)` (`agent_progress.go:134`) | `formatNumber(tokens)` with locale-aware formatting (`AgentProgressLine.tsx:92`) | 简化 |

### 3.2 Tool Result Rendering Architecture

| Aspect | Go (file:line) | Upstream (file:line) | Type |
|--------|----------------|---------------------|------|
| Tool result routing | Single path: `stripAnsi()` + regex extraction of tool names from `[+]`/`[ERR]` lines (`agent_progress.go:76-103`) | 4-way dispatch: `UserToolCanceledMessage`, `UserToolRejectMessage`, `UserToolErrorMessage`, `UserToolSuccessMessage` based on `param.content` prefix and `param.is_error` (`UserToolResultMessage.tsx:48-100`) | 简化 |
| Message type rendering | Flat text output via `fmt.Fprintf` | 40+ specialized React components in `components/messages/`: `AdvisorMessage`, `AssistantThinkingMessage`, `CompactBoundaryMessage`, `CollapsedReadSearchContent`, `GroupedToolUseContent`, `SnipBoundaryMessage`, `UserBashOutputMessage`, `UserForkBoilerplateMessage`, `PlanApprovalMessage`, etc. (`components/messages/`) | 缺失 |
| Thinking block rendering | `ThinkFilterState` wraps in ANSI dim (`streaming.go:300-397`) | `AssistantThinkingMessage` with expandable/collapsible UI, `AssistantRedactedThinkingMessage` for `redacted_thinking` blocks, `lastThinkingBlockId` for transcript-mode hiding (`Message.tsx:445-465`) | 简化 |
| Collapsed read/search grouping | **None** — all tool results displayed individually | `CollapsedReadSearchContent` — groups consecutive Read/Grep/Glob tool uses into collapsible UI with `OffscreenFreeze` for scrollback optimization (`Message.tsx:258-280`) | 缺失 |
| Grouped tool use rendering | **None** | `GroupedToolUseContent` — groups parallel tool calls into single expandable UI (`Message.tsx:247-255`) | 缺失 |
| Image rendering | **None** | `UserImageMessage` with `imageId` reference and `ClickableImageRef` (`Message.tsx:328-334`) | 缺失 |
| Attachment rendering | Inline text with `[history-snip]` prefix (`context.go:969`) | `AttachmentMessage` with rich rendering for file attachments, memory attachments (`Message.tsx:104-112`) | 简化 |

**Gap**: Go's rendering is a flat text stream with ANSI escape codes. Upstream's rendering is a full React component tree with 40+ specialized message components, each handling specific content types with rich interactive UI (expand/collapse, image display, grouped tool uses, thinking block visibility toggles). The `CollapsedReadSearchContent` and `GroupedToolUseContent` components provide significant UX improvements for tool-heavy conversations.

---

### 4. Conversation Branching — `/branch` Command

### Files Compared
- **Go**: No equivalent — sessions are linear
- **Upstream**: `src/commands/branch/branch.ts` (297 lines), `src/commands/branch/index.ts`, `src/commands/fork/fork.tsx` (74 lines), `src/commands/fork/index.ts`

### 4.1 `/branch` — Full Session Forking

| Aspect | Go (file:line) | Upstream (file:line) | Type |
|--------|----------------|---------------------|------|
| Conversation branching | **None** — sessions are linear, no fork/branch capability | `/branch [title]` creates a new session with full transcript copy (`branch.ts:222-296`) | 缺失 |
| Session ID generation | **None** | `randomUUID()` for fork session ID (`branch.ts:68`) | 缺失 |
| Transcript file copy | **None** — Go uses flat JSONL with no branching | Reads current JSONL, rewrites with new `sessionId`, preserved `parentUuid` chain, and `forkedFrom: { sessionId, messageUuid }` traceability (`branch.ts:88-146`) | 缺失 |
| Content-replacement preservation | **None** | Copies `content-replacement` entries with fork's sessionId to prevent cache miss on resume (`branch.ts:99-112, 150-158`) | 缺失 |
| Title management | **None** | `deriveFirstPrompt()` from first user message; `getUniqueForkName()` with collision-safe " (Branch N)" suffix (`branch.ts:38-54, 179-220`) | 缺失 |
| Title collision resolution | **None** | Regex-based enumeration: "name (Branch)" → "name (Branch 2)" → etc. with `usedNumbers` set (`branch.ts:192-219`) | 缺失 |
| Session resume into branch | **None** | `context.resume(sessionId, forkLog, 'fork')` switches REPL into the branched session with full state transfer (`branch.ts:279-281`) | 缺失 |
| Resume hint | **None** | `To resume the original: claude -r ${originalSessionId}` displayed after branching (`branch.ts:276`) | 缺失 |
| Analytics tracking | **None** | `logEvent('tengu_conversation_forked', { message_count, has_custom_title })` (`branch.ts:254-257`) | 缺失 |

### 4.2 `/fork` — Live Subagent Forking

| Aspect | Go (file:line) | Upstream (file:line) | Type |
|--------|----------------|---------------------|------|
| Fork subagent command | **None** — Go has `AgentTool.SpawnFunc` for sub-agents but no `/fork` command | `/fork <directive>` — JSX command with `FEATURE_FORK_SUBAGENT` feature flag gate (`fork.tsx:13-16`) | 缺失 |
| Recursive fork guard | **None** | `isInForkChild(context.messages)` prevents fork-within-fork (`fork.tsx:20-23`) | 缺失 |
| AgentTool delegation | Go's `AgentTool` spawns directly via `SpawnFunc` | `/fork` delegates to `AgentTool.call()` with `run_in_background: true` and omitted `subagent_type` for implicit fork (`fork.tsx:47-48, 55-59`) | Go适配 |

**Gap**: Go has no conversation branching. Upstream's `/branch` creates a full fork of the conversation with transcript copy, title management, collision resolution, content-replacement preservation, and session resume. The `/fork` command provides a live subagent that inherits the parent's conversation context. Both are major UX features for non-linear conversation workflows.

---

### 5. Subagent Context Isolation — `createSubagentContext`

### Files Compared
- **Go**: `forked_agent.go:81-270` (`RunForkedAgent` with `CanUseToolFn` permission check)
- **Upstream**: `src/utils/forkedAgent.ts:345-466` (`createSubagentContext` with `SubagentContextOverrides`)

### 5.1 Isolation Dimensions

| Aspect | Go (file:line) | Upstream (file:line) | Type |
|--------|----------------|---------------------|------|
| readFileState isolation | **None** — fork shares parent's tool registry directly | `cloneFileStateCache(parent.readFileState)` — deep clone prevents parent state mutation (`forkedAgent.ts:383-385`) | 缺失 |
| AbortController isolation | **None** — fork uses `context.WithTimeout` (120s) (`forked_agent.go:116`) | `createChildAbortController(parent.abortController)` — child linked to parent; parent abort propagates (`forkedAgent.ts:349-354`) | Go适配 |
| getAppState wrapping | **None** | Wraps parent's `getAppState` to set `shouldAvoidPermissionPrompts: true` unless sharing abortController (`forkedAgent.ts:357-374`) | 缺失 |
| setAppState isolation | **None** — Go has no React state | Default no-op `() => {}`; opt-in `shareSetAppState` for interactive subagents (`forkedAgent.ts:414-415`) | 缺失 |
| localDenialTracking | **None** — Go tracks denials via `CanUseToolFn` | Fresh `createDenialTrackingState()` when not sharing setAppState; else shares parent's (`forkedAgent.ts:424-426`) | 缺失 |
| nestedMemoryAttachmentTriggers | **None** | Fresh `new Set<string>()` per subagent (`forkedAgent.ts:386`) | 缺失 |
| discoveredSkillNames | **None** | Fresh `new Set<string>()` per subagent (`forkedAgent.ts:389`) | 缺失 |
| toolDecisions | **None** | `undefined` — no inherited tool decisions (`forkedAgent.ts:391`) | 缺失 |
| contentReplacementState | **None** | Cloned from parent via `cloneContentReplacementState()` — ensures identical cache-sharing replacement decisions (`forkedAgent.ts:396-407`) | 缺失 |
| setToolJSX / setStreamMode / setSDKStatus | **None** — Go has no React UI | All `undefined` for subagents — can't control parent UI (`forkedAgent.ts:443-446`) | 缺失 |
| queryTracking chain | **None** | New `chainId` via `randomUUID()`, incremented `depth` from parent (`forkedAgent.ts:456-459`) | 缺失 |
| criticalSystemReminder_EXPERIMENTAL | **None** | Per-subagent override for critical reminder injection at every user turn (`forkedAgent.ts:463`) | 缺失 |
| requireCanUseTool | **None** | Forces `canUseTool` even when hooks auto-approve — used by speculation for overlay file path rewriting (`forkedAgent.ts:298-299, 464`) | 缺失 |
| langfuseRootTrace | **None** | Preserves parent's Langfuse trace for nested side queries (`forkedAgent.ts:380-381`) | 缺失 |
| onMessage callback | **None** — fork returns `ForkedAgentResult` synchronously | `onMessage?: (message: Message) => void` — per-message streaming callback for UI updates (`forkedAgent.ts:107`) | 缺失 |
| skipTranscript | **None** — Go doesn't record fork transcripts | `skipTranscript` prevents sidechain transcript recording for ephemeral work (`forkedAgent.ts:109, 532-533`) | 缺失 |
| skipCacheWrite | **None** | `skipCacheWrite` skips writing new prompt cache entries on last message for fire-and-forget forks (`forkedAgent.ts:110-111`) | 缺失 |

**Gap**: Go's `RunForkedAgent` shares parent state freely — only `CanUseToolFn` provides permission isolation. Upstream's `createSubagentContext` has 25+ isolation dimensions with explicit opt-in sharing flags (`shareSetAppState`, `shareSetResponseLength`, `shareAbortController`). The `contentReplacementState` cloning is particularly notable — without it, cache-sharing forks would make divergent replacement decisions, causing cache misses.

---

### 6. Status Line and Cost Display Integration

### Files Compared
- **Go**: No status line — token counts printed via `fmt.Fprintf` after tool execution
- **Upstream**: `src/components/StatusLine.tsx`, `src/costHook.ts`

### 6.1 Status Line Component

| Aspect | Go (file:line) | Upstream (file:line) | Type |
|--------|----------------|---------------------|------|
| Status line | **None** — no persistent status bar | `StatusLine` React component with configurable display (`StatusLine.tsx:59-64`) | 缺失 |
| Model display | **None** | Model name with `renderModelName()` and context window percentage (`StatusLine.tsx:50-56`) | 缺失 |
| Permission mode | **None** | Permission mode badge in status line (`StatusLine.tsx:66-73`) | 缺失 |
| Cost display | Raw token counts via `fmt.Fprintf` after API calls | `getTotalCost()` from cost-tracker, `formatCost()` with smart decimal places (`StatusLine.tsx:24-25`) | 缺失 |
| Context usage | **None** | `calculateContextPercentages()` shows used/total context window percentage (`StatusLine.tsx:37`) | 缺失 |
| Duration tracking | **None** | `getTotalAPIDuration()`, `getTotalDuration()` — API vs wall time (`StatusLine.tsx:23`) | 缺失 |
| Lines changed | **None** | `getTotalLinesAdded()`, `getTotalLinesRemoved()` — code change tracking (`StatusLine.tsx:26`) | 缺失 |
| Vim mode indicator | **None** | Vim mode in status line for vim input users (`StatusLine.tsx:57`) | 缺失 |
| Custom status line command | **None** | `executeStatusLineCommand()` — user-configurable status line via hooks (`StatusLine.tsx:44`) | 缺失 |

**Gap**: Go has no persistent status line. Users cannot see model name, cost, context usage, or duration without scrolling through output. Upstream's status line provides real-time visibility into all key session metrics.

---

*End of Section XXI. Total new table rows: 68 across 6 subsections covering notification queue, input handling, TUI rendering, conversation branching, subagent isolation, and status line integration.*



---

## 47. TUI/UI System, Print/Rendering, Session Management

### Files Compared
- **Go**: No TUI — headless CLI with `fmt.Print/Fprintf` to stdout/stderr
- **Upstream**: `src/components/`, `src/screens/`, `src/commands/`, `src/utils/` — full React/Ink TUI

### 47.1 TUI Framework

| # | Aspect | Go | Upstream | Type |
|---|--------|----|----------|------|
| 1 | Full TUI framework (Ink/React) | No TUI; headless CLI with fmt.Print to stdout/stderr | `components/App.tsx` — Ink/React component tree with AppStateProvider, StatsProvider, FpsMetricsProvider | 缺失 |
| 2 | REPL screen | `main.go:190` — runInteractive() with bufio.NewReader + select on channels | `screens/REPL.tsx:803` — Full React REPL with 5700+ lines of interactive TUI | 缺失 |
| 3 | Virtual message list | No message list; output is sequential fmt.Println | `components/VirtualMessageList.tsx:257` — Virtual scroll, search highlighting, jump handle | 缺失 |
| 4 | Streaming progress spinner | `streaming.go:399` — TerminalHandler prints [Tool: name] and [THINK] as plain text | `components/Spinner/SpinnerGlyph.tsx` — Animated spinner with color interpolation, stall detection, reduced-motion mode | 缺失 |
| 5 | Think tag filter | `streaming.go:302` — ThinkFilterState state machine filters `<thinking>`/`</thinking>` tags, outputs dim text | `components/ThinkingMessage.tsx` — Collapsible thinking block with expand/collapse UI | 缺失 |
| 6 | Tool use rendering | `streaming.go:399` — [Tool: name] plain text prefix | `components/ToolUseMessage.tsx` — Rich tool use display with input schema, progress, result rendering | 缺失 |
| 7 | Permission prompt UI | `permissions.go:498-523` — Simple y/N console prompt | `components/ToolUseConfirm.tsx` — Full interactive permission dialog with allow/deny/ask, rule suggestions, always-allow toggle | 缺失 |
| 8 | Diff/patch rendering | No diff rendering | `components/DiffMessage.tsx` — Syntax-highlighted diff display | 缺失 |
| 9 | Error message rendering | Plain error text to stderr | `components/ErrorMessage.tsx` — Styled error display with context | 缺失 |
| 10 | Compact boundary rendering | No visual indicator | `components/CompactBoundaryMessage.tsx` — Visual compact boundary with token savings display | 缺失 |
| 11 | Image rendering | No image display | `components/ImageMessage.tsx` — Inline image display with Sixel/iTerm2 protocol | 缺失 |
| 12 | Command palette | No command palette | `components/CommandPalette.tsx` — Fuzzy search command palette with keyboard navigation | 缺失 |
| 13 | Vim mode | No vim mode | `vim/` — Full vim keybindings, modes (normal/insert/visual), navigation | 缺失 |
| 14 | Keybinding system | No configurable keybindings | `keybindings/` — Configurable keybinding system with user customization | 缺失 |
| 15 | Status line | No status line | `components/StatusLine.tsx` — Dynamic status line with model, tokens, cost, mode | 缺失 |

### 47.2 Print/Output System

| # | Aspect | Go | Upstream (`cli/print.ts`) | Type |
|---|--------|----|--------------------------|------|
| 1 | Print system architecture | Direct fmt.Print/Fprintf to stdout/stderr | `cli/print.ts` (~4000 lines) — Full print pipeline with message rendering, streaming, tool results | 缺失 |
| 2 | Tool result formatting | Plain text output with truncation | `printToolResult()` — Rich result rendering with syntax highlighting, diff display, image preview | 缺失 |
| 3 | Streaming text display | Sequential character output | Incremental rendering with virtual scroll, line wrapping, ANSI color support | 缺失 |
| 4 | Usage/cost display | Basic token counts in XML (agent_loop.go:237) | `formatTotalCost()` — Formatted cost display with per-model breakdown, API duration, wall duration | 缺失 |
| 5 | Session resume display | Simple "Resuming session..." text | Full session state restoration display with compact boundary, recovered files | 缺失 |

### 47.3 Slash Commands

| # | Aspect | Go (`main.go:363-456`) | Upstream (`commands/`) | Type |
|---|--------|------------------------|----------------------|------|
| 1 | Command count | 9 commands: /quit /tools /mode /help /compact /partialcompact /clear /resume /agents | 30+ commands: /login /mock-limits /config /doctor /share /review /mcp /memory /skill /dream /status etc. | 简化 |
| 2 | Command registration | Inline switch/case in main.go | `commands.ts` — Modular command registration system with help text, aliases, argument parsing | 简化 |
| 3 | /compact command | Basic manual compact trigger | Full /compact with custom instructions, partial compact, SM-compact options | 简化 |
| 4 | /resume command | Find transcript by number/name/last | Full session resume with worktree, teleport, state restoration | 简化 |
| 5 | /agents command | Simple list/show/stop | Full agent management with background tasks, progress, output streaming | 简化 |

### 47.4 Session Management

| # | Aspect | Go | Upstream | Type |
|---|--------|----|----------|------|
| 1 | Session persistence | JSONL transcript files in `.claude/transcripts/` | Full session state with transcript + metadata + cost state + permission state | 简化 |
| 2 | Session resume | Transcript replay with context rebuild | Full session resume: conversationRecovery with TurnInterruptionState, permission state, cost state | 简化 |
| 3 | Session sharing | No session sharing | `/share` command with conversation export, redaction, URL generation | 缺失 |
| 4 | Session teleport (CCR) | No remote session support | Bridge protocol with WebSocket, JWT auth, session management | 缺失 |
| 5 | Session backgrounding | No background session support | `useSessionBackgrounding.ts` — session pause/resume on terminal focus | 缺失 |

---


---

## 53. Upstream Component System vs Go Headless CLI

The upstream claude-code uses a React/Ink TUI component system to render the
interactive REPL, messages, diffs, permission prompts, status line, and other
visual elements. The Go port (miniClaudeCode-go) is a headless CLI that replaces
each React component with direct terminal I/O: `fmt.Fprintf(os.Stderr, ...)`,
ANSI escape sequences, and `bufio.Scanner` for prompts. The mapping below
documents what each upstream component does and how the Go version provides the
equivalent functionality without React/Ink.

---

### 53.1 REPL Component

**Upstream**: `src/screens/REPL.tsx` (lines 1-100+)
**Go**: `main.go:190` `runInteractive()`

| # | Aspect | Go (file:line) | Upstream (file:line) | Type |
|---|--------|---------------|---------------------|------|
| 1 | REPL entry point | `main.go:190` `runInteractive(agent)` | `REPL.tsx:1` `REPL()` component | Go适配 |
| 2 | Input loop mechanism | `main.go:260` `for { ... select { case r := <-inputCh: } }` with goroutine-based `readStdin` | React `useInput` hook + Ink `useStdin` for keyboard events | 简化 |
| 3 | Ctrl+C / SIGINT handling | `main.go:197` signal channel + double-press detection (`lastCtrlC` atomic + 2s window) | `REPL.tsx` uses Ink `useInput` with Escape/Ctrl+C keybinding dispatch | Go适配 |
| 4 | Double Ctrl+C = exit | `main.go:204` `time.Duration(now-prev) < 2*time.Second` → `os.Exit(0)` | Upstream: Escape key or double Ctrl+C via keybinding handler | 简化 |
| 5 | Ctrl+C during agent run | `main.go:211` `agent.SetInterrupted(true)` + non-blocking notify to `ctrlCh` | Upstream: abort controller + `AbortController.abort()` from keybinding | 简化 |
| 6 | Windows Ctrl+C EOF recovery | `main.go:314` detects Ctrl+C-triggered EOF, waits 200ms for `ctrlCh`/`lastCtrlC`, reopens stdin | N/A (upstream uses Ink which handles this internally) | Go增强 |
| 7 | stdin reader restart after agent run | `main.go:272` creates fresh `bufio.NewReader(os.Stdin)` + goroutine each loop iteration | N/A (React/Ink manages input lifecycle) | Go增强 |
| 8 | Prompt display | `main.go:288` `fmt.Print("\n> ")` | `PromptInput.tsx` renders styled prompt with mode prefix, context suggestions, typeahead | 简化 |
| 9 | Slash command dispatch | `main.go:359` `if strings.HasPrefix(userInput, "/")` + switch on known commands | `PromptInput.tsx` `useTypeahead` + `commands.ts` command registry with 40+ commands | 简化 |
| 10 | One-shot mode | `main.go:161` positional args → `agent.Run(prompt)` + print result | `REPL.tsx` detects non-interactive via `consumeEarlyInput()` | Go适配 |
| 11 | Streaming vs non-streaming output | `main.go:477` `if !agent.IsStreaming() \|\| !stdoutIsTerm` → print result | REPL renders `<Messages>` component which uses `VirtualMessageList` for scrollback | 简化 |
| 12 | Sub-agent notification drain | `main.go:277` `agent.DrainNotifications()` + `agent.InjectNotifications()` | Upstream: swarm/agent notification system via React state + `useNotifications` context | 简化 |
| 13 | Resume from transcript | `main.go:128` `findTranscript(*resumeFile)` + `NewAgentLoopFromTranscript()` | `REPL.tsx` uses `resume` command via command registry + `switchSession()` | Go适配 |
| 14 | Session cost tracking display | Not displayed in REPL loop (cost tracked internally) | `REPL.tsx` uses `useCostSummary` + `getTotalCost` for inline cost display | 缺失 |
| 15 | Terminal focus / notification | Not implemented | `REPL.tsx` uses `useTerminalFocus` + `useTerminalNotification` for OS notifications | 缺失 |
| 16 | Search/navigate in transcript | Not implemented in REPL | `REPL.tsx` uses `useSearchInput` + `VirtualMessageList` `JumpHandle` for `/` search | 缺失 |

---

### 53.2 Tool Use Display

**Upstream**: `src/components/messages/AssistantToolUseMessage.tsx` (lines 1-80)
**Go**: `streaming.go:317` `TerminalHandler`

| # | Aspect | Go (file:line) | Upstream (file:line) | Type |
|---|--------|---------------|---------------------|------|
| 1 | Tool call event | `streaming.go:404` `case ChunkTypeToolCall:` → `fmt.Fprintf(os.Stderr, "[Tool: %s] ", chunk.Name)` | `AssistantToolUseMessage.tsx:40` renders `<ToolUseLoader>` with spinner + tool name | 简化 |
| 2 | Tool argument accumulation | `streaming.go:421` `case ChunkTypeToolArgument:` → `h.curToolArgs.WriteString(chunk.Content)` | `AssistantToolUseMessage.tsx` uses `param.input` (already assembled by SDK) | Go适配 |
| 3 | Tool call flush with summary | `streaming.go:470` `flushToolCall()` → `toolArgSummary()` → compact one-line display | `AssistantToolUseMessage.tsx` renders full `MessageResponse` with diff previews, file paths | 简化 |
| 4 | Tool arg summary per tool | `streaming.go:482` `toolArgSummary()` — switch on tool name extracts key param (file_path, command, pattern, etc.) | `AssistantToolUseMessage.tsx` delegates to tool-specific `userFacingName()` + `MessageResponse` | 简化 |
| 5 | Pending tool progress | Not implemented (no spinner) | `ToolUseLoader.tsx` shows animated spinner + `inProgressToolUseIDs` set | 缺失 |
| 6 | Permission mode display | Not shown during tool use | `AssistantToolUseMessage.tsx:60` reads `toolPermissionContext.mode` for auto-classifier logic | 缺失 |
| 7 | Auto-classifier integration | `permissions.go:411` `checkAutoMode()` handles classifier logic separately | `AssistantToolUseMessage.tsx:60` `useIsClassifierChecking(param.id)` for inline display | 简化 |
| 8 | Tool call result preview | `agent_loop.go:2439` `toolResultPreview()` generates truncated output preview | `AssistantToolUseMessage.tsx` renders full `MessageResponse` with syntax highlighting | 简化 |
| 9 | Hook progress messages | Not displayed inline | `AssistantToolUseMessage.tsx:23` imports `HookProgressMessage` for hook status | 缺失 |
| 10 | Tool use animation | Not implemented | `AssistantToolUseMessage.tsx:34` `shouldAnimate` + `shouldShowDot` props control spinner | 缺失 |

---

### 53.3 Thinking Display

**Upstream**: `src/components/messages/AssistantThinkingMessage.tsx` (lines 1-66)
**Go**: `streaming.go:300` `ThinkFilterState` + `streaming.go:330` `filterThinking()`

| # | Aspect | Go (file:line) | Upstream (file:line) | Type |
|---|--------|---------------|---------------------|------|
| 1 | Thinking state machine | `streaming.go:302` `ThinkFilterState` enum: `ThinkNormal`, `ThinkInTag`, `ThinkInBlock`, `ThinkClosing` | `AssistantThinkingMessage.tsx:23` reads `param.thinking` string (SDK provides assembled block) | Go适配 |
| 2 | Filter `<thinking>...</thinking>` tags | `streaming.go:330` `filterThinking()` — character-by-character state machine strips tags, wraps content in ANSI dim (`\033[2m` / `\033[0m`) | `AssistantThinkingMessage.tsx:23` receives already-parsed `thinking` string, no tag stripping needed | Go增强 |
| 3 | Filter `<think>...</think>` tags | `streaming.go:345` handles both `<thinking>` and `<think>` variants | N/A (SDK normalizes to `ThinkingBlock`) | Go增强 |
| 4 | Extended thinking chunk | `streaming.go:401` `case ChunkTypeThinking:` → buffer in `h.thinkingBuf` | `AssistantThinkingMessage.tsx:25` receives `param.thinking` as complete string | 简化 |
| 5 | Thinking preview (first line) | `streaming.go:409` `lines[0]` truncated to 120 chars → `fmt.Fprintf(os.Stderr, "\n[THINK] %s\n", preview)` | `AssistantThinkingMessage.tsx:41` if not verbose, shows just `"∴ Thinking"` label with `CtrlOToExpand` hint | 简化 |
| 6 | Verbose/full thinking display | `streaming.go:461` filtered content printed to stderr in dim ANSI | `AssistantThinkingMessage.tsx:51` full `Markdown` render with dim color in verbose mode | 简化 |
| 7 | Thinking buffer flush before tool call | `streaming.go:407` shows buffered thinking before first tool call starts | N/A (React renders thinking and tool use as separate message blocks) | Go适配 |
| 8 | Thinking buffer flush on Done | `streaming.go:433` if no tool call seen, flush thinking at stream end | N/A (React renders all blocks at once) | Go适配 |
| 9 | Ctrl+O expand toggle | Not implemented | `AssistantThinkingMessage.tsx:46` `<CtrlOToExpand />` component for interactive expand | 缺失 |
| 10 | Transcript mode thinking | Not distinguished (always shown if verbose, always preview if not) | `AssistantThinkingMessage.tsx:39` `isTranscriptMode` controls full display; `hideInTranscript` hides past thinking | 缺失 |

---

### 53.4 Permission Prompts

**Upstream**: `src/components/permissions/PermissionPrompt.tsx` (lines 1-80)
**Go**: `permissions.go:402` `askUser()` / `permissions.go:498` `askUserWithWarning()`

| # | Aspect | Go (file:line) | Upstream (file:line) | Type |
|---|--------|---------------|---------------------|------|
| 1 | Permission prompt rendering | `permissions.go:507` `fmt.Print(prompt)` with `[Permission] Allow 'tool': detail? [y/N]` | `PermissionPrompt.tsx:52` `<Select>` component with styled options, keybinding hints, feedback input | 简化 |
| 2 | Accept/reject feedback text | Not implemented (simple y/N) | `PermissionPrompt.tsx:60` `acceptFeedback`/`rejectFeedback` state + Tab to expand input mode | 缺失 |
| 3 | Tool detail extraction | `permissions.go:500` switch on toolName → extract `command`, `file_path` for display | `PermissionPrompt.tsx` receives full `options` array with ReactNode labels | 简化 |
| 4 | Warning display | `permissions.go:511` `prompt += "\n  [WARN] " + warning` | `PermissionPrompt.tsx` uses `question` prop (defaults to "Do you want to proceed?") | 简化 |
| 5 | User input reading | `permissions.go:517` `bufio.NewScanner(os.Stdin)` + `scanner.Scan()` | `PermissionPrompt.tsx:52` Ink `<Select>` with keyboard navigation | 简化 |
| 6 | Permission gate flow (steps 0-4) | `permissions.go:102` `Check()` — full 5-step gauntlet: auto strip → deny rules → ask rules → tool self-check → bypass/allow | Upstream: `hasPermissionsToUseToolInner` — same 5-step flow in TypeScript | Go适配 |
| 7 | Auto mode classifier | `permissions.go:411` `checkAutoMode()` — classifier call + 3-consecutive-denial fallback + 20-total-denial cap | Upstream: `shouldAutoApproveTool` + `ClassifierApproval` with same fallback logic | Go适配 |
| 8 | Stripped dangerous rules (auto mode) | `permissions.go:105` `g.ruleStore.StripDangerousAllowRules()` + restore on exit | Upstream: `stripDangerousPermissionsForAutoMode` + `restoreDangerousPermissions` | Go适配 |
| 9 | Sub-agent prompt avoidance | `permissions.go:398` `shouldAvoidPrompts()` → `g.config.ShouldAvoidPermissionPrompts` | Upstream: `isSubAgent()` check to suppress interactive prompts for workers | Go适配 |
| 10 | Path validation | `permissions.go:145` `permissions.ValidateReadPath()` / `permissions.ValidatePath()` with bypass/auto skip | Upstream: `validatePathAccess` with same mode-dependent behavior | Go适配 |
| 11 | Denied patterns check | `permissions.go:269` `g.config.DeniedPatterns` — substring match on command/file_path | Upstream: `deniedPatterns` in settings with glob/regex matching | 简化 |
| 12 | Keybinding shortcuts (y/n) | Not implemented — raw y/N text input | `PermissionPrompt.tsx:21` `keybinding?: KeybindingAction` per option | 缺失 |
| 13 | Plan mode read-only enforcement | `permissions.go:298` write tools blocked in plan mode | Upstream: same plan mode read-only restriction | Go适配 |

---

### 53.5 Status Display

**Upstream**: `src/components/StatusLine.tsx` (lines 1-80) + `src/components/BuiltinStatusLine.tsx` (lines 1-80)
**Go**: `agent_progress.go:19` `subAgentProgressWriter`

| # | Aspect | Go (file:line) | Upstream (file:line) | Type |
|---|--------|---------------|---------------------|------|
| 1 | Main status line | Not implemented for main REPL loop | `StatusLine.tsx:59` `statusLineShouldDisplay()` + `BuiltinStatusLine.tsx:45` renders model, context %, tokens, cost, rate limits | 缺失 |
| 2 | Sub-agent progress display | `agent_progress.go:113` `writeProgressUnsafe()` → `\r\x1b[K[agent: desc] Running tool · N tool uses · M tokens` | `BuiltinStatusLine.tsx` same info in React/Ink with model name, context %, cost | 简化 |
| 3 | Single-line update with carriage return | `agent_progress.go:121` `\r\x1b[K` (CR + clear-to-end-line) for in-place progress | React/Ink re-renders the `StatusLine` component tree | Go适配 |
| 4 | Tool count tracking | `agent_progress.go:96` `w.toolCount++` on each tool completion | `BuiltinStatusLine.tsx` receives tool count from `getTotalInputTokens`/`getTotalOutputTokens` | 简化 |
| 5 | Token count tracking | `agent_progress.go:150` `UpdateTokens(totalTokens)` called periodically | `BuiltinStatusLine.tsx:17` `usedTokens` + `contextWindowSize` props | 简化 |
| 6 | Model name display | Not shown in progress | `BuiltinStatusLine.tsx:69` `shortModel` = first two words of model name | 缺失 |
| 7 | Context usage percentage | Not shown in progress | `BuiltinStatusLine.tsx:15` `contextUsedPct` with progress bar | 缺失 |
| 8 | Cost display | Not shown in progress | `BuiltinStatusLine.tsx:18` `totalCostUsd` formatted via `formatCost()` | 缺失 |
| 9 | Rate limit display | Not shown | `BuiltinStatusLine.tsx:19` `rateLimits` with 5-hour and 7-day utilization bars + countdown | 缺失 |
| 10 | Terminal width adaptation | Not implemented | `BuiltinStatusLine.tsx:71` `wide`/`narrow` flags based on `columns >= 100` / `columns < 60` | 缺失 |
| 11 | [THINK] line suppression in sub-agents | `agent_progress.go:81` `if strings.HasPrefix(clean, "[THINK]") { return }` | N/A (upstream sub-agents don't emit [THINK] lines) | Go增强 |
| 12 | Tool result line parsing | `agent_progress.go:86` extracts tool name from `[+]`, `[ERR]`, `[TIMEOUT]` lines | N/A (upstream sub-agents report via structured messages) | Go增强 |
| 13 | ANSI stripping for matching | `agent_progress.go:185` `stripAnsi()` removes escape sequences before pattern matching | N/A (React/Ink works with structured data, not raw ANSI) | Go增强 |

---

### 53.6 Command System

**Upstream**: `src/commands.ts` (lines 1-80) + `src/components/PromptInput/PromptInput.tsx` (lines 1-80)
**Go**: `main.go:359` slash command switch

| # | Aspect | Go (file:line) | Upstream (file:line) | Type |
|---|--------|---------------|---------------------|------|
| 1 | Command dispatch | `main.go:359` `if strings.HasPrefix(userInput, "/")` → switch on known commands | `commands.ts` exports 40+ command objects; `PromptInput.tsx` uses `useTypeahead` for autocomplete | 简化 |
| 2 | /help | `main.go:395` prints command list with `fmt.Println` | `commands/help/index.ts` renders formatted help with keybinding shortcuts | 简化 |
| 3 | /quit, /exit, /q | `main.go:371` `break loop` | `commands.ts` `quit` command via command registry | Go适配 |
| 4 | /tools | `main.go:374` iterates `agent.registry.AllTools()` | `commands.ts` no direct equivalent; tool list shown via `/help` | Go适配 |
| 5 | /mode | `main.go:381` `agent.config.PermissionMode = PermissionMode(modeVal)` | `commands/autonomy.ts` + `autonomyPanel.tsx` with mode switch UI | 简化 |
| 6 | /compact | `main.go:407` `agent.ForceCompact()` | `commands/compact/compact.ts` with LLM summarization + confirmation dialog | 简化 |
| 7 | /partialcompact | `main.go:410` `agent.ForcePartialCompact(dir, pivot)` | Upstream: partial compact via `/compact` with direction arg | Go适配 |
| 8 | /clear | `main.go:421` `agent.ClearHistory()` + `agent.registry.ClearFilesRead()` | `commands/clear/index.ts` clears messages + resets state | Go适配 |
| 9 | /resume | `main.go:430` `findTranscript()` + `NewAgentLoopFromTranscript()` | `commands/resume/index.ts` + session storage + `switchSession()` | Go适配 |
| 10 | /agents | `main.go:453` `handleAgentsCommand()` | `commands/agents/index.ts` with full agent management UI | 简化 |
| 11 | Command autocomplete/typeahead | Not implemented | `PromptInput.tsx:65` `useTypeahead` with fuzzy matching on command registry | 缺失 |
| 12 | Command keybindings | Not implemented (slash commands only) | `PromptInput.tsx:66` `useKeybinding` + `useKeybindings` for keyboard shortcuts | 缺失 |
| 13 | Unknown slash passthrough | `main.go:367` unknown `/xxx` treated as normal prompt text | Upstream: same behavior — unknown commands passed as text | Go适配 |
| 14 | Upstream-only commands | N/A | `/config`, `/cost`, `/doctor`, `/memory`, `/init`, `/mcp`, `/login`, `/logout`, `/theme`, `/vim`, `/share`, `/review`, `/skills`, `/status`, `/tasks`, `/diff`, `/copy`, `/commit`, `/pr`, etc. | 缺失 |
| 15 | Command result display | `fmt.Println` / `fmt.Printf` to stdout/stderr | Commands return `CommandResultDisplay` rendered in Ink | 简化 |

---

### 53.7 Message Display

**Upstream**: `src/components/VirtualMessageList.tsx` (lines 1-80) + `src/components/Messages.tsx`
**Go**: `streaming.go:317` `TerminalHandler` + `agent_loop.go:332` `out()`

| # | Aspect | Go (file:line) | Upstream (file:line) | Type |
|---|--------|---------------|---------------------|------|
| 1 | Message rendering | `streaming.go:399` `TerminalHandler.Handle()` dispatches by `ChunkType` — prints directly to stderr | `VirtualMessageList.tsx:1` virtualized scroll list with `useVirtualScroll` | 简化 |
| 2 | Virtual scrolling | Not implemented (all output scrolls naturally in terminal) | `VirtualMessageList.tsx:12` `useVirtualScroll` for efficient render of long transcripts | 缺失 |
| 3 | Search within messages | Not implemented | `VirtualMessageList.tsx:22` `renderableSearchText` + `JumpHandle` for `/` search with n/N navigation | 缺失 |
| 4 | Sticky prompt header | `main.go:288` `fmt.Print("\n> ")` — simple prompt | `VirtualMessageList.tsx:45` `StickyPrompt` type — header stays at top during scroll | 缺失 |
| 5 | Message selection / actions | Not implemented | `VirtualMessageList.tsx:31` `isNavigableMessage` + `MessageActionsNav` for message-level actions | 缺失 |
| 6 | Streaming text output | `streaming.go:443` `case ChunkTypeText:` → `filterThinking()` → `fmt.Fprint(os.Stderr, filtered)` | `Messages.tsx` renders `MessageRow` components with Markdown rendering | 简化 |
| 7 | Agent output writer | `agent_loop.go:332` `out()` writes to configurable `io.Writer` (defaults to `os.Stderr`) | React/Ink component tree renders to virtual DOM then terminal | Go适配 |
| 8 | Sub-agent output interception | `agent_progress.go:19` `subAgentProgressWriter` intercepts and condenses sub-agent output | Upstream: sub-agent output rendered via `CoordinatorAgentStatus` component | 简化 |
| 9 | Search text extraction | Not implemented | `VirtualMessageList.tsx:37` `defaultExtractSearchText` with `WeakMap` cache | 缺失 |
| 10 | Transcript mode rendering | Not distinguished | `VirtualMessageList.tsx` supports transcript mode with Ctrl+O toggle | 缺失 |

---

### 53.8 Diff Display

**Upstream**: `src/components/StructuredDiff.tsx` (lines 1-80) + `src/components/diff/`
**Go**: No diff rendering component

| # | Aspect | Go (file:line) | Upstream (file:line) | Type |
|---|--------|---------------|---------------------|------|
| 1 | Structured diff rendering | Not implemented | `StructuredDiff.tsx:1` renders `StructuredPatchHunk` with syntax highlighting, gutter, color diff | 缺失 |
| 2 | Color diff with ANSI | Not implemented | `StructuredDiff.tsx:58` `renderColorDiff()` via Rust NAPI module with theme-aware coloring | 缺失 |
| 3 | Gutter width computation | Not implemented | `StructuredDiff.tsx:49` `computeGutterWidth()` — marker + line number + padding | 缺失 |
| 4 | Render cache for remount | Not implemented | `StructuredDiff.tsx:32` `RENDER_CACHE = new WeakMap` for Ctrl+O remount performance | 缺失 |
| 5 | Syntax highlighting | Not implemented | `StructuredDiff.tsx:68` `ColorDiff` NAPI module with tree-sitter grammars | 缺失 |
| 6 | Diff file list view | Not implemented | `diff/DiffFileList.tsx:16` scrollable file list with pagination (5 visible) | 缺失 |
| 7 | Diff detail view | Not implemented | `diff/DiffDetailView.tsx` full hunk view with accept/reject | 缺失 |
| 8 | Diff dialog | Not implemented | `diff/DiffDialog.tsx` modal overlay for reviewing file edits | 缺失 |
| 9 | File edit diff preview | Tool results printed as plain text via `toolResultPreview()` | `FileEditToolDiff.tsx` renders structured diff for edit_file results | 缺失 |
| 10 | RawAnsi optimization | Not applicable (no diff rendering) | `StructuredDiff.tsx:38` `RawAnsi` columns bypass Ink parsing for perf | 缺失 |

---

### 53.9 Error Display

**Upstream**: `src/components/messages/SystemAPIErrorMessage.tsx` (lines 1-68)
**Go**: `error_types.go:1` + `main.go` `fmt.Fprintf(os.Stderr, ...)` + `agent_loop.go` error handling

| # | Aspect | Go (file:line) | Upstream (file:line) | Type |
|---|--------|---------------|---------------------|------|
| 1 | Error classification | `error_types.go:15` 15-category `ErrorClass` taxonomy (retryable, rate_limit, billing, context_overflow, etc.) | `SystemAPIErrorMessage.tsx:4` uses `formatAPIError()` from `@ant/model-provider` | Go增强 |
| 2 | Error pattern matching | `error_types.go:60` 100+ patterns for billing, rate limit, context overflow, auth, model not found, etc. | Upstream: error formatting via `formatAPIError` with message extraction | Go增强 |
| 3 | Retry countdown display | Not displayed (retry logic happens silently) | `SystemAPIErrorMessage.tsx:29` `useInterval` + countdown `Retrying in N seconds... (attempt X/Y)` | 缺失 |
| 4 | Error truncation | `error_types.go:165` `truncateStr(errMsg, 500)` in ClassifyResult | `SystemAPIErrorMessage.tsx:10` `MAX_API_ERROR_CHARS = 1000` + verbose toggle | 简化 |
| 5 | Verbose/full error toggle | Not implemented (always shows truncated) | `SystemAPIErrorMessage.tsx:46` `truncated && <CtrlOToExpand />` | 缺失 |
| 6 | Error color styling | Plain text to stderr | `SystemAPIErrorMessage.tsx:51` `<Text color="error">` for red error display | 简化 |
| 7 | API_TIMEOUT_MS hint | Not displayed | `SystemAPIErrorMessage.tsx:61` `API_TIMEOUT_MS=...ms, try increasing it` | 缺失 |
| 8 | Early retry suppression | Not implemented (all retries shown) | `SystemAPIErrorMessage.tsx:27` `hidden` = suppress first 4 retries on external builds | 缺失 |
| 9 | Recovery hints in ClassifyResult | `error_types.go:36` `Compress`, `RotateKey`, `Fallback`, `RetryAfter` fields drive recovery strategy | Upstream: retry logic in `callWithRetry` / `callWithRetryAndFallback` without structured hints | Go增强 |
| 10 | Stream error chunk | `streaming.go:166` `h.Err = fmt.Errorf("stream error: %s", chunk.Content)` | `SystemAPIErrorMessage.tsx` handles `error` type from API | 简化 |
| 11 | Error printing in REPL | `main.go:435` `fmt.Printf("Error: %v\n", err)` | `MessageResponse` wrapper with `<Text color="error">` | 简化 |

---

### 53.10 Compact Boundary

**Upstream**: `src/components/messages/CompactBoundaryMessage.tsx` (lines 1-19)
**Go**: `compact.go:99` `OmissionMarker` + `context.go:48` `CompactBoundaryContent`

| # | Aspect | Go (file:line) | Upstream (file:line) | Type |
|---|--------|---------------|---------------------|------|
| 1 | Boundary marker format | `compact.go:100` `OmissionMarker = "<!-- %d earlier conversation rounds omitted to save context -->"` | `CompactBoundaryMessage.tsx:14` `"✻ Conversation compacted (Ctrl+O for history)"` | 简化 |
| 2 | Boundary content type | `context.go:48` `CompactBoundaryContent` struct with Trigger, PreCompactTokens, UUID, PreCompactDiscoveredTools, PreservedSegment | Upstream: `compactMetadata` in message with same fields | Go适配 |
| 3 | AddCompactBoundary | `context.go:858` `AddCompactBoundary(trigger, preCompactTokens, opts...)` — inserts boundary marker into conversation | Upstream: `addCompactBoundaryMessage` inserts system message with compact metadata | Go适配 |
| 4 | Compaction trigger enum | `compact.go:880` `CompactTrigger.String()` with auto/forced/reactive/partial/llm/micro values | Upstream: `CompactionTrigger` type with same trigger kinds | Go适配 |
| 5 | Ctrl+O for history | Not implemented (no transcript viewer) | `CompactBoundaryMessage.tsx:7` `useShortcutDisplay('app:toggleTranscript', 'Global', 'ctrl+o')` | 缺失 |
| 6 | Omission count display | `compact.go:100` `<!-- %d earlier conversation rounds omitted -->` (HTML comment in context) | `CompactBoundaryMessage.tsx:14` user-visible "✻ Conversation compacted" message | 简化 |
| 7 | PreservedSegment for chain relinking | `context.go:68` `PreservedSegment` struct with HeadUUID, AnchorUUID, TailUUID | Upstream: `compactMetadata.preservedSegment` with same UUIDs for partial compact chain repair | Go适配 |
| 8 | CompactionResult summary | `compact.go:703` `Summary()` method with omitted/kept counts, token savings, compaction ratio | Upstream: `CompactionResult` with same metrics | Go适配 |
| 9 | Context window tracker | `compact.go:1127` `ContextWindowTracker` with model-aware window sizes, threshold, ShouldCompact() | Upstream: `ContextWindowTracker` with same logic | Go适配 |
| 10 | LLM-based compaction | `compact.go:1418` `compactConversationLLM()` — calls LLM to generate summary | Upstream: `compactWithSummary` — same LLM-based summarization | Go适配 |
| 11 | Reactive compaction | `compact.go:2556` `CheckReactiveCompact()` — triggers on token growth rate | Upstream: `reactiveCompact` with same growth-rate detection | Go适配 |
| 12 | Micro-compact (cache edits) | `compact.go:2721` `CachedMicrocompactTracker` for compacting tool results with cache edits | Upstream: `CachedMicrocompactTracker` same cache-edit bundling | Go适配 |


---

