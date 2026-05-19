# Agent Tools

> Sub-agent, forked agent

## Sections Included
- [##] Line 2586-2616 -- ## 19. Forked Agent / Parallel API Calls (`forked_agent.go`)
- [##] Line 2752-2787 -- ## 25. Sub-Agent Orchestration (`agent_sub.go`)
- [##] Line 3811-3893 -- ## Part 2: Sub-Agent Orchestration
- [##] Line 3957-3988 -- ## 9. agent_sub.go — 子代理编排（续接第7节已有内容）
- [##] Line 7526-7690 -- ## Part 2: Agent Tool Comparison
- [###] Line 11528-11558 -- ### 54.8 Agent Tool

---

## Content

## 19. Forked Agent / Parallel API Calls (`forked_agent.go`)

**Upstream reference**: `src/utils/forkedAgent.ts`

### 19.1 Cache-safe params model
- **上游**: `CacheSafeParams` carries `systemPrompt`, `userContext`, `systemContext`, `toolUseContext`, `forkContextMessages`. The `toolUseContext` bundles tools, model, and options. Global `lastCacheSafeParams` slot for post-turn forks (forkedAgent.ts:57-68, 73-81)
- **Go版**: `CacheSafeParams` carries `SystemPrompt`, `Model`, `Tools` ([]anthropic.ToolUnionParam), `Messages` ([]anthropic.MessageParam), `ThinkingConfig`. No global slot; caller captures params explicitly via `CaptureCacheSafeParams()` (forked_agent.go:31-37, 395-402)
- **类型**: Go适配 — Go uses explicit Anthropic SDK types instead of abstract ToolUseContext; no global slot

### 19.2 Forked agent execution model
- **上游**: `runForkedAgent()` is async generator-based: iterates over `query()` yielding messages, accumulates usage from `message_delta` stream events, records sidechain transcripts, logs `tengu_fork_agent_query` analytics. Creates isolated `ToolUseContext` via `createSubagentContext()` with cloned file state cache, isolated abort controller, denial tracking (forkedAgent.ts:493-630)
- **Go版**: `RunForkedAgent()` is synchronous loop: makes direct API calls via `client.Messages.New()`, executes tools concurrently via `sync.WaitGroup`, has its own retry logic with `retryForkedCall()`. No streaming, no sidechain transcript recording, no analytics events (forked_agent.go:81-270)
- **类型**: 简化 — no streaming, no transcript recording, no analytics, no isolated subagent context

### 19.3 Subagent context isolation
- **上游**: `createSubagentContext()` creates isolated `ToolUseContext` with: cloned file state cache, new child abort controller, `shouldAvoidPermissionPrompts: true`, no-op setAppState, fresh denial tracking, fresh tool decisions, cloned content replacement state. Explicit opt-in flags for sharing parent state (forkedAgent.ts:345-466)
- **Go版**: No equivalent — `ForkedAgentConfig` only carries `CanUseTool` callback. No file state cache, no content replacement, no denial tracking, no abort controller sharing (forked_agent.go:46-58)
- **类型**: 缺失 — no subagent context isolation; forked agent shares parent's global state

### 19.4 Permission model for forked agents
- **上游**: `CanUseToolFn` called before each tool execution in fork; `createGetAppStateWithAllowedTools()` wraps parent's `getAppState` to inject allowed tools into permission context (forkedAgent.ts:147-171)
- **Go版**: `CanUseToolFn` callback in `ForkedAgentConfig` — returns `(allowed bool, reason string)` (forked_agent.go:43). Simpler but equivalent in concept
- **类型**: 简化 — basic permission callback but no allowed-tools injection into permission context

### 19.5 Forked agent retry logic
- **上游**: Retry handled by the main `query()` loop with its own retry infrastructure (shared with parent agent)
- **Go版**: Dedicated `retryForkedCall()` with exponential backoff + jitter, 3 max retries, re-classifies errors on each attempt (forked_agent.go:275-303)
- **类型**: Go增强 — self-contained retry logic independent of parent agent

---


---

## 25. Sub-Agent Orchestration (`agent_sub.go`)

**Upstream reference**: `src/constants/src/tools/AgentTool/builtInAgents.ts` + `src/hooks/src/tools/AgentTool/` + `src/components/agents/src/tools/AgentTool/`

### 25.1 Agent type system
- **上游**: Built-in agent definitions in `builtInAgents.ts`: `general-purpose`, `explorer`, `planner`, `verifier`. Each has typed system prompts, tool restrictions, and model preferences. Agent types managed via `AgentDefinition` interface
- **Go版**: `AgentType` enum: `General`, `Explore`, `Plan`, `Verify`, `Fork`. Each mapped to `agentTypeConfig` with `promptModifier`, `denyTools`, `allowTools`. Fork mode inherits parent context (agent_sub.go:42-243)
- **类型**: Go适配 — similar agent types; Go adds explicit Fork type with `forkBoilerplate` directive

### 25.2 Fork mode
- **上游**: Fork is implicit in `ForkedAgentParams` with `cacheSafeParams`; the fork shares the parent's conversation prefix for cache hits. Fork boilerplate is not a separate concept — the fork just appends new messages
- **Go版**: Explicit fork mode with `AgentTypeFork`, `forkBoilerplate` (10-rule directive prepended to user message), `forkBoilerplate` XML tag wrapping. `filterEntriesForFork()` removes last ToolUseContent, CompactBoundaryContent, AttachmentContent from cloned entries (agent_sub.go:226-272, 841-871)
- **类型**: Go增强 — explicit fork protocol with structured output format; upstream fork is implicit

### 25.3 Tool filtering layers
- **上游**: Tool restrictions per agent type defined in `builtInAgents.ts` with `restrictedToolName` arrays. `AgentTool` applies restrictions at execution time
- **Go版**: 4-layer filtering: (1) `allAgentDisallowedTools` (7 tools: agent, task_output, plan_approval, task_create, task_update, task_list, task_get, task_stop, send_message), (2) `asyncAgentDisallowedTools` (empty, extensible), (3) agent type `denyTools`, (4) caller `disallowedTools`. Plus whitelist `allowedTools` with `*` wildcard (agent_sub.go:582-641)
- **类型**: Go增强 — structured 4-layer filtering; upstream has simpler per-type restrictions

### 25.4 Model alias resolution
- **上游**: Model resolution via `parseUserSpecifiedModel()` in model utils; "inherit" not explicitly handled at agent level
- **Go版**: `resolveModelAlias()` handles: empty/"inherit" → parent model, "sonnet"/"opus"/"haiku" → env var resolution with parent-tier matching (prevents downgrade), verbatim passthrough (agent_sub.go:489-523)
- **类型**: Go增强 — explicit alias resolution with parent-tier awareness

### 25.5 Sub-agent max_tokens
- **上游**: Default max_tokens with `CAPPED_DEFAULT_MAX_TOKENS` for sub-agents, escalation to higher limits on demand
- **Go版**: Sub-agents start with `MaxOutputTokens = 8000` matching upstream's `CAPPED_DEFAULT_MAX_TOKENS`, with escalation to 64000 (agent_sub.go:567)
- **类型**: 等价 — same cap strategy

### 25.6 Sub-agent output capture
- **上游**: Sub-agent output rendered via React components; `TaskOutputTool` reads completed task output
- **Go版**: `taskOutputWriter` captures output into `AgentTask` buffer + live output file at `.claude/sub-agents/{id}_output.txt` for non-blocking parent reads (agent_sub.go:1189-1205)
- **类型**: Go适配 — file-based output capture instead of React component tree

---


---

## Part 2: Sub-Agent Orchestration

### 1. Agent Lifecycle — Spawning, Communication, Result Collection

| Go (`agent_sub.go`) | Upstream (`forkedAgent.ts`, `LocalAgentTask.tsx`, `inProcessRunner.ts`) |
|---|---|
| **SpawnSubAgent** — creates a child `AgentLoop` struct, launches a goroutine. Sync spawn blocks until completion; async spawn returns immediately with `agentId`. | **`runForkedAgent()`** — creates isolated `ToolUseContext` via `createSubagentContext()`, runs `query()` loop in same event loop. For AgentTool agents, the spawning happens via `query()` internally. |
| Child agent is a **separate struct instance** with its own HTTP client (reused parent), registry, context, transcript, compactor | Upstream uses **the same `query()` function** but with an isolated context — no separate struct, just different context params |
| Fork mode: copies parent entries via `filterEntriesForFork()`, strips last ToolUseContent + boundaries | Upstream uses `forkContextMessages` — parent messages passed as prefix to maintain cache continuity |
| Result collection: sync returns `childResult`; async uses `taskOutputWriter` + file-based output | Upstream: `for await (const message of query(...))` — streaming message yield. Results extracted via `getLastAssistantMessage()` + `extractTextContent()` |
| Agent result stored in `taskStore` (legacy) and `agentTaskStore` (new) | Upstream stores in `appState.tasks` as `LocalAgentTaskState` with `messages`, `progress`, `result`, `retrieved`, `isBackgrounded` fields |
| **Go spawns OS-level goroutines** — true parallel execution | **Upstream agents run in-process** on the same JS event loop — cooperative concurrency |

### 2. Inter-Agent Messaging (send_message_tool)

| Go | Upstream |
|---|---|
| `SendMessageToSubAgent(agentID, message)` — queues message via `task.AddPendingMessage(message)` or `taskState.AddPendingMessage(message)` | Upstream uses `pendingMessages: string[]` in `LocalAgentTaskState` — messages queued mid-turn |
| Child drains pending messages at turn boundary: `childLoop.drainPendingMessagesFunc = func() []string { return bgTask.DrainPendingMessages() }` | Upstream drains at tool-round boundaries in `runInProcessTeammate()` — checks `task.pendingMessages` between query iterations |
| **No mailbox system** — direct task-to-task message queue | Upstream has `teammateMailbox.ts` — full mailbox system for swarm teammates with permission callbacks, read receipts, idle notifications |
| Messages are plain strings | Upstream supports structured messages: permission responses, shutdown requests, DM summaries |

### 3. Agent Cancellation

| Go | Upstream |
|---|---|
| `CancelSubAgent(agentID)` — calls `task.CancelFunc()` (Go `context.WithCancel`). Async context created per sub-agent. | `createChildAbortController(parentContext.abortController)` — creates abort controller linked to parent. Parent abort propagates to child. |
| `StopBackgroundTask(taskID)` — additionally kills OS process if tracked | Upstream uses `abortController.signal.aborted` checks at multiple points in query loop |
| Dual store: kills in both `agentTaskStore.Kill()` and legacy `taskStore.KillTask()` | Upstream uses single `AbortController` — one signal, no dual tracking |
| **Go uses Go's native `context.Context` cancellation** | **Upstream uses Web `AbortController`/`AbortSignal`** |

### 4. Agent Name Registry

| Go | Upstream |
|---|---|
| `agentNameRegistry map[string]string` — maps short name (first word of prompt) to agent ID. Set in `SpawnSubAgent` via `extractAgentName()` | Upstream has `subagentName` field in `SubagentContext` — tracked via `AsyncLocalStorage` in `agentContext.ts`. Names resolved via `getSubagentLogName()` |
| `resolveAgentID(nameOrID)` — looks up name in registry, falls back to treating input as direct ID | Upstream uses agent ID format `name@team` or UUID — no separate name-to-ID mapping registry |
| Simple map-based lookup | Upstream uses `AsyncLocalStorage<AgentContext>` for context-scoped agent identity |

### 5. Agent Output Collection (notificationChan)

| Go | Upstream |
|---|---|
| `EnqueueAgentNotification(taskID, status, result, transcriptPath, outputFilePath, turnsUsed, tokensUsed, durationMs)` — enqueues to parent's notification channel | Upstream uses `setAppState()` callbacks + `updateTaskState()` to update `LocalAgentTaskState.progress` in real-time |
| Notification includes: status, result text, transcript path, output file path, turns, tokens, duration | Upstream tracks `AgentProgress`: `toolUseCount`, `tokenCount`, `lastActivity`, `recentActivities`, `summary` |
| Output file: `.claude/sub-agents/{taskID}_output.txt` — live file parent can read non-blockingly | Upstream output: `getTaskOutputPath(agentId)` — symlink-based output on disk, evicted via `evictTaskOutput()` |
| **Go uses channel-based notifications** | **Upstream uses React state updates** (setAppState pattern) + SDK event queue |

### 6. Agent Task Store — Async Task Management

| Go | Upstream |
|---|---|
| Two stores: `taskStore` (legacy `TaskStore` with `TaskState`) and `agentTaskStore` (new `AgentTaskStore` with `tools.AgentTask`) | Single store: `appState.tasks` map of `TaskStateBase` with discriminated union (`local_agent`, `local_shell`, `local_teammate`, `local_mcp`) |
| Legacy `TaskState`: ID, Prompt, Model, SubagentType, Status, Result, CancelFunc, TranscriptPath, OutputFile, StartTime, ToolsUsed, DurationMs | `LocalAgentTaskState`: type, agentId, prompt, selectedAgent, agentType, model, abortController, error, result, progress, retrieved, messages, isBackgrounded, pendingMessages, retain, diskLoaded, panelGraceDeadline |
| New `AgentTask`: ID, Description, SubagentType, Prompt, Model, Status, Output (buffer + file), CancelFunc, TranscriptPath, StartTime | Upstream has `ProgressTracker` with `toolUseCount`, `latestInputTokens`, `cumulativeOutputTokens`, `recentActivities` |
| Tasks tracked in-memory (maps) | Upstream tasks persisted via `registerTask()`, `updateTaskState()`, evicted via `evictTerminalTask()` |
| **Go has no `retain`/`diskLoaded`/`panelGraceDeadline` lifecycle fields** | Upstream has sophisticated lifecycle: `retain` (UI holding task), `diskLoaded` (sidechain JSONL loaded), `panelGraceDeadline` (hide after terminal transition) |

### 7. Resume from Async Agent

| Go | Upstream |
|---|---|
| `ResumeAsyncAgent(taskID)` — reads transcript path from task, calls `NewAgentLoopFromTranscript()` to recreate agent from JSONL transcript | Upstream resume: reconstructs from sidechain transcript via `recordSidechainTranscript()` + `ContentReplacementState` for prompt cache stability |
| Uses stored transcript file path: `.claude/transcripts/sub-agents/{sessionID}.jsonl` | Upstream uses `getAgentTranscriptPath()` — sidechain JSONL records all messages per agent |
| **No content replacement state** — resumed agent may have different prompt cache prefix | Upstream tracks `contentReplacementState` — UUID-tagged tool results so resumed agent produces identical replacements (cache hit) |
| Resume is a new `AgentLoop` with same config/registry as parent | Upstream resume uses same `runAgent()` with reconstructed context, including `contentReplacementState` override |

### Sub-Agent Orchestration — Summary Divergence Table

| # | Divergence | Impact |
|---|---|---|
| 1 | **Go uses goroutines (OS threads); upstream runs on single event loop** | Go agents are truly parallel; upstream agents are cooperative. Go can have race conditions but more throughput. |
| 2 | **Go has no mailbox system** | Missing upstream's inter-agent communication infrastructure: permission forwarding, shutdown requests, idle notifications, DM summaries. |
| 3 | **Go has no `AsyncLocalStorage`-based agent context** | Upstream isolates agent identity across async ops without parameter drilling. Go passes context explicitly. |
| 4 | **Go has no `ContentReplacementState` for resume** | Resumed agents in Go may produce different tool result replacements, causing prompt cache misses. |
| 5 | **Go has no `retain`/`diskLoaded`/`panelGraceDeadline` lifecycle** | Missing upstream's sophisticated task eviction and UI state management for background agents. |
| 6 | **Go has no `ProgressTracker` with activity tracking** | Upstream tracks recent activities, token counts, tool use counts for real-time UI progress display. Go only tracks final totals. |
| 7 | **Go has no `teammateMailbox` for swarm coordination** | Upstream supports agent swarms with leader/worker topology, permission bridging, and cross-agent messaging. |
| 8 | **Go has no `drainPendingMessages` at turn boundary** | Actually Go DOES implement this — `childLoop.drainPendingMessagesFunc` is wired. This matches upstream's pattern. |
| 9 | **Go has no `createSubagentContext()` isolation factory** | Upstream's factory isolates ALL mutable state (readFileState, abortController, getAppState, callbacks) with explicit opt-in sharing. Go manually copies config fields. |

---


---

## 9. agent_sub.go — 子代理编排（续接第7节已有内容）

### 9.1 AgentType 定义

| Go | Upstream |
|---|---|
| 4种硬编码类型：`General`, `Explore`, `Plan`, `Verify`, `Fork`（`agent_sub.go:45-51`） | 上游通过 `AgentDefinition` 接口（`@ant/model-provider`）支持用户自定义 agent，从 `agents/` 目录加载 |
| 每种 agent 的 prompt modifier 和 deny tools 硬编码在 `agentTypeConfigs` map 中（`agent_sub.go:61-242`） | 上游从 agent 定义文件中读取 `promptModifier` 和 `allowedTools`/`deniedTools` |
| Fork agent 使用 `forkBoilerplate` 指令包裹用户 prompt（`agent_sub.go:247-272`） | 上游在 `UserForkBoilerplateMessage.tsx` 中渲染类似内容 |

### 9.2 子代理隔离与配置

| Go | Upstream |
|---|---|
| `buildSubAgentConfig()` 手动复制父 config 字段，设置 `PermissionMode=auto`, `ShouldAvoidPermissionPrompts=true`（`agent_sub.go:527-569`） | 上游 `createSubagentContext()` 工厂函数隔离所有可变状态，有显式 opt-in sharing（`forkedAgent.ts:345-466`） |
| `buildSubAgentRegistry()` 4层过滤工具：global disallowed → async disallowed → type deny → explicit deny（`agent_sub.go:582-641`） | 上游在 `createSubagentContext()` 中通过 `options` 字段控制工具集 |
| 子代理 `maxOutputTokens=8000`（`agent_sub.go:567`） | 上游通过 `COMPACT_MAX_OUTPUT_TOKENS` 和 `maxOutputTokensOverride` 控制 |
| **Go 缺失：无 `CacheSafeParams`** | 上游 `CacheSafeParams` 确保 fork 子代理与父共享 prompt cache（`forkedAgent.ts:57-68`） |
| **Go 缺失：无 `queryTracking` 链** | 上游每个子代理有 `queryTracking: { chainId, depth }` 用于遥测和深度限制（`forkedAgent.ts:456-459`） |

### 9.3 子代理执行

| Go | Upstream |
|---|---|
| `SpawnSubAgent()` 启动 goroutine 运行子代理，返回 `taskID`（`agent_sub.go:289-478`） | 上游 `runForkedAgent()` 使用 `for await` query loop 在单线程 event loop 中执行（`forkedAgent.ts:493-630`） |
| 使用 `context.WithCancel()` 实现取消（`agent_sub.go:350-357`） | 上游使用 `AbortController` 层次结构（`createChildAbortController()`） |
| Fork 模式通过 `filterEntriesForFork()` 过滤父上下文条目（`agent_sub.go:841-871`） | 上游通过 `forkContextMessages` 传递共享消息 |
| `resolveModelAlias()` 支持 sonnet/opus/haiku 别名解析（`agent_sub.go:489-523`） | **上游无 model alias 机制** — 使用完整 model ID |
| **Go 子代理使用独立 `AgentLoop` 实例** | 上游子代理共享同一个 `query()` loop，通过 `queryTracking.depth` 标识嵌套 |

---


---

## Part 2: Agent Tool Comparison

### 2.1 Agent Tool Parameter Schema

| Parameter | Go (`agent_tool.go`) | Upstream (`AgentTool.tsx`) |
|-----------|---------------------|---------------------------|
| `description` (required) | Yes | Yes |
| `prompt` (required) | Yes | Yes |
| `subagent_type` | Yes (optional, defaults to general-purpose) | Yes (optional; when omitted and fork gate on, triggers fork path) |
| `model` | `enum: ["sonnet", "opus", "haiku"]` | `enum: ["sonnet", "opus", "haiku"]` |
| `run_in_background` | **DEPRECATED** -- parameter exists but is ignored; sub-agents always run in background (`agent_tool.go:84-85`) | **Active** -- controls sync vs async execution (`AgentTool.tsx:183-187`) |
| `allowed_tools` | Yes (`array<string>`, `["*"]` for all) | **MISSING** -- tool filtering done via `Agent(allowed,denied)` syntax and permission rules |
| `disallowed_tools` | Yes (`array<string>`) | **MISSING** -- tool filtering done via permission rules |
| `inherit_context` | Yes (`boolean`, default false) | **MISSING as explicit param** -- fork subagent experiment (`isForkSubagentEnabled()`) implicitly inherits full conversation when `subagent_type` is omitted |
| `max_turns` | Yes (`integer`, default 200) | Yes (via agent definition `maxTurns` or `maxTurns` override) |
| `timeout` | Yes (`integer`, max 600000ms) | **MISSING** -- no explicit timeout param on Agent tool |
| `name` | **MISSING** | Yes -- makes agent addressable via `SendMessage({to: name})` (`AgentTool.tsx:196-200`) |
| `team_name` | **MISSING** | Yes -- spawns as teammate instead of subagent |
| `mode` | **MISSING** | Yes -- permission mode for teammate (`plan`, `acceptEdits`, etc.) |
| `isolation` | **MISSING** | Yes -- `worktree` or `remote` (ant-only) (`AgentTool.tsx:218-225`) |
| `cwd` | **MISSING** | Yes -- absolute path override (`AgentTool.tsx:227-232`) |

**File references:** Go: `agent_tool.go:60-110`. Upstream: `AgentTool.tsx:166-270`.

### 2.2 Agent Lifecycle Management

| Aspect | Go | Upstream |
|--------|-----|----------|
| **Execution model** | **Always async** -- sub-agents always run in background; `run_in_background` param is deprecated and ignored (`agent_tool.go:143-144`) | **Dual mode** -- sync (foreground, blocks caller) and async (background, returns immediately). Controlled by `run_in_background`, agent `background: true`, coordinator mode, or fork gate |
| **Sync agent flow** | Not supported | Complex flow: register foreground, race message iteration vs background signal, can be backgrounded mid-flight (`AgentTool.tsx:1066-1672`) |
| **Spawn mechanism** | Callback `AgentSpawnFunc` injected into tool (`agent_tool.go:26-28`) | Direct call to `runAgent()` with full `ToolUseContext` (`AgentTool.tsx:877-913`) |
| **Foreground-to-background transition** | Not applicable (always async) | Yes -- auto-background after configurable timeout (`getAutoBackgroundMs()`, default 120s) (`AgentTool.tsx:153-161`) |
| **Teammate spawning** | Not supported | `spawnTeammate()` when `team_name` and `name` both provided (`AgentTool.tsx:441-476`) |
| **Remote agent spawning** | Not supported | `teleportToRemote()` for CCR remote sessions (`AgentTool.tsx:669-712`) |

**File references:** Go: `agent_tool.go:143-144`. Upstream: `AgentTool.tsx:183-189` (schema), `AgentTool.tsx:827-835` (shouldRunAsync), `AgentTool.tsx:1066-1672` (sync flow).

### 2.3 Model Routing

| Aspect | Go | Upstream |
|--------|-----|----------|
| **Model override** | Yes -- `model` param: "sonnet", "opus", "haiku" | Yes -- same enum |
| **Model resolution** | Passed directly to `SpawnFunc` (`agent_tool.go:126-144`) | Via `getAgentModel()` which considers agent definition model, parent model, override param, and permission mode (`AgentTool.tsx:640-646`, `E:\Git\claude-code-upstream\packages\builtin-tools\src\utils\model\agent.js`) |
| **Thinking config** | Not exposed | Fork children inherit parent's thinking config for cache hits; regular sub-agents have thinking disabled (`runAgent.ts:688-691`) |
| **Model inheritance** | Simple -- passed through or default | Complex -- `getAgentModel()` considers `permissionMode`, agent definition `model` frontmatter, parent `mainLoopModel` |

### 2.4 Sub-Agent Isolation

| Aspect | Go | Upstream |
|--------|-----|----------|
| **Worktree isolation** | Not supported | Yes -- `isolation: "worktree"` creates temporary git worktree, auto-cleans if no changes (`AgentTool.tsx:861-955`) |
| **Remote isolation** | Not supported | Yes -- `isolation: "remote"` (ant-only) delegates to CCR (`AgentTool.tsx:669-712`) |
| **Cwd override** | Not supported | Yes -- `cwd` param overrides working directory (`AgentTool.tsx:917-919`) |
| **Context isolation** | `inherit_context` param for full conversation inheritance | `createSubagentContext()` with rich isolation options: cloned `readFileState`, cloned `contentReplacementState`, isolated `getAppState`, new `abortController`, no-op mutation callbacks by default (`forkedAgent.ts:345-466`) |
| **File state cache** | Not mentioned | Cloned from parent by default (`cloneFileStateCache`) to prevent interference |
| **Permission mode** | Not configurable per-agent | Agent definition can specify `permissionMode` (e.g., `bubble`, `acceptEdits`, `plan`) |
| **Claude.md context** | Not mentioned | Explore/Plan agents get Claude.md and gitStatus stripped from system context to save tokens (`runAgent.ts:396-416`) |
| **MCP servers** | Not mentioned | Agents can define additional MCP servers in frontmatter (`runAgent.ts:101-224`) |

**File references:** Go: `agent_tool.go:96-99` (inherit_context). Upstream: `forkedAgent.ts:345-466` (createSubagentContext), `runAgent.ts:101-224` (MCP), `runAgent.ts:396-416` (Claude.md stripping), `AgentTool.tsx:861-955` (worktree).

### 2.5 Agent Naming and Registry

| Aspect | Go | Upstream |
|--------|-----|----------|
| **Agent ID format** | 8-char hex string via `crypto/rand` (`agent_tools.go:366-373`) | UUID via `createAgentId()` (`E:\Git\claude-code-upstream\src\utils\uuid.js`) |
| **Agent name registry** | Not supported | `agentNameRegistry` in AppState -- maps name to agentId for `SendMessage` routing (`AgentTool.tsx:974-979`) |
| **Agent types** | `subagent_type` string (free-form) | `AgentDefinition` objects with `agentType`, `source`, `color`, `permissionMode`, `maxTurns`, `model`, `isolation`, `background`, `memory`, `hooks`, `skills`, `mcpServers` (`loadAgentsDir.js`) |
| **Built-in agents** | Not implemented | `builtInAgents.ts` registers: general-purpose, statusline setup, explore, plan, code guide, verification (`builtInAgents.ts:22-72`) |
| **Agent definitions loading** | Not supported | `loadAgentsDir()` loads from disk, frontmatter parsing, MCP requirements, filtering (`loadAgentsDir.js`) |
| **Fork subagent** | `inherit_context` approximates this | Dedicated `FORK_AGENT` synthetic definition, cache-identical API prefixes, parent system prompt inheritance (`forkSubagent.ts:60-71`) |

### 2.6 Agent Status Tracking

| Aspect | Go | Upstream |
|--------|-----|----------|
| **Status values** | `pending`, `running`, `completed`, `failed`, `killed` (`agent_tools.go:17-23`) | `running`, `completed`, `failed`, `killed` (via `TaskState.status`) |
| **Store** | `AgentTaskStore` -- in-memory map with RWMutex (`agent_tools.go:167-373`) | `AppState.tasks` -- React state record keyed by agentId (`LocalAgentTask.tsx`) |
| **Progress tracking** | `toolsUsed`, `durationMs`, `Output` buffer (50KB cap with truncation) | `AgentProgress`: `toolUseCount`, `tokenCount`, `lastActivity`, `recentActivities`, `summary` (`LocalAgentTask.tsx:57-63`) |
| **Progress display** | Text-based (`agent_tools.go:61-99`) | Rich UI with `renderGroupedAgentToolUse`, `BackgroundHint`, SDK `emitTaskProgress` events |
| **Notified flag** | `AgentTask.Notified` bool (`agent_tools.go:49`) | `LocalAgentTaskState.notified` -- atomic check-and-set to prevent duplicate notifications (`LocalAgentTask.tsx:299-316`) |
| **Output file** | `OutputFile` path for live output (`agent_tools.go:51`) | `getTaskOutputPath(taskId)` -- symlinked to transcript (`LocalAgentTask.tsx:330`) |
| **Transcript** | `TranscriptPath` stored on task | Sidechain JSONL transcript via `recordSidechainTranscript()` (`runAgent.ts:741`) |
| **Retention/eviction** | Not implemented | `retain`, `evictAfter`, `diskLoaded`, `PANEL_GRACE_MS` for UI memory management (`LocalAgentTask.tsx:193-202`) |

**File references:** Go: `agent_tools.go:35-55` (AgentTask struct), `agent_tools.go:167-373` (AgentTaskStore). Upstream: `LocalAgentTask.tsx:171-203` (LocalAgentTaskState), `LocalAgentTask.tsx:57-63` (AgentProgress).

### 2.7 Agent Notification System

| Aspect | Go | Upstream |
|--------|-----|----------|
| **Notification mechanism** | Not implemented in these files -- likely handled by main REPL loop | `enqueueAgentNotification()` -- XML-structured `task-notification` tags with taskId, outputFile, status, summary, result, usage, worktree info (`LocalAgentTask.tsx:272-350`) |
| **Notification format** | N/A | XML tags: `task_id`, `output_file`, `status`, `summary`, `result`, `usage`, `worktree` |
| **Duplicate prevention** | Not implemented | Atomic `notified` flag check-and-set in `updateTaskState` (`LocalAgentTask.tsx:299-316`) |
| **SDK events** | Not implemented | `enqueueSdkEvent()` for foreground agents (`AgentTool.tsx:1557-1574`) |
| **Speculation abort** | Not implemented | `abortSpeculation()` called when notification arrives to discard stale speculated responses (`LocalAgentTask.tsx:321`) |

### 2.8 Agent Cancellation and Cleanup

| Aspect | Go | Upstream |
|--------|-----|----------|
| **Kill mechanism** | `AgentTaskStore.Kill(id)` calls `CancelFunc()` and sets status to killed (`agent_tools.go:254-266`) | `killAsyncAgent()` -- aborts controller, unregisters cleanup, sets status, evicts output (`LocalAgentTask.tsx:370-392`) |
| **Kill all** | Not implemented | `killAllRunningAgentTasks()` -- for ESC cancellation in coordinator mode (`LocalAgentTask.tsx:398-407`) |
| **Cleanup handler** | Not implemented | `registerCleanup()` -- auto-kills on session end (`LocalAgentTask.tsx:624-628`) |
| **Post-kill cleanup** | Sets status only | Also: evicts output file, clears file state cache, kills shell tasks, clears session hooks, unregisters Perfetto agent, clears todos entry, kills MCP monitor tasks (`runAgent.ts:841-886`) |
| **Worktree cleanup** | Not supported | `cleanupWorktreeIfNeeded()` -- removes worktree if no changes detected via `hasWorktreeChanges()` (`AgentTool.tsx:922-955`) |
| **Abort controller linking** | Single `CancelFunc` per task | Complex: parent-child linking (`createChildAbortController`), unlinked for async agents, shared for sync agents (`AgentTool.tsx:526-535`) |

### 2.9 Agent Output Collection

| Aspect | Go | Upstream |
|--------|-----|----------|
| **Output capture** | `strings.Builder` with 50KB cap, truncation with marker (`agent_tools.go:61-100`) | Message array from `query()` async generator, streamed to task state |
| **Output truncation** | Keep first quarter + truncation marker + recent content (`agent_tools.go:72-99`) | Not truncated in memory; disk eviction via `evictTaskOutput()` |
| **Live output file** | `OutputFile` path written incrementally | Symlink to transcript file (`initTaskOutputAsSymlink`) |
| **Final result extraction** | Not in these files -- likely done when agent completes | `finalizeAgentTool()` -- extracts text from last assistant message, counts tool uses, computes tokens (`agentToolUtils.ts:277-358`) |
| **Partial result** | Not implemented | `extractPartialResult()` -- used when async agent is killed (`agentToolUtils.ts:489-501`) |
| **Usage tracking** | `ToolsUsed` (int), `DurationMs` (int64) | Full usage: `input_tokens`, `output_tokens`, `cache_creation_input_tokens`, `cache_read_input_tokens`, `cache_creation.ephemeral_*` |
| **Result schema** | Formatted string via `formatAgentResult()` (`agent_tool.go:182-197`) | Zod schema: `agentToolResultSchema` with agentId, agentType, content, totalToolUseCount, totalDurationMs, totalTokens, usage (`agentToolUtils.ts:227-258`) |

### 2.10 Agent Results Persistence

| Aspect | Go | Upstream |
|--------|-----|----------|
| **Sidechain transcript** | Not mentioned | JSONL transcript via `recordSidechainTranscript()` with parent chain UUID linking (`runAgent.ts:819-828`) |
| **Agent metadata** | Not mentioned | `writeAgentMetadata()` -- stores agentType, worktreePath, description (`runAgent.ts:744-748`) |
| **Resume from disk** | Not implemented | `resumeAgent.ts` -- reconstructs agent state from sidechain transcript and metadata |
| **Langfuse tracing** | Not implemented | Sub-agent traces via `createSubagentTrace()` sharing sessionId with parent (`runAgent.ts:756-769`) |
| **Perfetto tracing** | Not implemented | Agent hierarchy registration via `registerPerfettoAgent()` (`runAgent.ts:362-365`, `runAgent.ts:859`) |
| **Analytics** | Not implemented | Multiple events: `tengu_agent_tool_selected`, `tengu_agent_tool_completed`, `tengu_agent_tool_terminated`, `tengu_cache_eviction_hint`, `tengu_fork_agent_query` |
| **Handoff classifier** | Not implemented | `classifyHandoffIfNeeded()` -- YOLO safety classifier reviews sub-agent output before returning to parent (`agentToolUtils.ts:390-482`) |
| **Background summarization** | Not implemented | `startAgentSummarization()` -- periodic LLM summaries of running agent progress (`AgentTool.tsx:544-554`) |

### 2.11 Messaging Between Agents

| Aspect | Go | Upstream |
|--------|-----|----------|
| **Send to sub-agent** | `AddPendingMessage()` / `DrainPendingMessages()` on `AgentTask` (`agent_tools.go:111-128`) | `queuePendingMessage()` / `drainPendingMessages()` / `appendMessageToLocalAgent()` (`LocalAgentTask.tsx:224-267`) |
| **SendMessage tool** | Not implemented | `SendMessageTool` -- routes messages by name or agentId, requires `agentNameRegistry` |
| **Message queue** | Simple `[]string` field on task | `enqueuePendingNotification()` via `messageQueueManager` |

### 2.12 Key Architecture Differences Summary

1. **Go is a simplified async-only implementation** -- the upstream has dual sync/async execution with complex foreground-to-background transition, auto-backgrounding, and rich UI integration.

2. **Go has no agent type system** -- agents are identified only by free-form `subagent_type` string. Upstream has rich `AgentDefinition` with models, permission modes, MCP servers, hooks, skills, memory, colors.

3. **Go has no fork mechanism** -- the `inherit_context` boolean approximates upstream's fork subagent experiment, which has cache-identical API prefix sharing, parent system prompt inheritance, and recursive fork guards.

4. **Go has no isolation** -- no worktree, remote, or cwd override support. Upstream has comprehensive isolation options.

5. **Go has no teammate/multi-agent support** -- upstream has `spawnTeammate()`, `team_name`, `name`, `agentNameRegistry`, and `SendMessage` routing.

6. **Go has minimal cleanup** -- just cancel context. Upstream has extensive cleanup: file state cache, session hooks, prompt cache tracking, Perfetto, Langfuse, todos, shell tasks, MCP servers, worktrees.

7. **Go has no persistence** -- no sidechain transcript, no metadata, no resume. Upstream has full persistence and resume capability.

8. **Go has no analytics/tracing** -- upstream has extensive analytics events, Langfuse sub-agent traces, Perfetto hierarchy visualization.

9. **Go has no safety classifier** -- upstream runs `classifyHandoffIfNeeded()` before returning sub-agent results to parent.

10. **Go has no model resolution complexity** -- upstream considers agent definition, parent model, permission mode, and fork gate for model selection.

---


---

### 54.8 Agent Tool

**Go**: `tools/agent_tool.go` (198 lines) · **Upstream**: `AgentTool/AgentTool.tsx` + 14 helper files (~3000+ lines total)

| # | Aspect | Go (file:line) | Upstream (file:line) | Type |
|---|--------|----------------|----------------------|------|
| 1 | **Input schema** | `description`, `prompt`, `subagent_type`, `model`, `run_in_background`, `allowed_tools`, `disallowed_tools`, `inherit_context`, `max_turns`, `timeout` (line 60-109) | `description`, `prompt`, `subagent_type`, `model`, `run_in_background`, `name`, `team_name`, `mode`, `isolation`, `cwd` (line 167-234) | Go适配 |
| 2 | **Model selection** | `"sonnet"`, `"opus"`, `"haiku"` enum (line 78-79) | Same enum `z.enum(['sonnet', 'opus', 'haiku'])` (line 177-178) | ✅ Match |
| 3 | **allowed_tools / disallowed_tools** | Supported — explicit whitelist/blacklist (line 86-95) | Not in schema — tool filtering handled by permission context | Go增强 |
| 4 | **inherit_context** | Supported — fork mode boolean (line 96-98) | `isForkSubagentEnabled()` gate — not a schema parameter | Go适配 |
| 5 | **max_turns** | Supported — default 200 (line 100-102, 133-135) | Not a schema parameter — handled by agent loop limits | Go增强 |
| 6 | **timeout parameter** | Supported — max 600s (line 103-107) | Not a schema parameter — handled by task system | Go增强 |
| 7 | **Always background** | `run_in_background=true` always passed, description says DEPRECATED (line 83-84, 143) | Conditionally available based on `isBackgroundTasksDisabled` and `isForkSubagentEnabled` (line 254-256) | Go适配 |
| 8 | **Recursive agent blocking** | Always appends `"agent"` to disallowedTools (line 139) | `ALL_AGENT_DISALLOWED_TOOLS` constant | ✅ Match |
| 9 | **Multi-agent / team** | Not present | `name`, `team_name`, `mode` parameters (line 194-212) | 缺失 |
| 10 | **Isolation modes** | Not present | `isolation: "worktree"\|"remote"` — creates git worktree or remote session (line 217-226) | 缺失 |
| 11 | **CWD override** | Not present | `cwd` parameter — override working directory (line 227-233) | 缺失 |
| 12 | **Agent memory** | Not present | `agentMemory.ts` + `agentMemorySnapshot.ts` | 缺失 |
| 13 | **Agent color management** | Not present | `agentColorManager.ts` | 缺失 |
| 14 | **Built-in agent types** | `subagent_type` string only | `builtInAgents.ts` — explore, plan, general-purpose, verification, claudeCodeGuide agents | 缺失 |
| 15 | **Fork subagent** | `inherit_context` flag (simplified) | `forkSubagent.ts` — message construction, worktree notice | 简化 |
| 16 | **Agent resume** | Not present | `resumeAgent.ts` | 缺失 |
| 17 | **Progress tracking** | Not present | `createProgressTracker()`, `emitTaskProgress()`, `getProgressUpdate()` | 缺失 |
| 18 | **Agent summarization** | Not present | `startAgentSummarization()` | 缺失 |
| 19 | **Remote agent** | Not present | `checkRemoteAgentEligibility()`, `registerRemoteAgentTask()`, `teleportToRemote()` | 缺失 |
| 20 | **Agent ID format** | Generated by `SpawnFunc` | `createAgentId()` UUID utility | Go适配 |
| 21 | **Output format** | Plain text with `agentId`, `output_file`, usage metadata (line 147-158, 182-197) | Structured schema: `{status, prompt, result?, agentId?}` or `{status: "async_launched", agentId, ...}` | 简化 |
| 22 | **shouldDefer** | Not marked | Not deferred in upstream either | ✅ Match |

---


---

