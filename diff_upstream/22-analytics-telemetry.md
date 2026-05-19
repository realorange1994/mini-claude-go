# Analytics & Telemetry

> Analytics, telemetry, Langfuse, OTel

## Sections Included
- [##] Line 10185-10186 -- ## 46. Analytics/Telemetry, Cost Tracking, Feature Flags, OAuth, Settings
- [###] Line 10187-10190 -- ### Files Compared
- [###] Line 10191-10210 -- ### 46.1 Analytics / Telemetry System

---

## Content

## 46. Analytics/Telemetry, Cost Tracking, Feature Flags, OAuth, Settings


---

### Files Compared
- **Go**: No analytics, cost tracking, feature flags, OAuth, or settings management modules
- **Upstream**: `services/analytics/`, `utils/telemetry/`, `cost-tracker.ts`, `services/analytics/growthbook.ts`, `utils/auth.ts`, `utils/settings/`


---

### 46.1 Analytics / Telemetry System

| # | Aspect | Go | Upstream | Type |
|---|--------|----|----------|------|
| 1 | OpenTelemetry instrumentation | Not implemented | `utils/telemetry/instrumentation.ts:87-706` — bootstrapTelemetry, initializeTelemetry | 缺失 |
| 2 | OTel event logging | Not implemented | `utils/telemetry/events.ts:21-75` — logOTelEvent with event sequence, prompt ID, workspace | 缺失 |
| 3 | OTel session tracing | Not implemented | `utils/telemetry/sessionTracing.ts:1-928` — startInteractionSpan, startLLMRequestSpan, startToolSpan | 缺失 |
| 4 | OTel telemetry attributes | Not implemented | `utils/telemetry/telemetryAttributes.ts:29-71` — user.id, session.id, organization.id, terminal.type | 缺失 |
| 5 | BigQuery internal metrics exporter | Not implemented | `utils/telemetry/bigqueryExporter.ts:40-252` | 缺失 |
| 6 | Perfetto tracing (Chrome Trace Event format) | Not implemented | `utils/telemetry/perfettoTracing.ts` | 缺失 |
| 7 | 1st-party event logger (batched /api/event_logging/batch) | Not implemented | `services/analytics/firstPartyEventLogger.ts:1-451` — logEventTo1P, initialize1PEventLogging, event sampling | 缺失 |
| 8 | Analytics event routing sink (Datadog + 1P fanout) | Not implemented | `services/analytics/index.ts:1-80+` — logEvent, AnalyticsSink | 缺失 |
| 9 | Analytics config (isAnalyticsDisabled, privacy gating) | Not implemented | `services/analytics/config.ts:19-27` | 缺失 |
| 10 | Per-sink analytics killswitch (GrowthBook-driven) | Not implemented | `services/analytics/sinkKillswitch.ts:18-25` | 缺失 |
| 11 | Event metadata enrichment (model, session, env, betas) | Not implemented | `services/analytics/metadata.ts` | 缺失 |
| 12 | Privacy level system (default / no-telemetry / essential-traffic) | Not implemented | `utils/privacyLevel.ts:17-44` | 缺失 |
| 13 | Langfuse tracing integration | Not implemented | `services/langfuse/client.ts:13-40+` | 缺失 |
| 14 | User prompt redaction | Not implemented | `utils/telemetry/events.ts:17-19` — redactIfDisabled, OTEL_LOG_USER_PROMPTS | 缺失 |
| 15 | Telemetry shutdown + flush | Not implemented | `utils/telemetry/instrumentation.ts:658-753` | 缺失 |


---

