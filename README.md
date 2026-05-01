# miniClaudeCode-go

A lightweight implementation of Claude Code's agent loop framework written in Go.

## Overview

miniClaudeCode-go is a minimal AI agent framework that implements the core agentic loop pattern similar to Claude Code. It provides a tool-use paradigm where an LLM can execute various tools to accomplish complex tasks, with error handling, context management, and crash recovery. It supports Anthropic and OpenAI-compatible proxy endpoints through a unified configuration interface.

## Features

- **Agent Loop**: Turn-based conversation with IterationBudget (consume/refund/grace call), and preflight compression for resumed sessions
- **Streaming Support**: Real-time streaming output with ThinkFilter state machine (filters `<thinking>` blocks from model output), StreamProgress tracking (TTFB, throughput), dynamic stall timeout scaling, and DeltasState tracking for retry safety
- **Context Compaction**: Multi-phase automatic context management:
  - Micro-compact (every turn): lightweight clearing of old tool results with dedup + whitelist
  - SM-compact: uses Session Memory as summary (no LLM API call)
  - LLM-driven compaction: structured summary with iterative update support
  - Partial compact: directional summarization (prefix-preserving or suffix-preserving)
  - Reactive compact: triggers on token spikes between turns
  - Fallback: round-based, smart, selective, and aggressive truncation phases
- **@ Context References**: Inject file content, folder listings, git diffs, and URLs into prompts with `@file:path`, `@folder:path`, `@staged`, `@diff`, `@git:N`, `@url:URL`. Supports line ranges, token budget guardrails, and sensitive path protection
- **Tool System**: 35+ built-in tools with argument type coercion and schema validation:
  - `exec` -- Shell command execution with safety patterns
  - `read_file` / `write_file` / `edit_file` / `multi_edit` -- File operations with read-before-edit enforcement
  - `glob` / `grep` / `list_dir` -- File system search and navigation
  - `web_search` / `web_fetch` -- Web search (built-in + Exa) and content fetching with HTML parsing
  - `fileops` -- File operations (copy, move, delete, chmod, symlink)
  - `process` -- Process management (list, kill, pgrep, top, pstree)
  - `git` -- Full git operations (clone, commit, push, pull, branch, merge, rebase, stash, worktree, and more)
  - `system` -- System info (uname, df, free, uptime, hostname, arch)
  - `terminal` -- tmux/screen session management
  - `runtime_info` -- Go runtime and system information
  - `memory_add` / `memory_search` -- Session memory tools
  - `read_skill` / `list_skills` / `search_skills` -- Skill loading and discovery
  - `list_mcp_tools` / `call_mcp_tool` / `mcp_server_status` -- MCP tool integration
  - `agent` -- Sub-agent spawning for complex multi-step tasks, with support for specialized types (explore, plan, verify), background execution, tool whitelisting/blacklisting, and context inheritance
  - `send_message` -- Send messages to running sub-agents or query status
  - `task_output` -- Retrieve background sub-agent or bash task output with optional blocking wait
  - `task_stop` -- Stop a running background task by ID
  - `task_create` / `task_list` / `task_get` / `task_update` -- Work task management with dependency tracking (blocks/blockedBy), metadata, and owner assignment
  - Background bash task spawning -- spawn shell commands as tracked background processes with output file collection, completion notifications, and task lifecycle management
  - **File History**: 12 dedicated file history tools (snapshot, diff, rewind, restore, checkout, tag, annotate, search, timeline, batch)
- **Permission Modes**: Three permission modes for different use cases (`ask`, `auto`, `plan`)
- **Sub-Agent System (AgentTool)**: Spawn child agent loops to handle complex, multi-step tasks autonomously. Features:
  - Specialized agent types: `explore` (read-only search), `plan` (read-only architecture planning), `verify` (adversarial testing), and general-purpose (default)
  - Synchronous (block until completion) or asynchronous (run in background) execution
  - Tool access control via whitelisting (`allowed_tools`) and blacklisting (`disallowed_tools`), with a wildcard `"*"` option
  - Context inheritance (fork mode) -- sub-agent can inherit parent's full conversation history
  - Recursive spawning protection -- sub-agents cannot spawn further agents
  - Agent name registry for human-friendly references
  - Completion notifications delivered to parent via buffered channel, injected into conversation context
  - Task lifecycle tracking: pending -> running -> completed/failed/killed, with automatic eviction
- **Work Task Management (TaskTool)**: Structured task tracking system for organizing complex work:
  - Create, list, get, and update tasks with subject, description, and active form
  - Dependency graph with `blocks`/`blockedBy` relationships and cycle detection
  - Task status workflow: pending -> in_progress -> completed/deleted
  - Owner assignment for sub-agent delegation
  - Arbitrary metadata attachment (merge or delete individual keys)
- **Background Bash Tasks**: Spawn shell commands as tracked background processes:
  - Output written to disk files under `.claude/tasks/bash/`
  - Completion notifications with exit code, duration, and status
  - Process tracking for external kill support via `task_stop`
  - Large output truncation (head + tail with truncation marker)
- **MCP Support**: Model Context Protocol client for external tool integration (stdio + HTTP/SSE transports). Supports both project-level `.mcp.json` and home directory `~/.claude/.mcp.json`
- **Skills System**: Extensible skill loader with read_skill, list_skills, and search_skills, plus a SkillTracker for progressive disclosure across turns. Supports both workspace skills and binary-bundled builtin skills
- **Session Memory**: Persistent structured notes across the session stored in `.claude/session_memory.md`. Notes are categorized (preference, decision, state, reference) and flushed to disk periodically. Used by SM-compact as the summary source
- **Error Classification**: 15-category structured error taxonomy (retryable, context overflow, rate limit, auth, billing, tool pairing, timeout, overloaded, etc.) with recovery hints, key rotation, and fallback suggestions
- **Crash Recovery**: Per-call transcript flush to `.claude/transcripts/`, truncated line handling, tool pairing validation, and role alternation repair on resume
- **API Message Normalization**: JSON key sorting and whitespace normalization for KV cache reuse (prefix caching)
- **Prompt Caching**: Anthropic-style prompt caching with cache control markers (system + 3 breakpoints)
- **Rate Limiting**: Response-header-based rate limit tracking with retry delay estimation
- **System Prompt Caching**: CachedSystemPrompt with dirty flag, avoids rebuilding on every API call
- **Preflight Compression**: Automatically compresses long resumed sessions to ~100k tokens before the first API call
- **CLAUDE.md Support**: Automatically loads project-specific instructions from `CLAUDE.md` in the project root
- **Post-Compact Recovery**: After compaction, re-injects recently read file content and used skill content to prevent context loss

## Installation

```bash
go build -o miniclaudecode .
```

## Usage

```bash
# Interactive mode (streaming is off by default)
./miniclaudecode

# Enable streaming output
./miniclaudecode --stream

# Specify permission mode
./miniclaudecode --mode ask

# Specify model
./miniclaudecode --model claude-sonnet-4-6

# Specify project directory
./miniclaudecode --dir /path/to/project

# Resume a previous session
./miniclaudecode --resume last

# One-shot mode (single prompt, then exit)
./miniclaudecode "Explain this code"

# Combine options
./miniclaudecode --stream --mode auto --dir /path/to/project --resume last
```

### Slash Commands (in interactive mode)

- `/help` -- Show available commands
- `/resume [session]` -- Resume a previous conversation session (use `last`, number, or filename)
- `/compact` -- Force context compaction
- `/partialcompact [up_to|from] [pivot]` -- Directional partial compaction (default: up_to, auto pivot)
- `/clear` -- Clear conversation history
- `/mode [auto|ask|plan]` -- Switch permission mode
- `/tools` -- List all available tools
- `/quit` (or `/exit`, `/q`) -- Exit

### @ Context References

Inject external context directly into your prompt:

```
Read the main module @file:src/main.go and check the staged changes @staged
```

Supported references:
- `@file:path[:start-end]` -- File content with optional line range (e.g., `@file:main.go:10-50`)
- `@folder:path` -- Directory tree listing (max 3 levels deep)
- `@staged` -- Git staged diff
- `@diff` -- Git unstaged diff
- `@git:N` -- Git commit diff (N = number of commits, default 1, max 10)
- `@url:URL` -- Web page content (HTML extracted and cleaned)

Token budget guardrails:
- 50% hard limit: injection refused if it exceeds 50% of context window
- 25% soft limit: warning issued if it exceeds 25% of context window

## Configuration

Configuration is stored in `.claude/settings.json`:

```json
{
  "env": {
    "ANTHROPIC_AUTH_TOKEN": "your-api-key",
    "ANTHROPIC_BASE_URL": "https://api.anthropic.com",
    "ANTHROPIC_MODEL": "claude-sonnet-4-6"
  }
}
```

Or use environment variables:

```bash
export ANTHROPIC_API_KEY="your-api-key"
export ANTHROPIC_BASE_URL="https://api.anthropic.com"
export ANTHROPIC_MODEL="claude-sonnet-4-6"
```

Priority order: command-line flags > environment variables > `.claude/settings.json` > home `~/.claude/settings.json`

### MCP Configuration

Create `.mcp.json` in your project root (Claude Code compatible format):

```json
{
  "mcpServers": {
    "filesystem": {
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-filesystem", "/path/to/dir"]
    },
    "remote-server": {
      "url": "https://example.com/mcp"
    }
  }
}
```

### CLAUDE.md

Place a `CLAUDE.md` file in your project root to provide project-specific instructions that are automatically loaded into the system prompt.

## Compaction Modes Explained

### Micro-Compact (Every Turn)

Lightweight clearing of old tool results that are beyond a configurable keep-window (default: 5 recent). Uses a dedup pass (skips already-cleared results) and a whitelist approach (only clears read/exec/edit/grep/glob/web tools; preserves git, memory, skill, list_dir results). Zero API cost, runs every turn.

### SM-Compact (Session Memory Compact)

Uses Session Memory notes as the compaction summary instead of calling the LLM API. This saves an API call and leverages incrementally collected notes. Triggered when session memory has content and token count exceeds threshold. Includes post-compact recovery (re-injects recently read files and used skills).

### LLM-Driven Compaction

Full summary generation via the LLM API. Uses a 3-pass pre-pruning strategy:
1. **Dedup**: Replaces duplicate tool results with reference markers (FNV-1a hash)
2. **Summarize**: Replaces old tool results with one-line summaries (`[tool_name] -> status, N lines`)
3. **Truncate**: Limits large tool arguments to 2000 chars

Includes iterative summary updates (merges new information into previous summary), sensitive info redaction (API keys, passwords, tokens), and cooldown anti-thrashing (skips if tokens haven't grown 25% since last compaction).

### Partial Compact (Directional)

Summarizes a portion of conversation while preserving the other portion:

- **`up_to` (default)**: Summarize entries before pivot, keep recent context intact. Use when early conversation is less relevant.
- **`from`**: Summarize entries after pivot, keep early context intact. Use when recent conversation has redundant tool output.

Both directions adjust the pivot to avoid splitting tool_use/tool_result pairs.

### Reactive Compact

Triggers compaction when token count spikes above a threshold (default: 5000 tokens delta) between turns. Catches situations where a large file read or search result suddenly inflates context, before it becomes a problem.

### 4-Phase Fallback Chain

When automatic compaction is triggered:
1. **Compact**: Round-based, keeps last N rounds, omits the rest with a boundary marker
2. **SmartCompact**: Turn-based, keeps first 2 + last 2 turns, collapses the middle
3. **SelectiveCompact**: Clears readable tool outputs (read_file, grep, glob, web_fetch), preserves write/exec tools
4. **AggressiveTruncate**: Hard truncation to minimum viable context

## Session Memory

Session Memory (`session_memory.md`) maintains persistent structured notes across the session. Notes are categorized:

- `preference` -- User preferences and settings
- `decision` -- Key decisions made during the session
- `state` -- Current work state and progress
- `reference` -- Important references and resources

The memory:
- Persists to `.claude/session_memory.md` every 30 seconds (or on exit)
- Deduplicates identical notes (same category + content)
- Is used by SM-compact as the summary source (no LLM API call needed)
- Is automatically injected into the system prompt each turn
- Is searchable via the `memory_search` tool

## Permission Modes

- **`ask`** (default): Potentially dangerous operations (write, exec, delete) require user confirmation at the terminal. Safe operations proceed automatically.
- **`auto`**: All operations are auto-approved. Use with caution.
- **`plan`**: Only read-only operations are allowed. Write operations are blocked.

Switch modes interactively with `/mode auto`, `/mode ask`, or `/mode plan`.

## Sub-Agent System

The `agent` tool spawns a child `AgentLoop` with isolated conversation context and restricted tool access. Sub-agents are managed by a `TaskStore` that tracks lifecycle, results, and notifications.

### Agent Types

| Type | Read-Only | Purpose |
|------|-----------|---------|
| *(empty)* | No | General-purpose agent with full tool access |
| `explore` | Yes | Codebase search and analysis specialist |
| `plan` | Yes | Architecture and implementation planning specialist |
| `verify` | No* | Adversarial verification specialist |

*Verify agents can write ephemeral test scripts to temp directories but cannot modify the project.

### Tool Access Control

Sub-agent tool access is filtered in layers:

1. **Layer 1**: Global disallowed tools (always denied, e.g., `agent` -- prevents recursive spawning)
2. **Layer 2**: Async-specific disallowed tools (additionally denied for background agents)
3. **Layer 3**: Agent type deny list (e.g., `explore`/`plan` agents have `write_file`, `edit_file`, `exec`, `git` denied)
4. **Layer 4**: Explicit caller disallowed tools

After filtering, if `allowed_tools` (whitelist) is provided, only those tools are included. Use `["*"]` as a wildcard to allow all non-disallowed tools.

### Synchronous vs Asynchronous

- **Synchronous** (default): Blocks until the sub-agent completes. Returns the result directly.
- **Asynchronous** (`run_in_background=true`): Launches the sub-agent in a background goroutine. Returns the `agentId` immediately. Completion notifications are delivered via a buffered channel and injected into the parent's conversation context on the next prompt.

### Context Inheritance (Fork Mode)

When `inherit_context=true`, the sub-agent receives a clone of the parent's conversation history. Tool results are preserved as-is (not replaced with placeholders) so the child can reference them. Compact boundaries and attachments are excluded from the clone.

### Background Bash Tasks

The `exec` tool supports background bash task spawning. Commands are run as detached OS processes with output written to `.claude/tasks/bash/<taskID>.output`. The TaskStore tracks the process for kill support via `task_stop`. Completion notifications include exit code, duration, and status.

### Resuming Async Agents

Completed async agents can be resumed from their transcript via `ResumeAsyncAgent`. This creates a new `AgentLoop` loaded from the stored transcript, allowing continuation of a previously completed background task.

## Work Task Management

The work task system (distinct from the sub-agent task store) provides structured tracking for LLM work items:

- **Create**: `task_create` -- create a task with subject, description, active form, and optional metadata
- **List**: `task_list` -- display all tasks in a table with ID, subject, status, owner, and dependency info
- **Get**: `task_get` -- retrieve full details including description, metadata, and dependency graph
- **Update**: `task_update` -- change status, assign owner, edit description, or add dependency edges

### Dependency Tracking

Tasks support `blocks`/`blockedBy` relationships. Adding a `blockedBy` edge performs cycle detection using BFS across both `Blocks` and `BlockedBy` edges. Invalid references (non-existent task IDs) are silently removed. When a task is deleted, all references to it are cleaned up from other tasks.

## Architecture

```
miniClaudeCode-go/
├── main.go                  # Entry point and REPL with slash commands
├── agent_loop.go            # Core agent loop with IterationBudget, preflight compression, notification system
├── agent_sub.go             # Sub-agent system: SpawnSubAgent, specialized agent types (explore/plan/verify), fork mode
├── agent_task.go            # Sub-agent task store (TaskStore, TaskState), background bash task spawning
├── work_task.go             # Work task management (WorkTaskStore, dependency graph with cycle detection)
├── streaming.go             # Streaming with ThinkFilter, StreamProgress, StreamAdapter
├── context.go               # ConversationContext with tool pairing and role alternation
├── context_references.go    # @ reference expansion (file, folder, git, url)
├── compact.go               # All compaction strategies (micro, SM, LLM, partial, reactive)
├── error_types.go           # 15-category structured error classification
├── normalize.go             # API message normalization for KV cache reuse
├── permissions.go           # Permission gate (ask/auto/plan modes)
├── config.go                # Configuration loading from file/env/flags
├── system_prompt.go         # CachedSystemPrompt builder
├── prompt_caching.go        # Anthropic prompt caching (cache_control markers)
├── rate_limit.go            # Rate limit tracking with retry delay estimation
├── retry_utils.go           # Retry utilities with exponential backoff and jitter
├── filehistory.go           # File version history and snapshots
├── session_memory.go        # Session memory with persistent notes
├── skills/                  # Skill loading and tracking system
│   ├── loader.go           # SkillLoader with workspace and builtin support
│   └── tracker.go         # SkillTracker for progressive disclosure
├── tools/                   # Built-in tool implementations
│   ├── base.go            # ToolResultMetadata and schema validation
│   ├── coercion.go        # Argument type coercion
│   ├── agent_tool.go      # AgentTool -- sub-agent spawning with type, model, background support
│   ├── send_message_tool.go # SendMessageTool -- send/query running sub-agents
│   ├── task_tool.go       # TaskCreate/List/Get/Update/Stop tools for work task management
│   ├── task_output_tool.go # TaskOutputTool -- retrieve background task output with blocking wait
│   ├── exec_tool.go       # Shell command execution
│   ├── file_read.go       # File reading
│   ├── file_write.go      # File writing
│   ├── file_edit.go       # File editing
│   ├── multi_edit.go      # Multi-edit operations
│   ├── glob_tool.go       # Glob pattern matching
│   ├── grep_tool.go       # Grep search
│   ├── list_dir.go        # Directory listing
│   ├── web_search.go      # Web search
│   ├── exa_search.go      # Exa-powered search
│   ├── web_fetch.go       # Web content fetching
│   ├── fileops.go         # File operations (copy, move, delete, chmod)
│   ├── process.go         # Process management
│   ├── git_tool.go        # Git operations
│   ├── system_tool.go     # System information
│   ├── terminal_tool.go   # tmux/screen management
│   ├── runtime_info.go    # Go runtime info
│   ├── skill_tools.go     # read_skill, list_skills, search_skills
│   ├── memory_tool.go     # memory_add, memory_search
│   ├── mcp_tools.go       # list_mcp_tools, call_mcp_tool, mcp_server_status
│   └── file_history_tools.go # 12 file history tools
├── mcp/                    # MCP client (stdio + HTTP/SSE transports)
│   └── client.go          # MCP protocol implementation
├── transcript/             # Crash-safe JSONL conversation logging
│   └── transcript.go      # Transcript reader/writer
└── go.mod
```

## Compatibility

Works with Anthropic API and compatible endpoints. Set the model via `ANTHROPIC_MODEL` environment variable or `--model` flag.

## License

MIT
