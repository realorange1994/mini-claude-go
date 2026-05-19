# Tool Interface System

> Base tool interface, structured output

## Sections Included
- [##] Line 1629-1686 -- ## 8. Tool Interface & Registry (tools/base.go vs upstream Tool.ts, tools.ts)
- [##] Line 2788-2926 -- ## 26. Tool Implementations (`tools/`)
- [##] Line 8240-8537 -- ## 99. Structured Tool Output & API Parameter Deep Comparison
- [##] Line 11291-11296 -- ## 54. Tools Comparison — Write, Edit, Read, Bash, Grep, Glob, WebFetch, Agent, Notebook, Todo
- [###] Line 11605-11626 -- ### Summary Statistics

---

## Content

## 8. Tool Interface & Registry (tools/base.go vs upstream Tool.ts, tools.ts)

### Files Compared
- **Go**: `E:\Git\miniClaudeCode-go-github\tools\base.go` (549 lines)
- **Upstream**: `E:\Git\claude-code-upstream\src\Tool.ts` (801 lines), `E:\Git\claude-code-upstream\src\tools.ts`

### 8.1 Tool Interface Design
- **上游**: `buildTool()` factory pattern with Zod schemas for inputSchema and outputSchema. Tool has 40+ optional fields: `searchHint`, `validateInput`, `checkPermissions`, `backfillObservableInput`, `toAutoClassifierInput`, `renderToolUseMessage`, `renderToolUseTag`, `renderToolResultMessage`, `extractSearchText`, `renderToolUseErrorMessage`, `isConcurrencySafe`, `isReadOnly`, `isSearchOrReadCommand`, `getPath`, `getToolUseSummary`, `getActivityDescription`, `preparePermissionMatcher`, etc. (`Tool.ts` lines 1-100+)
- **Go版**: Go interface with 5 required methods: `Name()`, `Description()`, `InputSchema()`, `CheckPermissions()`, `Execute()`. Optional `ContextTool` interface adds `ExecuteContext()`. Schema is `map[string]any` instead of Zod (`base.go` lines 205-227)
- **类型**: 简化

### 8.2 Tool Schema System
- **上游**: Zod v4 schemas with `z.strictObject()`, discriminated unions for output (`z.discriminatedUnion`), `lazySchema()` for circular dependency handling. Input and output schemas are fully validated (`Tool.ts` lines 227-335, `FileReadTool.ts` lines 227-333)
- **Go版**: `map[string]any` JSON-schema-like maps. No runtime schema validation beyond `ValidateParams` which checks required fields and enums (`base.go` lines 230-273)
- **类型**: 简化

### 8.3 Tool Lifecycle Hooks
- **上游**: Extensive lifecycle: `prompt()` for dynamic prompt, `preparePermissionMatcher()` for pattern matching setup, `backfillObservableInput()` for input normalization, `renderToolUseMessage()`/`renderToolResultMessage()` for UI rendering, `extractSearchText()` for indexing, `isConcurrencySafe()`, `isReadOnly()` (`Tool.ts` lines 1-100+)
- **Go版**: No lifecycle hooks. Single `Execute()` method handles all logic. No UI rendering, no dynamic prompts, no concurrency safety markers
- **类型**: 缺失

### 8.4 File State Tracking (Registry.filesRead)
- **上游**: `FileStateCache` (LRU cache) in `src/types/fileState.ts`. Tracks `mtime`, `timestamp`, `offset`, `limit`, `content`, `isPartialView`. Used by read dedup, staleness checks, and cache edits (`FileReadTool.ts` lines 523-573)
- **Go版**: `Registry.filesRead` map with `fileReadInfo` struct. Tracks `mtime`, `readTime`, `readOffset`, `readLimit`, `content`, `isPartial`, `fromRead`. Thread-safe with `sync.RWMutex`. Similar data but simpler (no LRU eviction, no TTL) (`base.go` lines 277-363)
- **类型**: Go适配

### 8.5 Path Canonicalization
- **上游**: `expandPath()` + `normalizeCaseForComparison()` + `toPosixPath()` + `relativePath()` for cross-platform comparison. Uses `posix` module for gitignore-pattern matching. Memoized `getResolvedWorkingDirPaths` for performance (`filesystem.ts` lines 90-192, 676-744)
- **Go版**: `canonicalPath()` expands `~`, resolves to absolute, converts backslashes, lowercases. `normalizeFilePath()` uses `path.Clean` + backslash replacement + lowercase (`base.go` lines 39-49, 470-477)
- **类型**: Go适配

### 8.6 Path Allowlist (IsPathAllowed)
- **上游**: `pathInAllowedWorkingPath()` checks against multiple working directories (original CWD + `additionalWorkingDirectories`). Resolves symlinks via `getPathsForPermissionCheck()`. Handles macOS `/var` -> `/private/var` symlink resolution (`filesystem.ts` lines 683-744)
- **Go版**: `IsPathAllowed()` checks single working directory via `filepath.Rel()`. Resolves symlinks via `filepath.EvalSymlinks()`. No support for multiple working directories (`base.go` lines 481-520)
- **类型**: 简化

### 8.7 CRLF Line Ending Handling
- **上游**: `readFileSyncWithMetadata()` detects line endings (CRLF/LF/CR). `writeTextContent()` preserves original line endings. Uses `LineEndingType` enum (`fileRead.ts`, `file.ts`)
- **Go版**: `RestoreCRLF()` function converts LF to CRLF. Manual byte-level scan with `strings.Builder` for O(n) performance (`base.go` lines 524-536)
- **类型**: Go适配

### 8.8 Tool Result Metadata
- **上游**: Rich tool result with structured data types per tool (text, image, notebook, PDF, file_unchanged). Discriminated union output schema. Each tool defines its own output type (`FileReadTool.ts` lines 248-332)
- **Go版**: `ToolResult` struct with `Output` (string), `IsError` (bool), `Metadata` (`ToolResultMetadata`). Flat string output with metadata for exit code, duration, line count, truncation (`base.go` lines 59-94)
- **类型**: 简化

### 8.9 Auto-Classifier Input Conversion
- **上游**: `toAutoClassifierInput()` method on each tool converts tool input to classifier-friendly format. Bash returns command string, FileRead returns file path, etc. (`Tool.ts`, per-tool implementations)
- **Go版**: No equivalent method. Auto mode classifier receives tool name + params directly, no per-tool conversion
- **类型**: 缺失

### 8.10 Search Hint & Tool Discovery
- **上游**: `searchHint` field on each tool for natural language tool matching (e.g., "read files, images, PDFs, notebooks"). Used by tool search/discovery system (`Tool.ts` lines 1-100)
- **Go版**: `ToolSearchTool` and `BriefTool` provide tool discovery. No per-tool search hints — tools have `Name()` and `Description()` only
- **类型**: 简化

---


---

## 26. Tool Implementations (`tools/`)

### 26.1 Agent Store (`tools/agent_store.go`)
- **上游**: Task state managed via React `AppState` + `setAppStateForTasks` callback; no standalone store class
- **Go版**: `AgentTask` struct with `sync.Mutex`, fields: `ID`, `Description`, `SubagentType`, `Prompt`, `Model`, `Status`, `Output` (thread-safe buffer), `TranscriptPath`, `OutputFile`, `ToolsUsed`, `DurationMs`, `PendingMessages`, `CancelFunc`. `AgentTaskStore` with `CreateWithID()`, `Get()`, `Kill()`, `List()` methods
- **类型**: Go适配 — explicit thread-safe store; upstream uses React state

### 26.2 Agent Tools Registry (`tools/agent_tools.go`)
- **上游**: `AgentTool` registered in `tools.ts` as a single tool; agent type selection and orchestration happen within the tool's execution path
- **Go版**: `AgentTool` struct implementing `Tool` interface with `SpawnFunc` callback for dependency injection. Supports `subagentType`, `model`, `runInBackground`, `allowedTools`, `disallowedTools`, `inheritContext`, `maxTurns`, `parentMessages` parameters
- **类型**: Go适配 — callback-based spawning instead of React component lifecycle

### 26.3 AskUserQuestion (`tools/ask_user_question.go`)
- **上游**: `AskUserQuestionTool` with multiple-choice support, permission request UI rendered as React dialog; result includes selected option index
- **Go版**: `AskUserQuestionTool` with `question` and `options` parameters; returns user selection as text. No React UI — uses stdin/stdout for interactive prompting
- **类型**: 简化 — no React dialog, no option index tracking, no rich UI

### 26.4 Plan Mode (`tools/enter_plan_mode.go` + `tools/exit_plan_mode.go`)
- **上游**: `EnterPlanModeTool` and `ExitPlanModeV2Tool` with rich React UI (plan approval dialog, diff viewer, edit preview). Plan mode restricts tool set and adds confirmation step
- **Go版**: Two tools `EnterPlanModeTool`/`ExitPlanModeTool` that toggle a `planMode` flag on the `PermissionGate`. Plan mode restricts available tools (read-only + plan tools)
- **类型**: 简化 — basic flag toggle; no plan UI, no diff viewer, no approval workflow

### 26.5 Exa Search (`tools/exa_search.go`)
- **上游**: No built-in Exa search tool; external search via MCP or plugins
- **Go版**: `ExaSearchTool` with `query`, `numResults`, `useAutoprompt`, `type` (auto/keyword/neural), `category`, `startPublishedDate` parameters. Uses Exa API
- **类型**: Go增强 — no upstream equivalent; Go adds first-party Exa search

### 26.6 Process Tool (`tools/process.go`)
- **上游**: No standalone "Process" tool; process management integrated into `BashTool` and `LocalShellTask`
- **Go版**: `ProcessTool` with `action` (list/kill/attach), `pid`, `signal` parameters for OS process management
- **类型**: Go增强 — explicit process management tool

### 26.7 Runtime Info (`tools/runtime_info.go`)
- **上游**: Runtime information injected into system prompt via `enhanceSystemPromptWithEnvDetails()`; no separate tool
- **Go版**: `RuntimeInfoTool` returns OS, arch, Go version, working directory, environment info as a callable tool
- **类型**: Go增强 — LLM can query runtime info on demand

### 26.8 Send Message Tool (`tools/send_message_tool.go`)
- **上游**: No `SendMessageTool`; inter-agent messaging handled via React state updates and `TaskStore`
- **Go版**: `SendMessageTool` with `agentId` and `message` parameters; routes to `SendMessageToSubAgent()` which queues message in `AgentTask.PendingMessages`
- **类型**: Go增强 — explicit inter-agent messaging; upstream uses React state propagation

### 26.9 Skill Tools (`tools/skill_tools.go`)
- **上游**: `SkillTool` is a single tool that invokes slash commands; no search/list/read as separate tools
- **Go版**: Three tools: `ReadSkillTool`, `ListSkillsTool`, `SearchSkillsTool` (with weighted relevance scoring)
- **类型**: Go增强 — LLM-autonomous skill discovery vs user-driven slash commands

### 26.10 System Tool (`tools/system_tool.go`)
- **上游**: No explicit "SystemTool"; system-level operations (environment, shell) handled by `BashTool`
- **Go版**: `SystemTool` providing system-level operations
- **类型**: Go适配 — separates system operations from shell execution

### 26.11 Task Output Tool (`tools/task_output_tool.go`)
- **上游**: `TaskOutputTool` reads completed task output from `TaskStore`; renders via React components
- **Go版**: `TaskOutputTool` with `taskId`, `block`, `timeout` parameters; routes to `GetSubAgentOutput()` which dispatches by ID prefix (agent-/exec-/mcp-)
- **类型**: Go适配 — functionally equivalent with explicit blocking support

### 26.12 Terminal Tool (`tools/terminal_tool.go`)
- **上游**: `BashTool` is the primary terminal tool with rich features: streaming output, timeout handling, background task support, sandbox detection
- **Go版**: `TerminalTool` providing terminal/shell access, separate from `exec_tool.go`
- **类型**: 简化 — upstream BashTool is more feature-rich

### 26.13 Tool Search (`tools/tool_search_tool.go`)
- **上游**: No `ToolSearchTool`; tool descriptions are in the system prompt
- **Go版**: `ToolSearchTool` with `query` parameter; searches tool names and descriptions
- **类型**: Go增强 — LLM can discover tools dynamically

### 26.14 TodoWrite (`tools/todo_write.go`)
- **上游**: `TodoWriteTool` replaces entire todo list; items have `{ content, status, activeForm }`. Zod-validated with `TodoItemSchema` (TodoWriteTool.ts)
- **Go版**: `TodoWriteTool` with `todos` parameter (array of `{ id, subject, description, status, activeForm, owner, metadata, addBlocks, addBlockedBy }`). Integrates with `WorkTaskStore` for dependency graph management
- **类型**: Go增强 — upstream replaces entire list; Go version supports incremental updates with dependencies

### 26.15 MultiEdit (`tools/multi_edit.go`)
- **上游**: `FileEditTool` handles single edits; multiple edits achieved via multiple tool calls
- **Go版**: `MultiEditTool` applies multiple edits to the same file in a single tool call, ensuring consistent ordering and reducing round-trips
- **类型**: Go增强 — no upstream equivalent for atomic multi-edit

### 26.16 Coercion (`tools/coercion.go`)
- **上游**: Type coercion handled by Zod schemas in each tool definition; `zod` handles parsing and validation
- **Go版**: `CoerceArguments()` and `ValidateParams()` functions for converting JSON `map[string]any` types to expected Go types (float64→int, string→bool, etc.), plus `RemapDirParam()` for directory parameter renaming
- **类型**: Go适配 — replaces Zod with manual type coercion

### 26.17 Filesystem Safety (`tools/filesystem_safety.go`)
- **上游**: Safety checks distributed: path traversal prevention in `BashTool`, symlink following in `FileReadTool`, permission checks in `permissionSetup.ts`
- **Go版**: `FilesystemSafety` module with centralized path validation, traversal prevention, symlink resolution
- **类型**: Go适配 — centralized safety module vs distributed checks

### 26.18 Brief Tool (`tools/brief_tool.go`)
- **上游**: `/brief` slash command in `src/commands/brief.ts` — generates a summary of the codebase
- **Go版**: `BriefTool` as LLM-callable tool for generating codebase summaries
- **类型**: Go适配 — converts slash command to LLM tool

### 26.19 Git Tool (`tools/git_tool.go`)
- **上游**: Git operations via `BashTool` (git commands are shell commands); some git utilities in `src/utils/git/`
- **Go版**: Dedicated `GitTool` with structured git operations
- **类型**: Go增强 — first-class git tool vs ad-hoc shell commands

### 26.20 Web Tools (`tools/web_search.go` + `tools/web_fetch.go`)
- **上游**: `WebSearchTool` and `WebFetchTool` in `src/components/agents/src/tools/`. WebSearch uses Google/Bing API; WebFetch fetches and extracts content
- **Go版**: `WebSearchTool` and `WebFetchTool` with similar functionality; WebSearch can use multiple providers
- **类型**: 等价 — functionally similar

### 26.21 Memory Tool (`tools/memory_tool.go`)
- **上游**: Memory/notes managed via `sessionStorage` and `extractMemories.ts`; CLAUDE.md memory section auto-updated
- **Go版**: `MemoryTool` for reading/writing session memory files
- **类型**: 简化 — basic file-based memory vs upstream's sophisticated memory extraction pipeline

### 26.22 Task Tool (`tools/task_tool.go`)
- **上游**: Task management via React `AppState`; `TaskOutputTool` and `TaskStopTool` as separate tools
- **Go版**: `TaskTool` with actions: create, update, list, get, stop; integrates with `WorkTaskStore`
- **类型**: Go适配 — consolidates task operations into single tool

### 26.23 MCP Tools (`tools/mcp_tools.go`)
- **上游**: MCP integration via `src/services/mcp/` — tools discovered from MCP servers, registered dynamically, executed via `CallTool`
- **Go版**: `MCPToolWrapper` that wraps MCP server tools into the Go `Tool` interface for uniform execution
- **类型**: Go适配 — wrapper pattern for MCP tool integration

### 26.24 ListDir (`tools/list_dir.go`)
- **上游**: No standalone `ListDirTool`; directory listing via `BashTool` (`ls`) or `GlobTool`
- **Go版**: `ListDirTool` with `path` and `recursive` parameters; returns structured directory listing with file sizes
- **类型**: Go增强 — dedicated tool vs shell command

### 26.25 Glob Tool (`tools/glob_tool.go`)
- **上游**: `GlobTool` with `pattern` and `path` parameters; uses `globby` npm package with gitignore awareness
- **Go版**: `GlobTool` with `pattern` and `path`; uses `filepath.Glob` + custom `**` support
- **类型**: 简化 — no gitignore awareness, limited glob patterns

### 26.26 Grep Tool (`tools/grep_tool.go`)
- **上游**: `GrepTool` with `pattern`, `path`, `include`, `output_mode` (files_with_matches/content/count), `-i` flag; uses `ripgrep` via child process
- **Go版**: `GrepTool` with similar parameters; uses Go `regexp` package for in-process matching
- **类型**: 简化 — Go regex slower than ripgrep; no file type filtering, no binary file detection

### 26.27 FileOps (`tools/fileops.go`)
- **上游**: File operations via `FileWriteTool` and `FileEditTool`; no combined FileOps
- **Go版**: `FileOpsTool` combining mkdir, rm, mv, cp operations
- **类型**: Go增强 — consolidated file operations tool

---


---

## 99. Structured Tool Output & API Parameter Deep Comparison

**Date**: 2026-05-11

### Files Compared

**Upstream**:
- `src/services/api/claude.ts` — main API call path, `queryModel()`, `queryHaiku()`, `queryWithModel()`
- `src/utils/sideQuery.ts` — side query wrapper (tool_choice, output_format, temperature, thinking, stop_sequences, metadata)
- `src/utils/permissions/yoloClassifier.ts` — auto-mode classifier (tool_choice, temperature, thinking, stop_sequences)
- `src/utils/permissions/permissionExplainer.ts` — permission explainer (tool_choice for forced output)
- `src/utils/hooks/hookHelpers.ts` — `createStructuredOutputTool()`, `SyntheticOutputTool`
- `src/utils/hooks/execAgentHook.ts` — agent hook structured output enforcement
- `src/utils/hooks/execPromptHook.ts` — prompt hook with `outputFormat: { type: 'json_schema' }`
- `src/utils/sessionTitle.ts` — session title generation via `queryHaiku` with `outputFormat: { type: 'json_schema' }`
- `src/utils/teleport.tsx` — teleport session title with `outputFormat: { type: 'json_schema' }`
- `src/commands/rename/generateSessionName.ts` — rename with `outputFormat: { type: 'json_schema' }`
- `src/memdir/findRelevantMemories.ts` — memory relevance with `output_format: { type: 'json_schema' }`
- `src/main.tsx` — SDK `--json-schema` flag → `createSyntheticOutputTool()`
- `src/QueryEngine.ts` — structured output attachment handling, retry logic
- `src/services/tools/toolExecution.ts` — `structured_output` extraction from tool results
- `src/query.ts` — main loop passes `toolChoice: undefined`
- `src/services/compact/compact.ts` — compact with `toolChoice: undefined`

**Go**:
- `agent_loop.go` — main loop `buildMessageParams()`, `callWithNonStreamingFallback()`, `callWithNonStreamingNoTools()`
- `streaming.go` — streaming handler, no tool_choice in streaming calls
- `auto_classifier.go` — classifier uses `ToolChoice` for forced tool use
- `compact.go` — compact path disables thinking
- `forked_agent.go` — forked agent path, no tool_choice

---

### 99.1 tool_choice Parameter

#### Main Loop

| Aspect | Go | Upstream TS |
|--------|-----|-------------|
| **Main loop tool_choice** | **Never sent** — `buildMessageParams()` at `agent_loop.go:1801-1813` constructs `MessageNewParams` with `Model`, `MaxTokens`, `Messages`, `System`, `Tools` only. No `ToolChoice` field. | **Always sent** — `tool_choice: options.toolChoice` in `claude.ts:1769`. Main loop passes `toolChoice: undefined` from `query.ts:717`, which means the API default (`auto`) is used. |
| **Streaming path** | **Never sent** — `tryStreamOnce()` at `agent_loop.go:1689` receives params from `callWithNonStreamingFallback()` which comes from `buildMessageParams()`. No tool_choice added. | **Always sent** — streaming path uses the same `paramsFromContext()` builder which includes `tool_choice: options.toolChoice` at `claude.ts:1769`. |
| **Non-streaming fallback** | **Never sent** — `callWithNonStreamingFallback()` at `agent_loop.go:1489-1496` same param construction. | Same as streaming — uses the shared `paramsFromContext()`. |
| **Grace call (no tools)** | **No tools = no tool_choice** — `callWithNonStreamingNoTools()` at `agent_loop.go:1827-1834` sets no tools and no tool_choice. Model can only return text. | Upstream doesn't have a separate grace call path. The main loop handles max-turn exhaustion differently. |

#### Side Queries (Classifier, Compact, Permission Explainer)

| Aspect | Go | Upstream TS |
|--------|-----|-------------|
| **Classifier** | **Yes** — `auto_classifier.go:744-748` sends `ToolChoice: { OfTool: { Name: "classify_action" } }` forcing tool use. | **Yes** — `yoloClassifier.ts:1159-1162` sends `tool_choice: { type: 'tool', name: 'classify_result' }` (note: different tool name). |
| **Classifier Stage 1 (XML mode)** | **N/A** — Go always uses tool_use structured output for both stages. | **No tool_choice** — Stage 1 uses XML parsing with `stop_sequences: ['</block>']` and `temperature: 0` instead (`yoloClassifier.ts:782-795`). No `tool_choice` for Stage 1. |
| **Classifier Stage 2** | **Yes** — `auto_classifier.go:809-812` sends same forced tool choice. | **Yes** — `yoloClassifier.ts:1159-1162` uses forced tool choice `{ type: 'tool', name: 'classify_result' }`. |
| **Permission explainer** | **No** — Go doesn't have a permission explainer. | **Yes** — `permissionExplainer.ts:184` sends `tool_choice: { type: 'tool', name: 'explain_command' }` for guaranteed structured output. |
| **Compact** | **No** — `compact.go:1588-1600` sends no tool_choice (also no tools). | **No** — `compact.ts:1318` sends `toolChoice: undefined` (no tools in compact request either). |
| **Hook prompt queries** | **No** — Go doesn't have hook prompt queries. | **No** — `execPromptHook.ts:80` sends `toolChoice: undefined`. |
| **Skill improvement** | **No** — Go doesn't have skill improvement. | **No** — `skillImprovement.ts:269` sends `toolChoice: undefined`. |

**Summary**: Go sends `tool_choice` only in the auto-classifier (both stages), where it forces the `classify_action` tool. Upstream sends it additionally in the permission explainer and the classifier's thinking stage. The main loop difference is that upstream always includes `tool_choice` in the API request (even when `undefined`), while Go never includes it.

---

### 99.2 tool_choice for Forced Tool Use

| Use Case | Go | Upstream TS |
|----------|-----|-------------|
| **Auto-mode classifier** | `ToolChoice: { OfTool: { Name: "classify_action" } }` — forces model to use `classify_action` tool (`auto_classifier.go:744, 809`) | `tool_choice: { type: 'tool', name: 'classify_result' }` — forces model to use `classify_result` tool (`yoloClassifier.ts:1159`) |
| **Permission explainer** | **Not implemented** | `tool_choice: { type: 'tool', name: 'explain_command' }` — forces `explain_command` tool (`permissionExplainer.ts:184`) |
| **Agent hook structured output** | **Not implemented** | Uses `SyntheticOutputTool` with function hook enforcement (`hookHelpers.ts:70-83`), not `tool_choice`. The enforcement is via a `Stop` hook that checks `hasSuccessfulToolCall(messages, SYNTHETIC_OUTPUT_TOOL_NAME)`. |
| **SDK --json-schema** | **Not implemented** | Adds `SyntheticOutputTool` to tool array (`main.tsx:2901-2908`). Enforced via `registerStructuredOutputEnforcement()` function hook (`hookHelpers.ts:70-83`). No `tool_choice` forcing — relies on prompt + function hook. |

**Key difference**: Upstream has a richer "forced output" ecosystem:
1. `tool_choice: { type: 'tool', name: 'x' }` — API-level forced tool use (classifier, permission explainer)
2. `SyntheticOutputTool` — synthetic tool injected into tool array + function hook enforcement (SDK `--json-schema`, agent hooks)
3. `output_format: { type: 'json_schema', schema: ... }` — API-level structured output (session title, memory search, prompt hooks)

Go only has #1 (classifier). Missing #2 and #3 entirely.

---

### 99.3 Structured Output (output_format / json_schema)

| Feature | Go | Upstream TS |
|---------|-----|-------------|
| **output_format API parameter** | **Never sent** — no `output_format` or `output_config` field in any `MessageNewParams` | **Yes** — sent as `output_config.format` at `claude.ts:1782`. Requires `STRUCTURED_OUTPUTS_BETA_HEADER` (`claude.ts:1633-1638`). |
| **Structured-outputs beta header** | **Never sent** | **Yes** — `STRUCTURED_OUTPUTS_BETA_HEADER` added dynamically when `outputFormat` is set (`claude.ts:1633-1638`). Also added in `sideQuery()` (`sideQuery.ts:142-149`). |
| **Session title generation** | **No** — Go generates session IDs, not titles | **Yes** — `sessionTitle.ts:91-99` uses `outputFormat: { type: 'json_schema', schema: { title: string } }` |
| **Session rename** | **No** — Go doesn't have rename | **Yes** — `generateSessionName.ts:26-34` uses `outputFormat: { type: 'json_schema', schema: { name: string } }` |
| **Teleport title+branch** | **No** — Go doesn't have teleport | **Yes** — `teleport.tsx:164-174` uses `outputFormat: { type: 'json_schema', schema: { title: string, branch: string } }` |
| **Memory relevance search** | **No** — Go doesn't have memdir | **Yes** — `findRelevantMemories.ts:113-123` uses `output_format: { type: 'json_schema', schema: { selected_memories: string[] } }` |
| **Prompt hook output** | **No** — Go doesn't have prompt hooks | **Yes** — `execPromptHook.ts:88-99` uses `outputFormat: { type: 'json_schema', schema: { ok: boolean, reason: string } }` |
| **SDK --json-schema flag** | **Not implemented** | **Yes** — `main.tsx:2891-2927` parses user-provided JSON schema, creates `SyntheticOutputTool`, adds to tool array |
| **Agent hook output** | **Not implemented** | **Yes** — `execAgentHook.ts:89-104` creates `structuredOutputTool` via `createStructuredOutputTool()` with `{ ok: boolean, reason: string }` schema |
| **Structured output in tool results** | **Not handled** — Go only extracts text from tool results | **Yes** — `toolExecution.ts:1336-1341` detects `structured_output` in tool result, creates `structured_output` attachment |
| **Structured output in QueryEngine** | **Not handled** | **Yes** — `QueryEngine.ts:857-858` extracts `structured_output` attachments, includes in final result `QueryEngine.ts:1174`. Retry on failure with `error_max_structured_output_retries` (`QueryEngine.ts:1051`). |
| **SyntheticOutputTool exclusion** | **N/A** | **Yes** — filtered from normal tool lists at `tools.ts:312`, `constants/tools.ts:67,109`. Only added for specific contexts. |

**Gap**: Go has zero support for `output_format` / `output_config` with `json_schema`. Upstream uses this extensively for structured API responses (session titles, memory search, hooks). Go also has no `SyntheticOutputTool` equivalent for the SDK `--json-schema` feature. The `structured_output` attachment type from tool results is also unhandled.

---

### 99.4 Output Schema Enforcement / Model Response Validation

| Feature | Go | Upstream TS |
|---------|-----|-------------|
| **Tool outputSchema** | **No** — Go's `Tool` interface (`tools/base.go:81`) has no `outputSchema` field | **Yes** — `Tool.ts:408` defines `outputSchema?: z.ZodType<unknown>`. Used for parsing tool results at `CollapsedReadSearchContent.tsx:78` and `UserToolSuccessMessage.tsx:80`. |
| **MCP outputSchema** | **No** — Go doesn't convert MCP tool schemas to output schemas | **Yes** — `mcp.ts:68-91` converts `tool.outputSchema` via `zodToJsonSchema()`, enforces `type: "object"` at root level. |
| **Permission tool schema** | **No** — Go's classifier parses tool_use output manually | **Yes** — `PermissionPromptToolResultSchema.ts:75` defines zod schema for permission tool output. Parsed with `outputSchema.safeParse()` at `CollapsedReadSearchContent.tsx:78`. |
| **Function hook enforcement** | **No** — Go has no function hooks | **Yes** — `registerStructuredOutputEnforcement()` at `hookHelpers.ts:70-83` registers a `Stop` hook that checks `hasSuccessfulToolCall(messages, SYNTHETIC_OUTPUT_TOOL_NAME)`. If missing, injects a reminder message. |
| **Structured output retry** | **No** — no structured output retry | **Yes** — `QueryEngine.ts:1051` emits `error_max_structured_output_retries` subtype after exhausting retries for structured output. |

---

### 99.5 tool_choice in Plan Mode

| Feature | Go | Upstream TS |
|---------|-----|-------------|
| **Plan mode tool_choice** | **No** — Go's plan mode restricts tools by removing them from the tool list (`PermissionGate` filtering), not by `tool_choice` | **No** — Upstream also restricts plan mode by tool filtering, not `tool_choice`. Main loop always passes `toolChoice: undefined` (`query.ts:717`). |
| **Plan mode tool restriction** | Tool filtering via `DeniedPatterns` in config | Tool filtering via `getTools()` in `tools.ts` which respects mode-based filtering |

**Both codebases handle plan mode via tool filtering, not `tool_choice`.** Neither sends `tool_choice: { type: 'none' }` or similar.

---

### 99.6 tool_choice for Final Answer / Budget Exhaustion

| Feature | Go | Upstream TS |
|---------|-----|-------------|
| **Max turns exhaustion** | `callWithNonStreamingNoTools()` at `agent_loop.go:1818-1834` — sends API call with **no tools** (empty tool list), forcing text-only response. No `tool_choice` needed since no tools are provided. | No separate grace call path. Main loop handles turn budget differently. |
| **Token budget exhaustion** | Handled by iteration budget counter + grace call | Handled by `taskBudget` in `output_config.task_budget` (`claude.ts:719-723`) — sent to API so the model can pace itself. |
| **tool_choice for text-only** | **Not used** — relies on empty tool list to force text | **Not used** — upstream never sets `tool_choice: { type: 'none' }` |

**Key difference**: Go has an explicit "grace call" path that strips all tools to force a text-only final answer. Upstream doesn't need this because it uses `output_config.task_budget` for the API to pace itself, and handles budget exhaustion in the main loop.

---

### 99.7 Tool Choice Interaction with Streaming

| Feature | Go | Upstream TS |
|---------|-----|-------------|
| **Streaming with tool_choice** | **Never sent** — `tryStreamOnce()` at `agent_loop.go:1689` receives `MessageNewParams` without `ToolChoice` | **Sent** — `tool_choice` is part of the `paramsFromContext()` builder used by both streaming and non-streaming paths (`claude.ts:1769`). |
| **Streaming with output_format** | **Never sent** | **Sent** — `output_config.format` included in streaming request at `claude.ts:1782`. |
| **Impact** | No behavioral difference in practice for the main loop (both use default `auto`), but missing for side queries | Streaming path correctly includes all parameters |

**Note**: In the main loop, `toolChoice` is always `undefined`, so the effective behavior is the same (API defaults to `auto`). The difference is structural — upstream includes it in the request body (even when undefined, the field may be omitted by the SDK), while Go never constructs it.

---

### 99.8 Tool Choice Interaction with MCP Tools

| Feature | Go | Upstream TS |
|---------|-----|-------------|
| **MCP tools in tool_choice** | **N/A** — Go never sends `tool_choice` | **N/A** — Upstream never uses `tool_choice` to force MCP tools. Only used for built-in tools (classifier, permission explainer). |
| **MCP tools in tool list** | Included in `buildToolParams()` | Included via `options.mcpTools` passed to `queryModel()` |
| **MCP output schema** | **No** — Go only extracts text from MCP results | **Yes** — `mcp.ts:68-91` converts `tool.outputSchema` to JSON Schema for MCP tool registration |
| **MCP structured content** | **No** — `mcp_tools.go` only extracts text content blocks | **Yes** — `structuredContent` field preserved in tool results (`execution.ts:119-122`) |

---

### 99.9 Extended Thinking

| Feature | Go | Upstream TS |
|---------|-----|-------------|
| **Main loop thinking** | **Never configured** — `buildMessageParams()` at `agent_loop.go:1801-1813` never sets `Thinking` field. SDK default applies (which for Claude models is `enabled` with adaptive thinking). | **Configured** — `claude.ts:1647-1681` computes `thinking` based on: (1) `thinkingConfig` option, (2) `CLAUDE_CODE_DISABLE_THINKING` env, (3) `modelSupportsThinking()`, (4) `CLAUDE_CODE_DISABLE_ADAPTIVE_THINKING` env, (5) `modelSupportsAdaptiveThinking()`. Supports `adaptive` (no budget) and `enabled` (with budget_tokens). |
| **Adaptive thinking** | **Not explicitly configured** — relies on SDK default | **Yes** — `thinking = { type: 'adaptive' }` for models that support it (`claude.ts:1662-1664`). No `budget_tokens`. |
| **Budget thinking** | **Not explicitly configured** | **Yes** — `thinking = { type: 'enabled', budget_tokens: ... }` for non-adaptive models (`claude.ts:1676-1680`). Budget capped at `maxOutputTokens - 1`. |
| **Disable thinking** | **Only in compact** — `compact.go:1597-1598` sets `Thinking: { OfDisabled: ... }` | **Yes** — `{ type: 'disabled' }` used in: queryHaiku (`claude.ts:3360`), compact (`compact.ts:1309`), prompt hooks (`execPromptHook.ts:71`), agent hooks (`execAgentHook.ts:134`), skill improvement (`skillImprovement.ts:263`), API key verification (`claude.ts:557`), MCP (`mcp.ts:118`), away summary (`awaySummary.ts:65`). |
| **Always-on thinking models** | **Not handled** | **Yes** — `resolveAntModel(model)?.alwaysOnThinking` — some models reject `thinking: { type: 'disabled' }` and must always use adaptive thinking (`antModels.ts:14-15`). Classifier adapts with `getClassifierThinkingConfig()` (`yoloClassifier.ts:685-695`). |
| **Redacted thinking** | **Handled** — `streaming.go` processes `anthropic.RedactedThinkingBlock` | **Yes** — stripped from messages via `clearAllThinking` when `REDACT_THINKING_BETA_HEADER` is active. |
| **Effort level → thinking** | **Not supported** | **Yes** — `effortValue` setting controls `budget_tokens` per model. `configureEffortParams()` at `claude.ts:1614-1620`. |
| **Classifier thinking** | **Not explicitly configured** — Stage 2 adds 2048 padding to `MaxTokens` but doesn't set `Thinking` field | **Explicit** — `getClassifierThinkingConfig()` returns `[undefined, 2048]` for alwaysOnThinking models or `[false, 0]` for others (`yoloClassifier.ts:685-695`). `thinking: false` sends `{ type: 'disabled' }`. |

**Gap**: Go relies entirely on the SDK default for thinking configuration in the main loop. Upstream explicitly computes thinking config based on model capabilities, effort level, and env overrides. This means Go may send `thinking: enabled` with a default budget even when the model supports adaptive thinking (which is more efficient), or fail to disable thinking for side queries that don't need it.

---

### 99.10 Temperature

| Feature | Go | Upstream TS |
|---------|-----|-------------|
| **Main loop** | **Never set** — `buildMessageParams()` has no `Temperature` field. SDK default applies (1.0 for Claude models). | **Conditionally set** — `claude.ts:1743-1774`: only sent when thinking is disabled. When thinking enabled, temperature is omitted (API requires 1). When thinking disabled, `options.temperatureOverride ?? 1`. Main loop typically sends `temperature: 1`. |
| **Classifier** | **Never set** — `auto_classifier.go:716, 781` has no `Temperature` field. | **Explicit 0** — `yoloClassifier.ts:787, 1152` sends `temperature: 0` for deterministic classification. |
| **Prompt hooks** | **N/A** — Go doesn't have prompt hooks | **Explicit 0** — `apiQueryHookHelper.ts:102`, `skillImprovement.ts:272` send `temperatureOverride: 0`. |
| **Side queries** | **Never set** — Go doesn't use `sideQuery` | **Optional** — `sideQuery.ts:64, 239` sends `temperature` when provided. |
| **Compact** | **Never set** | **Not set** — compact sends no temperature override. |

**Gap**: Go never sets temperature. Most impactful is the classifier — upstream uses `temperature: 0` for deterministic classification decisions, while Go relies on the SDK default (1.0), making classifier decisions less deterministic.

---

### 99.11 top_p

| Feature | Go | Upstream TS |
|---------|-----|-------------|
| **top_p** | **Never set** — no `TopP` field anywhere in Go codebase | **Never set** — upstream never sends `top_p` in any API call. Only mentioned in a comment about DeepSeek ignoring it (`openai/requestBody.ts:89`). |

**No difference** — neither codebase sends `top_p`.

---

### 99.12 Stop Sequences

| Feature | Go | Upstream TS |
|---------|-----|-------------|
| **Main loop** | **Never set** — no `StopSequences` in `MessageNewParams` | **Never set** — not included in main loop params |
| **Classifier Stage 1** | **Never set** — Go uses tool_use structured output for Stage 1 | **Yes** — `yoloClassifier.ts:795` sends `stop_sequences: ['</block>']` for XML parsing in non-fast mode. Dropped in fast mode. |
| **Side queries** | **Never set** | **Optional** — `sideQuery.ts:68, 240` sends `stop_sequences` when provided. |
| **Compact** | **Never set** | **Never set** — compact doesn't use stop sequences |

**Gap**: Upstream uses `stop_sequences: ['</block>']` in the classifier's XML mode for early termination. Go doesn't need this because it uses tool_use structured output for the classifier instead of XML parsing.

---

### 99.13 Metadata

| Feature | Go | Upstream TS |
|---------|-----|-------------|
| **API metadata** | **Never sent** — `MessageNewParams` has no `Metadata` field in Go's main loop, classifier, or compact calls | **Always sent** — `metadata: getAPIMetadata()` at `claude.ts:1771` (main loop) and `claude.ts:552` (API key verification). |
| **Metadata content** | **N/A** | `getAPIMetadata()` at `claude.ts:495-519` returns `{ user_id: JSON.stringify({ device_id, account_uuid, session_id, ...CLAUDE_CODE_EXTRA_METADATA }) }`. Used for API analytics and attribution. |
| **sideQuery metadata** | **N/A** | **Not sent** — `sideQuery()` doesn't include `metadata` in its request. Only the main `queryModel()` path sends it. |

**Gap**: Go never sends `metadata` to the API. Upstream includes it for analytics, session tracking, and attribution. This means API-side analytics can't attribute Go-based sessions.

---

### 99.14 Additional API Parameters (Not in Original Request)

| Feature | Go | Upstream TS |
|---------|-----|-------------|
| **betas** | **Never sent** — no `Betas` field in `MessageNewParams` | **Sent** — `betas: filteredBetas` at `claude.ts:1770`. Includes: prompt caching, structured outputs, cache editing, AFK mode, task budgets, context management, fast mode, redact thinking. |
| **context_management** | **Sent via option** — `compact.go:1606-1611` uses `option.WithJSONSet("context_management", ...)` to send `clear_tool_uses` and `clear_thinking` edits | **Sent** — `context_management` in `paramsFromContext()` when beta is active (`claude.ts:1775-1779`). |
| **output_config** | **Never sent** — no `output_config` field | **Sent** — `output_config` at `claude.ts:1781-1783` includes: `format` (json_schema), `effort`, `task_budget`. |
| **speed** | **Never sent** | **Sent** — `speed: 'fast'` at `claude.ts:1784` when fast mode is active. |
| **max_tokens calculation** | **Simple** — `a.currentMaxTokens.Load()` which starts at config default (16384) and escalates to 64000 on max_tokens hit | **Complex** — considers `retryContext?.maxTokensOverride`, `options.maxOutputTokensOverride`, and `getMaxOutputTokensForModel()` (`claude.ts:1641-1645`). Per-model limits. |

---

### 99.15 Comprehensive API Parameter Comparison Matrix

| Parameter | Go Main Loop | Go Classifier | Go Compact | Upstream Main Loop | Upstream Classifier | Upstream Compact |
|-----------|-------------|---------------|------------|--------------------|--------------------|------------------|
| `model` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `max_tokens` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `messages` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `system` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `tools` | ✅ | ✅ (1 tool) | ❌ | ✅ | ✅ (1 tool) | ❌ |
| `tool_choice` | ❌ | ✅ forced | N/A | ✅ (undefined) | ✅ forced (stage 2) | ✅ (undefined) |
| `thinking` | ❌ (SDK default) | ❌ (SDK default) | ✅ disabled | ✅ (adaptive/budget) | ✅ (disabled/undefined) | ✅ disabled |
| `temperature` | ❌ | ❌ | ❌ | ✅ (1 or override) | ✅ (0) | ❌ |
| `stop_sequences` | ❌ | ❌ | ❌ | ❌ | ✅ (`</block>` stage 1) | ❌ |
| `metadata` | ❌ | ❌ | ❌ | ✅ | ❌ (sideQuery) | ❌ |
| `betas` | ❌ | ❌ | ❌ | ✅ | ✅ | ❌ |
| `output_config` | ❌ | ❌ | ❌ | ✅ | ❌ (sideQuery: output_format) | ❌ |
| `context_management` | ❌ | ❌ | ✅ (via option) | ✅ | ❌ | ❌ |
| `speed` | ❌ | ❌ | ❌ | ✅ (fast mode) | ❌ | ❌ |
| `cache_control` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `top_p` | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ |

---

### 99.16 Priority Assessment

| # | Difference | Severity | Impact |
|---|-----------|----------|--------|
| 1 | **No `output_format` / `json_schema` structured output** | **HIGH** | Missing SDK `--json-schema` feature, no structured API responses for session titles, memory search, hooks. Core SDK API feature gap. |
| 2 | **No thinking config in main loop** | **HIGH** | Go doesn't leverage adaptive thinking (more efficient) or model-specific thinking budgets. May waste tokens or produce lower-quality reasoning. Missing `CLAUDE_CODE_DISABLE_THINKING` support. |
| 3 | **No temperature=0 in classifier** | **MEDIUM** | Classifier decisions are non-deterministic in Go. Upstream uses `temperature: 0` for reproducible security decisions. |
| 4 | **No metadata in API calls** | **MEDIUM** | API-side analytics can't attribute Go sessions. No `device_id`, `session_id`, or `account_uuid` tracking. |
| 5 | **No betas headers** | **MEDIUM** | Missing: structured-outputs, cache-editing, AFK mode, task-budgets, context-management, fast-mode betas. Some features silently disabled. |
| 6 | **No SyntheticOutputTool** | **MEDIUM** | Missing SDK `--json-schema` output enforcement. No `structured_output` attachment handling. |
| 7 | **No output_config (effort, task_budget)** | **LOW** | Missing API-side effort control and task budget for model pacing. |
| 8 | **No stop_sequences in classifier** | **LOW** | Go uses tool_use instead of XML, so not needed. Different approach, not a gap. |
| 9 | **No speed parameter** | **LOW** | Missing fast-mode speed control. |
| 10 | **No top_p** | **NONE** | Neither codebase sends it. |

---

### 99.17 Architecture Differences

**Upstream's sideQuery pattern**: Upstream centralizes all "out-of-band" API calls through `sideQuery()` (`sideQuery.ts`), which handles:
- Fingerprint computation for OAuth
- Attribution header injection
- CLI system prompt prefix
- Model betas management
- Structured-outputs beta auto-addition
- API metadata
- Model normalization
- Temperature, thinking, stop_sequences passthrough
- tool_choice passthrough
- output_format → output_config mapping
- Langfuse observability

**Go's approach**: Each API call site (main loop, classifier, compact, forked agent) independently constructs `MessageNewParams`. No shared "side query" wrapper. This means:
- No consistent parameter handling across call sites
- Easy to miss parameters (as demonstrated — metadata, temperature, thinking all missing)
- No centralized fingerprint/attribution
- No centralized observability

---


---

## 54. Tools Comparison — Write, Edit, Read, Bash, Grep, Glob, WebFetch, Agent, Notebook, Todo

> **Methodology**: Each Go tool file was compared against its upstream TypeScript counterpart. Only features present in Go are analyzed; upstream-only features are noted as context for gaps. Line numbers are approximate. Type column: **缺失** = Go lacks upstream feature, **简化** = Go has simpler version, **Go增强** = Go adds upstream-missing feature, **Go适配** = Go adapts for Go/CLI context, **修正** = Go intentionally deviates.

---


---

### Summary Statistics

| Tool | Go Lines | Upstream Lines | Missing Features | Simplified Features | Go-Enhanced Features |
|------|----------|----------------|-----------------|--------------------|--------------------|
| File Write | 100 | 435 | 9 | 4 | 1 |
| File Edit | 414 | 626 | 8 | 5 | 0 |
| File Read | 469 | 1184 | 12 | 1 | 2 |
| Exec / Bash | 1622 | 4000+ | 6 | 3 | 6 |
| Grep | 626 | 300 | 5 | 1 | 6 |
| Glob | 184 | 199 | 5 | 2 | 3 |
| WebFetch | 395 | 319 | 7 | 0 | 5 |
| Agent | 198 | 3000+ | 12 | 2 | 4 |
| Notebook | 0 | 400 | 10 | 0 | 0 |
| TodoWrite | 224 | 116 | 5 | 1 | 2 |
| **Total** | **4232** | **~10379** | **79** | **19** | **29** |

**Key observations**:
1. **Go-enhanced features** (29) are concentrated in Exec (6), Grep (6), and WebFetch (5) — areas where the Go implementation adds practical parameters beyond upstream.
2. **Missing features** (79) cluster around infrastructure concerns: LSP integration, file history, analytics, skill discovery, VSCode notifications, and image/PDF/notebook support.
3. **Simplified features** (19) mostly involve output format (plain text vs structured schema) and error handling (hard error vs `{result: false, behavior: 'ask'}`).
4. The **Notebook tool** is completely absent in Go, with `file_edit.go` explicitly referencing it as a future need.


---

