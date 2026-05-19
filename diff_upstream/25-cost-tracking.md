# Cost Tracking

> Cost tracking, usage, model routing

## Sections Included
- [##] Line 6818-7006 -- ## Section XX: Model Routing, Cost Tracking, and Background Services (2026-05-11)
- [###] Line 10211-10227 -- ### 46.2 Cost Tracking

---

## Content

## Section XX: Model Routing, Cost Tracking, and Background Services (2026-05-11)

### 1. Model Selection -- Go's single-model vs upstream's multi-model routing

**Go**: Single model from config. The `Config.Model` field is a raw string loaded once from `settings.json` env var `ANTHROPIC_MODEL`.
- `E:\Git\miniClaudeCode-go-github\config.go:28` — `Model string` field
- `E:\Git\miniClaudeCode-go-github\config.go:161` — loaded as `s.Env.AnthropicModel`
- `E:\Git\miniClaudeCode-go-github\config.go:286` — default is `Model: ""` (empty string)
- `E:\Git\miniClaudeCode-go-github\agent_loop.go:382` — passed directly to API as `cfg.Model`
- Go uses one model for everything: main loop, sub-agents (same model), classifier (falls back to same model)

**Upstream**: Multi-model system with per-model defaults, provider-specific routing, and subscription-tier defaults.
- `E:\Git\claude-code-upstream\src\utils\model\model.ts:103-108` — `getMainLoopModel()` with 5-level priority: /model command > --model flag > ANTHROPIC_MODEL env > settings > built-in default
- `E:\Git\claude-code-upstream\src\utils\model\model.ts:223-245` — `getDefaultMainLoopModelSetting()`: Max/Team Premium gets Opus, everyone else gets Sonnet
- `E:\Git\claude-code-upstream\src\utils\model\model.ts:37-48` — `getSmallFastModel()`: provider-specific (OpenAI/Gemini/Anthropic) small fast model
- `E:\Git\claude-code-upstream\src\utils\model\model.ts:116-138` — `getDefaultOpusModel()`: provider-specific (OpenAI, Gemini, Anthropic, Bedrock, Vertex, Foundry all have separate env vars)
- `E:\Git\claude-code-upstream\src\utils\model\model.ts:141-163` — `getDefaultSonnetModel()`: same provider-aware resolution
- `E:\Git\claude-code-upstream\src\utils\model\model.ts:166-183` — `getDefaultHaikuModel()`: same provider-aware resolution
- `E:\Git\claude-code-upstream\src\utils\model\providers.ts:5-12` — 7 providers: `firstParty`, `bedrock`, `vertex`, `foundry`, `openai`, `gemini`, `grok`
- `E:\Git\claude-code-upstream\src\utils\model\modelOptions.ts:350-453` — `getModelOptionsBase()`: per-tier model picker (ant, Max, Team Premium, Pro, PAYG 1P, PAYG 3P all get different option lists)
- `E:\Git\claude-code-upstream\src\utils\model\agent.ts:37-95` — `getAgentModel()`: sub-agents can use `inherit`, `sonnet`, `opus`, `haiku`, or tool-specified model with Bedrock region inheritance

### 2. Model Fallback -- automatic fallback to cheaper models

**Go**: No model fallback. On 429/529, the Go code classifies the error but does NOT switch models.
- `E:\Git\miniClaudeCode-go-github\error_types.go:326-328` — `classify402()` disambiguates billing vs transient usage limit but takes no fallback action
- `E:\Git\miniClaudeCode-go-github\compact.go:1119-1152` — `ContextWindowTracker` has no fallback model concept

**Upstream**: Sophisticated model fallback via `withRetry`.
- `E:\Git\claude-code-upstream\src\services\api\withRetry.ts:130` — `RetryOptions.fallbackModel?: string`
- `E:\Git\claude-code-upstream\src\services\api\withRetry.ts:160-168` — `FallbackTriggeredError` class with `originalModel` and `fallbackModel` fields
- `E:\Git\claude-code-upstream\src\services\api\withRetry.ts:337-349` — On overload, switches to `fallbackModel` and logs analytics
- `E:\Git\claude-code-upstream\src\query.ts:941-993` — `query.ts` catches `FallbackTriggeredError`, updates `currentModel = fallbackModel`, notifies user "Switched to {fallbackModel} due to high demand for {originalModel}"
- `E:\Git\claude-code-upstream\src\cli\print.ts:487` / `cli/print.ts:1007` — `fallbackModel: string | undefined` passed through options
- `E:\Git\claude-code-upstream\src\services\api\claude.ts:700,845,1896,2622,2721` — `fallbackModel` threaded through all message/stream creation calls

### 3. Model-Specific Context Windows -- Go's 200K default vs upstream's per-model limits

**Go**: Hardcoded context windows based on string matching.
- `E:\Git\miniClaudeCode-go-github\compact.go:1138-1152` — `modelContextWindow()`:
  - `[1m]` suffix → 1,000,000
  - `sonnet-4`, `opus-4-6`, `opus-4-7` (without haiku/3.5/3.0 suffix) → 1,000,000
  - Everything else → 200,000
- `E:\Git\miniClaudeCode-go-github\compact.go:1119-1133` — `ContextWindowTracker` uses single `modelMaxTokens` field, no per-model output limits

**Upstream**: Dynamic, multi-source context window resolution.
- `E:\Git\claude-code-upstream\src\utils\context.ts:56-103` — `getContextWindowForModel()`:
  1. `CLAUDE_CODE_MAX_CONTEXT_TOKENS` env override (ant-only)
  2. `[1m]` suffix → 1,000,000
  3. `getModelCapability()` → API-fetched `max_input_tokens` from `/v1/models` (cached, `modelCapabilities.ts:78-86`)
  4. Beta header + `modelSupports1M()` → 1,000,000
  5. GrowthBook sonnet 1M experiment → 1,000,000
  6. Ant model config → `antModel.contextWindow`
  7. Default: 200,000
- `E:\Git\claude-code-upstream\src\utils\context.ts:160-224` — `getModelMaxOutputTokens()`: per-model output token limits:
  - Opus 4.6/4.7: default 64K, upper 128K
  - Sonnet 4.6: default 32K, upper 128K
  - Opus 4.5/Sonnet 4/Haiku 4: default 32K, upper 64K
  - Opus 4/4.1: default 32K, upper 32K
  - Claude 3 Opus: default 4K, upper 4K
  - Claude 3 Sonnet: default 8K, upper 8K
  - Claude 3 Haiku: default 4K, upper 4K
  - 3.5 Sonnet/Haiku: default 8K, upper 8K
  - 3.7 Sonnet: default 32K, upper 64K
  - Default: 8K / 64K
- `E:\Git\claude-code-upstream\src\utils\context.ts:32-34` — `is1mContextDisabled()` via `CLAUDE_CODE_DISABLE_1M_CONTEXT` (HIPAA compliance)
- `E:\Git\claude-code-upstream\src\utils\model\modelCapabilities.ts:88-121` — `refreshModelCapabilities()`: fetches capabilities from `/v1/models` API and caches to `~/.claude/cache/model-capabilities.json`

### 4. Cost Tracking -- Go's basic token tracking vs upstream's dollar-cost tracking

**Go**: Tracks only raw token counts (input/output). No USD cost calculation.
- `E:\Git\miniClaudeCode-go-github\agent_loop.go:318-320` — `recordTokenUsage()` accumulates `inputTokens` and `outputTokens` into `AgentLoop.totalInputTokens`/`totalOutputTokens`
- `E:\Git\miniClaudeCode-go-github\agent_progress.go:149` — token usage reflected in progress display
- `E:\Git\miniClaudeCode-go-github\streaming.go:44-48` — `Usage` struct: `InputTokens, OutputTokens, CacheCreationInputTokens, CacheReadInputTokens`
- No cost tracking, no model-specific pricing, no `costUSD` field anywhere in Go codebase

**Upstream**: Full per-model dollar-cost tracking.
- `E:\Git\claude-code-upstream\src\utils\modelCost.ts:1-232` — `MODEL_COSTS` maps every model to `ModelCosts{inputTokens, outputTokens, promptCacheWriteTokens, promptCacheReadTokens, webSearchRequests}`:
  - Sonnet: $3/$15 per Mtok, cache write $3.75, cache read $0.30
  - Opus 4/4.1: $15/$75 per Mtok, cache write $18.75, cache read $1.50
  - Opus 4.5/4.6: $5/$25 per Mtok, cache write $6.25, cache read $0.50
  - Opus 4.6 fast mode: $30/$150 per Mtok
  - Haiku 3.5: $0.80/$4 per Mtok
  - Haiku 4.5: $1/$5 per Mtok
  - Web search: $0.01 per request
- `E:\Git\claude-code-upstream\src\utils\modelCost.ts:177-180` — `calculateUSDCost()`: computes `(input/1M)*inputRate + (output/1M)*outputRate + (cache_read/1M)*cacheReadRate + (cache_write/1M)*cacheWriteRate + (web_searches)*$0.01`
- `E:\Git\claude-code-upstream\src\utils\modelCost.ts:186-202` — `calculateCostFromTokens()`: for side queries like classifier
- `E:\Git\claude-code-upstream\src\cost-tracker.ts:278-323` — `addToTotalSessionCost()`: accumulates cost USD per model, includes advisor model costs
- `E:\Git\claude-code-upstream\src\cost-tracker.ts:181-226` — `formatModelUsage()`: aggregates by short name, displays `$cost` per model
- `E:\Git\claude-code-upstream\src\cost-tracker.ts:228-244` — `formatTotalCost()`: full cost summary with duration, lines changed, model breakdown
- `E:\Git\claude-code-upstream\src\cost-tracker.ts:143-175` — `saveCurrentSessionCosts()`: persists costs to project config JSON (survives restarts)
- `E:\Git\claude-code-upstream\src\cost-tracker.ts:87-123` — `getStoredSessionCosts()`: restores costs when resuming a session
- `E:\Git\claude-code-upstream\src\costHook.ts:1-22` — `useCostSummary()`: React hook that prints cost on exit and saves to config
- `E:\Git\claude-code-upstream\src\cost-tracker.ts:304-321` — Advisor model cost tracking (nested model calls like tool advisors)

### 5. Model Aliases -- upstream's alias system

**Go**: No alias system. Model string is used verbatim.
- `E:\Git\miniClaudeCode-go-github\config.go:28` — `Model string` is raw string

**Upstream**: Rich alias system with 8 defined aliases and family-based wildcarding.
- `E:\Git\claude-code-upstream\src\utils\model\aliases.ts:1-9` — `MODEL_ALIASES = ['sonnet', 'opus', 'haiku', 'best', 'sonnet[1m]', 'opus[1m]', 'opusplan']`
- `E:\Git\claude-code-upstream\src\utils\model\aliases.ts:21-24` — `MODEL_FAMILY_ALIASES = ['sonnet', 'opus', 'haiku']` for wildcarding in allowlists
- `E:\Git\claude-code-upstream\src\utils\model\model.ts:497-559` — `parseUserSpecifiedModel()`:
  - `sonnet` → `getDefaultSonnetModel()` + optional `[1m]`
  - `opus` → `getDefaultOpusModel()` + optional `[1m]`
  - `haiku` → `getDefaultHaikuModel()` + optional `[1m]`
  - `best` → `getBestModel()` (Opus)
  - `opusplan` → Sonnet normally, Opus in plan mode
- `E:\Git\claude-code-upstream\src\utils\model\model.ts:530-536` — Legacy Opus 4.0/4.1 auto-remap to current Opus (configurable via `CLAUDE_CODE_DISABLE_LEGACY_MODEL_REMAP`)
- `E:\Git\claude-code-upstream\src\utils\model\model.ts:576-589` — `resolveSkillModelOverride()`: carries `[1m]` suffix from current model to skill-specified models
- `E:\Git\claude-code-upstream\src\utils\model\modelAllowlist.ts:100-170` — `isModelAllowed()`: multi-tier matching (family alias, version prefix, exact ID, bidirectional resolution)
- `E:\Git\claude-code-upstream\src\utils\model\model.ts:262-318` — `firstPartyNameToCanonical()`: normalizes all provider-specific IDs to canonical form (handles Bedrock ARNs, Vertex IDs, date suffixes)
- `E:\Git\claude-code-upstream\src\utils\model\modelStrings.ts:84-100` — `resolveOverriddenModel()`: resolves Bedrock inference profile ARNs back to canonical model IDs

### 6. Model Capabilities -- upstream's capability flags per model

**Go**: No model capability system. Hardcoded in `modelContextWindow()`.

**Upstream**: Dynamic capability fetching via `/v1/models` API.
- `E:\Git\claude-code-upstream\src\utils\model\modelCapabilities.ts:19-27` — `ModelCapability` schema: `{id, max_input_tokens, max_tokens}`
- `E:\Git\claude-code-upstream\src\utils\model\modelCapabilities.ts:29-34` — Cache file: `{models: ModelCapability[], timestamp}`
- `E:\Git\claude-code-upstream\src\utils\model\modelCapabilities.ts:46-54` — `isModelCapabilitiesEligible()`: only `firstParty` provider with `api.anthropic.com` base URL
- `E:\Git\claude-code-upstream\src\utils\model\modelCapabilities.ts:78-86` — `getModelCapability()`: cached lookup with exact match then substring match (longest-id-first)
- `E:\Git\claude-code-upstream\src\utils\model\modelCapabilities.ts:88-121` — `refreshModelCapabilities()`: fetches from API, writes to `~/.claude/cache/model-capabilities.json`
- `E:\Git\claude-code-upstream\src\utils\model\modelCost.ts:94-99` — `getOpus46CostTier()`: fast mode has different pricing ($30/$150 vs $5/$25)
- `E:\Git\claude-code-upstream\src\utils\model\model.ts:50-58` — `isNonCustomOpusModel()`: checks if model is a known Opus variant
- `E:\Git\claude-code-upstream\src\utils\context.ts:178-215` — `getModelMaxOutputTokens()`: per-model default/upper limits (Opus 4.7: 64K/128K, Sonnet 4.6: 32K/128K, etc.)
- `E:\Git\claude-code-upstream\src\utils\context.ts:233-235` — `getMaxThinkingTokensForModel()`: derived from output limits

### 7. Daemon Service -- upstream's background daemon

**Go**: No daemon. No background process management.

**Upstream**: Full daemon system with supervisor, workers, backoff, and session management.
- `E:\Git\claude-code-upstream\src\daemon\main.ts:52-101` — `daemonMain()`: subcommands: `start`, `stop`, `status`, `bg`, `attach`, `logs`, `kill`
- `E:\Git\claude-code-upstream\src\daemon\main.ts:230-312` — `runSupervisor()`: worker management with exponential backoff (2s initial, 120s cap, 2x multiplier)
- `E:\Git\claude-code-upstream\src\daemon\main.ts:317-412` — `spawnWorker()`: spawns child processes with `DAEMON_WORKER_*` env vars, pipes stdout/stderr, handles rapid failure parking (5 failures < 10s → park)
- `E:\Git\claude-code-upstream\src\daemon\main.ts:26-33` — `WorkerState`: `{kind, process, backoffMs, failureCount, parked, lastStartTime}`
- `E:\Git\claude-code-upstream\src\daemon\main.ts:16` — `EXIT_CODE_PERMANENT = 78`: permanent (non-retryable) worker failure
- `E:\Git\claude-code-upstream\src\daemon\state.ts:48-52` — daemon state file: `writeDaemonState`, `readDaemonState`, `removeDaemonState`, `queryDaemonStatus`, `stopDaemonByPid`
- `E:\Git\claude-code-upstream\src\assistant\index.ts:26-35` — Assistant (KAIROS) daemon mode: `isAssistantMode()`, `getAssistantLaunchMode()` returns 'daemon' or 'gate'
- `E:\Git\claude-code-upstream\src\utils\cliLaunch.ts:8-97` — `buildCliLaunch()`: shared launcher for daemon workers, bg sessions, bridge sessions

### 8. Proactive Features -- upstream's proactive suggestions

**Go**: No proactive system.

**Upstream**: Tick-driven autonomous agent mode.
- `E:\Git\claude-code-upstream\src\proactive\index.ts:1-136` — State machine: `inactive → active → (paused) → inactive`
  - `activateProactive(source?)` / `deactivateProactive()` / `pauseProactive()` / `resumeProactive()`
  - `shouldTick()`: returns `active && !paused && !contextBlocked`
  - `setContextBlocked()`: blocks tick generation on API errors to prevent tick→error→tick runaway
  - `setNextTickAt(ts)` / `getNextTickAt()`: epoch-ms timestamp scheduling
  - `subscribeToProactiveChanges(cb)`: pub/sub state change notifications
- `E:\Git\claude-code-upstream\src\proactive\useProactive.ts` — React hook for proactive tick management
- `E:\Git\claude-code-upstream\src\constants\prompts.ts:942-956` — `getProactiveSection()`: injected into system prompt when active
- `E:\Git\claude-code-upstream\src\proactive\__tests__\state.baseline.test.ts` — Test coverage for proactive state management

### 9. Job System -- upstream's background job scheduler

**Go**: No job system.

**Upstream**: Job classification and state management for dispatched background jobs.
- `E:\Git\claude-code-upstream\src\jobs\classifier.ts` — `classifyAndWriteState(jobDir, turnMessages)`: classifies job status from assistant messages, writes `state.json`
- `E:\Git\claude-code-upstream\src\jobs\state.ts` — Job state management (read/write `state.json` in job directory)
- `E:\Git\claude-code-upstream\src\jobs\templates.ts` — Job templates for dispatched executions
- `E:\Git\claude-code-upstream\src\query\stopHooks.ts:46,100-121` — Template job classification: when `CLAUDE_JOB_DIR` is set, classifies turn and writes state
- `E:\Git\claude-code-upstream\src\setup.ts:282-299` — Background job startup: `setup_background_jobs_starting` / `setup_background_jobs_launched` log events
- `E:\Git\claude-code-upstream\src\utils\plugins\pluginAutoupdate.ts:225` — Plugin autoupdate runs as background job during startup

### 10. Self-Hosted Runner -- upstream's self-hosted execution

**Go**: No self-hosted runner.

**Upstream**: Self-hosted bridge and runner system (partially stubbed).
- `E:\Git\claude-code-upstream\src\self-hosted-runner\main.ts:1-3` — Stub: `selfHostedRunnerMain: (args: string[]) => Promise<void> = () => Promise.resolve()` (auto-generated stub, not yet implemented)
- `E:\Git\claude-code-upstream\src\bridge\bridgeConfig.ts:44` — `isSelfHostedBridge()`: detects self-hosted vs official bridge
- `E:\Git\claude-code-upstream\src\bridge\bridgeEnabled.ts:32,57,82` — Bridge mode gates on `isSelfHostedBridge()`
- `E:\Git\claude-code-upstream\src\bridge\createSession.ts:74-75,213-214,293-294,357-358` — Self-hosted sessions use `orgUUID = 'self-hosted'`
- `E:\Git\claude-code-upstream\src\bridge\initReplBridge.ts:394-395` — Self-hosted bridge initialization
- `E:\Git\claude-code-upstream\src\utils\remoteControlStatus.ts:9-12` — Status display: `Remote Control: self-hosted` vs `official`
- `E:\Git\claude-code-upstream\src\services\api\logging.ts:95` — Gateway detection: distinguishes self-hosted from provider-owned domains
- `E:\Git\claude-code-upstream\src\services\analytics\growthbook.ts:839` — GrowthBook feature flag handling for fork/self-hosted deployments

---

# Part N+1: Context References (@ expansion) — Go vs Upstream


---

### 46.2 Cost Tracking

| # | Aspect | Go | Upstream (`cost-tracker.ts`) | Type |
|---|--------|----|-----------------------------|------|
| 1 | USD cost calculation per API call | Not implemented | `calculateUSDCost` per model (line 278-323) | 缺失 |
| 2 | Per-model usage breakdown | Only totalInputTokens/totalOutputTokens atomic counters (agent_loop.go:311-312) | `ModelUsage` map: input/output/cacheRead/cacheCreation/webSearch/costUSD (line 250-276) | 缺失 |
| 3 | Cache read/creation token tracking | Not implemented | `cache_read_input_tokens`, `cache_creation_input_tokens` (line 294-301) | 缺失 |
| 4 | Web search request cost tracking | Not implemented | `web_search_requests` (line 270-271) | 缺失 |
| 5 | Advisor tool token/cost tracking | Not implemented | `getAdvisorUsage`, recursive `addToTotalSessionCost` (line 304-322) | 缺失 |
| 6 | Session cost persistence | Not implemented | `saveCurrentSessionCosts`, `restoreCostStateForSession` (line 87-175) | 缺失 |
| 7 | Cost display formatting | Not implemented | `formatTotalCost`, `formatCost`, `formatModelUsage` (line 177-248) | 缺失 |
| 8 | OTel cost/token counters | Not implemented | `getCostCounter`, `getTokenCounter` (line 291-301) | 缺失 |
| 9 | Unknown model cost warning | Not implemented | `hasUnknownModelCost` (line 231-233) | 缺失 |
| 10 | Code lines changed tracking | Not implemented | `getTotalLinesAdded`, `getTotalLinesRemoved` (line 19-25) | 缺失 |
| 11 | API duration tracking | Not implemented | `totalAPIDuration`, `totalAPIDurationWithoutRetries` (line 17-18) | 缺失 |
| 12 | Tool duration tracking | Not implemented | `getTotalToolDuration` (line 24) | 缺失 |


---

