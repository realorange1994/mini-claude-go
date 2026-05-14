package tools

import "testing"

// Ported from upstream: packages/agent-tools/src/__tests__/registry.test.ts
// Tests for UpstreamToolAliases (toolMatchesName and findToolByName patterns)

func TestUpstreamToolAliasesAllEntries(t *testing.T) {
	// From upstream: verify all expected aliases are present
	expected := map[string]string{
		"read_file":  "Read",
		"write_file": "Write",
		"edit_file":  "Edit",
		"exec":       "Bash",
	}
	for internal, upstream := range expected {
		if got := InternalToUpstreamName(internal); got != upstream {
			t.Errorf("InternalToUpstreamName(%q) = %q, want %q", internal, got, upstream)
		}
	}
}

func TestUpstreamToolAliasesEmptyString(t *testing.T) {
	// From upstream: "handles undefined aliases" pattern
	got := InternalToUpstreamName("")
	if got != "" {
		t.Errorf("empty string should return empty, got %q", got)
	}
}

func TestUpstreamToolAliasesCompleteMap(t *testing.T) {
	if len(UpstreamToolAliases) != 4 {
		t.Errorf("expected 4 aliases, got %d", len(UpstreamToolAliases))
	}
}

func TestUpstreamToolAliasesNoCollisions(t *testing.T) {
	upstreamNames := make(map[string]string)
	for internal, upstream := range UpstreamToolAliases {
		if existing, ok := upstreamNames[upstream]; ok {
			t.Errorf("duplicate upstream name %q: mapped from both %q and %q", upstream, existing, internal)
		}
		upstreamNames[upstream] = internal
	}
}

// ─── Registry lookup patterns from upstream registry.test.ts ─────────────────

func TestRegistryGetByInternalName(t *testing.T) {
	r := NewRegistry()
	r.Register(&MemoryAddTool{})
	_, ok := r.Get("memory_add")
	if !ok {
		t.Error("should find tool by internal name")
	}
}

func TestRegistryLookupWithUpstreamAlias(t *testing.T) {
	// From upstream: "finds tool by alias" pattern
	r := NewRegistry()
	r.Register(&ExecTool{})
	_, ok := r.Get("exec")
	if !ok {
		t.Error("should find exec tool by internal name")
	}
	// Verify the upstream alias mapping
	upstream := InternalToUpstreamName("exec")
	if upstream != "Bash" {
		t.Errorf("exec should map to Bash, got %q", upstream)
	}
}

func TestRegistryUnknownNameReturnsNotFound(t *testing.T) {
	// From upstream: "returns undefined for unknown name"
	r := NewRegistry()
	r.Register(&MemoryAddTool{})
	_, ok := r.Get("nonexistent_tool")
	if ok {
		t.Error("should not find nonexistent tool")
	}
}

func TestRegistryEmptyArray(t *testing.T) {
	// From upstream: "handles empty tools array"
	r := NewRegistry()
	tools := r.AllTools()
	if len(tools) != 0 {
		t.Errorf("expected 0 tools, got %d", len(tools))
	}
}

func TestRegistryNoDuplicatesInAllTools(t *testing.T) {
	r := NewRegistry()
	r.Register(&MemoryAddTool{})
	r.Register(&MemorySearchTool{})
	tools := r.AllTools()
	seen := make(map[string]bool)
	for _, tool := range tools {
		if seen[tool.Name()] {
			t.Errorf("duplicate tool: %s", tool.Name())
		}
		seen[tool.Name()] = true
	}
}
