package permissions

import (
	"os"
	"path/filepath"
	"testing"
)

// ─── LoadRulesFromConfig ─────────────────────────────────────────────────────

func TestLoadRulesFromConfigNil(t *testing.T) {
	store, err := LoadRulesFromConfig(nil, "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(store.GetAllRules("allow")) != 0 {
		t.Error("nil config should have no rules")
	}
}

func TestLoadRulesFromConfigAllow(t *testing.T) {
	cfg := &PermissionsConfig{Allow: []string{"Bash", "Edit"}}
	store, err := LoadRulesFromConfig(cfg, "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	rules := store.GetAllRules("allow")
	if len(rules) != 2 {
		t.Errorf("expected 2 allow rules, got %d", len(rules))
	}
}

func TestLoadRulesFromConfigDeny(t *testing.T) {
	cfg := &PermissionsConfig{Deny: []string{"Bash(rm -rf *)"}}
	store, err := LoadRulesFromConfig(cfg, "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	rules := store.GetAllRules("deny")
	if len(rules) != 1 {
		t.Errorf("expected 1 deny rule, got %d", len(rules))
	}
}

func TestLoadRulesFromConfigAsk(t *testing.T) {
	cfg := &PermissionsConfig{Ask: []string{"Bash(sudo *)"}}
	store, err := LoadRulesFromConfig(cfg, "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	rules := store.GetAllRules("ask")
	if len(rules) != 1 {
		t.Errorf("expected 1 ask rule, got %d", len(rules))
	}
}

func TestLoadRulesFromConfigAll(t *testing.T) {
	cfg := &PermissionsConfig{
		Allow: []string{"Bash"},
		Deny:  []string{"Bash(rm)"},
		Ask:   []string{"Bash(sudo)"},
	}
	store, err := LoadRulesFromConfig(cfg, "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	allowRules := store.GetAllRules("allow")
	denyRules := store.GetAllRules("deny")
	askRules := store.GetAllRules("ask")
	if len(allowRules) != 1 || len(denyRules) != 1 || len(askRules) != 1 {
		t.Errorf("expected 1/1/1 rules, got %d/%d/%d", len(allowRules), len(denyRules), len(askRules))
	}
}

func TestLoadRulesFromConfigInvalid(t *testing.T) {
	cfg := &PermissionsConfig{Allow: []string{"Bash(git:*"}}
	_, err := LoadRulesFromConfig(cfg, "test")
	if err == nil {
		t.Error("invalid rule should return error")
	}
}

// ─── LoadRulesFromFile ───────────────────────────────────────────────────────

func TestLoadRulesFromFileNotExist(t *testing.T) {
	_, _, err := LoadRulesFromFile("/nonexistent/path")
	if err == nil {
		t.Error("nonexistent file should return error")
	}
}

func TestLoadRulesFromFilePermissionsOnly(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	data := `{"permissions": {"allow": ["Bash"], "deny": ["Edit"]}}`
	err := os.WriteFile(path, []byte(data), 0644)
	if err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	store, source, err := LoadRulesFromFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if source == "" {
		t.Error("source should not be empty")
	}
	allowRules := store.GetAllRules("allow")
	if len(allowRules) != 1 {
		t.Errorf("expected 1 allow rule, got %d", len(allowRules))
	}
	denyRules := store.GetAllRules("deny")
	if len(denyRules) != 1 {
		t.Errorf("expected 1 deny rule, got %d", len(denyRules))
	}
}

func TestLoadRulesFromFileFullSettings(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	data := `{"permissions": {"allow": ["Read"], "ask": ["Write"]}}`
	err := os.WriteFile(path, []byte(data), 0644)
	if err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	store, _, err := LoadRulesFromFile(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	allowRules := store.GetAllRules("allow")
	askRules := store.GetAllRules("ask")
	if len(allowRules) != 1 || len(askRules) != 1 {
		t.Errorf("expected 1 allow + 1 ask, got %d/%d", len(allowRules), len(askRules))
	}
}

// ─── sourceFromPath ─────────────────────────────────────────────────────────

func TestSourceFromPathUserSettings(t *testing.T) {
	home, _ := os.UserHomeDir()
	if home == "" {
		t.Skip("no home directory")
	}
	p := filepath.Join(home, ".claude", "settings.json")
	source := sourceFromPath(p)
	if source != "userSettings" {
		t.Errorf("expected 'userSettings', got %q", source)
	}
}

func TestSourceFromPathProjectSettings(t *testing.T) {
	p := "/project/.claude/settings.json"
	source := sourceFromPath(p)
	if source != "projectSettings" {
		t.Errorf("expected 'projectSettings', got %q", source)
	}
}

func TestSourceFromPathLocalSettings(t *testing.T) {
	p := "/project/.claude/settings.local.json"
	source := sourceFromPath(p)
	if source != "localSettings" {
		t.Errorf("expected 'localSettings', got %q", source)
	}
}

func TestSourceFromPathUnknown(t *testing.T) {
	p := "/some/other/config.yaml"
	source := sourceFromPath(p)
	if source != "unknown" {
		t.Errorf("expected 'unknown', got %q", source)
	}
}

// ─── isHomeDir ───────────────────────────────────────────────────────────────

func TestIsHomeDir(t *testing.T) {
	home, _ := os.UserHomeDir()
	if home == "" {
		t.Skip("no home directory")
	}
	claudeDir := filepath.Join(home, ".claude")
	if !isHomeDir(claudeDir) {
		t.Error("should detect home .claude dir")
	}
}

func TestIsHomeDirFalse(t *testing.T) {
	if isHomeDir("/some/random/path") {
		t.Error("random path should not be home dir")
	}
}

// ─── PermissionsConfig ───────────────────────────────────────────────────────

func TestPermissionsConfigStruct(t *testing.T) {
	cfg := PermissionsConfig{
		Allow: []string{"Bash"},
		Deny:  []string{"Edit"},
		Ask:   []string{"Read"},
	}
	if len(cfg.Allow) != 1 || len(cfg.Deny) != 1 || len(cfg.Ask) != 1 {
		t.Error("PermissionsConfig should have all fields")
	}
}
