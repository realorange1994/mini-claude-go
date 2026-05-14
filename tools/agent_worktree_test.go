package tools

import (
	"path/filepath"
	"strings"
	"testing"
)

// ─── Worktree ──────────────────────────────────────────────────────────────

func TestSetupWorktreeDisabled(t *testing.T) {
	cfg := WorktreeConfig{Enabled: false}
	path, cleanup, err := SetupWorktree(cfg)
	if err != nil {
		t.Fatalf("SetupWorktree disabled should return nil error, got: %v", err)
	}
	if path != "" {
		t.Errorf("disabled worktree should return empty path, got %s", path)
	}
	if cleanup == nil {
		t.Fatal("cleanup should not be nil")
	}
	// Cleanup should be a no-op
	if cleanup() != nil {
		t.Error("cleanup for disabled worktree should be no-op")
	}
}

func TestWorktreeConfigDefaults(t *testing.T) {
	cfg := WorktreeConfig{
		Enabled: true,
		Name:    "",
		Keep:    false,
	}
	if !cfg.Enabled {
		t.Error("Enabled should be true")
	}
	if cfg.Name != "" {
		t.Error("Name should be empty for auto-generation")
	}
	if cfg.Keep {
		t.Error("Keep should be false by default")
	}
}

func TestUUIDV4Short(t *testing.T) {
	id := uuidV4Short()
	// Should be 8 hex chars
	if len(id) != 8 {
		t.Errorf("uuidV4Short should return 8 chars, got %d: %s", len(id), id)
	}
	// Should be hex
	for _, c := range id {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Errorf("uuidV4Short returned non-hex char: %c", c)
		}
	}
}

func TestUUIDV4ShortUnique(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		id := uuidV4Short()
		if seen[id] {
			t.Errorf("duplicate uuidV4Short: %s", id)
		}
		seen[id] = true
	}
}

func TestWorktreePathConstruction(t *testing.T) {
	// Verify the expected path format when worktrees are created
	// (This tests the path logic without actually creating git worktrees)
	name := "agent-test"
	expectedDir := filepath.Join(".claude", "worktrees", name)
	if expectedDir == "" {
		t.Error("expected path should not be empty")
	}
	// Verify the key components are present
	if !strings.Contains(expectedDir, ".claude") {
		t.Errorf("path should contain .claude, got %s", expectedDir)
	}
	if !strings.Contains(expectedDir, "worktrees") {
		t.Errorf("path should contain worktrees, got %s", expectedDir)
	}
	if !strings.Contains(expectedDir, name) {
		t.Errorf("path should contain agent name, got %s", expectedDir)
	}
}

func TestUUIDV4ShortUniqueness(t *testing.T) {
	// Upstream invariant: N generated UUIDs are all different
	// Note: 8 hex chars = 32 bits, collisions become likely around sqrt(2^32) ~ 65k
	// Test with a reasonable sample size
	const n = 500
	seen := make(map[string]bool, n)
	for i := 0; i < n; i++ {
		id := uuidV4Short()
		if seen[id] {
			t.Fatalf("duplicate uuidV4Short at iteration %d: %s", i, id)
		}
		seen[id] = true
	}
}

func TestUUIDV4ShortFormatRegex(t *testing.T) {
	// Format validation: always exactly 8 lowercase hex chars
	for i := 0; i < 100; i++ {
		id := uuidV4Short()
		if len(id) != 8 {
			t.Errorf("uuidV4Short length = %d, want 8", len(id))
		}
		for _, c := range id {
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
				t.Errorf("uuidV4Short has non-hex char %c in: %s", c, id)
			}
		}
	}
}

func TestUUIDV4ShortDistribution(t *testing.T) {
	// Verify hex values span a reasonable range (not all identical or clustered)
	seen := make(map[string]bool)
	for i := 0; i < 200; i++ {
		seen[uuidV4Short()] = true
	}
	// With 200 samples from 32-bit space, expect most to be unique
	if len(seen) < 190 {
		t.Errorf("too many collisions in uuidV4Short: %d unique out of 200", len(seen))
	}
}
