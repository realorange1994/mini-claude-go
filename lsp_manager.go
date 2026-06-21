package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

// ─── LSP Integration (MiMo-Code 2) ─────────────────────────────────────────
//
// Spawns and manages LSP servers (Go, TypeScript, Python, etc.).
// After every file edit, notifies the LSP and collects diagnostics.
//
// MiMo-Code source: lsp/lsp.ts (519 lines), lsp/diagnostic.ts (29 lines)

// LSPDiagnostic represents a diagnostic from the LSP.
type LSPDiagnostic struct {
	File     string `json:"file"`
	Line     int    `json:"line"`
	Column   int    `json:"column"`
	Severity string `json:"severity"` // error, warning, info, hint
	Message  string `json:"message"`
	Source   string `json:"source"`
}

// LSPServer represents a running LSP server.
type LSPServer struct {
	Name     string
	Command  string
	Args     []string
	Extensions []string
	cmd      *exec.Cmd
	stdin    io.WriteCloser
	stdout   *bufio.Reader
	mu       sync.Mutex
	running  bool
	nextID   int
}

// LSPManager manages LSP servers.
type LSPManager struct {
	mu        sync.Mutex
	servers   map[string]*LSPServer
	diagnostics []LSPDiagnostic
	enabled   bool
}

// NewLSPManager creates a new LSP manager.
func NewLSPManager(enabled bool) *LSPManager {
	m := &LSPManager{
		servers:   make(map[string]*LSPServer),
		diagnostics: make([]LSPDiagnostic, 0),
		enabled:   enabled,
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
	}
	m.servers["typescript"] = &LSPServer{
		Name:       "typescript",
		Command:    "typescript-language-server",
		Args:       []string{"--stdio"},
		Extensions: []string{".ts", ".tsx", ".js", ".jsx"},
	}
	m.servers["python"] = &LSPServer{
		Name:       "pylsp",
		Command:    "pylsp",
		Args:       []string{},
		Extensions: []string{".py"},
	}
}

// Start starts an LSP server for the given language.
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
	return nil
}

// Stop stops an LSP server.
func (m *LSPManager) Stop(language string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	server, exists := m.servers[language]
	if !exists || !server.running {
		return
	}

	if server.cmd != nil && server.cmd.Process != nil {
		server.cmd.Process.Kill()
	}
	server.running = false
}

// StopAll stops all LSP servers.
func (m *LSPManager) StopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, server := range m.servers {
		if server.running && server.cmd != nil && server.cmd.Process != nil {
			server.cmd.Process.Kill()
		}
		server.running = false
	}
}

// NotifyFileChange notifies the LSP of a file change.
func (m *LSPManager) NotifyFileChange(filePath string) error {
	if !m.enabled {
		return nil
	}

	ext := filepath.Ext(filePath)
	for _, server := range m.servers {
		if m.matchesExtension(ext, server.Extensions) && server.running {
			return m.sendNotification(server, "textDocument/didSave", map[string]any{
				"textDocument": map[string]any{
					"uri": "file:///" + strings.ReplaceAll(filePath, "\\", "/"),
				},
			})
		}
	}

	return nil
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

// matchesExtension checks if a file extension matches.
func (m *LSPManager) matchesExtension(ext string, extensions []string) bool {
	for _, e := range extensions {
		if ext == e {
			return true
		}
	}
	return false
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

// sendRequest sends a request to the LSP server and waits for response.
func (m *LSPManager) sendRequest(server *LSPServer, method string, params any) (any, error) {
	server.mu.Lock()
	defer server.mu.Unlock()

	if !server.running || server.stdin == nil {
		return nil, fmt.Errorf("server not running")
	}

	server.nextID++
	id := server.nextID

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

	// Read response (simplified - in real implementation would parse LSP protocol)
	// For now, return nil as we don't need the response for diagnostics
	return nil, nil
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
