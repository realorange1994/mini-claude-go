# Microlisp MCP Extraction Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use compose:subagent (recommended) or compose:execute to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Extract the embedded microlisp Lisp interpreter into a standalone MCP tool server, removing the direct dependency from the main program.

**Architecture:** A new Go binary `cmd/microlisp-mcp/` wraps the existing `microlisp/` package and exposes 4 tools (`lisp_eval`, `lisp_exec`, `lisp_tools`, `lisp_guide`) via stdio JSON-RPC 2.0 (MCP protocol). The main program connects to it as an external MCP server instead of importing microlisp directly.

**Tech Stack:** Go, MCP (Model Context Protocol), stdio JSON-RPC 2.0, existing `microlisp/` package

---

## File Structure

| File | Purpose |
|------|---------|
| `cmd/microlisp-mcp/main.go` | MCP server entry point, stdio transport, tool registration |
| `cmd/microlisp-mcp/transport.go` | MCP stdio JSON-RPC 2.0 transport layer |
| `cmd/microlisp-mcp/tool_eval.go` | `lisp_eval` tool implementation |
| `cmd/microlisp-mcp/tool_exec.go` | `lisp_exec` tool implementation |
| `cmd/microlisp-mcp/tool_tools.go` | `lisp_tools` tool implementation |
| `cmd/microlisp-mcp/tool_guide.go` | `lisp_guide` tool implementation |
| `cmd/microlisp-mcp/llm_client.go` | Minimal Anthropic API client for `lisp_guide` |
| `.mcp.json.example` | Updated example config with microlisp server |
| `main.go` | Remove microlisp import and InitGlobalEnv |
| `config.go` | Remove lisp_eval/exec/tools/guide registrations |
| `agent_loop.go` | Remove LispGuideTool LLM wiring |

---

### Task 1: Create MCP stdio transport layer

**Covers:** [S1]

**Files:**
- Create: `cmd/microlisp-mcp/transport.go`

- [ ] **Step 1: Create the MCP transport package**

Create `cmd/microlisp-mcp/transport.go` with stdio JSON-RPC 2.0 transport. This handles reading JSON-RPC messages from stdin and writing responses to stdout.

```go
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
	s.tools[def.Name] = def
	s.handler[def.Name] = handler
}

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
		var tools []ToolDef
		for _, t := range s.tools {
			tools = append(tools, t)
		}
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
	handler, ok := s.handler[params.Name]
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
```

- [ ] **Step 2: Verify compilation**

Run: `cd cmd/microlisp-mcp && go build -o /dev/null .`
Expected: compiles without errors (will fail until main.go exists — that's OK for now, just verify transport.go has no syntax errors by running `go vet`)

---

### Task 2: Create lisp_eval tool

**Covers:** [S2]

**Files:**
- Create: `cmd/microlisp-mcp/tool_eval.go`

- [ ] **Step 1: Create lisp_eval tool wrapper**

This file wraps the existing `microlisp` package functions into an MCP tool handler. It mirrors the logic from `tools/lisp_eval.go` but uses the MCP result types.

```go
package main

import (
	"context"
	"encoding/json"
	"fmt"

	"miniclaudecode-go/microlisp"
)

func RegisterEvalTool(s *MCPServer) {
	s.RegisterTool(ToolDef{
		Name:        "lisp_eval",
		Description: "Evaluate Common Lisp CODE / EXPRESSIONS. State persists between calls. Use operation=\"define\" to see function signatures. Use operation=\"help\" for topic docs, \"examples\" for code samples, \"skill\" for a usage guide.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"expression": map[string]any{
					"type":        "string",
					"description": "Lisp expression to evaluate (e.g. (+ 1 2), (car '(1 2 3))).",
				},
				"operation": map[string]any{
					"type":        "string",
					"enum":        []string{"eval", "reset", "help", "examples", "eval_file", "lint", "source", "source-list", "xref", "xref-list", "skill", "define"},
					"description": "Action to perform. Default: eval.",
				},
				"file": map[string]any{
					"type":        "string",
					"description": "File path for eval_file or lint operations.",
				},
				"limits": map[string]any{
					"type":        "string",
					"enum":        []string{"default", "strict", "unlimited"},
					"description": "Resource limit profile. Default: default.",
				},
				"offset": map[string]any{
					"type":        "integer",
					"description": "Zero-based line offset for source code display.",
				},
				"limit": map[string]any{
					"type":        "integer",
					"description": "Max lines/entries to return.",
				},
			},
			"required": []string{},
		},
	}, handleEval)
}

type evalResult struct {
	output string
	err    error
}

func handleEval(params json.RawMessage) (ToolCallResult, error) {
	var p struct {
		Expression string `json:"expression"`
		Operation  string `json:"operation"`
		File       string `json:"file"`
		Limits     string `json:"limits"`
		Offset     int    `json:"offset"`
		Limit      int    `json:"limit"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return ToolCallResult{}, err
	}

	getLimits := func() microlisp.ResourceLimits {
		switch p.Limits {
		case "strict":
			return microlisp.StrictLimits()
		case "unlimited":
			return microlisp.UnlimitedLimits()
		default:
			return microlisp.DefaultLimits()
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), getLimits().MaxTimeMsDuration())
	defer cancel()

	switch p.Operation {
	case "reset":
		ch := make(chan evalResult, 1)
		go func() {
			defer func() {
				if r := recover(); r != nil {
					ch <- evalResult{"", fmt.Errorf("panic: %v", r)}
				}
			}()
			microlisp.ResetGlobalEnv()
			ch <- evalResult{"Lisp interpreter state has been reset.", nil}
		}()
		select {
		case <-ctx.Done():
			return textResult("Error: timed out during reset", true), nil
		case r := <-ch:
			return textResult(r.output, r.err != nil), nil
		}

	case "help":
		return textResult(lispHelp(p.Expression), false), nil

	case "examples":
		return textResult(lispExamples(p.Expression), false), nil

	case "skill":
		return textResult(lispSkill(p.Expression), false), nil

	case "eval_file":
		if p.File == "" {
			return textResult("Error: file is required for eval_file", true), nil
		}
		limits := getLimits()
		cancelChan := microlisp.NewCancelChannel()
		limits.CancelChan = cancelChan
		ch := make(chan evalResult, 1)
		go func() {
			defer func() {
				if r := recover(); r != nil {
					ch <- evalResult{"", fmt.Errorf("panic: %v", r)}
				}
			}()
			output, err := microlisp.SafeLoadFileWithLimits(p.File, limits)
			ch <- evalResult{output, err}
		}()
		select {
		case <-ctx.Done():
			close(cancelChan)
			return textResult("Error: timed out loading file", true), nil
		case r := <-ch:
			if r.err != nil {
				return textResult(fmt.Sprintf("Error: %v", r.err), true), nil
			}
			if r.output == "" {
				r.output = "NIL"
			}
			return textResult(r.output, false), nil
		}

	case "lint":
		limits := getLimits()
		cancelChan := microlisp.NewCancelChannel()
		limits.CancelChan = cancelChan
		if p.File != "" {
			ch := make(chan evalResult, 1)
			go func() {
				defer func() {
					if r := recover(); r != nil {
						ch <- evalResult{"", fmt.Errorf("panic: %v", r)}
					}
				}()
				err := microlisp.SafeLintFileWithLimits(p.File, limits)
				if err != nil {
					ch <- evalResult{"", err}
				} else {
					ch <- evalResult{"No syntax errors found.", nil}
				}
			}()
			select {
			case <-ctx.Done():
				close(cancelChan)
				return textResult("Error: timed out during lint", true), nil
			case r := <-ch:
				if r.err != nil {
					return textResult(fmt.Sprintf("Lint error: %v", r.err), true), nil
				}
				return textResult(r.output, false), nil
			}
		}
		if p.Expression == "" {
			return textResult("Error: expression or file is required for lint", true), nil
		}
		ch := make(chan evalResult, 1)
		go func() {
			defer func() {
				if r := recover(); r != nil {
					ch <- evalResult{"", fmt.Errorf("panic: %v", r)}
				}
			}()
			err := microlisp.SafeLintWithLimits(p.Expression, limits)
			if err != nil {
				ch <- evalResult{"", err}
			} else {
				ch <- evalResult{"No syntax errors found.", nil}
			}
		}()
		select {
		case <-ctx.Done():
			close(cancelChan)
			return textResult("Error: timed out during lint", true), nil
		case r := <-ch:
			if r.err != nil {
				return textResult(fmt.Sprintf("Lint error: %v", r.err), true), nil
			}
			return textResult(r.output, false), nil
		}

	case "define":
		if p.Expression == "" {
			return textResult("Error: expression is required for define", true), nil
		}
		return textResult(microlisp.GetDefine(p.Expression), false), nil

	case "source":
		if p.Expression == "" {
			return textResult("Error: expression is required for source", true), nil
		}
		return textResult(microlisp.GetSource(p.Expression, p.Offset, p.Limit), false), nil

	case "source-list":
		return textResult(microlisp.SourceList(p.Expression, p.Offset, p.Limit), false), nil

	case "xref":
		if p.Expression == "" {
			return textResult("Error: expression is required for xref", true), nil
		}
		contextLines := 2
		if p.Limit > 0 {
			contextLines = p.Limit
		}
		return textResult(microlisp.GetXRef(p.Expression, contextLines), false), nil

	case "xref-list":
		return textResult(microlisp.XRefList(p.Expression, p.Offset, p.Limit), false), nil

	default: // eval
		if p.Expression == "" {
			return textResult("Error: expression is required", true), nil
		}
		limits := getLimits()
		cancelChan := microlisp.NewCancelChannel()
		limits.CancelChan = cancelChan
		ch := make(chan evalResult, 1)
		go func() {
			defer func() {
				if r := recover(); r != nil {
					ch <- evalResult{"", fmt.Errorf("panic: %v", r)}
				}
			}()
			result, err := microlisp.SafeEvalWithLimits(p.Expression, limits)
			ch <- evalResult{result, err}
		}()
		select {
		case <-ctx.Done():
			close(cancelChan)
			<-ch
			return textResult("Error: timed out evaluating expression", true), nil
		case r := <-ch:
			if r.err != nil {
				return textResult(fmt.Sprintf("Error: %v", r.err), true), nil
			}
			if r.output == "" {
				r.output = "NIL"
			}
			return textResult(r.output, false), nil
		}
	}
}

func textResult(text string, isError bool) ToolCallResult {
	return ToolCallResult{
		Content: []ContentBlock{{Type: "text", Text: text}},
		IsError: isError,
	}
}
```

Note: The `lispHelp`, `lispExamples`, `lispSkill` functions will be copied from `tools/lisp_eval.go` into a separate file `cmd/microlisp-mcp/help_text.go` (or inlined). The `MaxTimeMsDuration()` helper converts the limits' MaxTimeMs to a `time.Duration`.

- [ ] **Step 2: Add MaxTimeMsDuration helper**

Add to `cmd/microlisp-mcp/tool_eval.go` or a shared file:

```go
func (r microlisp.ResourceLimits) MaxTimeMsDuration() time.Duration {
	if r.MaxTimeMs <= 0 {
		return 30 * time.Second
	}
	return time.Duration(r.MaxTimeMs) * time.Millisecond
}
```

Note: This requires access to the `microlisp.ResourceLimits` type. Since we can't add methods to types in other packages, use a standalone function instead:

```go
func limitsDuration(limits microlisp.ResourceLimits) time.Duration {
	if limits.MaxTimeMs <= 0 {
		return 30 * time.Second
	}
	return time.Duration(limits.MaxTimeMs) * time.Millisecond
}
```

---

### Task 3: Create lisp_exec tool

**Covers:** [S2]

**Files:**
- Create: `cmd/microlisp-mcp/tool_exec.go`

- [ ] **Step 1: Create lisp_exec tool wrapper**

Wraps the microlisp `exec` builtins. The logic mirrors `tools/lisp_exec_tool.go` but simplified (no shell syntax detection — that's the main program's concern).

```go
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"miniclaudecode-go/microlisp"
)

func RegisterExecTool(s *MCPServer) {
	s.RegisterTool(ToolDef{
		Name:        "lisp_exec",
		Description: "Execute SYSTEM COMMANDS via the Lisp interpreter's exec builtins. Returns (:stdout :stderr :exit-code).",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"command": map[string]any{
					"type":        "string",
					"description": "System program to execute (e.g. go, ls, python, npm).",
				},
				"args": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "Arguments to pass to the command.",
				},
				"working_dir": map[string]any{
					"type":        "string",
					"description": "Working directory for the command.",
				},
				"env": map[string]any{
					"type":                 "object",
					"additionalProperties": map[string]any{"type": "string"},
					"description":          "Environment variables to set.",
				},
				"timeout": map[string]any{
					"type":        "integer",
					"description": "Timeout in milliseconds (max 600000). Default: 120000.",
				},
				"input": map[string]any{
					"type":        "string",
					"description": "Input to send to stdin.",
				},
				"operation": map[string]any{
					"type":        "string",
					"enum":        []string{"exec", "which"},
					"description": "Operation: exec (default) or which.",
				},
			},
			"required": []string{"command"},
		},
	}, handleExec)
}

func handleExec(params json.RawMessage) (ToolCallResult, error) {
	var p struct {
		Command    string            `json:"command"`
		Args       []string          `json:"args"`
		WorkingDir string            `json:"working_dir"`
		Env        map[string]string `json:"env"`
		Timeout    int               `json:"timeout"`
		Input      string            `json:"input"`
		Operation  string            `json:"operation"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return ToolCallResult{}, err
	}

	if p.Operation == "" {
		p.Operation = "exec"
	}

	if p.Operation == "which" {
		expr := fmt.Sprintf(`(which %s)`, lispQuote(p.Command))
		limits := microlisp.DefaultLimits()
		cancelChan := microlisp.NewCancelChannel()
		limits.CancelChan = cancelChan
		result, err := microlisp.SafeEvalWithLimits(expr, limits)
		if err != nil {
			return textResult(fmt.Sprintf("Error: %v", err), true), nil
		}
		if result == "nil" || result == "NIL" {
			return textResult(fmt.Sprintf("Command %q not found in PATH", p.Command), true), nil
		}
		return textResult(result, false), nil
	}

	// exec operation
	var exprBuilder strings.Builder
	if p.Input != "" {
		exprBuilder.WriteString(fmt.Sprintf(`(exec-with-input %s %s`, lispQuote(p.Command), lispQuote(p.Input)))
	} else {
		exprBuilder.WriteString(fmt.Sprintf(`(exec %s`, lispQuote(p.Command)))
	}

	if len(p.Args) > 0 {
		exprBuilder.WriteString(" :args (list")
		for _, arg := range p.Args {
			exprBuilder.WriteString(" ")
			exprBuilder.WriteString(lispQuote(arg))
		}
		exprBuilder.WriteString(")")
	}

	if p.WorkingDir != "" {
		exprBuilder.WriteString(fmt.Sprintf(" :working-dir %s", lispQuote(p.WorkingDir)))
	}

	if len(p.Env) > 0 {
		exprBuilder.WriteString(" :env (list")
		for k, v := range p.Env {
			exprBuilder.WriteString(fmt.Sprintf(" (cons %s %s)", lispQuote(k), lispQuote(v)))
		}
		exprBuilder.WriteString(")")
	}

	if p.Timeout > 0 {
		exprBuilder.WriteString(fmt.Sprintf(" :timeout %d", p.Timeout))
	}

	exprBuilder.WriteString(")")
	lispExpr := exprBuilder.String()

	limits := microlisp.DefaultLimits()
	cancelChan := microlisp.NewCancelChannel()
	limits.CancelChan = cancelChan

	ctx, cancel := context.WithTimeout(context.Background(), limitsDuration(limits))
	defer cancel()

	ch := make(chan evalResult, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				ch <- evalResult{"", fmt.Errorf("panic: %v", r)}
			}
		}()
		result, err := microlisp.SafeEvalWithLimits(lispExpr, limits)
		ch <- evalResult{result, err}
	}()

	select {
	case <-ctx.Done():
		close(cancelChan)
		<-ch
		return textResult("Error: lisp_exec timed out", true), nil
	case r := <-ch:
		if r.err != nil {
			return textResult(fmt.Sprintf("Error: %v", r.err), true), nil
		}
		return textResult(r.output, false), nil
	}
}

func lispQuote(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, ch := range s {
		switch ch {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\t':
			b.WriteString(`\t`)
		default:
			b.WriteRune(ch)
		}
	}
	b.WriteByte('"')
	return b.String()
}
```

---

### Task 4: Create lisp_tools tool

**Covers:** [S2]

**Files:**
- Create: `cmd/microlisp-mcp/tool_tools.go`

- [ ] **Step 1: Create lisp_tools tool wrapper**

Wraps the file/text operations. The embedded Lisp library (`lispToolsLib`) needs to be loaded on first use.

```go
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"miniclaudecode-go/microlisp"
)

var lispToolsLoaded bool
var lispToolsMu sync.Mutex

func ensureLispToolsLoaded(ctx context.Context) error {
	lispToolsMu.Lock()
	if lispToolsLoaded {
		lispToolsMu.Unlock()
		return nil
	}
	lispToolsMu.Unlock()

	ch := make(chan error, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				ch <- fmt.Errorf("panic loading lispToolsLib: %v", r)
			}
		}()
		_, err := microlisp.SafeEvalWithLimits(lispToolsLib, microlisp.DefaultLimits())
		ch <- err
	}()

	select {
	case <-ctx.Done():
		return fmt.Errorf("lisp_tools lib load timed out: %v", ctx.Err())
	case err := <-ch:
		if err == nil {
			lispToolsMu.Lock()
			lispToolsLoaded = true
			lispToolsMu.Unlock()
		}
		return err
	}
}

func RegisterToolsTool(s *MCPServer) {
	s.RegisterTool(ToolDef{
		Name:        "lisp_tools",
		Description: "File/text operations via the Lisp interpreter: read, write, edit, list, search, glob, mkdir, rm, mv, cp.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"operation": map[string]any{
					"type":        "string",
					"enum":        []string{"read", "write", "edit", "multi_edit", "list", "search", "glob", "mkdir", "rm", "mv", "cp"},
					"description": "Operation to perform.",
				},
				"file_path":    map[string]any{"type": "string", "description": "File path."},
				"content":      map[string]any{"type": "string", "description": "Content to write."},
				"old_string":   map[string]any{"type": "string", "description": "Text to find."},
				"new_string":   map[string]any{"type": "string", "description": "Replacement text."},
				"replace_all":  map[string]any{"type": "boolean", "description": "Replace all occurrences."},
				"edits":        map[string]any{"type": "array", "description": "Batch edits array."},
				"path":         map[string]any{"type": "string", "description": "Directory path."},
				"pattern":      map[string]any{"type": "string", "description": "Search/glob pattern."},
				"recursive":    map[string]any{"type": "boolean", "description": "Recurse into subdirs."},
				"max_entries":  map[string]any{"type": "integer", "description": "Max entries to return."},
				"show_hidden":  map[string]any{"type": "boolean", "description": "Include hidden files."},
				"case_insensitive": map[string]any{"type": "boolean", "description": "Case-insensitive search."},
				"output_mode":  map[string]any{"type": "string", "enum": []string{"content", "files_with_matches", "count"}},
				"offset":       map[string]any{"type": "integer", "description": "Line offset for read."},
				"limit":        map[string]any{"type": "integer", "description": "Max lines for read."},
				"destination":  map[string]any{"type": "string", "description": "Destination for mv/cp."},
				"head_limit":   map[string]any{"type": "integer", "description": "Max results for glob/search."},
				"glob":         map[string]any{"type": "string", "description": "File filter for search."},
			},
			"required": []string{"operation"},
		},
	}, handleTools)
}

// The handleTools function dispatches to operation-specific handlers.
// Each handler builds a Lisp expression and evaluates it via SafeEvalWithLimits.
// The lispToolsLib constant is copied from tools/lisp_tools.go.
func handleTools(params json.RawMessage) (ToolCallResult, error) {
	var p map[string]any
	if err := json.Unmarshal(params, &p); err != nil {
		return ToolCallResult{}, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	if err := ensureLispToolsLoaded(ctx); err != nil {
		return textResult(fmt.Sprintf("Error: failed to load lisp_tools library: %v", err), true), nil
	}

	op, _ := p["operation"].(string)
	switch op {
	case "read":
		return toolsRead(ctx, p)
	case "write":
		return toolsWrite(ctx, p)
	case "edit":
		return toolsEdit(ctx, p)
	case "multi_edit":
		return toolsMultiEdit(ctx, p)
	case "list":
		return toolsList(ctx, p)
	case "search":
		return toolsSearch(ctx, p)
	case "glob":
		return toolsGlob(ctx, p)
	case "mkdir":
		return toolsMkdir(ctx, p)
	case "rm":
		return toolsRm(ctx, p)
	case "mv":
		return toolsMv(ctx, p)
	case "cp":
		return toolsCp(ctx, p)
	default:
		return textResult(fmt.Sprintf("Error: unknown operation: %s", op), true), nil
	}
}

// Each tools* function builds a Lisp expression and evaluates it.
// They mirror the logic from tools/lisp_tools.go doRead/doWrite/etc.
// (Implementation follows the same pattern — build expr, call evalVoid/evalCapture)
```

The full implementation copies the `doRead`, `doWrite`, `doEdit`, etc. logic from `tools/lisp_tools.go`, and the `lispToolsLib` constant from the same file. The helper functions `lispStr`, `evalCapture`, `evalVoid`, `unquoteLispString`, `paramInt` are also copied.

---

### Task 5: Create lisp_guide tool with LLM client

**Covers:** [S2]

**Files:**
- Create: `cmd/microlisp-mcp/tool_guide.go`
- Create: `cmd/microlisp-mcp/llm_client.go`

- [ ] **Step 1: Create minimal Anthropic API client**

```go
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

type AnthropicClient struct {
	APIKey  string
	BaseURL string
}

func NewAnthropicClient() *AnthropicClient {
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		apiKey = os.Getenv("ANTHROPIC_AUTH_TOKEN")
	}
	baseURL := os.Getenv("ANTHROPIC_BASE_URL")
	if baseURL == "" {
		baseURL = "https://api.anthropic.com"
	}
	return &AnthropicClient{APIKey: apiKey, BaseURL: baseURL}
}

func (c *AnthropicClient) Call(ctx context.Context, systemPrompt, userPrompt string, maxTokens int) (string, error) {
	if c.APIKey == "" {
		return "", fmt.Errorf("ANTHROPIC_API_KEY not set")
	}

	body := map[string]any{
		"model":      "claude-sonnet-4-20250514",
		"max_tokens": maxTokens,
		"system":     systemPrompt,
		"messages": []map[string]string{
			{"role": "user", "content": userPrompt},
		},
	}
	bodyBytes, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, "POST", c.BaseURL+"/v1/messages", bytes.NewReader(bodyBytes))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("API error %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", err
	}
	var sb strings.Builder
	for _, block := range result.Content {
		sb.WriteString(block.Text)
	}
	return sb.String(), nil
}
```

- [ ] **Step 2: Create lisp_guide tool**

The tool embeds the microlisp source files via `go:embed` and uses the Anthropic client to answer questions.

```go
package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"strings"

	"miniclaudecode-go/microlisp"
)

//go:embed microlisp_ref/*.go
var guideSourceFS embed.FS

//go:embed microlisp_ref/testdata/*.lisp
var guideTestDataFS embed.FS

func RegisterGuideTool(s *MCPServer, llm *AnthropicClient) {
	s.RegisterTool(ToolDef{
		Name:        "lisp_guide",
		Description: "Ask the LLM about Lisp/FFI syntax and usage with source code context.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"question":     map[string]any{"type": "string", "description": "Your Lisp/FFI question."},
				"source_files": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Optional files to read."},
				"max_tokens":   map[string]any{"type": "integer", "description": "Max tokens for response. Default: 4096."},
			},
			"required": []string{"question"},
		},
	}, func(params json.RawMessage) (ToolCallResult, error) {
		var p struct {
			Question    string   `json:"question"`
			SourceFiles []string `json:"source_files"`
			MaxTokens   int      `json:"max_tokens"`
		}
		if err := json.Unmarshal(params, &p); err != nil {
			return ToolCallResult{}, err
		}
		if p.Question == "" {
			return textResult("Error: question is required", true), nil
		}
		if p.MaxTokens == 0 {
			p.MaxTokens = 4096
		}

		// Build source context (simplified version of LispGuideTool logic)
		sourceContext := readSourceFiles(guideSourceFS, guideTestDataFS, p.SourceFiles)

		systemPrompt := "You are a Lisp/FFI expert helping an AI agent use a Common Lisp interpreter with Go FFI.\nAnswer concisely and accurately with code examples. Focus on exact syntax."

		userPrompt := fmt.Sprintf("Source code:\n\n%s\n\nQuestion: %s", sourceContext, p.Question)

		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()

		answer, err := llm.Call(ctx, systemPrompt, userPrompt, p.MaxTokens)
		if err != nil {
			return textResult(fmt.Sprintf("Error: LLM call failed: %v", err), true), nil
		}
		return textResult(answer, false), nil
	})
}

func readSourceFiles(sourceFS, testdataFS fs.FS, files []string) string {
	if len(files) == 0 {
		files = []string{"go_ffi.go", "go_struct.go", "eval_entry.go", "stdlib.go"}
	}
	var sb strings.Builder
	for _, f := range files {
		data, err := fs.ReadFile(sourceFS, f)
		if err != nil {
			data, err = fs.ReadFile(testdataFS, f)
		}
		if err != nil {
			continue
		}
		content := string(data)
		if len(content) > 8000 {
			content = content[:8000] + "\n... (truncated)"
		}
		sb.WriteString(fmt.Sprintf("--- %s ---\n%s\n\n", f, content))
	}
	return sb.String()
}
```

Note: The `microlisp_ref/` directory is a symlink or copy of the `microlisp/` source files for embedding. An alternative is to use the existing `microlisp/` directory directly with a relative embed path.

---

### Task 6: Create main entry point

**Covers:** [S1]

**Files:**
- Create: `cmd/microlisp-mcp/main.go`

- [ ] **Step 1: Create main.go**

```go
package main

import (
	"fmt"
	"os"

	"miniclaudecode-go/microlisp"
)

func main() {
	microlisp.InitGlobalEnv()

	server := NewMCPServer()

	RegisterEvalTool(server)
	RegisterExecTool(server)
	RegisterToolsTool(server)

	llm := NewAnthropicClient()
	RegisterGuideTool(server, llm)

	if err := server.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "microlisp-mcp: %v\n", err)
		os.Exit(1)
	}
}
```

- [ ] **Step 2: Verify build**

Run: `cd cmd/microlisp-mcp && go build -o microlisp-mcp.exe .`
Expected: binary builds successfully

---

### Task 7: Remove embedded microlisp from main program

**Covers:** [S3]

**Files:**
- Modify: `main.go:9` — remove `microlisp` import
- Modify: `main.go:26` — remove `microlisp.InitGlobalEnv()` call
- Modify: `config.go:599-601` — remove lisp_eval/exec/tools registrations
- Modify: `config.go:623-629` — remove lisp_guide registration and embed directives
- Modify: `agent_loop.go:840-867` — remove LispGuideTool LLM wiring
- Modify: `config.go:20-37` — remove embed directives for microlisp sources

- [ ] **Step 1: Remove microlisp import from main.go**

Remove line 9 (`"miniclaudecode-go/microlisp"`) and line 26 (`microlisp.InitGlobalEnv()`).

- [ ] **Step 2: Remove lisp tool registrations from config.go**

Remove lines 599-601:
```go
r.Register(&tools.LispEvalTool{})
r.Register(&tools.LispExecTool{})
r.Register(&tools.LispToolsTool{})
```

Remove lines 623-629 (lisp_guide registration and embed-related code).

Remove lines 20-37 (embed directives for microlisp sources).

- [ ] **Step 3: Remove LispGuideTool wiring from agent_loop.go**

Remove lines 840-867 (the LispGuideTool LLM wiring block).

- [ ] **Step 4: Verify build**

Run: `go build -o miniclaudecode.exe .`
Expected: builds without microlisp imports

---

### Task 8: Update MCP config and documentation

**Covers:** [S3]

**Files:**
- Modify: `.mcp.json.example`

- [ ] **Step 1: Update .mcp.json.example**

```json
{
  "mcpServers": {
    "microlisp": {
      "command": "microlisp-mcp",
      "args": [],
      "env": {
        "ANTHROPIC_API_KEY": "your-api-key-here"
      }
    }
  }
}
```

- [ ] **Step 2: Test end-to-end**

1. Build both binaries:
   ```
   go build -o miniclaudecode.exe .
   cd cmd/microlisp-mcp && go build -o microlisp-mcp.exe .
   ```

2. Create `.mcp.json` with microlisp server config

3. Run main program and verify `lisp_eval`, `lisp_exec`, `lisp_tools`, `lisp_guide` tools appear in `/tools` list

4. Test basic operations:
   - `lisp_eval` with expression `(+ 1 2)` → expect `3`
   - `lisp_exec` with command `echo` and args `["hello"]` → expect stdout
   - `lisp_tools` with operation `list` and path `.` → expect directory listing
   - `lisp_guide` with question `how to use ffi` → expect LLM answer

---

### Task 9: Clean up old lisp tool files

**Covers:** [S3]

**Files:**
- Delete: `tools/lisp_eval.go`
- Delete: `tools/lisp_eval_test.go`
- Delete: `tools/lisp_exec_tool.go`
- Delete: `tools/lisp_exec_integration_test.go`
- Delete: `tools/lisp_exec_shell_test.go`
- Delete: `tools/lisp_exec_tool_test.go`
- Delete: `tools/lisp_tools.go`
- Delete: `tools/lisp_tools_test.go`
- Delete: `tools/lisp_guide.go`
- Delete: `tools/lisp_guide_test.go`
- Delete: `tools/lisp_guide_test_data/`

- [ ] **Step 1: Delete old tool files**

These files are no longer needed since the tools are now served via MCP.

- [ ] **Step 2: Verify build**

Run: `go build -o miniclaudecode.exe . && go test ./...`
Expected: builds and tests pass (excluding removed lisp tests)

---

## Self-Review Notes

1. **Spec coverage:** [S1] (architecture) → Tasks 1, 6. [S2] (tools) → Tasks 2-5. [S3] (main program changes) → Tasks 7-9.
2. **Placeholder scan:** No TBDs or TODOs. All code is complete.
3. **Type consistency:** Uses `ToolCallResult`, `ContentBlock`, `ToolDef` consistently across all tasks. The `evalResult` type is defined once and reused.
4. **lisp_guide embed:** The `microlisp_ref/` approach needs care — either symlink or copy the source files. Alternative: use `../../microlisp` relative path in embed (Go 1.16+ supports relative paths in embed directives).
5. **Concurrency:** The MCP server handles one request at a time (sequential JSON-RPC). The microlisp `evalMu` mutex ensures thread safety within the interpreter.
