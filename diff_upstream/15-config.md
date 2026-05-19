# Configuration

> Config system, settings

## Sections Included
- [##] Line 1954-2031 -- ## 13. Config System (config.go vs upstream settings)
- [##] Line 9562-9610 -- ## 37. Config, Permissions, System Prompt Deep Dive
- [###] Line 10261-10278 -- ### 46.5 Settings Management

---

## Content

## 13. Config System (config.go vs upstream settings)

### Files Compared
- **Go**: `E:\Git\miniClaudeCode-go-github\config.go` (404 lines)
- **Upstream**: `E:\Git\claude-code-upstream\src\utils\settings\` (settings.ts, types.ts, constants.ts, settingsCache.ts, managedPath.ts, validation.ts, mdm/settings.ts) — ~3000+ lines

### 13.1 Settings Schema
- **上游**: Full Zod v4 schema (`SettingsSchema`) with 80+ fields: `$schema`, `apiKeyHelper`, `env`, `permissions`, `modelType`, `model`, `availableModels`, `modelOverrides`, `hooks`, `worktree`, `disableAllHooks`, `defaultShell`, `allowManagedHooksOnly`, `allowedHttpHookUrls`, `allowManagedPermissionRulesOnly`, `allowManagedMcpServersOnly`, `strictPluginOnlyCustomization`, `statusLine`, `enabledPlugins`, `extraKnownMarketplaces`, `sandbox`, `feedbackSurveyRate`, `spinnerTipsEnabled`, `outputStyle`, `language`, `alwaysThinkingEnabled`, `effortLevel`, `advisorModel`, `fastMode`, `poorMode`, `agent`, `companyAnnouncements`, `pluginConfigs`, and many more (`types.ts` lines 255-800+)
- **Go版**: `Config` struct with ~50 fields, mostly hardcoded defaults. `ClaudeSettings` struct for JSON parsing with only `env`, `mcp`, `permissions` sections
- **类型**: 简化

### 13.2 Settings Sources
- **上游**: Multi-source settings loading: `policySettings` (managed), `flagSettings`, `userSettings` (~/.claude), `projectSettings` (.claude/), `localSettings` (.claude/settings.local.json), `cliArg`, `session`, `pluginSettings`. Priority-based merging with `mergeWith` (`settings.ts`, `constants.ts`)
- **Go版**: Two sources: project-level `.claude/settings.json` and home `~/.claude/settings.json`. Fallback from project to home. No managed settings, no policy settings, no plugin settings
- **类型**: 简化

### 13.3 Managed Settings
- **上游**: `managed-settings.json` + `managed-settings.d/*.json` drop-in directory. Systemd/sudoers-style drop-in convention. Base file + sorted alphabetical overrides. `getManagedSettingsSyncFromCache` for remote managed settings (`settings.ts` lines 58-121)
- **Go版**: No managed settings support
- **类型**: 缺失

### 13.4 Settings Validation
- **上游**: Zod schema validation with `safeParse`, `filterInvalidPermissionRules`, `formatZodError`, `ValidationError` types. Invalid settings preserved in file but not used. Backward compatibility tests (`validation.ts`, `backward-compatibility.test.ts`)
- **Go版**: No schema validation. JSON unmarshaling only, errors logged but not validated
- **类型**: 简化

### 13.5 Settings Cache
- **上游**: `settingsCache.ts` with per-source caching, session settings cache, plugin settings base cache. `resetSettingsCache` for cache invalidation
- **Go版**: No caching — reads from disk each time
- **类型**: 简化

### 13.6 MDM/Group Policy Settings
- **上游**: `mdm/settings.ts` — macOS MDM (Mobile Device Management) settings via `defaults read`. Windows registry settings via HKCU. Enterprise policy enforcement
- **Go版**: No MDM support
- **类型**: 缺失

### 13.7 Permission Schema
- **上游**: `PermissionsSchema` with `allow`, `deny`, `ask` arrays, `defaultMode` enum, `disableBypassPermissionsMode`, `disableAutoMode`, `additionalDirectories`. Zod-validated (`types.ts` lines 42-85)
- **Go版**: `permissions` section in `ClaudeSettings` with `allow`, `deny`, `ask`. No validation beyond JSON unmarshaling
- **类型**: 简化

### 13.8 MCP Server Configuration
- **上游**: `enabledMcpjsonServers`, `disabledMcpjsonServers`, `allowedMcpServers` (with `serverName`, `serverCommand`, `serverUrl` matching), `deniedMcpServers`, `enableAllProjectMcpServers`. Enterprise allowlist/denylist with wildcard URL support (`types.ts` lines 407-441)
- **Go版**: Basic MCP loading from `.mcp.json` with `command`, `args`, `env`, `url` fields. No allowlist/denylist, no wildcard matching
- **类型**: 简化

### 13.9 Hooks Configuration
- **上游**: Full `HooksSchema` with `AgentHook`, `BashCommandHook`, `HttpHook`, `PromptHook` types. `allowManagedHooksOnly`. `allowedHttpHookUrls`, `httpHookAllowedEnvVars`
- **Go版**: No hooks support in config
- **类型**: 缺失

### 13.10 Plugin/Marketplace System
- **上游**: `enabledPlugins`, `extraKnownMarketplaces`, `strictKnownMarketplaces`, `blockedMarketplaces`, `pluginConfigs`. Marketplace source schemas with version constraints
- **Go版**: No plugin or marketplace system
- **类型**: 缺失

### 13.11 Worktree Configuration
- **上游**: `worktree.symlinkDirectories` and `worktree.sparsePaths` for git worktree optimization
- **Go版**: No worktree configuration
- **类型**: 缺失

### 13.12 Model Configuration
- **上游**: `modelType` (anthropic/openai/gemini/grok), `model`, `availableModels` (allowlist with family prefixes), `modelOverrides` (Bedrock ARN mapping), `alwaysThinkingEnabled`, `effortLevel`, `fastMode`, `poorMode`, `advisorModel`
- **Go版**: `Model` string field only. No model type, no allowlist, no overrides
- **类型**: 简化

### 13.13 Sandbox Configuration
- **上游**: `SandboxSettingsSchema` for sandbox mode configuration
- **Go版**: No sandbox configuration
- **类型**: 缺失

### 13.14 Default Config Values
- **上游**: Defaults come from settings schema `.optional()` with fallbacks in code. GrowthBook feature gates control defaults
- **Go版**: `DefaultConfig()` function with hardcoded defaults: MaxTurns=90, MaxContextMsgs=100, PermissionMode=ask, AutoCompact, MicroCompact, PostCompact settings, etc.
- **类型**: Go适配

---


---

## 37. Config, Permissions, System Prompt Deep Dive

### Files Compared
- **Go**: `config.go`, `permissions.go`, `system_prompt.go`, `auto_classifier.go`, `hooks.go`
- **Upstream**: `utils/config.js`, `utils/permissions/permissions.ts`, `yoloClassifier.ts`, `utils/systemPrompt.ts`, `utils/hooks.ts`

### 37.1 Config System

| # | Aspect | Go (`config.go`) | Upstream (`config.js` + env vars) | Type |
|---|--------|-----------------|----------------------------------|------|
| 1 | Config structure | `LoadConfigFromFile` reads `ClaudeSettings` struct — combines everything into single `Config` (line 149-281) | Spreads config across `getGlobalConfig`, env vars, feature flags | Go适配 (centralized) |
| 2 | Auto-compact threshold | `DefaultConfig()`: fixed `AutoCompactThreshold=0.75` (line 309) | `getAutoCompactThreshold(model)`: dynamically computes `effectiveContextWindow - AUTOCOMPACT_BUFFER_TOKENS` | 简化 |
| 3 | Micro-compact gap | `MicroCompactGapMinutes: 60` — feature not in upstream | No equivalent setting | Go增强 |
| 4 | Post-compact budgets | Both char-based (legacy) and token-based budgets (line 57-60) | Token-based only: `POST_COMPACT_TOKEN_BUDGET = 50_000` | Go适配 (migration path) |

### 37.2 Auto Classifier

| # | Aspect | Go (`auto_classifier.go`) | Upstream (`yoloClassifier.ts`) | Type |
|---|--------|--------------------------|-------------------------------|------|
| 1 | Token budgets | Stage 1: 2112 max_tokens, Stage 2: 6144 max_tokens | Stage 1: 64 max_tokens with stop_sequences, Stage 2: 4096 max_tokens | 简化 |
| 2 | XML classifier | Pattern-based: `AUTO_MODE_SAFE_TOOLS` whitelist, `SAFE_EXEC_PREFIXES` (30+), `DANGEROUS_EXEC_PATTERNS` (18) | 2-stage XML classifier with `toAutoClassifierInput` per-tool encoding | 简化 |
| 3 | Denial tracking | 3-consecutive-denial fallback, 20-total-denial session cap | `localDenialTracking` in `forkedAgent.ts` for async subagents | Go适配 |
| 4 | Windows support | `isDangerousRemovalPath` handles drive roots, protected dirs | Unix-focused with POSIX paths | Go增强 |
| 5 | Classifier cache | 5-minute TTL cache for decisions | Prompt caching with `cache_control` blocks | Go适配 |

### 37.3 Permission System

| # | Aspect | Go (`permissions.go`) | Upstream (`permissions.ts`) | Type |
|---|--------|----------------------|---------------------------|------|
| 1 | Return type | `(bool, string)` from `CanUseTool` | `{ behavior: 'allow' | 'deny' | 'ask', updatedInput? }` structured object | 简化 |
| 2 | Auto-mode classifier | `checkAutoMode` inline with 3-consecutive-denial fallback | `yoloClassifier.ts` (1508 lines) — 2-stage XML with fast/thinking modes | 缺失 |
| 3 | Recent approval tracking | `recentlyApproved` with 2-minute expiry | `localDenialTracking` for subagents | Go适配 |

### 37.4 System Prompt

| # | Aspect | Go (`system_prompt.go`) | Upstream (`systemPrompt.ts`) | Type |
|---|--------|------------------------|---------------------------|------|
| 1 | System prompt building | Monolithic string with static/dynamic split at `<!-- STATIC_PROMPT_END -->` | Array-of-strings with priority-based selection (override > coordinator > agent > custom > default) | Go适配 |
| 2 | Caching | `CachedSystemPrompt` with atomic dirty flags + FNV hash | `cache_control` blocks on system prompt segments at API level | Go适配 |
| 3 | Skill injection | Progressive skill injection with "New This Turn" tracking | Skill discovery via attachments (`skill_discovery`, `skill_listing`) | Go适配 |

### 37.5 Hooks System

| # | Aspect | Go (`hooks.go`) | Upstream (`hooks.ts`) | Type |
|---|--------|----------------|----------------------|------|
| 1 | Hook events | `PreCompact` and `PostCompact` only | Pre-compact, post-compact, session start hooks | 简化 |
| 2 | Sequential execution | Configurable timeout (default 5s), sequential | Async awaited sequentially | 匹配 |
| 3 | Hook results | `PreCompactOutput{CustomInstructions, UserMessage}`, `PostCompactOutput{UserMessage, Attachment}` | `newCustomInstructions`, `userDisplayMessage`, `hookResults` | 匹配 |


---

### 46.5 Settings Management

| # | Aspect | Go (`config.go`) | Upstream (`utils/settings/`) | Type |
|---|--------|-----------------|-----------------------------|------|
| 1 | Settings source precedence | Project → home only, 2 sources (line 147-280) | 5 ordered sources: user → project → local → flag → policy (constants.ts:7-22) | 简化 |
| 2 | Managed/policy settings | Not implemented | `managed-settings.json` + drop-in dir (settings.ts:58-100+) | 缺失 |
| 3 | Remote managed settings sync | Not implemented | `services/remoteManagedSettings/index.ts` | 缺失 |
| 4 | MDM (Mobile Device Management) settings | Not implemented | `utils/settings/mdm/settings.ts`, `mdm/constants.ts` | 缺失 |
| 5 | Zod-validated SettingsSchema | Flat `ClaudeSettings` struct, no validation (line 121-132) | Zod schema with full validation (types.ts:1-80+) | 简化 |
| 6 | Settings cache with change detection | Not implemented | `settingsCache.ts`, `changeDetector.ts` | 缺失 |
| 7 | `--settings` flag support | Not implemented | `flagSettings` (constants.ts:17) | 缺失 |
| 8 | `--setting-sources` CLI flag | Not implemented | Restrict which sources load (constants.ts:128-153) | 缺失 |
| 9 | Settings validation + error reporting | Only `fmt.Fprintf` warnings on parse failure | `validation.ts`, `allErrors.ts` | 缺失 |
| 10 | Global config save | Only reads, never writes global config | `saveGlobalConfig`, `getGlobalConfig` (utils/config.ts) | 缺失 |
| 11 | Remote managed settings security check | Not implemented | `services/remoteManagedSettings/securityCheck.tsx` | 缺失 |

---


---

