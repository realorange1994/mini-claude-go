package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestMockLLMServer_TextResponse(t *testing.T) {
	mock := NewMockLLMServer()
	defer mock.Close()

	mock.Push("Hello, world!")

	// Verify URL is set
	if mock.URL() == "" {
		t.Error("expected non-empty URL")
	}

	// Verify call count is 0
	if mock.CallCount() != 0 {
		t.Errorf("expected 0 calls, got %d", mock.CallCount())
	}
}

func TestMockLLMServer_ToolCallResponse(t *testing.T) {
	mock := NewMockLLMServer()
	defer mock.Close()

	mock.PushToolCall("read_file", map[string]any{"path": "main.go"})

	// Verify response is queued
	if len(mock.responses) != 1 {
		t.Errorf("expected 1 response, got %d", len(mock.responses))
	}
}

func TestMockLLMServer_ErrorResponse(t *testing.T) {
	mock := NewMockLLMServer()
	defer mock.Close()

	mock.PushError(fmt.Errorf("rate limit exceeded"))

	if len(mock.responses) != 1 {
		t.Errorf("expected 1 response, got %d", len(mock.responses))
	}
}

func TestMockLLMServer_MultipleResponses(t *testing.T) {
	mock := NewMockLLMServer()
	defer mock.Close()

	mock.Push("First response")
	mock.Push("Second response")
	mock.PushToolCall("exec", map[string]any{"command": "ls"})

	if len(mock.responses) != 3 {
		t.Errorf("expected 3 responses, got %d", len(mock.responses))
	}
}

func TestMockLLMServer_CaptureRequests(t *testing.T) {
	mock := NewMockLLMServer()
	defer mock.Close()

	mock.Push("test")

	// Make a request to the mock server
	body := map[string]any{"model": "test", "messages": []map[string]string{{"role": "user", "content": "hi"}}}
	bodyBytes, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", mock.URL()+"/v1/messages", strings.NewReader(string(bodyBytes)))
	req.Header.Set("Content-Type", "application/json")
	http.DefaultClient.Do(req)

	// Verify captured request
	captured := mock.Captured()
	if len(captured) != 1 {
		t.Errorf("expected 1 captured request, got %d", len(captured))
	}
	if mock.CallCount() != 1 {
		t.Errorf("expected 1 call, got %d", mock.CallCount())
	}
}

func TestTestFixture_CreateConfig(t *testing.T) {
	fixture := NewTestFixture(t)
	cfg := fixture.CreateConfig()

	if cfg.ProjectDir != fixture.TempDir {
		t.Errorf("expected ProjectDir %q, got %q", fixture.TempDir, cfg.ProjectDir)
	}
}

func TestTestFixture_CreateSessionMemory(t *testing.T) {
	fixture := NewTestFixture(t)
	sm := fixture.CreateSessionMemory()

	if sm == nil {
		t.Fatal("expected non-nil SessionMemory")
	}
}

func TestTestFixture_CreateWorkTaskStore(t *testing.T) {
	fixture := NewTestFixture(t)
	store := fixture.CreateWorkTaskStore()

	if store == nil {
		t.Fatal("expected non-nil WorkTaskStore")
	}
}

func TestAssertNoError(t *testing.T) {
	assertNoError(t, nil) // should not fail
}

func TestAssertError(t *testing.T) {
	assertError(t, fmt.Errorf("test error")) // should not fail
}

func TestAssertEqual(t *testing.T) {
	assertEqual(t, 42, 42)
	assertEqual(t, "hello", "hello")
}

func TestAssertContains(t *testing.T) {
	assertContains(t, "hello world", "world")
	assertContains(t, "hello world", "hello")
}
