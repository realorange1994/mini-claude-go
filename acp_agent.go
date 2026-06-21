package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"time"
)

// ─── ACP IDE Integration (MiMo-Code 6) ─────────────────────────────────────
//
// Implements the Agent Communication Protocol (ACP) for IDE integration.
// Enables IDEs like VS Code to communicate with the agent.
//
// MiMo-Code source: acp/agent.ts (1550+ lines)

// ACPMethod represents an ACP method.
type ACPMethod string

const (
	ACPInitialize     ACPMethod = "initialize"
	ACPNewSession     ACPMethod = "newSession"
	ACPLoadSession    ACPMethod = "loadSession"
	ACPListSessions   ACPMethod = "listSessions"
	ACPForkSession    ACPMethod = "forkSession"
	ACPResumeSession  ACPMethod = "resumeSession"
	ACPPrompt         ACPMethod = "prompt"
	ACPCancel         ACPMethod = "cancel"
	ACPSetModel       ACPMethod = "setSessionModel"
	ACPSetMode        ACPMethod = "setSessionMode"
)

// ACPEvent represents an ACP event type.
type ACPEvent string

const (
	ACPEventToolCall    ACPEvent = "toolCall"
	ACPEventToolResult  ACPEvent = "toolResult"
	ACPEventText        ACPEvent = "text"
	ACPEventReasoning   ACPEvent = "reasoning"
	ACPEventPermission  ACPEvent = "permission"
	ACPEventDone        ACPEvent = "done"
	ACPEventError       ACPEvent = "error"
)

// ACPRequest represents an ACP request.
type ACPRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Method  ACPMethod       `json:"method"`
	Params  json.RawMessage `json:"params"`
}

// ACPResponse represents an ACP response.
type ACPResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      int         `json:"id"`
	Result  any         `json:"result,omitempty"`
	Error   *ACPError   `json:"error,omitempty"`
}

// ACPError represents an ACP error.
type ACPError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// ACPNotification represents an ACP notification.
type ACPNotification struct {
	JSONRPC string      `json:"jsonrpc"`
	Method  ACPEvent    `json:"method"`
	Params  any         `json:"params"`
}

// ACPSession represents an ACP session.
type ACPSession struct {
	ID        string    `json:"id"`
	Model     string    `json:"model"`
	Mode      string    `json:"mode"`
	CreatedAt time.Time `json:"created_at"`
}

// ACPAgent implements the ACP protocol.
type ACPAgent struct {
	mu        sync.Mutex
	sessions  map[string]*ACPSession
	agentLoop *AgentLoop
	writer    io.Writer
	reader    *bufio.Reader
	nextID    int
	running   bool
}

// NewACPAgent creates a new ACP agent.
func NewACPAgent(agentLoop *AgentLoop) *ACPAgent {
	return &ACPAgent{
		sessions:  make(map[string]*ACPSession),
		agentLoop: agentLoop,
	}
}

// Serve starts serving ACP requests over stdio.
func (a *ACPAgent) Serve() error {
	a.mu.Lock()
	a.running = true
	a.reader = bufio.NewReader(os.Stdin)
	a.writer = os.Stdout
	a.mu.Unlock()

	for a.isRunning() {
		// Read request
		line, err := a.reader.ReadBytes('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			return fmt.Errorf("read request: %w", err)
		}

		// Parse request
		var req ACPRequest
		if err := json.Unmarshal(line, &req); err != nil {
			a.sendError(0, -32700, "Parse error")
			continue
		}

		// Handle request
		a.handleRequest(&req)
	}

	return nil
}

// ServeHTTP starts serving ACP requests over HTTP.
func (a *ACPAgent) ServeHTTP(addr string) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/acp", a.handleHTTP)

	return http.ListenAndServe(addr, mux)
}

// handleHTTP handles HTTP ACP requests.
func (a *ACPAgent) handleHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ACPRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Handle request
	result, err := a.executeMethod(&req)
	if err != nil {
		resp := ACPResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &ACPError{Code: -32000, Message: err.Error()},
		}
		json.NewEncoder(w).Encode(resp)
		return
	}

	resp := ACPResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  result,
	}
	json.NewEncoder(w).Encode(resp)
}

// handleRequest handles a single ACP request.
func (a *ACPAgent) handleRequest(req *ACPRequest) {
	result, err := a.executeMethod(req)
	if err != nil {
		a.sendError(req.ID, -32000, err.Error())
		return
	}

	a.sendResult(req.ID, result)
}

// executeMethod executes an ACP method.
func (a *ACPAgent) executeMethod(req *ACPRequest) (any, error) {
	switch req.Method {
	case ACPInitialize:
		return a.handleInitialize(req)
	case ACPNewSession:
		return a.handleNewSession(req)
	case ACPLoadSession:
		return a.handleLoadSession(req)
	case ACPListSessions:
		return a.handleListSessions(req)
	case ACPForkSession:
		return a.handleForkSession(req)
	case ACPResumeSession:
		return a.handleResumeSession(req)
	case ACPPrompt:
		return a.handlePrompt(req)
	case ACPCancel:
		return a.handleCancel(req)
	case ACPSetModel:
		return a.handleSetModel(req)
	case ACPSetMode:
		return a.handleSetMode(req)
	default:
		return nil, fmt.Errorf("unknown method: %s", req.Method)
	}
}

// handleInitialize handles the initialize method.
func (a *ACPAgent) handleInitialize(req *ACPRequest) (any, error) {
	return map[string]any{
		"serverName":    "miniclaudecode",
		"serverVersion": "1.0.0",
		"capabilities": map[string]any{
			"tools":      true,
			"reasoning":  true,
			"streaming":  true,
			"sessions":   true,
		},
	}, nil
}

// handleNewSession handles the newSession method.
func (a *ACPAgent) handleNewSession(req *ACPRequest) (any, error) {
	var params struct {
		Model string `json:"model"`
		Mode  string `json:"mode"`
	}
	json.Unmarshal(req.Params, &params)

	session := &ACPSession{
		ID:        fmt.Sprintf("session-%s", time.Now().Format("20060102-150405")),
		Model:     params.Model,
		Mode:      params.Mode,
		CreatedAt: time.Now(),
	}

	a.mu.Lock()
	a.sessions[session.ID] = session
	a.mu.Unlock()

	return session, nil
}

// handleLoadSession handles the loadSession method.
func (a *ACPAgent) handleLoadSession(req *ACPRequest) (any, error) {
	var params struct {
		SessionID string `json:"sessionId"`
	}
	json.Unmarshal(req.Params, &params)

	a.mu.Lock()
	session, exists := a.sessions[params.SessionID]
	a.mu.Unlock()

	if !exists {
		return nil, fmt.Errorf("session not found: %s", params.SessionID)
	}

	return session, nil
}

// handleListSessions handles the listSessions method.
func (a *ACPAgent) handleListSessions(req *ACPRequest) (any, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	var sessions []*ACPSession
	for _, s := range a.sessions {
		sessions = append(sessions, s)
	}

	return sessions, nil
}

// handleForkSession handles the forkSession method.
func (a *ACPAgent) handleForkSession(req *ACPRequest) (any, error) {
	var params struct {
		SessionID string `json:"sessionId"`
	}
	json.Unmarshal(req.Params, &params)

	a.mu.Lock()
	original, exists := a.sessions[params.SessionID]
	a.mu.Unlock()

	if !exists {
		return nil, fmt.Errorf("session not found: %s", params.SessionID)
	}

	forked := &ACPSession{
		ID:        fmt.Sprintf("session-%s", time.Now().Format("20060102-150405")),
		Model:     original.Model,
		Mode:      original.Mode,
		CreatedAt: time.Now(),
	}

	a.mu.Lock()
	a.sessions[forked.ID] = forked
	a.mu.Unlock()

	return forked, nil
}

// handleResumeSession handles the resumeSession method.
func (a *ACPAgent) handleResumeSession(req *ACPRequest) (any, error) {
	var params struct {
		SessionID string `json:"sessionId"`
	}
	json.Unmarshal(req.Params, &params)

	a.mu.Lock()
	session, exists := a.sessions[params.SessionID]
	a.mu.Unlock()

	if !exists {
		return nil, fmt.Errorf("session not found: %s", params.SessionID)
	}

	return session, nil
}

// handlePrompt handles the prompt method.
func (a *ACPAgent) handlePrompt(req *ACPRequest) (any, error) {
	var params struct {
		SessionID string `json:"sessionId"`
		Message   string `json:"message"`
	}
	json.Unmarshal(req.Params, &params)

	// Send notification that we're processing
	a.sendNotification(ACPEventText, map[string]any{
		"sessionId": params.SessionID,
		"text":      "Processing...",
	})

	// Process message through agent loop if available
	if a.agentLoop != nil {
		// Run agent loop with the message
		result := a.agentLoop.Run(params.Message)

		// Send text result
		a.sendNotification(ACPEventText, map[string]any{
			"sessionId": params.SessionID,
			"text":      result,
		})
	}

	// Send done notification
	a.sendNotification(ACPEventDone, map[string]any{
		"sessionId": params.SessionID,
	})

	return map[string]any{"status": "ok"}, nil
}

// handleCancel handles the cancel method.
func (a *ACPAgent) handleCancel(req *ACPRequest) (any, error) {
	return map[string]any{"status": "cancelled"}, nil
}

// handleSetModel handles the setSessionModel method.
func (a *ACPAgent) handleSetModel(req *ACPRequest) (any, error) {
	var params struct {
		SessionID string `json:"sessionId"`
		Model     string `json:"model"`
	}
	json.Unmarshal(req.Params, &params)

	a.mu.Lock()
	session, exists := a.sessions[params.SessionID]
	a.mu.Unlock()

	if !exists {
		return nil, fmt.Errorf("session not found: %s", params.SessionID)
	}

	session.Model = params.Model
	return map[string]any{"status": "ok"}, nil
}

// handleSetMode handles the setSessionMode method.
func (a *ACPAgent) handleSetMode(req *ACPRequest) (any, error) {
	var params struct {
		SessionID string `json:"sessionId"`
		Mode      string `json:"mode"`
	}
	json.Unmarshal(req.Params, &params)

	a.mu.Lock()
	session, exists := a.sessions[params.SessionID]
	a.mu.Unlock()

	if !exists {
		return nil, fmt.Errorf("session not found: %s", params.SessionID)
	}

	session.Mode = params.Mode
	return map[string]any{"status": "ok"}, nil
}

// sendResult sends a successful result.
func (a *ACPAgent) sendResult(id int, result any) {
	resp := ACPResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}
	a.writeResponse(resp)
}

// sendError sends an error response.
func (a *ACPAgent) sendError(id int, code int, message string) {
	resp := ACPResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &ACPError{Code: code, Message: message},
	}
	a.writeResponse(resp)
}

// sendNotification sends a notification.
func (a *ACPAgent) sendNotification(method ACPEvent, params any) {
	notif := ACPNotification{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	}
	data, _ := json.Marshal(notif)
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.writer != nil {
		a.writer.Write(data)
		a.writer.Write([]byte("\n"))
	}
}

// writeResponse writes a response to the output.
func (a *ACPAgent) writeResponse(resp ACPResponse) {
	data, _ := json.Marshal(resp)
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.writer != nil {
		a.writer.Write(data)
		a.writer.Write([]byte("\n"))
	}
}

// isRunning returns true if the agent is running.
func (a *ACPAgent) isRunning() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.running
}

// Stop stops the ACP agent.
func (a *ACPAgent) Stop() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.running = false
}
