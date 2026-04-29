# miniClaudeCode-go

A lightweight, production-grade implementation of Claude Code's agent loop framework written in Go.

## Overview

miniClaudeCode-go is a minimal AI agent framework that implements the core agentic loop pattern similar to Claude Code. It provides a tool-use paradigm where an LLM can execute various tools to accomplish complex tasks, with robust error handling, context management, and crash recovery.

## Features

- **Agent Loop**: Core agentic loop with turn-based conversation, tool execution, iteration budget, and context management
- **Streaming Support**: Real-time streaming output with thinking block filtering, progress tracking (TTFB, throughput), and enhanced retry strategies
- **Intelligent Context Compaction**: 4-phase automatic context degradation keeps conversations productive in limited context windows
- **@ Context References**: Inject file content, folder listings, git diffs, and URLs into prompts with `@file:path`, `@folder:path`, `@staged`, `@diff`, `@git:N`, `@url:URL`
- **Tool System**: 17+ built-in tools with argument type coercion and schema validation:
  - `exec` — Shell command execution with safety patterns
  - `read_file` / `write_file` / `edit_file` / `multi_edit` — File operations
  - `glob` / `grep` / `list_dir` — File system search and navigation
  - `web_search` / `web_fetch` — Web search and content fetching
  - `fileops` — File operations (copy, move, delete, chmod, symlink)
  - `process` — Process management (list, kill, pgrep, top, pstree)
  - `git` — Full git operations (clone, commit, push, pull, branch, merge, rebase, stash, worktree, and more)
  - `system` — System info (uname, df, free, uptime, hostname, arch)
  - `terminal` — tmux/screen session management
  - `runtime_info` — Go runtime and system information
- **File History**: Automatic snapshot, diff, rewind, restore, and tag-based file version management
- **Permission Modes**: Three permission modes for different use cases (auto, ask, plan)
- **MCP Support**: Model Context Protocol client for external tool integration
- **Skills System**: Extensible skill loader with read_skill, list_skills, and search_skills, plus a skill tracker for progressive disclosure across turns
- **Error Classification**: 15-category structured error taxonomy with retry hints, key rotation, and fallback suggestions
- **Crash Recovery**: Per-call transcript flush, truncated line handling, tool pairing validation, and role alternation repair on resume
- **API Message Normalization**: JSON key sorting and whitespace normalization for KV cache reuse (prefix caching)
- **Prompt Caching**: Anthropic-style prompt caching with cache control markers
- **Rate Limiting**: Token bucket rate limiter with exponential backoff

## Installation

```bash
go build -o miniclaudecode .
```

## Usage

```bash
# Interactive mode
./miniclaudecode

# With streaming
./miniclaudecode --stream

# Specify permission mode
./miniclaudecode --mode ask

# Specify model
./miniclaudecode --model claude-sonnet-4-6

# Specify project directory
./miniclaudecode --dir /path/to/project

# Resume a previous session
./miniclaudecode --resume last
```

### Slash Commands (in interactive mode)

- `/help` — Show available commands
- `/resume [session]` — Resume a previous conversation session
- `/compact` — Force context compaction
- `/clear` — Clear conversation history
- `/mode [auto|ask|plan]` — Switch permission mode
- `/quit` — Exit

### @ Context References

Inject external context directly into your prompt:

```
Read the main module @file:src/main.go and check the staged changes @staged
```

Supported references:
- `@file:path[:start-end]` — File content with optional line range
- `@folder:path` — Directory listing
- `@staged` — Git staged diff
- `@diff` — Git unstaged diff
- `@git:N` — Git commit diff (N = commit count or hash)
- `@url:URL` — Web page content

## Configuration

Configuration is stored in `.claude/settings.json`:

```json
{
  "env": {
    "ANTHROPIC_API_KEY": "your-api-key",
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

## Architecture

```
miniClaudeCode-go/
├── main.go                  # Entry point and REPL
├── agent_loop.go            # Core agent loop with iteration budget and preflight compression
├── streaming.go             # Streaming event handling with think filter and progress tracking
├── context.go               # Conversation context with tool pairing and role alternation
├── context_references.go    # @ reference expansion (file, folder, git, url)
├── compact.go               # 4-phase intelligent context compaction
├── error_types.go           # 15-category structured error classification
├── normalize.go             # API message normalization for KV cache reuse
├── permissions.go           # Permission gate implementation
├── config.go                # Configuration loading
├── prompt_caching.go        # Anthropic prompt caching support
├── rate_limit.go            # Token bucket rate limiter
├── retry_utils.go           # Retry utilities with exponential backoff
├── filehistory.go           # File version history and snapshots
├── skills/                  # Skill loading and tracking system
├── tools/                   # Built-in tool implementations
│   ├── coercion.go          # Argument type coercion
│   └── ...                  # 17+ tool implementations
├── mcp/                     # MCP client support
└── transcript/              # Crash-safe JSONL conversation logging
```

## Compatibility

Works with Anthropic API and compatible endpoints. Tested with:
- Anthropic Claude models (sonnet-4-6, opus-4-6, haiku-4-5)
- OpenAI-compatible proxies
- MiniMax models (via compatible proxy)

## License

MIT