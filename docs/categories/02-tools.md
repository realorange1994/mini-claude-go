# 02 — Tools

> Tool implementations, interfaces, registry, and gaps

## Overview

Go has a rich set of tools including some that upstream lacks (GitTool, @-references). However, upstream tools have deeper validation, richer safety systems, and better integration with the permission/hook pipeline.

---

## Section A: Tool Interface & Registry

> Source: [diff_upstream/07-tool-interface.md]

### A.1 Tool Interface Design

| Aspect | Go | Upstream | Gap |
|--------|-----|----------|-----|
| Tool interface | `Tool` struct: `Name()`, `Description()`, `InputSchema()`, `CheckPermissions()`, `Execute()` | `Command` class: `name`, `getDescription()`, `inputSchema`, `call()`, `getPromptForCommand()` | 差异 |
| Tool count | ~20 tools | ~30+ tools (including AskUserQuestion, EnterPlanMode, ExitPlanMode, TeamCreate, SendMessage, Sleep, TodoWrite, NotebookEdit) | 缺失 |
| Dynamic tool registration | No — all tools in `ToolRegistry` at startup | Yes — skills, MCP servers, plugins can add tools at runtime | 缺失 |
| Tool `tool_choice` parameter | Never sent to API | Used in classifier, compact, permission explainer, side queries | 缺失 |
| Structured output | Not supported | `output_format: { type: 'json_schema' }` for side queries (classifier, title generation, memory search) | 缺失 |

### A.2 Tool Interface & Registry (base.go vs upstream Tool.ts)

> Source: [diff_upstream/07-tool-interface.md] — §8 Tool Interface & Registry

- **Upstream**: `buildTool()` factory pattern with Zod schemas. Tool has 40+ optional fields: `searchHint`, `validateInput`, `checkPermissions`, `backfillObservableInput`, `toAutoClassifierInput`, `renderToolUseMessage`, `renderToolUseTag`, `renderToolResultMessage`, `extractSearchText`, `renderToolUseErrorMessage`, `isConcurrencySafe`, `isReadOnly`, `isSearchOrReadCommand`, `getPath`, `getToolUseSummary`, `getActivityDescription`, `preparePermissionMatcher`, etc. (`Tool.ts` lines 1-100+)
- **Go**: Go interface with 5 required methods: `Name()`, `Description()`, `InputSchema()`, `CheckPermissions()`, `Execute()`. Optional `ContextTool` interface adds `ExecuteContext()`. Schema is `map[string]any` instead of Zod (`base.go` lines 205-227)
- **Type**: 简化

### A.3 Tool Schema System

- **Upstream**: Zod v4 schemas with `z.strictObject()`, discriminated unions for output (`z.discriminatedUnion`), `lazySchema()` for circular dependency handling. Input and output schemas are fully validated.
- **Go**: `map[string]any` JSON-schema-like maps. No runtime schema validation beyond `ValidateParams` which checks required fields and enums (`base.go` lines 230-273)
- **Type**: 简化

### A.4 Tool Lifecycle Hooks

- **Upstream**: Extensive lifecycle: `prompt()` for dynamic prompt, `preparePermissionMatcher()` for pattern matching setup, `backfillObservableInput()` for input normalization, `renderToolUseMessage()`/`renderToolResultMessage()` for UI rendering, `extractSearchText()` for indexing, `isConcurrencySafe()`, `isReadOnly()`
- **Go**: No lifecycle hooks. Single `Execute()` method handles all logic. No UI rendering, no dynamic prompts, no concurrency safety markers
- **Type**: 缺失

### A.5 File State Tracking (Registry.filesRead)

- **Upstream**: `FileStateCache` (LRU cache) in `src/types/fileState.ts`. Tracks `mtime`, `timestamp`, `offset`, `limit`, `content`, `isPartialView`. Used by read dedup, staleness checks, and cache edits.
- **Go**: `Registry.filesRead` map with `fileReadInfo` struct. Tracks `mtime`, `readTime`, `readOffset`, `readLimit`, `content`, `isPartial`, `fromRead`. Thread-safe with `sync.RWMutex`. Similar data but simpler (no LRU eviction, no TTL) (`base.go` lines 277-363)
- **Type**: Go适配

### A.6 Path Canonicalization

- **Upstream**: `expandPath()` + `normalizeCaseForComparison()` + `toPosixPath()` + `relativePath()` for cross-platform comparison. Uses `posix` module for gitignore-pattern matching. Memoized `getResolvedWorkingDirPaths`.
- **Go**: `canonicalPath()` expands `~`, resolves to absolute, converts backslashes, lowercases. `normalizeFilePath()` uses `path.Clean` + backslash replacement + lowercase (`base.go` lines 39-49, 470-477)
- **Type**: Go适配

### A.7 Path Allowlist (IsPathAllowed)

- **Upstream**: `pathInAllowedWorkingPath()` checks against multiple working directories (original CWD + `additionalWorkingDirectories`). Resolves symlinks via `getPathsForPermissionCheck()`. Handles macOS `/var` -> `/private/var` symlink resolution.
- **Go**: `IsPathAllowed()` checks single working directory via `filepath.Rel()`. Resolves symlinks via `filepath.EvalSymlinks()`. No support for multiple working directories (`base.go` lines 481-520)
- **Type**: 简化

### A.8 CRLF Line Ending Handling

- **Upstream**: `readFileSyncWithMetadata()` detects line endings (CRLF/LF/CR). `writeTextContent()` preserves original line endings. Uses `LineEndingType` enum.
- **Go**: `RestoreCRLF()` function converts LF to CRLF. Manual byte-level scan with `strings.Builder` for O(n) performance (`base.go` lines 524-536)
- **Type**: Go适配

### A.9 Tool Result Metadata

- **Upstream**: Rich tool result with structured data types per tool (text, image, notebook, PDF, file_unchanged). Discriminated union output schema.
- **Go**: `ToolResult` struct with `Output` (string), `IsError` (bool), `Metadata` (`ToolResultMetadata`). Flat string output with metadata for exit code, duration, line count, truncation (`base.go` lines 59-94)
- **Type**: 简化

### A.10 Auto-Classifier Input Conversion

- **Upstream**: `toAutoClassifierInput()` method on each tool converts tool input to classifier-friendly format.
- **Go**: No equivalent method. Auto mode classifier receives tool name + params directly, no per-tool conversion.
- **Type**: 缺失

### A.11 Search Hint & Tool Discovery

- **Upstream**: `searchHint` field on each tool for natural language tool matching.
- **Go**: `ToolSearchTool` and `BriefTool` provide tool discovery. No per-tool search hints — tools have `Name()` and `Description()` only.
- **Type**: 简化

### A.12 Structured Tool Output & API Parameter Deep Comparison

> Source: [diff_upstream/07-tool-interface.md] — §99 Structured Tool Output

#### tool_choice Parameter

| Aspect | Go | Upstream TS |
|--------|-----|-------------|
| Main loop | **Never sent** — no `ToolChoice` field in `buildMessageParams()` | **Always sent** — `tool_choice: options.toolChoice` (even when `undefined` = API default `auto`) |
| Classifier | **Yes** — `ToolChoice: { OfTool: { Name: "classify_action" } }` | **Yes** — `tool_choice: { type: 'tool', name: 'classify_result' }` |
| Permission explainer | **Not implemented** | **Yes** — `tool_choice: { type: 'tool', name: 'explain_command' }` |
| Compact | **No** (no tools either) | **No** (`toolChoice: undefined`) |

#### Structured Output (output_format / json_schema)

| Feature | Go | Upstream TS |
|---------|-----|-------------|
| `output_format` API parameter | **Never sent** | **Yes** — sent as `output_config.format` |
| Session title generation | **No** | **Yes** — `outputFormat: { type: 'json_schema', schema: { title: string } }` |
| Memory relevance search | **No** | **Yes** — `output_format: { type: 'json_schema', schema: { selected_memories: string[] } }` |
| SDK --json-schema flag | **Not implemented** | **Yes** — `SyntheticOutputTool` + function hook enforcement |
| Structured output in tool results | **Not handled** | **Yes** — `structured_output` attachment extraction |

#### Thinking Configuration

| Feature | Go | Upstream TS |
|---------|-----|-------------|
| Main loop thinking | **Never configured** — relies on SDK default | **Configured** — adaptive/budget based on model capabilities, effort level, env overrides |
| Classifier thinking | **Not explicitly configured** | **Explicit** — `getClassifierThinkingConfig()` for alwaysOnThinking models |
| Disable thinking | **Only in compact** | **Yes** — used in 8+ contexts (compact, hooks, classifier, etc.) |

#### Temperature

| Feature | Go | Upstream TS |
|---------|-----|-------------|
| Main loop | **Never set** — SDK default (1.0) applies | **Conditionally set** — `temperature: 1` when thinking disabled |
| Classifier | **Never set** | **Explicit 0** — deterministic classification |

#### API Parameter Comparison Matrix

| Parameter | Go Main Loop | Go Classifier | Upstream Main | Upstream Classifier |
|-----------|-------------|---------------|---------------|--------------------|
| `tool_choice` | Never sent | Forced classify_action | Sent (undefined=auto) | Forced classify_result |
| `thinking` | SDK default | SDK default | Adaptive/budget | Disabled/undefined |
| `temperature` | Never set | Never set | 1 or override | 0 |
| `output_format` | Never sent | Never sent | Sent (sideQuery) | Sent (sideQuery) |
| `metadata` | Never sent | Never sent | Sent | Not sent (sideQuery) |
| `betas` | Never sent | Never sent | Sent | Sent |
| `stop_sequences` | Never sent | Never sent | Never sent | `</block>` (stage 1) |

#### Priority Assessment

| # | Difference | Severity | Impact |
|---|-----------|----------|--------|
| 1 | No `output_format` / `json_schema` structured output | **HIGH** | Missing SDK `--json-schema`, no structured API responses |
| 2 | No thinking config in main loop | **HIGH** | Doesn't leverage adaptive thinking; may waste tokens |
| 3 | No temperature=0 in classifier | **MEDIUM** | Classifier decisions non-deterministic |
| 4 | No metadata in API calls | **MEDIUM** | API-side analytics can't attribute Go sessions |
| 5 | No betas headers | **MEDIUM** | Missing: structured-outputs, cache-editing, AFK mode, etc. |
| 6 | No SyntheticOutputTool | **MEDIUM** | Missing SDK `--json-schema` enforcement |

**Action**: Add `tool_choice` support. Add structured output for side queries. Add thinking config computation. Add temperature=0 for classifier.

---

## Section B: File Tools — Read/Write/Edit

> Source: [diff_upstream/08-file-tools.md]

### B.1 File Read Tool

> Files: Go `tools/file_read.go` (469 lines) · Upstream `FileReadTool/FileReadTool.ts` (1184 lines)

#### Supported File Types

- **Upstream**: Supports text, image (PNG/JPEG/GIF/WebP with resizing/downsampling), PDF (page extraction, rendering), Jupyter notebooks (.ipynb), and file_unchanged dedup stub.
- **Go**: Supports text files and Jupyter notebooks (.ipynb). No image rendering, no PDF support. Binary detection via magic bytes.
- **Type**: 简化

#### File Size Limits

- **Upstream**: Dynamic limits via `getDefaultFileReadingLimits()` with GrowthBook feature flags. Token-based budget validation with `countTokensWithAPI`.
- **Go**: Fixed `maxFileSize = 256 * 1024` (256 KB). Offset/limit for large files. No token-based budget.
- **Type**: 简化

#### Dedup Implementation

- **Upstream**: `readFileState.get(fullFilePath)` LRU cache check. Compares `mtimeMs` vs `existingState.timestamp`. Returns `{type: 'file_unchanged'}` stub. ~18% dedup hit rate.
- **Go**: `Registry.HasFileBeenRead()` + `CheckFileStale()` for staleness. Returns `"File unchanged since last read."` prefix. No analytics, no killswitch.
- **Type**: Go适配

#### Device File Blocking

- **Upstream**: `BLOCKED_DEVICE_PATHS` set: `/dev/zero`, `/dev/random`, `/dev/urandom`, `/dev/full`, `/dev/stdin`, `/dev/tty`, `/dev/console`, `/dev/stdout`, `/dev/stderr`, `/dev/fd/0-2`, `/proc/*/fd/0-2`.
- **Go**: `blockedDevicePaths` slice with similar list. No `/dev/fd/` or `/proc/` pattern matching.
- **Type**: 简化

#### Binary File Detection

- **Upstream**: `hasBinaryExtension()` checks file extension. Excludes PDF, images, SVG (supported types).
- **Go**: Magic bytes detection: checks first bytes for PNG, GIF, JPEG, PDF signatures.
- **Type**: Go适配 (Go uses content-based detection, upstream uses extension-based)

#### Image Support — Missing

- **Upstream**: Full image support: read, resize, downsample with token limit. Returns base64 image data with MIME type and dimensions.
- **Go**: No image support. Detects binary images and returns error.
- **Type**: 缺失

#### PDF Support — Missing

- **Upstream**: PDF support: `readPDF`, `extractPDFPages`, `getPDFPageCount`, `parsePDFPageRange`. Page range parameter (`pages: "1-5"`).
- **Go**: No PDF support.
- **Type**: 缺失

#### Notebook Support

- **Upstream**: `readNotebook` + `mapNotebookCellsToToolResult` for Jupyter notebooks. Returns structured cell array with outputs.
- **Go**: Reads .ipynb files, returns all cells with outputs via `cat -n` format text.
- **Type**: Go适配 (Go returns text format, upstream returns structured data)

#### Token Budget Validation — Missing

- **Upstream**: `MaxFileReadTokenExceededError` with `countTokensWithAPI` validation.
- **Go**: No token-based validation. Byte-size limit only (256 KB).
- **Type**: 缺失

#### File Read Listeners — Missing

- **Upstream**: `registerFileReadListener(callback)` — pub/sub system notifying LSP, file history, etc.
- **Go**: No listener system. Registry tracks reads internally.
- **Type**: 缺失

#### Read Tool Comparison Table

| # | Aspect | Go (file:line) | Upstream (file:line) | Type |
|---|--------|----------------|----------------------|------|
| 1 | Input schema | `file_path`, `offset`, `limit` (line 41-60) | + `pages` (PDF) (line 227-243) | 缺失 |
| 2 | Max file size | 256 KB `maxFileSize` (line 13) | Configurable via feature flags | Go适配 |
| 3 | Pagination hint | `"Showing lines X-Y of Z"` (line 243-246) | Similar via `formatFileLines` | Match |
| 4 | Empty file warning | `<system-reminder>Warning: empty</system-reminder>` (line 217-219) | Same format | Match |
| 5 | Dedup | Mtime-based `FileUnchangedStub` (line 174-182) | LRU cache + GrowthBook killswitch | 简化 |
| 6 | Binary extension rejection | `isBinaryExtension()` — 60+ extensions (line 285-308) | `hasBinaryExtension()` | Go适配 |
| 7 | Magic bytes detection | `isBinaryMagic()` — 20+ signatures (line 342-458) | Not in read tool | Go增强 |
| 8 | Device file blocking | `isDeviceFile()` (line 314-337) | `isBlockedDevicePath()` | Match |
| 9 | UNC path blocking | `isUncPath()` (line 74-78) | Same in `validateInput` | Match |
| 10 | UTF-16 LE BOM support | BOM detection + `utf16.Decode` (line 191-196) | `readFileSyncWithMetadata` | Match |
| 11 | UTF-8 BOM stripping | Strips `\xEF\xBB\xBF` (line 200) | Same | Match |
| 12 | CRLF normalization | `strings.ReplaceAll` (line 198) | Same | Match |
| 13 | Image reading | Not present | Full image read with resize (line 866-891) | 缺失 |
| 14 | PDF reading | Not present | `extractPDFPages()`, `readPDF()` (line 893-1017) | 缺失 |
| 15 | PDF `pages` parameter | Not present | `pages: "1-5"` with `parsePDFPageRange()` | 缺失 |
| 16 | Token count validation | Not present | `validateContentTokens()` (line 755-772) | 缺失 |
| 17 | MacOS screenshot path | Not present | `getAlternateScreenshotPath()` (line 142-159) | 缺失 |
| 18 | Similar file suggestion | Not present | `findSimilarFile()` (line 639-647) | 缺失 |
| 19 | File read listeners | Not present | `registerFileReadListener()` (line 162-173) | 缺失 |
| 20 | Cyber risk reminder | Not present | `CYBER_RISK_MITIGATION_REMINDER` (line 729-738) | 缺失 |
| 21 | Memory file freshness | Not present | `memoryFreshnessNote()` (line 747-753) | 缺失 |
| 22 | Session file analytics | Not present | `detectSessionFileType()` (line 1067-1083) | 缺失 |
| 23 | Concurrent file retry | 50ms sleep + retry on ENOENT (line 84-89) | No equivalent | Go增强 |
| 24 | isConcurrencySafe | Not marked | `isConcurrencySafe() { return true }` | 缺失 |
| 25 | isReadOnly | Not marked | `isReadOnly() { return true }` | 缺失 |

### B.2 File Edit Tool

> Files: Go `tools/file_edit.go` (414 lines) · Upstream `FileEditTool/FileEditTool.ts` (626 lines)

#### Edit Validation (validateInput)

- **Upstream**: 10 error codes: 1=deny rule, 2=file not found, 3=notebook (wrong tool), 4=binary file, 5=too large, 6=old_string not found, 7=old_string empty, 8=new_string empty, 9=settings file validation, 10=path constraint.
- **Go**: Inline validation: checks file exists, old_string found, not a notebook. Returns error strings directly.
- **Type**: 简化

#### Quote Style Preservation

- **Upstream**: `preserveQuoteStyle` detects whether old_string uses single or double quotes and adjusts new_string to match. Handles curly/smart quote normalization.
- **Go**: `normalizeQuotes` function normalizes quote styles but does not preserve original quote style from old_string.
- **Type**: 简化

#### UTF-16 LE Encoding Support

- **Upstream**: `readFileSyncWithMetadata` detects UTF-16 LE BOM and preserves encoding.
- **Go**: UTF-16 LE BOM detection and conversion. Reads with BOM detection, converts to UTF-8 for matching, writes back with UTF-16 LE BOM if original had it.
- **Type**: Go适配

#### LSP/VSCode Integration — Missing

- **Upstream**: `clearDeliveredDiagnosticsForFile` on successful edit. `getLspServerManager` for LSP server notification. `notifyVscodeFileUpdated` for VSCode SDK MCP.
- **Go**: No LSP or IDE integration.
- **Type**: 缺失

#### File History Tracking — Missing

- **Upstream**: `fileHistoryEnabled` + `fileHistoryTrackEdit` for tracking edits in file history.
- **Go**: No file history tracking.
- **Type**: 缺失

#### Settings File Validation — Missing

- **Upstream**: `validateInputForSettingsFileEdit` prevents editing of Claude settings files without proper permission.
- **Go**: No settings file protection in edit tool.
- **Type**: 缺失

#### Edit Tool Comparison Table

| # | Aspect | Go (file:line) | Upstream (file:line) | Type |
|---|--------|----------------|----------------------|------|
| 1 | Input schema | `file_path`, `old_string`, `new_string`, `replace_all` (line 32-55) | Same + zod `strictObject` | Match |
| 2 | Output | Plain string (line 232) | Structured: patch, originalFile, gitDiff (line 561-573) | 简化 |
| 3 | old_string==new_string check | Returns error (line 89-91) | Returns `{result: false, behavior: 'ask'}` | 简化 |
| 4 | Empty old_string → new file | Handles: creates if empty/absent (line 93-118) | Same in `validateInput` (line 224-263) | Match |
| 5 | Quote normalization | `normalizeQuotes()` (line 160-162) | `findActualString()` | Match |
| 6 | Quote style preservation | `preserveQuoteStyle()` (line 248-276) | `preserveQuoteStyle()` | Match |
| 7 | CRLF normalization | Normalize for matching, `restoreCRLF()` after | `writeTextContent` with line ending param | Go适配 |
| 8 | UTF-16 LE support | Detect BOM, decode, re-encode (line 140-149, 219-222) | `readFileSyncWithMetadata` | Match |
| 9 | Trailing whitespace strip | `stripTrailingWhitespace()` except .md/.mdx | Same behavior | Match |
| 10 | Desanitization fallback | `desanitize()` (line 174-181) | `findActualString()` | Match |
| 11 | Line deletion handling | When newStr=="" strip trailing \n (line 199-210) | Same in `getPatchForEdit()` | Match |
| 12 | .ipynb rejection | Returns error (line 127-129) | Returns `{result: false, behavior: 'ask'}` | 简化 |
| 13 | Max edit file size | 1 GiB `maxEditSize` (line 120) | Same 1 GiB `MAX_EDIT_FILE_SIZE` | Match |
| 14 | Non-unique old_string error | Returns error (line 186-190) | Returns `{result: false, behavior: 'ask'}` | 简化 |
| 15 | Similar file suggestion | Not present | `findSimilarFile()` + `suggestPathUnderCwd()` | 缺失 |
| 16 | Settings file validation | Not present | `validateInputForSettingsFileEdit()` (line 346-359) | 缺失 |
| 17 | Inputs equivalence check | Not present | `areFileEditsInputsEquivalent()` (line 363-386) | 缺失 |
| 18 | LSP notification | Not present | `lspManager.changeFile()` + `saveFile()` (line 494-514) | 缺失 |
| 19 | VSCode notification | Not present | `notifyVscodeFileUpdated()` (line 517) | 缺失 |
| 20 | File history tracking | Not present | `fileHistoryTrackEdit()` (line 431-440) | 缺失 |
| 21 | Diff/patch generation | Not present | `getPatchForEdit()` (line 482-488) | 缺失 |
| 22 | userModified flag | Not present | Tracks if user modified (line 577-578) | 缺失 |
| 23 | Content-based staleness | mtime only | Content comparison fallback (line 453-467) | 缺失 |
| 24 | replace_all support | Supported | Supported (`types.ts`, `FileEditTool.ts:398`) | Match |

### B.3 File Write Tool

> Files: Go `tools/file_write.go` (101 lines) · Upstream `FileWriteTool/FileWriteTool.ts` (435 lines)

#### Read-Before-Write Validation

- **Upstream**: `validateInput` with 3 error codes: 1=deny rule, 2=path constraint, 3=settings file validation. Checks file state via `readFileState` for staleness.
- **Go**: Checks `Registry.CheckFileStale()` — validates file was read and not modified since read.
- **Type**: Go适配

#### Path Safety for Auto-Edit

- **Upstream**: `checkPathSafetyForAutoEdit` with `precomputedPathsToCheck` for symlink-resolved paths. Checks suspicious Windows patterns, Claude config files, dangerous files.
- **Go**: UNC path blocking + `IsPathAllowed` check. No suspicious Windows pattern detection (8.3 names, ADS, DOS device names).
- **Type**: 简化

#### Write Tool Comparison Table

| # | Aspect | Go (file:line) | Upstream (file:line) | Type |
|---|--------|----------------|----------------------|------|
| 1 | Input schema | `file_path` + `content` (line 32-46) | Same shape, zod `strictObject` | Go适配 |
| 2 | Output schema | Plain string (line 99) | Structured: type create/update, structuredPatch, gitDiff | 简化 |
| 3 | Read-before-write | `registry.CheckFileStale()` — mtime (line 76-79) | `readFileState.get()` — mtime + content fallback | 简化 |
| 4 | Content-based staleness | No content comparison | Content comparison on Windows false positives (line 291) | 缺失 |
| 5 | File encoding detection | None — always UTF-8 | Detects encoding, writes with `writeTextContent` (line 298, 305) | 缺失 |
| 6 | Line ending preservation | Not handled — writes raw | Explicitly writes `'LF'` (line 305) | 缺失 |
| 7 | Max write size | 10 MB `maxWriteSize` (line 63) | No explicit limit in tool | Go适配 |
| 8 | UNC path blocking | `isUncPath()` (line 71-73) | Same in `validateInput` (line 182-184) | Match |
| 9 | File sync after write | `os.Open` + `f.Sync()` (line 91-94) | No explicit sync | Go增强 |
| 10 | MkdirAll before write | `os.MkdirAll(dir, 0o755)` (line 83) | `await fs.mkdir(dir)` (line 254) | Match |
| 11 | Registry/mark-file-read | `registry.MarkFileReadWithContent()` (line 97) | `readFileState.set()` (line 332-337) | Match |
| 12 | LSP notification | Not present | `lspManager.changeFile()` + `saveFile()` (line 309-326) | 缺失 |
| 13 | VSCode notification | Not present | `notifyVscodeFileUpdated()` (line 329) | 缺失 |
| 14 | Team memory secret guard | Not present | `checkTeamMemSecrets()` (line 157-160) | 缺失 |
| 15 | Permission deny rule check | `CheckPathSafetyForAutoEdit` only | `matchingRuleForInput()` deny check (line 163-177) | 简化 |
| 16 | Diff/patch generation | Not present | `getPatchForDisplay()` (line 360-379) | 缺失 |
| 17 | Git diff (remote mode) | Not present | `fetchSingleFileGitDiff()` (line 344-357) | 缺失 |
| 18 | File history tracking | Not present | `fileHistoryTrackEdit()` (line 255-264) | 缺失 |
| 19 | Skill discovery | Not present | `discoverSkillDirsForPaths()` (line 233-245) | 缺失 |
| 20 | CLAUDE.md analytics | Not present | `logEvent('tengu_write_claudemd')` (line 340-342) | 缺失 |

### B.4 Multi-Edit (Atomic System)

> Source: [diff_upstream/08-file-tools.md] — Deep Comparison: Multi-Edit Atomic System
> Go file: `tools/multi_edit.go` · Upstream: `FileEditTool.ts` + `utils.ts`

#### Multi-edit Support

| Aspect | Go (`multi_edit.go`) | Upstream (`FileEditTool.ts`) |
|--------|---------------------|------------------------------|
| Schema | `edits: array<{old_string, new_string, replace_all}>` — multiple edits in one call | `old_string, new_string, replace_all` — one edit per call |
| Multi-edit internally | Yes — edits applied sequentially in one `Execute()` | `getPatchForEdits()` accepts `FileEdit[]` but called with single-element array |
| API exposure | Single tool call can apply N edits atomically | Only single-edit; multi-edit is internal-only for diff reconstruction |

**Go's multi_edit is a genuine multi-edit API.** Upstream's Edit tool only accepts one edit per invocation.

#### Edit Conflict Detection

Both implementations use the same mechanism:
- Tracks `appliedNewStrings[]`; checks if `oldTrimmed` is a substring of any previously applied `new_string`.
- Error message: `"old_string is a substring of a new_string from a previous edit"`
- Strips trailing newlines before check.

**Effectively identical.**

#### Atomic Rollback

| Aspect | Go (`multi_edit.go`) | Upstream (`FileEditTool.ts`) |
|--------|---------------------|------------------------------|
| Rollback mechanism | **None** — validates all edits in dry-run pass, but if WriteFile fails, file is not restored | **None** — validates via `validateInput()`, then applies in `call()`. No restore. |
| Dry run | Applies all edits to in-memory copy first. Fails before writing if any edit can't find old_string. | Validates in `validateInput()`, then applies in `call()` — TOCTOU window. |
| File history | **No file history/undo** | `fileHistoryTrackEdit()` backs up file before writing |

**Neither has true atomic rollback.** Go validates all edits against in-memory copy before writing, preventing partial application.

#### Edit Validation Comparison

| Check | Go (`multi_edit.go`) | Upstream (`FileEditTool.ts`) |
|-------|---------------------|------------------------------|
| file_path required | Yes | Yes (zod schema) |
| old_string non-empty | Yes — rejects | No — empty means "create new file" |
| File exists | Yes — `os.ReadFile` | Yes — validates + suggests similar files |
| File too large | 1 GiB guard | 1 GiB guard |
| UNC path blocking | Yes | Yes |
| Read-before-write | `CheckFileStale()` | `readFileState` check |
| CRLF normalization | Yes | Yes |
| Quote normalization | `normalizeQuotes()` | `findActualString()` + `normalizeQuotes()` |
| Quote style preservation | **No** | Yes — `preserveQuoteStyle()` |
| Desanitization | `desanitize()` with map | `desanitizeMatchString()` with same map |
| Multiple match check | **No** — silently replaces first | Yes — returns error if multiple matches |
| old_string === new_string | **No** — allows no-op | Yes — rejects identical old/new |
| Notebook blocking | **No** | Yes — `.ipynb` redirected to NotebookEditTool |
| Team memory secret check | **No** | Yes — `checkTeamMemSecrets()` |
| Settings file validation | **No** | Yes — `validateInputForSettingsFileEdit()` |
| LSP notification | **No** | Yes — `lspManager.changeFile()` |
| VSCode notification | **No** | Yes — `notifyVscodeFileUpdated()` |
| UTF-16LE encoding | **No** — assumes UTF-8 | Yes — detects BOM |
| Skill discovery | **No** | Yes — `discoverSkillDirsForPaths()` |

**Key differences**:
- Go allows empty old_string? No. Upstream allows for file creation.
- Go does NOT check for multiple matches when replace_all is false — silently does first replacement. Upstream errors.
- Go does NOT preserve curly quote style. Upstream does.
- Go does NOT detect UTF-16LE encoding.
- Go does NOT notify LSP servers or VSCode.
- Upstream has much richer validation (team secrets, settings files, notebooks, permission deny rules).

### B.5 Notebook Edit Tool

> Source: [diff_upstream/08-file-tools.md] — §54.9 Notebook Edit Tool
> Go: Not implemented · Upstream: `NotebookEditTool/NotebookEditTool.ts` (~400 lines)

| # | Aspect | Go | Upstream (file:line) | Type |
|---|--------|----|----------------------|------|
| 1 | Tool existence | **Not present** | Full implementation (line 30-400+) | 缺失 |
| 2 | Input schema | N/A | `notebook_path`, `cell_id`, `new_source`, `cell_type`, `edit_mode` | 缺失 |
| 3 | edit_mode | N/A | `replace`, `insert`, `delete` (line 50-56) | 缺失 |
| 4 | cell_type | N/A | `code`, `markdown` (line 43-49) | 缺失 |
| 5 | Read-before-edit | N/A | `readFileState.get()` + mtime check (line 221-237) | 缺失 |
| 6 | File encoding | N/A | `readFileSyncWithMetadata` preserves encoding + line endings | 缺失 |
| 7 | Cell ID lookup | N/A | `parseCellId()` — actual ID and `cell-N` index format (line 270-276) | 缺失 |
| 8 | Notebook format handling | N/A | nbformat 4+ with minor version check for cell IDs (line 381-390) | 缺失 |
| 9 | .ipynb validation | N/A | Rejects non-.ipynb files (line 189-196) | 缺失 |
| 10 | Replace→insert promotion | N/A | Auto-promotes to insert if replacing past end (line 371-377) | 缺失 |

> Go's `file_edit.go` explicitly rejects `.ipynb` files with message "use the notebook tool instead" (line 127-129), acknowledging this tool is needed but not yet implemented.

### B.6 Todo / TodoWrite Tool

> Source: [diff_upstream/08-file-tools.md] — §54.10 Todo / TodoWrite Tool
> Go: `tools/todo_write.go` (224 lines) · Upstream: `TodoWriteTool/TodoWriteTool.ts` (116 lines)

| # | Aspect | Go (file:line) | Upstream (file:line) | Type |
|---|--------|----------------|----------------------|------|
| 1 | Tool name | `"TodoWrite"` (line 115) | `TODO_WRITE_TOOL_NAME` | Match |
| 2 | Input schema | `todos` array with `content`, `status`, `activeForm` | `todos` via `TodoListSchema()` | Match |
| 3 | Status values | `pending`, `in_progress`, `completed` | Same — enforced by schema | Match |
| 4 | Output | Plain text success message (line 216) | Structured: `{oldTodos, newTodos, verificationNudgeNeeded}` | 简化 |
| 5 | All-done auto-clear | Not present | When all completed, clears list to `[]` (line 69-70) | 缺失 |
| 6 | Verification nudge | Not present | 3+ tasks completed without verification → nudge (line 76-86) | 缺失 |
| 7 | Todo v2 / Task system | Not present | `isTodoV2Enabled()` — can disable tool (line 53-54) | 缺失 |
| 8 | Session-based storage | In-memory `TodoList` struct | `appState.todos[todoKey]` with agent-scoped keys | Go适配 |
| 9 | Turn tracking / idle reminder | `IncrementTurn()` + `BuildIdleReminder()` — 10 turns (line 62-79) | Handled by attachment system | Go增强 |
| 10 | Reminder formatting | `BuildReminder()` — `○ ◐ ●` icons (line 82-106) | UI rendering layer | Go适配 |
| 11 | shouldDefer | Not marked | `shouldDefer: true` (line 51) | 缺失 |
| 12 | isEnabled | Always enabled | `!isTodoV2Enabled()` — feature flag | 缺失 |
| 13 | Multi-agent todo scoping | Not present — single global list | `agentId ?? getSessionId()` for scoping | 缺失 |

**Action**: Fix multiple-match check to error instead of silently replacing first. Add file history tracking. Add LSP/VSCode notifications.

---

## Section C: Exec/Bash Tool

> Source: [diff_upstream/09-exec-bash.md]

### C.1 Security Architecture

- **Upstream**: Multi-file defense-in-depth: `bashSecurity.ts` (24+ security check IDs, quote extraction, heredoc analysis, Tree-sitter AST), `bashPermissions.ts` (permission checking with classifier), `pathValidation.ts` (file operation path checks), `modeValidation.ts` (mode-specific validation), `shouldUseSandbox.ts` (sandbox detection). Uses Tree-sitter for AST parsing.
- **Go**: Single-file consolidation (`exec_tool.go`). Uses regex-based deny patterns (~10+ patterns). Command substitution detection, path validation, UNC path detection. No AST parsing, no Tree-sitter.
- **Type**: 简化

### C.2 Command Parsing

- **Upstream**: `tryParseShellCommand()` with full shell quote parsing, `splitCommand_DEPRECATED` for subcommand splitting, `parseCommandRaw` for AST parsing. Handles heredocs, process substitution, Zsh equals expansion.
- **Go**: Basic command string analysis. Regex-based pattern matching for dangerous commands. No shell AST parsing, no heredoc analysis.
- **Type**: 简化

### C.3 Security Check Coverage

- **Upstream**: 24+ numbered security checks: incomplete commands, jq system functions, obfuscated flags, shell metacharacters, dangerous variables, newlines, command substitution, IFS injection, git commit substitution, proc/environ access, malformed tokens, backslash-escaped whitespace, brace expansion, control characters, unicode whitespace, mid-word hash, Zsh dangerous commands.
- **Go**: Deny regex patterns cover: `rm -rf /`, `rm -rf ~`, `sudo rm`, `git push --force`, `git reset --hard`, `> /dev/sda`, `mkfs`, `dd if=`. Plus command substitution detection and path validation. ~10 deny patterns vs upstream's 24+ checks.
- **Type**: 简化

### C.4 Zsh-Specific Security — Missing

- **Upstream**: Comprehensive Zsh protection: `zmodload`, `emulate`, `sysopen/sysread/syswrite/sysseek`, `zpty`, `ztcp`, `zsocket`, `mapfile`, `zf_rm/zf_mv/zf_ln/zf_chmod/zf_chown/zf_mkdir/zf_rmdir/zf_chgrp`. Zsh equals expansion, glob qualifiers, always blocks, parameter expansion.
- **Go**: No Zsh-specific protections. Only basic command patterns.
- **Type**: 缺失

### C.5 Shell Wrapper Stripping

- **Upstream**: `stripSafeWrappers` and `stripSafeHeredocSubstitutions` remove safe redirections before validation while tracking pre-strip content.
- **Go**: `stripShellWrapper` function detects and removes common wrapper patterns.
- **Type**: Go适配

### C.6 Background Task Management

- **Upstream**: `ShellManager` with background task tracking, task IDs, streaming output. Background tasks accessible via `getBackgroundTask`, `listBackgroundTasks`, `waitForBackgroundTask`.
- **Go**: `backgroundTasks` map with `BackgroundTask` struct. `list_bg`, `wait_bg`, `kill_bg` subcommands.
- **Type**: Go适配

### C.7 Shell Selection

| Aspect | Go (`exec_tool.go`) | Upstream |
|--------|---------------------|----------|
| Unix | Always `bash -c` | Uses user's login shell (bash/zsh), detected by `Shell.ts` |
| Windows | `powershell -Command` > `bash -c` > `cmd /C` | Separate `PowerShellTool` for Windows; `BashTool` uses bash via WSL/Git Bash |
| Shell state | "The working directory persists between commands, but shell state does not." | "The shell environment is initialized from the user's profile (bash or zsh)." |
| Profile sourcing | **No** — bare `bash -c` runs without profile | **Yes** — "initialized from the user's profile (bash or zsh)" |

**Key difference**: Upstream runs commands through the user's login shell with profile sourcing. Go always uses bare `bash -c` on Unix.

### C.8 Process Group Management — Go's Platform-Specific Approach

| Aspect | Go (`exec_tool_unix.go`, `exec_tool_windows.go`) | Upstream |
|--------|-----|----------|
| Unix | `Setpgid: true` in `syscall.SysProcAttr` | Node.js child_process; `detached: true` equivalent |
| Windows | `CREATE_NEW_PROCESS_GROUP` in `syscall.SysProcAttr` | Windows process group via Node.js spawn options |
| Purpose | Prevents Ctrl+C forwarding to child; enables tree-kill | Same purpose — isolate child process for clean termination |
| Architecture | Two build-tagged files (`//go:build unix` and `//go:build windows`) | Single cross-platform `Shell.ts` with platform detection |

**Go's approach is cleaner architecturally** — build tags ensure only the correct platform code compiles.

### C.9 Signal Handling

| Aspect | Go (`exec_tool.go`) | Upstream |
|--------|---------------------|----------|
| User interrupt (Ctrl+C) | `<-ctx.Done()` → `killProcessGroup()` + `cmd.Process.Kill()` | `abortController.signal` aborts the shell command |
| Signal to child | `SIGKILL` to process group on Unix | Process kill via Node.js `childProcess.kill()` |
| Graceful shutdown | **No** — goes straight to SIGKILL | Shell command supports abort with potential graceful shutdown |
| Exit code on signal | Extracts `128 + signal_number` on Unix; returns -1 on Windows | Returns exit code from `result.code`; interrupted flag set |

### C.10 Timeout Management

| Aspect | Go (`exec_tool.go`) | Upstream (`BashTool.tsx`) |
|--------|---------------------|---------------------------|
| Default timeout | 120,000ms (2 min) | `getDefaultBashTimeoutMs()` — 120,000ms default |
| Max timeout | 600,000ms (10 min) | `getMaxBashTimeoutMs()` — 600,000ms default |
| On timeout behavior | Process continues in background (auto-backgrounding) | Shell command's `onTimeout` callback triggers backgrounding |
| Context vs Timer | **Explicitly NOT using context.WithTimeout** — timeout should not kill process | Uses `abortController.signal` for user interrupt; timeout is separate |

### C.11 Working Directory

| Aspect | Go (`exec_tool.go`) | Upstream |
|--------|---------------------|----------|
| CWD source | `params["working_dir"]` or `os.Getwd()` | `getCwd()` which returns tracked CWD state |
| CWD setting | `cmd.Dir = wd` — directly on exec.Cmd | Injects `cd ${cwd} &&` prefix into shell command |
| CWD persistence | **No** — each command is independent | **Yes** — CWD persists between calls via `setCwd()` / `getCwd()` |
| Cd restrictions | Permission check blocks `cd` to dangerous paths | Prevents CWD changes in sub-agents |

### C.12 Output Streaming

| Aspect | Go (`exec_tool.go`) | Upstream (`BashTool.tsx`) |
|--------|---------------------|---------------------------|
| stdout reading | `readLimited(stdout, 50000)` — reads up to 50KB in goroutine | `EndTruncatingAccumulator` — accumulates output with tail preservation |
| stderr reading | `readLimited(stderr, 25000)` — reads up to 25KB in goroutine | Merged into stdout (merged fd) |
| Real-time progress | **No** — output only available after process exits | **Yes** — async generator yields progress updates every ~1s |
| Streaming to user | **No** — result returned only when process finishes | Yes — `onProgress` callback updates UI in real-time |

### C.13 Output Truncation

| Aspect | Go (`exec_tool.go`) | Upstream |
|--------|---------------------|----------|
| Max output | 30,000 bytes | 30,000 chars default, configurable up to 150,000 |
| Truncation format | `"[N lines truncated]"` appended | Same format |
| Strategy | **Head-only** — truncation just cuts at 30K | **Head + tail** — `EndTruncatingAccumulator` preserves both beginning and end |
| Large output persistence | **No** | **Yes** — output > threshold persisted to disk; model gets `<persisted-output>` message with preview |
| Max persisted size | N/A | 64 MB |

### C.14 Background Execution

| Aspect | Go (`exec_tool.go`) | Upstream (`BashTool.tsx`) |
|--------|---------------------|---------------------------|
| Explicit background | `run_in_background` param → `BackgroundTaskCallback` | `run_in_background` param → `spawnShellTask()` |
| Auto-background on timeout | `TimeoutCallback` registers timed-out process as background task | `shellCommand.onTimeout` callback → `startBackgrounding()` |
| Assistant auto-background | **No** | **Yes** — 15s budget for assistant mode |
| Manual background (Ctrl+B) | **No** | **Yes** — user can press Ctrl+B to background running command |
| Foreground task tracking | **No** | `registerForeground()` / `unregisterForeground()` for Ctrl+B support |
| Sleep blocking | **No** | **Yes** — `detectBlockedSleepPattern()` blocks `sleep N` (N≥2) as first command |

### C.15 Safety Checks — Command Filtering

| Aspect | Go (`exec_tool.go`) | Upstream |
|--------|---------------------|----------|
| Deny regex patterns | 20+ patterns: `rm -rf`, `del /f`, `format`, `mkfs`, `dd of=`, fork bombs, PowerShell destructive, Docker prune, Git destructive | Tree-sitter AST parsing + semantic checks + deny/ask/allow rules + classifier |
| Command substitution detection | `$()`, `${}` (with safe var allowlist), backticks, process substitution | AST-based detection via tree-sitter; legacy regex fallback |
| Safe variable allowlist | `safeVarNames` map — 60+ entries | `SAFE_ENV_VARS` set — 25 entries + `ANT_ONLY_SAFE_ENV_VARS` |
| Glob/brace expansion detection | In destructive commands only: `rm`, `mv`, `cp`, `chmod`, `chown`, `git rm/clean/add` | Part of AST semantic checks |
| Deletion path validation | Dangerous Unix paths, Windows paths, wildcard, glob, path traversal, critical project files | `checkPathConstraints()` with AST-derived redirects |
| Redirect target validation | Blocks shell expansion in targets, /dev/*, /proc/, /sys/, /etc/, ~/.ssh/, .claude/, .env | `checkPathConstraints()` validates redirect targets |
| Compound command splitting | `splitCompoundCommand()` respects quoting | `splitCommand()` + AST-based splitting |
| Safe wrapper stripping | `stripSafeWrappers()` strips timeout, nice, nohup, time, stdbuf, ionice, env, sudo, doas | `stripSafeWrappers()` strips timeout, time, nice, nohup, stdbuf |
| Read-only command detection | `isReadOnlyCommand()` with extensive allowlist for git subcommands, npm, pip | `checkReadOnlyConstraints()` with similar but more granular checks |
| Destructive command warnings | `isDestructiveCommand()` returns warning message | `destructiveCommandWarning.ts` provides warnings for UI |
| Sandbox support | **No** | **Yes** — `SandboxManager` with filesystem/network restrictions |
| Classifier (ML-based) | **No** | **Yes** — `classifyBashCommand()` with allow/deny/ask descriptions |
| Tree-sitter AST parsing | **No** — regex-based only | **Yes** — `parseForSecurityFromAst()` provides structural command analysis |
| Permission rule system | **No** — deny/ask binary based on pattern matching | **Yes** — full deny/ask/allow rule system with exact, prefix, and wildcard matching |
| Cd + git compound block | **No** | **Yes** — blocks `cd X && git status` to prevent bare repo RCE |
| Subcommand count cap | **No** | **Yes** — 50 subcommand cap to prevent DoS |
| Heredoc handling | **No** | **Yes** — `stripSafeHeredocSubstitutions()` for safe heredoc patterns |

### C.16 Additional Upstream Features Missing from Go

1. **Image output handling** — upstream detects base64 image data URIs in stdout, resizes them, and returns as image content blocks. Go treats all output as text.
2. **Sed edit simulation** — upstream intercepts `sed -i` commands and applies them via `applySedEdit()` instead of running sed.
3. **Claude Code hints** — upstream strips `<claude-code-hint />` tags from stdout before sending to model.
4. **Git index lock detection** — upstream logs events when `.git/index.lock` errors are detected.
5. **Code indexing tool detection** — upstream tracks usage of code indexing tools.
6. **Command type classification** — upstream classifies commands as search/read/list/silent for UI display.
7. **Progress threshold** — upstream shows progress after 2 seconds.

### C.17 Exec/Bash Tool Comparison Table

> Source: [diff_upstream/09-exec-bash.md] — §54.4 Exec / Bash Tool

| # | Aspect | Go (file:line) | Upstream (file:line) | Type |
|---|--------|----------------|----------------------|------|
| 1 | Tool name | `"exec"` (line 37) | `"Bash"` via `BASH_TOOL_NAME` | Go适配 |
| 2 | Input schema | `command`, `working_dir`, `description`, `timeout`, `run_in_background` | + `dangerouslyDisableSandbox`, `_simulatedSedEdit` | Go适配 |
| 3 | working_dir parameter | Supported (line 56-58) | Not a parameter — CWD tracked via state | Go增强 |
| 4 | Timeout default/max | Default 120s, max 600s (line 251-264) | Dynamic via `getDefaultTimeoutMs()`/`getMaxTimeoutMs()` | Go适配 |
| 5 | run_in_background | Supported via `BackgroundTaskCallback` | Supported via `LocalShellTask` spawn system | Match |
| 6 | Auto-background on timeout | `TimeoutCallback` — process continues as background task | Same concept | Match |
| 7 | User interrupt (Ctrl+C) | Context cancellation kills process group | Same — abort controller | Match |
| 8 | Output truncation | 30KB max + `[N lines truncated]` | `EndTruncatingAccumulator` + persisted result | 简化 |
| 9 | Shell selection | PowerShell → bash → cmd on Windows; bash on Unix | Detected via `Shell` utility | Go适配 |
| 10 | Process group management | `setupProcessGroup` / `killProcessGroup` | Same concept | Match |
| 11 | Deny patterns | Regex-based `denyRegexps` — 20+ patterns | `bashSecurity.ts` + AST-based analysis | Go适配 |
| 12 | Command substitution detection | `$()`, `${}`, backtick, `<()`, `>()` | AST-based `parseForSecurity()` | 简化 |
| 13 | Safe variable allowlist | `safeVarNames` — 60+ variables | Similar allowlist | Match |
| 14 | Glob/brace expansion detection | Unquoted `*`, `?`, `[...]`, `{...}` in destructive cmds | Handled at permission rule level | Go增强 |
| 15 | Path validation | `validatePaths()` — blocks /etc, /usr, C:\, .git, etc. | `pathValidation.ts` — similar, more granular | Match |
| 16 | Redirect target validation | `validateRedirectTargets()` — blocks /dev/*, /etc/, ~/.ssh, .env | Handled in permission rules | Go增强 |
| 17 | Safe wrapper stripping | `stripSafeWrappers()` — timeout, nice, nohup, sudo, etc. | Same concept | Match |
| 18 | Compound command splitting | `splitCompoundCommand()` — `;`, `&&`, `\|\|`, `\|`, newlines | `splitCommandWithOperators()` — similar | Match |
| 19 | Read-only command detection | `isReadOnlyCommand()` — extensive map + git/npm subcommands | `readOnlyValidation.ts` — more exhaustive with AST | 简化 |
| 20 | Destructive command warning | `isDestructiveCommand()` — informational warning | `destructiveCommandWarning.ts` — similar | Match |
| 21 | UNC path blocking | `containsVulnerableUncPath()` | Same concept | Match |
| 22 | Internal URL detection | `containsInternalURL()` — localhost, private IPs | Handled at network level | Go增强 |
| 23 | Sandbox support | Not present | `dangerouslyDisableSandbox`, `SandboxManager` | 缺失 |
| 24 | Sed edit parsing | Not present | `parseSedEditCommand()` + `_simulatedSedEdit` | 缺失 |
| 25 | Sleep command blocking | Not present | Blocks `sleep` > 2s, suggests `run_in_background` | 缺失 |
| 26 | Image output detection | Not present | `isImageOutput()` + `resizeShellImageOutput()` | 缺失 |
| 27 | AST-based command parsing | Not present — regex-based | `parseForSecurity()` from `bash/ast` module | 缺失 |
| 28 | Foreground task management | Not present | `registerForeground`/`unforeground` for tracking | 缺失 |
| 29 | Code indexing detection | Not present | `detectCodeIndexingFromCommand()` | 缺失 |
| 30 | Claude Code hints | Not present | `extractClaudeCodeHints()` from command comments | 缺失 |
| 31 | IsReadOnly tool method | `IsReadOnly()` on `ExecTool` struct | `isReadOnly()` check in tool def | Match |
| 32 | stdin isolation | `cmd.Stdin = nil` to prevent interactive prompts | Not explicitly isolated | Go增强 |

**Go strength**: Platform-specific process group management (build-tagged files), double-kill for reliability, proper signal exit code extraction on Unix.

**Action**: Add head+tail output truncation. Add CWD persistence. Consider profile sourcing.

---

## Section D: Search Tools — Grep/Glob

> Source: [diff_upstream/10-search-tools.md]

### D.1 GrepTool

> Files: Go `tools/grep_tool.go` (626 lines) · Upstream `GrepTool/GrepTool.ts` (~300 lines)

#### Input Schema Comparison

| Parameter | Go | Upstream | Notes |
|-----------|-----|----------|-------|
| pattern | string, required | string, required | Match |
| path | string, optional | string, optional | Match |
| glob | string, optional | string, optional | Match |
| type | string, optional | string, optional | Match |
| -i | boolean, optional | boolean (semanticNumber), optional | Match |
| ignore_case | boolean, optional | **Missing** | Go only |
| case_insensitive | boolean, optional | **Missing** | Go only |
| fixed_strings | boolean, optional | **Missing** | Go only |
| output_mode | enum [content, files_with_matches, count] | Same three modes | Match |
| -B / -A / -C | number, optional | number, optional | Match |
| context | number, optional | number, optional | Match |
| context_before | number, optional | **Missing** | Go only |
| context_after | number, optional | **Missing** | Go only |
| -n | boolean, optional (defaults true) | boolean, optional (defaults true) | Match |
| multiline | boolean, optional | boolean, optional | Match |
| head_limit | number, optional (default 250) | number, optional (default 250) | Match |
| offset | number, optional (default 0) | number, optional (default 0) | Match |
| max_depth | number, optional | **Missing** | Go only |
| max_filesize | string, optional | **Missing** | Go only |

#### Execution Comparison

| Aspect | Go (`grep_tool.go`) | Upstream (`GrepTool/GrepTool.ts`) |
|--------|---------------------|-----------------------------------|
| ripgrep | Uses `exec.Command("rg", ...)` | Uses `ripGrep()` utility |
| Native Go fallback | **Yes** — `goSearch()` with `regexp.Regexp` | **No** — upstream requires ripgrep |
| --hidden flag | Passes `--hidden` | Passes `--hidden` |
| --max-columns | Passes `--max-columns 500` | Passes `--max-columns 500` |
| VCS exclusion | Excludes `.git`, `.svn`, `.hg`, `.bzr`, `.jj`, `.sl` | Same list |
| Type filter | Maps type names to extensions via `typeMap` | Passes `--type` directly to rg |
| Glob splitting | `splitGlobPatterns()` — handles commas/spaces with braces | Splits on commas/spaces preserving brace groups |
| Dash-prefixed patterns | Uses `-e` flag | Uses `-e` flag |
| Path validation | Basic `os.Stat` check | UNC path security check, ENOENT with CWD suggestion |
| Permission check | Always passthrough | `checkReadPermissionForTool()` — full permission system |
| Output: content mode | Plain text lines, relative paths | Structured: content string, numFiles, filenames, numLines, appliedLimit, appliedOffset |
| Output: files_with_matches | Plain text lines | Sorts by **modification time** (newest first) |
| Output: count mode | Plain text lines | Structured: content string, numFiles, numMatches |
| Output: no matches | `No matches found.` or `No matches found. (Searched N files)` | `No files found` / `No matches found` |
| Native Go search: binary skip | Skips `.exe`, `.dll`, `.so`, `.bin` | N/A |
| Zod validation | No schema validation | `z.strictObject()` with semantic types |
| ignore patterns from config | Not supported | `getFileReadIgnorePatterns()` + `normalizePatternsToPath()` |
| plugin cache exclusion | Not supported | `getGlobExclusionsForPluginCache()` |

**Key Differences:**
1. **Go has native fallback** when ripgrep is unavailable; upstream requires ripgrep.
2. **Go has extra parameters**: `ignore_case`, `case_insensitive`, `fixed_strings`, `context_before`, `context_after`, `max_depth`, `max_filesize`.
3. **Upstream has structured output** with typed fields (numFiles, filenames, appliedLimit); Go returns plain text.
4. **Upstream has full permission system** integration; Go always passes through.
5. **Upstream sorts files_with_matches by modification time**; Go native fallback doesn't sort.
6. **Upstream respects gitignore and plugin cache exclusions**; Go doesn't.

### D.2 GlobTool

> Files: Go `tools/glob_tool.go` (184 lines) · Upstream `GlobTool/GlobTool.ts` (199 lines)

| Aspect | Go (`glob_tool.go`) | Upstream (`GlobTool/GlobTool.ts`) |
|--------|---------------------|-----------------------------------|
| Input schema: pattern | string, required | string, required |
| Input schema: path | string, optional | string, optional |
| Input schema: head_limit | number, optional (default 100) | **Missing from schema** — handled via `globLimits` context |
| Input schema: excludes | array of strings, optional | **Missing** — upstream has no `excludes` param |
| Input schema: directory | Legacy alias for path | **Missing** |
| Execution | `filepath.WalkDir()` with `doublestar.Match()` | `glob()` utility (likely ripgrep-based glob) |
| Auto-prefix `**/` | When pattern has no slash | Same behavior (handled in glob utility) |
| Sort by modification time | Sorts by `ModTime().Unix()` (oldest first) | Sorts by mtimeMs (newest first) — **opposite order!** |
| Output format | Plain text, one path per line | Structured: filenames array, durationMs, numFiles, truncated |
| Truncation message | `(showing first N of M matches)` | `(Results are truncated. Consider using a more specific path or pattern.)` |
| Excludes filtering | `filepath.Match()` on dir names and relative paths | Handled by gitignore and permission context |
| UNC path security | `isUncPath()` check | UNC path security check |
| Permission check | Always passthrough | `checkReadPermissionForTool()` |
| Path validation | Basic `os.Stat` + IsDir check | ENOENT with CWD suggestion, directory type check |
| Zod validation | No schema validation | `z.strictObject()` |
| Max results default | 100 | 100 (via `globLimits.maxResults`) |
| gitignore support | No — only hardcoded `isIgnoredDir()` | Respects gitignore via glob utility |
| Plugin cache exclusion | Not supported | Via glob utility |

**Key Differences:**
1. **Go has `excludes` parameter**; upstream doesn't expose it.
2. **Go has `head_limit` in schema**; upstream handles it via context/glob utility.
3. **Sort order is opposite**: Go sorts oldest-first, upstream sorts newest-first.
4. **Go uses `doublestar` library** for glob matching; upstream uses its own glob utility.
5. **Upstream has gitignore support**; Go only has hardcoded ignored directory list.

### D.3 ListDirTool

| Aspect | Go (`list_dir.go`) | Upstream |
|--------|---------------------|----------|
| Exists in upstream? | Yes | **No equivalent tool** — upstream uses BashTool (`ls`) or GlobTool |
| Parameters | `path`, `recursive`, `max_entries` (default 200) | N/A |
| Output | Directories with `/` suffix, files without | N/A |
| Implementation | `os.Open` + `Readdirnames` (simple) / `filepath.Walk` (recursive) | N/A |

### D.4 Grep Tool Comparison Table

> Source: [diff_upstream/10-search-tools.md] — §54.5 Grep Tool

| # | Aspect | Go (file:line) | Upstream (file:line) | Type |
|---|--------|----------------|----------------------|------|
| 1 | Tool name | `"grep"` (line 18) | Search-related name via `GREP_TOOL_NAME` | Go适配 |
| 2 | Input schema | 20+ params including legacy aliases | 13 params — official names only | Go增强 |
| 3 | Dual-mode (rg / native) | Ripgrep if available, Go regexp fallback | Always uses ripgrep | Go增强 |
| 4 | Legacy parameter aliases | `ignore_case`, `case_insensitive`, `context_before`, `context_after`, `count_matches` | No legacy aliases | Go增强 |
| 5 | fixed_strings | Supported — adds `-F` to rg or `regexp.QuoteMeta` | Not a schema parameter | Go增强 |
| 6 | max_depth | Supported — adds `--max-depth` to rg | Not a parameter | Go增强 |
| 7 | max_filesize | Supported — adds `--max-filesize` to rg | Not a parameter | Go增强 |
| 8 | head_limit default | 250 `maxGrepMatches` | 250 `DEFAULT_HEAD_LIMIT` | Match |
| 9 | head_limit=0 unlimited | Treated as unlimited | Same — `limit === 0` = unlimited | Match |
| 10 | VCS directory exclusion | `.git`, `.svn`, `.hg`, `.bzr`, `.jj`, `.sl` | Same list | Match |
| 11 | Type filter map | `typeMap` — 11 language types with extensions | rg `--type` built-in | Go适配 |
| 12 | Output modes | `content`, `files_with_matches`, `count` | Same three modes | Match |
| 13 | Structured output | Plain text | Structured schema with typed fields | 简化 |
| 14 | Line truncation | 500 chars `maxGrepLineLen` | rg `--max-columns 500` | Match |
| 15 | Permission-based file ignore | Not present | `getFileReadIgnorePatterns()` | 缺失 |
| 16 | Plugin cache exclusion | Not present | `getGlobExclusionsForPluginCache()` | 缺失 |
| 17 | isConcurrencySafe | Not marked | `isConcurrencySafe() { return true }` | 缺失 |
| 18 | isReadOnly | Not marked | `isReadOnly() { return true }` | 缺失 |

### D.5 Glob Tool Comparison Table

> Source: [diff_upstream/10-search-tools.md] — §54.6 Glob Tool

| # | Aspect | Go (file:line) | Upstream (file:line) | Type |
|---|--------|----------------|----------------------|------|
| 1 | Input schema | `pattern`, `path`, `head_limit`, `excludes` | `pattern`, `path` only | Go增强 |
| 2 | head_limit parameter | Supported — default 100, configurable | Hard limit of 100 via `globLimits.maxResults` | Go增强 |
| 3 | excludes parameter | Supported — skip matching files/dirs | Not a parameter | Go增强 |
| 4 | Auto-prefix `**/` | Adds `**/` if pattern has no slash | Handled by `glob()` utility internally | Match |
| 5 | Matching engine | `doublestar.Match` with `filepath.WalkDir` | `glob()` utility | Go适配 |
| 6 | Sort by mtime | Newest first via `os.Stat` per match | Sorted by name via `toRelativePath` + `glob()` output | Go增强 |
| 7 | UNC path blocking | `isUncPath()` | Same check in `validateInput` | Match |
| 8 | Directory validation | `os.Stat` + `IsDir()` check | `fs.stat()` + `isDirectory()` check | Match |
| 9 | Similar path suggestion | Not present | `suggestPathUnderCwd()` on ENOENT | 缺失 |
| 10 | Output format | Plain text — relative paths + truncation notice | Structured: `{filenames, durationMs, numFiles, truncated}` | 简化 |
| 11 | Relative paths | `filepath.Rel(cwd, path)` | `toRelativePath()` utility | Match |
| 12 | isConcurrencySafe | Not marked | `isConcurrencySafe() { return true }` | 缺失 |
| 13 | isReadOnly | Not marked | `isReadOnly() { return true }` | 缺失 |
| 14 | Permission context | Not checked | `appState.toolPermissionContext` passed to `glob()` | 缺失 |
| 15 | Duration tracking | Not present | `durationMs` in output | 缺失 |

---

## Section E: Agent Tools

> Source: [diff_upstream/11-agent-tools.md]

### E.1 Forked Agent / Parallel API Calls

> Go file: `forked_agent.go` · Upstream reference: `src/utils/forkedAgent.ts`

#### Cache-Safe Params Model

- **Upstream**: `CacheSafeParams` carries `systemPrompt`, `userContext`, `systemContext`, `toolUseContext`, `forkContextMessages`. Global `lastCacheSafeParams` slot for post-turn forks.
- **Go**: `CacheSafeParams` carries `SystemPrompt`, `Model`, `Tools`, `Messages`, `ThinkingConfig`. No global slot; caller captures params explicitly via `CaptureCacheSafeParams()`.
- **Type**: Go适配 — Go uses explicit Anthropic SDK types instead of abstract ToolUseContext.

#### Forked Agent Execution Model

- **Upstream**: `runForkedAgent()` is async generator-based: iterates over `query()` yielding messages, accumulates usage from `message_delta` stream events, records sidechain transcripts, logs `tengu_fork_agent_query` analytics. Creates isolated `ToolUseContext` via `createSubagentContext()`.
- **Go**: `RunForkedAgent()` is synchronous loop: makes direct API calls via `client.Messages.New()`, executes tools concurrently via `sync.WaitGroup`, has its own retry logic with `retryForkedCall()`. No streaming, no sidechain transcript recording, no analytics events.
- **Type**: 简化 — no streaming, no transcript recording, no analytics, no isolated subagent context.

#### Subagent Context Isolation — Missing

- **Upstream**: `createSubagentContext()` creates isolated `ToolUseContext` with: cloned file state cache, new child abort controller, `shouldAvoidPermissionPrompts: true`, no-op setAppState, fresh denial tracking, fresh tool decisions, cloned content replacement state.
- **Go**: No equivalent — `ForkedAgentConfig` only carries `CanUseTool` callback. No file state cache, no content replacement, no denial tracking, no abort controller sharing.
- **Type**: 缺失

#### Permission Model for Forked Agents

- **Upstream**: `CanUseToolFn` called before each tool execution in fork; `createGetAppStateWithAllowedTools()` wraps parent's `getAppState` to inject allowed tools.
- **Go**: `CanUseToolFn` callback in `ForkedAgentConfig` — returns `(allowed bool, reason string)`. Simpler but equivalent in concept.
- **Type**: 简化

#### Forked Agent Retry Logic

- **Upstream**: Retry handled by the main `query()` loop with its own retry infrastructure.
- **Go**: Dedicated `retryForkedCall()` with exponential backoff + jitter, 3 max retries, re-classifies errors on each attempt.
- **Type**: Go增强 — self-contained retry logic independent of parent agent.

### E.2 Sub-Agent Orchestration

> Go file: `agent_sub.go` · Upstream reference: `builtInAgents.ts` + `AgentTool/`

#### Agent Type System

- **Upstream**: Built-in agent definitions: `general-purpose`, `explorer`, `planner`, `verifier`. Each has typed system prompts, tool restrictions, and model preferences.
- **Go**: `AgentType` enum: `General`, `Explore`, `Plan`, `Verify`, `Fork`. Each mapped to `agentTypeConfig` with `promptModifier`, `denyTools`, `allowTools`.
- **Type**: Go适配 — similar agent types; Go adds explicit Fork type with `forkBoilerplate` directive.

#### Fork Mode

- **Upstream**: Fork is implicit in `ForkedAgentParams` with `cacheSafeParams`; the fork shares the parent's conversation prefix for cache hits.
- **Go**: Explicit fork mode with `AgentTypeFork`, `forkBoilerplate` (10-rule directive prepended to user message), `filterEntriesForFork()` removes last ToolUseContent, CompactBoundaryContent, AttachmentContent from cloned entries.
- **Type**: Go增强 — explicit fork protocol with structured output format.

#### Tool Filtering Layers

- **Upstream**: Tool restrictions per agent type defined in `builtInAgents.ts` with `restrictedToolName` arrays.
- **Go**: 4-layer filtering: (1) `allAgentDisallowedTools` (7 tools), (2) `asyncAgentDisallowedTools`, (3) agent type `denyTools`, (4) caller `disallowedTools`. Plus whitelist `allowedTools` with `*` wildcard.
- **Type**: Go增强 — structured 4-layer filtering.

#### Model Alias Resolution

- **Upstream**: Model resolution via `parseUserSpecifiedModel()` in model utils.
- **Go**: `resolveModelAlias()` handles: empty/"inherit" → parent model, "sonnet"/"opus"/"haiku" → env var resolution with parent-tier matching (prevents downgrade), verbatim passthrough.
- **Type**: Go增强 — explicit alias resolution with parent-tier awareness.

#### Sub-Agent Max Tokens

- **Upstream**: Default max_tokens with `CAPPED_DEFAULT_MAX_TOKENS` for sub-agents.
- **Go**: Sub-agents start with `MaxOutputTokens = 8000` matching upstream's `CAPPED_DEFAULT_MAX_TOKENS`, with escalation to 64000.
- **Type**: Match — same cap strategy.

#### Sub-Agent Output Capture

- **Upstream**: Sub-agent output rendered via React components; `TaskOutputTool` reads completed task output.
- **Go**: `taskOutputWriter` captures output into `AgentTask` buffer + live output file at `.claude/sub-agents/{id}_output.txt` for non-blocking parent reads.
- **Type**: Go适配 — file-based output capture instead of React component tree.

### E.3 Agent Tool Comparison

#### Agent Lifecycle Management

| Aspect | Go | Upstream |
|--------|-----|----------|
| Execution model | **Always async** — sub-agents always run in background | **Dual mode** — sync (foreground, blocks caller) and async (background, returns immediately) |
| Sync agent flow | Not supported | Complex flow: register foreground, race message iteration vs background signal |
| Spawn mechanism | Callback `AgentSpawnFunc` injected into tool | Direct call to `runAgent()` with full `ToolUseContext` |
| Foreground-to-background transition | Not applicable (always async) | Auto-background after configurable timeout (default 120s) |
| Teammate spawning | Not supported | `spawnTeammate()` when `team_name` and `name` both provided |
| Remote agent spawning | Not supported | `teleportToRemote()` for CCR remote sessions |

#### Agent Tool Parameter Schema

| Parameter | Go (`agent_tool.go`) | Upstream (`AgentTool.tsx`) |
|-----------|---------------------|---------------------------|
| `description` (required) | Yes | Yes |
| `prompt` (required) | Yes | Yes |
| `subagent_type` | Yes (optional, defaults to general-purpose) | Yes (optional) |
| `model` | `enum: ["sonnet", "opus", "haiku"]` | `enum: ["sonnet", "opus", "haiku"]` |
| `run_in_background` | **DEPRECATED** — ignored; always background | **Active** — controls sync vs async execution |
| `allowed_tools` | Yes (`array<string>`, `["*"]` for all) | **MISSING** — tool filtering via permission rules |
| `disallowed_tools` | Yes (`array<string>`) | **MISSING** — tool filtering via permission rules |
| `inherit_context` | Yes (`boolean`, default false) | **MISSING as param** — fork gate experiment |
| `max_turns` | Yes (`integer`, default 200) | Via agent definition `maxTurns` |
| `timeout` | Yes (`integer`, max 600000ms) | **MISSING** — no explicit timeout param |
| `name` | **MISSING** | Yes — makes agent addressable via `SendMessage({to: name})` |
| `team_name` | **MISSING** | Yes — spawns as teammate instead of subagent |
| `mode` | **MISSING** | Yes — permission mode for teammate |
| `isolation` | **MISSING** | Yes — `worktree` or `remote` (ant-only) |
| `cwd` | **MISSING** | Yes — absolute path override |

#### Agent Naming and Registry

| Aspect | Go | Upstream |
|--------|-----|----------|
| Agent ID format | 8-char hex string via `crypto/rand` | UUID via `createAgentId()` |
| Agent name registry | Not supported | `agentNameRegistry` in AppState — maps name to agentId |
| Agent types | `subagent_type` string (free-form) | `AgentDefinition` objects with `agentType`, `source`, `color`, `permissionMode`, `maxTurns`, `model`, `isolation`, `background`, `memory`, `hooks`, `skills`, `mcpServers` |
| Built-in agents | Not implemented | `builtInAgents.ts` registers: general-purpose, statusline setup, explore, plan, code guide, verification |
| Fork subagent | `inherit_context` approximates this | Dedicated `FORK_AGENT` synthetic definition, cache-identical API prefixes |

#### Agent Status Tracking

| Aspect | Go | Upstream |
|--------|-----|----------|
| Status values | `pending`, `running`, `completed`, `failed`, `killed` | `running`, `completed`, `failed`, `killed` |
| Store | `AgentTaskStore` — in-memory map with RWMutex | `AppState.tasks` — React state record keyed by agentId |
| Progress tracking | `toolsUsed`, `durationMs`, `Output` buffer (50KB cap) | `AgentProgress`: `toolUseCount`, `tokenCount`, `lastActivity`, `recentActivities`, `summary` |
| Notified flag | `AgentTask.Notified` bool | `LocalAgentTaskState.notified` — atomic check-and-set |
| Output file | `OutputFile` path for live output | `getTaskOutputPath(taskId)` — symlinked to transcript |
| Retention/eviction | Not implemented | `retain`, `evictAfter`, `diskLoaded`, `PANEL_GRACE_MS` |

#### Agent Cancellation and Cleanup

| Aspect | Go | Upstream |
|--------|-----|----------|
| Kill mechanism | `AgentTaskStore.Kill(id)` calls `CancelFunc()` | `killAsyncAgent()` — aborts controller, unregisters cleanup |
| Cleanup handler | Not implemented | `registerCleanup()` — auto-kills on session end |
| Post-kill cleanup | Sets status only | Evicts output file, clears file state cache, kills shell tasks, clears session hooks, unregisters Perfetto agent, clears todos, kills MCP monitor tasks |
| Worktree cleanup | Not supported | `cleanupWorktreeIfNeeded()` — removes worktree if no changes |
| Abort controller linking | Single `CancelFunc` per task | Complex: parent-child linking, unlinked for async, shared for sync |

#### Agent Output Collection

| Aspect | Go | Upstream |
|--------|-----|----------|
| Output capture | `strings.Builder` with 50KB cap, truncation with marker | Message array from `query()` async generator, streamed to task state |
| Output truncation | Keep first quarter + truncation marker + recent content | Not truncated in memory; disk eviction via `evictTaskOutput()` |
| Live output file | `OutputFile` path written incrementally | Symlink to transcript file |
| Usage tracking | `ToolsUsed` (int), `DurationMs` (int64) | Full usage: `input_tokens`, `output_tokens`, `cache_creation_input_tokens`, `cache_read_input_tokens` |

#### Agent Results Persistence — Missing

| Aspect | Go | Upstream |
|--------|-----|----------|
| Sidechain transcript | Not mentioned | JSONL transcript via `recordSidechainTranscript()` with parent chain UUID |
| Agent metadata | Not mentioned | `writeAgentMetadata()` — stores agentType, worktreePath, description |
| Resume from disk | Not implemented | `resumeAgent.ts` — reconstructs agent state from sidechain transcript |
| Langfuse tracing | Not implemented | Sub-agent traces via `createSubagentTrace()` |
| Perfetto tracing | Not implemented | Agent hierarchy registration via `registerPerfettoAgent()` |
| Analytics | Not implemented | Multiple events: `tengu_agent_tool_selected`, `tengu_agent_tool_completed`, etc. |
| Handoff classifier | Not implemented | `classifyHandoffIfNeeded()` — YOLO safety classifier reviews sub-agent output |
| Background summarization | Not implemented | `startAgentSummarization()` — periodic LLM summaries |

#### Messaging Between Agents

| Aspect | Go | Upstream |
|--------|-----|----------|
| Send to sub-agent | `AddPendingMessage()` / `DrainPendingMessages()` on `AgentTask` | `queuePendingMessage()` / `drainPendingMessages()` / `appendMessageToLocalAgent()` |
| SendMessage tool | Not implemented | `SendMessageTool` — routes messages by name or agentId |
| Message queue | Simple `[]string` field on task | `enqueue_pendingNotification()` via `messageQueueManager` |
| Mailbox system | **No** | `teammateMailbox.ts` — full mailbox for swarm teammates |

#### Key Architecture Differences Summary

1. **Go is a simplified async-only implementation** — upstream has dual sync/async execution with complex foreground-to-background transition, auto-backgrounding, and rich UI integration.
2. **Go has no agent type system** — agents are identified only by free-form `subagent_type` string. Upstream has rich `AgentDefinition` with models, permission modes, MCP servers, hooks, skills, memory, colors.
3. **Go has no fork mechanism** — `inherit_context` boolean approximates upstream's fork subagent experiment with cache-identical API prefix sharing.
4. **Go has no isolation** — no worktree, remote, or cwd override support.
5. **Go has no teammate/multi-agent support** — upstream has `spawnTeammate()`, `team_name`, `name`, `agentNameRegistry`, and `SendMessage` routing.
6. **Go has minimal cleanup** — just cancel context. Upstream has extensive cleanup: file state cache, session hooks, prompt cache tracking, Perfetto, Langfuse, todos, shell tasks, MCP servers, worktrees.
7. **Go has no persistence** — no sidechain transcript, no metadata, no resume.
8. **Go has no analytics/tracing** — upstream has extensive analytics events, Langfuse sub-agent traces, Perfetto hierarchy visualization.
9. **Go has no safety classifier** — upstream runs `classifyHandoffIfNeeded()` before returning sub-agent results to parent.
10. **Go has no model resolution complexities** — upstream considers agent definition, parent model, permission mode, and fork gate for model selection.

### E.4 Agent Tool Comparison Table

> Source: [diff_upstream/11-agent-tools.md] — §54.8 Agent Tool

| # | Aspect | Go (file:line) | Upstream (file:line) | Type |
|---|--------|----------------|----------------------|------|
| 1 | Input schema | `description`, `prompt`, `subagent_type`, `model`, `run_in_background`, `allowed_tools`, `disallowed_tools`, `inherit_context`, `max_turns`, `timeout` | `description`, `prompt`, `subagent_type`, `model`, `run_in_background`, `name`, `team_name`, `mode`, `isolation`, `cwd` | Go适配 |
| 2 | Model selection | `"sonnet"`, `"opus"`, `"haiku"` enum | Same enum | Match |
| 3 | allowed_tools / disallowed_tools | Supported — explicit whitelist/blacklist | Not in schema — tool filtering handled by permission context | Go增强 |
| 4 | inherit_context | Supported — fork mode boolean | `isForkSubagentEnabled()` gate — not a schema parameter | Go适配 |
| 5 | max_turns | Supported — default 200 | Not a schema parameter | Go增强 |
| 6 | timeout parameter | Supported — max 600s | Not a schema parameter | Go增强 |
| 7 | Always background | `run_in_background=true` always passed (deprecated) | Conditionally available | Go适配 |
| 8 | Recursive agent blocking | Always appends `"agent"` to disallowedTools | `ALL_AGENT_DISALLOWED_TOOLS` constant | Match |
| 9 | Multi-agent / team | Not present | `name`, `team_name`, `mode` parameters | 缺失 |
| 10 | Isolation modes | Not present | `isolation: "worktree"\|"remote"` — creates git worktree or remote session | 缺失 |
| 11 | CWD override | Not present | `cwd` parameter — override working directory | 缺失 |
| 12 | Agent memory | Not present | `agentMemory.ts` + `agentMemorySnapshot.ts` | 缺失 |
| 13 | Agent color management | Not present | `agentColorManager.ts` | 缺失 |
| 14 | Built-in agent types | `subagent_type` string only | `builtInAgents.ts` — explore, plan, general-purpose, verification, claudeCodeGuide | 缺失 |
| 15 | Fork subagent | `inherit_context` flag (simplified) | `forkSubagent.ts` — message construction, worktree notice | 简化 |
| 16 | Agent resume | Not present | `resumeAgent.ts` | 缺失 |
| 17 | Progress tracking | Not present | `createProgressTracker()`, `emitTaskProgress()` | 缺失 |
| 18 | Agent summarization | Not present | `startAgentSummarization()` | 缺失 |
| 19 | Remote agent | Not present | `checkRemoteAgentEligibility()`, `teleportToRemote()` | 缺失 |
| 20 | Output format | Plain text with agentId, output_file, usage metadata | Structured schema: `{status, prompt, result?, agentId?}` or `{status: "async_launched", agentId, ...}` | 简化 |

**Action**: Add sync execution mode. Add agent definitions. Add handoff classifier for safety.

---

## Section F: Other Tools

> Source: [diff_upstream/12-other-tools.md]

### F.1 File History Tools

> Go files: `file_history_tools.go` + `filehistory.go` · Upstream reference: `src/utils/fileHistory.ts`

#### Architecture: Backup vs Snapshot

- **Upstream**: **Backup-based**: `fileHistoryTrackEdit()` creates hard-link copies of files at `~/.claude/file-history/{sessionId}/{hash}@v{N}`. Uses `copyFile()`/`link()` for file-level backups. MAX_SNAPSHOTS = 100.
- **Go**: **Snapshot-based**: `SnapshotHistory` stores full file content in `FileSnapshot` structs in-memory + persists to `.claude/snapshots/{timestamp}_{safeName}.json` files. Uses FNV-1a 128-bit checksums for dedup. `maxSnapshots = 50` per file.
- **Type**: Go适配 — in-memory snapshots with JSON persistence vs upstream's file-copy backup approach. Go version uses less disk space but more memory.

#### Tool Surface Area

- **Upstream**: File history is integrated into the undo/revert UI component; no explicit "file_history" tools exposed to the LLM.
- **Go**: 13 dedicated LLM-callable tools: `file_history`, `file_history_read`, `file_history_grep`, `file_restore`, `file_rewind`, `file_history_diff` (with chain diff), `file_history_summary`, `file_history_search`, `file_history_timeline`, `file_history_tag`, `file_history_annotate`, `file_history_checkout`, `file_history_batch`.
- **Type**: Go增强 — far more tool surface for LLM interaction; upstream uses TUI components only.

#### Version Specifier Resolution

- **Upstream**: Versions are numeric (`@v1`, `@v2`) tied to backup files; snapshots identified by `messageId`.
- **Go**: `ResolveVersion()` supports flexible specifiers: "v3"/"3" (absolute), "current"/"latest" (last), "last2" (relative), tag names like `[release]`.
- **Type**: Go增强 — richer version addressing scheme.

#### Tagging and Annotation — Go Enhancement

- **Upstream**: No tagging or annotation system for file versions.
- **Go**: `file_history_tag` tool supports add/list/delete/search actions on version tags. `file_history_annotate` adds user comments to specific versions.
- **Type**: Go增强 — no upstream equivalent.

#### Cross-File Timeline — Go Enhancement

- **Upstream**: No cross-file timeline; snapshots are per-message bundles.
- **Go**: `file_history_timeline` tool provides chronological cross-file change timeline with duration filtering.
- **Type**: Go增强 — cross-file view absent in upstream.

#### Batch Operations — Go Enhancement

- **Upstream**: No batch operations on file history.
- **Go**: `file_history_batch` tool supports glob-matched batch history/diff/restore/count operations.
- **Type**: Go增强 — no upstream equivalent.

#### File History Enablement — Missing

- **Upstream**: `fileHistoryEnabled()` checks global config + env vars `CLAUDE_CODE_DISABLE_FILE_CHECKPOINTING` / `CLAUDE_CODE_ENABLE_SDK_FILE_CHECKPOINTING`.
- **Go**: Always enabled; no enable/disable toggle.
- **Type**: 缺失 — no file history disable mechanism.

#### VSCode Notification Integration — Missing

- **Upstream**: `notifyVscodeSnapshotFilesUpdated()` sends `file_updated` notifications to VSCode MCP on snapshot changes.
- **Go**: No equivalent.
- **Type**: 缺失 — no IDE integration.

### F.2 Work Task / Dependency Graph

> Go file: `work_task.go` · Upstream reference: `src/utils/todo/types.ts` + `TodoWriteTool/TodoWriteTool.ts`

#### Task Model

| Go | Upstream |
|---|---|
| `WorkTask` = `{ ID, Subject, Description, ActiveForm, Status, Owner, Metadata, Blocks[], BlockedBy[], CreatedAt, UpdatedAt }` — full dependency graph with bidirectional edges | `TodoItem` = `{ content, status: 'pending'\|'in_progress'\|'completed', activeForm }` — flat list with no dependencies |
| 4 states: `pending`, `in_progress`, `completed`, `deleted` | 3 states: `pending`, `in_progress`, `completed`, no `deleted` state |

**Type**: Go增强 — dependency graph, ownership, metadata; upstream is a flat list.

#### Dependency Cycle Detection

- **Upstream**: No cycle detection; flat list has no dependencies.
- **Go**: `wouldCreateCycle()` performs BFS on both `Blocks` and `BlockedBy` edges from the blocker to detect if adding a new edge would create a cycle.
- **Type**: Go增强 — prevents deadlock in dependency graph.

#### Dependency Validation

- **Upstream**: No dependency validation.
- **Go**: `filterValidDeps()` removes references to non-existent tasks from dependency lists.
- **Type**: Go增强 — prevents dangling references.

#### Storage

| Go | Upstream |
|---|---|
| Pure memory `map[string]*WorkTask`, no persistence | **File-based persistence**: each task stored as `{configDir}/tasks/{taskListId}/{id}.json` |
| Task ID via `atomic.Int64` auto-increment | High water mark file (`.highwatermark`) ensures ID doesn't repeat |
| No taskListId concept — all tasks share single store | `getTaskListId()` supports multi-session/swarm shared task lists |

#### Concurrency Safety

| Go | Upstream |
|---|---|
| `sync.RWMutex` protects concurrent reads/writes | **File locks** (`proper-lockfile`), supports multi-process concurrency |
| Safe within single process | Supports multi-process/Claude swarm concurrent access |

#### Missing Features in Go

| Go Missing | Upstream Has |
|---|---|
| **No `claimTask()`** — no task claiming mechanism | `claimTask()` supports multi-agent competition |
| **No `resetTaskList()`** — no task list reset | `resetTaskList()` clears tasks and updates high water mark |
| **No `getAgentStatuses()`** — no agent busy/idle state | Based on task ownership computes `idle`/`busy` status |
| **No `unassignTeammateTasks()`** — no agent exit task recovery | `unassignTeammateTasks()` recovers tasks when agent is killed/closed |
| **No `TaskSchema` validation** | Zod schema validates task format |

### F.3 Git Tool Comparison

> Source: [diff_upstream/12-other-tools.md] — Part 1: Git Tool Comparison
> Go: `tools/git_tool.go` · Upstream: No dedicated GitTool — uses BashTool

#### Tool Interface — Parameter Schema

| Aspect | Go (`git_tool.go`) | Upstream (TypeScript) |
|--------|---------------------|----------------------|
| Tool existence | Dedicated `GitTool` struct implementing `Tool` interface | **No dedicated GitTool** — git operations via `BashTool` |
| Schema type | JSON Schema `map[string]interface{}` with 30+ typed properties | N/A — `BashTool` takes `command: string`, `dangerouslyOverrideSandbox: bool` |
| Operation enum | 35+ operations: clone, init, add, commit, push, pull, fetch, branch, checkout, merge, rebase, stash, reset, tag, status, diff, log, remote, show, describe, ls-files, ls-tree, rev-parse, rev-list, worktree, rm, mv, restore, switch, cherry-pick, revert, clean, blame, reflog, shortlog, gh, info | Commands must be expressed as raw shell commands |
| GitHub CLI | Built-in `gh` operation with read-only subcommand whitelist (pr view/list/diff/checks/status, issue view/list/status, run list/view, auth status, release list/view, search repos/issues/prs) | No built-in gh support; user runs `gh` via BashTool |
| Input validation | Rich per-operation validation in `buildGitCommand()` | BashTool validates at command level (path sandboxing, command allowlists) |

#### Git Operations Supported

| Operation | Go | Upstream |
|-----------|-----|----------|
| clone, init, add, commit, push, pull, fetch | Yes (structured params) | Via BashTool |
| branch, checkout, merge, rebase, reset, tag | Yes | Via BashTool |
| status, diff, log, show, describe, blame | Yes | Via BashTool |
| ls-files, ls-tree, rev-parse, rev-list, reflog, shortlog | Yes | Via BashTool |
| worktree add/remove/list | Yes (`worktree_name`, `worktree_branch`, `worktree_remove` params) | Via BashTool |
| rm, mv, restore, switch | Yes | Via BashTool |
| cherry-pick, revert (with `mainline`, `no_commit`, `no_edit`) | Yes | Via BashTool |
| clean (with `-f/-n/-d` flags) | Yes | Via BashTool |
| gh CLI (read-only) | Yes (whitelist-enforced) | Via BashTool |
| `info` (composite repo state) | Yes (special operation using utility functions) | **MISSING** — no equivalent composite info operation |

#### Safety Checks

| Aspect | Go | Upstream |
|--------|-----|----------|
| Dangerous operation detection | `isDangerousOperation()` blocks: `reset --hard`, `push --force`, `clean -f`, `branch -D` | BashTool: path sandboxing, command allowlists, denylists, permission modes |
| Flag whitelist validation | Per-subcommand safe flag maps: `gitDiffFlags`, `gitLogFlags`, `gitShowFlags`, `gitStatusFlags`, `gitBranchFlags`, `gitResetFlags`, `gitMergeFlags`, `gitRebaseFlags`, `gitPushFlags`, `gitPullFlags`, `gitStashPushFlags` | No git-specific flag validation — BashTool validates shell commands generically |
| gh subcommand safety | `GHSafeFlags` whitelist, `validateGHFlags()` rejects non-whitelisted flags | N/A |
| Path validation | `directory` param used as workdir; no path traversal checks in the git tool itself | BashTool has comprehensive path sandboxing (`pathSandbox`, `isPathAllowed`) |
| Remote check | push/pull/fetch verify remote exists | N/A |
| Proxy support | `proxy` param sets `https_proxy`/`http_proxy` env vars | BashTool inherits proxy from environment |

#### Permission Checks

| Aspect | Go | Upstream |
|--------|-----|----------|
| CheckPermissions | Returns `PermissionResultPassthrough()` for all non-dangerous ops; `PermissionResultDeny(msg)` for dangerous ops | BashTool uses `checkPermissions()` with permission modes: `allow`, `deny`, `passthrough` |
| Dangerous ops | Only 4: reset --hard, push --force, clean -f, branch -D | BashTool uses broader safety model: path sandboxing, command allow/deny lists, auto-mode classifier |
| User interaction | No user prompt — just returns deny message for dangerous ops | BashTool can prompt user for permission, use `auto` mode with YOLO classifier |

### F.4 MCP Tools

| Aspect | Go (`mcp_tools.go`) | Upstream (`MCPTool/MCPTool.ts`) |
|--------|---------------------|---------------------------------|
| Tool names | `list_mcp_tools`, `mcp_call_tool`, `mcp_server_status` | `mcp` (single dynamic tool) |
| Approach | **3 separate tools** with explicit schemas | **1 dynamic tool** — name/schema overridden per MCP server tool |
| list_mcp_tools | Dedicated tool with `server`/`pattern` filters | No equivalent — tools discovered dynamically |
| mcp_call_tool params | `server`, `tool`, `arguments`, `timeout`, `run_in_background` | Dynamic — schema comes from MCP server tool definition |
| Timeout handling | Configurable (1s-600s, default 30s) with goroutine + timer | Not in tool schema — handled by MCP client |
| Background execution | `run_in_background` param + `MCPTimeoutCallback` | No background mode |
| Timeout → background conversion | On timeout, registers as background task with task ID | Not supported |
| mcp_server_status | Dedicated tool showing connection status per server | No equivalent |
| Dynamic schema | Static schema for all MCP calls | **Overridden per MCP tool** in `mcpClient.ts` |

**Key Differences:**
1. Go uses 3 separate MCP tools; upstream uses 1 dynamic tool per MCP server tool.
2. Upstream dynamically overrides name/schema per MCP tool; Go uses static schemas.
3. Go has timeout with background conversion; upstream doesn't.
4. Upstream's approach is more model-friendly — each MCP tool appears as a first-class tool.

### F.5 Task Tool

| Aspect | Go (`task_tool.go`) | Upstream (`TaskCreateTool`, `TaskUpdateTool`, etc.) |
|--------|---------------------|-----------------------------------------------------|
| Tool names | `task_create`, `task_list`, `task_get`, `task_update`, `task_stop` | `TaskCreate`, `TaskList`, `TaskGet`, `TaskUpdate`, `TaskOutput`, `TaskStop` |
| TaskUpdate: verification nudge | **Missing** | Suggests verification agent when 3+ tasks close without verification |
| TaskUpdate: hooks | **Missing** | `executeTaskCompletedHooks()` — blocking hooks can prevent completion |
| TaskUpdate: mailbox notification | **Missing** | Notifies new owner via mailbox when ownership changes |
| TaskUpdate: auto-owner | **Missing** | Auto-sets owner when teammate marks task in_progress |
| TaskOutput tool | **Missing** | `TaskOutputTool` — wait for background task output |
| TaskList output | Plain text table | Structured output |
| Callback-based | `WorkTaskCreateFunc`, etc. | Uses `createTask()`/`updateTask()` from `src/utils/tasks.js` |
| Storage | In-memory (via callbacks) | File-based (`src/utils/tasks.js`) |
| Scalar→array coercion | `add_blocks`/`add_blocked_by` coerces scalars to arrays | Not needed — Zod enforces array type |

**Key Differences:**
1. Go uses callbacks; upstream uses file-based task storage.
2. Upstream has TaskOutputTool for waiting on background tasks; Go doesn't.
3. Upstream has verification nudge and task completion hooks; Go doesn't.
4. Naming: snake_case vs PascalCase for both tool names and parameter names.

### F.6 Memory Tool

| Aspect | Go (`memory_tool.go`) | Upstream |
|--------|----------------------|----------|
| Exists in upstream? | Yes — `memory_add`, `memory_search` | **No equivalent** — upstream uses TodoWriteTool + session storage |
| Categories | `preference`, `decision`, `state`, `reference` | N/A |
| Implementation | Callback-based (`MemoryAddCallback`, `MemorySearchCallback`) | N/A |
| Upstream equivalent | N/A | TodoWriteTool for task tracking, sessionStorage for persistent data, AgentMemory for agent-specific memory |

---

## Section G: Web Tools

> Source: [diff_upstream/13-web-tools.md]

### G.1 WebFetch Tool

> Go file: `tools/web_fetch.go` (395 lines) · Upstream: `WebFetchTool/WebFetchTool.ts` (319 lines)

| # | Aspect | Go (file:line) | Upstream (file:line) | Type |
|---|--------|----------------|----------------------|------|
| 1 | Tool name | `"web_fetch"` (line 20) | `WEB_FETCH_TOOL_NAME` | Go适配 |
| 2 | Input schema | `url` + `extractMode` (text/markdown/json) (line 25-39) | `url` + `prompt` (required prompt for LLM processing) (line 24-29) | 修正 |
| 3 | Content processing | HTML stripping (custom parser) or text mode (line 136-142) | LLM-based: `applyPromptToMarkdown(prompt, content)` (line 264-278) | 修正 |
| 4 | Prompt-based extraction | Not present — `extractMode` only | `prompt` parameter — sends content to LLM for extraction (line 271-278) | 缺失 |
| 5 | Redirect handling | Not present — follows redirects automatically | Detects cross-host redirects, returns redirect message asking to re-fetch (line 217-249) | 缺失 |
| 6 | Preapproved hosts | Not present | `isPreapprovedHost()` — auto-allow certain domains (line 111-118) | 缺失 |
| 7 | Permission rule by domain | Not present | `webFetchToolInputToPermissionRuleContent()` — `domain:hostname` (line 50-64) | 缺失 |
| 8 | file:// URL blocking | `CheckPermissions` blocks file:// (line 44-46) | Not checked (zod `z.string().url()` rejects file://) | Go适配 |
| 9 | Internal URL blocking | `containsInternalURL()` in CheckPermissions (line 47-49) | Not checked in WebFetch | Go增强 |
| 10 | Max body size | 1 MB `maxBodySize` (line 15) | No explicit limit — handled by `getURLMarkdownContent()` | Go适配 |
| 11 | Compression support | gzip + deflate decompression (line 110-126) | Handled by fetch utility | Match |
| 12 | Proxy support | `HTTP_PROXY` env var (line 76-79) | Not explicit in tool | Go增强 |
| 13 | Custom User-Agent | Full Chrome-like UA string (line 91) | Not visible in tool code — likely set in fetch utility | Go适配 |
| 14 | JSON output mode | `extractMode: "json"` — wraps as JSON object (line 155-156) | Not present — output always processed via LLM prompt | Go增强 |
| 15 | Title/description extraction | `extractHTMLTitle()` + `extractHTMLMeta()` (line 330-394) | Handled by markdown conversion utility | Go适配 |
| 16 | Binary content persistence | Not present | Saves binary content (PDFs, etc.) to disk, notes path in result (line 282-284) | 缺失 |
| 17 | Markdown passthrough | Not present | Preapproved markdown URLs < `MAX_MARKDOWN_LENGTH` pass through without LLM (line 265-269) | 缺失 |
| 18 | Abort/cancellation | Not present | `abortController.signal` passed through (line 214) | 缺失 |
| 19 | isConcurrencySafe | Not marked | `isConcurrencySafe() { return true }` | 缺失 |
| 20 | isReadOnly | Not marked | `isReadOnly() { return true }` | 缺失 |
| 21 | shouldDefer | Not marked | `shouldDefer: true` (line 71) | 缺失 |
| 22 | URL validation | `url.Parse()` + `IsAbs()` (line 59-62) | Zod `z.string().url()` + `new URL()` check (line 193-200) | Match |

**Key Differences:**
1. **Go does raw text extraction**; upstream uses an LLM to process content with a user-provided prompt.
2. **Upstream requires a `prompt` parameter**; Go doesn't have one.
3. **Go has `extractMode`** (text/markdown/json); upstream doesn't.
4. **Upstream handles redirects** explicitly; Go doesn't.
5. **Upstream has preapproved hosts** and binary content handling; Go doesn't.
6. **Go has proxy support**; upstream doesn't expose it.
7. **Fundamentally different architecture**: Go = fetch+strip, Upstream = fetch+convert+LLM-process.

### G.2 Cross-Cutting Observations

#### Parameters that Go has but upstream doesn't:
- **GrepTool**: `ignore_case`, `case_insensitive`, `fixed_strings`, `context_before`, `context_after`, `max_depth`, `max_filesize`
- **GlobTool**: `excludes`, `head_limit` (in schema), `directory` (legacy alias)
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

#### Architecture Patterns

| Pattern | Go | Upstream |
|---------|-----|----------|
| Schema validation | Manual type assertions (`v.(type)`) | Zod `z.strictObject()` with semantic types |
| Output format | Plain text strings | Structured typed objects with Zod schemas |
| Permission system | Always passthrough or simple block | Full permission framework with rules, suggestions, and modes |
| Error handling | `ToolResult{IsError: true}` | `ValidationResult` with error codes, messages, CWD suggestions |
| Tool registration | Static list in code | Dynamic via `buildTool()` with feature flags |
| UI rendering | Plain text only | React JSX components |
| Type safety | Runtime type assertions | Compile-time Zod + TypeScript |
| Proxy support | HTTP_PROXY env vars | No proxy configuration |
| Native fallback | Yes (GrepTool without rg) | No — requires ripgrep |
| Region adaptation | Yes (360.so for China) | No |

---

## Cross-References

- Permission system: [03-system-prompt.md](03-system-prompt.md) §3
- Hook system: [03-system-prompt.md](03-system-prompt.md) §4
- Error classification: [07-architecture.md](07-architecture.md) §3
- Go enhancements: [08-enhancements.md](08-enhancements.md)
- Source diffs: [diff_upstream/07-tool-interface.md], [diff_upstream/08-file-tools.md], [diff_upstream/09-exec-bash.md], [diff_upstream/10-search-tools.md], [diff_upstream/11-agent-tools.md], [diff_upstream/12-other-tools.md], [diff_upstream/13-web-tools.md]
