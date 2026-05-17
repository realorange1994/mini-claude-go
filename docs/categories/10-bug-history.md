# Bug History

## Timeout Auto-Background Testing

### Bug 1: timeout 参数被 agent_loop 删除，exec tool 收不到
- **位置**: agent_loop.go:2188 `delete(input, "timeout")`
- **原因**: agent_loop 把 timeout 当作 meta-parameter 删除了，但 exec tool 需要这个值
- **修复**: delete 后只对 exec/mcp 重新写入 `input["timeout"] = timeoutMs`（int 类型）

### Bug 2: timeout 单位不一致
- **位置**: agent_loop.go:2177 把 `input["timeout"].(float64)` 当秒用，但 exec schema 定义的是毫秒
- **原因**: 5000ms → agent_loop 当作 5000秒 → clamp 到 600秒
- **修复**: 所有超时统一到毫秒（ms）

### Bug 3: agent_loop 的 context timeout 和 exec/MCP 的 timer timeout 冲突
- **位置**: agent_loop.go:2263 `interruptCtx(context.Background(), timeout)`
- **原因**: agent_loop 用 context.WithTimeout 创建超时 context，5秒后 ctx.Done() 触发，杀掉进程。exec tool 的 timer-based 超时永远没机会触发
- **修复**: 对 exec 和 mcp_call_tool，agent_loop 使用 600000ms (10min) context deadline，让工具自己的 timer 先触发并自动后台

### Bug 4: agent_loop 使用秒而 schema 使用毫秒
- **原因**: toolTimeout 字段是 time.Duration（秒级别），但工具 schema 定义的是毫秒
- **修复**: 将 toolTimeout 改为 toolTimeoutMs (int，单位 ms)，所有超时统一到毫秒

### Bug 5: 关闭管道杀死 PowerShell 进程，导致 auto-background 失效
- **位置**: exec_tool.go 原来的 `stdout.Close(); stderr.Close()` (超时处理)
- **原因**: 在 Windows 上，关闭 stdout/stderr 管道会导致 PowerShell 进程收到 broken pipe 错误而立即退出，即使进程应该继续在后台运行
- **根因**: Go 的管道在进程退出前关闭会导致操作系统向进程发送 SIGPIPE（Unix）或终止进程（Windows）
- **修复**:
  1. 不再在超时时关闭管道，让进程继续运行
  2. 将 output drain 逻辑从超时 case 移到 `errCh` goroutine 中（等待 `cmd.Wait()` 返回后再 drain）
  3. `errCh` goroutine 在 `cmd.Wait()` 返回后 drain outputCh（此时 reader goroutines 才完成），然后写入输出文件和退出代码
  4. 修复 `onDone` 的 elapsed 计算错误（原来 `time.Since(start)` 在 `start` 之后立即执行，总是0）
- **验证**: ✅ 8行命令（8秒）超时设置为5秒，超时后继续运行3秒，所有输出正确捕获，Duration 准确 (3.457s)，无重复 footer/exit code

---

## Compaction Context Loss Bugs

### Bug 6: Stale session memory state persists across sessions
- **位置**: session_memory.go `NewSessionMemory()` → `loadFromDisk()` 加载所有条目包括 stale state
- **原因**: `state` 类别的条目是会话级别的临时上下文，不应跨会话保留。上一会话的 "Compaction: 4128 tokens" 等条目在新会话中变成无效的噪音
- **修复**: `NewSessionMemory()` 在 `loadFromDisk()` 后调用 `ClearStateEntries()` 清除 stale state
- **提交**: e6012b9

### Bug 7: toolStateTracker conclusions lost permanently after compaction
- **位置**: agent_loop.go `ClearConclusions()` 在 3 个地方被调用，但没有先保存结论
- **原因**: `ClearConclusions()` 直接清除所有结论，不保留到任何持久化存储。压缩后 agent 的工作知识（"已修复 bug X"、"已实现 feature Y"）永久丢失
- **修复**: 在每个 `ClearConclusions()` 调用前添加 `SaveConclusions()`，将结论保存到 session memory 的 `state` 类别
- **提交**: e6012b9

### Bug 8: SM-compact/LLM-compact use stale state & bloated summaries
- **位置**: agent_loop.go LLM-compact 路径 line 3646
- **原因**: `AddNote("state", fmt.Sprintf("Compaction: %s", summary), "auto")` 将整个 LLM 生成的摘要文本作为 state 条目保存，导致 session memory 膨胀并在未来会话中变成 stale 上下文
- **修复**:
  1. 在 SM-compact 和 LLM-compact 路径的 `OnCompaction()` 后添加 `ClearStateEntries()` 清除 stale state
  2. 将 LLM-compact 的 state 保存改为 `"Compaction (auto): %d tokens compressed"`（与 SM-compact 的 `"Compaction (sm-compact): %d tokens compressed"` 一致）
- **提交**: e6012b9

### Bug 9: buildCompactSummaryMessage called after CompactContext clears entries
- **位置**: agent_loop.go truncation fallback path (a.compactor == nil)
- **原因**: `buildCompactSummaryMessage()` 在 `CompactContext()` 之后调用，此时 `BuildMessages()` 返回空/截断的消息列表，导致摘要显示 "0 conversation turns with 0 tool calls"
- **修复**: 在 `CompactContext()` 之前捕获 `BuildMessages()` 和 `extractRecentToolCallsForSummary()`，将预捕获数据传递给 `buildCompactSummaryMessage()`
- **提交**: ea46040

### Bug 10: panic in KeepRecentMessagesAdaptive — incomparable type ToolUseContent
- **位置**: context.go:1021 `c.entries[i] == keptEntries[0]`
- **原因**: `conversationEntry.content` 是 `EntryContent` 接口，包装了 `ToolUseContent`（切片类型）。Go 不允许用 `==` 比较包含不可比较类型的接口值
- **修复**: 移除指针比较，改为在遍历时直接记录 `keptStartIdx`（最低索引），压缩后直接使用 `keptStartIdx` 设置 `lastSummarizedIndex`
- **状态**: 已修复

---

## Sub-Agent Task Management Bugs

### Bug 11: Parent agent's exec background tasks invisible to task_output/task_stop
- **位置**: agent_sub.go `createChildAgentLoop()` → `child.registerBashBgTool()`
- **原因**: `buildSubAgentRegistry` copies tool *pointers* from parent registry to child registry (no cloning). `child.registerBashBgTool()` then overwrites the **shared** `ExecTool.BackgroundTaskCallback` from `parent.spawnBackgroundBashCommand` to `child.spawnBackgroundBashCommand`. Subsequent `exec run_in_background=true` calls by the parent agent route through the shared ExecTool → `child.spawnBackgroundBashCommand` → registered in **child's** taskStore. The parent's `task_output` queries its own taskStore → "task not found"
- **根因**: Pointer-sharing pattern for tool instances between parent and child registries. Any mutable callback field (BackgroundTaskCallback, TimeoutCallback) on shared tools gets overwritten when child calls `registerBashBgTool()`
- **修复**: In `createChildAgentLoop()`, create **new** `ExecTool` and `MCPToolCaller` instances for the child agent instead of calling `registerBashBgTool()` on the shared instances. New instances point callbacks to `child.spawnBackgroundBashCommand` / `child.registerExistingProcessAsBgTask` / `child.registerMCPTimeoutAsBgTask`, leaving parent's shared instances untouched
- **影响文件**: `agent_sub.go` (createChildAgentLoop), `agent_loop.go` (registerBashBgTool — unchanged for parent path)
- **提交**: 2a8610e

### Bug 12: Killed sub-agent sends empty "completed" notification (0 tokens, empty result)
- **位置**: agent_sub.go `SpawnSubAgent()` child goroutine completion path
- **原因**: When a sub-agent is killed via `agent_kill`, the child goroutine completes with empty result. The completion path doesn't distinguish killed vs completed status → sends "completed" notification with no partial result, confusing the parent LLM
- **修复**:
  1. Add `TaskKilled` status check in child goroutine completion: if `bgTask.Status == tools.TaskKilled`, send "killed" notification with partial result instead of "completed"
  2. Update `InjectNotifications()` in agent_loop.go to detect killed tasks and use different system prefix: "some were killed" vs "completed"
- **影响文件**: `agent_sub.go` (SpawnSubAgent goroutine completion), `agent_loop.go` (InjectNotifications)
- **提交**: 2a8610e

---

## Microlisp Source Index & Search Bugs

### Bug 13: Source index missing ~255 helper functions and special form code
- **位置**: microlisp/source_index.go
- **原因**: Source index only covered 3 categories: builtins (scanned from `func builtinXXX`), stdlib (parsed `(define ...)`), special forms (names only, Start=0 End=0). All non-builtin Go functions (eqVal, evalQuasiquote, etc.) were invisible
- **修复**:
  1. `scanHelperFunctions()`: scan all `microlisp/*.go` for `func <name>(...)` patterns, skip builtins/tests, register as `Kind: "helper"`
  2. `scanSpecialForms()`: scan `eval_core.go` for `case "FORMNAME":` labels, bracket-match to find block end, populate Start/End with actual Go source lines
  3. `SourceList()` summary adds `Helpers: N` count
  4. `GetSource()` for helpers shows actual Go source code
- **影响文件**: `microlisp/source_index.go`, `microlisp/source_index_test.go`
- **提交**: d9d13ef

### Bug 14: scanSpecialForms fails on multi-name case labels
- **位置**: microlisp/source_index.go `scanSpecialForms()`
- **原因**: Some case labels combine multiple forms: `case "MAPCAR", "MAPCAN":` — the scanner only matched single-name patterns
- **修复**: Parse comma-separated names in case labels, register all names with same Start/End
- **提交**: 3a6baae

### Bug 15: readStdlibSnippet fails on non-standard define patterns
- **位置**: microlisp/source_index.go `readStdlibSnippet()`
- **原因**: Some stdlib defines use non-standard patterns (e.g., multi-line, inline bodies)
- **修复**: Handle all stdlib define patterns including multi-line and inline bodies
- **提交**: 3a6baae

### Bug 16: Source queries load entire source into context (token explosion)
- **位置**: microlisp/source_index.go `GetSource()` — returned all source without pagination
- **原因**: `SourceList("")` returns all ~1000+ functions at once, consuming entire context window
- **修复**:
  1. Add `offset` and `limit` parameters to `GetSource()` and `SourceList()`
  2. Default limit of 50 entries, pagination via offset
  3. Source code snippets truncated with offset/limit for display
  4. lisp_eval tool passes srcOffset/srcLimit from params
- **影响文件**: `microlisp/source_index.go`, `tools/lisp_eval.go`
- **提交**: 882a3f2, 03c04ac

### Bug 17: lisp_eval.go line 168 corrupted (all code collapsed into single line)
- **位置**: tools/lisp_eval.go:168
- **原因**: Previous sed command collapsed offset/limit extraction code into one line with mixed tabs and literal 't' character
- **修复**: Python script to replace the corrupted line with properly formatted multi-line extraction code
- **提交**: fb0e7b1 (included in broader source fix)

---

## Search & Output Limit Bugs

### Bug 18: Search tools (grep/glob/go-search) can output unlimited data causing context explosion
- **位置**: tools/grep_tool.go, tools/glob_tool.go, go-search in lisp
- **原因**: No output size cap on search results; a large grep result can fill the entire context window
- **修复**:
  1. Add `truncateOutput` utility with configurable max chars
  2. Apply to all search tool results paths (grep, glob, go-search)
  3. Add output size circuit breaker (606d618) — when output exceeds threshold, truncate with summary
  4. Add truncateOutput to streaming tool results path (74ddcee)
  5. Tighten default output limits (965fa1f)
- **提交**: 74ddcee, 965fa1f, 606d618

---

## Go-Search Behavior Bugs

### Bug 19: go-search recursive behavior differs from original Lisp implementation
- **位置**: microlisp/streams.go (go-search)
- **原因**: Original Lisp go-search was non-recursive (single directory only), but the reimplementation was recursive
- **修复**: Make go-search non-recursive to match original Lisp directory behavior
- **提交**: 65a1bb1

### Bug 20: go-search replaced with rgrep engine
- **位置**: tools/grep_tool.go
- **原因**: Custom Go search implementation had maintenance burden and feature gaps vs ripgrep
- **修复**: Replace custom go-search with rgrep engine (native ripgrep wrapper with .gitignore support, better performance)
- **提交**: 0e163f0

---

## Microlisp String/Unicode Bugs

### Bug 21: string-length and string-find use byte offsets instead of rune offsets
- **位置**: microlisp/strings.go
- **原因**: `string-length` counted bytes instead of Unicode code points; `string-find` returned byte positions. This broke multi-byte character strings (e.g., Chinese, emoji)
- **修复**: Use `utf8.RuneCountInString()` for length and `[]rune` indexing for positions
- **提交**: b59dad5

---

## Microlisp Language Compliance Bugs

### Bug 22: Missing 4-deep car/cdr composition functions (caaaar through cddddr)
- **位置**: microlisp/stdlib.go
- **原因**: stdlib only defined 2-deep (caar, cadr, cdar, cddr) and 3-deep (caaar..cdddr) car/cdr combinations. All 16 four-deep combinations were missing: caaaar, caaadr, caadar, caaddr, cadaar, cadadr, caddar, cadddr, cdaaar, cdaadr, cdadar, cdaddr, cddaar, cddadr, cdddar, cddddr
- **修复**: Add 16 `(define ...)` forms for all 4-deep car/cdr combinations after line 87
- **提交**: TBD

### Bug 23: defvar/defparameter return incorrect values
- **位置**: microlisp/eval_core.go `case "DEFVAR"`, `case "DEFPARAMETER"`
- **原因**: Both defvar and defparameter returned `vsym(name)` (the symbol name) instead of the evaluated value. CL spec requires defparameter to return the value of the initform, and defvar should return the value when an initform is provided
- **修复**:
  1. defvar: return the evaluated value when initform is provided and symbol is not already bound
  2. defparameter: return the evaluated value instead of vsym(name)
- **提交**: TBD

### Bug 24: parse-integer error messages inconsistent
- **位置**: microlisp/strings.go `builtinParseInteger()`
- **原因**: Two error paths returned different messages: `"parse-integer: no integer at position %d"` (empty/sign-only string) vs `"parse-integer: not an integer"` (parse failure). Both should show what failed
- **修复**: Unify both error paths to `"parse-integer: junk in string \"xxx\""` which includes the original substring that failed to parse, providing clearer debugging context
- **提交**: TBD

---

## Code Organization

### Refactoring: 90+ misplaced functions reorganized into single-responsibility files
- **位置**: microlisp/*.go (entire package)
- **原因**: Functions were scattered across files without regard for single responsibility principle
- **修复**:
  1. Phase 1: Create equality.go — move equality/comparison functions
  2. Phase 2: Clean type_system.go
  3. Phase 3: Clean concurrency.go + io.go
  4. Phase 4: Clean data_structures.go
  5. Phase 5: Clean list_ops.go
  6. Phase 6: Delete sequences.go stub
  7. Extract arithmetic functions from numbers.go into separate files
- **提交**: 012b581, 46cc80c, 012b581

### i18n: Chinese comments translated to English
- **范围**: All microlisp/*.go files
- **提交**: 2f0e06c
