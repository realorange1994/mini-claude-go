// Package mcp implements MCP (Model Context Protocol) client.
// Communicates with MCP servers via stdio JSON-RPC 2.0 or HTTP+SSE for remote servers.
package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// ─── JSON-RPC Types ───

type RPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type RPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

type RPCNotification struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// ─── Tool Types ───

type Tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

type ToolCallArgs map[string]any

type ToolResult struct {
	Content []ContentBlock `json:"content"`
	IsError bool           `json:"isError"`
}

type ContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// ─── Client ───

type serverType int

const (
	stdioServer serverType = iota
	remoteServer
)

type Client struct {
	name    string
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	stdout  *bufio.Reader
	stderr  io.ReadCloser
	mu      sync.Mutex
	nextID  int64
	tools   []Tool
	running bool

	// Server type
	serverType serverType

	// For remote servers
	url        string
	httpClient *http.Client
	headers    map[string]string
}

func NewClient(name, command string, args []string, env map[string]string) *Client {
	cmd := exec.Command(command, args...)
	if env != nil {
		cmd.Env = os.Environ()
		for k, v := range env {
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
		}
	}
	return &Client{
		name:       name,
		cmd:        cmd,
		serverType: stdioServer,
	}
}

func NewRemoteClient(name, url string, headers map[string]string) *Client {
	return &Client{
		name:       name,
		url:        url,
		serverType: remoteServer,
		httpClient: &http.Client{Timeout: 60 * time.Second},
		headers:    headers,
	}
}

// Start launches the MCP server process and initializes the connection.
func (c *Client) Start(ctx context.Context) error {
	if c.serverType == remoteServer {
		return c.startRemote(ctx)
	}
	return c.startStdio(ctx)
}

func (c *Client) startStdio(ctx context.Context) error {
	stdin, err := c.cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("stdin pipe: %w", err)
	}
	stdout, err := c.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}
	stderr, err := c.cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("stderr pipe: %w", err)
	}

	c.stdin = stdin
	c.stdout = bufio.NewReader(stdout)
	c.stderr = stderr
	c.running = true

	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := c.stderr.Read(buf)
			if n > 0 {
				fmt.Fprintf(os.Stderr, "[%s stderr] %s", c.name, string(buf[:n]))
			}
			if err != nil {
				return
			}
		}
	}()

	if err := c.cmd.Start(); err != nil {
		return fmt.Errorf("start %s: %w", c.name, err)
	}

	if err := c.initializeStdio(ctx); err != nil {
		return fmt.Errorf("initialize %s: %w", c.name, err)
	}

	tools, err := c.listToolsStdio(ctx)
	if err != nil {
		return fmt.Errorf("list tools %s: %w", c.name, err)
	}
	c.tools = tools

	return nil
}

func (c *Client) startRemote(ctx context.Context) error {
	c.running = true

	// Initialize via HTTP POST
	if err := c.initializeRemote(ctx); err != nil {
		return fmt.Errorf("initialize %s: %w", c.name, err)
	}

	// List tools via HTTP POST
	tools, err := c.listToolsRemote(ctx)
	if err != nil {
		return fmt.Errorf("list tools %s: %w", c.name, err)
	}
	c.tools = tools

	return nil
}

func (c *Client) initializeStdio(ctx context.Context) error {
	params := json.RawMessage(`{
		"protocolVersion": "2024-11-05",
		"capabilities": {},
		"clientInfo": {"name": "miniclaudecode", "version": "0.1.0"}
	}`)

	_, err := c.requestStdio(ctx, "initialize", params)
	if err != nil {
		return err
	}

	notif := RPCNotification{
		JSONRPC: "2.0",
		Method:  "notifications/initialized",
	}
	return c.sendStdio(notif)
}

func (c *Client) initializeRemote(ctx context.Context) error {
	params := map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "miniclaudecode", "version": "0.1.0"},
	}
	_, err := c.requestRemote(ctx, "initialize", params)
	return err
}

// ListTools discovers available tools from this MCP server.
func (c *Client) ListTools(ctx context.Context) ([]Tool, error) {
	if c.serverType == remoteServer {
		return c.listToolsRemote(ctx)
	}
	return c.listToolsStdio(ctx)
}

func (c *Client) listToolsStdio(ctx context.Context) ([]Tool, error) {
	resp, err := c.requestStdio(ctx, "tools/list", nil)
	if err != nil {
		return nil, err
	}

	var result struct {
		Tools []Tool `json:"tools"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, err
	}
	return result.Tools, nil
}

func (c *Client) listToolsRemote(ctx context.Context) ([]Tool, error) {
	resp, err := c.requestRemote(ctx, "tools/list", nil)
	if err != nil {
		return nil, err
	}

	var result struct {
		Tools []Tool `json:"tools"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, err
	}
	return result.Tools, nil
}

// CallTool invokes a tool on this MCP server.
func (c *Client) CallTool(ctx context.Context, name string, args ToolCallArgs) (*ToolResult, error) {
	paramsMap := map[string]any{"name": name}
	if args != nil {
		paramsMap["arguments"] = args
	}

	if c.serverType == remoteServer {
		return c.callToolRemote(ctx, name, paramsMap)
	}
	return c.callToolStdio(ctx, name, paramsMap)
}

func (c *Client) callToolStdio(ctx context.Context, name string, paramsMap map[string]any) (*ToolResult, error) {
	params, _ := json.Marshal(paramsMap)
	resp, err := c.requestStdio(ctx, "tools/call", params)
	if err != nil {
		return nil, err
	}

	var result ToolResult
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) callToolRemote(ctx context.Context, name string, paramsMap map[string]any) (*ToolResult, error) {
	resp, err := c.requestRemote(ctx, "tools/call", paramsMap)
	if err != nil {
		return nil, err
	}

	var result ToolResult
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// ─── Stdio transport ─────────────────────────────────────────────────────────

func (c *Client) requestStdio(ctx context.Context, method string, params json.RawMessage) (json.RawMessage, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.nextID++
	req := RPCRequest{
		JSONRPC: "2.0",
		ID:      c.nextID,
		Method:  method,
		Params:  params,
	}

	data, _ := json.Marshal(req)
	if _, err := c.stdin.Write(append(data, '\n')); err != nil {
		return nil, fmt.Errorf("write: %w", err)
	}

	done := make(chan struct{})
	var resp RPCResponse
	var readErr error

	go func() {
		line, err := c.stdout.ReadBytes('\n')
		if err != nil {
			readErr = err
			close(done)
			return
		}
		readErr = json.Unmarshal(line, &resp)
		close(done)
	}()

	select {
	case <-ctx.Done():
		c.stdin.Close()
		<-done
		return nil, ctx.Err()
	case <-done:
		if readErr != nil {
			return nil, fmt.Errorf("read: %w", readErr)
		}
		if resp.Error != nil {
			return nil, fmt.Errorf("MCP error [%d]: %s", resp.Error.Code, resp.Error.Message)
		}
		return resp.Result, nil
	}
}

func (c *Client) sendStdio(v any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	data, _ := json.Marshal(v)
	_, err := c.stdin.Write(append(data, '\n'))
	return err
}

// ─── Remote HTTP+SSE transport ────────────────────────────────────────────────

func (c *Client) requestRemote(ctx context.Context, method string, params any) (json.RawMessage, error) {
	c.mu.Lock()
	c.nextID++
	reqID := c.nextID
	c.mu.Unlock()

	var reqBody []byte
	var err error

	switch p := params.(type) {
	case nil:
		reqBody, err = json.Marshal(RPCRequest{JSONRPC: "2.0", ID: reqID, Method: method})
	case json.RawMessage:
		reqBody, err = json.Marshal(RPCRequest{JSONRPC: "2.0", ID: reqID, Method: method, Params: p})
	case map[string]any:
		reqBody, err = json.Marshal(map[string]any{
			"jsonrpc": "2.0",
			"id":      reqID,
			"method":  method,
			"params":  p,
		})
	default:
		reqBody, err = json.Marshal(RPCRequest{JSONRPC: "2.0", ID: reqID, Method: method})
	}
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.url, bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	for k, v := range c.headers {
		req.Header.Set(k, v)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	// Read SSE response
	reader := io.LimitReader(resp.Body, 2*1024*1024)
	sseResp, err := c.readSSE(reader)
	if err != nil {
		return nil, fmt.Errorf("SSE read: %w", err)
	}

	var rpcResp RPCResponse
	if err := json.Unmarshal(sseResp, &rpcResp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	if rpcResp.Error != nil {
		return nil, fmt.Errorf("MCP error [%d]: %s", rpcResp.Error.Code, rpcResp.Error.Message)
	}
	return rpcResp.Result, nil
}

func (c *Client) readSSE(reader io.Reader) ([]byte, error) {
	buf := new(bytes.Buffer)
	scanner := bufio.NewScanner(reader)
	// Increase buffer for large responses
	buf.Grow(256 * 1024)
	scanner.Buffer(make([]byte, 0, 64*1024), 256*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") {
			data := strings.TrimPrefix(line, "data: ")
			// Skip ping/keepalive lines
			if data == "" || data == ":" {
				continue
			}
			// If it's a JSON object with a result, return it
			if strings.Contains(data, `"result"`) || strings.Contains(data, `"error"`) {
				return []byte(data), nil
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// ─── Stop ─────────────────────────────────────────────────────────────────────

func (c *Client) Stop() error {
	if c.serverType == remoteServer {
		if !c.running {
			return nil
		}
		c.running = false
		return nil
	}

	if !c.running {
		return nil
	}
	c.running = false
	c.stdin.Close()

	done := make(chan error, 1)
	go func() {
		done <- c.cmd.Wait()
	}()

	select {
	case <-done:
		return nil
	case <-time.After(5 * time.Second):
		return c.cmd.Process.Kill()
	}
}

// Tools returns discovered tools.
func (c *Client) Tools() []Tool {
	return c.tools
}

// ─── Manager ───

type ToolWithServer struct {
	Tool
	Server string
}

type Manager struct {
	clients map[string]*Client
	mu      sync.RWMutex
}

func NewManager() *Manager {
	return &Manager{clients: make(map[string]*Client)}
}

// Register adds an MCP stdio server.
func (m *Manager) Register(name, command string, args []string, env map[string]string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.clients[name] = NewClient(name, command, args, env)
}

// RegisterRemote adds an MCP remote server via URL.
func (m *Manager) RegisterRemote(name, url string, headers map[string]string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.clients[name] = NewRemoteClient(name, url, headers)
}

// StartAll launches all registered MCP servers.
func (m *Manager) StartAll(ctx context.Context) error {
	m.mu.RLock()
	clients := make(map[string]*Client, len(m.clients))
	for k, v := range m.clients {
		clients[k] = v
	}
	m.mu.RUnlock()

	started := make(map[string]*Client)
	for name, client := range clients {
		if err := client.Start(ctx); err != nil {
			for n, c := range started {
				_ = c.Stop()
				delete(started, n)
			}
			return fmt.Errorf("MCP server %s: %w", name, err)
		}
		started[name] = client
	}
	return nil
}

// CallTool finds and calls a tool across all MCP servers.
func (m *Manager) CallTool(ctx context.Context, name string, args ToolCallArgs) (*ToolResult, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, client := range m.clients {
		for _, tool := range client.tools {
			if tool.Name == name {
				return client.CallTool(ctx, name, args)
			}
		}
	}
	return nil, fmt.Errorf("tool not found: %s", name)
}

// CallToolWithServer calls a tool on a specific server.
func (m *Manager) CallToolWithServer(ctx context.Context, server, toolName string, args ToolCallArgs) (*ToolResult, error) {
	m.mu.RLock()
	client, ok := m.clients[server]
	m.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("server not found: %s", server)
	}

	return client.CallTool(ctx, toolName, args)
}

// ListTools returns all discovered tools from all servers.
func (m *Manager) ListTools() []Tool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var all []Tool
	for _, client := range m.clients {
		all = append(all, client.tools...)
	}
	return all
}

// ListServers returns list of registered server names.
func (m *Manager) ListServers() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var servers []string
	for name := range m.clients {
		servers = append(servers, name)
	}
	return servers
}

// GetServerStatus returns the connection status of a server.
func (m *Manager) GetServerStatus(name string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	client, ok := m.clients[name]
	if !ok {
		return "not registered"
	}
	if client.running {
		return "connected"
	}
	return "disconnected"
}

// AllToolsWithServer returns all discovered tools annotated with their source server.
func (m *Manager) AllToolsWithServer() []ToolWithServer {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var all []ToolWithServer
	for serverName, client := range m.clients {
		for _, tool := range client.tools {
			all = append(all, ToolWithServer{
				Tool:   tool,
				Server: serverName,
			})
		}
	}
	return all
}

// StopAll terminates all MCP servers.
func (m *Manager) StopAll() {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, client := range m.clients {
		client.Stop()
	}
}
