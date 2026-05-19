# Exec/Bash Tool

> Shell execution, sandboxing, process management

## Sections Included
- [##] Line 1687-1764 -- ## 9. Bash/Exec Tool (tools/exec_tool.go vs upstream BashTool)
- [##] Line 7794-7999 -- ## Deep Comparison: Exec Tool Process Management vs Upstream BashTool
- [###] Line 11401-11441 -- ### 54.4 Exec / Bash Tool

---

## Content

## 9. Bash/Exec Tool (tools/exec_tool.go vs upstream BashTool)

### Files Compared
- **Go**: `E:\Git\miniClaudeCode-go-github\tools\exec_tool.go` (1622 lines)
- **Upstream**: `E:\Git\claude-code-upstream\packages\builtin-tools\src\tools\BashTool\` (multiple files: BashTool.ts, bashSecurity.ts, bashPermissions.ts, bashCommandHelpers.ts, pathValidation.ts, modeValidation.ts, shouldUseSandbox.ts, etc.)

### 9.1 Security Architecture
- **上游**: Multi-file defense-in-depth: `bashSecurity.ts` (24+ security check IDs, quote extraction, heredoc analysis, Tree-sitter AST), `bashPermissions.ts` (permission checking with classifier), `pathValidation.ts` (file operation path checks), `modeValidation.ts` (mode-specific validation), `shouldUseSandbox.ts` (sandbox detection). Uses Tree-sitter for AST parsing (`bashSecurity.ts` lines 77-101)
- **Go版**: Single-file consolidation (`exec_tool.go`). Uses regex-based deny patterns (~10+ patterns). Command substitution detection, path validation, UNC path detection. No AST parsing, no Tree-sitter
- **类型**: 简化

### 9.2 Command Parsing
- **上游**: `tryParseShellCommand()` with full shell quote parsing, `splitCommand_DEPRECATED` for subcommand splitting, `parseCommandRaw` for AST parsing. Handles heredocs, process substitution, Zsh equals expansion (`bashSecurity.ts` lines 12-75, `bashPermissions.ts` lines 12-94)
- **Go版**: Basic command string analysis. Regex-based pattern matching for dangerous commands. No shell AST parsing, no heredoc analysis
- **类型**: 简化

### 9.3 Security Check Coverage
- **上游**: 24+ numbered security checks: incomplete commands (1), jq system functions (2-3), obfuscated flags (4), shell metacharacters (5), dangerous variables (6), newlines (7), command substitution (8-10), IFS injection (11), git commit substitution (12), proc/environ access (13), malformed tokens (14), backslash-escaped whitespace (15), brace expansion (16), control characters (17), unicode whitespace (18), mid-word hash (19), Zsh dangerous commands (20-23) (`bashSecurity.ts` lines 77-101)
- **Go版**: Deny regex patterns cover: `rm -rf /`, `rm -rf ~`, `sudo rm`, `git push --force`, `git reset --hard`, `> /dev/sda`, `mkfs`, `dd if=`. Plus command substitution detection and path validation. ~10 deny patterns vs upstream's 24+ checks
- **类型**: 简化

### 9.4 Zsh-Specific Security
- **上游**: Comprehensive Zsh protection: `zmodload`, `emulate`, `sysopen/sysread/syswrite/sysseek`, `zpty`, `ztcp`, `zsocket`, `mapfile`, `zf_rm/zf_mv/zf_ln/zf_chmod/zf_chown/zf_mkdir/zf_rmdir/zf_chgrp`. Zsh equals expansion, glob qualifiers, always blocks, parameter expansion (`bashSecurity.ts` lines 43-74)
- **Go版**: No Zsh-specific protections. Only basic command patterns
- **类型**: 缺失

### 9.5 Shell Wrapper Stripping
- **上游**: `stripSafeWrappers` and `stripSafeHeredocSubstitutions` remove safe redirections before validation while tracking pre-strip content for validators. Handles `2>&1`, `>/dev/null`, `</dev/null` (`bashSecurity.ts` lines 176-188)
- **Go版**: `stripShellWrapper` function detects and removes common wrapper patterns (`exec_tool.go`)
- **类型**: Go适配

### 9.6 Quote Normalization
- **上游**: `extractQuotedContent` handles single quotes, double quotes, escape sequences. Three outputs: `withDoubleQuotes`, `fullyUnquoted`, `unquotedKeepQuoteChars`. Jq-specific mode includes quotes in extraction for analysis (`bashSecurity.ts` lines 128-174)
- **Go版**: `normalizeQuotes` function handles basic quote normalization in exec tool
- **类型**: 简化

### 9.7 Background Task Management
- **上游**: `ShellManager` with background task tracking, task IDs, streaming output. Background tasks accessible via `getBackgroundTask`, `listBackgroundTasks`, `waitForBackgroundTask`
- **Go版**: `backgroundTasks` map with `BackgroundTask` struct. Task ID, command, output, status (running/completed/failed). `list_bg`, `wait_bg`, `kill_bg` subcommands (`exec_tool.go`)
- **类型**: Go适配

### 9.8 Timeout Handling
- **上游**: Configurable timeout via `timeout` parameter. Uses `AbortController` for cancellation. Timeout behavior varies by mode
- **Go版**: `exec` tool has timeout parameter. Uses Go context with cancellation. Timeout returns error with partial output
- **类型**: Go适配

### 9.9 Path Validation for Bash
- **上游**: `checkPathConstraints` in `pathValidation.ts` validates file operation paths (mv, cp, rm, mkdir, touch, etc.). Extracts paths from command arguments using command-specific extractors. Handles `--` end-of-options delimiter to prevent bypass (`bashSecurity.ts` lines 126-139, `pathValidation.ts`)
- **Go版**: Path validation in exec tool checks for UNC paths, device paths, and path traversal. Less comprehensive — does not extract paths from individual commands
- **类型**: 简化

### 9.10 Sandbox Detection
- **上游**: `shouldUseSandbox` checks for sandbox configuration, feature flags, and policy settings. Sandbox mode changes permission checking behavior
- **Go版**: No sandbox mode support
- **类型**: 缺失

### 9.11 UNC Path Security
- **上游**: `containsVulnerableUncPath` in `readOnlyCommandValidation.ts`. Blocks `\\server\share` patterns to prevent NTLM credential leaks. Defense-in-depth check in multiple locations
- **Go版**: UNC path detection in exec tool: checks for `\\` and `//` prefixes. Blocks UNC paths in file operations
- **类型**: Go适配

### 9.12 Device File Blocking (Bash)
- **上游**: Blocks `/dev/zero`, `/dev/random`, `/dev/urandom`, `/dev/full`, `/dev/stdin`, `/dev/tty`, `/dev/console`, `/dev/stdout`, `/dev/stderr`, `/dev/fd/0-2`, `/proc/*/fd/0-2` in bash context
- **Go版**: Device file blocking in file_read.go: blocks `/dev/zero`, `/dev/random`, `/dev/urandom`, `/dev/full`, `/dev/stdin`, `/dev/tty`. Less comprehensive
- **类型**: 简化

### 9.13 Denial Tracking & Fallback
- **上游**: `createDenialTrackingState`, `recordDenial`, `recordSuccess`, `shouldFallbackToPrompting`. Tracks consecutive denials and total session denials. Falls back to interactive prompt after threshold (`permissions.ts` lines 94-101, `denialTracking.ts`)
- **Go版**: `denialCount` and `totalDenialCount` fields in PermissionGate. Falls back after 3 consecutive denials or 20 total denials (`permissions.go` lines 27-28, 463-486)
- **类型**: Go适配

### 9.14 Analytics & Telemetry
- **上游**: Extensive analytics: `logEvent('tengu_auto_mode_decision', ...)`, `sanitizeToolNameForAnalytics`, `classifierCostUSD`, `agentMsgId`, `inProtectedNamespace`, `fastPath`. Internal ant-only classifier result logging (`permissions.ts` lines 626-748)
- **Go版**: Basic `fmt.Fprintf(os.Stderr, ...)` for debugging. No analytics events
- **类型**: 缺失

---


---

## Deep Comparison: Exec Tool Process Management vs Upstream BashTool

**Go files:** `tools/exec_tool.go`, `tools/exec_tool_unix.go`, `tools/exec_tool_windows.go`
**Upstream files:** `packages/builtin-tools/src/tools/BashTool/BashTool.tsx`, `packages/builtin-tools/src/tools/BashTool/bashPermissions.ts`, `packages/builtin-tools/src/tools/BashTool/bashSecurity.ts`, `packages/builtin-tools/src/tools/BashTool/utils.ts`, `packages/builtin-tools/src/tools/BashTool/prompt.ts`

### 1. Shell selection — how each determines which shell to use

| Aspect | Go (`exec_tool.go`) | Upstream |
|--------|---------------------|----------|
| Unix | Always `bash -c` (`exec_tool.go:278`) | Uses user's login shell (bash/zsh), detected by `Shell.ts` |
| Windows | `powershell -Command` > `bash -c` > `cmd /C` (lookup order) (`exec_tool.go:268-276`) | Separate `PowerShellTool` for Windows; `BashTool` uses bash via WSL/Git Bash |
| Shell state | "The working directory persists between commands, but shell state does not." (`exec_tool.go:41`) | "The working directory persists between commands, but shell state does not. The shell environment is initialized from the user's profile (bash or zsh)." (`prompt.ts:357`) |
| CWD persistence | Sets `cmd.Dir` per command; no persistent shell session | Maintains CWD via `setCwd()` and injects `cd ${cwd} &&` prefix (`utils.ts:170-192`) |
| Profile sourcing | **No** — bare `bash -c` runs without profile | **Yes** — "initialized from the user's profile (bash or zsh)" (`prompt.ts:357`) |

**Key difference:** Upstream runs commands through the user's login shell with profile sourcing. Go always uses bare `bash -c` on Unix, which means no aliases, no shell functions, no PATH modifications from `.bashrc`/`.zshrc`. Upstream also has a separate PowerShellTool for Windows, while Go folds PowerShell into the same ExecTool.

### 2. Process group management — Go's platform-specific approach vs upstream

| Aspect | Go (`exec_tool_unix.go`, `exec_tool_windows.go`) | Upstream |
|--------|-----|----------|
| Unix | `Setpgid: true` in `syscall.SysProcAttr` (`exec_tool_unix.go:24-26`) | Node.js child_process; process group handling via `detached: true` equivalent in `src/utils/Shell.ts` |
| Windows | `CREATE_NEW_PROCESS_GROUP` in `syscall.SysProcAttr` (`exec_tool_windows.go:21-25`) | Windows process group via Node.js spawn options |
| Purpose | Prevents Ctrl+C forwarding to child; enables tree-kill (`exec_tool_unix.go:19-26`, `exec_tool_windows.go:17-25`) | Same purpose — isolate child process for clean termination |
| Architecture | Two build-tagged files (`//go:build unix` and `//go:build windows`) | Single cross-platform Shell.ts with platform detection |

**Go's approach is cleaner architecturally** — build tags ensure only the correct platform code compiles. Upstream uses runtime platform detection in a single file.

### 3. Signal handling — Go's SIGTERM/SIGINT vs upstream's approach

| Aspect | Go (`exec_tool.go`) | Upstream |
|--------|---------------------|----------|
| User interrupt (Ctrl+C) | `<-ctx.Done()` → `killProcessGroup()` + `cmd.Process.Kill()` (`exec_tool.go:434-441`) | `abortController.signal` aborts the shell command; `ShellCommand.abort()` kills process |
| Signal to child | `SIGKILL` to process group on Unix (`exec_tool_unix.go:32`) | Process kill via Node.js `childProcess.kill()` |
| Graceful shutdown | **No** — goes straight to SIGKILL | Shell command supports abort with potential graceful shutdown period |
| Exit code on signal | Extracts `128 + signal_number` on Unix (`exec_tool_unix.go:12-18`); returns -1 on Windows (`exec_tool_windows.go:13-15`) | Returns exit code from `result.code`; interrupted flag set |

**Go extracts signal exit codes properly on Unix** (128+signal). Upstream returns the raw exit code and sets an `interrupted` boolean separately.

### 4. Process tree kill — Go's taskkill/PID.Sendsignal vs upstream

| Aspect | Go | Upstream |
|--------|-----|----------|
| Unix tree-kill | `syscall.Kill(-pid, syscall.SIGKILL)` — kills entire process group (`exec_tool_unix.go:32`) | Shell command handles process cleanup; background tasks tracked via `LocalShellTask` |
| Windows tree-kill | `taskkill /F /T /PID <pid>` — kills process tree (`exec_tool_windows.go:32-36`) | Windows process tree kill via Node.js equivalent |
| Double-kill | Yes — `killProcessGroup()` then `cmd.Process.Kill()` (`exec_tool.go:437-438`) | Single kill via Shell command |
| Background task cleanup | `TimeoutCallback` handles draining output after process exits (`exec_tool.go:364-417`) | `LocalShellTask` manages background task lifecycle with proper cleanup (`BashTool.tsx:1184-1206`) |

**Go does a double-kill** (process group kill + direct process kill) for reliability. Upstream delegates to a more sophisticated `LocalShellTask` system that tracks tasks, manages cleanup callbacks, and supports Ctrl+B manual backgrounding.

### 5. Timeout management — Go's timer-based vs upstream's approach

| Aspect | Go (`exec_tool.go`) | Upstream (`BashTool.tsx`) |
|--------|---------------------|---------------------------|
| Default timeout | 120,000ms (2 min) (`exec_tool.go:251`) | `getDefaultBashTimeoutMs()` — 120,000ms default (`prompt.ts:27-29`) |
| Max timeout | 600,000ms (10 min) (`exec_tool.go:263`) | `getMaxBashTimeoutMs()` — 600,000ms default (`prompt.ts:31-33`) |
| Implementation | `time.NewTimer()` with `select` (`exec_tool.go:348-349`) | Shell command's built-in timeout via `exec()` options (`BashTool.tsx:1161-1178`) |
| On timeout behavior | Process continues in background (auto-backgrounding) (`exec_tool.go:352-433`) | Shell command's `onTimeout` callback triggers backgrounding (`BashTool.tsx:1265-1272`) |
| Context vs Timer | **Explicitly NOT using context.WithTimeout** — comment explains timeout should not kill process (`exec_tool.go:289-291`) | Uses `abortController.signal` for user interrupt; timeout is separate |

**Key difference:** Go explicitly avoids `context.WithTimeout` because on timeout it wants the process to continue running in the background. Context cancellation is reserved for user interrupts (Ctrl+C) which should kill the process. Upstream has the same separation — `abortController` for interrupts, `onTimeout` callback for backgrounding.

### 6. Working directory — how each sets CWD for commands

| Aspect | Go (`exec_tool.go`) | Upstream |
|--------|---------------------|----------|
| CWD source | `params["working_dir"]` or `os.Getwd()` (`exec_tool.go:282-287`) | `getCwd()` which returns tracked CWD state |
| CWD setting | `cmd.Dir = wd` — directly on exec.Cmd (`exec_tool.go:294`) | Injects `cd ${cwd} &&` prefix into shell command |
| CWD persistence | **No** — each command is independent; no state carried between calls | **Yes** — CWD persists between calls via `setCwd()` / `getCwd()` |
| CWD reset | N/A | `resetCwdIfOutsideProject()` resets to original if outside allowed paths (`utils.ts:170-192`) |
| Cd restrictions | Permission check blocks `cd` to dangerous paths (`exec_tool.go:149-154`) | Prevents CWD changes in sub-agents (`preventCwdChanges` flag) (`BashTool.tsx:878,1176`) |

**Go has no persistent CWD.** Each command starts in the specified working_dir (or current directory). Upstream maintains CWD state between commands and injects `cd` prefixes. Go's approach is simpler but requires the model to use absolute paths.

### 7. Environment variables — inheritance and injection

| Aspect | Go (`exec_tool.go`) | Upstream |
|--------|---------------------|----------|
| Inheritance | `exec.Command` inherits parent environment by default | Node.js child_process inherits parent environment |
| Injection | **No** — no env var injection | **Yes** — shell profile adds user's env vars |
| Safe env vars | `safeVarNames` map for command substitution detection (`exec_tool.go:574-612`) | `SAFE_ENV_VARS` set for wrapper stripping (`bashPermissions.ts:378-430`) |
| Env var filtering | **No** — all parent env vars inherited | Sandbox can restrict environment access |
| Stdin isolation | `cmd.Stdin = nil` — prevents interactive prompts (`exec_tool.go:295`) | Same — no stdin connected |

**Go does not inject or filter environment variables.** The `safeVarNames` map is only used for detecting dangerous command substitution patterns, not for actual env var handling. Upstream has sandbox-based env var restrictions and profile-sourced env vars.

### 8. Output streaming — real-time vs buffered

| Aspect | Go (`exec_tool.go`) | Upstream (`BashTool.tsx`) |
|--------|---------------------|---------------------------|
| stdout reading | `readLimited(stdout, 50000)` — reads up to 50KB in goroutine (`exec_tool.go:322-323`) | `EndTruncatingAccumulator` — accumulates output with tail preservation |
| stderr reading | `readLimited(stderr, 25000)` — reads up to 25KB in goroutine (`exec_tool.go:329-330`) | Merged into stdout (merged fd) (`BashTool.tsx:925`) |
| Real-time progress | **No** — output only available after process exits | **Yes** — async generator yields progress updates every ~1s (`BashTool.tsx:1356-1468`) |
| Progress display | **No** | Yes — `<BackgroundHint />` UI after 2s (`BashTool.tsx:1449-1457`) |
| Streaming to user | **No** — result returned only when process finishes | Yes — `onProgress` callback updates UI in real-time |

**Go is fully buffered.** Output is read into memory during execution but only returned after the process completes. Upstream has real-time streaming with progress updates via async generators, showing output as it arrives.

### 9. Output truncation — Go's head+tail vs upstream's approach

| Aspect | Go (`exec_tool.go`) | Upstream |
|--------|---------------------|----------|
| Max output | 30,000 bytes (`exec_tool.go:479`) | 30,000 chars default, configurable via `BASH_MAX_OUTPUT_LENGTH` env var, up to 150,000 (`outputLimits.ts:3-4`) |
| stdout cap | 50,000 bytes (`exec_tool.go:322`) | `getMaxOutputLength()` — configurable |
| stderr cap | 25,000 bytes (`exec_tool.go:329`) | Merged into stdout |
| Truncation format | `"[N lines truncated]"` appended to truncated part (`exec_tool.go:482`) | `"[N lines truncated]"` appended to truncated part (`utils.ts:158`) |
| Strategy | **Head-only** — truncation just cuts at 30K and counts remaining lines (`exec_tool.go:478-483`) | **Head + tail** — `EndTruncatingAccumulator` preserves both beginning and end of output |
| Large output persistence | **No** | **Yes** — output > threshold persisted to disk; model gets `<persisted-output>` message with preview (`BashTool.tsx:988-1012`) |
| Max persisted size | N/A | 64 MB (`BashTool.tsx:990`) |

**Go does head-only truncation** — it simply cuts at 30KB and appends a line count. Upstream uses `EndTruncatingAccumulator` which preserves both the beginning and end of output, and for very large outputs, persists to disk and sends a preview to the model. This is a significant difference — head-only truncation means the model may miss important error messages at the end of output.

### 10. Exit code handling — Go's signal exit code extraction vs upstream

| Aspect | Go (`exec_tool.go`) | Upstream |
|--------|---------------------|----------|
| Normal exit | `exitErr.ExitCode()` from Go's `exec.ExitError` (`exec_tool.go:467`) | `result.code` from Shell command |
| Signal exit (Unix) | `128 + signal_number` via `getSignalExitCode()` (`exec_tool_unix.go:12-18`) | `result.code` — exit code from process |
| Signal exit (Windows) | Returns -1 (`exec_tool_windows.go:13-15`) | Returns raw exit code |
| Error vs non-zero | `IsError: err != nil` — any non-zero exit is an error (`exec_tool.go:485`) | `interpretCommandResult()` applies semantic interpretation (`BashTool.tsx:928-933`) |
| Semantic interpretation | **No** — raw exit code + "Exit code: N" | **Yes** — `interpretCommandResult()` provides semantic meaning for special exit codes (`BashTool.tsx:928-933,1078`) |

**Go properly extracts signal exit codes on Unix** (128+signal), which upstream may not do. However, upstream has semantic exit code interpretation that Go lacks — for example, git's exit code 1 for "no changes" vs "error" gets different treatment.

### 11. Background execution — Go's auto-background on timeout vs upstream

| Aspect | Go (`exec_tool.go`) | Upstream (`BashTool.tsx`) |
|--------|---------------------|---------------------------|
| Explicit background | `run_in_background` param → `BackgroundTaskCallback` (`exec_tool.go:198-241`) | `run_in_background` param → `spawnShellTask()` (`BashTool.tsx:1299-1313`) |
| Auto-background on timeout | `TimeoutCallback` registers timed-out process as background task (`exec_tool.go:352-433`) | `shellCommand.onTimeout` callback → `startBackgrounding()` (`BashTool.tsx:1265-1272`) |
| Assistant auto-background | **No** | **Yes** — 15s budget for assistant mode (`BashTool.tsx:1277-1293`) |
| Manual background (Ctrl+B) | **No** | **Yes** — user can press Ctrl+B to background running command (`BashTool.tsx:1410-1423`) |
| Output file | Specified by `TimeoutCallback` return value; goroutine appends output after process exits (`exec_tool.go:364-417`) | `TaskOutput` writes to output file; `getTaskOutputPath()` for retrieval (`BashTool.tsx:1351`) |
| Foreground task tracking | **No** | `registerForeground()` / `unregisterForeground()` for Ctrl+B support (`BashTool.tsx:1389-1391,1438-1449`) |
| Sleep blocking | **No** | **Yes** — `detectBlockedSleepPattern()` blocks `sleep N` (N≥2) as first command (`BashTool.tsx:534-552`) |
| Disallowed auto-background | **No** | **Yes** — `sleep` cannot be auto-backgrounded (`BashTool.tsx:328-331`) |

**Upstream has much richer background execution support:** assistant-mode auto-backgrounding (15s budget), Ctrl+B manual backgrounding, foreground task registration, and sleep-pattern blocking. Go's background execution is simpler — only explicit `run_in_background` and timeout-based auto-backgrounding.

### 12. Interrupt handling — Ctrl+C propagation and cancellation

| Aspect | Go (`exec_tool.go`) | Upstream (`BashTool.tsx`) |
|--------|---------------------|---------------------------|
| Context cancellation | `<-ctx.Done()` channel in select (`exec_tool.go:434`) | `abortController.signal` abort |
| Kill on interrupt | `killProcessGroup()` + `cmd.Process.Kill()` (`exec_tool.go:437-438`) | `ShellCommand.abort()` kills process |
| Wait after kill | `<-errCh` — waits for `cmd.Wait()` to return (`exec_tool.go:440`) | Shell command handles wait internally |
| Result on interrupt | `"Interrupted by user"`, `IsError: true` (`exec_tool.go:441`) | `interrupted: true`, `<error>Command was aborted before completion</error>` (`BashTool.tsx:821-823`) |
| Pipe draining on timeout | **No** — pipes left open for background continuation (Windows-safe) (`exec_tool.go:358-363`) | Shell command manages pipe lifecycle |

**Go explicitly avoids closing pipes on timeout** to support auto-backgrounding on Windows. Upstream has a more sophisticated pipe management system integrated with the `LocalShellTask` infrastructure.

### 13. Safety checks — command filtering, dangerous pattern detection

| Aspect | Go (`exec_tool.go`) | Upstream |
|--------|---------------------|----------|
| Deny regex patterns | 20+ patterns: `rm -rf`, `del /f`, `format`, `mkfs`, `dd of=`, fork bombs, PowerShell destructive, Docker prune, Git destructive, chained background processes (`exec_tool.go:78-113`) | Tree-sitter AST parsing + semantic checks + deny/ask/allow rules + classifier (`bashPermissions.ts`) |
| Internal URL detection | Regex matching localhost, private IPs, RFC1918 ranges (`exec_tool.go:510-523`) | Part of path constraints and sandbox checks |
| UNC path blocking | Multiple regexes for backslash/forward-slash UNC, DavWWWRoot, IPv4 literal UNC (`exec_tool.go:527-567`) | Part of path constraints (`pathValidation.ts`) |
| Command substitution detection | `$()`, `${}` (with safe var allowlist), backticks, process substitution (`exec_tool.go:617-680`) | AST-based detection via tree-sitter; legacy regex fallback via `bashCommandIsSafeAsync` |
| Safe variable allowlist | `safeVarNames` map — 60+ entries (`exec_tool.go:574-612`) | `SAFE_ENV_VARS` set — 25 entries (`bashPermissions.ts:378-430`) + `ANT_ONLY_SAFE_ENV_VARS` |
| Glob/brace expansion detection | In destructive commands only: `rm`, `mv`, `cp`, `chmod`, `chown`, `git rm/clean/add` (`exec_tool.go:703-790`) | Part of AST semantic checks |
| Deletion path validation | Dangerous Unix paths, Windows paths, wildcard, glob, path traversal, critical project files (`exec_tool.go:900-1244`) | `checkPathConstraints()` with AST-derived redirects |
| Redirect target validation | Blocks shell expansion in targets, /dev/*, /proc/, /sys/, /etc/, ~/.ssh/, .claude/, .env (`exec_tool.go:954-1089`) | `checkPathConstraints()` validates redirect targets |
| Compound command splitting | `splitCompoundCommand()` respects quoting (`exec_tool.go:1247-1311`) | `splitCommand()` + AST-based splitting |
| Safe wrapper stripping | `stripSafeWrappers()` strips timeout, nice, nohup, time, stdbuf, ionice, env, sudo, doas (`exec_tool.go:1313-1421`) | `stripSafeWrappers()` strips timeout, time, nice, nohup, stdbuf with regex patterns (`bashPermissions.ts:524-615`) |
| Read-only command detection | `isReadOnlyCommand()` with extensive allowlist for git subcommands, npm, pip (`exec_tool.go:1437-1543`) | `checkReadOnlyConstraints()` with similar but more granular checks |
| Destructive command warnings | `isDestructiveCommand()` returns warning message (`exec_tool.go:1547-1621`) | `destructiveCommandWarning.ts` provides warnings for UI |
| Sandbox support | **No** | **Yes** — `SandboxManager` with filesystem/network restrictions (`BashTool.tsx:957-964,1083-1086`) |
| Classifier (ML-based) | **No** | **Yes** — `classifyBashCommand()` with allow/deny/ask descriptions, async auto-approval (`bashPermissions.ts:1463-1658`) |
| Tree-sitter AST parsing | **No** — regex-based only | **Yes** — `parseForSecurityFromAst()` provides structural command analysis (`bashPermissions.ts:1670-1807`) |
| Permission rule system | **No** — deny/ask binary based on pattern matching | **Yes** — full deny/ask/allow rule system with exact, prefix, and wildcard matching (`bashPermissions.ts:778-935`) |
| Cd + git compound block | **No** | **Yes** — blocks `cd X && git status` to prevent bare repo RCE (`bashPermissions.ts:2209-2225`) |
| Subcommand count cap | **No** | **Yes** — 50 subcommand cap to prevent DoS (`bashPermissions.ts:103`) |
| Heredoc handling | **No** | **Yes** — `stripSafeHeredocSubstitutions()` for safe heredoc patterns (`bashPermissions.ts:2096-2100`) |

**Go has extensive regex-based safety checks** that cover most of the same dangerous patterns as upstream. However, upstream has significantly more sophisticated safety infrastructure:
- **Tree-sitter AST parsing** for structural command analysis (vs Go's regex-only approach)
- **ML-based classifier** for permission auto-approval
- **Sandbox** with filesystem/network restrictions
- **Full permission rule system** with deny/ask/allow rules
- **Semantic command interpretation** for exit codes

Go's approach is more self-contained and portable (no external dependencies), while upstream's approach provides deeper analysis at the cost of complexity (tree-sitter WASM, classifier API calls, sandbox adapters).

### Additional Upstream Features Missing from Go

1. **Image output handling** — upstream detects base64 image data URIs in stdout, resizes them, and returns as image content blocks. Go treats all output as text. (`utils.ts:49-131`, `BashTool.tsx:790-797`)

2. **Sed edit simulation** — upstream intercepts `sed -i` commands and applies them via `applySedEdit()` instead of running sed, ensuring exact file writes. (`BashTool.tsx:579-638`)

3. **Claude Code hints** — upstream strips `<claude-code-hint />` tags from stdout before sending to model. (`BashTool.tsx:1043-1048`)

4. **Git index lock detection** — upstream logs events when `.git/index.lock` errors are detected. (`BashTool.tsx:936-941`)

5. **Code indexing tool detection** — upstream tracks usage of code indexing tools. (`BashTool.tsx:1026-1034`)

6. **Command type classification** — upstream classifies commands as search/read/list/silent for UI display purposes. (`BashTool.tsx:120-267`)

7. **Progress threshold** — upstream shows progress after 2 seconds. (`BashTool.tsx:115`)

8. **File history / undo** — upstream tracks file edits for undo support via `fileHistoryTrackEdit()`. Go has no file history.

9. **CRLF restoration** — Go has `RestoreCRLF()` for preserving Windows line endings on write; upstream has `writeTextContent()` with detected line endings.

---


---

### 54.4 Exec / Bash Tool

**Go**: `tools/exec_tool.go` (1622 lines) · **Upstream**: `BashTool/BashTool.tsx` + 14 helper files (~4000+ lines total)

| # | Aspect | Go (file:line) | Upstream (file:line) | Type |
|---|--------|----------------|----------------------|------|
| 1 | **Tool name** | `"exec"` (line 37) | `"Bash"` via `BASH_TOOL_NAME` | Go适配 |
| 2 | **Input schema** | `command`, `working_dir`, `description`, `timeout`, `run_in_background` (line 47-74) | `command`, `timeout`, `description`, `run_in_background`, `dangerouslyDisableSandbox`, `_simulatedSedEdit` (line 337-371) | Go适配 |
| 3 | **working_dir parameter** | Supported (line 56-58) | Not a parameter — CWD tracked via state, `resetCwdIfOutsideProject` | Go增强 |
| 4 | **Timeout default/max** | Default 120s, max 600s (line 251-264) | Dynamic via `getDefaultTimeoutMs()`/`getMaxTimeoutMs()` — may vary by mode | Go适配 |
| 5 | **run_in_background** | Supported via `BackgroundTaskCallback` (line 198-242) | Supported via `LocalShellTask` spawn system | ✅ Match |
| 6 | **Auto-background on timeout** | `TimeoutCallback` — process continues, registered as background task (line 349-426) | `auto-backgrounding on timeout` — same concept | ✅ Match |
| 7 | **User interrupt (Ctrl+C)** | Context cancellation kills process group (line 434-441) | Same — abort controller | ✅ Match |
| 8 | **Output truncation** | 30KB max + `[N lines truncated]` (line 477-483) | `EndTruncatingAccumulator` + `PREVIEW_SIZE_BYTES` persisted result | 简化 |
| 9 | **Shell selection** | PowerShell → bash → cmd on Windows; bash on Unix (line 267-279) | Detected via `Shell` utility | Go适配 |
| 10 | **Process group management** | `setupProcessGroup` / `killProcessGroup` (line 299, 437) | Same concept | ✅ Match |
| 11 | **Deny patterns** | Regex-based `denyRegexps` — 20+ patterns including fork bombs, PowerShell, Docker (line 78-113) | `bashSecurity.ts` + `parseForSecurity()` AST-based analysis | Go适配 |
| 12 | **Command substitution detection** | `detectCommandSubstitution()` — `$()`, `${}`, backtick, `<()`, `>()` (line 619-680) | AST-based `parseForSecurity()` in `bashSecurity.ts` | 简化 |
| 13 | **Safe variable allowlist** | `safeVarNames` — 60+ variables (line 574-612) | `bashPermissions.ts` — ENV_VAR_PATTERN with similar allowlist | ✅ Match |
| 14 | **Glob/brace expansion detection** | `detectExpansion()` — unquoted `*`, `?`, `[...]`, `{...}` in destructive cmds (line 709-790) | Not explicitly called out in BashTool — handled at permission rule level | Go增强 |
| 15 | **Path validation** | `validatePaths()` — blocks deletion of /etc, /usr, C:\, .git, etc. (line 1092-1144) | `pathValidation.ts` — similar, more granular | ✅ Match |
| 16 | **Redirect target validation** | `validateRedirectTargets()` — blocks /dev/*, /etc/, ~/.ssh, .env (line 954-1089) | Not a separate concern — handled in permission rules | Go增强 |
| 17 | **Safe wrapper stripping** | `stripSafeWrappers()` — timeout, nice, nohup, sudo, etc. (line 1336-1421) | `bashPermissions.ts` + `pathValidation.ts` — same concept | ✅ Match |
| 18 | **Compound command splitting** | `splitCompoundCommand()` — handles `;`, `&&`, `\|\|`, `\|`, newlines (line 1247-1311) | `splitCommandWithOperators()` — similar | ✅ Match |
| 19 | **Read-only command detection** | `isReadOnlyCommand()` — extensive map of read-only commands + git/npm subcommands (line 1437-1543) | `readOnlyValidation.ts` — more exhaustive with AST-based analysis | 简化 |
| 20 | **Destructive command warning** | `isDestructiveCommand()` — informational warning (line 1547-1621) | `destructiveCommandWarning.ts` — similar | ✅ Match |
| 21 | **UNC path blocking** | `containsVulnerableUncPath()` — backslash, forward-slash, IPv4, DavWWWRoot (line 536-567) | Same concept in `bashSecurity.ts` | ✅ Match |
| 22 | **Internal URL detection** | `containsInternalURL()` — localhost, private IPs (line 510-524) | Not in BashTool specifically — handled at network level | Go增强 |
| 23 | **Sandbox support** | Not present | `dangerouslyDisableSandbox`, `shouldUseSandbox()`, `SandboxManager` (line 360-362) | 缺失 |
| 24 | **Sed edit parsing** | Not present | `parseSedEditCommand()` + `_simulatedSedEdit` for preview (line 363-369) | 缺失 |
| 25 | **Sleep command blocking** | Not present | Blocks `sleep` > 2s, suggests `run_in_background` (line 742-748) | 缺失 |
| 26 | **Image output detection** | Not present | `isImageOutput()` + `resizeShellImageOutput()` (line 105-110) | 缺失 |
| 27 | **AST-based command parsing** | Not present — regex-based | `parseForSecurity()` from `bash/ast` module | 缺失 |
| 28 | **Foreground task management** | Not present | `registerForeground`/`unforeground` for tracking (line 32-33) | 缺失 |
| 29 | **Code indexing detection** | Not present | `detectCodeIndexingFromCommand()` (line 42) | 缺失 |
| 30 | **Claude Code hints** | Not present | `extractClaudeCodeHints()` from command comments (line 41) | 缺失 |
| 31 | **IsReadOnly tool method** | `IsReadOnly()` on `ExecTool` struct (line 1424-1427) | `isReadOnly()` check in tool def | ✅ Match |
| 32 | **stdin isolation** | `cmd.Stdin = nil` to prevent interactive prompts (line 295) | Not explicitly isolated | Go增强 |

---


---

