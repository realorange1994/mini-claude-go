package agent

import (
	"miniclaudecode-go/pkg/core/extensions"
	"testing"
)

func TestParseToolCalls(t *testing.T) {
	tests := []struct {
		name     string
		response string
		wantLen  int
	}{
		{
			name:     "empty response",
			response: "",
			wantLen:  0,
		},
		{
			name:     "plain text - no tools",
			response: "I'll help you with that task.",
			wantLen:  0,
		},
		{
			name:     "tool_use in response content",
			response: `{"content":[{"type":"tool_use","id":"tc_1","name":"Read","input":{"path":"test.txt"}}]}`,
			wantLen:  1,
		},
		{
			name:     "multiple tool_use blocks",
			response: `{"content":[{"type":"tool_use","id":"tc_1","name":"Read","input":{"path":"a.txt"}},{"type":"tool_use","id":"tc_2","name":"Read","input":{"path":"b.txt"}}]}`,
			wantLen:  2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &AgentSession{}
			calls, hasTools := s.parseToolCalls(tt.response)
			if hasTools && len(calls) != tt.wantLen {
				t.Errorf("parseToolCalls returned %d calls, want %d", len(calls), tt.wantLen)
			}
			if !hasTools && tt.wantLen > 0 {
				t.Errorf("parseToolCalls returned hasTools=false, want %d tools", tt.wantLen)
			}
		})
	}
}

func TestBuildMessages(t *testing.T) {
	s := &AgentSession{}
	s.messages = []extensions.Message{
		{Role: extensions.RoleUser, Content: []extensions.ContentBlock{extensions.TextContentBlock("Hello")}},
		{Role: extensions.RoleAssistant, Content: []extensions.ContentBlock{extensions.TextContentBlock("Hi there!")}},
	}

	result := s.buildMessages(s.messages)
	if len(result) != 2 {
		t.Fatalf("buildMessages returned %d messages, want 2", len(result))
	}

	if result[0]["role"] != "user" {
		t.Errorf("first message role = %v, want user", result[0]["role"])
	}
	if result[1]["role"] != "assistant" {
		t.Errorf("second message role = %v, want assistant", result[1]["role"])
	}
}
