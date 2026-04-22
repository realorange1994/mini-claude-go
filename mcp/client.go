// Package mcp implements MCP (Model Context Protocol) client.
// Communicates with MCP servers via stdio JSON-RPC 2.0.
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"
)

// --- JSON-RPC Types ---

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

// --- Tool Types ---

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

// --- Client ---

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
}

func NewClient(name, command string, args []string, env map[string]string) *Client {
	cmd := exec.Command(command, args...)
	if env != nil {
		cmd.Env = os.Environ()
		for k, v := range env {
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
		}
	}
	return &Client{name: name, cmd: cmd}
}

// Start launches the MCP server process and initializes the connection.
func (c *Client) Start(ctx context.Context) error {
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

	if err := c.initialize(ctx); err != nil {
		return fmt.Errorf("initialize %s: %w", c.name, err)
	}

	tools, err := c.ListTools(ctx)
	if err != nil {
		return fmt.Errorf("list tools %s: %w", c.name, err)
	}
	c.tools = tools

	return nil
}

func (c *Client) initialize(ctx context.Context) error {
	params := json.RawMessage(`{
		"protocolVersion": "2024-11-05",
		"capabilities": {},
		"clientInfo": {"name": "miniclaudecode", "version": "0.1.0"}
	}`)

	_, err := c.request(ctx, "initialize", params)
	if err != nil {
		return err
	}

	notif := RPCNotification{
		JSONRPC: "2.0",
		Method:  "notifications/initialized",
	}
	return c.send(notif)
}

// ListTools discovers available tools from this MCP server.
func (c *Client) ListTools(ctx context.Context) ([]Tool, error) {
	resp, err := c.request(ctx, "tools/list", nil)
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
	params, _ := json.Marshal(paramsMap)

	resp, err := c.request(ctx, "tools/call", json.RawMessage(params))
	if err != nil {
		return nil, err
	}

	var result ToolResult
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) request(ctx context.Context, method string, params json.RawMessage) (json.RawMessage, error) {
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
		// Close stdin to unblock the read goroutine and force server to exit
		c.stdin.Close()
		<-done // wait for reader to finish
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

func (c *Client) send(v any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	data, _ := json.Marshal(v)
	_, err := c.stdin.Write(append(data, '\n'))
	return err
}

// Stop terminates the MCP server process.
func (c *Client) Stop() error {
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

// --- Manager ---

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

// Register adds an MCP server.
func (m *Manager) Register(name, command string, args []string, env map[string]string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.clients[name] = NewClient(name, command, args, env)
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
