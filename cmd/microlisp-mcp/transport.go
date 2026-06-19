package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
)

type RPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *int64          `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type RPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *int64          `json:"id,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

type ToolDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

type ToolCallResult struct {
	Content []ContentBlock `json:"content"`
	IsError bool           `json:"isError"`
}

type ContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

type MCPServer struct {
	tools   map[string]ToolDef
	handler map[string]func(json.RawMessage) (ToolCallResult, error)
	mu      sync.Mutex
	reader  *bufio.Reader
	writer  io.Writer
}

func NewMCPServer() *MCPServer {
	return &MCPServer{
		tools:   make(map[string]ToolDef),
		handler: make(map[string]func(json.RawMessage) (ToolCallResult, error)),
		reader:  bufio.NewReader(os.Stdin),
		writer:  os.Stdout,
	}
}

func (s *MCPServer) RegisterTool(def ToolDef, handler func(json.RawMessage) (ToolCallResult, error)) {
	mcpMu.Lock()
	defer mcpMu.Unlock()
	s.tools[def.Name] = def
	s.handler[def.Name] = handler
}

var mcpMu sync.Mutex

func (s *MCPServer) Run() error {
	decoder := json.NewDecoder(s.reader)
	for {
		var req RPCRequest
		if err := decoder.Decode(&req); err != nil {
			if err == io.EOF {
				return nil
			}
			return fmt.Errorf("decode error: %w", err)
		}
		s.handleRequest(req)
	}
}

func (s *MCPServer) handleRequest(req RPCRequest) {
	switch req.Method {
	case "initialize":
		s.respond(req.ID, map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities": map[string]any{
				"tools": map[string]any{},
			},
			"serverInfo": map[string]any{
				"name":    "microlisp-mcp",
				"version": "1.0.0",
			},
		})
	case "tools/list":
		mcpMu.Lock()
		var tools []ToolDef
		for _, t := range s.tools {
			tools = append(tools, t)
		}
		mcpMu.Unlock()
		s.respond(req.ID, map[string]any{"tools": tools})
	case "tools/call":
		s.handleToolCall(req)
	case "notifications/initialized":
		// no response needed for notifications
	default:
		if req.ID != nil {
			s.respondError(req.ID, -32601, "Method not found: "+req.Method, nil)
		}
	}
}

func (s *MCPServer) handleToolCall(req RPCRequest) {
	var params struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		s.respondError(req.ID, -32602, "Invalid params: "+err.Error(), nil)
		return
	}
	mcpMu.Lock()
	handler, ok := s.handler[params.Name]
	mcpMu.Unlock()
	if !ok {
		s.respondError(req.ID, -32602, "Unknown tool: "+params.Name, nil)
		return
	}
	result, err := handler(params.Arguments)
	if err != nil {
		s.respondError(req.ID, -32603, err.Error(), nil)
		return
	}
	s.respond(req.ID, result)
}

func (s *MCPServer) respond(id *int64, result any) {
	data, _ := json.Marshal(result)
	resp := RPCResponse{JSONRPC: "2.0", ID: id, Result: data}
	s.writeResponse(resp)
}

func (s *MCPServer) respondError(id *int64, code int, message string, data any) {
	resp := RPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &RPCError{Code: code, Message: message, Data: data},
	}
	s.writeResponse(resp)
}

func (s *MCPServer) writeResponse(resp RPCResponse) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, _ := json.Marshal(resp)
	fmt.Fprintf(s.writer, "%s\n", data)
}
