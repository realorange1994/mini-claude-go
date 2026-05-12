# P2 — Nice-to-Have Gaps

> Quality-of-life improvements, architectural features, and long-term goals
> Updated: 2026-05-12 (4 items implemented: idle timeout, graceful shutdown, doctor, atomic writes)

These gaps would improve the user experience or enable future capabilities but are not blocking current functionality.

**REPL Positioning Note**: Go miniClaudeCode is a pure REPL CLI tool, NOT a TUI. Items tagged [TUI] require a terminal UI framework and are lowest priority. Items tagged [REPL] should reference upstream ideas but adapt for CLI. Items tagged [N/A] are core logic that should replicate upstream.

---

## P2-1: OAuth/PKCE Authentication

| Field | Value |
|-------|-------|
| Gap type | 缺失 |
| Severity | MEDIUM (blocking for non-API-key users) |
| Source | 05-services.md §1 |
| Status | NEW |
| Affected files | New: `auth.go` |
| Upstream | `src/services/auth/` — PKCE flow, keychain, token management |
| REPL | N/A — core authentication logic |

**Missing**: Full OAuth/PKCE flow with localhost callback, access/refresh token management, keychain integration, subscription type detection, org membership checks, auth status display.

**Why P2**: OAuth requires significant infrastructure. API-key users can still use Go today.

**Action items**:
1. Implement PKCE flow with localhost callback
2. Add keychain integration (go-keyring)
3. Add automatic token refresh
4. Add subscription type detection
5. Add `/login` and `/logout` commands

---

## P2-2: Basic TUI Layer [TUI — LOWEST PRIORITY]

| Field | Value |
|-------|-------|
| Gap type | 缺失 |
| Severity | LOW (architectural) |
| Source | 06-ui-tui.md §1 |
| Status | NEW |
| Affected files | New: entire TUI package |
| REPL | TUI — Go is a REPL CLI, not a TUI application |

**Missing**: Terminal UI framework, input component with history/autocomplete, streaming output display, permission dialogs, model picker, theme system, keybinding system.

**Why P2**: Per Go REPL positioning — this is explicitly deprioritized. Go is a headless CLI. Any TUI work should only happen if REPL approach is proven insufficient.

**Action items**:
1. Evaluate Bubble Tea vs other Go TUI frameworks
2. Implement basic REPL with streaming output
3. Add input history with search
4. Add permission confirmation UI
5. Add model picker component

---

## P2-3: Analytics/Telemetry Scaffolding

| Field | Value |
|-------|-------|
| Gap type | 缺失 |
| Severity | LOW |
| Source | 05-services.md §2 |
| Status | NEW |
| Affected files | New: `telemetry/` |
| Upstream | OpenTelemetry, Sentry, Langfuse integrations |
| REPL | N/A — core service logic |

**Missing**: OpenTelemetry spans/events, Sentry error reporting, Langfuse LLM observability, session analytics, telemetry opt-out.

**Why P2**: No production deployment yet; analytics can be deferred.

**Action items**:
1. Add OpenTelemetry SDK dependency
2. Instrument API calls with spans
3. Add error reporting pipeline
4. Add opt-out via env var

---

## P2-4: Feature Flag System

| Field | Value |
|-------|-------|
| Gap type | 缺失 |
| Severity | LOW |
| Source | 05-services.md §3 |
| Status | NEW |
| Affected files | New: `growthbook/` |
| Upstream | `src/services/growthbook/` — GrowthBook SDK |
| REPL | N/A — core service logic |

**Missing**: GrowthBook SDK integration, remote flag evaluation, A/B test enrollment, flag caching/refresh.

**Why P2**: Go can use config-file-based feature toggles as a simpler alternative.

**Action items**:
1. Implement simple feature flag system (config-based)
2. Add GrowthBook integration if needed for parity

---

## P2-5: Multi-Provider Routing

| Field | Value |
|-------|-------|
| Gap type | 缺失 |
| Severity | LOW |
| Source | 04-api-client.md §A.5 |
| Status | NEW |
| Affected files | `config.go`, `streaming.go` |
| Upstream | Provider routing in API client |
| REPL | N/A — core API logic |

**Missing**: 7 provider types (Bedrock, Vertex, Foundry, OpenAI, Gemini, Grok), per-provider model ID mapping, API adapters.

**Why P2**: Anthropic first-party API covers most use cases.

**Action items**:
1. Add provider type enum
2. Add Bedrock support (most demanded)
3. Add Vertex support
4. Consider OpenAI adapter

---

## P2-6: MCP Transport Expansion

| Field | Value |
|-------|-------|
| Gap type | 缺失 |
| Severity | LOW |
| Source | 05-services.md §5 |
| Status | NEW |
| Affected files | `mcp/` |
| Upstream | `src/services/mcp/` — SSE, streamableHttp, WebSocket transports |
| REPL | N/A — core MCP logic |

**Missing**: SSE transport, streamableHttp transport, WebSocket transport, OpenAPI transport, health monitoring + circuit breaker, output schema enforcement, per-agent MCP servers.

**Why P2**: stdio transport covers most MCP server implementations.

**Action items**:
1. Add SSE transport
2. Add streamableHttp transport
3. Add health monitoring
4. Add output schema enforcement

---

## P2-7: Session History & Resume Improvements

| Field | Value |
|-------|-------|
| Gap type | 缺失 |
| Severity | LOW |
| Source | 06-ui-tui.md §7 |
| Status | NEW |
| Affected files | `main.go`, `transcript/` |
| Upstream | Session persistence, picker, fork |
| REPL | REPL — reference upstream, adapt for CLI |

**Missing**: Prompt history persistence (`history.jsonl`), paste store, session picker UI, fork session, time-travel resume, rewind files on resume, cloud session discovery.

**Why P2**: Basic resume works; richer session management requires TUI.

**Action items**:
1. Add `history.jsonl` persistence
2. Add paste store
3. Add `--fork-session` flag
4. Add `--resume-session-at` for time travel

---

## P2-8: Plugin System

| Field | Value |
|-------|-------|
| Gap type | 缺失 |
| Severity | LOW |
| Source | 03-system-prompt.md §B.11 |
| Status | NEW |
| Affected files | New: `plugins/` |
| Upstream | Plugin registry, marketplace |
| REPL | N/A — core extensibility logic |

**Missing**: Plugin registry, plugin-provided skills/hooks/MCP servers, enterprise lockdown, marketplace integration, impersonation protection.

**Why P2**: Requires significant infrastructure.

---

## P2-9: Computer Use Integration

| Field | Value |
|-------|-------|
| Gap type | 缺失 |
| Severity | LOW |
| Source | 06-ui-tui.md §10 |
| Status | NEW |
| Affected files | New: `computer_use/` |
| REPL | TUI — requires desktop integration |

**Missing**: Full computer-use MCP server with screen/mouse/keyboard control.

**Why P2**: Requires TUI + MCP expansion + native desktop components.

---

## P2-10: Vim Mode [TUI — LOWEST PRIORITY]

| Field | Value |
|-------|-------|
| Gap type | 缺失 |
| Severity | LOW |
| Source | 06-ui-tui.md §3 |
| Status | NEW |
| Affected files | New: requires TUI |
| REPL | TUI — explicitly deprioritized per REPL positioning |

**Why P2**: Architectural gap — requires TUI input component first.

---

## P2-11: Diff Display & Edit Confirmation [TUI]

| Field | Value |
|-------|-------|
| Gap type | 缺失 |
| Severity | LOW |
| Source | 06-ui-tui.md §13 |
| Status | NEW |
| Affected files | TUI components |
| REPL | REPL — can implement CLI diff display without TUI |

**Missing**: Inline diff display for edits, unified diff view in transcript, edit confirmation with diff preview.

**Why P2**: Requires TUI for full implementation. A simpler CLI diff display could be done independently.

**Action items**:
1. Add CLI-friendly diff output for edits (without TUI)
2. Add diff confirmation for REPL mode
3. Consider TUI diff display as future enhancement

---

## P2-12: Attribution System

| Field | Value |
|-------|-------|
| Gap type | 缺失 |
| Severity | LOW |
| Source | 07-architecture.md §A.6 |
| Status | NEW |
| Affected files | New: `attribution.go` |
| Upstream | Character-level contribution tracking, git notes |
| REPL | N/A — core agent logic |

**Missing**: Character-level contribution tracking, commit attribution with file-level breakdowns, git notes storage, model name sanitization.

**Why P2**: Useful for organizations but not critical for individual users.

---

## P2-13: Daemon Mode

| Field | Value |
|-------|-------|
| Gap type | 缺失 |
| Severity | LOW |
| Source | 07-architecture.md §A.7 |
| Status | NEW |
| Affected files | New: `daemon/` |
| REPL | REPL — daemon mode is a REPL-specific feature |

**Why P2**: Niche feature for headless/remote operation.

---

## P2-14: Fast Mode / Poor Mode / Effort Level

| Field | Value |
|-------|-------|
| Gap type | 缺失 |
| Severity | LOW |
| Source | 04-api-client.md §A.6 |
| Status | NEW |
| Affected files | `config.go`, `agent_loop.go` |
| Upstream | Effort level, fast/poor mode in API client |
| REPL | N/A — core API logic |

**Why P2**: Requires subscription detection + model fallback first.

---

## P2-15: Multi-Source Settings

| Field | Value |
|-------|-------|
| Gap type | 缺失 |
| Severity | LOW |
| Source | 03-system-prompt.md §B.3 |
| Status | NEW |
| Affected files | `config.go` |
| Upstream | 5-level settings hierarchy in config |
| REPL | N/A — core config logic |

**Missing**: 5-level settings hierarchy (defaults < global < project < worktree < session), settings merge with precedence, versioned settings migration, remote settings from API.

**Why P2**: Current single-file config works for most use cases.

---

## P2-16: Thinking Block Handling in Streaming

| Field | Value |
|-------|-------|
| Gap type | 简化 |
| Severity | MEDIUM |
| Source | 04-api-client.md §A.7 |
| Status | NEW |
| Affected files | `streaming.go` |
| Upstream | Thinking block state machine in streaming handler |
| REPL | N/A — core streaming logic |

**Problem**: Go's streaming doesn't properly handle thinking blocks. Upstream has a state machine for filtering/displaying thinking content during streaming.

**Action items**:
1. Add thinking block state machine to streaming handler
2. Add thinking content filtering
3. Add thinking budget configuration

---

## P2-17: Streaming Non-Streaming Fallback

| Field | Value |
|-------|-------|
| Gap type | 缺失 |
| Severity | MEDIUM |
| Source | 04-api-client.md §A.8 |
| Status | NEW |
| Affected files | `streaming.go` |
| Upstream | Non-streaming fallback path in API client |
| REPL | N/A — core API logic |

**Problem**: Go only supports streaming API. Upstream has a non-streaming fallback for when streaming fails or is unavailable.

**Action items**:
1. Add non-streaming API call path
2. Add automatic fallback from streaming to non-streaming on error
3. Add configuration option to prefer non-streaming

---

## P2-18: MCP Reconnection Logic

| Field | Value |
|-------|-------|
| Gap type | 缺失 |
| Severity | MEDIUM |
| Source | 05-services.md §5 |
| Status | NEW |
| Affected files | `mcp/` |
| Upstream | MCP reconnection with exponential backoff |
| REPL | N/A — core MCP logic |

**Problem**: Go's MCP client doesn't reconnect on failure. Upstream has automatic reconnection with exponential backoff.

**Action items**:
1. Add reconnection with exponential backoff
2. Add health monitoring
3. Add circuit breaker pattern

---

## P2-19: MCP OAuth 2.0 Authentication

| Field | Value |
|-------|-------|
| Gap type | 缺失 |
| Severity | MEDIUM |
| Source | 05-services.md §5 |
| Status | NEW |
| Affected files | `mcp/` |
| Upstream | MCP OAuth 2.0 flow |
| REPL | N/A — core MCP logic |

**Problem**: No OAuth support for MCP servers that require authentication.

**Action items**:
1. Add OAuth 2.0 flow for MCP servers
2. Add token management
3. Add scope handling

---

## P2-20: Remote Enterprise Settings

| Field | Value |
|-------|-------|
| Gap type | 缺失 |
| Severity | MEDIUM |
| Source | 05-services.md §4 |
| Status | NEW |
| Affected files | `config.go`, new `remote_settings.go` |
| Upstream | Remote settings API, checksum validation, polling |
| REPL | N/A — core config logic |

**Problem**: Go only loads local settings.json. No remote API fetch for enterprise settings, no checksum validation, no background polling.

**Action items**:
1. Implement remote API fetch for enterprise settings
2. Add SHA-256 checksum validation
3. Add 1h interval background polling
4. Add fail-open behavior on API error

---

## P2-21: Langfuse Tracing Integration

| Field | Value |
|-------|-------|
| Gap type | 缺失 |
| Severity | LOW |
| Source | 05-services.md §14 |
| Status | NEW |
| Affected files | New: `langfuse/` |
| Upstream | `src/services/langfuse/` — Langfuse OpenTelemetry |
| REPL | N/A — core observability logic |

**Action items**:
1. Implement Langfuse OpenTelemetry initialization
2. Implement trace/span creation for agent runs
3. Implement LLM observation recording
4. Implement PII sanitization
5. Implement graceful shutdown with flush

---

## P2-22: Sentry Error Reporting

| Field | Value |
|-------|-------|
| Gap type | 缺失 |
| Severity | LOW |
| Source | 05-services.md §17 |
| Status | NEW |
| Affected files | New: `sentry.go` |
| Upstream | `src/services/sentry/` — Sentry SDK |
| REPL | N/A — core observability logic |

**Action items**:
1. Implement Sentry SDK initialization
2. Implement exception capture with context
3. Implement sensitive header stripping
4. Implement graceful shutdown with flush

---

## P2-23: Idle Timeout Manager

| Field | Value |
|-------|-------|
| Gap type | 缺失 |
| Severity | LOW |
| Source | 05-services.md §10 |
| Status | DONE (PASS) |
| Affected files | `agent_loop.go`, `main.go` |
| Upstream | Idle auto-exit with configurable delay |
| REPL | REPL — idle timeout is a REPL-specific feature |

**Implemented**: `CLAUDE_CODE_EXIT_AFTER_STOP_DELAY` env var support. Accepts duration strings ("5m", "30s") or milliseconds as plain number. When the REPL is idle for the configured duration, exits gracefully with resume hint. Timer starts after each `agent.Run()` completes.

**Audit**: PASS — matches upstream `idleTimeout.ts` behavior. Duration parsing covers both Go duration format and upstream millisecond format.

---

## P2-24: Graceful Shutdown System

| Field | Value |
|-------|-------|
| Gap type | 缺失 |
| Severity | LOW |
| Source | 05-services.md §10 |
| Status | DONE (PASS) |
| Affected files | `main.go` |
| Upstream | Signal handling, process cleanup |
| REPL | REPL — signal handling is REPL-specific |

**Implemented**: SIGTERM (exit 143) and SIGHUP (exit 129) handling in addition to SIGINT. On graceful shutdown: prints resume hint, calls `agent.Close()` (flushes session memory, stops MCP servers), then exits with appropriate code.

**Audit**: PASS — matches upstream signal handling. SIGINT double-press within 2s for immediate exit is preserved. Exit codes follow POSIX conventions.

---

## P2-25: File Cleanup System

| Field | Value |
|-------|-------|
| Gap type | 缺失 |
| Severity | LOW |
| Source | 05-services.md §11 |
| Status | NEW |
| Affected files | New: `cleanup.go` |
| Upstream | Cleanup function registry, periodic cleanup |
| REPL | N/A — core service logic |

**Missing**: Cleanup function registry, periodic message/session/MCP log/plan file cleanup, 30-day cutoff, configurable cleanup period.

---

## P2-26: Structured Diff Rendering

| Field | Value |
|-------|-------|
| Gap type | 缺失 |
| Severity | LOW |
| Source | 05-services.md §8 |
| Status | NEW |
| Affected files | New: `diff.go` |
| Upstream | Structured patch generation, diff rendering |
| REPL | REPL — can implement CLI-friendly diff output |

**Missing**: Structured patch generation, display patch, context lines, diff timeout, line change counting, hunk line number adjustment, whitespace-ignoring diff.

---

## P2-27: Conversation Branching

| Field | Value |
|-------|-------|
| Gap type | 缺失 |
| Severity | LOW |
| Source | 06-ui-tui.md §8 |
| Status | NEW |
| Affected files | `transcript/` |
| Upstream | `/branch` command, transcript copy |
| REPL | REPL — branching could work in CLI |

**Missing**: `/branch` command with transcript copy, session ID generation, parentUuid chain rewrite, title management.

---

## P2-28: Voice Input [TUI — LOWEST PRIORITY]

| Field | Value |
|-------|-------|
| Gap type | 缺失 |
| Severity | LOW |
| Source | 06-ui-tui.md §4 |
| Status | NEW |
| Affected files | New: requires OAuth + TUI |
| REPL | TUI — explicitly deprioritized per REPL positioning |

**Why P2**: Requires OAuth + TUI + native audio capture.

---

## P2-29: Doctor Diagnostics

| Field | Value |
|-------|-------|
| Gap type | 缺失 |
| Severity | LOW |
| Source | 05-services.md §15 |
| Status | DONE (PASS) |
| Affected files | `main.go` |
| Upstream | Installation detection, version validation |
| REPL | REPL — doctor is a CLI feature |

**Implemented**: `/doctor` REPL command that checks: model, API key config, base URL, permission mode, ripgrep availability, Python availability, Node.js availability, Git availability, shell config, MCP servers, loaded skills, transcript count, working directory, CLAUDE.md files.

---

## P2-30: Bridge/Teleport System

| Field | Value |
|-------|-------|
| Gap type | 缺失 |
| Severity | LOW |
| Source | 05-services.md §16 |
| Status | NEW |
| Affected files | New: `bridge/` |
| Upstream | Remote session bridge protocol |
| REPL | N/A — core service logic |

**Missing**: Remote session bridge protocol, bridge API client, session spawner, worktree creation, JWT token refresh, multi-session spawn.

---

## Summary Table

| # | Gap | Status | Effort | REPL Tag |
|---|-----|--------|--------|----------|
| P2-1 | OAuth/PKCE | NEW | Large | N/A |
| P2-2 | Basic TUI | NEW | Large | TUI |
| P2-3 | Analytics | NEW | Medium | N/A |
| P2-4 | Feature flags | NEW | Medium | N/A |
| P2-5 | Multi-provider | NEW | Large | N/A |
| P2-6 | MCP transports | NEW | Medium | N/A |
| P2-7 | Session improvements | NEW | Medium | REPL |
| P2-8 | Plugin system | NEW | Large | N/A |
| P2-9 | Computer use | NEW | Large | TUI |
| P2-10 | Vim mode | NEW | Medium | TUI |
| P2-11 | Diff display | NEW | Medium | REPL |
| P2-12 | Attribution | NEW | Medium | N/A |
| P2-13 | Daemon mode | NEW | Medium | REPL |
| P2-14 | Fast/poor/effort mode | NEW | Small | N/A |
| P2-15 | Multi-source settings | NEW | Medium | N/A |
| P2-16 | Thinking block streaming | NEW | Medium | N/A |
| P2-17 | Non-streaming fallback | NEW | Medium | N/A |
| P2-18 | MCP reconnection | NEW | Medium | N/A |
| P2-19 | MCP OAuth | NEW | Medium | N/A |
| P2-20 | Remote enterprise settings | NEW | Medium | N/A |
| P2-21 | Langfuse tracing | NEW | Medium | N/A |
| P2-22 | Sentry reporting | NEW | Small | N/A |
| P2-23 | Idle timeout | DONE (PASS) | Small | REPL |
| P2-24 | Graceful shutdown | DONE (PASS) | Medium | REPL |
| P2-25 | File cleanup | NEW | Small | N/A |
| P2-26 | Structured diff | NEW | Medium | REPL |
| P2-27 | Conversation branching | NEW | Medium | REPL |
| P2-28 | Voice input | NEW | Large | TUI |
| P2-29 | Doctor diagnostics | DONE (PASS) | Small | REPL |
| P2-30 | Bridge/teleport | NEW | Large | N/A |

## REPL Tag Legend

| Tag | Meaning |
|-----|---------|
| **N/A** | Core logic — must replicate upstream exactly |
| **REPL** | REPL-relevant — reference upstream but adapt for CLI |
| **TUI** | Requires TUI framework — lowest priority per REPL positioning |
