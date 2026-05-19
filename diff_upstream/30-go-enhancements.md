# Go Enhancements

> Go-specific improvements, sandbox

## Sections Included
- [##] Line 8538-8744 -- ## 33. Sandbox Execution and Security Features
- [##] Line 8779-8797 -- ## C. Top 10 Go Enhancements
- [##] Line 8869-8907 -- ## G. Go-Only Features Not in Upstream
- [##] Line 10119-10184 -- ## 45. Remaining Go Modules Sweep — main.go, retry, normalize, streaming, forked_agent, transcript_builder
- [##] Line 10337-10533 -- ## 49. Go-Specific Enhancements & Adaptations

---

## Content

## 33. Sandbox Execution and Security Features

### Files Compared

**Go security files**:
- `E:\Git\miniClaudeCode-go-github\permissions.go` (691 lines) -- permission gate
- `E:\Git\miniClaudeCode-go-github\tools\exec_tool.go` (1622 lines) -- command safety
- `E:\Git\miniClaudeCode-go-github\tools\file_read.go` (469 lines) -- binary/device detection
- `E:\Git\miniClaudeCode-go-github\tools\filesystem_safety.go` (179 lines) -- dangerous files/dirs
- `E:\Git\miniClaudeCode-go-github\permissions\path_validation.go` (248 lines) -- path validation
- `E:\Git\miniClaudeCode-go-github\permissions\internal_paths.go` (249 lines) -- internal path checks
- `E:\Git\miniClaudeCode-go-github\permissions\rules_loader.go` (166 lines) -- rule loading
- `E:\Git\miniClaudeCode-go-github\permissions\auto_strip.go` (190 lines) -- dangerous rule stripping
- `E:\Git\miniClaudeCode-go-github\permissions\rule_parser.go` -- rule parsing
- `E:\Git\miniClaudeCode-go-github\permissions\rule_store.go` -- rule storage
- `E:\Git\miniClaudeCode-go-github\config.go` (line 17-24) -- PermissionMode type

**Upstream security files**:
- `E:\Git\claude-code-upstream\src\utils\sandbox\sandbox-adapter.ts` (986 lines) -- sandbox runtime adapter
- `E:\Git\claude-code-upstream\src\entrypoints\sandboxTypes.ts` (157 lines) -- sandbox configuration types
- `E:\Git\claude-code-upstream\src\utils\permissions\filesystem.ts` (1600+ lines) -- dangerous files/dirs, path safety
- `E:\Git\claude-code-upstream\src\utils\permissions\pathValidation.ts` -- path validation
- `E:\Git\claude-code-upstream\src\utils\Shell.ts` -- sandbox-wrapped shell execution
- `E:\Git\claude-code-upstream\src\setup.ts` (line 420-430) -- sandbox detection for --dangerously-skip-permissions
- `E:\Git\claude-code-upstream\src\utils\env.ts` (line 294) -- Docker detection
- `E:\Git\claude-code-upstream\src\utils\envDynamic.ts` (line 13-14) -- bubblewrap detection
- `E:\Git\claude-code-upstream\src\constants\files.ts` (line 131) -- isBinaryContent
- `E:\Git\claude-code-upstream\src\memdir\teamMemPaths.ts` (line 97-280) -- symlink escape prevention
- `E:\Git\claude-code-upstream\src\bridge\bridgeMain.ts` (line 1774-1788) -- --sandbox / --permission-mode CLI args

### 33.1 Sandbox Execution

| Aspect | Go | Upstream TS |
|--------|-----|-------------|
| **Process sandboxing** | **NONE** -- commands execute directly via `os/exec` without OS-level isolation | **macOS Seatbelt** -- uses `sandbox-exec` profiles for mandatory filesystem and network restrictions (`sandbox-adapter.ts:704-724`) |
| **Linux sandboxing** | **NONE** | **Bubblewrap (bwrap)** -- seccomp-based filesystem namespace isolation. Requires `bwrap` + `socat` binaries (`SandboxDependenciesTab.tsx:64-72`) |
| **WSL2 sandboxing** | **NONE** | **Bubblewrap** -- same as Linux. WSL1 not supported (`sandbox-adapter.ts:488-493, 571-574`) |
| **Windows sandboxing** | **NONE** | **NONE** -- no sandbox on Windows (platform not in supported list) |
| **Sandbox CLI flag** | **NONE** | `--sandbox` / `--no-sandbox` flags at `bridgeMain.ts:1774-1777` |
| **Sandbox settings** | **NONE** | `sandbox.enabled`, `sandbox.failIfUnavailable`, `sandbox.autoAllowBashIfSandboxed`, `sandbox.allowUnsandboxedCommands`, `sandbox.excludedCommands` at `sandboxTypes.ts:94-134` |
| **Sandbox toggle command** | **NONE** | `/sandbox` command at `sandbox-toggle/index.ts:6` with enable/disable/exclude subcommands |
| **Auto-allow if sandboxed** | **N/A** | `autoAllowBashIfSandboxed` -- auto-approves Bash when sandbox is active (`sandbox-adapter.ts:469-472`) |
| **Dangerously disable sandbox** | **N/A** | `dangerouslyDisableSandbox` parameter on Bash tool, checked against `allowUnsandboxedCommands` setting (`sandboxTypes.ts:113-119`, `processBashCommand.tsx:97-124`) |
| **Sandbox initialization** | **N/A** | Async `initialize()` at `sandbox-adapter.ts:730-792` with `SandboxAskCallback` for network permission prompts |
| **Sandbox wrap command** | **N/A** | `wrapWithSandbox(command, binShell, customConfig, abortSignal)` at `sandbox-adapter.ts:704-725` |
| **Sandbox config refresh** | **N/A** | `refreshConfig()` at `sandbox-adapter.ts:798-803` -- reactive update on permission changes |
| **Sandbox unavailable reason** | **N/A** | `getSandboxUnavailableReason()` at `sandbox-adapter.ts:562-592` -- reports missing deps, unsupported platform |
| **Bare git repo scrub** | **N/A** | `scrubBareGitRepoFiles()` at `sandbox-adapter.ts:404-414` -- removes planted bare-repo files after sandboxed command |
| **enabledPlatforms** | **N/A** | Platform restriction list (e.g., `["macos"]`) for enterprise deployments (`sandboxTypes.ts:104-111`) |
| **CCR container mode** | **N/A** | Runs inside CCR containers with `IS_SANDBOX=1` env var (`sessionRunner.ts:312`, `setup.ts:426`) |

**CRITICAL GAP**: Go has zero OS-level process sandboxing. All commands run with full access to the host filesystem, network, and system resources. Upstream's sandbox provides mandatory kernel-level isolation on macOS (Seatbelt) and Linux (Bubblewrap), enforcing filesystem read/write restrictions and network domain filtering at the OS level.

### 33.2 Docker Containerization

| Aspect | Go | Upstream TS |
|--------|-----|-------------|
| **Docker detection** | **NONE** | Checks `/.dockerenv` at `env.ts:294` and via `test -f /.dockerenv` at `envDynamic.ts:13-14` |
| **Bubblewrap detection** | **NONE** | `envDynamic.getIsBubblewrapSandbox()` at `setup.ts:425` |
| **Sandbox environment** | **NONE** | `IS_SANDBOX=1` env var set in container mode (`setup.ts:426`) |
| **Internet access check** | **NONE** | `env.hasInternetAccess()` memoized at `env.ts:28` -- used to gate --dangerously-skip-permissions |
| **Dangerously skip permissions** | **NONE** | `--dangerously-skip-permissions` allowed ONLY in Docker/sandbox with NO internet (`setup.ts:420-430`) |
| **Container ID logging** | **NONE** | OCI container ID extracted from `/proc/self/mountinfo` at `internalLogging.ts:33-64` |
| **CCR container support** | **NONE** | Full container orchestration: upstreamproxy, egress gateway, managed containers (`upstreamproxy/upstreamproxy.ts:4-35`, `relay.ts:24`) |
| **Container lease** | **NONE** | CCR session container lease management (`ccrClient.ts:506`) |

**CRITICAL GAP**: Go cannot run inside Docker containers with permission bypass. It has no container detection, no internet access checks, and no CCR (Claude Code Remote) container integration.

### 33.3 File Path Restrictions

| Aspect | Go | Upstream TS |
|--------|-----|-------------|
| **Path validation entry** | `permissions/ValidatePath()` at `path_validation.go:36-136` | `validatePath()` + `isPathAllowed()` at `pathValidation.ts` |
| **UNC path blocking** | Yes -- `isUncPath()` at `path_validation.go:214-216` and `file_read.go:76-78` | Yes -- `containsVulnerableUncPath()` at `shell/readOnlyCommandValidation.ts` |
| **Tilde expansion** | Yes -- `expandTilde()` at `path_validation.go:196-203`; blocks `~root`, `~+`, `~-` variants | Yes -- `expandPath()` at `path.ts` |
| **Shell expansion blocking** | Yes -- blocks `$VAR`, `%VAR%`, `=prefix` via regex at `path_validation.go:57-61` | Yes -- similar checks in path validation |
| **Glob in writes** | Yes -- blocks `*?[]{} ` in write/create at `path_validation.go:64-68` | Yes -- similar restrictions |
| **Dangerous directories** | `.git`, `.vscode`, `.idea`, `.claude` at `filesystem_safety.go:26-31` | `.git`, `.vscode`, `.idea`, `.claude` at `filesystem.ts:74-79` (identical) |
| **Dangerous files** | `.gitconfig`, `.gitmodules`, `.bashrc`, etc. (10 files) at `filesystem_safety.go:11-22` | Same 9 files at `filesystem.ts:57-68` (identical list) |
| **Claude config protection** | `isClaudeConfigPath()` at `filesystem_safety.go:163-178` -- blocks `.claude/settings.json` etc. | `isClaudeSettingsPath()` at `filesystem.ts:200+` -- same paths |
| **Settings path deny-write** | **Not in sandbox layer** (no sandbox) | Settings files unconditionally denied in sandbox at `sandbox-adapter.ts:232-245` |
| **Internal editable paths** | `IsInternalEditablePath()` at `internal_paths.go:13-47` -- plans, scratchpad, jobs, agent-memory | More comprehensive at `filesystem.ts` -- includes session-memory, projects, tool-results, project-temp, tasks, teams, bundled-skills |
| **Internal readable paths** | `IsInternalReadablePath()` at `internal_paths.go:51-110` -- same categories | Same categories plus more |
| **Skill-scope permissions** | **NONE** | `getClaudeSkillScope()` at `filesystem.ts:101-157` -- narrow allow patterns for individual skills |
| **Path pattern resolution** | Simple string matching | `resolvePathPatternForSandbox()` at `sandbox-adapter.ts:99-119` -- handles `//path`, `/path`, `~/path` conventions |
| **Sandbox filesystem config** | **NONE** | `sandbox.filesystem.{allowWrite,denyWrite,denyRead,allowRead}` at `sandboxTypes.ts:45-85` |
| **Sandbox filesystem rules** | **NONE** | Permission rules (Edit/Read) converted to sandbox mount rules at `sandbox-adapter.ts:304-349` |
| **Managed read paths only** | **NONE** | `allowManagedReadPathsOnly` policy at `sandboxTypes.ts:78-83` |

### 33.4 Command Filtering

| Aspect | Go | Upstream TS |
|--------|-----|-------------|
| **Deny patterns (regex)** | 24+ patterns at `exec_tool.go:78-108` -- rm -rf, dd, shutdown, fork bombs, docker prune, git force push | Similar deny lists in sandbox denyWithinAllow at `main.tsx:728` |
| **Compound command splitting** | Yes -- `splitCompoundCommand()` at `exec_tool.go:1247-1311` | Yes -- similar parsing |
| **Command substitution detection** | Yes -- `detectCommandSubstitution()` at `exec_tool.go:619-680` -- blocks `$()`, backticks, `${...}` | Similar detection |
| **Glob expansion detection** | Yes -- `detectExpansion()` at `exec_tool.go:710-790` -- flags unquoted globs in destructive commands | Similar detection |
| **Safe wrapper stripping** | Yes -- `stripSafeWrappers()` at `exec_tool.go:1337-1421` -- timeout, nice, env, sudo, etc. | Similar stripping |
| **UNC path in commands** | Yes -- `containsVulnerableUncPath()` at `exec_tool.go:536-567` | Yes -- `containsVulnerableUncPath()` in readOnlyCommandValidation |
| **Internal URL detection** | Yes -- `containsInternalURL()` at `exec_tool.go:516-524` -- localhost, private IPs | Similar detection |
| **Read-only command classification** | Yes -- `isReadOnlyCommand()` at `exec_tool.go:1437-1544` -- whitelist of safe commands | Similar classification |
| **Destructive command warnings** | Yes -- `isDestructiveCommand()` at `exec_tool.go:1547-1621` | Similar detection |
| **Redirect target validation** | Yes -- `validateRedirectTargets()` at `exec_tool.go:954-1089` -- blocks redirects to /etc, /dev, ~/.ssh | Similar validation |
| **Environment variable safety** | Yes -- `safeVarNames` map at `exec_tool.go:574-612` -- whitelist of safe env vars | Similar checks |
| **Command exclusion list** | **NONE** | `sandbox.excludedCommands` -- commands exempted from sandboxing (`sandbox-adapter.ts:696-699`) |

### 33.5 Binary File Protection

| Aspect | Go | Upstream TS |
|--------|-----|-------------|
| **Extension-based rejection** | Yes -- 50+ binary extensions at `file_read.go:286-309` -- .exe, .zip, .png, .pdf, etc. | `hasBinaryExtension()` at `constants/files.ts` |
| **Magic bytes detection** | Yes -- `isBinaryMagic()` at `file_read.go:342-459` -- MZ, ELF, PNG, PDF, ZIP, Java class, Wasm, etc. | `isBinaryContent()` at `constants/files.ts:131` -- buffer scan for null bytes |
| **Binary read blocking** | Yes -- returns error at `file_read.go:106-108` and `file_read.go:119-121` | Blocks binary file display with message |
| **PDF handling** | Rejected as binary | PDF extraction support (separate PDF parser) |
| **Image handling** | Rejected as binary | Image support with display (file_read tool handles images) |
| **SVG handling** | Rejected as binary | SVG handled separately from binary |

**Finding**: Go's binary detection is actually MORE comprehensive in magic bytes (checks PE, ELF, GZIP, BZIP2, MP3, JPEG, PNG, GIF, ZIP, XZ, 7Z, WebP, WAV, MP4, Java class, Wasm, Python pyc, Lua bytecode). Upstream uses a simpler null-byte approach in `isBinaryContent()`.

### 33.6 Device File Blocking

| Aspect | Go | Upstream TS |
|--------|-----|-------------|
| **Unix device files** | Yes -- `isDeviceFile()` at `file_read.go:314-337` -- /dev/zero, /dev/random, /dev/stdin, /proc/*/fd/ | Similar blocking in sandbox mount rules |
| **Device write blocking** | Yes -- deny patterns `>/dev/sd` and `dd of=` at `exec_tool.go:86-87` | Similar in sandbox deny rules |

### 33.7 Symlink Following

| Aspect | Go | Upstream TS |
|--------|-----|-------------|
| **Symlink resolution** | Single `filepath.EvalSymlinks()` at `path_validation.go:206-211` -- only resolves the target path itself | Deep symlink resolution at `teamMemPaths.ts:97-280` (PSR M22186) |
| **Symlink escape prevention** | **NONE** -- resolves symlinks once, no containment check | Multi-pass: resolve deepest existing ancestor, verify real path is within real directory. Catches dangling symlinks, symlink-in-symlink attacks |
| **Dangling symlink detection** | **NONE** | `lstat` distinguishes dangling symlinks at `teamMemPaths.ts:131-144` |
| **Symlink deduplication** | **NONE** | `realpath` used for skill file dedup at `loadSkillsDir.ts:108-113` |
| **Path containment check** | **NONE** -- after resolving, no check that resolved path is still within allowed directory | `validateTeamMemWritePath()` checks resolved path stays within team dir at `teamMemPaths.ts:226-257` |

**Gap**: Go resolves symlinks but does not verify the resolved path stays within expected boundaries. Upstream has multi-layer symlink containment checks with dangling symlink detection.

### 33.8 Network Access Restrictions

| Aspect | Go | Upstream TS |
|--------|-----|-------------|
| **OS-level network sandbox** | **NONE** | **macOS Seatbelt** -- network domain filtering via sandbox profile |
| **Allowed domains** | **NONE** | `sandbox.network.allowedDomains` at `sandboxTypes.ts:17` -- list of allowed domain patterns |
| **Denied domains** | **NONE** | Extracted from `permissions.deny` rules with `WebFetch(domain:...)` at `sandbox-adapter.ts:212-220` |
| **Managed domains only** | **NONE** | `allowManagedDomainsOnly` -- enterprise policy at `sandboxTypes.ts:18-24` |
| **Unix socket blocking** | **NONE** | `allowUnixSockets`, `allowAllUnixSockets` at `sandboxTypes.ts:25-36` |
| **Local binding** | **NONE** | `allowLocalBinding` at `sandboxTypes.ts:37` |
| **HTTP/SOCKS proxy** | **NONE** | `httpProxyPort`, `socksProxyPort` at `sandboxTypes.ts:38-39` for MITM proxy support |
| **Network permission prompts** | **NONE** | `SandboxAskCallback` -- interactive prompts for blocked network access (`sandbox-adapter.ts:746-755`) |
| **Internal URL warning** | Yes -- `containsInternalURL()` at `exec_tool.go:516-524` -- warns about localhost/private IPs | Similar but enforced at sandbox level |
| **Weaker network isolation** | **NONE** | `enableWeakerNetworkIsolation` -- allows com.apple.trustd.agent for TLS verification with MITM proxy (`sandboxTypes.ts:125-133`) |

**CRITICAL GAP**: Go has no network sandboxing whatsoever. All commands have unrestricted network access. Upstream's sandbox enforces domain-level network filtering at the OS level, with allow/deny lists and interactive permission prompts.

### 33.9 Environment Variable Sanitization

| Aspect | Go | Upstream TS |
|--------|-----|-------------|
| **Safe env var whitelist** | Yes -- `safeVarNames` at `exec_tool.go:574-612` -- 50+ safe vars, blocks PATH, PYTHONPATH, NODE_OPTIONS, etc. | Similar concept at `subprocessEnv.ts:4` for GitHub Actions |
| **Env prefix stripping** | Yes -- `stripSafeWrappers()` strips `VAR=val` prefix at `exec_tool.go:1344-1358` | Similar |
| **Variable expansion blocking** | Yes -- blocks `${VAR}` for unsafe vars at `detectCommandSubstitution()` `exec_tool.go:654-666` | Similar |
| **Process environment inheritance** | **Full inheritance** -- `os/exec.Command` inherits all parent env vars | Selective env passing in sandbox mode |
| **Subprocess env stripping** | **NONE** | `subprocessEnv.ts` strips GitHub Actions-specific vars in subprocess environments |

### 33.10 Permission Modes

| Aspect | Go | Upstream TS |
|--------|-----|-------------|
| **Ask mode** | `ModeAsk` at `config.go:20` -- prompts for exec/write/edit/fileops | `'ask'` mode -- interactive prompts for dangerous tools |
| **Auto mode** | `ModeAuto` at `config.go:21` -- LLM classifier evaluates tools | `'auto'` mode -- YOLO classifier with allow/deny rules |
| **Plan mode** | `ModePlan` at `config.go:22` -- blocks all write tools | `'plan'` mode -- read-only, blocks mutations |
| **Bypass mode** | `ModeBypass` at `config.go:23` -- allows all | `'bypass'` mode -- only available with `--dangerously-skip-permissions` |
| **Default mode** | **No explicit default** (empty string falls through) | `'default'` mode -- defers to user preference/settings |
| **CLI flag** | `--permission-mode` not implemented | `--permission-mode <mode>` at `bridgeMain.ts:1787-1790` |
| **Mode transition** | Basic -- `PrePlanMode` remembers pre-plan mode for restoration | Full `transitionPermissionMode()` with EnterPlanMode/ExitPlanMode hooks at `state.ts:1355-1387` |
| **Bypass availability check** | **NONE** -- bypass always available | `isBypassPermissionsModeAvailable()` -- only available in sandboxed environments at `setup.ts:420-430` |
| **Allow-dangerously-skip-permissions** | **NONE** | `--allow-dangerously-skip-permissions` flag to enable bypass as option at `main.tsx:1443` |
| **Auto mode classifier** | Go LLM-based at `permissions.go:411-496` -- 3-denial fallback, 20-denial session cap | YOLO classifier with stages, fast mode, environment-aware prompts |
| **Auto mode dangerous rule stripping** | Yes -- `StripDangerousAllowRules()` at `auto_strip.go:99-144` -- Bash/Exec only | More comprehensive -- includes PowerShell, Agent tools |
| **AllowedCommands config** | Yes -- `AllowedCommands` list at `config.go:35`, safe command prefix matching | No direct equivalent (uses sandbox for auto-allow) |
| **DeniedPatterns config** | Yes -- `DeniedPatterns` at `config.go:36`, string containment matching | No direct equivalent (uses sandbox deny rules) |
| **Sub-agent permission prompts** | `shouldAvoidPrompts()` at `permissions.go:398-400` -- skips prompts for sub-agents | Similar -- container runs don't prompt |

### 33.11 Summary: Critical Security Gaps

| # | Gap | Severity | Description |
|---|-----|----------|-------------|
| 1 | **No OS-level sandbox** | **CRITICAL** | Go runs all commands without macOS Seatbelt, Linux Bubblewrap, or any kernel-level isolation. Upstream enforces mandatory filesystem and network restrictions via sandbox-exec (macOS) and bwrap (Linux). |
| 2 | **No network restrictions** | **CRITICAL** | Go has no domain allow/deny lists, no Unix socket blocking, no MITM proxy support, no network permission prompts. All outbound traffic is unrestricted. |
| 3 | **No Docker/container support** | **HIGH** | Go cannot detect or run inside Docker containers. No `--dangerously-skip-permissions` gating based on sandbox + no-internet checks. |
| 4 | **No sandbox filesystem rules** | **HIGH** | Go permission rules (allow/deny/ask) are checked at the application level only. Upstream converts these into OS-enforced mount rules (ro-bind, tmpfs, deny-write). |
| 5 | **No symlink containment** | **HIGH** | Go resolves symlinks once but does not verify the resolved path stays within allowed directories. Upstream has multi-pass containment checks with dangling symlink detection (PSR M22186). |
| 6 | **No environment isolation** | **MEDIUM** | Go inherits full parent process environment. Upstream sandbox mode selectively passes env vars. |
| 7 | **Bypass mode always available** | **MEDIUM** | Go's bypass mode requires no special conditions. Upstream only allows bypass in verified sandboxed containers with no internet access. |
| 8 | **No sandbox settings system** | **MEDIUM** | Go has no `sandbox.enabled`, `failIfUnavailable`, `allowUnsandboxedCommands`, `excludedCommands`, or `autoAllowBashIfSandboxed` settings. |
| 9 | **No skill-scope permissions** | **LOW** | Go cannot offer narrow "allow edits to this skill only" permissions. Upstream has `getClaudeSkillScope()` for per-skill permission suggestions. |
| 10 | **Simpler binary detection** | **NONE (Go is better)** | Go's magic bytes detection is actually more comprehensive (15+ signatures) than upstream's null-byte approach. |

---

# Comprehensive Final Summary

*Generated: 2026-05-11. Synthesizes all 8,717 lines across 105 sections and 724 subsections of the upstream comparison document.*

---


---

## C. Top 10 Go Enhancements

*Features Go has that upstream lacks or does worse.*

| # | Enhancement | Go Location | Why It's Better |
|---|------------|-------------|-----------------|
| 1 | **15-category error taxonomy with action flags** | `error_types.go` | Upstream has 25+ string labels for analytics only; Go's `ClassifyResult` carries actionable flags (`Compress`, `RotateKey`, `Fallback`, `RetryAfter`) directly, enabling automated recovery decisions without separate lookup logic. |
| 2 | **13 LLM-callable file history tools** | `file_history_tools.go` | Upstream exposes file history only through the TUI; Go exposes snapshot/restore/diff/timeline/tag as LLM-callable tools, enabling the model to autonomously undo changes. |
| 3 | **Dedicated GitTool with 35+ structured operations** | `tools/git_tool.go` | Upstream uses raw `BashTool` for all git operations. Go's structured tool gives the model type-safe parameters, per-operation flag validation, dangerous operation detection, and `gh` CLI integration with read-only whitelist. |
| 4 | **Work task dependency graph with cycle detection** | `work_task.go` | Upstream's todo list is flat. Go models task dependencies as a DAG with cycle detection, validation, and blocking/unblocking — enabling the model to manage complex multi-step workflows. |
| 5 | **Grace call mechanism** | `agent_loop.go` | After token budget exhaustion, Go allows one extra text-only turn for the model to produce a conclusion, preventing abrupt session termination. Upstream has no equivalent. |
| 6 | **Preflight compression on session resume** | `agent_loop.go` | Auto-compacts sessions exceeding 100K tokens on resume, preventing immediate context overflow. Upstream relies on reactive compact after the overflow error arrives. |
| 7 | **Exa search, Process, Runtime info, Tool search tools** | `tools/` | Multiple Go-original tools with no upstream equivalent: `ExaSearchTool`, `ProcessTool`, `RuntimeInfoTool`, `ToolSearchTool`. These give the model capabilities upstream simply doesn't offer. |
| 8 | **LLM-autonomous skill discovery** | `skills/tracker.go` + `tools/` | Go provides `SkillSearch`, `SkillList`, `SkillRead` tools so the model can discover and load skills on demand. Upstream requires skills to be pre-loaded or conditionally loaded via code. |
| 9 | **MultiEdit tool** | `tools/multi_edit_tool.go` | Go provides a multi-edit tool that applies multiple edits in a single invocation. Upstream uses repeated single Edit calls, which is slower and less atomic. |
| 10 | **TodoWrite with dependencies** | `tools/todo_tool.go` | Upstream's todo is a flat list. Go's TodoWrite supports `addBlocks`/`addBlockedBy` dependency relationships, enabling proper task ordering and parallelization. |

---


---

## G. Go-Only Features Not in Upstream

*Complete list of Go-original features that could benefit the upstream TypeScript implementation.*

| # | Feature | Go Source | Description | Upstream Could Benefit? |
|---|---------|-----------|-------------|------------------------|
| 1 | **Grace call mechanism** | `agent_loop.go` | One extra text-only turn after budget exhaustion for model conclusion | **Yes** — prevents abrupt session termination |
| 2 | **Conclusion extraction** | `agent_loop.go` | Regex-based session state extraction from model output | **Yes** — structured session end |
| 3 | **Preflight compression on resume** | `agent_loop.go` | Auto-compact sessions >100K tokens on resume | **Yes** — proactive vs. reactive overflow handling |
| 4 | **Grace eviction ticker** | `agent_loop.go` | Background task cleanup for expired agent resources | **Yes** — resource management |
| 5 | **Agent management tools** (agent_list/get/kill) | `tools/agent_tool.go` | LLM-callable tools to manage running sub-agents | **Yes** — model can observe and control its own agents |
| 6 | **Plan mode tools** (EnterPlanMode/ExitPlanMode) | `tools/` | Plan mode as LLM-callable tools rather than UI-only | **Maybe** — upstream has plan mode in UI but not as tools |
| 7 | **Permission pre-check** | `permissions.go` | Sequential pre-check avoids concurrent stdin reads | **Maybe** — upstream uses React UI for permission prompts |
| 8 | **13 file history tools** | `file_history_tools.go` | Snapshot/restore/diff/timeline/tag/auto-snapshot/batch as LLM tools | **Yes** — upstream only has TUI-based history |
| 9 | **File history cross-file timeline** | `filehistory.go` | Timeline view across all tracked files | **Yes** — unique visualization capability |
| 10 | **FileOps tool** | `tools/fileops_tool.go` | Combined file operations (copy, move, mkdir, etc.) | **Yes** — upstream uses Bash for simple file ops |
| 11 | **ListDir tool** | `tools/listdir_tool.go` | Directory listing with depth control | **Yes** — simpler than using Bash `ls` or Glob |
| 12 | **Exa search tool** | `tools/exa_search_tool.go` | Web search via Exa API | **Maybe** — upstream uses different search providers |
| 13 | **Process tool** | `tools/process_tool.go` | Process listing and management | **Yes** — no upstream equivalent for process introspection |
| 14 | **Runtime info tool** | `tools/runtime_info_tool.go` | Runtime environment information | **Maybe** — useful for debugging |
| 15 | **Tool search tool** | `tools/tool_search_tool.go` | Search available tools by name/description | **Yes** — helps model discover tools in large registries |
| 16 | **MultiEdit tool** | `tools/multi_edit_tool.go` | Multiple edits in single invocation | **Yes** — more atomic than repeated single edits |
| 17 | **Inter-agent messaging** (SendMessageTool) | `tools/` | Explicit message passing between agents | **Yes** — upstream's Agent tool lacks explicit messaging |
| 18 | **TodoWrite with dependencies** | `tools/todo_tool.go` | `addBlocks`/`addBlockedBy` dependency relationships | **Yes** — upstream's todo is flat list |
| 19 | **15-category error taxonomy with action flags** | `error_types.go` | ErrorClass enum + `Compress`/`RotateKey`/`Fallback`/`RetryAfter` flags | **Maybe** — upstream has more categories but no action flags |
| 20 | **Billing vs rate-limit disambiguation** | `error_types.go` | 9 billing + 13 rate-limit patterns for 402 classification | **Yes** — upstream has less granular 402 handling |
| 21 | **100+ error substring patterns (incl. Chinese)** | `error_types.go` | 11 pattern groups with multilingual support | **Maybe** — upstream is English-only in error matching |
| 22 | **Work task dependency graph** | `work_task.go` | DAG with cycle detection and validation | **Yes** — upstream has no task dependency model |
| 23 | **LLM-autonomous skill discovery** | `skills/tracker.go` | SkillSearch/SkillList/SkillRead tools | **Yes** — upstream requires pre-loaded skills |
| 24 | **Fork mode with structured protocol** | `agent_sub.go` | Explicit fork boilerplate for sub-agents | **Maybe** — upstream uses different sub-agent model |
| 25 | **4-layer tool filtering** | `agent_sub.go` | Structured deny/allow system for sub-agent tools | **Maybe** — upstream uses permission rules |
| 26 | **Model alias resolution with parent-tier matching** | `agent_sub.go` | Resolves aliases to parent model tier | **Maybe** — upstream has alias system but different logic |
| 27 | **GitTool with 35+ structured operations** | `tools/git_tool.go` | Type-safe git operations vs. raw BashTool | **Yes** — safer and more model-friendly than raw shell |
| 28 | **Magic bytes binary detection (15+ signatures)** | `file_read.go` | Detects binary files by content, not just extension | **Yes** — upstream uses simpler null-byte approach |
| 29 | **AllowedCommands / DeniedPatterns config** | `config.go` | Explicit allow/deny lists for commands | **Maybe** — upstream uses sandbox rules instead |
| 30 | **Background flush loop for session memory** | `session_memory.go` | 30-second ticker + dirty flag for efficient disk writes | **Maybe** — upstream has different persistence model |

---


---

## 45. Remaining Go Modules Sweep — main.go, retry, normalize, streaming, forked_agent, transcript_builder

### Files Compared
- **Go**: `main.go`, `retry_utils.go`, `normalize.go`, `prompt_caching.go`, `rate_limit.go`, `hooks.go`, `agent_progress.go`, `streaming.go`, `forked_agent.go`, `work_task.go`, `transcript_builder.go`, `file_lock_unix.go`
- **Upstream**: `main.tsx`, `print.ts`, `claude.ts`, `errors.ts`, `messages.ts`, `forkedAgent.ts`, `hookEvents.ts`

### 45.1 Main Entry & CLI

| # | Aspect | Go (`main.go`) | Upstream (`main.tsx`, `print.ts`) | Type |
|---|--------|---------------|-----------------------------------|------|
| 1 | CLI entry + REPL | Manual flag parsing + interactive REPL + slash commands (line 19-731) | React/TSX interactive UI (~244K main.tsx + ~4000 line print.ts), full command system, plugins, SSH, swarm | 简化 |
| 2 | --resume from transcript | Manual from `.claude/transcripts/*.jsonl` (line 128-151) | `conversationRecovery.ts` — full recovery with TurnInterruptionState | 简化 |
| 3 | /agents management | Simplified list/show/stop subcommands (line 602-731) | Full swarm/teammate system with multiple backends | 简化 |
| 4 | Ctrl+C double-press | Atomic timestamp tracking 2s double Ctrl+C (line 192-217) | `useDoublePress.ts` — generic double-press detection hook | Go适配 |
| 5 | Permission mode switching | `/mode` command (ask/auto/bypass/plan) (line 380-394) | Full permission system with always-allow/deny/rules | 简化 |

### 45.2 Error Handling & Retry

| # | Aspect | Go (`error_types.go`, `retry_utils.go`) | Upstream (`errors.ts`, `claude.ts`) | Type |
|---|--------|----------------------------------------|-----------------------------------|------|
| 1 | Error classification | 15 ErrorClass with Compress/RotateKey/Fallback recovery strategies (line 13-481) | ~12 string tag categories, no boolean strategy fields | 简化 |
| 2 | Error pattern matching | Large regex/string pattern arrays (billing/rateLimit/contextOverflow/auth), including Chinese patterns (line 61-145) | Scattered in individual branches (429/400/401/403/404/413), uses instanceof + status code | 简化 |
| 3 | Retry backoff utility | `jitteredBackoff` with functional options (retry_utils.go:1-71) | No independent retry utility — backoff logic embedded in query loop | 简化 |

### 45.3 Normalize & Prompt Caching

| # | Aspect | Go (`normalize.go`, `prompt_caching.go`) | Upstream (`messages.ts`, `betas.ts`) | Type |
|---|--------|----------------------------------------|-----------------------------------|------|
| 1 | JSON key sorting | `sortMapKeys` recursive sorting + `normalizeWhitespace` blank line folding (normalize.go:18-217) | `normalizeMessages` handles message normalization but no JSON key sorting | Go增强 |
| 2 | Prompt caching module | `ApplyPromptCaching` (4 breakpoints), `FormatCachedSystemPrompt`, `FormatBoundaryCachedSystemPrompt` (global+org dual scope) (prompt_caching.go:11-218) | SDK sets `cache_control` directly, no independent prompt_caching module | 简化 |

### 45.4 Rate Limiting

| # | Aspect | Go (`rate_limit.go`) | Upstream (`rateLimitMocking.ts`, `claudeAiLimits.ts`) | Type |
|---|--------|---------------------|-----------------------------------------------------|------|
| 1 | Rate limit parsing | 4-bucket RateLimitBucket/State, ParseRateLimitHeaders, progress bar/ASCII bar (rate_limit.go:14-448) | Mainly handles unified rate limit headers (anthropic-ratelimit-unified-*) | Go适配 |
| 2 | Third-party provider headers | Parses Nous Portal / OpenRouter / OpenAI compatible x-ratelimit-* headers (line 156-169) | Parses anthropic-ratelimit-unified-* headers (Anthropic proprietary) | Go适配 |

### 45.5 Streaming & Forked Agent

| # | Aspect | Go (`streaming.go`, `forked_agent.go`) | Upstream (`query.ts`, `forkedAgent.ts`) | Type |
|---|--------|---------------------------------------|---------------------------------------|------|
| 1 | SSE stream adapter | StreamAdapter, CollectHandler, TerminalHandler, StreamBus, DeltasState, timeout/stall detection (streaming.go:1-1006) | Query generator directly yields Message, rendered by React UI | Go适配 |
| 2 | CollectHandler | Thread-safe collection, toolUseAsText detection, partial tool call cleanup (streaming.go:59-267) | No direct equivalent — messages managed via React state | Go适配 |
| 3 | Terminal rendering | TerminalHandler (ANSI dim filtering thinking, [Tool: name] display) (streaming.go:317-562) | React component rendering thinking/tool call | Go适配 |
| 4 | Stream stall detection | stallTimeout 300s/600s, dynamic based on token volume (streaming.go:753-823) | AbortController + timeout handling | 简化 |
| 5 | Forked agent context isolation | `createForkedClient` simple from env key/baseURL (forked_agent.go:306-360) | `createSubagentContext` full isolation (readFileState, abortController, getAppState, denialTracking) (forkedAgent.ts:345-466) | 简化 |
| 6 | Forked tool execution | `executeForkedTool` directly queries registry, 50K truncation (forked_agent.go:327-360) | Full tool pipeline via query() (permissions, coercion, execution) | 简化 |

### 45.6 Work Tasks & Transcript Builder

| # | Aspect | Go (`work_task.go`, `transcript_builder.go`) | Upstream | Type |
|---|--------|---------------------------------------------|----------|------|
| 1 | Work task store | `WorkTaskStore` with create/update/delete/dependency/cycle detection (work_task.go:1-276) | TodoWriteTool as Tool implementation (not standalone store), state in message context | 简化 |
| 2 | Dependency cycle detection | `wouldCreateCycle` BFS bidirectional traversal (work_task.go:241-264) | No cycle detection (TodoWriteTool has no dependency graph) | Go增强 |
| 3 | Compact transcript builder | `BuildCompactTranscript` constructs auto-mode classifier input (transcript_builder.go:11-176) | Classifier takes raw messages directly, no independent transcript builder | 简化 |
| 4 | AskUserQuestion approval detection | `isAskUserApproval` detects affirmative responses (transcript_builder.go:147-175) | No equivalent (UI handles directly) | Go增强 |

### 45.7 Test Coverage

| # | Aspect | Go | Upstream | Type |
|---|--------|----|----------|------|
| 1 | Test files | 25 `*_test.go` files in root directory | 192 `*.test.ts` files covering entire project | 简化 |

---


---

## 49. Go-Specific Enhancements & Adaptations

This section catalogs aspects where the Go miniClaudeCode implementation is **enhanced** or **adapted** compared to the upstream TypeScript claude-code — things Go does **better** or **differently** (not missing or simplified).

---

### 49.1 Enhanced Error Classification

| # | Aspect | Go (file:line) | Upstream (file:line) | Type |
|---|--------|----------------|----------------------|------|
| 1 | 15 ErrorClass taxonomy with typed enum | error_types.go:17-33 (`ECRetryable`…`ECUnknown`) | errors.ts:968-1164 — `classifyAPIError()` returns ~20 flat string tags, no enum | Go增强 |
| 2 | ClassifyResult with 3 boolean recovery strategies (Compress/RotateKey/Fallback) | error_types.go:36-45 | errors.ts — error strings only, no structured recovery hints | Go增强 |
| 3 | RetryAfter duration field on classify result | error_types.go:43 | errors.ts — no Retry-After parsing in classifyAPIError | Go增强 |
| 4 | 402 disambiguation: billing exhaustion vs transient usage limit | error_types.go:327-338 (`classify402`) | errors.ts — 402 handled as flat `credit_balance_low` string | Go增强 |
| 5 | 400 deep classification: context overflow, model-not-found, rate-limit, billing, or format error | error_types.go:341-373 (`classify400`) | errors.ts — 400 returns `invalid_request` or `tool_use_mismatch` only | Go增强 |
| 6 | Usage-limit vs rate-limit disambiguation with transient signal detection | error_types.go:269-280 (`usageLimitPatterns` + `usageLimitTransientSignals`) | errors.ts — no usage-limit/rate-limit disambiguation | Go增强 |
| 7 | Server disconnect + large session heuristic → context overflow detection | error_types.go:296-302 | errors.ts — no server-disconnect→context-overflow heuristic | Go增强 |
| 8 | 400 + large session → probable context overflow (4/10 threshold) | error_types.go:367-369 | errors.ts — no heuristic for 400+large-session | Go增强 |
| 9 | Chinese-language context overflow patterns (超过最大长度, 上下文长度) | error_types.go:98 | errors.ts — English-only patterns | Go适配 |
| 10 | Transport error type heuristics (readtimeout, connecttimeout, pooltimeout, etc.) | error_types.go:134-141 | errors.ts — only `APIConnectionTimeoutError` check | Go增强 |
| 11 | OpenRouter 403 "key limit exceeded" → billing reclassification | error_types.go:188-193 | errors.ts — 403 always `auth_error` | Go增强 |

---

### 49.2 Work Task Dependency Graphs

| # | Aspect | Go (file:line) | Upstream | Type |
|---|--------|----------------|----------|------|
| 1 | Bidirectional dependency edges (Blocks/BlockedBy) on WorkTask | work_task.go:32-33 | Upstream has no equivalent todo/task dependency graph | Go增强 |
| 2 | wouldCreateCycle BFS — prevents circular dependencies when adding BlockedBy edges | work_task.go:242-264 | No cycle detection in upstream todo system | Go增强 |
| 3 | filterValidDeps — silently removes references to non-existent tasks | work_task.go:267-275 | No equivalent in upstream | Go增强 |
| 4 | Automatic bidirectional edge maintenance (addBlocks → update blocked task's BlockedBy) | work_task.go:167-173 | No equivalent in upstream | Go增强 |
| 5 | Auto-cleanup: deleted task removes itself from all other tasks' Blocks/BlockedBy | work_task.go:209-214 | No equivalent in upstream | Go增强 |
| 6 | `#` prefix stripping on dependency IDs (tolerates `#1` format from LLM) | work_task.go:161-162 | No equivalent in upstream | Go适配 |

---

### 49.3 File History: Tag-Based Categorization & Extended Tool Suite

| # | Aspect | Go (file:line) | Upstream (file:line) | Type |
|---|--------|----------------|----------------------|------|
| 1 | Tag system: AddTag, ListTags, RemoveTag, SearchTag, SearchTagAll | file_history_tools.go:757-875; filehistory.go:562-823 | fileHistory.ts — no tag/naming system at all | Go增强 |
| 2 | Tag-based version specifiers in ResolveVersion (e.g., `from: "stable-tag"`) | filehistory.go:488,547-558 | fileHistory.ts — version lookup by messageId UUID only | Go增强 |
| 3 | file_history_grep tool: regex search across history versions | file_history_tools.go:209-325 | fileHistory.ts — no grep-in-history feature | Go增强 |
| 4 | file_history_search tool: find versions where text was added/removed/changed | file_history_tools.go:638-701 | fileHistory.ts — no search-by-change-type feature | Go增强 |
| 5 | file_history_timeline tool: chronological cross-file change timeline | file_history_tools.go:706-753 | fileHistory.ts — no cross-file timeline | Go增强 |
| 6 | file_history_annotate tool: add user annotations to specific versions | file_history_tools.go:878-921 | fileHistory.ts — no annotation feature | Go增强 |
| 7 | file_history_checkout tool with flexible version specifiers (v3, current, last2, tag) | file_history_tools.go:925-966 | fileHistory.ts — rewind by messageId only | Go增强 |
| 8 | file_history_batch tool: batch operations on glob-matched files | file_history_tools.go:970-1084 | fileHistory.ts — no batch operations | Go增强 |
| 9 | file_history_diff chain diff (from→to→to2) | file_history_tools.go:541-570 | fileHistory.ts — no chain diff | Go增强 |
| 10 | file_history_diff stat/name-only/unified modes | file_history_tools.go:497-539 | fileHistory.ts — diffLines for insertions/deletions only | Go增强 |
| 11 | Version merging: consecutive snapshots with same checksum auto-merge display | file_history_tools.go:59-70 | fileHistory.ts — no merge display | Go增强 |
| 12 | Unix-style path handling on Windows (`/e/` → `E:\`) | file_history_tools.go:1163-1166 | fileHistory.ts — Node path module handles this | Go适配 |

---

### 49.4 API Message Normalization: Recursive JSON Key Sorting

| # | Aspect | Go (file:line) | Upstream (file:line) | Type |
|---|--------|----------------|----------------------|------|
| 1 | Recursive JSON key sorting for KV cache reuse (sortMapKeys → sortValueKeys) | normalize.go:139-171 | messages.ts — `normalizeMessagesForAPI` handles tool_result pairing, not key sorting | Go增强 |
| 2 | sortValueKeys: recursively sorts keys in nested maps and arrays | normalize.go:158-171 | messages.ts — no equivalent recursive key normalization | Go增强 |
| 3 | NormalizeJSONBytes: standalone byte-slice key sorting for raw JSON | normalize.go:205-216 | messages.ts — no raw JSON normalization utility | Go增强 |
| 4 | Whitespace normalization: collapse 3+ blank lines to 1, trim trailing whitespace/lines | normalize.go:175-201 | messages.ts — no whitespace normalization for cache reuse | Go增强 |
| 5 | Explicit KV cache reuse documentation (Hermes-style prefix caching) | normalize.go:1-8 | messages.ts — no cache-reuse documentation | Go增强 |

---

### 49.5 Rate Limit Parsing: 3rd-Party Provider Headers

| # | Aspect | Go (file:line) | Upstream (file:line) | Type |
|---|--------|----------------|----------------------|------|
| 1 | Multi-provider header parsing: Nous Portal / OpenRouter / OpenAI `x-ratelimit-*` format | rate_limit.go:156-229 | rateLimitMocking.ts — only Anthropic `anthropic-ratelimit-*` headers | Go增强 |
| 2 | 4-bucket rate limit state: RPM, RPH, TPM, TPH (requests/tokens × min/hour) | rate_limit.go:49-58 | rateLimitMocking.ts — single unified status/overage model | Go增强 |
| 3 | MostConstrainedBucket: identifies highest-usage bucket likely to cause next 429 | rate_limit.go:75-101 | rateLimitMocking.ts — no constraint analysis | Go增强 |
| 4 | RetryDelay: computes safe wait time based on exhausted buckets + 10% safety margin | rate_limit.go:105-129 | rateLimitMocking.ts — no computed retry delay | Go增强 |
| 5 | Thread-safe RateLimitState with sync.RWMutex | rate_limit.go:50 | rateLimitMocking.ts — no concurrency control (single-threaded JS) | Go适配 |
| 6 | Age-aware freshness tracking (just now / Ns ago / Nm ago) | rate_limit.go:61-71,363-371 | rateLimitMocking.ts — no age tracking | Go增强 |
| 7 | ASCII progress bar visualization for terminal display | rate_limit.go:325-335 | rateLimitMocking.ts — no progress bar | Go增强 |
| 8 | Compact one-line status bar format | rate_limit.go:420-447 | rateLimitMocking.ts — no compact display | Go增强 |
| 9 | 80% warning threshold on any bucket | rate_limit.go:394-409 | rateLimitMocking.ts — no warning thresholds | Go增强 |
| 10 | parseRateLimitHeadersFromMap: non-HTTP header parsing context | rate_limit.go:232-269 | rateLimitMocking.ts — only Headers object | Go增强 |
| 11 | Retry-After header fallback when no x-ratelimit-* headers present | rate_limit.go:194-207 | rateLimitMocking.ts — no Retry-After parsing | Go增强 |

---

### 49.6 Streaming Architecture: Standalone StreamAdapter / CollectHandler / TerminalHandler

| # | Aspect | Go (file:line) | Upstream (file:line) | Type |
|---|--------|----------------|----------------------|------|
| 1 | StreamAdapter: standalone SSE-to-StreamChunk bridge, decoupled from query loop | streaming.go:707-1005 | query.ts — streaming embedded in query() generator, no standalone adapter | Go增强 |
| 2 | CollectHandler: thread-safe chunk accumulator with mutex protection | streaming.go:59-298 | query.ts — mutable arrays, no concurrency concern (single-threaded) | Go适配 |
| 3 | TerminalHandler: clean terminal display with thinking filter state machine | streaming.go:317-562 | query.ts — no equivalent standalone terminal handler | Go增强 |
| 4 | ThinkFilterState: 4-state machine for `<thinking>...</thinking>` tag filtering | streaming.go:301-397 | query.ts — no think-tag terminal filter | Go增强 |
| 5 | StreamBus: pub/sub for StreamChunk events with buffered channels | streaming.go:568-621 | query.ts — no equivalent pub/sub | Go增强 |
| 6 | StreamProgress: TTFB + throughput tracking with mutex | streaming.go:625-681 | query.ts — queryProfiler.ts tracks latency, not TTFB/throughput per stream | Go增强 |
| 7 | DeltasState: tracks what was streamed (None/TextOnly/ToolInFlight) for retry safety | streaming.go:688-1005 | query.ts — no explicit deltas-state tracking | Go增强 |
| 8 | Tool-use-as-text detection (model echoing tool syntax as plain text) | streaming.go:137-146 | query.ts — no equivalent detection | Go增强 |
| 9 | ClearPartialToolCall: removes incomplete tool call before retry | streaming.go:226-232 | query.ts — streamingToolExecutor.discard() handles this differently | Go适配 |
| 10 | HasTruncatedToolArgs: validates JSON args for truncation detection | streaming.go:255-267 | query.ts — no JSON validation for truncation | Go增强 |
| 11 | AsParsedResponse: bypasses SDK ContentBlockUnion for non-Claude models | streaming.go:271-298 | query.ts — SDK-specific, no bypass needed | Go适配 |
| 12 | Dynamic stall timeout based on local provider / context size | streaming.go:733-748 | query.ts — no stall detection at stream level | Go增强 |
| 13 | Stall detector with goroutine + timer reset pattern | streaming.go:786-823 | query.ts — no stall detection | Go增强 |
| 14 | FinishReason tracking (end_turn, max_tokens, tool_use) on CollectHandler | streaming.go:86-92,198-210 | query.ts — tracked in MessageDeltaEvent but not on collector | Go增强 |
| 15 | toolArgSummary: per-tool compact terminal summary for 15+ tool types | streaming.go:482-562 | query.ts — no equivalent terminal summary | Go增强 |

---

### 49.7 Retry Utilities: Jittered Exponential Backoff

| # | Aspect | Go (file:line) | Upstream | Type |
|---|--------|----------------|----------|------|
| 1 | jitteredBackoff: standalone utility with functional options pattern | retry_utils.go:21-46 | Upstream uses `withRetry` wrapper but no standalone jittered backoff | Go增强 |
| 2 | Thundering-herd prevention: random jitter prevents synchronized retry spikes | retry_utils.go:12-13 | Upstream withRetry has no jitter | Go增强 |
| 3 | Configurable base/max/ratio via functional options (WithJitterBase, WithJitterMax, WithJitterRatio) | retry_utils.go:57-69 | Upstream withRetry has hardcoded retry counts | Go增强 |
| 4 | 63-bit exponent overflow guard | retry_utils.go:36-38 | No overflow guard in upstream | Go增强 |
| 5 | Formula: `delay = min(base × 2^(attempt-1), maxDelay) + uniform(0, jitterRatio × delay)` | retry_utils.go:20 | Upstream: simple retry count with no delay formula | Go增强 |

---

### 49.8 Ctrl+C Double-Press with Atomic Timestamp

| # | Aspect | Go (file:line) | Upstream | Type |
|---|--------|----------------|----------|------|
| 1 | atomic.Int64 timestamp for lock-free Ctrl+C double-press detection | main.go:192 (`lastCtrlC atomic.Int64`) | Upstream uses AbortController + UI-level "press esc twice" | Go适配 |
| 2 | 2-second double-press window → clean exit (Close + os.Exit) | main.go:204-208 | Upstream: double-esc via UI event handling | Go适配 |
| 3 | Windows EOF race handling: Ctrl+C closes stdin simultaneously with SIGINT | main.go:291-339 | Upstream: not applicable (Node.js stdin doesn't close on SIGINT) | Go适配 |
| 4 | 200ms grace period for ctrlCh/lastCtrlC check before treating as true EOF | main.go:327-336 | Upstream: no equivalent (different stdin semantics) | Go适配 |
| 5 | Goroutine-based stdin reader to avoid blocking on Ctrl+C | main.go:246-258 | Upstream: readline/ink handles this internally | Go适配 |
| 6 | Stale Ctrl+C drain after real input to prevent phantom interrupts | main.go:342-346 | Upstream: handled by input library | Go适配 |

---

### 49.9 Transcript Builder: isAskUserApproval Detection

| # | Aspect | Go (file:line) | Upstream (file:line) | Type |
|---|--------|----------------|----------------------|------|
| 1 | isAskUserApproval: detects explicit user consent in AskUserQuestion results | transcript_builder.go:150-175 | transcriptSearch.ts — no equivalent approval detection | Go增强 |
| 2 | 16 affirmative keyword matching (yes, ok, sure, continue, allow, proceed, etc.) | transcript_builder.go:163-168 | transcriptSearch.ts — no consent keyword list | Go增强 |
| 3 | Q:/A: format parsing for structured question-answer extraction | transcript_builder.go:152-161 | transcriptSearch.ts — no Q/A parsing | Go增强 |
| 4 | "USER EXPLICITLY APPROVED" tag in compact transcript for classifier | transcript_builder.go:59 | transcriptSearch.ts — no approval tagging | Go增强 |
| 5 | Security requirement: skip assistant text to prevent agent from influencing classifier | transcript_builder.go:41-42 | transcriptSearch.ts — no equivalent security constraint documented | Go增强 |

---

### 49.10 Windows-Specific Adaptations

| # | Aspect | Go (file:line) | Upstream | Type |
|---|--------|----------------|----------|------|
| 1 | Unix-style path `/e/` → Windows `E:\` conversion in expandPath | file_history_tools.go:1163-1166 | Node's path module handles platform differences automatically | Go适配 |
| 2 | Windows Ctrl+C stdin close handling with goroutine reader restart | main.go:244-274 | Upstream: not needed (Node.js different stdin semantics) | Go适配 |
| 3 | PowerShell detection in system prompt | system_prompt.go:29 | Upstream: similar but via runtime detection | Go适配 |

---

### 49.11 Concurrency Adaptations

| # | Aspect | Go (file:line) | Upstream | Type |
|---|--------|----------------|----------|------|
| 1 | sync.RWMutex on WorkTaskStore | work_task.go:41 | Upstream: React state (single-threaded) | Go适配 |
| 2 | sync.RWMutex on RateLimitState | rate_limit.go:50 | Upstream: single-threaded JS | Go适配 |
| 3 | sync.Mutex on CollectHandler | streaming.go:61 | Upstream: single-threaded JS | Go适配 |
| 4 | atomic.Bool for interrupted flag | agent_loop.go:293 | Upstream: AbortController.signal | Go适配 |
| 5 | atomic.Int32 for IterationBudget consumed counter | agent_loop.go:34 | Upstream: simple counter in closure | Go适配 |
| 6 | atomic.Bool for IterationBudget graceCalled | agent_loop.go:35 | Upstream: no equivalent | Go适配 |
| 7 | CompareAndSwap for budget consume/refund | agent_loop.go:44-67 | Upstream: simple increment | Go适配 |
| 8 | sync.Mutex on StreamProgress | streaming.go:637 | Upstream: single-threaded | Go适配 |

---

### 49.12 Summary Statistics

| Category | Count | Go增强 | Go适配 |
|----------|-------|--------|--------|
| 49.1 Error Classification | 11 | 10 | 1 |
| 49.2 Work Task Dependencies | 6 | 5 | 1 |
| 49.3 File History Tools | 12 | 11 | 1 |
| 49.4 Message Normalization | 5 | 5 | 0 |
| 49.5 Rate Limit Parsing | 11 | 10 | 1 |
| 49.6 Streaming Architecture | 15 | 11 | 4 |
| 49.7 Retry Utilities | 5 | 5 | 0 |
| 49.8 Ctrl+C Handling | 6 | 0 | 6 |
| 49.9 Transcript Builder | 5 | 5 | 0 |
| 49.10 Windows Adaptations | 3 | 0 | 3 |
| 49.11 Concurrency | 8 | 0 | 8 |
| **Total** | **87** | **52** | **25** |

**Key insight**: Go enhancements (52 items) are concentrated in four areas:
1. **Error classification** — structured taxonomy with recovery strategies vs. flat string tags
2. **File history** — tag-based categorization, grep/search/timeline/batch tools vs. simple backup-restore
3. **Rate limits** — multi-provider header parsing with 4-bucket state vs. Anthropic-only mock
4. **Streaming** — standalone adapter/handler/bus architecture vs. embedded query-loop logic

Go adaptations (25 items) fall into two categories:
1. **Concurrency primitives** (mutexes, atomics) — necessary because Go is multi-threaded
2. **Platform handling** (Windows paths, Ctrl+C/stdin) — Go's lower-level I/O requires explicit handling


---

