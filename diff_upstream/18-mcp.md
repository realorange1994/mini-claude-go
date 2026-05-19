# MCP Implementation

> Model Context Protocol

## Sections Included
- [##] Line 405-586 -- ## 5. MCP (Model Context Protocol) Implementation
- [##] Line 587-619 -- ## MCP Comparison Summary of Key Gaps

---

## Content

## 5. MCP (Model Context Protocol) Implementation

### Files Compared
- **Go**: `E:\Git\miniClaudeCode-go-github\mcp\client.go` (859 lines), `E:\Git\miniClaudeCode-go-github\tools\mcp_tools.go` (371 lines), `E:\Git\miniClaudeCode-go-github\config.go` (404 lines)
- **Upstream package**: `E:\Git\claude-code-upstream\packages\mcp-client\src\` (manager.ts, connection.ts, execution.ts, discovery.ts, types.ts, interfaces.ts, strings.ts, errors.ts, sanitization.ts, cache.ts)
- **Upstream service**: `E:\Git\claude-code-upstream\src\services\mcp\client.ts` (~3300+ lines), `E:\Git\claude-code-upstream\src\services\mcp\config.ts` (~1585 lines), `E:\Git\claude-code-upstream\src\services\mcp\useManageMCPConnections.ts`

### 5.1 Transport Layer

| Aspect | Go | Upstream TS |
|--------|-----|-------------|
| **stdio** | Yes -- `exec.Cmd` with `StdinPipe`/`StdoutPipe`/`StderrPipe` (client.go:134-155) | Yes -- `StdioClientTransport` from `@modelcontextprotocol/sdk` (client.ts:951-958) |
| **SSE** | Basic -- raw HTTP POST + SSE response parsing (client.go:525-617) | Full -- `SSEClientTransport` with auth provider, proxy, eventSourceInit (client.ts:619-707) |
| **HTTP (Streamable)** | No -- not implemented | Yes -- `StreamableHTTPClientTransport` with auth, proxy, TLS (client.ts:784-864) |
| **WebSocket** | No -- not implemented | Yes -- custom `WebSocketTransport` with proxy/TLS support (client.ts:735-783) |
| **SSE-IDE** | No -- not implemented | Yes -- `SSEClientTransport` with IDE-specific config (client.ts:678-707) |
| **WS-IDE** | No -- not implemented | Yes -- `WebSocketTransport` with IDE auth token (client.ts:708-734) |
| **In-process** | No -- not implemented | Yes -- `createLinkedTransportPair` for Chrome/Computer Use (client.ts:910-943) |
| **SDK control** | No -- not implemented | Yes -- `SdkControlClientTransport` for agent SDK (client.ts:866-881) |
| **claude.ai proxy** | No -- not implemented | Yes -- `StreamableHTTPClientTransport` with OAuth, session header (client.ts:868-904) |

**Finding**: Go only supports stdio and a basic HTTP+SSE transport. The SSE implementation is hand-rolled (custom `readSSE()` function) rather than using the official MCP SDK transport. Upstream supports 8 transport types including WebSocket, in-process, IDE transports, and the claude.ai proxy.

**Gap**: 6 transport types are missing from Go: HTTP Streamable, WebSocket, SSE-IDE, WS-IDE, in-process, SDK control, and claude.ai proxy. The hand-rolled SSE parser may also have edge-case incompatibilities with the SDK's production-tested implementation.

### 5.2 Connection Lifecycle (connect, reconnect, disconnect)

| Aspect | Go | Upstream TS |
|--------|-----|-------------|
| **Connect** | `Start()` -> `startStdio()` / `startRemote()` -> initialize + listTools (client.go:127-199) | `connectToServer()` -> create transport -> create Client -> `client.connect(transport)` with timeout (client.ts:595-1199) |
| **Reconnect** | **No** -- no reconnection logic; failed connections require manual restart | Full exponential backoff: `MAX_RECONNECT_ATTEMPTS=5`, `INITIAL_BACKOFF_MS=1000`, `MAX_BACKOFF_MS=30000` (useManageMCPConnections.ts:88-91) |
| **Disconnect** | `Stop()` -> close stdin -> `cmd.Wait()` with 5s timeout -> `Process.Kill()` (client.go:621-647) | `createCleanup()` -> in-process close / signal escalation (`SIGINT->SIGTERM->SIGKILL`) / `client.close()` (connection.ts:431-474) |
| **Connection monitor** | **No** | `installConnectionMonitor()` -- enhanced error/close handlers with terminal error detection, session expiry detection, consecutive error tracking (connection.ts:196-286) |
| **Connection timeout** | **No** -- uses context from caller | `withConnectionTimeout()` -- default 30s, configurable via `MCP_TIMEOUT` env (connection.ts:84-109, client.ts:456-458) |
| **Session expiry** | **No** -- no concept of session management | Detects 404 + JSON-RPC -32001 to auto-clear cache and reconnect (connection.ts:168-178) |
| **Consecutive error tracking** | **No** | Tracks up to `MAX_ERRORS_BEFORE_RECONNECT=3` terminal errors before forcing close (connection.ts:22-23, client.ts:1228-1366) |
| **Client capabilities** | Empty: `"capabilities": {}` (client.go:203) | Declares `roots: {}` and `elicitation: {}` (connection.ts:57-62, client.ts:986-1002) |
| **Client info** | `{"name": "miniclaudecode", "version": "0.1.0"}` (client.go:204) | Full metadata: name, title, version, description, websiteUrl (client.ts:986-993) |
| **ListRoots handler** | **No** | Yes -- returns `file://${process.cwd()}` (connection.ts:65-73, client.ts:1010-1018) |
| **Batch connect** | Sequential in `StartAll()` (client.go:690-710) | Concurrent with `MCP_SERVER_CONNECTION_BATCH_SIZE=3` and `MCP_REMOTE_SERVER_CONNECTION_BATCH_SIZE=20` (client.ts:552-561) |

**Finding**: Go has a minimal connect/disconnect lifecycle with no reconnection, session management, or health monitoring. Upstream has a sophisticated lifecycle with exponential backoff reconnection, connection monitors that detect terminal errors, session expiry handling, and batched connection with configurable concurrency.

### 5.3 Tool Discovery and Registration

| Aspect | Go | Upstream TS |
|--------|-----|-------------|
| **Tool listing** | `listToolsStdio()` / `listToolsRemote()` -> JSON-RPC `tools/list` (client.go:258-286) | `discoverTools()` via `client.request({method: 'tools/list'}, ListToolsResultSchema)` (discovery.ts:47-107) |
| **Tool storage** | `[]Tool` on Client struct -- flat list with Name, Description, InputSchema (client.go:50-54) | `CoreTool` objects with full metadata -- mcpInfo, isMcp, inputJSONSchema, permission check, annotations, userFacingName, etc. (discovery.ts:63-101) |
| **Tool name format** | Raw tool names from server (no prefixing) (client.go:258-286) | `mcp__${serverName}__${toolName}` via `buildMcpToolName()` (strings.ts:30-31) |
| **Tool annotations** | **No** -- `Tool` struct has only Name, Description, InputSchema (client.go:50-54) | Full annotation support: readOnlyHint, destructiveHint, openWorldHint, title (discovery.ts:80-85) |
| **Capability check** | **No** -- always attempts `tools/list` | Checks `capabilities?.tools` before fetching (discovery.ts:50-52) |
| **Unicode sanitization** | **No** | `recursivelySanitizeUnicode()` -- strips control chars, normalizes NFC (sanitization.ts:8-31) |
| **Description truncation** | **No** | Truncates at `MAX_MCP_DESCRIPTION_LENGTH=2048` (discovery.ts:77-79) |
| **Discovery caching** | **No** | `createCachedToolDiscovery()` with LRU of size 20 (discovery.ts:111-143) |
| **Manager-level tools** | `ListTools()` -- concatenation of all client.tools (client.go:770-779) | `getTools(serverName)` / `getAllTools()` with per-server caching (manager.ts:139-149) |
| **Tools with server info** | `AllToolsWithServer()` returns `ToolWithServer` (client.go:809-823) | `CoreTool` objects carry `mcpInfo: {serverName, toolName}` (discovery.ts:69) |
| **MCP tool commands** | **No** -- not implemented | `fetchCommandsForClient()` -- JSON-RPC `commands/list` for slash command discovery (client.ts:1700+) |
| **MCP resources** | **No** -- not implemented | `fetchResourcesForClient()` + `ListMcpResourcesTool` + `ReadMcpResourceTool` (client.ts:1690+) |
| **MCP prompts** | **No** -- not implemented | `fetchPromptsForClient()` with change notification (client.ts:1700+) |

**Finding**: Go's tool discovery is a bare-minimum implementation. Tools are stored as flat structs with no prefixing, annotations, or capability checks. Upstream uses fully qualified `mcp__server__tool` names, extracts tool annotations for permission decisions, sanitizes Unicode, truncates descriptions, and caches discovery results. Upstream also discovers commands, resources, and prompts from MCP servers, none of which Go supports.

### 5.4 Tool Call Execution (JSON-RPC request/response)

| Aspect | Go | Upstream TS |
|--------|-----|-------------|
| **Primary method** | `CallTool()` -> `callToolStdio()` / `callToolRemote()` -> `requestStdio()` / `requestRemote()` (client.go:289-326) | `callMcpTool()` from package -> `client.callTool()` from SDK (execution.ts:57-151) |
| **Request format** | Manual JSON-RPC: `{"name": name, "arguments": args}` (client.go:290-293) | SDK-typed: `{name, arguments, _meta}` with `CallToolResultSchema` validation (execution.ts:78-91) |
| **Timeout** | Two-level: `CallToolWithTimeout()` separates timeout from user interrupt (client.go:384-513) | `Promise.race` with `createTimeoutPromise()`, default ~27.8h, configurable via `MCP_TOOL_TIMEOUT` (execution.ts:19-21, client.ts:3072-3124) |
| **Background on timeout** | Yes -- `resultCh chan<- RPCResponse` receives result after timeout (client.go:384-513) | **No** -- timeout rejects the promise; no background continuation (execution.ts:168-182) |
| **Progress callback** | **No** | Yes -- `onProgress` with SDK's `onprogress` handler (execution.ts:37-38, client.ts:3105-3116) |
| **Progress logging** | **No** | Every 30s interval logs elapsed time (client.ts:3056-3068) |
| **Schema validation** | **No** -- raw `json.Unmarshal` into `ToolResult` (client.go:308-313) | Yes -- `CallToolResultSchema` validates response structure (execution.ts:85-91) |
| **isError handling** | Basic -- passes through `result.IsError` (mcp_tools.go:208-209) | Extracts error text from first content block, throws `McpToolCallError` (execution.ts:95-114) |
| **Elicitation retry** | **No** | `callMCPToolWithUrlElicitationRetry()` -- retries URL elicitations (client.ts:2815-2850) |
| **Session expiry retry** | **No** | Detects 404/-32001, clears connection cache, reconnects (client.ts:3219-3250) |
| **401 auth retry** | **No** | Detects 401/UnauthorizedError, throws `McpAuthError`, updates state to `needs-auth` (client.ts:3198-3211) |
| **Structured content** | **No** -- only text content blocks parsed (mcp_tools.go:209-213) | Yes -- `structuredContent` field preserved (execution.ts:119-122) |
| **`_meta` passthrough** | **No** | Yes -- `result._meta` forwarded to caller (execution.ts:118-119) |
| **Content processing** | **No** -- only `Type: "text"` blocks extracted (mcp_tools.go:209-213) | Full processing via `processMCPResult()` -- image downscaling, binary persistence, truncation (client.ts:3173+) |

**Finding**: Go has a unique advantage in its `CallToolWithTimeout()` method that preserves the MCP connection on timeout and continues the call in the background, sending the result via a channel. This is a deliberate design choice to avoid breaking stdio connections on timeout (noted in comments at client.go:380-383). However, Go lacks progress callbacks, schema validation, elicitations, session-expiry retry, auth retry, and content processing beyond text blocks.

### 5.5 Error Handling and Timeouts

| Aspect | Go | Upstream TS |
|--------|-----|-------------|
| **Error types** | Simple `fmt.Errorf` with string messages (client.go:344, 374, etc.) | Typed error hierarchy: `McpError`, `McpConnectionError`, `McpAuthError`, `McpTimeoutError`, `McpToolCallError`, `McpSessionExpiredError` (errors.ts:1-81) |
| **Connection timeout** | Uses caller's context -- no dedicated timeout | `withConnectionTimeout()` -- default 30s, configurable via `MCP_TIMEOUT` env (connection.ts:84-109) |
| **Tool call timeout** | User-configurable per-call: default 30s, max 600s, min 1s (mcp_tools.go:164-179) | Default ~27.8h (`100_000_000` ms), configurable via `MCP_TOOL_TIMEOUT` env (client.ts:211-229) |
| **Stderr capture** | Simple goroutine forwarding to os.Stderr (client.go:157-168) | `captureStderr()` -- accumulates up to 64MB with cleanup handler (connection.ts:113-142) |
| **Terminal error detection** | **No** | `isTerminalConnectionError()` -- matches ECONNRESET, ETIMEDOUT, EPIPE, EHOSTUNREACH, ECONNREFUSED, etc. (connection.ts:151-163) |
| **Session expiry detection** | **No** | `isMcpSessionExpiredError()` -- checks HTTP 404 + JSON-RPC -32001 (connection.ts:168-178) |
| **Auth error detection** | **No** | Checks `code === 401` and `UnauthorizedError` instance (client.ts:3198-3211) |
| **Error/close handler chaining** | **No** | Wraps original handlers, chains to them after custom logic (connection.ts:196-286) |
| **Per-request fetch timeout** | **No** | `wrapFetchWithTimeout()` -- 60s timeout per HTTP request, with Accept header normalization (client.ts:492-549) |

**Finding**: Go's error handling is minimal -- all errors are string-based with no type hierarchy. Upstream has a complete typed error hierarchy, per-connection timeouts, per-request fetch timeouts, and sophisticated error pattern detection for network issues, session expiry, and authentication failures.

### 5.6 Environment Variable Injection

| Aspect | Go | Upstream TS |
|--------|-----|-------------|
| **Config-level env** | `map[string]string` on `NewClient()` -> appended to `cmd.Env` (client.go:101-108) | `env` field on config -> merged with `subprocessEnv()` in StdioClientTransport (client.ts:951-958) |
| **Env expansion** | **No** -- `${VAR}` syntax not expanded | `expandEnvVarsInString()` -- supports `${VAR}` and `${VAR:-default}` (envExpansion.ts:1-38) |
| **Missing env tracking** | **No** | Reports `missingVars` in validation errors (config.ts:556-615) |
| **Windows npx detection** | **No** | Warns if `npx` used without `cmd /c` wrapper on Windows (config.ts:1351-1369) |
| **Subprocess env provider** | Direct `os.Environ()` (client.go:103) | `subprocessEnv()` utility with filtering and injection (client.ts:97) |
| **CLAUDE_CODE_SHELL_PREFIX** | **No** | Yes -- wraps command with shell prefix if set (client.ts:947-950) |

**Finding**: Go simply appends config env vars to the process environment. Upstream has a full env expansion system supporting `${VAR}` and `${VAR:-default}` syntax, Windows npx detection, missing variable tracking, and a configurable shell prefix.

### 5.7 Server Health Checking

| Aspect | Go | Upstream TS |
|--------|-----|-------------|
| **Health check mechanism** | **No** -- only `running` boolean on Client (client.go:86) | `installConnectionMonitor()` -- wraps `client.onerror` and `client.onclose` with enhanced handlers (connection.ts:196-286) |
| **Status reporting** | `GetServerStatus()` returns "connected"/"disconnected"/"not registered" (client.go:794-806) | `MCPServerConnection` tagged union: `connected`/`failed`/`needs-auth`/`pending`/`disabled` (types.ts:160-206) |
| **Connection drop detection** | **No** | Terminal error tracking with counter, auto-close on `MAX_ERRORS_BEFORE_RECONNECT=3` (connection.ts:260-265) |
| **Reconnection trigger** | **No** -- requires full `StopAll()` + `StartAll()` | Auto-reconnect via `onclose` -> clear cache -> next call auto-reconnects (useManageMCPConnections.ts) |
| **Uptime tracking** | **No** | Yes -- logs uptime in seconds on error/close (connection.ts:222-224, 271-273) |
| **Auth requirement detection** | **No** | `needs-auth` state, `McpAuthError`, 15-min cache for auth failures (client.ts:257-316) |
| **Server approval** | **No** | Project server approval flow (`getProjectMcpServerStatus`) before connecting (config.ts:1165-1168) |

**Finding**: Go has no health checking beyond a simple `running` boolean. Upstream has a multi-state connection model with 5 states, auto-reconnection, connection monitoring, auth requirement detection, and project server approval.

### 5.8 Remote MCP Server Support

| Aspect | Go | Upstream TS |
|--------|-----|-------------|
| **HTTP+SSE** | Basic -- hand-rolled SSE parser (client.go:525-617) | Full -- `SSEClientTransport` with auth provider, proxy, eventSourceInit (client.ts:619-707) |
| **HTTP Streamable** | **No** | Full -- `StreamableHTTPClientTransport` with auth, proxy, TLS (client.ts:784-864) |
| **WebSocket** | **No** | Full -- `WebSocketTransport` with proxy, TLS, IDE auth (client.ts:708-783) |
| **OAuth 2.0** | **No** | `ClaudeAuthProvider`with OAuth discovery, token refresh, step-up detection (auth.ts, client.ts:620-627) |
| **Headers helper** | **No** | `headersHelper` script execution for dynamic header injection (headersHelper.ts) |
| **Proxy support** | **No** | `getProxyFetchOptions()`, `getWebSocketProxyAgent()`, `getWebSocketProxyUrl()` (client.ts) |
| **TLS/MTLS** | **No** | `getWebSocketTLSOptions()`, `getTLSOptions()` with cert/key paths (client.ts) |
| **Session ingress** | **No** | `getSessionIngressAuthToken()` for session-routed connections (client.ts:617) |
| **CCR proxy unwrapping** | **No** | `unwrapCcrProxyUrl()` for deduplicating proxy-routed connectors (config.ts:170-193) |
| **Claude.ai connector fetch** | **No** | `fetchClaudeAIMcpConfigsIfEligible()` for web-configured servers (claudeai.ts) |

**Finding**: Go's remote server support is limited to a basic HTTP+SSE implementation. It lacks all authentication mechanisms (OAuth, session ingress, headers helper), proxy support, TLS configuration, and the full HTTP Streamable and WebSocket transports.

### 5.9 MCP Tool Result Parsing and Formatting

| Aspect | Go | Upstream TS |
|--------|-----|-------------|
| **Content block types** | Only `text` blocks extracted (mcp_tools.go:209-213) | Text, image, resource links; images downscaled and persisted (client.ts:3173+) |
| **Binary content** | **No** | `persistBinaryContent()` -- saves to disk, returns file path (client.ts:3173+) |
| **Image processing** | **No** | `maybeResizeAndDownsampleImageBuffer()` -- reduces image size for API (client.ts:3173+) |
| **Output truncation** | **No** | `truncateMcpContentIfNeeded()` -- caps at 100K chars per result (client.ts:3173+) |
| **Content size estimation** | **No** | `getContentSizeEstimate()` for proactive truncation decisions (client.ts:3173+) |
| **Large output instructions** | **No** | `getLargeOutputInstructions()` appended to truncated results (client.ts:3173+) |
| **Structured content** | **No** | `structuredContent` field preserved and forwarded (execution.ts:119-122) |
| **Description truncation** | **No** -- raw descriptions passed through | Truncates at `MAX_MCP_DESCRIPTION_LENGTH=2048` (connection.ts:497-507, discovery.ts:77-79) |
| **Instructions truncation** | **No** -- raw instructions passed through | Truncates at `MAX_MCP_DESCRIPTION_LENGTH=2048` (connection.ts:497-507) |
| **MCP skills** | **No** -- not implemented | `fetchMcpSkillsForClient()` -- discovers skills from MCP servers (client.ts:129-133) |
| **Tool result storage** | **No** | `persistToolResult()` + `isPersistError()` for large result caching (client.ts) |
| **Tool collapse classification** | **No** | `classifyMcpToolForCollapse()` -- determines which tool results can be collapsed in UI (client.ts:138) |

**Finding**: Go only extracts text content from MCP results. Upstream handles images (with downscaling), binary content (with persistence), resource links, structured content, and implements output truncation with large-output instructions for the model.

### 5.10 Server Capability Detection

| Aspect | Go | Upstream TS |
|--------|-----|-------------|
| **Capabilities parsed** | **No** -- `initializeStdio()` sends `"capabilities": {}` and ignores response capabilities (client.go:202-206) | Full -- `client.getServerCapabilities()` stored on `ConnectedMCPServer.capabilities` (client.ts:1158) |
| **Tools capability** | **No** -- always calls `tools/list` regardless | Checks `capabilities?.tools` before fetching (discovery.ts:50-52) |
| **Prompts capability** | **No** | Yes -- fetches only if server supports prompts (client.ts:1700+) |
| **Resources capability** | **No** | Yes -- `ListMcpResourcesTool` + `ReadMcpResourceTool` only if supported (client.ts:1690+) |
| **Resource subscribe** | **No** | Yes -- checks `capabilities?.resources?.subscribe` (client.ts:1181-1183) |
| **Elicitation capability** | **No** -- client declares empty capabilities | Declares `elicitation: {}` capability (connection.ts:58, client.ts:998) |
| **Roots capability** | **No** -- client declares empty capabilities | Declares `roots: {}` capability + registers `ListRootsRequestSchema` handler (connection.ts:57-73) |
| **Server info** | **No** -- not parsed from initialize response | `serverInfo` captured with name and version (connection.ts:498, client.ts:1159) |
| **Instructions** | Parsed from initialize response `instructions` field (client.go:213-219) | Parsed from `client.getInstructions()` with truncation (connection.ts:499-507) |
| **Protocol version** | Hardcoded `"2024-11-05"` (client.go:202) | Same hardcoded value |

**Finding**: Go sends empty client capabilities and ignores server capabilities from the initialize response. It always calls `tools/list` regardless of whether the server advertises the tools capability. Upstream properly detects and respects all server capabilities, conditionally fetching tools/resources/prompts, and declares client capabilities (roots, elicitation) that servers can rely on.

---


---

## MCP Comparison Summary of Key Gaps

### Critical Gaps (may cause correctness or compatibility issues)
1. **Missing transport types** -- 6 of 8 transports absent; only stdio and basic HTTP+SSE work
2. **No reconnection logic** -- Failed connections require full process restart; upstream auto-reconnects with exponential backoff
3. **No tool name prefixing** -- Go uses raw tool names; upstream uses `mcp__server__tool` format which is part of the MCP specification convention
4. **No server capability detection** -- Go always calls `tools/list` and ignores the server's advertised capabilities
5. **No OAuth/authentication** -- No auth provider for remote servers; all authenticated MCP servers will fail to connect
6. **No session management** -- No detection of expired sessions (HTTP 404 + JSON-RPC -32001); stale sessions cause tool call failures with no recovery

### Missing Features (may cause degraded behavior)
7. **No proxy/TLS support** -- No HTTP proxy, WebSocket proxy, or MTLS configuration for remote servers
8. **No progress callbacks** -- No `onProgress` handler for long-running tool calls
9. **No image/binary content** -- Only text blocks parsed; image results are silently dropped
10. **No output truncation** -- Large MCP results sent to model without truncation, potentially overflowing context
11. **No resource/prompt support** -- `resources/list`, `resources/read`, `prompts/list` endpoints not supported
12. **No env variable expansion** -- `${VAR}` and `${VAR:-default}` syntax not expanded in config
13. **No command/skill discovery** -- No `commands/list` or MCP skills support
14. **No Unicode sanitization** -- MCP server responses not sanitized for control characters
15. **No description/instructions truncation** -- Unbounded descriptions/instructions may bloat context
16. **No headers helper** -- No dynamic header injection for authenticated servers
17. **No Windows npx detection** -- No warning for common Windows misconfiguration
18. **No connection batching** -- Sequential server startup; upstream uses configurable concurrency

### Architectural Differences
19. **Typed error hierarchy vs string errors** -- Upstream has 6 error classes; Go uses `fmt.Errorf` strings
20. **5-state connection model vs 2-state** -- Upstream: connected/failed/needs-auth/pending/disabled; Go: running/not running
21. **Background timeout preservation** -- Go's `CallToolWithTimeout` preserves MCP connection on timeout; upstream rejects the promise
22. **SDK-based vs hand-rolled transports** -- Upstream uses `@modelcontextprotocol/sdk` transports; Go implements JSON-RPC and SSE parsing from scratch
23. **Monolithic client vs package split** -- Go has a single client.go; upstream splits into connection/execution/discovery/manager/errors/sanitization/cache/strings packages

---


---

