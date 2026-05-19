# Search Tools

> Grep, glob, list_dir

## Sections Included
- [##] Line 2992-3403 -- ## 27. Deep Comparison: Search/Listing Tools (Go vs Upstream TypeScript)
- [###] Line 11442-11471 -- ### 54.5 Grep Tool
- [###] Line 11472-11496 -- ### 54.6 Glob Tool

---

## Content

## 27. Deep Comparison: Search/Listing Tools (Go vs Upstream TypeScript)

> Date: 2026-05-11
> Go files: `tools/grep_tool.go`, `tools/glob_tool.go`, `tools/list_dir.go`, `tools/web_search.go`, `tools/web_fetch.go`, `tools/git_tool.go`, `tools/memory_tool.go`, `tools/mcp_tools.go`, `tools/task_tool.go`, `tools/agent_tool.go`
> Upstream files: `packages/builtin-tools/src/tools/{GrepTool,GlobTool,WebSearchTool,WebFetchTool,MCPTool,AgentTool,TaskCreateTool,TaskUpdateTool,...}/*.ts`

---

### 27.1 GrepTool

| Aspect | Go (`grep_tool.go`) | Upstream (`GrepTool/GrepTool.ts`) |
|---|---|---|
| **Tool name** | `grep` | `Grep` |
| **Input schema: pattern** | ✅ string, required | ✅ string, required — same |
| **Input schema: path** | ✅ string, optional | ✅ string, optional — same |
| **Input schema: glob** | ✅ string, optional | ✅ string, optional — same |
| **Input schema: type** | ✅ string, optional | ✅ string, optional — same |
| **Input schema: -i** | ✅ boolean, optional | ✅ boolean (semanticBoolean), optional — same |
| **Input schema: ignore_case** | ✅ boolean, optional (alias for -i) | ❌ **Missing** — upstream has no `ignore_case` alias |
| **Input schema: case_insensitive** | ✅ boolean, optional (alias for -i) | ❌ **Missing** — upstream has no `case_insensitive` alias |
| **Input schema: fixed_strings** | ✅ boolean, optional | ❌ **Missing** — upstream has no `fixed_strings` param |
| **Input schema: output_mode** | ✅ enum [content, files_with_matches, count] | ✅ enum [content, files_with_matches, count] — same |
| **Input schema: -B** | ✅ number, optional | ✅ number (semanticNumber), optional — same |
| **Input schema: -A** | ✅ number, optional | ✅ number (semanticNumber), optional — same |
| **Input schema: -C** | ✅ number, optional | ✅ number (semanticNumber), optional — same |
| **Input schema: context** | ✅ number, optional (alias for -C) | ✅ number, optional (alias for -C) — same |
| **Input schema: context_before** | ✅ number, optional (alias for -B) | ❌ **Missing** — upstream has no `context_before` alias |
| **Input schema: context_after** | ✅ number, optional (alias for -A) | ❌ **Missing** — upstream has no `context_after` alias |
| **Input schema: -n** | ✅ boolean, optional (defaults true) | ✅ boolean (semanticBoolean), optional (defaults true) — same |
| **Input schema: multiline** | ✅ boolean, optional | ✅ boolean, optional — same |
| **Input schema: head_limit** | ✅ number, optional (default 250) | ✅ number (semanticNumber), optional (default 250) — same |
| **Input schema: offset** | ✅ number, optional (default 0) | ✅ number (semanticNumber), optional (default 0) — same |
| **Input schema: max_depth** | ✅ number, optional | ❌ **Missing** — upstream has no `max_depth` param |
| **Input schema: max_filesize** | ✅ string, optional | ❌ **Missing** — upstream has no `max_filesize` param |
| **Input schema: count_matches** | ✅ boolean (internal only) | ❌ Not in schema — upstream doesn't expose this |
| **Execution: ripgrep** | ✅ Uses `exec.Command("rg", ...)` | ✅ Uses `ripGrep()` utility — same concept |
| **Execution: native Go fallback** | ✅ **Yes** — `goSearch()` with `regexp.Regexp` | ❌ **No** — upstream requires ripgrep |
| **ripgrep: --hidden flag** | ✅ Passes `--hidden` | ✅ Passes `--hidden` — same |
| **ripgrep: --max-columns** | ✅ Passes `--max-columns 500` | ✅ Passes `--max-columns 500` — same |
| **ripgrep: VCS exclusion** | ✅ Excludes `.git`, `.svn`, `.hg`, `.bzr`, `.jj`, `.sl` | ✅ Excludes `.git`, `.svn`, `.hg`, `.bzr`, `.jj`, `.sl` — same list |
| **ripgrep: type filter** | ✅ Maps type names to extensions via `typeMap`, uses `--type-add`/`--type` | ✅ Passes `--type` directly to rg (uses rg's built-in type definitions) |
| **ripgrep: glob splitting** | ✅ `splitGlobPatterns()` — splits on commas/spaces respecting braces | ✅ Splits on commas/spaces preserving brace groups — same logic |
| **ripgrep: dash-prefixed patterns** | ✅ Uses `-e` flag | ✅ Uses `-e` flag — same |
| **ripgrep: -m (max matches)** | ❌ **Not passed to rg** — all results retrieved then sliced in Go | ❌ **Not passed** — same approach, offset+head_limit applied post-hoc |
| **Path validation** | Basic `os.Stat` check | ✅ UNC path security check, ENOENT with CWD suggestion |
| **Permission check** | Always passthrough | ✅ `checkReadPermissionForTool()` — full permission system |
| **Output: content mode** | Plain text lines, relative paths | Structured: content string, numFiles, filenames, numLines, appliedLimit, appliedOffset |
| **Output: files_with_matches mode** | Plain text lines | ✅ Sorts by **modification time** (newest first), returns structured filenames array |
| **Output: count mode** | Plain text lines | Structured: content string, numFiles, numMatches |
| **Output: pagination format** | `(showing first N matches, truncated)` | `[Showing results with pagination = limit: N, offset: M]` |
| **Output: no matches** | `No matches found.` or `No matches found. (Searched N files)` | `No files found` / `No matches found` — different wording |
| **Native Go search: binary skip** | ✅ Skips `.exe`, `.dll`, `.so`, `.bin` | N/A — upstream always uses ripgrep |
| **Native Go search: glob matching** | ✅ `filepath.Match()` — single pattern only | N/A |
| **Native Go search: context lines** | ✅ Supports with `>>>` prefix markers | N/A |
| **Zod validation** | ❌ No schema validation | ✅ `z.strictObject()` with semantic types |
| **Error handling** | Returns `ToolResult{IsError: true}` | `ValidationResult` with error codes and messages |
| **ignore patterns from config** | ❌ Not supported | ✅ `getFileReadIgnorePatterns()` + `normalizePatternsToPath()` |
| **plugin cache exclusion** | ❌ Not supported | ✅ `getGlobExclusionsForPluginCache()` |

**Key Differences:**
1. **Go has native fallback** when ripgrep is unavailable; upstream requires ripgrep.
2. **Go has extra parameters**: `ignore_case`, `case_insensitive`, `fixed_strings`, `context_before`, `context_after`, `max_depth`, `max_filesize`.
3. **Upstream has structured output** with typed fields (numFiles, filenames, appliedLimit); Go returns plain text.
4. **Upstream has full permission system** integration; Go always passes through.
5. **Upstream has path validation** with ENOENT suggestions and UNC security checks.
6. **Upstream sorts files_with_matches by modification time**; Go native fallback doesn't sort.
7. **Upstream type filter uses rg's built-in `--type`** directly; Go maps type names to extensions manually via `typeMap`.
8. **Upstream respects gitignore and plugin cache exclusions**; Go doesn't.

---

### 27.2 GlobTool

| Aspect | Go (`glob_tool.go`) | Upstream (`GlobTool/GlobTool.ts`) |
|---|---|---|
| **Tool name** | `glob` | `Glob` |
| **Input schema: pattern** | ✅ string, required | ✅ string, required — same |
| **Input schema: path** | ✅ string, optional | ✅ string, optional — same |
| **Input schema: head_limit** | ✅ number, optional (default 100) | ❌ **Missing from schema** — limit handled internally via `globLimits` context |
| **Input schema: excludes** | ✅ array of strings, optional | ❌ **Missing** — upstream has no `excludes` param |
| **Input schema: directory** | ✅ Legacy alias for path | ❌ **Missing** — no legacy alias |
| **Execution** | `filepath.WalkDir()` with `doublestar.Match()` | `glob()` utility (likely ripgrep-based glob) |
| **Auto-prefix `**/`** | ✅ When pattern has no slash | Same behavior (handled in glob utility) |
| **Sort by modification time** | ✅ Sorts by `ModTime().Unix()` (oldest first) | ✅ Sorts by mtimeMs (newest first) — **opposite order!** |
| **Output format** | Plain text, one path per line | Structured: filenames array, durationMs, numFiles, truncated |
| **Truncation message** | `(showing first N of M matches)` | `(Results are truncated. Consider using a more specific path or pattern.)` |
| **Excludes filtering** | ✅ `filepath.Match()` on dir names and relative paths | ❌ No excludes parameter — handled by gitignore and permission context |
| **UNC path security** | ✅ `isUncPath()` check | ✅ UNC path security check |
| **Permission check** | Always passthrough | ✅ `checkReadPermissionForTool()` |
| **Path validation** | Basic `os.Stat` + IsDir check | ✅ ENOENT with CWD suggestion, directory type check |
| **Zod validation** | ❌ No schema validation | ✅ `z.strictObject()` |
| **Max results default** | 100 | 100 (via `globLimits.maxResults`) — same |
| **gitignore support** | ❌ No gitignore — only hardcoded `isIgnoredDir()` | ✅ Respects gitignore via glob utility |
| **Plugin cache exclusion** | ❌ Not supported | ✅ Via glob utility |

**Key Differences:**
1. **Go has `excludes` parameter**; upstream doesn't expose it.
2. **Go has `head_limit` in schema**; upstream handles it via context/glob utility.
3. **Sort order is opposite**: Go sorts oldest-first, upstream sorts newest-first.
4. **Go uses `doublestar` library** for glob matching; upstream uses its own glob utility (likely ripgrep-backed).
5. **Upstream has gitignore support**; Go only has hardcoded ignored directory list.
6. **Go has `directory` legacy alias** for `path`.

---

### 27.3 ListDirTool

| Aspect | Go (`list_dir.go`) | Upstream |
|---|---|---|
| **Exists in upstream?** | ✅ Yes | ❌ **No equivalent tool** — upstream has no `ListDir` tool |
| **Tool name** | `list_dir` | N/A |
| **Parameters** | `path`, `recursive`, `max_entries` (default 200) | N/A |
| **Output** | Directories with `/` suffix, files without | N/A |
| **Ignored dirs** | Same `isIgnoredDir()` list as GlobTool | N/A |
| **Implementation** | `os.Open` + `Readdirnames` (simple) / `filepath.Walk` (recursive) | N/A |
| **Upstream equivalent** | N/A | Upstream uses `BashTool` with `ls` or `GlobTool` |

**Key Differences:**
1. **ListDirTool is Go-only** — no upstream equivalent. Upstream users rely on BashTool (`ls`) or GlobTool.

---

### 27.4 WebSearchTool

| Aspect | Go (`web_search.go`) | Upstream (`WebSearchTool/WebSearchTool.ts`) |
|---|---|---|
| **Tool name** | `web_search_scraper` | `WebSearch` |
| **Description** | "Search the web using Bing/360 HTML scraping" | "search the web for current information" |
| **Input schema: query** | ✅ string, required | ✅ string, required, min 2 chars |
| **Input schema: count** | ✅ number, optional (1-10, default 10) | ❌ **Missing** — replaced by `num_results` |
| **Input schema: num_results** | ❌ **Missing** | ✅ number, optional (default 8) |
| **Input schema: allowed_domains** | ❌ **Missing** | ✅ array of strings, optional |
| **Input schema: blocked_domains** | ❌ **Missing** | ✅ array of strings, optional |
| **Input schema: livecrawl** | ❌ **Missing** | ✅ enum [fallback, preferred], optional |
| **Input schema: search_type** | ❌ **Missing** | ✅ enum [auto, fast, deep], optional |
| **Input schema: context_max_characters** | ❌ **Missing** | ✅ number, optional |
| **Execution** | **HTML scraping** of Bing.com (with 360.so fallback for China) | **API-based** via `createAdapter()` (server-side search or Bing fallback) |
| **Search backend** | Direct HTTP GET + regex parsing of Bing HTML | Adapter pattern: server-side API search or Bing API |
| **Proxy support** | ✅ `HTTP_PROXY` environment variable | ❌ No proxy config in schema |
| **Compressed response handling** | ✅ gzip/deflate decompression | Handled by adapter |
| **Bing redirect URL resolution** | ✅ `resolveBingURL()` with base64 decoding | N/A — API handles redirects |
| **360 search fallback** | ✅ `search360()` for China | ❌ **No** — upstream uses API-based search |
| **Last-resort link extraction** | ✅ `extractAnyLinks()` from any HTML | ❌ No equivalent |
| **Permission check** | ✅ Blocks internal URLs via `containsInternalURL()` | ✅ Passthrough with permission suggestions |
| **Input validation** | Basic empty check | ✅ Zod + mutual exclusion of allowed/blocked_domains |
| **Output format** | Plain text: numbered list with title, URL, snippet | Structured: query, results array, durationSeconds |
| **Output: source links** | Plain text URLs | Markdown hyperlinks `[Title](URL)` |
| **Output: sources reminder** | ❌ No reminder | ✅ MANDATORY sources reminder appended |
| **Progress events** | ❌ No progress | ✅ `onProgress` callback for streaming progress |
| **Abort support** | ✅ Via context timeout (30s) | ✅ Via `abortController.signal` |

**Key Differences:**
1. **Go uses HTML scraping**; upstream uses API-based search (much more reliable).
2. **Go has China fallback** (360.so); upstream has no region-specific fallback.
3. **Upstream has domain filtering** (allowed/blocked); Go doesn't.
4. **Upstream has livecrawl, search_type, context_max_characters**; Go doesn't.
5. **Go has proxy support**; upstream doesn't expose it.
6. **Upstream mandates source citation** in output; Go doesn't.
7. **Go is fundamentally different in approach** — scraping vs API.

---

### 27.5 WebFetchTool

| Aspect | Go (`web_fetch.go`) | Upstream (`WebFetchTool/WebFetchTool.ts`) |
|---|---|---|
| **Tool name** | `web_fetch` | `WebFetch` |
| **Input schema: url** | ✅ string, required | ✅ string (z.string().url()), required |
| **Input schema: extractMode** | ✅ enum [text, markdown, json] | ❌ **Missing** — upstream doesn't have extractMode |
| **Input schema: prompt** | ❌ **Missing** | ✅ **Required** — upstream requires a prompt to process content |
| **Execution** | Direct HTTP GET + HTML stripping | HTTP GET → markdown conversion → **LLM processing with prompt** |
| **Content extraction** | Custom `extractTextFromHTML()` / `stripHTMLSimple()` | `getURLMarkdownContent()` → `applyPromptToMarkdown()` (LLM-based) |
| **LLM processing** | ❌ No LLM — raw text extraction only | ✅ **Yes** — uses small model to process content with user prompt |
| **Proxy support** | ✅ `HTTP_PROXY` environment variable | ❌ No proxy config in schema |
| **Compressed response** | ✅ gzip/deflate decompression | Handled internally |
| **Max body size** | 1MB (`maxBodySize`) | Handled by `MAX_MARKDOWN_LENGTH` |
| **Truncation** | Splits at midpoint with truncation notice | Via LLM summarization |
| **Title extraction** | ✅ `extractHTMLTitle()` from `<title>` or `<h1>` | N/A — content processed by LLM |
| **Meta description** | ✅ `extractHTMLMeta()` from `<meta name="description">` | N/A — content processed by LLM |
| **JSON output mode** | ✅ `extractMode: "json"` returns structured JSON | ❌ No JSON output mode |
| **Text output mode** | ✅ `extractMode: "text"` strips all HTML | ❌ No text mode — always markdown |
| **Redirect handling** | ❌ Returns HTTP error for non-200 | ✅ **Detects cross-host redirects** and returns redirect info with instructions |
| **Preapproved hosts** | ❌ No concept of preapproved hosts | ✅ `isPreapprovedHost()` — skips LLM processing for trusted domains |
| **Binary content handling** | ❌ No binary handling | ✅ Saves binary content (PDFs, etc.) to disk with note |
| **Permission check** | ✅ Blocks `file://` and internal URLs | ✅ Full permission system with host-based rules, preapproved hosts |
| **file:// URL blocking** | ✅ In CheckPermissions | ✅ Via URL validation (z.string().url() rejects file://) |
| **Auth warning** | ❌ No warning | ✅ Warns about authenticated/private URLs in prompt |
| **Cache** | ❌ No cache | ✅ 15-minute self-cleaning cache |
| **GitHub URL preference** | ❌ No guidance | ✅ Recommends `gh` CLI for GitHub URLs |

**Key Differences:**
1. **Go does raw text extraction**; upstream uses an LLM to process content with a user-provided prompt.
2. **Upstream requires a `prompt` parameter**; Go doesn't have one.
3. **Go has `extractMode`** (text/markdown/json); upstream doesn't.
4. **Upstream handles redirects** explicitly; Go doesn't.
5. **Upstream has preapproved hosts** and binary content handling; Go doesn't.
6. **Go has proxy support**; upstream doesn't expose it.
7. **Fundamentally different architecture**: Go = fetch+strip, Upstream = fetch+convert+LLM-process.

---

### 27.6 GitTool

| Aspect | Go (`git_tool.go`) | Upstream |
|---|---|---|
| **Exists in upstream?** | ✅ Yes | ❌ **No standalone GitTool** — upstream delegates to BashTool with git safety |
| **Tool name** | `git` | N/A |
| **Operations** | 35+ git operations + `gh` CLI + `info` | N/A — upstream uses `BashTool` with git commands |
| **Flag validation** | ✅ Per-subcommand whitelist (`gitFlagConfig`) | Upstream: `BashTool/bashSecurity.ts` + `PowerShellTool/gitSafety.ts` |
| **Dangerous operation blocking** | ✅ `isDangerousOperation()` — blocks `reset --hard`, `push --force`, `clean -f`, `branch -D` | Upstream: `destructiveCommandWarning.ts` + git safety rules in BashTool |
| **GitHub CLI (gh) support** | ✅ Built-in with `gh_command` param + flag whitelist | Upstream: via BashTool with gh commands |
| **gh safety: flag whitelist** | ✅ `GHSafeFlags` map with per-subcommand validation | N/A |
| **gh safety: dangerous repo check** | ✅ `ghIsDangerousRepo()` blocks URLs/SSH/multi-slash | N/A |
| **Proxy support** | ✅ `proxy` parameter sets `https_proxy`/`http_proxy` env vars | N/A |
| **Git info operation** | ✅ `executeGitInfo()` returns structured repo state | N/A |
| **Git utility functions** | ✅ `FindGitRoot()`, `GetBranch()`, `IsDirty()`, etc. | Upstream: scattered across various utils |
| **`GetGitContext()`** | ✅ Returns short context string for system prompt | Upstream: similar but different implementation |

**Key Differences:**
1. **Go has a dedicated GitTool**; upstream uses BashTool with git safety rules.
2. **Go has explicit per-operation parameter mapping** (branch, message, files, etc.); upstream passes raw commands.
3. **Go has structured gh CLI support** with flag whitelist; upstream delegates to BashTool.
4. **Go has proxy support** for git operations; upstream doesn't.
5. **Go's `info` operation** provides a structured summary of repo state; upstream doesn't have this as a tool.

---

### 27.7 MemoryTool

| Aspect | Go (`memory_tool.go`) | Upstream |
|---|---|---|
| **Exists in upstream?** | ✅ Yes | ❌ **No equivalent** — upstream uses TodoWriteTool + session storage |
| **Tool names** | `memory_add`, `memory_search` | N/A |
| **Categories** | `preference`, `decision`, `state`, `reference` | N/A |
| **Implementation** | Callback-based (`MemoryAddCallback`, `MemorySearchCallback`) | N/A |
| **Upstream equivalent** | N/A | Upstream: `TodoWriteTool` for task tracking, `sessionStorage` for persistent data, `AgentMemory` for agent-specific memory |

**Key Differences:**
1. **Go has dedicated memory tools**; upstream doesn't.
2. **Upstream has TodoWriteTool** (task-focused) and session storage instead.
3. **Go's memory tool is simpler** — just category/content pairs with text search.
4. **Upstream's AgentMemory** (in `AgentTool/agentMemory.ts`) is more sophisticated but only available within agents.

---

### 27.8 MCPTool

| Aspect | Go (`mcp_tools.go`) | Upstream (`MCPTool/MCPTool.ts`) |
|---|---|---|
| **Tool names** | `list_mcp_tools`, `mcp_call_tool`, `mcp_server_status` | `mcp` (single dynamic tool) |
| **Approach** | **3 separate tools** with explicit schemas | **1 dynamic tool** — name/schema overridden per MCP server tool |
| **list_mcp_tools** | ✅ Dedicated tool with `server`/`pattern` filters | ❌ No equivalent — tools discovered dynamically |
| **mcp_call_tool params** | `server`, `tool`, `arguments`, `timeout`, `run_in_background` | Dynamic — schema comes from MCP server tool definition |
| **Timeout handling** | ✅ Configurable (1s-600s, default 30s) with goroutine + timer | ❌ Not in tool schema — handled by MCP client |
| **Background execution** | ✅ `run_in_background` param + `MCPTimeoutCallback` | ❌ No background mode |
| **Timeout → background conversion** | ✅ On timeout, registers as background task with task ID | ❌ Not supported |
| **mcp_server_status** | ✅ Dedicated tool showing connection status per server | ❌ No equivalent |
| **Permission check** | Always passthrough | Passthrough with permission suggestions |
| **Output format** | Plain text | String |
| **Dynamic schema** | ❌ Static schema for all MCP calls | ✅ **Overridden per MCP tool** in `mcpClient.ts` |
| **Tool registration** | ✅ 3 static tools | ✅ 1 dynamic tool registered per MCP server tool |

**Key Differences:**
1. **Go uses 3 separate MCP tools**; upstream uses 1 dynamic tool per MCP server tool.
2. **Upstream dynamically overrides** name/schema per MCP tool; Go uses static schemas.
3. **Go has timeout with background conversion**; upstream doesn't.
4. **Go has `list_mcp_tools` and `mcp_server_status`**; upstream doesn't expose these.
5. **Upstream's approach is more model-friendly** — each MCP tool appears as a first-class tool with its own schema.

---

### 27.9 TaskTool

| Aspect | Go (`task_tool.go`) | Upstream (`TaskCreateTool`, `TaskUpdateTool`, etc.) |
|---|---|---|
| **Tool names** | `task_create`, `task_list`, `task_get`, `task_update`, `task_stop` | `TaskCreate`, `TaskList`, `TaskGet`, `TaskUpdate`, `TaskOutput`, `TaskStop` |
| **Naming convention** | snake_case | PascalCase |
| **TaskCreate: subject** | ✅ string, required | ✅ string, required — same |
| **TaskCreate: description** | ✅ string, required | ✅ string, required — same |
| **TaskCreate: activeForm** | ✅ string, optional | ✅ string, optional — same |
| **TaskCreate: metadata** | ✅ object, optional | ✅ `z.record(z.string(), z.unknown())`, optional — same |
| **TaskUpdate: task_id** | ✅ string, required | ✅ string (`taskId`), required — camelCase in upstream |
| **TaskUpdate: status** | ✅ enum [pending, in_progress, completed, deleted] | ✅ same enum — same |
| **TaskUpdate: addBlocks** | ✅ array of strings | ✅ array of strings — same |
| **TaskUpdate: addBlockedBy** | ✅ array of strings | ✅ array of strings — same |
| **TaskUpdate: metadata** | ✅ object (merge keys, null to delete) | ✅ Same behavior — merge with null deletion |
| **TaskUpdate: owner** | ✅ string, optional | ✅ string, optional — same |
| **TaskUpdate: verification nudge** | ❌ **Missing** | ✅ Suggests verification agent when 3+ tasks close without verification |
| **TaskUpdate: hooks** | ❌ **Missing** | ✅ `executeTaskCompletedHooks()` — blocking hooks can prevent completion |
| **TaskUpdate: mailbox notification** | ❌ **Missing** | ✅ Notifies new owner via mailbox when ownership changes |
| **TaskUpdate: auto-owner** | ❌ **Missing** | ✅ Auto-sets owner when teammate marks task in_progress |
| **TaskList output** | Plain text table | Structured output |
| **TaskGet output** | Plain text with labels | Structured output |
| **TaskOutput tool** | ❌ **Missing** | ✅ `TaskOutputTool` — wait for background task output |
| **TaskStop** | ✅ `task_stop` — stop background task | ✅ `TaskStop` — same |
| **Callback-based** | ✅ `WorkTaskCreateFunc`, etc. | ❌ Uses `createTask()`/`updateTask()` from `src/utils/tasks.js` |
| **Storage** | In-memory (via callbacks) | File-based (`src/utils/tasks.js`) |
| **Scalar→array coercion** | ✅ `add_blocks`/`add_blocked_by` coerces scalars to arrays | ❌ Not needed — Zod enforces array type |

**Key Differences:**
1. **Go uses callbacks**; upstream uses file-based task storage.
2. **Upstream has TaskOutputTool** for waiting on background tasks; Go doesn't.
3. **Upstream has verification nudge** and task completion hooks; Go doesn't.
4. **Upstream has mailbox notification** for owner changes; Go doesn't.
5. **Go has scalar→array coercion** for `add_blocks`/`add_blocked_by`; upstream uses Zod validation.
6. **Naming: snake_case vs PascalCase** for both tool names and parameter names.

---

### 27.10 AgentTool

| Aspect | Go (`agent_tool.go`) | Upstream (`AgentTool/AgentTool.tsx`) |
|---|---|---|
| **Tool name** | `agent` | `Agent` (with `SubAgent` legacy alias) |
| **Input schema: description** | ✅ string, required | ✅ string, required — same |
| **Input schema: prompt** | ✅ string, required | ✅ string, required — same |
| **Input schema: subagent_type** | ✅ string, optional | ✅ string, optional — same |
| **Input schema: model** | ✅ enum [sonnet, opus, haiku] | ✅ enum [sonnet, opus, haiku] — same |
| **Input schema: run_in_background** | ✅ boolean — marked DEPRECATED | ✅ boolean, optional — still functional in upstream |
| **Input schema: allowed_tools** | ✅ array of strings | ❌ **Missing** — upstream doesn't expose tool filtering |
| **Input schema: disallowed_tools** | ✅ array of strings | ❌ **Missing** — upstream doesn't expose tool filtering |
| **Input schema: inherit_context** | ✅ boolean (fork mode) | ❌ **Missing** — upstream uses fork experiment gate instead |
| **Input schema: max_turns** | ✅ number (default 200) | ❌ **Missing** — upstream doesn't expose turn limit |
| **Input schema: timeout** | ✅ number (ms, default 600000) | ❌ **Missing** — upstream doesn't expose timeout |
| **Input schema: name** | ❌ **Missing** | ✅ For multi-agent (SendMessage routing) |
| **Input schema: team_name** | ❌ **Missing** | ✅ For agent teams |
| **Input schema: mode** | ❌ **Missing** | ✅ Permission mode for spawned teammate |
| **Input schema: isolation** | ❌ **Missing** | ✅ `worktree` / `remote` isolation modes |
| **Input schema: cwd** | ❌ **Missing** | ✅ CWD override for agent |
| **Agent spawning** | Always background via `SpawnFunc` callback | Both sync and async, with background conversion |
| **Fork subagent** | ✅ Via `inherit_context` parameter | ✅ Via fork experiment gate (`isForkSubagentEnabled()`) |
| **Worktree isolation** | ❌ **Missing** | ✅ `createAgentWorktree()` — git worktree per agent |
| **Remote agent** | ❌ **Missing** | ✅ `teleportToRemote()` for CCR environments |
| **Multi-agent teams** | ❌ **Missing** | ✅ `spawnTeammate()` with tmux/mailbox |
| **Agent summarization** | ❌ **Missing** | ✅ `startAgentSummarization()` for long-running agents |
| **Progress tracking** | ❌ **Missing** | ✅ `createProgressTracker()` with token/tool counting |
| **Auto-background** | ❌ Always background | ✅ After threshold or GrowthBook gate |
| **Recursive spawn prevention** | ✅ Always disallows `agent` in `disallowedTools` | ✅ Fork guard + message scanning |
| **Permission check** | Always passthrough | ✅ Mode-dependent — auto mode routes through classifier |
| **Output: async_launched** | ✅ agentId, status, output_file, description | ✅ agentId, description, prompt, outputFile, canReadOutputFile |
| **Output: completed** | Via `formatAgentResult()` with usage footer | ✅ Structured with agentId, totalTokens, totalToolUseCount, totalDurationMs |
| **Output: teammate_spawned** | ❌ **Missing** | ✅ tmux_session_name, name, team_name, etc. |
| **Output: remote_launched** | ❌ **Missing** | ✅ taskId, sessionUrl, outputFile |
| **MCP server requirements** | ❌ **Missing** | ✅ `requiredMcpServers` with polling wait |
| **Agent color management** | ❌ **Missing** | ✅ `setAgentColor()` for UI |
| **Handoff classification** | ❌ **Missing** | ✅ `classifyHandoffIfNeeded()` for transcript classifier |
| **Agent definitions** | ❌ **Missing** — no agent definition system | ✅ `loadAgentsDir()` — loads from filesystem |
| **Agent memory** | ❌ **Missing** | ✅ `AgentTool/agentMemory.ts` with scope-based memory |

**Key Differences:**
1. **Go is much simpler** — always runs agents in background, no sync mode.
2. **Go has `allowed_tools`/`disallowed_tools`/`max_turns`/`timeout`**; upstream doesn't expose these.
3. **Upstream has multi-agent teams**, worktree isolation, remote agents; Go doesn't.
4. **Upstream has agent summarization** and progress tracking; Go doesn't.
5. **Go has `inherit_context`** as explicit parameter; upstream uses experiment gate.
6. **Upstream is ~1800 lines** with React UI; Go is ~200 lines with callback-based spawning.
7. **Upstream has agent definition system** loaded from filesystem; Go has no such concept.

---

### 27.11 Cross-Cutting Observations

#### Parameters that Go has but upstream doesn't:
- **GrepTool**: `ignore_case`, `case_insensitive`, `fixed_strings`, `context_before`, `context_after`, `max_depth`, `max_filesize`
- **GlobTool**: `excludes`, `head_limit` (in schema), `directory` (legacy alias)
- **WebSearchTool**: `count` (replaced by `num_results`)
- **WebFetchTool**: `extractMode`
- **AgentTool**: `allowed_tools`, `disallowed_tools`, `max_turns`, `timeout`, `inherit_context`

#### Parameters that upstream has but Go doesn't:
- **WebSearchTool**: `allowed_domains`, `blocked_domains`, `livecrawl`, `search_type`, `context_max_characters`
- **WebFetchTool**: `prompt` (required!)
- **AgentTool**: `name`, `team_name`, `mode`, `isolation`, `cwd`
- **MCPTool**: Dynamic per-tool schemas

#### Tools that Go has but upstream doesn't:
- `list_dir` — no upstream equivalent (uses BashTool/GlobTool)
- `memory_add` / `memory_search` — no upstream equivalent
- `mcp_server_status` / `list_mcp_tools` — no upstream equivalent (dynamically registered)
- `git` — no upstream equivalent (uses BashTool with git safety)

#### Tools that upstream has but Go doesn't:
- `TaskOutput` — wait for background task completion
- `SendMessage` — inter-agent communication
- `EnterPlanMode` / `ExitPlanMode` — plan mode tools
- `EnterWorktree` / `ExitWorktree` — worktree tools
- `TodoWrite` — V1 task list
- `WebBrowser` — browser tool
- `LSPTool` — language server protocol tool
- `BriefTool` — file attachment/upload tool
- `ConfigTool` — configuration management
- `ToolSearch` — tool discovery
- `ScheduleCron` — cron scheduling
- `SkillTool` — skill execution
- Various team/teammate/monitor tools

#### Architecture Patterns:
| Pattern | Go | Upstream |
|---|---|---|
| **Schema validation** | Manual type assertions (`v.(type)`) | Zod `z.strictObject()` with semantic types |
| **Output format** | Plain text strings | Structured typed objects with Zod schemas |
| **Permission system** | Always passthrough or simple block | Full permission framework with rules, suggestions, and modes |
| **Error handling** | `ToolResult{IsError: true}` | `ValidationResult` with error codes, messages, CWD suggestions |
| **Tool registration** | Static list in code | Dynamic via `buildTool()` with feature flags |
| **UI rendering** | Plain text only | React JSX components |
| **Type safety** | Runtime type assertions | Compile-time Zod + TypeScript |
| **Proxy support** | HTTP_PROXY env vars | No proxy configuration |
| **Native fallback** | Yes (GrepTool without rg) | No — requires ripgrep |
| **Region adaptation** | Yes (360.so for China) | No |

---


---

### 54.5 Grep Tool

**Go**: `tools/grep_tool.go` (626 lines) · **Upstream**: `GrepTool/GrepTool.ts` (~300 lines)

| # | Aspect | Go (file:line) | Upstream (file:line) | Type |
|---|--------|----------------|----------------------|------|
| 1 | **Tool name** | `"grep"` (line 18) | Search-related name via `GREP_TOOL_NAME` | Go适配 |
| 2 | **Input schema** | `pattern`, `path`, `glob`, `type`, `-i`, `ignore_case`, `case_insensitive`, `fixed_strings`, `output_mode`, `-B`, `-A`, `-C`, `context`, `context_before`, `context_after`, `-n`, `multiline`, `max_depth`, `max_filesize`, `head_limit`, `offset` (line 26-118) | `pattern`, `path`, `glob`, `output_mode`, `-B`, `-A`, `-C`, `context`, `-n`, `-i`, `type`, `head_limit`, `offset`, `multiline` (line 33-90) | Go增强 |
| 3 | **Dual-mode (rg / native)** | Ripgrep if available, Go regexp fallback (line 227-230) | Always uses ripgrep via `ripGrep()` utility | Go增强 |
| 4 | **Legacy parameter aliases** | `ignore_case`, `case_insensitive`, `context_before`, `context_after`, `count_matches` (line 51-91) | No legacy aliases — only official names | Go增强 |
| 5 | **fixed_strings** | Supported — adds `-F` to rg or `regexp.QuoteMeta` (line 149, 311) | Not a schema parameter | Go增强 |
| 6 | **max_depth** | Supported — adds `--max-depth` to rg, limits `filepath.WalkDir` (line 99, 179-188, 330-332) | Not a parameter — handled by rg defaults | Go增强 |
| 7 | **max_filesize** | Supported — adds `--max-filesize` to rg (line 100-101, 191, 334-336) | Not a parameter | Go增强 |
| 8 | **head_limit default** | 250 `maxGrepMatches` (line 12) | 250 `DEFAULT_HEAD_LIMIT` (line 108) | ✅ Match |
| 9 | **head_limit=0 unlimited** | Treated as unlimited (line 175-177) | Same — `limit === 0` = unlimited (line 116-118) | ✅ Match |
| 10 | **VCS directory exclusion** | `.git`, `.svn`, `.hg`, `.bzr`, `.jj`, `.sl` (line 294) | Same list via `VCS_DIRECTORIES_TO_EXCLUDE` (line 95-102) | ✅ Match |
| 11 | **Type filter map** | `typeMap` — 11 language types with extensions (line 274-288) | rg `--type` built-in — no explicit map needed | Go适配 |
| 12 | **Glob pattern splitting** | `splitGlobPatterns()` — handles `*.ts, *.js` and brace groups (line 247-272) | Handled by rg `--glob` flag natively | Go适配 |
| 13 | **Output modes** | `content`, `files_with_matches`, `count` (line 63-65) | Same three modes (line 53-57) | ✅ Match |
| 14 | **Structured output** | Plain text — `"(Searched N files, M matches)"` (line 557-563) | Structured schema: `{mode, numFiles, filenames, content, numLines, numMatches, appliedLimit, appliedOffset}` (line 144-155) | 简化 |
| 15 | **Line truncation** | 500 chars `maxGrepLineLen` (line 13, 484-489) | rg `--max-columns 500` (line 291) | ✅ Match |
| 16 | **Dash-prefixed pattern** | Uses `-e` flag to prevent rg from interpreting as option (line 363-366) | Handled by rg utility | ✅ Match |
| 17 | **Permission-based file ignore** | Not present | `getFileReadIgnorePatterns()` from permission context (line 15) | 缺失 |
| 18 | **Plugin cache exclusion** | Not present | `getGlobExclusionsForPluginCache()` (line 20) | 缺失 |
| 19 | **isConcurrencySafe** | Not marked | `isConcurrencySafe() { return true }` | 缺失 |
| 20 | **isReadOnly** | Not marked | `isReadOnly() { return true }` | 缺失 |
| 21 | **isSearchOrReadCommand** | Not marked | `{isSearch: true, isRead: false}` | 缺失 |

---


---

### 54.6 Glob Tool

**Go**: `tools/glob_tool.go` (184 lines) · **Upstream**: `GlobTool/GlobTool.ts` (199 lines)

| # | Aspect | Go (file:line) | Upstream (file:line) | Type |
|---|--------|----------------|----------------------|------|
| 1 | **Input schema** | `pattern`, `path`, `head_limit`, `excludes` (line 27-51) | `pattern`, `path` only (line 26-36) | Go增强 |
| 2 | **head_limit parameter** | Supported — default 100, configurable (line 39-41, 68-79) | Not a parameter — hard limit of 100 via `globLimits.maxResults` (line 157) | Go增强 |
| 3 | **excludes parameter** | Supported — skip matching files/dirs (line 43-48, 82-94) | Not a parameter | Go增强 |
| 4 | **Auto-prefix `**/`** | Adds `**/` if pattern has no slash (line 107-111) | Handled by `glob()` utility internally | ✅ Match |
| 5 | **Matching engine** | `doublestar.Match` with `filepath.WalkDir` (line 114-143) | `glob()` utility (likely ripgrep-based or fast native) | Go适配 |
| 6 | **Sort by mtime** | Newest first via `os.Stat` per match (line 153-164) | Sorted by name via `toRelativePath` + `glob()` output | Go增强 |
| 7 | **UNC path blocking** | `isUncPath()` (line 97-99) | Same check in `validateInput` (line 100-103) | ✅ Match |
| 8 | **Directory validation** | `os.Stat` + `IsDir()` check (line 101-104) | `fs.stat()` + `isDirectory()` check in `validateInput` (line 106-131) | ✅ Match |
| 9 | **Similar path suggestion** | Not present | `suggestPathUnderCwd()` on ENOENT (line 110-119) | 缺失 |
| 10 | **Output format** | Plain text — relative paths + truncation notice (line 176-183) | Structured: `{filenames, durationMs, numFiles, truncated}` (line 167-172) | 简化 |
| 11 | **Truncation message** | `"(showing first N of M matches)"` (line 180) | `"(Results are truncated. Consider using a more specific path or pattern.)"` (line 189-195) | 简化 |
| 12 | **Relative paths** | `filepath.Rel(cwd, path)` (line 169-174) | `toRelativePath()` utility (line 166) | ✅ Match |
| 13 | **isConcurrencySafe** | Not marked | `isConcurrencySafe() { return true }` | 缺失 |
| 14 | **isReadOnly** | Not marked | `isReadOnly() { return true }` | 缺失 |
| 15 | **Permission context** | Not checked | `appState.toolPermissionContext` passed to `glob()` (line 163) | 缺失 |
| 16 | **Duration tracking** | Not present | `durationMs` in output (line 169) | 缺失 |

---


---

