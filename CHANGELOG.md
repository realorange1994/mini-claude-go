# Changelog

All notable changes to this project will be documented in this file.

## [Unreleased]

### Added
- Complete LSP protocol implementation (Go/TypeScript/Python)
- ACP IDE integration (Agent Communication Protocol)
- Plugin system with hook-based architecture
- Workflow runtime for multi-step orchestration
- Inter-agent inbox for persistent messaging
- Session sharing for collaborative debugging
- Common utility functions (BufferPool, StringPool)
- End-to-end integration tests (9 tests)
- CI/CD pipeline with GitHub Actions

### Changed
- Enhanced LSP manager with full protocol support
- Refactored work_task.go to use common utilities
- Updated README with all new features

## [1.0.0] - 2026-06-21

### Added
- Steps 1-10: Core optimizations
  - MCP extraction
  - Task system (persistence, priority, tags, dependencies)
  - AI reasoning layer (reasoning pruning, think-only detection)
  - Pressure level tracking (0-3 levels)
  - Smart truncation (error-aware head+tail)
  - Session checkpoint system
  - Wildcard permission matching
  - Subagent enhancement (completion gate, structured output, health check)
  - Streaming optimization (unified retry, smart delay)
  - Testing enhancement (Mock LLM, Test Fixtures)

- P1-P9: MiMo-Code inspired improvements
  - P1: Subagent message isolation (agent_id)
  - P2: Memory FTS search (BM25 ranking)
  - P3: Post-compact tool re-announcement
  - P4: Session forking
  - P5: Task Gate Stop
  - P6: Budgeted Read
  - P7: Two-Level Pruning
  - P8: Checkpoint Writer
  - P9: Auto-Dream

- P14: Goal/Judge stop condition

- 1A-4C: 12-feature batch
  - 1A: Provider message transform layer
  - 1B: Output-Capped Context
  - 1C: Auto-Continue After Compaction
  - 2A: Sophisticated truncation service
  - 2B: Per-tool invocation style
  - 2C: Doom Loop permission gate
  - 3A: Session revert with snapshot restoration
  - 3B: Session diff/summary tracking
  - 3C: Step classification (6 categories)
  - 4A: Checkpoint validator with quarantine
  - 4B: Progress reconciliation
  - 4C: Boundary adjustment for API invariants

- 7 new features
  - Code Search (Exa API)
  - Apply Patch (atomic multi-file edits)
  - Auto-Formatter (gofmt, prettier, ruff, etc.)
  - External Session Import (Claude Code, Codex)
  - Max Mode (multi-candidate + judge)
  - Git Snapshot (git-based snapshot/restore)
  - LSP Integration (language server protocol)

- 6 additional features
  - Skill Discovery (remote skill discovery)
  - Git Worktree Service (full lifecycle management)
  - Inter-Agent Inbox (persistent messaging)
  - Session Sharing (cloud sync)
  - Plugin System (hook-based architecture)
  - ACP IDE Integration (Agent Communication Protocol)
  - Workflow Runtime (multi-step orchestration)

### Technical Details
- 771 Go files
- ~7 MB codebase
- 286 tests (100% pass rate)
- GitHub Actions CI/CD
- MIT License
