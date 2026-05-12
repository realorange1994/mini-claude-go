# Upstream Comparison Documentation

> Go miniClaudeCode vs TypeScript claude-code — gap analysis & refactoring guide

## Quick Start

| Goal | Read |
|------|------|
| Executive overview | [summary.md](summary.md) |
| Must-fix gaps (P0) | [action-items/P0-critical.md](action-items/P0-critical.md) |
| Should-fix gaps (P1) | [action-items/P1-important.md](action-items/P1-important.md) |
| Nice-to-have (P2) | [action-items/P2-nice-to-have.md](action-items/P2-nice-to-have.md) |
| Specific subsystem | See category files below |

## Category Files

| # | File | Scope | Key Gaps |
|---|------|-------|----------|
| 01 | [categories/01-core-agent-loop.md](categories/01-core-agent-loop.md) | Agent loop, context, compaction, streaming | Tool pairing, role alternation, empty message filtering |
| 02 | [categories/02-tools.md](categories/02-tools.md) | All tool implementations | Multi-edit, exec safety, git tool, agent orchestration |
| 03 | [categories/03-system-prompt.md](categories/03-system-prompt.md) | System prompt, permissions, config | Hook types, skill pipeline, permission rules, settings hierarchy |
| 04 | [categories/04-api-client.md](categories/04-api-client.md) | API client, beta headers, model mgmt | Provider routing, model aliases, cost tracking, cache economics |
| 05 | [categories/05-services.md](categories/05-services.md) | OAuth, analytics, telemetry, settings | OAuth/PKCE, Otel/Sentry, GrowthBook flags, multi-source settings |
| 06 | [categories/06-ui-tui.md](categories/06-ui-tui.md) | TUI, components, rendering | Ink/React TUI, keybindings, vim mode, voice input |
| 07 | [categories/07-architecture.md](categories/07-architecture.md) | Cross-cutting patterns, concurrency | Transcript DAG, error classification, normalization pipeline |
| 08 | [categories/08-enhancements.md](categories/08-enhancements.md) | Go-specific enhancements & adaptations | @-references, skill tracker, git tool, file history, Chinese errors |
| 09 | [categories/09-testing.md](categories/09-testing.md) | Testing patterns & coverage gaps | Test framework, snapshot tests, integration coverage |

## Source

All data extracted from [diff_upstream/](../diff_upstream/) (32 分类文件, 原 11735 行拆分重组)。
交叉引用格式: `diff_upstream/XX-category.md` 或原节号 `§N`。

原始完整数据仍保留在 `diff_upstream.md` (11735 行，可作为备份参考)。

## Gap Type Legend

| Symbol | Meaning |
|--------|---------|
| **缺失** | Missing — upstream has it, Go does not |
| **简化** | Simplified — Go has a basic version, upstream is richer |
| **Go增强** | Go enhancement — Go has something upstream lacks |
| **Go适配** | Go adaptation — different but equivalent approach |
| **差异** | Difference — both have it, implemented differently |

## Priority Ordering

缺失 > 简化 > Go增强 — Missing features are highest priority for parity.

## Action Item Summary

| Priority | Total | Done | New | Rework |
|----------|-------|------|-----|--------|
| P0 (CRITICAL) | 13 | 11 | 2 | 0 |
| P1 (IMPORTANT) | 32 | 24 | 12 | 0 |
| P2 (NICE-TO-HAVE) | 30 | 0 | 30 | 0 |
| **Total** | **75** | **33** | **48** | **2** |

## Audit Summary (Rounds 1-21)

| Audit | Count | Items |
|-------|-------|-------|
| **PASS** | 12 | P0-6 (multi-edit match), P0-10 (orphan backfill), P0-11 (stop hooks), P0-13 (permission path safety — precise prefix/component checks + ADS/symlink defense), P1-2 (reactive compaction), P1-4 (model aliases — full [1m] suffix + beta headers + GetModelForAPI), P1-5 (cache detection — category-based tracking with 12 change categories + weights), P1-6 (classifier — removed fabricated escapeContentInjection, added JSONL transcript + XML tags), P1-16 (tool output structured format), P1-17 (exec tool safety), P1-28 (error classification — 15-category enum + recovery hints), P1-31 (MCP schema validation — ValidateSchema with type/enum/constraints) |
| **PARTIAL** | 17 | P0-1, P0-2, P0-3, P0-4, P0-5, P0-7, P0-8, P0-9 (streaming executor), P0-12, P1-1, P1-3, P1-7, P1-9, P1-10, P1-11, P1-12, P1-18 (file read — missing image/PDF support), P1-19 (file write safety — missing large file confirmation), P1-20 (grep/glob — glob lacks type filter), P1-26 (error classification — PARTIAL with P1-28 overlap) |
| **FAIL** | 0 | All FAIL items resolved: P1-4/P1-5 via R10/R11 reworks |
| **Critical issues** | 0 | ApplyPinnedCacheEdits stub fixed (R10), 9 unused hook types wired (R17), P0-10 orphan backfill bug fixed (R19) |

## Engineering Progress

| Round | Fix | Priority | Audit | Status |
|-------|-----|----------|-------|--------|
| 1 | Tool pairing + role alternation + empty message filtering | P0 | PARTIAL | Committed |
| 2 | Multi-edit multiple match check | P0 | PASS | Committed |
| 3 | Classifier fail-closed on parse errors | P0 | PARTIAL | Committed |
| 4 | Cache breakpoint 4→1 | P0 | PARTIAL | Committed |
| 5 | Cost tracking with per-model USD pricing | P1 | PARTIAL | Committed |
| 6 | Reactive compaction with token-gap parsing | P1 | PASS | Committed |
| 7 | 529 model fallback + 429 subscriber gating | P1 | PARTIAL | Committed |
| 8 | Model alias system with default model per tier | P1 | PASS | Committed |
| 9 | Per-model context window | P0 | PARTIAL | Committed |
| 10 | Cache break detection + pinned edits | P1 | PASS | Committed |
| 11 | Classifier improvements (removed fabricated escapeContentInjection, added JSONL transcript + <transcript> tags) | P1 | PASS | Committed |
| 12 | Skill content pipeline (args, variables, paths, MCP discovery) | P1 | PARTIAL | Committed |
| 13 | Hook system expansion (2→16 types, timeout, death spiral) | P1 | PASS | Committed |
| 14 | Normalization pipeline enhancements (attachments, images, PDF, virtual) | P1 | PARTIAL | Committed |
| 15 | Transcript DAG (UUID, parent chain, metadata, interrupt detection) | P1 | PARTIAL | Committed |
| 16 | Agent tool improvements (sync mode, naming, handoff, worktree, sidechain) | P1 | PARTIAL | Committed |
| 17 | Hook wiring (8 unused hook types now invoked in agent loop) | P1 | PASS | Committed |
| 18 | Stop hooks + permission path safety (HookStop at all Run() exit points; precise path checks with ADS/symlink defense) | P0 | PASS | Committed |
| 19 | Orphaned tool_result backfill + streaming tool executor (pipelined tool execution during streaming) | P0 | PARTIAL | Committed |
| 20 | Error classification system (15-category enum with structured recovery hints) | P1 | PARTIAL | Committed |
| 21 | MCP tool schema validation (ValidateSchema with type/enum/constraints) | P1 | PASS | Committed |

## REPL Positioning

Go miniClaudeCode is a **pure REPL CLI tool**, NOT a TUI. See [memory/go_repl_positioning.md](../../.claude/projects/E--workspace/memory/go_repl_positioning.md) for full positioning principles.

| Aspect | Guideline |
|--------|-----------|
| Core logic (agent loop, compaction, permissions, cache, normalization) | Replicate upstream exactly |
| REPL features (input handling, streaming display, notification) | Reference upstream, adapt for CLI |
| TUI features (Bubble Tea, keybinding, vim mode, status bar) | Lowest priority — not engineering focus |
