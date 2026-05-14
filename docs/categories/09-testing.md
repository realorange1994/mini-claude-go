# 09 — Testing Patterns & Coverage Gaps

> Test framework, snapshot testing, integration coverage, coverage comparison, cross-cutting test patterns

## Overview

Go has ~150 test files using standard `testing.T`; upstream has ~94 test files using Bun's `describe/test` framework. Both sides have unique test strengths. After the upstream test porting effort (commits 5c0988d through dad59aa), Go now has 3,108+ test functions with ~51.7K lines of test code, with upstream-quality patterns (invariants, roundtrips, idempotency, boundary conditions) ported across all modules where Go equivalents exist.

---

## 1. Test Framework

| Aspect | Go | Upstream | Gap |
|--------|-----|----------|-----|
| Test runner | `testing.T` (standard library) | Bun `describe/test` with `expect` assertions |
| Test count | ~150 files, ~3,100+ test functions, ~51.7K lines | ~94 files |
| Test types | Unit tests, integration, concurrency tests, benchmarks | Unit, integration, snapshot, E2E |
| Test configuration | None | `vitest.config.ts` with 120s timeout, file exclusions |
| Mock infrastructure | Minimal: simple struct stubs, callbacks | Minimal: simple object stubs |
| Test utilities | None (manual setup) | `createTestContext()`, `createTestQuery()`, `createMockServer()` |
| Benchmarks | Present (`BenchmarkNormalizeWhitespace`, `BenchmarkSortMapKeys`) | Absent |

**Impact**: No automated verification. Any change risks regression without detection.

---

## 2. Snapshot Testing

| Aspect | Go | Upstream | Gap |
|--------|-----|----------|-----|
| Snapshot framework | None | Vitest snapshot testing |
| Snapshot coverage | None | System prompt generation, permission decisions, tool output formatting, error messages |
| Snapshot update | N/A | `--update` flag for approved changes |
| VCR recording | None | `withStreamingVCR()` for API replay tests |

---

## 3. Per-Module Test Coverage Comparison

[diff_upstream/29-testing.md]

### 3.1 Agent Loop / Sub-Agent (`agent_sub_test.go`)

**Upstream counterpart**: No direct equivalent test file.

| What Go tests that upstream doesn't | What upstream tests that Go doesn't |
|--------------------------------------|--------------------------------------|
| Sub-agent system prompt section presence | `parseToolPreset` for tool preset parsing |
| Agent type-specific identity strings | `filterToolsByDenyRules` deny rule filtering |
| READ-ONLY MODE constraints for Explore/Plan agents | Tool `aliases` resolution (`toolMatchesName` with aliases) |
| Verify agent adversarial probing instructions | `buildTool` default property filling |
| Priority ordering of prompt sections | `filterToolProgressMessages` progress message filtering |
| Tool registry filtering via allowed/disallowed lists | `getEmptyToolPermissionContext` default context creation |
| Disallowed overrides allowed (disallowed_tools wins) | |
| Wildcard allowed tools (`*`) | |

**Coverage gaps**:
- **Go**: No test for sub-agent tool execution (only prompt construction and registry filtering)
- **Upstream**: No test for sub-agent orchestration at all
- **Edge cases (Go)**: Tests that disallowed always wins over allowed even when both specify same tool

### 3.2 Exec Tool / Bash Permission Checks (`tools/exec_tool_test.go`)

**Upstream counterpart**: `dangerousPatterns.test.ts`, `shellRuleMatching.test.ts`

| What Go tests that upstream doesn't | What upstream tests that Go doesn't |
|--------------------------------------|--------------------------------------|
| `rm -rf /` and `rm -rf ~` direct denial | `CROSS_PLATFORM_CODE_EXEC` list validation (python, node, ruby, etc.) |
| Internal URL blocking (10.0.0.1, localhost, 192.168.1.1) | `DANGEROUS_BASH_PATTERNS` duplicate entry detection |
| Fork bomb detection (`:(){ :|: & };:`) | `permissionRuleExtractPrefix` for `:*` syntax extraction |
| Command substitution detection (`$(whoami)`, backticks) | `hasWildcards` with escaped/unescaped wildcards |
| Dangerous variable expansion detection (`${IFS}`, `${!VAR}`) | `matchWildcardPattern` with case-insensitive matching |
| Safe variable expansion allowlist (`$HOME`, `$USER`, `$PWD`) | `parsePermissionRule` exact/prefix/wildcard classification |
| Glob expansion in destructive commands | `suggestionForExactCommand`/`suggestionForPrefix` |
| Compound command splitting and checking | |
| Safe wrapper stripping (timeout, nice, nohup, etc.) | |
| System path protection validation | |
| Critical project file protection (.git, go.mod, etc.) | |
| Windows path detection | |
| Background task execution with/without callback | |
| Deletion target extraction with `--` separator | |
| Path escape detection via `--` | |
| **`PosixToWindowsPath`** — MSYS2 mount points (/tmp/, /home/, /cygdrive/), drive letters (/x/), UNC paths (//server/share), Cygwin drive prefix, relative paths, already-Windows paths (R23) | |
| **Cygdrive path roundtrip** — `Windows→POSIX→Windows` equivalence for `C:\Users\foo`, `\\server\share`, `E:\workspace\project` (ported) | |
| **`containsVulnerableUncPath`** — `\\server\share`, `//server/share` (non-URL), `https://` not detected, `\\192.168.1.1\share` IPv4, `\\server@SSL@8443\path` WebDAV, `DavWWWRoot` marker (ported) | |
| **`IsReadOnlyCommand`** — empty string, pure whitespace, alwaysReadOnly list, git subcommand variants, `>` redirect, safe wrappers like `timeout ls` (ported) | |
| **`CheckDestructiveWarning`** — `rm -rf` warning, `echo hello` no warning, empty string no warning (ported) | |

**Test patterns**: Go uses `CheckPermissions()` integration tests with actual command strings; upstream uses pattern list membership checks. Go tests are more behavioral; upstream tests are more declarative.

### 3.3 Normalize / Whitespace (`normalize_test.go`)

**Upstream counterpart**: No single direct equivalent. Scattered across `stringUtils.test.ts` etc.

| What Go tests that upstream doesn't | What upstream tests that Go doesn't |
|--------------------------------------|--------------------------------------|
| Whitespace normalization (trailing spaces, blank line collapse) | `normalizeFullWidthDigits` (full-width to half-width) |
| JSON key sorting (unsorted → sorted) | `normalizeFullWidthSpace` (full-width space to half-width) |
| API message normalization with Anthropic SDK types | |
| Tool-use input key sorting | |
| Tool-result whitespace normalization | |
| `sortValueKeys` recursive map sorting | |
| `NormalizeJSONBytes` invalid JSON passthrough | |
| Benchmarks (`BenchmarkNormalizeWhitespace`, `BenchmarkSortMapKeys`) | |
| `escapeRegExp` special character escaping (ported from `stringUtils.test.ts`) | |
| `capitalize` first character uppercasing (ported) | |
| `plural` singular/plural forms (ported) | |
| `firstLineOf` extraction (ported) | |
| `EndTruncatingAccumulator` state machine (ported) | |
| `truncateToLines` / `truncateToWidth` (ported from `format.test.ts`) | |
| `wrapText` CJK-aware wrapping (ported) | |

**Coverage gaps**:
- **Go**: Missing full-width character normalization (important for Japanese/Chinese input)
- **Upstream**: Missing API message normalization tests (Anthropic SDK-specific)
- **Upstream**: Missing JSON key deterministic ordering tests

### 3.4 Prompt Caching (`prompt_caching_test.go`)

**Upstream counterpart**: No equivalent test file in upstream.

All test functions are Go-specific additions: cache marker placement, TTL field presence, tool role message cache_control, string/array format conversion, `FormatCachedSystemPrompt`, `deepCopyMessages` mutation independence.

**Coverage gap**: Go has no test for cache scope types (`global`, `org`); upstream supports these but Go only supports `ephemeral`.

### 3.5 Rate Limiting (`rate_limit_test.go`)

**Upstream counterpart**: No equivalent test file in upstream.

All 15+ test functions (bucket calculations, header parsing, display formatting, progress bar) are Go-specific additions with no upstream counterpart.

### 3.6 Retry / Backoff (`retry_utils_test.go`)

**Upstream counterpart**: No equivalent test file in upstream.

All 6 test functions (exponential growth, max cap, overflow protection, negative attempt, custom options, deterministic mode) are Go-specific additions.

### 3.7 Error Classification (`error_types_test.go`)

**Upstream counterpart**: No equivalent test file in upstream.

All 30+ test functions covering 14 error classes, sub-classifications, Chinese patterns, server disconnect heuristics, and backward compatibility wrappers are Go-specific additions.

### 3.8 Compaction (`compact_test.go`)

**Upstream counterpart**: `prompt.test.ts`, `grouping.test.ts`

| What Go tests that upstream doesn't | What upstream tests that Go doesn't |
|--------------------------------------|--------------------------------------|
| `EstimateTokens` character-based estimation | Upstream compact prompt template tests |
| `NeedsCompaction` threshold detection | Message grouping by round/conversation |
| `SmartCompact` full compaction flow | |
| `CheckReactiveCompact` token spike detection | |
| `PartialCompactUpTo` / `PartialCompactFrom` directional compaction | |
| `entriesToSummaryText` entry serialization | |
| `adjustPivotForToolPairs` tool pair boundary | |

### 3.9 File History (`filehistory_test.go`)

**Upstream counterpart**: No equivalent test file in upstream.

All 12 test functions (snapshot take/rewind/list, descriptions, multi-file, RestoreLast, RewindSteps, disk persistence) are Go-specific additions.

### 3.10 Streaming (`streaming_test.go`)

**Upstream counterpart**: No equivalent test file in upstream.

All 25+ test functions (CollectHandler, StreamBus pub/sub, contextErr, FinishReason, HasPartialToolCall, HasTruncatedToolArgs, DeltasState state machine, StreamProgress, toolUseAsText detection) are Go-specific additions.

### 3.11 MCP Client / Manager (`mcp/client_test.go`)

**Upstream counterpart**: `normalization.test.ts`, `filterUtils.test.ts`, `envExpansion.test.ts`, `mcpStringUtils.test.ts`, `channelNotification.test.ts`

| What Go tests that upstream doesn't | What upstream tests that Go doesn't |
|--------------------------------------|--------------------------------------|
| Manager register/list/get status | `normalizeNameForMCP` with claude.ai prefix collapsing |
| CallTool/CallToolWithServer error paths | Header parsing (`Key: Value` format) |
| `StopAll` on empty manager | Environment variable expansion |
| Client creation with env vars | MCP string utilities |
| Concurrency safety (10 goroutines registering simultaneously) | Channel notification utilities |

### 3.12 Skills / Loader (`skills/loader_test.go`)

**Upstream counterpart**: No equivalent test file in upstream.

All 12 test functions (frontmatter parsing, inline list parsing, unquote, strip frontmatter, load/list/summary, always skills, context loading) are Go-specific additions.

### 3.13 Permission Rule Parser (`permissions/rule_parser_test.go`)

**Upstream counterpart**: `permissionRuleParser.test.ts`

| What Go tests that upstream doesn't | What upstream tests that Go doesn't |
|--------------------------------------|--------------------------------------|
| `ParseRule` error handling for empty/unmatched parens | `normalizeLegacyToolName` (only Task→Agent, KillShell→TaskStop) |
| `resolveAlias` with 4 aliases (Task→Agent, etc.) | |
| `globMatch` with prefix/suffix/contains/wildcard/question | |
| MCP server-level matching (`mcp__server1` matches `mcp__server1__tool1`) | |
| MCP wildcard matching (`mcp__server1__*`) | |
| `FormatRule` output formatting | |
| `IsToolLevel` / `ToolMatches` / `ContentMatches` predicates | |
| `ParseRules` batch parsing with behavior assignment | |
| `escapeParens`/`unescapeParens` roundtrip (ported) | |
| `permissionRuleValueFromString`/`permissionRuleValueToString` roundtrip (ported) | |
| MCP-style parsing with `mcp__server__tool` format (ported) | |
| `FormatRule` roundtrip: `ParseRule(FormatRule(r)) == r` (ported) | |

**Edge cases**: Go has 4 aliases; upstream only has 2. Go adds AgentOutputTool→TaskOutput and BashOutputTool→TaskOutput.

### 3.14 Hook Shell Execution (`hooks_shell_test.go`)

**Upstream counterpart**: No equivalent test file in upstream.

All 12 test functions are Go-specific additions (R22):

| Test Function | What's Tested |
|---------------|---------------|
| `TestHookGlobMatch` | Glob pattern matching with `**`, `*`, `?`, character classes |
| `TestMatchHook` | Hook name matching against glob patterns and prefixes |
| `TestHookShellResult_ParseStdout` | Hook JSON stdout parsing with error extraction |
| `TestExecuteShellHook_Echo` | Successful shell hook execution with output capture |
| `TestExecuteShellHook_Block` | Hook block behavior (denial via exit code) |
| `TestHookBlockError` | Error wrapping for blocked hook executions |
| `TestLoadHooksFromSettings` | Loading hook definitions from settings.json |
| `TestLoadHooksFromSettings_MissingFile` | Graceful handling of missing settings file |
| `TestLoadHooksFromSettings_NoHooks` | Graceful handling of settings with no hooks section |
| `TestHookShellResult_UpdatedInput` | Hook-modified command input propagation |
| `TestHookEnvironment` | Environment variable injection into hook execution |
| `TestHookJSONRoundTrip` | JSON serialization/deserialization of hook input/output |

**Test patterns**: Hook tests use real shell execution (not mocked), testing end-to-end command execution with stdout/stderr capture, exit code handling, and JSON round-trip serialization.

### 3.15 Path Validation (`permissions/path_validation_test.go`)

**Upstream counterpart**: Partially `windowsPaths.test.ts`

| What Go tests that upstream doesn't | What upstream tests that Go doesn't |
|--------------------------------------|--------------------------------------|
| `IsInternalEditablePath` for plan files | Comprehensive Windows path handling (separators, long paths, UNC) |
| `IsInternalReadablePath` for various internal dirs | XDG directory resolution |
| Plan file .md extension check | Path manipulation utilities |
| `resolvePath` absolute/relative resolution | |
| `expandTilde` tilde expansion | |
| `isUncPath` UNC path detection | |
| `hasSuspiciousWindowsPathPattern` (8.3 short names, etc.) | |
| `ValidatePath` / `ValidateReadPath` with UNC, glob, suspicious patterns | |

### 3.15 Tool Registry (`tools/registry_test.go`)

**Upstream counterpart**: No direct equivalent for Go-specific `Registry` type.

| What Go tests that upstream doesn't | What upstream tests that Go doesn't |
|--------------------------------------|--------------------------------------|
| `GetRecentlyReadFiles` with time ordering | `findToolByName` with alias resolution |
| `CheckFileStale` mtime-based staleness | `toolMatchesName` with aliases |
| `MarkFileRead` tracking | `buildTool` default property filling |

---

## 4. Go Tool-Specific Test Files With No Upstream Counterpart

| Go Test File | What's Tested |
|-------------|---------------|
| `git_tool_test.go` | Git operations |
| `hooks_shell_test.go` | Hook shell execution: glob matching, JSON round-trip, settings loading, environment injection (R22) |
| `brief_tool_test.go` | Brief output tool |
| `runtime_info_test.go` | Runtime/system info |
| `web_search_test.go` | Web search tool |
| `mcp_tools_test.go` | MCP tool integration |
| `multi_edit_test.go` | Multi-file edit operations |
| `base_test.go` | Base tool interface |
| `agent_tools_test.go` | Agent tool registration |
| `agent_store_test.go` | Agent state storage |
| `tool_search_tool_test.go` | Tool search/discovery |
| `system_tool_test.go` | System operations |
| `file_read_magic_test.go` | File type detection by magic bytes |
| `todo_write_test.go` | Todo list management |
| `memory_tool_test.go` | Memory persistence tool |
| `filesystem_safety_test.go` | Filesystem safety checks |
| `grep_tool_test.go` | Grep/search tool |
| `file_edit_test.go` | File editing |
| `file_write_test.go` | File writing |
| `process_test.go` | Process management |
| `skill_tools_test.go` | Skill invocation tools |
| `task_tool_test.go` | Task management |
| `file_read_test.go` | File reading |
| `fileops_test.go` | File operations |
| `glob_tool_test.go` | Glob pattern matching |
| `web_fetch_test.go` | Web fetching |
| `agent_tool_test.go` | Agent delegation |
| `misc_tools_test.go` | Miscellaneous tools |
| `exa_search_test.go` | Exa search integration |
| `send_message_tool_test.go` | Message sending |
| `terminal_tool_test.go` | Terminal operations |
| `ask_user_question_test.go` | User interaction |

---

## 5. Major Upstream Test Files — Porting Status

### 5a. Ported (Go equivalents created)

| Upstream Test File | Go Equivalent(s) | Notes |
|-------------------|-------------------|-------|
| `CircularBuffer.test.ts` | `circular_buffer_test.go` | Ring buffer: Add, AddAll, Length, ToArray, GetRecent, Clear |
| `claudemd.test.ts` | `claudemd_test.go` | StripHtmlComments, IsMemoryFilePath, GetLargeMemoryFiles |
| `collapseHookSummaries.test.ts` | `hook_summaries_test.go` | Hook summary collapsing |
| `configConstants.test.ts` | `config_constants_test.go` | NotificationChannels, EditorModes, TeammateModes |
| `cron.test.ts` | `cron_test.go` | Cron parsing, next run, human-readable, leap year, roundtrip |
| `diff.test.ts` | `diff_test.go` | 18 test functions: identical, empty, Unicode, determinism, large content |
| `envUtils.test.ts` | *(Go stdlib)* | Environment utilities covered by existing config tests |
| `errors.test.ts` | `error_types_test.go` | 14 error classes, 30+ test functions |
| `file.test.ts` | `file_utils_test.go` | ConvertLeadingTabsToSpaces, AddLineNumbers, PathsEqual |
| `fingerprint.test.ts` | `fingerprint_test.go` | SHA-256 fingerprint with salt |
| `format.test.ts` | `truncate_utils_test.go`, `string_utils_test.go` | truncateToWidth, truncatePathMiddle, wrapText, capitalize, plural |
| `frontmatterParser.test.ts` | `skills/loader_test.go` | Frontmatter parsing, inline lists, unquote |
| `git.test.ts` | `tools/git_tool_test.go` | Git operations |
| `glob.test.ts` | `tools/glob_tool_test.go` | Glob with excludes, empty dir, UNC, head limit |
| `groupToolUses.test.ts` | `group_tool_uses_test.go` | ApplyGrouping, RenderGroupedToolUse |
| `hash.test.ts` | `hash_test.go` | djb2Hash, hashContent, hashPair |
| `markdown.test.ts` | `claudemd_test.go` | Markdown handling (code block preservation) |
| `modelCost.test.ts` | `cost_tracker_test.go` | Pricing, family names, token accuracy |
| `sanitization.test.ts` | `permissions/auto_strip_test.go` | DANGEROUS_SHELL_PATTERNS tests |
| `semver.test.ts` | `semver_test.go` | Semver comparison with pre-release precedence |
| `stream.test.ts` | `streaming_test.go` | 25+ test functions: CollectHandler, StreamBus, state machine |
| `uuid.test.ts` | `uuid_test.go`, `core_test.go` | validateUUID, createAgentId, randomHex |
| `windowsPaths.test.ts` | `permissions/path_validation_test.go`, `tools/exec_tool_test.go` | Mixed separators, UNC, long path prefix, Cygdrive roundtrip |
| `xml.test.ts` | `core_test.go` | XML handling in core tests |
| `notebook.test.ts` | `tools/notebook_edit_test.go` | Cell-N format edge cases, leading zeros |
| `shellRuleMatching.test.ts` | `tools/exec_tool_test.go` | Windows path roundtrip, fork bomb, command substitution |
| `config.test.ts` | `config_test.go` | settings.local.json skip, project MCP override |

### 5b. Remaining Upstream Files Without Go Counterpart

| Upstream Test File | What's Tested | Reason No Go Port |
|-------------------|---------------|-------------------|
| `contentArray.test.ts` | Content array manipulation | Upstream Anthropic SDK-specific |
| `gitDiff.test.ts` | Git diff parsing | Go delegates to `git diff` CLI |
| `slashCommandParsing.test.ts` | Slash command parsing | Go uses Skill tool, not CLI slash commands |
| `systemPrompt.test.ts` | System prompt assembly | Partially covered by `system_prompt_test.go` |
| `tokenBudget.test.ts` | Token budget calculation | Go uses simpler token estimation |
| `zodToJsonSchema.test.ts` | Zod schema conversion | Go uses struct tags, not Zod |
| Various bridge/transport/daemon tests | Bridge messaging, SSE transport, remote interrupts | Go architecture differs significantly |
| Various task tests | LocalAgentTask, DreamTask, RemoteAgentTask | Go task model is structurally different |

---

## 6. Cross-Cutting Test Patterns

[diff_upstream/29-testing.md §6.18]

### 6.1 Go Test Strengths

1. **Behavioral security testing** — Go's exec tool tests actually verify that dangerous commands are blocked at the permission layer, not just that patterns exist in a list
2. **Integration with real filesystem** — `t.TempDir()` pattern provides real file I/O testing for file history, skills, and directory listing
3. **Concurrency testing** — At least one concurrency test exists (`TestManagerConcurrency`) using goroutines
4. **Error classification exhaustiveness** — 14 error classes with sub-classifications, Chinese pattern support, server disconnect heuristics
5. **Benchmarks** — Go has `BenchmarkNormalizeWhitespace` and `BenchmarkSortMapKeys`; upstream has none
6. **Novel features tested** — Prompt caching, rate limiting, retry backoff, file history, streaming bus — all tested in Go but not in upstream
7. **Upstream-quality patterns ported** — Invariant tests, roundtrip tests, idempotency checks, boundary conditions, concurrency tests, JSON roundtrip integrity, Cygdrive path roundtrip — all ported from upstream across ~26 modules

### 6.2 Upstream Test Strengths

1. **Utility function coverage** — 60+ utility test files covering string operations, path handling, formatting, parsing, etc.
2. **Full-width character support** — `normalizeFullWidthDigits`, `normalizeFullWidthSpace` tests
3. **Schema validation** — `lazySchema.test.ts`, `zodToJsonSchema.test.ts`
4. **Bridge/transport layer** — SSE transport, bridge messaging, permission callbacks
5. **Configuration system** — Settings, config constants, XDG directories
6. **Model management** — Model aliases, providers, cost calculation

### 6.3 Common Patterns

- Both sides use table-driven test approaches
- Both sides prefer pure unit tests over integration tests
- Neither side has comprehensive E2E tests
- Neither side uses heavy mocking frameworks (Go avoids interfaces; upstream avoids complex test doubles)

### 6.4 Largest Gaps

1. **Go → Upstream**: Most upstream utility test files have been ported (~26 modules). Remaining gaps are platform-specific (Zod schema, content array, token budget) where Go uses different abstractions.
2. **Upstream → Go**: Upstream is missing all Go-specific feature tests (prompt caching, rate limiting, retry backoff, file history, streaming, error classification, sub-agent orchestration, MCP client management)
3. **Both**: Neither side tests the full agent loop end-to-end; both test individual components in isolation

---

## 7. Cross-Cutting Architectural Testing Patterns

[diff_upstream/27-cross-cutting.md §50.3]

| # | Aspect | Go | Upstream | Type |
|---|--------|----|----------|------|
| 1 | Test framework | Standard `testing` package; `t.Errorf`/`t.Fatalf` | `bun:test` (`describe`, `test`, `expect`, `beforeEach`) | Go适配 |
| 2 | Test organization | One `_test.go` per source file; flat `TestXxx` naming | Co-located `__tests__/*.test.ts`; BDD-style `describe/test` | Go适配 |
| 3 | Test isolation | Fresh structs per test (`DefaultConfig()`, `NewConversationContext()`) | `resetStateForTests()` in `beforeEach`; env saved/restored | Go适配 |
| 4 | Table-driven tests | Common: `tests := []struct{...}{...}; for _, tc := range tests` | BDD `test()` per case; `expect(...).toBe(...)` | Go适配 |
| 5 | Mocking | No framework; inject via struct fields or callbacks | `bun:mock` for module-level; DI via `QueryDeps`/`productionDeps()` | 简化 |
| 6 | Test count | ~150 `*_test.go` files, ~3,100+ test functions, ~51.7K lines | ~194 `*.test.ts`/`*.test.tsx` files | Go适配 |
| 7 | Integration tests | `combined_exec_test.go`, `forked_agent_test.go` | Multi-module integration: autonomy, daemon, bridge | 简化 |
| 8 | Snapshot testing | None | VCR recording via `withStreamingVCR()` for API replay | 缺失 |
| 9 | Feature-gated tests | None | `feature('FEATURE_NAME')` gates in test imports/bodies | 缺失 |

---

## 8. E2E Test Coverage

| Area | Upstream | Go |
|------|----------|-----|
| Full conversation | Yes — multi-turn conversations with tool use | None |
| File editing | Yes — edit, verify, undo | None |
| Session resume | Yes — create, exit, resume, verify | None |
| Compaction | Yes — trigger, verify, continue | None |
| MCP integration | Yes — connect, discover, use tools | None |

---

## 9. Recommended Go Test Priority

| Priority | Test Area | Rationale |
|----------|-----------|-----------|
| P0 | Message normalization (pairing, role alternation) | Critical for API correctness — 400 errors |
| P0 | Error classification | Foundation for retry/recovery |
| P0 | Permission classifier | Security — currently fail-open |
| P1 | Tool execution (edit, exec) | Most-used features |
| P1 | Cache breakpoint placement | Cost optimization |
| P1 | Compaction (reactive, micro) | Context management reliability |
| P2 | System prompt assembly | Cache efficiency |
| P2 | Retry/rate-limiting | Reliability |
| P2 | MCP server lifecycle | Extensibility |

---

## 10. Coverage Statistics

| Metric | Value |
|--------|-------|
| **Total document lines** | 8,717 |
| **Top-level sections** | 105 |
| **Subsections** | 724 |
| **Table rows (approx. documented differences)** | 2,744 |
| **Source files compared (Go)** | ~50+ `.go` files |
| **Source files compared (Upstream TS)** | ~200+ `.ts`/`.tsx` files |
| **Major comparison domains** | Streaming, Normalization, Caching, System Prompt, MCP, Testing, Agent Loop, Compaction, Tool Interface, Bash/Exec, File Read/Edit/Write, Config, Permissions, Deep Streaming/Caching/Memory/Hooks/Retry, Error Types, Progress Tracking, Async Tasks, Forked Agents, File History, Transcript, Work Tasks, Skills, Sub-Agents, Tool Implementations, Permissions Submodule, Search/Listing Tools, PostCompact Recovery, Context Management, Entry Points/Build, Hooks, Session Memory, Retry/Rate-Limit/Normalization, Error Classification, Transcript/Resume, Cache Breakpoints, Classifier, Model Routing/Cost/Background Services, Context References, File History Snapshots, Git Tool, Agent Tool, Security/Sandbox |

---

## Cross-References

- Permission classifier bug: [03-system-prompt.md](03-system-prompt.md) §2
- Message normalization gaps: [07-architecture.md](07-architecture.md) §3
- Cache breakpoint issue: [04-api-client.md](04-api-client.md) §5
- Go enhancements: [08-enhancements.md](08-enhancements.md)
