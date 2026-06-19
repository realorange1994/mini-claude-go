package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

// ─── Mock LLM Server (MiMo-Code pattern) ───────────────────────────────────

// MockLLMServer simulates an LLM API for testing without real API calls.
// MiMo-Code pattern: in-process mock that emits pre-canned responses.
type MockLLMServer struct {
	mu        sync.Mutex
	server    *httptest.Server
	responses []MockResponse
	captured  []MockLLMRequest
	callCount int
}

// MockResponse represents a pre-canned LLM response.
type MockResponse struct {
	Text       string
	ToolCalls  []MockToolCall
	Err        error
	Delay      int // milliseconds
}

// MockToolCall represents a tool call in a mock response.
type MockToolCall struct {
	ID   string
	Name string
	Args map[string]any
}

// MockLLMRequest represents a captured API request.
type MockLLMRequest struct {
	Body    map[string]any
	Headers http.Header
}

// NewMockLLMServer creates a new mock LLM server.
func NewMockLLMServer() *MockLLMServer {
	m := &MockLLMServer{}
	m.server = httptest.NewServer(http.HandlerFunc(m.handler))
	return m
}

// URL returns the mock server URL.
func (m *MockLLMServer) URL() string {
	return m.server.URL
}

// Push adds a response to the queue.
func (m *MockLLMServer) Push(text string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.responses = append(m.responses, MockResponse{Text: text})
}

// PushToolCall adds a tool call response to the queue.
func (m *MockLLMServer) PushToolCall(name string, args map[string]any) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.responses = append(m.responses, MockResponse{
		ToolCalls: []MockToolCall{{ID: "mock-" + name, Name: name, Args: args}},
	})
}

// PushError adds an error response to the queue.
func (m *MockLLMServer) PushError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.responses = append(m.responses, MockResponse{Err: err})
}

// Captured returns all captured requests.
func (m *MockLLMServer) Captured() []MockLLMRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]MockLLMRequest, len(m.captured))
	copy(result, m.captured)
	return result
}

// CallCount returns the number of API calls made.
func (m *MockLLMServer) CallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.callCount
}

// Close shuts down the mock server.
func (m *MockLLMServer) Close() {
	m.server.Close()
}

// handler processes HTTP requests.
func (m *MockLLMServer) handler(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	m.callCount++

	// Capture request
	var body map[string]any
	if r.Body != nil {
		json.NewDecoder(r.Body).Decode(&body)
	}
	m.captured = append(m.captured, MockLLMRequest{Body: body, Headers: r.Header})

	// Get next response
	if len(m.responses) == 0 {
		m.mu.Unlock()
		http.Error(w, "no more responses", 500)
		return
	}
	resp := m.responses[0]
	m.responses = m.responses[1:]
	m.mu.Unlock()

	// Return error if configured
	if resp.Err != nil {
		http.Error(w, resp.Err.Error(), 500)
		return
	}

	// Build SSE response
	w.Header().Set("Content-Type", "text/event-stream")
	w.WriteHeader(200)

	// Send message_start
	fmt.Fprintf(w, "event: message_start\ndata: %s\n\n", `{"type":"message_start","message":{"id":"mock-msg","type":"message","role":"assistant"}}`)

	// Send content blocks
	if resp.Text != "" {
		fmt.Fprintf(w, "event: content_block_start\ndata: %s\n\n", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`)
		fmt.Fprintf(w, "event: content_block_delta\ndata: %s\n\n", fmt.Sprintf(`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"%s"}}`, resp.Text))
		fmt.Fprintf(w, "event: content_block_stop\ndata: %s\n\n", `{"type":"content_block_stop","index":0}`)
	}

	for i, tc := range resp.ToolCalls {
		toolJSON, _ := json.Marshal(tc.Args)
		fmt.Fprintf(w, "event: content_block_start\ndata: %s\n\n", fmt.Sprintf(`{"type":"content_block_start","index":%d,"content_block":{"type":"tool_use","id":"%s","name":"%s","input":{}}}`, i+1, tc.ID, tc.Name))
		fmt.Fprintf(w, "event: content_block_delta\ndata: %s\n\n", fmt.Sprintf(`{"type":"content_block_delta","index":%d,"delta":{"type":"input_json_delta","partial_json":"%s"}}`, i+1, string(toolJSON)))
		fmt.Fprintf(w, "event: content_block_stop\ndata: %s\n\n", fmt.Sprintf(`{"type":"content_block_stop","index":%d}`, i+1))
	}

	// Send message_delta with stop_reason
	fmt.Fprintf(w, "event: message_delta\ndata: %s\n\n", `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":10}}`)
	fmt.Fprintf(w, "event: message_stop\ndata: %s\n\n", `{"type":"message_stop"}`)

	// Flush
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}

// ─── Test Fixtures (MiMo-Code pattern) ─────────────────────────────────────

// TestFixture provides common test setup and teardown.
type TestFixture struct {
	t       *testing.T
	TempDir string
}

// NewTestFixture creates a new test fixture with temp directory.
func NewTestFixture(t *testing.T) *TestFixture {
	return &TestFixture{
		t:       t,
		TempDir: t.TempDir(),
	}
}

// CreateConfig creates a test config with the fixture's temp directory.
func (f *TestFixture) CreateConfig() Config {
	cfg := DefaultConfig()
	cfg.ProjectDir = f.TempDir
	return cfg
}

// CreateSessionMemory creates a test session memory.
func (f *TestFixture) CreateSessionMemory() *SessionMemory {
	return NewSessionMemory(f.TempDir)
}

// CreateWorkTaskStore creates a test work task store.
func (f *TestFixture) CreateWorkTaskStore() *WorkTaskStore {
	return NewWorkTaskStore(f.TempDir)
}

// CreateMockLLM creates a mock LLM server.
func (f *TestFixture) CreateMockLLM() *MockLLMServer {
	return NewMockLLMServer()
}

// ─── Test Helpers (MiMo-Code pattern) ──────────────────────────────────────

// assertNoError fails the test if err is not nil.
func assertNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

// assertError fails the test if err is nil.
func assertError(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// assertEqual fails if expected != actual.
func assertEqual(t *testing.T, expected, actual interface{}) {
	t.Helper()
	if expected != actual {
		t.Fatalf("expected %v, got %v", expected, actual)
	}
}

// assertContains fails if s does not contain substr.
func assertContains(t *testing.T, s, substr string) {
	t.Helper()
	if !stringsContains(s, substr) {
		t.Fatalf("expected %q to contain %q", s, substr)
	}
}

// stringsContains checks if s contains substr.
func stringsContains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStrHelper(s, substr))
}

func containsStrHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
