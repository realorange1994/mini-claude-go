package permissions

import (
	"testing"
)

func TestMatchWildcard_ExactMatch(t *testing.T) {
	if !MatchWildcard("bash", "bash") {
		t.Error("expected exact match")
	}
}

func TestMatchWildcard_AnyWildcard(t *testing.T) {
	if !MatchWildcard("*", "anything") {
		t.Error("expected * to match anything")
	}
}

func TestMatchWildcard_PatternMatch(t *testing.T) {
	tests := []struct {
		pattern string
		s       string
		want    bool
	}{
		{"git *", "git checkout main", true},
		{"git *", "git", true},
		{"git *", "git status", true},
		{"read_file", "read_file", true},
		{"read_file", "write_file", false},
		{"*.go", "main.go", true},
		{"*.go", "main.js", false},
		{"test?.txt", "test1.txt", true},
		{"test?.txt", "test.txt", false},
	}

	for _, tt := range tests {
		got := MatchWildcard(tt.pattern, tt.s)
		if got != tt.want {
			t.Errorf("MatchWildcard(%q, %q) = %v, want %v", tt.pattern, tt.s, got, tt.want)
		}
	}
}

func TestMatchPermission(t *testing.T) {
	tests := []struct {
		toolName  string
		resource  string
		ruleTool  string
		ruleRes   string
		want      bool
	}{
		{"bash", "git status", "bash", "git *", true},
		{"bash", "rm -rf /", "bash", "git *", false},
		{"read_file", "main.go", "read_file", "*.go", true},
		{"read_file", "main.go", "*", "*", true},
	}

	for _, tt := range tests {
		got := MatchPermission(tt.toolName, tt.resource, tt.ruleTool, tt.ruleRes)
		if got != tt.want {
			t.Errorf("MatchPermission(%q, %q, %q, %q) = %v, want %v",
				tt.toolName, tt.resource, tt.ruleTool, tt.ruleRes, got, tt.want)
		}
	}
}

func TestEvaluatePermission_LastMatchWins(t *testing.T) {
	rules := []PermissionRule{
		{Tool: "*", Resource: "*", Action: "deny"},
		{Tool: "bash", Resource: "git *", Action: "allow"},
	}

	// Should match the last rule (allow)
	action := EvaluatePermission("bash", "git status", rules)
	if action != "allow" {
		t.Errorf("expected 'allow', got '%s'", action)
	}

	// Should match the first rule (deny) since bash rm doesn't match git *
	action = EvaluatePermission("bash", "rm -rf /", rules)
	if action != "deny" {
		t.Errorf("expected 'deny', got '%s'", action)
	}
}

func TestEvaluatePermission_DefaultAsk(t *testing.T) {
	rules := []PermissionRule{
		{Tool: "bash", Resource: "git *", Action: "allow"},
	}

	action := EvaluatePermission("read_file", "main.go", rules)
	if action != "ask" {
		t.Errorf("expected 'ask' (default), got '%s'", action)
	}
}

func TestIsToolDisabled(t *testing.T) {
	rules := []PermissionRule{
		{Tool: "bash", Resource: "*", Action: "deny"},
		{Tool: "read_file", Resource: "*", Action: "allow"},
	}

	if !IsToolDisabled("bash", rules) {
		t.Error("expected bash to be disabled")
	}
	if IsToolDisabled("read_file", rules) {
		t.Error("expected read_file to not be disabled")
	}
	if IsToolDisabled("unknown", rules) {
		t.Error("expected unknown to not be disabled")
	}
}

func TestIsInEditGroup(t *testing.T) {
	if !IsInEditGroup("write_file") {
		t.Error("expected write_file in edit group")
	}
	if !IsInEditGroup("edit_file") {
		t.Error("expected edit_file in edit group")
	}
	if IsInEditGroup("read_file") {
		t.Error("expected read_file to not be in edit group")
	}
}

func TestFilterDisabledTools(t *testing.T) {
	tools := []string{"bash", "read_file", "write_file", "edit_file"}
	rules := []PermissionRule{
		{Tool: "bash", Resource: "*", Action: "deny"},
	}

	filtered := FilterDisabledTools(tools, rules)
	if len(filtered) != 3 {
		t.Errorf("expected 3 tools, got %d", len(filtered))
	}
	for _, t2 := range filtered {
		if t2 == "bash" {
			t.Error("bash should be filtered out")
		}
	}
}

func TestIsMemoryPath(t *testing.T) {
	projectDir := t.TempDir()

	tests := []struct {
		path string
		want bool
	}{
		{".claude/memory/global.md", true},
		{".claude/session_memory.md", true},
		{"main.go", false},
		{"src/auth.go", false},
	}

	for _, tt := range tests {
		got := IsMemoryPath(tt.path, projectDir)
		if got != tt.want {
			t.Errorf("IsMemoryPath(%q, %q) = %v, want %v", tt.path, projectDir, got, tt.want)
		}
	}
}

func TestIsCheckpointWriterAllowed(t *testing.T) {
	projectDir := t.TempDir()

	tests := []struct {
		path    string
		allowed bool
	}{
		{".claude/memory/global.md", true},
		{".claude/memory/project.md", true},
		{".claude/session_memory.md", true},
		{".claude/memory/evil.txt", false}, // not .md
		{"main.go", false},                 // not memory path
	}

	for _, tt := range tests {
		got := IsCheckpointWriterAllowed(tt.path, projectDir)
		if got != tt.allowed {
			t.Errorf("IsCheckpointWriterAllowed(%q, %q) = %v, want %v", tt.path, projectDir, got, tt.allowed)
		}
	}
}
