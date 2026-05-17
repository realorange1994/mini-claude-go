# Executive Summary — Go vs Upstream Gap Analysis

## Overview

This document summarizes the gap analysis between Go **miniClaudeCode** and the upstream TypeScript **claude-code**, based on a comprehensive 11627-line comparison covering 54+ sections.

**Methodology**: Line-by-line diff comparison across all major subsystems. Gap types classified as: 缺失 (missing), 简化 (simplified), Go增强 (Go enhancement), Go适配 (Go adaptation), 差异 (difference), 匹配 (matched).

## Key Statistics

| Metric | Count |
|--------|-------|
| Total comparison sections | 54+ |
| Critical (P0) gaps | ~12 |
| Important (P1) gaps | ~18 |
| Nice-to-have (P2) gaps | ~15 |
| Go enhancements over upstream | ~8 |
| Fully matched areas | ~6 |
| Missing upstream systems in Go | ~25 |

## Top 10 Gaps by Impact

| # | Gap | Gap Type | Impact |
|---|-----|----------|--------|
| 1 | **OAuth/PKCE authentication** | 缺失 | Cannot use OAuth flow; API-key only |
| 2 | **Analytics & telemetry** (OpenTelemetry+Sentry+Langfuse) | 缺失 | No observability into user behavior or errors |
| 3 | **TUI** (Ink/React terminal interface) | 缺失 | Headless CLI only, no interactive UI |
| 4 | **Beta header system** (15+ constants) | 缺失 | Cannot access beta API features |
| 5 | **Model management** (aliases, context windows, providers) | 缺失 | Single hardcoded provider/model |
| 6 | **Tool pairing validation** (`ensureToolResultPairing`) | 已修复 | Tool result delivery fixed; shared tool pointer overwrite fixed |
| 7 | **Role alternation enforcement** | 缺失 | Multiple consecutive same-role messages rejected |
| 8 | **GrowthBook feature flags** | 缺失 | No remote feature gating |
| 9 | **Cost tracking** | 缺失 | No USD cost calculation or display |
| 10 | **Hook system** (20+ types vs ~5 in Go) | 缺失 | Limited extensibility |

## Go Strengths (Go增强)

| Feature | Go Advantage |
|---------|-------------|
| **@-reference system** | Explicit `@diff`, `@staged`, `@git:N`, `@url` types (upstream has none) |
| **SkillTracker** | Dedicated progressive discovery with shown/read/used state |
| **GitTool** | 35+ structured operations with safety checks (upstream uses BashTool) |
| **File history** | Rich tagging, annotations, cross-file timeline, per-file restore |
| **Context reference budgets** | Hard/soft token limits with blocking gates |
| **Rate limit state** | Persistent tracking across requests with proactive delays |
| **Chinese error patterns** | Supports Chinese provider error messages |
| **Stall detection** | Always-on with automatic recovery (upstream: env-gated, telemetry-only) |
| **Windows path handling** | `PosixToWindowsPath()` matches upstream's approach: MSYS2 mounts (/tmp/, /home/), Cygdrive, UNC |
| **Pipe input mode** | `io.ReadAll()` for non-terminal stdin fixes multi-line prompt splitting |
| **Git Bash detection** | `findGitBashForWindows()` with memoize pattern, auto-selected as default Windows shell |
| **Source index** | Full source code lookup for all ~1000 functions (builtins, helpers, special forms, stdlib) with pagination. Upstream has no equivalent |
| **rgrep engine** | Native ripgrep wrapper with .gitignore support, replaces custom go-search |
| **Sub-agent isolation** | Independent ExecTool/MCPToolCaller instances per child agent, preventing shared-pointer overwrite bugs |
| **Killed agent notifications** | Partial result extraction + "killed" status notification, matching upstream's AgentToolResult format |

## Go Simplifications (简化)

| Feature | Go Approach | Upstream Approach |
|---------|------------|-------------------|
| **Cache breakpoints** | 4 breakpoints | 1 (optimized for Mycro KV cache) |
| **Transcript** | Flat JSONL, 8 entry types | DAG-based JSONL, 19+ entry types |
| **System prompt** | Single string with sections | 10+ component pipeline |
| **Retry** | Simple exponential backoff | Async generator with subscriber logic |
| **Error classification** | 15-category enum, string matching | 25+ categories with type guards |
| **Skill content** | Raw markdown injection | Rich pipeline (args, vars, shell, fork, model) |
| **Multi-edit** | Atomic multi-edit per call | Single edit per call |

## Recently Resolved (R23)

| # | Gap | Resolution |
|---|-----|------------|
| 1 | **Path inconsistency** — file tools and exec resolved different physical files for `/tmp/` on Windows | `PosixToWindowsPath()` maps MSYS2 mounts (/tmp/, /home/, /cygdrive/) to Windows native paths. `expandPath()` uses the converter on Windows for all file tools |
| 2 | **Shell detection** — hardcoded static text in system prompt | `GetShellInfo()` dynamically detects Git Bash/PowerShell on Windows. `GetPathFormatInfo()` returns path format guidance injected into system prompt |
| 3 | **Pipe input split** — multi-line piped input split by `ReadString('\n')` causing empty first prompt | `io.ReadAll(stdinReader)` for non-terminal stdin reads entire input as single prompt |

## Recently Resolved (R24-R25)

| # | Gap | Resolution |
|---|-----|------------|
| 4 | **Exec background tasks invisible** — parent agent's `task_output`/`task_stop` return "task not found" for its own `exec run_in_background=true` tasks | Root cause: `buildSubAgentRegistry` copies tool pointers, `child.registerBashBgTool()` overwrites shared `ExecTool.BackgroundTaskCallback`. Fix: create new `ExecTool`/`MCPToolCaller` instances for child agents instead of modifying shared instances |
| 5 | **Killed sub-agent empty notification** — killed agents send "completed" notification with 0 tokens and empty result, confusing parent LLM | Detect `TaskKilled` status in child goroutine completion path, send "killed" notification with partial result. `InjectNotifications` uses different prefix when killed tasks exist |
| 6 | **Source index missing helpers** — ~255 helper functions and special form code invisible to source/source-list queries | `scanHelperFunctions()` indexes all non-builtin Go funcs as `Kind: "helper"`. `scanSpecialForms()` extracts actual Go source code for each special form from `eval_core.go` |
| 7 | **Source query token explosion** — `SourceList("")` returns all ~1000+ functions at once, consuming entire context | Add `offset`/`limit` pagination to `GetSource()` and `SourceList()`, default limit 50 entries |
| 8 | **Search output unlimited** — grep/glob/go-search can fill entire context window | Add `truncateOutput` utility with configurable max chars, circuit breaker for output size |
| 9 | **Unicode string bugs** — `string-length`/`string-find` use byte offsets instead of rune offsets for multi-byte chars | Use `utf8.RuneCountInString()` for length, `[]rune` indexing for positions |
| 10 | **Code organization** — 90+ functions scattered across files without single-responsibility principle | 6-phase reorganization: equality.go, type_system.go, concurrency.go, data_structures.go, list_ops.go cleanup; sequences.go stub deleted |
| 11 | **Custom go-search replaced** — maintenance burden and feature gaps | Replace with rgrep engine (native ripgrep wrapper with .gitignore support) |

## Recently Resolved (R26) — Microlisp Language Compliance

| # | Gap | Resolution |
|---|-----|------------|
| 12 | **4-deep car/cdr missing** — all 16 four-deep compositions (caaaar..cddddr) undefined | Add 16 `(define ...)` forms to stdlib.go: caaaar, caaadr, caadar, caaddr, cadaar, cadadr, caddar, cadddr, cdaaar, cdaadr, cdadar, cdaddr, cddaar, cddadr, cdddar, cddddr |
| 13 | **defvar/defparameter return symbol** — returned `vsym(name)` instead of evaluated value | defvar returns evaluated value when initform provided and unbound; defparameter returns evaluated value |
| 14 | **parse-integer inconsistent errors** — two paths returned different messages ("no integer at position" vs "not an integer") | Unify to `"parse-integer: junk in string \"xxx\""` showing the failed substring |

## Refactoring Priorities

### Phase 1: Critical Capability (P0)
1. Tool pairing validation + role alternation enforcement
2. Empty message filtering
3. Cache breakpoint optimization (4 -> 1)
4. Message normalization pipeline
5. Context window per-model resolution

### Phase 2: Quality & Reliability (P1)
1. Reactive compaction system
2. Cache economics (break detection, token tracking, pinning)
3. 529 model fallback / subscriber-aware 429 handling
4. Auto-classifier fail-closed behavior
5. Transcript DAG support

### Phase 3: Ecosystem & Extensibility (P1/P2)
1. Hook system expansion (5 -> 20+ types)
2. Skill content pipeline
3. Multi-source settings system
4. Cost tracking
5. Model alias system

### Phase 4: TUI & Services (P2)
1. OAuth/PKCE authentication
2. Basic TUI layer
3. Analytics/telemetry scaffolding
4. Feature flag system

## Cross-Reference Index

- **Core agent loop**: [01-core-agent-loop.md](categories/01-core-agent-loop.md)
- **Tools**: [02-tools.md](categories/02-tools.md)
- **System prompt**: [03-system-prompt.md](categories/03-system-prompt.md)
- **API client**: [04-api-client.md](categories/04-api-client.md)
- **Services**: [05-services.md](categories/05-services.md)
- **UI/TUI**: [06-ui-tui.md](categories/06-ui-tui.md)
- **Architecture**: [07-architecture.md](categories/07-architecture.md)
- **Go enhancements**: [08-enhancements.md](categories/08-enhancements.md)
- **Testing**: [09-testing.md](categories/09-testing.md)
- **Source data**: `diff_upstream.md` (11627 lines)
