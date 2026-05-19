# Hooks

> Hook system, lifecycle hooks

## Sections Included
- [##] Line 4712-4864 -- ## 13 Hooks System: Go vs Upstream
- [##] Line 10030-10118 -- ## 44. Hooks, MCP Client, Permissions, File Watcher, Cost Tracking

---

## Content

## 13 Hooks System: Go vs Upstream

### 13.1 Hook Types Supported

- **上游**: `src/utils/hooks.ts:1` (~5200 行)
- **Go版**: `hooks.go:1` (182 行)

| Hook 类型 | 上游 | Go版 |
|-----------|------|------|
| PreToolUse | 支持 (AsyncGenerator, 可阻塞工具执行) | 不支持 |
| PostToolUse | 支持 (AsyncGenerator, 可阻塞) | 不支持 |
| PostToolUseFailure | 支持 | 不支持 |
| PermissionDenied | 支持 | 不支持 |
| PermissionRequest | 支持 (可编程批准/拒绝) | 不支持 |
| SessionStart | 支持 (可注入 initialUserMessage, watchPaths) | 不支持 |
| Setup | 支持 | 不支持 |
| Stop | 支持 (含 subagent 上下文) | 不支持 |
| StopFailure | 支持 | 不支持 |
| SubagentStart/Stop | 支持 | 不支持 |
| SessionEnd | 支持 (并行执行) | 不支持 |
| PreCompact | 支持 | **支持** |
| PostCompact | 支持 | **支持** |
| Notification | 支持 | 不支持 |
| TeammateIdle | 支持 (swarm) | 不支持 |
| TaskCreated/Completed | 支持 (swarm) | 不支持 |
| UserPromptSubmit | 支持 (可修改/阻止 prompt) | 不支持 |
| ConfigChange | 支持 | 不支持 |
| CwdChanged | 支持 | 不支持 |
| FileChanged | 支持 (watchPaths 驱动) | 不支持 |
| InstructionsLoaded | 支持 | 不支持 |
| Elicitation/ElicitationResult | 支持 (MCP 权限请求) | 不支持 |
| WorktreeCreate/Remove | 支持 | 不支持 |

**结论**: Go 版仅支持 PreCompact 和 PostCompact 两个 hook 类型。上游支持 20+ 种 hook 事件。

### 13.2 Hook Execution Model

**上游执行模型**:

1. **外部命令 (Shell)**: 通过 `child_process.spawn()` 执行 shell 命令
   - 支持 bash/powershell 两种 shell
   - 可配置超时 (TOOL_HOOK_EXECUTION_TIMEOUT_MS = 10 分钟)
   - SessionEnd hooks 更紧的超时 (1500ms)
   - 支持 stdin/stdout/stderr 流式处理
   - 异步 hook 支持 (JSON `{async:true}` 响应 + rewake)

2. **回调函数 (SDK)**: `hook.callback()` 直接调用 JS 函数
   - 用于插件和 SDK 注册
   - 通过 `executeFunctionHook()` / `executeHookCallback()` 执行

3. **Prompt 交互**: hooks 可以通过 stdout 输出 `{prompt:...}` JSON 请求用户输入

4. **HTTP 调用**: `execHttpHook()` 支持 HTTP 端点调用

**Go 版执行模型**:

Go 版使用 **函数回调 (Go callbacks)**:
- `PreCompactHandler` 和 `PostCompactHandler` 是 Go 函数签名
- 通过 `context.WithTimeout()` 实现超时
- 同步顺序执行所有注册的 hooks
- 不支持外部 shell 命令
- 不支持 prompt 交互
- 不支持异步 rewake

### 13.3 Hook Input/Output Formats

**PreCompact**:

| 字段 | 上游 (PreCompactHookInput) | Go版 (PreCompactInput) |
|------|--------------------------|----------------------|
| hook_event_name | `"PreCompact"` | 无 |
| trigger | `"manual"` / `"auto"` | `HookTrigger` 枚举 (`manual`/`auto`/`sm_compact`) |
| custom_instructions | `string \| null` | `string` (已有自定义指令) |

| 输出 | 上游 | Go版 |
|------|------|------|
| customInstructions | 成功 hook 的 stdout 合并 (join `\n\n`) | `CustomInstructions` 拼接 (用 `\n\n`) |
| userDisplayMessage | 每个 hook 的结果 (成功/失败都显示) | `UserMessage` 拼接，格式: `PreCompact [hook:NAME] completed/failed: MSG` |

**PostCompact**:

| 字段 | 上游 (PostCompactHookInput) | Go版 (PostCompactInput) |
|------|---------------------------|----------------------|
| trigger | `"manual"` / `"auto"` | `HookTrigger` 枚举 |
| compact_summary | `string` | `CompactSummary` |
| (无) | 无 | `RecoveredFiles []string` (Go 独有) |

| 输出 | 上游 | Go版 |
|------|------|------|
| userDisplayMessage | 每个 hook 的结果 | `UserMessage` 拼接 |
| (无) | 不支持 attachment | `Attachment` 内容拼接 (Go 独有) |

**上游独有输出**:
- PreToolUse: `permissionDecision` (approve/block), `updatedInput`, `additionalContext`
- UserPromptSubmit: `additionalContext`, 可修改 prompt
- SessionStart: `initialUserMessage`, `watchPaths`

### 13.4 Matcher System

**上游匹配器**:
- 配置文件: `hooks` 对象，按事件名分组
- 格式: `{ matcher: "Bash", hooks: [{ command: "echo hi" }] }`
- 匹配器支持 glob 模式: `matchesPattern()` 函数
- 匹配查询: `matchQuery` 根据事件类型提取 (tool_name, source, trigger, reason 等)
- 多源合并: snapshot (settings.json) + registered (SDK) + session (agent frontmatter) + plugin + skill
- 管理权限: `shouldAllowManagedHooksOnly()` 限制只运行 managed hooks
- 条件匹配: `if` 字段支持复杂条件 (tool input schema 验证)

**Go版匹配器**: 不支持。所有 hooks 直接注册到 HookManager，无过滤/匹配机制。

### 13.5 Error Handling

| 维度 | 上游 | Go版 |
|------|------|------|
| 单个 hook 失败 | 不影响其他 hook，记录到 result.output | 记录到 `firstErr`，但继续执行后续 hooks |
| 总体错误 | 返回 results 数组，每个有 `succeeded` 标志 | 返回 `firstErr` (第一个错误) + 合并的 `UserMessage` |
| 超时 | `AbortSignal.timeout()` 终止子进程 | `context.WithTimeout()` 取消 context |
| 阻塞行为 | PreToolUse/Stop 等可阻塞主流程 (blockingError) | 不阻塞，仅返回错误信息 |
| 进度报告 | `startHookProgressInterval()` 定期上报进度 | 不支持 |

### 13.6 Hook Configuration Sources

**上游配置源**:
1. **settings.json**: `hooks` 字段 (userSettings/projectSettings/localSettings)
2. **SDK 注册**: `registerHook()` API 调用
3. **Agent frontmatter**: agent 定义中的 hooks
4. **Plugin hooks**: 插件提供的 hooks
5. **Skill hooks**: 技能提供的 hooks
6. **Session hooks**: 运行时动态注册

**Go版配置源**: 纯代码注册。通过 `RegisterPreCompact()` / `RegisterPostCompact()` 在代码中直接注册，无配置文件支持。

### 13.7 Execution Order

**上游**:
1. 收集所有匹配 hooks (snapshot + registered + session + plugin + skill)
2. 按来源过滤 (managed-only 模式)
3. 按 matchQuery 过滤
4. **并行执行**所有 hooks (默认)
5. 聚合结果

**Go版**:
1. 遍历已注册 hooks 列表
2. **顺序执行**每个 hook
3. 聚合 CustomInstructions/Attachments (拼接)
4. 聚合 UserMessage (拼接)
5. 聚合错误 (取第一个)

**结论**: Go 版 hooks 系统是上游的极小功能子集——仅支持 PreCompact/PostCompact 事件，使用 Go 函数回调而非外部命令，顺序执行而非并行，无匹配器系统，无配置源。设计思路类似但实现规模差距显著: 上游约 5200 行 vs Go 版 182 行。
上游 BriefTool 是完整的通信工具：支持 message/attachments/status(normal|proactive)、file 上传、feature gating（KAIROS/KAIROS_BRIEF）、analytics 日志、React UI 渲染。Go 版极简：返回一组硬编码的通信原则（concise/direct/skip filler），仅接受 task 参数定制上下文。上游是一个用户消息投递机制，Go 版更像系统提示词注入。

---


---

## 44. Hooks, MCP Client, Permissions, File Watcher, Cost Tracking

### Files Compared
- **Go**: `hooks.go`, `mcp/client.go`, `permissions.go`
- **Upstream**: `hooks.ts`, `services/mcp/`, `utils/permissions/`, `services/compact/gitFilesystem.ts`, `cost-tracker.ts`

### 44.1 Hooks System

| # | Aspect | Go (`hooks.go`) | Upstream (`hooks.ts`) | Type |
|---|--------|----------------|----------------------|------|
| 1 | Hook event types | Only `PreCompact`/`PostCompact` (line 12-15) | 25+ events: PreToolUse, PostToolUse, PostToolUseFailure, SessionStart, SessionEnd, Stop, SubagentStart, SubagentStop, UserPromptSubmit, PermissionDenied, PermissionRequest, ConfigChange, FileChanged, CwdChanged, Setup, StopFailure, TeammateIdle, TaskCreated, TaskCompleted, InstructionsLoaded, Elicitation, ElicitationResult, StatusLine, FileSuggestion, WorktreeCreate, WorktreeRemove (line 77-108) | 缺失 |
| 2 | Hook execution model | Registered Go function callbacks with timeout context (line 61-62) | Shell-spawned child processes via `spawn()`, with stdin/stdout capture, exit code handling, and environment injection (line 7, 302-900) | 简化 |
| 3 | Hook configuration via settings.json | Not implemented | `getMatchingHooks()` loads hook matchers from settings, skills, plugins; supports `when` conditions, `matcher` patterns (line 1741-1810) | 缺失 |
| 4 | Plugin/Skill hooks | Not implemented | `loadPluginHooks()` loads hooks from official marketplace plugins with source validation (line 125-128) | 缺失 |
| 5 | HTTP hooks | Not implemented | `execHttpHook()` for HTTP endpoint-based hooks (line 152) | 缺失 |
| 6 | Workspace trust requirement | Not implemented | `shouldSkipHookDueToTrust()` — ALL hooks require trust dialog acceptance (line 287-297) | 缺失 |
| 7 | Async hook registry | Not implemented | `registerPendingAsyncHook()` — hooks can run in background and return results later (line 134, 248-266) | 缺失 |
| 8 | PreToolUse hooks | Not implemented | `executePreToolHooks()` — can block tool execution, return `updatedInput`, `permissionRequestResult`, or `blockingError` (line 3536-3590) | 缺失 |
| 9 | PostToolUse hooks | Not implemented | `executePostToolHooks()` — runs after tool execution, can inject `updatedMCPToolOutput` (line 3592-3632) | 缺失 |
| 10 | PostToolUseFailure hooks | Not implemented | `executePostToolUseFailureHooks()` — runs when tool execution fails (line 3634-3669) | 缺失 |
| 11 | PermissionRequest hooks | Not implemented | `executePermissionRequestHooks()` — async generator for headless agents (line 4308-4343) | 缺失 |
| 12 | Session environment injection | Not implemented | `getHookEnvFilePath()`, `invalidateSessionEnvCache()` — hooks get session-scoped env vars (line 15-17) | 缺失 |
| 13 | OTel tracing for hooks | Not implemented | `startHookSpan()` / `endHookSpan()` for distributed tracing (line 61-64) | 缺失 |

### 44.2 MCP Client

| # | Aspect | Go (`mcp/client.go`) | Upstream (`services/mcp/`) | Type |
|---|--------|---------------------|--------------------------|------|
| 1 | Transport types | Only stdio + basic HTTP/SSE (line 72-74) | 7 types: stdio, SSE, HTTP, WebSocket, SDK, SSE-IDE, claudeai-proxy (`types.ts:24-135`) | 简化 |
| 2 | OAuth authentication | Not implemented | Full OAuth flow with `createClaudeAiProxyFetch()`, token refresh on 401, 15-min needs-auth cache (`client.ts:340-416`) | 缺失 |
| 3 | StreamableHTTP transport | Not implemented | `StreamableHTTPClientTransport` from MCP SDK (`client.ts:14-16`) | 缺失 |
| 4 | WebSocket transport | Not implemented | `WebSocketTransport` for IDE WebSocket connections (`client.ts:88`) | 缺失 |
| 5 | MCP Skills | Not implemented | `fetchMcpSkillsForClient()` — feature-gated skill loading for MCP servers (`client.ts:129-133`) | 缺失 |
| 6 | Image handling in MCP tool results | Not implemented | `maybeResizeAndDownsampleImageBuffer()` for base64 image content (`client.ts:74`) | 缺失 |
| 7 | Binary blob persistence | Not implemented | `persistBinaryContent()`, `getBinaryBlobSavedMessage()` — large binary results saved to disk (`client.ts:78-81`) | 缺失 |
| 8 | MCP tool output truncation | Not implemented | `mcpContentNeedsTruncation()`, `truncateMcpContentIfNeeded()` with size estimation (`client.ts:83-87`) | 缺失 |
| 9 | Tool result storage fallback | Not implemented | `isPersistError()`, `persistToolResult()` — handles oversized tool results by persisting to disk (`client.ts:99-101`) | 缺失 |
| 10 | MCP name normalization | Not implemented | `normalizeNameForMCP()`, `buildMcpToolName()` — sanitizes tool names for model consumption (`client.ts:112`) | 缺失 |
| 11 | Elicitation handling | Not implemented | `runElicitationHooks()`, `runElicitationResultHooks()`, `ElicitRequestSchema` — MCP elicitation protocol (`client.ts:107-110`) | 缺失 |
| 12 | Connection monitoring | Not implemented | `installConnectionMonitor()`, `isMcpSessionExpiredError()` — auto-detect session expiry, reconnect (`client.ts:119-121, 206`) | 缺失 |
| 13 | Config scope management | Not implemented | `ConfigScope` enum: local, user, project, dynamic, enterprise, claudeai, managed; merge from multiple sources (`types.ts:10-20`) | 缺失 |
| 14 | Env var expansion in MCP config | Not implemented | `expandEnvVarsInString()` — `${VAR}` expansion in MCP server command/args/env (`config.ts:44`) | 缺失 |
| 15 | Enterprise/managed MCP config | Not implemented | `getEnterpriseMcpFilePath()` — managed-mcp.json for enterprise policy enforcement (`config.ts:62-64`) | 缺失 |
| 16 | Plugin-provided MCP servers | Not implemented | `getPluginMcpServers()` — MCP servers provided by installed plugins (`config.ts:22`) | 缺失 |

### 44.3 Permissions System

| # | Aspect | Go (`permissions.go`) | Upstream (`permissions.ts`, `PermissionContext.ts`) | Type |
|---|--------|----------------------|---------------------------------------------------|------|
| 1 | Permission rule sources | 4 sources: localSettings, userSettings, projectSettings, cliArg (`rules_loader.go:87-100`) | 6+ sources: localSettings, userSettings, projectSettings, cliArg, session, enterprise, managed, policySettings, flagSettings (`permissions.ts:109-114`) | 简化 |
| 2 | Permission decision reasons | Simple `DecisionReason` string | Rich discriminated union: rule/hook/classifier/mode/safetyCheck/workingDir/subcommandResults/permissionPromptTool/sandboxOverride/asyncAgent/other (`PermissionResult.ts`) | 简化 |
| 3 | Permission approval/rejection source tracking | Not implemented | `PermissionApprovalSource` / `PermissionRejectionSource` types with type: hook/user/classifier/user_abort/user_reject (`PermissionContext.ts:45-53`) | 缺失 |
| 4 | Permission decision logging | Not implemented | `logPermissionDecision()` — centralized analytics/OTel logging for every allow/deny (`permissionLogging.ts:181-235`) | 缺失 |
| 5 | Permission update persistence | Not implemented | `persistPermissionUpdates()`, `applyPermissionUpdates()` — write rules to settings.json files (`PermissionUpdate.ts`) | 缺失 |
| 6 | Interactive permission queue | Simple y/N console prompt (line 498-523) | `pushToQueue(toolUseConfirm)` with full UI: ToolUseConfirm component, recheckPermission callback, onAbort/onAllow/onReject, pipe permission relay, bridge callbacks (`interactiveHandler.ts:134-705`) | 简化 |
| 7 | Bridge permission (claude.ai) | Not implemented | `bridgeCallbacks.sendRequest()` / `onResponse()` — race CLI and CCR permission prompts (`interactiveHandler.ts:395-452`) | 缺失 |
| 8 | Channel permission relay | Not implemented | Send permission prompts to Telegram/iMessage/Discord via MCP `channel_notification` (`interactiveHandler.ts:470-576`) | 缺失 |
| 9 | Pipe permission relay for subagents | Not implemented | `tryRelayPipePermissionRequest()` — forward permission requests between subagent and leader (`interactiveHandler.ts:328-383`) | 缺失 |
| 10 | Auto mode acceptEdits fast path | Not implemented | Re-check with `mode: 'acceptEdits'` before classifier to skip expensive API call (`permissions.ts:600-656`) | 缺失 |
| 11 | Classifier cost tracking | Not implemented | `classifierCostUSD`, stage1/stage2 cost, session overhead % computation (`permissions.ts:730-816`) | 缺失 |
| 12 | Classifier fail-open/fail-closed | Not implemented | `tengu_iron_gate_closed` GrowthBook feature gate with 30-min refresh (`permissions.ts:849-897`) | 缺失 |
| 13 | PowerShell auto-mode protection | Not implemented | PowerShell excluded from classifier unless `POWERSHELL_AUTO_MODE` flag (`permissions.ts:572-591`) | 缺失 |
| 14 | MCP server-level permission rules | No MCP-specific rule matching | `mcpInfoFromString()` for rules like `mcp__server1` or `mcp__server1__*` (`permissions.ts:258-268`) | 缺失 |
| 15 | Agent(agentType) denial rules | Not implemented | `getDenyRuleForAgent()`, `filterDeniedAgents()` — deny specific agent types (`permissions.ts:308-343`) | 缺失 |
| 16 | Sandbox auto-allow for bash | Not implemented | `canSandboxAutoAllow` — auto-allow bash commands when sandboxed (`permissions.ts:1211-1215`) | 缺失 |

### 44.4 File Watcher

| # | Aspect | Go | Upstream (`gitFilesystem.ts`) | Type |
|---|--------|----|-----------------------------|------|
| 1 | Git HEAD watcher | Not implemented | `GitFileWatcher` class with `fs.watchFile` polling (1s interval), caches branch/SHA, watches HEAD + branch ref + config (line 333-449) | 缺失 |
| 2 | Git ref file watching | Not implemented | `watchCurrentBranchRef()` — dynamically watches current branch's loose ref file, switches on branch change (line 391-427) | 缺失 |
| 3 | Git directory resolution cache | Not implemented | `resolveGitDir()` with memoization, handles worktrees/submodules via `gitdir:` files (line 28-76) | 缺失 |
| 4 | Git SHA/ref validation | Not implemented | `isSafeRefName()` (allowlist-based), `isValidGitSha()` (40/64 hex) — security against tampered .git files (line 98-131) | 缺失 |

### 44.5 Cost Tracking

| # | Aspect | Go | Upstream (`cost-tracker.ts`, `modelCost.ts`) | Type |
|---|--------|----|---------------------------------------------|------|
| 1 | Per-model cost pricing | Not implemented | `MODEL_COSTS` map with per-model pricing for input, output, cache read, cache write, web search (`modelCost.ts`) | 缺失 |
| 2 | USD cost calculation | Not implemented | `calculateUSDCost()` — computes `(input/1M)*inputRate + (output/1M)*outputRate + (cache/1M)*cacheRate + web_search*$0.01` (`modelCost.ts:177-180`) | 缺失 |
| 3 | Session cost persistence | Not implemented | `saveCurrentSessionCosts()` writes to project config: totalCostUSD, per-model usage, durations, FPS metrics (`cost-tracker.ts:143-175`) | 缺失 |
| 4 | Session cost restoration | Not implemented | `restoreCostStateForSession()` reads from config on resume (`cost-tracker.ts:130-137`) | 缺失 |
| 5 | Per-model usage tracking | Basic: `totalInputTokens`, `totalOutputTokens` (agent_loop.go:320-325) | `ModelUsage` per model: inputTokens, outputTokens, cacheReadInputTokens, cacheCreationInputTokens, webSearchRequests, costUSD, contextWindow, maxOutputTokens (`cost-tracker.ts:250-276`) | 简化 |
| 6 | Cost summary display | Basic token counts in usage XML | `formatTotalCost()` — formatted display with API duration, wall duration, lines added/removed, per-model usage breakdown (`cost-tracker.ts:228-244`) | 简化 |
| 7 | OTel cost/token counters | Not implemented | `getCostCounter()?.add(cost)`, `getTokenCounter()?.add(tokens, {type})` — Prometheus/OTel metrics (`cost-tracker.ts:291-301`) | 缺失 |

---


---

