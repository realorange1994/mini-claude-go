# Other Tools

> file_history, work_task, git tools, todo

## Sections Included
- [##] Line 2617-2667 -- ## 20. File History Tools (`file_history_tools.go` + `filehistory.go`)
- [##] Line 2684-2704 -- ## 22. Work Task / Dependency Graph (`work_task.go`)
- [##] Line 4019-4063 -- ## 11. work_task.go — 工作依赖图（TODO 管理）
- [##] Line 7455-7525 -- ## Part 1: Git Tool Comparison

---

## Content

## 20. File History Tools (`file_history_tools.go` + `filehistory.go`)

**Upstream reference**: `src/utils/fileHistory.ts`

### 20.1 Architecture: backup vs snapshot
- **上游**: **Backup-based**: `fileHistoryTrackEdit()` creates hard-link copies of files at `~/.claude/file-history/{sessionId}/{hash}@v{N}`. Uses `copyFile()`/`link()` for file-level backups. State managed via React `FileHistoryState` with `snapshots: FileHistorySnapshot[]` and `trackedFiles: Set<string>`. MAX_SNAPSHOTS = 100 (fileHistory.ts:33-55, 748-798)
- **Go版**: **Snapshot-based**: `SnapshotHistory` stores full file content in `FileSnapshot` structs in-memory + persists to `.claude/snapshots/{timestamp}_{safeName}.json` files. Uses FNV-1a 128-bit checksums for dedup. `maxSnapshots = 50` per file (filehistory.go:29-42)
- **类型**: Go适配 — in-memory snapshots with JSON persistence vs upstream's file-copy backup approach. Go version uses less disk space (JSON vs full file copies) but more memory

### 20.2 Tool surface area
- **上游**: File history is integrated into the undo/revert UI component; no explicit "file_history" tools exposed to the LLM. Users interact via the TUI diff view and revert button
- **Go版**: 13 dedicated LLM-callable tools: `file_history`, `file_history_read`, `file_history_grep`, `file_restore`, `file_rewind`, `file_history_diff` (with chain diff), `file_history_summary`, `file_history_search`, `file_history_timeline`, `file_history_tag`, `file_history_annotate`, `file_history_checkout`, `file_history_batch` (file_history_tools.go:20-1129)
- **类型**: Go增强 — far more tool surface for LLM interaction; upstream uses TUI components only

### 20.3 Version specifier resolution
- **上游**: Versions are numeric (`@v1`, `@v2`) tied to backup files; snapshots identified by `messageId`
- **Go版**: `ResolveVersion()` supports flexible specifiers: "v3"/"3" (absolute), "current"/"latest" (last), "last2" (relative), tag names like `[release]` (filehistory.go:488-560)
- **类型**: Go增强 — richer version addressing scheme

### 20.4 Tagging and annotation
- **上游**: No tagging or annotation system for file versions
- **Go版**: `file_history_tag` tool supports add/list/delete/search actions on version tags. `file_history_annotate` adds user comments to specific versions (file_history_tools.go:757-921)
- **类型**: Go增强 — no upstream equivalent

### 20.5 Cross-file timeline
- **上游**: No cross-file timeline; snapshots are per-message bundles
- **Go版**: `file_history_timeline` tool provides chronological cross-file change timeline with duration filtering (file_history_tools.go:704-753)
- **类型**: Go增强 — cross-file view absent in upstream

### 20.6 Batch operations
- **上游**: No batch operations on file history
- **Go版**: `file_history_batch` tool supports glob-matched batch history/diff/restore/count operations (file_history_tools.go:968-1084)
- **类型**: Go增强 — no upstream equivalent

### 20.7 Diff generation
- **上游**: Uses `diffLines` from the `diff` npm package for diff stats computation
- **Go版**: Uses `go-difflib` for unified diff generation with stat/name-only/chain diff modes, plus +/- line counting (file_history_tools.go:497-570)
- **类型**: 等价 — different libraries but equivalent functionality; Go adds chain diff (3-way)

### 20.8 File history enablement
- **上游**: `fileHistoryEnabled()` checks global config + env vars `CLAUDE_CODE_DISABLE_FILE_CHECKPOINTING` / `CLAUDE_CODE_ENABLE_SDK_FILE_CHECKPOINTING` (fileHistory.ts:63-78)
- **Go版**: Always enabled; no enable/disable toggle
- **类型**: 缺失 — no file history disable mechanism

### 20.9 VSCode notification integration
- **上游**: `notifyVscodeSnapshotFilesUpdated()` sends `file_updated` notifications to VSCode MCP on snapshot changes (fileHistory.ts:1054-1098)
- **Go版**: No equivalent
- **类型**: 缺失 — no IDE integration

---


---

## 22. Work Task / Dependency Graph (`work_task.go`)

**Upstream reference**: `src/utils/todo/types.ts` + `src/components/agents/src/tools/TodoWriteTool/TodoWriteTool.ts`

### 22.1 Task model
- **上游**: `TodoItem` = `{ content, status: 'pending'|'in_progress'|'completed', activeForm }` — flat list with no dependencies (types.ts:1-18)
- **Go版**: `WorkTask` = `{ ID, Subject, Description, ActiveForm, Status, Owner, Metadata, Blocks[], BlockedBy[], CreatedAt, UpdatedAt }` — full dependency graph with bidirectional `Blocks`/`BlockedBy` edges (work_task.go:24-36)
- **类型**: Go增强 — dependency graph, ownership, metadata; upstream is a flat list

### 22.2 Dependency cycle detection
- **上游**: No cycle detection; flat list has no dependencies
- **Go版**: `wouldCreateCycle()` performs BFS on both `Blocks` and `BlockedBy` edges from the blocker to detect if adding a new edge would create a cycle (work_task.go:242-264)
- **类型**: Go增强 — prevents deadlock in dependency graph

### 22.3 Dependency validation
- **上游**: No dependency validation
- **Go版**: `filterValidDeps()` removes references to non-existent tasks from dependency lists (work_task.go:267-275)
- **类型**: Go增强 — prevents dangling references

---


---

## 11. work_task.go — 工作依赖图（TODO 管理）

### 11.1 数据模型

| Go | Upstream |
|---|---|
| `WorkTask`: `ID`, `Subject`, `Description`, `ActiveForm`, `Status`, `Owner`, `Metadata`, `Blocks`, `BlockedBy`（`work_task.go:24-36`） | 上游 `Task`: `id`, `subject`, `description`, `activeForm`, `owner`, `status`, `blocks`, `blockedBy`, `metadata`（`utils/tasks.ts:76-88`） |
| 3种状态：`pending`, `in_progress`, `completed`, `deleted`（`work_task.go:15-19`） | 上游 3种状态：`pending`, `in_progress`, `completed`（`utils/tasks.ts:69`），无 `deleted` 状态 |
| 状态用 Go `string` 枚举（`work_task.go:13`） | 上游用 Zod schema 验证（`utils/tasks.ts:71-74`） |

### 11.2 存储方式

| Go | Upstream |
|---|---|
| 纯内存 `map[string]*WorkTask`，无持久化（`work_task.go:40-44`） | **上游基于文件持久化**：每个任务存为 `{configDir}/tasks/{taskListId}/{id}.json`（`utils/tasks.ts:221-231`） |
| 任务 ID 用 `atomic.Int64` 自增（`work_task.go:43,58`） | 上游使用 high water mark 文件（`.highwatermark`）确保 ID 不重复，即使任务被删除/重置（`utils/tasks.ts:92-131`） |
| **Go 无 taskListId 概念** — 所有任务共享单一 store | 上游有 `getTaskListId()` 支持多会话/swarm 共享任务列表（`utils/tasks.ts:199-210`） |

### 11.3 依赖管理与循环检测

| Go | Upstream |
|---|---|
| `UpdateTask()` 支持 `addBlocks` / `addBlockedBy`，双向更新依赖（`work_task.go:157-202`） | 上游 `blockTask()` 分别更新双方任务（`utils/tasks.ts:458-486`） |
| `wouldCreateCycle()` BFS 遍历 **双向边**（Blocks + BlockedBy）检测循环（`work_task.go:242-264`） | **上游无显式循环检测** — 依赖通过简单追加，不检查循环 |
| `filterValidDeps()` 自动删除指向不存在任务的引用（`work_task.go:267-275`） | 上游 `deleteTask()` 删除任务时遍历清理所有引用（`utils/tasks.ts:393-441`） |

### 11.4 并发安全

| Go | Upstream |
|---|---|
| `sync.RWMutex` 保护并发读写（`work_task.go:41`） | **上游使用文件锁**（`proper-lockfile`），支持多进程并发安全（`utils/tasks.ts:102-108`） |
| 单进程内安全 | 支持多进程/多 Claude swarm 并发访问同一任务列表 |

### 11.5 缺失功能

| Go 缺失 | 上游有 |
|---|---|
| **无 `claimTask()`** — 无任务认领机制 | 上游 `claimTask()` 支持多代理竞争认领（`utils/tasks.ts:541-612`） |
| **无 `resetTaskList()`** — 无任务列表重置 | 上游 `resetTaskList()` 清空任务并更新 high water mark（`utils/tasks.ts:147-188`） |
| **无 `getAgentStatuses()`** — 无代理忙闲状态 | 上游基于任务所有权计算 `idle`/`busy` 状态（`utils/tasks.ts:763-798`） |
| **无 `unassignTeammateTasks()`** — 无代理退出任务回收 | 上游 `unassignTeammateTasks()` 在代理被杀/关闭时回收任务（`utils/tasks.ts:818-860`） |
| **无 `TaskSchema` 验证** | 上游使用 Zod schema 验证任务格式（`utils/tasks.ts:76-88`） |

---


---

## Part 1: Git Tool Comparison

### 1.1 Tool Interface -- Parameter Schema

| Aspect | Go (`git_tool.go`) | Upstream (TypeScript) |
|--------|---------------------|----------------------|
| **Tool existence** | Dedicated `GitTool` struct implementing `Tool` interface (`Name()`, `Description()`, `InputSchema()`, `Execute()`) | **No dedicated GitTool exists** -- git operations are performed via `BashTool` (`packages/builtin-tools/src/tools/BashTool/BashTool.tsx`) |
| **Schema type** | JSON Schema `map[string]interface{}` with 30+ typed properties | N/A -- `BashTool` takes `command: string`, `dangerouslyOverrideSandbox: bool` |
| **Operation enum** | `operation` field with 35+ enum values: clone, init, add, commit, push, pull, fetch, branch, checkout, merge, rebase, stash, reset, tag, status, diff, log, remote, show, describe, ls-files, ls-tree, rev-parse, rev-list, worktree, rm, mv, restore, switch, cherry-pick, revert, clean, blame, reflog, shortlog, gh, info | Commands must be expressed as raw shell commands; no structured enum |
| **GitHub CLI** | Built-in `gh` operation with read-only subcommand whitelist (pr view/list/diff/checks/status, issue view/list/status, run list/view, auth status, release list/view, search repos/issues/prs) -- `git_tool.go:1248-1375` | No built-in gh support; user runs `gh` via BashTool with its own safety system |
| **Input validation** | Rich per-operation validation in `buildGitCommand()` (`git_tool.go:806-1177`) -- validates required fields, constructs `git` CLI args from structured params | BashTool validates at command level (path sandboxing, command allowlists) but not git-specific semantics |

**File references:** Go: `E:\Git\miniClaudeCode-go-github\tools\git_tool.go:22-160` (InputSchema). Upstream: `E:\Git\claude-code-upstream\src\tools.ts:5` (BashTool import), `E:\Git\claude-code-upstream\packages\builtin-tools\src\tools\BashTool\BashTool.tsx` (command execution).

### 1.2 Git Operations Supported

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
| `info` (composite repo state) | Yes (special operation using utility functions) | **MISSING** -- no equivalent composite info operation |

**File references:** Go operations: `git_tool.go:810-1160` (buildGitCommand switch).

### 1.3 Safety Checks

| Aspect | Go | Upstream |
|--------|-----|----------|
| **Dangerous operation detection** | `isDangerousOperation()` (`git_tool.go:776-804`) blocks: `reset --hard`, `push --force`, `clean -f`, `branch -D` | BashTool has separate safety system: path sandboxing, command allowlists, denylists, permission modes (`acceptEdits`, `bypassPermissions`, `auto`, `plan`) |
| **Flag whitelist validation** | Per-subcommand safe flag maps (`git_tool.go:300-652`): `gitDiffFlags`, `gitLogFlags`, `gitShowFlags`, `gitStatusFlags`, `gitBranchFlags`, `gitResetFlags`, `gitMergeFlags`, `gitRebaseFlags`, `gitPushFlags`, `gitPullFlags`, `gitStashPushFlags` | No git-specific flag validation -- BashTool validates shell commands generically |
| **gh subcommand safety** | `GHSafeFlags` whitelist (`git_tool.go:1248-1375`), `validateGHFlags()` rejects non-whitelisted flags, dangerous repo values | N/A |
| **Path validation** | `directory` param used as workdir; no path traversal checks in the git tool itself | BashTool has comprehensive path sandboxing (`pathSandbox`, `isPathAllowed`) |
| **Remote check** | push/pull/fetch verify remote exists (`git_tool.go:232-250`) | N/A |
| **Proxy support** | `proxy` param sets `https_proxy`/`http_proxy` env vars | BashTool inherits proxy from environment |

**File references:** Go safety: `git_tool.go:166-178` (CheckPermissions), `git_tool.go:776-804` (isDangerousOperation), `git_tool.go:693-773` (validateGitFlags). Upstream safety: `E:\Git\claude-code-upstream\packages\builtin-tools\src\tools\BashTool\BashTool.tsx` (BashTool safety), `E:\Git\claude-code-upstream\src\utils\permissions\permissions.ts`.

### 1.4 Output Formatting

| Aspect | Go | Upstream |
|--------|-----|----------|
| **Format** | Raw `git` CLI stdout/stderr combined, trimmed (`git_tool.go:1200-1208`) | Raw command output from BashTool |
| **Error format** | `Error executing 'git <args>' (exit code: N)\n\nOutput:\n<output>` (`git_tool.go:274-277`) | BashTool has structured error output with exit codes |
| **Info operation** | Structured multi-line text: root, branch, default branch, commit, dirty status, bare repo, change count (`git_tool.go:1717-1771`) | N/A |
| **Git context for prompt** | `GetGitContext()` returns formatted string injected into system prompt (`git_tool.go:1776-1805`) | Upstream uses `gitStatus` in system context but with different format |

### 1.5 Integration with File History

| Aspect | Go | Upstream |
|--------|-----|----------|
| **File history tool** | No dedicated file history tool; `blame` operation provides per-line attribution | Upstream has no dedicated git blame tool either; `git blame` via BashTool |
| **blame operation** | Structured: `path` or `files` param required (`git_tool.go:1110-1118`) | Via BashTool |
| **Git utilities** | Exported helper functions: `FindGitRoot`, `GetBranch`, `IsBareRepo`, `IsGitRepo`, `GetGitStatus`, `HasUncommittedChanges`, `GetDefaultBranch`, `GetCurrentCommitHash`, `IsDirty`, `GetGitContext` (`git_tool.go:1560-1805`) | Similar utilities exist but as separate modules: `findGitRoot`, `findCanonicalGitRoot`, `getGitState`, `getFileStatus`, `preserveGitStateForIssue` (`E:\Git\claude-code-upstream\src\utils\git.ts`) |

### 1.6 Permission Checks

| Aspect | Go | Upstream |
|--------|-----|----------|
| **CheckPermissions** | Returns `PermissionResultPassthrough()` for all non-dangerous ops; `PermissionResultDeny(msg)` for dangerous ops (`git_tool.go:166-178`) | BashTool uses `checkPermissions()` with permission modes: `allow`, `deny`, `passthrough` |
| **Dangerous ops** | Only 4: reset --hard, push --force, clean -f, branch -D | BashTool uses broader safety model: path sandboxing, command allow/deny lists, auto-mode classifier |
| **User interaction** | No user prompt -- just returns deny message for dangerous ops | BashTool can prompt user for permission, use `auto` mode with YOLO classifier |

---


---

