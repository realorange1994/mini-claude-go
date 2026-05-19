# File Tools

> File read/write/edit, multi-edit, notebook

## Sections Included
- [##] Line 1765-1842 -- ## 10. File Read Tool (tools/file_read.go vs upstream FileReadTool)
- [##] Line 1843-1910 -- ## 11. File Edit Tool (tools/file_edit.go vs upstream FileEditTool)
- [##] Line 1911-1953 -- ## 12. File Write Tool (tools/file_write.go vs upstream FileWriteTool)
- [##] Line 7691-7793 -- ## Deep Comparison: Multi-Edit Atomic System vs Upstream FileEditTool
- [##] Line 9274-9350 -- ## Section XXIII. Round 28 — Tool-Level Code Differences (Read/Write/Edit/Bash)
- [##] Line 9809-9940 -- ## 42. Tools Deep Dive — FileRead/Write/Edit, Bash, Grep/Glob, Agent/Skills, WebFetch/Search
- [###] Line 11297-11327 -- ### 54.1 File Write Tool
- [###] Line 11328-11361 -- ### 54.2 File Edit Tool
- [###] Line 11362-11400 -- ### 54.3 File Read Tool
- [###] Line 11559-11579 -- ### 54.9 Notebook Edit Tool
- [###] Line 11580-11604 -- ### 54.10 Todo / TodoWrite Tool

---

## Content

## 10. File Read Tool (tools/file_read.go vs upstream FileReadTool)

### Files Compared
- **Go**: `E:\Git\miniClaudeCode-go-github\tools\file_read.go` (469 lines)
- **Upstream**: `E:\Git\claude-code-upstream\packages\builtin-tools\src\tools\FileReadTool\FileReadTool.ts` (1184 lines), plus `prompt.ts`, `limits.ts`, `UI.ts`, etc.

### 10.1 Supported File Types
- **上游**: Supports text, image (PNG/JPEG/GIF/WebP with resizing/downsampling), PDF (page extraction, rendering), Jupyter notebooks (.ipynb), and file_unchanged dedup stub. Image compression with token limit. PDF page range support (`FileReadTool.ts` lines 187-332, 493-600)
- **Go版**: Supports text files and Jupyter notebooks (.ipynb). No image rendering, no PDF support. Binary detection via magic bytes (`file_read.go`)
- **类型**: 简化

### 10.2 File Size Limits
- **上游**: Dynamic limits via `getDefaultFileReadingLimits()` with GrowthBook feature flags. `maxSizeBytes`, `maxTokens`, `targetedRangeNudge`, `includeMaxSizeInPrompt`. Token-based budget validation with `countTokensWithAPI` and `roughTokenCountEstimationForFileType` (`FileReadTool.ts` lines 504-515, `limits.ts`)
- **Go版**: Fixed `maxFileSize = 256 * 1024` (256 KB). Offset/limit for large files. No token-based budget
- **类型**: 简化

### 10.3 Dedup Implementation
- **上游**: `readFileState.get(fullFilePath)` LRU cache check. Compares `mtimeMs` vs `existingState.timestamp`. Returns `{type: 'file_unchanged'}` stub. Killswitch via GrowthBook `tengu_read_dedup_killswitch`. Analytics event `tengu_file_read_dedup`. ~18% dedup hit rate reported. Only dedups text/notebook reads, not images/PDFs (`FileReadTool.ts` lines 523-573)
- **Go版**: `Registry.HasFileBeenRead()` + `CheckFileStale()` for staleness. Returns `FileUnchangedStub = "File unchanged since last read."` prefix. No analytics, no killswitch. Registry tracks `mtime` per file (`base.go` lines 349-429, `file_read.go`)
- **类型**: Go适配

### 10.4 Device File Blocking
- **上游**: `BLOCKED_DEVICE_PATHS` set: `/dev/zero`, `/dev/random`, `/dev/urandom`, `/dev/full`, `/dev/stdin`, `/dev/tty`, `/dev/console`, `/dev/stdout`, `/dev/stderr`, `/dev/fd/0-2`. Plus `/proc/*/fd/0-2` pattern matching (`FileReadTool.ts` lines 96-128)
- **Go版**: `blockedDevicePaths` slice with similar list. `/dev/zero`, `/dev/random`, `/dev/urandom`, `/dev/full`, `/dev/stdin`, `/dev/tty`, `/dev/console`, `/dev/stdout`, `/dev/stderr`. No `/dev/fd/` or `/proc/` pattern matching
- **类型**: 简化

### 10.5 UNC Path Blocking
- **上游**: UNC path check deferred until after permission grant: `fullFilePath.startsWith('\\\\') || fullFilePath.startsWith('//')`. Returns `{result: true}` (allows through but prompts for permission) (`FileReadTool.ts` lines 463-467)
- **Go版**: Blocks UNC paths in `IsPathAllowed` and file_read tool. Returns error immediately
- **类型**: Go适配

### 10.6 Binary File Detection
- **上游**: `hasBinaryExtension()` checks file extension. Excludes PDF, images, SVG (supported types). Returns error for unsupported binary files (`FileReadTool.ts` lines 471-482)
- **Go版**: Magic bytes detection: checks first bytes for PNG, GIF, JPEG, PDF signatures. Returns "binary file" error for detected binary types
- **类型**: Go适配 (Go uses content-based detection, upstream uses extension-based)

### 10.7 Image Support
- **上游**: Full image support: read, resize, downsample with token limit. `compressImageBufferWithTokenLimit`, `maybeResizeAndDownsampleImageBuffer`, `detectImageFormatFromBuffer`. Returns base64 image data with MIME type and dimensions (`FileReadTool.ts` lines 44-50, 270-297)
- **Go版**: No image support. Detects binary images and returns error
- **类型**: 缺失

### 10.8 PDF Support
- **上游**: PDF support: `readPDF`, `extractPDFPages`, `getPDFPageCount`, `parsePDFPageRange`. Page range parameter (`pages: "1-5"`). Returns base64 PDF or extracted page images. Max pages per read limit (`FileReadTool.ts` lines 61-66, 236-241, 306-324)
- **Go版**: No PDF support
- **类型**: 缺失

### 10.9 Notebook Support
- **上游**: `readNotebook` + `mapNotebookCellsToToolResult` for Jupyter notebooks. Returns structured cell array with outputs
- **Go版**: Reads .ipynb files, returns all cells with outputs via `cat -n` format text
- **类型**: Go适配 (Go returns text format, upstream returns structured data)

### 10.10 macOS Screenshot Resolution
- **上游**: `getAlternateScreenshotPath` handles macOS thin space (U+202F) in screenshot filenames. Tries alternate space character if file not found (`FileReadTool.ts` lines 130-159)
- **Go版**: No macOS screenshot path resolution
- **类型**: 缺失

### 10.11 Skill Discovery on Read
- **上游**: `discoverSkillDirsForPaths` + `activateConditionalSkillsForPaths` fire on file read. Background skill loading. Dynamic skill directory triggers (`FileReadTool.ts` lines 576-591)
- **Go版**: No skill discovery on file read
- **类型**: 缺失

### 10.12 Session File Detection
- **上游**: `detectSessionFileType` identifies session_memory and session_transcript files for analytics logging. Only matches within Claude config directory (`FileReadTool.ts` lines 195-225)
- **Go版**: No session file detection
- **类型**: 缺失

### 10.13 File Read Listeners
- **上游**: `registerFileReadListener` callback system for notifying other services when files are read. Used by LSP, file history, etc. (`FileReadTool.ts` lines 161-173)
- **Go版**: No listener system. Registry tracks reads internally
- **类型**: 缺失

### 10.14 Token Budget Validation
- **上游**: `MaxFileReadTokenExceededError` with `countTokensWithAPI` validation. File content token count checked against maxTokens limit. `roughTokenCountEstimationForFileType` for quick estimation (`FileReadTool.ts` lines 175-185)
- **Go版**: No token-based validation. Byte-size limit only (256 KB)
- **类型**: 缺失

---


---

## 11. File Edit Tool (tools/file_edit.go vs upstream FileEditTool)

### Files Compared
- **Go**: `E:\Git\miniClaudeCode-go-github\tools\file_edit.go` (414 lines)
- **Upstream**: `E:\Git\claude-code-upstream\packages\builtin-tools\src\tools\FileEditTool\FileEditTool.ts` (626 lines), plus `constants.ts`, `prompt.ts`, `types.ts`, `UI.ts`, `utils.ts`

### 11.1 Edit Validation (validateInput)
- **上游**: `validateInput` with 10 error codes: 1=deny rule, 2=file not found, 3=notebook (wrong tool), 4=binary file, 5=too large, 6=old_string not found, 7=old_string empty, 8=new_string empty, 9=settings file validation, 10=path constraint. Pre-I/O validation for path/rules, post-I/O for content (`FileEditTool.ts` lines 100-200)
- **Go版**: Inline validation: checks file exists, old_string found, not a notebook. Returns error strings directly
- **类型**: 简化

### 11.2 Quote Style Preservation
- **上游**: `preserveQuoteStyle` in `utils.js` — detects whether old_string uses single or double quotes and adjusts new_string to match. `findActualString` handles whitespace variations around quoted strings (`utils.ts`)
- **Go版**: `normalizeQuotes` function — normalizes quote styles but does not preserve original quote style from old_string
- **类型**: 简化

### 11.3 Curly Quote Handling
- **上游**: Curly/smart quote normalization in `preserveQuoteStyle`. Converts unicode curly quotes to straight quotes for matching
- **Go版**: Curly quote preservation flag — detects and preserves curly quotes in old_string/new_string
- **类型**: Go适配 (different approach: Go preserves, upstream normalizes)

### 11.4 UTF-16 LE Encoding Support
- **上游**: `readFileSyncWithMetadata` detects UTF-16 LE BOM and preserves encoding. `writeTextContent` writes with original encoding
- **Go版**: UTF-16 LE BOM detection and conversion. Reads with BOM detection, converts to UTF-8 for matching, writes back with UTF-16 LE BOM if original had it
- **类型**: Go适配

### 11.5 LSP/VSCode Integration
- **上游**: `clearDeliveredDiagnosticsForFile` on successful edit. `getLspServerManager` for LSP server notification. `notifyVscodeFileUpdated` for VSCode SDK MCP. Diagnostics are cleared and editors are notified (`FileEditTool.ts` lines 5-7)
- **Go版**: No LSP or IDE integration
- **类型**: 缺失

### 11.6 File History Tracking
- **上游**: `fileHistoryEnabled` + `fileHistoryTrackEdit` for tracking edits in file history. `logFileOperation` for analytics (`FileEditTool.ts` lines 29-32)
- **Go版**: No file history tracking
- **类型**: 缺失

### 11.7 Git Diff Integration
- **上游**: `fetchSingleFileGitDiff` for generating diff after edit. `ToolUseDiff` type for structured diff output (`FileEditTool.ts` lines 40-42)
- **Go版**: No git diff integration
- **类型**: 缺失

### 11.8 replace_all Support
- **上游**: `replace_all?: boolean` parameter in `FileEditInput` (default false). Upstream supports it — see `types.ts`, `FileEditTool.ts:398`. Previous entry incorrectly said "No native replace_all"
- **Go版**: `replace_all` parameter for replacing all occurrences of old_string
- **类型**: Go适配 (both support it, not a Go增强)

### 11.9 CRLF/Line Ending Handling
- **上游**: `readFileSyncWithMetadata` returns `LineEndingType`. `writeTextContent` preserves original line endings
- **Go版**: `RestoreCRLF` function. Detects and restores CRLF line endings after edit
- **类型**: Go适配

### 11.10 Notebook Rejection
- **上游**: `validateInput` error code 3: "Use the notebook edit tool for .ipynb files"
- **Go版**: Checks file extension for `.ipynb` and rejects with error message
- **类型**: Go适配

### 11.11 Settings File Validation
- **上游**: `validateInputForSettingsFileEdit` prevents editing of Claude settings files without proper permission
- **Go版**: No settings file protection in edit tool
- **类型**: 缺失

### 11.12 Skill Discovery on Edit
- **上游**: `discoverSkillDirsForPaths` + `activateConditionalSkillsForPaths` on edit, same as read
- **Go版**: No skill discovery on edit
- **类型**: 缺失

---


---

## 12. File Write Tool (tools/file_write.go vs upstream FileWriteTool)

### Files Compared
- **Go**: `E:\Git\miniClaudeCode-go-github\tools\file_write.go` (101 lines)
- **Upstream**: `E:\Git\claude-code-upstream\packages\builtin-tools\src\tools\FileWriteTool\FileWriteTool.ts` (435 lines)

### 12.1 Read-Before-Write Validation
- **上游**: `validateInput` with 3 error codes: 1=deny rule, 2=path constraint, 3=settings file validation. Checks file state via `readFileState` for staleness
- **Go版**: Checks `Registry.CheckFileStale()` — validates file was read and not modified since read
- **类型**: Go适配

### 12.2 Path Safety for Auto-Edit
- **上游**: `checkPathSafetyForAutoEdit` with `precomputedPathsToCheck` for symlink-resolved paths. Checks suspicious Windows patterns, Claude config files, dangerous files. Returns `{safe: true/false, message, classifierApprovable}` (`filesystem.ts` lines 620-665)
- **Go版**: UNC path blocking + `IsPathAllowed` check. No suspicious Windows pattern detection (8.3 names, ADS, DOS device names, etc.)
- **类型**: 简化

### 12.3 LSP/VSCode Notification
- **上游**: `notifyVscodeFileUpdated` + LSP diagnostic clearing on successful write
- **Go版**: None
- **类型**: 缺失

### 12.4 File History
- **上游**: `fileHistoryTrackWrite` for tracking writes in file history
- **Go版**: None
- **类型**: 缺失

### 12.5 Git Diff
- **上游**: `fetchSingleFileGitDiff` after write for diff output
- **Go版**: None
- **类型**: 缺失

### 12.6 New File Creation
- **上游**: `CheckFileStale` returns empty for non-existent files (new file creation allowed without read)
- **Go版**: `CheckFileStale` returns empty for `os.IsNotExist` — same behavior
- **类型**: Go适配 (matches upstream)

### 12.7 File Size
- **上游**: 435 lines with full validation, notification, history, diff
- **Go版**: 101 lines — basic write with staleness check and registry update
- **类型**: 简化

---


---

## Deep Comparison: Multi-Edit Atomic System vs Upstream FileEditTool

**Go file:** `tools/multi_edit.go`
**Upstream files:** `packages/builtin-tools/src/tools/FileEditTool/FileEditTool.ts`, `packages/builtin-tools/src/tools/FileEditTool/utils.ts`, `packages/builtin-tools/src/tools/FileEditTool/types.ts`, `packages/builtin-tools/src/tools/FileEditTool/constants.ts`

### 1. Multi-edit support — Go's atomic multi-edit vs upstream's single-edit-per-call

| Aspect | Go (`multi_edit.go`) | Upstream (`FileEditTool.ts`) |
|--------|---------------------|------------------------------|
| Schema | `edits: array<{old_string, new_string, replace_all}>` — multiple edits in one call (`multi_edit.go:54-78`) | `old_string, new_string, replace_all` — one edit per call (`types.ts:6-19`) |
| Multi-edit internally | Yes — edits applied sequentially in one `Execute()` | Yes — `getPatchForEdits()` accepts `FileEdit[]` but is called with a single-element array (`utils.ts:247-253`, `FileEditTool.ts:482-488`) |
| API exposure | Single tool call can apply N edits atomically | Only exposed as single-edit; multi-edit is internal-only used for diff reconstruction (`utils.ts:495-524`) |

**Go's multi_edit is a genuine multi-edit API.** The upstream's `Edit` tool only accepts one edit per invocation. The upstream's `getPatchForEdits()` function exists internally for computing patches from multiple edits, but the tool input schema (`types.ts:6-19`) only has `old_string`, `new_string`, `replace_all` — no `edits` array. The `inputsEquivalent` method wraps single edits into a one-element array to call `areFileEditsInputsEquivalent` (`FileEditTool.ts:363-386`), confirming the tool is fundamentally single-edit at the API level.

### 2. Edit conflict detection — Go's overlap detection vs upstream

| Aspect | Go (`multi_edit.go`) | Upstream (`utils.ts`) |
|--------|---------------------|----------------------|
| Mechanism | Tracks `appliedNewStrings[]`; checks if `oldTrimmed` is a substring of any previously applied `new_string` (`multi_edit.go:174-188`) | Identical: tracks `appliedNewStrings[]`; checks if `oldStringToCheck` is a substring of any previous `new_string` (`utils.ts:272,299-311`) |
| Error message | `"old_string is a substring of a new_string from a previous edit"` (`multi_edit.go:184`) | `"Cannot edit file: old_string is a substring of a new_string from a previous edit."` (`utils.ts:307-309`) |
| Stripping | `strings.TrimRight(e.old, "\n")` before check (`multi_edit.go:178`) | `edit.old_string.replace(/\n+$/, '')` before check (`utils.ts:299`) |

**Effectively identical** conflict detection logic. Both trim trailing newlines from old_string before the substring check, both track applied new_strings, and both throw/return error when overlap is detected.

### 3. Atomic rollback — Go's rollback on failure vs upstream

| Aspect | Go (`multi_edit.go`) | Upstream (`FileEditTool.ts`) |
|--------|---------------------|------------------------------|
| Rollback mechanism | **None** — Go validates all edits in a dry-run pass over the content, but if the final `os.WriteFile` fails, the file is **not** restored (`multi_edit.go:176-231`) | **None** — upstream validates via `validateInput()`, then applies in `call()`. If writing fails, the file is **not** restored (`FileEditTool.ts:387-574`) |
| Dry run | Go applies all edits to an in-memory copy of content first. If any edit fails to find old_string, returns error before writing (`multi_edit.go:177-216`) | Upstream validates in `validateInput()` which checks that old_string exists (`FileEditTool.ts:316-327`), then applies in `call()` which calls `getPatchForEdit` → `getPatchForEdits` (`FileEditTool.ts:482-488`, `utils.ts:234-254`) |
| File history | **No file history/undo** | `fileHistoryTrackEdit()` backs up file before writing (`FileEditTool.ts:432-440`) |

**Neither has true atomic rollback.** Go validates all edits against an in-memory copy before writing, which prevents partial application. Upstream splits validation and execution across `validateInput()` and `call()`, creating a TOCTOU window. However, upstream does have file history/undo support via `fileHistoryTrackEdit()` that Go lacks entirely.

### 4. Edit application order — top-to-bottom vs reverse

| Aspect | Go (`multi_edit.go`) | Upstream (`utils.ts`) |
|--------|---------------------|----------------------|
| Order | **Top-to-bottom (forward)** — iterates `edits` slice in order (`multi_edit.go:177`) | **Top-to-bottom (forward)** — iterates `edits` array in order (`utils.ts:297`) |
| Rationale | Each edit is applied to the result of the previous one, so order matters | Same — sequential application |

**Both apply edits top-to-bottom (forward).** Since each edit mutates the content string, both iterate in the order given. The overlap detection prevents edits from matching against text inserted by a previous edit's new_string.

### 5. Dry run support — upstream's dryRun parameter vs Go

| Aspect | Go (`multi_edit.go`) | Upstream |
|--------|---------------------|----------|
| Explicit dryRun param | **No** — no `dryRun` field in schema (`multi_edit.go:46-78`) | **No** — no `dryRun` field in input schema (`types.ts:6-19`) |
| Implicit dry run | Yes — validates all edits in-memory before writing (`multi_edit.go:177-216`) | Yes — `validateInput()` checks string existence before `call()` writes (`FileEditTool.ts:137-361`) |

**Neither has an explicit dryRun parameter.** Both effectively perform a "dry run" by validating edits against the file content before writing. Go does this inline in `Execute()`, upstream does it in the separate `validateInput()` phase.

### 6. Edit validation — what checks are performed before applying

| Check | Go (`multi_edit.go`) | Upstream (`FileEditTool.ts`) |
|-------|---------------------|------------------------------|
| file_path required | Yes (`multi_edit.go:91-93`) | Yes (via zod schema `types.ts:8`) |
| edits required & non-empty | Yes (`multi_edit.go:109-119`) | N/A — single edit always present |
| old_string non-empty | Yes (`multi_edit.go:135-137`) | No — empty old_string means "create new file" (`FileEditTool.ts:226-264`) |
| File exists | Yes — `os.ReadFile`, returns error if not found (`multi_edit.go:148-153`) | Yes — validates existence and suggests similar files (`FileEditTool.ts:224-246`) |
| File too large | 1 GiB guard (`multi_edit.go:142-146`) | 1 GiB guard (`FileEditTool.ts:84,186-200`) |
| UNC path blocking | Yes (`multi_edit.go:98-100`) | Yes (`FileEditTool.ts:177-181`) |
| Read-before-write | Yes — `CheckFileStale()` (`multi_edit.go:103-107`) | Yes — `readFileState` check (`FileEditTool.ts:275-311`) |
| Stale file detection | Yes — `CheckFileStale()` compares timestamps | Yes — compares mtime, with Windows content-fallback (`FileEditTool.ts:290-311`) |
| CRLF normalization | Yes (`multi_edit.go:156-164`) | Yes — `replaceAll('\r\n', '\n')` (`FileEditTool.ts:214`) |
| Quote normalization | Yes — `normalizeQuotes()` (`multi_edit.go:167-171`) | Yes — `findActualString()` + `normalizeQuotes()` (`utils.ts:31-37,73-93`) |
| Quote style preservation | **No** | Yes — `preserveQuoteStyle()` (`utils.ts:104-136`) |
| Desanitization | Yes — `desanitize()` with `DESANITIZATIONS` map (`multi_edit.go:12-31,193-200,249-256`) | Yes — `desanitizeMatchString()` with same map (`utils.ts:531-574`) |
| Trailing newline stripping for match | Yes — `findEditLocation()` tries trimmed (`multi_edit.go:236-247`) | No — upstream uses `findActualString()` which only normalizes quotes (`utils.ts:73-93`) |
| Multiple match check | **No** — Go silently replaces first occurrence | Yes — returns error if multiple matches and `replace_all` is false (`FileEditTool.ts:329-343`) |
| old_string === new_string | **No** — Go allows no-op edits | Yes — rejects identical old/new (`FileEditTool.ts:148-156`) |
| Notebook blocking | **No** | Yes — `.ipynb` files redirected to NotebookEditTool (`FileEditTool.ts:266-273`) |
| Permission settings deny | **No** — Go uses `CheckPathSafetyForAutoEdit()` | Yes — checks `matchingRuleForInput()` for deny rules (`FileEditTool.ts:159-174`) |
| Team memory secret check | **No** | Yes — `checkTeamMemSecrets()` (`FileEditTool.ts:144-147`) |
| Settings file validation | **No** | Yes — `validateInputForSettingsFileEdit()` (`FileEditTool.ts:346-359`) |
| LSP notification | **No** | Yes — `lspManager.changeFile()` / `saveFile()` (`FileEditTool.ts:493-514`) |
| VSCode notification | **No** | Yes — `notifyVscodeFileUpdated()` (`FileEditTool.ts:517`) |
| UTF-16LE encoding | **No** — Go assumes UTF-8 | Yes — detects BOM and decodes as `utf16le` (`FileEditTool.ts:208-214`) |
| Line ending preservation | Yes — `RestoreCRLF()` on output (`multi_edit.go:219-221`) | Yes — `writeTextContent()` with detected line endings (`FileEditTool.ts:491`) |
| Skill discovery | **No** | Yes — `discoverSkillDirsForPaths()` (`FileEditTool.ts:407-423`) |
| Edit result format | `"Applied N edits to <path>"` (`multi_edit.go:231`) | Returns structured patch + original content + git diff (`FileEditTool.ts:561-574`) |

**Key differences:**
- Go allows empty old_string? No — Go rejects it (`multi_edit.go:135-137`). Upstream allows empty old_string for file creation.
- Go does NOT check for multiple matches when replace_all is false — it silently does first replacement. Upstream errors.
- Go does NOT preserve curly quote style in new_string. Upstream does via `preserveQuoteStyle()`.
- Go does NOT detect UTF-16LE encoding. Upstream does.
- Go does NOT notify LSP servers or VSCode about edits.
- Upstream has much richer validation (team secrets, settings files, notebooks, permission deny rules).

### 7. Line number vs search/replace — which approach each uses

| Aspect | Go (`multi_edit.go`) | Upstream (`FileEditTool.ts`) |
|--------|---------------------|------------------------------|
| Approach | **Search/replace** — `old_string` / `new_string` pairs (`multi_edit.go:59-68`) | **Search/replace** — `old_string` / `new_string` pairs (`types.ts:8-13`) |
| Line number support | **No** | **No** |
| Fuzzy matching | Trailing newline stripping (`multi_edit.go:240-246`), desanitization (`multi_edit.go:193-200`), quote normalization (`multi_edit.go:167-171`) | Quote normalization (`utils.ts:73-93`), desanitization (`utils.ts:531-574`), trailing whitespace stripping for non-markdown (`utils.ts:44-64,597-644`) |

**Both use search/replace** — neither uses line numbers. The matching logic is very similar (exact match → quote normalization → desanitization), though upstream additionally strips trailing whitespace from new_string for non-markdown files (`utils.ts:597-644`), which Go does not do.

---


---

## Section XXIII. Round 28 — Tool-Level Code Differences (Read/Write/Edit/Bash)

**Date**: 2026-05-12

**Go files**: `E:\Git\miniClaudeCode-go-github\tools\file_read.go` (469 lines), `E:\Git\miniClaudeCode-go-github\tools\file_write.go` (101 lines), `E:\Git\miniClaudeCode-go-github\tools\file_edit.go` (414 lines), `E:\Git\miniClaudeCode-go-github\tools\exec_tool.go` (1622 lines), `E:\Git\miniClaudeCode-go-github\tools\exec_tool_windows.go` (37 lines)

**Upstream files**: `E:\Git\claude-code-upstream\packages\builtin-tools\src\tools\FileReadTool\FileReadTool.ts` (1184 lines), `E:\Git\claude-code-upstream\packages\builtin-tools\src\tools\FileWriteTool\FileWriteTool.ts` (435 lines), `E:\Git\claude-code-upstream\packages\builtin-tools\src\tools\FileEditTool\FileEditTool.ts` (626 lines), `E:\Git\claude-code-upstream\packages\builtin-tools\src\tools\FileEditTool\utils.ts` (776 lines), `E:\Git\claude-code-upstream\packages\builtin-tools\src\tools\BashTool\bashSecurity.ts` (2500+ lines), `E:\Git\claude-code-upstream\packages\builtin-tools\src\tools\BashTool\bashPermissions.ts` (2200+ lines), `E:\Git\claude-code-upstream\src\utils\file.ts` (writeTextContent at line 84)

This section documents differences found by reading actual source code that were NOT captured in earlier sections (9-12, deep comparison at line 7776).

### Read Tool — Newly Identified Differences

| # | Aspect | Go (`file_read.go`) | Upstream (`FileReadTool.ts`) | Type |
|---|--------|---------------------|----------------------------|------|
| 1 | `pages` parameter for PDFs | **Not in schema** — `InputSchema` has only `file_path`, `offset`, `limit` | `pages: z.string().optional()` for PDF page ranges (e.g., `"1-5"`, `"10-20"`). Max `PDF_MAX_PAGES_PER_READ` pages per request (`FileReadTool.ts:236-242`) | 缺失 |
| 2 | Token budget validation | Fixed 256KB `maxFileSize` size limit, no token counting | `validateContentTokens()` calls `roughTokenCountEstimationForFileType` + `countTokensWithAPI` against `maxTokens` budget. Throws `MaxFileReadTokenExceededError` with token counts (`FileReadTool.ts:755-772`) | 简化 |
| 3 | Efficient partial reads | Reads entire file via `os.ReadFile`, then splits into lines | `readFileInRange()` async with `abortController.signal`, reads only needed bytes. Returns `{content, lineCount, totalLines, totalBytes, readBytes, mtimeMs}` (`FileReadTool.ts:1020-1028`) | 简化 |
| 4 | Output schema | Plain string with `cat -n` format | Discriminated union: `text`, `image`, `notebook`, `pdf`, `parts`, `file_unchanged` — each with typed `file` sub-object (`FileReadTool.ts:248-332`) | 简化 |
| 5 | `pickLineFormatInstruction()` | Fixed `cat -n` line prefix format | Conditional format via `pickLineFormatInstruction()` — respects `LINE_FORMAT_INSTRUCTION` from prompt config (`FileReadTool.ts:720-722`) | 简化 |
| 6 | `isConcurrencySafe()` marker | Not present | Marked `isConcurrencySafe() { return true }` — signals concurrent-read safety to the tool runner (`FileReadTool.ts:373-375`) | 缺失 |
| 7 | File read listener system | `Registry` tracks reads internally for staleness | `registerFileReadListener(callback)` — pub/sub system notifying LSP, file history, etc. when files are read. Callback receives `(filePath, content)` (`FileReadTool.ts:162-173`) | 缺失 |
| 8 | Memory file freshness | Not present | `memoryFileMtimes` WeakMap + `memoryFileFreshnessPrefix()` — annotates auto-memory file reads with recency info (`FileReadTool.ts:747-753`) | 缺失 |
| 9 | Cyber risk mitigation reminder | Not in output | `CYBER_RISK_MITIGATION_REMINDER` appended to file content for non-exempt models. Exempt: `claude-opus-4-6` (`FileReadTool.ts:729-738`) | 缺失 |

### Write Tool — Newly Identified Differences

| # | Aspect | Go (`file_write.go`) | Upstream (`FileWriteTool.ts`) | Type |
|---|--------|---------------------|----------------------------|------|
| 1 | CRLF/line ending preservation | `os.WriteFile(fp, []byte(content), 0o644)` — writes raw content as-is | `writeTextContent(fullFilePath, content, enc, 'LF')` — explicit `'LF'` line ending parameter. If file was CRLF, model's `\n` become `\r\n`. `writeTextContent` normalizes: `content.replaceAll('\r\n', '\n').split('\n').join('\r\n')` (`FileWriteTool.ts:305`, `file.ts:84-98`) | Go适配 |
| 2 | Output schema | `"Wrote N chars to /path/to/file"` | Structured: `{type: 'create'\|'update', filePath, content, structuredPatch, originalFile, gitDiff?}` — includes full diff hunk array and original content for downstream consumers (`FileWriteTool.ts:68-88`) | 简化 |
| 3 | Structured patch computation | Not computed | `getPatchForDisplay({filePath, fileContents: oldContent, edits: [{old_string: oldContent, new_string: content}]})` — generates unified diff hunks for both create and update (`FileWriteTool.ts:360-370`) | 缺失 |
| 4 | Type discrimination | Not present | Output distinguishes `create` (new file) vs `update` (existing file modified), affecting downstream result messages (`FileWriteTool.ts:418-432`) | 简化 |
| 5 | `writeTextContent` with encoding | Direct `os.WriteFile` | `writeTextContent(filePath, content, encoding, endings)` — writes with detected encoding (UTF-8/UTF-16 LE) and detected line endings (LF/CRLF) (`file.ts:84-98`) | 简化 |
| 6 | Max content size | 10MB: `const maxWriteSize = 10 * 1024 * 1024` | `maxResultSizeChars: 100_000` for result rendering; no explicit content size limit in write itself | Go适配 (Go has explicit size guard) |

### Edit Tool — Newly Identified Differences

| # | Aspect | Go (`file_edit.go`) | Upstream (`FileEditTool.ts` + `utils.ts`) | Type |
|---|--------|---------------------|----------------------------------------|------|
| 1 | `replace_all` in upstream schema | **Go has `replace_all` parameter** (documented as Go增强 in existing section 11.8) | **Upstream ALSO has `replace_all`** in `FileEditInput` type: `replace_all?: boolean` (default false). Existing diff says "上游: No native replace_all" — this is incorrect. Upstream supports it (`types.ts`, `FileEditTool.ts:398`) | 修正 |
| 2 | `inputsEquivalent` handler for dedup | Not present | `inputsEquivalent(input1, input2)` → `areFileEditsInputsEquivalent()` — compares two edit inputs by applying both to file content and checking results. Prevents duplicate tool calls from inflating context (`FileEditTool.ts:363-385`, `utils.ts:732-775`) | 缺失 |
| 3 | Multi-edit via `getPatchForEdits` | Single edit: one `old_string` → one `new_string` | `getPatchForEdits({filePath, fileContents, edits: FileEdit[]})` — processes array of edits in sequence. Tracks `appliedNewStrings` to detect substring overlap between edits (`utils.ts:262-350`) | 缺失 |
| 4 | Substring overlap detection | Not present | Before each edit, checks `oldStringToCheck` against all `previousNewString` values. Throws `"Cannot edit file: old_string is a substring of a new_string from a previous edit"` (`utils.ts:302-311`) | 缺失 |
| 5 | Desanitization lookup table | Go has inline `desanitize()` call | Explicit `DESANITIZATIONS` map: `<fnr>`→`<function_results>`, `<n>`→`<name>`, `<o>`→`<output>`, `<e>`→`<error>`, `<s>`→`<system>`, `<r>`→`<result>`, `<META_START>`, `<EOT>`, `<META>`, `<SOS>`, `\n\nH:`→`\n\nHuman:`, `\n\nA:`→`\n\nAssistant:` (`utils.ts:531-550`) | 简化 |
| 6 | Markdown trailing whitespace handling | Strips trailing whitespace for all non-md/mdx files | `isMarkdown = /\.(md\|mdx)$/i.test(file_path)` — markdown preserves trailing spaces (hard line breaks). Non-markdown uses `stripTrailingWhitespace()` (`utils.ts:597-612`) | Go适配 (same behavior) |
| 7 | Patch tab normalization | No tab handling in patch generation | `convertLeadingTabsToSpaces(fileContents)` before `structuredPatch` — ensures patches display with spaces instead of tabs for consistency (`utils.ts:345-346`) | 缺失 |
| 8 | `validateInput` file size check | Size check done in `Execute()`: `info.Size() > maxEditSize` (1 GiB) | Size check in `validateInput()`: `fs.stat(fullFilePath).size > MAX_EDIT_FILE_SIZE` (1 GiB) — pre-I/O validation before permission prompt (`FileEditTool.ts:186-200`) | Go适配 |
| 9 | Empty old_string on existing file | Checks `strings.TrimSpace(string(existingData)) != ""` — allows write to existing empty file | Checks `fileContent.trim() !== ""` — same behavior, rejects non-empty files. Returns `"Cannot create new file - file already exists."` (`FileEditTool.ts:249-264`) | Go适配 (same behavior) |
| 10 | `readFileForEdit` helper | Direct `os.ReadFile` in `Execute()` | `readFileForEdit(absoluteFilePath)` wraps `readFileSyncWithMetadata` — returns `{content, fileExists, encoding, lineEndings}` for proper encoding/ending preservation (`FileEditTool.ts:599-625`) | 简化 |

### Bash/Exec Tool — Newly Identified Differences

| # | Aspect | Go (`exec_tool.go` + platform files) | Upstream (`bashPermissions.ts` + `bashSecurity.ts`) | Type |
|---|--------|--------------------------------------|--------------------------------------------------|------|
| 1 | Subcommand count cap | No limit on compound command segments | 50 subcommand cap: `const MAX_SUBCOMMANDS = 50`. Prevents exponential growth from complex compound commands that would starve the event loop (`bashPermissions.ts:100-103`) | 缺失 |
| 2 | Heredoc handling | No heredoc parsing or analysis | `extractHeredocs()` from command string. `stripSafeHeredocSubstitutions()` removes safe heredoc content before validation. Heredoc analysis integrated with Tree-sitter AST (`bashSecurity.ts`, `bashPermissions.ts:2096-2100`) | 缺失 |
| 3 | Image output handling in stdout | All output treated as plain text | Detects base64 image data URIs in stdout. Resizes images, returns as image content blocks. `utils.ts:49-131`, `BashTool.tsx:790-797` | 缺失 |
| 4 | Sed edit simulation | Runs sed as actual subprocess command | Intercepts `sed -i` commands and applies edits via `applySedEdit()` instead of executing sed. Ensures exact file writes without running potentially dangerous sed commands (`BashTool.tsx:579-638`) | 缺失 |
| 5 | Claude Code hint stripping | No hint tag processing | Strips `<claude-code-hint />` tags from stdout before sending to model. These tags are used by the TUI for internal signaling (`BashTool.tsx:1043-1048`) | 缺失 |
| 6 | Output accumulation strategy | `readLimited()` — head-only, caps at N bytes | `EndTruncatingAccumulator` — preserves both head AND tail of output. For large output, persists to disk and sends `<persisted-output>` with preview (`BashTool.tsx:988-1012`) | 简化 |
| 7 | Sleep pattern blocking | No sleep pattern detection | `detectBlockedSleepPattern()` blocks `sleep N` (N≥2) as first command — prevents stalling. Sleep cannot be auto-backgrounded (`BashTool.tsx:534-552`, `BashTool.tsx:328-331`) | 缺失 |
| 8 | Assistant-mode auto-background | No assistant-mode distinction | 15s budget for assistant mode: commands exceeding 15s are auto-backgrounded without explicit request (`BashTool.tsx:1277-1293`) | 缺失 |
| 9 | Manual background (Ctrl+B) | No manual background support | User can press Ctrl+B to background running command. `registerForeground()`/`unregisterForeground()` for foreground task tracking. `startBackgrounding()` transition (`BashTool.tsx:1410-1423`) | 缺失 |
| 10 | Git index lock detection | No special handling for `.git/index.lock` | Logs events when `.git/index.lock` errors detected in stderr. Analytics event for tracking lock contention (`BashTool.tsx:936-941`) | 缺失 |
| 11 | Code indexing tool tracking | No tracking | Detects and logs usage of code indexing tools (ctags, cscope, etc.) for telemetry (`BashTool.tsx:1026-1034`) | 缺失 |
| 12 | Command type classification | `isReadOnlyCommand()` binary check | Classifies commands as `search`/`read`/`list`/`silent` for UI display and permission decisions. `isSearchOrReadCommand()` for routing (`BashTool.tsx:120-267`) | 简化 |
| 13 | Zsh equals expansion blocking | No Zsh equals expansion protection | Blocks `=cmd` at word start (e.g., `=curl evil.com` → `/usr/bin/curl evil.com`) which bypasses deny rules. Pattern: `(?:^|[\s;&|])=[a-zA-Z_]` (`bashSecurity.ts:24-27`) | 缺失 |
| 14 | Progress threshold / UI feedback | No progress feedback | Shows `<BackgroundHint />` UI after 2s for running commands. Async generator yields progress updates every ~1s (`BashTool.tsx:115`, `BashTool.tsx:1449-1457`) | 缺失 |
| 15 | Cd + git compound command block | No compound cd+git protection | Blocks `cd X && git status` pattern to prevent bare repo RCE attack. Special case in `bashPermissions.ts:2209-2225` | 缺失 |
| 16 | `checkPermissionMode` validation | No mode-specific validation | `checkPermissionMode()` validates that command is appropriate for current permission mode (auto/accept/dontAsk). Different modes have different constraint sets (`modeValidation.ts`) | 缺失 |
| 17 | `checkSedConstraints` validation | sed runs as subprocess without special validation | `checkSedConstraints()` validates sed commands before execution/simulation. Prevents dangerous sed patterns (`sedValidation.ts`) | 缺失 |
| 18 | Safe redirection stripping (validators) | `stripSafeWrappers` strips command wrappers | `stripSafeRedirections()` removes `2>&1`, `>/dev/null`, `<>/dev/null` before security analysis. Tracks pre-strip content for validators to avoid false negatives (`bashSecurity.ts:176-188`) | 缺失 |

---

*End of Section XXIII. 35 new differences across 4 tools: 9 Read, 6 Write, 10 Edit (incl. 1 correction), 18 Bash/Exec.*


---

## 42. Tools Deep Dive — FileRead/Write/Edit, Bash, Grep/Glob, Agent/Skills, WebFetch/Search

### Files Compared
- **Go**: `tools/file_read.go`, `tools/file_write.go`, `tools/file_edit.go`, `tools/exec_tool.go`, `tools/grep_tool.go`, `tools/glob_tool.go`, `tools/list_dir.go`, `tools/agent_tool.go`, `tools/web_fetch.go`, `tools/web_search.go`
- **Upstream**: `FileReadTool.ts`, `FileWriteTool.ts`, `FileEditTool.ts`, `BashTool.tsx`, `GrepTool.ts`, `GlobTool.ts`, `ListDirTool.ts`, `AgentTool.tsx`, `WebFetchTool.ts`, `WebSearchTool.ts`, `NotebookEditTool.ts`

### 42.1 FileReadTool

| # | Aspect | Go (`tools/file_read.go`) | Upstream (`FileReadTool.ts`) | Type |
|---|--------|--------------------------|-----------------------------|------|
| 1 | Image/PDF support | No native image rendering, PDF reading/extraction (lines 103-108) | Full support: readImageWithTokenBudget(), readPDF(), extractPDFPages() (lines 866-1017) | 缺失 |
| 2 | Notebook reading | Reads .ipynb as plain JSON text (line 36 in description only) | Dedicated notebook path: readNotebook() with cell mapping (lines 822-863) | 简化 |
| 3 | Token-based content validation | No countTokensWithAPI() or token estimation | validateContentTokens() with API token counting (lines 755-772) | 缺失 |
| 4 | Skill discovery on file read | No activateConditionalSkillsForPaths or discoverSkillDirsForPaths | Fire-and-forget skill discovery and activation (lines 577-591) | 缺失 |
| 5 | Image handling | Images rejected as binary extension | Image detection, resizing, compression with token budget (lines 1097-1183) | 缺失 |
| 6 | Memory freshness note | No memoryFreshnessNote for auto-memory files | memoryFileFreshnessPrefix via WeakMap (lines 747-753) | 缺失 |
| 7 | FileReadListener | No event listener pattern | registerFileReadListener callback system (lines 162-173) | 缺失 |
| 8 | Cyber risk mitigation reminder | No CYBER_RISK_MITIGATION_REMINDER appended to results | Appended to non-exempt model results (lines 729-738) | 缺失 |
| 9 | Magic bytes detection | Extensive magic byte detection for 20+ binary formats (lines 342-459) | Only hasBinaryExtension() string check | Go增强 |
| 10 | Concurrent read/write retry | Retries stat() after 50ms for concurrent batch execution (lines 82-90) | No explicit retry for concurrent batch | Go增强 |
| 11 | Pagination hint | Matches upstream offset/limit pagination pattern (lines 243-247) | offset/limit with startLine/totalLines (lines 1046-1054) | 修正 |
| 12 | UTF-16 LE BOM support | Matches upstream UTF-16 LE detection (lines 191-194) | UTF-16 LE encoding detection in readFileForEdit (line 210) | 修正 |

### 42.2 FileWriteTool

| # | Aspect | Go (`tools/file_write.go`) | Upstream (`FileWriteTool.ts`) | Type |
|---|--------|--------------------------|-----------------------------|------|
| 1 | Read-before-write validation | Basic registry check (lines 76-79) | Full validateInput with mtime + content comparison (lines 153-221) | 简化 |
| 2 | LSP integration | No LSP server notification on write | Notifies LSP servers via didChange/didSave (lines 308-326) | 缺失 |
| 3 | Skill discovery | No discoverSkillDirsForPaths | Skill discovery on file write (lines 232-245) | 缺失 |
| 4 | Git diff | No fetchSingleFileGitDiff | Optional git diff computation (lines 344-357) | 缺失 |
| 5 | File history tracking | No fileHistoryTrackEdit | File history backup support (lines 255-264) | 缺失 |
| 6 | Structured diff output | Returns simple "Wrote N chars" message (line 99) | Returns structured output with patch, type create/update (lines 359-416) | 简化 |
| 7 | Team memory secrets guard | No checkTeamMemSecrets | Secret detection in written content (lines 157-160) | 缺失 |
| 8 | Content size limit | 10MB Hard limit (line 63) | No explicit content size limit; upstream relies on maxResultSizeChars | Go适配 |

### 42.3 FileEditTool

| # | Aspect | Go (`tools/file_edit.go`) | Upstream (`FileEditTool.ts`) | Type |
|---|--------|--------------------------|-----------------------------|------|
| 1 | String matching with quote normalization | Basic string find/replace | findActualString() for quote normalization, preserveQuoteStyle() (lines 316-377) | 简化 |
| 2 | LSP integration | No LSP notification on edit | LSP didChange/didSave after edit (lines 494-514) | 缺失 |
| 3 | Multi-edit (simultaneous) | Single old_string/new_string | Dedicated multi_edit tool for multiple edits in one call | 简化 |
| 4 | Git diff | No fetchSingleFileGitDiff | Optional git diff for remote sessions (lines 546-558) | 缺失 |
| 5 | Settings file validation | No validateInputForSettingsFileEdit | Validates edits to settings files (lines 346-359) | 缺失 |
| 6 | replace_all support | Supports replace_all parameter | Same replace_all functionality (line 332-343) | 修正 |
| 7 | Concurrent modification detection | Matches upstream mtime + content fallback | Windows timestamp staleness with content fallback (lines 291-310) | 修正 |

### 42.4 Bash/ExecTool

| # | Aspect | Go (`tools/exec_tool.go`) | Upstream (`BashTool.tsx`) | Type |
|---|--------|--------------------------|--------------------------|------|
| 1 | Command deny patterns | Comprehensive deny regex list (lines 78-113) | Similar deny pattern matching | 修正 |
| 2 | Command substitution detection | Full $(), ${}, backtick, process substitution detection (lines 619-680) | Shell injection detection | 修正 |
| 3 | Glob/brace expansion detection | Unquoted glob, bracket, brace detection (lines 710-790) | Glob safety checks | 修正 |
| 4 | Safe wrapper stripping | Comprehensive wrapper stripping: timeout, nice, nohup, env, sudo (lines 1337-1421) | Safe wrapper handling | 修正 |
| 5 | Read-only command classification | Extensive isReadOnlyCommand with git subcommand analysis (lines 1437-1544) | Basic read-only classification | Go增强 |
| 6 | Destructive command detection | isDestructiveCommand with docker/kubectl/terraform/git analysis (lines 1547-1621) | Destructive detection | Go增强 |
| 7 | Path validation for deletion | Comprehensive dangerous path detection with Windows drive root child matching (lines 1148-1238) | Path safety checks | Go增强 |
| 8 | Redirection target validation | validateRedirectTargets blocks writes to sensitive paths (lines 954-1089) | Redirect validation | Go增强 |
| 9 | Timeout/auto-backgrounding | Timeout continues process in background via TimeoutCallback (lines 352-426) | Timeout with auto-backgrounding | 修正 |

### 42.5 GrepTool & GlobTool

| # | Aspect | Go (`grep_tool.go`, `glob_tool.go`) | Upstream (`GrepTool.ts`, `GlobTool.ts`) | Type |
|---|--------|-----------------------------------|----------------------------------------|------|
| 1 | Grep implementation | Shell exec-based via exec command | Native file traversal with ripgrep integration | 简化 |
| 2 | Binary file exclusion | Matches upstream binary file exclusion | Binary file filtering | 修正 |
| 3 | Ignore patterns | .gitignore-aware traversal | Git ignore integration | 修正 |
| 4 | Doublestar glob support | Uses doublestar library for ** glob patterns (line 138) | Similar glob matching | 修正 |
| 5 | Auto-prefix **/ | Auto-prepends **/ when no slash in pattern (lines 107-111) | Auto-prefix behavior | 修正 |
| 6 | Result sorting by mtime | Sorts results by modification time (lines 153-164) | Sorts by modification time | 修正 |

### 42.6 AgentTool / Sub-agent System

| # | Aspect | Go (`tools/agent_tool.go` + `agent_sub.go`) | Upstream (`AgentTool.tsx`) | Type |
|---|--------|-------------------------------------------|---------------------------|------|
| 1 | Agent types | explore/plan/verify/fork agent types with detailed prompts (agent_sub.go:43-242) | Built-in agents + loadAgentsDir dynamic loading | 修正 |
| 2 | Fork mode | inherit_context with full parent context cloning (agent_sub.go:832-885) | buildForkedMessages, forkSubagent.js | 修正 |
| 3 | Worktree isolation | No git worktree support for agent isolation | createAgentWorktree/hasWorktreeChanges/removeAgentWorktree (line 42) | 缺失 |
| 4 | Remote agent execution | No remote/CCR agent execution | checkRemoteAgentEligibility, registerRemoteAgentTask, teleportToRemote (lines 15, 39) | 缺失 |
| 5 | Agent Teams (swarms) | No teammate spawning, team management | spawnTeammate, isTeammate, isInProcessTeammate, team_name (lines 46, 37-38) | 缺失 |
| 6 | Tool filtering layers | 4-layer filtering: global/async/type/caller (agent_sub.go:582-641) | assembleToolPool with filtering | 修正 |
| 7 | Background execution | All sub-agents run async with task output capture (agent_tool.go:143) | run_in_background: true/false (line 87) | Go增强 |
| 8 | Progress updates | Basic agent notification via EnqueueAgentNotification | Full progress tracking with createProgressTracker, emitTaskProgress (line 14) | 简化 |
| 9 | Agent color management | No setAgentColor for UI grouping | agentColorManager for display (line 47) | 缺失 |
| 10 | MCP server requirements | No filterAgentsByMcpRequirements | MCP server requirement filtering (lines 17-19, 53) | 缺失 |
| 11 | Agent summarization | No startAgentSummarization | Agent summarization for long-running agents (line 10) | 缺失 |

### 42.7 Skills System

| # | Aspect | Go (`skills/` + `tools/skill_tools.go`) | Upstream (`loadSkillsDir.ts` + `SkillTool.ts`) | Type |
|---|--------|----------------------------------------|-----------------------------------------------|------|
| 1 | Skill discovery | File-based SKILL.md parsing with frontmatter | Full loadSkillsDir with realpath dedup, ignore patterns, gitignore checks | 简化 |
| 2 | MCP skills | No MCP skill integration | registerMCPSkillBuilders, MCP prompt/skill loading | 缺失 |
| 3 | Bundled skills | Loads from filesystem | 20+ bundled skills (verify, loop, simplify, remember, etc.) | 简化 |
| 4 | Skill execution as sub-agent | No skill invocation via runAgent() | SkillTool invokes skills as sub-agents with runAgent() (line 62) | 缺失 |
| 5 | Skill improvement tracking | No skillImprovement.ts hook | skillImprovement.ts tracks and suggests skill improvements | 缺失 |
| 6 | Skill search | Weighted term scoring with name/description/tags/whenToUse (skill_tools.go:184-223) | Basic skill discovery via getCommands | Go增强 |

### 42.8 NotebookEditTool (Missing)

| # | Aspect | Go | Upstream (`NotebookEditTool.ts:1-491`) | Type |
|---|--------|----|---------------------------------------|------|
| 1 | Notebook editing tool | No dedicated NotebookEditTool | Full NotebookEditTool with cell replace/insert/delete (lines 90-490) | 缺失 |
| 2 | Cell operations | No cell_id, cell_type, edit_mode support | Cell operations: replace, insert, delete with language detection (lines 295-456) | 缺失 |
| 3 | Notebook read | Returns raw JSON of .ipynb file | Dedicated readNotebook() with cell mapping (lines 822-863) | 简化 |

### 42.9 WebFetchTool

| # | Aspect | Go (`tools/web_fetch.go`) | Upstream (`WebFetchTool.ts`) | Type |
|---|--------|--------------------------|-----------------------------|------|
| 1 | Content extraction | Basic HTML tag stripping (extractTextFromHTML, stripHTMLSimple) | LLM-powered extraction via applyPromptToMarkdown with Haiku model (line 271) | 简化 |
| 2 | Redirect handling | No redirect detection/follow-up message | Full redirect detection with follow-up instructions (lines 217-249) | 缺失 |
| 3 | Preapproved hosts | No isPreapprovedHost list | Preapproved host list with automatic allow (lines 109-121) | 缺失 |
| 4 | Binary content handling | No disk persistence for binary content | Binary content saved to disk with mime-derived extension (lines 283-285) | 缺失 |
| 5 | Permission system | Basic file:// and internal URL blocking | Full permission rule system with allow/deny/ask rules (lines 104-179) | 简化 |

### 42.10 WebSearchTool

| # | Aspect | Go (`tools/web_search.go`) | Upstream (`WebSearchTool.ts`) | Type |
|---|--------|--------------------------|-----------------------------|------|
| 1 | Search implementation | Bing/360 HTML scraping with regex parsing (searchBing, search360) | Native Anthropic API web_search_20250305 tool via queryModelWithStreaming (lines 268-291) | 简化 |
| 2 | Domain filtering | No allowed_domains/blocked_domains | Full allowed_domains and blocked_domains support (lines 28-35, 81) | 缺失 |
| 3 | Progress streaming | No progress updates during search | Full streaming with query_update and search_results_received events (lines 346-386) | 缺失 |
| 4 | Multi-model routing | No Haiku routing for search | Optional Haiku routing via tengu_plum_vx3 feature flag (lines 262-281) | 缺失 |
| 5 | Provider support | No firstParty/vertex/foundry provider checks | isEnabled() checks for firstParty, Vertex, Foundry (lines 168-193) | 缺失 |
| 6 | Max uses tracking | No 8-search limit enforcement | max_uses: 8 hardcoded (line 82) | 缺失 |
| 7 | Fallback search | 360 search fallback for China region when Bing fails (lines 194-199) | No fallback search engine | Go增强 |

---


---

### 54.1 File Write Tool

**Go**: `tools/file_write.go` (100 lines) · **Upstream**: `FileWriteTool/FileWriteTool.ts` (435 lines)

| # | Aspect | Go (file:line) | Upstream (file:line) | Type |
|---|--------|----------------|----------------------|------|
| 1 | **Input schema** | `file_path` + `content` only (line 32-46) | `file_path` + `content` (same shape, zod `strictObject`) | Go适配 |
| 2 | **Output schema** | Plain string `"Wrote N chars to X"` (line 99) | Structured: `{type: "create"\|"update", filePath, content, structuredPatch, originalFile, gitDiff}` (line 69-88) | 简化 |
| 3 | **Read-before-write** | Via `registry.CheckFileStale()` — mtime-based (line 76-79) | Via `readFileState.get()` — mtime + content comparison fallback for Windows false positives (line 198-221) | 简化 |
| 4 | **Concurrent modification detection** | `CheckFileStale` returns error message (line 77-79) | Two-phase: `validateInput()` mtime check + `call()` re-checks mtime with content fallback (line 279-295) | 简化 |
| 5 | **Content-based staleness fallback** | No content comparison — mtime only | Compares content when mtime differs on Windows (anti-virus, cloud sync) (line 291) | 缺失 |
| 6 | **File encoding detection** | None — always writes UTF-8 | Detects encoding via `readFileSyncWithMetadata`, writes with `writeTextContent(fullFilePath, content, enc, 'LF')` (line 298, 305) | 缺失 |
| 7 | **Line ending preservation** | Not handled — writes raw content | Explicitly writes `'LF'` regardless of original, with comment explaining why (line 305) | 缺失 |
| 8 | **Max write size** | 10 MB `maxWriteSize` (line 63) | No explicit limit in tool (handled by SDK layer) | Go适配 |
| 9 | **UNC path blocking** | `isUncPath()` check (line 71-73) | Same check in `validateInput` (line 182-184) | ✅ Match |
| 10 | **File sync after write** | `os.Open` + `f.Sync()` (line 91-94) | No explicit sync — relies on `writeTextContent` | Go增强 |
| 11 | **MkdirAll before write** | `os.MkdirAll(dir, 0o755)` (line 83) | `await fs.mkdir(dir)` (line 254) | ✅ Match |
| 12 | **Registry/mark-file-read** | `registry.MarkFileReadWithContent(fp, content)` (line 97) | `readFileState.set(fullFilePath, {content, timestamp, ...})` (line 332-337) | ✅ Match |
| 13 | **LSP notification** | Not present | `lspManager.changeFile()` + `saveFile()` after write (line 309-326) | 缺失 |
| 14 | **VSCode diff notification** | Not present | `notifyVscodeFileUpdated()` (line 329) | 缺失 |
| 15 | **Team memory secret guard** | Not present | `checkTeamMemSecrets(fullFilePath, content)` (line 157-160) | 缺失 |
| 16 | **Permission deny rule check** | `CheckPathSafetyForAutoEdit` only | `matchingRuleForInput()` deny rule check in `validateInput` (line 163-177) | 简化 |
| 17 | **Diff/patch generation** | Not present | `getPatchForDisplay()` generates structured patch (line 360-379) | 缺失 |
| 18 | **Git diff (remote mode)** | Not present | `fetchSingleFileGitDiff()` for remote mode (line 344-357) | 缺失 |
| 19 | **File history tracking** | Not present | `fileHistoryTrackEdit()` (line 255-264) | 缺失 |
| 20 | **Skill discovery** | Not present | `discoverSkillDirsForPaths()` + `activateConditionalSkillsForPaths()` (line 233-245) | 缺失 |
| 21 | **CLAUDE.md analytics** | Not present | `logEvent('tengu_write_claudemd')` (line 340-342) | 缺失 |
| 22 | **Path expansion in backfill** | `expandPath` in Execute only | `backfillObservableInput` expands `~` before hook matching (line 128-131) | 缺失 |

---


---

### 54.2 File Edit Tool

**Go**: `tools/file_edit.go` (414 lines) · **Upstream**: `FileEditTool/FileEditTool.ts` (626 lines)

| # | Aspect | Go (file:line) | Upstream (file:line) | Type |
|---|--------|----------------|----------------------|------|
| 1 | **Input schema** | `file_path`, `old_string`, `new_string`, `replace_all` (line 32-55) | Same + zod `strictObject` | ✅ Match |
| 2 | **Output** | Plain string `"Successfully edited X"` (line 232) | Structured: `{filePath, oldString, newString, originalFile, structuredPatch, userModified, replaceAll, gitDiff}` (line 561-573) | 简化 |
| 3 | **old_string==new_string check** | Returns error (line 89-91) | Returns `{result: false, behavior: 'ask'}` (line 148-155) | 简化 |
| 4 | **Empty old_string → new file** | Handles: checks if file exists with content, creates if empty/absent (line 93-118) | Same behavior in `validateInput` (line 224-263) | ✅ Match |
| 5 | **Quote normalization** | `normalizeQuotes()` — curly→straight matching (line 160-162) | `findActualString()` handles same (imported from utils.js) | ✅ Match |
| 6 | **Quote style preservation** | `preserveQuoteStyle()` — curly quote restoration (line 248-276) | `preserveQuoteStyle()` (imported from utils.js, line 475-479) | ✅ Match |
| 7 | **CRLF normalization** | Normalize for matching, `restoreCRLF()` after (line 165-169, 213-215) | Handled in `writeTextContent` with line ending param | Go适配 |
| 8 | **UTF-16 LE support** | Detect BOM, decode, re-encode on write (line 140-149, 219-222) | Detected in `readFileSyncWithMetadata`, preserved in `writeTextContent` | ✅ Match |
| 9 | **Trailing whitespace strip** | `stripTrailingWhitespace()` except `.md/.mdx` (line 152-156) | Same behavior (done in utils layer) | ✅ Match |
| 10 | **Desanitization fallback** | `desanitize()` for `<fnr>` → `<function_results>` (line 174-181) | `findActualString()` handles same patterns | ✅ Match |
| 11 | **Line deletion handling** | When `newStr==""` and old doesn't end with `\n`, strip trailing `\n` (line 199-210) | Same in `getPatchForEdit()` | ✅ Match |
| 12 | **.ipynb rejection** | Returns error (line 127-129) | Returns `{result: false, behavior: 'ask'}` (line 266-273) | 简化 |
| 13 | **Max edit file size** | 1 GiB `maxEditSize` (line 120) | Same 1 GiB `MAX_EDIT_FILE_SIZE` (line 84) | ✅ Match |
| 14 | **Non-unique old_string error** | Returns `"Warning: old_text appears N times"` as error (line 186-190) | Returns `{result: false, behavior: 'ask'}` with `actualOldString` in meta (line 330-343) | 简化 |
| 15 | **Not-found error** | `"Error: old_text not found"` (line 184) | Returns `{result: false}` with string preview in message (line 317-327) | 简化 |
| 16 | **Similar file suggestion** | Not present | `findSimilarFile()` + `suggestPathUnderCwd()` on ENOENT (line 229-238) | 缺失 |
| 17 | **Is-absolute path meta** | Not present | `isFilePathAbsolute` in validation meta (line 283-285) | 缺失 |
| 18 | **Settings file validation** | Not present | `validateInputForSettingsFileEdit()` (line 346-359) | 缺失 |
| 19 | **Inputs equivalence check** | Not present | `areFileEditsInputsEquivalent()` (line 363-386) | 缺失 |
| 20 | **LSP notification** | Not present | `lspManager.changeFile()` + `saveFile()` (line 494-514) | 缺失 |
| 21 | **VSCode diff notification** | Not present | `notifyVscodeFileUpdated()` (line 517) | 缺失 |
| 22 | **File history tracking** | Not present | `fileHistoryTrackEdit()` (line 431-440) | 缺失 |
| 23 | **Diff/patch generation** | Not present | `getPatchForEdit()` generates structured patch (line 482-488) | 缺失 |
| 24 | **userModified flag** | Not present | Tracks if user modified proposed changes (line 577-578) | 缺失 |
| 25 | **Content-based staleness** | `registry.CheckFileStale` — mtime only | Content comparison fallback when mtime differs (line 453-467) | 缺失 |

---


---

### 54.3 File Read Tool

**Go**: `tools/file_read.go` (469 lines) · **Upstream**: `FileReadTool/FileReadTool.ts` (1184 lines)

| # | Aspect | Go (file:line) | Upstream (file:line) | Type |
|---|--------|----------------|----------------------|------|
| 1 | **Input schema** | `file_path`, `offset`, `limit` (line 41-60) | `file_path`, `offset`, `limit`, `pages` (PDF) (line 227-243) | 缺失 |
| 2 | **Max file size** | 256 KB `maxFileSize` (line 13) | Configurable via `fileReadingLimits.maxSizeBytes` (line 505-507) | Go适配 |
| 3 | **Offset/limit parsing** | Supports `float64`, `int`, `string` coercion (line 131-162) | Zod `semanticNumber` with `nonnegative`/`positive` constraints (line 229-235) | Go适配 |
| 4 | **Partial read size bypass** | If both `offset` and `limit` set, skip size check (line 164-169) | Same behavior — `limit === undefined ? maxSizeBytes : undefined` (line 1026) | ✅ Match |
| 5 | **Line numbering format** | `cat -n` style: `"N\tcontent"` (line 235-237) | `addLineNumbers()` — same format (line 724-727) | ✅ Match |
| 6 | **Pagination hint** | `"Showing lines X-Y of Z"` (line 243-246) | Similar via `formatFileLines` + size info | ✅ Match |
| 7 | **Empty file warning** | `<system-reminder>Warning: empty</system-reminder>` (line 217-219) | Same system-reminder format (line 704-706) | ✅ Match |
| 8 | **Offset-too-large warning** | `<system-reminder>Warning: shorter than offset</system-reminder>` (line 221-224) | Same format (line 704-707) | ✅ Match |
| 9 | **Dedup (file unchanged)** | Mtime-based dedup returning `FileUnchangedStub` (line 174-182) | Same concept — `FILE_UNCHANGED_STUB` + GrowthBook killswitch (line 536-573) | 简化 |
| 10 | **Binary extension rejection** | `isBinaryExtension()` — hardcoded map of ~60+ extensions (line 285-308) | `hasBinaryExtension()` — imported from constants, images/PDF excluded (line 471-482) | Go适配 |
| 11 | **Magic bytes detection** | `isBinaryMagic()` — 20+ signatures (PE, ELF, ZIP, PNG, etc.) (line 342-458) | Not in read tool — handled by `readFileInRange` utility | Go增强 |
| 12 | **Device file blocking** | `isDeviceFile()` — blocks /dev/zero, /dev/random, etc. (line 314-337) | Same — `isBlockedDevicePath()` (line 97-128) | ✅ Match |
| 13 | **UNC path blocking** | `isUncPath()` (line 74-78) | Same check in `validateInput` (line 463-467) | ✅ Match |
| 14 | **UTF-16 LE BOM support** | BOM detection + `utf16.Decode` (line 191-196) | Same — `readFileSyncWithMetadata` detects encoding | ✅ Match |
| 15 | **UTF-8 BOM stripping** | Strips `\xEF\xBB\xBF` (line 200) | Same | ✅ Match |
| 16 | **CRLF normalization** | `strings.ReplaceAll(content, "\r\n", "\n")` (line 198) | Same | ✅ Match |
| 17 | **Image reading** | Not present (rejected by binary extension check) | Full image read with resize, token-budget compression, base64 output (line 866-891) | 缺失 |
| 18 | **PDF reading** | Not present (rejected by binary extension check) | `extractPDFPages()`, `readPDF()`, page range support (line 893-1017) | 缺失 |
| 19 | **Notebook reading** | Not present (rejected by binary extension check) | `readNotebook()` with cell parsing, size check, jq fallback hints (line 822-863) | 缺失 |
| 20 | **PDF `pages` parameter** | Not present | `pages: "1-5"` with `parsePDFPageRange()` (line 237-243) | 缺失 |
| 21 | **Token count validation** | Not present | `validateContentTokens()` — rough estimate then API count (line 755-772) | 缺失 |
| 22 | **MacOS screenshot path** | Not present | `getAlternateScreenshotPath()` — thin space ↔ regular space (line 142-159) | 缺失 |
| 23 | **Similar file suggestion** | Not present | `findSimilarFile()` + `suggestPathUnderCwd()` on ENOENT (line 639-647) | 缺失 |
| 24 | **File read listeners** | Not present | `registerFileReadListener()` for external services (line 162-173) | 缺失 |
| 25 | **Cyber risk mitigation reminder** | Not present | `CYBER_RISK_MITIGATION_REMINDER` appended to content (line 729-738) | 缺失 |
| 26 | **Memory file freshness note** | Not present | `memoryFreshnessNote()` for auto-memory files (line 747-753) | 缺失 |
| 27 | **Session file analytics** | Not present | `detectSessionFileType()` + analytics event (line 1067-1083) | 缺失 |
| 28 | **Concurrent file retry** | 50ms sleep + retry on ENOENT for write-then-read race (line 84-89) | No equivalent — upstream handles via `readFileState` ordering | Go增强 |
| 29 | **isConcurrencySafe** | Not marked | `isConcurrencySafe() { return true }` (line 373-375) | 缺失 |
| 30 | **isReadOnly** | Not marked | `isReadOnly() { return true }` (line 376-378) | 缺失 |

---


---

### 54.9 Notebook Edit Tool

**Go**: Not implemented · **Upstream**: `NotebookEditTool/NotebookEditTool.ts` (~400 lines)

| # | Aspect | Go | Upstream (file:line) | Type |
|---|--------|----|----------------------|------|
| 1 | **Tool existence** | **Not present** | Full implementation (line 30-400+) | 缺失 |
| 2 | **Input schema** | N/A | `notebook_path`, `cell_id`, `new_source`, `cell_type`, `edit_mode` (line 30-57) | 缺失 |
| 3 | **edit_mode** | N/A | `replace`, `insert`, `delete` (line 50-56) | 缺失 |
| 4 | **cell_type** | N/A | `code`, `markdown` (line 43-49) | 缺失 |
| 5 | **Read-before-edit** | N/A | `readFileState.get()` + mtime check (line 221-237) | 缺失 |
| 6 | **File encoding** | N/A | `readFileSyncWithMetadata` preserves encoding + line endings (line 324-325) | 缺失 |
| 7 | **Cell ID lookup** | N/A | `parseCellId()` — supports both actual ID and `cell-N` index format (line 270-276) | 缺失 |
| 8 | **Notebook format handling** | N/A | nbformat 4+ with minor version check for cell IDs (line 381-390) | 缺失 |
| 9 | **.ipynb validation** | N/A | Rejects non-.ipynb files (line 189-196) | 缺失 |
| 10 | **Replace→insert promotion** | N/A | If replacing past end, auto-promotes to insert mode (line 371-377) | 缺失 |

> **Note**: Go's `file_edit.go` explicitly rejects `.ipynb` files with message "use the notebook tool instead" (line 127-129), acknowledging this tool is needed but not yet implemented.

---


---

### 54.10 Todo / TodoWrite Tool

**Go**: `tools/todo_write.go` (224 lines) · **Upstream**: `TodoWriteTool/TodoWriteTool.ts` (116 lines)

| # | Aspect | Go (file:line) | Upstream (file:line) | Type |
|---|--------|----------------|----------------------|------|
| 1 | **Tool name** | `"TodoWrite"` (line 115) | `TODO_WRITE_TOOL_NAME` | ✅ Match |
| 2 | **Input schema** | `todos` array with `content`, `status`, `activeForm` (line 158-188) | `todos` via `TodoListSchema()` — same structure (line 13-16) | ✅ Match |
| 3 | **Status values** | `pending`, `in_progress`, `completed` (line 11-15) | Same — enforced by `TodoListSchema` | ✅ Match |
| 4 | **activeForm field** | Optional `activeForm` (line 23) | Same in `TodoListSchema` | ✅ Match |
| 5 | **Description text** | Extensive multi-paragraph guide (line 117-156) | External `DESCRIPTION` + `PROMPT` from prompt.ts | Go适配 |
| 6 | **Output** | Plain text success message (line 216) | Structured: `{oldTodos, newTodos, verificationNudgeNeeded}` (line 22-26) | 简化 |
| 7 | **All-done auto-clear** | Not present — list persists | When all todos `completed`, clears list to `[]` (line 69-70) | 缺失 |
| 8 | **Verification nudge** | Not present | When 3+ tasks completed without verification step, appends nudge to use verification agent (line 76-86) | 缺失 |
| 9 | **Todo v2 / Task system** | Not present | `isTodoV2Enabled()` — can disable tool in favor of TaskCreate/TaskGet/TaskUpdate/TaskList (line 53-54) | 缺失 |
| 10 | **Session-based storage** | In-memory `TodoList` struct (line 33-39) | `appState.todos[todoKey]` with agent-scoped keys (line 67-68) | Go适配 |
| 11 | **Turn tracking / idle reminder** | `IncrementTurn()` + `BuildIdleReminder()` — reminds after 10 turns (line 62-79) | Not in tool itself — handled by attachment system | Go增强 |
| 12 | **Reminder formatting** | `BuildReminder()` — `○ ◐ ●` icons with status (line 82-106) | Handled by UI rendering layer | Go适配 |
| 13 | **Permission check** | `PermissionResultPassthrough()` (line 190) | `{behavior: 'allow'}` — no permission needed (line 58-61) | ✅ Match |
| 14 | **shouldDefer** | Not marked | `shouldDefer: true` (line 51) | 缺失 |
| 15 | **isEnabled** | Always enabled | `!isTodoV2Enabled()` — can be disabled by feature flag (line 53-54) | 缺失 |
| 16 | **Multi-agent todo scoping** | Not present — single global list | `agentId ?? getSessionId()` for scoping (line 67-68) | 缺失 |

---


---

