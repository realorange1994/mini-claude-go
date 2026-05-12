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
| 01 | [categories/01-core-agent-loop.md](categories/01-core-agent-loop.md) | Agent loop, context, compaction, streaming | Tool pairing, role alternation, reactive compact, interrupt detection |
| 02 | [categories/02-tools.md](categories/02-tools.md) | All tool implementations | Multi-edit, exec safety, git tool, agent orchestration, file history |
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

## Engineering Progress

| Round | Fix | Priority | Status |
|-------|-----|----------|--------|
| 1 | Tool pairing + role alternation + empty message filtering | P0 (CRITICAL) | ✅ Committed |
| 2 | Multi-edit multiple match check | P0 | ✅ Committed |
| 3 | Classifier fail-closed on parse errors | P0 (Security) | ✅ Committed |
| 4 | Cache breakpoint 4→1 | P0 | ✅ Committed |
| 5 | Cost tracking with per-model USD pricing | P1 | ✅ Committed |
| 6 | Reactive compaction with token-gap parsing | P1 | ✅ Committed |
| 7 | 529 model fallback + 429 subscriber gating | P1 | ✅ Committed |
| 8 | Model alias system (sonnet/opus/haiku) | P1 | ✅ Committed |
