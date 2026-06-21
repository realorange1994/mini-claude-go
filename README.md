# miniClaudeCode-go

A lightweight CLI coding assistant and AI agent loop framework written in Go, modeled after Claude Code.

## Overview

miniClaudeCode-go implements the core agentic loop pattern of Claude Code: a turn-based tool-use paradigm where an LLM calls tools to accomplish tasks, with streaming output, context management, crash recovery, and multi-level permission control. It supports Anthropic API and OpenAI-compatible proxy endpoints through unified configuration.

## Features

### Agent Loop & Conversations

- **Turn-based agent loop** with `IterationBudget` (consume/refund/grace call) and configurable max turns (default 90)
- **Streaming output** with real-time SSE parsing, `ThinkFilter` state machine for `<thinking>` blocks, `StreamProgress` tracking (TTFB, throughput), dynamic stall timeout scaling, and delta-state tracking for retry safety
- **Multi-turn conversations** with persistent context across turns
- **Interactive REPL** with Ctrl+C interrupt handling (double-press to exit) and piped input support
- **One-shot mode**: pass a prompt as a positional argument for single-turn, non-interactive use
- **Session resume**: reload prior conversations from JSONL transcripts (`--resume last` or by name/number)

### Context Compaction

Multi-phase automatic context management to stay within token limits:

- **Micro-compact** (every turn): lightweight clearing of old tool results beyond a configurable keep-window (default 5). Zero API cost.
- **SM-compact** (Session Memory compact): uses accumulated session memory notes as the summary source, avoiding an LLM API call.
- **LLM-driven compaction**: full summary generation via 3-pass pre-pruning (dedup, summarize, truncate) with iterative updates and cooldown anti-thrashing.
- **Partial compact** (directional): `/partialcompact up_to` summarizes early conversation; `/partialcompact from` summarizes recent conversation.
- **Reactive compact**: triggers when token count spikes above a threshold (default 5000) between turns.
- **4-phase fallback chain**: round-based compact, turn-based smart compact, selective clear (read-only tools), then aggressive truncation.
- **Post-compact recovery**: re-injects recently read file content and used skill content after compaction.
- **Auto-Continue**: automatically continues work after compaction instead of waiting for user input.
- **Two-Level Pruning**: soft-trim (keep head+tail) and hard-clear (mark as compacted) based on pressure levels.
- **Budgeted Read**: token-budgeted file reading with section-aware truncation.

### @ Context References

Inject external context into prompts with `@file:path`, `@folder:path`, `@staged`, `@diff`, `@git:N`, `@url:URL`. Supports line ranges, token budget guardrails (50% hard / 25% soft limits), and sensitive path protection.

### Tool System

35+ built-in tools with argument type coercion and schema validation:

| Category | Tools |
|----------|-------|
| **File operations** | `read_file`, `write_file`, `edit_file`, `multi_edit`, `fileops` (copy/move/delete/chmod/symlink) |
| **File search** | `glob`, `grep`, `list_dir` |
| **Web** | `web_search`, `web_fetch`, `exa_search` (Exa-powered search) |
| **Git** | `git` -- full operations (clone/commit/push/pull/branch/merge/rebase/stash/worktree) with built-in dangerous operation detection (force push, reset --hard, clean -f, etc.) |
| **System** | `system` (uname/df/free/uptime/hostname/arch), `runtime_info` (Go runtime stats) |
| **Process** | `process` (list/kill/pgrep/top/pstree) |
| **Terminal** | `terminal` (tmux/screen session management) |
| **Task management** | `task_create`, `task_list`, `task_get`, `task_update`, `task_stop`, `task_output` |
| **Agent** | `agent` (sub-agent spawning), `send_message` (communicate with running agents) |
| **Advanced** | `apply_patch` (atomic multi-file edits), `code_search` (Exa API), `todo_write` (task list) |

### MiMo-Code Inspired Features

Features borrowed from [MiMo-Code](https://github.com/XiaomiMiMo/MiMo-Code) (TypeScript monorepo):

#### Context Management
- **Pressure Level Tracking**: 4-level pressure system (0-3) based on token usage
- **Output-Capped Context**: prevents large-output models from shrinking input window
- **Provider Message Transform**: Anthropic/Bedrock/Mistral message normalization
- **Boundary Adjustment**: ensures tool_use/tool_result pairs aren't split

#### Session Management
- **Session Forking**: create independent session copies from checkpoints
- **Session Diff Tracking**: track file changes per session
- **Step Classification**: 6 categories for loop control (final/continue/think-only/invalid/failed)
- **External Session Import**: import sessions from Claude Code/Codex

#### Memory System
- **Three-Level Memory**: global/project/session scopes
- **FTS Search**: full-text search with BM25 ranking
- **Fuzzy Deduplication**: content similarity detection
- **Auto-Dream**: periodic memory consolidation

#### Tool Enhancements
- **Smart Truncation**: error-aware head+tail splitting
- **Doom Loop Detection**: detects repeated tool calls
- **Recoverable Errors**: distinguishes fixable vs fatal errors
- **Invocation Style**: JSON or shell tool invocation

#### Sub-Agent System
- **Agent ID Isolation**: messages filtered by agent_id
- **Task Gate Stop**: prevents stopping with incomplete tasks
- **Completion Gate**: verifies agents finish work
- **Health Check**: detects stuck agents

#### Compression & Recovery
- **Checkpoint Writer**: structured checkpoint.md generation
- **Checkpoint Validator**: validates checkpoint structure
- **Progress Reconciliation**: integrates subagent progress
- **Session Revert**: git-based snapshot restoration

### LSP Integration

Full Language Server Protocol support:

- **Go** (gopls): diagnostics, hover, go-to-definition
- **TypeScript** (typescript-language-server): diagnostics, hover
- **Python** (pylsp): diagnostics, hover

Features:
- Initialize/initialized handshake
- Document sync (didOpen/didChange/didClose/didSave)
- Diagnostic publishing
- Synchronous request/response with timeout

### Plugin System

Extensible hook-based plugin architecture:

- **Hook Points**: preStop, postStop, config, event, preTool, postTool
- **Plugin Discovery**: scan external directories
- **Plugin Installation**: local path or git URL
- **Enable/Disable**: per-plugin control

### Workflow Runtime

Multi-step workflow orchestration:

- **Step Types**: agent, parallel, pipeline, condition
- **Workflow Definition**: JSON-based workflow definitions
- **Execution**: sequential step execution
- **Persistence**: save/load workflows to disk

### ACP IDE Integration

Agent Communication Protocol for IDE integration:

- **Methods**: initialize, newSession, loadSession, listSessions, forkSession, resumeSession, prompt, cancel, setSessionModel, setSessionMode
- **Events**: toolCall, toolResult, text, reasoning, permission, done, error
- **Transport**: stdio and HTTP

### Inter-Agent Inbox

Persistent messaging between agents:

- **Send/Receive**: send messages between agents
- **Drain/Peek**: read messages with or without marking as read
- **Persistence**: messages saved to disk
- **XML Format**: `<inbox>` blocks for agent injection

### Session Sharing

Cloud sync for collaborative debugging:

- **Share/Unshare**: share session to cloud
- **Sync**: incremental data synchronization
- **Share URL**: get shareable link

### Git Snapshot System

Git-based snapshot/restore system:

- **Track**: track file changes
- **Create**: create snapshots
- **Restore**: restore files from snapshot
- **Diff**: compute differences between snapshots

### Auto-Formatter

Automatic code formatting after file writes:

- **Supported**: gofmt, prettier, ruff, biome, rustfmt, clang-format, shfmt, nixfmt
- **Auto-detect**: detects project formatter from config files
- **Configurable**: enable/disable per formatter

### Max Mode

Multi-candidate + Judge for better output:

- **Candidates**: run N parallel candidates (default 5)
- **Judge**: select best candidate
- **Overhead Tracking**: track token overhead

### Skill Discovery

Remote skill discovery:

- **External Directories**: scan ~/.claude/skills, ~/.agents/skills, etc.
- **Remote Index**: fetch skills from URL
- **Skill Sources**: builtin, workspace, external, remote, compose

## Installation

```bash
# Clone the repository
git clone https://github.com/realorange1994/mini-claude-go.git
cd mini-claude-go

# Build
go build -o miniclaudecode.exe .

# Or install
go install .
```

## Quick Start

```bash
# Interactive mode
./miniclaudecode.exe

# One-shot mode
./miniclaudecode.exe "What is 2+2?"

# With specific model
./miniclaudecode.exe --model claude-sonnet-4-20250514

# Resume session
./miniclaudecode.exe --resume last
```

## Configuration

### Environment Variables

```bash
# API Key (required)
export ANTHROPIC_API_KEY=sk-ant-...

# Model selection
export CLAUDE_MODEL=claude-sonnet-4-20250514
export CLAUDE_DEFAULT_SONNET_MODEL=claude-sonnet-4-20250514
export CLAUDE_DEFAULT_HAIKU_MODEL=claude-haiku-4-5-20250610

# Token limits
export CLAUDE_CODE_MAX_CONTEXT_TOKENS=1000000
export CLAUDE_CODE_MAX_OUTPUT_TOKENS=64000

# Tool-specific
export CLAUDE_CODE_GIT_BASH_PATH="C:\Program Files\Git\bin\bash.exe"
```

### Settings File

Create `.claude/settings.json` in your project root:

```json
{
  "api_key": "sk-ant-...",
  "model": "claude-sonnet-4-20250514",
  "max_turns": 90,
  "permission_mode": "auto",
  "auto_classifier_enabled": true,
  "micro_compact_enabled": true,
  "lsp_enabled": true,
  "formatter_enabled": true
}
```

### MCP Configuration

Create `.mcp.json` in your project root:

```json
{
  "mcpServers": {
    "filesystem": {
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-filesystem", "/path/to/dir"]
    }
  }
}
```

### CLAUDE.md

Place a `CLAUDE.md` file in your project root to provide project-specific instructions automatically loaded into the system prompt.

## Architecture

```
miniClaudeCode-go/
├── main.go                          # Entry point, CLI flags, REPL loop
├── agent_loop.go                    # Core agent loop (IterationBudget, notifications, sub-agent spawning)
├── agent_sub.go                     # Sub-agent system: SpawnSubAgent, specialized types, fork mode
├── agent_task.go                    # Task store (TaskStore, TaskState), background bash tasks
├── work_task.go                     # Work task management (dependency graph, cycle detection)
├── auto_classifier.go               # Two-stage LLM classifier
├── permissions.go                   # Permission gate (ask/auto/plan modes)
├── streaming.go                     # SSE streaming, ThinkFilter, StreamProgress
├── context.go                       # ConversationContext, entry types, compaction boundaries
├── context_references.go            # @ reference expansion
├── compact.go                       # All compaction strategies
├── config.go                        # Configuration loading
├── system_prompt.go                 # CachedSystemPrompt builder
├── prompt_caching.go                # Anthropic prompt caching
├── error_types.go                   # 15-category structured error classification
├── normalize.go                     # API message normalization
├── rate_limit.go                    # Rate limit tracking
├── retry_utils.go                   # Retry with exponential backoff
├── filehistory.go                   # File version history and snapshots
├── session_memory.go                # Session memory with persistent notes
│
├── # MiMo-Code Inspired Features
├── provider_transform.go            # Provider message transform layer
├── budgeted_read.go                 # Token-budgeted file reading
├── prune.go                         # Two-level pruning (soft-trim + hard-clear)
├── step_classify.go                 # Step classification (6 categories)
├── session_diff.go                  # Session diff/summary tracking
├── revert_manager.go                # Session revert with git snapshots
├── boundary_adjust.go               # API boundary adjustment
├── checkpoint_validator.go          # Checkpoint validation
├── progress_reconcile.go            # Progress reconciliation
├── checkpoint_writer.go             # Background checkpoint writer
├── auto_dream.go                    # Auto-dream memory consolidation
├── task_gate.go                     # Task gate stop pattern
├── session_goal.go                  # Goal/Judge stop condition
├── memory_fts.go                    # FTS search with BM25 ranking
├── truncation_service.go            # Sophisticated truncation service
├── invocation_style.go              # Per-tool invocation style
├── consecutive_failure_tracker.go   # Doom loop detection
├── max_mode.go                      # Multi-candidate + Judge
├── apply_patch.go                   # Atomic multi-file patch application
├── code_search.go                   # Exa API code search
├── formatter_service.go             # Auto-formatter service
├── external_import.go               # External session import
├── git_snapshot.go                  # Git-based snapshot system
├── lsp_manager.go                   # LSP protocol implementation
├── skill_discovery.go               # Remote skill discovery
├── worktree_service.go              # Git worktree service
├── inbox_service.go                 # Inter-agent inbox
├── share_service.go                 # Session sharing
├── plugin_manager.go                # Plugin system
├── acp_agent.go                     # ACP IDE integration
├── workflow_runtime.go              # Workflow runtime
├── common_utils.go                  # Common utility functions
│
├── tools/                           # Built-in tool implementations
│   ├── base.go                      # ToolResultMetadata, schema validation
│   ├── coercion.go                  # Argument type coercion
│   ├── agent_tool.go                # AgentTool (sub-agent spawning)
│   ├── send_message_tool.go         # SendMessageTool
│   ├── task_tool.go                 # TaskCreate/List/Get/Update
│   ├── task_output_tool.go          # TaskOutputTool (blocking wait)
│   ├── exec_tool.go                 # Shell command execution
│   ├── file_read.go                 # File reading
│   ├── file_write.go                # File writing
│   ├── file_edit.go                 # File editing
│   ├── multi_edit.go                # Multi-edit operations
│   ├── glob_tool.go                 # Glob pattern matching
│   ├── grep_tool.go                 # Grep search
│   ├── list_dir.go                  # Directory listing
│   ├── web_search.go                # Web search
│   ├── exa_search.go                # Exa-powered search
│   ├── web_fetch.go                 # Web content fetching
│   ├── fileops.go                   # File operations (copy/move/delete/chmod)
│   ├── process.go                   # Process management
│   ├── git_tool.go                  # Git operations with dangerous op detection
│   ├── system_tool.go               # System information
│   ├── terminal_tool.go             # tmux/screen management
│   ├── runtime_info.go              # Go runtime info
│   ├── skill_tools.go               # read_skill, list_skills, search_skills
│   ├── memory_tool.go               # memory_add, memory_search
│   ├── mcp_tools.go                 # list_mcp_tools, call_mcp_tool, mcp_server_status
│   └── output_cleaner.go            # Smart truncation
│
├── skills/                          # Skill loading
│   ├── loader.go                    # SkillLoader (workspace + builtin skills)
│   └── tracker.go                   # SkillTracker for progressive disclosure
│
├── mcp/                             # MCP client
│   └── client.go                    # MCP protocol implementation
│
├── permissions/                     # Permission system
│   ├── wildcard.go                  # Wildcard pattern matching
│   └── internal_paths.go            # Memory path isolation
│
├── transcript/                      # Crash-safe JSONL logging
│   └── transcript.go                # Transcript reader/writer
│
├── .github/workflows/               # CI/CD configuration
│   └── ci.yml                       # GitHub Actions
│
└── go.mod
```

## Testing

```bash
# Run all tests
go test ./...

# Run with verbose output
go test -v ./...

# Run specific test
go test -run TestE2E_FullWorkflow

# Run with timeout
go test -count=1 -timeout 180s
```

### Test Coverage

- **Unit Tests**: 277 tests
- **End-to-End Tests**: 9 tests
- **Total**: 286 tests, 100% pass rate

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests for new functionality
5. Run `go test ./...`
6. Submit a pull request

## License

MIT

## Acknowledgments

- [Claude Code](https://docs.anthropic.com/en/docs/agents-and-tools/claude-code/overview) - Original inspiration
- [MiMo-Code](https://github.com/XiaomiMiMo/MiMo-Code) - Feature inspiration
- [Anthropic](https://www.anthropic.com/) - API and model support
