# Testing

> Test coverage

## Sections Included
- [##] Line 620-1063 -- ## 6. Test Coverage Comparison: Go vs Upstream TypeScript
- [##] Line 8745-8759 -- ## A. Coverage Statistics

---

## Content

## 6. Test Coverage Comparison: Go vs Upstream TypeScript

_Generated 2026-05-11_

### Overview

| Metric | Go | Upstream TypeScript |
|--------|-----|---------------------|
| Test files | ~40 (excluding worktrees) | ~80+ |
| Test pattern | Primarily `testing.T` unit tests | Bun `describe/test` unit tests |
| Mocking | Minimal: simple struct stubs | Minimal: simple object stubs |
| Integration tests | Few (file I/O with `t.TempDir()`) | Few (some with real FS operations) |
| Benchmarks | Present (`BenchmarkNormalizeWhitespace`, `BenchmarkSortMapKeys`) | Absent |

---

### 6.1 Agent Loop / Sub-Agent (`agent_sub_test.go`)

**Upstream counterpart**: No direct equivalent test file exists in upstream.

| What Go tests that upstream doesn't | What upstream tests that Go doesn't |
|--------------------------------------|--------------------------------------|
| Sub-agent system prompt section presence (Environment, Permission Mode, Available Tools, Notes) | `parseToolPreset` for tool preset parsing |
| Agent type-specific identity strings (Explore="file search specialist", Plan="software architect", Verify="verification specialist") | `filterToolsByDenyRules` deny rule filtering |
| READ-ONLY MODE constraints for Explore and Plan agents | Tool `aliases` resolution (`toolMatchesName` with aliases) |
| Verify agent adversarial probing instructions | `buildTool` default property filling (isEnabled, isConcurrencySafe, isReadOnly, isDestructive) |
| Priority ordering of prompt sections | `filterToolProgressMessages` progress message filtering |
| Tool registry filtering via allowed/disallowed lists | `getEmptyToolPermissionContext` default context creation |
| Disallowed overrides allowed (disallowed_tools wins over allowed_tools) | |
| Wildcard allowed tools (`*`) | |
| Multiple disallowed overrides allowed | |

**Coverage gaps**:
- **Go**: No test for sub-agent tool execution (only prompt construction and registry filtering)
- **Upstream**: No test for sub-agent orchestration at all
- **Edge cases (Go)**: Tests that disallowed always wins over allowed even when both lists specify the same tool

**Test patterns**: Both use unit tests. Go uses `tools.NewRegistry()` with mock `testRegistryTool` structs; upstream uses `makeMinimalToolDef()` factory functions.

---

### 6.2 Exec Tool / Bash Permission Checks (`tools/exec_tool_test.go`)

**Upstream counterpart**: `src/utils/permissions/__tests__/dangerousPatterns.test.ts`, `src/utils/permissions/__tests__/shellRuleMatching.test.ts`

| What Go tests that upstream doesn't | What upstream tests that Go doesn't |
|--------------------------------------|--------------------------------------|
| `rm -rf /` and `rm -rf ~` direct denial | `CROSS_PLATFORM_CODE_EXEC` list validation (python, node, ruby, perl, npx, bunx, tsx, deno, lua, php) |
| Internal URL blocking (10.0.0.1, localhost, 192.168.1.1, 127.0.0.1) | `DANGEROUS_BASH_PATTERNS` duplicate entry detection |
| Fork bomb detection (`:(){ :|: & };:`) | `DANGEROUS_BASH_PATTERNS` includes all cross-platform patterns |
| Command substitution detection (`$(whoami)`, backtick, `<(...)`, `$((1+$(id)))`) | `permissionRuleExtractPrefix` for `:*` syntax extraction |
| Dangerous variable expansion detection (`${IFS}`, `${!VAR}`, `${BASH_VERSION}`) | `hasWildcards` with escaped/unescaped wildcards, even backslash count |
| Safe variable expansion allowlist (`$HOME`, `$USER`, `$PWD`, `$?`, `$$`, `$!`) | `matchWildcardPattern` with case-insensitive matching, regex special chars |
| Glob expansion in destructive commands (`rm *.log`, `mv *.bak`) | `parsePermissionRule` exact/prefix/wildcard classification |
| Compound command splitting and checking | `suggestionForExactCommand` and `suggestionForPrefix` suggestion generation |
| Safe wrapper stripping (timeout, nice, nohup, time, stdbuf, ionice, env, command, builtin, unbuffer) | |
| System path protection validation (`/`, `/home`, `/tmp`, `/etc`, `/usr`, etc.) | |
| Critical project file protection (`.git`, `.gitignore`, `go.mod`, `package.json`, `Cargo.toml`, `Makefile`) | |
| Windows path detection (`C:\Windows`, `C:\Program Files`) | |
| Background task execution with/without callback | |
| `run_in_background` parameter in InputSchema | |
| Deletion target extraction with `--` separator | |
| Path escape detection via `--` (`rm -- -/../secret`) | |

**Coverage gaps**:
- **Go**: Missing upstream's `CROSS_PLATFORM_CODE_EXEC` pattern testing (python, node, ruby, etc. should be blocked)
- **Go**: Missing `suggestionForExactCommand` / `suggestionForPrefix` suggestion generation tests
- **Upstream**: Missing runtime execution tests (Go tests actually execute `echo hello`, `ls`); upstream only tests pattern lists
- **Upstream**: Missing compound command splitting, wrapper stripping, and path validation tests
- **Edge cases (Go)**: Tests `$?`, `$$`, `$!`, positional parameters as safe; `${HOME:-/default}` as safe; `env FOO=bar ./script.sh` as safe wrapper

**Test patterns**: Go uses `CheckPermissions()` integration tests with actual command strings; upstream uses pattern list membership checks. Go tests are more behavioral; upstream tests are more declarative.

---

### 6.3 Normalize / Whitespace (`normalize_test.go`)

**Upstream counterpart**: No single direct equivalent. Upstream has `src/utils/__tests__/stringUtils.test.ts` and various normalization spread across other test files.

| What Go tests that upstream doesn't | What upstream tests that Go doesn't |
|--------------------------------------|--------------------------------------|
| Whitespace normalization (trailing spaces, blank line collapse to max 2) | `escapeRegExp` special character escaping |
| JSON key sorting (unsorted -> sorted) | `capitalize` first character uppercasing |
| API message normalization with Anthropic SDK types | `plural` singular/plural forms |
| Tool-use input key sorting | `firstLineOf` extraction |
| Tool-result whitespace normalization | `countCharInString` with start offset |
| `sortValueKeys` recursive map sorting | `normalizeFullWidthDigits` (full-width to half-width) |
| `sortMapKeys` nil/empty handling | `normalizeFullWidthSpace` (full-width space to half-width) |
| `NormalizeJSONBytes` invalid JSON passthrough | `safeJoinLines` with truncation |
| Benchmarks (`BenchmarkNormalizeWhitespace`, `BenchmarkSortMapKeys`) | `EndTruncatingAccumulator` state machine (truncated flag, totalBytes, clear) |
| | `truncateToLines` line-based truncation |

**Coverage gaps**:
- **Go**: Missing full-width character normalization (important for Japanese/Chinese input)
- **Go**: Missing `EndTruncatingAccumulator` pattern for streaming truncation
- **Upstream**: Missing API message normalization tests (Anthropic SDK-specific)
- **Upstream**: Missing JSON key deterministic ordering tests

**Test patterns**: Go uses `testing.T` table-driven tests with struct literals; upstream uses Bun's `describe/test` blocks. Go has benchmarks; upstream does not.

---

### 6.4 Prompt Caching (`prompt_caching_test.go`)

**Upstream counterpart**: No equivalent test file in upstream.

| What Go tests that upstream doesn't | What upstream tests that Go doesn't |
|--------------------------------------|--------------------------------------|
| Cache marker placement on system messages | N/A |
| Short message list (all get markers) vs long message list (last 3 + system) | |
| TTL field presence (5m = no ttl, 1h = has ttl) | |
| Tool role message cache_control at message level | |
| String content -> array format conversion for cache markers | |
| Array content -> cache on last block only | |
| Empty string content -> message-level cache_control | |
| `FormatCachedSystemPrompt` structured output | |
| `deepCopyMessages` mutation independence | |

**Coverage gaps**:
- **Upstream**: No prompt caching tests at all
- **Go**: No test for cache scope types (`global`, `org`) -- upstream supports these but Go only supports `ephemeral`

**Test patterns**: Go unit tests with manual map construction. No integration tests with actual API calls.

---

### 6.5 Rate Limiting (`rate_limit_test.go`)

**Upstream counterpart**: No equivalent test file in upstream.

All 15+ test functions (bucket calculations, header parsing, display formatting, progress bar) are Go-specific additions with no upstream counterpart.

---

### 6.6 Retry / Backoff (`retry_utils_test.go`)

**Upstream counterpart**: No equivalent test file in upstream.

All 6 test functions (exponential growth, max cap, overflow protection, negative attempt, custom options, deterministic mode) are Go-specific additions with no upstream counterpart.

---

### 6.7 Error Classification (`error_types_test.go`)

**Upstream counterpart**: No equivalent test file in upstream.

All 30+ test functions covering 14 error classes, sub-classifications, Chinese patterns, server disconnect heuristics, and backward compatibility wrappers are Go-specific additions with no upstream counterpart.

---

### 6.8 Compaction / Context Management (`compact_test.go`)

**Upstream counterpart**: `src/services/compact/__tests__/prompt.test.ts`, `src/services/compact/__tests__/grouping.test.ts`

| What Go tests that upstream doesn't | What upstream tests that Go doesn't |
|--------------------------------------|--------------------------------------|
| `EstimateTokens` character-based estimation | Upstream compact prompt template tests |
| `NeedsCompaction` threshold detection | Message grouping by round/conversation |
| `SmartCompact` full compaction flow | |
| `CheckReactiveCompact` token spike detection | |
| `PartialCompactUpTo` / `PartialCompactFrom` directional compaction | |
| `entriesToSummaryText` entry serialization | |
| `adjustPivotForToolPairs` tool pair boundary | |
| `estimateEntriesTokens` token counting for entries | |
| `PartialCompact` invalid direction and empty entries error handling | |

**Coverage gaps**:
- **Go**: Missing upstream's compact prompt template testing
- **Go**: Missing grouping algorithm tests
- **Upstream**: Missing reactive compaction, partial compaction, and directional compaction tests

---

### 6.9 File History / Snapshots (`filehistory_test.go`)

**Upstream counterpart**: No equivalent test file in upstream.

All 12 test functions (snapshot take/rewind/list, descriptions, multi-file, RestoreLast, RewindSteps, disk persistence, etc.) are Go-specific additions with no upstream counterpart.

---

### 6.10 Streaming (`streaming_test.go`)

**Upstream counterpart**: No equivalent test file in upstream.

All 25+ test functions (CollectHandler, StreamBus pub/sub, contextErr, FinishReason, HasPartialToolCall, HasTruncatedToolArgs, DeltasState state machine, StreamProgress, toolUseAsText detection, etc.) are Go-specific additions with no upstream counterpart.

---

### 6.11 MCP Client / Manager (`mcp/client_test.go`)

**Upstream counterpart**: `src/services/mcp/__tests__/normalization.test.ts`, `src/services/mcp/__tests__/filterUtils.test.ts`, `src/services/mcp/__tests__/envExpansion.test.ts`, `src/services/mcp/__tests__/mcpStringUtils.test.ts`, `src/services/mcp/__tests__/channelNotification.test.ts`

| What Go tests that upstream doesn't | What upstream tests that Go doesn't |
|--------------------------------------|--------------------------------------|
| Manager register/list/get status | `normalizeNameForMCP` with claude.ai prefix collapsing |
| CallTool/CallToolWithServer error paths | Header parsing (`Key: Value` format) |
| `StopAll` on empty manager | Environment variable expansion |
| Client creation with env vars | MCP string utilities |
| Concurrency safety (10 goroutines registering simultaneously) | Channel notification utilities |
| `ToolWithServer` struct | |

**Coverage gaps**:
- **Go**: Missing MCP name normalization, header parsing, env expansion, string utility, and channel notification tests
- **Upstream**: Missing concurrency tests, server status tracking, tool call error handling

---

### 6.12 Skills / Loader (`skills/loader_test.go`)

**Upstream counterpart**: No equivalent test file in upstream.

All 12 test functions (frontmatter parsing, inline list parsing, unquote, strip frontmatter, load/list/summary, always skills, context loading, etc.) are Go-specific additions with no upstream counterpart.

---

### 6.13 Permission Rule Parser (`permissions/rule_parser_test.go`)

**Upstream counterpart**: `src/utils/permissions/__tests__/permissionRuleParser.test.ts`

| What Go tests that upstream doesn't | What upstream tests that Go doesn't |
|--------------------------------------|--------------------------------------|
| `ParseRule` error handling for empty/unmatched parens | `escapeRuleContent` / `unescapeRuleContent` bidirectional testing |
| `resolveAlias` with 4 aliases (Task->Agent, KillShell->TaskStop, AgentOutputTool->TaskOutput, BashOutputTool->TaskOutput) | `permissionRuleValueFromString` / `permissionRuleValueToString` roundtrip |
| `globMatch` with prefix/suffix/contains/wildcard/question patterns | `normalizeLegacyToolName` (only Task->Agent, KillShell->TaskStop) |
| MCP server-level matching (`mcp__server1` matches `mcp__server1__tool1`) | |
| MCP wildcard matching (`mcp__server1__*`) | |
| `FormatRule` output formatting | |
| `IsToolLevel` / `ToolMatches` / `ContentMatches` predicates | |
| `ParseRules` batch parsing with behavior assignment | |

**Coverage gaps**:
- **Go**: Missing `escapeRuleContent`/`unescapeRuleContent` roundtrip testing
- **Go**: Missing `permissionRuleValueToString` serialization tests
- **Upstream**: Missing MCP server-level matching, glob matching, wildcard matching tests
- **Edge cases**: Go has 4 aliases; upstream only has 2. Go adds AgentOutputTool->TaskOutput and BashOutputTool->TaskOutput.

---

### 6.14 Path Validation / Permissions (`permissions/path_validation_test.go`)

**Upstream counterpart**: Partially `src/utils/__tests__/windowsPaths.test.ts`

| What Go tests that upstream doesn't | What upstream tests that Go doesn't |
|--------------------------------------|--------------------------------------|
| `IsInternalEditablePath` for plan files | Comprehensive Windows path handling (separators, long paths, UNC) |
| `IsInternalReadablePath` for plans/projects/tool-results/bundled-skills/session-memory | XDG directory resolution |
| Plan file .md extension check | Path manipulation utilities |
| Scratchpad/session-memory/tool-results/bundled-skills directory detection | |
| `resolvePath` absolute/relative resolution | |
| `expandTilde` tilde expansion | |
| `isUncPath` UNC path detection | |
| `hasSuspiciousWindowsPathPattern` (8.3 short names, long path prefix, DOS device, UNC) | |
| `ValidatePath` / `ValidateReadPath` with UNC, shell expansion, glob, suspicious patterns | |

**Coverage gaps**:
- **Go**: Missing upstream's comprehensive Windows path testing
- **Go**: Missing XDG directory resolution tests
- **Upstream**: Missing internal path classification (plan files, tool-results, bundled-skills, session-memory, scratchpad)
- **Upstream**: Missing `ValidatePath` operation-aware (read vs write) glob and UNC blocking tests

---

### 6.15 Tools: Registry (`tools/registry_test.go`)

**Upstream counterpart**: No direct equivalent for the Go-specific `Registry` type.

| What Go tests that upstream doesn't | What upstream tests that Go doesn't |
|--------------------------------------|--------------------------------------|
| `GetRecentlyReadFiles` with time ordering | `findToolByName` with alias resolution |
| `CheckFileStale` mtime-based staleness | `toolMatchesName` with aliases |
| `MarkFileRead` tracking | `buildTool` default property filling |

**Coverage gaps**:
- **Go**: Missing tool alias resolution tests
- **Upstream**: Missing file staleness detection and recently-read tracking tests

---

### 6.16 Tools: Various Tool Tests

Go has many tool-specific test files with no upstream counterparts:

| Go Test File | What's Tested | Upstream Equivalent |
|-------------|---------------|---------------------|
| `git_tool_test.go` | Git operations | None |
| `brief_tool_test.go` | Brief output tool | None |
| `runtime_info_test.go` | Runtime/system info | None |
| `web_search_test.go` | Web search tool | None |
| `mcp_tools_test.go` | MCP tool integration | None |
| `multi_edit_test.go` | Multi-file edit operations | None |
| `base_test.go` | Base tool interface | `src/__tests__/Tool.test.ts` |
| `agent_tools_test.go` | Agent tool registration | None |
| `agent_store_test.go` | Agent state storage | None |
| `tool_search_tool_test.go` | Tool search/discovery | None |
| `system_tool_test.go` | System operations | None |
| `file_read_magic_test.go` | File type detection by magic bytes | None |
| `todo_write_test.go` | Todo list management | None |
| `memory_tool_test.go` | Memory persistence tool | None |
| `filesystem_safety_test.go` | Filesystem safety checks | None |
| `grep_tool_test.go` | Grep/search tool | None |
| `file_edit_test.go` | File editing | None |
| `file_write_test.go` | File writing | None |
| `process_test.go` | Process management | None |
| `skill_tools_test.go` | Skill invocation tools | None |
| `task_tool_test.go` | Task management | None |
| `file_read_test.go` | File reading | None |
| `fileops_test.go` | File operations | None |
| `glob_tool_test.go` | Glob pattern matching | None |
| `web_fetch_test.go` | Web fetching | None |
| `agent_tool_test.go` | Agent delegation | None |
| `misc_tools_test.go` | Miscellaneous tools | None |
| `exa_search_test.go` | Exa search integration | None |
| `send_message_tool_test.go` | Message sending | None |
| `terminal_tool_test.go` | Terminal operations | None |
| `ask_user_question_test.go` | User interaction | None |

---

### 6.17 Major Upstream Test Files With No Go Counterpart

| Upstream Test File | What's Tested | Go Equivalent |
|-------------------|---------------|---------------|
| `CircularBuffer.test.ts` | Ring buffer data structure | None |
| `bufferedWriter.test.ts` | Buffered writing | None |
| `claudemd.test.ts` | CLAUDE.md parsing/loading | None |
| `collapseHookSummaries.test.ts` | Hook summary collapsing | None |
| `configConstants.test.ts` | Config constant validation | None |
| `contentArray.test.ts` | Content array manipulation | None |
| `cron.test.ts` | Cron scheduling | None |
| `detectRepository.test.ts` | Repository detection | None |
| `diff.test.ts` | Diff generation | None |
| `directMemberMessage.test.ts` | Direct member messaging | None |
| `displayTags.test.ts` | Display tag rendering | None |
| `envUtils.test.ts` | Environment utilities | None |
| `errors.test.ts` | Error utilities | None |
| `file.test.ts` | File utilities | None |
| `fingerprint.test.ts` | Fingerprinting | None |
| `format.test.ts` | Formatting | None |
| `frontmatterParser.test.ts` | Frontmatter parsing | Partial: `skills/loader_test.go` |
| `generators.test.ts` | Async generators | None |
| `git.test.ts` | Git utilities | Partial: `tools/git_tool_test.go` |
| `gitDiff.test.ts` | Git diff parsing | None |
| `glob.test.ts` | Glob pattern utilities | Partial: `tools/glob_tool_test.go` |
| `groupToolUses.test.ts` | Tool use grouping | None |
| `hash.test.ts` | Hashing utilities | None |
| `horizontalScroll.test.ts` | Terminal scrolling | None |
| `hyperlink.test.ts` | Hyperlink generation | None |
| `lazySchema.test.ts` | Lazy schema validation | None |
| `markdown.test.ts` | Markdown rendering | None |
| `modelCost.test.ts` | Model cost calculation | None |
| `objectGroupBy.test.ts` | Object grouping | None |
| `privacyLevel.test.ts` | Privacy level handling | None |
| `sanitization.test.ts` | Output sanitization | None |
| `semanticBoolean.test.ts` | Semantic boolean parsing | None |
| `semanticNumber.test.ts` | Semantic number parsing | None |
| `semver.test.ts` | Semver utilities | None |
| `set.test.ts` | Set utilities | None |
| `slashCommandParsing.test.ts` | Slash command parsing | None |
| `sleep.test.ts` | Sleep utilities | None |
| `stream.test.ts` | Stream utilities | None |
| `systemPrompt.test.ts` | System prompt assembly | Partial: `system_prompt_test.go` |
| `taggedId.test.ts` | Tagged ID generation | None |
| `tokenBudget.test.ts` | Token budget calculation | None |
| `userPromptKeywords.test.ts` | User prompt keyword extraction | None |
| `uuid.test.ts` | UUID generation | None |
| `windowsPaths.test.ts` | Windows path handling | Partial: `permissions/path_validation_test.go` |
| `withResolvers.test.ts` | Promise resolver pattern | None |
| `words.test.ts` | Word counting | None |
| `xdg.test.ts` | XDG directory spec | None |
| `xml.test.ts` | XML utilities | None |
| `zodToJsonSchema.test.ts` | Zod schema conversion | None |
| `notebook.test.ts` | Jupyter notebook handling | None |
| `sequential.test.ts` | Sequential processing | None |
| `textHighlighting.test.ts` | Text highlighting | None |
| `formatBriefTimestamp.test.ts` | Timestamp formatting | None |
| `lanBeacon.test.ts` | LAN beacon | None |
| `path.test.ts` | Path utilities | None |
| `peerAddress.test.ts` | Peer address | None |
| `pipePermissionRelay.test.ts` | Permission relay | None |
| `pipeTransport.test.ts` | Pipe transport | None |
| `collapseTeammateShutdowns.test.ts` | Teammate shutdown collapsing | None |
| `controlMessageCompat.test.ts` | Control message compatibility | None |
| `gitConfigParser.test.ts` | Git config parsing | None |
| `aliases.test.ts` | Model aliases | None |
| `model.test.ts` | Model selection | None |
| `providers.test.ts` | Provider configuration | None |
| `shellRuleMatching.test.ts` | Shell rule matching | Partial: `tools/exec_tool_test.go` |
| `config.test.ts` (settings) | Settings configuration | Partial: `config_test.go` |
| `bridgeMessaging.test.ts` | Bridge messaging | None |
| `bridgePermissionCallbacks.test.ts` | Bridge permission callbacks | None |
| `bridgeResultScheduling.test.ts` | Bridge result scheduling | None |
| `remoteInterruptHandling.test.ts` | Remote interrupt handling | None |
| `detached.test.ts` | Background daemon | None |
| `engine.test.ts` | Background engine | None |
| `tail.test.ts` | Background tail | None |
| `SSETransport.test.ts` | SSE transport | None |
| `proactive.baseline.test.ts` | Proactive commands | None |
| `daemon.test.ts` | Daemon commands | None |
| `job.test.ts` | Job commands | None |
| `poorMode.test.ts` | Poor mode | None |
| `parseArgs.test.ts` | Plugin argument parsing | None |
| `useMasterMonitor.test.ts` | Master monitor hook | None |
| `grouping.test.ts` | Message grouping | Partial: `compact_test.go` |
| `prompt.test.ts` (compact) | Compact prompt | None |
| `store.test.ts` | State store | None |
| `handlePromptSubmit.test.ts` | Prompt submission | None |
| `context.baseline.test.ts` | Context baseline | None |
| `commandsBridgeSafety.test.ts` | Bridge safety | None |
| `history.test.ts` | History management | None |

---

### 6.18 Summary of Cross-Cutting Patterns

**Go Test Strengths**:
1. **Behavioral security testing** -- Go's exec tool tests actually verify that dangerous commands are blocked at the permission layer, not just that patterns exist in a list
2. **Integration with real filesystem** -- `t.TempDir()` pattern provides real file I/O testing for file history, skills, and directory listing
3. **Concurrency testing** -- At least one concurrency test exists (`TestManagerConcurrency`) using goroutines
4. **Error classification exhaustiveness** -- 14 error classes with sub-classifications, Chinese pattern support, server disconnect heuristics
5. **Benchmarks** -- Go has `BenchmarkNormalizeWhitespace` and `BenchmarkSortMapKeys`; upstream has none
6. **Novel features tested** -- Prompt caching, rate limiting, retry backoff, file history, streaming bus -- all tested in Go but not in upstream

**Upstream Test Strengths**:
1. **Utility function coverage** -- 60+ utility test files covering string operations, path handling, formatting, parsing, etc.
2. **Full-width character support** -- `normalizeFullWidthDigits`, `normalizeFullWidthSpace` tests
3. **Schema validation** -- `lazySchema.test.ts`, `zodToJsonSchema.test.ts`
4. **Bridge/transport layer** -- SSE transport, bridge messaging, permission callbacks
5. **Configuration system** -- Settings, config constants, XDG directories
6. **Model management** -- Model aliases, providers, cost calculation

**Common Patterns**:
- Both sides use table-driven test approaches
- Both sides prefer pure unit tests over integration tests
- Neither side has comprehensive E2E tests
- Neither side uses heavy mocking frameworks (Go avoids interfaces; upstream avoids complex test doubles)

**Largest Gaps**:
1. **Go -> Upstream**: Go is missing ~60 upstream utility test files (CircularBuffer, diff, glob, git, markdown, model cost, sanitization, semver, token budget, UUID, XML, etc.)
2. **Upstream -> Go**: Upstream is missing all Go-specific feature tests (prompt caching, rate limiting, retry backoff, file history, streaming, error classification, sub-agent orchestration, MCP client management)
3. **Both**: Neither side tests the full agent loop end-to-end; both test individual components in isolation

---


---

## A. Coverage Statistics

| Metric | Value |
|--------|-------|
| **Total document lines** | 8,717 |
| **Top-level sections (##)** | 105 |
| **Subsections (###)** | 724 |
| **Table rows (approx. documented differences)** | 2,744 |
| **Source files compared (Go)** | ~50+ `.go` files |
| **Source files compared (Upstream TS)** | ~200+ `.ts`/`.tsx` files |
| **Section numbering range** | §1–§33 + unnumbered deep-dive parts |
| **Major comparison domains** | Streaming, Normalization, Caching, System Prompt, MCP, Testing, Agent Loop, Compaction, Tool Interface, Bash/Exec, File Read/Edit/Write, Config, Permissions, Deep Streaming/Caching/Memory/Hooks/Retry, Error Types, Progress Tracking, Async Tasks, Forked Agents, File History, Transcript, Work Tasks, Skills, Sub-Agents, Tool Implementations, Permissions Submodule, Search/Listing Tools, PostCompact Recovery, Context Management, Entry Points/Build, Hooks, Session Memory, Retry/Rate-Limit/Normalization, Error Classification, Transcript/Resume, Cache Breakpoints, Classifier, Model Routing/Cost/Background Services, Context References, File History Snapshots, Git Tool, Agent Tool, Security/Sandbox |

---


---

