# Diff Upstream - Master Index

> Maps every line from `diff_upstream.md` (11735 lines) to organized category files.

**Source:** `diff_upstream.md`  
**Total sections (##):** 137  
**Category files:** 31  
**Coverage:** 11735/11735 (100.0%)

---

## Category Files

| # | File | Category | Chunks | Source Lines | Description |
|---|------|----------|--------|-------------|-------------|
| 1 | [01-core-agent-loop.md](01-core-agent-loop.md) | Core Agent Loop | 4 | 411 | Agent loop orchestration (sections 7, 35, 7.27) |
| 2 | [02-context.md](02-context.md) | Context Management | 17 | 522 | Context window, context references, entry system |
| 3 | [03-compact.md](03-compact.md) | Compaction System | 14 | 846 | Compaction, cache, post-compact recovery |
| 4 | [04-streaming.md](04-streaming.md) | Streaming | 55 | 507 | Streaming response handling, SSE, stream bus |
| 5 | [05-normalize.md](05-normalize.md) | Message Normalization | 1 | 58 | Message and content normalization |
| 6 | [06-error-handling.md](06-error-handling.md) | Error Handling & Retry | 5 | 695 | Errors, retry logic, rate limiting |
| 7 | [07-tool-interface.md](07-tool-interface.md) | Tool Interface System | 5 | 523 | Base tool interface, structured output |
| 8 | [08-file-tools.md](08-file-tools.md) | File Tools | 11 | 651 | File read/write/edit, multi-edit, notebook |
| 9 | [09-exec-bash.md](09-exec-bash.md) | Exec/Bash Tool | 3 | 325 | Shell execution, sandboxing, process management |
| 10 | [10-search-tools.md](10-search-tools.md) | Search Tools | 3 | 467 | Grep, glob, list_dir |
| 11 | [11-agent-tools.md](11-agent-tools.md) | Agent Tools | 6 | 378 | Sub-agent, forked agent |
| 12 | [12-other-tools.md](12-other-tools.md) | Other Tools | 4 | 188 | file_history, work_task, git tools, todo |
| 13 | [13-web-tools.md](13-web-tools.md) | Web Tools | 1 | 31 | Web fetch, web search, URL expansion |
| 14 | [14-system-prompt.md](14-system-prompt.md) | System Prompt | 2 | 225 | System prompt construction, templates |
| 15 | [15-config.md](15-config.md) | Configuration | 3 | 145 | Config system, settings |
| 16 | [16-permissions.md](16-permissions.md) | Permissions | 14 | 478 | Permission system, auto-classifier 8.1-8.11 |
| 17 | [17-hooks.md](17-hooks.md) | Hooks | 2 | 242 | Hook system, lifecycle hooks |
| 18 | [18-mcp.md](18-mcp.md) | MCP Implementation | 2 | 215 | Model Context Protocol |
| 19 | [19-session-memory.md](19-session-memory.md) | Session Memory & Skills | 19 | 1054 | Session memory, skills, file history, snapshots |
| 20 | [20-transcript.md](20-transcript.md) | Transcript | 4 | 450 | Transcript recording, playback, resume |
| 21 | [21-api-client.md](21-api-client.md) | API Client | 1 | 100 | API client, beta headers, model config |
| 22 | [22-analytics-telemetry.md](22-analytics-telemetry.md) | Analytics & Telemetry | 3 | 26 | Analytics, telemetry, Langfuse, OTel |
| 23 | [23-oauth-auth.md](23-oauth-auth.md) | OAuth & Authentication | 1 | 18 | OAuth, authentication |
| 24 | [24-feature-flags.md](24-feature-flags.md) | Feature Flags | 1 | 15 | GrowthBook, feature flags |
| 25 | [25-cost-tracking.md](25-cost-tracking.md) | Cost Tracking | 2 | 206 | Cost tracking, usage, model routing |
| 26 | [26-tui-ui.md](26-tui-ui.md) | TUI & UI | 3 | 470 | Terminal UI, rendering, components |
| 27 | [27-cross-cutting.md](27-cross-cutting.md) | Cross-Cutting | 15 | 1129 | Architecture, concurrency, utilities, gap analysis |
| 28 | [28-services.md](28-services.md) | Upstream Services | 2 | 355 | Upstream services, utils |
| 29 | [29-testing.md](29-testing.md) | Testing | 2 | 459 | Test coverage |
| 30 | [30-go-enhancements.md](30-go-enhancements.md) | Go Enhancements | 5 | 528 | Go-specific improvements, sandbox |
| 31 | [31-deep-dive-rounds.md](31-deep-dive-rounds.md) | Deep Dive Rounds | 2 | 10 | Supplementary deep dive analysis (section 15) |

---

## Section-to-File Mapping

| Line | Section Title | Category File |
|------|--------------|---------------|
| 9-56 | 目录 / Table of Contents | 01-core-agent-loop.md |
| 57-144 | 1. Streaming Implementation | 04-streaming.md |
| 145-202 | 2. Message Normalization | 05-normalize.md |
| 203-263 | 3. Prompt Caching | 03-compact.md |
| 264-379 | 4. System Prompt | 14-system-prompt.md |
| 380-404 | Summary of Key Gaps | 27-cross-cutting.md |
| 405-586 | 5. MCP (Model Context Protocol) Implementation | 18-mcp.md |
| 587-619 | MCP Comparison Summary of Key Gaps | 18-mcp.md |
| 620-1063 | 6. Test Coverage Comparison: Go vs Upstream TypeScript | 29-testing.md |
| 1064-1313 | 7. Agent Loop Core (`agent_loop.go`) | 01-core-agent-loop.md |
| 1314-1347 | 7.27 Summary: Agent Loop Key Differences | 01-core-agent-loop.md |
| 1348-1628 | compact.go vs Upstream Compaction System | 03-compact.md |
| 1629-1686 | 8. Tool Interface & Registry (tools/base.go vs upstream Tool.ts,  | 07-tool-interface.md |
| 1687-1764 | 9. Bash/Exec Tool (tools/exec_tool.go vs upstream BashTool) | 09-exec-bash.md |
| 1765-1842 | 10. File Read Tool (tools/file_read.go vs upstream FileReadTool) | 08-file-tools.md |
| 1843-1910 | 11. File Edit Tool (tools/file_edit.go vs upstream FileEditTool) | 08-file-tools.md |
| 1911-1953 | 12. File Write Tool (tools/file_write.go vs upstream FileWriteToo | 08-file-tools.md |
| 1954-2031 | 13. Config System (config.go vs upstream settings) | 15-config.md |
| 2032-2129 | 14. Permission System (permissions.go vs upstream permissions) | 16-permissions.md |
| 2130-2497 | 15. Deep Comparison: Streaming, Caching, Memory, Hooks, Retry, Ra | 31-deep-dive-rounds.md |
| 2498-2538 | 16. Error Types & Classification (`error_types.go`) | 06-error-handling.md |
| 2539-2559 | 17. Agent Progress Tracking (`agent_progress.go`) | 06-error-handling.md |
| 2560-2585 | 18. Async Task Management (`agent_task.go`) | 27-cross-cutting.md |
| 2586-2616 | 19. Forked Agent / Parallel API Calls (`forked_agent.go`) | 11-agent-tools.md |
| 2617-2667 | 20. File History Tools (`file_history_tools.go` + `filehistory.go | 12-other-tools.md |
| 2668-2683 | 21. Transcript Building (`transcript_builder.go`) | 20-transcript.md |
| 2684-2704 | 22. Work Task / Dependency Graph (`work_task.go`) | 12-other-tools.md |
| 2705-2735 | 23. Skills System (`skills/loader.go` + `skills/tracker.go`) | 19-session-memory.md |
| 2736-2751 | 24. Transcript Serialization (`transcript/transcript.go`) | 20-transcript.md |
| 2752-2787 | 25. Sub-Agent Orchestration (`agent_sub.go`) | 11-agent-tools.md |
| 2788-2926 | 26. Tool Implementations (`tools/`) | 07-tool-interface.md |
| 2927-2947 | 27. Permissions Submodule (`permissions/`) | 16-permissions.md |
| 2948-2991 | Summary of Key Differences (Sections 16-27) | 27-cross-cutting.md |
| 2992-3403 | 27. Deep Comparison: Search/Listing Tools (Go vs Upstream TypeScr | 10-search-tools.md |
| 3404-3709 | PostCompactRecovery System — Deep Comparison | 03-compact.md |
| 3710-3810 | Part 1: Context Management | 02-context.md |
| 3811-3893 | Part 2: Sub-Agent Orchestration | 11-agent-tools.md |
| 3894-3956 | 8. context.go — 上下文管理（ConversationContext + ToolStateTracker） | 02-context.md |
| 3957-3988 | 9. agent_sub.go — 子代理编排（续接第7节已有内容） | 11-agent-tools.md |
| 3989-4018 | 10. agent_task.go — 异步任务管理 | 27-cross-cutting.md |
| 4019-4063 | 11. work_task.go — 工作依赖图（TODO 管理） | 12-other-tools.md |
| 4064-4138 | 12. filehistory.go — 文件历史/快照 | 19-session-memory.md |
| 4139-4214 | 13. context_references.go — @context 展开 | 02-context.md |
| 4215-4227 | 总结：6个文件的关键差异 | 27-cross-cutting.md |
| 4228-4511 | Entry Points 与 Build System 对比 (Go vs 上游 TypeScript) | 27-cross-cutting.md |
| 4512-4711 | 12 Permissions Submodule: Go vs Upstream | 16-permissions.md |
| 4712-4864 | 13 Hooks System: Go vs Upstream | 17-hooks.md |
| 4865-5115 | Part 10: Session Memory System — Deep Comparison | 19-session-memory.md |
| 5116-5360 | Part 11: Skills System — Deep Comparison | 19-session-memory.md |
| 5361-5627 | 10. Retry, Rate-Limiting & Normalization -- Deep Algorithmic Comp | 06-error-handling.md |
| 5628-5906 | Gap Analysis: Previously Uncovered Topics | 27-cross-cutting.md |
| 5907-6183 | ERROR CLASSIFICATION AND ERROR TYPE SYSTEM: Go vs Upstream TypeSc | 06-error-handling.md |
| 6184-6554 | 12. Transcript & Resume System | 20-transcript.md |
| 6555-6572 | 7.1 Cache Breakpoint Placement | 03-compact.md |
| 6573-6594 | 7.2 Cache Edit Injection | 03-compact.md |
| 6595-6602 | 7.3 Cache Sharing Across Forked Agents | 03-compact.md |
| 6603-6617 | 7.4 Cache Invalidation on Micro-Compact | 03-compact.md |
| 6618-6626 | 7.5 Cache Creation/Read Token Tracking | 03-compact.md |
| 6627-6635 | 7.6 Pinned Cache Edits - Upstream's Always-Cached Blocks | 03-compact.md |
| 6636-6658 | 7.7 Cache Break Detection - Upstream Only | 03-compact.md |
| 6659-6670 | 8.1 Classifier Architecture | 16-permissions.md |
| 6671-6680 | 8.2 Token Budget Per Stage | 16-permissions.md |
| 6681-6690 | 8.3 Classification Criteria | 16-permissions.md |
| 6691-6700 | 8.4 Safety Check Types | 16-permissions.md |
| 6701-6709 | 8.5 Bypass Immunity Logic | 16-permissions.md |
| 6710-6722 | 8.6 Denial Tracking | 16-permissions.md |
| 6723-6732 | 8.7 Job Classifier (Upstream Only) | 16-permissions.md |
| 6733-6747 | 8.8 Auto-Allow Logic | 16-permissions.md |
| 6748-6758 | 8.9 Input Formatting | 16-permissions.md |
| 6759-6776 | 8.10 Integration with Permission Gate | 16-permissions.md |
| 6777-6817 | 8.11 Summary of All Differences | 16-permissions.md |
| 6818-7006 | Section XX: Model Routing, Cost Tracking, and Background Services | 25-cost-tracking.md |
| 7007-7009 | Go Implementation | 02-context.md |
| 7010-7018 | Upstream Implementation | 02-context.md |
| 7019-7046 | 1. Supported Reference Types | 02-context.md |
| 7047-7073 | 2. File Expansion — Reading and Injecting File Content | 02-context.md |
| 7074-7095 | 3. Folder Expansion — Directory Listing with Content | 02-context.md |
| 7096-7112 | 4. Diff Expansion — Git Diff Injection | 02-context.md |
| 7113-7123 | 5. Staged Expansion — Git Staged Changes | 02-context.md |
| 7124-7136 | 6. Git Log Expansion — Commit History | 02-context.md |
| 7137-7154 | 7. URL Expansion — Web Content Fetching | 02-context.md |
| 7155-7174 | 8. Token Budget for Expansion | 02-context.md |
| 7175-7192 | 9. Safety Checks — Path Validation, Size Limits | 02-context.md |
| 7193-7215 | 10. Error Handling — Missing Files, Failed URLs | 02-context.md |
| 7216-7218 | Go Implementation | 19-session-memory.md |
| 7219-7228 | Upstream Implementation | 19-session-memory.md |
| 7229-7245 | 1. Snapshot Creation — When Snapshots Are Taken | 19-session-memory.md |
| 7246-7271 | 2. Snapshot Storage — In-Memory vs Disk | 19-session-memory.md |
| 7272-7297 | 3. Snapshot Restoration — Undo/Rollback Mechanisms | 19-session-memory.md |
| 7298-7316 | 4. Snapshot Metadata — Timestamps, Annotations | 19-session-memory.md |
| 7317-7333 | 5. Snapshot Listing — Viewing Available Snapshots | 19-session-memory.md |
| 7334-7351 | 6. Diff Generation — Comparing Current vs Snapshot | 19-session-memory.md |
| 7352-7366 | 7. Cross-File Timeline — Go's Timeline Feature | 19-session-memory.md |
| 7367-7383 | 8. Batch Operations — Bulk Snapshot/Restore | 19-session-memory.md |
| 7384-7400 | 9. Tagging System — Snapshot Tags | 19-session-memory.md |
| 7401-7416 | 10. Auto-Snapshot — Before Write/Edit | 19-session-memory.md |
| 7417-7433 | Summary Table: Context References | 02-context.md |
| 7434-7454 | Summary Table: File History | 19-session-memory.md |
| 7455-7525 | Part 1: Git Tool Comparison | 12-other-tools.md |
| 7526-7690 | Part 2: Agent Tool Comparison | 11-agent-tools.md |
| 7691-7793 | Deep Comparison: Multi-Edit Atomic System vs Upstream FileEditToo | 08-file-tools.md |
| 7794-7999 | Deep Comparison: Exec Tool Process Management vs Upstream BashToo | 09-exec-bash.md |
| 8000-8239 | Final Comprehensive Gap Check — 2026-05-12 | 27-cross-cutting.md |
| 8240-8537 | 99. Structured Tool Output & API Parameter Deep Comparison | 07-tool-interface.md |
| 8538-8744 | 33. Sandbox Execution and Security Features | 30-go-enhancements.md |
| 8745-8759 | A. Coverage Statistics | 29-testing.md |
| 8760-8778 | B. Top 10 Critical Gaps | 27-cross-cutting.md |
| 8779-8797 | C. Top 10 Go Enhancements | 30-go-enhancements.md |
| 8798-8818 | D. Simplifications Map | 27-cross-cutting.md |
| 8819-8841 | E. Architectural Differences | 27-cross-cutting.md |
| 8842-8868 | F. Implementation Priority | 27-cross-cutting.md |
| 8869-8907 | G. Go-Only Features Not in Upstream | 30-go-enhancements.md |
| 8908-9089 | Section XXI: TUI Rendering, Input Handling, Notification System,  | 26-tui-ui.md |
| 9090-9273 | Section XXII: Deep Dive - Tool Result Persistence, Cached Microco | 19-session-memory.md |
| 9274-9350 | Section XXIII. Round 28 — Tool-Level Code Differences (Read/Write | 08-file-tools.md |
| 9351-9406 | 34. Context.go Deep Dive — Entry System & API Message Building | 02-context.md |
| 9407-9485 | 35. Agent Loop Deep Dive — Loop Structure, Retry, Tool Execution | 01-core-agent-loop.md |
| 9486-9561 | 36. Compact.go Deep Dive — Prompt, Retry, Post-Compact Recovery | 03-compact.md |
| 9562-9610 | 37. Config, Permissions, System Prompt Deep Dive | 15-config.md |
| 9611-9656 | 38. Session Memory, Skills, File History, Context References | 19-session-memory.md |
| 9657-9703 | 39. Streaming, Normalize, Prompt Caching, Rate Limit, Retry Utils | 04-streaming.md |
| 9704-9750 | 40. Transcript, Sub-Agent, Forked Agent, Tools Interface | 20-transcript.md |
| 9751-9808 | 41. Streaming API Layer — Supplementary Deep Dive | 04-streaming.md |
| 9809-9940 | 42. Tools Deep Dive — FileRead/Write/Edit, Bash, Grep/Glob, Agent | 08-file-tools.md |
| 9941-10029 | 43. Error Handling, Work Tasks, Agent Progress, CLI Entry Points | 06-error-handling.md |
| 10030-10118 | 44. Hooks, MCP Client, Permissions, File Watcher, Cost Tracking | 17-hooks.md |
| 10119-10184 | 45. Remaining Go Modules Sweep — main.go, retry, normalize, strea | 30-go-enhancements.md |
| 10185-10278 | 46. Analytics/Telemetry, Cost Tracking, Feature Flags, OAuth, Set | 22-analytics-telemetry.md |
| 10279-10336 | 47. TUI/UI System, Print/Rendering, Session Management | 26-tui-ui.md |
| 10337-10533 | 49. Go-Specific Enhancements & Adaptations | 30-go-enhancements.md |
| 10534-10633 | 48. API Client, Beta Headers, Model Management | 21-api-client.md |
| 10634-10705 | 50. Cross-Cutting Architectural Patterns | 27-cross-cutting.md |
| 10706-10909 | 51. Upstream Utils Deep Dive -- conversationRecovery, fileRead, d | 28-services.md |
| 10910-11060 | 52. Upstream Services Deep Dive — API, OAuth, Remote Settings, La | 28-services.md |
| 11061-11290 | 53. Upstream Component System vs Go Headless CLI | 26-tui-ui.md |
| 11291-11626 | 54. Tools Comparison — Write, Edit, Read, Bash, Grep, Glob, WebFe | 07-tool-interface.md |
| 11627-11735 | 55. System Prompt Construction | 14-system-prompt.md |

---

## Statistics

- **Total source lines:** 11735
- **Total ## sections:** 137
- **Content chunks:** 212
- **Lines covered:** 11735 (100.0%)
