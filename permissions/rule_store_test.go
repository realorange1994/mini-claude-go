package permissions

import (
	"testing"
)

// ─── NewRuleStore ────────────────────────────────────────────────────────────

func TestNewRuleStore(t *testing.T) {
	store := NewRuleStore()
	if store == nil {
		t.Fatal("NewRuleStore should not return nil")
	}
}

// ─── AddRules / HasAllowRule / HasDenyRule / HasAskRule ─────────────────────

func TestAddRulesAndHasAllow(t *testing.T) {
	store := NewRuleStore()
	rule, _ := ParseRule("Bash")
	rule.Behavior = "allow"
	store.AddRules("session", "allow", []*ParsedRule{rule})

	if !store.HasAllowRule("Bash") {
		t.Error("Bash should have allow rule")
	}
}

func TestAddRulesAndHasDeny(t *testing.T) {
	store := NewRuleStore()
	rule, _ := ParseRule("Bash")
	rule.Behavior = "deny"
	store.AddRules("session", "deny", []*ParsedRule{rule})

	if !store.HasDenyRule("Bash") {
		t.Error("Bash should have deny rule")
	}
}

func TestAddRulesAndHasAsk(t *testing.T) {
	store := NewRuleStore()
	rule, _ := ParseRule("Bash")
	rule.Behavior = "ask"
	store.AddRules("session", "ask", []*ParsedRule{rule})

	if !store.HasAskRule("Bash") {
		t.Error("Bash should have ask rule")
	}
}

func TestHasAllowRuleEmpty(t *testing.T) {
	store := NewRuleStore()
	if store.HasAllowRule("Bash") {
		t.Error("empty store should not have allow rule")
	}
}

func TestHasDenyRuleEmpty(t *testing.T) {
	store := NewRuleStore()
	if store.HasDenyRule("Bash") {
		t.Error("empty store should not have deny rule")
	}
}

// ─── FindContentRule ────────────────────────────────────────────────────────

func TestFindContentRuleMatch(t *testing.T) {
	store := NewRuleStore()
	rule, _ := ParseRule("Bash(git:*)")
	rule.Behavior = "allow"
	store.AddRules("session", "allow", []*ParsedRule{rule})

	found := store.FindContentRule("Bash", "git:status", "allow")
	if found == nil {
		t.Error("should find content rule for git:status")
	}
}

func TestFindContentRuleNoMatch(t *testing.T) {
	store := NewRuleStore()
	rule, _ := ParseRule("Bash(git:*)")
	rule.Behavior = "allow"
	store.AddRules("session", "allow", []*ParsedRule{rule})

	found := store.FindContentRule("Bash", "python script", "allow")
	if found != nil {
		t.Error("should not find content rule for python script")
	}
}

func TestFindContentRuleWrongBehavior(t *testing.T) {
	store := NewRuleStore()
	rule, _ := ParseRule("Bash(git:*)")
	rule.Behavior = "allow"
	store.AddRules("session", "allow", []*ParsedRule{rule})

	found := store.FindContentRule("Bash", "git status", "deny")
	if found != nil {
		t.Error("should not find rule with wrong behavior")
	}
}

// ─── GetAllRules ────────────────────────────────────────────────────────────

func TestGetAllRulesEmpty(t *testing.T) {
	store := NewRuleStore()
	rules := store.GetAllRules("allow")
	if len(rules) != 0 {
		t.Errorf("expected 0 rules, got %d", len(rules))
	}
}

func TestGetAllRulesWithRules(t *testing.T) {
	store := NewRuleStore()
	r1, _ := ParseRule("Bash")
	r1.Behavior = "allow"
	r2, _ := ParseRule("Edit")
	r2.Behavior = "allow"
	store.AddRules("session", "allow", []*ParsedRule{r1, r2})

	rules := store.GetAllRules("allow")
	if len(rules) != 2 {
		t.Errorf("expected 2 rules, got %d", len(rules))
	}
}

func TestGetAllRulesFilterByBehavior(t *testing.T) {
	store := NewRuleStore()
	r1, _ := ParseRule("Bash")
	r1.Behavior = "allow"
	r2, _ := ParseRule("Edit")
	r2.Behavior = "deny"
	store.AddRules("session", "allow", []*ParsedRule{r1})
	store.AddRules("session", "deny", []*ParsedRule{r2})

	allowRules := store.GetAllRules("allow")
	denyRules := store.GetAllRules("deny")
	if len(allowRules) != 1 || len(denyRules) != 1 {
		t.Errorf("expected 1 allow + 1 deny, got %d allow + %d deny", len(allowRules), len(denyRules))
	}
}

// ─── GetRulesForTool ────────────────────────────────────────────────────────

func TestGetRulesForTool(t *testing.T) {
	store := NewRuleStore()
	r1, _ := ParseRule("Bash")
	r1.Behavior = "allow"
	r2, _ := ParseRule("Edit")
	r2.Behavior = "allow"
	store.AddRules("session", "allow", []*ParsedRule{r1, r2})

	rules := store.GetRulesForTool("Bash")
	if len(rules) != 1 {
		t.Errorf("expected 1 rule for Bash, got %d", len(rules))
	}
}

func TestGetRulesForToolEmpty(t *testing.T) {
	store := NewRuleStore()
	rules := store.GetRulesForTool("Bash")
	if len(rules) != 0 {
		t.Errorf("expected 0 rules, got %d", len(rules))
	}
}

// ─── MergeRuleStores ────────────────────────────────────────────────────────

func TestMergeRuleStoresEmpty(t *testing.T) {
	merged := MergeRuleStores()
	if merged == nil {
		t.Fatal("merged store should not be nil")
	}
}

func TestMergeRuleStores(t *testing.T) {
	s1 := NewRuleStore()
	r1, _ := ParseRule("Bash")
	r1.Behavior = "allow"
	s1.AddRules("session", "allow", []*ParsedRule{r1})

	s2 := NewRuleStore()
	r2, _ := ParseRule("Edit")
	r2.Behavior = "allow"
	s2.AddRules("cli", "allow", []*ParsedRule{r2})

	merged := MergeRuleStores(s1, s2)
	if !merged.HasAllowRule("Bash") {
		t.Error("merged should have Bash allow")
	}
	if !merged.HasAllowRule("Edit") {
		t.Error("merged should have Edit allow")
	}
}

// ─── Clone ──────────────────────────────────────────────────────────────────

func TestClone(t *testing.T) {
	store := NewRuleStore()
	rule, _ := ParseRule("Bash")
	rule.Behavior = "allow"
	store.AddRules("session", "allow", []*ParsedRule{rule})

	clone := store.Clone()
	if !clone.HasAllowRule("Bash") {
		t.Error("clone should have same rules")
	}

	// Modify clone shouldn't affect original
	r2, _ := ParseRule("Edit")
	r2.Behavior = "allow"
	clone.AddRules("session", "allow", []*ParsedRule{r2})
	if store.HasAllowRule("Edit") {
		t.Error("modifying clone should not affect original")
	}
}

func TestCloneEmpty(t *testing.T) {
	store := NewRuleStore()
	clone := store.Clone()
	if clone == nil {
		t.Fatal("clone of empty store should not be nil")
	}
}

// ─── SourceForRule ──────────────────────────────────────────────────────────

func TestSourceForRule(t *testing.T) {
	store := NewRuleStore()
	rule, _ := ParseRule("Bash")
	rule.Behavior = "allow"
	store.AddRules("session", "allow", []*ParsedRule{rule})

	source := store.SourceForRule(rule)
	if source != "session" {
		t.Errorf("expected source 'session', got %q", source)
	}
}

func TestSourceForRuleNotFound(t *testing.T) {
	store := NewRuleStore()
	rule, _ := ParseRule("Bash")
	rule.Behavior = "allow"

	source := store.SourceForRule(rule)
	if source != "" {
		t.Errorf("expected empty source, got %q", source)
	}
}

// ─── FormatRule ─────────────────────────────────────────────────────────────

func TestFormatRuleToolLevel(t *testing.T) {
	rule := &ParsedRule{ToolName: "Bash"}
	if FormatRule(rule) != "Bash" {
		t.Errorf("expected 'Bash', got %q", FormatRule(rule))
	}
}

func TestFormatRuleWithContent(t *testing.T) {
	rule := &ParsedRule{ToolName: "Bash", Content: "git:*"}
	result := FormatRule(rule)
	if result != "Bash(git:*)" {
		t.Errorf("expected 'Bash(git:*)', got %q", result)
	}
}
