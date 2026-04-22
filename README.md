# mini-claude-go

A lightweight, distilled implementation of Claude Code's agent loop framework written in Go.

## Overview

mini-claude-go is a minimal AI agent framework that implements the core agentic loop pattern similar to Claude Code. It provides a tool-use paradigm where an LLM can execute various tools to accomplish complex tasks.

## Features

- **Agent Loop**: Implements the core agentic loop with turn-based conversation, tool execution, and context management
- **Streaming Support**: Real-time streaming output with thinking block handling for various LLM providers
- **Tool System**: 14+ built-in tools including:
  - `exec` - Shell command execution
  - `read_file` / `write_file` / `edit_file` / `multi_edit` - File operations
  - `glob` / `grep` / `list_dir` - File system search and navigation
  - `web_search` / `web_fetch` - Web search and content fetching
  - `fileops` - File operations (copy, move, delete)
  - `process` - Process management
  - `read_image` - Image file reading
- **Permission Modes**: Three permission modes for different use cases:
  - `auto` - Full automation
  - `ask` - Interactive permission prompts
  - `plan` - Read-only planning mode
- **MCP Support**: Model Context Protocol client for external tool integration
- **Context Recovery**: Automatic context truncation and recovery on context length errors
- **Transcript Logging**: Full conversation logging for debugging and analysis

## Installation

```bash
git clone https://github.com/realorange1994/mini-claude-go.git
cd mini-claude-go
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
```

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
mini-claude-go/
+-- agent_loop.go      # Core agent loop implementation
+-- streaming.go       # Streaming event handling
+-- context.go         # Conversation context management
+-- permissions.go     # Permission gate implementation
+-- tools/             # Built-in tool implementations
+-- mcp/               # MCP client support
+-- skills/            # Skill loading system
+-- transcript/        # Conversation logging
```

## Compatibility

Works with Anthropic API and compatible endpoints. Tested with:
- Anthropic Claude models
- MiniMax M2.5 (via compatible proxy)

## License

MIT
