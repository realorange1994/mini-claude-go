package modelregistry

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewRegistry(t *testing.T) {
	r := NewRegistry()
	models := r.All()
	if len(models) == 0 {
		t.Error("registry should have built-in models")
	}
	// Should have Anthropic models
	found := false
	for _, m := range models {
		if m.ID == "claude-sonnet-4-20250514" {
			found = true
			break
		}
	}
	if !found {
		t.Error("missing claude-sonnet-4-20250514")
	}
}

func TestFindByID(t *testing.T) {
	r := NewRegistry()
	m, ok := r.FindByID("claude-sonnet-4-20250514")
	if !ok {
		t.Fatal("not found")
	}
	if m.Name != "Claude Sonnet 4" {
		t.Errorf("Name = %q, want Claude Sonnet 4", m.Name)
	}
	if m.Provider != "anthropic" {
		t.Errorf("Provider = %q, want anthropic", m.Provider)
	}
}

func TestFindNotFound(t *testing.T) {
	r := NewRegistry()
	_, ok := r.FindByID("nonexistent")
	if ok {
		t.Error("should not find nonexistent model")
	}
}

func TestLoadFromJSON(t *testing.T) {
	r := NewRegistry()

	// Create temp models.json
	tmp := t.TempDir()
	modelsPath := filepath.Join(tmp, "models.json")
	modelsJSON := `{
		"providers": {
			"local": {
				"baseUrl": "http://localhost:11434",
				"models": [
					{"id": "llama3", "name": "Llama 3", "maxTokens": 8192}
				]
			}
		}
	}`
	if err := os.WriteFile(modelsPath, []byte(modelsJSON), 0644); err != nil {
		t.Fatal(err)
	}

	if err := r.LoadFromJSON(modelsPath); err != nil {
		t.Fatalf("LoadFromJSON: %v", err)
	}

	_, ok := r.FindByID("llama3")
	if !ok {
		t.Error("llama3 should be registered")
	}
	// Built-in should still exist
	_, ok = r.FindByID("claude-sonnet-4-20250514")
	if !ok {
		t.Error("claude-sonnet-4-20250514 should still exist")
	}
}

func TestLoadFromJSON_CustomOverride(t *testing.T) {
	r := NewRegistry()

	tmp := t.TempDir()
	modelsPath := filepath.Join(tmp, "models.json")
	modelsJSON := `{
		"providers": {
			"anthropic": {
				"baseUrl": "http://proxy.local/v1",
				"models": [
					{"id": "claude-sonnet-4-20250514", "name": "Sonnet via Proxy", "contextWindow": 100000}
				]
			}
		}
	}`
	if err := os.WriteFile(modelsPath, []byte(modelsJSON), 0644); err != nil {
		t.Fatal(err)
	}

	if err := r.LoadFromJSON(modelsPath); err != nil {
		t.Fatal(err)
	}

	m, ok := r.Find("anthropic", "claude-sonnet-4-20250514")
	if !ok {
		t.Fatal("model not found")
	}
	if m.Name != "Sonnet via Proxy" {
		t.Errorf("Name = %q, want Sonnet via Proxy", m.Name)
	}
	if m.ContextWindow != 100000 {
		t.Errorf("ContextWindow = %d, want 100000", m.ContextWindow)
	}
}

func TestLoadFromJSON_NonExistent(t *testing.T) {
	r := NewRegistry()
	// Non-existent file should not error
	if err := r.LoadFromJSON("/nonexistent/path"); err != nil {
		t.Error("LoadFromJSON should not error for non-existent file")
	}
}

func TestResolveAPIKey_EnvVar(t *testing.T) {
	r := NewRegistry()
	originalKey := os.Getenv("ANTHROPIC_API_KEY")
	defer func() {
		if originalKey != "" {
			os.Setenv("ANTHROPIC_API_KEY", originalKey)
		} else {
			os.Unsetenv("ANTHROPIC_API_KEY")
		}
	}()

	os.Setenv("ANTHROPIC_API_KEY", "test-key-123")

	m, _ := r.FindByID("claude-sonnet-4-20250514")
	key, err := r.ResolveAPIKey(m)
	if err != nil {
		t.Fatalf("ResolveAPIKey: %v", err)
	}
	if key != "test-key-123" {
		t.Errorf("key = %q, want test-key-123", key)
	}
}

func TestResolveAPIKey_ModelKey(t *testing.T) {
	r := NewRegistry()
	m := ModelInfo{
		ID:       "my-model",
		Provider: "custom",
		APIKey:   "direct-key",
	}
	key, err := r.ResolveAPIKey(m)
	if err != nil {
		t.Fatal(err)
	}
	if key != "direct-key" {
		t.Errorf("key = %q, want direct-key", key)
	}
}

func TestRegisterUnregister(t *testing.T) {
	r := NewRegistry()
	r.Register(ModelInfo{
		ID: "new-model", Provider: "test", Name: "New Model",
		BaseURL: "http://test.com", MaxTokens: 4096, ContextWindow: 8000,
	})

	_, ok := r.FindByID("new-model")
	if !ok {
		t.Error("new-model should be registered")
	}

	r.Unregister("test", "new-model")
	_, ok = r.FindByID("new-model")
	if ok {
		t.Error("new-model should be unregistered")
	}
}

func TestStripComments(t *testing.T) {
	input := `{
		"providers": {
			// This is a comment
			"anthropic": {
				"baseUrl": "http://api.com" // inline comment
			}
		}
	}`
	got := stripComments(input)
	if len(got) == 0 {
		t.Error("result should not be empty")
	}
	// Should not contain "This is a comment"
	if containsStr(got, "// This is a comment") {
		t.Error("comment should be removed")
	}
	// Should still contain "baseUrl"
	if !containsStr(got, "baseUrl") {
		t.Error("baseUrl should still be present")
	}
}

func TestAvailable_WithAuth(t *testing.T) {
	r := NewRegistry()
	originalKey := os.Getenv("ANTHROPIC_API_KEY")
	defer func() {
		if originalKey != "" {
			os.Setenv("ANTHROPIC_API_KEY", originalKey)
		} else {
			os.Unsetenv("ANTHROPIC_API_KEY")
		}
	}()

	os.Setenv("ANTHROPIC_API_KEY", "some-key")
	available := r.Available()
	if len(available) == 0 {
		t.Error("should have available models when ANTHROPIC_API_KEY is set")
	}
}

func containsStr(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
