# 04 — API Client, Beta Headers & Model Management

> API client implementation, beta header system, model routing, cost tracking, cache economics

## Overview

Go is locked to the Anthropic first-party API. Upstream supports 7 providers with automatic routing, extensive model management, and sophisticated cache economics.

---

## 1. Provider Routing

| Aspect | Go | Upstream | Gap |
|--------|-----|----------|-----|
| Provider types | 1 (Anthropic first-party only) | 7: `firstParty`, `bedrock`, `vertex`, `foundry`, `openai`, `gemini`, `grok` | 缺失 |
| Provider detection | Not applicable — hardcoded | `getAPIProvider()`: checks `modelType` setting + env vars (`CLAUDE_CODE_USE_BEDROCK`, `CLAUDE_CODE_USE_VERTEX`, `CLAUDE_CODE_USE_FOUNDRY`) | 缺失 |
| Per-provider model IDs | Not applicable | Each model mapped to 7 provider-specific IDs (`src/utils/model/configs.ts:9-143` — 12 models × 7 providers) | 缺失 |
| Model selection priority | Single `Config.Model` string loaded from settings/env | 5-level: `/model` command > `--model` flag > `ANTHROPIC_MODEL` env > settings > built-in default (`src/utils/model/model.ts:103-108`) | 缺失 |
| Subscription-aware defaults | Single configurable string | Max/TeamPremium→Opus, others→Sonnet (`src/utils/model/model.ts:223-245`) | 缺失 |
| OpenAI adapter | Not implemented | Full OpenAI-compatible API adapter with request body translation | 缺失 |
| Gemini adapter | Not implemented | Google Gemini API adapter | 缺失 |
| Grok adapter | Not implemented | xAI Grok API adapter | 缺失 |
| Self-hosted detection | Not applicable | `isSelfHostedBridge()` for self-hosted vs official | 缺失 |
| `isFirstPartyAnthropicBaseUrl` | Not implemented | `src/utils/model/providers.ts:40-56` — checks if `ANTHROPIC_BASE_URL` is `api.anthropic.com` | 缺失 |

**Impact**: **Fundamental architectural gap** — Go cannot be used with Bedrock, Vertex, OpenAI, Gemini, or Grok without significant refactoring.

[diff_upstream/25-cost-tracking.md §1]

---

## 2. Beta Header System

| # | Aspect | Go | Upstream | Gap |
|---|--------|----|----------|-----|
| 1 | Beta header constants | **Never defined** — no `CLAUDE_CODE_20250219_BETA_HEADER`, `INTERLEAVED_THINKING_BETA_HEADER`, `CONTEXT_1M_BETA_HEADER` etc. | `src/constants/betas.ts:3-28` — 15+ named constants (`claude-code-20250219`, `interleaved-thinking-2025-05-14`, `context-1m-2025-08-07`, `structured-outputs-2025-12-15`, `web-search-2025-03-05`, `redact-thinking-2026-02-12`, etc.) | 缺失 |
| 2 | Per-model beta aggregation (`getAllModelBetas`) | **Not implemented** — no beta header computation | `src/utils/betas.ts:232-348` — memoized, composes betas per model: claude-code header, 1M context, interleaved thinking, redact thinking, context management, structured outputs, token-efficient tools, web search, prompt caching scope, plus `ANTHROPIC_BETAS` env var | 缺失 |
| 3 | Provider-differentiated betas (`getModelBetas`) | **Not implemented** | `src/utils/betas.ts:350-356` — filters out `BEDROCK_EXTRA_PARAMS_HEADERS` for Bedrock; Bedrock betas moved to `extraBodyParams` (`src/services/api/claude.ts:1600-1608`) | 缺失 |
| 4 | `bedrockExtraBodyParamsBetas` | **Not implemented** | `src/utils/betas.ts:358-363` — for Bedrock, betas go into `extraBody.anthropic_beta` instead of HTTP headers | 缺失 |
| 5 | `getMergedBetas` (SDK + model betas) | **Not implemented** | `src/utils/betas.ts:376-407` — merges SDK-provided betas with auto-detected model betas, deduplicates | 缺失 |
| 6 | `filterAllowedSdkBetas` | **Not implemented** | `src/utils/betas.ts:63-84` — validates SDK betas against `ALLOWED_SDK_BETAS` allowlist, warns subscriber users | 缺失 |
| 7 | `shouldIncludeFirstPartyOnlyBetas` | **Not implemented** | `src/utils/betas.ts:213-218` — gates betas to firstParty/foundry providers, respects `CLAUDE_CODE_DISABLE_EXPERIMENTAL_BETAS` | 缺失 |
| 8 | `shouldUseGlobalCacheScope` | **Not implemented** | `src/utils/betas.ts:225-230` — firstParty only, gates global-scope prompt caching | 缺失 |
| 9 | Interleaved thinking beta gate | **Not implemented** | `src/utils/betas.ts:255-259` — `INTERLEAVED_THINKING_BETA_HEADER` added when `modelSupportsISP(model)` and not disabled | 缺失 |
| 10 | Context management beta gate | **Sent via option only** — `option.WithJSONSet("context_management", ...)` in `compact.go:1606-1611` | `src/utils/betas.ts:286-291` — `CONTEXT_MANAGEMENT_BETA_HEADER` + `context_management` body param, gated on `shouldIncludeFirstPartyOnlyBetas()` and model support | 简化 |
| 11 | Redact thinking beta | **Not sent as beta header** — handles `RedactedThinkingBlock` in `streaming.go` but never requests it | `src/utils/betas.ts:268-275` — `REDACT_THINKING_BETA_HEADER` sent when 1P, model supports ISP, interactive session, and `showThinkingSummaries !== true` | 缺失 |
| 12 | Structured outputs beta | **Never sent** | `src/utils/betas.ts:305-311` — dynamically added when `modelSupportsStructuredOutputs(model)` and `strictToolsEnabled` | 缺失 |
| 13 | Tool search beta (provider-differentiated) | **Not implemented** | `src/utils/betas.ts:200-206` — `advanced-tool-use-2025-11-20` for 1P/Foundry, `tool-search-tool-2025-10-19` for Vertex/Bedrock | 缺失 |
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

**Action**: Implement beta header system. Required for cache control, 128K output, interleaved thinking, and structured outputs.

[diff_upstream/21-api-client.md §48.1]

---

## 3. Model Management

| # | Aspect | Go | Upstream | Gap |
|---|--------|----|----------|-----|
| 1 | Model alias system | **Not implemented** — no `sonnet`, `opus`, `haiku`, `best`, `opusplan` alias resolution | 8 aliases + `isModelAlias()` type guard (`src/utils/model/aliases.ts:1-14`); `sonnet`→`getDefaultSonnetModel()`, `opus`→`getDefaultOpusModel()`, `haiku`→`getDefaultHaikuModel()`, `best`→Opus, `opusplan`→Sonnet normally, Opus in plan mode | 缺失 |
| 2 | Model strings per provider | **Not implemented** — no provider-specific model ID mapping | `src/utils/model/configs.ts:9-143` — 12 models × 7 providers with exact model IDs | 缺失 |
| 3 | Canonical name resolution | **Hardcoded string matching** — `modelContextWindow()` in `compact.go:1138-1152` does ad-hoc `strings.Contains` checks | `src/utils/model/model.ts:262-318` — `firstPartyNameToCanonical()` strips date/provider suffixes, handles 15+ model families, regex fallback | 简化 |
| 4 | `getCanonicalName` with Bedrock ARN resolution | **Not implemented** | `src/utils/model/model.ts:327-331` — resolves Bedrock ARNs via `resolveOverriddenModel()` before canonicalizing | 缺失 |
| 5 | `getModelStrings()` with Bedrock profile discovery | **Not implemented** | `src/utils/model/modelStrings.ts:136-145` — returns provider-specific model IDs; Bedrock dynamically discovers inference profiles | 缺失 |
| 6 | Model overrides from settings | **Not implemented** | `src/utils/model/modelStrings.ts:63-76` — `applyModelOverrides()` applies `settings.json.modelOverrides` | 缺失 |
| 7 | 1M context detection | **Hardcoded** — `[1m]` suffix + `sonnet-4`/`opus-4-6`/`opus-4-7` string matching | Multi-source: `[1m]` suffix, `modelSupports1M()`, GrowthBook experiment, API capabilities cache, `CLAUDE_CODE_MAX_CONTEXT_TOKENS` env override, `CLAUDE_CODE_DISABLE_1M_CONTEXT` compliance switch (`src/utils/context.ts:36-103`) | 简化 |
| 8 | Per-model max output tokens | Flat `16384` / escalated `64000` for all models | `src/utils/context.ts:160-224` — Opus 4.7=64K/128K, Sonnet 4.6=32K/128K, Opus 4.5/Sonnet 4/Haiku 4=32K/64K, Opus 4/4.1=32K, Claude 3 models=4-8K | 简化 |
| 9 | Per-model default by subscription tier | **Not implemented** — single default model | `src/utils/model/model.ts:223-245` — Max/Team Premium→Opus, others→Sonnet; provider-aware `getDefaultOpusModel()`, `getDefaultSonnetModel()`, `getDefaultHaikuModel()` | 缺失 |
| 10 | `modelSupportsStructuredOutputs` | **Not implemented** | `src/utils/betas.ts:139-155` — checks provider (1P/Foundry only), then canonical name | 缺失 |
| 11 | `modelSupportsISP` (interleaved thinking) | **Not implemented** | `src/utils/betas.ts:89-109` — Foundry→all, firstParty→not Claude 3, Bedrock/Vertex→Opus 4/Sonnet 4 only | 缺失 |
| 12 | `modelSupportsContextManagement` | **Not implemented** | `src/utils/betas.ts:122-136` — Foundry→all, firstParty→not Claude 3, 3P→Claude 4+ | 缺失 |
| 13 | `modelSupportsAutoMode` | **Partially implemented** — Go has auto mode classifier (`auto_classifier.go`) but no model-gating logic | `src/utils/betas.ts:158-193` — firstParty-only at launch, GrowthBook `allowModels` override, denylist/allowlist by model version | 简化 |
| 14 | `get3PModelCapabilityOverride` | **Not implemented** | `src/utils/model/modelSupportOverrides.ts:46-68` — env var overrides for 3P providers | 缺失 |
| 15 | `getModelCapability` (API-fetched) | **Not implemented** — no `/v1/models` API call | `src/utils/model/modelCapabilities.ts:78-86` — caches `max_input_tokens`/`max_tokens` from `/v1/models` API, disk-cached in `~/.claude/cache/model-capabilities.json` | 缺失 |
| 16 | Public model display names | **Not implemented** | `src/utils/model/model.ts:397-436` — "Opus 4.7", "Sonnet 4.6 (1M context)", etc. | 缺失 |
| 17 | `renderModelName` (masked for ant internal) | **Not implemented** | `src/utils/model/model.ts:447-467` — public names for known, masked codenames for ant internal, preserves custom names | 缺失 |
| 18 | `parseUserSpecifiedModel` with alias + [1m] + legacy remap | **Not implemented** | `src/utils/model/model.ts:497-559` — resolves aliases, strips [1m], legacy Opus remap, case preservation for Foundry deployment IDs | 缺失 |
| 19 | `resolveSkillModelOverride` (1m suffix carry-over) | **Not implemented** | `src/utils/model/model.ts:576-589` — carries [1m] suffix when skill model supports it | 缺失 |
| 20 | `getMarketingNameForModel` | **Not implemented** | `src/utils/model/model.ts:623-670` — canonical→marketing name, 1M-aware | 缺失 |
| 21 | Legacy Opus model remap | **Not implemented** | `src/utils/model/model.ts:591-607` — `claude-opus-4-0/4-1` → current Opus default on firstParty; configurable via `CLAUDE_CODE_DISABLE_LEGACY_MODEL_REMAP` | 缺失 |
| 22 | Model allowlist | **Not implemented** | `src/utils/model/modelAllowlist.ts` + `src/utils/model/antModels.ts` — multi-tier matching (family alias, version prefix, exact ID, bidirectional resolution) | 缺失 |
| 23 | `normalizeModelStringForAPI` | **Not implemented** | `src/utils/model/model.ts:672-674` — strips `[1m]`/`[2m]` suffixes before sending to API | 缺失 |
| 24 | Context window percentage calculation | **Not implemented** | `src/utils/context.ts:123-155` — `calculateContextPercentages()` computes used/remaining % | 缺失 |
| 25 | Model-specific thinking budget | **Not implemented** | `src/utils/context.ts:233-235` — `getMaxThinkingTokensForModel()` based on output limits | 缺失 |
| 26 | `getSmallFastModel` (provider-aware) | **Not implemented** | `src/utils/model/model.ts:37-48` — returns `OPENAI_SMALL_FAST_MODEL`/`GEMINI_SMALL_FAST_MODEL`/`ANTHROPIC_SMALL_FAST_MODEL` or default Haiku | 缺失 |
| 27 | Provider-specific default models | **Not implemented** | `src/utils/model/model.ts:116-183` — each default model function checks provider-specific env vars | 缺失 |
| 28 | Fast mode | Not supported | `speed: 'fast'` in API, 6x pricing, org gating, cooldown | 缺失 |
| 29 | Poor mode | Not supported | Skips extract_memories/prompt_suggestion to reduce tokens | 缺失 |
| 30 | Effort level | Not supported | `effortLevel` per model controls `budget_tokens` for thinking | 缺失 |
| 31 | Always-on thinking | Global enable/disable | Per-model: some models reject `thinking: disabled`, must use adaptive | 缺失 |
| 32 | Model deprecation | Not supported | Warns on deprecated models, suggests migration paths | 缺失 |

**Action**: Add model alias system (`sonnet`, `opus`, `haiku`). Add subscription-aware defaults. Add provider-specific model ID mapping.

[diff_upstream/21-api-client.md §48.2] [diff_upstream/25-cost-tracking.md §5,6]

---

## 4. Context Window Per Model

| # | Aspect | Go | Upstream | Gap |
|---|--------|----|----------|-----|
| 1 | Default window | 200K for all models | 200K default, but per-model limits via `/v1/models` API | 缺失 |
| 2 | 1M context | `[1m]` suffix + hardcoded model matching (`compact.go:1138-1152`) | Multi-source resolution (`src/utils/context.ts:56-103`): `CLAUDE_CODE_MAX_CONTEXT_TOKENS` env → `[1m]` suffix → API-fetched `max_input_tokens` → beta header + `modelSupports1M()` → GrowthBook sonnet 1M experiment → ant model config → 200K default | 简化 |
| 3 | Dynamic capabilities | Not supported | `refreshModelCapabilities()` fetches from `/v1/models` API, caches to `~/.claude/cache/model-capabilities.json` (`modelCapabilities.ts:88-121`) | 缺失 |
| 4 | Compliance disable | Not supported | `CLAUDE_CODE_DISABLE_1M_CONTEXT` for C4E/HIPAA (`context.ts:32-34`) | 缺失 |
| 5 | Max output tokens | Global `MaxOutputTokens=16384` + `EscalatedMaxOutputTokens=64000` | Per-model: Opus 4.6/4.7→64K/128K, Sonnet 4.6→32K/128K, Opus 4.5/Sonnet 4/Haiku 4→32K/64K, Opus 4/4.1→32K, Claude 3 Opus→4K, Claude 3 Sonnet→8K, Claude 3 Haiku→4K, 3.5→8K, 3.7→32K/64K (`context.ts:160-224`) | 简化 |
| 6 | Max thinking tokens | Not computed per-model | `getMaxThinkingTokensForModel()` derived from `getModelMaxOutputTokens().upperLimit - 1` (`context.ts:233-235`) | 缺失 |
| 7 | Sonnet 1M experiment treatment | Not applicable | `getSonnet1mExpTreatmentEnabled()` GrowthBook experiment (`context.ts:93-117`) | 缺失 |
| 8 | Context window resolution with 7 sources | Single `modelContextWindow()` with string matching | `getContextWindowForModel()` with 7-level cascade (`context.ts:56-103`) | 简化 |

**Action**: Add dynamic model capability fetching. Add per-model max output tokens. Add multi-source 1M context resolution.

[diff_upstream/25-cost-tracking.md §3]

---

## 5. Cache Economics

| Aspect | Go | Upstream | Gap |
|--------|-----|----------|-----|
| Breakpoint strategy | 4 breakpoints (system + last 3 non-system) | Exactly 1 breakpoint (optimized for Mycro turn-to-turn eviction) | 简化 |
| Cache token tracking | Not tracked | `cache_creation_input_tokens`, `cache_read_input_tokens` tracked | 缺失 |
| Cache break detection | Not supported | 2-phase: pre-call state hash → post-call read delta (>5% drop = break) (`src/services/api/promptCacheBreakDetection.ts`) | 缺失 |
| Pinned cache edits | `pinnedEdits` declared but never populated | Re-inserts at original positions on subsequent calls | 缺失 |
| Cache edit field naming | `tool_use_id` | `cache_reference` (upstream API field) | 简化 |
| Cache scope | Not supported | `scope: 'global'\|'org'` for static system prompt parts | 缺失 |
| Cache TTL | "5m" default, "1h" option | GrowthBook-gated 1h eligibility | 简化 |
| Cache sharing for forks | Not supported | `CacheSafeParams` saves/restores for fork cache hits | 缺失 |
| skipCacheWrite | Not supported | Shifts marker to second-to-last for fire-and-forget forks | 缺失 |
| Compaction awareness | No notification on compaction | `notifyCompaction()` resets baseline | 缺失 |
| Edit deduplication | Not implemented | `deduplicateEdits()` prevents duplicate deletions | 缺失 |

**Action**: Change cache breakpoint count from 4 to 1. Add cache token tracking. Add cache break detection.

---

## 6. Cost Tracking

| # | Aspect | Go | Upstream | Gap |
|---|--------|----|----------|-----|
| 1 | USD cost calculation per API call | Not implemented — only raw token counts | `calculateUSDCost` per model (`modelCost.ts:177-180`): `(input/1M)*inputRate + (output/1M)*outputRate + (cache_read/1M)*cacheReadRate + (cache_write/1M)*cacheWriteRate + (web_searches)*$0.01` | 缺失 |
| 2 | Model pricing tiers | None | 6 tiers: Sonnet ($3/$15), Opus 4/4.1 ($15/$75), Opus 4.5/4.6 ($5/$25), Fast ($30/$150), Haiku 3.5 ($0.80/$4), Haiku 4.5 ($1/$5); plus cache pricing per tier (`modelCost.ts:1-232`) | 缺失 |
| 3 | Per-model usage breakdown | Only totalInputTokens/totalOutputTokens atomic counters (`agent_loop.go:311-312`) | `ModelUsage` map: input/output/cacheRead/cacheCreation/webSearch/costUSD (`cost-tracker.ts:250-276`) | 缺失 |
| 4 | Cache token costs | None | Separate pricing for cache_write and cache_read per model | 缺失 |
| 5 | Web search costs | None | $0.01 per request (`modelCost.ts:94-99`) | 缺失 |
| 6 | Web search request tracking | Not implemented | `web_search_requests` count (`cost-tracker.ts:270-271`) | 缺失 |
| 7 | Advisor tool token/cost tracking | Not implemented | `getAdvisorUsage`, recursive `addToTotalSessionCost` (`cost-tracker.ts:304-322`) | 缺失 |
| 8 | Session cost persistence | None | `saveCurrentSessionCosts()` persists to project config JSON; `restoreCostStateForSession()` restores on resume (`cost-tracker.ts:87-175`) | 缺失 |
| 9 | Cost display formatting | Not implemented | `formatTotalCost`, `formatCost`, `formatModelUsage` — aggregates by short name, displays `$cost` per model (`cost-tracker.ts:181-248`) | 缺失 |
| 10 | Display | Basic token counter | Rich breakdown: total cost, API duration, wall duration, per-model cost, code lines changed | 缺失 |
| 11 | OTel cost/token counters | Not implemented | `getCostCounter`, `getTokenCounter` (`cost-tracker.ts:291-301`) | 缺失 |
| 12 | Unknown model cost warning | Not implemented | `hasUnknownModelCost` (`cost-tracker.ts:231-233`) | 缺失 |
| 13 | Code lines changed tracking | Not implemented | `getTotalLinesAdded`, `getTotalLinesRemoved` (`cost-tracker.ts:19-25`) | 缺失 |
| 14 | API duration tracking | Not implemented | `totalAPIDuration`, `totalAPIDurationWithoutRetries` (`cost-tracker.ts:17-18`) | 缺失 |
| 15 | Tool duration tracking | Not implemented | `getTotalToolDuration` (`cost-tracker.ts:24`) | 缺失 |
| 16 | `useCostSummary` React hook | Not applicable (headless CLI) | Prints cost on exit and saves to config (`costHook.ts:1-22`) | 缺失 |

**Action**: Add basic cost tracking with per-model pricing tiers. Add session cost persistence. Add duration tracking.

[diff_upstream/25-cost-tracking.md §4, 46.2]

---

## 7. Model Fallback

| # | Aspect | Go | Upstream | Gap |
|---|--------|----|----------|-----|
| 1 | Model fallback on overload | Not supported — 429/529 classified but no model switch | `FallbackTriggeredError` switches model on 529 exhaustion (`src/services/api/withRetry.ts:130,160-168`); `query.ts:941-993` catches it, updates `currentModel`, notifies user | 缺失 |
| 2 | 529 model fallback | Treated as `ECOverloaded`, retries until maxRetries | `MAX_529_RETRIES = 3` → Opus-to-Sonnet fallback | 缺失 |
| 3 | 429 subscriber gating | Always retries | ClaudeAI subscribers don't retry 429; enterprise subscribers do | 缺失 |
| 4 | Bedrock auth recovery | Not handled | `isBedrockAuthError` + `clearAwsCredentialsCache` (`auth.ts:604-808`) | 缺失 |
| 5 | Vertex auth recovery | Not handled | `gcpAuthRefresh` + credential check (`auth.ts:813-1010`) | 缺失 |
| 6 | x-should-retry header | Not parsed | Parsed and respected | 缺失 |
| 7 | Fast mode fallback | Not applicable | Short retry → fast speed; long retry → 30-min cooldown to standard | 缺失 |
| 8 | `fallbackModel` threading | Not implemented | Threaded through all message/stream creation calls (`claude.ts:700,845,1896,2622,2721`) | 缺失 |

**Action**: Add 529 model fallback (Opus → Sonnet after 3 consecutive). Add subscriber-aware 429 gating.

[diff_upstream/25-cost-tracking.md §2]

---

## 8. API Client & Request Construction

| # | Aspect | Go | Upstream | Gap |
|---|--------|----|----------|------|
| 1 | API client initialization | `agent_loop.go:366-374` — `anthropic.NewClient()` with `Authorization: Bearer` + custom HTTP client + optional `BaseURL` | `src/services/api/client.ts:84-312` — `getAnthropicClient()` dynamically creates Bedrock/Vertex/Foundry/firstParty clients with provider-specific auth | 简化 |
| 2 | OAuth authentication | **Not implemented** — only API key auth | `src/services/api/client.ts:297-309` — `isClaudeAISubscriber()` → OAuth token, `checkAndRefreshOAuthTokenIfNeeded()` | 缺失 |
| 3 | Bedrock client | **Not implemented** | `src/services/api/client.ts:149-186` — `BedrockClient` with AWS region, credential refresh, `AWS_BEARER_TOKEN_BEDROCK` | 缺失 |
| 4 | Vertex client | **Not implemented** | `src/services/api/client.ts:217-294` — `AnthropicVertex` with GCP region, `GoogleAuth`, project ID fallback | 缺失 |
| 5 | Foundry client | **Not implemented** | `src/services/api/client.ts:187-216` — `AnthropicFoundry` with Azure AD token provider | 缺失 |
| 6 | Default headers | `agent_loop.go:367` — only `Authorization: Bearer` | `src/services/api/client.ts:101-112` — `x-app: cli`, `User-Agent`, `X-Claude-Code-Session-Id`, `x-claude-remote-container-id`, `x-claude-remote-session-id`, `x-client-app` | 简化 |
| 7 | User-Agent format | **Not set** — SDK default used | `src/utils/http.ts:18-35` — `claude-cli/{VERSION} ({USER_TYPE}, {ENTRYPOINT}, agent-sdk/..., client-app/..., workload/...)` | 缺失 |
| 8 | Beta headers in API request | **Never sent** — no `Betas` field in any `MessageNewParams` | `src/services/api/claude.ts:1770` — `betas: filteredBetas` in every request | 缺失 |
| 9 | `extraBodyParams` (`CLAUDE_CODE_EXTRA_BODY`) | **Not implemented** | `src/services/api/claude.ts:278-323` — `getExtraBodyParams()` merges env var JSON with beta headers into `anthropic_beta` array | 缺失 |
| 10 | `context_management` body parameter | Sent via `option.WithJSONSet()` in `compact.go:1606-1611` only | `src/services/api/claude.ts:1775-1779` — sent conditionally when `CONTEXT_MANAGEMENT_BETA_HEADER` present, includes `clear_tool_uses`, `clear_thinking` | Go适配 |
| 11 | `output_config.format` (structured output) | **Never sent** | `src/services/api/claude.ts:1781-1783` — `output_config: outputConfig` with `format: { type: 'json_schema', schema: ... }` | 缺失 |
| 12 | `tool_choice` parameter | **Never sent** | `src/services/api/claude.ts:1769` — `tool_choice: options.toolChoice` for deterministic tool forcing | 缺失 |
| 13 | `metadata` parameter | **Never sent** | `src/services/api/claude.ts:1771` — `metadata: getAPIMetadata()` with `session_id`, `account_uuid` | 缺失 |
| 14 | `thinking` parameter | Only `Thinking: disabled` for compact calls (`compact.go:1597-1599`) | `src/services/api/claude.ts:1655-1681` — `thinking: { type: 'adaptive' }` or `{ budget_tokens: N, type: 'enabled' }` based on model capabilities | 简化 |
| 15 | `max_tokens` per model | Flat `16384` / escalated `64000` | `src/services/api/claude.ts:1642-1645` — `getMaxOutputTokensForModel()` returns per-model defaults | 简化 |
| 16 | `effort` parameter | **Not implemented** | `src/services/api/claude.ts:1614-1620` — `configureEffortParams()` adds effort to `output_config` | 缺失 |
| 17 | `task-budget` parameter | **Not implemented** | `src/services/api/claude.ts:1622-1626` — `configureTaskBudgetParams()` | 缺失 |
| 18 | `speed` (fast mode) parameter | **Not implemented** | `src/services/api/claude.ts:1703-1705,1784` — `speed: 'fast'` when fast mode enabled | 缺失 |
| 19 | Auth header: `x-api-key` vs `Bearer` | Always `Authorization: Bearer` | `src/services/api/client.ts:314-324` — `x-api-key` for API key users, `Authorization: Bearer` for OAuth subscribers | 简化 |
| 20 | Custom headers (`ANTHROPIC_CUSTOM_HEADERS`) | **Not implemented** | `src/services/api/client.ts:326-350` — parses newline-separated `Name: Value` headers | 缺失 |
| 21 | `x-anthropic-additional-protection` header | **Not implemented** | `src/services/api/client.ts:120-125` — added when `CLAUDE_CODE_ADDITIONAL_PROTECTION=1` | 缺失 |
| 22 | Client request ID header | **Not implemented** | `src/services/api/client.ts:352-385` — `x-client-request-id` with UUID, injected only for 1P | 缺失 |
| 23 | Timeout configuration | `600*time.Second` hardcoded per call | `src/services/api/client.ts:140` — `API_TIMEOUT_MS` env var, default 600s | 简化 |
| 24 | Proxy support | `newHTTPClient()` in Go standard library | `src/services/api/client.ts:142-144` — `getProxyFetchOptions()` | 简化 |
| 25 | Bedrock `extraBodyParams` adapter | **Not implemented** | `src/services/api/claude.ts:1600-1608` — Bedrock betas go into `extraBody.anthropic_beta` array | 缺失 |
| 26 | Cache break detection (prompt state recording) | **Not implemented** | `src/services/api/promptCacheBreakDetection.ts` — records hash of system prompt, tool schemas, cache control, betas, model, effort, extraBody | 缺失 |
| 27 | Per-call beta assembly (`paramsFromContext`) | **Not implemented** — each call site independently constructs params | `src/services/api/claude.ts:1456-1786` — centralized `paramsFromContext()` assembles all betas, thinking, effort, fast mode, cache editing, context management in one place | 简化 |
| 28 | Non-streaming fallback path | Not implemented | `src/services/api/claude.ts:837-899` | 缺失 |
| 29 | Tool deferral (LSP pending init) | Not implemented | `src/services/api/claude.ts:805-812` | 缺失 |
| 30 | `verifyApiKey` function | Not implemented | `src/services/api/claude.ts:522-577` | 缺失 |
| 31 | Session activity tracking | Not implemented | `src/services/api/claude.ts:211-213` | 缺失 |
| 32 | Query profiler | Not implemented | `src/services/api/claude.ts:183` | 缺失 |
| 33 | Langfuse integration at API level | Not implemented | `src/services/api/claude.ts:233-235` | 缺失 |

[diff_upstream/21-api-client.md §48.3]

---

## 9. Background Services

### 9.1 Daemon Service

| Aspect | Go | Upstream | Gap |
|--------|-----|----------|-----|
| Daemon process | None | Supervisor managing long-running workers (`daemon/main.ts:52-101`) | 缺失 |
| Worker types | None | `remoteControl` worker for headless bridge sessions | 缺失 |
| Crash recovery | None | Exponential backoff: 2s initial, 120s cap, 2x multiplier (`daemon/main.ts:230-312`) | 缺失 |
| Rapid failure parking | None | Parks after 5 failures in <10s | 缺失 |
| REPL commands | None | `/daemon status/start/stop/bg/attach/logs/kill` | 缺失 |
| State persistence | None | `writeDaemonState`, `readDaemonState`, `removeDaemonState`, `queryDaemonStatus` (`daemon/state.ts:48-52`) | 缺失 |
| Worker spawn | None | `spawnWorker()` with `DAEMON_WORKER_*` env vars, pipes stdout/stderr | 缺失 |
| Permanent exit code | None | `EXIT_CODE_PERMANENT = 78` for non-retryable failure | 缺失 |

### 9.2 Job System

| Aspect | Go | Upstream | Gap |
|--------|-----|----------|-----|
| Job classification | None | `classifyAndWriteState(jobDir, turnMessages)` — classifies job status from assistant messages (`jobs/classifier.ts`) | 缺失 |
| Job state management | None | Read/write `state.json` in job directory (`jobs/state.ts`) | 缺失 |
| Job templates | None | Templates for dispatched executions (`jobs/templates.ts`) | 缺失 |
| Template job classification | None | When `CLAUDE_JOB_DIR` is set, classifies turn and writes state (`query/stopHooks.ts:46,100-121`) | 缺失 |
| Background job startup | None | `setup_background_jobs_starting` / `setup_background_jobs_launched` log events | 缺失 |
| Plugin autoupdate | None | Plugin autoupdate runs as background job during startup | 缺失 |

### 9.3 Proactive Features

| Aspect | Go | Upstream | Gap |
|--------|-----|----------|-----|
| Proactive mode | None | Tick-driven autonomous agent with state machine (`inactive→active→paused`) (`proactive/index.ts:1-136`) | 缺失 |
| Tick scheduling | None | `shouldTick()`: active && !paused && !contextBlocked | 缺失 |
| Context blocking | None | `setContextBlocked()` prevents tick→error→tick runaway | 缺失 |
| Proactive section in system prompt | None | `getProactiveSection()` injected when active (`constants/prompts.ts:942-956`) | 缺失 |

### 9.4 Self-Hosted Runner

| Aspect | Go | Upstream | Gap |
|--------|-----|----------|-----|
| Self-hosted runner | None | Stub: `selfHostedRunnerMain: () => Promise.resolve()` (not yet implemented) (`self-hosted-runner/main.ts:1-3`) | 缺失 |
| Self-hosted bridge detection | None | `isSelfHostedBridge()` for self-hosted vs official bridge (`bridge/bridgeConfig.ts:44`) | 缺失 |
| Self-hosted sessions | None | Self-hosted sessions use `orgUUID = 'self-hosted'` (`bridge/createSession.ts`) | 缺失 |

**Impact**: Daemon/Job/Proactive features are niche for headless CLI. Lower priority for CLI users.

[diff_upstream/25-cost-tracking.md §7,8,9,10]

---

## Cross-References

- Retry/rate limiting: [07-architecture.md](07-architecture.md) §2
- OAuth auth details: [05-services.md](05-services.md) §1
- Cost display in TUI: [06-ui-tui.md](06-ui-tui.md) §6
- Permission scoping: [03-system-prompt.md](03-system-prompt.md) §3
- Feature flag usage in compaction: [01-core-agent-loop.md](01-core-agent-loop.md) §3
