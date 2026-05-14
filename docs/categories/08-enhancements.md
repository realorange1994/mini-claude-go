# 08 — Go-Specific Enhancements & Adaptations

> Features Go has that upstream lacks, or implements differently due to Go idioms

## Overview

Go has several genuine enhancements over upstream and a number of adaptations that differ due to Go's language characteristics. These are worth preserving and potentially contributing back.

---

## 1. Context Reference System (@-expansion)

| Feature | Go | Upstream |
|---------|-----|----------|
| `@diff` | Fully supported with color-coded hunk display | Not supported |
| `@staged` | Fully supported with staged changes display | Not supported |
| `@git:N` | Fully supported — diff from N commits ago | Not supported |
| `@url` | Fully supported — fetches and injects URL content | Not supported |
| `@file` | Supported (basic) | Supported (basic) |
| `@agent` | Not supported | `@agent-name` mention for sub-agent routing |
| `@mcp:resource` | Not supported | MCP resource URI mention |
| Reference budget | Hard/soft token limits with blocking | No reference budget |

**Go strength**: The `@diff`, `@staged`, `@git:N`, and `@url` references are unique to Go. Upstream has no equivalent. The reference budget system (hard/soft token limits) is also Go-specific.

---

## 2. SkillTracker

| Feature | Go | Upstream |
|---------|-----|----------|
| Discovery model | Dedicated `SkillTracker` with `shown/read/used` states | No dedicated tracker — skills discovered via file watching |
| State tracking | Progressive: `pending` → `shown` → `read` → `used` | None — skills loaded/unloaded only |
| Display control | `pendingCount` for badge/notification | No equivalent |
| Skill reset | `ResetPostCompact()` clears tracker after compaction | No dedicated reset |

**Go strength**: The SkillTracker provides a richer UX for skill discovery. Worth preserving.

---

## 3. GitTool (35+ Operations)

| Feature | Go | Upstream |
|---------|-----|----------|
| Dedicated tool | Yes — `GitTool` with 35+ structured operations | No — uses BashTool for all git commands |
| Operation enum | 35+ operations with typed params | N/A |
| Safety | Per-subcommand flag whitelists + dangerous operation blocking | BashTool safety (path sandboxing, command allowlists) |
| `gh` integration | Built-in with read-only whitelist | Via BashTool |
| Composite info | `info` operation: root, branch, commit, dirty status | No equivalent |

**Go strength**: GitTool is a major enhancement — structured, safe, and composable. Upstream's raw `git` via BashTool is error-prone.

---

## 4. File History

| Feature | Go | Upstream |
|---------|-----|----------|
| Per-file restore | 4 methods (RewindTo, RestoreLast, RewindSteps, Checkout) | No — all-or-nothing state restoration |
| Redo support | Auto-snapshot before restore | No built-in redo |
| Tags | Full tagging system | None |
| Annotations | Yes | None |
| Cross-file timeline | `GetTimeline()` across all files | None |
| Pattern search | `Search(filePath, pattern, mode)` for version history | Line-count diff stats only |
| Binary support | No (JSON inline text only) | Yes (file copies preserve permissions) |
| Session resume | No backup copy between sessions | `copyFileHistoryForResume()` with hard links |

**Go strength**: Richer metadata and per-file restore. Upstream is simpler but more robust for binary and session resume.

---

## 5. Stall Detection & Recovery

| Feature | Go | Upstream |
|---------|-----|----------|
| Stall detection | Always-on with 240-300s dynamic timeout | Env-gated watchdog (90s) |
| Dynamic timeouts | Yes: local=300s/600s, >100K tokens=300s/360s, >50K=240s/300s | Fixed configurable timeout |
| Automatic recovery | Progressive: TruncateHistory → AggressiveTruncate → MinimumHistory | Abort only, no auto-recovery in stream layer |
| Warning phase | None | Half-timeout warning before abort |

**Go strength**: More aggressive stall detection with automatic recovery. This is a significant reliability advantage.

---

## 6. Rate Limit State Persistence

| Feature | Go | Upstream |
|---------|-----|----------|
| State sharing | Pointer-based persistent state shared across API calls | Per-request only (reads headers each time) |
| Proactive delay | `calculateProactiveDelay()` with sliding window | No proactive delay |
| Overage tracking | Limited | `allowed`/`allowed_warning`/`rejected` status tracking |

**Go strength**: Persistent rate limit state enables proactive throttling. Upstream only reacts to headers.

---

## 7. Chinese Error Patterns

| Feature | Go | Upstream |
|---------|-----|----------|
| Chinese error messages | Yes — `超过最大长度`, `上下文长度`, etc. | English only |
| Chinese provider support | Pattern matching for Chinese API providers | Not applicable |

**Go strength**: Critical for Chinese users who use domestic API proxies.

---

## 8. Multi-Edit

| Feature | Go | Upstream |
|---------|-----|----------|
| Multi-edit API | Yes — `edits: array<{old_string, new_string, replace_all}>` | No — single edit per call |
| Atomic validation | Validates all edits in-memory before writing | Same |
| Go version | `multi_edit.go` with `MultiEditDescriptor` | `FileEditTool` (single edit only) |

**Go strength**: Multi-edit is a genuine UX improvement. Multiple edits in a single call reduces round-trips and maintains context.

---

## 9. Process Management (Platform-Specific)

| Feature | Go | Upstream |
|---------|-----|----------|
| Build tags | `unix` / `windows` build-tagged files | Runtime platform detection |
| Process groups | Unix: `syscall.SysProcAttr{Setpgid: true}`; Windows: `syscall.CREATE_NEW_PROCESS_GROUP` | Node.js `child_process` |
| Double-kill | Yes — SIGTERM then SIGKILL for reliability | SIGTERM only |
| Signal exit codes | Unix: extracts signal number from `WaitStatus` | Node.js default handling |

**Go strength**: More robust process lifecycle management due to Go's systems programming capabilities.

---

## 10. Go Adaptations (Not Enhancements)

These are differences driven by Go's language design rather than deliberate feature choices:

| Adaptation | Go Approach | Upstream Approach |
|------------|------------|-------------------|
| Concurrency | Goroutines + channels + mutexes | Async generators + Promises |
| Error handling | Multi-return `(value, error)` | Exceptions / try-catch |
| JSON handling | `json.Marshal`/`Unmarshal` with struct tags | `JSON.parse`/`stringify` |
| String processing | `strings` + `regexp` packages | Template literals + regex |
| File I/O | `os` + `bufio` packages | `fs` module |
| HTTP client | `net/http` with manual body management | `fetch` API with streams |

---

## 11. Sandbox & Security

[diff_upstream/30-go-enhancements.md §33]

### 11.1 Sandbox Execution

| Aspect | Go | Upstream TS |
|--------|-----|-------------|
| **Process sandboxing** | **NONE** — commands execute directly via `os/exec` without OS-level isolation | **macOS Seatbelt** — uses `sandbox-exec` profiles for mandatory filesystem and network restrictions |
| **Linux sandboxing** | **NONE** | **Bubblewrap (bwrap)** — seccomp-based filesystem namespace isolation |
| **WSL2 sandboxing** | **NONE** | **Bubblewrap** — same as Linux |
| **Windows sandboxing** | **NONE** | **NONE** — no sandbox on Windows |
| **Sandbox CLI flag** | **NONE** | `--sandbox` / `--no-sandbox` flags |
| **Sandbox settings** | **NONE** | `sandbox.enabled`, `failIfUnavailable`, `autoAllowBashIfSandboxed`, `allowUnsandboxedCommands`, `excludedCommands` |
| **Sandbox toggle command** | **NONE** | `/sandbox` command with enable/disable/exclude subcommands |

**CRITICAL GAP**: Go has zero OS-level process sandboxing. All commands run with full access to the host filesystem, network, and system resources.

### 11.2 Docker Containerization

| Aspect | Go | Upstream TS |
|--------|-----|-------------|
| **Docker detection** | **NONE** | Checks `/.dockerenv` |
| **Bubblewrap detection** | **NONE** | `envDynamic.getIsBubblewrapSandbox()` |
| **Internet access check** | **NONE** | `env.hasInternetAccess()` memoized — gates `--dangerously-skip-permissions` |
| **Dangerously skip permissions** | **NONE** | Allowed ONLY in Docker/sandbox with NO internet |
| **CCR container support** | **NONE** | Full container orchestration |

### 11.3 Network Access Restrictions

| Aspect | Go | Upstream TS |
|--------|-----|-------------|
| **OS-level network sandbox** | **NONE** | **macOS Seatbelt** — network domain filtering |
| **Allowed domains** | **NONE** | `sandbox.network.allowedDomains` |
| **Denied domains** | **NONE** | Extracted from `permissions.deny` rules |
| **Unix socket blocking** | **NONE** | `allowUnixSockets`, `allowAllUnixSockets` |
| **HTTP/SOCKS proxy** | **NONE** | `httpProxyPort`, `socksProxyPort` for MITM proxy support |

**CRITICAL GAP**: Go has no network sandboxing whatsoever. All commands have unrestricted network access.

### 11.4 File Path Restrictions

| Aspect | Go | Upstream TS |
|--------|-----|-------------|
| UNC path blocking | Yes — `isUncPath()` | Yes — `containsVulnerableUncPath()` |
| Tilde expansion | Yes — `expandTilde()`; blocks `~root`, `~+`, `~-` | Yes — `expandPath()` |
| Dangerous directories | `.git`, `.vscode`, `.idea`, `.claude` (identical to upstream) | Same list |
| Skill-scope permissions | **NONE** | `getClaudeSkillScope()` for narrow per-skill allow patterns |
| Sandbox filesystem config | **NONE** | `sandbox.filesystem.{allowWrite,denyWrite,denyRead,allowRead}` |

### 11.5 Binary File Protection

| Aspect | Go | Upstream TS |
|--------|-----|-------------|
| Extension-based rejection | 50+ binary extensions | `hasBinaryExtension()` |
| Magic bytes detection | **More comprehensive** — 15+ signatures (PE, ELF, GZIP, BZIP2, MP3, JPEG, PNG, GIF, ZIP, XZ, 7Z, WebP, WAV, MP4, Java, Wasm, Python pyc, Lua bytecode) | Simpler null-byte approach in `isBinaryContent()` |

**Go strength**: Magic bytes detection is actually more comprehensive than upstream's null-byte approach.

### 11.6 Symlink Following

| Aspect | Go | Upstream TS |
|--------|-----|-------------|
| Symlink resolution | Single `filepath.EvalSymlinks()` — only resolves target path | Deep symlink resolution with multi-pass containment checks |
| Symlink escape prevention | **NONE** — resolves but doesn't verify path stays within allowed directory | `validateTeamMemWritePath()` checks resolved path stays within team dir |
| Dangling symlink detection | **NONE** | `lstat` distinguishes dangling symlinks |

### 11.7 Permission Modes

| Aspect | Go | Upstream TS |
|--------|-----|-------------|
| Ask mode | `ModeAsk` — prompts for exec/write/edit | `'ask'` — interactive prompts |
| Auto mode | `ModeAuto` — LLM classifier evaluates tools | `'auto'` — YOLO classifier with allow/deny rules |
| Plan mode | `ModePlan` — blocks all write tools | `'plan'` — read-only |
| Bypass mode | `ModeBypass` — allows all | `'bypass'` — only available in sandboxed environments with no internet |
| CLI flag | `--permission-mode` not implemented | `--permission-mode <mode>` |

**Critical**: Go's bypass mode is always available. Upstream only allows bypass in verified sandboxed containers with no internet access.

### 11.8 Summary: Critical Security Gaps

| # | Gap | Severity | Description |
|---|-----|----------|-------------|
| 1 | **No OS-level sandbox** | CRITICAL | No Seatbelt, Bubblewrap, or any kernel-level isolation |
| 2 | **No network restrictions** | CRITICAL | No domain allow/deny, no Unix socket blocking, no MITM proxy |
| 3 | **No Docker/container support** | HIGH | No detection, no internet checks, no CCR integration |
| 4 | **No sandbox filesystem rules** | HIGH | Permission rules at application level only, no OS-enforced mount rules |
| 5 | **No symlink containment** | HIGH | Resolves but doesn't verify path stays within allowed directories |
| 6 | **No environment isolation** | MEDIUM | Full parent process environment inherited |
| 7 | **Bypass mode always available** | MEDIUM | No sandbox verification required |
| 8 | **No sandbox settings system** | MEDIUM | No `sandbox.enabled`, `failIfUnavailable`, etc. |

---

## 12. Enhanced Error Classification

[diff_upstream/30-go-enhancements.md §49.1]

| # | Aspect | Go | Upstream | Type |
|---|--------|-----|----------|------|
| 1 | 15 ErrorClass taxonomy with typed enum | `error_types.go:17-33` | ~20 flat string tags, no enum | Go增强 |
| 2 | ClassifyResult with 3 boolean recovery strategies | `error_types.go:36-45` | Error strings only, no structured recovery hints | Go增强 |
| 3 | RetryAfter duration field on classify result | `error_types.go:43` | No Retry-After parsing in classifier | Go增强 |
| 4 | 402 disambiguation: billing vs transient usage limit | `error_types.go:327-338` | 402 handled as flat `credit_balance_low` string | Go增强 |
| 5 | 400 deep classification: context overflow, model-not-found, etc. | `error_types.go:341-373` | 400 returns `invalid_request` or `tool_use_mismatch` only | Go增强 |
| 6 | Usage-limit vs rate-limit disambiguation with transient signals | `error_types.go:269-280` | No usage-limit/rate-limit disambiguation | Go增强 |
| 7 | Server disconnect + large session → context overflow detection | `error_types.go:296-302` | No server-disconnect→context-overflow heuristic | Go增强 |
| 8 | 400 + large session → probable context overflow | `error_types.go:367-369` | No heuristic for 400+large-session | Go增强 |
| 9 | Chinese-language context overflow patterns | `error_types.go:98` | English-only patterns | Go适配 |
| 10 | Transport error type heuristics (readtimeout, connecttimeout, etc.) | `error_types.go:134-141` | Only `APIConnectionTimeoutError` check | Go增强 |
| 11 | OpenRouter 403 "key limit exceeded" → billing reclassification | `error_types.go:188-193` | 403 always `auth_error` | Go增强 |

---

## 13. Work Task Dependency Graphs

[diff_upstream/30-go-enhancements.md §49.2]

| # | Aspect | Go | Upstream | Type |
|---|--------|-----|----------|------|
| 1 | Bidirectional dependency edges (Blocks/BlockedBy) | `work_task.go:32-33` | No equivalent todo/task dependency graph | Go增强 |
| 2 | `wouldCreateCycle` BFS — prevents circular dependencies | `work_task.go:242-264` | No cycle detection | Go增强 |
| 3 | `filterValidDeps` — silently removes references to non-existent tasks | `work_task.go:267-275` | No equivalent | Go增强 |
| 4 | Automatic bidirectional edge maintenance | `work_task.go:167-173` | No equivalent | Go增强 |
| 5 | Auto-cleanup: deleted task removes itself from all others' edges | `work_task.go:209-214` | No equivalent | Go增强 |
| 6 | `#` prefix stripping on dependency IDs (tolerates `#1` format from LLM) | `work_task.go:161-162` | No equivalent | Go适配 |

---

## 14. File History: Extended Tool Suite

[diff_upstream/30-go-enhancements.md §49.3]

| # | Feature | Go | Upstream | Type |
|---|---------|-----|----------|------|
| 1 | Tag system (AddTag, ListTags, RemoveTag, SearchTag) | `file_history_tools.go:757-875` | No tag/naming system | Go增强 |
| 2 | Tag-based version specifiers in ResolveVersion | `filehistory.go:488,547-558` | Version by messageId UUID only | Go增强 |
| 3 | file_history_grep: regex search across versions | `file_history_tools.go:209-325` | No grep-in-history feature | Go增强 |
| 4 | file_history_search: find versions where text was added/removed | `file_history_tools.go:638-701` | No search-by-change-type | Go增强 |
| 5 | file_history_timeline: cross-file change timeline | `file_history_tools.go:706-753` | No cross-file timeline | Go增强 |
| 6 | file_history_annotate: user annotations on versions | `file_history_tools.go:878-921` | No annotation feature | Go增强 |
| 7 | file_history_checkout with flexible version specifiers | `file_history_tools.go:925-966` | Rewind by messageId only | Go增强 |
| 8 | file_history_batch: batch operations on glob-matched files | `file_history_tools.go:970-1084` | No batch operations | Go增强 |
| 9 | Chain diff (from→to→to2) | `file_history_tools.go:541-570` | No chain diff | Go增强 |
| 10 | Diff stat/name-only/unified modes | `file_history_tools.go:497-539` | diffLines for insertions/deletions only | Go增强 |
| 11 | Version merging: consecutive snapshots with same checksum | `file_history_tools.go:59-70` | No merge display | Go增强 |
| 12 | Unix-style path handling on Windows | `file_history_tools.go:1163-1166` | Node path module handles | Go适配 |

---

## 15. Rate Limit Parsing: 3rd-Party Provider Headers

[diff_upstream/30-go-enhancements.md §49.5]

| # | Aspect | Go | Upstream | Type |
|---|--------|-----|----------|------|
| 1 | Multi-provider header parsing (Nous Portal / OpenRouter / OpenAI) | `rate_limit.go:156-229` | Only Anthropic unified headers | Go增强 |
| 2 | 4-bucket rate limit state (RPM, RPH, TPM, TPH) | `rate_limit.go:49-58` | Single unified status model | Go增强 |
| 3 | MostConstrainedBucket: highest-usage bucket for diagnostics | `rate_limit.go:75-101` | No constraint analysis | Go增强 |
| 4 | RetryDelay: safe wait time based on exhausted buckets + 10% margin | `rate_limit.go:105-129` | No computed retry delay | Go增强 |
| 5 | Thread-safe RateLimitState with `sync.RWMutex` | `rate_limit.go:50` | No concurrency control (single-threaded) | Go适配 |
| 6 | Age-aware freshness tracking | `rate_limit.go:61-71,363-371` | No age tracking | Go增强 |
| 7 | ASCII progress bar visualization | `rate_limit.go:325-335` | No progress bar | Go增强 |
| 8 | Compact one-line status bar | `rate_limit.go:420-447` | No compact display | Go增强 |
| 9 | 80% warning threshold | `rate_limit.go:394-409` | No warning thresholds | Go增强 |
| 10 | Non-HTTP header parsing context | `rate_limit.go:232-269` | Only Headers object | Go增强 |
| 11 | Retry-After header fallback | `rate_limit.go:194-207` | No Retry-After parsing | Go增强 |

---

## 16. Streaming Architecture

[diff_upstream/30-go-enhancements.md §49.6]

| # | Aspect | Go | Upstream | Type |
|---|--------|-----|----------|------|
| 1 | StreamAdapter: standalone SSE-to-StreamChunk bridge | `streaming.go:707-1005` | Streaming embedded in query() generator | Go增强 |
| 2 | CollectHandler: thread-safe chunk accumulator with mutex | `streaming.go:59-298` | Mutable arrays (single-threaded) | Go适配 |
| 3 | TerminalHandler: clean terminal display with thinking filter state machine | `streaming.go:317-562` | No standalone terminal handler | Go增强 |
| 4 | ThinkFilterState: 4-state machine for `<thinking>` tag filtering | `streaming.go:301-397` | No think-tag terminal filter | Go增强 |
| 5 | StreamBus: pub/sub for StreamChunk events with buffered channels | `streaming.go:568-621` | No equivalent pub/sub | Go增强 |
| 6 | StreamProgress: TTFB + throughput tracking | `streaming.go:625-681` | queryProfiler tracks latency only | Go增强 |
| 7 | DeltasState: tracks what was streamed for retry safety | `streaming.go:688-1005` | No explicit deltas-state tracking | Go增强 |
| 8 | Tool-use-as-text detection | `streaming.go:137-146` | No equivalent detection | Go增强 |
| 9 | ClearPartialToolCall: removes incomplete tool call before retry | `streaming.go:226-232` | Different: streamingToolExecutor.discard() | Go适配 |
| 10 | HasTruncatedToolArgs: validates JSON args for truncation | `streaming.go:255-267` | No JSON validation for truncation | Go增强 |
| 11 | Dynamic stall timeout based on provider / context size | `streaming.go:733-748` | No stall detection at stream level | Go增强 |
| 12 | FinishReason tracking on CollectHandler | `streaming.go:86-92,198-210` | Tracked in MessageDeltaEvent but not on collector | Go增强 |
| 13 | toolArgSummary: per-tool compact terminal summary for 15+ tool types | `streaming.go:482-562` | No equivalent terminal summary | Go增强 |
| 14 | Stall detector with goroutine + timer reset pattern | `streaming.go:786-823` | No stall detection | Go增强 |
| 15 | AsParsedResponse: bypasses SDK for non-Claude models | `streaming.go:271-298` | SDK-specific, no bypass needed | Go适配 |

---

## 17. Retry Utilities: Jittered Exponential Backoff

[diff_upstream/30-go-enhancements.md §49.7]

| # | Aspect | Go | Upstream | Type |
|---|--------|-----|----------|------|
| 1 | `jitteredBackoff`: standalone utility with functional options pattern | `retry_utils.go:21-46` | No standalone backoff — embedded in `withRetry` | Go增强 |
| 2 | Thundering-herd prevention: random jitter | `retry_utils.go:12-13` | No jitter in upstream `withRetry` | Go增强 |
| 3 | Configurable base/max/ratio via functional options | `retry_utils.go:57-69` | Hardcoded retry counts | Go增强 |
| 4 | 63-bit exponent overflow guard | `retry_utils.go:36-38` | No overflow guard | Go增强 |
| 5 | Formula: `min(base × 2^(attempt-1), maxDelay) + uniform(0, jitterRatio × delay)` | `retry_utils.go:20` | Simple retry count with no delay formula | Go增强 |

---

## 18. Transcript Builder: isAskUserApproval Detection

[diff_upstream/30-go-enhancements.md §49.9]

| # | Aspect | Go | Upstream | Type |
|---|--------|-----|----------|------|
| 1 | isAskUserApproval: detects explicit user consent | `transcript_builder.go:150-175` | No equivalent approval detection | Go增强 |
| 2 | 16 affirmative keyword matching | `transcript_builder.go:163-168` | No consent keyword list | Go增强 |
| 3 | Q:/A: format parsing for structured question-answer extraction | `transcript_builder.go:152-161` | No Q/A parsing | Go增强 |
| 4 | "USER EXPLICITLY APPROVED" tag in compact transcript | `transcript_builder.go:59` | No approval tagging | Go增强 |
| 5 | Security: skip assistant text to prevent agent from influencing classifier | `transcript_builder.go:41-42` | No equivalent security constraint | Go增强 |

---

## 19. Concurrency Adaptations

[diff_upstream/30-go-enhancements.md §49.11]

| # | Aspect | Go | Upstream | Type |
|---|--------|-----|----------|------|
| 1 | `sync.RWMutex` on WorkTaskStore | `work_task.go:41` | React state (single-threaded) | Go适配 |
| 2 | `sync.RWMutex` on RateLimitState | `rate_limit.go:50` | Single-threaded JS | Go适配 |
| 3 | `sync.Mutex` on CollectHandler | `streaming.go:61` | Single-threaded JS | Go适配 |
| 4 | `atomic.Bool` for interrupted flag | `agent_loop.go:293` | AbortController.signal | Go适配 |
| 5 | `atomic.Int32` for IterationBudget consumed counter | `agent_loop.go:34` | Simple counter in closure | Go适配 |
| 6 | `atomic.Bool` for IterationBudget graceCalled | `agent_loop.go:35` | No equivalent | Go适配 |
| 7 | CompareAndSwap for budget consume/refund | `agent_loop.go:44-67` | Simple increment | Go适配 |
| 8 | `sync.Mutex` on StreamProgress | `streaming.go:637` | Single-threaded | Go适配 |

---

## 20. Windows Path Handling & Pipe Input (R23)

### 20.1 Unified Path Handling: PosixToWindowsPath

On Windows with Git Bash, file tools and exec tool must resolve the same physical file despite using different path formats. Go now matches upstream's approach:

| Feature | Go | Upstream |
|---------|-----|----------|
| POSIX→Windows conversion | `PosixToWindowsPath()` in `exec_tool.go` | `posixPathToWindowsPath()` in `windowsPaths.ts` |
| MSYS2 mount: `/tmp/` | Maps to `os.TempDir()` | Same mapping |
| MSYS2 mount: `/home/` | Maps to `os.UserHomeDir()` (skips username segment) | Same mapping |
| Cygwin drive: `/cygdrive/x/` | Maps to `X:\` | Same mapping |
| Drive letter: `/x/` | Maps to `X:\` | Same mapping |
| UNC: `//server/share` | Maps to `\\server\share` | Same mapping |
| `expandPath()` integration | Uses `PosixToWindowsPath()` on Windows for all file tools | Uses `posixPathToWindowsPath()` in `expandPath()` |
| Path format guidance | `GetPathFormatInfo()` injected into system prompt | Path format guidance in system prompt |
| Git Bash detection | `findGitBashForWindows()` with memoize pattern | `Shell.ts` detection |

**Result**: `file_write /tmp/test.txt` and `exec cat /tmp/test.txt` now resolve to the same physical file on Windows.

### 20.2 Pipe Input Fix

When stdin is not a terminal (pipe mode), `ReadString('\n')` splits multi-line input into separate `agent.Run()` calls, causing the first (often empty) line to be sent as a prompt with no task context.

| Aspect | Before (broken) | After (fixed) |
|--------|-----------------|---------------|
| Pipe input reading | `ReadString('\n')` per line | `io.ReadAll(stdinReader)` — entire input as single prompt |
| Multi-line prompts | Split into separate incomplete calls | Single `agent.Run()` with full prompt |
| Exit code 124 | Common on multi-line pipe input | Eliminated |

---

## 21. Go-Only Features Not in Upstream

[diff_upstream/30-go-enhancements.md §G]

Complete list of Go-original features that could benefit the upstream TypeScript implementation:

| # | Feature | Go Source | Upstream Could Benefit? |
|---|---------|-----------|------------------------|
| 1 | **Grace call mechanism** | `agent_loop.go` | **Yes** — prevents abrupt session termination after budget exhaustion |
| 2 | **Conclusion extraction** | `agent_loop.go` | **Yes** — structured session end |
| 3 | **Preflight compression on resume** | `agent_loop.go` | **Yes** — proactive vs. reactive overflow handling |
| 4 | **Grace eviction ticker** | `agent_loop.go` | **Yes** — resource management |
| 5 | **Agent management tools** (agent_list/get/kill) | `tools/agent_tool.go` | **Yes** — model can observe and control its own agents |
| 6 | **Plan mode tools** (EnterPlanMode/ExitPlanMode) | `tools/` | **Maybe** — upstream has plan mode in UI but not as tools |
| 7 | **Permission pre-check** | `permissions.go` | **Maybe** — upstream uses React UI for permission prompts |
| 8 | **13 file history tools** | `file_history_tools.go` | **Yes** — upstream only has TUI-based history |
| 9 | **File history cross-file timeline** | `filehistory.go` | **Yes** — unique visualization capability |
| 10 | **FileOps tool** | `tools/fileops_tool.go` | **Yes** — upstream uses Bash for simple file ops |
| 11 | **ListDir tool** | `tools/listdir_tool.go` | **Yes** — simpler than Bash `ls` or Glob |
| 12 | **Exa search tool** | `tools/exa_search_tool.go` | **Maybe** — upstream uses different search providers |
| 13 | **Process tool** | `tools/process_tool.go` | **Yes** — no upstream equivalent for process introspection |
| 14 | **Runtime info tool** | `tools/runtime_info_tool.go` | **Maybe** — useful for debugging |
| 15 | **Tool search tool** | `tools/tool_search_tool.go` | **Yes** — helps model discover tools in large registries |
| 16 | **MultiEdit tool** | `tools/multi_edit_tool.go` | **Yes** — more atomic than repeated single edits |
| 17 | **Inter-agent messaging** (SendMessageTool) | `tools/` | **Yes** — upstream's Agent tool lacks explicit messaging |
| 18 | **TodoWrite with dependencies** | `tools/todo_tool.go` | **Yes** — upstream's todo is flat list |
| 19 | **15-category error taxonomy with action flags** | `error_types.go` | **Maybe** — upstream has more categories but no action flags |
| 20 | **Billing vs rate-limit disambiguation** | `error_types.go` | **Yes** — upstream has less granular 402 handling |
| 21 | **100+ error substring patterns (incl. Chinese)** | `error_types.go` | **Maybe** — upstream is English-only in error matching |
| 22 | **Work task dependency graph** | `work_task.go` | **Yes** — upstream has no task dependency model |
| 23 | **LLM-autonomous skill discovery** | `skills/tracker.go` | **Yes** — upstream requires pre-loaded skills |
| 24 | **Fork mode with structured protocol** | `agent_sub.go` | **Maybe** — upstream uses different sub-agent model |
| 25 | **4-layer tool filtering** | `agent_sub.go` | **Maybe** — upstream uses permission rules |
| 26 | **Model alias resolution with parent-tier matching** | `agent_sub.go` | **Maybe** — upstream has alias system but different logic |
| 27 | **GitTool with 35+ structured operations** | `tools/git_tool.go` | **Yes** — safer and more model-friendly than raw shell |
| 28 | **Magic bytes binary detection (15+ signatures)** | `file_read.go` | **Yes** — upstream uses simpler null-byte approach |
| 29 | **AllowedCommands / DeniedPatterns config** | `config.go` | **Maybe** — upstream uses sandbox rules instead |
| 30 | **Background flush loop for session memory** | `session_memory.go` | **Maybe** — upstream has different persistence model |

---

## 21. Top 10 Go Enhancements Summary

[diff_upstream/30-go-enhancements.md §C]

| # | Enhancement | Why It's Better |
|---|------------|-----------------|
| 1 | **15-category error taxonomy with action flags** | Upstream has 25+ string labels for analytics only; Go's `ClassifyResult` carries actionable flags directly |
| 2 | **13 LLM-callable file history tools** | Upstream exposes file history only through the TUI; Go exposes as LLM-callable tools |
| 3 | **Dedicated GitTool with 35+ structured operations** | Upstream uses raw BashTool; Go gives type-safe params + per-operation validation |
| 4 | **Work task dependency graph with cycle detection** | Upstream's todo list is flat; Go models task dependencies as a DAG |
| 5 | **Grace call mechanism** | After budget exhaustion, one extra text-only turn for conclusion |
| 6 | **Preflight compression on session resume** | Auto-compacts sessions >100K tokens on resume |
| 7 | **Exa search, Process, Runtime info, Tool search tools** | Multiple Go-original tools with no upstream equivalent |
| 8 | **LLM-autonomous skill discovery** | Go provides SkillSearch/SkillList/SkillRead tools; upstream requires pre-loaded skills |
| 9 | **MultiEdit tool** | Multiple edits in single invocation; upstream uses repeated single Edit calls |
| 10 | **TodoWrite with dependencies** | `addBlocks`/`addBlockedBy` dependency relationships; upstream's todo is flat list |

---

## 22. Enhancement Statistics

| Category | Count | Go增强 | Go适配 |
|----------|-------|--------|--------|
| Error Classification | 11 | 10 | 1 |
| Work Task Dependencies | 6 | 5 | 1 |
| File History Tools | 12 | 11 | 1 |
| Message Normalization | 5 | 5 | 0 |
| Rate Limit Parsing | 11 | 10 | 1 |
| Streaming Architecture | 15 | 11 | 4 |
| Retry Utilities | 5 | 5 | 0 |
| Ctrl+C Handling | 6 | 0 | 6 |
| Transcript Builder | 5 | 5 | 0 |
| Windows Adaptations | 3 | 0 | 3 |
| Concurrency | 8 | 0 | 8 |
| **Total** | **87** | **52** | **25** |

**Key insight**: Go enhancements (52 items) are concentrated in four areas:
1. **Error classification** — structured taxonomy with recovery strategies vs. flat string tags
2. **File history** — tag-based categorization, grep/search/timeline/batch tools vs. simple backup-restore
3. **Rate limits** — multi-provider header parsing with 4-bucket state vs. Anthropic-only mock
4. **Streaming** — standalone adapter/handler/bus architecture vs. embedded query-loop logic

Go adaptations (25 items) fall into two categories:
1. **Concurrency primitives** (mutexes, atomics) — necessary because Go is multi-threaded
2. **Platform handling** (Windows paths, Ctrl+C/stdin) — Go's lower-level I/O requires explicit handling

---

## Cross-References

- Tool details: [02-tools.md](02-tools.md) §4-6
- Stall detection: [01-core-agent-loop.md](01-core-agent-loop.md) §4
- Rate limiting: [04-api-client.md](04-api-client.md) §7
- File history: [02-tools.md](02-tools.md) §6
- Architecture patterns: [07-architecture.md](07-architecture.md)
- Testing patterns: [09-testing.md](09-testing.md)
