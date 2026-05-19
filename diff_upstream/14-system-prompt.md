# System Prompt

> System prompt construction, templates

## Sections Included
- [##] Line 264-379 -- ## 4. System Prompt
- [##] Line 11627-11735 -- ## 55. System Prompt Construction

---

## Content

## 4. System Prompt

### Files Compared
- **Go**: `E:\Git\miniClaudeCode-go-github\system_prompt.go` (621 lines)
- **Upstream**: `E:\Git\claude-code-upstream\src\utils\systemPrompt.ts` (124 lines) + `E:\Git\claude-code-upstream\src\constants\prompts.ts` (system prompt assembly) + `E:\Git\claude-code-upstream\src\utils\api.ts` (`splitSysPromptPrefix` at line 321)

### 4.1 Prompt Sections and Ordering

**Go** (`BuildSystemPrompt`, line 225):
```
1. systemPromptTemplateStatic (lines 23-170):
   - Model name + greeting
   - Environment (OS, WD, date/time, shell)
   - Git context
   - Tool descriptions (from registry)
   - System rules (25+ bullet points)
   - Doing tasks guidance
   - Executing actions with care
   - Using your tools (tool selection tree)
   - Communicating with the user
   - Tone and style

2. SYSTEM_PROMPT_STATIC_BOUNDARY ("<!-- STATIC_PROMPT_END -->")

3. systemPromptTemplateDynamic (lines 172-185):
   - Permission mode (ASK/AUTO/PLAN)
   - Mode description
   - Project instructions (from CLAUDE.md)
   - Skills section
   - Session-specific guidance
   - Context management info
```

**Upstream** (`getSystemPrompt` in prompts.ts, lines ~450-667):
```
1. Intro section (model identity + greeting)
2. System section (system rules)
3. Doing tasks section (optional, based on output style)
4. Actions section (careful execution)
5. Using your tools section
6. Tone and style section
7. Output efficiency section
8. SYSTEM_PROMPT_DYNAMIC_BOUNDARY ("__SYSTEM_PROMPT_DYNAMIC_BOUNDARY__")
9. Dynamic sections (registry-managed):
   - MCP instructions
   - Permission mode
   - Agent instructions
   - Skills
   - Memory
   - Proactive mode
   - Token budget
   - Brief mode
   - And many more feature-gated sections
```

### 4.2 Static/Dynamic Boundary

| Aspect | Go | Upstream TS |
|--------|-----|-------------|
| Boundary marker | `"<!-- STATIC_PROMPT_END -->"` (line 21) | `"__SYSTEM_PROMPT_DYNAMIC_BOUNDARY__"` |
| Static cache scope | `{"type": "ephemeral"}` (global concept in comments only) | `cacheScope: 'global'` (explicit API scope) |
| Dynamic cache scope | `{"type": "ephemeral"}` | `cacheScope: null` |
| Boundary detection | `SplitSystemPrompt` with `strings.Index` (line 612-619) | `systemPrompt.indexOf(SYSTEM_PROMPT_DYNAMIC_BOUNDARY)` (line 363) |

### 4.3 Dynamic Content Injection

| Content Type | Go | Upstream TS |
|-------------|-----|-------------|
| Tool descriptions | Yes -- from `tools.Registry.AllTools()` (line 370) with usage hints | Yes -- `getUsingYourToolsSection(enabledTools)` with feature-gated tool schemas |
| Permission mode | Yes -- inline text (line 351) | Yes -- separate dynamic section via registry |
| Project instructions (CLAUDE.md) | Yes -- `LoadProjectInstructions` (line 233) | Yes -- via dynamic sections registry |
| Skills | Yes -- always-on + "New This Turn" + summary (lines 243-333) | Yes -- skill discovery with feature gates |
| Session memory | **No** -- explicitly NOT injected (lines 239-240) | Yes -- via dynamic sections |
| MCP instructions | **No** | Yes -- `getMcpInstructions` at lines 669-694 |
| Git context | Yes -- `tools.GetGitContext()` (line 347) | Yes -- via `computeEnvInfo` |
| Attribution header | **No** | Yes -- `getAttributionHeader` injected as first block |
| CLI_SYSPROMPT_PREFIX | **No** | Yes -- recognized as separately cacheable block |
| Scratchpad directory | **No** | Yes -- `getScratchpadInstructions` at lines 889-911 |
| Function result clearing | **No** | Yes -- `getFunctionResultClearingSection` at lines 913+ |

### 4.4 System Prompt Caching (CachedSystemPrompt)

**Go** has a `CachedSystemPrompt` struct (lines 381-607) with:
- Static/dynamic dirty tracking via `atomic.Bool`
- FNV-1a hash for static content change detection (line 503)
- `MarkStaticDirty()` / `MarkDynamicDirty()` for invalidation
- `GetStaticHash()` for per-tool schema caching

**Upstream** has no equivalent runtime caching struct -- the system prompt is built fresh per-request but split into cacheable blocks via `splitSysPromptPrefix`.

**Finding**: Go's `CachedSystemPrompt` is a unique addition not present in upstream. It optimizes system prompt assembly across turns, which is reasonable for a lightweight client but not present in upstream's architecture.

### 4.5 Environment/Platform Info

| Info | Go | Upstream TS |
|------|-----|-------------|
| Model name | Yes -- `modelName` param (line 348) | Yes -- `getMarketingNameForModel` + model ID |
| OS/Platform | Yes -- `runtime.GOOS/Version/GOARCH` (line 231) | Yes -- `computeEnvInfo` with uname, platform |
| Working directory | Yes -- `os.Getwd()` (line 230) | Yes -- `getCwd()` |
| Shell info | Static text "PowerShell on Windows, sh/bash on Unix" (line 29) | Dynamic -- `getShellInfoLine()` reads `$SHELL` (line 825) |
| Date/time | Yes -- `time.Now().Format()` (line 335) | Yes -- included in env info |
| Git context | Yes -- `tools.GetGitContext()` | Yes -- `getIsGit()` |
| Knowledge cutoff | **No** | Yes -- `getKnowledgeCutoff` by model (lines 803-822) |
| Latest model info | **No** | Yes -- Claude 4.5/4.6/4.7 model IDs, desktop/web/IDE info |
| Worktree detection | **No** | Yes -- `getCurrentWorktreeSession` |

### 4.6 Memory/Context Injection

| Aspect | Go | Upstream TS |
|--------|-----|-------------|
| Session memory | Explicitly NOT injected (line 240) | Yes -- via dynamic sections in registry |
| Context prepend | **No** | Yes -- `prependUserContext` adds `<system-reminder>` with CLAUDE.md, git status (api.ts:447-472) |
| Context append | **No** | Yes -- `appendSystemContext` (api.ts:435-445) |

---


---

## 55. System Prompt Construction

### 55.1 Prompt Structure
| # | Aspect | Go (file:line) | Upstream (file:line) | Type |
|---|--------|----------------|----------------------|------|
| 1 | Static/dynamic boundary marker | `system_prompt.go:21` (`<!-- STATIC_PROMPT_END -->`) | `prompts.ts:116-117` (`__SYSTEM_PROMPT_DYNAMIC_BOUNDARY__`) | Go适配 |
| 2 | Section-based composition (modular functions) | `system_prompt.go:172-186` (inline template strings, 3 sections) | `prompts.ts:129-667` (15+ modular functions: getSimpleIntroSection, getSimpleSystemSection, getSimpleDoingTasksSection, getActionsSection, getUsingYourToolsSection, getSimpleToneAndStyleSection, getOutputEfficiencySection, getSessionSpecificGuidanceSection, getAgentToolSection, getDiscoverSkillsGuidance, etc.) | 简化 |
| 3 | Section ordering | `system_prompt.go:21-170` (Environment -> System -> Doing tasks -> Executing actions with care -> Using your tools -> Communicating with the user -> Tool Parameters -> Tone and style) | `prompts.ts:650-666` (Intro -> System -> Doing tasks -> Actions -> Using tools -> Tone and style -> Output efficiency -> BOUNDARY -> dynamic sections) | Go适配 |
| 4 | Prompt as string array | `system_prompt.go:21` (single concatenated string) | `prompts.ts:650-666` (returns `string[]` array, each element is a section) | 简化 |
| 5 | Simple/override mode | 缺失 | `prompts.ts:540-544` (CLAUDE_CODE_SIMPLE env returns minimal prompt) | 缺失 |
| 6 | Section caching with invalidation | `system_prompt.go:381-575` (CachedSystemPrompt with static/dynamic hash, MarkStaticDirty, MarkDynamicDirty) | `systemPromptSections.ts:8-68` (systemPromptSection + DANGEROUS_uncachedSystemPromptSection + resolveSystemPromptSections, cache cleared on /clear or /compact) | Go增强 |
| 7 | Agent SDK prefix selection | 缺失 | `system.ts:10-46` (getCLISyspromptPrefix: DEFAULT / AGENT_SDK_CLAUDE_CODE_PRESET / AGENT_SDK based on isNonInteractive + hasAppendSystemPrompt) | 缺失 |
| 8 | Effective prompt resolution (override/agent/custom/default) | 缺失 | `systemPrompt.ts:41-123` (buildEffectiveSystemPrompt: priority 0=override, 1=coordinator, 2=agent, 3=custom, 4=default + append) | 缺失 |
| 9 | Cyber risk instruction | 缺失 | `prompts.ts:102,183` (CYBER_RISK_INSTRUCTION prepended to intro) | 缺失 |
| 10 | Output Style section | 缺失 | `prompts.ts:153-159` (getOutputStyleSection with config.name + config.prompt) | 缺失 |
| 11 | Language section | 缺失 | `prompts.ts:144-150` (getLanguageSection for user language preference) | 缺失 |
| 12 | Numeric length anchors (ant-only) | 缺失 | `prompts.ts:619-626` ("Length limits: keep text between tool calls to <=25 words...") | 缺失 |
| 13 | Token budget section | 缺失 | `prompts.ts:628-641` (feature-gated token budget instructions) | 缺失 |
| 14 | Brief/Kairos section | 缺失 | `prompts.ts:642-643,935-950` (feature-gated BRIEF tool section) | 缺失 |
| 15 | Proactive/Autonomous mode section | 缺失 | `prompts.ts:956-1006` (getProactiveSection: tick handling, pacing, bias toward action) | 缺失 |
| 16 | Scratchpad directory section | 缺失 | `prompts.ts:889-911` (getScratchpadInstructions: temp file directory guidance) | 缺失 |
| 17 | Function result clearing section | `system_prompt.go:185` (hardcoded "keep 5 most recent results") | `prompts.ts:913-931` (getFunctionResultClearingSection: feature-gated, model-specific, config-driven) | 简化 |

### 55.2 Tool Descriptions
| # | Aspect | Go (file:line) | Upstream (file:line) | Type |
|---|--------|----------------|----------------------|------|
| 1 | Tool description source | `system_prompt.go:357-378` (buildToolList: iterates registry.AllTools(), appends Description() + optional hints) | `prompts.ts:260-403` (getUsingYourToolsSection: dynamic, per-tool-name conditional rendering, REPL-mode aware, embedded-search aware) | 简化 |
| 2 | Tool hints map | `system_prompt.go:359-367` (hardcoded map: glob/grep/file_read/exec/file_edit/file_write/TodoWrite) | `prompts.ts:282-293` (providedToolSubitems: dynamically imports tool name constants from @claude-code-best/builtin-tools) | 简化 |
| 3 | Tool selection decision tree | `system_prompt.go:96-100` (4 steps: need tool? dedicated tool? shell op? parallel?) | `prompts.ts:298-303` (toolSelectionDecisionTree: same 4 steps, dynamically interpolates tool name constants) | Go适配 |
| 4 | Few-shot tool examples | `system_prompt.go:126-135` (hardcoded Go-centric examples: go test, go get) | `prompts.ts:308-319` (fewShotExamples: dynamically uses BUN_TOOL_NAME, BUN add, dynamic tool name constants) | Go适配 |
| 5 | Grep query construction guidance | `system_prompt.go:106` | `prompts.ts:324` (identical guidance, dynamically interpolated tool name) | 修正 |
| 6 | Glob query construction guidance | `system_prompt.go:108` | `prompts.ts:326-328` (hidden when hasEmbeddedSearchTools() is true) | Go适配 |
| 7 | Fallback chain guidance | `system_prompt.go:110-114` | `prompts.ts:351-357` (identical structure, dynamic tool names) | 修正 |
| 8 | Cost asymmetry principle | `system_prompt.go:104` | `prompts.ts:344-347` (identical guidance, dynamic tool names) | 修正 |
| 9 | Anti-pattern guidance (when NOT to use tools) | `system_prompt.go:81-85` | `prompts.ts:333-339` (expanded: includes "don't re-read content already in context") | Go适配 |
| 10 | REPL mode awareness | 缺失 | `prompts.ts:268-276` (isReplModeEnabled: hides Read/Write/Edit/Glob/Grep/Bash/Agent, simplifies guidance) | 缺失 |
| 11 | Embedded search tools awareness | 缺失 | `prompts.ts:280,326-328` (hasEmbeddedSearchTools: hides Glob/Grep when find/grep aliased in shell) | 缺失 |
| 12 | Agent tool section | `system_prompt.go:122` (inline paragraph about agent tool forking) | `prompts.ts:405-409` (getAgentToolSection: conditional on isForkSubagentEnabled, dynamic tool name) | Go适配 |
| 13 | Agent prompt (full) | 缺失 | `AgentTool/prompt.ts:66-287` (extensive agent tool prompt: fork semantics, writing the prompt, examples, concurrency notes, isolation modes) | 缺失 |
| 14 | TodoWrite tool guidance | `system_prompt.go:93` (inline bullet about using TodoWrite) | `prompts.ts:270-272,379-381` (conditionally rendered when taskToolName is available) | 修正 |

### 55.3 Context Injection
| # | Aspect | Go (file:line) | Upstream (file:line) | Type |
|---|--------|----------------|----------------------|------|
| 1 | Environment info | `system_prompt.go:25-30` (OS, WD, DateTime, Shell, GitContext) | `prompts.ts:741-799` (computeSimpleEnvInfo: cwd, worktree detection, is git, platform, shell, OS version, model description, knowledge cutoff, Claude model family, Claude Code availability, fast mode info) | 简化 |
| 2 | Project instructions (CLAUDE.md) | `system_prompt.go:233-237` (LoadProjectInstructions: reads projectDir/CLAUDE.md) | `prompts.ts:650-666` (injected via resolvedDynamicSections, not directly in getSystemPrompt — loaded elsewhere) | Go适配 |
| 3 | Skills injection | `system_prompt.go:243-333` (always-on skills + "New This Turn" unsent skills + BuildSkillsSummary, with 4000 char budget) | `prompts.ts:441-491` (getSessionSpecificGuidanceSection: skill tool commands, DiscoverSkills guidance, skill invocation via /<skill-name>) | 简化 |
| 4 | Skill discovery | `system_prompt.go:248-253` (Skill System Guidance: search_skills + read_skill) | `prompts.ts:422-430` (getDiscoverSkillsGuidance: feature-gated EXPERIMENTAL_SKILL_SEARCH, DiscoverSkillsTool) | Go适配 |
| 5 | File attachment injection (post-compact) | `context.go:886-894` (AddAttachment: user-role entry with AttachmentContent) | `prompts.ts` (attachments.ts: agent_listing_delta, mcp_instructions_delta, skill_discovery attachments) | Go适配 |
| 6 | History snip after compaction | `context.go:902-972` (AddHistorySnip: preserves N entries before boundary as user-role text) | N/A (upstream uses session memory + attachments instead) | Go增强 |
| 7 | Keep recent messages after compaction | `context.go:987-1032` (KeepRecentMessages: preserves tool structure, adjusts for tool pairing) | `prompts.ts` + upstream compaction (uses messagesToKeepIndex + adjustIndexToPreserveAPIInvariants) | Go增强 |
| 8 | Keep recent messages adaptive (token-based) | `context.go:1040-1144` (KeepRecentMessagesAdaptive: minTokens/minTextMsgs/maxTokens budgets) | 缺失 | Go增强 |
| 9 | Append system prompt | 缺失 | `systemPrompt.ts:39-40,115-121` (--system-prompt flag always appended at end) | 缺失 |
| 10 | Additional working directories | 缺失 | `prompts.ts:696-723,767-777` (additionalWorkingDirectories parameter in computeEnvInfo/computeSimpleEnvInfo) | 缺失 |

### 55.4 Memory/Session
| # | Aspect | Go (file:line) | Upstream (file:line) | Type |
|---|--------|----------------|----------------------|------|
| 1 | Session memory NOT injected in system prompt | `system_prompt.go:239-240` (explicitly NOT injected; used during compaction as user message) | `prompts.ts:585` (memory section: `systemPromptSection('memory', () => loadMemoryPrompt())` — injected via resolveSystemPromptSections) | 差异 |
| 2 | ToolStateTracker (session state note) | `context.go:104-279` (epoch-based freshness tracking: readFiles, searchQueries, conclusions; BuildSessionStateNote with fresh/stale split) | N/A (upstream uses session_memory file with structured template) | Go增强 |
| 3 | Session memory template | `session_memory.go:19-48` (10-section template: Session Title, Current State, Task Spec, Files, Workflow, Errors, Documentation, Learnings, Key Results, Worklog) | `SessionMemory/prompts.ts:11-41` (identical 10-section structure) | 修正 |
| 4 | Session memory update prompt | 缺失 (Go uses different approach with memdir + memory_tool.go) | `SessionMemory/prompts.ts:43-80` (getDefaultUpdatePrompt: detailed editing instructions with section preservation rules) | 差异 |
| 5 | Session memory section size limits | `session_memory.go:54-55` (maxTokensPerSection=20000, maxTotal=60000 — increased from upstream's 2000/12000) | `SessionMemory/prompts.ts:8-9` (MAX_SECTION_LENGTH=2000, MAX_TOTAL_SESSION_MEMORY_TOKENS=12000) | Go增强 |
| 6 | Session memory custom template loading | 缺失 | `SessionMemory/prompts.ts:86-103` (loadSessionMemoryTemplate from ~/.claude/session-memory/config/template.md) | 缺失 |
| 7 | Session memory custom prompt loading | 缺失 | `SessionMemory/prompts.ts:111-128` (loadSessionMemoryPrompt from ~/.claude/session-memory/config/prompt.md with {{variable}} substitution) | 缺失 |
| 8 | Session memory compact truncation | `session_memory.go` (truncateSessionMemoryForCompact equivalent) | `SessionMemory/prompts.ts:256-324` (truncateSessionMemoryForCompact: per-section char-based truncation) | 修正 |
| 9 | Memdir (persistent memory) | Go has `memdir/memdir.go` with loadMemoryPrompt | `prompts.ts:61` (`loadMemoryPrompt` imported from memdir) + `SessionMemory/prompts.ts` | 修正 |
| 10 | Hooks feedback injection | `system_prompt.go:179` ("Users may configure 'hooks'...") | `prompts.ts:129-131` (getHooksSection: identical text) | 修正 |
| 11 | System reminders section | `system_prompt.go:37-40` (<system-reminder> tags, unlimited context) | `prompts.ts:133-136` (getSystemRemindersSection: identical text) | 修正 |

### 55.5 MCP Tools
| # | Aspect | Go (file:line) | Upstream (file:line) | Type |
|---|--------|----------------|----------------------|------|
| 1 | MCP tool descriptions in prompt | 缺失 (MCP tools exist only in worktree branch, not in main) | `prompts.ts:669-694` (getMcpInstructions: per-server instruction blocks with `## server_name\ninstructions`) | 缺失 |
| 2 | MCP instructions section | 缺失 | `prompts.ts:162-167` (getMcpInstructionsSection: conditional on mcpClients) | 缺失 |
| 3 | MCP delta instructions (cache-friendly) | 缺失 | `prompts.ts:603-609` (DANGEROUS_uncachedSystemPromptSection for MCP: isMcpInstructionsDeltaEnabled check) | 缺失 |
| 4 | MCP server connection awareness | 缺失 | `prompts.ts:538` (mcpClients parameter to getSystemPrompt) | 缺失 |
| 5 | MCP resource tools | 缺失 | builtin-tools: ListMcpResourcesTool/prompt.ts, ReadMcpResourceTool/prompt.ts | 缺失 |
| 6 | MCP tool prompt | 缺失 | builtin-tools: MCPTool/prompt.ts | 缺失 |

### 55.6 Permission Modes
| # | Aspect | Go (file:line) | Upstream (file:line) | Type |
|---|--------|----------------|----------------------|------|
| 1 | Permission mode descriptions in prompt | `system_prompt.go:218-222` (map: ask="potentially dangerous operations require confirmation", auto="all operations auto-approved", plan=planModeInstructions) | Upstream handles permissions via UI dialogs and tool-level PermissionResult, NOT embedded in system prompt text. Permission modes are managed by the UI layer. | 差异 |
| 2 | Plan mode instructions | `system_prompt.go:187-216` (5-phase workflow: Explore, Design, Review, Final Plan, ExitPlanMode; "In PLAN mode, only read-only operations are allowed") | `plan/index.ts` + `ExitPlanModeTool/prompt.ts` + `EnterPlanModeTool/prompt.ts` (plan mode managed via tools and UI state, not inline prompt text) | Go适配 |
| 3 | Plan agent | 缺失 | `AgentTool/built-in/planAgent.ts` (specialized PLAN_AGENT agent type for plan mode) | 缺失 |
| 4 | Explore/Plan agents enabled | `system_prompt.go:193-197` (references EXPLORE_AGENT, PLAN_AGENT in plan mode instructions) | `prompts.ts:463-470` (areExplorePlanAgentsEnabled + EXPLORE_AGENT_MIN_QUERIES checks) | Go适配 |
| 5 | AskUserQuestion for denied tools | `system_prompt.go:205` (Use AskUserQuestion to clarify in plan mode) | `prompts.ts:454-455` (hasAskUserQuestionTool conditional: "If you do not understand why the user has denied a tool call, use AskUserQuestion") | 修正 |
| 6 | Auto mode classifier | `permissions.go` (auto_classifier.go: classifier determines auto-approval) | `utils/permissions/yolo-classifier-prompts/auto_mode_system_prompt.txt` (classifier prompt file) + `autoModeDenials.ts` | 差异 |
| 7 | Verification agent | 缺失 | `prompts.ts:479-486` (feature-gated VERIFICATION_AGENT: adversarial verification for non-trivial implementation, PASS/FAIL/PARTIAL verdicts) | 缺失 |
| 8 | Sandbox instructions | 缺失 | `BashTool/prompt.ts:172-273` (getSimpleSandboxSection: filesystem/network restrictions, sandbox overrides, TMPDIR guidance) | 缺失 |

### 55.7 Platform Instructions
| # | Aspect | Go (file:line) | Upstream (file:line) | Type |
|---|--------|----------------|----------------------|------|
| 1 | Shell detection | `system_prompt.go:29` ("Shell: PowerShell on Windows, sh/bash on Unix" — hardcoded text) | `prompts.ts:824-834` (getShellInfoLine: reads process.env.SHELL, detects zsh/bash, platform-specific formatting for win32 with Unix syntax note) | 简化 |
| 2 | Platform/OS info | `system_prompt.go:231` (`runtime.GOOS / runtime.Version() / runtime.GOARCH`) | `prompts.ts:779-781` (env.platform + getUnameSR: os.type/os.release on POSIX, os.version/os.release on Windows) | Go适配 |
| 3 | Windows-specific shell guidance | `system_prompt.go:29` (static text) | `prompts.ts:831-832` (conditional: "use Unix shell syntax, not Windows — e.g., /dev/null not NUL, forward slashes in paths") | Go适配 |
| 4 | Windows worktree handling | 缺失 | `prompts.ts:765-770` (isWorktree detection + "Do NOT cd to original repo root" warning) | 缺失 |
| 5 | OS Version string | `system_prompt.go:231` (runtime.GOOS only) | `prompts.ts:837-847` (getUnameSR: full "Darwin 25.3.0" or "Windows 11 Pro 10.0.26200" style strings) | 简化 |
| 6 | Undercover mode (ant internal) | 缺失 | `prompts.ts:711-712,750-751,784-791` (isUndercover: suppresses model names/IDs in prompt, "go fully dark" for public repos) | 缺失 |
| 7 | Ant model override | 缺失 | `prompts.ts:138-142` (getAntModelOverrideSection: USER_TYPE === 'ant' + getAntModelOverrideConfig) | 缺失 |
| 8 | Attribution header | 缺失 | `system.ts:52-95` (x-anthropic-billing-header with cc_version, cc_entrypoint, cch attestation, cc_workload) | 缺失 |
| 9 | Knowledge cutoff | 缺失 | `prompts.ts:803-821` (getKnowledgeCutoff: per-model cutoff dates — Sonnet 4.6: Aug 2025, Opus 4.7: Jan 2026, etc.) | 缺失 |
| 10 | Claude Code availability info | 缺失 | `prompts.ts:787-789` (CLI, desktop, web, IDE, Chrome, Excel, Cowork availability) | 缺失 |
| 11 | Fast mode info | 缺失 | `prompts.ts:791-792` (Fast mode uses same model with faster output, /fast toggle) | 缺失 |
| 12 | Latest model family info | 缺失 | `prompts.ts:785-786` (Opus 4.7, Sonnet 4.6, Haiku 4.5 IDs for AI app development) | 缺失 |

---

---

