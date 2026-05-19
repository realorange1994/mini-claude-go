# API Client

> API client, beta headers, model config

## Sections Included
- [##] Line 10534-10633 -- ## 48. API Client, Beta Headers, Model Management

---

## Content

## 48. API Client, Beta Headers, Model Management

### 48.1 Beta Header System

| # | Aspect | Go | Upstream | Type |
|---|--------|----|----------|------|
| 1 | Beta header constants | **Never defined** — no `CLAUDE_CODE_20250219_BETA_HEADER`, `INTERLEAVED_THINKING_BETA_HEADER`, `CONTEXT_1M_BETA_HEADER` etc. in any Go file | `src/constants/betas.ts:3-28` — 15+ named constants (`claude-code-20250219`, `interleaved-thinking-2025-05-14`, `context-1m-2025-08-07`, `structured-outputs-2025-12-15`, `web-search-2025-03-05`, `redact-thinking-2026-02-12`, etc.) | 缺失 |
| 2 | Per-model beta aggregation (`getAllModelBetas`) | **Not implemented** — no beta header computation at all | `src/utils/betas.ts:232-348` — `getAllModelBetas(model)` memoized, composes betas per model: claude-code header, 1M context, interleaved thinking, redact thinking, context management, structured outputs, token-efficient tools, web search, prompt caching scope, plus `ANTHROPIC_BETAS` env var | 缺失 |
| 3 | Provider-differentiated betas (`getModelBetas`) | **Not implemented** | `src/utils/betas.ts:350-356` — filters out `BEDROCK_EXTRA_PARAMS_HEADERS` for Bedrock; Bedrock betas moved to `extraBodyParams` (`src/services/api/claude.ts:1600-1608`) | 缺失 |
| 4 | `bedrockExtraBodyParamsBetas` | **Not implemented** | `src/utils/betas.ts:358-363` — for Bedrock, betas go into `extraBody.anthropic_beta` instead of HTTP headers | 缺失 |
| 5 | `getMergedBetas` (SDK + model betas) | **Not implemented** | `src/utils/betas.ts:376-407` — merges SDK-provided betas with auto-detected model betas, deduplicates | 缺失 |
| 6 | `filterAllowedSdkBetas` | **Not implemented** | `src/utils/betas.ts:63-84` — validates SDK betas against `ALLOWED_SDK_BETAS` allowlist, warns subscriber users | 缺失 |
| 7 | `shouldIncludeFirstPartyOnlyBetas` | **Not implemented** | `src/utils/betas.ts:213-218` — gates betas to firstParty/foundry providers, respects `CLAUDE_CODE_DISABLE_EXPERIMENTAL_BETAS` | 缺失 |
| 8 | `shouldUseGlobalCacheScope` | **Not implemented** | `src/utils/betas.ts:225-230` — firstParty only, gates global-scope prompt caching | 缺失 |
| 9 | Interleaved thinking beta gate | **Not implemented** | `src/utils/betas.ts:255-259` — `INTERLEAVED_THINKING_BETA_HEADER` added when `modelSupportsISP(model)` and not disabled | 缺失 |
| 10 | Context management beta gate | **Sent via option only** — `option.WithJSONSet("context_management", ...)` in `compact.go:1606-1611` | `src/utils/betas.ts:286-291` — `CONTEXT_MANAGEMENT_BETA_HEADER` + `context_management` body param, gated on `shouldIncludeFirstPartyOnlyBetas()` and model support | 简化 |
| 11 | Redact thinking beta | **Not sent as beta header** — handles `RedactedThinkingBlock` in `streaming.go` but never requests it | `src/utils/betas.ts:268-275` — `REDACT_THINKING_BETA_HEADER` sent when 1P, model supports ISP, interactive session, and `showThinkingSummaries !== true` | 缺失 |
| 12 | Structured outputs beta | **Never sent** — no `STRUCTURED_OUTPUTS_BETA_HEADER` | `src/utils/betas.ts:305-311` — dynamically added when `modelSupportsStructuredOutputs(model)` and `strictToolsEnabled` | 缺失 |
| 13 | Tool search beta (provider-differentiated) | **Not implemented** | `src/utils/betas.ts:200-206` — `getToolSearchBetaHeader()` returns `advanced-tool-use-2025-11-20` for 1P/Foundry, `tool-search-tool-2025-10-19` for Vertex/Bedrock | 缺失 |
| 14 | Token-efficient tools beta | **Not implemented** | `src/utils/betas.ts:316-322` — ant-only, gated on `includeFirstPartyOnlyBetas` and `tokenEfficientToolsEnabled` | 缺失 |
| 15 | Web search beta (Vertex/Foundry) | **Not implemented** | `src/utils/betas.ts:325-331` — Vertex Claude 4.0+ and all Foundry models | 缺失 |
| 16 | Prompt caching scope beta | **Not implemented** | `src/utils/betas.ts:334-336` — `PROMPT_CACHING_SCOPE_BETA_HEADER` sent for 1P when not disabled | 缺失 |
| 17 | Beta header latching (session-stable) | **Not implemented** | `src/services/api/claude.ts:1456-1507` — sticky-on latches for AFK, fast mode, cache editing, thinking clear — prevents mid-session cache key changes | 缺失 |
| 18 | `CLAUDE_CODE_DISABLE_EXPERIMENTAL_BETAS` kill switch | **Not implemented** | `src/constants/betas.ts` + `src/utils/betas.ts:214` — strips all beta schema fields except `name, description, input_schema, cache_control` | 缺失 |
| 19 | Beta header cache clearing on /clear, /compact | **Not implemented** | `src/utils/betas.ts:409-413` — `clearBetasCaches()` clears all memoized beta caches | 缺失 |
| 20 | OAuth beta header | **Not implemented** | `src/constants/oauth.ts` — `OAUTH_BETA_HEADER` sent when user is Claude.ai subscriber | 缺失 |
| 21 | `ANTHROPIC_BETAS` env var passthrough | **Not implemented** | `src/utils/betas.ts:340-346` — splits comma-separated env var and appends to beta list | 缺失 |
| 22 | `BEDROCK_EXTRA_PARAMS_HEADERS` set | **Not implemented** | `src/constants/betas.ts:35-39` — set of betas that Bedrock only supports via extraBodyParams | 缺失 |
| 23 | `VERTEXT_COUNT_TOKENS_ALLOWED_BETAS` set | **Not implemented** | `src/constants/betas.ts:45-49` — limited betas allowed on Vertex countTokens API | 缺失 |

### 48.2 Model Management

| # | Aspect | Go | Upstream | Type |
|---|--------|----|----------|------|
| 1 | Model alias system | **Not implemented** — no `sonnet`, `opus`, `haiku`, `best`, `opusplan` alias resolution | `src/utils/model/aliases.ts:1-14` — 8 aliases + `isModelAlias()` type guard | 缺失 |
| 2 | Model strings per provider | **Not implemented** — no provider-specific model ID mapping | `src/utils/model/configs.ts:9-143` — 12 models × 7 providers (`firstParty`, `bedrock`, `vertex`, `foundry`, `openai`, `gemini`, `grok`) with exact model IDs | 缺失 |
| 3 | Canonical name resolution | **Hardcoded string matching** — `modelContextWindow()` in `compact.go:1138-1152` does ad-hoc `strings.Contains` checks | `src/utils/model/model.ts:262-318` — `firstPartyNameToCanonical()` strips date/provider suffixes, handles 15+ model families, regex fallback | 简化 |
| 4 | `getCanonicalName` with Bedrock ARN resolution | **Not implemented** | `src/utils/model/model.ts:327-331` — resolves Bedrock ARNs via `resolveOverriddenModel()` before canonicalizing | 缺失 |
| 5 | `getModelStrings()` with Bedrock profile discovery | **Not implemented** | `src/utils/model/modelStrings.ts:136-145` — returns provider-specific model IDs; Bedrock dynamically discovers inference profiles | 缺失 |
| 6 | Model overrides from settings | **Not implemented** | `src/utils/model/modelStrings.ts:63-76` — `applyModelOverrides()` applies `settings.json.modelOverrides` | 缺失 |
| 7 | 1M context detection | **Hardcoded** — `modelContextWindow()` in `compact.go:1138-1152` checks `[1m]` suffix + `sonnet-4`/`opus-4-6`/`opus-4-7` string matching, returns 1_000_000 or 200_000 | Multi-source: `[1m]` suffix (`has1mContext`), `modelSupports1M()`, GrowthBook experiment (`coral_reef_sonnet`), API capabilities cache, `CLAUDE_CODE_MAX_CONTEXT_TOKENS` env override, `CLAUDE_CODE_DISABLE_1M_CONTEXT` compliance switch (`src/utils/context.ts:36-103`) | 简化 |
| 8 | Per-model max output tokens | **Not implemented** — uses flat `MaxOutputTokens=16384` / `EscalatedMaxOutputTokens=64000` for all models | `src/utils/context.ts:160-224` — `getModelMaxOutputTokens()`: Opus 4.7=64K/128K, Sonnet 4.6=32K/128K, Opus 4.0/4.1=4K/4K, Claude 3 models=4-8K, etc. | 简化 |
| 9 | Per-model default model selection by subscription tier | **Not implemented** — single default model | `src/utils/model/model.ts:223-245` — Max/Team Premium→Opus, others→Sonnet; `getDefaultOpusModel()`, `getDefaultSonnetModel()`, `getDefaultHaikuModel()` all provider-aware | 缺失 |
| 10 | `modelSupportsStructuredOutputs` | **Not implemented** | `src/utils/betas.ts:139-155` — checks provider (firstParty/Foundry only), then canonical name for `claude-sonnet-4-6/4-5`, `claude-opus-4-1/4-5/4-6/4-7`, `claude-haiku-4-5` | 缺失 |
| 11 | `modelSupportsISP` (interleaved thinking) | **Not implemented** | `src/utils/betas.ts:89-109` — provider-differentiated: Foundry→all, firstParty→not Claude 3, Bedrock/Vertex→Opus 4/Sonnet 4 only | 缺失 |
| 12 | `modelSupportsContextManagement` | **Not implemented** | `src/utils/betas.ts:122-136` — Foundry→all, firstParty→not Claude 3, 3P→Claude 4+ | 缺失 |
| 13 | `modelSupportsAutoMode` | **Partially implemented** — Go has auto mode classifier (`auto_classifier.go`) but no model-gating logic | `src/utils/betas.ts:158-193` — firstParty-only at launch, GrowthBook `allowModels` override, denylist/allowlist by model version, regex-based version gating | 简化 |
| 14 | `get3PModelCapabilityOverride` | **Not implemented** | `src/utils/model/modelSupportOverrides.ts:46-68` — env var overrides for 3P providers (Anthropic/OpenAI default model capabilities) | 缺失 |
| 15 | `getModelCapability` (API-fetched) | **Not implemented** — no `/v1/models` API call | `src/utils/model/modelCapabilities.ts:78-86` — caches `max_input_tokens`/`max_tokens` from `/v1/models` API, disk-cached in `~/.claude/cache/model-capabilities.json` | 缺失 |
| 16 | Public model display names | **Not implemented** | `src/utils/model/model.ts:397-436` — `getPublicModelDisplayName()`: "Opus 4.7", "Sonnet 4.6 (1M context)", etc. | 缺失 |
| 17 | `renderModelName` (masked for ant internal models) | **Not implemented** | `src/utils/model/model.ts:447-467` — public names for known models, masked codenames for ant internal, preserves custom names | 缺失 |
| 18 | `parseUserSpecifiedModel` with alias + [1m] + legacy remap | **Not implemented** | `src/utils/model/model.ts:497-559` — resolves aliases, strips [1m], legacy Opus remap, ant model resolution, case preservation for Foundry deployment IDs | 缺失 |
| 19 | `resolveSkillModelOverride` (1m suffix carry-over) | **Not implemented** | `src/utils/model/model.ts:576-589` — carries [1m] suffix when skill model supports it | 缺失 |
| 20 | `getMarketingNameForModel` | **Not implemented** | `src/utils/model/model.ts:623-670` — canonical→marketing name, 1M-aware | 缺失 |
| 21 | Legacy Opus model remap | **Not implemented** | `src/utils/model/model.ts:591-607` — `claude-opus-4-0/4-1` → current Opus default on firstParty | 缺失 |
| 22 | Model allowlist | **Not implemented** | `src/utils/model/modelAllowlist.ts` + `src/utils/model/antModels.ts` — restricts which models can be used | 缺失 |
| 23 | `normalizeModelStringForAPI` | **Not implemented** | `src/utils/model/model.ts:672-674` — strips `[1m]`/`[2m]` suffixes before sending to API | 缺失 |
| 24 | Context window percentage calculation | **Not implemented** | `src/utils/context.ts:123-155` — `calculateContextPercentages()` computes used/remaining % from token usage | 缺失 |
| 25 | Model-specific thinking budget | **Not implemented** | `src/utils/context.ts:233-235` — `getMaxThinkingTokensForModel()` based on `getModelMaxOutputTokens().upperLimit - 1` | 缺失 |
| 26 | `getSmallFastModel` (provider-aware) | **Not implemented** | `src/utils/model/model.ts:37-48` — returns `OPENAI_SMALL_FAST_MODEL`/`GEMINI_SMALL_FAST_MODEL`/`ANTHROPIC_SMALL_FAST_MODEL` or default Haiku | 缺失 |
| 27 | Provider-specific default models | **Not implemented** | `src/utils/model/model.ts:116-183` — `getDefaultOpusModel()`, `getDefaultSonnetModel()`, `getDefaultHaikuModel()` each check provider-specific env vars | 缺失 |

### 48.3 API Client & Request Construction

| # | Aspect | Go | Upstream | Type |
|---|--------|----|----------|------|
| 1 | API client initialization | `agent_loop.go:366-374` — `anthropic.NewClient()` with `Authorization: Bearer` + custom HTTP client + optional `BaseURL` | `src/services/api/client.ts:84-312` — `getAnthropicClient()` dynamically creates Bedrock/Vertex/Foundry/firstParty clients with provider-specific auth | 简化 |
| 2 | OAuth authentication (Claude.ai subscriber) | **Not implemented** — only API key auth | `src/services/api/client.ts:297-309` — `isClaudeAISubscriber()` → OAuth token via `authToken`/`apiKey: null`, `checkAndRefreshOAuthTokenIfNeeded()` | 缺失 |
| 3 | Bedrock client | **Not implemented** | `src/services/api/client.ts:149-186` — `BedrockClient` with AWS region, credential refresh, `AWS_BEARER_TOKEN_BEDROCK` support | 缺失 |
| 4 | Vertex client | **Not implemented** | `src/services/api/client.ts:217-294` — `AnthropicVertex` with GCP region, `GoogleAuth`, project ID fallback to prevent 12s timeout | 缺失 |
| 5 | Foundry client | **Not implemented** | `src/services/api/client.ts:187-216` — `AnthropicFoundry` with Azure AD token provider, `DefaultAzureCredential` | 缺失 |
| 6 | Default headers | `agent_loop.go:367` — only `Authorization: Bearer` | `src/services/api/client.ts:101-112` — `x-app: cli`, `User-Agent`, `X-Claude-Code-Session-Id`, `x-claude-remote-container-id`, `x-claude-remote-session-id`, `x-client-app` | 简化 |
| 7 | User-Agent format | **Not set** — SDK default used | `src/utils/http.ts:18-35` — `claude-cli/{VERSION} ({USER_TYPE}, {ENTRYPOINT}, agent-sdk/..., client-app/..., workload/...)` | 缺失 |
| 8 | Beta headers in API request | **Never sent** — no `Betas` field in any `MessageNewParams` (`agent_loop.go:1489-1496`, `agent_loop.go:1801-1813`, `compact.go:1588-1600`) | `src/services/api/claude.ts:1770` — `betas: filteredBetas` in every request; includes prompt caching, structured outputs, cache editing, AFK mode, task budgets, context management, fast mode, redact thinking | 缺失 |
| 9 | `extraBodyParams` (CLAUDE_CODE_EXTRA_BODY) | **Not implemented** | `src/services/api/claude.ts:278-323` — `getExtraBodyParams()` merges env var JSON with beta headers into `anthropic_beta` array | 缺失 |
| 10 | `context_management` body parameter | Sent via `option.WithJSONSet()` in `compact.go:1606-1611` only | `src/services/api/claude.ts:1775-1779` — sent conditionally when `CONTEXT_MANAGEMENT_BETA_HEADER` present, includes `clear_tool_uses`, `clear_thinking` | Go适配 |
| 11 | `output_config.format` (structured output) | **Never sent** — no `OutputConfig` field | `src/services/api/claude.ts:1781-1783` — `output_config: outputConfig` with `format: { type: 'json_schema', schema: ... }` | 缺失 |
| 12 | `tool_choice` parameter | **Never sent** — no `ToolChoice` in `MessageNewParams` | `src/services/api/claude.ts:1769` — `tool_choice: options.toolChoice` for deterministic tool forcing in side queries | 缺失 |
| 13 | `metadata` parameter | **Never sent** | `src/services/api/claude.ts:1771` — `metadata: getAPIMetadata()` with `session_id`, `account_uuid` | 缺失 |
| 14 | `thinking` parameter (adaptive/enabled) | `compact.go:1597-1599` — only `Thinking: disabled` for compact calls | `src/services/api/claude.ts:1655-1681` — `thinking: { type: 'adaptive' }` or `{ budget_tokens: N, type: 'enabled' }` based on model capabilities | 简化 |
| 15 | `max_tokens` per model | Flat `16384` / escalated `64000` (`config.go:290,331`) | `src/services/api/claude.ts:1642-1645` — `getMaxOutputTokensForModel()` returns per-model defaults (Opus 4.7=64K, Sonnet 4.6=32K, etc.) | 简化 |
| 16 | `effort` parameter | **Not implemented** | `src/services/api/claude.ts:1614-1620` — `configureEffortParams()` adds effort to `output_config` | 缺失 |
| 17 | `task_budget` parameter | **Not implemented** | `src/services/api/claude.ts:1622-1626` — `configureTaskBudgetParams()` | 缺失 |
| 18 | `speed` (fast mode) parameter | **Not implemented** | `src/services/api/claude.ts:1703-1705,1784` — `speed: 'fast'` when fast mode enabled | 缺失 |
| 19 | Prompt caching (`cache_control`) | Implemented — `cacheMessageParams()` in `prompt_caching.go`, `system_and_3` strategy | Implemented — `addCacheBreakpoints()` in `claude.ts:1758-1766` with global/org/ephemeral cache scopes | 修正 |
| 20 | `cache_edits` (context_management edits) | Implemented — `option.WithJSONSet("context_management", ...)` in `compact.go:1606-1611` | Implemented — `context_management` in `claude.ts:1775-1779` with `clear_tool_uses_20250919` + `clear_thinking_20251015` | 修正 |
| 21 | Auth header: `x-api-key` vs `Bearer` | Always uses `Authorization: Bearer` (`agent_loop.go:367`) | `src/services/api/client.ts:314-324` — uses `x-api-key` for API key users, `Authorization: Bearer` for OAuth subscribers | 简化 |
| 22 | Custom headers (`ANTHROPIC_CUSTOM_HEADERS`) | **Not implemented** | `src/services/api/client.ts:326-350` — parses newline-separated `Name: Value` headers | 缺失 |
| 23 | `x-anthropic-additional-protection` header | **Not implemented** | `src/services/api/client.ts:120-125` — added when `CLAUDE_CODE_ADDITIONAL_PROTECTION=1` | 缺失 |
| 24 | Client request ID header | **Not implemented** | `src/services/api/client.ts:352-385` — `x-client-request-id` with UUID, injected only for 1P | 缺失 |
| 25 | Timeout configuration | `600*time.Second` hardcoded per call (`agent_loop.go:1515`) | `src/services/api/client.ts:140` — `API_TIMEOUT_MS` env var, default 600s | 简化 |
| 26 | Proxy support | `newHTTPClient()` in Go standard library | `src/services/api/client.ts:142-144` — `getProxyFetchOptions()` for proxy configuration | 简化 |
| 27 | `CLAUDE_CODE_USE_BEDROCK` env var detection | **Not implemented** | `src/services/api/client.ts:149` — switches to Bedrock client | 缺失 |
| 28 | `CLAUDE_CODE_USE_VERTEX` env var detection | **Not implemented** | `src/services/api/client.ts:217` — switches to Vertex client | 缺失 |
| 29 | `CLAUDE_CODE_USE_FOUNDRY` env var detection | **Not implemented** | `src/services/api/client.ts:187` — switches to Foundry client | 缺失 |
| 30 | `isFirstPartyAnthropicBaseUrl` | **Not implemented** | `src/utils/model/providers.ts:40-56` — checks if `ANTHROPIC_BASE_URL` is `api.anthropic.com` | 缺失 |
| 31 | Bedrock `extraBodyParams` adapter | **Not implemented** | `src/services/api/claude.ts:1600-1608` — Bedrock betas go into `extraBody.anthropic_beta` array | 缺失 |
| 32 | Cache break detection (prompt state recording) | **Not implemented** | `src/services/api/promptCacheBreakDetection.ts` — records hash of system prompt, tool schemas, cache control, betas, model, effort, extraBody | 缺失 |
| 33 | Per-call beta assembly (`paramsFromContext`) | **Not implemented** — each call site independently constructs params | `src/services/api/claude.ts:1456-1786` — centralized `paramsFromContext()` assembles all betas, thinking, effort, fast mode, cache editing, context management in one place | 简化 |


---

