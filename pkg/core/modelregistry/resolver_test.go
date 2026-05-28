package modelregistry

import "testing"

func TestResolve_ExactID(t *testing.T) {
	r := NewRegistry()
	m, err := r.Resolve("claude-sonnet-4-20250514")
	if err != nil {
		t.Fatal(err)
	}
	if m.ID != "claude-sonnet-4-20250514" {
		t.Errorf("ID = %q", m.ID)
	}
}

func TestResolve_Alias(t *testing.T) {
	r := NewRegistry()
	m, err := r.Resolve("sonnet")
	if err != nil {
		t.Fatal(err)
	}
	if m.ID != "claude-sonnet-4-20250514" {
		t.Errorf("ID = %q, want claude-sonnet-4-20250514", m.ID)
	}
}

func TestResolve_OpusAlias(t *testing.T) {
	r := NewRegistry()
	m, err := r.Resolve("opus")
	if err != nil {
		t.Fatal(err)
	}
	if m.ID != "claude-opus-4-20250514" {
		t.Errorf("ID = %q, want claude-opus-4-20250514", m.ID)
	}
}

func TestResolve_HaikuAlias(t *testing.T) {
	r := NewRegistry()
	m, err := r.Resolve("haiku")
	if err != nil {
		t.Fatal(err)
	}
	if m.ID != "claude-haiku-3-5-20241022" {
		t.Errorf("ID = %q, want claude-haiku-3-5-20241022", m.ID)
	}
}

func TestResolve_ProviderPrefix(t *testing.T) {
	r := NewRegistry()
	m, err := r.Resolve("anthropic:sonnet")
	if err != nil {
		t.Fatal(err)
	}
	if m.ID != "claude-sonnet-4-20250514" {
		t.Errorf("ID = %q", m.ID)
	}
	if m.Provider != "anthropic" {
		t.Errorf("Provider = %q, want anthropic", m.Provider)
	}
}

func TestResolve_CaseInsensitive(t *testing.T) {
	r := NewRegistry()
	_, err := r.Resolve("Claude-Sonnet-4-20250514")
	if err != nil {
		t.Fatalf("case-insensitive resolve should work: %v", err)
	}
}

func TestResolve_PartialMatch(t *testing.T) {
	r := NewRegistry()
	m, err := r.Resolve("claude-opus")
	if err != nil {
		t.Fatal(err)
	}
	if m.Provider != "anthropic" {
		t.Errorf("Provider = %q, want anthropic", m.Provider)
	}
}

func TestResolve_NotFound(t *testing.T) {
	r := NewRegistry()
	_, err := r.Resolve("nonexistent-model-xyz")
	if err == nil {
		t.Error("should error for unknown model")
	}
}

func TestResolve_EmptyID(t *testing.T) {
	r := NewRegistry()
	_, err := r.Resolve("")
	if err == nil {
		t.Error("should error for empty model id")
	}
}

func TestRegisterAlias(t *testing.T) {
	r := NewRegistry()
	r.RegisterAlias("my-sonnet", "claude-sonnet-4-20250514")
	m, err := r.Resolve("my-sonnet")
	if err != nil {
		t.Fatal(err)
	}
	if m.ID != "claude-sonnet-4-20250514" {
		t.Errorf("ID = %q", m.ID)
	}
}

func TestAliases(t *testing.T) {
	aliases := Aliases()
	if len(aliases) == 0 {
		t.Error("should have aliases")
	}
	if aliases["sonnet"] != "claude-sonnet-4-20250514" {
		t.Errorf("sonnet alias = %q", aliases["sonnet"])
	}
}
