package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ─── LSP Protocol Implementation ────────────────────────────────────────────
//
// Full LSP (Language Server Protocol) implementation.
// Supports Go (gopls), TypeScript (typescript-language-server), Python (pylsp).
//
// LSP Specification: https://microsoft.github.io/language-server-protocol/

// LSP Diagnostic severity levels
const (
	LSPSeverityError   = 1
	LSPSeverityWarning = 2
	LSPSeverityInfo    = 3
	LSPSeverityHint    = 4
)

// LSPDiagnostic represents a diagnostic from the LSP.
type LSPDiagnostic struct {
	File     string `json:"file"`
	Line     int    `json:"line"`
	Column   int    `json:"column"`
	Severity string `json:"severity"` // error, warning, info, hint
	Message  string `json:"message"`
	Source   string `json:"source"`
}

// LSPDocument represents an open document in the LSP.
type LSPDocument struct {
	URI        string `json:"uri"`
	LanguageID string `json:"languageId"`
	Version    int    `json:"version"`
	Text       string `json:"text"`
}

// LSPPosition represents a position in a text document.
type LSPPosition struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

// LSPRange represents a range in a text document.
type LSPRange struct {
	Start LSPPosition `json:"start"`
	End   LSPPosition `json:"end"`
}

// LSPDiagnosticFull represents a full LSP diagnostic.
type LSPDiagnosticFull struct {
	Range    LSPRange `json:"range"`
	Severity int      `json:"severity,omitempty"`
	Code     string   `json:"code,omitempty"`
	Source   string   `json:"source,omitempty"`
	Message  string   `json:"message"`
}

// LSPTextDocumentItem represents a text document item.
type LSPTextDocumentItem struct {
	URI        string `json:"uri"`
	LanguageID string `json:"languageId"`
	Version    int    `json:"version"`
	Text       string `json:"text"`
}

// LSPTextDocumentIdentifier represents a text document identifier.
type LSPTextDocumentIdentifier struct {
	URI string `json:"uri"`
}

// LSPVersionedTextDocumentIdentifier represents a versioned text document identifier.
type LSPVersionedTextDocumentIdentifier struct {
	URI     string `json:"uri"`
	Version int    `json:"version"`
}

// LSPTextDocumentContentChangeEvent represents a text document content change.
type LSPTextDocumentContentChangeEvent struct {
	Text string `json:"text"`
}

// LSPServer represents a running LSP server with full protocol support.
type LSPServer struct {
	Name        string
	Command     string
	Args        []string
	Extensions  []string
	cmd         *exec.Cmd
	stdin       io.WriteCloser
	stdout      *bufio.Reader
	mu          sync.Mutex
	running     bool
	nextID      int
	initialized bool
	documents   map[string]*LSPDocument
	pendingReqs map[int]chan *LSPResponse
	diagnostics map[string][]LSPDiagnosticFull
}

// LSPResponse represents an LSP JSON-RPC response.
type LSPResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *LSPResponseError `json:"error,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// LSPResponseError represents an LSP error response.
type LSPResponseError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// LSPInitializeParams represents the initialize request parameters.
type LSPInitializeParams struct {
	ProcessID             int                `json:"processId"`
	RootURI               string             `json:"rootUri"`
	Capabilities          LSPClientCapabilities `json:"capabilities"`
}

// LSPClientCapabilities represents client capabilities.
type LSPClientCapabilities struct {
	TextDocument *LSPTextDocumentClientCapabilities `json:"textDocument,omitempty"`
}

// LSPTextDocumentClientCapabilities represents text document capabilities.
type LSPTextDocumentClientCapabilities struct {
	Synchronization *LSPTextDocumentSyncClientCapabilities `json:"synchronization,omitempty"`
}

// LSPTextDocumentSyncClientCapabilities represents synchronization capabilities.
type LSPTextDocumentSyncClientCapabilities struct {
	WillSave             bool `json:"willSave,omitempty"`
	DidSave              bool `json:"didSave,omitempty"`
	WillSaveWaitUntil    bool `json:"willSaveWaitUntil,omitempty"`
}

// LSPInitializeResult represents the initialize response.
type LSPInitializeResult struct {
	Capabilities LSPServerCapabilities `json:"capabilities"`
}

// LSPServerCapabilities represents server capabilities.
type LSPServerCapabilities struct {
	TextDocumentSync int `json:"textDocumentSync,omitempty"`
}

// LSPManager manages LSP servers.
type LSPManager struct {
	mu        sync.Mutex
	servers   map[string]*LSPServer
	diagnostics []LSPDiagnostic
	enabled   bool
}

// NewLSPManager creates a new LSP manager with full protocol support.
func NewLSPManager(enabled bool) *LSPManager {
	m := &LSPManager{
		servers:     make(map[string]*LSPServer),
		diagnostics: make([]LSPDiagnostic, 0),
		enabled:     enabled,
	}
	if enabled {
		m.registerDefaultServers()
	}
	return m
}

// registerDefaultServers registers default LSP servers.
func (m *LSPManager) registerDefaultServers() {
	m.servers["go"] = &LSPServer{
		Name:       "gopls",
		Command:    "gopls",
		Args:       []string{"serve"},
		Extensions: []string{".go"},
		documents:  make(map[string]*LSPDocument),
		pendingReqs: make(map[int]chan *LSPResponse),
		diagnostics: make(map[string][]LSPDiagnosticFull),
	}
	m.servers["typescript"] = &LSPServer{
		Name:       "typescript",
		Command:    "typescript-language-server",
		Args:       []string{"--stdio"},
		Extensions: []string{".ts", ".tsx", ".js", ".jsx"},
		documents:  make(map[string]*LSPDocument),
		pendingReqs: make(map[int]chan *LSPResponse),
		diagnostics: make(map[string][]LSPDiagnosticFull),
	}
	m.servers["python"] = &LSPServer{
		Name:       "pylsp",
		Command:    "pylsp",
		Args:       []string{},
		Extensions: []string{".py"},
		documents:  make(map[string]*LSPDocument),
		pendingReqs: make(map[int]chan *LSPResponse),
		diagnostics: make(map[string][]LSPDiagnosticFull),
	}
}

// Start starts an LSP server with full protocol handshake.
func (m *LSPManager) Start(language string) error {
	if !m.enabled {
		return nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	server, exists := m.servers[language]
	if !exists {
		return fmt.Errorf("no LSP server for language: %s", language)
	}

	if server.running {
		return nil
	}

	// Check if command exists
	if _, err := exec.LookPath(server.Command); err != nil {
		return fmt.Errorf("LSP server not found: %s", server.Command)
	}

	// Start the server
	server.cmd = exec.Command(server.Command, server.Args...)

	var err error
	server.stdin, err = server.cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("create stdin pipe: %w", err)
	}

	stdout, err := server.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("create stdout pipe: %w", err)
	}
	server.stdout = bufio.NewReader(stdout)

	if err := server.cmd.Start(); err != nil {
		return fmt.Errorf("start LSP server: %w", err)
	}

	server.running = true

	// Start response reader goroutine
	go m.readResponses(server)

	// Send initialize request
	if err := m.initializeServer(server); err != nil {
		m.Stop(language)
		return fmt.Errorf("initialize server: %w", err)
	}

	return nil
}

// initializeServer sends the initialize request to the LSP server.
func (m *LSPManager) initializeServer(server *LSPServer) error {
	params := LSPInitializeParams{
		ProcessID: 0,
		RootURI:   "file:///",
		Capabilities: LSPClientCapabilities{
			TextDocument: &LSPTextDocumentClientCapabilities{
				Synchronization: &LSPTextDocumentSyncClientCapabilities{
					DidSave: true,
				},
			},
		},
	}

	// Send initialize request
	resp, err := m.sendRequestSync(server, "initialize", params, 5*time.Second)
	if err != nil {
		return fmt.Errorf("initialize request: %w", err)
	}

	if resp.Error != nil {
		return fmt.Errorf("initialize error: %s", resp.Error.Message)
	}

	// Parse capabilities
	var result LSPInitializeResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return fmt.Errorf("parse initialize result: %w", err)
	}

	// Send initialized notification
	if err := m.sendNotification(server, "initialized", map[string]any{}); err != nil {
		return fmt.Errorf("send initialized: %w", err)
	}

	server.initialized = true
	return nil
}

// readResponses reads responses from the LSP server.
func (m *LSPManager) readResponses(server *LSPServer) {
	for server.running {
		msg, err := m.readLSPMessage(server)
		if err != nil {
			if server.running {
				// Server disconnected
				server.running = false
			}
			return
		}

		// Parse the message
		var resp LSPResponse
		if err := json.Unmarshal(msg, &resp); err != nil {
			continue
		}

		// Handle response
		if resp.ID != nil {
			// This is a response to a request
			var id int
			json.Unmarshal(resp.ID, &id)

			server.mu.Lock()
			ch, exists := server.pendingReqs[id]
			if exists {
				delete(server.pendingReqs, id)
			}
			server.mu.Unlock()

			if exists {
				ch <- &resp
			}
		} else if resp.Method != "" {
			// This is a notification from the server
			m.handleNotification(server, &resp)
		}
	}
}

// readLSPMessage reads a single LSP message from the server.
func (m *LSPManager) readLSPMessage(server *LSPServer) ([]byte, error) {
	// Read headers
	contentLength := 0
	for {
		line, err := server.stdout.ReadString('\n')
		if err != nil {
			return nil, err
		}

		line = strings.TrimSpace(line)
		if line == "" {
			break
		}

		if strings.HasPrefix(line, "Content-Length: ") {
			lengthStr := strings.TrimPrefix(line, "Content-Length: ")
			contentLength, err = strconv.Atoi(lengthStr)
			if err != nil {
				return nil, fmt.Errorf("parse Content-Length: %w", err)
			}
		}
	}

	if contentLength == 0 {
		return nil, fmt.Errorf("no Content-Length header")
	}

	// Read body
	body := make([]byte, contentLength)
	_, err := io.ReadFull(server.stdout, body)
	if err != nil {
		return nil, err
	}

	return body, nil
}

// handleNotification handles notifications from the LSP server.
func (m *LSPManager) handleNotification(server *LSPServer, resp *LSPResponse) {
	switch resp.Method {
	case "textDocument/publishDiagnostics":
		m.handlePublishDiagnostics(server, resp.Params)
	}
}

// handlePublishDiagnostics handles diagnostic notifications.
func (m *LSPManager) handlePublishDiagnostics(server *LSPServer, params json.RawMessage) {
	var diagParams struct {
		URI         string              `json:"uri"`
		Diagnostics []LSPDiagnosticFull `json:"diagnostics"`
	}

	if err := json.Unmarshal(params, &diagParams); err != nil {
		return
	}

	// Convert URI to file path
	filePath := strings.TrimPrefix(diagParams.URI, "file:///")
	filePath = strings.ReplaceAll(filePath, "/", "\\")

	// Store diagnostics
	server.mu.Lock()
	server.diagnostics[filePath] = diagParams.Diagnostics
	server.mu.Unlock()

	// Update manager diagnostics
	m.mu.Lock()
	// Remove old diagnostics for this file
	var newDiags []LSPDiagnostic
	for _, d := range m.diagnostics {
		if d.File != filePath {
			newDiags = append(newDiags, d)
		}
	}

	// Add new diagnostics
	for _, d := range diagParams.Diagnostics {
		severity := "info"
		switch d.Severity {
		case LSPSeverityError:
			severity = "error"
		case LSPSeverityWarning:
			severity = "warning"
		case LSPSeverityHint:
			severity = "hint"
		}

		newDiags = append(newDiags, LSPDiagnostic{
			File:     filePath,
			Line:     d.Range.Start.Line + 1,
			Column:   d.Range.Start.Character + 1,
			Severity: severity,
			Message:  d.Message,
			Source:   d.Source,
		})
	}

	m.diagnostics = newDiags
	m.mu.Unlock()
}

// sendRequestSync sends a request and waits for response.
func (m *LSPManager) sendRequestSync(server *LSPServer, method string, params any, timeout time.Duration) (*LSPResponse, error) {
	server.mu.Lock()
	if !server.running || server.stdin == nil {
		server.mu.Unlock()
		return nil, fmt.Errorf("server not running")
	}

	server.nextID++
	id := server.nextID

	// Create response channel
	ch := make(chan *LSPResponse, 1)
	server.pendingReqs[id] = ch
	server.mu.Unlock()

	// Send request
	msg := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  method,
		"params":  params,
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return nil, err
	}

	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(data))
	server.stdin.Write([]byte(header))
	server.stdin.Write(data)

	// Wait for response
	select {
	case resp := <-ch:
		return resp, nil
	case <-time.After(timeout):
		server.mu.Lock()
		delete(server.pendingReqs, id)
		server.mu.Unlock()
		return nil, fmt.Errorf("timeout waiting for response")
	}
}

// sendNotification sends a notification to the LSP server.
func (m *LSPManager) sendNotification(server *LSPServer, method string, params any) error {
	server.mu.Lock()
	defer server.mu.Unlock()

	if !server.running || server.stdin == nil {
		return nil
	}

	msg := map[string]any{
		"jsonrpc": "2.0",
		"method":  method,
		"params":  params,
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(data))
	server.stdin.Write([]byte(header))
	server.stdin.Write(data)

	return nil
}

// NotifyFileOpen notifies the LSP of a file open.
func (m *LSPManager) NotifyFileOpen(filePath string, content string) error {
	if !m.enabled {
		return nil
	}

	ext := filepath.Ext(filePath)
	for _, server := range m.servers {
		if m.matchesExtension(ext, server.Extensions) && server.running {
			uri := "file:///" + strings.ReplaceAll(filePath, "\\", "/")

			// Store document
			server.mu.Lock()
			server.documents[filePath] = &LSPDocument{
				URI:        uri,
				LanguageID: m.getLanguageID(ext),
				Version:    1,
				Text:       content,
			}
			server.mu.Unlock()

			// Send didOpen notification
			return m.sendNotification(server, "textDocument/didOpen", LSPTextDocumentItem{
				URI:        uri,
				LanguageID: m.getLanguageID(ext),
				Version:    1,
				Text:       content,
			})
		}
	}

	return nil
}

// NotifyFileChange notifies the LSP of a file change.
func (m *LSPManager) NotifyFileChange(filePath string) error {
	if !m.enabled {
		return nil
	}

	ext := filepath.Ext(filePath)
	for _, server := range m.servers {
		if m.matchesExtension(ext, server.Extensions) && server.running {
			uri := "file:///" + strings.ReplaceAll(filePath, "\\", "/")

			// Send didSave notification
			return m.sendNotification(server, "textDocument/didSave", map[string]any{
				"textDocument": map[string]any{
					"uri": uri,
				},
			})
		}
	}

	return nil
}

// NotifyFileClose notifies the LSP of a file close.
func (m *LSPManager) NotifyFileClose(filePath string) error {
	if !m.enabled {
		return nil
	}

	ext := filepath.Ext(filePath)
	for _, server := range m.servers {
		if m.matchesExtension(ext, server.Extensions) && server.running {
			uri := "file:///" + strings.ReplaceAll(filePath, "\\", "/")

			// Remove document
			server.mu.Lock()
			delete(server.documents, filePath)
			server.mu.Unlock()

			// Send didClose notification
			return m.sendNotification(server, "textDocument/didClose", map[string]any{
				"textDocument": map[string]any{
					"uri": uri,
				},
			})
		}
	}

	return nil
}

// getLanguageID returns the language ID for a file extension.
func (m *LSPManager) getLanguageID(ext string) string {
	switch ext {
	case ".go":
		return "go"
	case ".ts", ".tsx":
		return "typescript"
	case ".js", ".jsx":
		return "javascript"
	case ".py":
		return "python"
	default:
		return "plaintext"
	}
}

// GetDiagnostics returns diagnostics for a file.
func (m *LSPManager) GetDiagnostics(filePath string) []LSPDiagnostic {
	m.mu.Lock()
	defer m.mu.Unlock()

	var result []LSPDiagnostic
	for _, d := range m.diagnostics {
		if d.File == filePath {
			result = append(result, d)
		}
	}
	return result
}

// GetAllDiagnostics returns all diagnostics.
func (m *LSPManager) GetAllDiagnostics() []LSPDiagnostic {
	m.mu.Lock()
	defer m.mu.Unlock()

	result := make([]LSPDiagnostic, len(m.diagnostics))
	copy(result, m.diagnostics)
	return result
}

// ClearDiagnostics clears all diagnostics.
func (m *LSPManager) ClearDiagnostics() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.diagnostics = make([]LSPDiagnostic, 0)
}

// ClearFileDiagnostics clears diagnostics for a specific file.
func (m *LSPManager) ClearFileDiagnostics(filePath string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var result []LSPDiagnostic
	for _, d := range m.diagnostics {
		if d.File != filePath {
			result = append(result, d)
		}
	}
	m.diagnostics = result
}

// Stop stops an LSP server.
func (m *LSPManager) Stop(language string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	server, exists := m.servers[language]
	if !exists || !server.running {
		return
	}

	// Send shutdown request
	if server.initialized {
		m.sendNotification(server, "shutdown", nil)
		m.sendNotification(server, "exit", nil)
	}

	if server.cmd != nil && server.cmd.Process != nil {
		server.cmd.Process.Kill()
	}
	server.running = false
	server.initialized = false
}

// StopAll stops all LSP servers.
func (m *LSPManager) StopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, server := range m.servers {
		if server.running {
			if server.initialized {
				m.sendNotification(server, "shutdown", nil)
				m.sendNotification(server, "exit", nil)
			}
			if server.cmd != nil && server.cmd.Process != nil {
				server.cmd.Process.Kill()
			}
			server.running = false
			server.initialized = false
		}
	}
}

// matchesExtension checks if a file extension matches.
func (m *LSPManager) matchesExtension(ext string, extensions []string) bool {
	for _, e := range extensions {
		if ext == e {
			return true
		}
	}
	return false
}

// IsEnabled returns true if LSP is enabled.
func (m *LSPManager) IsEnabled() bool {
	return m.enabled
}

// SetEnabled enables or disables LSP.
func (m *LSPManager) SetEnabled(enabled bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.enabled = enabled
}

// GetRunningServers returns a list of running LSP servers.
func (m *LSPManager) GetRunningServers() []string {
	m.mu.Lock()
	defer m.mu.Unlock()

	var running []string
	for name, server := range m.servers {
		if server.running {
			running = append(running, name)
		}
	}
	return running
}

// FormatDiagnostics formats diagnostics for display.
func FormatDiagnostics(diagnostics []LSPDiagnostic) string {
	if len(diagnostics) == 0 {
		return "No diagnostics."
	}

	var sb string
	sb += fmt.Sprintf("## Diagnostics (%d issues)\n\n", len(diagnostics))

	byFile := make(map[string][]LSPDiagnostic)
	for _, d := range diagnostics {
		byFile[d.File] = append(byFile[d.File], d)
	}

	for file, fileDiags := range byFile {
		sb += fmt.Sprintf("### %s\n\n", file)
		for _, d := range fileDiags {
			sb += fmt.Sprintf("- **%s** (L%d:%d): %s\n", d.Severity, d.Line, d.Column, d.Message)
		}
		sb += "\n"
	}

	return sb
}

// DiagnosticToHint converts diagnostics to a hint for the LLM.
func DiagnosticToHint(diagnostics []LSPDiagnostic) string {
	if len(diagnostics) == 0 {
		return ""
	}

	errors := 0
	warnings := 0
	for _, d := range diagnostics {
		if d.Severity == "error" {
			errors++
		} else if d.Severity == "warning" {
			warnings++
		}
	}

	if errors > 0 {
		return fmt.Sprintf("LSP found %d errors and %d warnings. Fix errors before continuing.", errors, warnings)
	}
	if warnings > 0 {
		return fmt.Sprintf("LSP found %d warnings. Consider fixing them.", warnings)
	}
	return ""
}
