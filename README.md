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
| **Memory** | `memory_add`, `memory_search` |
| **Skills** | `read_skill`, `list_skills`, `search_skills` |
| **MCP** | `list_mcp_tools`, `call_mcp_tool`, `mcp_server_status` |
| **Other** | `brief`, `runtime_info`, `tool_search` |

**File History**: 17 dedicated file history tools for version tracking and diffing: `file_history`, `file_history_read`, `file_history_grep`, `file_history_diff`, `file_history_search`, `file_history_summary`, `file_history_timeline`, `file_history_annotate`, `file_history_tag`, `file_history_checkout`, `file_history_batch`, `file_restore`, `file_rewind`.

### Permission Modes

Three permission strategies with an LLM-based security classifier in auto mode:

| Mode | Behavior |
|------|----------|
| `ask` (default) | Dangerous operations (write, exec, delete) require user confirmation. Safe commands (from allowlist) proceed automatically. |
| `auto` | LLM-based security classifier evaluates non-whitelisted actions. Read-only tools auto-allowed. After 3 consecutive denials, falls back to interactive prompt. |
| `plan` | Read-only mode. All write and exec operations blocked. |

Switch modes interactively with `/mode auto`, `/mode ask`, or `/mode plan`.

### Auto Mode Classifier

In `auto` mode, an LLM-powered security classifier (using Anthropic SDK's `tool_use` structured output) evaluates actions that are not in the safe whitelist:

- **Git**: read-only operations (`info`, `status`, `log`, `diff`, `show`, `reflog`, `blame`, `describe`, `shortlog`, `ls-tree`, `rev-parse`, `rev-list`) are auto-allowed. Write/destructive operations (`push`, `commit`, `merge`, `rebase`, `reset`, `clean`, etc.) go through the classifier.
- **Exec**: safe command prefixes are auto-allowed (file listing/reading, search, diff, version checks, build/test/lint, network inspection). Dangerous patterns (`rm`, `sudo`, `chmod`, `chown`, `mkfs`, `dd if=`, `curl|bash`, `wget|sh`, redirects to system directories) are not auto-allowlisted. Unknown commands go through the classifier. Combined commands (`&&`, `||`, `;`) are split and each segment is checked independently — `safe_cmd && evil_cmd` correctly blocks.
- **Process**: read-only operations (`list`, `pgrep`, `top`, `pstree`, `ps`) are auto-allowed. Destructive operations (`kill`, `pkill`, `terminate`) go through the classifier.
- **Fileops**: read-only operations (`read`, `stat`, `checksum`, `exists`, `ls`) are auto-allowed. Write/destructive operations (`rmrf`, `rm`, `mv`, `cp`, `chmod`, etc.) go through the classifier — no unconditional hard blocks, matching Claude Code's upstream design.
- **Cache**: classifier results cached by command/operation with 5-minute TTL to reduce API calls.
- **Fallback**: after 3 consecutive classifier denials, falls back to interactive user prompt.

#### Two-Stage Classifier

The classifier uses a two-stage approach modeled after Claude Code's upstream `yoloClassifier.ts`:

- **Stage 1 (fast)**: 2112 max_tokens — quick allow/block decision. Most safe commands are decided here.
- **Stage 2 (thinking)**: 6144 max_tokens with 2048-token thinking padding — full chain-of-thought reasoning with a richer prompt. Triggered when Stage 1 blocks, for deeper security analysis.

#### Dangerous Removal Path Validation

`rm`/`rmdir` commands are checked against system-critical paths before classification. The following paths are hard-blocked (cannot be auto-allowlisted):
- `/` (root directory)
- `~` and `*` / `/*` (wildcards)
- Direct children of `/` (`/usr`, `/tmp`, `/etc`, `/bin`, `/var`, etc.)
- Windows drive roots (`C:\`, `D:\`) and protected directories (`C:\Windows`, `C:\Users`, `C:\Program Files`)

Project-scoped paths (`./build`, `./node_modules`, `/home/user/project/dist`) pass the path check and are evaluated by the classifier.

#### Classifier Decision Categories

The classifier uses BLOCK ALWAYS semantic categories (modeled after upstream):
1. External Code Execution: `curl|bash`, `wget|sh`, piping to shell
2. Irreversible Local Destruction: `rm -rf`, recursive deletion, file truncation, database drops
3. Unauthorized Persistence: cron jobs, systemd services, shell profile modifications
4. Security Weakening: disabling firewalls, security policies, `chmod 777`
5. Privilege Escalation: `sudo`, `su`, `runas`
6. Unauthorized Network Services: starting servers, listeners, port bindings

### Sub-Agent System

The `agent` tool spawns isolated child agent loops with restricted tool access:

- **Agent types**: general-purpose (default), `explore` (read-only search), `plan` (read-only architecture planning), `verify` (adversarial testing, can write to temp only)
- **Execution modes**: synchronous (blocks until completion) or asynchronous (background goroutine with completion notifications)
- **Tool access control**: layer-based filtering with explicit whitelisting (`allowed_tools`), blacklisting (`disallowed_tools`), and wildcard `"*"` option
- **Context inheritance** (fork mode): sub-agent can receive a clone of the parent's conversation history
- **Recursive spawning protection**: sub-agents cannot spawn further agents
- **Completion notifications** delivered to parent via buffered channel and injected into conversation context

### Work Task Management

Structured task tracking with dependency graph:

- Create, list, get, and update tasks with subject, description, active form, and status workflow (pending -> in_progress -> completed/deleted)
- `blocks`/`blockedBy` dependency relationships with BFS cycle detection
- Owner assignment for sub-agent delegation
- Arbitrary metadata attachment (merge or delete individual keys)
- Cleanup on task deletion (all references removed from other tasks)

### Background Bash Tasks

Spawn shell commands as tracked background processes:

- Output written to disk files under `.claude/tasks/bash/`
- Completion notifications with exit code, duration, and status
- Process tracking for external kill support via `task_stop`
- Large output truncation (head + tail with truncation marker)

### MCP (Model Context Protocol)

External tool integration via stdio and HTTP/SSE transports. Supports Claude Code-compatible `.mcp.json` format (project-level) and `~/.claude/.mcp.json` (home directory).

### Skills System

Extensible skill loader with `read_skill`, `list_skills`, and `search_skills` tools, plus a `SkillTracker` for progressive disclosure across turns. Supports both workspace skills (`skills/` directory) and binary-bundled builtin skills (shipped alongside the executable).

### Session Memory

Persistent structured notes across the session, stored in `.claude/session_memory.md`:

- Categories: `preference`, `decision`, `state`, `reference`
- Periodic flush to disk (every 30 seconds) and on exit
- Deduplication of identical notes
- Used by SM-compact as the summary source
- Automatically injected into the system prompt each turn
- Searchable via `memory_search` tool

### Error Classification & Recovery

- **15-category structured error taxonomy**: retryable, context overflow, rate limit, auth, billing, tool pairing, timeout, overloaded, etc., with recovery hints
- **Key rotation** and fallback suggestions for auth/billing errors
- **Crash recovery**: per-call transcript flush to `.claude/transcripts/`, truncated line handling, tool pairing validation, and role alternation repair on resume
- **API message normalization**: JSON key sorting and whitespace normalization for KV cache reuse (prefix caching)

### Performance

- **Prompt caching**: Anthropic-style prompt caching with `cache_control` markers (system + 3 breakpoints)
- **CachedSystemPrompt**: dirty flag avoids rebuilding system prompt on every API call
- **Preflight compression**: automatically compresses long resumed sessions to ~100k tokens before the first API call
- **Rate limiting**: response-header-based rate limit tracking with retry delay estimation
- **Retry utilities**: exponential backoff with jitter

### CLAUDE.md Support

Automatically loads project-specific instructions from `CLAUDE.md` in the project root.

## Installation

```bash
git clone https://github.com/your-org/miniClaudeCode-go.git
cd miniClaudeCode-go
go build -o miniclaudecode .
```

Cross-platform builds are supported (Windows, Linux, macOS):

```bash
GOOS=linux GOARCH=amd64 go build -o miniclaudecode-linux .
GOOS=darwin GOARCH=arm64 go build -o miniclaudecode-macos .
GOOS=windows GOARCH=amd64 go build -o miniclaudecode.exe .
```

## Usage

```bash
# Interactive REPL (default: ask mode, no streaming)
./miniclaudecode

# Enable streaming output
./miniclaudecode --stream

# Specify permission mode
./miniclaudecode --mode ask
./miniclaudecode --mode auto
./miniclaudecode --mode plan

# Specify model
./miniclaudecode --model claude-sonnet-4-20250514

# Specify project directory
./miniclaudecode --dir /path/to/project

# Resume a previous session
./miniclaudecode --resume last

# One-shot mode (single prompt, then exit)
./miniclaudecode "Review the code in main.go"

# Combine options
./miniclaudecode --stream --mode auto --dir /path/to/project "Fix the bug in handler.go"
```

### CLI Flags

| Flag | Description | Default |
|------|-------------|---------|
| `--model` | Anthropic model to use | (required) |
| `--api-key` | API key (overrides env and config) | |
| `--base-url` | Custom API base URL | |
| `--mode` | Permission mode (`ask`, `auto`, `plan`) | `ask` |
| `--max-turns` | Max agent loop turns per message | `90` |
| `--stream` | Enable streaming output | `false` |
| `--dir` | Project directory | current dir |
| `--resume` | Resume from transcript (path, number, or `last`) | |

### Slash Commands (Interactive Mode)

| Command | Description |
|---------|-------------|
| `/help` | Show available commands |
| `/resume [session]` | Resume a previous session (`last`, number, or filename) |
| `/compact` | Force context compaction |
| `/partialcompact [up_to\|from] [pivot]` | Directional partial compaction |
| `/clear` | Clear conversation history |
| `/mode [ask\|auto\|plan]` | Switch permission mode |
| `/tools` | List all available tools |
| `/quit` (or `/exit`, `/q`) | Exit |

### @ Context References

Inject external context directly into prompts:

```
Read the main module @file:src/main.go and check the staged changes @staged
```

Supported references:

| Reference | Description |
|-----------|-------------|
| `@file:path[:start-end]` | File content with optional line range (e.g., `@file:main.go:10-50`) |
| `@folder:path` | Directory tree listing (max 3 levels deep) |
| `@staged` | Git staged diff |
| `@diff` | Git unstaged diff |
| `@git:N` | Git commit diff (N = number of commits, default 1, max 10) |
| `@url:URL` | Web page content (HTML extracted and cleaned) |

## Configuration

Configuration priority: **CLI flags > environment variables > `.claude/settings.json` > `~/.claude/settings.json`**

### Environment Variables

```bash
export ANTHROPIC_API_KEY="your-api-key"
# or
export ANTHROPIC_AUTH_TOKEN="your-api-key"
export ANTHROPIC_BASE_URL="https://api.anthropic.com"
export ANTHROPIC_MODEL="claude-sonnet-4-20250514"
```

### Settings File

Configuration is stored in `.claude/settings.json` (project-level) or `~/.claude/settings.json` (home directory):

```json
{
  "env": {
    "ANTHROPIC_AUTH_TOKEN": "your-api-key",
    "ANTHROPIC_BASE_URL": "https://api.anthropic.com",
    "ANTHROPIC_MODEL": "claude-sonnet-4-20250514"
  }
}
```

### MCP Configuration

Create `.mcp.json` in your project root (Claude Code-compatible format):

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

Home directory MCP config: `~/.claude/.mcp.json`

### CLAUDE.md

Place a `CLAUDE.md` file in your project root to provide project-specific instructions automatically loaded into the system prompt.

## Architecture

```
miniClaudeCode-go/
├── main.go                  # Entry point, CLI flags, REPL loop
├── agent_loop.go            # Core agent loop (IterationBudget, notifications, sub-agent spawning)
├── agent_sub.go             # Sub-agent system: SpawnSubAgent, specialized types, fork mode
├── agent_task.go            # Task store (TaskStore, TaskState), background bash tasks
├── work_task.go             # Work task management (dependency graph, cycle detection)
├── auto_classifier.go       # Two-stage LLM classifier, dangerous path validation, combined command splitting, operation-level allowlists
├── permissions.go           # Permission gate (ask/auto/plan modes), denied patterns
├── streaming.go             # SSE streaming, ThinkFilter, StreamProgress, CollectHandler
├── context.go               # ConversationContext, entry types, compaction boundaries
├── context_references.go    # @ reference expansion (file, folder, git, url)
├── compact.go               # All compaction strategies (micro, SM, LLM, partial, reactive)
├── config.go                # Configuration loading (file/env/flags), defaults, registry setup
├── system_prompt.go         # CachedSystemPrompt builder with dirty flag
├── prompt_caching.go        # Anthropic prompt caching (cache_control markers)
├── error_types.go           # 15-category structured error classification
├── normalize.go             # API message normalization for KV cache reuse
├── rate_limit.go            # Rate limit tracking with retry delay estimation
├── retry_utils.go           # Retry with exponential backoff and jitter
├── filehistory.go           # File version history and snapshots
├── session_memory.go        # Session memory with persistent notes
├── skills/                  # Skill loading and progressive disclosure
│   ├── loader.go           # SkillLoader (workspace + builtin skills)
│   └── tracker.go          # SkillTracker for progressive disclosure
├── tools/                   # Built-in tool implementations
│   ├── base.go             # ToolResultMetadata, schema validation
│   ├── coercion.go         # Argument type coercion
│   ├── agent_tool.go       # AgentTool (sub-agent spawning)
│   ├── send_message_tool.go # SendMessageTool
│   ├── task_tool.go        # TaskCreate/List/Get/Update
│   ├── task_output_tool.go # TaskOutputTool (blocking wait)
│   ├── exec_tool.go        # Shell command execution
│   ├── file_read.go        # File reading
│   ├── file_write.go       # File writing
│   ├── file_edit.go        # File editing
│   ├── multi_edit.go       # Multi-edit operations
│   ├── glob_tool.go        # Glob pattern matching
│   ├── grep_tool.go        # Grep search
│   ├── list_dir.go         # Directory listing
│   ├── web_search.go       # Web search
│   ├── exa_search.go       # Exa-powered search
│   ├── web_fetch.go        # Web content fetching
│   ├── fileops.go          # File operations (copy/move/delete/chmod)
│   ├── process.go          # Process management
│   ├── git_tool.go         # Git operations with dangerous op detection
│   ├── system_tool.go      # System information
│   ├── terminal_tool.go    # tmux/screen management
│   ├── runtime_info.go     # Go runtime info
│   ├── skill_tools.go      # read_skill, list_skills, search_skills
│   ├── memory_tool.go      # memory_add, memory_search
│   ├── mcp_tools.go        # list_mcp_tools, call_mcp_tool, mcp_server_status
│   └── file_history_tools.go # File history tools
├── mcp/                    # MCP client (stdio + HTTP/SSE transports)
│   └── client.go           # MCP protocol implementation
├── transcript/             # Crash-safe JSONL conversation logging
│   └── transcript.go       # Transcript reader/writer
└── go.mod
```

## License

MIT
