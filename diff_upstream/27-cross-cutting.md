# Cross-Cutting

> Architecture, concurrency, utilities, gap analysis

## Sections Included
- [##] Line 380-404 -- ## Summary of Key Gaps
- [###] Line 2472-2482 -- ### Summary of Gap Types
- [###] Line 2483-2497 -- ### Key Go Enhancements Not in Upstream
- [##] Line 2560-2585 -- ## 18. Async Task Management (`agent_task.go`)
- [##] Line 2948-2991 -- ## Summary of Key Differences (Sections 16-27)
- [##] Line 3989-4018 -- ## 10. agent_task.go — 异步任务管理
- [##] Line 4215-4227 -- ## 总结：6个文件的关键差异
- [##] Line 4228-4511 -- ## Entry Points 与 Build System 对比 (Go vs 上游 TypeScript)
- [##] Line 5628-5906 -- ## Gap Analysis: Previously Uncovered Topics
- [##] Line 8000-8239 -- ## Final Comprehensive Gap Check — 2026-05-12
- [##] Line 8760-8778 -- ## B. Top 10 Critical Gaps
- [##] Line 8798-8818 -- ## D. Simplifications Map
- [##] Line 8819-8841 -- ## E. Architectural Differences
- [##] Line 8842-8868 -- ## F. Implementation Priority
- [##] Line 10634-10705 -- ## 50. Cross-Cutting Architectural Patterns

---

## Content

## Summary of Key Gaps

### Critical Gaps (may cause correctness issues)
1. **Redacted thinking blocks** -- Go has no handling; API responses with redacted thinking will lose content
2. **Thinking signature** -- Go doesn't capture or preserve `signature_delta` for thinking blocks
3. **Single vs multiple cache markers** -- Go's 4-breakpoint strategy conflicts with upstream's documented KVCC behavior (claude.ts:3164-3174)
4. **Tool result pairing** -- Go lacks `ensureToolResultPairing`; malformed tool call/result sequences may cause API 400s
5. **Role alternation** -- Go doesn't merge consecutive user messages; may cause API rejections

### Missing Features (may cause degraded behavior)
6. **Model fallback** -- No streaming fallback to alternate model on errors
7. **Rate limit tracking** -- No quota status or throttle header parsing
8. **Cache edits** -- No cache_editing, cache_reference, or KVCC optimization
9. **Cache scope** -- No `global` or `org` cache scopes, only `ephemeral`
10. **Refusal handling** -- No detection of policy refusals in streaming
11. **Image validation** -- No pre-send image size validation
12. **Orphaned thinking filtering** -- May send orphaned thinking-only messages causing API 400s

### Architectural Differences
13. **System prompt assembly** -- Go uses monolithic string templates; upstream uses array of sections with feature-gated registry
14. **Normalization pipeline** -- Go is single-pass; upstream is 10+ pass with progressive filtering
15. **Error recovery** -- Go has manual partial-state management; upstream has `withRetry` framework with fallback models

---


---

### Summary of Gap Types

| Type | Count | Description |
|------|-------|-------------|
| **缺失** | 17 | Feature exists in upstream but not in Go |
| **简化** | 16 | Feature exists but with reduced scope/logic |
| **Go增强** | 14 | Feature exists in Go but not in upstream |
| **Go适配** | 9 | Different implementation due to Go/terminal constraints |
| **差异** | 2 | Different approach to the same problem |
| **匹配** | 1 | Behavior matches upstream |


---

### Key Go Enhancements Not in Upstream

1. **Two-stage auto classifier** (15.13) — faster allow decisions
2. **Tool-use-as-text detection** (15.2) — prevents model stuck patterns
3. **Think filter state machine** (15.3) — terminal display feature
4. **@context expansion** (15.41-15.44) — `@file:`, `@folder:`, `@diff:`, `@staged:`, `@git:`, `@url:` syntax
5. **StreamBus pub/sub** (15.5) — event distribution for Go architecture
6. **Explicit DeltasState** (15.1) — formal state machine for retry safety
7. **Session memory expiration** (15.25) — prevents unbounded growth
8. **Rate limit display formatting** (15.38) — terminal-friendly ASCII display
9. **5x session memory token budget** (15.24) — 60K vs 12K total tokens
10. **Classifier whitelist with 40+ safe tools** (15.14) — avoids unnecessary LLM calls

---


---

## 18. Async Task Management (`agent_task.go`)

**Upstream reference**: `src/tasks.ts` + `src/tasks/` directory + `src/utils/tasks.ts`

### 18.1 Task state model
- **上游**: Task state managed via `AppState` React context with `TaskStore`; tasks are React components (e.g., `LocalShellTask`, `InProcessTeammateTask`, `MonitorMcpTask`). Rich lifecycle with `stopTask()` and `killShellTasks()`
- **Go版**: `TaskState` struct with `sync.Mutex`, fields: `ID`, `Status` (5-state enum: Pending/Running/Completed/Failed/Killed), `Result`, `Error`, `CancelFunc`, `Process`, `OutputFile`, `PendingMessages`, `evictAfter` (auto-cleanup 30s after completion) (agent_task.go:46-65)
- **类型**: Go适配 — Go uses explicit struct + mutex instead of React state; adds `evictAfter` for automatic cleanup

### 18.2 Background bash task management
- **上游**: `LocalShellTask` renders shell output via React; `killShellTasks` kills all shell tasks for an agent; output stored in temp files via `diskOutput.ts`
- **Go版**: `spawnBackgroundBashCommand()` spawns `exec.Cmd` in goroutine, writes to `.claude/tasks/bash/` output files, sends XML `<task-notification>` via channel, supports `registerExistingProcessAsBgTask()` for timeout conversions. 50K char truncation with head+tail split (agent_task.go:291-492)
- **类型**: Go增强 — includes timeout-to-background conversion, XML notification protocol, output file management

### 18.3 MCP background task management
- **上游**: `MonitorMcpTask` component handles MCP tasks in React
- **Go版**: `registerMCPTimeoutAsBgTask()` / `finishMCPBgTask()` — MCP tasks that timeout are registered as background tasks with `mcp-` prefixed IDs, output written to `.claude/tasks/mcp/` files (agent_task.go:583-716)
- **类型**: Go适配 — Go consolidates MCP background tasks into the same TaskStore; upstream uses separate React component

### 18.4 Task eviction
- **上游**: No explicit eviction mechanism; tasks removed on component unmount
- **Go版**: `CleanupEvicted()` removes tasks whose `evictAfter` timestamp has passed (30s after completion), also deletes output files (agent_task.go:178-191)
- **类型**: Go增强 — automatic resource cleanup absent in upstream

---


---

## Summary of Key Differences (Sections 16-27)

### Go Enhancements (absent in upstream)
1. **15-category error taxonomy** (16.1) — far richer than upstream's flat class hierarchy
2. **Centralized error classification pipeline** (16.2) — deterministic priority ordering
3. **Billing vs rate-limit disambiguation** (16.4) — 9 billing + 13 rate-limit patterns
4. **13 LLM-callable file history tools** (20.2) — upstream uses TUI only
5. **Work task dependency graph** (22.1) — with cycle detection and validation
6. **Explicit skill tracking** (23.3) — with post-compact recovery
7. **LLM-autonomous skill discovery** (23.4/26.9) — search/list/read tools
8. **Fork mode with structured protocol** (25.2) — explicit fork boilerplate
9. **4-layer tool filtering** (25.3) — structured deny/allow system
10. **Model alias resolution** (25.4) — with parent-tier matching
11. **Exa search** (26.5), **Process tool** (26.6), **Runtime info** (26.7), **Tool search** (26.13), **MultiEdit** (26.15), **Git tool** (26.19), **ListDir** (26.24), **FileOps** (26.27) — no upstream equivalents
12. **Inter-agent messaging** (26.8) — explicit SendMessageTool
13. **TodoWrite with dependencies** (26.14) — upstream is flat list

### Go Simplifications (upstream has more)
1. **No telemetry-safe error handling** (16.6)
2. **No stack trace truncation** (16.7)
3. **No subagent context isolation** (19.3) — forked agent shares parent global state
4. **No streaming in forked agent** (19.2) — direct API calls only
5. **No conditional/dynamic skill discovery** (23.1)
6. **No MCP skills** (23.5)
7. **No file history enable/disable** (20.8)
8. **No VSCode integration** (20.9)
9. **No rich plan mode UI** (26.4)
10. **No permission hooks** (27.3)
11. **GlobTool without gitignore** (26.25)
12. **GrepTool without ripgrep** (26.26)
13. **Memory tool is basic** (26.21)

### Go Adaptations (same concept, different implementation)
1. **Progress Writer vs React components** (17.1) — io.Writer vs React tree
2. **TaskState struct vs React AppState** (18.1) — mutex-guarded struct vs React context
3. **Snapshot-based file history vs backup-based** (20.1) — JSON files vs hard-link copies
4. **AgentTaskStore vs React TaskStore** (26.1) — explicit store vs React state
5. **Callback-based agent spawning vs React lifecycle** (26.2)
6. **Manual type coercion vs Zod** (26.16)
7. **Centralized filesystem safety vs distributed checks** (26.17)
8. **MCPToolWrapper vs dynamic MCP registration** (26.23)

---


---

## 10. agent_task.go — 异步任务管理

### 10.1 TaskStore 架构

| Go | Upstream |
|---|---|
| `TaskStore` 管理 `map[string]*TaskState`，纯内存 map（`agent_sub.go:96-99`） | 上游任务存储在 `appState.tasks` map，通过 `registerTask()` / `updateTaskState()` 管理（`LocalAgentTask/LocalAgentTask.tsx`） |
| `TaskState` 含 `sync.Mutex` 保护内部字段（`agent_sub.go:46-65`） | 上游通过 React state 不可变更新，无显式锁 |
| `CleanupEvicted()` 定期清理过期任务（`agent_sub.go:178-191`） | 上游 `evictTerminalTask()` 在状态更新时检查并清理 |
| **Go 缺失：无 `retain`/`diskLoaded`/`panelGraceDeadline` 生命周期** | 上游有完整生命周期管理（`LocalAgentTask.tsx`），包括 UI 保留、磁盘加载、面板优雅期 |

### 10.2 Bash 后台任务

| Go | Upstream |
|---|---|
| `spawnBackgroundBashCommand()` 启动 shell 命令，输出写入 `.claude/tasks/bash/{taskID}.output`（`agent_sub.go:311-414`） | 上游使用 `LocalShellTask`（`LocalShellTask/LocalShellTask.tsx`）管理 shell 进程 |
| `registerExistingProcessAsBgTask()` 将超时进程转为后台任务（`agent_sub.go:510-579`） | **上游无等效机制** — 超时进程直接报告超时，不转为后台任务 |
| `registerMCPTimeoutAsBgTask()` 将超时 MCP 调用转为后台任务（`agent_sub.go:585-640`） | **上游无等效机制** — MCP 超时后直接返回 |
| 通过 `notificationChan` 发送 XML 格式通知（`agent_sub.go:478-491`） | 上游通过 React state 更新 UI，不使用 XML 通知格式 |
| 输出文件截断逻辑：超过 50000 字符则首尾各保留 25000（`agent_sub.go:708-713`） | 上游使用 `shellOutputTruncate()` 做类似截断 |

### 10.3 MCP 后台任务

| Go | Upstream |
|---|---|
| `registerMCPTimeoutAsBgTask()` + `finishMCPBgTask()` 管理超时 MCP 调用 | 上游 MCP 超时直接返回错误，不继续后台运行 |
| MCP 任务使用 `mcp-` 前缀 ID（`agent_sub.go:586`） | 上游使用统一的任务 ID 系统 |

---


---

## 总结：6个文件的关键差异

| 文件 | Go 版相对上游的核心差异 |
|---|---|
| **context.go** | Go 使用序列化/反序列化往返进行压缩；上游直接操作 Message 数组。Go 无 Session Memory 持久化摘要和 forked agent 生成摘要机制。ToolStateTracker 的 epoch 机制是有效的 Go 适配方案。 |
| **agent_sub.go** | Go 用 goroutines 实现真正的并行子代理；上游用单线程 event loop。Go 无 CacheSafeParams 和 queryTracking 遥测。子代理类型硬编码，不支持自定义 agent。 |
| **agent_task.go** | Go 的 bash/MCP 后台任务有超时转后台的独特设计；上游无此机制。Go 使用 XML 通知格式；上游使用 React state。无 retain/diskLoaded 生命周期管理。 |
| **work_task.go** | Go 纯内存存储，无持久化；上游基于文件持久化 + 文件锁支持多进程并发。Go 有循环检测；上游无。Go 缺失 claimTask、resetTaskList、getAgentStatuses 等 swarm 协作功能。 |
| **filehistory.go** | Go 存完整内容快照 + JSON 持久化；上游存 hash 引用 + 文件副本。Go 有独特的标签/时间线/历史搜索系统（上游无）；上游有自动跟踪编辑 + VSCode 通知（Go 无）。 |
| **context_references.go** | Go 使用文本预处理模式（@引用→Markdown注入）；上游使用 UI 附件模式。Go 支持 @diff/@staged/@git/@url 等丰富引用类型（上游无）；上游支持 MCP 资源和 Agent 引用（Go 无）。Go 有 token 预算门控（上游无）。 |

---


---

## Entry Points 与 Build System 对比 (Go vs 上游 TypeScript)

### 1. CLI 入口与参数解析

| 方面 | Go 版 | 上游 TypeScript |
|---|---|---|
| 入口文件 | `main.go` (`main.go:19-188`) | `src/entrypoints/cli.tsx` -> `src/main.tsx` |
| 参数库 | 标准库 `flag` 包 (`flag.String`, `flag.Int`, `flag.Bool`, `flag.Parse`) | Commander.js (`@commander-js/extra-typings`)，定义在 `main.tsx:1262-5780` |
| 快速路径 | 无 -- 所有代码同步执行 | `cli.tsx` 有多条快速路径：`--version`(74行)、`--acp`(136行)、`--daemon-worker`(173行)、`--bg`(256行)、`--computer-use-mcp`(124行)、`environment-runner`(324行)、`self-hosted-runner`(336行) 等，均在导入 main.tsx 之前短路 |
| 子命令 | 无子命令；仅 7 个 slash 命令在 REPL 内实现 (`/quit /tools /mode /help /compact /clear /resume /agents`) | 8+ 顶级子命令：`mcp` (5744行)、`auth` (6073行)、`plugin` (6134行)、`agents` (6376行)、`daemon` (239行)、`job` (297行)、`doctor` (6544行)、`task` (6689行)、`ssh` (5970行)、`open` (6007行)、`update` (6601行) |
| 主命令 | 位置参数作为 prompt (`flag.Args()`) | `program.argument("[prompt]", ...)` 在 `main.tsx:1246` |
| 帮助系统 | 手动 `fmt.Println` 输出 (`main.go:396-406`) | Commander 自动生成排序帮助 (`main.tsx:1263 configureHelp(createSortedHelpConfig())`) |

**关键差异**: Go 版用一个简单的 `main()` 函数处理所有逻辑；上游 `cli.tsx` 是一个精心设计的 bootstrap 调度器，在完全加载 main.tsx 之前通过动态 import 实现零/低延迟快速路径。

### 2. 交互模式 vs 无头模式

| 方面 | Go 版 | 上游 TypeScript |
|---|---|---|
| 模式检测 | 检测 stdin 是否为 TTY (`os.Stdin.Stat()`, `main.go:223-229`) | `isNonInteractive = hasPrintFlag || hasInitOnlyFlag || hasSdkUrl || (!forceInteractive && !process.stdout.isTTY)` (`main.tsx:1131-1132`) |
| 交互模式 | `runInteractive()` 函数，手写的 select/chan 循环处理 Ctrl+C 信号 (`main.go:190-485`) | React Ink TUI，通过 `launchRepl()` 渲染 `<App><REPL /></App>` (`replLauncher.tsx:14-28`) |
| 无头模式 | `agent.Run(prompt)` 返回字符串，`fmt.Println(result)` 输出 (`main.go:164-177`) | `print.ts` 处理 `--print` 模式，支持 `--output-format text/json/stream-json` (`main.tsx:3823-3890+`) |
| Ctrl+C | 自定义信号处理，2秒内双击退出 (`main.go:200-218`) | `abortController.signal` + Ink TUI 事件系统 |
| 输出格式 | 纯文本 (stdout) 或 stderr (流式) | `text`/`json`/`stream-json` 三种格式 (`main.tsx:1403-1408`) |
| 输入格式 | 纯文本 readline | `text` 或 `stream-json` (JSON-RPC over stdio) (`main.tsx:1426-1431`) |
| TUI 组件 | 无 -- 纯终端文本 | Ink (React 终端 UI)，包含 React reconciler (`react-reconciler`) |
| 启动画面 | 无 | Logo v2 + 最近活动显示 (`setup.ts:381-388`) |
| 信任对话框 | 无 -- 假设信任 | Ink 对话框，需用户接受才能继续 (`init.ts:244-249`) |

**关键差异**: 上游使用完整的 React TUI 栈（Ink + React + reconciler），Go 版是纯文本 readline REPL。上游有复杂的 TTY 检测逻辑考虑多种入口（SDK、远程、daemon），Go 版仅检查 stdin/stdout 是否为字符设备。

### 3. Build System 对比

| 方面 | Go 版 | 上游 TypeScript |
|---|---|---|
| 构建命令 | `go build` | `bun run build.ts` (dist/) 或 `bun run compile.ts` (单文件 exe) |
| 构建工具 | Go 编译器 (内置) | Bun (bundler + compiler) |
| 编译输出 | 单个静态链接二进制 (`miniclaudecode-go`) | 两个模式：(1) `dist/` 目录含多个 .js 文件 + `cli-bun.js`/`cli-node.js` 入口；(2) 单文件 `claude.exe`/`claude` (compile 模式) |
| 构建步骤 | 一步 (`go build`) | 多步：(1) 清理输出目录；(2) Bun.build 打包；(3) 后处理 `import.meta.require` 兼容性；(4) 复制原生 .node 文件；(5) 生成 shebang 入口脚本 (`build.ts:1-96`) |
| 编译步骤 | N/A | 额外 `compile.ts`：(1) 生成 ripgrep base64 资产；(2) 修补 SDK ripgrep 路径；(3) `bun build --compile` 生成单文件 exe (`compile.ts:1-198`) |
| Feature Flags | 无 | `feature("...")` 从 `bun:bundle` 导入，编译时 DCE (`compile.ts:39 featureArgs`) |
| 宏定义 | 无 | `getMacroDefines()` 注入 `MACRO.VERSION` 等 (`compile.ts:23-29`) |
| 原生模块 | 无 (纯 Go) | 嵌入 .node 文件 (audio-capture, image-processor, computer-use 等) (`compile.ts:50-68`) |
| 二进制嵌入 | N/A | Ripgrep 二进制 base64 编码嵌入，运行时解码到临时文件 (`compile.ts:83-129`) |

**关键差异**: Go 的 `go build` 是单步、自包含、无后处理的。上游需要 5 步构建流水线，包括代码修补、原生模块嵌入和资产生成。上游有复杂的 feature flag 系统实现编译时死代码消除 (DCE)。

### 4. 依赖管理

| 方面 | Go 版 | 上游 TypeScript |
|---|---|---|
| 包管理器 | Go modules (`go.mod`, `go.sum`) | npm + Bun workspaces |
| 依赖数量 | 5 个直接依赖 + 5 个间接依赖 | 140+ devDependencies (含大量 @opentelemetry、@anthropic-ai、@aws-sdk 等) |
| 核心依赖 | `anthropic-sdk-go`, `doublestar`, `go-difflib` | `@anthropic-ai/claude-agent-sdk`, `@anthropic-ai/sdk`, `react`, `react-reconciler`, `@modelcontextprotocol/sdk`, `zod`, `chalk` 等 |
| Workspace 结构 | 无 | Monorepo with `packages/*`, `packages/@ant/*`, `packages/@anthropic-ai/*` (`package.json:32-36`) |
| 运行时依赖 | 无 (静态编译) | Node.js >= 18 或 Bun >= 1.2.0 (`package.json:25`) |
| 原生编译 | 无 | 多个 .node 原生模块 (audio-capture, image-processor, modifiers, url-handler, color-diff, computer-use-input/swift) |
| OpenTelemetry | 无 | 完整的 OTEL 栈：exporters (grpc/http/proto), SDK (logs/metrics/traces), semantic conventions (20+ 包) |

**关键差异**: Go 版极其精简（10 个依赖，无原生编译）；上游是重量级 monorepo（140+ 依赖，原生模块，完整 OTEL 可观测性栈）。

### 5. 部署模型

| 方面 | Go 版 | 上游 TypeScript |
|---|---|---|
| 产物 | 单个二进制文件 (~10-20MB 静态链接) | 编译模式：单文件 exe (~100MB+，含嵌入资源)；构建模式：dist/ 目录 + node_modules |
| 安装方式 | 复制二进制到 PATH | npm install / bun install / 原生安装器 (`package.json:60 postinstall`) |
| 自动更新 | 无 | `update` 子命令，版本锁定/回滚机制 (`main.tsx:6557-6621`) |
| 运行时需求 | 无 -- 完全自包含 | Node.js/Bun 运行时 + npm 包 |
| 平台特定二进制 | 每个平台单独编译 (GOOS/GOARCH) | Bun 编译模式嵌入当前平台 ripgrep；SDK 模式分发多平台 |
| 发布流程 | 手动编译 | `npm publish` (含 prepublishOnly 钩子构建) |

**关键差异**: Go 版是典型的"单个二进制部署"模式；上游更像传统的 Node.js 包分发（npm install）+ 可选的编译为单文件 exe。

### 6. 跨平台支持

| 方面 | Go 版 | 上游 TypeScript |
|---|---|---|
| 编译时跨平台 | `GOOS=linux/windows/darwin GOARCH=amd64/arm64 go build` | Bun 编译模式仅嵌入当前平台原生模块 (`compile.ts:100-104`) |
| 平台检测 | `runtime.GOOS`, `os.Getenv("USERPROFILE")` vs `os.Getenv("HOME")` (`config.go:135-144`) | `process.platform`, `process.arch`, `process.getuid()` |
| Windows 处理 | `filepath.FromSlash` 路径规范化 (`main.go:36`) | `setShellIfWindows()`, git-bash 检测 (`init.ts:204`) |
| 终端备份恢复 | 无 | macOS Terminal.app / iTerm2 备份恢复 (`setup.ts:117-154`) |
| 原生模块 | 无 -- 纯 Go | 平台特定 .node 文件：macOS (computer-use, modifiers, url-handler), 跨平台 (audio-capture, image-processor) |
| 信号处理 | `syscall.SIGINT` (`main.go:199`) | `signal-exit` 包 + `abortController` |
| TTY 检测 | `os.Stdin.Stat()` (`main.go:223-229`) | `process.stdin.isTTY` / `process.stdout.isTTY` |
| 路径处理 | `filepath.Clean`, `filepath.Join` (Go 标准库) | `path.join`, `resolve`, `relative` (Node.js path 模块) |

**关键差异**: Go 版通过交叉编译原生支持所有平台，无需平台特定代码。上游需要为不同平台嵌入不同的原生 .node 模块，且 macOS 特有功能（Terminal 备份、keychain）在 Windows 上不运行。

### 7. 环境引导 (Bootstrap)

| 方面 | Go 版 | 上游 TypeScript |
|---|---|---|
| 认证 | `ANTHROPIC_API_KEY` / `ANTHROPIC_AUTH_TOKEN` 环境变量 或 `.claude/settings.json` (`main.go:68-78`) | OAuth 登录流、API Key Helper、Claude.ai 订阅、SSO、多提供商 (Bedrock/Vertex/Foundry) (`main.tsx:6073-6117`) |
| Auth 命令 | 无 | `claude auth login/status/logout` 子命令 (`main.tsx:6073-6117`) |
| 配置优先级 | 默认 -> settings.json(项目) -> settings.json(家目录) -> 环境变量 -> 命令行标志 (`main.go:43-92`) | `enableConfigs()` -> `applySafeConfigEnvironmentVariables()` -> trust dialog -> `applyConfigEnvironmentVariables()` (`init.ts:60-255`) |
| 配置来源 | `.claude/settings.json`, `.mcp.json`, `~/.claude/settings.json`, `~/.mcp.json` (`config.go:149-281`) | 同上 + MDM (macOS)、远程托管设置、策略限制、GrowthBook feature flags |
| 初始化步骤 | `DefaultConfig()` -> 加载文件 -> 加载环境变量 -> 加载 MCP -> 加载 Skills -> 创建 SessionMemory -> 创建 AgentLoop (`main.go:43-158`) | `cli.tsx` bootstrap -> `init()` (memoized) -> `setup()` -> trust dialog -> 挂载 React TUI 或进入 print 模式 |
| 遥测 | 无 | OpenTelemetry (metrics/logs/traces) + 1P 事件日志 + Sentry + Langfuse + DataDog (`init.ts:97-108, 163-169, 265-357`) |
| 策略限制 | 无 -- 仅 `AllowedCommands`/`DeniedPatterns` | 企业策略限制 (`policyLimits`)，组织级配置 (`remoteManagedSettings`) |
| Feature flags | 无 | GrowthBook A/B testing + `feature("...")` 编译时 DCE |
| 预连接 | 无 | `preconnectAnthropicApi()` 在 `init()` 中预热 TCP+TLS 连接 (`init.ts:177`) |
| mTLS/代理 | 无 | 全局 mTLS + HTTP 代理配置 (`init.ts:144-160`) |
| Git 检测 | 无 | `detectCurrentRepository()` 异步检测 + PR 链接 (`init.ts:127-128`) |
| JetBrains 检测 | 无 | `initJetBrainsDetection()` (`init.ts:123`) |

**关键差异**: Go 版是极简的"API key -> 直接调用"模式；上游有完整的 OAuth 认证流、企业策略、遥测栈、mTLS 代理、预连接优化等。Go 版不需要用户进行交互认证，只需一个 API key。

### 8. 无头模式 (Headless/Print) 详细对比

| 方面 | Go 版 | 上游 TypeScript |
|---|---|---|
| 进入条件 | 位置参数存在 (`flag.Args()`, `main.go:161-162`) | `-p/--print` 标志 或 stdout 非 TTY (`main.tsx:1127-1132`) |
| 输出 | `agent.Run(prompt)` 返回完整字符串 (`main.go:164`) | `print.ts` 中的异步生成器处理 stream-json/text/json (`main.tsx:3823+`) |
| 输出格式 | 纯文本 | `text`/`json`/`stream-json` (`main.tsx:1403-1408`) |
| JSON Schema | 无 | `--json-schema` 结构化输出 (`main.tsx:1409-1415`) |
| 输入格式 | 纯文本字符串 | `text` 或 `stream-json` (JSON-RPC over stdio) (`main.tsx:1426-1431`) |
| 流式输出 | `--stream` 标志 (`main.go:25`)，stderr 输出 | `--output-format stream-json` 实时流式 |
| 权限 | `--mode bypass` (`main.go:23`) | `--dangerously-skip-permissions` (需沙箱验证) (`main.tsx:1437-1441`) |
| 最大轮次 | `--max-turns 90` 默认 (`main.go:24`) | `--max-turns` (`main.tsx:1458-1466`) |
| 成本限制 | 无 | `--max-cost` 美元金额限制 (`main.tsx:1468-1474`) |
| 会话持久化 | 始终保存到 `.claude/transcripts/` | `--no-session-persistence` 可禁用 (`main.tsx:1622-1627`) |
| 模型回退 | 无 | `--fallback-model` (`main.tsx:1665-1669`) |
| Hook 事件 | 无 | `--include-hook-events` 流式生命周期事件 (`main.tsx:1416-1420`) |
| 部分消息 | 无 | `--include-partial-messages` (`main.tsx:1421-1425`) |
| 子命令支持 | 无 | 无头模式过滤可用命令 (`main.tsx:3867-3875`) |
| 初始状态 | `NewAgentLoop(cfg, registry, *stream)` (`main.go:153`) | `headlessInitialState` AppState + print.ts 引擎 (`main.tsx:3878-3890+`) |

**关键差异**: Go 版的无头模式极其简单 -- 调用 agent.Run() 并打印结果字符串。上游有无头模式的完整管道：结构化输出 (JSON Schema)、流式 JSON-RPC、成本限制、权限验证（沙箱检查）、模型回退等。

### 9. Query Loop 对比

| 方面 | Go 版 | 上游 TypeScript |
|---|---|---|
| 实现 | `agent.go` 中的 `AgentLoop.Run()` | `query.ts` 中的 `query()` 异步生成器 (`query.ts:222-282`) |
| 工具循环 | 简单的 while 循环检测 tool_use | `while(true)` 循环，7 种 continue 原因：`next_turn`, `max_output_tokens_recovery`, `max_output_tokens_escalate`, `stop_hook_blocking`, `reactive_compact_retry`, `collapse_drain_retry`, `token_budget_continuation` |
| 压缩 | `context.go` 中显式 `Compact()` 调用 | 多层压缩：snip -> microcompact -> context-collapse -> autocompact -> reactive compact (`query.ts:444-586`) |
| 工具执行 | 顺序执行，调用 `tool.Use()` | 支持流式工具执行 `StreamingToolExecutor` 或传统 `runTools()` (`query.ts:1427-1455`) |
| 模型回退 | 无 | `FallbackTriggeredError` 自动切换到备用模型 (`query.ts:941-999`) |
| 最大 token 恢复 | 无 | 3 次恢复循环 + 64K escalation (`query.ts:1232-1302`) |
| Stop hooks | 无 | `handleStopHooks()` 阻止/继续控制 (`query.ts:1314-1352`) |
| Token 预算 | 无 | `TOKEN_BUDGET` feature (`query.ts:1355-1401`) |
| Langfuse 追踪 | 无 | `createTrace` + `endTrace` 分布式追踪 (`query.ts:236-270`) |
| 内存预取 | 无 | `startRelevantMemoryPrefetch` 异步预取 (`query.ts:344-347`) |
| Skill 预取 | 无 | `startSkillDiscoveryPrefetch` 异步发现 (`query.ts:374-378`) |

### 10. Slash 命令对比

| Go 版 | 上游对应 | 差异 |
|---|---|---|
| `/quit, /exit, /q` | `:quit` 命令 | Go 简单 break loop；上游有状态保存 |
| `/tools` | `:tools` 命令 | Go 列出 registry 工具名+描述；上游列出完整工具树+参数 |
| `/mode` | `:permission-mode` 命令 | Go 支持 ask/auto/bypass/plan；上游有更多模式 (plan/auto/bypass) |
| `/help` | `:help` 命令 | Go 手动打印帮助文本；上游有动态帮助 |
| `/compact` | `:compact` 命令 | Go 调用 `ForceCompact()`；上游触发完整压缩管道 |
| `/partialcompact` | 无直接对应 | Go 独有 -- 方向性部分压缩 |
| `/clear` | `:clear` 命令 | Go 清除消息历史 + 文件缓存；上游清除整个会话状态 |
| `/resume` | `:resume` 命令 | Go 从 .jsonl 转录恢复；上游从 session JSONL 恢复 |
| `/agents` | 无直接对应 | Go 独有 -- 后台代理管理 (list/show/stop) |
| N/A | `/mcp`, `/plugin`, `/skill`, `/task`, `/eject`, `/cost`, `/model`, `/vim` 等 52+ 子命令 | Go 版缺失 |

### 11. 启动性能优化

| 方面 | Go 版 | 上游 TypeScript |
|---|---|---|
| 导入优化 | 无 -- 所有代码在 main() 前加载 | 动态 import 延迟加载大部分模块 (`cli.tsx:139-140, cli.tsx:188-193`) |
| 预加载 | 无 | 早期输入捕获 (`earlyInput.js`)、MDM 设置预读、keychain 预取 (`main.tsx:14-25`) |
| Profile 检测 | 无 | `profileCheckpoint`/`profileReport` 启动分析器 (`main.tsx:12, cli.tsx:89`) |
| API 预连接 | 无 | `preconnectAnthropicApi()` (`init.ts:177`) |
| 插件预取 | 无 | `getCommands()` 异步预取 (`setup.ts:317-324`) |
| 版本锁定 | 无 | `lockCurrentVersion()` (`setup.ts:298`) |

### 11.1 MultiEditTool
- **上游**: `packages/builtin-tools/src/tools/FileEditTool/FileEditTool.ts` + `FileEditTool/utils.ts:531-550` (DESANITIZATIONS); 无独立 MultiEdit 工具，统一走 FileEditTool
- **Go版**: `tools/multi_edit.go:1`
- **类型**: Go增强

上游通过 FileEditTool 统一处理编辑（单文件编辑 + patch 展示），Go 版的 MultiEdit 更接近早期 Claude Code 的 `apply_multi_edit` 工具，提供原子回滚。DESANITIZATIONS 映射表（第531-550行）完全匹配上游。Go 版额外具备：read-before-write 验证（registry.CheckFileStale）、1GiB 文件大小保护、CRLF 保留、重叠编辑检测。上游 FileEditTool 使用 diff patch 机制和结构化展示。

### 11.2 Coercion (type coercion utilities)
- **上游**: `src/utils/semanticBoolean.ts:1` / `src/utils/semanticNumber.ts:1` — Zod preprocess 函数，仅覆盖 boolean 和 number 类型
- **Go版**: `tools/coercion.go:1`
- **类型**: Go增强

上游的 semanticBoolean 和 semanticNumber 是 Zod schema 预处理器，分别处理字符串→布尔和字符串→数字。Go 版的 CoerceArguments 覆盖更广泛：string↔integer/number/boolean/array/object、bool→number、float64→integer，还有 RemapDirParam 用于目录参数重映射。上游没有 array/object coercion。Go 版是独立的参数层适配，上游依赖 Zod 的 schema 层。

### 11.3 Filesystem Safety (UNC/binary/device path checks)
- **上游**: `src/utils/permissions/filesystem.ts:435-654` (checkPathSafetyForAutoEdit, hasSuspiciousWindowsPathPattern, isDangerousFilePathToAutoEdit) / `src/utils/permissions/pathValidation.ts:1`
- **Go版**: `tools/filesystem_safety.go:1`
- **类型**: 简化

Go 版完全对应上游的 Windows 路径安全检查逻辑。DANGEROUS_FILES 列表（.gitconfig, .bashrc, .zshrc 等）和 DANGEROUS_DIRECTORIES（.git, .vscode, .claude）完全一致。hasSuspiciousWindowsPathPattern 覆盖了所有上游模式：NTFS ADS（冒号检查）、8.3 短名、长路径前缀、尾随点/空格、DOS 设备名、三点路径、UNC 路径。差异：上游还有 containsVulnerableUncPath（深层 UNC 检测）和 isClaudeConfigPath 的更完整实现。Go 版缺少 .claude/skills/ 路径的细粒度作用域检测。

### 11.4 AskUserQuestion
- **上游**: `packages/builtin-tools/src/tools/AskUserQuestionTool/AskUserQuestionTool.tsx:1`
- **Go版**: `tools/ask_user_question.go:1`
- **类型**: 简化

上游使用 React 渲染完整的 UI 对话框，支持 multiSelect（多选）、previews（选项预览）、annotations（notes）。Go 版通过终端 stdin 实现简易版本，仅支持单选（1-4选项），无 preview/annotation 功能。参数 schema 中上游多了 `multiSelect` 字段和 `annotations` 输出。Go 版的终端渲染使用 `┌─┐` 边框，上游是 Ink 组件。两者都要求 2-4 选项，header max 12 字符。

### 11.5 EnterPlanMode + ExitPlanMode
- **上游**: `packages/builtin-tools/src/tools/EnterPlanModeTool/EnterPlanModeTool.ts:1` / `packages/builtin-tools/src/tools/ExitPlanModeTool/ExitPlanModeV2Tool.ts:1`
- **Go版**: `tools/enter_plan_mode.go:1` / `tools/exit_plan_mode.go:1`
- **类型**: 简化

上游 EnterPlanMode 集成完整的权限系统（applyPermissionUpdate, prepareContextForPlanMode），支持 interview phase 模式，有自定义 UI 组件。上游 ExitPlanMode V2 更复杂：支持 prompt-based permissions、plan 文件持久化、TeamCreate/AgentTool 集成、邮箱写入等。Go 版使用简单的 mode setter/getter 回调，支持 reason 和 summary 参数，pre-plan 模式恢复。缺少 upstream 的 classifier 激活 side effects 和 plan 文件持久化逻辑。

### 11.6 ExaSearch (web_search / web_fetch)
- **上游**: `packages/builtin-tools/src/tools/WebSearchTool/adapters/exaAdapter.ts:1` — Exa 是 WebSearchTool 的内部适配器，不独立暴露为工具
- **Go版**: `tools/exa_search.go:1` (ExaSearchTool + ExaGetContentsTool)
- **类型**: Go适配

上游 Exa 是 WebSearchTool 的一个 SearchAdapter（内部调用），不是独立工具。Go 版将其包装为独立工具 `web_search` 和 `web_fetch`，直接调用 Exa MCP（`https://mcp.exa.ai/mcp`）。两者使用相同的 MCP 协议和参数（query, type, numResults, livecrawl, contextMaxCharacters）。Go 版额外有 internal URL 安全检查。上游 WebSearchTool 支持多适配器（Brave, Bing, Exa, Google），Go 版仅 Exa。

### 11.7 Process (process management)
- **上游**: 无直接等价物；上游通过 BashTool 执行 `ps`/`kill`/`top` 等命令
- **Go版**: `tools/process.go:1`
- **类型**: Go-only feature

Go 版将常用进程管理操作（list/kill/pkill/pgrep/top/pstree）封装为独立工具，跨平台（Windows PowerShell + Unix）。上游没有独立的 process 工具，用户需要通过 BashTool 执行 shell 命令。Go 版包含输入消毒（sanitizePSInput 防 PowerShell 注入），信号处理（SIGTERM/SIGKILL），以及 Windows 8.3 路径安全处理。

### 11.8 RuntimeInfo
- **上游**: 无等价物
- **Go版**: `tools/runtime_info.go:1`
- **类型**: Go-only feature

Go 版输出 Go 运行时信息（Version, GOOS, GOARCH, NumCPU, NumGoroutine, 内存统计）。上游无对应工具。这是 Go 版特有的诊断工具。

### 11.9 SendMessageTool (inter-agent messaging)
- **上游**: `packages/builtin-tools/src/tools/SendMessageTool/SendMessageTool.ts:1`
- **Go版**: `tools/send_message_tool.go:1`
- **类型**: 简化

上游 SendMessageTool 集成完整的 agent 团队系统（teammateMailbox, agentTeams, inProcessBackend 等），支持多种后端。Go 版通过回调函数（SendMessageFunc/GetStatusFunc）简化实现，支持 agent_id 和 name 两种寻址方式。两者参数 schema 基本一致（agent_id, message, summary）。Go 版缺少上游的 annotation 支持和复杂的后端路由逻辑。

### 11.10 Skill Tools (read_skill, list_skills, search_skills)
- **上游**: `packages/builtin-tools/src/tools/SkillTool/SkillTool.ts:1` — 统一 SkillTool，通过 operation 参数控制
- **Go版**: `tools/skill_tools.go:1` (ReadSkillTool + ListSkillsTool + SearchSkillsTool)
- **类型**: Go适配

上游使用统一的 SkillTool（支持 install, read, search, uninstall 等操作），集成 command/plugin/skill 体系。Go 版拆分为三个独立工具：read_skill, list_skills, search_skills。SearchSkillsTool 使用简单的关键词打分（name=50, desc=20, tag=30, when_to_use=15），上游使用更复杂的 LLM-based 搜索和 skill 安装流程。Go 版缺少 plugin marketplace 集成和 skill installation 功能。

### 11.11 SystemTool
- **上游**: 无直接等价物；上游有 MonitorTool (`packages/builtin-tools/src/tools/MonitorTool/MonitorTool.tsx`) 但功能不同
- **Go版**: `tools/system_tool.go:1`
- **类型**: Go-only feature

Go 版提供 info/uname/df/free/top/uptime/who/w/hostname/arch 等操作，跨平台（Windows PowerShell + Unix）。上游 MonitorTool 是资源监控面板，非命令行工具封装。Go 版的 systemFreeDarwin 特殊处理 macOS 的 vm_stat 解析。

### 11.12 TaskOutputTool
- **上游**: `packages/builtin-tools/src/tools/TaskOutputTool/TaskOutputTool.tsx:1`
- **Go版**: `tools/task_output_tool.go:1`
- **类型**: 简化

上游 TaskOutputTool 支持所有任务类型（local_bash, local_shell, remote_agent, local_agent），有完整的 UI 渲染（React/Ink），支持 stdout/stderr 分离显示、BashToolResultMessage 集成、退出码展示。Go 版通过回调（TaskOutputFunc）简化实现，仅支持 agent 任务。参数 schema 基本一致（task_id, block, timeout）。上游 default block=true，Go 版 default block=false（避免阻塞 agent 循环）。

### 11.13 TerminalTool
- **上游**: 无等价物
- **Go版**: `tools/terminal_tool.go:1`
- **类型**: Go-only feature

Go 版提供 tmux/screen 会话管理（list/new/detach/attach/send/kill/rename），Unix-only。上游无独立终端工具，agent 的终端交互通过 BashTool 完成。Go 版缺少 tmux pane 级别的精细控制。

### 11.14 ToolSearchTool
- **上游**: `packages/builtin-tools/src/tools/ToolSearchTool/ToolSearchTool.ts:1`
- **Go版**: `tools/tool_search_tool.go:1`
- **类型**: 简化

上游 ToolSearchTool 搜索的是 **deferred tools**（MCP 工具等延迟加载的工具），支持 feature-gating 和 analytics 日志。Go 版搜索的是整个 tool registry（所有已注册工具）。Go 版支持三种查询形式（select:精确名、关键词搜索、+前缀搜索），上游主要支持 select: 和关键词搜索。Go 版 max_results 默认 10（上游默认 5）。Go 版缺少 MCP server 延迟状态、analytics 上报。

### 11.15 TodoWrite
- **上游**: `packages/builtin-tools/src/tools/TodoWriteTool/TodoWriteTool.ts:1`
- **Go版**: `tools/todo_write.go:1`
- **类型**: 简化

两者参数 schema 基本一致（todos 数组，content/status/activeForm）。上游使用 Zod strictObject 校验，输出 oldTodos + newTodos + verificationNudgeNeeded。上游集成 TodoListSchema 和 isTodoV2Enabled 特性开关。Go 版额外有 TodoList 状态管理（增量更新、turn 计数器、idle reminder 机制）。上游在 allDone 且 todos 全部 completed 时清空列表，Go 版直接更新。

### 11.16 BriefTool
- **上游**: `packages/builtin-tools/src/tools/BriefTool/BriefTool.ts:1`
- **Go版**: `tools/brief_tool.go:1`
- **类型**: 简化

---


---

## Gap Analysis: Previously Uncovered Topics

*Researched: 2026-05-12. Topics checked against existing diff_upstream.md coverage and supplemented where missing.*

---

### A. Model Routing / Provider System

### Files Compared
- **Upstream**: `src/utils/model/providers.ts` (57 lines), `src/utils/model/configs.ts` (120+ lines), `src/utils/model/model.ts` (700+ lines), `src/services/api/openai/`, `src/services/api/grok/`, `src/services/api/gemini/`
- **Go**: `config.go` (404 lines), `streaming.go`, `error_types.go`

### A.1 API Provider Routing

| Aspect | Go | Upstream TS |
|--------|-----|-------------|
| **Provider types** | Single provider (Anthropic API only) | 7 providers: `firstParty`, `bedrock`, `vertex`, `foundry`, `openai`, `gemini`, `grok` (`providers.ts:5-11`) |
| **Provider detection** | Not applicable — hardcoded Anthropic | `getAPIProvider()`: checks `modelType` setting, then env vars `CLAUDE_CODE_USE_BEDROCK`, `CLAUDE_CODE_USE_VERTEX`, `CLAUDE_CODE_USE_FOUNDRY`, `CLAUDE_CODE_USE_OPENAI`, `CLAUDE_CODE_USE_GEMINI`, `CLAUDE_CODE_USE_GROK` (`providers.ts:14-29`) |
| **First-party URL check** | Not applicable | `isFirstPartyAnthropicBaseUrl()`: validates `ANTHROPIC_BASE_URL` against `api.anthropic.com` (and `api-staging.anthropic.com` for ant users) (`providers.ts:40-56`) |
| **Per-provider model IDs** | Not applicable | `configs.ts` maps each model to 7 provider-specific IDs: e.g., Sonnet 4 = `claude-sonnet-4-20250514` (firstParty) / `us.anthropic.claude-sonnet-4-20250514-v1:0` (bedrock) / `claude-sonnet-4@20250514` (vertex) / etc. (`configs.ts:49-57`) |
| **OpenAI adapter** | Not applicable | `src/services/api/openai/` — full OpenAI-compatible API adapter with request body translation, thinking mode support, streaming |
| **Gemini adapter** | Not applicable | `src/services/api/gemini/` — Google Gemini API adapter |
| **Grok adapter** | Not applicable | `src/services/api/grok/` — xAI Grok API adapter |

**Gap**: Go is locked to the Anthropic first-party API. Upstream supports 7 providers with automatic routing, per-provider model ID mapping, and dedicated API adapters for OpenAI, Gemini, and Grok. This is a **fundamental architectural gap** — Go cannot be used with Bedrock, Vertex, or any non-Anthropic provider without significant refactoring.

### A.2 Model Selection and Aliases

| Aspect | Go | Upstream TS |
|--------|-----|-------------|
| **Model aliases** | Not supported | `opusplan`, `sonnet`, `haiku` — parsed by `parseUserSpecifiedModel()` (`model.ts:487-516`) |
| **opusplan** | Not supported | Uses Opus in plan mode, Sonnet otherwise. `getRuntimeMainLoopModel()` switches model based on `permissionMode === 'plan'` (`model.ts:197-211`) |
| **Default model** | `config.Model` string, hardcoded | Subscription-dependent: Max/TeamPremium → Opus 4.7[1m], others → Sonnet 4.6 (`model.ts:223-240`) |
| **[1m] suffix** | Supported via `modelContextWindow()` parsing | Supported via `has1mContext()` + `modelSupports1M()` — enables 1M context window for Sonnet 4, Opus 4.6, Opus 4.7 (`context.ts:36-54`) |
| **Fast mode** | Not supported | `fastMode` setting with org-level availability check, cooldown state, overage handling, and model speed parameter (`fastMode.ts:1-527`) |
| **Poor mode** | Not supported | `poorMode` setting — skips extract_memories and prompt_suggestion to reduce token consumption (`poorMode.ts:1-24`) |
| **Effort level** | Not supported | `effortLevel` setting — per-model default effort/budget_tokens for thinking (`antModels.ts:9-10`) |

**Gap**: Go has no model alias system, no subscription-dependent defaults, no fast/poor mode, and no effort level control. These are significant user-facing features that affect model selection, cost, and performance.

---

### B. Cost Tracking

### Files Compared
- **Upstream**: `src/cost-tracker.ts` (323 lines), `src/utils/modelCost.ts` (232 lines), `src/services/providerUsage/` (store.ts, types.ts, adapters/, balance/)
- **Go**: No equivalent files

### B.1 Per-Model Cost Calculation

| Aspect | Go | Upstream TS |
|--------|-----|-------------|
| **Cost tracking** | **None** — only tracks `totalInputTokens` and `totalOutputTokens` (atomic.Int64) | Full cost tracking via `cost-tracker.ts` with `calculateUSDCost()` per API call (`modelCost.ts:131-142`) |
| **Model pricing tiers** | **None** | 6 pricing tiers: `$3/$15` (Sonnet), `$15/$75` (Opus 4/4.1), `$5/$25` (Opus 4.5/4.6), `$30/$150` (Opus 4.6 fast), `$0.80/$4` (Haiku 3.5), `$1/$5` (Haiku 4.5) — all per Mtok (`modelCost.ts:36-87`) |
| **Cache token costs** | **None** | Separate pricing: cache_write and cache_read per model tier (e.g., Sonnet: $3.75/Mtok write, $0.30/Mtok read) |
| **Web search costs** | **None** | $0.01 per request across all tiers |
| **Fast mode pricing** | **None** | Opus 4.6 fast = `$30/$150` per Mtok (6x base price) (`modelCost.ts:63-69`) |
| **Unknown model handling** | **None** | Falls back to `$5/$25` tier, logs `tengu_unknown_model_cost` analytics event (`modelCost.ts:89, 156-173`) |

**Gap**: Go has zero cost tracking. Upstream calculates USD costs per API call with per-model pricing, cache token economics, web search costs, and fast mode premium. The `formatTotalCost()` output shows per-model usage breakdown with cost in dollars.

### B.2 Cost Persistence and Session Resume

| Aspect | Go | Upstream TS |
|--------|-----|-------------|
| **Session cost persistence** | **None** | `saveCurrentSessionCosts()` persists to `.claude/config.json`: totalCostUSD, per-model usage (inputTokens, outputTokens, cacheRead, cacheWrite, webSearch, costUSD), API duration, lines changed (`cost-tracker.ts:143-175`) |
| **Session cost restore** | **None** | `restoreCostStateForSession()` reads from project config, only restores if `lastSessionId` matches current session (`cost-tracker.ts:130-137`) |
| **Cost display** | Basic token counter output | `formatTotalCost()`: shows total cost, API duration, wall duration, lines changed, per-model usage with cost (`cost-tracker.ts:228-244`) |
| **Advisor cost** | **None** | `getAdvisorUsage()` extracts advisor tool token usage from response, calculates separate cost, logs `tengu_advisor_tool_token_usage` event (`cost-tracker.ts:304-321`) |

**Gap**: Go loses cost data on session exit. Upstream persists costs across sessions, restores on resume, and provides rich per-model cost breakdowns including advisor tool costs.

### B.3 Provider Usage / Balance Tracking

| Aspect | Go | Upstream TS |
|--------|-----|-------------|
| **Usage bucket model** | `RateLimitState` with 4 buckets (requests/min, requests/hr, tokens/min, tokens/hr) — rate-limit only | `ProviderUsage` with typed buckets: `session` (5hr), `weekly` (7-day), `requests` (RPM), `tokens` (TPM), `throttle`, `custom` — quota tracking (`providerUsage/types.ts:9-22`) |
| **Balance tracking** | **None** | `ProviderBalance` with currency, remaining, total, updatedAt — tracks account balance (`providerUsage/types.ts:24-29`) |
| **Per-provider adapters** | **None** — single `ParseRateLimitHeaders()` for x-ratelimit-* headers | `ProviderUsageAdapter` interface with per-provider `parseHeaders()`: Anthropic, OpenAI, Bedrock adapters (`providerUsage/types.ts:37-40`) |
| **Balance poller** | **None** | `balance/poller.ts` — periodic balance fetching with configurable interval |

**Gap**: Go's `RateLimitState` is purely about rate limiting and retry timing. Upstream's `providerUsage` system is about quota and balance tracking — a fundamentally different purpose (cost awareness vs. retry scheduling).

---

### C. Tool Choice Parameter

### Files Compared
- **Upstream**: `src/services/api/claude.ts` (line 1769), `src/utils/sideQuery.ts` (line 52)
- **Go**: Not implemented

### C.1 Tool Choice in API Calls

| Aspect | Go | Upstream TS |
|--------|-----|-------------|
| **Main loop** | **No** — never sends `tool_choice` parameter | **Yes** — `tool_choice: options.toolChoice` passed to API (`claude.ts:1769`) |
| **Side queries** | **No** | **Yes** — `sideQuery()` accepts `tool_choice` parameter, forwarded to API (`sideQuery.ts:52, 237`). Used for forced tool output via `{ type: 'tool', name: 'x' }` |
| **Classifier** | **No** | Uses `{ type: 'tool', name: 'classify' }` to force the classifier tool call |
| **Compact** | **No** | Uses `tool_choice` to force specific output formats |

**Gap**: Go never sends the `tool_choice` parameter to the API. Upstream uses it in side queries (classifier, compact, permission explainer) to force specific tool use outputs. This means Go relies entirely on the model's default tool choice behavior, which may be less deterministic for structured outputs like classifier decisions.

---

### D. Context Window Per Model

### Files Compared
- **Upstream**: `src/utils/context.ts` (200+ lines), `src/utils/model/modelCapabilities.ts` (122 lines), `src/utils/model/antModels.ts` (64 lines)
- **Go**: `compact.go` (line 1138: `modelContextWindow()`), `agent_loop.go`

### D.1 Context Window Resolution

| Aspect | Go | Upstream TS |
|--------|-----|-------------|
| **Default window** | 200K for all models | `MODEL_CONTEXT_WINDOW_DEFAULT = 200_000` for all models (`context.ts:10`) |
| **1M context** | Supported via `[1m]` suffix parsing in `modelContextWindow()` + Sonnet-4/Opus-4 detection | Supported via `[1m]` suffix + `modelSupports1M()` for Sonnet 4, Opus 4.6, Opus 4.7 + `CONTEXT_1M_BETA_HEADER` + GrowthBook experiment treatment for Sonnet 4.6 (`context.ts:44-53, 90-96`) |
| **Dynamic API-based** | **No** — hardcoded | **Yes** — `refreshModelCapabilities()` fetches `/v1/models` API, caches `max_input_tokens` per model to `~/.claude/cache/model-capabilities.json` (`modelCapabilities.ts:78-121`) |
| **Env override** | **No** | `CLAUDE_CODE_MAX_CONTEXT_TOKENS` env var for ant users (`context.ts:64-72`) |
| **1M disable** | **No** | `CLAUDE_CODE_DISABLE_1M_CONTEXT` for C4E/HIPAA compliance (`context.ts:32-34`) |
| **Ant model override** | **No** | `resolveAntModel()` returns per-model `contextWindow` from GrowthBook remote config (`antModels.ts:51-63`) |

### D.2 Context Window Sizes (Upstream)

| Model | Default Context | 1M Capable |
|-------|----------------|------------|
| Haiku 3.5 | 200K | No |
| Haiku 4.5 | 200K | No |
| Sonnet 3.5 v2 | 200K | No |
| Sonnet 3.7 | 200K | No |
| Sonnet 4 | 200K | **Yes** (via `[1m]` suffix or beta header) |
| Sonnet 4.5 | 200K | No |
| Sonnet 4.6 | 200K | **Yes** (via GrowthBook experiment) |
| Opus 4 | 200K | No |
| Opus 4.1 | 200K | No |
| Opus 4.5 | 200K | No |
| Opus 4.6 | 200K | **Yes** (via `[1m]` suffix) |
| Opus 4.7 | 200K | **Yes** (via `[1m]` suffix) |

### D.3 Max Output Tokens Per Model (Upstream)

| Model | Default | Upper Limit |
|-------|---------|-------------|
| All models | 32K | 64K |
| Opus 4.7 | 64K | 64K |
| Slot-reservation cap | 8K | Escalates to 64K on hit |

**Gap**: Go's context window detection is hardcoded with `[1m]` suffix parsing. Upstream dynamically fetches model capabilities from the API, supports 1M context via multiple mechanisms (suffix, beta header, GrowthBook experiment), and has per-model max output token limits. Go misses the dynamic capabilities API, the compliance disable switch, and the GrowthBook experiment treatment.

---

### E. Daemon Mode

### Files Compared
- **Upstream**: `src/daemon/main.ts`, `src/daemon/state.ts`, `src/daemon/workerRegistry.ts`, `src/commands/daemon/daemon.tsx`, `src/cli/bg.ts`
- **Go**: No equivalent

### E.1 Daemon Architecture

| Aspect | Go | Upstream TS |
|--------|-----|-------------|
| **Daemon process** | **None** | Supervisor process (`daemonMain()`) that manages long-running workers (`daemon/main.ts:52-98`) |
| **Worker types** | **None** | `remoteControl` worker — runs headless bridge loop for remote sessions (`workerRegistry.ts:26-41`) |
| **Crash recovery** | **None** | Exponential backoff: `BACKOFF_INITIAL_MS=2000`, `BACKOFF_CAP_MS=120000`, `BACKOFF_MULTIPLIER=2`. Parks after `MAX_RAPID_FAILURES=5` (`daemon/main.ts:21-24`) |
| **Worker env** | **None** | `DAEMON_WORKER_DIR`, `DAEMON_WORKER_NAME`, `DAEMON_WORKER_SPAWN_MODE`, `DAEMON_WORKER_CAPACITY`, `DAEMON_WORKER_PERMISSION`, `DAEMON_WORKER_SANDBOX`, `DAEMON_WORKER_TIMEOUT_MS`, `DAEMON_WORKER_CREATE_SESSION` (`workerRegistry.ts:47-56`) |
| **Daemon state** | **None** | `writeDaemonState()`, `removeDaemonState()`, `queryDaemonStatus()`, `stopDaemonByPid()` — persisted PID and state file (`daemon/state.ts`) |
| **REPL command** | **None** | `/daemon` slash command with subcommands: `status`, `start`, `stop`, `bg`, `attach`, `logs`, `kill` (`commands/daemon/daemon.tsx:12-57`) |
| **Background sessions** | Background tasks via `backgroundTasks` map in exec_tool | `bg.ts` with `handleBgStart()`, `attachHandler()` — persistent background Claude sessions managed by daemon supervisor |

**Gap**: Go has no daemon mode. Upstream has a full daemon supervisor that manages long-running workers (currently remote control), with crash recovery, state persistence, and background session management. This is a significant infrastructure feature for headless/remote operation.

---

### F. SSH Remote Execution

### Files Compared
- **Upstream**: `src/ssh/createSSHSession.ts`, `src/ssh/SSHSessionManager.ts`, `src/hooks/useSSHSession.ts`
- **Go**: No equivalent

### F.1 SSH Session Support

| Aspect | Go | Upstream TS |
|--------|-----|-------------|
| **SSH sessions** | **None** | **Stub only** — `createSSHSession` and `createLocalSSHSession` throw `SSHSessionError('SSH sessions are not supported in this build')` (`createSSHSession.ts:24-29`) |
| **SSH session manager** | **None** | Interface defined: `SSHSessionManager` with `connect()`, `disconnect()`, `sendMessage()`, `sendInterrupt()`, `respondToPermissionRequest()` (`SSHSessionManager.ts:24-30`) |
| **Permission relay** | **None** | `SSHPermissionRequest` type: tool_name, tool_use_id, description, permission_suggestions, blocked_path, input (`SSHSessionManager.ts:15-22`) |
| **REPL hook** | **None** | `useSSHSession.ts` React hook for SSH session management in the TUI |
| **Implementation status** | Not applicable | **Stub** — not implemented in any available build. The types and interfaces exist but throw errors. |

**Gap**: SSH remote execution is **not implemented** in either version. Upstream has stub types/interfaces but throws "not supported" errors. This appears to be a planned but unshipped feature. Go has no stubs or interfaces for it.

---

### G. Vim Mode

### Files Compared
- **Upstream**: `src/vim/types.ts` (200 lines), `src/vim/transitions.ts`, `src/vim/motions.ts`, `src/vim/operators.ts`, `src/vim/textObjects.ts`, `src/commands/vim/vim.ts`, `src/hooks/useVimInput.ts`, `src/components/VimTextInput.tsx`
- **Go**: No equivalent

### G.1 Vim Keybinding Support

| Aspect | Go | Upstream TS |
|--------|-----|-------------|
| **Vim mode** | **None** — Go is headless CLI with no TUI input | Full vim emulation in TUI: INSERT/NORMAL modes, operators (d/c/y), motions (w/b/e/$/^), text objects (iw/aw/i"/a"), find (f/F/t/T), count prefix, dot-repeat, registers (`vim/types.ts:1-200`) |
| **Toggle** | Not applicable | `/vim` slash command toggles between `vim` and `normal` editor modes, persisted to global config (`commands/vim/vim.ts:8-38`) |
| **State machine** | Not applicable | Formal state machine: `VimState = INSERT | NORMAL`, with `CommandState` subtypes: idle → count → operator → operatorCount → operatorFind → operatorTextObj, plus find, g, replace, indent (`vim/types.ts:49-76`) |
| **Persistent state** | Not applicable | `PersistentState` with `lastChange` (for dot-repeat), `lastFind`, `register`, `registerIsLinewise` (`vim/types.ts:81-86`) |
| **Text objects** | Not applicable | Word/WORD (w/W), quotes ("/'/`), parens/brackets/braces/angle brackets (`vim/types.ts:164-180`) |

**Gap**: Go has no vim mode because it's a headless CLI without interactive text input. Upstream has a comprehensive vim emulation system for its Ink/React TUI. This gap is **architectural** — Go would need a TUI layer before vim keybindings make sense.

---

### H. Voice Input

### Files Compared
- **Upstream**: `src/voice/voiceModeEnabled.ts` (55 lines), `src/services/voice.ts`, `src/services/voiceStreamSTT.ts`, `src/services/voiceKeyterms.ts`, `src/commands/voice/voice.ts`, `src/hooks/useVoice.ts`, `src/hooks/useVoiceIntegration.tsx`, `src/context/voice.tsx`
- **Go**: No equivalent

### H.1 Voice Input System

| Aspect | Go | Upstream TS |
|--------|-----|-------------|
| **Voice mode** | **None** | Full voice input: push-to-talk recording, streaming STT via `voice_stream` endpoint on claude.ai (`services/voice.ts`, `services/voiceStreamSTT.ts`) |
| **Auth requirement** | Not applicable | Requires Anthropic OAuth (not API keys, Bedrock, Vertex, or Foundry) (`voiceModeEnabled.ts:32-44`) |
| **GrowthBook gate** | Not applicable | `tengu_amber_quartz_disabled` kill-switch. Default: enabled for OAuth users (`voiceModeEnabled.ts:16-23`) |
| **Audio capture** | Not applicable | Native audio via `audio-capture-napi` (CoreAudio on macOS, ALSA on Linux), falls back to SoX `rec` or `arecord` (`services/voice.ts:1-60`) |
| **Recording config** | Not applicable | 16kHz sample rate, 1 channel, silence detection (2.0s duration, 3% threshold) (`services/voice.ts:40-46`) |
| **Toggle** | Not applicable | `/voice` slash command toggles voice mode on/off |
| **React integration** | Not applicable | `VoiceIndicator` component, `VoiceModeNotice` in status bar, `useVoiceIntegration` hook |

**Gap**: Go has no voice input. Upstream has a complete voice system with native audio capture, streaming STT, and deep TUI integration. This is a **TUI-only feature** that requires both OAuth authentication and a graphical interface, neither of which Go has.

---

### I. Model-Specific Behavior

### Files Compared
- **Upstream**: `src/utils/context.ts`, `src/utils/model/model.ts`, `src/utils/model/antModels.ts`, `src/utils/modelCost.ts`, `src/utils/fastMode.ts`, `src/utils/effort.ts`, `src/utils/betas.ts`
- **Go**: `agent_loop.go`, `compact.go`, `auto_classifier.go`

### I.1 Model-Dependent Behavior Differences

| Behavior | Go | Upstream TS |
|----------|-----|-------------|
| **Default model per subscription** | **No** — single configurable model | Max/TeamPremium → Opus 4.7[1m]; Team Standard/Pro/Enterprise → Sonnet 4.6; ant users → GrowthBook-configured default (`model.ts:223-240`) |
| **Plan mode model switch** | Plan mode managed by `EnterPlanModeTool`/`ExitPlanModeTool`, no model change | `opusplan` alias: uses Opus in plan mode, Sonnet otherwise. `haiku` in plan mode → Sonnet (`model.ts:197-211`) |
| **Fast mode (speed parameter)** | **No** | Opus 4.6 fast mode: `speed: 'fast'` in API request, 6x pricing, org-level availability check, cooldown on 429/529 (`fastMode.ts`) |
| **Effort level** | **No** | `effortLevel` per model: controls `budget_tokens` for thinking. Per-model default in `AntModel.defaultEffortValue` (`antModels.ts:9-10`) |
| **Always-on thinking** | **No** — thinking is always enabled/disabled globally | `AntModel.alwaysOnThinking` — some models reject `thinking: { type: 'disabled' }` and must use adaptive thinking (`antModels.ts:14-15`) |
| **1M context eligibility** | Detected via `[1m]` suffix + model name matching | Same + `CONTEXT_1M_BETA_HEADER` + GrowthBook `coral_reef_sonnet` experiment + dynamic API capabilities (`context.ts:56-103`) |
| **Max output tokens per model** | Global `MaxOutputTokens` + `EscalatedMaxOutputTokens` | Per-model: Opus 4.7 → 64K default; others → 32K default; slot-reservation cap → 8K with escalation to 64K (`context.ts:160-179`) |
| **Cost per model** | **No** cost tracking | Per-model pricing tier with separate cache write/read costs and fast mode multiplier (`modelCost.ts:104-126`) |
| **Advisor model** | **No** advisor system | `advisorModel` setting — separate model for advisor tool responses, tracked with separate cost calculation |
| **Betas per model** | **No** beta headers | `getBetasForModel()` — different beta headers per model capability (1M context, output-128k, etc.) (`betas.ts`) |
| **Model deprecation** | **No** | `src/utils/model/deprecation.ts` — warns on deprecated models, suggests migration paths |
| **Thinking budget per model** | Single `thinkingBudget` config | Per-model `budget_tokens` derived from effort level and model capability |

**Gap**: Go treats all models uniformly. Upstream has extensive model-specific behavior: different defaults per subscription tier, plan-mode model switching, fast mode with per-model support, effort levels, always-on thinking, 1M context eligibility, per-model max output tokens, per-model cost tiers, advisor models, model-specific beta headers, and deprecation warnings. This is a **systematic gap** — Go's model handling is one-size-fits-all.

---

### J. Summary: Gap Analysis Severity Assessment

| Gap | Severity | Description |
|-----|----------|-------------|
| **Model provider routing** | CRITICAL | Go supports only Anthropic first-party API. No Bedrock, Vertex, OpenAI, Gemini, Grok. Fundamental architectural gap. |
| **Cost tracking** | HIGH | Go has zero cost tracking. Upstream calculates per-model USD costs with cache token economics. Users cannot monitor spend. |
| **Tool choice parameter** | MEDIUM | Go never sends `tool_choice` to API. Upstream uses it for deterministic classifier/compact outputs. Less reliable structured outputs. |
| **Context window per model** | MEDIUM | Go uses hardcoded 200K+[1m]. Upstream dynamically fetches from `/v1/models` API, supports beta headers, GrowthBook experiments, compliance disable. |
| **Model-specific behavior** | MEDIUM | Go treats all models uniformly. Upstream has subscription-dependent defaults, plan-mode model switching, per-model effort levels, always-on thinking, per-model max tokens. |
| **Daemon mode** | LOW | Go has no daemon. Upstream has supervisor with crash recovery. Niche feature for headless/remote operation. |
| **Vim mode** | LOW | Go has no TUI. Vim mode is TUI-only. Architectural gap, not a missing feature. |
| **Voice input** | LOW | Go has no voice. Requires OAuth + TUI. Architectural gap. |
| **SSH remote** | NONE | Not implemented in either version. Upstream has stubs only. Not a real gap. |
| **Fast mode** | MEDIUM | Go has no fast mode. Upstream has full fast mode with org gating, cooldown, overage, 6x pricing. Affects Opus 4.6 users. |
| **Poor mode** | LOW | Go has no poor mode. Upstream skips extract_memories/prompt_suggestion. Simple optimization, easy to add. |

---


---

## Final Comprehensive Gap Check — 2026-05-12

This section documents 18 critical topics verified against the upstream codebase. Each entry records whether it was already covered in the diff, the current state of research, and any newly discovered details.

### Topic Coverage Summary

| # | Topic | Already Covered? | Section Reference | Status |
|---|-------|-----------------|-------------------|--------|
| a | Structured Tool Output | Yes | Line 548, 3153, MCP structuredContent refs | Confirmed |
| b | Tool Choice API Parameter | Yes | Line 5705-5710, 5877 | Confirmed |
| c | Team/Cloud Memory | Partially | Line 5069, 7747 | **APPENDED below** |
| d | Plugin System | Partially | Line 1987-1989 | **APPENDED below** |
| e | Marketplace/Store | Partially | Line 1987-1989 | **APPENDED below** |
| f | Attribution | Partially | Line 233, 321, 6201, 6231 | **APPENDED below** |
| g | Worktree Support | Yes | Line 350, 1993, 4725, 3305, 6329 | Confirmed |
| h | Chrome Extension Integration | Partially | Line 404, 4218 | **APPENDED below** |
| i | Computer Use | Partially | Line 4218, 4252, 4266, 4292 | **APPENDED below** |
| j | Remote Sessions (CCR) | Yes | Line 526, 3306, 5771, 6390 | Confirmed |
| k | Audio/Voice | Yes | Line 5822-5840 | Confirmed |
| l | Vim Mode | Yes | Line 5802-5818 | Confirmed |
| m | TUI/REPL | Yes | Line 4218, 4369, 32, 35 | Confirmed |
| n | Keybindings | Partially | Line 5808 (vim only) | **APPENDED below** |
| o | History/REPL completion | Partially | Line 6412 | **APPENDED below** |
| p | Cost Limits | Yes | Line 4330, 5655 | Confirmed |
| q | Sandbox | Yes | Line 1720-1722, 2002-2004, 7943, 7954 | Confirmed |
| r | MCP Discovery | Yes | Line 391, 435-448, 586 | Confirmed |

---

### c. Team/Cloud Memory — Detailed Analysis

**Already covered**: Line 5069 mentions `feature('TEAMMEM')` and `isSessionMemoryEmpty()` with team memory extraction. Line 7747 mentions `checkTeamMemSecrets()`.

**Upstream** (`E:\Git\claude-code-upstream\src\memdir\` and `E:\Git\claude-code-upstream\src\utils\teamMemoryOps.ts`):

| Aspect | Go | Upstream TS |
|--------|-----|-------------|
| Team memory path | **No** | `<memoryBase>/projects/<sanitized-project-root>/memory/team/` (`teamMemPaths.ts:84-86`) |
| Team memory entrypoint | **No** | `MEMORY.md` file at `<teamMemPath>/MEMORY.md` (`teamMemPaths.ts:92-94`) |
| Path containment check | **No** | Two-pass: `path.resolve()` for string-level containment, then `realpath()` on deepest existing ancestor to detect symlink escapes (`teamMemPaths.ts:228-256`) |
| Symlink escape detection | **No** | `realpathDeepestExisting()` walks up from non-existing path until realpath succeeds, checking each ancestor with `lstat()` for dangling symlinks (`teamMemPaths.ts:109-171`) |
| Null byte injection | **No** | `sanitizePathKey()` rejects `\0`, URL-encoded traversals, Unicode normalization attacks (NFKC), backslashes, absolute paths (`teamMemPaths.ts:22-63`) |
| Write validation | **No** | `validateTeamMemWritePath()` rejects paths escaping team dir or escaping via symlink (`teamMemPaths.ts:228-256`) |
| Key validation | **No** | `validateTeamMemKey()` validates relative path keys from server-side (`teamMemPaths.ts:265-283`) |
| Feature gate | **No** | `isTeamMemoryEnabled()` checks `isAutoMemoryEnabled()` AND `tengu_herring_clock` GrowthBook flag (`teamMemPaths.ts:73-78`) |
| Team memory ops | **No** | `isTeamMemorySearch()`, `isTeamMemoryWriteOrEdit()`, `appendTeamMemorySummaryParts()` (`teamMemoryOps.ts`) |
| Memory file detection | **No** | `isTeamMemFile()` combines `isTeamMemoryEnabled()` + `isTeamMemPath()` (`teamMemPaths.ts:290-292`) |
| Memory types | **No** | `memoryTypes.ts` defines memory entry schemas |
| Memory age tracking | **No** | `memoryAge.ts` for recency-based memory prioritization |
| Memory scan | **No** | `memoryScan.ts` for searching/scanning memory entries |
| Team memory prompts | **No** | `teamMemPrompts.ts` with LLM prompts for team memory operations |
| Shape telemetry | **No** | `memoryShapeTelemetry.ts` for tracking memory usage patterns |

**Finding**: Go has no team memory system at all. Upstream has a complete team memory subsystem with a separate directory hierarchy under the auto-memory path, comprehensive security validation (path traversal, symlink escape, Unicode normalization attacks), GrowthBook feature gating, and integration with the session memory system for team-wide context sharing.

---

### d. Plugin System — Detailed Analysis

**Already covered**: Line 1987-1989 briefly mentions `enabledPlugins`, `extraKnownMarketplaces`, `strictKnownMarketplaces`, `blockedMarketplaces`, `pluginConfigs`.

**Upstream** (`E:\Git\claude-code-upstream\src\plugins\`):

| Aspect | Go | Upstream TS |
|--------|-----|-------------|
| Plugin registry | **No** | `BuiltinPluginDefinition` with `name`, `description`, `version`, `defaultEnabled`, `mcpServers`, `skills`, `hooks` (`builtinPlugins.ts`) |
| Plugin ID format | **No** | `{name}@{marketplace}` — e.g., `formatter@anthropic-tools`, `weixin@builtin` (`builtinPlugins.ts:12-13`) |
| Built-in plugins | **No** | `registerBuiltinPlugin()` API; plugins ship with CLI, appear in `/plugin` UI, users can enable/disable (`builtinPlugins.ts`) |
| Plugin components | **No** | Plugins can provide: skills, hooks, MCP servers, commands, settings customizations |
| Plugin enablement | **No** | `getBuiltinPlugins()` returns `{ enabled, disabled }` based on user settings with `defaultEnabled` fallback (`builtinPlugins.ts:57-60`) |
| Plugin-only policy | **No** | `isRestrictedToPluginOnly()` locks skills/hooks/skills/mcp to plugin sources only, blocking user-authored files (`pluginOnlyPolicy.ts`) |
| Policy bypass | **No** | Plugin, policySettings, built-in sources always bypass plugin-only policy; user-authored sources blocked when policy active |
| Weixin plugin | **No** | `weixin.ts` — WeChat channel integration, provides MCP server via `claude weixin serve` (`plugins/bundled/weixin.ts`) |
| Plugin hooks | **No** | `PluginHookMatcher` variant in hooks system (`hooks.ts`), hooks provided by plugins |
| Plugin dialog keybindings | **No** | `context: 'Plugin'` — `space: 'plugin:toggle'`, `i: 'plugin:install'` (`defaultBindings.ts:343-348`) |
| Plugin cache exclusion | **No** | `getGlobExclusionsForPluginCache()` excludes plugin directories from glob operations |

**Finding**: Go has no plugin system. Upstream has a full plugin architecture with built-in plugins (shipped with CLI, toggleable in `/plugin` UI), marketplace plugins (downloaded from external sources), plugin-only policy for enterprise lockdown, and plugins that can provide multiple components (skills, hooks, MCP servers).

---

### e. Marketplace/Store — Detailed Analysis

**Already covered**: Line 1987-1989 mentions marketplace settings. No deep comparison exists.

**Upstream** (`E:\Git\claude-code-upstream\src\utils\plugins\schemas.ts` and settings types):

| Aspect | Go | Upstream TS |
|--------|-----|-------------|
| Marketplace definition | **No** | `MarketplaceSourceSchema` — github/git/url sources with authentication (`schemas.ts`) |
| Official marketplace names | **No** | 9 reserved names: `claude-code-marketplace`, `claude-code-plugins`, `claude-plugins-official`, `anthropic-marketplace`, `anthropic-plugins`, `agent-skills`, `life-sciences`, `knowledge-work-plugins` (`schemas.ts:19-28`) |
| Impersonation protection | **No** | `BLOCKED_OFFICIAL_NAME_PATTERN` regex blocks "official-*", "claude-*", "anthropic-*" variations (`schemas.ts:71-72`) |
| Homograph attack protection | **No** | Non-ASCII character detection regex blocks Unicode lookalikes (`schemas.ts:79`) |
| Extra known marketplaces | **No** | `extraKnownMarketplaces` in settings — add custom marketplace sources per repository (`types.ts:576-606`) |
| Strict marketplace allowlist | **No** | `strictKnownMarketplaces` — enterprise policy, ONLY these sources can be added, check BEFORE download (`types.ts:609-618`) |
| Marketplace blocklist | **No** | `blockedMarketplaces` — enterprise policy, blocks specific sources before download (`types.ts:621-628`) |
| Auto-update control | **No** | Per-marketplace `autoUpdate` flag; official marketplaces auto-update by default (except `knowledge-work-plugins`) (`schemas.ts:48-57`) |
| Plugin install flow | **No** | Plugin dialog with install/enable/disable, trust warning, version constraints |
| Marketplace trust warning | **No** | `pluginTrustWarningMessage` — customizable admin message shown before installation (`types.ts:1079-1082`) |
| Plugin configuration | **No** | `pluginConfigs` — per-plugin MCP server user configs, keyed by `plugin@marketplace` format (`types.ts:803-806`) |
| Channel plugin allowlist | **No** | `allowedChannelPlugins` — array of `{ marketplace, plugin }` for channel-based plugin access (`types.ts:921-926`) |

**Finding**: Go has no marketplace or store system. Upstream has a complete plugin marketplace architecture with official marketplace name reservations, impersonation and homograph attack protection, enterprise policy controls (strict allowlists, blocklists), auto-update management, and trust warnings.

---

### f. Attribution — Detailed Analysis

**Already covered**: Line 233, 321 mention attribution header. Line 6201, 6219, 6231 mention `attribution-snapshot` transcript entry type. Line 6500 mentions worktree and attribution state restoration.

**Upstream** (`E:\Git\claude-code-upstream\src\utils\commitAttribution.ts` — 963 lines):

| Aspect | Go | Upstream TS |
|--------|-----|-------------|
| File state tracking | **No** | `AttributionState` with `fileStates: Map<relPath, FileAttributionState>`, `sessionBaselines`, `surface`, `startingHeadSha`, `promptCount` (`commitAttribution.ts:174-193`) |
| Character contribution | **No** | `computeFileModificationState()` — uses common prefix/suffix matching to find exact change region, tracks `claudeContribution` chars (`commitAttribution.ts:326-381`) |
| File modification tracking | **No** | `trackFileModification()` — called after Edit/Write, updates `claudeContribution` cumulatively (`commitAttribution.ts:403-434`) |
| File creation tracking | **No** | `trackFileCreation()` — tracks new files as modification from empty content (`commitAttribution.ts:440-448`) |
| File deletion tracking | **No** | `trackFileDeletion()` — tracks deleted files, contribution = old content length (`commitAttribution.ts:454-481`) |
| Bulk diff processing | **No** | `trackBulkFileChanges()` — O(n) single Map copy for large diffs (avoiding O(n^2) copy per file) (`commitAttribution.ts:490-543`) |
| Commit attribution | **No** | `calculateCommitAttribution()` — computes Claude% vs Human% for staged files, produces `AttributionData` with file-level breakdowns (`commitAttribution.ts:549-744`) |
| Generated file exclusion | **No** | `isGeneratedFile()` check — excludes build artifacts, lockfiles, etc. from attribution (`commitAttribution.ts:621-623`) |
| Surface key | **No** | `buildSurfaceKey(surface, model)` — format `"cli/claude-sonnet"` for per-surface attribution (`commitAttribution.ts:238-240`) |
| Internal model repo detection | **No** | `isInternalModelRepo()` — allowlist of 36+ private Anthropic repos where internal model names are allowed in git trailers (`commitAttribution.ts:30-75`) |
| Model name sanitization | **No** | `sanitizeModelName()` — maps internal variants to public names (e.g., `opus-4-7` -> `claude-opus-4-7`) (`commitAttribution.ts:154-169`) |
| Surface key sanitization | **No** | `sanitizeSurfaceKey()` — converts internal model names in surface keys to public equivalents (`commitAttribution.ts:135-147`) |
| Permission prompt tracking | **No** | `permissionPromptCount`, `escapeCount` — tracks user interaction metrics per commit (`commitAttribution.ts:187-192`) |
| Prompt count tracking | **No** | `incrementPromptCount()` — persists prompt count for steer/commit ratio calculation (`commitAttribution.ts:951-962`) |
| Snapshot persistence | **No** | `stateToSnapshotMessage()` converts attribution state to `attribution-snapshot` transcript entry (`commitAttribution.ts:873-895`) |
| Snapshot restoration | **No** | `restoreAttributionStateFromSnapshots()` — uses LAST snapshot only (not sum) to avoid quadratic growth bug (`commitAttribution.ts:900-930`) |
| Git transient state detection | **No** | `isGitTransientState()` — detects rebase-merge, rebase-apply, MERGE_HEAD, CHERRY_PICK_HEAD, BISECT_LOG (`commitAttribution.ts:844-868`) |
| Git diff size estimation | **No** | `getGitDiffSize()` — estimates chars from `git diff --cached --stat` with ~40 chars/line average (`commitAttribution.ts:752-789`) |
| Attribution trailer format | **No** | `formatAttributionTrailer()` — tree-shaken to separate module to avoid leaking internal strings in external builds (`commitAttribution.ts:838`) |
| Attribution in transcript | **No** | `attribution-snapshot` entry type in JSONL with full `fileStates` Map serialization (`types/logs.ts:208-219`) |
| Git notes storage | **No** | `AttributionData` stored in git notes JSON with `version: 1`, `summary`, `files`, `surfaceBreakdown`, `excludedGenerated`, `sessions` (`commitAttribution.ts:218-225`) |
| Entry point tracking | **No** | `getClientSurface()` reads `CLAUDE_CODE_ENTRYPOINT` env var, defaults to `cli` (`commitAttribution.ts:230-232`) |

**Finding**: Go has no code attribution system. Upstream tracks every file modification character-by-character, computing precise "Claude contributed X% of this commit" percentages. It uses common prefix/suffix matching to isolate actual change regions, tracks permission prompts and escape counts as quality metrics, persists state via transcript snapshots, stores attribution in git notes, and has sophisticated internal/external repo detection with model name sanitization to prevent leaking internal codenames in public repos.

---

### h. Chrome Extension Integration — Detailed Analysis

**Already covered**: Line 404 mentions `createLinkedTransportPair` for Chrome/Computer Use. Line 4218 mentions `--computer-use-mcp` CLI flag.

**Upstream** (`E:\Git\claude-code-upstream\packages\@ant\computer-use-mcp\`):

| Aspect | Go | Upstream TS |
|--------|-----|-------------|
| Computer-use MCP server | **No** | Full MCP server package (`packages/@ant/computer-use-mcp/src/`) with tools for screen, mouse, keyboard control |
| Chrome bridge transport | **No** | `createLinkedTransportPair` creates in-process linked transport pair between Claude Code CLI and Chrome MCP server (`client.ts:910-943`) |
| CLI flag | **No** | `--computer-use-mcp` fast path (124 lines) in `cli.tsx:124` — launches computer-use MCP server directly |
| MCP server definition | **No** | `mcpServer.ts` — stdio-based MCP server that exposes computer-use tools |
| Tool calls | **No** | `toolCalls.ts` — dispatches actions: screenshot, mouse_move, left_click, right_click, key, type, scroll, hold_key, cursor_position, etc. |
| Key blocklist | **No** | `keyBlocklist.ts` — blocks dangerous key combinations (e.g., Ctrl+Alt+Del, Cmd+Q) |
| Denied apps | **No** | `deniedApps.ts` — blocks automation of system-critical applications |
| Sentinel apps | **No** | `sentinelApps.ts` — monitors and blocks access to security-sensitive apps |
| Pixel comparison | **No** | `pixelCompare.ts` — verifies screen state changes after actions |
| Image resizing | **No** | `imageResize.ts` — downscales screenshots for model consumption |
| Sub-gates | **No** | `subGates.ts` — feature flags controlling computer-use capabilities |
| Executor | **No** | `executor.ts` — orchestrates tool call execution with permission checks |
| Coordinate modes | **No** | Supports `pixels` (absolute pixel) and `normalized_0_100` (percentage) coordinate conventions (`tools.ts:20-29`) |
| Batch actions | **No** | `computer_batch` tool supports arrays of actions in a single call (`toolCalls.ts`) |
| Teach mode | **No** | `request_teach_access` + `teach_step` tools for guided automation training |
| Session allowlist | **No** | Frontmost app must be in session allowlist or computer-use tools return error (`tools.ts:31-32`) |
| Swift backend (macOS) | **No** | `computer-use-swift` — native Swift component for macOS using AppleScript/JXA for display info, app management, screenshots (`darwin.ts`) |
| Linux backend | **No** | `computer-use-swift/src/backends/linux.ts` — Linux display/keyboard/mouse backend |
| Screenshot filtering | **No** | `screenshotFiltering: "native"` — platform-level compositor filtering vs `"none"` — all windows visible (`tools.ts:154-157`) |

**Finding**: Go has no computer-use or Chrome extension integration. Upstream has a complete computer-use system with an MCP server, Chrome MCP bridge transport, native Swift components for macOS, comprehensive safety systems (key blocklists, denied apps, sentinel apps, session allowlists), multiple coordinate conventions, batch actions, and teach mode for guided automation.

---

### i. Computer Use — Quick Reference

**Already covered**: Lines 4218, 4252, 4266, 4292 mention `.node` native modules for computer-use. The detailed analysis above in section (h) covers the full computer-use MCP server architecture. This is a **TUI/external-integration-only feature** requiring Chrome or native desktop components that Go's headless CLI cannot support.

---

### n. Keybindings System — Detailed Analysis

**Already covered**: Line 5808 covers vim mode keybindings. Full keybinding system not documented.

**Upstream** (`E:\Git\claude-code-upstream\src\keybindings\` — `defaultBindings.ts`, `schema.ts`, `useKeybinding.ts`, `types.ts`, `validate.ts`):

| Aspect | Go | Upstream TS |
|--------|-----|-------------|
| Keybinding system | **No** | Full keybinding architecture with context-aware binding resolution |
| Default bindings | **No** | `DEFAULT_BINDINGS` — 20+ context blocks: Global, Chat, Autocomplete, Settings, Confirmation, FormField, Tabs, Transcript, HistorySearch, Task, ThemePicker, Scroll, Help, Attachments, Footer, MessageSelector, DiffDialog, ModelPicker, Select, Plugin, MessageActions (`defaultBindings.ts:32-349`) |
| Context-aware resolution | **No** | Different keymaps active depending on current UI context (chat input, settings panel, transcript view, etc.) |
| User keybinding overrides | **No** | `keybindings.json` user config file overrides defaults |
| Reserved shortcuts | **No** | `ctrl+c` and `ctrl+d` are reserved (interrupt/exit), cannot be rebound (`defaultBindings.ts:36-41`) |
| Platform-specific bindings | **No** | Windows: `alt+v` for paste (ctrl+v is system paste); Linux/macOS: `ctrl+v` (`defaultBindings.ts:15`) |
| Kitty keyboard protocol | **No** | `cmd+c` for copy only fires on Kitty/WezTerm/Ghostty/iTerm2 with keyboard protocol (`defaultBindings.ts:213-219`) |
| Terminal VT mode detection | **No** | `SUPPORTS_TERMINAL_VT_MODE` — checks Node >=22.17.0/24.2.0 or Bun >=1.2.23 for reliable modifier chords on Windows (`defaultBindings.ts:21-25`) |
| Global shortcuts | **No** | `ctrl+c: interrupt`, `ctrl+d: exit`, `ctrl+l: redraw`, `ctrl+t: toggleTodos`, `ctrl+o: toggleTranscript`, `ctrl+r: historySearch` |
| Chat shortcuts | **No** | `ctrl+x ctrl+k: killAgents`, `shift+tab/meta+m: cycleMode`, `meta+p: modelPicker`, `meta+o: fastMode`, `meta+t: thinkingToggle`, `ctrl+s: stash`, `ctrl+g: externalEditor` |
| Voice shortcut | **No** | `space: voice:pushToTalk` (hold-to-talk), feature-gated by `VOICE_MODE` (`defaultBindings.ts:96`) |
| History search | **No** | `ctrl+r: next`, `enter: execute`, `tab/escape: accept` (`defaultBindings.ts:181-187`) |
| Plugin dialog | **No** | `space: plugin:toggle`, `i: plugin:install` (`defaultBindings.ts:343-348`) |
| Vim keybindings | **No** | See existing section at line 5808 — INSERT/NORMAL modes, operators, motions, text objects |
| Keybinding schema | **No** | `KeybindingBlock` type with `context` + `bindings` map, validated by `validate.ts` |

**Finding**: Go has no keybinding system because it is a headless CLI without an interactive TUI. Upstream has a comprehensive keybinding architecture with 20+ context-specific keymaps, platform-aware bindings (Windows vs macOS/Linux), terminal protocol detection (Kitty), user override support, and reserved shortcut protection.

---

### o. History/REPL Completion — Detailed Analysis

**Already covered**: Line 6412 mentions `history.jsonl`. Line 6485 mentions `removeLastFromHistory()`. No deep comparison exists.

**Upstream** (`E:\Git\claude-code-upstream\src\history.ts` — 465 lines):

| Aspect | Go | Upstream TS |
|--------|-----|-------------|
| History storage | **No** | `~/.claude/history.jsonl` — JSONL file with async buffered writes (`history.ts:115`) |
| In-memory buffer | **No** | `pendingEntries: LogEntry[]` — entries buffered in memory, flushed async to avoid blocking user input (`history.ts:281`) |
| File locking | **No** | Uses `lock()` with stale timeout (10s) and retry (3 retries, 50ms min) for concurrent process safety (`history.ts:298-314`) |
| Concurrent session handling | **No** | Current session entries yielded first, then other session entries — prevents interleaving history across concurrent sessions (`history.ts:190-217`) |
| Deduplication | **No** | `getTimestampedHistory()` deduplicates by display text, keeps newest, limited to `MAX_HISTORY_ITEMS=100` (`history.ts:162-179`) |
| Paste store integration | **No** | Large pasted content (>1024 chars) stored via hash reference in `~/.claude/paste-store/<hash>.txt`; small content stored inline (`history.ts:372-393`) |
| Paste reference expansion | **No** | `expandPastedTextRefs()` — replaces `[Pasted text #N]` placeholders with actual content from paste store (`history.ts:81-100`) |
| Reverse file reading | **No** | `readLinesReverse()` — reads history.jsonl backwards for efficient newest-first iteration (`history.ts:118`) |
| Auto-undo on interrupt | **No** | `removeLastFromHistory()` — removes most recent entry when Esc rewinds conversation before response; handles race with pending flush via `skippedTimestamps` set (`history.ts:453-464`) |
| TMUX session filtering | **No** | `CLAUDE_CODE_SKIP_PROMPT_HISTORY` env var — skips history recording in TMUX sessions spawned by Claude Code (prevents test pollution) (`history.ts:414-416`) |
| Cleanup registration | **No** | `registerCleanup()` — ensures final flush on process exit, waits for in-progress flush (`history.ts:419-430`) |
| ctrl+r search integration | **No** | `useHistorySearch.ts` React hook — connects keybinding `history:search` to interactive history search UI |
| Up-arrow history | **No** | `getHistory()` — current session entries first, then other sessions, within `MAX_HISTORY_ITEMS` window (`history.ts:190-217`) |

**Go has no history system.** Go's headless CLI mode does not maintain a command history file, paste store, or interactive history search. All of these are TUI-only features requiring an interactive prompt input.

---

### END OF GAP CHECK


10. **Windows content-based stale detection** — upstream compares file content as fallback when mtime changes on Windows (cloud sync, antivirus). Go uses timestamp-based stale detection only.

---


---

## B. Top 10 Critical Gaps

*Missing features that affect correctness, security, or API compatibility.*

| # | Gap | Severity | Why It Matters |
|---|-----|----------|----------------|
| 1 | **No OS-level sandbox** | CRITICAL | All commands run without Seatbelt (macOS), Bubblewrap (Linux), or any kernel-level isolation. Upstream enforces mandatory filesystem and network restrictions at the OS level. A compromised or misbehaving tool can read/write any file the process user can access. |
| 2 | **No network restrictions** | CRITICAL | No domain allow/deny lists, no Unix socket blocking, no MITM proxy, no network permission prompts. All outbound traffic is unrestricted — data exfiltration risk. |
| 3 | **No model fallback on overload** | CRITICAL | Go's `fallbackModel` field exists but is never used in the agent loop. On 429/529, the session simply retries the same model or dies. Upstream switches to a cheaper model and notifies the user, providing continuity. |
| 4 | **No tool result pairing enforcement** | HIGH | Go lacks `ensureToolResultPairing`. Malformed tool call/result sequences may cause API 400 errors that terminate the session. Upstream auto-repairs these sequences before sending. |
| 5 | **No streaming tool execution** | HIGH | Go collects all tool calls from a model response, then executes them as a batch. Upstream executes tools as they stream in from the model, enabling parallel streaming + execution and faster perceived latency. |
| 6 | **No redacted thinking block handling** | HIGH | API responses with redacted thinking blocks lose content. Go doesn't capture or preserve `signature_delta` for thinking blocks, which may cause context corruption on re-submission. |
| 7 | **No role alternation enforcement** | HIGH | Go doesn't merge consecutive user messages. The Anthropic API requires strict user/assistant alternation — violations cause 400 rejections. Upstream's 10+ pass normalization pipeline handles this. |
| 8 | **No MCP reconnection/auth** | HIGH | Failed MCP connections require full process restart; no OAuth for remote servers; no session expiry detection (HTTP 404 + JSON-RPC -32001). Upstream auto-reconnects with exponential backoff. |
| 9 | **No multi-provider API routing** | HIGH | Go is locked to the Anthropic first-party API only. Upstream supports 7 providers (first-party, Bedrock, Vertex, Foundry, OpenAI, Gemini, Grok) with per-provider model ID mapping and dedicated adapters. |
| 10 | **No tool result persistence / disk spillover** | MEDIUM-HIGH | On context overflow, Go truncates tool results with no recovery. Upstream persists oversized results to disk with XML tag references, allowing re-reading on demand without re-executing tools. |

---


---

## D. Simplifications Map

*Where Go intentionally simplifies upstream's complexity — acceptable trade-offs.*

| Upstream Complexity | Go Simplification | Acceptable? |
|---------------------|-------------------|-------------|
| 10+ pass message normalization pipeline | Single-pass normalization | **Borderline** — handles most cases but misses role alternation and orphaned thinking filtering |
| Multi-file compaction system (~10 files, 4000+ lines) | Monolithic `compact.go` (1100 lines) | **Yes** — functionally equivalent for basic compaction; lacks micro-compact, SM-compact, time-based MC |
| 5-state MCP connection model | 2-state (running/not running) | **Borderline** — cannot represent needs-auth or pending states |
| React Ink TUI with component tree | Pure text readline REPL | **Yes** — acceptable for headless/CLI use; TUI is a UI preference, not a correctness feature |
| 20+ hook event types | 2 hook types (PreCompact, PostCompact) | **Borderline** — hooks are an extension mechanism; lack of PreToolUse/Stop/UserPromptSubmit reduces customizability |
| Feature flag system with GrowthBook remote config | Hardcoded constants | **Yes** for self-hosted; **No** if targeting commercial feature parity |
| DAG-based transcript with parentUuid chains | Flat linear JSONL | **Yes** for single-session use; **No** if conversation branching or sub-agent transcript linking is needed |
| Async write queue with file locking for transcripts | Synchronous per-write fsync | **Yes** — trade-off: slower but crash-safe; upstream's async queue could lose data on crash |
| Per-provider model ID mapping (7 providers) | Single Anthropic provider | **Yes** for Anthropic-only use; **No** for enterprise multi-cloud |
| Sophisticated YOLO classifier with stages, fast mode, environment-aware prompts | LLM-based classifier with 3-denial fallback | **Yes** — different approach, similar outcome for basic use |
| Multi-source skill hierarchy (5 levels with policy controls) | Two-directory skill loading (builtin + workspace) | **Yes** for personal use; **No** for managed/enterprise deployments |
| OpenTelemetry full stack (20+ packages) | No telemetry | **Yes** for privacy/self-hosted; **No** for enterprise observability |

---


---

## E. Architectural Differences

*Fundamental design choices that aren't "better" or "worse" — just different.*

| Dimension | Go | Upstream TypeScript |
|-----------|-----|---------------------|
| **Language & Runtime** | Compiled Go binary, zero runtime dependencies | Node.js/Bun runtime, 140+ npm packages |
| **Deployment** | Single static binary (~10-20MB) | npm package or compiled exe (~100MB+ with embedded resources) |
| **State Management** | Mutex-guarded structs (`sync.RWMutex`, `atomic.Int64`) | React context + Ink component state + `AbortController` |
| **Streaming Model** | Blocking `Run()` — agent loop blocks until completion | Generator-based — `yield*` events in real-time, composable pipeline |
| **Error Recovery** | Escalating truncation (Truncate → AggressiveTruncate → MinimumHistory) | Compaction-based recovery (collapse drain → reactive compact → max_tokens escalation → multi-turn recovery) |
| **Retry Architecture** | Inline for-loop with manual backoff + proactive rate-limit checks | Composable `withRetry()` generator with subscriber-gated logic, model fallback, persistent mode |
| **Transcript Format** | Flat JSONL, 8 entry types, synchronous fsync | DAG-based JSONL, 19+ entry types, async write queue with parentUuid chains |
| **Hook System** | Go function callbacks, 2 event types | Shell commands + SDK callbacks + HTTP calls, 20+ event types with matcher system |
| **MCP Transport** | Hand-rolled JSON-RPC + SSE parsing | SDK-based transports (`@modelcontextprotocol/sdk`) |
| **Build System** | `go build` (single step, no post-processing) | 5-step pipeline with code patching, native module embedding, feature flag DCE |
| **Context Window** | Hardcoded string-matching (`modelContextWindow()`) | Dynamic multi-source resolution with API-fetched capabilities, remote config, beta header checks |
| **Cost Tracking** | Raw token counts only | Full USD cost per API call with model-specific pricing, session persistence, balance tracking |
| **Rate Limiting** | Proactive — `RetryDelay()` checks before sleeping | Reactive — `retry-after` header overrides backoff calculation |
| **Tool Execution** | Batch — all tools after model response completes | Streaming — tools execute as they stream in from model |

---


---

## F. Implementation Priority

*For closing the gaps, ordered by impact × (1/effort).*

| Priority | Feature | Impact | Effort | Rationale |
|----------|---------|--------|--------|-----------|
| **P0** | Tool result pairing (`ensureToolResultPairing`) | HIGH | LOW | ~200 lines; prevents API 400 crashes. Highest ROI fix. |
| **P0** | Role alternation enforcement | HIGH | LOW | ~150 lines; prevents API 400 rejections. Must merge consecutive user/assistant messages. |
| **P0** | Redacted thinking block handling | HIGH | LOW | ~100 lines; prevent context corruption on re-submission. |
| **P1** | Model fallback on 429/529 | HIGH | MEDIUM | ~300 lines; requires threading `fallbackModel` through the agent loop retry path. Major availability improvement. |
| **P1** | MCP reconnection with backoff | HIGH | MEDIUM | ~400 lines; auto-reconnect on transport failure. Current behavior (full restart) is unacceptable for production. |
| **P1** | Streaming tool execution | HIGH | HIGH | Major refactor of the agent loop; requires restructuring from batch-collect to streaming-execute model. High impact on latency. |
| **P2** | Basic sandbox (macOS Seatbelt) | HIGH | HIGH | Requires calling `sandbox-exec` binary with profile generation. Platform-specific but critical for security. |
| **P2** | Network restrictions (domain allow/deny) | HIGH | MEDIUM | ~500 lines; can be implemented at the HTTP transport layer without OS support. |
| **P2** | Tool result disk persistence | MEDIUM | MEDIUM | ~400 lines; persist oversized results to `.claude/` with XML tag references. Prevents data loss on compaction. |
| **P2** | MCP OAuth authentication | MEDIUM | MEDIUM | ~500 lines; required for remote MCP servers. Implement OAuth 2.0 flow per MCP spec. |
| **P3** | Multi-provider API routing | HIGH | VERY HIGH | Fundamental architecture change; requires provider abstraction layer, per-provider model config, and dedicated API adapters. |
| **P3** | Cost tracking with USD calculation | MEDIUM | LOW | ~300 lines; add `modelCosts` map, calculate per-call cost, display on session end. |
| **P3** | Expanded hooks (PreToolUse, Stop, UserPromptSubmit) | MEDIUM | MEDIUM | ~800 lines; implement matcher system and shell command execution. High extensibility value. |
| **P3** | Transcript DAG with parentUuid | MEDIUM | HIGH | ~600 lines; changes core transcript format, requires migration. Enables branching and sub-agent linking. |
| **P4** | Reactive compact (vs. truncation-only recovery) | MEDIUM | MEDIUM | ~500 lines; replace escalating truncation with collapse drain + reactive compact. Better quality recovery. |
| **P4** | Session memory custom template/prompt | LOW | LOW | ~100 lines; load custom template from `~/.claude/session-memory/config/`. |
| **P4** | Remote feature flags (GrowthBook) | LOW | HIGH | Requires GrowthBook SDK integration; not needed for self-hosted. |
| **P4** | OpenTelemetry integration | LOW | VERY HIGH | 20+ packages in upstream; major infrastructure commitment. Only justified for enterprise. |

---


---

## 50. Cross-Cutting Architectural Patterns

### 50.1 Concurrency Model

| # | Aspect | Go | Upstream | Type |
|---|--------|----|----------|------|
| 50.1.1 | Primary concurrency primitive | Goroutines + channels (`chan string` for notifications, buffered `chan StreamChunk, 100`) | `AsyncGenerator<...>` with `for await...of` loops | Go适配 |
| 50.1.2 | Agent loop execution | Synchronous `func (a *AgentLoop) Run()` with `for a.budget.Consume()` loop; each turn is a blocking API call | `async function* query()` / `queryLoop()` generators yielding `StreamEvent \| Message \| TombstoneMessage`; caller drives with `for await...of` | 简化 |
| 50.1.3 | Cancellation mechanism | `context.Context` with `context.WithTimeout` per API call; `atomic.Bool` for `interrupted` flag; `cancelCtx` for sub-agent kill | `AbortController` / `AbortSignal` propagated through `toolUseContext.abortController.signal`; `createChildAbortController` with `WeakRef` for memory-safe parent→child abort chains | Go适配 |
| 50.1.4 | Sub-agent concurrency | `sync.WaitGroup` (`activeSubAgents`) to track spawned goroutines; each sub-agent runs in its own `go a.runForkedAgent()` goroutine | Sub-agents run as recursive `yield* query()` calls within the same async generator; no explicit thread/goroutine per sub-agent — single event loop | 简化 |
| 50.1.5 | Streaming event distribution | `StreamBus` pub/sub: `sync.RWMutex`-protected `map[string]chan StreamChunk`; non-blocking publish (drops on full channel) | Direct `yield` in async generator; consumers subscribe by iterating. No pub/sub bus — streaming is pull-based | 简化 |
| 50.1.6 | Thread-safety pattern | `sync.Mutex` / `sync.RWMutex` on every shared struct (`ConversationContext`, `CollectHandler`, `ToolStateTracker`); `defer` unlock; `atomic.Int32/Int64/Bool` for counters | Immutability by convention: `State` is a single immutable record reassigned atomically (`state = { ... }`); React `useSyncExternalStore` for UI; `Store<T>` (observer pattern) for headless state | Go适配 |
| 50.1.7 | Timeout / stall detection | Dedicated goroutine with `time.Timer` + `stallReset` channel; `stallTimeoutMs`/`startupMs` dynamic thresholds (300s-600s) | `AbortSignal.timeout(ms)` for individual fetches; `APIConnectionTimeoutError` from SDK; no dedicated stall-detection goroutine | 简化 |
| 50.1.8 | Budget / rate limiting | `IterationBudget` with `atomic.Int32` CAS loop for lock-free `Consume()`; `atomic.Bool` for one-time `GraceCall()` | `createBudgetTracker()` / `checkTokenBudget()` in `query/tokenBudget.ts`; `taskBudget` via API beta header (`TASK_BUDGETS_BETA_HEADER`); `maxTurns` param | Go适配 |
| 50.1.9 | Retry logic | Synchronous `retryForkedCall()` / `classifyError()` with `RetryResult.Retryable` flag; explicit `context.WithTimeout` per retry attempt | `withRetry()` / `withStreamingVCR()` wrapper; `FallbackTriggeredError` for model fallback; `APIConnectionTimeoutError` auto-retry | 简化 |
| 50.1.10 | Background task tracking | `taskStore *TaskStore` for bash + sub-agents; `notificationChan chan string` for async notifications | `TaskState` in `AppState.tasks`; `remoteBackgroundTaskCount` for remote daemon; WS event-sourced from `system/task_started` | 简化 |

### 50.2 State Management

| # | Aspect | Go | Upstream | Type |
|---|--------|----|----------|------|
| 50.2.1 | Conversation state | `ConversationContext` struct with `[]conversationEntry`; `sync.RWMutex` for all mutations; `BuildMessages()` converts to SDK types | `Message[]` array in `State`; `getMessagesAfterCompactBoundary()` filters; `normalizeMessagesForAPI()` converts; immutability pattern — never mutate in-place | Go适配 |
| 50.2.2 | App state (UI) | N/A — CLI only, no UI | `AppState` type via `createStore<T>()` (observer pattern); `useSyncExternalStore` for React slice subscriptions; `AppStateProvider` context provider | 缺失 |
| 50.2.3 | Global/latched state | Module-level `Config` passed by value/reference; `Config.cachedPrompt` for system prompt caching | `src/bootstrap/state.ts`: latched module-level state (`getPromptCache1hEligible()`, `setFastModeHeaderLatched()`, `getSessionId()`) — computed once, stable for session | Go适配 |
| 50.2.4 | Tool execution state | `ToolStateTracker` with epoch-based staleness (`compactionEpoch`); `sync.RWMutex`; tracks read files, search queries, conclusions | `ToolUseContext` passed through query loop; `contentReplacementState` for tool result budgeting; `AutoCompactTrackingState` for compaction tracking | Go增强 |
| 50.2.5 | Streaming state tracking | `DeltasState` enum (`none`/`text_only`/`tool_in_flight`); `CollectHandler` with `sync.Mutex` for thread-safe accumulation | `StreamingToolExecutor` class: `addTool()`, `getCompletedResults()`, `discard()` for fallback recovery; async generator yields directly | Go适配 |
| 50.2.6 | Config-as-state | `Config` struct passed through `NewAgentLoop(cfg)`; mutations during run (`cfg.PermissionMode`, `cfg.cachedPrompt`) | `SettingsJson` in `AppState.settings`; `useSettingsChange()` for file watcher propagation; `applySettingsChange()` for sync | Go适配 |
| 50.2.7 | Permission state | `PermissionGate` struct with `recentlyApproved` list, `denialCount`, `totalDenialCount`, `strippedRules` | `ToolPermissionContext` in `AppState`; `mode` field (`ask`/`auto`/`plan`/`bypass`); `isBypassPermissionsModeAvailable` | Go适配 |
| 50.2.8 | Speculation / preflight state | N/A | `SpeculationState` type (`idle`/`active`) with mutable refs (`messagesRef`, `writtenPathsRef`) for speculative execution | 缺失 |
| 50.2.9 | Session memory | `SessionMemory` struct with file-based persistence (`~/.claude/notes.json`); in-process `sync.Mutex` | Memory attachments via `getAttachmentMessages()`, `startRelevantMemoryPrefetch()`; `MemoryPrefetch` with `using` pattern for disposal | Go适配 |

### 50.3 Testing Patterns

| # | Aspect | Go | Upstream | Type |
|---|--------|----|----------|------|
| 50.3.1 | Test framework | Standard library `testing` package; `t.Errorf` / `t.Fatalf` / `t.Fatal` | `bun:test` (`describe`, `test`, `expect`, `beforeEach`, `afterEach`) | Go适配 |
| 50.3.2 | Test organization | One `_test.go` file per source file (e.g., `context_test.go` for `context.go`); flat test function naming `TestXxx` | Co-located `__tests__/*.test.ts` next to source; BDD-style `describe('feature', () => test('case', ...))` | Go适配 |
| 50.3.3 | Test isolation | Each test creates fresh structs (`DefaultConfig()`, `NewConversationContext(cfg)`); no global state reset needed | `resetStateForTests()` in `beforeEach`; `process.env` saved/restored; temp directories with `rm -rf` in `afterEach` | Go适配 |
| 50.3.4 | Table-driven tests | Common pattern: `tests := []struct{...}{...}; for _, tc := range tests { ... }` | BDD `test()` per case; `expect(...).toBe(...)` assertions | Go适配 |
| 50.3.5 | Mocking | No mocking framework; inject dependencies via struct fields (`*tools.Registry`, `*mcp.Manager`) or callback functions (`SpawnFunc`, `GetOutputFunc`) | `bun:mock` for module-level mocking; `jest.mock`-style interception; dependency injection via `QueryDeps` / `productionDeps()` | 简化 |
| 50.3.6 | Test count | ~100 `*_test.go` files covering core logic (streaming, context, config, compact, tools, permissions) | ~194 `*.test.ts`/`*.test.tsx` files covering bridge, CLI, commands, assistant, components, utils | 简化 |
| 50.3.7 | Integration tests | `combined_exec_test.go` for end-to-end tool execution; `forked_agent_test.go` for forked agent behavior | `cli/handlers/__tests__/autonomy.test.ts`, `commands/daemon/__tests__/daemon.test.ts`, `bridge/__tests__/bridgeMessaging.test.ts` for multi-module integration | 简化 |
| 50.3.8 | Snapshot testing | N/A | VCR recording via `withStreamingVCR()` for API replay tests | 缺失 |
| 50.3.9 | Feature-gated tests | N/A | `feature('TRANSCRIPT_CLASSIFIER')` / `feature('CONTEXT_COLLAPSE')` gates in test imports and test bodies | 缺失 |

### 50.4 Platform Abstraction

| # | Aspect | Go | Upstream | Type |
|---|--------|----|----------|------|
| 50.4.1 | File naming convention | Build tags: `//go:build windows` / `//go:build unix` / `//go:build !windows`; separate files `exec_tool_windows.go`, `exec_tool_unix.go`, `file_lock_unix.go` | Runtime check: `process.platform === 'darwin' \| 'win32' \| 'linux'` in `getPlatform()`; single file with conditional branches | Go适配 |
| 50.4.2 | Process group management | Unix: `syscall.Setpgid` + `syscall.Kill(-pid, SIGKILL)`; Windows: `CREATE_NEW_PROCESS_GROUP` + `taskkill /F /T /PID` | N/A — process spawning via Node.js `child_process` with `detached` option; signal handling via `process.on('SIGINT')` | Go适配 |
| 50.4.3 | File locking | Unix: no-op (`lockFileEx` returns nil — advisory locking unreliable on Unix); Windows: Windows API `LockFileEx` via `syscall` | `proper-lockfile` npm package (cross-platform); lazy-loaded to avoid startup cost | Go适配 |
| 50.4.4 | Path handling | `filepath.Join`, `filepath.Abs`, `os.PathSeparator` — Go's `path/filepath` package handles OS-specific separators automatically | `path.join`, `path.resolve` from Node.js `path` module; custom `normalizePathForConfigKey()` for config keys | Go适配 |
| 50.4.5 | Home directory resolution | `os.Getenv("USERPROFILE")` (Windows) / `os.Getenv("HOME")` (Unix) in `homeClaudeDir()` | `os.homedir()` from Node.js `os` module; `getClaudeConfigHomeDir()` for config paths | Go适配 |
| 50.4.6 | WSL detection | N/A | `getPlatform()` reads `/proc/version` checking for "microsoft" or "wsl"; `getWslVersion()` for WSL version detection | 缺失 |
| 50.4.7 | Terminal / ANSI | Direct ANSI escape codes (`\033[2m` for dim, `\033[0m` for reset) in `TerminalHandler.filterThinking()` | `@anthropic/ink` library for terminal rendering; `ThemeProvider` for color schemes | Go适配 |

### 50.5 Configuration & Environment

| # | Aspect | Go | Upstream | Type |
|---|--------|----|----------|------|
| 50.5.1 | Settings file format | `.claude/settings.json` with `ClaudeSettings` struct (env + mcp + permissions sections); `ClaudeSettings` is manually parsed with `json.Unmarshal` | `.claude/settings.json` with Zod schema (`SettingsSchema`); strict validation, `.passthrough()` for unknown fields, lazy schema loading | Go适配 |
| 50.5.2 | Config loading | `LoadConfigFromFile()` reads project-level, falls back to home-level (`~/.claude/`); MCP from `.mcp.json`; flat struct population | Multi-source merge: managed settings (MDM), drop-in files (`managed-settings.d/*.json`), plugin settings, session settings, remote managed settings; `mergeWith()` from lodash | 简化 |
| 50.5.3 | Environment variables | Direct `os.Getenv()` calls (`ANTHROPIC_API_KEY`, `ANTHROPIC_BASE_URL`, `ANTHROPIC_MODEL`); `USERPROFILE`/`HOME` for paths | `process.env` access with `isEnvTruthy()` helper; `CLAUDE_CODE_EXTRA_BODY`, `DISABLE_PROMPT_CACHING`, etc.; `getManagedFilePath()` for config directories | Go适配 |
| 50.5.4 | Default values | `DefaultConfig()` returns struct literal with all defaults inline | `getDefaultAppState()` + Zod schema `.default()` values; `DEFAULT_PROJECT_CONFIG` constant | Go适配 |
| 50.5.5 | Settings validation | No validation — `json.Unmarshal` silently ignores unknown fields; missing fields get zero values | Zod v4 schema validation with `ValidationError[]` reporting; `filterInvalidPermissionRules()`; format error with `formatZodError()` | 简化 |
| 50.5.6 | Settings caching | `cachedPrompt *CachedSystemPrompt` for system prompt; no general settings cache | `settingsCache.ts`: `getCachedParsedFile()`, `getCachedSettingsForSource()`, `getSessionSettingsCache()` with `resetSettingsCache()`; LRU for `safeParseJSON` | 简化 |
| 50.5.7 | File watching | N/A — settings loaded once at startup | `watchFile()` / `unwatchFile()` from `node:fs` for settings.json changes; `useSettingsChange()` React hook for propagation | 缺失 |
| 50.5.8 | Permission rules | `permissions.RuleStore` with `LoadRulesFromAllSources()`; rule parsing in `permissions/` package | Zod-validated `PermissionsSchema` with `allow`/`deny`/`ask` arrays; `PermissionRuleSchema()` with lazy loading; enterprise MDM/allowlist via `AllowedMcpServerEntrySchema` | Go适配 |
| 50.5.9 | MCP configuration | `MCPConfigFile` struct matching `.mcp.json` format; `mcp.Manager` for registration/start/stop | MCP servers in settings + `mcpServers` in project config; `MCPServerConnection[]` in `AppState.mcp`; connection state management | 简化 |
| 50.5.10 | Feature flags | Compile-time via build tags only; runtime feature toggling via `Config` struct booleans (`AutoClassifierEnabled`, `MicroCompactEnabled`) | `feature('FEATURE_NAME')` from `bun:bundle` for tree-shakeable conditional requires; GrowthBook for remote feature flags (`getDynamicConfig_BLOCKS_ON_INIT`) | 简化 |


---

