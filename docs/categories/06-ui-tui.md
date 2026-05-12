# 06 — UI, TUI & Session Management

> Terminal UI, components, rendering, keybindings, voice, session history

## Overview

Go is a headless CLI with no interactive TUI. The upstream has a full Ink/React terminal interface with 20+ context-aware keymaps, vim mode, voice input, and rich session management.

---

## 1. TUI Architecture

| Aspect | Go | Upstream | Gap |
|--------|-----|----------|-----|
| TUI framework | None (headless CLI) | Ink (React for CLI) with 50+ components | 缺失 |
| Component tree | N/A | `App` → `REPL` → `ChatInput` + `Transcript` + `StatusBar` + `Footer` | 缺失 |
| State management | N/A | React hooks + Zustand stores | 缺失 |
| Rendering | N/A | Ink's Yoga-based flexbox layout | 缺失 |
| Theme system | N/A | Multiple themes with picker UI | 缺失 |

**Impact**: **Architectural gap** — most UI features below depend on a TUI layer. Go would need a TUI framework (e.g., Bubble Tea) before these make sense.

---

## 2. Keybinding System

| Aspect | Go | Upstream | Gap |
|--------|-----|----------|-----|
| Keybinding system | None | Full architecture with context-aware resolution | 缺失 |
| Context count | N/A | 20+: Global, Chat, Autocomplete, Settings, Confirmation, FormField, Tabs, Transcript, HistorySearch, Task, ThemePicker, etc. | 缺失 |
| User overrides | None | `keybindings.json` user config file | 缺失 |
| Reserved shortcuts | None | `ctrl+c` (interrupt) and `ctrl+d` (exit) cannot be rebound | 缺失 |
| Platform-specific | N/A | Windows: `alt+v` for paste; macOS/Linux: `ctrl+v` | 缺失 |
| Kitty keyboard protocol | N/A | Detection for Kitty/WezTerm/Ghostty/iTerm2 | 缺失 |

---

## 3. Vim Mode

| Aspect | Go | Upstream | Gap |
|--------|-----|----------|-----|
| Vim mode | None | Full emulation: INSERT/NORMAL, operators (d/c/y), motions (w/b/e), text objects (iw/aw/i"/a"), count prefix, dot-repeat, registers | 缺失 |
| State machine | N/A | Formal: INSERT/NORMAL with CommandState subtypes | 缺失 |
| Toggle | N/A | `/vim` slash command, persisted to global config | 缺失 |
| Persistent state | N/A | `lastChange` (dot-repeat), `lastFind`, `register` | 缺失 |

**Prerequisite**: Requires TUI layer.

---

## 4. Voice Input

| Aspect | Go | Upstream | Gap |
|--------|-----|----------|-----|
| Voice mode | None | Full: push-to-talk, streaming STT via `voice_stream` endpoint | 缺失 |
| Audio capture | N/A | Native via `audio-capture-napi` (CoreAudio/ALSA) | 缺失 |
| Auth requirement | N/A | Requires OAuth (not API keys) | N/A |
| GrowthBook gate | N/A | `tengu_amber_quartz_disabled` kill-switch | N/A |

**Prerequisite**: Requires OAuth + TUI.

---

## 5. Input Handling — Line Editing

| Aspect | Go | Upstream | Gap |
|--------|-----|----------|-----|
| Input reading | `bufio.NewReader(os.Stdin).ReadString('\n')` — raw line read with no editing (`main.go:252-254`) | `useTextInput()` hook with `Cursor` class — grapheme-aware cursor with line/offset tracking (`useTextInput.ts:74-530`) | 缺失 |
| Cursor abstraction | **None** — no cursor concept | `Cursor.fromText(value, columns, offset)` with grapheme segmentation, wrapped-line positioning, viewport scrolling (`Cursor.ts:1+`) | 缺失 |
| Line editing | **None** — user must use terminal's native line editing | Full readline: Ctrl+A/E, Ctrl+B/F, Ctrl+K (kill-to-end), Ctrl+U (kill-to-start), Ctrl+W (kill-word-back), Ctrl+H (delete-token-before), Meta+B/F (prev/next word), Meta+D (delete-word-after), Ctrl+Y (yank), Meta+Y (yank-pop) (`useTextInput.ts:225-246`) | 缺失 |
| Kill ring | **None** | 10-entry kill ring with `pushToKillRing(text, 'prepend'\|'append')`, consecutive kill accumulation, `yankPop()` cycling (`Cursor.ts:15-72`) | 缺失 |
| Double-press detection | **None** — single Ctrl+C immediately cancels input | `useDoublePress()` for Ctrl+C (clear input on 2nd press), Esc (clear input on 2nd press with notification hint), Ctrl+D on empty input (exit on 2nd press) (`useTextInput.ts:109-169`) | 缺失 |
| Multiline input | **None** — single line only | Backslash+Enter for explicit newline, Shift+Enter / Meta+Enter for newline, Apple Terminal modifier detection (`useTextInput.ts:250-266`) | 缺失 |
| Image paste | **None** | `onImagePaste` callback with base64, mediaType, filename, dimensions, sourcePath (`useTextInput.ts:57-63`) | 缺失 |
| Ghost text / typeahead | **None** | `inlineGhostText` with `insertPosition` + `dim` function for inline suggestion (`useTextInput.ts:507-510`) | 缺失 |
| SSH-coalesced Enter handling | **None** | Detects "o\r" coalesced input from slow SSH links; strips trailing \r and submits (`useTextInput.ts:491-500`) | 缺失 |
| DEL character filtering | **None** | Filters `\x7f` DEL chars that interfere with backspace in SSH/tmux (`useTextInput.ts:443-466`) | 缺失 |
| Input buffer for slow terminals | **None** | `useInputBuffer()` — debounces rapid keystrokes on slow terminals (`PromptInput.tsx:61`) | 缺失 |
| History search | **None** — only up/down arrow with `useArrowKeyHistory` | `useHistorySearch()` with search box, `useTypeahead()` with typeahead dropdown (`PromptInput.tsx:59,65`) | 缺失 |
| Keybinding system | **None** | `useKeybindings()` / `useKeybinding()` with `KeybindingProviderSetup`, `CommandKeybindingHandlers`, `defaultBindings.ts` (`PromptInput.tsx:67-72`) | 缺失 |

**Gap**: Go's REPL uses raw `bufio.Reader` with no line editing, no kill ring, no keybinding system, no multiline input, and no image paste.

[diff_upstream/26-tui-ui.md §2.1]

---

## 6. Notification / Toast Queue System

| Aspect | Go | Upstream | Gap |
|--------|-----|----------|-----|
| Notification queue | **None** — `fmt.Fprintf(os.Stderr, ...)` for all output (`context.go:686-715`) | Priority queue with 4 levels: `immediate`, `high`, `medium`, `low` (`notifications.tsx:6`) | 缺失 |
| Current + queue dual slot | **None** | `current: Notification \| null` + `queue: Notification[]` dual-slot model (`notifications.tsx:56-60`) | 缺失 |
| Auto-timeout with cleanup | **None** — messages printed to stderr persist in terminal scrollback | `setTimeout` with `DEFAULT_TIMEOUT_MS = 8000` per notification; `currentTimeoutId` tracked for cleanup (`notifications.tsx:41-44`) | 缺失 |
| Immediate priority | **None** | `immediate` priority preempts current notification, clears existing timeout, re-queues displaced current notification (`notifications.tsx:98-151`) | 缺失 |
| Fold / merge | **None** | `fold?: (accumulator, incoming) → Notification` — notifications with same key merge like `Array.reduce()`. Applied to both current and queued (`notifications.tsx:22-24, 157-217`) | 缺失 |
| Invalidation cascades | **None** | `invalidates?: string[]` — new notification can remove queued/displayed notifications by key (`notifications.tsx:15`) | 缺失 |
| Deduplication | **None** | Prevents duplicate keys in queue and current slot (`notifications.tsx:220-224`) | 缺失 |
| React hook integration | **None** | `useNotifications()` returns `addNotification` / `removeNotification` callbacks (`notifications.tsx:46-48`) | 缺失 |
| Text + JSX notifications | **None** | `TextNotification` (string) + `JSXNotification` (React.ReactNode) union type (`notifications.tsx:27-34`) | 缺失 |
| Esc-to-clear hint | **None** — double-press Esc not handled | `addNotification({ key: 'escape-again-to-clear', text: 'Esc again to clear', priority: 'immediate', timeoutMs: 1000 })` (`useTextInput.ts:131-137`) | 缺失 |

**Gap**: Go has no notification/toast system. Every status message is a permanent `fmt.Fprintf` to stderr. Upstream's queue provides transient, priority-ordered, deduplicated, mergeable notifications that auto-dismiss.

[diff_upstream/26-tui-ui.md §1.1]

---

## 7. Session History & Resume

| Aspect | Go | Upstream | Gap |
|--------|-----|----------|-----|
| Session discovery | Directory listing of `.jsonl` files | Multi-project scan + cloud CCR API + interactive picker | 缺失 |
| Prompt history | None | Global `~/.claude/history.jsonl` with paste store | 缺失 |
| Session picker | Numbered list | Interactive picker with search, filtering, metadata | 缺失 |
| Fork session | `continueTranscript=false` creates new file | `--fork-session` with proper chain fork | 缺失 |
| Time-travel resume | Not supported | `--resume-session-at <uuid>` for message-level resume | 缺失 |
| Rewind files | Not supported | `--rewind-files` restores files to pre-session state | 缺失 |
| Cloud sessions | None | CCR API: `discoverAssistantSessions()` | 缺失 |
| Resume pipeline | Restores context + skill state | Restores 15+ fields: context, skills, file history, attribution, content replacements, context collapse, worktree, agent metadata | 缺失 |

**Action**: Add prompt history persistence. Add `--fork-session`. Enrich resume pipeline.

---

## 8. Conversation Branching — `/branch` and `/fork` Commands

### 8.1 `/branch` — Full Session Forking

| Aspect | Go | Upstream | Gap |
|--------|-----|----------|-----|
| Conversation branching | **None** — sessions are linear | `/branch [title]` creates a new session with full transcript copy (`branch.ts:222-296`) | 缺失 |
| Session ID generation | **None** | `randomUUID()` for fork session ID (`branch.ts:68`) | 缺失 |
| Transcript file copy | **None** — Go uses flat JSONL with no branching | Reads current JSONL, rewrites with new `sessionId`, preserved `parentUuid` chain, and `forkedFrom: { sessionId, messageUuid }` traceability (`branch.ts:88-146`) | 缺失 |
| Content-replacement preservation | **None** | Copies `content-replacement` entries with fork's sessionId to prevent cache miss on resume (`branch.ts:99-112, 150-158`) | 缺失 |
| Title management | **None** | `deriveFirstPrompt()` from first user message; `getUniqueForkName()` with collision-safe " (Branch N)" suffix (`branch.ts:38-54, 179-220`) | 缺失 |
| Title collision resolution | **None** | Regex-based enumeration: "name (Branch)" → "name (Branch 2)" → etc. with `usedNumbers` set (`branch.ts:192-219`) | 缺失 |
| Session resume into branch | **None** | `context.resume(sessionId, forkLog, 'fork')` switches REPL into the branched session with full state transfer (`branch.ts:279-281`) | 缺失 |
| Resume hint | **None** | `To resume the original: claude -r ${originalSessionId}` displayed after branching (`branch.ts:276`) | 缺失 |
| Analytics tracking | **None** | `logEvent('tengu_conversation_forked', { message_count, has_custom_title })` (`branch.ts:254-257`) | 缺失 |

### 8.2 `/fork` — Live Subagent Forking

| Aspect | Go | Upstream | Gap |
|--------|-----|----------|-----|
| Fork subagent command | **None** — Go has `AgentTool.SpawnFunc` for sub-agents but no `/fork` command | `/fork <directive>` — JSX command with `FEATURE_FORK_SUBAGENT` feature flag gate (`fork.tsx:13-16`) | 缺失 |
| Recursive fork guard | **None** | `isInForkChild(context.messages)` prevents fork-within-fork (`fork.tsx:20-23`) | 缺失 |
| AgentTool delegation | Go's `AgentTool` spawns directly via `SpawnFunc` | `/fork` delegates to `AgentTool.call()` with `run_in_background: true` and omitted `subagent_type` for implicit fork (`fork.tsx:47-48, 55-59`) | Go适配 |

**Gap**: Go has no conversation branching. Upstream's `/branch` creates a full fork of the conversation with transcript copy, title management, collision resolution, content-replacement preservation, and session resume.

[diff_upstream/26-tui-ui.md §4.1, §4.2]

---

## 9. Subagent Context Isolation

| Aspect | Go | Upstream | Gap |
|--------|-----|----------|-----|
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

**Gap**: Go's `RunForkedAgent` shares parent state freely — only `CanUseToolFn` provides permission isolation. Upstream's `createSubagentContext` has 25+ isolation dimensions with explicit opt-in sharing flags.

[diff_upstream/26-tui-ui.md §5.1]

---

## 10. Chrome Extension & Computer Use

| Aspect | Go | Upstream | Gap |
|--------|-----|----------|-----|
| Computer-use MCP | None | Full MCP server with screen, mouse, keyboard control | 缺失 |
| Chrome bridge | None | `createLinkedTransportPair` in-process transport | 缺失 |
| Key blocklist | None | Blocks dangerous key combinations | 缺失 |
| Denied apps | None | Blocks automation of system-critical apps | 缺失 |
| Sentinel apps | None | Monitors security-sensitive apps | 缺失 |
| Coordinate modes | None | `pixels` (absolute) and `normalized_0_100` (percentage) | 缺失 |
| Teach mode | None | `request_teach_access` + `teach_step` for guided automation | 缺失 |

**Prerequisite**: Requires TUI + MCP expansion. Not applicable for headless CLI.

---

## 11. Daemon Mode

| Aspect | Go | Upstream | Gap |
|--------|-----|----------|-----|
| Daemon process | None | Supervisor managing long-running workers | 缺失 |
| Worker types | None | `remoteControl` worker for headless bridge sessions | 缺失 |
| Crash recovery | None | Exponential backoff: 2s initial, 120s cap, 2x multiplier | 缺失 |
| Rapid failure parking | None | Parks after 5 failures in <10s | 缺失 |
| REPL commands | None | `/daemon status/start/stop/bg/attach/logs/kill` | 缺失 |

**Impact**: Niche feature for headless/remote operation. Lower priority for CLI users.

---

## 12. Proactive Features

| Aspect | Go | Upstream | Gap |
|--------|-----|----------|-----|
| Proactive mode | None | Tick-driven autonomous agent with state machine (inactive→active→paused) | 缺失 |
| Tick scheduling | None | `shouldTick()`: active && !paused && !contextBlocked | 缺失 |
| Context blocking | None | `setContextBlocked()` prevents tick→error→tick runaway | 缺失 |

**Impact**: Enables autonomous agent behavior. Lower priority for interactive CLI.

---

## 13. TUI Rendering — Tool Result and Agent Progress

### 13.1 Agent Progress Display

| Aspect | Go | Upstream | Gap |
|--------|-----|----------|-----|
| Agent progress type | `subAgentProgressWriter` struct with `description`, `toolCount`, `totalTokens`, `lastToolName` (`agent_progress.go:19-27`) | `AgentProgressLine` React component with 16 props: `agentType`, `description`, `name`, `descriptionColor`, `taskDescription`, `toolUseCount`, `tokens`, `color`, `isLast`, `isResolved`, `isError`, `isAsync`, `shouldAnimate`, `lastToolInfo`, `hideType` (`AgentProgressLine.tsx:6-22`) | 简化 |
| Tree-style rendering | Flat ANSI: `"\r\x1b[K[agent: desc] Running tool · N tool uses · M tokens"` (`agent_progress.go:117-139`) | Tree chars: `├─` / `└─` with nested `⎿` status line (`AgentProgressLine.tsx:41,98-100`) | 简化 |
| Resolved state | **None** — always shows "Running" | `isResolved` toggles between "Running {tool}" → "Done" or "Running in the background" (`AgentProgressLine.tsx:45-53`) | 缺失 |
| Error state | **None** | `isError` prop for error rendering (`AgentProgressLine.tsx:19`) | 缺失 |
| Async/background tracking | **None** | `isAsync && isResolved` → "Running in the background" status, hides tool count/tokens (`AgentProgressLine.tsx:42,49-51,88-94`) | 缺失 |
| Color-coded agent type | **None** — plain text | `backgroundColor={color}` badge for agent type label with `inverseText` color (`AgentProgressLine.tsx:69-73`) | 缺失 |
| Animation control | **None** | `shouldAnimate` prop for spinner/animation state (`AgentProgressLine.tsx:19`) | 缺失 |
| Token formatting | Raw integer: `fmt.Sprintf("%d tokens", w.totalTokens)` (`agent_progress.go:134`) | `formatNumber(tokens)` with locale-aware formatting (`AgentProgressLine.tsx:92`) | 简化 |

### 13.2 Tool Result Rendering Architecture

| Aspect | Go | Upstream | Gap |
|--------|-----|----------|-----|
| Tool result routing | Single path: `stripAnsi()` + regex extraction of tool names from `[+]`/`[ERR]` lines (`agent_progress.go:76-103`) | 4-way dispatch: `UserToolCanceledMessage`, `UserToolRejectMessage`, `UserToolErrorMessage`, `UserToolSuccessMessage` based on `param.content` prefix and `param.is_error` (`UserToolResultMessage.tsx:48-100`) | 简化 |
| Message type rendering | Flat text output via `fmt.Fprintf` | 40+ specialized React components in `components/messages/`: `AdvisorMessage`, `AssistantThinkingMessage`, `CompactBoundaryMessage`, `CollapsedReadSearchContent`, `GroupedToolUseContent`, `SnipBoundaryMessage`, `UserBashOutputMessage`, `UserForkBoilerplateMessage`, `PlanApprovalMessage`, etc. | 缺失 |
| Thinking block rendering | `ThinkFilterState` wraps in ANSI dim (`streaming.go:300-397`) | `AssistantThinkingMessage` with expandable/collapsible UI, `AssistantRedactedThinkingMessage` for `redacted_thinking` blocks, `lastThinkingBlockId` for transcript-mode hiding (`Message.tsx:445-465`) | 简化 |
| Collapsed read/search grouping | **None** — all tool results displayed individually | `CollapsedReadSearchContent` — groups consecutive Read/Grep/Glob tool uses into collapsible UI with `OffscreenFreeze` for scrollback optimization (`Message.tsx:258-280`) | 缺失 |
| Grouped tool use rendering | **None** | `GroupedToolUseContent` — groups parallel tool calls into single expandable UI (`Message.tsx:247-255`) | 缺失 |
| Image rendering | **None** | `UserImageMessage` with `imageId` reference and `ClickableImageRef` (`Message.tsx:328-334`) | 缺失 |
| Attachment rendering | Inline text with `[history-snip]` prefix (`context.go:969`) | `AttachmentMessage` with rich rendering for file attachments, memory attachments (`Message.tsx:104-112`) | 简化 |

**Gap**: Go's rendering is a flat text stream with ANSI escape codes. Upstream's rendering is a full React component tree with 40+ specialized message components, each handling specific content types with rich interactive UI (expand/collapse, image display, grouped tool uses, thinking block visibility toggles).

[diff_upstream/26-tui-ui.md §3.1, §3.2]

---

## 14. Status Line and Cost Display

| Aspect | Go | Upstream | Gap |
|--------|-----|----------|-----|
| Status line | **None** — no persistent status bar | `StatusLine` React component with configurable display (`StatusLine.tsx:59-64`) | 缺失 |
| Model display | **None** | Model name with `renderModelName()` and context window percentage (`StatusLine.tsx:50-56`) | 缺失 |
| Permission mode | **None** | Permission mode badge in status line (`StatusLine.tsx:66-73`) | 缺失 |
| Cost display | Raw token counts via `fmt.Fprintf` after API calls | `getTotalCost()` from cost-tracker, `formatCost()` with smart decimal places (`StatusLine.tsx:24-25`) | 缺失 |
| Context usage | **None** | `calculateContextPercentages()` shows used/total context window percentage (`StatusLine.tsx:37`) | 缺失 |
| Duration tracking | **None** | `getTotalAPIDuration()`, `getTotalDuration()` — API vs wall time (`StatusLine.tsx:23`) | 缺失 |
| Lines changed | **None** | `getTotalLinesAdded()`, `getTotalLinesRemoved()` — code change tracking (`StatusLine.tsx:26`) | 缺失 |
| Vim mode indicator | **None** | Vim mode in status line for vim input users (`StatusLine.tsx:57`) | 缺失 |
| Custom status line command | **None** | `executeStatusLineCommand()` — user-configurable status line via hooks (`StatusLine.tsx:44`) | 缺失 |

**Gap**: Go has no persistent status line. Users cannot see model name, cost, context usage, or duration without scrolling through output.

[diff_upstream/26-tui-ui.md §6.1]

---

## 15. REPL Component Deep Dive

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

[diff_upstream/26-tui-ui.md §53.1]

---

## 16. Tool Use Display

**Upstream**: `src/components/messages/AssistantToolUseMessage.tsx`
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

[diff_upstream/26-tui-ui.md §53.2]

---

## 17. Thinking Display

**Upstream**: `src/components/messages/AssistantThinkingMessage.tsx`
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

[diff_upstream/26-tui-ui.md §53.3]

---

## 18. Permission Prompts

**Upstream**: `src/components/permissions/PermissionPrompt.tsx`
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

[diff_upstream/26-tui-ui.md §53.4]

---

## 19. Status Display Deep Dive

**Upstream**: `src/components/StatusLine.tsx` + `src/components/BuiltinStatusLine.tsx`
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

[diff_upstream/26-tui-ui.md §53.5]

---

## 20. Command System

**Upstream**: `src/commands.ts` + `src/components/PromptInput/PromptInput.tsx`
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

[diff_upstream/26-tui-ui.md §53.6]

---

## 21. Message Display

**Upstream**: `src/components/VirtualMessageList.tsx` + `src/components/Messages.tsx`
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

[diff_upstream/26-tui-ui.md §53.7]

---

## 22. Diff Display

**Upstream**: `src/components/StructuredDiff.tsx` + `src/components/diff/`
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

[diff_upstream/26-tui-ui.md §53.8]

---

## 23. Error Display

**Upstream**: `src/components/messages/SystemAPIErrorMessage.tsx`
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

[diff_upstream/26-tui-ui.md §53.9]

---

## 24. Compact Boundary

**Upstream**: `src/components/messages/CompactBoundaryMessage.tsx`
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

[diff_upstream/26-tui-ui.md §53.10]

---

## 25. Print/Output System

| # | Aspect | Go | Upstream (`cli/print.ts`) | Type |
|---|--------|----|--------------------------|------|
| 1 | Print system architecture | Direct fmt.Print/Fprintf to stdout/stderr | `cli/print.ts` (~4000 lines) — Full print pipeline with message rendering, streaming, tool results | 缺失 |
| 2 | Tool result formatting | Plain text output with truncation | `printToolResult()` — Rich result rendering with syntax highlighting, diff display, image preview | 缺失 |
| 3 | Streaming text display | Sequential character output | Incremental rendering with virtual scroll, line wrapping, ANSI color support | 缺失 |
| 4 | Usage/cost display | Basic token counts in XML (`agent_loop.go:237`) | `formatTotalCost()` — Formatted cost display with per-model breakdown, API duration, wall duration | 缺失 |
| 5 | Session resume display | Simple "Resuming session..." text | Full session state restoration display with compact boundary, recovered files | 缺失 |

[diff_upstream/26-tui-ui.md §47.2]

---

## Cross-References

- Session management: [07-architecture.md](07-architecture.md) §6
- MCP integration: [05-services.md](05-services.md) §5
- Keybindings in context: diff_upstream.md:§n (lines 8179-8203)
- Upstream services deep dive: [05-services.md](05-services.md) §18
