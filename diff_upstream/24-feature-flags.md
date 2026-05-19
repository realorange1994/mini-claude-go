# Feature Flags

> GrowthBook, feature flags

## Sections Included
- [###] Line 10228-10242 -- ### 46.3 Feature Flags (GrowthBook / Statsig)

---

## Content

### 46.3 Feature Flags (GrowthBook / Statsig)

| # | Aspect | Go | Upstream (`growthbook.ts`) | Type |
|---|--------|----|---------------------------|------|
| 1 | GrowthBook client initialization | Not implemented — no GrowthBook/Statsig dependency | `getGrowthBookClient`, `initializeGrowthBook` (line 565-700) | 缺失 |
| 2 | Remote eval payload processing | Not implemented | `processRemoteEvalPayload` (line 329-394) | 缺失 |
| 3 | GrowthBook disk cache sync | Not implemented | `syncRemoteEvalToDisk` (line 407-417) | 缺失 |
| 4 | Periodic refresh (20min ant / 6h external) | Not implemented | `setupPeriodicGrowthBookRefresh` (line 1119-1231) | 缺失 |
| 5 | Cached flag reads | Not implemented — all features hardcoded | `getFeatureValue_CACHED_MAY_BE_STALE` (line 819-870) | 缺失 |
| 6 | Local gate defaults (70+ feature defaults) | Not implemented | `LOCAL_GATE_DEFAULTS` (line 434-478) | 缺失 |
| 7 | Env-var overrides for features | Not implemented | `CLAUDE_INTERNAL_FC_OVERRIDES` (line 167-202) | 缺失 |
| 8 | Experiment exposure logging | Not implemented | `logExposureForFeature` with dedup (line 296-314) | 缺失 |
| 9 | Post-auth refresh (destroy + recreate client) | Not implemented | `refreshGrowthBookAfterAuthChange` (line 1050-1089) | 缺失 |
| 10 | Dynamic config support | Not implemented | `getDynamicConfig_CACHED_MAY_BE_STALE`, `getDynamicConfig_BLOCKS_ON_INIT` (line 1243-1262) | 缺失 |


---

